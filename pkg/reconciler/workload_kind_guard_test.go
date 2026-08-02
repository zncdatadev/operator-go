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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/config"
	"github.com/zncdatadev/operator-go/pkg/constant"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/sidecar"
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

var guardCRCounter int

func guardCRName(prefix string) string {
	guardCRCounter++
	return fmt.Sprintf("%s-%d", prefix, guardCRCounter)
}

// deploymentBuildCtx returns a build context for a single "web/default" role group, matching the
// shape the reconciler builds.
func deploymentBuildCtx(clusterName string) *reconciler.RoleGroupBuildContext {
	return &reconciler.RoleGroupBuildContext{
		ClusterName:      clusterName,
		ClusterNamespace: testNamespace,
		ClusterSpec: &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				"web": {RoleGroups: map[string]v1alpha1.RoleGroupSpec{"default": {Replicas: ptr.To(int32(2))}}},
			},
		},
		RoleName:      "web",
		RoleSpec:      &v1alpha1.RoleSpec{},
		RoleGroupName: "default",
		RoleGroupSpec: v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(2))},
		MergedConfig:  &config.MergedConfig{},
		ResourceName:  reconciler.RoleGroupResourceName(clusterName, "web", "default"),
	}
}

// The validation surface — sidecar injection, the podOverrides mount check, the main-container
// customizer — lives in one call the Deployment path makes after Build(). Deleting that call left
// the entire suite green, so nothing here was covered: these specs are what make the "validation
// failures are loud" contract true for a Deployment role, not just for a StatefulSet one.
var _ = Describe("Deployment role validation surface", func() {
	var ctx context.Context
	var handler *reconciler.BaseRoleGroupHandler[common.ClusterInterface]
	var cr *testutil.MockCluster
	var buildCtx *reconciler.RoleGroupBuildContext

	BeforeEach(func() {
		ctx = context.Background()
		handler = reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("test-image:latest", testScheme)
		handler.SetRoleWorkloadKind("web", reconciler.WorkloadKindDeployment)
		name := guardCRName("dep-guard")
		cr = testutil.NewMockCluster(name, testNamespace)
		buildCtx = deploymentBuildCtx(name)
	})

	It("rejects a podOverrides volumeMount that displaces the framework's config mount", func() {
		// Strategic merge patch keys volumeMounts by mountPath, so an override at the config path
		// rewrites the framework's entry to point at another volume — a completely valid pod that
		// starts with an empty config directory.
		handler.MainContainerName = "node"
		buildCtx.MergedConfig.PodOverrides = &corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name:         "node",
					VolumeMounts: []corev1.VolumeMount{{Name: "mine", MountPath: constant.KubedoopConfigDirMount}},
				}},
				Volumes: []corev1.Volume{{
					Name:         "mine",
					VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
				}},
			},
		}

		_, err := handler.BuildResources(ctx, k8sClient, cr, buildCtx)
		Expect(err).To(HaveOccurred())
		Expect(reconciler.IsValidationError(err)).To(BeTrue(), "want a *ValidationError, got %T: %v", err, err)
		Expect(err.Error()).To(ContainSubstring("displaced"))
		Expect(err.Error()).To(ContainSubstring(constant.KubedoopConfigDirMount))
		Expect(err.Error()).To(ContainSubstring("podOverrides"))
	})

	It("propagates a MainContainerCustomizer failure", func() {
		buildCtx.MainContainerCustomizer = func(*corev1.Container) error {
			return fmt.Errorf("customizer said no")
		}

		_, err := handler.BuildResources(ctx, k8sClient, cr, buildCtx)
		Expect(err).To(HaveOccurred())
		Expect(reconciler.IsValidationError(err)).To(BeTrue(), "want a *ValidationError, got %T: %v", err, err)
		Expect(err.Error()).To(ContainSubstring("customizer said no"))
	})

	It("injects registered sidecars into the Deployment's pod", func() {
		mgr := sidecar.NewSidecarManager()
		mgr.Register(sidecar.NewJMXExporterSidecarProvider(), &sidecar.SidecarConfig{
			Image:   "jmx:1",
			Enabled: true,
		})
		buildCtx.SidecarManager = mgr

		resources, err := handler.BuildResources(ctx, k8sClient, cr, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.Deployment).NotTo(BeNil())

		// Native sidecars go into InitContainers with RestartPolicy: Always. A Deployment that
		// never ran the injection would carry only the product container.
		init := resources.Deployment.Spec.Template.Spec.InitContainers
		names := make([]string, 0, len(init))
		for _, c := range init {
			names = append(names, c.Name)
		}
		Expect(names).NotTo(BeEmpty(), "no sidecar was injected into the Deployment's pod")
	})
})

