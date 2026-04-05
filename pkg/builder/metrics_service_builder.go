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
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// MetricsServiceBuilder constructs a headless Service with Prometheus scrape annotations.
// Conventions applied automatically:
//   - Service name: "{resourceName}-metrics"
//   - ClusterIP: None (headless)
//   - Prometheus annotations: scrape=true, port, scheme=http
//   - Selector: same as labels
//
// Override defaults with WithScheme() and WithPath().
type MetricsServiceBuilder struct {
	resourceName string
	namespace    string
	port         int32
	portName     string
	labels       map[string]string
	scheme       string
	path         string
}

// NewMetricsServiceBuilder creates a builder for a metrics headless service.
// resourceName is the role group resource name; "-metrics" suffix is appended automatically.
// port is the metrics port number.
// labels are used for both service labels and selector.
func NewMetricsServiceBuilder(resourceName, namespace string, port int32, labels map[string]string) *MetricsServiceBuilder {
	return &MetricsServiceBuilder{
		resourceName: resourceName,
		namespace:    namespace,
		port:         port,
		portName:     "metrics",
		labels:       labels,
		scheme:       "http",
		path:         "", // empty = default /metrics
	}
}

// WithScheme sets the Prometheus scrape scheme (default: "http").
func (b *MetricsServiceBuilder) WithScheme(scheme string) *MetricsServiceBuilder {
	b.scheme = scheme
	return b
}

// WithPath sets the Prometheus metrics path (default: "" which means /metrics).
func (b *MetricsServiceBuilder) WithPath(path string) *MetricsServiceBuilder {
	b.path = path
	return b
}

// WithPortName sets the service port name (default: "metrics").
func (b *MetricsServiceBuilder) WithPortName(name string) *MetricsServiceBuilder {
	b.portName = name
	return b
}

// Build creates the metrics Service.
func (b *MetricsServiceBuilder) Build() *corev1.Service {
	serviceLabels := maps.Clone(b.labels)
	serviceLabels["prometheus.io/scrape"] = "true"

	annotations := map[string]string{
		"prometheus.io/scrape": "true",
		"prometheus.io/port":   strconv.Itoa(int(b.port)),
		"prometheus.io/scheme": b.scheme,
	}
	if b.path != "" {
		annotations["prometheus.io/path"] = b.path
	}

	selector := maps.Clone(b.labels)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        b.resourceName + "-metrics",
			Namespace:   b.namespace,
			Labels:      serviceLabels,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			Selector:  selector,
			Ports: []corev1.ServicePort{
				{
					Name:       b.portName,
					Port:       b.port,
					TargetPort: intstr.FromInt(int(b.port)),
				},
			},
		},
	}
}
