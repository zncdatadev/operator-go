package builder

import (
	"context"

	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/util"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type StatefulSetBuilder interface {
	Builder
	GetObject() *appv1.StatefulSet
	SetReplicas(replicas *int32)
	GetReplicas() *int32

	AddContainers(containers []corev1.Container)
	AddContainer(container corev1.Container)
	ResetContainers(containers []corev1.Container)
	GetContainers() []corev1.Container

	AddInitContainers(containers []corev1.Container)
	AddInitContainer(container corev1.Container)
	ResetInitContainers(containers []corev1.Container)
	GetInitContainers() []corev1.Container

	AddVolumes(volumes []corev1.Volume)
	AddVolume(volume corev1.Volume)
	ResetVolumes(volumes []corev1.Volume)
	GetVolumes() []corev1.Volume

	AddVolumeClaimTemplates(claims []corev1.PersistentVolumeClaim)
	AddVolumeClaimTemplate(claim corev1.PersistentVolumeClaim)
	ResetVolumeClaimTemplates(claims []corev1.PersistentVolumeClaim)
	GetVolumeClaimTemplates() []corev1.PersistentVolumeClaim

	AddTerminationGracePeriodSeconds(i int64)
	GetTerminationGracePeriodSeconds() *int64

	AddAffinity(affinity corev1.Affinity)
	GetAffinity() *corev1.Affinity
}

var _ StatefulSetBuilder = &GenericStatefulSetBuilder{}

type GenericStatefulSetBuilder struct {
	BaseResourceBuilder
	Options *RoleGroupOptions

	replicas                      *int32
	initContainers                []corev1.Container
	containers                    []corev1.Container
	volumes                       []corev1.Volume
	volumeClaimTemplates          []corev1.PersistentVolumeClaim
	terminationGracePeriodSeconds *int64
	affinity                      *corev1.Affinity
}

func NewGenericStatefulSetBuilder(
	client *client.Client,
	options *RoleGroupOptions,
) *GenericStatefulSetBuilder {
	return &GenericStatefulSetBuilder{
		BaseResourceBuilder: BaseResourceBuilder{
			Client:  client,
			Options: options,
		},
		Options: options,
	}
}

func (b *GenericStatefulSetBuilder) GetObject() *appv1.StatefulSet {
	obj := &appv1.StatefulSet{
		ObjectMeta: b.GetObjectMeta(),
		Spec: appv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: b.Options.GetMatchingLabels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      b.Options.GetLabels(),
					Annotations: b.Options.GetAnnotations(),
				},
			},
		},
	}
	return obj
}

func (b *GenericStatefulSetBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	obj := b.GetObject()

	if len(obj.Spec.Template.Spec.Containers) == 0 {
		obj.Spec.Template.Spec.Containers = []corev1.Container{
			{
				Name:    b.Options.Name,
				Image:   b.Options.GetImage().String(),
				Env:     util.EnvsToEnvVars(b.Options.EnvOverrides),
				Command: b.Options.CommandOverrides,
			},
		}
	}

	if obj.Spec.Replicas == nil {
		replicas := int32(1)
		obj.Spec.Replicas = &replicas
	}

	return obj, nil
}

func (b *GenericStatefulSetBuilder) SetReplicas(replicas *int32) {
	b.replicas = replicas
}

func (b *GenericStatefulSetBuilder) GetReplicas() *int32 {
	return b.replicas
}

func (b *GenericStatefulSetBuilder) AddContainer(container corev1.Container) {
	b.containers = append(b.containers, container)
}

func (b *GenericStatefulSetBuilder) ResetContainers(containers []corev1.Container) {
	b.containers = containers
}

func (b *GenericStatefulSetBuilder) AddContainers(containers []corev1.Container) {
	b.containers = append(b.containers, containers...)
}

func (b *GenericStatefulSetBuilder) GetContainers() []corev1.Container {
	return b.containers
}

func (b *GenericStatefulSetBuilder) AddInitContainer(container corev1.Container) {
	b.initContainers = append(b.initContainers, container)
}

func (b *GenericStatefulSetBuilder) AddInitContainers(containers []corev1.Container) {
	b.initContainers = append(b.initContainers, containers...)
}

func (b *GenericStatefulSetBuilder) ResetInitContainers(containers []corev1.Container) {
	b.initContainers = containers
}

func (b *GenericStatefulSetBuilder) GetInitContainers() []corev1.Container {
	return b.initContainers
}

func (b *GenericStatefulSetBuilder) AddVolume(volume corev1.Volume) {
	b.volumes = append(b.volumes, volume)
}

func (b *GenericStatefulSetBuilder) AddVolumes(volumes []corev1.Volume) {
	b.volumes = append(b.volumes, volumes...)
}

func (b *GenericStatefulSetBuilder) ResetVolumes(volumes []corev1.Volume) {
	b.volumes = volumes
}

func (b *GenericStatefulSetBuilder) GetVolumes() []corev1.Volume {
	return b.volumes
}

func (b *GenericStatefulSetBuilder) AddVolumeClaimTemplate(claim corev1.PersistentVolumeClaim) {
	b.volumeClaimTemplates = append(b.volumeClaimTemplates, claim)
}

func (b *GenericStatefulSetBuilder) AddVolumeClaimTemplates(claims []corev1.PersistentVolumeClaim) {
	b.volumeClaimTemplates = append(b.volumeClaimTemplates, claims...)
}

func (b *GenericStatefulSetBuilder) ResetVolumeClaimTemplates(claims []corev1.PersistentVolumeClaim) {
	b.volumeClaimTemplates = claims
}

func (b *GenericStatefulSetBuilder) GetVolumeClaimTemplates() []corev1.PersistentVolumeClaim {
	return b.volumeClaimTemplates
}

func (b *GenericStatefulSetBuilder) GetTerminationGracePeriodSeconds() *int64 {
	return b.terminationGracePeriodSeconds
}

func (b *GenericStatefulSetBuilder) AddTerminationGracePeriodSeconds(i int64) {
	b.terminationGracePeriodSeconds = &i
}

func (b *GenericStatefulSetBuilder) AddAffinity(affinity corev1.Affinity) {
	b.affinity = &affinity
}

func (b *GenericStatefulSetBuilder) GetAffinity() *corev1.Affinity {
	return b.affinity
}
