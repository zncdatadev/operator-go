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
	"fmt"
	"strings"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
)

// CatalogConfigBuilder builds Trino catalog configuration
type CatalogConfigBuilder struct {
	catalogs []trinov1alpha1.CatalogSpec
}

// NewCatalogConfigBuilder creates a new CatalogConfigBuilder
func NewCatalogConfigBuilder() *CatalogConfigBuilder {
	return &CatalogConfigBuilder{}
}

// WithCatalogs sets the catalogs to configure
func (b *CatalogConfigBuilder) WithCatalogs(catalogs []trinov1alpha1.CatalogSpec) *CatalogConfigBuilder {
	b.catalogs = catalogs
	return b
}

// Build generates the catalog configurations as a map
func (b *CatalogConfigBuilder) Build() map[string]string {
	result := make(map[string]string)

	for _, catalog := range b.catalogs {
		result[catalog.Name] = b.buildCatalogProperties(catalog)
	}

	return result
}

// buildCatalogProperties builds the properties string for a catalog
func (b *CatalogConfigBuilder) buildCatalogProperties(catalog trinov1alpha1.CatalogSpec) string {
	lines := make([]string, 0, 1+len(catalog.Properties))

	// Add connector.name based on catalog type
	connectorName := b.getConnectorName(catalog.Type)
	lines = append(lines, fmt.Sprintf("connector.name=%s", connectorName))

	// Add custom properties
	for key, value := range catalog.Properties {
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}

	return strings.Join(lines, "\n")
}

// getConnectorName returns the connector name for a catalog type
func (b *CatalogConfigBuilder) getConnectorName(catalogType string) string {
	connectors := map[string]string{
		"hive":       "hive",
		"iceberg":    "iceberg",
		"kafka":      "kafka",
		"mysql":      "mysql",
		"postgresql": "postgresql",
		"delta":      "delta",
		"tpch":       "tpch",
		"tpcds":      "tpcds",
	}

	if connector, ok := connectors[catalogType]; ok {
		return connector
	}
	return catalogType
}
