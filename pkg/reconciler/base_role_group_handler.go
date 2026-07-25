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
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// managedByValue is the app.kubernetes.io/managed-by value stamped on framework-built resources.
const managedByValue = "operator-go"

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

	// ProductName is the kubedoop product name (e.g. "trino") used to resolve the cluster CR's
	// spec.image into a concrete image reference: "{repo}/{ProductName}:{version}-kubedoop{v}".
	// When set, the CR-declared image wins over RoleImages and Image for every role, so products
	// no longer have to patch the built container themselves — a post-build patch would also miss
	// the sidecars, which are told the image before the StatefulSet is built. Empty keeps the
	// static Image/RoleImages behavior (backward compatible).
	ProductName string

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
	//
	// Products whose primary container name differs per role (e.g. HDFS: namenode / datanode /
	// journalnode) set it per role via SetRoleMainContainerName; the per-role value wins over
	// this global default.
	MainContainerName string

	// RoleMainContainerName maps role names to a role-specific primary container name, overriding
	// MainContainerName for that role. Symmetric with RoleContainerPorts.
	RoleMainContainerName map[string]string

	// PublishNotReadyAddresses, when true, sets the same flag on the headless Service so
	// peers can resolve each other's DNS before readiness (required by quorum systems).
	PublishNotReadyAddresses bool

	// LabelDomain, when set (e.g. "zookeeper.kubedoop.dev"), enables product-owned identity
	// labels — "<domain>/cluster", "<domain>/role", "<domain>/role-group" — that are used
	// for resource selectors (StatefulSet, Services, PDB) instead of the descriptive
	// app.kubernetes.io/* labels. The product-domain prefix guarantees the selectors never
	// match another product's pods, and decoupling from app.kubernetes.io/* keeps the
	// immutable StatefulSet selector stable and free of user-mutable labels.
	// When empty, selectors fall back to the framework-owned app.kubernetes.io/* identity subset
	// (see frameworkSelectorLabels).
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
	// and mounts it on itself (pre-creating each producer's per-container log directory before
	// exec'ing vector). Whenever the sidecar does not land — Vector disabled, no producer, or no
	// source for vector.yaml — no shared volume exists and no file appender is emitted
	// (console-only); see RoleGroupBuildContext.VectorLogPipelineActive.
	//
	// Products whose primary container name (and therefore its logging key) differs per role set
	// this per role via SetRoleLoggingContainers; the per-role value wins over this global list.
	LoggingContainers []productlogging.ContainerLogging

	// RoleLoggingContainers maps role names to a role-specific logging-producer list, overriding
	// LoggingContainers for that role. Symmetric with RoleContainerPorts.
	RoleLoggingContainers map[string][]productlogging.ContainerLogging

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
	// customization goes through MergedConfig.PodOverrides, which strategic-merges per field —
	// an override must explicitly restate any default field it wants to CHANGE (e.g.
	// runAsNonRoot: false alongside runAsUser: 0), while unmentioned fields keep the default.
	containerSecurityContext *corev1.SecurityContext

	// podSecurityContext is the pod-level security context applied to the pod spec. See
	// containerSecurityContext for override semantics.
	podSecurityContext *corev1.PodSecurityContext
}

