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
	corev1 "k8s.io/api/core/v1"
)

const (
	// CSIDriverName is the CSI driver name for secret-operator.
	CSIDriverName = "secrets.stackable.tech"
	// SecretClassAnnotation is the annotation key for SecretClass.
	SecretClassAnnotation = "secrets.stackable.tech/class"
	// SecretClassScopeAnnotation is the annotation key for SecretClass scope.
	SecretClassScopeAnnotation = "secrets.stackable.tech/scope"
	// SecretPodInfoAnnotation is the annotation key for pod info injection.
	SecretPodInfoAnnotation = "secrets.stackable.tech/pod-info"
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
