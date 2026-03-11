# Common Product Cluster Operator SDK 技术架构文档

# 1. 文档概述

## 1.1 文档目的

本文档系统性地阐述了 Common Product Cluster Operator SDK（以下简称"SDK"）的设计理念、架构分层、核心模块实现、设计模式应用以及关键问题解决方案。它为开发者提供了接口规范、集成指南和扩展基础，确保在基于 SDK 开发多个产品（HDFS、HBase、DolphinScheduler 等）时的一致性和可维护性。

## 1.2 核心目标

- **通用逻辑复用**：提炼多产品集群的通用逻辑（调和流程、资源构建、配置合并），减少重复编码。

- **灵活的产品扩展**：通过抽象接口和扩展点机制，支持产品特定逻辑的定制，适应差异化需求。

- **精确的状态收敛**：确保 CR 期望状态（Spec）与集群实际状态一致，解决孤儿资源残留等问题。

- **无缝的生态兼容**：符合 K8s Operator 规范和 Kubebuilder 实践，适配 Webhook、Generics 等主流技术方案。

## 1.3 术语定义

- **Product（产品）**
  - 指由 Operator 管理的软件应用定义，如 HDFS、HBase 或 DolphinScheduler。它定义了可用的组件类型（Roles）和整体服务逻辑。

- **Cluster（集群）**
  - 表示 Product 的特定部署实例，由 Custom Resource (CR) 定义。它是聚合全局配置（如 Image 版本、Security 功能、Vector/Logging sidecars）和所有组件 Roles 的根对象。

- **Role（角色）**
  - 表示 Product 内的逻辑功能组件（如 HDFS 中的 NameNode 或 DataNode）。它作为 RoleGroups 的模板和分组机制，定义了由其定义继承的共享配置（Config Overrides、共享日志设置）。一个 Role 包含两个不同的配置部分：
    - `roleConfig`: Kubernetes 级别管理控制（如 PodDisruptionBudget），仅作用于 Role 级别，**不继承**到 RoleGroups。
    - `config`: 工作负载运行时配置（resources, affinity, logging），作为 RoleGroups 的默认值，可被继承和覆盖。

- **RoleGroup（角色组）**
  - 是 Role 下部署和资源隔离的物理单元。每个 RoleGroup 直接映射到一个 Kubernetes `StatefulSet`（以及关联的 Service、ConfigMap、PDB）。这允许将单个 Role 划分为多个具有不同硬件规格（CPU/Memory）、副本数量或特殊配置的组（例如"高性能" DataNode 组与"标准"组）。

- **SecretClass**
  - 由 `secret-operator` 管理的对象，通过 Kubernetes CSI (Container Storage Interface) 将敏感数据（Certificates、Kerberos Keytabs、Passwords）注入到 Pods 中。Workloads 引用 `SecretClass` 来挂载由特定安全后端动态填充的卷。

- **Overrides（覆盖配置）**
  - 一种分层配置机制，允许精确自定义生成的资源。它支持覆盖配置文件（如 XML/Properties）、环境变量、CLI 参数和 Pod 属性（通过 PodTemplateSpec）。**重要**：覆盖字段（`configOverrides`、`envOverrides`、`cliOverrides`、`podOverrides`）直接**平铺**在 Role/RoleGroup 级别，**而非**嵌套在 `overrides` 字段下。RoleGroup 覆盖配置继承自 Role 覆盖配置并优先于后者。

- **Webhook**
  - 集成到 SDK 中的 Kubernetes admission webhooks，用于默认值设置和验证。MutatingWebhook 首先运行，在持久化之前用安全默认值填充缺失字段，然后 ValidatingWebhook 运行以强制执行不变量和业务规则（如无效副本数、缺失依赖项）。验证失败会拒绝请求，因此只有有效的 spec 进入调和流程。

- **Extension（扩展）**
  - 一种 SDK 特定的插件机制，将自定义业务逻辑直接注入到 Reconciliation 循环中。扩展在 Reconcile 阶段（Pre/Post Reconcile）运行，用于处理复杂操作，如状态更新、动态配置生成或使用 Go 代码与外部系统交互。

- **Orphaned Resources（孤儿资源）**
  - 存在于实际集群中但不再定义在 CR 的 `Spec` 中的 Kubernetes 资源（StatefulSets、Services、ConfigMaps）（例如，删除 RoleGroup 后）。SDK 实现了严格的清理逻辑，以安全地识别和删除这些资源，确保状态收敛。

- **ClusterOperation**
  - 一种集群级控制块，在运行时影响 operator 行为（如 `reconciliationPaused` 和 `stopped`）。它不是覆盖机制的一部分；它是一个操作控制平面输入。

# 2. 核心设计理念

## 2.1 接口驱动设计 (IDD)

通过抽象接口定义核心契约，SDK 核心逻辑依赖于接口而非具体实现，实现"通用逻辑与产品特定逻辑的解耦"。新产品只需实现相应接口，无需修改 SDK 核心代码，降低扩展成本。

## 2.2 期望状态收敛

遵循 K8s Operator 核心范式，以 CR Spec 作为期望状态。通过调和循环将集群实际状态收敛至期望状态，辅以反向收敛逻辑（清理孤儿资源）确保双向一致性。

## 2.3 通用与特定分离

SDK 负责实现通用逻辑（如资源构建、配置合并、通用 Webhook 验证），产品侧通过扩展接口实现特定逻辑（如 HDFS ZK 验证、HBase Region 配置），平衡可复用性与灵活性。

## 2.4 类型安全与幂等性

引入 Go Generics 消除类型断言风险，确保编译时类型安全。所有核心操作（创建/更新/删除资源）实现幂等性，避免重复执行导致的异常。

## 2.5 严格合并策略

为解决 Role 和 RoleGroup 配置之间的冲突，SDK 定义了严格的合并策略：

- **Map 类型（Config/Env）**：使用**深度合并（Deep Merge）**。RoleGroup 中存在的键覆盖 Role 中的键；新键被追加。
- **Slice 类型（CLI/JVM/Volumes）**：支持**替换（Replace）**（默认）和**追加（Append）**模式。
  - **Replace**：如果 RoleGroup 定义了切片，则完全替换 Role 的切片。
  - **Append**：如果配置（例如，通过特定标志或约定），RoleGroup 项目将追加到 Role 的切片。
