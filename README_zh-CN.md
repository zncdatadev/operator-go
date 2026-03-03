# Operator-go

[English](./README.md) | 简体中文

[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/zncdatadev/operator-go)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/zncdatadev/operator-go)
[![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/zncdatadev/operator-go/test.yml)](https://github.com/zncdatadev/operator-go/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/zncdatadev/operator-go)](https://goreportcard.com/report/github.com/zncdatadev/operator-go)
[![GitHub License](https://img.shields.io/github/license/zncdatadev/operator-go)](https://github.com/zncdatadev/operator-go/blob/main/LICENSE)
[![GitHub release (latest by date)](https://img.shields.io/github/v/release/zncdatadev/operator-go)](https://github.com/zncdatadev/operator-go/releases)

一个用于构建 Kubernetes Operator 的 Golang SDK/框架。基于 [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) 构建，提供可复用的调和框架、CRD 和实用工具，用于创建产品特定的 Operator。

## 概述

**operator-go** 设计为与 [Kubebuilder](https://book.kubebuilder.io/) 无缝协作，Kubebuilder 是构建 Kubernetes API 的标准框架。我们建议遵循 [Kubebuilder 文档](https://book.kubebuilder.io/quick-start.html) 来搭建您的 Operator 项目脚手架，然后集成 operator-go 以利用其强大的调和框架。

### 架构

```txt
┌─────────────────────────────────────────────────────────────┐
│                     您的 Operator                            │
│  ┌─────────────────────────────────────────-────────────┐   │
│  │              operator-go 框架                         │   │
│  │  • GenericReconciler  • Resource Builders            │   │
│  │  • Extension System   • Config Generation            │   │
│  │  • Sidecar Management • CRD APIs                     │   │
│  └───────────────────────────────────────────────-──────┘   │
│  ┌──────────────────────────────────────────────-───────┐   │
│  │           controller-runtime (sigs.k8s.io)           │   │
│  └───────────────────────────────────────────────-──────┘   │
└─────────────────────────────────────────────────────────────┘
```

## 功能特性

- **GenericReconciler** - 基于模板方法模式的调和框架，具有可定制的扩展点
- **Extension System** - 在集群、角色和角色组级别的基于钩子的定制
- **Resource Builders** - 用于 StatefulSet、Service、ConfigMap、PDB 的流式构建器
- **Config Generation** - 多格式配置文件生成（XML、YAML、Properties、Env）
- **Sidecar Management** - 可插拔的 Sidecar 注入（Vector、JMX Exporter）
- **CRD APIs** - 用于认证、数据库连接、监听器和 S3 的通用类型

## 快速开始

### 前置条件

- [Go](https://golang.org/doc/install) 1.25+
- [Kubebuilder](https://book.kubebuilder.io/quick-start.html#install)（推荐用于脚手架搭建）

### 安装

```bash
go get github.com/zncdatadev/operator-go@latest
```

### 快速入门

我们推荐使用 Kubebuilder 来搭建您的 Operator 项目脚手架。遵循 [Kubebuilder 快速入门](https://book.kubebuilder.io/quick-start.html) 指南，然后集成 operator-go：

#### 1. 定义您的 CRD

创建实现 `ClusterInterface` 的自定义资源类型：

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

#### 2. 实现 RoleGroupHandler

定义如何为每个角色组构建资源：

```go
type TrinoHandler struct{}

func (h *TrinoHandler) BuildResources(ctx context.Context, client client.Client,
    cr *TrinoCluster, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
    // 构建 ConfigMaps、Services、StatefulSets 等
    return &reconciler.RoleGroupResources{...}, nil
}
```

#### 3. 设置 GenericReconciler

在您 Kubebuilder 生成的控制器中使用该框架：

```go
func (r *TrinoClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    return reconciler.NewGenericReconciler[*TrinoCluster](
        r.Client,
        r.Scheme,
        &TrinoHandler{},
    ).Reconcile(ctx, req)
}
```

## 核心包

| 包 | 描述 |
|---------|-------------|
| `pkg/apis/` | Kubernetes API 定义（CRD），用于认证、数据库、监听器、S3 |
| `pkg/builder/` | K8s 资源的流式构建器（StatefulSet、Service、ConfigMap、PDB） |
| `pkg/common/` | 核心接口、扩展注册表和错误类型 |
| `pkg/config/` | 多格式配置生成和合并 |
| `pkg/listener/` | 监听器相关的卷和服务构建器 |
| `pkg/reconciler/` | GenericReconciler、健康检查、依赖解析 |
| `pkg/security/` | Pod 安全上下文和 SecretClass 处理 |
| `pkg/sidecar/` | Sidecar 管理器和提供者（Vector、JMX Exporter） |
| `pkg/testutil/` | 测试工具（envtest、mocks、matchers） |
| `pkg/webhook/` | Webhook 基础设施（defaulter、validator） |

## 示例

完整的示例 Operator 可在 [`examples/trino-operator/`](./examples/trino-operator/) 中找到，演示了：

- 带有 `ClusterInterface` 实现的 CRD 定义
- coordinator 和 worker 角色的 RoleGroupHandler
- 自定义逻辑的扩展注册
- 用于验证和默认值设置的 Webhook 配置

## 开发

### 命令

| 命令 | 描述 |
|---------|-------------|
| `make generate` | 生成 DeepCopy 方法 |
| `make fmt` | 格式化代码 |
| `make vet` | 运行 go vet |
| `make test` | 运行带覆盖率的单元测试 |
| `make lint` | 运行 golangci-lint |

## 参考资料

- [Kubebuilder Book](https://book.kubebuilder.io/) - 构建 Kubernetes API 的官方文档
- [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) - operator-go 使用的核心运行时组件
- [Kubernetes API 文档](https://kubernetes.io/docs/reference/using-api/)

## 贡献

欢迎贡献！请确保您的代码在提交 PR 之前通过 `make fmt`、`make vet`、`make lint` 和 `make test`。

## 许可证

Apache 2.0 - 详见 [LICENSE](./LICENSE)。
