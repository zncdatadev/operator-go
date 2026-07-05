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

	// ServiceAccountName is the name of the ServiceAccount the workload pods should run as.
	// It is populated by GenericReconciler from its configured ServiceAccountName (the SA the
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

// LoggingProducerProvider is implemented by handlers (e.g. BaseRoleGroupHandler) that declare
// log-producer containers. GenericReconciler type-asserts its handler against this interface to
// configure the Vector sidecar (the single owner of the shared log volume) without depending on a
// concrete handler type.
type LoggingProducerProvider interface {
	// LoggingProducers returns the declared log-producer containers (the containers whose log
	// files Vector collects; the provider RW-mounts the shared log volume on each).
	LoggingProducers() []productlogging.ContainerLogging
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
	// Emit the rolling file appender only when the Vector agent is enabled: file logging is
	// coupled to Vector (without a consumer there is no shared log volume to write to). Gating
	// here means products building their own ConfigMap inherit the behavior for free.
	return productlogging.RenderConfigFile(
		buildCtx.ContainerLogging(decl.Container), decl, vectorEnabledFor(buildCtx))
}

// RenderLoggingConfigMapData renders the logging-related entries for a role group ConfigMap:
//   - one logging config file per declared producer (level config, plus the rolling file appender
//     when Vector is enabled), keyed by the generator file name (e.g. "logback.xml"), and
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
	if vectorEnabledFor(buildCtx) && buildCtx.VectorAggregatorAddress != "" {
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
