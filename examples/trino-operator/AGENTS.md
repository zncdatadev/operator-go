# operator-go/examples/trino-operator - Trino Operator Example

**Parent:** [../../AGENTS.md](../../AGENTS.md)
**Generated:** 2026-03-29

Complete Trino operator implementation demonstrating the operator-go framework with CRD definitions, reconciliation logic, and resource builders.

## Key Directories

| Directory | Purpose |
|-----------|---------|
| `api/` | Trino CRD definitions |
| `cmd/` | Operator entrypoint (wires the handler + `ProductConfig` into `GenericReconciler`) |
| `config/` | Kubernetes manifests and kustomize configs |
| `internal/controller/` | `TrinoRoleGroupHandler` — embeds the SDK `BaseRoleGroupHandler`; the framework owns resource orchestration |
| `internal/product/` | `ProductConfig` hook (`product.ComputeConfig`): Trino's `config.properties` (role-branched, derived) computed and returned as data |
| `internal/config/` | `JVMConfigBuilder` (non-key-value `jvm.config`) and `CatalogConfigBuilder` |
| `internal/extensions/` | Catalog validation + health (ClusterExtension / RoleExtension) |

## Architecture (ProductConfig pattern)

This example demonstrates the SDK's preferred division of labour:

- **Framework owns the 90%.** `TrinoRoleGroupHandler` embeds `reconciler.BaseRoleGroupHandler`, so the ConfigMap, Services, StatefulSet (with sidecars + `podOverrides` applied), and PDB are built by the SDK. The handler sets `ConfigMountPath` (`/etc/trino`), `MainContainerName` (`trino`), `LoggingContainers` (declarative Log4j2), and per-role ports.
- **Product config flows as data through the merge pipeline.** `product.ComputeConfig` computes `config.properties` and returns it as an `*OverridesSpec`, wired via `GenericReconcilerConfig.ProductConfig` — the lowest merge layer, so any user `configOverrides` in the CRD always win. This is config generation (recomputed every reconcile), not webhook defaulting.
- **Escape hatch for what the pipeline can't model.** `BuildResources` calls the base, then appends the CR-driven image, the non-key-value `jvm.config`, and coordinator-only catalog files. There is no hand-built `StatefulSet`.

## Working Instructions

1. **Building:** Run `make build` to compile the operator
2. **Testing:** Run `make test` for unit tests
3. **Deploying:** Use `make deploy` with kustomize configs in `config/`
4. **Development:** Use `.devcontainer/` for consistent development environment
