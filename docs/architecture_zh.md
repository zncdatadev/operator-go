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
    - `ClusterInterface`：产品 CR 需要满足的集群级契约。它内嵌 controller-runtime 的 `client.Object`——名称、命名空间、UID、标签、注解与 GVK 都来自 CR 内嵌的 `metav1.ObjectMeta`/`TypeMeta`，无需产品自己编写访问器——在此之上只声明两个 SDK 专有方法：`GetSpec() *v1alpha1.GenericClusterSpec` 与 `GetStatus() *v1alpha1.GenericClusterStatus`，把产品自己的 spec 与 status 投影成框架调和所依据的通用结构。没有 status setter：`GetStatus` 返回指向 CR 内部的指针，框架通过该指针写入 conditions、`observedGeneration` 与 role group 状态，这也是产品自有 status 字段能原样熬过一轮调和的原因。
    - `ClusterResource[T ClusterInterface]`：`ClusterInterface` 再加上 `DeepCopy() T`——controller-gen 已为每个根 API 类型生成的方法。它之所以存在，是因为当 `T` 是指针类型时无法用 `new(T)` 分配类型参数，调和器只能靠拷贝一份原型来得到要读入的空对象；走 `runtime.Object` 会拿回一个接口，从而重新引入运行时断言。持有 CR 时用 `ClusterInterface`，做类型参数时用 `ClusterResource[CR]`。
    - `RoleGroupHandler`：产品算子的核心实现扩展点。每个产品实现此接口，定义针对每个 RoleGroup 所构建的具体 Kubernetes 资源（StatefulSet、Service、ConfigMap）。`GenericReconciler` 在调和流程中为每个 RoleGroup 调用其 `BuildResources()` 方法。
    - **不存在 Role 级接口。** Role 与 RoleGroup 的配置以**数据**形式抵达 handler：调和器为每个 role group 构建 `reconciler.RoleGroupBuildContext` 并传给 `BuildResources`，其中携带 `RoleName`、`RoleSpec`、`RoleGroupName`、`RoleGroupSpec`（Role 级 `config` 已按字段折叠进来，group 优先）以及 `MergedConfig`（产品配置/Role/RoleGroup override 折叠后的结果）。调和器直接遍历 `GenericClusterSpec.Roles`，因此产品只需在 CRD 中声明 role，无需为其实现任何访问器。

- **扩展接口**：
    - `ClusterExtension[CR]/RoleExtension[CR]/RoleGroupExtension[CR]`：扩展点接口，定义各级别调和前后的自定义逻辑。三者都以产品自身的 CR 类型为泛型参数，钩子因此直接拿到该类型。对 `role.config` 的 Role 级定制在此完成（`RoleExtension.PreReconcile` 钩子），SDK 中没有单独的扩展器接口。
    - `ExtensionRegistry[CR ClusterInterface]`：扩展注册表，管理**单一** CR 类型下所有扩展的注册、优先级排序和执行。注册表归接收它的调和器所有（见 §4.2.3）；包内不提供任何进程级实例。

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
    - CR 结构体实现 `ClusterInterface`——只需 `GetSpec` 与 `GetStatus`，其余由内嵌的对象元数据和生成的深拷贝代码提供——并提供 `RoleGroupHandler` 定义产品特定资源。handler 所需的 role/role group 信息全部来自传入的 `RoleGroupBuildContext`，无需实现任何 role 级接口（见 §3.2.2）。
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

- **通用调和器骨架**：`GenericReconciler[CR ClusterResource[CR]]`（`GenericReconcilerConfig[CR]` 与 `NewGenericReconciler[CR]` 同理），约束 CR 类型，复用调和流程。约束用的是 `ClusterResource[CR]` 而非 `ClusterInterface`，因为调和器必须先造出一个空的 CR 实例来读入对象：它通过生成的 `DeepCopy() CR` 拷贝 `GenericReconcilerConfig.Prototype`，拿回的是具体类型，而不是还需断言一次的 `runtime.Object`。
- **通用扩展接口**：`ClusterExtension[CR ClusterInterface]`（以及 `RoleExtension[CR]`、`RoleGroupExtension[CR]`），钩子直接接收产品自身的 CR 类型——为 `*TrinoCluster` 声明的 `PreReconcile` 拿到的就是 `*TrinoCluster`。
- **通用 Webhook 契约**：`ProductDefaulter[CR]` / `ProductValidator[CR]`，与 controller-runtime 的 `admission.Defaulter[T]` / `admission.Validator[T]` 同形，因此带类型的实现可直接传给 webhook builder（见 §4.3）。
- **按 CR 类型实例化的注册表，不做类型擦除**：`ExtensionRegistry[CR ClusterInterface]` 以产品自身的实例化形式存放 `ClusterExtension[CR]` / `RoleExtension[CR]` / `RoleGroupExtension[CR]` 条目，`GenericReconcilerConfig[CR].ExtensionRegistry` 的类型即 `*ExtensionRegistry[CR]`。这个类型参数是实打实起作用的，而非装饰：Go 的泛型类型是不变的（invariant），`ClusterExtension[*TrinoCluster]` 并不满足 `ClusterExtension[ClusterInterface]`——擦除到宽接口的注册表只能装下针对 `ClusterInterface` 编写的扩展，逼着每个钩子在入口处把 CR 转换一次。正是把注册表按产品的 CR 类型实例化，才消掉了这次转换，产品扩展里因此一处类型断言都没有。
- **不存在进程级注册表**：包内既没有包级注册表实例，也没有全局访问函数——包级变量无法携带类型参数，而任何绕开的写法都会把这次重构要消除的类型擦除重新引回来。注册表用 `common.NewExtensionRegistry[CR]()` 构造，且只能经由调和器配置进入框架；这同时意味着同一进程内托管两个产品时，一个产品的钩子不可能作用到另一个产品的集群上——既无法构造（异类扩展通不过类型检查），也不会误连（不存在共享实例）。

### 4.1.3 核心价值

