package builder

import (
	"context"

	resourceClient "github.com/zncdatadev/operator-go/pkg/client"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ JobBuilder = &jobBuilder{}

type jobBuilder struct {
	BaseWorkloadBuilder

	resetPolicy *corev1.RestartPolicy
}

func NewGenericJobBuilder(
	client *resourceClient.Client,
	options *WorkloadOptions,
) JobBuilder {
	return &jobBuilder{
		BaseWorkloadBuilder: *NewBaseWorkloadBuilder(options),
	}
}

func (b *jobBuilder) GetObject() *batchv1.Job {
	obj := &batchv1.Job{
		ObjectMeta: b.GetObjectMeta(),
		Spec: batchv1.JobSpec{
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
					RestartPolicy:                 *b.resetPolicy,
					Affinity:                      b.Affinity,
					TerminationGracePeriodSeconds: b.terminationGracePeriodSeconds,
				},
			},
		},
	}
	return obj
}

func (b *jobBuilder) SetRestPolicy(policy *corev1.RestartPolicy) {
	b.resetPolicy = policy
}

func (b *jobBuilder) Build(_ context.Context) (ctrlclient.Object, error) {
	return b.GetObject(), nil
}
