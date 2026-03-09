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

package v1alpha1

import (
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

// GenericClusterSpec defines the common cluster configuration for all product operators.
// Product-specific specs should embed this struct to inherit common functionality.
type GenericClusterSpec struct {
	// ClusterOperation controls operator behavior at runtime.
	// Allows pausing reconciliation or stopping the cluster gracefully.
	// +kubebuilder:validation:Optional
	ClusterOperation *ClusterOperationSpec `json:"clusterOperation,omitempty"`

	// Roles defines the role configurations for the cluster.
	// Each role represents a logical functional component (e.g., NameNode, DataNode).
	// +kubebuilder:validation:Optional
	Roles map[string]RoleSpec `json:"roles,omitempty"`
}

// RoleSpec defines the configuration for a role within a cluster.
// A role acts as a template for its RoleGroups, defining shared configurations.
type RoleSpec struct {
	// RoleConfig contains Kubernetes-level role management controls.
	// These settings are role-scoped and NOT inherited or overridden by individual RoleGroups.
	// Examples: PodDisruptionBudget that covers all Pods across all RoleGroups.
	// +kubebuilder:validation:Optional
	RoleConfig *RoleConfigSpec `json:"roleConfig,omitempty"`

	// Config contains workload runtime configuration defaults for all RoleGroups.
	// Each RoleGroup inherits these values and can selectively override them.
	// Key distinction from 'roleConfig': this is workload behavior (resources, affinity, logging)
	// that propagates to RoleGroups, while roleConfig is Kubernetes resource management.
	// +kubebuilder:validation:Optional
	Config *RoleGroupConfigSpec `json:"config,omitempty"`

	// RoleGroups defines the role group configurations.
	// Each RoleGroup maps to a Kubernetes StatefulSet.
	// +kubebuilder:validation:Optional
	RoleGroups map[string]RoleGroupSpec `json:"roleGroups,omitempty"`

	// ConfigOverrides allows customization of configuration files (e.g., XML, properties).
	// Map[FileName]Map[Key]Value. These overrides apply to all RoleGroups unless overridden.
	// +kubebuilder:validation:Optional
	ConfigOverrides map[string]map[string]string `json:"configOverrides,omitempty"`

	// EnvOverrides allows customization of environment variables.
	// These overrides apply to all RoleGroups unless overridden.
	// +kubebuilder:validation:Optional
	EnvOverrides map[string]string `json:"envOverrides,omitempty"`

	// CliOverrides allows customization of CLI arguments.
	// These overrides apply to all RoleGroups unless overridden.
	// +kubebuilder:validation:Optional
	CliOverrides []string `json:"cliOverrides,omitempty"`

	// PodOverrides allows customization of Pod template using Strategic Merge Patch.
	// These overrides apply to all RoleGroups unless overridden.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=object
	PodOverrides *k8sruntime.RawExtension `json:"podOverrides,omitempty"`
}

// RoleGroupSpec defines the configuration for a role group.
// Each RoleGroup maps directly to a Kubernetes StatefulSet and its associated resources.
type RoleGroupSpec struct {
	// Replicas is the number of pod replicas for this role group.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	// +kubebuilder:validation:Optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Config contains role group level configurations.
	// These include resource limits, affinity, and logging settings.
	// +kubebuilder:validation:Optional
	Config *RoleGroupConfigSpec `json:"config,omitempty"`

	// ConfigOverrides allows customization of configuration files (e.g., XML, properties).
	// Map[FileName]Map[Key]Value. RoleGroup overrides take precedence over Role overrides.
	// +kubebuilder:validation:Optional
	ConfigOverrides map[string]map[string]string `json:"configOverrides,omitempty"`

	// EnvOverrides allows customization of environment variables.
	// RoleGroup overrides take precedence over Role overrides.
	// +kubebuilder:validation:Optional
	EnvOverrides map[string]string `json:"envOverrides,omitempty"`

	// CliOverrides allows customization of CLI arguments.
	// RoleGroup overrides take precedence over Role overrides.
	// +kubebuilder:validation:Optional
	CliOverrides []string `json:"cliOverrides,omitempty"`

	// PodOverrides allows customization of Pod template using Strategic Merge Patch.
	// RoleGroup overrides take precedence over Role overrides.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=object
	PodOverrides *k8sruntime.RawExtension `json:"podOverrides,omitempty"`
}

// GetReplicas returns the replica count, defaulting to 1 if not specified.
func (r *RoleGroupSpec) GetReplicas() int32 {
	if r.Replicas == nil {
		return 1
	}
	return *r.Replicas
}

// GetRoleGroups returns the map of role group names to their specifications.
func (r *RoleSpec) GetRoleGroups() map[string]RoleGroupSpec {
	if r.RoleGroups == nil {
		return make(map[string]RoleGroupSpec)
	}
	return r.RoleGroups
}

// GetOverrides returns the overrides specification built from flattened fields.
// Returns nil if no overrides are configured, avoiding unnecessary allocations.
// Note: This method creates a new OverridesSpec struct on each call, but only contains
// pointer references (not copies) to the underlying override maps. This is acceptable
// because it's called once per reconcile cycle per Role, not in hot paths.
func (r *RoleSpec) GetOverrides() *OverridesSpec {
	if r.ConfigOverrides == nil && r.EnvOverrides == nil && r.CliOverrides == nil && r.PodOverrides == nil {
		return nil
	}
	return &OverridesSpec{
		ConfigOverrides: r.ConfigOverrides,
		EnvOverrides:    r.EnvOverrides,
		CliOverrides:    r.CliOverrides,
		PodOverrides:    r.PodOverrides,
	}
}

// GetOverrides returns the overrides specification built from flattened fields.
// Returns nil if no overrides are configured, avoiding unnecessary allocations.
// See RoleSpec.GetOverrides for implementation details.
func (r *RoleGroupSpec) GetOverrides() *OverridesSpec {
	if r.ConfigOverrides == nil && r.EnvOverrides == nil && r.CliOverrides == nil && r.PodOverrides == nil {
		return nil
	}
	return &OverridesSpec{
		ConfigOverrides: r.ConfigOverrides,
		EnvOverrides:    r.EnvOverrides,
		CliOverrides:    r.CliOverrides,
		PodOverrides:    r.PodOverrides,
	}
}

// HasRoleConfig returns true if RoleConfig is set.
// Use this for nil-check semantics when needed.
func (r *RoleSpec) HasRoleConfig() bool {
	return r.RoleConfig != nil
}

// GetRoleConfig returns the Kubernetes-level role configuration.
// Returns an empty struct if not set, ensuring callers always get a valid reference.
// Use HasRoleConfig() to check if the configuration was explicitly set.
// This is NOT inherited by RoleGroups.
func (r *RoleSpec) GetRoleConfig() *RoleConfigSpec {
	if r.RoleConfig == nil {
		return &RoleConfigSpec{}
	}
	return r.RoleConfig
}

// GetConfig returns the workload runtime configuration defaults, or an empty config if not set.
// These values are inherited by RoleGroups.
func (r *RoleSpec) GetConfig() *RoleGroupConfigSpec {
	if r.Config == nil {
		return &RoleGroupConfigSpec{}
	}
	return r.Config
}

// GetConfig returns the role group configuration.
// Returns an empty struct if not set, ensuring callers always get a valid reference.
func (r *RoleGroupSpec) GetConfig() *RoleGroupConfigSpec {
	if r.Config == nil {
		return &RoleGroupConfigSpec{}
	}
	return r.Config
}