编译时类型检查把运行时类型断言从调和器和产品扩展中一并去掉；新产品只需绑定泛型类型，减少样板代码。

## 4.2 扩展点机制模块

### 4.2.1 设计方法

在调和流程的关键节点预留扩展点，支持在产品侧嵌入自定义逻辑，同时通过注册表统一管理，确保扩展的有序执行。

### 4.2.2 扩展点级别

1. **Cluster 级别**：`PreReconcile`（调和前）、`PostReconcile`（调和后）、`OnReconcileError`（异常时）。
2. **Role 级别**：`PreReconcile`、`PostReconcile`，针对单个 role 执行。
3. **RoleGroup 级别**：`PreReconcile`、`PostReconcile`，针对单个 role group 执行。

### 4.2.3 扩展注册

- **注册表实例**：`common.NewExtensionRegistry[CR]()` 为单个产品 CR 类型创建空注册表；类型实参必须显式写出，因为无参调用无法推导。注册表就是 operator 自己持有的一个普通值——不存在进程级实例，也没有全局访问函数（见 §4.1.2）。
- **注册时机**：扩展在 Operator 初始化期间注册，具体在 Manager 启动之前的 `main.go` 设置阶段，这样第一轮调和开始时它们已全部就位。它们进入的是 operator 自己构造的那个注册表，而不是某个共享实例。
- **接线方式**：注册表只能通过 `GenericReconcilerConfig[CR].ExtensionRegistry`（类型为 `*common.ExtensionRegistry[CR]`）进入框架。**扩展能否运行完全取决于这个字段**：未设置该字段构造出的调和器持有的是空注册表，所有钩子都会静默地成为空操作。管理多个 CR 类型的二进制需为每个类型建一个注册表——把同一个实例共享给两个产品会直接编译报错。
- **注册方法**：`RegisterClusterExtension(ext, opts ...RegistrationOption)`、`RegisterRoleExtension(...)`、`RegisterRoleGroupExtension(...)`。注册接口就这三个：选项是变长参数，因此不再有单独的优先级或选项变体。也没有统一的 `Register()` 方法——注册表为每个级别维护一份有序列表，级别因此写进了方法名。
- **注册选项**：`common.WithPriority(p)` 设置优先级（Lowest=0, Low=25, Normal=50, High=75, Highest=100，默认 Normal）；`common.WithStopOnError(bool)` 针对单次注册覆盖该钩子的默认容错策略（见 §4.2.5）。
- **执行顺序**：扩展按**优先级降序执行（优先级高者先执行）**。相同优先级的扩展按**注册顺序**执行——每个条目携带注册序号，因此顺序是全序的，不依赖排序算法是否稳定。
- **清空**：`Clear()` **原地**清空注册表并重置序号计数器。之所以是清空而非替换：调和器在构造时已经捕获了注册表指针，换一个新实例只会让它继续执行那个陈旧的注册表。测试在用例之间用的就是它，而不是重置全局状态。
- **查询**：`GetClusterExtensions()` / `GetRoleExtensions()` / `GetRoleGroupExtensions()` 按执行顺序返回已注册的扩展；`HasClusterExtensions()` 等同族方法与 `Count()` 报告注册情况。

```go
// main.go，mgr.Start() 之前：先建注册表，再交给调和器。
registry := common.NewExtensionRegistry[*trinov1alpha1.TrinoCluster]()
registry.RegisterClusterExtension(extensions.NewCatalogExtension())
registry.RegisterRoleExtension(extensions.NewHealthExtension())
registry.RegisterClusterExtension(extensions.NewDiscoveryExtension(mgr.GetScheme()),
    common.WithPriority(common.PriorityLow))

reconcilerCfg := &reconciler.GenericReconcilerConfig[*trinov1alpha1.TrinoCluster]{
    // ... client、scheme、recorder、role group handler、prototype ...
    ExtensionRegistry: registry, // 漏掉此字段，任何钩子都不会执行
}
```

### 4.2.4 扩展生命周期

- **初始化**：扩展在 Operator 启动期间实例化一次。SDK 不会每次调和重新创建扩展。
- **状态管理**：扩展应该是无状态的或管理自己的内部状态。SDK 将当前 CR 上下文传递给每个扩展方法，使其能够访问集群状态而无需持久化扩展状态。
- **关闭**：**没有关闭钩子**。扩展接口只声明 `Name`、`PreReconcile`、`PostReconcile` 以及（cluster 级别的）`OnReconcileError`；若某个扩展持有需要在 Operator 关闭时释放的资源，应自行注册 `manager.Runnable`。

### 4.2.5 执行流程

调和器按**优先级降序**遍历注册表条目，由每个钩子的容错策略决定失败是否跳过其后的条目。

- **正常执行**：扩展按顺序执行，每个扩展接收调和上下文、client 和 CR。
- **对 CR 的修改——spec 与 status 并不对称**：
  - **spec：只观察，不就地改。** 框架对 CR 的唯一写入是 `Status().Update`，API server 只把它作用于 status 子资源，因此内存中对 spec 的修改永远不会被持久化。它也未必能被**观察**到：`reconcile()` 在 cluster 级 `PreReconcile` **之前**只取一次 `spec := cr.GetSpec()`，role 遍历、清理与健康评估读的都是这个值——若 `GetSpec()` 每次调用都新建结构体（语法合法但不推荐，见 §5.1.4），它们拿到的就是一份此后任何修改都触及不到的快照。确需修改 spec 的钩子应通过 client 写回，由随之产生的 watch 事件驱动下一轮调和。
  - **status：就地修改即可——框架会持久化。** 钩子通过 `cr.GetStatus()` 返回的指针写入，或直接写产品自有的 status 字段，本轮末尾的 `updateStatus` 会把两者一并提交到 API server。这是刻意设计而非巧合：写入之所以直接从内存对象发出，正是为了让钩子写下的 status 存活（`ClusterInterface` 只暴露内嵌的通用 status，先重新拉取会用存储中的值覆盖产品自有字段，见 §4.13.2）。该保证由回归用例 `persists product-specific status fields written by an extension hook` 覆盖。
  - 两者都不写的钩子——职责纯粹是外部副作用的那种——其失败仍会通过 `Degraded` 条件反映到 CR 上（见下面的错误处理）。
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

