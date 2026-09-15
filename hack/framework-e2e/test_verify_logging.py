import importlib.util
import pathlib
import time
import unittest
from unittest import mock

SPEC = importlib.util.spec_from_file_location("verify_logging", pathlib.Path(__file__).with_name("verify-logging.py"))
M = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(M)


class ProvenanceTests(unittest.TestCase):
    def test_pod_delivery_may_precede_status_commit(self):
        verifier = object.__new__(M.Verification)
        verifier.controller = None
        verifier.deadline = time.monotonic() + 30
        cm = {"metadata": {"namespace": "logging-e03", "uid": "same-cm", "resourceVersion": "new"}}

        def observation(version):
            return {"metadata": {"generation": 1}, "status": {"groups": [{"role": "workers", "name": "default",
                "facts": {"state": "resolved", "observed": [{"apiVersion": "v1", "kind": "ConfigMap",
                    "namespace": "logging-e03", "name": "destination", "uid": "same-cm", "resourceVersion": version}]}}]}}

        verifier.get = mock.Mock(side_effect=[observation("old"), observation("old"), observation("new")])
        with mock.patch.object(M.time, "sleep"):
            observed = verifier.until("fresh provenance", lambda: verifier.refreshed_provenance(cm, 1))
        self.assertEqual(verifier.get.call_count, 3)
        self.assertEqual(observed[0]["resourceVersion"], "new")

        verifier.get = mock.Mock(return_value=observation("old"))
        with self.assertRaisesRegex(RuntimeError, "fresh CM provenance missing"):
            verifier.refreshed_provenance(cm, 1)

    def test_cr_generation_change_is_not_reference_only_refresh(self):
        verifier = object.__new__(M.Verification)
        verifier.get = mock.Mock(return_value={"metadata": {"generation": 2}})
        with self.assertRaisesRegex(RuntimeError, "edited CR spec"):
            verifier.refreshed_provenance({}, 1)
