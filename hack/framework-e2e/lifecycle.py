"""Formal Trino lifecycle acceptance on run.py's explicitly disposable cluster.

No alternate operator or product lifecycle implementation is used: this verifier
only changes CR intent and records the formal operator's/native kubelet's effects.
The caller owns the cluster, operator, materializer and commons restarter.
"""
import copy
import json
import pathlib
import subprocess
import time

from runtime import RuntimeVerifier, current_condition, named, require

ADMIN = "framework-e04-local-admin"

QUERY = r'''import http.client,json,sys,time,urllib.request,urllib.error
headers={'X-Trino-User':'framework-e2e'}
r=urllib.request.Request('http://127.0.0.1:8080/v1/statement',data=sys.argv[1].encode(),headers=headers)
rows=[];query=None;deadline=time.monotonic()+120;attempt=0
while True:
 remaining=deadline-time.monotonic()
 if remaining<=0:raise TimeoutError('original query did not complete within total deadline')
 attempt+=1;started=time.monotonic()
 print(json.dumps({'query_id':query,'request':r.full_url,'method':r.get_method(),'attempt':attempt}),flush=True)
 try:
  with urllib.request.urlopen(r,timeout=min(10,remaining)) as response:value=json.load(response)
 except (OSError,urllib.error.URLError,http.client.HTTPException) as error:
  print(json.dumps({'query_id':query,'transfer_error':type(error).__name__,'elapsed':time.monotonic()-started,'attempt':attempt}),flush=True)
  # A repeated POST could submit another query. Only replay the same GET token
  # for the known original query, within the unchanged overall deadline.
  if query is None or r.get_method()!='GET':raise
  if isinstance(error,urllib.error.HTTPError) and error.code not in (502,503,504):raise
  time.sleep(.25);continue
 if query is None:
  query=value['id'];print(json.dumps({'query_id':query,'started':True}),flush=True)
 if value.get('id')!=query:raise RuntimeError('response changed query identity')
 print(json.dumps({'query_id':query,'state':value.get('stats',{}).get('state'),'elapsed':time.monotonic()-started,'attempt':attempt}),flush=True)
 rows.extend(value.get('data',[]))
 if 'error' in value:
  print(json.dumps({'query_id':query,'error':value['error']}),flush=True);sys.exit(1)
 if not value.get('nextUri'):
  print(json.dumps({'query_id':query,'rows':rows,'complete':True}),flush=True);break
 r=urllib.request.Request(value['nextUri'],headers=headers);time.sleep(.1)
'''
TASKS = r'''import json,sys,urllib.request
r=urllib.request.Request('http://127.0.0.1:8080/v1/query/'+sys.argv[1],headers={'X-Trino-User':'framework-e2e'})
with urllib.request.urlopen(r,timeout=5) as response: value=json.load(response)
tasks=[]
def visit(stage):
 for task in stage.get('tasks',[]):
  s=task.get('taskStatus',{});tasks.append({k:s.get(k) for k in ('taskId','state','nodeId','self')})
 for child in stage.get('subStages',[]):visit(child)
visit(value.get('outputStage',{}))
print(json.dumps({'query_id':value.get('queryId'),'state':value.get('state'),'tasks':tasks}))
'''


def json_stream(path):
    text = pathlib.Path(path).read_text()
    decoder, offset, result = json.JSONDecoder(), 0, []
    while offset < len(text):
        while offset < len(text) and text[offset].isspace():
            offset += 1
        if offset == len(text):
            break
        value, offset = decoder.raw_decode(text, offset)
        result.append(value)
    return result


