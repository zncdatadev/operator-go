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

var _ = Describe("ListenerVolumeBuilder", func() {
	Describe("NewListenerVolumeBuilder", func() {
		It("should create a new builder with listener class", func() {
			builder := listener.NewListenerVolumeBuilder(listener.ListenerClassClusterInternal)
			Expect(builder).NotTo(BeNil())
		})
	})

	Describe("WithScope", func() {
		It("should set the scope", func() {
			builder := listener.NewListenerVolumeBuilder(listener.ListenerClassClusterInternal).
				WithScope("pod")

			Expect(builder).NotTo(BeNil())
		})
	})

	Describe("BuildPVC", func() {
		It("should create PVC with cluster-internal class annotation", func() {
			builder := listener.NewListenerVolumeBuilder(listener.ListenerClassClusterInternal)
			pvc := builder.BuildPVC("my-listener")

			Expect(pvc.Name).To(Equal("my-listener"))
			Expect(pvc.Annotations).To(HaveKeyWithValue(
				listener.ListenerClassAnnotation,
				"cluster-internal",
			))
		})

		It("should create PVC with external-stable class annotation", func() {
			builder := listener.NewListenerVolumeBuilder(listener.ListenerClassExternalStable)
			pvc := builder.BuildPVC("my-listener")

			Expect(pvc.Annotations).To(HaveKeyWithValue(
				listener.ListenerClassAnnotation,
				"external-stable",
			))
		})

		It("should create PVC with external-unstable class annotation", func() {
			builder := listener.NewListenerVolumeBuilder(listener.ListenerClassExternalUnstable)
			pvc := builder.BuildPVC("my-listener")

			Expect(pvc.Annotations).To(HaveKeyWithValue(
				listener.ListenerClassAnnotation,
				"external-unstable",
			))
		})

		It("should include scope annotation when set", func() {
			builder := listener.NewListenerVolumeBuilder(listener.ListenerClassClusterInternal).
				WithScope("service")
			pvc := builder.BuildPVC("my-listener")

			Expect(pvc.Annotations).To(HaveKeyWithValue(
				listener.ListenerScopeAnnotation,
				"service",
			))
		})

		It("should not include scope annotation when not set", func() {
			builder := listener.NewListenerVolumeBuilder(listener.ListenerClassClusterInternal)
			pvc := builder.BuildPVC("my-listener")

			Expect(pvc.Annotations).NotTo(HaveKey(listener.ListenerScopeAnnotation))
		})

		It("should set ReadWriteOnce access mode", func() {
			builder := listener.NewListenerVolumeBuilder(listener.ListenerClassClusterInternal)
			pvc := builder.BuildPVC("my-listener")

			Expect(pvc.Spec.AccessModes).To(ContainElement(corev1.ReadWriteOnce))
		})

		It("should set storage request of 1Mi", func() {
			builder := listener.NewListenerVolumeBuilder(listener.ListenerClassClusterInternal)
			pvc := builder.BuildPVC("my-listener")

			storage := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
			Expect(storage.String()).To(Equal("1Mi"))
		})
	})

	Describe("BuildVolumeMount", func() {
		It("should create volume mount with correct settings", func() {
			builder := listener.NewListenerVolumeBuilder(listener.ListenerClassClusterInternal)
			mount := builder.BuildVolumeMount("listener-volume", "/mnt/listener")

			Expect(mount.Name).To(Equal("listener-volume"))
			Expect(mount.MountPath).To(Equal("/mnt/listener"))
			Expect(mount.ReadOnly).To(BeTrue())
		})
	})
})

var _ = Describe("ListenerClass constants", func() {
	It("should have correct CSI driver name", func() {
		Expect(listener.CSIDriverName).To(Equal("listeners.stackable.tech"))
	})

	It("should have correct annotation names", func() {
		Expect(listener.ListenerClassAnnotation).To(Equal("listeners.stackable.tech/class"))
		Expect(listener.ListenerScopeAnnotation).To(Equal("listeners.stackable.tech/scope"))
	})

	It("should have correct listener class values", func() {
		Expect(string(listener.ListenerClassClusterInternal)).To(Equal("cluster-internal"))
		Expect(string(listener.ListenerClassExternalStable)).To(Equal("external-stable"))
		Expect(string(listener.ListenerClassExternalUnstable)).To(Equal("external-unstable"))
	})
})
