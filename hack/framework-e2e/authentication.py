"""Real PASSWORD authentication over verified TLS using the formal Trino operator."""
import base64
import copy
import hashlib
import json
import pathlib
import secrets
import time

from runtime import RuntimeVerifier, current_condition, named, require

CLIENT_USER = "framework-e02-user"

QUERY = r'''import base64,json,pathlib,ssl,sys,time,urllib.request,urllib.error
mode=sys.argv[1];root=pathlib.Path('/kubedoop/auth-client')
username=(root/'username').read_text();password=(root/'password').read_text()
headers={'X-Trino-User':username}
if mode!='anonymous':
 credential=username+':'+password+('incorrect' if mode=='wrong' else '')
 headers['Authorization']='Basic '+base64.b64encode(credential.encode()).decode()
context=ssl.create_default_context(cafile=str(root/'ca.crt'))
base='http://127.0.0.1:8080' if mode=='http' else 'https://127.0.0.1:8443'
request=urllib.request.Request(base+'/v1/statement',data=b'SELECT current_user, (SELECT count(*) FROM tpch.tiny.nation)',headers=headers)
rows=[];query=None;deadline=time.monotonic()+90
while True:
 try:
  with urllib.request.urlopen(request,context=context,timeout=10) as response:value=json.load(response)
 except urllib.error.HTTPError as error:
  print(json.dumps({'status':error.code,'mode':mode,'tls_verification':mode!='http'}));sys.exit(0)
 query=value.get('id',query)
 if 'error' in value:
  print(json.dumps({'query_id':query,'error':value['error'],'mode':mode}));sys.exit(0)
 rows.extend(value.get('data',[]))
 if not value.get('nextUri'):break
 if time.monotonic()>=deadline:raise TimeoutError('authenticated query timed out')
 request=urllib.request.Request(value['nextUri'],headers=headers);time.sleep(.2)
print(json.dumps({'query_id':query,'rows':rows,'mode':mode,'tls_verification':True}))
'''


def password_database(password):
    # Trino 476's native EncryptionUtil expects PBKDF2WithHmacSHA1 for this format.
    salt = secrets.token_bytes(32)
    iterations = 600000
    hashed = hashlib.pbkdf2_hmac("sha1", password.encode(), salt, iterations, dklen=32)
    return f"{CLIENT_USER}:{iterations}:{salt.hex()}:{hashed.hex()}\n"


