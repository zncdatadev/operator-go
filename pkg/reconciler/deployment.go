package reconciler

import (
	"context"

	appv1 "k8s.io/api/apps/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
)

var _ ResourceReconciler[builder.DeploymentBuilder] = &Deployment{}

type Deployment struct {
	GenericResourceReconciler[builder.DeploymentBuilder]

	// When the cluster is stopped, the deployment will be scaled to 0
	// and the reconcile will be not executed until the cluster is started
	Stopped bool
}

func (r *Deployment) Reconcile(ctx context.Context) (ctrl.Result, error) {
	// TODO: Extract a doBuild method to invoke the implementation side's Build method and append some framework logic.
	// Consider abstracting a WorkloadReconciler on top of DeploymentReconciler to extract some of the logic into it.
	resourceBuilder := r.GetBuilder()

	if r.Stopped {
		resourceBuilder.SetReplicas(ptr.To[int32](0))
	}

	resource, err := resourceBuilder.Build(ctx)

	if err != nil {
		return ctrl.Result{}, err
	}
	return r.ResourceReconcile(ctx, resource)
}

func (r *Deployment) Ready(ctx context.Context) (ctrl.Result, error) {
	obj := &appv1.Deployment{
		ObjectMeta: r.GetObjectMeta(),
	}
	logger.V(1).Info("Checking deployment ready", "namespace", obj.Namespace, "name", obj.Name)
	if err := r.Client.GetWithObject(ctx, obj); err != nil {
		return ctrl.Result{}, err
	}
	if obj.Status.ReadyReplicas == *obj.Spec.Replicas {
		logger.Info("Deployment is ready", "namespace", obj.Namespace, "name", obj.Name, "replicas", *obj.Spec.Replicas, "readyReplicas", obj.Status.ReadyReplicas)
		return ctrl.Result{}, nil
	}
	logger.Info("Deployment is not ready", "namespace", obj.Namespace, "name", obj.Name, "replicas", *obj.Spec.Replicas, "readyReplicas", obj.Status.ReadyReplicas)
	return ctrl.Result{Requeue: true}, nil
}

func NewDeployment(
	client *client.Client,
	deployBuilder builder.DeploymentBuilder,
	stopped bool,
) *Deployment {
	return &Deployment{
		GenericResourceReconciler: *NewGenericResourceReconciler[builder.DeploymentBuilder](
			client,
			deployBuilder,
		),
		Stopped: stopped,
	}
}
