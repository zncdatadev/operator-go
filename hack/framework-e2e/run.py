#!/usr/bin/env python3
"""Build and verify formal delivery artifacts in a disposable, isolated kind cluster."""
import argparse
import hashlib
import json
import os
import pathlib
import signal
import subprocess
import sys
import tarfile
import tempfile
import time

from runtime import RuntimeVerifier, require
from lifecycle import LifecycleVerifier
from platform_csi import PlatformInstaller, PlatformVerifier
from authentication import AuthenticationVerifier
from s3 import S3Verifier

ROOT = pathlib.Path(__file__).resolve().parents[2]
EXAMPLE = ROOT / "examples/trino-operator"
TRINO = "quay.io/zncdatadev/trino@sha256:6c7002a7e6d4f7a738f3e3066dabf06f433c32e57900d5697a7544187f40875f"
VECTOR = "quay.io/zncdatadev/vector@sha256:3b9a99d98905443924bee204bd76c2818ad2da7056388fd524b0ea000eb55682"
NODE = "kindest/node@sha256:4613778f3cfcd10e615029370f5786704559103cf27bef934597ba562b269661"
HIVE = "quay.io/zncdatadev/hive@sha256:b1de1210b9220f79c34e0b9e584b1b6b804ad8b9d9ee67f8e0050661ee0ebaa6"
MINIO = "minio/minio@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e"
MINIO_MC = "minio/mc@sha256:a7fe349ef4bd8521fb8497f55c6042871b2ae640607cf99d9bede5e9bdf11727"
RUNTIME_IMAGES = (TRINO, VECTOR, HIVE, MINIO, MINIO_MC)
OPERATOR_NS = "trino-operator-system"
DEPLOYMENT = "trino-operator-controller-manager"


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def resource_list(stream):
    """kubectl prints one JSON document per input resource, or a List."""
    decoder, items = json.JSONDecoder(), []
    remaining = stream.lstrip()
    while remaining:
        resource, end = decoder.raw_decode(remaining)
        require(isinstance(resource, dict), "kubectl resource must be an object")
        items.extend(resource["items"] if resource.get("kind") == "List" else [resource])
        remaining = remaining[end:].lstrip()
    require(items, "installer produced no resources")
    return {"apiVersion": "v1", "kind": "List", "items": items}


def context_snapshot():
    paths = (os.environ.get("KUBECONFIG") or str(pathlib.Path.home() / ".kube/config")).split(os.pathsep)
    configs = [digest(pathlib.Path(p)) if pathlib.Path(p).is_file() else None for p in paths]
    names = subprocess.check_output(["docker", "ps", "-a", "--filter", "label=io.x-k8s.kind.cluster",
                                     "--format", "{{.Names}}"], text=True, timeout=15).splitlines()
    nodes = []
    for name in sorted(names):
        value = json.loads(subprocess.check_output(["docker", "inspect", name], text=True, timeout=15))[0]
        nodes.append({"name": name, "id": value["Id"], "state": value["State"]["Status"]})
    return {"kubeconfig_hashes": configs, "existing_kind_nodes": nodes}


