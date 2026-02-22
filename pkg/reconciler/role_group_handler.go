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

package reconciler

import (
	"context"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RoleGroupResources contains all Kubernetes resources for a role group.
// Each role group maps to exactly one StatefulSet and its associated resources.
type RoleGroupResources struct {
	// StatefulSet is the main workload resource.
	StatefulSet *appsv1.StatefulSet

	// ConfigMap contains configuration files for the role group.
	ConfigMap *corev1.ConfigMap

	// Service is the client-facing service (optional).
	Service *corev1.Service

	// HeadlessService is the headless service for StatefulSet network identity.
	HeadlessService *corev1.Service

	// PodDisruptionBudget controls pod eviction (optional).
	PodDisruptionBudget *policyv1.PodDisruptionBudget
}

// RoleGroupBuildContext provides context for building role group resources.
// It contains all the information needed to construct Kubernetes resources.
type RoleGroupBuildContext struct {
	// ClusterName is the name of the cluster CR.
	ClusterName string

	// ClusterNamespace is the namespace of the cluster CR.
	ClusterNamespace string

	// ClusterLabels are the labels from the cluster CR.
	ClusterLabels map[string]string

	// ClusterSpec is the generic cluster specification.
	ClusterSpec *v1alpha1.GenericClusterSpec

	// RoleName is the name of the role (e.g., "namenode", "datanode").
	RoleName string

	// RoleSpec is the role specification.
	RoleSpec *v1alpha1.RoleSpec

	// RoleGroupName is the name of the role group.
	RoleGroupName string

	// RoleGroupSpec is the role group specification.
	RoleGroupSpec v1alpha1.RoleGroupSpec

	// MergedConfig is the merged configuration from role and role group overrides.
	MergedConfig *config.MergedConfig

	// ResourceName is the derived resource name: {cluster}-{group}.
	ResourceName string
}

// RoleGroupHandler is the interface that product operators must implement
// to define how resources are built for each role group.
//
// The GenericReconciler handles the "when" and "how to apply" resources,
// while the RoleGroupHandler handles the "what" - the product-specific resource definitions.
type RoleGroupHandler[CR common.ClusterInterface] interface {
	// BuildResources builds all Kubernetes resources for a role group.
	// The GenericReconciler will apply these resources in the correct order.
	//
	// Implementations should:
	// 1. Use the build context to get cluster info, labels, and merged config
	// 2. Build product-specific ConfigMap data
	// 3. Build StatefulSet with appropriate containers, volumes, etc.
	// 4. Build Services if needed
	// 5. Build PDB if needed
	//
	// Returns RoleGroupResources containing all built resources, or an error.
	BuildResources(ctx context.Context, k8sClient client.Client, cr CR, buildCtx *RoleGroupBuildContext) (*RoleGroupResources, error)

	// GetContainerImage returns the container image for a given role.
	// This allows different roles to use different images.
	GetContainerImage(roleName string) string

	// GetContainerPorts returns the container ports for a role group.
	// These ports are used for the main container in the StatefulSet.
	GetContainerPorts(roleName, roleGroupName string) []corev1.ContainerPort

	// GetServicePorts returns the service ports for a role group.
	// These ports are exposed by the Service (if one is created).
	GetServicePorts(roleName, roleGroupName string) []corev1.ServicePort
}

// RoleGroupHandlerFuncs is an adapter to allow using functions as RoleGroupHandler.
// This is useful for simple handlers that don't need a full struct.
type RoleGroupHandlerFuncs[CR common.ClusterInterface] struct {
	// BuildResourcesFunc is the function for building resources.
	BuildResourcesFunc func(ctx context.Context, k8sClient client.Client, cr CR, buildCtx *RoleGroupBuildContext) (*RoleGroupResources, error)

	// GetContainerImageFunc is the function for getting container images.
	GetContainerImageFunc func(roleName string) string

	// GetContainerPortsFunc is the function for getting container ports.
	GetContainerPortsFunc func(roleName, roleGroupName string) []corev1.ContainerPort

	// GetServicePortsFunc is the function for getting service ports.
	GetServicePortsFunc func(roleName, roleGroupName string) []corev1.ServicePort
}

// BuildResources implements RoleGroupHandler.
func (f *RoleGroupHandlerFuncs[CR]) BuildResources(ctx context.Context, k8sClient client.Client, cr CR, buildCtx *RoleGroupBuildContext) (*RoleGroupResources, error) {
	if f.BuildResourcesFunc == nil {
		return &RoleGroupResources{}, nil
	}
	return f.BuildResourcesFunc(ctx, k8sClient, cr, buildCtx)
}

// GetContainerImage implements RoleGroupHandler.
func (f *RoleGroupHandlerFuncs[CR]) GetContainerImage(roleName string) string {
	if f.GetContainerImageFunc == nil {
		return ""
	}
	return f.GetContainerImageFunc(roleName)
}

// GetContainerPorts implements RoleGroupHandler.
func (f *RoleGroupHandlerFuncs[CR]) GetContainerPorts(roleName, roleGroupName string) []corev1.ContainerPort {
	if f.GetContainerPortsFunc == nil {
		return nil
	}
	return f.GetContainerPortsFunc(roleName, roleGroupName)
}

// GetServicePorts implements RoleGroupHandler.
func (f *RoleGroupHandlerFuncs[CR]) GetServicePorts(roleName, roleGroupName string) []corev1.ServicePort {
	if f.GetServicePortsFunc == nil {
		return nil
	}
	return f.GetServicePortsFunc(roleName, roleGroupName)
}

// Verify that RoleGroupHandlerFuncs implements RoleGroupHandler.
var _ RoleGroupHandler[common.ClusterInterface] = &RoleGroupHandlerFuncs[common.ClusterInterface]{}
