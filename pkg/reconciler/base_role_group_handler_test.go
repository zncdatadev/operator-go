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
	"fmt"
	"strings"
	"sync"

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
	k8slabels "k8s.io/apimachinery/pkg/labels"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("BaseRoleGroupHandler", func() {
	var handler *reconciler.BaseRoleGroupHandler[common.ClusterInterface]
	var ctx context.Context
	var mockCR *testutil.MockCluster
	var buildCtx *reconciler.RoleGroupBuildContext

	BeforeEach(func() {
		ctx = context.Background()
		handler = reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("test-image:latest", testScheme)

		mockCluster := testutil.NewMockCluster("test-cluster", "default")
		mockCR = mockCluster

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
						Enabled:        ptr.To(true),
						MaxUnavailable: &maxUnavailable,
					},
				},
			}

			resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)

			Expect(err).NotTo(HaveOccurred())
			Expect(resources.PodDisruptionBudget).To(BeNil())
		})

		It("should propagate the CR's own labels to all resources", func() {
			// The cluster CR's labels are the framework's one label channel: an operator's user
			// labels the CR and every built resource carries it, which is how a platform opt-in
			// like restarter.kubedoop.dev/enable reaches the StatefulSet metadata.
			buildCtx.ClusterLabels["custom-label"] = "custom-value"
			handler.SetRoleServicePorts("test-role", []corev1.ServicePort{
				{Name: "http", Port: 8080},
			})

			resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)

			Expect(err).NotTo(HaveOccurred())
			Expect(resources.ConfigMap.Labels["custom-label"]).To(Equal("custom-value"))
			Expect(resources.HeadlessService.Labels["custom-label"]).To(Equal("custom-value"))
			Expect(resources.Service.Labels["custom-label"]).To(Equal("custom-value"))
			Expect(resources.StatefulSet.Labels["custom-label"]).To(Equal("custom-value"))
			// The StatefulSet's own metadata is what the restarter's watch predicate reads, but
			// the pod template carries it too.
			Expect(resources.StatefulSet.Spec.Template.Labels["custom-label"]).To(Equal("custom-value"))
		})

		It("should build resources with no annotations of its own", func() {
			// The framework sets no annotations on the resources it builds. Nothing sensible can
			// be propagated wholesale from the CR (kubectl's last-applied blob, and the cleaner's
			// own orphan.zncdata.dev/* progress markers, both live there), and a compile-time
			// handler field is the wrong layer for what is a deployment decision. Service
			// annotations specifically are tracked in zncdatadev/operator-go#553.
			handler.SetRoleServicePorts("test-role", []corev1.ServicePort{
				{Name: "http", Port: 8080},
			})

			resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)

			Expect(err).NotTo(HaveOccurred())
			Expect(resources.ConfigMap.Annotations).To(BeEmpty())
			Expect(resources.HeadlessService.Annotations).To(BeEmpty())
			Expect(resources.Service.Annotations).To(BeEmpty())
			Expect(resources.StatefulSet.Annotations).To(BeEmpty())
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

	roleCtx := func(roleSpec *v1alpha1.RoleSpec, clusterLabels map[string]string) *reconciler.RoleBuildContext {
		return &reconciler.RoleBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			ClusterLabels:    clusterLabels,
			RoleName:         "test-role",
			RoleSpec:         roleSpec,
		}
	}

	It("should name the PDB at role level (<cluster>-<role>), not per role group", func() {
		roleSpec := roleWithPDB(&v1alpha1.PodDisruptionBudgetSpec{Enabled: ptr.To(true)})

		pdb := handler.BuildRolePodDisruptionBudget(roleCtx(roleSpec, nil))
		Expect(pdb).NotTo(BeNil())
		Expect(pdb.Name).To(Equal("test-cluster-test-role"))
		Expect(pdb.Namespace).To(Equal("default"))
	})

	It("should set maxUnavailable correctly", func() {
		maxUnavailable := int32(2)
		roleSpec := roleWithPDB(&v1alpha1.PodDisruptionBudgetSpec{Enabled: ptr.To(true), MaxUnavailable: &maxUnavailable})

		pdb := handler.BuildRolePodDisruptionBudget(roleCtx(roleSpec, nil))
		Expect(pdb).NotTo(BeNil())
		Expect(pdb.Spec.MaxUnavailable.IntVal).To(Equal(int32(2)))
	})

	It("should build a role-scoped selector without the role group label", func() {
		roleSpec := roleWithPDB(&v1alpha1.PodDisruptionBudgetSpec{Enabled: ptr.To(true)})

		pdb := handler.BuildRolePodDisruptionBudget(roleCtx(roleSpec, map[string]string{"app": "test"}))
		Expect(pdb.Spec.Selector).NotTo(BeNil())
		Expect(pdb.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/instance", "test-cluster"))
		Expect(pdb.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/component", "test-role"))
		// The selector must match all of the role's pods across role groups, so it must not
		// carry the role group marker "<cluster>-<group>" that scopes a single group.
		Expect(pdb.Spec.Selector.MatchLabels).NotTo(HaveKey("test-cluster-default"))
	})

	It("should return nil when disabled", func() {
		roleSpec := roleWithPDB(&v1alpha1.PodDisruptionBudgetSpec{Enabled: ptr.To(false)})
		Expect(handler.BuildRolePodDisruptionBudget(roleCtx(roleSpec, nil))).To(BeNil())
	})

	It("should return nil when PodDisruptionBudget or RoleConfig is unset", func() {
		Expect(handler.BuildRolePodDisruptionBudget(roleCtx(roleWithPDB(nil), nil))).To(BeNil())
		Expect(handler.BuildRolePodDisruptionBudget(roleCtx(&v1alpha1.RoleSpec{}, nil))).To(BeNil())
	})

	It("should return nil rather than panic when the context or role spec is missing", func() {
		Expect(handler.BuildRolePodDisruptionBudget(nil)).To(BeNil())
		Expect(handler.BuildRolePodDisruptionBudget(roleCtx(nil, nil))).To(BeNil())
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

	It("fails loudly when a podOverride container matches nothing (typo or sidecar name)", func() {
		// Sidecars are injected AFTER the overrides merge, so an override naming one (or a
		// typo) yields an image-less container; without this guard the API server would
		// reject the StatefulSet and the CR would sit silently Degraded.
		handler.MainContainerName = "node"
		buildCtx.MergedConfig = &config.MergedConfig{
			PodOverrides: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "vector",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
							},
						},
					},
				},
			},
		}

		_, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).To(MatchError(ContainSubstring(`container "vector" has no image`)))
		Expect(err).To(MatchError(ContainSubstring(`main container: "node"`)))
	})

	It("fails the role group when a podOverride displaces the framework's config mount", func() {
		// Strategic merge keys volumeMounts by mountPath, so a mount declared at the config path
		// REPLACES the framework's rather than being added next to it. With the override also
		// declaring its volume, the pod spec stays valid and the API server accepts it — the pods
		// would come up with the generated ConfigMap mounted nowhere and read an empty config
		// directory. Refusing to build is the only thing that stops that being silent.
		handler.MainContainerName = "node"
		buildCtx.MergedConfig = &config.MergedConfig{
			PodOverrides: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{{
						Name:         "my-overlay",
						VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
					}},
					Containers: []corev1.Container{{
						Name: "node",
						VolumeMounts: []corev1.VolumeMount{
							{Name: "my-overlay", MountPath: constant.KubedoopConfigDirMount},
						},
					}},
				},
			},
		}

		_, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).To(HaveOccurred())
		Expect(reconciler.IsValidationError(err)).To(BeTrue(),
			"a broken podOverride is a validation failure, not an opaque build error")
		Expect(err).To(MatchError(ContainSubstring("displaced")))
		Expect(err).To(MatchError(ContainSubstring(constant.KubedoopConfigDirMount)))
		Expect(err).To(MatchError(ContainSubstring("podOverrides")))
	})

	It("accepts a podOverride that mounts at a path the framework does not own", func() {
		handler.MainContainerName = "node"
		buildCtx.MergedConfig = &config.MergedConfig{
			PodOverrides: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{{
						Name:         "my-overlay",
						VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
					}},
					Containers: []corev1.Container{{
						Name: "node",
						VolumeMounts: []corev1.VolumeMount{
							{Name: "my-overlay", MountPath: "/kubedoop/overlay"},
						},
					}},
				},
			},
		}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		mounts := resources.StatefulSet.Spec.Template.Spec.Containers[0].VolumeMounts
		Expect(mounts).To(ContainElement(HaveField("MountPath", constant.KubedoopConfigDirMount)))
		Expect(mounts).To(ContainElement(HaveField("MountPath", "/kubedoop/overlay")))
	})

	It("does not alias the live CR spec through the merged logging config", func() {
		roleCfg := &v1alpha1.RoleGroupConfigSpec{
			Logging: &v1alpha1.LoggingSpec{
				EnableVectorAgent: ptr.To(true),
			},
		}
		merged := reconciler.MergeRoleGroupConfig(roleCfg, &v1alpha1.RoleGroupConfigSpec{})
		Expect(merged.Logging).NotTo(BeIdenticalTo(roleCfg.Logging),
			"the merged result must be a fresh copy, never the CR's own object")

		*merged.Logging.EnableVectorAgent = false
		Expect(*roleCfg.Logging.EnableVectorAgent).To(BeTrue(),
			"mutating the merge result must not touch the input")
	})

	It("consumes role-level config when the role group declares none (role->group fallback)", func() {
		// Regression: only logging and overrides were merged role->group; role-level
		// resources/affinity/gracefulShutdownTimeout were silently dropped.
		merged := reconciler.MergeRoleGroupConfig(
			&v1alpha1.RoleGroupConfigSpec{
				GracefulShutdownTimeout: ptr.To("60s"),
				Resources: &v1alpha1.ResourcesSpec{
					CPU: &v1alpha1.CPUResource{Max: ptr.To(resource.MustParse("2"))},
				},
			},
			nil,
		)
		Expect(merged).NotTo(BeNil())
		Expect(merged.GetGracefulShutdownTimeout()).To(Equal("60s"))
		Expect(merged.Resources.CPU.Max.String()).To(Equal("2"))
	})

	It("lets role group config win per field over role config", func() {
		merged := reconciler.MergeRoleGroupConfig(
			&v1alpha1.RoleGroupConfigSpec{
				GracefulShutdownTimeout: ptr.To("60s"),
				Resources: &v1alpha1.ResourcesSpec{
					CPU:    &v1alpha1.CPUResource{Max: ptr.To(resource.MustParse("2"))},
					Memory: &v1alpha1.MemoryResource{Limit: ptr.To(resource.MustParse("2Gi"))},
				},
			},
			&v1alpha1.RoleGroupConfigSpec{
				Resources: &v1alpha1.ResourcesSpec{
					CPU: &v1alpha1.CPUResource{Max: ptr.To(resource.MustParse("4"))},
				},
			},
		)
		Expect(merged.Resources.CPU.Max.String()).To(Equal("4"), "group wins")
		Expect(merged.Resources.Memory.Limit.String()).To(Equal("2Gi"), "role fills the gap")
		Expect(merged.GetGracefulShutdownTimeout()).To(Equal("60s"))
	})

	It("merges a podOverride addressing the renamed main container instead of appending a phantom", func() {
		// Regression: with MainContainerName set, a podOverride container carrying the
		// user-facing name (the documented contract, e.g. spark's "node") must merge into the
		// primary container. Before the fix the rename ran after Build(), so the override was
		// appended as a second, image-less container and the API server rejected the
		// StatefulSet.
		handler.MainContainerName = "node"
		buildCtx.MergedConfig = &config.MergedConfig{
			PodOverrides: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "node",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("3Gi"),
								},
							},
						},
					},
				},
			},
		}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())

		containers := resources.StatefulSet.Spec.Template.Spec.Containers
		Expect(containers).To(HaveLen(1))
		Expect(containers[0].Name).To(Equal("node"))
		Expect(containers[0].Image).To(Equal("test-image:latest"), "the merged container keeps the built image")
		Expect(containers[0].Resources.Limits.Cpu().String()).To(Equal("2"))
		Expect(containers[0].Resources.Limits.Memory().String()).To(Equal("3Gi"))
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
		// fsGroup without a change policy means Kubernetes' default of Always: the kubelet walks
		// the whole data volume chown'ing every file before the container starts, on every start.
		Expect(podSpec.SecurityContext.FSGroupChangePolicy).NotTo(BeNil())
		Expect(*podSpec.SecurityContext.FSGroupChangePolicy).To(Equal(corev1.FSGroupChangeOnRootMismatch))

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

	It("should deep-merge PodOverrides into the default pod security context (strategic merge)", func() {
		// The override sets only RunAsUser. Strategic-merge semantics (the documented merge
		// strategy for PodTemplate) mean the rest of the default hardening (RunAsGroup, FSGroup,
		// RunAsNonRoot, SeccompProfile) is kept; an override must explicitly restate a field
		// (e.g. runAsNonRoot: false) to change it.
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
		// The default hardening fields the override did not mention survive the merge.
		Expect(podSC.FSGroup).NotTo(BeNil())
		Expect(podSC.RunAsGroup).NotTo(BeNil())
		Expect(podSC.RunAsNonRoot).NotTo(BeNil())
		Expect(podSC.SeccompProfile).NotTo(BeNil())
	})

	It("should deep-merge PodOverrides into the default container security context (strategic merge)", func() {
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
		// The default hardening fields the override did not mention survive the merge.
		Expect(csc.AllowPrivilegeEscalation).NotTo(BeNil())
		Expect(csc.Capabilities).NotTo(BeNil())
		Expect(csc.RunAsNonRoot).NotTo(BeNil())
		Expect(csc.SeccompProfile).NotTo(BeNil())
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
			GracefulShutdownTimeout: ptr.To("30s"),
		}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		grace := resources.StatefulSet.Spec.Template.Spec.TerminationGracePeriodSeconds
		Expect(grace).NotTo(BeNil())
		Expect(*grace).To(Equal(int64(30)))
	})

	It("fails the build loudly on an unparsable gracefulShutdownTimeout", func() {
		buildCtx.RoleGroupSpec.Config = &v1alpha1.RoleGroupConfigSpec{
			GracefulShutdownTimeout: ptr.To("not-a-duration"),
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
				GracefulShutdownTimeout: ptr.To(timeout),
			}

			_, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
			Expect(err).To(HaveOccurred(), "timeout %q must be rejected", timeout)
			Expect(err.Error()).To(ContainSubstring("gracefulShutdownTimeout"))
			Expect(err.Error()).To(ContainSubstring(timeout))
			Expect(err.Error()).To(ContainSubstring("must be a positive duration"))
		}
	})

	It("leaves affinity unset but writes the default grace period when the config fields are empty", func() {
		// Config present but with neither affinity nor gracefulShutdownTimeout set. Affinity stays
		// nil for backward compatibility: products that post-process the built StatefulSet with
		// `if podSpec.Affinity == nil { ... }` default guards remain correct.
		buildCtx.RoleGroupSpec.Config = &v1alpha1.RoleGroupConfigSpec{}

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		podSpec := resources.StatefulSet.Spec.Template.Spec
		Expect(podSpec.Affinity).To(BeNil())
		// The grace period IS written, explicitly. It used to come from the CRD default the API
		// server stamped into every config block; removing that default (so a role-level value can
		// reach its groups) would otherwise have silently changed the effective grace period of
		// every existing cluster to whatever Kubernetes happens to default to.
		Expect(podSpec.TerminationGracePeriodSeconds).NotTo(BeNil())
		Expect(*podSpec.TerminationGracePeriodSeconds).To(Equal(int64(30)))
	})

	It("writes the default grace period when the whole role group config is nil", func() {
		Expect(buildCtx.RoleGroupSpec.Config).To(BeNil())

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		podSpec := resources.StatefulSet.Spec.Template.Spec
		Expect(podSpec.Affinity).To(BeNil())
		Expect(podSpec.TerminationGracePeriodSeconds).NotTo(BeNil())
		Expect(*podSpec.TerminationGracePeriodSeconds).To(Equal(int64(30)))
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
		// The builder applies PodOverrides last via strategic merge: affinity kinds the override
		// sets win, while config-declared affinity kinds it does not mention are kept.
		merged := resources.StatefulSet.Spec.Template.Spec.Affinity
		Expect(merged).NotTo(BeNil())
		Expect(merged.NodeAffinity).To(Equal(overrideAffinity.NodeAffinity))
		Expect(merged.PodAntiAffinity).NotTo(BeNil(),
			"config-declared podAntiAffinity survives a nodeAffinity-only override")
	})

	It("lets a PodOverrides terminationGracePeriodSeconds win over gracefulShutdownTimeout", func() {
		buildCtx.RoleGroupSpec.Config = &v1alpha1.RoleGroupConfigSpec{
			GracefulShutdownTimeout: ptr.To("30s"),
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

var _ = Describe("BaseRoleGroupHandler CR label propagation", func() {
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

	It("should carry every CR label onto all resources", func() {
		buildCtx.ClusterLabels["custom-label"] = "custom-value"
		buildCtx.ClusterLabels["team"] = "platform"

		resources, err := handler.BuildResources(context.Background(), nil, nil, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.ConfigMap.Labels).To(HaveKey("custom-label"))
		Expect(resources.ConfigMap.Labels).To(HaveKey("team"))
		Expect(resources.StatefulSet.Labels).To(HaveKey("custom-label"))
		Expect(resources.HeadlessService.Labels).To(HaveKey("custom-label"))
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
		return handler.BuildRolePodDisruptionBudget(&reconciler.RoleBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			RoleName:         "test-role",
			RoleSpec:         roleSpec,
		})
	}

	It("should create PDB when MaxUnavailable is set and Enabled is true", func() {
		maxUnavailable := int32(1)
		pdb := buildRolePDB(&v1alpha1.PodDisruptionBudgetSpec{Enabled: ptr.To(true), MaxUnavailable: &maxUnavailable})
		Expect(pdb).NotTo(BeNil())
		Expect(pdb.Spec.MaxUnavailable).NotTo(BeNil())
	})

	It("should not create PDB when Enabled is false", func() {
		maxUnavailable := int32(1)
		Expect(buildRolePDB(&v1alpha1.PodDisruptionBudgetSpec{Enabled: ptr.To(false), MaxUnavailable: &maxUnavailable})).To(BeNil())
	})

	It("should not create PDB when PodDisruptionBudget is nil", func() {
		Expect(buildRolePDB(nil)).To(BeNil())
	})

	It("should not create PDB when RoleConfig is nil", func() {
		Expect(handler.BuildRolePodDisruptionBudget(&reconciler.RoleBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			RoleName:         "test-role",
			RoleSpec:         &v1alpha1.RoleSpec{},
		})).To(BeNil())
	})
})

var _ = Describe("BaseRoleGroupHandler enhancements", func() {
	var ctx context.Context
	var mockCR *testutil.MockCluster

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
		mockCR = testutil.NewMockCluster("test-cluster", "default")
		buildCtx = newBuildCtx(&v1alpha1.StorageResource{Capacity: ptr.To(resource.MustParse("10Gi"))})
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

		mockCR := testutil.NewMockCluster("test-cluster", "default")
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

		mockCR := testutil.NewMockCluster("test-cluster", "default")
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

		mockCR := testutil.NewMockCluster("test-cluster", "default")
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
				vector.WithProducers([]productlogging.ContainerLogging{{Container: "test-cluster-default"}}),
			),
			&sidecar.SidecarConfig{Enabled: true},
		)

		mockCR := testutil.NewMockCluster("test-cluster", "default")
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
				vector.WithProducers([]productlogging.ContainerLogging{{Container: "test-cluster-default"}}),
				vector.WithLogVolumeSize(resource.MustParse("128Mi")),
			),
			&sidecar.SidecarConfig{Enabled: true},
		)

		mockCR := testutil.NewMockCluster("test-cluster", "default")
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

		mockCR := testutil.NewMockCluster("test-cluster", "default")
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

		mockCR := testutil.NewMockCluster("test-cluster", "default")
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

		mockCR := testutil.NewMockCluster("test-cluster", "default")
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

		mockCR := testutil.NewMockCluster("test-cluster", "default")
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

