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
	"maps"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/constant"
	"github.com/zncdatadev/operator-go/pkg/productlogging"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/sidecar"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	"github.com/zncdatadev/operator-go/pkg/vector"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
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
		handler := &reconciler.RoleGroupHandlerFuncs[*testutil.MockCluster]{
			BuildResourcesFunc: func(context.Context, client.Client, *testutil.MockCluster, *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
				panic("handler exploded")
			},
		}
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         fakeRecorder,
			RoleGroupHandler: handler,
			Prototype:        testutil.NewMockCluster("proto", testNamespace),
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

	newReconciler := func(cfg *reconciler.GenericReconcilerConfig[*testutil.MockCluster]) *reconciler.GenericReconciler[*testutil.MockCluster] {
		cfg.Client = k8sClient
		cfg.Scheme = testScheme
		cfg.Recorder = recorder
		cfg.Prototype = testutil.NewMockCluster("proto", testNamespace)
		if cfg.RoleGroupHandler == nil {
			cfg.RoleGroupHandler = &handlerAdapter{handler: testutil.NewMockRoleGroupHandler()}
		}
		r, err := reconciler.NewGenericReconciler(cfg)
		Expect(err).NotTo(HaveOccurred())
		return r
	}

	It("requeues on the health check interval", func() {
		cr, _ := newResilienceCR(ctx, uniqueCRName("requeue-health"))
		r := newReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
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
		r := newReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
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

		r := newReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
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

		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: &handlerAdapter{handler: testutil.NewMockRoleGroupHandler()},
			Prototype:        testutil.NewMockCluster("proto", testNamespace),
			Dependencies: func(*testutil.MockCluster) []reconciler.Dependency {
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

		handler := &reconciler.RoleGroupHandlerFuncs[*testutil.MockCluster]{
			BuildResourcesFunc: func(_ context.Context, _ client.Client, _ *testutil.MockCluster, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
				buildCtx.SidecarManager.Register(&failingSidecarProvider{}, &sidecar.SidecarConfig{Enabled: true})
				return &reconciler.RoleGroupResources{
					ConfigMap:   testutil.NewTestConfigMap(buildCtx.ResourceName, buildCtx.ClusterNamespace),
					StatefulSet: testutil.NewTestStatefulSet(buildCtx.ResourceName, buildCtx.ClusterNamespace),
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
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           conflicting,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: &handlerAdapter{handler: testutil.NewMockRoleGroupHandler()},
			Prototype:        testutil.NewMockCluster("proto", testNamespace),
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
		handler := &reconciler.RoleGroupHandlerFuncs[*testutil.MockCluster]{
			BuildResourcesFunc: func(_ context.Context, _ client.Client, _ *testutil.MockCluster, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
				resources := &reconciler.RoleGroupResources{
					ConfigMap: testutil.NewTestConfigMap(buildCtx.ResourceName, buildCtx.ClusterNamespace),
				}
				if withMetrics {
					resources.MetricsService = testutil.NewTestService(buildCtx.ResourceName+"-metrics", buildCtx.ClusterNamespace)
				}
				return resources, nil
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

// secretExtraHandler emits a Secret as an ExtraResource and counts how often it was asked to
// build, so a spec can tell whether an event actually reached the controller.
type secretExtraHandler struct {
	mu     sync.Mutex
	builds int
	token  string
}

func (h *secretExtraHandler) BuildResources(
	_ context.Context,
	_ client.Client,
	_ *testutil.MockCluster,
	buildCtx *reconciler.RoleGroupBuildContext,
) (*reconciler.RoleGroupResources, error) {
	h.mu.Lock()
	h.builds++
	h.mu.Unlock()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: buildCtx.ClusterName + "-extra", Namespace: buildCtx.ClusterNamespace},
		Data:       map[string][]byte{"token": []byte(h.token)},
	}
	return &reconciler.RoleGroupResources{
		ConfigMap:      testutil.NewTestConfigMap(buildCtx.ResourceName, buildCtx.ClusterNamespace),
		ExtraResources: []client.Object{secret},
	}, nil
}

func (h *secretExtraHandler) buildCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.builds
}

var _ = Describe("GenericReconciler controller setup", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("reconciles when an owned object of an extra GVK is changed out of band", func() {
		crName := uniqueCRName("extra-owns")
		// A dedicated namespace keeps this controller — the only one in the suite that runs
		// against a live manager — from reconciling the CRs of the other specs.
		namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: crName}}
		Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

		cr := testutil.NewMockCluster(crName, namespace.Name).
			WithRoles(map[string]v1alpha1.RoleSpec{
				"broker": {RoleGroups: map[string]v1alpha1.RoleGroupSpec{"default": {Replicas: ptr.To(int32(1))}}},
			})
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, cr)
		})

		handler := &secretExtraHandler{token: "desired"}
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: handler,
			// No periodic requeue: the repair below can only be triggered by a watch event.
			HealthCheckInterval: -1,
			Prototype:           testutil.NewMockCluster("proto", namespace.Name),
		})
		Expect(err).NotTo(HaveOccurred())

		mgr, err := ctrl.NewManager(testEnv.GetConfig(), ctrl.Options{
			Scheme:  testScheme,
			Metrics: metricsserver.Options{BindAddress: "0"},
			Cache:   cache.Options{DefaultNamespaces: map[string]cache.Config{namespace.Name: {}}},
		})
		Expect(err).NotTo(HaveOccurred())

		// ExtraResources have arbitrary GVKs; without this seam their events never reach the
		// controller and out-of-band drift is only repaired by the informer resync.
		Expect(r.SetupWithManagerOpts(mgr, reconciler.SetupWithManagerOptions{
			ExtraOwns: []client.Object{&corev1.Secret{}},
		})).To(Succeed())

		mgrCtx, stopManager := context.WithCancel(ctx)
		DeferCleanup(stopManager)
		go func() {
			defer GinkgoRecover()
			Expect(mgr.Start(mgrCtx)).To(Succeed())
		}()
		Expect(mgr.GetCache().WaitForCacheSync(mgrCtx)).To(BeTrue())

		secretKey := types.NamespacedName{Namespace: namespace.Name, Name: crName + "-extra"}
		Eventually(func() error {
			return k8sClient.Get(ctx, secretKey, &corev1.Secret{})
		}, "30s", "200ms").Should(Succeed())

		// Wait until the cluster settles: two consecutive samples with the same build count mean
		// nothing else is scheduling reconciles any more.
		settled := -1
		Eventually(func() bool {
			current := handler.buildCount()
			quiet := current == settled
			settled = current
			return quiet
		}, "30s", "500ms").Should(BeTrue())
		before := handler.buildCount()

		live := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, secretKey, live)).To(Succeed())
		live.Data = map[string][]byte{"token": []byte("tampered")}
		Expect(k8sClient.Update(ctx, live)).To(Succeed())

		Eventually(func() (string, error) {
			current := &corev1.Secret{}
			if err := k8sClient.Get(ctx, secretKey, current); err != nil {
				return "", err
			}
			return string(current.Data["token"]), nil
		}, "30s", "200ms").Should(Equal("desired"))
		Expect(handler.buildCount()).To(BeNumerically(">", before))
	})
})

