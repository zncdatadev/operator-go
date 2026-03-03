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
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/constants"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/handlers"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Role name constants (using plural form, following conventions)
const (
	RoleCoordinators = "coordinators"
	RoleWorkers      = "workers"
)

// TrinoRoleGroupHandler implements the operator-go RoleGroupHandler interface
// This is the key to using operator-go SDK: delegate product-specific resource building logic through this interface
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

// GetContainerImage returns the container image
func (h *TrinoRoleGroupHandler) GetContainerImage(roleName string) string {
	return "trinodb/trino:435"
}

// GetContainerPorts returns the container ports
func (h *TrinoRoleGroupHandler) GetContainerPorts(roleName, roleGroupName string) []corev1.ContainerPort {
	switch roleName {
	case RoleCoordinators, RoleWorkers:
		return []corev1.ContainerPort{
			{Name: "http", ContainerPort: constants.DefaultHTTPPort, Protocol: corev1.ProtocolTCP},
		}
	default:
		return nil
	}
}

// GetServicePorts returns the Service ports
func (h *TrinoRoleGroupHandler) GetServicePorts(roleName, roleGroupName string) []corev1.ServicePort {
	switch roleName {
	case RoleCoordinators, RoleWorkers:
		return []corev1.ServicePort{
			{
				Name:       "http",
				Port:       constants.DefaultHTTPPort,
				TargetPort: intstr.FromInt(int(constants.DefaultHTTPPort)),
				Protocol:   corev1.ProtocolTCP,
			},
		}
	default:
		return nil
	}
}

// Ensure interface implementation
var _ reconciler.RoleGroupHandler[*trinov1alpha1.TrinoCluster] = &TrinoRoleGroupHandler{}