// embeddingImageHandler mimics the documented product pattern: a handler embedding
// BaseRoleGroupHandler that resolves the CR-driven image inside its BuildResources override,
// immediately before delegating to the base implementation.
type embeddingImageHandler struct {
	*reconciler.BaseRoleGroupHandler[common.ClusterInterface]
	image string
}

func (h *embeddingImageHandler) BuildResources(
	ctx context.Context,
	k8sClient client.Client,
	cr common.ClusterInterface,
	buildCtx *reconciler.RoleGroupBuildContext,
) (*reconciler.RoleGroupResources, error) {
	h.Image = h.image
	return h.BaseRoleGroupHandler.BuildResources(ctx, k8sClient, cr, buildCtx)
}

var _ = Describe("Sidecar product image propagation", func() {
	// Regression: the propagation used to live in GenericReconciler behind a concrete
	// *BaseRoleGroupHandler type assertion, so embedding handlers (every product operator)
	// never had the product image set on framework-registered sidecars and had to call
	// SetProductImage by hand.
	It("propagates the image resolved in an embedding handler's BuildResources to the Vector sidecar", func() {
		base := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("", testScheme)
		base.LoggingContainers = []productlogging.ContainerLogging{{
			Container: "main",
			Framework: productlogging.LoggingFrameworkLogback,
		}}
		handler := &embeddingImageHandler{BaseRoleGroupHandler: base, image: "product-image:1.2.3"}

		// The producer must name the pod's primary container, which MainContainerName pins here.
		base.MainContainerName = "main"

		mgr := sidecar.NewSidecarManager()
		mgr.Register(
			vector.NewVectorSidecarProvider("",
				vector.WithConfigMapName("test-cluster-default"),
				vector.WithProducers([]productlogging.ContainerLogging{{Container: "main"}})),
			&sidecar.SidecarConfig{Enabled: true},
		)

		mockCR := testutil.NewMockCluster("test-cluster", "default")
		buildCtx := &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			RoleName:         "test-role",
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    "default",
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(1))},
			ResourceName:     "test-cluster-default",
			SidecarManager:   mgr,
			MergedConfig: &config.MergedConfig{
				Logging: &v1alpha1.LoggingSpec{
					EnableVectorAgent: ptr.To(true),
					Containers:        map[string]v1alpha1.LoggingConfigSpec{"main": {}},
				},
			},
		}

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR, buildCtx)
		Expect(err).NotTo(HaveOccurred())

		podSpec := resources.StatefulSet.Spec.Template.Spec
		var vectorImage string
		for _, c := range append(podSpec.InitContainers, podSpec.Containers...) {
			if c.Name == vector.VectorSidecarName {
				vectorImage = c.Image
			}
		}
		Expect(vectorImage).To(Equal("product-image:1.2.3"))
	})
})

