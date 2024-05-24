package builder

import (
	"context"

	resourceClient "github.com/zncdatadev/operator-go/pkg/client"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type JobBuilder interface {
	Builder
	GetObject() *batchv1.Job

	AddContainers([]corev1.Container)
	AddContainer(corev1.Container)
	ResetContainers([]corev1.Container)
	GetContainers() []corev1.Container

	AddInitContainers([]corev1.Container)
	AddInitContainer(corev1.Container)
	ResetInitContainers([]corev1.Container)
	GetInitContainers() []corev1.Container

	AddVolumes([]corev1.Volume)
	AddVolume(corev1.Volume)
	ResetVolumes([]corev1.Volume)
	GetVolumes() []corev1.Volume

	SetRestPolicy(corev1.RestartPolicy)
}

type GenericJobBuilder struct {
	BaseResourceBuilder

	containers     []corev1.Container
	initContainers []corev1.Container
	volumes        []corev1.Volume
	resetPolicy    corev1.RestartPolicy
}

func NewGenericJobBuilder(
	client *resourceClient.Client,
	options Options,
) *GenericJobBuilder {
	return &GenericJobBuilder{
		BaseResourceBuilder: BaseResourceBuilder{
			Client:  client,
			Options: options,
		},
	}
}

func (b *GenericJobBuilder) GetObject() *batchv1.Job {
	obj := &batchv1.Job{
		ObjectMeta: b.GetObjectMeta(),
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: b.Options.GetLabels(),
				},
				Spec: corev1.PodSpec{
					InitContainers: b.initContainers,
					Containers:     b.containers,
					Volumes:        b.volumes,
					RestartPolicy:  b.resetPolicy,
				},
			},
		},
	}
	return obj
}

func (b *GenericJobBuilder) AddContainers(containers []corev1.Container) {
	b.containers = append(b.containers, containers...)
}

func (b *GenericJobBuilder) AddContainer(container corev1.Container) {
	b.AddContainers([]corev1.Container{container})
}

func (b *GenericJobBuilder) ResetContainers(containers []corev1.Container) {
	b.containers = containers
}

func (b *GenericJobBuilder) GetContainers() []corev1.Container {
	return b.containers
}

func (b *GenericJobBuilder) AddInitContainers(containers []corev1.Container) {
	b.initContainers = append(b.initContainers, containers...)

}

func (b *GenericJobBuilder) AddInitContainer(container corev1.Container) {
	b.AddInitContainers([]corev1.Container{container})
}

func (b *GenericJobBuilder) ResetInitContainers(containers []corev1.Container) {
	b.initContainers = containers
}

func (b *GenericJobBuilder) GetInitContainers() []corev1.Container {
	return b.initContainers
}

func (b *GenericJobBuilder) AddVolumes(volumes []corev1.Volume) {
	b.volumes = append(b.volumes, volumes...)

}

func (b *GenericJobBuilder) AddVolume(volume corev1.Volume) {
	b.AddVolumes([]corev1.Volume{volume})
}

func (b *GenericJobBuilder) ResetVolumes(volumes []corev1.Volume) {
	b.volumes = volumes
}

func (b *GenericJobBuilder) GetVolumes() []corev1.Volume {
	return b.volumes
}

func (b *GenericJobBuilder) SetRestPolicy(policy corev1.RestartPolicy) {
	b.resetPolicy = policy
}

func (b *GenericJobBuilder) Build(_ context.Context) (ctrlclient.Object, error) {
	return b.GetObject(), nil
}