- **PodTemplate**：遵循 Kubernetes **Strategic Merge Patch** 标准，允许对 Pod 字段进行细粒度覆盖（例如，在保持卷挂载的同时更改容器镜像）。

# 3. 分层架构设计

SDK 采用分层架构设计，自上而下分为 API 层、抽象接口层、核心组件层和工具层。每层职责明确，依赖可控。具体分层和依赖关系如下：

## 3.1 分层架构图

以下展示了架构分层关系（依赖从上到下）：特定产品层 → 抽象接口层 → 核心组件层 → 工具层 → API 层；特定产品层基于抽象接口层实现，依赖 SDK 提供的能力。

```plain text


┌───────────────────┐  Implements
│  Specific Product │←─────────────┐
│  Layer            │             │
│ (HDFS/HBase etc.) │             │
└────────┬──────────┘             │
         │                        │
         ▼                        │
┌───────────────────┐             │
│ Abstract Interface│  Defines Contract
│ Layer             │─────────────┘
│ (Interfaces/Exts) │
└────────┬──────────┘
         │
         ▼
┌───────────────────┐
│ Core Component    │  Common Logic Implementation
│ Layer             │
│(Reconciler/Builder)
└────────┬──────────┘
         │
         ▼
┌───────────────────┐
│ Tools Layer       │  Common Utility Functions
│ (K8s Ops/Exec)    │
└────────┬──────────┘
         │
         ▼
┌───────────────────┐
│ API Layer         │  Data Model Definition
│ (Spec/Status)     │
└───────────────────┘

```

## 3.2 各层核心职责与组件

### 3.2.1 API 层（数据契约层）

定义通用数据模型，作为 SDK 与产品侧的数据交换契约。它不依赖任何其他层，确保模型的稳定性。

- **核心组件**：
    - `GenericClusterSpec`：通用集群配置，包含集群级配置、role 和 role group 配置。
    - `GenericClusterStatus`：通用集群状态，采用标准 Kubernetes **Conditions**（如 `Available`、`Progressing`、`Degraded`、`ServiceHealthy`）来表示超越简单副本数量的复杂状态。
    - **辅助模型**：`RoleCommonConfig`（Role 通用配置）、`RoleGroupCommonConfig`（RoleGroup 通用配置）、`ZKConfig`（ZK 通用配置）等。

- **设计要点**：特定产品的 Spec/Status 必须嵌入通用模型（例如 `HdfsClusterStatus` 嵌入 `GenericClusterStatus`）以实现状态复用。`ServiceHealthy` condition 允许产品报告业务级别的就绪状态（例如 HDFS 安全模式关闭）。

### 3.2.2 抽象接口层（契约定义层）

定义核心接口和扩展契约。它仅依赖于 API 层，是 SDK"多产品复用"的核心，分为业务接口和扩展接口。

- **业务接口**：
    - `ClusterInterface`：集群级接口，定义集群名称、Spec/Status 访问、状态更新等方法。
    - `RoleInterface`：Role 级接口，定义角色名称、默认端口、配置扩展器等方法。
    - `RoleExtender`：Role 扩展器接口，定义扩展 Role 配置的逻辑（如扩展 `role.config` 字段用于产品特定的工作负载设置）。
    - `RoleGroupHandler`：产品算子的核心实现扩展点。每个产品实现此接口，定义针对每个 RoleGroup 所构建的具体 Kubernetes 资源（StatefulSet、Service、ConfigMap）。`GenericReconciler` 在调和流程中为每个 RoleGroup 调用其 `BuildResources()` 方法。

- **扩展接口**：
    - `ClusterExtension/RoleExtension/RoleGroupExtension`：扩展点接口，定义各级别调和前后的自定义逻辑。
    - `ExtensionRegistry`：扩展注册表，管理所有扩展的注册、优先级排序和执行。

### 3.2.3 核心组件层（通用逻辑层）

基于抽象接口实现通用业务逻辑。它依赖于抽象接口层和工具层，不直接依赖特定产品，确保逻辑复用。

- **核心组件**：
    - `GenericReconciler`：通用集群调和器，核心调和流程的入口点，包括 role 遍历、扩展点执行和孤儿资源清理。
    - `ConfigMerger`：配置合并器，实现 role 和 role group 配置的合并和差异化覆盖。
    - `ConfigGenerator`：配置生成器，将合并的配置映射转换为特定文件格式（XML、Properties、YAML 等）。
    - `SidecarManager`：Sidecar 容器管理器，处理将辅助容器（如日志收集、监控）注入到 Pod Spec 中。
    - `StatefulSetBuilder`：资源构建器，生成与 role groups 对应的 StatefulSet 和 Service 等 K8s 资源。
    - `RoleGroupCleaner`：孤儿资源清理器，基于 Spec 和 Status 比较结果清理孤儿 role group 资源。

### 3.2.4 工具层（通用工具层）

为核心组件层提供非侵入式通用工具函数，减少重复编码。

- **核心工具**：
    - `K8sUtil`：K8s 资源操作工具，封装 CreateOrUpdate 和 Delete 等幂等操作。
    - `ExecUtil`：Pod 命令执行工具，支持在调和过程中在容器内执行命令（如磁盘检查）。

### 3.2.5 特定产品层（扩展实现层）

基于 SDK 抽象接口实现产品特定逻辑，无需修改 SDK 核心代码，仅依赖 API 层和抽象接口层。

- **实现要点**：
    - CR 结构体实现 `ClusterInterface`/`RoleInterface` 接口，并提供 `RoleGroupHandler` 定义产品特定资源。
    - 通过扩展接口实现特定逻辑（如 HDFS ZK 连通性检查、Namenode 堆大小配置）。
    - 集成 Webhook 特定验证和默认值填充逻辑。

# 4. 核心模块实现

本节详细介绍 SDK 的核心模块，按五个功能类别组织：

