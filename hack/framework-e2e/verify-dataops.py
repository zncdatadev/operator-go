#!/usr/bin/env python3
"""Run E05 against disposable data through the real framework and data controllers.

Requires a disposable cluster, generated Trino and data CRDs, a Retain-capable
provisioner, and the same pinned Python-capable image used by the storage harness.
Creates its own namespace and refuses to touch any pre-existing namespace.
"""
import argparse
import hashlib
import json
import os
import pathlib
import signal
import subprocess
import time
import uuid

RESOURCE = 'trinoclusters.trino.kubedoop.dev'
ASSETS = 'dataassets.data.framework.kubedoop.dev'
OPERATIONS = 'dataoperations.data.framework.kubedoop.dev'


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def source(value):
    return {key: value[key] for key in ('version', 'crUID', 'role', 'group', 'slot', 'storageClass', 'capacity')}


def identity(value):
    return {'cluster': {key:value['cluster'][key] for key in ('apiVersion','kind','name','uid')}, 'claimName': value['claimName'],
            'binding': {key: value['binding'][key] for key in ('version', 'pvcUID', 'pvUID', 'volumeName')},
            'source': source(value['source'])}


def cluster(value):
    return {'apiVersion': value['apiVersion'], 'kind': value['kind'],
            'name': value['metadata']['name'], 'uid': value['metadata']['uid']}


