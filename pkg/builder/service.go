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

func NewServiceBuilder(
	client *client.Client,
	name string,
	labels map[string]string,
	annotations map[string]string,
	ports []corev1.ContainerPort,
) *BaseServiceBuilder {

	servicePorts := ContainerPorts2ServicePorts(ports)

	return &BaseServiceBuilder{
		BaseResourceBuilder: BaseResourceBuilder{
			Client: client,
			Name:   name,
			labels: labels,
		},
		ports: servicePorts,
	}
}
