# Internal framework controller

Parent instructions: [AGENTS.md](../../../AGENTS.md).
Authoritative design: [framework design, sections 5–7](../../../docs/architecture.md#framework-design).

This package executes the formal pipeline. It imports framework/input/pipeline and
Kubernetes/controller-runtime; production files import neither the discussion
prototype nor product adapters. Products use generated registration through
`pkg/framework/operator`, not this mutable internal Reconciler.

## Files and execution

- `reconciler.go` reads the generated object, gates operation before Project,
  combines Projection with separate registered Facts, prepares/builds each group,
  applies independent role/group/shared outputs, retires withdrawn groups and
  conditionally publishes status for the observed UID/generation.
- `dependencies.go` provides exact Get with per-pass GVK/namespace/name caching,
  caller copies and framework-owned UID/resourceVersion observations. Classified
  pending/invalid/readError results withhold only their group. Non-NotFound reader
  failures cannot be converted into a resolved value by a resolver ignoring errors.
- `apply.go` validates ownership, fixed source slots, reserved metadata and storage
  before writes. Server dry-run normalizes updates; unchanged desired state keeps
  resourceVersion. API allocations, status and undeclared external metadata survive.
- `shared_configmap.go` owns the complete shared ConfigMap set. Ready applies that
  set and retires absent trusted slots; Ready/empty withdraws all. Pending preserves
  previous output and its reason; invalid output or generation errors preserve old
  output and fail. No callback is Ready/empty. Live receipts survive controller
  restart/status loss. Custom same-owner ConfigMaps are neither adopted nor deleted.
- `role_pdb.go` applies enabled role budgets independently of group config/facts and
  retires disabled/removed role slots. Budgets use full declared replicas.
- `retirement.go` reconstructs fixed slots from live sources; scales to zero,
  requires current controller zero observations plus actual Pod absence, then
  deletes and confirms one slot per pass. Re-add waits for terminating objects.
  Fully validated shared ConfigMaps are excluded before group-name/label candidate
  heuristics, so shared output can retain user-supplied group-looking metadata.
- `storage.go` checks single RWO/Filesystem Retain data, StorageClass/PV reclaim,
  PVC source and binding receipts, all historical ordinals and actual consumers.
  It permits first-consumer creation while unbound, never deletes PVC/PV, and
  refuses conflicting surviving data rather than adopting or silently migrating it.
- `stop.go` independently stops trusted live workloads even when projection or
  config fails. It reobserves StatefulSet UID/RV and actual Pods; incomplete
  inventory cannot become a successful all-stopped observation.
- `operation_client.go` wraps each pass and rechecks current CR identity,
  generation, deletion and operation before every API request/retry. Checks and
  child writes are not an atomic multi-resource transaction.

`Client` must be a direct API client. The public registration implementation
constructs it; manager caching is used for scheduling watches. Setup watches the
CR plus owned ConfigMaps, Services, StatefulSets and role PDBs. Resolvers refresh
at 30 seconds by default, pending work at min(refresh, 2 seconds), including built-in references when no product resolver is registered. Per-key error backoff is bounded; this is a queue
scheduling bound, not a wall-clock guarantee during API stalls or queue backlog.

Pause reads Operation before full Project/facts/resource access. It updates only
Paused and the top observed generation, preserving execution condition generations
and group/role observations. Stable pause does not write or poll. Stop preserves
all declared replicas and PDB budgets while execution targets become zero; stopped
workload readiness remains Unknown rather than asserting application availability.

## Internal provenance

All formal receipts use `framework.kubedoop.dev/`; old prototype annotations are
not authority and receive no compatibility adoption:

| Suffix | Payload |
| --- | --- |
| `managed-metadata` | Object/template labels and annotations declared by the controller |
| `group-slot` | role, group, one of four fixed slots |
| `role-pdb` | role and fixed pdb slot |
| `shared-configmap` | version 1, crUID and exact ConfigMap name |
| `retained-data` | version 1, crUID, role/group/slot, class and canonical capacity |
| `retained-binding` | version 1, exact PVC UID, PV UID and volumeName |

Receipts alone are insufficient: current owner UID/name/GVK, namespace/name,
expected slot and managed identity metadata must agree. Source checks run again
inside conflict retries; deletes carry exact UID/resourceVersion preconditions.
The controller does not manufacture proof of a previously lost PVC or data bytes.

## Tests and proof boundaries

Fake-client tests exercise API-call ordering, identity conflicts, metadata/no-op
logic, partial failure isolation, facts refresh, operation races, retirement and
Retain safety. `fixture_test.go` provides a minimal test model and consumes U02's
formally generated test input. It does not import a production product adapter.

`TestControllerAPIConvergence` starts an isolated real API server and manager. It
checks shared output create/no-op/Pending/error/replacement/empty withdrawal,
pause/stop/latest-replica recovery, actual API Pod presence blocking retirement,
and persisted Retain VCT/PVC/PV source and binding identity across re-add.
Its zero-workload and Bound statuses are explicit fixture observations: envtest
runs no StatefulSet controller, provisioner, kubelet or product process. Mounts,
filesystem contents, actual shutdown and product health require deployment evidence.

Focused commands from repository root:

```sh
go test ./internal/framework/controller -count=1
go test ./internal/framework/controller -run '^TestControllerAPIConvergence$' -count=1 -v
./bin/golangci-lint run ./internal/framework/controller/...
```

API tests require KUBEBUILDER_ASSETS or the repository's Kubernetes 1.35 binaries;
missing assets fail the test. They never connect to an existing Kubernetes cluster.

`coordination.go` consumes WorkloadCoordination receipts. Normal scale-down, stop and retirement
share one-ordinal stepping with direct UID-authenticated Pod observations. Stop/retirement honor
ascending live shutdown priority. Native OrderedReady/RollingUpdate owns rolling replacement.
Versioned workload-progress annotations preserve progress deadlines across controller restart;
timeout reports an error without force-deleting or asserting graceful success.

## E05 independent data identity

Normal `preflightStorage` records a `dataops.DataAsset` after binding checks.
Shared checks used by stop/retirement do not create assets. Existing recorded
claims and the independent asset history are checked before reuse; a missing
original PVC, active operation lock or transferred data owner blocks implicit
fresh data creation. `dataops` types and CRDs are part of the formal persistent
storage installation. Product reconciliation does not execute DataOperations:
a separately deployed executor owns approved copy/rebind/erase and backend
reclamation. See [data-operation protocol](../../../docs/architecture.md#framework-data-operations).

## Platform observations

`platform_references.go` reads declared platform sources and Secret env references
from the final Pod, including optional references. `platform_observation.go` keeps
pre-apply readiness separate from post-apply current Pod/PVC/PV/Listener identity.
Native Secret UID/RV and class UID/generation produce a template digest; Listener
status refresh updates shared output without changing the template. Pending
post-creation results never withhold their producer. Groups expose a separate
PlatformObservation and the cluster exposes PlatformReady. Shared generation is
refreshed after observations and preserves the prior valid output while Pending.

Platform generic ephemeral volumes are admitted only from the typed runtime declaration.
`platform_storage.go` reconstructs exact sources, compares final Pod volumes, and stamps a
reserved StatefulSet `framework.kubedoop.dev/platform-claims` receipt bound to the CR UID and
role/group. Storage preflight, apply, stop, and retirement validate that source alongside existing
slot ownership. A matching platform StorageClass alone never admits an arbitrary Pod override
claim. These temporary platform volumes coexist with a separately declared retained data slot;
Kubernetes ephemeral ownership and the platform provisioner own their cleanup, not DataOperation.
Preparing references cannot publish Observing readiness, and zero active producers keep prior
shared outputs pending rather than fabricating an address.
