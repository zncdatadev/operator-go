# Common Product Cluster Operator SDK Technical Architecture Document

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
| `affinity` | **wholesale** — the RoleGroup's replaces the Role's, and `{}` clears it | The exception, and deliberate. `resources` is independent knobs; an affinity is a single scheduling *policy*, so a partial edit produces a constraint neither author wrote (a `nodeSelectorTerms` list is an OR of ANDs). Kubernetes replaces `PodSpec.Affinity` too. Per-member inheritance also removes the only way to express "no affinity", which a single-node group needs. |
| `gracefulShutdownTimeout`, `logging` | per field / per container, and **per level** inside a container | Scalars and an already keyed map. Within a container, an entry naming `console`, `file` or a logger **without stating a level** is "inherit", not "clear" — `console: {}` keeps the Role's threshold. |

This works only because these fields carry **no CRD-level default**: structural defaulting fills a field as soon as its enclosing object exists, so a `+kubebuilder:default` on a leaf makes "unset here" indistinguishable from "explicitly the default" and the Role's value can never win. Defaults therefore live at consumption time (`StorageResource.GetCapacity`, `RoleGroupConfigSpec.GetGracefulShutdownTimeout`, the renderers' root-level INFO).

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

## 2.6 Product Config vs. Defaulting (Separation of Layers)

The SDK distinguishes **two different mechanisms** by which a product supplies values that are not typed by the user, and they must not be conflated:

| | **`ProductDefaulter`** (Webhook) | **`ProductConfig`** (merge layer) |
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
operator upgrade cannot move it onto the co-released image. `BaseRoleGroupHandler.ImageDefaults` is
therefore evaluated on every reconcile, and `ImageSpec.ResolveImage(productName, defaults)` folds it
under whatever the user wrote:

| layer | source | when |
| --- | --- | --- |
| user | `spec.image` | whatever the CR states, per field |
| product | `handler.ImageDefaults` | every reconcile |

`ProductName` no longer decides *whether* `spec.image` is read — it supplies the product name, which
is both the `app.kubernetes.io/name` value and the repository path segment. Coupling the two meant a
product that wanted the labels had to accept an image path that could not express the tag
convention, and three migrated operators consequently hand-rolled image resolution and dropped
`app.kubernetes.io/version` entirely. An unresolvable `spec.image` is now an **error** rather than a
silent fall back to the handler's static image: running a version nobody asked for is not a safe
default for a stateful product (the same call as `config.affinity` above).

**The `ProductConfig` hook receives a `ctx` and a client, and may fail.** "Recomputed every reconcile, and may reflect the current state of the cluster" is only true if the hook can *read* the cluster; without those parameters it was a pure function of the CR, so the products that most needed a product-config layer — anything resolving an `S3Connection` reference or a ZooKeeper address — could not use it, and a failed lookup had nowhere to go but a swallowed error or a panic. Zero operators used the hook, which was the symptom rather than the disease.

Products that already perform the lookup inside `BuildResources` use the imperative counterpart, `RoleGroupBuildContext.ApplyProductDefaults`, rather than repeating it: it folds an overrides layer **beneath** everything already merged, by the merge's own per-dimension rules. Two operators had hand-written that fold, identically, along with a second rule for environment variables that the framework can express directly because `MergedConfig.EnvVars` is a map rather than an ordered list.

- **`ProductDefaulter`** is the right place for stable, user-facing **typed Spec defaults** (see §4.3). The value becomes part of the user's persisted Spec and is visible via `kubectl get`.
- **`ProductConfig`** is the right place for **product-intrinsic and derived config-file content** — e.g. a ZooKeeper connection string built from the actual resources, a quorum peer list from pod ordinals, or a JVM heap sized from the role group's resources. It is *config generation, not defaulting*: computing it at reconcile time (rather than freezing it into the Spec at admission) means an operator upgrade **recomputes** the configuration for existing clusters, and values derived from mutable state stay fresh. It is injected as the lowest merge layer (§2.5), so user overrides still win.

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
| handler fields (`Image`, `RoleContainerPorts`, …) | process | reconcile-**invariant** settings — a static image, `ProductName`, `ImageDefaults`, ports that do not depend on the CR |
| `RoleGroupBuildContext` (`Image`, `ImagePullPolicy`, `ContainerPorts`, `ServicePorts`) | one role group of one reconcile | anything derived from **this** CR |

