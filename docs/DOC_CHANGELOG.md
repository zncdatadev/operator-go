# Documentation Changelog

This document tracks all changes made to the SDK documentation.

---

## [2026-07-26] (adversarial review — six sections describing code that no longer exists)

An adversarial re-read of the branch found the architecture document still describing pre-wave
behavior in six places. Because `architecture.md` is declared authoritative, a reader following it
would have written code against contracts the SDK does not have. Every replacement claim below was
re-verified against the committed source named beside it; both language versions carry identical
edits.

### Architecture Documentation (`architecture.md`, `architecture_zh.md`)

- **§4.2.5 told hook authors the opposite of the truth about status.** The "Hooks are
  observe-and-act … mutations to the in-memory CR are never persisted" bullet contradicted the
  status path, which issues `Status().Update` from the in-memory object *without* re-fetching
  precisely so that a hook's product status fields survive (`generic_reconciler.go:updateStatus`,
  regression test `persists product-specific status fields written by an extension hook`). The
  bullet is split: spec mutations are neither persisted nor reliably observed; status mutations
  through `cr.GetStatus()` or a product's own fields **are** persisted by the final write.
- **§4.4 documented the deleted single-pass cleaner.** Rewritten from `pkg/reconciler/cleaner.go`:
  orphan cleanup is a multi-pass state machine — ordered scale-to-zero under `RetryOnConflict`, a
  drain wait on `.status.replicas`, per-step deletion confirmation (`confirmDeleted`), per-group
  error isolation, the role-PDB reclaim by `pdb.kubedoop.dev/role` label, the derived-name collision
  guard, the `DefaultDrainPollInterval` pacing, and a returned requeue duration. §4.4.2 step 4,
  §4.4.3's "best-effort, single-pass semantics" block and §4.4.4's 409/429 rows all asserted the
  reverse; §4.4.4 now records that a 429 becomes a `*RateLimitError` that aborts the pass into a
  backoff instead of a `Degraded` condition.
- **§8.2 listed two delivered items as future work** (ordered drain, cleaner conflict/throttling
  resilience). Removed.
- **§3.2.2, §3.2.5, §5.1.2 documented `RoleInterface`/`RoleInfo`/`RoleGroupInfo`**, which
  `pkg/common/role_interface.go` no longer defines. Replaced with how role and role-group
  configuration actually reaches a handler: the `reconciler.RoleGroupBuildContext` the reconciler
  builds per role group. The role level now costs a product zero methods, which is the ISP point
  §5.1.2 was making.
- **§4.6.2 described two Vector gates; there are three.** Documented the third — a source for
  `vector.yaml`, either `VectorAggregatorProvider` on the CR or the new
  `reconciler.VectorConfigProvider` on the handler — and the `VectorSidecarSkipped` Warning event
  emitted when neither is present, together with the reason it is a skip rather than a failure.
- **Consequential cross-references corrected**: §4.8.4 and the §5.3.3 template box (the cleaner's
  requeue is a gray-delete deadline *or* a drain poll interval), §4.13.2 (the cleaner does retry on
  conflict, on the scale-to-zero), §6 (orphan-cleanup solution summary), and §4.7.2
  (`ValidateZKConfig`; `ValidateZKConnection` is a deprecated alias forwarding to it).

### Project Documentation (`AGENTS.md`)

- §14: documented the three Vector gates and the `VectorSidecarSkipped` event.
- §2: the requeue cadence includes the drain poll interval, and a new paragraph describes orphan
  cleanup as a multi-pass state machine with per-group error isolation, 429 backoff and the role-PDB
  label reclaim.

### Upgrade Guide (`docs/UPGRADING.md`)

- **Added a "Which before this guide describes" note.** Removed symbols are checked against `main`
  before the v0.13.0 work — *not* against the last tag: v0.12.6 predates the framework entirely (no
  `pkg/common`, no `GenericReconciler`, no `pkg/sidecar`/`pkg/vector`/`pkg/security`, and
  `pkg/constants` rather than `pkg/constant`), so an operator on v0.12.6 is adopting the framework,
  not upgrading it.
- **Four "Removed" rows cited symbols that never shipped.** `common.NewExtensionRegistry()`
  (non-generic), `common.AsClusterExtension`/`AsRoleExtension`/`AsRoleGroupExtension` and
  `RegisterClusterExtensionWithOptions` exist nowhere in `git show main:pkg/common/`; the
  `RoleGroupCleaner.Cleanup` row invented a five-parameter "before" when only the return type
  changed. The §4 worked example's "Before" block cited two of the phantom symbols and is now the
  code `main` actually had. Added the two genuine registry behaviour changes a reader must know
  about (`WithStopOnError` defaults, and same-priority extensions now running in registration order
  where `sort.Slice` on priority alone used to leave them arbitrary).