| 类别 | 模块 | 描述 |
|----------|---------|-------------|
| **基础与生命周期** | 4.1-4.4 | 核心框架、扩展、webhooks 和清理 |
| **资源生成** | 4.5-4.6 | 配置和 sidecar 管理 |
| **运维管理** | 4.7-4.8, 4.13-4.14 | 依赖、健康、错误和事件 |
| **安全与网络** | 4.9-4.10 | 安全和服务暴露 |
| **操作控制** | 4.11-4.12 | 运行时控制和连接 |

---

## 4.1 泛型转换模块

### 4.1.1 设计背景

原始接口依赖类型断言，存在运行时错误风险和代码冗余。引入 Go Generics 实现编译时类型安全，减少样板代码。

### 4.1.2 核心实现

- **通用调和器骨架**：`GenericReconciler[CR ClusterInterface]`，约束 CR 类型，复用调和流程。
- **通用扩展接口**：`ClusterExtension[CR ClusterInterface]`，消除类型断言，直接接收特定 CR 类型。
- **通用 Role 扩展器**：`RoleExtender[ExtConfig any]`，约束扩展配置类型，用于扩展 Role 级别配置（如 `role.config` 字段），确保类型安全。

### 4.1.3 核心价值

编译时类型检查减少对运行时类型断言的依赖；新产品只需绑定泛型类型，减少样板代码。

## 4.2 扩展点机制模块

### 4.2.1 设计方法

在调和流程的关键节点预留扩展点，支持在产品侧嵌入自定义逻辑，同时通过注册表统一管理，确保扩展的有序执行。

### 4.2.2 扩展点级别

1. **Cluster 级别**：`PreReconcile`（调和前）、`PostReconcile`（调和后）、`OnReconcileError`（异常时）。
2. **Role 级别**：`PreReconcile`、`PostReconcile`，针对单个 role 执行。
3. **RoleGroup 级别**：`PreReconcile`、`PostReconcile`，针对单个 role group 执行。

### 4.2.3 扩展注册

- **注册时机**：扩展必须在 Operator 初始化期间注册，具体在 Manager 启动之前的 `main.go` 设置阶段。这确保在调和开始时所有扩展都可用。
- **注册方法**：使用 `ExtensionRegistry.Register()` 方法添加扩展。每个扩展必须实现适当的接口（`ClusterExtension`、`RoleExtension` 或 `RoleGroupExtension`）。
- **执行顺序**：扩展按**优先级降序执行（优先级高者先执行）**。相同优先级的扩展按注册顺序执行。使用 `RegisterXxxExtensionWithPriority()` 分配显式优先级值（Lowest=0, Low=25, Normal=50, High=75, Highest=100）。

### 4.2.4 扩展生命周期

- **初始化**：扩展在 Operator 启动期间实例化一次。SDK 不会每次调和重新创建扩展。
- **状态管理**：扩展应该是无状态的或管理自己的内部状态。SDK 将当前 CR 上下文传递给每个扩展方法，使其能够访问集群状态而无需持久化扩展状态。
- **清理**：扩展可以实现可选的 `Cleanup()` 方法，用于在 Operator 关闭期间释放资源。

### 4.2.5 执行流程

调和器遍历扩展注册表中的扩展，按**优先级降序执行**，支持配置"扩展失败时中断流程"以适应不同的容错需求。

- **正常执行**：扩展按顺序执行。每个扩展接收当前上下文，可以修改 CR 或返回错误。
- **错误处理**：
  - 如果扩展返回错误，SDK 捕获错误并将其传播到 CR Status。
  - 触发 `OnReconcileError` 钩子进行清理或日志记录。
  - 后续扩展可能会被跳过，具体取决于错误严重程度（可通过 `StopOnError` 标志配置）。
- **状态恢复**：如果扩展修改了 CR 且后续扩展失败，SDK 不会自动回滚更改。扩展应在需要时实现自己的补偿逻辑。

## 4.3 Webhook 集成模块

### 4.3.1 集成方案

基于 Kubebuilder 注解驱动实践，集成 MutatingWebhook 和 ValidatingWebhook 实现配置预处理和合法性验证。

### 4.3.2 核心功能

- **MutatingWebhook**：
    - **通用逻辑**：填充资源默认值（CPU/Memory）、ZK 配置默认值（端口 2181）、日志路径默认值。
    - **特定逻辑**：产品侧实现 `ProductDefaulter` 接口填充产品特定默认值（如 HDFS Namenode 堆大小）。
- **ValidatingWebhook**：
    - **通用逻辑**：必填字段验证、资源格式验证（CPU/Memory 格式）、副本数量合法性验证。
    - **特定逻辑**：产品侧实现 `ProductValidator` 接口执行业务规则验证（如 HDFS HA 模式配置验证）。

### 4.3.3 Admission 工作流程概述

MutatingWebhook 首先运行以应用默认值。ValidatingWebhook 随后运行以强制执行不变量。验证失败会在持久化之前拒绝请求，确保只有有效的 spec 进入调和流程。

### 4.3.4 部署适配

通过 cert-manager 自动生成 TLS 证书，通过 Kubebuilder 自动生成 Webhook 配置文件。部署时无需手动配置证书和访问规则。

## 4.4 孤儿 RoleGroup 资源清理模块

### 4.4.1 核心方案

采用"Spec 与 Status 比较为主，集群资源查询为辅"的混合方案，提高效率的同时避免误删。

### 4.4.2 执行流程

1. 从 Spec 获取 roles 的期望 role group 列表（`desiredGroups`）。
2. 从 Status.RoleGroups 获取历史实际 role group 列表（`oldActualGroups`）。
3. 计算孤儿 role groups：`orphanedGroups = oldActualGroups - desiredGroups`。
4. 删除前验证资源存在性，按"PDB → StatefulSet → ConfigMap → Service"顺序删除资源。
5. 将 Status.RoleGroups 同步到 `desiredGroups` 并更新实际状态快照。

### 4.4.3 安全保护机制

