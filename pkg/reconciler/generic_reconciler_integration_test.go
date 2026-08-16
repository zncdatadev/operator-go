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
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// productStatusExtension stands in for a product hook that computes a status field the SDK's
// ClusterInterface knows nothing about. Being registered for the concrete CR type, it reaches
// that field directly.
type productStatusExtension struct {
	value string
}

func (e *productStatusExtension) Name() string { return "product-status" }

func (e *productStatusExtension) PreReconcile(context.Context, client.Client, *testutil.MockCluster) error {
	return nil
}

func (e *productStatusExtension) PostReconcile(_ context.Context, _ client.Client, cr *testutil.MockCluster) error {
	cr.Status.ProductField = e.value
	return nil
}

func (e *productStatusExtension) OnReconcileError(context.Context, client.Client, *testutil.MockCluster, error) error {
	return nil
}

// countingStatusClient records how many status update requests actually reach the API server.
type countingStatusClient struct {
	client.Client
	updates int
}

func (c *countingStatusClient) Status() client.SubResourceWriter {
	return &countingStatusWriter{SubResourceWriter: c.Client.Status(), owner: c}
}

type countingStatusWriter struct {
	client.SubResourceWriter
	owner *countingStatusClient
}

func (w *countingStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	w.owner.updates++
	return w.SubResourceWriter.Update(ctx, obj, opts...)
}

var _ = Describe("GenericReconciler steady-state status writes", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	newReconcilerWithClient := func(c client.Client) *reconciler.GenericReconciler[*testutil.MockCluster] {
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           c,
			Scheme:           testScheme,
			ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:         recorder,
			RoleGroupHandler: &handlerAdapter{handler: testutil.NewMockRoleGroupHandler()},
			Prototype:        testutil.NewMockCluster("proto", testNamespace),
		})
		Expect(err).NotTo(HaveOccurred())
		return r
	}

	It("recomputes a byte-identical status, so a settled cluster stops changing resourceVersion", func() {
		cr, _ := newResilienceCR(ctx, uniqueCRName("status-stable"))
		r := newReconcilerWithClient(k8sClient)

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cr.Name}}
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		settled := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, req.NamespacedName, settled)).To(Succeed())
		before := settled.ResourceVersion

		// Any per-reconcile churn in the computed status — a re-stamped LastTransitionTime, a
		// nondeterministically ordered field — bumps resourceVersion here, and because the
		// controller watches its own CR that bump schedules yet another reconcile.
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		after := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, req.NamespacedName, after)).To(Succeed())
		Expect(after.ResourceVersion).To(Equal(before))
	})

	It("persists product-specific status fields written by an extension hook", func() {
		cr, _ := newResilienceCR(ctx, uniqueCRName("status-product"))

		// Products compute their own status fields in a hook; ClusterInterface only exposes the
		// embedded generic status, so a write path that re-fetches the CR before updating would
		// silently reload and re-persist the stored value instead.
		registry := common.NewExtensionRegistry[*testutil.MockCluster]()
		registry.RegisterClusterExtension(&productStatusExtension{value: "ready-42"})
		DeferCleanup(registry.Clear)

		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:            k8sClient,
			Scheme:            testScheme,
			ImageResolution:   reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:          recorder,
			RoleGroupHandler:  &handlerAdapter{handler: testutil.NewMockRoleGroupHandler()},
			Prototype:         testutil.NewMockCluster("proto", testNamespace),
			ExtensionRegistry: registry,
		})
		Expect(err).NotTo(HaveOccurred())

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cr.Name}}
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		persisted := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, req.NamespacedName, persisted)).To(Succeed())
		Expect(persisted.Status.ProductField).To(Equal("ready-42"))
	})

	It("issues no status update request at all once the status has settled", func() {
		cr, _ := newResilienceCR(ctx, uniqueCRName("status-quiet"))
		counting := &countingStatusClient{Client: k8sClient}
		r := newReconcilerWithClient(counting)

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cr.Name}}
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(counting.updates).To(BeNumerically(">", 0), "the first reconcile must persist the computed status")

		// The API server would collapse an identical write anyway, but the request itself is
		// what this guard removes from the steady state.
		counting.updates = 0
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(counting.updates).To(Equal(0))
	})
})

