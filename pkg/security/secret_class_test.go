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
	"github.com/zncdatadev/operator-go/pkg/security"
)

var _ = Describe("SecretClassVolumeBuilder", func() {
	var builder *security.SecretClassVolumeBuilder

	BeforeEach(func() {
		builder = security.NewSecretClassVolumeBuilder("tls")
	})

	Describe("NewSecretClassVolumeBuilder", func() {
		It("should create a new builder with secret class name", func() {
			Expect(builder).NotTo(BeNil())
		})
	})

	Describe("WithScope", func() {
		It("should set the scope", func() {
			builder.WithScope("pod")
			volume := builder.BuildVolume("tls-volume")
			Expect(volume.VolumeSource.CSI).NotTo(BeNil())
			Expect(volume.VolumeSource.CSI.VolumeAttributes).To(HaveKey(security.SecretClassScopeAnnotation))
			Expect(volume.VolumeSource.CSI.VolumeAttributes[security.SecretClassScopeAnnotation]).To(Equal("pod"))
		})
	})

	Describe("WithReadOnly", func() {
		It("should set read-only flag", func() {
			builder.WithReadOnly(true)
			volume := builder.BuildVolume("tls-volume")
			Expect(volume.VolumeSource.CSI).NotTo(BeNil())
			Expect(*volume.VolumeSource.CSI.ReadOnly).To(BeTrue())
		})
	})

	Describe("BuildVolume", func() {
		It("should build a CSI volume with correct attributes", func() {
			volume := builder.BuildVolume("tls-volume")

			Expect(volume.Name).To(Equal("tls-volume"))
			Expect(volume.VolumeSource.CSI).NotTo(BeNil())
			Expect(volume.VolumeSource.CSI.Driver).To(Equal(security.CSIDriverName))
			Expect(volume.VolumeSource.CSI.VolumeAttributes).To(HaveKey(security.SecretClassAnnotation))
			Expect(volume.VolumeSource.CSI.VolumeAttributes[security.SecretClassAnnotation]).To(Equal("tls"))
			Expect(volume.VolumeSource.CSI.VolumeAttributes).To(HaveKey(security.SecretPodInfoAnnotation))
		})

		It("should include scope in volume attributes when set", func() {
			builder.WithScope("node")
			volume := builder.BuildVolume("tls-volume")

			Expect(volume.VolumeSource.CSI.VolumeAttributes[security.SecretClassScopeAnnotation]).To(Equal("node"))
		})
	})

	Describe("BuildVolumeMount", func() {
		It("should build a volume mount", func() {
			mount := builder.BuildVolumeMount("tls-volume", "/etc/tls")

			Expect(mount.Name).To(Equal("tls-volume"))
			Expect(mount.MountPath).To(Equal("/etc/tls"))
			Expect(mount.ReadOnly).To(BeTrue()) // Default is true
		})
	})
})

var _ = Describe("SecretVolumeBuilder", func() {
	var builder *security.SecretVolumeBuilder

	BeforeEach(func() {
		builder = security.NewSecretVolumeBuilder()
	})

	Describe("BuildTLSSecretVolume", func() {
		It("should build a TLS secret volume", func() {
			volume := builder.BuildTLSSecretVolume("tls-class", "tls-volume")

			Expect(volume.Name).To(Equal("tls-volume"))
			Expect(volume.VolumeSource.CSI).NotTo(BeNil())
			Expect(volume.VolumeSource.CSI.Driver).To(Equal(security.CSIDriverName))
			Expect(volume.VolumeSource.CSI.VolumeAttributes[security.SecretClassAnnotation]).To(Equal("tls-class"))
			Expect(volume.VolumeSource.CSI.VolumeAttributes[security.SecretClassScopeAnnotation]).To(Equal("pod"))
		})
	})

	Describe("BuildTLSSecretMount", func() {
		It("should build a TLS secret mount", func() {
			mount := builder.BuildTLSSecretMount("tls-volume")

			Expect(mount.Name).To(Equal("tls-volume"))
			Expect(mount.MountPath).To(Equal("/etc/ssl/certs"))
			Expect(mount.ReadOnly).To(BeTrue())
		})
	})

	Describe("BuildKerberosKeytabVolume", func() {
		It("should build a Kerberos keytab volume", func() {
			volume := builder.BuildKerberosKeytabVolume("kerberos-class", "kerberos-volume")

			Expect(volume.Name).To(Equal("kerberos-volume"))
			Expect(volume.VolumeSource.CSI).NotTo(BeNil())
			Expect(volume.VolumeSource.CSI.VolumeAttributes[security.SecretClassAnnotation]).To(Equal("kerberos-class"))
		})
	})

	Describe("BuildKerberosKeytabMount", func() {
		It("should build a Kerberos keytab mount", func() {
			mount := builder.BuildKerberosKeytabMount("kerberos-volume")

			Expect(mount.Name).To(Equal("kerberos-volume"))
			Expect(mount.MountPath).To(Equal("/etc/kerberos"))
			Expect(mount.ReadOnly).To(BeTrue())
		})
	})

	Describe("BuildK8sSecretVolume", func() {
		It("should build a K8s secret volume", func() {
			volume := builder.BuildK8sSecretVolume("my-secret", "secret-volume")

			Expect(volume.Name).To(Equal("secret-volume"))
			Expect(volume.VolumeSource.Secret).NotTo(BeNil())
			Expect(volume.VolumeSource.Secret.SecretName).To(Equal("my-secret"))
		})
	})

	Describe("BuildConfigMapVolume", func() {
		It("should build a ConfigMap volume", func() {
			volume := builder.BuildConfigMapVolume("my-config", "config-volume")

			Expect(volume.Name).To(Equal("config-volume"))
			Expect(volume.VolumeSource.ConfigMap).NotTo(BeNil())
			Expect(volume.VolumeSource.ConfigMap.Name).To(Equal("my-config"))
		})
	})
})
