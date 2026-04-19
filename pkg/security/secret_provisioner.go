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
	"path"
	"strings"
	"time"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/constant"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecretVolumeRegistration declares a CSI secret volume need.
// Created via convenience constructors (TLS, Kerberos) or Custom builder.
type SecretVolumeRegistration struct {
	volumeName       string
	secretClass      string
	format           SecretFormat
	scope            string
	password         string
	storageSize      string
	certLifetime     *time.Duration
	certJitter       *float64
	certBuffer       *time.Duration
	kerberosSvcNames []string
	extraAnnotations map[string]string
}

// TLS creates a TLS PKCS12 secret volume registration with scope "pod,node".
//
// Note: the default PKCS12 password "changeit" is stored as a PVC template annotation
// and is therefore visible in etcd to anyone with get/pvc access.
// Use WithPassword() to set a custom password or WithNoPassword() to omit it.
func TLS(volumeName, secretClass string) *SecretVolumeRegistration {
	return &SecretVolumeRegistration{
		volumeName:  volumeName,
		secretClass: secretClass,
		format:      TLSP12,
		scope:       string(PodScope) + CommonDelimiter + string(NodeScope),
		password:    "changeit",
		storageSize: "10Mi",
	}
}

// TLSPEMFormat creates a TLS PEM secret volume registration with scope "pod,node".
func TLSPEMFormat(volumeName, secretClass string) *SecretVolumeRegistration {
	return &SecretVolumeRegistration{
		volumeName:  volumeName,
		secretClass: secretClass,
		format:      TLSPEM,
		scope:       string(PodScope) + CommonDelimiter + string(NodeScope),
		storageSize: "10Mi",
	}
}

// KerberosVolume creates a Kerberos keytab secret volume registration with scope "pod,node".
// serviceName is required and specifies the primary Kerberos service principal name.
// Additional service names can be provided via variadic arguments.
func KerberosVolume(volumeName, secretClass, serviceName string, additionalServiceNames ...string) *SecretVolumeRegistration {
	svcNames := append([]string{serviceName}, additionalServiceNames...)
	return &SecretVolumeRegistration{
		volumeName:       volumeName,
		secretClass:      secretClass,
		format:           Kerberos,
		scope:            string(PodScope) + CommonDelimiter + string(NodeScope),
		storageSize:      "10Mi",
		kerberosSvcNames: svcNames,
	}
}

// ServiceTLS creates a TLS PKCS12 secret volume registration with scope "pod,node,service=<serviceName>".
// This is used for service-scoped TLS certificates where each service instance gets a unique certificate.
//
// Note: the default PKCS12 password "changeit" is stored as a PVC template annotation.
// Use WithPassword() to set a custom password or WithNoPassword() to omit it.
func ServiceTLS(volumeName, secretClass, serviceName string) *SecretVolumeRegistration {
	return &SecretVolumeRegistration{
		volumeName:  volumeName,
		secretClass: secretClass,
		format:      TLSP12,
		scope:       string(PodScope) + CommonDelimiter + string(NodeScope) + CommonDelimiter + "service=" + serviceName,
		password:    "changeit",
		storageSize: "10Mi",
	}
}

// ListenerVolume creates a secret volume registration with "listener-volume" scope.
// This is used for listener-scoped secrets shared across service instances.
func ListenerVolume(volumeName, secretClass string, format SecretFormat) *SecretVolumeRegistration {
	return &SecretVolumeRegistration{
		volumeName:  volumeName,
		secretClass: secretClass,
		format:      format,
		scope:       string(ListenerVolumeScope),
		storageSize: "10Mi",
	}
}

// Custom creates a secret volume registration with an explicit format.
func Custom(volumeName, secretClass string, format SecretFormat) *SecretVolumeRegistration {
	return &SecretVolumeRegistration{
		volumeName:  volumeName,
		secretClass: secretClass,
		format:      format,
		scope:       string(PodScope) + CommonDelimiter + string(NodeScope),
		storageSize: "10Mi",
	}
}

// WithScope sets the CSI scope annotation value. Default is "pod,node".
func (r *SecretVolumeRegistration) WithScope(scope string) *SecretVolumeRegistration {
	r.scope = scope
	return r
}