var _ = Describe("GenericReconciler configuration warnings", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("emits a Warning when a podOverrides layer cannot be decoded", func() {
		name := uniqueCRName("bad-override")
		cr := testutil.NewMockCluster(name, testNamespace).
			WithRoles(map[string]v1alpha1.RoleSpec{
				"broker": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {
							Replicas: ptr.To(int32(1)),
							// containers must be a list; the merger drops the layer.
							PodOverrides: &k8sruntime.RawExtension{
								Raw: []byte(`{"spec":{"containers":"not-a-list"}}`),
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
			_ = k8sClient.Delete(ctx, &corev1.Service{ObjectMeta: meta})
		})

		fakeRecorder := record.NewFakeRecorder(100)
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:         fakeRecorder,
			RoleGroupHandler: &handlerAdapter{handler: testutil.NewMockRoleGroupHandler()},
			Prototype:        testutil.NewMockCluster("proto", testNamespace),
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: name}})
		Expect(err).NotTo(HaveOccurred())

		// Dropping a user's override silently is worse than applying it partially: the author
		// gets no signal that their podOverrides never took effect.
		Expect(drainRecorder(fakeRecorder)).To(ContainElement(SatisfyAll(
			ContainSubstring("Warning"),
			ContainSubstring("PodOverrideIgnored"),
		)))
	})

})

var _ = Describe("GenericReconciler metrics Service reclaim", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("leaves a sibling role group's Service alone when its name collides with the derived metrics name", func() {
		name := uniqueCRName("metrics-clash")
		// Role group "default-metrics" owns the Service "<cluster>-broker-default-metrics",
		// which is exactly the name derived for role group "default"'s metrics slot.
		cr := testutil.NewMockCluster(name, testNamespace).
			WithRoles(map[string]v1alpha1.RoleSpec{
				"broker": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default":         {Replicas: ptr.To(int32(1))},
						"default-metrics": {Replicas: ptr.To(int32(1))},
					},
				},
			})
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		siblingName := reconciler.RoleGroupResourceName(name, "broker", "default-metrics")
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, cr)
			for _, n := range []string{reconciler.RoleGroupResourceName(name, "broker", "default"), siblingName} {
				meta := metav1.ObjectMeta{Name: n, Namespace: testNamespace}
				_ = k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: meta})
				_ = k8sClient.Delete(ctx, &corev1.Service{ObjectMeta: meta})
				_ = k8sClient.Delete(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: n + "-headless", Namespace: testNamespace}})
			}
		})

		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:         recorder,
			RoleGroupHandler: &handlerAdapter{handler: testutil.NewMockRoleGroupHandler()},
			Prototype:        testutil.NewMockCluster("proto", testNamespace),
		})
		Expect(err).NotTo(HaveOccurred())

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: name}}
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		siblingKey := types.NamespacedName{Namespace: testNamespace, Name: siblingName}
		sibling := &corev1.Service{}
		Expect(k8sClient.Get(ctx, siblingKey, sibling)).To(Succeed())
		originalUID := sibling.UID

		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		// Both objects carry this CR's controller owner reference, so ownership alone cannot
		// tell the sibling's client Service apart from a metrics Service to reclaim. Role groups
		// are iterated in map order, so an unguarded reclaim either deletes the Service outright
		// or deletes and recreates it within the pass — the UID catches both.
		Expect(k8sClient.Get(ctx, siblingKey, sibling)).To(Succeed())
		Expect(sibling.UID).To(Equal(originalUID), "the sibling role group's Service was deleted and recreated")
	})
})

var _ = Describe("GenericReconciler status stability under a failing service health check", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("stops writing once an unhealthy-but-stable cluster has been reported", func() {
		cr, _ := newResilienceCR(ctx, uniqueCRName("svc-unhealthy"))
		counting := &countingStatusClient{Client: k8sClient}
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:             counting,
			Scheme:             testScheme,
			ImageResolution:    reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:           recorder,
			RoleGroupHandler:   &handlerAdapter{handler: testutil.NewMockRoleGroupHandler()},
			Prototype:          testutil.NewMockCluster("proto", testNamespace),
			ServiceHealthCheck: common.AlwaysUnhealthy,
		})
		Expect(err).NotTo(HaveOccurred())

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cr.Name}}
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		persisted := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, req.NamespacedName, persisted)).To(Succeed())
		Expect(persisted.Status.GetCondition(v1alpha1.ConditionDegraded).Status).To(Equal(metav1.ConditionTrue))
		Expect(persisted.Status.GetCondition(v1alpha1.ConditionServiceHealthy).Status).To(Equal(metav1.ConditionFalse))

		// "Pods ready, product probe says not ready" (HDFS in SafeMode, a Trino coordinator not
		// yet accepting queries) is a stable state, not a changing one. Computing Degraded in two
		// steps — cleared after the pod check, set again after the service check — flips the
		// condition inside one pass and re-stamps LastTransitionTime, so the status would differ
		// every cycle and the controller would reschedule itself forever.
		counting.updates = 0
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(counting.updates).To(Equal(0))
	})
})

