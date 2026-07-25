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
  - 一种分层配置机制，允许精确自定义生成的资源。它支持覆盖配置文件（如 XML/Properties）、环境变量、CLI 参数和 Pod 属性（通过 PodTemplateSpec）。**重要**：覆盖字段（`configOverrides`、`envOverrides`、`cliOverrides`、`podOverrides`）直接**平铺**在 Role/RoleGroup 级别，**而非**嵌套在 `overrides` 字段下。RoleGroup 覆盖配置继承自 Role 覆盖配置并优先于后者，二者又都优先于产品计算出的配置层（见 §2.5–§2.6）：完整优先级为 **产品配置 < Role < RoleGroup**。

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

配置由一个**有序的层栈**折叠而成,每一层覆盖其下层。`ConfigMerger.Merge` 是变长的,按优先级从低到高依次应用各层:

```
产品配置(最低)  <  Role 覆盖  <  RoleGroup 覆盖(最高)
```

- **产品配置(Product Config)** 是产品*计算*出的配置(见 §2.6),在 reconcile 时作为最低层注入。
- **Role / RoleGroup 覆盖** 是用户在 CRD 里写的 `configOverrides`/`envOverrides`/`cliOverrides`/`podOverrides`。

由于用户的 CRD 覆盖位于产品层之上,**用户在 CRD 中设置的值永远覆盖产品计算出的值**。

各字段类型按既定策略折叠:

- **Map 类型(配置文件 / Env)**:**深度合并(Deep Merge)**。高层的键覆盖低层同名键;新键追加。
- **Slice 类型(CLI 参数)**:由 `ConfigMerger.SliceMergeStrategy` 决定。
  - **Replace**(`MergeStrategyReplace`,默认):高层的**非空**切片完全替换低层切片。
  - **Append**(`MergeStrategyAppend`):高层项目追加到低层切片之后。
  - **空切片表示「未设置」,而非「清空」**:高层为 nil 或空切片时保持低层不变,因此 RoleGroup 无法擦除 Role 设置的 CLI 参数,只能替换它们。
  - `GenericReconciler` 使用 `config.NewConfigMerger()` 构造合并器且不暴露该策略,因此**框架 reconcile 路径上的策略恒为 Replace**;Append 只对自行驱动 `config.ConfigMerger` 的产品代码可用。
- **PodTemplate(`podOverrides`)**:Kubernetes **Strategic Merge Patch**,逐层叠加,允许对 Pod 字段细粒度覆盖(如保持卷挂载的同时更改容器镜像)。若某一层的原始 JSON 无法解析为 `PodTemplateSpec`,或 patch 失败,该层按「不存在」处理;失败原因记录在 `MergedConfig.PodOverrideErrors` 上,并由 reconciler 以 `Warning` 事件暴露(见 §4.14.2),而不是被静默丢弃。

> Role↔RoleGroup 两层合并是该折叠在「无产品层」时的特例;只传这两层的现有调用不受影响。

## 2.6 产品配置 vs. 默认值(层的分离)

SDK 区分产品提供「非用户输入值」的**两种不同机制**,二者不可混为一谈:

| | **`ProductDefaulter`**(Webhook) | **`ProductConfig`**(合并层) |
|---|---|---|
| **对象** | 强类型 **Spec 字段**(image、ports、replicas) | **配置文件内容**(如 `config.properties`、连接串) |
| **时机** | 入场期(MutatingWebhook),**持久化进 Spec** | 每轮 reconcile,**不持久化** |
| **语义** | 静态回退**默认值**(「缺则填」) | **配置计算**(可派生自实时集群状态) |
| **升级传播** | 否——入场期焊进 Spec | **是**——每轮用当前算子重算 |
| **派生自实时状态** | 冻结/过期 | **每轮重算** |

- **`ProductDefaulter`** 适合稳定、用户可见的**强类型 Spec 默认值**(见 §4.3)。其值成为用户持久化 Spec 的一部分,`kubectl get` 可见。
- **`ProductConfig`** 适合**产品内禀及派生的配置文件内容**——如按实际资源拼出的 ZooKeeper 连接串、按 Pod 序号生成的 quorum 列表、按 rolegroup 资源算出的 JVM 堆。它是*配置生成,而非默认*:在 reconcile 时计算(而非入场期焊进 Spec),保证算子升级能把配置变更传播到既有集群,且派生自可变状态的值始终新鲜。它作为最低合并层注入(§2.5),用户覆盖仍然胜出。

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
    - **辅助模型**：`RoleSpec` / `RoleGroupSpec`（role 与 role group 定义）、`RoleConfigSpec`（Role 级 Kubernetes 管控项，如 PDB）、`RoleGroupConfigSpec`（工作负载运行时配置）、`OverridesSpec`（扁平化的覆盖字段）、`ImageSpec`、`LoggingSpec`、`ResourcesSpec`。

- **设计要点**：特定产品的 Spec/Status 必须嵌入通用模型（例如 `HdfsClusterStatus` 嵌入 `GenericClusterStatus`）以实现状态复用。`ServiceHealthy` condition 允许产品报告业务级别的就绪状态（例如 HDFS 安全模式关闭）。

### 3.2.2 抽象接口层（契约定义层）

定义核心接口和扩展契约。它仅依赖于 API 层，是 SDK"多产品复用"的核心，分为业务接口和扩展接口。

- **业务接口**：
    - `ClusterInterface`：集群级接口，定义集群名称、Spec/Status 访问、状态更新等方法。
    - `RoleInterface`：Role 级描述接口，仅提供 `GetRoleName()`、`GetRoleSpec()`、`GetRoleGroups()`、`GetOverrides()`，并附带默认实现 `RoleInfo`/`RoleGroupInfo`。它不包含默认端口，也没有配置扩展器。**接入 SDK 并不需要实现它**：`GenericReconciler` 直接遍历 `GenericClusterSpec.Roles`，从不调用该接口，它只是产品代码的可选便利封装。
    - `RoleGroupHandler`：产品算子的核心实现扩展点。每个产品实现此接口，定义针对每个 RoleGroup 所构建的具体 Kubernetes 资源（StatefulSet、Service、ConfigMap）。`GenericReconciler` 在调和流程中为每个 RoleGroup 调用其 `BuildResources()` 方法。

- **扩展接口**：
    - `ClusterExtension/RoleExtension/RoleGroupExtension`：扩展点接口，定义各级别调和前后的自定义逻辑。对 `role.config` 的 Role 级定制在此完成（`RoleExtension.PreReconcile` 钩子），SDK 中没有单独的扩展器接口。
    - `ExtensionRegistry`：扩展注册表，管理所有扩展的注册、优先级排序和执行。

### 3.2.3 核心组件层（通用逻辑层）

基于抽象接口实现通用业务逻辑。它依赖于抽象接口层和工具层，不直接依赖特定产品，确保逻辑复用。

- **核心组件**：
    - `ClusterReconciler`（SDK 中实现为 `GenericReconciler`）：集群调和器，核心调和流程的入口点，包括 role 遍历、扩展点执行和孤儿资源清理。
    - `ConfigMerger`：配置合并器。通过变长 `Merge(...)` 折叠有序的配置层栈（产品配置 < Role < RoleGroup，见 §2.5），按字段类型应用各自策略（深度合并 / 替换-追加 / Strategic Merge Patch）。
    - `ConfigGenerator`：配置生成器，将合并的配置映射转换为特定文件格式（XML、Properties、YAML 等）。
    - `SidecarManager`：Sidecar 容器管理器，处理将辅助容器（如日志收集、监控）注入到 Pod Spec 中。
    - `StatefulSetBuilder`：资源构建器，生成与 role groups 对应的 StatefulSet 和 Service 等 K8s 资源。
    - `RoleGroupCleaner`：孤儿资源清理器，基于 Spec 和 Status 比较结果清理孤儿 role group 资源。

### 3.2.4 工具层（通用工具层）

为核心组件层提供非侵入式通用工具函数，减少重复编码。

- **核心工具**：
    - `K8sUtil`：K8s 资源操作工具，封装 CreateOrUpdate、Delete 等幂等操作。它是调和循环唯一会自行装配的工具；CR 的 status 写入由调和器自己处理（见 §4.13.2）。
    - `ExecUtil`：Pod 命令执行工具（`util.NewExecUtil(client, restConfig)`），用于在容器内执行命令。它面向**使用者**：调和器从不构造它，需要 exec 的产品自行用手上的 `*rest.Config` 创建。

### 3.2.5 特定产品层（扩展实现层）

基于 SDK 抽象接口实现产品特定逻辑，无需修改 SDK 核心代码，仅依赖 API 层和抽象接口层。

- **实现要点**：
    - CR 结构体实现 `ClusterInterface`，并提供 `RoleGroupHandler` 定义产品特定资源。（`RoleInterface` 是可选的，见 §3.2.2。）
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
| **常量与配置** | 4.15 | 跨领域常量和配置 |

---

## 4.1 泛型转换模块

### 4.1.1 设计背景

原始接口依赖类型断言，存在运行时错误风险和代码冗余。引入 Go Generics 实现编译时类型安全，减少样板代码。

