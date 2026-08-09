# Operator-Go Security Architecture

## 1. Overview
This document outlines the security architecture integrated into the `operator-go` SDK. It adopts a defense-in-depth approach, split into two primary layers:
1.  **Application Security**: Focused on safely injecting sensitive data (Secrets, Keys) into workloads.
2.  **Infrastructure Security**: Focused on securing the Kubernetes execution environment (RBAC, Service Accounts, Pod Constraints).

---

# 2. Application Security (SecretClass & CSI)

The core design philosophy is **"Zero-Touch Security"**. The Product Operator does not directly handle sensitive data; it delegates provisioning to a specialized `secret-operator`.

## 2.1 Core Concept: SecretClass

`SecretClass` is a resource managed by `secret-operator`. It defines "how" to obtain security artifacts, while the workload (Pod) simply declares "what" it needs by referencing a `SecretClass` **by name**. The CRD itself — its scope and schema — is owned by the `secret-operator`, not by this SDK; `operator-go` only emits the `secrets.kubedoop.dev/class: <name>` annotation and never reads the object.

This mechanism is implemented using the **Kubernetes CSI (Container Storage Interface)**. The `secret-operator` provides a CSI driver that intercepts volume mount requests, generates or retrieves the required secrets on-the-fly, and injects them into the container file system as files.

### 2.1.1 Workflow

1. **Definition**: Admin creates a `SecretClass` containing the policy (e.g., "Issue certificates using ClusterIssuer 'my-ca'").
2. **Reference**: The Product CR (e.g., HdfsCluster) specifies `secretClass: "hdfs-secret-class"`.
3. **Declaration**: The Product Operator registers the need on a `security.SecretProvisioner` and
   appends it to `RoleGroupBuildContext.VolumeProviders` (or calls `AutoInject(stsBuilder)`). The SDK
   then emits, on the Pod template, a **generic ephemeral volume** whose `volumeClaimTemplate`
   carries the secret-operator annotations (`secrets.kubedoop.dev/class`, and `…/scope`,
   `…/format`, … when set) and the `secrets.kubedoop.dev` StorageClass. Kubernetes' ephemeral-volume
   controller materializes one Pod-owned PVC per Pod, so the operator needs no PVC create RBAC and
   the PVC lifecycle is Pod-bound. This step is **opt-in**: nothing is mounted unless the product
   registers a volume.
4. **Injection**: When a Pod starts, the CSI driver calls the backend to generate artifacts (TLS
   certs, Keytabs) and mounts them **read-only** under the SDK's canonical base,
   `/kubedoop/mount/<volumeName>` (`constant.KubedoopMountDir`, overridable per provisioner with
   `SecretProvisioner.WithMountBasePath`).

> **Never hardcode the mount path.** Ask the provisioner:
> `provisioner.Path("server-tls")` (or `MustPath`) returns `/kubedoop/mount/server-tls`, and stays
> correct when the base path is overridden. Some helpers deliberately use a different base — e.g.
> `s3.ConnectionInfo.CredentialsProvisioner` mounts under `constant.KubedoopSecretDir`
> (`/kubedoop/secret/<volume>`) — which is exactly why the path must come from the API.

### 2.1.2 Declaring a Secret Volume

`security` ships one constructor per common need. Each requests a 10Mi PVC and sets the scope shown
below (`CredentialsVolume` deliberately sets none — credential secrets are usually class-resolved;
add one with `WithScope`, e.g. from `ScopeString`):

| Constructor | Format | Default scope |
| --- | --- | --- |
| `TLS(volumeName, secretClass)` | `tls-p12` | `pod,node` |
| `TLSPEMFormat(volumeName, secretClass)` | `tls-pem` | `pod,node` |
| `ServiceTLS(volumeName, secretClass, serviceName)` | `tls-p12` | `pod,node,service=<serviceName>` |
| `KerberosVolume(volumeName, secretClass, serviceName, …)` | `kerberos` | `pod,node` |
| `ListenerVolume(volumeName, secretClass, listenerVolumeName, format)` | caller-supplied | `listener-volume=<listenerVolumeName>` |
| `Custom(volumeName, secretClass, format)` | caller-supplied | `pod,node` |
| `CredentialsVolume(volumeName, secretClass)` | none | none |

