"""The business verifier must keep one query identity across transient GET loss."""
import contextlib
import io
import json
import unittest
from unittest import mock

from lifecycle import QUERY


class QueryReplayTests(unittest.TestCase):
    def run_query(self, responses):
        calls = []

        def open_request(request, **options):
            calls.append((request.get_method(), request.full_url, options['timeout']))
            value = responses.pop(0)
            if isinstance(value, Exception):
                raise value
            return io.StringIO(json.dumps(value))

        output = io.StringIO()
        with mock.patch('sys.argv', ['query', 'SELECT count(*) FROM original']), \
                mock.patch('urllib.request.urlopen', side_effect=open_request), \
                mock.patch('time.sleep'), contextlib.redirect_stdout(output):
            exec(compile(QUERY, 'lifecycle-query', 'exec'), {})
        return calls, [json.loads(line) for line in output.getvalue().splitlines()]

    def test_timeout_replays_only_same_get_and_preserves_rows(self):
        uri = 'http://127.0.0.1:8080/v1/statement/executing/original/token/1'
        calls, events = self.run_query([
            {'id': 'original', 'nextUri': uri},
            TimeoutError('transient GET loss'),
            {'id': 'original', 'data': [[64]]},
        ])
        self.assertEqual([call[:2] for call in calls], [
            ('POST', 'http://127.0.0.1:8080/v1/statement'), ('GET', uri), ('GET', uri)])
        self.assertTrue(all(call[2] <= 10 for call in calls))
        self.assertEqual(events[-1], {'query_id': 'original', 'rows': [[64]], 'complete': True})
        self.assertEqual(sum(event.get('transfer_error') == 'TimeoutError' for event in events), 1)

    def test_uncertain_submit_is_not_repeated(self):
        with self.assertRaises(TimeoutError):
            self.run_query([TimeoutError('POST result unknown')])

    def test_server_query_failure_is_not_replaced_with_another_query(self):
        with self.assertRaises(SystemExit) as exited:
            self.run_query([{'id': 'original', 'error': {'message': 'business failure'}}])
        self.assertEqual(exited.exception.code, 1)

    def test_transport_retry_keeps_total_deadline(self):
        # Submission resolves, one GET times out, and the original total budget
        # expires. Retrying does not create a new deadline or a new query.
        with mock.patch('time.monotonic', side_effect=[0, 0, 0, 0, 0, 0, 1, 121]):
            with self.assertRaisesRegex(TimeoutError, 'total deadline'):
                self.run_query([{'id': 'original', 'nextUri': 'http://127.0.0.1/original/1'},
                                TimeoutError('GET timeout')])

    def test_changed_query_identity_fails(self):
        with self.assertRaisesRegex(RuntimeError, 'changed query identity'):
            self.run_query([{'id': 'original', 'nextUri': 'http://127.0.0.1/original/1'}, {'id': 'different'}])


if __name__ == '__main__':
    unittest.main()
