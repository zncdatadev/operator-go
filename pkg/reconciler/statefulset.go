package reconciler

import (
	"context"

	appv1 "k8s.io/api/apps/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
)

var _ ResourceReconciler[builder.StatefulSetBuilder] = &StatefulSet{}

type StatefulSet struct {
	GenericResourceReconciler[builder.StatefulSetBuilder]

	// When the cluster is stopped, the statefulset will be scaled to 0
	// and the reconcile will be not executed until the cluster is started
	Stopped bool
}

func (r *StatefulSet) Reconcile(ctx context.Context) (ctrl.Result, error) {
	resourceBuilder := r.GetBuilder()

	if r.Stopped {
		resourceBuilder.SetReplicas(&[]int32{0}[0])
	}

	resource, err := resourceBuilder.Build(ctx)

	if err != nil {
		return ctrl.Result{}, err
	}
	return r.ResourceReconcile(ctx, resource)
}

func (r *StatefulSet) Ready(ctx context.Context) (ctrl.Result, error) {
	obj := appv1.StatefulSet{
		ObjectMeta: r.GetObjectMeta(),
	}
	logger.V(1).Info("Checking statefulset ready", "namespace", obj.Namespace, "name", obj.Name)
	if err := r.Client.Get(ctx, &obj); err != nil {
		return ctrl.Result{}, err
	}
	if obj.Status.ReadyReplicas == *obj.Spec.Replicas {
		logger.Info("StatefulSet is ready", "namespace", obj.Namespace, "name", obj.Name, "replicas", *obj.Spec.Replicas, "readyReplicas", obj.Status.ReadyReplicas)
		return ctrl.Result{}, nil
	}
	logger.Info("StatefulSet is not ready", "namespace", obj.Namespace, "name", obj.Name, "replicas", *obj.Spec.Replicas, "readyReplicas", obj.Status.ReadyReplicas)
	return ctrl.Result{Requeue: true}, nil
}

func NewStatefulSet(
	client *client.Client,
	name string,
	stsBuilder builder.StatefulSetBuilder,
	stopped bool,
) *StatefulSet {
	return &StatefulSet{
		GenericResourceReconciler: *NewGenericResourceReconciler[builder.StatefulSetBuilder](
			client,
			name,
			stsBuilder,
		),
		Stopped: stopped,
	}
}
