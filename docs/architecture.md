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
  - The physical unit of deployment and resource isolation under a Role. Each RoleGroup maps directly to a Kubernetes `StatefulSet` (and associated Service, ConfigMap, PDB). This allows a single Role to be partitioned into multiple groups with distinct hardware specifications (CPU/Memory), replica counts, or specialized configurations (e.g., a "high-performance" DataNode group vs. a "standard" group).

- **SecretClass**
  - An object managed by `secret-operator`, enabling the injection of sensitive data (Certificates, Kerberos Keytabs, Passwords) into Pods via the Kubernetes CSI (Container Storage Interface). Workloads reference a `SecretClass` to mount volumes that are dynamically populated by specific security backends.

- **Overrides**
  - A hierarchical configuration mechanism allowing precise customization of generated resources. It supports overriding Configuration Files (e.g., XML/Properties), Environment Variables, CLI arguments, and Pod attributes (via PodTemplateSpec). **Important**: Override fields (`configOverrides`, `envOverrides`, `cliOverrides`, `podOverrides`) are **flattened** directly at Role/RoleGroup level, NOT nested under an `overrides` field. RoleGroup overrides inherit from and take precedence over Role overrides.

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

To resolve conflicts between Role and RoleGroup configurations, the SDK defines strict merge strategies:

- **Map Types (Config/Env)**: Uses **Deep Merge**. Keys present in RoleGroup override those in Role; new keys are appended.
- **Slice Types (CLI/JVM/Volumes)**: Supports **Replace** (default) and **Append** modes.
  - **Replace**: If RoleGroup defines a slice, it completely replaces the Role's slice.
  - **Append**: If configured (e.g., via specific flags or conventions), RoleGroup items are appended to the Role's slice.
- **PodTemplate**: Follows the Kubernetes **Strategic Merge Patch** standard, allowing fine-grained overrides of Pod fields (e.g., changing container image while keeping volume mounts).

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
    - `GenericClusterStatus`: Common cluster status, employing standard Kubernetes **Conditions** (e.g., `Available`, `Progressing`, `Degraded`, `ServiceHealthy`) to represent complex states beyond simple replica counts.
    - **Auxiliary Models**: `RoleCommonConfig` (Role Common Configuration), `RoleGroupCommonConfig` (Role Group Common Configuration), `ZKConfig` (ZK Common Configuration), etc.

- **Design Points**: Specific product Spec/Status must embed common models (e.g., `HdfsClusterStatus` embeds `GenericClusterStatus`) to achieve state reuse. The `ServiceHealthy` condition allows products to report business-level readiness (e.g., HDFS safe mode off).

### 3.2.2 Abstract Interface Layer (Contract Definition Layer)

Defines core interfaces and extension contracts. It only depends on the API layer and is the core of SDK's "multi-product reuse," divided into business interfaces and extension interfaces.

- **Business Interfaces**:
    - `ClusterInterface`: Cluster-level interface, defining methods for cluster name, Spec/Status access, state updates, etc.
    - `RoleInterface`: Role-level interface, defining methods for role name, default ports, configuration extenders, etc.
    - `RoleExtender`: Role extender interface, defining logic for extending Role configurations (e.g., extending `role.config` fields for product-specific workload settings).
    - `RoleGroupHandler`: The primary implementation extension point for product operators. Each product implements this interface to define the specific Kubernetes resources (StatefulSet, Services, ConfigMaps) built for each RoleGroup. The `GenericReconciler` calls `BuildResources()` on this handler during reconciliation.

- **Extension Interfaces**:
    - `ClusterExtension/RoleExtension/RoleGroupExtension`: Extension point interfaces, defining custom logic before and after reconciliation at each level.
    - `ExtensionRegistry`: Extension registry, managing the registration, priority-based ordering, and execution of all extensions.

### 3.2.3 Core Component Layer (Common Logic Layer)

Implements common business logic based on abstract interfaces. It depends on the Abstract Interface Layer and Tools Layer, and does not directly depend on specific products, ensuring logic reuse.

- **Core Components**:
    - `ClusterReconciler` (implemented as `GenericReconciler` in the SDK): Cluster reconciler, the entry point for the core reconciliation process, including role traversal, extension point execution, and orphaned resource cleanup.
    - `ConfigMerger`: Configuration merger, implementing the merging and differentiated override of role and role group configurations.
    - `ConfigGenerator`: Configuration generator, transforming merged configuration maps into specific file formats (XML, Properties, YAML, etc.).
    - `SidecarManager`: Sidecar container manager, handling the injection of auxiliary containers (e.g., Log collection, Monitoring) into the Pod Spec.
    - `StatefulSetBuilder`: Resource builder, generating K8s resources such as StatefulSet and Service corresponding to role groups.
    - `RoleGroupCleaner`: Orphaned resource cleaner, cleaning up orphaned role group resources based on the comparison results of Spec and Status.

