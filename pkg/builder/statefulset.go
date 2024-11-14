package builder

import (
	"context"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/util"
)

var _ StatefulSetBuilder = &StatefulSet{}

type StatefulSet struct {
	BaseWorkloadReplicasBuilder

	volumeClaimTemplates []corev1.PersistentVolumeClaim
}

func NewStatefulSetBuilder(
	client *client.Client,
	name string,
	replicas *int32,
	image *util.Image,
	overrides *commonsv1alpha1.OverridesSpec,
	roleGroupConfig *commonsv1alpha1.RoleGroupConfigSpec,
	options ...Option,
) *StatefulSet {
	return &StatefulSet{
		BaseWorkloadReplicasBuilder: *NewBaseWorkloadReplicasBuilder(
			client,
			name,
			replicas,
			image,
			overrides,
			roleGroupConfig,
			options...,
		),
	}
}

func (b *StatefulSet) GetObject() (*appv1.StatefulSet, error) {
	tpl, err := b.GetPodTemplate()
	if err != nil {
		return nil, err
	}
	obj := &appv1.StatefulSet{
		ObjectMeta: b.GetObjectMeta(),
		Spec: appv1.StatefulSetSpec{
			Replicas:             b.GetReplicas(),
			Selector:             b.GetLabelSelector(),
			ServiceName:          b.GetName(),
			Template:             *tpl,
			VolumeClaimTemplates: b.GetVolumeClaimTemplates(),
		},
	}
	return obj, nil
}

func (b *StatefulSet) Build(ctx context.Context) (ctrlclient.Object, error) {
	return b.GetObject()
}

func (b *StatefulSet) AddVolumeClaimTemplate(pvc *corev1.PersistentVolumeClaim) {
	b.volumeClaimTemplates = append(b.volumeClaimTemplates, *pvc)
}

func (b *StatefulSet) AddVolumeClaimTemplates(claims []corev1.PersistentVolumeClaim) {
	b.volumeClaimTemplates = append(b.volumeClaimTemplates, claims...)
}

func (b *StatefulSet) ResetVolumeClaimTemplates(claims []corev1.PersistentVolumeClaim) {
	b.volumeClaimTemplates = claims
}

func (b *StatefulSet) GetVolumeClaimTemplates() []corev1.PersistentVolumeClaim {
	if b.volumeClaimTemplates == nil {
		b.volumeClaimTemplates = []corev1.PersistentVolumeClaim{}
	}
	return b.volumeClaimTemplates
}
