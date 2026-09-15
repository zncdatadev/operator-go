# Trino operator example

This module is the Trino reference product for the formal `operator-go/pkg/framework` API. The executable registers its generated API and `product.Definition()` with the public operator package. The SDK owns folding, resource assembly, apply, observation, stop/pause and retained-resource safety; the product owns Trino configuration and its read-only catalog, authentication and S3 resolvers.

The reference image is `quay.io/zncdatadev/trino:476-kubedoop0.0.0-dev`. Its launcher is `/kubedoop/trino-server/bin/launcher`, running as UID/GID 1001. It uses one coordinator group with one replica and any number of worker groups. Default catalogs contain the bundled `tpch` connector. Catalog declarations do not establish plugin availability, credentials or external service readiness.

## Product and generated input

The author-maintained product lives in `internal/product/`:

- `TrinoConfig`: role/group HTTP port, catalog reference, explicit shutdown identity and typed Hive/S3 configuration.
- `TrinoClusterConfig`: cluster-wide environment, ListenerClass, TLS source and internal shared-secret reference.
- `TrinoFacts`: resolved catalogs, authentication and S3 references, supplied independently for each group.
- `Definition()`: image and role defaults, product validation, runtime/files, final relationship checks and shared discovery output.
- `ResolveFacts`: combines referenced `catalogs.json`, authentication and S3 inputs for each group; missing/deleting required sources remain Pending. It does not write Kubernetes resources.

`cmd/generate` derives the API role list from the definition and generates the input types, CRD, binding and registration companion. Generated presence fields preserve `false`, zero, empty strings and empty collections. `Operation` reads fixed controls before product projection. The generated raw projection does not merge role layers or resolve facts.

```sh
make generate
make verify-generate
make test
make lint
make build
```

`make test` uses the root checkout's Kubernetes 1.35 envtest binaries, or an explicit `KUBEBUILDER_ASSETS`. The published sample is tested through YAML-to-JSON decoding and real API-server create/get/projection. These tests do not start Trino or prove query readiness. Runtime delivery validation is maintained separately by the repository's [framework acceptance harness](../../hack/framework-e2e/README.md).

The module retains a local SDK `replace` to `../..`; generated files are checked into this module. There is no alternate legacy handler, extension registry, webhook or controller entrypoint.

## Deploy the actual executable

Build the materializer from the same SDK revision as the operator. The materializer and operator images contain statically compiled Linux executables, so no Go builder image is needed:

```sh
make -C ../.. materializer-image
make docker-build IMG=trino-operator:dev
make build-installer IMG=trino-operator:dev
kubectl apply --validate=strict -f dist/install.yaml
kubectl apply --validate=strict -f config/samples/trino_v1alpha1_trinocluster.yaml
```

Images must be available to the target cluster: load locally built images into a local cluster, or tag/push them to a reachable registry. `TARGETARCH` controls the operator cross-compile and image platform; `MATERIALIZER_ARCH` controls the SDK helper. The installer uses the fixed Vector digest declared in `config/manager/manager.yaml`. Change image references in a deployment overlay for another platform/release.

The installer creates namespace `trino-operator-system`, Deployment `trino-operator-controller-manager`, its ServiceAccount/RBAC, `trinoclusters.trino.kubedoop.dev` and independent DataAsset/DataOperation CRDs. It does not deploy the destructive data executor. It has no admission-webhook or cert-manager dependency. `make deploy` builds and applies the installer. The operator defaults to watching all namespaces; `--namespace` limits the manager watch.

For a local process, pass the same helper images explicitly:

```sh
make run ARGS='--materializer-image=quay.io/zncdatadev/operator-go-materializer:0.0.0-dev --vector-image=quay.io/zncdatadev/vector@sha256:3b9a99d98905443924bee204bd76c2818ad2da7056388fd524b0ea000eb55682'
```

Other options include `--fact-refresh-interval` (30s), `--health-probe-bind-address`, `--metrics-bind-address`, `--leader-elect` and `--leader-election-namespace`. Helper images are required at startup. The health endpoints report manager health; Trino `/v1/info` probes and the initialization process are declared by the product through typed framework process fields.

