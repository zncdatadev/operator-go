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

package extensions_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/extensions"
	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("DiscoveryExtension", func() {
	var scheme *runtime.Scheme
	var cr *trinov1alpha1.TrinoCluster

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(trinov1alpha1.AddToScheme(scheme)).To(Succeed())

		cr = &trinov1alpha1.TrinoCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "trino-sample", Namespace: "default"},
			Spec: trinov1alpha1.TrinoClusterSpec{
				Coordinators: &trinov1alpha1.CoordinatorsSpec{
					RoleSpec: commonsv1alpha1.RoleSpec{
						RoleGroups: map[string]commonsv1alpha1.RoleGroupSpec{
							"default": {},
						},
					},
				},
			},
		}
	})

	It("publishes the discovery ConfigMap with the coordinator URI and owner reference", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).Build()
		ext := extensions.NewDiscoveryExtension(scheme)

		Expect(ext.PostReconcile(context.Background(), c, cr)).To(Succeed())

		cm := &corev1.ConfigMap{}
		Expect(c.Get(context.Background(),
			types.NamespacedName{Namespace: "default", Name: "trino-sample"}, cm)).To(Succeed())
		Expect(cm.Data).To(HaveKeyWithValue(extensions.TrinoDiscoveryKey,
			"http://trino-sample-coordinators-default:8080"))
		Expect(cm.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "trino"))
		Expect(cm.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", "trino-sample"))
		Expect(cm.OwnerReferences).To(HaveLen(1))
		Expect(cm.OwnerReferences[0].Kind).To(Equal("TrinoCluster"))
		Expect(*cm.OwnerReferences[0].Controller).To(BeTrue())
	})

	It("is idempotent and refreshes the data on repeated reconciles", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).Build()
		ext := extensions.NewDiscoveryExtension(scheme)

		Expect(ext.PostReconcile(context.Background(), c, cr)).To(Succeed())
		Expect(ext.PostReconcile(context.Background(), c, cr)).To(Succeed())

		cm := &corev1.ConfigMap{}
		Expect(c.Get(context.Background(),
			types.NamespacedName{Namespace: "default", Name: "trino-sample"}, cm)).To(Succeed())
		Expect(cm.Data).To(HaveKey(extensions.TrinoDiscoveryKey))
	})

	It("skips clusters of other products without error", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		ext := extensions.NewDiscoveryExtension(scheme)
		Expect(ext.PostReconcile(context.Background(), c, nil)).To(Succeed())
	})
})
