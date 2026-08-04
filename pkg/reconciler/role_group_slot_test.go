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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/constant"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// slotCRCounter keeps generated cluster names unique and short: the longest name these specs
// derive is "<cr>-<role>-<group>-metrics", which must stay inside the 63-byte DNS label limit.
var slotCRCounter int

func slotCRName(prefix string) string {
	slotCRCounter++
	return fmt.Sprintf("%s-%d", prefix, slotCRCounter)
}

// slotReconciler builds a reconciler over the real BaseRoleGroupHandler, wrapped so a spec can
// post-process what the handler produced. The real handler is used rather than the mock because
// these specs are about labels and names the base handler derives — buildLabels copying
// ClusterLabels in particular, which the mock does not do.
func slotReconciler(
	c client.Client,
	configure func(*reconciler.BaseRoleGroupHandler[*testutil.MockCluster]),
	post func(*reconciler.RoleGroupBuildContext, *reconciler.RoleGroupResources),
) *reconciler.GenericReconciler[*testutil.MockCluster] {
	base := reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster]("product:1", testScheme)
	if configure != nil {
		configure(base)
	}
	handler := &reconciler.RoleGroupHandlerFuncs[*testutil.MockCluster]{
		BuildResourcesFunc: func(ctx context.Context, k8sClient client.Client, cr *testutil.MockCluster,
			buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
			res, err := base.BuildResources(ctx, k8sClient, cr, buildCtx)
			if err != nil {
				return nil, err
			}
			if post != nil {
				post(buildCtx, res)
			}
			return res, nil
		},
	}
	r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
		Client:           c,
		Scheme:           testScheme,
		Recorder:         recorder,
		RoleGroupHandler: handler,
		Prototype:        testutil.NewMockCluster("proto", testNamespace),
	})
	Expect(err).NotTo(HaveOccurred())
	return r
}

// createSlotCR creates a cluster CR with the given roles and registers a best-effort teardown of
// everything the framework derives from it.
func createSlotCR(ctx context.Context, name string, labels map[string]string, roles map[string]v1alpha1.RoleSpec) *testutil.MockCluster {
	cr := testutil.NewMockCluster(name, testNamespace).WithRoles(roles)
	if labels != nil {
		cr.SetLabels(labels)
	}
	Expect(k8sClient.Create(ctx, cr)).To(Succeed())
	DeferCleanup(func() {
		_ = k8sClient.Delete(ctx, cr)
		for roleName, role := range roles {
			for groupName := range role.RoleGroups {
				base := reconciler.RoleGroupResourceName(name, roleName, groupName)
				for _, n := range []string{base, base + "-headless", base + "-metrics"} {
					objMeta := metav1.ObjectMeta{Name: n, Namespace: testNamespace}
					_ = k8sClient.Delete(ctx, &corev1.Service{ObjectMeta: objMeta})
					_ = k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: objMeta})
					_ = k8sClient.Delete(ctx, &policyv1.PodDisruptionBudget{ObjectMeta: objMeta})
				}
			}
		}
	})
	return cr
}

func reconcileSlotCR(ctx context.Context, r *reconciler.GenericReconciler[*testutil.MockCluster], name string) error {
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: name}})
	return err
}

// defaultGroupRole is a role with the single role group these specs use throughout. The group
// name is fixed rather than a parameter: the one spec that needs two groups declares them inline,
// because the names have to collide in a specific way.
func defaultGroupRole() v1alpha1.RoleSpec {
	return v1alpha1.RoleSpec{
		RoleGroups: map[string]v1alpha1.RoleGroupSpec{"default": {Replicas: ptr.To(int32(1))}},
	}
}

