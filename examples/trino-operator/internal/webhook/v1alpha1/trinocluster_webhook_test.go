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

package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/constants"
)

var _ = Describe("TrinoCluster Webhook", func() {
	var (
		ctx       context.Context
		obj       *trinov1alpha1.TrinoCluster
		oldObj    *trinov1alpha1.TrinoCluster
		validator TrinoClusterCustomValidator
		defaulter TrinoClusterCustomDefaulter
	)

	BeforeEach(func() {
		ctx = context.Background()
		obj = &trinov1alpha1.TrinoCluster{
			Spec: trinov1alpha1.TrinoClusterSpec{},
		}
		oldObj = &trinov1alpha1.TrinoCluster{
			Spec: trinov1alpha1.TrinoClusterSpec{},
		}
		validator = TrinoClusterCustomValidator{}
		defaulter = TrinoClusterCustomDefaulter{}
	})

	Context("When creating TrinoCluster under Defaulting Webhook", func() {
		It("Should apply default image when not specified", func() {
			obj.Spec.Image = ""
			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.Image).To(Equal(constants.DefaultImage))
		})

		It("Should not override image when specified", func() {
			obj.Spec.Image = "custom/trino:latest"
			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.Image).To(Equal("custom/trino:latest"))
		})

		It("Should initialize coordinators with default port", func() {
			obj.Spec.Coordinators = nil
			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.Coordinators).NotTo(BeNil())
			Expect(obj.Spec.Coordinators.HTTPPort).To(Equal(constants.DefaultHTTPPort))
		})

		It("Should not override coordinator port when specified", func() {
			obj.Spec.Coordinators = &trinov1alpha1.CoordinatorsSpec{HTTPPort: 9090}
			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.Coordinators.HTTPPort).To(Equal(int32(9090)))
		})

		It("Should initialize workers with default port", func() {
			obj.Spec.Workers = nil
			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.Workers).NotTo(BeNil())
			Expect(obj.Spec.Workers.HTTPPort).To(Equal(constants.DefaultHTTPPort))
		})

		It("Should not override worker port when specified", func() {
			obj.Spec.Workers = &trinov1alpha1.WorkersSpec{HTTPPort: 9091}
			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.Workers.HTTPPort).To(Equal(int32(9091)))
		})
	})

	Context("When creating TrinoCluster under Validating Webhook", func() {
		It("Should admit valid TrinoCluster", func() {
			obj.Spec.Image = constants.DefaultImage
			obj.Spec.Coordinators = &trinov1alpha1.CoordinatorsSpec{HTTPPort: 8080}
			obj.Spec.Workers = &trinov1alpha1.WorkersSpec{HTTPPort: 8080}
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(warnings).To(BeNil())
			Expect(err).To(Succeed())
		})

		It("Should deny invalid image format", func() {
			obj.Spec.Image = "INVALID_IMAGE!"
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.image"))
		})

		It("Should admit coordinator port 0 (unset, will be defaulted)", func() {
			obj.Spec.Coordinators = &trinov1alpha1.CoordinatorsSpec{HTTPPort: 0}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(Succeed())
		})

		It("Should deny invalid coordinator port (too high)", func() {
			obj.Spec.Coordinators = &trinov1alpha1.CoordinatorsSpec{HTTPPort: 70000}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.coordinators.httpPort"))
		})

		It("Should deny invalid worker port", func() {
			obj.Spec.Workers = &trinov1alpha1.WorkersSpec{HTTPPort: -1}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.workers.httpPort"))
		})

		It("Should deny catalog with empty name", func() {
			obj.Spec.Catalogs = []trinov1alpha1.CatalogSpec{
				{Name: "", Type: "hive"},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.catalogs[0].name"))
		})

		It("Should deny catalog with invalid name (starts with number)", func() {
			obj.Spec.Catalogs = []trinov1alpha1.CatalogSpec{
				{Name: "1hive", Type: "hive"},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.catalogs[0].name"))
		})

		It("Should deny catalog with invalid name (uppercase)", func() {
			obj.Spec.Catalogs = []trinov1alpha1.CatalogSpec{
				{Name: "Hive", Type: "hive"},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.catalogs[0].name"))
		})

		It("Should deny catalog with empty type", func() {
			obj.Spec.Catalogs = []trinov1alpha1.CatalogSpec{
				{Name: "my_hive", Type: ""},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.catalogs[0].type"))
		})

		It("Should deny catalog with invalid type", func() {
			obj.Spec.Catalogs = []trinov1alpha1.CatalogSpec{
				{Name: "my_catalog", Type: "invalid_type"},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.catalogs[0].type"))
		})

		It("Should admit valid catalog configuration", func() {
			obj.Spec.Catalogs = []trinov1alpha1.CatalogSpec{
				{Name: "hive_catalog", Type: "hive", Properties: map[string]string{"key": "value"}},
				{Name: "iceberg_catalog", Type: "iceberg"},
				{Name: "kafka_catalog", Type: "kafka"},
				{Name: "mysql_catalog", Type: "mysql"},
				{Name: "postgres_catalog", Type: "postgresql"},
				{Name: "delta_catalog", Type: "delta"},
				{Name: "tpch_catalog", Type: "tpch"},
				{Name: "tpcds_catalog", Type: "tpcds"},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(Succeed())
		})
	})

	Context("When updating TrinoCluster under Validating Webhook", func() {
		It("Should admit valid update", func() {
			oldObj.Spec.Image = "trinodb/trino:435"
			obj.Spec.Image = constants.DefaultImage
			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(Succeed())
		})

		It("Should deny image change", func() {
			oldObj.Spec.Image = "trinodb/trino:435"
			obj.Spec.Image = "trinodb/trino:436"
			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("image cannot be changed"))
		})

		It("Should admit update when old image was empty", func() {
			oldObj.Spec.Image = ""
			obj.Spec.Image = constants.DefaultImage
			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(Succeed())
		})
	})

	Context("When deleting TrinoCluster under Validating Webhook", func() {
		It("Should always admit deletion", func() {
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).To(Succeed())
		})
	})
})
