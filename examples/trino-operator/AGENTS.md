# operator-go/examples/trino-operator - Trino Operator Example

**Parent:** [../../AGENTS.md](../../AGENTS.md)
**Generated:** 2026-03-29

Complete Trino operator implementation demonstrating the operator-go framework with CRD definitions, reconciliation logic, and resource builders.

## Key Directories

| Directory | Purpose |
|-----------|---------|
| `api/` | Trino CRD definitions |
| `cmd/` | Operator entrypoint |
| `config/` | Kubernetes manifests and kustomize configs |
| `internal/` | Reconciler and business logic |

## Working Instructions

1. **Building:** Run `make build` to compile the operator
2. **Testing:** Run `make test` for unit tests
3. **Deploying:** Use `make deploy` with kustomize configs in `config/`
4. **Development:** Use `.devcontainer/` for consistent development environment