删除是**跨多轮调和推进的状态机**，而不是一趟走完。孤儿 role group 上仍跑着 Pod，有状态产品期望它们像自身滚动更新那样退场，因此 cleaner 先把工作负载缩容到 0，等待 StatefulSet 控制器按序号逆序排空，然后才删除；并且每一步都要先确认生效，下一步才会发出。整个过程不阻塞调和 worker：某一步仍在进行中就结束该 role group 本轮的处理并返回重新入队的等待时长，下一轮从第一个尚未落定的步骤继续。每一步都是 Get-then-act，因此重入是幂等的。

### 4.4.2 执行流程

1. 从 Spec 获取 roles 的期望 role group 列表（`desiredGroups`）。本轮调和过的每个 role group 都会记入 `Status.RoleGroups`。
2. 从 `Status.RoleGroups` 获取历史实际 role group 列表（`oldActualGroups`）。
3. 计算孤儿 role groups：`orphanedGroups = oldActualGroups - desiredGroups`。
4. 回收**已从 Spec 中整体消失的 role 的 role 级 PDB**（见下文"被移除的 role"）。它排在 role group 循环之前，且与之无关：当 `orphanedGroups` 为空时循环会提前返回，而某个 role 的各 group 会随着删除完成被逐个从状态快照中裁剪掉——等到它的 PDB 需要重试时，可能已经没有孤儿 group 能把这一趟带起来了。
5. 对每个孤儿 role group——role 按名称排序遍历，使得跨多轮的事件序列可复现——推进一趟删除状态机：先过灰度删除闸门，再按 `PDB → StatefulSet（缩容到 0 → 排空 → 删除）→ ConfigMap → Service → headless Service → metrics Service` 执行，在第一个仍在进行中的步骤处停下。
6. **只有资源确实被删除干净**（本轮所有步骤都已落定）的 role group 才从 `Status.RoleGroups` 中移除。仍处于灰度删除宽限期内、排空尚未结束、以及本轮失败的 role group 都保留在状态快照里，等下一轮调和重试，而不是被悄悄遗忘。裁剪后的映射由调和流程末尾的 status 更新（循环第 7 步）持久化。
7. 返回清理所需的最近一次唤醒时间——尚未走完的灰度删除截止时间，或进行中删除的轮询间隔；没有待办时为 `0`——供调和循环精确地安排重新入队（见 §4.8.4）。

### 4.4.3 安全保护机制

- **删除前验证**：
  - 每个资源在删除前都会先 Get；`NotFound` 视为"已经不存在"并直接按成功短路。
  - 所有权通过 **ownerReferences** 确认：资源必须带有 UID 与 CR 匹配、且 `controller` 为 true 的引用。（owner UID 为空时跳过该检查，供直接驱动 cleaner 的调用方使用。）
  - 不属于本集群的资源**不会被删除**——这可以避免同名的手工资源或外部资源被误删。外部资源计为**已落定**而非待办：本集群永远不会删除它，继续等待只会把该 role group 永久钉在 `Status.RoleGroups` 里。
  - headless（`<resource>-headless`）与 metrics（`<resource>-metrics`）Service 是按**派生名**定位的，而一个 role group 完全可以就叫 `<group>-headless` 或 `<group>-metrics`，它自己的 Service 会与孤儿的派生名撞名，且两者带着同一个 controller owner reference，所有权无法区分。因此，凡是仍被 Spec 声明的 role group 所占用的派生名，一律跳过。

- **删除顺序**——顺序之所以有意义，正是因为每一步都**确认消失**后才发出下一步：
    1. **PDB**（PodDisruptionBudget）——最先删除，以免阻塞随后 Pod 的驱逐。
    2. **StatefulSet**——见下文的有序排空。
    3. **ConfigMap**。
    4. **Service**，然后是 **headless Service** 与 **metrics Service**——Service 放在最后，好让正在终止的 Pod 仍能相互解析。metrics Service 与另外两个一样是框架槽位，因此在此回收，否则它会比所属的 role group 活得更久。

- **StatefulSet 的有序排空**（`deleteStatefulSet`）：直接删除对象会把 Pod 交给级联垃圾回收，其顺序是任意的。取而代之：
    1. 把 `spec.replicas` 置 0（replicas 为 nil 意味着 API server 默认值 1，因此同样是一次缩容）。该写入包在 `retry.RetryOnConflict` 中：同一对象也会被 apply 路径以及任何指向它的 autoscaler 写入，一次常规 409 不该让 role group 停在删了一半的状态。此处的 `NotFound` 表示 StatefulSet 在缩容途中消失了——已无可排空。
    2. 本轮结束并重新入队。StatefulSet 控制器按序号逆序退役 Pod，每个 Pod 遵守自己的 `terminationGracePeriodSeconds`。
    3. 后续各轮在 `.status.replicas > 0` 期间继续等待。在它归零前就删除，等于取消了缩容所要换取的有序停机。
    4. 到此才删除 StatefulSet，并确认删除生效。

- **删除确认**（`confirmDeleted`）：被接受不等于已移除。被 finalizer 挂住的对象在 finalizer 清除前仍会响应 `Get`，带缓存的 client 也会落后于自己的写入。把"`Delete` 返回 nil"当作"已消失"，恰恰会让删除顺序失去意义，因此每次被接受的 `Delete` 之后都会重新读取一次；对象仍在则判为**进行中**，留待后续调和继续。

- **按 role group 隔离错误**：失败被限制在其所属的 role group 内。错误被收集起来，该组保留状态条目与重新入队，**其余各组照常推进**——否则一个卡住的 role group 会让其他所有孤儿无限期存活。收集到的失败合并后返回给调和循环，循环记录日志并继续；清理失败对本轮不致命（例外是 429，见下）。