var _ = Describe("Selector label stability", func() {
	var handler *reconciler.BaseRoleGroupHandler[common.ClusterInterface]
	var mockCR *testutil.MockCluster

	newBuildCtx := func(clusterLabels map[string]string) *reconciler.RoleGroupBuildContext {
		return &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			ClusterLabels:    clusterLabels,
			RoleName:         "server",
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    "default",
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(1))},
			MergedConfig:     &config.MergedConfig{},
			ResourceName:     "test-cluster-server-default",
		}
	}

	BeforeEach(func() {
		handler = reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("img:1", testScheme)
		mockCR = testutil.NewMockCluster("test-cluster", "default")
	})

	It("builds the selector from framework-owned labels only", func() {
		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR,
			newBuildCtx(map[string]string{"env": "prod", "team": "platform"}))
		Expect(err).NotTo(HaveOccurred())

		Expect(resources.StatefulSet.Spec.Selector.MatchLabels).To(Equal(map[string]string{
			"app.kubernetes.io/instance":   "test-cluster",
			"app.kubernetes.io/component":  "server",
			"app.kubernetes.io/managed-by": "operator-go",
			"test-cluster-default":         "true",
		}))

		// The CR's labels stay on the pod template: StatefulSets created before the selector was
		// narrowed froze them into their immutable selector, so dropping them from the template
		// would leave those objects unmatchable.
		Expect(resources.StatefulSet.Spec.Template.Labels).To(HaveKeyWithValue("env", "prod"))
		Expect(resources.StatefulSet.Spec.Template.Labels).To(HaveKeyWithValue("team", "platform"))
	})

	It("keeps a pod template built after a CR label change matching the frozen selector", func() {
		before, err := handler.BuildResources(context.Background(), k8sClient, mockCR,
			newBuildCtx(map[string]string{"env": "prod"}))
		Expect(err).NotTo(HaveOccurred())
		frozenSelector := before.StatefulSet.Spec.Selector.MatchLabels

		// The user edits the cluster CR's labels; the next reconcile builds with the new set.
		after, err := handler.BuildResources(context.Background(), k8sClient, mockCR,
			newBuildCtx(map[string]string{"env": "staging"}))
		Expect(err).NotTo(HaveOccurred())

		Expect(after.StatefulSet.Spec.Selector.MatchLabels).To(Equal(frozenSelector))
		Expect(k8slabels.SelectorFromSet(frozenSelector).
			Matches(k8slabels.Set(after.StatefulSet.Spec.Template.Labels))).To(BeTrue())
		Expect(after.HeadlessService.Spec.Selector).To(Equal(frozenSelector))
	})

	It("keeps satisfying the selector a StatefulSet froze under the previous label scheme", func() {
		clusterLabels := map[string]string{"env": "prod", "team": "platform"}
		buildCtx := newBuildCtx(clusterLabels)

		// The selector the previous framework version wrote into .spec.selector: the FULL
		// descriptive label set (cluster labels, then the framework identity labels, then the
		// product's own). It is immutable on the live object, so every pod template this version
		// builds must still match it or the API server rejects every update.
		legacySelector := map[string]string{
			"env":                          "prod",
			"app.kubernetes.io/instance":   "test-cluster",
			"app.kubernetes.io/component":  "server",
			"app.kubernetes.io/managed-by": "operator-go",
			"test-cluster-default":         "true",
			"team":                         "platform",
		}

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR, buildCtx)
		Expect(err).NotTo(HaveOccurred())

		Expect(k8slabels.SelectorFromSet(legacySelector).
			Matches(k8slabels.Set(resources.StatefulSet.Spec.Template.Labels))).To(BeTrue())
	})

	It("lets a framework label win over a CR label that collides with it", func() {
		// This is what replaces the ExtraLabels collision check. A CR label is applied FIRST and
		// the framework's identity labels overwrite it, so a colliding CR label is inert: the
		// selector and the pod template still agree on the framework's value, and no live object
		// can end up demanding a value its template no longer carries.
		//
		// ExtraLabels was applied LAST and therefore won in the label map, while the builder
		// re-wrote the selector keys into the pod template afterwards — which is exactly the
		// mismatch that needed a build-time rejection. Ordering makes it impossible instead.
		handler.SetRoleServicePorts("server", []corev1.ServicePort{{Name: "http", Port: 8080}})

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR,
			newBuildCtx(map[string]string{
				"app.kubernetes.io/component":  "hijacked",
				"app.kubernetes.io/managed-by": "someone-else",
				"env":                          "prod",
			}))
		Expect(err).NotTo(HaveOccurred())

		// The ConfigMap and Services are the load-bearing assertions: they carry buildLabels'
		// output verbatim. The StatefulSet builder re-asserts the selector keys into the
		// StatefulSet's own labels and pod template on its way out, so checking only those would
		// pass even with the ordering reversed — and would leave the ConfigMap and Services
		// disagreeing with the workload about which role they belong to.
		for _, labels := range []map[string]string{
			resources.ConfigMap.Labels,
			resources.HeadlessService.Labels,
			resources.Service.Labels,
			resources.StatefulSet.Labels,
			resources.StatefulSet.Spec.Template.Labels,
			resources.StatefulSet.Spec.Selector.MatchLabels,
		} {
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/component", "server"))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "operator-go"))
		}
		// A non-colliding CR label is untouched.
		Expect(resources.StatefulSet.Labels).To(HaveKeyWithValue("env", "prod"))
		Expect(k8slabels.SelectorFromSet(resources.StatefulSet.Spec.Selector.MatchLabels).
			Matches(k8slabels.Set(resources.StatefulSet.Spec.Template.Labels))).To(BeTrue())
	})
})

