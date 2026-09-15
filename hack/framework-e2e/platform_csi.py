"""Build unchanged platform operators and verify their actual CSI consumption."""
import copy
import hashlib
import json
import pathlib
import time

from runtime import RuntimeVerifier, current_condition, named, require

BASE = 'registry.access.redhat.com/ubi9/ubi-minimal@sha256:2f06ae0e6d3d9c4f610d32c480338eef474867f435d8d28625f2985e8acde6e8'
SIDECARS = (
    'quay.io/zncdatadev/sig-storage/csi-node-driver-registrar:v2.12.0',
    'quay.io/zncdatadev/sig-storage/csi-provisioner:v5.1.0',
    'quay.io/zncdatadev/sig-storage/livenessprobe:v2.14.0',
)


def resources(stream):
    decoder, result = json.JSONDecoder(), []
    while stream.strip():
        value, end = decoder.raw_decode(stream.lstrip())
        result.extend(value['items'] if value.get('kind') == 'List' else [value])
        stream = stream.lstrip()[end:]
    return result


class PlatformInstaller:
    def __init__(self, run):
        self.run = run
        self.image = 'operator-go/platform:' + run.cluster
        self.root = pathlib.Path(run.args.commons).expanduser().resolve().parent
        self.output = run.output / 'platform-build'
        self.output.mkdir(parents=True, exist_ok=True)
        self.sidecars = {}
        self.manifests = {}
        self.report = {'sources': {}, 'images': {}, 'manifest_adjustments': [
            'replace manager and CSI executables with unchanged local source builds',
            'pin CSI sidecars to observed immutable registry digests',
            'disable metrics endpoints; use fsGroup 65532 for writable CSI controller socket directory',
            'supply secret manager Deployment commented out by upstream config/default',
            'give node CSI processes bounded 256Mi memory and 500m CPU during mount/key generation',
        ]}

    def build(self):
        self.run.phase('build-real-secret-and-listener-operators')
        for product in ('secret', 'listener'):
            repo = self.root / (product + '-operator')
            tracked = self.run.command(['git', 'diff', 'HEAD', '--name-only'], cwd=repo).strip()
            untracked = self.run.command(['git', 'ls-files', '--others', '--exclude-standard', '--', '.', ':(exclude).worktree/**'], cwd=repo).strip()
            require(not tracked and not untracked, 'Platform source must be clean: ' + str(repo))
            source_hashes = {str(path.relative_to(repo)): hashlib.sha256(path.read_bytes()).hexdigest()
                             for folder in ('api', 'internal', 'pkg', 'cmd', 'config')
                             for path in (repo / folder).rglob('*') if path.is_file() and path.suffix in ('.go', '.yaml')}
            for name in ('go.mod', 'go.sum'):
                source_hashes[name] = hashlib.sha256((repo / name).read_bytes()).hexdigest()
            self.report['sources'][product] = {'commit': self.run.command(['git', 'rev-parse', 'HEAD'], cwd=repo).strip(),
                                                'files': source_hashes}
            for entry, binary in (('./cmd/main.go', product + '-manager'), ('./cmd/csiplugin', product + '-csi')):
                self.run.command(['env', 'CGO_ENABLED=0', 'GOOS=linux', 'GOARCH=arm64', 'go', 'build', '-mod=readonly',
                                  '-trimpath', '-buildvcs=false', '-o', self.output / binary, entry], cwd=repo, timeout=600)
                self.report.setdefault('binaries', {})[binary] = hashlib.sha256((self.output / binary).read_bytes()).hexdigest()
            self.manifests[product] = self.run.command([repo / 'bin/kustomize', 'build', repo / 'config/default'], cwd=repo)
            (self.output / (product + '-upstream.yaml')).write_text(self.manifests[product])
        dockerfile = self.output / 'Dockerfile'
        dockerfile.write_text('FROM ' + BASE + '\nUSER 0\nRUN microdnf install -y util-linux openssl krb5-workstation cyrus-sasl && microdnf clean all\n'
                              'COPY secret-manager secret-csi listener-manager listener-csi /\nUSER 65532:65532\n')
        self.run.images.append(self.image)
        self.run.command(['docker', 'build', '--platform=linux/arm64', '-t', self.image, self.output], timeout=900)
        self.report['images']['platform'] = json.loads(self.run.command(['docker', 'image', 'inspect', self.image]))
        for original in SIDECARS:
            self.run.command(['docker', 'image', 'inspect', original], check=False)
            if self.run.last_command['returncode'] != 0:
                self.pull_image(original)
            observed = json.loads(self.run.command(['docker', 'image', 'inspect', original]))[0]
            require(observed['Architecture'] == 'arm64' and observed.get('RepoDigests'), 'CSI sidecar lacks pinned arm64 identity')
            digest = next((d for d in observed['RepoDigests'] if d.split('@')[0] == original.rsplit(':', 1)[0]), None)
            require(digest, 'CSI sidecar digest does not belong to requested registry repository')
            self.sidecars[original] = digest
            self.report['images'][original] = {'digest': digest, 'id': observed['Id']}
        self.run.write('platform-build.json', self.report)

    def pull_image(self, image):
        # Only the idempotent registry download is retried. Each command keeps its
        # ordinary stdout/stderr receipt, including timeouts and failed attempts.
        for attempt in range(1, 4):
            self.run.command(['docker', 'pull', '--platform=linux/arm64', image], timeout=600, check=False)
            receipt = self.run.last_command
            self.report.setdefault('image_pull_attempts', []).append({
                'image': image, 'attempt': attempt, 'command': f'{self.run.command_index:05d}',
                'returncode': receipt['returncode'], 'timed_out': receipt.get('timed_out', False),
            })
            self.run.write('platform-build.json', self.report)
            if receipt['returncode'] == 0:
                return
            if attempt < 3:
                time.sleep(2 ** attempt)
        require(False, 'Image pull exhausted 3 attempts: ' + image + ': ' + receipt['stderr'][-3000:])

    def load(self):
        transports = []
        for index, digest in enumerate(self.sidecars.values()):
            tag = 'operator-go/platform-sidecar:' + self.run.cluster + '-' + str(index)
            self.run.command(['docker', 'tag', digest, tag])
            self.run.images.append(tag)
            transports.append(tag)
        self.run.load_images([self.image, *transports])
        for digest, tag in zip(self.sidecars.values(), transports):
            self.run.ensure_image_digest(digest, tag)

    def install(self):
        self.run.phase('install-real-secret-listener-CSI')
        for product, stream in self.manifests.items():
            rendered = resources(self.run.kube('create', '--dry-run=client', '-f', '-', '-o', 'json', input=stream))
            manager_found = False
            for value in rendered:
                if value['kind'] in ('Deployment', 'DaemonSet'):
                    pod = value['spec']['template']['spec']
                    pod.setdefault('securityContext', {})['fsGroup'] = 65532
                    for container in pod['containers']:
                        name = container['name']
                        if name in ('manager', 'csi-controller', 'csi-node'):
                            container['image'] = self.image
                            container['command'] = ['/' + product + ('-manager' if name == 'manager' else '-csi')]
                            container['args'] = [arg for arg in container.get('args', []) if not arg.startswith('--metrics-bind-address')]
                            container['args'].append('--metrics-bind-address=0')
                            container.setdefault('resources', {})['limits'] = {'memory': '256Mi', 'cpu': '500m'}
                            manager_found = manager_found or name == 'manager'
                        else:
                            require(container['image'] in self.sidecars, 'Unpinned platform sidecar: ' + container['image'])
                            container['image'] = self.sidecars[container['image']]
                self.run.apply(value)
            if not manager_found:
                rendered.append(self.manager(product))
                self.run.apply(rendered[-1])
            self.run.write('platform-' + product + '-deployment.json', rendered)
            namespace = product + '-operator-system'
            for kind, suffix in (('deployment', 'controller-manager'), ('deployment', 'csi-controller'), ('daemonset', 'csi-node')):
                self.run.kube('-n', namespace, 'rollout', 'status', kind + '/' + product + '-operator-' + suffix,
                              '--timeout=180s', timeout=200)
            self.run.wait_crd_established(product + 'classes.' + product + 's.kubedoop.dev')
        node = json.loads(self.run.kube('get', 'csinode', self.run.cluster + '-control-plane', '-o', 'json'))
        drivers = {d['name'] for d in node['spec']['drivers']}
        require({'secrets.kubedoop.dev', 'listeners.kubedoop.dev'} <= drivers, 'Actual kubelet CSI registration missing')
        self.run.write('platform-csi-node.json', node)

    def manager(self, product):
        name = product + '-operator-controller-manager'
        labels = {'app.kubernetes.io/name': product + '-operator', 'control-plane': 'controller-manager'}
        return {'apiVersion': 'apps/v1', 'kind': 'Deployment', 'metadata': {'name': name, 'namespace': product + '-operator-system'},
                'spec': {'replicas': 1, 'selector': {'matchLabels': labels}, 'template': {'metadata': {'labels': labels}, 'spec': {
                    'serviceAccountName': name, 'securityContext': {'runAsNonRoot': True, 'runAsUser': 65532},
                    'containers': [{'name': 'manager', 'image': self.image, 'command': ['/' + product + '-manager'],
                                    'args': ['--health-probe-bind-address=:8081', '--metrics-bind-address=0'],
                                    'resources': {'requests': {'cpu': '10m', 'memory': '32Mi'}, 'limits': {'cpu': '500m', 'memory': '256Mi'}},
                                    'readinessProbe': {'httpGet': {'path': '/readyz', 'port': 8081}}}]}}}}


