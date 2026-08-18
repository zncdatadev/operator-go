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
	"path"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/config"
	"github.com/zncdatadev/operator-go/pkg/constant"
	"github.com/zncdatadev/operator-go/pkg/productlogging"
	"github.com/zncdatadev/operator-go/pkg/sidecar"
	"github.com/zncdatadev/operator-go/pkg/vector"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RoleGroupResources contains all Kubernetes resources for a role group.
// Each role group maps to exactly one StatefulSet and its associated resources.
//
// The framework owns the NAME and NAMESPACE of every fixed slot below; the handler owns their
// content. The names are derived from RoleGroupBuildContext.ResourceName —
// "<resource>" for the ConfigMap, Service, StatefulSet and PodDisruptionBudget,
// "<resource>-headless" and "<resource>-metrics" for the two suffixed Services — and both paths
// that REMOVE a slot address it by that name: the in-spec reclaim when a handler stops shipping
// one, and RoleGroupCleaner's teardown when the role group leaves the spec. A slot filled under a
// different name is therefore applied and owner-referenced but reclaimed by nothing, surviving
// until the cluster CR itself is deleted. Rather than leak it silently, the framework fails the
// role group with a *ValidationError before applying anything (see validateRoleGroupResources).
//
// ExtraResources takes the other branch of that trade: the product owns those names, so their
// reclaim is label-based and opt-in.
type RoleGroupResources struct {
	// StatefulSet is the main workload resource.
	StatefulSet *appsv1.StatefulSet

	// ConfigMap contains configuration files for the role group.
	ConfigMap *corev1.ConfigMap

	// Service is the client-facing service (optional).
	Service *corev1.Service

	// HeadlessService is the headless service for StatefulSet network identity.
	HeadlessService *corev1.Service

	// PodDisruptionBudget is an optional escape hatch for a custom, role-group-scoped PDB.
	// The framework's own PDB (from roleConfig.podDisruptionBudget) is a role-level resource
	// covering all of a role's groups and is emitted once per role by the generic reconciler
	// (see BaseRoleGroupHandler.BuildRolePodDisruptionBudget); it is NOT set here. Leave this
	// nil unless a product deliberately needs an extra per-group PDB.
	PodDisruptionBudget *policyv1.PodDisruptionBudget

	// MetricsService is a headless service with Prometheus scrape annotations (optional).
	MetricsService *corev1.Service

	// ExtraResources are additional product-specific resources for this role group that the
	// framework's fixed fields have no slot for — e.g. a listeners.kubedoop.dev Listener CR
	// that the pods reference by name through an ephemeral CSI volume. They flow through the
	// same apply path as the fixed fields: each object gets a controller owner reference to
	// the cluster CR (so it is garbage-collected when the CR is deleted) and is created or
	// updated idempotently. Each object's type must be registered in the reconciler's scheme,
	// and products should label extras with the role group's labels (see
	// BaseRoleGroupHandler.SelectorLabels) like any other resource they build.
	//
	// Ordering: extras are applied after the ConfigMap and Services but BEFORE the
	// StatefulSet, in slice order. Extras are typically prerequisites for pod scheduling —
	// e.g. a Listener CR must exist before the pods that mount its CSI volume are created,
	// otherwise the pods hang in ContainerCreating.
	//
	// Cleanup: extras of a removed or renamed role group ARE reclaimed, provided two things hold.
	// The product registers their kinds — which it does by listing them in
	// SetupWithManagerOptions.ExtraOwns, the same declaration that gives them watches — and it
	// stamps the role group's labels on them (see BaseRoleGroupHandler.SelectorLabels). The cleaner
	// then deletes objects of those kinds that carry the departing group's identity labels AND this
	// CR's controller owner reference; both are required, because instance is the cluster NAME and
	// a namespace can hold another product's CR under the same one.
	//
	// Unlabelled extras, or kinds never registered, keep the old behaviour: they survive until the
	// cluster CR is deleted and owner-reference GC collects them. The cleaner has no other handle
	// on them, so it fails closed rather than guessing.
	//
	// Constraints. The type is deliberately open — arbitrary GVKs are the point — but three
	// properties are checked before ANY resource of the role group is applied, and each failure is a
	// *ValidationError naming the offending index (see validateExtraResources):
	//
	//   - every entry has a name. CreateOrUpdate addresses the object by name each reconcile, so
	//     metadata.generateName cannot converge one object here — it would create another.
	//   - every entry lives in the cluster's namespace. The framework sets a controller owner
	//     reference, and Kubernetes honours neither a cross-namespace one nor a namespaced owner
	//     for a cluster-scoped object; a cluster-scoped resource therefore has no lifecycle the
	//     framework can give it, and the product owns it outright, cleanup included.
	//   - no two entries — nor an entry and a fixed slot above — address the same object (same GVK
	//     and name). Two writers in one pass mean the later apply wins and the other is silently
	//     discarded, and when the two desired states differ the object is rewritten every reconcile,
	//     each write waking the framework's own watch with nothing to back the loop off.
	//
	// A nil/empty slice behaves exactly as before this field existed; nil entries are skipped.
	ExtraResources []client.Object
}

