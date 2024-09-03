package builder

import (
	"fmt"
	"slices"

	"github.com/zncdatadev/operator-go/pkg/constants"
	"github.com/zncdatadev/operator-go/pkg/productlogging"
	"github.com/zncdatadev/operator-go/pkg/util"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	VectorImage         = "timberio/vector:0.38.0-alpine"
	VectorContainerName = "vector"
	VectorConfigFile    = "vector.yaml"

	VectorConfigVolumeName = "config"
	VectorLogVolumeName    = "log"
)

var _ productlogging.WorkloadDecorator = &VectorDecorator{}

type VectorDecorator struct {
	WorkloadObject ctrlclient.Object

	LogVolumeName          string
	VectorConfigVolumeName string
	VectorConfigMapName    string

	LogProviderContainerName []string
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
	//append shared log volume to workload
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

	vectorContainer := NewContainerBuilder(VectorContainerName, VectorImage).
		SetImagePullPolicy(DefaultImagePullPolicy).
		SetCommand(VectorCommand()).
		SetArgs(VectorCommandArgs()).
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
	}
}

func VectorCommandArgs() []string {
	arg := `log_dir="/kubedoop/log/_vector"
data_dir="/kubedoop/vector/var"
if [ ! -d "$data_dir" ]; then
	mkdir -p "$data_dir"
fi

vector --config /kubedoop/config/vector.yaml &
vector_pid=$!

if [ ! -f "$log_dir/shutdown" ]; then
	mkdir -p "$log_dir"
fi

previous_count=$(ls -1 "$log_dir" | wc -l)

while true; do
	current_count=$(ls -1 "$log_dir" | wc -l)

	if [ "$current_count" -gt "$previous_count" ]; then
		new_file=$(ls -1 "$log_dir" | tail -n 1)
		echo "New file created: $new_file"

		previous_count=$current_count
	fi

	if [ -f "$log_dir/shutdown" ]; then
		kill $vector_pid
		break
	fi

	sleep 1
done
`
	return []string{util.IndentTab4Spaces(arg)}
}

func VectorCommand() []string {
	return []string{
		"ash",
		"-euo",
		"pipefail",
		"-c",
	}
}
