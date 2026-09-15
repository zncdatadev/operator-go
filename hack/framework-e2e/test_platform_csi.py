"""Bounded registry retry and evidence retention; no live registry access."""
import copy
import unittest
from unittest import mock

from platform_csi import PlatformInstaller
from runtime import VerificationError


class PullTests(unittest.TestCase):
    def installer(self, outcomes):
        run = mock.Mock(command_index=0)
        receipts = iter(outcomes)

        def command(argv, **kwargs):
            self.assertEqual(argv, ['docker', 'pull', '--platform=linux/arm64', 'registry/image:v1'])
            self.assertEqual(kwargs, {'timeout': 600, 'check': False})
            run.command_index += 1
            run.last_command = next(receipts)

        run.command.side_effect = command
        installer = PlatformInstaller.__new__(PlatformInstaller)
        installer.run, installer.report = run, {}
        run.write.side_effect = lambda _, report: snapshots.append(copy.deepcopy(report))
        snapshots = []
        return installer, run, snapshots

    @mock.patch('platform_csi.time.sleep')
    def test_transient_failure_preserves_attempts_then_succeeds(self, sleep):
        installer, run, snapshots = self.installer([
            {'returncode': 124, 'timed_out': True, 'stderr': 'TLS handshake timeout'},
            {'returncode': 0, 'stderr': ''},
        ])
        installer.pull_image('registry/image:v1')
        self.assertEqual(run.command.call_count, 2)
        self.assertEqual(sleep.call_args_list, [mock.call(2)])
        self.assertEqual(len(snapshots[0]['image_pull_attempts']), 1)
        attempts = snapshots[-1]['image_pull_attempts']
        self.assertEqual([v['command'] for v in attempts], ['00001', '00002'])
        self.assertEqual([v['returncode'] for v in attempts], [124, 0])
        self.assertTrue(attempts[0]['timed_out'])

    @mock.patch('platform_csi.time.sleep')
    def test_exhaustion_is_bounded_and_failure_remains_visible(self, sleep):
        installer, run, snapshots = self.installer([{'returncode': 1, 'stderr': 'registry unavailable'}] * 3)
        with self.assertRaisesRegex(VerificationError, 'exhausted 3 attempts.*registry unavailable'):
            installer.pull_image('registry/image:v1')
        self.assertEqual(run.command.call_count, 3)
        self.assertEqual(sleep.call_args_list, [mock.call(2), mock.call(4)])
        self.assertEqual(len(snapshots[-1]['image_pull_attempts']), 3)


if __name__ == '__main__':
    unittest.main()
