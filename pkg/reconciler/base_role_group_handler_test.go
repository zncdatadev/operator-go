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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/config"
	"github.com/zncdatadev/operator-go/pkg/constant"
	"github.com/zncdatadev/operator-go/pkg/listener"
	"github.com/zncdatadev/operator-go/pkg/productlogging"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/security"
	"github.com/zncdatadev/operator-go/pkg/sidecar"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	"github.com/zncdatadev/operator-go/pkg/vector"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
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

	Describe("per-role logging and main container name", func() {
		It("SetRoleLoggingContainers wins over the global LoggingContainers for that role", func() {
			global := []productlogging.ContainerLogging{{Container: "app", Framework: productlogging.LoggingFrameworkLogback}}
			perRole := []productlogging.ContainerLogging{{Container: "namenode", Framework: productlogging.LoggingFrameworkLog4j}}
			handler.LoggingContainers = global
			handler.SetRoleLoggingContainers("namenode", perRole)

			Expect(handler.LoggingProducers("namenode")).To(Equal(perRole))
			Expect(handler.LoggingProducers("datanode")).To(Equal(global), "roles without an override fall back to global")
		})

		It("SetRoleMainContainerName records a per-role override", func() {
			handler.MainContainerName = "app"
			handler.SetRoleMainContainerName("namenode", "namenode")
			Expect(handler.RoleMainContainerName["namenode"]).To(Equal("namenode"))
		})

		It("initializes the per-role maps when nil", func() {
			nilHandler := &reconciler.BaseRoleGroupHandler[common.ClusterInterface]{}
			nilHandler.SetRoleMainContainerName("r", "c")
			nilHandler.SetRoleLoggingContainers("r", []productlogging.ContainerLogging{{Container: "c"}})
			Expect(nilHandler.RoleMainContainerName).NotTo(BeNil())
			Expect(nilHandler.RoleLoggingContainers).NotTo(BeNil())
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

		It("should never set a role-group PDB (the PDB is a role-level resource)", func() {
			// Even with roleConfig.podDisruptionBudget configured, BuildResources must not emit
			// a per-group PDB: the framework builds exactly one PDB per role via
			// BuildRolePodDisruptionBudget (covered in the "PodDisruptionBudget building" suite).
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

	BeforeEach(func() {
		handler = reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("test-image:latest", testScheme)
	})

	roleWithPDB := func(spec *v1alpha1.PodDisruptionBudgetSpec) *v1alpha1.RoleSpec {
		return &v1alpha1.RoleSpec{RoleConfig: &v1alpha1.RoleConfigSpec{PodDisruptionBudget: spec}}
	}

	It("should name the PDB at role level (<cluster>-<role>), not per role group", func() {
		roleSpec := roleWithPDB(&v1alpha1.PodDisruptionBudgetSpec{Enabled: true})

		pdb := handler.BuildRolePodDisruptionBudget("test-cluster", "default", "test-role", nil, roleSpec)
		Expect(pdb).NotTo(BeNil())
		Expect(pdb.Name).To(Equal("test-cluster-test-role"))
		Expect(pdb.Namespace).To(Equal("default"))
	})

	It("should set maxUnavailable correctly", func() {
		maxUnavailable := int32(2)
		roleSpec := roleWithPDB(&v1alpha1.PodDisruptionBudgetSpec{Enabled: true, MaxUnavailable: &maxUnavailable})

		pdb := handler.BuildRolePodDisruptionBudget("test-cluster", "default", "test-role", nil, roleSpec)
		Expect(pdb).NotTo(BeNil())
		Expect(pdb.Spec.MaxUnavailable.IntVal).To(Equal(int32(2)))
	})

	It("should build a role-scoped selector without the role group label", func() {
		roleSpec := roleWithPDB(&v1alpha1.PodDisruptionBudgetSpec{Enabled: true})

		pdb := handler.BuildRolePodDisruptionBudget("test-cluster", "default", "test-role",
			map[string]string{"app": "test"}, roleSpec)
		Expect(pdb.Spec.Selector).NotTo(BeNil())
		Expect(pdb.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/instance", "test-cluster"))
		Expect(pdb.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/component", "test-role"))
		// The selector must match all of the role's pods across role groups, so it must not
		// carry the role group marker "<cluster>-<group>" that scopes a single group.
		Expect(pdb.Spec.Selector.MatchLabels).NotTo(HaveKey("test-cluster-default"))
	})

	It("should return nil when disabled", func() {
		roleSpec := roleWithPDB(&v1alpha1.PodDisruptionBudgetSpec{Enabled: false})
		Expect(handler.BuildRolePodDisruptionBudget("test-cluster", "default", "test-role", nil, roleSpec)).To(BeNil())
	})

	It("should return nil when PodDisruptionBudget or RoleConfig is unset", func() {
		Expect(handler.BuildRolePodDisruptionBudget("test-cluster", "default", "test-role", nil,
			roleWithPDB(nil))).To(BeNil())
		Expect(handler.BuildRolePodDisruptionBudget("test-cluster", "default", "test-role", nil,
			&v1alpha1.RoleSpec{})).To(BeNil())
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

	It("forces replicas to 0 when the cluster is stopped, still building the StatefulSet", func() {
		// Stopped scales pods to 0 while all resources are reconciled/preserved: the StatefulSet is
		// still built (with the declared image, config volume, etc.), only its replica count is 0.
		buildCtx.ClusterSpec = &v1alpha1.GenericClusterSpec{
			ClusterOperation: &v1alpha1.ClusterOperationSpec{Stopped: true},
		}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.StatefulSet).NotTo(BeNil())
		Expect(resources.StatefulSet.Spec.Replicas).NotTo(BeNil())
		Expect(*resources.StatefulSet.Spec.Replicas).To(Equal(int32(0)))
	})

	It("keeps the declared replicas when ClusterOperation is set but not stopped", func() {
		// A non-stopped ClusterOperation (e.g. only reconciliationPaused set elsewhere) must not
		// affect the replica count.
		buildCtx.ClusterSpec = &v1alpha1.GenericClusterSpec{
			ClusterOperation: &v1alpha1.ClusterOperationSpec{Stopped: false},
		}

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

var _ = Describe("RoleGroupConfig affinity and gracefulShutdownTimeout consumption", func() {
	var handler *reconciler.BaseRoleGroupHandler[common.ClusterInterface]
	var buildCtx *reconciler.RoleGroupBuildContext

	// configAffinity is the affinity declared in the CRD role group config (as RawExtension).
	configAffinity := &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
				{
					TopologyKey: corev1.LabelHostname,
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app.kubernetes.io/instance": "test-cluster"},
					},
				},
			},
		},
	}

	rawConfigAffinity := func() *k8sruntime.RawExtension {
		raw, err := json.Marshal(configAffinity)
		Expect(err).NotTo(HaveOccurred())
		return &k8sruntime.RawExtension{Raw: raw}
	}

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

	It("applies the config affinity (RawExtension) to the pod spec", func() {
		buildCtx.RoleGroupSpec.Config = &v1alpha1.RoleGroupConfigSpec{
			Affinity: rawConfigAffinity(),
		}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.StatefulSet.Spec.Template.Spec.Affinity).To(Equal(configAffinity))
	})

	It("fails the build loudly on invalid affinity JSON", func() {
		buildCtx.RoleGroupSpec.Config = &v1alpha1.RoleGroupConfigSpec{
			Affinity: &k8sruntime.RawExtension{Raw: []byte(`{"podAntiAffinity": [`)},
		}

		_, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("affinity"))
	})

	It("maps gracefulShutdownTimeout to terminationGracePeriodSeconds", func() {
		buildCtx.RoleGroupSpec.Config = &v1alpha1.RoleGroupConfigSpec{
			GracefulShutdownTimeout: "30s",
		}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		grace := resources.StatefulSet.Spec.Template.Spec.TerminationGracePeriodSeconds
		Expect(grace).NotTo(BeNil())
		Expect(*grace).To(Equal(int64(30)))
	})

	It("fails the build loudly on an unparsable gracefulShutdownTimeout", func() {
		buildCtx.RoleGroupSpec.Config = &v1alpha1.RoleGroupConfigSpec{
			GracefulShutdownTimeout: "not-a-duration",
		}

		_, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).To(HaveOccurred())
		// The error names the field and the offending value.
		Expect(err.Error()).To(ContainSubstring("gracefulShutdownTimeout"))
		Expect(err.Error()).To(ContainSubstring("not-a-duration"))
	})

	It("fails the build loudly on a zero or negative gracefulShutdownTimeout", func() {
		for _, timeout := range []string{"0s", "-30s"} {
			buildCtx.RoleGroupSpec.Config = &v1alpha1.RoleGroupConfigSpec{
				GracefulShutdownTimeout: timeout,
			}

			_, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
			Expect(err).To(HaveOccurred(), "timeout %q must be rejected", timeout)
			Expect(err.Error()).To(ContainSubstring("gracefulShutdownTimeout"))
			Expect(err.Error()).To(ContainSubstring(timeout))
			Expect(err.Error()).To(ContainSubstring("must be a positive duration"))
		}
	})

	It("leaves affinity and terminationGracePeriodSeconds unset when the config fields are empty", func() {
		// Config present but with neither affinity nor gracefulShutdownTimeout set. Backward
		// compatible: products that post-process the built StatefulSet with
		// `if podSpec.Affinity == nil { ... }` default guards remain correct.
		buildCtx.RoleGroupSpec.Config = &v1alpha1.RoleGroupConfigSpec{}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		podSpec := resources.StatefulSet.Spec.Template.Spec
		Expect(podSpec.Affinity).To(BeNil())
		Expect(podSpec.TerminationGracePeriodSeconds).To(BeNil())
	})

	It("leaves the pod spec untouched when the whole role group config is nil", func() {
		Expect(buildCtx.RoleGroupSpec.Config).To(BeNil())

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		podSpec := resources.StatefulSet.Spec.Template.Spec
		Expect(podSpec.Affinity).To(BeNil())
		Expect(podSpec.TerminationGracePeriodSeconds).To(BeNil())
	})

	It("lets a PodOverrides affinity win over the config affinity", func() {
		buildCtx.RoleGroupSpec.Config = &v1alpha1.RoleGroupConfigSpec{
			Affinity: rawConfigAffinity(),
		}
		overrideAffinity := &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{Key: "disktype", Operator: corev1.NodeSelectorOpIn, Values: []string{"ssd"}},
							},
						},
					},
				},
			},
		}
		buildCtx.MergedConfig = &config.MergedConfig{
			PodOverrides: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Affinity: overrideAffinity},
			},
		}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		// The builder applies PodOverrides last, so the user's pod override replaces the
		// config-declared affinity.
		Expect(resources.StatefulSet.Spec.Template.Spec.Affinity).To(Equal(overrideAffinity))
	})

	It("lets a PodOverrides terminationGracePeriodSeconds win over gracefulShutdownTimeout", func() {
		buildCtx.RoleGroupSpec.Config = &v1alpha1.RoleGroupConfigSpec{
			GracefulShutdownTimeout: "30s",
		}
		overrideGrace := int64(120)
		buildCtx.MergedConfig = &config.MergedConfig{
			PodOverrides: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{TerminationGracePeriodSeconds: &overrideGrace},
			},
		}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		// The builder applies PodOverrides last, so the user's pod override replaces the
		// config-declared termination grace.
		Expect(resources.StatefulSet.Spec.Template.Spec.TerminationGracePeriodSeconds).To(Equal(&overrideGrace))
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

	BeforeEach(func() {
		handler = reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("test-image:latest", testScheme)
	})

	buildRolePDB := func(spec *v1alpha1.PodDisruptionBudgetSpec) *policyv1.PodDisruptionBudget {
		roleSpec := &v1alpha1.RoleSpec{RoleConfig: &v1alpha1.RoleConfigSpec{PodDisruptionBudget: spec}}
		return handler.BuildRolePodDisruptionBudget("test-cluster", "default", "test-role", nil, roleSpec)
	}

	It("should create PDB when MaxUnavailable is set and Enabled is true", func() {
		maxUnavailable := int32(1)
		pdb := buildRolePDB(&v1alpha1.PodDisruptionBudgetSpec{Enabled: true, MaxUnavailable: &maxUnavailable})
		Expect(pdb).NotTo(BeNil())
		Expect(pdb.Spec.MaxUnavailable).NotTo(BeNil())
	})

	It("should not create PDB when Enabled is false", func() {
		maxUnavailable := int32(1)
		Expect(buildRolePDB(&v1alpha1.PodDisruptionBudgetSpec{Enabled: false, MaxUnavailable: &maxUnavailable})).To(BeNil())
	})

	It("should not create PDB when PodDisruptionBudget is nil", func() {
		Expect(buildRolePDB(nil)).To(BeNil())
	})

	It("should not create PDB when RoleConfig is nil", func() {
		Expect(handler.BuildRolePodDisruptionBudget("test-cluster", "default", "test-role", nil,
			&v1alpha1.RoleSpec{})).To(BeNil())
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
		Expect(logback).To(ContainSubstring("<file>/kubedoop/log/main/main.log4j.xml</file>"))
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
		Expect(logback).NotTo(ContainSubstring("main.log4j.xml"))
		Expect(logback).NotTo(ContainSubstring("RollingFileAppender"))

		// And no shared log volume is created on the pod (the Vector provider owns it and is not
		// wired when the agent is disabled).
		for _, v := range resources.StatefulSet.Spec.Template.Spec.Volumes {
			Expect(v.Name).NotTo(Equal("log"))
		}
	})

	It("end-to-end: the Vector provider creates the shared log volume, RW-mounts producers, mounts itself", func() {
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

		// The Vector consumer mounts the same volume on its own init container — read-write,
		// because it pre-creates the producers' per-container log dirs before exec'ing vector.
		vectorIdx := -1
		for i := range podSpec.InitContainers {
			if podSpec.InitContainers[i].Name == "vector" {
				vectorIdx = i
			}
		}
		Expect(vectorIdx).To(BeNumerically(">=", 0))
		var vectorMounted bool
		for _, m := range podSpec.InitContainers[vectorIdx].VolumeMounts {
			if m.Name == "log" {
				vectorMounted = true
				Expect(m.ReadOnly).To(BeFalse())
				Expect(m.MountPath).To(Equal(constant.KubedoopLogDir))
			}
		}
		Expect(vectorMounted).To(BeTrue())
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

// fakeVolumeProvider is a minimal VolumeProvider returning one known volume + mount, used to
// lock the injection + ordering contract exercised by the "VolumeProvider injection" specs.
type fakeVolumeProvider struct {
	volume corev1.Volume
	mount  corev1.VolumeMount
}

func (f *fakeVolumeProvider) Volumes() []corev1.Volume { return []corev1.Volume{f.volume} }
func (f *fakeVolumeProvider) VolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{f.mount}
}

var _ reconciler.VolumeProvider = &fakeVolumeProvider{}

// Compile-time assertions that the framework's CSI provisioners satisfy VolumeProvider. Kept in
// a test file so the core reconciler package needs no production dependency on pkg/security or
// pkg/listener for the contract check.
var (
	_ reconciler.VolumeProvider = (*security.SecretProvisioner)(nil)
	_ reconciler.VolumeProvider = (*listener.ListenerProvisioner)(nil)
)

var _ = Describe("VolumeProvider injection", func() {
	var handler *reconciler.BaseRoleGroupHandler[common.ClusterInterface]
	var buildCtx *reconciler.RoleGroupBuildContext
	var provider *fakeVolumeProvider

	// hasVolume reports whether a pod-spec volume with the given name is present.
	hasVolume := func(sts *appsv1.StatefulSet, name string) bool {
		for _, v := range sts.Spec.Template.Spec.Volumes {
			if v.Name == name {
				return true
			}
		}
		return false
	}

	// primaryMountNames returns the mount names on the primary container (container[0]).
	primaryMountNames := func(sts *appsv1.StatefulSet) []string {
		Expect(sts.Spec.Template.Spec.Containers).NotTo(BeEmpty())
		names := make([]string, 0, len(sts.Spec.Template.Spec.Containers[0].VolumeMounts))
		for _, m := range sts.Spec.Template.Spec.Containers[0].VolumeMounts {
			names = append(names, m.Name)
		}
		return names
	}

	BeforeEach(func() {
		handler = reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("test-image:latest", testScheme)
		provider = &fakeVolumeProvider{
			volume: corev1.Volume{
				Name: "tls-cert",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
			mount: corev1.VolumeMount{
				Name:      "tls-cert",
				MountPath: "/kubedoop/tls",
				ReadOnly:  true,
			},
		}
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

	It("injects the provider volume onto the pod spec and its mount onto the primary container", func() {
		buildCtx.VolumeProviders = []reconciler.VolumeProvider{provider}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())

		Expect(hasVolume(resources.StatefulSet, "tls-cert")).To(BeTrue())
		Expect(primaryMountNames(resources.StatefulSet)).To(ContainElement("tls-cert"))
	})

	It("keeps the provider mount on container[0] after MainContainerName rename and sidecar injection", func() {
		// A renamed primary container plus an injected sidecar (init container) is the exact
		// scenario the ordering contract must survive: the provider mount must still be on
		// container[0] under its FINAL renamed name, not lost or moved to the sidecar.
		handler.MainContainerName = "zookeeper"

		sidecarMgr := sidecar.NewSidecarManager()
		sidecarMgr.Register(
			sidecar.NewStaticContainerProvider(corev1.Container{
				Name:  "init-config",
				Image: "busybox:latest",
			}),
			&sidecar.SidecarConfig{Enabled: true},
		)
		buildCtx.SidecarManager = sidecarMgr
		buildCtx.VolumeProviders = []reconciler.VolumeProvider{provider}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())

		sts := resources.StatefulSet
		// The primary container carries the renamed value AND still has the provider mount.
		Expect(sts.Spec.Template.Spec.Containers).NotTo(BeEmpty())
		Expect(sts.Spec.Template.Spec.Containers[0].Name).To(Equal("zookeeper"))
		Expect(primaryMountNames(sts)).To(ContainElement("tls-cert"))
		// The volume is on the pod spec, and the sidecar landed as an init container (not merged
		// into the primary container).
		Expect(hasVolume(sts, "tls-cert")).To(BeTrue())
		Expect(sts.Spec.Template.Spec.InitContainers).To(ContainElement(HaveField("Name", "init-config")))
	})

	It("applies the per-role MainContainerName over the global one when building the StatefulSet", func() {
		// buildCtx.RoleName is "test-role"; the per-role override must win over the global name in
		// the actual built StatefulSet, exercising the mainContainerNameFor wiring end-to-end.
		handler.MainContainerName = "global-main"
		handler.SetRoleMainContainerName(buildCtx.RoleName, "per-role-main")

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())

		sts := resources.StatefulSet
		Expect(sts.Spec.Template.Spec.Containers).NotTo(BeEmpty())
		Expect(sts.Spec.Template.Spec.Containers[0].Name).To(Equal("per-role-main"))
	})

	It("supports multiple providers, injecting every volume + mount", func() {
		second := &fakeVolumeProvider{
			volume: corev1.Volume{
				Name:         "listener-addr",
				VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			},
			mount: corev1.VolumeMount{Name: "listener-addr", MountPath: "/kubedoop/listener"},
		}
		buildCtx.VolumeProviders = []reconciler.VolumeProvider{provider, second}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())

		Expect(hasVolume(resources.StatefulSet, "tls-cert")).To(BeTrue())
		Expect(hasVolume(resources.StatefulSet, "listener-addr")).To(BeTrue())
		mounts := primaryMountNames(resources.StatefulSet)
		Expect(mounts).To(ContainElement("tls-cert"))
		Expect(mounts).To(ContainElement("listener-addr"))
	})

	It("adds no extra volumes/mounts beyond the baseline when no providers are registered", func() {
		// Backward compatibility: with no VolumeProviders the only pod volume/mount is the
		// framework "config" volume (StorageMountPath unset, so no "data" PVC).
		Expect(buildCtx.VolumeProviders).To(BeEmpty())

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())

		volumeNames := make([]string, 0, len(resources.StatefulSet.Spec.Template.Spec.Volumes))
		for _, v := range resources.StatefulSet.Spec.Template.Spec.Volumes {
			volumeNames = append(volumeNames, v.Name)
		}
		Expect(volumeNames).To(ConsistOf("config"))
		Expect(primaryMountNames(resources.StatefulSet)).To(ConsistOf("config"))
	})
})