TLS_QUERY = r'''import hashlib,json,pathlib,ssl,sys,urllib.request,time
root=pathlib.Path('/kubedoop/platform-tls')
listener=pathlib.Path('/kubedoop/listener/default-address')
host=(listener/'address').read_text().strip();port=(listener/'ports'/'https').read_text().strip()
endpoint='https://'+('['+host+']' if ':' in host else host)+':'+port
context=ssl.create_default_context(cafile=str(root/'ca.crt'))
request=urllib.request.Request(endpoint+'/v1/statement',data=b'SELECT count(*) FROM tpch.tiny.nation',headers={'X-Trino-User':'platform-e02'})
rows=[];query=None;deadline=time.monotonic()+90
while True:
 with urllib.request.urlopen(request,context=context,timeout=15) as response: result=json.load(response)
 query=result.get('id',query)
 if 'error' in result: raise RuntimeError(json.dumps(result['error']))
 rows.extend(result.get('data',[]))
 if not result.get('nextUri'): break
 if time.monotonic()>deadline: raise TimeoutError(query)
 request=urllib.request.Request(result['nextUri'],headers={'X-Trino-User':'platform-e02'});time.sleep(.2)
print(json.dumps({'rows':rows,'query_id':query,'tls_verified':True,'ca_sha256':hashlib.sha256((root/'ca.crt').read_bytes()).hexdigest(),
 'certificate_sha256':hashlib.sha256((root/'tls.crt').read_bytes()).hexdigest(),
 'listener_address':host,'listener_https':port,'verified_listener_uri':endpoint}))
'''


