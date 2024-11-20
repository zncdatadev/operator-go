package builder

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/constants"
)

// Deprecated: Use PortsConverter instead
func ContainerPorts2ServicePorts(port []corev1.ContainerPort) []corev1.ServicePort {
	return PortsConverter(port)
}

func PortsConverter(port []corev1.ContainerPort) []corev1.ServicePort {
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

// ListenerClass2ServiceType converts listener class to k8s service type
//
//	ClusterInternal --> ClusterIP
//	ExternalUnstable --> NodePort
//	ExternalStable --> LoadBalancer
//	Default --> ClusterIP
func ListenerClass2ServiceType(listenerClass constants.ListenerClass) corev1.ServiceType {
	switch listenerClass {
	case constants.ClusterInternal:
		return corev1.ServiceTypeClusterIP
	case constants.ExternalUnstable:
		return corev1.ServiceTypeNodePort
	case constants.ExternalStable:
		return corev1.ServiceTypeLoadBalancer
	default:
		return corev1.ServiceTypeClusterIP
	}
}

var _ ServiceBuilder = &BaseServiceBuilder{}

type BaseServiceBuilder struct {
	ObjectMeta

	ports         []corev1.ServicePort
	listenerClass constants.ListenerClass
	headless      bool
	// Setting this parameter will override the default matching labels, generally not needed
	matchingLabels map[string]string
}

type ServiceBuilderOptions struct {
	Options

	// If not set, ClusterIP will be used
	ListenerClass  constants.ListenerClass
	Headless       bool
	MatchingLabels map[string]string
}

type ServiceBuilderOption func(*ServiceBuilderOptions)

func NewServiceBuilder(
	client *client.Client,
	name string,
	ports []corev1.ContainerPort,
	options ...ServiceBuilderOption,
) *BaseServiceBuilder {

	opts := &ServiceBuilderOptions{}

	for _, o := range options {
		o(opts)
	}

	return &BaseServiceBuilder{
		ObjectMeta: ObjectMeta{
			Client:      client,
			Name:        name,
			labels:      opts.Labels,
			annotations: opts.Annotations,
		},
		ports: PortsConverter(ports),

		headless:       opts.Headless,
		matchingLabels: opts.MatchingLabels,
		listenerClass:  opts.ListenerClass,
	}
}

func (b *BaseServiceBuilder) GetObject() *corev1.Service {
	matchingLabels := b.GetMatchingLabels()
	if b.matchingLabels != nil {
		matchingLabels = b.matchingLabels
	}
	obj := &corev1.Service{
		ObjectMeta: b.GetObjectMeta(),
		Spec: corev1.ServiceSpec{
			Ports:    b.GetPorts(),
			Selector: matchingLabels,
			Type:     ListenerClass2ServiceType(b.listenerClass),
		},
	}

	if b.headless && b.listenerClass == constants.ClusterInternal {
		obj.Spec.ClusterIP = corev1.ClusterIPNone
	}

	return obj
}

func (b *BaseServiceBuilder) AddPort(port *corev1.ServicePort) {
	b.ports = append(b.ports, *port)
}

func (b *BaseServiceBuilder) GetPorts() []corev1.ServicePort {
	return b.ports
}

func (b *BaseServiceBuilder) Build(_ context.Context) (ctrlclient.Object, error) {
	obj := b.GetObject()
	return obj, nil
}
