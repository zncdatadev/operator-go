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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var _ = Describe("DeploymentBuilder", func() {
	const (
		name      = "test-deployment"
		namespace = "test-namespace"
		image     = "test-image:latest"
	)

	var deploymentBuilder *builder.DeploymentBuilder

	BeforeEach(func() {
		deploymentBuilder = builder.NewDeploymentBuilder(name, namespace)
	})

	Describe("NewDeploymentBuilder", func() {
		It("should create a builder with default values", func() {
			Expect(deploymentBuilder.Name).To(Equal(name))
			Expect(deploymentBuilder.Namespace).To(Equal(namespace))
			Expect(deploymentBuilder.Replicas).To(Equal(int32(1)))
			Expect(deploymentBuilder.ImagePullPolicy).To(Equal(corev1.PullIfNotPresent))
		})
	})

	Describe("Build", func() {
		It("should build a Deployment with the workload-level fields a Deployment has, and none it does not", func() {
			deployment := deploymentBuilder.
				WithImage(image, corev1.PullAlways).
				WithReplicas(3).
				WithSelectorLabels(map[string]string{"app": "test"}).
				Build()

			Expect(deployment.Name).To(Equal(name))
			Expect(deployment.Namespace).To(Equal(namespace))
			Expect(*deployment.Spec.Replicas).To(Equal(int32(3)))
			Expect(deployment.Spec.Selector.MatchLabels).To(Equal(map[string]string{"app": "test"}))
			// The selector must be a subset of the template labels or the API server rejects the
			// Deployment.
			Expect(deployment.Spec.Template.Labels).To(HaveKeyWithValue("app", "test"))

			// The rollout is deliberately left to the Kubernetes default (RollingUpdate,
			// 25%/25%): an explicit strategy here would pin server defaults for no reason.
			Expect(deployment.Spec.Strategy).To(Equal(appsv1.DeploymentStrategy{}))
		})

		It("should return an independent object on each Build", func() {
			deploymentBuilder.WithImage(image, "").WithSelectorLabels(map[string]string{"app": "test"})

			first := deploymentBuilder.Build()
			first.Spec.Template.Spec.Containers = append(first.Spec.Template.Spec.Containers,
				corev1.Container{Name: "sidecar", Image: "sidecar:latest"})
			first.Spec.Selector.MatchLabels["mutated"] = "true"

			second := deploymentBuilder.Build()
			Expect(second.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(second.Spec.Selector.MatchLabels).NotTo(HaveKey("mutated"))
		})
	})

	Describe("pod template parity with StatefulSetBuilder", func() {
		// The whole point of the shared workloadBuilder: for identical inputs the two workload
		// kinds must render identical pods, or a role switched between kinds would change behavior
		// the user never asked to change.
		It("should render the same pod template a StatefulSetBuilder renders for identical inputs", func() {
			labels := map[string]string{"role": "web"}
			selectorLabels := map[string]string{"app": "parity"}
			annotations := map[string]string{"team": "data"}
			ports := []corev1.ContainerPort{{Name: "http", ContainerPort: 8080, Protocol: corev1.ProtocolTCP}}
			volume := corev1.Volume{
				Name: "config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: "cfg"},
					},
				},
			}
			mount := corev1.VolumeMount{Name: "config", MountPath: "/etc/config"}
			initContainer := corev1.Container{Name: "init", Image: "busybox:latest"}
			resources := &v1alpha1.ResourcesSpec{
				CPU: &v1alpha1.CPUResource{
					Min: ptr.To(resource.MustParse("250m")),
					Max: ptr.To(resource.MustParse("1")),
				},
				Memory: &v1alpha1.MemoryResource{Limit: ptr.To(resource.MustParse("512Mi"))},
			}
			securityCtx := &corev1.SecurityContext{RunAsUser: ptr.To(int64(1001))}
			podSecurityCtx := &corev1.PodSecurityContext{FSGroup: ptr.To(int64(1001))}
			affinity := &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{{
							MatchExpressions: []corev1.NodeSelectorRequirement{{
								Key: "zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"a"},
							}},
						}},
					},
				},
			}
			cfg := &config.MergedConfig{
				EnvVars: map[string]string{"B_KEY": "2", "A_KEY": "1"},
				CliArgs: []string{"--verbose"},
			}
			customizer := func(c *corev1.Container) error {
				c.Command = []string{"/bin/product", "server"}
				return nil
			}

			sts := builder.NewStatefulSetBuilder(name, namespace).
				WithLabels(labels).
				WithSelectorLabels(selectorLabels).
				WithAnnotations(annotations).
				WithReplicas(2).
				WithImage(image, corev1.PullAlways).
				WithConfig(cfg).
				WithPorts(ports).
				AddVolume(volume).
				AddVolumeMount(mount).
				AddInitContainer(initContainer).
				AddEnvVar("EXPLICIT", "yes").
				WithResources(resources).
				WithSecurityContext(securityCtx, podSecurityCtx).
				WithAffinity(affinity).
				WithServiceAccount("svc-account").
				WithEnableServiceLinks(false).
				WithTerminationGracePeriod(120).
				WithPreStopHook([]string{"/bin/stop"}).
				WithMainContainerName("product").
				WithMainContainerCustomizer(customizer).
				Build()

			deployment := builder.NewDeploymentBuilder(name, namespace).
				WithLabels(labels).
				WithSelectorLabels(selectorLabels).
				WithAnnotations(annotations).
				WithReplicas(2).
				WithImage(image, corev1.PullAlways).
				WithConfig(cfg).
				WithPorts(ports).
				AddVolume(volume).
				AddVolumeMount(mount).
				AddInitContainer(initContainer).
				AddEnvVar("EXPLICIT", "yes").
				WithResources(resources).
				WithSecurityContext(securityCtx, podSecurityCtx).
				WithAffinity(affinity).
				WithServiceAccount("svc-account").
				WithEnableServiceLinks(false).
				WithTerminationGracePeriod(120).
				WithPreStopHook([]string{"/bin/stop"}).
				WithMainContainerName("product").
				WithMainContainerCustomizer(customizer).
				Build()

			Expect(deployment.Spec.Template).To(Equal(sts.Spec.Template))
			Expect(deployment.Spec.Selector).To(Equal(sts.Spec.Selector))
			Expect(deployment.Labels).To(Equal(sts.Labels))
			Expect(deployment.Annotations).To(Equal(sts.Annotations))
		})
	})

	Describe("main container customizer", func() {
		It("should run the customizer on the assembled container", func() {
			deployment := deploymentBuilder.
				WithImage(image, "").
				WithMainContainerCustomizer(func(c *corev1.Container) error {
					c.Command = []string{"/bin/serve"}
					c.Env = append(c.Env, corev1.EnvVar{Name: "MODE", Value: "web"})
					return nil
				}).
				Build()

			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.Command).To(Equal([]string{"/bin/serve"}))
			Expect(container.Env).To(ContainElement(corev1.EnvVar{Name: "MODE", Value: "web"}))
			Expect(deploymentBuilder.MainContainerViolations()).To(BeNil())
		})

		It("should let podOverrides outrank the customizer — the timing is the point", func() {
			deployment := deploymentBuilder.
				WithImage(image, "").
				WithMainContainerCustomizer(func(c *corev1.Container) error {
					c.Command = []string{"/bin/product-default"}
					return nil
				}).
				WithPodOverrides(&corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: name, Command: []string{"/bin/user-override"}},
						},
					},
				}).
				Build()

			// The customizer ran before the strategic merge, so the user's override has the last
			// word — the same precedence the StatefulSet builder guarantees.
			Expect(deployment.Spec.Template.Spec.Containers[0].Command).To(Equal([]string{"/bin/user-override"}))
			Expect(deploymentBuilder.PodOverrideViolations()).To(BeNil())
		})

		It("should record a customizer error as a violation instead of dropping it", func() {
			deploymentBuilder.
				WithImage(image, "").
				WithMainContainerCustomizer(func(c *corev1.Container) error {
					return fmt.Errorf("boom")
				}).
				Build()

			violations := deploymentBuilder.MainContainerViolations()
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Error()).To(ContainSubstring("boom"))
		})

		It("should reject an image change and restore the resolved image", func() {
			deployment := deploymentBuilder.
				WithImage(image, "").
				WithMainContainerCustomizer(func(c *corev1.Container) error {
					c.Image = "sneaky:latest"
					return nil
				}).
				Build()

			// The image is resolved once and propagated to the sidecars before the workload is
			// built, so the change is recorded and undone rather than accepted.
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal(image))
			violations := deploymentBuilder.MainContainerViolations()
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Error()).To(ContainSubstring("changed the image"))
		})

		It("should reset the violations on a rebuild", func() {
			deploymentBuilder.
				WithImage(image, "").
				WithMainContainerCustomizer(func(c *corev1.Container) error {
					return fmt.Errorf("boom")
				}).
				Build()
			Expect(deploymentBuilder.MainContainerViolations()).To(HaveLen(1))

			deploymentBuilder.WithMainContainerCustomizer(func(c *corev1.Container) error { return nil }).Build()
			Expect(deploymentBuilder.MainContainerViolations()).To(BeNil())
		})
	})

	Describe("pod overrides", func() {
		It("should strategic-merge overrides onto the built template", func() {
			deployment := deploymentBuilder.
				WithImage(image, "").
				WithTerminationGracePeriod(30).
				WithPodOverrides(&corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{"custom": "value"},
					},
					Spec: corev1.PodSpec{
						TerminationGracePeriodSeconds: ptr.To(int64(300)),
					},
				}).
				Build()

			// The override wins over the config-declared grace period, and the built values the
			// override does not state survive.
			Expect(*deployment.Spec.Template.Spec.TerminationGracePeriodSeconds).To(Equal(int64(300)))
			Expect(deployment.Spec.Template.Annotations).To(HaveKeyWithValue("custom", "value"))
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
		})

		It("should re-assert the selector labels after the merge", func() {
			deployment := deploymentBuilder.
				WithImage(image, "").
				WithSelectorLabels(map[string]string{"app": "test"}).
				WithPodOverrides(&corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"extra": "label"},
					},
				}).
				Build()

			// An override may add labels but can never break the invariant that the immutable
			// .spec.selector matches the template labels.
			Expect(deployment.Spec.Template.Labels).To(HaveKeyWithValue("app", "test"))
			Expect(deployment.Spec.Template.Labels).To(HaveKeyWithValue("extra", "label"))
		})

		It("should record a displaced framework mount as a violation", func() {
			deployment := deploymentBuilder.
				WithImage(image, "").
				AddVolume(corev1.Volume{
					Name: "config",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "cfg"},
						},
					},
				}).
				AddVolumeMount(corev1.VolumeMount{Name: "config", MountPath: "/etc/config"}).
				WithPodOverrides(&corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{{
							Name: "my-volume",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						}},
						Containers: []corev1.Container{{
							Name: name,
							VolumeMounts: []corev1.VolumeMount{
								// Strategic merge keys volumeMounts by mountPath, so this REPLACES
								// the framework's config mount instead of joining it.
								{Name: "my-volume", MountPath: "/etc/config"},
							},
						}},
					},
				}).
				Build()

			Expect(deployment).NotTo(BeNil())
			violations := deploymentBuilder.PodOverrideViolations()
			Expect(violations).NotTo(BeEmpty())
			Expect(violations[0].Error()).To(ContainSubstring("displaced"))
		})
	})

	Describe("probes", func() {
		It("should auto-generate a TCP readiness probe on the first declared port and no liveness probe", func() {
			deployment := deploymentBuilder.
				WithImage(image, "").
				AddPort("http", 8080, corev1.ProtocolTCP).
				Build()

			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.ReadinessProbe).NotTo(BeNil())
			Expect(container.ReadinessProbe.TCPSocket.Port.IntValue()).To(Equal(8080))
			Expect(container.LivenessProbe).To(BeNil())
		})
	})

	Describe("NamespacedName", func() {
		It("should return the NamespacedName", func() {
			nn := deploymentBuilder.NamespacedName()
			Expect(nn.Name).To(Equal(name))
			Expect(nn.Namespace).To(Equal(namespace))
		})
	})
})
