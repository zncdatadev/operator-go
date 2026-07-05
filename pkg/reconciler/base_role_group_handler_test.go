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

package reconciler_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/config"
	"github.com/zncdatadev/operator-go/pkg/constant"
	"github.com/zncdatadev/operator-go/pkg/productlogging"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/sidecar"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	"github.com/zncdatadev/operator-go/pkg/vector"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("BaseRoleGroupHandler", func() {
	var handler *reconciler.BaseRoleGroupHandler[common.ClusterInterface]
	var ctx context.Context
	var mockCR *testutil.ClusterWrapper
	var buildCtx *reconciler.RoleGroupBuildContext

	BeforeEach(func() {
		ctx = context.Background()
		handler = reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("test-image:latest", testScheme)

		mockCluster := testutil.NewMockCluster("test-cluster", "default")
		mockCR = testutil.WrapMockCluster(mockCluster)

		buildCtx = &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			ClusterLabels: map[string]string{
				"app.kubernetes.io/name": "test-cluster",
			},
			ClusterSpec: &v1alpha1.GenericClusterSpec{
				Roles: map[string]v1alpha1.RoleSpec{
					"test-role": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{
							"default": {Replicas: ptr.To(int32(3))},
						},
					},
				},
			},
			RoleName:      "test-role",
			RoleSpec:      &v1alpha1.RoleSpec{},
			RoleGroupName: "default",
			RoleGroupSpec: v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(3))},
			MergedConfig:  &config.MergedConfig{},
			ResourceName:  "test-cluster-default",
		}
	})

	Describe("NewBaseRoleGroupHandler", func() {
		It("should create a handler with default values", func() {
			Expect(handler).NotTo(BeNil())
			Expect(handler.Image).To(Equal("test-image:latest"))
			Expect(handler.ImagePullPolicy).To(Equal(corev1.PullIfNotPresent))
			Expect(handler.RoleImages).NotTo(BeNil())
			Expect(handler.RoleContainerPorts).NotTo(BeNil())
			Expect(handler.RoleServicePorts).NotTo(BeNil())
			Expect(handler.ExtraLabels).NotTo(BeNil())
			Expect(handler.ExtraAnnotations).NotTo(BeNil())
		})

		It("should set the scheme correctly", func() {
			Expect(handler.Scheme).To(Equal(testScheme))
		})
	})

	Describe("SetRoleImage", func() {
		It("should set image for a role", func() {
			handler.SetRoleImage("test-role", "custom-image:v2")
			Expect(handler.RoleImages["test-role"]).To(Equal("custom-image:v2"))
		})

		It("should initialize RoleImages if nil", func() {
			nilHandler := &reconciler.BaseRoleGroupHandler[common.ClusterInterface]{}
			Expect(nilHandler.RoleImages).To(BeNil())
			nilHandler.SetRoleImage("role", "image:v1")
			Expect(nilHandler.RoleImages).NotTo(BeNil())
			Expect(nilHandler.RoleImages["role"]).To(Equal("image:v1"))
		})
	})

	Describe("SetRoleContainerPorts", func() {
		It("should set ports for a role", func() {
			testPorts := []corev1.ContainerPort{{Name: "http", ContainerPort: 8080}}
			handler.SetRoleContainerPorts("web", testPorts)
			Expect(handler.RoleContainerPorts["web"]).To(Equal(testPorts))
		})

		It("should initialize RoleContainerPorts if nil", func() {
			nilHandler := &reconciler.BaseRoleGroupHandler[common.ClusterInterface]{}
			Expect(nilHandler.RoleContainerPorts).To(BeNil())
			nilHandler.SetRoleContainerPorts("role", []corev1.ContainerPort{{Name: "http", ContainerPort: 8080}})
			Expect(nilHandler.RoleContainerPorts).NotTo(BeNil())
		})
	})

	Describe("SetRoleServicePorts", func() {
		It("should set ports for a role", func() {
			testPorts := []corev1.ServicePort{{Name: "http", Port: 80}}
			handler.SetRoleServicePorts("web", testPorts)
			Expect(handler.RoleServicePorts["web"]).To(Equal(testPorts))
		})

		It("should initialize RoleServicePorts if nil", func() {
			nilHandler := &reconciler.BaseRoleGroupHandler[common.ClusterInterface]{}
			Expect(nilHandler.RoleServicePorts).To(BeNil())
			nilHandler.SetRoleServicePorts("role", []corev1.ServicePort{})
			Expect(nilHandler.RoleServicePorts).NotTo(BeNil())
		})
	})

	Describe("BuildResources", func() {
		It("should build all resources successfully", func() {
			handler.SetRoleServicePorts("test-role", []corev1.ServicePort{
				{Name: "http", Port: 8080, TargetPort: intstr.FromInt(8080)},
			})

			resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)

			Expect(err).NotTo(HaveOccurred())
			Expect(resources).NotTo(BeNil())
			Expect(resources.ConfigMap).NotTo(BeNil())
			Expect(resources.HeadlessService).NotTo(BeNil())
			Expect(resources.Service).NotTo(BeNil())
			Expect(resources.StatefulSet).NotTo(BeNil())
		})

		It("should build resources without Service when no ports defined", func() {
			resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)

			Expect(err).NotTo(HaveOccurred())
			Expect(resources.Service).To(BeNil())
		})

		It("should build ConfigMap with correct name and namespace", func() {
			resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)

			Expect(err).NotTo(HaveOccurred())
			Expect(resources.ConfigMap.Name).To(Equal("test-cluster-default"))
			Expect(resources.ConfigMap.Namespace).To(Equal("default"))
		})

		It("should build HeadlessService with correct name", func() {
			resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)

			Expect(err).NotTo(HaveOccurred())
			Expect(resources.HeadlessService.Name).To(Equal("test-cluster-default-headless"))
			Expect(resources.HeadlessService.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
		})

		It("should build StatefulSet with correct configuration", func() {
			resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)

			Expect(err).NotTo(HaveOccurred())
			Expect(resources.StatefulSet.Name).To(Equal("test-cluster-default"))
			Expect(resources.StatefulSet.Namespace).To(Equal("default"))
		})

		It("should not build PDB when not configured", func() {
			resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)

			Expect(err).NotTo(HaveOccurred())
			Expect(resources.PodDisruptionBudget).To(BeNil())
		})

		It("should build PDB when configured", func() {
			maxUnavailable := int32(1)
			buildCtx.RoleSpec = &v1alpha1.RoleSpec{
				RoleConfig: &v1alpha1.RoleConfigSpec{
					PodDisruptionBudget: &v1alpha1.PodDisruptionBudgetSpec{
						Enabled:        true,
						MaxUnavailable: &maxUnavailable,
					},
				},
			}

			resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)

			Expect(err).NotTo(HaveOccurred())
			Expect(resources.PodDisruptionBudget).NotTo(BeNil())
			Expect(resources.PodDisruptionBudget.Name).To(Equal("test-cluster-default"))
		})

		It("should not build PDB when disabled", func() {
			enabled := false
			buildCtx.RoleSpec = &v1alpha1.RoleSpec{
				RoleConfig: &v1alpha1.RoleConfigSpec{
					PodDisruptionBudget: &v1alpha1.PodDisruptionBudgetSpec{
						Enabled: enabled,
					},
				},
			}

			resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)

			Expect(err).NotTo(HaveOccurred())
			Expect(resources.PodDisruptionBudget).To(BeNil())
		})

		It("should include extra labels in all resources", func() {
			handler.ExtraLabels["custom-label"] = "custom-value"
			handler.SetRoleServicePorts("test-role", []corev1.ServicePort{
				{Name: "http", Port: 8080},
			})

			resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)

			Expect(err).NotTo(HaveOccurred())
			Expect(resources.ConfigMap.Labels["custom-label"]).To(Equal("custom-value"))
			Expect(resources.HeadlessService.Labels["custom-label"]).To(Equal("custom-value"))
			Expect(resources.Service.Labels["custom-label"]).To(Equal("custom-value"))
			Expect(resources.StatefulSet.Labels["custom-label"]).To(Equal("custom-value"))
		})

		It("should include extra annotations in all resources", func() {
			handler.ExtraAnnotations["custom-annotation"] = "annotation-value"
			handler.SetRoleServicePorts("test-role", []corev1.ServicePort{
				{Name: "http", Port: 8080},
			})

			resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)

			Expect(err).NotTo(HaveOccurred())
			Expect(resources.ConfigMap.Annotations["custom-annotation"]).To(Equal("annotation-value"))
			Expect(resources.HeadlessService.Annotations["custom-annotation"]).To(Equal("annotation-value"))
			Expect(resources.Service.Annotations["custom-annotation"]).To(Equal("annotation-value"))
			Expect(resources.StatefulSet.Annotations["custom-annotation"]).To(Equal("annotation-value"))
		})

		It("should build ConfigMap with config files", func() {
			buildCtx.MergedConfig = &config.MergedConfig{
				ConfigFiles: map[string]map[string]string{
					"config.properties": {
						"key1": "value1",
						"key2": "value2",
					},
				},
			}

			resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)

			Expect(err).NotTo(HaveOccurred())
			Expect(resources.ConfigMap.Data).To(HaveKey("config.properties"))
		})

		It("should include standard labels in resources", func() {
			resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)

			Expect(err).NotTo(HaveOccurred())
			Expect(resources.ConfigMap.Labels["app.kubernetes.io/instance"]).To(Equal("test-cluster"))
			Expect(resources.ConfigMap.Labels["app.kubernetes.io/component"]).To(Equal("test-role"))
			Expect(resources.ConfigMap.Labels["app.kubernetes.io/managed-by"]).To(Equal("operator-go"))
		})

		It("should include role group label in resources", func() {
			resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)

			Expect(err).NotTo(HaveOccurred())
			Expect(resources.ConfigMap.Labels["test-cluster-default"]).To(Equal("true"))
		})
	})

	// The shared log volume (creation + sizing) is now owned by the Vector provider; its behavior
	// is covered by pkg/vector provider tests and the "end-to-end" specs in the declarative
	// logging block below. An invalid LogVolumeSize override is handled by the GenericReconciler's
	// buildSidecarManager (logs and falls back to the default; never panics).

	Describe("FetchConfigMap", func() {
		It("should return error when ConfigMap does not exist", func() {
			_, err := handler.FetchConfigMap(ctx, k8sClient, "default", "non-existent")
			Expect(err).To(HaveOccurred())
		})

		It("should return ConfigMap when it exists", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fetch-cm",
					Namespace: "default",
				},
				Data: map[string]string{"key": "value"},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			fetched, err := handler.FetchConfigMap(ctx, k8sClient, "default", "test-fetch-cm")
			Expect(err).NotTo(HaveOccurred())
			Expect(fetched).NotTo(BeNil())
			Expect(fetched.Data["key"]).To(Equal("value"))

			Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
		})
	})

	Describe("FetchSecret", func() {
		It("should return error when Secret does not exist", func() {
			_, err := handler.FetchSecret(ctx, k8sClient, "default", "non-existent")
			Expect(err).To(HaveOccurred())
		})

		It("should return Secret when it exists", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fetch-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{"password": []byte("secret")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			fetched, err := handler.FetchSecret(ctx, k8sClient, "default", "test-fetch-secret")
			Expect(err).NotTo(HaveOccurred())
			Expect(fetched).NotTo(BeNil())
			Expect(fetched.Data["password"]).To(Equal([]byte("secret")))

			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
		})
	})

	Describe("RoleGroupHandler interface compliance", func() {
		It("should implement RoleGroupHandler interface", func() {
			var _ reconciler.RoleGroupHandler[common.ClusterInterface] = handler
		})
	})
})

