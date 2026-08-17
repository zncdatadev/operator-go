# AGENTS.md

## Project Overview
`operator-go` is a Golang SDK/framework for building Kubernetes operators. It provides a reusable reconciliation framework, CRDs, and utilities for creating product-specific operators.

**Key Features:**
- **GenericReconciler**: Template Method Pattern-based reconciliation framework
- **Extension System**: Hook-based customization at cluster/role/role-group levels, with per-product registries
- **Resource Builders**: Fluent builders for StatefulSet, Service, ConfigMap, PDB, RBAC, ServiceAccount
- **Role Declaration**: `RoleProvider`/`RoleCatalog` — a product states each role as data, once per
  reconcile, with the CR in hand
- **Config Folding**: `FoldCommonConfig` / `FoldProductConfig` — the framework folds its half of the
  `config` block, the product folds its own, both under one rule set
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
   `ServiceAccountResourceName`) and, when `WorkloadRBACRules` is set, its Role/RoleBinding (§11c)
5. PreReconcile Extensions (Hook)
6. Validate declared dependencies (`GenericReconcilerConfig.Dependencies`)
6b. `RoleProvider.DeclareRoles` — the product's role catalog for this CR, obtained **once** and
   validated against `spec.roles` before any role is reconciled (§3). A catalog error fails the
   pass. The catalog is threaded down the call chain rather than stored on the reconciler: one
   reconciler instance serves every cluster, so a field would leak one CR's declarations into
   another's pass
7. For Each Role (**best effort** — see below):
   - Role PreReconcile Extensions
   - For Each RoleGroup:
     - RoleGroup PreReconcile Extensions
     - Build RoleGroupBuildContext: **fold** the typed config (`FoldCommonConfig`, §4b) → resolve
       the image and the Vector gates → **derive** (`RoleGroupResolver`, §10) → **merge** the
       overrides
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