Assigning a per-cluster value to a handler field from inside `BuildResources` fails in two ways, and the quieter one bites first:

- above `MaxConcurrentReconciles: 1` the writes **race**, and one cluster's image or TLS-dependent ports are built into another cluster's workload;
- at the default concurrency of 1 it **leaks**: a product that conditionally skips one assignment silently inherits the previous CR's value. spark-k8s-operator shipped exactly that — a CR omitting `pullPolicy` took the last CR's — with a serial reconcile loop and no race involved.

`BuildResources` on the base handler is **read-only on the handler**; the per-call fields are read from the build context, which is rebuilt per role group. The one place the framework itself held per-CR state was a handler-registered `SidecarManager`: `SetProductImage` writes the resolved image into its configs, so it is now cloned for the build. `VolumeProviders` already followed this shape and is the precedent the new fields copy.

Handler fields are still the right home for invariant configuration — this is a split, not a deprecation.

**The same context carries the product's declared intent, replacing the post-build patch.** `BuildResources` returns a finished object, and for anything the framework does not model the only extension point used to be mutating it afterwards. That cost twice: products re-derived what the handler already knew (zookeeper-operator located the primary container as `Containers[0]`, which a sidecar provider inserting a container earlier silently invalidates), and the edit landed *after* the framework's own ordering.

| field | replaces | why declaring it first matters |
| --- | --- | --- |
| `MainContainerCustomizer` | patching `Containers[0]` of the returned StatefulSet | it runs before `podOverrides` are strategic-merged, so the **user's** overrides still win; a post-build edit inverts that silently |
| `ListenerClass` | patching `Service.Spec.Type` after the fact | one shared `listener.ServiceTypeFor` mapping instead of a three-way switch re-derived per product |

Both fail loudly: a customizer that returns an error, or that changes the image (resolved once and already propagated to the sidecars), fails the role group with a `*ValidationError` rather than shipping a workload missing what the product meant to set.

### 4.1.5 Writing a Role Group Handler

§4.1.4 covers a handler's *lifetime*; this covers its *contract*. Everything below was source-only before it was written down, which is why six operators independently rediscovered parts of it by reading each other's code.

#### The build context is the whole input, and half of it is writable

`BuildResources` receives no CR fields it has not been handed: `buildStatefulSet` takes `_ client.Client, _ CR` and reads nothing from either. Everything CR-derived reaches the framework through `RoleGroupBuildContext`, and the fields split into two kinds:

| the framework writes, the handler reads | the handler writes before delegating |
| --- | --- |
| `ClusterName`, `ClusterNamespace`, `ClusterSpec`, `RoleName`, `RoleSpec`, `RoleGroupName`, `RoleGroupSpec`, `ResourceName`, `ServiceAccountName`, `MergedConfig`, `VectorAggregatorAddress`, `VectorLogPipelineActive` | `Image`, `ImagePullPolicy`, `ContainerPorts`, `ServicePorts`, `ListenerClass`, `MainContainerCustomizer`, `VolumeProviders`, `ClusterLabels`, and `MergedConfig` via `ApplyProductDefaults` |

`ClusterLabels` is a materialised clone and is **never nil**, so a handler may add to it unconditionally; whatever it holds is merged into every built resource's metadata and pod template. `SidecarManager` is always non-nil under the reconciler — which makes `BaseRoleGroupHandler.WithSidecarManager` inert on that path, and useful only to a product that calls `BuildResources` itself.

#### What comes back, and what a nil field means

`RoleGroupResources` is not a bag of optional outputs. The apply path treats **nil as an instruction**, not as "leave it alone": a nil `MetricsService` or `PodDisruptionBudget` makes the framework *delete* the corresponding live object, because that is how a role group that stops declaring one is converged. `ExtraResources` carries arbitrary GVKs, which must be registered in the reconciler's scheme and live in the CR's namespace — the framework sets a controller owner reference, and a cross-namespace owner is rejected by Kubernetes.

