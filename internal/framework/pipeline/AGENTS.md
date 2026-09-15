# Internal config-to-resource pipeline

Parent repository instructions: [AGENTS.md](../../../AGENTS.md).
Design intent: [core specification](../../../docs/architecture.md#framework-design).

This package implements the internal build pipeline. It does not contain a
Kubernetes reconciler, a dependency reader, a resource applier or a product-specific
production entry point. Products import `pkg/framework`; Source, Prepared and Plan
remain internal execution structures.

## Existing code responsibilities

| Files | Responsibility |
| --- | --- |
| `contracts.go` | Internal source, prepared, group/role resource and plan structures; shared public domain types and fixed helper identifiers |
| `source.go` | Clone raw Projection, preserve complete roles, fold replica presence, combine separately supplied facts/operation; pure `Build` entry |
| `definition_validation.go`, `input.go`, `cluster_config.go` | Supported data profile and fixed public/product/cluster configuration resolution |
| `image_config.go`, `affinity_domain.go`, `duration_domain.go` | Presence-aware image selection and fixed native domain rules |
| `naming.go`, `role_config.go` | Complete source identity checks, derived resource names, role-only config and PDBs from all declared replicas |
| `prepare.go`, `generation.go`, `build.go` | Isolated effective inputs, explicit fact outcomes, product validation/generation, fixed pipeline order and shared output result validation |
| `runtime_validation.go`, `overrides.go`, `file_overrides.go` | Runtime references, isolated declarations and layered file/env/CLI overrides |
| `resource_build.go`, `vector_assembly.go` | Platform collector selection, materialization plan, fixed resource assembly and final PodTemplate patches |
| `resource_checks.go`, `retained_data.go`, `helpers.go` | Independent final structure and modeled-consumer checks, retained slot declaration and common lookup helpers |
| `materialization.go` | Versioned file plan, static encoding, bounded PodName binding and confined filesystem writes |
| `testinput/`, `trino_*_fixture_test.go` | Test-only generated input/CRD and existing Trino adapter shapes; no production Trino import or runtime proof |

`cmd/materialize` is a separate executable in the root module. It imports this
formal implementation and does not use the discussion prototype or old SDK execution packages.

## Execution and ownership of data

`Build` takes a generated raw Projection, a ProductDefinition, base facts and
assembly options. Its default operation is normal execution. Internal execution
code can use `SourceFromProjection`, then `PrepareInputs`, then
`BuildPreparedResources` to provide independent operation and per-group facts.
These functions construct plans and do not read or write Kubernetes objects.
`GeneratePreparedGroups` and `AssemblePreparedGroups` split generation from
assembly for runtime-dependent references: the controller inspects actual
LogOutputs between them, without running product generation twice.

The source boundary folds replicas as 1 -> role -> group. It retains empty roles,
raw config layers and independently copied overrides. `PrepareInputs` validates
shape/definition, resolves clusterConfig/image/config and records all declared
identities without invoking product callbacks or performing external IO.

A non-nil facts map must supply each configuration-valid group. Missing results
become Invalid, unresolved groups get no new resource set, and unknown group keys
are errors. A nil map uses base facts. Product validation gets isolated snapshots;
generation sees input failures in the complete topology. Final group outcomes,
including assembly failures, are supplied to the shared-output callback.

Resource assembly composes platform files before file overrides, then applies
role/group file, env and CLI overrides. It assembles the complete Pod before
role.podOverrides and then group.podOverrides. A role Pod patch therefore outranks
group env/CLI. Patches never feed back into effective configuration or regeneration.

`Check` has consistent/conflict/unknown states. Conflict withholds that group's
resource set. Unknown preserves a limited premise; it cannot suppress an independent
known conflict. Product final validation receives copies and cannot repair the plan.
The framework copies declared data, not arbitrary mutable state captured by callbacks
or codecs.

## Platform collector and helpers

`composeVector` consumes `Common.Logging.EnableVectorAgent`. `LogOutput` declares
actual files and has no Collect field. Enabled collection consumes all declared
outputs; disabled or empty output inventories create no collector. No outputs
means no Vector image requirement. No configuration files means no materializer
image requirement either.

Selected Vector uses generated `vector.yaml` and file sources. A resolved
`AssemblyOptions.VectorDestination` selects a native Vector protocol sink; nil
selects stdout JSON. The destination also enters the collector's template env so
a discovery update reaches the running process without a separate restarter.
Its file participates in ordinary file overrides. Consumer checks compare the
framework-selected baseline with the final Pod and files. A user-added container,
or a main process called vector while collection is unselected, cannot produce
`LogCollectionKnown=true`. That boolean is structural evidence, not log delivery.

Materialization plan version is `v1`; runtime Properties codec is `properties-v1`.
Static custom codecs execute while building the plan. Deferred PodName binding
requires the built-in PropertiesCodec. The executable accepts required `--plan`
and `--root`, takes the binding from `POD_NAME`, and rejects positional arguments.
The assembler uses `/plan/materialization.json` and `/materialized`.

Materialize resolves all values before writing within an exclusively owned output
tree. It replaces individual files through temporary inodes, does not clean stale
files, and does not provide whole-plan atomicity or global ownership changes.
Image packaging and kubelet execution are verified by the formal delivery harness,
not by this package's unit tests.

## Identity and lifecycle limits

Complete source identities validate group names and the headless suffix before
successful group outputs can narrow the inventory. Ordinary/headless Service names
use DNS1035 label rules; StatefulSet/ConfigMap/PDB names use DNS1123 subdomain rules.
The inventory checks ordinary/headless cross-group collisions without truncation.
Generated addresses and resources use the same GroupIdentity naming methods.

Role PDBs use all declared replicas, including groups with failed config or pending
facts. Stopped plans change only successful StatefulSet execution replicas to zero;
this package does not stop or observe existing Pods.

`storage.go` folds the standard storage discriminator outside generic object merging.
Changing type clears the inherited branch; the final persistent branch requires class and positive capacity.
One `Directory.Data` binds effective `Resources.Storage`: ephemeral becomes emptyDir;
persistent becomes RWO/Filesystem with Retain/Retain. Persistent input without a Data consumer fails. It cannot
also hold generated configuration or declared log outputs. Final checks reject
incompatible data mount changes. No live PVC/PV reads, provenance receipts, binding
checks, reclaim checks, data adoption or cleanup are implemented here.

ResourcePlan carries `ClusterOutput` plus `ClusterError`. Ready is a complete shared
ConfigMap set, including empty withdrawal; Pending requires a reason and no partial
set. An absent callback is Ready/empty. Invalid output or conflicting producers fail
the shared output without discarding otherwise valid group plans. These are plan
contracts; ownership, application and withdrawal cleanup belong to U03.
Shared ConfigMaps cannot occupy any declared group's reserved ConfigMap slot,
including a group withheld by invalid config or pending facts.

## Verification and generated fixture

Pure tests cover configuration/presence, image/native fields, names, PDBs, overrides,
materialization, retained mounts, Vector selection and independent final checks.
The helper command tests build and execute a local binary. Neither is evidence of a
container image, kubelet, Trino or Vector running.

The test-only generated API is checked by `TestInputFixtureCurrent`. To intentionally
regenerate it after changing the test definition or inputgen:

```sh
go test ./internal/framework/pipeline -run '^TestInputFixtureCurrent$' -update-input-fixture
```

The persisted-input test uses envtest to create/read the generated CR, build a plan,
create its resources and materialize bytes locally. It requires real assets and fails
when they are unavailable. It does not run Kubernetes workload controllers or Pods.
Focused commands include:

```sh
go test ./internal/framework/pipeline -run 'Test(BuildGroup|Vector|AssemblyChecks)' -count=1
go test ./cmd/materialize -count=1
go test ./internal/framework/pipeline -run '^TestPersistedInputBuildsResourcesAndMaterializedBytes$' -count=1
```

Supply `KUBEBUILDER_ASSETS` for API acceptance. The current checkout also locates the
repository's Kubernetes 1.35 assets when that variable is unset. Run root `make lint`
and `make test GOTESTFLAGS='-p=2'` for the final gate, and record actual results in the
U02 implementation record before describing the unit as accepted.

`lifecycle.go` validates native probes/ordered initializer declarations and reports lifecycle
premise changes after Pod overrides. Assembly runs initializers after materialization, uses explicit
OrderedReady/RollingUpdate for coordinated workloads and clones every mutable lifecycle/probe value.