### 3.2.4 Tools Layer (Common Utility Layer)

Provides non-intrusive common utility functions for the Core Component Layer to call, reducing repetitive coding.

- **Core Tools**:
    - `K8sUtil`: K8s resource operation tool, encapsulating idempotent operations like CreateOrUpdate and Delete.
    - `ExecUtil`: Pod command execution tool, supporting the execution of commands inside containers during the reconciliation process (e.g., disk checks).

### 3.2.5 Specific Product Layer (Extension Implementation Layer)

Implements product-specific logic based on SDK abstract interfaces without modifying SDK core code, relying only on the API Layer and Abstract Interface Layer.

- **Implementation Points**:
    - **CR structs implement `ClusterInterface`/`RoleInterface` interfaces and provide `RoleGroupHandler` to define product-specific resources.**    - Implement specific logic through extension interfaces (e.g., HDFS ZK connectivity check, Namenode heap size configuration).
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

- **Generic Reconciler Skeleton**: `GenericReconciler[CR ClusterInterface]`, constraining CR type and reusing the reconciliation process.
- **Generic Extension Interface**: `ClusterExtension[CR ClusterInterface]`, eliminating type assertions and directly receiving specific CR types.
- **Generic Role Extender**: `RoleExtender[ExtConfig any]`, constraining extension configuration types for extending Role-level settings (e.g., `role.config` fields) to ensure type safety.

### 4.1.3 Core Value

Compile-time type checking reduces reliance on runtime type assertions; new products only need to bind generic types, reducing boilerplate.

## 4.2 Extension Point Mechanism Module

### 4.2.1 Design Approach

Reserve extension points at key nodes in the reconciliation process to support embedding custom logic on the product side, while unified management through a registry ensures ordered execution of extensions.

### 4.2.2 Extension Point Levels

1. **Cluster Level**: `PreReconcile` (Before Reconciliation), `PostReconcile` (After Reconciliation), `OnReconcileError` (On Exception).
2. **Role Level**: `PreReconcile`, `PostReconcile`, executed for a single role.
3. **Role Group Level**: `PreReconcile`, `PostReconcile`, executed for a single role group.

### 4.2.3 Extension Registration

- **Registration Timing**: Extensions must be registered during Operator initialization, specifically in the `main.go` setup phase before the Manager starts. This ensures all extensions are available when reconciliation begins.
- **Registration Method**: Use the `ExtensionRegistry.Register()` method to add extensions. Each extension must implement the appropriate interface (`ClusterExtension`, `RoleExtension`, or `RoleGroupExtension`).
- **Execution Order**: Extensions execute in **priority order (highest first)**. When multiple extensions share the same priority, they execute in registration order. Use `RegisterXxxExtensionWithPriority()` to assign explicit priority values (Lowest=0, Low=25, Normal=50, High=75, Highest=100).

### 4.2.4 Extension Lifecycle

- **Initialization**: Extensions are instantiated once during Operator startup. The SDK does not recreate extensions per reconciliation.
- **State Management**: Extensions should be stateless or manage their own internal state. The SDK passes the current CR context to each extension method, enabling access to cluster state without requiring persistent extension state.
- **Cleanup**: Extensions can implement an optional `Cleanup()` method for resource release during Operator shutdown.

### 4.2.5 Execution Process

The reconciler iterates through extensions in the extension registry, executing them in **priority order (highest first)**, supporting configuration for "process interruption on extension failure" to adapt to different fault tolerance needs.

- **Normal Execution**: Extensions execute sequentially. Each extension receives the current context and can modify the CR or return an error.
- **Error Handling**:
  - If an extension returns an error, the SDK captures the error and propagates it to the CR Status.
  - The `OnReconcileError` hook is triggered for cleanup or logging.
  - Subsequent extensions may be skipped depending on error severity (configurable via `StopOnError` flag).
- **State Recovery**: If an extension modifies the CR and a subsequent extension fails, the SDK does not automatically rollback changes. Extensions should implement their own compensation logic if needed.

## 4.3 Webhook Integration Module

### 4.3.1 Integration Scheme

Based on Kubebuilder annotation-driven practices, integrating MutatingWebhook and ValidatingWebhook to implement configuration pre-processing and legitimacy validation.

### 4.3.2 Core Functions

