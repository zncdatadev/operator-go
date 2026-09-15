package pipeline

import (
	"fmt"
	"path"
	"reflect"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// CheckAssembly reports independent structural and modeled-consumer relations.
// It does not repair the Pod or replace Kubernetes admission/product validation.
// A changed execution premise makes that consumer unknown, not every relation.
func CheckAssembly(expected, actual GroupResources, generated RuntimeDescription, files []File) []Check {
	pod, baseline := actual.StatefulSet.Spec.Template, expected.StatefulSet.Spec.Template
	checks := checkPodStructure(pod.Spec, actual.StatefulSet.Spec.VolumeClaimTemplates...)
	checks = append(checks, checkRetainedData(expected, actual, generated)...)
	checks = append(checks, checkPlatformVolumes(expected, actual, generated)...)
	checks = append(checks, checkLifecycle(expected, actual, generated)...)
	checks = append(checks, checkResourceSelectors(actual)...)
	main := findContainer(pod, generated.Main.Name)
	mainKnown := processPremise(generated.Main, main) && main != nil && main.Image == generated.Main.Image
	if main == nil {
		checks = append(checks, Check{Subject: "assembly.main", State: Conflict,
			Reason: "declared main container is missing"})
	} else if !mainKnown {
		checks = append(checks, Check{Subject: "assembly.main.execution", State: Unknown,
			Reason: "main image, command, args, environment or working directory changed"})
	} else {
		checks = append(checks, Check{Subject: "assembly.main.execution", State: Consistent,
			Reason: "declared main execution premise is retained"})
	}
	helperCheck, helperKnown := checkMaterializer(baseline.Spec, pod.Spec, len(files) > 0)
	checks = append(checks, helperCheck...)
	checks = append(checks, checkPreparedAccess(pod.Spec, generated, files, mainKnown, helperKnown)...)
	checks = append(checks, checkVectorConsumer(baseline, pod, generated, files, mainKnown, helperKnown)...)
	return checks
}

func checkPodStructure(pod corev1.PodSpec, claims ...corev1.PersistentVolumeClaim) []Check {
	checks := []Check{}
	volumes := map[string]bool{}
	for _, claim := range claims {
		if claim.Name == "" || volumes[claim.Name] {
			checks = append(checks, Check{Subject: "statefulset.volumeClaimTemplates", State: Conflict,
				Reason: "empty or duplicate claim template name"})
		}
		volumes[claim.Name] = true
	}
	for _, volume := range pod.Volumes {
		if volume.Name == "" || volumes[volume.Name] {
			checks = append(checks, Check{Subject: "pod.volumes", State: Conflict,
				Reason: fmt.Sprintf("empty or duplicate volume name %q", volume.Name)})
		}
		volumes[volume.Name] = true
	}
	containers := map[string]bool{}
	for _, container := range append(slices.Clone(pod.InitContainers), pod.Containers...) {
		if container.Name == "" || containers[container.Name] {
			checks = append(checks, Check{Subject: "pod.containers", State: Conflict,
				Reason: fmt.Sprintf("empty or duplicate container name %q", container.Name)})
		}
		containers[container.Name] = true
		mounts := map[string]bool{}
		for _, mount := range container.VolumeMounts {
			subject := fmt.Sprintf("pod.container[%s].mount[%s]", container.Name, mount.MountPath)
			if mounts[mount.MountPath] {
				checks = append(checks, Check{Subject: subject, State: Conflict, Reason: "duplicate mount path"})
			}
			mounts[mount.MountPath] = true
			if !volumes[mount.Name] {
				checks = append(checks, Check{Subject: subject, State: Conflict,
					Reason: fmt.Sprintf("volume %q is missing", mount.Name)})
			}
		}
	}
	return checks
}

func checkResourceSelectors(resources GroupResources) []Check {
	checks := []Check{}
	podLabels := labels.Set(resources.StatefulSet.Spec.Template.Labels)
	selector, err := metav1.LabelSelectorAsSelector(resources.StatefulSet.Spec.Selector)
	if err != nil || selector.Empty() || !selector.Matches(podLabels) {
		checks = append(checks, Check{Subject: "statefulset.selector", State: Conflict,
			Reason: "selector does not identify the final Pod template"})
	} else {
		checks = append(checks, Check{Subject: "statefulset.selector", State: Consistent,
			Reason: "selector matches the final Pod template labels"})
	}
	if resources.StatefulSet.Spec.ServiceName != resources.HeadlessService.Name {
		checks = append(checks, Check{Subject: "statefulset.serviceName", State: Conflict,
			Reason: "governing headless Service name does not match"})
	}
	for _, service := range []corev1.Service{resources.Service, resources.HeadlessService} {
		subject := fmt.Sprintf("service[%s].selector", service.Name)
		selector := labels.SelectorFromSet(service.Spec.Selector)
		state, reason := Consistent, "Service selector matches the final Pod template labels"
		if selector.Empty() || !selector.Matches(podLabels) {
			state, reason = Conflict, "Service selector does not identify the final Pod template"
		}
		checks = append(checks, Check{Subject: subject, State: state, Reason: reason})
		checks = append(checks, checkServicePorts(service, resources.StatefulSet.Spec.Template.Spec)...)
	}
	return checks
}

func portProtocol(protocol corev1.Protocol) corev1.Protocol {
	if protocol == "" {
		return corev1.ProtocolTCP
	}
	return protocol
}

func checkServicePorts(service corev1.Service, pod corev1.PodSpec) []Check {
	checks := []Check{}
	for _, port := range service.Spec.Ports {
		if port.TargetPort.Type != intstr.String {
			continue
		}
		matches := 0
		for _, container := range pod.Containers {
			for _, target := range container.Ports {
				if target.Name == port.TargetPort.StrVal && portProtocol(target.Protocol) == portProtocol(port.Protocol) {
					matches++
				}
			}
		}
		state, reason := Consistent, "named target resolves to one final container port of the same protocol"
		if matches != 1 || port.TargetPort.StrVal == "" {
			state = Conflict
			reason = fmt.Sprintf("named target %q resolves to %d matching ports", port.TargetPort.StrVal, matches)
		}
		checks = append(checks, Check{Subject: fmt.Sprintf("service[%s].port[%s]", service.Name, port.Name), State: state,
			Reason: reason})
	}
	return checks
}

func findInitContainer(pod corev1.PodSpec, name string) *corev1.Container {
	for i := range pod.InitContainers {
		if pod.InitContainers[i].Name == name {
			return &pod.InitContainers[i]
		}
	}
	return nil
}

func findMount(container *corev1.Container, mountPath string) *corev1.VolumeMount {
	if container != nil {
		for i := range container.VolumeMounts {
			if container.VolumeMounts[i].MountPath == mountPath {
				return &container.VolumeMounts[i]
			}
		}
	}
	return nil
}

func findPodVolume(pod corev1.PodSpec, name string) *corev1.Volume {
	for i := range pod.Volumes {
		if pod.Volumes[i].Name == name {
			return &pod.Volumes[i]
		}
	}
	return nil
}

// A more specific mount can replace a file without changing its directory's
// original mount. Its contents cannot be inferred from the generated files.
func shadowsFile(container *corev1.Container, directoryPath, relativePath string) bool {
	if container == nil {
		return false
	}
	target := path.Join(directoryPath, relativePath)
	for _, mount := range container.VolumeMounts {
		if strings.HasPrefix(mount.MountPath, directoryPath+"/") &&
			(target == mount.MountPath || strings.HasPrefix(target, mount.MountPath+"/")) {
			return true
		}
	}
	return false
}

func sameContainerExecution(expected, actual *corev1.Container) bool {
	if expected == nil || actual == nil || expected.Image != actual.Image {
		return false
	}
	return processPremise(Process{
		Image: expected.Image, Command: expected.Command, Args: expected.Args, Env: expected.Env,
	}, actual)
}

// A mount includes its backing source. Keeping a mount name while pointing it
// at another ConfigMap does not establish the original consumer's input.
func sameMount(expectedPod, actualPod corev1.PodSpec, expected, actual *corev1.VolumeMount) bool {
	return expected != nil && actual != nil && reflect.DeepEqual(expected, actual) &&
		reflect.DeepEqual(findPodVolume(expectedPod, expected.Name), findPodVolume(actualPod, actual.Name))
}

func checkMaterializer(expected, actual corev1.PodSpec, hasFiles bool) ([]Check, bool) {
	before := findInitContainer(expected, materializerContainerName)
	after := findInitContainer(actual, materializerContainerName)
	if before == nil && !hasFiles {
		for _, container := range actual.InitContainers {
			for _, mount := range container.VolumeMounts {
				if !mount.ReadOnly {
					return []Check{{Subject: "assembly.materialization", State: Unknown,
							Reason: "an unmodeled writable init process may create files absent from the plan"}},
						false
				}
			}
		}
		return nil, true
	}
	check := Check{Subject: "assembly.materialization", State: Unknown,
		Reason: "the modeled file preparation path is missing or changed"}
	if !sameContainerExecution(before, after) {
		return []Check{check}, false
	}
	for i := range before.VolumeMounts {
		mount := &before.VolumeMounts[i]
		if !sameMount(expected, actual, mount, findMount(after, mount.MountPath)) {
			check.Reason = fmt.Sprintf("materializer mount %q or its backing volume changed", mount.MountPath)
			return []Check{check}, false
		}
	}
	if len(before.VolumeMounts) != len(after.VolumeMounts) {
		check.Reason = "materializer acquired unmodeled mounts"
		return []Check{check}, false
	}
	if writer := otherFileWriter(before, actual); writer != "" {
		check.Reason = fmt.Sprintf("container %q has writable access to materialized files", writer)
		return []Check{check}, false
	}
	check.State, check.Reason = Consistent, "materializer execution and required mounts retain their declared structure"
	return []Check{check}, true
}

func otherFileWriter(materializer *corev1.Container, pod corev1.PodSpec) string {
	outputs := map[string]bool{}
	for _, mount := range materializer.VolumeMounts {
		if mount.Name != materializationPlanVolume {
			outputs[mount.Name] = true
		}
	}
	for _, container := range append(slices.Clone(pod.InitContainers), pod.Containers...) {
		if container.Name == materializer.Name {
			continue
		}
		for _, mount := range container.VolumeMounts {
			if outputs[mount.Name] && !mount.ReadOnly {
				return container.Name
			}
		}
	}
	return ""
}

func checkPreparedAccess(
	pod corev1.PodSpec, generated RuntimeDescription, files []File, mainKnown, helperKnown bool,
) []Check {
	checks := []Check{}
	main, helper := findContainer(corev1.PodTemplateSpec{Spec: pod}, generated.Main.Name),
		findInitContainer(pod, materializerContainerName)
	for _, access := range generated.Main.Access {
		if !slices.ContainsFunc(files, func(file File) bool { return file.Directory == access.Directory }) {
			continue
		}
		check := Check{Subject: "assembly.files[" + access.Directory + "]", State: Unknown,
			Reason: "main execution or file preparation is not known"}
		if mainKnown && helperKnown {
			shadowed := slices.ContainsFunc(files, func(file File) bool {
				return file.Directory == access.Directory && shadowsFile(main, access.MountPath, file.Path)
			})
			if shadowed {
				check.Reason = "a more specific mount shadows a generated file"
			} else {
				consumer := findMount(main, access.MountPath)
				writer := findMount(helper, path.Join(materializationRoot, access.Directory))
				check.State, check.Reason = volumeSharing(consumer, writer)
			}
		}
		checks = append(checks, check)
	}
	return checks
}

func volumeSharing(consumer, producer *corev1.VolumeMount) (CheckState, string) {
	if consumer == nil || producer == nil {
		return Conflict, "a required producer or consumer directory mount is missing"
	}
	if consumer.SubPathExpr != "" || producer.SubPathExpr != "" {
		return Unknown, "dynamic subPathExpr prevents proving a shared directory"
	}
	if consumer.Name != producer.Name || consumer.SubPath != producer.SubPath {
		return Conflict, "producer and consumer refer to different volumes or subpaths"
	}
	if producer.ReadOnly {
		return Conflict, "the declared producer directory is mounted read-only"
	}
	return Consistent, "producer and consumer share one mounted volume and subpath"
}

func checkVectorConsumer(
	expected, actual corev1.PodTemplateSpec, generated RuntimeDescription, files []File, mainKnown, helperKnown bool,
) []Check {
	before, after := findContainer(expected, vectorContainerName), findContainer(actual, vectorContainerName)
	if before == nil || generated.Main.Name == vectorContainerName {
		return nil // No platform collector was declared by this assembly.
	}
	check := Check{Subject: "vector.config", State: Unknown, Reason: "collector execution or configuration mount changed"}
	if !helperKnown {
		check.Reason = "file preparation is not known; planned content cannot establish the collector's actual input"
		return []Check{check}
	}
	if !sameContainerExecution(before, after) || !sameMount(expected.Spec, actual.Spec,
		findMount(before, vectorConfigPath), findMount(after, vectorConfigPath)) ||
		shadowsFile(after, vectorConfigPath, vectorConfigFile) {
		return []Check{check}
	}
	file := findFile(files, generated.ConfigDirectory, vectorConfigFile)
	if file == nil {
		check.State, check.Reason = Conflict, "the retained collector command requires its removed configuration file"
		return []Check{check}
	}
	original := findFile(generated.Files, generated.ConfigDirectory, vectorConfigFile)
	if original == nil || !reflect.DeepEqual(original.Content, file.Content) {
		check.Reason = "collector configuration content changed; its sources have not been parsed"
		return []Check{check}
	}
	check.State, check.Reason = Consistent, "collector execution, config mount and generated config content are retained"
	checks := []Check{check}
	for _, output := range generated.LogOutputs {
		checks = append(checks, checkLogSharing(before, after, actual, generated, output, mainKnown))
	}
	return checks
}

func checkLogSharing(
	before, collector *corev1.Container, pod corev1.PodTemplateSpec,
	generated RuntimeDescription, output LogOutput, mainKnown bool,
) Check {
	check := Check{Subject: "vector.source[" + output.Directory + "/" + output.RelativePath + "]", State: Unknown,
		Reason: "producer execution or its original directory mapping is not known"}

	var consumer *corev1.VolumeMount
	for _, mount := range before.VolumeMounts {
		if mount.Name == output.Directory {
			if shadowsFile(collector, mount.MountPath, output.RelativePath) {
				check.Reason = "a more specific collector mount shadows the declared log source"
				return check
			}
			consumer = findMount(collector, mount.MountPath)
			if consumer == nil {
				check.State, check.Reason = Conflict, "retained collector config reads a directory mount that was removed"
				return check
			}
		}
	}
	if !mainKnown {
		return check
	}
	producerPaths := 0
	for _, access := range generated.Main.Access {
		if access.Directory == output.Directory {
			producerPaths++
		}
	}
	if producerPaths != 1 {
		check.Reason = "multiple producer access paths prevent locating the native log writer"
		return check
	}
	for _, access := range generated.Main.Access {
		if access.Directory == output.Directory {
			producerContainer := findContainer(pod, output.Container)
			if shadowsFile(producerContainer, access.MountPath, output.RelativePath) {
				check.Reason = "a more specific producer mount shadows the declared log source"
				return check
			}
			producer := findMount(producerContainer, access.MountPath)
			check.State, check.Reason = volumeSharing(consumer, producer)
			return check
		}
	}
	return check
}
