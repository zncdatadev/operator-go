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

var _ StatefulSetBuilder = &GenericStatefulSetBuilder{}

type GenericStatefulSetBuilder struct {
	BaseWorkloadReplicasBuilder

	volumeClaimTemplates []corev1.PersistentVolumeClaim
}

func NewGenericStatefulSetBuilder(
	client *client.Client,
	options *WorkloadReplicasOptions,
) *GenericStatefulSetBuilder {
	return &GenericStatefulSetBuilder{
		BaseWorkloadReplicasBuilder: *NewBaseWorkloadReplicasBuilder(options),
	}
}

func (b *GenericStatefulSetBuilder) GetObject() *appv1.StatefulSet {
	obj := &appv1.StatefulSet{
		ObjectMeta: b.GetObjectMeta(),
		Spec: appv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: b.GetMatchingLabels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      b.GetLabels(),
					Annotations: b.GetAnnotations(),
				},
				Spec: corev1.PodSpec{
					InitContainers:                b.initContainers,
					Containers:                    b.containers,
					Volumes:                       b.volumes,
					Affinity:                      b.Affinity,
					TerminationGracePeriodSeconds: b.terminationGracePeriodSeconds,
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
				Name:    b.GetName(),
				Image:   b.Image.String(),
				Env:     util.EnvsToEnvVars(b.EnvOverrides),
				Command: b.CommandOverrides,
			},
		}
	}

	if obj.Spec.Replicas == nil {
		replicas := int32(1)
		obj.Spec.Replicas = &replicas
	}

	return obj, nil
}

func (b *GenericStatefulSetBuilder) AddVolumeClaimTemplate(pvc *corev1.PersistentVolumeClaim) {
	b.volumeClaimTemplates = append(b.volumeClaimTemplates, *pvc)
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
