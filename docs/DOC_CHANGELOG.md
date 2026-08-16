# Documentation Changelog

This document tracks all changes made to the SDK documentation.

---

## [2026-08-16] (declare / fold / derive — the role config default refactor, #631)

### Core architecture

- `docs/architecture.md` gains **§2.5b Four Concepts, Not One "Default"**, the design statement the
  framework was missing. #631 asked for "role config defaults"; the framework held three partial
  answers and no account of what it was answering, so each case was resolved by whichever the author
  found first. Names the four — **declaration** (no user layer), **default** (beneath the user),
  **derivation** (computed from the folded result), **constraint** (above the user) — and records
  that constraint is deliberately **not implemented**: a framework that overrules what a user wrote
  in their own CR has no honest way to tell them, so that rejection belongs in a webhook where it
  surfaces at `kubectl apply`.
- §2.5 rewrites the third merge layer. The product's defaults are `RoleDeclaration.ConfigDefaults`,
  folded by `FoldCommonConfig`, and `affinity` now folds **per member** rather than wholesale —
  the rule changed *because* the default layer arrived: with only the CR's two levels, both written
  by one person in one file, replacement was the simpler semantic; with a product default beneath
  them, replacement means a user adding a `nodeAffinity` silently deletes the anti-affinity the
  product ships to spread a quorum. Records why empty-clears works for `affinity` and not for
  `resources` (the former is `x-kubernetes-preserve-unknown-fields`, so a stored `{}` is always
  something the user wrote; the latter is structural, where `cpu: {}` may be a pruning artifact),
  and adds the product's half of the block — `FoldProductConfig[T]`, `ValidateProductConfigType[T]`
  and the `kubedoop:"atomic"` opt-out — since embedding `*RoleGroupConfigSpec` inline is the
  sanctioned way a product extends the type.
- §2.6 becomes **Derived Config vs. Defaulting**: `ProductConfig` is replaced by `RoleGroupResolver`
  returning a `Contribution`. The substantive change is *position in the pass* — it runs after the
  fold and before anything is built, which is the window the framework did not previously have. The
  effective config was not computed until after the role group's ConfigMap had been assembled, so
  nothing derived from it could reach a config file at all; that is what forced three operators to
  hand-write the same JVM-heap calculation and a fourth to freeze the answer into a literal
  `-Xmx419430k`. Records that `Contribution` carries no CLI dimension on purpose (`cliOverrides`
  merge by replacement, so a contributed layer is not a default in either direction) and that the
  resolver must be deterministic, since its output lands in a ConfigMap the framework applies and
  watches.
- §4.1.4 and §4.1.5 replace the handler-state tables. The hazard they documented — one handler
  instance serving every cluster — is now removed rather than described: the handler has no field
  for a role's ports, image or container name, so the assignment that raced cannot be written.
  `MainContainerCustomizer` is recorded as **removed**, with the reason: a callback is a channel
  whose precedence must be explained and enforced (it had to be blocked from changing the image),
  while a declaration field has no user layer to beat.
- §4.6 rewrites the Vector gates. One producer list now serves both jobs, and a producer opts out of
  config-file rendering by leaving its `Framework` empty — replacing a seam that worked only because
  Go has no virtual dispatch, which was discoverable by nobody and silently gave no pipeline to a
  product that overrode the wrong list. The obligations that seam left to the product are now
  enforced. Records that the resolved "is the pipeline active" answer stays **inside** the
  framework: every input to it is already the framework's, so a product sees only the conclusion it
  can act on, `LogFileTarget`.
- The `Warning` event vocabulary loses `UnknownConfiguredRole` and gains `UnusedRoleDeclaration`
  (§4.14, §7.1): "Configured" named the deleted setter world.
- The optional-capability list drops `RoleNameProvider` and `LoggingProducerProvider`.
  `RoleProvider` and `RoleGroupResolver` are explicit config fields, deliberately not type-asserted
  capabilities — a handler that implemented the old method on the wrong receiver disabled the whole
  Vector pipeline with nothing reporting it.

### AGENTS.md

- Root `AGENTS.md` §3 becomes **RoleProvider, RoleDeclaration and RoleGroupHandler**, §4 documents
  the build context's settled answers and the retired escape hatches, §9 documents the empty-
  `Framework` logging seam and `LogFileTarget`, §10 becomes **RoleGroupResolver**, and a new §4b
  documents config folding end to end. §2's flow gains the catalog step and the
  fold → resolve → derive → merge ordering inside the role group loop.
- `pkg/reconciler/AGENTS.md` gains file rows for `role_declaration.go`, `role_group_resolver.go`,
  `config_fold.go` and `resolved_image.go`.
- `examples/trino-operator` docs follow the example's own migration to `DeclareRoles`,
  `RoleGroupResolver` and `ImageResolution`.

---

## [2026-08-13] (the operator's own ClusterRole, and the two grants that do not announce themselves)

### Core architecture

- `docs/security.md` gains **§3.3 Operator RBAC**, the third axis alongside workload identity (§3.1)
  and workload RBAC (§3.2): the permissions the operator *process* must hold because
  `GenericReconciler` writes on its identity. Derived from the framework's own call sites rather
  than copied from the example, split into a baseline every operator needs and conditional grants
  with their exact triggers. The minimality rule it applies is stated explicitly, because the
  obvious one is wrong: **omit a verb only when omitting it actually removes a capability.** Two
  verbs are therefore withheld — `delete` on serviceaccounts, and `update`/`patch` on the CR body —
  while `patch` stays on kinds the framework only ever Updates, since next to `update` it grants
  nothing extra and the SDK exports a helper (`K8sUtil.Patch`) that needs it.
  Records why the SDK cannot declare any of this itself: controller-gen never walks a dependency's
  packages, so a marker in `pkg/` generates nothing anywhere. Old §3.3/§3.4 shift to §3.4/§3.5.
