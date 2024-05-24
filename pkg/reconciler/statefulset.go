package reconciler

import (
	"context"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	appv1 "k8s.io/api/apps/v1"
)

var _ ResourceReconciler[builder.StatefulSetBuilder] = &StatefulSetReconciler{}

type StatefulSetReconciler struct {
	GenericResourceReconciler[builder.StatefulSetBuilder]
	Options *builder.RoleGroupOptions
}

// getReplicas returns the number of replicas for the role group.
// handle cluster operation stopped state.
func (r *StatefulSetReconciler) getReplicas() *int32 {
	clusterOptions := r.Options.GetClusterOperation()
	if clusterOptions != nil && clusterOptions.Stopped {
		logger.Info("Cluster operation stopped, set replicas to 0")
		zero := int32(0)
		return &zero
	}
	return nil
}

func (r *StatefulSetReconciler) Reconcile(ctx context.Context) Result {
	resourceBuilder := r.GetBuilder()
	resourceBuilder.SetReplicas(r.getReplicas())
	resource, err := resourceBuilder.Build(ctx)

	if err != nil {
		return NewResult(true, 0, err)
	}
	return r.ResourceReconcile(ctx, resource)
}

func (r *StatefulSetReconciler) Ready(ctx context.Context) Result {
	obj := appv1.StatefulSet{
		ObjectMeta: r.GetObjectMeta(),
	}
	logger.V(1).Info("Checking statefulset ready", "namespace", obj.Namespace, "name", obj.Name)
	if err := r.Client.Get(ctx, &obj); err != nil {
		return NewResult(true, 0, err)
	}
	if obj.Status.ReadyReplicas == *obj.Spec.Replicas {
		logger.Info("StatefulSet is ready", "namespace", obj.Namespace, "name", obj.Name, "replicas", *obj.Spec.Replicas, "readyReplicas", obj.Status.ReadyReplicas)
		return NewResult(false, 0, nil)
	}
	logger.Info("StatefulSet is not ready", "namespace", obj.Namespace, "name", obj.Name, "replicas", *obj.Spec.Replicas, "readyReplicas", obj.Status.ReadyReplicas)
	return NewResult(false, 5, nil)
}

func NewStatefulSetReconciler(
	client *client.Client,
	options *builder.RoleGroupOptions,
	stsBuilder builder.StatefulSetBuilder,
) *StatefulSetReconciler {
	return &StatefulSetReconciler{
		GenericResourceReconciler: *NewGenericResourceReconciler[builder.StatefulSetBuilder](
			client,
			options,
			stsBuilder,
		),
	}
}
