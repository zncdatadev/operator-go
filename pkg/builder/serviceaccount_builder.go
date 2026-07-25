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
		Name:        name,
		Namespace:   namespace,
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
	}
}

// WithLabels merges the given labels into the builder-owned label map, like every other builder's
// WithLabels: repeated calls accumulate and the caller's map is never aliased.
func (b *ServiceAccountBuilder) WithLabels(labels map[string]string) *ServiceAccountBuilder {
	for k, v := range labels {
		b.Labels[k] = v
	}
	return b
}

// WithAnnotations merges the given annotations into the builder-owned annotation map.
func (b *ServiceAccountBuilder) WithAnnotations(annotations map[string]string) *ServiceAccountBuilder {
	for k, v := range annotations {
		b.Annotations[k] = v
	}
	return b
}

// Build constructs and returns the ServiceAccount.
func (b *ServiceAccountBuilder) Build() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        b.Name,
			Namespace:   b.Namespace,
			Labels:      maps.Clone(b.Labels),
			Annotations: maps.Clone(b.Annotations),
		},
	}
}

// NamespacedName returns the NamespacedName for the ServiceAccount.
func (b *ServiceAccountBuilder) NamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      b.Name,
		Namespace: b.Namespace,
	}
}
