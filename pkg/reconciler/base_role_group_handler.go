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
	"fmt"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/config"
	"github.com/zncdatadev/operator-go/pkg/sidecar"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// BaseRoleGroupHandler provides a base implementation of RoleGroupHandler.
// Product operators can embed this struct and override methods as needed.
//
// Usage:
//
//	type HdfsRoleGroupHandler struct {
//	    reconciler.BaseRoleGroupHandler[*hdfsv1alpha1.HdfsCluster]
//	}
//
//	func (h *HdfsRoleGroupHandler) BuildResources(...) (*RoleGroupResources, error) {
//	    resources, err := h.BaseRoleGroupHandler.BuildResources(...)
//	    if err != nil {
//	        return nil, err
//	    }
//	    // Add HDFS-specific customizations
//	    return resources, nil
//	}
type BaseRoleGroupHandler[CR common.ClusterInterface] struct {
	// Image is the default container image for all roles.
	Image string

	// ImagePullPolicy is the default image pull policy.
	ImagePullPolicy corev1.PullPolicy

	// RoleImages maps role names to specific images.
	// If a role is not found here, the default Image is used.
	RoleImages map[string]string

	// RoleContainerPorts maps role names to container ports.
	RoleContainerPorts map[string][]corev1.ContainerPort

	// RoleServicePorts maps role names to service ports.
	RoleServicePorts map[string][]corev1.ServicePort

	// ConfigGenerator is used to generate configuration files.
	// Optional - if nil, config files are generated from MergedConfig only.
	ConfigGenerator *config.MultiFormatConfigGenerator

	// Scheme is the runtime scheme for ownership setup.
	Scheme *runtime.Scheme

	// Product-specific labels to add to all resources.
	ExtraLabels map[string]string

	// Product-specific annotations to add to all resources.
	ExtraAnnotations map[string]string

	// SidecarManager manages sidecar injection into pods.
	// Optional - if nil, no sidecars are injected.
	sidecarManager *sidecar.SidecarManager
}

// NewBaseRoleGroupHandler creates a new BaseRoleGroupHandler with defaults.
func NewBaseRoleGroupHandler[CR common.ClusterInterface](image string, scheme *runtime.Scheme) *BaseRoleGroupHandler[CR] {
	return &BaseRoleGroupHandler[CR]{
		Image:              image,
		ImagePullPolicy:    corev1.PullIfNotPresent,
		RoleImages:         make(map[string]string),
		RoleContainerPorts: make(map[string][]corev1.ContainerPort),
		RoleServicePorts:   make(map[string][]corev1.ServicePort),
		Scheme:             scheme,
		ExtraLabels:        make(map[string]string),
		ExtraAnnotations:   make(map[string]string),
	}
}

// WithSidecarManager sets the SidecarManager for sidecar injection.
func (h *BaseRoleGroupHandler[CR]) WithSidecarManager(m *sidecar.SidecarManager) *BaseRoleGroupHandler[CR] {
	h.sidecarManager = m
	return h
}

// BuildResources builds the default Kubernetes resources for a role group.
// This implementation creates:
// - ConfigMap from merged configuration
// - Headless Service for StatefulSet
// - Service (if ports are defined)
// - StatefulSet with standard configuration
// - PodDisruptionBudget (if configured in RoleConfig)
func (h *BaseRoleGroupHandler[CR]) BuildResources(
	ctx context.Context,
	k8sClient client.Client,
	cr CR,
	buildCtx *RoleGroupBuildContext,
) (*RoleGroupResources, error) {
	logger := log.FromContext(ctx)

	resources := &RoleGroupResources{}

	// Build labels
	labels := h.buildLabels(buildCtx)

	// Build ConfigMap
	configMap, err := h.buildConfigMap(buildCtx, labels)
	if err != nil {
		return nil, fmt.Errorf("failed to build ConfigMap: %w", err)
	}
	resources.ConfigMap = configMap

	// Build Headless Service
	headlessSvc := h.buildHeadlessService(buildCtx, labels)
	resources.HeadlessService = headlessSvc

	// Build Service (if ports are defined)
	svcPorts := h.servicePorts(buildCtx.RoleName, buildCtx.RoleGroupName)
	if len(svcPorts) > 0 {
		resources.Service = h.buildService(buildCtx, labels, svcPorts)
	}

	// Build StatefulSet
	sts, err := h.buildStatefulSet(ctx, k8sClient, cr, buildCtx, labels)
	if err != nil {
		return nil, fmt.Errorf("failed to build StatefulSet: %w", err)
	}
	resources.StatefulSet = sts

	// Build PodDisruptionBudget
	pdb := h.buildPodDisruptionBudget(buildCtx, labels)
	if pdb != nil {
		resources.PodDisruptionBudget = pdb
	}

	logger.V(1).Info("Built role group resources",
		"role", buildCtx.RoleName,
		"group", buildCtx.RoleGroupName,
		"resourceName", buildCtx.ResourceName)

	return resources, nil
}