- **轮询间隔**：进行中的步骤要求调用方等待 `DefaultDrainPollInterval`（5 秒），可用 `RoleGroupCleaner.WithDrainPollInterval` 覆盖（非正值保持默认）。它调度的是状态机的节奏，而非 Pod 终止本身——被安排的那一轮只重新读取它正在等待的资源——因此 `terminationGracePeriodSeconds` 很长的产品可以调大它以减少轮询。

- **被移除的 role**：role *group* 级孤儿靠 diff `Status.RoleGroups` 找出，但从 Spec 中整体删掉的 role 没有任何可 diff 的对象，其 role 级 PDB（仅在该 role 被声明期间才会写入）会遗留下来，选择器匹配的是已不存在的 Pod。这类 PDB 通过 **label `pdb.kubedoop.dev/role`（其值即 role 名）列出**，而不是按派生名查找：产品可以通过 `RoleGroupResources.PodDisruptionBudget` 交付自己的 PDB，它带着同一个 controller owner reference，仅凭所有权无法认出框架的槽位。owner UID 为空时该回收整体禁用——没有 owner 可比对，命名空间内每个带此 label 的 PDB（包括兄弟集群的）都会像是本集群的。

- **灰度删除（可选的宽限期）**：
  - 当 `GenericReconcilerConfig.GrayDeleteGracePeriod > 0` 时，首次发现孤儿 role group 并不会立即删除。cleaner 会在该 role group 的主资源（StatefulSet，回退到 ConfigMap）上打上 `orphan.zncdata.dev/pending-deletion` 注解（RFC3339 时间戳）并推迟删除。
  - 宽限期结束后的某一轮调和才真正执行删除。剩余时间会返回给调和循环并转换为 `RequeueAfter`，因此删除按时发生，而不必等待无关的 watch 事件。
  - 若该 role group 在截止时间前被重新加回 Spec，注解会被清除，下次再次成为孤儿时可重新获得完整宽限期。
  - 属于**其他**集群的主资源永远不会被打注解（撞名时那会去修改一个无关对象），因而也没有时间戳可用来计算宽限期。此时本轮直接放行而不是推迟：每次删除各自都会做所有权检查，外部对象会被跳过，本集群在该名字下真正拥有的资源则被回收。推迟只会让该 role group 在外部对象存在期间一直留在 `Status.RoleGroups` 中。
  - 默认值 `0` 表示不写注解，删除状态机在首次发现时即启动。

- **PVC 处理**：
  - 默认情况下，**PVC 在孤儿资源清理期间被保留**以保护数据。
  - 在集群 CR 上设置注解 `operator.zncdata.dev/delete-pvcs: "true"` 后，cleaner 会同时删除孤儿 StatefulSet 的 PVC（按 StatefulSet 的 Pod 选择器列出，且在缩容到 0 之前执行，此时选择器仍然有意义）。
  - **适用范围**：仅限孤儿清理，即从 Spec 中移除的 role group。SDK 不注册 finalizer，因此删除整个 CR 不会执行任何 SDK 代码：被删除集群的 PVC 交由 Kubernetes 自身的垃圾回收规则处理。

### 4.4.4 并发冲突处理

- **404 Not Found**：视为成功——资源已被其他进程删除。
- **409 Conflict**：打注解与缩容路径都是 Get-then-Update，会携带 `resourceVersion`，并发修改因此表现为冲突。**缩容会在内部重试**，包在 `retry.RetryOnConflict` 中（`scaleToZero` 每次尝试都重新读取活动的 StatefulSet）：apply 路径与 autoscaler 写的是同一个对象，一次常规 409 不该变成失败的一趟、把 role group 留在删了一半的状态。灰度删除的打注解不重试：其冲突被返回，该组本轮结束，由下一轮调和重新评估。
- **429 Too Many Requests**：被映射为携带 `GenericReconcilerConfig.RateLimitRetryAfter`（默认 10 秒；由 `RoleGroupCleaner.WithRateLimitRetryAfter` 设置，产品自行构造的 cleaner 回退到同样的 10 秒）的 `*reconciler.RateLimitError`。与其他清理失败不同，429 会**立即中止整趟清理**——继续处理剩余各组只会给正在拒绝请求的 API server 火上浇油——并作为限流错误（而非清理错误）向上传出调和循环：限流不说明集群状态，因此它只产生一次 `RequeueAfter` 退避，而不会把健康集群标记为 `Degraded`（见 §4.8.4）。这是固定延迟，不是指数退避。
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

- **拆分后的格式契约**：输出是唯一**必需**的契约，解析则是叠加在其上的可选能力。
  - `ConfigMarshaler`（**必需**）——`Marshal(data map[string]string) (string, error)`。`config.NewConfigGenerator`、`MultiFormatConfigGenerator.RegisterFormat` 接收的以及 `config.GetFormat(ConfigFormatType)` 返回的都是它。框架的写路径——各生成器、`BaseRoleGroupHandler` 与 `ConfigMapBuilder`——从不回读已生成的文件，因此产品只需要*写出*的格式，光有 `Marshal` 就是完整的。
  - `ConfigUnmarshaler`（**可选**）——`Unmarshal(data string) (map[string]string, error)`。注册时从不要求它：只输出的适配器和其他适配器一样能注册、能生成。`Parse` 路径在调用时把已注册的适配器向上升级到该接口——这是包内唯一检查动态类型的地方——未实现时返回 `*config.UnsupportedParseError`，其中写明格式（注册用的扩展名加适配器的 Go 类型），调用方知道文件名时还会带上文件名。用 `errors.As` 匹配该错误是稳定的判定方式；而格式为 nil 时返回的是哨兵值 `config.ErrNoFormat`。
  - SDK 自带的每个适配器都同时实现两者，并在 `format.go` 中以编译期断言固化。因此 `GetFormat` 的结果在实践中总是可以拿去解析，尽管其静态类型只承诺 `Marshal`。
