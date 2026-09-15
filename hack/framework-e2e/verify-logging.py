#!/usr/bin/env python3
"""Verify standard Vector discovery and native Python events in an isolated namespace.

Requires an existing disposable cluster with the generated Trino CRD and the
pinned product/Vector images plus the built materializer loaded by run.py.
Owns only its newly created namespace and its local fixture controller process.
"""
import argparse
import hashlib
import json
import os
import pathlib
import signal
import subprocess
import sys
import time
import uuid

RESOURCE = "trinoclusters.trino.kubedoop.dev"
NAME = "logging"


def require(value, message):
    if not value:
        raise RuntimeError(message)


class Verification:
    def __init__(self, args):
        self.args = args
        self.output = pathlib.Path(args.output_dir).resolve()
        self.output.mkdir(parents=True, exist_ok=True)
        require(not any(self.output.iterdir()), "Use a new empty output directory")
        (self.output / "commands").mkdir()
        self.deadline = time.monotonic() + args.timeout
        self.index = 0
        self.controller = self.stream = None
        self.created = False
        self.report = {"passed": False, "cleanup": False, "phases": [],
                       "scope": "generated CR reference, live controller refresh, native Python log file and Vector protocol receivers"}
        self.persist()

    def persist(self):
        (self.output / "verification.json").write_text(json.dumps(self.report, indent=2) + "\n")

    def command(self, argv, input=None, timeout=30, check=True):
        self.index += 1
        started = time.monotonic()
        result = subprocess.run(list(map(str, argv)), input=input, capture_output=True, text=True, timeout=timeout, check=False)
        receipt = {"argv": list(map(str, argv)), "returncode": result.returncode, "stdout": result.stdout,
                   "stderr": result.stderr, "elapsed_seconds": time.monotonic() - started}
        if input is not None:
            receipt["stdin_sha256"] = hashlib.sha256(input.encode()).hexdigest()
        (self.output / "commands" / f"{self.index:05d}.json").write_text(json.dumps(receipt, indent=2) + "\n")
        if check:
            require(result.returncode == 0, "command failed: " + result.stderr[-1500:])
        return result.stdout

    def kube(self, *args, **kwargs):
        return self.command(["kubectl", "--kubeconfig", self.args.kubeconfig, "--request-timeout=20s", "-n", self.args.namespace,
                             *args], **kwargs)

    def get(self, kind, name):
        output = self.kube("get", kind, name, "--ignore-not-found", "-o", "json")
        return json.loads(output) if output.strip() else None

    def apply(self, value):
        self.kube("apply", "-f", "-", input=json.dumps(value))

    def until(self, label, fn, seconds=150):
        end, last = min(self.deadline, time.monotonic() + seconds), None
        while time.monotonic() < end:
            if self.controller is not None:
                require(self.controller.poll() is None, "fixture controller exited")
            try:
                value = fn()
                if value:
                    return value
            except (RuntimeError, KeyError, TypeError, ValueError) as error:
                last = str(error)
            time.sleep(1)
        raise RuntimeError(label + " timed out: " + str(last))

    def phase(self, name, evidence):
        self.report["phases"].append({"name": name, "evidence": evidence})
        self.persist()
        print("PASS logging:", name, flush=True)

    def start(self):
        require(self.get("namespace", self.args.namespace) is None, "Namespace already exists; refusing to use it")
        self.apply({"apiVersion": "v1", "kind": "Namespace", "metadata": {"name": self.args.namespace}})
        self.created = True
        binary = pathlib.Path(self.args.controller_binary).resolve()
        argv = [str(binary), "--kubeconfig", self.args.kubeconfig, "--namespace", self.args.namespace,
                "--image", self.args.image, "--materializer-image", self.args.materializer_image,
                "--vector-image", self.args.vector_image]
        self.stream = (self.output / "controller.log").open("wb")
        self.controller = subprocess.Popen(argv, stdout=self.stream, stderr=subprocess.STDOUT, start_new_session=True)
        self.report["controller"] = {"argv": argv, "pid": self.controller.pid,
                                     "binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest()}
        self.persist()

    def group(self, cr, name="default"):
        return next((g for g in cr.get("status", {}).get("groups", []) if g["role"] == "workers" and g["name"] == name), {})

    def workload_name(self, group="default"):
        return NAME + "-workers-" + group

    def refreshed_provenance(self, cm, generation):
        cr = self.get(RESOURCE, NAME)
        require(cr and cr["metadata"]["generation"] == generation, "refresh unexpectedly edited CR spec")
        facts = self.group(cr).get("facts", {})
        observed = facts.get("observed", [])
        require(facts.get("state") == "resolved" and any(
            o.get("apiVersion") == "v1" and o.get("kind") == "ConfigMap" and
            o.get("namespace") == cm["metadata"]["namespace"] and o.get("name") == "destination" and
            o.get("uid") == cm["metadata"]["uid"] and o.get("resourceVersion") == cm["metadata"]["resourceVersion"]
            for o in observed), "fresh CM provenance missing")
        return observed

    def ready(self, group="default", previous=None):
        cr = self.get(RESOURCE, NAME)
        require(cr and cr.get("status", {}).get("observedGeneration") == cr["metadata"]["generation"], "current CR not observed")
        observation = self.group(cr, group)
        require(observation.get("applied"), "group not applied")
        if group == "default":
            require(observation.get("facts", {}).get("state") == "resolved", "destination not resolved")
        name = self.workload_name(group)
        sts, pod = self.get("statefulset", name), self.get("pod", name + "-0")
        require(sts and pod and not pod["metadata"].get("deletionTimestamp"), "missing or terminating workload")
        status = sts.get("status", {})
        require(status.get("observedGeneration", 0) >= sts["metadata"]["generation"] and
                status.get("currentRevision") == status.get("updateRevision") and status.get("readyReplicas") == 1,
                "StatefulSet not converged")
        require(all(c.get("ready") and c.get("restartCount") == 0 for c in pod.get("status", {}).get("containerStatuses", [])),
                "container not ready or restarted")
        if previous:
            require(pod["metadata"]["uid"] != previous["metadata"]["uid"], "destination refresh did not replace Pod")
        return pod

    def destination(self, address):
        self.apply({"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": "destination", "namespace": self.args.namespace},
                    "data": {"ADDRESS": address}})
        return self.get("configmap", "destination")

    def receiver(self, name):
        config = {"data_dir": "/var/lib/vector", "sources": {"upstream": {"type": "vector", "address": "0.0.0.0:6000"}},
                  "sinks": {"proof": {"type": "console", "inputs": ["upstream"], "target": "stdout", "encoding": {"codec": "json"}}}}
        metadata = {"name": name, "namespace": self.args.namespace}
        self.apply({"apiVersion": "v1", "kind": "ConfigMap", "metadata": metadata, "data": {"vector.json": json.dumps(config)}})
        self.apply({"apiVersion": "v1", "kind": "Service", "metadata": metadata,
                    "spec": {"selector": {"receiver": name}, "ports": [{"name": "vector", "port": 6000, "targetPort": 6000}]}})
        self.apply({"apiVersion": "v1", "kind": "Pod", "metadata": {**metadata, "labels": {"receiver": name}},
                    "spec": {"securityContext": {"fsGroup": 1000}, "containers": [{"name": "vector", "image": self.args.vector_image,
                             "command": ["vector"], "args": ["--config", "/config/vector.json"],
                             "resources": {"requests": {"cpu": "25m", "memory": "64Mi"}, "limits": {"cpu": "500m", "memory": "192Mi"}},
                             "readinessProbe": {"tcpSocket": {"port": 6000}, "periodSeconds": 1},
                             "volumeMounts": [{"name": "config", "mountPath": "/config", "readOnly": True},
                                              {"name": "data", "mountPath": "/var/lib/vector"}]}],
                             "volumes": [{"name": "config", "configMap": {"name": name}}, {"name": "data", "emptyDir": {}}]}})
        self.until("receiver " + name, lambda: any(c.get("ready") for c in
                   self.get("pod", name).get("status", {}).get("containerStatuses", [])))

    def emit(self, marker):
        code = "import urllib.request; print(urllib.request.urlopen('http://127.0.0.1:8080/?marker=" + marker + "',timeout=3).read().decode())"
        result = self.kube("exec", self.workload_name() + "-0", "-c", "python", "--", "python3", "-c", code)
        require(marker in result, "native process did not acknowledge marker emission")
        return True

    def delivered(self, receiver, marker):
        self.until("native marker emission", lambda: self.emit(marker), seconds=30)
        filename = "/logs/server.log"
        native = self.kube("exec", self.workload_name() + "-0", "-c", "python", "--", "cat", filename).splitlines()
        line = next((line for line in native if marker + "-debug" in line), None)
        require(line, "native Python file did not consume DEBUG threshold")
        console = self.kube("logs", self.workload_name() + "-0", "-c", "python")
        require(marker + "-warning" in console and marker + "-debug" not in console, "native console threshold mismatch")
        def observed():
            records = []
            for value in self.kube("logs", receiver, "-c", "vector").splitlines():
                try:
                    record = json.loads(value)
                except ValueError:
                    continue
                if record.get("message") == line:
                    records.append(record)
            return records
        events = self.until("exact native event at " + receiver, observed, seconds=60)
        return {"marker": marker, "native_file": filename, "native_line": line, "received_event": events[0],
                "receiver": receiver, "receiver_pod_uid": self.get("pod", receiver)["metadata"]["uid"]}

    def verify(self):
        self.start()
        self.apply({"apiVersion": "trino.kubedoop.dev/v1alpha1", "kind": "TrinoCluster",
                    "metadata": {"name": NAME, "namespace": self.args.namespace},
                    "spec": {"clusterConfig": {"vectorAgentConfigMap": "destination"},
                             "workers": {"roleGroups": {"default": {"replicas": 1},
                                 "quiet": {"replicas": 1, "config": {"logging": {"enableVectorAgent": False}}},
                                 "nofiles": {"replicas": 1, "config": {"logging": {"containers": {"python": {"file": {"level": "OFF"}}}}}}}}}})
        def missing():
            cr = self.get(RESOURCE, NAME)
            require(cr and self.group(cr).get("facts", {}).get("state") == "pending", "missing CM not Pending")
            require(self.get("statefulset", self.workload_name()) is None, "unresolved collector producer was created")
            return self.ready("quiet")
        quiet = self.until("missing destination isolates enabled group", missing)
        nofiles = self.until("file-OFF group does not wait", lambda: self.ready("nofiles"))
        require(not any(c["name"] == "vector" for c in quiet["spec"]["containers"]), "disabled group has a collector")
        require(not any(c["name"] == "vector" for c in nofiles["spec"]["containers"]), "file-OFF group has a collector")
        self.phase("missing-reference-isolated", {"cr": self.get(RESOURCE, NAME), "quiet_pod_uid": quiet["metadata"]["uid"],
                   "nofiles_pod_uid": nofiles["metadata"]["uid"]})
        self.receiver("receiver-a")
        self.receiver("receiver-b")
        cm = self.destination("receiver-a." + self.args.namespace + ".svc:6000")
        first = self.until("destination resolved", self.ready)
        event = self.delivered("receiver-a", "e03-a-" + uuid.uuid4().hex)
        self.phase("native-file-to-central-vector", {"event": event, "pod": first,
                   "destination_uid": cm["metadata"]["uid"], "destination_rv": cm["metadata"]["resourceVersion"]})
        generation = self.get(RESOURCE, NAME)["metadata"]["generation"]
        cm = self.destination("receiver-b." + self.args.namespace + ".svc:6000")
        second = self.until("CM-only refresh replaced enabled Pod", lambda: self.ready(previous=first))
        event = self.delivered("receiver-b", "e03-b-" + uuid.uuid4().hex)
        # Pod replacement and log delivery can finish before this reconcile has
        # observed the sibling groups and persisted its final CR status.
        observed = self.until("fresh destination provenance persisted", lambda: self.refreshed_provenance(cm, generation))
        require(self.ready("quiet")["metadata"]["uid"] == quiet["metadata"]["uid"], "disabled group rolled on destination update")
        require(self.ready("nofiles")["metadata"]["uid"] == nofiles["metadata"]["uid"], "file-OFF group rolled on destination update")
        self.phase("cm-only-destination-refresh", {"event": event, "pod": second, "cr_generation": generation, "observed": observed})
        self.destination("")
        def invalid():
            cr = self.get(RESOURCE, NAME)
            require(self.group(cr).get("facts", {}).get("state") == "invalid", "empty ADDRESS not Invalid")
            require(self.get("pod", self.workload_name() + "-0")["metadata"]["uid"] == second["metadata"]["uid"],
                    "invalid reference destroyed the previous running Pod")
            return cr
        self.phase("invalid-destination-preserves-runtime", self.until("invalid discovery", invalid))
        self.report["passed"] = True

    def close(self):
        failures = []
        if self.controller is not None:
            try:
                if self.controller.poll() is None:
                    os.killpg(self.controller.pid, signal.SIGTERM)
                code = self.controller.wait(timeout=20)
                self.report["controller"].update(returncode=code, reaped=True)
                require(code == 0, "controller shutdown was not graceful")
            except Exception as error:
                failures.append(repr(error))
                if self.controller.poll() is None:
                    os.killpg(self.controller.pid, signal.SIGKILL)
                    self.controller.wait(timeout=5)
            finally:
                self.stream.close()
        if self.created:
            try:
                for name in (self.workload_name() + "-0", "receiver-a", "receiver-b"):
                    self.kube("logs", name, "-c", "vector", check=False)
                self.kube("get", "pods,configmaps,services,statefulsets," + RESOURCE, "-o", "json")
                self.kube("delete", "namespace", self.args.namespace, "--wait=true", "--timeout=120s", timeout=140)
                require(self.get("namespace", self.args.namespace) is None, "namespace cleanup not confirmed")
            except Exception as error:
                failures.append(repr(error))
        self.report.update(cleanup=not failures, cleanup_errors=failures)
        if failures:
            self.report["passed"] = False
        self.persist()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("kubeconfig", "namespace", "output-dir", "controller-binary", "image", "materializer-image", "vector-image"):
        parser.add_argument("--" + name, required=True)
    parser.add_argument("--timeout", type=int, default=600)
    run = Verification(parser.parse_args())
    def interrupted(signum, _frame):
        raise RuntimeError("interrupted by signal " + str(signum))
    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGINT, interrupted)
    try:
        run.verify()
    except Exception as error:
        run.report.update(passed=False, error=repr(error))
        print("Logging acceptance failed:", error, file=sys.stderr, flush=True)
    finally:
        run.close()
    print("Evidence:", run.output, "passed:", run.report["passed"], "cleanup:", run.report["cleanup"], flush=True)
    raise SystemExit(0 if run.report["passed"] and run.report["cleanup"] else 1)


if __name__ == "__main__":
    main()