- **删除前验证**：
  - 在删除任何资源之前，SDK 验证资源是否仍存在于集群中。
  - 检查资源标签以确认所有权（匹配 CR 的所有权引用）。
  - 没有正确所有权标签的资源**不会被删除**，以防止意外删除手动创建的资源。

- **删除顺序**：
  - 资源按依赖顺序删除以避免孤儿引用：
    1. **PDB** (PodDisruptionBudget) - 首先删除以避免阻塞 StatefulSet 删除。
    2. **StatefulSet** - 先缩放到 0，然后删除（确保优雅的 pod 终止）。
    3. **ConfigMap** - 在 StatefulSet 删除后删除。
    4. **Service** - 最后删除，因为其他资源可能引用它。
  - 每次删除在继续下一个资源类型之前等待确认。

- **PVC 处理**：
  - 默认情况下，**PVC 在孤儿资源清理期间被保留**以保护数据。
  - 如果明确请求删除 PVC，SDK 需要通过特定注解进行确认。

### 4.4.4 并发冲突处理

- **乐观锁**：
  - SDK 使用 Kubernetes 资源版本控制来检测并发修改。
  - 如果资源在读取和删除之间被另一个进程修改，操作将使用最新资源版本重试。

- **冲突解决**：
  - **409 Conflict**：自动重新获取资源并重试删除。
  - **429 Too Many Requests**：在重试前实施指数退避。
  - **404 Not Found**：视为成功（资源已被另一个进程删除）。

- **状态同步**：
  - 清理后，SDK 原子性地更新 CR Status 和实际集群状态。
  - 如果 Status 更新失败，下一个调和周期会重新评估孤儿资源。

### 4.4.5 边界处理

- **CR 首次创建**：Status 为空，无孤儿资源，直接将期望 role groups 同步到 Status。
- **手动资源删除**：依赖幂等删除（IgnoreNotFound）避免错误，在下次调和中同步 Status。
- **Status 篡改**：删除前查询集群资源，只删除实际存在的资源以避免误删。

## 4.5 配置生成器模块

### 4.5.1 设计背景

大数据组件通常需要各种格式的配置文件（如 Hadoop 的 XML、Kafka/Zookeeper 的 Properties、其他的 YAML）。为每个产品硬编码序列化逻辑会导致重复和不一致。

### 4.5.2 核心实现

- **ConfigFormat 接口**：定义配置序列化的契约。
  - `Marshal(data map[string]string) (string, error)`
- **FormatAdapter**：适配器模式实现，支持常见格式：
  - `XMLAdapter`：将键值对转换为 Hadoop 风格的 `<property><name>...</name><value>...</value></property>` XML 结构。
  - `PropertiesAdapter`：将键值对转换为标准 Java `.properties` 格式。
  - `YAMLAdapter`：将结构化数据转换为 YAML 格式。
  - `EnvAdapter`：格式化为 shell 环境变量导出或 .env 文件内容。
- **LoggingGenerator**：用于处理结构化日志抽象的专用组件。
  - **输入**：通用 YAML logger 配置（如 `containers.coordinator.loggers.ROOT.level: DEBUG`）。
  - **转换**：将通用级别（DEBUG、INFO）映射到框架特定格式（Log4j2 XML、Logback XML、Python Logging）。
  - **输出**：生成最终日志配置文件（如 `log4j2.properties`）注入到 ConfigMap 中。
- **集成**：`StatefulSetBuilder` 利用 `ConfigGenerator` 将合并的配置映射（来自 `ConfigMerger`）处理为存储在 ConfigMaps 中的最终字符串数据。

### 4.5.3 核心价值

- **统一逻辑**：集中处理文件格式生成的复杂性，避免在每个产品 operator 中重复实现。
- **可扩展性**：通过实现 `ConfigFormat` 接口轻松支持新格式。
- **一致性**：确保生成的配置文件遵循标准格式和转义规则。

## 4.6 Sidecar 注入模块

### 4.6.1 设计背景

日志收集（Vector）、指标监控（JMX Exporter）和服务网格集成等操作需要向业务 Pods 注入辅助容器。在每个 CRD 中手动配置这些 sidecars 会导致配置冗余和维护困难。

### 4.6.2 核心实现

- **SidecarProvider 接口**：定义 sidecar 注入的抽象。
  - `Inject(podSpec *corev1.PodSpec, config SidecarConfig) error`
- **标准实现**：
  - `VectorSidecarProvider`：注入 Vector agent 容器，挂载日志卷，并根据 `vectorAgentConfigMap` 配置环境变量。
  - `JmxExporterSidecarProvider`：注入 Prometheus JMX Exporter agent 并暴露指标端口。
- **工作流程**：`StatefulSetBuilder` 在 Pod Spec 构建期间调用 `SidecarManager`。管理器遍历已启用的提供者并注入 Containers、Volumes 和 VolumeMounts。

### 4.6.3 核心价值

- **解耦**：将辅助功能（Logging/Monitoring）与核心业务逻辑分离。
- **可复用性**：标准 sidecars 可以在 HDFS、HBase 和其他产品之间复用，无需代码重复。
- **一致性**：确保整个平台的日志和指标配置统一。

## 4.7 依赖管理模块

### 4.7.1 设计背景

大数据系统通常有严格的启动依赖顺序（如 Zookeeper -> BookKeeper -> Pulsar Broker）。在依赖项准备好之前启动组件通常会导致"CrashLoopBackOff"状态，污染日志并使故障排除复杂化。

### 4.7.2 核心实现

- **外部引用验证**：
  - SDK 自动验证 CR Spec 中定义的引用外部资源（ConfigMaps、Secrets）的存在性。
- **DependencyResolver**：
  - **组件**：在 `PreReconcile` 期间验证外部依赖（如 Zookeeper Connection）。
  - **动作**：如果依赖缺失，Reconciler 暂停流程并设置带有描述性消息的 `Degraded` condition，有效阻止 Pod 创建直到满足依赖。

### 4.7.3 核心价值
- **稳定性**：通过在启动前强制执行依赖检查，防止级联故障和 pod 崩溃循环的"噪音"。
- **清晰性**：在 CR Status 中清楚地指示缺失的先决条件。

## 4.8 健康管理模块