// VolumeProvider supplies extra pod volumes and their container mounts (typically CSI
// volumes such as secret/TLS certificates or listener address volumes). Products register
// providers on the RoleGroupBuildContext before the base handler builds the StatefulSet;
// the base handler injects each provider's Volumes() and VolumeMounts() through the same
// builder path as the config volume. Both pkg/security.SecretProvisioner and
// pkg/listener.ListenerProvisioner satisfy this interface.
//
// Reserved names: the framework already uses the pod volume/mount name "config" (the config
// ConfigMap volume, always present), and the name of the role's data volume when
// RoleDeclaration.DataVolume is set — builder.DefaultDataVolumeName ("data") unless the
// declaration names it. A provider must not reuse either, because duplicate volume names make the
// Kubernetes API server reject the pod — a hard reconcile failure.
type VolumeProvider interface {
	Volumes() []corev1.Volume
	VolumeMounts() []corev1.VolumeMount
}

// RoleBuildContext provides context for building role-level resources — those that cover every
// pod of a role across all of its role groups (today: the role's single PodDisruptionBudget).
// Its ProductName and ProductVersion are what put app.kubernetes.io/name and /version on the role
// PDB, so it carries the same identity as every other resource of the role.
//
// It is the role-scoped analogue of RoleGroupBuildContext and is built by GenericReconciler once
// per role.
//
// It is a struct rather than a positional argument list because role-level resources need the same
// identity inputs as role group ones — ProductName and ProductVersion, which carry
// app.kubernetes.io/name and /version — and a struct lets a later input be added without breaking
// every handler that builds a role-level resource.
type RoleBuildContext struct {
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

	// ProductName is GenericReconcilerConfig.ImageResolution.ProductName, stamped as
	// app.kubernetes.io/name. Empty means the product resolves its own images and the label is
	// omitted.
	//
	// It is on the ROLE context too, and not only the role group's, because the role-level PDB is
	// built from here. Losing the label there breaks no selector — the reclaim keys on
	// instance + managed-by + component + role-group — which is exactly why it would go unnoticed.
	ProductName string

	// ProductVersion is the resolved product version, stamped as app.kubernetes.io/version. It
	// follows the RESOLVED image, so it is present whenever the version came from the operator's
	// own defaults too, and empty when the framework did not assemble the reference.
	ProductVersion string
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

	// RoleGroupSpec is the role group specification, with ONE substitution: its Config field
	// carries the EFFECTIVE config — RoleDeclaration.ConfigDefaults, the CR's role level and the
	// CR's role group level already folded into one answer by FoldCommonConfig — not the raw
	// per-group block the user wrote. Read it through EffectiveConfig().
	//
	// The fold is substituted in rather than published beside the raw spec because there is no
	// legitimate consumer of the unfolded value: a product reading it would re-derive an answer
	// the framework already computed, and would get it wrong in exactly the way #631 describes —
	// a role group that states one field of `resources` does not thereby decline the rest.
	RoleGroupSpec v1alpha1.RoleGroupSpec

	// MergedConfig is the folded override stack: the product's derived contribution beneath the
	// CR's role level beneath its role group level.
	//
	// It is NIL inside RoleGroupResolver, and deliberately so: that stage runs before this is
	// merged, because its return value is the bottom layer of it. A resolver deriving from the
	// effective TYPED config reads RoleGroupSpec.Config, which is already folded by then.
	MergedConfig *config.MergedConfig

	// ResourceName is the derived resource name for the role group: "{cluster}-{role}-{group}"
	// (see RoleGroupResourceName, which also truncates over-long names with a hash suffix). The
	// role segment prevents collisions between same-named groups of different roles.
	ResourceName string

	// ServiceAccountName is the name of the ServiceAccount the workload pods run as. The
	// GenericReconciler DERIVES it from the CR (ServiceAccountResourceName: "<lowercased
	// kind>-<cluster>") and ensures that ServiceAccount before any role is built, so a handler
	// can rely on it being both non-empty and already existing. The base StatefulSet builder
	// binds it to the pod template via WithServiceAccount.
	//
	// It is not configurable: the framework owns this name the same way it owns ResourceName
	// above. A handler constructing a RoleGroupBuildContext by hand (in a test, say) may leave it
	// empty, and the base handler then binds nothing — pods fall back to the namespace default.
	ServiceAccountName string

	// SidecarManager is the sidecar manager for this role group, always set (non-nil) by
	// GenericReconciler. Built-in sidecars (e.g. Vector when EnableVectorAgent is set) are
	// pre-registered; products register their own containers (e.g. init containers via
	// sidecar.StaticContainerProvider) and call InjectAll so all pod container injection
	// flows through the manager. May be empty if nothing is configured.
	SidecarManager *sidecar.SidecarManager

	// VolumeProviders supply extra pod volumes + mounts (CSI secret/listener volumes) that the
	// base handler injects into the StatefulSet. This is per-build-context (rebuilt every
	// reconcile), so registrations never accumulate across reconciles or leak across CRs. A
	// product appends its provisioners here (e.g. buildCtx.VolumeProviders = append(...)) before
	// calling BaseRoleGroupHandler.BuildResources. Empty means no extra volumes (backward compatible).
	VolumeProviders []VolumeProvider

	// Declaration is the product's RoleDeclaration for this role group's role, as returned by
	// GenericReconcilerConfig.RoleProvider for THIS cr. It is the single source for everything a
	// role's shape is made of: ports, container name, command, data volume, log producers, probes.
	//
	// WRITTEN BY THE FRAMEWORK. A zero value means no RoleProvider is registered.
	//
	// Assigning it in a BuildResources override is a HALF-honoured change, which is worse than one
	// wholly ignored: the image, the config fold and the Vector gates are already settled from it
	// by the time BuildResources runs, while ports, container name, command, probes, data volume,
	// listener class and log producers are read during the build and WOULD take effect. Declare the
	// role in RoleProvider, where every consumer sees the same answer.
	Declaration RoleDeclaration

	// ResolvedImage is the role's image and everything that follows from it — pull policy, pull
	// secret, product version — resolved ONCE by the reconciler from spec.image over the role's
	// declared Image over ImageResolution.Defaults.
	//
	// It is one struct because its four consumers must agree: the primary container, the sidecars
	// (a Vector agent ships inside the product image, so it must be the SAME reference), the pod's
	// imagePullSecrets and the app.kubernetes.io/version label. They used to be derived
	// independently at four call sites, which is how the pull secret got silently dropped for ten
	// product CRDs at once.
	//
	// WRITTEN BY THE FRAMEWORK. There is no fallback behind an empty Reference: on the
	// BaseRoleGroupHandler path it fails the role group with a *ValidationError naming the knobs
	// that could set it (see ResolvedImage.Reference).
	ResolvedImage ResolvedImage

	// ProductName is GenericReconcilerConfig.ImageResolution.ProductName, stamped as
	// app.kubernetes.io/name. Empty means the product resolves its own images and the label is
	// omitted.
	ProductName string

	// VectorAggregatorAddress is the resolved Vector aggregator discovery address, populated by
	// GenericReconciler when the Vector agent is enabled and the CR implements
	// VectorAggregatorProvider (the reconciler reads its ConfigMap name and resolves the address
	// via discovery). When set, RenderLoggingConfigMapData generates vector.yaml; empty means the
	// framework does not own vector.yaml for this role group (the product builds it, or Vector is
	// off).
	VectorAggregatorAddress string

	// vectorLogPipelineActive is the resolved answer to "will the Vector sidecar actually be
	// injected into this role group's pods?" — the agent is enabled AND at least one producer is
	// declared AND something supplies vector.yaml (see GenericReconciler.buildSidecarManager,
	// which populates it). The Vector provider owns the shared log emptyDir and its mounts, so
	// the logging renderers gate the rolling file appender on this rather than on the enablement
	// flag alone: a skipped sidecar means no shared volume, and a file appender would send the
	// product's logs to an unmounted path.
	//
	// It is UNEXPORTED because every input to it — logging.enableVectorAgent from the folded
	// config, the producer list and the vector.yaml source from the role declaration — is already
	// the framework's, so the framework settles the chain and nothing else re-derives it. A product
	// that renders its own logging config asks LogFileTarget where the file goes; that is the
	// conclusion, and the only thing a product can act on without re-deciding.
	//
	// Nil means the build context was not produced by GenericReconciler; the renderers then fall
	// back to the enablement flag.
	vectorLogPipelineActive *bool
}

