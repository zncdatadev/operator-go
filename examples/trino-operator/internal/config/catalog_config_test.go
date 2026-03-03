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

package config

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
)

var _ = Describe("CatalogConfigBuilder", func() {
	Describe("NewCatalogConfigBuilder", func() {
		It("should create a new builder with empty catalogs", func() {
			builder := NewCatalogConfigBuilder()
			Expect(builder).NotTo(BeNil())
			Expect(builder.catalogs).To(BeEmpty())
		})
	})

	Describe("WithCatalogs", func() {
		It("should set catalogs on the builder", func() {
			builder := NewCatalogConfigBuilder()
			catalogs := []trinov1alpha1.CatalogSpec{
				{Name: "hive", Type: "hive"},
			}
			result := builder.WithCatalogs(catalogs)
			Expect(result).To(Equal(builder))
			Expect(builder.catalogs).To(HaveLen(1))
			Expect(builder.catalogs[0].Name).To(Equal("hive"))
		})

		It("should allow method chaining", func() {
			catalogs := []trinov1alpha1.CatalogSpec{
				{Name: "iceberg", Type: "iceberg"},
			}
			builder := NewCatalogConfigBuilder().WithCatalogs(catalogs)
			Expect(builder).NotTo(BeNil())
			Expect(builder.catalogs).To(HaveLen(1))
		})
	})

	Describe("Build", func() {
		Context("with empty catalog list", func() {
			It("should return an empty map", func() {
				result := NewCatalogConfigBuilder().WithCatalogs([]trinov1alpha1.CatalogSpec{}).Build()
				Expect(result).To(BeEmpty())
			})

			It("should return an empty map when no catalogs are set", func() {
				result := NewCatalogConfigBuilder().Build()
				Expect(result).To(BeEmpty())
			})
		})

		Context("with single catalog", func() {
			DescribeTable("should set connector.name correctly for each catalog type",
				func(catalogType string, expectedConnector string) {
					catalogs := []trinov1alpha1.CatalogSpec{
						{Name: catalogType, Type: catalogType},
					}
					result := NewCatalogConfigBuilder().WithCatalogs(catalogs).Build()
					Expect(result).To(HaveKey(catalogType))
					Expect(result[catalogType]).To(ContainSubstring("connector.name=" + expectedConnector))
				},
				Entry("hive catalog", "hive", "hive"),
				Entry("iceberg catalog", "iceberg", "iceberg"),
				Entry("kafka catalog", "kafka", "kafka"),
				Entry("mysql catalog", "mysql", "mysql"),
				Entry("postgresql catalog", "postgresql", "postgresql"),
				Entry("delta catalog", "delta", "delta"),
				Entry("tpch catalog", "tpch", "tpch"),
				Entry("tpcds catalog", "tpcds", "tpcds"),
			)

			It("should use catalog type as connector name for unknown types", func() {
				catalogs := []trinov1alpha1.CatalogSpec{
					{Name: "custom", Type: "custom-connector"},
				}
				result := NewCatalogConfigBuilder().WithCatalogs(catalogs).Build()
				Expect(result["custom"]).To(ContainSubstring("connector.name=custom-connector"))
			})

			It("should include custom properties in the output", func() {
				catalogs := []trinov1alpha1.CatalogSpec{
					{
						Name: "hive",
						Type: "hive",
						Properties: map[string]string{
							"hive.metastore.uri": "thrift://metastore:9083",
						},
					},
				}
				result := NewCatalogConfigBuilder().WithCatalogs(catalogs).Build()
				Expect(result["hive"]).To(ContainSubstring("connector.name=hive"))
				Expect(result["hive"]).To(ContainSubstring("hive.metastore.uri=thrift://metastore:9083"))
			})

			It("should include multiple custom properties", func() {
				catalogs := []trinov1alpha1.CatalogSpec{
					{
						Name: "iceberg",
						Type: "iceberg",
						Properties: map[string]string{
							"iceberg.catalog.type":       "hadoop",
							"iceberg.warehouse.location": "/warehouse/iceberg",
						},
					},
				}
				result := NewCatalogConfigBuilder().WithCatalogs(catalogs).Build()
				Expect(result["iceberg"]).To(ContainSubstring("connector.name=iceberg"))
				Expect(result["iceberg"]).To(ContainSubstring("iceberg.catalog.type=hadoop"))
				Expect(result["iceberg"]).To(ContainSubstring("iceberg.warehouse.location=/warehouse/iceberg"))
			})
		})

		Context("with multiple catalogs", func() {
			It("should return a map with all catalog names as keys", func() {
				catalogs := []trinov1alpha1.CatalogSpec{
					{Name: "hive", Type: "hive"},
					{Name: "iceberg", Type: "iceberg"},
					{Name: "kafka", Type: "kafka"},
				}
				result := NewCatalogConfigBuilder().WithCatalogs(catalogs).Build()
				Expect(result).To(HaveLen(3))
				Expect(result).To(HaveKey("hive"))
				Expect(result).To(HaveKey("iceberg"))
				Expect(result).To(HaveKey("kafka"))
			})

			It("should set correct connector.name for each catalog", func() {
				catalogs := []trinov1alpha1.CatalogSpec{
					{Name: "my-hive", Type: "hive"},
					{Name: "my-iceberg", Type: "iceberg"},
					{Name: "my-kafka", Type: "kafka"},
				}
				result := NewCatalogConfigBuilder().WithCatalogs(catalogs).Build()
				Expect(result["my-hive"]).To(ContainSubstring("connector.name=hive"))
				Expect(result["my-iceberg"]).To(ContainSubstring("connector.name=iceberg"))
				Expect(result["my-kafka"]).To(ContainSubstring("connector.name=kafka"))
			})

			It("should handle catalogs with and without properties", func() {
				catalogs := []trinov1alpha1.CatalogSpec{
					{
						Name: "hive",
						Type: "hive",
						Properties: map[string]string{
							"hive.metastore.uri": "thrift://metastore:9083",
						},
					},
					{Name: "tpch", Type: "tpch"},
				}
				result := NewCatalogConfigBuilder().WithCatalogs(catalogs).Build()
				Expect(result["hive"]).To(ContainSubstring("connector.name=hive"))
				Expect(result["hive"]).To(ContainSubstring("hive.metastore.uri=thrift://metastore:9083"))
				Expect(result["tpch"]).To(ContainSubstring("connector.name=tpch"))
				Expect(strings.Count(result["tpch"], "\n")).To(BeZero()) // Only connector.name line
			})

			It("should handle multiple catalogs of the same type", func() {
				catalogs := []trinov1alpha1.CatalogSpec{
					{
						Name: "hive-prod",
						Type: "hive",
						Properties: map[string]string{
							"hive.metastore.uri": "thrift://prod-metastore:9083",
						},
					},
					{
						Name: "hive-dev",
						Type: "hive",
						Properties: map[string]string{
							"hive.metastore.uri": "thrift://dev-metastore:9083",
						},
					},
				}
				result := NewCatalogConfigBuilder().WithCatalogs(catalogs).Build()
				Expect(result).To(HaveLen(2))
				Expect(result["hive-prod"]).To(ContainSubstring("connector.name=hive"))
				Expect(result["hive-prod"]).To(ContainSubstring("hive.metastore.uri=thrift://prod-metastore:9083"))
				Expect(result["hive-dev"]).To(ContainSubstring("connector.name=hive"))
				Expect(result["hive-dev"]).To(ContainSubstring("hive.metastore.uri=thrift://dev-metastore:9083"))
			})
		})

		Context("output format", func() {
			It("should format properties as key=value with newlines", func() {
				catalogs := []trinov1alpha1.CatalogSpec{
					{
						Name: "test",
						Type: "hive",
						Properties: map[string]string{
							"property1": "value1",
							"property2": "value2",
						},
					},
				}
				result := NewCatalogConfigBuilder().WithCatalogs(catalogs).Build()
				lines := strings.Split(result["test"], "\n")
				Expect(lines).To(ContainElements(
					"connector.name=hive",
					"property1=value1",
					"property2=value2",
				))
			})

			It("should start with connector.name as the first property", func() {
				catalogs := []trinov1alpha1.CatalogSpec{
					{
						Name: "test",
						Type: "postgresql",
						Properties: map[string]string{
							"connection-url": "jdbc:postgresql://localhost:5432/db",
						},
					},
				}
				result := NewCatalogConfigBuilder().WithCatalogs(catalogs).Build()
				Expect(result["test"]).To(HavePrefix("connector.name=postgresql"))
			})
		})
	})
})