## Standard CRD controls

See `config/samples/trino_v1alpha1_trinocluster.yaml` for a complete coordinator/worker deployment. The standard shape is:

- `spec.image`: structured repo/productVersion/kubedoopVersion, or `custom` for an explicit reference/digest; independent pull policy and pull Secret.
- `spec.clusterConfig`: common authentication/Vector references, product platform settings, plus independent `stopped` and `reconciliationPaused`.
- `coordinators` / `workers`: optional role replicas, role-only PDB configuration, common/product `config`, four overrides, and named `roleGroups`.

Common configuration controls CPU, memory, affinity, termination grace and structured logging. Product configuration defaults to HTTP port 8080. User workload configuration folds product defaults, role and group layers. PDB computation uses the complete declared replica inventory, including groups whose resources are waiting or invalid. Stop retains those declarations while scaling authenticated workloads to zero; pause prevents business reads and resource mutations until resumed.

The four override channels are explicit. For example, within `workers.roleGroups.default`:

```yaml
configOverrides:
  config.properties:
    properties:
      set:
        query.max-memory-per-node: 256MB
envOverrides:
  EXAMPLE_ENV: enabled
cliOverrides:
  - --etc-dir=/etc/trino
  - run
podOverrides:
  metadata:
    annotations:
      example.kubedoop.dev/intent: explicit
```

File actions are `properties` (set/remove/replace), `lines`, `text`, or `remove`; the framework does not infer syntax from the file extension. CLI input replaces the full argument list, so the launcher arguments must remain present when intended. Role and group Pod patches are applied after file/env/CLI channels. A role Pod patch can therefore override a group env/CLI entry. Known final relationship conflicts block that group's apply; unknown relationships remain visibly unknown and do not establish runtime correctness.

The sample carries `restarter.kubedoop.dev/enable: "true"`. Product file ConfigMap changes need the platform's commons-operator restarter to reach existing processes; initial opt-in also causes one rollout. Env/CLI/Pod changes alter the Pod template directly. The SDK does not synthesize restarter stamps. Install the restarter when exercising file-only delivery. Central Vector destination changes are resolved into the Pod template and roll the collector through that template change.

## Native Trino logging and process identity

Trino 476 uses Airlift/JUL logging. This product generates `log.properties` and native `log.enable-console`, `log.path`, and JSON file logging properties; it does not emit Logback or Log4j configuration. `ROOT` maps to the empty native property key. Supported levels are TRACE, DEBUG, INFO, WARN, ERROR and OFF. FATAL is rejected because it has no faithful native mapping here.

One active sink, or two active sinks with the same threshold, is supported. Each logger threshold is clamped by the active sink threshold. Different active console/file thresholds are explicitly rejected. The defaults are console OFF, file TRACE and ROOT INFO. Quote `"OFF"` in YAML to avoid YAML 1.1 boolean conversion.

Vector is assembled only when `config.logging.enableVectorAgent` is true and the product declares a real file output. File OFF withdraws the native file output; console logging alone is not a file source. The sample also supplies a `vector` resources patch. If disabling file collection in a descendant group, remove that inherited container patch explicitly:

```yaml
config:
  logging:
    enableVectorAgent: false
podOverrides:
  spec:
    containers:
      - name: vector
        $patch: delete
```

The product writes `node.id=${ENV:TRINO_NODE_ID}` and supplies the Pod UID via the downward API. A container restart in the same Pod keeps that identity; a replacement Pod has a new identity. Configuration materialization leaves the native environment expression literal for Trino to resolve. The product declares initialization, probes and ordered workload coordination. Initialization checks this Pod's required materialized files and data/log directory writability on each execution; it does not use a historical marker to skip initialization. Startup/readiness probes require `/v1/info` to report `starting=false`; liveness checks the native info endpoint. Workers have shutdown priority 0 and coordinators 100.

