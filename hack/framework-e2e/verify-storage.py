#!/usr/bin/env python3
"""Observe real Retain/Retain volumes in one explicitly isolated namespace."""
import argparse
import base64
import copy
import datetime
import decimal
import hashlib
import json
import os
import pathlib
import re
import signal
import subprocess
import sys
import time
import uuid

SOURCE = 'framework.kubedoop.dev/retained-data'
BINDING = 'framework.kubedoop.dev/retained-binding'
FINALIZER = 'storage-experiment.design.kubedoop.dev/hold'
RESOURCE = 'trinoclusters.trino.kubedoop.dev'


class VerificationError(RuntimeError):
    pass


def require(value, message):
    if not value:
        raise VerificationError(message)


def now():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def sha(data):
    return hashlib.sha256(data).hexdigest()


def capacity_bytes(value):
    match = re.fullmatch(r'([0-9]+(?:\.[0-9]+)?)([KMGTPE]i|[kMGTPE])?', value)
    require(match, 'unsupported capacity representation in experiment: ' + repr(value))
    suffix = match[2] or ''
    powers = {'': 1, **{unit + 'i': 1024 ** index for index, unit in enumerate('KMGTPE', 1)},
              **{unit: 1000 ** index for index, unit in enumerate('kMGTPE', 1)}}
    return decimal.Decimal(match[1]) * powers[suffix]


def identity(pod):
    main = next((c for c in pod.get('status', {}).get('containerStatuses', []) if c['name'] == 'trino'), {})
    return {'pod_name': pod['metadata']['name'], 'pod_uid': pod['metadata']['uid'],
            'container_id': main.get('containerID'), 'restart_count': main.get('restartCount'),
            'image_id': main.get('imageID'), 'pod_ip': pod.get('status', {}).get('podIP')}


def json_stream(text):
    decoder, values, position = json.JSONDecoder(), [], 0
    while position < len(text):
        while position < len(text) and text[position].isspace():
            position += 1
        if position == len(text):
            return values, False
        try:
            value, position = decoder.raw_decode(text, position)
            values.append(value)
        except json.JSONDecodeError:
            return values, True
    return values, False


def validate_mount(pod, pvc_name):
    main = next((c for c in pod['spec']['containers'] if c['name'] == 'trino'), None)
    require(main is not None, 'main container is missing')
    mounted = [m for m in main.get('volumeMounts', []) if m.get('mountPath') == '/data']
    require(len(mounted) == 1 and not mounted[0].get('subPath') and not mounted[0].get('subPathExpr'), 'main data mount changed')
    volume = next((v for v in pod['spec']['volumes'] if v['name'] == mounted[0]['name']), {})
    require(volume.get('persistentVolumeClaim', {}).get('claimName') == pvc_name, 'main /data does not mount the observed retained PVC')


def source_receipt(cr, storage_class, capacity):
    return {'version': 1, 'crUID': cr['metadata']['uid'], 'role': 'workers', 'group': 'default',
            'slot': 'data', 'storageClass': storage_class, 'capacity': capacity}


def inspect_binding(cr, pvc, pv, storage_class, capacity, expected=None):
    require(pvc and pv, 'PVC and PV must both exist')
    meta, spec = pvc['metadata'], pvc['spec']
    require(not meta.get('deletionTimestamp') and not pv['metadata'].get('deletionTimestamp'), 'data objects are deleting')
    require(not meta.get('ownerReferences') and not pv['metadata'].get('ownerReferences'), 'data object has an owner reference')
    annotations = meta.get('annotations', {})
    source = json.loads(annotations.get(SOURCE, 'null'))
    require(source == source_receipt(cr, storage_class, capacity), 'retained provenance differs from the original CR/slot/spec')
    require(meta['name'] == 'data-' + cr['metadata']['name'] + '-workers-default-0', 'PVC name has a different slot or ordinal')
    require(meta['namespace'] == cr['metadata']['namespace'], 'PVC namespace differs')
    require(spec.get('storageClassName') == storage_class and spec.get('accessModes') == ['ReadWriteOnce'] and
            spec.get('volumeMode', 'Filesystem') == 'Filesystem', 'PVC shape differs')
    require(capacity_bytes(spec['resources']['requests']['storage']) == capacity_bytes(capacity), 'PVC requested capacity differs')
    require(pvc.get('status', {}).get('phase') == 'Bound' and pv.get('status', {}).get('phase') == 'Bound', 'binding is not Bound')
    require(spec.get('volumeName') == pv['metadata']['name'], 'PVC points to another PV')
    pvs = pv['spec']
    require(pvs.get('persistentVolumeReclaimPolicy') == 'Retain' and pvs.get('storageClassName') == storage_class,
            'PV class/reclaim policy differs')
    require(pvs.get('volumeMode', 'Filesystem') == 'Filesystem' and pvs.get('accessModes') == ['ReadWriteOnce'], 'PV mode differs')
    require(capacity_bytes(pvs['capacity']['storage']) >= capacity_bytes(capacity), 'PV capacity is too small')
    claim = pvs.get('claimRef', {})
    require(all(claim.get(k) == meta[k] for k in ('name', 'namespace', 'uid')), 'PV claimRef differs from the actual PVC UID')
    binding = json.loads(annotations.get(BINDING, 'null'))
    require(binding == {'version': 1, 'pvcUID': meta['uid'], 'pvUID': pv['metadata']['uid'], 'volumeName': pv['metadata']['name']},
            'retained binding receipt differs from live PVC/PV identities')
    result = {'cr_uid': cr['metadata']['uid'], 'pvc_name': meta['name'], 'pvc_uid': meta['uid'],
              'pv_name': pv['metadata']['name'], 'pv_uid': pv['metadata']['uid'],
              'source': source, 'binding': binding, 'pv_source': copy.deepcopy({k: pvs[k] for k in ('hostPath', 'local', 'csi', 'nodeAffinity') if k in pvs})}
    if expected:
        require(result == expected, 'the retained data identity, source or binding changed')
    return result


