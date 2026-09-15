# Operator-Go Architecture

## Applicability and design authority

This document is the design authority for both implemented API families, with explicit scope:

- [Product-description framework](#framework-design): the public `pkg/framework` contracts used by the Trino reference.
- [Existing GenericReconciler SDK](#existing-generic-reconciler-sdk): the numbered English sections retained for the existing API.

Use the applicable section; GenericReconciler hooks, admission defaults and merge semantics do not add requirements to the product-description framework.
[Security](security.md), the [Trino developer guide](../examples/trino-operator/README.md) and the
[delivery guide](../hack/framework-e2e/README.md) cover their respective responsibilities.
Discussion notes, iteration plans and runtime reports are Git-ignored local records, not another source of design authority.

<a id="framework-design"></a>

## Product-description framework

产品声明业务输入和运行方式；框架负责继承、覆盖、资源装配和持续收敛。产品作者不编排这些阶段。
以下规范适用于正式的 `pkg/framework` API，不是 SDK 版本或镜像发布声明。

领域细节：[存储](#framework-storage) · [平台依赖](#framework-platform) ·
[认证](security.md#framework-authentication) · [S3](#framework-s3) · [日志](#framework-logging) ·
[生命周期](#framework-lifecycle) · [数据操作](#framework-data-operations)。

### 1. 能力与支持边界

| 领域 | 实现范围与边界 |
| --- | --- |
| CR 输入 | 标准 image、独立 clusterConfig、显式角色与角色组、公共/产品 config、roleConfig/PDB、四通道 overrides；生成存在性输入与 CRD |
| 产品运行描述 | 一个主进程、有序初始化、原生 lifecycle/probe、文件、目录访问、端点和日志产出；物化器与可选 Vector；不提供任意 sidecar/资源注册表 |
| 文件 | KeyValues、Lines、Text；明确覆盖动作和受限运行期属性绑定；不按扩展名猜编码器 |
| 资源 | 每组 ConfigMap、StatefulSet、普通/Headless Service；每角色 PDB；显式共享 ConfigMap及同源撤回 |
| 外部事实与平台结果 | 精确只读 Get、按组结果、单轮引用快照和有界刷新；创建前来源解析与创建后 CSI/Listener 观察分离 |
| 日志 | enableVectorAgent、原生适配和实际文件声明；Vector 到 stdout JSON 或集中目的地；产品明确原生格式支持范围 |
| 保留存储 | [标准存储](architecture.md#framework-storage)绑定一个 Data 目录；RWO/Filesystem、Retain/Retain；[数据操作](architecture.md#framework-data-operations)独立授权 |
| 协调控制 | 暂停、停止、恢复、组退休、固定资源所有权、条件状态；逐 ordinal 缩容、停止优先级和持久进展预算；不承诺通用事务一致性 |
| 开发与交付 | 生成输入/schema/注册、参考 operator、物化 helper、独立数据 executor、CRD/RBAC/部署和可复用验收工具 |

具体产品只接收有实际消费者的配置。旧 SDK 的能力不自动成为新框架的支持范围。
运行/API 测试分别验证对应层次；发布制品必须满足[交付指南](../hack/framework-e2e/README.md)的检查要求。

### 2. 产品作者的三个对象

| 对象 | 构造者 | 内容和责任 |
| --- | --- | --- |
| `ProductDefinition[C,S,F]` | 产品 | 角色及默认配置、镜像默认、输入校验、组运行描述生成、可选共享 ConfigMap 生成和最终业务关系检查 |
| `EffectiveInput[C,S,F]` | 框架 | 当前组身份、完整有效配置、产品集群配置、标准 Platform 输入、有效镜像、该组事实和完整声明拓扑 |
| `RuntimeDescription` | 产品生成函数 | 主进程与有序初始化、原生生命周期/探针、协调策略、文件、目录访问、端点、日志和 Data/Secret/Listener 目录声明 |

`C` 是组级产品配置，`S` 是集群级产品配置，`F` 是产品使用的事实数据；没有相应内容时用 `struct{}`。
有效值不携带“用户是否填写”的指针。公共字段与产品字段在 Go 中分别处于 `Config.Common` 和 `Config.Product`，
在 CR 的 `config` 中仍是平铺字段；生成时拒绝字段重名。

定义按进程注册，CR 数据按协调轮次读取。默认数据、基础事实、平台选项在注册时复制；
每个回调取得隔离的数据快照。框架不承诺复制函数闭包捕获的可变状态，产品必须避免跨 CR 的隐式状态。
生成和校验函数不得访问集群或写外部状态；需要读取的依赖由只读事实适配器提供。

普通产品通过函数字段提供语义，不需要继承基类、实现空钩子或构造 reconciler。
可选 `GenerateCluster` 首批只生成共享 ConfigMap，并显式报告 Ready 或 Pending（第 6.1 节），
不扩为任意资源或事后修改 Pod 的入口。共享输出结果是辅助数据类型，不增加一个产品生命周期阶段。
`ValidateFinal` 只返回检查结果；检查之后不存在能修改最终产物的产品钩子。

### 3. 输入领域与继承

#### 3.1 各领域拥有自己的作用域

| 输入 | 折叠来源，左低右高 | 最终消费位置 |
| --- | --- | --- |
| 组 `config` | 角色完整默认 → CR role.config → roleGroup.config | 公共装配和产品生成，共用一次确定的有效值 |
| 产品 `clusterConfig` | ClusterConfigDefaults → CR clusterConfig 中的产品字段 | 集群、事实和组生成；不向 role.config 隐式继承 |
| image | 产品镜像默认 → CR image，按镜像领域解析一次 | 有效镜像、主进程默认、pull policy/secrets |
| roleConfig | RoleDefinition 中的角色管理默认 → CR role.roleConfig | 角色资源；组不能声明此入口 |
| replicas | 框架默认 1 → role.replicas → roleGroup.replicas | 声明拓扑；停止时另外计算执行副本 |
| stopped / reconciliationPaused | clusterConfig 中的固定运行控制；省略为 false | 执行控制；不接受产品默认，不参与产品 S 的折叠 |

组生成失败仍保留该组的声明身份；零组角色仍是一个角色。配置、事实和构建错误都不能把期望资源误判为退休对象。
资源名、选择器和供产品生成地址使用的名称必须共用同一确定性命名规则。
首批要求在构建写入前拒绝非法组合名称及碰撞；不要求通过静默截断接受所有合法单段名称的组合。

#### 3.2 普通配置的固定规则

普通产品对象按字段递归，字符串键 map 按键递归；带点号的键是字面键，不解释为路径。
序列整体替换，不提供按字段选择 Append、Atomic 或自定义合并策略的接口。

| 更高层输入 | 语义 |
| --- | --- |
| 未填写 | 继承 |
| `false`、`0`、`""` | 明确的值，覆盖低层 |
| 对象或 map 的 `{}` | 没有新字段/键，继承；不是清空动作 |
| 序列 `[]` | 明确替换为空 |
| 普通配置中的 `null` | 本地解码拒绝；不是一套隐式删除语法 |

先检查各输入层的结构、类型和歧义，再折叠，最后检查有效业务值。
低层某个业务值无效但被高层修正，不应在折叠前阻断；无效类型和歧义则不能依靠覆盖掩盖。
CRD 不写入继承默认，确保持久化不会把“未填写”变成“用户显式填写”。

原生领域单独定义语义：Affinity 的 node/pod/podAnti 三个分支分别处理，省略分支继承，
出现分支则整体替换，`nodeAffinity: {}` 清空该分支。Quantity 按数量值处理，Duration 是字符串，
最终 gracefulShutdownTimeout 必须是非负整秒，0 有效。CPU 有 min/max，内存 limit 同时成为 request 和 limit。
这些是固定领域规则，不是允许产品注册任意反射策略的先例。

#### 3.3 可生成的 Go 类型与 admission 边界

首批 C/S 支持导出字段的普通结构体、标量、字符串键 map、slice，以及固定支持的 Quantity/Duration。
不支持嵌入、指针、interface、`[]byte`、自定义 JSON/Text 编解码等任意 Go 模型。
生成注册包引用的顶层 C/S 必须是可跨包引用的命名结构体或 `struct{}`；非空匿名结构体、未导出类型和
暂不支持的泛型实例必须报错。生成器检查不替代生成包的真实编译。

presence 输入、CRD schema、投影和注册代码从同一描述生成，不手工维护多份业务字段清单。
首批保留已验证的 schema 容量约束：普通配置集合上限 32，原生 Affinity 集合上限 16；
这不是 status 列表上限，也不保证任意深度/嵌套 schema 都能安装。扩大容量必须有 admission 成本和真实安装证据。

本地严格解码拒绝重复键、未知字段和普通配置 null。API server 可能裁剪未知字段，
因此“本地拒绝”不能冒充“所有 API 请求都会拒绝”；交付示例用严格字段校验，并测试真实持久化结果。
JSON Merge Patch 的 null 可以删除已存储输入而恢复继承，与配置对象显式含 null 的含义不同。

### 4. 从有效输入到最终资源

#### 4.1 框架拥有固定执行顺序

1. 投影完整身份清单，折叠公共与产品输入，解析镜像，准备声明拓扑。
2. 通过事实适配器取得组所需的外部值；只有有效且已就绪的组进入产品生成。
3. 产品为有效组生成一次运行描述；框架根据实际 LogOutputs 解析集中日志目的地，并检查目录、产出归属、文件、端点和进程声明。
4. 组合平台能力，处理文件/env/CLI 的角色层和组层覆盖，装配工作负载及配套资源。
5. 对完整 PodTemplate 先应用 role.podOverrides，再应用 roleGroup.podOverrides。
6. 独立检查最终产物之间的已知关系，形成可执行计划或带原因的失败；控制器负责应用。
7. 应用后独立观察平台生产者及 CSI/Listener 结果，通过 RefreshClusterOutput 刷新共享输出；结果 Pending 保留旧输出。

Source、Prepared、Plan 是内部工作结构，不是要求产品作者依次调用的公共阶段。
最终 Pod 改变不会反向重算有效配置、重新派生文件或触发一次隐藏的产品生成。

#### 4.2 四个覆盖通道

| 通道 | 目标和行为 |
| --- | --- |
| configOverrides | 产品显式 ConfigDirectory 下的相对文件路径；按角色、组依次执行文件动作 |
| envOverrides | 主进程 env，按名称覆盖；空字符串是值，不是删除 |
| cliOverrides | 主进程 Args 整体替换，保持 Command；`[]` 清空参数 |
| podOverrides | 完整 PodTemplate 的原生 Strategic Merge Patch；平台装配及其他覆盖全部完成后执行 |

这里的优先级按通道顺序确定：**role.podOverrides 也高于 roleGroup.envOverrides/cliOverrides**。
Pod 补丁中的 null、`$patch`、按 mountPath 等原生合并键保留其补丁语义，不套用普通配置规则。
“最高优先级”意味着最终值生效，不意味着能绕过最终资源和已知消费关系的检查。
可确定的冲突使该组不能应用；无法静态判断的产品消费关系必须保留 Unknown，不擅自修复用户输入。

文件有三种内容类型：`KeyValues{Codec,Values}`、`Lines`、`Text`。
覆盖动作是 `properties.set/remove`、`properties.replace`、`lines`、`text`、`remove: true`；
replace 与 set/remove 互斥，同一键不能同时 set/remove，每个文件一次只选一种内容/删除动作。
空内容与删除文件不同；后续空 patch 不会使已删除文件复活。
properties 操作依赖产品原始结构化编码声明，不能从 `.xml` 或 `.properties` 后缀推断。
覆盖语法由[输入契约](../pkg/framework/input/contract.go)定义，例如：

```yaml
configOverrides:
  config.properties:
    properties:
      set:
        query.max-memory: "3GB"
      remove:
        - an.optional.property
  jvm.config:
    lines:
      - "-Xmx1024m"
  custom.conf:
    text: ""
  catalog/unused.properties:
    remove: true
```

`properties.replace: {}`、`lines: []` 和 `text: ""` 分别产生对应类型的空文件；`remove: true` 删除文件。

#### 4.3 文件物化与运行期绑定

文件标识由目录与相对路径组成，ConfigMap 存储键是交付细节。框架验证越界、冲突、重复生产和目录访问，
通过版本化计划及 init helper 写入文件。主进程身份、共享组和 helper 身份显式提供，不根据镜像猜测权限。

属性值是 Literal 或明确的运行期绑定，不能同时存在；首批运行期绑定是 PodName。
覆盖触及键/文件时取消对应旧绑定，运行期值作为编码数据写入，不拼进 shell 命令。
纯生成时 PropertyCodec 可以由产品实现；跨进程物化只支持交付 helper 明确实现的 codec，
首批为 PropertiesCodec，不承诺任意 Go 编码器能在 Pod 中执行。

ConfigMap 更新不自动证明进程重载。文件配置送达由平台 restarter 合约完成，部署者通过 CR 标签选择启用；
框架传递 workload 标签并保留 restarter 的 PodTemplate 注解，不自行写 restarter stamp。
env/CLI/Pod 覆盖改变模板后由 StatefulSet 控制器滚动。两种路径必须分别验收实际进程消费。

#### 4.4 日志是产品语法与平台采集的协作

产品从有效 logging 配置生成自己的原生格式，并声明确实会产生的日志文件。
无法表达的阈值应返回明确错误，不能静默改成最接近的值。每个产品须明确其原生日志支持范围；
Trino 的 sink 和级别限制见[示例说明](../examples/trino-operator/README.md#native-trino-logging-and-process-identity)。

框架统一消费有效 `enableVectorAgent`：开启时采集已声明的日志文件，关闭时不装配采集器。
产品只声明实际日志产出，LogOutput 不携带采集开关；该开关由框架统一消费。
未指定集中目的地时，Vector 输出到 stdout JSON。当前 `clusterConfig.vectorAgentConfigMap` 已由框架
通过精确只读 Get 解析同 namespace ConfigMap 的 ADDRESS；仅开启采集且有实际文件时读取该依赖。
解析后的地址进入原生 Vector sink 和 Pod 模板，引用缺失为 Pending，地址更新触发模板收敛。
最终文件、挂载和 Vector 路径仍独立检查，产品关系 Unknown 不得屏蔽确定的采集冲突。

### 5. 外部事实、来源与诊断

Facts resolver 位于产品适配包，使用框架提供的只读 `FactsReader.Get`，不得获得写客户端。
它收到有效组配置、集群配置、基础 F 和声明拓扑，返回 resolved、pending、invalid 或 readError。
未解析的组不生成/应用新计划；无关组、角色资源和明确退休仍可推进。
共享输出能看到每组结果，但“已生成端点”不能当作“已经可访问”。

框架按完整 GVK/namespace/name 缓存本轮 Get，记录 UID/resourceVersion，保证本轮同引用的一致观察；
记录不包含外部对象内容。UID/RV 表示读取来源，不表示文件已送达或业务已加载。
注册基础 F 与各组解析 F 分离，禁止把某个 CR 的事实存到进程级定义中。

使用定时刷新，不承诺动态依赖 watch。默认周期为 30 秒，包括没有产品 resolver 的配置；
pending 最早按 `min(刷新间隔, 2秒)` 重试，错误退避上限不得阻断该刷新。
自定义间隔为正数，零使用默认。这些是调度上限，不是 API 阻塞、队列积压下的 wall-clock SLA。

最终关系检查 `Check` 固定三态：consistent、conflict、unknown。consistent 证明已知结构关系；
conflict 阻止对应组应用；unknown 保留诊断且不隐式改写结果，不代表业务健康。
status 的条件类型、三态值和事实状态是机器契约；Subject/Reason/Message 供定位，未声明为枚举的文本不得用作稳定解析接口。
诊断应带组/角色身份、输入通道或检查对象；不把外部 Secret 内容、整个 facts 或完整有效配置复制到 status。
首批不承诺逐字段来源图或完整 explain/dry-run 公共 API。

### 6. 控制器拥有执行责任

#### 6.1 资源与所有权

框架自行构造直接 API 客户端，从当前观察执行判断、冲突重试和写入；manager 的缓存用于 watch 调度，
不能替代修改意图、所有权和保留卷的即时读取。多资源操作不是事务，已发出的请求无法取消。

固定资源槽由 CR owner UID、规范化槽位记录、名称及框架管理元数据共同识别，不能仅凭 label 接管或删除对象。
apply 前检查身份、来源、不可变字段和存储约束；不可实现的声明报错，不能保留旧值后报告成功。
更新保留 API 分配值与外部控制器管理的元数据，框架自己的字段按期望收敛。
使用 server dry-run 规范化默认值后比较，稳态不发持久化的无变化更新；这不意味着零 API 请求。

角色 PDB 独立于组构建成功与否，产品显式选择启用；按全部声明副本计算
`minAvailable = max(0, sum(replicas) - maxUnavailable)`，覆盖该角色全部组。
停止或 facts pending 不减少声明预算；PDB 不提供缩容、直接删除或业务排空保证。

共享 ConfigMap 也需要可识别的完整期望集合与同源撤回清理，不能只 apply 新输出而永久遗留旧输出。
GenerateCluster 必须返回有明确状态的共享输出结果：Ready 携带完整期望集合，空集合表示撤回全部；
Pending 携带原因并保留旧输出，不得同时提交部分输出；未声明状态无效。返回 error 同样保留旧输出并报告失败。
没有注册可选 GenerateCluster 回调表示完整空集合，可回收此前可信的共享槽位。
共享输出的状态必须显式表达，不能用 nil 切片同时表示等待与撤回。

#### 6.2 暂停、停止与恢复

暂停在完整投影、配置校验、facts 和资源读写前判断，仅允许报告 Paused 与当前顶层观察代次；
此前执行条件及组/角色观察保持原代次。稳定暂停不写 status、不轮询，不表示 Kubernetes GC 或其他控制器暂停。

停止独立扫描可信的 live 固定槽位，使已有 StatefulSet 降为 0，即使当前 image/产品配置/facts 错误。
声明 replicas、拓扑和 PDB 不变，计划另持有执行副本 0；恢复使用最新 CR 声明。
停止本身不删除资源，明确移除的组仍走退休。配置失败组没有新的执行计划，不能伪造已应用状态。

Stopped 需要当前 StatefulSet 代次已观察、status.replicas/readyReplicas/updatedReplicas 为零、
实际 Pod 消失并复读 StatefulSet 的 UID/resourceVersion；
清单不完整则 Unknown，不能以空列表得出全停成功。Stopped 与 Applied 独立；停止期间 WorkloadsReady 不声称业务就绪。
每轮发请求及冲突重试前重查 CR UID、generation、deletion 和运行控制；已发请求与检查后的竞争窗口不作原子保证。
首批守卫不把任意 metadata-only 更新当作撤销整轮的事务屏障，元数据在后续协调收敛。

#### 6.3 退休、删除和保留数据

组退出期望清单后，控制器从 live 来源重建退休工作，不依赖 status 账本或进程内历史。
按缩容到 0、确认控制器和实际 Pod 排空、逐项删除并确认固定槽位消失推进；不强杀 Pod 或移除 finalizer。
排空中重加按最新声明恢复；旧对象已 Terminating 时等待删除完成再创建。
CR 删除依赖 owner-reference GC；首批不提供产品 finalizer 清理协议，不能把组退休保证扩展到 CR 删除。

保留数据只支持一个明确的 RWO/Filesystem 槽位，StatefulSet 的 whenDeleted/whenScaled 均为 Retain，
StorageClass 及实际 PV 的 reclaimPolicy 也必须为 Retain。
在 apply/重试/停止/退休前核对 StorageClass、claim、PV 绑定、来源记录和实际消费者，包括缩容后的高序号 claim。
合法首次创建及未绑定 PVC 可以先创建消费者，以支持 WaitForFirstConsumer；实际 PV 出现后再核验绑定与策略，
不能把已有 PV 验证成功作为首次创建 Pod 的前提。
产品协调器不删除 PVC/PV；同 CR UID 重加只能复用仍存活、可验证的原卷。
E05 的独立 DataAsset 自动记录已确认绑定，在原 claim 丢失时拒绝当作首次创建。
跨 CR 接管、容量/类迁移和销毁通过独立 DataOperation 与独立 executor 执行；
批准内容绑定实际数据和集群 UID，每个阶段复核来源、暂停与真实消费者排空。
迁移必须实际复制并校验，销毁必须实际清空并等待后端 provisioner 回收，不能只删除 Retain PV 对象。
数据身份、旧副本、授权、恢复阶段与历史的完整契约见 [数据操作协议](architecture.md#framework-data-operations)。
这些文件系统操作不提供产品级一致性或备份恢复保证。

#### 6.4 状态与可验证程度

条件包括 Built、Applied、WorkloadsReady、PlatformReady、RoleResourcesApplied、Retired、Paused、Stopped；
组状态区分 declared desiredReplicas、可选 executionReplicas、readyReplicas、检查和事实观察。
条件观察代次必须与产生它的执行轮次对应，暂停时不得把旧成功条件重新盖成当前代次。
完整可信身份清单取得后，组级配置、facts、构建和应用失败不能阻断其他组；
身份非法、名称碰撞或完整清单不可取得属于整体输入失败，可以阻断依赖该清单的构建和回收。
状态写入也需避免无变化循环。
WorkloadsReady 描述工作负载观察，不替代产品服务健康或查询成功；Applied 也只证明资源应用阶段。

### 7. Go 包与生成代码的边界

使用现有 Go module，不增加 `/v2` 或独立 runtime module。公开与内部包按下表划分职责。

| 包 | 公开程度 | 拥有的责任 |
| --- | --- | --- |
| `pkg/framework` | 产品作者使用 | 三个对象、Config/运行描述领域值、FactsReader/FactInput/FactResult、Check/status 数据、平台装配选项 |
| `pkg/framework/input` | 生成代码契约 | presence 基础类型、原始 CR Projection、类型明确的绑定以及严格解码/复制辅助；不包含有效配置或资源计划 |
| `pkg/framework/operator` | 部署入口 | Options 和供生成 registration 调用的注册函数；隐藏可变 Reconciler |
| `pkg/framework/inputgen` | 开发工具库 | 输入/schema/投影/注册生成；工具依赖不进入运行期输入包 |
| `internal/framework/pipeline` | SDK 内部 | Source/Prepared/Plan、折叠、覆盖、平台组合、物化计划、装配、最终检查 |
| `internal/framework/controller` | SDK 内部 | API/facts 观察、apply/status、运行控制、退休、保留卷检查 |

`framework` 是叶级领域契约，可引用 Kubernetes 数据类型与必要的只读接口，不依赖本表其他包或产品代码。
`input` 只向 `framework` 依赖；pipeline 使用二者；controller 使用它们及 pipeline；operator 连接到 controller。
inputgen 依赖领域与输入描述，不依赖 controller。`internal` 放在 module 根下，使仓库工具能合法使用内部构建/物化能力，
无需为了 render 或 helper 人为开放 Plan 公共 API。

产品生成的 API 包依赖 framework/input 和状态数据，不依赖 operator/controller。
其独立 registration 子包引用生成 API、原始 C/S 产品类型和 framework/operator，固定 C/S，只留下 F 泛型。
产品定义包不能反向导入生成 API/registration；产品事实适配器也不能放入通用 controller 包。

外部 operator 的生成代码无法导入本 module 的 internal，因此必须有薄的公开生成代码契约。
Projection 只携带原始 image、clusterConfig、角色/组输入及完整身份清单，不含 F、有效配置或计划。
正式投影保留嵌套 Roles/Groups，每层持有自己的配置/overrides 和可选 replicas，不提前折叠副本或复制角色层到每组。
controller 将注册的基础 F 与 Projection 组合为内部 Source；公共 API 不暴露 SourceSnapshot 或构建阶段编排。
固定运行控制通过独立 `Operation(cr)` 读取，不能为了统一投影而重新让暂停依赖完整 Snapshot 成功。
生成绑定还提供对象构造、scheme 注册和 status 访问；任何需要外部生成包实现的函数/类型都必须可公开引用，
不能使用含内部返回类型或不可从外部实现的私有方法来“隐藏”它。

生成代码契约可导入，但不是普通产品手工接线接口。它和 inputgen 一起交付，具有显式的生成契约版本；
生成代码记录该版本，生成检查和注册校验拒绝不兼容版本。不同 SDK 发布版本可以共享兼容的生成契约版本，
不能声称仅凭编译通过就证明 SDK 版本完全相同。无公共合并引擎、阶段注册表或事后资源修改钩子。

### 8. 注册与交付

普通入口是生成的 `registration.Register(manager, definition, options)`，只返回 error。
Options 包含基础 F、可选只读 resolver、AssemblyOptions 和刷新间隔；初始 AssemblyOptions 仅声明
物化/Vector 镜像及 helper 身份，不构建尚无消费者的通用能力插件体系。

注册验证配置类型、完整角色清单和绑定，复制部署数据，加入生成 API scheme，并注册使用直接客户端的控制器。
注册不执行产品生成或业务值校验，不读取 CR，不安装 CRD/RBAC，不启动 manager，也不是热更新接口。
manager 配置和启动、CRD/RBAC/镜像部署属于 operator 的组合入口及交付清单。

正式 SDK 不得导入本地实验原型或具体产品代码；新框架的执行路径不得借旧 GenericReconciler
和 hook 机制恢复另一套配置语义。发布路径不得依赖本地过程记录。
参考 operator 必须真实使用新包和生成注册，不以在示例旁新增未被启动的演示代码算作落地。
helper 计划版本、镜像和 SDK 的兼容关系、生成一致性、RBAC 及部署说明一起交付。

<a id="framework-storage"></a>

## 标准数据存储

标准输入是 `config.resources.storage`，与 CPU、内存并列。产品运行描述用 `Directory.Data: true`
标记一个数据目录，并通过 `Main.Access` 声明产品实际使用的路径。产品无需重新构造 StorageClass、容量或 PVC。
未声明 Data、Secret 或 Listener 来源的目录为临时目录。配置文件和日志必须使用独立临时目录，数据目录必须有主进程可写访问。

```yaml
workers:
  config:
    resources:
      storage:
        type: persistent
        storageClassName: retained
        capacity: 32Gi
  roleGroups:
    default:
      config:
        resources:
          storage:
            capacity: 64Gi
```

- 类型是 `ephemeral` 或 `persistent`；产品零值默认解析为 ephemeral，API 不填充默认值。
- 默认值 → 角色 → 角色组。省略类型或保持同一类型时按字段继承；空对象继承。
- 显式改变类型时先清除低层存储分支，再应用高层字段。切到 ephemeral 不携带旧 class/capacity。
- 每层检查字段形状与互斥关系；最终持久类型必须有有效的显式 StorageClass 和正容量。
  用户层 `type: ephemeral` 不能同时指定 class/capacity；null 不是删除或继承动作。
- persistent 必须有 `Data` 消费者，不能接受输入后不生成卷。该单元支持一个 RWO/Filesystem 数据槽。
- persistent 装配为 claim template，scale/delete 均 Retain。控制器继续要求实际 StorageClass 显式 Retain，
  验证来源、绑定、现有消费者；最终 podOverrides 不能挪走或遮蔽已声明的保留数据挂载。
- **继承中的分支切换不迁移已有数据。** 现有 StatefulSet 的存储声明改变会报错；组删除后还有保留卷时，
  改成 ephemeral 也会被来源检查拒绝。跨身份迁移和销毁必须使用独立授权的 [DataOperation 流程](architecture.md#framework-data-operations)。

产品通过 Data 标记声明数据目录。卷身份和字节保持不等于业务数据恢复；产品须验证自己的恢复语义。


<a id="framework-platform"></a>

## 平台依赖与运行结果

产品仍只声明输入、解析事实、生成运行描述。平台依赖按生命周期拆分：创建前已有的引用必须先解析；只有 Pod 启动后才出现的地址和 CSI 绑定，在应用工作负载之后观察。两者各自保留状态和刷新记录，不能用同一个 Pending 阻断所有阶段。

### 输入和调用链

生成的 `spec.clusterConfig` 平铺三部分：独立运行控制、`framework.ClusterConfig` 和产品 S。框架公共部分包含 `authentication` 与 `vectorAgentConfigMap`。字段冲突在定义和生成时拒绝；公共字段不会混入产品 S 的反序列化。`EffectiveInput.Platform` 和 `FactInput.Platform` 提供公共平台输入，产品字段继续通过 `ClusterConfig` 访问。

内部执行顺序：

1. 严格输入、公共/产品配置继承和完整拓扑准备。
2. 产品 `ResolveFacts` 通过精确只读 Get 解析已有外部对象；缺失/无效事实只保留对应组的旧资源。
3. 每组产品生成一次运行描述；框架按真实 LogOutputs 决定是否解析集中日志目的地。
4. 纯装配、四通道覆盖、最终消费关系检查。
5. 对声明的 Secret/Listener 来源和最终 Pod 的 Secret 环境变量引用进行创建前核对，再应用资源。
6. 从当前 StatefulSet 和 Pod 观察 CSI 绑定及 Listener 地址，刷新组的 `PlatformObservation`。
7. 使用新的组观察生成共享输出。Pending 保留上次有效输出，不撤回生产结果的 Pod。

`GeneratePreparedGroups` / `AssemblePreparedGroups` / `RefreshClusterOutput` 属于内部管线。它们不是产品可编排的阶段接口，也没有新增事后任意资源修改钩子。

### 平台目录

`Directory.Secret` 和 `Directory.Listener` 声明目录的来源；同一目录不能同时是 Data、Secret、Listener。平台目录不是框架生成文件或日志的写入目标，必须有主进程或初始化进程的只读访问。

| 声明 | 实际装配 | 结果观察 |
| --- | --- | --- |
| `SecretVolume.SecretName` | 当前 namespace 的原生 Secret 卷，文件 mode 0440 | 当前生产者 Pod 就绪 |
| `SecretVolume.SecretClass` | `secrets.kubedoop.dev` 通用临时 PVC，原生 class/format/scope/Kerberos service 注解 | 当前 Pod 的 PVC/PV 身份与挂载就绪 |
| `ListenerVolume.Class` | `listeners.kubedoop.dev` 通用临时 PVC，class 注解 | CSI 创建的 Listener 属于当前 PV，读取其实际地址/端口 |
| `ListenerVolume.Name` | 同 namespace 现有 Listener 的通用临时 PVC，listenerName 注解 | 当前绑定、现有 Listener 的实际结果 |

SecretName 与 SecretClass、Listener Class 与 Name 各自互斥。框架不把凭据字节写进生成 ConfigMap、状态或模板注解。
SecretClass 的凭据生成、证书内容/有效期及 CSI 挂载属于平台组件；产品负责将挂载文件转换成自身原生认证配置。

最终 podOverrides 仍最后执行。它可以调整未受约束的原生字段；若移除、替换、使用 subPath 或嵌套挂载遮蔽已声明的平台目录，会得到明确的消费关系冲突，不能静默启动一个失去原配置来源的产品。

### 观察与刷新

`status.groups[].facts` 描述创建前产品事实及按实际日志声明解析的集中目的地事实；`status.groups[].platform` 描述平台准备或后置观察，包含 phase、diagnostic、观测对象 UID/RV 和 Listener 地址。`PlatformReady` 是单独条件，不能替代 WorkloadsReady 或业务查询成功。

后置读取使用新的一轮精确读取缓存。它检查 StatefulSet 当前 revision、Pod owner 和 Ready、通用临时 PVC 的 Pod owner、PV 的 claim UID，以及自动 Listener 的 PV owner。任何缺失结果为 Pending；身份冲突/API 错误保留错误诊断。未全部就绪时不发布局部地址清单，不把旧 PV 的 Listener 当作新实例的结果。

原生 Secret 的 UID/resourceVersion、SecretClass/ListenerClass/已指定 Listener 的 UID/generation 构成不含值的模板摘要。Secret 更新会使使用其文件或最终 SecretKeyRef/EnvFrom 的 Pod 替换；可选且缺失的 Secret 不阻止创建，后续出现会刷新。类或 Listener 的纯 status 更新不会导致无休止滚动。地址本身在下一次观察中刷新共享 ConfigMap，不要求 CR 编辑或 Pod 替换。

有无自定义 resolver 都默认每 30 秒刷新，Pending 使用较短的 2 秒周期；错误的单 key 退避同样有界。该间隔是队列调度上界，不是 API 故障或队列拥塞时的墙钟保证。暂停仍优先阻止执行；停止继续使用独立的工作负载排空路径。


<a id="framework-s3"></a>

## S3 连接领域

S3 是一个有明确语义的连接领域。产品在自身配置中嵌入 `framework.S3Connection`，框架识别这个确定的类型、处理角色继承和分支切换，产品使用解析后的连接事实生成原生配置。不引入字段级 atomic 标签、可注册 union 引擎或“任何对象都是原子”的合并开关。

### 输入、继承与解析

`S3Connection` 的 type 为 `disabled`、`inline` 或 `reference`；产品零值默认等价于 disabled。inline 提供 host、port、tls、region、pathStyle 和 credentials；reference 指向同 namespace 的 `s3.kubedoop.dev/v1alpha1 S3Connection`。

默认值 → 角色 → 角色组。同一 type 或省略 type 时按字段继承；显式切换 type 清除全部旧分支，然后应用新分支。角色组可以只改变 pathStyle 并继承同分支其余字段；切换到 reference 时不携带旧 inline 端点和凭据。disabled 清除整条连接。每层拒绝跨分支字段，完整值在折叠结束后验证。该确定领域可嵌套于产品 struct/map，序列仍遵循整体替换规则。

引用缺失/删除中为 Pending，API 读取失败保留为读取错误，非法端点和不支持的 TLS 配置为 Invalid。通过 FactsReader 精确读取，框架记录 UID/resourceVersion 并刷新；事实包含 endpoint、region、pathStyle 和凭据**引用**，不读取或返回凭据字节。

host 必须是 DNS/IP；未指定 port 时 HTTP 为 80、HTTPS 为 443；未指定 region 时为 us-east-1。当前支持经过系统 CA 验证的 HTTPS，不接受组织 S3Connection 中 `verification.none` 或尚未有消费者的自定义 CA 声明。

### 凭据与运行期边界

inline.credentials 必须显式选择 native `secretName` 或 `secretClass`，只有后者可设置 scope。reference 读取组织 S3Connection 的 credentials.secretClass 及 node/pod/service/listener-volume scope。两条路径都声明包含 `ACCESS_KEY`、`SECRET_KEY` 的只读运行目录：

- native Secret 使用 Kubernetes 原生卷；
- SecretClass 使用 generic ephemeral 卷，经 Pod 所属 PVC 绑定到真实 secret-operator CSI PV；创建前验证声明，创建后由平台观察挂载结果。

端点解析不等待 CSI 生成的秘密字节，因此不会形成“必须先有 Pod 的结果才能创建 Pod”的依赖环。secretName 与 secretClass 不同时选择，也不退回未声明的默认凭据链。

产品须在自己的进程中读取凭据文件并按原生配置机制消费；init 容器不能向主容器导出环境变量。
固定启动命令与可覆盖的 Args 必须分离，避免环境准备过程破坏 CLI 覆盖契约。
凭据不得写入 ConfigMap、状态、facts 或启动参数。Trino 的具体属性及 launcher 说明见[产品指南](../examples/trino-operator/README.md#platform-domains)。


<a id="framework-logging"></a>

## 日志配置与集中送达

本领域保持两种责任：产品把有效 `logging` 翻译成其进程真正读取的原生配置，并声明实际产生的日志文件；框架决定是否装配 Vector、解析集中目的地并连接所有声明文件。产品不重复合并日志输入，也不构造 Vector 容器。

### 集中目的地契约

标准 `spec.clusterConfig.vectorAgentConfigMap` 是当前 CR namespace 中的 ConfigMap 名称。该 ConfigMap 的 `data.ADDRESS` 是单个 DNS/IP 与端口，例如：

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: log-destination
data:
  ADDRESS: vector-aggregator.observability.svc:6000
---
# Cluster CR 的 spec 片段
clusterConfig:
  vectorAgentConfigMap: log-destination
workers:
  config:
    logging:
      enableVectorAgent: true
```

`ADDRESS` 不是任意 Vector YAML、URL 或凭据。框架验证 DNS/IP 和 1–65535 端口，JSON/YAML 编码仍由编码器负责。使用 [Vector 原生 sink](https://vector.dev/docs/reference/configuration/sinks/vector/) 连接对应的 Vector source。目的地未指定时保留文件到 stdout JSON 的消费路径。

`ResolveVectorDestination` 使用只读 `FactsReader` 精确读取。缺失引用为 `Pending / VectorDestinationMissing`；缺失或无效 ADDRESS 为 `Invalid / InvalidVectorDestination`；API 读取失败仍为读取错误。控制器负责记录实际 UID/resourceVersion、定期重新读取和按组隔离。已禁用 Vector 或没有实际 LogOutputs（例如 file OFF）的组不因该引用等待。产品生成只调用一次，控制器在拿到真实日志声明后解析目的地，再进入纯装配。解析成功仅表示地址配置已得到验证，不表示远端正在接收。

解析后的 `AssemblyOptions.VectorDestination` 只传入当前组装配。生成的 `vector.yaml` 使用该地址；同时地址进入框架 Vector 容器的 `FRAMEWORK_VECTOR_DESTINATION` 环境变量，作为 PodTemplate 的配置变更触发值。因此只更新引用 ConfigMap 的 ADDRESS，就会更新物化配置并替换运行中的采集器，无需修改 CR，也无需依赖另一个 restarter 才交付这个目的地变更。其他配置文件的变更仍遵循既有 restarter 契约。

文件覆盖与最终 Pod 覆盖仍执行既有规则。更改采集器命令或生成配置会失去框架的结构关系证明，不能把这种未知关系报告为送达保证。网络故障时的缓冲/重试由 Vector 负责；当前 Vector 数据目录为临时卷，本领域不提供日志的永久保存或恰好一次送达承诺。

### 有原生消费者的适配器

Trino 的原生日志约束见[产品指南](../examples/trino-operator/README.md#native-trino-logging-and-process-identity)。

`pkg/framework/logging.Python` 接收有效 `framework.ContainerLogging`，返回 Python 标准库 `logging.config.dictConfig` 可直接加载的 JSON：

- console/file handler 各自消费阈值，不为了最低共同配置而强行令二者相等；
- ROOT 与命名 logger 使用 Python 自身的层级传播规则；
- TRACE/DEBUG/INFO/WARN/ERROR/FATAL 映射为 5/10/20/30/40/50，OFF 关闭对应 handler；
- file 使用标准库 RotatingFileHandler，10 MiB、3 份备份；产品提供实际绝对文件路径和可写目录；
- 产品显式加载 JSON，并只在 File.Level 非 OFF 时声明该文件为 LogOutput。

`TestPythonNativeConsumerThresholds` 启动实际 python3 进程加载生成文档，再观察 stdout 和实际日志文件。它验证文件 DEBUG、console WARN、命名 logger DEBUG 与 ROOT INFO 的共同作用，以及 file OFF 不创建文件。它不通过 Go 重实现 Python 过滤规则来模拟成功。

不列出只有编码函数、没有真实消费和执行证据的 Java 日志适配器支持矩阵。


状态提交与 Pod 替换是不同的异步观察点。新 Pod Ready 不能替代同 generation 下新 ConfigMap UID/resourceVersion 的 resolved 事实记录。


<a id="framework-lifecycle"></a>

## 初始化与工作负载协调

### 产品声明与执行责任

`RuntimeDescription.Initializers` 声明有序的 `Process` 列表。有文件需要物化时，框架先装配物化器，再按声明顺序
装配普通 init containers，最后启动主进程。每个初始化进程有自己的 image、command/args、env、
身份和显式目录访问；没有隐式获得主容器所有挂载。未指定 image 时继承已解析的产品镜像。
普通 init container 不能声明 lifecycle/probe。产品初始化必须幂等：kubelet 可以重新执行它。
失败初始化不会启动主进程，也不能用一个历史完成标记绕过本次实际执行。

`Process.Lifecycle` 和 `StartupProbe`、`ReadinessProbe`、`LivenessProbe` 使用 Kubernetes 类型。
框架将它们装配到主容器；产品选择真实协议和命令，kubelet 执行。它们不经过 file/env/CLI 覆盖；
最终 podOverrides 仍然最高。覆盖改变生命周期、初始化顺序、探针或退出预算时，
`assembly.lifecycle` 报告 Unknown，不能继续把原声明当成经过验证的消费前提。

### 可恢复的协调

`RuntimeDescription.Coordination` 包含 `ProgressDeadline` 和 `ShutdownPriority`。
前者为 1 秒至 1 小时的无进展预算；后者越小越先退出，同优先级组独立推进。
拥有初始化进程的工作负载必须声明协调预算。

- 框架显式使用 StatefulSet `OrderedReady` 和 `RollingUpdate`；Kubernetes 持久化并执行逐 Pod 滚动。
  框架不再实现一套与 StatefulSet 竞争的副本控制器。
- 普通 CR 缩容、停止和组退休共用 `nextScaleDown`。每次最多降低一个 ordinal；开始下一步前，
  必须直接读取实际 Pod，确认上一 ordinal 消失并核对当前 StatefulSet UID。
- 全集群停止与多组退休按已验证 live 工作负载上的优先级执行。较低优先级仍 Pending/失败时，
  不开始较高优先级。停止时的普通 apply 不能跳过这个顺序，把所有 StatefulSet 一次设为零。
- `framework.kubedoop.dev/workload-coordination` 保存当前产品策略；`workload-progress` 保存版本、
  StatefulSet UID、目标摘要、进展摘要和起始时间。控制器重启后从这些 live 信息继续，status 丢失不重置预算。
- 进展只计副本/版本、当前 Pod UID/phase/终止状态、成功完成的初始化阶段。
  resourceVersion、失败重试计数和经过时间不构成进展。
- 超时报告失败并保留工作负载；不强删 Pod、不删除数据、不把超时改写为业务退出成功。
  实际进展或新的目标重新建立预算，已经恢复的工作负载清除过期进展记录。

停止完成仍要求目标副本为零、当前 StatefulSet 控制器观察和实际 Pod 全部消失。
数据操作不把 Stopped 条件当作数据操作授权或静止证明；独立 executor 要求相关 StatefulSet 已退休、
仍存在的相关 CR 已暂停，并直接读取源/目标 PVC 消费者。停止完成不等于产品事务提交或磁盘内容正确。

协调策略必须跨越最终资源克隆和 apply 边界。资源克隆先保留完整值，再深拷贝引用字段。
即使状态计数已经为零，只要前序工作负载的实际 Pod 尚在，后续优先级仍须等待。

当前保证覆盖框架发起的缩容、stopped、组退休，以及原生 StatefulSet 滚动和 kubelet lifecycle。
CR 删除仍由 Kubernetes owner-reference GC 执行，不保证跨角色停止顺序；管理员强删、节点断电、
进程 OOM、跨组零中断、业务数据升级/回滚不在此协调协议的保证范围。
产品原生退出、查询结果和主进程终态需要分别验证，具体 Trino 行为见[产品指南](../examples/trino-operator/README.md)。


<a id="framework-data-operations"></a>

## 数据身份与显式数据操作

框架使用独立的 `DataAsset` 和 `DataOperation`，产品 CR 的退休或删除不再同时抹去数据历史。
正常协调在确认 PVC/PV 双向绑定之后自动创建账本；停止和退休的共享检查只读取账本，不创建资源。
账本记录原始 PVC/PV UID、实际源 CR 的 GVK/name/UID、角色/组/槽位、StorageClass 和规范容量。
这些是实际观察到的身份，不能从名称或用户提供的标记推断出来。

### 责任与调用链

```text
产品正常协调 → 检查 Retain/来源/双向绑定 → 创建 DataAsset → 在 PVC 记录 asset 名称
产品停止/退休/重新创建 → 读取账本 → 拒绝身份丢失、操作锁、已转移数据的隐式重建

授权者创建不可变 DataOperation → 独立 executor → 持久阶段/Job → 重新验证来源与绑定
  → 写入目标 retained-data / retained-binding → 写历史并解除 asset 锁
  → 产品正常协调消费已授权目标 PVC
```

`DataAsset` 没有产品 CR 的 owner reference；历史与当前身份留在独立 CR 中。
账本是 namespaced 资源，生命周期独立于产品 CR，不承诺抵抗命名空间本身被删除。
迁移的旧副本进入 `status.retiredCopies`，仍可通过准确身份发起独立销毁操作。
迁移不会顺便删除源数据，销毁旧副本也不会删除当前数据。

部署分为两个权限域。产品 operator 只有账本 get/list/watch/create 和既有 PVC 收据权限；
独立 `cmd/dataops` executor 才有执行 Job、重绑和回收卷的权限。
`framework-data-authorizer` 可以创建操作，没有修改执行状态、PV 或数据账本的权限。
CRD/平台 RBAC 位于 `config/framework-data`，独立 executor 部署位于 `config/framework-data-executor`；CRD 和 DeepCopy 由根 Makefile 生成。

### 授权前置条件

操作输入、approval 与 RBAC 的区别、不可变请求、源/目标身份、暂停、退休及实际消费者检查，
统一见[数据操作安全契约](security.md#framework-data-authorization)。这些条件在开始和每个执行阶段都必须满足。

### 三条实际执行路径

| 动作 | 持久阶段与实际行为 | 完成证据 |
| --- | --- | --- |
| adopt | Locked → 创建目标名 PVC → ReleaseSource → Rebind → BindTarget → Record | 原 PV UID 保持；新 PVC UID、目标双向绑定与正式框架来源收据 |
| migrate | Locked → 创建目标 PVC → Copy → BindTarget → Record | 源复制前、源复制后、目标三个 SHA256 文件树一致；目标绑定完整；旧副本留账 |
| destroy | Locked → Erase → DeleteClaim → DeleteVolume → ReclaimVolume → Record | worker 确认目录为空；按 UID/RV 删除 PVC；PV 记录 operation UID 后 Retain→Delete；实际 provisioner 回收后端并删除 PV |

复制 worker 在源只读挂载下复制普通文件、目录、符号链接，并核对文件内容、长度、权限和链接目标。
迁移对象是卷根之下的数据树；卷挂载根目录仍由 provisioner 管理，worker 不复制或改变其权限、
时间戳等元数据。执行保持审批绑定的非 root UID/GID，以 fsGroup 提供的数据写权限完成复制。
特殊文件会报错；不会把复制成功扩张为 POSIX ACL、数据库日志恢复或存储快照一致性保证。
目标由本操作新建，只有属于同一 operation UID 的半成品目录允许重试；已有未知数据的目标拒绝覆盖。

销毁是在已批准的文件系统上删除并核验文件，随后要求实际存储 provisioner 回收卷。
它不承诺介质级擦除或清除供应商独立快照。改变 reclaimPolicy 前必须已有持久的擦空收据；
仅删除 Retain PV 对象会遗留后端存储，因此这里不把该动作当作销毁完成。
只有已持久化 `ReclaimVolume` 阶段才接受 PV 消失作为回收完成；此前丢失 PV 保留锁并报告完成状态未知。
这也覆盖写入 Delete 策略成功、但保存阶段失败且 PV 随即消失的窗口，避免猜测后端回收结果。

### 重启、失败和证据

每轮将 phase、Job UID、attempt、worker termination receipt 写入 DataOperation status。
worker receipt 包含 operation UID、校验类型与迁移文件树摘要，先持久化再删除已完成的 worker Pod。
Job 保留；controller 重启从已保存阶段继续，不重复创建已存在目标或凭空替代丢失身份。
检查固定 worker 同时覆盖 Pod 与容器的执行身份和容器安全限制，防止容器配置覆盖批准的非 root UID/GID。
完成记录以 operation UID 去重；账本已更新但 operation status 写失败时可以继续完成。

worker 的执行有 30 分钟上限。失败的 Job、Pod 和原因保留，asset 保持锁定。
修复原因并检查/移除失败 Pod 后，授权者设置 `framework.kubedoop.dev/data-retry` 为下一整数尝试号。
下一 Job 使用新 attempt 名称，旧失败 Job 保留，审批意图保持不变。
身份变化、未知消费者或来源错误不会被重试参数绕过；删除正在执行的操作会暂停后续动作并保留锁，
不自动把“请求删除”解释成恢复源数据或继续销毁的授权。


<a id="existing-generic-reconciler-sdk"></a>

# Existing GenericReconciler SDK

The following numbered sections describe the existing GenericReconciler API only.
They remain available for its maintenance and do not override the product-description framework above.

# 1. Document Overview

## 1.1 Document Purpose

This document systematically expounds the design philosophy, architectural layering, core module implementation, design pattern application, and key problem solutions of the Common Product Cluster Operator SDK (hereinafter referred to as "SDK"). It provides interface specifications, integration guides, and extension bases for developers, ensuring consistency and maintainability when developing multiple products (HDFS, HBase, DolphinScheduler, etc.) based on the SDK.

## 1.2 Core Objectives

- **Common Logic Reuse**: Distill common logic for multiple products cluster (reconciliation process, resource construction, configuration merging) to reduce repetitive coding.

- **Flexible Product Extension**: Support product-specific logic customization and adapt to differentiated needs through abstract interfaces and extension point mechanisms.

- **Precise State Convergence**: Ensure the CR desired state (Spec) is consistent with the cluster actual state, resolving issues such as orphaned resource residue.

- **Seamless Ecosystem Compatibility**: Align with K8s Operator specifications and Kubebuilder practices, adapting to mainstream technical solutions such as Webhook and Generics.

## 1.3 Terminology Definition

- **Product**
  - Specifies the software application definition managed by the Operator, such as HDFS, HBase, or DolphinScheduler. It defines the available component types (Roles) and overall service logic.

- **Cluster**
  - Represents a specific deployment instance of a Product, defined by a Custom Resource (CR). It serves as the root object aggregating global configurations (e.g., Image version, Security features, Vector/Logging sidecars) and all component Roles.

- **Role**
  - Represents a logical functional component within a Product (e.g., NameNode or DataNode in HDFS). It acts as a template and grouping mechanism for RoleGroups, defining shared configurations (Config Overrides, shared logging settings) that are inherited by its definition. A Role contains two distinct configuration sections:
    - `roleConfig`: Kubernetes-level management controls (e.g., PodDisruptionBudget), Role-scoped only, NOT inherited by RoleGroups.
    - `config`: Workload runtime configuration (resources, affinity, logging), serves as defaults for RoleGroups and CAN be inherited and overridden.

- **RoleGroup**
  - The physical unit of deployment and resource isolation under a Role. Each RoleGroup maps directly to a Kubernetes `StatefulSet` (and associated Services and ConfigMap; the PodDisruptionBudget is **role**-level, covering every group of the role — see §4.1.5). This allows a single Role to be partitioned into multiple groups with distinct hardware specifications (CPU/Memory), replica counts, or specialized configurations (e.g., a "high-performance" DataNode group vs. a "standard" group).

- **Naming**
  - Role and RoleGroup names are **identifiers, not labels**: the framework derives the name of every resource it builds (`<cluster>-<role>-<group>`), the value of several `app.kubernetes.io/*` labels, and a label *key* from them. They are therefore constrained to lowercase RFC 1123 labels and validated at admission, so a name that cannot become a Kubernetes identifier is rejected where the user can act on it rather than partway through a reconcile.
  - Every identifier the framework derives must be **bounded**, because the user-supplied parts are not. A derived name is truncated with a hash suffix (`RoleGroupResourceName`); a derived label key falls back to that bounded name when the natural form would exceed the 63-byte limit (`RoleGroupMarkerLabelKey`). A derivation that lands inside an immutable field — the StatefulSet's `.spec.selector` — may only change its output for inputs that could never have produced that object in the first place, or existing clusters become unpatchable.

- **SecretClass**
  - An object managed by `secret-operator`, enabling the injection of sensitive data (Certificates, Kerberos Keytabs, Passwords) into Pods via the Kubernetes CSI (Container Storage Interface). Workloads reference a `SecretClass` to mount volumes that are dynamically populated by specific security backends.

- **Overrides**
  - A hierarchical configuration mechanism allowing precise customization of generated resources. It supports overriding Configuration Files (e.g., XML/Properties), Environment Variables, CLI arguments, and Pod attributes (via PodTemplateSpec). **Important**: Override fields (`configOverrides`, `envOverrides`, `cliOverrides`, `podOverrides`) are **flattened** directly at Role/RoleGroup level, NOT nested under an `overrides` field. RoleGroup overrides inherit from and take precedence over Role overrides, and both take precedence over the product's computed config layer (see §2.5–§2.6): the full precedence is **Product Config < Role < RoleGroup**.

- **Webhook**
  - Kubernetes admission webhooks integrated into the SDK for defaulting and validation. MutatingWebhook runs first to populate missing fields with safe defaults before persistence, while ValidatingWebhook runs next to enforce invariants and business rules (e.g., invalid replica counts, missing dependencies). Failed validation rejects the request so only valid specs enter reconciliation.

- **Extension**
  - An SDK-specific plugin mechanism that injects custom business logic directly into the Reconciliation loop. Extensions run during the Reconcile phase (Pre/Post Reconcile) to handle complex operations like status updates, dynamic config generation, or interaction with external systems using Go code.

- **Orphaned Resources**
  - Kubernetes resources (StatefulSets, Services, ConfigMaps) that exist in the actual cluster but are no longer defined in the CR's `Spec` (e.g., after a RoleGroup is removed). The SDK implements a strict cleanup logic to safely identify and delete these resources to ensure state convergence.

- **ClusterOperation**
  - A cluster-level control block that influences operator behavior at runtime (e.g., `reconciliationPaused` and `stopped`). It is not part of override mechanisms; it is an operational control-plane input.

# 2. Core Design Philosophy

## 2.1 Interface-Driven Design (IDD)

By defining core contracts through abstract interfaces, the SDK core logic relies on interfaces rather than concrete implementations, achieving "decoupling of common logic and product-specific logic." New products only need to implement corresponding interfaces without modifying the SDK core code, reducing extension costs.

## 2.2 Desired State Convergence

Following the K8s Operator core paradigm, the CR Spec serves as the desired state. The actual state of the cluster is converged towards the desired state through a reconciliation loop, supplemented by reverse convergence logic (cleaning up orphaned resources) to ensure bidirectional consistency.

## 2.3 Separation of Common and Specific

The SDK is responsible for implementing common logic (such as resource construction, configuration merging, and generic Webhook validation), while the product side implements specific logic (such as HDFS ZK validation, HBase Region configuration) through extension interfaces, balancing reusability and flexibility.

## 2.4 Type Safety and Idempotency

Go Generics are introduced to eliminate the risk of type assertions and ensure compile-time type safety. All core operations (create/update/delete resources) implement idempotency to avoid exceptions caused by repeated execution.

## 2.5 Strict Merge Strategy

Configuration is assembled by folding an **ordered stack of layers**, each layer overriding the ones below it. The `ConfigMerger.Merge` operation is variadic and applies the layers in increasing precedence (lowest first):

```
Product Config (lowest)  <  Role overrides  <  RoleGroup overrides (highest)
```

- **Product Config** is the product's *computed* configuration (see §2.6), contributed at reconcile time as the lowest layer.
- **Role / RoleGroup overrides** are the user's CRD `configOverrides`/`envOverrides`/`cliOverrides`/`podOverrides`.

Because the user's CRD overrides sit above the product layer, **a value a user sets in the CRD always wins** over the product's computed value.

Each field type folds with a defined strategy:

- **Map Types (Config files / Env)**: **Deep Merge**. A higher layer's keys override the same keys in a lower layer; new keys are appended.
- **Slice Types (CLI args)**: governed by `ConfigMerger.SliceMergeStrategy`.
  - **Replace** (`MergeStrategyReplace`, the default): a higher layer's non-empty slice completely replaces the lower layer's slice.
  - **Append** (`MergeStrategyAppend`): the higher layer's items are appended to the lower layer's slice.
  - **Empty means "unset", not "clear"**: an empty or nil higher-layer slice leaves the lower layer untouched, so a RoleGroup cannot erase the CLI arguments its Role set — it can only replace them.
  - The `GenericReconciler` builds its merger with `config.NewConfigMerger()` and does not expose the strategy, so **inside the framework reconcile path the strategy is always Replace**. Append is reachable only by product code that drives its own `config.ConfigMerger`.
- **PodTemplate (`podOverrides`)**: Kubernetes **Strategic Merge Patch**, applied layer over layer, allowing fine-grained overrides of Pod fields (e.g., changing container image while keeping volume mounts). A layer whose raw JSON does not decode into a `PodTemplateSpec`, or whose patch fails, is treated as absent; the reason is recorded on `MergedConfig.PodOverrideErrors` and surfaced by the reconciler as a `Warning` event (see §4.14.2) rather than silently dropped.

> The two-layer Role↔RoleGroup merge is the special case of this fold with no product layer; existing callers that pass only those two layers are unaffected.

**`config` (the typed workload config) folds by its own rule: the finest granularity at which the result still means what both authors said.** This is a separate axis from the override stack above, and it is a design constraint rather than an implementation detail — the failure it prevents is *silent partial loss*, where overriding one knob discards the siblings the Role configured:

| field | granularity | why not coarser / finer |
| --- | --- | --- |
| `resources` (cpu/memory/storage) | **per leaf** | Overriding one leaf — a `storageClass`, a `cpu.min` — is the normal way to use the API; struct granularity silently dropped every sibling. |
| `affinity` | **wholesale** — any layer that states one replaces the layer beneath it entirely; `affinity: {}` clears. What the replacement discarded is **reported** as an `AffinityOverridden` Warning event | This is the one field in the table that Kubernetes itself defines, so the granularity question is not ours to answer freely: `PodSpec.affinity`, a Helm value and a Kustomize patch all replace wholesale, and a framework that folded it per member would oblige a user to learn a merge semantic for exactly one field of one CRD. `resources.cpu.min` is a knob and can fold per leaf without that cost. The loss a product default takes when a user states any affinity is paid to the event, not to a second semantic. |
| `gracefulShutdownTimeout`, `logging` | per field / per container, and **per level** inside a container | Scalars and an already keyed map. Within a container, an entry naming `console`, `file` or a logger **without stating a level** is "inherit", not "clear" — `console: {}` keeps the Role's threshold. |

This works only because these fields carry **no CRD-level default**: structural defaulting fills a field as soon as its enclosing object exists, so a `+kubebuilder:default` on a leaf makes "unset here" indistinguishable from "explicitly the default" and the Role's value can never win. Defaults therefore live at consumption time (`StorageResource.GetCapacity`, `RoleGroupConfigSpec.GetGracefulShutdownTimeout`, the renderers' root-level INFO).

**A product's own defaults for this block are a third layer beneath the two, folded by the same rule.** `RoleDeclaration.ConfigDefaults` supplies what a role's `resources`, `affinity` and `gracefulShutdownTimeout` should be when the CR says nothing. Reusing `FoldCommonConfig` rather than adding a "fill the nil fields" pass is the whole design: a struct-level nil check would discard a product's `cpu.min` the moment a user set `cpu.max`, reintroducing the silent partial loss this table exists to prevent. One rule, three layers, no new precedence for a user to learn.

That third layer is what makes `affinity` the hard case, and the resolution is **not** to fold it per member. Per-member inheritance was implemented and reverted once before, and the objection that carried the revert is not weakened by the new layer: `affinity` is a Kubernetes type, and every adjacent tool a user knows replaces it wholesale, so per-member folding buys the product's default at the price of a semantic the user can only discover from `kubectl explain`. Wholesale replacement stands. What it costs — a user pinning an instance type also discarding the `podAntiAffinity` their product ships to spread a quorum — is answered by making the loss **loud** rather than by changing the rule: `FoldCommonConfig` returns the members each replacement discarded, and the reconciler emits an `AffinityOverridden` Warning naming the layer, the members and how to keep them. An `affinity: {}` clears and reports nothing, because clearing is precisely what that value asks for; that clearing rule works for `affinity` and not for `resources` because the schemas differ — `affinity` is `x-kubernetes-preserve-unknown-fields`, so the API server never prunes inside it and a stored `{}` is always something the user wrote, while `resources` is structural and `cpu: {}` may be a pruning artifact.

This is the framework's general answer whenever a user's value legitimately beats a product's: let the user win, and say what was displaced. It is the same shape as `ImmutableFieldIgnored` and `PodOverrideIgnored`, and it is distinct from **Constraint** (§2.5b) — the framework is not overruling the user here, it is reporting the consequence of honouring them.

**The product's OWN fields in that block are folded by the product**, with `FoldProductConfig[T]` — presence-wins per top-level field, with no depth, applied to the struct a product defines by embedding `*RoleGroupConfigSpec` inline and adding its own fields beside it. The framework skips the embedded pointer so the two halves cannot fight over it, and `ValidateProductConfigType[T]()` refuses shapes that cannot be folded honestly: a **bare scalar** (the zero value is the fold's "was this stated?" test, so product fields are pointers) and an untagged **composite** (replacing one wholesale drops the siblings a user did not restate). A composite that is a single policy rather than a set of knobs accepts wholesale replacement explicitly with the `kubedoop:"atomic"` struct tag; otherwise it is flattened into scalar pointers.

`logging` is **included**, and folds through `productlogging.MergeLoggingSpec` like the rest. It used to be rejected with a `*ValidationError`, and that was correct for the code as it stood: logging was merged in the reconciler from the CR's two levels only, and Vector enablement and the rendered config file were both decided before a product default was ever read, so one set there would have applied to neither. Moving the fold ahead of both consumers is what made the field honourable — the reconciler now reads Vector enablement off the folded value. This is issue #631's item 2, fixed by making the code match the doc rather than the reverse.

**This rule binds product operators too, and it is checkable.** It is not an explanation of why the
SDK's own fields are shaped the way they are — it is a constraint on any CRD whose `config` block
this framework folds, including every field a product adds of its own. Documentation alone was not
enough: the rule has been written here since #544 and trino-operator carries
`+kubebuilder:default:="5GB"` on `queryMaxMemory` inside `config` today, so a role group that
declares `config` merely to set `resources` silently gets `5GB` instead of the role's value. The
executable form is `testutil.HaveNoInheritedConfigDefaults`, a static scan of the generated CRD YAML
that needs no envtest, no CR fixture and no cluster:

```go
It("declares no CRD default inside a role config block", func() {
    Expect("config/crd/bases/*.yaml").To(testutil.HaveNoInheritedConfigDefaults())
})
```

It reports **every** default under a folded `config`, at any depth, and deliberately applies no
depth heuristic — see the note below for why "deeply nested is safe" is false. Roles are detected
structurally (any schema node declaring a `roleGroups` property), so it covers both the generic
`spec.roles[*]` map and products that flatten roles into named fields such as `spec.coordinators`.
An argument matching no files is an error rather than a pass, because a guard that silently
inspects nothing reports success.

> **The rule is easy to satisfy in one place and forget in another.** `LogLevelSpec.Level` kept `+kubebuilder:default:="INFO"` long after the same defect was fixed for `resources`, and it was not inert: a Role asking for `DEBUG` plus a RoleGroup writing an empty `console: {}` produced `INFO`, because the API server filled the leaf the moment `console` existed. `mergeContainerLogging` even carried a guard written for exactly that case — it could never fire, since the value it tested for emptiness had already been filled. **A guard against a defaulted field must be verified through the API server**; the unit test covering that guard passed throughout, because a Go-constructed spec never meets structural defaulting.

**Schema-free fields must be decoded strictly.** `config.affinity` is a `RawExtension`, so the API server neither validates nor prunes it, and a lenient decode discards what it does not recognise — which made a misspelled key (`nodeAffinty`) pass admission and evaporate, scheduling the pods anywhere with nothing reported. Any field the SDK accepts as opaque JSON and then interprets must reject unknown members loudly (`reconciler.DecodeAffinity`), because it is the only layer left that can.

## 2.5b Four Concepts, Not One "Default"

Issue #631 asked for "role config defaults". The framework had grown three partial answers to that
request and no statement of what it was answering, so each new case was resolved by whichever of the
three the author found first. The design position is that **four distinct concepts** were being
called "a default", and each has a different relationship to the user's own CR:

| concept | position relative to the user | who states it | mechanism |
| --- | --- | --- | --- |
| **Declaration** | no user layer exists at all | product | `RoleDeclaration` |
| **Default** | beneath the user's role and role group levels | product | `RoleDeclaration.ConfigDefaults`, folded by `FoldCommonConfig` |
| **Derivation** | computed *from* the folded result, then folded beneath the user's overrides | product | `RoleGroupResolver` → `Contribution` |
| **Constraint** | above the user — deliberately not implemented | — | — |

The separation is what makes each mechanism answerable. A role's ports, primary container name,
command and log producers are **declarations**: the CRD gives the user no field to state them, so
there is nothing to merge and no precedence to decide — which is why they are plain struct fields
rather than a merge layer, and why the framework can resolve the image and the Vector gates from
them before any role group is built. A role's `resources` are a **default**, so they fold. A JVM heap
sized from the effective memory limit is a **derivation**, and cannot be either of the others: it is
not knowable before the fold, and it must still lose to a user's `configOverrides`.

**Constraint is absent on purpose.** A framework that can overrule what a user wrote in their own CR
needs a way to tell them, and every candidate is worse than not doing it: silently clamping produces
a cluster that does not match its spec with nothing naming the cause; failing the role group turns a
product's opinion into an outage; a Warning event is not read. A product that genuinely must reject a
value rejects it in a **webhook**, where the user sees it at `kubectl apply` and no reconcile has
happened yet.

Conflating the four is what produced the defect the issue reports. The one mechanism that existed
for "product default" was reached only by a handler setter, was consulted after the role group's
ConfigMap had already been built, and replaced `affinity` wholesale — so it could not express a
declaration (too late), could not express a derivation (input not yet computed), and lost sibling
fields when it did express a default.

## 2.6 Derived Config vs. Defaulting (Separation of Layers)

The SDK distinguishes **two different mechanisms** by which a product supplies values that are not typed by the user, and they must not be conflated:

| | **`ProductDefaulter`** (Webhook) | **`RoleGroupResolver`** (merge layer) |
|---|---|---|
| **Targets** | Typed **Spec fields** (image, ports, replicas) | **Config-file content** (e.g. `config.properties`, connection strings) |
| **When** | Admission (Mutating Webhook), **persisted into the Spec** | Every reconcile, **never persisted** |
| **Semantics** | Static fallback **defaulting** ("fill if absent") | **Config computation** (may derive from live cluster state) |
| **Upgrade propagation** | No — frozen into the Spec at admission time | **Yes** — recomputed with the current operator each reconcile |
| **Derived-from-live-state** | Freezes / goes stale | **Recomputed every reconcile** |

**Image resolution is the case that shows why the split matters, and it was on the wrong side.**
Kubedoop product images are published only with the `-kubedoop<version>` suffix, and the natural
value of that suffix is the **operator's own build version** — a reconcile-time fact that moves when
the operator binary is upgraded. Defaulting it in a webhook persists it into the spec at admission,
so a cluster admitted by operator 0.1.0 keeps asking for `-kubedoop0.1.0` images forever and an
operator upgrade cannot move it onto the co-released image. `GenericReconcilerConfig.ImageResolution.Defaults` is
therefore evaluated on every reconcile, and `ImageSpec.ResolveImage(productName, defaults)` folds it
under whatever the user wrote:

| layer | source | when |
| --- | --- | --- |
| user | `spec.image` | whatever the CR states, per field |
| product | `ImageResolution.Defaults` (and per-role `RoleDeclaration.Image`) | every reconcile |

`ProductName` no longer decides *whether* `spec.image` is read — it supplies the product name, which
is both the `app.kubernetes.io/name` value and the repository path segment. Coupling the two meant a
product that wanted the labels had to accept an image path that could not express the tag
convention, and three migrated operators consequently hand-rolled image resolution and dropped
`app.kubernetes.io/version` entirely. An unresolvable `spec.image` is now an **error** rather than a
silent fall back to the handler's static image: running a version nobody asked for is not a safe
default for a stateful product (the same call as `config.affinity` above).

The assembled **tag** is validated for the same reason, and it is the only place a check can happen:
`productVersion` and `kubedoopVersion` both land in it, its grammar is
`[A-Za-z0-9_][A-Za-z0-9._-]{0,127}`, and the Kubernetes API server does not validate
`container.image` at all — so a value that breaks the grammar is accepted, stored, and reported only
by the kubelet, as `InvalidImageName` on a pod, while the reconcile returns success and the cluster's
conditions stay green. The failure this closes is structural rather than hypothetical: the
recommended wiring is `KubedoopVersion: version.BuildVersion`, and a build variable's unset value is
whatever the scaffold chose — `"N/A"` in the current one, whose `/` makes the reference unparsable.
`spec.image.custom` is exempt, because the framework assembles nothing there.

**A registry credential is not part of image resolution.** `spec.image.pullSecretName` folds over
`ImageResolution.Defaults.PullSecretName` per field like everything else, but is resolved by its own accessor
and applied to the pod directly. Where the image *lives* is independent of how its reference was
built, so the credential must survive the two paths that assemble no reference at all — `custom`, and
a product resolving its own images with `ProductName` empty — which is precisely where a private
registry is most likely to be in play.

**`RoleGroupResolver` receives a `ctx`, a client, the CR and the role group's build context, and may fail.** "Recomputed every reconcile, and may reflect the current state of the cluster" is only true if the hook can *read* the cluster; without those parameters it was a pure function of the CR, so the products that most needed this layer — anything resolving an `S3Connection` reference or a ZooKeeper address — could not use it, and a failed lookup had nowhere to go but a swallowed error or a panic.

**Its position in the pass is the substantive part.** It runs after `FoldCommonConfig` has produced the role group's effective config and before anything is built, which is the window the framework did not previously have: the effective config was not computed until *after* the role group's ConfigMap had been assembled, so nothing derived from it could reach a config file at all. That is what forced three operators to hand-write the same JVM-heap calculation and a fourth to freeze the answer into a literal `-Xmx419430k` — 0.8 of one role's default memory, applied to every JVM component and immune to every user override. `reconciler.HeapMB` is that calculation, centralised, and `RoleGroupBuildContext.EffectiveConfig()` is its input.

The returned `Contribution` folds **beneath** everything already merged, by the merge's own per-dimension rules. It carries no CLI dimension on purpose: `cliOverrides` merge by replacement, so a contributed layer would either be erased whole by any user value or erase the user's — neither is a default. Product arguments are `RoleDeclaration.Command`, which has no user layer at all.

**It must be deterministic for a given (CR, effective config).** Its output lands in a ConfigMap the framework applies with `CreateOrUpdate` and watches; a value that varies per pass rewrites that ConfigMap every pass, wakes the reconciler through its own watch, and never errors, so the workqueue never backs the loop off.

- **`ProductDefaulter`** is the right place for stable, user-facing **typed Spec defaults** (see §4.3). The value becomes part of the user's persisted Spec and is visible via `kubectl get`.
- **`RoleGroupResolver`** is the right place for **product-intrinsic and derived config-file content** — e.g. a ZooKeeper connection string built from the actual resources, a quorum peer list from pod ordinals, or a JVM heap sized from the role group's resources. It is *config generation, not defaulting*: computing it at reconcile time (rather than freezing it into the Spec at admission) means an operator upgrade **recomputes** the configuration for existing clusters, and values derived from mutable state stay fresh. It is injected as the lowest merge layer (§2.5), so user overrides still win.

  **Recomputing is not the same as delivering.** A change to config-file content converges the role group ConfigMap and nothing more: the pod template is unchanged, so no rollout follows, and these products do not re-read their configuration at runtime. Restarting the pods is the platform's job, not this SDK's — `commons-operator`'s restarter watches workloads whose **object metadata** carries `restarter.kubedoop.dev/enable=true` and, when a ConfigMap or Secret the pod references — as a volume or through an env var's `valueFrom` — changes, stamps the pod template so the workload controller rolls it. The SDK deliberately does not reimplement that: doing so would cover only the ConfigMap it owns (not mounted Secrets, not a product's own ConfigMaps, not secret expiry) and would give one intent two competing expressions. Labelling the workload is therefore a deployment decision, made by labelling the **cluster CR** (whose labels the reconciler propagates into every resource's metadata) rather than in operator code; unlabelled, a config-file change reaches the running processes at the next restart, whenever that is.

# 3. Layered Architecture Design

The SDK adopts a layered architecture design, divided from top to bottom into the API Layer, Abstract Interface Layer, Core Component Layer, and Tools Layer. Each layer has clear responsibilities and controllable dependencies. The specific layering and dependencies are as follows:

## 3.1 Layered Architecture Diagram

The following shows the architecture layering relationship (dependency from top to bottom): Specific Product Layer → Abstract Interface Layer → Core Component Layer → Tools Layer → API Layer; the specific product layer is implemented based on the abstract interface layer and relies on the capabilities provided by the SDK.

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

## 3.2 Core Responsibilities and Components of Each Layer

### 3.2.1 API Layer (Data Contract Layer)

Defines the common data model, serving as the data exchange contract between the SDK and the product side. It does not depend on any other layers, ensuring model stability.

- **Core Components**:
    - `GenericClusterSpec`: Common cluster configuration, containing cluster-level configuration, role, and role group configuration.
    - `GenericClusterStatus`: Common cluster status, employing standard Kubernetes **Conditions** (`Available`, `Progressing`, `Degraded`, `Paused`, `ServiceHealthy`, `ReconcileComplete`) to represent complex states beyond simple replica counts. Each answers a distinct question and none may be derived from another — see §4.8.2.
    - **Auxiliary Models**: `RoleSpec` / `RoleGroupSpec` (role and role group definitions), `RoleConfigSpec` (Role-scoped Kubernetes controls, e.g. PDB), `RoleGroupConfigSpec` (workload runtime configuration), `OverridesSpec` (the flattened override fields), `ImageSpec`, `LoggingSpec`, `ResourcesSpec`.

- **Design Points**: Specific product Spec/Status must embed common models (e.g., `HdfsClusterStatus` embeds `GenericClusterStatus`) to achieve state reuse. The `ServiceHealthy` condition allows products to report business-level readiness (e.g., HDFS safe mode off).

### 3.2.2 Abstract Interface Layer (Contract Definition Layer)

Defines core interfaces and extension contracts. It only depends on the API layer and is the core of SDK's "multi-product reuse," divided into business interfaces and extension interfaces.

- **Business Interfaces**:
    - `ClusterInterface`: The cluster-level contract a product CR satisfies. It embeds controller-runtime's `client.Object` — so name, namespace, UID, labels, annotations and GVK come from the CR's embedded `metav1.ObjectMeta`/`TypeMeta`, not from product-written accessors — and adds exactly two SDK-specific methods: `GetSpec() *v1alpha1.GenericClusterSpec` and `GetStatus() *v1alpha1.GenericClusterStatus`, which project the product's own spec and status onto the generic shapes the framework reconciles against. There is no status setter: `GetStatus` returns a pointer into the CR and the framework writes conditions, `observedGeneration` and role group state through it, which is why a product's own status fields survive a cycle untouched.
    - `ClusterResource[T ClusterInterface]`: `ClusterInterface` plus `DeepCopy() T`, the method controller-gen already generates for every root API type. It exists because a type parameter cannot be allocated with `new(T)` when `T` is a pointer type, so the reconciler materialises the object it reads into by copying a prototype; going through `runtime.Object` would hand back an interface and reintroduce a runtime assertion. Hold a CR as `ClusterInterface`; parameterise over one as `ClusterResource[CR]`.
    - `RoleGroupHandler`: The primary implementation extension point for product operators. Each product implements this interface to define the specific Kubernetes resources (StatefulSet, Services, ConfigMaps) built for each RoleGroup. The `GenericReconciler` calls `BuildResources()` on this handler during reconciliation.
    - **There is no role-level interface.** Role and role group configuration reaches a handler as *data*, through the `reconciler.RoleGroupBuildContext` value the reconciler builds per role group and passes to `BuildResources`: it carries `RoleName`, `RoleSpec`, `RoleGroupName`, `RoleGroupSpec` (with the role-level `config` already folded in, group winning per field) and `MergedConfig` (the folded product-config/role/role-group override stack). The reconciler iterates `GenericClusterSpec.Roles` directly, so a product declares roles in its CRD and never implements an accessor for them.

- **Extension Interfaces**:
    - `ClusterExtension[CR]/RoleExtension[CR]/RoleGroupExtension[CR]`: Extension point interfaces, defining custom logic before and after reconciliation at each level. Each is generic over the product's own CR type, so a hook receives that type directly. Role-level customization of `role.config` is done here (a `RoleExtension.PreReconcile` hook), not through a separate extender interface.
    - `ExtensionRegistry[CR ClusterInterface]`: Extension registry, managing the registration, priority-based ordering, and execution of the extensions of **one** CR type. A registry is owned by the reconciler it is passed to (§4.2.3); the package exposes no process-wide instance.

### 3.2.3 Core Component Layer (Common Logic Layer)

Implements common business logic based on abstract interfaces. It depends on the Abstract Interface Layer and Tools Layer, and does not directly depend on specific products, ensuring logic reuse.

- **Core Components**:
    - `ClusterReconciler` (implemented as `GenericReconciler` in the SDK): Cluster reconciler, the entry point for the core reconciliation process, including role traversal, extension point execution, and orphaned resource cleanup.
    - `ConfigMerger`: Configuration merger. Folds the ordered configuration layer stack (Product Config < Role < RoleGroup, see §2.5) via a variadic `Merge(...)`, applying per-type strategies (deep merge / replace-append / strategic merge patch).
    - `ConfigGenerator`: Configuration generator, transforming merged configuration maps into specific file formats (XML, Properties, YAML, etc.).
    - `SidecarManager`: Sidecar container manager, handling the injection of auxiliary containers (e.g., Log collection, Monitoring) into the Pod Spec.
    - `StatefulSetBuilder`: Resource builder, generating K8s resources such as StatefulSet and Service corresponding to role groups.
    - `RoleGroupCleaner`: Orphaned resource cleaner, cleaning up orphaned role group resources based on the comparison results of Spec and Status.

### 3.2.4 Tools Layer (Common Utility Layer)

Provides non-intrusive common utility functions for the Core Component Layer to call, reducing repetitive coding.

- **Core Tools**:
    - `K8sUtil`: K8s resource operation tool, encapsulating idempotent operations like CreateOrUpdate and Delete. This is the only tool the reconcile loop itself wires in; the CR status write is handled by the reconciler directly (see §4.13.2).
    - `ExecUtil`: Pod command execution tool (`util.NewExecUtil(client, restConfig)`) for running commands inside containers. It is a **consumer-facing helper**: the reconciler never constructs it, so a product that needs in-container exec builds it from its own `*rest.Config`.

### 3.2.5 Specific Product Layer (Extension Implementation Layer)

Implements product-specific logic based on SDK abstract interfaces without modifying SDK core code, relying only on the API Layer and Abstract Interface Layer.

- **Implementation Points**:
    - **CR structs implement `ClusterInterface` — `GetSpec` and `GetStatus`, the rest comes from the embedded object metadata and generated deep-copy code — and provide a `RoleGroupHandler` to define product-specific resources.** The handler reads everything it needs about the role and the role group from the `RoleGroupBuildContext` it is handed; there is no role-level interface to implement (see §3.2.2).
    - Implement specific logic through extension interfaces (e.g., HDFS ZK connectivity check, Namenode heap size configuration).
    - Integrate Webhook specific validation and default value population logic.

# 4. Core Module Implementation

This section details the core modules of the SDK, organized into five functional categories:

| Category | Modules | Description |
|----------|---------|-------------|
| **Foundation & Lifecycle** | 4.1-4.4 | Core framework, extensions, webhooks, and cleanup |
| **Resource Generation** | 4.5-4.6 | Configuration and sidecar management |
| **Operational Management** | 4.7-4.8, 4.13-4.14 | Dependencies, health, errors, and events |
| **Security & Network** | 4.9-4.10 | Security and service exposure |
| **Operational Control** | 4.11-4.12 | Runtime controls and connections |
| **Constants & Configuration** | 4.15 | Constants architecture and domain derivation |

---

## 4.1 Generics Transformation Module

### 4.1.1 Design Background

Original interfaces relied on type assertions, presenting runtime error risks and code redundancy. Introducing Go Generics achieves compile-time type safety and reduces boilerplate code.

### 4.1.2 Core Implementation

- **Generic Reconciler Skeleton**: `GenericReconciler[CR ClusterResource[CR]]` (likewise `GenericReconcilerConfig[CR]` and `NewGenericReconciler[CR]`), constraining CR type and reusing the reconciliation process. The constraint is `ClusterResource[CR]` rather than `ClusterInterface` because the reconciler has to produce an empty instance of the CR to read into; it copies `GenericReconcilerConfig.Prototype` through the generated `DeepCopy() CR`, which returns the concrete type instead of a `runtime.Object` the reconciler would have to assert.
- **Generic Extension Interfaces**: `ClusterExtension[CR ClusterInterface]` (likewise `RoleExtension[CR]`, `RoleGroupExtension[CR]`), whose hooks receive the product's own CR type — a `PreReconcile` declared for `*TrinoCluster` is handed a `*TrinoCluster`.
- **Generic Webhook Contracts**: `ProductDefaulter[CR]` / `ProductValidator[CR]`, which mirror controller-runtime's `admission.Defaulter[T]` / `admission.Validator[T]` so a typed implementation is passed straight to the webhook builder (§4.3).
- **Per-CR-type registry, no erasure**: `ExtensionRegistry[CR ClusterInterface]` stores `ClusterExtension[CR]` / `RoleExtension[CR]` / `RoleGroupExtension[CR]` entries at the product's own instantiation, and `GenericReconcilerConfig[CR].ExtensionRegistry` is typed `*ExtensionRegistry[CR]`. The type parameter is load-bearing rather than cosmetic: Go generic types are invariant, so a `ClusterExtension[*TrinoCluster]` does not satisfy `ClusterExtension[ClusterInterface]` — a registry erased to the wide interface could only hold extensions written against `ClusterInterface` and would force every hook to convert its CR on entry. Instantiating the registry for the product's CR type is what removes that conversion, so a product extension contains no type assertion at all.
- **No process-global registry**: there is no package-level registry instance and no global accessor; a type parameter cannot be carried by a package-level variable, and every workaround reintroduces exactly the erasure this design removes. A registry is constructed with `common.NewExtensionRegistry[CR]()` and reaches the framework only through the reconciler config, which also means a binary hosting two products cannot run one product's hooks against the other's clusters — neither by construction (a foreign extension does not typecheck) nor by accident (there is no shared instance).

### 4.1.3 Core Value

Compile-time type checking removes runtime type assertions from the reconciler and from product extensions alike; new products only need to bind generic types, reducing boilerplate.

### 4.1.4 Handler Lifetime and Per-CR Inputs

**One `RoleGroupHandler` instance serves every cluster.** It is constructed once in `main.go` and the controller reuses it for every CR and every reconcile, so every field on it is process-wide state. That is not obvious from the embedding idiom the SDK encourages, and it has produced real defects downstream.

| where a value lives | lifetime | what belongs there |
| --- | --- | --- |
| handler fields | process | reconcile-**invariant** settings only — `ConfigGenerator`, `ConfigMountPath`, `LabelDomain`, the sidecar manager, the security contexts |
| `RoleDeclaration`, from `RoleProvider.DeclareRoles` | one reconcile pass | everything a ROLE is made of — ports, container name, command, probes, data volume, log producers, config defaults — computed from **this** CR |
| `Contribution`, from `RoleGroupResolver` | one role group of one reconcile | anything derived from **this** role group's effective config |

The middle row is what removed the hazard rather than documenting it. The handler no longer has a
field for a role's ports, image or container name, so the assignment that raced cannot be written.

Assigning a per-cluster value to a handler field from inside `BuildResources` fails in two ways, and the quieter one bites first:

- above `MaxConcurrentReconciles: 1` the writes **race**, and one cluster's image or TLS-dependent ports are built into another cluster's workload;
- at the default concurrency of 1 it **leaks**: a product that conditionally skips one assignment silently inherits the previous CR's value. spark-k8s-operator shipped exactly that — a CR omitting `pullPolicy` took the last CR's — with a serial reconcile loop and no race involved.

`BuildResources` on the base handler is **read-only on the handler**; the per-call fields are read from the build context, which is rebuilt per role group. The one place the framework itself held per-CR state was a handler-registered `SidecarManager`: `SetProductImage` writes the resolved image into its configs, so it is now cloned for the build. `VolumeProviders` already followed this shape and is the precedent the new fields copy.

Handler fields are still the right home for invariant configuration — this is a split, not a deprecation.

**The declaration also replaces the post-build patch.** `BuildResources` returns a finished object, and for anything the framework does not model the only extension point used to be mutating it afterwards. That cost twice: products re-derived what the handler already knew (zookeeper-operator located the primary container as `Containers[0]`, which a sidecar provider inserting a container earlier silently invalidates), and the edit landed *after* the framework's own ordering.

| declaration field | replaces | why declaring it first matters |
| --- | --- | --- |
| `Command`, `Lifecycle`, `ReadinessProbe`/`LivenessProbe`/`StartupProbe` | patching `Containers[0]` of the returned StatefulSet | they are applied to the primary container **by identity**, before `podOverrides` are strategic-merged, so the **user's** overrides still win; a post-build edit inverts that silently |
| `ListenerClass` (or `Contribution.ListenerClass` when the value is user-settable) | patching `Service.Spec.Type` after the fact | one shared `listener.ServiceTypeFor` mapping instead of a three-way switch re-derived per product |

An earlier design gave the product a `MainContainerCustomizer` callback here. It has been removed:
a callback is a channel whose precedence has to be explained and enforced (it had to be blocked from
changing the image, which was resolved once and already propagated to the sidecars), while a
declaration field simply has no user layer to beat. The image is now published read-only as
`RoleGroupBuildContext.ResolvedImage`, so that failure mode does not exist to guard against.

### 4.1.5 Writing a Role Group Handler

§4.1.4 covers a handler's *lifetime*; this covers its *contract*. Everything below was source-only before it was written down, which is why six operators independently rediscovered parts of it by reading each other's code.

#### The build context is the whole input, and half of it is writable

`BuildResources` receives no CR fields it has not been handed: `buildStatefulSet` takes `_ client.Client, _ CR` and reads nothing from either. Everything CR-derived reaches the framework through `RoleGroupBuildContext`, and the fields split into two kinds:

| the framework writes, the handler reads | the handler writes before delegating |
| --- | --- |
| `ClusterName`, `ClusterNamespace`, `ClusterSpec`, `RoleName`, `RoleSpec`, `RoleGroupName`, `RoleGroupSpec` (whose `Config` is the FOLD — read it as `EffectiveConfig()`), `ResourceName`, `ServiceAccountName`, `MergedConfig`, `VectorAggregatorAddress`, `Declaration`, `ResolvedImage`, `ProductName` | `VolumeProviders`, `ClusterLabels` |

The writable column shrank to two. Every other per-call channel was a second way to state something
the declaration already states, with its own precedence rule; `Declaration` and `ResolvedImage` are
now the framework's settled answers, and assigning `Declaration` here is **half-honoured** — the
image, the fold and the Vector gates are already derived from it — which is worse than being
ignored. The one output a product reaches for by method rather than field is
`LogFileTarget(decl)`: where a product-owned logging config must write its rolling file, or `""`
for console-only.

`ClusterLabels` is a materialised clone and is **never nil**, so a handler may add to it unconditionally; whatever it holds is merged into every built resource's metadata and pod template. `SidecarManager` is always non-nil under the reconciler — which makes `BaseRoleGroupHandler.WithSidecarManager` inert on that path, and useful only to a product that calls `BuildResources` itself.

#### What comes back, and what a nil field means

`RoleGroupResources` is not a bag of optional outputs. The apply path treats **nil as an instruction**, not as "leave it alone": a nil `MetricsService` or `PodDisruptionBudget` makes the framework *delete* the corresponding live object, because that is how a role group that stops declaring one is converged. `ExtraResources` carries arbitrary GVKs, which must be registered in the reconciler's scheme and live in the CR's namespace — the framework sets a controller owner reference, and a cross-namespace owner is rejected by Kubernetes.

**`ExtraResources` stays `[]client.Object`, and is validated instead.** Narrowing the type is the wrong lever: arbitrary GVKs are the feature, and no Go type can express "a kind the framework has no opinion about" more precisely than the interface already does. What *is* expressible is whether the apply path can write each entry at all, so three properties are checked before any resource of the role group is applied — a name (`CreateOrUpdate` addresses the object by name every reconcile, so `generateName` would create a new object each pass rather than converge one), the cluster's namespace (which also rejects a cluster-scoped object: Kubernetes honours no owner reference from a namespaced CR to one, so the framework has no lifecycle to offer it), and a **distinct identity** — no two entries, and no entry and a fixed slot, may address the same GVK and name.

That last one is the reason this is a validation rather than documentation. Two writers for one object in a single pass do not fail: the later apply wins and the earlier entry is silently discarded, and when the two desired states differ the object is rewritten on every reconcile — each write bumping its `resourceVersion`, waking the framework's own `Owns()` watch, and scheduling the reconcile that does it again. Nothing errors, so nothing backs off — the same self-sustaining shape that two writers produced for the restarter's pod-template annotation, reached from the other direction.

**The framework owns the fixed slots' names, and the handler owns their content.** `ConfigMap`, `Service`, `StatefulSet` and `PodDisruptionBudget` must be named `buildCtx.ResourceName`; `HeadlessService` and `MetricsService` that name plus `-headless` and `-metrics`; all six must live in `buildCtx.ClusterNamespace`. A slot that breaks either rule fails the role group with a `*ValidationError` *before any resource is applied*.

This is the direct consequence of the paragraph above. "Nil is an instruction to delete" only works if the framework can find the object the instruction refers to, and it finds all six by their derived names — in the in-spec reclaims and, when the role group leaves the spec, in `RoleGroupCleaner`. Nothing recovers a slot filled under a different name: live-orphan discovery rejects any object whose name is not the derived one, and the teardown's final confirmation re-checks the same fixed list before pruning the group's status entry. Such an object is applied, owner-referenced, reported healthy, and then outlives every teardown until the cluster CR is deleted.

The general rule this encodes: **an optional, product-supplied resource slot must have either a framework-owned name or a framework-stamped identity — never neither.** The framework picks the name for the fixed slots because a name is checkable at build time, against no cluster and in one reconcile; an identity label is only checkable against a live List, on a path where a stale cache answering "nothing here" is terminal. `ExtraResources` takes the other branch deliberately, because there the names are the product's: its reclaim is label-selected and opt-in through `SetupWithManagerOptions.ExtraOwns`. A product that needs a metrics Service under its own name uses that door.

The same reasoning bounds what a CR label may say. Labels are the one channel from a cluster's *deployer* to the built resources (§4.1.4), but three keys — `metrics.kubedoop.dev/service`, `pdb.kubedoop.dev/role`, `pdb.kubedoop.dev/role-group` — are the framework's own slot markers, and a reclaim deletes by their presence or value. They are filtered out of `ClusterLabels` for that reason: a marker that a user can set is a delete instruction that a user can forge. The filter is an enumerated set, not a domain prefix rule, because `restarter.kubedoop.dev/enable` establishes that `kubedoop.dev` is shared with the platform rather than private to this framework.

#### The container contract

- The primary container's name resolves `RoleDeclaration.MainContainerName` → the role group's resource name, and must be settled **before** `Build()`: `podOverrides` are strategic-merged by container name, so a later rename leaves the user's override appended as a phantom, image-less container.
- A **readiness** probe is generated on `Ports[0]`, so the first entry of `ContainerPorts` is part of the contract — put the port that means "this pod can serve" first. **No liveness probe is ever generated**; `builder.DefaultTCPLivenessProbe` is the opt-in.
- `config` and `data` are reserved pod volume/mount names. A `VolumeProvider` reusing either produces a pod the API server rejects.
- `MergedConfig.CliArgs` reaches the container as `args`; `MergedConfig.JvmArgs` reaches **nothing** — the framework merges it and never renders it, so a product populating it must render it itself.

#### The config mount is read-only

The generated ConfigMap is mounted **read-only** at `ConfigMountPath` (default `constant.KubedoopConfigDirMount`, `/kubedoop/mount/config/`). A product whose start-up rewrites a config file — Kerberos realm substitution, credential interpolation — must copy it to a writable directory first, conventionally `constant.KubedoopConfigDir` (`/kubedoop/config/`):

```sh
mkdir -p /kubedoop/config/
cp -RL /kubedoop/mount/config/* /kubedoop/config/
```

`-L` is the part worth knowing: a ConfigMap volume is a farm of symlinks into a hidden `..data/` directory, so a copy that preserves symlinks produces dangling links at the destination. This is a *framework consequence* with no framework helper, and it is stated here because it was discoverable only by reading a sibling operator's start script.

#### Ways to fail the build

Eleven causes across twelve sites inside `BaseRoleGroupHandler.BuildResources`. **Four** produce a `*ValidationError`, each carrying a distinct `Subject` a product can route on — `"image"` (an unresolvable or empty reference), `"podOverrides"` (a mount that displaces a framework-owned one), `"sidecar"` (a registered provider's `Validate` failing) and `"logging"` — and the rest are plain wrapped errors that the reconciler re-wraps as a `*ResourceBuildError`. All of them fail one role group, not the pass: role iteration is best-effort and sorted (§4.4).

#### If you do not embed `BaseRoleGroupHandler`

A handler implementing the interface directly inherits none of the conventions, and four of them are load-bearing:

1. every fixed slot must carry its derived name — the headless Service `<ResourceName>-headless` above all, since the StatefulSet's `serviceName` is derived from it and immutable. This one the framework now checks and rejects rather than leaving to convention;
2. the pod must mount the ConfigMap named `buildCtx.ResourceName`, which is what the framework's own ConfigMap is called;
3. `clusterOperation.stopped` must force replicas to 0 — it is implemented in the base handler, not in the reconciler;
4. `BuildRolePodDisruptionBudget` is an optional capability interface the reconciler type-asserts for; not implementing it silently disables the role-level PDB. `RoleProvider` and `RoleGroupResolver` are deliberately **not** in that class — they are explicit `GenericReconcilerConfig` fields, so a product that forgets to wire one gets the framework's stated default rather than a capability that was silently not detected. Role declarations and log producers used to be discovered by assertion, and a handler that implemented the method on the wrong receiver disabled the whole Vector pipeline with nothing reporting it.

## 4.2 Extension Point Mechanism Module

### 4.2.1 Design Approach

Reserve extension points at key nodes in the reconciliation process to support embedding custom logic on the product side, while unified management through a registry ensures ordered execution of extensions.

### 4.2.2 Extension Point Levels

1. **Cluster Level**: `PreReconcile` (Before Reconciliation), `PostReconcile` (After Reconciliation), `OnReconcileError` (On Exception).
2. **Role Level**: `PreReconcile`, `PostReconcile`, executed for a single role.
3. **Role Group Level**: `PreReconcile`, `PostReconcile`, executed for a single role group.

### 4.2.3 Extension Registration

- **Registry Instance**: `common.NewExtensionRegistry[CR]()` builds an empty registry for one product CR type; the type argument is explicit, since a no-argument call cannot infer it. The registry is a plain value the operator owns — there is no process-wide instance and no global accessor (§4.1.2).
- **Registration Timing**: Extensions are registered during Operator initialization, in the `main.go` setup phase before the Manager starts, so all of them are present when the first reconcile runs. They go into the registry the operator constructed, not into a shared one.
- **Wiring**: The registry reaches the framework only through `GenericReconcilerConfig[CR].ExtensionRegistry` (typed `*common.ExtensionRegistry[CR]`). **This field is what makes extensions run at all**: a reconciler constructed without it runs against an empty registry, so every hook is a silent no-op. A binary managing several CR types builds one registry per type — sharing one instance across two products is a compile error.
- **Registration Methods**: `RegisterClusterExtension(ext, opts ...RegistrationOption)`, `RegisterRoleExtension(...)` and `RegisterRoleGroupExtension(...)`. These three are the entire registration surface: options are variadic, so there are no separate priority or options variants. There is no generic `Register()` method — the level is part of the method name because the registry keeps one ordered list per level.
- **Registration Options**: `common.WithPriority(p)` sets the priority (Lowest=0, Low=25, Normal=50, High=75, Highest=100; default Normal); `common.WithStopOnError(bool)` overrides the hook's default fault tolerance for that one registration (see §4.2.5).
- **Execution Order**: Extensions execute in **priority order (highest first)**. Same-priority extensions execute in **registration order** — each entry carries a registration sequence number, so the ordering is total and does not depend on sort stability.
- **Clearing**: `Clear()` empties the registry **in place** and resets the sequence counter. Emptying rather than replacing matters because a constructed reconciler captured the registry pointer: handing out a fresh instance would leave it executing a stale one. This is what a test uses between cases instead of resetting global state.
- **Introspection**: `GetClusterExtensions()` / `GetRoleExtensions()` / `GetRoleGroupExtensions()` return the registered extensions in execution order; `HasClusterExtensions()` and its siblings, plus `Count()`, report what is registered.

```go
// main.go, before mgr.Start(): build the registry, then hand it to the reconciler.
registry := common.NewExtensionRegistry[*trinov1alpha1.TrinoCluster]()
registry.RegisterClusterExtension(extensions.NewCatalogExtension())
registry.RegisterRoleExtension(extensions.NewHealthExtension())
registry.RegisterClusterExtension(extensions.NewDiscoveryExtension(mgr.GetScheme()),
    common.WithPriority(common.PriorityLow))

reconcilerCfg := &reconciler.GenericReconcilerConfig[*trinov1alpha1.TrinoCluster]{
    // ... client, scheme, recorder, role group handler, prototype ...
    ExtensionRegistry: registry, // omitting this field means no hook ever runs
}
```

### 4.2.4 Extension Lifecycle

- **Initialization**: Extensions are instantiated once during Operator startup. The SDK does not recreate extensions per reconciliation.
- **State Management**: Extensions should be stateless or manage their own internal state. The SDK passes the current CR context to each extension method, enabling access to cluster state without requiring persistent extension state.
- **Shutdown**: There is **no shutdown hook**. The extension interfaces declare only `Name`, `PreReconcile`, `PostReconcile` and (cluster level) `OnReconcileError`; an extension owning a resource that must be released on operator shutdown registers its own `manager.Runnable`.

### 4.2.5 Execution Process

The reconciler iterates through the registry's entries in **priority order (highest first)**, and per-hook fault tolerance decides whether a failure skips the entries behind it.

- **Normal Execution**: Extensions execute sequentially. Each extension receives the reconcile context, the client, and the CR.
- **CR Mutation — spec and status are not symmetric**:
  - **Spec: observe, do not mutate.** The framework's only write to the CR is `Status().Update`, which the API server applies to the status subresource alone, so an in-memory spec edit is never persisted. It is not reliably *observed* either: `reconcile()` takes `spec := cr.GetSpec()` once, *before* the cluster `PreReconcile` hooks run, and role iteration, cleanup and health evaluation all read that value — a `GetSpec()` that materialises a fresh struct per call (legal but discouraged, §5.1.4) hands them a snapshot no later edit can reach. A hook that must change the spec writes it through the client and lets the resulting watch event drive the next reconcile.
  - **Status: mutate in place — the framework persists it.** A hook writes status through the pointer `cr.GetStatus()` returns, or straight onto the product's own status fields, and the cycle's final `updateStatus` carries both to the API server. That is by design, not incidental: the write is issued from the in-memory object precisely so a hook's status contribution survives (`ClusterInterface` exposes only the embedded generic status, so re-fetching first would reload the stored value over a product's own fields; see §4.13.2). The guarantee is covered by a regression test, `persists product-specific status fields written by an extension hook`.
  - A hook that writes *neither* — one whose whole job is an external side effect — still gets its failure reported on the CR through the `Degraded` condition (see Error Handling below).
- **Error Handling**:
  - Every hook failure is wrapped in an `*ExtensionError` naming the extension.
  - `PreReconcile`/`PostReconcile` **stop on the first failure by default** and return it, which aborts the reconcile and maps to the `Degraded` condition. An extension registered with `common.WithStopOnError(false)` does not stop the loop; its failure is logged, the remaining extensions still run, and the collected failures are joined and returned so they still reach the CR status.
  - `OnReconcileError` handlers **all run by default** and their own failures are only logged — the original reconcile error stays authoritative. Registering an error handler with `common.WithStopOnError(true)` makes its failure abort the remaining handlers instead.
- **State Recovery**: If an extension modifies external state and a subsequent extension fails, the SDK does not roll anything back. Extensions implement their own compensation logic, typically in `OnReconcileError`.

## 4.3 Webhook Integration Module

### 4.3.1 Integration Scheme

Based on Kubebuilder annotation-driven practices, integrating MutatingWebhook and ValidatingWebhook to implement configuration pre-processing and legitimacy validation.

### 4.3.2 Core Functions

- **MutatingWebhook**:
    - **Common Logic**: `webhook.DefaultGenericClusterSpec(spec, defaultImage)` defaults **the image only** — it copies the operator's default `ImageSpec` when `spec.image` is absent, and sets `spec.image.pullPolicy` to `IfNotPresent` when empty. The SDK ships no CPU/Memory, ZooKeeper or log-path defaulting.
    - **Specific Logic**: Product side implements the `ProductDefaulter[CR]` interface to populate product-specific default values for **typed Spec fields** (e.g., HDFS Namenode heap size, default ports). These are *defaults* — static fallbacks persisted into the Spec at admission.
    - **Scope boundary**: `ProductDefaulter` defaults typed Spec fields only. Product **config-file content** (and any value derived from live cluster state) is *computed* at reconcile time via `RoleGroupResolver`, not defaulted here — see §2.6 for the distinction.
- **ValidatingWebhook**:
    - **Common Logic**: `webhook.ValidateGenericClusterSpec(spec, fldPath)` validates **the image only** — when `spec.image.custom` is unset, `repo`, `productVersion` and `kubedoopVersion` are required, and `pullPolicy` must be one of `Always`/`IfNotPresent`/`Never`. It returns a `field.ErrorList` for composition with the product's own checks. Two opt-in helpers are available for product validators: `webhook.ValidateFieldLength` and `webhook.ValidateNonEmptyMap`.
    - **Specific Logic**: Product side implements the `ProductValidator[CR]` interface to execute business rule validation (e.g., HDFS HA mode configuration validation).
- **Enforced by the CRD schema, not by admission code**: replica bounds (`RoleGroupSpec.Replicas` carries `+kubebuilder:validation:Minimum=0` and `+kubebuilder:default=1`) and CPU/Memory quantity formats (`resource.Quantity` fields) are checked by the OpenAPI schema the apiserver applies. The SDK deliberately does not duplicate them in webhook code.

### 4.3.3 Admission Workflow Overview

MutatingWebhook runs first to apply defaults. ValidatingWebhook runs next to enforce invariants. Failed validations reject the request before persistence, ensuring only valid specs enter reconciliation.

`ProductDefaulter[CR]`/`ProductValidator[CR]` mirror controller-runtime's `admission.Defaulter[T]`/`admission.Validator[T]`, so a typed implementation is wired directly (controller-runtime v0.23.x):

```go
func SetupWebhookWithManager(mgr ctrl.Manager) error {
    return ctrl.NewWebhookManagedBy(mgr, &HdfsCluster{}).
        WithDefaulter(&HdfsClusterDefaulter{}).
        WithValidator(&HdfsClusterValidator{}).
        Complete()
}
```

`webhook.NewDefaulterAdapter` / `webhook.NewValidatorAdapter` erase the CR type to `runtime.Object` for the older `WithCustomDefaulter`/`WithCustomValidator` entry points; they remain available but are no longer the recommended wiring.

### 4.3.4 Deployment Adaptation

Automatically generate TLS certificates via cert-manager, and Webhook configuration files via Kubebuilder. No manual configuration of certificates and access rules is required during deployment.

## 4.4 Orphaned Role Group Resource Cleanup Module

### 4.4.1 Core Scheme

Adopts a hybrid scheme of "Spec vs Status comparison as primary, cluster resource query as secondary," which improves efficiency while avoiding accidental deletion.

Deletion is a **state machine driven across several reconciles**, not a single pass. An orphaned role group holds pods that a stateful product expects to retire the way its own rolling update would, so the cleaner scales the workload to zero, waits for the StatefulSet controller's ordered reverse-ordinal drain, and only then deletes — and every step confirms its effect before the next one is issued. Nothing blocks a reconcile worker: a step still in flight ends the pass for that role group and returns a requeue delay, and the next cycle resumes from the first step that has not settled. Every step is a Get-then-act, so re-entering is idempotent.

### 4.4.2 Execution Process

1. Get the desired role group list (`desiredGroups`) of roles from Spec. Each role group reconciled in this cycle is recorded in `Status.RoleGroups`.
2. Get the actual role group list from **two** sources and union them:
   - the **live cluster** — the role group ConfigMaps and StatefulSets in the CR's namespace carrying `app.kubernetes.io/instance` and `app.kubernetes.io/managed-by`, controller-owned by this CR, whose `app.kubernetes.io/component` + `app.kubernetes.io/role-group` labels reconstruct exactly the object's own name via `RoleGroupResourceName`;
   - `Status.RoleGroups`, the ledger the operator writes for itself.
3. Calculate orphaned role groups: `orphanedGroups = actualGroups - desiredGroups`.

   > **Why the live cluster and not the ledger alone.** `Status.RoleGroups` is a record the operator must first have *successfully written*. Anything that loses it — the process dying between applying a role group's resources and updating the CR, a backup tool restoring the CR without its status subresource, a `kubectl replace` — makes the resources it named invisible to the cleaner permanently, because nothing else ever enumerates them. They keep their PVCs, their PDB and their pods until a human notices. Reading the live cluster removes that dependency; the ledger stays in the union to cover resources created before the framework stamped `app.kubernetes.io/role-group`, whose role group cannot be recovered from their labels.
   >
   > All four conditions on a live object are required. A discovery ConfigMap (§ discovery) carries the same instance/managed-by pair and the same owner reference, and a product's `RoleGroupResources.ExtraResources` may carry the handler's entire label set — only a name equal to what `RoleGroupResourceName` would produce for those labels identifies the framework's own slot. Both kinds are listed because the teardown deletes the StatefulSet before the ConfigMap: a pass interrupted in between leaves a ConfigMap a StatefulSet-only inventory would never see again.
   >
   > An empty owner UID disables live discovery entirely, exactly as it disables the role-PDB reclaim: with no owner to match, every labelled object in the namespace — including a sibling cluster's — would look like this cluster's.
4. Reclaim the **role-level PDBs of roles that vanished from the Spec entirely** (see "Removed roles" below). This runs before — and independently of — the group loop, which returns early when `orphanedGroups` is empty: a role's groups are pruned from the status snapshot as they are deleted, so by the time its PDB needs a retry there may be no orphaned group left to carry the pass.
5. For each orphaned role group — roles in sorted order, so the sequence of events is reproducible across the several cycles a deletion spans — advance the deletion state machine one pass: gray-delete gate, then `PDB → StatefulSet (scale to zero → drain → delete) → ConfigMap → Service → headless Service → metrics Service`, stopping at the first step that is still in flight.
6. Remove from `Status.RoleGroups` **only those role groups whose resources were really deleted** — every step settled in this pass. A group still inside its gray-delete grace period, one whose drain is still running, and one whose pass failed all stay in the status snapshot and are retried on the next reconcile instead of being silently forgotten. The pruned map is persisted by the reconcile's final status update (step 7 of the loop).
7. Return the earliest wakeup the cleanup needs — a remaining gray-delete deadline, or the poll interval of a deletion in flight; `0` when nothing is pending — so the reconcile loop requeues exactly when the pending work becomes due (see §4.8.4).

### 4.4.3 Safety Protection Mechanisms

- **Pre-Delete Validation**:
  - Every resource is fetched before deletion; `NotFound` is treated as "already gone" and short-circuits to success.
  - Ownership is confirmed through the **ownerReferences** — the resource must carry a reference whose UID matches the CR and whose `controller` flag is true. (An empty owner UID disables the check, for callers that drive the cleaner directly.)
  - Resources not owned by this cluster are **NOT deleted** — this prevents a name collision with a manually created or foreign resource from destroying it. A foreign resource counts as *settled*, not as pending: this cluster will never delete it, so waiting for it would pin the role group in `Status.RoleGroups` forever.
  - The headless (`<resource>-headless`) and metrics (`<resource>-metrics`) Services are addressed by **derived name**, and a role group may legitimately be called `<group>-headless` or `<group>-metrics` — making its own Service collide with the orphan's derived name under the same controller owner reference, which ownership alone cannot separate. A derived name that belongs to a role group the Spec still declares is therefore skipped.

- **Deletion Order** — the order only means anything because each step is **confirmed gone** before the next is issued:
    1. **PDB** (PodDisruptionBudget) — removed first so it cannot block the eviction of the pods that follow.
    2. **StatefulSet** — the ordered drain, below.
    3. **ConfigMap**.
    4. **Service**, then **headless Service** and **metrics Service** — the Services go last so the terminating pods can still resolve each other. The metrics Service is a framework slot like the other two, so it is reclaimed here instead of outliving its role group.

- **Ordered drain of the StatefulSet** (`deleteStatefulSet`): deleting the object outright leaves its pods to cascade garbage collection, which removes them in arbitrary order. Instead:
    1. `spec.replicas` is set to `0` (a nil replica count means the API server default of `1`, so it is a scale-down like any other). The write is wrapped in `retry.RetryOnConflict`: the same object is written by the apply path and by any autoscaler pointed at it, and a routine 409 must not leave the role group half-deleted. A `NotFound` here means the StatefulSet vanished mid-scale-down — nothing left to drain.
    2. The pass ends and requeues. The StatefulSet controller retires the pods in reverse-ordinal order, each honouring its `terminationGracePeriodSeconds`.
    3. Later passes wait while `.status.replicas > 0`. Deleting before that reaches zero would cancel the ordered shutdown the scale-down was for.
    4. Only then is the StatefulSet deleted, and the deletion confirmed.

- **Deletion confirmation** (`confirmDeleted`): acceptance is not removal. An object held by a finalizer keeps answering `Get` until the finalizer clears, and a cached client lags behind its own writes. Treating "`Delete` returned nil" as "gone" is exactly what would make the deletion order meaningless, so every accepted `Delete` is followed by a re-read; an object still present yields *in flight*, and the pass resumes on a later reconcile.

- **Per-group error isolation**: a failure is confined to its own role group. The error is collected, that group keeps its status entry and its requeue, and the **remaining groups still make progress** — otherwise one wedged role group would keep every other orphan alive indefinitely. The collected failures are joined and returned to the reconcile loop, which logs them and continues; cleanup failures are non-fatal for the cycle (the exception is a 429, below).

- **Poll interval**: a step in flight asks the caller to wait `DefaultDrainPollInterval` (5 s), overridable with `RoleGroupCleaner.WithDrainPollInterval` (a non-positive value keeps the default). It paces the state machine, not the pod termination itself — the cycle it schedules only re-reads the resources it is waiting on — so products with a long `terminationGracePeriodSeconds` can raise it to avoid polling.

- **Removed roles**: role *group* orphans are found by diffing `Status.RoleGroups`, but a role deleted from the Spec outright leaves nothing to diff against, and its role-level PDB (applied only while the role is declared) would survive with a selector matching pods that no longer exist. Those PDBs are found by **listing on the label `pdb.kubedoop.dev/role`**, which carries the role name, rather than by derived name: a product may ship its own PDB through `RoleGroupResources.PodDisruptionBudget` under the same controller owner reference, so ownership alone cannot identify the framework's slot. An empty owner UID disables this reclaim entirely — with no owner to match, every labelled PDB in the namespace (including a sibling cluster's) would look like this cluster's.

- **Gray Deletion (opt-in grace period)**:
  - With `GenericReconcilerConfig.GrayDeleteGracePeriod > 0`, an orphaned role group is not deleted on first detection. The cleaner stamps `orphan.zncdata.dev/pending-deletion` (an RFC3339 timestamp) on the group's primary resource — its StatefulSet, falling back to its ConfigMap — and defers.
  - Deletion proceeds on a later reconcile once the grace period has elapsed. The remaining time is returned to the reconcile loop and turned into a `RequeueAfter`, so the deletion happens on schedule rather than waiting for an unrelated watch event.
  - If the role group is re-added to the Spec before the deadline, the annotation is cleared, so a future re-orphaning gets a full grace period again.
  - A primary resource owned by **another** cluster is never annotated (that would mutate an unrelated object on a name collision), which also leaves no timestamp to run a grace period from. The pass proceeds instead of deferring: each deletion is ownership-checked on its own, so the foreign objects are skipped and whatever this cluster does own under that name is reclaimed. Deferring would keep the role group in `Status.RoleGroups` for as long as the foreign object exists.
  - With the default value `0` the annotation is never written and the deletion state machine starts on first detection.

- **PVC Handling**:
  - By default, **PVCs are PRESERVED** during orphaned resource cleanup to protect data.
  - Setting the annotation `operator.zncdata.dev/delete-pvcs: "true"` on the cluster CR makes the cleaner also delete the PVCs of an orphaned StatefulSet, listed by the StatefulSet's pod selector (which is what the StatefulSet controller stamps on the PVCs it provisions).
  - **The irreversible step goes last.** The deletion runs *after* the drain — once `.status.replicas` has reached 0, or once the drain deadline expires — and immediately before the StatefulSet itself. Deleting a role group is undoable right up until its data goes, so nothing irreversible may happen while the pods are still running: a user who removes a role group by mistake and re-adds it during the drain gets a restart, not a restore. This is a design constraint on any future teardown step, not a detail of this one.
  - PVCs before the StatefulSet, not after: the cleaner reaches them *through* the StatefulSet's selector, so deleting the workload first would leave them unreachable. In this order a process death between the two steps simply re-enters the same pass. The drain-timeout path falls through to the same deletion, so a pod that will not terminate cannot silently leak the volumes the user asked to reclaim.
  - **Scope**: this applies to orphan cleanup only — role groups removed from the Spec. The SDK registers no finalizer, so deleting the whole CR runs no SDK teardown code: the PVCs of a deleted cluster are left to Kubernetes' own garbage collection rules. `Reconcile` still has to *recognise* deletion, because foreground propagation keeps the CR readable until its dependents are gone — it returns as soon as `deletionTimestamp` is set, so the loop never re-creates the dependents that deletion is waiting on.

### 4.4.4 Concurrency Conflict Handling

- **404 Not Found**: treated as success — the resource was already deleted by another process.
- **409 Conflict**: the annotate and scale-down paths are Get-then-Update, so they carry a `resourceVersion` and a concurrent modification surfaces as a conflict. The **scale-down retries internally** under `retry.RetryOnConflict` (`scaleToZero` re-reads the live StatefulSet on each attempt): the apply path and any autoscaler write the same object, so a routine 409 must not turn into a failed pass that leaves the role group half-deleted. The gray-delete annotate does not retry — its conflict is returned, that group's pass ends, and the next reconcile re-evaluates.
- **429 Too Many Requests**: mapped to a `*reconciler.RateLimitError` carrying `GenericReconcilerConfig.RateLimitRetryAfter` (default 10 s; `RoleGroupCleaner.WithRateLimitRetryAfter` sets it, and a cleaner built directly by a product falls back to the same 10 s). Unlike every other cleanup failure a 429 **aborts the whole pass immediately** — the remaining groups would only add to the requests the API server is already rejecting — and it propagates out of the reconcile loop as a rate-limit error rather than a cleanup error: throttling says nothing about the cluster's state, so it produces a plain `RequeueAfter` backoff instead of marking a healthy cluster `Degraded` (§4.8.4). It is a flat delay, not exponential backoff.
- **Status Synchronization**: cleanup and the CR Status are not updated atomically. The cleaner prunes the in-memory `Status.RoleGroups` for the groups it really deleted, and the reconcile's final status update persists it. If that write fails, the next reconciliation re-evaluates the same orphans — deletion is idempotent, so a repeated pass is safe.
- **Events**: when an `EventManager` is wired (`RoleGroupCleaner.WithEventManager`), each removed resource emits a `Normal`/`Deleted` event; without it deletions are recorded only in the operator log.

### 4.4.5 Boundary Handling

- **CR First Creation**: Status is empty, no orphaned resources, the reconciled role groups are recorded in Status.
- **Manual Resource Deletion**: Rely on idempotent deletion (`IsNotFound` short-circuit) to avoid errors, syncing Status in the next reconciliation.
- **Status Tampering**: Query cluster resources before deletion, and verify the ownerReference, so only resources this cluster actually owns are deleted.

## 4.5 Configuration Generator Module

### 4.5.1 Design Background

Big data components often require configuration files in various formats (e.g., XML for Hadoop, Properties for Kafka/Zookeeper, YAML for others). Hardcoding serialization logic for each product leads to duplication and inconsistency.

### 4.5.2 Core Implementation

- **Split format contract**: Emitting is the whole *required* contract; parsing is an optional capability layered on top.
  - `ConfigMarshaler` (**required**) — `Marshal(data map[string]string) (string, error)`. This is what `config.NewConfigGenerator`, `MultiFormatConfigGenerator.RegisterFormat` and `config.GetFormat(ConfigFormatType)` take and return. The framework's write path — the generators, `BaseRoleGroupHandler` and `ConfigMapBuilder` — never reads a generated file back, so a format a product only needs to *write* is complete with `Marshal` alone.
  - `ConfigUnmarshaler` (**optional**) — `Unmarshal(data string) (map[string]string, error)`. It is never required at registration: an emit-only adapter registers and generates like any other. The `Parse` paths upgrade the registered adapter to this interface at call time — the single place the package inspects a dynamic type — and a format that does not implement it fails with a `*config.UnsupportedParseError` naming the format (registered extension plus the adapter's Go type) and, where the caller knows one, the file. Matching that failure with `errors.As` is the stable check; a nil format instead yields the sentinel `config.ErrNoFormat`.
  - Every adapter shipped with the SDK implements both, asserted at compile time in `format.go`, so in practice `GetFormat`'s result can always parse as well as emit — even though its static type promises only `Marshal`.
- **FormatAdapter**: Adapter pattern implementation supporting common formats, selected by `config.GetFormat(ConfigFormatType)` (`xml`, `properties`, `yaml`, `env`, `ini`; unknown types fall back to properties). Adapters validate their input and return an error rather than emitting output the target parser would misread:
  - `XMLAdapter`: Converts key-value pairs into Hadoop-style `<property><name>...</name><value>...</value></property>` XML structure. It rejects text XML 1.0 cannot carry — C0 control characters other than tab/newline/carriage return, and non-UTF-8 bytes — naming the offending key, and writes a carriage return as `&#13;` because a parser normalizes literal line endings in content.
  - `PropertiesAdapter`: Converts key-value pairs into standard Java `.properties` format, escaping separators, comment markers and edge whitespace in keys and line continuations in values. On read it decodes `\uXXXX` escapes (surrogate pairs included) and drops layout whitespace that was not escaped, including the indentation of a continuation line.
  - `YAMLAdapter`: Emits a flat mapping through `gopkg.in/yaml.v3` (values that would otherwise parse as bool/number are quoted to stay strings); `Unmarshal` rejects a document that is not a flat mapping — and a duplicate key, which is invalid YAML — instead of returning partial data.
  - `EnvAdapter`: Formats as shell environment variable exports or .env file content. Keys must be valid shell variable names (`^[A-Za-z_][A-Za-z0-9_]*$`) — anything else is an error rather than corrupt output. A value is written bare only when every character is in the shell-inert allowlist `[A-Za-z0-9_@%+=:,./-]`; anything else — a command separator, a redirection, a subshell, a tilde, whitespace — is double-quoted with `$`, backticks, `\` and `"` escaped, so sourcing the file can never execute a config value. Newlines, carriage returns and tabs in values are written as dotenv-style `\n`/`\r`/`\t` escapes, so a multi-line value is not byte-faithful when a POSIX shell sources the file. On read, a single-quoted value is taken literally, as a POSIX shell does.
  - `INIAdapter`: Emits INI sections; rejects keys/values containing line breaks and keys containing `=`, `:` or a leading `[`, `#`, `;`.
- **Product Logging Engine** (`pkg/productlogging`): A dedicated, product-agnostic logging engine (separate from the config-format adapters above).
  - **Input**: The deep-merged CRD logging spec (e.g., `containers.coordinator.loggers.ROOT.level: DEBUG`), converted once into a framework-neutral `LogConfig`.
  - **Generators**: A registry of `LogFileGenerator`s renders framework-specific files (Logback XML, Log4j2 properties, Python logging) from the neutral model — including console/file appender thresholds and a bounded rolling file appender.
  - **Declaration**: Products declare per-container logging via `ContainerLogging` (container, framework, pattern). The framework owns the stable log file-path convention that the Vector sources glob — `<LogDir>/<lowercased container>/<container>.<framework suffix>`, where the suffix selects the edge parser (`.log4j.xml` for log4j/logback XMLLayout, `.log4j2.xml` for log4j2 XMLLayout, `.py.json` for python JSON lines) — so producers and the consumer cannot drift. Vector parses each format at the edge and normalizes every event to the stable schema (`.timestamp`/`.logger`/`.level`/`.message` + `.errors`, flat `.namespace`/`.cluster`/`.role`/`.roleGroup` metadata, and `.container`/`.file` extracted from the path).
  - **Vector coupling**: The rolling file appender is emitted only when the Vector agent is enabled — without a consumer there is no shared log volume to write to (see the Sidecar Injection module).
- **Integration**: Config generation happens on the **ConfigMap** path, not in the StatefulSet builder. `BaseRoleGroupHandler.ConfigGenerator` (a `config.MultiFormatConfigGenerator`) renders `MergedConfig.ConfigFiles` into `map[filename]content`, which `builder.ConfigMapBuilder.WithMergedConfig(mergedConfig, generator)` turns into the role group ConfigMap's `Data`. When no generator is set, the handler falls back to a deterministic properties-style rendering (keys sorted, separators and line breaks escaped). The StatefulSet only *mounts* the resulting ConfigMap.
- **Adapter selection**: `RegisterFormat` matches its string as a **file-name suffix**, so a whole file name (`server.properties`) is a legal registration. When several registrations match a name the **longest** wins, deterministically — selection must not depend on Go's map iteration order, or the same file renders differently between reconciles and the ConfigMap churns. A file matching nothing falls back to the properties adapter. Reading a file back through the same dispatch is `MultiFormatConfigGenerator.Parse(filename, content)`, which is the supported way to parse by file name rather than reaching into the adapter map.

### 4.5.3 Core Value

- **Unified Logic**: Centralizes the complexity of file format generation, avoiding repetitive implementation in each product operator.
- **Extensibility**: Easily supports new formats by implementing the `ConfigMarshaler` interface — one method, and only formats something actually reads back grow an `Unmarshal`.
- **Consistency**: Ensures generated configuration files adhere to standard formats and escaping rules.

## 4.6 Sidecar Injection Module

### 4.6.1 Design Background

Operations such as log collection (Vector), metric monitoring (JMX Exporter), and service mesh integration require injecting auxiliary containers into the business Pods. Manually configuring these sidecars in each CRD leads to configuration redundancy and maintenance difficulties.

### 4.6.2 Core Implementation

- **SidecarProvider Interface**: Defines the abstraction for sidecar injection. The pod spec is mutated in place and injection must be idempotent; a nil config means "provider defaults".
  - `Name() string`
  - `Inject(podSpec *corev1.PodSpec, config *SidecarConfig) error`
  - `Validate(ctx context.Context, c client.Client, namespace string) error` — checks the provider's external dependencies (e.g. a required ConfigMap key).
- **Injection Phases**: `SidecarManager.InjectAll` orders providers by `(phase, name)`, so injection is deterministic and a pod template does not re-render between reconciles. The phases are `SidecarPhaseProducer` (10), `SidecarPhaseDefault` (50) and `SidecarPhasePipeline` (90). A provider declares its phase by implementing `PhasedProvider`, or the caller pins one with `SidecarManager.RegisterWithPhase` (an explicit registration phase wins). This is what guarantees a pipeline provider — Vector, which must RW-mount the shared log volume onto the containers it collects from — runs after the producers that inject those containers.
- **Dependency Validation**: The `GenericReconciler` calls `SidecarManager.ValidateAll` for every role group **after** the ConfigMap, Services and extra resources are applied and **before** the StatefulSet. A registered, enabled provider whose `Validate` fails aborts the reconcile with a `reconciler.ValidationError` instead of producing pods that crash-loop on a broken mount. Validation only runs once a client and namespace are wired into the manager (the namespace is per CR).
- **Provider Placement**: Providers with config generation or external service discovery are placed in their own domain package. Trivial providers remain in `pkg/sidecar/`.
- **Standard Implementations**:
  - `VectorSidecarProvider` (in `pkg/vector/`): The **single owner of the shared log pipeline**. It creates the size-limited shared log `emptyDir`, RW-mounts it on the declared producer containers (so the product writes its log files there), mounts it on the Vector agent container (read-write: the agent is a native init container that starts before the producers and pre-creates each producer's per-container log directory, since log4j 1.x and Python's file handlers do not create parent directories), and injects the agent. Config generation (`RenderVectorConfig`) and aggregator discovery (`DiscoverAggregatorAddress`) are separate pure functions in the same package. It declares `SidecarPhasePipeline`, so it is always injected after the producer containers exist, and its `Validate` requires the target ConfigMap to exist **and to carry the `vector.yaml` key** — an agent mounted on a ConfigMap without its config would otherwise start and immediately fail.
  - `JMXExporterSidecarProvider` (in `pkg/sidecar/`): runs `jmx_prometheus_httpserver.jar` from `/opt/jmx_exporter` as its **own container** scraping the product's JMX port. It is not a java agent — that is a different mechanism, `constant.JMXJavaAgentOpt`, which the product puts on its own JVM command line (§4.1.5).
  - `OAuth2ProxySidecarProvider` (in `pkg/sidecar/`): the one **data-path** sidecar, which is why it is the only one carrying a readiness probe (§4.6.4).
  - `StaticContainerProvider` (in `pkg/sidecar/`): injects a container the product built itself, unchanged. `NewStaticContainerProvider(container)` is the escape hatch for a sidecar the framework has no opinion about — a statsd-exporter, a log shipper, a product-specific helper — and it is why the framework does not grow a provider per such container. Note what it deliberately does **not** do: its `Inject` ignores `SidecarConfig` entirely, so `SetProductImage` cannot fill in its image, and neither `DefaultSecurityContext()` nor `ApplyProbes` runs for it. The product sets those on the container it passes. An image-less container is caught at build time.

    ```go
    buildCtx.SidecarManager.Register(
        sidecar.NewStaticContainerProvider(corev1.Container{
            Name:            "statsd-exporter",
            // The product supplies it: SetProductImage does not reach a static container.
            Image:           buildCtx.ResolvedImage.Reference,
            Ports:           []corev1.ContainerPort{{Name: "metrics", ContainerPort: 9102}},
            RestartPolicy:   sidecar.SidecarRestartPolicy(), // the provider does not add this
            SecurityContext: sidecar.DefaultSecurityContext(),
        }),
        &sidecar.SidecarConfig{Enabled: true},
    )
    ```
- **Workflow**: The `GenericReconciler` registers the Vector provider — configured with the producer declarations (`RoleDeclaration.LogProducers`) and the shared log volume size (`RoleDeclaration.LogVolumeSize`, per role because log volume is: a DataNode writes far more than a JournalNode) — only when **all three** gates pass. Any one of them failing means the sidecar could not do its job, so the provider is not registered and the rest of the cluster keeps converging:
    1. **The agent is enabled** for the role group (`logging.enableVectorAgent`, after the role/role-group logging merge).
    2. **At least one producer is declared** in `RoleDeclaration.LogProducers`. An agent with nothing to collect would mount an empty pipeline; the reconciler logs the mismatch and skips, so enablement and producer declaration stay consistent in one place.

       **One list serves two jobs** — naming the pipeline's producers, and saying how each one's config file is rendered — and a producer opts out of the second by leaving its `Framework` empty. That is the seam for a product whose logging config file is product-owned: the producer joins the pipeline in full, the framework renders no file for it, and there is no ConfigMap key collision. Airflow's `log_config.py` is the case, and the reason is OWNERSHIP rather than impossibility — its content must import and patch Airflow's own `DEFAULT_LOGGING_CONFIG`, which is renderable but only by something carrying one product's knowledge, while the python renderer here is shared by every Python product and emits a standalone `dictConfig`. What makes the seam necessary rather than merely tidy is that the python renderer's default file name IS `log_config.py`, so a rendered file would take the key the product writes itself. The earlier design gave the two jobs two separately-addressable lists, which worked only because Go has no virtual dispatch — an implementation accident, discoverable by nobody, and a product that overrode the wrong one silently got no pipeline. The obligations the framework can then no longer meet are now **enforced** rather than documented: such a producer must set `LogFileName`, it must carry one of the known suffixes (that suffix selects Vector's edge parser), and its `Container` must name a real container in the assembled pod. The product asks `RoleGroupBuildContext.LogFileTarget(decl)` where the file goes rather than composing the path, so "Vector is off this cycle" resolves to console-only instead of an appender writing where nothing collects.
    3. **Something supplies `vector.yaml`.** The sidecar runs `vector --config <mount>/vector.yaml`, so it is only injected when that key will actually be written into the role group ConfigMap: either the **CR** implements `reconciler.VectorAggregatorProvider` (the framework then renders the file itself) or the **role** sets `RoleDeclaration.OwnsVectorConfig` (the product writes it). With neither, registering the provider would fail sidecar validation (§4.6.2, Dependency Validation) on every cycle and abort the whole cluster's reconcile over a product that is simply not wired for Vector. It is reported as the product-configuration mistake it is: a `Warning`/`VectorSidecarSkipped` event on the CR naming the role group and both ways to satisfy the gate, and the reconcile continues.

       **The framework owns this whole chain, and the resolved answer stays inside it.** Every input — `logging.enableVectorAgent` from the folded config, the producer list, and the `vector.yaml` source — is already the framework's, so it settles the question once and nothing re-derives it. The boolean is unexported; a product sees only the conclusion it can act on, `LogFileTarget`. Exposing the flag instead would make the product a second participant in a decision already made, and would leave it composing the log path itself — correct while Vector is on and silently wrong the moment it is off.

  The `BaseRoleGroupHandler` then invokes the `SidecarManager` after StatefulSet construction, and the manager injects Volumes, VolumeMounts and the sidecar containers themselves — into **`InitContainers`**, with `RestartPolicy: Always` (`sidecar.SidecarRestartPolicy()`). These are *native sidecars* (KEP-753 — on by default since Kubernetes v1.29, GA in v1.33), not ordinary containers, and the placement is load-bearing rather than cosmetic: the kubelet starts them before the main container and terminates them **after** it, which is what guarantees a log agent outlives the process it collects from.

  That guarantee used to be hand-rolled. Before #441 the Vector container ran a shell that backgrounded the agent and blocked on `inotifywait` for a shutdown file, and the product's main container was expected to `touch` that file on exit — a two-sided contract whose product half lived in `pkg/util/bash.go`. **Both halves were removed in the same commit**, and #494 replaced the mechanism with the native-sidecar ordering above. A product migrating from a pre-#441 operator should therefore **delete** its shutdown-file commands rather than look for a framework helper that emits them: nothing reads that file any more, and the ordering it approximated is now the kubelet's. The old design was also strictly worse in one case, since the write side fired whenever the main process exited — including a crash the kubelet was about to restart, which told the agent to shut down. Gate 3's first branch is the one the framework owns end to end: for a CR exposing the aggregator ConfigMap the reconciler resolves the aggregator address and generates `vector.yaml` into the role group ConfigMap — keeping producer, consumer, and config in lockstep in one place rather than spread across product operators. Within that branch, an empty `VectorAggregatorConfigMapName()` or an address that cannot be discovered is a hard error rather than a skip: the CR claimed the framework would supply the config, so shipping a Vector sidecar with no aggregator to send to would be worse than failing loudly.

### 4.6.3 Core Value

- **Decoupling**: Separates auxiliary functions (Logging/Monitoring) from core business logic.
- **Reusability**: Standard sidecars can be reused across HDFS, HBase, and other products without code duplication.
- **Consistency**: Ensures uniform configuration for logs and metrics across the entire platform.

## 4.7 Dependency Management Module

### 4.7.1 Design Background

Big Data systems often have strict startup dependency orders (e.g., Zookeeper -> BookKeeper -> Pulsar Broker). Starting a component before its dependencies are ready typically results in "CrashLoopBackOff" states, polluting logs and complicating troubleshooting.

### 4.7.2 Core Implementation

- **External Reference Validation is OPT-IN, declarative, and not derived from the Spec.** The SDK does **not** traverse the CR Spec looking for object references. A product declares what to check by setting the `GenericReconcilerConfig.Dependencies` hook:

  ```go
  Dependencies: func(cr *HdfsCluster) []reconciler.Dependency {
      return []reconciler.Dependency{
          {Kind: reconciler.DependencySecret, Name: cr.Spec.Kerberos.SecretName},
          {Kind: reconciler.DependencyConfigMap, Name: cr.Spec.ZookeeperConfigMap},
      }
  },
  ```

  - Supported kinds: `DependencyConfigMap` and `DependencySecret`. An empty `Dependency.Namespace` defaults to the CR's namespace; an empty `Name` is itself an error.
  - When the hook is nil (the default), **no dependency checking happens at all**.
- **Placement in the loop**: the check runs after the cluster `PreReconcile` extensions and **before any role is reconciled**, so a missing object aborts the cycle with a `DependencyValidation` reconcile error, which maps to the `Degraded` condition and a `Warning` event. No Pods are created for that cycle.
- **DependencyResolver**: the helper behind the hook. Its exported methods — `ValidateConfigMap`, `ValidateSecret`, `ValidateS3Connection`, `ValidateDatabaseConnection`, `ValidateZKConfig` (`ValidateZKConnection` is a deprecated alias that forwards to it), `ValidateEndpointFormat`, `ParseConnectionStrings` — are also usable directly from product code (e.g. from a `ClusterExtension.PreReconcile`) for checks richer than existence. Failures are `*DependencyError`, which products map to their own conditions.
  - `DependencyResolver.Validate(ctx, spec)` is a stable **no-op** kept for source compatibility; the reconcile flow no longer calls it. Do not rely on it to check anything.

### 4.7.3 Core Value
- **Stability**: Prevents cascading failures and "noise" from pod crash loops by declaring the prerequisites that must exist before startup.
- **Clarity**: Clearly indicates missing prerequisites in the CR Status.

## 4.8 Health Management Module

### 4.8.1 Design Background
Stateful systems distinguish between "Infrastructure Ready" (Pod Running) and "Service Ready" (Business logic active). For example, an HDFS NameNode might be running but stuck in SafeMode, or a Database might be performing recovery. The Operator status must reflect this business reality.

### 4.8.2 Health Check Mechanism

**The three workload conditions answer three different questions, and must not be derived from one number.** This is a design constraint, not an implementation detail:

| condition | question | derived from |
| --- | --- | --- |
| `Available` | *Can it serve?* | `readyReplicas >= desired` for every role group |
| `Progressing` | *Is it changing?* | a revision rollout or replica change in flight |
| `Degraded` | *Must a human look?* | **failure states**, never replica counts |

`Degraded` is the condition an operator alerts on, so it may only fire for something the operator cannot resolve on its own. Deriving it from replica counts makes it fire during every rolling update, every scale-up and every scale-down — planned changes that reduce ready replicas on purpose — and a signal that fires on every planned change is one nobody can alert on. `Available=False` is the honest report for those; alert on it with a duration.

Consequently `Degraded` is computed from **state, not time**: a pod wedged in `CrashLoopBackOff`, `ImagePullBackOff`, `InvalidImageName`, a `CreateContainer*`/`RunContainerError`, or a pod that cannot be scheduled; a role group whose StatefulSet cannot be read; a failing `ServiceHealthCheck`. Because these are states rather than elapsed times, a **stuck** rollout still reports `Degraded=True` — its pods are visibly failing — while a healthy rollout does not, with no progress-deadline machinery required. Transient startup states (`ContainerCreating`, `PodInitializing`) and pods already being deleted are deliberately excluded: they are what a healthy pod looks like on the way in and on the way out.

The health step runs once per reconcile, after orphan cleanup, and evaluates:
- **Workload Status**: per role group, `readyReplicas` against the desired replicas, producing `Available` and `Progressing`. The comparison is `>=`, so a role group mid-scale-down — briefly reporting MORE ready replicas than desired — is available, and one deliberately scaled to `replicas: 0` is available at 0.
- **Pod Failures**: one `List` of the cluster's pods (matched on `app.kubernetes.io/instance` + `managed-by`) producing `Degraded`, with a message naming the offending pods and their reasons, capped and with the remainder counted rather than silently truncated.
- **Service Availability**: the optional product-level `ServiceHealthCheck` (below), reported through the `ServiceHealthy` condition, and also setting `Degraded`.
- **ClusterOperation states are not faults.** `stopped` reports `Available=False` with `Degraded=False`, and `reconciliationPaused` reports the dedicated **`Paused`** condition with `Degraded=False` — pausing is an administrator's decision (a maintenance window, an investigation), and reporting it through the fault signal pages someone for a planned action. While paused the framework still *observes*: the pause freezes the resources, not the reporting, so `Available`/`Progressing` are re-evaluated from the live StatefulSets instead of being left at whatever the last running cycle wrote. The `ServiceHealthy` condition goes `Unknown` rather than keeping a stale verdict, because an active probe against a paused cluster is exactly what a pause asks the operator not to do.

- **Check Cadence**: `GenericReconcilerConfig.HealthCheckInterval` (default **120 s**) is the interval at which a successful reconcile requeues itself, which is what makes health re-evaluation periodic — see §4.8.4. A negative value disables the periodic wakeup.
- **Timeout**: `GenericReconcilerConfig.HealthCheckTimeout` (default **300 s**) is applied as a `context.WithTimeout` around the product-level `ServiceHealthCheck` call, so a hanging probe cannot pin a reconcile worker. A non-positive value disables the deadline. It does not bound the workload checks, which are ordinary client reads governed by the reconcile context.
- **Failure Handling**:
  - Replicas short of the desired count mark the CR **`Available=False`** — not Degraded. The message names the offenders: `Role groups with fewer ready replicas than desired: <role>/<group>, ...`.
  - A pod the operator cannot help marks the CR **Degraded**, naming it: `Pods requiring attention: <pod> (CrashLoopBackOff), ...`.
  - A `ServiceHealthCheck` that errors or reports unhealthy sets both `Degraded=True` and `ServiceHealthy=False` with the probe's message.
  - An error raised by the health step itself is logged and does **not** fail the reconcile; the state is re-evaluated on the next cycle.
  - If the controller itself encounters an internal error (a recovered panic), the Status is **NOT modified** — an internal fault says nothing about the cluster's actual state. The panic is instead returned as an error so the work queue retries with backoff (§4.13.2).

### 4.8.3 Core Implementation

- **Status Definition**: The SDK standardizes cluster status through Generic Conditions:
  - **Available**: every role group has at least as many ready replicas as its spec asks for.
  - **Progressing**: The cluster is rolling out a new version or scaling replicas.
  - **Degraded**: something is wrong that the operator cannot resolve on its own — a wedged or unschedulable pod, an unreadable StatefulSet, a failing application health check. Explicitly **not** "replicas are converging"; see §4.8.2.
  - **Paused**: `spec.clusterOperation.reconciliationPaused` is set. Carries `Degraded=False`: a pause is a decision, not a fault.
  - **ServiceHealthy**: The application-level check passed (e.g., SafeMode off, RegionServer registered).
  - **ReconcileComplete**: The SDK has finished the latest reconciliation loop successfully.
- **ServiceHealthCheck Interface**:
  - **Contract**: `CheckHealthy(ctx context.Context, client client.Client, namespace, name string) (bool, error)`. `common.ServiceHealthCheckFunc` adapts a plain function to it, and `common.CompositeHealthCheck` composes several.
  - **Mechanism**: The SDK hands the probe a `client.Client` and the cluster's namespace/name, so the natural implementation reads Kubernetes objects or queries the product's own HTTP/RPC endpoint. The framework does **not** provide an in-container exec handle — no `*rest.Config` is threaded into this path. A product that needs to exec inside a Pod constructs `util.NewExecUtil(client, restConfig)` itself from the config it already has in `main.go`.
  - **Example**: HDFS implements this by querying the NameNode's JMX/HTTP SafeMode endpoint; running `hdfs dfsadmin -safemode get` inside the container is possible only through a product-built `ExecUtil`.
  - **Registration**: `GenericReconcilerConfig.ServiceHealthCheck`.
- **Status Aggregation**: The SDK aggregates workload readiness and the business health check into the final `GenericClusterStatus`. Conditions carry `observedGeneration`, and `SetCondition` preserves `lastTransitionTime` when the status value does not actually change, so an idle cluster produces no condition churn.

### 4.8.4 Reconcile Requeue Policy

Watches only cover the resource kinds the framework owns (`StatefulSet`, `ConfigMap`, `Service`, `PodDisruptionBudget`, `ServiceAccount`, plus any GVK a product registers through `SetupWithManagerOptions`). Anything that changes **without** producing a watch event — a product `ServiceHealthCheck` whose remote side degrades, a gray-delete grace period running out — would otherwise never be re-evaluated. The reconcile loop therefore schedules its own wakeups:

- On the **success path**, `Reconcile` returns `ctrl.Result{RequeueAfter: d}` where `d` is the **earliest strictly-positive** of:
  1. `HealthCheckInterval` (default 120 s) — the periodic health cadence;
  2. the earliest pending wakeup returned by the cleaner (§4.4.2 step 7) — either a remaining **gray-delete deadline** (the time until the next orphaned role group becomes deletable) or the **drain poll interval** of a deletion already in flight, whichever comes first.

  A cleanup deadline sooner than the health cadence wins, so a deferred deletion runs on time and the multi-pass drain advances on its own clock rather than waiting for an unrelated watch event. When both are non-positive (`HealthCheckInterval` set negative and nothing pending), `d` is `0` — no periodic wakeup, purely watch-driven.
- On the **429 rate-limit path**, `Reconcile` returns `RequeueAfter: RateLimitRetryAfter` (default 10 s) with a nil error, so no `Degraded` condition and no error event are produced for throttling.
- On the **error path** (including a recovered panic), `Reconcile` returns the error and lets controller-runtime's rate limiter apply exponential backoff. No `RequeueAfter` is set — setting both is meaningless.
- On the **paused path** (`reconciliationPaused: true`), the loop returns `RequeueAfter: HealthCheckInterval` like a normal successful pass. A pause freezes the *resources*, not the reporting: `Available`/`Progressing` are re-evaluated from the live StatefulSets on every wakeup, and a pod that crash-loops during a maintenance window changes nothing in the CR and so produces no watch event of its own.

Because the cadence makes the operator write to the API server on a timer, the final status update is skipped when the computed status is deep-equal to the live one — a steady-state cluster costs one read, not a write, per wakeup.

### 4.8.5 Framework Metrics

The status conditions above are the operator's report to a human reading `kubectl describe`. They are not, by themselves, an alerting surface: turning a CR condition into a series needs kube-state-metrics configured for that product's CRD, which is a per-deployment step an operator author cannot take on the user's behalf.

The SDK therefore exports exactly two Prometheus series of its own, both from `pkg/reconciler/metrics.go`, registered on controller-runtime's `metrics.Registry` at init so they appear on the metrics endpoint an operator already serves with no wiring in `main.go`. Both are labelled `namespace` and `cluster`:

| Metric | Type | Meaning |
| --- | --- | --- |
| `operator_go_orphan_cleanup_pending` | Gauge | Role groups whose orphaned resources are not finished being reclaimed |
| `operator_go_orphan_drain_timeouts_total` | Counter | Orphaned StatefulSets deleted with pods still terminating |

Both design points are about not lying:

- the gauge is written on **every** pass, including at zero. A gauge only set while something is pending keeps publishing its last non-zero value after the teardown finishes, and an alert on it would never clear;
- a deleted CR's series are **removed**, not zeroed, on the `IsNotFound` branch of `Reconcile` — the only place the framework learns a cluster is gone, since it registers no finalizer and so has no teardown callback (§4.4.3). A zeroed series still publishes a series for something that does not exist.

**The boundary is deliberate, and this list is meant to stay short.** controller-runtime already exports reconcile counts, error counts and durations (`controller_runtime_reconcile_*`); re-exporting those per cluster would add cardinality and no information. What neither it nor kube-state-metrics covers is the orphan cleanup state machine (§4.4.2 step 7), because it is internal to this SDK: it spans many reconciles, records its progress in annotations on the objects it is retiring, and reports the rest in log lines. A role group stuck mid-teardown for three days produces no error, no failing reconcile and no condition transition — while its pods keep running and its PVCs keep costing. The drain-timeout counter marks the one event in that machine with no other surface at all, and the one that matters most: reaching it means a stateful product was denied the ordered shutdown the scale-to-zero existed to give it, so a pod was killed mid-flush.

## 4.9 Security Module

### 4.9.1 Design Philosophy
The SDK adopts a layered security strategy, addressing both **Infrastructure Security** (K8s access control, Pod context) and **Application Security** (identity, encryption). The core philosophy relies on "Privilege Separation" and "Automated Provisioning."

### 4.9.2 Infrastructure Security (Operator & K8s Layer)
- **ServiceAccount Provisioning**: The SDK gives every cluster a workload ServiceAccount, so Pods run with an identity distinct from the Operator's own. Its name is derived from the CR (`ServiceAccountResourceName(kind, cluster)`), not configured — see `docs/security.md` §3.1.
- **RBAC Integration**: `GenericReconcilerConfig.WorkloadRBACRules` lets a product declare the API permissions its **pods** need; the SDK maintains the namespaced Role and RoleBinding against the derived workload ServiceAccount, adhering to the Principle of Least Privilege. Cluster-scoped RBAC is out of scope — a namespaced CR cannot controller-own it. See `docs/security.md` §3.2.
- **Pod Security Context**: Enforces secure defaults for Pod execution (e.g., non-root users, fsGroup controls) to prevent container breakouts.

### 4.9.3 Application Security (Workload Layer)
- **Zero-Touch Secret Management**: Leverages `secret-operator` and the `SecretClass` abstraction to inject sensitive data (Kerberos Keytabs, TLS Certificates) via CSI volumes, preventing the Operator from directly handling secrets.
- **Automated Identity**: Supports backend mechanisms like `AutoTLS` (for mTLS) and `KerberosKeytab` (for Hadoop ecosystem identity) without manual intervention.

> **Note**: For detailed architecture, backend mechanisms, and workflow regarding Application Security and SecretClass, please refer to the dedicated security documentation: [Operator-Go Security Architecture](security.md).

### 4.9.4 Generate-Once Secrets

Everything the framework applies is idempotent against a desired state — the handler rebuilds it every reconcile and the apply path overwrites the live object. A **generated** secret is the exact opposite: rewriting it is the failure. The oauth2-proxy session cookie key signs every session the proxy trusts, so a fresh value on each pass rolls the pods and logs every user out.

`reconciler.EnsureGeneratedSecret` is the ensure-helper for that shape, alongside `EnsureDiscoveryConfigMap`. It creates the Secret with generated values when absent, fills in only **missing** keys when it exists, and **never rewrites an existing value**; it sets a controller owner reference, and tolerates the `IsAlreadyExists` of a concurrent reconcile by re-reading — one generated value, whoever generated it.

Filling a missing key is a deliberate choice rather than an oversight. Sidecar providers fail the reconcile on a missing key (`OAuth2ProxySidecarProvider.Validate` does), so a Secret that lost one — a partial restore from backup, a hand-edit — would wedge the cluster with no recovery short of deleting the whole Secret, which rotates every *other* key too and logs out every user to fix one. Filling only what is absent keeps the blast radius at the key that was actually lost.

The Secret is **not** created from the sidecar provider's `Validate`: a validation hook that creates objects is a side effect in the one step whose job is to have none. Products call the helper from a `ClusterExtension` `PreReconcile` hook, mirroring how discovery ConfigMaps are published from `PostReconcile`.

## 4.10 Network Access & Service Exposure Module

### 4.10.1 Design Background
Big Data services often require complex network exposure strategies (e.g., UIs need LoadBalancers, internal RPCs need ClusterIP, stateful nodes need predictable DNS). Hardcoding `Service` resources in the Operator is rigid and limits deployment adaptability (e.g., On-Prem vs Cloud).

### 4.10.2 Core Implementation
- **Listener Operator Integration**: The SDK delegates network exposure to `listener-operator`, effectively decoupling "Service Definition" from "Service Exposure".
- **Concept: ListenerClass**:
  - Similar to StorageClass, it defines the exposure policy abstractly.
  - **cluster-internal**: Creates a standard ClusterIP Service for intra-cluster communication.
  - **external-stable**: Creates a LoadBalancer/NodePort with stable external IPs (crucial for Kafka/HDFS clients).
  - **external-unstable**: Creates a LoadBalancer with dynamic IPs for ephemeral access.
- **Workflow (CSI-Based)**:
  1. **Declaration**: The Product CR defines that a Role needs a listener by referencing a `ListenerClass`. The operator registers it with `listener.NewVolume(volumeName, class)` (optionally `.WithListenerName(...)`) on a `ListenerProvisioner`.
  2. **Injection**: The SDK declares a **generic ephemeral volume** on the Pod template — `Ephemeral.VolumeClaimTemplate` with the `listeners.kubedoop.dev` StorageClass, `ReadWriteOnce`, a 1Mi request, and the listener annotations (`listeners.kubedoop.dev/class`, and `listeners.kubedoop.dev/listenerName` when set) on the *template's* metadata. The SDK does **not** create a `PersistentVolumeClaim` object and does **not** create a Kubernetes `Service`. Kubernetes' ephemeral-volume controller materializes one pod-owned PVC per Pod, so the operator needs no PVC create permission and the PVC's lifecycle is bound to its Pod.
  3. **Realization**: The `listener-operator`'s CSI driver intercepts the Pod mount, automatically provisions the required Kubernetes `Service`, and projects the resulting public address/port into the Pod's filesystem (readable through `ListenerProvisioner.Path()`/`MustPath()`).

> **Note**: there is no listener *scope* annotation. Scope selection is a `secret-operator` concept (see `pkg/security`), not a listener one; `pkg/listener` emits only the class and listener-name annotations.

### 4.10.3 Core Value
- **Decoupling**: Developers define *logical* ports (e.g., "WebUI"), while Ops define *physical* exposure strategies via `ListenerClass`.
- **Dynamic Address Awareness**: Applications can read their own external address (e.g., public LoadBalancer IP) from the mounted file, solving the "NAT Advertisement" problem common in distributed systems like Kafka and Zookeeper.

## 4.11 Operational Management Module (ClusterOperation)

### 4.11.1 Design Background

Day-2 operations (maintenance, debugging, emergency stop) require safe and predictable controls over the Operator's behavior. Direct manipulation of underlying resources (e.g., deleting StatefulSets manually) is risky and can conflict with the Operator's reconciliation loop.

### 4.11.2 Core Capabilities

- **Reconciliation Pause (`reconciliationPaused: true`)**:
  - **Mechanism**: The Reconciler checks this flag at the very beginning of the loop, before any resource mutation (ServiceAccount provisioning, PreReconcile extensions, role reconciliation). If true, it surfaces the dedicated **`Paused`** condition — with `Degraded=False`, because a maintenance window is not a fault (§ status conditions) — then skips all resource reconciliation for that loop, leaving managed resources untouched while still re-reading the live workloads so the health conditions stay current.
  - **Use Case**: Allows admins to manually modify underlying K8s resources (e.g., patching a StatefulSet for debugging) without the Operator reverting changes immediately.
- **Graceful Stop (`stopped: true`)**:
  - **Mechanism**: `BaseRoleGroupHandler.buildStatefulSet` forces the replica count to 0 for every RoleGroup — in the *handler*, not the reconciler, which matters to a product that implements `RoleGroupHandler` directly and must reproduce it (§4.1.5).
  - **Persistence**: Crucially, **PVCs (Persistent Volume Claims) and ConfigMaps are PRESERVED**. This ensures data safety while freeing up compute resources.
- **Graceful Shutdown**:
  - **Mechanism**: The `gracefulShutdownTimeout` field configures the `terminationGracePeriodSeconds` of the Pod.
  - **Lifecycle Hooks**: `preStop` hooks are opt-in on the product side — `StatefulSetBuilder.WithPreStopHook(command)` / `WithPreStopHTTPGet(path, port)` inject application-specific decommissioning logic (e.g., `hdfs dfsadmin -saveNamespace`) before SIGTERM. The framework does not add one by default.

### 4.11.3 Core Value

- **Safety**: Provides "Emergency Brakes" for operators.
- **Flexibility**: Enables manual intervention without fighting the controller.

## 4.12 Connection & Resource Binding Module

### 4.12.1 Design Background

Big Data applications typically require connections to external infrastructure:
- **Object Storage**: S3/GCS/Azure Blob for data persistence (e.g., Hive Warehouse, spark-logs).
- **Metadata Databases**: MySQL/Postgres for storing application metadata (e.g., Hive Metastore, DolphinScheduler).
Hardcoding these connections in `configOverrides` is error-prone and leaks credentials.

### 4.12.2 Core Implementation

- **Unified Types** (`pkg/apis/s3/v1alpha1`, `pkg/apis/database/v1alpha1`):
  - `S3Connection` / `S3Bucket`: Standard CRDs for Endpoint, Region, TLS, path-style access, bucket name, and a credentials `SecretClass` reference. Both are usable **inline or by reference** from a product CR.
  - `DatabaseConnection`: Standard CRD for Host, Port, driver class, database name, and a credentials reference.
- **S3 Resolution and Rendering** (`pkg/s3`) — **opt-in helpers, not an automatic pass**:
  - `s3.ResolveConnection(ctx, client, ns, inline, reference)` and `s3.ResolveBucket(...)` collapse the inline-or-reference pair into a flat `ConnectionInfo` / `BucketInfo`.
  - `ConnectionInfo.S3AProperties()` returns the Hadoop S3A client properties — `fs.s3a.endpoint`, `fs.s3a.path.style.access`, `fs.s3a.connection.ssl.enabled`, and `fs.s3a.endpoint.region` when a region is set. `BucketInfo.S3AURI(prefix)` renders an `s3a://<bucket>/<prefix>` URI.
  - **The product merges the returned map into its own config files** (prefixing where the engine requires it, e.g. `spark.hadoop.`). The `ConfigGenerator` knows nothing about connection objects — it is a pure `map → XML/Properties/YAML/Env/INI` serializer.
  - **Access and secret keys are never rendered as configuration properties.** `ConnectionInfo.CredentialsProvisioner(volumeName)` returns a `security.SecretProvisioner` (it satisfies `reconciler.VolumeProvider`) that mounts the credentials as a `secret-operator` CSI volume under `/kubedoop/secret/<volumeName>`; the container reads them via `s3.CredentialsExportScript`, which exports `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`.
  - **`pathStyle` defaults to `false`, and adopting `S3AProperties()` is therefore a behaviour change.** `fs.s3a.path.style.access` renders the user's `spec.pathStyle`, whose CRD default is `false` — virtual-host addressing, which is right for AWS S3 and wrong for most self-hosted backends. **MinIO serves path-style only**: with virtual-host addressing the client resolves `<bucket>.<host>` (`warehouse.minio` in-cluster) and gets NXDOMAIN. Every product implementation this helper replaces pinned the key to `true` for exactly that reason, so a product migrating onto `S3AProperties()` silently flips the addressing mode for every existing cluster whose `S3Connection` does not say `pathStyle: true` — and the failure surfaces at first bucket access, not at admission. **Adding `pathStyle: true` to those `S3Connection` resources is part of the migration, not a follow-up.** Honouring the field rather than pinning it is deliberate (a value the user wrote must reach the client, and AWS has deprecated path-style); the trap is the silent default, not the rendering.
- **DatabaseConnection has no rendering support.** The SDK ships the CRD types and `DependencyResolver.ValidateDatabaseConnection` (a shape check on host and credentials `SecretClass`) — no JDBC-URL builder, no credentials volume helper. Products build the connection string themselves. *(Not yet implemented: a `pkg/database` resolver mirroring `pkg/s3`.)*
- **Credential Resolution**: Credentials are referenced as a `SecretClass` and delivered through the CSI volume described above, so the Operator never reads the secret material itself. See [security.md](security.md).

### 4.12.3 Core Value

- **Decoupling**: The product's CRD accepts a stable, typed connection description instead of a pile of `configOverrides`.
- **No Credential Leakage**: Credentials travel over CSI into the Pod; they are never written into a ConfigMap or a rendered config file.

## 4.13 Error Handling & Resilience Module

### 4.13.1 Design Background

Distributed systems and Kubernetes Controllers face unpredictable failures: network flakiness, API throttling, resource conflicts, and logical errors. A robust SDK must ensure that errors are handled gracefully, ensuring the Controller remains stable (no crashes) and provides feedback (Status updates) without manual intervention.

### 4.13.2 Core Strategies

- **Reconciler Resilience**:
  - **Panic Recovery**: A top-level `recover()` catches panics inside the reconciliation loop, so a bug in one CR handler cannot crash the operator process. The recovered panic is logged with its stack, emitted as a `Warning`/`ReconcilePanic` event on the CR (when the CR was already fetched), and **returned as an error** — swallowing it would report the cycle as successful and the work queue would neither retry nor back off. On this path the **CR Status is deliberately left untouched**: an internal fault is not evidence about the cluster's actual state.
  - **Exponential Backoff**: Returning an error hands the request back to controller-runtime's rate limiter, which requeues with exponentially increasing delay. The SDK adds no backoff of its own; the one flat delay it does apply is the 429 path (§4.8.4).

- **Pre-flight Validation (fail fast, before the workload)**:
  - **Role names**: the handler's configured role names are checked against the roles actually present in the CR Spec. A handler configured for a role the CR does not declare is a wiring mistake that would otherwise silently produce nothing — it is reported as an error instead.
  - **Declared dependencies**: `GenericReconcilerConfig.Dependencies` is verified before any role is reconciled (§4.7.2).
  - **Sidecar dependencies**: each enabled provider's `Validate` runs before the StatefulSet is applied, failing with a `ValidationError` rather than creating pods that crash-loop (§4.6.2).
  - **Malformed `podOverrides`**: a layer that cannot be decoded or patched is recorded on `MergedConfig.PodOverrideErrors` and surfaced as a `Warning` event; the layer is skipped rather than silently dropped without trace.

- **Concurrency Control**:
  - **Optimistic Locking on status writes**: the status write is issued from the in-memory CR without re-fetching it first, because a re-fetch would replace the whole status stanza and discard the product-specific fields an extension hook computed during this cycle (`ClusterInterface` exposes only the embedded generic status, which the framework mutates through the pointer `GetStatus` returns — there is no setter that could replace the stanza wholesale). On a 409 only the `resourceVersion` is refreshed — through the uncached `APIReader` when one is configured, since the informer cache has by definition not seen the competing write — and the write is retried with this cycle's status unchanged. That is last-writer-wins: it is correct because the controller is the sole writer of its own CR's status, and it does mean a status field written by a *different* actor between the read and the write is overwritten. A `NotFound` (the CR was deleted mid-cycle) is treated as success. The *cleaner* applies the same `RetryOnConflict` treatment to its own contended write, the scale-to-zero of an orphaned StatefulSet — see §4.4.4.
  - **Idempotency**: All side-effect operations (Create/Update/Delete) are designed to be idempotent. A retry after a partial failure is safe and will not result in duplicated resources.

- **Extension Fault Tolerance**:
  - **Fail-Fast by default**: a `PreReconcile`/`PostReconcile` failure aborts the reconciliation, preventing a partially configured (e.g. insecure) deployment. A single registration can opt out with `common.WithStopOnError(false)`; its failure is still returned.
  - **Error Propagation**: Errors returned by Extensions are wrapped in `*ExtensionError` and propagated to the CR Status.

- **Status Visibility**:
  - **Condition Mapping**: Top-level errors are automatically mapped to the `Degraded` Condition in `GenericClusterStatus`.
  - **Reasoning**: The `Reason` and `Message` fields of the Condition are populated with the error details, allowing users/admins to diagnose issues (e.g., "DependencyMissing: Zookeeper secret not found") via `kubectl get`.
  - **No churn**: the status write is skipped when the computed status is deep-equal to the live one, so the periodic requeue cadence (§4.8.4) does not translate into a stream of no-op writes.

## 4.14 Event Management Module

### 4.14.1 Design Background

K8s Events provide a chronological log of significant occurrences within the cluster. Unlike Status Conditions (which represent the *current* state), Events record *what happened* (transitions, errors, actions). Systematic event recording is crucial for troubleshooting "Why did it fail 10 minutes ago?".

### 4.14.2 Core Implementation

- **Unified Recorder**: The SDK encapsulates the Kubernetes `EventRecorder` in an `EventManager`, held as a field on the reconciler and handed to the `RoleGroupCleaner`. It is **not** placed in the `context` — a hook or handler that wants to emit events constructs its own `NewEventManager(recorder, scheme)`.
- **Automated Lifecycle Events**:
  - **Resource Operations**: `Created` / `Updated` / `Deleted` `Normal` events whenever the framework applies or reclaims a sub-resource (StatefulSet, Service, ConfigMap, PDB), giving auditability without boilerplate. Orphan cleanup emits `Deleted` per removed resource once the cleaner has an `EventManager` (`RoleGroupCleaner.WithEventManager`). Each message names the object's **Kind**, resolved through the scheme: the typed objects `pkg/builder` produces carry no `TypeMeta`, so the kind cannot be read off the object, and it is exactly the disambiguator between a role group's Service, its headless Service and its metrics Service.
  - **There are no reconcile start / completion events.** The framework emits nothing at the beginning or end of a successful pass; progress is reported through **status conditions**, which is what a controller should be watched on. Do not build alerting on a "reconcile completed" event.
- **Error Integration**: an error that reaches the top of the loop (including from Extensions) sets `Degraded` **and** emits a `ReconcileError` `Warning` carrying the error text.
- **Degraded-input warnings**: some inputs are bad but not fatal, and they get a `Warning` of their own rather than being dropped silently. The complete vocabulary the framework emits: `ReconcileError`, `ReconcilePanic` (a recovered panic), `PodOverrideIgnored` (a `podOverrides` layer that fails to decode or patch — `MergedConfig.PodOverrideErrors`), `UnusedRoleDeclaration` (the product declares a role this cluster does not use), `ImmutableFieldIgnored` (a desired change to a preserved immutable field), `VectorSidecarSkipped` (the agent is enabled but nothing supplies `vector.yaml`), plus the three resource-operation `Normal` events above.
- **Product-facing helpers**: `EmitWarningEvent`, `EmitNormalEvent`, `LogAndEmitError` and `LogAndEmitInfo` are available to extensions and product code. The framework calls only the first two.

### 4.14.3 The precondition: `core/events` `create;patch`

Everything in §4.14 depends on a permission the SDK cannot declare for the operator that embeds it,
and whose absence announces itself nowhere useful. **The operator's own ClusterRole must carry
`+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch`** (see `security.md` §3.3).
`patch` is not conventional slack: a repeated event is aggregated onto the existing object, so
client-go patches rather than creates it.

Without the grant, client-go treats the 403 as permanent — it logs `Server rejected event (will not
retry!)` and **discards** the event. Emission is fire-and-forget onto a broadcaster channel, so no
error reaches the reconcile, the status is untouched and the pass reports success. The rejection is
not literally silent (the log line carries the whole event, because `*v1.Event` renders itself), but
it is *misrouted*: it leaves `kubectl describe` and `kubectl get events` entirely, and it does not
go through the operator's own structured logger — controller-runtime never redirects klog, so it
lands on stderr in klog's text format, rate-limited to roughly 25 and then one per 300s per object.

The cost is not evenly spread across §4.14.2's vocabulary. Five of the six `Warning` events survive
the loss because the same information exists elsewhere: `ReconcileError` also sets `Degraded`,
and `ReconcilePanic`, `PodOverrideIgnored`, `VectorSidecarSkipped` and `UnusedRoleDeclaration` each
have a paired log line. **`ImmutableFieldIgnored` has neither** — no log line, no status condition —
so it is the one framework warning whose information exists *only* as an event. It is also the one
whose loss reintroduces a known data defect: it exists because preserving an immutable field
silently is what let a storage resize be accepted, reported as `ReconcileComplete=True`, and never
applied.

### 4.14.4 Core Value

- **Auditability**: Provides a trace of actions taken by the Operator.
- **Troubleshooting**: Warning events appear directly in `kubectl describe`, giving immediate
  visibility into failures — **provided** the operator holds the grant in §4.14.3.

## 4.15 Constants Architecture Module

### 4.15.1 Design Philosophy

The SDK uses a **hybrid constants architecture** that separates cross-cutting constants from domain-specific constants:

- **Cross-cutting constants** (`pkg/constant/`): Shared across all packages — domain name, directory paths, Kubernetes labels, and operational labels (enrichment, restarter).
- **Domain-specific constants** (`pkg/listener/`, `pkg/security/`): Constants meaningful only within their domain — CSI driver names, annotation keys, format/scope types.

All labels, annotations, and CSI-related constants in the SDK derive from a single domain constant:

```go
// pkg/constant/domain.go
const KubedoopDomain = "kubedoop.dev"
```

Domain packages derive their constants from this root:

```go
// pkg/listener/volume_builder.go (constants)
const ListenerAPIGroup = "listeners." + constant.KubedoopDomain

// pkg/security/secret_class.go
const SecretAPIGroup = "secrets." + constant.KubedoopDomain
```

This ensures changing the organization domain requires updating only one constant.

### 4.15.2 Constant Categories

**`pkg/constant/domain.go`** — Organization domain:
- `KubedoopDomain` (`"kubedoop.dev"`)

**`pkg/constant/path.go`** — Directory paths:
- `KubedoopRoot` (`"/kubedoop/"`)
- Derived paths: `KubedoopKerberosDir`, `KubedoopTlsDir`, `KubedoopListenerDir`, `KubedoopJmxDir`, `KubedoopSecretDir`, `KubedoopDataDir`, `KubedoopConfigDir`, `KubedoopLogDir`, `KubedoopConfigDirMount`, `KubedoopLogDirMount`

**`pkg/constant/label.go`** — Kubernetes recommended labels:
- `LabelKubernetesComponent`, `LabelKubernetesInstance`, `LabelKubernetesName`, `LabelKubernetesManagedBy`, `LabelKubernetesRoleGroup`, `LabelKubernetesVersion`
- `MatchingLabelsNames()` — returns label keys for selector matching
- Enrichment labels: `LabelEnrichmentEnable`, `LabelEnrichmentNodeAddress`

**`pkg/constant/restarter.go`** — Restarter policy:
- `LabelRestarterEnable`, `AnnotationSecretRestarterPrefix`, `AnnotationConfigMapRestarterPrefix`, `LabelRestarterExpiresAtPrefix`

**`pkg/listener/`** — Listener operator constants:
- `ListenerAPIGroup`, `ListenerStorageClass`, `CSIDriverName`
- Annotations: `ListenerClassAnnotation`, `AnnotationListenerName` (there is no listener scope annotation — scope is a `secret-operator` concept)
- Types: `ListenerClass` (cluster-internal, external-stable, external-unstable)
- Provisioner: `ListenerProvisioner` (declarative CSI listener volume registration with `RegisterVolume()`, `Volumes()`/`VolumeMounts()`, `AutoInject()`, `Path()`/`MustPath()`; the `listener-operator` creates the Service, not the SDK)

**`pkg/security/`** — Secret operator constants:
- `SecretAPIGroup`, `SecretStorageClass`, `CSIDriverName`
- Annotations: `SecretClassAnnotation`, `SecretClassScopeAnnotation`, etc.
- Labels: `LabelSecretsNode`, `LabelSecretsPod`, `LabelSecretsService`
- Types: `SecretFormat` (tls-pem, tls-p12, kerberos), `SecretScope` (pod, node, service, listener-volume)
- Provisioner: `SecretProvisioner` (declarative CSI secret volume registration with `TLS()`, `KerberosVolume()`, `Custom()` constructors)

### 4.15.3 Core Value

- **DRY**: All platform constants derive from `KubedoopDomain` — one change propagates everywhere.
- **Discoverability**: Cross-cutting constants in `pkg/constant/`, domain constants alongside their domain code.
- **Type Safety**: Domain types like `ListenerClass`, `SecretFormat`, `SecretScope` prevent invalid values at compile time.
- **Go Idiomatic**: Package named `constant` (singular, per Go convention), MixedCaps naming, `const` blocks for grouping.

# 5. Application of Design Patterns

The core design of the SDK reuses multiple classic design patterns to enhance architectural flexibility and maintainability. This section provides detailed explanations of each pattern's application within the SDK.

## 5.1 Interface Segregation Pattern

### 5.1.1 Pattern Overview

The Interface Segregation Principle (ISP) states that clients should not be forced to depend on interfaces they do not use. The SDK applies this by splitting functionality into fine-grained, focused interfaces.

### 5.1.2 Application in SDK

- **`ClusterInterface`**: `client.Object` plus two methods — `GetSpec()` and `GetStatus()`. Everything a Kubernetes object already answers is inherited from the embedded `client.Object`; the only thing the SDK asks a product to write is the projection of its spec and status onto the framework's generic shapes.
- **`ClusterResource[T ClusterInterface]`**: `ClusterInterface` plus `DeepCopy() T`. It is a *constraint*, used only as `GenericReconciler`'s type parameter, and it is satisfied by controller-gen's generated code rather than by anything hand-written.
- **`RoleGroupHandler`**: Defines the `BuildResources()` contract that product operators implement to produce RoleGroup-specific Kubernetes resources. Role-level information is *passed in* through `RoleGroupBuildContext` rather than pulled through a role interface the product would have to implement — segregation taken to its limit: the role level costs a product zero methods.
- **`RoleExtension` / `RoleGroupExtension`**: Define the Pre/PostReconcile hooks products use to customize behavior at role and role group level.
- **`ServiceHealthCheck`**: Defines health check contract for business-level readiness.

### 5.1.3 Benefits

- **Reduced Implementation Cost**: Product developers implement only the interfaces they need.
- **Interface Clarity**: Each interface has a single, well-defined responsibility.
- **Testability**: Smaller interfaces are easier to mock for unit testing.

### 5.1.4 Example

```go
// The SDK interface itself: client.Object, plus the two projections.
type ClusterInterface interface {
    client.Object

    GetSpec() *v1alpha1.GenericClusterSpec
    GetStatus() *v1alpha1.GenericClusterStatus
}

// The constraint GenericReconciler parameterises over.
type ClusterResource[T ClusterInterface] interface {
    ClusterInterface

    DeepCopy() T
}

// A product CR implements ClusterInterface; the other interfaces are opt-in.
// +kubebuilder:object:root=true
type HdfsCluster struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec              HdfsClusterSpec   `json:"spec,omitempty"`
    Status            HdfsClusterStatus `json:"status,omitempty"`
}

// Embedding metav1.TypeMeta and metav1.ObjectMeta supplies every metadata accessor, and
// `make generate` emits DeepCopyObject() (completing client.Object) and DeepCopy()
// *HdfsCluster (completing ClusterResource). So the CR writes exactly two methods:
func (h *HdfsCluster) GetSpec() *v1alpha1.GenericClusterSpec { return &h.Spec.GenericClusterSpec }
func (h *HdfsCluster) GetStatus() *v1alpha1.GenericClusterStatus {
    return &h.Status.GenericClusterStatus
}
```

> The CR must also be registered with the manager's scheme (`SchemeBuilder.Register(&HdfsCluster{}, &HdfsClusterList{})`): the reconciler reads the fetched object into the CR itself, so an unregistered type fails at `client.Get` with "no kind is registered for the type".

> A `GetSpec()` implementation that builds a fresh `GenericClusterSpec` on every call is legal but subtle: the reconcile loop snapshots the spec once per cycle, so in-memory mutations made after that snapshot are not observed consistently (see §4.2.5). Returning a pointer into the CR is the simpler contract.

## 5.2 Strategy Pattern

### 5.2.1 Pattern Overview

The Strategy Pattern defines a family of algorithms, encapsulates each one, and makes them interchangeable. The SDK uses this pattern extensively for extension points and configurable behaviors.

### 5.2.2 Application in SDK

- **Extension Interfaces**: Products implement `ClusterExtension[CR]`, `RoleExtension[CR]`, or `RoleGroupExtension[CR]` to inject custom reconciliation logic.
- **ConfigMarshaler Interface**: Different configuration serializers (XML, Properties, YAML, Env, INI) implement the same one-method interface.
- **SidecarProvider Interface**: Different sidecar injectors (Vector, JMX Exporter) follow a common contract.

### 5.2.3 Benefits

- **Flexibility**: Strategies can be swapped at runtime without modifying the SDK core.
- **Open/Closed Principle**: New strategies can be added without modifying existing code.
- **Isolation**: Each strategy is isolated, making it easier to test and maintain.

### 5.2.4 Example

```go
// The required half of the strategy: emitting is the whole contract of a format.
type ConfigMarshaler interface {
    Marshal(data map[string]string) (string, error)
}

// The optional half, discovered by interface upgrade on the Parse paths only.
type ConfigUnmarshaler interface {
    Unmarshal(data string) (map[string]string, error)
}

// Concrete strategies (all five implement both halves)
type XMLAdapter struct{}        // Hadoop XML format
type PropertiesAdapter struct{} // Java .properties format
type YAMLAdapter struct{}       // YAML format
type EnvAdapter struct{}        // shell / .env format
type INIAdapter struct{}        // INI format

// Context uses the strategy. It stores only the required half; Parse upgrades the value
// and returns *UnsupportedParseError when the format cannot read its own output back.
type ConfigGenerator struct {
    format ConfigMarshaler
}
```

## 5.3 Template Method Pattern

### 5.3.1 Pattern Overview

The Template Method Pattern defines the skeleton of an algorithm in a base class, letting subclasses override specific steps without changing the algorithm's structure.

### 5.3.2 Application in SDK

- **`ClusterReconciler`** (SDK: `GenericReconciler`): Defines the reconciliation workflow (PreReconcile → Reconcile → PostReconcile) as a fixed template.
- **Extension Hooks**: Products customize behavior by implementing extension interfaces at specific hook points.
- **Resource Construction**: `StatefulSetBuilder` follows a template for constructing K8s resources.

### 5.3.3 Reconciliation Template

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

**Resource Application Order (per RoleGroup)**

Within step 3, resources are applied in a strict dependency order:

```
ConfigMap → HeadlessService → Service → ExtraResources → StatefulSet → PDB → MetricsService
```

The rationale follows Kubernetes resource dependency rules:

1. **ConfigMap**: Applied first because Pods reference ConfigMaps as volume mounts or environment sources. The configuration data must exist before any Pod starts.
2. **HeadlessService**: A StatefulSet requires a `serviceName` pointing to a headless Service. Kubernetes uses it to create stable, predictable DNS entries (`pod-0.svc.ns.svc.cluster.local`) for inter-pod communication. It must exist before the StatefulSet is created.
3. **Service** (client-facing): Created before the StatefulSet so that client endpoints are available as soon as Pods become ready.
4. **ExtraResources** (product-specific objects): Applied before the StatefulSet because they are typically pod-scheduling prerequisites — e.g. a Listener CR that the pods reference through an ephemeral CSI volume (see `RoleGroupResources.ExtraResources`). **Teardown mirrors this**: an orphaned role group's extras are deleted immediately *after* its StatefulSet, so nothing a pod might still need is reclaimed while a pod could still exist. Discovery is by the role group's identity labels plus this CR's controller owner reference, over the kinds the product declared in `SetupWithManagerOptions.ExtraOwns` — the same list that gives them watches, so the two cannot drift. Extras that carry no role group labels are undiscoverable in principle and are left to owner-reference GC.
   Between this step and the StatefulSet, the registered sidecar providers' `Validate` checks run (§4.6.2) — late enough that the ConfigMap and any extras they depend on already exist, early enough that a failure never produces a Pod.
   **The position is fixed, and there is no per-extra ordering control.** The only ordering an extra has ever needed is "before the thing that would fail without it", and every extra is already there. An "after the workload" phase would buy nothing a `PostReconcile` hook does not, while doubling the states the teardown has to mirror: the safety property above — nothing a pod might need is reclaimed while a pod could exist — holds because there is exactly one extras position to invert.
5. **StatefulSet**: Applied after all its dependencies (configs, DNS, extras) are in place. The StatefulSet controller then creates Pods in ordinal order.
6. **PDB** (PodDisruptionBudget): Applied after the workload, as it references existing Pods. It enforces availability guarantees during voluntary disruptions once the workload is running.
7. **MetricsService**: Applied last; it only exposes already-running Pods to Prometheus discovery and nothing depends on it.

Orphan cleanup uses its own order — `PDB → StatefulSet → ConfigMap → Service → headless Service → metrics Service` (see §4.4.3) — which is **not** the exact inverse of this creation order. The two orders answer different questions: creation sequences prerequisites before dependants, while deletion removes the PDB first so it cannot block pod eviction and drops the Services last.

**Resource Application Semantics (create-or-update)**

Applying a resource is not create-only: when the resource already exists, `applyResource` updates the live object to the handler-built desired state on every reconcile, so CR spec changes (replicas, config overrides, ports, ...) propagate to existing resources (issue #526). The update rules live in `copyDesiredState` (`pkg/reconciler/apply.go`):

- **Labels** are framework-owned and replaced wholesale; **annotations** are merged, so foreign annotations (e.g. `kubectl.kubernetes.io/last-applied-configuration`) survive.
- **Typed kinds** copy their spec/data from the desired object while preserving Kubernetes immutable/allocated fields: StatefulSet `selector`, `serviceName`, `volumeClaimTemplates` and `podManagementPolicy` keep their live values (changing them requires a manual delete/recreate migration); ConfigMap data is replaced wholesale (removed keys disappear).
- **A preserved field the rest of the object depends on must be preserved coherently.** `volumeClaimTemplates` is the only one of these that another part of the same object refers to, and preserving it in isolation produced a StatefulSet nobody asked for in both directions: a desired claim that was not created left the pod template mounting a volume that does not exist, naming a `volumeMounts` field the user never wrote (Kubernetes 1.34+ rejects the entire Update; older servers accept it and reject every pod the StatefulSet controller then creates), and a claim the user removed stayed behind while its mount did not (accepted silently, so the product rolls onto the container's writable layer with its PVC still bound). The apply path therefore reconciles the mounts against the claim templates that survived — dropping a mount for a claim that was not created, and restoring a preserved claim's mount **from the live template** rather than deriving a path. The general rule for any future preserved field: *preserve the field's whole contract, or the object converges into a state neither the user nor the handler described.*
- **Service** is assigned the desired `ServiceSpec` **as a whole**, after which only the server-owned/immutable fields are restored — `clusterIP`/`clusterIPs`, `ipFamilies`/`ipFamilyPolicy`, `healthCheckNodePort`, `loadBalancerClass` — and a NodePort the API server already allocated is carried over onto the matching desired port (matched by name, falling back to port number) unless the handler pinned one explicitly. The consequence for handler authors: **any mutable `ServiceSpec` field left at its zero value overwrites the live value**, so a handler must build the Service it wants in full rather than relying on previously applied state.
- **Arbitrary GVKs** (`ExtraResources`) get a generic copy of every top-level field except `apiVersion`/`kind`/`metadata`/`status` via unstructured conversion.

### 5.3.4 Benefits

- **Consistency**: All products follow the same reconciliation structure.
- **Controlled Extension**: Products can only extend at designated points.
- **Maintainability**: Changes to the core flow affect all products uniformly.

## 5.4 Owned Collaborator Pattern (Composition over Global State)

### 5.4.1 Pattern Overview

Shared machinery is held as an explicitly constructed value, owned by whoever needs it and handed to its collaborators through their configuration, instead of living in a package-level variable reached through a global accessor. Ownership is visible in the type, and lifetime is visible in the wiring.

### 5.4.2 Application in SDK

- **`ExtensionRegistry[CR]`**: The registry of one product's extensions. The operator constructs it with `common.NewExtensionRegistry[CR]()`, registers into it, and passes it to exactly one reconciler through `GenericReconcilerConfig[CR].ExtensionRegistry` (§4.2.3). The SDK holds no registry of its own: no package-level instance, no accessor function. A binary managing several CR types builds one registry per type, and the type parameter makes sharing one across products a compile error rather than a runtime surprise.
- **Scheme**: The `runtime.Scheme` is likewise built once in `main` (in practice the manager's, via `mgr.GetScheme()`) and passed explicitly — the reconciler takes it as `GenericReconcilerConfig.Scheme`. The SDK declares no global scheme.

### 5.4.3 Benefits

- **Isolation**: One product's hooks cannot execute against another product's clusters, because no object is reachable from both.
- **Explicit Wiring**: The reconciler's dependencies are visible in its config, which also makes the failure mode of forgetting one a locally diagnosable "no hooks run" rather than a global-state mystery.
- **Testability**: A test constructs its own registry, so cases neither leak registrations into each other nor need a global reset; per-test instances are safe to run in parallel.
- **Deterministic Execution**: Extensions execute in priority order (highest first), with the registration sequence number as a total tiebreaker.
- **Thread Safety**: The registry is guarded by a `sync.RWMutex`; hook execution runs against a snapshot of the entries.

### 5.4.4 Example

```go
// Registrations are wrapped in entries so priority, registration sequence and
// per-registration fault tolerance travel with the extension. The registry is
// instantiated for the product's own CR type, so the entries hold extensions
// that already speak that type.
type extensionEntry[T Extension] struct {
    extension   T
    priority    ExtensionPriority
    seq         uint64 // registration sequence: total order for equal priorities
    stopOnError *bool  // nil = use the hook's default
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

An extension declares the CR it operates on and is registered directly — there is no adapter and no type assertion anywhere on the path:

```go
// func (e *SafeModeExtension) PreReconcile(
//     ctx context.Context, c client.Client, cr *HdfsCluster) error
var _ common.ClusterExtension[*HdfsCluster] = &SafeModeExtension{}

registry := common.NewExtensionRegistry[*HdfsCluster]()
registry.RegisterClusterExtension(&SafeModeExtension{}, common.WithPriority(common.PriorityHigh))
```

## 5.5 Builder Pattern

### 5.5.1 Pattern Overview

The Builder Pattern separates the construction of a complex object from its representation, allowing the same construction process to create different representations.

### 5.5.2 Application in SDK

- **StatefulSetBuilder**: Constructs `StatefulSet` resources step-by-step, handling complex configurations like volumes, containers, and affinity rules.
- **ConfigMapBuilder**: Builds ConfigMaps with merged configurations (`WithMergedConfig`, see §4.5.2).
- **ServiceBuilder** / **MetricsServiceBuilder**: Constructs Service resources with appropriate ports and selectors.
- **PDBBuilder**: the role-level PodDisruptionBudget — the last kind the framework itself builds through this package.
- **RoleBuilder / RoleBindingBuilder / ClusterRoleBuilder / ClusterRoleBindingBuilder / ServiceAccountBuilder**: offered for product code; the framework calls none of them. It builds the workload ServiceAccount and its Role/RoleBinding inline (`ensureServiceAccount`, `buildWorkloadRBAC`), and it never builds a **ClusterRole** at all — the operator's own ClusterRole is generated by controller-gen from `+kubebuilder:rbac` markers in the adopting operator (`security.md` §3.3), not by this SDK.
- `BaseRoleGroupHandler` builds the role group ConfigMap and both Services through these builders, so a product that overrides one part of the workload inherits the same construction rules for the rest.
- **Ownership of returned values**: `Build()` returns deep copies — mutating a built object never reconfigures the builder — and `WithLabels`/`WithAnnotations` on the RBAC and ServiceAccount builders **merge** into the existing set rather than replacing it.

### 5.5.3 Builder Workflow

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

### 5.5.4 Benefits

- **Step-by-Step Construction**: Complex resources are built incrementally.
- **Configuration Flexibility**: Different configurations produce different resource representations.
- **Separation of Concerns**: Construction logic is isolated from business logic.

## 5.6 Adapter Pattern

### 5.6.1 Pattern Overview

The Adapter Pattern converts the interface of a class into another interface that clients expect, enabling classes with incompatible interfaces to work together.

### 5.6.2 Application in SDK

- **Config Format Adapters**: Convert internal configuration maps to various external formats:
  - `XMLAdapter`: Adapts to Hadoop XML format
  - `PropertiesAdapter`: Adapts to Java .properties format
  - `YAMLAdapter`: Adapts to YAML format
  - `EnvAdapter`: Adapts to environment variable format
  - `INIAdapter`: Adapts to INI format

### 5.6.3 Benefits

- **Format Independence**: SDK core works with internal map representation.
- **Extensibility**: New formats can be added by implementing `ConfigMarshaler`; `ConfigUnmarshaler` is added only for a format that is read back.
- **Reusability**: Same configuration source can produce multiple output formats.

## 5.7 Observer Pattern

### 5.7.1 Pattern Overview

The Observer Pattern defines a one-to-many dependency between objects so that when one object changes state, all its dependents are notified and updated automatically.

### 5.7.2 Application in SDK

- **Event Recording**: The SDK uses Kubernetes `EventRecorder` to emit events when resources change.
- **Status Updates**: Extensions can observe and react to status changes via hooks.

### 5.7.3 Benefits

- **Decoupling**: Event emission is decoupled from business logic.
- **Auditability**: All significant changes are recorded as events.
- **Troubleshooting**: Events provide a chronological log of operations.

## 5.8 Pattern Summary

| Pattern | Primary Application | Key Benefit |
|---------|---------------------|-------------|
| Interface Segregation | `ClusterInterface` (`client.Object` + 2 methods), `RoleGroupHandler` | Focused, implementable contracts |
| Strategy | Extensions, `ConfigMarshaler` | Swappable behaviors |
| Template Method | Reconciliation flow | Consistent process with hooks |
| Owned Collaborator | `ExtensionRegistry[CR]`, Scheme | Explicit wiring, no global state |
| Builder | StatefulSetBuilder | Complex object construction |
| Adapter | Config format adapters | Format interoperability |
| Observer | Event recording | Change notification |

# 6. Key Problems and Solutions

- **Runtime errors and code redundancy caused by type assertions**
  - **Solution**: Introduce Go Generics for the reconciler, the extension interfaces, the extension registry and the webhook contracts, so a product hook receives its own CR type and no adapter or assertion sits on the path.
  - **Core Advantage**: Compile-time type safety, reduced boilerplate code, improved development efficiency.

- **Residual orphaned resources after role group deletion**
  - **Solution**: Compare Spec against the Status snapshot, verify ownership through ownerReferences, and retire the orphans through a multi-pass state machine — scale to zero, ordered drain, then deletion in a fixed order with each step confirmed gone before the next; an optional gray-delete grace period defers the whole sequence, and the reconcile loop requeues for whatever is pending.
  - **Core Advantage**: Efficient and precise, avoiding accidental deletion and abrupt pod termination, ensuring state convergence.

- **Repetitive multi-product configuration validation/default value logic**
  - **Solution**: Webhook divided into common and specific logic; SDK provides common tools, product side implements specific interfaces.
  - **Core Advantage**: Logic reuse, flexible extension, intercepting illegal configurations upfront.

- **Complex logic for external infrastructure binding (S3/DB)**
  - **Solution**: Introduce high-level `Connection`/`Bucket` CRDs plus opt-in resolution and rendering helpers (`pkg/s3`), with credentials delivered over CSI instead of being rendered into config. Database connections currently get the typed CRDs and validation only (§4.12.2).
  - **Core Advantage**: Decouples business logic from infrastructure details, reducing configuration complexity and common misconfigurations.

# 7. Deployment and Extension Guide

## 7.1 SDK Deployment Dependencies

- **K8s Version**: 1.31+ (Adapts to Webhook AdmissionReviewVersions=v1).
- **Dependent Components**: cert-manager (for Webhook certificate generation), kubebuilder 3.0+ (for code generation).
- **Permission Requirements**: the operator's own ClusterRole must cover everything the framework
  calls on its identity. The enumerated set — baseline, conditional grants and the two whose absence
  is not self-announcing — is `security.md` §3.3. It is deliberately not restated here: one copy
  drifts, two copies disagree.

## 7.2 New Product Extension Steps

1. **Define the CRD struct.** Embed `metav1.TypeMeta` and `metav1.ObjectMeta`, mark the type `+kubebuilder:object:root=true`, and embed the SDK Generic Spec/Status model in the product's own Spec/Status.
2. **Register it with the scheme.** `SchemeBuilder.Register(&YourCluster{}, &YourClusterList{})` — the reconciler reads the fetched object into the CR itself, so an unregistered type fails at `client.Get`.
3. **Run `make generate`.** controller-gen emits `DeepCopyObject()` (completing `client.Object`) and `DeepCopy() *YourCluster` (completing `ClusterResource[*YourCluster]`). Neither is hand-written.
4. **Write the two `ClusterInterface` methods**: `GetSpec() *v1alpha1.GenericClusterSpec` and `GetStatus() *v1alpha1.GenericClusterStatus` (see §5.1.4). That is the whole cluster-level contract.
5. **Implement `RoleGroupHandler[*YourCluster]`** — typically by embedding `BaseRoleGroupHandler` — to describe the Kubernetes resources of a role group.
6. **Wire a `GenericReconciler`** with a `GenericReconcilerConfig[*YourCluster]`, setting at least `Client`, `Scheme`, `Recorder`, `RoleGroupHandler` and `Prototype` (`&YourCluster{}`), plus `RoleProvider` (your `RoleCatalog`) and `ImageResolution`, and the optional hooks the product needs (`APIReader`, `RoleGroupResolver`, `Dependencies`, `ServiceHealthCheck`, `WorkloadRBACRules`, gray-delete and health intervals). Call `SetupWithManager`.
7. *(Optional)* **Add extensions**: implement `ClusterExtension[*YourCluster]` / `RoleExtension[*YourCluster]` / `RoleGroupExtension[*YourCluster]` (declaring the concrete CR in the hook signatures), build a registry with `common.NewExtensionRegistry[*YourCluster]()` in `main.go` before the manager starts, register them with `RegisterClusterExtension`/`RegisterRoleExtension`/`RegisterRoleGroupExtension` (with `common.WithPriority` / `common.WithStopOnError` where ordering or fault tolerance matters), and **set `GenericReconcilerConfig.ExtensionRegistry`** — without that field the hooks never run.
8. *(Optional)* Implement `ProductDefaulter`/`ProductValidator` interfaces to customize Webhook logic.
9. *(Optional)* Add product config formats: an adapter implementing `config.ConfigMarshaler`, registered on the handler's `MultiFormatConfigGenerator`.
10. **Declare the operator's own RBAC.** Copy the `+kubebuilder:rbac` block from `security.md` §3.3.1
    onto your controller and add whichever §3.3.2 conditional grants your operator triggers. The SDK
    consumes these permissions but cannot declare them — controller-gen never walks a dependency's
    packages — and the scaffold generates markers only for your own CR, not for anything the
    framework touches. Two of them fail without announcing themselves (§3.3.3).
11. Generate Webhook and CRD configurations via Kubebuilder and deploy for verification.

# 8. Summary and Outlook

## 8.1 Summary of Core Advantages

Through layered architecture, interface-driven design, generics transformation, and extension point mechanisms, this SDK achieves common logic reuse and flexible extension for multi-cluster products. It simultaneously resolves key issues such as orphaned resources, terminology conflicts, and type safety, aligning with K8s ecosystem standards and adapting to production-grade Operator development needs.

## 8.2 Future Optimization Directions

The following are **not yet implemented**; they describe intended direction, not current behavior:

- Support **ConversionWebhook** to achieve smooth CRD version upgrades.
- Add monitoring metrics for extension execution time, resource cleanup counts, etc., facilitating troubleshooting.
- A `pkg/database` resolver mirroring `pkg/s3` (JDBC URL construction plus a credentials volume).
- Opt-in finalizer support so cluster deletion — not just role group orphaning — can run SDK cleanup such as PVC removal.