var _ = Describe("GenericReconciler status on failure and non-running states", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("falsifies ReconcileComplete when the cycle fails", func() {
		cr, _ := newResilienceCR(ctx, uniqueCRName("reconcile-failed"))

		registry := common.NewExtensionRegistry[*testutil.MockCluster]()
		registry.RegisterClusterExtension(&failingPostReconcileExtension{})
		DeferCleanup(registry.Clear)

		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:            k8sClient,
			Scheme:            testScheme,
			ImageResolution:   reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:          recorder,
			RoleGroupHandler:  &handlerAdapter{handler: testutil.NewMockRoleGroupHandler()},
			Prototype:         testutil.NewMockCluster("proto", testNamespace),
			ExtensionRegistry: registry,
		})
		Expect(err).NotTo(HaveOccurred())

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cr.Name}}
		_, err = r.Reconcile(ctx, req)
		Expect(err).To(HaveOccurred())

		// Without this, a cluster that succeeded once keeps advertising ReconcileComplete=True
		// next to Degraded=True for every later failure.
		persisted := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, req.NamespacedName, persisted)).To(Succeed())
		Expect(persisted.Status.GetCondition(v1alpha1.ConditionReconcileComplete).Status).To(Equal(metav1.ConditionFalse))
		Expect(persisted.Status.GetCondition(v1alpha1.ConditionDegraded).Status).To(Equal(metav1.ConditionTrue))
	})

	It("reports a cluster that declares no role groups as unavailable", func() {
		name := uniqueCRName("no-roles")
		cr := testutil.NewMockCluster(name, testNamespace).WithRoles(map[string]v1alpha1.RoleSpec{})
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cr) })

		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:         recorder,
			RoleGroupHandler: &handlerAdapter{handler: testutil.NewMockRoleGroupHandler()},
			Prototype:        testutil.NewMockCluster("proto", testNamespace),
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: name}})
		Expect(err).NotTo(HaveOccurred())

		// Nothing runs, so Available=True would make the condition useless as a readiness gate.
		persisted := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: name}, persisted)).To(Succeed())
		Expect(persisted.Status.GetCondition(v1alpha1.ConditionAvailable).Status).To(Equal(metav1.ConditionFalse))
	})
})

// altMockClusterGVK identifies the second mock product CR (CRD in
// config/crd/bases/test.zncdata.dev_altmockclusters.yaml).
var altMockClusterGVK = schema.GroupVersionKind{Group: "test.zncdata.dev", Version: "v1alpha1", Kind: "AltMockCluster"}

// addAltMockClusterToScheme registers the alt product CR. Every scheme a client or reconciler is
// built with needs it, since a ClusterInterface is read and owned as itself.
func addAltMockClusterToScheme(s *k8sruntime.Scheme) {
	s.AddKnownTypeWithName(altMockClusterGVK, &testutil.AltMockCluster{})
	metav1.AddToGroupVersion(s, altMockClusterGVK.GroupVersion())
}

// newAltCR creates a single-role-group alt-product cluster CR in the API server and registers
// its teardown, returning the CR and the role group's resource name.
func newAltCR(ctx context.Context, name string) (*testutil.AltMockCluster, string) {
	cr := &testutil.AltMockCluster{
		TypeMeta:   metav1.TypeMeta{Kind: altMockClusterGVK.Kind, APIVersion: altMockClusterGVK.GroupVersion().String()},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				"broker": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {Replicas: ptr.To(int32(1))},
					},
				},
			},
		},
	}
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

// newAltReconciler builds a GenericReconciler for the minimal CR type.
func newAltReconciler(registry *common.ExtensionRegistry[*testutil.AltMockCluster]) *reconciler.GenericReconciler[*testutil.AltMockCluster] {
	r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.AltMockCluster]{
		Client:            k8sClient,
		Scheme:            testScheme,
		ImageResolution:   reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
		Recorder:          recorder,
		RoleGroupHandler:  testutil.NewMockRoleGroupHandlerFor[*testutil.AltMockCluster](),
		Prototype:         &testutil.AltMockCluster{},
		ExtensionRegistry: registry,
	})
	Expect(err).NotTo(HaveOccurred())
	return r
}

