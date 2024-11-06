package reconciler

import (
	"context"
	"errors"
	"reflect"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	ErrRoleSpecNotPointer = errors.New("role spec must be a pointer")
)

type RoleReconciler interface {
	Reconciler
	GetResources() []Reconciler
	AddResource(resource Reconciler)
	RegisterResources(ctx context.Context) error
	IsStopped() bool

	// Get the full name of the role, formatted as `<clusterName>-<roleName>`
	GetFullName() string
}

type RoleGroupResourceReconcilersGetter interface {
	GetResourceReconcilers(info *RoleGroupInfo, roleGroupSpec any) ([]Reconciler, error)
}

var _ RoleReconciler = &BaseRoleReconciler[AnySpec]{}

type BaseRoleReconciler[T AnySpec] struct {
	BaseReconciler[T]
	ClusterStopped bool
	ClusterInfo    ClusterInfo
	resources      []Reconciler
	RoleInfo       RoleInfo
}

func NewBaseRoleReconciler[T AnySpec](
	client *client.Client,
	clusterStopped bool,
	roleInfo RoleInfo,
	spec T, // spec of the role
) *BaseRoleReconciler[T] {
	return &BaseRoleReconciler[T]{
		BaseReconciler: BaseReconciler[T]{
			Client: client,
			Spec:   spec,
		},
		ClusterStopped: clusterStopped,
		ClusterInfo:    roleInfo.ClusterInfo,
		RoleInfo:       roleInfo,
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

func (r *BaseRoleReconciler[T]) AddResource(resource Reconciler) {
	r.resources = append(r.resources, resource)
}

func (r *BaseRoleReconciler[T]) GetResources() []Reconciler {
	return r.resources
}

func (r *BaseRoleReconciler[T]) IsStopped() bool {
	return r.ClusterStopped
}

func (r *BaseRoleReconciler[T]) Reconcile(ctx context.Context) (ctrl.Result, error) {
	logger.V(5).Info("Reconciling role", "namespace", r.GetNamespace(), "cluster", r.ClusterInfo.GetClusterName(), "role", r.GetName())
	// add pdb resource for the role
	if pdbReconciler, err := r.getPdbReconciler(ctx); err == nil && pdbReconciler != nil {
		r.resources = append(r.resources, pdbReconciler)
	} else {
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	for _, resource := range r.resources {
		if res, err := resource.Reconcile(ctx); !res.IsZero() || err != nil {
			return res, err
		}
	}
	logger.V(5).Info("Reconciled role", "namespace", r.GetNamespace(), "cluster", r.ClusterInfo.GetClusterName(), "role", r.GetName())
	return ctrl.Result{}, nil
}

// reconcile pdb resource for the role
func (r *BaseRoleReconciler[T]) getPdbReconciler(_ context.Context) (Reconciler, error) {
	logger.V(5).Info("get pdb for role", "namespace", r.GetNamespace(), "cluster", r.ClusterInfo.GetClusterName(), "role", r.GetName())
	value := reflect.ValueOf(r.Spec)
	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}
	RoleConfigReflect := value.FieldByName("RoleConfig")
	// check if the RoleConfig field exists
	if !RoleConfigReflect.IsValid() {
		logger.V(5).Info("RoleConfig field does not exist, skipping pdb reconciliation")
		return nil, nil
	}
	// transform the RoleConfigReflect to RoleConfigSpec
	RoleConfigSpec := RoleConfigReflect.Interface().(*commonsv1alpha1.RoleConfigSpec)
	if RoleConfigSpec == nil {
		logger.V(5).Info("RoleConfig field is nil, skipping pdb reconciliation")
		return nil, nil
	}
	pdb := RoleConfigSpec.PodDisruptionBudget
	// check if the PodDisruptionBudget field exists
	if pdb == nil {
		logger.V(5).Info("PDB field does not exist, skipping pdb reconciliation")
		return nil, nil
	}
	// check if the pdb is enabled
	if !pdb.Enabled {
		logger.V(5).Info("PDB is disabled, skipping pdb reconciliation")
		return nil, nil
	}
	option := func(opt *builder.PDBBuilderOptions) {
		opt.Labels = r.RoleInfo.labels
		opt.Annotations = r.RoleInfo.annotations
		opt.MaxUnavailableAmount = pdb.MaxUnavailable
	}
	logger.V(5).Info("get pdb success", "namespace", r.GetNamespace(), "cluster", r.ClusterInfo.GetClusterName(), "role", r.GetName(),
		"maxUnavailable", pdb.MaxUnavailable)
	return NewPDBReconciler(r.Client, r.GetFullName(), option)
}

func (r *BaseRoleReconciler[T]) Ready(ctx context.Context) (ctrl.Result, error) {
	logger.V(5).Info("Checking readiness of role", "namespace", r.GetNamespace(), "cluster", r.ClusterInfo.GetClusterName(), "role", r.GetName())
	for _, resource := range r.resources {
		if res, err := resource.Ready(ctx); !res.IsZero() || err != nil {
			return res, err
		}
	}
	logger.V(5).Info("Role is ready", "namespace", r.GetNamespace(), "cluster", r.ClusterInfo.GetClusterName(), "role", r.GetName())
	return ctrl.Result{}, nil
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
//		CliOverrides:  cliOverrides,
//	}
//
//	result := &RoleGroupSpec{
//		Replicas:          1,
//		Config:            config,
//		EnvOverrides:      envOverridesRoleGroup,   // `EnvOverrides` exists in left, so it is not replaced
//		CliOverrides:  cliOverrides,        // Add RoleSpec.CliOverrides to left
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