// The framework addresses every fixed slot of RoleGroupResources by a name it derives, on both
// paths that remove one. A slot filled under a different name used to be applied, owner-referenced
// and then reclaimed by nothing — surviving every teardown until the cluster CR itself was deleted.
var _ = Describe("Fixed role group slot names", func() {
	var ctx context.Context

	BeforeEach(func() { ctx = context.Background() })

	It("fails the role group when a slot carries a name the framework does not own", func() {
		name := slotCRName("slot-name")
		createSlotCR(ctx, name, nil, map[string]v1alpha1.RoleSpec{"worker": defaultGroupRole()})
		resourceName := reconciler.RoleGroupResourceName(name, "worker", "default")

		r := slotReconciler(k8sClient, nil, func(buildCtx *reconciler.RoleGroupBuildContext, res *reconciler.RoleGroupResources) {
			res.MetricsService = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: buildCtx.ResourceName + "-prom", Namespace: buildCtx.ClusterNamespace},
				Spec:       corev1.ServiceSpec{ClusterIP: corev1.ClusterIPNone, Ports: []corev1.ServicePort{{Name: "m", Port: 9505}}},
			}
		})

		err := reconcileSlotCR(ctx, r, name)
		Expect(err).To(HaveOccurred())
		Expect(reconciler.IsValidationError(err)).To(BeTrue(), "want a *ValidationError, got %T: %v", err, err)
		// Both names have to appear: "wrong name" without saying what the right one is leaves the
		// product author guessing at a convention the framework never wrote down.
		Expect(err.Error()).To(ContainSubstring(resourceName + "-prom"))
		Expect(err.Error()).To(ContainSubstring(resourceName + "-metrics"))
		Expect(err.Error()).To(ContainSubstring("ExtraResources"))

		// The check runs BEFORE anything is applied. Failing at step 7 instead would leave the
		// role group half-converged — a ConfigMap and a StatefulSet for a build that was rejected.
		cm := &corev1.ConfigMap{}
		getErr := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: resourceName}, cm)
		Expect(k8serrors.IsNotFound(getErr)).To(BeTrue(), "no resource may be applied when the declaration is rejected")
	})

	It("fails the role group when a slot is built in another namespace", func() {
		name := slotCRName("slot-ns")
		createSlotCR(ctx, name, nil, map[string]v1alpha1.RoleSpec{"worker": defaultGroupRole()})

		r := slotReconciler(k8sClient, nil, func(buildCtx *reconciler.RoleGroupBuildContext, res *reconciler.RoleGroupResources) {
			res.ConfigMap.Namespace = "kube-system"
		})

		err := reconcileSlotCR(ctx, r, name)
		Expect(err).To(HaveOccurred())
		Expect(reconciler.IsValidationError(err)).To(BeTrue(), "want a *ValidationError, got %T: %v", err, err)
		Expect(err.Error()).To(ContainSubstring("kube-system"))
	})

	It("accepts everything BaseRoleGroupHandler builds, including the suffixed Services", func() {
		name := slotCRName("slot-ok")
		createSlotCR(ctx, name, nil, map[string]v1alpha1.RoleSpec{"worker": defaultGroupRole()})
		resourceName := reconciler.RoleGroupResourceName(name, "worker", "default")

		// Declaring service ports makes the base handler emit the client Service too, so this
		// control covers all four names the handler produces plus a metrics slot at the fifth.
		r := slotReconciler(k8sClient,
			func(b *reconciler.BaseRoleGroupHandler[*testutil.MockCluster]) {
				b.SetRoleServicePorts("worker", []corev1.ServicePort{{Name: "http", Port: 8080, TargetPort: intstr.FromInt32(8080)}})
			},
			func(buildCtx *reconciler.RoleGroupBuildContext, res *reconciler.RoleGroupResources) {
				res.MetricsService = &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{Name: buildCtx.ResourceName + "-metrics", Namespace: buildCtx.ClusterNamespace},
					Spec:       corev1.ServiceSpec{ClusterIP: corev1.ClusterIPNone, Ports: []corev1.ServicePort{{Name: "m", Port: 9505}}},
				}
			})

		Expect(reconcileSlotCR(ctx, r, name)).To(Succeed())
		for _, n := range []string{resourceName, resourceName + "-headless", resourceName + "-metrics"} {
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: n}, &corev1.Service{})).
				To(Succeed(), "Service %q should have been applied", n)
		}
	})
})

