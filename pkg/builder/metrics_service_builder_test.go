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
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestMetricsServiceBuilder_Defaults(t *testing.T) {
	labels := map[string]string{
		"app.kubernetes.io/name":      "test-cluster",
		"app.kubernetes.io/component": "default",
	}
	svc := NewMetricsServiceBuilder("test-cluster-default", "test-ns", 9505, labels).Build()

	assert.Equal(t, "test-cluster-default-metrics", svc.Name)
	assert.Equal(t, "test-ns", svc.Namespace)
	assert.Equal(t, corev1.ClusterIPNone, svc.Spec.ClusterIP)

	// Labels include prometheus scrape
	assert.Equal(t, "true", svc.Labels["prometheus.io/scrape"])
	assert.Equal(t, "test-cluster", svc.Labels["app.kubernetes.io/name"])

	// Prometheus annotations
	assert.Equal(t, "true", svc.Annotations["prometheus.io/scrape"])
	assert.Equal(t, "9505", svc.Annotations["prometheus.io/port"])
	assert.Equal(t, "http", svc.Annotations["prometheus.io/scheme"])
	assert.NotContains(t, svc.Annotations, "prometheus.io/path") // default: no path override

	// Selector matches input labels
	assert.Equal(t, "test-cluster", svc.Spec.Selector["app.kubernetes.io/name"])
	assert.Equal(t, "default", svc.Spec.Selector["app.kubernetes.io/component"])
	// Selector should NOT include prometheus label
	assert.NotContains(t, svc.Spec.Selector, "prometheus.io/scrape")

	// Port
	assert.Len(t, svc.Spec.Ports, 1)
	assert.Equal(t, "metrics", svc.Spec.Ports[0].Name)
	assert.Equal(t, int32(9505), svc.Spec.Ports[0].Port)
}

func TestMetricsServiceBuilder_WithScheme(t *testing.T) {
	labels := map[string]string{"app": "test"}
	svc := NewMetricsServiceBuilder("test", "ns", 9505, labels).
		WithScheme("https").
		Build()

	assert.Equal(t, "https", svc.Annotations["prometheus.io/scheme"])
}

func TestMetricsServiceBuilder_WithPath(t *testing.T) {
	labels := map[string]string{"app": "test"}
	svc := NewMetricsServiceBuilder("test", "ns", 9505, labels).
		WithPath("/prom").
		Build()

	assert.Equal(t, "/prom", svc.Annotations["prometheus.io/path"])
}

func TestMetricsServiceBuilder_WithPathEmpty(t *testing.T) {
	labels := map[string]string{"app": "test"}
	svc := NewMetricsServiceBuilder("test", "ns", 9505, labels).
		Build()

	// Empty path means default /metrics, don't set annotation
	assert.NotContains(t, svc.Annotations, "prometheus.io/path")
}

func TestMetricsServiceBuilder_WithPortName(t *testing.T) {
	labels := map[string]string{"app": "test"}
	svc := NewMetricsServiceBuilder("test", "ns", 9505, labels).
		WithPortName("jmx-metrics").
		Build()

	assert.Equal(t, "jmx-metrics", svc.Spec.Ports[0].Name)
}

func TestMetricsServiceBuilder_LabelsNotMutated(t *testing.T) {
	labels := map[string]string{"app": "test"}
	svc := NewMetricsServiceBuilder("test", "ns", 9505, labels).Build()

	// Original labels map should not be mutated
	assert.NotContains(t, labels, "prometheus.io/scrape")
	// But service labels should have it
	assert.Equal(t, "true", svc.Labels["prometheus.io/scrape"])
}