// containerImage returns the container image for a role.
func (h *BaseRoleGroupHandler[CR]) containerImage(roleName string) string {
	if image, ok := h.RoleImages[roleName]; ok {
		return image
	}
	return h.Image
}

// containerPorts returns the container ports for a role group.
func (h *BaseRoleGroupHandler[CR]) containerPorts(roleName, _ string) []corev1.ContainerPort {
	if ports, ok := h.RoleContainerPorts[roleName]; ok {
		return ports
	}
	return nil
}

// servicePorts returns the service ports for a role group.
func (h *BaseRoleGroupHandler[CR]) servicePorts(roleName, _ string) []corev1.ServicePort {
	if ports, ok := h.RoleServicePorts[roleName]; ok {
		return ports
	}
	return nil
}

// buildLabels creates the labels for resources.
func (h *BaseRoleGroupHandler[CR]) buildLabels(buildCtx *RoleGroupBuildContext) map[string]string {
	labels := make(map[string]string)

	// Add cluster labels
	for k, v := range buildCtx.ClusterLabels {
		labels[k] = v
	}

	// Add standard labels
	labels["app.kubernetes.io/instance"] = buildCtx.ClusterName
	labels["app.kubernetes.io/component"] = buildCtx.RoleName
	labels["app.kubernetes.io/managed-by"] = "operator-go"

	// Role group label
	labels[buildCtx.ClusterName+"-"+buildCtx.RoleGroupName] = "true"

	// Add extra labels
	for k, v := range h.ExtraLabels {
		labels[k] = v
	}

	return labels
}

// buildAnnotations creates the annotations for resources.
func (h *BaseRoleGroupHandler[CR]) buildAnnotations(_ *RoleGroupBuildContext) map[string]string {
	annotations := make(map[string]string)

	// Add extra annotations
	for k, v := range h.ExtraAnnotations {
		annotations[k] = v
	}

	return annotations
}

// buildConfigMap creates the ConfigMap for the role group.
func (h *BaseRoleGroupHandler[CR]) buildConfigMap(buildCtx *RoleGroupBuildContext, labels map[string]string) (*corev1.ConfigMap, error) {
	// Build config data
	data := make(map[string]string)

	// Add config files from merged config
	for filename, cfg := range buildCtx.MergedConfig.ConfigFiles {
		// Convert config map to string format
		data[filename] = h.configMapToString(cfg)
	}

	// Use ConfigGenerator if available
	if h.ConfigGenerator != nil && len(buildCtx.MergedConfig.ConfigFiles) > 0 {
		generatedData, err := h.ConfigGenerator.GenerateFiles(buildCtx.MergedConfig.ConfigFiles)
		if err != nil {
			return nil, err
		}
		for filename, content := range generatedData {
			data[filename] = content
		}
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        buildCtx.ResourceName,
			Namespace:   buildCtx.ClusterNamespace,
			Labels:      labels,
			Annotations: h.buildAnnotations(buildCtx),
		},
		Data: data,
	}, nil
}

// configMapToString converts a config map to a string representation.
func (h *BaseRoleGroupHandler[CR]) configMapToString(cfg map[string]string) string {
	var result string
	for k, v := range cfg {
		result += fmt.Sprintf("%s=%s\n", k, v)
	}
	return result
}

// buildHeadlessService creates the headless service for StatefulSet.
func (h *BaseRoleGroupHandler[CR]) buildHeadlessService(buildCtx *RoleGroupBuildContext, labels map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        buildCtx.ResourceName + "-headless",
			Namespace:   buildCtx.ClusterNamespace,
			Labels:      labels,
			Annotations: h.buildAnnotations(buildCtx),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			Selector:  labels,
			Ports:     h.servicePorts(buildCtx.RoleName, buildCtx.RoleGroupName),
		},
	}
}

// buildService creates the client-facing service.
func (h *BaseRoleGroupHandler[CR]) buildService(buildCtx *RoleGroupBuildContext, labels map[string]string, ports []corev1.ServicePort) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        buildCtx.ResourceName,
			Namespace:   buildCtx.ClusterNamespace,
			Labels:      labels,
			Annotations: h.buildAnnotations(buildCtx),
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports:    ports,
		},
	}
}

