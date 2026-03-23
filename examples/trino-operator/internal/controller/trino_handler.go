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
// This is the key to using operator-go SDK: delegate product-specific resource building logic through this interface.
//
// Note: GetContainerImage, GetContainerPorts, and GetServicePorts are required by the RoleGroupHandler
// interface because BaseRoleGroupHandler uses them internally when building StatefulSets and Services.
// This handler builds resources directly in CoordinatorsHandler/WorkersHandler without delegating to
// BaseRoleGroupHandler, so those methods are not invoked by the SDK reconciliation loop here.
// They are retained for interface compliance and would be used if this handler were refactored to
// extend BaseRoleGroupHandler.
type TrinoRoleGroupHandler struct {
	coordinatorsHandler *handlers.CoordinatorsHandler
	workersHandler      *handlers.WorkersHandler
	defaultImage        string
}

// NewTrinoRoleGroupHandler creates a new Handler.
// defaultImage is returned by GetContainerImage and should match the CRD default
// (i.e. constants.DefaultImage). At runtime the actual image comes from cr.Spec.Image
// which is applied directly inside BuildStatefulSet, so this value acts as a
// consistent fallback for any caller that queries the handler before reconciling.
func NewTrinoRoleGroupHandler(defaultImage string) *TrinoRoleGroupHandler {
	return &TrinoRoleGroupHandler{
		coordinatorsHandler: handlers.NewCoordinatorsHandler(),
		workersHandler:      handlers.NewWorkersHandler(),
		defaultImage:        defaultImage,
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

// GetContainerImage returns the default container image for the given role.
// The actual image used at runtime is taken from cr.Spec.Image inside BuildResources;
// this method provides a consistent fallback for callers such as BaseRoleGroupHandler.
func (h *TrinoRoleGroupHandler) GetContainerImage(roleName string) string {
	return h.defaultImage
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
