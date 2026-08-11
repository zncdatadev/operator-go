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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

// AuthenticationClassSpec defines the desired state of AuthenticationClass
type AuthenticationClassSpec struct {
	// +kubebuilder:validation:Required
	AuthenticationProvider *AuthenticationProvider `json:"provider"`
}

// AuthenticationProvider names EXACTLY ONE authentication method. A deployment offering several
// methods declares several AuthenticationClasses; it does not put several providers in one.
//
// The rule is enforced at admission by CEL (ExactlyOneOf desugars to x-kubernetes-validations)
// because nothing else can enforce it: AuthenticationClass has no controller in any repo, so this
// object is only ever read at build time by whichever product operator references it. Before the
// rule, the CRD accepted both zero providers and several, and the five operators that read it had
// five different readings — first-match with the remaining providers silently unhandled, or, in one
// case, BOTH an OIDC and an LDAP block concatenated into the same generated config file where the
// second assignment silently won. An empty provider was worse than ambiguous: it failed OPEN, with
// the product's authenticator skipped and its UI served unauthenticated, logged at V(5).
//
// The marker only stops NEW bad objects, and only once the regenerated CRD is applied. Use
// ResolveProvider to read one: it gives every operator the same answer on every cluster, including
// for objects stored before the rule existed.
//
// +kubebuilder:validation:ExactlyOneOf=oidc;tls;static;ldap;kerberos
type AuthenticationProvider struct {
	// +kubebuilder:validation:Optional
	OIDC *OIDCProvider `json:"oidc,omitempty"`

	// +kubebuilder:validation:Optional
	TLS *TLSProvider `json:"tls,omitempty"`

	// +kubebuilder:validation:Optional
	Static *StaticProvider `json:"static,omitempty"`

	// +kubebuilder:validation:Optional
	LDAP *LDAPProvider `json:"ldap,omitempty"`

	// +kubebuilder:validation:Optional
	Kerberos *KerberosProvider `json:"kerberos,omitempty"`
}

type KerberosProvider struct {
	// +kubebuilder:validation:Optional
	KerberosStorageClass string `json:"kerberosStorageClass,omitempty"`
}

type OIDCProvider struct {

	// +kubebuilder:validation:Required
	Hostname string `json:"hostname"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	Port int `json:"port,omitempty"`

	// +kubebuilder:validation:Required
	PrincipalClaim string `json:"principalClaim"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=keycloak
	ProviderHint string `json:"providerHint"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default="/"
	RootPath string `json:"rootPath,omitempty"`

	// +kubebuilder:validation:Optional
	Scopes []string `json:"scopes,omitempty"`

	// +kubebuilder:validation:Optional
	TLS *OIDCTls `json:"tls,omitempty"`
}

type OIDCTls struct {
	// +kubebuilder:validation:Required
	Verification *commonsv1alpha1.TLSVerificationSpec `json:"verification"`
}

type TLSProvider struct {
	// +kubebuilder:validation:Optional
	ClientCertSecretClass string `json:"clientCertSecretClass,omitempty"`
}

type StaticProvider struct {
	// +kubebuilder:validation:Required
	UserCredentialsSecret *StaticCredentialsSecret `json:"userCredentialsSecret"`
}

type StaticCredentialsSecret struct {
	// The secret name that contains the user credentials.
	// The data contained in secret is related to the data required for the specific product certification function.
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

type LDAPProvider struct {
	// Provide ldap credentials mounts for Pods via k8s-search secret-class.
	// The secret searched by k8s-search must contain the following data:
	//  - user: bind user, e.g. cn=admin,dc=example,dc=com
	//  - password: bind password
	BindCredentials *commonsv1alpha1.Credentials `json:"bindCredentials"`

	// +kubebuilder:validation:Required
	Hostname string `json:"hostname"`

	// LDAP server port. Default is 389, if tls default is 636.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	Port int `json:"port,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={"email": "mail", "givenName": "givenName", "group": "memberof", "surname": "sn", "uid": "uid"}
	LDAPFieldNames *LDAPFieldNames `json:"ldapFieldNames,omitempty"`

	// LDAP search base, for example: ou=users,dc=example,dc=com.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=""
	SearchBase string `json:"searchBase,omitempty"`

	// LDAP search filter, for example: (ou=teams,dc=example,dc=com).
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=""
	SearchFilter string `json:"searchFilter,omitempty"`

	// +kubebuilder:validation:Optional
	TLS *LDAPTLS `json:"tls,omitempty"`
}

type LDAPTLS struct {
	// +kubebuilder:validation:Required
	Verification *commonsv1alpha1.TLSVerificationSpec `json:"verification"`
}

type LDAPFieldNames struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=mail
	Email string `json:"email,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=givenName
	GivenName string `json:"givenName,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=memberof
	Group string `json:"group,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=sn
	Surname string `json:"surname,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=uid
	Uid string `json:"uid,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=authenticationclasses,scope=Cluster,shortName=authclass
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

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

// +kubebuilder:object:root=true

// AuthenticationClassList contains a list of AuthenticationClass
type AuthenticationClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AuthenticationClass `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AuthenticationClass{}, &AuthenticationClassList{})
}