// NewBaseRoleGroupHandler creates a new BaseRoleGroupHandler with defaults.
func NewBaseRoleGroupHandler[CR common.ClusterInterface](image string, scheme *runtime.Scheme) *BaseRoleGroupHandler[CR] {
	return &BaseRoleGroupHandler[CR]{
		Image:                 image,
		ImagePullPolicy:       corev1.PullIfNotPresent,
		RoleImages:            make(map[string]string),
		RoleContainerPorts:    make(map[string][]corev1.ContainerPort),
		RoleServicePorts:      make(map[string][]corev1.ServicePort),
		RoleMainContainerName: make(map[string]string),
		RoleLoggingContainers: make(map[string][]productlogging.ContainerLogging),
		Scheme:                scheme,
		ExtraLabels:           make(map[string]string),
		ExtraAnnotations:      make(map[string]string),
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
// prefer MergedConfig.PodOverrides instead of this handler-wide override; PodOverrides
// strategic-merges the security context per field, so an override must explicitly restate any
// default field it wants to change (unmentioned fields keep the default).
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
//
// The PodDisruptionBudget is intentionally NOT built here: it is a role-level resource
// (roleConfig.podDisruptionBudget covers all pods of a role across every role group), so the
// generic reconciler builds exactly one per role via BuildRolePodDisruptionBudget. Building it
// per role group here would emit one PDB per group and split the role's disruption budget.
func (h *BaseRoleGroupHandler[CR]) BuildResources(
	ctx context.Context,
	k8sClient client.Client,
	cr CR,
	buildCtx *RoleGroupBuildContext,
) (*RoleGroupResources, error) {
	logger := log.FromContext(ctx)

	resources := &RoleGroupResources{}

	// Propagate the product image to the registered sidecars (e.g. Vector, which ships inside
	// the product image). This must happen here — not earlier in GenericReconciler — because the
	// documented embedding pattern resolves the CR-driven image inside the product's
	// BuildResources override, immediately before delegating to this method; any earlier
	// propagation would see a stale (or empty) image. Doing it in the base implementation means
	// every embedding handler gets it for free instead of hand-calling SetProductImage.
	//
	// Select the manager exactly as sidecar injection does below (prefer the SDK-created one,
	// fall back to the instance field) so propagation and injection can never target different
	// managers. Call SetProductImage unconditionally: it rejects an empty image, so a
	// misconfigured product fails loudly here instead of silently injecting a sidecar with an
	// empty image field.
	if sidecarMgr := buildCtx.SidecarManager; sidecarMgr != nil || h.sidecarManager != nil {
		if sidecarMgr == nil {
			sidecarMgr = h.sidecarManager
		}
		image, pullPolicy := h.containerImage(buildCtx, buildCtx.RoleName)
		if err := sidecarMgr.SetProductImage(image, pullPolicy); err != nil {
			return nil, fmt.Errorf("failed to set product image on sidecars: %w", err)
		}
	}

	// An ExtraLabel that collides with a selector key can never take effect, and on an existing
	// StatefulSet it makes every update fail. Reject it before anything is built.
	if err := h.validateExtraLabels(buildCtx); err != nil {
		return nil, err
	}

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

	logger.V(1).Info("Built role group resources",
		"role", buildCtx.RoleName,
		"group", buildCtx.RoleGroupName,
		"resourceName", buildCtx.ResourceName)

	return resources, nil
}

// vectorEnabledFor reports whether the Vector agent is enabled for this role group, based on
// the deep-merged logging spec. It is the enablement FLAG only — whether the sidecar is really
// injected is decided by vectorLogPipelineActive.
func vectorEnabledFor(buildCtx *RoleGroupBuildContext) bool {
	if buildCtx == nil || buildCtx.MergedConfig == nil {
		return false
	}
	// vector.IsAgentEnabled is the single, shared predicate used by both this producer side and
	// the consumer side (generic_reconciler.buildSidecarManager), so they can never drift.
	return vector.IsAgentEnabled(buildCtx.MergedConfig.Logging)
}

// vectorLogPipelineActive reports whether the shared Vector log pipeline really exists for this
// role group. The Vector provider is the sole owner of the shared log emptyDir and of its mounts,
// and the reconciler skips it whenever the agent is enabled but the sidecar cannot run (no
// declared producer, or nothing supplying vector.yaml). Everything that writes INTO that volume —
// above all the rolling file appender — has to key off the same resolved decision, or the product
// is pointed at a path no volume backs.
//
// A nil RoleGroupBuildContext.VectorLogPipelineActive means the context was not built by
// GenericReconciler (a product assembling one by hand): the enablement flag is then all that is
// known, which is the behavior such a caller already had.
func vectorLogPipelineActive(buildCtx *RoleGroupBuildContext) bool {
	if !vectorEnabledFor(buildCtx) {
		return false
	}
	if buildCtx.VectorLogPipelineActive == nil {
		return true
	}
	return *buildCtx.VectorLogPipelineActive
}

// LoggingProducers implements LoggingProducerProvider: it exposes the role's declared
// log-producer containers so the GenericReconciler can configure the Vector sidecar (the single
// owner of the shared log volume) without reaching into handler internals. The per-role list set
// via SetRoleLoggingContainers wins over the global LoggingContainers.
func (h *BaseRoleGroupHandler[CR]) LoggingProducers(roleName string) []productlogging.ContainerLogging {
	return h.loggingContainersFor(roleName)
}

// loggingContainersFor returns the role-specific logging-producer list when set, else the global
// LoggingContainers.
func (h *BaseRoleGroupHandler[CR]) loggingContainersFor(roleName string) []productlogging.ContainerLogging {
	if lc, ok := h.RoleLoggingContainers[roleName]; ok {
		return lc
	}
	return h.LoggingContainers
}

// mainContainerNameFor returns the role-specific primary container name when set, else the global
// MainContainerName.
func (h *BaseRoleGroupHandler[CR]) mainContainerNameFor(roleName string) string {
	if name, ok := h.RoleMainContainerName[roleName]; ok {
		return name
	}
	return h.MainContainerName
}

// SetRoleLoggingContainers sets the logging-producer list for a specific role, overriding the
// global LoggingContainers for that role. Symmetric with SetRoleContainerPorts.
func (h *BaseRoleGroupHandler[CR]) SetRoleLoggingContainers(roleName string, containers []productlogging.ContainerLogging) {
	if h.RoleLoggingContainers == nil {
		h.RoleLoggingContainers = make(map[string][]productlogging.ContainerLogging)
	}
	h.RoleLoggingContainers[roleName] = containers
}

// SetRoleMainContainerName sets the primary container name for a specific role, overriding the
// global MainContainerName for that role. Symmetric with SetRoleContainerPorts.
func (h *BaseRoleGroupHandler[CR]) SetRoleMainContainerName(roleName, name string) {
	if h.RoleMainContainerName == nil {
		h.RoleMainContainerName = make(map[string]string)
	}
	h.RoleMainContainerName[roleName] = name
}

// LogVolumeSizeLimit implements LoggingProducerProvider: the shared log volume SizeLimit override
// ("" uses vector.DefaultLogVolumeSize).
func (h *BaseRoleGroupHandler[CR]) LogVolumeSizeLimit() string {
	return h.LogVolumeSize
}

// clusterImage resolves the image the cluster CR declares in spec.image. It returns "" unless the
// product opted in by setting ProductName — spec.image is product-scoped
// ("{repo}/{ProductName}:{version}"), so the framework cannot resolve it on its own.
func (h *BaseRoleGroupHandler[CR]) clusterImage(buildCtx *RoleGroupBuildContext) (string, corev1.PullPolicy) {
	if h.ProductName == "" || buildCtx == nil || buildCtx.ClusterSpec == nil || buildCtx.ClusterSpec.Image == nil {
		return "", ""
	}
	image := buildCtx.ClusterSpec.Image.GetImage(h.ProductName)
	if image == "" {
		return "", ""
	}
	return image, buildCtx.ClusterSpec.Image.GetPullPolicy()
}

// containerImage returns the container image and pull policy for a role, in precedence order:
// the CR's spec.image (when ProductName is set), then the per-role override, then the handler
// default. Resolving it here — rather than leaving each product to patch the built container —
// keeps the image the sidecars are told about (SetProductImage, e.g. the Vector agent, which ships
// inside the product image) identical to the one the main container actually runs.
func (h *BaseRoleGroupHandler[CR]) containerImage(buildCtx *RoleGroupBuildContext, roleName string) (string, corev1.PullPolicy) {
	if image, pullPolicy := h.clusterImage(buildCtx); image != "" {
		return image, pullPolicy
	}
	if image, ok := h.RoleImages[roleName]; ok {
		return image, h.ImagePullPolicy
	}
	return h.Image, h.ImagePullPolicy
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

// buildLabels creates the descriptive labels for resources. It is a superset of
// buildSelectorLabels: the CR's own labels and the product's ExtraLabels are metadata (and pod
// template) only, never selectors.
func (h *BaseRoleGroupHandler[CR]) buildLabels(buildCtx *RoleGroupBuildContext) map[string]string {
	labels := make(map[string]string)

	// Add cluster labels
	for k, v := range buildCtx.ClusterLabels {
		labels[k] = v
	}

	// The framework-owned identity labels win over same-named cluster labels: they are what the
	// selectors match on.
	for k, v := range h.frameworkSelectorLabels(buildCtx) {
		labels[k] = v
	}

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

// frameworkSelectorLabels returns the framework-owned identity labels of a role group. They are
// derived exclusively from the cluster/role/role group names, never from buildCtx.ClusterLabels or
// h.ExtraLabels: a StatefulSet's .spec.selector is immutable after creation, so a user-mutable
// label inside it would freeze at its creation-time value and every later edit of that label would
// leave the live object unmatchable — and, because the pod template keeps carrying the current
// value, permanently unpatchable.
func (h *BaseRoleGroupHandler[CR]) frameworkSelectorLabels(buildCtx *RoleGroupBuildContext) map[string]string {
	return map[string]string{
		"app.kubernetes.io/instance":                        buildCtx.ClusterName,
		"app.kubernetes.io/component":                       buildCtx.RoleName,
		"app.kubernetes.io/managed-by":                      managedByValue,
		buildCtx.ClusterName + "-" + buildCtx.RoleGroupName: valueTrue,
	}
}

// buildSelectorLabels returns the labels used for resource selectors. When LabelDomain is
// set, these are the product-owned identity labels (cluster + role + role-group), which are
// product-namespaced and stable. Otherwise it falls back to the framework-owned identity subset
// of buildLabels — a subset of the pod template labels, as the API server requires, but without
// the user-mutable CR/extra labels.
func (h *BaseRoleGroupHandler[CR]) buildSelectorLabels(buildCtx *RoleGroupBuildContext) map[string]string {
	if h.LabelDomain == "" {
		return h.frameworkSelectorLabels(buildCtx)
	}
	return h.identityLabels(buildCtx)
}

// SelectorLabels exposes the role group's selector labels so embedding products can build
// matching selectors for resources they add themselves (e.g. a metrics Service).
func (h *BaseRoleGroupHandler[CR]) SelectorLabels(buildCtx *RoleGroupBuildContext) map[string]string {
	return h.buildSelectorLabels(buildCtx)
}

// validateExtraLabels rejects an ExtraLabel whose key is a selector key but whose value differs
// from the selector's. Such a label is silently ineffective and actively harmful:
//
//   - buildLabels applies ExtraLabels last, but the StatefulSet builder re-writes the selector
//     keys into the pod template AFTER the labels, so the product's value never reaches the pod.
//   - The .spec.selector of a live StatefulSet is immutable. A cluster created while the selector
//     still included the ExtraLabels carries the product's value there forever, and the pod
//     template the framework now builds no longer satisfies it — every update is rejected by the
//     API server, with nothing in the object to explain why.
//
// A collision that agrees with the selector value changes nothing and is allowed, so a product
// restating e.g. app.kubernetes.io/managed-by keeps working.
func (h *BaseRoleGroupHandler[CR]) validateExtraLabels(buildCtx *RoleGroupBuildContext) error {
	selector := h.buildSelectorLabels(buildCtx)

	var collisions []string
	for key, value := range h.ExtraLabels {
		if selectorValue, isSelector := selector[key]; isSelector && selectorValue != value {
			collisions = append(collisions, fmt.Sprintf("%s (selector value %q, ExtraLabels value %q)",
				key, selectorValue, value))
		}
	}
	if len(collisions) == 0 {
		return nil
	}

	slices.Sort(collisions)
	return fmt.Errorf(
		"ExtraLabels collide with the selector labels of role %q group %q: %s; a selector label is framework-owned and cannot be overridden",
		buildCtx.RoleName, buildCtx.RoleGroupName, strings.Join(collisions, ", "))
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
	// Build config data. The ConfigGenerator, when set, owns the rendering of every file it
	// recognizes; the fallback below only fills the gaps, so the two paths can never disagree
	// about the same filename.
	data := make(map[string]string)

	if h.ConfigGenerator != nil && len(buildCtx.MergedConfig.ConfigFiles) > 0 {
		generatedData, err := h.ConfigGenerator.GenerateFiles(buildCtx.MergedConfig.ConfigFiles)
		if err != nil {
			return nil, err
		}
		for filename, content := range generatedData {
			data[filename] = content
		}
	}

	// Fallback rendering for files no generator produced. It goes through the properties adapter
	// because that emits keys in sorted order (and escapes them properly): Go randomizes map
	// iteration, so a hand-concatenated "k=v" rendering would produce different content on every
	// reconcile. The apply path replaces ConfigMap.Data wholesale and the reconciler watches
	// ConfigMaps, so that churn becomes a self-triggering reconcile loop.
	propertiesAdapter := config.NewPropertiesAdapter()
	for filename, cfg := range buildCtx.MergedConfig.ConfigFiles {
		if _, exists := data[filename]; exists {
			continue
		}
		content, err := propertiesAdapter.Marshal(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to render config file %q: %w", filename, err)
		}
		data[filename] = content
	}

	// Generate the logging-related ConfigMap entries: one config file per declared producer plus
	// vector.yaml when the Vector agent is enabled (RenderLoggingConfigMapData owns the file
	// appender and vector.yaml gating). Fail fast on a key collision rather than silently
	// overwriting a file the product already produced (e.g. via MergedConfig.ConfigFiles).
	loggingData, err := RenderLoggingConfigMapData(buildCtx, h.loggingContainersFor(buildCtx.RoleName))
	if err != nil {
		return nil, err
	}
	for filename, content := range loggingData {
		if _, exists := data[filename]; exists {
			return nil, fmt.Errorf("logging config file %q collides with an existing ConfigMap key", filename)
		}
		data[filename] = content
	}

	cm := builder.NewConfigMapBuilder(buildCtx.ResourceName, buildCtx.ClusterNamespace).
		WithLabels(labels).
		WithAnnotations(h.buildAnnotations(buildCtx)).
		WithConfigFiles(data).
		Build()

	// The documented way to extend this handler is to call BuildResources and then customize the
	// returned objects, and `resources.ConfigMap.Data[k] = v` is the obvious way to add a file.
	// The builder leaves Data nil when a cluster declares no config at all, which would make that
	// assignment panic, so the map is always present here.
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	return cm, nil
}

// buildHeadlessService creates the headless service for StatefulSet.
func (h *BaseRoleGroupHandler[CR]) buildHeadlessService(buildCtx *RoleGroupBuildContext, labels map[string]string) *corev1.Service {
	return builder.NewHeadlessServiceBuilder(buildCtx.ResourceName+"-headless", buildCtx.ClusterNamespace).
		WithLabels(labels).
		WithAnnotations(h.buildAnnotations(buildCtx)).
		WithSelector(h.buildSelectorLabels(buildCtx)).
		WithPublishNotReadyAddresses(h.PublishNotReadyAddresses).
		WithPorts(h.servicePorts(buildCtx.RoleName, buildCtx.RoleGroupName)).
		Build()
}

// buildService creates the client-facing service.
func (h *BaseRoleGroupHandler[CR]) buildService(buildCtx *RoleGroupBuildContext, labels map[string]string, ports []corev1.ServicePort) *corev1.Service {
	return builder.NewServiceBuilder(buildCtx.ResourceName, buildCtx.ClusterNamespace).
		WithLabels(labels).
		WithAnnotations(h.buildAnnotations(buildCtx)).
		WithSelector(h.buildSelectorLabels(buildCtx)).
		WithPorts(ports).
		Build()
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

	// Effective replica count. Normally the role group's declared replicas, but when the cluster
	// is stopped (ClusterOperation.stopped) it is forced to 0: stopping a cluster means "run zero
	// pods while every resource — ConfigMap, Service, StatefulSet, PDB, ServiceAccount, PVCs — is
	// still reconciled and preserved so the cluster can be resumed". Only the pod count changes;
	// the full resource set is created/updated (and spec/config changes are applied) as usual.
	replicas := buildCtx.RoleGroupSpec.GetReplicas()
	if buildCtx.ClusterSpec != nil &&
		buildCtx.ClusterSpec.ClusterOperation != nil &&
		buildCtx.ClusterSpec.ClusterOperation.Stopped {
		replicas = int32(0)
	}

	// Set basic properties
	image, pullPolicy := h.containerImage(buildCtx, buildCtx.RoleName)
	stsBuilder.WithLabels(labels).
		WithSelectorLabels(h.buildSelectorLabels(buildCtx)).
		WithAnnotations(h.buildAnnotations(buildCtx)).
		WithReplicas(replicas).
		WithImage(image, pullPolicy).
		WithConfig(buildCtx.MergedConfig).
		WithPorts(h.containerPorts(buildCtx.RoleName, buildCtx.RoleGroupName))

	// Bind the reconciler-managed ServiceAccount to the pod template when configured, so the
	// created SA is actually used. Empty leaves ServiceAccountName unset (pods use the namespace
	// default SA), preserving backward compatibility.
	if buildCtx.ServiceAccountName != "" {
		stsBuilder.WithServiceAccount(buildCtx.ServiceAccountName)
	}

	// Wire the role group's declared runtime config (commons RoleGroupConfigSpec) into the
	// StatefulSet. All of these land on the builder BEFORE pod overrides take effect: the
	// builder applies PodOverrides last in Build(), so a user-supplied pod override (e.g. an
	// affinity in podOverrides) always keeps precedence over the config-declared value.
	roleGroupConfig := buildCtx.RoleGroupSpec.GetConfig()
	if roleGroupConfig != nil {
		if roleGroupConfig.Resources != nil {
			stsBuilder.WithResources(roleGroupConfig.Resources)
			// Opt-in data PVC: when a product sets StorageMountPath, build a VolumeClaimTemplate
			// from the configured storage and mount it, so the volume/mount contract is enforced
			// by the builder instead of being hand-assembled by each product.
			if h.StorageMountPath != "" && roleGroupConfig.Resources.Storage != nil {
				stsBuilder.WithStorage(roleGroupConfig.Resources.Storage, h.StorageMountPath)
			}
		}

		// Affinity: the CRD carries it as a schema-free RawExtension holding a corev1.Affinity.
		// Invalid JSON fails the build loudly — silently dropping a user's scheduling
		// constraints could place pods on nodes the user explicitly excluded (same loud-failure
		// stance as a Vector misconfiguration). Backward compatible: the framework sets affinity
		// only when the CRD config provides one, so products that post-process the built
		// StatefulSet with `if podSpec.Affinity == nil { ... }` default guards remain correct.
		if roleGroupConfig.Affinity != nil && len(roleGroupConfig.Affinity.Raw) > 0 {
			affinity := &corev1.Affinity{}
			if err := json.Unmarshal(roleGroupConfig.Affinity.Raw, affinity); err != nil {
				return nil, fmt.Errorf("invalid affinity in role group config (role %q, group %q): %w",
					buildCtx.RoleName, buildCtx.RoleGroupName, err)
			}
			stsBuilder.WithAffinity(affinity)
		}

		// GracefulShutdownTimeout maps to the pod's terminationGracePeriodSeconds (see
		// docs/architecture.md §4.11.2 Graceful Shutdown). An unparsable or non-positive
		// duration fails the build loudly rather than silently falling back. A positive
		// sub-second duration rounds up to 1s so it never truncates to 0 (which would mean
		// immediate SIGKILL). Empty leaves the pod spec untouched (k8s default of 30s applies).
		if roleGroupConfig.GracefulShutdownTimeout != "" {
			d, err := time.ParseDuration(roleGroupConfig.GracefulShutdownTimeout)
			if err != nil {
				return nil, fmt.Errorf("invalid gracefulShutdownTimeout %q in role group config (role %q, group %q): %w",
					roleGroupConfig.GracefulShutdownTimeout, buildCtx.RoleName, buildCtx.RoleGroupName, err)
			}
			if d <= 0 {
				return nil, fmt.Errorf("invalid gracefulShutdownTimeout %q in role group config (role %q, group %q): must be a positive duration",
					roleGroupConfig.GracefulShutdownTimeout, buildCtx.RoleName, buildCtx.RoleGroupName)
			}
			stsBuilder.WithTerminationGracePeriod(int64((d + time.Second - 1) / time.Second))
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

	// Mount the role group ConfigMap as the "config" volume at configMountPath().
	//
	// This is intentionally NOT gated on MergedConfig.ConfigFiles. ConfigFiles is populated
	// only from role/role-group configOverrides, but the role group ConfigMap
	// (buildCtx.ResourceName) is ALWAYS produced by buildConfigMap — a product can populate its
	// real config (e.g. zoo.cfg, security.properties, logback.xml) directly into ConfigMap.Data
	// with no overrides at all. Gating the mount on ConfigFiles would starve those products of
	// their config in the common no-overrides case, forcing them to hand-create a config volume
	// and strip the framework's. The mount references buildCtx.ResourceName, which the same
	// handler's buildConfigMap always creates, so the referenced ConfigMap always exists.
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

	// Inject product-registered CSI volumes (secret/TLS certificates, listener address
	// volumes). These flow through the same builder path as the config volume (volumes on the
	// pod, mounts on the primary container), before the container rename and sidecar injection.
	// buildCtx.VolumeProviders is per-build-context, so nothing accumulates across reconciles.
	for _, vp := range buildCtx.VolumeProviders {
		if vp == nil {
			continue
		}
		for _, v := range vp.Volumes() {
			stsBuilder.AddVolume(v)
		}
		for _, m := range vp.VolumeMounts() {
			stsBuilder.AddVolumeMount(m)
		}
	}

	// Name the primary container when the product needs a significant name (e.g. to match its
	// per-container logging key). This must reach the builder BEFORE Build(): podOverrides are
	// strategic-merged by container name inside Build(), so an override addressing the
	// user-facing name (e.g. "node") must find the primary container already carrying it — a
	// post-build rename would leave the override appended as a phantom, image-less container.
	// Sidecar injection below also sees the final name (the Vector provider RW-mounts the
	// shared log volume on the producer containers by name).
	if mainName := h.mainContainerNameFor(buildCtx.RoleName); mainName != "" {
		stsBuilder.WithMainContainerName(mainName)
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

	// Fail loudly on image-less containers instead of shipping a StatefulSet the API server
	// rejects (a silent Degraded loop). The typical cause: a podOverride container whose name
	// matches nothing — a typo, or a sidecar name; sidecars are injected AFTER the overrides
	// merge, so they cannot be addressed by podOverrides.
	mainName := stsBuilder.MainContainerName
	if mainName == "" {
		mainName = buildCtx.ResourceName
	}
	for _, c := range sts.Spec.Template.Spec.Containers {
		if c.Image == "" {
			return nil, fmt.Errorf(
				"container %q has no image: a podOverrides container must either address an existing container by name (main container: %q) or be fully specified; sidecar containers cannot be overridden via podOverrides",
				c.Name, mainName)
		}
	}
	for _, c := range sts.Spec.Template.Spec.InitContainers {
		if c.Image == "" {
			return nil, fmt.Errorf(
				"init container %q has no image: a podOverrides container must either address an existing container by name or be fully specified",
				c.Name)
		}
	}

	return sts, nil
}

// BuildRolePodDisruptionBudget builds the role-level PodDisruptionBudget from
// roleConfig.podDisruptionBudget. A role's PDB covers every pod of the role across all of its
// role groups (name "<cluster>-<role>", selector on the cluster+role identity labels), so
// exactly one is emitted per role. Returns nil when the PDB is unset or explicitly disabled.
func (h *BaseRoleGroupHandler[CR]) BuildRolePodDisruptionBudget(
	clusterName, namespace, roleName string,
	clusterLabels map[string]string,
	roleSpec *v1alpha1.RoleSpec,
) *policyv1.PodDisruptionBudget {
	roleConfig := roleSpec.GetRoleConfig()
	if roleConfig == nil || roleConfig.PodDisruptionBudget == nil {
		return nil
	}

	b := builder.NewPDBBuilder(RoleResourceName(clusterName, roleName), namespace).
		WithLabels(h.buildRoleLabels(clusterName, roleName, clusterLabels)).
		WithAnnotations(h.ExtraAnnotations).
		WithSelector(h.buildRoleSelectorLabels(clusterName, roleName)).
		WithSpec(roleConfig.PodDisruptionBudget)

	// Enabled defaults to true in the CRD; honor an explicit disable.
	if !b.IsEnabled() {
		return nil
	}

	return b.Build()
}

// roleIdentityLabels are the product-owned labels that identify a role (cluster + role, without
// a role group). They are a subset of every pod's identity labels, so a selector built from them
// matches all of a role's pods across role groups. Empty when the handler has no LabelDomain.
func (h *BaseRoleGroupHandler[CR]) roleIdentityLabels(clusterName, roleName string) map[string]string {
	if h.LabelDomain == "" {
		return nil
	}
	return map[string]string{
		ClusterLabelKey(h.LabelDomain): clusterName,
		RoleLabelKey(h.LabelDomain):    roleName,
	}
}

// buildRoleSelectorLabels is the role-scoped analogue of buildSelectorLabels: it must match all
// pods of the role across every role group, so it omits the role group marker/identity label.
func (h *BaseRoleGroupHandler[CR]) buildRoleSelectorLabels(clusterName, roleName string) map[string]string {
	if h.LabelDomain == "" {
		// Without product identity labels the selector falls back to the recommended labels.
		// Include managed-by (framework pods always carry it) so the PDB stays scoped to
		// operator-go-managed pods and cannot accidentally match another operator's workloads
		// that reuse the same instance/component labels.
		return map[string]string{
			"app.kubernetes.io/instance":   clusterName,
			"app.kubernetes.io/component":  roleName,
			"app.kubernetes.io/managed-by": managedByValue,
		}
	}
	return h.roleIdentityLabels(clusterName, roleName)
}

// buildRoleLabels is the role-scoped analogue of buildLabels for metadata on role-level
// resources: the standard recommended labels plus cluster/extra labels, but no role group marker.
func (h *BaseRoleGroupHandler[CR]) buildRoleLabels(clusterName, roleName string, clusterLabels map[string]string) map[string]string {
	labels := make(map[string]string)

	for k, v := range clusterLabels {
		labels[k] = v
	}

	labels["app.kubernetes.io/instance"] = clusterName
	labels["app.kubernetes.io/component"] = roleName
	labels["app.kubernetes.io/managed-by"] = managedByValue

	for k, v := range h.roleIdentityLabels(clusterName, roleName) {
		labels[k] = v
	}

	for k, v := range h.ExtraLabels {
		labels[k] = v
	}

	return labels
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

// RoleNameProvider is implemented by handlers that carry per-role configuration keyed by role name
// (BaseRoleGroupHandler's Set* methods). A role name is a bare string that has to match a key of
// the CR's spec.roles; when it does not, every lookup silently returns nil — no ports, no image
// override, and no Service at all. Exposing the configured keys lets the reconciler cross-check
// them against the CR's declared roles and fail loudly on a typo instead.
type RoleNameProvider interface {
	ConfiguredRoleNames() []string
}

// ConfiguredRoleNames implements RoleNameProvider: the sorted union of the role names this handler
// was configured for through the Set* methods.
func (h *BaseRoleGroupHandler[CR]) ConfiguredRoleNames() []string {
	names := make(map[string]struct{})
	for name := range h.RoleImages {
		names[name] = struct{}{}
	}
	for name := range h.RoleContainerPorts {
		names[name] = struct{}{}
	}
	for name := range h.RoleServicePorts {
		names[name] = struct{}{}
	}
	for name := range h.RoleMainContainerName {
		names[name] = struct{}{}
	}
	for name := range h.RoleLoggingContainers {
		names[name] = struct{}{}
	}
	return slices.Sorted(maps.Keys(names))
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
