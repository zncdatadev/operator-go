package builder

import (
	"fmt"
	"path"
	"slices"

	"github.com/zncdatadev/operator-go/pkg/config"
	"github.com/zncdatadev/operator-go/pkg/constants"
	"github.com/zncdatadev/operator-go/pkg/util"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	VectorContainerName = "vector"
	VectorConfigFile    = "vector.yaml"

	VectorConfigVolumeName = "config"
	VectorLogVolumeName    = "log"

	vectorDataDirVolumeName = "vector-data"
	vectorDataDir           = constants.KubedoopRoot + "vector/var"
	vectorApiPort           = 8686
)

var (
	VectorLogDir       = path.Join(constants.KubedoopLogDir, "_vector")
	VectorShutdownFile = path.Join(VectorLogDir, "shutdown")
)

var _ WorkloadDecorator = &VectorDecorator{}

func NewVectorDecorator(
	workloadObject ctrlclient.Object,
	image *util.Image,
	logVolumeName string,
	vectorConfigVolumeName string,
	vectorConfigMapName string,
) *VectorDecorator {
	return &VectorDecorator{
		WorkloadObject:         workloadObject,
		Image:                  image,
		LogVolumeName:          logVolumeName,
		VectorConfigVolumeName: vectorConfigVolumeName,
		VectorConfigMapName:    vectorConfigMapName,
	}
}

type VectorDecorator struct {
	WorkloadObject ctrlclient.Object
	Image          *util.Image

	LogVolumeName          string
	VectorConfigVolumeName string
	VectorConfigMapName    string

	LogProviderContainerName []string // optional
}

func (v *VectorDecorator) Decorate() error {
	// assert WorkloadObject is a statefulset or deployment
	var volumes *[]corev1.Volume
	var containers *[]corev1.Container
	switch o := v.WorkloadObject.(type) {
	case *appv1.StatefulSet:
		volumes = &o.Spec.Template.Spec.Volumes
		containers = &o.Spec.Template.Spec.Containers

	case *appv1.Deployment:
		volumes = &o.Spec.Template.Spec.Volumes
		containers = &o.Spec.Template.Spec.Containers
	default:
		return fmt.Errorf("unsupported workload object type %T", o)
	}

	*volumes = append(*volumes, corev1.Volume{
		Name: vectorDataDirVolumeName, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}})
	// append shared log volume to workload
	if !v.volumeExists(*volumes, v.LogVolumeName) {
		*volumes = append(*volumes, v.createLogVolume())
	}
	// append shared vector config volume to workload
	if !v.volumeExists(*volumes, v.VectorConfigVolumeName) {
		*volumes = append(*volumes, v.createVectorConfigVolume())
	}
	// log provider container must share log dir by volume mount
	v.appendSharedVolumeMount(containers)
	// workload object add vector container
	v.appendVectorContainer(containers)
	return nil
}

// append shared log volume to workload

// check if the volume exists by volume name
func (v *VectorDecorator) volumeExists(volumes []corev1.Volume, volumeName string) bool {
	for _, volume := range volumes {
		if volume.Name == volumeName {
			return true
		}
	}
	return false
}

// check if the volume mount exists by volume name
func (v *VectorDecorator) volumeMountExists(volumeMounts []corev1.VolumeMount, volumeMountName string) bool {
	for _, volumeMount := range volumeMounts {
		if volumeMount.Name == volumeMountName {
			return true
		}
	}
	return false
}

// create shared log volume
func (v *VectorDecorator) createLogVolume() corev1.Volume {
	return corev1.Volume{
		Name: v.LogVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				SizeLimit: func() *resource.Quantity {
					r := resource.MustParse("33Mi")
					return &r
				}(),
			},
		},
	}
}

// create shared vector config volume
func (v *VectorDecorator) createVectorConfigVolume() corev1.Volume {
	return corev1.Volume{
		Name: v.VectorConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: v.VectorConfigMapName,
				},
			},
		},
	}
}

// log provider container must share log dir and vector config dir
/*
* appendSharedVolumeMount iterates over the containers and appends shared volume mounts based on the LogProviderContainerName.
* If LogProviderContainerName is not empty, it checks if the container's name is in the LogProviderContainerName list and appends volume mounts accordingly.
* If LogProviderContainerName is empty, it appends volume mounts for all containers.
*
* Parameters:
* containers: A pointer to a slice of corev1.Container representing the containers to append volume mounts to.
 */
func (v *VectorDecorator) appendSharedVolumeMount(containers *[]corev1.Container) {
	if len(*containers) == 0 {
		panic("containers is empty")
	}
	for i, container := range *containers {
		if len(v.LogProviderContainerName) != 0 {
			if slices.Contains(v.LogProviderContainerName, container.Name) {
				v.appendVectorVolumeMounts(&container, containers, i)
			}
		} else {
			v.appendVectorVolumeMounts(&container, containers, i)
		}
	}
}

