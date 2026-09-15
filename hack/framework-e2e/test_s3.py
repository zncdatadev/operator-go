import copy
import unittest
from unittest import mock

from runtime import VerificationError
from s3 import S3Verifier


class SecretClassReceiptTests(unittest.TestCase):
    def setUp(self):
        self.group = {"pod": "worker-0", "pod_uid": "current-pod"}
        self.pod = {"metadata": {"uid": "current-pod"}, "spec": {"volumes": [{"name": "s3-credentials",
            "ephemeral": {"volumeClaimTemplate": {"metadata": {"annotations": {"secrets.kubedoop.dev/class": "s3-class"}},
                "spec": {"storageClassName": "secrets.kubedoop.dev"}}}}]}}
        self.claim = {"metadata": {"name": "worker-0-s3-credentials", "namespace": "fixture", "uid": "current-pvc",
            "ownerReferences": [{"kind": "Pod", "controller": True, "uid": "current-pod"}]},
            "spec": {"volumeName": "csi-volume"}, "status": {"phase": "Bound"}}
        self.pv = {"metadata": {"name": "csi-volume", "uid": "current-pv"},
            "spec": {"claimRef": {"name": "worker-0-s3-credentials", "namespace": "fixture", "uid": "current-pvc"},
                     "csi": {"driver": "secrets.kubedoop.dev"}}, "status": {"phase": "Bound"}}
        self.verifier = object.__new__(S3Verifier)
        self.verifier.get = lambda kind, _name: {"pod": self.pod, "pvc": self.claim, "pv": self.pv}[kind]

    def test_actual_ephemeral_pvc_pv_chain_proves_csi(self):
        receipt = self.verifier.secretclass_receipt(self.group, "s3-class")
        self.assertEqual(receipt, {"pod_uid": "current-pod", "secret_class": "s3-class",
                                  "pvc_uid": "current-pvc", "pv_uid": "current-pv", "driver": "secrets.kubedoop.dev"})
        self.assertNotIn("csi", self.pod["spec"]["volumes"][0])

    def test_stale_pod_or_different_pv_does_not_prove_consumption(self):
        for kind in ("pod", "pvc", "driver"):
            with self.subTest(kind=kind):
                pod, claim, pv = copy.deepcopy((self.pod, self.claim, self.pv))
                if kind == "pod":
                    pod["metadata"]["uid"] = "old-pod"
                elif kind == "pvc":
                    pv["spec"]["claimRef"]["uid"] = "other-pvc"
                else:
                    pv["spec"]["csi"]["driver"] = "unrelated-driver"
                self.verifier.get = lambda resource, _name: {"pod": pod, "pvc": claim, "pv": pv}[resource]
                with self.assertRaises(VerificationError):
                    self.verifier.secretclass_receipt(self.group, "s3-class")


class RefreshedWriteTests(unittest.TestCase):
    def setUp(self):
        self.verifier = object.__new__(S3Verifier)
        self.previous = [{"key": "e02/marker/original", "size": 246}]
        self.objects = self.previous + [{"key": "e02/marker_refreshed/new", "size": 246}]
        self.verifier.query = mock.Mock()
        self.verifier.query_ready = mock.Mock(return_value={"rows": [[42], [84]]})
        self.verifier.observe_files = mock.Mock(return_value=self.objects)

    def test_single_ctas_reads_both_tables_and_observes_new_object(self):
        selected, objects = self.verifier.refreshed_write_read(self.previous)
        self.verifier.query.assert_called_once_with("CREATE TABLE hive.e02.marker_refreshed AS SELECT BIGINT '84' AS value")
        self.verifier.query_ready.assert_called_once_with("SELECT value FROM hive.e02.marker UNION ALL "
                                                         "SELECT value FROM hive.e02.marker_refreshed ORDER BY value")
        self.assertEqual(selected["rows"], [[42], [84]])
        self.assertEqual(objects, self.objects)

    def test_ctas_uncertain_failure_is_not_retried(self):
        self.verifier.query.side_effect = VerificationError("write outcome unknown")
        with self.assertRaises(VerificationError):
            self.verifier.refreshed_write_read(self.previous)
        self.verifier.query.assert_called_once()
        self.verifier.query_ready.assert_not_called()

    def test_existing_objects_alone_do_not_prove_a_new_write(self):
        self.verifier.observe_files.return_value = self.previous
        with self.assertRaisesRegex(VerificationError, "new independently observed"):
            self.verifier.refreshed_write_read(self.previous)