### 4.8.1 设计背景
有状态系统区分"基础设施就绪"（Pod Running）和"服务就绪"（业务逻辑活跃）。例如，HDFS NameNode 可能正在运行但卡在 SafeMode，或者数据库可能正在执行恢复。Operator 状态必须反映这种业务现实。

### 4.8.2 健康检查机制

SDK 实现了全面的健康检查机制，验证：
- **外部依赖**：所需外部资源的可用性（如 Zookeeper、S3、Database）。
- **服务可用性**：服务是否准备好接受流量。
- **Pod 状态**：各个 Pod 的健康和就绪状态。

- **检查间隔**：健康检查在调和期间每**120 秒**执行一次。
- **超时**：每个健康检查操作的最大超时时间为**300 秒**。
- **失败处理**：
  - 如果健康检查失败，CR Status 被标记为 **Degraded**，并附带适当的 reason 和 message。
  - 如果控制器本身遇到内部错误（如 panic、意外异常），Status **不会被修改**，以防止错误状态传播。
  - 瞬态故障触发重新排队，在下一个调和周期重试。

### 4.8.3 核心实现

- **状态定义**：SDK 通过 Generic Conditions 标准化集群状态：
  - **Available**：至少有一个副本已准备好并正在服务流量。
  - **Progressing**：集群正在推出新版本或扩展副本。
  - **Degraded**：集群遇到问题（如缺失依赖、崩溃循环、健康检查失败）。
  - **ServiceHealthy**：应用级检查通过（如 SafeMode 关闭、RegionServer 注册）。
  - **ReconcileComplete**：SDK 已成功完成最新的调和循环。
- **ServiceHealthCheck 接口**：
  - **契约**：`CheckHealthy(ctx context.Context) (bool, error)`
  - **机制**：通过容器内的 `ExecUtil` 执行或通过查询外部 API。
  - **示例**：HDFS 实现此接口以运行 `hdfs dfsadmin -safemode get`。
- **状态聚合**：SDK 将 Pod Readiness、Dependency Status 和 Business Health Checks 聚合到最终的 `GenericClusterStatus` 中。

### 4.8.4 核心价值

## 4.9 安全模块

### 4.9.1 设计理念
SDK 采用分层安全策略，同时解决**基础设施安全**（K8s 访问控制、Pod 上下文）和**应用安全**（身份、加密）。核心理念依赖于"权限分离"和"自动供应"。

### 4.9.2 基础设施安全（Operator & K8s 层）
- **ServiceAccount 供应**：SDK 可以自动管理 workloads 的 ServiceAccounts，确保 Pods 以与 Operator 自身身份不同的适当身份运行。
- **RBAC 集成**：支持将最低所需权限（RoleBindings）绑定到 workload ServiceAccounts，遵循最小权限原则。
- **Pod Security Context**：强制执行 Pod 执行的安全默认值（如非 root 用户、fsGroup 控制）以防止容器逃逸。

### 4.9.3 应用安全（Workload 层）
- **零接触密钥管理**：利用 `secret-operator` 和 `SecretClass` 抽象，通过 CSI 卷注入敏感数据（Kerberos Keytabs、TLS Certificates），防止 Operator 直接处理 secrets。
- **自动化身份**：支持 `AutoTLS`（用于 mTLS）和 `KerberosKeytab`（用于 Hadoop 生态系统身份）等后端机制，无需手动干预。

> **注意**：有关应用安全和 SecretClass 的详细架构、后端机制和工作流程，请参考专门的安全文档：[Operator-Go Security Architecture](security.md)。

## 4.10 网络访问与服务暴露模块

### 4.10.1 设计背景
大数据服务通常需要复杂的网络暴露策略（如 UI 需要 LoadBalancers、内部 RPC 需要 ClusterIP、有状态节点需要可预测的 DNS）。在 Operator 中硬编码 `Service` 资源是僵化的，限制了部署适应性（如本地部署 vs 云部署）。

### 4.10.2 核心实现
- **Listener Operator 集成**：SDK 将网络暴露委托给 `listener-operator`，有效地将"服务定义"与"服务暴露"解耦。
- **概念：ListenerClass**：
  - 类似于 StorageClass，它抽象地定义暴露策略。
  - **cluster-internal**：为集群内通信创建标准 ClusterIP Service。
  - **external-stable**：创建具有稳定外部 IP 的 LoadBalancer/NodePort（对 Kafka/HDFS 客户端至关重要）。
  - **external-unstable**：创建具有动态 IP 的 LoadBalancer 用于临时访问。
- **工作流程（基于 CSI）**：
  1. **声明**：Product CR 通过引用 `ListenerClass` 定义 Role 需要 listener。
  2. **注入**：SDK 创建带有指向 listener 配置的特定注解的 `PersistentVolumeClaim` (PVC)，而不是直接创建 Kubernetes `Service`。
  3. **实现**：`listener-operator` 的 CSI 驱动程序拦截 Pod 挂载，自动供应所需的 Kubernetes `Service`，并将结果公共地址/端口投影到 Pod 的文件系统中。

### 4.10.3 核心价值
- **解耦**：开发者定义*逻辑*端口（如"WebUI"），而运维通过 `ListenerClass` 定义*物理*暴露策略。
- **动态地址感知**：应用程序可以从挂载的文件中读取自己的外部地址（如公共 LoadBalancer IP），解决 Kafka 和 Zookeeper 等分布式系统中常见的"NAT 广告"问题。

## 4.11 运营管理模块 (ClusterOperation)

### 4.11.1 设计背景

Day-2 运维（维护、调试、紧急停止）需要对 Operator 行为进行安全可预测的控制。直接操作底层资源（如手动删除 StatefulSets）是有风险的，可能与 Operator 的调和循环冲突。

### 4.11.2 核心能力

- **调和暂停（`reconciliationPaused: true`）**：
  - **机制**：Reconciler 在循环最开始（`PreReconcile`）检查此标志。如果为 true，则跳过所有后续逻辑和状态更新。
  - **用例**：允许管理员手动修改底层 K8s 资源（如修补 StatefulSet 进行调试），而 Operator 不会立即还原更改。