var _ = Describe("Role group Services", func() {
	It("preserves every field of a declared service port on both Services", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("img:1", testScheme)
		handler.SetRoleServicePorts("server", []corev1.ServicePort{{
			Name:       "http",
			Port:       8080,
			TargetPort: intstr.FromString("http"),
			NodePort:   31234,
			Protocol:   corev1.ProtocolTCP,
		}})

		buildCtx := &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			RoleName:         "server",
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    "default",
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(1))},
			MergedConfig:     &config.MergedConfig{},
			ResourceName:     "test-cluster-server-default",
		}

		resources, err := handler.BuildResources(context.Background(), k8sClient,
			testutil.NewMockCluster("test-cluster", "default"), buildCtx)
		Expect(err).NotTo(HaveOccurred())

		for _, svc := range []*corev1.Service{resources.Service, resources.HeadlessService} {
			Expect(svc.Spec.Ports).To(HaveLen(1))
			Expect(svc.Spec.Ports[0].NodePort).To(Equal(int32(31234)))
			Expect(svc.Spec.Ports[0].TargetPort).To(Equal(intstr.FromString("http")))
		}
	})
})

var _ = Describe("Role group ConfigMap rendering", func() {
	var handler *reconciler.BaseRoleGroupHandler[common.ClusterInterface]
	var mockCR *testutil.MockCluster

	// Enough keys that a randomized map iteration order practically never matches the sorted one.
	configFile := map[string]string{
		"alpha": "1", "bravo": "2", "charlie": "3", "delta": "4", "echo": "5",
		"foxtrot": "6", "golf": "7", "hotel": "8", "india": "9", "juliett": "10",
	}

	newBuildCtx := func() *reconciler.RoleGroupBuildContext {
		return &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			RoleName:         "server",
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    "default",
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(1))},
			MergedConfig: &config.MergedConfig{
				ConfigFiles: map[string]map[string]string{"custom.conf": configFile},
			},
			ResourceName: "test-cluster-server-default",
		}
	}

	BeforeEach(func() {
		handler = reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("img:1", testScheme)
		mockCR = testutil.NewMockCluster("test-cluster", "default")
	})

	It("renders a config file with no registered generator in sorted key order", func() {
		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR, newBuildCtx())
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.ConfigMap.Data["custom.conf"]).To(Equal(
			"alpha=1\nbravo=2\ncharlie=3\ndelta=4\necho=5\nfoxtrot=6\ngolf=7\nhotel=8\nindia=9\njuliett=10\n"))
	})

	It("produces byte-identical data on every build", func() {
		// Non-deterministic rendering makes every reconcile rewrite ConfigMap.Data, which the
		// reconciler's own ConfigMap watch turns straight back into another reconcile.
		first, err := handler.BuildResources(context.Background(), k8sClient, mockCR, newBuildCtx())
		Expect(err).NotTo(HaveOccurred())
		for range 20 {
			again, err := handler.BuildResources(context.Background(), k8sClient, mockCR, newBuildCtx())
			Expect(err).NotTo(HaveOccurred())
			Expect(again.ConfigMap.Data).To(Equal(first.ConfigMap.Data))
		}
	})
})

