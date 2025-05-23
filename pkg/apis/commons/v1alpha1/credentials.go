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

type Credentials struct {

	// SecretClass scope
	// +kubebuilder:validation:Optional
	Scope *CredentialsScope `json:"scope,omitempty"`

	// +kubebuilder:validation:Required
	SecretClass string `json:"secretClass"`
}

type CredentialsScope struct {

	// +kubebuilder:validation:Optional
	Node bool `json:"node,omitempty"`

	// +kubebuilder:validation:Optional
	Pod bool `json:"pod,omitempty"`

	// +kubebuilder:validation:Optional
	Services []string `json:"services,omitempty"`

	// +kubebuilder:validation:Optional
	ListenerVolumes []string `json:"listenerVolumes,omitempty"`
}