// throttlingCreateClient answers every Create with a 429, as the API server does when the
// operator exceeds its request budget.
type throttlingCreateClient struct {
	client.Client
}

func (c *throttlingCreateClient) Create(_ context.Context, _ client.Object, _ ...client.CreateOption) error {
	return k8serrors.NewTooManyRequests("slow down", 1)
}

var _ = Describe("GenericReconciler rate limiting", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("backs off on a 429 instead of reporting the cluster as degraded", func() {
		crName := uniqueCRName("throttled")
		cr, resourceName := newResilienceCR(ctx, crName)

		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:              &throttlingCreateClient{Client: k8sClient},
			Scheme:              testScheme,
			Recorder:            recorder,
			RoleGroupHandler:    &handlerAdapter{handler: testutil.NewMockRoleGroupHandler()},
			RateLimitRetryAfter: 3 * time.Second,
			Prototype:           testutil.NewMockCluster("proto", testNamespace),
		})
		Expect(err).NotTo(HaveOccurred())

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cr.Name}}
		result, err := r.Reconcile(ctx, req)

		// Throttling is the API server asking for a pause, not a broken cluster: returning the
		// error would run the product's error hooks, emit a Warning and flip Degraded on a cluster
		// whose state nobody could even read.
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(3 * time.Second))

		fetched := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, req.NamespacedName, fetched)).To(Succeed())
		Expect(fetched.Status.GetCondition(v1alpha1.ConditionDegraded)).To(BeNil())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: resourceName}, &corev1.ConfigMap{})).
			To(MatchError(k8serrors.IsNotFound, "IsNotFound"))
	})
})

