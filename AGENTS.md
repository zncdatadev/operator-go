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
│   ├── constant/                 # Kubedoop paths, labels, domains, restarter annotations, JMX agent
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
│   ├── architecture.md           # Core Technical Architecture
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
4. Ensure the workload ServiceAccount (always; name derived from the CR by
   `ServiceAccountResourceName`) and, when `WorkloadRBACRules` is set, its Role/RoleBinding (§11c);
   warn about handler-configured role names the CR
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

Each "Apply" is create-OR-UPDATE (issue #526): when the resource already exists, the live object is updated to the handler-built desired state every reconcile — labels are replaced wholesale, annotations are merged (foreign annotations survive, at both the object level and inside the StatefulSet's pod template), and spec/data is copied per kind while preserving Kubernetes immutable/allocated fields (StatefulSet `selector`/`serviceName`/`volumeClaimTemplates`/`podManagementPolicy`; Service `clusterIP(s)`/`ipFamilies` and allocated NodePorts). Arbitrary-GVK extras get a generic top-level field copy. See `copyDesiredState` in `pkg/reconciler/apply.go`. Changing an immutable field for an existing cluster requires a manual delete/recreate migration — and the framework now **says so**: when a handler's desired value for a preserved field differs from the live one, `applyResource` emits an `ImmutableFieldIgnored` Warning event on the CR naming the resource and the field paths. Preserving those fields silently is what let a storage resize be accepted, reported as `ReconcileComplete=True`, and never applied. Only a field the handler actually set is reported (an unset field is declining to have an opinion, not a change request), and among the Service's preserved fields only `clusterIP` is — the others are API-server allocations, so a difference there would be noise on every reconcile. The event is emitted **before** the write's own error is returned, so a rejected Update still says which of the user's changes the framework had already dropped.

**`volumeClaimTemplates` is the one preserved field the pod template depends on, so the mounts follow
what was actually preserved.** Preserving the claim templates alone left an incoherent StatefulSet in
both directions. Adding `config.resources.storage` to a **live** role group produced a template
mounting a claim the framework had just declined to create — `volumeMounts[0].name: Not found:
"data"`, a field the user never wrote. On Kubernetes 1.34+ the API server rejects that Update
outright, leaving the role group `Degraded` on every pass with no recovery short of deleting the
StatefulSet by hand; older servers accept it and reject every **pod** the StatefulSet controller then
creates, so the workload never progresses and the error never reaches the cluster's status at all. **Removing** it was worse, because it was *accepted*: the claim template stayed,
the mount did not, and the pods rolled into a product writing to the container's writable layer while
its bound PVCs sat mounted nowhere — no event, no condition, no log line. `copyStatefulSetState`
therefore drops a mount whose claim was not created (unless an ordinary volume of that name backs it)
and restores **every** mount a preserved claim had **from the live template**, so the path is read
rather than invented; the restore is keyed on the mount *path*, since Kubernetes lets one volume be
mounted several times and a path the desired template already uses wins. A rename hits both branches
and lands on the preserved claim. This makes the transition
converge and stay coherent — it does not make it happen, and `spec.volumeClaimTemplates` is still
reported as ignored. That report is also the one place an **empty** desired value counts: everywhere
else an unset field is the handler declining to have an opinion, but an empty `volumeClaimTemplates`
is the handler stating this role group has no storage, which is exactly the direction that used to be
applied in silence.

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

Three keys are withheld from that channel — `metrics.kubedoop.dev/service`, `pdb.kubedoop.dev/role`
and `pdb.kubedoop.dev/role-group`. They are the framework's **slot markers**: a reclaim selects an
object for deletion by their presence or value, and unlike the `app.kubernetes.io/*` set nothing
overwrites them afterwards, so a CR carrying one would stamp it on every built resource and make
each answer to a reclaim aimed at the slot. The filter is an enumerated set rather than a
`kubedoop.dev` prefix rule precisely because `restarter.kubedoop.dev/enable` shows that domain is
shared with the platform; every other CR label propagates unchanged.

> **Caveat (upstream bug).** commons-operator's `getRefConfigMapRefs` returns after the *first*
> ConfigMap volume it finds, so a pod mounting several ConfigMaps only ever gets one of them
> watched — see zncdatadev/commons-operator#298. The secret path (`getRefSecretRefs`) is correct.

The framework already satisfies the restarter's precondition: the role group ConfigMap is mounted as
the `config` volume. What it does **not** do is set the label, and without it a `configOverrides`
change simply does not roll.

**The apply path preserves the restarter's stamp.** The pod template's annotations are MERGED, not
replaced — the same rule the object's own annotations follow, for the same reason: another controller
writes there. `copyStatefulSetState` assigns `live.Spec = desired.Spec` wholesale so that new mutable
fields converge by default, and the pod template lives inside that spec, so before this rule a
handler that never builds `configmap.restarter.kubedoop.dev/<name>` silently removed it on the next
reconcile. That Update woke the restarter — its predicate matches the label on every Update, not only
on Create — which re-stamped, which woke the reconciler through its own `Owns(&appsv1.StatefulSet{})`
watch. Neither side is failing, so the workqueue `Forget`s each pass and nothing backs off: the pods
rolled for as long as the label was set. Pod-template *labels* are still replaced wholesale, because
they must match the StatefulSet's immutable `.spec.selector`.

This applies to `configOverrides` alone. `envOverrides` and `cliOverrides` reach the container as
env vars and args through `MergedConfig` (`StatefulSetBuilder.WithConfig`), and `podOverrides`
patches the template directly — all three change the pod template, so they roll natively with no
restarter involved.

**Role iteration is best-effort.** A failing role does not stop the others, and a failing role group does not stop its siblings, the role-level PDB, or the role's PostReconcile hook. Steps 8-10 (orphan cleanup, health, cluster PostReconcile) run regardless. Roles and role groups are independent workloads: aborting at the first failure meant one unparsable value on the alphabetically-first role indefinitely blocked the *deletion* of an unrelated role group, the health of every other role, and the discovery ConfigMap a product publishes from PostReconcile. The per-role errors are combined with `errors.Join` and returned once, so the cluster still goes `Degraded` and the workqueue still backs off. Iteration stays **sorted** so that aggregated message is byte-stable across cycles — an unstable message would defeat the no-op guard in `updateStatus` and make the controller reschedule itself forever. The single exception is a 429: a `*RateLimitError` aborts the pass immediately, because pushing the remaining roles through would only deepen the backlog.

**Requeue cadence.** A successful reconcile returns `ctrl.Result{RequeueAfter: HealthCheckInterval}` (`DefaultHealthCheckInterval` = 120s; a negative value disables the periodic wakeup), or the cleaner's earliest pending wakeup when that is sooner — a remaining gray-delete deadline, or the drain poll interval of an orphan deletion in flight. Watches only cover the kinds the framework owns, so anything that changes without producing an event — a product `ServiceHealthCheck` probe, a grace period running out, a StatefulSet finishing its drain — depends on this timer.

**Orphan cleanup discovers its work from the live cluster, not only from `status.roleGroups`.** The cleaner unions two inventories: the role group ConfigMaps and StatefulSets this CR controller-owns that carry the framework's labels (`instance` + `managed-by` + `component` + `role-group`) **and** whose name is exactly what `RoleGroupResourceName` produces for those labels, plus the `status.roleGroups` ledger. The ledger alone is a record the operator must have *successfully written*, so losing it — a process death between applying a role group's resources and updating the CR, a backup tool restoring the CR without its status subresource, a `kubectl replace` — used to make those resources invisible to the cleaner permanently, holding their PVCs and pods until a human noticed. The name check is what keeps the live half safe: a discovery ConfigMap carries the same instance/managed-by pair and owner reference, and a product's `ExtraResources` may carry the handler's entire label set. An empty owner UID disables live discovery, as it does the role-PDB reclaim.

**Orphan cleanup is a multi-pass state machine.** A role group removed from the spec is retired over several reconciles: scale the StatefulSet to zero (under `RetryOnConflict`), wait for the controller's ordered drain (`.status.replicas` reaching 0), then delete `PDB → [PVCs] → StatefulSet → [product extras] → ConfigMap → Service → headless → metrics`, each step confirmed absent before the next is issued. The PVC step is opt-in (`operator.zncdata.dev/delete-pvcs`) and sits **after** the drain on purpose: deleting a role group is undoable right up until its data goes, so nothing irreversible happens while the pods are still running, and re-adding a group mid-teardown costs a restart rather than the data. It goes before the StatefulSet because the cleaner finds the PVCs through its selector — the other order would strand them — and the drain-timeout path falls through to it so a stuck pod cannot silently leak the volumes. A group's status entry is pruned only after a real deletion; a failure is isolated to its own group and the others still progress; the state machine's progress annotations (`orphan.zncdata.dev/pending-deletion`, `orphan.zncdata.dev/drain-started`) are reset on every role group that IS in the spec, so a group that was orphaned and then re-added starts its next teardown from scratch instead of inheriting the previous one's timestamps; a 429 becomes a `*reconciler.RateLimitError` that aborts the pass and backs off instead of marking the cluster `Degraded`. The cleaner also reclaims the role-level PDB of a role deleted from the spec outright, found by the `pdb.kubedoop.dev/role` label (`reconciler.LabelRolePodDisruptionBudget`) rather than by derived name.

**Status writes are conditional.** `updateStatus` skips the write entirely when the whole CR is `apiequality.Semantic.DeepEqual` to the object read at the start of the cycle — comparing the whole object, not just the embedded generic status, so a product's own status fields count too. Without that guard the controller's watch on its own CR would turn every reconcile into another reconcile. The write itself goes out from the in-memory object (a re-fetch would discard product-specific status fields); a 409 refreshes only the `resourceVersion`, preferring `GenericReconcilerConfig.APIReader` because the informer cache has not seen the competing write, and a `NotFound` is treated as success.

**Deletion uses owner-reference garbage collection, not finalizers.** The SDK registers **no** finalizer anywhere, so deleting a cluster CR runs **no SDK teardown code**. Everything the framework applies carries a controller owner reference and is reclaimed by Kubernetes GC. `Reconcile` detects deletion on two paths: background propagation (the `kubectl delete` default) removes the CR immediately and hits the `IsNotFound` branch, while foreground propagation (`--cascade=foreground`) and any product-registered finalizer leave the CR readable with a `deletionTimestamp` — checked right after the fetch, before the ClusterOperation gate and any mutating step. The second check is load-bearing: without it the pass re-creates every owned resource with `BlockOwnerDeletion: true`, which foreground deletion can never get past, producing a permanently `Terminating` CR in an un-backed-off recreate loop. The `operator.zncdata.dev/delete-pvcs` annotation (`reconciler.AnnotationDeletePVCs`) therefore only affects the **orphan** path — PVCs of a StatefulSet whose role group was removed or renamed in the spec. On cluster deletion those PVCs remain, because the SDK sets no `persistentVolumeClaimRetentionPolicy` and StatefulSet-managed PVCs carry no owner reference. Products with state outside owner-reference GC must clean it up themselves.

**Validation failures are loud.** Registered, enabled sidecar providers are validated before the StatefulSet is applied; a failure aborts the role group with `*reconciler.ValidationError` (`NewValidationError` / `IsValidationError`). A `podOverrides` layer that fails to decode is recorded on `config.MergedConfig.PodOverrideErrors` and re-emitted as a `PodOverrideIgnored` Warning event rather than being dropped silently. A fixed `RoleGroupResources` slot built under a name or namespace the framework does not own fails the same way, **before anything is applied** (§3).

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

**`ProductName` names the product; `ImageDefaults` supplies the image.** They used to be one
switch, and because the image half could not express the kubedoop tag convention, a product that
wanted `app.kubernetes.io/name` had to give up all of it.

```go
handler.ProductName = "trino"                 // app.kubernetes.io/name AND the repo path segment
handler.ImageDefaults = commonsv1alpha1.ImageSpec{
    Repo:            "quay.io/zncdatadev",
    ProductVersion:  "476",
    KubedoopVersion: version.BuildVersion,      // the operator's own build version
}
```

`ImageSpec.ResolveImage(productName, defaults)` folds the two layers per field, user first, so a CR
stating only `productVersion` still yields a valid `…:476-kubedoop0.2.0` reference. `ImageDefaults`
is read **every reconcile**, which is what a webhook cannot do: webhook defaults are persisted at
admission and never recomputed, freezing `kubedoopVersion` at whatever operator version first
admitted the CR (§10 and `docs/architecture.md` §2.6).

An unresolvable `spec.image` **fails the role group**, naming the missing field, instead of silently
falling back to the handler's static image and running a version nobody asked for. With
`ProductName` empty the handler resolves nothing from the CR beyond `spec.image.custom` — the shape
a product uses when it resolves images itself — and that path never errors.
`app.kubernetes.io/version` follows the **resolved** version, so it is present whenever the image
came from `ImageDefaults` too.

`BaseRoleGroupHandler` also implements `reconciler.RoleNameProvider`:
```go
type RoleNameProvider interface {
    ConfiguredRoleNames() []string
}
```
`ConfiguredRoleNames()` returns the sorted union of the role names the handler carries settings for (images, container ports, service ports, logging containers, main container names). The reconciler checks it against `spec.roles` and emits an `UnknownConfiguredRole` Warning event for names the CR does not declare — a typo there would otherwise silently produce a role group with no ports, no image override and no Service. It is a warning, not a failure, because a handler may legitimately be configured for optional roles.

When building the StatefulSet, `BaseRoleGroupHandler` consumes the role group's `config` (commons `RoleGroupConfigSpec`): `resources` (requests/limits, plus an opt-in data PVC via `StorageMountPath`; `storage.storageClass` is a `*string` because Kubernetes reads `storageClassName: ""` as "bind only a pre-provisioned PV, never dynamically provision one" — a role group that set it to `""` to mean "inherit the role's" would get a PVC that stays `Pending` forever), `affinity` (see below), and `gracefulShutdownTimeout` (a Go duration mapped to `terminationGracePeriodSeconds` — unparsable or non-positive values fail the build). All of these are applied before `podOverrides`, so user pod overrides keep precedence. The framework sets affinity only when the config provides one, so products that post-process the built StatefulSet with `if podSpec.Affinity == nil {...}` default guards remain correct.

**`config.affinity` is decoded STRICTLY, and merges per member.** The CRD carries it as a
schema-free `RawExtension` (`type: object` + `x-kubernetes-preserve-unknown-fields`), so the API
server neither validates nor prunes it. `reconciler.DecodeAffinity` therefore decodes with
`DisallowUnknownFields` and an unknown field **fails the build**, naming it. Before that, `nodeAffinty`
(one letter short) passed admission, decoded into an empty `corev1.Affinity`, and the pods were
scheduled anywhere — with no event, no log line and no status change, even though affinity is the
scheduling *contract* for these products (rack awareness, spreading a quorum, colocating a worker
with its data). The trade-off is deliberate: a field from a newer Kubernetes API than the SDK is
built against is now rejected rather than ignored, which is the honest answer, since the framework
cannot honor a field it does not know.

`config.affinity` is also the one field in that block that **does not fold per leaf**: a role
group's affinity REPLACES the role's rather than merging into it, and `affinity: {}` clears it.
`resources` is a set of independent knobs where overriding `cpu.max` and keeping the rest is the
normal thing to want; an affinity is a single scheduling *policy*, and a role group that needs
different scheduling needs a different policy rather than a partial edit of the role's. Kubernetes
treats `PodSpec.Affinity` the same way — Helm values and Kustomize patches replace it. Per-member
inheritance was tried and reverted: it invented a semantic users would have to learn, and it removed
the only way to say "this group has no affinity", which a single-node development group needs.

`RoleGroupHandlerFuncs` is a function adapter for simple handlers that don't need a full struct.

**The framework owns the NAME of every fixed slot; the handler owns its content.** `ConfigMap`,
`Service`, `StatefulSet` and `PodDisruptionBudget` must be named `RoleGroupBuildContext.ResourceName`,
`HeadlessService` and `MetricsService` that name plus `-headless` / `-metrics`, and all six must sit
in the cluster's namespace. Both paths that *remove* a slot address it by that derived name — the
in-spec reclaim and the orphan teardown — so a slot filled under another name used to be applied,
owner-referenced and then reclaimed by nothing, surviving until the cluster CR itself was deleted (a
metrics Service left as a Prometheus target with no endpoints). It is now a `*ValidationError`
raised **before any resource is applied**, so a rejected declaration leaves nothing half-converged.
`builder.MetricsServiceBuilder` therefore exposes no name override, and `ExtraResources` is the
supported route for an object whose name the product chooses.

Besides the fixed fields (ConfigMap, Services, StatefulSet, PDB, MetricsService), `RoleGroupResources.ExtraResources []client.Object` lets products ship arbitrary per-role-group resources (e.g. a `listeners.kubedoop.dev` Listener CR) through the framework's apply path: same controller owner reference, applied BEFORE the StatefulSet because extras are typically pod-scheduling prerequisites. **Extras of a removed role group are reclaimed too**, provided the product registers their kinds through `SetupWithManagerOptions.ExtraOwns` and labels them with the role group's labels; the teardown deletes them right after the StatefulSet, mirroring the apply order. Unregistered or unlabelled extras keep the old behaviour and wait for owner-reference GC on cluster deletion (see §13 and the field's doc comment).

### 4. RoleGroupBuildContext
Role and role group configuration reaches a handler through one struct, built per role group by the
reconciler and passed to `BuildResources`. There is **no role-level interface a product implements**:
`pkg/common/role_interface.go` (`RoleInterface`, `RoleInfo`, `RoleGroupInfo`) does not exist — the
reconciler iterates `spec.Roles` directly.

`RoleGroupBuildContext` carries `ClusterName`, `ClusterNamespace`, `ClusterLabels`, `ClusterSpec`,
`RoleName`, `RoleSpec`, `RoleGroupName`, `RoleGroupSpec`, `MergedConfig` (the folded
product-config/role/role-group overrides), `ResourceName` (`{cluster}-{role}-{group}`, truncated
with a hash suffix by `RoleGroupResourceName`), `ServiceAccountName` (the SA the reconciler derived
and ensured — `ServiceAccountResourceName(kind, cluster)`, never configured and never empty),
`SidecarManager`, `VolumeProviders` (see §16) and `VectorAggregatorAddress`.

**It is also where a product puts its per-CR inputs.** `Image`, `ImagePullPolicy`, `ContainerPorts`
and `ServicePorts` on the context outrank the handler's own, and the context is rebuilt per role
group per reconcile:

```go
func (h *MyHandler) BuildResources(ctx context.Context, c client.Client, cr *MyCluster,
    buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
    buildCtx.ContainerPorts = h.portsFor(cr, buildCtx.RoleName) // depends on cr.Spec.Tls
    return h.BaseRoleGroupHandler.BuildResources(ctx, c, cr, buildCtx)
}
```

`ListenerClass` and `MainContainerCustomizer` ride the same channel, and replace the other
post-build habit: **the framework builds a complete object and the product reaches in to edit it.**

```go
buildCtx.ListenerClass = listener.ListenerClassExternalUnstable   // Service type set at Build()
buildCtx.MainContainerCustomizer = func(c *corev1.Container) error {
    c.Command = []string{"/bin/zkServer.sh"}                      // no Containers[0] lookup
    return nil
}
```

The customizer runs on the assembled primary container **before** `podOverrides` are strategic-merged,
which is why it cannot be a post-build patch: a product editing the returned StatefulSet lands
*after* the merge and silently beats the user. It is handed the container by identity, so nobody
indexes `Containers[0]` — an assumption the framework never made, and which a sidecar provider
inserting a container earlier quietly breaks. Returning an error fails the role group with a
`*ValidationError`; **changing the image is rejected the same way**, since the image is resolved once
and propagated to the sidecars before the StatefulSet is built (`buildCtx.Image` is that channel).

`listener.ServiceTypeFor` is the shared class→type mapping restored from v0.12.6:
`cluster-internal` → ClusterIP, `external-unstable` → **NodePort**, `external-stable` →
LoadBalancer, anything else → ClusterIP. It lives in `pkg/listener` rather than `pkg/builder`
because `pkg/listener` already imports `pkg/builder`.

**One handler instance serves every cluster** — it is built once in `main.go` — so the older idiom
of assigning `h.Image` or calling `h.SetRoleContainerPorts` inside `BuildResources` writes
per-cluster values into process-wide state. Above `MaxConcurrentReconciles: 1` those writes race
between clusters; at 1 they still leak, because a product that conditionally skips one assignment
inherits the previous CR's value (spark-k8s-operator shipped exactly that). Handler fields remain
correct for reconcile-**invariant** configuration; `sidecar.SidecarManager.CloneForBuild` covers the
framework's own instance of the same hazard, a handler-registered manager whose configs
`SetProductImage` writes into. See `docs/architecture.md` §4.1.4.

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

**Those two jobs are separately addressable, and that is the seam for a product-owned config file.**
The reconciler reads the producer list off the **outer** handler — `r.roleGroupHandler`, through the
`LoggingProducerProvider` assertion — while the config file is rendered from the embedded
`BaseRoleGroupHandler`'s own `LoggingContainers`. Go has no virtual dispatch, so a base handler can
never observe an override; a product that **overrides `LoggingProducers` and leaves
`LoggingContainers` empty** therefore joins the Vector pipeline (shared volume, RW mount, log
directory, source) with no framework-rendered file and no ConfigMap key to collide with its own.
Airflow's `log_config.py` — which must be built on Airflow's own `DEFAULT_LOGGING_CONFIG` and so can
never be a rendered template — is the case this exists for. The product picks up the two obligations
the framework can then no longer meet: the declaration's `Container` must name a real container in
the assembled pod (enforced; see §9's producer validation), and its log file must land at
`productlogging.LogDirFor(decl)` carrying the framework's `LogFileSuffix`, or the pipeline comes up
and collects nothing. No new API is involved — overriding the method **is** the API.

**A level a role group does not state is inherited, never cleared.** `LogLevelSpec.Level` carries no
`+kubebuilder:default` — it sits inside the folded `config` block, where structural defaulting would fill
it as soon as its enclosing object existed, so `console: {}` in a role group would arrive as
`console: {level: INFO}` and beat the role's `DEBUG`. `MergeLoggingSpec` applies the same rule to
`console`, `file` and every entry of `loggers`: a level-less entry keeps the role's value, and only a
stated level overrides. The effective default (root logger `INFO`, no appender threshold) is applied by
the renderers at consumption time.

Default config file names come from each generator's `DefaultFileName()`: `logback.xml`, `log4j.properties`, `log4j2.properties` and — deliberately **not** `logging.py` — `log_config.py`, since a config directory on `sys.path` would otherwise shadow the standard library's `logging` module. `ContainerLogging.FileName` overrides the name per container. The rolling *log* file the Vector sources glob is separate and framework-owned: `<KubedoopLogDir>/<lowercased container>/<container><suffix>`, with `.log4j.xml` (log4j/logback), `.log4j2.xml` (log4j2) or `.py.json` (python) selecting the Vector edge parser; `ContainerLogging.LogFileName` may rename it only if the suffix survives and it stays a bare file name.

**The directory segment is the Vector event's `container` tag, and `LogDirName` decouples it from the pod container name.** The collector extracts that field from the path segment and from nothing else (`parse_regex!(.file, r'^<LogDir>(?P<container>.*?)/(?P<file>.*?)$')`), so one string used to name the pod container *and* identify the log stream — and a product whose container name is pinned by one contract could not keep a different, equally pinned log tag. `ContainerLogging.LogDirName` overrides the segment; empty keeps the default byte for byte. It moves **only** the directory and the tag: the shared log volume is still mounted on the container named by `Container`, per-container logging is still configured under `logging.containers.<Container>`, and the default log-file base name still follows `Container` — which is what keeps two producers sharing a directory from resolving to the same file. `productlogging.LogDirFor(decl)` is the single implementation; a product composing its own log path (a stdout redirect, a hand-written config) must call it rather than hardcode the directory.

An explicit `LogDirName` must be a single lowercase RFC 1123 label. It lands unquoted in the Vector container's `mkdir -p … && exec vector …` command — until now the value was always a pod container name the API server had already constrained — and as one path segment that an embedded `/` would silently truncate the tag at, `.`/`..` would escape, and a space would turn into two `mkdir` arguments against a read-only rootfs. **A producer naming no container in the assembled pod is now a build failure too**: it used to be skipped in silence, producing a pod whose log directory exists and whose config points into it with nothing mounting the volume — the appender writes into the container filesystem, Vector collects nothing, and every signal reports healthy.

### 9b. Image Conventions the Framework Requires

Two conventions the kubedoop images define, that the framework's own behaviour depends on, and that
every product previously re-typed as string literals.

**The JMX exporter runs as a java agent.** `constant.KubedoopJmxAgentJar` is the unversioned symlink
the images provide, and `constant.JMXJavaAgentOpt(port, configFile)` renders the JVM option:

```go
constant.JMXJavaAgentOpt(8081, "config.yaml")
// -javaagent:/kubedoop/jmx/jmx_prometheus_javaagent.jar=8081:/kubedoop/jmx/config.yaml
```

The config file is a **parameter**, not a constant: the hadoop image ships no `config.yaml`, only
`namenode.yaml` / `datanode.yaml` / `journalnode.yaml`, because the metrics worth exporting differ
per role. This is a different mechanism from `pkg/sidecar/jmx_exporter.go`, which runs
`jmx_prometheus_httpserver.jar` from `/opt/jmx_exporter` as a separate container — a path no
kubedoop image contains.

**The config mount is read-only.** The generated ConfigMap is mounted read-only at
`BaseRoleGroupHandler.ConfigMountPath` (default `constant.KubedoopConfigDirMount`), so a product
whose start-up rewrites a config file must copy it to a writable directory first:

```sh
mkdir -p /kubedoop/config/
cp -RL /kubedoop/mount/config/* /kubedoop/config/
```

`-L` is load-bearing: a ConfigMap volume is a farm of symlinks into a hidden `..data/` directory, so
a copy that preserves them leaves dangling links. The SDK ships no helper for this — the existing
call sites disagree on flags and the mount path is configurable — but the requirement is now stated
in `docs/architecture.md` §4.1.5 rather than discoverable only by reading a sibling operator.

### 10. Product Config (`ProductConfig`)
Products contribute their computed configuration **as data through the same merge pipeline as CRD overrides**, instead of imperatively constructing resources. Set the optional `ProductConfig` field on `GenericReconcilerConfig` — a pure function returning an `*v1alpha1.OverridesSpec` (the same shape users write in the CRD):

```go
reconcilerCfg := &reconciler.GenericReconcilerConfig[*v1alpha1.TrinoCluster]{
    // ...
    ProductConfig: func(ctx context.Context, c client.Client, cr *v1alpha1.TrinoCluster,
        roleName, roleGroupName string) (*commonsv1alpha1.OverridesSpec, error) {
        overrides := map[string]map[string]string{
            "config.properties": {"http-server.http.port": "8080"},
        }
        if roleName == "coordinators" {
            overrides["config.properties"]["coordinator"] = "true"
        }
        return &commonsv1alpha1.OverridesSpec{ConfigOverrides: overrides}, nil
    },
}
```

**The `ctx` and client are what make "may derive from live cluster state" true.** Without them the
hook could only be a pure function of the CR, so a product needing an API lookup — resolving an
`S3Connection` reference to an endpoint — could not use it at all, and a `Get` failure could only be
swallowed (rendering a silently wrong config) or panicked. A returned error fails the role group.

**`RoleGroupBuildContext.ApplyProductDefaults(*OverridesSpec)` is the imperative counterpart**, for a
product that performs its lookup inside `BuildResources` and does not want to repeat it:

```go
conn, err := s3.ResolveConnection(ctx, c, cr.Namespace, inline, ref)
if err != nil { return nil, err }
buildCtx.ApplyProductDefaults(&commonsv1alpha1.OverridesSpec{
    ConfigOverrides: map[string]map[string]string{"hive-site.xml": conn.S3AProperties()},
})
```

It folds the layer **beneath** everything already merged, using the merge's own per-dimension rules
(`config.ConfigMerger.MergeBeneath`): config files and env vars per key, CLI/JVM args as a whole,
podOverrides through the same strategic merge patch. Repeated calls accumulate, each landing beneath
the last. hive-operator and spark-k8s-operator each hand-wrote the same "set only keys the user did
not set" helper, and both then discovered the same second rule for env vars — that product defaults
must not overwrite `envOverrides`, which they solved by prepending to the container's env list. Both
rules are the framework's own precedence, and neither needs an ordering dance here because
`MergedConfig.EnvVars` is a map.

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

### 11b. Generate-Once Secrets

Some objects must **not** converge. `reconciler.EnsureGeneratedSecret` is the counterpart to
`EnsureDiscoveryConfigMap` for those: it creates the Secret with generated values if absent, fills
in only **missing** keys if it exists, and **never rewrites an existing value**.

```go
_, err := reconciler.EnsureGeneratedSecret(ctx, c, scheme, cr, cr.GetName()+"-oauth2-cookie",
    map[string]func() (string, error){"cookie-secret": sidecar.GenerateCookieSecret},
    reconciler.WithGeneratedSecretProductName("trino"),
)
```

The oauth2-proxy session cookie key is the shipped case: `GenerateCookieSecret`'s doc says to call
it once and store the result, because a fresh value every pass rolls the pods and logs every user
out — so `RoleGroupResources.ExtraResources`, whose apply path is idempotent `CreateOrUpdate`
against a desired object, cannot serve. `OAuth2ProxySidecarProvider.Validate` fails the reconcile
when the key is missing, so the framework *requires* such a Secret.

Filling a missing key is deliberate: a Secret that lost one key (a partial restore, a hand-edit)
would otherwise wedge the cluster with no recovery short of deleting the whole Secret, which rotates
every *other* key too. Generators run only for absent keys, so the steady-state path invokes none of
them. Call it from a `common.ClusterExtension` `PreReconcile` hook, where a ctx and client exist and
the workload has not been built yet; it is deliberately **not** created from the sidecar provider's
`Validate`, a step whose job is to have no side effects.

### 11c. Workload Identity and Workload RBAC

Two halves of one thing, both settled per CR at the top of the reconcile (steps 0 and 0b), before
any extension hook or role runs. **Neither is the operator's own RBAC** — that comes from the
operator's ClusterRole and is a separate axis entirely.

**The identity is derived and unconditional.** Every cluster gets a ServiceAccount named
`ServiceAccountResourceName(kind, cluster)` — `"<lowercased kind>-<cluster>"`, e.g.
`hdfscluster-prod` — in the CR's namespace, with the CR as controller owner. There is no config
field for the name and no way to skip it. The reconciler propagates it through
`RoleGroupBuildContext.ServiceAccountName`, which is therefore **never empty**, and
`BaseRoleGroupHandler` binds it to the pod template.

The Kind is in the name because a CR name alone is not unique in a namespace: an `HdfsCluster` and a
`TrinoCluster` both called `prod` would otherwise select one ServiceAccount and the second
controller could never own it. The name is derived rather than configured for the same reason every
other framework-owned name is (§3): the framework creates the object, controller-owns it,
garbage-collects it with the CR and binds the workload's Role to it, so nothing needs to address it
by a product-chosen name. Configuring it is what produced the failure class this replaced — a static
name was a *constant*, so every CR of a product in one namespace resolved to the SAME
ServiceAccount: whichever cluster reconciled second failed with `AlreadyOwnedError` forever, and
deleting the first garbage-collected the SA out from under the second's running pods.
`ServiceAccountResourceName` is **exported** because anything outside the operator that must name the
SA — a hand-written RoleBinding, an admission policy, an audit query — should derive it from the
formula. An object squatting on the derived name is never adopted; the reconcile fails naming both
owners.

**The permissions are declared by the product.** `GenericReconcilerConfig.WorkloadRBACRules func(cr
CR) []rbacv1.PolicyRule` says what this cluster's *pods* call; the framework maintains a namespaced
`Role` + `RoleBinding` named after that same derived ServiceAccount, controller-owned by the CR:

```go
WorkloadRBACRules: func(cr *v1alpha1.NifiCluster) []rbacv1.PolicyRule {
    return []rbacv1.PolicyRule{{
        APIGroups: []string{"coordination.k8s.io"},
        Resources: []string{"leases"},
        Verbs:     []string{"get", "list", "watch", "create", "update"},
    }}
},
```

The framework passes the name it derived itself, so identity and permissions cannot drift apart —
that is why this is a config field and not only the exported `reconciler.EnsureWorkloadRBAC` helper
(which takes the name as a parameter, so a caller *can* pass one no pod uses: both objects exist, the
pods start, and the first API call 403s). The helper stays exported for a product driving the
reconciler's pieces itself; `WithWorkloadRBACProductName` and `WithWorkloadRBACExtraLabels` are its
options, and canonical labels always win over extras.

**There is no `ClusterRole` path.** A namespaced CR cannot controller-own a cluster-scoped object, so
the framework would have no lifecycle for one — no GC with the cluster, no ownership gate on the
reclaim. A product needing cluster-scoped permissions maintains those objects itself, cleanup
included.

**An empty rule set REVOKES**, deleting both objects when this CR controller-owns them — the same
reading every optional slot gets (a nil `MetricsService` reclaims the Service), and the only one that
makes a rule set shrinking to nothing actually stop granting. A **nil hook** is the different
statement "this product never opted in", and touches no RBAC object at all: running a revoke every
pass would cost two Gets per cluster and demand RBAC read permission from operators that never asked
for the feature.

**A pre-existing RoleBinding is never adopted.** `roleRef` is immutable, so one already at this name
pointing elsewhere cannot be converged; the framework fails with a `*ValidationError` naming both
refs and the fixing command, rather than rewriting only the subject — which would hand these pods
whatever the old ref allows and report success.

**Setting the hook obliges the operator twice over.** Kubernetes forbids granting permissions the
granter lacks, so the operator's ClusterRole must be a superset of every rule passed — *and* it needs
write access to the RBAC API itself. Both are kubebuilder markers, and a 403 has those two distinct
causes needing opposite fixes, so the SDK re-explains the API server's message rather than
pre-checking (that message already computed rule covering against the operator's real effective
permissions and names the missing rule). The `Owns(&rbacv1.Role{})`/`Owns(&rbacv1.RoleBinding{})`
watches are registered **only** when the hook is set: an unconditional one would force every operator
on this SDK to grant itself cluster-wide `list;watch` on RBAC, and a forbidden informer fails
`WaitForCacheSync` for *all* sources, so `manager.Start` returns and the process exits.

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

`ExtraOwns` does double duty: `SetupWithManagerOpts` also hands it to the orphan cleaner, so the
kinds a product declares for watches are the kinds reclaimed when a role group leaves the spec.
Deriving one from the other is what stops them drifting — a product adding an extra kind has to
register it for watches anyway. Only objects carrying the departing role group's identity labels
**and** this CR's controller owner reference are deleted, so a kind listed here that is not
per-role-group is simply never matched.

`ControllerBuilder(mgr, opts)` returns the configured `*builder.Builder` for operators that need to
add `WithOptions`, predicates, or anything else before calling `Complete`. A product that builds its
own controller this way does **not** get extras cleanup — the wiring lives in `SetupWithManagerOpts`
— and can call `RoleGroupCleaner.WithExtraResourceKinds` itself if it needs it.

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
2. the **outer** handler's `LoggingProducers(roleName)` returns at least one producer — an agent with
   nothing to collect would mount an empty pipeline. Read from `r.roleGroupHandler`, so a product's
   override counts even when `BaseRoleGroupHandler.LoggingContainers` is empty (§9);
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

**Every sidecar is injected as a native sidecar — a `RestartPolicy: Always` init container**
(`sidecar.SidecarRestartPolicy()`; KEP-753 — on by default since Kubernetes v1.29, GA in v1.33). The kubelet starts those
before the main container and terminates them **after** it, which is what guarantees a log agent
outlives the process it collects from. That ordering used to be hand-rolled: before #441 the Vector
container blocked on `inotifywait` for a shutdown file the product's main container was expected to
`touch` on exit, and both halves of that contract — the watcher in `pkg/builder/vector.go` and the
writer helpers in `pkg/util/bash.go` — were **deleted in the same commit**. A product migrating from
a pre-#441 operator should therefore *remove* its shutdown-file commands rather than look for a
framework helper to emit them: nothing reads that file, and the old mechanism was strictly worse in
one case, firing whenever the main process exited — including a crash the kubelet was about to
restart, which told the agent to shut down.

**`sidecar.NewStaticContainerProvider(container)` injects a container the product built itself**,
unchanged — the escape hatch for a sidecar the framework has no opinion about (a statsd-exporter, a
product-specific helper), and the reason the framework does not grow a provider per such container.
Its `Inject` ignores `SidecarConfig` entirely, so **the product owns everything a normal provider
would get for free**: `SetProductImage` does not fill in its image (an image-less container fails the
build), `DefaultSecurityContext()` is not applied, and `ApplyProbes` does not run. Set
`RestartPolicy: sidecar.SidecarRestartPolicy()` on the container so it is a native sidecar like the
rest — the provider does not add it.

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
`RestartPolicy: Always` (native sidecars — on by default since Kubernetes v1.29, GA in v1.33), and on such a container
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
- **`testutil.HaveNoInheritedConfigDefaults()` is the exported guard against the #544 defect class**,
  and product operators are expected to run it over their own generated CRDs:

  ```go
  It("declares no CRD default inside a role config block", func() {
      Expect("config/crd/bases/*.yaml").To(testutil.HaveNoInheritedConfigDefaults())
  })
  ```

  It statically scans CRD YAML — no envtest, no CR fixture, no cluster — and reports every
  `default` under a role or role group `config` block, at any depth. `FindInheritedConfigDefaults`
  is the same check returning `[]InheritedConfigDefault` (CRD, version, JSON path, default, file).
  Roles are found structurally (a schema node declaring `roleGroups`), so a product that flattens
  its roles into `spec.coordinators` is covered as well as the generic `spec.roles[*]` map.
  Arguments matching no files are an **error**, not a pass. This repository runs it over
  `config/crd/bases/*.yaml` and `examples/trino-operator` over its own.
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
