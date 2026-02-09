# Operator-Go Security Architecture

## 1. Overview
This document outlines the security architecture integrated into the `operator-go` SDK. It adopts a defense-in-depth approach, split into two primary layers:
1.  **Application Security**: Focused on safely injecting sensitive data (Secrets, Keys) into workloads.
2.  **Infrastructure Security**: Focused on securing the Kubernetes execution environment (RBAC, Service Accounts, Pod Constraints).

---

# 2. Application Security (SecretClass & CSI)

The core design philosophy is **"Zero-Touch Security"**. The Product Operator does not direct handle sensitive data but delegates provisioning to a specialized `secret-operator`.

## 2.1 Core Concept: SecretClass

`SecretClass` is a namespaced resource managed by `secret-operator`. It defines "how" to obtain security artifacts, while the workload (Pod) simply declares "what" it needs by referencing a `SecretClass`.

This mechanism is implemented using the **Kubernetes CSI (Container Storage Interface)**. The `secret-operator` provides a CSI driver that intercepts volume mount requests, generates or retrieves the required secrets on-the-fly, and injects them into the container file system as files.

### 2.1.1 Workflow

1. **Definition**: Admin creates a `SecretClass` containing the policy (e.g., "Issue certificates using ClusterIssuer 'my-ca'").
2. **Reference**: The Product CR (e.g., HdfsCluster) specifies `secretClass: "hdfs-secret-class"`.
3. **Mount**: The Operator SDK constructs the StatefulSet with a CSI Volume referencing this `SecretClass`.
4. **Injection**: When a Pod starts, the CSI driver calls the backend to generate artifacts (TLS certs, Keytabs) and mounts them to `/etc/secret-volume`.

## 2.2 Supported Security Backends
The SDK and `secret-operator` support multiple backends to address different security needs:

### 2.2.1 AutoTLS (Automatic Certificate Management)

Calculates and issues TLS certificates for components.

- **Scenario**: Internal mTLS communication (e.g., DataNode <-> NameNode) or external HTTPS access.
- **Mechanism**:
  - Automatically generates SANs (Subject Alternative Names) based on Pod DNS names (e.g., `*.hdfs.svc.cluster.local`).
  - Solves the comprehensive trust problem: Components from different products (e.g., Flink connecting to HDFS) can trust each other if they use `SecretClasses` signed by the same Root CA.

### 2.2.2 KerberosKeytab (Identity Provisioning)

Automates Kerberos integration for Hadoop/Big Data ecosystems.

- **Scenario**: Secure clusters requiring Kerberos authentication.
- **Mechanism**:
  - **Dynamic Principal**: Supports generating principals based on the Pod's specific hostname (e.g., `nn/hdfs-namenode-0.hdfs.svc@REALM`). This is critical for K8s StatefulSets where Pod names are deterministic but distinct.
  - **Keytab Injection**: Generates the keytab on the KDC and securely mounts it to the container.

### 2.2.3 K8sSearch (Secret Projection)

Searches and injects existing Kubernetes Secrets or ConfigMaps.

- **Scenario**: Legacy applications or reusing existing static secrets.
- **Mechanism**:
  - Searches for resources matching specific labels or names in the cluster.
  - **Security Benefit**: The Product Operator does not need `LIST/WATCH/GET Secret` permissions for the entire namespace. Only the privileged `secret-operator` accesses the data, minimizing the attack surface.

### 2.2.4 OIDC (OpenID Connect) Integration

Automates the injection of Identity Provider (IdP) credentials.

- **Scenario**: Workloads requiring modern authentication (e.g., Trino interacting with external Data Lakes, or Presto Web UI login).
- **Mechanism**:
  - **Credential Injection**: The `secret-operator` mounts the OIDC client credentials (client-id, client-secret) from a reference secret into the container.
  - **Configuration**: The Operator SDK automatically configures necessary environmental variables or JVM system properties (e.g., `-Dsolr.authentication.oidc.client.secret=...`) to enable the OIDC module in the application.

---

# 3. Infrastructure Security

This layer focuses on how the Operator constructs the Kubernetes Pods and Resources to minimize the attack surface and ensure proper isolation.

## 3.1 Workload Identity (Service Accounts)

Every specific Product Cluster managed by the SDK operates with its own distinct identity.

- **Automated Provisioning**: The SDK automatically creates a dedicated `ServiceAccount` for each Cluster instance (or specific RoleGroup, depending on configuration).
- **Scope**: Pods run as this ServiceAccount, meaning any audit logs in Kubernetes will reflect the specific application identity rather than a generic "default" account.
- **Customization**: Users can override the generated ServiceAccount name in the CR Spec if integration with external IAM (like AWS IRSA or Google Workload Identity) is required.

## 3.2 RBAC Integration (Principle of Least Privilege)

Workloads often need to interact with the Kubernetes API (e.g., Flink JobManager creating generic Jobs, Spark driver creating executor pods).

- **Dynamic Binding**: The SDK allows Products to define the *exact* RBAC permissions required by their workloads. The Operator then creates `Role` and `RoleBinding` resources linking the workload's `ServiceAccount` to these permissions.
- **Benefit**: No manual `kubectl create rolebinding` is needed, yet the permissions are scoped strictly to what the application declares it needs, preventing over-privileged pods.

## 3.3 Pod Security Guidelines

The SDK generates `PodSpecs` that adhere to modern container security best practices.

- **Non-Root Execution**:
  - By default, the SDK configures `securityContext.runAsUser` and `runAsGroup` to non-zero values (typically 1001), ensuring processes do not run as root.
- **Volume Ownership (fsGroup)**:
  - To ensure that the non-root process can write to Persistent Volumes, the SDK automatically sets `securityContext.fsGroup`. This instructs Kubernetes to change the ownership of mounted volumes to the correct group ID at startup.
- **Capability Dropping**:
  - Where possible, the SDK encourages dropping unnecessary Linux capabilities (e.g., `ALL`) and only adding required ones (e.g., `NET_BIND_SERVICE`).

## 3.4 Security Benefits Summary

- **Access Isolation**: Product Operators operate with minimal RBAC privileges, reducing the blast radius if an operator is compromised.
- **Lifecycle Management**: Certificates are automatically renewed by the `secret-operator` without restarting Pods (if the application supports hot-reload) or via simple rolling restarts.
- **Consistency**: Standardizes security configurations across all data products (HDFS, Hive, Trino, etc.).
