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
	"github.com/zncdatadev/operator-go/pkg/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// This mirrors what cmd/main.go wires up: the extensions this operator ships are written for
// *TrinoCluster and go into a registry instantiated for that same type, which is then handed to
// the GenericReconciler. An extension of another product's CR type would not compile here.
var _ = Describe("Extension registration", func() {
	var scheme *runtime.Scheme
	var cr *trinov1alpha1.TrinoCluster

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(trinov1alpha1.AddToScheme(scheme)).To(Succeed())

		cr = &trinov1alpha1.TrinoCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "trino-registered", Namespace: "default"},
			Spec: trinov1alpha1.TrinoClusterSpec{
				Catalogs: []trinov1alpha1.CatalogSpec{{Name: "my-hive", Type: "hive"}},
				Coordinators: &trinov1alpha1.CoordinatorsSpec{
					RoleSpec: commonsv1alpha1.RoleSpec{
						RoleGroups: map[string]commonsv1alpha1.RoleGroupSpec{"default": {}},
					},
				},
			},
		}
	})

	It("registers this operator's extensions in a TrinoCluster registry", func() {
		registry := common.NewExtensionRegistry[*trinov1alpha1.TrinoCluster]()
		registry.RegisterClusterExtension(extensions.NewCatalogExtension())
		registry.RegisterRoleExtension(extensions.NewHealthExtension())
		registry.RegisterClusterExtension(extensions.NewDiscoveryExtension(scheme), common.WithPriority(common.PriorityLow))

		Expect(registry.Count()).To(Equal(3))
		// The discovery extension registers at a lower priority, so it runs after the catalog
		// extension has refreshed the status.
		Expect(registry.GetClusterExtensions()[0].Name()).To(Equal("catalog-extension"))
		Expect(registry.GetClusterExtensions()[1].Name()).To(Equal("discovery-extension"))
	})

	It("hands the concrete TrinoCluster to every hook the registry executes", func() {
		ctx := context.Background()
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).Build()

		registry := common.NewExtensionRegistry[*trinov1alpha1.TrinoCluster]()
		registry.RegisterClusterExtension(extensions.NewCatalogExtension())
		registry.RegisterRoleExtension(extensions.NewHealthExtension())
		registry.RegisterClusterExtension(extensions.NewDiscoveryExtension(scheme), common.WithPriority(common.PriorityLow))

		Expect(registry.ExecuteClusterPreReconcile(ctx, c, cr)).To(Succeed())
		Expect(registry.ExecuteRolePostReconcile(ctx, c, cr, "coordinators")).To(Succeed())
		Expect(registry.ExecuteClusterPostReconcile(ctx, c, cr)).To(Succeed())

		// The catalog extension wrote a Trino-only status field, so the hooks received the
		// product CR rather than the SDK's ClusterInterface view of it.
		Expect(cr.Status.CatalogsReady).To(Equal([]string{"my-hive"}))
	})
})