var _ = Describe("RoleGroupHandlerFuncs", func() {
	var funcs *reconciler.RoleGroupHandlerFuncs[common.ClusterInterface]

	BeforeEach(func() {
		funcs = &reconciler.RoleGroupHandlerFuncs[common.ClusterInterface]{}
	})

	Describe("BuildResources", func() {
		It("should return empty resources when function is nil", func() {
			resources, err := funcs.BuildResources(context.Background(), nil, nil, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(resources).NotTo(BeNil())
		})

		It("should call the function when set", func() {
			called := false
			funcs.BuildResourcesFunc = func(ctx context.Context, k8sClient client.Client, cr common.ClusterInterface, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
				called = true
				return &reconciler.RoleGroupResources{}, nil
			}

			_, _ = funcs.BuildResources(context.Background(), nil, nil, nil)
			Expect(called).To(BeTrue())
		})
	})

	Describe("Interface compliance", func() {
		It("should implement RoleGroupHandler interface", func() {
			var _ reconciler.RoleGroupHandler[common.ClusterInterface] = funcs
		})
	})
})

var _ = Describe("PodDisruptionBudget building", func() {
	var handler *reconciler.BaseRoleGroupHandler[common.ClusterInterface]
	var buildCtx *reconciler.RoleGroupBuildContext

	BeforeEach(func() {
		handler = reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("test-image:latest", testScheme)
		buildCtx = &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			ClusterLabels:    map[string]string{},
			RoleName:         "test-role",
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    "default",
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{},
			MergedConfig:     &config.MergedConfig{},
			ResourceName:     "test-cluster-default",
		}
	})

	It("should set maxUnavailable correctly", func() {
		maxUnavailable := int32(2)
		buildCtx.RoleSpec.RoleConfig = &v1alpha1.RoleConfigSpec{
			PodDisruptionBudget: &v1alpha1.PodDisruptionBudgetSpec{
				Enabled:        true,
				MaxUnavailable: &maxUnavailable,
			},
		}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.PodDisruptionBudget).NotTo(BeNil())
		Expect(resources.PodDisruptionBudget.Spec.MaxUnavailable.IntVal).To(Equal(int32(2)))
	})

	It("should set selector with correct labels", func() {
		buildCtx.RoleSpec.RoleConfig = &v1alpha1.RoleConfigSpec{
			PodDisruptionBudget: &v1alpha1.PodDisruptionBudgetSpec{
				Enabled: true,
			},
		}
		buildCtx.ClusterLabels["app"] = "test"

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.PodDisruptionBudget.Spec.Selector).NotTo(BeNil())
		Expect(resources.PodDisruptionBudget.Spec.Selector.MatchLabels).NotTo(BeEmpty())
	})
})

