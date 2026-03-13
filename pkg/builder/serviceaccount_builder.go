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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServiceAccountBuilder builds a Kubernetes ServiceAccount.
type ServiceAccountBuilder struct {
	Name        string
	Namespace   string
	Labels      map[string]string
	Annotations map[string]string
}

// NewServiceAccountBuilder creates a new ServiceAccountBuilder.
func NewServiceAccountBuilder(name, namespace string) *ServiceAccountBuilder {
	return &ServiceAccountBuilder{
		Name:      name,
		Namespace: namespace,
	}
}

// WithLabels sets the labels on the ServiceAccount.
func (b *ServiceAccountBuilder) WithLabels(labels map[string]string) *ServiceAccountBuilder {
	b.Labels = labels
	return b
}

// WithAnnotations sets annotations on the ServiceAccount.
func (b *ServiceAccountBuilder) WithAnnotations(annotations map[string]string) *ServiceAccountBuilder {
	b.Annotations = annotations
	return b
}

// Build constructs and returns the ServiceAccount.
func (b *ServiceAccountBuilder) Build() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        b.Name,
			Namespace:   b.Namespace,
			Labels:      b.Labels,
			Annotations: b.Annotations,
		},
	}
}
