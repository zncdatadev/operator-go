# operator-go/examples/trino-operator - Trino Operator Example

**Parent:** [../../AGENTS.md](../../AGENTS.md)
**Generated:** 2026-03-29

Complete Trino operator implementation demonstrating the operator-go framework with CRD definitions, reconciliation logic, and resource builders.

## Key Directories

| Directory | Purpose |
|-----------|---------|
| `api/` | Trino CRD definitions |
| `cmd/` | Operator entrypoint (wires the handler as both `RoleGroupHandler` and `RoleProvider`, plus `RoleGroupResolver` and `ImageResolution`, into `GenericReconciler`) |
| `config/` | Kubernetes manifests and kustomize configs |
| `internal/controller/` | `TrinoRoleGroupHandler` — embeds the SDK `BaseRoleGroupHandler`; the framework owns resource orchestration |
| `internal/product/` | `RoleGroupResolver` (`product.ComputeConfig`): Trino's `config.properties` (role-branched, derived from the role group's effective config) returned as a `*reconciler.Contribution` |
| `internal/config/` | `JVMConfigBuilder` (non-key-value `jvm.config`) and `CatalogConfigBuilder` |
| `internal/extensions/` | Catalog validation + health + discovery ConfigMap (ClusterExtension / RoleExtension; discovery uses `reconciler.EnsureDiscoveryConfigMap`) |

## Architecture (declare → fold → derive)

This example demonstrates the SDK's preferred division of labour:

- **Framework owns the 90%.** `TrinoRoleGroupHandler` embeds `reconciler.BaseRoleGroupHandler`, so the ConfigMap, Services, StatefulSet (with sidecars + `podOverrides` applied), and PDB are built by the SDK. The handler itself carries only reconcile-invariant collaborators — `ConfigMountPath` (`/etc/trino`) and the `ConfigGenerator`. Everything role-shaped (primary container name `trino`, per-role ports, the Log4j2 log producer) is stated by `DeclareRoles`, which implements `reconciler.RoleProvider` and is called once per pass with the cr in hand.
- **Logging is fully framework-owned.** `RoleDeclaration.LogProducers` declares only the container + framework (no output file — the framework derives `<LogDir>/<lowercased container>/<container>.<framework suffix>`, e.g. `.log4j.xml` for log4j/logback). When a role group enables the Vector agent, the SDK's Vector provider is the single owner of the shared log volume (creates it, RW-mounts the producer, mounts it on the agent, which pre-creates the per-container log dirs before exec'ing vector), and — because `TrinoCluster` implements `reconciler.VectorAggregatorProvider` (`VectorAggregatorConfigMapName()` from `spec.clusterConfig`) — the framework also generates `vector.yaml` into the ConfigMap. The product writes no Vector wiring by hand.
- **Derived config flows as data through the merge pipeline.** `product.ComputeConfig` computes `config.properties` from the role group's **effective** config and returns a `*reconciler.Contribution`, wired via `GenericReconcilerConfig.RoleGroupResolver` — the lowest merge layer, so any user `configOverrides` in the CRD always win. This is config generation (recomputed every reconcile), not webhook defaulting.
- **Scheduling and shutdown are declarative.** The framework consumes the role group config's `affinity` (a raw `corev1.Affinity`) and `gracefulShutdownTimeout` (mapped to `terminationGracePeriodSeconds`); user `podOverrides` keep precedence over both. The sample CR demonstrates `gracefulShutdownTimeout`.
- **Discovery is a one-liner.** `extensions.DiscoveryExtension` (a `ClusterExtension` running PostReconcile) publishes the coordinator URI in a discovery ConfigMap named after the cluster via `reconciler.EnsureDiscoveryConfigMap` — the framework owns CreateOrUpdate + controller owner reference + canonical labels; the product only computes the data map.
- **Escape hatch for what the pipeline can't model.** `BuildResources` calls the base, then appends the CR-driven image, the non-key-value `jvm.config`, and coordinator-only catalog files. There is no hand-built `StatefulSet`.

## Working Instructions

1. **Building:** Run `make build` to compile the operator
2. **Testing:** Run `make test` for unit tests
3. **Deploying:** Use `make deploy` with kustomize configs in `config/`
4. **Development:** Use `.devcontainer/` for consistent development environment
