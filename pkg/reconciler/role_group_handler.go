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
// Reserved names: the framework already uses the pod volume/mount names "config" (the config
// ConfigMap volume, always present) and "data" (the data PVC, when StorageMountPath is set); a
// provider must not reuse either name, because duplicate volume names make the Kubernetes API
// server reject the pod — a hard reconcile failure.
type VolumeProvider interface {
	Volumes() []corev1.Volume
	VolumeMounts() []corev1.VolumeMount
}

// RoleBuildContext provides context for building role-level resources — those that cover every
// pod of a role across all of its role groups (today: the role's single PodDisruptionBudget).
// It is the role-scoped analogue of RoleGroupBuildContext and is built by GenericReconciler once
// per role.
//
// It is a struct rather than a positional argument list because role-level resources need the
// same identity inputs as role group ones — including ClusterSpec, from which the
// app.kubernetes.io/version label is derived — and a struct lets a later input be added without
// breaking every handler that builds a role-level resource.
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

	// RoleGroupSpec is the role group specification.
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
	// WRITTEN BY THE FRAMEWORK, read by the build path. Assigning it has no effect — the reconciler
	// has already resolved the image and the Vector gates from it by the time BuildResources runs.
	// A zero value means no RoleProvider is registered.
	Declaration RoleDeclaration

	// ResolvedImage is the role's image and everything that follows from it — pull policy, pull
	// secret, product version — resolved ONCE by the reconciler from spec.image over the role's
	// declared Image over the operator's ImageDefaults.
	//
	// It is one struct because its four consumers must agree: the primary container, the sidecars
	// (a Vector agent ships inside the product image, so it must be the SAME reference), the pod's
	// imagePullSecrets and the app.kubernetes.io/version label. They used to be derived
	// independently at four call sites, which is how the pull secret got silently dropped for ten
	// product CRDs at once.
	//
	// WRITTEN BY THE FRAMEWORK. An empty Reference means nobody expressed an opinion and the
	// handler falls back to whatever image it would have used anyway.
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

	// VectorLogPipelineActive is the resolved answer to "will the Vector sidecar actually be
	// injected into this role group's pods?" — the agent is enabled AND at least one producer is
	// declared AND something supplies vector.yaml (see GenericReconciler.buildSidecarManager,
	// which populates it). The Vector provider owns the shared log emptyDir and its mounts, so
	// the logging renderers gate the rolling file appender on this rather than on the enablement
	// flag alone: a skipped sidecar means no shared volume, and a file appender would send the
	// product's logs to an unmounted path.
	//
	// Nil means the build context was not produced by GenericReconciler; the renderers then fall
	// back to the enablement flag.
	VectorLogPipelineActive *bool
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
	if c.MergedConfig == nil || c.MergedConfig.Logging == nil {
		return nil
	}
	if cfg, ok := c.MergedConfig.Logging.Containers[container]; ok {
		return &cfg
	}
	return nil
}

// RenderContainerLogging is a build-context convenience over productlogging.RenderConfigFile:
// it resolves the container's merged logging spec from the build context and renders the
// config file. Handlers embedding BaseRoleGroupHandler get this wired automatically via
// LoggingContainers; handlers that build their own ConfigMap can call it directly.
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
		// A producer whose config file the PRODUCT writes still counts as a producer everywhere
		// else — the shared log volume, its mount, the pre-created directory, the Vector source —
		// but the framework renders nothing for it. Rendering one would collide with the key the
		// product writes itself and fail the role group.
		if lc.OwnConfigFile {
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
