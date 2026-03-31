# operator-go/pkg/reconciler - GenericReconciler Framework

**Parent:** [../AGENTS.md](../AGENTS.md)
**Generated:** 2026-03-29

GenericReconciler framework for operator reconciliation logic and state management.

## Key Files

| File | Purpose |
|------|---------|
| `generic_reconciler.go` | Core reconciliation framework |
| `reconciler.go` | Reconciler interface definitions |
| `status.go` | Status management utilities |
| `finalizer.go` | Finalizer handling |

## Working Instructions

1. **Implementing a reconciler:** Extend GenericReconciler with custom logic
2. **Status updates:** Use status utilities for consistent status management
3. **Finalizers:** Use finalizer helpers for cleanup operations