**The framework owns the fixed slots' names, and the handler owns their content.** `ConfigMap`, `Service`, `StatefulSet` and `PodDisruptionBudget` must be named `buildCtx.ResourceName`; `HeadlessService` and `MetricsService` that name plus `-headless` and `-metrics`; all six must live in `buildCtx.ClusterNamespace`. A slot that breaks either rule fails the role group with a `*ValidationError` *before any resource is applied*.

This is the direct consequence of the paragraph above. "Nil is an instruction to delete" only works if the framework can find the object the instruction refers to, and it finds all six by their derived names — in the in-spec reclaims and, when the role group leaves the spec, in `RoleGroupCleaner`. Nothing recovers a slot filled under a different name: live-orphan discovery rejects any object whose name is not the derived one, and the teardown's final confirmation re-checks the same fixed list before pruning the group's status entry. Such an object is applied, owner-referenced, reported healthy, and then outlives every teardown until the cluster CR is deleted.

The general rule this encodes: **an optional, product-supplied resource slot must have either a framework-owned name or a framework-stamped identity — never neither.** The framework picks the name for the fixed slots because a name is checkable at build time, against no cluster and in one reconcile; an identity label is only checkable against a live List, on a path where a stale cache answering "nothing here" is terminal. `ExtraResources` takes the other branch deliberately, because there the names are the product's: its reclaim is label-selected and opt-in through `SetupWithManagerOptions.ExtraOwns`. A product that needs a metrics Service under its own name uses that door.

The same reasoning bounds what a CR label may say. Labels are the one channel from a cluster's *deployer* to the built resources (§4.1.4), but three keys — `metrics.kubedoop.dev/service`, `pdb.kubedoop.dev/role`, `pdb.kubedoop.dev/role-group` — are the framework's own slot markers, and a reclaim deletes by their presence or value. They are filtered out of `ClusterLabels` for that reason: a marker that a user can set is a delete instruction that a user can forge. The filter is an enumerated set, not a domain prefix rule, because `restarter.kubedoop.dev/enable` establishes that `kubedoop.dev` is shared with the platform rather than private to this framework.

#### The container contract

- The primary container's name resolves `RoleMainContainerName[role]` → `MainContainerName` → the role group's resource name, and must be settled **before** `Build()`: `podOverrides` are strategic-merged by container name, so a later rename leaves the user's override appended as a phantom, image-less container.
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

Eleven causes across twelve sites inside `BaseRoleGroupHandler.BuildResources`. Two produce a `*ValidationError` — a `podOverrides` mount that displaces a framework-owned one, and a `MainContainerCustomizer` that errors or changes the image — and the rest are plain wrapped errors that the reconciler re-wraps as a `*ResourceBuildError`. All of them fail one role group, not the pass: role iteration is best-effort and sorted (§4.4).

#### If you do not embed `BaseRoleGroupHandler`

A handler implementing the interface directly inherits none of the conventions, and four of them are load-bearing:

