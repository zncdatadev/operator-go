# operator-go/pkg/reconciler - GenericReconciler Framework

**Parent:** [../AGENTS.md](../AGENTS.md)
**Generated:** 2026-03-29

GenericReconciler framework for operator reconciliation logic and state management.

## Key Files

| File | Purpose |
|------|---------|
| `generic_reconciler.go` | Core reconciliation framework |
| `apply.go` | `copyDesiredState` — update semantics of the apply path (issue #526): labels replaced wholesale, annotations merged, per-kind spec/data copy that preserves Kubernetes immutable fields (StatefulSet selector/serviceName/volumeClaimTemplates/podManagementPolicy; Service clusterIP/allocated NodePorts), unstructured top-level copy for arbitrary-GVK extras |
| `reconciler.go` | Reconciler interface definitions |
| `status.go` | Status management utilities |
| `finalizer.go` | Finalizer handling |
| `discovery.go` | `EnsureDiscoveryConfigMap` — shared ensure-helper for product discovery ConfigMaps (CreateOrUpdate + controller owner ref + canonical labels; the product computes the data map) |

## Working Instructions

1. **Implementing a reconciler:** Extend GenericReconciler with custom logic
2. **Status updates:** Use status utilities for consistent status management
3. **Finalizers:** Use finalizer helpers for cleanup operations
4. **ServiceAccounts:** Prefer per-CR naming via `GenericReconcilerConfig.ServiceAccountNameFunc`
   (e.g. `"<product>-" + cr.GetName()`). The reconciler resolves the SA name per CR (func result >
   static `ServiceAccountName` > "" = skip), ensures the SA with the CR as controller owner
   (`ensureServiceAccount`), and propagates the resolved name through
   `RoleGroupBuildContext.ServiceAccountName` to the STS pod template. A static name shared by two
   clusters in one namespace permanently fails the second cluster's reconcile (AlreadyOwnedError,
   surfaced as a clear both-owners error) and GC-deletes the SA under the survivor when the owner
   cluster is deleted.
