# operator-go/pkg/reconciler - GenericReconciler Framework

**Parent:** [../AGENTS.md](../AGENTS.md)

GenericReconciler framework for operator reconciliation logic and state management.

## Key Files

Every non-test file in this package:

| File | Purpose |
|------|---------|
| `generic_reconciler.go` | `GenericReconcilerConfig` / `GenericReconciler` — the reconcile loop, panic recovery, ServiceAccount provisioning, dependency checks, role/role-group iteration, sidecar validation, status write, `SetupWithManager*` |
| `role_group_handler.go` | `RoleGroupHandler` / `RoleGroupHandlerFuncs`, `RoleGroupBuildContext`, `RoleGroupResources`, `VolumeProvider`, `VectorAggregatorProvider`, `LoggingProducerProvider`, `MergeRoleGroupConfig`, logging-config rendering helpers |
| `base_role_group_handler.go` | `BaseRoleGroupHandler` — the default resource builder products embed; `RoleNameProvider`, `BuildRolePodDisruptionBudget`, per-role setters |
| `apply.go` | `copyDesiredState` — update semantics of the apply path (issue #526): labels replaced wholesale, annotations merged, per-kind spec assigned wholesale minus the API-server-owned/immutable fields that are restored from the live object (StatefulSet selector/serviceName/volumeClaimTemplates/podManagementPolicy; Service clusterIP(s)/ipFamilies/ipFamilyPolicy/healthCheckNodePort/loadBalancerClass/allocated NodePorts), unstructured top-level copy for arbitrary-GVK extras |
| `cleaner.go` | `RoleGroupCleaner` — orphan cleanup (PDB → StatefulSet → ConfigMap → Service → headless → metrics), gray-delete grace period, status pruning, `WithEventManager`, `AnnotationPendingDeletion` / `AnnotationDeletePVCs` |
| `health.go` | `HealthManager` — role group aggregation into Available/Progressing/Degraded plus the optional product `ServiceHealthCheck` (run under `Timeout`) |
| `dependency.go` | `Dependency` / `DependencyKind` / `DependencyResolver` — declarative existence checks for referenced ConfigMaps and Secrets, plus the explicit `ValidateS3Connection` / `ValidateDatabaseConnection` / `ValidateZKConfig` helpers |
| `errors.go` | Typed reconcile errors: `ReconcileError`, `ConfigError`, `ResourceBuildError`, `ResourceApplyError`, `ValidationError`, `RateLimitError` and their `Is*` predicates |
| `event.go` | `EventManager` — Normal/Warning event emission on the CR |
| `discovery.go` | `EnsureDiscoveryConfigMap` — shared ensure-helper for product discovery ConfigMaps (CreateOrUpdate + controller owner ref + canonical labels; the product computes the data map) |

There is no `reconciler.go`, `status.go` or `finalizer.go` in this package — status is written by
`GenericReconciler.updateStatus` (see below) and the SDK registers no finalizer at all.

## Working Instructions

1. **Implementing a reconciler:** build a `GenericReconcilerConfig[CR]` and pass it to
   `NewGenericReconciler`. Product-specific resource building goes in a `RoleGroupHandler`
   (usually by embedding `BaseRoleGroupHandler`), not in a subclass of the reconciler.
2. **Status updates:** the reconciler owns the status write. `updateStatus` retries on conflict
   (re-Get, re-apply this cycle's conditions/roleGroups/observedGeneration) and **skips the write
   entirely** when the computed status is `apiequality.Semantic.DeepEqual` to the stored one — the
   controller watches its own CR, so an unconditional write would loop. Handlers and extensions
   contribute status by mutating `cr.GetStatus()` in place — they must never write the CR's status
   subresource themselves, or they will race the reconciler's own write.
3. **Deletion / finalizers:** the SDK registers **no finalizer**, and nothing in `pkg/` references
   one. CR deletion therefore executes **no SDK code**: every resource the framework applies carries
   a controller owner reference to the CR, and Kubernetes garbage collection reclaims it. Two
   consequences:
   - `AnnotationDeletePVCs` (`operator.zncdata.dev/delete-pvcs`) only affects the **orphan** path in
     `cleaner.go` — PVCs of a StatefulSet whose role group was removed or renamed in the spec. It
     has no effect when the whole cluster CR is deleted; those PVCs survive, because the SDK sets
     no `persistentVolumeClaimRetentionPolicy` and StatefulSet-managed PVCs therefore carry no
     owner reference for GC to follow.
   - Any external state a product creates outside owner-reference GC (objects in another namespace,
     cluster-scoped objects, non-Kubernetes resources) must be cleaned up by the product itself.
     `Reconcile` short-circuits on `IsNotFound` after deletion, so there is no hook to do it in.
4. **ServiceAccounts:** Prefer per-CR naming via `GenericReconcilerConfig.ServiceAccountNameFunc`
   (e.g. `"<product>-" + cr.GetName()`). The reconciler resolves the SA name per CR (func result >
   static `ServiceAccountName` > "" = skip), ensures the SA with the CR as controller owner
   (`ensureServiceAccount`), and propagates the resolved name through
   `RoleGroupBuildContext.ServiceAccountName` to the STS pod template. A static name shared by two
   clusters in one namespace permanently fails the second cluster's reconcile (AlreadyOwnedError,
   surfaced as a clear both-owners error) and GC-deletes the SA under the survivor when the owner
   cluster is deleted. The SDK creates no Role/RoleBinding — see `pkg/builder` for the builders a
   product uses to emit its own RBAC as `RoleGroupResources.ExtraResources`.
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
   negative disables), or earlier when a gray-delete grace period is still pending —
   `Cleanup` returns the earliest pending deadline and `earliestRequeue` picks the sooner of the
   two. Products with a `ServiceHealthCheck` depend on it: a probe result produces no watch event.