- **优雅停止（`stopped: true`）**：
  - **机制**：Reconciler 将所有 RoleGroup StatefulSets 缩放到 0 副本。
  - **持久性**：关键的是，**PVC（Persistent Volume Claims）和 ConfigMaps 被保留**。这确保了数据安全，同时释放计算资源。
- **优雅关闭**：
  - **机制**：`gracefulShutdownTimeout` 字段配置 Pod 的 `terminationGracePeriodSeconds`。
  - **生命周期钩子**：SDK 可以选择性地注入 `preStop` 钩子，在 SIGTERM 信号之前执行应用程序特定的退役逻辑（如 `hdfs dfsadmin -saveNamespace`）。

### 4.11.3 核心价值

- **安全性**：为操作员提供"紧急刹车"。
- **灵活性**：支持手动干预而不会与控制器冲突。

## 4.12 连接与资源绑定模块

### 4.12.1 设计背景

大数据应用通常需要连接到外部基础设施：
- **对象存储**：S3/GCS/Azure Blob 用于数据持久化（如 Hive Warehouse、spark-logs）。
- **元数据数据库**：MySQL/Postgres 用于存储应用元数据（如 Hive Metastore、DolphinScheduler）。
在 `configOverrides` 中硬编码这些连接容易出错且会泄露凭据。

### 4.12.2 核心实现

- **统一类型**：
  - `S3Connection`：Endpoint、Bucket、Region 和 Credential 引用的标准结构体。
  - `DatabaseConnection`：Host、Port、Drive Class 和 Credential 引用的标准结构体。
- **配置渲染**：
  - SDK 自动将这些高级对象转换为应用程序特定的配置文件。
  - *示例*：`S3Connection` 对象由 **ConfigGenerator** 转换为 `core-site.xml` 属性（`fs.s3a.access.key`、`fs.s3a.endpoint` 等）。
- **凭据解析**：对 Secrets 的引用（如 `credentials: secret-name`）被验证并挂载，或解析为 CSI `SecretClass` 引用以进行安全注入。

### 4.12.3 核心价值

- **抽象**：用户定义"什么"（连接到此 S3 存储桶），而不是"如何"（设置这 50 个 Hadoop XML 属性）。
- **可移植性**：从 MinIO 切换到 AWS S3 只需更改 Connection spec，而不是整个应用程序配置。

## 4.13 错误处理与弹性模块

### 4.13.1 设计背景

分布式系统和 Kubernetes 控制器面临不可预测的故障：网络不稳定、API 限流、资源冲突和逻辑错误。健壮的 SDK 必须确保优雅地处理错误，确保控制器保持稳定（不崩溃）并提供反馈（状态更新），无需人工干预。

### 4.13.2 核心策略

- **调和器弹性**：
  - **Panic 恢复**：SDK 包含顶级恢复机制来捕获调和循环内的 panic，防止整个 Operator 进程因特定 CR 处理程序中的错误而崩溃。
  - **指数退避**：瞬态错误（如 API 服务器超时）触发以指数增加的延迟重新排队，防止"惊群"问题。

- **并发控制**：
  - **乐观锁**：更新 K8s 资源时，SDK 处理由并发修改引起的 `Conflict` 错误（HTTP 409）（如 `ResourceVersion` 不匹配）。它采用"Retry-On-Conflict"工具自动刷新对象并重试更新。
  - **幂等性**：所有副作用操作（Create/Update/Delete）设计为幂等的。部分失败后的重试是安全的，不会导致重复资源。

- **扩展容错**：
  - **快速失败**：扩展中的关键错误（如安全配置失败）立即冒泡，停止调和以防止不安全的部署。
  - **错误传播**：扩展返回的错误被捕获并传播到 CR Status。

- **状态可见性**：
  - **Condition 映射**：顶级错误自动映射到 `GenericClusterStatus` 中的 `Degraded` Condition。
  - **推理**：Condition 的 `Reason` 和 `Message` 字段填充错误详情，允许用户/管理员通过 `kubectl get` 诊断问题（如"DependencyMissing: Zookeeper secret not found"）。

## 4.14 事件管理模块

### 4.14.1 设计背景

K8s Events 提供集群内重要事件的时间顺序日志。与 Status Conditions（表示*当前*状态）不同，Events 记录*发生了什么*（转换、错误、操作）。系统化的事件记录对于故障排除"为什么 10 分钟前失败了？"至关重要。

### 4.14.2 核心实现

- **统一记录器**：SDK 封装 Kubernetes `EventRecorder` 并将其注入到 Reconciler 上下文中。
- **自动化生命周期事件**：
  - **资源操作**：SDK 在创建、更新或删除子资源（StatefulSet、Service、PDB）时自动发出 `Normal` 事件，确保可审计性而无需样板代码。
  - **调和里程碑**：为调和开始（调试级别）、完成和关键失败发出事件。
- **错误集成**：从调和循环（包括扩展）冒泡的任何触发 `Degraded` 状态的错误都会自动生成带有错误原因的 `Warning` 事件。

### 4.14.3 核心价值

- **可审计性**：提供 Operator 采取的操作跟踪。
- **故障排除**：警告事件直接出现在 `kubectl describe` 中，提供对失败的即时可见性。

# 5. 设计模式的应用

SDK 的核心设计复用了多种经典设计模式，以增强架构的灵活性和可维护性。本节详细介绍每种模式在 SDK 中的应用。

## 5.1 接口隔离模式

### 5.1.1 模式概述

接口隔离原则 (ISP) 指出，客户端不应被迫依赖它们不使用的接口。SDK 通过将功能拆分为细粒度、专注的接口来应用这一点。

### 5.1.2 SDK 中的应用

- **`ClusterInterface`**：定义集群级操作（GetName、GetNamespace、GetSpec、GetStatus、SetStatus）。
- **`RoleInterface`**：定义 role 级操作（GetRoleName、GetConfig、GetRoleGroups）。
- **`RoleGroupHandler`**：定义产品算子实现的 `BuildResources()` 契约，用于生成 RoleGroup 特定的 Kubernetes 资源。
- **`RoleExtender`**：定义 Role 扩展点，用于扩展 `role.config` 字段（产品特定的工作负载运行时配置）。
- **`ServiceHealthCheck`**：定义用于业务级就绪状态的健康检查契约。

