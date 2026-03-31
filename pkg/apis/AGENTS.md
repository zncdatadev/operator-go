# operator-go/pkg/apis - CRD Definitions

**Parent:** [../AGENTS.md](../AGENTS.md)
**Generated:** 2026-03-29

API type definitions and Custom Resource Definitions (CRDs) for the operator framework.

## Key Directories

| Directory | Purpose |
|-----------|---------|
| `authentication/` | Authentication-related CRDs |
| `commons/` | Common API types |
| `database/` | Database-related CRDs |
| `listeners/` | Listener-related CRDs |
| `s3/` | S3-related CRDs |

## Working Instructions

1. **Adding a new CRD:** Create a new directory under `apis/` with `v1alpha1/` or `v1/` subdirectory
2. **Defining types:** Use `+kubebuilder` markers for validation and generation
3. **Generating code:** Run `make generate` to create deepcopy and other generated files