- **MutatingWebhook**:
    - **Common Logic**: Populate resource defaults (CPU/Memory), ZK configuration defaults (Port 2181), log path defaults.
    - **Specific Logic**: Product side implements the `ProductDefaulter` interface to populate product-specific default values (e.g., HDFS Namenode heap size).
- **ValidatingWebhook**:
    - **Common Logic**: Required field validation, resource format validation (CPU/Memory format), replica count legitimacy validation.
    - **Specific Logic**: Product side implements the `ProductValidator` interface to execute business rule validation (e.g., HDFS HA mode configuration validation).

### 4.3.3 Admission Workflow Overview

MutatingWebhook runs first to apply defaults. ValidatingWebhook runs next to enforce invariants. Failed validations reject the request before persistence, ensuring only valid specs enter reconciliation.

### 4.3.4 Deployment Adaptation

Automatically generate TLS certificates via cert-manager, and Webhook configuration files via Kubebuilder. No manual configuration of certificates and access rules is required during deployment.

## 4.4 Orphaned Role Group Resource Cleanup Module

### 4.4.1 Core Scheme

Adopts a hybrid scheme of "Spec vs Status comparison as primary, cluster resource query as secondary," which improves efficiency while avoiding accidental deletion.

### 4.4.2 Execution Process

1. Get the desired role group list (`desiredGroups`) of roles from Spec.
2. Get the historical actual role group list (`oldActualGroups`) from Status.RoleGroups.
3. Calculate orphaned role groups: `orphanedGroups = oldActualGroups - desiredGroups`.
4. Validate resource existence before deletion, deleting resources in the order of "PDB → StatefulSet → ConfigMap → Service".
5. Sync Status.RoleGroups to `desiredGroups` and update the actual status snapshot.

### 4.4.3 Safety Protection Mechanisms

