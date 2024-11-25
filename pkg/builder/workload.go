/*
Copyright 2024 ZNCDataDev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package builder

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	_ WorkloadBuilder = &BaseWorkloadBuilder{}

	_ WorkloadReplicas = &BaseWorkloadReplicasBuilder{}
)

var ErrNoContainers = errors.New("no containers defined")

// WorkloadOptions is a struct to hold the options for a workload
//
// Note: The values of envOverrides and cliOverrides will
// only be overridden on the container with the same name as roleGroupInfo.RoleName,
// if roleGroupInfo exists and roleGroupInfo.RoleName has a value.
type BaseWorkloadBuilder struct {
	ObjectMeta

	Image *util.Image

	Overrides *commonsv1alpha1.OverridesSpec

	RoleGroupConfig *commonsv1alpha1.RoleGroupConfigSpec

	containers      map[string]corev1.Container
	initContainers  map[string]corev1.Container
	volumes         map[string]corev1.Volume
	securityContext *corev1.PodSecurityContext // do not init this field when constructing the struct
	// Parse runtime.RawExtension from RoleGroupSpec.Affinity, if exists
	affinity  *corev1.Affinity               // do not init this field when constructing the struct
	resources *commonsv1alpha1.ResourcesSpec // do not init this field when constructing the struct
	// Parse runtime.RawExtension from OverridesSpec.PodOverrides, if exists
	terminationGracePeriod *time.Duration // do not init this field when constructing the struct
}

func NewBaseWorkloadBuilder(
	client *client.Client,
	// the name is the resource name when creating
	name string,
	image *util.Image,
	overrides *commonsv1alpha1.OverridesSpec,
	roleGroupConfig *commonsv1alpha1.RoleGroupConfigSpec,
	options ...Option,
) *BaseWorkloadBuilder {

	opts := &Options{}
	for _, opt := range options {
		opt(opts)
	}

	return &BaseWorkloadBuilder{
		ObjectMeta:      *NewObjectMeta(client, name, options...),
		Image:           image,
		Overrides:       overrides,
		RoleGroupConfig: roleGroupConfig,

		containers:     map[string]corev1.Container{},
		initContainers: map[string]corev1.Container{},
		volumes:        map[string]corev1.Volume{},
	}
}

func (b *BaseWorkloadBuilder) SetImage(image *util.Image) {
	b.Image = image
}

func (b *BaseWorkloadBuilder) GetImage() *util.Image {
	return b.Image
}

func (b *BaseWorkloadBuilder) GetImageWithTag() (string, error) {
	return b.Image.GetImageWithTag()
}

func (b *BaseWorkloadBuilder) AddContainers(containers []corev1.Container) {
	for i := range containers {
		container := containers[i]
		if _, ok := b.containers[container.Name]; ok {
			logger.V(5).Info(
				"Replacing container with the same name",
				"cluster", b.ClusterName, "role", b.RoleName, "roleGroup", b.RoleGroupName, "container", container.Name,
			)
		}

		b.containers[container.Name] = container
	}
}

func (b *BaseWorkloadBuilder) AddContainer(container *corev1.Container) {
	if _, ok := b.containers[container.Name]; ok {
		logger.V(5).Info(
			"Replacing container with the same name",
			"cluster", b.ClusterName, "role", b.RoleName, "roleGroup", b.RoleGroupName, "container", container.Name,
		)
	}

	b.containers[container.Name] = *container
}

func (b *BaseWorkloadBuilder) ResetContainers(containers []corev1.Container) {
	b.containers = map[string]corev1.Container{}
}

func (b *BaseWorkloadBuilder) GetContainers() []corev1.Container {
	if b.containers == nil {
		b.containers = map[string]corev1.Container{}
	}
	containers := make([]corev1.Container, 0, len(b.containers))

	for _, container := range b.containers {
		containers = append(containers, container)
	}
	slices.SortFunc(containers, func(i, j corev1.Container) int {
		if i.Name < j.Name {
			return -1
		}
		if i.Name > j.Name {
			return 1
		}
		return 0
	})
	return containers
}

func (b *BaseWorkloadBuilder) GetContainer(name string) *corev1.Container {
	if b.containers == nil {
		return nil
	}
	container, ok := b.containers[name]
	if !ok {
		return nil
	}
	return &container
}

func (b *BaseWorkloadBuilder) AddInitContainers(containers []corev1.Container) {
	for i := range containers {
		container := containers[i]
		if _, ok := b.containers[container.Name]; ok {
			logger.V(5).Info(
				"Replacing init container with the same name",
				"cluster", b.ClusterName, "role", b.RoleName, "roleGroup", b.RoleGroupName, "container", container.Name,
			)
		}
		b.initContainers[container.Name] = container
	}
}

func (b *BaseWorkloadBuilder) AddInitContainer(container *corev1.Container) {
	if _, ok := b.containers[container.Name]; ok {
		logger.V(5).Info(
			"Replacing init container with the same name",
			"cluster", b.ClusterName, "role", b.RoleName, "roleGroup", b.RoleGroupName, "container", container.Name,
		)
	}
	b.initContainers[container.Name] = *container
}

func (b *BaseWorkloadBuilder) ResetInitContainers(containers []corev1.Container) {
	b.initContainers = map[string]corev1.Container{}
	for i := range containers {
		container := containers[i]
		b.initContainers[container.Name] = container
	}
}

func (b *BaseWorkloadBuilder) GetInitContainers() []corev1.Container {
	containers := make([]corev1.Container, 0, len(b.initContainers))

	for _, container := range b.initContainers {
		containers = append(containers, container)
	}

	slices.SortFunc(containers, func(i, j corev1.Container) int {
		if i.Name < j.Name {
			return -1
		}
		if i.Name > j.Name {
			return 1
		}
		return 0
	})
	return containers
}

func (b *BaseWorkloadBuilder) GetInitContainer(name string) *corev1.Container {
	container, ok := b.containers[name]
	if !ok {
		return nil
	}
	return &container
}

func (b *BaseWorkloadBuilder) SetResources(resources *commonsv1alpha1.ResourcesSpec) {
	b.resources = resources
}

func (b *BaseWorkloadBuilder) GetResources() *commonsv1alpha1.ResourcesSpec {
	if b.RoleGroupConfig != nil && b.RoleGroupConfig.Resources != nil {
		b.resources = b.RoleGroupConfig.Resources
	}
	return b.resources
}

func (b *BaseWorkloadBuilder) SetSecurityContext(user int64, group int64, nonRoot bool) {
	securityContext := &corev1.PodSecurityContext{
		RunAsUser:    &user,
		RunAsGroup:   &group,
		RunAsNonRoot: &nonRoot,
	}

	b.securityContext = securityContext
}

func (b *BaseWorkloadBuilder) GetSecurityContext() *corev1.PodSecurityContext {
	return b.securityContext
}

func (b *BaseWorkloadBuilder) OverrideContainer() {
	if b.Overrides == nil {
		return
	}

	if b.RoleName == "" {
		logger.V(10).Info("Skipping override container as RoleName is not set", "cluster", b.ClusterName, "role", b.RoleName, "roleGroup", b.RoleGroupName)
		return
	}

	mainContainer := b.GetContainer(b.RoleName)
	if mainContainer == nil {
		logger.V(10).Info(
			"Skipping override container with RoleName is not found",
			"cluster", b.ClusterName, "role", b.RoleName, "roleGroup", b.RoleGroupName,
		)
		return
	}

	b.overridedEnv(mainContainer)
	b.overridedCli(mainContainer)

	b.containers[mainContainer.Name] = *mainContainer
}

func (b *BaseWorkloadBuilder) overridedEnv(container *corev1.Container) {
	// Override the env
	for key, value := range b.Overrides.EnvOverrides {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  key,
			Value: value,
		})
	}
	logger.V(5).Info("Env override", "container", container.Name, "env", container.Env)
}

func (b *BaseWorkloadBuilder) overridedCli(container *corev1.Container) {
	container.Command = b.Overrides.CliOverrides
	container.Args = []string{}
	logger.V(5).Info("Command override", "container", container.Name, "command", container.Command)
}

func (b *BaseWorkloadBuilder) AddVolumes(volumes []corev1.Volume) {
	for i := range volumes {
		volume := volumes[i]
		if _, ok := b.volumes[volume.Name]; ok {
			logger.V(5).Info(
				"Replacing volume with the same name",
				"cluster", b.ClusterName, "role", b.RoleName, "roleGroup", b.RoleGroupName, "volume", volume.Name,
			)
		}
		b.volumes[volume.Name] = volume
	}
}

func (b *BaseWorkloadBuilder) AddVolume(volume *corev1.Volume) {
	if _, ok := b.volumes[volume.Name]; ok {
		logger.V(5).Info(
			"Replacing volume with the same name",
			"cluster", b.ClusterName, "role", b.RoleName, "roleGroup", b.RoleGroupName, "volume", volume.Name,
		)
	}
	b.volumes[volume.Name] = *volume
}

func (b *BaseWorkloadBuilder) ResetVolumes(volumes []corev1.Volume) {
	b.volumes = map[string]corev1.Volume{}
	for i := range volumes {
		volume := volumes[i]
		b.volumes[volume.Name] = volume
	}
}

func (b *BaseWorkloadBuilder) GetVolumes() []corev1.Volume {
	if b.volumes == nil {
		b.volumes = map[string]corev1.Volume{}
	}

	volumes := make([]corev1.Volume, 0, len(b.volumes))
	for _, volume := range b.volumes {
		volumes = append(volumes, volume)
	}

	slices.SortFunc(volumes, func(i, j corev1.Volume) int {
		if i.Name < j.Name {
			return -1
		}
		if i.Name > j.Name {
			return 1
		}
		return 0
	})

	return volumes
}

func (b *BaseWorkloadBuilder) SetAffinity(affinity *corev1.Affinity) {
	b.affinity = affinity
}

func (b *BaseWorkloadBuilder) GetAffinity() (*corev1.Affinity, error) {
	if b.affinity == nil && b.RoleGroupConfig != nil && b.RoleGroupConfig.Affinity != nil {
		affinity, err := convertRawExtension[corev1.Affinity](b.RoleGroupConfig.Affinity)
		if err != nil {
			return nil, err
		}
		b.affinity = affinity
	}

	return b.affinity, nil

}

func (b *BaseWorkloadBuilder) GetTerminationGracePeriod() (*time.Duration, error) {
	if b.terminationGracePeriod == nil {
		if b.RoleGroupConfig != nil && b.RoleGroupConfig.GracefulShutdownTimeout != "" {
			timeout := b.RoleGroupConfig.GracefulShutdownTimeout
			t, err := time.ParseDuration(timeout)
			if err != nil {
				return nil, err
			}
			b.terminationGracePeriod = &t
		}
	}
	return b.terminationGracePeriod, nil
}

func (b *BaseWorkloadBuilder) GetTerminationGracePeriodSeconds() (*int64, error) {
	terminationGracePeriod, err := b.GetTerminationGracePeriod()
	if err != nil {
		return nil, err
	}
	if terminationGracePeriod != nil {
		seconds := int64(terminationGracePeriod.Seconds())
		return &seconds, nil
	}
	return nil, nil
}

func (b *BaseWorkloadBuilder) GetImagePullSecrets() []corev1.LocalObjectReference {
	if b.Image.PullSecretName != "" {
		return []corev1.LocalObjectReference{{Name: b.Image.PullSecretName}}
	}
	return nil
}

// getPodTemplate returns the pod template for the workload without any overrides,
// you should use GetPodTemplate to get the pod template with overrides applied.
// getPodTemplate build a basic pod template with basic properties.
// Then, merge the pod template with the overrides pod template.
func (b *BaseWorkloadBuilder) getPodTemplate() (*corev1.PodTemplateSpec, error) {
	affinity, err := b.GetAffinity()
	if err != nil {
		return nil, err
	}
	terminationGracePeriodSeconds, err := b.GetTerminationGracePeriodSeconds()
	if err != nil {
		return nil, err
	}

	b.setContainerResources()

	pod := &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      b.GetLabels(),
			Annotations: b.GetAnnotations(),
		},
		Spec: corev1.PodSpec{
			InitContainers:                b.GetInitContainers(),
			Containers:                    b.GetContainers(),
			Volumes:                       b.GetVolumes(),
			Affinity:                      affinity,
			TerminationGracePeriodSeconds: terminationGracePeriodSeconds,
			ImagePullSecrets:              b.GetImagePullSecrets(),
			SecurityContext:               b.GetSecurityContext(),
		},
	}
	return pod, nil
}

func (b *BaseWorkloadBuilder) getOverridesPodTemplate() (*corev1.PodTemplateSpec, error) {
	if b.Overrides.PodOverrides != nil {
		return convertRawExtension[corev1.PodTemplateSpec](b.Overrides.PodOverrides)
	}

	return nil, nil
}

func (b *BaseWorkloadBuilder) setContainerResources() {
	resources := b.GetResources()
	if resources == nil {
		logger.V(10).Info(
			"Skipping setting container resources as resources not found",
			"cluster", b.ClusterName, "role", b.RoleName, "roleGroup", b.RoleGroupName,
		)
		return
	}

	mainContainer := b.GetContainer(b.RoleName)

	if mainContainer == nil {
		logger.V(10).Info(
			"Skipping setting container resources as container not found with RoleName",
			"cluster", b.ClusterName, "role", b.RoleName, "roleGroup", b.RoleGroupName,
		)
		return
	}

	if resources.CPU != nil {
		logKWargs := []any{
			"cluster", b.ClusterName, "role", b.RoleName, "roleGroup", b.RoleGroupName,
		}
		if !resources.CPU.Min.IsZero() {
			if mainContainer.Resources.Requests == nil {
				mainContainer.Resources.Requests = corev1.ResourceList{}
			}
			mainContainer.Resources.Requests[corev1.ResourceCPU] = resources.CPU.Min
			logKWargs = append(logKWargs, "min", resources.CPU.Min)
		}

		if !resources.CPU.Max.IsZero() {
			if mainContainer.Resources.Limits == nil {
				mainContainer.Resources.Limits = corev1.ResourceList{}
			}
			mainContainer.Resources.Limits[corev1.ResourceCPU] = resources.CPU.Max
			logKWargs = append(logKWargs, "max", resources.CPU.Max)
		}

		logger.V(5).Info("Setting container CPU resources with RoleName", logKWargs...)
	}

	if resources.Memory != nil && !resources.Memory.Limit.IsZero() {
		if mainContainer.Resources.Limits == nil {
			mainContainer.Resources.Limits = corev1.ResourceList{}
		}

		mainContainer.Resources.Limits[corev1.ResourceMemory] = resources.Memory.Limit
		logger.V(5).Info(
			"Setting container memory resources with RoleName",
			"cluster", b.ClusterName, "role", b.RoleName, "roleGroup", b.RoleGroupName,
			"limit", resources.Memory.Limit,
		)
	}

	b.containers[mainContainer.Name] = *mainContainer
}

// GetPodTemplate returns the pod template for the workload with overrides applied.
// All the overrides are applied to the pod template.
func (b *BaseWorkloadBuilder) GetPodTemplate() (*corev1.PodTemplateSpec, error) {
	b.OverrideContainer()
	pod, err := b.getPodTemplate()
	if err != nil {
		return nil, err
	}

	if b.Overrides == nil {
		return pod, nil
	}

	overridesPod, err := b.getOverridesPodTemplate()
	if err != nil {
		return nil, err
	}

	podTemplate, err := util.MergeObjectWithStrategic(pod, overridesPod)

	if err != nil {
		return nil, err
	}

	return podTemplate, nil
}

func (b *BaseWorkloadBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	panic("implement me")
}

var _ WorkloadReplicas = &BaseWorkloadReplicasBuilder{}

type BaseWorkloadReplicasBuilder struct {
	BaseWorkloadBuilder
	replicas *int32
}

func NewBaseWorkloadReplicasBuilder(
	client *client.Client,
	name string,
	replicas *int32,
	image *util.Image,
	overrides *commonsv1alpha1.OverridesSpec,
	roleGroupConfig *commonsv1alpha1.RoleGroupConfigSpec,
	options ...Option,
) *BaseWorkloadReplicasBuilder {
	return &BaseWorkloadReplicasBuilder{
		BaseWorkloadBuilder: *NewBaseWorkloadBuilder(
			client,
			name,
			image,
			overrides,
			roleGroupConfig,
			options...,
		),
		replicas: replicas,
	}
}

func (b *BaseWorkloadReplicasBuilder) SetReplicas(replicas *int32) {
	b.replicas = replicas
}

func (b *BaseWorkloadReplicasBuilder) GetReplicas() *int32 {
	if b.replicas == nil {
		replicas := int32(1)
		logger.Info("Replicas not set, defaulting to 1")
		return &replicas
	}
	return b.replicas
}

func convertRawExtension[T any](raw *runtime.RawExtension) (*T, error) {
	var obj T
	if raw == nil || raw.Raw == nil {
		return &obj, nil
	}

	if err := json.Unmarshal(raw.Raw, &obj); err != nil {
		return &obj, err
	}
	return &obj, nil
}