**Inside a claim template, a value only the server filled in is not a change request.** The live
template comes back carrying `spec.volumeMode: Filesystem` and a `status` block a handler-built one
has no way to state, so a whole-slice comparison was true on *every* pass: each role group with a
data PVC emitted `ImmutableFieldIgnored` forever while its StatefulSet's `generation` stayed at 1 and
no pod rolled (#627). That is not merely noisy — it is the same warning a genuine resize produces, so
the event stopped distinguishing "your resize was dropped" from the background. Those two fields are
therefore compared only when the handler states one; everything else — capacity, access modes, the
claim's name, `storageClassName` — still counts, and `storageClassName` in particular is **not**
defaulted into the template by the API server, so changing it is still reported.

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

### 3. RoleProvider, RoleDeclaration and RoleGroupHandler

**A product declares its roles as DATA, once per reconcile, with the CR in hand.**
`RoleProvider` is that seam:

```go
type RoleProvider[CR common.ClusterInterface] interface {
    DeclareRoles(ctx context.Context, c client.Client, cr CR) (RoleCatalog, error)
}

type RoleCatalog map[string]RoleDeclaration
```

Set it as `GenericReconcilerConfig.RoleProvider`. It is called **once per pass**, after the
dependency check and before any role is reconciled — so a product resolving a cluster-wide fact (an
authentication class that decides whether every role speaks TLS, an S3 connection) pays for that
lookup once rather than N times for N role groups. Returning a `*common.RequeueAfterError` reports
"not ready yet" without the cluster going `Degraded` (§5); any other error fails the pass.
`RoleProviderFunc` adapts a plain function.

`RoleDeclaration` is everything the product knows about **one** role:

```go
return reconciler.RoleCatalog{
    "coordinator": {
        MainContainerName: "trino",
        ContainerPorts:    []corev1.ContainerPort{{Name: "http", ContainerPort: port}},
        ServicePorts:      []corev1.ServicePort{{Name: "http", Port: port}},
        Command:           []string{"/bin/bash", "-c", "launcher run"},
        LogProducers:      []productlogging.ContainerLogging{{Container: "trino", Framework: productlogging.LoggingFrameworkLogback}},
        ConfigDefaults:    &commonsv1alpha1.RoleGroupConfigSpec{Affinity: aff},
    },
    "worker": { /* … */ },
}
```

Full field set: `Image`, `ContainerPorts`, `ServicePorts`, `MainContainerName`, `Command`,
`Lifecycle`, `ReadinessProbe`/`LivenessProbe`/`StartupProbe`, `DataVolume`, `ListenerClass`,
`PublishNotReadyAddresses`, `LogProducers`, `OwnsVectorConfig`, `LogVolumeSize`, `Env`,
`ConfigDefaults`, `Optional`.

**Almost none of it has a user layer above it**, and that is the point. A role's ports, its primary
container's name and command, whether it has a data PVC, which of its containers produce logs — the
CRD offers the user no way to state any of these, so nothing merges and nothing is beaten. This
replaced seven role-keyed maps on the handler, their handler-global fallbacks, and four per-call
fields on the build context that outranked them: the declaration used to be shredded across three
objects each with its own precedence rule, and the rule for one of them **dropped sibling fields**,
which is the defect (#631) that started this.

Two fields are the exceptions, in opposite directions:

- **`ConfigDefaults`** is folded BENEATH the CR's role and role group levels by `FoldCommonConfig`
  (§4b). Anything the user states anywhere wins. An anti-affinity default belongs here and could
  not live on the old handler at all, because its selector names the **cluster** while the handler
  is a process-wide singleton shared by every cluster the operator serves.
- **`Env`** is emitted beneath the merged overrides. Kubernetes resolves a duplicate env name to
  the last entry, so a user's `envOverrides` of the same name wins. It exists because the merged
  channel is `map[string]string` and cannot carry a `valueFrom` at all; a value the product
  *computes* belongs in `Contribution.EnvVars` (§10) instead.

**Being produced per reconcile is what retires the per-call escape hatches.** A port that moves
because the CR enabled TLS is computed *here*, from *this* CR, rather than assigned into
process-wide handler state that the next cluster inherits.

**The catalog is validated once per pass, asymmetrically.** A role the CR declares that the catalog
does not is a **hard error** naming the typo and listing what it could have meant — before, an
unrecognised name silently produced a workload with no ports, no image and no Service while the
reconcile reported success. A role the catalog declares that the CR does not use is a **Warning
event** (`UnusedRoleDeclaration`), since a product may support more roles than a given cluster runs;
marking it `Optional: true` silences even that. `reconciler.ValidateCatalog` is the exported check.

**Images are resolved once per role, by the framework.** `GenericReconcilerConfig.ImageResolution`
carries the product name and the operator's defaults:

```go
ImageResolution: reconciler.ImageResolution{
    ProductName: "trino",                       // app.kubernetes.io/name AND the repo path segment
    Defaults: commonsv1alpha1.ImageSpec{
        Repo:            "quay.io/zncdatadev",
        ProductVersion:  "476",
        KubedoopVersion: version.BuildVersion,   // the operator's own build version
    },
},
```

The layers fold per field — CR `spec.image` first, then `RoleDeclaration.Image`, then `Defaults` —
so a CR stating only `productVersion` still yields a valid `…:476-kubedoop0.2.0` reference. The
result reaches the handler as `RoleGroupBuildContext.ResolvedImage` (`Reference`, `PullPolicy`,
`PullSecretName`, `ProductVersion`). `RoleDeclaration.Image` sits **under** the CR deliberately: a
product pinning one role to a different image must not silently beat a user who pinned
`spec.image.custom` to an air-gapped mirror.

`Defaults` is read **every reconcile**, which is what a webhook cannot do: webhook defaults are
persisted at admission and never recomputed, freezing `kubedoopVersion` at whatever operator version
first admitted the CR (§10 and `docs/architecture.md` §2.6).

An unresolvable `spec.image` **fails the role group**, naming the missing field, instead of silently
falling back to a static image and running a version nobody asked for. With `ProductName` empty the
framework resolves nothing from the CR beyond `spec.image.custom` — the shape a product uses when it
resolves images itself — and that path never errors. `app.kubernetes.io/version` follows the
**resolved** version, so it is present whenever the version came from `Defaults` too.

**The assembled tag is validated, and an unusable one fails the role group.** `productVersion` and
`kubedoopVersion` both land in the image tag, whose grammar is `[A-Za-z0-9_][A-Za-z0-9._-]{0,127}`,
and nothing downstream would catch a value that breaks it: the API server does not validate
`container.image` at all, so an unparsable reference is accepted, stored, and surfaces only as
`InvalidImageName` on a pod while the reconcile reports success. The case this exists for is
`KubedoopVersion: version.BuildVersion` with the scaffold's dev default of `"N/A"` — the `/` makes
`…:476-kubedoopN/A` unparsable, so **every development build on the structured image path produced
pods that could never start**. The error names the offending field and says the value may have come
from the operator's own defaults rather than from the CR. `spec.image.custom` is deliberately **not**
validated: it is the user's verbatim reference, so a wrong one is their own visible mistake.

**`spec.image.pullSecretName` names a docker-registry Secret added to every pod's
`imagePullSecrets`.** It folds per field like the rest and is resolved **independently of the
image**: a pull secret is a property of where the image lives, so it applies on all three paths —
the assembled reference, `custom`, and a product that resolves its own images with `ProductName`
empty. Ten product CRDs already declared this field, and migrating to the commons `ImageSpec` used
to delete the behaviour silently — the CRD still accepted the value and no pod ever carried the
entry, so a private-registry install failed with `ImagePullBackOff` and nothing naming the cause.
One name rather than a list, matching those CRDs; it is applied before `podOverrides`, and strategic
merge patch keys `imagePullSecrets` by `name`, so an override **adds** a credential rather than
replacing this one.

#### 3b. BaseRoleGroupHandler

`RoleGroupHandler` is what turns the declaration and the folded config into objects:
```go
type RoleGroupHandler[CR common.ClusterInterface] interface {
    BuildResources(ctx context.Context, k8sClient client.Client, cr CR, buildCtx *RoleGroupBuildContext) (*RoleGroupResources, error)
}
```

`BaseRoleGroupHandler.BuildResources` returns a ConfigMap, a headless Service, a StatefulSet, and a
client-facing Service **when the role declares service ports**. Construct it with the scheme alone —
everything else now arrives per reconcile:
```go
handler := reconciler.NewBaseRoleGroupHandler[*v1alpha1.TrinoCluster](scheme)
```
The handler carries only reconcile-**invariant** settings a role cannot differ on:
`ConfigGenerator`, `Scheme`, `ConfigMountPath`, `LabelDomain`, the sidecar manager and the security
contexts. Everything role-shaped moved to `RoleDeclaration`; everything CR-shaped moved to
`RoleProvider`/`RoleGroupResolver`. That is what makes one handler instance safe for every cluster
the operator reconciles — the older idiom of assigning `h.Image` or calling `h.SetRoleContainerPorts`
inside `BuildResources` wrote per-cluster values into process-wide state, which races above
`MaxConcurrentReconciles: 1` and leaks even at 1 (spark-k8s-operator shipped exactly that).

It does not return a PDB: the framework's PDB comes from `roleConfig.podDisruptionBudget` and is a
**role-level** resource built by `BuildRolePodDisruptionBudget` and applied once per role by the
reconciler (`RoleGroupResources.PodDisruptionBudget` remains an escape hatch for an extra per-group
PDB). `BuildRolePodDisruptionBudget` takes a single `*RoleBuildContext` — the role-scoped analogue
of `RoleGroupBuildContext`, carrying `ClusterName`, `ClusterNamespace`, `ClusterLabels`,
`ClusterSpec`, `RoleName`, `RoleSpec`, `ProductName` and `ProductVersion` — so a later role-level
input needs no new signature.

When building the StatefulSet, `BaseRoleGroupHandler` consumes the role group's folded `config`
(commons `RoleGroupConfigSpec`): `resources` (requests/limits, plus the data PVC when the role
declares a `DataVolume`; `storage.storageClass` is a `*string` because Kubernetes reads
`storageClassName: ""` as "bind only a pre-provisioned PV, never dynamically provision one" — a role
group that set it to `""` to mean "inherit the role's" would get a PVC that stays `Pending`
forever), `affinity` (see below), and `gracefulShutdownTimeout` (a Go duration mapped to
`terminationGracePeriodSeconds` — unparsable or non-positive values fail the build). All of these
are applied before `podOverrides`, so user pod overrides keep precedence. The framework sets
affinity only when the config provides one, so products that post-process the built StatefulSet with
`if podSpec.Affinity == nil {...}` default guards remain correct.

**`config.affinity` is decoded STRICTLY, and is replaced wholesale.** The CRD carries it as a
schema-free `RawExtension` (`type: object` + `x-kubernetes-preserve-unknown-fields`), so the API
server neither validates nor prunes it. `reconciler.DecodeAffinity` therefore decodes with
`DisallowUnknownFields` and an unknown field **fails the build**, naming it. Before that, `nodeAffinty`
(one letter short) passed admission, decoded into an empty `corev1.Affinity`, and the pods were
scheduled anywhere — with no event, no log line and no status change, even though affinity is the
scheduling *contract* for these products (rack awareness, spreading a quorum, colocating a worker
with its data). The trade-off is deliberate: a field from a newer Kubernetes API than the SDK is
built against is now rejected rather than ignored, which is the honest answer, since the framework
cannot honor a field it does not know.

`config.affinity` is **replaced wholesale** by any layer that states one — the rule Kubernetes uses
for `PodSpec.affinity` — and an empty value clears. What a replacement discarded is reported as an
`AffinityOverridden` Warning event. See §4b for why the Kubernetes rule is kept and what it costs.

**The affinity helpers are composable terms, not one canned policy.**
`PreferredAffinityTerm(weight, topologyKey, selector)` builds one weighted term;
`ClusterSelectorLabels(cluster)` and `RoleSelectorLabels(cluster, role)` are the selectors, built
from the framework's own identity labels (`app.kubernetes.io/instance` +
`app.kubernetes.io/component`) — the part worth centralising, since a downstream operator re-typing
those keys as string literals gets a selector matching nothing the moment they change, and a
preferred term matching no pod is not an error. `EncodeAffinity` is `DecodeAffinity`'s inverse, so a
product supplies a default with the typed Kubernetes API rather than a JSON literal:

```go
aff, err := reconciler.EncodeAffinity(&corev1.Affinity{
    // spread this role's pods across nodes
    PodAntiAffinity: &corev1.PodAntiAffinity{PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
        reconciler.PreferredAffinityTerm(70, reconciler.TopologyKeyHostname,
            reconciler.RoleSelectorLabels(cr.GetName(), "datanode")),
    }},
    // …while keeping the cluster together
    PodAffinity: &corev1.PodAffinity{PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
        reconciler.PreferredAffinityTerm(20, reconciler.TopologyKeyHostname,
            reconciler.ClusterSelectorLabels(cr.GetName())),
    }},
})
if err != nil { return nil, err }
decl.ConfigDefaults = &commonsv1alpha1.RoleGroupConfigSpec{Affinity: aff}
```

This replaces a single-shot `DefaultAntiAffinity` helper that emitted exactly **one** anti-affinity
term — which is why hdfs-operator, the product with the most roles, could not use it at all: its
default is composite (a cluster-level pod *affinity* at weight 20 beside a role-level
*anti*-affinity at weight 70). Both operators that needed a composite hand-wrote it, and both got it
wrong — one commented the merge out entirely, the other applied its default unconditionally and
silently discarded the user's own `config.affinity`.

There is deliberately **no Required constructor**: a required spread turns a too-small cluster into
pods that never schedule (three nodes, five replicas, two `Pending` forever), with no way to say
"spread as far as you can". A product that genuinely requires it writes the `corev1` term itself.
`NormalizeAffinity` is exported so a hand-assembled affinity produces the same bytes the fold does —
without it a member cleared with `{}` reaches the pod template as an empty struct, which differs
from absent in the serialized spec and shows up as a diff on every reconcile.

**`logging` IS a supported config default now**, folded through `productlogging.MergeLoggingSpec`
like every other field in the block. It used to be rejected with a `*ValidationError`, and the
rejection was correct for the code as it stood: the framework merged logging in the reconciler from
the CR's two levels only, and had already decided Vector enablement and rendered the config file
before a default was ever read — so one set here would have applied to neither. Moving the fold
ahead of both consumers is what made the field honourable, and the reconciler now reads Vector
enablement off the **folded** value. This is issue #631's item 2: the old godoc advertised `logging`
while the code hard-rejected it, and the fix was to make the code match the doc rather than the
other way round.

**A data PVC is per role, declared not configured.** `RoleDeclaration.DataVolume{Name, MountPath}`
opts the role in; nil means the role has none. That is a structural property of the role rather than
a consequence of what the user wrote — a product with one stateful and one stateless role could
previously only choose between "every role" and "none", so one migration deleted a
`volumeClaimTemplate` its predecessor rendered. `Name` defaults to `builder.DefaultDataVolumeName`
(`"data"`); the **size** comes from the folded `config.resources.storage`, so the user still sets it.

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

**The slice type stays open; the entries are validated.** Arbitrary GVKs are the point, so nothing
narrower than `client.Object` can be the type — but three properties are checked in the same
pre-apply gate the fixed slots use, each failing the role group with a `*ValidationError` naming the
index: every entry has a **name** (`CreateOrUpdate` addresses the object by name every reconcile, so
`generateName` would create a new object per pass instead of converging one), every entry sits in the
**cluster's namespace** (which also rejects a cluster-scoped object — Kubernetes honours no owner
reference from a namespaced CR to one, so the framework has no lifecycle to give it), and no two
entries — nor an entry and a fixed slot — address the **same object**. The first two only move a
failure that already existed inside `applyResource` earlier, to where nothing is half-applied yet.
The third catches what failed nowhere at all: two writers for one object in one pass mean the later
apply silently discards the earlier, and if their desired states differ the object is rewritten every
reconcile, each write waking the framework's own watch with nothing to back the loop off.