### 4.1.2 核心实现

- **通用调和器骨架**：`GenericReconciler[CR ClusterInterface]`，约束 CR 类型，复用调和流程。
- **通用扩展接口**：`ClusterExtension[CR ClusterInterface]`（以及 `RoleExtension[CR]`、`RoleGroupExtension[CR]`），消除类型断言，直接接收特定 CR 类型。
- **通用 Webhook 契约**：`ProductDefaulter[CR]` / `ProductValidator[CR]`，与 controller-runtime 的 `admission.Defaulter[T]` / `admission.Validator[T]` 同形，因此带类型的实现可直接传给 webhook builder（见 §4.3）。
- **注册表边界上的类型擦除**：`ExtensionRegistry` 内部存放的是 `ClusterExtension[ClusterInterface]` 条目，因此针对具体 CR 类型编写的扩展需通过 `common.AsClusterExtension` / `AsRoleExtension` / `AsRoleGroupExtension` 适配器注册——仅剩的一次类型断言被收敛在这一处。

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
- **注册方法**：`RegisterClusterExtension`、`RegisterRoleExtension`、`RegisterRoleGroupExtension`，每个都另有 `...WithPriority(ext, priority)` 与变长参数的 `...WithOptions(ext, opts...)` 形式。没有统一的 `Register()` 方法——注册表为每个级别维护一份有序列表，级别因此写进了方法名。
- **注册表实例**：`common.GetExtensionRegistry()` 返回进程级单例，进程内所有 `GenericReconciler` 共享它——为某个 CR 类型写的扩展也会在其他产品的集群上执行（并可能失败）。用 `common.NewExtensionRegistry()` 创建独立实例，并通过 `GenericReconcilerConfig.ExtensionRegistry` 注入，即可让某个控制器拥有自己的注册表。`common.ResetExtensionRegistry()` 原地清空单例（供测试使用）而不替换指针，因此已经持有该注册表的调和器能观察到重置结果。
- **注册选项**：`common.WithPriority(p)` 设置优先级（Lowest=0, Low=25, Normal=50, High=75, Highest=100，默认 Normal）；`common.WithStopOnError(bool)` 针对单次注册覆盖该钩子的默认容错策略（见 §4.2.5）。
- **执行顺序**：扩展按**优先级降序执行（优先级高者先执行）**。相同优先级的扩展按**注册顺序**执行——每个条目携带注册序号，因此顺序是全序的，不依赖排序算法是否稳定。

### 4.2.4 扩展生命周期

- **初始化**：扩展在 Operator 启动期间实例化一次。SDK 不会每次调和重新创建扩展。
- **状态管理**：扩展应该是无状态的或管理自己的内部状态。SDK 将当前 CR 上下文传递给每个扩展方法，使其能够访问集群状态而无需持久化扩展状态。
- **关闭**：**没有关闭钩子**。扩展接口只声明 `Name`、`PreReconcile`、`PostReconcile` 以及（cluster 级别的）`OnReconcileError`；若某个扩展持有需要在 Operator 关闭时释放的资源，应自行注册 `manager.Runnable`。

### 4.2.5 执行流程

调和器按**优先级降序**遍历注册表条目，由每个钩子的容错策略决定失败是否跳过其后的条目。

- **正常执行**：扩展按顺序执行，每个扩展接收调和上下文、client 和 CR。
- **对 CR 的修改**：钩子的定位是「观察并行动」，而不是就地修改。框架从不持久化对内存中 CR 的修改；而且 `reconcile()` 在执行 cluster 级 `PreReconcile` **之前**就已快照 `spec := cr.GetSpec()`，因此 role 遍历、清理和健康评估未必能看到钩子改动后的 spec。确需修改 CR 的钩子应通过 client 写回，由随之产生的 watch 事件驱动下一轮调和。
- **错误处理**：
  - 任何钩子失败都会被包装成标明扩展名的 `*ExtensionError`。
  - `PreReconcile`/`PostReconcile` **默认在首个失败处中断**并返回该错误，从而终止本轮调和并映射到 `Degraded` 条件。以 `common.WithStopOnError(false)` 注册的扩展不会中断循环：其失败会被记录日志，后续扩展继续执行，最终把收集到的失败合并返回，仍会进入 CR status。
  - `OnReconcileError` 处理器**默认全部执行**，它们自身的失败仅记录日志——原始调和错误保持权威。若以 `common.WithStopOnError(true)` 注册某个错误处理器，其失败则会中断后续处理器。
- **状态恢复**：若某个扩展已修改外部状态而后续扩展失败，SDK 不做任何回滚。扩展需自行实现补偿逻辑，通常放在 `OnReconcileError` 中。

## 4.3 Webhook 集成模块

### 4.3.1 集成方案

基于 Kubebuilder 注解驱动实践，集成 MutatingWebhook 和 ValidatingWebhook 实现配置预处理和合法性验证。

### 4.3.2 核心功能

- **MutatingWebhook**：
    - **通用逻辑**：`webhook.DefaultGenericClusterSpec(spec, defaultImage)` **只处理镜像**——`spec.image` 缺失时拷贝 operator 的默认 `ImageSpec`，`spec.image.pullPolicy` 为空时设为 `IfNotPresent`。SDK 不提供 CPU/Memory、ZooKeeper 或日志路径的默认值填充。
    - **特定逻辑**：产品侧实现 `ProductDefaulter[CR]` 接口为**强类型 Spec 字段**填充产品特定默认值（如 HDFS Namenode 堆大小、默认端口）。这些是*默认值*——入场期持久化进 Spec 的静态回退。
    - **范围边界**：`ProductDefaulter` 只默认强类型 Spec 字段。产品的**配置文件内容**（以及任何派生自实时集群状态的值）在 reconcile 时由 `ProductConfig` *计算*,不在此默认——区别见 §2.6。
- **ValidatingWebhook**：
    - **通用逻辑**：`webhook.ValidateGenericClusterSpec(spec, fldPath)` **只校验镜像**——当 `spec.image.custom` 未设置时，`repo`、`productVersion`、`kubedoopVersion` 为必填，且 `pullPolicy` 必须是 `Always`/`IfNotPresent`/`Never` 之一。它返回 `field.ErrorList`，便于与产品自身的校验合并。另有两个可选辅助函数供产品校验器使用：`webhook.ValidateFieldLength` 和 `webhook.ValidateNonEmptyMap`。
    - **特定逻辑**：产品侧实现 `ProductValidator[CR]` 接口执行业务规则验证（如 HDFS HA 模式配置验证）。
- **由 CRD schema 而非 admission 代码强制的约束**：副本数下界（`RoleGroupSpec.Replicas` 带 `+kubebuilder:validation:Minimum=0` 与 `+kubebuilder:default=1`）以及 CPU/Memory 数量格式（`resource.Quantity` 类型）由 apiserver 应用的 OpenAPI schema 校验。SDK 有意不在 webhook 代码里重复这些校验。

### 4.3.3 Admission 工作流程概述

MutatingWebhook 首先运行以应用默认值。ValidatingWebhook 随后运行以强制执行不变量。验证失败会在持久化之前拒绝请求，确保只有有效的 spec 进入调和流程。

`ProductDefaulter[CR]`/`ProductValidator[CR]` 与 controller-runtime 的 `admission.Defaulter[T]`/`admission.Validator[T]` 同形，因此带类型的实现可直接接线（controller-runtime v0.23.x）：

```go
func SetupWebhookWithManager(mgr ctrl.Manager) error {
    return ctrl.NewWebhookManagedBy(mgr, &HdfsCluster{}).
        WithDefaulter(&HdfsClusterDefaulter{}).
        WithValidator(&HdfsClusterValidator{}).
        Complete()
}
```

`webhook.NewDefaulterAdapter` / `webhook.NewValidatorAdapter` 会把 CR 类型擦除为 `runtime.Object`，用于较旧的 `WithCustomDefaulter`/`WithCustomValidator` 入口；它们仍然可用，但不再是推荐接法。

### 4.3.4 部署适配

通过 cert-manager 自动生成 TLS 证书，通过 Kubebuilder 自动生成 Webhook 配置文件。部署时无需手动配置证书和访问规则。

## 4.4 孤儿 RoleGroup 资源清理模块

### 4.4.1 核心方案

采用"Spec 与 Status 比较为主，集群资源查询为辅"的混合方案，提高效率的同时避免误删。

### 4.4.2 执行流程

1. 从 Spec 获取 roles 的期望 role group 列表（`desiredGroups`）。本轮调和过的每个 role group 都会记入 `Status.RoleGroups`。
2. 从 `Status.RoleGroups` 获取历史实际 role group 列表（`oldActualGroups`）。
3. 计算孤儿 role groups：`orphanedGroups = oldActualGroups - desiredGroups`。
4. 对每个孤儿 role group，按"PDB → StatefulSet → ConfigMap → Service → headless Service → metrics Service"的顺序，在**一趟**内逐个 Get-then-Delete。
5. **只有真正执行了删除的 role group** 才从 `Status.RoleGroups` 中移除。仍处于灰度删除宽限期内、或主资源属于其他集群的 role group 会保留在状态快照里，等下一轮调和重试，而不是被悄悄遗忘。裁剪后的映射由调和流程末尾的 status 更新（循环第 7 步）持久化。
6. 返回最近的一个待生效灰度删除截止时间，供调和循环精确地安排重新入队（见 §4.8.4）。

