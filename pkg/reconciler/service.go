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

func NewServiceReconciler(
	client *client.Client,
	name string,
	labels map[string]string,
	annotations map[string]string,
	ports []corev1.ContainerPort,
	serviceType *corev1.ServiceType,
) *Service {
	svcBuilder := builder.NewServiceBuilder(
		client,
		name,
		labels,
		annotations,
		ports,
		serviceType,
	)
	return &Service{
		GenericResourceReconciler: *NewGenericResourceReconciler[builder.ServiceBuilder](
			client,
			name,
			svcBuilder,
		),
	}
}