**A `batchv1.Job` in that slice is create-once, and that is a typed rule rather than the generic
copy.** The fallback assigns `spec` wholesale, and the API server *generates* `spec.selector` and
injects four UID-derived labels into `spec.template` at creation — neither of which a handler-built
desired object can carry. So the **second** reconcile of an unchanged Job was rejected
(`spec.selector: Required value`, `spec.template: field is immutable`) and the role group went
permanently `Degraded` quoting a field the user never wrote. `copyJobState` preserves the live
`selector`, `template`, `completions`, `completionMode` and `manualSelector`, and lets `parallelism`,
`suspend`, `backoffLimit`, `activeDeadlineSeconds` and `ttlSecondsAfterFinished` converge — the knobs
batch deliberately left mutable. Create-once is also the only semantics a Job *has*: its work is a
side effect that already happened, so a product needing it re-run changes the **name**, and a
differing template is reported through `ImmutableFieldIgnored` like any other preserved field.

### 4. RoleGroupBuildContext
Role and role group configuration reaches a handler through one struct, built per role group by the
reconciler and passed to `BuildResources`. There is **no role-level interface a product implements**:
`pkg/common/role_interface.go` (`RoleInterface`, `RoleInfo`, `RoleGroupInfo`) does not exist — the
reconciler iterates `spec.Roles` directly.