- **§2 named the removed Python logging file but not its replacement.** It is `log_config.py`
  (`productlogging.pythonGenerator.DefaultFileName`), with the sites a product must repoint and a
  checklist item; the Vector-collected rolling log file (`<container>.py.json`) is unaffected.

---

## [2026-07-25] (second pass — three breaking redesigns)

### Architecture Documentation (`architecture.md`, `architecture_zh.md`)

Follow-up to the consistency pass below, covering three breaking API redesigns that landed after it.
Every claim was re-verified against the working-tree code; both language versions were edited with
identical section numbering.

#### Generic, per-CR-type extension registry (no process-global instance)

- §4.1.2: replaced the "Erasure at the registry boundary" bullet — the registry no longer stores
  `ClusterExtension[ClusterInterface]` entries and the `AsClusterExtension`/`AsRoleExtension`/
  `AsRoleGroupExtension` adapters no longer exist. It now describes `ExtensionRegistry[CR]`, why the
  type parameter is load-bearing (Go generic types are invariant), and states plainly that there is
  no process-global registry and no global accessor.
- §4.2.3: rewritten to the real API surface — `common.NewExtensionRegistry[CR]()` (explicit type
  argument), the three variadic `Register{Cluster,Role,RoleGroup}Extension(ext, opts...)` methods
  (the nine `...WithPriority`/`...WithOptions` variants are gone), `WithPriority`/`WithStopOnError`,
  `Clear()` (empties in place, since a constructed reconciler captured the pointer), the
  introspection methods, and wiring through `GenericReconcilerConfig[CR].ExtensionRegistry`. Added a
  `main.go` snippet and the warning that omitting the field makes every hook a silent no-op —
  `GetExtensionRegistry()`/`ResetExtensionRegistry()` and the global fallback are removed.
- §5.4: was "Singleton Pattern" with `ExtensionRegistry` as its exemplar. The pattern no longer
  applies; the section is now **Owned Collaborator Pattern (Composition over Global State)**,
  describing the registry (and the scheme) as explicitly constructed values passed through
  configuration. The code snippet matches the generic declaration, and the registration example
  shows a `*HdfsCluster` extension registered directly, with no adapter and no type assertion.
- §5.8: the pattern table row `Singleton | ExtensionRegistry | Global state management` became
  `Owned Collaborator | ExtensionRegistry[CR], Scheme | Explicit wiring, no global state`. The zh
  table was also localized (it had remained in English).
- §3.2.2, §6: extension interfaces and registry described as generic over the product CR; the
  "confining the remaining erasure to the registry adapters" claim removed.

#### Split configuration format contract

- §4.5.2: `ConfigFormat` no longer exists. Documented `ConfigMarshaler` (**required**, `Marshal` —
  what `NewConfigGenerator`, `RegisterFormat` and `GetFormat` take/return) and `ConfigUnmarshaler`
  (**optional**, discovered by interface upgrade on the `Parse` paths). An emit-only format
  registers and generates normally; only a parse attempt fails, with `*config.UnsupportedParseError`
  naming the format and file (`errors.As` is the stable check; a nil format yields
  `config.ErrNoFormat`).
- §4.5.2 adapter bullets corrected to the shipped behavior: Env value quoting is an allowlist
  (`[A-Za-z0-9_@%+=:,./-]`, everything else double-quoted) and single-quoted values read literally;
  XML rejects C0 controls and non-UTF-8 and emits `&#13;` for CR; Properties decodes `\uXXXX` and
  drops continuation indentation; YAML rejects duplicate keys.
- §4.5.2: new **Adapter selection** bullet — registrations match as file-name suffixes, the longest
  match wins deterministically, and `MultiFormatConfigGenerator.Parse(filename, content)` is the
  supported parse-by-file-name entry point.
- §4.5.3, §5.2.2, §5.2.4, §5.6.2, §5.6.3: `ConfigFormat` → `ConfigMarshaler`; the strategy snippet
  shows both halves and `ConfigGenerator` holding only the required one. `INIAdapter` added to the
  adapter lists.

#### Shrunk `ClusterInterface`

