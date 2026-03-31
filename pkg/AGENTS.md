# operator-go/pkg - Package Overview

**Parent:** [../AGENTS.md](../AGENTS.md)
**Generated:** 2026-03-29

Core packages for the operator framework, including CRD definitions, resource builders, reconciliation logic, and configuration generation.

## Key Packages

| Package | Purpose |
|---------|---------|
| `apis/` | CRD definitions and API types |
| `builder/` | Kubernetes resource builders (StatefulSet, Service, ConfigMap, etc.) |
| `reconciler/` | GenericReconciler framework for operator logic |
| `config/` | Configuration generation and adaptation |
| `common/` | Shared utilities and constants |
| `webhook/` | Validation and defaulting webhooks |
| `util/` | General utilities (K8s, exec, etc.) |
| `listener/` | Listener service and volume builders |
| `sidecar/` | Sidecar injection logic |
| `security/` | Security-related utilities |
| `testutil/` | Testing helpers |

## Subdirectories

- `apis/` - API type definitions
- `builder/` - Resource builders
- `reconciler/` - Reconciliation framework
- `config/` - Config generation
- `common/` - Shared code
- `webhook/` - Webhooks
- `util/` - Utilities
- `listener/` - Listener builders
- `sidecar/` - Sidecar logic
- `security/` - Security utilities
- `testutil/` - Test helpers

## Working Instructions

1. **Adding a new resource builder:** Create in `builder/` following the pattern of existing builders (e.g., `statefulset_builder.go`)
2. **Adding reconciliation logic:** Extend `reconciler/` with new reconcilers
3. **Adding config generation:** Add adapters in `config/`
4. **Adding utilities:** Place in `common/` or `util/` as appropriate
