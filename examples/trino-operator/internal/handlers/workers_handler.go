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

// WorkersHandler handles the Workers role
type WorkersHandler struct{}

// NewWorkersHandler creates a new WorkersHandler
func NewWorkersHandler() *WorkersHandler {
	return &WorkersHandler{}
}

// BuildResources builds resources for the Workers role group
func (h *WorkersHandler) BuildResources(
	ctx context.Context,
	k8sClient client.Client,
	cr *trinov1alpha1.TrinoCluster,
	buildCtx *reconciler.RoleGroupBuildContext,
) (*reconciler.RoleGroupResources, error) {
	// Get ports from CRD spec or use defaults
	coordinatorPort := GetCoordinatorPort(cr)
	workerPort := GetWorkerPort(cr)

	// Build ConfigMap with Trino configuration
	configMap := h.buildConfigMap(cr, buildCtx, coordinatorPort)

	// Build Headless Service for StatefulSet
	headlessService := BuildHeadlessService(buildCtx, workerPort)

	// Get replicas
	replicas := GetReplicas(buildCtx, constants.DefaultWorkerReplicas)

	// Build StatefulSet using SDK builder
	statefulSet := BuildStatefulSet(
		buildCtx,
		cr,
		fmt.Sprintf("%s-config", buildCtx.ResourceName),
		workerPort,
		replicas,
		constants.DefaultWorkerCPURequest,
		constants.DefaultWorkerCPULimit,
		constants.DefaultWorkerMemoryLimit,
	)

	// Build PDB if configured
	var pdb = BuildPDB(buildCtx, getWorkerRoleConfigPDB(cr.Spec.Workers))

	return &reconciler.RoleGroupResources{
		ConfigMap:           configMap,
		HeadlessService:     headlessService,
		StatefulSet:         statefulSet,
		PodDisruptionBudget: pdb,
	}, nil
}

// buildConfigMap builds the ConfigMap with Trino configuration
func (h *WorkersHandler) buildConfigMap(
	cr *trinov1alpha1.TrinoCluster,
	buildCtx *reconciler.RoleGroupBuildContext,
	coordinatorPort int32,
) *corev1.ConfigMap {
	// Generate Trino configuration for Worker
	trinoConfig := config.NewTrinoConfigBuilder().
		ForWorker(cr, buildCtx, coordinatorPort).
		Build()

	// Generate JVM configuration
	jvmConfig := config.NewJVMConfigBuilder().
		ForWorker().
		Build()

	// Build ConfigMap data
	data := map[string]string{
		"config.properties": trinoConfig,
		"jvm.config":        jvmConfig,
	}

	// Use SDK builder to create ConfigMap
	return BuildConfigMap(buildCtx, data)
}

// getWorkerRoleConfigPDB extracts PDB spec from WorkersSpec
func getWorkerRoleConfigPDB(spec *trinov1alpha1.WorkersSpec) *v1alpha1.PodDisruptionBudgetSpec {
	if spec == nil || spec.RoleConfig == nil || spec.RoleConfig.PodDisruptionBudget == nil {
		return nil
	}
	return spec.RoleConfig.PodDisruptionBudget
}
