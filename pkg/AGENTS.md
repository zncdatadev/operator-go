# operator-go/pkg - Package Overview

**Parent:** [../AGENTS.md](../AGENTS.md)

Core packages for the operator framework, including CRD definitions, resource builders,
reconciliation logic, and configuration generation.

## Key Packages

Every directory under `pkg/`:

| Package | Purpose | Detail |
|---------|---------|--------|
| `apis/` | CRD definitions and API types (`authentication`, `commons`, `database`, `listeners`, `s3`) | [apis/AGENTS.md](apis/AGENTS.md) |
| `builder/` | Kubernetes resource builders (StatefulSet, Service, ConfigMap, PDB, RBAC, ServiceAccount, metrics Service) | [builder/AGENTS.md](builder/AGENTS.md) |
| `common/` | Framework interfaces: `ClusterInterface` / `ClusterResource[T]`, the extension system and its per-CR-type `ExtensionRegistry[CR]`, `ServiceHealthCheck`, shared error types | |
| `config/` | Config-file serialization (XML/Properties/YAML/Env/INI) and the layered override merge | [config/AGENTS.md](config/AGENTS.md) |
| `constant/` | Kubedoop path, label, domain and restarter constants (`KubedoopMountDir`, `KubedoopSecretDir`, …) | |
| `listener/` | Listener CSI volume registration and `ListenerProvisioner` | |
| `productlogging/` | Product logging config generation (Log4j, Log4j2, Logback, Python) and `ContainerLogging` | |
| `reconciler/` | `GenericReconciler` framework, handlers, cleaner, health, dependencies, apply semantics | [reconciler/AGENTS.md](reconciler/AGENTS.md) |
| `s3/` | `S3Connection`/`S3Bucket` resolution, S3A properties, CSI credential wiring | |
| `security/` | `SecretProvisioner` (secret-operator CSI volumes), secret-class constants, pod SecurityContext defaults | |
| `sidecar/` | `SidecarManager`, `SidecarProvider`/`PhasedProvider`, JMX exporter and oauth2-proxy providers | |
| `testutil/` | envtest harness, mocks, matchers, CR builders, the exported CRD-default guard (`HaveNoInheritedConfigDefaults`) | |
| `util/` | `K8sUtil` (CreateOrUpdate, status update with retry) and `ExecUtil` (in-pod exec, consumer-facing) | |
| `vector/` | Vector agent sidecar: config rendering, aggregator discovery, `VectorSidecarProvider` | |
| `webhook/` | Webhook infrastructure: `WebhookManager`, `ProductDefaulter`/`ProductValidator`, common image defaults/validators | |

## Package Boundaries

- **`pkg/common` holds interfaces, `pkg/reconciler` holds the loop.** A type that both the SDK and a
  product implement belongs in `common`; anything that talks to the API server on the reconcile path
  belongs in `reconciler`.
- **`pkg/builder` never writes to the API server.** Builders produce objects; `pkg/reconciler`
  applies them.
- **`pkg/util.ExecUtil` is a consumer-facing helper, not a reconcile-loop component.** Nothing in
  `pkg/reconciler` or `pkg/common` constructs it, and the health path is given a `client.Client`
  rather than a `*rest.Config` — a product that wants exec-based checks builds
  `util.NewExecUtil(client, restConfig)` itself.
- **`pkg/s3` rendering is opt-in.** It resolves inline-or-reference connections and offers
  `S3AProperties()` / `S3AURI()` / `CredentialsProvisioner()`; the product merges the results into
  its own config. `pkg/config`'s generators know nothing about connection objects.

  `S3AProperties()` renders `fs.s3a.path.style.access` from `spec.pathStyle`, whose CRD default is
  **`false`** (virtual-host addressing). Every product implementation it replaces pinned that key
  to `true`, because MinIO serves path-style only — virtual-host addressing resolves
  `<bucket>.<host>` and gets NXDOMAIN. A product adopting this method therefore changes behaviour
  for existing clusters whose `S3Connection` omits `pathStyle: true`, and the failure appears at
  first bucket access rather than at admission.

## Working Instructions

1. **Adding a new resource builder:** create it in `builder/` following the pattern of the existing
   builders (constructor, chainable setters, deep-copying `Build()`, `NamespacedName()`).
2. **Adding reconciliation logic:** extend `reconciler/`. Product-specific behavior goes through
   `RoleGroupHandler`, the extension hooks or `GenericReconcilerConfig` fields — not by forking the
   loop.
3. **Adding config generation:** add an adapter in `config/` implementing `config.ConfigMarshaler`
   (`Marshal`). `config.ConfigUnmarshaler` (`Unmarshal`) is optional — add it only when something
   actually reads that format back; the parse paths discover it at runtime and otherwise fail with
   an `*UnsupportedParseError`. Adapters shipped by the SDK implement both.
4. **Adding utilities:** put framework interfaces in `common/`, Kubernetes plumbing in `util/`, and
   shared literals in `constant/` rather than hardcoding paths or label keys.
