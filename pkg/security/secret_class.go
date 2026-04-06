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
	"github.com/zncdatadev/operator-go/pkg/constant"
	corev1 "k8s.io/api/core/v1"
)

// Secret constants for secret-operator CSI integration.
// All annotations and labels derive from KubedoopDomain for single source of truth.
const (
	SecretAPIGroup       = "secrets." + constant.KubedoopDomain
	secretAPIGroupPrefix = SecretAPIGroup + "/"

	// SecretStorageClass is the CSI storage class name for secret volumes.
	SecretStorageClass = SecretAPIGroup

	// CSI driver name for secret-operator.
	CSIDriverName = SecretAPIGroup

	// PVC template annotation keys consumed by the secret-operator CSI driver.
	SecretClassAnnotation      = secretAPIGroupPrefix + "class"
	SecretClassScopeAnnotation = secretAPIGroupPrefix + "scope"
	SecretPodInfoAnnotation    = secretAPIGroupPrefix + "pod-info"

	// Additional annotations for secret provisioning configuration.
	AnnotationSecretsFormat               = secretAPIGroupPrefix + "format"
	AnnotationSecretsPKCS12Password       = secretAPIGroupPrefix + "tlsPKCS12Password"
	AnnotationSecretsCertLifetime         = secretAPIGroupPrefix + "autoTlsCertLifetime"
	AnnotationSecretsCertJitterFactor     = secretAPIGroupPrefix + "autoTlsCertJitterFactor"
	AnnotationSecretsCertRestartBuffer    = secretAPIGroupPrefix + "autoTlsCertRestartBuffer"
	AnnotationSecretsKerberosServiceNames = secretAPIGroupPrefix + "kerberosServiceNames"

	// Labels for secret-operator to identify pods.
	LabelSecretsNode    = secretAPIGroupPrefix + "node"
	LabelSecretsPod     = secretAPIGroupPrefix + "pod"
	LabelSecretsService = secretAPIGroupPrefix + "service"

	// Delimiter constants.
	CommonDelimiter               = ","
	ListenerVolumeDelimiter       = CommonDelimiter
	KerberosServiceNamesDelimiter = CommonDelimiter
)

// SecretStorageClassPtr returns a pointer to the SecretStorageClass.
func SecretStorageClassPtr() *string {
	v := SecretStorageClass
	return &v
}

// SecretFormat defines the format of secrets provisioned by secret-operator.
type SecretFormat string

const (
	TLSPEM   SecretFormat = "tls-pem"
	TLSP12   SecretFormat = "tls-p12"
	Kerberos SecretFormat = "kerberos"
)

// SecretScope defines the scope of secrets provisioned by secret-operator.
type SecretScope string

const (
	PodScope            SecretScope = "pod"
	NodeScope           SecretScope = "node"
	ServiceScope        SecretScope = "service"
	ListenerVolumeScope SecretScope = "listener-volume"
)

// SecretClassVolumeBuilder builds CSI volumes for SecretClass.
type SecretClassVolumeBuilder struct {
	secretClassName string
	scope           string
	readOnly        bool
}

// NewSecretClassVolumeBuilder creates a new SecretClassVolumeBuilder.
func NewSecretClassVolumeBuilder(secretClassName string) *SecretClassVolumeBuilder {
	return &SecretClassVolumeBuilder{
		secretClassName: secretClassName,
		readOnly:        true,
	}
}

// WithScope sets the scope for the SecretClass (e.g., "pod", "node", "service").
func (b *SecretClassVolumeBuilder) WithScope(scope string) *SecretClassVolumeBuilder {
	b.scope = scope
	return b
}

// WithReadOnly sets whether the volume should be read-only.
func (b *SecretClassVolumeBuilder) WithReadOnly(readOnly bool) *SecretClassVolumeBuilder {
	b.readOnly = readOnly
	return b
}

// BuildVolume creates a CSI volume for the SecretClass.
func (b *SecretClassVolumeBuilder) BuildVolume(volumeName string) corev1.Volume {
	attributes := map[string]string{
		SecretClassAnnotation: b.secretClassName,
	}

	if b.scope != "" {
		attributes[SecretClassScopeAnnotation] = b.scope
	}

	// Add pod-info attribute to enable pod metadata injection
	attributes[SecretPodInfoAnnotation] = "true"

	return corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			CSI: &corev1.CSIVolumeSource{
				Driver:           CSIDriverName,
				ReadOnly:         &b.readOnly,
				VolumeAttributes: attributes,
			},
		},
	}
}

// BuildVolumeMount creates a volume mount for the SecretClass volume.
func (b *SecretClassVolumeBuilder) BuildVolumeMount(volumeName, mountPath string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      volumeName,
		MountPath: mountPath,
		ReadOnly:  b.readOnly,
	}
}

// SecretVolumeBuilder provides convenience methods for common secret types.
type SecretVolumeBuilder struct{}

// NewSecretVolumeBuilder creates a new SecretVolumeBuilder.
func NewSecretVolumeBuilder() *SecretVolumeBuilder {
	return &SecretVolumeBuilder{}
}

// BuildTLSSecretVolume creates a volume for TLS certificates from SecretClass.
func (b *SecretVolumeBuilder) BuildTLSSecretVolume(secretClassName, volumeName string) corev1.Volume {
	return NewSecretClassVolumeBuilder(secretClassName).
		WithScope("pod").
		BuildVolume(volumeName)
}

// BuildTLSSecretMount creates a mount for TLS certificates.
func (b *SecretVolumeBuilder) BuildTLSSecretMount(volumeName string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      volumeName,
		MountPath: "/etc/ssl/certs",
		ReadOnly:  true,
	}
}

// BuildKerberosKeytabVolume creates a volume for Kerberos keytab from SecretClass.
func (b *SecretVolumeBuilder) BuildKerberosKeytabVolume(secretClassName, volumeName string) corev1.Volume {
	return NewSecretClassVolumeBuilder(secretClassName).
		WithScope("pod").
		BuildVolume(volumeName)
}

// BuildKerberosKeytabMount creates a mount for Kerberos keytab.
func (b *SecretVolumeBuilder) BuildKerberosKeytabMount(volumeName string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      volumeName,
		MountPath: "/etc/kerberos",
		ReadOnly:  true,
	}
}

// BuildK8sSecretVolume creates a volume from a Kubernetes Secret.
func (b *SecretVolumeBuilder) BuildK8sSecretVolume(secretName, volumeName string) corev1.Volume {
	return corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: secretName,
			},
		},
	}
}

// BuildConfigMapVolume creates a volume from a ConfigMap.
func (b *SecretVolumeBuilder) BuildConfigMapVolume(configMapName, volumeName string) corev1.Volume {
	return corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMapName,
				},
			},
		},
	}
}
