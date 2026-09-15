# New framework contracts and generated input

**Parent:** [../AGENTS.md](../AGENTS.md)

Design intent: [core specification](../../docs/architecture.md#framework-design).
This subtree implements public contracts and generated registration. The pure resource pipeline lives
in [internal/framework/pipeline](../../internal/framework/pipeline/AGENTS.md);
the internal controller is reached only through framework/operator registration.

## Existing packages

| Package | Current responsibility |
| --- | --- |
| `framework` | ProductDefinition, EffectiveInput, RuntimeDescription and their domain values; facts/read interface, diagnostics/status, shared output states, PropertiesCodec |
| `framework/logging` | Pure Python stdlib dictConfig adapter consuming effective container logging; actual native-process tests |
| `framework/input` | Versioned generated Binding, raw Projection, presence input types, strict decoding, raw config projection and generated-data copying |
| `framework/inputgen` | Generate presence API/CRD and optional registration companions; Check all requested artifacts |
| `framework/dataops` | Independent retained data identity, approved data operation protocol and separate executor registration |
| `framework/operator` | Validate/freeze deployment registration and install the internal direct-client controller; no public mutable reconciler |

The root package imports only standard-library and Kubernetes packages. It does
not import input, inputgen, old SDK execution packages or the discussion prototype.
The input package depends on the root contracts; inputgen uses both. No public
Source, Prepared, Plan, merge function or mutable Reconciler is provided here.

## Product contracts

`ProductDefinition[C,S,F]` uses concrete effective C/S/F data. `GenerateCluster`
returns `(ClusterOutput, error)`: Ready supplies a complete ConfigMap inventory,
including an empty withdrawal; Pending requires a reason and no partial output.
`ValidateClusterOutput` checks only this data/state contract. The
[internal controller](../../internal/framework/controller/AGENTS.md) validates
resource ownership and reconciles the complete shared output set, preserving old
outputs when generation is Pending or fails.

`LogOutput` declares actual files and has no Collect field. The internal pipeline
consumes effective `Logging.EnableVectorAgent` and composes Vector only when enabled
and there are actual outputs. Helper images are explicit deployment inputs; the
[Trino reference executable](../../examples/trino-operator/README.md) uses generated
registration. Live-workload acceptance is recorded separately from API tests.
`FactsReader.Get` takes a
`types.NamespacedName` and `FactResource` (`metav1.Object` + `runtime.Object`),
without importing or exposing a writable controller-runtime client.

`FileContent` and `PropertyValue` are closed value variants. `PropertyCodec` is a
pure encoder seam; the included PropertiesCodec writes deterministic UTF-8 data
and rejects invalid UTF-8. A custom Go codec is not a runtime helper capability.
Status uses its own DeepCopy methods, without an inverse dependency on input.

`Resources.Storage` is the standard ephemeral/persistent data-storage domain.
`Directory.Data` binds one runtime data directory to that effective configuration;
class/capacity are not repeated in RuntimeDescription. Other directories default to ephemeral;
SecretVolume and ListenerVolume explicitly select read-only platform sources.
The internal physical retained-slot representation is not a product API.

## Generated input contract

`input.ContractVersion` is 1, independent of SDK release versions. Generated Go
source records `InputContractVersion`; Check validates it before byte comparison.
`input.CheckVersion` rejects incompatible versions. The companion checks its own version and operator registration validates the generated binding version.

`input.Binding[CR]` contains Version, Roles, AddToScheme, NewObject, Operation,
Project and Status. Generated `Binding()` creates a new role slice on each call.
The constraint requires Kubernetes object interfaces, not a product-specific CR base.

`Projection` contains Cluster identity/labels, raw Image, raw ClusterConfig containing
framework platform fields plus product fields, and all declared Roles. Each Role has raw Config, role-only RoleConfig, optional
Replicas, Overrides and Groups. Each Group holds its own raw layer and optional
replicas. Projection does not fold defaults/replicas, duplicate role layers into
groups, hold F, resolve config, or include an execution plan. Empty roles survive.
`Operation(cr)` reads fixed controls separately; Project strips operation fields from
ClusterConfig. The pipeline separates framework platform inputs from product S before folding.

`DecodeJSON` validates exact field names, duplicate keys, non-null ordinary input
and native Quantity/Duration/Affinity shapes before assigning a fresh result.
Failure leaves the caller's destination unchanged. Pod RawMessage permits native
null/$patch but still rejects duplicate keys on local Decode. API pruning is a
separate behavior: normal typed GET follows API-server validation, not local Decode.

Project is for inputs from strict Decode or typed API reads. It marshals config
presence and copies override channels; it is not a replacement for final override
or resource validation. Clone copies only the generated data profile, not arbitrary
Go program state. Kubernetes metadata and status use dedicated copy methods.

## Generation and checks

`inputgen.Generate[C,S](Names, roles)` returns GoSource, CRD and optional RegistrationSource. It checks exported
root types/`struct{}`, fixed product type profile, field/name collisions and roles.
The generated API exposes Decode, Operation, Project and Binding; it does not
import original product config types or a controller. Nonempty `Names.ImportPath`
emits a separate typed registration companion, leaving C/S fixed and F inferred.

The CRD has no inherited defaults. Config collection limits remain 32 and native
Affinity collections 16; status lists are not constrained to those config limits.
Generate/Check do not write files; the caller owns its generator command and IO.

`inputgen/integration_test.go` builds a temporary `example.com/framework-consumer`
module from testdata using this checkout via replace. It generates two CRDs,
compiles external product/binding/registration code and exercises an envtest API
and a real manager with dependency refresh through the generated entry point. This is not
a new product case or a new SDK module. An explicit invalid KUBEBUILDER_ASSETS
fails; API tests can skip only when no assets were supplied or found. Full root
`make test` supplies assets, so a local skip is not accepted as API verification.

Run focused tests with `go test ./pkg/framework/...`; use the root `make lint`
and `make test GOTESTFLAGS='-p=2'` before committing. Changes to the generator must
also pass the external consumer test, not only source-string assertions.

`ResolveVectorDestination` resolves the standard same-namespace ConfigMap ADDRESS
through FactsReader. Missing objects are Pending, invalid addresses Invalid, and
API read errors stay errors. Controller-owned observations carry UID/RV. The
internal controller selects this dependency only after a generated runtime has
actual log outputs and effective Vector selection is enabled. `AssemblyOptions`
carries the resolved address into native Vector sink generation.

`Process` now declares native lifecycle/probes. `RuntimeDescription.Initializers` runs in order
after materialization, with explicit per-process directory access. `Coordination` supplies bounded
progress and shutdown priority; the controller persists observations and serializes scale-down.

`ResolveAuthenticationClass` reads the exact cluster-scoped platform class and returns a
single typed provider branch with references only. Product adapters explicitly select
supported providers; this parse result never asserts login or external provider health.

`ClusterConfig` owns authentication references and Vector destination references
alongside the separate operation controls and product S. EffectiveInput.Platform
and FactInput.Platform carry that common half. Directory.Secret/Listener represent
platform sources; GroupOutcome.Platform and GroupReconcileStatus.Platform carry
post-creation observations and Listener addresses. They do not expose credentials.
