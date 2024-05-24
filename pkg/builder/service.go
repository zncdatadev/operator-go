package builder

import (
	"context"

	"github.com/zncdatadev/operator-go/pkg/client"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func ContainerPort2ServicePort(port corev1.ContainerPort) *corev1.ServicePort {
	target := intstr.FromString(port.Name)

	if port.Name == "" {
		target = intstr.FromInt32(port.ContainerPort)
	}

	return &corev1.ServicePort{
		Name:       port.Name,
		Port:       port.ContainerPort,
		Protocol:   port.Protocol,
		TargetPort: target,
	}
}

type ServiceBuilder interface {
	Builder
	GetObject() *corev1.Service
	AddPort(port *corev1.ContainerPort)
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
			Selector: b.Options.GetMatchingLabels(),
			Type:     b.GetServiceType(),
		},
	}
}

func (b *BaseServiceBuilder) AddPort(port *corev1.ContainerPort) {
	p := ContainerPort2ServicePort(*port)

	b.ports = append(b.ports, *p)
}

func (b *BaseServiceBuilder) GetPorts() []corev1.ServicePort {
	optionsPorts := b.Options.GetPorts()
	ports := b.ports

	for _, port := range optionsPorts {
		ports = append(ports, *ContainerPort2ServicePort(port))
	}

	return ports
}

func (b *BaseServiceBuilder) GetServiceType() corev1.ServiceType {
	return corev1.ServiceTypeClusterIP
}

func (b *BaseServiceBuilder) Build(_ context.Context) (ctrlclient.Object, error) {
	obj := b.GetObject()
	return obj, nil
}

func NewServiceBuilder(
	client *client.Client,
	options Options,
) *BaseServiceBuilder {
	return &BaseServiceBuilder{
		BaseResourceBuilder: BaseResourceBuilder{
			Client:  client,
			Options: options,
		},
	}
}