### 4.4.3 安全保护机制

- **删除前验证**：
  - 每个资源在删除前都会先 Get；`NotFound` 视为"已经不存在"并直接按成功短路。
  - 所有权通过 **ownerReferences** 确认：资源必须带有 UID 与 CR 匹配、且 `controller` 为 true 的引用。（owner UID 为空时跳过该检查，供直接驱动 cleaner 的调用方使用。）
  - 不属于本集群的资源**不会被删除**——这可以避免同名的手工资源或外部资源被误删。

- **删除顺序**：
  - 资源按依赖顺序删除以避免孤儿引用：
    1. **PDB**（PodDisruptionBudget）——最先删除，以免阻塞 Pod 驱逐。
    2. **StatefulSet**——当 `replicas > 0` 时，cleaner 先把 `spec.replicas` 改为 0，然后再发起 Delete。
    3. **ConfigMap**。
    4. **Service**、**headless Service**（`<resource>-headless`）与 **metrics Service**（`<resource>-metrics`）——metrics Service 与另外两个一样是框架槽位，因此必须在此回收，否则它会比所属的 role group 活得更久。

- **尽力而为的单趟语义**（重要）：
  - 缩容到 0 的 Update 是**尽力而为**的：失败只按 V(1) 记录日志，随后仍然发起 Delete。
  - **任何一次删除都不会等待上一次被观察到已消失**，cleaner 也不会等待 StatefulSet 缩容完成。Pod 通过常规的级联垃圾回收按其 `terminationGracePeriodSeconds` 终止，没有按序号逆序排空的过程。
  - 一次删除失败会中止该集群本轮的清理并把错误返回给调和循环；循环记录日志后继续（清理失败不致命），该 role group 留在 `Status.RoleGroups` 中，下一轮重试。

- **灰度删除（可选的宽限期）**：
  - 当 `GenericReconcilerConfig.GrayDeleteGracePeriod > 0` 时，首次发现孤儿 role group 并不会立即删除。cleaner 会在该 role group 的主资源（StatefulSet，回退到 ConfigMap）上打上 `orphan.zncdata.dev/pending-deletion` 注解（RFC3339 时间戳）并推迟删除。
  - 宽限期结束后的某一轮调和才真正执行删除。剩余时间会返回给调和循环并转换为 `RequeueAfter`，因此删除按时发生，而不必等待无关的 watch 事件。
  - 若该 role group 在截止时间前被重新加回 Spec，注解会被清除，下次再次成为孤儿时可重新获得完整宽限期。
  - 默认值 `0` 表示不写注解，孤儿资源立即删除。

- **PVC 处理**：
  - 默认情况下，**PVC 在孤儿资源清理期间被保留**以保护数据。
  - 在集群 CR 上设置注解 `operator.zncdata.dev/delete-pvcs: "true"` 后，cleaner 会同时删除孤儿 StatefulSet 的 PVC（按 StatefulSet 的 Pod 选择器列出，且在缩容到 0 之前执行，此时选择器仍然有意义）。
  - **适用范围**：仅限孤儿清理，即从 Spec 中移除的 role group。SDK 不注册 finalizer，因此删除整个 CR 不会执行任何 SDK 代码：被删除集群的 PVC 交由 Kubernetes 自身的垃圾回收规则处理。

### 4.4.4 并发冲突处理

- **404 Not Found**：视为成功——资源已被其他进程删除。
- **409 Conflict**：打注解 / 缩容路径是 Get-then-Update，会携带 `resourceVersion`，并发修改因此表现为冲突。cleaner **不会**在内部重试：错误被返回，本轮清理停止，由下一轮调和重新评估。（`retry.RetryOnConflict` 用在 *status 写入* 路径上的 `K8sUtil.UpdateStatusWithRetry`，而不是 cleaner 内部。）
- **429 Too Many Requests**：清理路径上**没有**针对 429 的处理。只有*应用（apply）*路径会把 429 映射为固定的 `RequeueAfter`（`GenericReconcilerConfig.RateLimitRetryAfter`，默认 10s）——这是固定延迟，不是指数退避。
- **状态同步**：清理与 CR Status 并非原子更新。cleaner 只在内存中裁剪真正删除掉的 `Status.RoleGroups` 条目，由调和末尾的 status 更新持久化。若该写入失败，下一轮调和会重新评估同一批孤儿——删除是幂等的，重复执行是安全的。
- **事件**：接线了 `EventManager`（`RoleGroupCleaner.WithEventManager`）时，每个被删除的资源都会发出 `Normal`/`Deleted` 事件；否则删除只体现在 operator 日志中。

### 4.4.5 边界处理

- **CR 首次创建**：Status 为空，无孤儿资源，直接将期望 role groups 同步到 Status。
- **手动资源删除**：依赖幂等删除（IgnoreNotFound）避免错误，在下次调和中同步 Status。
- **Status 篡改**：删除前查询集群资源，只删除实际存在的资源以避免误删。

## 4.5 配置生成器模块

### 4.5.1 设计背景

大数据组件通常需要各种格式的配置文件（如 Hadoop 的 XML、Kafka/Zookeeper 的 Properties、其他的 YAML）。为每个产品硬编码序列化逻辑会导致重复和不一致。

### 4.5.2 核心实现

- **ConfigFormat 接口**：定义配置序列化的契约。它是**双向**的，适配器必须同时实现两个方法。
  - `Marshal(data map[string]string) (string, error)`
  - `Unmarshal(data string) (map[string]string, error)`
- **FormatAdapter**：适配器模式实现，通过 `config.GetFormat(ConfigFormatType)` 选择（`xml`、`properties`、`yaml`、`env`、`ini`；未知类型回退到 properties）：
  - `XMLAdapter`：将键值对转换为 Hadoop 风格的 `<property><name>...</name><value>...</value></property>` XML 结构。
  - `PropertiesAdapter`：转换为标准 Java `.properties` 格式，并对键中的分隔符与值中的续行进行转义。
  - `YAMLAdapter`：通过 `gopkg.in/yaml.v3` 输出扁平映射（会被解析为 bool/数字的值加引号以保持字符串语义）；`Unmarshal` 遇到非扁平映射的文档会直接报错，而不是返回残缺数据。
  - `EnvAdapter`：格式化为 shell 环境变量导出或 .env 文件内容。键必须是合法的 shell 变量名（`^[A-Za-z_][A-Za-z0-9_]*$`），否则报错而不是输出损坏内容。值中的换行、回车与制表符写成 dotenv 风格的 `\n`/`\r`/`\t` 转义，因此多行值在 POSIX shell `source` 该文件时并非逐字节保真。
  - `INIAdapter`：输出 INI 段；键或值含换行、键含 `=`/`:` 或以 `[`、`#`、`;` 开头时报错。
- **产品日志引擎**（`pkg/productlogging`）：独立于上述配置格式适配器的、与产品无关的专用日志引擎。
  - **输入**：深度合并后的 CRD 日志规格（如 `containers.coordinator.loggers.ROOT.level: DEBUG`），一次性转换为框架中立的 `LogConfig`。
  - **生成器**：`LogFileGenerator` 注册表基于中立模型渲染框架特定文件（Logback XML、Log4j2 properties、Python logging）——包含 console/file appender 阈值以及有界滚动文件 appender。
  - **声明**：产品通过 `ContainerLogging`（容器、框架、pattern）声明每容器日志；框架拥有 Vector source 所 glob 的稳定日志文件路径约定——`<LogDir>/<小写容器名>/<容器名>.<框架后缀>`，后缀决定边缘解析器（log4j/logback XMLLayout 为 `.log4j.xml`，log4j2 XMLLayout 为 `.log4j2.xml`，python JSON 行为 `.py.json`）——使生产者与消费者不会漂移。Vector 在边缘解析每种格式，并把事件规范化为稳定 schema（`.timestamp`/`.logger`/`.level`/`.message` + `.errors`，扁平的 `.namespace`/`.cluster`/`.role`/`.roleGroup` 元数据，以及从路径提取的 `.container`/`.file`）。
  - **与 Vector 耦合**：仅当启用 Vector agent 时才生成滚动文件 appender——没有消费者时不存在可写入的共享日志卷（见 Sidecar 注入模块）。
- **集成**：配置生成发生在 **ConfigMap** 路径上，而不是 StatefulSet 构建器里。`BaseRoleGroupHandler.ConfigGenerator`（一个 `config.MultiFormatConfigGenerator`）把 `MergedConfig.ConfigFiles` 渲染成 `map[文件名]内容`，再由 `builder.ConfigMapBuilder.WithMergedConfig(mergedConfig, generator)` 写入角色组 ConfigMap 的 `Data`。未设置生成器时，handler 回退到确定性的 properties 风格渲染（键排序，分隔符与换行转义）。StatefulSet 只负责*挂载*生成出来的 ConfigMap。

