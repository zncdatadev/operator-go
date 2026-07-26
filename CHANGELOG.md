<!-- markdownlint-disable -->
# CHANGELOG

## v0.13.0 (unreleased)

Architecture review follow-up: four waves of correctness fixes across the reconcile loop, the
builders, config/logging rendering and the CSI wiring, followed by three API redesigns that shrink
the contracts a product operator implements. **Downstream operators must migrate — the breaking
changes are listed below.**

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

### security

- Every framework-injected sidecar now carries a hardened container security context by default
  (`sidecar.DefaultSecurityContext()`: `runAsNonRoot`, `allowPrivilegeEscalation: false`,
  `capabilities: drop ALL`, `seccompProfile: RuntimeDefault`). Previously oauth2-proxy and the JMX
  exporter shipped with a nil security context while the product's own container was hardened, so a
  namespace enforcing the restricted Pod Security Standard rejected the whole workload — the
  sidecars were the reason the pod could not be admitted. An explicit
  `SidecarConfig.SecurityContext` still replaces the default wholesale.
- The Vector agent's API no longer binds `0.0.0.0`. It is Vector's unauthenticated GraphQL
  endpoint, and `vector tap` over it streams the log events flowing through the pipeline — the
  product's application logs — so it was reachable from anywhere in the pod network. It now binds
  `127.0.0.1` and stays enabled for in-pod debugging (`kubectl exec` + `vector top`).

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

- Re-verified every concrete claim in `docs/architecture.md`, `docs/architecture_zh.md` and the
  `AGENTS.md` files against the code; see `docs/DOC_CHANGELOG.md` for the itemized list

### tests

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
