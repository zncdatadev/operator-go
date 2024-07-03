package builder

import (
	"errors"

	"github.com/zncdatadev/operator-go/pkg/client"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ ResourceBuilder                       = &BaseWorkloadBuilder{}
	_ WorkloadContainers                    = &BaseWorkloadBuilder{}
	_ WorkloadInitContainers                = &BaseWorkloadBuilder{}
	_ WorkloadVolumes                       = &BaseWorkloadBuilder{}
	_ WorkloadAffinity                      = &BaseWorkloadBuilder{}
	_ WorkloadTerminationGracePeriodSeconds = &BaseWorkloadBuilder{}
)

var ErrNoContainers = errors.New("no containers defined")

type BaseWorkloadBuilder struct {
	BaseResourceBuilder

	affinity *corev1.Affinity

	podOverrides *corev1.PodTemplateSpec

	terminationGracePeriodSeconds *int64

	containers     []corev1.Container // do not init this field when constructing the struct
	initContainers []corev1.Container // do not init this field when constructing the struct
	volumes        []corev1.Volume    // do not init this field when constructing the struct
}

func NewBaseWorkloadBuilder(
	client *client.Client,
	name string,
	labels map[string]string,
	annotations map[string]string,
	affinity *corev1.Affinity,
	podOverrides *corev1.PodTemplateSpec,
	terminationGracePeriodSeconds *int64,
) *BaseWorkloadBuilder {
	return &BaseWorkloadBuilder{
		BaseResourceBuilder: BaseResourceBuilder{
			Client:      client,
			name:        name,
			labels:      labels,
			annotations: annotations,
		},

		affinity: affinity,

		podOverrides: podOverrides,

		terminationGracePeriodSeconds: terminationGracePeriodSeconds,
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
	b.affinity = affinity
}

func (b *BaseWorkloadBuilder) GetAffinity() *corev1.Affinity {
	return b.affinity
}

func (b *BaseWorkloadBuilder) AddTerminationGracePeriodSeconds(seconds *int64) {
	b.terminationGracePeriodSeconds = seconds
}

func (b *BaseWorkloadBuilder) GetTerminationGracePeriodSeconds() *int64 {
	return b.terminationGracePeriodSeconds
}

func (b *BaseWorkloadBuilder) getDefaultPodTemplate() (*corev1.PodTemplateSpec, error) {
	containers := b.GetContainers()

	if len(containers) == 0 {
		return &corev1.PodTemplateSpec{}, ErrNoContainers
	}

	pod := &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      b.GetLabels(),
			Annotations: b.GetAnnotations(),
		},
		Spec: corev1.PodSpec{
			InitContainers:                b.GetInitContainers(),
			Containers:                    b.GetContainers(),
			Volumes:                       b.GetVolumes(),
			Affinity:                      b.GetAffinity(),
			TerminationGracePeriodSeconds: b.GetTerminationGracePeriodSeconds(),
		},
	}
	return pod, nil
}

func (b *BaseWorkloadBuilder) getOverridedPodTemplate() (*corev1.PodTemplateSpec, error) {
	pod, err := b.getDefaultPodTemplate()

	if err != nil {
		return nil, err
	}

	if b.podOverrides != nil {
		// Merge labels
		if len(b.podOverrides.Labels) > 0 {
			if pod.ObjectMeta.Labels == nil {
				pod.ObjectMeta.Labels = make(map[string]string)
			}
			for key, value := range b.podOverrides.Labels {
				pod.ObjectMeta.Labels[key] = value
			}
		}

		// Merge annotations
		if len(b.podOverrides.Annotations) > 0 {
			if pod.ObjectMeta.Annotations == nil {
				pod.ObjectMeta.Annotations = make(map[string]string)
			}
			for key, value := range b.podOverrides.Annotations {
				pod.ObjectMeta.Annotations[key] = value
			}
		}

		// Merge init containers
		if len(b.podOverrides.Spec.InitContainers) > 0 {
			pod.Spec.InitContainers = append(pod.Spec.InitContainers, b.podOverrides.Spec.InitContainers...)
		}

		// Merge containers
		if len(b.podOverrides.Spec.Containers) > 0 {
			pod.Spec.Containers = append(pod.Spec.Containers, b.podOverrides.Spec.Containers...)
		}

		// Merge volumes
		if len(b.podOverrides.Spec.Volumes) > 0 {
			pod.Spec.Volumes = append(pod.Spec.Volumes, b.podOverrides.Spec.Volumes...)
		}

		// Merge affinity
		if b.podOverrides.Spec.Affinity != nil {
			pod.Spec.Affinity = b.podOverrides.Spec.Affinity
		}

		// Merge termination grace period seconds
		if b.podOverrides.Spec.TerminationGracePeriodSeconds != nil {
			pod.Spec.TerminationGracePeriodSeconds = b.podOverrides.Spec.TerminationGracePeriodSeconds
		}
	}

	return pod, nil
}

func (b *BaseWorkloadBuilder) getPodTemplate() (*corev1.PodTemplateSpec, error) {
	if b.podOverrides != nil {
		return b.getOverridedPodTemplate()
	}
	return b.getDefaultPodTemplate()
}

var _ WorkloadReplicas = &BaseWorkloadReplicasBuilder{}

type BaseWorkloadReplicasBuilder struct {
	BaseWorkloadBuilder
	replicas *int32
}

func NewBaseWorkloadReplicasBuilder(
	client *client.Client,
	name string,
	labels map[string]string,
	annotations map[string]string,
	affinity *corev1.Affinity,
	podOverrides *corev1.PodTemplateSpec,
	terminationGracePeriodSeconds *int64,
	replicas *int32,
) *BaseWorkloadReplicasBuilder {
	return &BaseWorkloadReplicasBuilder{
		BaseWorkloadBuilder: *NewBaseWorkloadBuilder(
			client,
			name,
			labels,
			annotations,
			affinity,
			podOverrides,
			terminationGracePeriodSeconds,
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