var _ = Describe("StatefulSet building", func() {
	var handler *reconciler.BaseRoleGroupHandler[common.ClusterInterface]
	var buildCtx *reconciler.RoleGroupBuildContext

	BeforeEach(func() {
		handler = reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("test-image:latest", testScheme)
		buildCtx = &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			ClusterLabels:    map[string]string{},
			RoleName:         "test-role",
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    "default",
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(3))},
			MergedConfig:     &config.MergedConfig{},
			ResourceName:     "test-cluster-default",
		}
	})

	It("should set replicas correctly", func() {
		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.StatefulSet.Spec.Replicas).NotTo(BeNil())
		Expect(*resources.StatefulSet.Spec.Replicas).To(Equal(int32(3)))
	})

	It("should set image correctly", func() {
		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		containers := resources.StatefulSet.Spec.Template.Spec.Containers
		Expect(containers).NotTo(BeEmpty())
		Expect(containers[0].Image).To(Equal("test-image:latest"))
	})

	It("should bind the ServiceAccount to the pod template when configured", func() {
		buildCtx.ServiceAccountName = "test-cluster-sa"
		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.StatefulSet.Spec.Template.Spec.ServiceAccountName).To(Equal("test-cluster-sa"))
	})

	It("should leave ServiceAccountName unset when not configured", func() {
		// buildCtx.ServiceAccountName defaults to "" — backward compatible: pods use the
		// namespace default SA, the pod template ServiceAccountName must stay empty.
		Expect(buildCtx.ServiceAccountName).To(BeEmpty())
		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.StatefulSet.Spec.Template.Spec.ServiceAccountName).To(BeEmpty())
	})

	It("should add config volume when config files present", func() {
		buildCtx.MergedConfig = &config.MergedConfig{
			ConfigFiles: map[string]map[string]string{
				"config.properties": {"key": "value"},
			},
		}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())

		volumes := resources.StatefulSet.Spec.Template.Spec.Volumes
		var configVolume *corev1.Volume
		for i := range volumes {
			if volumes[i].Name == "config" {
				configVolume = &volumes[i]
				break
			}
		}
		Expect(configVolume).NotTo(BeNil())
		Expect(configVolume.ConfigMap).NotTo(BeNil())
		Expect(configVolume.ConfigMap.Name).To(Equal("test-cluster-default"))
	})

	It("should add config volume mount when config files present", func() {
		buildCtx.MergedConfig = &config.MergedConfig{
			ConfigFiles: map[string]map[string]string{
				"config.properties": {"key": "value"},
			},
		}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())

		containers := resources.StatefulSet.Spec.Template.Spec.Containers
		Expect(containers).NotTo(BeEmpty())
		var configMount *corev1.VolumeMount
		for i := range containers[0].VolumeMounts {
			if containers[0].VolumeMounts[i].Name == "config" {
				configMount = &containers[0].VolumeMounts[i]
				break
			}
		}
		Expect(configMount).NotTo(BeNil())
		// With no ConfigMountPath set, the config volume mounts at the kubedoop-canonical
		// config mount path, not the old foreign "/etc/config".
		Expect(configMount.MountPath).To(Equal(constant.KubedoopConfigDirMount))
		Expect(configMount.ReadOnly).To(BeTrue())
	})

	It("should honor ConfigMountPath override for the config volume mount", func() {
		handler.ConfigMountPath = "/etc/trino"
		buildCtx.MergedConfig = &config.MergedConfig{
			ConfigFiles: map[string]map[string]string{
				"config.properties": {"key": "value"},
			},
		}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())

		containers := resources.StatefulSet.Spec.Template.Spec.Containers
		Expect(containers).NotTo(BeEmpty())
		var configMount *corev1.VolumeMount
		for i := range containers[0].VolumeMounts {
			if containers[0].VolumeMounts[i].Name == "config" {
				configMount = &containers[0].VolumeMounts[i]
				break
			}
		}
		Expect(configMount).NotTo(BeNil())
		Expect(configMount.MountPath).To(Equal("/etc/trino"))
		Expect(configMount.ReadOnly).To(BeTrue())
	})

	It("should add config volume and mount even when no config-file overrides are present", func() {
		// The role group ConfigMap is always produced by buildConfigMap (a product may populate
		// its real config directly into ConfigMap.Data with no overrides). MergedConfig.ConfigFiles
		// is empty here, yet the config volume + mount must still be present so the product can read
		// its config. The mount must NOT be gated on ConfigFiles.
		buildCtx.MergedConfig = &config.MergedConfig{}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())

		volumes := resources.StatefulSet.Spec.Template.Spec.Volumes
		var configVolume *corev1.Volume
		for i := range volumes {
			if volumes[i].Name == "config" {
				configVolume = &volumes[i]
				break
			}
		}
		Expect(configVolume).NotTo(BeNil())
		Expect(configVolume.ConfigMap).NotTo(BeNil())
		Expect(configVolume.ConfigMap.Name).To(Equal("test-cluster-default"))

		containers := resources.StatefulSet.Spec.Template.Spec.Containers
		Expect(containers).NotTo(BeEmpty())
		var configMount *corev1.VolumeMount
		for i := range containers[0].VolumeMounts {
			if containers[0].VolumeMounts[i].Name == "config" {
				configMount = &containers[0].VolumeMounts[i]
				break
			}
		}
		Expect(configMount).NotTo(BeNil())
		Expect(configMount.MountPath).To(Equal(constant.KubedoopConfigDirMount))
		Expect(configMount.ReadOnly).To(BeTrue())
	})

	It("should use role-specific image when set", func() {
		handler.SetRoleImage("test-role", "custom-role-image:v2")
		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		containers := resources.StatefulSet.Spec.Template.Spec.Containers
		Expect(containers).NotTo(BeEmpty())
		Expect(containers[0].Image).To(Equal("custom-role-image:v2"))
	})

	It("should set resources when configured", func() {
		buildCtx.RoleGroupSpec.Config = &v1alpha1.RoleGroupConfigSpec{
			Resources: &v1alpha1.ResourcesSpec{
				CPU:    &v1alpha1.CPUResource{},
				Memory: &v1alpha1.MemoryResource{},
			},
		}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.StatefulSet.Spec.Template.Spec.Containers).NotTo(BeEmpty())
	})

	It("should set pod overrides when configured", func() {
		buildCtx.MergedConfig = &config.MergedConfig{
			PodOverrides: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					PriorityClassName: "high-priority",
				},
			},
		}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.StatefulSet.Spec.Template.Spec.PriorityClassName).To(Equal("high-priority"))
	})

	It("should default EnableServiceLinks to false (kubedoop standard)", func() {
		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())

		esl := resources.StatefulSet.Spec.Template.Spec.EnableServiceLinks
		Expect(esl).NotTo(BeNil())
		Expect(*esl).To(BeFalse())
	})

	It("should let PodOverrides override EnableServiceLinks to true", func() {
		buildCtx.MergedConfig = &config.MergedConfig{
			PodOverrides: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					EnableServiceLinks: ptr.To(true),
				},
			},
		}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())

		esl := resources.StatefulSet.Spec.Template.Spec.EnableServiceLinks
		Expect(esl).NotTo(BeNil())
		Expect(*esl).To(BeTrue())
	})

	It("should set container ports when configured", func() {
		handler.SetRoleContainerPorts("test-role", []corev1.ContainerPort{
			{Name: "http", ContainerPort: 8080},
			{Name: "https", ContainerPort: 8443},
		})

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		containers := resources.StatefulSet.Spec.Template.Spec.Containers
		Expect(containers).NotTo(BeEmpty())
		Expect(containers[0].Ports).To(HaveLen(2))
	})

	It("should apply the canonical default security context (1001 identity + hardening) when nothing is configured", func() {
		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())

		podSpec := resources.StatefulSet.Spec.Template.Spec

		// Pod-level default: kubedoop org-standard identity (uid 1001, gid 0, fsGroup 1001) +
		// RunAsNonRoot + RuntimeDefault seccomp.
		Expect(podSpec.SecurityContext).NotTo(BeNil())
		Expect(podSpec.SecurityContext.RunAsUser).NotTo(BeNil())
		Expect(*podSpec.SecurityContext.RunAsUser).To(Equal(int64(1001)))
		Expect(podSpec.SecurityContext.RunAsGroup).NotTo(BeNil())
		Expect(*podSpec.SecurityContext.RunAsGroup).To(Equal(int64(0)))
		Expect(podSpec.SecurityContext.FSGroup).NotTo(BeNil())
		Expect(*podSpec.SecurityContext.FSGroup).To(Equal(int64(1001)))
		Expect(podSpec.SecurityContext.RunAsNonRoot).NotTo(BeNil())
		Expect(*podSpec.SecurityContext.RunAsNonRoot).To(BeTrue())
		Expect(podSpec.SecurityContext.SeccompProfile).NotTo(BeNil())
		Expect(podSpec.SecurityContext.SeccompProfile.Type).To(Equal(corev1.SeccompProfileTypeRuntimeDefault))

		// Container-level default: uid 1001, gid 0, hardened (drop ALL caps, no privilege escalation).
		Expect(podSpec.Containers).NotTo(BeEmpty())
		csc := podSpec.Containers[0].SecurityContext
		Expect(csc).NotTo(BeNil())
		Expect(csc.RunAsUser).NotTo(BeNil())
		Expect(*csc.RunAsUser).To(Equal(int64(1001)))
		Expect(csc.RunAsGroup).NotTo(BeNil())
		Expect(*csc.RunAsGroup).To(Equal(int64(0)))
		Expect(csc.RunAsNonRoot).NotTo(BeNil())
		Expect(*csc.RunAsNonRoot).To(BeTrue())
		Expect(csc.AllowPrivilegeEscalation).NotTo(BeNil())
		Expect(*csc.AllowPrivilegeEscalation).To(BeFalse())
		Expect(csc.Capabilities).NotTo(BeNil())
		Expect(csc.Capabilities.Drop).To(ContainElement(corev1.Capability("ALL")))
		Expect(csc.SeccompProfile).NotTo(BeNil())
		Expect(csc.SeccompProfile.Type).To(Equal(corev1.SeccompProfileTypeRuntimeDefault))
	})

	It("should let PodOverrides REPLACE the default pod security context (no deep merge)", func() {
		// The override sets only RunAsUser. Replace semantics mean the rest of the default
		// (RunAsGroup, FSGroup, RunAsNonRoot, SeccompProfile) is wiped, not merged on top.
		buildCtx.MergedConfig = &config.MergedConfig{
			PodOverrides: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser: ptr.To(int64(1234)),
					},
				},
			},
		}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())

		podSC := resources.StatefulSet.Spec.Template.Spec.SecurityContext
		Expect(podSC).NotTo(BeNil())
		Expect(podSC.RunAsUser).NotTo(BeNil())
		Expect(*podSC.RunAsUser).To(Equal(int64(1234)))
		// Negative assertions documenting REPLACE (not merge): default hardening fields are gone.
		Expect(podSC.FSGroup).To(BeNil())
		Expect(podSC.RunAsGroup).To(BeNil())
		Expect(podSC.RunAsNonRoot).To(BeNil())
		Expect(podSC.SeccompProfile).To(BeNil())
	})

	It("should let PodOverrides REPLACE the default container security context (no deep merge)", func() {
		buildCtx.MergedConfig = &config.MergedConfig{
			PodOverrides: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "test-cluster-default",
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: ptr.To(int64(4321)),
							},
						},
					},
				},
			},
		}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())

		containers := resources.StatefulSet.Spec.Template.Spec.Containers
		Expect(containers).NotTo(BeEmpty())
		csc := containers[0].SecurityContext
		Expect(csc).NotTo(BeNil())
		Expect(csc.RunAsUser).NotTo(BeNil())
		Expect(*csc.RunAsUser).To(Equal(int64(4321)))
		// Negative assertions documenting REPLACE (not merge): default hardening fields are gone.
		Expect(csc.AllowPrivilegeEscalation).To(BeNil())
		Expect(csc.Capabilities).To(BeNil())
		Expect(csc.RunAsNonRoot).To(BeNil())
		Expect(csc.SeccompProfile).To(BeNil())
	})

	It("should allow disabling the default security context", func() {
		handler.WithoutDefaultSecurityContext()

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())

		podSpec := resources.StatefulSet.Spec.Template.Spec
		Expect(podSpec.SecurityContext).To(BeNil())
		Expect(podSpec.Containers).NotTo(BeEmpty())
		Expect(podSpec.Containers[0].SecurityContext).To(BeNil())
	})
})