var _ = Describe("Container image resolution", func() {
	var mockCR *testutil.MockCluster

	newBuildCtx := func(image *v1alpha1.ImageSpec) *reconciler.RoleGroupBuildContext {
		return &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			ClusterSpec:      &v1alpha1.GenericClusterSpec{Image: image},
			RoleName:         "server",
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    "default",
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(1))},
			MergedConfig:     &config.MergedConfig{},
			ResourceName:     "test-cluster-server-default",
		}
	}

	crImage := &v1alpha1.ImageSpec{
		Repo:            "quay.io/kubedoop",
		ProductVersion:  "4.7.0",
		KubedoopVersion: "0.1.0",
		PullPolicy:      corev1.PullAlways,
	}

	BeforeEach(func() {
		mockCR = testutil.NewMockCluster("test-cluster", "default")
	})

	It("resolves the main container image from the CR spec.image when ProductName is set", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("fallback:1", testScheme)
		handler.ProductName = "trino"
		handler.SetRoleImage("server", "role-image:1")

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR, newBuildCtx(crImage))
		Expect(err).NotTo(HaveOccurred())

		container := resources.StatefulSet.Spec.Template.Spec.Containers[0]
		Expect(container.Image).To(Equal("quay.io/kubedoop/trino:4.7.0-kubedoop0.1.0"))
		Expect(container.ImagePullPolicy).To(Equal(corev1.PullAlways))
	})

	It("keeps the static image when the product declares no ProductName", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("fallback:1", testScheme)

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR, newBuildCtx(crImage))
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.StatefulSet.Spec.Template.Spec.Containers[0].Image).To(Equal("fallback:1"))
	})

	It("fails rather than running a different version than the user asked for", func() {
		// This used to fall back to the per-role image and start it, so a user writing
		// `productVersion: 4.7.0` with no repo anywhere silently got whatever the handler was
		// built with — no error, no event, no status change. Deploying an unrequested version of a
		// stateful product is not a safe default; the same call as config.affinity.
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("fallback:1", testScheme)
		handler.ProductName = "trino"
		handler.SetRoleImage("server", "role-image:1")

		_, err := handler.BuildResources(context.Background(), k8sClient, mockCR,
			newBuildCtx(&v1alpha1.ImageSpec{ProductVersion: "4.7.0"}))
		Expect(err).To(MatchError(ContainSubstring("repo is unset")))
	})

	It("falls back to the per-role image when nobody stated an image at all", func() {
		// No opinion anywhere is not a misconfiguration — it is a handler running its own image.
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("fallback:1", testScheme)
		handler.ProductName = "trino"
		handler.SetRoleImage("server", "role-image:1")

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR, newBuildCtx(nil))
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.StatefulSet.Spec.Template.Spec.Containers[0].Image).To(Equal("role-image:1"))
	})

	It("completes a partial spec.image from ImageDefaults", func() {
		// The case that made three operators hand-roll image resolution: kubedoop publishes only
		// the "-kubedoop<version>" tag, and a user writing just productVersion could not produce
		// one, because the suffix was appended only when the USER supplied kubedoopVersion.
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("fallback:1", testScheme)
		handler.ProductName = "trino"
		handler.ImageDefaults = v1alpha1.ImageSpec{
			Repo:            "quay.io/zncdatadev",
			ProductVersion:  "476",
			KubedoopVersion: "0.0.0-dev",
		}

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR,
			newBuildCtx(&v1alpha1.ImageSpec{ProductVersion: "4.7.0"}))
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.StatefulSet.Spec.Template.Spec.Containers[0].Image).
			To(Equal("quay.io/zncdatadev/trino:4.7.0-kubedoop0.0.0-dev"))
	})

	It("runs the defaults when the CR states no image, so an operator upgrade moves the cluster", func() {
		// ImageDefaults is read every reconcile. A webhook could not do this: its values are
		// persisted at admission and never recomputed, freezing kubedoopVersion at whatever
		// operator version first admitted the CR.
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("fallback:1", testScheme)
		handler.ProductName = "trino"
		handler.ImageDefaults = v1alpha1.ImageSpec{
			Repo: "quay.io/zncdatadev", ProductVersion: "476", KubedoopVersion: "0.2.0",
		}

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR, newBuildCtx(nil))
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.StatefulSet.Spec.Template.Spec.Containers[0].Image).
			To(Equal("quay.io/zncdatadev/trino:476-kubedoop0.2.0"))
	})

	It("keeps a ProductName-less handler on its static image, and never errors", func() {
		// The shape hive and zookeeper use today: they resolve images themselves and leave
		// ProductName empty. Their CRs DO carry a webhook-filled spec.image, so treating an
		// unresolvable spec as an error here would have broken every one of their clusters.
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("fallback:1", testScheme)

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR,
			newBuildCtx(&v1alpha1.ImageSpec{Repo: "quay.io/kubedoop", ProductVersion: "4.7.0"}))
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.StatefulSet.Spec.Template.Spec.Containers[0].Image).To(Equal("fallback:1"))
	})

	It("honours spec.image.custom even without a ProductName", func() {
		// A fully qualified reference needs no product name to build, and ignoring it meant a user
		// pinning an image on such an operator was silently overruled.
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("fallback:1", testScheme)

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR,
			newBuildCtx(&v1alpha1.ImageSpec{Custom: "my-registry/trino:pinned"}))
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.StatefulSet.Spec.Template.Spec.Containers[0].Image).To(Equal("my-registry/trino:pinned"))
	})

	It("gives the sidecars the same CR-resolved image as the main container", func() {
		// The Vector agent ships inside the product image, so a product that patches the main
		// container after BuildResources leaves the sidecar behind on the stale image.
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("fallback:1", testScheme)
		handler.ProductName = "trino"
		// The producer must name the pod's primary container, which MainContainerName pins here.
		handler.MainContainerName = "main"
		handler.LoggingContainers = []productlogging.ContainerLogging{{
			Container: "main",
			Framework: productlogging.LoggingFrameworkLogback,
		}}

		mgr := sidecar.NewSidecarManager()
		mgr.Register(
			vector.NewVectorSidecarProvider("",
				vector.WithConfigMapName("test-cluster-server-default"),
				vector.WithProducers([]productlogging.ContainerLogging{{Container: "main"}})),
			&sidecar.SidecarConfig{Enabled: true},
		)

		buildCtx := newBuildCtx(crImage)
		buildCtx.SidecarManager = mgr
		buildCtx.MergedConfig = &config.MergedConfig{
			Logging: &v1alpha1.LoggingSpec{
				EnableVectorAgent: ptr.To(true),
				Containers:        map[string]v1alpha1.LoggingConfigSpec{"main": {}},
			},
		}

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR, buildCtx)
		Expect(err).NotTo(HaveOccurred())

		podSpec := resources.StatefulSet.Spec.Template.Spec
		var vectorImage string
		for _, c := range append(podSpec.InitContainers, podSpec.Containers...) {
			if c.Name == vector.VectorSidecarName {
				vectorImage = c.Image
			}
		}
		Expect(vectorImage).To(Equal("quay.io/kubedoop/trino:4.7.0-kubedoop0.1.0"))
	})
})

var _ = Describe("ConfiguredRoleNames", func() {
	It("returns the sorted union of every per-role configuration key", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("img:1", testScheme)
		handler.SetRoleImage("workers", "img:2")
		handler.SetRoleContainerPorts("coordinators", []corev1.ContainerPort{{Name: "http", ContainerPort: 8080}})
		handler.SetRoleServicePorts("coordinators", []corev1.ServicePort{{Name: "http", Port: 8080}})
		handler.SetRoleMainContainerName("workers", "trino")
		handler.SetRoleLoggingContainers("gateways", nil)

		Expect(handler.ConfiguredRoleNames()).To(Equal([]string{"coordinators", "gateways", "workers"}))
	})

	It("returns an empty list for a handler with no per-role configuration", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("img:1", testScheme)
		Expect(handler.ConfiguredRoleNames()).To(BeEmpty())
	})
})

var _ = Describe("BaseRoleGroupHandler ConfigMap data", func() {
	It("always returns a non-nil Data map so products can add files to it", func() {
		// The documented extension pattern is to call BuildResources and then customize the
		// returned objects; `resources.ConfigMap.Data[k] = v` panics on a nil map.
		handler := reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster]("product:latest", testScheme)
		cr := testutil.NewMockCluster("cm-nil", testNamespace)

		resources, err := handler.BuildResources(context.Background(), k8sClient, cr, &reconciler.RoleGroupBuildContext{
			ClusterName:      "cm-nil",
			ClusterNamespace: testNamespace,
			RoleName:         "broker",
			RoleGroupName:    "default",
			ResourceName:     "cm-nil-broker-default",
			ClusterSpec:      cr.GetSpec(),
			MergedConfig:     &config.MergedConfig{},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.ConfigMap).NotTo(BeNil())
		Expect(resources.ConfigMap.Data).NotTo(BeNil())
	})
})