- §3.2.2, §5.1.2: `ClusterInterface` is `sigs.k8s.io/controller-runtime/pkg/client.Object` plus
  exactly `GetSpec()` and `GetStatus()`. `SetStatus`, `GetObjectMeta`, `GetScheme`,
  `GetRuntimeObject` and `DeepCopyCluster` are gone. Documented the companion constraint
  `ClusterResource[T ClusterInterface]` (= `ClusterInterface` + `DeepCopy() T`) and why the
  reconciler needs it.
- §5.1.4: the example no longer claims embedding is "NOT enough" and no longer mentions
  `common.ClusterObject` (the type is deleted). It shows the real interface, the constraint, and a
  CR writing exactly two methods — `metav1.TypeMeta`/`ObjectMeta` plus controller-gen's
  `DeepCopy`/`DeepCopyObject` cover the rest — with a note that the CR must be scheme-registered
  because the reconciler reads into the CR itself.
- §4.1.2, §3.2.5: the reconciler's type parameter is `[CR ClusterResource[CR]]`
  (`GenericReconciler`, `GenericReconcilerConfig`, `NewGenericReconciler`), with the reason
  (prototype `DeepCopy` returns the concrete type). `RoleGroupHandler[CR ClusterInterface]` and the
  extension interfaces are unchanged and stay on `ClusterInterface`.
- §4.13.2: the optimistic-locking bullet now says the framework mutates the status through the
  pointer `GetStatus` returns, so no reader infers a setter.
- §7.2: rewritten as a checklist a product author can follow today — CRD struct with
  `TypeMeta`/`ObjectMeta` and the root marker, scheme registration, `make generate`, the two
  interface methods, `RoleGroupHandler`, the required `GenericReconcilerConfig` fields, and an
  optional extension step that ends in setting `ExtensionRegistry`.

---

## [2026-07-25]

### Architecture Documentation (`architecture.md`, `architecture_zh.md`)

Consistency pass: every concrete claim was re-verified against the code, and claims the code does
not implement were removed or restated as the real, weaker behavior. All edits were applied to both
language versions with identical section numbering.

#### Removed (documented but never implemented)

- `RoleExtender` — deleted from §3.2.2, §4.1.2 and §5.1.2. No such type exists; role-level
  customization happens through `RoleExtension` Pre/PostReconcile hooks. (Supersedes the
  `RoleConfigExtender` → `RoleExtender` rename recorded under [2026-03-09], which renamed a type
  that was never in the code.)
- Extension `Cleanup()` shutdown hook (§4.2.4) — the extension interfaces declare only
  `Name`/`PreReconcile`/`PostReconcile`/`OnReconcileError`.
- `ExtensionRegistry.Register()` (§4.2.3) — replaced with the real per-level methods.
- Webhook "common logic" claims about CPU/Memory, ZooKeeper port 2181, log paths and replica
  validation (§4.3.2) — none of it exists in `pkg/webhook`.
- Cleanup guarantees that do not hold (§4.4.3, §4.4.4): per-delete confirmation, waiting for
  scale-down, in-cleaner 409 retry, 429 exponential backoff, atomic status+cluster update.
- §5.3.3's "creation order is the inverse of the deletion order" — the two orders genuinely differ.
- `ListenerScopeAnnotation` (§4.15.2) and the listener scope concept (§4.10.2) — removed from the API.
- Auxiliary API models that do not exist (`RoleCommonConfig`, `RoleGroupCommonConfig`) in §3.2.1.

#### Corrected

- §3.2.2 / §5.1.2 / §7.2: `RoleInterface`'s real methods, and that implementing it is optional
  (the reconciler never calls it); integration requires `ClusterInterface` + `RoleGroupHandler`.
- §3.2.4 / §4.8.3: `ExecUtil` is a consumer-facing helper, not a reconcile-loop component;
  `ServiceHealthCheck.CheckHealthy` takes `(ctx, client, namespace, name)`.
- §2.5: CLI-args slice strategy — the framework path is always Replace; empty means "unset";
  `podOverrides` decode failures are recorded and surfaced as events.
- §4.2.3/§4.2.5: real registration API, registration-order tiebreak, and the actual per-hook
  fault-tolerance policy including `common.WithStopOnError`.
- §4.4.2–§4.4.5: single-pass best-effort deletion, ownerReference-based ownership check, metrics
  Service in the deletion set, gray-delete grace period with a real requeue, status pruned only for
  really-deleted groups, and the PVC annotation's actual scope.