**Named scopes must carry a name.** The secret-operator parses `service` and `listener-volume`
entries as `key=<value>` and unconditionally reads the value, so a bare `service` or
`listener-volume` entry is unresolvable. The SDK refuses to build one:

- `ListenerVolume` **requires** `listenerVolumeName` (the name of the Pod volume that mounts the
  listener, which the secret-operator resolves to that listener's addresses) and panics on an empty
  string. The emitted scope is `listener-volume=<listenerVolumeName>`.
- `SecretVolumeRegistration.WithScope` and `SecretProvisioner.Register` panic on a scope whose
  `service` / `listener-volume` entry has no value, and on an empty comma entry. Unknown scope keys
  pass — the scope vocabulary belongs to the secret-operator and may grow ahead of this SDK.
- `security.ScopeString(*commonsv1alpha1.CredentialsScope)` renders a CRD-declared scope
  (`node`, `pod`, `services`, `listenerVolumes`) into that annotation value, emitting the `key=`
  prefix for the named entries. It returns `""` for a nil/empty scope, in which case the annotation
  is omitted.

**A scope name may contain neither `,` nor `=`.** The annotation is one comma-delimited string of
`key=value` entries with no escaping, so a name carrying either character does not quote itself —
it **adds scopes**. `services: ["mysvc,node"]` renders `service=mysvc,node`, which the
secret-operator reads as a service scope *and* a **node** scope: the CR author silently receives a
certificate covering the node's hostname and IP, and a reviewer reading the CR sees nothing
unusual. Two layers stop that:

- `CredentialsScope.Services` and `.ListenerVolumes` carry `+kubebuilder:validation:items:Pattern`
  (`^[^,=]+$`) and `items:MinLength=1`, so the **API server rejects it at `kubectl apply`** — where
  the user sees the error and can fix it. This is the real defence.
- `ScopeString` drops any entry it cannot render as itself, covering what admission cannot: a CR
  stored before those markers existed, and a scope built in Go. Dropping is the safe direction —
  splicing grants a *broader* credential than requested, invisibly, while dropping withholds a
  requested scope, which surfaces as the application rejecting the certificate.

The default PKCS12 password (`changeit`) is stored as a **PVC template annotation** and is therefore
readable by anyone with `get` on PVCs. Use `WithPassword` or `WithNoPassword`.

Certificate rotation is configured on the registration (`WithCertLifetime`, `WithCertJitter`,
`WithCertBuffer`), which the secret-operator honours when issuing the artifact.

Listener volumes themselves are declared through `listener.NewVolume(name, class)` plus optional
`.WithListenerName(name)` on a `listener.ListenerProvisioner`. **`pkg/listener` has no scope API** —
there is no `WithScope`, no `ListenerScope` type and no `listeners.kubedoop.dev/scope` annotation on
listener PVC templates. Scoping a *secret* to a listener is done on the secret side, with
`ListenerVolume` above.

## 2.2 Supported Security Backends

The backends below are implemented by the `secret-operator`; the SDK's part is declaring the volume
and its annotations (§2.1.2) and resolving the mount path. Backend selection is a property of the
`SecretClass` the admin creates, not of the SDK call.

> The mechanisms described in §2.2.1–§2.2.3 are **`secret-operator` behavior**, documented here so
> product authors know what the platform provides. They are not implemented in `operator-go` and
> cannot be verified against this repository — consult the `secret-operator` documentation for the
> authoritative contract. Only the `security.*` API names and mount paths in §2.1 are SDK behavior.

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

## 2.3 OIDC (OpenID Connect) Integration

Unlike §2.2, OIDC is **not** a secret-operator CSI backend in this SDK. It is delivered by the
`sidecar.OAuth2ProxySidecarProvider`, which terminates the login flow in an oauth2-proxy sidecar in
front of the product's HTTP port.

- **Scenario**: Workloads requiring modern authentication (e.g., a product Web UI behind an IdP).
- **Mechanism**:
  - **Configuration source**: an `AuthenticationClass` with an `OIDCProvider`
    (`pkg/apis/authentication/v1alpha1`). The product resolves the class and passes the provider to
    `sidecar.NewOAuth2ProxySidecarProvider(oidcProvider, clientCredentialsSecret, upstreamPort,
    opts...)`.
  - **Credential injection**: a **plain Kubernetes Secret** named by `clientCredentialsSecret`,
    carrying the keys `CLIENT_ID`, `CLIENT_SECRET` and `COOKIE_SECRET` (`sidecar.OIDCClientIDKey` /
    `OIDCClientSecretKey` / `OIDCCookieSecretKey`). All three reach the sidecar as
    `OAUTH2_PROXY_*` env vars via `secretKeyRef` — they are **not** mounted through the
    secret-operator CSI driver, and they are never written into a config file or inlined into the
    PodSpec. `Validate` fails the reconcile before the StatefulSet is applied if the Secret is
    missing, or if it does not carry the cookie-secret key.
  - **Configuration of the proxy**: the provider sets the `OAUTH2_PROXY_*` env vars (issuer URL,
    scopes, provider hint, upstream, listen address, PKCE `S256`). The SDK does **not** inject JVM
    system properties or configure the product application's own OIDC module — a product that needs
    in-application OIDC generates that configuration itself.
  - **Cookie secret**: read from the Secret above (relocate it with
    `WithOAuth2ProxyCookieSecretRef`). It is **never** derived from the CR and never inlined: this
    value signs every session cookie the proxy subsequently trusts, so anything readable through
    the API — a UID, a name, an env `value` in the PodSpec — would let a reader forge a session and
    bypass authentication entirely. Generate it once with `sidecar.GenerateCookieSecret()` and
    store it; the value must stay stable, or each reconcile would roll the pods and log every user
    out.
  - **Authorization**: authenticating against an IdP is not authorization for this cluster. On a
    shared realm, every account the IdP can issue a token for would otherwise reach the product, so
    the provider requires an explicit policy — `WithOAuth2ProxyEmailDomains(...)` to restrict
    logins, or `WithOAuth2ProxyAllowAllEmails()` to admit everyone deliberately. **Exactly one**:
    building a proxy with neither fails, and so does building one with both, because "allow all"
    would otherwise win and silently discard the domain list. Post-login redirects are likewise
    closed by default (the proxy's own host only); `WithOAuth2ProxyWhitelistDomains(...)` widens
    that, one domain at a time.
  - **Probes**: the sidecar carries a **readiness** probe and a **liveness** probe on `/ping` —
    never `/ready`, which oauth2-proxy documents as a deep health check and which would make a
    runtime IdP outage evict the pod from every Service. Readiness is the right gate here, and this
    is an availability property with a security edge: this container terminates client traffic, but
    pod readiness is otherwise decided by the *main* container's probe on the product's own port, so
    without it the pod joined its Services — and received requests — while the proxy was still
    starting. Requests arriving then are refused rather than authenticated, which on a rollout looks
    to clients like an outage and to an operator like a working auth layer. There is deliberately no
    **startup** probe: as a native sidecar, a proxy that could never satisfy it would stop the
    product's own container from ever starting.

---

# 3. Infrastructure Security

This layer focuses on how the Operator constructs the Kubernetes Pods and Resources to minimize the attack surface and ensure proper isolation.

## 3.1 Workload Identity (Service Accounts)

A Product Cluster managed by the SDK can operate with its own distinct identity.

- **Unconditional Provisioning**: every cluster gets a workload `ServiceAccount`. The reconciler
  creates (or updates) it in the CR's namespace with the CR as controller owner at step 0 of the
  reconcile — before any extension hook or role is processed — and propagates the name through
  `RoleGroupBuildContext.ServiceAccountName` into the Pod template. There is no switch to turn it
  off: a ServiceAccount is pure metadata, and the opt-in it replaced mostly produced clusters
  running as the namespace `default` by accident, which is the opposite of what this section
  promises.
- **The name is DERIVED, not configured**: `reconciler.ServiceAccountResourceName(kind, cluster)`
  yields `"<lowercased kind>-<cluster>"` — an `HdfsCluster` named `prod` runs as
  `hdfscluster-prod`. The Kind is part of it because a CR name alone is not unique in a namespace:
  an `HdfsCluster` and a `TrinoCluster` both named `prod` would otherwise select one ServiceAccount
  and the second controller could never take ownership of it.

  This follows the framework's general rule that it owns the name of every slot it owns the
  lifecycle of (`docs/architecture.md` §3): the framework creates the object, controller-owns it and
  garbage-collects it with the CR, so nothing needs to address it by a product-chosen name. It also
  removes a whole failure class rather than detecting it — a configurable *static* name was a
  constant, so every CR of a product in one namespace resolved to the SAME ServiceAccount: the
  second cluster failed with `AlreadyOwnedError` forever, and deleting the first garbage-collected
  the SA out from under the second's running pods.
- **Granularity is per CR, not per RoleGroup.** One identity per cluster; there is no per-role or
  per-role-group ServiceAccount.
- **Scope**: Pods run as this ServiceAccount, so Kubernetes audit logs reflect the specific
  application identity rather than a generic `default` account.
- **Predictability is part of the contract**: `ServiceAccountResourceName` is exported so that
  anything outside the operator that must name this ServiceAccount — a hand-written RoleBinding, an
  admission policy, an audit query — derives it from the formula rather than from the operator's
  source.
- **Not user-overridable**: the common CRD types (`GenericClusterSpec`) carry **no**
  `serviceAccountName` field, and there is no config field either. The identity is the framework's.

## 3.2 Workload RBAC (Principle of Least Privilege)

Workloads often need to interact with the Kubernetes API (e.g., a Flink JobManager creating Jobs, a
Spark driver creating executor pods, a member electing a leader through a Lease).

This is **the other half of §3.1**. That section gives the workload an *identity*; this one gives
that identity its *permissions*. The framework maintains both at the same point in the same way —
once per CR, at cluster level, before any role is built — and the role group merely consumes the
result.

- **The product declares rules; the framework owns everything else.** Set
  `GenericReconcilerConfig.WorkloadRBACRules func(cr CR) []rbacv1.PolicyRule`. The framework
  maintains a namespaced `Role` and `RoleBinding`, named after the derived ServiceAccount, in the
  CR's namespace, with the CR as **controller** owner. The split is deliberate: only the product
  knows what its pods call, and only the framework can guarantee the naming, the ownership, the
  idempotent apply and the revocation.

  ```go
  WorkloadRBACRules: func(cr *v1alpha1.NifiCluster) []rbacv1.PolicyRule {
      return []rbacv1.PolicyRule{{
          APIGroups: []string{"coordination.k8s.io"},
          Resources: []string{"leases"},
          Verbs:     []string{"get", "list", "watch", "create", "update"},
      }}
  },
  ```

- **The framework passes the ServiceAccount name it derived itself**, so the identity and its
  permissions cannot drift apart. That is why this is a config field rather than only the exported
  `reconciler.EnsureWorkloadRBAC` helper: the helper takes the name as a parameter, so a caller can
  pass one that is not the workload's, producing a Role that grants to a ServiceAccount no pod uses —
  two objects that both exist, pods that start fine, and a 403 on the first API call. The helper
  stays exported for a product driving the reconciler's pieces itself.

- **Namespaced only, by construction.** There is no `ClusterRole` path: a namespaced CR cannot
  controller-own a cluster-scoped object, so the framework would have no lifecycle for one — no
  garbage collection with the cluster, and no ownership gate on the reclaim. A product that genuinely
  needs cluster-scoped permissions maintains those objects itself, including their cleanup.

- **An empty rule set REVOKES.** Returning `nil` or an empty slice deletes both objects (gated on
  this CR controller-owning them). Like every other optional slot in the framework — a nil
  `MetricsService` reclaims the Service — nil is an instruction, not "leave it alone". Without that
  reading, narrowing a rule set to nothing would be the one change that silently did nothing while
  the pods kept permissions the product had stopped granting. A **nil hook** is different from a hook
  returning empty: nil means the product never opted in and no RBAC object is touched at all.

- **A pre-existing RoleBinding is never adopted.** `roleRef` is immutable, so a RoleBinding already
  at this name pointing elsewhere cannot be converged. The framework fails with a
  `*reconciler.ValidationError` naming both refs and the command that fixes it, rather than rewriting
  only the subject — which would hand this cluster's pods whatever the old ref allows, and report
  success.

- **The operator must hold what it grants.** Kubernetes refuses to let a subject grant permissions it
  does not itself hold, so the operator's own ClusterRole must be a superset of every rule passed
  here. This is invisible at compile time and invisible in any test running as cluster-admin (envtest
  does), so it needs **two** kinds of marker:

  ```go
  // the rules being granted — Kubernetes forbids granting what the granter lacks
  // +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update
  // write access to the RBAC API itself, without which nothing here can be created at all
  // +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
  ```

  A 403 has those two distinct causes needing opposite fixes, so the SDK re-explains the API server's
  own message rather than pre-checking: that message has already computed rule covering against the
  operator's real effective permissions (wildcards, `resourceNames`, aggregated ClusterRoles) and
  names the missing rule.

- **Watches are registered only when the hook is set.** An unconditional `Owns(&rbacv1.Role{})` would
  force *every* operator built on this SDK to grant itself cluster-wide `list;watch` on roles and
  rolebindings — and a forbidden informer fails `WaitForCacheSync` for **all** sources, so
  `manager.Start` returns and the process exits. Operators that never use the feature pay nothing.

- **The RBAC builders remain** (`builder.RoleBuilder`, `RoleBindingBuilder`, `ClusterRoleBuilder`,
  `ClusterRoleBindingBuilder`) for products assembling RBAC objects the framework does not maintain.
  They write nothing to the API server.

## 3.3 Pod Security Guidelines

The SDK generates `PodSpecs` that adhere to modern container security best practices. The base
role-group handler applies a **single, canonical default** pod/container `SecurityContext` with no
opt-in required. This default hardcodes the kubedoop org-standard identity, because all kubedoop
product images run as uid `1001`.

The pod-level context lands on `.spec.template.spec.securityContext` (so it covers every container
in the Pod); the container-level context is set on the **primary container** the base handler
builds. Sidecar containers carry whatever `SidecarConfig.SecurityContext` their provider was given.

### 3.3.1 Default SecurityContext

**Pod-level (`spec.securityContext`):**

| Field | Value | Rationale |
| --- | --- | --- |
| `runAsUser` | `1001` | kubedoop images run as uid 1001 |
| `runAsGroup` | `0` | OpenShift-compatible: OpenShift assigns an arbitrary uid but keeps gid 0, and kubedoop images are group-0 readable/writable |
| `fsGroup` | `1001` | mounted volumes are chowned so the non-root process can write to them |
| `fsGroupChangePolicy` | `OnRootMismatch` | apply that chown **once**, not on every pod start — see below |
| `runAsNonRoot` | `true` | refuse to start as root |
| `seccompProfile.type` | `RuntimeDefault` | apply the runtime's default seccomp profile |

`fsGroupChangePolicy` is paired with `fsGroup` deliberately. Kubernetes documents the field as
"Valid values are `OnRootMismatch` and `Always`. If not specified, `Always` is used" — and `Always`
means the kubelet walks the **entire** volume, chown'ing and chmod'ing every file, before the
container starts. On a data PVC holding millions of files (an HDFS DataNode, a Kafka broker) that
is tens of minutes to hours, repeated on every restart, every rollout and every node drain, while
the pod sits in `ContainerCreating` with nothing in its events explaining why.

`OnRootMismatch` performs the same recursion only when the volume's root directory does not already
have the expected owner and mode — true exactly once, the first time a freshly provisioned volume is
mounted. The trade-off is deliberate: ownership that drifts *inside* a volume whose root is still
correct will not be repaired. That is a repair the framework never promised, and paying for it on
every start of every stateful pod is the wrong price. A product that wants it back sets
`fsGroupChangePolicy: Always` through `PodOverrides` (§3.3.2).

The policy has no effect on ephemeral volume types (secret, configMap, emptyDir), so the config
mount and the shared log volume behave identically either way; the data PVC is what it is about.

**Container-level (`container.securityContext`):**

| Field | Value | Rationale |
| --- | --- | --- |
| `runAsUser` | `1001` | kubedoop images run as uid 1001 |
| `runAsGroup` | `0` | OpenShift-compatible group 0 |
| `runAsNonRoot` | `true` | refuse to start as root |
| `allowPrivilegeEscalation` | `false` | block privilege escalation |
| `capabilities.drop` | `[ALL]` | drop all Linux capabilities |
| `seccompProfile.type` | `RuntimeDefault` | apply the runtime's default seccomp profile |

### 3.3.2 Overriding via PodOverrides (strategic-merge semantics)

Products customize the security context through `MergedConfig.PodOverrides`, which is applied
as a Kubernetes **Strategic Merge Patch** (the merge strategy `docs/architecture.md` §2.5
documents for the whole pod template). **Security contexts deep-merge per field**: an override
stating only `runAsUser` keeps the framework-hardened remainder (`runAsNonRoot`,
`capabilities.drop`, `seccompProfile`, …).

A field the override does not mention keeps its default, so an override must **explicitly
restate** any default it wants to change — e.g. an image that must run as root sets both
`runAsUser: 0` **and** `runAsNonRoot: false`; setting only `runAsUser: 0` inherits
`runAsNonRoot: true` and the kubelet refuses to start the container.

Two handler-wide escape hatches sit alongside the per-role-group overrides:
`BaseRoleGroupHandler.WithSecurityContext(containerCtx, podCtx)` replaces the defaults for every
role group, and `WithoutDefaultSecurityContext()` disables them entirely (the StatefulSet is then
built with no SecurityContext unless `PodOverrides` supplies one).

## 3.4 Security Benefits Summary

- **Access Isolation**: Product Operators operate with minimal RBAC privileges, reducing the blast radius if an operator is compromised.
- **Consistency**: Standardizes security configurations across all data products (HDFS, Hive, Trino, etc.).
- **Lifecycle Management**: certificate *issuance* and *renewal* are the `secret-operator`'s job;
  the SDK's part is the rotation annotations on the registration (`WithCertLifetime`,
  `WithCertJitter`, `WithCertBuffer`). Propagating a renewal into a running Pod is handled by the
  separate **restarter** component, whose contract `pkg/constant/restarter.go` defines
  (`restarter.kubedoop.dev/enable=true` on the workload; `secret.restarter.kubedoop.dev/*` and
  `configmap.restarter.kubedoop.dev/*` annotations; `restarter.kubedoop.dev/expires-at.*` for
  expiry-driven Pod restarts).

  > **Not applied by the SDK.** `pkg/constant` only declares these names — no builder or reconciler
  > sets `restarter.kubedoop.dev/enable` on the StatefulSet. Enabling it is a deployment decision:
  > label the **cluster CR**, and the reconciler propagates the CR's labels into every resource it
  > builds, including the `StatefulSet.metadata.labels` the restarter watches. (Its watch predicate
  > and its `MatchingLabels` list both read object metadata, so a pod-template-only label —
  > everything `podOverrides` can reach — enables nothing.) Without the label *and* a deployed
  > restarter, a rotated certificate reaches the container only if the application hot-reloads it,
  > or on the next manual rolling restart.
