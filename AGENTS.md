# AGENTS.md

## Project Overview
`operator-go` is a Golang SDK/framework for building Kubernetes operators. It provides a reusable reconciliation framework, CRDs, and utilities for creating product-specific operators.

**Key Features:**
- **GenericReconciler**: Template Method Pattern-based reconciliation framework
- **Extension System**: Hook-based customization at cluster/role/role-group levels
- **Resource Builders**: Fluent builders for StatefulSet, Service, ConfigMap, PDB, RBAC, ServiceAccount
- **Config Generation**: Multi-format config file generation (XML, YAML, Properties, Env, INI)
- **Logging Config**: Framework-aware logging configuration generation (Log4j, Log4j2, Logback, Python)
- **Health Checks**: Business-level health check interface with composite checks
- **Sidecar Management**: Pluggable sidecar injection framework with domain-specific providers
- **CRD APIs**: Common types for authentication, database, listeners, S3

## Architecture Documentation (Authoritative Design Source)

> **IMPORTANT**: The `docs/` directory contains architecture documents that are the **authoritative source of design constraints** for this project. All implementations — including the SDK itself and any operators built with it — **must follow** the design defined in these documents. When code and documentation conflict, the documentation takes precedence. Consult these docs before making design decisions.

### Documentation Structure

| File | Description |
|------|-------------|
| `docs/architecture.md` | **Core Technical Architecture** — design philosophy, layered architecture, core module specifications, design patterns, key problem solutions. This is the primary reference for all SDK design decisions. |
| `docs/architecture_zh.md` | Chinese version of the architecture document |
| `docs/security.md` | **Security Architecture** — application security (SecretClass, CSI, AutoTLS, Kerberos) and infrastructure security (RBAC, ServiceAccounts, Pod security) |
| `docs/DOC_CHANGELOG.md` | Changelog tracking all documentation updates |
| `docs/examples/` | CRD example YAMLs demonstrating the SDK's data model |

### CRD Examples (`docs/examples/`)

| File | Description |
|------|-------------|
| `crd-base-example.yaml` | Base CRD template showing the generic structure all product CRDs follow |
| `crd-hdfs-example.yaml` | HDFS cluster CRD example (HA with NameNode, JournalNode, DataNode) |
| `crd-hive-example.yaml` | Hive Metastore CRD example (S3 integration, TLS, Kerberos) |

### Key Architectural Principles (from `docs/architecture.md`)

1. **Interface-Driven Design (IDD)**: SDK core relies on interfaces, not concrete implementations. New products implement interfaces without modifying SDK core.
2. **Desired State Convergence**: CR Spec is the desired state; reconciliation loop converges actual state. Bidirectional: also cleans orphaned resources.
3. **Separation of Common and Specific**: SDK handles common logic (resource construction, config merging, webhook validation); products handle specific logic via extension interfaces.
4. **Type Safety and Idempotency**: Go Generics for compile-time safety. All operations are idempotent.
5. **Strict Merge Strategy**: Role/RoleGroup config merging follows defined rules — Deep Merge for maps, Replace/Append for slices, Strategic Merge Patch for PodTemplate.
6. **Layered Architecture**: Specific Product Layer → Abstract Interface Layer → Core Component Layer → Tools Layer → API Layer.

## Development Environment
- **Language**: Go 1.25.3
- **Dependency Management**: Go Modules (`go.mod`)
- **Testing**: Ginkgo v2 + Gomega
- **Tooling**: Uses `Makefile` to manage local binaries in `bin/`

### Tool Versions
- `controller-gen`: v0.19.0
- `golangci-lint`: v2.10.1
- `kustomize`: v5.7.1
- `controller-runtime`: v0.23.3
- `k8s.io/api`: v0.35.3

## Common Commands
Run these from the project root:

| Command | Description |
|---------|-------------|
| `make generate` | Generate DeepCopy methods via `controller-gen` |
| `make fmt` | Run `go fmt` against code |
| `make vet` | Run `go vet` against code |
| `make test` | Run unit tests with coverage (uses envtest for K8s integration) |
| `make lint` | Run `golangci-lint` |
| `make lint-fix` | Run `golangci-lint` with auto-fix |
| `make lint-config` | Verify golangci-lint configuration |

## Directory Structure
> Subdirectories with their own `AGENTS.md` provide detailed file-level documentation. This section shows the top-level layout only.

