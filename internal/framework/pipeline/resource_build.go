package pipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path"
	"strings"
	"time"

	"github.com/zncdatadev/operator-go/pkg/framework"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/util/validation"
	kubernetesjson "sigs.k8s.io/json"
)

const (
	labelInstance                = "app.kubernetes.io/instance"
	labelComponent               = "app.kubernetes.io/component"
	kindConfigMap                = "ConfigMap"
	kindService                  = "Service"
	kindStatefulSet              = "StatefulSet"
	materializerPodNameEnv       = "POD_NAME"
	materializerPodNameFieldPath = "metadata.name"
)

func buildGroup(identity GroupIdentity, common CommonConfig, image ResolvedImage, runtime RuntimeDescription,
	source GroupSource, options AssemblyOptions, final func(FinalView) []Check,
) (*GroupResources, []File, []Check, error) {
	composed, vector, err := composeVector(runtime, common.Logging.EnableVectorAgent, options)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := ValidateRuntime(composed); err != nil {
		return nil, nil, nil, err
	}
	fileLayers := make([]map[string]FileOverride, 0, 2)
	for _, layer := range []*Overrides{source.RoleOverrides, source.Overrides} {
		if layer != nil {
			fileLayers = append(fileLayers, layer.ConfigOverrides)
		}
	}
	files := cloneDeclaredFiles(composed.Files)
	if composed.ConfigDirectory != "" || hasFileOverrides(fileLayers) {
		files, err = ApplyFileOverrides(composed.Files, composed.ConfigDirectory, fileLayers...)
	}
	if err != nil {
		return nil, nil, nil, err
	}
	process := CloneRuntime(composed).Main
	if err := applyProcessOverrides(&process, source.RoleOverrides, source.Overrides); err != nil {
		return nil, files, nil, err
	}
	expected, err := assembleGroup(identity, common, image, composed, process, vector, files, options)
	if err != nil {
		return nil, files, nil, err
	}
	actual := cloneGroupResources(expected)
	for index, layer := range []*Overrides{source.RoleOverrides, source.Overrides} {
		if layer == nil || len(layer.PodOverrides) == 0 {
			continue
		}
		if err := patchPod(&actual.StatefulSet.Spec.Template, layer.PodOverrides); err != nil {
			return nil, files, nil, fmt.Errorf("podOverrides layer %d: %w", index, err)
		}
	}
	checks := CheckAssembly(expected, actual, composed, files)
	if final != nil {
		view := FinalView{Generated: CloneRuntime(composed), Files: cloneDeclaredFiles(files),
			FilePreparationKnown: filePreparationKnown(expected, actual, files),
			LogCollectionKnown:   collectorKnown(expected, actual, composed, files),
			Pod:                  *actual.StatefulSet.Spec.Template.DeepCopy(), Services: []corev1.Service{
				*actual.Service.DeepCopy(), *actual.HeadlessService.DeepCopy()}}
		checks = append(checks, final(view)...)
	}
	conflicts := []error{}
	for _, check := range checks {
		if check.State == Conflict {
			conflicts = append(conflicts, fmt.Errorf("%s: %s", check.Subject, check.Reason))
		}
	}
	if err := errors.Join(conflicts...); err != nil {
		return nil, files, checks, err
	}
	return &actual, files, checks, nil
}

func hasFileOverrides(layers []map[string]FileOverride) bool {
	for _, layer := range layers {
		if len(layer) > 0 {
			return true
		}
	}
	return false
}

