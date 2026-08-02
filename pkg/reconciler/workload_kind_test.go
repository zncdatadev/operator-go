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
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// staticWorkloadKinds is a minimal WorkloadKindProvider for wiring the health manager directly,
// the way a product handler embedding BaseRoleGroupHandler provides it through the promoted method.
type staticWorkloadKinds map[string]reconciler.WorkloadKind

func (s staticWorkloadKinds) WorkloadKindFor(roleName string) reconciler.WorkloadKind {
	if kind, ok := s[roleName]; ok {
		return kind
	}
	return reconciler.WorkloadKindStatefulSet
}

var _ = Describe("Workload kind selection", func() {
	var handler *reconciler.BaseRoleGroupHandler[common.ClusterInterface]
	var ctx context.Context
	var mockCR *testutil.MockCluster
	var buildCtx *reconciler.RoleGroupBuildContext

	BeforeEach(func() {
		ctx = context.Background()
		handler = reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("test-image:latest", testScheme)
		mockCR = testutil.NewMockCluster("wk-cluster", "default")

		buildCtx = &reconciler.RoleGroupBuildContext{
			ClusterName:      "wk-cluster",
			ClusterNamespace: "default",
			ClusterSpec: &v1alpha1.GenericClusterSpec{
				Roles: map[string]v1alpha1.RoleSpec{
					"web": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{
							"default": {Replicas: ptr.To(int32(2))},
						},
					},
				},
			},
			RoleName:      "web",
			RoleSpec:      &v1alpha1.RoleSpec{},
			RoleGroupName: "default",
			RoleGroupSpec: v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(2))},
			MergedConfig:  &config.MergedConfig{},
			ResourceName:  "wk-cluster-web-default",
		}
	})

	Describe("WorkloadKindFor", func() {
		It("should default to StatefulSet for a role never configured", func() {
			Expect(handler.WorkloadKindFor("anything")).To(Equal(reconciler.WorkloadKindStatefulSet))
		})

		It("should return the configured kind, initializing the map when nil", func() {
			nilHandler := &reconciler.BaseRoleGroupHandler[common.ClusterInterface]{}
			Expect(nilHandler.RoleWorkloadKinds).To(BeNil())
			nilHandler.SetRoleWorkloadKind("web", reconciler.WorkloadKindDeployment)
			Expect(nilHandler.WorkloadKindFor("web")).To(Equal(reconciler.WorkloadKindDeployment))
			Expect(nilHandler.WorkloadKindFor("other")).To(Equal(reconciler.WorkloadKindStatefulSet))
		})
	})

	Describe("ConfiguredRoleNames", func() {
		It("should include roles configured only through SetRoleWorkloadKind", func() {
			// A typo in the role name would otherwise silently build the role as a StatefulSet;
			// listing the name is what lets the reconciler's cross-check warn about it.
			handler.SetRoleWorkloadKind("ui", reconciler.WorkloadKindDeployment)
			Expect(handler.ConfiguredRoleNames()).To(ContainElement("ui"))
		})
	})

	Describe("BuildResources kind switching", func() {
		It("should build a StatefulSet with a headless Service by default", func() {
			resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(resources.StatefulSet).NotTo(BeNil())
			Expect(resources.Deployment).To(BeNil())
			Expect(resources.HeadlessService).NotTo(BeNil())
		})

		It("should build a Deployment and skip the headless Service for a Deployment role", func() {
			handler.SetRoleWorkloadKind("web", reconciler.WorkloadKindDeployment)
			handler.SetRoleServicePorts("web", []corev1.ServicePort{{Name: "http", Port: 8080}})

			resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)
			Expect(err).NotTo(HaveOccurred())

			Expect(resources.Deployment).NotTo(BeNil())
			Expect(resources.StatefulSet).To(BeNil())
			// No stable pod identity exists for a headless Service to name.
			Expect(resources.HeadlessService).To(BeNil())
			// The client-facing Service is kind-independent: clients reach the role the same way.
			Expect(resources.Service).NotTo(BeNil())

			deployment := resources.Deployment
			Expect(deployment.Name).To(Equal("wk-cluster-web-default"))
			Expect(*deployment.Spec.Replicas).To(Equal(int32(2)))
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal("test-image:latest"))
			// The config ConfigMap is wired exactly as it is for a StatefulSet role.
			Expect(deployment.Spec.Template.Spec.Volumes).To(ContainElement(SatisfyAll(
				WithTransform(func(v corev1.Volume) string { return v.Name }, Equal("config")),
			)))
			// The selector must be the framework identity subset and a subset of the pod labels.
			Expect(deployment.Spec.Selector.MatchLabels).To(HaveKeyWithValue(constant.LabelKubernetesInstance, "wk-cluster"))
			Expect(deployment.Spec.Template.Labels).To(HaveKeyWithValue(constant.LabelKubernetesInstance, "wk-cluster"))
		})

		It("should ignore StorageMountPath for a Deployment role", func() {
			// A Deployment has no volumeClaimTemplates: the data-PVC opt-in is a StatefulSet
			// contract, and a Deployment role with storage configured must not grow a mount that
			// references a claim nothing will ever create.
			handler.SetRoleWorkloadKind("web", reconciler.WorkloadKindDeployment)
			handler.StorageMountPath = "/kubedoop/data"
			buildCtx.RoleGroupSpec.Config = &v1alpha1.RoleGroupConfigSpec{
				Resources: &v1alpha1.ResourcesSpec{
					Storage: &v1alpha1.StorageResource{},
				},
			}

			resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)
			Expect(err).NotTo(HaveOccurred())

			for _, mount := range resources.Deployment.Spec.Template.Spec.Containers[0].VolumeMounts {
				Expect(mount.Name).NotTo(Equal("data"))
			}
		})

		It("should still build the data PVC for a StatefulSet role with the same configuration", func() {
			handler.StorageMountPath = "/kubedoop/data"
			buildCtx.RoleGroupSpec.Config = &v1alpha1.RoleGroupConfigSpec{
				Resources: &v1alpha1.ResourcesSpec{
					Storage: &v1alpha1.StorageResource{},
				},
			}

			resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)
			Expect(err).NotTo(HaveOccurred())

			Expect(resources.StatefulSet.Spec.VolumeClaimTemplates).To(HaveLen(1))
			Expect(resources.StatefulSet.Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(
				WithTransform(func(m corev1.VolumeMount) string { return m.Name }, Equal("data")),
			))
		})

		It("should force a Deployment role's replicas to zero when the cluster is stopped", func() {
			handler.SetRoleWorkloadKind("web", reconciler.WorkloadKindDeployment)
			buildCtx.ClusterSpec.ClusterOperation = &v1alpha1.ClusterOperationSpec{Stopped: true}

			resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(*resources.Deployment.Spec.Replicas).To(Equal(int32(0)))
		})

		It("should fail loudly on a workload kind it does not know", func() {
			// The kinds are typed constants, so this takes a deliberate cast — but silently
			// building a StatefulSet for it would deploy a workload shape nobody asked for.
			handler.SetRoleWorkloadKind("web", reconciler.WorkloadKind("DaemonSet"))

			_, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown workload kind"))
			Expect(err.Error()).To(ContainSubstring("DaemonSet"))
		})

		It("should render the same pod spec for both kinds given identical inputs", func() {
			// The builder-level parity spec pins the template; this pins the HANDLER wiring — the
			// config volume, security context, image resolution and replica handling must flow
			// into a Deployment exactly as they flow into a StatefulSet.
			stsResources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)
			Expect(err).NotTo(HaveOccurred())

			handler.SetRoleWorkloadKind("web", reconciler.WorkloadKindDeployment)
			deploymentResources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)
			Expect(err).NotTo(HaveOccurred())

			Expect(deploymentResources.Deployment.Spec.Template).To(Equal(stsResources.StatefulSet.Spec.Template))
			Expect(deploymentResources.Deployment.Spec.Selector).To(Equal(stsResources.StatefulSet.Spec.Selector))
		})
	})
})