- §3.3.3 names what does not announce itself, which is the reason the section exists: `core/events`
  (client-go discards a 403 on an event with no retry and no error, so the pass reports success),
  `core/pods` (the health pass Lists through the cache, so a 403 stops `Degraded` being computed
  rather than reporting a fault), and the whole **cleanup** path, whose errors the reconciler logs
  and swallows — only a 429 is fatal — so a 403 on a teardown delete leaves the pass reporting
  success. Everything else fails loudly on the apply path. It also points at the exact-match RBAC
  spec as the one automated link between the published set and a deployed operator, rather than
  claiming no gate can catch this — the gate is the one this change adds.
- `docs/architecture.md` §4.14 gains **§4.14.3**, the events precondition, attached to the sentence
  in §4.14.4 that promised `kubectl describe` visibility unconditionally. Records that the loss is
  uneven: five of the six `Warning` events have a paired log line or condition, and
  `ImmutableFieldIgnored` has neither — so it is the one whose information exists only as an event,
  and losing it reintroduces the silent-storage-resize defect it was added for.
- `docs/architecture.md` §7.1's "Operator requires CRUD permissions for StatefulSet, Service,
  ConfigMap, etc." is replaced by a pointer to §3.3 rather than a second copy.
- `docs/security.md` §3.2 corrects the escalation escape hatch: the Role and RoleBinding writes are
  checked **separately**, with different bypass verbs (`escalate` on roles, `bind` on the referenced
  role), so `escalate` alone half-converges — the Role lands, the RoleBinding is refused, and the
  reconcile fails at step 0b on every pass. The same correction lands in
  `pkg/reconciler/workload_rbac.go`'s godoc, which claimed the failure was at Role create time.

- §3.3.1 gains **"Why a `Get` needs `list;watch`"**: the framework reads through the manager's cache,
  so a read of a kind with no informer lazily creates one — which LISTs and WATCHes. That is why
  `core/secrets` and the `s3` rows carry `list;watch` for code that only ever `Get`s, and tightening
  them to `get` is the one "obvious" correction that breaks an operator, at the first read rather
  than at boot. It also records the consequence worth weighing before granting: an informer is
  cluster-wide and unfiltered by default, so `core/secrets` caches every Secret in every namespace
  in the operator's memory — with `cache.Options.ByObject` as the way to scope it.
- §3.3.3's loud half is split by where the watch came from: an `Owns()` kind fails at **boot**
  (`WaitForCacheSync` takes `manager.Start` down), a lazily-created informer fails at the **first
  read**, mid-reconcile. The two were previously one undifferentiated "fails loudly".
- §3.2's "a pre-existing RoleBinding is never adopted" is corrected to "never **re-pointed**": one
  already pointing at *this* cluster's Role **is** adopted, which is the intended migration path off
  a hand-maintained binding — and adoption overwrites, replacing the subjects with the single derived
  ServiceAccount and taking the controller reference, so anything else that binding granted
  disappears. The godoc heading in `workload_rbac.go` follows.
- §3.3.2's two write rows (`core/secrets` for `EnsureGeneratedSecret`, and the `ExtraResources` row)
  gain `patch`, which §3.3.1's rule already required of them — both are `CreateOrUpdate` paths like
  the baseline. `persistentvolumeclaims` deliberately keeps none: it has no `update` either, so there
  `patch` would genuinely add the ability to modify a claim.
- §3.3.2's `roles;rolebindings` row now names the **second** obligation setting `WorkloadRBACRules`
  creates — the operator's ClusterRole must also be a superset of every rule the hook returns — and
  why it cannot be tabulated. That table is the copy-paste surface, and without the superset the
  operator 403s at step 0b on every pass.
