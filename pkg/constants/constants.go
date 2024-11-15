package constants

// k8s recommended labels for app
// https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
// https://kubernetes.io/docs/reference/labels-annotations-taints/
const (
	LabelKubernetesComponent = "app.kubernetes.io/component"
	LabelKubernetesInstance  = "app.kubernetes.io/instance"
	LabelKubernetesName      = "app.kubernetes.io/name"
	LabelKubernetesManagedBy = "app.kubernetes.io/managed-by"
	LabelKubernetesRoleGroup = "app.kubernetes.io/role-group"
	LabelKubernetesVersion   = "app.kubernetes.io/version"
)

const (
	KubedoopDomain = "kubedoop.dev"
)

const (
	KubedoopRoot = "/kubedoop/"

	KubedoopKerberosDir    = KubedoopRoot + "kerberos/"
	KubedoopTlsDir         = KubedoopRoot + "tls/"
	KubedoopListenerDir    = KubedoopRoot + "listener/"
	KubedoopJmxDir         = KubedoopRoot + "jmx/"
	KubedoopSecretDir      = KubedoopRoot + "secret/"
	KubedoopDataDir        = KubedoopRoot + "data/"
	KubedoopConfigDir      = KubedoopRoot + "config/"
	KubedoopLogDir         = KubedoopRoot + "log/"
	KubedoopConfigDirMount = KubedoopRoot + "mount/config/"
	KubedoopLogDirMount    = KubedoopRoot + "mount/log/"
)

// When a pod has the label `enrichment.kubedoop.dev/enable=true`,
// the enrichment controller will set the node address to the pod annotation when the pod is created.
const (
	LabelEnrichmentEnable      = "enrichment." + KubedoopDomain + "/enable"
	LabelEnrichmentEnableValue = "true"
	LabelEnrichmentNodeAddress = "enrichment." + KubedoopDomain + "/node-address"
)

// Restarter policy has workload restart and pod expiration.
//
// Workload restarter:
//
//	If a workload has the label `restarter.kubedoop.dev/enable=true`,
//	 and a configmap or secret is updated when mounted as a volume in the pod,
//	 the restarter will update the annotations in the workload podTemplate.
//	 The workload controller will update all the pods of the workload.
//
// Pod expiration:
//
//	When workload mount with secret-class of secret-operator, some secrets will be
//	 created and mount for the pod by the secret-operator. Eg: kerberos, tls, etc.
//	 Tls and kerberos secrets have expiration time, when the secrets is created,
//	 secret-operator will set the expiration time in the pod annotation.
//	 The restarter will check the expiration time in the pod annotation, if the expiration time is expired,
//	 the restarter will restart the pod.
const (
	LabelRestarterEnable      = "restarter." + KubedoopDomain + "/enable"
	LabelRestarterEnableValue = "true"

	// eg:
	// 	- secret.restarter.kubedoop.dev/foo-secret: <secret-uuid>/<secret-resourceversion>
	// 	- configmap.restarter.kubedoop.dev/foo-configmap: <configmap-uuid>/<configmap-resourceversion>

	AnnotationSecretRestarterPrefix    = "secret.restarter." + KubedoopDomain + "/"
	AnnotationConfigmapRestarterPrefix = "configmap.restarter." + KubedoopDomain + "/"

	// eg:
	// 	- restarter.kubedoop.dev/expires-at.<RFC3339>: <volume-id>
	// RFC3339: 2006-01-02T15:04:05Z07:00
	PrefixLabelRestarterExpiresAt = "restarter." + KubedoopDomain + "/expires-at."
)

func MatchingLabelsNames() []string {
	return []string{
		LabelKubernetesName,
		LabelKubernetesInstance,
		LabelKubernetesRoleGroup,
		LabelKubernetesComponent,
		LabelKubernetesManagedBy,
	}
}
