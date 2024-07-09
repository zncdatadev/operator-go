package reconciler

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type InfoName interface {
	GetFullName() string
	GetClusterName() string
}

type InfoLabel interface {
	AddLabel(key, value string) InfoLabel
	GetLabels() map[string]string
}

type InfoAnnotation interface {
	AddAnnotation(key, value string) InfoAnnotation
	GetAnnotations() map[string]string
}

var _ InfoName = &ClusterInfo{}
var _ InfoLabel = &ClusterInfo{}
var _ InfoAnnotation = &ClusterInfo{}

type ClusterInfo struct {
	GVK         *metav1.GroupVersionKind
	ClusterName string
	Namespace   string

	annotations map[string]string

	labels map[string]string
}

func (i *ClusterInfo) GetFullName() string {
	return i.ClusterName
}

func (i *ClusterInfo) GetClusterName() string {
	return i.ClusterName
}

func (i *ClusterInfo) AddLabel(key, value string) InfoLabel {
	if i.labels == nil {
		i.labels = map[string]string{}
	}
	i.labels[key] = value
	return i
}

func (i *ClusterInfo) GetLabels() map[string]string {
	if i.labels == nil {
		i.labels = map[string]string{
			"app.kubernetes.io/instance":   i.ClusterName,
			"app.kubernetes.io/name":       strings.ToLower(i.GVK.Kind),
			"app.kubernetes.io/managed-by": i.GVK.Group,
		}
	}
	return i.labels
}

func (i *ClusterInfo) AddAnnotation(key, value string) InfoAnnotation {
	if i.annotations == nil {
		i.annotations = map[string]string{}
	}
	i.annotations[key] = value
	return i
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
}

func (i *RoleInfo) GetFullName() string {
	return i.GetClusterName() + "-" + i.RoleName
}

func (i *RoleInfo) GetRoleName() string {
	return i.RoleName
}

func (i *RoleInfo) GetLabels() map[string]string {
	labels := i.ClusterInfo.GetLabels()
	labels["app.kubernetes.io/component"] = i.RoleName
	return i.labels
}

type RoleGroupInfo struct {
	RoleInfo
	GroupName string
}

func (i *RoleGroupInfo) GetFullName() string {
	return i.RoleInfo.GetFullName() + "-" + i.GroupName
}

func (i *RoleGroupInfo) GetGroupName() string {
	return i.GroupName
}

func (i *RoleGroupInfo) GetLabels() map[string]string {
	labels := i.RoleInfo.GetLabels()
	labels["app.kubernetes.io/role-group"] = i.GroupName
	return labels
}
