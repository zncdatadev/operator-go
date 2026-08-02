# operator-go/pkg/builder - Resource Builders

**Parent:** [../AGENTS.md](../AGENTS.md)

Kubernetes resource builders for StatefulSet, Service, ConfigMap, PDB, RBAC, and other resources.

## Key Files

Every non-test file in this package:

| File | Purpose |
|------|---------|
| `statefulset_builder.go` | `StatefulSetBuilder` — the workload builder, including `WithPodOverrides`, the strategic-merge-patch application of pod overrides, and `PodOverrideViolations()` (the framework mounts an override displaced; `Build()` cannot return an error, so they are read back afterwards) |
| `service_builder.go` | `ServiceBuilder` / `HeadlessServiceBuilder` and the `ServiceType` constants |
| `configmap_builder.go` | `ConfigMapBuilder`, including `WithMergedConfig(cfg *config.MergedConfig, generator *config.MultiFormatConfigGenerator) (*ConfigMapBuilder, error)` |
| `pdb_builder.go` | PodDisruptionBudget builder |
| `rbac_builder.go` | `RoleBuilder`, `RoleBindingBuilder`, `ClusterRoleBuilder`, `ClusterRoleBindingBuilder` |
| `serviceaccount_builder.go` | `ServiceAccountBuilder` |
| `metrics_service_builder.go` | Metrics headless Service builder (Prometheus scrape annotations; targetPort defaults to the numeric port, `WithTargetPortName` opts into a named targetPort) |
| `pod_override_merge.go` | The pod template strategic merge patch and `clearSupersededUnions`, which resolves the collisions the patch format cannot express |
| `copy.go` | `cloneSlice` / `clonePtr` — the internal deep-copy helpers the builders use so `Build()` never hands out builder-owned slices or pointers |

Nothing in this package writes to the API server. The RBAC builders in particular are helpers only:
`GenericReconciler` creates no Role/RoleBinding, so a product that needs workload RBAC builds the
objects here and ships them through `reconciler.RoleGroupResources.ExtraResources` (or its own
extension).

## Builder Semantics

- **`Build()` returns an independent object.** Every builder deep-copies its maps, slices and
  scalar pointers into the result (including the `[]byte` values of `ConfigMap.BinaryData`), so
  building twice, or mutating a built object, cannot corrupt builder state or another built
  object. `ServiceBuilder.BuildHeadless()` likewise leaves the builder's service type alone.
- **A builder never aliases a caller's map.** `WithLabels` / `WithAnnotations` / `WithSelector`
  copy what they are given, as does `NewMetricsServiceBuilder`.
- **`WithLabels` / `WithAnnotations` merge, they do not replace.** This holds for every builder
  that has them (StatefulSet, Service, ConfigMap, PDB, RBAC, ServiceAccount); calling them twice
  unions the entries.
- **`MetricsServiceBuilder` name and annotations are customizable.** The Service is named
  `{resourceName}-metrics` unless `WithName` overrides it — the migration path for products whose
  pre-framework operator published the metrics Service under the role group resource name.
  `WithAnnotations` merges extra entries into the generated Prometheus annotation set; caller
  entries win on key collisions, so any default can be restated or replaced. The reconciler's
  metrics slot identifies this Service by the labels it stamps at apply time (the slot label plus
  the role group marker), so apply and reclaim behave identically under a custom name.
- **`PDBBuilder.Build()` panics without a selector.** An empty `LabelSelector` is accepted by the
  API server and selects *every* pod in the namespace, so a PDB built without `WithSelector` would
  silently block node drains for workloads the operator does not own. It is the only object in
  this package whose invalidity no API server error would reveal, so the builder refuses to build
  it.
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
- **Mutually exclusive fields are replaced, not merged.** Kubernetes' "one of" structs — a
  `Volume`'s source, a probe handler, a lifecycle handler, `EnvVar.value` vs `valueFrom` — cannot
  deep-merge: an object with two members set is rejected outright by the API server. When an
  override states a different member than the framework did, `clearSupersededUnions` drops the
  framework's member first, so the override's volume (probe, handler, env source) replaces it
  wholesale at its original position. This is what `PodSpec.Volumes`' declared
  `patchStrategy:"merge,retainKeys"` prescribes; a patch derived from a typed `PodTemplateSpec`
  carries no `$retainKeys` directive to say so. An override naming the *same* member still merges
  field by field (e.g. adding a `sizeLimit` to an `emptyDir`), and this is what makes overriding a
  framework-owned volume such as `config` a supported operation rather than an opaque apply
  failure.

- **`StatefulSetBuilder` generates a readiness probe and never a liveness probe.** When ports are
  declared, `Build()` attaches a TCP readiness probe to `Ports[0]`; `LivenessProbe` is set only by
  `WithLivenessProbe`. The asymmetry is the point. The framework does not know which of a product's
  ports means "healthy" — the first declared one is an accident of declaration order and is as
  likely to be a metrics port — nor how long the product takes to open it, and a liveness probe on a
  wrong guess *kills the container on a timer forever* (the removed default gave ~90-120s before the
  first kill, which is inside the startup time of a NameNode loading an fsimage). Readiness's worst
  case is a pod held out of its Services: visible as `0/1` and self-correcting. Removing readiness
  too would not be neutral — with no readiness probe a pod is Ready the instant it starts, so a
  rolling update walks a whole role group without waiting for any member to come up.

  Consequences for callers: **the first entry of `SetRoleContainerPorts`/`WithPorts` is part of the
  contract** (put the port that means "this pod can serve" first), and
  `builder.DefaultTCPLivenessProbe(port)` reproduces the removed probe on a port the product picks.
  This is deliberately *not* the same call as the sidecar probes in `pkg/sidecar`, which the
  framework does author — there it owns the image, the port and the endpoint.
- **`podManagementPolicy` defaults to `Parallel`, and that is a choice.** `OrderedReady` starts pod
  N+1 only once pod N is Ready, which deadlocks a quorum product at pod-0: a ZooKeeper member or an
  HDFS JournalNode is not Ready until it sees a quorum that does not exist until its peers start.
  (`WithPublishNotReadyAddresses` on the headless Service is the other half of the same problem.)
  `WithPodManagementPolicy` overrides it, but only at creation — the field is immutable, and the
  apply path preserves the live value with an `ImmutableFieldIgnored` warning.
- **`WithUpdateStrategy` is the canary knob.** `.spec.updateStrategy` is mutable and is *not*
  preserved by the apply path, so a partitioned rollout (raise the partition, upgrade the high
  ordinals, verify, lower it) converges through the normal reconcile. Left unset the field stays
  empty and Kubernetes applies RollingUpdate with partition 0. Neither this nor
  `podManagementPolicy` is reachable through `podOverrides`, which is a `PodTemplateSpec`.

## Working Instructions

1. **Creating a new builder:** Follow the pattern of existing builders — a `New*Builder(name,
   namespace)` constructor, chainable `With*`/`Add*` methods, a `Build()` that deep-copies, and a
   `NamespacedName()`.
2. **Testing builders:** Add corresponding `*_test.go` files with unit tests (Ginkgo v2 + Gomega;
   the package's `suite_test.go` wires the runner).
3. **Builder pattern:** each builder supports method chaining; a step that can fail returns
   `(*Builder, error)` instead of stashing the error.