### 5.1.3 优势

- **降低实现成本**：产品开发者只实现他们需要的接口。
- **接口清晰性**：每个接口有单一、明确定义的职责。
- **可测试性**：较小的接口更容易为单元测试进行模拟。

### 5.1.4 示例

```go
// Product implements only ClusterInterface, not all interfaces
type HdfsCluster struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec              HdfsClusterSpec   `json:"spec,omitempty"`
    Status            HdfsClusterStatus `json:"status,omitempty"`
}

// HdfsCluster automatically satisfies ClusterInterface by embedding GenericClusterSpec
```

## 5.2 策略模式

### 5.2.1 模式概述

策略模式定义一系列算法，封装每个算法，并使它们可互换。SDK 广泛使用此模式进行扩展点和可配置行为。

### 5.2.2 SDK 中的应用

- **扩展接口**：产品实现 `ClusterExtension`、`RoleExtension` 或 `RoleGroupExtension` 来注入自定义调和逻辑。
- **ConfigFormat 接口**：不同的配置序列化器（XML、Properties、YAML、Env）实现相同的接口。
- **SidecarProvider 接口**：不同的 sidecar 注入器（Vector、JMX Exporter）遵循通用契约。

### 5.2.3 优势

- **灵活性**：策略可以在运行时交换而不修改 SDK 核心。
- **开闭原则**：可以在不修改现有代码的情况下添加新策略。
- **隔离性**：每个策略都是隔离的，更容易测试和维护。

### 5.2.4 示例

```go
// ConfigFormat strategy interface
type ConfigFormat interface {
    Marshal(data map[string]string) (string, error)
}

// Concrete strategies
type XMLAdapter struct{}       // Hadoop XML format
type PropertiesAdapter struct{} // Java .properties format
type YAMLAdapter struct{}      // YAML format

// Context uses the strategy
type ConfigGenerator struct {
    format ConfigFormat
}
```

## 5.3 模板方法模式

### 5.3.1 模式概述

模板方法模式在基类中定义算法骨架，让子类在不改变算法结构的情况下重写特定步骤。

### 5.3.2 SDK 中的应用

- **`GenericReconciler`**：将调和工作流（PreReconcile → Reconcile → PostReconcile）定义为固定模板。
- **扩展钩子**：产品通过在特定钩子点实现扩展接口来自定义行为。
- **资源构建**：`StatefulSetBuilder` 遵循构建 K8s 资源的模板。

### 5.3.3 调和模板

```
┌─────────────────────────────────────────────────────────────┐
│                    Reconciliation Template                   │
├─────────────────────────────────────────────────────────────┤
│  1. PreReconcile Extensions (Hook)                          │
│     └── Product-specific pre-processing                     │
│  2. Validate Dependencies                                   │
│     └── Check external resources (ZK, S3, DB)               │
│  3. For Each Role:                                          │
│     ├── Role PreReconcile Extensions (Hook)                 │
│     ├── For Each RoleGroup:                                 │
│     │   ├── RoleGroup PreReconcile Extensions (Hook)        │
│     │   ├── Build/Apply Resources (ordered, see below)      │
│     │   └── RoleGroup PostReconcile Extensions (Hook)       │
│     └── Role PostReconcile Extensions (Hook)                │
│  4. Cleanup Orphaned Resources                              │
│  5. Update Status                                           │
│  6. PostReconcile Extensions (Hook)                         │
│     └── Product-specific post-processing                    │
└─────────────────────────────────────────────────────────────┘
```

**每个 RoleGroup 的资源应用顺序**

在步骤 3 中，资源按严格的依赖顺序应用：

```
ConfigMap → HeadlessService → Service → StatefulSet → PDB
```

顺序依据 Kubernetes 资源依赖关系确定：

1. **ConfigMap**：最先创建，因为 Pod 通过卷挂载或环境变量引用 ConfigMap，配置数据必须在 Pod 启动前就绪。
2. **HeadlessService**：StatefulSet 的 `serviceName` 字段必须指向一个 Headless Service。Kubernetes 利用它为每个 Pod 创建稳定可预测的 DNS 条目（`pod-0.svc.ns.svc.cluster.local`），供 Pod 间通信使用，必须在 StatefulSet 创建前存在。
3. **Service**（客户端访问）：在 StatefulSet 之前创建，确保 Pod 就绪后客户端即可连接。
4. **StatefulSet**：在所有依赖（配置、DNS）就绪后创建，StatefulSet 控制器随后按序号顺序创建 Pod。
5. **PDB**（PodDisruptionBudget）：最后应用，语义上针对已存在的 Pod，在工作负载运行后保障自愿中断期间的可用性。

此创建顺序与孤儿资源清理时的删除顺序（见 §4.4.2）相反。

### 5.3.4 优势

- **一致性**：所有产品遵循相同的调和结构。
- **可控扩展**：产品只能在指定点扩展。
- **可维护性**：核心流程的更改统一影响所有产品。

## 5.4 单例模式

### 5.4.1 模式概述

单例模式确保一个类只有一个实例，并提供全局访问点。

### 5.4.2 SDK 中的应用

- **ExtensionRegistry**：全局唯一注册表，管理所有扩展。确保扩展只注册一次并按确定性顺序执行。
- **Scheme**：Kubernetes scheme 在 operator 初始化期间注册一次。

### 5.4.3 优势

- **一致性**：扩展管理的单一事实来源。
- **确定性执行**：扩展按优先级降序执行，相同优先级内按注册顺序执行。
- **线程安全**：防止并发场景中的重复注册。

### 5.4.4 示例

```go
// ExtensionRegistry is a global singleton
var globalRegistry = &ExtensionRegistry{
    clusterExtensions:  make([]ClusterExtension, 0),
    roleExtensions:     make([]RoleExtension, 0),
    roleGroupExtensions: make([]RoleGroupExtension, 0),
}

func GetExtensionRegistry() *ExtensionRegistry {
    return globalRegistry
}
```

