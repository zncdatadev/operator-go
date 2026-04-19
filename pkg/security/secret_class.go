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
