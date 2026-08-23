<!-- markdownlint-disable -->
# CHANGELOG

## v0.13.0 2026-08-23

The framework was redesigned during this cycle. `GenericReconciler` + `RoleGroupHandler` replace the per-resource reconciler tree, a product declares its roles as data through `RoleProvider`, and the framework folds the `config` block before anything is built. Upgrading from v0.12.6 is a migration, not a dependency bump — read the breaking changes below alongside `docs/architecture.md` and `AGENTS.md`.

### breaking changes

- Framework architecture redesigned around `GenericReconciler`, `RoleGroupHandler` and a per-CR extension registry; the per-resource reconciler tree is gone (#441)
- `ClusterInterface` now embeds `client.Object` and declares only `GetSpec`/`GetStatus`. Product CRs delete `SetStatus`, `GetObjectMeta`, `GetScheme`, `GetUID`, `GetRuntimeObject` and `DeepCopyCluster`, and are registered with the scheme as themselves — the CR is the object the client reads into. `GenericReconciler`, `GenericReconcilerConfig` and `NewGenericReconciler` are constrained by `common.ClusterResource[CR]`, so the CR type must expose `DeepCopy() *T`. `common.ClusterObject`, `testutil.ClusterWrapper` and `testutil.WrapMockCluster` are removed (#539)
- `common.ExtensionRegistry` is generic over the CR type; build it with `common.NewExtensionRegistry[*MyCluster]()`. The process-wide `GetExtensionRegistry`/`ResetExtensionRegistry` and the `As*Extension` adapters are removed; the `Register*WithPriority`/`WithOptions` variants fold into variadic options (#539)
- `config.ConfigFormat` splits into `config.ConfigMarshaler` (required) and `config.ConfigUnmarshaler` (optional); a file name matching several registered extensions now selects the longest (#539)
- `RoleSpec`/`RoleGroupSpec` override fields are flattened rather than nested under an `overrides` field (#447)
- `RoleGroupHandler` no longer declares `GetContainerImage`/`GetContainerPorts`/`GetServicePorts` (#457)
- `SecretClassVolumeBuilder` is replaced by the declarative `security.SecretProvisioner` and the listener builders by `listener.ListenerProvisioner`, both reaching the pod through `VolumeProvider`; the dead listener Service API is removed (#478, #479, #510)
- `NewOAuth2ProxySidecarProvider` drops its `cookieSeed` parameter, `DeterministicCookieSecret` and `WithOAuth2ProxyCookieSecret` are removed, and an authorization policy option is now mandatory. Downstream operators add a `COOKIE_SECRET` key to the Secret they already create (#541)
- Commons quantity fields become pointers so a CRD default can no longer beat a role-level value: `StorageResource.Capacity`, `CPUResource.Min`/`Max` and `MemoryResource.Limit` are `*resource.Quantity`, `RoleGroupConfigSpec.GracefulShutdownTimeout` is `*string`, `PodDisruptionBudgetSpec.Enabled` is `*bool`, `StorageSpec.StorageClass` is `*string`. Product CRDs must be regenerated; CR YAML is unaffected (#544, #567)
- `LogLevelSpec.Level` loses `+kubebuilder:default:="INFO"`; downstream CRDs lose `default: INFO` on every logging level at their next bump (#573)
- `ExtraLabels`/`ExtraAnnotations` are removed, leaving one label channel (#555)
- The framework no longer guesses a liveness probe for the product's container; use `builder.DefaultTCPLivenessProbe(port)` or `WithLivenessProbe` (#562)
- `Degraded` is a fault signal and is no longer derived from replica counts, so it no longer fires during a rolling update, scale-up or scale-down (#563)
- `BaseRoleGroupHandler.ProductName` only names the product; the new `ImageDefaults` supplies whatever `spec.image` leaves empty, evaluated every reconcile (#581)
- `GenericReconcilerConfig.ProductConfig` gains a `ctx`, a `client.Client` and an error return (#591)
- The workload ServiceAccount is derived per CR as `<lowercased kind>-<cluster>` and is no longer configurable; `GenericReconcilerConfig.WorkloadRBACRules` owns its permissions (#528, #616)
- `BaseRoleGroupHandler` carries no per-role state: declare roles through `GenericReconcilerConfig.RoleProvider` and derive config through `RoleGroupResolver`; `NewBaseRoleGroupHandler` takes only a scheme. `StatefulSetBuilder.WithMainContainerCustomizer`, `MainContainerViolations` and `AddEnvVar`, `ConfigMerger.MergeBeneath`, `reconciler.DefaultAntiAffinity` and `productlogging.ContainerLogging.OwnConfigFile` are removed; `FoldCommonConfig` returns three values, and `affinity: {}` now inherits rather than clears (#632)

### features

- Role groups can ship arbitrary extra product resources through the framework's apply path, and the contract is checked before anything is applied (#514, #619)
- Framework-owned shared Vector log pipeline with a producer/consumer split and a framework-rendered `vector.yaml` (#501, #503)
- Product logging extracted into `pkg/productlogging` with declarative container logging, a restored log4j 1.x generator for products on reload4j, and a log tag decoupled from the pod container name (#495, #513, #613)
- Layered config merge with a computed product-config layer; a product can default the role config and pick storage per role (#496, #623)
- `EnsureGeneratedSecret` for values that must not converge, and a shared ensure-helper for product discovery ConfigMaps (#518, #583)
- Identity-label selectors, native-sidecar injection and role-scoped resource names (#494)
- The whole recommended label set is emitted, not half of it (#554)
- Hardened default pod and container SecurityContext, and sidecars that run the product image instead of hardcoded defaults (#467, #499)
- Configurable probes via the native Kubernetes Probe API (#471)
- `MetricsService` builder and `RoleGroupResources` support, with an opt-in named `targetPort` (#462, #519)
- The base handler consumes role group `affinity` and `gracefulShutdownTimeout` (#517)
- Per-role `MainContainerName` and logging containers (#531)
- A handler can declare intent before `Build` instead of patching the result afterwards (#585)
- A removed role group's product extras are reclaimed, and the orphan cleanup state machine emits metrics (#566, #567)
- An extension hook can report "not ready yet" without the cluster reporting a fault (#625)
- Image tag validation, and the `pullSecretName` the CRDs already promised (#622)
- `constant.JMXJavaAgentOpt` renders the JMX java agent option (#595)
- `PodSpec.EnableServiceLinks` defaults to false, overridable via `podOverrides` (#506)
- `testutil.HaveNoInheritedConfigDefaults` exported as a guard against CRD defaults inside a folded config block (#580)
- Framework enhancements for the spark-k8s-operator migration: S3, oauth2-proxy, podOverrides fidelity, log file naming (#538)
- SDK coverage improvements and assorted fixes (#450)

### refactor

- Constants system redesigned with a hybrid architecture (#464)
- Sidecar injection quality and correctness (#466)
- Manual pointer helpers replaced with `ptr.To` (#480)
- `GetUID` returns `types.UID` (#461)
- Trino example migrated to the SDK `ImageSpec` and stripped of its `GenericClusterSpec` embedding (#449, #472)

### fix

- Roles reconcile best-effort instead of aborting on the first failure (#546)
- A CR being deleted is no longer reconciled (#540)
- `ClusterOperation` pause/stop is evaluated before any mutation, and a stopped cluster still reconciles every resource with replicas forced to 0 (#512, #529)
- `applyResource` propagates desired state on update, and reports a dropped immutable-field change instead of preserving it silently (#527, #545)
- The data PVC converges instead of wedging or silently unmounting, and an unchanged one is no longer reported as a dropped change (#618, #628)
- A `batchv1.Job` survives the framework's own ExtraResources channel (#624)
- The restarter's pod-template annotation is no longer wiped on every apply (#614)
- Orphans are discovered from the live cluster, not just the status ledger; their PVCs are deleted after the drain rather than before the scale-down; the state machine's progress annotations are reset on re-added groups (#550, #556, #565)
- The PodDisruptionBudget is emitted once per role, not once per role group (#530)
- A role group slot's identity is neither forgeable nor unaddressable (#611)
- A `podOverrides` mount that displaces a framework volume is rejected (#557)
- A misspelled `affinity` key is rejected instead of scheduling pods anywhere (#564)
- Each sidecar gets the probe that fits it, injected containers are hardened, and observability no longer gates traffic (#542, #548)
- The oauth2-proxy session key no longer leaks, and the proxy no longer admits everyone (#541)
- CRD defaults no longer override role-level config (#544)
- The auto-created ServiceAccount is bound to workload pods (#498)
- `fsGroup` is paired with `OnRootMismatch` so it is applied once rather than on every start (#561)
- A credential scope name carrying `,` or `=` no longer widens the credential (#559)
- The class annotation is omitted for by-name listener volume registrations (#520)
- The label key derived from cluster and role group names is bounded (#560)
- The v0.12.6 stable Vector log pipeline is restored (edge parsing, stable event schema) (#523)
- Root logger appender refs are bound in the log4j2 renderer (#537)
- Config env vars are emitted in sorted order to stop reconcile churn (#534)
- The config ConfigMap is mounted regardless of `ConfigFiles`, at the kubedoop-canonical path (#507, #508)
- The product image reaches sidecars for embedding handlers (#536)
- Per-CR inputs get a per-call home instead of the shared handler (#582)
- `fetchCR` uses a prototype instead of a nil zero value (#453)
- The INI format is registered in `RegisterDefaultFormats` (#465)
- The documented event vocabulary matches the emitted one (#558)
- Examples are excluded from the controller-gen paths (#451)
- Trino example: the coordinator service name is derived from the spec, and the hardcoded image string is gone (#454, #456)

### tests

- The envtest CRDs are generated from the Go types instead of hand-written, so `+kubebuilder` markers are actually exercised (#543)
- The two Service apply rules that carry weight are pinned (#620)
- The shared `FakeRecorder` buffer is large enough to avoid a mid-suite hang (#532)

### ci

- Fail on stale generated files, run the race detector, and cover the `examples/trino-operator` module in both lint and test (#551)

### docs

- README rewritten, with a Chinese counterpart (#442, #443)
- `architecture_zh.md` removed, leaving one architecture document (#568)
- The changelog is generated at release time from the commit history, not written per PR (#621)
- Per-package `AGENTS.md` guides added and kept current (#460, #463)
- Architecture, security, sidecar, logging, S3 and example-CRD documentation corrected and expanded throughout the cycle (#448, #455, #515, #524, #533, #549, #572, #592, #617, #630, #633)
- Documentation changes are tracked in detail in `docs/DOC_CHANGELOG.md`

### chore

- `.serena` added to `.gitignore`, and the AI worktree configuration synced with main (#468, #492)

### dependencies

- Dropped five direct dependencies: `github.com/cisco-open/k8s-objectmatcher`, `github.com/evanphx/json-patch`, `github.com/pkg/errors`, `github.com/stretchr/testify` and `k8s.io/kubectl`
- Added `github.com/prometheus/client_golang` v1.23.2, `k8s.io/apiextensions-apiserver` v0.35.4, `gopkg.in/yaml.v3` v3.0.1 and `sigs.k8s.io/yaml` v1.6.0
- Bumped sigs.k8s.io/controller-runtime from 0.22.4 to 0.23.3 (#434, #435, #452)
- Bumped k8s.io deps from 0.35.0 to 0.35.4 (#444, #445, #458, #474)
- Bumped github.com/onsi/ginkgo/v2 from 2.27.5 to 2.28.3 (#437, #486)
- Bumped github.com/onsi/gomega from 1.39.0 to 1.40.0 (#436)
- Bumped golang.org/x/net (#504, #505)
- Bumped github.com/moby/spdystream (#476)
- Bumped actions/setup-go from 6 to 7 (#535)
- Bumped DavidAnson/markdownlint-cli2-action from 22 to 24 (#459, #522)

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