def fixture(name, namespace, present=True):
    group = {'replicas': 1, 'config': {'logging': {'enableVectorAgent': False}},
             'podOverrides': {'spec': {'terminationGracePeriodSeconds': 10, 'containers': [{
                 'name': 'trino', 'readinessProbe': {'httpGet': {'path': '/healthz', 'port': 8080},
                 'periodSeconds': 1, 'failureThreshold': 30}}]}}}
    return {'apiVersion': 'trino.kubedoop.dev/v1alpha1', 'kind': 'TrinoCluster',
            'metadata': {'name': name, 'namespace': namespace},
            'spec': {'workers': {'roleGroups': {'default': group} if present else {}}}}


WRITE_MARKER = r'''import base64,hashlib,json,os,pathlib,sys
path=pathlib.Path(sys.argv[1]); data=sys.stdin.buffer.read()
fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
with os.fdopen(fd,'wb') as stream:
 stream.write(data); stream.flush(); os.fsync(stream.fileno())
directory=os.open(path.parent,os.O_RDONLY|os.O_DIRECTORY)
try: os.fsync(directory)
finally: os.close(directory)
actual=path.read_bytes()
assert actual==data
print(json.dumps({'bytes':len(actual),'sha256':hashlib.sha256(actual).hexdigest(),'base64':base64.b64encode(actual).decode(),'uid':os.getuid(),'gid':os.getgid(),'fsync_file':True,'fsync_parent_directory':True}))
'''

READ_MARKER = r'''import base64,hashlib,json,os,pathlib,sys
data=pathlib.Path(sys.argv[1]).read_bytes()
print(json.dumps({'bytes':len(data),'sha256':hashlib.sha256(data).hexdigest(),'base64':base64.b64encode(data).decode(),'uid':os.getuid(),'gid':os.getgid()}))
'''

HTTP_MARKER = r'''import base64,hashlib,json,urllib.request
with urllib.request.urlopen('http://127.0.0.1:8080/marker',timeout=3) as response:
 data=response.read()
 print(json.dumps({'status':response.status,'bytes':len(data),'sha256':hashlib.sha256(data).hexdigest(),'base64':base64.b64encode(data).decode(),'content_length':response.headers.get('Content-Length')}))
'''