var _ = Describe("Recommended label set", func() {
	var mockCR *testutil.MockCluster

	newHandler := func(productName string) *reconciler.BaseRoleGroupHandler[*testutil.MockCluster] {
		h := reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster]("static-image:1", testScheme)
		h.ProductName = productName
		h.SetRoleServicePorts("server", []corev1.ServicePort{{Name: "http", Port: 8080}})
		return h
	}

	newBuildCtx := func(image *v1alpha1.ImageSpec) *reconciler.RoleGroupBuildContext {
		return &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: testNamespace,
			ClusterSpec:      &v1alpha1.GenericClusterSpec{Image: image},
			RoleName:         "server",
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    "default",
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(1))},
			MergedConfig:     &config.MergedConfig{},
			ResourceName:     "test-cluster-server-default",
		}
	}

	kubedoopImage := func(productVersion string) *v1alpha1.ImageSpec {
		return &v1alpha1.ImageSpec{
			Repo:            "quay.io/kubedoop",
			ProductVersion:  productVersion,
			KubedoopVersion: "0.0.1",
		}
	}

	BeforeEach(func() {
		mockCR = testutil.NewMockCluster("test-cluster", testNamespace)
	})

	It("stamps the complete set on every role group resource and its pod template", func() {
		handler := newHandler("trino")

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR,
			newBuildCtx(kubedoopImage("476")))
		Expect(err).NotTo(HaveOccurred())

		expected := map[string]string{
			constant.LabelKubernetesName:      "trino",
			constant.LabelKubernetesInstance:  "test-cluster",
			constant.LabelKubernetesVersion:   "476",
			constant.LabelKubernetesComponent: "server",
			constant.LabelKubernetesRoleGroup: "default",
			constant.LabelKubernetesManagedBy: "operator-go",
		}

		objects := []client.Object{
			resources.ConfigMap, resources.StatefulSet, resources.HeadlessService, resources.Service,
		}
		for _, obj := range objects {
			Expect(obj).NotTo(BeNil())
			for key, value := range expected {
				Expect(obj.GetLabels()).To(HaveKeyWithValue(key, value),
					"%T should carry %s=%s", obj, key, value)
			}
		}

		// The pod template too: these labels are what makes `kubectl get pods -l
		// app.kubernetes.io/role-group=default` work, and MatchingLabelsNames() promises pods
		// are selectable by them.
		for key, value := range expected {
			Expect(resources.StatefulSet.Spec.Template.Labels).To(HaveKeyWithValue(key, value))
		}
	})

	It("keeps the added labels out of the immutable StatefulSet selector", func() {
		handler := newHandler("trino")

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR,
			newBuildCtx(kubedoopImage("476")))
		Expect(err).NotTo(HaveOccurred())

		// .spec.selector is a one-way door: whatever goes in can never be edited again. Version
		// changes on every product upgrade and role-group is already covered by the marker key,
		// so a cluster upgraded into this framework version must find its selector byte-identical.
		Expect(resources.StatefulSet.Spec.Selector.MatchLabels).To(Equal(map[string]string{
			constant.LabelKubernetesInstance:  "test-cluster",
			constant.LabelKubernetesComponent: "server",
			constant.LabelKubernetesManagedBy: "operator-go",
			"test-cluster-default":            "true",
		}))
		Expect(resources.HeadlessService.Spec.Selector).To(Equal(resources.StatefulSet.Spec.Selector.MatchLabels))

		// And the wider template still satisfies the narrower selector.
		Expect(k8slabels.SelectorFromSet(resources.StatefulSet.Spec.Selector.MatchLabels).
			Matches(k8slabels.Set(resources.StatefulSet.Spec.Template.Labels))).To(BeTrue())
	})

	It("omits name and version for a handler that ignores spec.image", func() {
		// No ProductName: the handler runs its static Image and never reads spec.image, so
		// publishing a version from there would label the pods with something they do not run.
		handler := newHandler("")

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR,
			newBuildCtx(kubedoopImage("476")))
		Expect(err).NotTo(HaveOccurred())

		Expect(resources.StatefulSet.Labels).NotTo(HaveKey(constant.LabelKubernetesName))
		Expect(resources.StatefulSet.Labels).NotTo(HaveKey(constant.LabelKubernetesVersion))
		// role-group is derived from the build context, so it is unconditional.
		Expect(resources.StatefulSet.Labels).To(HaveKeyWithValue(constant.LabelKubernetesRoleGroup, "default"))
	})

	It("publishes the declared productVersion even when a custom image overrides the reference", func() {
		handler := newHandler("trino")

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR,
			newBuildCtx(&v1alpha1.ImageSpec{Custom: "my-registry/trino:custom", ProductVersion: "476"}))
		Expect(err).NotTo(HaveOccurred())

		// Custom replaces the image *reference*; productVersion still declares which product
		// version that reference is expected to be.
		Expect(resources.StatefulSet.Labels).To(HaveKeyWithValue(constant.LabelKubernetesVersion, "476"))
	})

	It("drops a productVersion that is not a legal label value instead of failing the build", func() {
		handler := newHandler("trino")

		// A legal image tag (up to 128 chars) that is not a legal label value (max 63).
		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR,
			newBuildCtx(kubedoopImage(strings.Repeat("9", 64))))
		Expect(err).NotTo(HaveOccurred())

		Expect(resources.StatefulSet.Labels).NotTo(HaveKey(constant.LabelKubernetesVersion))
		Expect(resources.StatefulSet.Labels).To(HaveKeyWithValue(constant.LabelKubernetesName, "trino"))

		// The point of dropping it: every label the framework emits stays applyable, so one
		// cosmetic label goes missing instead of the API server rejecting every resource.
		for key, value := range resources.StatefulSet.Labels {
			Expect(validation.IsValidLabelValue(value)).To(BeEmpty(), "label %s=%s", key, value)
		}
	})

	It("stamps the set minus role-group on the role-level PodDisruptionBudget", func() {
		handler := newHandler("trino")

		pdb := handler.BuildRolePodDisruptionBudget(&reconciler.RoleBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: testNamespace,
			ClusterSpec:      &v1alpha1.GenericClusterSpec{Image: kubedoopImage("476")},
			RoleName:         "server",
			RoleSpec: &v1alpha1.RoleSpec{RoleConfig: &v1alpha1.RoleConfigSpec{
				PodDisruptionBudget: &v1alpha1.PodDisruptionBudgetSpec{Enabled: ptr.To(true)},
			}},
		})
		Expect(pdb).NotTo(BeNil())

		Expect(pdb.Labels).To(HaveKeyWithValue(constant.LabelKubernetesName, "trino"))
		Expect(pdb.Labels).To(HaveKeyWithValue(constant.LabelKubernetesVersion, "476"))
		Expect(pdb.Labels).To(HaveKeyWithValue(constant.LabelKubernetesComponent, "server"))
		// A role-level resource covers every group of the role.
		Expect(pdb.Labels).NotTo(HaveKey(constant.LabelKubernetesRoleGroup))

		// The selector must stay version-free: a version-scoped PDB would stop matching its pods
		// the moment the label changed — i.e. during the upgrade rollout the PDB exists for.
		Expect(pdb.Spec.Selector.MatchLabels).NotTo(HaveKey(constant.LabelKubernetesVersion))
		Expect(pdb.Spec.Selector.MatchLabels).NotTo(HaveKey(constant.LabelKubernetesName))
	})
})

var _ = Describe("Role group marker label key", func() {
	// The marker is a label KEY built from two free-form user strings ("<cluster>-<group>"), and it
	// lands in the StatefulSet's immutable .spec.selector, in both Services' selectors and in every
	// pod template label set. A label key's name part is capped at 63 bytes, which ordinary
	// big-data names exceed: 43 + 1 + 21 below is 65.
	const (
		longCluster = "analytics-platform-production-trino-cluster"
		longGroup   = "memory-optimized-pool"
	)

	newHandler := func() *reconciler.BaseRoleGroupHandler[*testutil.MockCluster] {
		h := reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster]("static-image:1", testScheme)
		h.SetRoleServicePorts("worker", []corev1.ServicePort{{Name: "http", Port: 8080}})
		return h
	}

	newBuildCtx := func(clusterName, roleName, groupName string) *reconciler.RoleGroupBuildContext {
		return &reconciler.RoleGroupBuildContext{
			ClusterName:      clusterName,
			ClusterNamespace: testNamespace,
			ClusterSpec:      &v1alpha1.GenericClusterSpec{},
			RoleName:         roleName,
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    groupName,
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(1))},
			MergedConfig:     &config.MergedConfig{},
			ResourceName:     reconciler.RoleGroupResourceName(clusterName, roleName, groupName),
		}
	}

	It("builds a role group the API server accepts when the natural key would overrun", func() {
		// This is the whole point: not "the key is short" but "the role group can be created".
		// With the natural key the API server rejects the StatefulSet, both Services and the PDB,
		// quoting a label key the user never wrote.
		Expect(len(longCluster+"-"+longGroup)).To(BeNumerically(">", 63),
			"the fixture must actually overrun, or this spec proves nothing")

		cr := testutil.NewMockCluster(longCluster, testNamespace)
		resources, err := newHandler().BuildResources(context.Background(), k8sClient, cr,
			newBuildCtx(longCluster, "worker", longGroup))
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Create(context.Background(), resources.StatefulSet)).To(Succeed(),
			"the API server must accept a role group whose cluster+group names exceed the label key limit")
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(context.Background(), resources.StatefulSet))).To(Succeed())
		})

		// The client Service carries the same selector, and the pod template the same key set.
		for key := range resources.Service.Spec.Selector {
			Expect(validation.IsQualifiedName(key)).To(BeEmpty(), "Service selector key %q", key)
		}
		for key := range resources.StatefulSet.Spec.Template.Labels {
			Expect(validation.IsQualifiedName(key)).To(BeEmpty(), "pod template label key %q", key)
		}
	})

	It("keeps the natural key byte-identical when it fits", func() {
		// .spec.selector is immutable. A cluster created by an earlier framework version must find
		// its selector unchanged, or the pod template stops matching the frozen selector and every
		// later update is rejected — so the substitute may only ever apply to combinations that
		// could not have produced a StatefulSet in the first place.
		Expect(reconciler.RoleGroupMarkerLabelKey("trino", "worker", "default")).
			To(Equal("trino-default"))

		cr := testutil.NewMockCluster("trino", testNamespace)
		resources, err := newHandler().BuildResources(context.Background(), k8sClient, cr,
			newBuildCtx("trino", "worker", "default"))
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.StatefulSet.Spec.Selector.MatchLabels).To(HaveKeyWithValue("trino-default", "true"))
	})

	It("stamps the SAME marker on two roles sharing a role group name", func() {
		// Documented property, not an oversight (issue #605): the natural key is "<cluster>-<group>"
		// with no role in it, so every role using the conventional group name "default" carries the
		// same marker. The role cannot be added — the key lands in the immutable .spec.selector, so
		// changing it would break every running cluster on upgrade — which is exactly why nothing
		// may select on this key alone. Its one consumer reaches it only after a Get by the role
		// group's derived name, which does carry the role.
		Expect(reconciler.RoleGroupMarkerLabelKey("trino", "worker", "default")).
			To(Equal(reconciler.RoleGroupMarkerLabelKey("trino", "coordinator", "default")))

		// The over-long substitute is RoleGroupResourceName, which IS role-scoped. The two forms
		// deliberately identify different things; a caller may rely on neither.
		Expect(reconciler.RoleGroupMarkerLabelKey(longCluster, "worker", longGroup)).
			NotTo(Equal(reconciler.RoleGroupMarkerLabelKey(longCluster, "coordinator", longGroup)))
	})

	It("keeps two overrunning role groups of one role distinguishable", func() {
		// The substitute must stay unique per role group: two groups sharing a marker would make
		// each role group's Services select the other's pods as well.
		a := reconciler.RoleGroupMarkerLabelKey(longCluster, "worker", longGroup)
		b := reconciler.RoleGroupMarkerLabelKey(longCluster, "worker", longGroup+"-spot")
		Expect(a).NotTo(Equal(b))
		Expect(validation.IsQualifiedName(a)).To(BeEmpty())
		Expect(validation.IsQualifiedName(b)).To(BeEmpty())

		// And it stays stable across calls, or the selector would differ between reconciles.
		Expect(reconciler.RoleGroupMarkerLabelKey(longCluster, "worker", longGroup)).To(Equal(a))
	})
})