// buildStatefulSet creates the StatefulSet for the role group.
func (h *BaseRoleGroupHandler[CR]) buildStatefulSet(
	ctx context.Context,
	_ client.Client,
	_ CR,
	buildCtx *RoleGroupBuildContext,
	labels map[string]string,
) (*appsv1.StatefulSet, error) {
	// Use the builder pattern from the existing codebase
	stsBuilder := builder.NewStatefulSetBuilder(buildCtx.ResourceName, buildCtx.ClusterNamespace)

	// Set basic properties
	stsBuilder.WithLabels(labels).
		WithAnnotations(h.buildAnnotations(buildCtx)).
		WithReplicas(buildCtx.RoleGroupSpec.GetReplicas()).
		WithImage(h.containerImage(buildCtx.RoleName), h.ImagePullPolicy).
		WithConfig(buildCtx.MergedConfig).
		WithPorts(h.containerPorts(buildCtx.RoleName, buildCtx.RoleGroupName))

	// Set resources if configured
	roleGroupConfig := buildCtx.RoleGroupSpec.GetConfig()
	if roleGroupConfig != nil && roleGroupConfig.Resources != nil {
		stsBuilder.WithResources(roleGroupConfig.Resources)
	}

	// Set pod overrides if present
	if buildCtx.MergedConfig.PodOverrides != nil {
		stsBuilder.WithPodOverrides(buildCtx.MergedConfig.PodOverrides)
	}

	// Add config volume if ConfigMap exists
	if buildCtx.MergedConfig != nil && len(buildCtx.MergedConfig.ConfigFiles) > 0 {
		stsBuilder.AddVolume(corev1.Volume{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: buildCtx.ResourceName,
					},
				},
			},
		})
		stsBuilder.AddVolumeMount(corev1.VolumeMount{
			Name:      "config",
			MountPath: "/etc/config",
			ReadOnly:  true,
		})
	}

	// Build the StatefulSet
	sts := stsBuilder.Build()

	// Inject sidecars: prefer buildCtx (SDK auto-created), fallback to instance field
	sidecarMgr := buildCtx.SidecarManager
	if sidecarMgr == nil {
		sidecarMgr = h.sidecarManager
	}
	if sidecarMgr != nil {
		if err := sidecarMgr.InjectAll(&sts.Spec.Template.Spec); err != nil {
			return nil, fmt.Errorf("sidecar injection failed: %w", err)
		}
	}

	return sts, nil
}

// buildPodDisruptionBudget creates the PDB for the role group.
func (h *BaseRoleGroupHandler[CR]) buildPodDisruptionBudget(buildCtx *RoleGroupBuildContext, labels map[string]string) *policyv1.PodDisruptionBudget {
	// Check if PDB is configured in RoleConfig
	roleConfig := buildCtx.RoleSpec.GetRoleConfig()
	if roleConfig == nil || roleConfig.PodDisruptionBudget == nil {
		return nil
	}

	pdbSpec := roleConfig.PodDisruptionBudget

	// Check if PDB is enabled (default is true if Enabled is not set)
	if !pdbSpec.Enabled {
		return nil
	}

	// Build PDB spec
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:        buildCtx.ResourceName,
			Namespace:   buildCtx.ClusterNamespace,
			Labels:      labels,
			Annotations: h.buildAnnotations(buildCtx),
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
		},
	}

	// Set max unavailable (only option available in PodDisruptionBudgetSpec)
	if pdbSpec.MaxUnavailable != nil {
		pdb.Spec.MaxUnavailable = pdbSpec.MaxUnavailable
	}

	return pdb
}

// SetRoleImage sets the image for a specific role.
func (h *BaseRoleGroupHandler[CR]) SetRoleImage(roleName, image string) {
	if h.RoleImages == nil {
		h.RoleImages = make(map[string]string)
	}
	h.RoleImages[roleName] = image
}

// SetRoleContainerPorts sets the container ports for a specific role.
func (h *BaseRoleGroupHandler[CR]) SetRoleContainerPorts(roleName string, ports []corev1.ContainerPort) {
	if h.RoleContainerPorts == nil {
		h.RoleContainerPorts = make(map[string][]corev1.ContainerPort)
	}
	h.RoleContainerPorts[roleName] = ports
}

// SetRoleServicePorts sets the service ports for a specific role.
func (h *BaseRoleGroupHandler[CR]) SetRoleServicePorts(roleName string, ports []corev1.ServicePort) {
	if h.RoleServicePorts == nil {
		h.RoleServicePorts = make(map[string][]corev1.ServicePort)
	}
	h.RoleServicePorts[roleName] = ports
}

// FetchConfigMap fetches a ConfigMap from the cluster.
func (h *BaseRoleGroupHandler[CR]) FetchConfigMap(ctx context.Context, k8sClient client.Client, namespace, name string) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	key := types.NamespacedName{Namespace: namespace, Name: name}
	if err := k8sClient.Get(ctx, key, cm); err != nil {
		return nil, err
	}
	return cm, nil
}

// FetchSecret fetches a Secret from the cluster.
func (h *BaseRoleGroupHandler[CR]) FetchSecret(ctx context.Context, k8sClient client.Client, namespace, name string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: namespace, Name: name}
	if err := k8sClient.Get(ctx, key, secret); err != nil {
		return nil, err
	}
	return secret, nil
}

// Verify that BaseRoleGroupHandler implements RoleGroupHandler.
var _ RoleGroupHandler[common.ClusterInterface] = &BaseRoleGroupHandler[common.ClusterInterface]{}
