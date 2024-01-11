/*
Copyright 2024 zncdata-labs.

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

type ResponseType string

const (
	ResponseTypeCode  ResponseType = "code"
	ResponseTypeToken ResponseType = "id_token"
)

type AuthenticationClass struct {
	// +kubebuilder:validation:Required
	AuthenticationProvider string `json:"provider,omitempty"`
}

type AuthenticationProvider struct {
	// +kubebuilder:validation:Optional
	OIDC *OIDCProvider `json:"oidc,omitempty"`
}

type OIDCProvider struct {
	Issuer   string `json:"issuer,omitempty"`
	ClientId string `json:"clientId,omitempty"`
	// +kubebuilder:validation:Optional
	ClientSecret string `json:"clientSecret,omitempty"`
	// +kubebuilder:validation:Optional
	ResponseType []ResponseType `json:"responseType,omitempty"`
	// +kubebuilder:validation:Optional
	RedirectURL string `json:"redirectUrl,omitempty"`
}
