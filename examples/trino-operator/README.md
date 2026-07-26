# Trino Operator Example

This is an example operator built with [Kubebuilder](https://book.kubebuilder.io/) and the
[operator-go](../../) SDK. It demonstrates all core capabilities of the operator-go SDK.

## Features Demonstrated

- **GenericReconciler Template Method Pattern**: The reconciliation loop is owned by the SDK's
  `GenericReconciler`, which calls product-specific seams at fixed points.
- **BaseRoleGroupHandler Delegation**: `TrinoRoleGroupHandler` embeds
  `reconciler.BaseRoleGroupHandler`, so the framework builds the ConfigMap, Services, StatefulSet
  and role PDB; the override only appends what the merge pipeline cannot express.
- **ProductConfig Hook**: `product.ComputeConfig` contributes Trino's `config.properties` as the
  lowest-precedence merge layer, so any user `configOverrides` wins over it.
- **Typed Extension Registry**: `common.NewExtensionRegistry[*TrinoCluster]()` holds
  `ClusterExtension` (Catalog, Discovery) and `RoleExtension` (Health) hooks, each declaring
  `*TrinoCluster` in its signatures.
- **Admission Webhook**: a `CustomDefaulter` fills product defaults into the typed spec and a
  `CustomValidator` rejects invalid clusters before they reach the reconciler.
- **Declarative Logging**: `LoggingContainers` lets the framework render the Log4j2 config from
  the CRD logging spec.

## Project Structure

```text
trino-operator/
├── api/v1alpha1/                    # CRD definitions
│   ├── trinocluster_types.go        # TrinoCluster CRD (implements ClusterInterface)
│   ├── groupversion_info.go         # Auto-generated
│   └── zz_generated.deepcopy.go     # Auto-generated
├── cmd/
│   └── main.go                      # Entry point: registry + GenericReconciler setup
├── config/
│   ├── crd/                         # CRD YAMLs (auto-generated)
│   ├── rbac/                        # RBAC configuration (auto-generated)
│   ├── webhook/                     # Webhook configuration (auto-generated)
│   ├── certmanager/                 # Serving certificates for the webhook
│   ├── samples/                     # Sample CRs
│   ├── manager/                     # Manager configuration
│   └── default/                     # Kustomize overlay wiring all of the above
├── internal/
│   ├── controller/
│   │   └── trino_handler.go         # RoleGroupHandler (embeds BaseRoleGroupHandler)
│   ├── extensions/
│   │   ├── catalog_extension.go     # ClusterExtension example
│   │   ├── discovery_extension.go   # ClusterExtension + discovery ConfigMap example
│   │   └── health_extension.go      # RoleExtension example
│   ├── product/
│   │   └── config.go                # ProductConfig hook and role name constants
│   ├── config/
│   │   ├── trino_config.go          # jvm.config generation
│   │   └── catalog_config.go        # Catalog properties generation
│   ├── constants/
│   │   └── constants.go             # Image, port and container name constants
│   └── webhook/v1alpha1/
│       └── trinocluster_webhook.go  # Defaulter and validator
├── test/
│   ├── e2e/                         # E2E tests (build tag `e2e`)
│   └── utils/                       # E2E helpers
├── Dockerfile                       # Container image
├── Makefile                         # Build targets
└── README.md                        # This file
```

## Quick Start

### Prerequisites

- Go 1.25+ (see `go.mod`)
- Docker
- kubectl
- Access to a Kubernetes cluster
- [cert-manager](https://cert-manager.io/) in the cluster — the default overlay deploys the
  admission webhook and takes its serving certificate from cert-manager

### Build and Run Locally

`main.go` always registers the admission webhook, so the manager needs a serving certificate even
when run from the host — the webhook server fails to start without one. Either point
`--webhook-cert-path` at a directory holding `tls.crt`/`tls.key`, or place them in
controller-runtime's default directory, `<temp-dir>/k8s-webhook-server/serving-certs`.

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
# Run unit and envtest suites
make test

# Run the e2e suite against a Kind cluster
make test-e2e
```

## Architecture

### GenericReconciler Flow

```text
┌─────────────────────────────────────────────────────────────────┐
│                     GenericReconciler                           │
├─────────────────────────────────────────────────────────────────┤
│ 1. Fetch CR, record observedGeneration                          │
│ 2. ClusterOperation gate (reconciliationPaused returns here)    │
│ 3. Ensure ServiceAccount (when SA management is enabled)        │
│ 4. Execute Cluster PreReconcile extensions                      │
│ 5. Validate dependencies                                        │
│ 6. For each Role (sorted by name):                              │
│    a. Execute Role PreReconcile extensions                      │
│    b. For each RoleGroup:                                       │
│       - Execute RoleGroup PreReconcile extensions               │
│       - Build RoleGroupBuildContext (merged config + sidecars)  │
│       - Delegate to RoleGroupHandler.BuildResources()           │
│       - Apply CM → HeadlessSvc → Svc → extras → STS → PDB       │
│       - Execute RoleGroup PostReconcile extensions              │
│    c. Reconcile the role-level PodDisruptionBudget              │
│    d. Execute Role PostReconcile extensions                     │
│ 7. Cleanup orphaned resources                                   │
│ 8. Update health status                                         │
│ 9. Execute Cluster PostReconcile extensions                     │
│ 10. Write status and schedule the next wakeup                   │
└─────────────────────────────────────────────────────────────────┘
```

A failure anywhere in this flow runs the `OnReconcileError` extensions and maps to the `Degraded`
condition on the CR. API-server rate limiting is the exception: it backs off and retries without
marking the cluster degraded.

### Resource Building Split

```text
GenericReconciler
    │
    ├── per role group: TrinoRoleGroupHandler.BuildResources()
    │       │
    │       ├── BaseRoleGroupHandler.BuildResources()   # the framework's 90%
    │       │       ├── ConfigMap (merged config + Log4j2 logging file)
    │       │       ├── Headless Service + Service
    │       │       └── StatefulSet (image, sidecars, podOverrides)
    │       │
    │       └── product-specific additions
    │               ├── jvm.config            (both roles)
    │               └── catalog/*.properties  (coordinators only)
    │
    └── per role: BaseRoleGroupHandler.BuildRolePodDisruptionBudget()
```

The PDB is deliberately outside `BuildResources`: `roleConfig.podDisruptionBudget` covers all
pods of a role across every role group, so the framework builds exactly one per role instead of
one per group.

Both roles share one handler; the role is read from `buildCtx.RoleName` rather than routed to
separate handler types.

## CRD Example

```yaml
apiVersion: trino.kubedoop.dev/v1alpha1
kind: TrinoCluster
metadata:
  name: demo-trino
spec:
  image:
    productVersion: "476"
    kubedoopVersion: "0.0.0-dev"

  coordinators:
    roleGroups:
      default:
        replicas: 1
        config:
          gracefulShutdownTimeout: "30s"
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

See `config/samples/trino_v1alpha1_trinocluster.yaml` for the full sample.

## Key Integration Points

### 1. Implementing ClusterInterface

`ClusterInterface` has two methods. Everything else the SDK needs — metadata accessors, object
kind, `DeepCopyObject` — comes from the embedded `TypeMeta`/`ObjectMeta` and the generated
deep-copy code.

```go
// GetSpec builds a GenericClusterSpec from the typed coordinators/workers fields, bridging the
// type-safe CRD structure to the SDK's generic Roles map without a redundant spec.roles field.
func (t *TrinoCluster) GetSpec() *commonsv1alpha1.GenericClusterSpec {
    roles := make(map[string]commonsv1alpha1.RoleSpec)
    if t.Spec.Coordinators != nil {
        roles["coordinators"] = t.Spec.Coordinators.RoleSpec
    }
    if t.Spec.Workers != nil {
        roles["workers"] = t.Spec.Workers.RoleSpec
    }
    return &commonsv1alpha1.GenericClusterSpec{
        Image:            t.Spec.Image,
        ClusterOperation: t.Spec.ClusterOperation,
        Roles:            roles,
    }
}

// GetStatus returns a pointer into the CR, so product-specific status fields survive a
// reconcile cycle untouched. There is no SetStatus: the framework mutates through this pointer.
func (t *TrinoCluster) GetStatus() *commonsv1alpha1.GenericClusterStatus {
    return &t.Status.GenericClusterStatus
}
```

Optional seams are separate interfaces the CR may also satisfy — `TrinoCluster` implements
`reconciler.VectorAggregatorProvider` so the framework owns `vector.yaml` generation.

### 2. Implementing RoleGroupHandler

```go
// TrinoRoleGroupHandler embeds the SDK handler, so the framework builds the bulk of the
// resources and the override only appends what the merge pipeline cannot express.
type TrinoRoleGroupHandler struct {
    *reconciler.BaseRoleGroupHandler[*trinov1alpha1.TrinoCluster]
}

func (h *TrinoRoleGroupHandler) BuildResources(
    ctx context.Context,
    k8sClient client.Client,
    cr *trinov1alpha1.TrinoCluster,
    buildCtx *reconciler.RoleGroupBuildContext,
) (*reconciler.RoleGroupResources, error) {
    resources, err := h.BaseRoleGroupHandler.BuildResources(ctx, k8sClient, cr, buildCtx)
    if err != nil {
        return nil, err
    }

    if resources.ConfigMap != nil {
        // setIfAbsent never clobbers a key the merge pipeline already produced (CRD always wins).
        setIfAbsent(resources.ConfigMap.Data, "jvm.config", func() string { return jvmConfig(buildCtx.RoleName) })

        if buildCtx.RoleName == product.RoleCoordinators {
            // ... catalog/<name>.properties, coordinator only
        }
    }

    return resources, nil
}
```

`NewTrinoRoleGroupHandler` configures the framework defaults on the embedded handler
(`ConfigGenerator`, `ConfigMountPath`, `MainContainerName`, `ProductName`, `LoggingContainers`,
per-role container and service ports).

### 3. Contributing Product Config

```go
// ComputeConfig is merged as the LOWEST layer (product < role < role group), so a user's
// configOverrides always win over it. It is recomputed every reconcile and may derive from
// live cluster state — here, the discovery URI of the coordinator Service.
func ComputeConfig(cr *trinov1alpha1.TrinoCluster, roleName, _ string) *commonsv1alpha1.OverridesSpec {
    port := CoordinatorPort(cr)

    props := map[string]string{
        "http-server.http.port": fmt.Sprintf("%d", port),
        "discovery.uri":         discoveryURI(cr, port),
    }
    switch roleName {
    case RoleCoordinators:
        props["coordinator"] = "true"
        props["node-scheduler.include-coordinator"] = "false"
        props["discovery-server.enabled"] = "true"
    case RoleWorkers:
        props["coordinator"] = "false"
    }
    return &commonsv1alpha1.OverridesSpec{
        ConfigOverrides: map[string]map[string]string{
            "config.properties": props,
        },
    }
}
```

### 4. Registering Extensions

The registry is instantiated for the product's own CR type, which is what lets extensions
declare `*TrinoCluster` in their hooks instead of the SDK's wide `ClusterInterface`. There is no
process-global registry: a registry is handed to exactly one reconciler, and an operator that
manages several CR types builds one registry per type.

```go
// In main.go
func newExtensionRegistry(scheme *runtime.Scheme) *common.ExtensionRegistry[*trinov1alpha1.TrinoCluster] {
    registry := common.NewExtensionRegistry[*trinov1alpha1.TrinoCluster]()

    registry.RegisterClusterExtension(extensions.NewCatalogExtension())
    registry.RegisterRoleExtension(extensions.NewHealthExtension())

    // Priority (not registration order) is what keeps the discovery extension running after the
    // catalog extension has refreshed the status.
    registry.RegisterClusterExtension(extensions.NewDiscoveryExtension(scheme), common.WithPriority(common.PriorityLow))

    return registry
}
```

### 5. Wiring the GenericReconciler

The registry only runs when it reaches the reconciler through `ExtensionRegistry`; without that
field the hooks are never executed.

```go
// In main.go
reconcilerCfg := &reconciler.GenericReconcilerConfig[*trinov1alpha1.TrinoCluster]{
    Client: mgr.GetClient(),
    // Uncached: refreshes the resourceVersion after a conflicting status write, which the
    // informer cache is by definition too stale to serve.
    APIReader:           mgr.GetAPIReader(),
    Scheme:              mgr.GetScheme(),
    Recorder:            mgr.GetEventRecorderFor("trino-cluster-controller"),
    RoleGroupHandler:    trinocontroller.NewTrinoRoleGroupHandler(mgr.GetScheme()),
    ProductConfig:       product.ComputeConfig,
    HealthCheckInterval: 120 * time.Second,
    HealthCheckTimeout:  300 * time.Second,
    Prototype:           &trinov1alpha1.TrinoCluster{},
    ExtensionRegistry:   newExtensionRegistry(mgr.GetScheme()),
}

trinoReconciler, err := reconciler.NewGenericReconciler(reconcilerCfg)
if err != nil {
    setupLog.Error(err, "unable to create reconciler")
    os.Exit(1)
}
if err := trinoReconciler.SetupWithManager(mgr); err != nil {
    setupLog.Error(err, "unable to create controller", "controller", "TrinoCluster")
    os.Exit(1)
}
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
