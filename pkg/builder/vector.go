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
	"github.com/zncdatadev/operator-go/pkg/constants"
	"github.com/zncdatadev/operator-go/pkg/util"
)

const (
	VectorImage         = "timberio/vector:0.38.0-alpine"
	VectorContainerName = "vector"
	VectorConfigFile    = "vector.yaml"

	VectorConfigVolumeName = "config"
	VectorLogVolumeName    = "log"
)

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
		"LogDir":                  constants.KubedoopLogDir,
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
data_dir: /kubedoop/vector/var
log_schema:
	host_key: "pod"
sources:
  vector:
    type: internal_logs

  files_stdout:
    type: file
    include:
      - {{.LogDir}}*/*.stdout.log

  files_stderr:
    type: file
    include:
      - {{.LogDir}}*/*.stderr.log

  files_log4j:
    type: file
    include:
      - {{.LogDir}}*/*.log4j.xml
    line_delimiter: "\r\n"
    multiline:
      mode: halt_before
      start_pattern: ^<log4j:event
      condition_pattern: ^<log4j:event
      timeout_ms: 1000

  files_log4j2:
    type: file
    include:
      - {{.LogDir}}*/*.log4j2.xml
    line_delimiter: "\r\n"

	files_airlift:
		type: "file"
		include:
			- "{{.LogDir}}*/*.airlift.json"