class Run:
    def __init__(self, args):
        self.args = args
        self.output = pathlib.Path(args.output).resolve()
        self.output.mkdir(parents=True, exist_ok=True)
        require(not any(self.output.iterdir()), "Use a new empty output directory")
        os.chmod(self.output, 0o700)
        (self.output / "commands").mkdir()
        self.cluster = "framework-u05-" + str(os.getpid()) + "-" + str(int(time.time()))
        self.kubeconfig = self.output / "kubeconfig"
        self.operator_image = "operator-go/trino-operator:" + self.cluster
        self.helper_image = "operator-go/materializer:" + self.cluster
        self.created = False
        self.images, self.processes = [], []
        self.command_index = 0
        self.last_command = None
        self.report = {"passed": False, "cluster": self.cluster, "phases": [], "cleanup": False,
                       "scope": "formal SDK, generated registration, deployed Trino reference operator and Retain fixture"}
        self.platform = PlatformInstaller(self)
        self.before = context_snapshot()
        self.write("context-before.json", self.before)
        self.persist()

    def write(self, name, data):
        path = self.output / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(data, indent=2) + "\n")

    def persist(self):
        self.write("verification.json", self.report)

    def command(self, argv, timeout=60, input=None, check=True, cwd=ROOT):
        self.command_index += 1
        key = f"{self.command_index:05d}"
        receipt = {"argv": list(map(str, argv)), "cwd": str(cwd), "started_at": time.time()}
        data = input.encode() if isinstance(input, str) else input
        if data is not None:
            receipt["stdin_sha256"] = hashlib.sha256(data).hexdigest()
        started = time.monotonic()
        try:
            process = subprocess.Popen(receipt["argv"], cwd=cwd, stdin=subprocess.PIPE if data is not None else None,
                                       stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
            try:
                stdout, stderr = process.communicate(input=data, timeout=timeout)
                receipt["returncode"] = process.returncode
            except subprocess.TimeoutExpired:
                os.killpg(process.pid, signal.SIGTERM)
                try:
                    stdout, stderr = process.communicate(timeout=20)
                except subprocess.TimeoutExpired:
                    os.killpg(process.pid, signal.SIGKILL)
                    stdout, stderr = process.communicate(timeout=5)
                receipt.update(returncode=124, timed_out=True)
            receipt.update(stdout=stdout.decode(errors="replace"), stderr=stderr.decode(errors="replace"))
        except OSError as error:
            receipt.update(returncode=125, stdout="", stderr=repr(error))
        receipt["elapsed_seconds"] = time.monotonic() - started
        self.write("commands/" + key + ".json", receipt)
        self.last_command = receipt
        if check:
            require(receipt["returncode"] == 0, "command " + key + " failed: " + receipt["stderr"][-3000:])
        return receipt["stdout"]

    def kube(self, *args, **kwargs):
        return self.command(["kubectl", "--kubeconfig", self.kubeconfig, "--request-timeout=20s", *args], **kwargs)

    def apply(self, obj):
        self.kube("apply", "-f", "-", input=json.dumps(obj))

    def wait_crd_established(self, name):
        """Observe establishment without kubectl wait's null-conditions accessor race."""
        started = time.monotonic()
        deadline = started + 60
        report = {"name": name, "budget_seconds": 60, "established": False, "attempts": []}
        evidence = "crd-establishment/" + name + ".json"
        while time.monotonic() < deadline:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                break
            request_timeout = min(10, remaining)
            raw = self.kube("get", "crd", name, "--ignore-not-found", "-o", "json",
                            f"--request-timeout={request_timeout:.9f}s",
                            timeout=request_timeout, check=False)
            attempt = {"command": f"{self.command_index:05d}",
                       "returncode": self.last_command["returncode"], "state": "Pending"}
            if self.last_command["returncode"] != 0:
                attempt.update(state="ReadError", error=self.last_command.get("stderr", ""))
            elif not raw.strip():
                attempt["reason"] = "CRD not found"
            else:
                try:
                    value = json.loads(raw)
                    status = value.get("status") or {}
                    conditions = status.get("conditions")
                    if conditions is None:
                        conditions = []
                    if not isinstance(conditions, list):
                        raise ValueError("CRD conditions must be a list or null")
                    attempt["conditions"] = conditions
                    if any(isinstance(c, dict) and c.get("type") == "Established" and
                           c.get("status") == "True" for c in conditions):
                        attempt["state"] = "Established"
                except (ValueError, AttributeError) as error:
                    attempt.update(state="ReadError", error=str(error))
            report["attempts"].append(attempt)
            report["elapsed_seconds"] = time.monotonic() - started
            report["established"] = attempt["state"] == "Established" and time.monotonic() <= deadline
            self.write(evidence, report)
            if report["established"]:
                return
            time.sleep(min(1, max(0, deadline - time.monotonic())))
        report["elapsed_seconds"] = time.monotonic() - started
        self.write(evidence, report)
        require(False, "CRD " + name + " was not Established within 60 seconds; evidence: " + evidence +
                "; last observation: " + json.dumps(report["attempts"][-1] if report["attempts"] else {}))

    def phase(self, name):
        self.report["phases"].append({"name": name, "time": time.time()})
        self.persist()
        print("U05:", name, flush=True)

    def restart_operator(self):
        self.kube("-n", OPERATOR_NS, "rollout", "restart", "deployment/" + DEPLOYMENT)
        self.kube("-n", OPERATOR_NS, "rollout", "status", "deployment/" + DEPLOYMENT, "--timeout=120s", timeout=150)

    def sources(self):
        files = []
        for folder in ("pkg/framework", "internal/framework", "cmd/materialize", "cmd/dataops", "config/framework-data", "config/framework-data-executor", "examples/trino-operator", "hack/framework-e2e"):
            files.extend(path for path in (ROOT / folder).rglob("*") if path.is_file() and
                         path.suffix in (".go", ".py", ".yaml", ".mod", ".sum") and
                         not any(part in ("bin", "__pycache__") for part in path.relative_to(ROOT).parts))
        files += [ROOT / "Makefile", ROOT / "cmd/materialize/Dockerfile", ROOT / "Dockerfile.dataops", EXAMPLE / "Makefile", EXAMPLE / "Dockerfile"]
        self.write("sources.json", {str(path.relative_to(ROOT)): digest(path) for path in sorted(files)})
        self.write("revision.json", {"commit": self.command(["git", "rev-parse", "HEAD"]).strip(),
                                     "status": self.command(["git", "status", "--short"])})

    def build(self):
        self.phase("build-formal-artifacts")
        self.sources()
        architecture = self.command(["docker", "info", "--format", "{{.Architecture}}"]).strip()
        require(architecture in ("aarch64", "arm64"), "This pinned runtime fixture is verified for arm64")
        self.images.append(self.helper_image)
        self.command(["make", "materializer-image", "MATERIALIZER_ARCH=arm64", "MATERIALIZER_IMG=" + self.helper_image], timeout=600)
        self.images.append(self.operator_image)
        self.command(["make", "docker-build", "IMG=" + self.operator_image], cwd=EXAMPLE, timeout=600)
        self.command(["go", "build", "-mod=readonly", "-trimpath", "-buildvcs=false", "-o", self.output / "storage-controller",
                      "./test/runtime/storage-controller"], cwd=EXAMPLE, timeout=300)
        self.command(["go", "build", "-mod=readonly", "-trimpath", "-buildvcs=false", "-o", self.output / "logging-controller",
                      "./test/runtime/logging-controller"], cwd=EXAMPLE, timeout=300)
        self.command(["go", "build", "-mod=readonly", "-trimpath", "-buildvcs=false", "-o", self.output / "data-controller",
                      "./cmd/dataops"], timeout=300)
        commons = pathlib.Path(self.args.commons).expanduser().resolve()
        tracked = self.command(["git", "diff", "HEAD", "--name-only"], cwd=commons).strip()
        untracked = self.command(["git", "ls-files", "--others", "--exclude-standard", "--", ".", ":(exclude).worktree/**"], cwd=commons).strip()
        require(not tracked and not untracked, "commons-operator tracked/build inputs must be clean")
        self.write("commons-revision.json", {"commit": self.command(["git", "rev-parse", "HEAD"], cwd=commons).strip()})
        self.command(["go", "build", "-mod=readonly", "-trimpath", "-buildvcs=false", "-o", self.output / "commons-operator", "./cmd"],
                     cwd=commons, timeout=300)
        self.write("binaries.json", {name: digest(self.output / name) for name in ("storage-controller", "logging-controller", "data-controller", "commons-operator")})
        for image in RUNTIME_IMAGES:
            self.command(["docker", "image", "inspect", image], check=False)
            if self.last_command["returncode"] != 0:
                self.command(["docker", "pull", image], timeout=600)
        self.write("images.json", json.loads(self.command(["docker", "image", "inspect", self.operator_image, self.helper_image, *RUNTIME_IMAGES])))
        (self.output / "fixture-image.json").write_text(self.command(["docker", "image", "inspect", TRINO]))
        self.platform.build()

    def load_images(self, images):
        # Docker may cache only arm64 children of a multi-platform index. Export
        # and import that explicit platform rather than requesting absent ones.
        node = self.cluster + "-control-plane"
        with tempfile.TemporaryDirectory(prefix=self.cluster + "-images-") as temporary:
            archive = pathlib.Path(temporary) / "arm64.tar"
            remote = "/var/lib/containerd/" + pathlib.Path(temporary).name + ".tar"
            try:
                self.command(["docker", "image", "save", "--platform=linux/arm64", "--output", archive, *images], timeout=600)
                identities = getattr(self, "loaded_image_identities", {})
                with tarfile.open(archive) as saved:
                    entries = json.load(saved.extractfile("manifest.json"))
                    configs = {tag: "sha256:" + hashlib.sha256(saved.extractfile(entry["Config"]).read()).hexdigest()
                               for entry in entries for tag in entry["RepoTags"]}
                for image in images:
                    require(image in configs, "Image missing from selected platform archive: " + image)
                    host = json.loads(self.command(["docker", "image", "inspect", image]))[0]
                    require(host["Architecture"] == "arm64", "Unexpected host image architecture: " + image)
                    identities[image] = {"host_id": host["Id"], "host_repo_digests": host.get("RepoDigests", []),
                                         "archive_config_id": configs[image]}
                self.loaded_image_identities = identities
                self.write("loaded-image-identities.json", identities)
                self.command(["docker", "cp", archive, node + ":" + remote], timeout=300)
                self.command(["docker", "exec", node, "test", "-s", remote])
                self.command(["docker", "exec", node, "ctr", "--namespace=k8s.io", "images", "import",
                              "--platform=linux/arm64", remote], timeout=600)
                for image in images:
                    observed = json.loads(self.command(["docker", "exec", node, "crictl", "inspecti", image]))
                    require(observed["status"]["id"] == identities[image]["archive_config_id"],
                            "Node image differs from exported host image: " + image)
                    identities[image]["node_config_id"] = observed["status"]["id"]
                    self.write("loaded-image-identities.json", identities)
            finally:
                self.command(["docker", "exec", node, "rm", "-f", remote], check=False)

    def ensure_image_digest(self, image, transport):
        node = self.cluster + "-control-plane"
        self.command(["docker", "exec", node, "crictl", "inspecti", image], check=False)
        if self.last_command["returncode"] != 0:
            def qualified(reference):
                if "/" not in reference:
                    return "docker.io/library/" + reference
                registry = reference.split("/", 1)[0]
                return reference if "." in registry or ":" in registry or registry == "localhost" else "docker.io/" + reference
            self.command(["docker", "exec", node, "ctr", "--namespace=k8s.io", "images", "tag",
                          qualified(transport), qualified(image)])
        observed = json.loads(self.command(["docker", "exec", node, "crictl", "inspecti", image]))
        identity = self.loaded_image_identities[transport]
        host = json.loads(self.command(["docker", "image", "inspect", image]))[0]
        require(host["Id"] == identity["host_id"], "Transport tag no longer represents selected host image: " + image)
        require(observed["status"]["id"] == identity["archive_config_id"], "Pinned node image identity differs: " + image)
        self.loaded_image_identities[image] = {**identity, "selected_digest": image, "node_config_id": observed["status"]["id"]}
        self.write("loaded-image-identities.json", self.loaded_image_identities)

    def start_cluster(self):
        self.phase("create-isolated-kind")
        self.created = True
        self.command(["kind", "create", "cluster", "--name", self.cluster, "--image", NODE,
                      "--kubeconfig", self.kubeconfig, "--wait", "90s"], timeout=240)
        transports = []
        for image in RUNTIME_IMAGES:
            transport = image.split("@")[0] + ":" + self.cluster
            self.images.append(transport)
            self.command(["docker", "tag", image, transport])
            transports.append(transport)
        self.load_images([self.helper_image, self.operator_image, *transports])
        for image, transport in zip(RUNTIME_IMAGES, transports):
            self.ensure_image_digest(image, transport)
        self.kube("create", "namespace", "trino-u05")
        self.kube("create", "namespace", "storage-u05")
        self.write("kubernetes-version.json", json.loads(self.kube("version", "-o", "json")))
        self.platform.load()

    def deploy(self):
        self.phase("deploy-generated-api-rbac-operator")
        self.command(["make", "build-installer"], cwd=EXAMPLE, timeout=120)
        manifests = (EXAMPLE / "dist/install.yaml").read_text()
        resources = resource_list(self.kube("create", "--dry-run=client", "-f", "-", "-o", "json", input=manifests))
        items = resources.get("items", [resources])
        for obj in items:
            if obj["kind"] != "Deployment":
                continue
            container = obj["spec"]["template"]["spec"]["containers"][0]
            container["image"] = self.operator_image
            container["imagePullPolicy"] = "IfNotPresent"
            args = [arg for arg in container.get("args", []) if not arg.startswith(("--materializer-image=", "--vector-image=", "--namespace="))]
            container["args"] = args + ["--materializer-image=" + self.helper_image, "--vector-image=" + VECTOR, "--namespace=trino-u05"]
        self.write("install.json", resources)
        self.apply(resources)
        self.wait_crd_established("trinoclusters.trino.kubedoop.dev")
        self.kube("-n", OPERATOR_NS, "rollout", "status", "deployment/" + DEPLOYMENT, "--timeout=180s", timeout=210)
        stream = (self.output / "commons-operator.log").open("wb")
        command = [str(self.output / "commons-operator"), "--kubeconfig=" + str(self.kubeconfig),
                   "--metrics-bind-address=0", "--health-probe-bind-address=0", "--leader-elect=false"]
        process = subprocess.Popen(command, stdout=stream, stderr=subprocess.STDOUT, start_new_session=True)
        self.processes.append((process, stream))
        self.write("commons-process.json", {"pid": process.pid, "argv": command})
        sample = json.loads(self.kube("create", "--dry-run=client", "-f", EXAMPLE / "config/samples/trino_v1alpha1_trinocluster.yaml", "-o", "json"))
        sample["metadata"].update(name="demo-trino", namespace="trino-u05")
        sample["spec"]["image"] = {"custom": TRINO, "pullPolicy": "IfNotPresent"}
        self.write("input.json", sample)
        self.apply(sample)
        RuntimeVerifier(self, "trino-u05", "demo-trino").verify()
        require(process.poll() is None, "commons restarter exited during runtime acceptance")
        self.kube("-n", OPERATOR_NS, "logs", "deployment/" + DEPLOYMENT, "--tail=2000")

    def storage(self):
        self.phase("retained-volume-acceptance")
        self.kube("-n", "local-path-storage", "rollout", "status", "deployment/local-path-provisioner", "--timeout=90s", timeout=120)
        storage_class = self.cluster + "-retain"
        self.apply({"apiVersion": "storage.k8s.io/v1", "kind": "StorageClass", "metadata": {"name": storage_class},
                    "provisioner": "rancher.io/local-path", "reclaimPolicy": "Retain", "volumeBindingMode": "WaitForFirstConsumer"})
        self.command([sys.executable, "-B", ROOT / "hack/framework-e2e/verify-storage.py", "--kubeconfig", self.kubeconfig,
                      "--namespace", "storage-u05", "--output-dir", self.output / "storage", "--controller-binary", self.output / "storage-controller",
                      "--image", TRINO, "--node-name", self.cluster + "-control-plane", "--image-inventory", self.output / "fixture-image.json",
                      "--storage-class", storage_class, "--capacity", "64Mi", "--timeout", "900"], timeout=950)
        require(json.loads((self.output / "storage/verification.json").read_text())["passed"], "storage acceptance failed")

    def dataops(self):
        self.phase("explicit-data-identity-adoption-migration-destruction")
        self.command([sys.executable, "-B", ROOT / "hack/framework-e2e/verify-dataops.py",
                      "--kubeconfig", self.kubeconfig, "--output-dir", self.output / "dataops",
                      "--storage-controller-binary", self.output / "storage-controller",
                      "--data-controller-binary", self.output / "data-controller",
                      "--image", TRINO, "--storage-class", self.cluster + "-retain",
                      "--node-name", self.cluster + "-control-plane"], timeout=1500)
        report = json.loads((self.output / "dataops/verification.json").read_text())
        require(report["passed"] and report["cleanup"], "data operation acceptance or cleanup failed")

    def logging(self):
        self.phase("central-logging-discovery-and-delivery")
        self.command([sys.executable, "-B", ROOT / "hack/framework-e2e/verify-logging.py",
                      "--kubeconfig", self.kubeconfig, "--namespace", "logging-e03",
                      "--output-dir", self.output / "logging", "--controller-binary", self.output / "logging-controller",
                      "--image", TRINO, "--materializer-image", self.helper_image, "--vector-image", VECTOR,
                      "--timeout", "600"], timeout=780)
        logging_report = json.loads((self.output / "logging/verification.json").read_text())
        require(logging_report["passed"] and logging_report["cleanup"], "central logging acceptance or cleanup failed")

    def close(self):
        errors, stopped = [], []
        for process, stream in reversed(self.processes):
            observation = {"pid": process.pid, "signal": None, "reaped": False}
            try:
                if process.poll() is None:
                    try:
                        os.killpg(process.pid, signal.SIGTERM)
                        observation["signal"] = "TERM"
                    except ProcessLookupError:
                        pass
                    try:
                        process.wait(timeout=15)
                    except subprocess.TimeoutExpired:
                        os.killpg(process.pid, signal.SIGKILL)
                        observation["signal"] = "KILL"
                        process.wait(timeout=5)
                observation.update(returncode=process.wait(timeout=1), reaped=True)
            except Exception as error:
                errors.append("process cleanup: " + repr(error))
                observation["error"] = repr(error)
            finally:
                stream.close()
                stopped.append(observation)
        self.write("process-cleanup.json", stopped)
        if self.created:
            try:
                self.kube("get", "pods,statefulsets,services,configmaps,trinoclusters", "-A", "-o", "json", check=False)
                self.kube("get", "events", "-A", check=False)
                if not self.report["passed"]:
                    self.kube("-n", OPERATOR_NS, "logs", "deployment/" + DEPLOYMENT, "--tail=500", check=False)
                    for role in ("coordinators", "workers"):
                        for container in ("prepare-files", "trino", "vector"):
                            self.kube("-n", "trino-u05", "logs", "demo-trino-" + role + "-default-0", "-c", container, "--tail=200", check=False)
            except Exception as error:
                errors.append("capture final state: " + repr(error))
            try:
                self.command(["kind", "delete", "cluster", "--name", self.cluster, "--kubeconfig", self.kubeconfig], timeout=180)
            except Exception as error:
                errors.append("kind cleanup: " + repr(error))
        for image in reversed(self.images):
            try:
                self.command(["docker", "image", "rm", image], check=False)
                self.command(["docker", "image", "inspect", image], check=False)
                missing = self.last_command["returncode"] != 0 and "No such image:" in self.last_command["stderr"]
                require(missing, "temporary image cleanup unconfirmed: " + image)
            except Exception as error:
                errors.append("image cleanup: " + repr(error))
        try:
            after = context_snapshot()
            self.write("context-after.json", after)
            require(after == self.before, "default kubeconfig or existing kind cluster state changed")
        except Exception as error:
            errors.append("context verification: " + repr(error))
        self.report.update(cleanup=not errors, cleanup_errors=errors)
        if errors:
            self.report["passed"] = False
        self.persist()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True)
    parser.add_argument("--commons", default="~/workspace/git/github/zncdatadev/commons-operator")
    args = parser.parse_args()
    run = Run(args)
    def interrupted(signum, _frame):
        raise RuntimeError("interrupted by signal " + str(signum))
    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGINT, interrupted)
    try:
        run.build()
        run.start_cluster()
        run.platform.install()
        run.deploy()
        LifecycleVerifier(run, "trino-u05", "demo-trino").verify()
        AuthenticationVerifier(run, "trino-u05", "demo-trino").verify()
        PlatformVerifier(run, "trino-u05", "demo-trino").verify()
        S3Verifier(run, "trino-u05", "demo-trino", HIVE, MINIO, MINIO_MC).verify()
        run.storage()
        run.dataops()
        run.logging()
        run.report["passed"] = True
    except Exception as error:
        run.report["error"] = repr(error)
        print("U05 failed:", error, file=sys.stderr, flush=True)
    finally:
        try:
            run.close()
        except Exception as error:
            run.report.update(passed=False, cleanup_error=repr(error))
        run.persist()
    print("Evidence:", run.output, "passed:", run.report["passed"], "cleanup:", run.report["cleanup"], flush=True)
    raise SystemExit(0 if run.report["passed"] and run.report["cleanup"] else 1)


if __name__ == "__main__":
    main()
