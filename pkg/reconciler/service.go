package reconciler

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
)

var _ ResourceReconciler[builder.ServiceBuilder] = &Service{}

type Service struct {
	GenericResourceReconciler[builder.ServiceBuilder]
}

func NewServiceReconciler(
	client *client.Client,
	name string,
	ports []corev1.ContainerPort,
	options ...builder.ServiceBuilderOption,
) *Service {
	svcBuilder := builder.NewServiceBuilder(
		client,
		name,
		ports,
		options...,
	)
	return &Service{
		GenericResourceReconciler: *NewGenericResourceReconciler[builder.ServiceBuilder](
			client,
			svcBuilder,
		),
	}
}
