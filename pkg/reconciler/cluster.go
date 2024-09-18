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
	IsStopped() bool

	Run(ctx context.Context) (ctrl.Result, error)
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

func (r *BaseCluster[T]) IsStopped() bool {
	return r.ClusterOperation != nil && r.ClusterOperation.Stopped
}

func (r *BaseCluster[T]) RegisterResources(ctx context.Context) error {

	panic("unimplemented")
}

func (r *BaseCluster[T]) IsPaused(ctx context.Context) bool {
	if r.ClusterOperation != nil && r.ClusterOperation.ReconciliationPaused {
		logger.Info("Reconciliation paused", "cluster", r.GetName(), "namespace", r.GetNamespace(), "paused", "true")
		return true
	}
	return false
}

func (r *BaseCluster[T]) Ready(ctx context.Context) (ctrl.Result, error) {
	logger.V(5).Info("Checking readiness of cluster", "namespace", r.GetNamespace(), "cluster", r.GetName())
	for _, resource := range r.resources {
		if result, err := resource.Ready(ctx); !result.IsZero() || err != nil {
			return result, err
		}
	}
	logger.V(5).Info("Cluster is ready", "namespace", r.GetNamespace(), "cluster", r.GetName())
	return ctrl.Result{}, nil
}

func (r *BaseCluster[T]) Reconcile(ctx context.Context) (ctrl.Result, error) {
	logger.V(5).Info("Reconciling cluster", "namespace", r.GetNamespace(), "cluster", r.GetName())
	for _, resource := range r.resources {
		if result, err := resource.Reconcile(ctx); !result.IsZero() || err != nil {
			return result, err
		}
	}
	logger.V(5).Info("Reconciled cluster", "namespace", r.GetNamespace(), "cluster", r.GetName())
	return ctrl.Result{}, nil
}

func (r *BaseCluster[T]) Run(ctx context.Context) (ctrl.Result, error) {
	if r.IsPaused(ctx) {
		logger.Info("Reconciliation paused, skip reconcile", "cluster", r.GetName(), "namespace", r.GetNamespace())
		return ctrl.Result{}, nil
	}

	if result, err := r.Reconcile(ctx); !result.IsZero() || err != nil {
		return result, err
	}

	if result, err := r.Ready(ctx); !result.IsZero() || err != nil {
		return result, err
	}

	logger.Info("Reconciliation completed, all resources are ready", "cluster", r.GetName(), "namespace", r.GetNamespace())
	return ctrl.Result{}, nil
}
