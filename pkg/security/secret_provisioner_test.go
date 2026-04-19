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

package security

import (
	"fmt"
	"testing"
	"time"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestTLSRegistration(t *testing.T) {
	prov := NewSecretProvisioner()
	prov.Register(TLS("server-tls", "my-secret-class").WithPassword("mypass"))

	vols := prov.Volumes()
	require.Len(t, vols, 1)

	vol := vols[0]
	assert.Equal(t, "server-tls", vol.Name)
	require.NotNil(t, vol.Ephemeral)
	require.NotNil(t, vol.Ephemeral.VolumeClaimTemplate)

	pvc := vol.Ephemeral.VolumeClaimTemplate
	annotations := pvc.Annotations
	assert.Equal(t, "my-secret-class", annotations[SecretClassAnnotation])
	assert.Equal(t, "pod,node", annotations[SecretClassScopeAnnotation])
	assert.Equal(t, "tls-p12", annotations[AnnotationSecretsFormat])
	assert.Equal(t, "mypass", annotations[AnnotationSecretsPKCS12Password])
	assert.Equal(t, "secrets.kubedoop.dev", *pvc.Spec.StorageClassName)
	assert.Equal(t, corev1.ReadWriteOnce, pvc.Spec.AccessModes[0])
	require.NotNil(t, pvc.Spec.VolumeMode)
	assert.Equal(t, corev1.PersistentVolumeFilesystem, *pvc.Spec.VolumeMode)
	assert.Equal(t, "10Mi", pvc.Spec.Resources.Requests.Storage().String())

	mounts := prov.VolumeMounts()
	require.Len(t, mounts, 1)
	assert.Equal(t, "server-tls", mounts[0].Name)
	assert.Equal(t, "/kubedoop/mount/server-tls", mounts[0].MountPath)
	assert.True(t, mounts[0].ReadOnly)
}

func TestTLSPEMRegistration(t *testing.T) {
	prov := NewSecretProvisioner()
	prov.Register(TLSPEMFormat("client-pem", "pem-class"))

	vols := prov.Volumes()
	require.Len(t, vols, 1)

	annotations := vols[0].Ephemeral.VolumeClaimTemplate.Annotations
	assert.Equal(t, "tls-pem", annotations[AnnotationSecretsFormat])
	// PEM format should not have PKCS12 password
	_, hasPassword := annotations[AnnotationSecretsPKCS12Password]
	assert.False(t, hasPassword, "PEM format should not have PKCS12 password annotation")
}

func TestKerberosVolumeRegistration(t *testing.T) {
	prov := NewSecretProvisioner()
	prov.Register(KerberosVolume("zk-keytab", "krb-class").
		WithKerberosServiceNames("zookeeper", "zk"))

	vols := prov.Volumes()
	require.Len(t, vols, 1)

	annotations := vols[0].Ephemeral.VolumeClaimTemplate.Annotations
	assert.Equal(t, "kerberos", annotations[AnnotationSecretsFormat])
	assert.Equal(t, "zookeeper,zk", annotations[AnnotationSecretsKerberosServiceNames])
	// Kerberos should not have PKCS12 password
	_, hasPassword := annotations[AnnotationSecretsPKCS12Password]
	assert.False(t, hasPassword, "Kerberos format should not have PKCS12 password")

	mounts := prov.VolumeMounts()
	require.Len(t, mounts, 1)
	assert.Equal(t, "/kubedoop/mount/zk-keytab", mounts[0].MountPath)
}

func TestCustomScope(t *testing.T) {
	prov := NewSecretProvisioner()
	prov.Register(TLS("broker-tls", "broker-class").
		WithScope("pod,node,service=zk-server"))

	vols := prov.Volumes()
	require.Len(t, vols, 1)

	annotations := vols[0].Ephemeral.VolumeClaimTemplate.Annotations
	assert.Equal(t, "pod,node,service=zk-server", annotations[SecretClassScopeAnnotation])
}

func TestCertRotationAnnotations(t *testing.T) {
	prov := NewSecretProvisioner()
	prov.Register(TLS("server-tls", "my-class").
		WithCertLifetime(168 * time.Hour).
		WithCertJitter(0.2).
		WithCertBuffer(8 * time.Hour))

	vols := prov.Volumes()
	require.Len(t, vols, 1)

	annotations := vols[0].Ephemeral.VolumeClaimTemplate.Annotations
	assert.Equal(t, "168h0m0s", annotations[AnnotationSecretsCertLifetime])
	assert.Equal(t, "0.2", annotations[AnnotationSecretsCertJitterFactor])
	assert.Equal(t, "8h0m0s", annotations[AnnotationSecretsCertRestartBuffer])
}

func TestAutoInject(t *testing.T) {
	prov := NewSecretProvisioner()
	prov.Register(
		TLS("server-tls", "server-class"),
		KerberosVolume("zk-keytab", "krb-class"),
	)

	stsBuilder := builder.NewStatefulSetBuilder("test-sts", "default")
	prov.AutoInject(stsBuilder)

	require.Len(t, stsBuilder.Volumes, 2)
	require.Len(t, stsBuilder.VolumeMounts, 2)

	assert.Equal(t, "server-tls", stsBuilder.Volumes[0].Name)
	assert.Equal(t, "zk-keytab", stsBuilder.Volumes[1].Name)
	assert.Equal(t, "/kubedoop/mount/server-tls", stsBuilder.VolumeMounts[0].MountPath)
	assert.Equal(t, "/kubedoop/mount/zk-keytab", stsBuilder.VolumeMounts[1].MountPath)
}

func TestPathAPI(t *testing.T) {
	prov := NewSecretProvisioner()
	prov.Register(
		TLS("server-tls", "server-class"),
		TLS("quorum-tls", "quorum-class"),
	)

	// Path() returns without trailing slash
	path, err := prov.Path("server-tls")
	assert.NoError(t, err)
	assert.Equal(t, "/kubedoop/mount/server-tls", path)
	assert.NotContains(t, path[len("/kubedoop/mount/"):], "/", "Path should have no trailing slash")

	path2, err := prov.Path("quorum-tls")
	assert.NoError(t, err)
	assert.Equal(t, "/kubedoop/mount/quorum-tls", path2)

	// Unregistered name returns error
	_, err = prov.Path("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")

	// MustPath returns correct path
	assert.Equal(t, "/kubedoop/mount/server-tls", prov.MustPath("server-tls"))

	// MustPath panics on unregistered name
	assert.Panics(t, func() {
		prov.MustPath("nonexistent")
	})
}

func TestDefaultValues(t *testing.T) {
	reg := TLS("test-vol", "test-class")

	assert.Equal(t, "pod,node", reg.scope)
	assert.Equal(t, "changeit", reg.password)
	assert.Equal(t, "10Mi", reg.storageSize)
	assert.Equal(t, TLSP12, reg.format)

	prov := NewSecretProvisioner()
	prov.Register(reg)

	vols := prov.Volumes()
	require.Len(t, vols, 1)

	pvc := vols[0].Ephemeral.VolumeClaimTemplate
	assert.Equal(t, "10Mi", pvc.Spec.Resources.Requests.Storage().String())

	mounts := prov.VolumeMounts()
	require.Len(t, mounts, 1)
	assert.True(t, mounts[0].ReadOnly, "Default mount should be ReadOnly=true")
}

func TestConfigIntegration(t *testing.T) {
	// Simulates the full ZK config generation flow
	prov := NewSecretProvisioner()
	prov.Register(
		TLS("server-tls", "server-class"),
		TLS("quorum-tls", "quorum-class"),
	)

	// Verify volumes and mounts are generated correctly
	vols := prov.Volumes()
	mounts := prov.VolumeMounts()
	assert.Len(t, vols, 2)
	assert.Len(t, mounts, 2)

	// Simulate config generation using Path() API
	// This mirrors how ZK operator would compose zoo.cfg properties
	keystorePath := fmt.Sprintf("%s/keystore.p12", prov.MustPath("server-tls"))
	truststorePath := fmt.Sprintf("%s/truststore.p12", prov.MustPath("server-tls"))
	quorumKeystorePath := fmt.Sprintf("%s/keystore.p12", prov.MustPath("quorum-tls"))

	assert.Equal(t, "/kubedoop/mount/server-tls/keystore.p12", keystorePath)
	assert.Equal(t, "/kubedoop/mount/server-tls/truststore.p12", truststorePath)
	assert.Equal(t, "/kubedoop/mount/quorum-tls/keystore.p12", quorumKeystorePath)

	// Verify no double slashes (trailing slash safety)
	assert.NotContains(t, keystorePath, "//", "Path composition should not produce double slashes")
	assert.NotContains(t, quorumKeystorePath, "//", "Path composition should not produce double slashes")
}