class PlatformVerifier(RuntimeVerifier):
    def persist(self):
        self.run.write('platform-runtime.json', self.report)

    def group(self):
        return next(g for g in self.cr().get('status', {}).get('groups', []) if g['role'] == 'coordinators' and g['name'] == 'default')

    def producer_pending(self):
        cr = self.cr()
        group = self.group()
        pod = self.get('pod', self.name + '-coordinators-default-0')
        platform = group.get('platform', {})
        return {'generation': cr['metadata']['generation'], 'pod_uid': pod['metadata']['uid'], 'platform': platform} if (
            pod and platform.get('phase') == 'Observing' and platform.get('diagnostic', {}).get('state') == 'pending') else None

    def verified(self):
        self.healthy()
        group = self.group()
        platform = group.get('platform', {})
        require(platform.get('diagnostic', {}).get('state') == 'resolved', 'post-creation platform facts unresolved')
        result = self.exec('coordinators', TLS_QUERY)
        require(result.get('rows') == [[25]] and result.get('tls_verified'), 'TLS native SQL failed')
        discovery = self.get('configmap', self.name + '-discovery')
        expected = result['verified_listener_uri']
        require(discovery['data']['TRINO_URI'] == expected, 'shared discovery did not consume actual CSI listener endpoint')
        addresses = platform.get('listeners', [])
        require(any(a['address'] == result['listener_address'] and a['ports']['https'] == int(result['listener_https']) for a in addresses),
                'status did not observe actual mounted listener endpoint')
        claims = []
        for directory, driver in (('listener', 'listeners.kubedoop.dev'), ('tls-source', 'secrets.kubedoop.dev')):
            claim = self.get('pvc', self.name + '-coordinators-default-0-' + directory)
            pv = self.get('pv', claim['spec']['volumeName'])
            require(pv['spec']['csi']['driver'] == driver, 'directory did not use real platform CSI')
            claims.append({'directory': directory, 'pvc_uid': claim['metadata']['uid'], 'pv_uid': pv['metadata']['uid'], 'driver': driver})
        return {'query': result, 'platform': platform, 'claims': claims, 'discovery': discovery['data']}

    def verify(self):
        original = copy.deepcopy(self.cr()['spec'])
        class_name = self.run.cluster + '-platform'
        self.report['scope'] = 'real platform CSI producer creation, mounted AutoTLS credentials, verified HTTPS SQL and observed Listener discovery'
        try:
            self.run.apply({'apiVersion': 'listeners.kubedoop.dev/v1alpha1', 'kind': 'ListenerClass',
                            'metadata': {'name': class_name}, 'spec': {'serviceType': 'ClusterIP'}})
            self.run.apply({'apiVersion': 'secrets.kubedoop.dev/v1alpha1', 'kind': 'SecretClass', 'metadata': {'name': class_name},
                            'spec': {'backend': {'autoTls': {'ca': {'secret': {'name': 'e02-platform-ca', 'namespace': self.namespace},
                                                                  'autoGenerate': True, 'keyGeneration': {'rsa': {'length': 2048}}}}}}})
            self.patch({'clusterConfig': {'listenerClass': class_name, 'tlsSecretClass': class_name},
                        'coordinators': {'podOverrides': {'spec': {'containers': [{'name': 'trino', 'volumeMounts': [
                            {'name': 'tls-source', 'mountPath': '/kubedoop/platform-tls', 'readOnly': True}]}]}}}})
            pending = self.until('Pod produces post-creation CSI result', self.producer_pending, timeout=120)
            self.phase('producer-created-before-platform-ready', pending)
            result = self.until('real CSI Listener and verified AutoTLS SQL', self.verified, timeout=360)
            self.phase('real-listener-autotls-consumed', result)
            self.report['passed'] = True
        finally:
            self.kube('patch', 'trinoclusters.trino.kubedoop.dev', self.name, '--type=json', '-p',
                      json.dumps([{'op': 'replace', 'path': '/spec', 'value': original}]))
            self.until('restore platform-free fixture', lambda: self.healthy(), timeout=300)
            self.run.kube('delete', 'listenerclass', class_name, '--ignore-not-found')
            self.run.kube('delete', 'secretclass', class_name, '--ignore-not-found')
            self.kube('delete', 'secret', 'e02-platform-ca', '--ignore-not-found')
            self.persist()
