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
	"github.com/zncdatadev/operator-go/pkg/listener"
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
// Each role group maps to exactly one workload — a StatefulSet, or a Deployment for the roles
// whose handler declares WorkloadKindDeployment — and its associated resources.
//
// The framework owns the NAME and NAMESPACE of every fixed slot below; the handler owns their
// content. The names are derived from RoleGroupBuildContext.ResourceName —
// "<resource>" for the ConfigMap, Service, StatefulSet, Deployment and PodDisruptionBudget,
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
	// StatefulSet is the main workload resource for StatefulSet roles (the default kind).
	// Mutually exclusive with Deployment: a role group is exactly one workload, and the apply
	// path rejects a handler that fills both slots.
	StatefulSet *appsv1.StatefulSet

	// Deployment is the main workload resource for roles the handler declares as
	// WorkloadKindDeployment (BaseRoleGroupHandler.RoleWorkloadKinds). Mutually exclusive with
	// StatefulSet — both non-nil is a validation error at apply time, because two workloads under
	// one role group name would fight over the same pods, Services and status entry. It is applied
	// in exactly the StatefulSet's slot of the apply order (after ConfigMap, Services and extras;
	// before the PDB), since its prerequisites are the same.
	Deployment *appsv1.Deployment

	// ConfigMap contains configuration files for the role group.
	ConfigMap *corev1.ConfigMap

	// Service is the client-facing service (optional).
	Service *corev1.Service

	// HeadlessService is the headless service for StatefulSet network identity. Deployment role
	// groups have none: a headless Service exists to give each pod a stable DNS name, and a
	// Deployment's pods have no stable identity to name.
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

	// ResourceName is the derived resource name for the role group: "{cluster}-{role}-{group}"
	// (see RoleGroupResourceName, which also truncates over-long names with a hash suffix). The
	// role segment prevents collisions between same-named groups of different roles.
	ResourceName string

	// ServiceAccountName is the name of the ServiceAccount the workload pods should run as.
	// It is populated by GenericReconciler with the resolved SA name — the per-CR
	// ServiceAccountNameFunc result when set, else the static ServiceAccountName (the SA the
	// reconciler auto-creates). When non-empty, the base StatefulSet builder binds it to the
	// pod template via WithServiceAccount, so the created SA is actually used. Empty means no
	// binding — pods fall back to the namespace default SA (backward compatible).
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

	// Image, ImagePullPolicy, ContainerPorts and ServicePorts are the per-CR inputs a product
	// derives from the cluster it is building for. Set them here, NOT on the handler.
	//
	//	func (h *MyHandler) BuildResources(ctx context.Context, c client.Client, cr *MyCluster,
	//	    buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
	//	    buildCtx.ContainerPorts = h.portsFor(cr, buildCtx.RoleName) // depends on cr.Spec.Tls
	//	    return h.BaseRoleGroupHandler.BuildResources(ctx, c, cr, buildCtx)
	//	}
	//
	// The handler instance is constructed once in main.go and shared by the controller across
	// every CR and every reconcile, so the older idiom — assigning h.Image or calling
	// h.SetRoleContainerPorts from inside BuildResources — writes per-cluster values into
	// process-wide state. That has two failure modes, and the quieter one bites first:
	//
	//   - with MaxConcurrentReconciles above 1 the writes RACE, and one cluster's image or
	//     TLS-dependent ports are built into another cluster's workload;
	//   - even at the default concurrency of 1, a product that conditionally SKIPS one of those
	//     assignments silently inherits the previous CR's value. spark-k8s-operator shipped
	//     exactly that (a CR omitting pullPolicy took the last CR's), with a serial reconcile
	//     loop and no race involved.
	//
	// This context is rebuilt for every role group of every reconcile, so nothing set here can
	// reach another cluster. Unset fields fall back to the handler's own configuration, which is
	// where reconcile-INVARIANT settings belong (a static image, ProductName, ImageDefaults, the
	// ports of a product whose ports do not depend on the CR).
	//
	// Precedence, highest first: this context > the handler's per-role maps > the handler's
	// defaults. For the image specifically, the CR's own spec.image still outranks all three —
	// see BaseRoleGroupHandler.ImageDefaults.
	Image           string
	ImagePullPolicy corev1.PullPolicy
	ContainerPorts  []corev1.ContainerPort
	ServicePorts    []corev1.ServicePort

	// ListenerClass sets the client Service's type through listener.ServiceTypeFor, so the
	// Service is right when it is BUILT. Products used to patch Service.Spec.Type after
	// BuildResources returned, each re-deriving the same three-way switch — two of them with raw
	// string literals rather than the framework constants, and disagreeing about what
	// external-stable meant. Empty leaves the builder's default (ClusterIP).
	ListenerClass listener.ListenerClass

	// MainContainerCustomizer gets the assembled primary container just before podOverrides are
	// applied, which is the whole point: a user's podOverrides still outrank whatever the product
	// sets here, and a post-build edit would invert that precedence silently.
	//
	// The framework owns the primary container's name, image, ports, security context, volume
	// mounts and probes; everything else — command, args, env, probes the product wants to
	// replace, lifecycle hooks — has no other channel, so products reached into the returned
	// StatefulSet and edited it, re-deriving the container they had just been handed.
	// zookeeper-operator located it as Containers[0], which the framework has never promised: the
	// moment a sidecar provider inserts a container earlier, that configures the wrong one.
	//
	// Returning an error fails the role group with a *ValidationError. Changing the image is
	// rejected the same way — it is resolved once and propagated to the sidecars before the
	// StatefulSet is built, so set Image above instead.
	MainContainerCustomizer func(*corev1.Container) error

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

