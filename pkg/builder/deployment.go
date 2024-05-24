package builder

import (
	"context"

	client "github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/util"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	DefaultReplicas = int32(1)
)

type DeploymentBuilder interface {
	Builder

	GetObject() *appv1.Deployment
	SetReplicas(replicas *int32)

	AddContainer(*corev1.Container)
	AddContainers([]corev1.Container)
	ResetContainers(containers []corev1.Container)

	AddInitContainer(*corev1.Container)
	AddInitContainers([]corev1.Container)
	ResetInitContainers(containers []corev1.Container)

	AddVolume(*corev1.Volume)
	AddVolumes([]corev1.Volume)
	ResetVolumes(volumes []corev1.Volume)

	AddTerminationGracePeriodSeconds(seconds *int64)
	AddAffinity(*corev1.Affinity)
}

var _ DeploymentBuilder = &GenericDeploymentBuilder{}

type GenericDeploymentBuilder struct {
	BaseResourceBuilder
	Options *RoleGroupOptions

	replicas                      *int32
	initContainers                []corev1.Container
	containers                    []corev1.Container
	volumes                       []corev1.Volume
	terminationGracePeriodSeconds *int64
	affinity                      *corev1.Affinity
}

func NewGenericDeploymentBuilder(
	client *client.Client,
	options *RoleGroupOptions,
) *GenericDeploymentBuilder {
	return &GenericDeploymentBuilder{
		BaseResourceBuilder: BaseResourceBuilder{
			Client:  client,
			Options: options,
		},
		Options: options,
	}
}

func (b *GenericDeploymentBuilder) GetObject() *appv1.Deployment {
	if b.replicas == nil {
		b.replicas = &DefaultReplicas
	}
	obj := &appv1.Deployment{
		ObjectMeta: b.GetObjectMeta(),
		Spec: appv1.DeploymentSpec{
			Replicas: b.replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: b.Options.GetMatchingLabels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      b.Options.GetLabels(),
					Annotations: b.Options.GetAnnotations(),
				},
				Spec: corev1.PodSpec{
					InitContainers:                b.initContainers,
					Containers:                    b.containers,
					Volumes:                       b.volumes,
					Affinity:                      b.affinity,
					TerminationGracePeriodSeconds: b.terminationGracePeriodSeconds,
				},
			},
		},
	}
	return obj
}

func (b *GenericDeploymentBuilder) SetReplicas(replicas *int32) {
	b.replicas = replicas
}

func (b *GenericDeploymentBuilder) AddContainer(container *corev1.Container) {
	b.containers = append(b.containers, *container)
}

func (b *GenericDeploymentBuilder) AddContainers(containers []corev1.Container) {
	b.containers = append(b.containers, containers...)
}

func (b *GenericDeploymentBuilder) ResetContainers(containers []corev1.Container) {
	b.containers = containers
}
func (b *GenericDeploymentBuilder) AddInitContainers(containers []corev1.Container) {
	b.initContainers = append(b.initContainers, containers...)
}

func (b *GenericDeploymentBuilder) AddInitContainer(container *corev1.Container) {
	b.initContainers = append(b.initContainers, *container)
}
func (b *GenericDeploymentBuilder) ResetInitContainers(containers []corev1.Container) {
	b.initContainers = containers
}

func (b *GenericDeploymentBuilder) AddVolume(volume *corev1.Volume) {
	b.volumes = append(b.volumes, *volume)
}
func (b *GenericDeploymentBuilder) AddVolumes(volumes []corev1.Volume) {
	b.volumes = append(b.volumes, volumes...)
}

func (b *GenericDeploymentBuilder) ResetVolumes(volumes []corev1.Volume) {
	b.volumes = volumes
}

func (b *GenericDeploymentBuilder) AddTerminationGracePeriodSeconds(seconds *int64) {
	b.terminationGracePeriodSeconds = seconds
}

func (b *GenericDeploymentBuilder) AddAffinity(affinity *corev1.Affinity) {
	b.affinity = affinity
}

func (b *GenericDeploymentBuilder) Build(_ context.Context) (ctrlclient.Object, error) {
	obj := b.GetObject()

	if b.containers == nil {
		obj.Spec.Template.Spec.Containers = []corev1.Container{
			{
				Name:    b.Options.Name,
				Image:   b.Options.GetImage().String(),
				Env:     util.EnvsToEnvVars(b.Options.EnvOverrides),
				Command: b.Options.CommandOverrides,
			},
		}
	}
	return obj, nil
}
