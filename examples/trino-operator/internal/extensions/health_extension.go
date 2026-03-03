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

// HealthExtension is a RoleExtension that performs health checks for Trino roles
// This demonstrates the RoleExtension extension point of operator-go SDK
type HealthExtension struct {
	common.BaseExtension
}

// NewHealthExtension creates a new HealthExtension
func NewHealthExtension() *HealthExtension {
	return &HealthExtension{
		BaseExtension: common.NewBaseExtension("health-extension"),
	}
}

// PreReconcile is called before role reconciliation starts
func (e *HealthExtension) PreReconcile(
	ctx context.Context,
	k8sClient client.Client,
	cr common.ClusterInterface,
	roleName string,
) error {
	logger := log.FromContext(ctx)
	logger.Info("HealthExtension PreReconcile", "cluster", cr.GetName(), "role", roleName)
	return nil
}

// PostReconcile is called after role reconciliation completes
func (e *HealthExtension) PostReconcile(
	ctx context.Context,
	k8sClient client.Client,
	cr common.ClusterInterface,
	roleName string,
) error {
	logger := log.FromContext(ctx)
	logger.Info("HealthExtension PostReconcile", "cluster", cr.GetName(), "role", roleName)

	// Cast to TrinoCluster for type-specific operations
	trinoCR, ok := cr.(*trinov1alpha1.TrinoCluster)
	if !ok {
		err := fmt.Errorf("expected *TrinoCluster, got %T", cr)
		logger.Error(err, "type assertion failed")
		return err
	}

	// Perform role-specific health checks
	switch roleName {
	case "coordinators":
		e.checkCoordinatorHealth(ctx, trinoCR)
	case "workers":
		e.checkWorkerHealth(ctx, trinoCR)
	}

	return nil
}

// checkCoordinatorHealth checks the health of the coordinator role
// TODO: Implement actual health checks:
// - HTTP endpoint availability
// - Query processing capability
// - Worker registration status
func (e *HealthExtension) checkCoordinatorHealth(ctx context.Context, cr *trinov1alpha1.TrinoCluster) {
	logger := log.FromContext(ctx)
	logger.Info("Health check not yet implemented - checking coordinator health", "cluster", cr.Name)
}

// checkWorkerHealth checks the health of the worker role
// TODO: Implement actual health checks:
// - Worker registration with coordinator
// - Task execution capability
// - Memory/CPU usage
func (e *HealthExtension) checkWorkerHealth(ctx context.Context, cr *trinov1alpha1.TrinoCluster) {
	logger := log.FromContext(ctx)
	logger.Info("Health check not yet implemented - checking worker health", "cluster", cr.Name)
}

// Ensure interface implementation
var _ common.RoleExtension[common.ClusterInterface] = &HealthExtension{}