- §4.5.2: `ConfigFormat` requires `Marshal` **and** `Unmarshal`; config generation is integrated on
  the ConfigMap path (`BaseRoleGroupHandler.ConfigGenerator` + `ConfigMapBuilder.WithMergedConfig`),
  not in `StatefulSetBuilder`.
- §4.6.2: `Inject` takes `*SidecarConfig`; `Validate` exists and now runs before the StatefulSet.
- §4.7.2: dependency validation is an opt-in `GenericReconcilerConfig.Dependencies` hook, not
  automatic traversal of the CR spec; `DependencyResolver.Validate` is a no-op.
- §4.10.2: the SDK declares a generic ephemeral CSI volume whose PVC template carries the
  annotations — it creates neither a PVC object nor a Service.
- §4.12.2: `pkg/s3` renders S3A properties as opt-in helpers (not "automatically", not via
  `ConfigGenerator`), credentials travel over CSI and are never rendered as properties;
  `DatabaseConnection` has no renderer.
- §5.3.3: apply semantics for Service (whole `ServiceSpec` assigned, server-owned fields restored).
- §5.4.4: registry snippet now matches the real generic, entry-wrapped, sequence-ordered declaration.
- §8.2: the roadmap list is explicitly labelled as not-yet-implemented; delivered items (gray
  deletion, extension fault-tolerance degradation) were removed from it.

#### Added

- §4.3.3: a webhook wiring example that compiles against controller-runtime v0.23.x.
- §4.8.4 **Reconcile Requeue Policy**: the success path requeues at `HealthCheckInterval`
  (default 120s), with the earliest pending gray-delete deadline winning when sooner; 429, error,
  panic and paused paths documented; status writes skipped when deep-equal to the live object.
- §4.13.2: pre-flight validation (handler role names, declared dependencies, sidecar dependencies,
  malformed `podOverrides`) and the panic path returning an error so controller-runtime backs off
  while the status is left untouched.
- §4.14.2: `Deleted` events from the cleaner, and Warning events for `ReconcilePanic` and
  `podOverrides` failures.

## [2026-06-29]

### Security Documentation (`security.md`)

#### Changed
- Rewrote Section 3.3 (Pod Security Guidelines) to document the framework's single, canonical
  default pod/container `SecurityContext` applied unconditionally by the base role-group handler:
  - Pod-level: `runAsUser=1001`, `runAsGroup=0` (OpenShift-compatible), `fsGroup=1001`,
    `runAsNonRoot=true`, `seccompProfile.type=RuntimeDefault`
  - Container-level: `runAsUser=1001`, `runAsGroup=0`, `runAsNonRoot=true`,
    `allowPrivilegeEscalation=false`, `capabilities.drop=[ALL]`, `seccompProfile.type=RuntimeDefault`

#### Added
- Documented that `MergedConfig.PodOverrides` **REPLACES** the whole `SecurityContext` (no deep
  merge): a product overriding it must restate any hardening fields it wants to keep, and special
  images override the full SecurityContext this way. Noted the `WithoutDefaultSecurityContext()`
  escape hatch that disables the default entirely.

---

## [2026-06-28]

### Architecture Documentation (`architecture.md`, `architecture_zh.md`)

#### Changed
- Section 2.5 (Strict Merge Strategy): reframed config merging as an **ordered, variadic layer fold** (`Product Config < Role < RoleGroup`) instead of a fixed two-layer Role↔RoleGroup merge. Clarified that the two-layer merge is the special case with no product layer (backward compatible).
- Section 2.5: corrected the slice-merge description — only `cliOverrides` is a merged slice type (removed the inaccurate "JVM/Volumes").
- Section 1.3 (Overrides) and 3.2.3 (`ConfigMerger`): updated to state the full precedence `Product Config < Role < RoleGroup`.
- Section 4.3.2: scoped `ProductDefaulter` to typed Spec-field defaulting and cross-referenced the new product-config-computation mechanism.

#### Added
- Section 2.6 (Product Config vs. Defaulting): new section drawing a clear boundary between **`ProductDefaulter`** (Webhook; static defaults for typed Spec fields, persisted at admission) and **`ProductConfig`** (reconcile-time *computed* config-file content, merged as the lowest layer, never persisted). Captures the rationale: upgrade propagation and freshness of values derived from live cluster state.

## [2026-03-09]

### Architecture Documentation (`architecture.md`, `architecture_zh.md`)

#### Added
- Added detailed explanation of Role's two configuration sections in Section 1.3 (Terminology Definition):
  - `roleConfig`: Kubernetes-level management controls (e.g., PodDisruptionBudget), Role-scoped only, NOT inherited by RoleGroups
  - `config`: Workload runtime configuration (resources, affinity, logging), serves as defaults for RoleGroups and CAN be inherited and overridden
