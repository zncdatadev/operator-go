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
	"fmt"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/common"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// CatalogExtension is a ClusterExtension that validates and processes catalog configurations
// This demonstrates the ClusterExtension extension point of operator-go SDK
type CatalogExtension struct {
	common.BaseExtension
}

// NewCatalogExtension creates a new CatalogExtension
func NewCatalogExtension() *CatalogExtension {
	return &CatalogExtension{
		BaseExtension: common.NewBaseExtension("catalog-extension"),
	}
}

// PreReconcile is called before reconciliation starts
func (e *CatalogExtension) PreReconcile(
	ctx context.Context,
	k8sClient client.Client,
	cr common.ClusterInterface,
) error {
	logger := log.FromContext(ctx)
	logger.Info("CatalogExtension PreReconcile", "cluster", cr.GetName())

	// Cast to TrinoCluster for type-specific operations
	trinoCR, ok := cr.(*trinov1alpha1.TrinoCluster)
	if !ok {
		return fmt.Errorf("expected *TrinoCluster, got %T", cr)
	}

	// Validate catalog configurations
	if err := e.validateCatalogs(trinoCR); err != nil {
		return fmt.Errorf("catalog validation failed: %w", err)
	}

	return nil
}

// PostReconcile is called after reconciliation completes
func (e *CatalogExtension) PostReconcile(
	ctx context.Context,
	k8sClient client.Client,
	cr common.ClusterInterface,
) error {
	logger := log.FromContext(ctx)
	logger.Info("CatalogExtension PostReconcile", "cluster", cr.GetName())

	// Cast to TrinoCluster for type-specific operations
	trinoCR, ok := cr.(*trinov1alpha1.TrinoCluster)
	if !ok {
		return fmt.Errorf("expected *TrinoCluster, got %T", cr)
	}

	// Update status with ready catalogs
	readyCatalogs := make([]string, 0, len(trinoCR.Spec.Catalogs))
	for _, catalog := range trinoCR.Spec.Catalogs {
		readyCatalogs = append(readyCatalogs, catalog.Name)
	}
	trinoCR.Status.CatalogsReady = readyCatalogs

	return nil
}

// OnReconcileError is called when reconciliation encounters an error
func (e *CatalogExtension) OnReconcileError(
	ctx context.Context,
	k8sClient client.Client,
	cr common.ClusterInterface,
	err error,
) error {
	logger := log.FromContext(ctx)
	logger.Error(err, "CatalogExtension OnReconcileError", "cluster", cr.GetName())
	return nil
}

// validateCatalogs validates the catalog configurations
func (e *CatalogExtension) validateCatalogs(cr *trinov1alpha1.TrinoCluster) error {
	seenNames := make(map[string]bool)

	for _, catalog := range cr.Spec.Catalogs {
		// Check for duplicate catalog names
		if seenNames[catalog.Name] {
			return fmt.Errorf("duplicate catalog name: %s", catalog.Name)
		}
		seenNames[catalog.Name] = true

		// Validate catalog type
		validTypes := map[string]bool{
			"hive":       true,
			"iceberg":    true,
			"kafka":      true,
			"mysql":      true,
			"postgresql": true,
			"delta":      true,
			"tpch":       true,
			"tpcds":      true,
		}

		if !validTypes[catalog.Type] {
			return fmt.Errorf("invalid catalog type: %s for catalog %s", catalog.Type, catalog.Name)
		}
	}

	return nil
}

// Ensure interface implementation
var _ common.ClusterExtension[common.ClusterInterface] = &CatalogExtension{}
