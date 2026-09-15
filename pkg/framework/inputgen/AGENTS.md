# Framework input and registration generation

Parent: [../AGENTS.md](../AGENTS.md). Design: [architecture](../../../docs/architecture.md#framework-design), sections 7–8.

`Generate[C,S](Names, roles)` emits deterministic presence API and structural CRD
bytes. `Names.ImportPath` is an optional, explicit Go import path for the generated
API package, independent of output directories. Nonempty produces
`Artifacts.RegistrationSource` for a separate `registration` package; empty retains
API-only generation. Generate and Check never write files.

Generated API imports framework/input and public domain/status values. It does
not import operator, internal execution packages or the original C/S packages.
The registration companion imports the API package, the original concrete C/S
types and framework/operator. It exposes `Options[F]` and
`Register(manager, definition, options) error`; C/S are fixed, F is inferred.
Products must keep their definition/types package independent from generated API
and registration packages to avoid import cycles. Static type names and import
aliases are checked and deterministic; external compilation remains required.

Both source artifacts record an integer `InputContractVersion`. Companion Register
checks its own version before passing the API Binding to operator registration,
which checks the binding version. `Check(expected, actualGo, actualCRD,
actualRegistration...)` supports the existing API-only call. A generated companion
requires exactly one actual nonempty source; missing, extra, unsupported-version
or changed companion content is rejected. Unexpected companions are not ignored.

The external-module fixture uses this checkout through replace and disables
network module lookup. It generates/compiles Trino and presence API companions,
then exercises real envtest persistence and a manager created through generated
Register. Catalog facts refresh from an external ConfigMap without CR edits;
missing/invalid references retain previous resources; updated/new-UID sources
reach actual generated catalog bytes after final file overrides. There is no
kubelet, product process or restarter in that test.

Run `go test ./pkg/framework/inputgen`. An explicit invalid KUBEBUILDER_ASSETS must
fail; envtest skips only if neither explicit nor discovered assets exist. Full
root checks provide envtest assets, so API verification requires non-skipped output.

Standard storage input is generated from `framework.Storage` with presence fields.
The CRD restricts the type to ephemeral/persistent and rejects class/capacity alongside
an explicit ephemeral type. Partial persistent fields are allowed for role inheritance;
final completeness is checked by the pipeline. No admission defaults are generated.

The flat clusterConfig schema combines operation fields, framework.ClusterConfig
and product S with collision checks across all three. Generated status includes
PlatformObservation diagnostics and observed Listener addresses, with no admission
defaults or input inheritance rules applied to status.