// VectorConfigProvider is optionally implemented by a RoleGroupHandler whose product writes the
// Vector agent config ("vector.yaml") into the role group ConfigMap itself, instead of letting the
// framework generate it from the CR's VectorAggregatorProvider.
//
// It exists because the Vector sidecar runs "vector --config <config mount>/vector.yaml": with
// neither source that key is missing, the container cannot start, and the sidecar's own dependency
// validation fails the cluster's reconcile every cycle. GenericReconciler therefore registers the
// Vector provider only when the CR implements VectorAggregatorProvider or the handler answers true
// here — implementing this interface is the product's assertion that its ConfigMap carries the key
// for that role.
type VectorConfigProvider interface {
	// ProvidesVectorConfig reports whether the handler writes vector.yaml for the given role.
	// roleName selects the declaration, mirroring LoggingProducers: a product may build the file
	// for some roles and leave the rest to the framework.
	ProvidesVectorConfig(roleName string) bool
}

// LoggingProducerProvider is implemented by handlers (e.g. BaseRoleGroupHandler) that declare
// log-producer containers. GenericReconciler type-asserts its handler against this interface to
// configure the Vector sidecar (the single owner of the shared log volume) without depending on a
// concrete handler type.
type LoggingProducerProvider interface {
	// LoggingProducers returns the declared log-producer containers for the given role (the
	// containers whose log files Vector collects; the provider RW-mounts the shared log volume on
	// each). roleName selects the declaration: an implementation may return a role-specific list
	// (e.g. a per-role container name), falling back to a global default for roles without an
	// override. The GenericReconciler calls it once per role group, passing buildCtx.RoleName.
	LoggingProducers(roleName string) []productlogging.ContainerLogging
	// LogVolumeSizeLimit returns the shared log volume SizeLimit override; "" uses the framework
	// default (vector.DefaultLogVolumeSize).
	LogVolumeSizeLimit() string
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
	data := make(map[string]string)
	for _, lc := range producers {
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

// MergeRoleGroupConfig merges a role-level RoleGroupConfigSpec into a role group's config, the
// group winning per LEAF (not per struct). The inputs are never mutated; the result is always a
// fresh copy (or nil when both inputs are nil). GenericReconciler applies this when building the
// RoleGroupBuildContext, so role-wide config defaults reach every group.
//
// Leaf granularity is the whole point. Merging `resources` struct-by-struct meant a group that
// set one leaf — a storageClass, a cpu.min — discarded every sibling the role had configured,
// because the group's enclosing struct was non-nil and the role's was therefore never consulted.
// Overriding one knob is the normal way to use this API, and it silently dropped the rest.
//
// This works only because the fields are pointers with no CRD-level default. Structural
// defaulting fills a field as soon as its enclosing object exists, so a `+kubebuilder:default`
// on a leaf makes "unset here" indistinguishable from "explicitly the default" and the role's
// value can never win. Defaults therefore live at consumption time — StorageResource.GetCapacity,
// RoleGroupConfigSpec.GetGracefulShutdownTimeout — not in the schema.
func MergeRoleGroupConfig(role, group *v1alpha1.RoleGroupConfigSpec) *v1alpha1.RoleGroupConfigSpec {
	switch {
	case role == nil && group == nil:
		return nil
	case role == nil:
		return group.DeepCopy()
	case group == nil:
		return role.DeepCopy()
	}

	merged := group.DeepCopy()
	// Affinity is replaced wholesale, NOT merged member by member — the one field in this struct
	// that does not fold per leaf, and deliberately so. `resources` is a set of independent knobs
	// where overriding cpu.max and keeping the rest is the normal thing to want; an affinity is a
	// single scheduling POLICY, and a role group that needs different scheduling needs a different
	// policy rather than a partial edit of the role's. Kubernetes treats PodSpec.Affinity the same
	// way: Helm values and Kustomize patches replace it, they do not interleave it.
	//
	// Merging per member was tried and reverted: it invented a semantic users would have to learn,
	// and it removed the only way to say "this group has no affinity" — with inheritance per member,
	// `affinity: {}` inherits everything and nothing expresses the empty policy a single-node group
	// wants. Replacement keeps that: an absent affinity inherits the role's, `{}` clears it.
	if merged.Affinity == nil {
		merged.Affinity = role.Affinity.DeepCopy()
	}
	if merged.GracefulShutdownTimeout == nil {
		merged.GracefulShutdownTimeout = role.GracefulShutdownTimeout
	}
	// MergeLoggingSpec may return one of its inputs unchanged (e.g. a nil counterpart);
	// deep-copy so the merged config never aliases the live CR spec.
	merged.Logging = productlogging.MergeLoggingSpec(role.Logging, group.Logging).DeepCopy()
	merged.Resources = mergeResources(role.Resources, group.Resources)
	return merged
}

// mergeResources folds the role's resources into the group's, leaf by leaf.
func mergeResources(role, group *v1alpha1.ResourcesSpec) *v1alpha1.ResourcesSpec {
	switch {
	case role == nil && group == nil:
		return nil
	case role == nil:
		return group.DeepCopy()
	case group == nil:
		return role.DeepCopy()
	}

	merged := group.DeepCopy()

	switch {
	case merged.CPU == nil:
		merged.CPU = role.CPU.DeepCopy()
	case role.CPU != nil:
		if merged.CPU.Min == nil {
			merged.CPU.Min = copyQuantity(role.CPU.Min)
		}
		if merged.CPU.Max == nil {
			merged.CPU.Max = copyQuantity(role.CPU.Max)
		}
	}

	switch {
	case merged.Memory == nil:
		merged.Memory = role.Memory.DeepCopy()
	case role.Memory != nil:
		if merged.Memory.Limit == nil {
			merged.Memory.Limit = copyQuantity(role.Memory.Limit)
		}
	}

	switch {
	case merged.Storage == nil:
		merged.Storage = role.Storage.DeepCopy()
	case role.Storage != nil:
		if merged.Storage.Capacity == nil {
			merged.Storage.Capacity = copyQuantity(role.Storage.Capacity)
		}
		if merged.Storage.StorageClass == nil {
			merged.Storage.StorageClass = copyString(role.Storage.StorageClass)
		}
	}

	return merged
}

// ApplyProductDefaults folds an overrides layer UNDERNEATH the already-merged config: a value the
// user set anywhere in the CRD is kept, and only what they left unset is contributed.
//
// It is the imperative half of the product-config seam, for the case GenericReconcilerConfig's
// ProductConfig hook cannot serve: a product whose configuration depends on an API lookup — an
// S3Connection reference resolved to an endpoint, a ZooKeeper connection string built from the
// live resources — does that lookup inside BuildResources, where a ctx and a client already exist,
// and only then knows what to contribute.
//
//	func (h *MyHandler) BuildResources(ctx context.Context, c client.Client, cr *MyCluster,
//	    buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
//	    conn, err := s3.ResolveConnection(ctx, c, cr.Namespace, inline, ref)
//	    if err != nil {
//	        return nil, err
//	    }
//	    buildCtx.ApplyProductDefaults(&commonsv1alpha1.OverridesSpec{
//	        ConfigOverrides: map[string]map[string]string{"hive-site.xml": conn.S3AProperties()},
//	    })
//	    return h.BaseRoleGroupHandler.BuildResources(ctx, c, cr, buildCtx)
//	}
//
// hive-operator and spark-k8s-operator each hand-wrote the same "set only keys the user did not
// set" helper for exactly this, byte for byte including its doc comment, and both then discovered
// the same second rule for environment variables — that product defaults must not overwrite
// envOverrides. Both rules are the framework's own merge precedence, so they belong here rather
// than being re-derived per product.
//
// A nil layer, or a nil MergedConfig, is a no-op.
func (c *RoleGroupBuildContext) ApplyProductDefaults(defaults *v1alpha1.OverridesSpec) {
	if c == nil || defaults == nil || c.MergedConfig == nil {
		return
	}
	c.MergedConfig = config.NewConfigMerger().MergeBeneath(c.MergedConfig, defaults)
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
