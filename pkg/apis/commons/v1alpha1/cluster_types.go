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
	// Image specifies the container image configuration for the product workload.
	// If not set, the product operator webhook should provide defaults.
	// When Custom is set, it takes precedence over Repo/ProductVersion/KubedoopVersion.
	// +kubebuilder:validation:Optional
	Image *ImageSpec `json:"image,omitempty"`

	// ClusterOperation controls operator behavior at runtime.
	// Allows pausing reconciliation or stopping the cluster gracefully.
	// +kubebuilder:validation:Optional
	ClusterOperation *ClusterOperationSpec `json:"clusterOperation,omitempty"`

	// Roles defines the role configurations for the cluster.
	// Each role represents a logical functional component (e.g., NameNode, DataNode).
	//
	// A role name is not free-form: it becomes a segment of the name of every resource built for
	// the role ("<cluster>-<role>-<group>") and the value of the app.kubernetes.io/component
	// label. A name that is not a lowercase RFC 1123 label therefore produces resource names the
	// API server rejects — "Coordinator", "my_role" and "a.b" all fail on the StatefulSet's or the
	// Service's metadata.name. Without this rule that rejection surfaces halfway through a
	// reconcile, as a permanently Degraded role complaining about a field the user never wrote;
	// with it, `kubectl apply` says so immediately.
	//
	// MaxProperties is not a product constraint — no product has 64 distinct roles — but the CEL
	// cost estimator's only handle on a map. Without a declared bound it assumes the theoretical
	// worst case and rejects the rule below at CRD creation time with "estimated rule cost exceeds
	// budget by factor of more than 100x". The bound is set far above any real deployment so it
	// never binds in practice, and low enough to leave a product's own CRD room in the per-schema
	// budget it shares.
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxProperties=64
	// +kubebuilder:validation:XValidation:rule=`self.all(k, size(k) <= 63 && k.matches('^[a-z0-9]([-a-z0-9]*[a-z0-9])?$'))`,message=`each role name must be a lowercase RFC 1123 label (lowercase alphanumerics and '-', starting and ending with an alphanumeric, at most 63 characters): role names become part of the name and labels of every resource built for the role`
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
	//
	// Constrained for the same reason as the role name above: a role group name is a segment of
	// "<cluster>-<role>-<group>" and the value of the app.kubernetes.io/role-group label, so a name
	// that is not a lowercase RFC 1123 label yields resource names the API server refuses.
	//
	// MaxProperties bounds the CEL cost estimate, not the deployment — see the note on Roles.
	// 256 role groups in a single role is far past one per rack in a very large cluster.
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxProperties=256
	// +kubebuilder:validation:XValidation:rule=`self.all(k, size(k) <= 63 && k.matches('^[a-z0-9]([-a-z0-9]*[a-z0-9])?$'))`,message=`each role group name must be a lowercase RFC 1123 label (lowercase alphanumerics and '-', starting and ending with an alphanumeric, at most 63 characters): role group names become part of the name and labels of every resource built for the group`
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
