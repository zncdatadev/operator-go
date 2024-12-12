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

package constants

const (
	SecretAPIGroup     string = "secrets." + KubedoopDomain
	SecretStorageClass string = SecretAPIGroup

	secretAPIGroupPrefix string = SecretAPIGroup + "/"
)

func SecretStorageClassPtr() *string {
	secretStorageClass := SecretStorageClass
	return &secretStorageClass
}

// Labels for k8s search secret
// k8s search secret obj by filter one or more labels
const (
	LabelSecretsNode    string = secretAPIGroupPrefix + "node"
	LabelSecretsPod     string = secretAPIGroupPrefix + "pod"
	LabelSecretsService string = secretAPIGroupPrefix + "service"
)

// Kubedoop defined annotations for PVCTemplate.
// Then csi driver can extract annotations from PVC to prepare the secret for pod.
const (
	AnnotationSecretsClass string = secretAPIGroupPrefix + "class"

	// Scope is the scope of the secret.
	// It can be one of the following values:
	//	- pod
	//	- node
	//	- service	// can be multiple
	//	- listener-volume	// can be multiple
	//
	// Example:
	//	- "secrets.kubedoop.dev/scope": "pod"
	//	- "secrets.kubedoop.dev/scope": "node"
	//	- "secrets.kubedoop.dev/scope": "service=foo"
	//	- "secrets.kubedoop.dev/scope": "listener-volume=foo"
	//	- "secrets.kubedoop.dev/scope": "pod,service=foo,service=bar,listner-volume=xyz"
	AnnotationSecretsScope string = secretAPIGroupPrefix + "scope"

	// Format is mounted format of the secret.
	// It can be one of the following values:
	//	- tls-pem  A PEM-encoded TLS certificate, include "tls.crt", "tls.key", "ca.crt".
	//	- tls-p12 A PKCS#12 archive, include "keystore.p12", "truststore.p12".
	//	- kerberos A Kerberos keytab, include "keytab", "krb5.conf".
	AnnotationSecretsFormat string = secretAPIGroupPrefix + "format"

	// PKCS12 format password, it will be used truststore and keystore password.
	AnnotationSecretsPKCS12Password string = secretAPIGroupPrefix + "tlsPKCS12Password"
	// golang time.Duration string, it will be used to create certificate expiration time.
	AnnotationSecretCertLifeTime      string = secretAPIGroupPrefix + "autoTlsCertLifetime"
	AnnotationSecretsCertJitterFactor string = secretAPIGroupPrefix + "autoTlsCertJitterFactor"
	// When a large number of Pods restart at a similar time,
	// because the pod restart time is uncertain, the restart process may be relatively long,
	// even if there is a time limit for elegant shutdown, there will still be a case of pod late restart
	// resulting in certificate expiration.
	// To avoid this, the pod expiration time is checked before this buffer time.
	AnnotationSecretsCertRestartBuffer string = "secrets.kubedoop.dev/" + "autoTlsCertRestartBuffer"

	// KerberosServiceNames is the list of Kerberos service names.
	// It is a comma separated list of Kerberos realms.
	//
	// If this filed value is "HTTP,NN,DN", and scope is specified a service name: "service=<k8s-service>".
	// It is used to create kerberos realm.
	// 	- HTTP -> HTTP/<k8s-service>.<k8s-namespace>.cluster.local@REALM
	// 	- NN -> nn/<k8s-service>.<k8s-namespace>.cluster.local@REALM
	// 	- DN -> dn/<k8s-service>.<k8s-namespace>.cluster.local@REALM
	//
	// If this field value is "NN", and scope is "pod"
	// It is used to create kerberos realm:
	// 	- nn/<pod-name>.<pod-subdomain>.<k8s-namespace>.cluster.local@REALM		# https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/#pods
	//
	// If this field value is "DN", and scope is "node"
	// It is used to create kerberos realm:
	// 	- dn/<node-name>.<k8s-namespace>.cluster.local@REALM
	//
	// If this field value is "HTTP", and scope is "listener-volume=foo"
	// It is used to create kerberos realm:
	// 	- HTTP/<the-service-of-listener-foo>.<k8s-namespace>.cluster.local@REALM
	AnnotationSecretsKerberosServiceNames string = secretAPIGroupPrefix + "kerberosServiceNames"
)

type SecretFormat string

const (
	TLSPEM   SecretFormat = "tls-pem"
	TLSP12   SecretFormat = "tls-p12"
	Kerberos SecretFormat = "kerberos"
)

const (
	CommonDelimiter               string = ","
	ListenerVolumeDelimiter       string = CommonDelimiter
	KerberosServiceNamesDelimiter string = CommonDelimiter
)

type SecretScope string

const (
	PodScope            SecretScope = "pod"
	NodeScope           SecretScope = "node"
	ServiceScope        SecretScope = "service"
	ListenerVolumeScope SecretScope = "listener-volume"
)
