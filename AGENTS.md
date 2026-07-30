# AGENTS.md

## Project Overview
`operator-go` is a Golang SDK/framework for building Kubernetes operators. It provides a reusable reconciliation framework, CRDs, and utilities for creating product-specific operators.

**Key Features:**
- **GenericReconciler**: Template Method Pattern-based reconciliation framework
- **Extension System**: Hook-based customization at cluster/role/role-group levels, with per-product registries
- **Resource Builders**: Fluent builders for StatefulSet, Service, ConfigMap, PDB, RBAC, ServiceAccount
- **Config Generation**: Multi-format config file generation (XML, YAML, Properties, Env, INI)
- **Logging Config**: Framework-aware logging configuration generation (Log4j, Log4j2, Logback, Python)
- **Health Checks**: Business-level health check interface with composite checks
- **Sidecar Management**: Phase-ordered, validated sidecar injection framework with domain-specific providers
- **Secret/Listener CSI Wiring**: `SecretProvisioner` and `ListenerProvisioner` declare secret-operator / listener-operator CSI volumes and resolve their mount paths
- **CRD APIs**: Common types for authentication, database, listeners, S3

## Architecture Documentation (Authoritative Design Source)

> **IMPORTANT**: The `docs/` directory contains architecture documents that are the **authoritative source of design constraints** for this project. All implementations — including the SDK itself and any operators built with it — **must follow** the design defined in these documents. When code and documentation conflict, the documentation takes precedence. Consult these docs before making design decisions.

> **Scope of that rule.** `docs/architecture.md` is authoritative about **design intent**: a
> conflict means the code should change, not that the doc should be quietly relaxed. The
> `AGENTS.md` files (this one and the per-package ones) are the opposite: they describe the API and
> behavior that **exist today**, and must be corrected whenever the code changes. Never treat a
> statement in an `AGENTS.md` as a requirement the code has yet to meet — anything aspirational
> belongs in `docs/architecture.md` and must be explicitly labelled as such.

### Documentation Structure

| File | Description |
|------|-------------|
| `docs/architecture.md` | **Core Technical Architecture** — design philosophy, layered architecture, core module specifications, design patterns, key problem solutions. This is the primary reference for all SDK design decisions. |
| `docs/architecture_zh.md` | Chinese version of the architecture document |
| `docs/security.md` | **Security Architecture** — application security (SecretClass, CSI, AutoTLS, Kerberos) and infrastructure security (RBAC, ServiceAccounts, Pod security) |
| `docs/DOC_CHANGELOG.md` | Changelog tracking all documentation updates |
| `docs/examples/` | CRD example YAMLs demonstrating the SDK's data model |

### CRD Examples (`docs/examples/`)

| File | Description |
|------|-------------|
| `crd-base-example.yaml` | Base CRD template showing the generic structure all product CRDs follow |
| `crd-hdfs-example.yaml` | HDFS cluster CRD example (HA with NameNode, JournalNode, DataNode) |
| `crd-hive-example.yaml` | Hive Metastore CRD example (S3 integration, TLS, Kerberos) |

### Key Architectural Principles (from `docs/architecture.md`)

1. **Interface-Driven Design (IDD)**: SDK core relies on interfaces, not concrete implementations. New products implement interfaces without modifying SDK core.
2. **Desired State Convergence**: CR Spec is the desired state; reconciliation loop converges actual state. Bidirectional: also cleans orphaned resources.
3. **Separation of Common and Specific**: SDK handles common logic (resource construction, config merging, webhook validation); products handle specific logic via extension interfaces.
4. **Type Safety and Idempotency**: Go Generics for compile-time safety. All operations are idempotent.
5. **Strict Merge Strategy**: Role/RoleGroup config merging follows defined rules — Deep Merge for maps, `SliceMergeStrategy` (Replace by default) for slices, Strategic Merge Patch for PodTemplate.
6. **Layered Architecture**: Specific Product Layer → Abstract Interface Layer → Core Component Layer → Tools Layer → API Layer.

## Development Environment
- **Language**: Go 1.25.3
- **Dependency Management**: Go Modules (`go.mod`)
- **Testing**: Ginkgo v2 + Gomega
- **Tooling**: Uses `Makefile` to manage local binaries in `bin/`

### Tool Versions
- `controller-gen`: v0.19.0
- `golangci-lint`: v2.12.2
- `kustomize`: v5.7.1
- `controller-runtime`: v0.23.3
- `k8s.io/api`: v0.35.4

## Common Commands
Run these from the project root:

| Command | Description |
|---------|-------------|
| `make generate` | Generate DeepCopy methods via `controller-gen` |
| `make manifests` | Generate the test CRDs (`config/crd/bases/`) from `pkg/testutil` types |
| `make verify-generate` | Regenerate and fail if the committed generated files differ (both modules) |
| `make fmt` | Run `go fmt` against code |
| `make vet` | Run `go vet` against code |
| `make test` | Run unit tests with coverage (uses envtest for K8s integration); `GOTESTFLAGS=-race` for the race detector |
| `make lint` | Run `golangci-lint` |
| `make lint-fix` | Run `golangci-lint` with auto-fix |
| `make lint-config` | Verify golangci-lint configuration |

## Directory Structure
> Subdirectories with their own `AGENTS.md` provide detailed file-level documentation. This section shows the top-level layout only.

```
operator-go/
├── pkg/                          # Core SDK packages (see pkg/AGENTS.md)
│   ├── apis/                     # Kubernetes API definitions — CRDs (see pkg/apis/AGENTS.md)
│   ├── builder/                  # Fluent resource builders (see pkg/builder/AGENTS.md)
│   ├── common/                   # Core interfaces, extensions, errors
│   ├── config/                   # Config file generation and override merging (see pkg/config/AGENTS.md)
│   ├── constant/                 # Kubedoop paths, labels, domains, restarter annotations
│   ├── listener/                 # Listener provisioner (CSI volume registration)
│   ├── productlogging/           # Product logging config generation (Log4j, Log4j2, Logback, Python)
│   ├── reconciler/               # Reconciliation framework (see pkg/reconciler/AGENTS.md)
│   ├── s3/                       # S3Connection/S3Bucket resolution, S3A properties, credential wiring
│   ├── security/                 # Pod security defaults, SecretProvisioner (secret-operator CSI)
│   ├── sidecar/                  # Sidecar injection framework (SidecarManager, SidecarProvider interface)
│   ├── vector/                   # Vector sidecar implementation (config generation, discovery, provider)
│   ├── testutil/                 # Testing utilities (envtest, mocks, matchers)
│   ├── util/                     # K8s utilities, exec utilities
│   └── webhook/                  # Webhook infrastructure (defaulter, validator)
├── docs/                         # Architecture and design documentation (authoritative design source)
│   ├── architecture.md           # Core Technical Architecture (English)
│   ├── architecture_zh.md        # Core Technical Architecture (Chinese)
│   ├── security.md               # Security Architecture (SecretClass, CSI, RBAC, Pod security)
│   ├── DOC_CHANGELOG.md          # Documentation changelog
│   └── examples/                 # CRD example YAMLs (base, HDFS, Hive)
├── examples/                     # Example operators (see examples/AGENTS.md)
│   └── trino-operator/           # Trino operator example (see examples/trino-operator/AGENTS.md)
├── hack/                         # Scripts and boilerplate
└── bin/                          # Local binaries (controller-gen, etc.)
```

## Key Concepts

### 1. ClusterInterface and ClusterResource
All product CRs must implement `ClusterInterface` (defined in `pkg/common/cluster_interface.go`):
```go
type ClusterInterface interface {
    client.Object // sigs.k8s.io/controller-runtime/pkg/client

    GetSpec() *v1alpha1.GenericClusterSpec
    GetStatus() *v1alpha1.GenericClusterStatus
}
```

The embedded `client.Object` supplies name, namespace, UID, labels, annotations, generation and
GVK, and makes the CR usable directly wherever controller-runtime expects an object (`client.Get`,
`Status().Update`, `SetControllerReference`, event recording). A CR that embeds `metav1.TypeMeta`
and `metav1.ObjectMeta` and is registered with the manager's scheme already satisfies that half, so
**`GetSpec` and `GetStatus` are the only two methods a product writes**. There is no `SetStatus`:
`GetStatus` returns a pointer into the CR and the framework mutates the generic status through it,
which is why a product's own status fields survive a reconcile cycle.

