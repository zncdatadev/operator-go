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

// Zncdata defined annotations for PVCTemplate.
// Then csi driver can extract annotations from PVC to prepare the secret for pod.
const (
	AnnotationSecretsClass string = secretAPIGroupPrefix + "class"

	// Scope is the scope of the secret.
	// It can be one of the following values:
	//	- pod
	//	- node
	//	- service
	//	- listener-volume
	//
	// Example:
	//	- "secrets.zncdata.dev/scope": "pod"
	//	- "secrets.zncdata.dev/scope": "node"
	//	- "secrets.zncdata.dev/scope": "service=foo"
	//	- "secrets.zncdata.dev/scope": "listener-volume=foo"
	//	- "secrets.zncdata.dev/scope": "pod,service=foo,bar,listner-volume=xyz"
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

	// Annotation for expiration time of zncdata secret for pod.
	// When the secret is created, the expiration time is set to the current time plus the lifetime.
	// Then we can clean up the secret after expiration time
	AnnonationSecretExpirationTimeName string = secretAPIGroupPrefix + "expirationTime"

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