## 5.5 构建者模式

### 5.5.1 模式概述

构建者模式将复杂对象的构建与其表示分离，允许相同的构建过程创建不同的表示。

### 5.5.2 SDK 中的应用

- **StatefulSetBuilder**：逐步构建 `StatefulSet` 资源，处理复杂的配置如卷、容器和亲和规则。
- **ConfigMapBuilder**：使用合并配置构建 ConfigMaps。
- **ServiceBuilder**：使用适当的端口和选择器构建 Service 资源。

### 5.5.3 构建者工作流程

```go
// StatefulSetBuilder constructs resources step-by-step
type StatefulSetBuilder struct {
    roleGroup    *RoleGroup
    config       *MergedConfig
    sidecars     []SidecarProvider
}

func (b *StatefulSetBuilder) Build() *appsv1.StatefulSet {
    sts := &appsv1.StatefulSet{}
    b.setName(sts)
    b.setLabels(sts)
    b.setReplicas(sts)
    b.setPodSpec(sts)      // Includes containers, volumes, affinity
    b.setVolumeClaims(sts) // PVC configuration
    return sts
}
```

### 5.5.4 优势

- **逐步构建**：复杂资源逐步构建。
- **配置灵活性**：不同的配置产生不同的资源表示。
- **关注点分离**：构建逻辑与业务逻辑隔离。

## 5.6 适配器模式

### 5.6.1 模式概述

适配器模式将一个类的接口转换为客户端期望的另一个接口，使具有不兼容接口的类能够协同工作。

### 5.6.2 SDK 中的应用

- **ConfigFormat 适配器**：将内部配置映射转换为各种外部格式：
  - `XMLAdapter`：适配为 Hadoop XML 格式
  - `PropertiesAdapter`：适配为 Java .properties 格式
  - `YAMLAdapter`：适配为 YAML 格式
  - `EnvAdapter`：适配为环境变量格式

### 5.6.3 优势

- **格式独立性**：SDK 核心使用内部映射表示。
- **可扩展性**：通过实现适配器接口可以添加新格式。
- **可复用性**：同一配置源可以产生多种输出格式。

## 5.7 观察者模式

### 5.7.1 模式概述

观察者模式定义对象间的一对多依赖关系，以便当一个对象更改状态时，所有依赖项都会被通知并自动更新。

### 5.7.2 SDK 中的应用

- **事件记录**：SDK 使用 Kubernetes `EventRecorder` 在资源更改时发出事件。
- **状态更新**：扩展可以通过钩子观察并对状态更改做出反应。

### 5.7.3 优势

- **解耦**：事件发射与业务逻辑解耦。
- **可审计性**：所有重要更改都记录为事件。
- **故障排除**：事件提供操作的时间顺序日志。

## 5.8 模式总结

| Pattern | Primary Application | Key Benefit |
|---------|---------------------|-------------|
| Interface Segregation | `ClusterInterface`, `RoleInterface` | Focused, implementable contracts |
| Strategy | Extensions, ConfigFormat | Swappable behaviors |
| Template Method | Reconciliation flow | Consistent process with hooks |
| Singleton | ExtensionRegistry | Global state management |
| Builder | StatefulSetBuilder | Complex object construction |
| Adapter | ConfigFormat adapters | Format interoperability |
| Observer | Event recording | Change notification |

# 6. 关键问题与解决方案

- **类型断言导致的运行时错误和代码冗余**
  - **解决方案**：引入 Go Generics，设计通用调和器、扩展接口和配置扩展器。
  - **核心优势**：编译时类型安全，减少样板代码，提高开发效率。

- **删除 role group 后残留孤儿资源**
  - **解决方案**：基于 Spec 和 Status 比较结合资源存在性验证，按顺序删除孤儿资源。
  - **核心优势**：高效精确，避免误删，确保状态收敛。

- **多产品重复的配置验证/默认值逻辑**
  - **解决方案**：Webhook 分为通用和特定逻辑；SDK 提供通用工具，产品侧实现特定接口。
  - **核心优势**：逻辑复用，灵活扩展，前置拦截非法配置。

- **外部基础设施绑定的复杂逻辑（S3/DB）**
  - **解决方案**：引入高级 `Connection` 抽象和自动配置渲染策略。
  - **核心优势**：将业务逻辑与基础设施细节解耦，降低配置复杂性和常见配置错误。

# 7. 部署与扩展指南

## 7.1 SDK 部署依赖

- **K8s 版本**：1.31+（适配 Webhook AdmissionReviewVersions=v1）。
- **依赖组件**：cert-manager（用于 Webhook 证书生成）、kubebuilder 3.0+（用于代码生成）。
- **权限要求**：Operator 需要对 StatefulSet、Service、ConfigMap 等资源具有 CRUD 权限。

## 7.2 新产品扩展步骤

1. 定义 CRD 结构体，嵌入 SDK Generic Spec/Status 模型。
2. 实现 `ClusterInterface`/`RoleInterface` 接口和 `RoleGroupHandler` 以适配 SDK 调和流程。
3. （可选）实现 `ProductDefaulter`/`ProductValidator` 接口以自定义 Webhook 逻辑。
4. 注册产品特定扩展以实现差异化业务逻辑。
5. 通过 Kubebuilder 生成 Webhook 和 CRD 配置，部署验证。

# 8. 总结与展望

## 8.1 核心优势总结

通过分层架构、接口驱动设计、泛型转换和扩展点机制，本 SDK 实现了多集群产品的通用逻辑复用和灵活扩展。同时解决了孤儿资源、术语冲突和类型安全等关键问题，符合 K8s 生态系统标准，适应生产级 Operator 开发需求。

## 8.2 未来优化方向

- 支持 **ConversionWebhook** 实现平滑的 CRD 版本升级。
- 扩展扩展点容错机制，支持部分扩展失败时的降级策略。
- 添加监控指标统计扩展执行时间、资源清理次数等，便于故障排除。
- 支持 role group 资源的灰度删除，降低误删风险。
