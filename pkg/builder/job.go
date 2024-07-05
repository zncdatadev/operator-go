package builder

import (
	"context"

	resourceClient "github.com/zncdatadev/operator-go/pkg/client"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ JobBuilder = &jobBuilder{}

type jobBuilder struct {
	BaseWorkloadBuilder

	resetPolicy *corev1.RestartPolicy
}

func NewGenericJobBuilder(
	client *resourceClient.Client,
	name string,
	labels map[string]string,
	annotations map[string]string,
	affinity *corev1.Affinity,
	podOverrides *corev1.PodTemplateSpec,
	terminationGracePeriodSeconds *int64,
) JobBuilder {
	return &jobBuilder{
		BaseWorkloadBuilder: *NewBaseWorkloadBuilder(
			client,
			name,
			labels,
			annotations,
			affinity,
			podOverrides,
			terminationGracePeriodSeconds,
		),
	}
}

func (b *jobBuilder) GetObject() (*batchv1.Job, error) {
	tpl, err := b.getPodTemplate()
	if err != nil {
		return nil, err
	}
	obj := &batchv1.Job{
		ObjectMeta: b.GetObjectMeta(),
		Spec: batchv1.JobSpec{
			Selector: b.GetLabelSelector(),
			Template: *tpl,
		},
	}
	return obj, nil
}

func (b *jobBuilder) SetRestPolicy(policy *corev1.RestartPolicy) {
	b.resetPolicy = policy
}

func (b *jobBuilder) Build(_ context.Context) (ctrlclient.Object, error) {
	return b.GetObject()
}
