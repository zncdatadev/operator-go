package reconciler

import (
	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	corev1 "k8s.io/api/core/v1"
)

var _ ResourceReconciler[builder.ServiceBuilder] = &Service{}

type Service struct {
	GenericResourceReconciler[builder.ServiceBuilder]
}

type ServiceReconcilerOptions struct {
	ResourceReconcilerOptions
	Labels      map[string]string
	Annotations map[string]string

	Ports []corev1.ContainerPort
}

func NewServiceReconciler(
	client *client.Client,
	options *ServiceReconcilerOptions,
) *Service {
	svcBuilder := builder.NewServiceBuilder(
		client,
		&builder.ServiceBuilderOptions{
			Name:        options.Name,
			Labels:      options.Labels,
			Annotations: options.Annotations,
			Ports:       options.Ports,
		},
	)
	return &Service{
		GenericResourceReconciler: *NewGenericResourceReconciler[builder.ServiceBuilder](
			client,
			&options.ResourceReconcilerOptions,
			svcBuilder,
		),
	}
}