Worker management writes are disabled by default. Set `config.shutdownUser` or `config.shutdownCredentialsSecret` explicitly to enable preStop; the Secret supplies read-only `username` and `password` files. The hook constructs Basic Authorization at runtime when credentials are supplied, identifies the current JVM by PID and birth identity, sends native `SHUTTING_DOWN`, and waits for that JVM to exit. Coordinators do not call this worker protocol. The hook currently uses loopback HTTP: authentication must match that management endpoint, and PASSWORD/TLS configuration does not implicitly make coordinator HTTPS available on a worker or enable insecure HTTP authentication.

The hook budget is `config.gracefulShutdownTimeout` minus two seconds; enabling it with a configured budget below ten seconds is rejected. Final Pod overrides can change the actual kubelet budget but do not recompute hook arguments, so such a change reports an unknown lifecycle relationship. Request acceptance, original query completion, main JVM exit and hook exit are separate observations. Finite budgets can still end in SIGKILL. Cross-role ordering covers framework stop/retirement; CR deletion uses Kubernetes GC and does not preserve that guarantee. See [lifecycle coordination](../../docs/architecture.md#framework-lifecycle) for recovery and progress rules.

## External catalogs and scope

Set role/group `config.catalogConfigMapName` to consume a same-namespace ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: trino-catalogs
data:
  catalogs.json: |
    {"tpch":{"connector.name":"tpch"}}
```

A resolved source replaces that group's base catalog map. The framework records the observed source UID/resourceVersion; the product resolver returns catalog facts and diagnostics without copying catalog contents into diagnostic messages. Periodic refresh makes external changes observable; file delivery still requires the restarter. Empty reference means no external lookup and uses the base TPCH catalog.

This reference defaults to ephemeral data directories. Set role/group `config.resources.storage` to
`{type: persistent, storageClassName: <explicit-Retain-class>, capacity: 64Gi}` to back
the declared `/var/trino/data` directory with a retained RWO/Filesystem claim. Role groups
can override capacity while inheriting type/class. Existing storage changes require an
explicit migration process; changing this field does not migrate data. `test/runtime/storage-controller` is a separate marker-only validation executable for the framework's explicitly retained slot; the production Trino executable does not import it. Explicit cross-CR adoption, migration and destruction belong to the independently deployed SDK data executor. They do not happen through storage configuration changes and do not imply Trino database recovery.


## Platform domains

`clusterConfig.vectorAgentConfigMap` references a same-namespace ConfigMap with
`data.ADDRESS=host:port`, pointing to a Vector source. It is an address, not arbitrary Vector YAML or a URL. If no destination is specified, Vector emits stdout JSON. Only enabled collectors with actual file outputs consume
that dependency. Missing sources are Pending; invalid addresses are Invalid and retain the preceding valid workload. ADDRESS changes update the collector template; native file
configuration delivery otherwise continues to use the platform restarter. Collection configuration does not prove remote delivery; Vector buffering/retry applies and its data directory is ephemeral.

`clusterConfig.authentication` accepts one static AuthenticationClass for PASSWORD
authentication. The class is cluster-scoped; its credentials Secret is in the Trino CR namespace and must contain nonempty `password.db` in native Trino hash format. Configure exactly
one of `tlsSecret` (keys `tls.crt`, `tls.key`) and `tlsSecretClass` (platform PEM
output), plus `internalSecret` (key `shared-secret`). HTTPS uses port 8443; the
product refuses other authentication providers until a concrete adapter exists.
Native Secret files are mounted read-only with mode 0440. An initialization process prepares a mode-0600 PEM in an ephemeral directory from read-only mounts; the internal shared identity reaches all roles through SecretKeyRef and native `${ENV:TRINO_INTERNAL_SHARED_SECRET}`. PASSWORD requires both TLS and the internal identity. TLS may also be enabled independently. Neither insecure HTTP authentication nor trust of forwarded headers is enabled implicitly. With a Listener, the TLS scope includes `listener-volume=listener` to cover its published address.

Secret bytes stay in volumes or SecretKeyRef, and native Secret revisions refresh consuming Pods. Missing required references retain the previous valid resources while waiting. AuthenticationClass resolution alone does not prove user authentication or grant worker shutdown permission. See the [authentication boundary](../../docs/security.md#framework-authentication) and the pinned Trino 476 [TLS](https://github.com/trinodb/trino/blob/476/docs/src/main/sphinx/security/tls.md), [password file](https://github.com/trinodb/trino/blob/476/docs/src/main/sphinx/security/password-file.md) and [internal communication](https://github.com/trinodb/trino/blob/476/docs/src/main/sphinx/security/internal-communication.md) contracts.

`clusterConfig.listenerClass` declares the coordinator Listener CSI producer.
Discovery waits for the current Pod/PVC/PV/Listener result, then publishes its
observed address and named HTTP/HTTPS port, preferring HTTPS when enabled. With no Listener it uses the declared Service DNS. Pending observations retain the previous discovery output; a zero-replica group has no newly observed live address. Install the actual platform Secret and
Listener operators when selecting their CSI sources; the Trino installer grants
reference reads but does not install those platform components.

For an actual Hive catalog, set role/group `config.hive.metastoreURI` and
`config.hive.s3`. S3 has explicit `disabled`, `inline`, `reference` variants;
changing the variant clears the inherited branch. Within the same variant, omitted fields inherit from role to group, so a group can change pathStyle while retaining its endpoint and credentials. An unspecified region becomes `us-east-1`; an unspecified port becomes 80 for HTTP or 443 for HTTPS, and host must be a DNS name or IP. Inline credentials select a
native Secret or SecretClass; reference uses an actual namespaced platform
S3Connection. The Trino launcher reads mounted ACCESS_KEY/SECRET_KEY files into
its process environment and native catalog expressions reference those variables.
No credential bytes are emitted into generated ConfigMaps. Empty or unreadable credential files fail startup; no undeclared credential chain is used. The fixed launcher remains in `Main.Command`, so inherited CLI overrides still replace only `Main.Args`, including an explicit empty argument list, and final Pod arguments retain their higher priority.

The fixed `hive` catalog uses Trino 476's `fs.native-s3.enabled`, `s3.endpoint`, `s3.region`, `s3.path-style-access`, and `${ENV:TRINO_S3_ACCESS_KEY}` / `${ENV:TRINO_S3_SECRET_KEY}` credential values. A catalog ConfigMap that also defines `hive` conflicts with this typed input; final file overrides keep their existing priority. Only system-CA-verified S3 HTTPS is supported; verification bypass or an unconsumed custom CA is rejected. Connection endpoint/region/pathStyle changes also update `TRINO_S3_CONNECTION` in the Pod template so refreshed references reach a new process without editing the CR. This does not change Trino's default policy for writes to non-managed Hive tables. See the pinned [Trino 476 S3 filesystem configuration](https://github.com/trinodb/trino/blob/476/docs/src/main/sphinx/object-storage/file-system-s3.md).

Shared contracts are maintained in [framework design](../../docs/architecture.md#framework-design), [platform inputs and observation](../../docs/architecture.md#framework-platform), [S3](../../docs/architecture.md#framework-s3), [logging](../../docs/architecture.md#framework-logging) and [data operations](../../docs/architecture.md#framework-data-operations). Product runtime and fixture validation use the [acceptance harness](../../hack/framework-e2e/README.md); framework marker/logging fixtures do not establish Trino database recovery or product query results.

To package the independently authorized data executor, run from the SDK root:

```sh
make dataops-image DATAOPS_IMAGE=operator-go-dataops:dev
kustomize build config/framework-data-executor > /tmp/data-executor.yaml
```

Load the image into the intended cluster and review/apply that explicit deployment
when data operations are needed. Its worker image contains Python 3 and is pinned
in the overlay. Creating an approved immutable DataOperation is a separate action;
ordinary product reconciliation never requests adoption, migration or destruction. The operation binds exact asset/source/target UIDs and an explicit non-root worker identity; RBAC grants authorization and the approval digest binds the reviewed input. Before execution, relevant StatefulSets must be retired, all actual data consumers absent, and any existing source CR paused. Adoption and migration also require the target CR to be paused. A Stopped condition or pause alone is insufficient. Migration retains the source copy; destruction is an independently approved action. See [data authorization](../../docs/security.md#framework-data-authorization) and [retained storage](../../docs/architecture.md#framework-storage) before preparing an operation.