var _ = Describe("ConfigGenerator integration", func() {
	var handler *reconciler.BaseRoleGroupHandler[common.ClusterInterface]
	var buildCtx *reconciler.RoleGroupBuildContext

	BeforeEach(func() {
		handler = reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("test-image:latest", testScheme)
		buildCtx = &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			ClusterLabels:    map[string]string{},
			RoleName:         "test-role",
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    "default",
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(1))},
			MergedConfig:     &config.MergedConfig{},
			ResourceName:     "test-cluster-default",
		}
	})

	It("should use ConfigGenerator when set and config files present", func() {
		buildCtx.MergedConfig = &config.MergedConfig{
			ConfigFiles: map[string]map[string]string{
				"server.properties": {
					"port": "8080",
				},
			},
		}

		generator := config.NewMultiFormatConfigGenerator()
		generator.RegisterFormat("server.properties", config.GetFormat(config.FormatProperties))
		handler.ConfigGenerator = generator

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.ConfigMap).NotTo(BeNil())
		Expect(resources.ConfigMap.Data).To(HaveKey("server.properties"))
	})

	It("should not use ConfigGenerator when no config files", func() {
		buildCtx.MergedConfig = &config.MergedConfig{
			ConfigFiles: map[string]map[string]string{},
		}

		generator := config.NewMultiFormatConfigGenerator()
		handler.ConfigGenerator = generator

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.ConfigMap).NotTo(BeNil())
	})

	It("should build ConfigMap with both basic config and generated config", func() {
		buildCtx.MergedConfig = &config.MergedConfig{
			ConfigFiles: map[string]map[string]string{
				"basic.properties": {
					"key": "value",
				},
			},
		}

		generator := config.NewMultiFormatConfigGenerator()
		generator.RegisterFormat("basic.properties", config.GetFormat(config.FormatProperties))
		handler.ConfigGenerator = generator

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.ConfigMap).NotTo(BeNil())
		Expect(resources.ConfigMap.Data).To(HaveKey("basic.properties"))
	})
})