### 4.5.3 核心价值

- **统一逻辑**：集中处理文件格式生成的复杂性，避免在每个产品 operator 中重复实现。
- **可扩展性**：通过实现 `ConfigFormat` 接口轻松支持新格式。
- **一致性**：确保生成的配置文件遵循标准格式和转义规则。

## 4.6 Sidecar 注入模块

### 4.6.1 设计背景

日志收集（Vector）、指标监控（JMX Exporter）和服务网格集成等操作需要向业务 Pods 注入辅助容器。在每个 CRD 中手动配置这些 sidecars 会导致配置冗余和维护困难。

### 4.6.2 核心实现

- **SidecarProvider 接口**：定义 sidecar 注入的抽象。Pod spec 是就地修改的，注入必须幂等；config 为 nil 表示"使用 provider 默认值"。
  - `Name() string`
  - `Inject(podSpec *corev1.PodSpec, config *SidecarConfig) error`
  - `Validate(ctx context.Context, c client.Client, namespace string) error`——检查该 provider 的外部依赖（如某个必需的 ConfigMap 键）。
- **注入阶段（Phase）**：`SidecarManager.InjectAll` 按 `(phase, name)` 排序 provider，因此注入是确定性的，Pod 模板不会在多轮调和之间反复变化。阶段常量为 `SidecarPhaseProducer`(10)、`SidecarPhaseDefault`(50)、`SidecarPhasePipeline`(90)。provider 通过实现 `PhasedProvider` 声明自身阶段，或由调用方用 `SidecarManager.RegisterWithPhase` 指定（显式注册的阶段优先）。正是这一机制保证了管道类 provider——例如必须把共享日志卷以读写方式挂到被采集容器上的 Vector——一定在注入这些容器的生产者之后运行。
- **依赖校验**：`GenericReconciler` 会为每个 role group 调用 `SidecarManager.ValidateAll`，时机在 ConfigMap、Services 与额外资源应用**之后**、StatefulSet 应用**之前**。已注册且启用的 provider 若 `Validate` 失败，调和会以 `reconciler.ValidationError` 中止，而不是先创建出因挂载损坏而不断崩溃重启的 Pod。只有在 manager 上接好了 client 与 namespace 之后校验才会真正执行（namespace 是按 CR 变化的）。
- **Provider 放置**：需要生成配置或做外部服务发现的 provider 放在各自的领域包中；简单 provider 保留在 `pkg/sidecar/`。
- **标准实现**：
  - `VectorSidecarProvider`（位于 `pkg/vector/`）：**共享日志管道的唯一所有者**。它创建有大小限制的共享日志 `emptyDir`，以读写方式挂载到声明的生产者容器（产品在此写入日志文件），同样以读写方式挂载到 Vector agent 容器（agent 是先于生产者启动的 native init container，会预创建每个生产者的容器级日志目录——log4j 1.x 与 Python 的文件 handler 不会创建父目录），并注入 agent。配置生成（`RenderVectorConfig`）与聚合器发现（`DiscoverAggregatorAddress`）是同包内独立的纯函数。它声明 `SidecarPhasePipeline`，因此总在生产者容器就位之后注入；其 `Validate` 要求目标 ConfigMap 存在**且带有 `vector.yaml` 键**——否则 agent 会挂上一个没有配置的 ConfigMap 并立即失败。
  - `JmxExporterSidecarProvider`（位于 `pkg/sidecar/`）：注入 Prometheus JMX Exporter agent 并暴露指标端口。
- **工作流程**：`GenericReconciler` 仅在启用 agent **且**至少声明了一个生产者时才注册 Vector provider（配置生产者容器名与日志卷大小）；否则告警并跳过，因此无内容可采集的 agent 永远不会产生非法 Pod。随后 `BaseRoleGroupHandler` 在 StatefulSet 构建后调用 `SidecarManager` 注入 Containers、Volumes 和 VolumeMounts。对于通过 `VectorAggregatorProvider` 暴露聚合器 ConfigMap 的 CR，框架还会将 `vector.yaml` 生成到角色组 ConfigMap 中——使生产者、消费者与配置在同一处保持一致，而非分散在各产品 operator 中。

### 4.6.3 核心价值

- **解耦**：将辅助功能（Logging/Monitoring）与核心业务逻辑分离。
- **可复用性**：标准 sidecars 可以在 HDFS、HBase 和其他产品之间复用，无需代码重复。
- **一致性**：确保整个平台的日志和指标配置统一。

## 4.7 依赖管理模块

### 4.7.1 设计背景

大数据系统通常有严格的启动依赖顺序（如 Zookeeper -> BookKeeper -> Pulsar Broker）。在依赖项准备好之前启动组件通常会导致"CrashLoopBackOff"状态，污染日志并使故障排除复杂化。

### 4.7.2 核心实现

- **外部引用验证是「按需开启」的显式声明，而不是从 Spec 推导出来的。** SDK **不会**遍历 CR Spec 去寻找对象引用。产品通过设置 `GenericReconcilerConfig.Dependencies` 钩子来声明要检查什么：

  ```go
  Dependencies: func(cr *HdfsCluster) []reconciler.Dependency {
      return []reconciler.Dependency{
          {Kind: reconciler.DependencySecret, Name: cr.Spec.Kerberos.SecretName},
          {Kind: reconciler.DependencyConfigMap, Name: cr.Spec.ZookeeperConfigMap},
      }
  },
  ```

  - 支持的 Kind：`DependencyConfigMap` 与 `DependencySecret`。`Dependency.Namespace` 为空时默认取 CR 所在命名空间；`Name` 为空本身即为错误。
  - 钩子为 nil（默认）时，**完全不做任何依赖检查**。
- **在调和循环中的位置**：该检查在 cluster 级 `PreReconcile` 扩展之后、**任何 role 被调和之前**执行，因此缺失对象会以 `DependencyValidation` 调和错误中止本轮，映射为 `Degraded` 条件与 `Warning` 事件，本轮不会创建任何 Pod。
- **DependencyResolver**：钩子背后的辅助组件。它导出的方法——`ValidateConfigMap`、`ValidateSecret`、`ValidateS3Connection`、`ValidateDatabaseConnection`、`ValidateZKConnection`、`ValidateEndpointFormat`、`ParseConnectionStrings`——也可由产品代码直接调用（例如在 `ClusterExtension.PreReconcile` 中）做比"存在性"更丰富的检查。失败返回 `*DependencyError`，由产品映射到自己的条件上。
  - `DependencyResolver.Validate(ctx, spec)` 是为源码兼容保留的稳定 **no-op**，调和流程已不再调用它，请不要依赖它做任何检查。

### 4.7.3 核心价值
- **稳定性**：通过在启动前强制执行依赖检查，防止级联故障和 pod 崩溃循环的"噪音"。
- **清晰性**：在 CR Status 中清楚地指示缺失的先决条件。

## 4.8 健康管理模块

### 4.8.1 设计背景
有状态系统区分"基础设施就绪"（Pod Running）和"服务就绪"（业务逻辑活跃）。例如，HDFS NameNode 可能正在运行但卡在 SafeMode，或者数据库可能正在执行恢复。Operator 状态必须反映这种业务现实。

### 4.8.2 健康检查机制

健康检查步骤在每轮调和中执行一次，位于孤儿清理之后，评估内容包括：
- **工作负载状态**：对 Spec 中每个 role group，用 StatefulSet 的 `readyReplicas` 与该 role group 的期望副本数比较，得出 `Available`、`Progressing`（版本滚动进行中）与 `Degraded`。有意缩容到 `replicas: 0` 的 role group 在 0 个就绪副本时被视为健康；StatefulSet 读取失败的 role group 则既不健康也不可用。
- **服务可用性**：可选的产品级 `ServiceHealthCheck`（见下），通过 `ServiceHealthy` 条件上报。
- **ClusterOperation 短路**：`reconciliationPaused` 上报 `Degraded/ReconciliationPaused`；`stopped` 上报 `Available=False` 且 `Degraded=False`（已停止的集群正是按要求运行的）。

- **检查节奏**：`GenericReconcilerConfig.HealthCheckInterval`（默认 **120 秒**）是调和成功后自我重新入队的间隔，正是它让健康评估具有周期性——见 §4.8.4。设为负值可关闭周期性唤醒。
- **超时**：`GenericReconcilerConfig.HealthCheckTimeout`（默认 **300 秒**）以 `context.WithTimeout` 包裹产品级 `ServiceHealthCheck` 调用，使卡住的探测不会长期占用调和 worker。非正值表示不设截止时间。它不约束工作负载检查——那些只是受调和上下文管辖的普通 client 读取。
- **失败处理**：
  - 健康评估失败会把 CR Status 标记为 **Degraded**，消息中直接点名问题对象：`Unhealthy role groups: <role>/<group>, ...`。
  - `ServiceHealthCheck` 报错或返回不健康时，同时置 `Degraded=True` 与 `ServiceHealthy=False`，并带上探测给出的消息。
  - 健康检查步骤自身抛出的错误只记录日志，**不会**让调和失败；状态在下一轮重新评估。
  - 如果控制器本身遇到内部错误（被捕获的 panic），Status **不会被修改**——内部故障并不能说明集群的真实状态。该 panic 会以错误形式返回，使工作队列按退避策略重试（§4.13.2）。

