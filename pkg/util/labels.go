package util

const (
	AppKubernetesComponentName = "app.kubernetes.io/component"
	AppKubernetesInstanceName  = "app.kubernetes.io/instance"
	AppKubernetesNameName      = "app.kubernetes.io/name"
	AppKubernetesManagedByName = "app.kubernetes.io/managed-by"
	AppKubernetesRoleGroupName = "app.kubernetes.io/role-group"

	StackDomain = "zncdata.dev"
)

var (
	AppMatchingLabelsNames = []string{
		AppKubernetesNameName,
		AppKubernetesInstanceName,
		AppKubernetesRoleGroupName,
		AppKubernetesComponentName,
		AppKubernetesManagedByName,
	}
)

func MergeStringMaps(maps ...map[string]string) map[string]string {
	result := map[string]string{}
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}
