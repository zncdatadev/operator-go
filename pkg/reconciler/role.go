package reconciler

import (
	"context"
	"reflect"

	apiv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/client"
)

type RoleReconciler interface {
	ClusterReconciler

	RegisterResourceWithRoleGroup(ctx context.Context, name *RoleGroupName, roleGroup any) error
}

var _ RoleReconciler = &BaseRoleReconciler[AnySpec]{}

type BaseRoleReconciler[T AnySpec] struct {
	BaseCluster[T]
	name string
}

func NewBaseRoleReconciler[T AnySpec](
	client *client.Client,
	clusterName string, // name of the cluster
	name string, // name of the role
	clusterOperation *apiv1alpha1.ClusterOperationSpec,
	spec T, // spec of the role
) *BaseRoleReconciler[T] {
	return &BaseRoleReconciler[T]{
		BaseCluster: *NewBaseCluster[T](
			client,
			clusterName,
			clusterOperation,
			spec,
		),
		name: name,
	}
}

func (r *BaseRoleReconciler[T]) GetClusterName() string {
	return r.BaseCluster.name
}

func (r *BaseRoleReconciler[T]) RegisterResources(ctx context.Context) error {

	value := reflect.ValueOf(r.Spec)

	// if value is a pointer, get the value it points to
	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}

	roleGroups := value.FieldByName("RoleGroups").Interface().(map[string]any)

	for name, rg := range roleGroups {
		rgName := NewRoleGroupName(name, r.name, r.GetClusterName())
		r.MergeRoleGroupSpec(rg)

		if err := r.RegisterResourceWithRoleGroup(ctx, rgName, rg); err != nil {
			return err
		}
	}

	return nil
}

func (r *BaseRoleReconciler[T]) RegisterResourceWithRoleGroup(
	ctx context.Context,
	name *RoleGroupName,
	roleGroup any,
) error {
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
