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

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/constants"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/product"
	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/constant"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/vector"
)

// TrinoCluster is the only type in either module that implements reconciler.VectorAggregatorProvider,
// so this is the one place the framework's OWN vector.yaml generation can be exercised end to end.
// Its absence is what let a stage-ordering regression ship: the aggregator address was resolved
// before MergedConfig existed, its gate read MergedConfig, and so the address was never resolved,
// no vector.yaml was written, and — because the sidecar was still registered — every role group
// with the agent enabled failed validation and stayed Degraded with no StatefulSet.
//
// Building a RoleGroupBuildContext by hand cannot catch that: the bug is in the order
// buildRoleGroupContext assigns its own fields. Only a real Reconcile can.
var _ = Describe("Vector agent, end to end through the reconciler", func() {
	const (
		ns          = "default"
		aggregator  = "vector-aggregator-discovery"
		clusterName = "vector-e2e"
	)

	It("writes vector.yaml into the role group ConfigMap", func() {
		By("publishing the aggregator discovery ConfigMap the CR points at")
		Expect(k8sClient.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: aggregator, Namespace: ns},
			Data:       map[string]string{"ADDRESS": "vector-aggregator:6000"},
		})).To(Succeed())

		By("creating a TrinoCluster with the vector agent enabled")
		cr := &trinov1alpha1.TrinoCluster{
			ObjectMeta: metav1.ObjectMeta{Name: clusterName, Namespace: ns},
			Spec: trinov1alpha1.TrinoClusterSpec{
				Coordinators: &trinov1alpha1.CoordinatorsSpec{
					RoleSpec: commonsv1alpha1.RoleSpec{
						Config: &commonsv1alpha1.RoleGroupConfigSpec{
							Logging: &commonsv1alpha1.LoggingSpec{EnableVectorAgent: ptr.To(true)},
						},
						RoleGroups: map[string]commonsv1alpha1.RoleGroupSpec{
							"default": {Replicas: ptr.To(int32(1))},
						},
					},
				},
				ClusterConfig: &trinov1alpha1.ClusterConfigSpec{
					VectorAggregatorConfigMapName: ptr.To(aggregator),
				},
			},
		}
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())

		By("reconciling")
		handler := NewTrinoRoleGroupHandler(scheme.Scheme)
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*trinov1alpha1.TrinoCluster]{
			Client:            k8sClient,
			Scheme:            scheme.Scheme,
			Recorder:          record.NewFakeRecorder(100),
			RoleGroupHandler:  handler,
			RoleProvider:      handler,
			RoleGroupResolver: reconciler.RoleGroupResolverFunc[*trinov1alpha1.TrinoCluster](product.ComputeConfig),
			ImageResolution: reconciler.ImageResolution{
				ProductName: constants.ProductName,
				Defaults:    constants.ImageDefaults(),
			},
			Prototype: &trinov1alpha1.TrinoCluster{},
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = r.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Namespace: ns, Name: clusterName},
		})
		Expect(err).NotTo(HaveOccurred(),
			"a role group with the vector agent enabled must reconcile; a missing vector.yaml "+
				"fails the sidecar's Validate and aborts the role group")

		By("asserting the framework generated vector.yaml")
		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Namespace: ns,
			Name:      reconciler.RoleGroupResourceName(clusterName, product.RoleCoordinators, "default"),
		}, cm)).To(Succeed())
		Expect(cm.Data).To(HaveKey(vector.VectorConfigFileName),
			"the CR implements VectorAggregatorProvider, so the framework owns vector.yaml")
		Expect(cm.Data[vector.VectorConfigFileName]).To(ContainSubstring("vector-aggregator:6000"),
			"the resolved aggregator address must reach the rendered pipeline")

		By("asserting the identity labels survive a real reconcile")
		// ProductName reaches the label builder only through the build context, and the reconciler
		// is the only thing that fills it in. Every handler-level test sets it by hand, so an
		// omission there is invisible: app.kubernetes.io/name is simply absent on every resource
		// the framework builds, and nothing fails.
		Expect(cm.Labels).To(HaveKeyWithValue(constant.LabelKubernetesName, constants.ProductName),
			"app.kubernetes.io/name comes from ImageResolution.ProductName via the build context")
		Expect(cm.Labels).To(HaveKeyWithValue(constant.LabelKubernetesInstance, clusterName))
	})
})
