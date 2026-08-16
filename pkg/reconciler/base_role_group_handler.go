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
	stderrors "errors"
	"fmt"
	"time"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/config"
	"github.com/zncdatadev/operator-go/pkg/constant"
	"github.com/zncdatadev/operator-go/pkg/listener"
	"github.com/zncdatadev/operator-go/pkg/security"
	"github.com/zncdatadev/operator-go/pkg/sidecar"
	"github.com/zncdatadev/operator-go/pkg/vector"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
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
//
// ONE INSTANCE SERVES EVERY CLUSTER. A handler is constructed once in main.go and the controller
// reuses it for every CR and every reconcile, so every field below is process-wide state. Set them
// at construction, from values that do not depend on which cluster is being reconciled.
//
// Anything that DOES depend on the cluster — an image derived from its spec, ports that move when
// its TLS toggle flips — goes on RoleGroupBuildContext instead, which is rebuilt per role group per
// reconcile. Assigning such a value here from inside BuildResources races above
// MaxConcurrentReconciles 1, and even at 1 it leaks: skip the assignment on one CR and it silently
// inherits the previous CR's value.
type BaseRoleGroupHandler[CR common.ClusterInterface] struct {

	// ProductName is the kubedoop product name (e.g. "trino"). It supplies two unrelated things:
	// the app.kubernetes.io/name label value, and the repository path segment used when resolving
	// spec.image into "{repo}/{ProductName}:{version}-kubedoop{v}".
	//
	// It no longer decides WHETHER spec.image is read — that is ImageDefaults' job now. The two
	// were coupled, and because the image half could not express the kubedoop tag convention (see
	// ImageDefaults), three migrated operators gave up all of it: they left ProductName empty,
	// hand-rolled image resolution, and two of them emit no app.kubernetes.io/version at all.
	//

	// ImageDefaults fills in whatever spec.image leaves empty. It is read on every reconcile,
	// which is the whole point:
	//
	//	handler.ProductName = "hive"
	//	handler.ImageDefaults = commonsv1alpha1.ImageSpec{
	//	    Repo:            "quay.io/zncdatadev",
	//	    ProductVersion:  "4.0.1",
	//	    KubedoopVersion: version.BuildVersion, // the operator's own build version
	//	}
	//
	// KubedoopVersion is why this cannot be a webhook's job. Kubedoop product images are published
	// only with the "-kubedoop<version>" suffix, and the natural value of that suffix is the
	// operator's build version — a reconcile-time fact that moves when the operator binary is
	// upgraded. Webhook defaults are persisted into the spec at admission and never recomputed
	// (docs/architecture.md §2.6), so a cluster admitted by operator 0.1.0 would keep asking for
	// -kubedoop0.1.0 images forever. Evaluated here, an operator upgrade moves existing clusters
	// onto the co-released product image.
	//

	// ConfigGenerator is used to generate configuration files.
	// Optional - if nil, config files are generated from MergedConfig only.
	ConfigGenerator *config.MultiFormatConfigGenerator

	// Scheme is the runtime scheme for ownership setup.
	Scheme *runtime.Scheme

	// RoleStorageMountPaths maps role names to a role-specific data PVC mount path, overriding
	// StorageMountPath for that role. Symmetric with RoleContainerPorts and its siblings, and the
	// one per-role override the handler was missing: a product either gave a data PVC to every
	// StatefulSet role or to none.
	//

	// ConfigMountPath is where the generated config ConfigMap is mounted in the primary
	// container. Products whose application reads config from a specific directory (e.g.
	// "/etc/trino") set this. Defaults to the kubedoop-canonical config mount path
	// (constant.KubedoopConfigDirMount) when empty.
	//
	// The mount is READ-ONLY. A product whose start-up rewrites a config file (Kerberos realm
	// substitution, credential interpolation) must copy it to a writable directory first —
	// conventionally constant.KubedoopConfigDir — with `cp -RL`: a ConfigMap volume is a farm of
	// symlinks into a hidden ..data/ directory, so a copy that preserves symlinks leaves dangling
	// links at the destination. See docs/architecture.md §4.1.5.
	ConfigMountPath string

	// MainContainerName, when set, renames the primary (first) container of the StatefulSet.
	// Products use this when the container name is significant — e.g. it must match the
	// per-container logging key (logging.containers.<name>) declared in LoggingContainers.
	// Defaults to the resource name (set by the StatefulSet builder) when empty.
	//

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
	//

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
func NewBaseRoleGroupHandler[CR common.ClusterInterface](scheme *runtime.Scheme) *BaseRoleGroupHandler[CR] {
	return &BaseRoleGroupHandler[CR]{Scheme: scheme}
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
			// The handler's manager is process-wide, and SetProductImage below writes THIS
			// cluster's image into it. Build against a copy, or the next cluster inherits it —
			// the framework's own instance of the shared-state defect this context's Image field
			// exists to fix. The reconciler-created manager (the common path) is already
			// per-role-group, so it is used as-is.
			sidecarMgr = h.sidecarManager.CloneForBuild()
			buildCtx.SidecarManager = sidecarMgr
		}
		image, pullPolicy := buildCtx.ResolvedImage.Reference, buildCtx.ResolvedImage.PullPolicy
		if err := sidecarMgr.SetProductImage(image, pullPolicy); err != nil {
			return nil, fmt.Errorf("failed to set product image on sidecars: %w", err)
		}
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
	svcPorts := buildCtx.Declaration.ServicePorts
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

// ClusterLabelKey returns the identity label key for the cluster, under the given domain.
func ClusterLabelKey(domain string) string { return domain + "/cluster" }

// RoleLabelKey returns the identity label key for the role, under the given domain.
func RoleLabelKey(domain string) string { return domain + "/role" }

// RoleGroupLabelKey returns the identity label key for the role group, under the given domain.
func RoleGroupLabelKey(domain string) string { return domain + "/role-group" }

// recommendedLabels returns the Kubernetes recommended labels the framework stamps on every
// resource it builds. roleGroupName is empty for role-level resources, which span every group of
// the role and therefore carry no role-group label.
//
// name and version are conditional. app.kubernetes.io/name is the product name, which only a
// handler that declared one has; app.kubernetes.io/version comes from productVersion above. Both
// are dropped when the value is not a legal label value, because both are free-form input —
// spec.image.productVersion is written by the user and constrained by nothing — and an
// out-of-range value has to cost one cosmetic label rather than make every resource of the
// cluster rejected by the API server. instance/component are deliberately not guarded the same
// way: they also feed the StatefulSet's .spec.selector, so a value too long to be a label is
// already fatal there and silently dropping it here would only hide where it failed.
func (h *BaseRoleGroupHandler[CR]) recommendedLabels(
	clusterName, roleName, roleGroupName, productName, productVersion string,
) map[string]string {
	labels := map[string]string{
		constant.LabelKubernetesInstance:  clusterName,
		constant.LabelKubernetesComponent: roleName,
		constant.LabelKubernetesManagedBy: managedByValue,
	}
	if roleGroupName != "" {
		labels[constant.LabelKubernetesRoleGroup] = roleGroupName
	}
	if productName != "" && len(validation.IsValidLabelValue(productName)) == 0 {
		labels[constant.LabelKubernetesName] = productName
	}
	if productVersion != "" && len(validation.IsValidLabelValue(productVersion)) == 0 {
		labels[constant.LabelKubernetesVersion] = productVersion
	}
	return labels
}

// buildLabels creates the descriptive labels for resources. It is a superset of
// buildSelectorLabels: the CR's own labels are metadata (and pod template) only, never selectors.
func (h *BaseRoleGroupHandler[CR]) buildLabels(buildCtx *RoleGroupBuildContext) map[string]string {
	labels := make(map[string]string)

	// The CR's own labels, which is how an operator's user propagates a label to the workloads —
	// including the platform opt-ins the framework does not own, e.g.
	// restarter.kubedoop.dev/enable.
	for k, v := range buildCtx.ClusterLabels {
		labels[k] = v
	}

	// The Kubernetes recommended set is framework-owned, so it is applied over the CR's labels.
	for k, v := range h.recommendedLabels(buildCtx.ClusterName, buildCtx.RoleName, buildCtx.RoleGroupName,
		buildCtx.ProductName, buildCtx.ResolvedImage.ProductVersion) {
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
// buildCtx.ClusterLabels: a StatefulSet's .spec.selector is immutable after creation, so a
// user-mutable label inside it would freeze at its creation-time value and every later edit of that
// label would leave the live object unmatchable — and, because the pod template keeps carrying the
// current value, permanently unpatchable.
//
// This is a strict subset of the recommended set that recommendedLabels stamps on metadata, and
// stays that way: app.kubernetes.io/version changes on every product upgrade and role-group is
// already covered by the marker key below, so neither may enter a selector that can never be
// edited again.
//
// The marker key goes through RoleGroupMarkerLabelKey rather than being concatenated here, because
// "<cluster>-<group>" is a label KEY built from two free-form user strings and overruns the 63-byte
// limit with ordinary names — see that function for why the natural form has to be preserved
// wherever it is legal.
func (h *BaseRoleGroupHandler[CR]) frameworkSelectorLabels(buildCtx *RoleGroupBuildContext) map[string]string {
	return map[string]string{
		constant.LabelKubernetesInstance:  buildCtx.ClusterName,
		constant.LabelKubernetesComponent: buildCtx.RoleName,
		constant.LabelKubernetesManagedBy: managedByValue,
		RoleGroupMarkerLabelKey(buildCtx.ClusterName, buildCtx.RoleName, buildCtx.RoleGroupName): valueTrue,
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
	loggingData, err := RenderLoggingConfigMapData(buildCtx, buildCtx.Declaration.LogProducers)
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
		WithSelector(h.buildSelectorLabels(buildCtx)).
		WithPublishNotReadyAddresses(buildCtx.Declaration.PublishNotReadyAddresses).
		WithPorts(buildCtx.Declaration.ServicePorts).
		Build()
}

// buildService creates the client-facing service.
func (h *BaseRoleGroupHandler[CR]) buildService(buildCtx *RoleGroupBuildContext, labels map[string]string, ports []corev1.ServicePort) *corev1.Service {
	svcBuilder := builder.NewServiceBuilder(buildCtx.ResourceName, buildCtx.ClusterNamespace).
		WithLabels(labels).
		WithSelector(h.buildSelectorLabels(buildCtx)).
		WithPorts(ports)
	// Set the type from the declared listener class before Build(), rather than leaving the
	// product to patch Service.Spec.Type on the object it gets back.
	if buildCtx.Declaration.ListenerClass != "" {
		svcBuilder = svcBuilder.WithServiceType(builder.ServiceType(listener.ServiceTypeFor(buildCtx.Declaration.ListenerClass)))
	}
	return svcBuilder.Build()
}

// wireVolumes attaches the role group ConfigMap and the product's CSI volumes to the builder.
//
// Extracted from buildStatefulSet, which was over the cyclomatic budget: this is the one part of it
// that is a self-contained unit (pod volumes plus the matching mounts on the primary container),
// and it runs before the container rename and sidecar injection so both see the final shape.
func (h *BaseRoleGroupHandler[CR]) wireVolumes(
	stsBuilder *builder.StatefulSetBuilder, buildCtx *RoleGroupBuildContext,
) {
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
	image, pullPolicy := buildCtx.ResolvedImage.Reference, buildCtx.ResolvedImage.PullPolicy
	stsBuilder.WithLabels(labels).
		WithSelectorLabels(h.buildSelectorLabels(buildCtx)).
		WithReplicas(replicas).
		WithImage(image, pullPolicy).
		WithConfig(buildCtx.MergedConfig).
		WithPorts(buildCtx.Declaration.ContainerPorts)

	// Bind the reconciler-managed ServiceAccount to the pod template when configured, so the
	// created SA is actually used. Empty leaves ServiceAccountName unset (pods use the namespace
	// default SA), preserving backward compatibility.
	if buildCtx.ServiceAccountName != "" {
		stsBuilder.WithServiceAccount(buildCtx.ServiceAccountName)
	}

	// The pull secret is resolved from spec.image over ImageDefaults, independently of the image
	// itself: it is needed on every path the image can come from — `custom`, the assembled
	// reference, and a product that resolves its own images and leaves ProductName empty. Ten
	// product CRDs already declare this field, and before it existed here every one of them
	// silently dropped it on migration: the CRD still accepted `pullSecretName` and no pod ever
	// carried an imagePullSecrets entry, so a private-registry install failed with ImagePullBackOff
	// and no indication of why.
	if secretName := buildCtx.ResolvedImage.PullSecretName; secretName != "" {
		stsBuilder.WithImagePullSecretName(secretName)
	}

	// Wire the role group's declared runtime config (commons RoleGroupConfigSpec) into the
	// StatefulSet. All of these land on the builder BEFORE pod overrides take effect: the
	// builder applies PodOverrides last in Build(), so a user-supplied pod override (e.g. an
	// affinity in podOverrides) always keeps precedence over the config-declared value.
	// The effective config is folded ONCE, by the reconciler, before this runs — over the role's
	// declared defaults, the CR's role level and its role group level. This used to be a SECOND
	// fold computed here and thrown away, so the field a product could read was missing its own
	// defaults, and nothing derived from the effective config could reach the ConfigMap because
	// that had already been built.
	roleGroupConfig := buildCtx.RoleGroupSpec.GetConfig()
	if roleGroupConfig != nil {
		if roleGroupConfig.Resources != nil {
			stsBuilder.WithResources(roleGroupConfig.Resources)
			// Opt-in data PVC. Whether the role HAS one is a structural property of the role, so it
			// comes from the declaration; how big it is and which class it uses come from the
			// effective config, so a user can size it.
			if dv := buildCtx.Declaration.DataVolume; dv != nil && roleGroupConfig.Resources.Storage != nil {
				stsBuilder.WithNamedStorage(dv.Name, roleGroupConfig.Resources.Storage, dv.MountPath)
			}
		}

		// Affinity: the CRD carries it as a schema-free RawExtension holding a corev1.Affinity.
		// Invalid JSON fails the build loudly — silently dropping a user's scheduling
		// constraints could place pods on nodes the user explicitly excluded (same loud-failure
		// stance as a Vector misconfiguration). Backward compatible: the framework sets affinity
		// only when the CRD config provides one, so products that post-process the built
		// StatefulSet with `if podSpec.Affinity == nil { ... }` default guards remain correct.
		if roleGroupConfig.Affinity != nil && len(roleGroupConfig.Affinity.Raw) > 0 {
			affinity, err := DecodeAffinity(roleGroupConfig.Affinity)
			if err != nil {
				return nil, fmt.Errorf("invalid affinity in role group config (role %q, group %q): %w",
					buildCtx.RoleName, buildCtx.RoleGroupName, err)
			}
			stsBuilder.WithAffinity(affinity)
		}

		// GracefulShutdownTimeout maps to the pod's terminationGracePeriodSeconds (see
		// docs/architecture.md §4.11.2 Graceful Shutdown). An unparsable or non-positive
		// duration fails the build loudly rather than silently falling back. A positive
		// sub-second duration rounds up to 1s so it never truncates to 0 (which would mean
		// immediate SIGKILL).
		//
		// GetGracefulShutdownTimeout supplies DefaultGracefulShutdownTimeout when neither the role
		// nor the group set one, so the value is always written explicitly rather than relying on
		// the Kubernetes default — the field used to carry a CRD default, and removing it must not
		// silently change the effective grace period of an existing cluster.
		timeout := roleGroupConfig.GetGracefulShutdownTimeout()
		d, err := time.ParseDuration(timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid gracefulShutdownTimeout %q in role group config (role %q, group %q): %w",
				timeout, buildCtx.RoleName, buildCtx.RoleGroupName, err)
		}
		if d <= 0 {
			return nil, fmt.Errorf("invalid gracefulShutdownTimeout %q in role group config (role %q, group %q): must be a positive duration",
				timeout, buildCtx.RoleName, buildCtx.RoleGroupName)
		}
		stsBuilder.WithTerminationGracePeriod(int64((d + time.Second - 1) / time.Second))
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

	h.wireVolumes(stsBuilder, buildCtx)

	// Name the primary container when the product needs a significant name (e.g. to match its
	// per-container logging key). This must reach the builder BEFORE Build(): podOverrides are
	// strategic-merged by container name inside Build(), so an override addressing the
	// user-facing name (e.g. "node") must find the primary container already carrying it — a
	// post-build rename would leave the override appended as a phantom, image-less container.
	// Sidecar injection below also sees the final name (the Vector provider RW-mounts the
	// shared log volume on the producer containers by name).
	if mainName := buildCtx.Declaration.MainContainerName; mainName != "" {
		stsBuilder.WithMainContainerName(mainName)
	}

	applyDeclaredContainerFields(stsBuilder, buildCtx.Declaration)

	// Build the StatefulSet
	sts := stsBuilder.Build()

	// A podOverrides mount at a mountPath the framework owns REPLACES the framework's mount
	// (strategic merge keys volumeMounts by mountPath, not by name). When the override also
	// declares its volume the result is a valid pod spec the API server accepts, with the config
	// ConfigMap no longer mounted anywhere — the product then starts on an empty config directory.
	// Refusing to build is the only way that stops being silent.
	if violations := stsBuilder.PodOverrideViolations(); len(violations) > 0 {
		return nil, NewValidationError("podOverrides", buildCtx.RoleName, buildCtx.RoleGroupName,
			stderrors.Join(violations...))
	}

	// Inject sidecars: prefer buildCtx (SDK auto-created), fallback to instance field
	sidecarMgr := buildCtx.SidecarManager
	if sidecarMgr == nil {
		sidecarMgr = h.sidecarManager
	}
	if sidecarMgr != nil {
		if err := sidecarMgr.InjectAll(&sts.Spec.Template.Spec); err != nil {
			// A sidecar provider refusing the assembled pod is a declaration fault the product
			// author can fix (a producer naming no container, an unusable log directory), so it
			// carries the same type as every other build-time rejection rather than an opaque wrap.
			return nil, NewValidationError("sidecar", buildCtx.RoleName, buildCtx.RoleGroupName, err)
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
	buildCtx *RoleBuildContext,
) *policyv1.PodDisruptionBudget {
	if buildCtx == nil || buildCtx.RoleSpec == nil {
		return nil
	}
	roleConfig := buildCtx.RoleSpec.GetRoleConfig()
	if roleConfig == nil || roleConfig.PodDisruptionBudget == nil {
		return nil
	}

	b := builder.NewPDBBuilder(
		RoleResourceName(buildCtx.ClusterName, buildCtx.RoleName), buildCtx.ClusterNamespace).
		WithLabels(h.buildRoleLabels(buildCtx)).
		WithSelector(h.buildRoleSelectorLabels(buildCtx.ClusterName, buildCtx.RoleName)).
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
			constant.LabelKubernetesInstance:  clusterName,
			constant.LabelKubernetesComponent: roleName,
			constant.LabelKubernetesManagedBy: managedByValue,
		}
	}
	return h.roleIdentityLabels(clusterName, roleName)
}

// buildRoleLabels is the role-scoped analogue of buildLabels for metadata on role-level
// resources: the recommended labels plus cluster/extra labels, but no role group label — a
// role-level resource covers every group of the role.
func (h *BaseRoleGroupHandler[CR]) buildRoleLabels(buildCtx *RoleBuildContext) map[string]string {
	labels := make(map[string]string)

	for k, v := range buildCtx.ClusterLabels {
		labels[k] = v
	}

	for k, v := range h.recommendedLabels(buildCtx.ClusterName, buildCtx.RoleName, "",
		buildCtx.ProductName, buildCtx.ProductVersion) {
		labels[k] = v
	}

	for k, v := range h.roleIdentityLabels(buildCtx.ClusterName, buildCtx.RoleName) {
		labels[k] = v
	}

	return labels
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

// applyDeclaredContainerFields puts the role declaration's primary-container fields on the builder.
//
// All of them land BEFORE Build(), so a user's podOverrides — strategic-merged inside Build() —
// still have the last word. They replace a general-purpose container callback that ran on the
// ASSEMBLED container, i.e. after the user's merged cliOverrides and env had already been written
// into it: a product setting Args or Env there silently deleted what the user wrote, which is what
// one shipped operator does today.
//
// Every field here has NO user layer, which is why declaring them beats nobody. Args are absent on
// purpose — they reach the container through cliOverrides, which the user can state.
func applyDeclaredContainerFields(b *builder.StatefulSetBuilder, decl RoleDeclaration) {
	if len(decl.Command) > 0 {
		b.WithCommand(decl.Command)
	}
	if decl.Lifecycle != nil {
		b.WithLifecycle(decl.Lifecycle)
	}
	if len(decl.Env) > 0 {
		b.WithBaseEnvVars(decl.Env)
	}
	// A nil readiness probe keeps the generated TCP probe on ContainerPorts[0]; nil liveness and
	// startup mean none, which is the framework's deliberate position rather than an omission.
	if decl.ReadinessProbe != nil {
		b.WithReadinessProbe(decl.ReadinessProbe)
	}
	if decl.LivenessProbe != nil {
		b.WithLivenessProbe(decl.LivenessProbe)
	}
	if decl.StartupProbe != nil {
		b.WithStartupProbe(decl.StartupProbe)
	}
}