- **FormatAdapter**：适配器模式实现，通过 `config.GetFormat(ConfigFormatType)` 选择（`xml`、`properties`、`yaml`、`env`、`ini`；未知类型回退到 properties）。适配器会校验输入并直接报错，而不是输出目标解析器会读错的内容：
  - `XMLAdapter`：将键值对转换为 Hadoop 风格的 `<property><name>...</name><value>...</value></property>` XML 结构。它拒绝 XML 1.0 根本承载不了的文本——制表符/换行/回车之外的 C0 控制字符，以及非 UTF-8 字节——并在报错中点名出问题的键；回车写作 `&#13;`，因为解析器会把内容中的字面行结束符归一化。
  - `PropertiesAdapter`：转换为标准 Java `.properties` 格式，对键中的分隔符、注释符与首尾空白以及值中的续行进行转义。读取时会解码 `\uXXXX` 转义（含代理对），并丢弃未经转义的排版空白，包括续行的缩进。
  - `YAMLAdapter`：通过 `gopkg.in/yaml.v3` 输出扁平映射（会被解析为 bool/数字的值加引号以保持字符串语义）；`Unmarshal` 遇到非扁平映射的文档——以及重复键这种非法 YAML——会直接报错，而不是返回残缺数据。
  - `EnvAdapter`：格式化为 shell 环境变量导出或 .env 文件内容。键必须是合法的 shell 变量名（`^[A-Za-z_][A-Za-z0-9_]*$`），否则报错而不是输出损坏内容。只有当值的每个字符都落在 shell 惰性字符白名单 `[A-Za-z0-9_@%+=:,./-]` 内时才裸写；其余情况——命令分隔符、重定向、子 shell、波浪号、空白——一律用双引号包裹，并转义 `$`、反引号、`\` 与 `"`，因此 `source` 该文件绝不会执行到配置值。值中的换行、回车与制表符写成 dotenv 风格的 `\n`/`\r`/`\t` 转义，因此多行值在 POSIX shell `source` 该文件时并非逐字节保真。读取时，单引号包裹的值按字面量处理，与 POSIX shell 一致。
  - `INIAdapter`：输出 INI 段；键或值含换行、键含 `=`/`:` 或以 `[`、`#`、`;` 开头时报错。
- **产品日志引擎**（`pkg/productlogging`）：独立于上述配置格式适配器的、与产品无关的专用日志引擎。
  - **输入**：深度合并后的 CRD 日志规格（如 `containers.coordinator.loggers.ROOT.level: DEBUG`），一次性转换为框架中立的 `LogConfig`。
  - **生成器**：`LogFileGenerator` 注册表基于中立模型渲染框架特定文件（Logback XML、Log4j2 properties、Python logging）——包含 console/file appender 阈值以及有界滚动文件 appender。
  - **声明**：产品通过 `ContainerLogging`（容器、框架、pattern）声明每容器日志；框架拥有 Vector source 所 glob 的稳定日志文件路径约定——`<LogDir>/<小写容器名>/<容器名>.<框架后缀>`，后缀决定边缘解析器（log4j/logback XMLLayout 为 `.log4j.xml`，log4j2 XMLLayout 为 `.log4j2.xml`，python JSON 行为 `.py.json`）——使生产者与消费者不会漂移。Vector 在边缘解析每种格式，并把事件规范化为稳定 schema（`.timestamp`/`.logger`/`.level`/`.message` + `.errors`，扁平的 `.namespace`/`.cluster`/`.role`/`.roleGroup` 元数据，以及从路径提取的 `.container`/`.file`）。
  - **与 Vector 耦合**：仅当启用 Vector agent 时才生成滚动文件 appender——没有消费者时不存在可写入的共享日志卷（见 Sidecar 注入模块）。
- **集成**：配置生成发生在 **ConfigMap** 路径上，而不是 StatefulSet 构建器里。`BaseRoleGroupHandler.ConfigGenerator`（一个 `config.MultiFormatConfigGenerator`）把 `MergedConfig.ConfigFiles` 渲染成 `map[文件名]内容`，再由 `builder.ConfigMapBuilder.WithMergedConfig(mergedConfig, generator)` 写入角色组 ConfigMap 的 `Data`。未设置生成器时，handler 回退到确定性的 properties 风格渲染（键排序，分隔符与换行转义）。StatefulSet 只负责*挂载*生成出来的 ConfigMap。
- **适配器选择**：`RegisterFormat` 注册的字符串按**文件名后缀**匹配，因此整个文件名（`server.properties`）也是合法的注册项。多个注册项同时命中某个文件名时，**最长者**确定性地胜出——选择不能依赖 Go 的 map 遍历顺序，否则同一个文件在不同调和轮次渲染结果不同，ConfigMap 就会反复抖动。什么都匹配不上的文件回退到 properties 适配器。要经由同一套分发逻辑回读文件，用 `MultiFormatConfigGenerator.Parse(filename, content)`——按文件名解析的受支持方式就是它，而不是去翻适配器表。

### 4.5.3 核心价值

