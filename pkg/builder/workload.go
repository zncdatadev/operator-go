package builder

import (
	"github.com/zncdatadev/operator-go/pkg/util"
	corev1 "k8s.io/api/core/v1"
)

var (
	_ ResourceBuilder                       = &BaseWorkloadBuilder{}
	_ WorkloadContainers                    = &BaseWorkloadBuilder{}
	_ WorkloadInitContainers                = &BaseWorkloadBuilder{}
	_ WorkloadVolumes                       = &BaseWorkloadBuilder{}
	_ WorkloadAffinity                      = &BaseWorkloadBuilder{}
	_ WorkloadTerminationGracePeriodSeconds = &BaseWorkloadBuilder{}
)

type WorkloadOptions struct {
	Name        string
	Labels      map[string]string
	Annotations map[string]string
	Affinity    *corev1.Affinity

	Image            *util.Image
	Ports            []corev1.ContainerPort
	CommandOverrides []string
	EnvOverrides     map[string]string
	PodOverrides     *corev1.PodTemplateSpec
}

type BaseWorkloadBuilder struct {
	BaseResourceBuilder

	Affinity *corev1.Affinity

	Image *util.Image
	Ports []corev1.ContainerPort

	CommandOverrides []string
	EnvOverrides     map[string]string
	PodOverrides     *corev1.PodTemplateSpec

	terminationGracePeriodSeconds *int64

	containers     []corev1.Container // do not init this field when constructing the struct
	initContainers []corev1.Container // do not init this field when constructing the struct
	volumes        []corev1.Volume    // do not init this field when constructing the struct
}

func NewBaseWorkloadBuilder(options *WorkloadOptions) *BaseWorkloadBuilder {
	return &BaseWorkloadBuilder{
		BaseResourceBuilder: BaseResourceBuilder{
			name:        options.Name,
			labels:      options.Labels,
			annotations: options.Annotations,
		},

		Affinity: options.Affinity,

		Image:            options.Image,
		Ports:            options.Ports,
		CommandOverrides: options.CommandOverrides,
		EnvOverrides:     options.EnvOverrides,
		PodOverrides:     options.PodOverrides,
	}
}

func (b *BaseWorkloadBuilder) AddContainers(containers []corev1.Container) {
	b.containers = append(b.containers, containers...)
}

func (b *BaseWorkloadBuilder) AddContainer(container *corev1.Container) {
	b.containers = append(b.containers, *container)
}

func (b *BaseWorkloadBuilder) ResetContainers(containers []corev1.Container) {
	b.containers = containers
}

func (b *BaseWorkloadBuilder) GetContainers() []corev1.Container {
	return b.containers
}

func (b *BaseWorkloadBuilder) AddInitContainers(containers []corev1.Container) {
	b.initContainers = append(b.initContainers, containers...)
}

func (b *BaseWorkloadBuilder) AddInitContainer(container *corev1.Container) {
	b.initContainers = append(b.initContainers, *container)
}

func (b *BaseWorkloadBuilder) ResetInitContainers(containers []corev1.Container) {
	b.initContainers = containers
}

func (b *BaseWorkloadBuilder) GetInitContainers() []corev1.Container {
	return b.initContainers
}

func (b *BaseWorkloadBuilder) AddVolumes(volumes []corev1.Volume) {
	b.volumes = append(b.volumes, volumes...)
}

func (b *BaseWorkloadBuilder) AddVolume(volume *corev1.Volume) {
	b.volumes = append(b.volumes, *volume)
}

func (b *BaseWorkloadBuilder) ResetVolumes(volumes []corev1.Volume) {
	b.volumes = volumes
}

func (b *BaseWorkloadBuilder) GetVolumes() []corev1.Volume {
	return b.volumes
}

func (b *BaseWorkloadBuilder) AddAffinity(affinity *corev1.Affinity) {
	b.Affinity = affinity
}

func (b *BaseWorkloadBuilder) GetAffinity() *corev1.Affinity {
	return b.Affinity
}

func (b *BaseWorkloadBuilder) AddTerminationGracePeriodSeconds(seconds *int64) {
	b.terminationGracePeriodSeconds = seconds
}

func (b *BaseWorkloadBuilder) GetTerminationGracePeriodSeconds() *int64 {
	return b.terminationGracePeriodSeconds
}

var _ WorkloadReplicas = &BaseWorkloadReplicasBuilder{}

type WorkloadReplicasOptions struct {
	Replicas *int32
	WorkloadOptions
}

type BaseWorkloadReplicasBuilder struct {
	BaseWorkloadBuilder
	replicas *int32
}

func NewBaseWorkloadReplicasBuilder(options *WorkloadReplicasOptions) *BaseWorkloadReplicasBuilder {
	return &BaseWorkloadReplicasBuilder{
		BaseWorkloadBuilder: *NewBaseWorkloadBuilder(&options.WorkloadOptions),
		replicas:            options.Replicas,
	}
}

func (b *BaseWorkloadReplicasBuilder) SetReplicas(replicas *int32) {
	b.replicas = replicas
}

func (b *BaseWorkloadReplicasBuilder) GetReplicas() *int32 {
	return b.replicas
}