func (v *VectorDecorator) appendVectorVolumeMounts(container *corev1.Container, containers *[]corev1.Container, i int) {
	if !v.volumeMountExists(container.VolumeMounts, v.LogVolumeName) { // if log volume mount exists
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      v.LogVolumeName,
			MountPath: constants.KubedoopLogDir,
		})
		(*containers)[i] = *container
	}
	if !v.volumeMountExists(container.VolumeMounts, v.VectorConfigVolumeName) { // if vector config volume mount exists
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      v.VectorConfigVolumeName,
			MountPath: constants.KubedoopConfigDir,
		})
		(*containers)[i] = *container
	}
}

// append vector container
func (v *VectorDecorator) appendVectorContainer(containers *[]corev1.Container) {
	*containers = append(*containers, *v.NewVectorContainer())
}

func (v *VectorDecorator) NewVectorContainer() *corev1.Container {
	// see https://vector.dev/docs/reference/api/
	// and see https://github.com/vectordotdev/helm-charts/blob/develop/charts/vector/values.yaml
	// requires api.enabled to be set to true.
	var vectorReadinessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Port: intstr.FromInt(vectorApiPort),
				Path: "/health",
			},
		},
		InitialDelaySeconds: 5,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		FailureThreshold:    3,
		TimeoutSeconds:      1,
	}
	vectorContainer := NewContainerBuilder(VectorContainerName, v.Image).
		SetCommand(VectorCommand()).
		SetArgs(VectorCommandArgs()).
		SetReadinessProbe(vectorReadinessProbe).
		AddPorts([]corev1.ContainerPort{
			{
				Name:          "api",
				ContainerPort: vectorApiPort,
			},
		}).
		AddVolumeMounts(VectorVolumeMount(v.VectorConfigVolumeName, v.LogVolumeName)).
		Build()

	return vectorContainer
}

func VectorVolumeMount(vectorConfigVolumeName string, vectorLogVolumeName string) []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      vectorConfigVolumeName,
			MountPath: constants.KubedoopConfigDir,
		},
		{
			Name:      vectorLogVolumeName,
			MountPath: constants.KubedoopLogDir,
		},
		{
			Name:      vectorDataDirVolumeName,
			MountPath: vectorDataDir,
		},
	}
}

func VectorCommandArgs() []string {
	arg := fmt.Sprintf(`
# Vector will ignore SIGTERM (as PID != 1) and must be shut down by writing a shutdown trigger file
CONFIG_DIR=%s
LOG_DIR=%s
VECTOR_LOG_DIR=%s
VECTOR_SHUTDOWN_FILE=%s
VECTOR_CONFIG_FILE=%s
vector --config ${CONFIG_DIR}${VECTOR_CONFIG_FILE} & vector_pid=$!
if [ ! -f "${LOG_DIR}${VECTOR_LOG_DIR}$/${VECTOR_SHUTDOWN_FILE}" ]; then
  mkdir -p ${LOG_DIR}${VECTOR_LOG_DIR} && \
  inotifywait -qq --event create ${LOG_DIR}${VECTOR_LOG_DIR}; \
fi
sleep 1
kill $vector_pid
`, constants.KubedoopConfigDir, constants.KubedoopLogDir, VectorLogDir, VectorShutdownFile, VectorConfigFile)
	return []string{util.IndentTab4Spaces(arg)}
}

func VectorCommand() []string {
	return []string{
		"/bin/bash",
		"-x",
		"-euo",
		"pipefail",
		"-c",
	}
}

// ============= log provider container ================

func LogProviderCommand(entrypointScript string) ([]string, error) {
	template := `
prepare_signal_handlers()
{
	unset term_child_pid
	unset term_kill_needed
	trap 'handle_term_signal' TERM
}

handle_term_signal()
{
	if [ "${term_child_pid}" ]; then
		kill -TERM "${term_child_pid}" 2>/dev/null
	else
		term_kill_needed="yes"
	fi
}

wait_for_termination()
{
	set +e
	term_child_pid=$1
	if [[ -v term_kill_needed ]]; then
		kill -TERM "${term_child_pid}" 2>/dev/null
	fi
	wait ${term_child_pid} 2>/dev/null
	trap - TERM
	wait ${term_child_pid} 2>/dev/null
	set -e
}

rm -f {{ .VectorShutdownFile }}
prepare_signal_handlers

{{ .EntrypointScript }}

wait_for_termination $!
mkdir -p {{ .VectorLogDir }} && touch {{ .VectorShutdownFile }}
`
	data := map[string]interface{}{
		"LogDir":             constants.KubedoopLogDir,
		"EntrypointScript":   entrypointScript,
		"VectorLogDir":       VectorLogDir,
		"VectorShutdownFile": VectorShutdownFile,
	}
	parser := config.TemplateParser{
		Value:    data,
		Template: template,
	}
	res, err := parser.Parse()
	if err != nil {
		return nil, err
	}
	res = util.IndentTab4Spaces(res)
	return []string{res}, nil

}

type WorkloadDecorator interface {
	Decorate() error
}