var _ = Describe("GenericReconciler Deployment workload", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	// cleanupRoleGroupResources removes whatever a reconcile created for the role group, so a
	// failing spec does not leak objects into the shared namespace.
	cleanupRoleGroupResources := func(resourceName string) {
		meta := metav1.ObjectMeta{Name: resourceName, Namespace: testNamespace}
		_ = k8sClient.Delete(ctx, &appsv1.Deployment{ObjectMeta: meta})
		_ = k8sClient.Delete(ctx, &appsv1.StatefulSet{ObjectMeta: meta})
		_ = k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: meta})
		_ = k8sClient.Delete(ctx, &corev1.Service{ObjectMeta: meta})
		_ = k8sClient.Delete(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: resourceName + "-headless", Namespace: testNamespace}})
	}

	It("should apply a Deployment in the workload slot and no headless Service", func() {
		name := uniqueCRName("deploy-workload")
		cr := testutil.NewMockCluster(name, testNamespace).
			WithRoles(map[string]v1alpha1.RoleSpec{
				"web": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {Replicas: ptr.To(int32(2))},
					},
				},
			})
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		resourceName := reconciler.RoleGroupResourceName(name, "web", "default")
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, cr)
			cleanupRoleGroupResources(resourceName)
		})

		handler := reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster]("web-image:1", testScheme)
		handler.SetRoleWorkloadKind("web", reconciler.WorkloadKindDeployment)
		handler.SetRoleServicePorts("web", []corev1.ServicePort{{Name: "http", Port: 8080}})

		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: handler,
			Prototype:        testutil.NewMockCluster("proto", testNamespace),
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: name}})
		Expect(err).NotTo(HaveOccurred())

		key := types.NamespacedName{Namespace: testNamespace, Name: resourceName}

		deployment := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, key, deployment)).To(Succeed())
		Expect(*deployment.Spec.Replicas).To(Equal(int32(2)))

		// The workload slot holds exactly one kind, and a Deployment role has no per-pod DNS
		// identity for a headless Service to serve.
		Expect(k8serrors.IsNotFound(k8sClient.Get(ctx, key, &appsv1.StatefulSet{}))).To(BeTrue())
		headlessKey := types.NamespacedName{Namespace: testNamespace, Name: resourceName + "-headless"}
		Expect(k8serrors.IsNotFound(k8sClient.Get(ctx, headlessKey, &corev1.Service{}))).To(BeTrue())

		// ConfigMap and client Service arrive exactly as they do for a StatefulSet role.
		Expect(k8sClient.Get(ctx, key, &corev1.ConfigMap{})).To(Succeed())
		Expect(k8sClient.Get(ctx, key, &corev1.Service{})).To(Succeed())
	})

	It("should update an existing Deployment while preserving its immutable selector", func() {
		name := uniqueCRName("deploy-update")
		cr := testutil.NewMockCluster(name, testNamespace).
			WithRoles(map[string]v1alpha1.RoleSpec{
				"web": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {Replicas: ptr.To(int32(1))},
					},
				},
			})
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		resourceName := reconciler.RoleGroupResourceName(name, "web", "default")
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, cr)
			cleanupRoleGroupResources(resourceName)
		})

		handler := reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster]("web-image:1", testScheme)
		handler.SetRoleWorkloadKind("web", reconciler.WorkloadKindDeployment)

		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: handler,
			Prototype:        testutil.NewMockCluster("proto", testNamespace),
		})
		Expect(err).NotTo(HaveOccurred())

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: name}}
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		key := types.NamespacedName{Namespace: testNamespace, Name: resourceName}
		deployment := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, key, deployment)).To(Succeed())
		originalSelector := deployment.Spec.Selector.DeepCopy()

		// A spec change must propagate through the update path (the apply path is not
		// create-only), and the immutable selector must survive it untouched.
		fetched := cr.DeepCopy()
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: name}, fetched)).To(Succeed())
		roleSpec := fetched.Spec.Roles["web"]
		roleSpec.RoleGroups["default"] = v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(3))}
		fetched.Spec.Roles["web"] = roleSpec
		Expect(k8sClient.Update(ctx, fetched)).To(Succeed())

		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, key, deployment)).To(Succeed())
		Expect(*deployment.Spec.Replicas).To(Equal(int32(3)))
		Expect(deployment.Spec.Selector).To(Equal(originalSelector))
	})

	It("should reject a handler that fills both workload slots", func() {
		name := uniqueCRName("both-workloads")
		cr := testutil.NewMockCluster(name, testNamespace).
			WithRoles(map[string]v1alpha1.RoleSpec{
				"web": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {Replicas: ptr.To(int32(1))},
					},
				},
			})
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cr) })

		workloadMeta := func(suffix string) metav1.ObjectMeta {
			return metav1.ObjectMeta{
				Name:      reconciler.RoleGroupResourceName(name, "web", "default") + suffix,
				Namespace: testNamespace,
			}
		}
		podTemplate := corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "both"}},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "c", Image: "busybox"}},
			},
		}

		handler := &reconciler.RoleGroupHandlerFuncs[*testutil.MockCluster]{
			BuildResourcesFunc: func(ctx context.Context, c client.Client, cr *testutil.MockCluster, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
				return &reconciler.RoleGroupResources{
					StatefulSet: &appsv1.StatefulSet{
						ObjectMeta: workloadMeta(""),
						Spec: appsv1.StatefulSetSpec{
							Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "both"}},
							Template: podTemplate,
						},
					},
					Deployment: &appsv1.Deployment{
						ObjectMeta: workloadMeta(""),
						Spec: appsv1.DeploymentSpec{
							Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "both"}},
							Template: podTemplate,
						},
					},
				}, nil
			},
		}

		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: handler,
			Prototype:        testutil.NewMockCluster("proto", testNamespace),
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: name}})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("exactly one workload"))

		// The rejection happens before anything is written: neither workload may exist.
		key := types.NamespacedName{Namespace: testNamespace, Name: reconciler.RoleGroupResourceName(name, "web", "default")}
		Expect(k8serrors.IsNotFound(k8sClient.Get(ctx, key, &appsv1.StatefulSet{}))).To(BeTrue())
		Expect(k8serrors.IsNotFound(k8sClient.Get(ctx, key, &appsv1.Deployment{}))).To(BeTrue())
	})
})

