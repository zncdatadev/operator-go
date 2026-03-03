# Trino Operator Example

This is an example operator built with [Kubebuilder](https://book.kubebuilder.io/) and the [operator-go](../../) SDK. It demonstrates all core capabilities of the operator-go SDK.

## Features Demonstrated

- **GenericReconciler Template Method Pattern**: The core reconciliation logic is handled by the SDK’s `GenericReconciler`, which calls product-specific handlers at appropriate points.
- **RoleGroupHandler Product Logic Delegation**: Product-specific resource building is delegated to `TrinoRoleGroupHandler`, which routes to `CoordinatorsHandler` and `WorkersHandler`.
- **Extension Mechanism**: Demonstrates `ClusterExtension` (CatalogExtension) and `RoleExtension` (HealthExtension) for lifecycle hooks.
- **Builder Pattern**: K8s resources are built using a fluent builder pattern.
- **Config Generation**: Trino configuration files (`config.properties`, `jvm.config`, catalog properties) are generated dynamically.

## Project Structure

```
trino-operator/
├── api/v1alpha1/                    # CRD definitions
│   ├── trinocluster_types.go        # TrinoCluster CRD (implements ClusterInterface)
│   ├── groupversion_info.go         # Auto-generated
│   └── zz_generated.deepcopy.go     # Auto-generated
├── cmd/
│   └── main.go                      # Entry point with GenericReconciler setup
├── config/
│   ├── crd/                         # CRD YAMLs (auto-generated)
│   ├── rbac/                        # RBAC configuration (auto-generated)
│   ├── samples/                     # Sample CRs
│   └── manager/                     # Manager configuration
├── internal/
│   ├── controller/
│   │   └── trino_handler.go         # RoleGroupHandler implementation
│   ├── handlers/
│   │   ├── coordinators_handler.go  # Coordinators role handler
│   │   └── workers_handler.go       # Workers role handler
│   ├── extensions/
│   │   ├── catalog_extension.go     # ClusterExtension example
│   │   └── health_extension.go      # RoleExtension example
│   └── config/
│       ├── trino_config.go          # Trino config generation
│       └── catalog_config.go        # Catalog config generation
├── e2e/                             # E2E tests
├── Dockerfile                       # Container image
├── Makefile                         # Build targets
└── README.md                        # This file
```

## Quick Start

### Prerequisites

- Go 1.21+
- Docker
- kubectl
- Access to a Kubernetes cluster

### Build and Run Locally

```bash
# Install CRDs into the cluster
make install

# Run the controller locally
make run

# In another terminal, apply the sample CR
kubectl apply -f config/samples/trino_v1alpha1_trinocluster.yaml
```

### Build and Deploy

```bash
# Build the Docker image
make docker-build IMG=trino-operator:latest

# Push to registry
make docker-push IMG=trino-operator:latest

# Deploy to cluster
make deploy IMG=trino-operator:latest
```

### Run Tests

```bash
# Run unit tests
make test
```

## Architecture

### GenericReconciler Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                     GenericReconciler                           │
├─────────────────────────────────────────────────────────────────┤
│ 1. Fetch CR                                                     │
│ 2. Execute PreReconcile extensions                              │
│ 3. Validate dependencies                                        │
│ 4. For each Role:                                               │
│    a. Execute Role PreReconcile extensions                      │
│    b. For each RoleGroup:                                       │
│       - Execute RoleGroup PreReconcile extensions               │
│       - Build RoleGroupBuildContext                             │
│       - Delegate to RoleGroupHandler.BuildResources()           │
│       - Apply resources (CM → HeadlessSvc → Service → STS → PDB)│
│       - Execute RoleGroup PostReconcile extensions              │
│    c. Execute Role PostReconcile extensions                     │
│ 5. Cleanup orphaned resources                                   │
│ 6. Update health status                                         │
│ 7. Execute PostReconcile extensions                             │
│ 8. Update status                                                │
└─────────────────────────────────────────────────────────────────┘
```

### RoleGroupHandler Routing

```
TrinoRoleGroupHandler.BuildResources()
    │
    ├── RoleCoordinators → CoordinatorsHandler.BuildResources()
    │                           ├── buildConfigMap()
    │                           ├── buildHeadlessService()
    │                           ├── buildService()
    │                           └── buildStatefulSet()
    │
    └── RoleWorkers → WorkersHandler.BuildResources()
                          ├── buildConfigMap()
                          ├── buildHeadlessService()
                          └── buildStatefulSet()
```

## CRD Example

```yaml
apiVersion: trino.kubedoop.dev/v1alpha1
kind: TrinoCluster
metadata:
  name: demo-trino
spec:
  image: trinodb/trino:435

  coordinators:
    roleGroups:
      default:
        replicas: 1
        config:
          resources:
            cpu:
              min: "500m"
              max: "1"
            memory:
              limit: "2Gi"

  workers:
    roleGroups:
      default:
        replicas: 3
        config:
          resources:
            cpu:
              min: "1"
              max: "2"
            memory:
              limit: "4Gi"

  catalogs:
    - name: hive
      type: hive
      properties:
        hive.metastore.uri: "thrift://hive-metastore:9083"
    - name: tpch
      type: tpch
```

## Key Integration Points

### 1. Implementing ClusterInterface

```go
// TrinoCluster implements ClusterInterface
func (t *TrinoCluster) GetSpec() *commonsv1alpha1.GenericClusterSpec {
    return &t.Spec.GenericClusterSpec
}

func (t *TrinoCluster) GetStatus() *commonsv1alpha1.GenericClusterStatus {
    return &t.Status.GenericClusterStatus
}

func (t *TrinoCluster) SetStatus(status *commonsv1alpha1.GenericClusterStatus) {
    t.Status.GenericClusterStatus = *status
}
```

### 2. Implementing RoleGroupHandler

```go
// TrinoRoleGroupHandler implements RoleGroupHandler
func (h *TrinoRoleGroupHandler) BuildResources(
    ctx context.Context,
    k8sClient client.Client,
    cr *trinov1alpha1.TrinoCluster,
    buildCtx *reconciler.RoleGroupBuildContext,
) (*reconciler.RoleGroupResources, error) {
    // Route to role-specific handlers
    switch buildCtx.RoleName {
    case RoleCoordinators:
        return h.coordinatorsHandler.BuildResources(ctx, k8sClient, cr, buildCtx)
    case RoleWorkers:
        return h.workersHandler.BuildResources(ctx, k8sClient, cr, buildCtx)
    }
}
```

### 3. Registering Extensions

```go
// In main.go
catalogExt := extensions.NewCatalogExtension()
common.GetExtensionRegistry().RegisterClusterExtension(catalogExt)

healthExt := extensions.NewHealthExtension()
common.GetExtensionRegistry().RegisterRoleExtension(healthExt)
```

## License

Copyright 2024 ZNCDataDev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