// A label on the cluster CR reaches every resource the base handler builds. Three of the
// framework's own label keys SELECT an object for deletion, so a CR carrying one of them would
// make ordinary resources answer to a reclaim aimed at a slot.
var _ = Describe("Reserved framework labels on the cluster CR", func() {
	var ctx context.Context

	BeforeEach(func() { ctx = context.Background() })

	It("does not let the metrics slot label reach a sibling role group's client Service", func() {
		name := slotCRName("resv-metrics")
		// Role group "g" derives its metrics name as "<cr>-w-g-metrics", which is exactly role
		// group "g-metrics"'s own resource name. Only the slot label tells the two apart.
		createSlotCR(ctx, name, map[string]string{reconciler.LabelMetricsService: "true"},
			map[string]v1alpha1.RoleSpec{"w": {RoleGroups: map[string]v1alpha1.RoleGroupSpec{
				"g":         {Replicas: ptr.To(int32(1))},
				"g-metrics": {Replicas: ptr.To(int32(1))},
			}}})
		victim := reconciler.RoleGroupResourceName(name, "w", "g-metrics")
		Expect(victim).To(Equal(reconciler.RoleGroupResourceName(name, "w", "g")+"-metrics"),
			"the spec only bites if the two names really collide")

		r := slotReconciler(k8sClient, func(b *reconciler.BaseRoleGroupHandler[*testutil.MockCluster]) {
			b.SetRoleServicePorts("w", []corev1.ServicePort{{Name: "http", Port: 8080, TargetPort: intstr.FromInt32(8080)}})
		}, nil)

		Expect(reconcileSlotCR(ctx, r, name)).To(Succeed())
		svc := &corev1.Service{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: victim}, svc)).To(Succeed())
		uid := svc.UID

		// The UID is the assertion that bites. Role groups are reconciled in sorted order, so "g"'s
		// reclaim runs before "g-metrics" is applied: a deletion here is immediately papered over by
		// the recreate later in the same pass, and only the churn is observable — a Service whose
		// endpoints and virtual IP are rebuilt every reconcile, which nothing ever reports.
		Expect(reconcileSlotCR(ctx, r, name)).To(Succeed())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: victim}, svc)).
			To(Succeed(), "role group g-metrics' client Service was deleted by role group g's metrics reclaim")
		Expect(svc.UID).To(Equal(uid),
			"role group g-metrics' client Service was deleted and recreated by role group g's metrics reclaim")
		Expect(svc.Labels).NotTo(HaveKey(reconciler.LabelMetricsService))
	})

	It("does not let the role PDB label reach an escape-hatch per-group PDB", func() {
		name := slotCRName("resv-pdb")
		// The value names a role the spec does not declare, which is precisely what
		// cleanupOrphanedRolePDBs treats as "this role is gone, reclaim its PDB".
		createSlotCR(ctx, name, map[string]string{reconciler.LabelRolePodDisruptionBudget: "ghost"},
			map[string]v1alpha1.RoleSpec{"w": defaultGroupRole()})
		resourceName := reconciler.RoleGroupResourceName(name, "w", "default")

		r := slotReconciler(k8sClient, nil, func(buildCtx *reconciler.RoleGroupBuildContext, res *reconciler.RoleGroupResources) {
			res.PodDisruptionBudget = &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      buildCtx.ResourceName,
					Namespace: buildCtx.ClusterNamespace,
					Labels:    buildCtx.ClusterLabels,
				},
				Spec: policyv1.PodDisruptionBudgetSpec{
					MaxUnavailable: ptr.To(intstr.FromInt32(1)),
					Selector:       &metav1.LabelSelector{MatchLabels: map[string]string{"app": "x"}},
				},
			}
		})

		Expect(reconcileSlotCR(ctx, r, name)).To(Succeed())
		Expect(reconcileSlotCR(ctx, r, name)).To(Succeed())

		pdb := &policyv1.PodDisruptionBudget{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: resourceName}, pdb)).
			To(Succeed(), "the per-group PDB was reaped by the role-PDB cleaner")
		Expect(pdb.Labels).NotTo(HaveKey(reconciler.LabelRolePodDisruptionBudget))
	})

	It("still propagates ordinary CR labels, including the restarter opt-in", func() {
		name := slotCRName("resv-pass")
		createSlotCR(ctx, name, map[string]string{
			constant.LabelRestarterEnable:  constant.LabelRestarterEnableValue,
			"example.com/team":             "data",
			reconciler.LabelMetricsService: "true",
		}, map[string]v1alpha1.RoleSpec{"w": defaultGroupRole()})

		var seen map[string]string
		r := slotReconciler(k8sClient, nil, func(buildCtx *reconciler.RoleGroupBuildContext, res *reconciler.RoleGroupResources) {
			seen = buildCtx.ClusterLabels
		})
		Expect(reconcileSlotCR(ctx, r, name)).To(Succeed())

		// The filter is an enumerated set, not a blanket ban on the kubedoop domain: the restarter
		// opt-in is a documented, supported CR label and must survive.
		Expect(seen).To(HaveKeyWithValue(constant.LabelRestarterEnable, constant.LabelRestarterEnableValue))
		Expect(seen).To(HaveKeyWithValue("example.com/team", "data"))
		Expect(seen).NotTo(HaveKey(reconciler.LabelMetricsService))
	})
})