var _ = Describe("HealthManager Deployment workloads", func() {
	var healthManager *reconciler.HealthManager
	var ctx context.Context
	var spec *v1alpha1.GenericClusterSpec
	var status *v1alpha1.GenericClusterStatus

	const clusterName = "health-deploy"

	BeforeEach(func() {
		ctx = context.Background()
		healthManager = reconciler.NewHealthManager(k8sClient).
			WithWorkloadKindProvider(staticWorkloadKinds{"web": reconciler.WorkloadKindDeployment})
		spec = &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				"web": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {Replicas: ptr.To(int32(2))},
					},
				},
			},
		}
		status = &v1alpha1.GenericClusterStatus{}
	})

	// newHealthDeployment creates the role group's Deployment and stamps the status the (absent
	// in envtest) deployment controller would have written.
	newHealthDeployment := func(name string, ready int32) *appsv1.Deployment {
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
				Labels:    map[string]string{"app": name},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To(int32(2)),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "c", Image: "busybox"}},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, deployment)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, deployment) })

		deployment.Status = appsv1.DeploymentStatus{
			ObservedGeneration: deployment.Generation,
			Replicas:           2,
			UpdatedReplicas:    2,
			ReadyReplicas:      ready,
			AvailableReplicas:  ready,
		}
		Expect(k8sClient.Status().Update(ctx, deployment)).To(Succeed())
		return deployment
	}

	It("should report Available from the Deployment's ready replicas", func() {
		newHealthDeployment(reconciler.RoleGroupResourceName(clusterName, "web", "default"), 2)

		Expect(healthManager.Check(ctx, "default", clusterName, spec, status)).To(Succeed())

		available := status.GetCondition(v1alpha1.ConditionAvailable)
		Expect(available).NotTo(BeNil())
		Expect(available.Status).To(Equal(metav1.ConditionTrue))
	})

	It("should report the short role group when the Deployment has fewer ready replicas than desired", func() {
		newHealthDeployment(reconciler.RoleGroupResourceName(clusterName, "web", "default"), 1)

		Expect(healthManager.Check(ctx, "default", clusterName, spec, status)).To(Succeed())

		available := status.GetCondition(v1alpha1.ConditionAvailable)
		Expect(available).NotTo(BeNil())
		Expect(available.Status).To(Equal(metav1.ConditionFalse))
		Expect(available.Message).To(ContainSubstring("fewer ready replicas than desired: web/default"))
	})

	It("should name the Deployment — not a StatefulSet — when the workload cannot be read", func() {
		// No Deployment exists at all. The message must send the operator to the object the role
		// actually runs; before workload kinds the health manager would have looked for (and
		// named) a StatefulSet this role never had.
		Expect(healthManager.Check(ctx, "default", clusterName, spec, status)).To(Succeed())

		available := status.GetCondition(v1alpha1.ConditionAvailable)
		Expect(available).NotTo(BeNil())
		Expect(available.Status).To(Equal(metav1.ConditionFalse))
		Expect(available.Message).To(ContainSubstring("Deployment could not be read: web/default"))

		degraded := status.GetCondition(v1alpha1.ConditionDegraded)
		Expect(degraded).NotTo(BeNil())
		Expect(degraded.Status).To(Equal(metav1.ConditionTrue))
		Expect(degraded.Message).To(ContainSubstring("Deployment could not be read for role groups: web/default"))
	})

	It("should fall back to the StatefulSet path without a provider", func() {
		// A hand-built HealthManager without the wiring keeps the pre-workload-kinds behavior.
		plain := reconciler.NewHealthManager(k8sClient)

		Expect(plain.Check(ctx, "default", clusterName, spec, status)).To(Succeed())

		available := status.GetCondition(v1alpha1.ConditionAvailable)
		Expect(available).NotTo(BeNil())
		Expect(available.Message).To(ContainSubstring("StatefulSet could not be read: web/default"))
	})
})