// vectorTestHandler builds the role group resources by hand and declares a log producer, which is
// what makes the framework consider injecting the Vector sidecar. providesVectorConfig switches
// the VectorConfigProvider answer: false is a product that neither generates vector.yaml itself
// nor exposes a Vector aggregator on its CR.
type vectorTestHandler struct {
	providesVectorConfig bool
	configMapData        map[string]string
}

func (h *vectorTestHandler) LoggingProducers(string) []productlogging.ContainerLogging {
	return []productlogging.ContainerLogging{{Container: "app"}}
}

func (h *vectorTestHandler) LogVolumeSizeLimit() string { return "" }

func (h *vectorTestHandler) ProvidesVectorConfig(string) bool { return h.providesVectorConfig }

func (h *vectorTestHandler) BuildResources(
	_ context.Context,
	_ client.Client,
	_ *testutil.MockCluster,
	buildCtx *reconciler.RoleGroupBuildContext,
) (*reconciler.RoleGroupResources, error) {
	sts := testutil.NewTestStatefulSet(buildCtx.ResourceName, buildCtx.ClusterNamespace)
	sts.Spec.Template.Spec.Containers[0].Name = "app"

	if buildCtx.SidecarManager.HasSidecars() {
		if err := buildCtx.SidecarManager.SetProductImage("test-image:latest", corev1.PullIfNotPresent); err != nil {
			return nil, err
		}
		if err := buildCtx.SidecarManager.InjectAll(&sts.Spec.Template.Spec); err != nil {
			return nil, err
		}
	}

	return &reconciler.RoleGroupResources{
		ConfigMap: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: buildCtx.ResourceName, Namespace: buildCtx.ClusterNamespace},
			Data:       h.configMapData,
		},
		StatefulSet: sts,
	}, nil
}

// hasVectorSidecar reports whether the Vector sidecar was injected (it is a native sidecar, i.e.
// an init container with restartPolicy Always).
func hasVectorSidecar(sts *appsv1.StatefulSet) bool {
	for _, c := range sts.Spec.Template.Spec.InitContainers {
		if c.Name == vector.VectorSidecarName {
			return true
		}
	}
	for _, c := range sts.Spec.Template.Spec.Containers {
		if c.Name == vector.VectorSidecarName {
			return true
		}
	}
	return false
}

