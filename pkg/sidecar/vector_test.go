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

var _ = Describe("VectorSidecarProvider", func() {
	Describe("NewVectorSidecarProvider", func() {
		It("should create a new provider with default settings", func() {
			provider := sidecar.NewVectorSidecarProvider()
			Expect(provider).NotTo(BeNil())
			Expect(provider.Name()).To(Equal(sidecar.VectorSidecarName))
		})
	})

	Describe("Name", func() {
		It("should return the sidecar name", func() {
			provider := sidecar.NewVectorSidecarProvider()
			Expect(provider.Name()).To(Equal("vector"))
		})
	})

	Describe("Inject", func() {
		var provider *sidecar.VectorSidecarProvider
		var podSpec *corev1.PodSpec

		BeforeEach(func() {
			provider = sidecar.NewVectorSidecarProvider()
			podSpec = &corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "main", Image: "main-image"},
				},
			}
		})

		It("should inject Vector container into pod spec", func() {
			config := &sidecar.SidecarConfig{Enabled: true}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(podSpec.Containers).To(HaveLen(2))
			Expect(podSpec.Containers[1].Name).To(Equal(sidecar.VectorSidecarName))
		})

		It("should use default image when not specified", func() {
			config := &sidecar.SidecarConfig{Enabled: true}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(podSpec.Containers[1].Image).To(Equal(sidecar.VectorDefaultImage))
		})

		It("should use custom image when specified", func() {
			config := &sidecar.SidecarConfig{
				Enabled: true,
				Image:   "custom/vector:latest",
			}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(podSpec.Containers[1].Image).To(Equal("custom/vector:latest"))
		})

		It("should add required volume mounts", func() {
			config := &sidecar.SidecarConfig{Enabled: true}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			volumeMounts := podSpec.Containers[1].VolumeMounts
			Expect(volumeMounts).To(HaveLen(3))

			mountNames := make([]string, len(volumeMounts))
			for i, m := range volumeMounts {
				mountNames[i] = m.Name
			}
			Expect(mountNames).To(ContainElements(
				sidecar.VectorConfigVolumeName,
				sidecar.VectorDataVolumeName,
				sidecar.VectorLogVolumeName,
			))
		})

		It("should add required volumes to pod", func() {
			config := &sidecar.SidecarConfig{Enabled: true}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			Expect(podSpec.Volumes).To(HaveLen(3))

			volumeNames := make([]string, len(podSpec.Volumes))
			for i, v := range podSpec.Volumes {
				volumeNames[i] = v.Name
			}
			Expect(volumeNames).To(ContainElements(
				sidecar.VectorConfigVolumeName,
				sidecar.VectorDataVolumeName,
				sidecar.VectorLogVolumeName,
			))
		})

		It("should mount log volume to main container", func() {
			config := &sidecar.SidecarConfig{Enabled: true}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			// Check main container has log volume mount
			mainContainer := podSpec.Containers[0]
			var foundLogMount bool
			for _, m := range mainContainer.VolumeMounts {
				if m.Name == sidecar.VectorLogVolumeName {
					foundLogMount = true
					Expect(m.MountPath).To(Equal(sidecar.VectorLogMountPath))
					break
				}
			}
			Expect(foundLogMount).To(BeTrue())
		})

		It("should set correct command", func() {
			config := &sidecar.SidecarConfig{Enabled: true}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			cmd := podSpec.Containers[1].Command
			Expect(cmd).To(ContainElements("vector", "--config"))
			Expect(cmd).To(ContainElement(sidecar.VectorConfigMountPath + "/vector.yaml"))
		})

		It("should apply custom resources", func() {
			resources := corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("200m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
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
					"VECTOR_LOG": "debug",
				},
			}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			Expect(podSpec.Containers[1].Env).NotTo(BeEmpty())
		})

		It("should apply custom volume mounts", func() {
			customMounts := []corev1.VolumeMount{
				{Name: "custom-data", MountPath: "/custom"},
			}
			config := &sidecar.SidecarConfig{
				Enabled:      true,
				VolumeMounts: customMounts,
			}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			var found bool
			for _, m := range podSpec.Containers[1].VolumeMounts {
				if m.Name == "custom-data" {
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

var _ = Describe("Vector constants", func() {
	It("should have correct default values", func() {
		Expect(sidecar.VectorSidecarName).To(Equal("vector"))
		Expect(sidecar.VectorDefaultImage).To(ContainSubstring("vector"))
		Expect(sidecar.VectorConfigVolumeName).To(Equal("vector-config"))
		Expect(sidecar.VectorDataVolumeName).To(Equal("vector-data"))
		Expect(sidecar.VectorLogVolumeName).To(Equal("log-volume"))
		Expect(sidecar.VectorConfigMountPath).To(Equal("/etc/vector"))
		Expect(sidecar.VectorDataMountPath).To(Equal("/var/lib/vector"))
		Expect(sidecar.VectorLogMountPath).To(Equal("/var/log/app"))
	})
})
