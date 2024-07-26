package reconciler

import (
	"context"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	appv1 "k8s.io/api/apps/v1"
)

var _ ResourceReconciler[builder.DeploymentBuilder] = &Deployment{}

type Deployment struct {
	GenericResourceReconciler[builder.DeploymentBuilder]

	// When the cluster is stopped, the deployment will be scaled to 0
	// and the reconcile will be not executed until the cluster is started
	Stopped bool
}

func (r *Deployment) Reconcile(ctx context.Context) *Result {
	// TODO: Extract a doBuild method to invoke the implementation side's Build method and append some framework logic.
	// Consider abstracting a WorkloadReconciler on top of DeploymentReconciler to extract some of the logic into it.
	resourceBuilder := r.GetBuilder()

	if r.Stopped {
		resourceBuilder.SetReplicas(&[]int32{0}[0])
	}

	resource, err := resourceBuilder.Build(ctx)

	if err != nil {
		return NewResult(true, 0, err)
	}
	return r.ResourceReconcile(ctx, resource)
}

func (r *Deployment) Ready(ctx context.Context) *Result {

	obj := appv1.Deployment{
		ObjectMeta: r.GetObjectMeta(),
	}
	logger.V(1).Info("Checking deployment ready", "namespace", obj.Namespace, "name", obj.Name)
	if err := r.Client.Get(ctx, &obj); err != nil {
		return NewResult(true, 0, err)
	}
	if obj.Status.ReadyReplicas == *obj.Spec.Replicas {
		logger.Info("Deployment is ready", "namespace", obj.Namespace, "name", obj.Name, "replicas", *obj.Spec.Replicas, "readyReplicas", obj.Status.ReadyReplicas)
		return NewResult(false, 0, nil)
	}
	logger.Info("Deployment is not ready", "namespace", obj.Namespace, "name", obj.Name, "replicas", *obj.Spec.Replicas, "readyReplicas", obj.Status.ReadyReplicas)
	return NewResult(false, 5, nil)
}

func NewDeployment(
	client *client.Client,
	name string,
	deployBuilder builder.DeploymentBuilder,
	stopped bool,
) *Deployment {
	return &Deployment{
		GenericResourceReconciler: *NewGenericResourceReconciler[builder.DeploymentBuilder](
			client,
			name,
			deployBuilder,
		),
		Stopped: stopped,
	}
}
