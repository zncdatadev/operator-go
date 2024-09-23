package reconciler

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	batchv1 "k8s.io/api/batch/v1"
)

var _ ResourceReconciler[builder.JobBuilder] = &Job{}

type Job struct {
	*GenericResourceReconciler[builder.JobBuilder]
}

func (r *Job) Reconcile(ctx context.Context) (ctrl.Result, error) {
	// TODO: Extract a doBuild method to invoke the implementation side's Build method and append some framework logic.
	// Consider abstracting a WorkloadReconciler on top of DeploymentReconciler to extract some of the logic into it.
	resourceBuilder := r.GetBuilder()

	resource, err := resourceBuilder.Build(ctx)

	if err != nil {
		return ctrl.Result{}, err
	}
	return r.ResourceReconcile(ctx, resource)
}

func (r *Job) Ready(ctx context.Context) (ctrl.Result, error) {

	obj := batchv1.Job{
		ObjectMeta: r.GetObjectMeta(),
	}
	logger.V(1).Info("Checking job ready", "namespace", obj.Namespace, "name", obj.Name)
	if err := r.Client.GetWithObject(ctx, &obj); err != nil {
		return ctrl.Result{}, err
	}
	if obj.Status.Succeeded == *obj.Spec.Parallelism {
		logger.Info("Job is ready", "namespace", obj.Namespace, "name", obj.Name, "Parallelism", *obj.Spec.Parallelism, "succeeded", obj.Status.Succeeded)
		return ctrl.Result{}, nil
	}
	logger.Info("Job is not ready", "namespace", obj.Namespace, "name", obj.Name, "Parallelism", *obj.Spec.Parallelism, "succeeded", obj.Status.Succeeded)
	return ctrl.Result{Requeue: true}, nil
}

func NewJob(
	client *client.Client,
	name string,
	jobBuilder builder.JobBuilder,
) *Job {
	return &Job{
		GenericResourceReconciler: NewGenericResourceReconciler[builder.JobBuilder](
			client,
			name,
			jobBuilder,
		),
	}
}
