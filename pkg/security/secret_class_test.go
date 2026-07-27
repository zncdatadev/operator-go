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

package security_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/security"
)

var _ = Describe("ScopeString", func() {
	It("returns empty for a nil scope", func() {
		Expect(security.ScopeString(nil)).To(Equal(""))
	})

	It("returns empty for an empty scope", func() {
		Expect(security.ScopeString(&commonsv1alpha1.CredentialsScope{})).To(Equal(""))
	})

	It("renders node and pod scopes as bare entries", func() {
		scope := &commonsv1alpha1.CredentialsScope{Node: true, Pod: true}
		Expect(security.ScopeString(scope)).To(Equal("node,pod"))
	})

	It("renders services and listener volumes with the key=value convention", func() {
		scope := &commonsv1alpha1.CredentialsScope{
			Services:        []string{"minio", "gateway"},
			ListenerVolumes: []string{"listener"},
		}
		Expect(security.ScopeString(scope)).To(
			Equal("service=minio,service=gateway,listener-volume=listener"))
	})
})

var _ = Describe("CredentialsVolume", func() {
	It("provisions a volume with only the class annotation by default", func() {
		provisioner := security.NewSecretProvisioner().
			Register(security.CredentialsVolume("s3-credentials", "s3-credentials"))

		volumes := provisioner.Volumes()
		Expect(volumes).To(HaveLen(1))
		annotations := volumes[0].Ephemeral.VolumeClaimTemplate.Annotations
		Expect(annotations).To(HaveKeyWithValue(security.SecretClassAnnotation, "s3-credentials"))
		Expect(annotations).NotTo(HaveKey(security.AnnotationSecretsFormat))
		Expect(annotations).NotTo(HaveKey(security.SecretClassScopeAnnotation))
	})

	It("carries a scope when one is set", func() {
		provisioner := security.NewSecretProvisioner().
			Register(security.CredentialsVolume("creds", "class").WithScope("node,pod"))

		annotations := provisioner.Volumes()[0].Ephemeral.VolumeClaimTemplate.Annotations
		Expect(annotations).To(HaveKeyWithValue(security.SecretClassScopeAnnotation, "node,pod"))
	})
})

// Scope lists come from a CR, where an empty list item is legal input rather than a programmer
// error; a nameless "service=" entry is not resolvable and must not reach the annotation.
var _ = Describe("ScopeString with empty list entries", func() {
	It("skips empty service and listener-volume names", func() {
		Expect(security.ScopeString(&commonsv1alpha1.CredentialsScope{
			Pod:             true,
			Services:        []string{"", "trino"},
			ListenerVolumes: []string{""},
		})).To(Equal("pod,service=trino"))
	})
})

// A scope name carrying the annotation's own syntax does not escape — it adds scopes. This is the
// privilege half of the same problem the empty-entry specs above cover.
var _ = Describe("ScopeString with delimiter-bearing names", func() {
	It("drops a service name containing the delimiter instead of granting a second scope", func() {
		// "mysvc,node" would render "service=mysvc,node", which the secret-operator parses as a
		// service scope AND a node scope: the CR author silently receives a certificate covering
		// the node's hostname and IP, and a reviewer reading the CR sees nothing unusual.
		rendered := security.ScopeString(&commonsv1alpha1.CredentialsScope{
			Services: []string{"mysvc,node"},
		})

		Expect(rendered).NotTo(ContainSubstring("node"))
		Expect(rendered).To(BeEmpty())
	})

	It("drops a name containing the key separator", func() {
		// "a=b" would render "service=a=b"; the secret-operator's Cut on the first "=" yields the
		// value "a=b" for some parsers and "a" for others. Either way it is not what was asked for.
		Expect(security.ScopeString(&commonsv1alpha1.CredentialsScope{
			Services:        []string{"a=b"},
			ListenerVolumes: []string{"c=d"},
		})).To(BeEmpty())
	})

	It("keeps the well-formed entries alongside a dropped one", func() {
		// Dropping is per entry: one malformed item must not cost the user the scopes that are
		// fine, and the surviving scope is narrower than requested rather than wider.
		Expect(security.ScopeString(&commonsv1alpha1.CredentialsScope{
			Node:            true,
			Services:        []string{"good", "bad,node"},
			ListenerVolumes: []string{"lv-ok", "lv=bad"},
		})).To(Equal("node,service=good,listener-volume=lv-ok"))
	})

	It("never emits a scope the CR did not ask for", func() {
		// The property that matters, stated directly: whatever a user writes, the rendered
		// annotation contains no entry beyond the ones their CR named.
		for _, hostile := range []string{"x,node", "x,pod", "x=y", ",node", "node,"} {
			rendered := security.ScopeString(&commonsv1alpha1.CredentialsScope{
				Services: []string{hostile},
			})
			Expect(rendered).To(BeEmpty(), "input %q leaked a scope", hostile)
		}
	})
})
