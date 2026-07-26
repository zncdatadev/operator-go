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
	"path"

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

	Describe("WithConfigMapName", func() {
		It("should set a custom ConfigMap name", func() {
			provider := sidecar.NewJMXExporterSidecarProvider().WithConfigMapName("custom-config")
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

		const testImage = "test-product-image:latest"

		BeforeEach(func() {
			provider = sidecar.NewJMXExporterSidecarProvider()
			podSpec = &corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "main", Image: "main-image"},
				},
			}
		})

		It("should inject JMX exporter container into pod spec", func() {
			config := &sidecar.SidecarConfig{Enabled: true, Image: testImage}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(podSpec.InitContainers).To(HaveLen(1))
			Expect(podSpec.InitContainers[0].Name).To(Equal(sidecar.JMXExporterSidecarName))
		})

		It("should return error when image is not specified", func() {
			config := &sidecar.SidecarConfig{Enabled: true}
			err := provider.Inject(podSpec, config)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("image is required"))
		})

		It("should use custom image when specified", func() {
			config := &sidecar.SidecarConfig{
				Enabled: true,
				Image:   "custom/jmx-exporter:latest",
			}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(podSpec.InitContainers[0].Image).To(Equal("custom/jmx-exporter:latest"))
		})

		It("should use default port", func() {
			config := &sidecar.SidecarConfig{Enabled: true, Image: testImage}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(podSpec.InitContainers[0].Ports[0].ContainerPort).To(Equal(int32(sidecar.JMXExporterPort)))
		})

		It("should use custom port from provider", func() {
			provider = provider.WithPort(9999)
			config := &sidecar.SidecarConfig{Enabled: true, Image: testImage}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(podSpec.InitContainers[0].Ports[0].ContainerPort).To(Equal(int32(9999)))
		})

		It("should use custom port from config", func() {
			config := &sidecar.SidecarConfig{
				Enabled: true,
				Image:   testImage,
				Ports: []corev1.ContainerPort{
					{ContainerPort: 8888},
				},
			}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(podSpec.InitContainers[0].Ports[0].ContainerPort).To(Equal(int32(8888)))
		})

		It("should add config volume mount", func() {
			config := &sidecar.SidecarConfig{Enabled: true, Image: testImage}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			volumeMounts := podSpec.InitContainers[0].VolumeMounts
			Expect(volumeMounts).NotTo(BeEmpty())
			Expect(volumeMounts[0].Name).To(Equal(sidecar.JMXExporterConfigVolumeName))
			Expect(volumeMounts[0].MountPath).To(Equal(sidecar.JMXExporterConfigMountPath))
			Expect(volumeMounts[0].ReadOnly).To(BeTrue())
		})

		It("should not mount the config over the directory holding the jar", func() {
			config := &sidecar.SidecarConfig{Enabled: true, Image: testImage}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			jarDir := path.Dir(sidecar.JMXExporterJarPath)
			for _, m := range podSpec.InitContainers[0].VolumeMounts {
				Expect(m.MountPath).NotTo(Equal(jarDir))
				Expect(jarDir).NotTo(HavePrefix(m.MountPath + "/"))
			}
		})

		It("should run the jar and read the config from the mounted config path", func() {
			config := &sidecar.SidecarConfig{Enabled: true, Image: testImage}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			command := podSpec.InitContainers[0].Command
			Expect(command).To(ContainElement(sidecar.JMXExporterJarPath))
			Expect(command).To(ContainElement(
				sidecar.JMXExporterConfigMountPath + "/" + sidecar.JMXExporterConfigFileName,
			))
		})

		It("should add caller-supplied volumes backing caller-supplied mounts", func() {
			config := &sidecar.SidecarConfig{
				Enabled: true,
				Image:   testImage,
				Volumes: []corev1.Volume{
					{
						Name:         "extra",
						VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
					},
				},
				VolumeMounts: []corev1.VolumeMount{
					{Name: "extra", MountPath: "/extra"},
				},
			}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			names := make([]string, 0, len(podSpec.Volumes))
			for _, v := range podSpec.Volumes {
				names = append(names, v.Name)
			}
			Expect(names).To(ContainElement("extra"))
		})

		It("should add config volume to pod", func() {
			config := &sidecar.SidecarConfig{Enabled: true, Image: testImage}
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

		It("should set no readiness probe, so a broken exporter cannot empty the product's Services", func() {
			// Kubernetes documents that for a sidecar container (an init container with
			// restartPolicy Always) "if a readinessProbe is specified for this init container, its
			// result will be used to determine the ready state of the Pod". A metrics exporter is
			// not in the request path, so a probe here converts a scraping failure into a product
			// outage.
			config := &sidecar.SidecarConfig{Enabled: true, Image: testImage}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			container := podSpec.InitContainers[0]
			Expect(container.ReadinessProbe).To(BeNil())
			Expect(container.StartupProbe).To(BeNil(), "nothing waits on the exporter")
		})

		It("should set a liveness probe, so a lost JMX connection is recovered", func() {
			// The counterpart to the assertion above: a liveness failure restarts only this
			// container and never touches Service membership, so it is the probe that CAN
			// guarantee the sidecar keeps working. Having neither probe — the previous
			// iteration's state — left a running-but-useless exporter invisible and permanent.
			config := &sidecar.SidecarConfig{Enabled: true, Image: testImage}
			Expect(provider.Inject(podSpec, config)).To(Succeed())

			probe := podSpec.InitContainers[0].LivenessProbe
			Expect(probe).NotTo(BeNil())
			Expect(probe.HTTPGet).NotTo(BeNil())
			// The literal, not the constant: /metrics is fixed by jmx_prometheus_httpserver, so a
			// constant pointing elsewhere is the bug — and an assertion against the constant
			// would move with it.
			Expect(probe.HTTPGet.Path).To(Equal("/metrics"))
			Expect(probe.HTTPGet.Port.IntValue()).To(Equal(sidecar.JMXExporterPort))

			// Scraping /metrics makes the exporter collect from the JVM over JMX, so the response
			// time tracks the product's GC. The probe must tolerate a stop-the-world pause: a
			// readiness-probe-grade 5s timeout is what made the original probe flap.
			Expect(probe.TimeoutSeconds).To(BeNumerically(">=", 10))
			Expect(probe.PeriodSeconds*probe.FailureThreshold).To(BeNumerically(">=", 120),
				"a long GC pause must not be read as a broken exporter")
		})

		It("should let a product replace or remove the probe", func() {
			// SidecarConfig could previously not express a probe at all, so the framework's policy
			// was unconfigurable rather than a default.
			custom := &corev1.Probe{
				ProbeHandler:  corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{"true"}}},
				PeriodSeconds: 7,
			}
			config := &sidecar.SidecarConfig{Enabled: true, Image: testImage}
			config.Probes.Liveness = custom
			Expect(provider.Inject(podSpec, config)).To(Succeed())

			probe := podSpec.InitContainers[0].LivenessProbe
			Expect(probe).NotTo(BeNil())
			Expect(probe.Exec).NotTo(BeNil())
			Expect(probe.HTTPGet).To(BeNil(), "the override replaces wholesale; a probe with two handlers is rejected by the API server")

			podSpec = &corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "main"}}}
			disabled := &sidecar.SidecarConfig{Enabled: true, Image: testImage}
			disabled.Probes.DisableLiveness = true
			Expect(provider.Inject(podSpec, disabled)).To(Succeed())
			Expect(podSpec.InitContainers[0].LivenessProbe).To(BeNil())
		})

		It("should harden the container by default so restricted Pod Security admits it", func() {
			config := &sidecar.SidecarConfig{Enabled: true, Image: testImage}
			Expect(provider.Inject(podSpec, config)).To(Succeed())

			sc := podSpec.InitContainers[0].SecurityContext
			Expect(sc).NotTo(BeNil(), "a nil security context is rejected under restricted PSS")
			Expect(*sc.RunAsNonRoot).To(BeTrue())
			Expect(*sc.AllowPrivilegeEscalation).To(BeFalse())
			Expect(sc.Capabilities.Drop).To(ConsistOf(corev1.Capability("ALL")))
			Expect(sc.SeccompProfile.Type).To(Equal(corev1.SeccompProfileTypeRuntimeDefault))

			// The JVM writes hsperfdata into /tmp at startup, so a read-only root filesystem
			// would break this container specifically.
			Expect(sc.ReadOnlyRootFilesystem).To(BeNil())
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
				Image:     testImage,
				Resources: &resources,
			}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			Expect(podSpec.InitContainers[0].Resources.Limits).To(HaveKey(corev1.ResourceCPU))
		})

		It("should apply custom environment variables", func() {
			config := &sidecar.SidecarConfig{
				Enabled: true,
				Image:   testImage,
				EnvVars: map[string]string{
					"JAVA_OPTS": "-Xmx256m",
				},
			}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			Expect(podSpec.InitContainers[0].Env).NotTo(BeEmpty())
		})

		It("should apply custom volume mounts", func() {
			customMounts := []corev1.VolumeMount{
				{Name: "custom", MountPath: "/custom"},
			}
			config := &sidecar.SidecarConfig{
				Enabled:      true,
				Image:        testImage,
				VolumeMounts: customMounts,
			}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			var found bool
			for _, m := range podSpec.InitContainers[0].VolumeMounts {
				if m.Name == "custom" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue())
		})

		It("should apply security context when provided", func() {
			securityContext := &corev1.SecurityContext{
				RunAsNonRoot:             ptrBool(true),
				ReadOnlyRootFilesystem:   ptrBool(true),
				AllowPrivilegeEscalation: ptrBool(false),
			}
			config := &sidecar.SidecarConfig{
				Enabled:         true,
				Image:           testImage,
				SecurityContext: securityContext,
			}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			Expect(podSpec.InitContainers[0].SecurityContext).NotTo(BeNil())
			Expect(*podSpec.InitContainers[0].SecurityContext.RunAsNonRoot).To(BeTrue())
		})

		It("should apply custom image pull policy when provided", func() {
			config := &sidecar.SidecarConfig{
				Enabled:         true,
				Image:           testImage,
				ImagePullPolicy: corev1.PullAlways,
			}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			Expect(podSpec.InitContainers[0].ImagePullPolicy).To(Equal(corev1.PullAlways))
		})

		It("should use default pull policy when not specified", func() {
			config := &sidecar.SidecarConfig{Enabled: true, Image: testImage}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			Expect(podSpec.InitContainers[0].ImagePullPolicy).To(Equal(corev1.PullIfNotPresent))
		})

		It("should return error with nil config", func() {
			err := provider.Inject(podSpec, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("image is required"))
		})

		It("should be idempotent - not duplicate container on repeated inject", func() {
			config := &sidecar.SidecarConfig{Enabled: true, Image: testImage}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(podSpec.InitContainers).To(HaveLen(1))

			// Inject again
			err = provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			// Should still have 2 containers (main + jmx-exporter), not 3
			Expect(podSpec.InitContainers).To(HaveLen(1))

			// Count jmx-exporter containers
			jmxCount := 0
			for _, c := range podSpec.InitContainers {
				if c.Name == sidecar.JMXExporterSidecarName {
					jmxCount++
				}
			}
			Expect(jmxCount).To(Equal(1))
		})

		It("should use custom ConfigMap name for volume", func() {
			provider = sidecar.NewJMXExporterSidecarProvider().WithConfigMapName("custom-jmx-config")
			config := &sidecar.SidecarConfig{Enabled: true, Image: testImage}
			err := provider.Inject(podSpec, config)
			Expect(err).NotTo(HaveOccurred())

			// Find the config volume
			var configVolume *corev1.Volume
			for i, v := range podSpec.Volumes {
				if v.Name == sidecar.JMXExporterConfigVolumeName {
					configVolume = &podSpec.Volumes[i]
					break
				}
			}
			Expect(configVolume).NotTo(BeNil())
			Expect(configVolume.ConfigMap).NotTo(BeNil())
			Expect(configVolume.ConfigMap.Name).To(Equal("custom-jmx-config"))
		})
	})
})

var _ = Describe("JMXExporter constants", func() {
	It("should have correct default values", func() {
		Expect(sidecar.JMXExporterSidecarName).To(Equal("jmx-exporter"))
		Expect(int32(sidecar.JMXExporterPort)).To(Equal(int32(5556)))
		Expect(sidecar.JMXExporterConfigVolumeName).To(Equal("jmx-exporter-config"))
		Expect(sidecar.JMXExporterJarPath).To(Equal("/opt/jmx_exporter/jmx_prometheus_httpserver.jar"))
		Expect(sidecar.JMXExporterConfigMountPath).To(Equal("/kubedoop/mount/config/jmx-exporter"))
		Expect(sidecar.JMXExporterConfigFileName).To(Equal("config.yaml"))
		Expect(sidecar.JMXExporterDefaultConfigMapName).To(Equal("jmx-exporter-config"))
	})
})

func ptrBool(b bool) *bool {
	return &b
}
