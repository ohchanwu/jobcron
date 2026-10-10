"""Review164 regressions: offline state/transport, not native or production proof."""
import copy
import os
from unittest.mock import patch
import unittest

import app_release_recovery_test as recovery_tests
from app_release_state_test import r, SimulatedHost


class CompletionTests(unittest.TestCase):
    setUp = recovery_tests.RecoveryTests.setUp
    tearDown = recovery_tests.RecoveryTests.tearDown
    runner = recovery_tests.RecoveryTests.runner
    recovery = recovery_tests.RecoveryTests.recovery

    def pending_pair(self):
        class CrashPair(r.Runner):
            def commit_effect(self, pending):
                if pending['kind'] == 'replace':
                    raise r.Unknown('interrupt after durable success/postimage pair')
                super().commit_effect(pending)
        runner = CrashPair(self.d, self.e, self.a, self.t, self.root, host_class=SimulatedHost)
        with self.assertRaises(r.Unknown):
            runner.start()
        return runner

    def recover_pair(self, change):
        runner = self.pending_pair()
        original = r.read_bytes(runner.checkpoint)
        SimulatedHost.state.update(change)
        with self.assertRaises(r.Refuse):
            self.runner().start(resume=True)
        self.assertEqual(r.read_bytes(runner.checkpoint), original)
        recovery = self.recovery()
        receipt = recovery.start(recovery=True, failed_state=self.root)
        self.assertTrue(receipt['recovery'])
        self.assertEqual(SimulatedHost.state['replacements'], 2)
        self.assertEqual(r.read_bytes(runner.checkpoint), original)
        handoff = r.read_json(recovery.root / 'handoff.json')
        self.assertEqual(handoff['original_pending'], r.read_json(runner.checkpoint)['pending'])
        self.assertTrue(handoff['failed_acceptance_not_promoted'])
        with self.assertRaises(r.Refuse):
            self.runner().start(resume=True)
        recovery.start(recovery=True, resume=True)
        self.assertEqual(SimulatedHost.state['replacements'], 2)

    def test_pending_pair_stopped_explicit_same_lock_recovery(self):
        self.recover_pair({'stopped': True})

    def test_pending_pair_restarted_explicit_same_lock_recovery(self):
        self.recover_pair({'restarts': 1, 'started_at': 'restarted-same-container'})

    def test_pending_pair_recovery_refuses_foreign_protected_selector_or_authority(self):
        runner = self.pending_pair()
        original = r.read_bytes(runner.checkpoint)
        baseline = copy.deepcopy(SimulatedHost.state)
        SimulatedHost.state['stopped'] = True
        recovery = self.recovery()
        marker = r.read_bytes(self.lock / 'lock.json')
        for edit in ({'container': 'foreign'}, {'protected_drift': True}, {'version': 'foreign'}):
            SimulatedHost.state = dict(baseline, stopped=True, **edit)
            # Even freshly scoped authority cannot waive a foreign/protected state.
            recovery.a['recovery_preimage'] = dict(baseline, **edit)
            with self.assertRaises(r.Refuse):
                recovery.start(recovery=True, failed_state=self.root)
            self.assertEqual(r.read_bytes(self.lock / 'lock.json'), marker)
        SimulatedHost.state = dict(baseline, stopped=True)
        recovery.a['recovery_preimage'] = recovery.host.snapshot(allow_stopped=True)
        for edit in ({'recovery_preimage': {}}, {'effects': ['observe', 'persist', 'replace', 'recovery-point']}):
            saved = copy.deepcopy(recovery.a)
            recovery.a.update(edit)
            with self.assertRaises(r.Refuse):
                recovery.start(recovery=True, failed_state=self.root)
            recovery.a = saved
        self.assertEqual(r.read_bytes(runner.checkpoint), original)
        self.assertEqual(SimulatedHost.state['replacements'], 1)

    def test_pending_pair_recovery_refuses_live_or_nonterminal_native_process(self):
        runner = self.pending_pair()
        cp = r.read_json(runner.checkpoint)
        SimulatedHost.state['stopped'] = True
        recovery = self.recovery()
        for process in ({'pid': os.getpid(), 'terminal': True, 'exit_code': 0},
                        {'pid': 99999999, 'terminal': False},
                        {'pid': 99999999, 'terminal': True, 'exit_code': 1}):
            changed = copy.deepcopy(cp)
            changed['pending']['native_process'] = process
            r.atomic(runner.checkpoint, changed)
            with self.assertRaises(r.Refuse):
                recovery.start(recovery=True, failed_state=self.root)
        self.assertEqual(SimulatedHost.state['replacements'], 1)

    def test_pending_pair_recovery_refuses_stop_and_concurrent_owner(self):
        self.pending_pair()
        SimulatedHost.state['stopped'] = True
        recovery = self.recovery()
        with r.TargetLock(self.lock, reconcile=True, owner=r.read_json(self.lock / 'lock.json')['owner']):
            with self.assertRaises(r.Refuse):
                recovery.start(recovery=True, failed_state=self.root)

        r.atomic(self.lock / 'STOP', {'global_owner_stop': True})
        with self.assertRaises(r.Refuse):
            recovery.start(recovery=True, failed_state=self.root)
        self.assertEqual(SimulatedHost.state['replacements'], 1)

    def test_unbound_pair_cannot_use_direct_recovery(self):
        SimulatedHost.state['lost_replace'] = True
        with self.assertRaises(r.Unknown):
            self.runner().start()
        SimulatedHost.state['stopped'] = True
        with self.assertRaises(r.Refuse):
            self.recovery().start(recovery=True, failed_state=self.root)
        self.assertEqual(SimulatedHost.state['replacements'], 1)

    def test_complete_receipt_binding_and_unchanged_no_replay(self):
        result = self.runner().start()
        cp = r.read_json(self.root / 'checkpoint.json')
        self.assertEqual(cp['receipt_sha256'], r.digest(result))
        before = r.read_bytes(self.root / 'receipt.json')
        resumed = self.runner().start(resume=True)
        self.assertTrue(resumed['resume_readback']['historical_observation_not_restarted'])
        self.assertEqual(r.read_bytes(self.root / 'receipt.json'), before)
        self.assertEqual(SimulatedHost.state['replacements'], 1)

    def test_final_recorded_sample_covers_boundary_not_a_later_clock_read(self):
        clock = iter(i / 25 for i in range(1, 100))
        with patch.object(r.time, 'monotonic', side_effect=lambda: next(clock)), patch.object(r.time, 'sleep'):
            result = self.runner().start()
        cp = r.read_json(self.root / 'checkpoint.json')
        self.assertGreaterEqual(cp['samples'][-1]['elapsed'], self.d['observe_seconds'])
        self.assertGreaterEqual(result['samples'], 3)

    def test_completed_receipt_mutations_refuse_without_history_regeneration(self):
        result = self.runner().start()
        checkpoint = r.read_bytes(self.root / 'checkpoint.json')
        changes = ({'release_id': 'unrelated-release'}, {'descriptor_sha256': '0' * 64},
                   {'source': '0' * 40}, {'image_id': self.d['current']['image_id']},
                   {'platform': 'linux/amd64'}, {'container': 'unrelated-container'},
                   {'approval_id': 'unrelated-approval'}, {'approved_effects': ['observe']},
                   {'stage_outcomes': []}, {'observation_seconds': 0}, {'journey': {}},
                   {'samples': 1}, {'unexpected_restarts': 2}, {'exact_ledger': False},
                   {'status': 'production-verified'}, {'recovery': True})
        for change in changes:
            bad = dict(result, **change)
            r.atomic(self.root / 'receipt.json', bad)
            with self.subTest(change=change), self.assertRaises(r.Refuse):
                self.runner().start(resume=True)
            self.assertEqual(r.read_json(self.root / 'receipt.json'), bad)
            self.assertEqual(r.read_bytes(self.root / 'checkpoint.json'), checkpoint)
        self.assertEqual(SimulatedHost.state['replacements'], 1)

    def test_receipt_semantics_refuse_even_with_matching_durable_digest(self):
        result = self.runner().start()
        cp = r.read_json(self.root / 'checkpoint.json')
        for change in ({'source': '0' * 40}, {'approved_effects': ['observe']},
                       {'observation_seconds': 0}, {'container': 'foreign'}):
            bad = dict(result, **change)
            r.atomic(self.root / 'receipt.json', bad)
            r.atomic(self.root / 'checkpoint.json', dict(cp, receipt_sha256=r.digest(bad)))
            with self.assertRaises(r.Refuse):
                self.runner().start(resume=True)
        self.assertEqual(SimulatedHost.state['replacements'], 1)

    def test_copied_receipt_and_missing_binding_refuse(self):
        result = self.runner().start()
        cp = r.read_json(self.root / 'checkpoint.json')
        unrelated = dict(result, release_id='copied-other-release', container='other-container')
        r.atomic(self.root / 'receipt.json', unrelated)
        with self.assertRaises(r.Refuse):
            self.runner().start(resume=True)
        r.atomic(self.root / 'receipt.json', result)
        cp.pop('receipt_sha256')
        r.atomic(self.root / 'checkpoint.json', cp)
        with self.assertRaises(r.Refuse):
            self.runner().start(resume=True)

    def test_missing_completed_receipt_refuses_without_recreating(self):
        self.runner().start()
        checkpoint = r.read_bytes(self.root / 'checkpoint.json')
        (self.root / 'receipt.json').unlink()
        with self.assertRaises(r.Refuse):
            self.runner().start(resume=True)
        self.assertFalse((self.root / 'receipt.json').exists())
        self.assertEqual(r.read_bytes(self.root / 'checkpoint.json'), checkpoint)
        self.assertEqual(SimulatedHost.state['replacements'], 1)

    def publication_window(self, delivered):
        actual_atomic = r.atomic
        def interrupt(path, value, raw=False):
            if path == self.root / 'receipt.json':
                if delivered:
                    actual_atomic(path, value, raw=raw)
                raise r.Unknown('publication interrupted')
            return actual_atomic(path, value, raw=raw)
        with patch.object(r, 'atomic', interrupt), self.assertRaises(r.Unknown):
            self.runner().start()
        return r.read_bytes(self.root / 'checkpoint.json')

    def test_receipt_published_before_complete_resume_validates_without_reobservation(self):
        self.publication_window(True)
        cp = r.read_json(self.root / 'checkpoint.json')
        self.assertEqual(cp['stage'], 'receipt-ready')
        samples = copy.deepcopy(cp['samples'])
        result = self.runner().start(resume=True)
        self.assertEqual(result['status'], 'local-rehearsal-verified')
        self.assertEqual(r.read_json(self.root / 'checkpoint.json')['samples'], samples)
        self.assertEqual(SimulatedHost.state['replacements'], 1)

    def test_receipt_not_published_refuses_preserving_ready_evidence(self):
        checkpoint = self.publication_window(False)
        with self.assertRaises(r.Refuse):
            self.runner().start(resume=True)
        self.assertEqual(r.read_bytes(self.root / 'checkpoint.json'), checkpoint)
        self.assertFalse((self.root / 'receipt.json').exists())
        self.assertEqual(SimulatedHost.state['replacements'], 1)

    def test_conflicting_receipt_ready_refuses_and_can_recover_failed_same_container(self):
        self.publication_window(True)
        result = r.read_json(self.root / 'receipt.json')
        r.atomic(self.root / 'receipt.json', dict(result, container='foreign'))
        checkpoint = r.read_bytes(self.root / 'checkpoint.json')
        with self.assertRaises(r.Refuse):
            self.runner().start(resume=True)
        self.assertEqual(r.read_bytes(self.root / 'checkpoint.json'), checkpoint)
        SimulatedHost.state['stopped'] = True
        self.recovery().start(recovery=True, failed_state=self.root)
        self.assertEqual(r.read_bytes(self.root / 'checkpoint.json'), checkpoint)
        self.assertEqual(SimulatedHost.state['replacements'], 2)


if __name__ == '__main__':
    unittest.main()
