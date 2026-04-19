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
	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/listener"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

var _ = Describe("ListenerProvisioner", func() {
	var provisioner *listener.ListenerProvisioner

	BeforeEach(func() {
		provisioner = listener.NewProvisioner()
	})

	Describe("ServiceRegistration", func() {
		var testPort corev1.ServicePort

		BeforeEach(func() {
			testPort = corev1.ServicePort{
				Name:     "http",
				Port:     8080,
				Protocol: corev1.ProtocolTCP,
			}
		})

		It("should create ClusterIP service for cluster-internal class", func() {
			provisioner.RegisterService(
				listener.NewService("test-svc", listener.ListenerClassClusterInternal).
					WithPorts(testPort),
			)
			services := provisioner.Services("default")
			Expect(services).To(HaveLen(1))
			Expect(services[0].Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(services[0].Name).To(Equal("test-svc"))
			Expect(services[0].Namespace).To(Equal("default"))
		})

		It("should create LoadBalancer service for external-stable class", func() {
			provisioner.RegisterService(
				listener.NewService("test-svc", listener.ListenerClassExternalStable).
					WithPorts(testPort),
			)
			services := provisioner.Services("default")
			Expect(services).To(HaveLen(1))
			Expect(services[0].Spec.Type).To(Equal(corev1.ServiceTypeLoadBalancer))
		})

		It("should create LoadBalancer service for external-unstable class", func() {
			provisioner.RegisterService(
				listener.NewService("test-svc", listener.ListenerClassExternalUnstable).
					WithPorts(testPort),
			)
			services := provisioner.Services("default")
			Expect(services).To(HaveLen(1))
			Expect(services[0].Spec.Type).To(Equal(corev1.ServiceTypeLoadBalancer))
		})

		It("should default to ClusterIP for unknown class", func() {
			provisioner.RegisterService(
				listener.NewService("test-svc", listener.ListenerClass("unknown")).
					WithPorts(testPort),
			)
			services := provisioner.Services("default")
			Expect(services).To(HaveLen(1))
			Expect(services[0].Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
		})

		It("should set listener class annotation on service", func() {
			provisioner.RegisterService(
				listener.NewService("test-svc", listener.ListenerClassClusterInternal),
			)
			services := provisioner.Services("default")
			Expect(services[0].Annotations).To(HaveKeyWithValue(
				listener.ListenerClassAnnotation,
				"cluster-internal",
			))
		})

		It("should propagate ports to service", func() {
			ports := []corev1.ServicePort{
				{Name: "http", Port: 8080},
				{Name: "https", Port: 8443},
			}
			provisioner.RegisterService(
				listener.NewService("test-svc", listener.ListenerClassClusterInternal).
					WithPorts(ports...),
			)
			services := provisioner.Services("default")
			Expect(services[0].Spec.Ports).To(HaveLen(2))
			Expect(services[0].Spec.Ports[0].Name).To(Equal("http"))
			Expect(services[0].Spec.Ports[1].Name).To(Equal("https"))
		})

		It("should produce service without ports when none set", func() {
			provisioner.RegisterService(
				listener.NewService("test-svc", listener.ListenerClassClusterInternal),
			)
			services := provisioner.Services("default")
			Expect(services[0].Spec.Ports).To(BeEmpty())
		})
	})

	Describe("ServiceRegistration headless", func() {
		It("should produce two services when headless is enabled", func() {
			provisioner.RegisterService(
				listener.NewService("zk", listener.ListenerClassClusterInternal).
					WithHeadless(true),
			)
			services := provisioner.Services("default")
			Expect(services).To(HaveLen(2))
		})

		It("should produce headless service with ClusterIP None", func() {
			provisioner.RegisterService(
				listener.NewService("zk", listener.ListenerClassClusterInternal).
					WithHeadless(true),
			)
			services := provisioner.Services("default")
			headlessSvc := services[1]
			Expect(headlessSvc.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
			Expect(headlessSvc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
		})

		It("should name headless service with -headless suffix", func() {
			provisioner.RegisterService(
				listener.NewService("zk", listener.ListenerClassClusterInternal).
					WithHeadless(true),
			)
			services := provisioner.Services("default")
			Expect(services[1].Name).To(Equal("zk-headless"))
		})

		It("should not set ListenerClassAnnotation on headless service", func() {
			provisioner.RegisterService(
				listener.NewService("zk", listener.ListenerClassClusterInternal).
					WithHeadless(true),
			)
			services := provisioner.Services("default")
			headlessSvc := services[1]
			Expect(headlessSvc.Annotations).NotTo(HaveKey(listener.ListenerClassAnnotation))
		})
	})

	Describe("VolumeRegistration", func() {
		It("should create volume with listener class annotation", func() {
			provisioner.RegisterVolume(
				listener.NewVolume("listener", listener.ListenerClassClusterInternal),
			)
			volumes := provisioner.Volumes()
			Expect(volumes).To(HaveLen(1))
			Expect(volumes[0].Name).To(Equal("listener"))
			Expect(volumes[0].Ephemeral).NotTo(BeNil())
		})

		It("should set class annotation on PVC template", func() {
			provisioner.RegisterVolume(
				listener.NewVolume("listener", listener.ListenerClassExternalStable),
			)
			volumes := provisioner.Volumes()
			annotations := volumes[0].Ephemeral.VolumeClaimTemplate.Annotations
			Expect(annotations).To(HaveKeyWithValue(
				listener.ListenerClassAnnotation, "external-stable",
			))
		})

		It("should set scope annotation when WithScope is used", func() {
			provisioner.RegisterVolume(
				listener.NewVolume("listener", listener.ListenerClassClusterInternal).
					WithScope(listener.ListenerScopeCluster),
			)
			volumes := provisioner.Volumes()
			annotations := volumes[0].Ephemeral.VolumeClaimTemplate.Annotations
			Expect(annotations).To(HaveKeyWithValue(
				listener.ListenerScopeAnnotation, "Cluster",
			))
		})

		It("should not set scope annotation when WithScope is not used", func() {
			provisioner.RegisterVolume(
				listener.NewVolume("listener", listener.ListenerClassClusterInternal),
			)
			volumes := provisioner.Volumes()
			annotations := volumes[0].Ephemeral.VolumeClaimTemplate.Annotations
			Expect(annotations).NotTo(HaveKey(listener.ListenerScopeAnnotation))
		})

		It("should set listener name annotation when WithListenerName is used", func() {
			provisioner.RegisterVolume(
				listener.NewVolume("listener", listener.ListenerClassClusterInternal).
					WithListenerName("my-listener"),
			)
			volumes := provisioner.Volumes()
			annotations := volumes[0].Ephemeral.VolumeClaimTemplate.Annotations
			Expect(annotations).To(HaveKeyWithValue(
				listener.AnnotationListenerName, "my-listener",
			))
		})

		It("should use EphemeralVolumeSource with correct PVC spec", func() {
			provisioner.RegisterVolume(
				listener.NewVolume("listener", listener.ListenerClassClusterInternal),
			)
			volumes := provisioner.Volumes()
			pvcSpec := volumes[0].Ephemeral.VolumeClaimTemplate.Spec
			Expect(pvcSpec.AccessModes).To(ContainElement(corev1.ReadWriteOnce))
			Expect(*pvcSpec.StorageClassName).To(Equal(listener.ListenerStorageClass))
			Expect(*pvcSpec.VolumeMode).To(Equal(corev1.PersistentVolumeFilesystem))
		})

		It("should set storage request of 1Mi", func() {
			provisioner.RegisterVolume(
				listener.NewVolume("listener", listener.ListenerClassClusterInternal),
			)
			volumes := provisioner.Volumes()
			storage := volumes[0].Ephemeral.VolumeClaimTemplate.Spec.Resources.Requests
			expected := resource.MustParse("1Mi")
			Expect(storage[corev1.ResourceStorage]).To(Equal(expected))
		})
	})

	Describe("Provisioner registration", func() {
		It("should register and return multiple services", func() {
			provisioner.RegisterService(
				listener.NewService("svc1", listener.ListenerClassClusterInternal),
				listener.NewService("svc2", listener.ListenerClassExternalStable),
			)
			services := provisioner.Services("default")
			Expect(services).To(HaveLen(2))
		})

		It("should register and return multiple volumes", func() {
			provisioner.RegisterVolume(
				listener.NewVolume("vol1", listener.ListenerClassClusterInternal),
				listener.NewVolume("vol2", listener.ListenerClassExternalStable),
			)
			volumes := provisioner.Volumes()
			Expect(volumes).To(HaveLen(2))
		})

		It("should panic on duplicate service name", func() {
			Expect(func() {
				provisioner.RegisterService(
					listener.NewService("svc", listener.ListenerClassClusterInternal),
				)
				provisioner.RegisterService(
					listener.NewService("svc", listener.ListenerClassExternalStable),
				)
			}).To(Panic())
		})

		It("should panic when service name collides with headless name", func() {
			Expect(func() {
				provisioner.RegisterService(
					listener.NewService("zk", listener.ListenerClassClusterInternal),
				)
				provisioner.RegisterService(
					listener.NewService("zk", listener.ListenerClassClusterInternal).
						WithHeadless(true),
				)
			}).To(Panic())
		})

		It("should allow non-headless service with headless-suffix name after headless registration", func() {
			provisioner.RegisterService(
				listener.NewService("zk", listener.ListenerClassClusterInternal).
					WithHeadless(true),
			)
			// Registering "zk-headless" as a regular service should panic
			Expect(func() {
				provisioner.RegisterService(
					listener.NewService("zk-headless", listener.ListenerClassClusterInternal),
				)
			}).To(Panic())
		})

		It("should allow registering non-headless service when headless-suffix name is taken", func() {
			// Register "zk-headless" first as a regular service
			provisioner.RegisterService(
				listener.NewService("zk-headless", listener.ListenerClassClusterInternal),
			)
			// Then registering "zk" with headless=true should panic (headless name collision)
			Expect(func() {
				provisioner.RegisterService(
					listener.NewService("zk", listener.ListenerClassClusterInternal).
						WithHeadless(true),
				)
			}).To(Panic())
		})

		It("should panic on duplicate volume name", func() {
			Expect(func() {
				provisioner.RegisterVolume(
					listener.NewVolume("vol", listener.ListenerClassClusterInternal),
				)
				provisioner.RegisterVolume(
					listener.NewVolume("vol", listener.ListenerClassExternalStable),
				)
			}).To(Panic())
		})

		It("should return empty services when none registered", func() {
			Expect(provisioner.Services("default")).To(BeEmpty())
		})

		It("should return empty volumes when none registered", func() {
			Expect(provisioner.Volumes()).To(BeEmpty())
		})

		It("should panic when Services called with empty namespace", func() {
			provisioner.RegisterService(
				listener.NewService("svc", listener.ListenerClassClusterInternal),
			)
			Expect(func() {
				provisioner.Services("")
			}).To(Panic())
		})

		It("should panic when Services called with empty namespace even with no registrations", func() {
			Expect(func() {
				provisioner.Services("")
			}).To(Panic())
		})
	})

	Describe("VolumeMounts", func() {
		It("should return mounts with correct paths and ReadOnly", func() {
			provisioner.RegisterVolume(
				listener.NewVolume("listener", listener.ListenerClassClusterInternal),
			)
			mounts := provisioner.VolumeMounts()
			Expect(mounts).To(HaveLen(1))
			Expect(mounts[0].Name).To(Equal("listener"))
			Expect(mounts[0].MountPath).To(Equal("/kubedoop/listener/listener"))
			Expect(mounts[0].ReadOnly).To(BeTrue())
		})
	})

	Describe("WithMountBasePath", func() {
		It("should override the default mount base path", func() {
			custom := listener.NewProvisioner().WithMountBasePath("/custom")
			custom.RegisterVolume(
				listener.NewVolume("my-vol", listener.ListenerClassClusterInternal),
			)
			mounts := custom.VolumeMounts()
			Expect(mounts[0].MountPath).To(Equal("/custom/my-vol"))
		})

		It("should not override when given empty string", func() {
			p := listener.NewProvisioner().WithMountBasePath("")
			p.RegisterVolume(
				listener.NewVolume("my-vol", listener.ListenerClassClusterInternal),
			)
			mounts := p.VolumeMounts()
			Expect(mounts[0].MountPath).To(Equal("/kubedoop/listener/my-vol"))
		})
	})

	Describe("AutoInject", func() {
		It("should inject volumes and mounts into StatefulSetBuilder", func() {
			provisioner.RegisterVolume(
				listener.NewVolume("listener", listener.ListenerClassClusterInternal),
			)

			stsBuilder := builder.NewStatefulSetBuilder("test", "default")
			provisioner.AutoInject(stsBuilder)

			sts := stsBuilder.Build()
			Expect(sts.Spec.Template.Spec.Volumes).To(HaveLen(1))
			Expect(sts.Spec.Template.Spec.Volumes[0].Name).To(Equal("listener"))
		})

		It("should inject mounts into containers", func() {
			provisioner.RegisterVolume(
				listener.NewVolume("listener", listener.ListenerClassClusterInternal),
			)

			stsBuilder := builder.NewStatefulSetBuilder("test", "default")
			provisioner.AutoInject(stsBuilder)

			sts := stsBuilder.Build()
			Expect(sts.Spec.Template.Spec.Containers[0].VolumeMounts).NotTo(BeEmpty())
		})

		It("should not inject services into StatefulSetBuilder", func() {
			provisioner.RegisterService(
				listener.NewService("svc", listener.ListenerClassClusterInternal),
			)

			stsBuilder := builder.NewStatefulSetBuilder("test", "default")
			provisioner.AutoInject(stsBuilder)

			sts := stsBuilder.Build()
			Expect(sts.Spec.Template.Spec.Volumes).To(BeEmpty())
		})
	})

	Describe("Path API", func() {
		It("should return correct mount path for registered volume", func() {
			provisioner.RegisterVolume(
				listener.NewVolume("my-vol", listener.ListenerClassClusterInternal),
			)
			p, err := provisioner.Path("my-vol")
			Expect(err).NotTo(HaveOccurred())
			Expect(p).To(Equal("/kubedoop/listener/my-vol"))
		})

		It("should return error for unregistered volume", func() {
			_, err := provisioner.Path("nonexistent")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nonexistent"))
		})

		It("should return path for registered volume via MustPath", func() {
			provisioner.RegisterVolume(
				listener.NewVolume("my-vol", listener.ListenerClassClusterInternal),
			)
			Expect(provisioner.MustPath("my-vol")).To(Equal("/kubedoop/listener/my-vol"))
		})

		It("should panic for unregistered volume via MustPath", func() {
			Expect(func() {
				provisioner.MustPath("nonexistent")
			}).To(Panic())
		})

		It("should produce paths without double slashes", func() {
			provisioner.RegisterVolume(
				listener.NewVolume("test", listener.ListenerClassClusterInternal),
			)
			p, err := provisioner.Path("test")
			Expect(err).NotTo(HaveOccurred())
			Expect(p).NotTo(ContainSubstring("//"))
		})
	})

	Describe("Constants preservation", func() {
		It("should have correct CSI driver name", func() {
			Expect(listener.CSIDriverName).To(Equal("listeners.kubedoop.dev"))
		})

		It("should have correct listener class annotation", func() {
			Expect(listener.ListenerClassAnnotation).To(Equal("listeners.kubedoop.dev/class"))
		})

		It("should have correct scope annotation", func() {
			Expect(listener.ListenerScopeAnnotation).To(Equal("listeners.kubedoop.dev/scope"))
		})

		It("should have correct listener name annotation", func() {
			Expect(listener.AnnotationListenerName).To(Equal("listeners.kubedoop.dev/listenerName"))
		})

		It("should have correct cluster-internal class value", func() {
			Expect(listener.ListenerClassClusterInternal).To(Equal(listener.ListenerClass("cluster-internal")))
		})

		It("should have correct external-stable class value", func() {
			Expect(listener.ListenerClassExternalStable).To(Equal(listener.ListenerClass("external-stable")))
		})

		It("should have correct external-unstable class value", func() {
			Expect(listener.ListenerClassExternalUnstable).To(Equal(listener.ListenerClass("external-unstable")))
		})

		It("should preserve ListenerStorageClassPtr", func() {
			Expect(listener.ListenerStorageClassPtr()).NotTo(BeNil())
			Expect(*listener.ListenerStorageClassPtr()).To(Equal("listeners.kubedoop.dev"))
		})
	})
})