1. every fixed slot must carry its derived name — the headless Service `<ResourceName>-headless` above all, since the StatefulSet's `serviceName` is derived from it and immutable. This one the framework now checks and rejects rather than leaving to convention;
2. the pod must mount the ConfigMap named `buildCtx.ResourceName`, which is what the framework's own ConfigMap is called;
3. `clusterOperation.stopped` must force replicas to 0 — it is implemented in the base handler, not in the reconciler;
4. `RoleNameProvider`, `LoggingProducerProvider` and `BuildRolePodDisruptionBudget` are optional capability interfaces the reconciler type-asserts for; not implementing them silently disables the role-name typo warning, the Vector wiring and the role-level PDB.

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
    - **Scope boundary**: `ProductDefaulter` defaults typed Spec fields only. Product **config-file content** (and any value derived from live cluster state) is *computed* at reconcile time via `ProductConfig`, not defaulted here — see §2.6 for the distinction.
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
            Image:           buildCtx.Image, // the product resolves it; SetProductImage will not
            Ports:           []corev1.ContainerPort{{Name: "metrics", ContainerPort: 9102}},
            RestartPolicy:   sidecar.SidecarRestartPolicy(), // the provider does not add this
            SecurityContext: sidecar.DefaultSecurityContext(),
        }),
        &sidecar.SidecarConfig{Enabled: true},
    )
    ```
- **Workflow**: The `GenericReconciler` registers the Vector provider — configured with the producer declarations (`[]productlogging.ContainerLogging`, from the handler's `LoggingProducers`) and the shared log volume size — only when **all three** gates pass. Any one of them failing means the sidecar could not do its job, so the provider is not registered and the rest of the cluster keeps converging:
    1. **The agent is enabled** for the role group (`logging.enableVectorAgent`, after the role/role-group logging merge).
    2. **At least one producer is declared** by the handler's `LoggingProducers`. An agent with nothing to collect would mount an empty pipeline; the reconciler logs the mismatch and skips, so enablement and producer declaration stay consistent in one place.

       This gate reads the **outer** handler — the object passed as `GenericReconcilerConfig.RoleGroupHandler` — while the logging *config file* is rendered from `BaseRoleGroupHandler`'s own `LoggingContainers`. Go has no virtual dispatch, so a base handler can never see an override, and the two lists are therefore independently addressable. That is the seam for a product whose logging config file is product-owned (Airflow's `log_config.py`, which must be built on Airflow's own `DEFAULT_LOGGING_CONFIG`): **override `LoggingProducers` on your handler and leave `LoggingContainers` empty.** The producer joins the pipeline, the framework renders no file for it, and there is no ConfigMap key collision. The producer's `Container` must still name a real container in the assembled pod, and the product's log file must land at `productlogging.LogDirFor(decl)` with the framework's `LogFileSuffix`, or the pipeline comes up and collects nothing.
    3. **Something supplies `vector.yaml`.** The sidecar runs `vector --config <mount>/vector.yaml`, so it is only injected when that key will actually be written into the role group ConfigMap: either the **CR** implements `reconciler.VectorAggregatorProvider` (the framework then renders the file itself) or the **role group handler** implements `reconciler.VectorConfigProvider` and answers `ProvidesVectorConfig(roleName) == true` (the product writes it). With neither, registering the provider would fail sidecar validation (§4.6.2, Dependency Validation) on every cycle and abort the whole cluster's reconcile over a product that is simply not wired for Vector. It is reported as the product-configuration mistake it is: a `Warning`/`VectorSidecarSkipped` event on the CR naming the role group and both interfaces, and the reconcile continues.

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
- **Degraded-input warnings**: some inputs are bad but not fatal, and they get a `Warning` of their own rather than being dropped silently. The complete vocabulary the framework emits: `ReconcileError`, `ReconcilePanic` (a recovered panic), `PodOverrideIgnored` (a `podOverrides` layer that fails to decode or patch — `MergedConfig.PodOverrideErrors`), `UnknownConfiguredRole` (a handler configured for a role the CR does not declare), `ImmutableFieldIgnored` (a desired change to a preserved immutable field), `VectorSidecarSkipped` (the agent is enabled but nothing supplies `vector.yaml`), plus the three resource-operation `Normal` events above.
- **Product-facing helpers**: `EmitWarningEvent`, `EmitNormalEvent`, `LogAndEmitError` and `LogAndEmitInfo` are available to extensions and product code. The framework calls only the first two.

### 4.14.3 Core Value

- **Auditability**: Provides a trace of actions taken by the Operator.
- **Troubleshooting**: Warning events appear directly in `kubectl describe`, giving immediate visibility into failures.

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
5. **StatefulSet**: Applied after all its dependencies (configs, DNS, extras) are in place. The StatefulSet controller then creates Pods in ordinal order.
6. **PDB** (PodDisruptionBudget): Applied after the workload, as it references existing Pods. It enforces availability guarantees during voluntary disruptions once the workload is running.
7. **MetricsService**: Applied last; it only exposes already-running Pods to Prometheus discovery and nothing depends on it.

Orphan cleanup uses its own order — `PDB → StatefulSet → ConfigMap → Service → headless Service → metrics Service` (see §4.4.3) — which is **not** the exact inverse of this creation order. The two orders answer different questions: creation sequences prerequisites before dependants, while deletion removes the PDB first so it cannot block pod eviction and drops the Services last.

**Resource Application Semantics (create-or-update)**

Applying a resource is not create-only: when the resource already exists, `applyResource` updates the live object to the handler-built desired state on every reconcile, so CR spec changes (replicas, config overrides, ports, ...) propagate to existing resources (issue #526). The update rules live in `copyDesiredState` (`pkg/reconciler/apply.go`):

- **Labels** are framework-owned and replaced wholesale; **annotations** are merged, so foreign annotations (e.g. `kubectl.kubernetes.io/last-applied-configuration`) survive.
- **Typed kinds** copy their spec/data from the desired object while preserving Kubernetes immutable/allocated fields: StatefulSet `selector`, `serviceName`, `volumeClaimTemplates` and `podManagementPolicy` keep their live values (changing them requires a manual delete/recreate migration); ConfigMap data is replaced wholesale (removed keys disappear).
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
- **PDBBuilder**, **RBACBuilder**, **ServiceAccountBuilder**: Cover the remaining framework-owned kinds.
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
- **Permission Requirements**: Operator requires CRUD permissions for resources such as StatefulSet, Service, ConfigMap, etc.

## 7.2 New Product Extension Steps

1. **Define the CRD struct.** Embed `metav1.TypeMeta` and `metav1.ObjectMeta`, mark the type `+kubebuilder:object:root=true`, and embed the SDK Generic Spec/Status model in the product's own Spec/Status.
2. **Register it with the scheme.** `SchemeBuilder.Register(&YourCluster{}, &YourClusterList{})` — the reconciler reads the fetched object into the CR itself, so an unregistered type fails at `client.Get`.
3. **Run `make generate`.** controller-gen emits `DeepCopyObject()` (completing `client.Object`) and `DeepCopy() *YourCluster` (completing `ClusterResource[*YourCluster]`). Neither is hand-written.
4. **Write the two `ClusterInterface` methods**: `GetSpec() *v1alpha1.GenericClusterSpec` and `GetStatus() *v1alpha1.GenericClusterStatus` (see §5.1.4). That is the whole cluster-level contract.
5. **Implement `RoleGroupHandler[*YourCluster]`** — typically by embedding `BaseRoleGroupHandler` — to describe the Kubernetes resources of a role group.
6. **Wire a `GenericReconciler`** with a `GenericReconcilerConfig[*YourCluster]`, setting at least `Client`, `Scheme`, `Recorder`, `RoleGroupHandler` and `Prototype` (`&YourCluster{}`), plus the optional hooks the product needs (`APIReader`, `ProductConfig`, `Dependencies`, `ServiceHealthCheck`, gray-delete and health intervals). Call `SetupWithManager`.
7. *(Optional)* **Add extensions**: implement `ClusterExtension[*YourCluster]` / `RoleExtension[*YourCluster]` / `RoleGroupExtension[*YourCluster]` (declaring the concrete CR in the hook signatures), build a registry with `common.NewExtensionRegistry[*YourCluster]()` in `main.go` before the manager starts, register them with `RegisterClusterExtension`/`RegisterRoleExtension`/`RegisterRoleGroupExtension` (with `common.WithPriority` / `common.WithStopOnError` where ordering or fault tolerance matters), and **set `GenericReconcilerConfig.ExtensionRegistry`** — without that field the hooks never run.
8. *(Optional)* Implement `ProductDefaulter`/`ProductValidator` interfaces to customize Webhook logic.
9. *(Optional)* Add product config formats: an adapter implementing `config.ConfigMarshaler`, registered on the handler's `MultiFormatConfigGenerator`.
10. Generate Webhook and CRD configurations via Kubebuilder and deploy for verification.

# 8. Summary and Outlook

## 8.1 Summary of Core Advantages

Through layered architecture, interface-driven design, generics transformation, and extension point mechanisms, this SDK achieves common logic reuse and flexible extension for multi-cluster products. It simultaneously resolves key issues such as orphaned resources, terminology conflicts, and type safety, aligning with K8s ecosystem standards and adapting to production-grade Operator development needs.

## 8.2 Future Optimization Directions

The following are **not yet implemented**; they describe intended direction, not current behavior:

- Support **ConversionWebhook** to achieve smooth CRD version upgrades.
- Add monitoring metrics for extension execution time, resource cleanup counts, etc., facilitating troubleshooting.
- A `pkg/database` resolver mirroring `pkg/s3` (JDBC URL construction plus a credentials volume).
- Opt-in finalizer support so cluster deletion — not just role group orphaning — can run SDK cleanup such as PVC removal.
