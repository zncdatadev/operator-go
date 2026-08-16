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

package controller

import (
	"context"
	"fmt"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	trinoconfig "github.com/zncdatadev/operator-go/examples/trino-operator/internal/config"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/constants"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/product"
	"github.com/zncdatadev/operator-go/pkg/config"
	"github.com/zncdatadev/operator-go/pkg/productlogging"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Compile-time proof that TrinoCluster wires framework-owned vector.yaml generation: when a role
// group enables the Vector agent, the GenericReconciler reads this ConfigMap name, resolves the
// aggregator address, and generates vector.yaml into the role group ConfigMap.
var _ reconciler.VectorAggregatorProvider = (*trinov1alpha1.TrinoCluster)(nil)

// RBAC for the resources the SDK GenericReconciler consumes on behalf of a TrinoCluster. This is
// the OPERATOR's own ClusterRole, not the workload's (that is GenericReconcilerConfig's
// WorkloadRBACRules, which this example does not use) — the canonical set, with the reason for each
// grant, is docs/security.md §3.3. Keep this block in step with it: the SDK cannot declare these
// itself, because controller-gen never walks a dependency's packages, so this file is what every
// adopter copies. Regenerate config/rbac/role.yaml with `make manifests` after editing.
//
// Two verbs are deliberately absent, and each absence removes a capability this operator does not
// need — which is the test docs/security.md §3.3.1 applies, rather than "the framework never calls
// it". `patch` alongside `update` grants nothing extra (a PATCH is reachable through a
// read-modify-write PUT), and the SDK exports helpers that need it, so it stays:
//   - no `delete` on serviceaccounts — nothing deletes one; it is reclaimed by owner-reference GC
//   - no `update`/`patch` on the CR body — the framework writes only Status().Update, and an
//     operator that can rewrite its users' spec is a different trust proposition. Add it back if
//     this operator ever registers a finalizer.
//
// Three things do not announce themselves when missing (§3.3.3): `events` is discarded by client-go
// with no error; `pods` stops the Degraded condition being computed; and the cleanup path swallows
// its errors, so a 403 on a teardown delete — the persistentvolumeclaims grant, say — leaves the
// pass reporting success. Everything else fails loudly on the apply path — a forbidden informer
// takes manager.Start down with it.
//
// +kubebuilder:rbac:groups=trino.kubedoop.dev,resources=trinoclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=trino.kubedoop.dev,resources=trinoclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=trino.kubedoop.dev,resources=trinoclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services;configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// TrinoRoleGroupHandler builds Trino role group resources. It embeds the SDK's
// BaseRoleGroupHandler so the framework owns the bulk of resource orchestration — ConfigMap
// (rendered from the merged config, including the product config computed by
// product.ComputeConfig), Services, the StatefulSet (with sidecars and podOverrides applied
// by the framework), and the PDB. The override below only adds the product-specific bits the
// merge pipeline cannot model declaratively.
type TrinoRoleGroupHandler struct {
	*reconciler.BaseRoleGroupHandler[*trinov1alpha1.TrinoCluster]
}

// NewTrinoRoleGroupHandler creates the handler. It now carries only reconcile-invariant
// collaborators: everything a ROLE is made of is declared per reconcile by DeclareRoles below,
// with the cr in hand.
func NewTrinoRoleGroupHandler(scheme *runtime.Scheme) *TrinoRoleGroupHandler {
	base := reconciler.NewBaseRoleGroupHandler[*trinov1alpha1.TrinoCluster](scheme)

	// config.properties (provided as a map by the role group resolver / CRD overrides) is
	// rendered with the properties format adapter.
	base.ConfigGenerator = config.NewMultiFormatConfigGenerator()
	base.ConfigGenerator.RegisterDefaultFormats()

	// Trino reads config from /etc/trino.
	base.ConfigMountPath = "/etc/trino"

	return &TrinoRoleGroupHandler{BaseRoleGroupHandler: base}
}

// DeclareRoles implements reconciler.RoleProvider: one statement per role, produced once per
// reconcile pass with the cr in hand.
//
// It replaces seven role-keyed maps on the handler plus their per-call twins on the build context.
// Taking the cr is what lets it be static data — a port that moves because the CR enabled TLS is
// computed here, from THIS cr, rather than assigned into process-wide handler state that the next
// cluster would inherit.
func (h *TrinoRoleGroupHandler) DeclareRoles(
	_ context.Context, _ client.Client, _ *trinov1alpha1.TrinoCluster,
) (reconciler.RoleCatalog, error) {
	// Ports are the same for both roles. Ports[0] backs the generated TCP readiness probe.
	shared := reconciler.RoleDeclaration{
		MainContainerName: constants.MainContainerName,
		ContainerPorts: []corev1.ContainerPort{
			{Name: "http", ContainerPort: constants.DefaultHTTPPort, Protocol: corev1.ProtocolTCP},
		},
		ServicePorts: []corev1.ServicePort{
			{Name: "http", Port: constants.DefaultHTTPPort, Protocol: corev1.ProtocolTCP},
		},
		// Declarative logging: the framework renders the Log4j2 config file into the ConfigMap
		// from the folded CRD logging spec. The container named here must be the pod's primary
		// container, which MainContainerName pins.
		LogProducers: []productlogging.ContainerLogging{
			{Container: constants.MainContainerName, Framework: productlogging.LoggingFrameworkLog4j2},
		},
	}
	return reconciler.RoleCatalog{
		product.RoleCoordinators: shared,
		product.RoleWorkers:      shared,
	}, nil
}

// BuildResources delegates the 90% to the framework, then appends the product-specific pieces
// the merge pipeline cannot express:
//   - the CR-driven container image (resolved with the product name),
//   - jvm.config (a newline-delimited flag list, not key=value),
//   - the coordinator-only catalog files.
func (h *TrinoRoleGroupHandler) BuildResources(
	ctx context.Context,
	k8sClient client.Client,
	cr *trinov1alpha1.TrinoCluster,
	buildCtx *reconciler.RoleGroupBuildContext,
) (*reconciler.RoleGroupResources, error) {
	resources, err := h.BaseRoleGroupHandler.BuildResources(ctx, k8sClient, cr, buildCtx)
	if err != nil {
		return nil, err
	}

	if resources.ConfigMap != nil {
		if resources.ConfigMap.Data == nil {
			resources.ConfigMap.Data = make(map[string]string)
		}

		// jvm.config is a flag list, not key=value, so it is generated here as a whole file
		// (like the logging file) rather than flowing through the merge pipeline. setIfAbsent
		// only avoids clobbering a jvm.config the pipeline already placed under this key; note
		// that configOverrides renders as key=value, so it is NOT a suitable channel for tuning
		// JVM flags — a product needing user-tunable JVM options would expose a typed field/env.
		setIfAbsent(resources.ConfigMap.Data, "jvm.config", func() string { return jvmConfig(buildCtx.RoleName) })

		// Catalog connector files live only on the coordinator.
		if buildCtx.RoleName == product.RoleCoordinators {
			catalogs := trinoconfig.NewCatalogConfigBuilder().WithCatalogs(cr.Spec.Catalogs).Build()
			for name, content := range catalogs {
				key := fmt.Sprintf("catalog/%s.properties", name)
				setIfAbsent(resources.ConfigMap.Data, key, func() string { return content })
			}
		}
	}

	return resources, nil
}

// setIfAbsent writes value() into data[key] only when the key is not already present, so
// product config never overwrites config the merge pipeline produced (CRD always wins).
func setIfAbsent(data map[string]string, key string, value func() string) {
	if _, exists := data[key]; !exists {
		data[key] = value()
	}
}

// jvmConfig renders the role-specific JVM options.
func jvmConfig(roleName string) string {
	b := trinoconfig.NewJVMConfigBuilder()
	if roleName == product.RoleWorkers {
		b.ForWorker()
	} else {
		b.ForCoordinator()
	}
	return b.Build()
}

// Ensure interface implementation.
var _ reconciler.RoleGroupHandler[*trinov1alpha1.TrinoCluster] = &TrinoRoleGroupHandler{}