var _ = Describe("fsGroup ownership recursion", func() {
	// fsGroup makes the kubelet apply group ownership to every volume that supports it. Without a
	// change policy Kubernetes uses Always, which recurses over the ENTIRE volume on every mount —
	// on a data PVC holding millions of files that is tens of minutes of ContainerCreating on every
	// single pod start. These specs pin the default and the escape hatch.
	newHandler := func() *reconciler.BaseRoleGroupHandler[*testutil.MockCluster] {
		h := reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster]("product:1", testScheme)
		// A data PVC is what the policy actually applies to; ephemeral volumes ignore it.
		h.StorageMountPath = "/kubedoop/data"
		return h
	}

	newBuildCtx := func(name string, overrides *corev1.PodTemplateSpec) *reconciler.RoleGroupBuildContext {
		return &reconciler.RoleGroupBuildContext{
			ClusterName:      name,
			ClusterNamespace: testNamespace,
			ClusterSpec:      &v1alpha1.GenericClusterSpec{},
			RoleName:         "datanode",
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    "default",
			RoleGroupSpec: v1alpha1.RoleGroupSpec{
				Replicas: ptr.To(int32(1)),
				Config: &v1alpha1.RoleGroupConfigSpec{
					Resources: &v1alpha1.ResourcesSpec{
						Storage: &v1alpha1.StorageResource{Capacity: ptr.To(resource.MustParse("1Gi"))},
					},
				},
			},
			MergedConfig: &config.MergedConfig{PodOverrides: overrides},
			ResourceName: reconciler.RoleGroupResourceName(name, "datanode", "default"),
		}
	}

	It("survives the API server on a StatefulSet that actually has a data PVC", func() {
		// Building the field is not the same as it reaching the workload: it has to be a field the
		// API server stores rather than prunes, on the object the kubelet will read.
		cr := testutil.NewMockCluster("fsg-store", testNamespace)
		resources, err := newHandler().BuildResources(context.Background(), k8sClient, cr,
			newBuildCtx("fsg-store", nil))
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.StatefulSet.Spec.VolumeClaimTemplates).NotTo(BeEmpty(),
			"the fixture must carry a PVC, or the policy under test would not apply to anything")

		Expect(k8sClient.Create(context.Background(), resources.StatefulSet)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(context.Background(), resources.StatefulSet))).To(Succeed())
		})

		stored := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(context.Background(),
			client.ObjectKeyFromObject(resources.StatefulSet), stored)).To(Succeed())
		Expect(stored.Spec.Template.Spec.SecurityContext.FSGroupChangePolicy).NotTo(BeNil())
		Expect(*stored.Spec.Template.Spec.SecurityContext.FSGroupChangePolicy).
			To(Equal(corev1.FSGroupChangeOnRootMismatch))
	})

	It("lets a product take the recursion back with podOverrides", func() {
		// OnRootMismatch trades one repair for a lot of startup time: it will not fix ownership
		// that drifted deep inside a volume whose root is still correct. A product that wants that
		// repair must be able to ask for it, and must keep the rest of the hardening while doing so.
		resources, err := newHandler().BuildResources(context.Background(), k8sClient,
			testutil.NewMockCluster("fsg-override", testNamespace),
			newBuildCtx("fsg-override", &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						FSGroupChangePolicy: ptr.To(corev1.FSGroupChangeAlways),
					},
				},
			}))
		Expect(err).NotTo(HaveOccurred())

		podSC := resources.StatefulSet.Spec.Template.Spec.SecurityContext
		Expect(podSC.FSGroupChangePolicy).NotTo(BeNil())
		Expect(*podSC.FSGroupChangePolicy).To(Equal(corev1.FSGroupChangeAlways))
		// The fields the override did not mention survive the strategic merge.
		Expect(podSC.FSGroup).NotTo(BeNil())
		Expect(*podSC.FSGroup).To(Equal(int64(1001)))
		Expect(podSC.RunAsNonRoot).NotTo(BeNil())
		Expect(*podSC.RunAsNonRoot).To(BeTrue())
	})
})

var _ = Describe("Per-CR inputs on the build context", func() {
	// The handler is constructed once in main.go and shared across every CR and every reconcile.
	// These specs pin the channel that lets a product supply per-cluster values without writing
	// them into that shared instance.
	var mockCR *testutil.MockCluster

	BeforeEach(func() { mockCR = testutil.NewMockCluster("test-cluster", "default") })

	buildCtxWith := func(mutate func(*reconciler.RoleGroupBuildContext)) *reconciler.RoleGroupBuildContext {
		buildCtx := &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			RoleName:         "server",
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    "default",
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(1))},
			MergedConfig:     &config.MergedConfig{},
			ResourceName:     "test-cluster-server-default",
		}
		if mutate != nil {
			mutate(buildCtx)
		}
		return buildCtx
	}

	It("prefers the per-call image and ports over the handler's own", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("handler-image:1", testScheme)
		handler.SetRoleImage("server", "role-image:1")
		handler.SetRoleContainerPorts("server", []corev1.ContainerPort{{Name: "http", ContainerPort: 8080}})
		handler.SetRoleServicePorts("server", []corev1.ServicePort{{Name: "http", Port: 8080}})

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR,
			buildCtxWith(func(b *reconciler.RoleGroupBuildContext) {
				b.Image = "per-call:2"
				b.ImagePullPolicy = corev1.PullAlways
				b.ContainerPorts = []corev1.ContainerPort{{Name: "https", ContainerPort: 8443}}
				b.ServicePorts = []corev1.ServicePort{{Name: "https", Port: 8443}}
			}))
		Expect(err).NotTo(HaveOccurred())

		container := resources.StatefulSet.Spec.Template.Spec.Containers[0]
		Expect(container.Image).To(Equal("per-call:2"))
		Expect(container.ImagePullPolicy).To(Equal(corev1.PullAlways))
		Expect(container.Ports).To(HaveLen(1))
		Expect(container.Ports[0].ContainerPort).To(Equal(int32(8443)))
		Expect(resources.Service.Spec.Ports[0].Port).To(Equal(int32(8443)))
	})

	It("falls back to the handler when the build states nothing", func() {
		// Reconcile-INVARIANT settings still belong on the handler; this channel is only for the
		// values that depend on which cluster is being built.
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("handler-image:1", testScheme)
		handler.SetRoleContainerPorts("server", []corev1.ContainerPort{{Name: "http", ContainerPort: 8080}})

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR, buildCtxWith(nil))
		Expect(err).NotTo(HaveOccurred())

		container := resources.StatefulSet.Spec.Template.Spec.Containers[0]
		Expect(container.Image).To(Equal("handler-image:1"))
		Expect(container.Ports[0].ContainerPort).To(Equal(int32(8080)))
	})

	It("does not carry one cluster's values into the next through the shared handler", func() {
		// The quiet half of the hazard, and the one that bites at the default concurrency of 1:
		// spark-k8s-operator shipped a CR inheriting the previous CR's pullPolicy, with a serial
		// reconcile loop and no race involved.
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("handler-image:1", testScheme)

		first, err := handler.BuildResources(context.Background(), k8sClient, mockCR,
			buildCtxWith(func(b *reconciler.RoleGroupBuildContext) {
				b.Image = "cluster-a:1"
				b.ImagePullPolicy = corev1.PullAlways
				b.ContainerPorts = []corev1.ContainerPort{{Name: "https", ContainerPort: 8443}}
			}))
		Expect(err).NotTo(HaveOccurred())
		Expect(first.StatefulSet.Spec.Template.Spec.Containers[0].Image).To(Equal("cluster-a:1"))

		// The second cluster states nothing — under the old idiom it would inherit cluster A's.
		second, err := handler.BuildResources(context.Background(), k8sClient,
			testutil.NewMockCluster("other-cluster", "default"), buildCtxWith(nil))
		Expect(err).NotTo(HaveOccurred())

		container := second.StatefulSet.Spec.Template.Spec.Containers[0]
		Expect(container.Image).To(Equal("handler-image:1"), "cluster A's image must not leak")
		Expect(container.ImagePullPolicy).NotTo(Equal(corev1.PullAlways))
		Expect(container.Ports).To(BeEmpty(), "cluster A's ports must not leak")
	})

	It("builds concurrently on one handler without crossing values", func() {
		// The loud half: this is what raising MaxConcurrentReconciles above 1 does. `make test`
		// runs with -race in CI, so a handler-side write would be reported as a data race here
		// rather than as a wrong value.
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("handler-image:1", testScheme)

		const workers = 8
		images := make([]string, workers)
		var wg sync.WaitGroup
		for i := range workers {
			wg.Add(1)
			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				want := fmt.Sprintf("cluster-%d:1", i)
				resources, err := handler.BuildResources(context.Background(), k8sClient,
					testutil.NewMockCluster(fmt.Sprintf("cluster-%d", i), "default"),
					buildCtxWith(func(b *reconciler.RoleGroupBuildContext) {
						b.ClusterName = fmt.Sprintf("cluster-%d", i)
						b.ResourceName = fmt.Sprintf("cluster-%d-server-default", i)
						b.Image = want
					}))
				Expect(err).NotTo(HaveOccurred())
				images[i] = resources.StatefulSet.Spec.Template.Spec.Containers[0].Image
			}()
		}
		wg.Wait()

		for i := range workers {
			Expect(images[i]).To(Equal(fmt.Sprintf("cluster-%d:1", i)))
		}
	})

	It("does not write one cluster's image into a handler-wide sidecar manager", func() {
		// The framework's own instance of the same defect: SetProductImage writes into the
		// manager's configs, and a manager registered on the handler outlives the build.
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("handler-image:1", testScheme)
		mgr := sidecar.NewSidecarManager()
		mgr.Register(&recordingSidecarProvider{name: "probe"}, &sidecar.SidecarConfig{Enabled: true})
		handler.WithSidecarManager(mgr)

		_, err := handler.BuildResources(context.Background(), k8sClient, mockCR,
			buildCtxWith(func(b *reconciler.RoleGroupBuildContext) { b.Image = "cluster-a:1" }))
		Expect(err).NotTo(HaveOccurred())

		cfg, ok := mgr.GetConfig("probe")
		Expect(ok).To(BeTrue())
		Expect(cfg.Image).To(BeEmpty(),
			"the handler's manager is process-wide; this cluster's image must not be written into it")
	})
})

