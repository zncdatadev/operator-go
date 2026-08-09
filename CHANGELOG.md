<!-- markdownlint-disable -->
# CHANGELOG

## v0.13.0 (unreleased)

Architecture review follow-up: four waves of correctness fixes across the reconcile loop, the
builders, config/logging rendering and the CSI wiring, followed by three API redesigns that shrink
the contracts a product operator implements. **Downstream operators must migrate — the breaking
changes are listed below.**

### fix (data PVC transitions)

- **Adding `config.resources.storage` to a live role group no longer wedges it, and removing it no
  longer unmounts the data silently.** `volumeClaimTemplates` is immutable and the apply path
  preserves it — correctly — but the pod template it belongs to came from the handler, so the two
  disagreed on every transition:
  - **Add**: the template mounted a claim the framework had just declined to create —
    `volumeMounts[0].name: Not found: "data"`, a field the user never wrote. On Kubernetes 1.34+ the
    API server rejects that Update outright and the role group stays `Degraded` on every subsequent
    pass, recoverable only by deleting the StatefulSet by hand; on older servers the Update is
    accepted and every **pod** the StatefulSet controller creates is rejected instead, so the
    workload never progresses and the error never reaches the cluster's status.
  - **Remove**: this one was *accepted*. The claim template stayed (immutable), the mount did not,
    and the pods rolled into a product writing to the container's writable layer while its bound PVCs
    sat there mounted nowhere — with no event, no condition and no log line.

  `copyStatefulSetState` now reconciles the mounts against the claim templates that actually
  survived: a mount whose claim was not created is dropped (unless an ordinary volume of that name
  backs it), and **every** mount a preserved claim had is restored **from the live template**, so the
  mount path is read rather than invented. The restore is keyed on the mount *path* — Kubernetes lets
  one volume be mounted several times, so keying on the volume name would stop after the first and
  drop the rest, and a path the desired template already uses wins because two mounts at one path is
  a pod spec the API server rejects. A rename (`data` → `data2`) hits both branches and lands on the
  preserved claim. This makes the transition converge and stay coherent; it does not make the
  immutable change happen, and `spec.volumeClaimTemplates` is still reported as ignored.
- **Removing storage is now reported at all.** The `ImmutableFieldIgnored` check carried a
  `len(desired) > 0` guard, so the direction that *emptied* `volumeClaimTemplates` was the one
  direction nobody was told about. That guard is right for every other preserved field — an unset
  field is the handler declining to have an opinion — and wrong for this one, where an empty list is
  the handler stating that this role group has no storage.
- **`ImmutableFieldIgnored` is emitted before the write's error is returned.** The mutate func has
  already run by then, so what the framework refused to change is known even when the Update failed —
  and a rejected Update is exactly when the user most needs to be told which of their changes had
  already been dropped. Emitting after the return meant the one event that explains the situation was
  the one event that never fired.

### docs (sidecar and logging seams)

- **The seam that lets a product own its logging config file is now documented and pinned by a
  spec.** The reconciler reads the Vector producer list off the **outer** handler
  (`r.roleGroupHandler`, through the `LoggingProducerProvider` assertion) while
  `BaseRoleGroupHandler` renders config files from its own `LoggingContainers` field. Go has no
  virtual dispatch, so a base handler can never see an override — which means **overriding
  `LoggingProducers` and leaving `LoggingContainers` empty** joins a container to the Vector pipeline
  with no framework-rendered file and no ConfigMap key to collide with the product's own. That is
  what Airflow's `log_config.py` needs (it has to extend Airflow's `DEFAULT_LOGGING_CONFIG`, so it can
  never be a rendered template), and it already worked — it was just undocumented and unpinned, so
  the obvious "read one list" cleanup would have removed it silently. No new API: overriding the
  method *is* the API. Documented on the `LoggingContainers` field, `LoggingProducers`,
  `buildSidecarManager`, `AGENTS.md` §9/§14 and `docs/architecture.md` §4.6.2, with a regression spec
  in `pkg/reconciler` that fails if either half is rewired.
- **`sidecar.NewStaticContainerProvider` and `sidecar.SidecarRestartPolicy` are documented for the
  first time.** Both have existed since the architecture redesign with zero mentions in any `.md`,
  which is why products kept asking for a framework provider per helper container. The static
  provider deliberately ignores `SidecarConfig` — no `SetProductImage`, no `DefaultSecurityContext()`,
  no `ApplyProbes` — so the product must set the image, the security context and
  `RestartPolicy: sidecar.SidecarRestartPolicy()` on the container it passes.
- `docs/architecture.md` §4.6.2 listed **two** of the four shipped providers, said the manager injects
  "Containers" (it injects `InitContainers` with `RestartPolicy: Always` — native sidecars, KEP-753),
  misspelled `JMXExporterSidecarProvider` and called it a java *agent* (it is a separate container
  running `jmx_prometheus_httpserver.jar`; the agent is `constant.JMXJavaAgentOpt`, a different
  mechanism), and described producers as "container names" though they have been
  `[]productlogging.ContainerLogging` since the log-tag decoupling. All corrected.

### docs (removals recorded late)

- **The Vector shutdown-file handshake was retired in #441 and is recorded only now.** Before it, the
  Vector container ran a shell that backgrounded the agent and blocked on `inotifywait` for a
  shutdown file, and the product's main container was expected to `touch` that file on exit. Both
  halves went in the same commit — the watcher (`pkg/builder/vector.go`) and the writer helpers
  (`pkg/util/bash.go`: `CommonBashTrapFunctions`, `CreateVectorShutdownFileCommand`,
  `RemoveVectorShutdownFileCommand`) — because native sidecars supersede the mechanism: the kubelet
  terminates a `RestartPolicy: Always` init container **after** the main container, which is the
  ordering the handshake approximated. **Migration**: delete the shutdown-file commands from your
  operator; there is no framework helper to replace them and nothing reads the file. The old design
  was also strictly worse in one case, firing whenever the main process exited — including a crash
  the kubelet was about to restart, which told the agent to shut down.
- The same commit removed two things that are **not** part of that handshake and have no replacement:
  `util.ExportPodAddress` (also in `pkg/util/bash.go`) and all of `pkg/util/code.go`
  (`IndentTab4Spaces` and its siblings). They are recorded here as removals; whether the framework
  should carry generic shell/indent helpers at all is a separate question, not something the
  native-sidecar change decided.

### BREAKING (workload ServiceAccount)

- **The workload ServiceAccount's name is now DERIVED from the CR, and the two config fields that
  chose it are removed.** `GenericReconcilerConfig.ServiceAccountName` and
  `ServiceAccountNameFunc` are gone; the name is
  `reconciler.ServiceAccountResourceName(kind, cluster)` = `"<lowercased kind>-<cluster>"`
  (`hdfscluster-prod`), exported because IAM trust policies and hand-written RoleBindings have to
  name it. **Migration**: delete both fields from your `GenericReconcilerConfig`. If your operator
  set them, its pods move to the derived ServiceAccount on the next reconcile — a rolling restart —
  and the old SA is left behind for you to delete (it is controller-owned by the CR, so it also goes
  on cluster deletion).

  This follows the framework's own rule that it owns the name of every slot it owns the lifecycle of
  (§3): it creates the SA, controller-owns it and GCs it with the CR, so nothing needed to address
  it by a product-chosen name. Making it derived also makes a whole failure class unrepresentable
  rather than merely diagnosed — a static name was a *constant*, so every CR of a product in one
  namespace resolved to the SAME ServiceAccount: whichever cluster reconciled second failed with
  `AlreadyOwnedError` forever, and deleting the first garbage-collected the SA out from under the
  second's running pods. The Kind is in the derived name because a CR name alone is not unique in a
  namespace — an `HdfsCluster` and a `TrinoCluster` both named `prod` would otherwise reintroduce the
  same collision one level down.
- **Every cluster now gets a ServiceAccount; there is no way to skip it.** Previously, leaving both
  fields unset skipped SA management entirely and the pods ran as the namespace `default` — the
  opposite of what `docs/security.md` §3.1 claims the framework guarantees ("audit logs reflect the
  specific application identity rather than a generic default account"). A ServiceAccount is pure
  metadata, so the switch bought nothing and mostly fired by accident.
- **A 429 from the ServiceAccount step now backs off instead of degrading the cluster.**
  `ensureServiceAccount` returned the raw API error rather than routing it through the same
  translation `applyResource` uses, so throttling ran the product's error hooks, emitted a Warning
  and flipped `Degraded` on a cluster nobody had managed to read yet. While the SA was opt-in this
  branch was reachable only by the products that configured one; it is now on every cluster's
  critical path.

### features (workload RBAC)

- **`GenericReconcilerConfig.WorkloadRBACRules func(cr CR) []rbacv1.PolicyRule`** declares the
  Kubernetes API permissions a cluster's **pods** need — a separate axis from the operator's own
  ClusterRole. The framework maintains a namespaced `Role` and `RoleBinding` named after the derived
  workload ServiceAccount, in the CR's namespace, controller-owned by the CR (so both are
  garbage-collected with the cluster). It is the other half of the identity above, settled at the
  same point in the same way, from the same derived name — which is what keeps a workload's identity
  and its permissions from drifting apart. Products previously hand-built these objects and shipped
  them through `RoleGroupResources.ExtraResources`, which put a cluster-level concern on a
  per-role-group path and left the SA-name/subject correspondence to the product to get right.
- **`reconciler.EnsureWorkloadRBAC`** is the exported helper behind it (options:
  `WithWorkloadRBACProductName`, `WithWorkloadRBACExtraLabels`), for a product driving the
  reconciler's pieces itself. Prefer the config field: the helper takes the ServiceAccount name as a
  parameter, so a caller can pass one that is not the workload's — producing a Role that grants to a
  ServiceAccount no pod uses, with both objects present, the pods starting fine, and a 403 on the
  first API call.
- **An empty rule set revokes**, deleting both objects when this CR controller-owns them — the same
  reading every optional slot gets, and the only one under which a rule set shrinking to nothing
  actually stops granting. A **nil hook** is the distinct statement "this product never opted in" and
  touches no RBAC object at all.
- **A pre-existing `RoleBinding` with a different `roleRef` is never adopted.** `roleRef` is
  immutable, so it cannot be converged; the framework fails with a `*ValidationError` naming both
  refs and the fixing command, rather than rewriting only the subject — which would hand this
  cluster's pods whatever the old ref allows, and return success.
- **No `ClusterRole` path, deliberately**: a namespaced CR cannot controller-own a cluster-scoped
  object, so the framework would have no lifecycle for one.
- **The RBAC watches are registered only when the hook is set.** An unconditional
  `Owns(&rbacv1.Role{})` would force every operator built on this SDK to grant itself cluster-wide
  `list;watch` on roles and rolebindings, and a forbidden informer fails `WaitForCacheSync` for *all*
  sources — `manager.Start` returns and the process exits.
- Setting the hook requires the operator to hold those permissions itself (Kubernetes forbids
  granting what the granter lacks) **plus** write access to the RBAC API. A 403 from either cause is
  re-explained rather than pre-checked: the API server's message has already computed rule covering
  against the operator's real effective permissions and names the missing rule.

### fixes (role group slot identity)

- **A `RoleGroupResources` slot under a name the framework does not own is now a build failure**
  instead of a permanent leak (#584). The six fixed slots must be named `<resource>` (ConfigMap,
  Service, StatefulSet, PodDisruptionBudget), `<resource>-headless` or `<resource>-metrics`, and
  must live in the cluster's namespace; anything else fails the role group with a
  `*reconciler.ValidationError` **before any resource is applied**. Both paths that remove a slot
  address it by that derived name — the in-spec reclaim and the orphan teardown — so a
  differently named one was applied, owner-referenced, reported healthy and then survived every
  teardown until the cluster CR was deleted (for the metrics slot: a Prometheus target with no
  endpoints). Products needing their own name use `RoleGroupResources.ExtraResources` plus
  `SetupWithManagerOptions.ExtraOwns`, whose reclaim is label-based for exactly that reason.
  Nothing in any operator is affected today — every Gen 3 branch builds these names already.
- **CR labels can no longer forge a framework slot marker.** `metrics.kubedoop.dev/service`,
  `pdb.kubedoop.dev/role` and `pdb.kubedoop.dev/role-group` are dropped from the CR labels that
  reach a handler. A reclaim selects objects for deletion by these keys' presence or value, and
  nothing overwrote them downstream, so `kubectl label <cr> pdb.kubedoop.dev/role=anything` made
  the role-PDB cleaner reap every per-group PDB shipped through the escape hatch, and
  `metrics.kubedoop.dev/service=true` made a role group's reclaim delete the client Service of a
  sibling group named `<x>-metrics`. `restarter.kubedoop.dev/enable` and every other CR label are
  unaffected — the filter is an enumerated set, not a domain rule.
- **A 429 from a reclaim backs off instead of degrading the cluster.** `reclaimMetricsService`,
  `reclaimRolePDB` and `reclaimRoleGroupPDB` returned the raw API error, so throttling was
  reported as a resource-apply failure. These are the branches every role group of every product
  takes, since nothing in the SDK fills the metrics slot.

