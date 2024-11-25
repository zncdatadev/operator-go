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

// TLSPrivider defines the TLS provider for authentication.
// You can specify the none or server or mutual verification.
type TLSVerificationSpec struct {

	// +kubebuilder:validation:Optional
	None *NoneVerification `json:"none,omitempty"`

	// +kubebuilder:validation:Optional
	Server *ServerVerification `json:"server,omitempty"`

	// +kubebuilder:validation:Optional
	Mutual *MutualVerification `json:"mutual,omitempty"`
}

type MutualVerification struct {
	// +kubebuilder:validation:Required
	CertSecretClass string `json:"certSecretClass"`
}

type NoneVerification struct {
}

type ServerVerification struct {
	// +kubebuilder:validation:Required
	CACert *CACert `json:"caCert"`
}

// CACert is the CA certificate for server verification.
// You can specify the secret class or the webPki.
type CACert struct {
	// +kubebuilder:validation:Optional
	SecretClass string `json:"secretClass,omitempty"`

	// +kubebuilder:validation:Optional
	WebPki *WebPki `json:"webPki,omitempty"`
}

type WebPki struct {
}
