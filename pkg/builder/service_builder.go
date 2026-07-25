/*
Copyright 2024 ZNCDataDev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package builder

import (
	"maps"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ServiceType defines the type of service to create.
type ServiceType string

const (
	// ServiceTypeHeadless creates a headless service (ClusterIP: None).
	ServiceTypeHeadless ServiceType = "Headless"
	// ServiceTypeClusterIP creates a ClusterIP service.
	ServiceTypeClusterIP ServiceType = "ClusterIP"
	// ServiceTypeNodePort creates a NodePort service.
	ServiceTypeNodePort ServiceType = "NodePort"
	// ServiceTypeLoadBalancer creates a LoadBalancer service.
	ServiceTypeLoadBalancer ServiceType = "LoadBalancer"
)

// ServiceBuilder constructs Service resources.
type ServiceBuilder struct {
	Name        string
	Namespace   string
	Labels      map[string]string
	Annotations map[string]string
	Selector    map[string]string
	Type        corev1.ServiceType
	Ports       []corev1.ServicePort
	Headless    bool

	// PublishNotReadyAddresses exposes a pod's DNS record before it passes its readiness probe.
	// Quorum systems need it: their members must resolve each other to form the quorum that makes
	// them ready in the first place.
	PublishNotReadyAddresses bool
}

// NewServiceBuilder creates a new ServiceBuilder.
func NewServiceBuilder(name, namespace string) *ServiceBuilder {
	return &ServiceBuilder{
		Name:        name,
		Namespace:   namespace,
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
		Selector:    make(map[string]string),
		Type:        corev1.ServiceTypeClusterIP,
		Ports:       make([]corev1.ServicePort, 0),
	}
}

// WithLabels sets the labels.
func (b *ServiceBuilder) WithLabels(labels map[string]string) *ServiceBuilder {
	for k, v := range labels {
		b.Labels[k] = v
	}
	return b
}

// WithAnnotations sets the annotations.
func (b *ServiceBuilder) WithAnnotations(annotations map[string]string) *ServiceBuilder {
	for k, v := range annotations {
		b.Annotations[k] = v
	}
	return b
}

// WithSelector sets the selector.
func (b *ServiceBuilder) WithSelector(selector map[string]string) *ServiceBuilder {
	for k, v := range selector {
		b.Selector[k] = v
	}
	return b
}

// WithServiceType sets the service type.
func (b *ServiceBuilder) WithServiceType(svcType ServiceType) *ServiceBuilder {
	switch svcType {
	case ServiceTypeHeadless:
		b.Headless = true
		b.Type = corev1.ServiceTypeClusterIP
	case ServiceTypeClusterIP:
		b.Type = corev1.ServiceTypeClusterIP
	case ServiceTypeNodePort:
		b.Type = corev1.ServiceTypeNodePort
	case ServiceTypeLoadBalancer:
		b.Type = corev1.ServiceTypeLoadBalancer
	}
	return b
}

// WithPublishNotReadyAddresses sets .spec.publishNotReadyAddresses.
func (b *ServiceBuilder) WithPublishNotReadyAddresses(publish bool) *ServiceBuilder {
	b.PublishNotReadyAddresses = publish
	return b
}

// AddServicePort appends a fully specified service port. Unlike AddPort it preserves every field
// the caller set (nodePort, appProtocol, a named targetPort), so callers holding ready-made
// corev1.ServicePort values do not silently lose them.
func (b *ServiceBuilder) AddServicePort(port corev1.ServicePort) *ServiceBuilder {
	b.Ports = append(b.Ports, *port.DeepCopy())
	return b
}

// WithPorts appends the given fully specified service ports (see AddServicePort).
func (b *ServiceBuilder) WithPorts(ports []corev1.ServicePort) *ServiceBuilder {
	for _, port := range ports {
		b.AddServicePort(port)
	}
	return b
}

// AddPort adds a service port.
func (b *ServiceBuilder) AddPort(name string, port int32, targetPort intstr.IntOrString, protocol corev1.Protocol) *ServiceBuilder {
	b.Ports = append(b.Ports, corev1.ServicePort{
		Name:       name,
		Port:       port,
		TargetPort: targetPort,
		Protocol:   protocol,
	})
	return b
}

// AddPortSimple adds a simple service port (port equals target port).
func (b *ServiceBuilder) AddPortSimple(name string, port int32, protocol corev1.Protocol) *ServiceBuilder {
	return b.AddPort(name, port, intstr.FromInt(int(port)), protocol)
}

// Build creates the Service. Like the other builders, the returned object shares no map or slice
// with the builder, so mutating it (or building twice) cannot corrupt the builder's state.
func (b *ServiceBuilder) Build() *corev1.Service {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        b.Name,
			Namespace:   b.Namespace,
			Labels:      maps.Clone(b.Labels),
			Annotations: maps.Clone(b.Annotations),
		},
		Spec: corev1.ServiceSpec{
			Selector:                 maps.Clone(b.Selector),
			Ports:                    cloneSlice(b.Ports),
			Type:                     b.Type,
			PublishNotReadyAddresses: b.PublishNotReadyAddresses,
		},
	}

	if b.Headless {
		svc.Spec.ClusterIP = corev1.ClusterIPNone
	}

	return svc
}

// BuildHeadless creates a headless service.
func (b *ServiceBuilder) BuildHeadless() *corev1.Service {
	b.Headless = true
	return b.Build()
}

// NamespacedName returns the NamespacedName for the Service.
func (b *ServiceBuilder) NamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      b.Name,
		Namespace: b.Namespace,
	}
}

// HeadlessServiceBuilder creates a headless service for StatefulSets.
type HeadlessServiceBuilder struct {
	*ServiceBuilder
}

// NewHeadlessServiceBuilder creates a new HeadlessServiceBuilder.
func NewHeadlessServiceBuilder(name, namespace string) *HeadlessServiceBuilder {
	return &HeadlessServiceBuilder{
		ServiceBuilder: NewServiceBuilder(name, namespace).WithServiceType(ServiceTypeHeadless),
	}
}

// Build creates the headless service.
func (b *HeadlessServiceBuilder) Build() *corev1.Service {
	return b.ServiceBuilder.Build()
}
