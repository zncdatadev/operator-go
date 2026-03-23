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

package controller

import (
	"context"
	"fmt"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/handlers"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Role name constants (using plural form, following conventions)
const (
	RoleCoordinators = "coordinators"
	RoleWorkers      = "workers"
)

// TrinoRoleGroupHandler implements the operator-go RoleGroupHandler interface.
// It routes BuildResources calls to role-specific handlers (CoordinatorsHandler / WorkersHandler)
// which build all Kubernetes resources directly using the SDK builder utilities.
type TrinoRoleGroupHandler struct {
	coordinatorsHandler *handlers.CoordinatorsHandler
	workersHandler      *handlers.WorkersHandler
}

// NewTrinoRoleGroupHandler creates a new Handler
func NewTrinoRoleGroupHandler() *TrinoRoleGroupHandler {
	return &TrinoRoleGroupHandler{
		coordinatorsHandler: handlers.NewCoordinatorsHandler(),
		workersHandler:      handlers.NewWorkersHandler(),
	}
}

// BuildResources implements the RoleGroupHandler interface
// GenericReconciler calls this method to build resources for each RoleGroup
func (h *TrinoRoleGroupHandler) BuildResources(
	ctx context.Context,
	k8sClient client.Client,
	cr *trinov1alpha1.TrinoCluster,
	buildCtx *reconciler.RoleGroupBuildContext,
) (*reconciler.RoleGroupResources, error) {
	// Route to the corresponding handler based on role type
	switch buildCtx.RoleName {
	case RoleCoordinators:
		return h.coordinatorsHandler.BuildResources(ctx, k8sClient, cr, buildCtx)
	case RoleWorkers:
		return h.workersHandler.BuildResources(ctx, k8sClient, cr, buildCtx)
	default:
		return nil, fmt.Errorf("unknown role: %s", buildCtx.RoleName)
	}
}

// Ensure interface implementation
var _ reconciler.RoleGroupHandler[*trinov1alpha1.TrinoCluster] = &TrinoRoleGroupHandler{}
