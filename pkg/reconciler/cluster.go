package reconciler

import (
	"context"
	"reflect"

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
	resources        []Reconciler
}

func NewBaseCluster[T AnySpec](
	client *client.Client,
	name string, // name of the cluster, Normally it is the name of CR
	clusterOperation *apiv1alpha1.ClusterOperationSpec,

	spec T, // spec of the cluster
) *BaseCluster[T] {
	return &BaseCluster[T]{
		BaseReconciler: BaseReconciler[T]{
			Client: client,
			Name:   name,
			Spec:   spec,
		},
		ClusterOperation: clusterOperation,
	}
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

func (r *BaseCluster[T]) Ready(ctx context.Context) *Result {
	for _, resource := range r.resources {
		if result := resource.Ready(ctx); result.RequeueOrNot() {
			return result
		}
	}
	return NewResult(false, 0, nil)
}

func (r *BaseCluster[T]) Reconcile(ctx context.Context) *Result {
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

	RegisterResourceWithRoleGroup(ctx context.Context, name string, roleGroup any) error
}

var _ RoleReconciler = &BaseRoleReconciler[AnySpec]{}

type BaseRoleReconciler[T AnySpec] struct {
	BaseCluster[T]
}

func NewBaseRoleReconciler[T AnySpec](
	client *client.Client,
	name string, // name of the role
	clusterOperation *apiv1alpha1.ClusterOperationSpec,
	spec T, // spec of the role
) *BaseRoleReconciler[T] {
	return &BaseRoleReconciler[T]{
		BaseCluster: *NewBaseCluster[T](
			client,
			name,
			clusterOperation,
			spec,
		),
	}
}

func (r *BaseRoleReconciler[T]) RegisterResources(ctx context.Context) error {

	value := reflect.ValueOf(r.Spec)

	// if value is a pointer, get the value it points to
	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}

	roleGroups := value.FieldByName("RoleGroups").Interface().(map[string]any)

	for name, rg := range roleGroups {
		r.MergeRoleGroupSpec(rg)

		if err := r.RegisterResourceWithRoleGroup(ctx, name, rg); err != nil {
			return err
		}
	}

	return nil

}

func (r *BaseRoleReconciler[T]) RegisterResourceWithRoleGroup(ctx context.Context, name string, roleGroup any) error {
	panic("unimplemented")
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
