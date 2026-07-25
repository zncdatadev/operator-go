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
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/sidecar"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// resilienceCRCounter keeps generated CR names unique within a suite run while staying short
// enough that derived resource names ("<cr>-<role>-<group>-metrics") stay under the 63-char
// DNS limit.
var resilienceCRCounter int

// uniqueCRName returns a short, per-spec unique cluster name.
func uniqueCRName(prefix string) string {
	resilienceCRCounter++
	return fmt.Sprintf("%s-%d", prefix, resilienceCRCounter)
}

// newResilienceCR creates a single-role-group cluster CR in the API server and registers its
// teardown, returning the CR and the role group's resource name.
func newResilienceCR(ctx context.Context, name string) (*testutil.MockCluster, string) {
	cr := testutil.NewMockCluster(name, testNamespace).
		WithRoles(map[string]v1alpha1.RoleSpec{
			"broker": {
				RoleGroups: map[string]v1alpha1.RoleGroupSpec{
					"default": {Replicas: ptr.To(int32(1))},
				},
			},
		})
	Expect(k8sClient.Create(ctx, cr)).To(Succeed())
	resourceName := reconciler.RoleGroupResourceName(name, "broker", "default")
	DeferCleanup(func() {
		_ = k8sClient.Delete(ctx, cr)
		meta := metav1.ObjectMeta{Name: resourceName, Namespace: testNamespace}
		_ = k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: meta})
		_ = k8sClient.Delete(ctx, &corev1.Service{ObjectMeta: meta})
		_ = k8sClient.Delete(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: resourceName + "-metrics", Namespace: testNamespace}})
	})
	return cr, resourceName
}

// drainRecorder returns and clears every event buffered by a FakeRecorder.
func drainRecorder(r *record.FakeRecorder) []string {
	var events []string
	for {
		select {
		case e := <-r.Events:
			events = append(events, e)
		default:
			return events
		}
	}
}

var _ = Describe("GenericReconciler panic recovery", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("returns an error (so the workqueue backs off) and emits a Warning event", func() {
		_, _ = newResilienceCR(ctx, uniqueCRName("panic-cr"))
		cr, _ := newResilienceCR(ctx, uniqueCRName("panic-cr"))

		fakeRecorder := record.NewFakeRecorder(100)
		handler := &reconciler.RoleGroupHandlerFuncs[*testutil.ClusterWrapper]{
			BuildResourcesFunc: func(context.Context, client.Client, *testutil.ClusterWrapper, *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
				panic("handler exploded")
			},
		}
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         fakeRecorder,
			RoleGroupHandler: handler,
			Prototype:        testutil.WrapMockCluster(testutil.NewMockCluster("proto", testNamespace)),
		})
		Expect(err).NotTo(HaveOccurred())

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cr.Name}}
		result, err := r.Reconcile(ctx, req)

		// Swallowing the panic would report a successful cycle: no requeue, no backoff, and
		// the cluster silently stops converging.
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("panic in reconciliation"))
		Expect(err.Error()).To(ContainSubstring("handler exploded"))
		Expect(result).To(Equal(ctrl.Result{}))
		Expect(drainRecorder(fakeRecorder)).To(ContainElement(SatisfyAll(
			ContainSubstring("Warning"),
			ContainSubstring("ReconcilePanic"),
		)))
	})
})

var _ = Describe("GenericReconciler requeue aggregation", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	newReconciler := func(cfg *reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]) *reconciler.GenericReconciler[*testutil.ClusterWrapper] {
		cfg.Client = k8sClient
		cfg.Scheme = testScheme
		cfg.Recorder = recorder
		cfg.Prototype = testutil.WrapMockCluster(testutil.NewMockCluster("proto", testNamespace))
		if cfg.RoleGroupHandler == nil {
			cfg.RoleGroupHandler = &handlerAdapter{handler: testutil.NewMockRoleGroupHandler()}
		}
		r, err := reconciler.NewGenericReconciler(cfg)
		Expect(err).NotTo(HaveOccurred())
		return r
	}

	It("requeues on the health check interval", func() {
		cr, _ := newResilienceCR(ctx, uniqueCRName("requeue-health"))
		r := newReconciler(&reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
			HealthCheckInterval: 45 * time.Second,
		})

		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cr.Name}})
		Expect(err).NotTo(HaveOccurred())
		// A ServiceHealthCheck (or any state that produces no watch event) is only
		// re-evaluated because of this requeue.
		Expect(result.RequeueAfter).To(Equal(45 * time.Second))
	})

	It("does not requeue when the health check interval is disabled", func() {
		cr, _ := newResilienceCR(ctx, uniqueCRName("requeue-off"))
		r := newReconciler(&reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
			HealthCheckInterval: -1,
		})

		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cr.Name}})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))
	})

	It("wakes up at the pending gray-delete deadline when it is earlier", func() {
		crName := uniqueCRName("requeue-gray")
		cr, _ := newResilienceCR(ctx, crName)
		orphanName := reconciler.RoleGroupResourceName(crName, "broker", "removed")
		orphan := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: orphanName, Namespace: testNamespace},
		}
		// The cleaner only touches resources it controls, so the leftover ConfigMap carries the
		// same controller reference the reconciler would have set.
		Expect(controllerutil.SetControllerReference(cr, orphan, testScheme)).To(Succeed())
		Expect(k8sClient.Create(ctx, orphan)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, orphan)
		})

		r := newReconciler(&reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
			HealthCheckInterval:   time.Hour,
			GrayDeleteGracePeriod: 30 * time.Second,
		})
		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cr.Name}}

		// First pass tracks the role group that the second pass will find orphaned.
		fetched := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: cr.Name}, fetched)).To(Succeed())
		fetched.Status.SetRoleGroup("broker", "removed")
		Expect(k8sClient.Status().Update(ctx, fetched)).To(Succeed())

		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		// The grace period is shorter than the health interval, so it wins the aggregation —
		// otherwise the deferred deletion would sit until an unrelated event arrives.
		Expect(result.RequeueAfter).To(Equal(30 * time.Second))
	})
})

