package framework

import (
	"context"
	"encoding/json"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	kubernetesjson "sigs.k8s.io/json"
)

// AuthenticationClassProvider is the platform's provider union. Resolving a class
// selects exactly one branch; products explicitly accept the providers they can
// consume. Credential references are returned, never credential bytes.
type AuthenticationClassProvider struct {
	Static   *StaticAuthentication   `json:"static,omitempty"`
	OIDC     *OIDCAuthentication     `json:"oidc,omitempty"`
	TLS      *TLSAuthentication      `json:"tls,omitempty"`
	LDAP     *LDAPAuthentication     `json:"ldap,omitempty"`
	Kerberos *KerberosAuthentication `json:"kerberos,omitempty"`
}

type NamedSecret struct {
	Name string `json:"name"`
}
type StaticAuthentication struct {
	UserCredentialsSecret *NamedSecret `json:"userCredentialsSecret"`
}
type TLSAuthentication struct {
	ClientCertSecretClass string `json:"clientCertSecretClass,omitempty"`
}
type KerberosAuthentication struct {
	KerberosStorageClass string `json:"kerberosStorageClass,omitempty"`
}
type ProviderTLS struct {
	Verification *ProviderTLSVerification `json:"verification"`
}
type ProviderTLSVerification struct {
	None   *struct{}                   `json:"none,omitempty"`
	Server *ProviderServerVerification `json:"server,omitempty"`
}
type ProviderServerVerification struct {
	CACert *ProviderCA `json:"caCert"`
}
type ProviderCA struct {
	SecretClass string    `json:"secretClass,omitempty"`
	WebPKI      *struct{} `json:"webPki,omitempty"`
}
type OIDCAuthentication struct {
	Hostname       string       `json:"hostname"`
	Port           int32        `json:"port,omitempty"`
	PrincipalClaim string       `json:"principalClaim"`
	ProviderHint   string       `json:"providerHint"`
	RootPath       string       `json:"rootPath,omitempty"`
	Scopes         []string     `json:"scopes,omitempty"`
	TLS            *ProviderTLS `json:"tls,omitempty"`
}
type ProviderCredentials struct {
	SecretClass string `json:"secretClass"`
}
type LDAPAuthentication struct {
	BindCredentials *ProviderCredentials `json:"bindCredentials"`
	Hostname        string               `json:"hostname"`
	Port            int32                `json:"port,omitempty"`
	FieldNames      map[string]string    `json:"ldapFieldNames,omitempty"`
	SearchBase      string               `json:"searchBase,omitempty"`
	SearchFilter    string               `json:"searchFilter,omitempty"`
	TLS             *ProviderTLS         `json:"tls,omitempty"`
}

// ResolveAuthenticationClass performs one exact cluster-scoped read. The result
// is platform configuration, not an assertion that a provider or user is ready.
func ResolveAuthenticationClass(ctx context.Context, reader FactsReader, name string) (FactResult[AuthenticationClassProvider], error) {
	if len(validation.IsDNS1123Subdomain(name)) != 0 {
		return authenticationFailure(FactsInvalid, "InvalidAuthenticationReference"), nil
	}
	object := &unstructured.Unstructured{}
	object.SetGroupVersionKind(schema.GroupVersionKind{Group: "authentication.kubedoop.dev", Version: "v1alpha1", Kind: "AuthenticationClass"})
	if err := reader.Get(ctx, types.NamespacedName{Name: name}, object); err != nil {
		if apierrors.IsNotFound(err) {
			return authenticationFailure(FactsPending, "AuthenticationClassMissing"), nil
		}
		return FactResult[AuthenticationClassProvider]{}, err
	}
	if !object.GetDeletionTimestamp().IsZero() {
		return authenticationFailure(FactsPending, "AuthenticationClassDeleting"), nil
	}
	provider, found, err := unstructured.NestedMap(object.Object, "spec", "provider")
	if err != nil || !found || len(provider) != 1 {
		return authenticationFailure(FactsInvalid, "InvalidAuthenticationProvider"), nil
	}
	data, err := json.Marshal(provider)
	if err != nil {
		return FactResult[AuthenticationClassProvider]{}, fmt.Errorf("encode authentication provider: %w", err)
	}
	var value AuthenticationClassProvider
	strict, err := kubernetesjson.UnmarshalStrict(data, &value)
	if err != nil || len(strict) != 0 || !validAuthenticationProvider(value) {
		return authenticationFailure(FactsInvalid, "InvalidAuthenticationProvider"), nil
	}
	return FactResult[AuthenticationClassProvider]{Value: &value, Diagnostic: FactDiagnostic{
		State: FactsResolved, Reason: "AuthenticationClassResolved", Message: "Provider configuration resolved; authentication success is not observed"}}, nil
}

func authenticationFailure(state FactState, reason string) FactResult[AuthenticationClassProvider] {
	return FactResult[AuthenticationClassProvider]{Diagnostic: FactDiagnostic{State: state, Reason: reason,
		Message: "Referenced authentication class is unavailable or has an invalid provider declaration"}}
}

func validAuthenticationProvider(value AuthenticationClassProvider) bool {
	switch {
	case value.Static != nil:
		return value.Static.UserCredentialsSecret != nil && len(validation.IsDNS1123Subdomain(value.Static.UserCredentialsSecret.Name)) == 0
	case value.OIDC != nil:
		return value.OIDC.Hostname != "" && value.OIDC.PrincipalClaim != "" && value.OIDC.Port >= 0 && value.OIDC.Port <= 65535
	case value.LDAP != nil:
		return value.LDAP.Hostname != "" && value.LDAP.Port >= 0 && value.LDAP.Port <= 65535
	case value.TLS != nil, value.Kerberos != nil:
		return true
	default:
		return false
	}
}