// A role group is one workload. Two kinds under one name is rejected as a declaration (both slots
// filled) and as a cluster state (a live workload of the other kind).
var _ = Describe("One workload per role group", func() {
	var ctx context.Context

	BeforeEach(func() { ctx = context.Background() })

	It("rejects a handler that fills both workload slots, before anything is applied", func() {
		name := guardCRName("both-slots")
		createSlotCR(ctx, name, nil, map[string]v1alpha1.RoleSpec{"w": defaultGroupRole()})
		resourceName := reconciler.RoleGroupResourceName(name, "w", "default")

		r := slotReconciler(k8sClient, nil, func(buildCtx *reconciler.RoleGroupBuildContext, res *reconciler.RoleGroupResources) {
			res.Deployment = &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: buildCtx.ResourceName, Namespace: buildCtx.ClusterNamespace},
			}
		})

		err := reconcileSlotCR(ctx, r, name)
		Expect(err).To(HaveOccurred())
		Expect(reconciler.IsValidationError(err)).To(BeTrue(), "want a *ValidationError, got %T: %v", err, err)
		Expect(err.Error()).To(ContainSubstring("exactly one workload"))

		// Pre-flight, not per-slot: a rejected declaration writes nothing at all.
		cmErr := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: resourceName}, &corev1.ConfigMap{})
		Expect(k8serrors.IsNotFound(cmErr)).To(BeTrue(), "no resource may be applied when the declaration is rejected")
	})

	It("rejects a Deployment slot under a name the framework does not own", func() {
		name := guardCRName("dep-name")
		createSlotCR(ctx, name, nil, map[string]v1alpha1.RoleSpec{"w": defaultGroupRole()})
		resourceName := reconciler.RoleGroupResourceName(name, "w", "default")

		r := slotReconciler(k8sClient, nil, func(buildCtx *reconciler.RoleGroupBuildContext, res *reconciler.RoleGroupResources) {
			res.StatefulSet = nil
			res.HeadlessService = nil
			res.Deployment = &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: buildCtx.ResourceName + "-ui", Namespace: buildCtx.ClusterNamespace},
			}
		})

		err := reconcileSlotCR(ctx, r, name)
		Expect(err).To(HaveOccurred())
		Expect(reconciler.IsValidationError(err)).To(BeTrue(), "want a *ValidationError, got %T: %v", err, err)
		// The cleaner's live inventory admits an object only under the derived name, and
		// deleteDeployment addresses that same name: a differently named Deployment would run
		// pods that nothing ever reclaims.
		Expect(err.Error()).To(ContainSubstring(resourceName + "-ui"))
		Expect(err.Error()).To(ContainSubstring(resourceName))
	})

	It("refuses to add a Deployment beside a live StatefulSet of the same name", func() {
		name := guardCRName("kind-switch")
		createSlotCR(ctx, name, nil, map[string]v1alpha1.RoleSpec{"w": defaultGroupRole()})
		resourceName := reconciler.RoleGroupResourceName(name, "w", "default")

		// Pass 1: the role runs as a StatefulSet, the default.
		r := slotReconciler(k8sClient, nil, nil)
		Expect(reconcileSlotCR(ctx, r, name)).To(Succeed())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: resourceName}, &appsv1.StatefulSet{})).To(Succeed())

		// Pass 2: the operator is upgraded and the handler now declares this role a Deployment.
		switched := slotReconciler(k8sClient, nil, func(buildCtx *reconciler.RoleGroupBuildContext, res *reconciler.RoleGroupResources) {
			res.StatefulSet = nil
			res.HeadlessService = nil
			res.Deployment = &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: buildCtx.ResourceName, Namespace: buildCtx.ClusterNamespace},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"a": "b"}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "b"}},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "i"}}},
					},
				},
			}
		})

		err := reconcileSlotCR(ctx, switched, name)
		Expect(err).To(HaveOccurred())
		Expect(reconciler.IsValidationError(err)).To(BeTrue(), "want a *ValidationError, got %T: %v", err, err)
		Expect(err.Error()).To(ContainSubstring("already runs as a StatefulSet"))
		// The remedy has to be actionable, and it has to mention the headless Service: nothing
		// reclaims it once the role stops building one.
		Expect(err.Error()).To(ContainSubstring("kubectl delete statefulset/" + resourceName))
		Expect(err.Error()).To(ContainSubstring(resourceName + "-headless"))

		// Nothing was created: the whole point is that two workloads never run at once.
		depErr := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: resourceName}, &appsv1.Deployment{})
		Expect(k8serrors.IsNotFound(depErr)).To(BeTrue(), "the Deployment was applied beside the live StatefulSet")
		// And the StatefulSet is untouched — detection must never be destructive.
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: resourceName}, &appsv1.StatefulSet{})).To(Succeed())
	})

	It("ignores a foreign object of the other kind that this cluster does not own", func() {
		name := guardCRName("foreign")
		createSlotCR(ctx, name, nil, map[string]v1alpha1.RoleSpec{"w": defaultGroupRole()})
		resourceName := reconciler.RoleGroupResourceName(name, "w", "default")

		// Somebody else's Deployment happens to carry this role group's derived name. It is not
		// ours to complain about, and blocking on it would wedge the cluster for good.
		foreign := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: testNamespace},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foreign": "yes"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foreign": "yes"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "i"}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, foreign)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, foreign) })

		r := slotReconciler(k8sClient, nil, nil)
		Expect(reconcileSlotCR(ctx, r, name)).To(Succeed())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: resourceName}, &appsv1.StatefulSet{})).To(Succeed())
	})
})

// The health manager reads whichever kind the handler declares. Without the provider reaching it,
// a healthy Deployment cluster reports Degraded forever, naming a StatefulSet that never existed.
var _ = Describe("Deployment health wiring", func() {
	var ctx context.Context

	BeforeEach(func() { ctx = context.Background() })

	It("does not report a Deployment role group as an unreadable StatefulSet", func() {
		name := guardCRName("dep-health")
		cr := createSlotCR(ctx, name, nil, map[string]v1alpha1.RoleSpec{"w": defaultGroupRole()})

		base := reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster]("product:1", testScheme)
		base.SetRoleWorkloadKind("w", reconciler.WorkloadKindDeployment)
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: base,
			Prototype:        testutil.NewMockCluster("proto", testNamespace),
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: name}})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: name}, cr)).To(Succeed())
		for _, cond := range cr.GetStatus().Conditions {
			Expect(cond.Message).NotTo(ContainSubstring("StatefulSet"),
				"health read the wrong kind: WorkloadKindProvider did not reach the health manager")
		}
	})
})

// Referenced to keep the client import honest across build tags.
var _ client.Object = &appsv1.Deployment{}