var _ = Describe("GenericReconciler dependency validation", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("fails the cycle with a Degraded condition while a declared ConfigMap is missing", func() {
		crName := uniqueCRName("dependency-cr")
		cr, resourceName := newResilienceCR(ctx, crName)
		dependencyName := crName + "-external"

		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: &handlerAdapter{handler: testutil.NewMockRoleGroupHandler()},
			Prototype:        testutil.WrapMockCluster(testutil.NewMockCluster("proto", testNamespace)),
			Dependencies: func(*testutil.ClusterWrapper) []reconciler.Dependency {
				return []reconciler.Dependency{{Kind: reconciler.DependencyConfigMap, Name: dependencyName}}
			},
		})
		Expect(err).NotTo(HaveOccurred())

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cr.Name}}
		_, err = r.Reconcile(ctx, req)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("ConfigMapNotFound"))

		// Reconciliation is paused before any resource is built, so the workload is not
		// created against a dependency that does not exist.
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: resourceName}, &corev1.ConfigMap{})).
			To(MatchError(k8serrors.IsNotFound, "IsNotFound"))

		fetched := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, req.NamespacedName, fetched)).To(Succeed())
		degraded := fetched.Status.GetCondition(v1alpha1.ConditionDegraded)
		Expect(degraded).NotTo(BeNil())
		Expect(degraded.Status).To(Equal(metav1.ConditionTrue))

		// Once the dependency exists the cycle proceeds normally.
		dep := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: dependencyName, Namespace: testNamespace}}
		Expect(k8sClient.Create(ctx, dep)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, dep)
		})

		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: resourceName}, &corev1.ConfigMap{})).To(Succeed())
	})
})

// failingSidecarProvider is a sidecar provider whose dependency never resolves.
type failingSidecarProvider struct{}

func (p *failingSidecarProvider) Name() string { return "failing-sidecar" }

func (p *failingSidecarProvider) Inject(*corev1.PodSpec, *sidecar.SidecarConfig) error { return nil }

func (p *failingSidecarProvider) Validate(context.Context, client.Client, string) error {
	return fmt.Errorf("required secret is missing")
}

var _ = Describe("GenericReconciler sidecar validation", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("fails the reconcile before the workload is applied when a provider's dependency is missing", func() {
		crName := uniqueCRName("sidecar-validation")
		cr, resourceName := newResilienceCR(ctx, crName)

		handler := &reconciler.RoleGroupHandlerFuncs[*testutil.ClusterWrapper]{
			BuildResourcesFunc: func(_ context.Context, _ client.Client, _ *testutil.ClusterWrapper, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
				buildCtx.SidecarManager.Register(&failingSidecarProvider{}, &sidecar.SidecarConfig{Enabled: true})
				return &reconciler.RoleGroupResources{
					ConfigMap:   testutil.NewTestConfigMap(buildCtx.ResourceName, buildCtx.ClusterNamespace),
					StatefulSet: testutil.NewTestStatefulSet(buildCtx.ResourceName, buildCtx.ClusterNamespace),
				}, nil
			},
		}
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: handler,
			Prototype:        testutil.WrapMockCluster(testutil.NewMockCluster("proto", testNamespace)),
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cr.Name}})
		Expect(err).To(HaveOccurred())
		Expect(reconciler.IsValidationError(err)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring("required secret is missing"))

		// The ConfigMap is applied before validation (built-in providers point at it), but the
		// workload must not be created with a sidecar that cannot run.
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: resourceName}, &corev1.ConfigMap{})).To(Succeed())
	})
})

