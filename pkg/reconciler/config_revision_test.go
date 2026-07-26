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
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Config revision", func() {
	var ctx context.Context
	var counter int

	BeforeEach(func() {
		ctx = context.Background()
	})

	// newCR creates a single-role-group cluster and returns it with its derived resource name.
	newCR := func(prefix string) (*testutil.MockCluster, string) {
		counter++
		name := fmt.Sprintf("%s-%d", prefix, counter)
		cr := testutil.NewMockCluster(name, testNamespace).
			WithRoles(map[string]v1alpha1.RoleSpec{
				"broker": {RoleGroups: map[string]v1alpha1.RoleGroupSpec{
					"default": {Replicas: ptr.To(int32(1))},
				}},
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

	// newReconciler builds a reconciler whose ConfigMap contents come from configData.
	newReconciler := func(policy reconciler.ConfigRevisionPolicy, configData func() map[string]string) *reconciler.GenericReconciler[*testutil.MockCluster] {
		handler := &reconciler.RoleGroupHandlerFuncs[*testutil.MockCluster]{
			BuildResourcesFunc: func(_ context.Context, _ client.Client, _ *testutil.MockCluster, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
				cm := testutil.NewTestConfigMap(buildCtx.ResourceName, buildCtx.ClusterNamespace)
				cm.Data = configData()
				return &reconciler.RoleGroupResources{
					ConfigMap:   cm,
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
			ConfigRevision:   policy,
		})
		Expect(err).NotTo(HaveOccurred())
		return r
	}

	revisionOf := func(resourceName string) string {
		sts := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: resourceName}, sts)).To(Succeed())
		return sts.Spec.Template.Annotations[reconciler.AnnotationConfigRevision]
	}

	It("rolls the pods when the rendered config changes", func() {
		// The behaviour the framework advertises and did not have: a configOverrides edit must
		// reach the running processes. A ConfigMap volume refreshes the FILES on disk, but none of
		// the products this SDK targets re-read their configuration at runtime.
		cr, resourceName := newCR("config-revision")
		value := "1"
		r := newReconciler(reconciler.ConfigRevisionEnabled, func() map[string]string {
			return map[string]string{"server.properties": "replicas=" + value}
		})
		req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cr)}

		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		before := revisionOf(resourceName)
		Expect(before).NotTo(BeEmpty())

		value = "2"
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		Expect(revisionOf(resourceName)).NotTo(Equal(before),
			"a changed pod template annotation is what makes the StatefulSet controller roll the pods")
	})

	It("is stable across reconciles for unchanged config", func() {
		// THE failure mode of this feature. The digest is recomputed every pass from a freshly
		// built ConfigMap, so any dependence on Go's map iteration order would produce a different
		// value each time and roll the pods forever — a far worse bug than the one being fixed.
		// Several keys, so an order-dependent implementation is very unlikely to survive by luck.
		cr, resourceName := newCR("config-revision-stable")
		r := newReconciler(reconciler.ConfigRevisionEnabled, func() map[string]string {
			return map[string]string{
				"a.properties": "1", "b.properties": "2", "c.properties": "3",
				"d.properties": "4", "e.properties": "5", "f.properties": "6",
			}
		})
		req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cr)}

		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		first := revisionOf(resourceName)

		for i := 0; i < 5; i++ {
			_, err = r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(revisionOf(resourceName)).To(Equal(first), "reconcile %d changed the digest", i+2)
		}
	})

	It("stamps nothing when the policy is disabled", func() {
		// The default. Turning the stamp on rolls every pod of every managed cluster exactly once,
		// which has to be scheduled rather than inherited from an operator upgrade.
		cr, resourceName := newCR("config-revision-off")
		r := newReconciler(reconciler.ConfigRevisionDisabled, func() map[string]string {
			return map[string]string{"server.properties": "replicas=1"}
		})

		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cr)})
		Expect(err).NotTo(HaveOccurred())

		sts := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: resourceName}, sts)).To(Succeed())
		Expect(sts.Spec.Template.Annotations).NotTo(HaveKey(reconciler.AnnotationConfigRevision))
	})

	It("distinguishes configs that differ only in how keys and values split", func() {
		// Length-prefixing guards this: concatenating key and value would make {"ab":"c"} and
		// {"a":"bc"} hash identically, so a real config change could land on the previous digest
		// and produce no rollout at all.
		crA, nameA := newCR("config-revision-split-a")
		rA := newReconciler(reconciler.ConfigRevisionEnabled, func() map[string]string {
			return map[string]string{"ab": "c"}
		})
		_, err := rA.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(crA)})
		Expect(err).NotTo(HaveOccurred())

		crB, nameB := newCR("config-revision-split-b")
		rB := newReconciler(reconciler.ConfigRevisionEnabled, func() map[string]string {
			return map[string]string{"a": "bc"}
		})
		_, err = rB.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(crB)})
		Expect(err).NotTo(HaveOccurred())

		Expect(revisionOf(nameA)).NotTo(Equal(revisionOf(nameB)))
	})
})
