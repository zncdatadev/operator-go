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
	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// The metrics slot is identified by framework-stamped labels, not by its name, so a handler that
// publishes the metrics Service under a custom name (builder.WithName — the migration path for
// products whose pre-framework operator named the metrics Service after the role group itself)
// gets the same apply and reclaim guarantees as the default "<resource>-metrics" name.
var _ = Describe("Metrics Service slot", func() {
	var crName, namespace, resourceName string
	var cr *testutil.MockCluster
	var emitMetrics bool
	var r *reconciler.GenericReconciler[*testutil.MockCluster]

	BeforeEach(func() {
		namespace = testNamespace
		crName = fmt.Sprintf("metrics-cr-%d", time.Now().UnixNano())
		resourceName = crName + "-test-role-default"
		emitMetrics = true

		mockHandler := testutil.NewMockRoleGroupHandler().WithBuildResourcesFunc(
			func(ctx context.Context, k8sClient client.Client, mc *testutil.MockCluster, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
				res, err := testutil.NewMockRoleGroupHandler().BuildResources(ctx, k8sClient, mc, buildCtx)
				if err != nil {
					return nil, err
				}
				// Keep the role group's own name free for the custom-named metrics slot, like a
				// product whose metrics Service shares the role group resource name.
				res.Service = nil
				if emitMetrics {
					res.MetricsService = builder.NewMetricsServiceBuilder(buildCtx.ResourceName, buildCtx.ClusterNamespace, 9102, buildCtx.ClusterLabels).
						WithName(buildCtx.ResourceName).
						Build()
				}
				return res, nil
			})

		cr = testutil.NewMockCluster(crName, namespace).WithRoles(map[string]v1alpha1.RoleSpec{
			"test-role": {
				RoleGroups: map[string]v1alpha1.RoleGroupSpec{
					"default": {Replicas: ptr.To(int32(1))},
				},
			},
		})
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())

		cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: &handlerAdapter{handler: mockHandler},
			Prototype:        cr,
		}
		var err error
		r, err = reconciler.NewGenericReconciler(cfg)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, cr))).To(Succeed())
	})

	reconcileCR := func() {
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}})
		Expect(err).NotTo(HaveOccurred())
	}

	It("stamps the slot and identity labels on a custom-named metrics service", func() {
		reconcileCR()

		svc := &corev1.Service{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName}, svc)).To(Succeed())
		Expect(svc.Labels).To(HaveKeyWithValue(reconciler.LabelMetricsService, "true"))
		Expect(svc.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", crName))
		Expect(svc.Labels).To(HaveKeyWithValue(
			reconciler.RoleGroupMarkerLabelKey(crName, "test-role", "default"), "true"))
		Expect(metav1.GetControllerOf(svc)).NotTo(BeNil())
	})

	It("reclaims a custom-named metrics service when the handler stops shipping one", func() {
		reconcileCR()
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName}, &corev1.Service{})).To(Succeed())

		emitMetrics = false
		reconcileCR()

		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName}, &corev1.Service{})
		Expect(errors.IsNotFound(err)).To(BeTrue(), "custom-named metrics service should be reclaimed")
	})

	It("leaves a product service under the derived name alone when it is not the slot", func() {
		emitMetrics = false
		reconcileCR()

		// A product object under the derived "<resource>-metrics" name, owned by the CR but not
		// carrying the slot label: reclaim must not touch it.
		foreign := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName + "-metrics", Namespace: namespace},
			Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 1234}}},
		}
		Expect(controllerutil.SetControllerReference(cr, foreign, testScheme)).To(Succeed())
		Expect(k8sClient.Create(ctx, foreign)).To(Succeed())

		reconcileCR()

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName + "-metrics"}, &corev1.Service{})).To(Succeed())
	})
})