var _ = Describe("GenericReconciler vector sidecar gating", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	// newVectorCR creates a cluster whose single role group enables the Vector agent.
	newVectorCR := func(name string) (*testutil.MockCluster, string) {
		cr := testutil.NewMockCluster(name, testNamespace).
			WithRoles(map[string]v1alpha1.RoleSpec{
				"broker": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {
							Replicas: ptr.To(int32(1)),
							Config: &v1alpha1.RoleGroupConfigSpec{
								Logging: &v1alpha1.LoggingSpec{EnableVectorAgent: ptr.To(true)},
							},
						},
					},
				},
			})
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		resourceName := reconciler.RoleGroupResourceName(name, "broker", "default")
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, cr)
			meta := metav1.ObjectMeta{Name: resourceName, Namespace: testNamespace}
			_ = k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: meta})
			_ = k8sClient.Delete(ctx, &appsv1.StatefulSet{ObjectMeta: meta})
		})
		return cr, resourceName
	}

	newReconciler := func(handler reconciler.RoleGroupHandler[*testutil.MockCluster], rec record.EventRecorder) *reconciler.GenericReconciler[*testutil.MockCluster] {
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         rec,
			RoleGroupHandler: handler,
			Prototype:        testutil.NewMockCluster("proto", testNamespace),
		})
		Expect(err).NotTo(HaveOccurred())
		return r
	}

	It("skips the sidecar when nothing supplies vector.yaml", func() {
		crName := uniqueCRName("vector-unwired")
		cr, resourceName := newVectorCR(crName)

		fakeRecorder := record.NewFakeRecorder(100)
		r := newReconciler(&vectorTestHandler{}, fakeRecorder)

		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cr.Name}})
		// The CR does not implement VectorAggregatorProvider and the handler does not write
		// vector.yaml, so injecting the sidecar would fail its own dependency validation and abort
		// the whole cluster's reconcile, every cycle, over a misconfigured logging flag.
		Expect(err).NotTo(HaveOccurred())

		sts := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: resourceName}, sts)).To(Succeed())
		Expect(hasVectorSidecar(sts)).To(BeFalse())
		Expect(drainRecorder(fakeRecorder)).To(ContainElement(SatisfyAll(
			ContainSubstring("Warning"),
			ContainSubstring("VectorSidecarSkipped"),
		)))
	})

	It("injects the sidecar when the handler supplies vector.yaml itself", func() {
		crName := uniqueCRName("vector-product")
		cr, resourceName := newVectorCR(crName)

		handler := &vectorTestHandler{
			providesVectorConfig: true,
			configMapData:        map[string]string{vector.VectorConfigFileName: "# product-owned vector config"},
		}
		r := newReconciler(handler, record.NewFakeRecorder(100))

		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cr.Name}})
		Expect(err).NotTo(HaveOccurred())

		// Gating on the CR interface alone would silently drop the sidecar from every product that
		// builds its own vector.yaml — a supported case.
		sts := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: resourceName}, sts)).To(Succeed())
		Expect(hasVectorSidecar(sts)).To(BeTrue())
	})

	It("omits the rolling file appender when the sidecar is skipped", func() {
		crName := uniqueCRName("vector-appender")
		cr, resourceName := newVectorCR(crName)
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName + "-headless", Namespace: testNamespace}})
		})

		handler := reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster]("img:1", testScheme)
		handler.MainContainerName = "app"
		handler.LoggingContainers = []productlogging.ContainerLogging{
			{Container: "app", Framework: productlogging.LoggingFrameworkLogback},
		}
		r := newReconciler(handler, record.NewFakeRecorder(100))

		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cr.Name}})
		Expect(err).NotTo(HaveOccurred())

		sts := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: resourceName}, sts)).To(Succeed())
		Expect(hasVectorSidecar(sts)).To(BeFalse())

		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: resourceName}, cm)).To(Succeed())
		Expect(cm.Data).To(HaveKey("logback.xml"))
		// The Vector provider is the sole owner of the shared log emptyDir and its mounts, so a
		// file appender emitted without it points the product at a path no volume backs.
		Expect(cm.Data["logback.xml"]).NotTo(ContainSubstring(constant.KubedoopLogDir))
	})
})

