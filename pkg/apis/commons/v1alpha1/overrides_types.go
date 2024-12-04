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

type OverridesSpec struct {
	// +kubebuilder:validation:Optional
	CliOverrides []string `json:"cliOverrides,omitempty"`
	// +kubebuilder:validation:Optional
	EnvOverrides map[string]string `json:"envOverrides,omitempty"`
	// +kubebuilder:validation:Optional
	ConfigOverrides map[string]map[string]string `json:"configOverrides,omitempty"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=object
	PodOverrides *k8sruntime.RawExtension `json:"podOverrides,omitempty"`
}
