package reconciler

import (
	"context"

	apiv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/client"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	logger = ctrl.Log.WithName("reconciler")
)

type ClusterReconciler interface {
	Reconciler
	GetClusterOperation() *apiv1alpha1.ClusterOperationSpec
	GetResources() []Reconciler
	AddResource(resource Reconciler)
	RegisterResources(ctx context.Context) error
}

var _ ClusterReconciler = &BaseCluster[AnySpec]{}

type BaseCluster[T AnySpec] struct {
	BaseReconciler[T]
	ClusterOperation *apiv1alpha1.ClusterOperationSpec
	ClusterInfo      ClusterInfo
	resources        []Reconciler
}

func NewBaseCluster[T AnySpec](
	client *client.Client,
	clusterInfo ClusterInfo,
	clusterOperation *apiv1alpha1.ClusterOperationSpec,
	spec T, // spec of the cluster
) *BaseCluster[T] {
	return &BaseCluster[T]{
		BaseReconciler: BaseReconciler[T]{
			Client: client,
			Spec:   spec,
		},
		ClusterOperation: clusterOperation,
		ClusterInfo:      clusterInfo,
	}
}

func (r *BaseCluster[T]) GetName() string {
	return r.ClusterInfo.GetClusterName()
}

func (r *BaseCluster[T]) GetClusterOperation() *apiv1alpha1.ClusterOperationSpec {
	return r.ClusterOperation
}

func (r *BaseCluster[T]) GetResources() []Reconciler {
	return r.resources
}

func (r *BaseCluster[T]) AddResource(resource Reconciler) {
	r.resources = append(r.resources, resource)
}

func (r *BaseCluster[T]) RegisterResources(ctx context.Context) error {

	panic("unimplemented")
}

func (r *BaseCluster[T]) Paused(ctx context.Context) bool {
	if r.ClusterOperation != nil && r.ClusterOperation.ReconciliationPaused {
		logger.Info("Reconciliation paused", "cluster", r.GetName(), "namespace", r.GetNamespace(), "paused", "true")
		return true
	}
	return false
}

func (r *BaseCluster[T]) Ready(ctx context.Context) *Result {
	if r.Paused(ctx) {
		logger.Info("Reconciliation paused, skip ready check", "cluster", r.GetName(), "namespace", r.GetNamespace())
		return NewResult(true, 0, nil)
	}
	for _, resource := range r.resources {
		if result := resource.Ready(ctx); result.RequeueOrNot() {
			return result
		}
	}
	return NewResult(false, 0, nil)
}

func (r *BaseCluster[T]) Reconcile(ctx context.Context) *Result {
	if r.Paused(ctx) {
		logger.Info("Reconciliation paused, skip reconcile", "cluster", r.GetName(), "namespace", r.GetNamespace())
		return NewResult(true, 0, nil)
	}

	for _, resource := range r.resources {
		logger.Info("Reconciling resource", "cluster", r.GetName(), "namespace", r.GetNamespace(), "resource", resource.GetName())
		result := resource.Reconcile(ctx)
		if result.RequeueOrNot() {
			return result
		}
		logger.Info("Reconcile completed", "cluster", r.GetName(), "namespace", r.GetNamespace())
	}
	return NewResult(false, 0, nil)
}
