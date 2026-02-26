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

var _ = Describe("ServiceBuilder", func() {
	const (
		name      = "test-svc"
		namespace = "test-namespace"
	)

	var svcBuilder *builder.ServiceBuilder

	BeforeEach(func() {
		svcBuilder = builder.NewServiceBuilder(name, namespace)
	})

	Describe("NewServiceBuilder", func() {
		It("should create a builder with default values", func() {
			Expect(svcBuilder.Name).To(Equal(name))
			Expect(svcBuilder.Namespace).To(Equal(namespace))
			Expect(svcBuilder.Type).To(Equal(corev1.ServiceTypeClusterIP))
		})
	})

	Describe("WithLabels", func() {
		It("should add labels to the builder", func() {
			labels := map[string]string{"app": "test"}
			result := svcBuilder.WithLabels(labels)

			Expect(result).To(Equal(svcBuilder))
			Expect(svcBuilder.Labels).To(HaveKeyWithValue("app", "test"))
		})
	})

	Describe("WithAnnotations", func() {
		It("should add annotations to the builder", func() {
			annotations := map[string]string{"description": "test"}
			result := svcBuilder.WithAnnotations(annotations)

			Expect(result).To(Equal(svcBuilder))
			Expect(svcBuilder.Annotations).To(HaveKeyWithValue("description", "test"))
		})
	})

	Describe("WithServiceType", func() {
		It("should set the service type to NodePort", func() {
			result := svcBuilder.WithServiceType(builder.ServiceTypeNodePort)

			Expect(result).To(Equal(svcBuilder))
			Expect(svcBuilder.Type).To(Equal(corev1.ServiceTypeNodePort))
		})

		It("should set headless to true for headless type", func() {
			result := svcBuilder.WithServiceType(builder.ServiceTypeHeadless)

			Expect(result).To(Equal(svcBuilder))
			Expect(svcBuilder.Headless).To(BeTrue())
		})
	})

	Describe("WithSelector", func() {
		It("should set the selector", func() {
			selector := map[string]string{"app": "test-app"}
			result := svcBuilder.WithSelector(selector)

			Expect(result).To(Equal(svcBuilder))
			Expect(svcBuilder.Selector).To(HaveKeyWithValue("app", "test-app"))
		})
	})

	Describe("AddPort", func() {
		It("should add a port to the service", func() {
			result := svcBuilder.AddPort("http", 8080, intstr.FromInt(80), corev1.ProtocolTCP)

			Expect(result).To(Equal(svcBuilder))
			Expect(svcBuilder.Ports).To(HaveLen(1))
			Expect(svcBuilder.Ports[0].Name).To(Equal("http"))
			Expect(svcBuilder.Ports[0].Port).To(Equal(int32(8080)))
		})
	})

	Describe("AddPortSimple", func() {
		It("should add a simple port where port equals target port", func() {
			result := svcBuilder.AddPortSimple("http", 8080, corev1.ProtocolTCP)

			Expect(result).To(Equal(svcBuilder))
			Expect(svcBuilder.Ports).To(HaveLen(1))
			Expect(svcBuilder.Ports[0].Port).To(Equal(int32(8080)))
			Expect(svcBuilder.Ports[0].TargetPort.IntValue()).To(Equal(8080))
		})
	})

	Describe("Build", func() {
		It("should build a valid Service", func() {
			svc := svcBuilder.
				WithLabels(map[string]string{"app": "test"}).
				WithSelector(map[string]string{"app": "test-app"}).
				AddPortSimple("http", 8080, corev1.ProtocolTCP).
				Build()

			Expect(svc).NotTo(BeNil())
			Expect(svc.Name).To(Equal(name))
			Expect(svc.Namespace).To(Equal(namespace))
			Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(svc.Spec.Ports).To(HaveLen(1))
			Expect(svc.Spec.Selector).To(HaveKeyWithValue("app", "test-app"))
		})

		It("should build a headless service when Headless is true", func() {
			svc := svcBuilder.
				WithServiceType(builder.ServiceTypeHeadless).
				Build()

			Expect(svc.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
		})
	})

	Describe("BuildHeadless", func() {
		It("should build a headless service", func() {
			svc := builder.NewServiceBuilder(name, namespace).
				WithSelector(map[string]string{"app": "test"}).
				BuildHeadless()

			Expect(svc.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
		})
	})

	Describe("NamespacedName", func() {
		It("should return the correct NamespacedName", func() {
			nn := svcBuilder.NamespacedName()

			Expect(nn.Name).To(Equal(name))
			Expect(nn.Namespace).To(Equal(namespace))
		})
	})
})

var _ = Describe("HeadlessServiceBuilder", func() {
	const (
		name      = "test-headless-svc"
		namespace = "test-namespace"
	)

	It("should create a headless service by default", func() {
		builder := builder.NewHeadlessServiceBuilder(name, namespace)
		Expect(builder.Headless).To(BeTrue())
	})
})
