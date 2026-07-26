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

import k8sruntime "k8s.io/apimachinery/pkg/runtime"

type RoleConfigSpec struct {
	// +kubebuilder:validation:Optional
	PodDisruptionBudget *PodDisruptionBudgetSpec `json:"podDisruptionBudget,omitempty"`
}

type RoleGroupConfigSpec struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=object
	Affinity *k8sruntime.RawExtension `json:"affinity,omitempty"`
	// GracefulShutdownTimeout maps to the pod's terminationGracePeriodSeconds. Unset means
	// DefaultGracefulShutdownTimeout.
	//
	// It is a pointer and carries no `+kubebuilder:default` on purpose. Structural defaulting
	// fills a field as soon as its enclosing object exists, so with a CRD default every role group
	// that declared a config block for ANY reason — just `resources`, say — was stamped with "30s",
	// which is then indistinguishable from an explicit group value and wins the merge over the
	// role's setting. A role-level graceful shutdown could therefore only ever reach groups with no
	// config block at all.
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern="^([0-9]+(\\.[0-9]+)?(ns|us|ms|s|m|h))+$"
	GracefulShutdownTimeout *string `json:"gracefulShutdownTimeout,omitempty"`
	// +kubebuilder:validation:Optional
	Logging *LoggingSpec `json:"logging,omitempty"`
	// +kubebuilder:validation:Optional
	Resources *ResourcesSpec `json:"resources,omitempty"`
}

// DefaultGracefulShutdownTimeout is the termination grace period the framework applies when
// neither the role nor the role group sets one. Applied at consumption time, not as a CRD default
// — see the field doc above.
const DefaultGracefulShutdownTimeout = "30s"

// GetGracefulShutdownTimeout returns the configured timeout, or DefaultGracefulShutdownTimeout
// when unset.
func (c *RoleGroupConfigSpec) GetGracefulShutdownTimeout() string {
	if c == nil || c.GracefulShutdownTimeout == nil {
		return DefaultGracefulShutdownTimeout
	}
	return *c.GracefulShutdownTimeout
}
