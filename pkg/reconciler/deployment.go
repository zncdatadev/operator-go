package reconciler

import (
	"context"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	appv1 "k8s.io/api/apps/v1"
)

var _ ResourceReconciler[builder.DeploymentBuilder] = &DeploymentReconciler{}

type DeploymentReconciler struct {
	GenericResourceReconciler[builder.DeploymentBuilder]
	Options *builder.RoleGroupOptions
}

// getReplicas returns the number of replicas for the role group.
// handle cluster operation stopped state.
func (r *DeploymentReconciler) getReplicas() *int32 {
	clusterOperations := r.Options.GetClusterOperation()
	if clusterOperations != nil && clusterOperations.Stopped {
		logger.Info("Cluster operation stopped, set replicas to 0")
		zero := int32(0)
		return &zero
	}
	return nil
}

func (r *DeploymentReconciler) Reconcile(ctx context.Context) Result {
	resourceBuilder := r.GetBuilder()
	replicas := r.getReplicas()
	if replicas != nil {
		resourceBuilder.SetReplicas(replicas)
	}
	resource, err := resourceBuilder.Build(ctx)

	if err != nil {
		return NewResult(true, 0, err)
	}
	return r.ResourceReconcile(ctx, resource)
}

func (r *DeploymentReconciler) Ready(ctx context.Context) Result {

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

func NewDeploymentReconciler(
	client *client.Client,
	options *builder.RoleGroupOptions,
	deployBuilder builder.DeploymentBuilder,
) *DeploymentReconciler {
	return &DeploymentReconciler{
		GenericResourceReconciler: *NewGenericResourceReconciler[builder.DeploymentBuilder](
			client,
			options,
			deployBuilder,
		),
		Options: options,
	}
}