class LifecycleVerifier(RuntimeVerifier):
    def persist(self):
        self.run.write("lifecycle.json", self.report)

    def ready_pods(self, workers=1):
        cr = self.cr()
        require(current_condition(cr, "WorkloadsReady"), "workloads not ready")
        result = {}
        for role, count in (("coordinators", 1), ("workers", workers)):
            set_name = self.name + "-" + role + "-default"
            sts = self.get("statefulset", set_name)
            require(sts and sts["spec"]["replicas"] == count and
                    sts.get("status", {}).get("readyReplicas", 0) == count, "replicas not converged")
            policy = json.loads(sts["metadata"].get("annotations", {}).get("framework.kubedoop.dev/workload-coordination", "null"))
            require(policy and policy["shutdownPriority"] == (100 if role == "coordinators" else 0),
                    "controller coordination receipt missing or role shutdown priority lost")
            require(sts["spec"]["podManagementPolicy"] == "OrderedReady" and
                    sts["spec"]["updateStrategy"]["type"] == "RollingUpdate", "native ordered policy absent")
            for ordinal in range(count):
                pod = self.get("pod", set_name + "-" + str(ordinal))
                require(pod and not pod["metadata"].get("deletionTimestamp"), "Pod missing or terminating")
                states = pod.get("status", {})
                init = named(states.get("initContainerStatuses", []), "initialize-trino")
                require(init and init.get("state", {}).get("terminated", {}).get("exitCode") == 0,
                        "product initialization did not finish before startup")
                main = named(states.get("containerStatuses", []), "trino")
                require(main and main.get("ready") and main.get("restartCount") == 0, "main not ready or restarted")
                result[pod["metadata"]["name"]] = {"uid": pod["metadata"]["uid"], "container_id": main["containerID"]}
        return result

    def verify(self):
        original = copy.deepcopy(self.cr()["spec"])
        processes, handles = [], []
        self.report["scope"] = "formal initialization, ordinal scale-down, native rolling and active-query coordinated stop"
        try:
            # This isolated test explicitly authorizes a local management identity.
            # The production default does not generate this access-control grant.
            rules = {"catalogs": [{"user": ".*", "catalog": ".*", "allow": "all"}],
                     "system_information": [{"user": ADMIN, "allow": ["read", "write"]},
                                            {"user": ".*", "allow": ["read"]}]}
            files = {"access-control.properties": {"text": "access-control.name=file\nsecurity.config-file=/etc/trino/access-control.json\n"},
                     "access-control.json": {"text": json.dumps(rules)},
                     "catalog/blackhole.properties": {"text": "connector.name=blackhole\n"},
                     "config.properties": {"properties": {"set": {"shutdown.grace-period": "5s"}}}}
            self.patch({role: {"config": {"gracefulShutdownTimeout": "120s", **({"shutdownUser": ADMIN} if role == "workers" else {})},
                               "configOverrides": files} for role in ("coordinators", "workers")})
            baseline = self.until("formal lifecycle initialization", self.ready_pods)
            self.phase("initialization", baseline)
            self.patch({"workers": {"roleGroups": {"default": {"replicas": 2}}}})
            two = self.until("two initialized workers", lambda: self.ready_pods(2))
            self.phase("scale-up-initialization", two)
            self.patch({"workers": {"roleGroups": {"default": {"replicas": 1}}}})
            scaled = self.until("ordinal scale-down", self.ready_pods)
            require(scaled[self.name + "-workers-default-0"] == two[self.name + "-workers-default-0"],
                    "scale-down replaced the surviving ordinal")
            self.phase("scale-down", scaled)
            self.patch({"workers": {"roleGroups": {"default": {"envOverrides": {"E04_ROLLOUT": "typed-lifecycle"}}}}})
            def rolled():
                result = self.ready_pods()
                require(result[self.name + "-workers-default-0"]["uid"] != scaled[self.name + "-workers-default-0"]["uid"],
                        "native rolling replacement not observed")
                return result
            rolled = self.until("native rolling update", rolled)
            self.phase("rolling", rolled)
            # Re-establish real membership before the single business query.
            self.wait_membership({"groups": {name: {"pod_uid": state["uid"]} for name, state in rolled.items()}})
            self.query("CREATE TABLE blackhole.default.e04_rows (id BIGINT) WITH (split_count=2,pages_per_split=4,rows_per_page=8,page_processing_delay='2s')")
            # kubectl performs the initial list and resumes its watch at that
            # list's version. Wait for the original Pod event before querying;
            # `kubectl get` has no --resource-version flag.

            watch_path = self.run.output / "lifecycle-pod-watch.json"
            stream = watch_path.open("w"); handles.append(stream)
            watch_args = ["kubectl", "--kubeconfig", str(self.run.kubeconfig), "-n", self.namespace, "get", "pods",
                          "-l", "app.kubernetes.io/instance=" + self.name, "--watch", "--output-watch-events", "-o", "json"]
            watch_error = (self.run.output / "lifecycle-pod-watch.stderr").open("w"); handles.append(watch_error)
            watch = subprocess.Popen(watch_args, stdout=stream, stderr=watch_error, text=True); processes.append(watch)
            def watch_started():
                require(watch.poll() is None, "Pod watch exited before shutdown; see lifecycle-pod-watch.stderr")
                target = rolled[self.name + "-workers-default-0"]["uid"]
                return any(event.get("object", {}).get("metadata", {}).get("uid") == target
                           for event in json_stream(watch_path))
            self.until("Pod watch initial worker identity", watch_started, timeout=30)
            query_path = self.run.output / "lifecycle-query.jsonl"
            stream = query_path.open("w"); handles.append(stream)
            args = ["kubectl", "--kubeconfig", str(self.run.kubeconfig), "-n", self.namespace, "exec",
                    self.name + "-coordinators-default-0", "-c", "trino", "--", "python3", "-c", QUERY,
                    "SELECT count(*) FROM blackhole.default.e04_rows"]
            query = subprocess.Popen(args, stdout=stream, stderr=subprocess.PIPE, text=True); processes.append(query)
            self.report["processes"] = [{"pid": watch.pid, "argv": watch_args}, {"pid": query.pid, "argv": args}]
            self.persist()
            def active():
                require(query.poll() is None, "business query finished before shutdown trigger")
                values = json_stream(query_path)
                started = next((value for value in values if value.get("started")), None)
                if started is None:
                    return None
                tasks = self.exec("coordinators", TASKS, started["query_id"])
                target_uid = rolled[self.name + "-workers-default-0"]["uid"]
                return tasks if any(task["state"] == "RUNNING" and task["nodeId"] == target_uid for task in tasks["tasks"]) else None
            active = self.until("exact current worker has running query tasks", active, timeout=60)
            self.patch({"clusterConfig": {"stopped": True}})
            # Observe the persisted transition before restarting the real operator.
            def draining():
                sts = self.get("statefulset", self.name + "-workers-default")
                return sts if sts["spec"]["replicas"] == 0 else None
            self.until("worker shutdown issued", draining, timeout=30)
            self.run.restart_operator()
            self.report["tasks_before_stop"] = active
            self.persist()
            query.wait(timeout=150)
            query_error = query.stderr.read()
            result = json_stream(query_path)
            require(query.returncode == 0 and result[-1].get("complete") and result[-1].get("rows") == [[64]],
                    "single in-flight query did not succeed: " + query_error + repr(result))
            def stopped():
                cr = self.cr()
                require(current_condition(cr, "Stopped"), "coordinated stop is not complete")
                for name in rolled:
                    require(self.get("pod", name) is None, "actual Pod remains")
                return cr["status"]
            stopped = self.until("all workloads stopped after worker completion", stopped, timeout=180)
            watch.terminate(); watch.wait(timeout=10); handles[0].flush()
            events = json_stream(watch_path)
            target = rolled[self.name + "-workers-default-0"]
            exits = []
            for event in events:
                pod = event.get("object", {})
                if pod.get("metadata", {}).get("uid") != target["uid"]:
                    continue
                main = named(pod.get("status", {}).get("containerStatuses", []), "trino")
                if main and main.get("containerID") == target["container_id"]:
                    exit_state = main.get("state", {}).get("terminated")
                    if exit_state and exit_state.get("finishedAt") and exit_state.get("reason") != "ContainerStatusUnknown":
                        exits.append(exit_state)
            require(exits and all(value["exitCode"] == 0 for value in exits),
                    "no exact original main-process exit=0 proof; Pod absence alone is insufficient")
            hook_events = json.loads(self.kube("get", "events", "--field-selector", "involvedObject.uid=" + target["uid"], "-o", "json"))
            self.phase("active-query-stop-controller-restart", {"tasks_before": active, "query": result,
                       "worker_main_exits": exits, "hook_events": hook_events, "stopped": stopped,
                       "boundary": "main exit and query are proven separately; hook success is not inferred"})
            self.patch({"clusterConfig": {"stopped": False}})
            restored = self.until("resume after business stop", self.ready_pods)
            self.phase("resume", restored)
            self.report["passed"] = True
        finally:
            for process in reversed(processes):
                if process.poll() is None:
                    process.terminate()
                    process.wait(timeout=10)
            for handle in handles:
                handle.close()
            # Replace spec rather than merge so all temporary test authority and
            # shutdown fields disappear. This is the caller's disposable CR only.
            self.kube("patch", "trinoclusters.trino.kubedoop.dev", self.name, "--type=json", "-p",
                      json.dumps([{"op": "replace", "path": "/spec", "value": original}]))
            self.persist()
