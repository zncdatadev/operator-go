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

package common

import (
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// RoleInterface defines role-level operations.
// This interface is used by the SDK to interact with role configurations.
type RoleInterface interface {
	// GetRoleName returns the role name (e.g., "namenode", "datanode").
	GetRoleName() string

	// GetRoleSpec returns the role specification.
	GetRoleSpec() *v1alpha1.RoleSpec

	// GetRoleGroups returns all role group specifications.
	GetRoleGroups() map[string]v1alpha1.RoleGroupSpec

	// GetOverrides returns the role-level overrides.
	GetOverrides() *v1alpha1.OverridesSpec
}

// RoleInfo provides a default implementation of RoleInterface.
type RoleInfo struct {
	Name        string
	Spec        *v1alpha1.RoleSpec
	Annotations map[string]string
	Labels      map[string]string
}

// GetRoleName returns the role name.
func (r *RoleInfo) GetRoleName() string {
	return r.Name
}

// GetRoleSpec returns the role specification.
func (r *RoleInfo) GetRoleSpec() *v1alpha1.RoleSpec {
	if r.Spec == nil {
		return &v1alpha1.RoleSpec{}
	}
	return r.Spec
}

// GetRoleGroups returns all role group specifications.
func (r *RoleInfo) GetRoleGroups() map[string]v1alpha1.RoleGroupSpec {
	return r.GetRoleSpec().GetRoleGroups()
}

// GetOverrides returns the role-level overrides.
func (r *RoleInfo) GetOverrides() *v1alpha1.OverridesSpec {
	return r.GetRoleSpec().GetOverrides()
}

// RoleGroupInfo contains information about a role group.
type RoleGroupInfo struct {
	RoleName      string
	RoleGroupName string
	Spec          v1alpha1.RoleGroupSpec
	Annotations   map[string]string
	Labels        map[string]string
}

// GetName returns the role group name.
func (r *RoleGroupInfo) GetName() string {
	return r.RoleGroupName
}

// GetRoleName returns the parent role name.
func (r *RoleGroupInfo) GetRoleName() string {
	return r.RoleName
}

// GetSpec returns the role group specification.
func (r *RoleGroupInfo) GetSpec() v1alpha1.RoleGroupSpec {
	return r.Spec
}

// GetReplicas returns the number of replicas.
func (r *RoleGroupInfo) GetReplicas() int32 {
	return r.Spec.GetReplicas()
}

// GetOverrides returns the role group overrides.
func (r *RoleGroupInfo) GetOverrides() *v1alpha1.OverridesSpec {
	return r.Spec.GetOverrides()
}

// GetAffinity returns the affinity configuration.
func (r *RoleGroupInfo) GetAffinity() *corev1.Affinity {
	// Affinity is stored in RoleGroupConfigSpec as RawExtension
	// This will be parsed by the reconciler
	return nil
}

// GetResources returns the resource requirements.
func (r *RoleGroupInfo) GetResources() *v1alpha1.ResourcesSpec {
	config := r.Spec.GetConfig()
	if config == nil {
		return nil
	}
	return config.Resources
}

// GetLogging returns the logging configuration.
func (r *RoleGroupInfo) GetLogging() *v1alpha1.LoggingSpec {
	config := r.Spec.GetConfig()
	if config == nil {
		return nil
	}
	return config.Logging
}

// GetGracefulShutdownTimeout returns the graceful shutdown timeout.
func (r *RoleGroupInfo) GetGracefulShutdownTimeout() string {
	config := r.Spec.GetConfig()
	if config == nil {
		return "30s"
	}
	return config.GracefulShutdownTimeout
}
