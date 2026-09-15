"""CRD establishment polling handles initial API state without mutating it."""
import copy
import json
import unittest
from unittest import mock

from run import Run
from runtime import VerificationError


class EstablishmentTests(unittest.TestCase):
    name = "trinoclusters.trino.kubedoop.dev"

    def observer(self, responses, duration=0):
        observer = Run.__new__(Run)
        observer.command_index = 0
        self.now = 0
        self.snapshots = []
        outcomes = iter(responses)

        def kube(*args, **kwargs):
            self.assertEqual(args[:7], ("get", "crd", self.name, "--ignore-not-found", "-o", "json",
                                       f"--request-timeout={kwargs['timeout']:.9f}s"))
            self.assertFalse(kwargs["check"])
            self.assertLessEqual(kwargs["timeout"], 60 - self.now)
            observer.command_index += 1
            code, body, error = next(outcomes)
            observer.last_command = {"returncode": code, "stderr": error}
            self.now += min(duration, kwargs["timeout"])
            return body

        observer.kube = mock.Mock(side_effect=kube)
        observer.write = lambda _, data: self.snapshots.append(copy.deepcopy(data))
        return observer

    def poll(self, observer):
        def sleep(seconds):
            self.now += seconds

        with mock.patch("run.time.monotonic", side_effect=lambda: self.now), mock.patch("run.time.sleep", side_effect=sleep):
            observer.wait_crd_established(self.name)

    def test_absent_null_and_other_conditions_wait_for_exact_established_true(self):
        values = ["", {}, {"status": None}, {"status": {"conditions": None}},
                  {"status": {"conditions": [{"type": "NamesAccepted", "status": "True"}]}},
                  {"status": {"conditions": [{"type": "Established", "status": "False", "reason": "Installing"}]}},
                  {"status": {"conditions": [{"type": "Established", "status": "True"}]}}]
        observer = self.observer([(0, json.dumps(value) if value != "" else "", "") for value in values])
        self.poll(observer)
        report = self.snapshots[-1]
        self.assertTrue(report["established"])
        self.assertEqual([a["state"] for a in report["attempts"]], ["Pending"] * 6 + ["Established"])
        self.assertEqual(observer.kube.call_count, 7)
        self.assertEqual(report["attempts"][5]["conditions"][0]["reason"], "Installing")

    def test_api_and_malformed_response_errors_are_preserved_before_success(self):
        ready = json.dumps({"status": {"conditions": [{"type": "Established", "status": "True"}]}})
        observer = self.observer([(1, "", "apiserver unavailable"), (0, "not json", ""),
                                  (0, '{"status":{"conditions":{}}}', ""), (0, ready, "")])
        self.poll(observer)
        attempts = self.snapshots[-1]["attempts"]
        self.assertEqual([a["state"] for a in attempts], ["ReadError"] * 3 + ["Established"])
        self.assertEqual(attempts[0]["error"], "apiserver unavailable")
        self.assertEqual(attempts[0]["command"], "00001")
        self.assertIn("list or null", attempts[2]["error"])

    def test_deadline_bounds_reads_and_retains_last_failure(self):
        observer = self.observer([(1, "", "connection refused")] * 6, duration=10)
        with self.assertRaisesRegex(VerificationError, "within 60 seconds.*connection refused"):
            self.poll(observer)
        self.assertEqual(self.now, 60)
        self.assertEqual(observer.kube.call_count, 6)
        self.assertEqual(observer.kube.call_args.kwargs["timeout"], 5)
        report = self.snapshots[-1]
        self.assertFalse(report["established"])
        self.assertEqual(report["elapsed_seconds"], 60)
        self.assertEqual(len(report["attempts"]), 6)


if __name__ == "__main__":
    unittest.main()
