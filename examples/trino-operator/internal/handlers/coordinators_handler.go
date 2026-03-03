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

package handlers

import (
	"context"
	"fmt"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/config"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/constants"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CoordinatorsHandler handles the Coordinators role
type CoordinatorsHandler struct{}

// NewCoordinatorsHandler creates a new CoordinatorsHandler
func NewCoordinatorsHandler() *CoordinatorsHandler {
	return &CoordinatorsHandler{}
}

// BuildResources builds resources for the Coordinators role group
func (h *CoordinatorsHandler) BuildResources(
	ctx context.Context,
	k8sClient client.Client,
	cr *trinov1alpha1.TrinoCluster,
	buildCtx *reconciler.RoleGroupBuildContext,
) (*reconciler.RoleGroupResources, error) {
	// Get port from CRD spec or use default
	port := GetCoordinatorPort(cr)

	// Build ConfigMap with Trino configuration
	configMap, err := h.buildConfigMap(cr, buildCtx, port)
	if err != nil {
		return nil, fmt.Errorf("failed to build ConfigMap: %w", err)
	}

	// Build Headless Service for StatefulSet
	headlessService := BuildHeadlessService(buildCtx, port)

	// Build Service (client-facing)
	service := BuildService(buildCtx, port)

	// Get replicas
	replicas := GetReplicas(buildCtx, constants.DefaultCoordinatorReplicas)

	// Build StatefulSet using SDK builder
	statefulSet := BuildStatefulSet(
		buildCtx,
		cr,
		fmt.Sprintf("%s-config", buildCtx.ResourceName),
		port,
		replicas,
		constants.DefaultCoordinatorCPURequest,
		constants.DefaultCoordinatorCPULimit,
		constants.DefaultCoordinatorMemoryLimit,
	)

	// Build PDB if configured
	var pdb = BuildPDB(buildCtx, getRoleConfigPDB(cr.Spec.Coordinators))

	return &reconciler.RoleGroupResources{
		ConfigMap:           configMap,
		HeadlessService:     headlessService,
		Service:             service,
		StatefulSet:         statefulSet,
		PodDisruptionBudget: pdb,
	}, nil
}

// buildConfigMap builds the ConfigMap with Trino configuration
func (h *CoordinatorsHandler) buildConfigMap(
	cr *trinov1alpha1.TrinoCluster,
	buildCtx *reconciler.RoleGroupBuildContext,
	port int32,
) (*corev1.ConfigMap, error) {
	// Generate Trino configuration
	trinoConfig := config.NewTrinoConfigBuilder().
		ForCoordinator(cr, buildCtx, port).
		Build()

	// Generate JVM configuration
	jvmConfig := config.NewJVMConfigBuilder().
		ForCoordinator().
		Build()

	// Generate Catalog configurations
	catalogConfigs := config.NewCatalogConfigBuilder().
		WithCatalogs(cr.Spec.Catalogs).
		Build()

	// Build ConfigMap data
	data := map[string]string{
		"config.properties": trinoConfig,
		"jvm.config":        jvmConfig,
	}

	// Add catalog configurations
	for name, catalogData := range catalogConfigs {
		data[fmt.Sprintf("catalog/%s.properties", name)] = catalogData
	}

	// Use SDK builder to create ConfigMap
	return BuildConfigMap(buildCtx, data), nil
}

// getRoleConfigPDB extracts PDB spec from CoordinatorsSpec
func getRoleConfigPDB(spec *trinov1alpha1.CoordinatorsSpec) *v1alpha1.PodDisruptionBudgetSpec {
	if spec == nil || spec.RoleConfig == nil || spec.RoleConfig.PodDisruptionBudget == nil {
		return nil
	}
	return spec.RoleConfig.PodDisruptionBudget
}
