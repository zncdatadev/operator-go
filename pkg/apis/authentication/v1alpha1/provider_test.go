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

package v1alpha1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/apis/authentication/v1alpha1"
)

var _ = Describe("ResolveProvider", func() {
	It("names the single method that is set", func() {
		kind, err := v1alpha1.ResolveProvider(&v1alpha1.AuthenticationProvider{
			OIDC: &v1alpha1.OIDCProvider{},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(kind).To(Equal(v1alpha1.ProviderKindOIDC))

		kind, err = v1alpha1.ResolveProvider(&v1alpha1.AuthenticationProvider{
			Kerberos: &v1alpha1.KerberosProvider{},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(kind).To(Equal(v1alpha1.ProviderKindKerberos))
	})

	It("refuses an empty provider rather than failing open", func() {
		// The sharp case. `provider: {}` passes the CRD's Required check on the field itself, and a
		// product treating "no provider" as "no authenticator" served its UI unauthenticated with
		// nothing but a V(5) log line to say so.
		_, err := v1alpha1.ResolveProvider(&v1alpha1.AuthenticationProvider{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("names no method"))
		// The message lists what to set, because the reader is usually not the author.
		Expect(err.Error()).To(ContainSubstring("oidc"))
		Expect(err.Error()).To(ContainSubstring("kerberos"))
	})

	It("refuses two methods instead of picking one", func() {
		// Five operators picked differently — first-match by declaration order, or, in one case,
		// emitting BOTH config blocks so the later assignment silently won. An error at the point of
		// use is the only answer that is the same everywhere.
		_, err := v1alpha1.ResolveProvider(&v1alpha1.AuthenticationProvider{
			OIDC: &v1alpha1.OIDCProvider{},
			LDAP: &v1alpha1.LDAPProvider{},
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("names 2 methods"))
		Expect(err.Error()).To(ContainSubstring("oidc"))
		Expect(err.Error()).To(ContainSubstring("ldap"))
		// The fix is to split the object, not to choose a winner.
		Expect(err.Error()).To(ContainSubstring("one AuthenticationClass per method"))
	})

	It("treats a nil provider as unset rather than panicking", func() {
		// AuthenticationClassSpec.AuthenticationProvider is a pointer, so a CR stored before the
		// field was Required — or one built in Go — reaches this as nil.
		_, err := v1alpha1.ResolveProvider(nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not set"))
	})
})