var _ = Describe("GenericReconciler status stability", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("reports the same Degraded message on every pass when several role groups fail", func() {
		crName := uniqueCRName("degraded-order")
		groups := map[string]v1alpha1.RoleGroupSpec{}
		for i := 1; i <= 8; i++ {
			groups[fmt.Sprintf("group-%d", i)] = v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(1))}
		}
		cr := testutil.NewMockCluster(crName, testNamespace).
			WithRoles(map[string]v1alpha1.RoleSpec{"broker": {RoleGroups: groups}})
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, cr)
		})

		handler := &reconciler.RoleGroupHandlerFuncs[*testutil.MockCluster]{
			BuildResourcesFunc: func(_ context.Context, _ client.Client, _ *testutil.MockCluster, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
				return nil, fmt.Errorf("cannot build role group %s", buildCtx.RoleGroupName)
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

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: crName}}
		var messages []string
		for i := 0; i < 6; i++ {
			_, reconcileErr := r.Reconcile(ctx, req)
			Expect(reconcileErr).To(HaveOccurred())

			fetched := &testutil.MockCluster{}
			Expect(k8sClient.Get(ctx, req.NamespacedName, fetched)).To(Succeed())
			degraded := fetched.Status.GetCondition(v1alpha1.ConditionDegraded)
			Expect(degraded).NotTo(BeNil())
			messages = append(messages, degraded.Message)
		}

		// The first failing group aborts the role and its error becomes the Degraded message. In
		// map order a different group wins every cycle, so the status never settles and each write
		// schedules the next reconcile.
		Expect(messages).To(HaveEach(messages[0]))
		Expect(messages[0]).To(ContainSubstring("group-1"))
	})

	It("stops emitting Updated events once the cluster has settled", func() {
		crName := uniqueCRName("settled-events")
		cr, _ := newResilienceCR(ctx, crName)

		fakeRecorder := record.NewFakeRecorder(100)
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         fakeRecorder,
			RoleGroupHandler: &handlerAdapter{handler: testutil.NewMockRoleGroupHandler()},
			Prototype:        testutil.NewMockCluster("proto", testNamespace),
		})
		Expect(err).NotTo(HaveOccurred())

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cr.Name}}
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(drainRecorder(fakeRecorder)).To(ContainElement(ContainSubstring("Created")))

		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		// Handler-built objects omit server-defaulted fields, so CreateOrUpdate reports "Updated"
		// on every pass even when the API server stores exactly what it already had. Trusting that
		// verdict makes a settled cluster emit a Normal event per resource, forever.
		Expect(drainRecorder(fakeRecorder)).NotTo(ContainElement(ContainSubstring("Updated")))
	})
})

// clusterLabelWritingHandler writes into the label map the framework hands it, which is what a
// product does when it derives its own label set from the cluster's. It records what the CR object
// itself carried immediately afterwards.
type clusterLabelWritingHandler struct {
	*reconciler.BaseRoleGroupHandler[*testutil.MockCluster]
	roleGroupCRLabels map[string]string
	rolePDBCRLabels   map[string]string
}

func (h *clusterLabelWritingHandler) BuildResources(
	ctx context.Context,
	k8sClient client.Client,
	cr *testutil.MockCluster,
	buildCtx *reconciler.RoleGroupBuildContext,
) (*reconciler.RoleGroupResources, error) {
	buildCtx.ClusterLabels["injected-by-handler"] = "true"
	h.roleGroupCRLabels = maps.Clone(cr.GetLabels())
	return h.BaseRoleGroupHandler.BuildResources(ctx, k8sClient, cr, buildCtx)
}

func (h *clusterLabelWritingHandler) BuildRolePodDisruptionBudget(
	clusterName, namespace, roleName string,
	clusterLabels map[string]string,
	roleSpec *v1alpha1.RoleSpec,
) *policyv1.PodDisruptionBudget {
	clusterLabels["injected-by-handler"] = "true"
	h.rolePDBCRLabels = clusterLabels
	return h.BaseRoleGroupHandler.BuildRolePodDisruptionBudget(clusterName, namespace, roleName, clusterLabels, roleSpec)
}

var _ = Describe("GenericReconciler cluster label ownership", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("hands handlers a copy of the CR's labels, not the live map", func() {
		crName := uniqueCRName("label-alias")
		cr := testutil.NewMockCluster(crName, testNamespace).
			WithLabels(map[string]string{"env": "prod"}).
			WithRoles(map[string]v1alpha1.RoleSpec{
				"broker": {RoleGroups: map[string]v1alpha1.RoleGroupSpec{"default": {Replicas: ptr.To(int32(1))}}},
			})
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		resourceName := reconciler.RoleGroupResourceName(crName, "broker", "default")
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, cr)
			for _, suffix := range []string{"", "-headless"} {
				meta := metav1.ObjectMeta{Name: resourceName + suffix, Namespace: testNamespace}
				_ = k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: meta})
				_ = k8sClient.Delete(ctx, &corev1.Service{ObjectMeta: meta})
				_ = k8sClient.Delete(ctx, &appsv1.StatefulSet{ObjectMeta: meta})
			}
		})

		handler := &clusterLabelWritingHandler{
			BaseRoleGroupHandler: reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster]("img:1", testScheme),
		}
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: handler,
			Prototype:        testutil.NewMockCluster("proto", testNamespace),
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: crName}})
		Expect(err).NotTo(HaveOccurred())

		// GetLabels hands out the live CR's map: without a clone the handler's write lands on the
		// object this cycle's status decisions are computed from.
		Expect(handler.roleGroupCRLabels).NotTo(HaveKey("injected-by-handler"))
		Expect(handler.rolePDBCRLabels).NotTo(BeNil())
		Expect(handler.rolePDBCRLabels).To(HaveKey("injected-by-handler"))

		fetched := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: crName}, fetched)).To(Succeed())
		Expect(fetched.GetLabels()).NotTo(HaveKey("injected-by-handler"))
	})
})

