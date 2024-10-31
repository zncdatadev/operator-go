package builder

import (
	"context"

	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	logger = ctrl.Log.WithName("builder")
)

var _ ResourceBuilder = &BaseResourceBuilder{}

type BaseResourceBuilder struct {
	Client *client.Client

	Name        string // this is resource name when creating
	labels      map[string]string
	annotations map[string]string

	ClusterName   string
	RoleName      string
	RoleGroupName string
}

func NewBaseResourceBuilder(
	client *client.Client,
	name string, // this is resource name when creating
	options ...Options,
) *BaseResourceBuilder {

	var opt Option

	for _, o := range options {
		opt = o(opt)
	}

	return &BaseResourceBuilder{
		Client:        client,
		Name:          name,
		labels:        opt.Labels,
		annotations:   opt.Annotations,
		ClusterName:   opt.ClusterName,
		RoleName:      opt.RoleName,
		RoleGroupName: opt.RoleGroupName,
	}
}

func (b *BaseResourceBuilder) GetClient() *client.Client {
	return b.Client
}

func (b *BaseResourceBuilder) SetName(name string) {
	b.Name = name
}

func (b *BaseResourceBuilder) GetName() string {
	return b.Name
}

func (b *BaseResourceBuilder) AddLabels(labels map[string]string) {
	if b.labels == nil {
		b.labels = make(map[string]string)
	}
	for k, v := range labels {
		b.labels[k] = v
	}
}

func (b *BaseResourceBuilder) GetLabels() map[string]string {
	if b.labels == nil {
		b.labels = map[string]string{
			constants.LabelKubernetesInstance:  b.Client.GetOwnerName(),
			constants.LabelKubernetesManagedBy: constants.KubedoopDomain,
		}

		if b.ClusterName != "" {
			b.labels[constants.LabelKubernetesInstance] = b.ClusterName
		}

		if b.RoleName != "" {
			b.labels[constants.LabelKubernetesComponent] = b.RoleName
		}

		if b.RoleGroupName != "" {
			b.labels[constants.LabelKubernetesRoleGroup] = b.RoleGroupName
		}
	}

	return b.labels
}

func (o *BaseResourceBuilder) filterLabels(labels map[string]string) map[string]string {
	matchingLabels := make(map[string]string)
	for _, label := range constants.MatchingLabelsNames() {
		if value, ok := labels[label]; ok {
			matchingLabels[label] = value
		}
	}
	return matchingLabels
}

func (b *BaseResourceBuilder) GetMatchingLabels() map[string]string {
	return b.filterLabels(b.GetLabels())
}

func (b *BaseResourceBuilder) GetLabelSelector() *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: b.GetMatchingLabels(),
	}
}

func (b *BaseResourceBuilder) AddAnnotations(annotations map[string]string) {
	if b.annotations == nil {
		b.annotations = make(map[string]string)
	}
	for k, v := range annotations {
		b.annotations[k] = v
	}
}

func (b *BaseResourceBuilder) GetAnnotations() map[string]string {
	return b.annotations
}

func (b *BaseResourceBuilder) GetObjectMeta() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:        b.GetName(),
		Namespace:   b.Client.GetOwnerNamespace(),
		Labels:      b.GetLabels(),
		Annotations: b.annotations,
	}
}

// GetObjectMetaWithClusterScope returns the object meta with cluster scope,
// meaning that the object is not namespaced.
func (b *BaseResourceBuilder) GetObjectMetaWithClusterScope() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:        b.GetName(),
		Labels:      b.GetLabels(),
		Annotations: b.annotations,
	}
}

func (b *BaseResourceBuilder) GetObject() (ctrlclient.Object, error) {
	panic("implement me")
}

func (b *BaseResourceBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	panic("implement me")
}