### features

- `builder.MetricsServiceBuilder.WithAnnotations(map[string]string)` merges extra annotations into
  the generated Prometheus set, with caller entries winning on key collisions, so a product can add
  its own scrape hints or correct a generated value. The builder still exposes no name or ClusterIP
  override: the reconciler's metrics slot is addressed by derived name (above), and the Service is
  always headless.

### fixes (config propagation)

- **The apply path no longer wipes the restarter's pod-template annotation**, which made a cluster
  that opted into `restarter.kubedoop.dev/enable` roll its pods indefinitely. `copyStatefulSetState`
  assigns `live.Spec = desired.Spec` wholesale — deliberately, so new mutable fields converge — and
  the pod template is inside that spec, so the `configmap.restarter.kubedoop.dev/<name>` key
  commons-operator stamps there was removed on the next reconcile. The resulting Update woke the
  restarter (its predicate matches the label on every Update, not only on Create), which re-stamped,
  which woke the reconciler through its own `Owns(&appsv1.StatefulSet{})` watch. Neither side errors,
  so the workqueue `Forget`s every pass and nothing backs off. Pod-template annotations are now
  merged with desired winning per key — the same rule the object's own annotations already followed —
  while pod-template *labels* stay replaced wholesale, because they must match the StatefulSet's
  immutable `.spec.selector`. This is the framework's only documented mechanism for delivering a
  `configOverrides` change to running pods, so it was broken for exactly the users who followed the
  documentation.

### BREAKING (log producer declarations)

- **`vector.WithProducers` takes `[]productlogging.ContainerLogging` instead of `[]string`.** The
  provider needs two names per producer and they may now differ: the shared log volume is mounted
  on the pod container (`ContainerLogging.Container`), while the Vector command pre-creates the
  producer's log directory (`productlogging.LogDirFor`, which honours the new `LogDirName`).
  Passing one string for both is what made the log tag inseparable from the container name.
  Migration is mechanical: `WithProducers([]string{"node"})` becomes
  `WithProducers([]productlogging.ContainerLogging{{Container: "node"}})`. Handlers that go through
  `BaseRoleGroupHandler.LoggingContainers` are unaffected — the reconciler passes the declarations
  it already had.
- **A log producer that names no container in the assembled pod now fails the role group**
  (`*reconciler.ValidationError`) instead of being skipped in silence. The silence produced a pod
  whose log directory was created and whose generated config pointed into it, with no container
  mounting the shared volume: the appender wrote into the container's own filesystem, Vector
  collected nothing, and every signal reported healthy. Two fixtures in this repository's own
  suite were declaring a phantom producer and passing.

### features (log tag decoupling)

- **`productlogging.ContainerLogging.LogDirName`** overrides the log-directory segment a producer
  writes under — and therefore the `container` field Vector tags its events with, since the
  collector extracts that field from the path segment and from nothing else. Empty keeps today's
  behaviour byte for byte. It moves only the directory and the tag: the volume mount still follows
  `Container`, `logging.containers.<Container>` is still the CRD key (it must be able to address
  containers that produce no log directory at all), and the default log-file base name still
  follows `Container`, which is what keeps two producers sharing a directory from resolving to one
  file. `productlogging.LogDirFor` / `LogDirSegment` expose the effective values; `ValidateProducers`
  checks a declaration list as a whole.
- An explicit `LogDirName` must be a single lowercase RFC 1123 label. The segment lands unquoted in
  the Vector container's `mkdir -p … && exec vector …` command — previously always a pod container
  name the API server had already constrained — and as one path segment where an embedded `/`
  silently truncates the tag, `.`/`..` escape the log root, and a space becomes a second `mkdir`
  argument against a read-only rootfs. Rejecting rather than quoting is deliberate: the command is
  part of the pod template, so quoting would roll every pod of every product on upgrade.
- Two producers resolving to the **same absolute log file** are rejected — two appenders with
  independent rotation policies on one file in one emptyDir lose entries with nothing to show for
  it. Sharing a *directory* stays legal, which is the coherent shape (one product tag, several
  containers, distinct files).

### BREAKING CHANGES

- `common.ClusterInterface` shrank from twelve methods to `client.Object` plus `GetSpec` and
  `GetStatus`. `GetName`, `GetNamespace`, `GetUID`, `GetLabels`, `GetAnnotations`, `SetStatus`,
  `GetObjectMeta`, `GetScheme`, `DeepCopyCluster` and `GetRuntimeObject` are no longer declared, and
  `common.ClusterObject` is removed. Product CRs delete those methods, embed
  `metav1.TypeMeta`/`metav1.ObjectMeta`, and must be registered with the scheme — the CR is now the
  object `client.Get` reads into. Status is mutated through the pointer `GetStatus` returns.
- `reconciler.GenericReconciler`, `GenericReconcilerConfig` and `NewGenericReconciler` are
  constrained by the new `common.ClusterResource[CR]` (`ClusterInterface` + `DeepCopy() CR`, which
  `make generate` already emits) instead of `common.ClusterInterface`.
- `common.ExtensionRegistry` is generic over the CR type: `common.NewExtensionRegistry[*MyCluster]()`.
  Extension hooks receive the concrete CR, so `common.AsClusterExtension`, `AsRoleExtension` and
  `AsRoleGroupExtension` are removed along with the erased registry they bridged.
- `common.GetExtensionRegistry` and `common.ResetExtensionRegistry` are removed; there is no
  process-wide registry. A reconciler executes only the registry passed as
  `GenericReconcilerConfig.ExtensionRegistry`, and `Clear()` replaces the global reset.
- The nine registration methods collapsed to three variadic ones:
  `Register{Cluster,Role,RoleGroup}Extension(ext, opts ...RegistrationOption)`. The
  `*WithPriority` and `*WithOptions` variants are removed; `WithPriority` and `WithStopOnError`
  themselves are unchanged.
- `config.ConfigFormat` is replaced by `config.ConfigMarshaler` (required, `Marshal`) and
  `config.ConfigUnmarshaler` (optional, `Unmarshal`, discovered by interface upgrade).
  `RegisterFormat`, `NewConfigGenerator` and `GetFormat` take/return `ConfigMarshaler`; parse through
  `ConfigGenerator.Parse` or the new `MultiFormatConfigGenerator.Parse(filename, content)`.
- Config adapter errors no longer wrap through `common.ConfigParseError`; emit failures read
  "failed to serialize `<format>` configuration" and the generators wrap them with the file and
  format. `config.ErrNoFormat` and `*config.UnsupportedParseError` are the typed checks.
- A file name matching several registered extensions now selects the **longest** registration
  instead of an arbitrary one.
- Adapter output changed where it was wrong: `.env` values are quoted unless every character is
  shell-inert (a generated file can no longer execute a config value when sourced), single-quoted
  `.env` values read back literally, XML rejects C0 controls/non-UTF-8 and emits `&#13;` for CR,
  `.properties` decodes `\uXXXX` and drops continuation indentation, and YAML rejects duplicate keys.
- Removed dead exported API: `commons/v1alpha1.ZKConfig`, `GracefulShutdownSpec`,
  `StorageResourceSpec`, the OIDC `ResponseType` enum, `common.HealthCheckResult`,
  `common.RoleInterface`/`RoleInfo`/`RoleGroupInfo`, `common.PrioritizedExtension` and
  `common.NoOpExtension`.
- `security.ListenerVolume(volumeName, secretClass, listenerVolumeName string, format SecretFormat)`
  gained the required `listenerVolumeName` parameter; it emits `listener-volume=<name>`, and an
  empty name panics (as does any named scope key without a value).
- `pkg/listener` lost its scope API: `VolumeRegistration.WithScope`, `ListenerScope`,
  `ListenerScopeNode`, `ListenerScopeCluster` and `ListenerScopeAnnotation` are removed, and PVC
  templates no longer carry the `listeners.kubedoop.dev/scope` annotation the listener-operator
  never read.
- `sidecar.SidecarConfig.MainContainerName` and `sidecar.FindMainContainer` are removed; use
  `sidecar.FindContainer(podSpec, name)` and set the primary container name through
  `BaseRoleGroupHandler.MainContainerName` / `SetRoleMainContainerName`.
- `reconciler.RoleGroupCleaner.Cleanup` takes `ownerUID types.UID` and `crAnnotations map[string]string`
  and returns `(time.Duration, error)` — the earliest pending wakeup plus the error.
- `S3ConnectionStatus`, `S3BucketStatus` and `DatabaseConnectionStatus` serialize their conditions
  under `.status.conditions` (was `.status.condition`); the CRDs must be re-applied.
- `ListenerSpec.PublishNotReadyAddresses` is a `*bool` with the nil-safe accessor
  `GetPublishNotReadyAddresses()`, so a Go client can express `false`.
- `sidecar.JMXExporterConfigMountPath` moved from `/opt/jmx_exporter` to
  `/kubedoop/mount/config/jmx-exporter`; the ConfigMap volume no longer shadows the directory
  holding the jar the container executes. `sidecar.JMXExporterJarPath` is the new constant for the
  jar.
- `testutil.ClusterWrapper` and `testutil.WrapMockCluster` are removed; `testutil.MockCluster`
  implements `common.ClusterInterface` directly and `testutil.MockRoleGroupHandler` is bound to
  `*testutil.MockCluster`.
- Four commons API fields became pointers so that "unset" is representable, and lost their
  `+kubebuilder:default`: `StorageResource.Capacity`, `CPUResource.Min`/`Max`,
  `MemoryResource.Limit` (all `*resource.Quantity`), `RoleGroupConfigSpec.GracefulShutdownTimeout`
  (`*string`) and `PodDisruptionBudgetSpec.Enabled` (`*bool`). The defaults moved to consumption
  time via new accessors — `StorageResource.GetCapacity`,
  `RoleGroupConfigSpec.GetGracefulShutdownTimeout`, `PodDisruptionBudgetSpec.IsEnabled` — and the
  new constants `DefaultStorageCapacity` and `DefaultGracefulShutdownTimeout`. Product CRDs must
  be regenerated; Go code constructing these specs takes a pointer (`ptr.To(...)`).

- `sidecar.NewOAuth2ProxySidecarProvider` lost its `cookieSeed` parameter: the session cookie
  secret is now read from a Secret via `secretKeyRef` (the client-credentials Secret's
  `COOKIE_SECRET` key by default; relocate with `WithOAuth2ProxyCookieSecretRef`). Add that key to
  the Secret, generating the value once with the new `sidecar.GenerateCookieSecret()`.
  `sidecar.DeterministicCookieSecret` and `WithOAuth2ProxyCookieSecret` are removed — both produced
  a value that ended up inlined in the PodSpec.
- The oauth2-proxy provider now requires exactly one explicit authorization policy. Pass
  `WithOAuth2ProxyEmailDomains(...)` or `WithOAuth2ProxyAllowAllEmails()`; `Inject` and `Validate`
  both fail with neither, and with both (allow-all would otherwise win and silently discard the
  domain list). `OAUTH2_PROXY_WHITELIST_DOMAINS` is no longer emitted by default — declare redirect
  targets with `WithOAuth2ProxyWhitelistDomains(...)`.
- `BaseRoleGroupHandler.BuildRolePodDisruptionBudget` takes a single `*reconciler.RoleBuildContext`
  instead of `(clusterName, namespace, roleName string, clusterLabels map[string]string, roleSpec
  *v1alpha1.RoleSpec)`. The new struct is the role-scoped analogue of `RoleGroupBuildContext` and
  additionally carries `ClusterSpec`, from which the `app.kubernetes.io/version` label is derived;
  a later role-level input now needs no further signature change. Handlers that override the method
  (it is detected through a method-set interface, so an override is a supported extension point)
  update their signature to match.
- `BaseRoleGroupHandler.ExtraLabels` and `BaseRoleGroupHandler.ExtraAnnotations` are removed, along
  with the `validateExtraLabels` build-time check they needed. Both were compile-time fields on a
  handler — a decision frozen when the operator is *built* — for something decided when a cluster is
  *deployed*. Labels move to the **cluster CR**: the reconciler already propagates
  `RoleGroupBuildContext.ClusterLabels` into every built resource's metadata and pod template, which
  is also the only route to the `StatefulSet.metadata.labels` that commons-operator's restarter
  watches. `ExtraLabels` largely existed to supply the three recommended labels the framework was
  not emitting, which it now does (see the label-set entry under *features* above).
  - `DiscoveryConfigMapOptions.ExtraLabels` / `WithDiscoveryExtraLabels` are a **different**,
    per-call API for one ConfigMap and are unaffected.
  - **Annotations get no replacement.** The framework now sets none on the resources it builds, and
    the CR's annotations are deliberately not propagated the way its labels are — that map holds
    `kubectl.kubernetes.io/last-applied-configuration` and the cleaner's own `orphan.zncdata.dev/*`
    progress markers. A product that needs e.g. cloud LoadBalancer annotations on the client Service
    has no supported way to set them; tracked in #553.

### docs (RoleGroupHandler PDB guidance)

