/*
Copyright 2024 ZNCDataDev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
	// Get the name of the cluster
	GetClusterName() string
	// Get the name of the role
	GetRoleName() string
	// Get the full name of the role, formatted as `<clusterName>-<roleName>`
	GetFullName() string

	// GetResources returns the resources in the role
	GetResources() []Reconciler

	// AddResource adds a resource to the role
	AddResource(resource Reconciler)

	// RegisterResources registers resources with the role
	RegisterResources(ctx context.Context) error

	// Deprecated: Use IsStopped instead.
	// This method is marked deprecated in `v0.12.0` and will be removed in next release.
	IsStopped() bool
	ClusterStopped() bool
}

var _ RoleReconciler = &BaseRoleReconciler[AnySpec]{}

type BaseRoleReconciler[T AnySpec] struct {
	BaseReconciler[T]

	RoleInfo RoleInfo

	clusterStopped bool
	// Resources in the role, e.g. StatefulSet, Service, etc.
	resources []Reconciler
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
		clusterStopped: clusterStopped,
		RoleInfo:       roleInfo,
	}
}

// GetName returns the name of the role
func (r *BaseRoleReconciler[T]) GetName() string {
	return r.RoleInfo.GetRoleName()
}

func (r *BaseRoleReconciler[T]) GetClusterName() string {
	return r.RoleInfo.GetClusterName()
}

func (r *BaseRoleReconciler[T]) GetRoleName() string {
	return r.RoleInfo.GetRoleName()
}

func (r *BaseRoleReconciler[T]) GetFullName() string {
	return r.RoleInfo.GetFullName()
}

func (r *BaseRoleReconciler[T]) AddResource(resource Reconciler) {
	r.resources = append(r.resources, resource)
}

func (r *BaseRoleReconciler[T]) GetResources() []Reconciler {
	return r.resources
}

// Deprecated: Use IsStopped instead.
// This method is marked deprecated in `v0.12.0` and will be removed in next release.
func (r *BaseRoleReconciler[T]) IsStopped() bool {
	return r.clusterStopped
}

func (r *BaseRoleReconciler[T]) ClusterStopped() bool {
	return r.clusterStopped
}

func (r *BaseRoleReconciler[T]) Reconcile(ctx context.Context) (ctrl.Result, error) {
	logger.V(1).Info("Reconciling role", "namespace", r.GetNamespace(), "cluster", r.GetClusterName(), "role", r.GetName())
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
	logger.V(1).Info("Reconciled role", "namespace", r.GetNamespace(), "cluster", r.GetClusterName(), "role", r.GetName())
	return ctrl.Result{}, nil
}

// reconcile pdb resource for the role
func (r *BaseRoleReconciler[T]) getPdbReconciler(_ context.Context) (Reconciler, error) {
	logger.V(5).Info("get role of pdb reconciler", "namespace", r.GetNamespace(), "cluster", r.GetClusterName(), "role", r.GetName())
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
		opt.Labels = r.RoleInfo.GetLabels()
		opt.Annotations = r.RoleInfo.GetAnnotations()
		opt.MaxUnavailableAmount = pdb.MaxUnavailable
	}
	logger.V(5).Info("got pdb config", "namespace", r.GetNamespace(), "cluster", r.GetClusterName(), "role", r.GetName(),
		"maxUnavailable", pdb.MaxUnavailable)
	return NewPDBReconciler(r.Client, r.GetFullName(), option)
}

func (r *BaseRoleReconciler[T]) Ready(ctx context.Context) (ctrl.Result, error) {
	logger.V(5).Info("Checking readiness of role", "namespace", r.GetNamespace(), "cluster", r.GetClusterName(), "role", r.GetName())
	for _, resource := range r.resources {
		if res, err := resource.Ready(ctx); !res.IsZero() || err != nil {
			return res, err
		}
	}
	logger.V(5).Info("Role is ready", "namespace", r.GetNamespace(), "cluster", r.GetClusterName(), "role", r.GetName())
	return ctrl.Result{}, nil
}

func (r *BaseRoleReconciler[T]) RegisterResources(ctx context.Context) error {
	panic("unimplemented")
}
