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
//   - Service labels: input labels + "prometheus.io/scrape=true"
//   - Selector: input labels (without prometheus annotation)
//
// Override defaults with WithScheme() and WithPath().
type MetricsServiceBuilder struct {
	resourceName   string
	namespace      string
	port           int32
	portName       string
	targetPortName string
	labels         map[string]string
	selector       map[string]string
	scheme         string
	path           string
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

// WithTargetPortName targets the container port by name instead of by number.
// By default the Service targets the numeric port; opting into a named
// targetPort keeps the Service valid if the container port number changes,
// as long as the container declares a port with the given name.
func (b *MetricsServiceBuilder) WithTargetPortName(name string) *MetricsServiceBuilder {
	b.targetPortName = name
	return b
}

// WithSelector sets a dedicated pod selector (default: the labels). Use this to decouple the
// selector from the descriptive labels.
func (b *MetricsServiceBuilder) WithSelector(selector map[string]string) *MetricsServiceBuilder {
	b.selector = selector
	return b
}

// Build creates the metrics Service.
func (b *MetricsServiceBuilder) Build() *corev1.Service {
	serviceLabels := maps.Clone(b.labels)
	if serviceLabels == nil {
		serviceLabels = map[string]string{}
	}
	serviceLabels["prometheus.io/scrape"] = "true"

	annotations := map[string]string{
		"prometheus.io/scrape": "true",
		"prometheus.io/port":   strconv.Itoa(int(b.port)),
		"prometheus.io/scheme": b.scheme,
	}
	if b.path != "" {
		annotations["prometheus.io/path"] = b.path
	}

	selectorSource := b.selector
	if selectorSource == nil {
		selectorSource = b.labels
	}
	selector := maps.Clone(selectorSource)
	if selector == nil {
		selector = map[string]string{}
	}

	targetPort := intstr.FromInt(int(b.port))
	if b.targetPortName != "" {
		targetPort = intstr.FromString(b.targetPortName)
	}

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
					TargetPort: targetPort,
				},
			},
		},
	}
}
