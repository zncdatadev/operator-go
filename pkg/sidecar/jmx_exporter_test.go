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

package sidecar_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/sidecar"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

var _ = Describe("JMXExporterSidecarProvider", func() {
	Describe("NewJMXExporterSidecarProvider", func() {
		It("should create a new provider with default settings", func() {
			provider := sidecar.NewJMXExporterSidecarProvider()
			Expect(provider).NotTo(BeNil())
			Expect(provider.Name()).To(Equal(sidecar.JMXExporterSidecarName))
		})
	})

	Describe("WithPort", func() {
		It("should set a custom port", func() {
			provider := sidecar.NewJMXExporterSidecarProvider().WithPort(9999)
			Expect(provider).NotTo(BeNil())
		})
	})

	Describe("Name", func() {
		It("should return the sidecar name", func() {
			provider := sidecar.NewJMXExporterSidecarProvider()
			Expect(provider.Name()).To(Equal("jmx-exporter"))
		})
	})

	Describe("Inject", func() {
		var provider *sidecar.JMXExporterSidecarProvider
		var podSpec *corev1.PodSpec

		BeforeEach(func() {
			provider = sidecar.NewJMXExporterSidecarProvider()
			podSpec = &corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "main", Image: "main-image"},
				},
			}
		})

		It("should inject JMX exporter container into pod spec", func() {
			config := &sidecar.SidecarConfig{Enabled: true}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(podSpec.Containers).To(HaveLen(2))
			Expect(podSpec.Containers[1].Name).To(Equal(sidecar.JMXExporterSidecarName))
		})

		It("should use default image when not specified", func() {
			config := &sidecar.SidecarConfig{Enabled: true}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(podSpec.Containers[1].Image).To(Equal(sidecar.JMXExporterDefaultImage))
		})

		It("should use custom image when specified", func() {
			config := &sidecar.SidecarConfig{
				Enabled: true,
				Image:   "custom/jmx-exporter:latest",
			}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(podSpec.Containers[1].Image).To(Equal("custom/jmx-exporter:latest"))
		})

		It("should use default port", func() {
			config := &sidecar.SidecarConfig{Enabled: true}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(podSpec.Containers[1].Ports[0].ContainerPort).To(Equal(int32(sidecar.JMXExporterPort)))
		})

		It("should use custom port from provider", func() {
			provider = provider.WithPort(9999)
			config := &sidecar.SidecarConfig{Enabled: true}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(podSpec.Containers[1].Ports[0].ContainerPort).To(Equal(int32(9999)))
		})

		It("should use custom port from config", func() {
			config := &sidecar.SidecarConfig{
				Enabled: true,
				Ports: []corev1.ContainerPort{
					{ContainerPort: 8888},
				},
			}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(podSpec.Containers[1].Ports[0].ContainerPort).To(Equal(int32(8888)))
		})

		It("should add config volume mount", func() {
			config := &sidecar.SidecarConfig{Enabled: true}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			volumeMounts := podSpec.Containers[1].VolumeMounts
			Expect(volumeMounts).NotTo(BeEmpty())
			Expect(volumeMounts[0].Name).To(Equal(sidecar.JMXExporterConfigVolumeName))
			Expect(volumeMounts[0].MountPath).To(Equal(sidecar.JMXExporterConfigMountPath))
			Expect(volumeMounts[0].ReadOnly).To(BeTrue())
		})

		It("should add config volume to pod", func() {
			config := &sidecar.SidecarConfig{Enabled: true}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			Expect(podSpec.Volumes).NotTo(BeEmpty())
			var foundVolume *corev1.Volume
			for i, v := range podSpec.Volumes {
				if v.Name == sidecar.JMXExporterConfigVolumeName {
					foundVolume = &podSpec.Volumes[i]
					break
				}
			}
			Expect(foundVolume).NotTo(BeNil())
			Expect(foundVolume.ConfigMap).NotTo(BeNil())
			Expect(foundVolume.ConfigMap.Name).To(Equal("jmx-exporter-config"))
		})

		It("should set readiness probe", func() {
			config := &sidecar.SidecarConfig{Enabled: true}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			probe := podSpec.Containers[1].ReadinessProbe
			Expect(probe).NotTo(BeNil())
			Expect(probe.HTTPGet).NotTo(BeNil())
			Expect(probe.HTTPGet.Path).To(Equal("/metrics"))
		})

		It("should apply custom resources", func() {
			resources := corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
			}
			config := &sidecar.SidecarConfig{
				Enabled:   true,
				Resources: &resources,
			}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			Expect(podSpec.Containers[1].Resources.Limits).To(HaveKey(corev1.ResourceCPU))
		})

		It("should apply custom environment variables", func() {
			config := &sidecar.SidecarConfig{
				Enabled: true,
				EnvVars: map[string]string{
					"JAVA_OPTS": "-Xmx256m",
				},
			}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			Expect(podSpec.Containers[1].Env).NotTo(BeEmpty())
		})

		It("should apply custom volume mounts", func() {
			customMounts := []corev1.VolumeMount{
				{Name: "custom", MountPath: "/custom"},
			}
			config := &sidecar.SidecarConfig{
				Enabled:      true,
				VolumeMounts: customMounts,
			}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			var found bool
			for _, m := range podSpec.Containers[1].VolumeMounts {
				if m.Name == "custom" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue())
		})

		It("should work with nil config", func() {
			err := provider.Inject(podSpec, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(podSpec.Containers).To(HaveLen(2))
		})
	})
})

var _ = Describe("JMXExporter constants", func() {
	It("should have correct default values", func() {
		Expect(sidecar.JMXExporterSidecarName).To(Equal("jmx-exporter"))
		Expect(sidecar.JMXExporterDefaultImage).To(ContainSubstring("jmx-exporter"))
		Expect(int32(sidecar.JMXExporterPort)).To(Equal(int32(5556)))
		Expect(sidecar.JMXExporterConfigVolumeName).To(Equal("jmx-exporter-config"))
		Expect(sidecar.JMXExporterConfigMountPath).To(Equal("/opt/jmx_exporter"))
	})
})