var _ = Describe("RoleGroupCleaner Deployment teardown", func() {
	var ctx context.Context
	var cleaner *reconciler.RoleGroupCleaner

	BeforeEach(func() {
		ctx = context.Background()
		cleaner = reconciler.NewRoleGroupCleaner(k8sClient, testScheme)
	})

	slotLabels := func(clusterName, groupName string) map[string]string {
		return map[string]string{
			constant.LabelKubernetesInstance:  clusterName,
			constant.LabelKubernetesManagedBy: "operator-go",
			constant.LabelKubernetesComponent: "web",
			constant.LabelKubernetesRoleGroup: groupName,
		}
	}

	ownedBy := func(uid types.UID) []metav1.OwnerReference {
		return []metav1.OwnerReference{{
			APIVersion: "test.zncdata.dev/v1alpha1",
			Kind:       "MockCluster",
			Name:       "owner",
			UID:        uid,
			Controller: ptr.To(true),
		}}
	}

	newOrphanDeployment := func(clusterName string, uid types.UID, replicas int32) *appsv1.Deployment {
		resourceName := reconciler.RoleGroupResourceName(clusterName, "web", "removed")
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:            resourceName,
				Namespace:       cleanerTestNamespace,
				Labels:          slotLabels(clusterName, "removed"),
				OwnerReferences: ownedBy(uid),
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To(replicas),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": resourceName}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": resourceName}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "c", Image: "busybox"}},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, deployment)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, deployment) })
		return deployment
	}

	emptySpec := &v1alpha1.GenericClusterSpec{Roles: map[string]v1alpha1.RoleSpec{}}

	It("should discover an orphaned Deployment from the live cluster and retire it without a drain wait", func() {
		clusterName := "deploy-orphan"
		ownerUID := types.UID("deploy-orphan-uid")
		deployment := newOrphanDeployment(clusterName, ownerUID, 2)
		key := client.ObjectKeyFromObject(deployment)

		// The ledger knows nothing — the Deployment must be found by the live inventory, under
		// the same label + owner + derived-name gates a StatefulSet is.
		status := &v1alpha1.GenericClusterStatus{}

		// Pass 1: the Deployment is scaled to zero and the teardown reports itself pending.
		requeue, err := cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, emptySpec, status, ownerUID, nil)
		Expect(err).To(Succeed())
		Expect(requeue).NotTo(BeZero())

		scaled := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, key, scaled)).To(Succeed())
		Expect(*scaled.Spec.Replicas).To(Equal(int32(0)))

		condition := status.GetCondition(reconciler.ConditionOrphanCleanupPending)
		Expect(condition).NotTo(BeNil())
		Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		Expect(condition.Message).To(ContainSubstring("web/removed"))

		// Report pods still terminating, as a live cluster would between the scale-down and the
		// ReplicaSet finishing. A StatefulSet in this state is waited on (the ordered drain); the
		// Deployment must be deleted anyway — its ReplicaSet retires pods all at once, so there is
		// no order for the cleaner to preserve by waiting.
		scaled.Status.Replicas = 2
		Expect(k8sClient.Status().Update(ctx, scaled)).To(Succeed())

		// Pass 2: deleted without any drain wait.
		_, err = cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, emptySpec, status, ownerUID, nil)
		Expect(err).To(Succeed())

		Expect(k8serrors.IsNotFound(k8sClient.Get(ctx, key, &appsv1.Deployment{}))).To(BeTrue())

		condition = status.GetCondition(reconciler.ConditionOrphanCleanupPending)
		Expect(condition).NotTo(BeNil())
		Expect(condition.Status).To(Equal(metav1.ConditionFalse))
	})

	It("should leave another cluster's Deployment alone", func() {
		clusterName := "deploy-foreign"
		deployment := newOrphanDeployment(clusterName, types.UID("someone-else"), 1)

		status := &v1alpha1.GenericClusterStatus{}
		_, err := cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, emptySpec, status,
			types.UID("deploy-foreign-uid"), nil)
		Expect(err).To(Succeed())

		fetched := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(deployment), fetched)).To(Succeed())
		Expect(*fetched.Spec.Replicas).To(Equal(int32(1)), "a foreign Deployment must not even be scaled down")
	})

	It("should prune the status ledger entry once the Deployment role group is reclaimed", func() {
		clusterName := "deploy-ledger"
		ownerUID := types.UID("deploy-ledger-uid")
		newOrphanDeployment(clusterName, ownerUID, 0)

		// The ledger names the group; the Deployment is already at zero replicas, so the whole
		// teardown completes in one pass plus the confirmation.
		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("web", "removed")

		_, err := cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, emptySpec, status, ownerUID, nil)
		Expect(err).To(Succeed())

		Expect(status.RoleGroups).NotTo(HaveKey("web"))
	})
})
