# operator-go/pkg/builder - Resource Builders

**Parent:** [../AGENTS.md](../AGENTS.md)

Kubernetes resource builders for StatefulSet, Service, ConfigMap, PDB, RBAC, and other resources.

## Key Files

Every non-test file in this package:

| File | Purpose |
|------|---------|
| `statefulset_builder.go` | `StatefulSetBuilder` — the workload builder, including `WithPodOverrides` and the strategic-merge-patch application of pod overrides |
| `service_builder.go` | `ServiceBuilder` / `HeadlessServiceBuilder` and the `ServiceType` constants |
| `configmap_builder.go` | `ConfigMapBuilder`, including `WithMergedConfig(cfg *config.MergedConfig, generator *config.MultiFormatConfigGenerator) (*ConfigMapBuilder, error)` |
| `pdb_builder.go` | PodDisruptionBudget builder |
| `rbac_builder.go` | `RoleBuilder`, `RoleBindingBuilder`, `ClusterRoleBuilder`, `ClusterRoleBindingBuilder` |
| `serviceaccount_builder.go` | `ServiceAccountBuilder` |
| `metrics_service_builder.go` | Metrics headless Service builder (Prometheus scrape annotations; targetPort defaults to the numeric port, `WithTargetPortName` opts into a named targetPort) |
| `copy.go` | `cloneSlice` — the internal deep-copy helper the builders use so `Build()` never hands out builder-owned slices |

Nothing in this package writes to the API server. The RBAC builders in particular are helpers only:
`GenericReconciler` creates no Role/RoleBinding, so a product that needs workload RBAC builds the
objects here and ships them through `reconciler.RoleGroupResources.ExtraResources` (or its own
extension).

## Builder Semantics

- **`Build()` returns an independent object.** Every builder deep-copies its maps and slices into
  the result, so building twice, or mutating a built object, cannot corrupt builder state or
  another built object.
- **`WithLabels` / `WithAnnotations` merge, they do not replace.** This holds for every builder
  that has them (StatefulSet, Service, ConfigMap, PDB, RBAC, ServiceAccount); calling them twice
  unions the entries.
- **`NamespacedName()`** is available on the StatefulSet, Service, ConfigMap, PDB, `RoleBuilder`,
  `RoleBindingBuilder`, `ClusterRoleBuilder`, `ClusterRoleBindingBuilder` and ServiceAccount
  builders, for callers that need the key without building the object. The two cluster-scoped RBAC
  builders return an empty `Namespace`. `MetricsServiceBuilder` does not expose one.
- **`ServiceBuilder` port APIs.** `AddPort` / `AddPortSimple` construct a port from scalars;
  `AddServicePort(corev1.ServicePort)` and `WithPorts([]corev1.ServicePort)` append fully specified
  ports and preserve every field the caller set (`nodePort`, `appProtocol`, a named `targetPort`).
  Use the latter pair whenever you already hold `corev1.ServicePort` values.
- **`ServiceBuilder.WithPublishNotReadyAddresses(bool)`** sets `.spec.publishNotReadyAddresses`,
  which quorum systems need so members can resolve each other's DNS before they are ready.
- **`.spec.type` is always explicit.** `NewServiceBuilder` defaults to `ClusterIP` and `Build()`
  writes it, rather than leaving the field empty for the API server to default.
- **`ConfigMapBuilder.WithMergedConfig` returns `(*ConfigMapBuilder, error)`** — it is not a pure
  fluent step, because config generation can fail. A ConfigMap with no config files at all is built
  with `.Data` left nil rather than an empty map.
- **`StatefulSetBuilder` pod overrides** are applied last in `Build()` via a strategic merge patch
  on the pod template: containers merge by name, struct fields (including security contexts)
  deep-merge per field, and the selector labels are re-asserted afterwards so an override can never
  break the immutable `.spec.selector` ↔ template-labels invariant. Anything that must be
  addressable by an override (notably the primary container name) has to be set on the builder
  **before** `Build()`.

## Working Instructions

1. **Creating a new builder:** Follow the pattern of existing builders — a `New*Builder(name,
   namespace)` constructor, chainable `With*`/`Add*` methods, a `Build()` that deep-copies, and a
   `NamespacedName()`.
2. **Testing builders:** Add corresponding `*_test.go` files with unit tests (Ginkgo v2 + Gomega;
   the package's `suite_test.go` wires the runner).
3. **Builder pattern:** each builder supports method chaining; a step that can fail returns
   `(*Builder, error)` instead of stashing the error.
