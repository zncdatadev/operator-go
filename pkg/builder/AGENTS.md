# operator-go/pkg/builder - Resource Builders

**Parent:** [../AGENTS.md](../AGENTS.md)
**Generated:** 2026-03-29

Kubernetes resource builders for StatefulSet, Service, ConfigMap, PDB, RBAC, and other resources.

## Key Files

| File | Purpose |
|------|---------|
| `statefulset_builder.go` | StatefulSet resource builder |
| `service_builder.go` | Service resource builder |
| `configmap_builder.go` | ConfigMap resource builder |
| `pdb_builder.go` | PodDisruptionBudget builder |
| `rbac_builder.go` | RBAC (Role, RoleBinding) builder |
| `serviceaccount_builder.go` | ServiceAccount builder |

## Working Instructions

1. **Creating a new builder:** Follow the pattern of existing builders with `Build()` and `BuildWithContext()` methods
2. **Testing builders:** Add corresponding `*_test.go` files with unit tests
3. **Builder pattern:** Each builder should support method chaining for configuration