var _ = Describe("BaseRoleGroupHandler extra labels and annotations", func() {
	var handler *reconciler.BaseRoleGroupHandler[common.ClusterInterface]
	var buildCtx *reconciler.RoleGroupBuildContext

	BeforeEach(func() {
		handler = reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("test-image:latest", testScheme)
		buildCtx = &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			ClusterLabels:    map[string]string{"app": "myapp", "env": "test"},
			RoleName:         "test-role",
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    "default",
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(1))},
			MergedConfig:     &config.MergedConfig{},
			ResourceName:     "test-cluster-default",
		}
	})

	It("should include extra labels in all resources", func() {
		handler.ExtraLabels = map[string]string{"custom-label": "custom-value", "team": "platform"}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.ConfigMap.Labels).To(HaveKey("custom-label"))
		Expect(resources.ConfigMap.Labels).To(HaveKey("team"))
		Expect(resources.StatefulSet.Labels).To(HaveKey("custom-label"))
		Expect(resources.HeadlessService.Labels).To(HaveKey("custom-label"))
	})

	It("should include extra annotations in resources", func() {
		handler.ExtraAnnotations = map[string]string{"custom-annotation": "annotation-value"}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.ConfigMap.Annotations).To(HaveKey("custom-annotation"))
	})

	It("should merge cluster labels with standard labels", func() {
		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.ConfigMap.Labels).To(HaveKey("app"))
		Expect(resources.ConfigMap.Labels).To(HaveKey("app.kubernetes.io/instance"))
		Expect(resources.ConfigMap.Labels).To(HaveKey("app.kubernetes.io/component"))
		Expect(resources.ConfigMap.Labels["app.kubernetes.io/instance"]).To(Equal("test-cluster"))
		Expect(resources.ConfigMap.Labels["app.kubernetes.io/component"]).To(Equal("test-role"))
	})
})