class AuthenticationVerifier(RuntimeVerifier):
    def persist(self):
        self.run.write("authentication.json", self.report)

    def secret(self, name, data):
        self.run.apply({"apiVersion": "v1", "kind": "Secret", "metadata": {"name": name, "namespace": self.namespace},
                        "type": "Opaque", "data": {key: base64.b64encode(value.encode() if isinstance(value, str) else value).decode()
                                                   for key, value in data.items()}})

    def ready(self):
        require(current_condition(self.cr(), "WorkloadsReady"), "authenticated workloads not ready")
        result = {}
        for role in ("coordinators", "workers"):
            name = self.name + "-" + role + "-default-0"
            pod = self.get("pod", name)
            require(pod and not pod["metadata"].get("deletionTimestamp"), "Pod missing/terminating")
            main = named(pod.get("status", {}).get("containerStatuses", []), "trino")
            require(main and main.get("ready"), "Trino process is not ready")
            result[role] = {"uid": pod["metadata"]["uid"], "container_id": main["containerID"]}
            if role == "coordinators":
                init = named(pod.get("status", {}).get("initContainerStatuses", []), "initialize-tls")
                require(init and init.get("state", {}).get("terminated", {}).get("exitCode") == 0,
                        "TLS material assembly was not executed")
        return result

    def authenticated(self):
        self.ready()
        result = self.exec("coordinators", QUERY, "valid")
        self.report.setdefault("authentication_probes", []).append(result)
        self.persist()
        return result if result.get("rows") == [[CLIENT_USER, 25]] else None

    def verify(self):
        original = copy.deepcopy(self.cr()["spec"])
        class_name = self.run.cluster + "-auth"
        certificate = self.run.output / "e02-tls.crt"
        key = self.run.output / "e02-tls.key"
        created = []
        self.report["scope"] = "real platform AuthenticationClass, native PASSWORD provider, verified HTTPS SQL and credential Secret refresh"
        try:
            # Use the platform's actual CRD, not a look-alike test schema.
            crd = pathlib.Path(self.run.args.commons).expanduser() / "config/crd/bases/authentication.kubedoop.dev_authenticationclasses.yaml"
            self.run.kube("apply", "-f", crd)
            self.run.wait_crd_established("authenticationclasses.authentication.kubedoop.dev")
            self.run.command(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1", "-subj", "/CN=localhost",
                              "-addext", "subjectAltName=DNS:localhost,IP:127.0.0.1", "-keyout", key, "-out", certificate])
            cert = certificate.read_bytes()
            self.secret("e02-tls", {"tls.crt": cert, "tls.key": key.read_bytes()}); created.append("e02-tls")
            key.unlink()
            password = secrets.token_urlsafe(32)
            self.secret("e02-users", {"password.db": password_database(password)}); created.append("e02-users")
            self.secret("e02-internal", {"shared-secret": secrets.token_urlsafe(512)}); created.append("e02-internal")
            self.secret("e02-client", {"username": CLIENT_USER, "password": password, "ca.crt": cert}); created.append("e02-client")
            self.run.apply({"apiVersion": "authentication.kubedoop.dev/v1alpha1", "kind": "AuthenticationClass",
                            "metadata": {"name": class_name}, "spec": {"provider": {"static": {"userCredentialsSecret": {"name": "e02-users"}}}}})
            self.patch({"clusterConfig": {"authentication": [{"authenticationClass": class_name}], "tlsSecret": "e02-tls",
                                           "internalSecret": "e02-internal"},
                        "coordinators": {"podOverrides": {"spec": {
                            "containers": [{"name": "trino", "volumeMounts": [{"name": "auth-client", "mountPath": "/kubedoop/auth-client", "readOnly": True}]}],
                            "volumes": [{"name": "auth-client", "secret": {"secretName": "e02-client"}}]}}}})
            query = self.until("verified TLS and native PASSWORD query", self.authenticated, timeout=300)
            before = self.ready()
            self.phase("authenticated-native-sql", {"query": query, "pods": before,
                       "certificate_sha256": hashlib.sha256(cert).hexdigest(), "ca_verification": True})
            for mode in ("anonymous", "wrong", "http"):
                result = self.exec("coordinators", QUERY, mode)
                require(result.get("status") in (401, 403), "authentication/TLS prerequisite bypassed: " + repr(result))
                self.phase("rejected-" + mode, result)
            generation = self.cr()["metadata"]["generation"]
            password = secrets.token_urlsafe(32)
            self.secret("e02-users", {"password.db": password_database(password)})
            self.secret("e02-client", {"username": CLIENT_USER, "password": password, "ca.crt": cert})
            def refreshed():
                result = self.authenticated()
                current = self.ready()
                require(current["coordinators"]["uid"] != before["coordinators"]["uid"],
                        "native credential revision has not reached a new coordinator")
                return {"query": result, "pods": current} if result else None
            after = self.until("Secret credential refresh without CR edit", refreshed, timeout=300)
            require(self.cr()["metadata"]["generation"] == generation, "credential refresh required a CR edit")
            self.phase("credential-secret-refresh", {"generation_unchanged": generation, **after})
            self.report["passed"] = True
        finally:
            self.kube("patch", "trinoclusters.trino.kubedoop.dev", self.name, "--type=json", "-p",
                      json.dumps([{"op": "replace", "path": "/spec", "value": original}]))
            # Keep inputs present while the old authenticated Pods retire.
            self.until("restore unauthenticated fixture", lambda: self.healthy(), timeout=300)
            for name in created:
                self.kube("delete", "secret", name, "--ignore-not-found")
            self.run.kube("delete", "authenticationclass", class_name, "--ignore-not-found")
            if key.exists():
                key.unlink()
            self.persist()