`RoleGroupBuildContext` carries `ClusterName`, `ClusterNamespace`, `ClusterLabels`, `ClusterSpec`,
`RoleName`, `RoleSpec`, `RoleGroupName`, `RoleGroupSpec`, `MergedConfig` (the folded
product-contribution/role/role-group overrides), `ResourceName` (`{cluster}-{role}-{group}`,
truncated with a hash suffix by `RoleGroupResourceName`), `ServiceAccountName` (the SA the reconciler
derived and ensured — `ServiceAccountResourceName(kind, cluster)`, never configured and never empty),
`SidecarManager`, `VolumeProviders` (see §16) and `VectorAggregatorAddress`.

**Three fields are the framework's already-settled answers, written before `BuildResources` runs:**

| field | what it is |
|---|---|
| `Declaration` | the `RoleDeclaration` this role group's product returned (§3) |
| `ResolvedImage` | `Reference`, `PullPolicy`, `PullSecretName`, `ProductVersion` — resolved once so the container and the sidecars cannot be told different things |
| `ProductName` | `ImageResolution.ProductName`, the `app.kubernetes.io/name` value |

Assigning `Declaration` from inside `BuildResources` is a **half-honoured** change and therefore
worse than one wholly ignored: the image, the config fold and the Vector gates are already settled
from it, while ports, container name, command, probes, data volume, listener class and log producers
are read during the build and *would* take effect. Declare the role in `RoleProvider`, where every
consumer sees the same answer.

**The per-call escape hatches are gone.** `Image`, `ImagePullPolicy`, `ContainerPorts`,
`ServicePorts` and `MainContainerCustomizer` on the build context no longer exist. Each was a
channel that outranked the handler's own state, and together they made the role's definition a
three-object precedence puzzle. A value that depends on the CR is computed in `DeclareRoles`, which
receives the CR:

```go
func (h *MyHandler) DeclareRoles(ctx context.Context, c client.Client, cr *MyCluster) (reconciler.RoleCatalog, error) {
    port := int32(8080)
    if cr.Spec.Tls != nil { port = 8443 }
    return reconciler.RoleCatalog{"server": {
        ContainerPorts: []corev1.ContainerPort{{Name: "http", ContainerPort: port}},
        Command:        []string{"/bin/zkServer.sh"},
    }}, nil
}
```

`MainContainerCustomizer` in particular is not replaced by a hook. `Command`, `Lifecycle` and the
three probes are declaration fields, applied to the primary container by identity — nobody indexes
`Containers[0]`, an assumption a sidecar provider inserting a container earlier quietly breaks — and
applied **before** `podOverrides` are strategic-merged, which is why they cannot be a post-build
patch: a product editing the returned StatefulSet lands *after* the merge and silently beats the
user. **Changing the image this way is impossible by construction now**, rather than rejected by a
guard: the image is resolved once, propagated to the sidecars, and published read-only as
`ResolvedImage`.

`listener.ServiceTypeFor` is the shared class→type mapping restored from v0.12.6:
`cluster-internal` → ClusterIP, `external-unstable` → **NodePort**, `external-stable` →
LoadBalancer, anything else → ClusterIP. It lives in `pkg/listener` rather than `pkg/builder`
because `pkg/listener` already imports `pkg/builder`. A role's class is
`RoleDeclaration.ListenerClass`; a product that exposes the class as a **user-settable** field in its
own config block must set it from `Contribution.ListenerClass` (§10) instead, because such a value
arrives from the config fold per role group, after the declaration is fixed.