- **统一逻辑**：集中处理文件格式生成的复杂性，避免在每个产品 operator 中重复实现。
- **可扩展性**：实现 `ConfigMarshaler` 接口即可支持新格式——只有一个方法，只有真正会被回读的格式才需要再加 `Unmarshal`。
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
- **工作流程**：`GenericReconciler` 仅在**三道闸门全部通过**时才注册 Vector provider（配置生产者容器名与日志卷大小）。任一闸门不通过都意味着该 sidecar 无法完成本职工作，于是不注册 provider，集群其余部分继续收敛：
    1. **该 role group 启用了 agent**（`logging.enableVectorAgent`，取 role/role group 合并后的结果）。
    2. **handler 的 `LoggingProducers` 至少声明了一个生产者**。无内容可采集的 agent 只会挂上一条空管道；调和器记录这一不一致并跳过，使"启用"与"生产者声明"在同一处保持一致。
    3. **确实有一方提供 `vector.yaml`。** sidecar 运行的是 `vector --config <mount>/vector.yaml`，因此只有当该键确实会被写进角色组 ConfigMap 时才注入：要么 **CR** 实现 `reconciler.VectorAggregatorProvider`（由框架渲染该文件），要么 **role group handler** 实现 `reconciler.VectorConfigProvider` 且 `ProvidesVectorConfig(roleName)` 返回 true（由产品自行写入）。两者皆无时，注册 provider 会导致 sidecar 校验（见本节"依赖校验"）每轮失败，仅仅因为某个产品没有接入 Vector 就中止整个集群的调和。因此它被当作真正的产品配置错误来上报：在 CR 上发出 `Warning`/`VectorSidecarSkipped` 事件，指明 role group 与这两个接口，然后调和继续。

  随后 `BaseRoleGroupHandler` 在 StatefulSet 构建后调用 `SidecarManager` 注入 Containers、Volumes 和 VolumeMounts。第 3 道闸门的第一个分支是框架端到端拥有的那条路径：对于暴露聚合器 ConfigMap 的 CR，调和器解析聚合器地址并把 `vector.yaml` 生成到角色组 ConfigMap 中——使生产者、消费者与配置在同一处保持一致，而非分散在各产品 operator 中。在该分支内部，`VectorAggregatorConfigMapName()` 为空、或地址无法发现，都是硬错误而非跳过：CR 既已声明由框架提供配置，那么交付一个无处投递的 Vector sidecar，还不如大声失败。

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
- **DependencyResolver**：钩子背后的辅助组件。它导出的方法——`ValidateConfigMap`、`ValidateSecret`、`ValidateS3Connection`、`ValidateDatabaseConnection`、`ValidateZKConfig`（`ValidateZKConnection` 是转发到它的过时别名）、`ValidateEndpointFormat`、`ParseConnectionStrings`——也可由产品代码直接调用（例如在 `ClusterExtension.PreReconcile` 中）做比"存在性"更丰富的检查。失败返回 `*DependencyError`，由产品映射到自己的条件上。
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
  2. cleaner 返回的最近一次待办唤醒（§4.4.2 第 7 步）——尚未走完的**灰度删除截止时间**（距下一个孤儿 role group 可删除的剩余时间），或已在进行中的删除的**排空轮询间隔**，取更早者。

  当清理的截止时间早于健康检查节奏时以前者为准，从而保证延迟删除按时执行、多趟排空按自己的时钟推进，而不必等待无关的 watch 事件。若两者都非正（`HealthCheckInterval` 设为负值且没有待处理项），`d` 为 `0`——不做周期性唤醒，完全由 watch 驱动。
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
  - **status 写入上的乐观锁**：status 写入直接使用内存中的 CR，不先重新获取——重取会替换整个 status 段，把扩展钩子在本轮算出的产品自有字段一并丢弃（`ClusterInterface` 只暴露内嵌的通用 status，框架经由 `GetStatus` 返回的指针原地修改它——不存在能整体替换该段的 setter）。遇到 409 时只刷新 `resourceVersion`（配置了未缓存的 `APIReader` 时走它，因为 informer 缓存按定义还没看到那次竞争写入），随后带着本轮的 status 原样重试。这是 last-writer-wins：因为控制器是自身 CR status 的唯一写者，所以它是正确的；代价是**其他**写者在读与写之间写入的 status 字段会被覆盖。`NotFound`（CR 在本轮中途被删除）按成功处理。*cleaner* 对自己那次会被竞争的写入——孤儿 StatefulSet 的缩容到 0——采用同样的 `RetryOnConflict` 处理，见 §4.4.4。
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

- **`ClusterInterface`**：`client.Object` 加两个方法——`GetSpec()` 与 `GetStatus()`。凡是 Kubernetes 对象本就能回答的，都由内嵌的 `client.Object` 提供；SDK 唯一要求产品编写的，是把自己的 spec 与 status 投影到框架通用结构上的那两个方法。
- **`ClusterResource[T ClusterInterface]`**：`ClusterInterface` 加 `DeepCopy() T`。它是*约束*，只用作 `GenericReconciler` 的类型参数，且由 controller-gen 生成的代码满足，无需任何手写实现。
- **`RoleGroupHandler`**：定义产品算子实现的 `BuildResources()` 契约，用于生成 RoleGroup 特定的 Kubernetes 资源。Role 级信息由 `RoleGroupBuildContext` **传入**，而不是通过一个产品必须实现的 role 接口拉取——这是接口隔离的极限：role 这一层对产品的方法开销为零。
- **`RoleExtension` / `RoleGroupExtension`**：定义产品在 role 与 role group 级别定制行为所用的 Pre/PostReconcile 钩子。
- **`ServiceHealthCheck`**：定义用于业务级就绪状态的健康检查契约。

### 5.1.3 优势

- **降低实现成本**：产品开发者只实现他们需要的接口。
- **接口清晰性**：每个接口有单一、明确定义的职责。
- **可测试性**：较小的接口更容易为单元测试进行模拟。

### 5.1.4 示例

```go
// SDK 接口本身：client.Object，加上两个投影方法。
type ClusterInterface interface {
    client.Object

    GetSpec() *v1alpha1.GenericClusterSpec
    GetStatus() *v1alpha1.GenericClusterStatus
}

// GenericReconciler 用作类型参数的约束。
type ClusterResource[T ClusterInterface] interface {
    ClusterInterface

    DeepCopy() T
}

// 产品 CR 实现 ClusterInterface；其余接口都是可选的。
// +kubebuilder:object:root=true
type HdfsCluster struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec              HdfsClusterSpec   `json:"spec,omitempty"`
    Status            HdfsClusterStatus `json:"status,omitempty"`
}

// 内嵌 metav1.TypeMeta 与 metav1.ObjectMeta 提供全部元数据访问方法，
// `make generate` 生成 DeepCopyObject()（补齐 client.Object）与
// DeepCopy() *HdfsCluster（补齐 ClusterResource）。于是 CR 只需写两个方法：
func (h *HdfsCluster) GetSpec() *v1alpha1.GenericClusterSpec { return &h.Spec.GenericClusterSpec }
func (h *HdfsCluster) GetStatus() *v1alpha1.GenericClusterStatus {
    return &h.Status.GenericClusterStatus
}
```