8. **Per-product extensions:** pass `GenericReconcilerConfig.ExtensionRegistry` to isolate a
   product's hooks; the default is the process-wide singleton shared by every reconciler.
9. **Pre-apply validation:** registered, enabled sidecar providers are validated via
   `SidecarManager.ValidateAll` after the ConfigMap/Services/extras are applied and **before** the
   StatefulSet. A failure aborts the role group with a `*ValidationError` (`NewValidationError` /
   `IsValidationError`) instead of creating pods that crash-loop on a missing mount.
10. **Configuration mistakes are surfaced, not swallowed:** a handler implementing
    `RoleNameProvider` has its `ConfiguredRoleNames()` checked against `spec.roles`; names the CR
    does not declare produce an `UnknownConfiguredRole` Warning event (a warning, not a failure —
    a handler may be configured for optional roles). A `podOverrides` layer that fails to decode is
    recorded on `config.MergedConfig.PodOverrideErrors` and re-emitted as a `PodOverrideIgnored`
    Warning event, so a dropped override is visible on the CR.

## Reconcile Flow

`Reconcile` fetches the CR (NotFound ⇒ done), then runs `reconcile` under a deferred panic
recovery that turns a recovered panic into a returned error plus a `ReconcilePanic` Warning event
(status untouched). `reconcile` performs:

0. `ClusterOperation` gate — `reconciliationPaused` returns immediately; `stopped` falls through
   (replicas are forced to 0 downstream so every resource is still reconciled).
1. ServiceAccount ensure (when configured), unknown-configured-role warning.
2. Cluster `PreReconcile` extensions.
3. Declared dependency validation.
4. Per role: role `PreReconcile` → per role group (`PreReconcile` → build context →
   `BuildResources` → apply `ConfigMap → HeadlessService → Service → ExtraResources →
   [sidecar validation] → StatefulSet → per-group PDB → MetricsService` → status tracking →
   `PostReconcile`) → role-level PDB → role `PostReconcile`.
5. Orphan cleanup (returns the earliest pending gray-delete deadline; errors are logged,
   not fatal).
6. Health aggregation (errors logged, not fatal).
7. Cluster `PostReconcile` extensions.
8. Final status update, then `ctrl.Result{RequeueAfter: earliestRequeue(...)}`.

On error the reconciler runs `OnReconcileError` hooks, sets `Degraded`, writes the status and
emits an error event. A `*RateLimitError` is special-cased: it backs off with
`RateLimitRetryAfter` (default 10s) without setting Degraded.
