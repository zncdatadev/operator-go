# Operator-go

[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/zncdatadev/operator-go)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/zncdatadev/operator-go)
[![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/zncdatadev/operator-go/test.yml)](https://github.com/zncdatadev/operator-go/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/zncdatadev/operator-go)](https://goreportcard.com/report/github.com/zncdatadev/operator-go)
[![GitHub License](https://img.shields.io/github/license/zncdatadev/operator-go)](https://github.com/zncdatadev/operator-go/blob/main/LICENSE)
[![GitHub release (latest by date)](https://img.shields.io/github/v/release/zncdatadev/operator-go)](https://github.com/zncdatadev/operator-go/releases)

A Golang SDK/framework for building Kubernetes operators. Built on [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime), it provides a reusable reconciliation framework, CRDs, and utilities for creating product-specific operators.

## Overview

**operator-go** is designed to work seamlessly with [Kubebuilder](https://book.kubebuilder.io/), the standard framework for building Kubernetes APIs. We recommend following the [Kubebuilder documentation](https://book.kubebuilder.io/quick-start.html) to scaffold your operator project, then integrate operator-go to leverage its powerful reconciliation framework.

### Architecture

```txt
┌─────────────────────────────────────────────────────────────┐
│                     Your Operator                           │
│  ┌─────────────────────────────────────────-────────────┐   │
│  │              operator-go Framework                   │   │
│  │  • GenericReconciler  • Resource Builders            │   │
│  │  • Extension System   • Config Generation            │   │
│  │  • Sidecar Management • CRD APIs                     │   │
│  └───────────────────────────────────────────────-──────┘   │
│  ┌──────────────────────────────────────────────-───────┐   │
│  │           controller-runtime (sigs.k8s.io)           │   │
│  └───────────────────────────────────────────────-──────┘   │
└─────────────────────────────────────────────────────────────┘
```

## Features

- **GenericReconciler** - Template Method Pattern-based reconciliation framework with customizable extension points
- **Extension System** - Hook-based customization at cluster, role, and role-group levels
- **Resource Builders** - Fluent builders for StatefulSet, Service, ConfigMap, PDB
- **Config Generation** - Multi-format config file generation (XML, YAML, Properties, Env)
- **Sidecar Management** - Pluggable sidecar injection (Vector, JMX Exporter)
- **CRD APIs** - Common types for authentication, database connections, listeners, and S3

## Getting Started

### Prerequisites

- [Go](https://golang.org/doc/install) 1.25+
- [Kubebuilder](https://book.kubebuilder.io/quick-start.html#install) (recommended for scaffolding)

### Installation

```bash
go get github.com/zncdatadev/operator-go@latest
```

### Quick Start

We recommend using Kubebuilder to scaffold your operator project. Follow the [Kubebuilder Quick Start](https://book.kubebuilder.io/quick-start.html) guide, then integrate operator-go:

#### 1. Define Your CRD

Create your custom resource type implementing `ClusterInterface`:

```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type TrinoCluster struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec              v1alpha1.GenericClusterSpec `json:"spec,omitempty"`
    Status            v1alpha1.GenericClusterStatus `json:"status,omitempty"`
}
```

#### 2. Implement RoleGroupHandler

Define how to build resources for each role group:

```go
type TrinoHandler struct{}

func (h *TrinoHandler) BuildResources(ctx context.Context, client client.Client,
    cr *TrinoCluster, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
    // Build ConfigMaps, Services, StatefulSets, etc.
    return &reconciler.RoleGroupResources{...}, nil
}
```

#### 3. Setup GenericReconciler

Use the framework in your Kubebuilder-generated controller:

```go
func (r *TrinoClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    return reconciler.NewGenericReconciler[*TrinoCluster](
        r.Client,
        r.Scheme,
        &TrinoHandler{},
    ).Reconcile(ctx, req)
}
```

## Core Packages

| Package | Description |
|---------|-------------|
| `pkg/apis/` | Kubernetes API definitions (CRDs) for authentication, database, listeners, S3 |
| `pkg/builder/` | Fluent builders for K8s resources (StatefulSet, Service, ConfigMap, PDB) |
| `pkg/common/` | Core interfaces, extension registry, and error types |
| `pkg/config/` | Config generation and merging for multiple formats |
| `pkg/listener/` | Listener-related volume and service builders |
| `pkg/reconciler/` | GenericReconciler, health checks, dependency resolution |
| `pkg/security/` | Pod security context and secret class handling |
| `pkg/sidecar/` | Sidecar manager and providers (Vector, JMX Exporter) |
| `pkg/testutil/` | Testing utilities (envtest, mocks, matchers) |
| `pkg/webhook/` | Webhook infrastructure (defaulter, validator) |

## Example

A complete example operator is available in [`examples/trino-operator/`](./examples/trino-operator/), demonstrating:

- CRD definition with `ClusterInterface` implementation
- RoleGroupHandler for coordinator and worker roles
- Extension registration for custom logic
- Webhook setup for validation and defaulting

## Development

### Commands

| Command | Description |
|---------|-------------|
| `make generate` | Generate DeepCopy methods |
| `make fmt` | Format code |
| `make vet` | Run go vet |
| `make test` | Run unit tests with coverage |
| `make lint` | Run golangci-lint |

## References

- [Kubebuilder Book](https://book.kubebuilder.io/) - Official documentation for building Kubernetes APIs
- [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) - Core runtime components used by operator-go
- [Kubernetes API Documentation](https://kubernetes.io/docs/reference/using-api/)

## Contributing

Contributions are welcome! Please ensure your code passes `make fmt`, `make vet`, `make lint`, and `make test` before submitting a PR.

## License

Apache 2.0 - see [LICENSE](./LICENSE) for details.
