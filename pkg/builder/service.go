package builder

import (
	"context"

	"github.com/zncdatadev/operator-go/pkg/client"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func ContainerPorts2ServicePorts(port []corev1.ContainerPort) []corev1.ServicePort {
	ports := make([]corev1.ServicePort, 0)
	for _, p := range port {
		target := intstr.FromString(p.Name)

		if p.Name == "" {
			target = intstr.FromInt32(p.ContainerPort)
		}

		ports = append(ports, corev1.ServicePort{
			Name:       p.Name,
			Port:       p.ContainerPort,
			Protocol:   p.Protocol,
			TargetPort: target,
		})
	}

	return ports
}

type ServiceBuilder interface {
	ResourceBuilder
	GetObject() *corev1.Service
	AddPort(port *corev1.ServicePort)
	GetPorts() []corev1.ServicePort
	GetServiceType() corev1.ServiceType
}

var _ ServiceBuilder = &BaseServiceBuilder{}

type BaseServiceBuilder struct {
	BaseResourceBuilder

	// if you want to get ports, please use GetPorts() method
	ports []corev1.ServicePort
}

func (b *BaseServiceBuilder) GetObject() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: b.GetObjectMeta(),
		Spec: corev1.ServiceSpec{
			Ports:    b.GetPorts(),
			Selector: b.GetMatchingLabels(),
			Type:     b.GetServiceType(),
		},
	}
}

func (b *BaseServiceBuilder) AddPort(port *corev1.ServicePort) {
	b.ports = append(b.ports, *port)
}

func (b *BaseServiceBuilder) GetPorts() []corev1.ServicePort {
	return b.ports
}

func (b *BaseServiceBuilder) GetServiceType() corev1.ServiceType {
	return corev1.ServiceTypeClusterIP
}

func (b *BaseServiceBuilder) Build(_ context.Context) (ctrlclient.Object, error) {
	obj := b.GetObject()
	return obj, nil
}

type ServiceBuilderOptions struct {
	Name        string
	Labels      map[string]string
	Annotations map[string]string
	Ports       []corev1.ContainerPort
}

func NewServiceBuilder(
	client *client.Client,
	options *ServiceBuilderOptions,
) *BaseServiceBuilder {

	ports := ContainerPorts2ServicePorts(options.Ports)

	return &BaseServiceBuilder{
		BaseResourceBuilder: BaseResourceBuilder{
			Client: client,
			name:   options.Name,
			labels: options.Labels,
		},
		ports: ports,
	}
}