// recordingSidecarProvider is a minimal provider: it only has to be registered so SetProductImage
// has a config to write into.
type recordingSidecarProvider struct{ name string }

func (p *recordingSidecarProvider) Name() string { return p.name }
func (p *recordingSidecarProvider) Inject(podSpec *corev1.PodSpec, cfg *sidecar.SidecarConfig) error {
	podSpec.InitContainers = append(podSpec.InitContainers,
		corev1.Container{Name: p.name, Image: cfg.Image})
	return nil
}
func (p *recordingSidecarProvider) Validate(context.Context, client.Client, string) error { return nil }

var _ = Describe("Declaring intent before Build", func() {
	// Both of these replace the same shape: the framework builds a complete object and the product
	// reaches in and edits it. Declaring the intent first keeps ordering framework-owned.
	var mockCR *testutil.MockCluster

	BeforeEach(func() { mockCR = testutil.NewMockCluster("test-cluster", "default") })

	declareCtx := func(mutate func(*reconciler.RoleGroupBuildContext)) *reconciler.RoleGroupBuildContext {
		buildCtx := &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			RoleName:         "server",
			RoleSpec:         &v1alpha1.RoleSpec{},
			RoleGroupName:    "default",
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(1))},
			MergedConfig:     &config.MergedConfig{},
			ResourceName:     "test-cluster-server-default",
			ServicePorts:     []corev1.ServicePort{{Name: "http", Port: 8080}},
		}
		if mutate != nil {
			mutate(buildCtx)
		}
		return buildCtx
	}

	It("sets the client Service type from the declared listener class", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("img:1", testScheme)

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR,
			declareCtx(func(b *reconciler.RoleGroupBuildContext) {
				b.ListenerClass = listener.ListenerClassExternalUnstable
			}))
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.Service.Spec.Type).To(Equal(corev1.ServiceTypeNodePort))
	})

	It("leaves the Service a ClusterIP when no class is declared", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("img:1", testScheme)

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR, declareCtx(nil))
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.Service.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
	})

	It("hands the customizer the primary container, whichever index it ends up at", func() {
		// zookeeper-operator located it as Containers[0], an assumption the framework never made:
		// a sidecar provider inserting a container earlier silently configures the wrong one.
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("img:1", testScheme)
		handler.MainContainerName = "zookeeper"

		var sawName string
		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR,
			declareCtx(func(b *reconciler.RoleGroupBuildContext) {
				b.MainContainerCustomizer = func(c *corev1.Container) error {
					sawName = c.Name
					c.Command = []string{"/bin/zkServer.sh"}
					c.Args = []string{"start-foreground"}
					c.Env = append(c.Env, corev1.EnvVar{Name: "ZK_ID", Value: "1"})
					return nil
				}
			}))
		Expect(err).NotTo(HaveOccurred())
		Expect(sawName).To(Equal("zookeeper"), "the customizer is handed the container by identity, not by index")

		main := resources.StatefulSet.Spec.Template.Spec.Containers[0]
		Expect(main.Command).To(Equal([]string{"/bin/zkServer.sh"}))
		Expect(main.Args).To(Equal([]string{"start-foreground"}))
		Expect(main.Env).To(ContainElement(corev1.EnvVar{Name: "ZK_ID", Value: "1"}))
	})

	It("lets podOverrides outrank the customizer", func() {
		// The reason this is a pre-Build hook rather than a post-build patch. A product editing the
		// returned StatefulSet would land AFTER the strategic merge and silently beat the user.
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("img:1", testScheme)
		handler.MainContainerName = "main"

		resources, err := handler.BuildResources(context.Background(), k8sClient, mockCR,
			declareCtx(func(b *reconciler.RoleGroupBuildContext) {
				b.MergedConfig.PodOverrides = &corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{{
						Name: "main",
						Env:  []corev1.EnvVar{{Name: "LEVEL", Value: "from-user"}},
					}}},
				}
				b.MainContainerCustomizer = func(c *corev1.Container) error {
					c.Env = append(c.Env, corev1.EnvVar{Name: "LEVEL", Value: "from-product"})
					return nil
				}
			}))
		Expect(err).NotTo(HaveOccurred())

		main := resources.StatefulSet.Spec.Template.Spec.Containers[0]
		Expect(main.Env).To(ContainElement(corev1.EnvVar{Name: "LEVEL", Value: "from-user"}))
		Expect(main.Env).NotTo(ContainElement(corev1.EnvVar{Name: "LEVEL", Value: "from-product"}))
	})

	It("fails the role group when the customizer errors", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("img:1", testScheme)

		_, err := handler.BuildResources(context.Background(), k8sClient, mockCR,
			declareCtx(func(b *reconciler.RoleGroupBuildContext) {
				b.MainContainerCustomizer = func(*corev1.Container) error {
					return fmt.Errorf("no JVM heap could be computed")
				}
			}))
		Expect(err).To(MatchError(ContainSubstring("no JVM heap could be computed")))
		Expect(reconciler.IsValidationError(err)).To(BeTrue())
	})

	It("refuses an image change from the customizer, and keeps the resolved one", func() {
		// The image is propagated to the sidecars before the StatefulSet is built (the Vector agent
		// ships inside the product image), so changing it here would leave them on the old one.
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("img:1", testScheme)

		_, err := handler.BuildResources(context.Background(), k8sClient, mockCR,
			declareCtx(func(b *reconciler.RoleGroupBuildContext) {
				b.MainContainerCustomizer = func(c *corev1.Container) error {
					c.Image = "sneaky:2"
					return nil
				}
			}))
		Expect(err).To(MatchError(ContainSubstring("RoleGroupBuildContext.Image")))
		Expect(err).To(MatchError(ContainSubstring("sneaky:2")))
	})
})
