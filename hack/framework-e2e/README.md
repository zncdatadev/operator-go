# Formal framework runtime acceptance

This harness builds the actual Trino reference operator and the SDK materializer, installs the example's generated CRD and deployment RBAC, and exercises them in a new disposable kind cluster. Historical discussion prototypes are local records, not runtime dependencies.

Run from the repository root on an arm64 Docker host with Go 1.25, Python 3, Docker, kind, kubectl and kustomize on PATH. Allow about 8 GiB of Docker memory. The workload images and Kubernetes node image are fixed by digest in `run.py`; amd64 is not covered by this fixture. Supply a clean commons-operator checkout; this acceptance baseline uses `6e65371`. The harness records the supplied revision and builds its unmodified restarter.

```sh
make lint
make test GOTESTFLAGS='-p=2'
make verify-generate
make -C examples/trino-operator test lint build
python3 -B -m unittest discover -s hack/framework-e2e -p 'test_*.py'
python3 -B hack/framework-e2e/run.py \
  --commons ../commons-operator \
  --output .local/engineering-notes/runs/framework-acceptance-01
```

Use a new empty output directory for every run. Store local reports under the Git-ignored `.local/engineering-notes/runs/`; never commit generated runtime evidence. The runner cross-compiles the operator and materializer to Linux arm64, builds unique local images, compiles the host restarter and retained-volume fixture, and loads the images into its own kind cluster. It uses an explicit private kubeconfig for every cluster operation. It compares the default kubeconfig hashes and existing kind node identities/states before and after; it reaps its own restarter and removes its own cluster and temporary image tags on success or failure.

Acceptance includes:

- Generated registration and the actual executable running under delivered RBAC.
- Native Trino node IDs equal to the current Pod UIDs, and TPCH query results requiring the worker.
- File, env, CLI and Pod updates; external catalog refresh without editing the CR; commons restarter stamps and delivered file bytes.
- Native JSON file events appearing in Vector stdout, pause/resume, stopped workloads, worker retirement, operator restart and re-addition.
- A stable final observation across a full facts-refresh interval, including CR/resource versions and restart counts.
- A separate Retain marker fixture using public registration and standard CR storage input
  (role class/type/32Mi, role-group capacity override): same live PVC/PV identities and marker bytes after retirement, operator restart and same-CR re-addition.

`verification.json` must say both `passed: true` and `cleanup: true`. The evidence directory contains source and binary hashes, exact images, inputs, resource snapshots, SQL/log observations, all command receipts and cleanup records. The directory's `kubeconfig` is an ephemeral cluster credential: keep raw evidence local. CI artifact selection must explicitly omit kubeconfig and executables. A successful storage marker fixture is not Trino business-data recovery, and SQL probes after convergence do not promise uninterrupted queries during rollout.

## Delivery versions

Build the SDK, generator, generated API/registration, operator and materializer from the same reviewed source revision. The checked-in generated API and registration each declare input contract `1`; registration and `generate -check` reject a mismatched contract. Re-run generation and commit all generated files when C/S or generator output changes.

The materialization plan is `v1` and its runtime properties codec is `properties-v1`; the helper rejects unknown plan/codec versions. These are internal delivery contracts, not product-author APIs. The materializer image labels record those versions and the build's source revision. A matching label alone does not replace the execution tests.

```sh
make materializer-image MATERIALIZER_IMG=registry.example/team/materializer:<revision>
make -C examples/trino-operator docker-build IMG=registry.example/team/trino-operator:<revision>
make -C examples/trino-operator build-installer IMG=registry.example/team/trino-operator:<revision>
```

Set the installer materializer and Vector references to the tested immutable image digests before distributing it. Image builds above do not publish anything. The local development tags in the sample deployment are build inputs, not evidence that a public registry release exists. Remote registry publication and release tagging are separate release operations.


## E01–E05 integrated domain acceptance

The same runner now includes the implemented domain expansion. It builds the
unchanged local secret-operator and listener-operator manager/CSI executables,
records their source hashes, packages a unique platform image and pins the native
CSI sidecars to observed immutable digests. Those components run only in the
runner's disposable kind cluster. Platform source repositories must have clean
build inputs; `.worktree/` is excluded from unrelated untracked-file checks.

Additional stages verify actual typed initialization and active-query worker
shutdown; PASSWORD SQL over verified TLS and native Secret refresh; AutoTLS CSI
mounts and observed Listener discovery; Hive/MinIO S3 writes/reads through inline
and reference connection branches; approved data adoption/migration/destruction
and provisioner backend reclamation; and native Python log delivery to a Vector
receiver with destination refresh. Product/operator code uses formal registration.
The separate lightweight logging/storage processes are explicit framework
fixtures and are not substituted for the Trino authentication, S3 or lifecycle
processes.

Data experiments create their own namespace and new marker files. No preexisting
business volume is an input. Failed attempts keep diagnostics and are marked
failed; a later complete run has a separate output directory. Raw evidence may
contain temporary credentials and private kubeconfig, so only inspected receipts
may be published or checked in. Do not include executables or TLS private keys.
