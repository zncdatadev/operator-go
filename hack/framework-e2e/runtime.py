"""Verify the delivered Trino operator through CR/API/process observations."""
import copy
import json
import time

RESOURCE = "trinoclusters.trino.kubedoop.dev"


class VerificationError(RuntimeError):
    pass


def require(value, message):
    if not value:
        raise VerificationError(message)


def named(items, name):
    return next((item for item in items if item.get("name") == name), None)


def current_condition(cr, name, value="True"):
    # Kubernetes conditions identify their kind by type, not name.
    condition = next((c for c in cr.get("status", {}).get("conditions", []) if c["type"] == name), None)
    return bool(condition and condition.get("status") == value and
                condition.get("observedGeneration") == cr["metadata"]["generation"])


QUERY = r'''import json,sys,time,urllib.request
sql=sys.argv[1]
request=urllib.request.Request('http://127.0.0.1:8080/v1/statement',data=sql.encode(),headers={'X-Trino-User':'framework-e2e'})
rows=[];query_id=None;end=time.monotonic()+90
while True:
 with urllib.request.urlopen(request,timeout=15) as response: result=json.load(response)
 query_id=result.get('id',query_id)
 if 'error' in result: raise RuntimeError(json.dumps(result['error']))
 rows.extend(result.get('data',[]))
 if not result.get('nextUri'): break
 if time.monotonic()>end: raise TimeoutError('query '+str(query_id))
 request=urllib.request.Request(result['nextUri'],headers={'X-Trino-User':'framework-e2e'})
 time.sleep(.2)
print(json.dumps({'query_id':query_id,'sql':sql,'rows':rows}))
'''


