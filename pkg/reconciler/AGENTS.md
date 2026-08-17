# operator-go/pkg/reconciler - GenericReconciler Framework

**Parent:** [../AGENTS.md](../AGENTS.md)

GenericReconciler framework for operator reconciliation logic and state management.

## Key Files

Every non-test file in this package:

| File | Purpose |
|------|---------|
| `generic_reconciler.go` | `GenericReconcilerConfig` / `GenericReconciler` — the reconcile loop, panic recovery, workload identity + RBAC provisioning, dependency checks, role/role-group iteration, sidecar validation, status write, `SetupWithManager*` |
| `role_group_handler.go` | `RoleGroupHandler` / `RoleGroupHandlerFuncs`, `RoleGroupBuildContext` (incl. `EffectiveConfig()` and `LogFileTarget()`), `RoleBuildContext`, `RoleGroupResources`, `VolumeProvider`, `VectorAggregatorProvider`, logging-config rendering helpers |
| `role_declaration.go` | `RoleDeclaration`, `RoleCatalog`, `RoleProvider` / `RoleProviderFunc`, `DataVolume`, `ValidateCatalog` — a product's per-role statement, produced once per pass with the cr in hand |
| `role_group_resolver.go` | `RoleGroupResolver` / `RoleGroupResolverFunc`, `Contribution`, `HeapMB` — the seam that derives config-file content from a role group's EFFECTIVE config, folded beneath the user's overrides |
| `config_fold.go` | `FoldCommonConfig` (the framework's half of the `config` block), `FoldProductConfig[T]` / `ValidateProductConfigType[T]` (the product's half), `ConfigFoldTag` / `ConfigFoldAtomic`, `AffinityReplacement` — per-leaf for `resources`, wholesale for `affinity` (with the discarded members reported), empty clears |
| `resolved_image.go` | `ResolvedImage`, `ImageResolution` — image, pull policy, pull secret and product version resolved ONCE per role group so the container and the sidecars cannot be told different things |
| `base_role_group_handler.go` | `BaseRoleGroupHandler` — the default resource builder products embed; `NewBaseRoleGroupHandler(scheme)`, `BuildRolePodDisruptionBudget`. Carries only reconcile-INVARIANT settings (`ConfigGenerator`, `Scheme`, `ConfigMountPath`, `LabelDomain`, sidecar manager, security contexts); everything role-shaped is in `RoleDeclaration` |
| `apply.go` | `copyDesiredState` — update semantics of the apply path (issue #526): labels replaced wholesale, annotations merged, per-kind spec assigned wholesale minus the API-server-owned/immutable fields that are restored from the live object (StatefulSet selector/serviceName/volumeClaimTemplates/podManagementPolicy; Service clusterIP(s)/ipFamilies/ipFamilyPolicy/healthCheckNodePort/loadBalancerClass/allocated NodePorts), unstructured top-level copy for arbitrary-GVK extras. `reconcileClaimVolumeMounts` keeps the pod template consistent with the claim templates that were preserved: a mount for a claim that was not created is dropped (the API server rejects the whole StatefulSet for it), and every mount a preserved claim had is restored from the live template (or the PVC stays bound and mounted nowhere), keyed on mount path so a multiply-mounted claim keeps all of its paths and a path the desired template already uses wins. `claimTemplatesDiffer` decides both that repair and the `ImmutableFieldIgnored` report, and asks whether the handler REQUESTED something else rather than whether the slices are byte-equal: `spec.volumeMode` and `status` are filled in by the server and count only when the handler states one, so an unchanged data PVC is silent (#627) while a resize or a rename is still reported |
| `cleaner.go` | `RoleGroupCleaner` — orphan cleanup as a multi-pass state machine (PDB → StatefulSet drain → [PVCs] → StatefulSet → [product extras] → ConfigMap → Service → headless → metrics, plus the role PDB of a removed role), `WithExtraResourceKinds`, gray-delete grace period, status pruning, `WithEventManager` / `WithDrainPollInterval` / `WithDrainTimeout` / `WithAPIReader` / `WithRateLimitRetryAfter`, `AnnotationPendingDeletion` / `AnnotationDeletePVCs` / `AnnotationDrainStarted`, `LabelRolePodDisruptionBudget` / `LabelRoleGroupPodDisruptionBudget`, `ConditionOrphanCleanupPending`, `DefaultDrainPollInterval` / `DefaultDrainTimeout` |
| `health.go` | `HealthManager` — role group aggregation into Available/Progressing, pod-failure detection into Degraded, the `Paused` condition, plus the optional product `ServiceHealthCheck` (run under `Timeout`) |
| `dependency.go` | `Dependency` / `DependencyKind` / `DependencyResolver` — declarative existence checks for referenced ConfigMaps and Secrets, plus the explicit `ValidateS3Connection` / `ValidateDatabaseConnection` / `ValidateZKConfig` helpers |
| `errors.go` | Typed reconcile errors: `ReconcileError`, `ConfigError`, `ResourceBuildError`, `ResourceApplyError`, `ValidationError`, `RateLimitError` and their `Is*` predicates |
| `event.go` | `EventManager` — Normal/Warning event emission on the CR. `NewEventManager(recorder, scheme)`: the scheme resolves the Kind named in resource events, which the typed objects `pkg/builder` produces do not carry. The framework emits exactly `Created`/`Updated`/`Deleted` (Normal) and `ReconcileError`/`ReconcilePanic`/`PodOverrideIgnored`/`UnusedRoleDeclaration`/`ImmutableFieldIgnored`/`VectorSidecarSkipped` (Warning) — **there are no reconcile start/completion events**. `LogAndEmitError`/`LogAndEmitInfo` exist for product code and the framework never calls them. **Every emit here depends on the operator's own ClusterRole holding `core/events: create;patch`** (`docs/security.md` §3.3): client-go treats a 403 on an event as permanent, logs `Server rejected event (will not retry!)` and discards it, and emission is fire-and-forget, so nothing reaches `Reconcile` and the pass reports success. `ImmutableFieldIgnored` is the only Warning with no paired log line and no status condition, so it is the one that goes completely dark |
| `discovery.go` | `EnsureDiscoveryConfigMap` — shared ensure-helper for product discovery ConfigMaps (CreateOrUpdate + controller owner ref + canonical labels; the product computes the data map) |
| `generated_secret.go` | `EnsureGeneratedSecret` — the ensure-helper for an object whose content must NOT converge: values are generated once, a missing key is filled, an existing value is never rewritten. Controller owner ref + canonical labels, `IsAlreadyExists` tolerated |
| `workload_rbac.go` | `EnsureWorkloadRBAC` — the namespaced `Role` + `RoleBinding` giving the cluster's PODS their API permissions, named after the derived ServiceAccount. Empty rules revoke; a foreign `roleRef` is never adopted; 403s are re-explained by cause (missing RBAC API access vs. escalation refusal) |
| `metrics.go` | `OrphanCleanupPending` (GaugeVec) and `OrphanDrainTimeouts` (CounterVec), registered on controller-runtime's `metrics.Registry` at init, plus `forgetClusterMetrics`. The framework exports **only** the cleanup state machine — see below |

There is no `reconciler.go`, `status.go` or `finalizer.go` in this package — status is written by
`GenericReconciler.updateStatus` (see below) and the SDK registers no finalizer at all.

## Working Instructions

1. **Implementing a reconciler:** build a `GenericReconcilerConfig[CR]` and pass it to
   `NewGenericReconciler`. Product-specific resource building goes in a `RoleGroupHandler`
   (usually by embedding `BaseRoleGroupHandler`), not in a subclass of the reconciler. The type
   parameter is constrained by `common.ClusterResource[CR]` (`common.ClusterInterface` plus
   `DeepCopy() CR`, which controller-gen generates), because the fetch path materialises the object
   it reads into by copying `GenericReconcilerConfig.Prototype` — `new(T)` is unavailable for a
   pointer type parameter. The CR must be registered with the scheme: it *is* the object
   `client.Get` reads into.
2. **Status updates:** the reconciler owns the status write. `updateStatus` retries on conflict
   (re-Get, re-apply this cycle's conditions/roleGroups/observedGeneration) and **skips the write
   entirely** when the computed status is `apiequality.Semantic.DeepEqual` to the stored one — the
   controller watches its own CR, so an unconditional write would loop. Handlers and extensions
   contribute status by mutating `cr.GetStatus()` in place — they must never write the CR's status
   subresource themselves, or they will race the reconciler's own write.
3. **Deletion / finalizers:** the SDK registers **no finalizer** of its own, and nothing in `pkg/`
   references one. Every resource the framework applies carries a controller owner reference to the
   CR, and Kubernetes garbage collection reclaims it.

   `Reconcile` recognises deletion **twice**, because a deleted CR is not necessarily an absent one:
   - **Background propagation** (the `kubectl delete` default) removes the CR from etcd at once, so
     the next event hits the `IsNotFound` branch and returns.
   - **Foreground propagation** (`--cascade=foreground`) and any product-registered finalizer leave
     the object *readable* with a `deletionTimestamp` until its dependents are gone. `Reconcile`
     therefore checks `cr.GetDeletionTimestamp()` immediately after the fetch and returns before the
     ClusterOperation gate and before any mutating step. Without that check the pass would re-create
     every owned resource with `BlockOwnerDeletion: true`, which foreground deletion can never get
     past, while each GC delete fired an `Owns()` watch event that scheduled the recreate again —
     an un-backed-off recreate loop against a permanently `Terminating` CR.

   Both paths run **no SDK teardown code**. Three consequences:
   - `AnnotationDeletePVCs` (`operator.zncdata.dev/delete-pvcs`) only affects the **orphan** path in
     `cleaner.go` — PVCs of a StatefulSet whose role group was removed or renamed in the spec. The
     deletion runs **after the drain** (or after the drain deadline) and immediately before the
     StatefulSet: nothing irreversible may happen while the pods are still running, so re-adding a
     role group mid-teardown costs a restart rather than the data. PVCs go before the StatefulSet
     because the cleaner finds them through its selector; a crash between the two re-enters the same
     pass, whereas the other order would strand them. It
     has no effect when the whole cluster CR is deleted; those PVCs survive, because the SDK sets
     no `persistentVolumeClaimRetentionPolicy` and StatefulSet-managed PVCs therefore carry no
     owner reference for GC to follow.
   - Any external state a product creates outside owner-reference GC (objects in another namespace,
     cluster-scoped objects, non-Kubernetes resources) must be cleaned up by the product itself.
     `GenericReconciler` offers no teardown hook: it returns on both deletion paths above.
   - A product that needs teardown work registers **its own** finalizer and observes the same
     `deletionTimestamp` from its own controller. The SDK's guard makes that safe — the framework
     stops re-creating resources as soon as the timestamp is set, instead of fighting the product's
     teardown for as long as the finalizer is held.
4. **Workload identity:** nothing to configure. Every cluster gets a ServiceAccount named
   `ServiceAccountResourceName(kind, cluster)` = `"<lowercased kind>-<cluster>"`
   (`hdfscluster-prod`). The reconciler ensures it with the CR as controller owner
   (`ensureServiceAccount`) at step 0 and propagates the same derived name through
   `RoleGroupBuildContext.ServiceAccountName` — never empty — to the STS pod template. The Kind is
   in the name because a CR name alone is not unique in a namespace; the name is derived rather
   than configured because the framework owns the object's whole lifecycle. Exported because things
   outside the operator have to name the SA.
4b. **Workload RBAC:** `GenericReconcilerConfig.WorkloadRBACRules func(cr CR) []rbacv1.PolicyRule`
   declares what the cluster's PODS call; the framework maintains a namespaced `Role` +
   `RoleBinding` at the derived SA's name, controller-owned by the CR (`workload_rbac.go`,
   `EnsureWorkloadRBAC`). Namespaced only — a namespaced CR cannot own a `ClusterRole`. An empty
   rule set REVOKES (ownership-gated); a **nil hook** touches nothing. A pre-existing RoleBinding
   with a different `roleRef` is never adopted (immutable field) and fails with a
   `*ValidationError`. Setting the hook requires the operator to hold those permissions itself plus
   write access to the RBAC API; the RBAC watches are registered only when the hook is set, because
   a forbidden informer kills the whole manager. `pkg/builder`'s RBAC builders remain for objects
   the framework does not maintain.
5. **External dependencies:** Declare referenced ConfigMaps/Secrets with
   `GenericReconcilerConfig.Dependencies func(cr CR) []Dependency`. They are checked before any
   role is reconciled; a missing one aborts the cycle with a Degraded condition. Nil = no checks.
   `DependencyResolver.Validate` is a retained no-op that the reconcile flow no longer calls;
   richer checks (`ValidateS3Connection`, `ValidateDatabaseConnection`, `ValidateZKConfig`) are
   explicit product-side calls, because only the product knows where those specs live.
6. **Watches:** `SetupWithManager` covers only the kinds the framework owns. Products emitting
   `RoleGroupResources.ExtraResources` must register those GVKs through
   `SetupWithManagerOpts(mgr, SetupWithManagerOptions{ExtraOwns: ...})` (or build on
   `ControllerBuilder`), otherwise out-of-band changes to them produce no reconcile event.
7. **Requeue cadence:** a successful reconcile requeues after
   `GenericReconcilerConfig.HealthCheckInterval` (default `DefaultHealthCheckInterval` = 120s,
   negative disables), or earlier when orphan cleanup has work pending — a gray-delete grace period
   that has not elapsed, or a deletion in flight (`DefaultDrainPollInterval` = 5s). `Cleanup`
   returns the earliest of those and `earliestRequeue` picks the sooner of the two. Products with a
   `ServiceHealthCheck` depend on it: a probe result produces no watch event.
8. **Per-product extensions:** `GenericReconcilerConfig.ExtensionRegistry` is a
   `*common.ExtensionRegistry[CR]`, typed for this reconciler's CR, and it is the **only** registry
   the reconciler executes — there is no process-wide fallback. Leaving it nil is legal and means
   every hook is a no-op (`NewGenericReconciler` substitutes an empty registry so the hook call
   sites stay unconditional), so an operator that registers extensions must wire the field or they
   silently never run. Build it with `common.NewExtensionRegistry[CR]()`; a binary hosting two CR
   types needs one registry per type, and sharing one is a compile error.
9. **Pre-apply validation:** registered, enabled sidecar providers are validated via
   `SidecarManager.ValidateAll` after the ConfigMap/Services/extras are applied and **before** the
   StatefulSet. A failure aborts the role group with a `*ValidationError` (`NewValidationError` /
   `IsValidationError`) instead of creating pods that crash-loop on a missing mount.
10. **Configuration mistakes are surfaced, not swallowed:** the `RoleCatalog` is checked against
    `spec.roles` once per pass, asymmetrically — a role the CR declares that the catalog does not is
    a **hard error** naming the typo and listing the near misses, while a role the catalog declares
    that the CR does not use is an `UnusedRoleDeclaration` Warning (a product may support more roles
    than a cluster runs; `RoleDeclaration.Optional` silences it). A `podOverrides` layer that fails to decode is
    recorded on `config.MergedConfig.PodOverrideErrors` and re-emitted as a `PodOverrideIgnored`
    Warning event, so a dropped override is visible on the CR.

    A `podOverrides` volumeMount at a **mountPath the framework already owns** fails the role group
    with a `*ValidationError`. Strategic merge keys volumeMounts by `mountPath`, not by `name`, so
    such a mount *replaces* the framework's instead of joining it. If the override also declares
    its volume the pod spec stays valid and the API server accepts it — the config ConfigMap ends
    up mounted nowhere and the product reads an empty config directory, with nothing anywhere
    reporting a problem. `StatefulSetBuilder.PodOverrideViolations()` reports it (plus any mount
    left referencing no declared volume) and `BaseRoleGroupHandler.buildStatefulSet` raises it.
    Mounting at a new path is untouched.
11. **Orphan cleanup is a state machine, not a single pass:** each role group advances one step per
    reconcile — the orphaned StatefulSet is scaled to zero, then left to the StatefulSet
    controller's ordered drain (`.status.replicas` back to 0), then deleted; every deletion is
    confirmed absent before the next resource type is touched, and the pass requeues itself
    (`DrainPollInterval`) until the group is fully reclaimed. The drain is bounded by
    `DrainTimeout` (default 10m, deadline stamped on the object as `AnnotationDrainStarted`), so a
    pod that can never terminate cannot strand the rest of the group; what is still pending is
    reported on the CR as `ConditionOrphanCleanupPending`. The "nothing left" verdict is
    re-confirmed through the uncached `APIReader` before the group's entry is dropped from
    `status.roleGroups`, because a cached read can answer NotFound for an object that exists. A failure
    is confined to its role group (the others still progress, errors are joined), and a 429
    anywhere in the cleanup surfaces as a `*RateLimitError` that `Reconcile` turns into a plain
    backoff. A role removed from `spec.roles` wholesale has its role PDB reclaimed by label
    (`LabelRolePodDisruptionBudget`, stamped by the apply path) — no role group is left to diff it
    out of the status snapshot. Every PDB reclaim is gated on its slot label
    (`LabelRolePodDisruptionBudget` / `LabelRoleGroupPodDisruptionBudget`), because a product's own
    PDB may share the derived name and carries the same controller owner reference.
12. **Orphans are discovered from the live cluster, not only from the status ledger.**
    `discoverOrphans` unions two sources: the role group ConfigMaps and StatefulSets this CR
    controller-owns that carry the framework's labels, and `status.roleGroups`. The ledger alone is
    a record the operator must have *successfully written*, so anything that loses it — a process
    death between applying a role group's resources and updating the CR, a backup tool restoring
    the CR without its status subresource, a `kubectl replace` — used to make those resources
    invisible to the cleaner permanently.

    A live object is claimed **only** when it carries the full
    instance/managed-by/component/role-group label set, is controller-owned by this CR, and is
    named exactly what `RoleGroupResourceName` produces for those labels. The name check is the
    decisive one: a discovery ConfigMap carries the same instance/managed-by pair and owner
    reference, and a product's `ExtraResources` may carry the handler's whole label set. Both kinds
    are listed because the teardown deletes the StatefulSet before the ConfigMap. An empty owner UID
    disables live discovery, as it does the role-PDB reclaim.

    `status.roleGroups` remains, written by the reconciler and pruned on a real deletion — it is
    still the only source that can attribute a *pre-labels* resource to a role group.
13. **Vector sidecar gating:** the framework injects the Vector sidecar only when something supplies
    `vector.yaml` — the CR implements `VectorAggregatorProvider` (the framework then renders it) or
    the role sets `RoleDeclaration.OwnsVectorConfig` (the product writes it). Otherwise it logs,
    emits a `VectorSidecarSkipped` Warning and skips the sidecar: registering it would fail the
    provider's own validation on every cycle and abort the whole cluster's reconcile. The resolved
    answer is recorded on the build context **unexported** — every input to it is already the
    framework's, so nothing else re-derives it — and reaches a product only as the conclusion
    `RoleGroupBuildContext.LogFileTarget(decl)`, which returns "" for console-only. The logging
    renderers gate the rolling file appender on the same answer: the Vector provider owns the shared
    log volume, so without the sidecar an appender would write to an unmounted path.
14. **Framework-owned selector labels:** the StatefulSet/Service/PDB selectors are derived from the
    cluster/role/role group names alone (or the product's `LabelDomain` identity labels). A CR
    label that collides with a selector key is **inert**, not an error: `buildLabels` applies the
    CR's labels first and the framework's identity labels over them, so the selector and the pod
    template can never end up disagreeing. (The predecessor of this rule was a build-time rejection
    of a colliding `BaseRoleGroupHandler.ExtraLabels` entry, which was needed because `ExtraLabels`
    was applied *last* and therefore won in the label map while the builder re-wrote the selector
    keys into the pod template afterwards. Ordering removes the failure mode instead of detecting
    it.)
15. **The recommended label set is metadata, and wider than the selector.** Every resource the
    framework builds carries the six keys declared in `pkg/constant/label.go`
    (`constant.LabelKubernetes*`), on both the object metadata and the pod template:

    | key | value | notes |
    |---|---|---|
    | `app.kubernetes.io/name` | `ImageResolution.ProductName` | omitted when the operator declares none |
    | `app.kubernetes.io/instance` | cluster CR name | in the selector |
    | `app.kubernetes.io/version` | `ResolvedImage.ProductVersion` | omitted unless `ProductName` is set — without it the product resolves its own images and the framework reads nothing beyond `spec.image.custom` |
    | `app.kubernetes.io/component` | role name | in the selector |
    | `app.kubernetes.io/role-group` | role group name | absent on role-level resources, which span every group |
    | `app.kubernetes.io/managed-by` | `operator-go` | in the selector |

    `name` and `version` are dropped when the value is not a legal label value: `productVersion` is
    free-form user input and a legal image tag may still be an illegal label value (>63 chars), and
    that has to cost one cosmetic label rather than make every resource of the cluster rejected.
    `instance`/`component` are deliberately *not* guarded that way — they also feed the selector, so
    an over-long value is already fatal there and hiding it here would only move the failure.

    **`version` and `role-group` are metadata only and must stay out of `.spec.selector`** (see
    `frameworkSelectorLabels`): a StatefulSet selector is immutable, `version` changes on every
    product upgrade, and the role group is already pinned by the `<cluster>-<group>` marker key.
    Upgrading an existing cluster into this label set rolls its pods once (the pod template gains
    labels) but leaves the frozen selector satisfied.

    **The marker key is bounded by `RoleGroupMarkerLabelKey`, not concatenated.** A label key's name
    part is capped at 63 bytes, and `<cluster>-<group>` is built from two free-form user strings: a
    43-character cluster plus a 21-character role group is 65, at which point the API server rejects
    the StatefulSet, both Services *and* the PDB of that role group, quoting a label key the user
    never wrote. The resource *name* built from the same two strings was already bounded
    (`RoleGroupResourceName`); this second derivation was not, so the framework applied a limit it
    knew about to one of the two places it applies.

    The helper returns the natural `<cluster>-<group>` whenever that is a legal label key, and
    `RoleGroupResourceName` otherwise. Preserving the natural form is **load-bearing, not
    cosmetic**: `.spec.selector` is immutable, so changing this key for a role group that already
    has a StatefulSet would leave the pod template no longer matching the frozen selector and every
    later update would be rejected. Only combinations that could never have produced a StatefulSet
    get the substitute, which is what makes the change safe to roll out to running clusters.
16. **There is exactly one label channel, and it is the cluster CR.** `buildLabels` layers, low to
    high: `buildCtx.ClusterLabels` (the CR's own, cloned per cycle) → the recommended set →
    `frameworkSelectorLabels` → the product's `LabelDomain` identity labels. Anything an operator's
    *user* wants on the workloads — above all the platform opt-ins the SDK does not own, such as
    `restarter.kubedoop.dev/enable` — is set by labelling the CR.

    **Three label keys are withheld from that channel** (`reservedSlotLabelKeys` in
    `generic_reconciler.go`): `metrics.kubedoop.dev/service`, `pdb.kubedoop.dev/role` and
    `pdb.kubedoop.dev/role-group`. These are the framework's *slot markers* — labels whose presence,
    or whose value, makes a reclaim SELECT an object for deletion — and unlike the
    `app.kubernetes.io/*` set nothing downstream overwrites them, so a CR carrying one would stamp
    it on every resource the handler builds and make each of them answer to a reclaim aimed at the
    slot. A CR labelled `pdb.kubedoop.dev/role=anything` was enough to make
    `cleanupOrphanedRolePDBs` reap every per-group PDB shipped through the
    `RoleGroupResources.PodDisruptionBudget` escape hatch, since it reads the role name from the
    label's value and deletes any PDB naming a role the spec does not declare. The filter is an
    enumerated set rather than a `kubedoop.dev` prefix rule precisely because
    `restarter.kubedoop.dev/enable` proves that domain is shared with the platform, not
    framework-private; every other CR label still propagates unchanged.

    `BaseRoleGroupHandler.ExtraLabels` and `ExtraAnnotations` are **gone**. They were compile-time
    fields on a handler, i.e. a decision frozen when the operator was built, for something decided
    when a cluster is deployed; and `ExtraLabels` specifically existed to paper over the three
    recommended labels the framework was not emitting (§15), which it now does.

    **Annotations have no replacement channel: the framework sets none on the resources it builds.**
    The CR's annotations are deliberately *not* propagated the way its labels are — that map holds
    `kubectl.kubernetes.io/last-applied-configuration` and the cleaner's own
    `orphan.zncdata.dev/*` progress markers, neither of which belongs on a ConfigMap. A product
    needing e.g. cloud LoadBalancer annotations on the client Service has no supported way to set
    them today; see zncdatadev/operator-go#553.
17. **The framework exports metrics for the orphan cleanup state machine and nothing else.** Both
    live in `metrics.go`, are registered on controller-runtime's `metrics.Registry` at init (so
    they appear on the endpoint an operator already serves, with no wiring in `main.go`), and are
    labelled `namespace` + `cluster`:

    - `operator_go_orphan_cleanup_pending` (Gauge) — role groups not finished being reclaimed.
      Written on **every** pass including at zero; a gauge only set while something is pending keeps
      its last non-zero value after the teardown finishes.
    - `operator_go_orphan_drain_timeouts_total` (Counter) — orphaned StatefulSets deleted with pods
      still terminating. A counter because the interesting question is "did this ever happen", and
      the answer must survive an operator restart.

    `forgetClusterMetrics` **deletes** (not zeroes) a cluster's series on the `IsNotFound` branch of
    `Reconcile`, which is the only place the framework learns a CR is gone — it registers no
    finalizer, so there is no teardown callback. A zeroed series still publishes a series for
    something that does not exist.

    The boundary is deliberate. controller-runtime already exports reconcile counts, errors and
    durations (`controller_runtime_reconcile_*`), and kube-state-metrics already turns CR status
    conditions into series once configured for the product's CRD — `Available`/`Progressing`/
    `Degraded` belong there. What neither covers is the cleanup state machine, because it is
    internal to this SDK: it spans many reconciles, records progress in annotations on the objects
    it is retiring, and reports the rest in log lines, so a role group stuck mid-teardown for three
    days produces no error, no failing reconcile and no condition transition. Do not grow this file
    into a second copy of tools that already exist.

18. **The framework owns the NAME of every fixed `RoleGroupResources` slot; the handler owns its
    content.** `validateRoleGroupResources` runs before step 1 of `applyResources` and fails the
    role group with a `*ValidationError` when a slot's name is not the derived one —
    `<resource>` for `ConfigMap`, `Service`, `StatefulSet` and `PodDisruptionBudget`,
    `<resource>-headless` and `<resource>-metrics` for the two suffixed Services — or when its
    namespace is not the cluster's.

    The rule is not stylistic. Both paths that REMOVE a slot address it by that derived name: the
    in-spec reclaims (`reclaimMetricsService`, `reclaimRoleGroupPDB`) and `RoleGroupCleaner`'s
    teardown. Nothing recovers a slot filled under another name — `discoverLiveOrphans` rejects any
    object whose name is not what `RoleGroupResourceName` produces, and `confirmRoleGroupReclaimed`
    re-checks the same fixed list before pruning the group's status entry — so such an object was
    applied, owner-referenced, reported healthy, and then survived every teardown until the cluster
    CR itself was deleted. For the metrics slot that is a Prometheus target with no endpoints; for
    the ConfigMap and StatefulSet it is a workload nothing will ever look at again.

    The check is **pre-flight** rather than at each slot's apply step, so a rejected declaration
    leaves nothing half-converged. It is a hard failure rather than a warning for the same reason
    the `podOverrides` mountPath violation is (§10): the alternative reinstates the leak.

    `ExtraResources` takes the other branch of the same trade — the product owns those names, so
    their reclaim is label-based and opt-in through `SetupWithManagerOptions.ExtraOwns`. That is
    the supported route for a differently named metrics Service.

## Reconcile Flow

`Reconcile` fetches the CR (NotFound ⇒ done), then runs `reconcile` under a deferred panic
recovery that turns a recovered panic into a returned error plus a `ReconcilePanic` Warning event
(status untouched). `reconcile` performs:

0. `ClusterOperation` gate — `reconciliationPaused` returns immediately; `stopped` falls through
   (replicas are forced to 0 downstream so every resource is still reconciled).
1. ServiceAccount ensure (always; name derived from the CR), workload RBAC ensure (only when
   `WorkloadRBACRules` is set), unknown-configured-role warning.
2. Cluster `PreReconcile` extensions.
3. Declared dependency validation.
4. Per role: role `PreReconcile` → per role group (`PreReconcile` → build context →
   `BuildResources` → apply `ConfigMap → HeadlessService → Service → ExtraResources →
   [sidecar validation] → StatefulSet → per-group PDB → MetricsService` → status tracking →
   `PostReconcile`) → role-level PDB → role `PostReconcile`.
5. Orphan cleanup (returns the earliest pending wakeup — a gray-delete deadline or a deletion in
   flight; errors are logged and non-fatal, except a `*RateLimitError`, which aborts the cycle into
   a backoff).
6. Health aggregation (errors logged, not fatal).
7. Cluster `PostReconcile` extensions.
8. Final status update, then `ctrl.Result{RequeueAfter: earliestRequeue(...)}`.

On error the reconciler runs `OnReconcileError` hooks, sets `Degraded`, writes the status and
emits an error event. A `*RateLimitError` is special-cased: it backs off with
`RateLimitRetryAfter` (default 10s) without setting Degraded.
