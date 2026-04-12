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
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
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
		It("should set resource requirements from v1alpha1.ResourcesSpec", func() {
			maxCPU := resource.MustParse("500m")
			minCPU := resource.MustParse("100m")
			memLimit := resource.MustParse("512Mi")
			resourcesSpec := &v1alpha1.ResourcesSpec{
				CPU: &v1alpha1.CPUResource{
					Max: maxCPU,
					Min: minCPU,
				},
				Memory: &v1alpha1.MemoryResource{
					Limit: memLimit,
				},
			}
			result := stsBuilder.WithResources(resourcesSpec)

			Expect(result).To(Equal(stsBuilder))
			Expect(stsBuilder.Resources).NotTo(BeNil())
			Expect(stsBuilder.Resources.Requests[corev1.ResourceCPU]).To(Equal(minCPU))
			Expect(stsBuilder.Resources.Limits[corev1.ResourceCPU]).To(Equal(maxCPU))
			Expect(stsBuilder.Resources.Requests[corev1.ResourceMemory]).To(Equal(memLimit))
			Expect(stsBuilder.Resources.Limits[corev1.ResourceMemory]).To(Equal(memLimit))
		})

		It("should return builder unchanged when resources spec is nil", func() {
			result := stsBuilder.WithResources(nil)

			Expect(result).To(Equal(stsBuilder))
			Expect(stsBuilder.Resources).To(BeNil())
		})

		It("should handle partial CPU resources", func() {
			maxCPU := resource.MustParse("500m")
			resourcesSpec := &v1alpha1.ResourcesSpec{
				CPU: &v1alpha1.CPUResource{
					Max: maxCPU,
				},
			}
			stsBuilder.WithResources(resourcesSpec)

			Expect(stsBuilder.Resources.Limits[corev1.ResourceCPU]).To(Equal(maxCPU))
			Expect(stsBuilder.Resources.Requests).NotTo(HaveKey(corev1.ResourceCPU))
		})

		It("should build StatefulSet with resources", func() {
			maxCPU := resource.MustParse("500m")
			minCPU := resource.MustParse("100m")
			memLimit := resource.MustParse("512Mi")
			resourcesSpec := &v1alpha1.ResourcesSpec{
				CPU: &v1alpha1.CPUResource{
					Max: maxCPU,
					Min: minCPU,
				},
				Memory: &v1alpha1.MemoryResource{
					Limit: memLimit,
				},
			}
			sts := stsBuilder.
				WithImage(image, corev1.PullIfNotPresent).
				WithResources(resourcesSpec).
				Build()

			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.Resources.Requests[corev1.ResourceCPU]).To(Equal(minCPU))
			Expect(container.Resources.Limits[corev1.ResourceCPU]).To(Equal(maxCPU))
			Expect(container.Resources.Requests[corev1.ResourceMemory]).To(Equal(memLimit))
			Expect(container.Resources.Limits[corev1.ResourceMemory]).To(Equal(memLimit))
		})
	})

	Describe("WithSecurityContext", func() {
		It("should set container and pod security context", func() {
			runAsUser := int64(1000)
			containerCtx := &corev1.SecurityContext{
				RunAsUser: &runAsUser,
			}
			podCtx := &corev1.PodSecurityContext{
				RunAsNonRoot: boolPtr(true),
			}
			result := stsBuilder.WithSecurityContext(containerCtx, podCtx)

			Expect(result).To(Equal(stsBuilder))
			Expect(stsBuilder.SecurityContext).To(Equal(containerCtx))
			Expect(stsBuilder.PodSecurityContext).To(Equal(podCtx))
		})

		It("should build StatefulSet with security context", func() {
			runAsUser := int64(1000)
			containerCtx := &corev1.SecurityContext{
				RunAsUser: &runAsUser,
			}
			podCtx := &corev1.PodSecurityContext{
				RunAsNonRoot: boolPtr(true),
			}
			sts := stsBuilder.
				WithImage(image, corev1.PullIfNotPresent).
				WithSecurityContext(containerCtx, podCtx).
				Build()

			Expect(sts.Spec.Template.Spec.SecurityContext).To(Equal(podCtx))
			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.SecurityContext).To(Equal(containerCtx))
		})
	})

	Describe("WithPodOverrides", func() {
		It("should set pod template overrides", func() {
			overrides := &corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"override": "true"},
				},
				Spec: corev1.PodSpec{
					PriorityClassName: "high-priority",
				},
			}
			result := stsBuilder.WithPodOverrides(overrides)

			Expect(result).To(Equal(stsBuilder))
			Expect(stsBuilder.PodOverrides).To(Equal(overrides))
		})

		It("should apply pod overrides to annotations", func() {
			overrides := &corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"custom": "annotation"},
				},
			}
			sts := stsBuilder.
				WithLabels(map[string]string{"app": "test"}).
				WithPodOverrides(overrides).
				Build()

			Expect(sts.Spec.Template.Annotations).To(HaveKeyWithValue("custom", "annotation"))
		})

		It("should apply pod overrides to labels", func() {
			overrides := &corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"custom": "label"},
				},
			}
			sts := stsBuilder.
				WithLabels(map[string]string{"app": "test"}).
				WithPodOverrides(overrides).
				Build()

			Expect(sts.Spec.Template.Labels).To(HaveKeyWithValue("custom", "label"))
			Expect(sts.Spec.Template.Labels).To(HaveKeyWithValue("app", "test"))
		})

		It("should apply pod overrides to affinity", func() {
			overrides := &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "node-type",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"compute"},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			sts := stsBuilder.
				WithPodOverrides(overrides).
				Build()

			Expect(sts.Spec.Template.Spec.Affinity).NotTo(BeNil())
			Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity).NotTo(BeNil())
		})

		It("should apply pod overrides to tolerations", func() {
			overrides := &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Tolerations: []corev1.Toleration{
						{
							Key:      "dedicated",
							Operator: corev1.TolerationOpEqual,
							Value:    "compute",
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
				},
			}
			sts := stsBuilder.
				WithPodOverrides(overrides).
				Build()

			Expect(sts.Spec.Template.Spec.Tolerations).To(HaveLen(1))
			Expect(sts.Spec.Template.Spec.Tolerations[0].Key).To(Equal("dedicated"))
		})

		It("should apply pod overrides to node selector", func() {
			overrides := &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{"node-type": "compute"},
				},
			}
			sts := stsBuilder.
				WithPodOverrides(overrides).
				Build()

			Expect(sts.Spec.Template.Spec.NodeSelector).To(HaveKeyWithValue("node-type", "compute"))
		})

		It("should apply pod overrides to priority class name", func() {
			overrides := &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					PriorityClassName: "high-priority",
				},
			}
			sts := stsBuilder.
				WithPodOverrides(overrides).
				Build()

			Expect(sts.Spec.Template.Spec.PriorityClassName).To(Equal("high-priority"))
		})

		It("should create annotations map when nil in pod overrides", func() {
			// Build without any annotations first to ensure nil map
			stsBuilderWithoutAnnotations := builder.NewStatefulSetBuilder(name, namespace)
			overrides := &corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"custom": "annotation"},
				},
			}
			sts := stsBuilderWithoutAnnotations.
				WithLabels(map[string]string{"app": "test"}).
				WithPodOverrides(overrides).
				Build()

			Expect(sts.Spec.Template.Annotations).To(HaveKeyWithValue("custom", "annotation"))
		})

		It("should create labels map when nil in pod overrides", func() {
			// Build without any labels first to ensure nil map
			stsBuilderWithoutLabels := builder.NewStatefulSetBuilder(name, namespace)
			overrides := &corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"custom": "label"},
				},
			}
			sts := stsBuilderWithoutLabels.
				WithAnnotations(map[string]string{"description": "test"}).
				WithPodOverrides(overrides).
				Build()

			Expect(sts.Spec.Template.Labels).To(HaveKeyWithValue("custom", "label"))
		})
	})

	Describe("WithStorage", func() {
		It("should set storage configuration", func() {
			capacity := resource.MustParse("10Gi")
			storage := &v1alpha1.StorageResource{
				Capacity:     capacity,
				StorageClass: "fast-ssd",
			}
			mountPath := "/data"
			result := stsBuilder.WithStorage(storage, mountPath)

			Expect(result).To(Equal(stsBuilder))
			Expect(stsBuilder.StorageConfig).NotTo(BeNil())
			Expect(stsBuilder.StorageConfig.StorageClass).To(Equal("fast-ssd"))
			Expect(stsBuilder.StorageConfig.VolumeClaimTemplates).To(HaveLen(1))
		})

		It("should return builder unchanged when storage is nil", func() {
			result := stsBuilder.WithStorage(nil, "/data")

			Expect(result).To(Equal(stsBuilder))
			Expect(stsBuilder.StorageConfig).To(BeNil())
		})

		It("should build StatefulSet with volume claim templates", func() {
			capacity := resource.MustParse("10Gi")
			storage := &v1alpha1.StorageResource{
				Capacity:     capacity,
				StorageClass: "fast-ssd",
			}
			sts := stsBuilder.
				WithImage(image, corev1.PullIfNotPresent).
				WithStorage(storage, "/data").
				Build()

			Expect(sts.Spec.VolumeClaimTemplates).To(HaveLen(1))
			Expect(sts.Spec.VolumeClaimTemplates[0].Name).To(Equal("data"))
			Expect(*sts.Spec.VolumeClaimTemplates[0].Spec.StorageClassName).To(Equal("fast-ssd"))
			Expect(sts.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage]).To(Equal(capacity))
		})

		It("should add volume mount for storage", func() {
			capacity := resource.MustParse("10Gi")
			storage := &v1alpha1.StorageResource{
				Capacity: capacity,
			}
			sts := stsBuilder.
				WithImage(image, corev1.PullIfNotPresent).
				WithStorage(storage, "/data").
				Build()

			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(HaveLen(1))
			Expect(container.VolumeMounts[0].Name).To(Equal("data"))
			Expect(container.VolumeMounts[0].MountPath).To(Equal("/data"))
		})

		It("should not set storage class if empty", func() {
			capacity := resource.MustParse("10Gi")
			storage := &v1alpha1.StorageResource{
				Capacity: capacity,
			}
			sts := stsBuilder.
				WithImage(image, corev1.PullIfNotPresent).
				WithStorage(storage, "/data").
				Build()

			Expect(sts.Spec.VolumeClaimTemplates[0].Spec.StorageClassName).To(BeNil())
		})
	})

	Describe("WithPreStopHTTPGet", func() {
		It("should set a preStop HTTP GET hook", func() {
			result := stsBuilder.WithPreStopHTTPGet("/health", 8080)

			Expect(result).To(Equal(stsBuilder))
		})

		It("should build StatefulSet with HTTP GET preStop hook", func() {
			sts := stsBuilder.
				WithImage(image, corev1.PullIfNotPresent).
				WithPreStopHTTPGet("/health", 8080).
				Build()

			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.Lifecycle).NotTo(BeNil())
			Expect(container.Lifecycle.PreStop).NotTo(BeNil())
			Expect(container.Lifecycle.PreStop.HTTPGet).NotTo(BeNil())
			Expect(container.Lifecycle.PreStop.HTTPGet.Path).To(Equal("/health"))
			Expect(container.Lifecycle.PreStop.HTTPGet.Port.IntValue()).To(Equal(8080))
		})
	})

	Describe("WithPostStartHook", func() {
		It("should set a postStart exec hook", func() {
			command := []string{"/bin/sh", "-c", "echo started"}
			result := stsBuilder.WithPostStartHook(command)

			Expect(result).To(Equal(stsBuilder))
		})

		It("should build StatefulSet with postStart hook", func() {
			command := []string{"/bin/sh", "-c", "echo started"}
			sts := stsBuilder.
				WithImage(image, corev1.PullIfNotPresent).
				WithPostStartHook(command).
				Build()

			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.Lifecycle).NotTo(BeNil())
			Expect(container.Lifecycle.PostStart).NotTo(BeNil())
			Expect(container.Lifecycle.PostStart.Exec).NotTo(BeNil())
			Expect(container.Lifecycle.PostStart.Exec.Command).To(Equal(command))
		})
	})

	Describe("Build with config", func() {
		It("should include env vars from merged config", func() {
			cfg := &config.MergedConfig{
				EnvVars: map[string]string{
					"CONFIG_KEY": "config-value",
				},
			}
			sts := stsBuilder.
				WithImage(image, corev1.PullIfNotPresent).
				WithConfig(cfg).
				AddEnvVar("OVERRIDE_KEY", "override-value").
				Build()

			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.Env).To(ContainElements(
				HaveField("Name", "CONFIG_KEY"),
				HaveField("Name", "OVERRIDE_KEY"),
			))
		})

		It("should include CLI args from merged config", func() {
			cfg := &config.MergedConfig{
				CliArgs: []string{"--arg1", "--arg2"},
			}
			sts := stsBuilder.
				WithImage(image, corev1.PullIfNotPresent).
				WithConfig(cfg).
				Build()

			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.Args).To(ContainElements("--arg1", "--arg2"))
		})

		It("should set container command when provided", func() {
			stsBuilder.Command = []string{"/bin/custom", "start"}
			sts := stsBuilder.
				WithImage(image, corev1.PullIfNotPresent).
				Build()

			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.Command).To(Equal([]string{"/bin/custom", "start"}))
		})

		It("should set container args when provided", func() {
			stsBuilder.Args = []string{"--verbose", "--debug"}
			sts := stsBuilder.
				WithImage(image, corev1.PullIfNotPresent).
				Build()

			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.Args).To(ContainElements("--verbose", "--debug"))
		})

		It("should include both command and args when provided", func() {
			stsBuilder.Command = []string{"/bin/app"}
			stsBuilder.Args = []string{"--config", "/etc/config.yaml"}
			sts := stsBuilder.
				WithImage(image, corev1.PullIfNotPresent).
				Build()

			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.Command).To(Equal([]string{"/bin/app"}))
			Expect(container.Args).To(Equal([]string{"--config", "/etc/config.yaml"}))
		})
	})

	Describe("Probe configuration", func() {
		It("should use custom HTTP GET liveness probe when set", func() {
			probe := builder.NewHTTPGetProbe("/healthz", 8080, 15, 5, 20, 1, 3)
			sts := stsBuilder.
				WithImage(image, corev1.PullIfNotPresent).
				WithLivenessProbe(probe).
				Build()

			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.LivenessProbe).NotTo(BeNil())
			Expect(container.LivenessProbe.HTTPGet).NotTo(BeNil())
			Expect(container.LivenessProbe.HTTPGet.Path).To(Equal("/healthz"))
			Expect(container.LivenessProbe.HTTPGet.Port.IntValue()).To(Equal(8080))
			Expect(container.LivenessProbe.InitialDelaySeconds).To(Equal(int32(15)))
		})

		It("should use custom exec readiness probe when set", func() {
			probe := builder.NewExecProbe([]string{"/bin/check"}, 5, 3, 10, 1, 3)
			sts := stsBuilder.
				WithImage(image, corev1.PullIfNotPresent).
				WithReadinessProbe(probe).
				Build()

			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.ReadinessProbe).NotTo(BeNil())
			Expect(container.ReadinessProbe.Exec).NotTo(BeNil())
			Expect(container.ReadinessProbe.Exec.Command).To(Equal([]string{"/bin/check"}))
		})

		It("should use custom TCP socket probe via NewTCPSocketProbe", func() {
			probe := builder.NewTCPSocketProbe(9090, 10, 5, 15, 1, 3)
			sts := stsBuilder.
				WithImage(image, corev1.PullIfNotPresent).
				WithLivenessProbe(probe).
				Build()

			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.LivenessProbe).NotTo(BeNil())
			Expect(container.LivenessProbe.TCPSocket).NotTo(BeNil())
			Expect(container.LivenessProbe.TCPSocket.Port.IntValue()).To(Equal(9090))
			Expect(container.LivenessProbe.PeriodSeconds).To(Equal(int32(15)))
		})

		It("should set and use a startup probe", func() {
			probe := builder.NewExecProbe([]string{"/bin/startup-check"}, 0, 5, 10, 1, 30)
			sts := stsBuilder.
				WithImage(image, corev1.PullIfNotPresent).
				WithStartupProbe(probe).
				Build()

			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.StartupProbe).NotTo(BeNil())
			Expect(container.StartupProbe.Exec).NotTo(BeNil())
			Expect(container.StartupProbe.Exec.Command).To(Equal([]string{"/bin/startup-check"}))
			Expect(container.StartupProbe.FailureThreshold).To(Equal(int32(30)))
		})

		It("should not have a startup probe by default", func() {
			sts := stsBuilder.
				WithImage(image, corev1.PullIfNotPresent).
				Build()

			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.StartupProbe).To(BeNil())
		})

		It("should override default TCP liveness probe with custom probe even when ports are defined", func() {
			httpProbe := builder.NewHTTPGetProbe("/ready", 8080, 5, 3, 10, 1, 3)
			sts := stsBuilder.
				AddPort("http", 8080, corev1.ProtocolTCP).
				WithLivenessProbe(httpProbe).
				Build()

			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.LivenessProbe).NotTo(BeNil())
			Expect(container.LivenessProbe.HTTPGet).NotTo(BeNil())
			Expect(container.LivenessProbe.TCPSocket).To(BeNil())
		})
	})
})

func boolPtr(b bool) *bool {
	return &b
}
