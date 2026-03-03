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

package listener_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/listener"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("ListenerServiceBuilder", func() {
	Describe("NewListenerServiceBuilder", func() {
		It("should create a new builder with required parameters", func() {
			builder := listener.NewListenerServiceBuilder(
				"my-service",
				"default",
				listener.ListenerClassClusterInternal,
			)
			Expect(builder).NotTo(BeNil())
		})
	})

	Describe("WithPorts", func() {
		It("should set the ports", func() {
			ports := []corev1.ServicePort{
				{Name: "http", Port: 8080},
				{Name: "https", Port: 8443},
			}
			builder := listener.NewListenerServiceBuilder(
				"my-service",
				"default",
				listener.ListenerClassClusterInternal,
			).WithPorts(ports)

			Expect(builder).NotTo(BeNil())
		})
	})

	Describe("Build", func() {
		Context("with ClusterInternal listener class", func() {
			It("should create ClusterIP service", func() {
				builder := listener.NewListenerServiceBuilder(
					"my-service",
					"default",
					listener.ListenerClassClusterInternal,
				)
				svc := builder.Build()

				Expect(svc.Name).To(Equal("my-service"))
				Expect(svc.Namespace).To(Equal("default"))
				Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			})
		})

		Context("with ExternalStable listener class", func() {
			It("should create LoadBalancer service", func() {
				builder := listener.NewListenerServiceBuilder(
					"my-service",
					"production",
					listener.ListenerClassExternalStable,
				)
				svc := builder.Build()

				Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeLoadBalancer))
			})
		})

		Context("with ExternalUnstable listener class", func() {
			It("should create LoadBalancer service", func() {
				builder := listener.NewListenerServiceBuilder(
					"my-service",
					"default",
					listener.ListenerClassExternalUnstable,
				)
				svc := builder.Build()

				Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeLoadBalancer))
			})
		})

		Context("with unknown listener class", func() {
			It("should default to ClusterIP service", func() {
				builder := listener.NewListenerServiceBuilder(
					"my-service",
					"default",
					listener.ListenerClass("unknown"),
				)
				svc := builder.Build()

				Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			})
		})

		It("should set listener class annotation", func() {
			builder := listener.NewListenerServiceBuilder(
				"my-service",
				"default",
				listener.ListenerClassClusterInternal,
			)
			svc := builder.Build()

			Expect(svc.Annotations).To(HaveKeyWithValue(
				listener.ListenerClassAnnotation,
				"cluster-internal",
			))
		})

		It("should set ports when provided", func() {
			ports := []corev1.ServicePort{
				{Name: "http", Port: 8080, Protocol: corev1.ProtocolTCP},
				{Name: "metrics", Port: 9090, Protocol: corev1.ProtocolTCP},
			}
			builder := listener.NewListenerServiceBuilder(
				"my-service",
				"default",
				listener.ListenerClassClusterInternal,
			).WithPorts(ports)
			svc := builder.Build()

			Expect(svc.Spec.Ports).To(HaveLen(2))
			Expect(svc.Spec.Ports[0].Name).To(Equal("http"))
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(8080)))
			Expect(svc.Spec.Ports[1].Name).To(Equal("metrics"))
			Expect(svc.Spec.Ports[1].Port).To(Equal(int32(9090)))
		})

		It("should have empty ports when not provided", func() {
			builder := listener.NewListenerServiceBuilder(
				"my-service",
				"default",
				listener.ListenerClassClusterInternal,
			)
			svc := builder.Build()

			Expect(svc.Spec.Ports).To(BeEmpty())
		})
	})
})
