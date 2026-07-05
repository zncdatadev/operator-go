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
	"github.com/zncdatadev/operator-go/pkg/constant"
	"github.com/zncdatadev/operator-go/pkg/productlogging"
	"github.com/zncdatadev/operator-go/pkg/security"
	"github.com/zncdatadev/operator-go/pkg/sidecar"
	"github.com/zncdatadev/operator-go/pkg/vector"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
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

	// StorageMountPath, when non-empty, opts the role group into a data PVC. The base
	// StatefulSet then gets a VolumeClaimTemplate built from RoleGroupConfig.Resources.Storage
	// mounted at this path, keeping the volume/mount contract consistent. Left empty for
	// backward compatibility (no data PVC unless a product opts in).
	StorageMountPath string

	// ConfigMountPath is where the generated config ConfigMap is mounted in the primary
	// container. Products whose application reads config from a specific directory (e.g.
	// "/etc/trino") set this. Defaults to the kubedoop-canonical config mount path
	// (constant.KubedoopConfigDirMount) when empty.
	ConfigMountPath string

	// MainContainerName, when set, renames the primary (first) container of the StatefulSet.
	// Products use this when the container name is significant — e.g. it must match the
	// per-container logging key (logging.containers.<name>) declared in LoggingContainers.
	// Defaults to the resource name (set by the StatefulSet builder) when empty.
	MainContainerName string

	// PublishNotReadyAddresses, when true, sets the same flag on the headless Service so
	// peers can resolve each other's DNS before readiness (required by quorum systems).
	PublishNotReadyAddresses bool

	// LabelDomain, when set (e.g. "zookeeper.kubedoop.dev"), enables product-owned identity
	// labels — "<domain>/cluster", "<domain>/role", "<domain>/role-group" — that are used
	// for resource selectors (StatefulSet, Services, PDB) instead of the descriptive
	// app.kubernetes.io/* labels. The product-domain prefix guarantees the selectors never
	// match another product's pods, and decoupling from app.kubernetes.io/* keeps the
	// immutable StatefulSet selector stable and free of user-mutable labels.
	// When empty, selectors fall back to the descriptive labels (backward compatible).
	LabelDomain string

	// LoggingContainers declares, per container, how its logging config file is generated
	// from the deep-merged CRD logging spec and injected into the role group ConfigMap.
	// The framework owns the whole pipeline (merge -> convert -> render -> ConfigMap key);
	// products only declare the product-specific bits (framework, pattern). Empty means the
	// product handles logging itself (or has none).
	//
	// LoggingContainers also names the producers of the Vector log pipeline. When the role group
	// enables the Vector agent, the GenericReconciler passes these container names to the Vector
	// sidecar provider (via LoggingProducers()), which is the single owner of the shared log
	// volume: it creates the size-limited log emptyDir, RW-mounts it on each producer container,
	// and RO-mounts it on itself. When Vector is disabled, no shared volume exists and no file
	// appender is emitted (console-only).
	LoggingContainers []productlogging.ContainerLogging

	// LogVolumeSize overrides the SizeLimit of the shared log emptyDir. Empty uses
	// vector.DefaultLogVolumeSize. The GenericReconciler forwards it to the Vector provider,
	// which owns the volume (always a node-disk emptyDir, never medium=Memory, never a PVC).
	LogVolumeSize string

	// SidecarManager manages sidecar injection into pods.
	// Optional - if nil, no sidecars are injected.
	sidecarManager *sidecar.SidecarManager

	// securityContextConfigured tracks whether the security context fields below were set
	// explicitly (including to nil to disable the default). When false, the framework applies
	// its canonical default security context (the kubedoop org-standard 1001 identity plus
	// hardening — see pkg/security.BuildDefault*SecurityContext).
	securityContextConfigured bool

	// containerSecurityContext is the container-level security context applied to the main
	// container. When the framework default is in effect, this is
	// security.NewPodSecurityBuilder().BuildDefaultSecurityContext(). Products override it via
	// WithSecurityContext (or disable it via WithoutDefaultSecurityContext). Per-role-group
	// customization goes through MergedConfig.PodOverrides, which REPLACES the security context
	// (no deep merge — a product overriding must restate any fields it wants to keep).
	containerSecurityContext *corev1.SecurityContext

	// podSecurityContext is the pod-level security context applied to the pod spec. See
	// containerSecurityContext for override semantics.
	podSecurityContext *corev1.PodSecurityContext
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

// WithSecurityContext overrides the framework's default pod/container security context for the
// role group's StatefulSet. Passing nil for either argument removes that level's security
// context entirely (the framework default is no longer applied). For per-role-group overrides,
// prefer MergedConfig.PodOverrides instead of this handler-wide override; note that PodOverrides
// REPLACES the security context (no deep merge), so an override must restate any fields it wants
// to keep.
func (h *BaseRoleGroupHandler[CR]) WithSecurityContext(
	containerCtx *corev1.SecurityContext,
	podCtx *corev1.PodSecurityContext,
) *BaseRoleGroupHandler[CR] {
	h.containerSecurityContext = containerCtx
	h.podSecurityContext = podCtx
	h.securityContextConfigured = true
	return h
}

