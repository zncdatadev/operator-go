package builder

import (
	"context"

	resourceClient "github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/util"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ JobBuilder = &Job{}

type Job struct {
	BaseWorkloadBuilder

	resetPolicy *corev1.RestartPolicy
}

func NewGenericJobBuilder(
	client *resourceClient.Client,
	name string, // this is resource name when creating
	image *util.Image,
	options WorkloadOptions,
) JobBuilder {
	return &Job{
		BaseWorkloadBuilder: *NewBaseWorkloadBuilder(
			client,
			name,
			image,
			options,
		),
	}
}

func (b *Job) GetObject() (*batchv1.Job, error) {
	podTemplate, err := b.getPodTemplate()
	if err != nil {
		return nil, err
	}

	if b.resetPolicy != nil {
		podTemplate.Spec.RestartPolicy = *b.resetPolicy
	}

	obj := &batchv1.Job{
		ObjectMeta: b.GetObjectMeta(),
		Spec: batchv1.JobSpec{
			Selector: b.GetLabelSelector(),
			Template: *podTemplate,
		},
	}
	return obj, nil
}

func (b *Job) SetRestPolicy(policy *corev1.RestartPolicy) {
	b.resetPolicy = policy
}

func (b *Job) Build(_ context.Context) (ctrlclient.Object, error) {
	return b.GetObject()
}
