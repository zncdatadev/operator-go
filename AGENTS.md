# AGENTS.md

## Project Overview
`operator-go` is a Golang SDK/framework for building Kubernetes operators. It provides a reusable reconciliation framework, CRDs, and utilities for creating product-specific operators.

**Key Features:**
- **GenericReconciler**: Template Method Pattern-based reconciliation framework
- **Extension System**: Hook-based customization at cluster/role/role-group levels
- **Resource Builders**: Fluent builders for StatefulSet, Service, ConfigMap, PDB, RBAC, ServiceAccount
- **Config Generation**: Multi-format config file generation (XML, YAML, Properties, Env, INI)
- **Logging Config**: Framework-aware logging configuration generation (Log4j2, Logback, Python)
- **Health Checks**: Business-level health check interface with composite checks
- **Sidecar Management**: Pluggable sidecar injection (Vector, JMX Exporter)
- **CRD APIs**: Common types for authentication, database, listeners, S3

## Architecture

### Core Packages

| Package | Description |
|---------|-------------|
| `pkg/apis/` | Kubernetes API definitions (CRDs) |
| `pkg/builder/` | Fluent builders for K8s resources (StatefulSet, Service, ConfigMap, PDB, RBAC, ServiceAccount) |
| `pkg/common/` | Core interfaces (`ClusterInterface`, `RoleInterface`, `ServiceHealthCheck`), extension registry, errors |
| `pkg/config/` | Config generation (XML, YAML, Properties, Env, INI formats), merging, logging config |
| `pkg/listener/` | Listener-related volume and service builders |
| `pkg/reconciler/` | `GenericReconciler`, `BaseRoleGroupHandler`, health checks, dependency resolution, cleanup |
| `pkg/security/` | Pod security context, secret class handling |
| `pkg/sidecar/` | Sidecar manager and providers (Vector, JMX Exporter) |
| `pkg/testutil/` | Testing utilities (envtest, mocks, matchers) |
| `pkg/util/` | K8s utilities (CreateOrUpdate, status updates) |
| `pkg/webhook/` | Webhook infrastructure (defaulter, validator, common defaults/validators) |

### API Packages (`pkg/apis/`)

