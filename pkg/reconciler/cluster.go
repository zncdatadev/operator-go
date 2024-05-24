package reconciler

import (
	"context"
	"reflect"

	apiv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/builder"
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

type BaseClusterReconciler[T AnySpec] struct {
	BaseReconciler[T]
	resources []Reconciler
}

func NewBaseClusterReconciler[T AnySpec](
	client *client.Client,
	options builder.Options,
	spec T,
) *BaseClusterReconciler[T] {
	return &BaseClusterReconciler[T]{
		BaseReconciler: BaseReconciler[T]{
			Client:  client,
			Options: options,
			Spec:    spec,
		},
	}
}

func (r *BaseClusterReconciler[T]) GetResources() []Reconciler {
	return r.resources
}

func (r *BaseClusterReconciler[T]) AddResource(resource Reconciler) {
	r.resources = append(r.resources, resource)
}

func (r *BaseClusterReconciler[T]) RegisterResources(ctx context.Context) error {
	panic("unimplemented")
}

func (r *BaseClusterReconciler[T]) Ready(ctx context.Context) Result {
	for _, resource := range r.resources {
		if result := resource.Ready(ctx); result.RequeueOrNot() {
			return result
		}
	}
	return NewResult(false, 0, nil)
}

func (r *BaseClusterReconciler[T]) Reconcile(ctx context.Context) Result {
	for _, resource := range r.resources {
		result := resource.Reconcile(ctx)
		if result.RequeueOrNot() {
			return result
		}
	}
	return NewResult(false, 0, nil)
}

type RoleReconciler interface {
	ClusterReconciler
}

var _ RoleReconciler = &BaseRoleReconciler[AnySpec]{}

type BaseRoleReconciler[T AnySpec] struct {
	BaseClusterReconciler[T]
	Options *builder.RoleOptions
}

// MergeRoleGroupSpec
// merge right to left, if field of right not exist in left, add it to left.
// else skip it.
// merge will modify left, so left must be a pointer.
func (b *BaseRoleReconciler[T]) MergeRoleGroupSpec(roleGroup any) {
	leftValue := reflect.ValueOf(roleGroup)
	rightValue := reflect.ValueOf(b.Spec)

	if leftValue.Kind() == reflect.Ptr {
		leftValue = leftValue.Elem()
	} else {
		panic("roleGroup is not a pointer")
	}

	if rightValue.Kind() == reflect.Ptr {
		rightValue = rightValue.Elem()
	}

	for i := 0; i < rightValue.NumField(); i++ {
		rightField := rightValue.Field(i)

		if rightField.IsZero() {
			continue
		}
		rightFieldName := rightValue.Type().Field(i).Name
		leftField := leftValue.FieldByName(rightFieldName)

		// if field exist in left, add it to left
		if leftField.IsValid() && leftField.IsZero() {
			leftValue.Set(rightField)
			logger.V(5).Info("Merge role group", "field", rightFieldName, "value", rightField)
		}
	}
}

func (b *BaseRoleReconciler[T]) GetClusterOperation() *apiv1alpha1.ClusterOperationSpec {
	return b.Options.GetClusterOperation()
}

func (b *BaseRoleReconciler[T]) Ready(ctx context.Context) Result {
	for _, resource := range b.resources {
		if result := resource.Ready(ctx); result.RequeueOrNot() {
			return result
		}
	}
	return NewResult(false, 0, nil)
}

func NewBaseRoleReconciler[T AnySpec](
	client *client.Client,
	roleOptions *builder.RoleOptions,
	spec T,
) *BaseRoleReconciler[T] {

	return &BaseRoleReconciler[T]{
		BaseClusterReconciler: *NewBaseClusterReconciler[T](
			client,
			roleOptions,
			spec,
		),
		Options: roleOptions,
	}
}