func assembleGroup(identity GroupIdentity, common CommonConfig, image ResolvedImage, runtime RuntimeDescription,
	process Process, vector *corev1.Container, files []File, options AssemblyOptions,
) (GroupResources, error) {
	var out GroupResources
	name := identity.ServiceName()
	headless := name + "-headless"
	if len(validation.IsDNS1035Label(headless)) != 0 {
		return out, fmt.Errorf("headless Service name %q is too long", headless)
	}
	selector := map[string]string{labelInstance: identity.ClusterIdentity.Name,
		labelComponent: identity.Role, "role-group": identity.Name}
	labels := resourceLabels(identity.ClusterIdentity, selector)
	metadata := func(value string) metav1.ObjectMeta {
		return metav1.ObjectMeta{Name: value, Namespace: identity.Namespace,
			Labels: CloneInput(labels)}
	}
	plan, err := PrepareMaterialization(files)
	if err != nil {
		return out, err
	}
	encoded, err := EncodeMaterializationPlan(plan)
	if err != nil {
		return out, err
	}
	if len(encoded) > 1024*1024 {
		return out, fmt.Errorf("materialization plan exceeds ConfigMap data capacity")
	}
	out.ConfigMap = corev1.ConfigMap{ObjectMeta: metadata(name),
		Data: map[string]string{materializationPlanFile: string(encoded)}}
	main := corev1.Container{Name: process.Name, Image: process.Image, Command: process.Command, Args: process.Args,
		Env: process.Env, SecurityContext: process.Identity, ImagePullPolicy: image.PullPolicy,
		Lifecycle: process.Lifecycle.DeepCopy(), StartupProbe: process.StartupProbe.DeepCopy(),
		ReadinessProbe: process.ReadinessProbe.DeepCopy(), LivenessProbe: process.LivenessProbe.DeepCopy(),
		Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
			corev1.ResourceCPU:    common.Resources.CPU.Min.DeepCopy(),
			corev1.ResourceMemory: common.Resources.Memory.Limit.DeepCopy()},
			Limits: corev1.ResourceList{corev1.ResourceCPU: common.Resources.CPU.Max.DeepCopy(),
				corev1.ResourceMemory: common.Resources.Memory.Limit.DeepCopy()}}}
	for _, access := range process.Access {
		main.VolumeMounts = append(main.VolumeMounts, corev1.VolumeMount{
			Name: access.Directory, MountPath: access.MountPath, ReadOnly: access.ReadOnly})
	}
	pod := corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: CloneInput(labels)}, Spec: corev1.PodSpec{
		Containers:      []corev1.Container{main},
		SecurityContext: &corev1.PodSecurityContext{FSGroup: CloneInput(runtime.SharedGroup)},
		Affinity:        common.Affinity.DeepCopy()}}
	if image.PullSecretName != "" {
		pod.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: image.PullSecretName}}
	}
	graceSeconds := int64(common.GracefulShutdownTimeout.Duration / time.Second)
	pod.Spec.TerminationGracePeriodSeconds = &graceSeconds
	for _, directory := range runtime.Directories {
		if directory.Name == materializationPlanVolume {
			return out, fmt.Errorf("directory %q conflicts with plan volume", directory.Name)
		}
		if volume, ok := platformVolume(directory); ok {
			pod.Spec.Volumes = append(pod.Spec.Volumes, volume)
			continue
		}
		if directory.Data && common.Resources.Storage.Type == framework.StoragePersistent {
			out.RetainedData = &RetainedDataSlot{Name: directory.Name, RetainedData: RetainedData{
				StorageClassName: common.Resources.Storage.StorageClassName,
				Capacity:         common.Resources.Storage.Capacity.DeepCopy()}}
			continue
		}
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{Name: directory.Name,
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}})
	}
	if err := validateStorage(common.Resources.Storage); err != nil {
		return out, err
	}
	if common.Resources.Storage.Type == framework.StoragePersistent && out.RetainedData == nil {
		return out, fmt.Errorf("config.resources.storage: persistent storage requires a declared Data directory")
	}
	if len(files) > 0 {
		if strings.TrimSpace(options.MaterializerImage) == "" {
			return out, fmt.Errorf("materializer image is required")
		}
		if process.Name == materializerContainerName {
			return out, fmt.Errorf("main container conflicts with materializer name")
		}
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{Name: materializationPlanVolume,
			VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: name}}}})
		init := corev1.Container{Name: materializerContainerName, Image: options.MaterializerImage,
			Command:         []string{"/materialize"},
			Args:            []string{"--plan=" + materializationPlanPath, "--root=" + materializationRoot},
			SecurityContext: options.HelperIdentity.DeepCopy(), Env: []corev1.EnvVar{{Name: materializerPodNameEnv,
				ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: materializerPodNameFieldPath}}}},
			VolumeMounts: []corev1.VolumeMount{{Name: materializationPlanVolume, MountPath: "/plan", ReadOnly: true}}}
		directories := map[string]bool{}
		for _, file := range files {
			directories[file.Directory] = true
		}
		for _, directory := range sortedKeys(directories) {
			init.VolumeMounts = append(init.VolumeMounts, corev1.VolumeMount{
				Name: directory, MountPath: path.Join(materializationRoot, directory)})
		}
		pod.Spec.InitContainers = []corev1.Container{init}
	}
	for _, initializer := range runtime.Initializers {
		container := corev1.Container{Name: initializer.Name, Image: initializer.Image,
			Command: initializer.Command, Args: initializer.Args, Env: initializer.Env,
			SecurityContext: initializer.Identity.DeepCopy(), ImagePullPolicy: image.PullPolicy,
			Resources: *main.Resources.DeepCopy()}
		for _, access := range initializer.Access {
			container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
				Name: access.Directory, MountPath: access.MountPath, ReadOnly: access.ReadOnly})
		}
		pod.Spec.InitContainers = append(pod.Spec.InitContainers, container)
	}
	if vector != nil {
		pod.Spec.Containers = append(pod.Spec.Containers, *vector.DeepCopy())
	}
	out.Service = corev1.Service{ObjectMeta: metadata(name), Spec: corev1.ServiceSpec{Selector: CloneInput(selector)}}
	for _, endpoint := range runtime.Endpoints {
		pod.Spec.Containers[0].Ports = append(pod.Spec.Containers[0].Ports, corev1.ContainerPort{
			Name: endpoint.Name, ContainerPort: endpoint.Port, Protocol: corev1.ProtocolTCP})
		out.Service.Spec.Ports = append(out.Service.Spec.Ports, corev1.ServicePort{
			Name: endpoint.Name, Port: endpoint.Port, TargetPort: intstr.FromString(endpoint.Name),
			Protocol: corev1.ProtocolTCP})
	}
	out.HeadlessService = *out.Service.DeepCopy()
	out.HeadlessService.ObjectMeta = metadata(headless)
	out.HeadlessService.Spec.ClusterIP = corev1.ClusterIPNone
	out.StatefulSet = appsv1.StatefulSet{ObjectMeta: metadata(name), Spec: appsv1.StatefulSetSpec{
		Replicas: &identity.Replicas, ServiceName: headless,
		Selector: &metav1.LabelSelector{MatchLabels: CloneInput(selector)}, Template: pod}}
	out.Coordination = CloneInput(runtime.Coordination)
	if out.Coordination != nil {
		out.StatefulSet.Spec.PodManagementPolicy = appsv1.OrderedReadyPodManagement
		out.StatefulSet.Spec.UpdateStrategy.Type = appsv1.RollingUpdateStatefulSetStrategyType
	}
	if out.RetainedData != nil {
		out.StatefulSet.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{retainedClaimTemplate(*out.RetainedData)}
		out.StatefulSet.Spec.PersistentVolumeClaimRetentionPolicy = &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{
			WhenDeleted: appsv1.RetainPersistentVolumeClaimRetentionPolicyType,
			WhenScaled:  appsv1.RetainPersistentVolumeClaimRetentionPolicyType,
		}
	}
	return out, nil
}