class Verifier:
    def __init__(self, args):
        self.args = args
        self.output = pathlib.Path(args.output_dir)
        self.output.mkdir(parents=True, exist_ok=True)
        require(not any(self.output.iterdir()), 'verifier output directory must be empty')
        (self.output / 'commands').mkdir()
        self.deadline = time.monotonic() + args.timeout
        self.command_index = 0
        self.controller = None
        self.controller_stream = None
        self.storage_watch = None
        self.watch_streams = []
        self.checked_images = set()
        self.capacity = args.capacity
        self.closing = False
        self.inspectors = []
        self.report = {'scope': 'one-slot Retain/Retain and same-source reuse with a synthetic Python marker process; no Trino SQL',
                       'started_at': now(), 'passed': False, 'phases': [], 'controllers': [], 'mutations': [],
                       'snapshots': [], 'marker_observations': [], 'negative_cases': [],
                       'image_observations': [], 'provisioner_observations': [],
                       'clock_note': 'command at/elapsed use host UTC/monotonic; Kubernetes timestamps retain their original server/node clock'}
        self.persist()

    def persist(self):
        temporary = self.output / 'verification.json.tmp'
        temporary.write_text(json.dumps(self.report, indent=2) + '\n')
        temporary.replace(self.output / 'verification.json')

    def command(self, argv, timeout=20, input_bytes=None, check=True):
        if not self.closing:
            remaining = self.deadline - time.monotonic()
            require(remaining > 0, 'experiment deadline exhausted before command')
            timeout = min(timeout, remaining)
        self.command_index += 1
        index = self.command_index
        receipt = {'index': index, 'command': argv, 'at': now(), 'stdin_bytes': len(input_bytes) if input_bytes is not None else None,
                   'stdin_sha256': sha(input_bytes) if input_bytes is not None else None}
        started = time.monotonic()
        try:
            result = subprocess.run(argv, input=input_bytes, capture_output=True, timeout=timeout, check=False)
            receipt.update(returncode=result.returncode, stdout=result.stdout.decode(errors='replace'), stderr=result.stderr.decode(errors='replace'))
        except subprocess.TimeoutExpired as error:
            receipt.update(returncode=None, timed_out=True, stdout=(error.stdout or b'').decode(errors='replace'),
                           stderr=(error.stderr or b'').decode(errors='replace'))
        except OSError as error:
            receipt.update(returncode=None, command_error=repr(error), stdout='', stderr='')
        finally:
            receipt.update(completed_at=now(), elapsed_seconds=time.monotonic() - started)
            (self.output / 'commands' / f'{index:05d}.json').write_text(json.dumps(receipt, indent=2) + '\n')
        if check:
            require(receipt.get('returncode') == 0, 'command failed: ' + repr(receipt))
        return receipt

    def kubectl(self, *argv, input_bytes=None, timeout=20, check=True, namespaced=True):
        command = ['kubectl', '--kubeconfig', self.args.kubeconfig, '--request-timeout=15s']
        if namespaced:
            command += ['-n', self.args.namespace]
        return self.command(command + list(argv), timeout, input_bytes, check)

    def get(self, kind, name=None):
        argv = ['get', kind] + ([name, '--ignore-not-found'] if name else []) + ['-o', 'json']
        result = self.kubectl(*argv, namespaced=kind not in {'pv', 'storageclass'})
        return json.loads(result['stdout']) if result['stdout'].strip() else None

    def apply(self, value, phase):
        encoded = (json.dumps(value, sort_keys=True) + '\n').encode()
        result = self.kubectl('apply', '-f', '-', input_bytes=encoded)
        self.report['mutations'].append({'phase': phase, 'at': now(), 'intent': value, 'command_index': result['index']})
        self.persist()

    def phase(self, name):
        self.report['phases'].append({'name': name, 'at': now()})
        self.persist()

    def until(self, name, fn, seconds=120):
        end = min(self.deadline, time.monotonic() + seconds)
        last = None
        while time.monotonic() < end:
            last = fn()
            if last:
                return last
            time.sleep(.5)
        raise VerificationError(name + ' exceeded its bounded observation deadline; last=' + repr(last))

    def start_controller(self, capacity=None):
        require(self.controller is None, 'controller is already running')
        if capacity is not None:
            self.capacity = capacity
        index = len(self.report['controllers'])
        log_name = f'controller-{index}.log'
        binary_hash = sha(pathlib.Path(self.args.controller_binary).read_bytes())
        previous = self.report['controllers'][-1] if self.report['controllers'] else None
        if previous:
            require(previous.get('reaped') and self.report['controllers'][0]['binary_sha256'] == binary_hash,
                    'controller restart did not preserve the binary or reap its previous process')
        argv = [self.args.controller_binary, '--kubeconfig', self.args.kubeconfig, '--namespace', self.args.namespace,
                '--image', self.args.image, '--storage-class', self.args.storage_class, '--capacity', self.capacity]
        self.controller_stream = (self.output / log_name).open('wb')
        try:
            self.controller = subprocess.Popen(argv, stdout=self.controller_stream, stderr=subprocess.STDOUT)
        except OSError:
            self.controller_stream.close()
            self.controller_stream = None
            raise
        self.report['controllers'].append({'index': index, 'pid': self.controller.pid, 'argv': argv, 'started_at': now(),
                'binary_sha256': binary_hash, 'log_path': log_name})
        self.persist()
        if previous:
            require(previous['pid'] != self.controller.pid, 'new controller reused the old PID; observation is ambiguous')
        time.sleep(.2)
        require(self.controller.poll() is None, 'controller exited at startup')

    def stop_controller(self):
        if self.controller is None:
            return
        record = self.report['controllers'][-1]
        record.update(stop_requested_at=now(), forced=False)
        if self.controller.poll() is None:
            self.controller.terminate()
        try:
            code = self.controller.wait(timeout=20)
        except subprocess.TimeoutExpired:
            record['forced'] = True
            self.controller.kill()
            code = self.controller.wait(timeout=5)
        record.update(exited_at=now(), exit_code=code, reaped=True)
        self.controller_stream.close()
        self.controller, self.controller_stream = None, None
        self.persist()
        require(not record['forced'], 'controller required forced termination')

    def image_proof(self, pod):
        main = next(c for c in pod['spec']['containers'] if c['name'] == 'trino')
        require(main['image'] == self.args.image, 'Pod requests another fixture image')
        process = identity(pod)
        require(process['container_id'] and process['image_id'], 'Pod has no actual container/image identity')
        if process['container_id'] in self.checked_images:
            return
        require(pod['spec']['nodeName'] == self.args.node_name, 'Pod is running outside the dedicated kind node')
        inventory = json.loads(pathlib.Path(self.args.image_inventory).read_text())
        require(len(inventory) == 1 and self.args.image in inventory[0].get('RepoDigests', []), 'host image inventory lacks the pinned digest')
        resolved = self.command(['docker', 'exec', self.args.node_name, 'crictl', 'inspecti', self.args.image])
        image = json.loads(resolved['stdout'])['status']
        normalize = lambda value: value.removeprefix('docker-pullable://').removeprefix('containerd://')
        identities = {normalize(image['id']), *map(normalize, image.get('repoDigests', []))}
        require(normalize(process['image_id']) in identities, 'actual Pod imageID differs from the pinned runtime image resolution')
        container_id = process['container_id'].split('://', 1)[-1]
        inspected = self.command(['docker', 'exec', self.args.node_name, 'crictl', 'inspect', container_id])
        container = json.loads(inspected['stdout'])['status']
        require(container['id'] == container_id and container.get('labels', {}).get('io.kubernetes.pod.uid') == process['pod_uid'] and
                container.get('metadata', {}).get('name') == 'trino', 'CRI container observation belongs to another process')
        require(normalize(container['imageRef']) in identities, 'actual CRI container did not resolve to the pinned image artifact')
        self.report['image_observations'].append({'at': now(), 'process': process, 'requested_image': self.args.image,
            'host_image_id': inventory[0]['Id'], 'host_repo_digests': inventory[0]['RepoDigests'],
            'host_image_inventory_sha256': sha(pathlib.Path(self.args.image_inventory).read_bytes()),
            'runtime_image': image, 'cri_container_image_ref': container['imageRef'],
            'image_command_index': resolved['index'], 'container_command_index': inspected['index']})
        self.checked_images.add(process['container_id'])
        self.persist()

    def start_storage_watch(self):
        argv = ['kubectl', '--kubeconfig', self.args.kubeconfig, '--request-timeout=900s', '-n', 'local-path-storage',
                'get', 'pods', '--watch', '--output-watch-events', '-o', 'json']
        self.watch_streams = [(self.output / 'provisioner-pod-watch.json').open('wb'),
                              (self.output / 'provisioner-pod-watch.stderr').open('wb')]
        self.storage_watch = subprocess.Popen(argv, stdout=self.watch_streams[0], stderr=self.watch_streams[1])
        self.report['storage_watch'] = {'pid': self.storage_watch.pid, 'command': argv, 'started_at': now()}
        self.persist()
        def attached():
            require(self.storage_watch.poll() is None, 'storage Pod watch exited before attachment')
            values, _ = json_stream((self.output / 'provisioner-pod-watch.json').read_text())
            return any(v.get('type') == 'ADDED' and v.get('object', {}).get('metadata', {}).get('namespace') == 'local-path-storage'
                       and v['object']['metadata'].get('uid') for v in values)
        self.until('initial storage Pod watch', attached, 20)
        self.report['storage_watch']['attached_at'] = now()
        self.persist()

    def record_provisioner(self, pv_name):
        def observed():
            values, _ = json_stream((self.output / 'provisioner-pod-watch.json').read_text())
            for index, event in enumerate(values):
                pod = event.get('object', {})
                if pod.get('metadata', {}).get('name') != 'helper-pod-create-' + pv_name:
                    continue
                for state in pod.get('status', {}).get('containerStatuses', []):
                    if state.get('imageID') and state.get('containerID'):
                        return {'watch_event_index': index, 'watch_event': event, 'actual_image_id': state['imageID'],
                                'actual_container_id': state['containerID']}
            return None
        helper = self.until('actual local-path provisioning helper image', observed, 30)
        images = self.command(['docker', 'exec', self.args.node_name, 'crictl', 'images', '-o', 'json'])
        inventory = json.loads(images['stdout'])
        actual = helper['actual_image_id'].removeprefix('docker-pullable://').removeprefix('containerd://')
        matched = [item for item in inventory['images'] if actual == item['id'] or actual in item.get('repoDigests', [])]
        require(len(matched) == 1, 'actual provisioning helper imageID not found in the post-provision runtime inventory')
        helper.update(at=now(), pv_name=pv_name, runtime_image=matched[0], runtime_inventory_command_index=images['index'])
        self.report['provisioner_observations'].append(helper)
        self.persist()

    def live_bundle(self, name):
        cr = self.get(RESOURCE, name)
        pvc = self.get('pvc', 'data-' + name + '-workers-default-0')
        pv = self.get('pv', pvc['spec']['volumeName']) if pvc and pvc.get('spec', {}).get('volumeName') else None
        return cr, pvc, pv

    def snapshot(self, name, phase, expected=None, capacity=None):
        cr, pvc, pv = self.live_bundle(name)
        observed = inspect_binding(cr, pvc, pv, self.args.storage_class, capacity or self.args.capacity, expected)
        pod = self.get('pod', name + '-workers-default-0')
        sts = self.get('statefulset', name + '-workers-default')
        if pod:
            validate_mount(pod, pvc['metadata']['name'])
            self.image_proof(pod)
        record = {'phase': phase, 'at': now(), 'identity': observed, 'cr': cr, 'pvc': pvc, 'pv': pv, 'pod': pod, 'statefulset': sts}
        self.report['snapshots'].append(record)
        self.persist()
        return observed

    def wait_ready(self, name):
        def ready():
            require(self.controller and self.controller.poll() is None, 'controller stopped during convergence')
            cr, pvc, pv = self.live_bundle(name)
            pod = self.get('pod', name + '-workers-default-0')
            sts = self.get('statefulset', name + '-workers-default')
            if not cr or not pvc or not pv or not pod or not sts:
                return None
            annotations = pvc['metadata'].get('annotations', {})
            if not annotations.get(BINDING) or pvc.get('status', {}).get('phase') != 'Bound' or pv.get('status', {}).get('phase') != 'Bound':
                return None
            observed = inspect_binding(cr, pvc, pv, self.args.storage_class, self.capacity)
            conditions = {c['type']: c for c in cr.get('status', {}).get('conditions', [])}
            if not (conditions.get('Applied', {}).get('status') == 'True' and
                    conditions.get('WorkloadsReady', {}).get('status') == 'True' and
                    cr.get('status', {}).get('observedGeneration') == cr['metadata']['generation']):
                return None
            require(sts['spec'].get('persistentVolumeClaimRetentionPolicy') == {'whenDeleted': 'Retain', 'whenScaled': 'Retain'},
                    'StatefulSet did not retain claims on both transitions')
            status = next((c for c in pod.get('status', {}).get('containerStatuses', []) if c['name'] == 'trino'), {})
            if not status.get('ready') or not status.get('containerID') or pod['metadata'].get('deletionTimestamp'):
                return None
            owner = next((o for o in pod['metadata'].get('ownerReferences', []) if o.get('controller')), {})
            require(owner.get('uid') == sts['metadata']['uid'], 'Pod does not belong to the observed StatefulSet')
            validate_mount(pod, pvc['metadata']['name'])
            self.image_proof(pod)
            return {'binding': observed, 'process': identity(pod), 'statefulset_uid': sts['metadata']['uid']}
        result = self.until(name + ' ready and binding receipt', ready, 180)
        self.snapshot(name, name + '-ready', result['binding'])
        return result

    def set_group(self, name, present, phase):
        value = fixture(name, self.args.namespace, present)
        # The public CR owns storage. The role group overrides capacity only;
        # class and type must survive role inheritance all the way to the PVC.
        value['spec']['workers']['config'] = {'resources': {'storage': {
            'type': 'persistent', 'storageClassName': self.args.storage_class, 'capacity': '32Mi'}}}
        if present:
            value['spec']['workers']['roleGroups']['default']['config']['resources'] = {
                'storage': {'capacity': self.capacity}}
        self.apply(value, phase)

    def absent_group(self, name):
        def absent():
            slots = [('statefulset', name + '-workers-default'), ('service', name + '-workers-default'),
                     ('service', name + '-workers-default-headless'), ('configmap', name + '-workers-default')]
            if any(self.get(kind, slot) is not None for kind, slot in slots):
                return False
            pods = self.get('pods')['items']
            if any(p['metadata']['name'].startswith(name + '-workers-default-') for p in pods):
                return False
            cr = self.get(RESOURCE, name)
            retired = next((c for c in cr.get('status', {}).get('conditions', []) if c['type'] == 'Retired'), {})
            return retired.get('status') == 'True' and cr.get('status', {}).get('observedGeneration') == cr['metadata']['generation']
        self.until(name + ' fixed slots and original Pods absent', absent)
        self.report['phases'].append({'name': name + '-retired-slots-absent', 'at': now()})
        self.persist()

    def exec_python(self, pod, code, *args, data=None):
        result = self.kubectl('exec', '-i', pod, '-c', 'trino', '--', 'python3', '-c', code, *args, input_bytes=data)
        return json.loads(result['stdout']), result['index']

    def write_marker(self, name, ready):
        payload = (json.dumps({'nonce': str(uuid.uuid4()), 'initial_binding': ready['binding']}, sort_keys=True) + '\n').encode()
        before = identity(self.get('pod', ready['process']['pod_name']))
        require(before == ready['process'], 'process changed before exclusive marker write')
        written, command = self.exec_python(before['pod_name'], WRITE_MARKER, '/data/marker.json', data=payload)
        require(written.get('uid') == 1000 and written.get('gid') == 1000, 'marker writer is not the expected non-root identity')
        require(written['sha256'] == sha(payload) and base64.b64decode(written['base64']) == payload and
                written.get('fsync_file') and written.get('fsync_parent_directory'), 'fsync marker write did not match the known bytes')
        require(identity(self.get('pod', before['pod_name'])) == before, 'process changed during marker write')
        record = {'phase': name + '-exclusive-write-fsync', 'at': now(), 'process': before, 'command_index': command, 'result': written}
        self.report['marker_observations'].append(record)
        self.persist()
        return payload

    def read_marker(self, pod, payload, phase, http=True):
        original_pod = self.get('pod', pod)
        before = identity(original_pod)
        self.image_proof(original_pod)
        result, command = self.exec_python(pod, HTTP_MARKER if http else READ_MARKER, *([] if http else ['/data/marker.json']))
        require(result['sha256'] == sha(payload) and result['bytes'] == len(payload) and base64.b64decode(result['base64']) == payload,
                'original marker bytes were not retained')
        if http:
            require(result.get('status') == 200 and int(result['content_length']) == len(payload), 'HTTP marker response did not match')
        require(identity(self.get('pod', pod)) == before, 'reader process changed during data observation')
        self.report['marker_observations'].append({'phase': phase, 'at': now(), 'process': before, 'read_only': True,
                                                  'command_index': command, 'result': result})
        self.persist()

    def inspect_retired_marker(self, name, payload, expected):
        reader_name = 'retained-reader-' + str(len(self.inspectors))
        reader = {'apiVersion': 'v1', 'kind': 'Pod', 'metadata': {'name': reader_name, 'namespace': self.args.namespace,
                  'labels': {'operator-go.design/fixture': 'read-only-observer'}},
                  'spec': {'restartPolicy': 'Never', 'terminationGracePeriodSeconds': 1,
                  'securityContext': {'runAsUser': 1000, 'runAsGroup': 1000, 'fsGroup': 1000, 'runAsNonRoot': True},
                  'containers': [{'name': 'trino', 'image': self.args.image, 'command': ['python3', '-c', 'import time;time.sleep(180)'],
                    'volumeMounts': [{'name': 'data', 'mountPath': '/data', 'readOnly': True}]}],
                  'volumes': [{'name': 'data', 'persistentVolumeClaim': {'claimName': expected['pvc_name'], 'readOnly': True}}]}}
        self.inspectors.append(reader_name)
        self.apply(reader, name + '-read-only-inspector')
        self.until('read-only inspector running', lambda: any(c.get('ready') for c in (self.get('pod', reader_name) or {}).get('status', {}).get('containerStatuses', [])), 90)
        self.read_marker(reader_name, payload, name + '-retired-volume-read', http=False)
        self.delete_object('pod', reader_name, name + '-delete-inspector')
        self.until('inspector absent before re-add', lambda: self.get('pod', reader_name) is None, 45)
        self.snapshot(name, name + '-retired-binding-after-read', expected)

    def delete_object(self, kind, name, phase):
        live = self.get(kind, name)
        require(live, 'explicit deletion target does not exist: ' + name)
        # kubectl delete --raw supports Kubernetes DeleteOptions preconditions.
        if kind == 'pod':
            path = '/api/v1/namespaces/' + self.args.namespace + '/pods/' + name
        elif kind == 'pvc':
            path = '/api/v1/namespaces/' + self.args.namespace + '/persistentvolumeclaims/' + name
        elif kind == RESOURCE:
            path = '/apis/trino.kubedoop.dev/v1alpha1/namespaces/' + self.args.namespace + '/trinoclusters/' + name
        else:
            raise VerificationError('unsupported explicit delete kind')
        options = {'apiVersion': 'v1', 'kind': 'DeleteOptions', 'preconditions': {'uid': live['metadata']['uid'],
                   'resourceVersion': live['metadata']['resourceVersion']}, 'propagationPolicy': 'Background'}
        result = self.kubectl('delete', '--raw', path, '-f', '-', input_bytes=json.dumps(options).encode())
        self.report['mutations'].append({'phase': phase, 'at': now(), 'kind': kind, 'name': name, 'delete_options': options,
                                         'command_index': result['index']})
        self.persist()

    def patch_pvc(self, name, patch, phase):
        live = self.get('pvc', name)
        operations = [{'op': 'test', 'path': '/metadata/uid', 'value': live['metadata']['uid']},
                      {'op': 'test', 'path': '/metadata/resourceVersion', 'value': live['metadata']['resourceVersion']}] + patch
        result = self.kubectl('patch', 'pvc', name, '--type=json', '-p', json.dumps(operations))
        self.report['mutations'].append({'phase': phase, 'at': now(), 'pvc_name': name, 'operations': operations,
                                         'command_index': result['index']})
        self.persist()

    def wait_blocked(self, name, expected_text, original, phase):
        def blocked():
            cr = self.get(RESOURCE, name)
            require(self.get('statefulset', name + '-workers-default') is None, 'unsafe claim was consumed by a new StatefulSet')
            pods = self.get('pods')['items']
            require(not any(v.get('persistentVolumeClaim', {}).get('claimName') == original['pvc_name']
                            for p in pods for v in p.get('spec', {}).get('volumes', [])), 'unsafe claim was consumed by a Pod')
            if cr.get('status', {}).get('observedGeneration') != cr['metadata']['generation']:
                return None
            groups = cr.get('status', {}).get('groups', [])
            message = '\n'.join(g.get('message', '') for g in groups)
            return cr if expected_text in message else None
        observed = self.until(phase + ' reported storage conflict', blocked, 60)
        # Repeated fresh observations ensure a reported error did not race workload creation.
        for _ in range(3):
            time.sleep(.5)
            require(blocked(), 'storage conflict disappeared or did not hold')
        pvc = self.get('pvc', original['pvc_name'])
        pv = self.get('pv', original['pv_name'])
        require(pvc and pv and pvc['metadata']['uid'] == original['pvc_uid'] and pv['metadata']['uid'] == original['pv_uid'],
                'negative scenario changed the data resource identity')
        self.report['negative_cases'].append({'phase': phase, 'at': now(), 'expected_message_fragment': expected_text,
                'cr': observed, 'pvc': pvc, 'pv': pv, 'no_dependent_statefulset_or_pod': True})
        self.persist()

    def prepare_retired(self, name):
        self.set_group(name, True, name + '-create')
        ready = self.wait_ready(name)
        self.record_provisioner(ready['binding']['pv_name'])
        payload = self.write_marker(name, ready)
        self.set_group(name, False, name + '-remove-group')
        self.absent_group(name)
        self.snapshot(name, name + '-retained', ready['binding'])
        return ready, payload

    def run(self):
        self.start_storage_watch()
        self.start_controller()
        name = 'retained-main'
        self.phase('same-source-core')
        ready, payload = self.prepare_retired(name)
        self.inspect_retired_marker(name, payload, ready['binding'])
        self.stop_controller()
        self.start_controller()
        self.set_group(name, True, 'same-source-readd-after-controller-restart')
        restored = self.wait_ready(name)
        require(restored['binding'] == ready['binding'], 're-add replaced the original data binding')
        require(restored['process']['pod_uid'] != ready['process']['pod_uid'] and
                restored['statefulset_uid'] != ready['statefulset_uid'], 're-add did not create new workload identities')
        self.read_marker(restored['process']['pod_name'], payload, 'restored-original-bytes')
        self.snapshot(name, 'restored-binding', ready['binding'])
        self.set_group(name, False, 'remove-restored-main-before-negative')
        self.absent_group(name)

        self.phase('negative-same-name-new-cr-uid')
        self.delete_object(RESOURCE, name, 'delete-original-cr-after-retirement')
        self.until('original CR absent', lambda: self.get(RESOURCE, name) is None, 45)
        self.set_group(name, True, 'recreate-same-name-cr')
        require(self.get(RESOURCE, name)['metadata']['uid'] != ready['binding']['cr_uid'], 'new CR did not get a new UID')
        self.wait_blocked(name, 'provenance', ready['binding'], 'same-name-new-cr-uid')
        self.set_group(name, False, 'remove-blocked-new-cr-group')

        for case in ('missing-provenance', 'dangerous-ownerref', 'claim-deleting', 'spec-diff'):
            self.phase('negative-' + case)
            current = 'negative-' + case
            state, marker = self.prepare_retired(current)
            claim = state['binding']['pvc_name']
            if case == 'missing-provenance':
                self.patch_pvc(claim, [{'op': 'remove', 'path': '/metadata/annotations/' + SOURCE.replace('~', '~0').replace('/', '~1')}], case)
                expected = 'provenance'
            elif case == 'dangerous-ownerref':
                guard = {'apiVersion': 'v1', 'kind': 'ConfigMap', 'metadata': {'name': 'ownerref-guard', 'namespace': self.args.namespace}}
                self.apply(guard, 'create-live-ownerref-guard')
                guard = self.get('configmap', 'ownerref-guard')
                self.patch_pvc(claim, [{'op': 'add', 'path': '/metadata/ownerReferences', 'value': [{'apiVersion': 'v1',
                    'kind': 'ConfigMap', 'name': guard['metadata']['name'], 'uid': guard['metadata']['uid'], 'controller': False}]}], case)
                expected = 'owner references'
            elif case == 'claim-deleting':
                pvc = self.get('pvc', claim)
                finalizers = pvc['metadata'].get('finalizers', []) + [FINALIZER]
                self.patch_pvc(claim, [{'op': 'add', 'path': '/metadata/finalizers', 'value': finalizers}], case + '-hold-finalizer')
                self.delete_object('pvc', claim, case + '-delete-held-claim')
                self.until('claim deletion timestamp', lambda: self.get('pvc', claim)['metadata'].get('deletionTimestamp'), 30)
                expected = 'deleting'
            else:
                self.capacity = '128Mi'
                expected = 'provenance'
            self.set_group(current, True, case + '-readd')
            self.wait_blocked(current, expected, state['binding'], case)
            self.set_group(current, False, case + '-remove-blocked-group')
            if case == 'spec-diff':
                self.capacity = self.args.capacity
            # The held/decorated claims remain untouched until dedicated kind cleanup.
            require(sha(marker), 'negative marker evidence missing')
        self.report['passed'] = True

    def close(self):
        self.closing = True
        cleanup_errors = []
        try:
            self.stop_controller()
        except Exception as error:
            cleanup_errors.append(repr(error))
        for name in self.inspectors:
            try:
                self.kubectl('delete', 'pod', name, '--ignore-not-found', '--wait=false', timeout=20)
            except Exception as error:
                cleanup_errors.append(repr(error))
        if self.storage_watch is not None:
            record = self.report['storage_watch']
            record['forced'] = False
            if self.storage_watch.poll() is None:
                self.storage_watch.terminate()
            try:
                code = self.storage_watch.wait(timeout=10)
            except subprocess.TimeoutExpired:
                record['forced'] = True
                self.storage_watch.kill()
                code = self.storage_watch.wait(timeout=5)
            record.update(exit_code=code, reaped=True, exited_at=now())
            for stream in self.watch_streams:
                stream.close()
            _, partial = json_stream((self.output / 'provisioner-pod-watch.json').read_text())
            record['incomplete_final_json_record'] = partial
            if record['forced']:
                cleanup_errors.append('storage watch required forced termination')
        self.report.update(completed_at=now(), command_count=self.command_index, cleanup_errors=cleanup_errors,
                           all_owned_processes_stopped=all(c.get('reaped') and not c.get('forced') for c in self.report['controllers']))
        if cleanup_errors or not self.report['all_owned_processes_stopped']:
            self.report['passed'] = False
        self.persist()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('kubeconfig', 'namespace', 'output-dir', 'controller-binary', 'image', 'storage-class', 'node-name', 'image-inventory'):
        parser.add_argument('--' + name, required=True)
    parser.add_argument('--capacity', default='64Mi')
    parser.add_argument('--timeout', type=float, default=900)
    args = parser.parse_args()
    if os.getpgrp() != os.getpid():
        os.setsid()
    verifier = Verifier(args)
    verifier.report['process_group'] = {'pid': os.getpid(), 'pgid': os.getpgrp(), 'owned_controller_children_share_this_group': True}
    def interrupted(signum, _frame):
        raise VerificationError('experiment interrupted by signal ' + str(signum))
    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGINT, interrupted)
    try:
        verifier.run()
    except Exception as error:
        verifier.report.update(passed=False, error=repr(error))
        print(repr(error), file=sys.stderr)
    finally:
        verifier.close()
    raise SystemExit(0 if verifier.report['passed'] else 1)


if __name__ == '__main__':
    main()