The reconciler is parameterised by a companion constraint, not by `ClusterInterface` itself:
```go
type ClusterResource[T ClusterInterface] interface {
    ClusterInterface
    DeepCopy() T
}
```
`DeepCopy() T` is what `make generate` (controller-gen) already emits for every root API type; the
reconciler needs it because `new(T)` is unavailable for a pointer type parameter, so it materialises
the object it reads into by copying a prototype. `GenericReconciler`, `GenericReconcilerConfig` and
`NewGenericReconciler` are declared `[CR common.ClusterResource[CR]]`. Everything else that is
generic over a CR — `RoleGroupHandler[CR]`, `ClusterExtension[CR]`, `ExtensionRegistry[CR]` — is
constrained by plain `common.ClusterInterface`.

Hold a CR as `ClusterInterface`; parameterise over one as `ClusterResource[CR]`.

### 2. GenericReconciler (Template Method Pattern)
`GenericReconciler[CR common.ClusterResource[CR]]` provides a fixed reconciliation flow with
customizable extension points. It is built from a `GenericReconcilerConfig[CR]` through
`NewGenericReconciler[CR]`:

**Reconciliation Flow:**
1. Fetch CR (NotFound ⇒ done; non-zero `deletionTimestamp` ⇒ done — see "Deletion" below)
2. Panic recovery: a recovered panic becomes a returned error plus a `ReconcilePanic` Warning
   event; the status is left untouched
3. ClusterOperation gate: `reconciliationPaused` returns immediately; `stopped` falls through so
   every resource is still reconciled with replicas forced to 0
4. Ensure the ServiceAccount (when configured); warn about handler-configured role names the CR
   does not declare
5. PreReconcile Extensions (Hook)
6. Validate declared dependencies (`GenericReconcilerConfig.Dependencies`)
7. For Each Role (**best effort** — see below):
   - Role PreReconcile Extensions
   - For Each RoleGroup:
     - RoleGroup PreReconcile Extensions
     - Build RoleGroupBuildContext
     - Delegate to RoleGroupHandler.BuildResources()
     - Apply Resources (CM -> HeadlessSvc -> Service -> Extras -> [sidecar Validate] -> STS ->
       per-group PDB -> MetricsSvc)
     - RoleGroup PostReconcile Extensions
   - Role-level PodDisruptionBudget
   - Role PostReconcile Extensions
8. Cleanup Orphaned Resources
9. Update Health Status
10. PostReconcile Extensions
11. Final Status Update, then requeue