// VectorAggregatorProvider is optionally implemented by a product CR to expose the name of the
// ConfigMap carrying the Vector aggregator discovery address (typically
// spec.clusterConfig.vectorAggregatorConfigMapName). When the CR implements it and the Vector
// agent is active for a role group (enabled AND at least one declared producer), GenericReconciler
// resolves the aggregator address and generates vector.yaml into the role group ConfigMap.
//
// Returning "" means no aggregator ConfigMap is configured. When the Vector agent is active this is
// a misconfiguration and the reconciler fails loudly (there would otherwise be a Vector sidecar
// with no aggregator to ship to); when the agent is not active the method is not consulted.
type VectorAggregatorProvider interface {
	VectorAggregatorConfigMapName() string
}

// ContainerLogging returns the deep-merged logging config for a container (keyed by
// container name), or nil when the product CRD configured no logging for it. The declaration
// type and rendering live in pkg/productlogging; this accessor must live here because it is a
// method on the reconciler's RoleGroupBuildContext.
func (c *RoleGroupBuildContext) ContainerLogging(container string) *v1alpha1.LoggingConfigSpec {
	// The FOLD, not MergedConfig.Logging, for the same reason vectorEnabledFor reads it: a product
	// calls this from its RoleGroupResolver, which runs in stage 2 — before stage 3 assigns
	// MergedConfig. Reading the copy returned nil there, so a product rendering its own logging
	// config saw no per-container settings at all and silently emitted defaults, with the user's
	// levels dropped and nothing reporting it. MergedConfig.Logging is assigned from this same
	// value in stage 3, so this is the source rather than a second opinion.
	logging := c.RoleGroupSpec.GetConfig().Logging
	if logging == nil && c.MergedConfig != nil {
		// A context assembled by hand carries no folded spec.
		logging = c.MergedConfig.Logging
	}
	if logging == nil {
		return nil
	}
	if cfg, ok := logging.Containers[container]; ok {
		return &cfg
	}
	return nil
}