```
operator-go/
├── pkg/                          # Core SDK packages (see pkg/AGENTS.md)
│   ├── apis/                     # Kubernetes API definitions — CRDs (see pkg/apis/AGENTS.md)
│   ├── builder/                  # Fluent resource builders (see pkg/builder/AGENTS.md)
│   ├── common/                   # Core interfaces, extensions, errors
│   ├── config/                   # Config generation, merging, logging (see pkg/config/AGENTS.md)
│   ├── listener/                 # Listener provisioner (CSI volume and service registration)
│   ├── reconciler/               # Reconciliation framework (see pkg/reconciler/AGENTS.md)
│   ├── security/                 # Pod security, secret class handling
│   ├── sidecar/                  # Sidecar injection framework (SidecarManager, SidecarProvider interface)
│   ├── vector/                   # Vector sidecar implementation (config generation, discovery, provider)
│   ├── testutil/                 # Testing utilities (envtest, mocks, matchers)
│   ├── util/                     # K8s utilities, exec utilities
│   └── webhook/                  # Webhook infrastructure (defaulter, validator)
├── docs/                         # Architecture and design documentation (authoritative design source)
│   ├── architecture.md           # Core Technical Architecture (English)
│   ├── architecture_zh.md        # Core Technical Architecture (Chinese)
│   ├── security.md               # Security Architecture (SecretClass, CSI, RBAC, Pod security)
│   ├── DOC_CHANGELOG.md          # Documentation changelog
│   └── examples/                 # CRD example YAMLs (base, HDFS, Hive)
├── examples/                     # Example operators (see examples/AGENTS.md)
│   └── trino-operator/           # Trino operator example (see examples/trino-operator/AGENTS.md)
├── hack/                         # Scripts and boilerplate
└── bin/                          # Local binaries (controller-gen, etc.)
```

## Key Concepts

### 1. ClusterInterface
All product CRs must implement `ClusterInterface` (defined in `pkg/common/cluster_interface.go`):
```go
type ClusterInterface interface {
    GetName() string
    GetNamespace() string
    GetUID() types.UID
    GetLabels() map[string]string
    GetAnnotations() map[string]string
    GetSpec() *v1alpha1.GenericClusterSpec
    GetStatus() *v1alpha1.GenericClusterStatus
    SetStatus(status *v1alpha1.GenericClusterStatus)
    GetObjectMeta() *metav1.ObjectMeta
    GetScheme() *runtime.Scheme
    DeepCopyCluster() ClusterInterface
    GetRuntimeObject() runtime.Object
}
```

`ClusterObject` is a helper struct that can be embedded in product CRs to provide default implementations.

### 2. GenericReconciler (Template Method Pattern)
The `GenericReconciler` provides a fixed reconciliation flow with customizable extension points:

**Reconciliation Flow:**
1. Fetch CR
2. Panic recovery
3. PreReconcile Extensions (Hook)
4. Validate Dependencies
5. For Each Role:
   - Role PreReconcile Extensions
   - For Each RoleGroup:
     - RoleGroup PreReconcile Extensions
     - Build RoleGroupBuildContext
     - Delegate to RoleGroupHandler.BuildResources()
     - Apply Resources (CM -> HeadlessSvc -> Service -> Extras -> STS -> PDB -> MetricsSvc)
     - RoleGroup PostReconcile Extensions
   - Role PostReconcile Extensions
6. Cleanup Orphaned Resources
7. Update Health Status
8. PostReconcile Extensions
9. Final Status Update

Each "Apply" is create-OR-UPDATE (issue #526): when the resource already exists, the live object is updated to the handler-built desired state every reconcile — labels are replaced wholesale, annotations are merged (foreign annotations survive), and spec/data is copied per kind while preserving Kubernetes immutable/allocated fields (StatefulSet `selector`/`serviceName`/`volumeClaimTemplates`/`podManagementPolicy`; Service `clusterIP(s)`/`ipFamilies` and allocated NodePorts). Arbitrary-GVK extras get a generic top-level field copy. See `copyDesiredState` in `pkg/reconciler/apply.go`. Changing an immutable field for an existing cluster requires a manual delete/recreate migration.

### 3. RoleGroupHandler and BaseRoleGroupHandler
Product operators implement `RoleGroupHandler` to define resource building logic:
```go
type RoleGroupHandler[CR common.ClusterInterface] interface {
    BuildResources(ctx context.Context, k8sClient client.Client, cr CR, buildCtx *RoleGroupBuildContext) (*RoleGroupResources, error)
}
```

`BaseRoleGroupHandler` provides a default implementation that creates ConfigMap, Headless Service, Service, StatefulSet, and PDB. Product operators can embed it and override specific methods:
```go
handler := reconciler.NewBaseRoleGroupHandler[*v1alpha1.TrinoCluster](image, scheme)
handler.SetRoleContainerPorts("coordinator", ports)
handler.SetRoleServicePorts("coordinator", svcPorts)
```

When building the StatefulSet, `BaseRoleGroupHandler` consumes the role group's `config` (commons `RoleGroupConfigSpec`): `resources` (requests/limits, plus an opt-in data PVC via `StorageMountPath`), `affinity` (a RawExtension unmarshaled into `corev1.Affinity` and set on the pod spec — invalid JSON fails the build), and `gracefulShutdownTimeout` (a Go duration mapped to `terminationGracePeriodSeconds` — unparsable or non-positive values fail the build). All of these are applied before `podOverrides`, so user pod overrides keep precedence. The framework sets affinity only when the config provides one, so products that post-process the built StatefulSet with `if podSpec.Affinity == nil {...}` default guards remain correct.

`RoleGroupHandlerFuncs` is a function adapter for simple handlers that don't need a full struct.

Besides the fixed fields (ConfigMap, Services, StatefulSet, PDB, MetricsService), `RoleGroupResources.ExtraResources []client.Object` lets products ship arbitrary per-role-group resources (e.g. a `listeners.kubedoop.dev` Listener CR) through the framework's apply path: same controller owner reference, applied BEFORE the StatefulSet because extras are typically pod-scheduling prerequisites. The cleaner does not discover arbitrary-GVK extras — extras of removed role groups are reclaimed only via owner-reference GC when the CR is deleted (see the field's doc comment).

