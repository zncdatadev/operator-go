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

package extensions

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

var _ = Describe("CatalogExtension", func() {
	var (
		ctx     context.Context
		ext     *CatalogExtension
		trinoCR *trinov1alpha1.TrinoCluster
	)

	BeforeEach(func() {
		ctx = context.Background()
		ext = NewCatalogExtension()
		trinoCR = &trinov1alpha1.TrinoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-trino",
				Namespace: "default",
			},
			Spec: trinov1alpha1.TrinoClusterSpec{
				Image: &commonsv1alpha1.ImageSpec{Custom: "trinodb/trino:435"},
			},
		}
	})

	Describe("Name", func() {
		It("should return catalog-extension", func() {
			Expect(ext.Name()).To(Equal("catalog-extension"))
		})
	})

	Describe("Validate", func() {
		Context("with valid catalogs", func() {
			It("should pass validation with hive catalog", func() {
				trinoCR.Spec.Catalogs = []trinov1alpha1.CatalogSpec{
					{Name: "my-hive", Type: "hive"},
				}
				err := ext.PreReconcile(ctx, nil, trinoCR)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should pass validation with iceberg catalog", func() {
				trinoCR.Spec.Catalogs = []trinov1alpha1.CatalogSpec{
					{Name: "my-iceberg", Type: "iceberg"},
				}
				err := ext.PreReconcile(ctx, nil, trinoCR)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should pass validation with kafka catalog", func() {
				trinoCR.Spec.Catalogs = []trinov1alpha1.CatalogSpec{
					{Name: "my-kafka", Type: "kafka"},
				}
				err := ext.PreReconcile(ctx, nil, trinoCR)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should pass validation with mysql catalog", func() {
				trinoCR.Spec.Catalogs = []trinov1alpha1.CatalogSpec{
					{Name: "my-mysql", Type: "mysql"},
				}
				err := ext.PreReconcile(ctx, nil, trinoCR)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should pass validation with postgresql catalog", func() {
				trinoCR.Spec.Catalogs = []trinov1alpha1.CatalogSpec{
					{Name: "my-postgres", Type: "postgresql"},
				}
				err := ext.PreReconcile(ctx, nil, trinoCR)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should pass validation with delta catalog", func() {
				trinoCR.Spec.Catalogs = []trinov1alpha1.CatalogSpec{
					{Name: "my-delta", Type: "delta"},
				}
				err := ext.PreReconcile(ctx, nil, trinoCR)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should pass validation with tpch catalog", func() {
				trinoCR.Spec.Catalogs = []trinov1alpha1.CatalogSpec{
					{Name: "my-tpch", Type: "tpch"},
				}
				err := ext.PreReconcile(ctx, nil, trinoCR)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should pass validation with tpcds catalog", func() {
				trinoCR.Spec.Catalogs = []trinov1alpha1.CatalogSpec{
					{Name: "my-tpcds", Type: "tpcds"},
				}
				err := ext.PreReconcile(ctx, nil, trinoCR)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should pass validation with multiple catalogs", func() {
				trinoCR.Spec.Catalogs = []trinov1alpha1.CatalogSpec{
					{Name: "my-hive", Type: "hive"},
					{Name: "my-iceberg", Type: "iceberg"},
					{Name: "my-kafka", Type: "kafka"},
				}
				err := ext.PreReconcile(ctx, nil, trinoCR)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should pass validation with empty catalogs", func() {
				trinoCR.Spec.Catalogs = []trinov1alpha1.CatalogSpec{}
				err := ext.PreReconcile(ctx, nil, trinoCR)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("with invalid catalogs", func() {
			It("should fail validation with duplicate catalog names", func() {
				trinoCR.Spec.Catalogs = []trinov1alpha1.CatalogSpec{
					{Name: "my-hive", Type: "hive"},
					{Name: "my-hive", Type: "iceberg"},
				}
				err := ext.PreReconcile(ctx, nil, trinoCR)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("duplicate catalog name"))
			})

			It("should fail validation with invalid catalog type", func() {
				trinoCR.Spec.Catalogs = []trinov1alpha1.CatalogSpec{
					{Name: "my-invalid", Type: "invalid-type"},
				}
				err := ext.PreReconcile(ctx, nil, trinoCR)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid catalog type"))
			})

			It("should fail validation with empty catalog type", func() {
				trinoCR.Spec.Catalogs = []trinov1alpha1.CatalogSpec{
					{Name: "my-empty-type", Type: ""},
				}
				err := ext.PreReconcile(ctx, nil, trinoCR)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid catalog type"))
			})
		})

		Context("with invalid cluster type", func() {
			It("should fail when passed wrong cluster type", func() {
				// The type assertion check in PreReconcile is a runtime safety check
				// Skip this test as it requires a mock that implements ClusterInterface but isn't *TrinoCluster
				Skip("Cannot create a type that implements ClusterInterface but isn't *TrinoCluster without extensive mocking")
			})
		})
	})
})