Each "Apply" is create-OR-UPDATE (issue #526): when the resource already exists, the live object is updated to the handler-built desired state every reconcile — labels are replaced wholesale, annotations are merged (foreign annotations survive), and spec/data is copied per kind while preserving Kubernetes immutable/allocated fields (StatefulSet `selector`/`serviceName`/`volumeClaimTemplates`/`podManagementPolicy`; Service `clusterIP(s)`/`ipFamilies` and allocated NodePorts). Arbitrary-GVK extras get a generic top-level field copy. See `copyDesiredState` in `pkg/reconciler/apply.go`. Changing an immutable field for an existing cluster requires a manual delete/recreate migration — and the framework now **says so**: when a handler's desired value for a preserved field differs from the live one, `applyResource` emits an `ImmutableFieldIgnored` Warning event on the CR naming the resource and the field paths. Preserving those fields silently is what let a storage resize be accepted, reported as `ReconcileComplete=True`, and never applied. Only a field the handler actually set is reported (an unset field is declining to have an opinion, not a change request), and among the Service's preserved fields only `clusterIP` is — the others are API-server allocations, so a difference there would be noise on every reconcile.

**A `configOverrides` change does not roll the pods by itself — the platform restarter does.**
Editing `configOverrides` makes the framework rewrite the role group ConfigMap, and stop there: the
pod template is byte-identical, so the StatefulSet controller has no reason to roll anything, and
none of these products re-read their configuration files at runtime.

Delivering that change to the running processes is **commons-operator's restarter**, not this SDK.
Label the workload `restarter.kubedoop.dev/enable=true` (`constant.LabelRestarterEnable` /
`LabelRestarterEnableValue`) and, whenever a ConfigMap or Secret the pod references — as a **volume**
or through an env var's `valueFrom` — changes, the restarter writes
`configmap.restarter.kubedoop.dev/<name>` (or `secret.restarter.kubedoop.dev/<name>`) into the
workload's pod template; the StatefulSet controller then rolls the pods. The annotation's value is
`<uid>/<resourceVersion>`, so **enabling the label always costs one rollout**: the first pass stamps
a template that had no annotation. **The SDK writes neither annotation** — those prefixes exist in
`pkg/constant/restarter.go` to document the restarter's half of the contract, not for the framework
to emit. The same component also restarts pods whose secret-operator TLS/Kerberos secrets have
passed the expiry recorded in `restarter.kubedoop.dev/expires-at.<...>`.

The label goes on **`StatefulSet.metadata.labels`**, which is where the restarter's watch predicate
and its `client.MatchingLabels` list both read it — a pod-template label does not enable anything,
so `podOverrides` is not a way in. The framework's own channel is the **cluster CR's labels**: the
reconciler passes a writable clone of `cr.GetLabels()` as `RoleGroupBuildContext.ClusterLabels`, and
`BaseRoleGroupHandler` merges them into every built resource's metadata (and pod template), so
`kubectl label <cluster-cr> restarter.kubedoop.dev/enable=true` reaches the StatefulSet metadata the
restarter watches. Opting in is a **deployment** decision by whoever runs the cluster, not something
the operator's author hardcodes.

> **Caveat (upstream bug).** commons-operator's `getRefConfigMapRefs` returns after the *first*
> ConfigMap volume it finds, so a pod mounting several ConfigMaps only ever gets one of them
> watched — see zncdatadev/commons-operator#298. The secret path (`getRefSecretRefs`) is correct.

The framework already satisfies the restarter's precondition: the role group ConfigMap is mounted as
the `config` volume. What it does **not** do is set the label, and without it a `configOverrides`
change simply does not roll.

This applies to `configOverrides` alone. `envOverrides` and `cliOverrides` reach the container as
env vars and args through `MergedConfig` (`StatefulSetBuilder.WithConfig`), and `podOverrides`
patches the template directly — all three change the pod template, so they roll natively with no
restarter involved.

**Role iteration is best-effort.** A failing role does not stop the others, and a failing role group does not stop its siblings, the role-level PDB, or the role's PostReconcile hook. Steps 8-10 (orphan cleanup, health, cluster PostReconcile) run regardless. Roles and role groups are independent workloads: aborting at the first failure meant one unparsable value on the alphabetically-first role indefinitely blocked the *deletion* of an unrelated role group, the health of every other role, and the discovery ConfigMap a product publishes from PostReconcile. The per-role errors are combined with `errors.Join` and returned once, so the cluster still goes `Degraded` and the workqueue still backs off. Iteration stays **sorted** so that aggregated message is byte-stable across cycles — an unstable message would defeat the no-op guard in `updateStatus` and make the controller reschedule itself forever. The single exception is a 429: a `*RateLimitError` aborts the pass immediately, because pushing the remaining roles through would only deepen the backlog.

**Requeue cadence.** A successful reconcile returns `ctrl.Result{RequeueAfter: HealthCheckInterval}` (`DefaultHealthCheckInterval` = 120s; a negative value disables the periodic wakeup), or the cleaner's earliest pending wakeup when that is sooner — a remaining gray-delete deadline, or the drain poll interval of an orphan deletion in flight. Watches only cover the kinds the framework owns, so anything that changes without producing an event — a product `ServiceHealthCheck` probe, a grace period running out, a StatefulSet finishing its drain — depends on this timer.

**Orphan cleanup discovers its work from the live cluster, not only from `status.roleGroups`.** The cleaner unions two inventories: the role group ConfigMaps and StatefulSets this CR controller-owns that carry the framework's labels (`instance` + `managed-by` + `component` + `role-group`) **and** whose name is exactly what `RoleGroupResourceName` produces for those labels, plus the `status.roleGroups` ledger. The ledger alone is a record the operator must have *successfully written*, so losing it — a process death between applying a role group's resources and updating the CR, a backup tool restoring the CR without its status subresource, a `kubectl replace` — used to make those resources invisible to the cleaner permanently, holding their PVCs and pods until a human noticed. The name check is what keeps the live half safe: a discovery ConfigMap carries the same instance/managed-by pair and owner reference, and a product's `ExtraResources` may carry the handler's entire label set. An empty owner UID disables live discovery, as it does the role-PDB reclaim.

**Orphan cleanup is a multi-pass state machine.** A role group removed from the spec is retired over several reconciles: scale the StatefulSet to zero (under `RetryOnConflict`), wait for the controller's ordered drain (`.status.replicas` reaching 0), then delete `PDB → StatefulSet → ConfigMap → Service → headless → metrics`, each step confirmed absent before the next is issued. A group's status entry is pruned only after a real deletion; a failure is isolated to its own group and the others still progress; the state machine's progress annotations (`orphan.zncdata.dev/pending-deletion`, `orphan.zncdata.dev/drain-started`) are reset on every role group that IS in the spec, so a group that was orphaned and then re-added starts its next teardown from scratch instead of inheriting the previous one's timestamps; a 429 becomes a `*reconciler.RateLimitError` that aborts the pass and backs off instead of marking the cluster `Degraded`. The cleaner also reclaims the role-level PDB of a role deleted from the spec outright, found by the `pdb.kubedoop.dev/role` label (`reconciler.LabelRolePodDisruptionBudget`) rather than by derived name.

**Status writes are conditional.** `updateStatus` skips the write entirely when the whole CR is `apiequality.Semantic.DeepEqual` to the object read at the start of the cycle — comparing the whole object, not just the embedded generic status, so a product's own status fields count too. Without that guard the controller's watch on its own CR would turn every reconcile into another reconcile. The write itself goes out from the in-memory object (a re-fetch would discard product-specific status fields); a 409 refreshes only the `resourceVersion`, preferring `GenericReconcilerConfig.APIReader` because the informer cache has not seen the competing write, and a `NotFound` is treated as success.

**Deletion uses owner-reference garbage collection, not finalizers.** The SDK registers **no** finalizer anywhere, so deleting a cluster CR runs **no SDK teardown code**. Everything the framework applies carries a controller owner reference and is reclaimed by Kubernetes GC. `Reconcile` detects deletion on two paths: background propagation (the `kubectl delete` default) removes the CR immediately and hits the `IsNotFound` branch, while foreground propagation (`--cascade=foreground`) and any product-registered finalizer leave the CR readable with a `deletionTimestamp` — checked right after the fetch, before the ClusterOperation gate and any mutating step. The second check is load-bearing: without it the pass re-creates every owned resource with `BlockOwnerDeletion: true`, which foreground deletion can never get past, producing a permanently `Terminating` CR in an un-backed-off recreate loop. The `operator.zncdata.dev/delete-pvcs` annotation (`reconciler.AnnotationDeletePVCs`) therefore only affects the **orphan** path — PVCs of a StatefulSet whose role group was removed or renamed in the spec. On cluster deletion those PVCs remain, because the SDK sets no `persistentVolumeClaimRetentionPolicy` and StatefulSet-managed PVCs carry no owner reference. Products with state outside owner-reference GC must clean it up themselves.

**Validation failures are loud.** Registered, enabled sidecar providers are validated before the StatefulSet is applied; a failure aborts the role group with `*reconciler.ValidationError` (`NewValidationError` / `IsValidationError`). A `podOverrides` layer that fails to decode is recorded on `config.MergedConfig.PodOverrideErrors` and re-emitted as a `PodOverrideIgnored` Warning event rather than being dropped silently.

**A `podOverrides` volumeMount at a framework-owned `mountPath` replaces it — and that is now a build failure.** Strategic merge patch keys `volumeMounts` by **`mountPath`**, not by `name`, so a mount declared at a path the framework already owns (`/kubedoop/config`, a CSI secret/listener path, the shared log volume) does not sit alongside the framework's — it rewrites the framework's entry to point at a different volume. When the override also declares that volume the resulting pod spec is **completely valid**, the API server accepts it, and the pods come up with the generated ConfigMap mounted nowhere: the product reads an empty config directory and crash-loops or silently runs on its built-in defaults. When it declares only the mount, the API server rejects the StatefulSet naming `spec.template.spec.containers[0].volumeMounts[0].name` — a field the user never wrote, with no mention of `podOverrides`. `StatefulSetBuilder` records both on `PodOverrideViolations()` and `BaseRoleGroupHandler` turns them into a `*reconciler.ValidationError` naming the mountPath, the displaced volume and `podOverrides`. Mounting at a *new* path is unaffected, which is what users normally mean.

### 3. RoleGroupHandler and BaseRoleGroupHandler
Product operators implement `RoleGroupHandler` to define resource building logic:
```go
type RoleGroupHandler[CR common.ClusterInterface] interface {
    BuildResources(ctx context.Context, k8sClient client.Client, cr CR, buildCtx *RoleGroupBuildContext) (*RoleGroupResources, error)
}
```

`BaseRoleGroupHandler.BuildResources` returns a ConfigMap, a headless Service, a StatefulSet, and a client-facing Service **when the role declares service ports**. It does not return a PDB: the framework's PDB comes from `roleConfig.podDisruptionBudget` and is a **role-level** resource built by `BuildRolePodDisruptionBudget` and applied once per role by the reconciler (`RoleGroupResources.PodDisruptionBudget` remains an escape hatch for an extra per-group PDB). `BuildRolePodDisruptionBudget` takes a single `*RoleBuildContext` — the role-scoped analogue of `RoleGroupBuildContext`, carrying `ClusterName`, `ClusterNamespace`, `ClusterLabels`, `ClusterSpec`, `RoleName` and `RoleSpec` — so a later role-level input needs no new signature. Product operators embed the base handler and override specific methods:
```go
handler := reconciler.NewBaseRoleGroupHandler[*v1alpha1.TrinoCluster](image, scheme)
handler.ProductName = "trino" // resolves spec.image into "{repo}/trino:{version}-kubedoop{v}"
handler.SetRoleContainerPorts("coordinator", ports)
handler.SetRoleServicePorts("coordinator", svcPorts)
```

`ProductName` is what opts a handler into CR-driven images: with it set, `spec.image` is resolved per role group through `ImageSpec.GetImage(ProductName)`; left empty, the handler's static `Image` (and any per-role override) is used and `spec.image` is ignored.

`BaseRoleGroupHandler` also implements `reconciler.RoleNameProvider`:
```go
type RoleNameProvider interface {
    ConfiguredRoleNames() []string
}
```
`ConfiguredRoleNames()` returns the sorted union of the role names the handler carries settings for (images, container ports, service ports, logging containers, main container names). The reconciler checks it against `spec.roles` and emits an `UnknownConfiguredRole` Warning event for names the CR does not declare — a typo there would otherwise silently produce a role group with no ports, no image override and no Service. It is a warning, not a failure, because a handler may legitimately be configured for optional roles.

When building the StatefulSet, `BaseRoleGroupHandler` consumes the role group's `config` (commons `RoleGroupConfigSpec`): `resources` (requests/limits, plus an opt-in data PVC via `StorageMountPath`), `affinity` (a RawExtension unmarshaled into `corev1.Affinity` and set on the pod spec — invalid JSON fails the build), and `gracefulShutdownTimeout` (a Go duration mapped to `terminationGracePeriodSeconds` — unparsable or non-positive values fail the build). All of these are applied before `podOverrides`, so user pod overrides keep precedence. The framework sets affinity only when the config provides one, so products that post-process the built StatefulSet with `if podSpec.Affinity == nil {...}` default guards remain correct.

**Labels.** Every resource the base handler builds carries the Kubernetes recommended set declared
in `pkg/constant/label.go` — `app.kubernetes.io/{name,instance,version,component,role-group,managed-by}` —
on both the object metadata and the pod template. `name` comes from `ProductName` and `version` from
`spec.image.productVersion`, so both are omitted for a handler that declares no `ProductName` (it
runs its static `Image` and never reads `spec.image`); `role-group` is absent on role-level
resources, which span every group of the role. An illegal label value for `name`/`version` drops
that one label rather than making every resource of the cluster unappliable.

The `.spec.selector` of the StatefulSet, Services and PDBs stays **narrower**: instance, component,
managed-by and the `<cluster>-<group>` marker (or the product's `LabelDomain` identity labels). A
selector is immutable, and `version` changes on every product upgrade. Upgrading an existing cluster
into this label set rolls its pods once, because the pod template gains labels; the frozen selector
keeps matching. See `pkg/reconciler/AGENTS.md` §15.

The marker goes through `reconciler.RoleGroupMarkerLabelKey(cluster, role, group)`, which returns
the natural `<cluster>-<group>` when that is a legal label key and `RoleGroupResourceName` when it
is not. A label key's name part is capped at 63 bytes, and ordinary names overrun it — a
43-character cluster plus a 21-character role group is 65 — at which point the API server rejects
the StatefulSet, both Services and the PDB of that role group. Keeping the natural form wherever it
is legal is what makes this safe for running clusters: the key is inside the immutable
`.spec.selector`, so only combinations that could never have produced a StatefulSet may change.

On top of that set the handler adds the **cluster CR's own labels**
(`RoleGroupBuildContext.ClusterLabels`), applied *first* so the framework's identity labels win over
a colliding key — which makes a colliding CR label inert rather than an error. This is the framework's
**only** label channel: whatever an operator's user wants on the workloads, including platform
opt-ins the SDK does not own such as `restarter.kubedoop.dev/enable`, is set by labelling the CR.
`BaseRoleGroupHandler.ExtraLabels` and `ExtraAnnotations` no longer exist — they put a deployment-time
decision in a compile-time field, and `ExtraLabels` mostly existed to supply the recommended labels
the framework now emits itself.

**The framework sets no annotations on the resources it builds**, and the CR's annotations are
deliberately not propagated the way its labels are: that map carries
`kubectl.kubernetes.io/last-applied-configuration` and the cleaner's `orphan.zncdata.dev/*` progress
markers. A product needing e.g. cloud LoadBalancer annotations on the client Service has no
supported way to set them today — tracked in zncdatadev/operator-go#553.

`RoleGroupHandlerFuncs` is a function adapter for simple handlers that don't need a full struct.

Besides the fixed fields (ConfigMap, Services, StatefulSet, PDB, MetricsService), `RoleGroupResources.ExtraResources []client.Object` lets products ship arbitrary per-role-group resources (e.g. a `listeners.kubedoop.dev` Listener CR) through the framework's apply path: same controller owner reference, applied BEFORE the StatefulSet because extras are typically pod-scheduling prerequisites. The cleaner does not discover arbitrary-GVK extras — extras of removed role groups are reclaimed only via owner-reference GC when the CR is deleted (see the field's doc comment).

### 4. RoleGroupBuildContext
Role and role group configuration reaches a handler through one struct, built per role group by the
reconciler and passed to `BuildResources`. There is **no role-level interface a product implements**:
`pkg/common/role_interface.go` (`RoleInterface`, `RoleInfo`, `RoleGroupInfo`) does not exist — the
reconciler iterates `spec.Roles` directly.

`RoleGroupBuildContext` carries `ClusterName`, `ClusterNamespace`, `ClusterLabels`, `ClusterSpec`,
`RoleName`, `RoleSpec`, `RoleGroupName`, `RoleGroupSpec`, `MergedConfig` (the folded
product-config/role/role-group overrides), `ResourceName` (`{cluster}-{role}-{group}`, truncated
with a hash suffix by `RoleGroupResourceName`), `ServiceAccountName` (the SA the reconciler
resolved and ensured), `SidecarManager`, `VolumeProviders` (see §16) and
`VectorAggregatorAddress`.

**Role and role group names are constrained by the CRD, not by convention.** The keys of
`spec.roles` and `spec.roles.<role>.roleGroups` must be lowercase RFC 1123 labels — a CEL
`x-kubernetes-validations` rule on `GenericClusterSpec.Roles` and `RoleSpec.RoleGroups` rejects
anything else at `kubectl apply`. They are not free-form: each becomes a segment of
`<cluster>-<role>-<group>` and the value of an `app.kubernetes.io/*` label, so `Coordinator`,
`my_role` and `a.b` all yield resource names the API server refuses. Without the rule that refusal
surfaced mid-reconcile as a permanently `Degraded` role quoting a `metadata.name` the user never
wrote. Both maps also carry a `maxProperties` bound (64 roles, 256 role groups): it exists because
the CEL cost estimator has no other handle on a map — without it the API server rejects the rule at
CRD creation time — and is set far above any real deployment.

### 5. Extension System
Three levels of extensions for injecting custom logic, all generic over the product CR type:
- **`ClusterExtension[CR]`**: `Name`, `PreReconcile`, `PostReconcile`, `OnReconcileError`
- **`RoleExtension[CR]`**: `Name`, per-role `PreReconcile` / `PostReconcile`
- **`RoleGroupExtension[CR]`**: `Name`, per-role-group `PreReconcile` / `PostReconcile`

There is no `Cleanup()` hook and no shutdown callback.

Hooks receive the **concrete product CR**, so an extension reads its own spec and writes its own
status with no type assertion:
```go
func (e *CatalogExtension) PreReconcile(ctx context.Context, c client.Client,
    cr *v1alpha1.TrinoCluster) error { /* cr.Spec.Catalogs is reachable directly */ }

var _ common.ClusterExtension[*v1alpha1.TrinoCluster] = &CatalogExtension{}
```

**Registration.** `ExtensionRegistry[CR common.ClusterInterface]` is generic over the CR type and
holds one ordered list per level. There are exactly three registration methods, each variadic over
`common.RegistrationOption`:
```go
registry := common.NewExtensionRegistry[*v1alpha1.TrinoCluster]()
registry.RegisterClusterExtension(myExtension,
    common.WithPriority(common.PriorityHigh),
    common.WithStopOnError(false),
)
registry.RegisterRoleExtension(myRoleExtension)
registry.RegisterRoleGroupExtension(myRoleGroupExtension, common.WithPriority(common.PriorityLow))
```
The level is part of the method name because the registry keeps one list per level; there is no
generic `Register()`. The type parameter is load-bearing: Go generic types are invariant, so a
registry instantiated for one product's CR cannot hold — or be shared with a reconciler of —
another product's extensions. Building one registry per CR type is therefore the only shape that
compiles.

**Ordering.** Extensions execute from highest to lowest priority (Lowest=0, Low=25, Normal=50,
High=75, Highest=100); same-priority extensions execute in **registration order** (each entry
carries a registration sequence number, so the order never depends on sort stability).

**Fault tolerance.** `WithStopOnError(bool)` overrides the per-hook default. `PreReconcile` and
`PostReconcile` stop at the first failure (a broken precondition makes the rest meaningless) and
return it as a `*common.ExtensionError`; a failure from an extension registered with
`WithStopOnError(false)` does not stop the remaining ones, but is still reported — the per-extension
`*ExtensionError` values are combined with `errors.Join` and returned. `OnReconcileError` runs every
handler and only logs their failures (returning nil), so the original reconcile error stays
authoritative — unless a handler opted into `WithStopOnError(true)`.

**Ownership.** A reconciler executes hooks from exactly one registry: the
`*common.ExtensionRegistry[CR]` passed as `GenericReconcilerConfig.ExtensionRegistry`. There is **no
process-wide registry** — `common.GetExtensionRegistry` and `common.ResetExtensionRegistry` do not
exist, because a package-level variable cannot carry a type parameter. Leaving the field unset is
legal and means *zero hooks run*: `NewGenericReconciler` substitutes an empty registry so the hook
call sites stay unconditional. A binary hosting several products builds one registry per CR type.

`registry.Clear()` empties a registry **in place** (for tests) rather than replacing it, so a
reconciler that captured the pointer at construction observes the reset. `Count()`,
`Has{Cluster,Role,RoleGroup}Extensions()` and `Get{Cluster,Role,RoleGroup}Extensions()` expose the
registered set in execution order.

### 6. Resource Builders
Fluent builders for K8s resources:
```go
sts := builder.NewStatefulSetBuilder(name, namespace).
    WithLabels(labels).
    WithReplicas(3).
    WithImage("trino:latest", corev1.PullIfNotPresent).
    WithResources(spec.Resources).
    AddPort("http", 8080, corev1.ProtocolTCP).
    Build()
```

Additional builders: `ServiceBuilder`, `HeadlessServiceBuilder`, `ConfigMapBuilder`, `PDBBuilder`,
`MetricsServiceBuilder`, `RoleBuilder`, `RoleBindingBuilder`, `ClusterRoleBuilder`,
`ClusterRoleBindingBuilder`, `ServiceAccountBuilder`.

Framework-wide builder semantics (see `pkg/builder/AGENTS.md` for the details): `Build()` deep-copies
every map and slice, so the returned object shares no state with the builder; `WithLabels` /
`WithAnnotations` **merge** rather than replace; and each builder exposes `NamespacedName()`
(`MetricsServiceBuilder` excepted).

`ServiceBuilder` additions worth knowing: `AddServicePort(corev1.ServicePort)` and
`WithPorts([]corev1.ServicePort)` append fully specified ports without dropping `nodePort`,
`appProtocol` or a named `targetPort` (unlike the scalar `AddPort`/`AddPortSimple`), and
`WithPublishNotReadyAddresses(bool)` sets `.spec.publishNotReadyAddresses` for quorum systems whose
members must resolve each other before readiness. `.spec.type` is always written explicitly
(default `ClusterIP`).

The reconciler builds the role group ConfigMap and both Services through these builders, so
behavior changes made here reach the framework path directly.

**Probes on the primary container: readiness is generated, liveness never is.** With ports declared,
`StatefulSetBuilder` attaches a TCP readiness probe to **`Ports[0]`** — which makes the first entry
of `SetRoleContainerPorts` part of the contract, not decoration: put the port that means "this pod
can serve" first. No liveness probe is generated at all. The framework knows neither which port
means healthy (the first declared one is as likely to be a metrics port) nor how long the product
takes to open it, and a liveness probe on a wrong guess kills the container on a timer forever;
readiness's worst case is a pod held out of its Services, which is visible and self-correcting.
`builder.DefaultTCPLivenessProbe(port)` reproduces the removed probe on a port the product chooses,
and `WithLivenessProbe` takes any probe. This is the opposite call from the **sidecar** probes
(§14), and deliberately so: there the framework owns the container's image, port and endpoint.

**Two StatefulSetSpec fields `podOverrides` cannot reach** (it is a `PodTemplateSpec`) have builder
knobs: `WithPodManagementPolicy` and `WithUpdateStrategy`. The default `podManagementPolicy` is
`Parallel` and that is a choice, not an inherited default — `OrderedReady` deadlocks a quorum
product at pod-0, because a ZooKeeper member or an HDFS JournalNode is not Ready until it sees a
quorum that does not exist until its peers start. It is immutable, so it can only be chosen at
creation. `updateStrategy` is mutable and not preserved by the apply path, which is what makes a
partitioned canary rollout converge through a normal reconcile.

### 7. Config Generation
Multi-format config generation over a split format contract:
```go
generator := config.NewMultiFormatConfigGenerator()
generator.RegisterDefaultFormats() // .xml, .properties, .yaml, .yml, .env, .ini
files, err := generator.GenerateFiles(map[string]map[string]string{
    "config.properties": {"key": "value"},
    "config.yaml":       {"nested": "data"},
})
parsed, err := generator.Parse("config.properties", content) // reverse direction, by file name
```

`config.ConfigMarshaler` (`Marshal(map[string]string) (string, error)`) is the **whole required
contract** — it is what `RegisterFormat`, `NewConfigGenerator` and `GetFormat` take and return,
because the framework's write path never reads a generated file back. `config.ConfigUnmarshaler`
(`Unmarshal(string) (map[string]string, error)`) is an **optional** capability discovered by
interface upgrade on the parse paths; a format that only emits is complete with `Marshal` alone, and
only an actual parse attempt fails, with a `*config.UnsupportedParseError` naming the file and the
format. Every adapter shipped with the SDK implements both (asserted at compile time in
`pkg/config/format.go`), so `GetFormat` results always parse in practice even though their static
type does not expose `Unmarshal`.

Supported formats: XML, Properties, YAML, Env, INI; all six extensions are registered by
`RegisterDefaultFormats`, and `RegisterFormat` adds or replaces one. When a file name matches
several registered extensions, the **longest** match wins deterministically. `config.ErrNoFormat`
is the `errors.Is`-able sentinel for a generator with no format configured. See
`pkg/config/AGENTS.md` for the per-adapter escaping and validation rules.

Adapters reject input they cannot represent faithfully rather than emitting output the target parser
would misread: YAML only round-trips a flat mapping, Env rejects keys that are not shell variable
names, and INI rejects keys/values containing line breaks or delimiters.

### 8. Health Checks
`ServiceHealthCheck` interface for business-level health verification (beyond Pod readiness):
```go
type ServiceHealthCheck interface {
    CheckHealthy(ctx context.Context, client client.Client, namespace, name string) (bool, error)
}
```

**The status conditions answer three separate questions and none is derived from another.**
`Available` = every role group has at least as many ready replicas as its spec asks for (`>=`, so a
scale-down that briefly leaves MORE ready than desired is still available). `Progressing` = a
revision rollout or replica change is in flight. `Degraded` = something is wrong the operator cannot
fix: a pod wedged in `CrashLoopBackOff`/`ImagePullBackOff`/`InvalidImageName`/a `CreateContainer*` or
`RunContainerError`, a pod that cannot be scheduled, a role group whose StatefulSet cannot be read,
or a failing `ServiceHealthCheck`. It is found by one `List` of the cluster's pods per pass.

`Degraded` is deliberately **not** derived from replica counts. Doing so made it fire during every
rolling update, scale-up and scale-down — planned changes that reduce ready replicas on purpose —
which is how the one condition worth alerting on became unalertable. Because the new inputs are
*states* rather than elapsed times, a **stuck** rollout still reports `Degraded=True` (its pods are
visibly failing) while a healthy one does not, and no progress-deadline machinery is needed.
Transient states (`ContainerCreating`, `PodInitializing`) and pods already being deleted are
excluded.

`reconciliationPaused` gets its own **`Paused`** condition with `Degraded=False` — a maintenance
window is not a fault, and the sibling `stopped` has always been reported that way. While paused the
framework still *observes*: `Available`/`Progressing` are re-evaluated from the live StatefulSets
rather than left at whatever the last running cycle wrote, and `ServiceHealthy` goes `Unknown`
instead of keeping a stale verdict. A paused reconcile therefore returns
`RequeueAfter: HealthCheckInterval` so those conditions keep up with reality.

`CompositeHealthCheck` combines multiple checks (all must pass); `ServiceHealthCheckFunc` adapts a
bare function. `AlwaysHealthy` and `AlwaysUnhealthy` are provided as convenience values. A check
reports a plain `(bool, error)` — there is no result struct.

The check runs against the API server through the injected `client.Client`; the SDK does not hand a `*rest.Config` to `ServiceHealthCheck`, so a product wanting an in-container exec probe constructs `util.NewExecUtil(client, restConfig)` itself. Each pass runs under a `context.WithTimeout` of `GenericReconcilerConfig.HealthCheckTimeout` (`DefaultHealthCheckTimeout` = 300s; non-positive disables the deadline), so a hanging probe cannot stall a reconcile worker.

### 9. Logging Configuration
`LoggingFramework`-aware logging config generation (Log4j, Log4j2, Logback, Python) lives in `pkg/productlogging`. `BaseRoleGroupHandler.LoggingContainers` (or the per-role `SetRoleLoggingContainers`) declares which containers get a generated config file; the framework merges the CRD logging spec, renders the file and injects it into the role group ConfigMap. The same declaration names the producers of the Vector log pipeline.

Default config file names come from each generator's `DefaultFileName()`: `logback.xml`, `log4j.properties`, `log4j2.properties` and — deliberately **not** `logging.py` — `log_config.py`, since a config directory on `sys.path` would otherwise shadow the standard library's `logging` module. `ContainerLogging.FileName` overrides the name per container. The rolling *log* file the Vector sources glob is separate and framework-owned: `<KubedoopLogDir>/<lowercased container>/<container><suffix>`, with `.log4j.xml` (log4j/logback), `.log4j2.xml` (log4j2) or `.py.json` (python) selecting the Vector edge parser; `ContainerLogging.LogFileName` may rename it only if the suffix survives and it stays a bare file name.

### 10. Product Config (`ProductConfig`)
Products contribute their computed configuration **as data through the same merge pipeline as CRD overrides**, instead of imperatively constructing resources. Set the optional `ProductConfig` field on `GenericReconcilerConfig` — a pure function returning an `*v1alpha1.OverridesSpec` (the same shape users write in the CRD):

```go
reconcilerCfg := &reconciler.GenericReconcilerConfig[*v1alpha1.TrinoCluster]{
    // ...
    ProductConfig: func(cr *v1alpha1.TrinoCluster, roleName, roleGroupName string) *commonsv1alpha1.OverridesSpec {
        overrides := map[string]map[string]string{
            "config.properties": {"http-server.http.port": "8080"},
        }
        if roleName == "coordinators" {
            overrides["config.properties"]["coordinator"] = "true"
        }
        return &commonsv1alpha1.OverridesSpec{ConfigOverrides: overrides}
    },
}
```

Precedence (low → high): **product config < role overrides < role group overrides**. Any value a user sets in the CRD always wins. `ConfigMerger.Merge` is variadic (`Merge(...*OverridesSpec)`) and folds layers in order; the previous two-argument call (`Merge(role, group)`) is still valid.

**This is config generation, not defaulting** — do not confuse it with the webhook `ProductDefaulter`:

| | `ProductDefaulter` (webhook) | `ProductConfig` (this) |
|---|---|---|
| Targets | typed **spec fields** (image, ports, replicas) | **config-file content** (config.properties, etc.) |
| When | admission, **persisted into spec** | every reconcile, **not persisted** |
| Upgrade propagation | no (frozen at admission) | **yes** (recomputed with current operator) |
| Derived-from-live-state | freezes/stales | **recomputed each reconcile** |

Use `ProductConfig` for product-intrinsic and derived config (e.g. a ZooKeeper connection string built from the actual resources, a JVM heap sized from resources, role-specific keys) so the product no longer hand-builds ConfigMaps/StatefulSets. Use `ProductDefaulter` for stable, user-facing typed spec defaults.

### 11. Discovery ConfigMaps
Every product operator publishes "discovery ConfigMaps" — cluster-level ConfigMaps (namespaced, in the CR's namespace; usually named `<cluster>`, optionally suffixed like `<cluster>-nodeport`) carrying client connection info. `reconciler.EnsureDiscoveryConfigMap` (in `pkg/reconciler/discovery.go`) owns the ensure semantics; the product owns computing the data map (address aggregation differs per product) and typically calls it from a `ClusterExtension.PostReconcile`:

```go
err := reconciler.EnsureDiscoveryConfigMap(ctx, client, scheme, cr, cr.GetName(),
    map[string]string{"KAFKA": bootstrapServers},
    reconciler.WithDiscoveryProductName("kafka"),
    reconciler.WithDiscoveryExtraLabels(map[string]string{
        reconciler.ClusterLabelKey("kafka.kubedoop.dev"): cr.GetName(),
    }),
)
```

The helper is idempotent (CreateOrUpdate), sets a controller owner reference (the ConfigMap is GC'd with the CR), and applies canonical labels (`app.kubernetes.io/instance`, `app.kubernetes.io/managed-by`, plus `app.kubernetes.io/name` via `WithDiscoveryProductName`); extra labels/annotations are merged via options, but canonical labels always win. Data is replaced wholesale.

### 12. External Dependencies

`GenericReconcilerConfig.Dependencies` declares the external objects a CR references but does not
create, so a missing one fails the cycle with a `Degraded` condition instead of producing pods that
crash-loop on an absent mount:

```go
Dependencies: func(cr *v1alpha1.TrinoCluster) []reconciler.Dependency {
    return []reconciler.Dependency{
        {Kind: reconciler.DependencySecret, Name: cr.Spec.ClusterConfig.KeytabSecret},
        {Kind: reconciler.DependencyConfigMap, Name: cr.Spec.ClusterConfig.AuthConfigMap},
    }
},
```

`DependencyKind` supports `DependencyConfigMap` and `DependencySecret`; an empty `Namespace`
defaults to the CR's namespace, and an empty `Name` is itself an error. Checks run before any role
is reconciled. The hook is **opt-in**: nil (the default) performs no checks.

`DependencyResolver.Validate(ctx, spec)` is a retained no-op that the reconcile flow does not call.
Richer, product-shaped checks — `ValidateS3Connection`, `ValidateDatabaseConnection`,
`ValidateZKConfig` — stay explicit product-side calls, because only the product knows where those
specs live in its CRD.

### 13. Controller Setup and Watches

`SetupWithManager(mgr)` registers watches for the kinds the framework owns. Products emitting
`RoleGroupResources.ExtraResources` must add those GVKs, or out-of-band changes to them produce no
reconcile event:

```go
err := r.SetupWithManagerOpts(mgr, reconciler.SetupWithManagerOptions{
    ExtraOwns: []client.Object{&listenersv1alpha1.Listener{}},
    Watches: []func(*builder.Builder) *builder.Builder{ /* arbitrary extra watches */ },
})
```

`ControllerBuilder(mgr, opts)` returns the configured `*builder.Builder` for operators that need to
add `WithOptions`, predicates, or anything else before calling `Complete`.

### 14. Sidecar Injection

`SidecarManager` owns injection into the pod spec. Registration is `Register(provider, config)` or
`RegisterWithPhase(provider, config, phase)`; a provider may instead declare its own phase by
implementing `sidecar.PhasedProvider`:

```go
type PhasedProvider interface{ Phase() int }
```

Phases order `InjectAll`: `SidecarPhaseProducer` (10) → `SidecarPhaseDefault` (50, the phase of
providers that declare none) → `SidecarPhasePipeline` (90). Within a phase, providers are injected in
name order, so a pod template does not re-render across reconciles. Resolution order is: the phase
passed to `RegisterWithPhase` > the provider's own `PhasedProvider.Phase()` > `SidecarPhaseDefault`;
`SidecarManager.Phase(name)` reports the effective one. The Vector provider declares
`SidecarPhasePipeline`, so — unless a caller overrides it with `RegisterWithPhase` — it is injected
after the producer containers whose shared log volume it mounts.

`SidecarProvider.Validate(ctx, client, namespace)` is a real gate, not advisory: the reconciler calls
`ValidateAll` before applying the StatefulSet, and a failure aborts the role group with a
`*reconciler.ValidationError`. The Vector provider's `Validate` requires the target ConfigMap to
exist **and** to carry the `vector.yaml` key, without which the agent starts unconfigured and
crash-loops.

**Vector registration passes three gates**, all evaluated in `buildSidecarManager`; failing any one
of them skips the sidecar rather than failing the role group:

1. the agent is enabled (`logging.enableVectorAgent`, after the role/role-group logging merge);
2. the handler's `LoggingProducers(roleName)` returns at least one producer — an agent with nothing
   to collect would mount an empty pipeline;
3. **something supplies `vector.yaml`**: the CR implements `reconciler.VectorAggregatorProvider` (the
   framework renders the file) *or* the handler implements `reconciler.VectorConfigProvider` and
   answers `ProvidesVectorConfig(roleName) == true` (the product writes it).

Gate 3 exists because registering the provider without a source for `vector.yaml` would fail the
`Validate` above on every cycle and abort the whole cluster's reconcile over a product that is simply
not wired for Vector. Instead the reconciler emits a `VectorSidecarSkipped` **Warning event** on the
CR naming the role group and both interfaces, and reconciliation continues. Within gate 3's first
branch, an empty `VectorAggregatorConfigMapName()` or an undiscoverable aggregator address is a hard
error, not a skip.

`SidecarConfig` carries `Image`, `ImagePullPolicy`, `Resources`, `EnvVars`, `Volumes`,
`VolumeMounts`, `Ports`, `Enabled`, `SecurityContext` and `Probes`. There is no `MainContainerName`
field and no `FindMainContainer` helper — a provider that must address the primary container uses
`sidecar.FindContainer(podSpec, name)`, and the primary container's name is controlled by
`BaseRoleGroupHandler.MainContainerName` / `SetRoleMainContainerName`.

**Every framework-injected sidecar is hardened by default.** Each provider sets
`sidecar.DefaultSecurityContext()` — `runAsNonRoot`, `allowPrivilegeEscalation: false`,
`capabilities: drop ALL`, `seccompProfile: RuntimeDefault`, i.e. exactly the container-level
requirements of the restricted Pod Security Standard — and a non-nil `SidecarConfig.SecurityContext`
replaces it **wholesale**, never merged. It deliberately sets neither `runAsUser` (identity is a
pod-level property, and a third-party image such as oauth2-proxy has its own `USER`) nor
`readOnlyRootFilesystem` (not part of the restricted profile, and it breaks a JVM writing
hsperfdata to `/tmp`); the Vector provider adds the read-only root itself, having established that
it only writes into its own volumes.

**Each sidecar carries the probe that fits its role, and the axis is whether it is in the data
path — not "probes are dangerous".** All three providers inject into `InitContainers` with
`RestartPolicy: Always` (native sidecars, stable since Kubernetes v1.33), and on such a container
the *type* of probe is a correctness question, because the three have three different blast radii:

| probe | effect on a native sidecar | reversible? |
|---|---|---|
| `readinessProbe` | decides the **Pod's** ready state — the pod leaves every Service (KEP-753: "Readiness probes of sidecars will contribute to determine the whole Pod readiness") | yes |
| `livenessProbe` | restarts that container only; pod readiness and Service membership untouched | yes |
| `startupProbe` | regular containers start only once every restartable init container has *started*, and with a `startupProbe` that means the probe **succeeded** | **no** |

From which the framework's policy follows:

- **Out-of-band sidecars (Vector, JMX exporter) → `livenessProbe` only.** A readiness probe here is a
  lie: their failure does not mean the product cannot serve, so gating the pod on them empties every
  Service for no reason. Liveness delivers the guarantee readiness cannot — a wedged-but-running
  agent is *recovered*, not merely visible. Deleting the probe outright is a third, worse option: it
  removes the coupling *and* the guarantee.
- **Data-path sidecars (oauth2-proxy) → `readinessProbe` + `livenessProbe`, never `startupProbe`.**
  Here readiness is the honest signal: the Service routes to the proxy's port, so a proxy that is not
  listening genuinely means the pod cannot serve. Istio's Envoy makes the same call. A `startupProbe`
  would also close the startup window and is the wrong tool — a proxy that can never answer would
  block the product's own container from ever starting, permanently, which is a *larger* blast radius
  than the coupling being avoided.

Per-provider specifics:

- **Vector** — `livenessProbe` `httpGet` on `VectorMetricsPath` at `VectorMetricsPort` (9598), with
  ~2 minutes of tolerance because restarting the agent drops its in-memory buffer. It targets the
  pipeline's `prometheus_exporter` sink rather than the API's `/health` because that endpoint
  exercises the **running topology**, while `/health` reports only that the API server is up. The
  rendered pipeline therefore carries an `internal_metrics` source and a `prometheus_exporter` sink
  on `0.0.0.0`, which is also the only way to alert on the pipeline
  (`vector_component_sent_events_total`, `vector_buffer_events`); it exposes component throughput,
  error counters and buffer depth, never log content. The port is declared as a container port but is
  **not** overridable via `SidecarConfig.Ports`, because it is baked into the rendered `vector.yaml`.
  The probe target is independent of what address the Vector *API* binds (`0.0.0.0`, as it always
  has) — whether that unauthenticated GraphQL endpoint should be reachable from the pod network is a
  security question about that endpoint, and letting probe placement decide it is what put it on the
  wildcard address in the first place.
- **JMX exporter** — `livenessProbe` `httpGet` `/metrics`, with deliberately forgiving timings
  (`timeoutSeconds: 10`, ~3 minutes to failure). Scraping makes the exporter collect from the JVM
  over JMX, so its response time tracks the product's GC; a readiness-grade 5s timeout is what made
  the original probe flap a healthy product out of its Services.
- **oauth2-proxy** — `readinessProbe` and `livenessProbe` on `/ping` (`OAuth2ProxyPingPath`), never
  `/ready`, which oauth2-proxy documents as a *deep* health check. Probing the local endpoint is what
  keeps readiness safe: `/ping` never contacts the IdP, so a runtime issuer outage cannot evict the
  pod. Readiness follows a fast-start / slow-evict shape (2s period, 30 failures ≈ 60s). Both probes
  are built from the *effective* port, so a `SidecarConfig.Ports` override cannot leave them pointing
  at a port nothing listens on.

Every tolerance window (60–180s) deliberately exceeds the default 30s termination grace period.
Sidecars are stopped **after** the main container and are probed while draining, so a shorter window
would let a normally-terminating sidecar be restarted mid-shutdown.

`SidecarConfig.Probes` (`sidecar.SidecarProbes`) makes all of this a **default rather than a law**:
`Startup`/`Liveness`/`Readiness` replace a provider's probe **wholesale** (never merged — probe
handlers are a Kubernetes one-of, and a probe carrying two handlers is rejected), and
`DisableStartup`/`DisableLiveness`/`DisableReadiness` remove it. A `Disable` flag wins if both are
set. `sidecar.ApplyProbes` deep-copies the override in, so a built pod spec never aliases the
caller's config. Setting `Readiness` is legal — a product whose sidecar genuinely is in the request
path may want the pod gated on it — but on a native sidecar that field means "this **Pod** is
unready", not "this container is unready".

Providers shipping their own upstream image (oauth2-proxy) implement `sidecar.OwnImageProvider` so
`SetProductImage` leaves their pinned default alone.

### 15. Slice Merge Strategy

`config.ConfigMerger.SliceMergeStrategy` selects how `cliOverrides` merge:
`MergeStrategyReplace` (the default) or `MergeStrategyAppend`. **The framework path is always
Replace** — `GenericReconciler` constructs its merger with `config.NewConfigMerger()` and exposes no
knob — so `Append` is reachable only by driving `config.ConfigMerger` directly from product code.

An empty override slice means "unset", not "clear": a role group cannot erase the CLI arguments its
role set, because only a non-empty override replaces (or extends) the base.

### 16. Secret and Listener CSI Volumes

`security.SecretProvisioner` declares secret-operator CSI volumes and resolves their mount paths;
`listener.ListenerProvisioner` does the same for the listener-operator. Both satisfy
`reconciler.VolumeProvider`, so a product appends them to
`RoleGroupBuildContext.VolumeProviders` (or calls `AutoInject(stsBuilder)`), and both expose
`Path(volumeName) (string, error)` / `MustPath(volumeName) string` — config generation should ask
for the path rather than hardcode it.

Mount bases: secret volumes default to `constant.KubedoopMountDir` (`/kubedoop/mount/<volume>`),
listener volumes to `constant.KubedoopListenerDir` (`/kubedoop/listener/<volume>`). Both
`WithMountBasePath` overrides ignore an empty argument (keeping the default) and panic on a
relative path, since a relative base composes into a container `mountPath` the API server rejects.

Registration constructors: `TLS`, `TLSPEMFormat`, `ServiceTLS`, `KerberosVolume`, `ListenerVolume`,
`Custom`, `CredentialsVolume`.

```go
security.ListenerVolume(volumeName, secretClass, listenerVolumeName string, format SecretFormat)
```
`ListenerVolume` requires `listenerVolumeName` — the name of the pod volume mounting the listener,
which the secret-operator resolves to that listener's addresses. It emits the scope
`listener-volume=<listenerVolumeName>`; a bare `listener-volume` entry carries no name for the CSI
driver to resolve, so an empty argument panics. The same rule is enforced for every named scope key
(`service=`, `listener-volume=`) by `SecretVolumeRegistration.WithScope` and
`SecretProvisioner.Register`. Unknown scope keys still pass — the scope vocabulary belongs to the
secret-operator. `security.ScopeString(*commonsv1alpha1.CredentialsScope)` renders a CRD scope into
that annotation value.

A scope **name** may contain neither `,` nor `=`: the annotation has no escaping, so such a name
does not quote itself — it adds scopes. `services: ["mysvc,node"]` would render `service=mysvc,node`
and hand the CR author a **node-scoped** certificate covering the node's hostname and IP. The CRD
rejects it at admission (`items:Pattern` on `CredentialsScope.Services`/`.ListenerVolumes`), and
`ScopeString` drops any entry it cannot render as itself for the cases admission cannot reach (a CR
stored before those markers, a scope built in Go).

`pkg/listener` has **no scope API**: `VolumeRegistration.WithScope`, the `ListenerScope` type,
`ListenerScopeNode`, `ListenerScopeCluster` and `ListenerScopeAnnotation` do not exist, and listener
PVC templates carry no `listeners.kubedoop.dev/scope` annotation. A listener volume is declared with
`listener.NewVolume(name, class)` and optionally `.WithListenerName(name)`; a registration with
neither a class nor a listener name is rejected.

## Building a New Operator

1. **Define CRD** - Create API types embedding `metav1.TypeMeta` + `metav1.ObjectMeta`, marked
   `+kubebuilder:object:root=true` and registered with `SchemeBuilder.Register`; implement
   `ClusterInterface`'s two methods (`GetSpec`, `GetStatus`) and run `make generate` for
   `DeepCopy`/`DeepCopyObject`. Scheme registration is required — the reconciler reads the fetched
   object into the CR itself
2. **Create RoleGroupHandler** - Embed `BaseRoleGroupHandler` for default resource building, or implement `RoleGroupHandler` directly; set `ProductName` to opt into CR-driven images
3. **Provide Product Config** (optional) - Set `ProductConfig` on `GenericReconcilerConfig` to contribute product-intrinsic/derived config as the lowest merge layer
4. **Declare Dependencies** (optional) - Set `Dependencies` so missing ConfigMaps/Secrets fail fast with a Degraded condition
5. **Register Extensions** (optional) - Add custom hooks on a `common.NewExtensionRegistry[*MyCluster]()` and pass it via `GenericReconcilerConfig.ExtensionRegistry` — without that field the reconciler runs no hooks at all
6. **Setup Webhooks** (optional) - Use common defaults/validators from `pkg/webhook/`
7. **Register Health Checks** (optional) - Implement `ServiceHealthCheck` for business-level health verification
8. **Create main.go** - Use `GenericReconciler` with your handler, and register any extra-resource GVKs through `SetupWithManagerOpts`

See `examples/trino-operator/` for a complete example.

## Development Rules

All AI agents and developers working on this project **must** follow these rules:

### Before Committing Code
1. Run `make generate` if you modified any API structs in `pkg/apis/`
2. Run `make lint` — must pass with zero errors
3. Run `make test` — all tests must pass
4. **Never commit if lint or tests fail**

### After Code Changes
- Always run `make test` to verify nothing is broken
- Always run `make lint` to ensure code quality
- If adding new public interfaces, update AGENTS.md accordingly

### Code Style & Conventions
- **Formatting**: Must pass `go fmt`
- **Linting**: Must pass `golangci-lint`
- **CRDs**: Uses `kubebuilder` markers (tags) for code generation
- **Generation**: When modifying API structs in `pkg/apis`, always run `make generate`
- **Testing**: Use Ginkgo v2 + Gomega; test files use `suite_test.go` pattern
- **Error Handling**: Use error types from `pkg/common/errors.go` and `pkg/reconciler/errors.go`
- **Generics**: the framework is generic over the product CR type (`GenericReconciler[CR]`,
  `RoleGroupHandler[CR]`, `ClusterExtension[CR]`), so product code never asserts on a CR type.
  Inside the SDK, a type assertion is acceptable **only** to detect an optional capability on a
  method-set interface — `RoleNameProvider`, `rolePodDisruptionBudgetBuilder`,
  `sidecar.PhasedProvider`, `sidecar.OwnImageProvider` — never to recover a concrete product type

### Design Constraints
- Follow the layered architecture defined in `docs/architecture.md`
- Use Go Generics for type safety; reserve type assertions for optional-capability interfaces
- All operations must be idempotent
- Config merging follows the strict merge strategy: Deep Merge for maps, `SliceMergeStrategy` for
  slices (Replace by default and always on the framework path — see §15), Strategic Merge Patch for
  PodTemplate
- Extensions must be registered during Operator initialization (in `main.go` before Manager starts)
  on a `common.NewExtensionRegistry[*MyCluster]()` passed via
  `GenericReconcilerConfig.ExtensionRegistry`. That field is the only path to the hooks; there is no
  process-wide registry to fall back on
- Override fields (`configOverrides`, `envOverrides`, `cliOverrides`, `podOverrides`) are **flattened** directly at Role/RoleGroup level, NOT nested under an `overrides` field
- Resource cleanup relies on owner references, not finalizers — do not add a finalizer without
  updating the deletion contract documented above

## Testing
- Unit tests use Ginkgo v2 with Gomega matchers
- Each package has a `suite_test.go` for test setup
- `pkg/testutil/` provides envtest helpers (`TestEnv`), object/CR builders, matchers, and mocks.
  `MockCluster` implements `common.ClusterInterface` directly (there is no wrapper type);
  `MockRoleGroupHandlerFor[CR]` is the generic handler mock and `MockRoleGroupHandler` is the alias
  bound to `*MockCluster`. `testutil.AltMockCluster` is a second, deliberately minimal CR type,
  used to prove per-CR-type isolation (one reconciler and one extension registry per CR type); it
  lives in `pkg/testutil` rather than a `_test.go` file because controller-gen cannot generate a
  CRD schema from an unexported type in a test file.
- **The test CRDs are generated, not hand-written.** `make manifests` runs controller-gen over
  `./pkg/testutil/...` and writes `config/crd/bases/test.zncdata.dev_{mock,altmock}clusters.yaml`,
  which `TestEnv` installs. This is load-bearing rather than tidiness: those CRDs previously
  declared `x-kubernetes-preserve-unknown-fields: true` for spec and status and nothing else, so
  the API server performed **no defaulting, no validation and no pruning** anywhere in the suite —
  every `+kubebuilder:default` and `+kubebuilder:validation:*` marker in `pkg/apis` was inert, and
  any test that believed it exercised them was exercising Go zero values. `make test` depends on
  `manifests`, and `pkg/testutil/crd_schema_test.go` asserts the schema is live; if those specs
  fail, the CRDs have regressed to schema-free and the rest of the suite's guarantees are gone.
- Adding a new root type to `pkg/testutil` needs `+kubebuilder:object:root=true` (plus
  `+kubebuilder:subresource:status` for a cluster CR). A status struct that **embeds** another
  status needs an explicit `+kubebuilder:object:generate=true`: embedding promotes the parent's
  `DeepCopyInto`, so controller-gen concludes the type is already satisfied and emits nothing,
  producing code that does not compile. The package deliberately carries no package-level
  `generate=true` marker, which would drag in the mock handlers whose fields are funcs.
- Run tests: `make test`
- All tests must pass before any code is committed

## AI Worktree Development Mode

**IMPORTANT**: When making code changes, work in a worktree under `.worktree/`, NOT in the main working directory.

### Workflow
1. Create worktree: `git worktree add .worktree/<branch-name> -b <branch-name>`
2. Work in `.worktree/<branch-name>/` directory
3. Test: `cd .worktree/<branch-name> && make lint && make test`
4. Commit changes in the worktree
5. Push and create PR from the worktree branch
6. Cleanup: `git worktree remove .worktree/<branch-name>`

### Rules
- NEVER modify files directly in the main working directory
- Each task gets its own worktree with a descriptive branch name
- Run `make generate` if API structs are modified
- Run `make lint && make test` before committing
