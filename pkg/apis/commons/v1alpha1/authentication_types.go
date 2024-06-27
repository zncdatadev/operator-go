/*
Copyright 2024 zncdatadev.

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

// Package v1alpha1 contains API Schema definitions for the v1alpha1 API group
// +kubebuilder:object:generate=true
// +groupName=zncdata.dev

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// GroupVersion is group version used to register these objects
	AuthenticationGroupVersion = schema.GroupVersion{Group: "authentication.zncdata.dev", Version: "v1alpha1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	AuthenticationSchemeBuilder = &scheme.Builder{GroupVersion: AuthenticationGroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AuthenticationAddToScheme = AuthenticationSchemeBuilder.AddToScheme
)

type ResponseType string

const (
	ResponseTypeCode  ResponseType = "code"
	ResponseTypeToken ResponseType = "id_token"
)

type AuthenticationClassSpec struct {
	// +kubebuilder:validation:Required
	AuthenticationProvider string `json:"provider,omitempty"`
}

type AuthenticationProvider struct {
	// +kubebuilder:validation:Optional
	OIDC *OIDCProvider `json:"oidc,omitempty"`

	// +kubebuilder:validation:Optional
	TLS *TLSPrivider `json:"tls,omitempty"`

	// +kubebuilder:validation:Optional
	Static *StaticProvider `json:"static,omitempty"`

	// +kubebuilder:validation:Optional
	LDAP *LDAPProvider `json:"ldap,omitempty"`
}

type OIDCProvider struct {

	// +kubebuilder:validation:Required
	Hostname string `json:"hostname"`

	// +kubebuilder:validation:Optional
	Port int `json:"port,omitempty"`

	PrincipalClaim string `json:"principalClaim"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=oidc;keycloak;dexidp;authentik
	ProviderHint string `json:"providerHint"`

	// +kubebuilder:validation:Optional
	RootPath string `json:"rootPath,omitempty"`

	// +kubebuilder:validation:Optional
	Scopes []string `json:"scopes,omitempty"`

	// +kubebuilder:validation:Optional
	Tls *Tls `json:"tls,omitempty"`
}

type TLSPrivider struct {
	// +kubebuilder:validation:Optional
	SecretClass string `json:"secretClass,omitempty"`
}

type StaticProvider struct {
	CerdentialSecret string `json:"credential"`
}

type LDAPProvider struct {
	// +kubebuilder:validation:Required
	Credential *LDAPCredential `json:"credential"`

	// +kubebuilder:validation:Required
	Hostname string `json:"hostname"`

	// +kubebuilder:validation:Optional
	Port int `json:"port,omitempty"`

	// +kubebuilder:validation:Optional
	LDAPFieldNames *LDAPFieldNames `json:"ldapFieldNames,omitempty"`

	// LDAP search base, for example: ou=users,dc=example,dc=org.
	// +kubebuilder:validation:Optional
	SearchBase string `json:"searchBase,omitempty"`

	// LDAP search filter, for example: (uid=%s).
	// +kubebuilder:validation:Optional
	SearchFilter string `json:"searchFilter,omitempty"`

	// +kubebuilder:validation:Optional
	Tls *Tls `json:"tls,omitempty"`
}

type LDAPCredential struct {
	// +kubebuilder:validation:Optional
	Scopes *CrendentialScope `json:"scopes,omitempty"`

	// +kubebuilder:validation:Required
	SecretClass string `json:"secretClass"`
}

type CrendentialScope struct {
	// +kubebuilder:validation:Optional
	Node string `json:"node,omitempty"`

	// +kubebuilder:validation:Optional
	Pod string `json:"pod,omitempty"`

	// +kubebuilder:validation:Optional
	Services []string `json:"services,omitempty"`
}

type LDAPFieldNames struct {
	// +kubebuilder:validation:Optional
	Email string `json:"email,omitempty"`

	// +kubebuilder:validation:Optional
	GivenName string `json:"givenName,omitempty"`

	// +kubebuilder:validation:Optional
	Group string `json:"group,omitempty"`

	// +kubebuilder:validation:Optional
	Surname string `json:"surname,omitempty"`

	// +kubebuilder:validation:Optional
	Uid string `json:"uid,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// AuthenticationClass is the Schema for the authenticationclasses API
type AuthenticationClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AuthenticationClassSpec   `json:"spec,omitempty"`
	Status AuthenticationClassStatus `json:"status,omitempty"`
}

// AuthenticationClassStatus defines the observed state of AuthenticationClass
type AuthenticationClassStatus struct {
}

//+kubebuilder:object:root=true

// AuthenticationClassList contains a list of AuthenticationClass
type AuthenticationClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AuthenticationClass `json:"items"`
}

func init() {
	AuthenticationSchemeBuilder.Register(&AuthenticationClass{}, &AuthenticationClassList{})
}
