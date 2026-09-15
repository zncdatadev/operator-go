# Framework registration

Parent: [../AGENTS.md](../AGENTS.md). Design: [architecture](../../../docs/architecture.md#framework-design), sections 7–8.

This package implements the public deployment seam used by generated registration
companions. `Options[C,S,F]` contains base `Facts`, optional read-only `ResolveFacts`,
`Assembly` and `FactRefreshInterval`. Zero refresh selects the controller default;
negative intervals are rejected.

`Register(manager, definition, options, input.Binding[CR]) error` validates the
explicit generated contract version, complete binding, config/facts type profiles
and exact role inventory before touching the manager. It copies mutable defaults,
base facts, role names and helper identity. Callback identities remain unchanged;
authors remain responsible for state captured by closures.

Registration adds core/apps/policy/storage/data ledger and generated API types to the manager
scheme, constructs an independent direct API client using its configuration,
HTTP client and REST mapper, then installs the internal controller. It does not
return a reconciler, read CRs, evaluate business defaults, resolve dependencies,
install CRD/RBAC or start the manager. Register before `manager.Start`.

Ordinary products call their generated `registration.Register`, which fixes C/S
and infers F. The public input binding is generated-code plumbing. No public
Source/Prepared/Plan or configurable execution stages are added here.

Tests: `go test ./pkg/framework/operator`; actual external-module registration,
manager startup and dependency refresh are verified by
`TestExternalConsumerGenerationAndAPIRoundtrip` in `../inputgen`.

DataAsset scheme registration supports automatic retained-data identity records.
It does not start the independent DataOperation executor or grant destructive
permissions; deployment of that executor is a separate composition decision.