### 4.8.3 核心实现

- **状态定义**：SDK 通过 Generic Conditions 标准化集群状态：
  - **Available**：至少有一个副本已准备好并正在服务流量。
  - **Progressing**：集群正在推出新版本或扩展副本。
  - **Degraded**：集群遇到问题（如缺失依赖、崩溃循环、健康检查失败）。
  - **ServiceHealthy**：应用级检查通过（如 SafeMode 关闭、RegionServer 注册）。
  - **ReconcileComplete**：SDK 已成功完成最新的调和循环。
- **ServiceHealthCheck 接口**：
  - **契约**：`CheckHealthy(ctx context.Context, client client.Client, namespace, name string) (bool, error)`。`common.ServiceHealthCheckFunc` 可把普通函数适配为该接口，`common.CompositeHealthCheck` 可组合多个检查。
  - **机制**：SDK 传给探测的是 `client.Client` 以及集群的 namespace/name，因此自然的实现方式是读取 Kubernetes 对象或请求产品自身的 HTTP/RPC 端点。框架**不提供**容器内 exec 句柄——这条路径上没有 `*rest.Config`。需要在 Pod 内执行命令的产品，应自行用 `main.go` 中已有的配置构造 `util.NewExecUtil(client, restConfig)`。
  - **示例**：HDFS 通过查询 NameNode 的 JMX/HTTP SafeMode 端点实现；若要在容器内执行 `hdfs dfsadmin -safemode get`，只能借助产品自建的 `ExecUtil`。
  - **注册方式**：`GenericReconcilerConfig.ServiceHealthCheck`。
- **状态聚合**：SDK 将工作负载就绪状态与业务健康检查聚合到最终的 `GenericClusterStatus` 中。Conditions 会携带 `observedGeneration`；`SetCondition` 在状态值未真正翻转时保留原有的 `lastTransitionTime`，因此空闲集群不会产生条件抖动。

### 4.8.4 调和重新入队策略

watch 只覆盖框架自身拥有的资源类型（`StatefulSet`、`ConfigMap`、`Service`、`PodDisruptionBudget`、`ServiceAccount`，以及产品通过 `SetupWithManagerOptions` 注册的任意 GVK）。那些**不会**产生 watch 事件的变化——产品 `ServiceHealthCheck` 探测到的远端劣化、灰度删除宽限期到期——否则将永远不会被重新评估。因此调和循环会自行安排唤醒：

- **成功路径**上，`Reconcile` 返回 `ctrl.Result{RequeueAfter: d}`，其中 `d` 取以下两者中**最早的正值**：
  1. `HealthCheckInterval`（默认 120 秒）——周期性健康检查节奏；
  2. cleaner 返回的最近一个待生效**灰度删除截止时间**（§4.4.3），即距下一个孤儿 role group 可删除的剩余时间。

  当灰度删除截止时间早于健康检查节奏时以前者为准，从而保证延迟删除按时执行。若两者都非正（`HealthCheckInterval` 设为负值且没有待处理项），`d` 为 `0`——不做周期性唤醒，完全由 watch 驱动。
- **429 限流路径**上，`Reconcile` 返回 `RequeueAfter: RateLimitRetryAfter`（默认 10 秒）且 error 为 nil，因此限流不会产生 `Degraded` 条件和错误事件。
- **错误路径**上（含被捕获的 panic），`Reconcile` 返回错误，由 controller-runtime 的限速器施加指数退避，不设置 `RequeueAfter`——两者同时设置没有意义。
- **暂停路径**上（`reconciliationPaused: true`），循环返回 `ctrl.Result{}` 且不重新入队：在用户改动 CR 之前不会有任何变化，而改动本身就会产生 watch 事件。

由于这一节奏会让 operator 定时访问 API server，当计算出的 status 与线上 status 深度相等时会跳过写入——稳态集群的每次唤醒只花费一次读取，而不是一次写入。

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
  1. **声明**：Product CR 通过引用 `ListenerClass` 定义 Role 需要 listener；operator 用 `listener.NewVolume(volumeName, class)`（可选 `.WithListenerName(...)`）注册到 `ListenerProvisioner`。
  2. **注入**：SDK 在 Pod 模板上声明一个**通用临时卷（generic ephemeral volume）**——`Ephemeral.VolumeClaimTemplate`，使用 `listeners.kubedoop.dev` StorageClass、`ReadWriteOnce`、1Mi 请求，并把 listener 注解（`listeners.kubedoop.dev/class`，以及设置了名称时的 `listeners.kubedoop.dev/listenerName`）写在*模板*的 metadata 上。SDK **不会**创建 `PersistentVolumeClaim` 对象，也**不会**创建 Kubernetes `Service`。由 Kubernetes 的临时卷控制器为每个 Pod 物化一个归属该 Pod 的 PVC，因此 operator 无需 PVC 的创建权限，PVC 生命周期与 Pod 绑定。
  3. **实现**：`listener-operator` 的 CSI 驱动程序拦截 Pod 挂载，自动供应所需的 Kubernetes `Service`，并将结果公共地址/端口投影到 Pod 的文件系统中（可通过 `ListenerProvisioner.Path()`/`MustPath()` 读取）。

> **注意**：不存在 listener 的 scope 注解。scope 是 `secret-operator` 的概念（见 `pkg/security`），与 listener 无关；`pkg/listener` 只写出 class 与 listenerName 两个注解。

### 4.10.3 核心价值
- **解耦**：开发者定义*逻辑*端口（如"WebUI"），而运维通过 `ListenerClass` 定义*物理*暴露策略。
- **动态地址感知**：应用程序可以从挂载的文件中读取自己的外部地址（如公共 LoadBalancer IP），解决 Kafka 和 Zookeeper 等分布式系统中常见的"NAT 广告"问题。

## 4.11 运营管理模块 (ClusterOperation)

### 4.11.1 设计背景

Day-2 运维（维护、调试、紧急停止）需要对 Operator 行为进行安全可预测的控制。直接操作底层资源（如手动删除 StatefulSets）是有风险的，可能与 Operator 的调和循环冲突。

### 4.11.2 核心能力

- **调和暂停（`reconciliationPaused: true`）**：
  - **机制**：Reconciler 在循环最开始、任何资源变更（ServiceAccount 创建、PreReconcile 扩展、角色调和）之前检查此标志。如果为 true，则设置 `ReconciliationPaused`（Degraded）状态条件以使暂停可观测，随后跳过该轮所有资源调和，保持被管理资源不变。
  - **用例**：允许管理员手动修改底层 K8s 资源（如修补 StatefulSet 进行调试），而 Operator 不会立即还原更改。
- **优雅停止（`stopped: true`）**：
  - **机制**：Reconciler 将所有 RoleGroup StatefulSets 缩放到 0 副本。
  - **持久性**：关键的是，**PVC（Persistent Volume Claims）和 ConfigMaps 被保留**。这确保了数据安全，同时释放计算资源。
- **优雅关闭**：
  - **机制**：`gracefulShutdownTimeout` 字段配置 Pod 的 `terminationGracePeriodSeconds`。
  - **生命周期钩子**：`preStop` 钩子由产品侧按需接入——`StatefulSetBuilder.WithPreStopHook(command)` / `WithPreStopHTTPGet(path, port)` 可在 SIGTERM 之前执行应用特定的退役逻辑（如 `hdfs dfsadmin -saveNamespace`）。框架默认不注入任何 preStop 钩子。

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

- **统一类型**（`pkg/apis/s3/v1alpha1`、`pkg/apis/database/v1alpha1`）：
  - `S3Connection` / `S3Bucket`：描述 Endpoint、Region、TLS、path-style access、bucket 名称以及凭据 `SecretClass` 引用的标准 CRD。两者都可以在产品 CR 中**内联或按引用**使用。
  - `DatabaseConnection`：描述 Host、Port、驱动类、数据库名与凭据引用的标准 CRD。
- **S3 解析与渲染**（`pkg/s3`）——**按需调用的辅助函数，而非自动流程**：
  - `s3.ResolveConnection(ctx, client, ns, inline, reference)` 与 `s3.ResolveBucket(...)` 把「内联或引用」这对形式收敛为扁平的 `ConnectionInfo` / `BucketInfo`。
  - `ConnectionInfo.S3AProperties()` 返回 Hadoop S3A 客户端属性——`fs.s3a.endpoint`、`fs.s3a.path.style.access`、`fs.s3a.connection.ssl.enabled`，以及设置了 region 时的 `fs.s3a.endpoint.region`。`BucketInfo.S3AURI(prefix)` 渲染 `s3a://<bucket>/<prefix>` 形式的 URI。
  - **由产品自行把返回的 map 合并进自己的配置文件**（引擎需要前缀时自行加前缀，如 `spark.hadoop.`）。`ConfigGenerator` 完全不认识 Connection 对象——它是纯粹的 `map → XML/Properties/YAML/Env/INI` 序列化器。
  - **access key / secret key 从不会被渲染成配置属性。** `ConnectionInfo.CredentialsProvisioner(volumeName)` 返回一个 `security.SecretProvisioner`（满足 `reconciler.VolumeProvider`），把凭据作为 `secret-operator` CSI 卷挂载到 `/kubedoop/secret/<volumeName>`；容器通过 `s3.CredentialsExportScript` 读取，它导出 `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`。