var _ = Describe("GenericReconciler orphan drain wiring", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("requeues an orphan deletion in flight on the configured drain poll interval", func() {
		crName := uniqueCRName("drain-poll")
		cr, _ := newResilienceCR(ctx, crName)

		orphanName := reconciler.RoleGroupResourceName(crName, "broker", "removed")
		orphan := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: orphanName, Namespace: testNamespace},
			Spec: appsv1.StatefulSetSpec{
				Replicas: ptr.To(int32(2)),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": orphanName}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": orphanName}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img"}}},
				},
			},
		}
		Expect(controllerutil.SetControllerReference(cr, orphan, testScheme)).To(Succeed())
		Expect(k8sClient.Create(ctx, orphan)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, orphan)
		})

		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:              k8sClient,
			Scheme:              testScheme,
			Recorder:            recorder,
			RoleGroupHandler:    &handlerAdapter{handler: testutil.NewMockRoleGroupHandler()},
			Prototype:           testutil.NewMockCluster("proto", testNamespace),
			HealthCheckInterval: time.Hour,
			DrainPollInterval:   7 * time.Second,
		})
		Expect(err).NotTo(HaveOccurred())

		fetched := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: cr.Name}, fetched)).To(Succeed())
		fetched.Status.SetRoleGroup("broker", "removed")
		Expect(k8sClient.Status().Update(ctx, fetched)).To(Succeed())

		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cr.Name}})
		Expect(err).NotTo(HaveOccurred())
		// The orphan is scaled to zero and its deletion resumes on a later pass; without this
		// wiring the state machine only advanced when an unrelated event happened to arrive.
		Expect(result.RequeueAfter).To(Equal(7 * time.Second))
	})
})

var _ = Describe("GenericReconciler role PodDisruptionBudget reclaim", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("reclaims only the PDB carrying the framework's role slot", func() {
		crName := uniqueCRName("role-pdb-slot")
		cr, _ := newResilienceCR(ctx, crName)

		// The role declares no podDisruptionBudget, so the framework builds none and the apply
		// path reclaims the role slot. A product's own PDB may legitimately live under the derived
		// name and carries this CR's controller owner reference like every framework object.
		name := reconciler.RoleResourceName(crName, "broker")
		custom := &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
			Spec: policyv1.PodDisruptionBudgetSpec{
				MaxUnavailable: ptr.To(intstr.FromInt32(1)),
				Selector:       &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			},
		}
		Expect(controllerutil.SetControllerReference(cr, custom, testScheme)).To(Succeed())
		Expect(k8sClient.Create(ctx, custom)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, custom)
		})

		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster]("img:1", testScheme),
			Prototype:        testutil.NewMockCluster("proto", testNamespace),
		})
		Expect(err).NotTo(HaveOccurred())

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cr.Name}}
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(custom), &policyv1.PodDisruptionBudget{})).To(Succeed())

		// Stamped with the slot label, the same object IS the framework's role PDB, and turning
		// the role's budget off has to remove it.
		live := &policyv1.PodDisruptionBudget{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(custom), live)).To(Succeed())
		live.Labels = map[string]string{reconciler.LabelRolePodDisruptionBudget: "broker"}
		Expect(k8sClient.Update(ctx, live)).To(Succeed())

		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(custom), &policyv1.PodDisruptionBudget{})).
			To(MatchError(k8serrors.IsNotFound, "IsNotFound"))
	})
})