transforms:
  processed_files_stdout:
    inputs:
      - files_stdout
    type: remap
    source: |
      .logger = "ROOT"
      .level = "INFO"

  processed_files_stderr:
    inputs:
      - files_stderr
    type: remap
    source: |
      .logger = "ROOT"
      .level = "ERROR"

  processed_files_log4j:
    inputs:
      - files_log4j
    type: remap
    source: |
      raw_message = string!(.message)

      .timestamp = now()
      .logger = ""
      .level = "INFO"
      .message = ""
      .errors = []

      # Wrap the event so that the log4j namespace is defined when parsing the event
      wrapped_xml_event = "<root xmlns:log4j=\"http://jakarta.apache.org/log4j/\">" + raw_message + "</root>"
      parsed_event, err = parse_xml(wrapped_xml_event)
      if err != null {{"{{"}}
        error = "XML not parsable: " + err
        .errors = push(.errors, error)
        log(error, level: "warn")
        .message = raw_message
      {{"}}"}} else {{"{{"}}
        root = object!(parsed_event.root)
        if !is_object(root.event) {{"{{"}}
          error = "Parsed event contains no \"event\" tag."
          .errors = push(.errors, error)
          log(error, level: "warn")
          .message = raw_message
        {{"}}"}} else {{"{{"}}
          if keys(root) != ["event"] {{"{{"}}
            .errors = push(.errors, "Parsed event contains multiple tags: " + join!(keys(root), ", "))
          {{"}}"}}
          event = object!(root.event)

          epoch_milliseconds, err = to_int(event.@timestamp)
          if err == null && epoch_milliseconds != 0 {{"{{"}}
            converted_timestamp, err = from_unix_timestamp(epoch_milliseconds, "milliseconds")
            if err == null {{"{{"}}
              .timestamp = converted_timestamp
            {{"}}"}} else {{"{{"}}
              .errors = push(.errors, "Time not parsable, using current time instead: " + err)
            {{"}}"}}
          {{"}}"}} else {{"{{"}}
            .errors = push(.errors, "Timestamp not found, using current time instead.")
          {{"}}"}}

          .logger, err = string(event.@logger)
          if err != null || is_empty(.logger) {{"{{"}}
            .errors = push(.errors, "Logger not found.")
          {{"}}"}}

          level, err = string(event.@level)
          if err != null {{"{{"}}
            .errors = push(.errors, "Level not found, using \"" + .level + "\" instead.")
          {{"}}"}} else if !includes(["TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL"], level) {{"{{"}}
            .errors = push(.errors, "Level \"" + level + "\" unknown, using \"" + .level + "\" instead.")
          {{"}}"}} else {{"{{"}}
            .level = level
          {{"}}"}}

          message, err = string(event.message)
          if err != null || is_empty(message) {{"{{"}}
            .errors = push(.errors, "Message not found.")
          {{"}}"}}
          throwable = string(event.throwable) ?? ""
          .message = join!(compact([message, throwable]), "\n")
        {{"}}"}}
      {{"}}"}}

  processed_files_log4j2:
    inputs:
      - files_log4j2
    type: remap
    source: |
      raw_message = string!(.message)

      .timestamp = now()
      .logger = ""
      .level = "INFO"
      .message = ""
      .errors = []

      event = {{"{{"}}}}
      parsed_event, err = parse_xml(raw_message)
      if err != null {{"{{"}}
        error = "XML not parsable: " + err
        .errors = push(.errors, error)
        log(error, level: "warn")
        .message = raw_message
      {{"}}"}} else {{"{{"}}
        if !is_object(parsed_event.Event) {{"{{"}}
          error = "Parsed event contains no \"Event\" tag."
          .errors = push(.errors, error)
          log(error, level: "warn")
          .message = raw_message
        {{"}}"}} else {{"{{"}}
          event = object!(parsed_event.Event)

          tag_instant_valid = false
          instant, err = object(event.Instant)
          if err == null {{"{{"}}
            epoch_nanoseconds, err = to_int(instant.@epochSecond) * 1_000_000_000 + to_int(instant.@nanoOfSecond)
            if err == null && epoch_nanoseconds != 0 {{"{{"}}
              converted_timestamp, err = from_unix_timestamp(epoch_nanoseconds, "nanoseconds")
              if err == null {{"{{"}}
                .timestamp = converted_timestamp
                tag_instant_valid = true
              {{"}}"}} else {{"{{"}}
                .errors = push(.errors, "Instant invalid, trying property timeMillis instead: " + err)
              {{"}}"}}
            {{"}}"}} else {{"{{"}}
              .errors = push(.errors, "Instant invalid, trying property timeMillis instead: " + err)
            {{"}}"}}
          {{"}}"}}
          if !tag_instant_valid {{"{{"}}
            epoch_milliseconds, err = to_int(event.@timeMillis)
            if err == null && epoch_milliseconds != 0 {{"{{"}}
              converted_timestamp, err = from_unix_timestamp(epoch_milliseconds, "milliseconds")
              if err == null {{"{{"}}
                .timestamp = converted_timestamp
              {{"}}"}} else {{"{{"}}
                .errors = push(.errors, "timeMillis not parsable, using current time instead: " + err)
              {{"}}"}}
            {{"}}"}} else {{"{{"}}
              .errors = push(.errors, "timeMillis not parsable, using current time instead: " + err)
            {{"}}"}}
          {{"}}"}}

          .logger, err = string(event.@loggerName)
          if err != null || is_empty(.logger) {{"{{"}}
            .errors = push(.errors, "Logger not found.")
          {{"}}"}}

          level, err = string(event.@level)
          if err != null {{"{{"}}
            .errors = push(.errors, "Level not found, using \"" + .level + "\" instead.")
          {{"}}"}} else if !includes(["TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL"], level) {{"{{"}}
            .errors = push(.errors, "Level \"" + level + "\" unknown, using \"" + .level + "\" instead.")
          {{"}}"}} else {{"{{"}}
            .level = level
          {{"}}"}}

          exception = null
          thrown = event.Thrown
          if is_object(thrown) {{"{{"}}
            exception = "Exception"
            thread, err = string(event.@thread)
            if err == null && !is_empty(thread) {{"{{"}}
              exception = exception + " in thread \"" + thread + "\""
            {{"}}"}}
            thrown_name, err = string(thrown.@name)
            if err == null && !is_empty(exception) {{"{{"}}
              exception = exception + " " + thrown_name
            {{"}}"}}
            message = string(thrown.@localizedMessage) ??
              string(thrown.@message) ??
              ""
            if !is_empty(message) {{"{{"}}
              exception = exception + ": " + message
            {{"}}"}}
            stacktrace_items = array(thrown.ExtendedStackTrace.ExtendedStackTraceItem) ?? []
            stacktrace = ""
            for_each(stacktrace_items) -> |_index, value| {{"{{"}}
              stacktrace = stacktrace + "        "
              class = string(value.@class) ?? ""
              method = string(value.@method) ?? ""
              if !is_empty(class) && !is_empty(method) {{"{{"}}
                stacktrace = stacktrace + "at " + class + "." + method
              {{"}}"}}
              file = string(value.@file) ?? ""
              line = string(value.@line) ?? ""
              if !is_empty(file) && !is_empty(line) {{"{{"}}
                stacktrace = stacktrace + "(" + file + ":" + line + ")"
              {{"}}"}}
              exact = to_bool(value.@exact) ?? false
              location = string(value.@location) ?? ""
              version = string(value.@version) ?? ""
              if !is_empty(location) && !is_empty(version) {{"{{"}}
                stacktrace = stacktrace + " "
                if !exact {{"{{"}}
                  stacktrace = stacktrace + "~"
                {{"}}"}}
                stacktrace = stacktrace + "[" + location + ":" + version + "]"
              {{"}}"}}
              stacktrace = stacktrace + "\n"
            {{"}}"}}
            if stacktrace != "" {{"{{"}}
              exception = exception + "\n" + stacktrace
            {{"}}"}}
          {{"}}"}}

          message, err = string(event.Message)
          if err != null || is_empty(message) {{"{{"}}
            message = null
            .errors = push(.errors, "Message not found.")
          {{"}}"}}
          .message = join!(compact([message, exception]), "\n")
        {{"}}"}}
      {{"}}"}}

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
			. |= parse_regex!(.file, r'^/kubedoop/log/(?P<container>.*?)/(?P<file>.*?)$')
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

rm -f {{ .LogDir }}_vector/shutdown
prepare_signal_handlers

{{ .EntrypointScript }}

wait_for_termination $!
mkdir -p {{ .LogDir }}_vector && touch {{ .LogDir }}_vector/shutdown
`
	data := map[string]interface{}{"LogDir": constants.KubedoopLogDir, "EntrypointScript": entrypointScript}
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