> CR 还必须注册进 manager 的 scheme（`SchemeBuilder.Register(&HdfsCluster{}, &HdfsClusterList{})`）：调和器把取回的对象直接读入 CR 本身，未注册的类型会在 `client.Get` 处以 "no kind is registered for the type" 失败。

> `GetSpec()` 每次调用都新建一个 `GenericClusterSpec` 在语法上合法，但语义微妙：调和循环每轮只快照一次 spec，因此快照之后在内存中所做的修改无法被一致地观察到（见 §4.2.5）。返回指向 CR 内部的指针是更简单的契约。

## 5.2 策略模式

### 5.2.1 模式概述

策略模式定义一系列算法，封装每个算法，并使它们可互换。SDK 广泛使用此模式进行扩展点和可配置行为。

### 5.2.2 SDK 中的应用

- **扩展接口**：产品实现 `ClusterExtension[CR]`、`RoleExtension[CR]` 或 `RoleGroupExtension[CR]` 来注入自定义调和逻辑。
- **ConfigMarshaler 接口**：不同的配置序列化器（XML、Properties、YAML、Env、INI）实现同一个单方法接口。
- **SidecarProvider 接口**：不同的 sidecar 注入器（Vector、JMX Exporter）遵循通用契约。

### 5.2.3 优势

- **灵活性**：策略可以在运行时交换而不修改 SDK 核心。
- **开闭原则**：可以在不修改现有代码的情况下添加新策略。
- **隔离性**：每个策略都是隔离的，更容易测试和维护。

### 5.2.4 示例

```go
// 策略的必需半边：输出即格式的全部契约。
type ConfigMarshaler interface {
    Marshal(data map[string]string) (string, error)
}

// 可选半边，仅在 Parse 路径上通过接口升级发现。
type ConfigUnmarshaler interface {
    Unmarshal(data string) (map[string]string, error)
}

// 具体策略（五个都实现了两个半边）
type XMLAdapter struct{}        // Hadoop XML 格式
type PropertiesAdapter struct{} // Java .properties 格式
type YAMLAdapter struct{}       // YAML 格式
type EnvAdapter struct{}        // shell / .env 格式
type INIAdapter struct{}        // INI 格式

// 上下文使用策略。它只保存必需的半边；Parse 会把该值升级一次，
// 若格式无法回读自己的输出，则返回 *UnsupportedParseError。
type ConfigGenerator struct {
    format ConfigMarshaler
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
│  4. Cleanup Orphans (one pass -> pending wakeup)            │
│  5. Health Check -> Status Conditions                       │
│  6. PostReconcile Extensions (Hook)                         │
│     └── Product-specific post-processing                    │
│  7. Final Status Update (skipped if deep-equal)             │
│  8. Requeue = min(health cadence, cleanup wakeup)           │
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

## 5.4 持有式协作者模式（以组合替代全局状态）

### 5.4.1 模式概述

共享设施以显式构造的值形式存在，由需要它的一方持有，并通过配置交给协作者，而不是放在包级变量里靠全局访问函数取用。所有权体现在类型上，生命周期体现在接线上。

### 5.4.2 SDK 中的应用

- **`ExtensionRegistry[CR]`**：单个产品扩展的注册表。operator 用 `common.NewExtensionRegistry[CR]()` 构造它、把扩展注册进去，再通过 `GenericReconcilerConfig[CR].ExtensionRegistry` 交给恰好一个调和器（§4.2.3）。SDK 自身不持有任何注册表：既无包级实例，也无访问函数。管理多个 CR 类型的二进制为每个类型建一个注册表，而类型参数使跨产品共享同一实例成为编译错误，而非运行时的意外。
- **Scheme**：`runtime.Scheme` 同样在 `main` 中构建一次（实践中就是 manager 的，经 `mgr.GetScheme()` 取得）并显式传递——调和器通过 `GenericReconcilerConfig.Scheme` 接收它。SDK 不声明任何全局 scheme。

### 5.4.3 优势

- **隔离性**：一个产品的钩子不可能作用到另一个产品的集群上，因为不存在两边都能触达的对象。
- **显式接线**：调和器的依赖都写在它的配置里；漏配某一项的故障因此是"钩子不执行"这种就地可诊断的现象，而不是全局状态引发的悬案。
- **可测试性**：测试自建注册表，用例之间既不会互相泄漏注册项，也不需要全局重置；按用例隔离的实例可以安全并行。
- **确定性执行**：扩展按优先级降序执行，相同优先级内以注册序号作为全序的次级比较键。
- **线程安全**：注册表由 `sync.RWMutex` 保护；钩子执行基于条目快照进行。

### 5.4.4 示例

```go
// 注册项被包装成 entry，使优先级、注册序号与单次注册的容错策略随扩展一起流转。
// 注册表按产品自身的 CR 类型实例化，因此条目里放的扩展本就说这门类型。
type extensionEntry[T Extension] struct {
    extension   T
    priority    ExtensionPriority
    seq         uint64 // 注册序号：同优先级下的全序
    stopOnError *bool  // nil 表示使用该钩子的默认值
}

type ExtensionRegistry[CR ClusterInterface] struct {
    clusterExtensions   []extensionEntry[ClusterExtension[CR]]
    roleExtensions      []extensionEntry[RoleExtension[CR]]
    roleGroupExtensions []extensionEntry[RoleGroupExtension[CR]]
    nextSeq             uint64
    mu                  sync.RWMutex
}

func NewExtensionRegistry[CR ClusterInterface]() *ExtensionRegistry[CR]
```

扩展直接声明自己操作的 CR 类型并直接注册——整条路径上既没有适配器，也没有类型断言：

```go
// func (e *SafeModeExtension) PreReconcile(
//     ctx context.Context, c client.Client, cr *HdfsCluster) error
var _ common.ClusterExtension[*HdfsCluster] = &SafeModeExtension{}

