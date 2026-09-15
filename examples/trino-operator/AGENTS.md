# Trino reference operator

Parent: [../AGENTS.md](../AGENTS.md). Design source: [../../docs/architecture.md#framework-design](../../docs/architecture.md#framework-design).

This nested Go module now uses the formal public `pkg/framework`, `pkg/framework/input`, `pkg/framework/inputgen`, and `pkg/framework/operator` packages. It has no legacy GenericReconciler/RoleProvider/extension/webhook path and must not import local discussion prototypes, SDK internal packages, or old execution packages.

## Current files and responsibility

- `internal/product/definition.go`: exported TrinoConfig/TrinoClusterConfig/TrinoFacts, Definition, pure validation/generation and discovery output.
- `internal/product/logging.go`: Trino 476 Airlift/JUL adapter; ROOT empty key, sink/logger clamp, explicit unsupported-level/unequal-sink errors.
- `internal/product/facts.go` and `data.go`: compose bounded catalog, authentication and S3 resolution; clone facts and preserve sanitized diagnostics.
- `internal/product/final.go` and `helpers.go`: product relationships checked under known final execution/file premises.
- `cmd/generate`: derives roles from Definition and emits API, CRD and registration. `-check` checks all three artifacts exactly.
- `api/v1alpha1/zz_generated.input.go`: presence-preserving generated API, DeepCopy, early Operation, raw Project and Binding.
- `api/v1alpha1/registration/zz_generated.register.go`: fixed C/S companion calling public operator.Register.
- `cmd/main.go`: actual manager startup, explicit helper images, base facts/resolver and generated registration.
- `config/default`, `manager`, `rbac`, `crd`, `samples`: complete minimal installer and actual workload sample. No webhook/cert-manager scaffolding remains.
- `test/runtime/storage-controller`: marker-only framework validation tool, separate from the Trino production executable; no Trino query claim.
- `test/runtime/logging-controller`: Python native logging/central Vector acceptance fixture using the generated API/registration; separate namespace and executable from production Trino.

## Current runtime boundaries

Product defaults target organization Trino 476, launcher `/kubedoop/trino-server/bin/launcher`, UID/GID 1001. Native `node.id=${ENV:TRINO_NODE_ID}` consumes Pod UID through downward API. Config lives at `/etc/trino`; data defaults to ephemeral; real native JSON file output is under `/kubedoop/log/trino`. The framework assembles the materializer and selected Vector collector. It owns `enableVectorAgent`; LogOutput has no Collect field.

Only one coordinator group with one replica is supported. Workers share the coordinator endpoint from complete desired topology. Default TPCH catalogs can be replaced by per-group same-namespace `catalogs.json` ConfigMaps through FactsReader. Source readiness is distinct from query readiness. PDB is role-scoped and stop preserves desired replica inventory.

All four overrides use the formal input actions, with final Pod patches taking precedence. Unknown final relationships are not proof of compatibility. Product file ConfigMap changes rely on the separately installed platform restarter; the sample opts in with a workload-propagated CR label. Central Vector destination changes update the Pod template directly. Disabling Vector in a descendant sample group also requires deleting its inherited resources-only vector Pod patch.

## Verification and builds

Use `make generate`, `make verify-generate`, `make test`, `make lint`, `make build`, and `make build-installer`. `make test` uses explicit/root envtest assets and verifies the published sample through the real generated CRD. Unit tests alone do not prove a running Trino workload; U05 owns deployment evidence.

`make docker-build` cross-compiles a static Linux manager into `bin/image` and builds the scratch Dockerfile from that bounded context. `TARGETARCH` chooses architecture; root `make materializer-image` builds the co-released helper. `make build-installer IMG=...` creates a temporary Kustomize overlay, writes `dist/install.yaml`, and leaves source manifests unchanged. Kustomize is supplied through PATH or `KUSTOMIZE`.

Keep generated artifacts synchronized, and update this file when actual behavior changes. Do not add compatibility aliases, old startup paths, duplicate handwritten CRD projections, or product-specific framework bindings.

The product marks its data directory with `Directory.Data`. Standard `resources.storage`
is folded and consumed by the framework; the product default remains ephemeral.
The marker fixture also declares a symbolic Data directory and supplies storage defaults.
Its runtime harness overrides storage through the generated CR input.

`internal/product/lifecycle.go` declares ordered idempotent initialization and native startup,
readiness and liveness probes. Optional shutdownUser/shutdownCredentialsSecret enables the worker
SHUTTING_DOWN protocol with exact JVM identity and bounded waiting. Defaults grant no management
write. Workers stop before coordinator through declared coordination priority. Native probes moved
from the sample Pod patches into product intent. See docs/architecture.md#framework-lifecycle for limits.

`authentication.go` consumes one Static AuthenticationClass as coordinator PASSWORD authentication.
TLS from tlsSecret/tlsSecretClass is assembled on the coordinator; internalSecret SecretKeyRef is consumed by all roles. Secrets are
references in facts; TLS PEM assembly runs in a separate initializer. With ListenerClass,
AutoTLS scopes include the Listener volume so the published endpoint is covered by certificate SANs. Unsupported provider
combinations fail visibly. `facts.go` composes catalog, authentication and S3 resolution.
The sample API test starts generated registration against envtest and inspects the actual
StatefulSet probes and ordered initializers; kubelet/process success remains runtime evidence.
