package reconciler

import (
	"strings"

	"github.com/zncdatadev/operator-go/pkg/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ClusterInfo struct {
	GVK         *metav1.GroupVersionKind
	ClusterName string

	annotations map[string]string

	labels map[string]string
}

func (i *ClusterInfo) GetFullName() string {
	return i.ClusterName
}

func (i *ClusterInfo) GetClusterName() string {
	return i.ClusterName
}

func (i *ClusterInfo) AddLabel(key, value string) {
	if i.labels == nil {
		i.labels = map[string]string{}
	}
	i.labels[key] = value
}

func (i *ClusterInfo) GetLabels() map[string]string {
	if i.labels == nil {
		i.labels = map[string]string{
			constants.LabelKubernetesInstance:  i.ClusterName,
			constants.LabelKubernetesName:      strings.ToLower(i.GVK.Kind),
			constants.LabelKubernetesManagedBy: i.GVK.Group,
		}
	}
	return i.labels
}

func (i *ClusterInfo) AddAnnotation(key, value string) {
	if i.annotations == nil {
		i.annotations = map[string]string{}
	}
	i.annotations[key] = value
}

func (i *ClusterInfo) GetAnnotations() map[string]string {
	if i.annotations == nil {
		i.annotations = map[string]string{}
	}
	return i.annotations
}

type RoleInfo struct {
	ClusterInfo
	RoleName string

	annotations map[string]string

	labels map[string]string
}

func (i *RoleInfo) GetFullName() string {
	return i.GetClusterName() + "-" + i.RoleName
}

func (i *RoleInfo) GetRoleName() string {
	return i.RoleName
}

func (i *RoleInfo) AddLabel(key, value string) {
	if i.labels == nil {
		i.labels = map[string]string{}
		for k, v := range i.ClusterInfo.GetLabels() {
			i.labels[k] = v
		}
	}
	i.labels[key] = value
}

func (i *RoleInfo) GetLabels() map[string]string {
	if i.labels == nil {
		i.labels = map[string]string{}
		for k, v := range i.ClusterInfo.GetLabels() {
			i.labels[k] = v
		}
	}

	i.labels[constants.LabelKubernetesComponent] = i.RoleName
	return i.labels
}

func (i *RoleInfo) AddAnnotation(key, value string) {
	if i.annotations == nil {
		i.annotations = map[string]string{}
		for k, v := range i.ClusterInfo.GetAnnotations() {
			i.annotations[k] = v
		}
	}
	i.annotations[key] = value
}

func (i *RoleInfo) GetAnnotations() map[string]string {
	if i.annotations == nil {
		i.annotations = map[string]string{}
		for k, v := range i.ClusterInfo.GetAnnotations() {
			i.annotations[k] = v
		}
	}
	return i.annotations
}

type RoleGroupInfo struct {
	RoleInfo
	RoleGroupName string

	annotations map[string]string

	labels map[string]string
}

func (i *RoleGroupInfo) GetFullName() string {
	return i.RoleInfo.GetFullName() + "-" + i.RoleGroupName
}

func (i *RoleGroupInfo) GetGroupName() string {
	return i.RoleGroupName
}

func (i *RoleGroupInfo) GetLabels() map[string]string {
	if i.labels == nil {
		i.labels = map[string]string{}
		for k, v := range i.RoleInfo.GetLabels() {
			i.labels[k] = v
		}
	}

	i.labels[constants.LabelKubernetesRoleGroup] = i.RoleGroupName
	return i.labels
}

func (i *RoleGroupInfo) GetAnnotations() map[string]string {
	if i.annotations == nil {
		i.annotations = map[string]string{}
		for k, v := range i.RoleInfo.GetAnnotations() {
			i.annotations[k] = v
		}
	}
	return i.annotations
}

func (i *RoleGroupInfo) AddLabel(key, value string) {
	if i.labels == nil {
		i.labels = map[string]string{}
		for k, v := range i.RoleInfo.GetLabels() {
			i.labels[k] = v
		}
	}
	i.labels[key] = value
}
