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
	"fmt"
	"strings"
)

// ProviderKind names which authentication method an AuthenticationProvider selected.
type ProviderKind string

const (
	ProviderKindOIDC     ProviderKind = "oidc"
	ProviderKindTLS      ProviderKind = "tls"
	ProviderKindStatic   ProviderKind = "static"
	ProviderKindLDAP     ProviderKind = "ldap"
	ProviderKindKerberos ProviderKind = "kerberos"
)

// ResolveProvider reports which single authentication method a provider selected, and fails when
// the answer is not exactly one.
//
// It exists because the CEL rule on AuthenticationProvider cannot reach objects that already exist:
// it rejects new writes, but an AuthenticationClass stored before the rule shipped — or on a cluster
// whose CRD has not been upgraded — is read exactly as it was. This function gives every operator
// the same answer everywhere, which is the half that actually fixes the defect.
//
// The defect being fixed is divergence, not a missing feature. Five operators read this one object
// five different ways: first-match `if/else` chains whose remaining providers fall through
// silently, and one that concatenated an OIDC block and an LDAP block into the same generated
// config file, where both assigned the same key and the second silently won. Reading it through one
// function makes an ambiguous object an ERROR at the point of use rather than a coin toss.
//
// Zero providers is an error for the same reason and a sharper one: it used to fail OPEN. A product
// that treated "no provider" as "no authenticator" served its UI unauthenticated, and the only
// trace was a log line at V(5). "The user asked for authentication and named no method" is a
// mistake, not a request for none.
func ResolveProvider(provider *AuthenticationProvider) (ProviderKind, error) {
	if provider == nil {
		return "", fmt.Errorf("authentication provider is not set: exactly one of %s is required",
			strings.Join(providerFieldNames(), ", "))
	}

	var found []ProviderKind
	if provider.OIDC != nil {
		found = append(found, ProviderKindOIDC)
	}
	if provider.TLS != nil {
		found = append(found, ProviderKindTLS)
	}
	if provider.Static != nil {
		found = append(found, ProviderKindStatic)
	}
	if provider.LDAP != nil {
		found = append(found, ProviderKindLDAP)
	}
	if provider.Kerberos != nil {
		found = append(found, ProviderKindKerberos)
	}

	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return "", fmt.Errorf("authentication provider names no method: set exactly one of %s",
			strings.Join(providerFieldNames(), ", "))
	default:
		// Naming all of them matters: the object is usually shared, so whoever has to fix it is
		// rarely whoever wrote it, and the fix is to split it into one AuthenticationClass per
		// method rather than to pick a winner here.
		names := make([]string, 0, len(found))
		for _, kind := range found {
			names = append(names, string(kind))
		}
		return "", fmt.Errorf("authentication provider names %d methods (%s): exactly one is "+
			"allowed — declare one AuthenticationClass per method instead",
			len(found), strings.Join(names, ", "))
	}
}

// providerFieldNames lists the provider fields in declaration order, for error messages.
func providerFieldNames() []string {
	return []string{
		string(ProviderKindOIDC),
		string(ProviderKindTLS),
		string(ProviderKindStatic),
		string(ProviderKindLDAP),
		string(ProviderKindKerberos),
	}
}