### 4. RoleInterface and RoleGroupInfo
`RoleInterface` defines role-level operations for interacting with role configurations:
```go
type RoleInterface interface {
    GetRoleName() string
    GetRoleSpec() *v1alpha1.RoleSpec
    GetRoleGroups() map[string]v1alpha1.RoleGroupSpec
    GetOverrides() *v1alpha1.OverridesSpec
}
```

`RoleInfo` provides a default implementation. `RoleGroupInfo` contains role group details including replicas, resources, logging, and graceful shutdown config.

### 5. Extension System
Three levels of extensions for injecting custom logic:
- **ClusterExtension**: PreReconcile, PostReconcile, OnReconcileError
- **RoleExtension**: Per-role hooks
- **RoleGroupExtension**: Per-role-group hooks

Extensions have priorities (Lowest=0, Low=25, Normal=50, High=75, Highest=100).

### 6. Resource Builders
Fluent builders for K8s resources:
```go
sts := builder.NewStatefulSetBuilder(name, namespace).
    WithLabels(labels).
    WithReplicas(3).
    WithImage("trino:latest", corev1.PullIfNotPresent).
    WithResources(spec.Resources).
    AddPort("http", 8080, corev1.ProtocolTCP).
    Build()
```

Additional builders: `RoleBuilder`, `RoleBindingBuilder`, `ServiceAccountBuilder`.

### 7. Config Generation
Multi-format config generation with `ConfigFormat` interface:
```go
generator := config.NewMultiFormatConfigGenerator()
generator.RegisterDefaultFormats() // .xml, .properties, .yaml, .yml, .env
generator.RegisterFormat(".ini", config.NewINIAdapter()) // INI requires explicit registration
files, err := generator.GenerateFiles(map[string]map[string]string{
    "config.properties": {"key": "value"},
    "config.yaml":       {"nested": "data"},
})
```

`ConfigFormat` interface supports custom format adapters via `Marshal`/`Unmarshal`. Supported formats: XML, Properties, YAML, Env, INI.

### 8. Health Checks
`ServiceHealthCheck` interface for business-level health verification (beyond Pod readiness):
```go
type ServiceHealthCheck interface {
    CheckHealthy(ctx context.Context, client client.Client, namespace, name string) (bool, error)
}
```

`CompositeHealthCheck` combines multiple checks (all must pass). `AlwaysHealthy` and `AlwaysUnhealthy` are provided as convenience constants.

### 9. Logging Configuration
`LoggingFramework`-aware logging config generation (Log4j, Log4j2, Logback, Python) via `pkg/config/logging_generator.go`.

### 10. Product Config (`ProductConfig`)
Products contribute their computed configuration **as data through the same merge pipeline as CRD overrides**, instead of imperatively constructing resources. Set the optional `ProductConfig` field on `GenericReconcilerConfig` — a pure function returning an `*v1alpha1.OverridesSpec` (the same shape users write in the CRD):

```go
reconcilerCfg := &reconciler.GenericReconcilerConfig[*v1alpha1.TrinoCluster]{
    // ...
    ProductConfig: func(cr *v1alpha1.TrinoCluster, roleName, roleGroupName string) *commonsv1alpha1.OverridesSpec {
        overrides := map[string]map[string]string{
            "config.properties": {"http-server.http.port": "8080"},
        }
        if roleName == "coordinators" {
            overrides["config.properties"]["coordinator"] = "true"
        }
        return &commonsv1alpha1.OverridesSpec{ConfigOverrides: overrides}
    },
}
```

Precedence (low → high): **product config < role overrides < role group overrides**. Any value a user sets in the CRD always wins. `ConfigMerger.Merge` is variadic (`Merge(...*OverridesSpec)`) and folds layers in order; the previous two-argument call (`Merge(role, group)`) is still valid.