// resourceLabels propagates CR metadata under resource-specific declarations.
// Selector identity is supplied by the group assembler, not copied from the CR;
// ordinary label changes must never alter an immutable StatefulSet selector.
func resourceLabels(cluster ClusterIdentity, declared map[string]string) map[string]string {
	labels := make(map[string]string, len(cluster.Labels)+len(declared)+1)
	maps.Copy(labels, cluster.Labels)
	maps.Copy(labels, declared)
	labels[labelInstance] = cluster.Name
	return labels
}

func cloneGroupResources(in GroupResources) GroupResources {
	out := in
	out.ConfigMap = *in.ConfigMap.DeepCopy()
	out.StatefulSet = *in.StatefulSet.DeepCopy()
	out.Service = *in.Service.DeepCopy()
	out.HeadlessService = *in.HeadlessService.DeepCopy()
	out.RetainedData = CloneInput(in.RetainedData)
	out.Coordination = CloneInput(in.Coordination)
	return out
}

func patchPod(pod *corev1.PodTemplateSpec, patch json.RawMessage) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(patch, &object); err != nil || object == nil {
		return fmt.Errorf("patch must be an object")
	}
	before, err := json.Marshal(pod)
	if err != nil {
		return err
	}
	after, err := strategicpatch.StrategicMergePatch(before, patch, corev1.PodTemplateSpec{})
	if err != nil {
		return err
	}
	var next corev1.PodTemplateSpec
	strict, err := kubernetesjson.UnmarshalStrict(after, &next)
	if err != nil {
		return err
	}
	if err := errors.Join(strict...); err != nil {
		return fmt.Errorf("unsupported PodTemplate fields: %w", err)
	}
	*pod = next
	return nil
}

func checkResourceInventory[C, S, F any](plan ResourcePlan[C, S, F]) error {
	seen := map[string]bool{}
	add := func(kind, namespace, name string) error {
		key := kind + "/" + namespace + "/" + name
		if seen[key] {
			return fmt.Errorf("duplicate resource producer %s", key)
		}
		seen[key] = true
		return nil
	}
	for _, group := range plan.Groups {
		if group.Resources != nil {
			r := group.Resources
			for _, item := range []struct {
				kind   string
				object metav1.Object
			}{
				{kindConfigMap, &r.ConfigMap}, {kindService, &r.Service}, {kindService, &r.HeadlessService},
				{kindStatefulSet, &r.StatefulSet},
			} {
				if err := add(item.kind, item.object.GetNamespace(), item.object.GetName()); err != nil {
					return err
				}
			}
		}
	}
	for _, cm := range plan.ClusterOutput.ConfigMaps {
		if err := add(kindConfigMap, cm.Namespace, cm.Name); err != nil {
			return err
		}
	}
	return nil
}

func filePreparationKnown(expected, actual GroupResources, files []File) bool {
	_, known := checkMaterializer(expected.StatefulSet.Spec.Template.Spec, actual.StatefulSet.Spec.Template.Spec,
		len(files) > 0)
	return known
}
