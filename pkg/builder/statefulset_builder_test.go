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
	"github.com/zncdatadev/operator-go/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("StatefulSetBuilder", func() {
	const (
		name      = "test-sts"
		namespace = "test-namespace"
		image     = "test-image:latest"
	)

	var stsBuilder *builder.StatefulSetBuilder

	BeforeEach(func() {
		stsBuilder = builder.NewStatefulSetBuilder(name, namespace)
	})

	Describe("NewStatefulSetBuilder", func() {
		It("should create a builder with default values", func() {
			Expect(stsBuilder.Name).To(Equal(name))
			Expect(stsBuilder.Namespace).To(Equal(namespace))
			Expect(stsBuilder.Replicas).To(Equal(int32(1)))
			Expect(stsBuilder.ImagePullPolicy).To(Equal(corev1.PullIfNotPresent))
		})
	})

	Describe("WithLabels", func() {
		It("should add labels to the builder", func() {
			labels := map[string]string{
				"app":  "test",
				"tier": "backend",
			}
			result := stsBuilder.WithLabels(labels)

			Expect(result).To(Equal(stsBuilder))
			Expect(stsBuilder.Labels).To(HaveKeyWithValue("app", "test"))
			Expect(stsBuilder.Labels).To(HaveKeyWithValue("tier", "backend"))
		})
	})

	Describe("WithAnnotations", func() {
		It("should add annotations to the builder", func() {
			annotations := map[string]string{
				"description": "test annotation",
			}
			result := stsBuilder.WithAnnotations(annotations)

			Expect(result).To(Equal(stsBuilder))
			Expect(stsBuilder.Annotations).To(HaveKeyWithValue("description", "test annotation"))
		})
	})

	Describe("WithReplicas", func() {
		It("should set the replica count", func() {
			result := stsBuilder.WithReplicas(3)

			Expect(result).To(Equal(stsBuilder))
			Expect(stsBuilder.Replicas).To(Equal(int32(3)))
		})
	})

	Describe("WithImage", func() {
		It("should set the image and pull policy", func() {
			result := stsBuilder.WithImage(image, corev1.PullAlways)

			Expect(result).To(Equal(stsBuilder))
			Expect(stsBuilder.Image).To(Equal(image))
			Expect(stsBuilder.ImagePullPolicy).To(Equal(corev1.PullAlways))
		})

		It("should not change pull policy if empty", func() {
			stsBuilder.WithImage(image, "")
			Expect(stsBuilder.ImagePullPolicy).To(Equal(corev1.PullIfNotPresent))
		})
	})

	Describe("WithConfig", func() {
		It("should set the merged config", func() {
			cfg := &config.MergedConfig{
				EnvVars: map[string]string{
					"KEY": "value",
				},
			}
			result := stsBuilder.WithConfig(cfg)

			Expect(result).To(Equal(stsBuilder))
			Expect(stsBuilder.Config).To(Equal(cfg))
		})
	})

	Describe("WithPorts", func() {
		It("should set container ports", func() {
			ports := []corev1.ContainerPort{
				{Name: "http", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
			}
			result := stsBuilder.WithPorts(ports)

			Expect(result).To(Equal(stsBuilder))
			Expect(stsBuilder.Ports).To(HaveLen(1))
			Expect(stsBuilder.Ports[0].Name).To(Equal("http"))
		})
	})

	Describe("AddPort", func() {
		It("should add a container port", func() {
			result := stsBuilder.AddPort("http", 8080, corev1.ProtocolTCP)

			Expect(result).To(Equal(stsBuilder))
			Expect(stsBuilder.Ports).To(HaveLen(1))
			Expect(stsBuilder.Ports[0].ContainerPort).To(Equal(int32(8080)))
		})
	})

	Describe("AddVolume", func() {
		It("should add a volume", func() {
			vol := corev1.Volume{
				Name: "config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: "config"},
					},
				},
			}
			result := stsBuilder.AddVolume(vol)

			Expect(result).To(Equal(stsBuilder))
			Expect(stsBuilder.Volumes).To(HaveLen(1))
			Expect(stsBuilder.Volumes[0].Name).To(Equal("config"))
		})
	})

	Describe("AddVolumeMount", func() {
		It("should add a volume mount", func() {
			mount := corev1.VolumeMount{
				Name:      "config",
				MountPath: "/etc/config",
			}
			result := stsBuilder.AddVolumeMount(mount)

			Expect(result).To(Equal(stsBuilder))
			Expect(stsBuilder.VolumeMounts).To(HaveLen(1))
			Expect(stsBuilder.VolumeMounts[0].MountPath).To(Equal("/etc/config"))
		})
	})

	Describe("AddEnvVar", func() {
		It("should add an environment variable", func() {
			result := stsBuilder.AddEnvVar("KEY", "value")

			Expect(result).To(Equal(stsBuilder))
			Expect(stsBuilder.EnvVars).To(HaveLen(1))
			Expect(stsBuilder.EnvVars[0].Name).To(Equal("KEY"))
			Expect(stsBuilder.EnvVars[0].Value).To(Equal("value"))
		})
	})

	Describe("WithServiceAccount", func() {
		It("should set the service account name", func() {
			result := stsBuilder.WithServiceAccount("my-sa")

			Expect(result).To(Equal(stsBuilder))
			Expect(stsBuilder.ServiceAccountName).To(Equal("my-sa"))
		})
	})

	Describe("WithAffinity", func() {
		It("should set the affinity", func() {
			affinity := &corev1.Affinity{
				PodAntiAffinity: &corev1.PodAntiAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							TopologyKey: "kubernetes.io/hostname",
						},
					},
				},
			}
			result := stsBuilder.WithAffinity(affinity)

			Expect(result).To(Equal(stsBuilder))
			Expect(stsBuilder.Affinity).To(Equal(affinity))
		})
	})

	Describe("WithTerminationGracePeriod", func() {
		It("should set the termination grace period", func() {
			result := stsBuilder.WithTerminationGracePeriod(60)

			Expect(result).To(Equal(stsBuilder))
			Expect(*stsBuilder.TerminationGracePeriodSeconds).To(Equal(int64(60)))
		})
	})

	Describe("WithPreStopHook", func() {
		It("should set a preStop exec hook", func() {
			command := []string{"/bin/sh", "-c", "sleep 10"}
			result := stsBuilder.WithPreStopHook(command)

			Expect(result).To(Equal(stsBuilder))
		})
	})

	Describe("Build", func() {
		It("should build a valid StatefulSet", func() {
			sts := stsBuilder.
				WithLabels(map[string]string{"app": "test"}).
				WithReplicas(3).
				WithImage(image, corev1.PullIfNotPresent).
				AddPort("http", 8080, corev1.ProtocolTCP).
				Build()

			Expect(sts).NotTo(BeNil())
			Expect(sts.Name).To(Equal(name))
			Expect(sts.Namespace).To(Equal(namespace))
			Expect(*sts.Spec.Replicas).To(Equal(int32(3)))
			Expect(sts.Spec.ServiceName).To(Equal(name + "-headless"))
			Expect(sts.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal(image))
		})

		It("should set correct selector labels", func() {
			labels := map[string]string{"app": "test"}
			sts := stsBuilder.WithLabels(labels).Build()

			Expect(sts.Spec.Selector.MatchLabels).To(Equal(labels))
			Expect(sts.Spec.Template.Labels).To(Equal(labels))
		})

		It("should create liveness and readiness probes when ports are defined", func() {
			sts := stsBuilder.
				AddPort("http", 8080, corev1.ProtocolTCP).
				Build()

			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.LivenessProbe).NotTo(BeNil())
			Expect(container.ReadinessProbe).NotTo(BeNil())
		})

		It("should not create probes when no ports are defined", func() {
			sts := stsBuilder.Build()

			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.LivenessProbe).To(BeNil())
			Expect(container.ReadinessProbe).To(BeNil())
		})

		It("should include preStop hook when set", func() {
			command := []string{"/bin/sh", "-c", "sleep 10"}
			sts := stsBuilder.
				WithPreStopHook(command).
				Build()

			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.Lifecycle).NotTo(BeNil())
			Expect(container.Lifecycle.PreStop).NotTo(BeNil())
			Expect(container.Lifecycle.PreStop.Exec).NotTo(BeNil())
			Expect(container.Lifecycle.PreStop.Exec.Command).To(Equal(command))
		})
	})

	Describe("NamespacedName", func() {
		It("should return the correct NamespacedName", func() {
			nn := stsBuilder.NamespacedName()

			Expect(nn.Name).To(Equal(name))
			Expect(nn.Namespace).To(Equal(namespace))
		})
	})

	Describe("WithResources", func() {
		It("should set resource requirements", func() {
			resources := &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
			}
			stsBuilder.Resources = resources
			sts := stsBuilder.Build()

			Expect(sts.Spec.Template.Spec.Containers[0].Resources).To(Equal(*resources))
		})
	})
})
