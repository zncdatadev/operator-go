package builder

import (
	"context"
	"fmt"
	"slices"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	pkgclient "github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/config"
	"github.com/zncdatadev/operator-go/pkg/util"
)

// todo: in future,  all operator should config log, config and data to the same dir, like '/zncdata/log', '/zncdata/config'
const (
	VectorImage         = "timberio/vector:0.38.0-alpine"
	VectorContainerName = "vector"
	ConfigDir           = "/zncdata/config"
	LogDir              = "/zncdata/log"
)

func VectorVolumeMount(vectorConfigVolumeName string, vectorLogVolumeName string) []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      vectorConfigVolumeName,
			MountPath: ConfigDir,
		},
		{
			Name:      vectorLogVolumeName,
			MountPath: LogDir,
		},
	}
}

func VectorCommandArgs() []string {
	arg := `log_dir="/zncdata/log/_vector"
data_dir="/zncdata/vector/var"
if [ ! -d "$data_dir" ]; then
	mkdir -p "$data_dir"
fi

vector --config /zncdata/config/vector.yaml &
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

func MakeVectorYaml(
	ctx context.Context,
	client ctrlclient.Client,
	namespace string,
	cluster string,
	role string,
	groupName string,
	vectorAggregatorDiscovery string) (string, error) {
	vectorAggregatorDiscoveryURI := vectorAggregatorDiscoveryURI(ctx, client, namespace, vectorAggregatorDiscovery)
	data := map[string]interface{}{
		"LogDir":                  LogDir,
		"Namespace":               namespace,
		"Cluster":                 cluster,
		"Role":                    role,
		"GroupName":               groupName,
		"VectorAggregatorAddress": vectorAggregatorDiscoveryURI,
	}
	return ParseVectorYaml(data)
}

func ParseVectorYaml(data map[string]interface{}) (string, error) {
	var tmpl = `api:
	enabled: true
data_dir: /zncdata/vector/var
log_schema:
	host_key: "pod"
sources:
	files_airlift:
	type: "file"
	include:
		- "{{.LogDir}}/*/*.airlift.json"
transforms:
	processed_files_airlift:
	inputs:
		- files_airlift
	type: remap
	source: |
		parsed_event = parse_json!(string!(.message))
		.message = join!(compact([parsed_event.message, parsed_event.stackTrace]), "\n")
		.timestamp = parse_timestamp!(parsed_event.timestamp, "%Y-%m-%dT%H:%M:%S.%fZ")
		.logger = parsed_event.logger
		.level = parsed_event.level
		.thread = parsed_event.thread
	extended_logs_files:
		inputs:
			- processed_files_*
		type: remap
		source: |
			. |= parse_regex!(.file, r'^/zncdata/log/(?P<container>.*?)/(?P<file>.*?)$')
			del(.source_type)
	extended_logs:
		inputs:
			- extended_logs_*
		type: remap
		source: |
			.namespace = "{{.Namespace}}"
			.cluster = "{{.Cluster}}"
			.role = "{{.Role}}"
			.roleGroup = "{{.GroupName}}"
sinks:
	aggregator:
		inputs:
			- extended_logs
		type: vector
		address: "{{.VectorAggregatorAddress}}"
`
	parser := config.TemplateParser{
		Value:    data,
		Template: tmpl,
	}

	str, err := parser.Parse()
	if err != nil {
		return "", err
	}
	str = util.IndentTab2Spaces(str)
	return str, nil
}

func vectorAggregatorDiscoveryURI(
	ctx context.Context,
	client ctrlclient.Client,
	namespace string,
	discoveryConfigName string) *string {
	if discoveryConfigName != "" {
		cli := pkgclient.Client{Client: client}
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      discoveryConfigName,
				Namespace: namespace,
			},
		}
		err := cli.Get(ctx, cm)
		if err != nil {
			return nil
		}
		address := cm.Data["ADDRESS"]
		return &address
	}
	return nil
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

rm -f {{ .LogDir }}/_vector/shutdown
prepare_signal_handlers

{{ .EntrypointScript }}

wait_for_termination $!
mkdir -p {{ .LogDir }}/_vector && touch {{ .LogDir }}/_vector/shutdown
`
	data := map[string]interface{}{"LogDir": LogDir, "EntrypointScript": entrypointScript}
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

var _ WorkloadDecorator = &VectorDecorator{}

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
func (v *VectorDecorator) appendSharedVolumeMount(containers *[]corev1.Container) {
	if len(*containers) == 0 {
		panic("containers is empty")
	}
	for i, container := range *containers {
		if slices.Contains(v.LogProviderContainerName, container.Name) { // if log provider container
			if !v.volumeMountExists(container.VolumeMounts, v.LogVolumeName) { // if log volume mount exists
				container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
					Name:      v.LogVolumeName,
					MountPath: LogDir,
				})
				(*containers)[i] = container
			}
			if !v.volumeMountExists(container.VolumeMounts, v.VectorConfigVolumeName) { // if vector config volume mount exists
				container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
					Name:      v.VectorConfigVolumeName,
					MountPath: ConfigDir,
				})
				(*containers)[i] = container
			}
		}
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