- **DatabaseConnection 没有渲染支持。** SDK 只提供 CRD 类型和 `DependencyResolver.ValidateDatabaseConnection`（对 host 与凭据 `SecretClass` 的形态检查）——没有 JDBC URL 构造器，也没有凭据卷辅助函数。连接串由产品自行拼装。*（尚未实现：对标 `pkg/s3` 的 `pkg/database` 解析器。）*
- **凭据解析**：凭据以 `SecretClass` 形式引用，并通过上述 CSI 卷交付，Operator 自身从不读取密钥材料。详见 [security.md](security.md)。

### 4.12.3 核心价值

- **解耦**：产品 CRD 接受一个稳定、强类型的连接描述，而不是一堆 `configOverrides`。
- **凭据不泄露**：凭据经由 CSI 进入 Pod，从不写入 ConfigMap 或渲染出的配置文件。

## 4.13 错误处理与弹性模块

### 4.13.1 设计背景

分布式系统和 Kubernetes 控制器面临不可预测的故障：网络不稳定、API 限流、资源冲突和逻辑错误。健壮的 SDK 必须确保优雅地处理错误，确保控制器保持稳定（不崩溃）并提供反馈（状态更新），无需人工干预。

### 4.13.2 核心策略

- **调和器弹性**：
  - **Panic 恢复**：顶层 `recover()` 捕获调和循环内的 panic，使某个 CR 处理逻辑中的 bug 不会拖垮整个 Operator 进程。被捕获的 panic 会连同调用栈写入日志，在 CR 上发出 `Warning`/`ReconcilePanic` 事件（前提是 CR 已被取到），并**以错误形式返回**——若吞掉它，本轮会被当作成功，工作队列既不会重试也不会退避。该路径上 **CR Status 被刻意保持原样**：内部故障并不能说明集群的真实状态。
  - **指数退避**：返回错误即把请求交还给 controller-runtime 的限速器，由它以指数增长的延迟重新入队。SDK 自身不额外实现退避；它唯一施加的固定延迟是 429 路径（§4.8.4）。

- **前置校验（在创建工作负载之前快速失败）**：
  - **Role 名称**：会校验 handler 配置的 role 名称确实存在于 CR Spec 的 roles 中。为 CR 未声明的 role 配置 handler 属于接线错误，否则将悄无声息地什么也不产出，现在会直接报错。
  - **声明的依赖**：`GenericReconcilerConfig.Dependencies` 在任何 role 被调和之前完成校验（§4.7.2）。
  - **Sidecar 依赖**：每个启用的 provider 的 `Validate` 在 StatefulSet 应用之前执行，失败时返回 `ValidationError`，而不是先创建出不断崩溃重启的 Pod（§4.6.2）。
  - **非法的 `podOverrides`**：无法解析或 patch 失败的层会被记录到 `MergedConfig.PodOverrideErrors` 并以 `Warning` 事件暴露；该层被跳过，但不会毫无痕迹地消失。

- **并发控制**：
  - **status 写入上的乐观锁**：status 写入直接使用内存中的 CR，不先重新获取——重取会替换整个 status 段，把扩展钩子在本轮算出的产品自有字段一并丢弃（`ClusterInterface` 只暴露内嵌的通用 status）。遇到 409 时只刷新 `resourceVersion`（配置了未缓存的 `APIReader` 时走它，因为 informer 缓存按定义还没看到那次竞争写入），随后带着本轮的 status 原样重试。这是 last-writer-wins：因为控制器是自身 CR status 的唯一写者，所以它是正确的；代价是**其他**写者在读与写之间写入的 status 字段会被覆盖。`NotFound`（CR 在本轮中途被删除）按成功处理。注意 *cleaner* 不做冲突重试——见 §4.4.4。
  - **幂等性**：所有副作用操作（Create/Update/Delete）设计为幂等的。部分失败后的重试是安全的，不会导致重复资源。

- **扩展容错**：
  - **默认快速失败**：`PreReconcile`/`PostReconcile` 失败会中止调和，避免产生配置不完整（例如不安全）的部署。单次注册可用 `common.WithStopOnError(false)` 退出该行为，其失败仍会被返回。
  - **错误传播**：扩展返回的错误被包装为 `*ExtensionError` 并传播到 CR Status。

- **状态可见性**：
  - **Condition 映射**：顶级错误自动映射到 `GenericClusterStatus` 中的 `Degraded` Condition。
  - **推理**：Condition 的 `Reason` 和 `Message` 字段填充错误详情，允许用户/管理员通过 `kubectl get` 诊断问题（如"DependencyMissing: Zookeeper secret not found"）。
  - **无抖动**：当计算出的 status 与线上 status 深度相等时跳过写入，因此周期性重新入队的节奏（§4.8.4）不会变成一连串无意义的写操作。

## 4.14 事件管理模块

### 4.14.1 设计背景

K8s Events 提供集群内重要事件的时间顺序日志。与 Status Conditions（表示*当前*状态）不同，Events 记录*发生了什么*（转换、错误、操作）。系统化的事件记录对于故障排除"为什么 10 分钟前失败了？"至关重要。

### 4.14.2 核心实现

- **统一记录器**：SDK 把 Kubernetes `EventRecorder` 封装为 `EventManager` 并注入到 Reconciler 上下文中。
- **自动化生命周期事件**：
  - **资源操作**：SDK 在创建、更新或删除子资源（StatefulSet、Service、ConfigMap、PDB）时发出 `Normal` 事件，确保可审计性而无需样板代码。孤儿清理在接好 `EventManager`（`RoleGroupCleaner.WithEventManager`）后，会为每个被删除的资源发出 `Deleted` 事件。
  - **调和里程碑**：为调和开始（调试级别）、完成和关键失败发出事件。
- **错误集成**：从调和循环（包括扩展）冒泡的任何触发 `Degraded` 状态的错误都会自动生成带有错误原因的 `Warning` 事件。
- **劣化输入的告警**：有些输入有问题但不致命，它们会单独产生 `Warning` 事件而不是被静默丢弃——解析或 patch 失败的 `podOverrides` 层（`MergedConfig.PodOverrideErrors`），以及被捕获的 panic（`ReconcilePanic`）。

### 4.14.3 核心价值

- **可审计性**：提供 Operator 采取的操作跟踪。
- **故障排除**：警告事件直接出现在 `kubectl describe` 中，提供对失败的即时可见性。

## 4.15 常量架构模块

### 4.15.1 设计理念

SDK 采用**混合常量架构**，将跨领域常量与领域特定常量分离：

- **跨领域常量**（`pkg/constant/`）：跨所有包共享 — 域名、目录路径、Kubernetes 标签、运维标签（enrichment、restarter）。
- **领域特定常量**（`pkg/listener/`、`pkg/security/`）：仅在各自领域内有意义的常量 — CSI 驱动名称、注解键、格式/范围类型。

SDK 中所有标签、注解和 CSI 相关常量均从单一域名常量派生：

```go
// pkg/constant/domain.go
const KubedoopDomain = "kubedoop.dev"
```

领域包从此根常量派生各自的常量：

```go
// pkg/listener/volume_builder.go
const ListenerAPIGroup = "listeners." + constant.KubedoopDomain

// pkg/security/secret_class.go
const SecretAPIGroup = "secrets." + constant.KubedoopDomain
```

这确保了更改组织域名只需更新一处常量。

### 4.15.2 常量分类

**`pkg/constant/domain.go`** — 组织域名：
- `KubedoopDomain`（`"kubedoop.dev"`）

**`pkg/constant/path.go`** — 目录路径：
- `KubedoopRoot`（`"/kubedoop/"`）
- 派生路径：`KubedoopKerberosDir`、`KubedoopTlsDir`、`KubedoopListenerDir`、`KubedoopJmxDir`、`KubedoopSecretDir`、`KubedoopDataDir`、`KubedoopConfigDir`、`KubedoopLogDir`、`KubedoopConfigDirMount`、`KubedoopLogDirMount`

**`pkg/constant/label.go`** — Kubernetes 推荐标签：
- `LabelKubernetesComponent`、`LabelKubernetesInstance`、`LabelKubernetesName`、`LabelKubernetesManagedBy`、`LabelKubernetesRoleGroup`、`LabelKubernetesVersion`
- `MatchingLabelsNames()`：返回用于标签选择器匹配的标签键列表
- Enrichment 标签：`LabelEnrichmentEnable`、`LabelEnrichmentNodeAddress`

**`pkg/constant/restarter.go`** — Restarter 策略：
- `LabelRestarterEnable`、`AnnotationSecretRestarterPrefix`、`AnnotationConfigMapRestarterPrefix`、`LabelRestarterExpiresAtPrefix`

