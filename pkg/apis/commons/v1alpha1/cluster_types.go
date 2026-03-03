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
	// RoleConfig contains role-level common configurations.
	// These configurations are inherited by all RoleGroups under this role.
	// +kubebuilder:validation:Optional
	RoleConfig *RoleConfigSpec `json:"roleConfig,omitempty"`

	// RoleGroups defines the role group configurations.
	// Each RoleGroup maps to a Kubernetes StatefulSet.
	// +kubebuilder:validation:Optional
	RoleGroups map[string]RoleGroupSpec `json:"roleGroups,omitempty"`

	// Overrides allows customization of configuration files, environment variables, and CLI arguments.
	// These overrides apply to all RoleGroups unless overridden at the RoleGroup level.
	// +kubebuilder:validation:Optional
	Overrides *OverridesSpec `json:"overrides,omitempty"`
}

// RoleGroupSpec defines the configuration for a role group.
// Each RoleGroup maps directly to a Kubernetes StatefulSet and its associated resources.
type RoleGroupSpec struct {
	// Replicas is the number of pod replicas for this role group.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	// +kubebuilder:validation:Optional
	Replicas *int32 `json:"replicas,omitempty"`

	// RoleGroupConfig contains role group level configurations.
	// These include resource limits, affinity, and logging settings.
	// +kubebuilder:validation:Optional
	RoleGroupConfig *RoleGroupConfigSpec `json:"roleGroupConfig,omitempty"`

	// Overrides allows customization at the role group level.
	// RoleGroup overrides take precedence over Role overrides.
	// +kubebuilder:validation:Optional
	Overrides *OverridesSpec `json:"overrides,omitempty"`
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

// GetOverrides returns the overrides specification, or nil if not set.
func (r *RoleSpec) GetOverrides() *OverridesSpec {
	if r.Overrides == nil {
		return &OverridesSpec{}
	}
	return r.Overrides
}

// GetOverrides returns the overrides specification, or nil if not set.
func (r *RoleGroupSpec) GetOverrides() *OverridesSpec {
	if r.Overrides == nil {
		return &OverridesSpec{}
	}
	return r.Overrides
}

// GetRoleConfig returns the role configuration, or an empty config if not set.
func (r *RoleSpec) GetRoleConfig() *RoleConfigSpec {
	if r.RoleConfig == nil {
		return &RoleConfigSpec{}
	}
	return r.RoleConfig
}

// GetRoleGroupConfig returns the role group configuration, or an empty config if not set.
func (r *RoleGroupSpec) GetRoleGroupConfig() *RoleGroupConfigSpec {
	if r.RoleGroupConfig == nil {
		return &RoleGroupConfigSpec{}
	}
	return r.RoleGroupConfig
}