**One handler instance serves every cluster** — it is built once in `main.go` — which is why the
handler now carries nothing role- or CR-shaped at all (§3b).
`sidecar.SidecarManager.CloneForBuild` covers the framework's own instance of the same hazard, a
handler-registered manager whose configs `SetProductImage` writes into. See
`docs/architecture.md` §4.1.4.

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

### 4b. Config Folding — Two Owners, One Rule Set

The `config` block is the one place a product **extends** a framework type. A product CRD embeds the
commons struct inline and adds its own fields beside it:

```go
type ConfigSpec struct {
    *commonsv1alpha1.RoleGroupConfigSpec `json:",inline"`   // resources, affinity, logging, …

    // A POINTER, not a bare string: the fold's "was this stated?" test is the zero value, so a
    // bare string cannot tell `myProductSetting: ""` from an absent field.
    MyProductSetting *string `json:"myProductSetting,omitempty"`

    // A composite must say how it folds. `atomic` accepts wholesale replacement; the alternative
    // is to flatten it into scalar pointers. Untagged, ValidateProductConfigType REFUSES it.
    Tls *TlsSpec `json:"tls,omitempty" kubedoop:"atomic"`
}
```

Both annotations are load-bearing rather than stylistic — `ValidateProductConfigType[ConfigSpec]()`
rejects the shape without them, and `FoldProductConfig` calls that validator first, so the call below
would return an error rather than a config.

That is the sanctioned extension mechanism, so folding it has **two owners**, and each folds its own
half with the framework's machinery:

```go
common, err := reconciler.FoldCommonConfig(decl.ConfigDefaults, roleCfg.RoleGroupConfigSpec, groupCfg.RoleGroupConfigSpec)
product, err := reconciler.FoldProductConfig(productDefaults, roleCfg, groupCfg)  // *ConfigSpec
```

`FoldCommonConfig` is what the **framework** calls on every pass, over three layers in this order:
`RoleDeclaration.ConfigDefaults` → the CR's role `config` → the CR's role group `config`. The result
is `RoleGroupBuildContext.EffectiveConfig()`. `FoldProductConfig` is generic and folds the product's
own fields the same way, skipping the embedded `*RoleGroupConfigSpec` so the two halves cannot fight
over it; a product calls it wherever it reads its own config.

