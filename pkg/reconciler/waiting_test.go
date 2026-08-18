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
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// waitingExtension returns whatever its hook func returns; nil means "nothing to wait for".
type waitingExtension struct {
	name string
	pre  func() error
}

func (e *waitingExtension) Name() string { return e.name }
func (e *waitingExtension) PreReconcile(context.Context, client.Client, *testutil.MockCluster) error {
	if e.pre == nil {
		return nil
	}
	return e.pre()
}
func (e *waitingExtension) PostReconcile(context.Context, client.Client, *testutil.MockCluster) error {
	return nil
}
func (e *waitingExtension) OnReconcileError(context.Context, client.Client, *testutil.MockCluster, error) error {
	return nil
}

var _ = Describe("An extension that is waiting", func() {
	var ctx context.Context

	BeforeEach(func() { ctx = context.Background() })

	newCR := func(name string) *testutil.MockCluster {
		cr := testutil.NewMockCluster(name, testNamespace).
			WithRoles(map[string]v1alpha1.RoleSpec{"worker": defaultGroupRole()})
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, cr)
			// envtest runs no garbage collector, so owner references reclaim nothing: every object
			// a spec creates has to be deleted by name or it leaks into the next one.
			base := reconciler.RoleGroupResourceName(name, "worker", "default")
			for _, n := range []string{base, base + "-headless"} {
				meta := metav1.ObjectMeta{Name: n, Namespace: testNamespace}
				_ = k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: meta})
				_ = k8sClient.Delete(ctx, &corev1.Service{ObjectMeta: meta})
				_ = k8sClient.Delete(ctx, &appsv1.StatefulSet{ObjectMeta: meta})
			}
		})
		return cr
	}

	reconcileWith := func(crName string, ext *waitingExtension, rec record.EventRecorder) (ctrl.Result, error) {
		registry := common.NewExtensionRegistry[*testutil.MockCluster]()
		registry.RegisterClusterExtension(ext)
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:            k8sClient,
			Scheme:            testScheme,
			ImageResolution:   reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:          rec,
			RoleGroupHandler:  testutil.NewMockRoleGroupHandler(),
			ExtensionRegistry: registry,
			Prototype:         testutil.NewMockCluster("proto", testNamespace),
		})
		Expect(err).NotTo(HaveOccurred())
		return r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: crName}})
	}

	It("is not a failure: no Degraded, no Warning event, and it requeues on its own delay", func() {
		// The whole complaint in #608: a cluster that is merely waiting for a database migration
		// pages, because the only channel a hook had was an error.
		crName := uniqueCRName("waiting-basic")
		cr := newCR(crName)
		fakeRecorder := record.NewFakeRecorder(100)

		result, err := reconcileWith(cr.Name, &waitingExtension{
			name: "db-init",
			pre: func() error {
				return common.NewRequeueAfterError(15*time.Second, "WaitingForDBInit", "database migration Job has not completed")
			},
		}, fakeRecorder)

		Expect(err).NotTo(HaveOccurred(), "a wait must not be returned as an error")
		Expect(result.RequeueAfter).To(Equal(15 * time.Second))

		fetched := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: cr.Name}, fetched)).To(Succeed())
		Expect(fetched.Status.IsDegraded()).To(BeFalse())

		waiting := fetched.Status.GetCondition(reconciler.ConditionWaiting)
		Expect(waiting).NotTo(BeNil(), "the wait has to be visible somewhere")
		Expect(waiting.Status).To(Equal(metav1.ConditionTrue))
		Expect(waiting.Reason).To(Equal("WaitingForDBInit"))
		Expect(waiting.Message).To(ContainSubstring("database migration Job"))

		// The documented generation gate must not report a spec edit as applied while the
		// framework is still waiting to apply it.
		complete := fetched.Status.GetCondition(v1alpha1.ConditionReconcileComplete)
		Expect(complete).NotTo(BeNil())
		Expect(complete.Status).To(Equal(metav1.ConditionFalse))
		Expect(complete.Reason).To(Equal(reconciler.ReasonWaiting))

		Expect(drainRecorder(fakeRecorder)).NotTo(ContainElement(ContainSubstring("ReconcileError")))
	})

	It("still runs health, so Degraded cannot latch at a fault that has healed", func() {
		// The reason the wait is reported at the END of the pass rather than by returning where it
		// was raised. SetDegraded(false, ...) has exactly one writer — HealthManager.Check — so an
		// early return leaves Degraded frozen at whatever the last completed pass wrote. A cluster
		// whose pods recovered while an extension waits would keep paging with a message naming a
		// pod that no longer exists, and observedGeneration would match, so no staleness heuristic
		// catches it.
		crName := uniqueCRName("waiting-latch")
		cr := newCR(crName)

		// Pass 1: a real failure sets Degraded.
		_, err := reconcileWith(cr.Name, &waitingExtension{
			name: "boom",
			pre:  func() error { return errors.New("catalog backend is unreachable") },
		}, record.NewFakeRecorder(100))
		Expect(err).To(HaveOccurred())

		fetched := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: cr.Name}, fetched)).To(Succeed())
		Expect(fetched.Status.IsDegraded()).To(BeTrue(), "precondition: the cluster is degraded")

		// Pass 2: the failure is gone and the extension is merely waiting.
		_, err = reconcileWith(cr.Name, &waitingExtension{
			name: "boom",
			pre: func() error {
				return common.NewRequeueAfterError(30*time.Second, "WaitingForCatalog", "catalog is initialising")
			},
		}, record.NewFakeRecorder(100))
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: cr.Name}, fetched)).To(Succeed())
		Expect(fetched.Status.IsDegraded()).To(BeFalse(),
			"health ran on the waiting pass, so the healed fault was cleared instead of latching")
	})

	It("blocks the roles while a cluster-level hook waits, but still observes", func() {
		// #608's case is a schema migration that must finish BEFORE any workload starts; applying
		// the StatefulSets anyway would run the product against an uninitialised database.
		crName := uniqueCRName("waiting-blocks")
		cr := newCR(crName)
		resourceName := reconciler.RoleGroupResourceName(cr.Name, "worker", "default")

		_, err := reconcileWith(cr.Name, &waitingExtension{
			name: "db-init",
			pre: func() error {
				return common.NewRequeueAfterError(20*time.Second, "WaitingForDBInit", "migration Job has not completed")
			},
		}, record.NewFakeRecorder(100))
		Expect(err).NotTo(HaveOccurred())

		sts := &appsv1.StatefulSet{}
		getErr := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: resourceName}, sts)
		Expect(getErr).To(HaveOccurred(), "no workload may be applied while a cluster-level hook is waiting")

		// But the pass still ran to the end: health observed the cluster rather than freezing it.
		fetched := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: cr.Name}, fetched)).To(Succeed())
		Expect(fetched.Status.GetCondition(v1alpha1.ConditionAvailable)).NotTo(BeNil(),
			"health still ran, so Available reflects this pass rather than a stale one")
	})

	It("clears the Waiting condition once nothing is waiting", func() {
		crName := uniqueCRName("waiting-cleared")
		cr := newCR(crName)

		_, err := reconcileWith(cr.Name, &waitingExtension{
			name: "gate",
			pre: func() error {
				return common.NewRequeueAfterError(time.Minute, "WaitingForPeer", "peer cluster is not ready")
			},
		}, record.NewFakeRecorder(100))
		Expect(err).NotTo(HaveOccurred())

		_, err = reconcileWith(cr.Name, &waitingExtension{name: "gate"}, record.NewFakeRecorder(100))
		Expect(err).NotTo(HaveOccurred())

		fetched := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: cr.Name}, fetched)).To(Succeed())
		waiting := fetched.Status.GetCondition(reconciler.ConditionWaiting)
		Expect(waiting).NotTo(BeNil())
		Expect(waiting.Status).To(Equal(metav1.ConditionFalse))
		Expect(fetched.Status.GetCondition(v1alpha1.ConditionReconcileComplete).Status).
			To(Equal(metav1.ConditionTrue))
	})

	It("never raises the condition for a product that does not wait", func() {
		// The same rule ConditionOrphanCleanupPending follows: only clear what was raised, so a
		// product that never waits carries no extra condition at all.
		crName := uniqueCRName("waiting-none")
		cr := newCR(crName)

		_, err := reconcileWith(cr.Name, &waitingExtension{name: "quiet"}, record.NewFakeRecorder(100))
		Expect(err).NotTo(HaveOccurred())

		fetched := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: cr.Name}, fetched)).To(Succeed())
		Expect(fetched.Status.GetCondition(reconciler.ConditionWaiting)).To(BeNil())
	})

	It("still fails the cluster when a genuine error accompanies the wait", func() {
		// A wait must never launder a real failure. The hook returns both, joined.
		crName := uniqueCRName("waiting-mixed")
		cr := newCR(crName)

		_, err := reconcileWith(cr.Name, &waitingExtension{
			name: "mixed",
			pre: func() error {
				return errors.Join(
					common.NewRequeueAfterError(time.Minute, "WaitingForPeer", "peer is not ready"),
					errors.New("catalog config is invalid"),
				)
			},
		}, record.NewFakeRecorder(100))
		Expect(err).To(HaveOccurred(), "a real failure in the aggregate outranks the wait")

		fetched := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: cr.Name}, fetched)).To(Succeed())
		Expect(fetched.Status.IsDegraded()).To(BeTrue())
	})
})