**`pkg/listener/`** — Listener-operator 常量：
- `ListenerAPIGroup`、`ListenerStorageClass`、`CSIDriverName`
- 注解：`ListenerClassAnnotation`、`AnnotationListenerName`（不存在 listener scope 注解——scope 是 `secret-operator` 的概念）
- 类型：`ListenerClass`（cluster-internal、external-stable、external-unstable）
- 供给器：`ListenerProvisioner`（声明式 CSI 监听器卷注册，提供 `RegisterVolume()`、`Volumes()`/`VolumeMounts()`、`AutoInject()`、`Path()`/`MustPath()` 方法；Service 由 `listener-operator` 创建，而非 SDK）

**`pkg/security/`** — Secret-operator 常量：
- `SecretAPIGroup`、`SecretStorageClass`、`CSIDriverName`
- 注解：`SecretClassAnnotation`、`SecretClassScopeAnnotation` 等
- 标签：`LabelSecretsNode`、`LabelSecretsPod`、`LabelSecretsService`
- 类型：`SecretFormat`（tls-pem、tls-p12、kerberos）、`SecretScope`（pod、node、service、listener-volume）
- 供给器：`SecretProvisioner`（声明式 CSI 密钥卷注册，提供 `TLS()`、`KerberosVolume()`、`Custom()` 构造函数）

### 4.15.3 核心价值

- **DRY 原则**：所有平台常量从 `KubedoopDomain` 派生 — 一处修改全局生效。
- **可发现性**：跨领域常量在 `pkg/constant/`，领域常量与领域代码共存。
- **类型安全**：`ListenerClass`、`SecretFormat`、`SecretScope` 等类型在编译时防止无效值。
- **Go 惯用法**：包名为 `constant`（单数，遵循 Go 规范），MixedCaps 命名，`const` 块分组。

# 5. 设计模式的应用

SDK 的核心设计复用了多种经典设计模式，以增强架构的灵活性和可维护性。本节详细介绍每种模式在 SDK 中的应用。

## 5.1 接口隔离模式

### 5.1.1 模式概述

接口隔离原则 (ISP) 指出，客户端不应被迫依赖它们不使用的接口。SDK 通过将功能拆分为细粒度、专注的接口来应用这一点。

### 5.1.2 SDK 中的应用

- **`ClusterInterface`**：定义集群级操作（GetName、GetNamespace、GetSpec、GetStatus、SetStatus、GetRuntimeObject、DeepCopyCluster）。
- **`RoleInterface`**：可选的 role 级描述接口（GetRoleName、GetRoleSpec、GetRoleGroups、GetOverrides）。调和器并不使用它。
- **`RoleGroupHandler`**：定义产品算子实现的 `BuildResources()` 契约，用于生成 RoleGroup 特定的 Kubernetes 资源。
- **`RoleExtension` / `RoleGroupExtension`**：定义产品在 role 与 role group 级别定制行为所用的 Pre/PostReconcile 钩子。
- **`ServiceHealthCheck`**：定义用于业务级就绪状态的健康检查契约。

### 5.1.3 优势

- **降低实现成本**：产品开发者只实现他们需要的接口。
- **接口清晰性**：每个接口有单一、明确定义的职责。
- **可测试性**：较小的接口更容易为单元测试进行模拟。

### 5.1.4 示例

```go
// 产品 CR 实现 ClusterInterface；其余接口都是可选的。
type HdfsCluster struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec              HdfsClusterSpec   `json:"spec,omitempty"`
    Status            HdfsClusterStatus `json:"status,omitempty"`
}

// 内嵌 metav1.ObjectMeta（或 common.ClusterObject）可提供元数据访问方法，
// 但仅靠内嵌是不够的：CR 仍需显式实现 SDK 专有方法——
// GetSpec() *v1alpha1.GenericClusterSpec、GetStatus()、SetStatus()、
// GetScheme()、GetRuntimeObject() 与 DeepCopyCluster()。
func (h *HdfsCluster) GetSpec() *v1alpha1.GenericClusterSpec { return &h.Spec.GenericClusterSpec }
```

> `GetSpec()` 每次调用都新建一个 `GenericClusterSpec` 在语法上合法，但语义微妙：调和循环每轮只快照一次 spec，因此快照之后在内存中所做的修改无法被一致地观察到（见 §4.2.5）。返回指向 CR 内部的指针是更简单的契约。

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
// ConfigFormat 策略接口（双向：两个方法都必须实现）
type ConfigFormat interface {
    Marshal(data map[string]string) (string, error)
    Unmarshal(data string) (map[string]string, error)
}

// 具体策略
type XMLAdapter struct{}        // Hadoop XML 格式
type PropertiesAdapter struct{} // Java .properties 格式
type YAMLAdapter struct{}       // YAML 格式
type EnvAdapter struct{}        // shell / .env 格式
type INIAdapter struct{}        // INI 格式

// 上下文使用策略
type ConfigGenerator struct {
    format ConfigFormat
}
```

## 5.3 模板方法模式

### 5.3.1 模式概述

模板方法模式在基类中定义算法骨架，让子类在不改变算法结构的情况下重写特定步骤。

### 5.3.2 SDK 中的应用

- **`ClusterReconciler`**（SDK：`GenericReconciler`）：将调和工作流（PreReconcile → Reconcile → PostReconcile）定义为固定模板。
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
│     └── Declared ConfigMaps/Secrets (opt-in hook)           │
│  3. For Each Role:                                          │
│     ├── Role PreReconcile Extensions (Hook)                 │
│     ├── For Each RoleGroup:                                 │
│     │   ├── RoleGroup PreReconcile Extensions (Hook)        │
│     │   ├── Build/Apply Resources (ordered, see below)      │
│     │   └── RoleGroup PostReconcile Extensions (Hook)       │
│     └── Role PostReconcile Extensions (Hook)                │
│  4. Cleanup Orphans (-> gray-delete deadline)               │
│  5. Health Check -> Status Conditions                       │
│  6. PostReconcile Extensions (Hook)                         │
│     └── Product-specific post-processing                    │
│  7. Final Status Update (skipped if deep-equal)             │
│  8. Requeue = min(health cadence, gray deadline)            │
└─────────────────────────────────────────────────────────────┘
```

**每个 RoleGroup 的资源应用顺序**

在步骤 3 中，资源按严格的依赖顺序应用：

```
ConfigMap → HeadlessService → Service → ExtraResources → StatefulSet → PDB → MetricsService
```

顺序依据 Kubernetes 资源依赖关系确定：

1. **ConfigMap**：最先创建，因为 Pod 通过卷挂载或环境变量引用 ConfigMap，配置数据必须在 Pod 启动前就绪。
2. **HeadlessService**：StatefulSet 的 `serviceName` 字段必须指向一个 Headless Service。Kubernetes 利用它为每个 Pod 创建稳定可预测的 DNS 条目（`pod-0.svc.ns.svc.cluster.local`），供 Pod 间通信使用，必须在 StatefulSet 创建前存在。
3. **Service**（客户端访问）：在 StatefulSet 之前创建，确保 Pod 就绪后客户端即可连接。
4. **ExtraResources**（产品特定对象）：在 StatefulSet 之前应用，因为它们通常是 Pod 调度的前置条件——例如 Pod 通过临时 CSI 卷引用的 Listener CR（见 `RoleGroupResources.ExtraResources`）。

   在这一步与 StatefulSet 之间，会执行已注册 sidecar provider 的 `Validate` 检查（§4.6.2）——时机足够晚，它们依赖的 ConfigMap 与额外资源都已存在；也足够早，校验失败绝不会产生任何 Pod。
5. **StatefulSet**：在所有依赖（配置、DNS、额外资源）就绪后创建，StatefulSet 控制器随后按序号顺序创建 Pod。
6. **PDB**（PodDisruptionBudget）：在工作负载之后应用，语义上针对已存在的 Pod，在工作负载运行后保障自愿中断期间的可用性。
7. **MetricsService**：最后应用；它只是把已经在运行的 Pod 暴露给 Prometheus 发现，没有任何东西依赖它。

孤儿清理使用的是另一套顺序——`PDB → StatefulSet → ConfigMap → Service → headless Service → metrics Service`（见 §4.4.3）——它**不是**上述创建顺序的严格逆序。二者回答的是不同问题：创建顺序让前置条件先于依赖方就绪，而删除顺序先摘掉 PDB 以免阻塞 Pod 驱逐，最后才删除各个 Service。

**资源应用语义（create-or-update）**

应用资源并非"只创建"：资源已存在时，`applyResource` 会在每轮调和把线上对象更新为 handler 构建出的期望状态，因此 CR spec 的变更（副本数、config overrides、端口……）都会传播到既有资源（issue #526）。更新规则位于 `copyDesiredState`（`pkg/reconciler/apply.go`）：

