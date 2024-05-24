package builder

import (
	"context"

	"github.com/zncdatadev/operator-go/pkg/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	logger = ctrl.Log.WithName("builder")
)

type Builder interface {
	Build(ctx context.Context) (ctrlclient.Object, error)
	GetObjectMeta() metav1.ObjectMeta
	GetClient() *client.Client
	SetName(name string)
	GetName() string
}

var _ Builder = &BaseResourceBuilder{}

type BaseResourceBuilder struct {
	Client  *client.Client
	Options Options

	modifiedName string
}

func (b *BaseResourceBuilder) GetClient() *client.Client {
	return b.Client
}

func (b *BaseResourceBuilder) SetName(name string) {
	b.modifiedName = name
}

func (b *BaseResourceBuilder) GetName() string {
	if b.modifiedName != "" {
		return b.modifiedName
	}
	return b.Options.GetFullName()
}

func (b *BaseResourceBuilder) GetObjectMeta() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:        b.GetName(),
		Namespace:   b.Client.GetOwnerNamespace(),
		Labels:      b.Options.GetLabels(),
		Annotations: b.Options.GetAnnotations(),
	}
}

// GetObjectMetaWithClusterScope returns the object meta with cluster scope
func (b *BaseResourceBuilder) GetObjectMetaWithClusterScope() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:        b.GetName(),
		Labels:      b.Options.GetLabels(),
		Annotations: b.Options.GetAnnotations(),
	}
}

func (b *BaseResourceBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	panic("implement me")
}
