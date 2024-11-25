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

// This struct is used to configure:
//  1. If PodDisruptionBudgets are created by the operator
//  2. The allowed number of Pods to be unavailable (`maxUnavailable`)
type PodDisruptionBudgetSpec struct {
	// +kubebuilder:validation:Optional
	// MinAvailable *int32 `json:"minAvailable,omitempty"`

	// Whether a PodDisruptionBudget should be written out for this role.
	// Disabling this enables you to specify your own - custom - one.
	// Defaults to true.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`

	// The number of Pods that are allowed to be down because of voluntary disruptions.
	// If you don't explicitly set this, the operator will use a sane default based
	// upon knowledge about the individual product.
	// +kubebuilder:validation:Optional
	MaxUnavailable *int32 `json:"maxUnavailable,omitempty"`
}
