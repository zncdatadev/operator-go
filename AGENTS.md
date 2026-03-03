# AGENTS.md

## Project Overview
`operator-go` is a Golang SDK/framework for building Kubernetes operators. It provides a reusable reconciliation framework, CRDs, and utilities for creating product-specific operators.

**Key Features:**
- **GenericReconciler**: Template Method Pattern-based reconciliation framework
- **Extension System**: Hook-based customization at cluster/role/role-group levels
- **Resource Builders**: Fluent builders for StatefulSet, Service, ConfigMap, PDB
- **Config Generation**: Multi-format config file generation (XML, YAML, Properties, Env)
- **Sidecar Management**: Pluggable sidecar injection (Vector, JMX Exporter)
- **CRD APIs**: Common types for authentication, database, listeners, S3

## Architecture

### Core Packages

| Package | Description |
|---------|-------------|
| `pkg/apis/` | Kubernetes API definitions (CRDs) |
| `pkg/builder/` | Fluent builders for K8s resources (StatefulSet, Service, ConfigMap, PDB) |
| `pkg/common/` | Core interfaces (`ClusterInterface`), extension registry, errors |
| `pkg/config/` | Config generation (XML, YAML, Properties, Env formats) and merging |
| `pkg/listener/` | Listener-related volume and service builders |
| `pkg/reconciler/` | `GenericReconciler`, health checks, dependency resolution, cleanup |
| `pkg/security/` | Pod security context, secret class handling |
| `pkg/sidecar/` | Sidecar manager and providers (Vector, JMX Exporter) |
| `pkg/testutil/` | Testing utilities (envtest, mocks, matchers) |
| `pkg/util/` | K8s utilities (CreateOrUpdate, status updates) |
| `pkg/webhook/` | Webhook infrastructure (defaulter, validator) |

### API Packages (`pkg/apis/`)

| Package | Description |
|---------|-------------|
| `commons/v1alpha1` | Core types: GenericClusterSpec, RoleSpec, RoleGroupSpec, resources, TLS, credentials |
| `authentication/v1alpha1` | Authentication CRDs |
| `database/v1alpha1` | Database connection CRDs |
| `listeners/v1alpha1` | Listener, ListenerClass, PodListener CRDs |
| `s3/v1alpha1` | S3 connection CRDs |

## Development Environment
- **Language**: Go 1.25
- **Dependency Management**: Go Modules (`go.mod`)
- **Testing**: Ginkgo v2 + Gomega
- **Tooling**: Uses `Makefile` to manage local binaries in `bin/`

### Tool Versions
- `controller-gen`: v0.19.0
- `golangci-lint`: v2.10.1
- `kustomize`: v5.7.1
- `controller-runtime`: v0.23.0
- `k8s.io/api`: v0.35.0

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
│   │   ├── database/             # Database connection CRDs
│   │   ├── listeners/            # Listener configurations
│   │   └── s3/                   # S3 connection types
│   ├── builder/                  # Resource builders
│   │   ├── configmap_builder.go  # ConfigMap fluent builder
│   │   ├── pdb_builder.go        # PodDisruptionBudget builder
│   │   ├── service_builder.go    # Service builder
│   │   └── statefulset_builder.go # StatefulSet builder
│   ├── common/                   # Core interfaces and utilities
│   │   ├── cluster_interface.go  # ClusterInterface (all CRs must implement)
│   │   ├── extension.go          # Extension interfaces (Cluster/Role/RoleGroup)
│   │   ├── extension_registry.go # Extension registry
│   │   └── errors.go             # Common error types
│   ├── config/                   # Configuration generation
│   │   ├── generator.go          # ConfigGenerator, MultiFormatConfigGenerator
│   │   ├── merger.go             # ConfigMerger for role/role-group overrides
│   │   ├── xml_adapter.go        # XML format adapter
│   │   ├── yaml_adapter.go       # YAML format adapter
│   │   ├── properties_adapter.go # Properties format adapter
│   │   └── env_adapter.go        # Environment variables adapter
│   ├── listener/                 # Listener utilities
│   │   ├── service_builder.go    # Listener service builder
│   │   └── volume_builder.go     # Listener volume builder
│   ├── reconciler/               # Reconciliation framework
│   │   ├── generic_reconciler.go # GenericReconciler (main entry point)
│   │   ├── role_group_handler.go # RoleGroupHandler interface
│   │   ├── health.go             # Health check manager
│   │   ├── dependency.go         # Dependency resolver
│   │   ├── cleaner.go            # Orphaned resource cleaner
│   │   ├── event.go              # Event manager
│   │   └── errors.go             # Reconcile errors
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
│   │   └── builder_test.go       # Builder test utilities
│   ├── util/                     # General utilities
│   │   ├── k8s_util.go           # K8s utilities (CreateOrUpdate, etc.)
│   │   └── exec_util.go          # Execution utilities
│   └── webhook/                  # Webhook infrastructure
│       ├── webhook.go            # Webhook setup
│       ├── defaulter.go          # Default value setter
│       ├── validator.go          # Validation logic
│       └── errors.go             # Webhook errors
├── examples/                     # Example operators
│   └── trino-operator/           # Trino operator example
│       ├── api/v1alpha1/         # TrinoCluster CRD
│       ├── cmd/main.go           # Operator entry point
│       └── internal/             # Controller, handlers, extensions, webhooks
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
    GetUID() string
    GetLabels() map[string]string
    GetSpec() *v1alpha1.GenericClusterSpec
    GetStatus() *v1alpha1.GenericClusterStatus
    SetStatus(status *v1alpha1.GenericClusterStatus)
    DeepCopyCluster() ClusterInterface
    GetRuntimeObject() runtime.Object
    // ... more methods
}
```

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

### 3. RoleGroupHandler
Product operators implement `RoleGroupHandler` to define resource building logic:
```go
type RoleGroupHandler[CR common.ClusterInterface] interface {
    BuildResources(ctx context.Context, client client.Client, cr CR, buildCtx *RoleGroupBuildContext) (*RoleGroupResources, error)
}
```

### 4. Extension System
Three levels of extensions for injecting custom logic:
- **ClusterExtension**: PreReconcile, PostReconcile, OnReconcileError
- **RoleExtension**: Per-role hooks
- **RoleGroupExtension**: Per-role-group hooks

Extensions have priorities (Lowest=0, Low=25, Normal=50, High=75, Highest=100).

### 5. Resource Builders
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

### 6. Config Generation
Multi-format config generation:
```go
generator := config.NewMultiFormatConfigGenerator()
generator.RegisterDefaultFormats() // .xml, .properties, .yaml, .env
files, err := generator.GenerateFiles(map[string]map[string]string{
    "config.properties": {"key": "value"},
    "config.yaml": {"nested": "data"},
})
```

## Building a New Operator

1. **Define CRD** - Create API types implementing `ClusterInterface`
2. **Create RoleGroupHandler** - Implement resource building logic
3. **Register Extensions** (optional) - Add custom hooks via extension registry
4. **Setup Webhooks** (optional) - Validation and defaulting
5. **Create main.go** - Use `GenericReconciler` with your handler

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
