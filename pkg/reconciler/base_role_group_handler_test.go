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
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	corev1 "k8s.io/api/core/v1"
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

	Describe("GetContainerImage", func() {
		It("should return default image when role not in RoleImages", func() {
			image := handler.GetContainerImage("unknown-role")
			Expect(image).To(Equal("test-image:latest"))
		})

		It("should return role-specific image when set", func() {
			handler.SetRoleImage("namenode", "hadoop-namenode:v1")
			image := handler.GetContainerImage("namenode")
			Expect(image).To(Equal("hadoop-namenode:v1"))
		})

		It("should return default image for other roles after setting one", func() {
			handler.SetRoleImage("namenode", "hadoop-namenode:v1")
			image := handler.GetContainerImage("datanode")
			Expect(image).To(Equal("test-image:latest"))
		})
	})

	Describe("GetContainerPorts", func() {
		It("should return nil when role not in RoleContainerPorts", func() {
			ports := handler.GetContainerPorts("unknown-role", "default")
			Expect(ports).To(BeNil())
		})

		It("should return ports when set", func() {
			expectedPorts := []corev1.ContainerPort{
				{Name: "http", ContainerPort: 8080},
			}
			handler.SetRoleContainerPorts("web", expectedPorts)
			ports := handler.GetContainerPorts("web", "default")
			Expect(ports).To(Equal(expectedPorts))
		})
	})

	Describe("GetServicePorts", func() {
		It("should return nil when role not in RoleServicePorts", func() {
			ports := handler.GetServicePorts("unknown-role", "default")
			Expect(ports).To(BeNil())
		})

		It("should return ports when set", func() {
			expectedPorts := []corev1.ServicePort{
				{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)},
			}
			handler.SetRoleServicePorts("web", expectedPorts)
			ports := handler.GetServicePorts("web", "default")
			Expect(ports).To(Equal(expectedPorts))
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

	Describe("GetContainerImage", func() {
		It("should return empty string when function is nil", func() {
			image := funcs.GetContainerImage("test-role")
			Expect(image).To(BeEmpty())
		})

		It("should call the function when set", func() {
			funcs.GetContainerImageFunc = func(roleName string) string {
				return "custom-image:v1"
			}

			image := funcs.GetContainerImage("test-role")
			Expect(image).To(Equal("custom-image:v1"))
		})
	})

	Describe("GetContainerPorts", func() {
		It("should return nil when function is nil", func() {
			ports := funcs.GetContainerPorts("test-role", "default")
			Expect(ports).To(BeNil())
		})

		It("should call the function when set", func() {
			expectedPorts := []corev1.ContainerPort{{Name: "http", ContainerPort: 8080}}
			funcs.GetContainerPortsFunc = func(roleName, roleGroupName string) []corev1.ContainerPort {
				return expectedPorts
			}

			ports := funcs.GetContainerPorts("test-role", "default")
			Expect(ports).To(Equal(expectedPorts))
		})
	})

	Describe("GetServicePorts", func() {
		It("should return nil when function is nil", func() {
			ports := funcs.GetServicePorts("test-role", "default")
			Expect(ports).To(BeNil())
		})

		It("should call the function when set", func() {
			expectedPorts := []corev1.ServicePort{{Name: "http", Port: 80}}
			funcs.GetServicePortsFunc = func(roleName, roleGroupName string) []corev1.ServicePort {
				return expectedPorts
			}

			ports := funcs.GetServicePorts("test-role", "default")
			Expect(ports).To(Equal(expectedPorts))
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
		Expect(configMount.MountPath).To(Equal("/etc/config"))
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