**Four concepts were conflated before this, and separating them is the whole fix (#631):**

| | where it sits | who states it |
|---|---|---|
| **Declaration** | no user layer at all | product (`RoleDeclaration`) |
| **Default** | beneath the user's two levels | product (`ConfigDefaults`) |
| **Derivation** | computed *from* the folded result | product (`RoleGroupResolver`, §10) |
| **Constraint** | above the user — deliberately not implemented | — |

Constraint is absent on purpose. A framework that can overrule what a user wrote in their own CR
needs a way to *tell* them, and every candidate (silently clamping, failing the role group, a
Warning) is worse than letting the value through and letting the product validate it in a webhook,
where the user sees the rejection at `kubectl apply`.

**Per-field rules.** `resources` folds per **leaf**, so a default `cpu.min` survives a user who set
only `cpu.max` — a struct-level nil check would have discarded it. Scalars fold on presence.
`affinity` is replaced **wholesale**, and the replacement is **reported**:

```yaml
roles:
  server:
    config:
      affinity:                        # role: spread the quorum
        podAntiAffinity: {…}
    roleGroups:
      default:
        config:
          affinity:
            nodeAffinity: {…}          # group: pin to an instance type
```

The group wins the whole field: the role's `podAntiAffinity` is gone. That is the rule Kubernetes
itself uses for `PodSpec.affinity`, and the rule a Helm value and a Kustomize patch use, and keeping
it is a deliberate choice about **what a user has to learn**. Per-member folding was implemented once
and reverted, and the objection that carried the revert still holds: it obliges a user to learn a
merge semantic for exactly one field of one CRD, and `kubectl explain` is the only place they could
learn it. `resources` can fold per leaf without that cost because `resources.cpu.min` is a knob, not
a Kubernetes type a user already knows.

The cost of keeping the Kubernetes rule is real, and it is paid to an **event** rather than to a
second semantic. `FoldCommonConfig` returns a second value naming the members each replacement
discarded, and the reconciler emits a **`AffinityOverridden` Warning** on the CR:

```
role "server" group "default": the role group's config replaces config.affinity wholesale,
discarding the podAntiAffinity declared beneath it. config.affinity follows the Kubernetes rule
and is not merged per member; restate the discarded member alongside your own to keep it
```

Without it the loss is invisible everywhere: the CR still reads as the user wrote it, the pod spec is
valid, and every status condition stays green while the quorum quietly stops being spread. An
**empty** value (`affinity: {}`) clears — the single-node development escape hatch — and reports
**nothing**, because clearing is exactly what that value asks for. That clearing rule works here and
**not** in `resources` because the schemas differ: `affinity` is
`x-kubernetes-preserve-unknown-fields`, so the API server never prunes inside it and a stored `{}` is
always something the user wrote, while `resources` is structural and `cpu: {}` may be a pruning
artifact.

**A product field is folded presence-wins per top-level field, with no depth.** A composite
(a struct or pointer-to-struct) is **refused** unless it carries the `kubedoop:"atomic"` struct tag,
which accepts wholesale replacement for a value that is a single policy rather than a set of knobs;
the alternative is to flatten it into scalar pointers. A bare scalar is refused too — the fold's
"was this stated?" test is the zero value, and a bare `string` cannot distinguish a stated `""` from
an absent field, so product config fields are **pointers**. `ValidateProductConfigType[T]()` is that
check; call it at start-up to turn a latent mis-fold into a boot failure. `FoldProductConfig` calls
it too, so a caller that skips the start-up check still gets an error rather than a silently wrong
config.

**Overrides are a separate axis and keep their own rules.** The typed `config` block folds as above;
`configOverrides`/`envOverrides`/`cliOverrides`/`podOverrides` fold through `config.ConfigMerger`
(§7, §15). The two never mix.

### 5. Extension System
Three levels of extensions for injecting custom logic, all generic over the product CR type:
- **`ClusterExtension[CR]`**: `Name`, `PreReconcile`, `PostReconcile`, `OnReconcileError`
- **`RoleExtension[CR]`**: `Name`, per-role `PreReconcile` / `PostReconcile`
- **`RoleGroupExtension[CR]`**: `Name`, per-role-group `PreReconcile` / `PostReconcile`

There is no `Cleanup()` hook and no shutdown callback.

**A hook can say "not ready YET" without the cluster going Degraded.** Returning
`common.NewRequeueAfterError(after, reason, message)` reports that the desired state is not
satisfiable yet — a database migration Job, a peer cluster's discovery ConfigMap, a secret an
external controller has not issued. The framework requeues after the delay with **no `Degraded`, no
Warning event and no workqueue backoff**, and raises its own **`Waiting`** condition
(`reconciler.ConditionWaiting`) carrying the reason and message, while `ReconcileComplete` goes
False with reason `Waiting`.

```go
func (e *DBInit) PreReconcile(ctx context.Context, c client.Client, cr *v1alpha1.MyCluster) error {
    if !done { return common.NewRequeueAfterError(15*time.Second, "WaitingForDBInit", "migration Job has not completed") }
    return nil
}
```

Four things about it are load-bearing:

- **The wait is reported at the END of the pass, not by returning where it was raised.** Cleanup,
  health and PostReconcile all still run. `SetDegraded(false, …)` has exactly one writer —
  `HealthManager.Check` — so an early return would leave `Degraded` **latched** at whatever the last
  completed pass wrote: a cluster whose pods recovered while an extension waits would keep paging
  with a message naming a pod that no longer exists, and `observedGeneration` would match, so no
  staleness heuristic catches it. The framework already diagnosed exactly this on the
  `reconciliationPaused` gate and fixed it the same way — observe, then return.
- **A wait is not merged with a failure.** `common.WaitingErrors` walks the error tree and reports a
  wait only when **every leaf** is one, taking the **shortest** delay. The framework joins per-role
  and per-extension errors, so a tree can hold a wait next to a genuine failure — and a plain
  `errors.As` would find the wait and launder the failure into one, or return the first delay
  depth-first and let a ten-minute wait hide a five-second one.
- **The delay is clamped.** controller-runtime treats `RequeueAfter` as a switch, so a non-positive
  value never requeues at all; `deadline.Sub(time.Now())` would wedge the cluster permanently the
  moment the deadline passed. A non-positive value becomes `common.DefaultRequeueAfter`, anything
  under `common.MinRequeueAfter` becomes that.
- **`Reason` and `Message` must be STABLE while the wait lasts.** The status write is guarded by a
  whole-object `DeepEqual`, and this path returns no error so the workqueue `Forget`s the item —
  a message that counts ("3/5 brokers registered") rewrites the CR every pass with nothing to back
  the ping-pong off. Put the varying detail in a log line.

**A CLUSTER-level wait blocks the roles**, which is the point of #608's case — a schema migration
that must finish before any workload starts. The pass still continues to cleanup, health and
PostReconcile, so blocking the workloads does not also freeze the cluster's own observations. A
ROLE-level wait does not block: it is raised mid-iteration, so the roles before it have already been
reconciled and skipping the rest would make the outcome depend on map ordering.

A waiting extension also no longer aborts its lower-priority siblings (the default
`stopOnError: true` treated it as a precondition failure) and is logged at Info rather than Error —
`#608`'s complaint is that a normal first install pages, and an ERROR line every pass is the other
channel that pages.

**Relatedly, a role group the framework has not created yet is `Creating`, not `Degraded`.** Health
tells "never applied" from "applied, then vanished" using `status.roleGroups`, the ledger of role
groups a pass successfully applied. Before that, a first install reported `Degraded=True` with
`WorkloadUnreadable` for every role group whose StatefulSet did not exist yet — for any reason,
including a build still waiting on something external. A group **in** the ledger whose StatefulSet
has gone is still a fault, which is the guarantee that had to survive.

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
of `RoleDeclaration.ContainerPorts` part of the contract, not decoration: put the port that means
"this pod can serve" first. No liveness probe is generated at all. The framework knows neither which port
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
`LoggingFramework`-aware logging config generation (Log4j, Log4j2, Logback, Python) lives in
`pkg/productlogging`. `RoleDeclaration.LogProducers` declares which containers produce logs; the
framework merges the CRD logging spec, renders each one's config file and injects it into the role
group ConfigMap. **The same list names the producers of the Vector log pipeline** — one list, two
jobs.

**An empty `Framework` means the product writes that container's config file itself.** That is the
seam, and it is a value in the declaration rather than a method to override:

```go
LogProducers: []productlogging.ContainerLogging{{
    Container:   "airflow",
    Framework:   "",                     // the framework renders nothing
    LogFileName: "airflow.py.json",      // REQUIRED here, and must carry a known suffix
}},
```

Such a producer still joins the pipeline in full — shared volume, RW mount, log directory, Vector
source — with no framework-rendered file and no ConfigMap key to collide with its own.

Airflow's `log_config.py` is the case this exists for, and the reason is ownership rather than
impossibility: its content has to import Airflow's own `DEFAULT_LOGGING_CONFIG` and patch it, or the
task-log machinery that config wires up (`airflow.task` → `FileTaskHandler`) is lost. That file is
perfectly renderable — but only by something that knows Airflow's import path and dict layout, and
`pkg/productlogging`'s python renderer is shared by every Python product, so it emits a standalone
`dictConfig` instead. The collision is what makes the seam necessary rather than merely tidy: the
python renderer's default file name **is** `log_config.py`, so a rendered file would take the key the
product writes itself.

The product picks up the two obligations the framework can then no longer meet, and both are
**enforced** rather than left to a doc comment: `LogFileName` is required (without it nothing decides
where the file goes), and it must carry one of `productlogging.KnownLogFileSuffixes()` —
`.log4j.xml`, `.log4j2.xml` or `.py.json` — because that suffix is what selects Vector's edge
parser, and a file the parser does not recognise is collected as unstructured text with no level, no
logger and no timestamp.

**Ask the framework where the file goes; do not compose the path.**

```go
target := buildCtx.LogFileTarget(decl)   // "" means console-only this cycle
```

`LogFileTarget` returns a **conclusion**, not the inputs to one. Whether the Vector pipeline is
active is a pure function of things the framework already holds — `logging.enableVectorAgent` from
the folded config, the producer list, and the `vector.yaml` source from the declaration — so the
framework settles it and nothing else re-derives it. An empty return is not a failure: it means this
role group has no shared log volume this cycle, so a file appender would write nowhere useful and the
product should emit a console-only config. Composing the path by hand from `LogDirFor` and
`ContainerLogFileName` is correct while Vector is on and silently wrong the moment it is off, because
the appender then writes into the container's writable layer where nothing collects it.

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

### 10. RoleGroupResolver — Deriving from the Effective Config

A product derives values **from the folded config**, and that is a distinct stage with its own seam:

```go
type RoleGroupResolver[CR common.ClusterInterface] interface {
    ResolveRoleGroup(ctx context.Context, c client.Client, cr CR,
        rg *RoleGroupBuildContext) (*Contribution, error)
}
```

Set it as `GenericReconcilerConfig.RoleGroupResolver`; `RoleGroupResolverFunc` adapts a plain
function. It runs **once per role group**, after `FoldCommonConfig` has produced the effective
config and before anything is built — the only window where that config exists and no object has
been assembled yet. Read it with `rg.EffectiveConfig()`, which is never nil.

```go
func resolve(ctx context.Context, c client.Client, cr *v1alpha1.TrinoCluster,
    rg *reconciler.RoleGroupBuildContext) (*reconciler.Contribution, error) {

    props := map[string]string{"http-server.http.port": "8080"}
    if rg.RoleName == "coordinator" {
        props["coordinator"] = "true"
    }
    jvm := []string{}
    if mb, ok := reconciler.HeapMB(rg.EffectiveConfig(), 0.8); ok {
        jvm = append(jvm, fmt.Sprintf("-Xmx%dm", mb))
    }
    return &reconciler.Contribution{
        ConfigOverrides: map[string]map[string]string{
            "config.properties": props,
            "jvm.config":        {"jvm": strings.Join(jvm, " ")},
        },
    }, nil
}
```

`Contribution` carries `ConfigOverrides`, `EnvVars`, `PodOverrides` and `ListenerClass`. The first
three fold **beneath** the user's own `configOverrides`/`envOverrides`/`podOverrides`, per key,
through the merge's own rules — so a derived value is a default the user can still refine, never a
decision made over their head. `PodOverrides` is also the only channel that can carry a container env
var with a `valueFrom`: the override maps are `map[string]string` and cannot express a downward-API
reference or a `secretKeyRef` at all.

**There is deliberately no CLI dimension.** `cliOverrides` merge by *replacement* (§15), so a
contributed layer would either be erased whole by any user value or erase the user's — neither is a
default. Product arguments belong in `RoleDeclaration.Command`, which has no user layer.

**This is the seam that did not exist**, and it is why #631 was filed. A JVM heap sized from the
effective memory limit was hand-written three times with the same 0.8 factor, and a fourth product
gave up and froze the result into a literal `-Xmx419430k` — 0.8 of one role's default memory,
applied to every JVM component including one whose own default is twice that, and immune to every
user override. The framework made that inevitable: the effective config was not computed until
*after* the role group's ConfigMap had been built, so nothing derived from it could reach a config
file at all. `reconciler.HeapMB(cfg, factor)` is that calculation, centralised.

**It must be DETERMINISTIC for a given (cr, effective config).** Its output lands in the role group
ConfigMap, the framework applies that with `CreateOrUpdate`, and the reconciler watches what it
owns — so a value that varies between passes rewrites the ConfigMap every pass, wakes the reconciler
through its own watch, and never errors, which means the workqueue never backs the loop off. Put a
timestamp or a random suffix here and the cluster rewrites itself forever.

A returned error fails **only that role group** (the role loop is best-effort, §2); a
`*common.RequeueAfterError` reports "not ready yet" without going `Degraded` (§5).

**Precedence, low → high: product contribution < role overrides < role group overrides.** Any value
a user sets in the CRD always wins. `ConfigMerger.Merge` is variadic (`Merge(...*OverridesSpec)`) and
folds layers in order.

**This is config generation, not defaulting** — do not confuse it with the webhook `ProductDefaulter`:

| | `ProductDefaulter` (webhook) | `RoleGroupResolver` (this) |
|---|---|---|
| Targets | typed **spec fields** (image, ports, replicas) | **config-file content** (config.properties, etc.) |
| When | admission, **persisted into spec** | every reconcile, **not persisted** |
| Upgrade propagation | no (frozen at admission) | **yes** (recomputed with current operator) |
| Derived-from-live-state | freezes/stales | **recomputed each reconcile** |

Use `RoleGroupResolver` for product-intrinsic and derived config (a ZooKeeper connection string built
from the actual resources, a JVM heap sized from the effective memory limit, role-specific keys) so
the product no longer hand-builds ConfigMaps/StatefulSets. Use `ProductDefaulter` for stable,
user-facing typed spec defaults.


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
operator's ClusterRole and is a separate axis entirely, enumerated in `docs/security.md` §3.3.
Three things on that axis are worth knowing here because nothing announces them: `core/events`
`create;patch` (without it every `Warning` in §2 of this file is discarded by client-go with no
error and no retry, including `ImmutableFieldIgnored`, which has no log line and no condition to
fall back on); `core/pods` `get;list;watch` (the health pass Lists through the cache, so a 403 stops
`Degraded` being computed rather than reporting a fault); and the whole **cleanup** path, whose
errors the reconciler logs and swallows — only a 429 is fatal — so a 403 on a teardown delete leaves
the pass reporting success. Everything else fails loudly on the apply path: a forbidden informer
takes `manager.Start` down, and a forbidden create/update sets `Degraded`.

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
2. the role's `RoleDeclaration.LogProducers` is non-empty — an agent with nothing to collect would
   mount an empty pipeline;
3. **something supplies `vector.yaml`**: the CR implements `reconciler.VectorAggregatorProvider` (the
   framework renders the file) *or* the role sets `RoleDeclaration.OwnsVectorConfig` (the product
   writes it).

Gate 3 exists because registering the provider without a source for `vector.yaml` would fail the
`Validate` above on every cycle and abort the whole cluster's reconcile over a product that is simply
not wired for Vector. Instead the reconciler emits a `VectorSidecarSkipped` **Warning event** on the
CR naming the role group and both ways to satisfy the gate, and reconciliation continues. Within gate 3's first
branch, an empty `VectorAggregatorConfigMapName()` or an undiscoverable aggregator address is a hard
error, not a skip.

`SidecarConfig` carries `Image`, `ImagePullPolicy`, `Resources`, `EnvVars`, `Volumes`,
`VolumeMounts`, `Ports`, `Enabled`, `SecurityContext` and `Probes`. There is no `MainContainerName`
field and no `FindMainContainer` helper — a provider that must address the primary container uses
`sidecar.FindContainer(podSpec, name)`, and the primary container's name is controlled by
`RoleDeclaration.MainContainerName`.

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
2. **Declare your roles** - Implement `RoleProvider` (or pass a `RoleProviderFunc`) returning a
   `RoleCatalog`: one `RoleDeclaration` per role, carrying its ports, container name, command,
   probes, data volume, log producers and config defaults. This is where anything CR-dependent is
   computed — the CR is in scope
3. **Create RoleGroupHandler** - Embed `BaseRoleGroupHandler` for default resource building, or
   implement `RoleGroupHandler` directly. Construct it with the scheme alone; set
   `GenericReconcilerConfig.ImageResolution` to opt into CR-driven images
3b. **Derive from the effective config** (optional) - Implement `RoleGroupResolver` to contribute
   config-file content, env vars, pod overrides or a listener class computed from the folded config,
   beneath the user's own overrides
4. **Declare Dependencies** (optional) - Set `Dependencies` so missing ConfigMaps/Secrets fail fast with a Degraded condition
5. **Register Extensions** (optional) - Add custom hooks on a `common.NewExtensionRegistry[*MyCluster]()` and pass it via `GenericReconcilerConfig.ExtensionRegistry` — without that field the reconciler runs no hooks at all
6. **Setup Webhooks** (optional) - Use common defaults/validators from `pkg/webhook/`
7. **Register Health Checks** (optional) - Implement `ServiceHealthCheck` for business-level health verification
8. **Declare the operator's own RBAC** - copy the `+kubebuilder:rbac` block from `docs/security.md`
   §3.3.1 onto your controller, add whichever §3.3.2 conditional grants your operator actually
   triggers, and run `make manifests`. The SDK cannot declare these for you (controller-gen never
   walks a dependency's packages), and the kubebuilder scaffold generates markers only for your
   **own CR** — nothing the framework consumes. Unlike every other step here, two of these do not
   announce themselves when missing (§3.3.3)
9. **Create main.go** - Use `GenericReconciler` with your handler, and register any extra-resource GVKs through `SetupWithManagerOpts`

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

### CHANGELOG.md
**Do NOT edit `CHANGELOG.md` in a pull request.** The entries under a version heading are generated
**at release time from the commit history**, not written by hand per PR. The unreleased section
therefore stays empty between releases, and the release process fills it in.

What this means in practice:
- A PR carries its user-visible description in its **commit message**, which is the input the
  generator reads — so write the commit message as the changelog entry you would have written
  (Conventional Commits `type(scope): summary`, with `!` or a `BREAKING CHANGE:` footer for a
  breaking change, and the body explaining the *why* a reader needs).
- Behavioral and API documentation belongs in `AGENTS.md` (what exists today) and
  `docs/architecture.md` (design intent) — those **are** part of the PR. Hand-maintaining the same
  prose in three places is what made the changelog drift.
- Only the release itself touches `CHANGELOG.md`: see the release procedure for tagging and version
  bumps.

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
  method-set interface — `rolePodDisruptionBudgetBuilder`, `sidecar.PhasedProvider`,
  `sidecar.OwnImageProvider` — never to recover a concrete product type. `RoleProvider` and
  `RoleGroupResolver` are NOT in that class: they are explicit `GenericReconcilerConfig` fields, so
  a product that forgets to wire one gets the framework's stated default rather than a capability
  silently not detected

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