| Package | Description |
|---------|-------------|
| `commons/v1alpha1` | Core types: GenericClusterSpec, RoleSpec, RoleGroupSpec, resources, TLS, credentials, logging, graceful shutdown, overrides, cluster operation |
| `authentication/v1alpha1` | Authentication CRDs |
| `database/v1alpha1` | Database connection CRDs |
| `listeners/v1alpha1` | Listener, ListenerClass, PodListener CRDs |
| `s3/v1alpha1` | S3 connection CRDs |

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
```
operator-go/
├── cmd/                          # (Not present - operators provide their own main.go)
├── pkg/
│   ├── apis/                     # Kubernetes API definitions (CRDs)
│   │   ├── authentication/       # Authentication CRDs
│   │   ├── commons/              # Core types (cluster, role, resources, etc.)
│   │   │   ├── cluster_types.go      # GenericClusterSpec, GenericClusterStatus
│   │   │   ├── cluster_operation.go  # Cluster operation types
│   │   │   ├── cluster_status.go     # Cluster status conditions
│   │   │   ├── config_types.go       # RoleGroupConfigSpec
│   │   │   ├── credentials.go        # Credentials types
│   │   │   ├── graceful_shutdown.go  # Graceful shutdown configuration
│   │   │   ├── image_types.go        # Image spec
│   │   │   ├── logging_types.go      # Logging configuration
│   │   │   ├── overrides_types.go    # Overrides spec
│   │   │   ├── pdb_types.go          # PodDisruptionBudget spec
│   │   │   ├── resource_types.go     # Resource requirements
│   │   │   └── tls.go               # TLS configuration
│   │   ├── database/             # Database connection CRDs
│   │   ├── listeners/            # Listener configurations
│   │   └── s3/                   # S3 connection types
│   ├── builder/                  # Resource builders
│   │   ├── configmap_builder.go  # ConfigMap fluent builder
│   │   ├── pdb_builder.go        # PodDisruptionBudget builder
│   │   ├── rbac_builder.go       # Role and RoleBinding builders
│   │   ├── service_builder.go    # Service builder
│   │   ├── serviceaccount_builder.go # ServiceAccount builder
│   │   └── statefulset_builder.go # StatefulSet builder
│   ├── common/                   # Core interfaces and utilities
│   │   ├── cluster_interface.go  # ClusterInterface, ClusterObject helper
│   │   ├── role_interface.go     # RoleInterface, RoleInfo, RoleGroupInfo
│   │   ├── health_interface.go   # ServiceHealthCheck, CompositeHealthCheck
│   │   ├── extension.go          # Extension interfaces (Cluster/Role/RoleGroup)
│   │   ├── extension_registry.go # Extension registry
│   │   └── errors.go             # Common error types
│   ├── config/                   # Configuration generation
│   │   ├── format.go             # ConfigFormat interface, format type registry
│   │   ├── generator.go          # ConfigGenerator, MultiFormatConfigGenerator
│   │   ├── logging_generator.go  # Logging config generator (Log4j2, Logback, Python)
│   │   ├── merger.go             # ConfigMerger for role/role-group overrides
│   │   ├── xml_adapter.go        # XML format adapter
│   │   ├── yaml_adapter.go       # YAML format adapter
│   │   ├── properties_adapter.go # Properties format adapter
│   │   ├── env_adapter.go        # Environment variables adapter
│   │   └── ini_adapter.go        # INI format adapter
│   ├── listener/                 # Listener utilities
│   │   ├── service_builder.go    # Listener service builder
│   │   └── volume_builder.go     # Listener volume builder
│   ├── reconciler/               # Reconciliation framework
│   │   ├── generic_reconciler.go      # GenericReconciler (main entry point)
│   │   ├── role_group_handler.go      # RoleGroupHandler interface, RoleGroupHandlerFuncs
│   │   ├── base_role_group_handler.go # BaseRoleGroupHandler (default implementation)
│   │   ├── health.go                  # Health check manager
│   │   ├── dependency.go              # Dependency resolver
│   │   ├── cleaner.go                 # Orphaned resource cleaner
│   │   ├── event.go                   # Event manager
│   │   └── errors.go                  # Reconcile errors
│   ├── security/                 # Security utilities
│   │   ├── pod_security.go       # Pod security context
│   │   └── secret_class.go       # Secret class handling
│   ├── sidecar/                  # Sidecar injection
│   │   ├── manager.go            # SidecarManager
│   │   ├── interface.go          # SidecarProvider interface
│   │   ├── vector.go             # Vector sidecar provider
│   │   └── jmx_exporter.go       # JMX Exporter sidecar provider
│   ├── testutil/                 # Testing utilities
│   │   ├── envtest.go            # Envtest setup helpers
│   │   ├── mocks.go              # Mock implementations
│   │   ├── matchers.go           # Gomega matchers
│   │   └── builder.go            # Builder test utilities
│   ├── util/                     # General utilities
│   │   ├── k8s_util.go           # K8s utilities (CreateOrUpdate, etc.)
│   │   └── exec_util.go          # Execution utilities
│   └── webhook/                  # Webhook infrastructure
│       ├── webhook.go            # Webhook setup
│       ├── defaulter.go          # Default value setter
│       ├── validator.go          # Validation logic
│       ├── common_defaults.go    # Common defaulting helpers
│       ├── common_validators.go  # Common validation helpers
│       └── errors.go             # Webhook errors
├── examples/                     # Example operators
│   └── trino-operator/           # Trino operator example
│       ├── api/v1alpha1/         # TrinoCluster CRD
│       ├── cmd/main.go           # Operator entry point
│       ├── internal/             # Controller, handlers, extensions, webhooks
│       │   ├── config/           # Trino-specific config generation
│       │   ├── constants/        # Product constants
│       │   ├── controller/       # Trino controller
│       │   ├── extensions/       # Catalog and health extensions
│       │   ├── handlers/         # Coordinator and worker handlers
│       │   └── webhook/          # TrinoCluster webhook
│       └── test/e2e/             # End-to-end tests
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
     - Apply Resources (CM -> HeadlessSvc -> Service -> STS -> PDB)
     - RoleGroup PostReconcile Extensions
   - Role PostReconcile Extensions
6. Cleanup Orphaned Resources
7. Update Health Status
8. PostReconcile Extensions
9. Final Status Update

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

`RoleGroupHandlerFuncs` is a function adapter for simple handlers that don't need a full struct.

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
generator.RegisterDefaultFormats() // .xml, .properties, .yaml, .yml, .env, .ini
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
`LoggingFramework`-aware logging config generation (Log4j2, Logback, Python) via `pkg/config/logging_generator.go`.

## Building a New Operator

1. **Define CRD** - Create API types implementing `ClusterInterface` (embed `ClusterObject` for defaults)
2. **Create RoleGroupHandler** - Embed `BaseRoleGroupHandler` for default resource building, or implement `RoleGroupHandler` directly
3. **Register Extensions** (optional) - Add custom hooks via extension registry
4. **Setup Webhooks** (optional) - Use common defaults/validators from `pkg/webhook/`
5. **Register Health Checks** (optional) - Implement `ServiceHealthCheck` for business-level health verification
6. **Create main.go** - Use `GenericReconciler` with your handler

See `examples/trino-operator/` for a complete example.

## Code Style & Conventions
- **Formatting**: Must pass `go fmt`
- **Linting**: Must pass `golangci-lint`
- **CRDs**: Uses `kubebuilder` markers (tags) for code generation
- **Generation**: When modifying API structs in `pkg/apis`, always run `make generate`
- **Testing**: Use Ginkgo v2 + Gomega; test files use `suite_test.go` pattern
- **Error Handling**: Use error types from `pkg/common/errors.go` and `pkg/reconciler/errors.go`
- **Generics**: Extensive use of generics for type-safe operator framework

## Testing
- Unit tests use Ginkgo v2 with Gomega matchers
- Each package has a `suite_test.go` for test setup
- `pkg/testutil/` provides envtest helpers and mocks
- Run tests: `make test`
