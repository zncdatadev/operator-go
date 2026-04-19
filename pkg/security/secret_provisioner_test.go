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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/security"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("SecretProvisioner", func() {
	Describe("TLS Registration", func() {
		It("should build a TLS P12 volume with correct annotations", func() {
			prov := security.NewSecretProvisioner()
			prov.Register(security.TLS("server-tls", "my-secret-class").WithPassword("mypass"))

			vols := prov.Volumes()
			Expect(vols).To(HaveLen(1))

			vol := vols[0]
			Expect(vol.Name).To(Equal("server-tls"))
			Expect(vol.Ephemeral).NotTo(BeNil())
			Expect(vol.Ephemeral.VolumeClaimTemplate).NotTo(BeNil())

			pvc := vol.Ephemeral.VolumeClaimTemplate
			annotations := pvc.Annotations
			Expect(annotations[security.SecretClassAnnotation]).To(Equal("my-secret-class"))
			Expect(annotations[security.SecretClassScopeAnnotation]).To(Equal("pod,node"))
			Expect(annotations[security.AnnotationSecretsFormat]).To(Equal("tls-p12"))
			Expect(annotations[security.AnnotationSecretsPKCS12Password]).To(Equal("mypass"))
			Expect(*pvc.Spec.StorageClassName).To(Equal("secrets.kubedoop.dev"))
			Expect(pvc.Spec.AccessModes[0]).To(Equal(corev1.ReadWriteOnce))
			Expect(*pvc.Spec.VolumeMode).To(Equal(corev1.PersistentVolumeFilesystem))
			Expect(pvc.Spec.Resources.Requests.Storage().String()).To(Equal("10Mi"))

			mounts := prov.VolumeMounts()
			Expect(mounts).To(HaveLen(1))
			Expect(mounts[0].Name).To(Equal("server-tls"))
			Expect(mounts[0].MountPath).To(Equal("/kubedoop/mount/server-tls"))
			Expect(mounts[0].ReadOnly).To(BeTrue())
		})
	})

	Describe("TLS PEM Registration", func() {
		It("should build a TLS PEM volume without PKCS12 password", func() {
			prov := security.NewSecretProvisioner()
			prov.Register(security.TLSPEMFormat("client-pem", "pem-class"))

			vols := prov.Volumes()
			Expect(vols).To(HaveLen(1))

			annotations := vols[0].Ephemeral.VolumeClaimTemplate.Annotations
			Expect(annotations[security.AnnotationSecretsFormat]).To(Equal("tls-pem"))
			_, hasPassword := annotations[security.AnnotationSecretsPKCS12Password]
			Expect(hasPassword).To(BeFalse())
		})
	})

	Describe("Kerberos Volume Registration", func() {
		It("should build a Kerberos volume with service names", func() {
			prov := security.NewSecretProvisioner()
			prov.Register(security.KerberosVolume("zk-keytab", "krb-class", "zookeeper", "zk"))

			vols := prov.Volumes()
			Expect(vols).To(HaveLen(1))

			annotations := vols[0].Ephemeral.VolumeClaimTemplate.Annotations
			Expect(annotations[security.AnnotationSecretsFormat]).To(Equal("kerberos"))
			Expect(annotations[security.AnnotationSecretsKerberosServiceNames]).To(Equal("zookeeper,zk"))
			_, hasPassword := annotations[security.AnnotationSecretsPKCS12Password]
			Expect(hasPassword).To(BeFalse())

			mounts := prov.VolumeMounts()
			Expect(mounts).To(HaveLen(1))
			Expect(mounts[0].MountPath).To(Equal("/kubedoop/mount/zk-keytab"))
		})
	})

	Describe("Custom Scope", func() {
		It("should set custom scope annotation", func() {
			prov := security.NewSecretProvisioner()
			prov.Register(security.TLS("broker-tls", "broker-class").
				WithScope("pod,node,service=zk-server"))

			vols := prov.Volumes()
			Expect(vols).To(HaveLen(1))

			annotations := vols[0].Ephemeral.VolumeClaimTemplate.Annotations
			Expect(annotations[security.SecretClassScopeAnnotation]).To(Equal("pod,node,service=zk-server"))
		})
	})

	Describe("Cert Rotation Annotations", func() {
		It("should set lifetime, jitter, and buffer annotations", func() {
			prov := security.NewSecretProvisioner()
			prov.Register(security.TLS("server-tls", "my-class").
				WithCertLifetime(168 * time.Hour).
				WithCertJitter(0.2).
				WithCertBuffer(8 * time.Hour))

			vols := prov.Volumes()
			Expect(vols).To(HaveLen(1))

			annotations := vols[0].Ephemeral.VolumeClaimTemplate.Annotations
			Expect(annotations[security.AnnotationSecretsCertLifetime]).To(Equal("168h0m0s"))
			Expect(annotations[security.AnnotationSecretsCertJitterFactor]).To(Equal("0.2"))
			Expect(annotations[security.AnnotationSecretsCertRestartBuffer]).To(Equal("8h0m0s"))
		})
	})

	Describe("AutoInject", func() {
		It("should inject volumes and mounts into StatefulSetBuilder", func() {
			prov := security.NewSecretProvisioner()
			prov.Register(
				security.TLS("server-tls", "server-class"),
				security.KerberosVolume("zk-keytab", "krb-class", "zookeeper"),
			)

			stsBuilder := builder.NewStatefulSetBuilder("test-sts", "default")
			prov.AutoInject(stsBuilder)

			Expect(stsBuilder.Volumes).To(HaveLen(2))
			Expect(stsBuilder.VolumeMounts).To(HaveLen(2))
			Expect(stsBuilder.Volumes[0].Name).To(Equal("server-tls"))
			Expect(stsBuilder.Volumes[1].Name).To(Equal("zk-keytab"))
			Expect(stsBuilder.VolumeMounts[0].MountPath).To(Equal("/kubedoop/mount/server-tls"))
			Expect(stsBuilder.VolumeMounts[1].MountPath).To(Equal("/kubedoop/mount/zk-keytab"))
		})
	})

	Describe("Path API", func() {
		It("should return mount paths and handle errors", func() {
			prov := security.NewSecretProvisioner()
			prov.Register(
				security.TLS("server-tls", "server-class"),
				security.TLS("quorum-tls", "quorum-class"),
			)

			p, err := prov.Path("server-tls")
			Expect(err).NotTo(HaveOccurred())
			Expect(p).To(Equal("/kubedoop/mount/server-tls"))
			Expect(p[len("/kubedoop/mount/"):]).NotTo(ContainSubstring("/"))

			p2, err := prov.Path("quorum-tls")
			Expect(err).NotTo(HaveOccurred())
			Expect(p2).To(Equal("/kubedoop/mount/quorum-tls"))

			_, err = prov.Path("nonexistent")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not registered"))

			Expect(prov.MustPath("server-tls")).To(Equal("/kubedoop/mount/server-tls"))

			Expect(func() { prov.MustPath("nonexistent") }).To(Panic())
		})
	})

	Describe("Default Values", func() {
		It("should have correct TLS defaults through the public API", func() {
			prov := security.NewSecretProvisioner()
			prov.Register(security.TLS("test-vol", "test-class"))

			vols := prov.Volumes()
			Expect(vols).To(HaveLen(1))

			annotations := vols[0].Ephemeral.VolumeClaimTemplate.Annotations
			Expect(annotations[security.SecretClassScopeAnnotation]).To(Equal("pod,node"))
			Expect(annotations[security.AnnotationSecretsFormat]).To(Equal("tls-p12"))
			Expect(annotations[security.AnnotationSecretsPKCS12Password]).To(Equal("changeit"))

			pvc := vols[0].Ephemeral.VolumeClaimTemplate
			Expect(pvc.Spec.Resources.Requests.Storage().String()).To(Equal("10Mi"))

			mounts := prov.VolumeMounts()
			Expect(mounts).To(HaveLen(1))
			Expect(mounts[0].ReadOnly).To(BeTrue())
		})

		It("should omit password when WithNoPassword is called", func() {
			prov := security.NewSecretProvisioner()
			prov.Register(security.TLS("test-vol", "test-class").WithNoPassword())

			vols := prov.Volumes()
			Expect(vols).To(HaveLen(1))

			annotations := vols[0].Ephemeral.VolumeClaimTemplate.Annotations
			_, hasPassword := annotations[security.AnnotationSecretsPKCS12Password]
			Expect(hasPassword).To(BeFalse())
		})
	})

	Describe("Config Integration", func() {
		It("should compose file paths correctly without double slashes", func() {
			prov := security.NewSecretProvisioner()
			prov.Register(
				security.TLS("server-tls", "server-class"),
				security.TLS("quorum-tls", "quorum-class"),
			)

			vols := prov.Volumes()
			mounts := prov.VolumeMounts()
			Expect(vols).To(HaveLen(2))
			Expect(mounts).To(HaveLen(2))

			keystorePath := fmt.Sprintf("%s/keystore.p12", prov.MustPath("server-tls"))
			truststorePath := fmt.Sprintf("%s/truststore.p12", prov.MustPath("server-tls"))
			quorumKeystorePath := fmt.Sprintf("%s/keystore.p12", prov.MustPath("quorum-tls"))

			Expect(keystorePath).To(Equal("/kubedoop/mount/server-tls/keystore.p12"))
			Expect(truststorePath).To(Equal("/kubedoop/mount/server-tls/truststore.p12"))
			Expect(quorumKeystorePath).To(Equal("/kubedoop/mount/quorum-tls/keystore.p12"))

			Expect(keystorePath).NotTo(ContainSubstring("//"))
			Expect(quorumKeystorePath).NotTo(ContainSubstring("//"))
		})
	})

	Describe("Duplicate Registration", func() {
		It("should panic on duplicate volume name", func() {
			prov := security.NewSecretProvisioner()
			prov.Register(security.TLS("tls", "class"))
			Expect(func() {
				prov.Register(security.TLS("tls", "other-class"))
			}).To(Panic())
		})
	})

	Describe("WithMountBasePath", func() {
		It("should handle trailing slashes in custom base path", func() {
			prov := security.NewSecretProvisioner().WithMountBasePath("/custom/mount/")
			prov.Register(security.TLS("my-tls", "my-class"))

			mounts := prov.VolumeMounts()
			Expect(mounts).To(HaveLen(1))
			Expect(mounts[0].MountPath).To(Equal("/custom/mount/my-tls"))
			Expect(mounts[0].MountPath).NotTo(ContainSubstring("//"))
		})
	})

	Describe("ServiceTLS", func() {
		It("should build a TLS P12 volume with service scope", func() {
			prov := security.NewSecretProvisioner()
			prov.Register(security.ServiceTLS("broker-tls", "tls-class", "zk-server"))

			vols := prov.Volumes()
			Expect(vols).To(HaveLen(1))

			annotations := vols[0].Ephemeral.VolumeClaimTemplate.Annotations
			Expect(annotations[security.SecretClassAnnotation]).To(Equal("tls-class"))
			Expect(annotations[security.SecretClassScopeAnnotation]).To(Equal("pod,node,service=zk-server"))
			Expect(annotations[security.AnnotationSecretsFormat]).To(Equal("tls-p12"))
			Expect(annotations[security.AnnotationSecretsPKCS12Password]).To(Equal("changeit"))

			mounts := prov.VolumeMounts()
			Expect(mounts).To(HaveLen(1))
			Expect(mounts[0].MountPath).To(Equal("/kubedoop/mount/broker-tls"))
		})
	})

	Describe("ListenerVolume", func() {
		It("should build a volume with listener-volume scope", func() {
			prov := security.NewSecretProvisioner()
			prov.Register(security.ListenerVolume("listener-vol", "tls-class", security.Kerberos))

			vols := prov.Volumes()
			Expect(vols).To(HaveLen(1))

			annotations := vols[0].Ephemeral.VolumeClaimTemplate.Annotations
			Expect(annotations[security.SecretClassScopeAnnotation]).To(Equal("listener-volume"))
			Expect(annotations[security.AnnotationSecretsFormat]).To(Equal("kerberos"))

			mounts := prov.VolumeMounts()
			Expect(mounts).To(HaveLen(1))
			Expect(mounts[0].MountPath).To(Equal("/kubedoop/mount/listener-vol"))
		})
	})
})