// WithoutDefaultSecurityContext disables the framework's default security context, so the
// StatefulSet is built with no pod/container security context unless one is supplied via
// MergedConfig.PodOverrides.
func (h *BaseRoleGroupHandler[CR]) WithoutDefaultSecurityContext() *BaseRoleGroupHandler[CR] {
	return h.WithSecurityContext(nil, nil)
}

// resolveSecurityContext returns the container- and pod-level security contexts to apply. When
// the product has not configured them, the framework's canonical default is used: the kubedoop
// org-standard 1001 identity plus hardening (see pkg/security.BuildDefaultSecurityContext /
// BuildDefaultPodSecurityContext).
func (h *BaseRoleGroupHandler[CR]) resolveSecurityContext() (*corev1.SecurityContext, *corev1.PodSecurityContext) {
	if h.securityContextConfigured {
		return h.containerSecurityContext, h.podSecurityContext
	}
	builder := security.NewPodSecurityBuilder()
	return builder.BuildDefaultSecurityContext(), builder.BuildDefaultPodSecurityContext()
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

// vectorEnabledFor reports whether the Vector agent is enabled for this role group, based on
// the deep-merged logging spec. It is the single source of truth for both the producer (shared
// log volume + RW mounts) and Option A (file-appender gating).
func vectorEnabledFor(buildCtx *RoleGroupBuildContext) bool {
	if buildCtx == nil || buildCtx.MergedConfig == nil {
		return false
	}
	// vector.IsAgentEnabled is the single, shared predicate used by both this producer side and
	// the consumer side (generic_reconciler.buildSidecarManager), so they can never drift.
	return vector.IsAgentEnabled(buildCtx.MergedConfig.Logging)
}

// LoggingProducers implements LoggingProducerProvider: it exposes the handler's declared
// log-producer containers so the GenericReconciler can configure the Vector sidecar (the single
// owner of the shared log volume) without reaching into handler internals.
func (h *BaseRoleGroupHandler[CR]) LoggingProducers() []productlogging.ContainerLogging {
	return h.LoggingContainers
}

// LogVolumeSizeLimit implements LoggingProducerProvider: the shared log volume SizeLimit override
// ("" uses vector.DefaultLogVolumeSize).
func (h *BaseRoleGroupHandler[CR]) LogVolumeSizeLimit() string {
	return h.LogVolumeSize
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

// ClusterLabelKey returns the identity label key for the cluster, under the given domain.
func ClusterLabelKey(domain string) string { return domain + "/cluster" }

// RoleLabelKey returns the identity label key for the role, under the given domain.
func RoleLabelKey(domain string) string { return domain + "/role" }

// RoleGroupLabelKey returns the identity label key for the role group, under the given domain.
func RoleGroupLabelKey(domain string) string { return domain + "/role-group" }

// buildLabels creates the descriptive labels for resources. When LabelDomain is set, the
// product-owned identity labels are added too (they are also used as selectors — see
// buildSelectorLabels).
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

	// Product-owned identity labels (also used for selectors).
	for k, v := range h.identityLabels(buildCtx) {
		labels[k] = v
	}

	// Add extra labels
	for k, v := range h.ExtraLabels {
		labels[k] = v
	}

	return labels
}

// identityLabels returns the product-owned identity labels when LabelDomain is set, else nil.
func (h *BaseRoleGroupHandler[CR]) identityLabels(buildCtx *RoleGroupBuildContext) map[string]string {
	if h.LabelDomain == "" {
		return nil
	}
	return map[string]string{
		ClusterLabelKey(h.LabelDomain):   buildCtx.ClusterName,
		RoleLabelKey(h.LabelDomain):      buildCtx.RoleName,
		RoleGroupLabelKey(h.LabelDomain): buildCtx.RoleGroupName,
	}
}

// buildSelectorLabels returns the labels used for resource selectors. When LabelDomain is
// set, these are the product-owned identity labels (cluster + role + role-group), which are
// product-namespaced and stable. Otherwise it falls back to the full descriptive labels for
// backward compatibility.
func (h *BaseRoleGroupHandler[CR]) buildSelectorLabels(buildCtx *RoleGroupBuildContext) map[string]string {
	if h.LabelDomain == "" {
		return h.buildLabels(buildCtx)
	}
	return h.identityLabels(buildCtx)
}

// SelectorLabels exposes the role group's selector labels so embedding products can build
// matching selectors for resources they add themselves (e.g. a metrics Service).
func (h *BaseRoleGroupHandler[CR]) SelectorLabels(buildCtx *RoleGroupBuildContext) map[string]string {
	return h.buildSelectorLabels(buildCtx)
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

// configMountPath returns the directory where the config ConfigMap is mounted, defaulting
// to the kubedoop-canonical config mount path (constant.KubedoopConfigDirMount) when the
// product did not set ConfigMountPath.
func (h *BaseRoleGroupHandler[CR]) configMountPath() string {
	if h.ConfigMountPath != "" {
		return h.ConfigMountPath
	}
	return constant.KubedoopConfigDirMount
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

	// Generate the logging-related ConfigMap entries: one config file per declared producer plus
	// vector.yaml when the Vector agent is enabled (RenderLoggingConfigMapData owns the file
	// appender and vector.yaml gating). Fail fast on a key collision rather than silently
	// overwriting a file the product already produced (e.g. via MergedConfig.ConfigFiles).
	loggingData, err := RenderLoggingConfigMapData(buildCtx, h.LoggingContainers)
	if err != nil {
		return nil, err
	}
	for filename, content := range loggingData {
		if _, exists := data[filename]; exists {
			return nil, fmt.Errorf("logging config file %q collides with an existing ConfigMap key", filename)
		}
		data[filename] = content
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
			ClusterIP:                corev1.ClusterIPNone,
			PublishNotReadyAddresses: h.PublishNotReadyAddresses,
			Selector:                 h.buildSelectorLabels(buildCtx),
			Ports:                    h.servicePorts(buildCtx.RoleName, buildCtx.RoleGroupName),
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
			Selector: h.buildSelectorLabels(buildCtx),
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
		WithSelectorLabels(h.buildSelectorLabels(buildCtx)).
		WithAnnotations(h.buildAnnotations(buildCtx)).
		WithReplicas(buildCtx.RoleGroupSpec.GetReplicas()).
		WithImage(h.containerImage(buildCtx.RoleName), h.ImagePullPolicy).
		WithConfig(buildCtx.MergedConfig).
		WithPorts(h.containerPorts(buildCtx.RoleName, buildCtx.RoleGroupName))

	// Bind the reconciler-managed ServiceAccount to the pod template when configured, so the
	// created SA is actually used. Empty leaves ServiceAccountName unset (pods use the namespace
	// default SA), preserving backward compatibility.
	if buildCtx.ServiceAccountName != "" {
		stsBuilder.WithServiceAccount(buildCtx.ServiceAccountName)
	}

	// Set resources if configured
	roleGroupConfig := buildCtx.RoleGroupSpec.GetConfig()
	if roleGroupConfig != nil && roleGroupConfig.Resources != nil {
		stsBuilder.WithResources(roleGroupConfig.Resources)
		// Opt-in data PVC: when a product sets StorageMountPath, build a VolumeClaimTemplate
		// from the configured storage and mount it, so the volume/mount contract is enforced
		// by the builder instead of being hand-assembled by each product.
		if h.StorageMountPath != "" && roleGroupConfig.Resources.Storage != nil {
			stsBuilder.WithStorage(roleGroupConfig.Resources.Storage, h.StorageMountPath)
		}
	}

	// Apply the security context (framework canonical default unless the product overrode it).
	// This is set before pod overrides are applied so that any security context supplied via
	// MergedConfig.PodOverrides takes precedence over (replaces) the default.
	containerSecurityCtx, podSecurityCtx := h.resolveSecurityContext()
	stsBuilder.WithSecurityContext(containerSecurityCtx, podSecurityCtx)

	// Default enableServiceLinks to false — the kubedoop standard. Products use DNS + config and
	// never the <SVC>_SERVICE_HOST/PORT env vars kubelet injects for every Service in the
	// namespace, which only bloat env and slow startup. This is set before pod overrides are
	// applied, so a value supplied via MergedConfig.PodOverrides takes precedence. The builder's
	// WithEnableServiceLinks itself serves as the escape hatch (e.g. a product embedding this
	// handler could reconfigure the builder), so no separate handler field is needed.
	stsBuilder.WithEnableServiceLinks(false)

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
			MountPath: h.configMountPath(),
			ReadOnly:  true,
		})
	}

	// Build the StatefulSet
	sts := stsBuilder.Build()

	// Rename the primary container when the product needs a significant name (e.g. to match
	// its per-container logging key). The builder makes the primary container index 0. This runs
	// before sidecar injection so the Vector provider (which RW-mounts the shared log volume on
	// the producer containers by name) sees the final container names.
	if h.MainContainerName != "" && len(sts.Spec.Template.Spec.Containers) > 0 {
		sts.Spec.Template.Spec.Containers[0].Name = h.MainContainerName
	}

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
				MatchLabels: h.buildSelectorLabels(buildCtx),
			},
		},
	}

	// Set max unavailable (only option available in PodDisruptionBudgetSpec)
	if pdbSpec.MaxUnavailable != nil {
		pdb.Spec.MaxUnavailable = &intstr.IntOrString{
			Type:   intstr.Int,
			IntVal: *pdbSpec.MaxUnavailable,
		}
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