var _ = Describe("BaseRoleGroupHandler with PDB", func() {
	var handler *reconciler.BaseRoleGroupHandler[common.ClusterInterface]
	var buildCtx *reconciler.RoleGroupBuildContext

	BeforeEach(func() {
		handler = reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("test-image:latest", testScheme)
		buildCtx = &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			ClusterLabels:    map[string]string{},
			RoleName:         "test-role",
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    "default",
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(3))},
			MergedConfig:     &config.MergedConfig{},
			ResourceName:     "test-cluster-default",
		}
	})

	It("should create PDB when MaxUnavailable is set and Enabled is true", func() {
		maxUnavailable := int32(1)
		buildCtx.RoleSpec = &v1alpha1.RoleSpec{
			RoleConfig: &v1alpha1.RoleConfigSpec{
				PodDisruptionBudget: &v1alpha1.PodDisruptionBudgetSpec{
					Enabled:        true,
					MaxUnavailable: &maxUnavailable,
				},
			},
		}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.PodDisruptionBudget).NotTo(BeNil())
		Expect(resources.PodDisruptionBudget.Spec.MaxUnavailable).NotTo(BeNil())
	})

	It("should not create PDB when Enabled is false", func() {
		maxUnavailable := int32(1)
		buildCtx.RoleSpec = &v1alpha1.RoleSpec{
			RoleConfig: &v1alpha1.RoleConfigSpec{
				PodDisruptionBudget: &v1alpha1.PodDisruptionBudgetSpec{
					Enabled:        false,
					MaxUnavailable: &maxUnavailable,
				},
			},
		}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.PodDisruptionBudget).To(BeNil())
	})

	It("should not create PDB when PodDisruptionBudget is nil", func() {
		buildCtx.RoleSpec = &v1alpha1.RoleSpec{
			RoleConfig: &v1alpha1.RoleConfigSpec{
				PodDisruptionBudget: nil,
			},
		}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.PodDisruptionBudget).To(BeNil())
	})

	It("should not create PDB when RoleConfig is nil", func() {
		buildCtx.RoleSpec = &v1alpha1.RoleSpec{
			RoleConfig: nil,
		}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.PodDisruptionBudget).To(BeNil())
	})
})

var _ = Describe("BaseRoleGroupHandler enhancements", func() {
	var ctx context.Context
	var mockCR *testutil.ClusterWrapper

	newBuildCtx := func(storage *v1alpha1.StorageResource) *reconciler.RoleGroupBuildContext {
		cfg := &v1alpha1.RoleGroupConfigSpec{}
		if storage != nil {
			cfg.Resources = &v1alpha1.ResourcesSpec{Storage: storage}
		}
		return &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			RoleName:         "server",
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    "default",
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(3)), Config: cfg},
			MergedConfig:     &config.MergedConfig{},
			ResourceName:     reconciler.RoleGroupResourceName("test-cluster", "server", "default"),
		}
	}

	var buildCtx *reconciler.RoleGroupBuildContext

	BeforeEach(func() {
		ctx = context.Background()
		mockCR = testutil.WrapMockCluster(testutil.NewMockCluster("test-cluster", "default"))
		buildCtx = newBuildCtx(&v1alpha1.StorageResource{Capacity: resource.MustParse("10Gi")})
	})

	It("creates a data PVC from storage when StorageMountPath is set", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("img:1", testScheme)
		handler.StorageMountPath = "/kubedoop/data"

		resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.StatefulSet.Spec.VolumeClaimTemplates).To(HaveLen(1))
		pvc := resources.StatefulSet.Spec.VolumeClaimTemplates[0]
		q := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		Expect(q.String()).To(Equal("10Gi"))
		mounts := resources.StatefulSet.Spec.Template.Spec.Containers[0].VolumeMounts
		Expect(mounts).To(ContainElement(HaveField("MountPath", "/kubedoop/data")))
	})

	It("does not create a data PVC when StorageMountPath is unset (backward compatible)", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("img:1", testScheme)
		resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.StatefulSet.Spec.VolumeClaimTemplates).To(BeEmpty())
	})

	It("sets PublishNotReadyAddresses on the headless service when enabled", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("img:1", testScheme)
		handler.PublishNotReadyAddresses = true
		resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.HeadlessService.Spec.PublishNotReadyAddresses).To(BeTrue())
	})

	It("uses product-owned identity labels for selectors when LabelDomain is set", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("img:1", testScheme)
		handler.LabelDomain = "zookeeper.kubedoop.dev"
		resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)
		Expect(err).NotTo(HaveOccurred())

		// The immutable StatefulSet selector is the identity subset, decoupled from
		// the descriptive app.kubernetes.io/* labels.
		sel := resources.StatefulSet.Spec.Selector.MatchLabels
		Expect(sel).To(HaveKeyWithValue("zookeeper.kubedoop.dev/cluster", "test-cluster"))
		Expect(sel).To(HaveKeyWithValue("zookeeper.kubedoop.dev/role", "server"))
		Expect(sel).To(HaveKeyWithValue("zookeeper.kubedoop.dev/role-group", "default"))
		Expect(sel).NotTo(HaveKey("app.kubernetes.io/component"))

		// Descriptive labels and identity labels are both on the pod template.
		tmpl := resources.StatefulSet.Spec.Template.Labels
		Expect(tmpl).To(HaveKeyWithValue("app.kubernetes.io/component", "server"))
		Expect(tmpl).To(HaveKeyWithValue("zookeeper.kubedoop.dev/cluster", "test-cluster"))

		// The headless Service selector is identity-only too.
		Expect(resources.HeadlessService.Spec.Selector).To(HaveKey("zookeeper.kubedoop.dev/role"))
		Expect(resources.HeadlessService.Spec.Selector).NotTo(HaveKey("app.kubernetes.io/component"))
	})

	It("falls back to descriptive labels for selectors when LabelDomain is empty", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("img:1", testScheme)
		resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.StatefulSet.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/component", "server"))
	})
})