var _ = Describe("GenericReconciler role PodDisruptionBudget lifecycle", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("reclaims the role PDB when the role is removed from the spec", func() {
		crName := uniqueCRName("role-pdb-cr")
		roleSpec := func() v1alpha1.RoleSpec {
			return v1alpha1.RoleSpec{
				RoleConfig: &v1alpha1.RoleConfigSpec{
					PodDisruptionBudget: &v1alpha1.PodDisruptionBudgetSpec{
						Enabled:        ptr.To(true),
						MaxUnavailable: ptr.To(int32(1)),
					},
				},
				RoleGroups: map[string]v1alpha1.RoleGroupSpec{"default": {Replicas: ptr.To(int32(1))}},
			}
		}
		cr := testutil.NewMockCluster(crName, testNamespace).
			WithRoles(map[string]v1alpha1.RoleSpec{"broker": roleSpec(), "worker": roleSpec()})
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, cr)
			for _, role := range []string{"broker", "worker"} {
				name := reconciler.RoleResourceName(crName, role)
				_ = k8sClient.Delete(ctx, &policyv1.PodDisruptionBudget{
					ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace}})
				for _, suffix := range []string{"", "-headless"} {
					meta := metav1.ObjectMeta{Name: reconciler.RoleGroupResourceName(crName, role, "default") + suffix, Namespace: testNamespace}
					_ = k8sClient.Delete(ctx, &corev1.Service{ObjectMeta: meta})
					_ = k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: meta})
					_ = k8sClient.Delete(ctx, &appsv1.StatefulSet{ObjectMeta: meta})
				}
			}
		})

		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster]("test-image:latest", testScheme),
			Prototype:        testutil.NewMockCluster("proto", testNamespace),
		})
		Expect(err).NotTo(HaveOccurred())

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: crName}}
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		workerPDBKey := types.NamespacedName{Namespace: testNamespace, Name: reconciler.RoleResourceName(crName, "worker")}
		brokerPDBKey := types.NamespacedName{Namespace: testNamespace, Name: reconciler.RoleResourceName(crName, "broker")}
		workerPDB := &policyv1.PodDisruptionBudget{}
		Expect(k8sClient.Get(ctx, workerPDBKey, workerPDB)).To(Succeed())
		// The slot label carries the role, which is the only thing that survives the role's
		// removal from the spec.
		Expect(workerPDB.Labels).To(HaveKeyWithValue(reconciler.LabelRolePodDisruptionBudget, "worker"))
		Expect(k8sClient.Get(ctx, brokerPDBKey, &policyv1.PodDisruptionBudget{})).To(Succeed())

		fetched := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, req.NamespacedName, fetched)).To(Succeed())
		delete(fetched.Spec.Roles, "worker")
		Expect(k8sClient.Update(ctx, fetched)).To(Succeed())

		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		// The role PDB is applied only for roles the spec declares, so removing the whole role
		// leaves a budget guarding pods that no longer exist.
		Expect(k8sClient.Get(ctx, workerPDBKey, &policyv1.PodDisruptionBudget{})).
			To(MatchError(k8serrors.IsNotFound, "IsNotFound"))
		Expect(k8sClient.Get(ctx, brokerPDBKey, &policyv1.PodDisruptionBudget{})).To(Succeed())
	})
})