// WithPassword sets the PKCS12 password. Default is "changeit" for TLS P12 format.
func (r *SecretVolumeRegistration) WithPassword(password string) *SecretVolumeRegistration {
	r.password = password
	return r
}

// WithNoPassword removes the PKCS12 password annotation from the volume.
func (r *SecretVolumeRegistration) WithNoPassword() *SecretVolumeRegistration {
	r.password = ""
	return r
}

// WithStorageSize sets the PVC storage request size. Default is "10Mi".
func (r *SecretVolumeRegistration) WithStorageSize(size string) *SecretVolumeRegistration {
	r.storageSize = size
	return r
}

// WithCertLifetime sets the certificate lifetime annotation for auto-rotation.
func (r *SecretVolumeRegistration) WithCertLifetime(lifetime time.Duration) *SecretVolumeRegistration {
	r.certLifetime = &lifetime
	return r
}

// WithCertJitter sets the certificate jitter factor annotation.
func (r *SecretVolumeRegistration) WithCertJitter(factor float64) *SecretVolumeRegistration {
	r.certJitter = &factor
	return r
}

// WithCertBuffer sets the certificate restart buffer annotation.
func (r *SecretVolumeRegistration) WithCertBuffer(buffer time.Duration) *SecretVolumeRegistration {
	r.certBuffer = &buffer
	return r
}

// WithKerberosServiceNames sets the Kerberos service names annotation.
func (r *SecretVolumeRegistration) WithKerberosServiceNames(names ...string) *SecretVolumeRegistration {
	r.kerberosSvcNames = names
	return r
}

// WithExtraAnnotation adds an extra annotation to the PVC template.
func (r *SecretVolumeRegistration) WithExtraAnnotation(key, value string) *SecretVolumeRegistration {
	if r.extraAnnotations == nil {
		r.extraAnnotations = make(map[string]string)
	}
	r.extraAnnotations[key] = value
	return r
}

// buildAnnotations constructs the PVC template annotations for this registration.
func (r *SecretVolumeRegistration) buildAnnotations() map[string]string {
	annotations := map[string]string{
		SecretClassAnnotation:      r.secretClass,
		SecretClassScopeAnnotation: r.scope,
		AnnotationSecretsFormat:    string(r.format),
	}

	// PKCS12 password for TLS formats
	if r.password != "" {
		annotations[AnnotationSecretsPKCS12Password] = r.password
	}

	// Certificate rotation annotations
	if r.certLifetime != nil {
		annotations[AnnotationSecretsCertLifetime] = r.certLifetime.String()
	}
	if r.certJitter != nil {
		annotations[AnnotationSecretsCertJitterFactor] = fmt.Sprintf("%v", *r.certJitter)
	}
	if r.certBuffer != nil {
		annotations[AnnotationSecretsCertRestartBuffer] = r.certBuffer.String()
	}

	// Kerberos service names
	if len(r.kerberosSvcNames) > 0 {
		annotations[AnnotationSecretsKerberosServiceNames] = strings.Join(r.kerberosSvcNames, KerberosServiceNamesDelimiter)
	}

	// Extra annotations
	for k, v := range r.extraAnnotations {
		annotations[k] = v
	}

	return annotations
}

// buildVolume constructs the EphemeralVolumeSource PVC volume for this registration.
func (r *SecretVolumeRegistration) buildVolume() corev1.Volume {
	return corev1.Volume{
		Name: r.volumeName,
		VolumeSource: corev1.VolumeSource{
			Ephemeral: &corev1.EphemeralVolumeSource{
				VolumeClaimTemplate: &corev1.PersistentVolumeClaimTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: r.buildAnnotations(),
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						StorageClassName: func() *string {
							v := string(SecretStorageClass)
							return &v
						}(),
						VolumeMode: func() *corev1.PersistentVolumeMode {
							v := corev1.PersistentVolumeFilesystem
							return &v
						}(),
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse(r.storageSize),
							},
						},
					},
				},
			},
		},
	}
}

