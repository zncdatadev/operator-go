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

	// Services scopes the credential to the named Services.
	//
	// A name may contain neither "," nor "=": the scopes are handed to the secret-operator as one
	// comma-delimited annotation of "key=value" entries, so a name carrying either character would
	// be parsed as additional scopes. "mysvc,node" would request a NODE-scoped certificate — one
	// covering the node's hostname and IP — that nobody reading the CR asked for or could see.
	// Rejecting it here means the user is told at `kubectl apply`, instead of quietly receiving a
	// broader credential.
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:Pattern=`^[^,=]+$`
	Services []string `json:"services,omitempty"`

	// ListenerVolumes scopes the credential to the named listener volumes.
	//
	// Same delimiter rule as Services: neither "," nor "=" may appear in a name.
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:Pattern=`^[^,=]+$`
	ListenerVolumes []string `json:"listenerVolumes,omitempty"`
}