// conflictingStatusClient injects a fixed number of 409 Conflict responses into status writes,
// reproducing a CR that another writer touched between the reconciler's Get and its status update.
type conflictingStatusClient struct {
	client.Client
	conflicts int
}

func (c *conflictingStatusClient) Status() client.SubResourceWriter {
	return &conflictingStatusWriter{SubResourceWriter: c.Client.Status(), owner: c}
}

type conflictingStatusWriter struct {
	client.SubResourceWriter
	owner *conflictingStatusClient
}

func (w *conflictingStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	if w.owner.conflicts > 0 {
		w.owner.conflicts--
		return k8serrors.NewConflict(schema.GroupResource{Resource: "mockclusters"}, obj.GetName(),
			fmt.Errorf("simulated conflict"))
	}
	return w.SubResourceWriter.Update(ctx, obj, opts...)
}

var _ = Describe("GenericReconciler status write conflicts", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("retries the status update and still persists this cycle's status", func() {
		crName := uniqueCRName("status-conflict")
		cr, _ := newResilienceCR(ctx, crName)

		conflicting := &conflictingStatusClient{Client: k8sClient, conflicts: 2}
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
			Client:           conflicting,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: &handlerAdapter{handler: testutil.NewMockRoleGroupHandler()},
			Prototype:        testutil.WrapMockCluster(testutil.NewMockCluster("proto", testNamespace)),
		})
		Expect(err).NotTo(HaveOccurred())

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cr.Name}}
		_, err = r.Reconcile(ctx, req)
		// A conflict is not a reconcile failure: the stored object simply moved on, and the
		// computed status must be re-applied on top of it rather than discarded.
		Expect(err).NotTo(HaveOccurred())
		Expect(conflicting.conflicts).To(BeZero())

		fetched := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, req.NamespacedName, fetched)).To(Succeed())
		Expect(fetched.Status.GetCondition(v1alpha1.ConditionReconcileComplete)).NotTo(BeNil())
		Expect(fetched.Status.GetRoleGroups()).To(HaveKeyWithValue("broker", ConsistOf("default")))
	})
})

var _ = Describe("GenericReconciler metrics Service reclaim", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("deletes the metrics Service when the handler stops building it", func() {
		crName := uniqueCRName("metrics-toggle")
		cr, resourceName := newResilienceCR(ctx, crName)
		metricsName := resourceName + "-metrics"

		withMetrics := true
		handler := &reconciler.RoleGroupHandlerFuncs[*testutil.ClusterWrapper]{
			BuildResourcesFunc: func(_ context.Context, _ client.Client, _ *testutil.ClusterWrapper, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
				resources := &reconciler.RoleGroupResources{
					ConfigMap: testutil.NewTestConfigMap(buildCtx.ResourceName, buildCtx.ClusterNamespace),
				}
				if withMetrics {
					resources.MetricsService = testutil.NewTestService(buildCtx.ResourceName+"-metrics", buildCtx.ClusterNamespace)
				}
				return resources, nil
			},
		}
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: handler,
			Prototype:        testutil.WrapMockCluster(testutil.NewMockCluster("proto", testNamespace)),
		})
		Expect(err).NotTo(HaveOccurred())

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cr.Name}}
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: metricsName}, &corev1.Service{})).To(Succeed())

		// Metrics turned off in the CR: the stale Service would otherwise stay a Prometheus
		// target forever, exactly like the reclaimed legacy per-group PDB.
		withMetrics = false
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: metricsName}, &corev1.Service{})).
			To(MatchError(k8serrors.IsNotFound, "IsNotFound"))
	})
})

var _ = Describe("GenericReconciler controller setup", func() {
	It("registers extra watches for product-owned GVKs", func() {
		mgr, err := ctrl.NewManager(testEnv.GetConfig(), ctrl.Options{Scheme: testScheme})
		Expect(err).NotTo(HaveOccurred())

		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: &handlerAdapter{handler: testutil.NewMockRoleGroupHandler()},
			Prototype:        testutil.WrapMockCluster(testutil.NewMockCluster("proto", testNamespace)),
		})
		Expect(err).NotTo(HaveOccurred())

		// ExtraResources have arbitrary GVKs; without this seam their events never reach the
		// controller and out-of-band drift is only repaired by the informer resync.
		Expect(r.SetupWithManagerOpts(mgr, reconciler.SetupWithManagerOptions{
			ExtraOwns: []client.Object{&corev1.Secret{}},
		})).To(Succeed())
	})
})
