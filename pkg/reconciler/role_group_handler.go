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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"

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
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
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
	// Cleanup: RoleGroupCleaner only deletes the framework's fixed, well-known resources
	// (PDB, StatefulSet, ConfigMap, Services) when a role group is removed or renamed; it
	// cannot discover arbitrary-GVK extras. Extras of a removed role group therefore remain
	// until the cluster CR itself is deleted (owner-reference GC). Products that need eager
	// removal must delete such extras themselves (e.g. in a role group extension).
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
	merged.Affinity = mergeAffinity(role.Affinity, group.Affinity)
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
		if merged.Storage.StorageClass == "" {
			merged.Storage.StorageClass = role.Storage.StorageClass
		}
	}

	return merged
}

// affinityFields are the top-level members of corev1.Affinity. mergeAffinity folds a role's
// affinity into a role group's one member at a time, so the list has to be the complete set: a
// member missing from here would silently stop being inheritable.
var affinityFields = []string{"nodeAffinity", "podAffinity", "podAntiAffinity"}

// DecodeAffinity decodes the schema-free `config.affinity` RawExtension into a corev1.Affinity,
// REJECTING fields the type does not have.
//
// The CRD declares this field `type: object` with `x-kubernetes-preserve-unknown-fields: true`, so
// the API server neither validates nor prunes it — and a plain json.Unmarshal ignores what it does
// not recognise. The two together made a typo completely silent: `nodeAffinty` (missing an `i`)
// passed admission, decoded into an empty Affinity, and the pods were scheduled anywhere at all.
// Affinity is a scheduling *contract* for these products — rack awareness, spreading a quorum
// across failure domains, keeping a worker near its data — so losing it is not a cosmetic problem,
// and it produced no event, no log line and no status change.
//
// A strict decode turns that into a build failure naming the field. The trade-off is deliberate: a
// field that exists in a newer Kubernetes API than the one this SDK is built against is now
// rejected rather than ignored. That is the honest answer — the framework cannot honor a field it
// does not know — and it fails where someone can read it instead of at 3am when a node drains.
func DecodeAffinity(raw *k8sruntime.RawExtension) (*corev1.Affinity, error) {
	if raw == nil || len(raw.Raw) == 0 {
		return nil, nil
	}
	// json.Decoder reads ONE value and leaves the rest of the stream untouched, so
	// `{"nodeAffinity":{...}} {"podAffinity":{...}}` decodes the first object and drops the second
	// without complaint — the same silent partial loss this function exists to prevent, arriving
	// through the decoder rather than through an unknown field. json.Valid is the cheap way to
	// require exactly one value first. It also keeps this function consistent with affinityMembers,
	// which uses json.Unmarshal and rejects trailing data already.
	//
	// A value that reached here through the API server is always a single re-serialized object, so
	// this guards a RawExtension built in Go: a product's webhook defaulter, or product code
	// assembling a config by hand.
	if !json.Valid(raw.Raw) {
		return nil, fmt.Errorf("affinity is not a single valid JSON value")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw.Raw))
	decoder.DisallowUnknownFields()

	affinity := &corev1.Affinity{}
	if err := decoder.Decode(affinity); err != nil {
		return nil, err
	}
	return affinity, nil
}

// mergeAffinity folds a role's affinity into a role group's, one top-level member at a time: the
// group wins for a member it declares, and inherits the role's for a member it does not.
//
// The previous rule was all-or-nothing — any affinity at the group level discarded the role's
// entirely — which is the exact behavior MergeRoleGroupConfig's own doc comment identifies as a bug
// for `resources`: "Overriding one knob is the normal way to use this API, and it silently dropped
// the rest." Affinity had the same shape and is where it hurts most: a role that spreads its pods
// with podAntiAffinity, plus a group that adds a nodeAffinity to pin itself to an instance type,
// silently lost the spreading and put the whole group on one node.
//
// Merging stops at the top level on purpose. Below it sit `requiredDuringSchedulingIgnoredDuringExecution`
// and its preferred counterpart, whose values are complete scheduling statements — a nodeSelectorTerm
// list is an OR of ANDs, and interleaving two of them produces a constraint neither author wrote.
// Choosing per member is the finest granularity where the result still means what both sides said.
//
// Neither input is mutated and the result shares no memory with them.
func mergeAffinity(role, group *k8sruntime.RawExtension) *k8sruntime.RawExtension {
	roleFields, roleOK := affinityMembers(role)
	groupFields, groupOK := affinityMembers(group)

	// Anything that is not a JSON object cannot be merged member-wise. Rather than guess, keep the
	// old precedence (group wins whole) and let DecodeAffinity fail the build with the real reason —
	// the same malformed value reaches it a moment later.
	if !roleOK || !groupOK {
		if group != nil {
			return group.DeepCopy()
		}
		return role.DeepCopy()
	}

	merged := make(map[string]json.RawMessage, len(affinityFields))
	for _, field := range affinityFields {
		if value, ok := groupFields[field]; ok {
			merged[field] = value
			continue
		}
		if value, ok := roleFields[field]; ok {
			merged[field] = value
		}
	}
	// Members outside the known set are preserved so the strict decode still gets to reject them
	// with a message naming the field, instead of this merge quietly deleting the evidence.
	for _, fields := range []map[string]json.RawMessage{roleFields, groupFields} {
		for field, value := range fields {
			if _, known := merged[field]; known {
				continue
			}
			if !isAffinityField(field) {
				merged[field] = value
			}
		}
	}

	if len(merged) == 0 {
		return nil
	}
	encoded, err := json.Marshal(merged)
	if err != nil {
		// A map of already-valid json.RawMessage values cannot fail to marshal; keep the group's
		// value rather than dropping the user's configuration on an impossible path.
		if group != nil {
			return group.DeepCopy()
		}
		return role.DeepCopy()
	}
	return &k8sruntime.RawExtension{Raw: encoded}
}

// affinityMembers splits a RawExtension into its top-level JSON members. The bool reports whether
// the value was a JSON object at all; an absent value is an empty object, which merges cleanly.
func affinityMembers(raw *k8sruntime.RawExtension) (map[string]json.RawMessage, bool) {
	if raw == nil || len(raw.Raw) == 0 {
		return map[string]json.RawMessage{}, true
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw.Raw, &members); err != nil {
		return nil, false
	}
	if members == nil {
		// Explicit JSON null.
		return map[string]json.RawMessage{}, true
	}
	return members, true
}

// isAffinityField reports whether name is a known top-level member of corev1.Affinity.
func isAffinityField(name string) bool {
	return slices.Contains(affinityFields, name)
}

// affinityFieldsAreComplete fails the build if corev1.Affinity grows a member affinityFields does
// not list, which would silently stop that member from being inheritable from the role level.
func affinityFieldsAreComplete() error {
	encoded, err := json.Marshal(corev1.Affinity{
		NodeAffinity:    &corev1.NodeAffinity{},
		PodAffinity:     &corev1.PodAffinity{},
		PodAntiAffinity: &corev1.PodAntiAffinity{},
	})
	if err != nil {
		return err
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &members); err != nil {
		return err
	}
	for name := range members {
		if !isAffinityField(name) {
			return fmt.Errorf("corev1.Affinity has member %q that affinityFields does not list, "+
				"so a role-level value for it would not be inherited by its role groups", name)
		}
	}
	if len(members) != len(affinityFields) {
		return fmt.Errorf("affinityFields lists %d members but corev1.Affinity marshals %d",
			len(affinityFields), len(members))
	}
	return nil
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
	// 5. Build PDB if needed
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