// RenderContainerLogging is a build-context convenience over productlogging.RenderConfigFile:
// it resolves the container's merged logging spec from the build context and renders the
// config file. Handlers embedding BaseRoleGroupHandler get this wired automatically from
// RoleDeclaration.LogProducers; handlers that build their own ConfigMap can call it directly.
func RenderContainerLogging(buildCtx *RoleGroupBuildContext, decl productlogging.ContainerLogging) (string, string, error) {
	// Emit the rolling file appender only when the Vector sidecar is really injected: file logging
	// is coupled to Vector, which owns the shared log volume the appender writes into. Gating here
	// means products building their own ConfigMap inherit the behavior for free.
	return productlogging.RenderConfigFile(
		buildCtx.ContainerLogging(decl.Container), decl, vectorLogPipelineActive(buildCtx))
}

// RenderLoggingConfigMapData renders the logging-related entries for a role group ConfigMap:
//   - one logging config file per declared producer (level config, plus the rolling file appender
//     when the Vector sidecar is injected), keyed by the generator file name (e.g. "logback.xml"), and
//   - the Vector agent config ("vector.yaml") when the Vector agent is enabled AND the aggregator
//     address has been resolved (buildCtx.VectorAggregatorAddress, populated by GenericReconciler
//     from the CR's VectorAggregatorProvider).
//
// The Vector sidecar reads its config from the role group ConfigMap (the provider mounts it and
// runs "vector --config <mount>/vector.yaml"), so the framework owns vector.yaml generation from
// the shared log-dir convention — products implementing VectorAggregatorProvider no longer
// hand-write it and cannot drift from the source glob. Products that build their own ConfigMap
// compose this map into theirs (checking for collisions against their own keys); handlers
// embedding BaseRoleGroupHandler get it automatically. Returns an empty map when there are no
// producers and Vector is disabled.
func RenderLoggingConfigMapData(buildCtx *RoleGroupBuildContext, producers []productlogging.ContainerLogging) (map[string]string, error) {
	// Whole-list checks first: an unusable log directory or two producers writing the same log
	// file are properties of the declaration set, not of any one entry, and both must fail the
	// role group before a single resource is applied.
	if err := productlogging.ValidateProducers(producers); err != nil {
		return nil, NewValidationError("logging", buildCtx.RoleName, buildCtx.RoleGroupName, err)
	}

	data := make(map[string]string)
	for _, lc := range producers {
		// No framework means the PRODUCT writes this container's config file. It still counts as a
		// producer everywhere else — the shared log volume, its mount, the pre-created directory,
		// the Vector source — but the framework renders nothing for it, so there is no key to
		// collide with the one the product writes.
		if lc.Framework == "" {
			continue
		}
		filename, content, err := RenderContainerLogging(buildCtx, lc)
		if err != nil {
			return nil, fmt.Errorf("failed to render logging config for container %q: %w", lc.Container, err)
		}
		if _, exists := data[filename]; exists {
			return nil, fmt.Errorf("logging config file %q for container %q collides with another logging entry", filename, lc.Container)
		}
		data[filename] = content
	}
	// Generate vector.yaml only when the aggregator address is known. If Vector is enabled but the
	// CR does not expose an aggregator ConfigMap (VectorAggregatorProvider), the address is empty
	// and the framework leaves vector.yaml to the product.
	if vectorLogPipelineActive(buildCtx) && buildCtx.VectorAggregatorAddress != "" {
		vectorConfig, err := vector.RenderVectorConfig(vector.VectorConfigData{
			LogDir:            constant.KubedoopLogDir,
			AggregatorAddress: buildCtx.VectorAggregatorAddress,
			Namespace:         buildCtx.ClusterNamespace,
			ClusterName:       buildCtx.ClusterName,
			RoleName:          buildCtx.RoleName,
			RoleGroupName:     buildCtx.RoleGroupName,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to render vector config: %w", err)
		}
		if _, exists := data[vector.VectorConfigFileName]; exists {
			return nil, fmt.Errorf("vector config file %q collides with a logging config file", vector.VectorConfigFileName)
		}
		data[vector.VectorConfigFileName] = vectorConfig
	}
	return data, nil
}

// RoleGroupHandler is the interface that product operators must implement
// to define how resources are built for each role group.
//
// The GenericReconciler handles the "when" and "how to apply" resources,
// while the RoleGroupHandler handles the "what" - the product-specific resource definitions.
//
// Implementations can embed BaseRoleGroupHandler to get default behaviour for
// common resources (ConfigMap, Services, StatefulSet, PDB). Override BuildResources
// or individual build steps as needed for product-specific logic.
type RoleGroupHandler[CR common.ClusterInterface] interface {
	// BuildResources builds all Kubernetes resources for a role group.
	// The GenericReconciler will apply these resources in the correct order.
	//
	// Implementations should:
	// 1. Use the build context to get cluster info, labels, and merged config
	// 2. Build product-specific ConfigMap data
	// 3. Build StatefulSet with appropriate containers, volumes, etc.
	// 4. Build Services if needed
	//
	// Do NOT build a PodDisruptionBudget per role group: the PDB is role-level and
	// framework-built once per role from roleConfig.podDisruptionBudget
	// (BaseRoleGroupHandler.BuildRolePodDisruptionBudget). RoleGroupResources.PodDisruptionBudget
	// is an escape hatch for exceptional per-group budgets only.
	//
	// Returns RoleGroupResources containing all built resources, or an error.
	BuildResources(ctx context.Context, k8sClient client.Client, cr CR, buildCtx *RoleGroupBuildContext) (*RoleGroupResources, error)
}

// RoleGroupHandlerFuncs is an adapter to allow using functions as RoleGroupHandler.
// This is useful for simple handlers that don't need a full struct.
type RoleGroupHandlerFuncs[CR common.ClusterInterface] struct {
	// BuildResourcesFunc is the function for building resources.
	BuildResourcesFunc func(ctx context.Context, k8sClient client.Client, cr CR, buildCtx *RoleGroupBuildContext) (*RoleGroupResources, error)
}

// BuildResources implements RoleGroupHandler.
func (f *RoleGroupHandlerFuncs[CR]) BuildResources(ctx context.Context, k8sClient client.Client, cr CR, buildCtx *RoleGroupBuildContext) (*RoleGroupResources, error) {
	if f.BuildResourcesFunc == nil {
		return &RoleGroupResources{}, nil
	}
	return f.BuildResourcesFunc(ctx, k8sClient, cr, buildCtx)
}

// Verify that RoleGroupHandlerFuncs implements RoleGroupHandler.
var _ RoleGroupHandler[common.ClusterInterface] = &RoleGroupHandlerFuncs[common.ClusterInterface]{}

// copyQuantity deep-copies a quantity pointer, preserving nil. resource.Quantity.DeepCopy has a
// VALUE receiver, so calling it through a nil *Quantity dereferences and panics — which is exactly
// the case this merge hits whenever a role leaves one leaf of a resource block unset.
func copyQuantity(q *resource.Quantity) *resource.Quantity {
	if q == nil {
		return nil
	}
	out := q.DeepCopy()
	return &out
}

// copyString is copyQuantity for the pointer fields whose element is a plain string. Inheriting the
// role's pointer directly would leave the merged config aliasing the live CR spec, which is what
// every other branch of this merge deep-copies to avoid.
func copyString(s *string) *string {
	if s == nil {
		return nil
	}
	return ptr.To(*s)
}

// EffectiveConfig returns this role group's folded framework-owned config: the product's
// RoleDeclaration.ConfigDefaults, the CR's role `config` and the CR's role group `config`, resolved
// into one answer by FoldCommonConfig.
//
// It is the input a RoleGroupResolver derives from, and it is never nil — a role group that states
// no config at all yields an empty struct (RoleGroupSpec.GetConfig's guarantee) rather than
// requiring every caller to nil-check before reaching for a field.
//
// A product's OWN config fields are not here: those are the product's half of the embedded struct,
// folded by the product with FoldProductConfig.
func (c *RoleGroupBuildContext) EffectiveConfig() *v1alpha1.RoleGroupConfigSpec {
	return c.RoleGroupSpec.GetConfig()
}

// LogFileTarget returns the path a producer's rolling log file must be written to, or "" meaning
// console only.
//
// It exists for a product that renders its own logging config file — Airflow's log_config.py, which
// must be built on Airflow's own DEFAULT_LOGGING_CONFIG and so can never be a rendered template.
// Such a product declares its producer with an empty Framework and calls this to learn where the
// file goes.
//
// It hands back a CONCLUSION, not the inputs to one. Whether the Vector pipeline is active is a
// pure function of things the framework already holds — logging.enableVectorAgent from the folded
// config, the producer list and the vector.yaml source from the declaration — so the framework
// settles it and nothing else re-derives it. Exposing the boolean instead would make the product a
// second participant in a decision already made, and it would leave the product composing the path
// itself from LogDirFor and ContainerLogFileName: a composition that is correct while Vector is on
// and silently wrong the moment it is off, because the appender then writes into the container's
// writable layer where nothing collects it.
//
// The empty return is not a failure. It means this role group has no shared log volume this cycle,
// so a file appender would write nowhere useful and the product should emit a console-only config.
func (c *RoleGroupBuildContext) LogFileTarget(decl productlogging.ContainerLogging) string {
	if !vectorLogPipelineActive(c) {
		return ""
	}
	name := decl.LogFileName
	if name == "" {
		name = productlogging.ContainerLogFileName(decl.Framework, decl.Container)
	}
	if name == "" {
		return ""
	}
	return path.Join(productlogging.LogDirFor(decl), name)
}
