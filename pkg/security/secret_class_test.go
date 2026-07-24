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
