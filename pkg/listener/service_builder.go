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

package listener

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ListenerServiceBuilder builds Services for listener exposure.
type ListenerServiceBuilder struct {
	name          string
	namespace     string
	listenerClass ListenerClass
	ports         []corev1.ServicePort
}

// NewListenerServiceBuilder creates a new ListenerServiceBuilder.
func NewListenerServiceBuilder(name, namespace string, listenerClass ListenerClass) *ListenerServiceBuilder {
	return &ListenerServiceBuilder{
		name:          name,
		namespace:     namespace,
		listenerClass: listenerClass,
	}
}

// WithPorts sets the service ports.
func (b *ListenerServiceBuilder) WithPorts(ports []corev1.ServicePort) *ListenerServiceBuilder {
	b.ports = ports
	return b
}

// Build creates the Service.
func (b *ListenerServiceBuilder) Build() *corev1.Service {
	serviceType := corev1.ServiceTypeClusterIP

	// Map listener class to service type
	switch b.listenerClass {
	case ListenerClassExternalStable, ListenerClassExternalUnstable:
		serviceType = corev1.ServiceTypeLoadBalancer
	case ListenerClassClusterInternal:
		fallthrough
	default:
		serviceType = corev1.ServiceTypeClusterIP
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.name,
			Namespace: b.namespace,
			Annotations: map[string]string{
				ListenerClassAnnotation: string(b.listenerClass),
			},
		},
		Spec: corev1.ServiceSpec{
			Type:  serviceType,
			Ports: b.ports,
		},
	}
}