// A product CR owes the framework its metadata (through client.Object), its spec and its status,
// and nothing else. Everything the reconciler does with the cluster object — read it, own the
// resources it builds, write its status back — goes through the CR itself, so a CR that adds no
// framework plumbing at all has to complete a full cycle.
var _ = Describe("GenericReconciler minimal cluster contract", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("reconciles a CR whose only SDK-specific methods are GetSpec and GetStatus", func() {
		cr, resourceName := newAltCR(ctx, uniqueCRName("minimal"))
		r := newAltReconciler(nil)

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cr.Name}}
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		// The role group ConfigMap is owned by the CR itself: the reconciler passed the fetched
		// cluster object straight to SetControllerReference.
		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: resourceName}, cm)).To(Succeed())
		owner := metav1.GetControllerOf(cm)
		Expect(owner).NotTo(BeNil())
		Expect(owner.Kind).To(Equal(altMockClusterGVK.Kind))
		Expect(owner.Name).To(Equal(cr.Name))
		Expect(owner.UID).To(Equal(cr.UID))

		// The status landed on the stored CR, which is only possible if the object the reconciler
		// fetched into and the object it wrote back are the same one.
		persisted := &testutil.AltMockCluster{}
		Expect(k8sClient.Get(ctx, req.NamespacedName, persisted)).To(Succeed())
		Expect(persisted.Status.ObservedGeneration).To(Equal(cr.Generation))
		Expect(persisted.Status.RoleGroups).To(HaveKey("broker"))
	})
})

// namingExtension records the clusters whose hooks it ran for, per CR type.
type namingExtension[CR common.ClusterInterface] struct {
	common.BaseExtension
	seen *[]string
}

func (e *namingExtension[CR]) PreReconcile(_ context.Context, _ client.Client, cr CR) error {
	*e.seen = append(*e.seen, e.Name()+":"+cr.GetName())
	return nil
}

func (e *namingExtension[CR]) PostReconcile(context.Context, client.Client, CR) error { return nil }

func (e *namingExtension[CR]) OnReconcileError(context.Context, client.Client, CR, error) error {
	return nil
}

// A registry belongs to the reconciler it is configured on, and its type parameter is the
// reconciler's CR type. Two products in one binary therefore cannot reach each other's
// extensions: a foreign extension does not satisfy the registry's instantiation, and there is no
// process-wide registry for either to fall back to.
var _ = Describe("GenericReconciler extension registry ownership", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("executes only the extensions of the registry it was configured with", func() {
		var ran []string

		mainCR, _ := newResilienceCR(ctx, uniqueCRName("registry-main"))
		altCR, _ := newAltCR(ctx, uniqueCRName("registry-alt"))

		mainRegistry := common.NewExtensionRegistry[*testutil.MockCluster]()
		mainRegistry.RegisterClusterExtension(&namingExtension[*testutil.MockCluster]{
			BaseExtension: common.NewBaseExtension("main"), seen: &ran,
		})

		altRegistry := common.NewExtensionRegistry[*testutil.AltMockCluster]()
		altRegistry.RegisterClusterExtension(&namingExtension[*testutil.AltMockCluster]{
			BaseExtension: common.NewBaseExtension("alt"), seen: &ran,
		})

		mainReconciler, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:            k8sClient,
			Scheme:            testScheme,
			ImageResolution:   reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:          recorder,
			RoleGroupHandler:  &handlerAdapter{handler: testutil.NewMockRoleGroupHandler()},
			Prototype:         testutil.NewMockCluster("proto", testNamespace),
			ExtensionRegistry: mainRegistry,
		})
		Expect(err).NotTo(HaveOccurred())

		altReconciler := newAltReconciler(altRegistry)

		_, err = mainReconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: mainCR.Name}})
		Expect(err).NotTo(HaveOccurred())
		_, err = altReconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: altCR.Name}})
		Expect(err).NotTo(HaveOccurred())

		Expect(ran).To(ConsistOf("main:"+mainCR.Name, "alt:"+altCR.Name))
	})

	It("reconciles with an empty registry of its own when the config declares none", func() {
		cr, _ := newResilienceCR(ctx, uniqueCRName("registry-none"))

		// There is no process-wide registry left to fall back on, so an unconfigured reconciler
		// has to substitute an empty one: the hook call sites invoke the registry unconditionally
		// on every cycle, at cluster, role and role group level.
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:         recorder,
			RoleGroupHandler: &handlerAdapter{handler: testutil.NewMockRoleGroupHandler()},
			Prototype:        testutil.NewMockCluster("proto", testNamespace),
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cr.Name}})
		Expect(err).NotTo(HaveOccurred())
	})
})

// failingPostReconcileExtension fails the cycle after every resource has been applied.
type failingPostReconcileExtension struct{}

func (e *failingPostReconcileExtension) Name() string { return "failing-post-reconcile" }

func (e *failingPostReconcileExtension) PreReconcile(context.Context, client.Client, *testutil.MockCluster) error {
	return nil
}

func (e *failingPostReconcileExtension) PostReconcile(context.Context, client.Client, *testutil.MockCluster) error {
	return fmt.Errorf("post reconcile exploded")
}

func (e *failingPostReconcileExtension) OnReconcileError(context.Context, client.Client, *testutil.MockCluster, error) error {
	return nil
}
