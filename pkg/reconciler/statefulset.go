package reconciler

import (
	"context"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	appv1 "k8s.io/api/apps/v1"
)

var _ ResourceReconciler[builder.StatefulSetBuilder] = &StatefulSet{}

type StatefulSet struct {
	GenericResourceReconciler[builder.StatefulSetBuilder]

	Stopped bool
}

// getReplicas returns the number of replicas for the StatefulSet.
func (r *StatefulSet) getReplicas() *int32 {
	if r.Stopped {
		logger.Info("Cluster operation stopped, set replicas to 0")
		zero := int32(0)
		return &zero
	}
	return nil
}

func (r *StatefulSet) Reconcile(ctx context.Context) *Result {
	resourceBuilder := r.GetBuilder()
	resourceBuilder.SetReplicas(r.getReplicas())
	resource, err := resourceBuilder.Build(ctx)

	if err != nil {
		return NewResult(true, 0, err)
	}
	return r.ResourceReconcile(ctx, resource)
}

func (r *StatefulSet) Ready(ctx context.Context) *Result {
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

func NewStatefulSet(
	client *client.Client,
	options *ResourceReconcilerOptions,
	stsBuilder builder.StatefulSetBuilder,
) *StatefulSet {
	return &StatefulSet{
		GenericResourceReconciler: *NewGenericResourceReconciler[builder.StatefulSetBuilder](
			client,
			options,
			stsBuilder,
		),
	}
}
