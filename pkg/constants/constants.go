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

	ZncdataDomain = "zncdata.dev"
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