class RuntimeVerifier:
    def __init__(self, run, namespace, name):
        self.run, self.namespace, self.name = run, namespace, name
        self.phases = []
        self.report = {"passed": False, "phases": self.phases,
                       "scope": "formal operator deployment, API state, Trino SQL and Vector delivery"}
        self.persist()

    def persist(self):
        self.run.write("runtime.json", self.report)

    def phase(self, name, evidence):
        self.phases.append({"name": name, "evidence": evidence})
        self.persist()
        print("PASS runtime phase:", name, flush=True)

    def kube(self, *args, **kwargs):
        return self.run.kube("-n", self.namespace, *args, **kwargs)

    def get(self, kind, name):
        result = self.kube("get", kind, name, "--ignore-not-found", "-o", "json")
        return json.loads(result) if result.strip() else None

    def cr(self):
        return self.get(RESOURCE, self.name)

    def patch(self, spec):
        self.kube("patch", RESOURCE, self.name, "--type=merge", "-p", json.dumps({"spec": spec}))

    def until(self, label, observe, timeout=600):
        end, error = time.monotonic() + timeout, None
        while time.monotonic() < end:
            try:
                value = observe()
                if value:
                    return value
            except (VerificationError, KeyError, TypeError, ValueError) as caught:
                error = str(caught)
            time.sleep(2)
        raise VerificationError(label + " timed out; last observation: " + str(error))

    def healthy(self, roles=("coordinators", "workers"), previous=None):
        cr = self.cr()
        require(cr is not None, "cluster CR missing")
        for condition in ("Built", "Applied", "WorkloadsReady", "RoleResourcesApplied"):
            require(current_condition(cr, condition), "current " + condition + " is not true")
        result = {"generation": cr["metadata"]["generation"], "cr_uid": cr["metadata"]["uid"], "cr_rv": cr["metadata"]["resourceVersion"], "groups": {}}
        for role in roles:
            name = self.name + "-" + role + "-default"
            sts, pod, cm = self.get("statefulset", name), self.get("pod", name + "-0"), self.get("configmap", name)
            require(sts and pod and cm, "missing runtime object for " + role)
            observed = sts.get("status", {})
            require(not pod["metadata"].get("deletionTimestamp"), "Pod is terminating")
            require(observed.get("observedGeneration", 0) >= sts["metadata"]["generation"] and
                    observed.get("currentRevision") == observed.get("updateRevision") and
                    all(observed.get(k) == 1 for k in ("replicas", "readyReplicas", "updatedReplicas")),
                    "StatefulSet has not converged")
            require(pod["metadata"]["labels"].get("controller-revision-hash") == observed["updateRevision"],
                    "Pod has an old revision")
            for container in pod.get("status", {}).get("containerStatuses", []):
                require(container.get("ready") and container.get("restartCount") == 0, "container unready/restarted")
            init = named(pod.get("status", {}).get("initContainerStatuses", []), "prepare-files")
            require(init and init.get("state", {}).get("terminated", {}).get("exitCode") == 0,
                    "formal materializer did not complete")
            for obj in (sts, cm):
                owners = [o for o in obj["metadata"].get("ownerReferences", []) if o.get("controller")]
                require(len(owners) == 1 and owners[0]["uid"] == cr["metadata"]["uid"], "object owner differs")
                require("framework.kubedoop.dev/group-slot" in obj["metadata"].get("annotations", {}),
                        "formal slot receipt missing")
            stamp = "configmap.restarter.kubedoop.dev/" + name
            expected_stamp = cm["metadata"]["uid"] + "/" + cm["metadata"]["resourceVersion"]
            require(sts["metadata"]["labels"].get("restarter.kubedoop.dev/enable") == "true", "restarter opt-in absent")
            require(sts["spec"]["template"]["metadata"].get("annotations", {}).get(stamp) == expected_stamp and
                    pod["metadata"].get("annotations", {}).get(stamp) == expected_stamp, "configuration has not reached Pod")
            result["groups"][role] = {"statefulset_uid": sts["metadata"]["uid"], "pod_uid": pod["metadata"]["uid"],
                                      "statefulset_rv": sts["metadata"]["resourceVersion"], "configmap_rv": cm["metadata"]["resourceVersion"],
                                      "pod": name + "-0", "stamp": expected_stamp}
            if previous and role in previous["groups"]:
                require(pod["metadata"]["uid"] != previous["groups"][role]["pod_uid"], "expected replacement Pod not observed")
        return result

    def wait_healthy(self, roles=("coordinators", "workers"), previous=None):
        return self.until("healthy " + repr(roles), lambda: self.healthy(roles, previous))

    def exec(self, role, code, *args):
        output = self.kube("exec", self.name + "-" + role + "-default-0", "-c", "trino", "--", "python3", "-c", code, *args,
                           timeout=120)
        return json.loads(output)

    def query(self, sql):
        return self.exec("coordinators", QUERY, sql)

    def wait_membership(self, snapshot):
        expected = {g["pod_uid"] for g in snapshot["groups"].values()}
        probes = []
        def membership():
            observation = self.query("SELECT node_id FROM system.runtime.nodes WHERE state = 'active'")
            probes.append(observation)
            return observation if {row[0] for row in observation["rows"]} == expected else None
        node_ids = self.until("current native node identities", membership, timeout=120)
        self.report.setdefault("membership_probes", []).append(probes)
        self.persist()
        return node_ids

    def prove_processes(self, snapshot):
        node_ids = self.wait_membership(snapshot)
        count = self.query("SELECT count(*) FROM tpch.tiny.nation")
        require(count["rows"] == [[25]], "TPCH did not complete on the recovered cluster")
        return {"node_ids": node_ids, "count": count}

    def prove_logs(self):
        lines = self.exec("workers", "import json,pathlib; print(json.dumps(pathlib.Path('/kubedoop/log/trino/server.json').read_text().splitlines()))")
        native = [line for line in lines if line.startswith("{")]
        require(native, "Trino has no native JSON file events")
        def collected():
            logs = self.kube("logs", self.name + "-workers-default-0", "-c", "vector", "--tail=1000")
            messages = []
            for line in logs.splitlines():
                try:
                    record = json.loads(line)
                except ValueError:
                    continue
                if record.get("message") in native and record.get("file") == "/logs/log/server.json":
                    messages.append(record)
            return messages
        events = self.until("Vector native-file delivery", collected, timeout=90)
        return events[0]

    def verify(self):
        baseline = self.wait_healthy()
        self.phase("baseline", {"resources": baseline, "sql": self.prove_processes(baseline), "collected_event": self.prove_logs()})
        previous = baseline
        patches = {
            "file": {"configOverrides": {"config.properties": {"properties": {"set": {"query.max-memory": "320MB"}}}}},
            "env": {"envOverrides": {"FRAMEWORK_UPDATE_ENV": "env-v2"}},
            "cli": {"cliOverrides": ["--etc-dir=/etc/trino", "-D", "operator.go.update=cli-v2", "run"]},
            "pod": {"podOverrides": {"spec": {"containers": [{"name": "trino", "env": [{"name": "FRAMEWORK_UPDATE_POD", "value": "pod-v2"}]}]}}},
        }
        for phase, patch in patches.items():
            self.patch({"workers": {"roleGroups": {"default": patch}}})
            current = self.wait_healthy(previous={"groups": {"workers": previous["groups"]["workers"]}})
            observation = self.exec("workers", r'''import json,os,pathlib
jvm=[]
for process in pathlib.Path('/proc').iterdir():
 if process.name.isdigit():
  try: argv=[part.decode() for part in (process/'cmdline').read_bytes().split(b'\0') if part]
  except OSError: continue
  if argv and pathlib.Path(argv[0]).name=='java' and 'io.trino.server.TrinoServer' in argv: jvm.append(' '.join(argv))
print(json.dumps({'config':pathlib.Path('/etc/trino/config.properties').read_text(),'env':{key:os.getenv(key) for key in ('FRAMEWORK_UPDATE_ENV','FRAMEWORK_UPDATE_POD','TRINO_NODE_ID')},'jvm':jvm}))
''')
            if phase == "file": require("query.max-memory=320MB\n" in observation["config"], "file update absent")
            if phase == "env": require(observation["env"].get("FRAMEWORK_UPDATE_ENV") == "env-v2", "env update absent")
            if phase == "cli": require(any("-Doperator.go.update=cli-v2" in line for line in observation["jvm"]), "CLI not consumed by JVM")
            if phase == "pod": require(observation["env"].get("FRAMEWORK_UPDATE_POD") == "pod-v2", "Pod env update absent")
            self.phase("override-" + phase, {"resources": current, "process": observation})
            previous = current
        self.verify_catalogs()
        self.verify_pause_stop()
        self.verify_retirement()
        final = self.wait_healthy()
        self.phase("final-business", {"resources": final, "sql": self.prove_processes(final), "collected_event": self.prove_logs()})
        time.sleep(35)  # Includes a full default facts refresh interval.
        stable = self.healthy()
        require(stable == final, "stable resources changed during a full refresh interval")
        self.phase("steady", {"seconds": 35, "unchanged": stable})
        self.report["passed"] = True
        self.persist()

    def verify_catalogs(self):
        catalogs = {"tpch": {"connector.name": "tpch"}}
        self.run.apply({"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": "runtime-catalogs", "namespace": self.namespace},
                        "data": {"catalogs.json": json.dumps(catalogs)}})
        self.patch({role: {"config": {"catalogConfigMapName": "runtime-catalogs"}} for role in ("coordinators", "workers")})
        self.wait_healthy()
        before = self.cr()["metadata"]["generation"]
        catalogs["tpch_extra"] = {"connector.name": "tpch"}
        self.kube("patch", "configmap", "runtime-catalogs", "--type=merge", "-p", json.dumps({"data": {"catalogs.json": json.dumps(catalogs)}}))
        def delivered():
            snapshot = self.healthy()
            for role in ("coordinators", "workers"):
                values = self.exec(role, "import json,pathlib; print(json.dumps([p.name for p in pathlib.Path('/etc/trino/catalog').glob('*.properties')]))")
                require("tpch_extra.properties" in values, "catalog not yet materialized in " + role)
            return snapshot
        current = self.until("external catalog refresh", delivered)
        require(self.cr()["metadata"]["generation"] == before, "catalog refresh required a CR spec change")
        # HTTP readiness and materialized files precede SQL access-control readiness.
        # Wait through explicit membership probes; execute the business query once.
        node_ids = self.wait_membership(current)
        sql = self.query("SELECT count(*) FROM tpch_extra.tiny.nation")
        require(sql["rows"] == [[25]], "new catalog not consumed by coordinator and worker")
        self.phase("catalog-refresh", {"generation_unchanged": before, "resources": current, "node_ids": node_ids, "query": sql})

    def verify_pause_stop(self):
        before = self.wait_healthy()
        self.patch({"clusterConfig": {"reconciliationPaused": True}})
        self.until("paused", lambda: current_condition(self.cr(), "Paused"))
        self.patch({"workers": {"roleGroups": {"default": {"envOverrides": {"FRAMEWORK_PAUSED_EDIT": "latest"}}}}})
        self.until("paused latest generation", lambda: current_condition(self.cr(), "Paused"))
        time.sleep(5)
        for role, state in before["groups"].items():
            sts = self.get("statefulset", self.name + "-" + role + "-default")
            require(sts["metadata"]["resourceVersion"] == state["statefulset_rv"], "paused controller changed workload")
        self.patch({"clusterConfig": {"reconciliationPaused": False}})
        resumed = self.wait_healthy(previous={"groups": {"workers": before["groups"]["workers"]}})
        require(self.exec("workers", "import os,json; print(json.dumps(os.getenv('FRAMEWORK_PAUSED_EDIT')))") == "latest",
                "resume did not consume latest CR")
        self.phase("pause-resume", resumed)
        self.patch({"clusterConfig": {"stopped": True}})
        def stopped():
            cr = self.cr()
            require(current_condition(cr, "Stopped"), "controller has not observed fully stopped")
            pods = json.loads(self.kube("get", "pods", "-l", "app.kubernetes.io/instance=" + self.name, "-o", "json"))
            require(not pods["items"], "stopped still has actual Pods")
            for role in ("coordinators", "workers"):
                require(self.get("statefulset", self.name + "-" + role + "-default")["spec"]["replicas"] == 0,
                        "stopped workload still has execution replicas")
            return cr["status"]
        observation = self.until("fully stopped", stopped)
        self.patch({"clusterConfig": {"stopped": False}})
        resumed = self.wait_healthy(previous=resumed)
        self.phase("stop-resume", {"stopped_status": observation, "resources": resumed, "sql": self.prove_processes(resumed)})

    def verify_retirement(self):
        before = self.wait_healthy()
        group = copy.deepcopy(self.cr()["spec"]["workers"]["roleGroups"]["default"])
        self.patch({"workers": {"roleGroups": {"default": None}}})
        name = self.name + "-workers-default"
        def retired():
            require(current_condition(self.cr(), "Retired"), "retirement not yet observed")
            for kind, resource in (("pod", name + "-0"), ("statefulset", name), ("service", name),
                                   ("service", name + "-headless"), ("configmap", name)):
                require(self.get(kind, resource) is None, "retired resource remains: " + kind + "/" + resource)
            return True
        self.until("worker retirement", retired)
        self.run.restart_operator()
        self.patch({"workers": {"roleGroups": {"default": group}}})
        after = self.wait_healthy()
        require(after["groups"]["workers"]["statefulset_uid"] != before["groups"]["workers"]["statefulset_uid"],
                "retired workload was not recreated")
        require(after["groups"]["coordinators"]["pod_uid"] == before["groups"]["coordinators"]["pod_uid"],
                "unrelated coordinator was recreated")
        self.phase("retirement-restart-readd", {"before": before, "after": after})
