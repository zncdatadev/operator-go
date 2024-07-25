package reconciler

import (
	"context"
	"errors"
	"reflect"

	apiv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/client"
)

var (
	ErrRoleSpecNotPointer = errors.New("role spec must be a pointer")
)

type RoleReconciler interface {
	ClusterReconciler
	// Get the full name of the role, formatted as `<clusterName>-<roleName>`
	GetFullName() string
	// Register resources based on roleGroup
}

type RoleGroupResourceReconcilersGetter interface {
	GetResourceReconcilers(info *RoleGroupInfo, roleGroupSpec any) ([]Reconciler, error)
}

var _ RoleReconciler = &BaseRoleReconciler[AnySpec]{}

type BaseRoleReconciler[T AnySpec] struct {
	BaseCluster[T]
	RoleInfo RoleInfo
}

func NewBaseRoleReconciler[T AnySpec](
	client *client.Client,
	roleInfo RoleInfo,
	clusterOperation *apiv1alpha1.ClusterOperationSpec,
	spec T, // spec of the role
) *BaseRoleReconciler[T] {
	return &BaseRoleReconciler[T]{
		BaseCluster: *NewBaseCluster[T](
			client,
			roleInfo.ClusterInfo,
			clusterOperation,
			spec,
		),
		RoleInfo: roleInfo,
	}
}

func (r *BaseRoleReconciler[T]) GetName() string {
	return r.RoleInfo.GetRoleName()
}

func (r *BaseRoleReconciler[T]) GetFullName() string {
	return r.RoleInfo.GetFullName()
}

func (r *BaseRoleReconciler[T]) GetRoleGroups() (map[string]AnySpec, error) {

	roleGroups := map[string]AnySpec{}

	value := reflect.ValueOf(r.Spec)

	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}

	roleGroupsReflect := value.FieldByName("RoleGroups")

	iter := roleGroupsReflect.MapRange()
	for iter.Next() {
		key := iter.Key()
		value := iter.Value()

		// construct a new roleGroup pointer
		roleGroupPrt := reflect.New(value.Type())
		roleGroupPrt.Elem().Set(value)

		mergedRoleGroup := r.MergeRoleGroupSpec(roleGroupPrt.Interface())
		name := key.String()
		roleGroups[name] = mergedRoleGroup
		logger.Info("Merged field to role group", "role", r.GetName(), "roleGroup", name)
	}

	return roleGroups, nil

}

func (r *BaseRoleReconciler[T]) RegisterResources(ctx context.Context) error {
	panic("unimplemented")
}

// MergeRoleGroupSpec merges the roleGroup spec with the base role spec.
// It merges the fields from the right (roleSpec) to the left (roleGroup).
// If a field exists in the left but is zero, it will be replaced by the corresponding field from the right.
// The left must be a pointer, as the merge operation modifies it.
// The fields "RoleGroups" and "PodDisruptionBudget" are excluded during the merge.
// You don't need to use the return value of this method because it directly modifies the passed roleGroup.
//
// Example:
//
//	left := &RoleGroupSpec{
//		Replicas:      1,
//		Config:        config,
//		EnvOverrides:  envOverridesRoleGroup,
//	}
//
//	right := RoleSpec{
//		RoleGroups:        rolegroups,            // this field is excluded
//		EnvOverrides:      envOverridesRole,
//		CommandOverrides:  commandOverrides,
//	}
//
//	result := &RoleGroupSpec{
//		Replicas:          1,
//		Config:            config,
//		EnvOverrides:      envOverridesRoleGroup,   // `EnvOverrides` exists in left, so it is not replaced
//		CommandOverrides:  commandOverrides,        // Add RoleSpec.CommandOverrides to left
//	}
func (b *BaseRoleReconciler[T]) MergeRoleGroupSpec(roleGroup AnySpec) AnySpec {
	leftValue := reflect.ValueOf(roleGroup)
	rightValue := reflect.ValueOf(b.Spec) // When b.Spec is T, it is not a pointer

	if leftValue.Kind() == reflect.Ptr {
		leftValue = leftValue.Elem()
	} else {
		panic(ErrRoleSpecNotPointer)
	}

	// If the b.Spec is a pointer, get the actual value pointed to directly
	if rightValue.Kind() == reflect.Ptr {
		rightValue = rightValue.Elem()
	}

	for i := 0; i < rightValue.NumField(); i++ {
		rightField := rightValue.Field(i)

		if rightField.IsZero() {
			logger.V(5).Info("Field in role is empty, skipping", "field", rightValue.Type().Field(i).Name)
			continue // Skip if the right field is zero
		}

		rightFieldName := rightValue.Type().Field(i).Name
		leftField := leftValue.FieldByName(rightFieldName)

		// If the left field exists and is zero, perform the merge
		if leftField.IsValid() && leftField.CanSet() && leftField.IsZero() {
			leftField.Set(rightField)                                                     // Copy the field value
			logger.V(5).Info("Merging role filed to role group", "field", rightFieldName) // Log the field and its value
		} else {
			logExtra := map[string]interface{}{
				"isValid": leftField.IsValid(),
				"canSet":  leftField.CanSet(),
			}

			if leftField.IsValid() {
				logExtra["isZero"] = leftField.IsZero()
			}

			logger.V(5).Info("Can not merge role field to role group, skipping", logExtra)
		}
	}
	return roleGroup
}