- **Labels** 由框架拥有，整体替换；**annotations** 是合并的，因此外部注解（如 `kubectl.kubernetes.io/last-applied-configuration`）得以保留。
- **强类型资源**从期望对象拷贝 spec/data，同时保留 Kubernetes 的不可变/已分配字段：StatefulSet 的 `selector`、`serviceName`、`volumeClaimTemplates` 与 `podManagementPolicy` 保持线上值（要改动它们需要人工删除重建迁移）；ConfigMap 的 data 整体替换（被移除的键会消失）。
- **Service** 会被整体赋上期望的 `ServiceSpec`，随后仅还原服务端拥有的/不可变字段——`clusterIP`/`clusterIPs`、`ipFamilies`/`ipFamilyPolicy`、`healthCheckNodePort`、`loadBalancerClass`——并把 API server 已分配的 NodePort 迁移到匹配的期望端口上（先按名称匹配，回退到端口号），除非 handler 已显式指定 NodePort。对 handler 作者的直接含义是：**任何保留为零值的可变 `ServiceSpec` 字段都会覆盖线上值**，因此 handler 必须完整构建它想要的 Service，而不能依赖此前已应用的状态。
- **任意 GVK**（`ExtraResources`）通过 unstructured 转换，通用地拷贝除 `apiVersion`/`kind`/`metadata`/`status` 之外的所有顶层字段。

### 5.3.4 优势

- **一致性**：所有产品遵循相同的调和结构。
- **可控扩展**：产品只能在指定点扩展。
- **可维护性**：核心流程的更改统一影响所有产品。

## 5.4 单例模式

### 5.4.1 模式概述

单例模式确保一个类只有一个实例，并提供全局访问点。

### 5.4.2 SDK 中的应用

- **ExtensionRegistry**：进程级的默认注册表，管理所有扩展并按确定性顺序执行。单例只是*默认值*而非约束：`common.NewExtensionRegistry()` 可创建独立实例，由控制器通过 `GenericReconcilerConfig.ExtensionRegistry` 注入（§4.2.3）。
- **Scheme**：Kubernetes scheme 在 operator 初始化期间注册一次。

### 5.4.3 优势

- **一致性**：扩展管理的单一事实来源。
- **确定性执行**：扩展按优先级降序执行，相同优先级内以注册序号作为全序的次级比较键。
- **线程安全**：注册表由 `sync.RWMutex` 保护；钩子执行基于条目快照进行。

### 5.4.4 示例

```go
// 注册项被包装成 entry，使优先级、注册序号与单次注册的容错策略随扩展一起流转。
// 扩展接口本身是泛型的，注册表以 ClusterInterface 这一实例化形式存放它们。
type extensionEntry[T Extension] struct {
    extension   T
    priority    ExtensionPriority
    seq         uint64 // 注册序号：同优先级下的全序
    stopOnError *bool  // nil 表示使用该钩子的默认值
}

type ExtensionRegistry struct {
    clusterExtensions   []extensionEntry[ClusterExtension[ClusterInterface]]
    roleExtensions      []extensionEntry[RoleExtension[ClusterInterface]]
    roleGroupExtensions []extensionEntry[RoleGroupExtension[ClusterInterface]]
    nextSeq             uint64
    mu                  sync.RWMutex
}

// 进程级默认实例。
var globalRegistry = NewExtensionRegistry()

func GetExtensionRegistry() *ExtensionRegistry { return globalRegistry }
```

针对具体 CR 类型编写的扩展需通过适配器注册，类型断言由此收敛到一处：

```go
registry.RegisterClusterExtensionWithOptions(
    common.AsClusterExtension[*HdfsCluster](&SafeModeExtension{}),
    common.WithPriority(common.PriorityHigh),
)
```

## 5.5 构建者模式

### 5.5.1 模式概述

构建者模式将复杂对象的构建与其表示分离，允许相同的构建过程创建不同的表示。

### 5.5.2 SDK 中的应用

- **StatefulSetBuilder**：逐步构建 `StatefulSet` 资源，处理复杂的配置如卷、容器和亲和规则。
- **ConfigMapBuilder**：使用合并配置构建 ConfigMaps（`WithMergedConfig`，见 §4.5.2）。
- **ServiceBuilder** / **MetricsServiceBuilder**：使用适当的端口和选择器构建 Service 资源。
- **PDBBuilder**、**RBACBuilder**、**ServiceAccountBuilder**：覆盖其余框架拥有的资源类型。
- `BaseRoleGroupHandler` 通过这些构建器生成角色组 ConfigMap 与两个 Service，因此产品即使覆盖工作负载的某一部分，其余部分仍沿用同一套构建规则。
- **返回值归属**：`Build()` 返回的是深拷贝——修改构建出来的对象绝不会反过来影响构建器；并且 RBAC 与 ServiceAccount 构建器上的 `WithLabels`/`WithAnnotations` 是**合并**而非替换既有集合。

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
| Interface Segregation | `ClusterInterface`, `RoleGroupHandler` | Focused, implementable contracts |
| Strategy | Extensions, ConfigFormat | Swappable behaviors |
| Template Method | Reconciliation flow | Consistent process with hooks |
| Singleton | ExtensionRegistry | Global state management |
| Builder | StatefulSetBuilder | Complex object construction |
| Adapter | ConfigFormat adapters | Format interoperability |
| Observer | Event recording | Change notification |

# 6. 关键问题与解决方案

- **类型断言导致的运行时错误和代码冗余**
  - **解决方案**：为调和器、扩展接口与 Webhook 契约引入 Go Generics，把仅剩的类型擦除收敛在注册表适配器中。
  - **核心优势**：编译时类型安全，减少样板代码，提高开发效率。

- **删除 role group 后残留孤儿资源**
  - **解决方案**：对比 Spec 与 Status 快照，通过 ownerReferences 校验归属，并按固定顺序删除孤儿资源；可选的灰度删除宽限期会推迟删除并安排重新入队。
  - **核心优势**：高效精确，避免误删，确保状态收敛。

- **多产品重复的配置验证/默认值逻辑**
  - **解决方案**：Webhook 分为通用和特定逻辑；SDK 提供通用工具，产品侧实现特定接口。
  - **核心优势**：逻辑复用，灵活扩展，前置拦截非法配置。

- **外部基础设施绑定的复杂逻辑（S3/DB）**
  - **解决方案**：引入高级 `Connection`/`Bucket` CRD，配合按需调用的解析与渲染辅助函数（`pkg/s3`），凭据经 CSI 交付而非渲染进配置。数据库连接目前只有强类型 CRD 与校验（§4.12.2）。
  - **核心优势**：将业务逻辑与基础设施细节解耦，降低配置复杂性和常见配置错误。

# 7. 部署与扩展指南

## 7.1 SDK 部署依赖

- **K8s 版本**：1.31+（适配 Webhook AdmissionReviewVersions=v1）。
- **依赖组件**：cert-manager（用于 Webhook 证书生成）、kubebuilder 3.0+（用于代码生成）。
- **权限要求**：Operator 需要对 StatefulSet、Service、ConfigMap 等资源具有 CRUD 权限。

## 7.2 新产品扩展步骤

1. 定义 CRD 结构体，嵌入 SDK Generic Spec/Status 模型。
2. 在 CR 类型上实现 `ClusterInterface` 以适配 SDK 调和流程（见 §5.1.4）。
3. 实现 `RoleGroupHandler`——通常通过内嵌 `BaseRoleGroupHandler`——描述一个 role group 的 Kubernetes 资源。
4. 用 `GenericReconcilerConfig` 接线出 `GenericReconciler`，声明产品需要的可选钩子（`ProductConfig`、`Dependencies`、`ServiceHealthCheck`、`ExtensionRegistry`、灰度删除与健康检查间隔等）。
5. （可选）实现 `ProductDefaulter`/`ProductValidator` 接口以自定义 Webhook 逻辑。
6. （可选）注册产品特定扩展以实现差异化业务逻辑。
7. 通过 Kubebuilder 生成 Webhook 和 CRD 配置，部署验证。

# 8. 总结与展望

## 8.1 核心优势总结

通过分层架构、接口驱动设计、泛型转换和扩展点机制，本 SDK 实现了多集群产品的通用逻辑复用和灵活扩展。同时解决了孤儿资源、术语冲突和类型安全等关键问题，符合 K8s 生态系统标准，适应生产级 Operator 开发需求。

## 8.2 未来优化方向

以下内容**尚未实现**，描述的是演进方向而非当前行为：

- 支持 **ConversionWebhook** 实现平滑的 CRD 版本升级。
- 添加监控指标统计扩展执行时间、资源清理次数等，便于故障排除。
- 孤儿清理的有序排空：等待 StatefulSet 就绪副本降为 0，并在发起下一次删除前确认上一次已完成（当前清理是尽力而为的单趟流程，§4.4.3）。
- cleaner 内部的冲突/限流韧性（`RetryOnConflict`、429 退避），目前这类处理只存在于 status 写入与资源应用路径上。
- 对标 `pkg/s3` 的 `pkg/database` 解析器（构造 JDBC URL 并提供凭据卷）。
- 可选的 finalizer 支持，使集群删除（而不仅是 role group 变成孤儿）也能执行 SDK 的清理逻辑，例如删除 PVC。
