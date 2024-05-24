package builder

import (
	apiv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/util"
	corev1 "k8s.io/api/core/v1"
)

var (
	MatchingLabelsNames = []string{
		"app.kubernetes.io/name",
		"app.kubernetes.io/instance",
		"app.kubernetes.io/role-group",
		"app.kubernetes.io/component",
	}
)

type Options interface {
	GetClusterName() string
	GetName() string
	GetNamespace() string
	GetFullName() string
	GetLabels() map[string]string
	AddLabels(labels map[string]string)
	GetMatchingLabels() map[string]string
	GetAnnotations() map[string]string

	GetClusterOperation() *apiv1alpha1.ClusterOperationSpec
	GetImage() *util.Image
	GetPorts() []corev1.ContainerPort
	SetPorts(ports []corev1.ContainerPort)
}

var _ Options = &ClusterOptions{}

type ClusterOptions struct {
	Name             string
	Namespace        string
	Labels           map[string]string
	Annotations      map[string]string
	ClusterOperation *apiv1alpha1.ClusterOperationSpec
	Image            *util.Image
	Ports            []corev1.ContainerPort
}

func (o *ClusterOptions) GetClusterName() string {
	return o.Name
}

func (o *ClusterOptions) GetName() string {
	return o.Name
}

func (o *ClusterOptions) GetNamespace() string {
	return o.Namespace
}

func (o *ClusterOptions) GetFullName() string {
	return o.Name
}

func (o *ClusterOptions) GetLabels() map[string]string {
	return o.Labels
}

func (o *ClusterOptions) GetAnnotations() map[string]string {
	return o.Annotations
}

func (o *ClusterOptions) AddLabels(labels map[string]string) {
	for k, v := range labels {
		o.Labels[k] = v
	}
}

func (o *ClusterOptions) filterLabels(labels map[string]string) map[string]string {

	matchingLabels := make(map[string]string)
	for _, label := range MatchingLabelsNames {
		if value, ok := labels[label]; ok {
			matchingLabels[label] = value
		}
	}
	return matchingLabels
}

func (o *ClusterOptions) GetMatchingLabels() map[string]string {
	return o.filterLabels(o.GetLabels())
}

func (o *ClusterOptions) GetClusterOperation() *apiv1alpha1.ClusterOperationSpec {
	return o.ClusterOperation
}

func (o *ClusterOptions) GetImage() *util.Image {
	return o.Image
}

func (o *ClusterOptions) GetPorts() []corev1.ContainerPort {
	return o.Ports
}

func (o *ClusterOptions) SetPorts(ports []corev1.ContainerPort) {
	o.Ports = ports
}

type RoleOptions struct {
	ClusterOptions
	Name string
}

func (o *RoleOptions) GetName() string {
	return o.Name
}

func (o *RoleOptions) GetFullName() string {
	return o.ClusterOptions.Name + "-" + o.Name
}

func (o *RoleOptions) GetLabels() map[string]string {
	labels := o.ClusterOptions.Labels
	labels["app.kubernetes.io/component"] = o.Name
	return labels
}

func (o *RoleOptions) GetMatchingLabels() map[string]string {
	return o.filterLabels(o.GetLabels())
}

type RoleGroupOptions struct {
	RoleOptions
	Name     string
	Replicas *int32

	PodDisruptionBudget *apiv1alpha1.PodDisruptionBudgetSpec

	CommandOverrides []string
	EnvOverrides     map[string]string
	PodOverrides     *corev1.PodTemplateSpec
}

func (o *RoleGroupOptions) GetName() string {
	return o.Name
}

func (o *RoleGroupOptions) GetFullName() string {
	return o.RoleOptions.GetFullName() + "-" + o.Name
}

func (o *RoleGroupOptions) GetLabels() map[string]string {
	labels := o.RoleOptions.GetLabels()
	labels["app.kubernetes.io/role-group"] = o.Name
	return labels
}

func (o *RoleGroupOptions) GetMatchingLabels() map[string]string {
	return o.filterLabels(o.GetLabels())
}