**This is config generation, not defaulting** — do not confuse it with the webhook `ProductDefaulter`:

| | `ProductDefaulter` (webhook) | `ProductConfig` (this) |
|---|---|---|
| Targets | typed **spec fields** (image, ports, replicas) | **config-file content** (config.properties, etc.) |
| When | admission, **persisted into spec** | every reconcile, **not persisted** |
| Upgrade propagation | no (frozen at admission) | **yes** (recomputed with current operator) |
| Derived-from-live-state | freezes/stales | **recomputed each reconcile** |

Use `ProductConfig` for product-intrinsic and derived config (e.g. a ZooKeeper connection string built from the actual resources, a JVM heap sized from resources, role-specific keys) so the product no longer hand-builds ConfigMaps/StatefulSets. Use `ProductDefaulter` for stable, user-facing typed spec defaults.

### 11. Discovery ConfigMaps
Every product operator publishes "discovery ConfigMaps" — cluster-level ConfigMaps (namespaced, in the CR's namespace; usually named `<cluster>`, optionally suffixed like `<cluster>-nodeport`) carrying client connection info. `reconciler.EnsureDiscoveryConfigMap` (in `pkg/reconciler/discovery.go`) owns the ensure semantics; the product owns computing the data map (address aggregation differs per product) and typically calls it from a `ClusterExtension.PostReconcile`:

```go
err := reconciler.EnsureDiscoveryConfigMap(ctx, client, scheme, cr, cr.GetName(),
    map[string]string{"KAFKA": bootstrapServers},
    reconciler.WithDiscoveryProductName("kafka"),
    reconciler.WithDiscoveryExtraLabels(map[string]string{
        reconciler.ClusterLabelKey("kafka.kubedoop.dev"): cr.GetName(),
    }),
)
```

The helper is idempotent (CreateOrUpdate), sets a controller owner reference (the ConfigMap is GC'd with the CR), and applies canonical labels (`app.kubernetes.io/instance`, `app.kubernetes.io/managed-by`, plus `app.kubernetes.io/name` via `WithDiscoveryProductName`); extra labels/annotations are merged via options, but canonical labels always win. Data is replaced wholesale.

## Building a New Operator

1. **Define CRD** - Create API types implementing `ClusterInterface` (embed `ClusterObject` for defaults)
2. **Create RoleGroupHandler** - Embed `BaseRoleGroupHandler` for default resource building, or implement `RoleGroupHandler` directly
3. **Provide Product Config** (optional) - Set `ProductConfig` on `GenericReconcilerConfig` to contribute product-intrinsic/derived config as the lowest merge layer
4. **Register Extensions** (optional) - Add custom hooks via extension registry
5. **Setup Webhooks** (optional) - Use common defaults/validators from `pkg/webhook/`
6. **Register Health Checks** (optional) - Implement `ServiceHealthCheck` for business-level health verification
7. **Create main.go** - Use `GenericReconciler` with your handler

See `examples/trino-operator/` for a complete example.

## Development Rules

All AI agents and developers working on this project **must** follow these rules:

### Before Committing Code
1. Run `make generate` if you modified any API structs in `pkg/apis/`
2. Run `make lint` — must pass with zero errors
3. Run `make test` — all tests must pass
4. **Never commit if lint or tests fail**

### After Code Changes
- Always run `make test` to verify nothing is broken
- Always run `make lint` to ensure code quality
- If adding new public interfaces, update AGENTS.md accordingly

### Code Style & Conventions
- **Formatting**: Must pass `go fmt`
- **Linting**: Must pass `golangci-lint`
- **CRDs**: Uses `kubebuilder` markers (tags) for code generation
- **Generation**: When modifying API structs in `pkg/apis`, always run `make generate`
- **Testing**: Use Ginkgo v2 + Gomega; test files use `suite_test.go` pattern
- **Error Handling**: Use error types from `pkg/common/errors.go` and `pkg/reconciler/errors.go`
- **Generics**: Extensive use of generics for type-safe operator framework — no type assertions

### Design Constraints
- Follow the layered architecture defined in `docs/architecture.md`
- Use Go Generics for type safety — no type assertions
- All operations must be idempotent
- Config merging follows the strict merge strategy (Deep Merge for maps, Replace/Append for slices, Strategic Merge Patch for PodTemplate)
- Extensions must be registered during Operator initialization (in `main.go` before Manager starts)
- Override fields (`configOverrides`, `envOverrides`, `cliOverrides`, `podOverrides`) are **flattened** directly at Role/RoleGroup level, NOT nested under an `overrides` field

## Testing
- Unit tests use Ginkgo v2 with Gomega matchers
- Each package has a `suite_test.go` for test setup
- `pkg/testutil/` provides envtest helpers and mocks
- Run tests: `make test`
- All tests must pass before any code is committed
