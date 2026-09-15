"""Real Trino 476 -> Hive Metastore -> MinIO acceptance for the S3 domain.

Uses the delivered Trino CR/operator after platform CSI installation. Saves and
restores that CR's spec; owns only uniquely named experiment dependencies and
bucket bytes. Secret values are sent to kubectl stdin, never written to reports.
"""
import copy
import json
import pathlib
import uuid
import xml.etree.ElementTree as ET

from runtime import RuntimeVerifier, require


class S3Verifier(RuntimeVerifier):
    def __init__(self, run, namespace, name, hive_image, minio_image, mc_image):
        self.run, self.namespace, self.name = run, namespace, name
        self.hive_image, self.minio_image, self.mc_image = hive_image, minio_image, mc_image
        self.prefix = "s3-e02-" + uuid.uuid4().hex[:8]
        self.phases, self.owned = [], []
        self.report = {"passed": False, "cleanup": False, "phases": self.phases,
                       "scope": "Trino 476 native S3 writes/reads, inline inheritance, live S3Connection reference and SecretClass CSI"}
        self.original_spec = None
        self.persist()

    def persist(self):
        self.run.write("s3.json", self.report)

    def secretclass_receipt(self, group, class_name):
        pod = self.get("pod", group["pod"])
        require(pod and pod["metadata"]["uid"] == group["pod_uid"] and
                not pod["metadata"].get("deletionTimestamp"), "SecretClass observation changed Pod identity")
        volume = next((v for v in pod["spec"]["volumes"] if v["name"] == "s3-credentials"), {})
        template = volume.get("ephemeral", {}).get("volumeClaimTemplate", {})
        require(template.get("metadata", {}).get("annotations", {}).get("secrets.kubedoop.dev/class") == class_name and
                template.get("spec", {}).get("storageClassName") == "secrets.kubedoop.dev",
                "reference did not declare the expected SecretClass ephemeral volume")
        claim = self.get("pvc", group["pod"] + "-s3-credentials")
        require(claim and not claim["metadata"].get("deletionTimestamp") and
                claim.get("status", {}).get("phase") == "Bound", "SecretClass PVC is not bound")
        owners = [o for o in claim["metadata"].get("ownerReferences", []) if o.get("controller")]
        require(len(owners) == 1 and owners[0].get("kind") == "Pod" and owners[0].get("uid") == group["pod_uid"],
                "SecretClass PVC does not belong to the observed Pod")
        pv = self.get("pv", claim["spec"]["volumeName"])
        require(pv and not pv["metadata"].get("deletionTimestamp") and
                pv.get("status", {}).get("phase") == "Bound", "SecretClass PV is not bound")
        ref = pv["spec"].get("claimRef", {})
        require(all(ref.get(key) == claim["metadata"][key] for key in ("name", "namespace", "uid")),
                "SecretClass PVC/PV binding identity changed")
        driver = pv["spec"].get("csi", {}).get("driver")
        require(driver == "secrets.kubedoop.dev", "reference did not consume actual SecretClass CSI")
        return {"pod_uid": group["pod_uid"], "secret_class": class_name,
                "pvc_uid": claim["metadata"]["uid"], "pv_uid": pv["metadata"]["uid"], "driver": driver}

    def create(self, kind, name, body, api_version="v1", labels=None, cluster=False):
        require(self.get(kind, name) is None, "Refusing to overwrite pre-existing dependency " + kind + "/" + name)
        metadata = {"name": name}
        if not cluster:
            metadata["namespace"] = self.namespace
        if labels:
            metadata["labels"] = labels
        self.run.apply({"apiVersion": api_version, "kind": kind, "metadata": metadata, **body})
        self.owned.append((kind, name))

    def secret_env(self, variable, key):
        return {"name": variable, "valueFrom": {"secretKeyRef": {"name": self.prefix + "-credentials", "key": key}}}

    def dependency_pod(self, name, image, command, args, env=None, volumes=None, mounts=None, port=None, memory="512Mi"):
        container = {"name": "main", "image": image, "command": command, "args": args,
                     "resources": {"requests": {"cpu": "50m", "memory": "128Mi"}, "limits": {"cpu": "1", "memory": memory}}}
        if env:
            container["env"] = env
        if mounts:
            container["volumeMounts"] = mounts
        if port:
            container["readinessProbe"] = {"tcpSocket": {"port": port}, "periodSeconds": 2}
        self.create("Pod", name, {"spec": {"securityContext": {"fsGroup": 1000},
                    "containers": [container], "volumes": volumes or [], "restartPolicy": "Never"}}, labels={"fixture": name})
        self.until("dependency " + name, lambda: any(c.get("ready") for c in
                   self.get("pod", name).get("status", {}).get("containerStatuses", [])), timeout=240)

    def service(self, name, selected, port):
        self.create("Service", name, {"spec": {"selector": {"fixture": selected},
                    "ports": [{"name": "service", "port": port, "targetPort": port}]}})

    def mc(self, suffix, operation):
        name = self.prefix + "-" + suffix
        endpoint = "http://" + self.prefix + "-minio:9000"
        self.create("Pod", name, {"spec": {"restartPolicy": "Never", "containers": [{"name": "mc", "image": self.mc_image,
                    "command": ["/bin/sh", "-ec"], "args": [
                        'mc alias set target ' + endpoint + ' "$ACCESS_KEY" "$SECRET_KEY" >/dev/null\n' + operation],
                    "env": [self.secret_env("ACCESS_KEY", "ACCESS_KEY"), self.secret_env("SECRET_KEY", "SECRET_KEY")],
                    "resources": {"requests": {"cpu": "25m", "memory": "32Mi"}, "limits": {"cpu": "500m", "memory": "128Mi"}}}]}})
        def complete():
            pod = self.get("pod", name)
            phase = pod.get("status", {}).get("phase")
            require(phase != "Failed", "MinIO client failed: " + self.kube("logs", name, "-c", "mc") if phase == "Failed" else "")
            return phase == "Succeeded"
        self.until("MinIO client " + suffix, complete, timeout=90)
        return self.kube("logs", name, "-c", "mc")

    def dependencies(self):
        class_name = self.prefix + "-class"
        self.create("SecretClass", class_name, {"spec": {"backend": {"k8sSearch": {"searchNamespace": {"pod": {}}}}}},
                    api_version="secrets.kubedoop.dev/v1alpha1", cluster=True)
        self.create("Secret", self.prefix + "-credentials", {"stringData": {
                    "ACCESS_KEY": "e02" + uuid.uuid4().hex, "SECRET_KEY": uuid.uuid4().hex + uuid.uuid4().hex}},
                    labels={"secrets.kubedoop.dev/class": class_name})
        minio = self.prefix + "-minio"
        self.service(minio, minio, 9000)
        self.service(self.prefix + "-minio-alt", minio, 9000)
        self.dependency_pod(minio, self.minio_image, ["minio"], ["server", "/data", "--console-address", ":9001"],
                            env=[self.secret_env("MINIO_ROOT_USER", "ACCESS_KEY"), self.secret_env("MINIO_ROOT_PASSWORD", "SECRET_KEY")],
                            volumes=[{"name": "data", "emptyDir": {}}], mounts=[{"name": "data", "mountPath": "/data"}], port=9000)
        self.mc("bucket", "mc mb target/warehouse")
        properties = {
            "javax.jdo.option.ConnectionURL": "jdbc:derby:;databaseName=/tmp/metastore_db;create=true",
            "javax.jdo.option.ConnectionDriverName": "org.apache.derby.jdbc.EmbeddedDriver",
            "javax.jdo.option.ConnectionUserName": "APP", "javax.jdo.option.ConnectionPassword": "mine",
            "hive.metastore.warehouse.dir": "file:///tmp/warehouse", "hive.metastore.schema.verification": "true",
            "fs.s3.impl": "org.apache.hadoop.fs.s3a.S3AFileSystem", "fs.s3a.impl": "org.apache.hadoop.fs.s3a.S3AFileSystem",
            "fs.s3a.endpoint": "http://" + minio + ":9000", "fs.s3a.endpoint.region": "us-east-1",
            "fs.s3a.path.style.access": "true", "fs.s3a.connection.ssl.enabled": "false",
            "fs.s3a.access.key": "__ACCESS_KEY__", "fs.s3a.secret.key": "__SECRET_KEY__",
            "fs.s3a.aws.credentials.provider": "org.apache.hadoop.fs.s3a.SimpleAWSCredentialsProvider",
        }
        root = ET.Element("configuration")
        for key, value in properties.items():
            prop = ET.SubElement(root, "property")
            ET.SubElement(prop, "name").text = key
            ET.SubElement(prop, "value").text = value
        config = ET.tostring(root, encoding="unicode")
        self.create("ConfigMap", self.prefix + "-hive-config", {"data": {"hive-site.xml": config}})
        metastore = self.prefix + "-metastore"
        self.service(metastore, metastore, 9083)
        # Experiment keys contain only alphanumerics. The replacement happens
        # inside the container's writable config; generated CM has placeholders.
        script = ('mkdir -p /tmp/hive-conf\ncp /input/hive-site.xml /tmp/hive-conf/hive-site.xml\n'
                  'sed -i "s/__ACCESS_KEY__/$ACCESS_KEY/g;s/__SECRET_KEY__/$SECRET_KEY/g" /tmp/hive-conf/hive-site.xml\n'
                  'exec /kubedoop/hive-metastore/bin/start-metastore --config /tmp/hive-conf --db-type derby '
                  '--hive-bin-dir /kubedoop/hive-metastore/bin')
        self.dependency_pod(metastore, self.hive_image, ["/bin/bash", "-ec"], [script],
                            env=[self.secret_env("ACCESS_KEY", "ACCESS_KEY"), self.secret_env("SECRET_KEY", "SECRET_KEY"),
                                 {"name": "HADOOP_HEAPSIZE", "value": "256"}],
                            volumes=[{"name": "config", "configMap": {"name": self.prefix + "-hive-config"}}],
                            mounts=[{"name": "config", "mountPath": "/input", "readOnly": True}], port=9083, memory="768Mi")
        return class_name, minio, metastore

    def replace_spec(self, spec):
        self.kube("patch", "trinoclusters", self.name, "--type=json", "-p",
                  json.dumps([{"op": "replace", "path": "/spec", "value": spec}]))

    def query_ready(self, sql):
        return self.until("S3 query " + sql, lambda: self.query(sql), timeout=120)

    def observe_files(self, suffix):
        output = self.mc(suffix, "mc ls --json --recursive target/warehouse")
        files = [json.loads(line) for line in output.splitlines() if line.startswith("{")]
        require(any(record.get("size", 0) > 0 for record in files), "No nonempty S3 object was observed independently")
        return files

    def refreshed_write_read(self, previous_objects):
        # Hive 4 creates this fixture's tables as non-managed. A new CTAS proves
        # fresh S3 writes without enabling external-table modification globally.
        self.query("CREATE TABLE hive.e02.marker_refreshed AS SELECT BIGINT '84' AS value")
        selected = self.query_ready("SELECT value FROM hive.e02.marker UNION ALL "
                                    "SELECT value FROM hive.e02.marker_refreshed ORDER BY value")
        require(selected["rows"] == [[42], [84]], "refreshed reference did not read original and newly written S3 data")
        objects = self.observe_files("objects-refreshed")
        previous = {o["key"] for o in previous_objects if o.get("size", 0) > 0}
        current = {o["key"] for o in objects if o.get("size", 0) > 0}
        require(previous and previous.issubset(current), "original S3 objects disappeared after reference refresh")
        require(any(key.startswith("e02/marker_refreshed/") for key in current - previous),
                "refreshed CTAS did not produce a new independently observed S3 object")
        return selected, objects

    def verify(self):
        self.original_spec = copy.deepcopy(self.cr()["spec"])
        try:
            commons = pathlib.Path(self.run.args.commons).expanduser().resolve()
            self.run.kube("apply", "-f", commons / "config/crd/bases/s3.kubedoop.dev_s3connections.yaml")
            self.run.wait_crd_established("s3connections.s3.kubedoop.dev")
            class_name, minio, metastore = self.dependencies()
            baseline = self.wait_healthy()
            spec = copy.deepcopy(self.original_spec)
            for role in ("coordinators", "workers"):
                spec[role].setdefault("config", {})["hive"] = {"metastoreURI": "thrift://" + metastore + ":9083",
                    "s3": {"type": "inline", "inline": {"host": minio, "port": 9000, "region": "us-east-1",
                           "pathStyle": False, "credentials": {"secretName": self.prefix + "-credentials"}}}}
                spec[role]["roleGroups"]["default"].setdefault("config", {})["hive"] = {"s3": {"inline": {"pathStyle": True}}}
            self.replace_spec(spec)
            inline = self.wait_healthy(previous=baseline)
            self.wait_membership(inline)
            self.query("CREATE SCHEMA hive.e02 WITH (location = 's3://warehouse/e02/')")
            self.query("CREATE TABLE hive.e02.marker AS SELECT BIGINT '42' AS value")
            selected = self.query_ready("SELECT value FROM hive.e02.marker")
            require(selected["rows"] == [[42]], "Trino did not read its native S3 write")
            inline_objects = self.observe_files("objects-inline")
            self.phase("inline-inheritance-native-s3-write-read", {"resources": inline, "query": selected,
                       "objects": inline_objects})
            connection_name = self.prefix + "-connection"
            connection = {"host": minio, "port": 9000, "region": "us-east-1", "pathStyle": True,
                          "credentials": {"secretClass": class_name}}
            self.create("S3Connection", connection_name, {"spec": connection}, api_version="s3.kubedoop.dev/v1alpha1")
            for role in ("coordinators", "workers"):
                spec[role]["roleGroups"]["default"]["config"]["hive"]["s3"] = {"type": "reference", "reference": connection_name}
            self.replace_spec(spec)
            reference = self.wait_healthy(previous=inline)
            self.wait_membership(reference)
            selected = self.query_ready("SELECT value FROM hive.e02.marker")
            require(selected["rows"] == [[42]], "SecretClass-backed reference could not read the original object")
            csi = {role: self.secretclass_receipt(reference["groups"][role], class_name)
                   for role in ("coordinators", "workers")}
            self.phase("branch-switch-reference-secretclass-read", {"resources": reference, "query": selected,
                       "connection": self.get("s3connection", connection_name), "csi": csi})
            generation = self.cr()["metadata"]["generation"]
            self.kube("patch", "s3connection", connection_name, "--type=merge", "-p", json.dumps({"spec": {"host": self.prefix + "-minio-alt"}}))
            refreshed = self.wait_healthy(previous=reference)
            self.wait_membership(refreshed)
            selected, refreshed_objects = self.refreshed_write_read(inline_objects)
            cr, connection_object = self.cr(), self.get("s3connection", connection_name)
            require(cr["metadata"]["generation"] == generation, "S3 reference refresh edited the cluster CR")
            for group in cr["status"]["groups"]:
                observations = group.get("facts", {}).get("observed", [])
                require(any(o.get("kind") == "S3Connection" and o.get("uid") == connection_object["metadata"]["uid"] and
                            o.get("resourceVersion") == connection_object["metadata"]["resourceVersion"] for o in observations),
                        "S3 dependency refresh lacks actual object provenance")
            self.phase("reference-only-refresh-write-read", {"resources": refreshed, "query": selected,
                       "objects": refreshed_objects, "cr_generation": generation})
            self.report["passed"] = True
        finally:
            self.close()
        require(self.report["passed"] and self.report["cleanup"], "S3 acceptance or cleanup failed")

    def close(self):
        errors = []
        if self.original_spec is not None:
            try:
                self.kube("patch", "trinoclusters", self.name, "--type=json", "-p",
                          json.dumps([{"op": "replace", "path": "/spec", "value": self.original_spec}]))
                restored = self.wait_healthy()
                self.report["restored"] = restored
            except Exception as error:
                errors.append("restore original Trino: " + repr(error))
        for kind, name in reversed(self.owned):
            try:
                if kind == "Pod":
                    self.kube("logs", name, "--all-containers=true", check=False)
                self.kube("delete", kind, name, "--wait=true", "--timeout=90s", timeout=110)
                require(self.get(kind, name) is None, "dependency deletion not confirmed")
            except Exception as error:
                errors.append(kind + "/" + name + ": " + repr(error))
        self.report.update(cleanup=not errors, cleanup_errors=errors)
        if errors:
            self.report["passed"] = False
        self.persist()