- The eight-step "Building a New Operator" checklist (and `architecture.md` §7.2's parallel list)
  gains a step for declaring the operator's own `+kubebuilder:rbac` markers. Every other obligation
  in those lists announces itself when missed; this is the only one that partly does not, and it was
  the only one absent.

### Package guides

- Root `AGENTS.md` §11c resolves its own dangling pointer: it named the operator's ClusterRole as "a
  separate axis entirely" and pointed nowhere. It now points at §3.3 and names the two quiet grants.
- `pkg/reconciler/AGENTS.md`'s `event.go` row and the `GenericReconcilerConfig.Recorder` field doc
  both state the permission the recorder obliges, and what a 403 does.
- `examples/trino-operator`'s marker block is narrowed to the derived set and explains each grant;
  its generated `config/rbac/role.yaml` follows, pinned by an exact-match spec over the generated
  file. `make verify-generate` now actually covers it: the pathspec gained `*/config/rbac/*`, and
  the trailing `/*` is the whole fix — a git pathspec with a wildcard is matched against the full
  path with no directory expansion, so `*/config/rbac` matched nothing, exactly as the pre-existing
  `*/config/crd/bases` had been matching nothing since it was written. Both are corrected, so the
  examples module's generated CRD is covered for the first time too.

## [2026-08-10c] (a hook can wait without the cluster reporting a fault)

### Package guides

- Root `AGENTS.md` §5 documents `common.RequeueAfterError`, `common.WaitingErrors`, the `Waiting`
  condition, and the four properties that are load-bearing: the wait is reported at the END of the
  pass so health still runs (otherwise `Degraded` latches, since `SetDegraded(false, …)` has exactly
  one writer), an aggregate is a wait only when every leaf is, the delay is clamped (a non-positive
  `RequeueAfter` never requeues at all), and Reason/Message must be stable or the status write
  ping-pongs with nothing to back it off.
- Records the health change that goes with it: a role group absent from `status.roleGroups` has no
  StatefulSet because the framework has not applied one yet — `Creating`, not `Degraded`. A group in
  the ledger whose StatefulSet vanished is still a fault.

## [2026-08-10b] (a Job could not survive the framework's own extras channel)

### Package guides

- Root `AGENTS.md` §3 documents the `batchv1.Job` apply rule and why it is typed rather than generic:
  the API server generates `spec.selector` and injects UID-derived pod-template labels at creation,
  so the generic wholesale-`spec` copy made the SECOND reconcile of an unchanged Job fail. Records
  that create-once is the only semantics a Job has, and that re-running one means changing its name.

## [2026-08-10] (product config defaults reuse the CR's own merge)

### Core architecture

- `docs/architecture.md` §2.5 records why a product's role-config defaults fold through
  `MergeRoleGroupConfig` rather than a separate nil-fill pass: reusing the merge inherits its rules
  (resources per leaf, affinity wholesale) instead of inventing a second precedence users would have
  to learn, and it is what keeps `affinity: {}` meaning "no affinity" rather than "inherit".

### Package guides

- Root `AGENTS.md` §3 documents `SetRoleConfigDefaults`, `RoleGroupBuildContext.ConfigDefaults`,
  `DefaultAntiAffinity`, `EncodeAffinity`, `TopologyKeyHostname` and `SetRoleStorageMountPath` —
  including why the per-CR channel has to exist (an anti-affinity selector names the cluster, and the
  handler is shared) and why `logging` is rejected in a default.

## [2026-08-09d] (the image spec gains a validated tag and a pull secret)

### Core architecture

- `docs/architecture.md` §2.6 gains two paragraphs: why the assembled TAG is validated (the API
  server does not validate `container.image` at all, so an unparsable reference surfaces only as a
  kubelet `InvalidImageName` on a pod while the reconcile reports success), and why a registry
  credential is deliberately NOT part of image resolution — where an image lives is independent of
  how its reference was built, so the credential has to survive the two paths that assemble no
  reference at all.

### Package guides

- Root `AGENTS.md` §3 documents `spec.image.pullSecretName` and the tag validation, including what is
  exempt (`custom`, the user's verbatim reference) and how an override interacts with it (strategic
  merge patch keys `imagePullSecrets` by `name`, so a `podOverrides` entry adds rather than replaces).

## [2026-08-09c] (two claims corrected: what the apply path preserves, what a marker key identifies)

### Package guides

- `copyServiceState` gains a table recording which of its six carry-overs are load-bearing and which
  are defensive. The code reads as six interchangeable lines and only an experiment against a real
  API server tells them apart: four are restored by the API server anyway, while a port RENAME costs
  the allocated node port and `loadBalancerClass` makes the API server reject the whole update.
- `RoleGroupMarkerLabelKey`'s doc corrected. It said the key "marks a resource as belonging to one
  specific role group"; the natural form omits the ROLE, so it is not unique across roles sharing a
  group name. Now stated as a fingerprint that nothing may select on alone, with why the role cannot
  be added (the key lands in the immutable `.spec.selector`) and why the over-long substitute is
  role-scoped where the natural form is not.

## [2026-08-09] (the ExtraResources contract stops being documentation-only)

### Core architecture

- `docs/architecture.md` §"What comes back, and what a nil field means" gains the reasoning for
  keeping `ExtraResources` as `[]client.Object` and validating the entries instead: a narrower type
  cannot express "a kind the framework has no opinion about", but a name, the cluster's namespace and
  a distinct GVK+name identity all are expressible. The duplicate-identity rule is written down with
  its failure mode — two writers in one pass, the later winning silently, and an endless
  write/watch/reconcile cycle when their desired states differ — because that is the one the old
  contract could not catch anywhere.
- §5.3.3 (resource application order) now records why the extras position is **fixed** and carries no
  per-resource ordering control: the safety property that an orphan's extras are deleted immediately
  after its StatefulSet holds because there is exactly one position to invert.

### Package guides

- Root `AGENTS.md` §3 states the three checks, which failure each one moves earlier and which one it
  creates outright.

---

## [2026-08-09] (sidecar and logging seams that existed but were never written down)

### Core architecture

- `docs/architecture.md` §4.6.2 **Standard Implementations** listed two of the four shipped
  providers. `OAuth2ProxySidecarProvider` and `StaticContainerProvider` added;
  `JmxExporterSidecarProvider` corrected to `JMXExporterSidecarProvider` and its description
  corrected from "injects Prometheus JMX Exporter agent" to what it is — a separate container running
  `jmx_prometheus_httpserver.jar`. The java-agent mechanism is `constant.JMXJavaAgentOpt` (§4.1.5),
  and conflating the two is how a product ends up wiring neither.
- §4.6.2 said the manager "injects Containers". It injects **`InitContainers`** with
  `RestartPolicy: Always` — native sidecars (KEP-753; on by default since Kubernetes v1.29, GA in v1.33) — and the ordering that buys
  (started before, terminated after the main container) is the whole reason the pre-#441 shutdown-file
  handshake could be deleted. Both facts added, with the migration note that a product carrying
  shutdown-file commands should now delete them.
- §4.6.2 gate 2 now states that the producer list is read from the **outer** handler while config
  files are rendered from `BaseRoleGroupHandler.LoggingContainers`, and that this is the supported
  seam for a product-owned logging config file rather than an accident.
- §4.6.2 "Workflow" described producers as "container names"; they have been
  `[]productlogging.ContainerLogging` since the log-tag decoupling.

### Package guides

- Root `AGENTS.md` §9 gains the product-owned-config-file seam (override `LoggingProducers`, leave
  `LoggingContainers` empty) and the two obligations it transfers to the product.
- Root `AGENTS.md` §14 gains the native-sidecar paragraph (why init containers, and that the
  shutdown-file contract and its `pkg/util/bash.go` half were both removed in #441) and
  `sidecar.NewStaticContainerProvider` / `sidecar.SidecarRestartPolicy`, neither of which appeared in
  any `.md` before — which is why products kept asking for a framework provider per helper container.

## [2026-08-09] (a preserved field's contract, not just its value)

### Core architecture

- `docs/architecture.md` §5.3.3 gains the rule the data-PVC defect generalises to: *preserve the
  field's whole contract, or the object converges into a state neither the user nor the handler
  described.* `volumeClaimTemplates` is the only preserved field another part of the same object
  refers to, and preserving it alone produced a rejected Update in one direction and a silently
  unmounted PVC in the other.

### Package guides

- Root `AGENTS.md` §2 documents both transitions, what the apply path now does about them (drop a
  mount for a claim that was not created, restore a preserved claim's mount from the live template),
  and why an **empty** desired `volumeClaimTemplates` counts as a change request when an empty value
  for every other preserved field does not.
- `pkg/reconciler/AGENTS.md` records `reconcileClaimVolumeMounts` in the `apply.go` row.

## [2026-08-08] (workload identity and workload RBAC become framework concerns)

### Package guides

- Root `AGENTS.md` gains **§11c Workload Identity and Workload RBAC**, documenting the two halves as
  one thing: the derived ServiceAccount and the Role/RoleBinding built from
  `WorkloadRBACRules`, both settled per CR at the top of the reconcile. Previously the SA was two
  sentences inside the flow list and workload RBAC appeared only as "use the builders and ship them
  as ExtraResources".
- `pkg/builder/AGENTS.md`: the RBAC builders are no longer *the* route to workload RBAC; they are
  for objects the framework does not maintain.

### Security architecture

- `docs/security.md` §3.1 rewritten. It described an **opt-in** ServiceAccount whose name came from
  one of two config fields, then claimed in the next bullet that pods run under a specific
  application identity rather than `default` — true only for products that opted in, and the default
  was the opposite. Now states unconditional provisioning, the derived name
  (`ServiceAccountResourceName(kind, cluster)`) and why the Kind is in it.
- `docs/security.md` §3.2 rewritten from "Builders, not automation" to the
  `WorkloadRBACRules` contract: what the framework owns, why an empty rule set revokes, why a
  pre-existing `roleRef` is never adopted, the two distinct causes of a 403, and why the RBAC watches
  are conditional.

### Design rationale recorded

- The name of an object the framework creates, owns, and garbage-collects is not a product decision.
  §3's "the framework owns the NAME of every fixed slot" applied to ConfigMap/Service/StatefulSet/PDB
  — enforced to the point of failing a build — and the ServiceAccount was the lone exception, for no
  reason anyone could point at. Deriving it removes the shared-name failure mode by construction
  instead of detecting it and printing a paragraph telling the author to use the other field.
- An identity that can be switched off is an identity that will be off by accident. A ServiceAccount
  costs nothing, so the switch was removed rather than defaulted.
- Identity and permissions belong at the same level and in the same step. Workload RBAC as
  `ExtraResources` put a **cluster-level** concern on a **per-role-group** path, and made every
  product responsible for matching the RoleBinding's subject to the SA the framework had chosen. The
  framework now passes the name it derived, so that correspondence is not something a product can get
  wrong.
- Cluster-scoped RBAC is out of scope, and that is a lifecycle statement rather than a limitation: a
  namespaced CR cannot controller-own a cluster-scoped object, so there would be no garbage collection
  with the cluster and no ownership gate on the reclaim.
- The framework re-explains the API server's 403 rather than pre-checking it. A pre-check would have
  to reimplement RBAC rule covering — wildcards, `resourceNames`, aggregated ClusterRoles,
  non-resource URLs — and would then be wrong in both directions, while the server's own message has
  already done that computation against the operator's real effective permissions.

## [2026-08-07] (the log tag stops being the container name)

### Package guides

- Root `AGENTS.md` §9: the log path was documented as `<KubedoopLogDir>/<lowercased container>/…`
  with no mention that the directory segment *is* the Vector event's `container` field. Now states
  the coupling, what `LogDirName` decouples and — as importantly — what it does **not** move (the
  volume mount, the CRD key, the log-file base name), plus why an explicit value must be an RFC
  1123 label and why a producer naming no container is now a build failure rather than a silent
  skip.

### Design rationale recorded

- Which name each derived artifact follows is the whole design, and getting one on the wrong side
  is the defect: the appender directory, the Vector tag and the sidecar's `mkdir` follow the log
  dir; the volume mount, the CRD `logging.containers.<key>` and the default log-file base name
  follow the pod container. The CRD key must stay on the container because that map has to be able
  to address containers producing no log directory at all — a sidecar, an init container — which
  have no tag to key on.
- `vector.yaml` is deliberately untouched: its source globs are wildcard per framework
  (`<LogDir>*/*<suffix>`), not per container, so the tag follows the directory mechanically with no
  pipeline change. The issue's proposal to make the globs honour the override would have broken
  byte-identity for every product and stopped collecting undeclared containers' stdout.

---

## [2026-08-04] (slot identity: neither forgeable nor unaddressable)

### Core Architecture (`architecture.md`)

- **§4.1.5** now states the fixed-slot name and namespace contract as a rule with a reason, next to
  the "nil is an instruction to delete" paragraph it follows from: that instruction only works if
  the framework can find the object it refers to, and it finds all six fixed slots by derived name.
  Generalised to the design rule — *an optional, product-supplied resource slot must have either a
  framework-owned name or a framework-stamped identity, never neither* — with why the fixed slots
  take the first branch (a name is checkable at build time, an identity label only against a live
  List whose stale answer is terminal) and `ExtraResources` the second.
- Records that three CR label keys are withheld from `ClusterLabels`, and the principle behind it:
  a marker a user can set is a delete instruction a user can forge. Notes why the filter is an
  enumerated set rather than a `kubedoop.dev` prefix rule.
- The non-embedding-handler checklist item 1 was "the headless Service must be named
  `<ResourceName>-headless`" as a convention; it is now checked and rejected.

### Package guides

- `pkg/reconciler/AGENTS.md`: new §18 on slot names (which names, why pre-flight rather than
  per-step, why a hard failure); §16 gains the reserved-label paragraph with the concrete
  `pdb.kubedoop.dev/role` failure it prevents.
- `pkg/builder/AGENTS.md`: `WithAnnotations` semantics, and why `MetricsServiceBuilder` exposes no
  name or ClusterIP override.
- Root `AGENTS.md`: the contract in §3, the reserved keys next to the restarter-label passage, and
  a pointer from the "Validation failures are loud" paragraph.

---

## [2026-08-03e] (the handler contract stops being source-only)

### Core Architecture (`architecture.md`)

- New **§4.1.5 Writing a Role Group Handler**, the section #579 asks for: the build-context
  read/write split (12 fields the framework writes, 9 the handler does), what a nil
  `RoleGroupResources` field *instructs* rather than omits, the container contract (name resolution
  before `Build()`, the `Ports[0]` readiness probe, the reserved `config`/`data` volume names,
  `JvmArgs` reaching nothing), the read-only config mount, the eleven ways to fail a build, and a
  four-item checklist for a handler that does not embed `BaseRoleGroupHandler`.
- **Three doc-vs-code corrections found while writing it.** §4.11.2 attributed the `stopped`
  replica forcing to the Reconciler; it is in `BaseRoleGroupHandler.buildStatefulSet`, which is
  exactly what a non-embedding handler has to reproduce. The terminology section said a RoleGroup
  maps to a StatefulSet "and associated Service, ConfigMap, PDB" — the PDB is **role**-level.
- Records that `BaseRoleGroupHandler.WithSidecarManager` is **inert under the reconciler**, which
  always supplies a non-nil manager on the build context.

### Framework Documentation (`AGENTS.md`)

- New §9b for the two image conventions: the JMX java agent and the read-only config mount, both
  with the reason the obvious simplification is wrong (per-role JMX configs; `cp -RL` and the
  `..data/` symlink farm).

### Go doc comments

- `BaseRoleGroupHandler.ConfigMountPath` states the mount is read-only and what that costs.
- `RoleGroupHandler.BuildResources`'s stale "5. Build PDB if needed" is deliberately NOT touched
  here: PR #592 already fixes it, and duplicating the fix would only produce a conflict. §4.1.5
  carries the part that PR does not — that a nil `RoleGroupResources` field is an instruction to
  delete rather than an omission.

---

## [2026-08-03d] (the product-config seam can read the cluster it documents)

### Core Architecture (`architecture.md`)

- §2.6 records that `ProductConfig` now receives a `ctx` and a client and may fail — "recomputed
  every reconcile, and may reflect the current state of the cluster" was only true if the hook could
  *read* the cluster — and that zero adoption was the symptom rather than the disease.
- Documents `ApplyProductDefaults` as the imperative counterpart for products that already perform
  the lookup inside `BuildResources`.

### Framework Documentation (`AGENTS.md`)

- §10 carries the new signature, both entry points, and why the env-var precedence rule two
  operators hand-wrote needs no ordering dance here.

---

## [2026-08-03c] (declaring intent before Build, instead of patching after)

### Core Architecture (`architecture.md`)

- §4.1.4 gains the second half of the build-context story: `MainContainerCustomizer` and
  `ListenerClass` replace the post-build patch, with a table of what each one replaces and why the
  *timing* is the point rather than the hook's existence.

### Framework Documentation (`AGENTS.md`)

- §4 documents both fields, the pre-`podOverrides` ordering, the rejection of an image change, and
  where `listener.ServiceTypeFor` lives and why.

### API doc correction (`pkg/listener`)

- `ListenerClassExternalUnstable`'s comment said "creates LoadBalancer with dynamic IPs". It is a
  **NodePort** — the LoadBalancer is the *stable* class. The wrong wording is the documented reason
  two downstream operators reached opposite conclusions about `external-stable`.

---

## [2026-08-03b] (an object whose content must not converge)

### Core Architecture (`architecture.md`)

- New **§4.9.4 Generate-Once Secrets**: why the SDK's usual "rebuild and overwrite" shape is wrong
  for a generated value, what `EnsureGeneratedSecret` guarantees, why filling a *missing* key is a
  deliberate choice, and why the Secret is not created from a sidecar provider's `Validate`.

### Framework Documentation (`AGENTS.md`, `pkg/reconciler/AGENTS.md`)

- New §11b next to the discovery-ConfigMap section, with the call and the oauth2-proxy case.
- `generated_secret.go` added to the reconciler file table.

---

## [2026-08-03a] (handler lifetime, and where per-CR inputs go)

### Core Architecture (`architecture.md`)

- New **§4.1.4 Handler Lifetime and Per-CR Inputs**: one handler instance serves every cluster, a
  table of what belongs on the handler versus the build context, and the two failure modes of
  getting it wrong — the race above `MaxConcurrentReconciles: 1`, and the quieter stale-value leak
  at concurrency 1 that spark-k8s-operator actually shipped.
- Records that `BuildResources` is read-only on the handler, and that the framework's own instance
  of the hazard (a handler-registered `SidecarManager`, whose configs `SetProductImage` writes into)
  is now cloned per build.
- Answers the first of the three gaps #579 names.

### Framework Documentation (`AGENTS.md`)

- §4 documents the per-CR input fields on `RoleGroupBuildContext`, with the example call and the
  reason the older handler-mutating idiom is unsafe.

---

## [2026-08-02d] (image resolution moves to reconcile time)

### Core Architecture (`architecture.md`)

- §2.6 now carries the case that shows why the webhook/reconcile split matters, and that image
  resolution was on the wrong side of it: the `-kubedoop<version>` suffix's natural value is the
  operator's own build version, which a webhook freezes into the spec at admission. Documents the
  two-layer fold (user `spec.image` over `handler.ImageDefaults`), that `ProductName` no longer
  gates whether `spec.image` is read, and that an unresolvable image is now an error rather than a
  silent fall back.
- Closes the gap #569 named: `ProductName`, `ImageDefaults` and the resolution precedence had **no**
  mention in the authoritative document at all.

### Framework Documentation (`AGENTS.md`)

- §3 replaces the "ProductName opts a handler into CR-driven images" sentence with the two fields
  and their precedence, the reason `ImageDefaults` cannot be a webhook's job, and the
  ProductName-less path that never errors.

---

## [2026-08-02c] (the no-CRD-default-inside-config rule becomes checkable)

### Core Architecture (`architecture.md`)

- The no-CRD-default rule is now stated as a **constraint on product operators**, not only as an
  explanation of the SDK's own field shapes, with the evidence that documentation alone did not
  hold: the rule has been written here since #544, and trino-operator carries
  `+kubebuilder:default:="5GB"` inside `config` today.
- Records the executable form (`testutil.HaveNoInheritedConfigDefaults`), that it applies **no depth
  heuristic**, that roles are detected structurally so flattened-role CRDs are covered, and that an
  argument matching no files is an error rather than a pass.

### Framework Documentation (`AGENTS.md`, `pkg/AGENTS.md`)

- Testing section documents the exported guard and the three-line product-side usage.

---

## [2026-08-02b] (log levels inherit; the CRD default that stopped them is gone)

### Core Architecture (`architecture.md`)

- The `config` merge-granularity table now states that `logging` folds **per level** inside a
  container: an entry naming `console`, `file` or a logger without stating a level means "inherit",
  not "clear".
- The no-CRD-default rule under that table gains the case that proves it is easy to satisfy in one
  place and forget in another — `LogLevelSpec.Level` kept `+kubebuilder:default:="INFO"` long after
  `resources` was fixed, and it was not inert. Records the transferable lesson: **a guard against a
  defaulted field must be verified through the API server**, because a Go-constructed spec never
  meets structural defaulting, which is why the unit test covering that guard passed throughout.

### Framework Documentation (`AGENTS.md`)

- §9 states the inheritance rule and why the field carries no CRD default.

---

## [2026-08-02a] (pkg/s3 pathStyle: the default, and what adopting S3AProperties changes)

### Core Architecture (`architecture.md`)

- §4.12.2 gains the `pathStyle` adoption note: the CRD default is `false` (virtual-host
  addressing), MinIO serves path-style **only**, and every product implementation
  `S3AProperties()` replaces pinned the key to `true`. A product migrating onto the helper
  therefore flips the addressing mode for every existing cluster whose `S3Connection` omits
  `pathStyle: true` — and the failure lands at first bucket access, not at admission. Also records
  why the helper honours the field instead of pinning it.

### CRD Examples (`docs/examples/crd-hive-example.yaml`)

- The commented inline `s3` block now shows `pathStyle` with the MinIO caveat, so someone copying
  the example meets the field rather than inheriting the default unseen.

### Framework Documentation (`pkg/AGENTS.md`)

- The `pkg/s3` bullet states the default and the adoption consequence.

### API and package docs

- `s3v1alpha1.S3ConnectionSpec.PathStyle` had **no doc comment at all**, so `kubectl explain
  s3connection.spec.pathStyle` printed nothing. It now explains both addressing styles and names
  the backends that need `true`. This changes the generated CRD description in downstream repos on
  their next bump.
- `s3.ConnectionInfo.S3AProperties` carries the adoption note as a godoc section, where a product
  author reaches it at the call site.

---

## [2026-08-01c] (the Chinese architecture document is removed)

### Core Architecture

- **`docs/architecture_zh.md` is deleted.** `docs/architecture.md` is now the single architecture
  document.
- The reason is the one the previous entry documents: a second copy of an authoritative design
  document is only authoritative while it is current, and it was not. Its §4.8.2/§4.8.3 spent an
  entire review round describing a status model the code had already replaced — `Degraded` derived
  from replica counts, no `Paused` condition — so anyone reading it was being told the framework
  behaves in a way it does not. Every change to the design now costs one edit instead of two, and
  cannot half-land.
- Historical entries in this changelog still name `architecture_zh.md`. They are left as written:
  they record what was true when each change was made, and rewriting them would make this file
  wrong in the same way the translation was.

### Framework Documentation (`AGENTS.md`)

- Removed the file from the Documentation Structure table and from the directory tree.

Note: `README_zh-CN.md` at the repository root is unaffected — a README is a short entry point that
does not drift the way a 116 KB design document does.

---

## [2026-08-01b] (framework metrics, and the Chinese doc catches up on status conditions)

### Core Architecture (`architecture.md`, `architecture_zh.md`)

- New **§4.8.5 Framework Metrics**: the two Prometheus series the SDK exports for the orphan cleanup
  state machine, why the gauge is written at zero, why a deleted CR's series are removed rather than
  zeroed, and — at least as important — why the list stops there (controller-runtime already
  publishes the reconcile metrics; kube-state-metrics already publishes CR conditions).
- **`architecture_zh.md` §4.8.2/§4.8.3 were still describing the pre-R3-4 status model** — `Degraded`
  derived from replica counts, no `Paused` condition, `Available` as "at least one replica ready".
  They contradicted the code and their own English counterpart. Translated the three-question
  condition table, the state-not-time rationale, the `>=` comparison, the pod-failure pass and the
  `Paused` condition.
- Both languages: the §4.11.2 ClusterOperation entry still said `reconciliationPaused` surfaces a
  `ReconciliationPaused` (Degraded) condition, contradicting §4.8 in the same document; and the
  §4.8.4 requeue list still said the paused path does not requeue. Both now match R3-4 — the
  dedicated `Paused` condition with `Degraded=False`, and `RequeueAfter: HealthCheckInterval` so the
  health conditions keep up with reality during a maintenance window.

### Framework Documentation (`AGENTS.md`, `pkg/reconciler/AGENTS.md`)

- `pkg/reconciler/AGENTS.md`: `metrics.go` added to the file table; new working instruction 17
  covering both series, the label set, `forgetClusterMetrics`, and the instruction not to grow the
  file into a second copy of tools that already exist.
- `AGENTS.md` §3: `resources.storage.storageClass` is a `*string`, and why `""` cannot mean unset.

---

## [2026-08-01a] (a removed role group's product extras are reclaimed)

### Core Architecture (`architecture.md`)

- §4.x's apply-order list now states the teardown counterpart: extras are deleted immediately after
  the StatefulSet, mirroring their creation before it, so nothing a pod might still need is
  reclaimed while a pod could still exist. Recorded the discovery rule (role group labels **and**
  controller ownership, over the kinds declared in `ExtraOwns`) and that unlabelled extras are
  undiscoverable in principle.

### Framework Documentation (`AGENTS.md`, `pkg/reconciler/AGENTS.md`)

- Corrected the standing claim that "the cleaner does not discover arbitrary-GVK extras", in §3 and
  on `RoleGroupResources.ExtraResources` itself.
- §13 now documents that `ExtraOwns` does double duty — watches and cleanup — why the two are
  derived from one list rather than declared twice, and that `ControllerBuilder` callers do not get
  the wiring and can call `WithExtraResourceKinds` themselves.

---

## [2026-07-31a] (the irreversible teardown step goes last)

### Core Architecture (`architecture.md`, `architecture_zh.md`)

- §4.4.3's PVC handling carried the same false justification as the code — "before the scale-to-0 so
  the selector is still meaningful" — for an ordering the implementation never needed. Replaced with
  the rule it should have stated: **the irreversible step goes last**, after the drain, because
  deleting a role group is undoable right up until its data goes. Written as a constraint on any
  future teardown step rather than a detail of this one, plus why PVCs still precede the StatefulSet
  (the cleaner reaches them through its selector) and why the drain-timeout path falls through.

### Framework Documentation (`AGENTS.md`, `pkg/reconciler/AGENTS.md`)

- The teardown order now shows the PVC step and where it sits, with the same reasoning.

---

## [2026-07-30b] (schema-free config must be decoded strictly; affinity replaces, it does not merge)

### Core Architecture (`architecture.md`, `architecture_zh.md`)

- §2.5 gained the rule the typed `config` block folds by, as a design constraint rather than an
  implementation note: **the finest granularity at which the result still means what both authors
  said**. Added the per-field table with the reason each granularity is not coarser *or* finer, and
  the reason `affinity` is the deliberate exception that replaces wholesale — it is one policy, not
  a set of knobs, and per-member inheritance would leave no way to express "no affinity".
- Added the general rule for schema-free fields: anything the SDK accepts as opaque JSON and then
  interprets must reject unknown members loudly, because with
  `x-kubernetes-preserve-unknown-fields` the API server validates nothing and the SDK is the last
  layer that can.

### Framework Documentation (`AGENTS.md`)

- Documented `reconciler.DecodeAffinity`'s strict decode, the concrete failure it prevents
  (`nodeAffinty` passing admission and evaporating), the accepted trade-off for newer-Kubernetes
  fields, and the previously undocumented wholesale replacement of `config.affinity` including that
  `{}` clears it.

---

## [2026-07-30a] (Degraded is a fault signal, not a progress signal)

### Core Architecture (`architecture.md`)

- §4.8.2 now opens with the design constraint the implementation had drifted from: the three workload
  conditions answer three different questions (`Available` = can it serve, `Progressing` = is it
  changing, `Degraded` = must a human look) and **none may be derived from another**. Added the table,
  the reason (a signal that fires on every planned change is unalertable), and the state-based inputs
  Degraded now uses — including why state-based detection catches a *stuck* rollout without any
  progress-deadline machinery.
- Recorded that ClusterOperation states are not faults: `stopped` and the new `Paused` condition both
  carry `Degraded=False`, and a paused cluster is still *observed* (the pause freezes resources, not
  reporting) with `ServiceHealthy` going `Unknown` rather than stale.
- §4.8.3 condition list updated: `Available` restated as "every role group has at least as many ready
  replicas as its spec asks for", `Degraded` restated as a fault signal, `Paused` added.

### Framework Documentation (`AGENTS.md`, `pkg/reconciler/AGENTS.md`)

- Documented the same split, the `>=` fix for `Available`, the single pod `List` behind `Degraded`,
  the excluded transient/terminating states, and the paused-path behaviour including the
  `RequeueAfter` that keeps a paused cluster's conditions current.

---

## [2026-07-29a] (the framework probes only what it owns)

### Framework Documentation (`AGENTS.md`, `pkg/builder/AGENTS.md`)

- Recorded that `StatefulSetBuilder` generates a readiness probe and never a liveness probe, and
  why the two are treated differently: a liveness probe on a guessed port kills the container on a
  timer, a readiness probe on the same guess only holds the pod out of its Services. Documented the
  consequence for callers — the FIRST entry of `SetRoleContainerPorts`/`WithPorts` is now part of
  the contract — and `builder.DefaultTCPLivenessProbe` as the one-line way back.
- Marked the contrast with the sidecar probes (§14) explicitly: the framework authors probes for
  containers it owns and declines to guess for containers the product owns.
- Documented `WithPodManagementPolicy` / `WithUpdateStrategy`, that neither field is reachable
  through `podOverrides`, and the reason the `Parallel` default is a choice: `OrderedReady`
  deadlocks a quorum product at pod-0.

---

## [2026-07-28b] (fsGroup no longer recurses the whole volume on every start)

### Security Documentation (`security.md`)

- Added `fsGroupChangePolicy: OnRootMismatch` to the default pod SecurityContext table, with the
  reason it is paired with `fsGroup` at all: unset means Kubernetes' `Always`, i.e. a full
  chown/chmod walk of the data volume before the container starts, on every start. Recorded the
  deliberate trade-off (drift *inside* a volume with a correct root is not repaired), the
  `podOverrides` escape hatch, and that the policy does not apply to ephemeral volume types.

---

## [2026-07-28a] (identifiers derived from user names are bounded and validated)

### Framework Documentation (`AGENTS.md`, `pkg/reconciler/AGENTS.md`)

- Documented that the role group marker label key goes through `RoleGroupMarkerLabelKey` rather
  than being concatenated: `<cluster>-<group>` is a label *key* built from two free-form user
  strings and overruns the 63-byte limit with ordinary names, at which point the API server rejects
  the whole role group. Recorded why the natural form has to be preserved wherever it is legal —
  the key sits inside the immutable `.spec.selector`, so only combinations that could never have
  produced a StatefulSet may change.
- Documented the new CRD constraint on role and role group names (lowercase RFC 1123 labels,
  enforced by CEL at admission) and the `maxProperties` bounds that exist only so the CEL cost
  estimator can size the rule.

---

## [2026-07-27f] (scope names cannot carry the annotation's own syntax)

### Security Documentation (`security.md`)

- The "Named scopes must carry a name" section covered the empty-name case but not the opposite
  one: a name containing `,` or `=` does not quote itself, it **adds scopes**. Added the rule, the
  concrete escalation (`services: ["mysvc,node"]` yields a node-scoped certificate nobody asked
  for), and the two layers that now stop it — CRD `items:Pattern` at admission, and `ScopeString`
  dropping unrenderable entries for what admission cannot reach.

---

## [2026-07-27e] (event vocabulary; clusterConfig is product-owned)

Two claims the docs made that the code does not support. Both were checked by enumerating the
actual call sites rather than by reading the prose.

### Architecture Documentation (`architecture.md`, `architecture_zh.md`)

- **§4.14.2 promised reconcile start/completion events that do not exist.** Nothing is emitted at
  the beginning or end of a pass; an SRE alerting on a documented "reconcile completed" event would
  have had an alert that is permanently firing or permanently silent depending on how it was
  written, and the silence would read as an operator outage. The clause is replaced with an
  explicit statement that progress is reported through **status conditions**, plus the complete
  list of the nine events the framework really emits.
- **§4.14.2 said the `EventManager` is "injected into the Reconciler context".** It is a struct
  field, handed to the cleaner; nothing puts it in a `context`. Corrected, with what a hook should
  do instead.
- Documented that resource events name the object's Kind and why that needs the scheme.

### Examples (`docs/examples/crd-base-example.yaml`)

- **`clusterConfig` was presented under a "--- Standard SDK Fields ---" heading, but commons
  defines no `ClusterConfigSpec` and `GenericClusterSpec` has only `image`, `clusterOperation` and
  `roles`.** Five products each hand-roll the block with their own spelling of the same concepts,
  and the doc asserting otherwise meant each author believed they were implementing a shared
  contract. The block is now labelled product-owned and illustrative, with a pointer to the
  explicit APIs (`VectorAggregatorProvider`, `SecretProvisioner`, `ListenerProvisioner`) through
  which the SDK genuinely consumes those concepts.

### AGENTS.md files

- `pkg/reconciler/AGENTS.md`: the `event.go` row now carries the full emitted vocabulary, the
  scheme argument, and which helpers the framework never calls.

---

## [2026-07-27d] (podOverrides volumeMount merge key)

### AGENTS.md files

- Root `AGENTS.md`: new paragraph under "Validation failures are loud" documenting that strategic
  merge patch keys `volumeMounts` by **`mountPath`**, not by `name` — so a `podOverrides` mount at
  a framework-owned path replaces the framework's rather than joining it, silently when the
  override also declares its volume — and that this is now a build failure.
- `pkg/reconciler/AGENTS.md` §10 and `pkg/builder/AGENTS.md`: the same, plus
  `StatefulSetBuilder.PodOverrideViolations()`.

---

## [2026-07-27c] (orphan discovery reads the live cluster)

### Architecture Documentation (`architecture.md`, `architecture_zh.md`)

- **§4.4.2 steps 2-3 described the status ledger as the only inventory.** Rewritten: the actual
  role group list is the union of a live-cluster query and `Status.RoleGroups`, with the four
  conditions a live object must satisfy and why each is required (the discovery ConfigMap and a
  product's `ExtraResources` both survive labels-and-ownership alone; only the derived name
  identifies the framework's slot), why both ConfigMaps and StatefulSets are listed, and why an
  empty owner UID disables the live half.

### AGENTS.md files

- Root `AGENTS.md`: new paragraph on live orphan discovery ahead of the state-machine one.
- `pkg/reconciler/AGENTS.md`: new §12 for the two-source discovery; §11's "pruning is terminal —
  nothing enumerates it afterwards" claim corrected, since the live inventory now does. Subsequent
  sections renumbered (§13-§16).

---

## [2026-07-27b] (one label channel: ExtraLabels/ExtraAnnotations removed)

### AGENTS.md files

- Root `AGENTS.md` §3: the **Labels** paragraph now states that the cluster CR is the framework's
  only label channel, that CR labels are applied *first* so a colliding key is inert rather than an
  error, and that the framework sets **no** annotations — with the reason the CR's annotations are
  not propagated the way its labels are (`kubectl.kubernetes.io/last-applied-configuration` and the
  cleaner's `orphan.zncdata.dev/*` markers live there) and a pointer to
  zncdatadev/operator-go#553 for the Service-annotation gap that leaves open.
- `pkg/reconciler/AGENTS.md` §13 rewritten: the `ExtraLabels` selector-collision *rejection* is
  replaced by the ordering that makes the collision impossible, and the old rule is kept as the
  explanation of why a check was once needed. New §15 documents the single channel and the
  annotation gap.

---

## [2026-07-27] (restarter contract re-verified against commons-operator; recommended label set)

The restarter opt-in was documented from the SDK's side only, and got the audience wrong. Every
claim below was re-read against `commons-operator/internal/controller/restart/statefulset_controller.go`.

### Architecture Documentation (`architecture.md`, `architecture_zh.md`)

- **§2.6 said "a ConfigMap or Secret mounted as a volume".** The restarter also follows env-var
  `valueFrom` references (`getRefSecretRefs` / `getRefConfigMapRefs`), so a Secret reaching the pod
  only as an env var is covered too. Both language versions updated.
- **§2.6 left "labelling the workload" ambiguous about *who* labels it.** The restarter's watch
  predicate and its `client.MatchingLabels` list both read **object metadata**, so the label belongs
  on the cluster CR (whose labels the reconciler propagates into every built resource's metadata) —
  a deployment decision, not an operator-code one.

### Security Documentation (`security.md`)

- The note on `restarter.kubedoop.dev/enable` told products to add the label "e.g. through
  `BaseRoleGroupHandler.ExtraLabels`". That is the wrong layer: `ExtraLabels` is set when the
  operator is *compiled*, while enabling restarts is decided when a cluster is *deployed*. Replaced
  with the CR-label channel, plus why `podOverrides` cannot substitute (it reaches the pod template,
  the restarter reads object metadata).

### AGENTS.md files

- Root `AGENTS.md`: the same `ExtraLabels` correction; added the one-rollout cost of enabling the
  label (the annotation value is `<uid>/<resourceVersion>`, so the first stamp is itself a template
  change) and the upstream single-ConfigMap bug (zncdatadev/commons-operator#298). Added a **Labels**
  paragraph to §3 and the `RoleBuildContext` signature of `BuildRolePodDisruptionBudget`.
- `pkg/reconciler/AGENTS.md`: new §14 tabulating the recommended label set, the conditions under
  which `name`/`version` are omitted, and why `version`/`role-group` must stay out of the immutable
  `.spec.selector`.

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