registry := common.NewExtensionRegistry[*HdfsCluster]()
registry.RegisterClusterExtension(&SafeModeExtension{}, common.WithPriority(common.PriorityHigh))
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

- **配置格式适配器**：将内部配置映射转换为各种外部格式：
  - `XMLAdapter`：适配为 Hadoop XML 格式
  - `PropertiesAdapter`：适配为 Java .properties 格式
  - `YAMLAdapter`：适配为 YAML 格式
  - `EnvAdapter`：适配为环境变量格式
  - `INIAdapter`：适配为 INI 格式

### 5.6.3 优势

- **格式独立性**：SDK 核心使用内部映射表示。
- **可扩展性**：实现 `ConfigMarshaler` 即可添加新格式；只有需要回读的格式才补上 `ConfigUnmarshaler`。
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

| 模式 | 主要应用 | 关键收益 |
|---------|---------------------|-------------|
| 接口隔离 | `ClusterInterface`（`client.Object` + 2 个方法）、`RoleGroupHandler` | 聚焦、可落地的契约 |
| 策略 | 扩展接口、`ConfigMarshaler` | 行为可替换 |
| 模板方法 | 调和流程 | 流程统一且预留钩子 |
| 持有式协作者 | `ExtensionRegistry[CR]`、Scheme | 显式接线，无全局状态 |
| 构建者 | StatefulSetBuilder | 复杂对象的分步构建 |
| 适配器 | 配置格式适配器 | 格式互操作 |
| 观察者 | 事件记录 | 变更通知 |

# 6. 关键问题与解决方案

- **类型断言导致的运行时错误和代码冗余**
  - **解决方案**：为调和器、扩展接口、扩展注册表与 Webhook 契约引入 Go Generics，使产品钩子直接拿到自己的 CR 类型，路径上不再有任何适配器或断言。
  - **核心优势**：编译时类型安全，减少样板代码，提高开发效率。

- **删除 role group 后残留孤儿资源**
  - **解决方案**：对比 Spec 与 Status 快照，通过 ownerReferences 校验归属，再用跨多轮的状态机退役孤儿资源——缩容到 0、有序排空，然后按固定顺序删除，且每一步确认消失后才发起下一步；可选的灰度删除宽限期会推迟整个序列，调和循环则为待办项安排重新入队。
  - **核心优势**：高效精确，避免误删与 Pod 粗暴终止，确保状态收敛。

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

1. **定义 CRD 结构体**：内嵌 `metav1.TypeMeta` 与 `metav1.ObjectMeta`，为类型加上 `+kubebuilder:object:root=true` 标记，并在产品自身的 Spec/Status 中嵌入 SDK Generic Spec/Status 模型。
2. **注册进 scheme**：`SchemeBuilder.Register(&YourCluster{}, &YourClusterList{})`——调和器把取回的对象直接读入 CR 本身，未注册的类型会在 `client.Get` 处失败。
3. **执行 `make generate`**：controller-gen 生成 `DeepCopyObject()`（补齐 `client.Object`）与 `DeepCopy() *YourCluster`（补齐 `ClusterResource[*YourCluster]`），两者都不需要手写。
4. **编写 `ClusterInterface` 的两个方法**：`GetSpec() *v1alpha1.GenericClusterSpec` 与 `GetStatus() *v1alpha1.GenericClusterStatus`（见 §5.1.4）。集群级契约到此为止。
5. **实现 `RoleGroupHandler[*YourCluster]`**——通常通过内嵌 `BaseRoleGroupHandler`——描述一个 role group 的 Kubernetes 资源。
6. **接线 `GenericReconciler`**：用 `GenericReconcilerConfig[*YourCluster]` 至少设置 `Client`、`Scheme`、`Recorder`、`RoleGroupHandler` 与 `Prototype`（`&YourCluster{}`），再加上产品需要的可选钩子（`APIReader`、`ProductConfig`、`Dependencies`、`ServiceHealthCheck`、灰度删除与健康检查间隔等），最后调用 `SetupWithManager`。
7. *（可选）* **添加扩展**：实现 `ClusterExtension[*YourCluster]` / `RoleExtension[*YourCluster]` / `RoleGroupExtension[*YourCluster]`（钩子签名中直接写具体 CR 类型），在 `main.go` 中于 Manager 启动前用 `common.NewExtensionRegistry[*YourCluster]()` 建好注册表，用 `RegisterClusterExtension`/`RegisterRoleExtension`/`RegisterRoleGroupExtension` 注册（顺序或容错策略有要求时配合 `common.WithPriority` / `common.WithStopOnError`），并**设置 `GenericReconcilerConfig.ExtensionRegistry`**——漏掉该字段，钩子永远不会执行。
8. *（可选）* 实现 `ProductDefaulter`/`ProductValidator` 接口以自定义 Webhook 逻辑。
9. *（可选）* 增加产品自有配置格式：实现 `config.ConfigMarshaler` 的适配器，注册到 handler 的 `MultiFormatConfigGenerator` 上。
10. 通过 Kubebuilder 生成 Webhook 和 CRD 配置，部署验证。

# 8. 总结与展望

## 8.1 核心优势总结

通过分层架构、接口驱动设计、泛型转换和扩展点机制，本 SDK 实现了多集群产品的通用逻辑复用和灵活扩展。同时解决了孤儿资源、术语冲突和类型安全等关键问题，符合 K8s 生态系统标准，适应生产级 Operator 开发需求。

## 8.2 未来优化方向

以下内容**尚未实现**，描述的是演进方向而非当前行为：

- 支持 **ConversionWebhook** 实现平滑的 CRD 版本升级。
- 添加监控指标统计扩展执行时间、资源清理次数等，便于故障排除。
- 对标 `pkg/s3` 的 `pkg/database` 解析器（构造 JDBC URL 并提供凭据卷）。
- 可选的 finalizer 支持，使集群删除（而不仅是 role group 变成孤儿）也能执行 SDK 的清理逻辑，例如删除 PVC。
