import base64
import copy
import importlib.util
import json
import pathlib
import subprocess
import sys
import tempfile
import types
import unittest
from unittest import mock

PATH = pathlib.Path(__file__).with_name('verify-storage.py')
SPEC = importlib.util.spec_from_file_location('verify_storage', PATH)
M = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(M)


def bound_objects():
    cr = {'metadata': {'name': 'example', 'namespace': 'isolated', 'uid': 'cr-original'}}
    pvc = {'metadata': {'name': 'data-example-workers-default-0', 'namespace': 'isolated', 'uid': 'pvc-original',
            'annotations': {M.SOURCE: json.dumps(M.source_receipt(cr, 'retained', '64Mi')),
                            M.BINDING: json.dumps({'version': 1, 'pvcUID': 'pvc-original', 'pvUID': 'pv-original', 'volumeName': 'volume'})}},
           'spec': {'storageClassName': 'retained', 'accessModes': ['ReadWriteOnce'], 'volumeMode': 'Filesystem',
                    'volumeName': 'volume', 'resources': {'requests': {'storage': '64Mi'}}}, 'status': {'phase': 'Bound'}}
    pv = {'metadata': {'name': 'volume', 'uid': 'pv-original'}, 'spec': {'storageClassName': 'retained',
            'persistentVolumeReclaimPolicy': 'Retain', 'volumeMode': 'Filesystem', 'accessModes': ['ReadWriteOnce'],
            'capacity': {'storage': '128Mi'}, 'claimRef': {'name': pvc['metadata']['name'], 'namespace': 'isolated', 'uid': 'pvc-original'},
            'hostPath': {'path': '/var/local-path-provisioner/volume'}}, 'status': {'phase': 'Bound'}}
    return cr, pvc, pv


class BindingTests(unittest.TestCase):
    def check(self, objects):
        return M.inspect_binding(*objects, 'retained', '64Mi')

    def test_real_binding_requires_original_uid_but_allows_larger_pv(self):
        result = self.check(bound_objects())
        self.assertEqual(result['pvc_uid'], 'pvc-original')
        self.assertEqual(result['pv_uid'], 'pv-original')

    def test_same_name_new_cr_is_not_original_source(self):
        cr, pvc, pv = bound_objects()
        cr['metadata']['uid'] = 'new-cr'
        with self.assertRaisesRegex(M.VerificationError, 'provenance'):
            self.check((cr, pvc, pv))

    def test_receipts_do_not_replace_bidirectional_actual_binding(self):
        for change in ('pvc-uid', 'pv-uid', 'claim-ref', 'volume-name'):
            cr, pvc, pv = bound_objects()
            if change == 'pvc-uid':
                pvc['metadata']['uid'] = 'another-pvc'
            elif change == 'pv-uid':
                pv['metadata']['uid'] = 'another-pv'
            elif change == 'claim-ref':
                pv['spec']['claimRef']['uid'] = 'another-pvc'
            else:
                pvc['spec']['volumeName'] = 'another-volume'
            with self.subTest(change=change), self.assertRaises(M.VerificationError):
                self.check((cr, pvc, pv))

    def test_unbound_or_reclaiming_data_is_not_proved_retained(self):
        for change in ('pending', 'delete', 'owner', 'terminating', 'small-pv'):
            cr, pvc, pv = bound_objects()
            if change == 'pending':
                pvc['status']['phase'] = 'Pending'
            elif change == 'delete':
                pv['spec']['persistentVolumeReclaimPolicy'] = 'Delete'
            elif change == 'owner':
                pvc['metadata']['ownerReferences'] = [{'uid': 'live-guard', 'controller': False}]
            elif change == 'terminating':
                pvc['metadata']['deletionTimestamp'] = '2026-09-14T00:00:00Z'
            else:
                pv['spec']['capacity']['storage'] = '1Mi'
            with self.subTest(change=change), self.assertRaises(M.VerificationError):
                self.check((cr, pvc, pv))

    def test_missing_or_changed_provenance_is_rejected(self):
        for key in (M.SOURCE, M.BINDING):
            cr, pvc, pv = bound_objects()
            pvc['metadata']['annotations'].pop(key)
            with self.subTest(key=key), self.assertRaises(M.VerificationError):
                self.check((cr, pvc, pv))
        cr, pvc, pv = bound_objects()
        pvc['spec']['resources']['requests']['storage'] = '65Mi'
        with self.assertRaises(M.VerificationError):
            self.check((cr, pvc, pv))

    def test_same_uid_does_not_hide_replaced_data_path(self):
        values = bound_objects()
        prior = self.check(values)
        values[2]['spec']['hostPath']['path'] = '/another-path'
        with self.assertRaisesRegex(M.VerificationError, 'identity, source or binding changed'):
            M.inspect_binding(*values, 'retained', '64Mi', prior)