- **Pre-Delete Validation**:
  - Before deleting any resource, the SDK verifies the resource still exists in the cluster.
  - Resource labels are checked to confirm ownership (matching the CR's ownership references).
  - Resources without proper ownership labels are **NOT deleted** to prevent accidental deletion of manually created resources.

- **Deletion Order**:
  - Resources are deleted in dependency order to avoid orphaned references:
    1. **PDB** (PodDisruptionBudget) - Remove first to avoid blocking StatefulSet deletion.
    2. **StatefulSet** - Scale to 0 first, then delete (ensures graceful pod termination).
    3. **ConfigMap** - Delete after StatefulSet is removed.
    4. **Service** - Delete last as other resources may reference it.
  - Each deletion waits for confirmation before proceeding to the next resource type.

- **PVC Handling**:
  - By default, **PVCs are PRESERVED** during orphaned resource cleanup to protect data.
  - If PVC deletion is explicitly requested, the SDK requires confirmation via a specific annotation.

### 4.4.4 Concurrency Conflict Handling

- **Optimistic Locking**:
  - The SDK uses Kubernetes resource versioning to detect concurrent modifications.
  - If a resource was modified by another process between read and delete, the operation is retried with the latest resource version.

- **Conflict Resolution**:
  - **409 Conflict**: Automatically re-fetches the resource and retries the deletion.
  - **429 Too Many Requests**: Implements exponential backoff before retry.
  - **404 Not Found**: Treats as success (resource already deleted by another process).

- **Status Synchronization**:
  - After cleanup, the SDK atomically updates both the CR Status and the actual cluster state.
  - If Status update fails, the next reconciliation cycle re-evaluates orphaned resources.

### 4.4.5 Boundary Handling

- **CR First Creation**: Status is empty, no orphaned resources, directly sync desired role groups to Status.
- **Manual Resource Deletion**: Rely on idempotent deletion (IgnoreNotFound) to avoid errors, syncing Status in the next reconciliation.
- **Status Tampering**: Query cluster resources before deletion, only deleting actually existing resources to avoid accidental deletion.

## 4.5 Configuration Generator Module

### 4.5.1 Design Background

Big data components often require configuration files in various formats (e.g., XML for Hadoop, Properties for Kafka/Zookeeper, YAML for others). Hardcoding serialization logic for each product leads to duplication and inconsistency.

### 4.5.2 Core Implementation

- **ConfigFormat Interface**: Defines the contract for configuration serialization.
  - `Marshal(data map[string]string) (string, error)`
- **FormatAdapter**: Adapter pattern implementation supporting common formats:
  - `XMLAdapter`: Converts key-value pairs into Hadoop-style `<property><name>...</name><value>...</value></property>` XML structure.
  - `PropertiesAdapter`: Converts key-value pairs into standard Java `.properties` format.
  - `YAMLAdapter`: Converts structured data into YAML format.
  - `EnvAdapter`: Formats as shell environment variable exports or .env file content.
- **LoggingGenerator**: A specialized component for handling structured logging abstraction.
  - **Input**: Generic YAML logger configuration (e.g., `containers.coordinator.loggers.ROOT.level: DEBUG`).
  - **Transformation**: Maps generic levels (DEBUG, INFO) to framework-specific formats (Log4j2 XML, Logback XML, Python Logging).
  - **Output**: Generates the final logging configuration file (e.g., `log4j2.properties`) injected into the ConfigMap.
- **Integration**: The `StatefulSetBuilder` utilizes the `ConfigGenerator` to process the merged configuration map (from `ConfigMerger`) into the final string data stored in ConfigMaps.

### 4.5.3 Core Value

- **Unified Logic**: Centralizes the complexity of file format generation, avoiding repetitive implementation in each product operator.
- **Extensibility**: Easily supports new formats by implementing the `ConfigFormat` interface.
- **Consistency**: Ensures generated configuration files adhere to standard formats and escaping rules.

## 4.6 Sidecar Injection Module

### 4.6.1 Design Background

Operations such as log collection (Vector), metric monitoring (JMX Exporter), and service mesh integration require injecting auxiliary containers into the business Pods. Manually configuring these sidecars in each CRD leads to configuration redundancy and maintenance difficulties.

### 4.6.2 Core Implementation

- **SidecarProvider Interface**: Defines the abstraction for sidecar injection.
  - `Inject(podSpec *corev1.PodSpec, config SidecarConfig) error`
- **Standard Implementations**:
  - `VectorSidecarProvider`: Injects Vector agent container, mounts log volumes, and configures environment variables based on `vectorAgentConfigMap`.
  - `JmxExporterSidecarProvider`: Injects Prometheus JMX Exporter agent and exposes metric ports.
- **Workflow**: The `StatefulSetBuilder` invokes the `SidecarManager` during Pod Spec construction. The manager iterates through enabled providers and injects Containers, Volumes, and VolumeMounts.

### 4.6.3 Core Value

- **Decoupling**: Separates auxiliary functions (Logging/Monitoring) from core business logic.
- **Reusability**: Standard sidecars can be reused across HDFS, HBase, and other products without code duplication.
- **Consistency**: Ensures uniform configuration for logs and metrics across the entire platform.

## 4.7 Dependency Management Module

### 4.7.1 Design Background

Big Data systems often have strict startup dependency orders (e.g., Zookeeper -> BookKeeper -> Pulsar Broker). Starting a component before its dependencies are ready typically results in "CrashLoopBackOff" states, polluting logs and complicating troubleshooting.

### 4.7.2 Core Implementation

- **External Reference Validation**:
  - The SDK automatically validates the existence of referenced external resources (ConfigMaps, Secrets) defined in the CR Spec.
- **DependencyResolver**:
  - **Component**: Validates external dependencies (e.g., Zookeeper Connection) during `PreReconcile`.
  - **Action**: If dependencies are missing, the Reconciler pauses the process and sets the `Degraded` condition with a descriptive message, effectively preventing the creation of Pods until dependencies are satisfied.

### 4.7.3 Core Value
- **Stability**: Prevents cascading failures and "noise" from pod crash loops by enforcing dependency checks before startup.
- **Clarity**: Clearly indicates missing prerequisites in the CR Status.

## 4.8 Health Management Module

### 4.8.1 Design Background
Stateful systems distinguish between "Infrastructure Ready" (Pod Running) and "Service Ready" (Business logic active). For example, an HDFS NameNode might be running but stuck in SafeMode, or a Database might be performing recovery. The Operator status must reflect this business reality.

### 4.8.2 Health Check Mechanism

The SDK implements a comprehensive health check mechanism that validates:
- **External Dependencies**: Availability of required external resources (e.g., Zookeeper, S3, Database).
- **Service Availability**: Whether the service is ready to accept traffic.
- **Pod Status**: Health and readiness of individual Pods.

- **Check Interval**: Health checks execute every **120 seconds** during reconciliation.
- **Timeout**: Each health check operation has a maximum timeout of **300 seconds**.
- **Failure Handling**:
  - If a health check fails, the CR Status is marked as **Degraded** with an appropriate reason and message.
  - If the controller itself encounters an internal error (e.g., panic, unexpected exception), the Status is **NOT modified** to prevent incorrect state propagation.
  - Transient failures trigger a requeue for retry in the next reconciliation cycle.

### 4.8.3 Core Implementation

- **Status Definition**: The SDK standardizes cluster status through Generic Conditions:
  - **Available**: At least one replica is ready and serving traffic.
  - **Progressing**: The cluster is rolling out a new version or scaling replicas.
  - **Degraded**: The cluster is experiencing issues (e.g., missing dependencies, crash loops, health check failures).
  - **ServiceHealthy**: The application-level check passed (e.g., SafeMode off, RegionServer registered).
  - **ReconcileComplete**: The SDK has finished the latest reconciliation loop successfully.
- **ServiceHealthCheck Interface**:
  - **Contract**: `CheckHealthy(ctx context.Context) (bool, error)`
  - **Mechanism**: Executed via `ExecUtil` inside the container or by querying external APIs.
  - **Example**: HDFS implements this to run `hdfs dfsadmin -safemode get`.
- **Status Aggregation**: The SDK aggregates Pod Readiness, Dependency Status, and Business Health Checks into the final `GenericClusterStatus`.

### 4.8.4 Core Value

## 4.9 Security Module

### 4.9.1 Design Philosophy
The SDK adopts a layered security strategy, addressing both **Infrastructure Security** (K8s access control, Pod context) and **Application Security** (identity, encryption). The core philosophy relies on "Privilege Separation" and "Automated Provisioning."

### 4.9.2 Infrastructure Security (Operator & K8s Layer)
- **ServiceAccount Provisioning**: The SDK can automatically manage ServiceAccounts for workloads, ensuring Pods run with appropriate identities distinct from the Operator's own identity.
- **RBAC Integration**: Supports binding minimum required permissions (RoleBindings) to workload ServiceAccounts, adhering to the Principle of Least Privilege.
- **Pod Security Context**: Enforces secure defaults for Pod execution (e.g., non-root users, fsGroup controls) to prevent container breakouts.

### 4.9.3 Application Security (Workload Layer)
- **Zero-Touch Secret Management**: Leverages `secret-operator` and the `SecretClass` abstraction to inject sensitive data (Kerberos Keytabs, TLS Certificates) via CSI volumes, preventing the Operator from directly handling secrets.
- **Automated Identity**: Supports backend mechanisms like `AutoTLS` (for mTLS) and `KerberosKeytab` (for Hadoop ecosystem identity) without manual intervention.

> **Note**: For detailed architecture, backend mechanisms, and workflow regarding Application Security and SecretClass, please refer to the dedicated security documentation: [Operator-Go Security Architecture](security.md).

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
  1. **Declaration**: The Product CR defines that a Role needs a listener by referencing a `ListenerClass`.
  2. **Injection**: The SDK creates a `PersistentVolumeClaim` (PVC) with specific annotations pointing to the listener configuration, instead of creating a Kubernetes `Service` directly.
  3. **Realization**: The `listener-operator`'s CSI driver intercepts the Pod mount, automatically provisions the required Kubernetes `Service`, and projects the resulting public address/port into the Pod's filesystem.

### 4.10.3 Core Value
- **Decoupling**: Developers define *logical* ports (e.g., "WebUI"), while Ops define *physical* exposure strategies via `ListenerClass`.
- **Dynamic Address Awareness**: Applications can read their own external address (e.g., public LoadBalancer IP) from the mounted file, solving the "NAT Advertisement" problem common in distributed systems like Kafka and Zookeeper.

## 4.11 Operational Management Module (ClusterOperation)

### 4.11.1 Design Background

Day-2 operations (maintenance, debugging, emergency stop) require safe and predictable controls over the Operator's behavior. Direct manipulation of underlying resources (e.g., deleting StatefulSets manually) is risky and can conflict with the Operator's reconciliation loop.

### 4.11.2 Core Capabilities

- **Reconciliation Pause (`reconciliationPaused: true`)**:
  - **Mechanism**: The Reconciler checks this flag at the very beginning of the loop (`PreReconcile`). If true, it skips all subsequent logic and status updates.
  - **Use Case**: Allows admins to manually modify underlying K8s resources (e.g., patching a StatefulSet for debugging) without the Operator reverting changes immediately.
- **Graceful Stop (`stopped: true`)**:
  - **Mechanism**: The Reconciler scales all RoleGroup StatefulSets to 0 replicas.
  - **Persistence**: Crucially, **PVCs (Persistent Volume Claims) and ConfigMaps are PRESERVED**. This ensures data safety while freeing up compute resources.
- **Graceful Shutdown**:
  - **Mechanism**: The `gracefulShutdownTimeout` field configures the `terminationGracePeriodSeconds` of the Pod.
  - **Lifecycle Hooks**: The SDK can optionally inject `preStop` hooks to execute application-specific decommissioning logic (e.g., `hdfs dfsadmin -saveNamespace`) before the SIGTERM signal.

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

- **Unified Types**:
  - `S3Connection`: Standard struct for Endpoint, Bucket, Region, and Credential reference.
  - `DatabaseConnection`: Standard struct for Host, Port, Drive Class, and Credential reference.
- **Configuration Rendering**:
  - The SDK automatically converts these high-level objects into application-specific configuration files.
  - *Example*: An `S3Connection` object is transformed into `core-site.xml` properties (`fs.s3a.access.key`, `fs.s3a.endpoint`, etc.) by the **ConfigGenerator**.
- **Credential Resolution**: References to Secrets (e.g., `credentials: secret-name`) are validated and mounted, or resolved to CSI `SecretClass` references for secure injection.

### 4.12.3 Core Value

## 4.13 Error Handling & Resilience Module

### 4.13.1 Design Background

Distributed systems and Kubernetes Controllers face unpredictable failures: network flakiness, API throttling, resource conflicts, and logical errors. A robust SDK must ensure that errors are handled gracefully, ensuring the Controller remains stable (no crashes) and provides feedback (Status updates) without manual intervention.

### 4.13.2 Core Strategies

- **Reconciler Resilience**:
  - **Panic Recovery**: The SDK includes a top-level recovery mechanism to catch panics within the reconciliation loop, preventing the entire Operator process from crashing due to a bug in a specific CR handler.
  - **Exponential Backoff**: Transient errors (e.g., API server timeouts) trigger requeueing with exponentially increasing delays, preventing "thundering herd" issues.

- **Concurrency Control**:
  - **Optimistic Locking**: When updating K8s resources, the SDK handles `Conflict` errors (HTTP 409) caused by concurrent modifications (e.g., mismatched `ResourceVersion`). It employs a "Retry-On-Conflict" utility that automatically refreshes the object and retries the update.
  - **Idempotency**: All side-effect operations (Create/Update/Delete) are designed to be idempotent. A retry after a partial failure is safe and will not result in duplicated resources.

- **Extension Fault Tolerance**:
  - **Fail-Fast**: Critical errors in extensions (e.g., Security configuration failure) bubble up immediately, stopping the reconciliation to prevent an insecure deployment.
  - **Error Propagation**: Errors returned by Extensions are captured and propagated to the CR Status.

- **Status Visibility**:
  - **Condition Mapping**: Top-level errors are automatically mapped to the `Degraded` Condition in `GenericClusterStatus`.
  - **Reasoning**: The `Reason` and `Message` fields of the Condition are populated with the error details, allowing users/admins to diagnose issues (e.g., "DependencyMissing: Zookeeper secret not found") via `kubectl get`.

## 4.14 Event Management Module

### 4.14.1 Design Background

K8s Events provide a chronological log of significant occurrences within the cluster. Unlike Status Conditions (which represent the *current* state), Events record *what happened* (transitions, errors, actions). Systematic event recording is crucial for troubleshooting "Why did it fail 10 minutes ago?".

### 4.14.2 Core Implementation

- **Unified Recorder**: The SDK encapsulates the Kubernetes `EventRecorder` and injects it into the Reconciler context.
- **Automated Lifecycle Events**:
  - **Resource Operations**: The SDK automatically emits `Normal` events whenever it creates, updates, or deletes a sub-resource (StatefulSet, Service, PDB), ensuring auditability without boilerplate code.
  - **Reconciliation Milestones**: Emits events for Reconcile start (debug level), completion, and critical failures.
- **Error Integration**: Any error bubbling up from the Reconciliation loop (including Extensions) that triggers a `Degraded` status automatically generates a `Warning` event with the error reason.

### 4.14.3 Core Value

- **Auditability**: Provides a trace of actions taken by the Operator.
- **Troubleshooting**: Warning events appear directly in `kubectl describe`, giving immediate visibility into failures.

## 4.15 Constants Architecture Module

### 4.15.1 Design Philosophy

The SDK uses a **hybrid constants architecture** that separates cross-cutting constants from domain-specific constants:

- **Cross-cutting constants** (`pkg/constant/`): Shared across all packages — domain name, directory paths, Kubernetes labels, and operational labels (enrichment, restarter).
- **Domain-specific constants** (`pkg/listener/`, `pkg/security/`): Constants meaningful only within their domain — CSI driver names, annotation keys, format/scope types.

All Kubedoop platform constants derive from a single domain constant:

```go
// pkg/constant/domain.go
const KubedoopDomain = "kubedoop.dev"
```

Domain packages derive their constants from this root:

```go
// pkg/listener/volume_builder.go
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
- `LabelRestarterEnable`, `AnnotationSecretRestarterPrefix`, `AnnotationConfigmapRestarterPrefix`, `PrefixLabelRestarterExpiresAt`

**`pkg/listener/`** — Listener operator constants:
- `ListenerAPIGroup`, `ListenerStorageClass`, `CSIDriverName`
- Annotations: `ListenerClassAnnotation`, `ListenerScopeAnnotation`, `AnnotationListenerName`
- Types: `ListenerClass` (cluster-internal, external-stable, external-unstable)
- Builders: `ListenerVolumeBuilder`

**`pkg/security/`** — Secret operator constants:
- `SecretAPIGroup`, `SecretStorageClass`, `CSIDriverName`
- Annotations: `SecretClassAnnotation`, `SecretClassScopeAnnotation`, etc.
- Labels: `LabelSecretsNode`, `LabelSecretsPod`, `LabelSecretsService`
- Types: `SecretFormat` (tls-pem, tls-p12, kerberos), `SecretScope` (pod, node, service, listener-volume)
- Builders: `SecretClassVolumeBuilder`, `SecretVolumeBuilder`

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

- **`ClusterInterface`**: Defines cluster-level operations (GetName, GetNamespace, GetSpec, GetStatus, SetStatus).
- **`RoleInterface`**: Defines role-level operations (GetRoleName, GetConfig, GetRoleGroups).
- **`RoleGroupHandler`**: Defines the `BuildResources()` contract that product operators implement to produce RoleGroup-specific Kubernetes resources.
- **`RoleExtender`**: Defines Role extension points for extending `role.config` fields with product-specific settings.
- **`ServiceHealthCheck`**: Defines health check contract for business-level readiness.

### 5.1.3 Benefits

- **Reduced Implementation Cost**: Product developers implement only the interfaces they need.
- **Interface Clarity**: Each interface has a single, well-defined responsibility.
- **Testability**: Smaller interfaces are easier to mock for unit testing.

### 5.1.4 Example

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

## 5.2 Strategy Pattern

### 5.2.1 Pattern Overview

The Strategy Pattern defines a family of algorithms, encapsulates each one, and makes them interchangeable. The SDK uses this pattern extensively for extension points and configurable behaviors.

### 5.2.2 Application in SDK

- **Extension Interfaces**: Products implement `ClusterExtension`, `RoleExtension`, or `RoleGroupExtension` to inject custom reconciliation logic.
- **ConfigFormat Interface**: Different configuration serializers (XML, Properties, YAML, Env) implement the same interface.
- **SidecarProvider Interface**: Different sidecar injectors (Vector, JMX Exporter) follow a common contract.

### 5.2.3 Benefits

- **Flexibility**: Strategies can be swapped at runtime without modifying the SDK core.
- **Open/Closed Principle**: New strategies can be added without modifying existing code.
- **Isolation**: Each strategy is isolated, making it easier to test and maintain.

### 5.2.4 Example

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

**Resource Application Order (per RoleGroup)**

Within step 3, resources are applied in a strict dependency order:

```
ConfigMap → HeadlessService → Service → StatefulSet → PDB
```

The rationale follows Kubernetes resource dependency rules:

1. **ConfigMap**: Applied first because Pods reference ConfigMaps as volume mounts or environment sources. The configuration data must exist before any Pod starts.
2. **HeadlessService**: A StatefulSet requires a `serviceName` pointing to a headless Service. Kubernetes uses it to create stable, predictable DNS entries (`pod-0.svc.ns.svc.cluster.local`) for inter-pod communication. It must exist before the StatefulSet is created.
3. **Service** (client-facing): Created before the StatefulSet so that client endpoints are available as soon as Pods become ready.
4. **StatefulSet**: Applied after all its dependencies (configs, DNS) are in place. The StatefulSet controller then creates Pods in ordinal order.
5. **PDB** (PodDisruptionBudget): Applied last, as it references existing Pods. It enforces availability guarantees during voluntary disruptions once the workload is running.

This creation order is the inverse of the deletion order used during orphaned resource cleanup (see §4.4.2).

### 5.3.4 Benefits

- **Consistency**: All products follow the same reconciliation structure.
- **Controlled Extension**: Products can only extend at designated points.
- **Maintainability**: Changes to the core flow affect all products uniformly.

## 5.4 Singleton Pattern

### 5.4.1 Pattern Overview

The Singleton Pattern ensures a class has only one instance and provides a global point of access to it.

### 5.4.2 Application in SDK

- **ExtensionRegistry**: Globally unique registry that manages all extensions. Ensures extensions are registered only once and executed in a deterministic order.
- **Scheme**: The Kubernetes scheme is registered once during operator initialization.

### 5.4.3 Benefits

- **Consistency**: Single point of truth for extension management.
- **Deterministic Execution**: Extensions execute in priority order (highest first); registration order is used as a tiebreaker.
- **Thread Safety**: Prevents duplicate registration in concurrent scenarios.

### 5.4.4 Example

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

## 5.5 Builder Pattern

### 5.5.1 Pattern Overview

The Builder Pattern separates the construction of a complex object from its representation, allowing the same construction process to create different representations.

### 5.5.2 Application in SDK

- **StatefulSetBuilder**: Constructs `StatefulSet` resources step-by-step, handling complex configurations like volumes, containers, and affinity rules.
- **ConfigMapBuilder**: Builds ConfigMaps with merged configurations.
- **ServiceBuilder**: Constructs Service resources with appropriate ports and selectors.

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

- **ConfigFormat Adapters**: Convert internal configuration maps to various external formats:
  - `XMLAdapter`: Adapts to Hadoop XML format
  - `PropertiesAdapter`: Adapts to Java .properties format
  - `YAMLAdapter`: Adapts to YAML format
  - `EnvAdapter`: Adapts to environment variable format

### 5.6.3 Benefits

- **Format Independence**: SDK core works with internal map representation.
- **Extensibility**: New formats can be added by implementing the adapter interface.
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
| Interface Segregation | `ClusterInterface`, `RoleInterface` | Focused, implementable contracts |
| Strategy | Extensions, ConfigFormat | Swappable behaviors |
| Template Method | Reconciliation flow | Consistent process with hooks |
| Singleton | ExtensionRegistry | Global state management |
| Builder | StatefulSetBuilder | Complex object construction |
| Adapter | ConfigFormat adapters | Format interoperability |
| Observer | Event recording | Change notification |

# 6. Key Problems and Solutions

- **Runtime errors and code redundancy caused by type assertions**
  - **Solution**: Introduce Go Generics, designing generic reconcilers, extension interfaces, and configuration extenders.
  - **Core Advantage**: Compile-time type safety, reduced boilerplate code, improved development efficiency.

- **Residual orphaned resources after role group deletion**
  - **Solution**: Based on Spec and Status comparison combined with resource existence validation, delete orphaned resources in dependency order.
  - **Core Advantage**: Efficient and precise, avoiding accidental deletion, ensuring state convergence.

- **Repetitive multi-product configuration validation/default value logic**
  - **Solution**: Webhook divided into common and specific logic; SDK provides common tools, product side implements specific interfaces.
  - **Core Advantage**: Logic reuse, flexible extension, intercepting illegal configurations upfront.

- **Complex logic for external infrastructure binding (S3/DB)**
  - **Solution**: Introduce high-level `Connection` abstractions and automatic configuration rendering strategies.
  - **Core Advantage**: Decouples business logic from infrastructure details, reducing configuration complexity and common misconfigurations.

# 7. Deployment and Extension Guide

## 7.1 SDK Deployment Dependencies

- **K8s Version**: 1.31+ (Adapts to Webhook AdmissionReviewVersions=v1).
- **Dependent Components**: cert-manager (for Webhook certificate generation), kubebuilder 3.0+ (for code generation).
- **Permission Requirements**: Operator requires CRUD permissions for resources such as StatefulSet, Service, ConfigMap, etc.

## 7.2 New Product Extension Steps

1. Define the CRD struct, embedding the SDK Generic Spec/Status model.
2. Implement `ClusterInterface`/`RoleInterface` interfaces to adapt to the SDK reconciliation process.
3. (Optional) Implement `ProductDefaulter`/`ProductValidator` interfaces to customize Webhook logic.
4. Register product-specific extensions to implement differentiated business logic.
5. Generate Webhook and CRD configurations via Kubebuilder and deploy for verification.

# 8. Summary and Outlook

## 8.1 Summary of Core Advantages

Through layered architecture, interface-driven design, generics transformation, and extension point mechanisms, this SDK achieves common logic reuse and flexible extension for multi-cluster products. It simultaneously resolves key issues such as orphaned resources, terminology conflicts, and type safety, aligning with K8s ecosystem standards and adapting to production-grade Operator development needs.

## 8.2 Future Optimization Directions

- Support **ConversionWebhook** to achieve smooth CRD version upgrades.
- Extend extension point fault tolerance mechanisms to support degradation strategies when partial extensions fail.
- Add monitoring metrics to statistics extension execution time, resource cleanup counts, etc., facilitating troubleshooting.
- Support gray deletion of role group resources to reduce the risk of accidental deletion.