- The `RoleGroupHandler.BuildResources` doc comment no longer instructs implementers to "Build PDB
  if needed" (#592, fixes #588). Per-group PDB construction contradicts the role-level design: the framework
  builds one PDB per role from `roleConfig.podDisruptionBudget`
  (`BaseRoleGroupHandler.BuildRolePodDisruptionBudget`), and `RoleGroupResources.PodDisruptionBudget`
  is an escape hatch for exceptional per-group budgets. The comment now states this explicitly.

### features (image conventions)

- **`constant.KubedoopJmxAgentJar` and `constant.JMXJavaAgentOpt(port, configFile)`** render the
  `-javaagent` option six operators hand-build today (#578). The config file is a **parameter**
  rather than a baked-in `config.yaml`: the hadoop image ships no `config.yaml`, only
  `namenode.yaml` / `datanode.yaml` / `journalnode.yaml`, so baking it in would have excluded the
  product with the most roles.
- This is a different mechanism from `pkg/sidecar/jmx_exporter.go`, which runs
  `jmx_prometheus_httpserver.jar` from `/opt/jmx_exporter` as a separate container — **a path no
  kubedoop image contains**, so that provider cannot run against a kubedoop product image as
  `SetProductImage` wires it. Filed separately; not changed here.
- **No `CopyMountedConfigScript` helper**, which the issue also proposed. The read-only config mount
  is real and is now documented, but the nine existing copy sites across ten operators disagree on
  flags and paths and `ConfigMountPath` is configurable, so a helper would be wrong for most callers
  or so parameterised it saves nothing. The transferable knowledge — that `-L` is required because a
  ConfigMap volume is a symlink farm into `..data/` — is documented instead.

### docs (the role group handler contract)

- **`docs/architecture.md` §4.1.5 "Writing a Role Group Handler"** (#579): the build-context
  read/write split, what a nil `RoleGroupResources` field instructs rather than omits, the container
  contract, the read-only config mount, the ways to fail a build, and what a handler that does not
  embed `BaseRoleGroupHandler` must reproduce.
- Two doc-vs-code corrections: `stopped` is forced in the base handler and not the reconciler, and
  the PodDisruptionBudget is role-level rather than per role group. A third — `BuildResources`'s
  stale "5. Build PDB if needed" — is left to PR #592, which already fixes it.
- `WithSidecarManager` is documented as **inert under the reconciler**, which always supplies a
  non-nil `SidecarManager` on the build context.

### BREAKING (product config)

- **`GenericReconcilerConfig.ProductConfig` gains a `ctx`, a `client.Client` and an `error`** (#574):

  ```go
  ProductConfig func(ctx context.Context, c client.Client, cr CR,
      roleName, roleGroupName string) (*v1alpha1.OverridesSpec, error)
  ```

  Adopters add the two parameters and `, nil` to the return. `examples/trino-operator` is migrated
  in this change.
- **Why.** The hook's own doc says it "may derive from live cluster state", but the signature had no
  `ctx`, no client and no error return — so it could only be a pure function of the CR, and a failed
  lookup could only be swallowed (rendering a silently wrong config) or panicked. The products that
  most needed a product-config layer were exactly the ones it could not serve, which is why **zero**
  operators used it while two hand-wrote the same workaround.
- **New: `RoleGroupBuildContext.ApplyProductDefaults(*OverridesSpec)`**, the imperative counterpart
  for a product that performs its lookup inside `BuildResources` and does not want to repeat it. It
  folds the layer **beneath** everything already merged.
- **New: `config.ConfigMerger.MergeBeneath(*MergedConfig, *OverridesSpec)`** implements that fold
  once, by the merge's own per-dimension rules — config files and env vars per key, CLI/JVM args as
  a whole, podOverrides through the same strategic merge patch. `Merge` could not express it: it
  folds left to right, so the lowest layer must be known before the merge runs, which a product
  computing from a live lookup cannot promise.
- The env-var half needs no ordering dance. Both operators had discovered that product defaults must
  not overwrite `envOverrides` and both solved it by **prepending** to the container's env list;
  `MergedConfig.EnvVars` is a map, so contributing beneath is simply "set what is absent".
- `mergePodTemplates` is extracted so the `RawExtension` path and `MergeBeneath` share one strategic
  merge implementation rather than two that can drift.
- **`reconciler.ConfigError` now carries a `Cause` and implements `Unwrap`**, with the new
  `WrapConfigError(field, cause)` constructor. It was the only error type in the package without
  one, so building it from another error flattened the chain to a string and callers lost
  `errors.Is` / `errors.As` — which is exactly how a product tells "the S3Connection does not exist
  yet" (`apierrors.IsNotFound`) from a real failure. `NewConfigError` is unchanged.
- `MergedConfig.JvmArgs` has no `OverridesSpec` field, so a lower layer cannot contribute any;
  `MergeBeneath` carries the higher config's through for callers that populated them imperatively
  via `AddJvmArg`.

### features (declare intent before Build)

- **`RoleGroupBuildContext.MainContainerCustomizer`** (#577) hands a product the assembled primary
  container just before `podOverrides` are strategic-merged. The framework owns the container's
  name, image, ports, security context, mounts and probes and nothing else, so every migrated
  product edited the StatefulSet the framework had just returned — zookeeper-operator locating the
  container as `Containers[0]`, an assumption the framework never made and which a sidecar provider
  inserting a container earlier silently invalidates.
- **The timing is the point, not the hook.** Running before the strategic merge is what keeps a
  user's `podOverrides` outranking the product; a post-build edit inverts that precedence silently.
  A customizer that returns an error fails the role group with a `*ValidationError`, and so does one
  that changes the **image** — the image is resolved once and propagated to the sidecars before the
  StatefulSet is built (the Vector agent ships inside the product image), so
  `RoleGroupBuildContext.Image` is that channel.
- **`RoleGroupBuildContext.ListenerClass` and `listener.ServiceTypeFor`** (#576) restore the
  class→Service-type mapping v0.12.6 shipped as `builder.ListenerClass2ServiceType`, which v0.13
  dropped while keeping the class constants: `cluster-internal` → ClusterIP, `external-unstable` →
  **NodePort**, `external-stable` → LoadBalancer, anything unrecognised → ClusterIP (the narrowest
  exposure, never an accidental public address). The client Service now gets its type at `Build()`
  instead of being patched afterwards.
- It lives in `pkg/listener`, not `pkg/builder` where the issue proposed it: `pkg/listener` already
  imports `pkg/builder`, so that direction does not compile. It pairs with the Service builder's
  existing `WithServiceType` rather than adding a second entry point.
- **Doc correction with teeth**: `ListenerClassExternalUnstable`'s comment said "creates LoadBalancer
  with dynamic IPs". It is a NodePort — the LoadBalancer is the *stable* class — and that wording is
  the documented reason two migrated operators disagreed about what `external-stable` meant.
- Both fields are additive: unset, the Service is a ClusterIP and no customizer runs, so an
  unmigrated product renders byte-identical output.

### features (generate-once secrets)

- **`reconciler.EnsureGeneratedSecret`** creates a Secret whose values are generated once and then
  never change (#575). It creates with generated values when absent, fills only **missing** keys
  when it exists, **never rewrites an existing value**, sets a controller owner reference, and
  tolerates the `IsAlreadyExists` of a concurrent reconcile by re-reading.
- **Why it was needed.** v0.13 made a generated Secret mandatory —
  `OAuth2ProxySidecarProvider.Validate` fails the reconcile when the cookie key is missing, and
  `GenerateCookieSecret`'s doc says to call it once and store the result — while removing
  `builder.SecretBuilder`, the only way to make one. `RoleGroupResources.ExtraResources` cannot
  serve: its apply path is idempotent `CreateOrUpdate` against a desired object, which rewrites the
  value every pass and produces exactly the "log every user out" failure the doc warns about. Five
  operators need this primitive; spark-k8s-operator hand-wrote 58 lines of it while migrating.
- **Filling a missing key is deliberate.** Providers fail the reconcile on a missing key, so a
  Secret that lost one — a partial restore, a hand-edit — would wedge the cluster with no recovery
  short of deleting the whole Secret, which rotates every *other* key too. Generators run only for
  absent keys, so the steady-state path invokes none of them and a generator with a side effect (an
  external KMS call) is not re-triggered.
- `Type` is applied only at creation, because it is immutable and writing it on update would make
  every later reconcile fail. Options mirror the discovery helper
  (`WithGeneratedSecretProductName` / `ExtraLabels` / `Annotations` / `Type`), and the canonical
  labels always win.
- No `sidecar.WithOAuth2ProxyManagedCookieSecret` convenience: the provider's `Validate` is the only
  place it could create from, and a validation hook that creates objects is a side effect in the one
  step whose job is to have none. Products call the helper from a `ClusterExtension` `PreReconcile`.

### fix (shared handler state)

- **`RoleGroupBuildContext` gains `Image`, `ImagePullPolicy`, `ContainerPorts` and `ServicePorts`**
  (#525) — the per-CR inputs a product derives from the cluster it is building for. They outrank the
  handler's own values, and the context is rebuilt per role group per reconcile.
- **Why it matters.** One handler instance is constructed in `main.go` and serves every CR and every
  reconcile, so the established idiom — assigning `h.Image` or calling `h.SetRoleContainerPorts`
  from inside `BuildResources` — writes per-cluster values into process-wide state. Above
  `MaxConcurrentReconciles: 1` those writes **race** between clusters; at the default of 1 they
  still **leak**, because a product that conditionally skips one assignment inherits the previous
  CR's value. spark-k8s-operator shipped exactly that (a CR omitting `pullPolicy` took the last
  CR's), with a serial reconcile loop and no race involved.
- **The framework held one too.** `SidecarManager.SetProductImage` writes the resolved image into
  the manager's configs, so a manager registered through `BaseRoleGroupHandler.WithSidecarManager` —
  process-wide — carried one cluster's image into the next. New
  `sidecar.SidecarManager.CloneForBuild()` copies the configs for the build; providers and phases
  are shared, being read-only during it. The reconciler-created manager is already per-role-group
  and is used as-is.
- Additive and backward compatible: unset context fields fall back to the handler exactly as before,
  so a product that has not migrated renders byte-identical output. Handler fields stay correct for
  reconcile-**invariant** configuration — this is a split, not a deprecation — and their doc
  comments plus the `SetRole*` setters now say which is which.
- Pinned by five specs, one of which builds eight clusters concurrently through a single handler.
  `make test` runs with `-race` in CI, so the old idiom is reported there as a data race rather than
  as a wrong value — verified by temporarily reintroducing it.

### BREAKING (image resolution)

- **`BaseRoleGroupHandler.ProductName` no longer decides whether `spec.image` is read** (#569). It
  now only names the product: the `app.kubernetes.io/name` value and the repository path segment.
  The new **`BaseRoleGroupHandler.ImageDefaults`** (a `commonsv1alpha1.ImageSpec`) supplies whatever
  `spec.image` leaves empty, evaluated **every reconcile**.
- **New: `ImageSpec.ResolveImage(productName string, defaults ImageSpec) (string, error)`**, plus
  `ResolvedProductVersion(defaults)` and `ResolvedPullPolicy(defaults)`. `GetImage` is retained and
  now delegates to `ResolveImage`, so the two cannot disagree.
- **Why this could not stay in a webhook.** Kubedoop publishes product images only with the
  `-kubedoop<version>` suffix, whose natural value is the operator's own build version — a
  reconcile-time fact. Webhook defaults are persisted into the spec at admission and never
  recomputed, so a cluster admitted by operator 0.1.0 kept asking for `-kubedoop0.1.0` images
  forever. `ImageDefaults` is read per reconcile, so an operator upgrade moves existing clusters
  onto the co-released image.
- **What was broken.** `GetImage` appended the `-kubedoop` suffix only when the *user* wrote
  `kubedoopVersion`, so a CR stating just `productVersion` produced `quay.io/…/hive:4.0.1` — a tag
  that does not exist. And when `repo` or `productVersion` was empty it returned `""`, after which
  the handler silently ran its static image: the user's `productVersion` discarded with no error, no
  event and no status change. Three migrated operators worked around this by hand-rolling image
  resolution, and two of them consequently emit no `app.kubernetes.io/version` at all.
- **An unresolvable `spec.image` now fails the role group**, naming the missing field. Running a
  version nobody asked for is not a safe default for a stateful product — the same call as
  `config.affinity`. With `ProductName` empty nothing is resolved from the CR beyond
  `spec.image.custom`, and that path never errors, so operators that resolve images themselves are
  unaffected.
- `spec.image.custom` is now honoured even when `ProductName` is empty; it used to be ignored, so a
  user pinning an image on such an operator was silently overruled.
- `app.kubernetes.io/version` follows the **resolved** version, so it is emitted when the version
  came from `ImageDefaults` too. A `custom` reference still publishes the user's own
  `productVersion` when they stated one — `custom` replaces the *reference*, and that field remains
  their declaration of which version it is.
- `ImageSpec.GetPullPolicy()` is now nil-safe. `spec.image` is an optional pointer, and it began
  reaching that method as nil once a nil spec could still resolve to a real image.
- **Adopters**: set `ProductName` + `ImageDefaults` and delete the hand-rolled resolver; drop the
  image branch of any defaulting webhook. `examples/trino-operator` is migrated in this change and
  is the reference. Rendered YAML gains `app.kubernetes.io/version` on the pod template (one rolling
  update); `.spec.selector` is untouched.

### features (CRD default guard)

- **`testutil.HaveNoInheritedConfigDefaults()` and `testutil.FindInheritedConfigDefaults()`** export
  the guard against the #544 defect class so product operators can pin it on their own fields
  (#570). Three lines, no envtest, no CR fixture, no cluster — it statically scans the CRD YAML
  `make manifests` generates:

  ```go
  It("declares no CRD default inside a role config block", func() {
      Expect("config/crd/bases/*.yaml").To(testutil.HaveNoInheritedConfigDefaults())
  })
  ```

- It reports every `default` under a role or role group `config` block **at any depth, with no depth
  heuristic**. "Deeply nested is safe" is false and the SDK has the scar: `LogLevelSpec.Level` sat
  two levels down under `logging.containers[*].console` and was a live defect for months (#573).
- Roles are detected **structurally** — any schema node declaring a `roleGroups` property — so the
  check covers both the generic `spec.roles[*]` map and products that flatten roles into named
  fields (`spec.coordinators`, `spec.workers`). Verified against the pre-#573 CRDs, where it finds
  all 18 real defects with exact paths.
- Arguments matching no files are an **error**, not a pass: a guard that silently inspects nothing
  reports success, which is worse than having no guard. So is a document that says it is a
  CustomResourceDefinition and fails to parse.
- The scan is scoped to `.spec`. The contract covers the desired state the framework folds Role ->
  RoleGroup; a `config` under `.status` is written by the operator and never merged. This is not
  only noise reduction — `GenericClusterStatus` already carries a `roleGroups` field, so an unscoped
  walk treats `.status` as a role node.
- Multi-document files are split with apimachinery's YAML reader rather than on `"\n---"`: a `---`
  inside a block scalar is ordinary text, and CRD descriptions are block scalars carrying whatever a
  Go doc comment said.
- `FindInheritedConfigDefaults` returns `[]InheritedConfigDefault` (CRD, version, JSON path,
  default, file) for callers that want to report differently.
- Wired into this repository's own suite and into `examples/trino-operator`, which is also the
  reference for the product-side usage.

### BREAKING (log levels)

- **`LogLevelSpec.Level` no longer carries `+kubebuilder:default:="INFO"`** (#570). It sits inside
  `config`, the block folded Role -> RoleGroup, where structural defaulting fills a leaf as soon as
  its *enclosing object* exists — so a role group that wrote `console: {}` got `console:
  {level: INFO}` back from the API server, and a role asking for `DEBUG` lost. This is the same
  defect #544 fixed for `resources`, still live on the logging fields.
- `mergeContainerLogging` already carried a guard for exactly this case, with a comment saying so.
  **It could never fire**: it tested `group.Console.Level != ""`, and the API server had filled the
  field before the merge ever saw it. The unit test covering that guard passed throughout, because a
  Go-constructed spec never meets structural defaulting — verified in envtest that role `DEBUG` +
  group `console: {}` produced `INFO`.
- The loggers map had **no** such guard at all: group entries were copied in unconditionally, so
  `loggers: {ROOT: {}}` replaced the role's entry with a level-less one, which the renderers skip —
  the logger fell back to the product's built-in default rather than the role's value. `console`,
  `file` and `loggers` now share one rule: **an entry that states no level means "inherit"**.
- A level-less logger entry with nothing to inherit is now **dropped** rather than carried into the
  merged map, which also keeps that map free of nil values. `loggers: {foo: null}` is a legal
  spelling of `loggers: {foo: {}}`, and the old unconditional copy carried the nil through, so a
  product reading `merged.Loggers[k].Level` had to know which spelling the user chose.
- **What changes for users.** A role group writing `console: {}` (or `file: {}`, or a level-less
  logger) now inherits the role's threshold instead of silently getting `INFO`; where no role value
  exists, no appender threshold is emitted instead of an `INFO` one — a no-op whenever the root
  logger is at `INFO`, and the *requested* behaviour when the root is `DEBUG` (today that case
  suppresses the debug output the user asked for). Explicit levels are unaffected. `kubectl get`
  stops showing `level: INFO` on blocks where the user wrote none. **CRs stored before this change
  keep the `INFO` the API server already persisted** — nothing rewrites them; re-applying the
  manifest clears it.

### docs (s3 pathStyle)

- **Documented that `spec.pathStyle` defaults to `false` and what that costs on adoption** (#571).
  `ConnectionInfo.S3AProperties()` renders `fs.s3a.path.style.access` from the user's field, whose
  CRD default is virtual-host addressing. Every product implementation the helper replaces pinned
  the key to `true`, because MinIO serves path-style **only** — with virtual-host addressing the
  client resolves `<bucket>.<host>` and gets NXDOMAIN. A product switching to the helper therefore
  changes behaviour for every existing cluster whose `S3Connection` omits `pathStyle: true`, and
  the failure appears at first bucket access rather than at admission.
- `S3ConnectionSpec.PathStyle` gained a doc comment; it had none, so `kubectl explain` printed
  nothing for it. Downstream CRD descriptions pick this up on their next bump. No behaviour change:
  the rendering, the default and the field are all unchanged.

### docs (single architecture document)

- **`docs/architecture_zh.md` is removed.** `docs/architecture.md` is now the only architecture
  document. A translated copy of an authoritative design document is authoritative only while it is
  current, and it was not: its status-condition sections spent a whole review round describing a
  model the code had already replaced, telling readers the framework behaves in a way it does not.
  A design change now costs one edit instead of two and cannot half-land.
- `README_zh-CN.md` is unaffected.

### features (framework metrics)

- **The orphan cleanup state machine now has a metrics surface.** `pkg/reconciler/metrics.go` exports
  `operator_go_orphan_cleanup_pending` (Gauge) and `operator_go_orphan_drain_timeouts_total`
  (Counter), both labelled `namespace` + `cluster`, registered on controller-runtime's
  `metrics.Registry` at init — they appear on the endpoint an operator already serves with no wiring
  in `main.go`.
- Teardown is the one part of the framework with no other observability: it spans many reconciles,
  records progress in annotations on the objects it is retiring, and reports the rest in log lines.
  A role group stuck mid-teardown for three days produces no error, no failing reconcile and no
  condition transition — while its pods keep running and its PVCs keep costing. The `Degraded`
  condition will not fire for it either, by design (it is not a fault the operator can resolve).
- The gauge is written on **every** pass, including at zero. A gauge only set while something is
  pending keeps publishing its last non-zero value after the teardown finishes, and an alert on it
  would never clear.
- The drain-timeout counter marks the one event in that machine that had no surface at all beyond a
  log line, and the one that matters most: reaching it means a stateful product was denied the
  ordered shutdown the scale-to-zero existed to give it, so a pod was killed mid-flush. A counter
  rather than a gauge, because the question is "did this ever happen" and the answer must survive an
  operator restart.
- A deleted CR's series are **removed**, not zeroed, on the `IsNotFound` branch of `Reconcile` — the
  only place the framework learns a cluster is gone, since it registers no finalizer and so has no
  teardown callback. A zeroed series still publishes a series for something that does not exist.
- **The boundary is deliberate**: nothing else is exported. controller-runtime already publishes
  reconcile counts, errors and durations (`controller_runtime_reconcile_*`), and kube-state-metrics
  already turns CR status conditions into series once configured for a product's CRD.

### fix (cleanups)

- `podOverrides` whose strategic merge fails is now recorded on `PodOverrideViolations()` instead of
  being dropped with a bare `return`. The branch is **unreachable through the public API** — every
  step operates on a valid `*corev1.PodTemplateSpec` — and therefore carries no spec; it is recorded
  rather than ignored so a later refactor cannot reopen a silent-drop hole at the one point nobody
  is watching.
- The framework's ServiceAccount no longer aliases `cr.GetLabels()`. It is cloned, and the canonical
  `app.kubernetes.io/instance` and `managed-by` labels are added, so the SA is identifiable the same
  way every other framework-owned object is.
- Removed the dead health helpers `CheckPodHealth`, `isPodReady` and `UpdateStatusCondition`, which
  nothing in the SDK called after the condition rework, and the duplicate `DefaultCheckInterval` /
  `DefaultTimeout` constants — `NewHealthManager` uses `DefaultHealthCheckInterval` and
  `DefaultHealthCheckTimeout`, the values the reconciler config documents.

### BREAKING (storage class)

- `commons/v1alpha1.StorageSpec.StorageClass` is now `*string`. **The empty string is not a synonym
  for unset here**: Kubernetes reads `storageClassName: ""` as "bind only a pre-provisioned PV,
  never dynamically provision one", so a role group that wrote `storageClass: ""` to mean "inherit
  the role's" got a PVC that stays `Pending` forever. With a pointer, unset means "use the cluster
  default" and `""` keeps its Kubernetes meaning. Go code reading the field takes the pointer
  (`ptr.To("fast-ssd")` to set it); CR YAML is unaffected.

### features (orphan cleanup of product extras)

- **A removed role group's `ExtraResources` are now reclaimed instead of surviving until the cluster
  CR is deleted.** They were invisible to the cleaner: it deleted the framework's own kinds, pruned
  the group's `status.roleGroups` entry — so nothing in the SDK would ever look at the group again —
  and left the product's arbitrary-GVK objects behind. For a `listeners.kubedoop.dev` Listener that
  is not untidiness: the listener-operator turns it into a Service of type LoadBalancer, so a role
  group scaled away in the morning is still billing for a cloud load balancer in the evening.
- Discovery needs no new declaration. `SetupWithManagerOpts` hands
  `SetupWithManagerOptions.ExtraOwns` — the kinds a product already registers to get watches — to
  the cleaner as `RoleGroupCleaner.WithExtraResourceKinds`. Deriving one from the other is what
  keeps them in step: a product adding an extra kind must register it for watches anyway.
- **Both guards are required**, and each covers what the other cannot. An object is deleted only if
  it carries the departing role group's identity labels (`instance` + `managed-by` + `component` +
  `role-group`) **and** this CR is its controller owner. Labels alone would reach another cluster's
  object — `instance` is the cluster *name*, and a namespace can hold a second product's CR under
  the same one. Ownership alone would take every surviving role group's extras along with the
  orphan's. The name check the framework's own kinds use is unavailable here: an extra's name
  belongs to the product.
- The listing goes through the **uncached** reader (`WithAPIReader`), for the reason
  `confirmRoleGroupReclaimed` already gives: the role group's status entry is pruned on the strength
  of the pass settling, and a cached `List` that has not caught up answers "nothing here" for an
  extra that exists. For extras that answer is *terminal* rather than early — the framework's own
  kinds are re-found by live orphan discovery, extras are not in that inventory and have no derived
  name to look them up by.
- It fails closed. An unregistered kind, an unlabelled object, or an empty owner UID (the same
  precedent as live orphan discovery and the role-PDB reclaim) means nothing is deleted — exactly
  the behaviour before this change. A product that builds its controller through `ControllerBuilder`
  rather than `SetupWithManagerOpts` gets no extras cleanup and can call `WithExtraResourceKinds`
  itself.
- Teardown order mirrors the apply order: extras are created *before* the StatefulSet because they
  are pod-scheduling prerequisites, so they are deleted immediately *after* it — nothing a pod might
  still need is reclaimed while a pod could still exist.

### fix (orphan cleanup)

- **The PVCs of an orphaned role group are now deleted after the drain, not before the
  scale-to-zero.** With `operator.zncdata.dev/delete-pvcs` set, the most destructive and least
  reversible step of the whole teardown was being issued first — while the pods were still running
  and writing — on the strength of a comment claiming "replica count is still valid at this point".
  That was never true: `deletePVCsForStatefulSet` has listed by the pod selector since it was
  written, precisely so it does not depend on the replica count.

  It matters because deleting a role group is undoable right up until the data goes. A user who
  removes a role group by mistake and re-adds it a minute later — a `git revert` of a bad CR edit —
  used to find the volumes already reclaimed while the StatefulSet was still there draining. Now the
  drain window is safe: re-adding costs a restart, not a restore.

  PVCs still go **before** the StatefulSet, because the cleaner reaches them through its selector
  and the other order would leave them unreachable; a process death between the two steps simply
  re-enters the same pass. The drain-timeout path falls through to the same deletion, so a pod that
  will not terminate cannot silently leak the volumes the user asked to reclaim.

### BREAKING (config.affinity)

- **A misspelled affinity key now fails the build instead of scheduling the pods anywhere.**
  `config.affinity` is a schema-free `RawExtension`, so the API server neither validates nor prunes
  it, and `json.Unmarshal` ignores what it does not recognise. The two together made a typo
  completely silent: `nodeAffinty` (one letter short) passed `kubectl apply`, decoded into an empty
  `corev1.Affinity`, and the pods were scheduled wherever the scheduler liked — no event, no log
  line, no status change. Affinity is the scheduling *contract* for these products (rack awareness,
  spreading a quorum across failure domains, keeping a worker near its data), so losing it is not
  cosmetic.

  `reconciler.DecodeAffinity` now decodes with `DisallowUnknownFields`, and the build error names the
  offending field. **The trade-off is deliberate**: a field belonging to a newer Kubernetes API than
  the SDK is built against is now rejected rather than ignored. That is the honest answer — the
  framework cannot honor a field it does not know — and it fails where someone can read it rather
  than when a node drains at 3am.
- **`config.affinity` replaces rather than merges, and that is now documented and tested.** A role
  group's affinity has always replaced the role's wholesale, with `affinity: {}` clearing it; none
  of that was written down or covered by a spec. It stays that way deliberately: `resources` is a
  set of independent knobs where overriding `cpu.max` and keeping the rest is the normal thing to
  want, but an affinity is a single scheduling *policy*, and Kubernetes replaces `PodSpec.Affinity`
  too (Helm values, Kustomize patches). Per-member inheritance was implemented and then reverted —
  it invented a semantic users would have to learn, and it removed the only way to say "this group
  has no affinity", which a single-node development group needs.
- A typed `*corev1.Affinity` CRD field was measured and **rejected**: it grows the generated CRD from
  39 KB to 192 KB (the field appears at both role and role-group level), and `kubectl apply` stores
  the whole CRD in `last-applied-configuration` — within one annotation's 256 KB limit, leaving a
  real product's own fields no room. The strict decode recovers most of the diagnostic value at no
  size cost.

### BREAKING (status conditions)

- **`Degraded` no longer fires during a rolling update, a scale-up or a scale-down.** It was derived
  from `readyReplicas == desiredReplicas`, so every planned change that reduces ready replicas
  reported the cluster as broken. Measured against a real API server:

  | situation | before | after |
  |---|---|---|
  | rolling update in flight | `Progressing=True` **and `Degraded=True`** | `Progressing=True`, `Degraded=False` |
  | scale-down 5→3 | `Available=True`, `Progressing=False`, **`Degraded=True`** | `Available=True`, `Degraded=False` |
  | pod in `ImagePullBackOff` | `Degraded=True` | `Degraded=True` (**reason `PodFailure`, naming the pod**) |

  The scale-down row is the worst of the three: `Progressing=False` meant nothing in the status even
  hinted the cluster was mid-operation. A signal that fires on every planned change is one nobody
  can alert on.

  `Degraded` is now computed from **failure states**: a pod wedged in `CrashLoopBackOff`,
  `ImagePullBackOff`, `InvalidImageName`, a `CreateContainer*`/`RunContainerError`, or a pod that
  cannot be scheduled; a role group whose StatefulSet cannot be read; a failing `ServiceHealthCheck`.
  Because those are states and not elapsed times, a **stuck** rollout still reports `Degraded=True` —
  its pods are visibly failing — while a healthy rollout does not, and no progress-deadline
  machinery is required. Transient states (`ContainerCreating`, `PodInitializing`) and pods already
  being deleted are excluded. Cost: one `List` of the cluster's pods per health pass.
- **`Available` now uses `>=` instead of `==`.** A role group mid-scale-down briefly reports more
  ready replicas than desired, which the old test called unhealthy.
- **New reasons**: `PodsNotReady` and `WorkloadUnreadable` (for `Available=False`), `PodFailure` and
  `WorkloadUnreadable` (for `Degraded`). The `Available=False` message names the offending role
  groups and distinguishes the two ways a role group can be unavailable — short of ready replicas,
  or a StatefulSet that could not be read at all, which has no replica counts to quote. The
  `Degraded` message names the offending pods with their reasons, capped at three with the remainder
  counted rather than silently truncated.
- **Alerting guidance**: alert on `Degraded=True` for faults, and on `Available=False` **with a
  duration** for "not serving". `Progressing=True` with an old `lastTransitionTime` is the signal for
  a rollout that is taking too long.

### BREAKING (reconciliationPaused)

- **A paused cluster reports the new `Paused` condition with `Degraded=False`**, instead of
  `Degraded=True`. Pausing is an administrator's decision — a maintenance window, an investigation —
  and reporting it through the fault signal pages someone for a planned action. The framework already
  drew this distinction for the sibling operation `stopped` (`Degraded=False`, "intentionally
  stopped"); the two are the same class of state and now read the same way.
- **The paused path no longer leaves the other conditions stale.** It returned before any condition
  but `Degraded` was written, so a cluster paused mid-rollout advertised `Progressing=True` from the
  last running cycle for as long as the pause lasted, and one paused after a failure kept claiming
  `Available`. The pause freezes the *resources*, not the reporting: `Available` and `Progressing` are
  now re-evaluated from the live StatefulSets (reads mutate nothing), and `ServiceHealthy` goes
  `Unknown` rather than keeping a stale verdict — an active probe is exactly what a pause asks the
  operator not to run.
- **A paused reconcile now returns `RequeueAfter: HealthCheckInterval`** instead of no requeue, so
  those conditions keep up with a paused cluster's pods. A container entering `ImagePullBackOff`
  without ever having been ready changes no StatefulSet field, so the `Owns()` watch would not
  deliver anything.
- `GenericClusterStatus` gains `SetPaused`, `IsPaused` and `SetServiceHealthyUnknown`, and the
  `ConditionPaused` type.

### BREAKING (primary container probes)

- **`StatefulSetBuilder` no longer generates a liveness probe.** It used to attach a TCP probe to
  `Ports[0]` whenever any container port was declared, with ~90-120s to the first kill. Both halves
  of that were guesses the framework is not in a position to make: `Ports[0]` is an accident of the
  product's declaration order — in the SDK's own test fixture it lands on a *metrics* port, not the
  service port — and the kill budget is inside the startup time of several products this SDK exists
  for (a NameNode loading an fsimage, a broker replaying log segments). The result was a
  CrashLoopBackOff caused by a probe the user never wrote.

  A TCP-accept check is also close to worthless *as* a liveness signal — a wedged JVM still accepts
  connections — so this removes a mechanism that was nearly all risk and almost no guarantee, not a
  guarantee. Products that want the old behavior:

  ```go
  stsBuilder.WithLivenessProbe(builder.DefaultTCPLivenessProbe(8020)) // a port THEY choose
  ```

  `DefaultTCPLivenessProbe` is exported for exactly that, with the previous timings.
- **The readiness probe is kept**, and its target is now a documented contract rather than an
  accident: it is `Ports[0]`, so the first entry of `SetRoleContainerPorts`/`WithPorts` must be the
  port that means "this pod can serve". Readiness is kept because its failure modes are the
  acceptable ones — at worst the pod stays out of its Services, visible as `0/1` and self-correcting
  — and because removing it would assert the opposite: with no readiness probe a pod is Ready the
  instant its container starts, so a rolling update walks a whole role group without ever waiting
  for a member to come up.
- This is the opposite decision from the sidecar probes (#548) and deliberately so. There the
  framework owns the container — its image, its port, its endpoint — so it can say what healthy
  means. Here the container belongs to the product.

### features (StatefulSet rollout knobs)

- `StatefulSetBuilder.WithUpdateStrategy` and `WithPodManagementPolicy` expose the two
  `StatefulSetSpec` fields `podOverrides` cannot reach (it is a `PodTemplateSpec`).
  `updateStrategy` is mutable and not preserved by the apply path, so a partitioned canary rollout
  — raise the partition, upgrade the high ordinals, verify, lower it — converges through a normal
  reconcile. `podManagementPolicy` is immutable and can only be chosen at creation.
- The `Parallel` default for `podManagementPolicy` is unchanged and now carries its reason:
  `OrderedReady` starts pod N+1 only once pod N is Ready, which **deadlocks a quorum product at
  pod-0** — a ZooKeeper member or an HDFS JournalNode is not Ready until it sees a quorum that does
  not exist until its peers start. It is a choice, not an inherited default.

### fix (pod security)

- **The framework's default pod security context now sets `fsGroupChangePolicy: OnRootMismatch`
  alongside `fsGroup: 1001`.** Setting `fsGroup` without it means Kubernetes' own default of
  `Always`, under which the kubelet walks the **entire** volume — chown'ing and chmod'ing every
  file — before the container starts, on *every* start. For the workloads this SDK exists for that
  is the difference between a pod starting in seconds and a pod sitting in `ContainerCreating` for
  tens of minutes on every restart, rollout and node drain, with nothing in its events explaining
  why: an HDFS DataNode or a Kafka broker data PVC holds millions of files.

  `OnRootMismatch` recurses only when the volume root does not already carry the expected owner and
  mode, which is true exactly once per freshly provisioned volume. The trade-off is deliberate:
  ownership that drifts *inside* a volume whose root is still correct is not repaired — a repair
  the framework never promised, and not worth paying for on every start. A product that wants it
  back sets `fsGroupChangePolicy: Always` through `podOverrides`, which deep-merges per field and
  so keeps the rest of the hardening.

  The policy has no effect on ephemeral volumes (secret, configMap, emptyDir), so the config mount
  and the shared log volume are unaffected either way.
- `PodSecurityBuilder.WithFSGroupChangePolicy` exposes the field to callers assembling a context by
  hand. Like its siblings it is opt-in: only the framework default pairs the two, so a hand-built
  context is not given an opinion it did not state.

### fix (naming)

- **A role group whose cluster and group names together exceed 63 bytes can now be created at
  all.** The framework stamps a marker label keyed `<cluster>-<group>`, and a label key's name part
  is capped at 63 bytes. Two entirely ordinary big-data names overrun it — a 43-character cluster
  plus a 21-character role group is 65 — and because the key lands in the StatefulSet's
  `.spec.selector`, in both Services' selectors and in every pod template label set, the API server
  rejected the StatefulSet, both Services *and* the PDB of that role group, quoting a label key the
  user never wrote. The resource *name* built from the same two strings was already bounded by
  `RoleGroupResourceName`; the framework had applied a limit it knew about to one of the two places
  it applies.

  `reconciler.RoleGroupMarkerLabelKey(cluster, role, group)` now produces the key: the natural
  `<cluster>-<group>` whenever that is a legal label key, `RoleGroupResourceName` otherwise.
  Preserving the natural form is load-bearing rather than cosmetic — `.spec.selector` is immutable,
  so changing this key for a role group that already has a StatefulSet would leave the pod template
  no longer matching the frozen selector and every later update rejected. Only combinations that
  could never have produced a StatefulSet get the substitute, so **running clusters are
  byte-for-byte unaffected**.
- `RoleResourceName` is unchanged, and its doc comment now gives the real reason: a
  PodDisruptionBudget name is validated as a DNS *subdomain* (253 bytes), not the 63-byte DNS label
  that bounds a Service name. The previous comment justified the absence of truncation by comparing
  against the role group name, which is itself truncated — right conclusion, wrong argument.

### BREAKING (role and role group names)

- The keys of `spec.roles` and `spec.roles.<role>.roleGroups` must now be **lowercase RFC 1123
  labels**, enforced by a CEL `x-kubernetes-validations` rule on `GenericClusterSpec.Roles` and
  `RoleSpec.RoleGroups`. Every product CRD picks this up on the next `make manifests`.

  This only rejects what never worked: each key becomes a segment of `<cluster>-<role>-<group>` and
  the value of an `app.kubernetes.io/*` label, so `Coordinator`, `my_role` and `a.b` have always
  produced resource names the API server refuses. What changes is *when* the user hears about it —
  at `kubectl apply`, in a message naming the rule, instead of halfway through a reconcile as a
  permanently `Degraded` role quoting a `metadata.name` nobody wrote.
- Both maps gained `maxProperties` (64 roles, 256 role groups per role). This is not a product
  constraint but the CEL cost estimator's only handle on a map: without a declared bound it assumes
  the theoretical worst case and the API server refuses the rule at CRD creation time with
  "estimated rule cost exceeds budget by factor of more than 100x". The bounds sit far above any
  real deployment.

### security (secret scopes)

- **A CR-supplied scope name containing `,` or `=` no longer widens the credential it asks for.**
  The secret-operator scope annotation is one comma-delimited string of `key=value` entries with no
  escaping, and `scope.Services` / `scope.ListenerVolumes` were spliced into it verbatim. A value
  of `mysvc,node` rendered `service=mysvc,node`, which the secret-operator parses as a service
  scope **and a node scope** — the CR author silently received a certificate covering the node's
  hostname and IP, and a reviewer reading the CR saw nothing unusual.
  - `CredentialsScope.Services` and `.ListenerVolumes` now carry
    `+kubebuilder:validation:items:Pattern=^[^,=]+$` and `items:MinLength=1`, so the **API server
    rejects it at admission**, where the user is told about it. That is the real fix.
  - `security.ScopeString` additionally drops any entry it cannot render as itself, covering what
    admission cannot: a CR stored before those markers existed, and a scope built in Go. Dropping
    is the safe direction — splicing grants a broader credential invisibly, while dropping
    withholds a requested one, which surfaces as the application rejecting the certificate.

### fix (events)

- `Created`/`Updated`/`Deleted` events now name the object's **Kind**. Everything the framework
  applies is a typed struct from `pkg/builder` round-tripped through controller-runtime's typed
  client, which does not populate `TypeMeta`, so the message read `Created  ns/kafka-broker-default`
  — with a hole exactly where the disambiguator between a role group's Service, its headless
  Service and its metrics Service belongs. `NewEventManager` now takes the scheme and resolves the
  kind from it, falling back to the **bare** Go type name (`Listener`, not `*v1alpha1.Listener`)
  for an object the scheme does not know. The reconciler's `resourceKind` now shares that
  resolution, so an object is never called two different things in the event and in the error.

### BREAKING (events)

- `NewEventManager(recorder)` → `NewEventManager(recorder, scheme)`.
- `EventManager.EmitProgressingEvent`, `EmitAvailableEvent` and `EmitDegradedEvent` are removed.
  The framework never called them, and they mirror transitions it already owns through status
  conditions — an exported emitter the framework never calls reads as a framework guarantee, and a
  product calling these would publish a second, unsynchronized account of the cluster's state.
  `EmitWarningEvent` / `EmitNormalEvent` / `LogAndEmitError` / `LogAndEmitInfo` remain for product
  code; the last two are documented as never called by the framework.

### fix (podOverrides)

- A `podOverrides` volumeMount at a **mountPath the framework already owns** now fails the role
  group with a `*reconciler.ValidationError` instead of silently unmounting the framework's volume.
  Strategic merge patch keys `volumeMounts` by `mountPath`, not by `name`, so such a mount does not
  join the framework's — it *replaces* it:
  - when the override also declares its own volume the result is a perfectly valid pod spec that
    the API server accepts, with the generated ConfigMap mounted **nowhere**. The product comes up
    on an empty config directory and crash-loops, or silently runs on its built-in defaults, and
    nothing anywhere reports a problem. This is the case the change exists for;
  - when the override declares only the mount, the API server rejects the StatefulSet naming
    `spec.template.spec.containers[0].volumeMounts[0].name` — a field the user never wrote, with no
    mention of `podOverrides`.
  The error names the mountPath, the volume that displaced the framework's, and `podOverrides`.
  Mounting at a path the framework does **not** own is unaffected — that is what users normally
  mean, and it still appends.
- `StatefulSetBuilder.PodOverrideViolations()` exposes the same findings to products driving the
  builder directly (`Build()` cannot return an error, so they are read back after it).

### fix (orphan discovery)

- Orphaned role groups are now discovered from the **live cluster** as well as from
  `status.roleGroups`. The ledger alone was a record the operator had to have *successfully
  written*: when the process died between applying a role group's resources and updating the CR,
  when a backup tool restored the CR without its status subresource, or after a `kubectl replace`,
  the resources it had named became invisible to the cleaner **permanently** — nothing else ever
  enumerated them, so they held their PVCs, their PDB and their pods until someone deleted them by
  hand.
  - The live half lists the role group ConfigMaps and StatefulSets this CR controller-owns that
    carry `app.kubernetes.io/{instance,managed-by,component,role-group}` **and** are named exactly
    what `RoleGroupResourceName` produces for those labels. That last check is what keeps it safe:
    a discovery ConfigMap carries the same instance/managed-by pair and owner reference, and a
    product's `RoleGroupResources.ExtraResources` may carry the handler's entire label set.
  - Both kinds are listed because the teardown deletes the StatefulSet before the ConfigMap — a
    pass interrupted in between leaves a ConfigMap a StatefulSet-only inventory would never see.
  - `status.roleGroups` stays in the union (and is still written and pruned): it is the only source
    that can attribute a resource created *before* the framework stamped `app.kubernetes.io/role-group`
    to a role group.
  - A failed discovery no longer reads as "no orphans": it returns the error and leaves
    `OrphanCleanupPending` untouched rather than clearing it on the strength of a failed list.
  - An empty owner UID disables live discovery entirely, exactly as it disables the role-PDB
    reclaim.

### security

- Every framework-injected sidecar now carries a hardened container security context by default
  (`sidecar.DefaultSecurityContext()`: `runAsNonRoot`, `allowPrivilegeEscalation: false`,
  `capabilities: drop ALL`, `seccompProfile: RuntimeDefault`). Previously oauth2-proxy and the JMX
  exporter shipped with a nil security context while the product's own container was hardened, so a
  namespace enforcing the restricted Pod Security Standard rejected the whole workload — the
  sidecars were the reason the pod could not be admitted. An explicit
  `SidecarConfig.SecurityContext` still replaces the default wholesale.
- The oauth2-proxy session cookie secret is no longer inlined into the PodSpec as an env `value`.
  It signs every session the proxy trusts, so anyone able to `get pod` could forge a session and
  bypass authentication; it was additionally derived (via SHA-256) from a caller-supplied seed that
  products naturally set to the CR's UID, making it reconstructible by anyone who could read the
  CR. It is now referenced with `secretKeyRef` like the client credentials beside it, generated
  from `crypto/rand`, and `Validate` fails the reconcile when the referenced key is absent instead
  of letting the proxy crash-loop.
- `OAUTH2_PROXY_EMAIL_DOMAINS` no longer defaults to `*`. Authenticating against an identity
  provider is not authorization for the cluster: on a shared realm that default admitted every
  account the IdP could issue a token for. `OAUTH2_PROXY_WHITELIST_DOMAINS` no longer defaults to
  `*` either, which had made the post-login `rd` parameter an open redirect.

### features

- Framework-built resources now carry the **complete** Kubernetes recommended label set that
  `pkg/constant/label.go` has always declared. `app.kubernetes.io/name` (from
  `BaseRoleGroupHandler.ProductName`), `app.kubernetes.io/version` (from `spec.image.productVersion`)
  and `app.kubernetes.io/role-group` join the instance/component/managed-by trio that was actually
  emitted, on object metadata and pod template alike; the role-level PDB gets the same set minus
  `role-group`, which spans every group by definition. Three of the six keys and
  `constant.MatchingLabelsNames()` were previously documented API that nothing wrote, so
  `kubectl get pods -l app.kubernetes.io/name=trino` returned nothing across every kubedoop
  operator.
  - `name` and `version` are omitted for a handler with no `ProductName`: that handler runs its
    static `Image` and never reads `spec.image`, so publishing a version from there would label the
    pods with a version they are not running.
  - Either is also dropped when its value is not a legal label value. `productVersion` is free-form
    user input and a legal image tag may still be an illegal label value (over 63 characters), and
    that has to cost one cosmetic label rather than make every resource of the cluster rejected by
    the API server.
  - `.spec.selector` is **unchanged** — `version` changes on every product upgrade and a selector is
    immutable. Existing clusters roll once as the pod template gains labels, and keep satisfying the
    selector they froze.
  - `RoleGroupBuildContext.ClusterLabels` and `RoleBuildContext.ClusterLabels` are now always a
    non-nil map. Both are documented as the handler's to build on, but they were produced by
    `maps.Clone(cr.GetLabels())`, which preserves nil — so a CR carrying no labels at all (perfectly
    ordinary) made a handler adding an entry to `ClusterLabels` panic with "assignment to entry in
    nil map". Found by Copilot's review of #554.
- `SidecarConfig.Probes` (`sidecar.SidecarProbes`) lets a product override the probes a provider
  sets: `Startup`/`Liveness`/`Readiness` replace one **wholesale** (probe handlers are a Kubernetes
  one-of, and a merged probe carrying two handlers is rejected by the API server), and
  `DisableStartup`/`DisableLiveness`/`DisableReadiness` remove it. `SidecarConfig` previously had no
  way to express a probe at all, so the framework's policy was unconfigurable rather than a default,
  and a product that needed one had to reach for raw `podOverrides`.
- The rendered Vector pipeline now carries an `internal_metrics` source and a `prometheus_exporter`
  sink on `0.0.0.0:9598` (`vector.VectorMetricsPort`, declared as the `vector-metrics` container
  port), so the log agent is finally observable: `vector_component_sent_events_total` and
  `vector_buffer_events` make "the agent stopped shipping" alertable, which nothing in the pipeline
  previously could — its only sink was the aggregator it may have lost. The endpoint carries the
  agent's own counters and gauges (component throughput, errors, buffer depth) and never log content.
  It is also what the container's liveness probe targets, because it exercises the running topology.
- Split the config format contract into a required `ConfigMarshaler` and an optional
  `ConfigUnmarshaler`, so a product that only emits a format no longer writes a parser nobody calls
- Added `MultiFormatConfigGenerator.Parse(filename, content)` and the typed errors
  `config.ErrNoFormat` / `*config.UnsupportedParseError`
- Added `common.ClusterResource[T]`, the reconciler-facing constraint that adds `DeepCopy() T`
- Made extension execution order deterministic: same-priority extensions run in registration order
  via a per-entry sequence number, and `WithStopOnError` overrides the per-hook fault tolerance for
  a single registration
- A successful reconcile now requeues on `HealthCheckInterval` (default 120s, negative disables) or
  the earliest pending gray-delete deadline, so state that produces no watch event still converges
- Wired sidecar provider validation, declared dependency checks and `SetupWithManagerOpts.ExtraOwns`
  into the reconcile loop
- Added `VectorConfigProvider` so a handler can declare it supplies `vector.yaml`; with no source
  the Vector sidecar is skipped with a `VectorSidecarSkipped` warning instead of failing the cluster
- `SidecarManager` injects providers in dependency phases (`Producer` → `Default` → `Pipeline`), so
  Vector always runs after the log producers whose volume it mounts
- `ServiceBuilder` gained `AddServicePort`, `WithPorts` and `WithPublishNotReadyAddresses`
- `productlogging.SupportedLoggingFrameworks()` enumerates the registered frameworks, so the
  vector/productlogging drift guards range over a real table

### fix

- The orphan state machine's progress annotations are now reset when a role group comes back into
  the spec, so a re-orphaned group no longer inherits the previous teardown's timestamps. Two
  separate leaks, both failing toward **faster destruction**: the gray-delete mark
  (`orphan.zncdata.dev/pending-deletion`) is stamped on every primary but was cleared only on the
  StatefulSet, leaving the ConfigMap's stale timestamp authoritative and skipping the grace period
  entirely; and `orphan.zncdata.dev/drain-started` was cleared nowhere at all — it survives
  re-apply because annotations are merged — so the drain deadline was evaluated against the
  previous teardown and the StatefulSet was force-deleted without waiting for the ordered drain.
  The reset is also no longer gated on `grayDeleteGracePeriod > 0`, which had meant the drain mark
  was never reset in the default configuration.
- Role and role group iteration is **best effort**: a failing role no longer aborts the whole
  reconcile. Orphan cleanup, the health check and the cluster PostReconcile hook now run even when
  a role failed, and the per-role errors are combined with `errors.Join` so the cluster still goes
  Degraded and the workqueue still backs off. Previously one unparsable value on the
  alphabetically-first role indefinitely blocked the deletion of an unrelated role group, the
  health of every other role, and the discovery ConfigMap products publish from PostReconcile —
  and because iteration was sorted, the same later roles were starved on every cycle. A 429 still
  aborts the pass. The cleaner had already adopted this policy for the same situation; the hot
  path now matches it.
- An immutable field the apply path refuses to change now produces an `ImmutableFieldIgnored`
  Warning event on the CR, naming the resource and the field paths. Preserving
  `volumeClaimTemplates`, `selector`, `serviceName` and `podManagementPolicy` is correct — the
  alternative is an Update the API server rejects every reconcile — but doing it silently meant a
  storage resize was accepted, reported as `ReconcileComplete=True`, and never applied, with
  nothing in the API explaining why. Only a field the handler actually set is reported, so a
  settled cluster stays quiet.
- **A role group overriding one leaf of `config.resources` no longer discards the role's siblings.**
  `MergeRoleGroupConfig` merged `resources` struct-by-struct, so a group that set only
  `storage.storageClass` — or only `cpu.min` — dropped every other value the role had configured,
  because the group's enclosing struct was non-nil and the role's was never consulted. Overriding
  one knob is the ordinary way to use this API. The merge is now leaf-granular.
- **A role-level `gracefulShutdownTimeout` now reaches its role groups.** The field carried a CRD
  default of `30s`, and structural defaulting stamps a default into a block as soon as that block
  exists — so any group declaring a `config` for an unrelated reason (just `resources`, say) was
  given `30s`, indistinguishable from an explicit group value, and the role's setting lost. The
  same shape applied to `storage.capacity`, where the consequence was worse: the resulting figure
  is baked into a StatefulSet `volumeClaimTemplate`, which Kubernetes will not let the operator
  change afterwards. Both were previously documented as a known caveat in a code comment.
- `PodDisruptionBudgetSpec.Enabled` was a bare `bool` with a CRD default of `true`, so it read as
  `false` — the zero value — in every Go-constructed spec, silently disabling the PDB for callers
  that build the spec in code rather than YAML.
- `StatefulSetBuilder.WithResources` now honours an explicit zero CPU request instead of skipping
  it: `min: "0"` is a legitimate ask on a burstable workload, and the previous `IsZero()` check
  could not tell it apart from "unset".
- Each framework sidecar carries the probe that fits its role. All three are injected as native
  sidecars (init container, `restartPolicy: Always`; stable since Kubernetes v1.33), and on such a
  container the three probes have three different blast radii: a `readinessProbe` decides the
  **Pod's** ready state, so a failing one pulls every pod of the role group out of every Service; a
  `livenessProbe` restarts only that container; and a `startupProbe` is a precondition for the
  regular containers starting at all — irreversibly, since a probe that never succeeds means they
  never start. The deciding question is therefore whether a sidecar is **in the data path**:
  - **Vector and the JMX exporter are not**, so they now declare `livenessProbe`s and no readiness
    probe. They previously declared *readiness* probes, which let a crash-looping log agent, or a
    metrics scrape timing out during a GC pause, take a healthy product offline. Liveness keeps the
    guarantee "a wedged agent is recovered" without that outage mode; deleting the probe outright
    would have removed both.
    - Vector probes the pipeline's `prometheus_exporter` endpoint rather than the API's `/health`,
      because that exercises the running topology while `/health` only reports that the API server is
      up. Timings tolerate ~2 minutes of failure, since restarting the agent drops its in-memory
      buffer.
    - The JMX exporter's timings are deliberately forgiving (`timeoutSeconds: 10`, ~3 minutes to
      failure). Scraping `/metrics` makes it collect from the JVM over JMX, so its response time
      tracks the product's GC — the original readiness-grade 5s timeout is what made it flap.
  - **oauth2-proxy is**, so it declares a `readinessProbe` plus a `livenessProbe` on `/ping`, never
    `/ready` (a deep health check that would let a runtime IdP outage evict the pod). The Service
    routes to the proxy's port, so a proxy that is not listening genuinely means the pod cannot
    serve — while pod readiness was otherwise decided by the *main* container's probe on the
    product's own port, leaving the pod Ready and receiving traffic during every rollout while the
    proxy had merely been launched. Readiness is fast-start / slow-evict (2s period, ~60s to evict).
    It does **not** get a `startupProbe`: a proxy that could never answer one would stop the
    product's own container from ever starting, which is a larger blast radius than the coupling
    being avoided.
  - Every tolerance window exceeds the default 30s termination grace period, because sidecars are
    stopped after the main container and are probed while draining.
- `vector.VectorReadinessInitialDelaySeconds` and `vector.VectorReadinessPeriodSeconds` are removed;
  the liveness probe's timings are not configurable through constants (use `SidecarConfig.Probes`).
- Orphan cleanup is a confirmed multi-pass state machine: scale to zero, wait for the ordered drain,
  delete, and confirm each resource is gone before the next type is touched; per-group failures no
  longer abort the pass, a 429 surfaces as `*RateLimitError` and backs off instead of marking the
  cluster Degraded, and the role-level PDB of a removed role is reclaimed by label
- A recovered panic returns an error (with a `ReconcilePanic` warning event) instead of recording
  the cycle as a success
- Status converges: role groups are pruned only after a real deletion, the write is skipped when
  nothing changed, it is issued from the in-memory object so product status fields survive, and the
  Degraded verdict is computed once per pass from a sorted role iteration
- `GenericClusterStatus.SetCondition` preserves `LastTransitionTime` unless the status actually
  flips, so a settled cluster stops rewriting its status every reconcile
- `Build()` deep-copies every map, slice and pointer it hands out, so a built object can no longer
  reach the builder or the next role group; a PDB built without a selector (which would block
  eviction of every pod in the namespace) is rejected
- `podOverrides` no longer collide with framework-owned one-of members: a strategic merge patch that
  states another member of a mutually exclusive struct (volume sources, probe and lifecycle
  handlers, `EnvVar` value/valueFrom) drops the framework's member instead of producing an object
  the API server rejects; decode failures are recorded on `MergedConfig.PodOverrideErrors` and
  re-emitted as a `PodOverrideIgnored` warning event
- Generated log4j2 config binds its appender refs and uses consistent logger ids; Python logging no
  longer emits every record twice, and its module is no longer named `logging.py`
- Logger names from the CRD are escaped per target format, so a name containing a space, separator,
  quote or line break can no longer re-shape the generated file
- `.properties` round-trips exactly over the whole grammar (escaped separators, trailing
  backslashes, edge whitespace, `#`/`!` in keys)
- `K8sUtil.SetOwnerReference` resolves the GVK from the scheme; `ExecUtil` selects pods by label
  even with a custom `Executor` injected and reports the command's real exit code
- Vector validates the discovered aggregator address and escapes values interpolated into its config
- The example operator's manager ClusterRole is generated from kubebuilder markers (it previously
  granted only `pods get/list/watch`, so the deployed operator could not start its informers), and
  `main.go` wires the metrics and webhook TLS flags it already parsed

### refactor

- Typed the extension registry per CR and removed the process-wide singleton and the adapter shims
- Shrank `ClusterInterface` to `client.Object` + spec/status projection and removed both runtime
  type assertions from the fetch/apply path
- Removed dead exported API (see BREAKING CHANGES)

### docs

- Documented how a `configOverrides` change reaches running processes, which was previously
  described nowhere: the framework rewrites the role group ConfigMap, and **commons-operator's
  restarter** rolls the pods when the workload carries `restarter.kubedoop.dev/enable=true` and a
  mounted ConfigMap/Secret changes. The SDK does not set that label — it is a per-product decision —
  and does not write the `configmap.restarter.kubedoop.dev/` or `secret.restarter.kubedoop.dev/`
  annotations, which belong to the restarter. `docs/architecture.md` §2.6 previously claimed
  `ProductConfig` "ensures operator upgrades propagate config changes to existing clusters"; it now
  distinguishes recomputing the ConfigMap from delivering it to the processes. `envOverrides`,
  `cliOverrides` and `podOverrides` are unaffected — they change the pod template and roll natively.
- Corrected that restarter documentation against commons-operator's actual implementation. It said
  a product opts in "through `BaseRoleGroupHandler.ExtraLabels`", which put a **deployment**
  decision at the operator's compile time; the label goes on the **cluster CR**, whose labels the
  reconciler propagates into `StatefulSet.metadata.labels` — the only place the restarter's watch
  predicate and its `MatchingLabels` list look, so `podOverrides` (pod template only) cannot enable
  it. Also documented that env-var `valueFrom` references count alongside volume mounts, that
  enabling the label always costs one rollout (the first pass stamps a template that had no
  `<uid>/<resourceVersion>` annotation), and the upstream bug where only the first ConfigMap volume
  of a pod is watched (zncdatadev/commons-operator#298).

- Re-verified every concrete claim in `docs/architecture.md`, `docs/architecture_zh.md` and the
  `AGENTS.md` files against the code; see `docs/DOC_CHANGELOG.md` for the itemized list

### tests

- CI gained three guards it had been missing. `make test` regenerates before it tests, so a commit
  with stale generated files went green and CI tested a tree that differed from the one under
  review — the new `verify-generate` target and job regenerate and fail on any difference, across
  **both** modules. The race detector now runs (on one Kubernetes version), on a framework whose
  whole job is concurrent reconcile loops. And `examples/trino-operator`, a separate Go module that
  no root-module job ever compiled, is now linted and tested; it is the reference implementation
  downstream operators copy, and it was green only because nothing ran it.
- Regenerated `examples/trino-operator`'s CRD, which was stale: it still carried the
  `gracefulShutdownTimeout` CRD default removed from the commons types, so the example operator
  shipped a CRD with the defaulting bug still in it.

- The CRDs envtest installs for the mock cluster resources are now **generated** from the Go types
  by `make manifests` instead of being hand-written. The previous fixtures declared
  `x-kubernetes-preserve-unknown-fields: true` for spec and status and no schema at all, so the API
  server performed no defaulting, no validation and no pruning in ANY test in the repository: every
  `+kubebuilder:default` and `+kubebuilder:validation:*` marker in `pkg/apis` was inert, and tests
  that appeared to exercise defaulting were exercising Go zero values. `make test` now depends on
  `manifests`, and `pkg/testutil/crd_schema_test.go` fails if the schema ever goes missing again.
- Turning the schema on immediately caught one real defect: `UpdateStatusWithRetry`'s spec wrote a
  `metav1.Condition` with no `lastTransitionTime`, which the API server rejects as a required
  field. It had been persisting invalid conditions and reporting success.
- `testutil.AltMockCluster` is exported (it was an unexported type inside a `pkg/reconciler` test
  file), so controller-gen can generate its CRD.

- `pkg/apis` tests now run in CI, and the reconciler suite gained envtest-backed integration and
  resilience specs (a second test CR type, `AltMockCluster`, covers per-CR-type isolation)

## v0.12.6 2025-12-01

### features

- Added PodListeners API with DeepCopy methods (#432)
- Added customizable requeueAfter options to resource reconcilers (#424)
- Added KerberosProvider to Authentication (#405)

### refactor

- Replaced deprecated Requeue with RequeueAfter in reconcilers (#422)
- Updated golangci-lint version and improve Makefile targets (#431)
- Updated Go version from 1.24.9 to 1.25.3 (#402)

### fix

- Enhanced GetLabels() to return immutable labels map (#406)
- Updated Go setup to use go-version-file in lint and test workflows (#403)

### dependencies

- Bumped github.com/onsi/gomega from 1.38.2 to 1.39.0 (#425)
- Bumped github.com/onsi/ginkgo/v2 from 2.27.2 to 2.27.5 (#427, #397)
- Bumped k8s.io deps to 0.35.0 (#430)
- Bumped k8s.io/client-go from 0.34.2 to 0.34.3 and 0.34.1 to 0.34.2 (#418, #410)
- Bumped k8s.io/kubectl from 0.34.1 to 0.34.2 and 0.33.3 to 0.34.1 (#411, #390)
- Bumped sigs.k8s.io/controller-runtime from 0.21.0 to 0.22.4 (#398)
- Bumped golangci/golangci-lint-action from 8 to 9 (#407)
- Bumped actions/checkout from 5 to 6 (#413)
- Bumped DavidAnson/markdownlint-cli2-action from 20 to 21 and 21 to 22 (#412, #416)

## v0.12.5 2025-11-04

### features

- Updated Go version from 1.24.1 to 1.24.9 (#394)

### bugs

- Fixed test workflow does not contain permissions (#396)
- Fixed lint workflow does not contain permissions (#395)

### dependencies

- Bumped sigs.k8s.io/controller-runtime from v0.20.4 to v0.21.0 (#362)
- Bumped k8s.io/client-go from 0.32.3 to 0.33.4 (#374, #368, #364, #356)
- Bumped k8s.io/kubectl from 0.32.3 to 0.33.3 (#369, #365)
- Bumped github.com/onsi/ginkgo/v2 from 2.23.4 to 2.25.1 (#380)
- Bumped github.com/onsi/gomega from 1.37.0 to 1.38.2 (#384, #371)
- Bumped github.com/stretchr/testify from 1.10.0 to 1.11.1 (#385)
- Bumped actions/setup-go from 5 to 6 (#389)
- Bumped actions/checkout from 4 to 5 (#372)
- Bumped DavidAnson/markdownlint-cli2-action from 19 to 20 (#355)

### chore

- Updated CHANGELOG for v0.12.4 release (#400)
- Updated golangci-lint version to v2.1.6 in workflow and Makefile (#361)

## v0.12.4 2025-05-11

### features

- Implemented retry mechanism for resource updates (#342)
- Updated commons operator CRD (#332)
- Updated listener CRD (#337)

### improvements

- Reduced log verbosity for cluster and pdb reconciler (#344)
- Optimized envOverrides handling in BaseWorkloadBuilder (#343)

### bugs

- Fixed order envOverrides in BaseWorkloadBuilder (#341)
- Updated default value for LDAP email field to 'mail' (#339)
- Updated default value for LDAP group field to 'memberof' (#338)

### dependencies

- Bumped golang.org/x/net in the go_modules group (#345, #324)
- Bumped sigs.k8s.io/controller-runtime from 0.20.2 to 0.20.3 (#319)
- Bumped github.com/onsi/ginkgo/v2 from 2.22.2 to 2.23.4 (#333, #327, #318)
- Bumped go to 1.24 and k8s api to 0.23.3 (#328)

### ci

- Added gh action depbot support (#335)
- Bumped ci go version to 1.24 (#334)
- Bumped DavidAnson/markdownlint-cli2-action from 18 to 19 (#336)

## v0.12.3 2025-01-25

### tests

- Enabled k8s matrix testing (#304)

### dependencies

- Bumped k8s.io/kubectl from 0.32.1 to 0.32.2 (#313)
- Bumped k8s.io/client-go from 0.32.1 to 0.32.2 (#314)
- Bumped sigs.k8s.io/controller-runtime from 0.20.0 to 0.20.1 (#308)
- Bumped github.com/onsi/ginkgo/v2 from 2.22.1 to 2.22.2 (#306)
- Bumped github.com/evanphx/json-patch (#310)
- Bumped k8s.io/kubectl from 0.31.3 to 0.32.1 (#300)
- Updated dependencies version in Makefile (#303)
- Bumped k8s.io/client-go from 0.31.4 to 0.32.1 (#298)
- Bumped sigs.k8s.io/controller-runtime from 0.19.3 to 0.20.0 (#301)

## v0.12.2 2025-01-10

### features

- Added support for processing Python log files in Vector (#291)

### refactor

- Added service traffic policy and preferred address type to listener class (#294)
- Added listener read field and updated pod select field (#293)

### bugs

- Fixed incorrect vector data volume name (#289)

### dependencies

- Bumped github.com/onsi/gomega from 1.36.1 to 1.36.2 (#295)
- Bumped github.com/onsi/ginkgo/v2 from 2.22.0 to 2.22.1 (#292)

## v0.12.1 2024-12-12

### refactor

- Improved secret volume scope interface readable (#286)
- Refactored vector build to make it easier to use (#284)
- Used getter methods for labels and annotations in PDB options (#274)
- Removed kubebuilder validation tags to fix CR installation failure (#273)

### dependencies

- Bumped golang from 1.23.2 to 1.23.4 (#283)
- Bumped k8s.io/client-go from 0.31.3 to 0.31.4 (#276)
- Bumped github.com/onsi/gomega from 1.35.1 to 1.36.1 (#275, #269)
- Bumped sigs.k8s.io/controller-runtime from 0.19.1 to 0.19.3 (#272, #267)
- Bumped github.com/onsi/ginkgo/v2 from 2.21.0 to 2.22.0 (#268)

## v0.12.0 2024-11-25

### features

- Added API `CreateDoesNotExist` in client package (#241)

### refactor

- Added option argument to rbac and config in builder (#251)
- Refactored GitHub Action (#244)
- Refactored Makefile to be more standardized (#243)
- Bumped domain to kubedoop.dev (#242)
- Added merge role-group from role support (#239)
- Refactored code base to pass Go lint (#246)
- Merge supports structs and safely handles struct pointers (#247)

### dependencies

- Bumped `k8s.io/kubectl` from 0.31.2 to 0.31.3 (#264)
- Bumped `github.com/stretchr/testify` from 1.9.0 to 1.10.0 (#262)
- Bumped `k8s.io/client-go` from 0.31.2 to 0.31.3 (#263)
- Bumped `k8s.io/api` from 0.31.2 to 0.31.3 (#261)

### bugs

- Fixed the origin cli-args override to empty when `cliOverrides` is nil in builder (#256)
- Fixed `clusterIp` is None when `serviceType` is nodePort in builder (#255)
- Fixed vector config name error (#254)
- Checked container name is passed for log config in productlogging (#253)
- Fixed vector watcher path error (#252)
- Fixed set `gracefulShutdownTimeout` panic in builder (#250)
- Fixed logback render some nil value in productlogging (#240)

### tests

- Updated example data in reconciler tests (#249)

### chore

- Updated shields in README (#245)
- Added code of conduct (#258)
- Changed update interval from weekly to daily (#260)
- Updated project license (#257)

## v0.11.2 2024-11-08

### bugs

- Fixed typo `providerHint` in auth (#234)
- Added the missed `logType` field and corrected typo in productlogging (#231)

### dependencies

- Bumped toolchain in Makefile to latest version (#233)
- Bumped golang from 1.23.0 to 1.23.2 (#232)

### chore

- Refactored GitHub Action (#235)

## v0.11.1 2024-11-06

### bugs

- Fixed nil `RoleConfigSpec` handling in `PodDisruptionBudget` reconciliation

## v0.11.0 2024-11-05

### features

- Added PodDisruptionBudget support and reconciliation logic (#227)
- Added readiness probe for vector (#224)
- Added LoggingSpec support (#210)
- Added AuthenticationSpec support (#209)
- Enhanced rbac builder to add policy rules (#196)

### improvements

- Refactored log config generation in productlogging (#220)
- Replaced slice dereference with ptr.To for replica counts (#223)
- Renamed CommandOverrides to CliOverrides for consistency (#222)
- Unified Options struct to Option type for consistency in builder (#221)
- Changed AuthenticationClass to cluster scope in api (#208)
- Changed authentication.oidc.provisioner to providerHint in api (#207)

### bugs

- Fixed client.Get to reset err and ignore object does not exist error (#219)
- Fixed container env out of order in build (#211)
- Appended vector data dir volume (#199)

### dependencies

- Bumped github.com/onsi/gomega from 1.34.2 to 1.35.1 (#225)
- Bumped github.com/onsi/ginkgo/v2 from 2.20.2 to 2.21.0 (#226)
- Bumped k8s.io/kubectl from 0.31.1 to 0.31.2 (#216)
- Bumped k8s.io/client-go from 0.31.1 to 0.31.2 (#215)
- Bumped sigs.k8s.io/controller-runtime from 0.19.0 to 0.19.1 (#218)

### ci

- Removed doc issue template in GitHub Actions (#206)

## v0.10.0 2024-09-20

### features

- Remove `expirationTime` from constants
- Add `AnnotationSecretsCertRestartBuffer` to constants
- Add inline connection to s3 bucket connection
- Refactor cluster and role reconciler to improve log and error handling, and
improve the cluster stoppped logic
- Add CRD doc for LDAP provider credentials
- Refactor `client.Get`, now it wrapped `ctrlclient.Get` and add `client.GetWithOwnerNamespace` `client.GetWithObject`
- Refactor config builder, remove `AddDecodeData` and `AddDecodeData`. Secret builder use `stringData` now, it don't need to decode data
- Add `AddItem` to config builder to add single item to config
- Add Properties util to config package, support get and set properties, and move xml util to config package.
- Service builder support listener class
- Rename image spec fields, such as `platformVersion` to `kubedoopVersion`

### bugs

- Remove `AddDecodeData` and `AddDecodeData` from config builder, because it only convert string to byte, and it is not necessary

### chore

### dependencies

- bump k8s.io/client-go from 0.31.0 to 0.31.1
- bump k8s.io/kubectl from 0.31.0 to 0.31.1

## v0.9.2 2024-09-08

### features

- Add `listenerclass` type to apis
- Add enrichment and restarter labels to constants
- Replace vector image with product image, now vector command available in product image

### bugs

- Fix image default policy is `Always`, and can not auto referenct CRD image policy

## v0.9.1 2024-09-03

### features

- Add `ServiceType` field to service builder, can build `headless` service
- Add `productlogging` package, and implement logback, log4j, log4j2 configuration
- Add vector builder for easy integration with vector sidecar

### bugs

- Fix re-reconciler with 0 seconds interval when resource not ready
- Fix rbac builder could not set subjects and roleRef
- Fix container builder missing memory request field

### chore

### dependencies

- Bump github.com/onsi/ginkgo/v2 from 2.20.1 to 2.20.2
- Bump github.com/onsi/gomega from 1.34.1 to 1.34.2

## v0.9.0 2024-08-28

**BROKENCHANGE:**

- Bump k8s version to 1.31.0
- Bump golang version to 1.23.0
- Remove `Database` API group

### features

- Add `ServiceType` field to service builder
- Remove include `zncdata` variable
- Remove `Database` API group
- Enchance bash util script, insert `shutdown` to script

### bugs

### chore

- Fix dependabot commit message prefix is not `build`

### dependencies

#### upgrade

- sigs.k8s.io/controller-runtime from 0.18.2 to 0.19.0
- k8s.io/client-go from 0.30.1 to 0.31.0
- k8s.io/api from 0.30.1 to 0.31.0
- k8s.io/apimachinery from 0.30.1 to 0.31.0
- k8s.io/kubectl from 0.30.1 to 0.31.0
- github.com/cisco-open/k8s-objectmatcher from 1.9.0 to 1.10.0
- github.com/onsi/ginkgo/v2 from 2.20.0 to 2.20.1

- golang from 1.22.6 to 1.23.0

## v0.8.7 2024-08-20

### features

- Change stack name from `stack` to `kubedoop`
- Add directory constants for kubedoop system， directories include data, config, logs, kerberso,tls and so on
- Add generic bash script utilities and constants for operator, include some reuseful scripts, vector constants and so on
- Change `Image` struct field `stackVersion` to `platformVersion`, `Repository` to `Repo`

### bugs

### chore

- Add depbot to auto update dependencies
- Add golang lint to Makefile and update gh action use it
- Remove manual dispatch workflow
- Rename image spec fields, such as `stackVersion` to `platformVersion`

### dependencies

#### upgrade

- golang from 1.22.3 to 1.22.6
- github.com/onsi/ginkgo/v2 from 2.19.0 to 2.20.0
- github.com/stretchr/testify from 1.8.4 to 1.9.0


## v0.8.6 2024-08-07

### features

- Added support for logging to standard output and error streams in Vector, including new log sources and transformers.
- Added log collection for log4j and log4j2 XML log files in Vector, introducing corresponding log sources and transformers.
- Added listener and secret constants.
- Added a volume builder for listener and secret.

### bugs

## v0.8.5 2024-07-26

### features

### bugs

- Fix formatting errors that still exist in `vector.yaml`

## v0.8.4 2024-07-26

### features

- `JobBuilder` default implementation `Job` is public now
- Remove `builder.RoleGroupInfo`, because it confuses with `reconciler.RoleGroupInfo`

### bugs

- Fix Can not get image pull policy in workload builder, now you can get `util.Image` object.
- Fix Can not set vaild replicas and the value always be 1
- Fix the cluster is still running when operation be updated `stopped`
- Fix the `vectory.yaml` indent format error

### chore

- `MergeRoleGroupSpec` method can be modified directly on the passed roleGroup object, we did not
  update any code, but add some test cases to ensure it is correct.

## v0.8.3 2024-07-23

### features

- Add listener CRD type to apis

## v0.8.2 2024-07-22

### bugs

- Fix `CreateOrUpdate` method can not handle crd
- Fix xml configurations may not be in the same propretry order after marshal

## v0.8.1 2024-07-11

### features

- Update `EnvsToEnvVars` to `NewEnvVarsFromMap`, because the previous method name was ambiguous
- Update `XMLConfiguration` functions to support add, delete and marshall and construct from xml string
- Extract `CreateOrUpdate` from `client.Client`, so you can use alone

### bugs

- Fix `XMLConfiguration` can not unmarshal xml string to struct
- Fix `XMLConfiguration` can not handle xml string header when unmarshal to marshal
- Fix `XMLConfiguration` marshal xml string contains escape characters, e.g: \n
- Update `GenerateRandomStr` to generate random string with length, letters, numbers and special characters, and add `GenerateSimplePassword`
- Remove the base64 decode of Secret.Data obtained by controller-runtime, because the data is already decoded

### chore

- Remove incorrectly named functions in `util.configuration.go`

## v0.8.0 2024-07-10

### features

**BROKENCHANGE:** Update `reconciler` and `builder` package

- Add vector log builder
- Refactor reconciler and builder, and add test case
- Add image selection
- Remove s3 finalizer constant

### bugs

- Fix code indentation can not handle multiple lines

## v0.7.0 2024-06-27

### features

**BROKENCHANGE:** Update `S3Connection` and `S3Bucket` group to `S3Connection.s3.zncdata.dev` and `S3Bucket.s3.zncdata.dev`, Update `DatabaseConnection` and `Database` group to `DatabaseConnection.database.zncdata.dev` and `Database.database.zncdata.dev`

- Add group `s3.zncdata.dev` to `s3` package, and move `S3Connection` and `S3Bucket` to `s3` package
- Add group `database.zncdata.dev` to `database` package, and move `DatabaseConnection` and `Database` to `database` package
- Add `AuthenticationClass.authentication.zncdata.dev`, and support oidc ldap tls and static
- Use `SecretClass` provide `S3Connection` credential

### chore

- typo fix issue template

## v0.6.0 2024-06-24

### features

- Add `cluster operation` `logging` `pdb` `resource` api to commons
- Add resource builder and basic reconciler, you can implement in operator
- Add s3 client-side to verification tls certificate

### chore

- Add issue template


## v0.5.1 2024-05-23

### features

- Bump go version to 1.22

### chore

- Add go boilerplate to auto generate license header

## v0.5.0 2024-05-21

**BREAKCHANGE** Update github group to `zncdatadev`

### features

- Add `properties` configuration util
- Add `xml` configuration util
- Add code of string intendation, tabs and spaces can be converted to each other
- Add name string generator, use `-` to connect words
- Add StatefulSet check to `CreateOrUpdate`
- Add golang template parse function

## v0.4.0 2024-03-20

### features

- Add base64 util function, support method: `Base64.Encode` and `Base64.Decode`

## v0.3.0 2024-0206

### features

- Update domain to `zncdata.dev`
- Update credential struct name
- Fix `mysql` word typo

## v0.2.1 2024-01-17

### bugs

- remove `DatabaseConnection.spec.provider` enum by CRD validation

## v0.2.0 2024-01-16

### bugs

- fix crds not register to k8s
- update `s3bucket.spec.name` to `bucketName` and `databass.spec.name` to `databaseName`

## v0.1.0 2024-01-11

### features

- Add `DatabaseConnection` and `Database` struct, and implement mysql, postgres, redis.
- Add `S3Conection` and `S3Bucket` struct.
- Add `AuthenticationClass` struct, and implement oidc.
- Add errors and conditions constants
- Add `CreateOrUpdate` for k8s object create or update.