class MarkerAndMountTests(unittest.TestCase):
    def test_exclusive_fsync_write_cannot_recreate_existing_marker(self):
        with tempfile.TemporaryDirectory() as root:
            path = pathlib.Path(root) / 'marker.json'
            original = b'{"random-nonce":"original"}\n'
            run = lambda code, data=None: subprocess.run([sys.executable, '-c', code, str(path)], input=data,
                                                         capture_output=True, timeout=10)
            written = run(M.WRITE_MARKER, original)
            self.assertEqual(written.returncode, 0, written.stderr)
            record = json.loads(written.stdout)
            self.assertTrue(record['fsync_file'] and record['fsync_parent_directory'])
            self.assertEqual(base64.b64decode(record['base64']), original)
            refused = run(M.WRITE_MARKER, b'replacement')
            self.assertNotEqual(refused.returncode, 0)
            self.assertEqual(path.read_bytes(), original)
            read = run(M.READ_MARKER)
            self.assertEqual(read.returncode, 0)
            self.assertEqual(base64.b64decode(json.loads(read.stdout)['base64']), original)

    def test_mount_to_same_named_emptydir_does_not_prove_pvc_consumption(self):
        pod = {'spec': {'containers': [{'name': 'trino', 'volumeMounts': [{'name': 'data', 'mountPath': '/data'}]}],
                        'volumes': [{'name': 'data', 'persistentVolumeClaim': {'claimName': 'original'}}]}}
        M.validate_mount(pod, 'original')
        empty = copy.deepcopy(pod)
        empty['spec']['volumes'][0] = {'name': 'data', 'emptyDir': {}}
        with self.assertRaises(M.VerificationError):
            M.validate_mount(empty, 'original')
        changed = copy.deepcopy(pod)
        changed['spec']['containers'][0]['volumeMounts'][0]['subPath'] = 'different'
        with self.assertRaises(M.VerificationError):
            M.validate_mount(changed, 'original')

    def test_watch_keeps_complete_records_before_partial_tail(self):
        first = {'type': 'ADDED', 'object': {'metadata': {'uid': 'actual'}}}
        text = json.dumps(first) + '\n{"type":"MOD'
        values, partial = M.json_stream(text)
        self.assertEqual(values, [first])
        self.assertTrue(partial)
        self.assertEqual(M.json_stream(json.dumps(first) + '\n'), ([first], False))


class ImageTests(unittest.TestCase):
    def test_tag_and_declared_digest_are_not_actual_image_proof(self):
        with tempfile.TemporaryDirectory() as root:
            path = pathlib.Path(root) / 'image.json'
            expected = 'example/image@sha256:manifest'
            path.write_text(json.dumps([{'Id': 'sha256:configuration', 'RepoDigests': [expected]}]))
            verifier = object.__new__(M.Verifier)
            verifier.args = types.SimpleNamespace(image=expected, image_inventory=str(path), node_name='dedicated-node')
            verifier.checked_images = set()
            verifier.report = {'image_observations': []}
            verifier.persist = lambda: None
            def command(argv):
                status = ({'id': 'sha256:configuration', 'repoDigests': [expected]} if 'inspecti' in argv else
                          {'id': 'container', 'metadata': {'name': 'trino'}, 'labels': {'io.kubernetes.pod.uid': 'pod'},
                           'imageRef': expected})
                return {'index': 1, 'stdout': json.dumps({'status': status})}
            verifier.command = command
            pod = {'metadata': {'name': 'worker', 'uid': 'pod'}, 'spec': {'nodeName': 'dedicated-node',
                   'containers': [{'name': 'trino', 'image': expected}]}, 'status': {'containerStatuses': [
                   {'name': 'trino', 'containerID': 'containerd://container', 'imageID': 'another/image@sha256:wrong'}]}}
            with self.assertRaisesRegex(M.VerificationError, 'actual Pod imageID'):
                verifier.image_proof(pod)
            pod['status']['containerStatuses'][0]['imageID'] = expected
            verifier.image_proof(pod)
            self.assertEqual(len(verifier.report['image_observations']), 1)
            pod['spec']['containers'][0]['image'] = 'mutable:tag'
            with self.assertRaisesRegex(M.VerificationError, 'requests another fixture image'):
                verifier.image_proof(pod)


class ControllerCleanupTests(unittest.TestCase):
    def verifier(self, root):
        binary = pathlib.Path(root) / 'controller'
        binary.write_bytes(b'frozen-binary')
        value = object.__new__(M.Verifier)
        value.output = pathlib.Path(root)
        value.args = types.SimpleNamespace(controller_binary=str(binary), kubeconfig='isolated', namespace='isolated',
                                          image='fixed', storage_class='retained')
        value.controller, value.controller_stream, value.capacity = None, None, '64Mi'
        value.report = {'controllers': [{'pid': 42, 'reaped': True, 'binary_sha256': M.sha(binary.read_bytes())}]}
        value.persist = lambda: None
        return value

    def test_unreaped_previous_child_blocks_before_spawn(self):
        with tempfile.TemporaryDirectory() as root:
            value = self.verifier(root)
            value.report['controllers'][0]['reaped'] = False
            with mock.patch.object(M.subprocess, 'Popen') as spawn, self.assertRaises(M.VerificationError):
                value.start_controller()
            spawn.assert_not_called()
            self.assertFalse((pathlib.Path(root) / 'controller-1.log').exists())

    def test_post_spawn_rejection_still_records_and_reaps_new_child(self):
        with tempfile.TemporaryDirectory() as root:
            value = self.verifier(root)
            original = copy.deepcopy(value.report['controllers'][0])
            child = mock.Mock(pid=42)
            child.poll.return_value = None
            child.wait.return_value = 0
            with mock.patch.object(M.subprocess, 'Popen', return_value=child), self.assertRaises(M.VerificationError):
                value.start_controller()
            self.assertEqual(len(value.report['controllers']), 2)
            value.stop_controller()
            self.assertEqual(value.report['controllers'][0], original)
            self.assertTrue(value.report['controllers'][1]['reaped'])
            self.assertEqual(value.report['controllers'][1]['exit_code'], 0)
            child.terminate.assert_called_once()


if __name__ == '__main__':
    unittest.main()