var _ = Describe("BaseRoleGroupHandler declarative logging", func() {
	It("renders a declared logback container into the role group ConfigMap (Vector enabled emits file appender)", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("test-image:latest", testScheme)
		handler.LoggingContainers = []productlogging.ContainerLogging{{
			Container: "main",
			Framework: productlogging.LoggingFrameworkLogback,
			Pattern:   "%d [myid:%X{myid}] %m%n",
		}}

		mockCR := testutil.WrapMockCluster(testutil.NewMockCluster("test-cluster", "default"))
		buildCtx := &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			RoleName:         "test-role",
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    "default",
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(1))},
			ResourceName:     "test-cluster-default",
			MergedConfig: &config.MergedConfig{
				Logging: &v1alpha1.LoggingSpec{
					EnableVectorAgent: ptr.To(true),
					Containers: map[string]v1alpha1.LoggingConfigSpec{
						"main": {
							Loggers: map[string]*v1alpha1.LogLevelSpec{
								"ROOT":  {Level: "WARN"},
								"org.x": {Level: "DEBUG"},
							},
						},
					},
				},
			},
		}

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.ConfigMap).NotTo(BeNil())

		logback := resources.ConfigMap.Data["logback.xml"]
		Expect(logback).NotTo(BeEmpty())
		Expect(logback).To(ContainSubstring(`<root level="WARN">`))
		Expect(logback).To(ContainSubstring(`<logger name="org.x" level="DEBUG" />`))
		Expect(logback).To(ContainSubstring("<file>/kubedoop/log/main.stdout.log</file>"))
		// The aggregator address was not resolved (buildCtx.VectorAggregatorAddress empty), so the
		// framework leaves vector.yaml to the product.
		Expect(resources.ConfigMap.Data).NotTo(HaveKey("vector.yaml"))
	})

	It("generates vector.yaml when Vector is enabled and the aggregator address is resolved", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("test-image:latest", testScheme)
		handler.LoggingContainers = []productlogging.ContainerLogging{{
			Container: "main",
			Framework: productlogging.LoggingFrameworkLogback,
		}}

		mockCR := testutil.WrapMockCluster(testutil.NewMockCluster("test-cluster", "default"))
		buildCtx := &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			RoleName:         "test-role",
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    "default",
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(1))},
			ResourceName:     "test-cluster-default",
			// The GenericReconciler resolves this from the CR's VectorAggregatorProvider; set it
			// directly here to exercise framework-owned vector.yaml generation.
			VectorAggregatorAddress: "vector-aggregator.default.svc:6123",
			MergedConfig: &config.MergedConfig{
				Logging: &v1alpha1.LoggingSpec{
					EnableVectorAgent: ptr.To(true),
					Containers:        map[string]v1alpha1.LoggingConfigSpec{"main": {}},
				},
			},
		}

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.ConfigMap.Data).To(HaveKey("vector.yaml"))
		Expect(resources.ConfigMap.Data["vector.yaml"]).To(ContainSubstring("vector-aggregator.default.svc:6123"))
	})

	It("Option A: omits the file appender (console-only) when Vector is disabled", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("test-image:latest", testScheme)
		handler.LoggingContainers = []productlogging.ContainerLogging{{
			Container: "main",
			Framework: productlogging.LoggingFrameworkLogback,
		}}

		mockCR := testutil.WrapMockCluster(testutil.NewMockCluster("test-cluster", "default"))
		buildCtx := &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			RoleName:         "test-role",
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    "default",
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(1))},
			ResourceName:     "test-cluster-default",
			// No EnableVectorAgent -> Vector disabled.
			MergedConfig: &config.MergedConfig{
				Logging: &v1alpha1.LoggingSpec{
					Containers: map[string]v1alpha1.LoggingConfigSpec{
						"main": {},
					},
				},
			},
		}

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		logback := resources.ConfigMap.Data["logback.xml"]
		Expect(logback).NotTo(BeEmpty())
		// No file appender is emitted when Vector is disabled (file logging is coupled to Vector).
		Expect(logback).NotTo(ContainSubstring("main.stdout.log"))
		Expect(logback).NotTo(ContainSubstring("RollingFileAppender"))

		// And no shared log volume is created on the pod (the Vector provider owns it and is not
		// wired when the agent is disabled).
		for _, v := range resources.StatefulSet.Spec.Template.Spec.Volumes {
			Expect(v.Name).NotTo(Equal("log"))
		}
	})

	It("end-to-end: the Vector provider creates the shared log volume, RW-mounts producers, RO-mounts itself", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("test-image:latest", testScheme)
		// The base StatefulSet main container is named after the resource name; declare it as
		// the logging container so the Vector provider RW-mounts the shared volume on it.
		handler.LoggingContainers = []productlogging.ContainerLogging{{
			Container: "test-cluster-default",
			Framework: productlogging.LoggingFrameworkLogback,
		}}

		// Wire the Vector provider through a SidecarManager configured with the producer container
		// (the GenericReconciler does this automatically via buildSidecarManager in production; here
		// we set it explicitly to exercise the full assembly — BuildResources -> InjectAll -> the
		// provider owning the volume + all mounts).
		sidecarMgr := sidecar.NewSidecarManager()
		sidecarMgr.Register(
			vector.NewVectorSidecarProvider("test-image:latest",
				vector.WithConfigMapName("test-cluster-default"),
				vector.WithProducers([]string{"test-cluster-default"}),
			),
			&sidecar.SidecarConfig{Enabled: true},
		)

		mockCR := testutil.WrapMockCluster(testutil.NewMockCluster("test-cluster", "default"))
		buildCtx := &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			RoleName:         "test-role",
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    "default",
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(1))},
			ResourceName:     "test-cluster-default",
			SidecarManager:   sidecarMgr,
			MergedConfig: &config.MergedConfig{
				Logging: &v1alpha1.LoggingSpec{
					EnableVectorAgent: ptr.To(true),
					Containers: map[string]v1alpha1.LoggingConfigSpec{
						"test-cluster-default": {},
					},
				},
			},
		}

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR, buildCtx)
		Expect(err).NotTo(HaveOccurred())

		podSpec := resources.StatefulSet.Spec.Template.Spec

		// Exactly one "log" volume, a size-limited node-disk emptyDir.
		var logVol *corev1.Volume
		count := 0
		for i := range podSpec.Volumes {
			if podSpec.Volumes[i].Name == "log" {
				logVol = &podSpec.Volumes[i]
				count++
			}
		}
		Expect(count).To(Equal(1))
		Expect(logVol.EmptyDir).NotTo(BeNil())
		Expect(logVol.EmptyDir.SizeLimit).NotTo(BeNil())
		Expect(logVol.EmptyDir.SizeLimit.String()).To(Equal(vector.DefaultLogVolumeSize))
		// Node-disk medium, never Memory.
		Expect(string(logVol.EmptyDir.Medium)).To(Equal(""))

		// The logging container has an RW mount at the canonical log dir.
		main := podSpec.Containers[0]
		var foundRW bool
		for _, m := range main.VolumeMounts {
			if m.Name == "log" {
				foundRW = true
				Expect(m.ReadOnly).To(BeFalse())
				Expect(m.MountPath).To(Equal(constant.KubedoopLogDir))
			}
		}
		Expect(foundRW).To(BeTrue())

		// The Vector consumer RO-mounts the same volume on its own init container.
		vectorIdx := -1
		for i := range podSpec.InitContainers {
			if podSpec.InitContainers[i].Name == "vector" {
				vectorIdx = i
			}
		}
		Expect(vectorIdx).To(BeNumerically(">=", 0))
		var vectorRO bool
		for _, m := range podSpec.InitContainers[vectorIdx].VolumeMounts {
			if m.Name == "log" {
				vectorRO = true
				Expect(m.ReadOnly).To(BeTrue())
				Expect(m.MountPath).To(Equal(constant.KubedoopLogDir))
			}
		}
		Expect(vectorRO).To(BeTrue())
	})

	It("end-to-end: honors a custom shared log volume size via the Vector provider", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("test-image:latest", testScheme)
		handler.LoggingContainers = []productlogging.ContainerLogging{{
			Container: "test-cluster-default",
			Framework: productlogging.LoggingFrameworkLogback,
		}}

		// In production the GenericReconciler forwards handler.LogVolumeSize to the provider via
		// WithLogVolumeSize; here we configure the provider directly to exercise the assembly.
		sidecarMgr := sidecar.NewSidecarManager()
		sidecarMgr.Register(
			vector.NewVectorSidecarProvider("test-image:latest",
				vector.WithConfigMapName("test-cluster-default"),
				vector.WithProducers([]string{"test-cluster-default"}),
				vector.WithLogVolumeSize(resource.MustParse("128Mi")),
			),
			&sidecar.SidecarConfig{Enabled: true},
		)

		mockCR := testutil.WrapMockCluster(testutil.NewMockCluster("test-cluster", "default"))
		buildCtx := &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			RoleName:         "test-role",
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    "default",
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(1))},
			ResourceName:     "test-cluster-default",
			SidecarManager:   sidecarMgr,
			MergedConfig: &config.MergedConfig{
				Logging: &v1alpha1.LoggingSpec{
					EnableVectorAgent: ptr.To(true),
					Containers:        map[string]v1alpha1.LoggingConfigSpec{"test-cluster-default": {}},
				},
			},
		}

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		var found bool
		for _, v := range resources.StatefulSet.Spec.Template.Spec.Volumes {
			if v.Name == "log" {
				found = true
				Expect(v.EmptyDir.SizeLimit.String()).To(Equal("128Mi"))
			}
		}
		Expect(found).To(BeTrue())
	})

	It("falls back to defaults when no logging is configured for the container", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("test-image:latest", testScheme)
		handler.LoggingContainers = []productlogging.ContainerLogging{{
			Container: "main",
			Framework: productlogging.LoggingFrameworkLogback,
		}}

		mockCR := testutil.WrapMockCluster(testutil.NewMockCluster("test-cluster", "default"))
		buildCtx := &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			RoleName:         "test-role",
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    "default",
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(1))},
			ResourceName:     "test-cluster-default",
			MergedConfig:     &config.MergedConfig{},
		}

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.ConfigMap.Data["logback.xml"]).To(ContainSubstring(`<root level="INFO">`))
	})

	It("fails fast when a declared logging file collides with an existing ConfigMap key", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("test-image:latest", testScheme)
		handler.LoggingContainers = []productlogging.ContainerLogging{{
			Container: "main",
			Framework: productlogging.LoggingFrameworkLogback, // default file name logback.xml
		}}

		mockCR := testutil.WrapMockCluster(testutil.NewMockCluster("test-cluster", "default"))
		buildCtx := &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			RoleName:         "test-role",
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    "default",
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(1))},
			ResourceName:     "test-cluster-default",
			MergedConfig: &config.MergedConfig{
				ConfigFiles: map[string]map[string]string{
					"logback.xml": {"foo": "bar"},
				},
			},
		}

		_, err := handler.BuildResources(context.Background(), k8sClient, mockCR, buildCtx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("collides"))
	})

	// #502 fixed structurally: Vector enabled with no producers no longer yields an invalid pod.
	// The base handler builds successfully and creates no shared log volume (the Vector provider,
	// the single owner of the volume, is only wired by the GenericReconciler when there is at
	// least one producer — otherwise it warns and skips). Here no SidecarManager is set, mirroring
	// "no vector wired".
	It("builds successfully (no error, no log volume) when Vector is enabled but no producers are declared", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("test-image:latest", testScheme)
		// No LoggingContainers declared.

		mockCR := testutil.WrapMockCluster(testutil.NewMockCluster("test-cluster", "default"))
		buildCtx := &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			RoleName:         "test-role",
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    "default",
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(1))},
			ResourceName:     "test-cluster-default",
			MergedConfig: &config.MergedConfig{
				Logging: &v1alpha1.LoggingSpec{EnableVectorAgent: ptr.To(true)},
			},
		}

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		for _, v := range resources.StatefulSet.Spec.Template.Spec.Volumes {
			Expect(v.Name).NotTo(Equal("log"))
		}
	})

	// The mirror case: Vector enabled WITH a logging container also builds cleanly.
	It("builds cleanly when Vector is enabled and a logging container is declared", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("test-image:latest", testScheme)
		handler.LoggingContainers = []productlogging.ContainerLogging{{
			Container: "main",
			Framework: productlogging.LoggingFrameworkLogback,
		}}

		mockCR := testutil.WrapMockCluster(testutil.NewMockCluster("test-cluster", "default"))
		buildCtx := &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			RoleName:         "test-role",
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    "default",
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(1))},
			ResourceName:     "test-cluster-default",
			MergedConfig: &config.MergedConfig{
				Logging: &v1alpha1.LoggingSpec{
					EnableVectorAgent: ptr.To(true),
					Containers:        map[string]v1alpha1.LoggingConfigSpec{"main": {}},
				},
			},
		}

		_, err := handler.BuildResources(context.Background(), k8sClient, mockCR, buildCtx)
		Expect(err).NotTo(HaveOccurred())
	})
})