- Added important note in Overrides terminology: override fields (`configOverrides`, `envOverrides`, `cliOverrides`, `podOverrides`) are **flattened** directly at Role/RoleGroup level, NOT nested under an `overrides` field

#### Changed
- Renamed interface from `RoleConfigExtender` to `RoleExtender` across documentation
- Updated interface description from "configuration extender for parsing and merging differentiated configurations" to "Role extender for extending `role.config` fields with product-specific settings"
- Updated corresponding generic type description from "Generic Config Extender" to "Generic Role Extender"
- Removed JVM arguments from Overrides terminology description (no longer supported)

### Security Documentation (`security.md`)

No changes in this release.

### Examples

#### Added
- Added comprehensive Role-level comments in `crd-base-example.yaml`:
  - Field inheritance explanation (Role → RoleGroup)
  - Override precedence documentation
- Added `roleConfig` section example with PodDisruptionBudget configuration
- Added detailed comments distinguishing `roleConfig` vs `config` sections

#### Changed
- Updated `crd-base-example.yaml` with concrete example values instead of type placeholders:
  - `gracefulShutdownTimeout`: `"30s"`
  - CPU resources: `min: "500m"`, `max: "1"` (Role), `min: "1"`, `max: "2"` (RoleGroup)
  - Memory resources: `limit: "2Gi"` (Role), `limit: "4Gi"` (RoleGroup)

#### Fixed
- Fixed typo in `crd-base-example.yaml`: `affnity` → `affinity`
- Fixed typo in `crd-base-example.yaml`: `StatefuleSets` → `StatefulSets`

### New Files

No new files in this release.

---

## [2025-02-21]

### Architecture Documentation (`architecture.md`)

#### Added
- Added module category table in Section 4 (Core Module Implementation) to organize 14 modules into 5 functional categories
- Added detailed Extension registration, lifecycle, and execution process documentation in Section 4.2
- Added comprehensive health check mechanism description in Section 4.8, including:
  - Check interval: 120 seconds
  - Timeout: 300 seconds
  - Failure handling strategy (Degraded status marking)
  - Controller error handling (no status modification on internal errors)
- Added safety protection mechanisms for orphaned resource cleanup in Section 4.4:
  - Pre-delete validation
  - Safe deletion order
  - PVC preservation by default
- Added concurrency conflict handling for orphaned resource cleanup:
  - Optimistic locking
  - Conflict resolution strategies
  - Status synchronization
- Enriched Section 5 (Design Patterns) with detailed explanations:
  - Interface Segregation Pattern (5.1)
  - Strategy Pattern (5.2)
  - Template Method Pattern (5.3)
  - Singleton Pattern (5.4)
  - Builder Pattern (5.5)
  - Adapter Pattern (5.6)
  - Observer Pattern (5.7)
  - Pattern Summary Table (5.8)

#### Changed
- Updated Kubernetes version requirement from 1.19+ to 1.31+
- Removed `Connection` terminology from Section 1.3 (Terminology Definition) as it's not an abstract concept

#### Fixed
- Unified zookeeper-related terminology to `zookeeperConfigMap` across all examples

### Security Documentation (`security.md`)

No changes in this release.

### Examples

#### Changed
- Updated `crd-hdfs-example.yaml`: Unified zookeeper-related field name
- Updated `crd-hive-example.yaml`: No changes
- Updated `crd-base-example.yaml`: No changes

### New Files

- Added `TODO.md` at project root to track pending issues and improvements
- Added `docs/architecture_zh.md` - Chinese version of architecture documentation
- Added `docs/DOC_CHANGELOG.md` - This changelog file

---

## Template for Future Entries

```markdown
## [YYYY-MM-DD]

### Architecture Documentation (`architecture.md`)

#### Added
- Item 1
- Item 2

#### Changed
- Item 1
- Item 2

#### Fixed
- Item 1
- Item 2

#### Removed
- Item 1

### Security Documentation (`security.md`)

#### Added/Changed/Fixed/Removed
- Items as applicable

### Examples

#### Added/Changed/Fixed/Removed
- Items as applicable

### New Files

- `path/to/new/file` - Description
```

---

## Legend

- **Added**: New features or content
- **Changed**: Modifications to existing features or content
- **Fixed**: Bug fixes or corrections
- **Removed**: Deprecated features or removed content