class Verification:
    def __init__(self, args):
        self.args = args
        self.output = pathlib.Path(args.output_dir).resolve()
        self.output.mkdir(parents=True, exist_ok=True)
        require(not any(self.output.iterdir()), 'Use a new empty evidence directory')
        (self.output / 'commands').mkdir()
        self.index = 0
        self.processes = []
        self.created = False
        self.volumes = {}
        self.backends = {}
        self.report = {'passed': False, 'cleanup': False, 'phases': [], 'controllers': []}
        self.persist()

    def persist(self):
        (self.output / 'verification.json').write_text(json.dumps(self.report, indent=2) + '\n')

    def command(self, argv, input=None, timeout=40, check=True):
        self.index += 1
        started = time.monotonic()
        result = subprocess.run(list(map(str, argv)), input=input, capture_output=True, text=True, timeout=timeout, check=False)
        receipt = {'argv': list(map(str, argv)), 'returncode': result.returncode, 'stdout': result.stdout,
                   'stderr': result.stderr, 'elapsed_seconds': time.monotonic() - started}
        if input is not None:
            receipt['stdin'] = input
        (self.output / 'commands' / f'{self.index:05d}.json').write_text(json.dumps(receipt, indent=2) + '\n')
        if check:
            require(result.returncode == 0, 'command failed: ' + result.stderr[-2000:])
        return result.stdout

    def kube(self, *args, **kwargs):
        return self.command(['kubectl', '--kubeconfig', self.args.kubeconfig, '--request-timeout=25s',
                             '-n', self.args.namespace, *args], **kwargs)

    def get(self, kind, name):
        value = self.kube('get', kind, name, '--ignore-not-found', '-o', 'json')
        return json.loads(value) if value.strip() else None

    def apply(self, value):
        self.kube('apply', '-f', '-', input=json.dumps(value))

    def until(self, label, fn, seconds=180):
        end = time.monotonic() + seconds
        last = None
        while time.monotonic() < end:
            for process, _, _ in self.processes:
                require(process.poll() is None, 'controller exited')
            try:
                result = fn()
                if result:
                    return result
            except (RuntimeError, TypeError, KeyError) as error:
                last = str(error)
            time.sleep(1)
        raise RuntimeError(label + ' timed out; last=' + str(last))

    def phase(self, name, evidence):
        self.report['phases'].append({'name': name, 'evidence': evidence})
        self.persist()
        print('PASS dataops:', name, flush=True)

    def launch(self, binary, arguments, name):
        path = pathlib.Path(binary).resolve()
        stream = (self.output / (name + '.log')).open('wb')
        env = dict(os.environ, KUBECONFIG=self.args.kubeconfig)
        process = subprocess.Popen([str(path), *arguments], stdout=stream, stderr=subprocess.STDOUT,
                                   env=env, start_new_session=True)
        self.processes.append((process, stream, name))
        self.report['controllers'].append({'name': name, 'pid': process.pid,
                                           'binary_sha256': hashlib.sha256(path.read_bytes()).hexdigest()})
        self.persist()

    def create_cr(self, name, active=False, capacity='64Mi'):
        cr = {'apiVersion': 'trino.kubedoop.dev/v1alpha1', 'kind': 'TrinoCluster',
              'metadata': {'name': name, 'namespace': self.args.namespace},
              'spec': {'clusterConfig': {'reconciliationPaused': not active},
                       'workers': {'config': {'resources': {'storage': {'type': 'persistent',
                                    'storageClassName': self.args.storage_class, 'capacity': capacity}}},
                                   'roleGroups': {'default': {'replicas': 1}} if active else {}}}}
        self.apply(cr)
        return self.get(RESOURCE, name)

    def ready(self, name):
        pod = self.get('pod', name + '-workers-default-0')
        if pod and any(c['type'] == 'Ready' and c['status'] == 'True' for c in pod.get('status', {}).get('conditions', [])):
            return pod
        return None

    def retire(self, name):
        self.kube('patch', RESOURCE, name, '--type=merge', '-p', json.dumps({'spec': {'workers': {'roleGroups': {}}}}))
        # Merge patch retains keys in an object; explicitly remove the sole group.
        self.kube('patch', RESOURCE, name, '--type=json', '-p', json.dumps([{'op': 'remove', 'path': '/spec/workers/roleGroups/default'}]))
        self.until('retire ' + name, lambda: not self.get('statefulset', name + '-workers-default')
                   and not self.get('pod', name + '-workers-default-0'))
        self.kube('patch', RESOURCE, name, '--type=merge', '-p', json.dumps({'spec': {'clusterConfig': {'reconciliationPaused': True}}}))
        return self.get(RESOURCE, name)

    def operation(self, name, action, asset, from_identity, source_cr, target_cr=None, capacity=None):
        spec = {'action': action, 'assetName': asset['metadata']['name'], 'assetUID': asset['metadata']['uid'],
                'source': identity(from_identity), 'sourceCluster': cluster(source_cr), 'workerIdentity': {'uid':1000, 'gid':1000}}
        if target_cr:
            next_source = source(from_identity['source'])
            next_source['crUID'] = target_cr['metadata']['uid']
            if capacity:
                next_source['capacity'] = capacity
            spec['target'] = {'cluster': cluster(target_cr),
                              'claimName': 'data-' + target_cr['metadata']['name'] + '-workers-default-0', 'source': next_source}
        spec['approval'] = ''
        spec['approval'] = hashlib.sha256(json.dumps(spec, separators=(',', ':'), ensure_ascii=False).encode()).hexdigest()
        self.apply({'apiVersion': 'data.framework.kubedoop.dev/v1alpha1', 'kind': 'DataOperation',
                    'metadata': {'name': name, 'namespace': self.args.namespace}, 'spec': spec})
        def completed():
            op = self.get(OPERATIONS, name)
            if op.get('status', {}).get('phase') == 'Complete':
                return op
            self.report['pending'] = op.get('status', {})
            self.persist()
            return None
        result = self.until(action, completed, 300)
        self.phase(action + '-' + name, result)
        return self.get(ASSETS, asset['metadata']['name'])

    def remember(self, data):
        name = data['binding']['volumeName']
        self.volumes[name] = data['binding']['pvUID']
        pv = self.get('pv', name)
        path = pv['spec'].get('hostPath', pv['spec'].get('local', {})).get('path')
        require(path and path.startswith('/'), 'Runtime fixture requires observable local-path backend storage')
        self.backends[name] = {'path': path, 'pvUID': pv['metadata']['uid'], 'node': self.args.node_name}
        self.report['backends'] = self.backends
        self.persist()

    def backend_removed(self, data):
        backend = self.backends[data['binding']['volumeName']]
        require(backend['pvUID'] == data['binding']['pvUID'], 'backend proof identity changed')
        self.command(['docker', 'exec', backend['node'], 'test', '!', '-e', backend['path']])
        self.phase('backend-path-reclaimed', backend)

    def marker(self, name):
        return self.kube('exec', name + '-workers-default-0', '-c', 'trino', '--',
                         'python3', '-c', "from pathlib import Path; print(Path('/data/marker').read_text())").strip()

    def run(self):
        require(self.get('namespace', self.args.namespace) is None, 'Refusing an existing namespace')
        self.apply({'apiVersion': 'v1', 'kind': 'Namespace', 'metadata': {'name': self.args.namespace}})
        self.created = True
        self.launch(self.args.storage_controller_binary, ['--kubeconfig', self.args.kubeconfig, '--namespace', self.args.namespace,
                    '--image', self.args.image, '--storage-class', self.args.storage_class], 'storage-controller')
        self.launch(self.args.data_controller_binary, ['--worker-image', self.args.image], 'data-controller')
        old = self.create_cr('source', active=True)
        pod = self.until('source ready', lambda: self.ready('source'))
        require(pod['spec']['nodeName'] == self.args.node_name, 'Fixture scheduled outside dedicated kind node')
        claim_name = 'data-source-workers-default-0'
        claim = self.until('automatic data asset', lambda: (p if (p := self.get('pvc', claim_name)) and
                           p['metadata'].get('annotations', {}).get('framework.kubedoop.dev/data-asset') else None))
        asset = self.get(ASSETS, claim['metadata']['annotations']['framework.kubedoop.dev/data-asset'])
        self.remember(asset['spec'])
        marker = 'data-operation-' + str(uuid.uuid4())
        self.kube('exec', 'source-workers-default-0', '-c', 'trino', '--', 'python3', '-c',
                  "import os; f=open('/data/marker','x'); f.write(" + repr(marker) + "); f.flush(); os.fsync(f.fileno()); f.close()")
        self.phase('framework-automatic-identity', asset)
        self.kube('delete', RESOURCE, 'source', '--wait=true')
        self.until('source CR and workloads gone', lambda: not self.get('pod', 'source-workers-default-0')
                   and not self.get('statefulset', 'source-workers-default'))
        adopted = self.create_cr('adopted')
        asset = self.operation('adopt', 'adopt', asset, asset['spec'], old, adopted)
        adopted_data = asset['status']['current']
        require(adopted_data['binding']['pvUID'] == asset['spec']['binding']['pvUID'], 'adoption changed physical volume')
        self.create_cr('adopted', active=True)
        self.until('adopted framework consumption', lambda: self.ready('adopted'))
        require(self.marker('adopted') == marker, 'adopted marker differs')
        self.phase('adopted-framework-consumed-original-bytes', adopted_data)
        adopted = self.retire('adopted')
        migrated = self.create_cr('migrated', capacity='128Mi')
        asset = self.operation('migrate', 'migrate', asset, adopted_data, adopted, migrated, '128Mi')
        migrated_data = asset['status']['current']
        self.remember(migrated_data)
        require(migrated_data['binding']['pvUID'] != adopted_data['binding']['pvUID'], 'migration reused source volume')
        self.create_cr('migrated', active=True, capacity='128Mi')
        self.until('migrated framework consumption', lambda: self.ready('migrated'))
        require(self.marker('migrated') == marker, 'migrated marker differs')
        self.phase('migrated-framework-consumed-verified-copy', migrated_data)
        migrated = self.retire('migrated')
        asset = self.operation('destroy-current', 'destroy', asset, migrated_data, migrated)
        require(not self.get('pvc', migrated_data['claimName']) and not self.get('pv', migrated_data['binding']['volumeName']),
                'destroyed current binding remains')
        self.backend_removed(migrated_data)
        asset = self.operation('destroy-retired-copy', 'destroy', asset, adopted_data, adopted)
        self.backend_removed(adopted_data)
        require(asset['status']['destroyed'] and not asset['status'].get('retiredCopies'), 'asset still has live copies')
        require(len(asset['status']['history']) == 4, 'missing independent operation history')
        self.phase('all-authorized-copies-destroyed-history-retained', asset)
        self.report['passed'] = True
        self.persist()

    def cleanup(self):
        for process, stream, name in reversed(self.processes):
            if process.poll() is None:
                os.killpg(process.pid, signal.SIGTERM)
            code = process.wait(timeout=20)
            stream.close()
            self.report['controllers'].append({'name': name, 'exit_code': code, 'reaped': True})
        self.processes = []
        if self.created:
            inventory = json.loads(self.kube('get', 'pv', '-o', 'json'))
            for pv in inventory['items']:
                if pv.get('spec', {}).get('claimRef', {}).get('namespace') == self.args.namespace:
                    self.volumes[pv['metadata']['name']] = pv['metadata']['uid']
            self.kube('delete', 'namespace', self.args.namespace, '--wait=true', '--timeout=90s', timeout=100)
            # Only exact PV identities first observed in our newly created namespace.
            for name, uid in self.volumes.items():
                pv = self.get('pv', name)
                if pv:
                    require(pv['metadata']['uid'] == uid and pv['spec']['claimRef']['namespace'] == self.args.namespace,
                            'cleanup PV identity or source namespace changed')
                    self.kube('delete', 'pv', name, '--wait=true', '--timeout=30s')
        self.report['cleanup'] = True
        self.persist()


def main():
    parser = argparse.ArgumentParser()
    for name in ('kubeconfig', 'storage-controller-binary', 'data-controller-binary', 'image', 'storage-class', 'output-dir', 'node-name'):
        parser.add_argument('--' + name, required=True)
    parser.add_argument('--namespace', default='framework-data-e2e-' + uuid.uuid4().hex[:8])
    args = parser.parse_args()
    require(args.namespace.startswith('framework-data-e2e-'), 'Use the reserved disposable namespace prefix')
    verification = Verification(args)
    try:
        verification.run()
    except BaseException as error:
        verification.report['error'] = repr(error)
        verification.persist()
        raise
    finally:
        verification.cleanup()


if __name__ == '__main__':
    main()