var _ = Describe("Metrics slot reclaim", func() {
	var ctx context.Context

	BeforeEach(func() { ctx = context.Background() })

	It("leaves a sibling ROLE's metrics Service alone when this role ships none", func() {
		name := slotCRName("mx-sibling")
		// Two roles sharing one role group name. RoleGroupMarkerLabelKey is "<cluster>-<group>",
		// with no role in it, so a reclaim selecting on that marker matches both roles' metrics
		// Services — the reason the slot is addressed by its derived name, which does carry the role.
		createSlotCR(ctx, name, nil, map[string]v1alpha1.RoleSpec{
			"alpha": defaultGroupRole(),
			"beta":  defaultGroupRole(),
		})
		betaMetrics := reconciler.RoleGroupResourceName(name, "beta", "default") + "-metrics"

		r := slotReconciler(k8sClient, nil, func(buildCtx *reconciler.RoleGroupBuildContext, res *reconciler.RoleGroupResources) {
			if buildCtx.RoleName != "beta" {
				return // alpha ships no metrics Service: its reclaim branch runs every pass
			}
			res.MetricsService = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: buildCtx.ResourceName + "-metrics", Namespace: buildCtx.ClusterNamespace},
				Spec:       corev1.ServiceSpec{ClusterIP: corev1.ClusterIPNone, Ports: []corev1.ServicePort{{Name: "m", Port: 9505}}},
			}
		})

		Expect(reconcileSlotCR(ctx, r, name)).To(Succeed())
		svc := &corev1.Service{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: betaMetrics}, svc)).To(Succeed())
		uid := svc.UID

		// Deleted-and-recreated is as broken as deleted: a scrape target that churns its
		// endpoints every reconcile loses data without ever looking absent.
		for i := range 2 {
			Expect(reconcileSlotCR(ctx, r, name)).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: betaMetrics}, svc)).
				To(Succeed(), "alpha's reclaim deleted beta's metrics Service on pass %d", i+2)
			Expect(svc.UID).To(Equal(uid), "beta's metrics Service was deleted and recreated on pass %d", i+2)
		}
	})

	It("deletes this role group's metrics Service once the handler stops shipping one", func() {
		name := slotCRName("mx-off")
		createSlotCR(ctx, name, nil, map[string]v1alpha1.RoleSpec{"w": defaultGroupRole()})
		metricsName := reconciler.RoleGroupResourceName(name, "w", "default") + "-metrics"

		emit := true
		r := slotReconciler(k8sClient, nil, func(buildCtx *reconciler.RoleGroupBuildContext, res *reconciler.RoleGroupResources) {
			if !emit {
				return
			}
			res.MetricsService = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: buildCtx.ResourceName + "-metrics", Namespace: buildCtx.ClusterNamespace},
				Spec:       corev1.ServiceSpec{ClusterIP: corev1.ClusterIPNone, Ports: []corev1.ServicePort{{Name: "m", Port: 9505}}},
			}
		})

		Expect(reconcileSlotCR(ctx, r, name)).To(Succeed())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: metricsName}, &corev1.Service{})).To(Succeed())

		emit = false
		Expect(reconcileSlotCR(ctx, r, name)).To(Succeed())
		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: metricsName}, &corev1.Service{})
		Expect(k8serrors.IsNotFound(err)).To(BeTrue(),
			"turning metrics off must remove the Service, or Prometheus keeps a target with no exporter")
	})

	It("backs off instead of degrading the cluster when the reclaim is throttled", func() {
		name := slotCRName("mx-429")
		cr := createSlotCR(ctx, name, nil, map[string]v1alpha1.RoleSpec{"w": defaultGroupRole()})
		metricsName := reconciler.RoleGroupResourceName(name, "w", "default") + "-metrics"

		// The reclaim branch is the one EVERY role group of EVERY product takes, since nothing in
		// the SDK fills the metrics slot. A 429 there used to be reported as a resource-apply
		// failure: the cluster went Degraded and the operator kept hammering a throttled API
		// server, which is the opposite of what a 429 asks for.
		throttled := &throttleServiceGetClient{Client: k8sClient, name: metricsName}
		r := slotReconciler(throttled, nil, nil)

		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: name}})
		Expect(err).NotTo(HaveOccurred(), "throttling is not a reconcile failure")
		Expect(result.RequeueAfter).To(Equal(10*time.Second), "the pass must back off on the rate-limit interval")

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: name}, cr)).To(Succeed())
		degraded := meta.FindStatusCondition(cr.GetStatus().Conditions, string(v1alpha1.ConditionDegraded))
		if degraded != nil {
			Expect(degraded.Status).NotTo(Equal(metav1.ConditionTrue), "a throttled operator is not a broken cluster")
		}
	})
})

// throttleServiceGetClient answers 429 for a Get of one named Service and delegates everything
// else, standing in for an API server that is rate-limiting this controller.
type throttleServiceGetClient struct {
	client.Client
	name string
}

func (c *throttleServiceGetClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, isSvc := obj.(*corev1.Service); isSvc && key.Name == c.name {
		return k8serrors.NewTooManyRequests("slow down", 1)
	}
	return c.Client.Get(ctx, key, obj, opts...)
}