var _ = Describe("A RoleProvider can say 'not ready yet' too", func() {
	// RoleProvider's contract says a *common.RequeueAfterError reports "not ready yet" — a product
	// whose declaration depends on an authentication class or an S3 connection that has not been
	// issued. Returning it raw from the catalog step made that a lie: Reconcile left early WITH an
	// error, so the workqueue backed off exponentially, no Waiting condition was raised, and
	// Degraded stayed latched at whatever the last completed pass wrote — cleanup and health never
	// ran. That is precisely the shape §5 says a wait must not have.
	var ctx context.Context

	BeforeEach(func() { ctx = context.Background() })

	It("requeues on its own delay without Degraded, and still observes the cluster", func() {
		crName := uniqueCRName("waiting-roleprovider")
		cr := testutil.NewMockCluster(crName, testNamespace).
			WithRoles(map[string]v1alpha1.RoleSpec{"worker": defaultGroupRole()})
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cr) })

		fakeRecorder := record.NewFakeRecorder(100)
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:         fakeRecorder,
			RoleGroupHandler: testutil.NewMockRoleGroupHandler(),
			RoleProvider: reconciler.RoleProviderFunc[*testutil.MockCluster](
				func(context.Context, client.Client, *testutil.MockCluster) (reconciler.RoleCatalog, error) {
					return nil, common.NewRequeueAfterError(25*time.Second,
						"WaitingForAuthClass", "the authentication class has not been issued")
				}),
			Prototype: testutil.NewMockCluster("proto", testNamespace),
		})
		Expect(err).NotTo(HaveOccurred())

		result, err := r.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: crName},
		})
		Expect(err).NotTo(HaveOccurred(), "a wait must not be returned as an error")
		Expect(result.RequeueAfter).To(Equal(25 * time.Second))

		fetched := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: crName}, fetched)).To(Succeed())
		Expect(fetched.Status.IsDegraded()).To(BeFalse(), "waiting on a declaration is not a fault")

		waiting := fetched.Status.GetCondition(reconciler.ConditionWaiting)
		Expect(waiting).NotTo(BeNil())
		Expect(waiting.Reason).To(Equal("WaitingForAuthClass"))

		// The pass must have continued past the catalog: health is what writes these, and an early
		// return is exactly what used to leave them stale.
		Expect(fetched.Status.GetCondition(v1alpha1.ConditionAvailable)).NotTo(BeNil(),
			"cleanup and health still run while a declaration waits")
	})

	It("still fails the pass for an ordinary catalog error", func() {
		crName := uniqueCRName("failing-roleprovider")
		cr := testutil.NewMockCluster(crName, testNamespace).
			WithRoles(map[string]v1alpha1.RoleSpec{"worker": defaultGroupRole()})
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cr) })

		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:         record.NewFakeRecorder(100),
			RoleGroupHandler: testutil.NewMockRoleGroupHandler(),
			RoleProvider: reconciler.RoleProviderFunc[*testutil.MockCluster](
				func(context.Context, client.Client, *testutil.MockCluster) (reconciler.RoleCatalog, error) {
					return nil, errors.New("cannot reach the metastore")
				}),
			Prototype: testutil.NewMockCluster("proto", testNamespace),
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = r.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: crName},
		})
		Expect(err).To(HaveOccurred(), "a real failure must still back the workqueue off")
	})
})