var _ = Describe("GenericReconciler deletion handling", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	newDeletionReconciler := func() *reconciler.GenericReconciler[*testutil.MockCluster] {
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: &handlerAdapter{handler: testutil.NewMockRoleGroupHandler()},
			Prototype:        testutil.NewMockCluster("proto", testNamespace),
		})
		Expect(err).NotTo(HaveOccurred())
		return r
	}

	// newTerminatingCR creates a cluster CR held by a finalizer and deletes it, which is what
	// foreground propagation looks like from the reconciler's side: the object keeps answering Get,
	// with deletionTimestamp set, until its dependents are gone. Without a finalizer the API server
	// would remove the object outright and the test would only exercise the IsNotFound branch.
	newTerminatingCR := func(name string) (*testutil.MockCluster, string) {
		cr, resourceName := newResilienceCR(ctx, name)

		Expect(controllerutil.AddFinalizer(cr, "test.zncdata.dev/hold")).To(BeTrue())
		Expect(k8sClient.Update(ctx, cr)).To(Succeed())
		DeferCleanup(func() {
			live := &testutil.MockCluster{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), live); err == nil {
				controllerutil.RemoveFinalizer(live, "test.zncdata.dev/hold")
				_ = k8sClient.Update(ctx, live)
			}
		})

		Expect(k8sClient.Delete(ctx, cr)).To(Succeed())

		terminating := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), terminating)).To(Succeed())
		Expect(terminating.DeletionTimestamp.IsZero()).To(BeFalse())

		return cr, resourceName
	}

	It("creates nothing for a CR that is already terminating", func() {
		cr, resourceName := newTerminatingCR(uniqueCRName("deleting-fresh"))
		r := newDeletionReconciler()

		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cr)})
		Expect(err).NotTo(HaveOccurred())
		// Nothing is pending, so nothing should be scheduled: a requeue here would poll a CR that
		// is on its way out.
		Expect(result.RequeueAfter).To(BeZero())

		key := types.NamespacedName{Namespace: testNamespace, Name: resourceName}
		Expect(k8sClient.Get(ctx, key, &appsv1.StatefulSet{})).
			To(MatchError(k8serrors.IsNotFound, "IsNotFound"))
		Expect(k8sClient.Get(ctx, key, &corev1.ConfigMap{})).
			To(MatchError(k8serrors.IsNotFound, "IsNotFound"))
	})

	It("does not re-create a garbage-collected resource while the CR is terminating", func() {
		// The regression this guards: under foreground propagation the CR stays readable, so a
		// reconciler that only checks IsNotFound runs a full pass and re-creates every owned
		// resource with BlockOwnerDeletion: true. Deletion can then never retire the CR, and each
		// GC delete fires an Owns() watch event that schedules the recreate again — an
		// un-backed-off loop against a permanently Terminating CR.
		cr, resourceName := newResilienceCR(ctx, uniqueCRName("deleting-gc"))
		r := newDeletionReconciler()
		req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cr)}

		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		key := types.NamespacedName{Namespace: testNamespace, Name: resourceName}
		Expect(k8sClient.Get(ctx, key, &appsv1.StatefulSet{})).To(Succeed())

		live := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, req.NamespacedName, live)).To(Succeed())
		Expect(controllerutil.AddFinalizer(live, "test.zncdata.dev/hold")).To(BeTrue())
		Expect(k8sClient.Update(ctx, live)).To(Succeed())
		DeferCleanup(func() {
			remaining := &testutil.MockCluster{}
			if err := k8sClient.Get(ctx, req.NamespacedName, remaining); err == nil {
				controllerutil.RemoveFinalizer(remaining, "test.zncdata.dev/hold")
				_ = k8sClient.Update(ctx, remaining)
			}
		})
		Expect(k8sClient.Delete(ctx, live)).To(Succeed())

		// Stand in for the garbage collector reclaiming a dependent.
		Expect(k8sClient.Delete(ctx, &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: testNamespace},
		})).To(Succeed())

		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, key, &appsv1.StatefulSet{})).
			To(MatchError(k8serrors.IsNotFound, "IsNotFound"))
	})

	It("leaves the status of a terminating CR untouched", func() {
		cr, _ := newTerminatingCR(uniqueCRName("deleting-status"))
		r := newDeletionReconciler()

		before := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), before)).To(Succeed())

		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cr)})
		Expect(err).NotTo(HaveOccurred())

		// A status write on a terminating CR is pure noise: it cannot describe anything actionable,
		// and because the controller watches its own CR it would schedule yet another pass.
		after := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), after)).To(Succeed())
		Expect(after.ResourceVersion).To(Equal(before.ResourceVersion))
		Expect(after.Status.Conditions).To(HaveLen(len(before.Status.Conditions)))
	})
})
