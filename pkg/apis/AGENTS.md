# operator-go/pkg/apis - CRD Definitions

**Parent:** [../AGENTS.md](../AGENTS.md)

API type definitions and Custom Resource Definitions (CRDs) for the operator framework.

## Key Directories

| Directory | Purpose |
|-----------|---------|
| `authentication/v1alpha1/` | `AuthenticationClass` and its providers (Kerberos, LDAP, OIDC, static, TLS) |
| `commons/v1alpha1/` | Shared spec/status building blocks every product CRD embeds |
| `database/v1alpha1/` | `DatabaseConnection` / `Database` types |
| `listeners/v1alpha1/` | `Listener`, `ListenerClass`, `PodListeners` |
| `s3/v1alpha1/` | `S3Connection` / `S3Bucket` types |

## commons/v1alpha1 — the types products embed

| File | Purpose |
|------|---------|
| `cluster_types.go` | `GenericClusterSpec` (`image`, `clusterOperation`, `roles`), `RoleSpec`, `RoleGroupSpec` |
| `cluster_status.go` | `GenericClusterStatus` and its condition helpers |
| `cluster_operation.go` | `ClusterOperationSpec` (`stopped`, `reconciliationPaused`) |
| `config_types.go` | `RoleConfigSpec` / `RoleGroupConfigSpec` (resources, affinity, logging, gracefulShutdownTimeout) |
| `overrides_types.go` | `OverridesSpec` — `configOverrides`, `envOverrides`, `cliOverrides`, `podOverrides` |
| `resource_types.go` | `ResourcesSpec`, CPU/memory/storage requests |
| `logging_types.go` | `LoggingSpec`, `LoggingConfigSpec`, per-container logger levels |
| `image_types.go` | `ImageSpec` and `GetImage(productName)` |
| `credentials.go` | `Credentials`, `CredentialsScope` (`node`, `pod`, `services`, `listenerVolumes`) |
| `pdb_types.go` | `PodDisruptionBudgetSpec` |
| `tls.go` | TLS spec building blocks |
| `graceful_shutdown.go`, `zk_config.go` | **Deprecated** — no CRD embeds `GracefulShutdownSpec`, and nothing consumes `ZKConfig`; retained only so downstream compilation does not break |

`resource_types.go` also carries a deprecated `StorageResourceSpec`; use `ResourcesSpec.Storage`.

## Status Conventions

- **`GenericClusterStatus.SetCondition` is not a wholesale replace.** An existing condition of the
  same type keeps its `LastTransitionTime` unless the `Status` actually flips, so a steady-state
  reconcile does not churn the status object (which would produce a watch event and an endless
  reconcile loop). A caller-supplied `LastTransitionTime` is therefore ignored when the `Status` is
  unchanged; `Reason`, `Message` and `ObservedGeneration` are always refreshed.
- **`SetObservedGeneration(int64)`** records the CR generation the status reflects. Conditions
  written afterwards inherit it when the caller leaves `ObservedGeneration` at zero, so call it
  *before* the `Set*` condition helpers.
- **Status conditions live under `.status.conditions`** (a `[]metav1.Condition` with
  `patchStrategy:"merge"` / `patchMergeKey:"type"`) in every group here, including
  `S3ConnectionStatus`, `S3BucketStatus` and `DatabaseConnectionStatus`.

## Serialization Conventions

- **Required scalars carry no `omitempty`**, so zero values are always emitted (`"host": ""`,
  `"className": ""`, `"credentials": null`). A request that omits them is rejected by the required
  check instead of silently passing.
- **Defaulted fields are pointers, not bare values with `omitempty`.**
  `ListenerSpec.PublishNotReadyAddresses` is a `*bool`, with
  `ListenerSpec.GetPublishNotReadyAddresses() bool` as the nil-safe accessor (nil ⇒ `false`).
  Emitting `false` explicitly would suppress CRD defaulting, so pointer + accessor is the pattern to
  follow for any new defaulted field. Fields that are nil-able by nature (map, slice, pointer) keep
  `omitempty` so they do not start emitting explicit nulls.

## Working Instructions

1. **Adding a new CRD:** create a new directory under `apis/` with a `v1alpha1/` (or `v1/`)
   subdirectory and a `groupversion_info.go`.
2. **Defining types:** use `+kubebuilder` markers for validation and generation. Replica bounds and
   resource-quantity formats are enforced by the CRD OpenAPI schema, not by webhook code.
3. **Generating code:** run `make generate` to refresh `zz_generated.deepcopy.go`.
4. **Changing a required field to defaulted:** make it a pointer and add a `Get*` accessor rather
   than removing `omitempty` from a value type.