// SecretProvisioner manages CSI secret volume declarations and provides
// integration methods for StatefulSet construction.
//
// Primary integration methods:
//   - Volumes() and VolumeMounts() for direct StatefulSet injection
//   - Path() and MustPath() for config generation
//
// Convenience method:
//   - AutoInject() for operators using StatefulSetBuilder
type SecretProvisioner struct {
	registrations []*SecretVolumeRegistration
	volumeNames   map[string]struct{}
	mountBasePath string
}

// NewSecretProvisioner creates a provisioner with the default mount base path.
func NewSecretProvisioner() *SecretProvisioner {
	return &SecretProvisioner{
		mountBasePath: constant.KubedoopMountDir,
		volumeNames:   make(map[string]struct{}),
	}
}

// WithMountBasePath overrides the default mount base path.
// Trailing slashes are stripped automatically.
func (p *SecretProvisioner) WithMountBasePath(basePath string) *SecretProvisioner {
	p.mountBasePath = strings.TrimRight(basePath, "/")
	return p
}

// Register adds secret volume declarations to the provisioner.
// Panics if a volume with the same name is already registered.
func (p *SecretProvisioner) Register(registrations ...*SecretVolumeRegistration) *SecretProvisioner {
	for _, reg := range registrations {
		if _, exists := p.volumeNames[reg.volumeName]; exists {
			panic(fmt.Sprintf("secret volume %q is already registered", reg.volumeName))
		}
		p.volumeNames[reg.volumeName] = struct{}{}
		p.registrations = append(p.registrations, reg)
	}
	return p
}

// Volumes returns all registered volumes for manual StatefulSet injection.
// This is the PRIMARY integration method for operators that construct
// StatefulSet directly (not via StatefulSetBuilder).
func (p *SecretProvisioner) Volumes() []corev1.Volume {
	volumes := make([]corev1.Volume, 0, len(p.registrations))
	for _, reg := range p.registrations {
		volumes = append(volumes, reg.buildVolume())
	}
	return volumes
}

// VolumeMounts returns all registered volume mounts for manual container injection.
// This is the PRIMARY integration method for operators that construct
// StatefulSet directly (not via StatefulSetBuilder).
// All mounts have ReadOnly set to true.
func (p *SecretProvisioner) VolumeMounts() []corev1.VolumeMount {
	mounts := make([]corev1.VolumeMount, 0, len(p.registrations))
	for _, reg := range p.registrations {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      reg.volumeName,
			MountPath: p.mountPath(reg.volumeName),
			ReadOnly:  true,
		})
	}
	return mounts
}

// Path returns the mount path for a registered volume (WITHOUT trailing slash).
// Returns an error if the volume name is not registered.
//
// Example:
//
//	provisioner.Path("server-tls") // => "/kubedoop/mount/server-tls"
//	fmt.Sprintf("%s/keystore.p12", path) // => "/kubedoop/mount/server-tls/keystore.p12"
func (p *SecretProvisioner) Path(volumeName string) (string, error) {
	for _, reg := range p.registrations {
		if reg.volumeName == volumeName {
			return p.mountPath(volumeName), nil
		}
	}
	return "", fmt.Errorf("secret volume %q not registered", volumeName)
}

// MustPath returns the mount path for a registered volume (WITHOUT trailing slash).
// Panics if the volume name is not registered.
//
// Example:
//
//	provisioner.MustPath("server-tls") // => "/kubedoop/mount/server-tls"
func (p *SecretProvisioner) MustPath(volumeName string) string {
	result, err := p.Path(volumeName)
	if err != nil {
		panic(err)
	}
	return result
}

// AutoInject adds all registered volumes and mounts to a StatefulSetBuilder.
// This is a CONVENIENCE method for operators that use StatefulSetBuilder.
// Operators constructing StatefulSet directly should use Volumes() and VolumeMounts().
func (p *SecretProvisioner) AutoInject(stsBuilder *builder.StatefulSetBuilder) {
	for _, vol := range p.Volumes() {
		stsBuilder.AddVolume(vol)
	}
	for _, mount := range p.VolumeMounts() {
		stsBuilder.AddVolumeMount(mount)
	}
}

// mountPath returns the full mount path for a volume name (no trailing slash).
func (p *SecretProvisioner) mountPath(volumeName string) string {
	return path.Join(p.mountBasePath, volumeName)
}
