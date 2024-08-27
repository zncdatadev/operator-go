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

	KubedoopDomain = "zncdata.dev"
)

const (
	KubedoopRoot = "/kubedoop/"

	KubedoopKerberosDir    = KubedoopRoot + "kerberos/"
	KubedoopTlsDir         = KubedoopRoot + "tls/"
	KubedoopListenerDir    = KubedoopRoot + "listener/"
	KubedoopSecretDir      = KubedoopRoot + "secret/"
	KubedoopDataDir        = KubedoopRoot + "data/"
	KubedoopConfigDir      = KubedoopRoot + "config/"
	KubedoopLogDir         = KubedoopRoot + "log/"
	KubedoopConfigDirMount = KubedoopRoot + "mount/config/"
	KubedoopLogDirMount    = KubedoopRoot + "mount/log/"
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
