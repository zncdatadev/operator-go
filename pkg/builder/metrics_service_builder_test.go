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

package builder_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/builder"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("MetricsServiceBuilder", func() {
	const (
		resourceName = "test-cluster-default"
		namespace    = "test-ns"
		port         = int32(9505)
	)

	var labels map[string]string

	BeforeEach(func() {
		labels = map[string]string{
			"app.kubernetes.io/name":      "test-cluster",
			"app.kubernetes.io/component": "default",
		}
	})

	Describe("NewMetricsServiceBuilder", func() {
		It("should create a builder with default values", func() {
			b := builder.NewMetricsServiceBuilder(resourceName, namespace, port, labels)
			Expect(b).NotTo(BeNil())
		})
	})

	Describe("Build", func() {
		It("should build a headless metrics service with defaults", func() {
			svc := builder.NewMetricsServiceBuilder(resourceName, namespace, port, labels).Build()

			Expect(svc.Name).To(Equal("test-cluster-default-metrics"))
			Expect(svc.Namespace).To(Equal(namespace))
			Expect(svc.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))

			// Labels include prometheus scrape
			Expect(svc.Labels).To(HaveKeyWithValue("prometheus.io/scrape", "true"))
			Expect(svc.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "test-cluster"))

			// Prometheus annotations
			Expect(svc.Annotations).To(HaveKeyWithValue("prometheus.io/scrape", "true"))
			Expect(svc.Annotations).To(HaveKeyWithValue("prometheus.io/port", "9505"))
			Expect(svc.Annotations).To(HaveKeyWithValue("prometheus.io/scheme", "http"))
			Expect(svc.Annotations).NotTo(HaveKey("prometheus.io/path"))

			// Selector matches input labels
			Expect(svc.Spec.Selector).To(HaveKeyWithValue("app.kubernetes.io/name", "test-cluster"))
			Expect(svc.Spec.Selector).To(HaveKeyWithValue("app.kubernetes.io/component", "default"))
			Expect(svc.Spec.Selector).NotTo(HaveKey("prometheus.io/scrape"))

			// Port
			Expect(svc.Spec.Ports).To(HaveLen(1))
			Expect(svc.Spec.Ports[0].Name).To(Equal("metrics"))
			Expect(svc.Spec.Ports[0].Port).To(Equal(port))
			Expect(svc.Spec.Ports[0].TargetPort).To(Equal(intstr.FromInt(int(port))))
		})

		It("should not mutate the original labels map", func() {
			svc := builder.NewMetricsServiceBuilder(resourceName, namespace, port, labels).Build()

			Expect(labels).NotTo(HaveKey("prometheus.io/scrape"))
			Expect(svc.Labels).To(HaveKeyWithValue("prometheus.io/scrape", "true"))
		})

		It("should not follow the caller's map after construction", func() {
			b := builder.NewMetricsServiceBuilder(resourceName, namespace, port, labels)
			labels["app.kubernetes.io/name"] = "reused-for-another-role"

			svc := b.Build()
			Expect(svc.Spec.Selector).To(HaveKeyWithValue("app.kubernetes.io/name", "test-cluster"))
		})
	})

	Describe("WithScheme", func() {
		It("should set the prometheus scrape scheme", func() {
			svc := builder.NewMetricsServiceBuilder(resourceName, namespace, port, labels).
				WithScheme("https").
				Build()

			Expect(svc.Annotations).To(HaveKeyWithValue("prometheus.io/scheme", "https"))
		})
	})

	Describe("WithPath", func() {
		It("should set the prometheus metrics path", func() {
			svc := builder.NewMetricsServiceBuilder(resourceName, namespace, port, labels).
				WithPath("/prom").
				Build()

			Expect(svc.Annotations).To(HaveKeyWithValue("prometheus.io/path", "/prom"))
		})

		It("should not set path annotation when path is empty", func() {
			svc := builder.NewMetricsServiceBuilder(resourceName, namespace, port, labels).Build()

			Expect(svc.Annotations).NotTo(HaveKey("prometheus.io/path"))
		})
	})

	Describe("WithPortName", func() {
		It("should set the service port name", func() {
			svc := builder.NewMetricsServiceBuilder(resourceName, namespace, port, labels).
				WithPortName("jmx-metrics").
				Build()

			Expect(svc.Spec.Ports[0].Name).To(Equal("jmx-metrics"))
		})

		It("should not change the numeric targetPort", func() {
			svc := builder.NewMetricsServiceBuilder(resourceName, namespace, port, labels).
				WithPortName("jmx-metrics").
				Build()

			Expect(svc.Spec.Ports[0].TargetPort).To(Equal(intstr.FromInt(int(port))))
		})
	})

	Describe("WithTargetPortName", func() {
		It("should target the container port by name", func() {
			svc := builder.NewMetricsServiceBuilder(resourceName, namespace, port, labels).
				WithTargetPortName("metrics").
				Build()

			Expect(svc.Spec.Ports[0].TargetPort).To(Equal(intstr.FromString("metrics")))
		})

		It("should keep the numeric port and default port name", func() {
			svc := builder.NewMetricsServiceBuilder(resourceName, namespace, port, labels).
				WithTargetPortName("metrics").
				Build()

			Expect(svc.Spec.Ports[0].Name).To(Equal("metrics"))
			Expect(svc.Spec.Ports[0].Port).To(Equal(port))
			Expect(svc.Annotations).To(HaveKeyWithValue("prometheus.io/port", "9505"))
		})

		It("should combine with WithPortName independently", func() {
			svc := builder.NewMetricsServiceBuilder(resourceName, namespace, port, labels).
				WithPortName("jmx-metrics").
				WithTargetPortName("jmx").
				Build()

			Expect(svc.Spec.Ports[0].Name).To(Equal("jmx-metrics"))
			Expect(svc.Spec.Ports[0].TargetPort).To(Equal(intstr.FromString("jmx")))
		})
	})

	Describe("WithAnnotations", func() {
		It("should merge extra annotations into the generated Prometheus set", func() {
			svc := builder.NewMetricsServiceBuilder(resourceName, namespace, port, labels).
				WithAnnotations(map[string]string{"example.com/team": "data"}).
				Build()

			Expect(svc.Annotations).To(HaveKeyWithValue("example.com/team", "data"))
			// Merged, not replaced: the generated set has to survive alongside the caller's.
			Expect(svc.Annotations).To(HaveKeyWithValue("prometheus.io/scrape", "true"))
			Expect(svc.Annotations).To(HaveKeyWithValue("prometheus.io/port", "9505"))
			Expect(svc.Annotations).To(HaveKeyWithValue("prometheus.io/scheme", "http"))
		})

		It("should let a caller entry win over the generated default for the same key", func() {
			svc := builder.NewMetricsServiceBuilder(resourceName, namespace, port, labels).
				WithPath("/generated").
				WithAnnotations(map[string]string{
					"prometheus.io/path": "/override",
					"prometheus.io/port": "19505",
				}).
				Build()

			// Merging in the other order would silently discard the override, which is the whole
			// reason this method exists — a product cannot otherwise correct a generated value.
			Expect(svc.Annotations).To(HaveKeyWithValue("prometheus.io/path", "/override"))
			Expect(svc.Annotations).To(HaveKeyWithValue("prometheus.io/port", "19505"))
		})

		It("should accumulate across calls", func() {
			svc := builder.NewMetricsServiceBuilder(resourceName, namespace, port, labels).
				WithAnnotations(map[string]string{"a": "1"}).
				WithAnnotations(map[string]string{"b": "2"}).
				Build()

			Expect(svc.Annotations).To(HaveKeyWithValue("a", "1"))
			Expect(svc.Annotations).To(HaveKeyWithValue("b", "2"))
		})

		It("should copy the caller's map rather than alias it", func() {
			caller := map[string]string{"a": "1"}
			b := builder.NewMetricsServiceBuilder(resourceName, namespace, port, labels).WithAnnotations(caller)
			caller["a"] = "mutated"
			caller["late"] = "entry"

			svc := b.Build()
			Expect(svc.Annotations).To(HaveKeyWithValue("a", "1"))
			Expect(svc.Annotations).NotTo(HaveKey("late"))
		})

		It("should leave the generated set untouched when never called", func() {
			svc := builder.NewMetricsServiceBuilder(resourceName, namespace, port, labels).Build()

			Expect(svc.Annotations).To(HaveLen(3))
			Expect(svc.Name).To(Equal(resourceName + "-metrics"))
		})
	})
})
