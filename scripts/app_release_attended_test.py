"""Approach2 transition contract; simulated states are NOT native provenance."""
import copy
from pathlib import Path
from unittest.mock import patch
import sys
import unittest
import os

import app_release_recovery_test as recovery_tests
from app_release_state_test import r, SimulatedHost


def disposition(runner, action='accept-effective-state'):
    """Explicit synthetic operator input, never called by production runner."""
    cp = r.read_json(runner.checkpoint)
    state = runner.host.snapshot(allow_stopped=True)
    binding = {'target': runner.d['target'], 'descriptor_sha256': r.digest(runner.d),
               'evidence_sha256': r.digest(runner.e), 'target_sha256': r.digest(runner.t),
               'checkpoint_sha256': r.file_digest(runner.checkpoint), 'sequence': cp['sequence'],
               'operation_id': cp['pending']['operation_id'], 'artifact': cp['wanted'],
               'observed': state, 'selector': runner.selector_state()}
    evidence = dict(binding, schema=1, native_invocation_terminated=True,
                    native_effects_settled=True, exclusive_operator=True,
                    record='explicit simulated operator independent inspection; not provider proof')
    evidence_file = runner.root / 'attended-evidence.json'
    r.atomic(evidence_file, evidence)
    return dict(binding, schema=1, disposition=action, approval_id='synthetic-attended-owner',
                authority=action, evidence_file=str(evidence_file), evidence_sha256_file=r.file_digest(evidence_file))


class AttendedTests(unittest.TestCase):
    setUp = recovery_tests.RecoveryTests.setUp
    tearDown = recovery_tests.RecoveryTests.tearDown
    runner = recovery_tests.RecoveryTests.runner
    recovery = recovery_tests.RecoveryTests.recovery
    def lost(self, host=SimulatedHost):
        SimulatedHost.state['lost_replace'] = True
        runner = self.runner(host)
        with self.assertRaises(r.Unknown):
            runner.start()
        return runner

    def test_unbound_same_image_never_adopted(self):
        runner = self.lost()
        original = r.read_json(runner.checkpoint)
        for change in ({}, {'container': 'foreign'}, {'started_at': 'foreign-start'}):
            saved = copy.deepcopy(SimulatedHost.state)
            SimulatedHost.state.update(change)
            with self.assertRaises(r.Refuse):
                runner.start(resume=True)
            self.assertEqual(r.read_json(runner.checkpoint), original)
            self.assertEqual(SimulatedHost.state['replacements'], 1)
            SimulatedHost.state = saved

    def test_attended_resolution_appends_unknown_history_not_causal_proof(self):
        runner = self.lost()
        prior = r.read_bytes(runner.checkpoint)
        approval = disposition(runner)
        result = runner.start(resume=True, disposition=approval)
        self.assertEqual(result['status'], 'local-rehearsal-verified')
        cp = r.read_json(runner.checkpoint)
        effect = cp['effects'][-1]
        self.assertEqual(effect['status'], 'attended-effective-state')
        self.assertEqual(effect['original_native_result'], 'unknown')
        self.assertTrue(any(p.read_bytes() == prior for p in self.root.glob('unknown-checkpoint-*.json')))
        self.assertEqual(SimulatedHost.state['replacements'], 1)
        runner.start(resume=True)
        self.assertEqual(SimulatedHost.state['replacements'], 1)

    def test_missing_conflicting_stale_inflight_attended_input_refuses(self):
        runner = self.lost()
        good = disposition(runner)
        before = r.read_bytes(runner.checkpoint)
        for edit in ({'checkpoint_sha256': '0' * 64}, {'sequence': -1}, {'observed': {}},
                     {'selector': {}}, {'approval_id': ''}, {'authority': 'observe'},
                     {'artifact': self.d['current']}, {'target_sha256': '0' * 64}):
            with self.subTest(edit=edit), self.assertRaises(r.Refuse):
                runner.start(resume=True, disposition=dict(good, **edit))
            self.assertEqual(r.read_bytes(runner.checkpoint), before)
        path = Path(str(good['evidence_file']))
        evidence = r.read_json(path)
        for flag in ('native_invocation_terminated', 'native_effects_settled', 'exclusive_operator'):
            r.atomic(path, dict(evidence, **{flag: False}))
            bad = dict(good, evidence_sha256_file=r.file_digest(path))
            with self.assertRaises(r.Refuse):
                runner.start(resume=True, disposition=bad)
        self.assertEqual(SimulatedHost.state['replacements'], 1)

    def test_durable_pair_pending_crash_no_replay_and_drift_refusal(self):
        class CrashCommit(r.Runner):
            def commit_effect(self, pending):
                if pending['kind'] == 'replace':
                    raise r.Unknown('crash AFTER durable outcome/postimage pair')
                super().commit_effect(pending)
        runner = CrashCommit(self.d, self.e, self.a, self.t, self.root, host_class=SimulatedHost)
        with self.assertRaises(r.Unknown):
            runner.start()
        cp = r.read_json(runner.checkpoint)
        self.assertEqual(cp['pending']['native_outcome'], 'delivered-success')
        original = copy.deepcopy(SimulatedHost.state)
        for edit in ({'container': 'foreign'}, {'started_at': 'other'}, {'restarts': 1}, {'version': 'foreign'}):
            SimulatedHost.state = dict(original, **edit)
            with self.assertRaises(r.Refuse):
                self.runner().start(resume=True)
        SimulatedHost.state = original
        self.runner().start(resume=True)
        self.assertEqual(SimulatedHost.state['replacements'], 1)

    def test_success_without_first_postimage_stays_unknown(self):
        class LateReadback(SimulatedHost):
            def snapshot(self, allow_stopped=False, allow_absent=False):
                if self.state['replacements']:
                    raise r.Unsubmitted('late helper unavailable')
                return super().snapshot(allow_stopped=allow_stopped, allow_absent=allow_absent)
        with self.assertRaises(r.Unsubmitted):
            self.runner(LateReadback).start()
        cp = r.read_json(self.root / 'checkpoint.json')
        self.assertNotIn('result', cp['pending'])
        with self.assertRaises(r.Refuse):
            self.runner().start(resume=True)
        self.assertEqual(SimulatedHost.state['replacements'], 1)

    def test_each_durable_pending_pair_can_resume_without_effect_replay(self):
        for kind in ('recovery-point', 'persist', 'replace'):
            with self.subTest(kind=kind):
                class CrashPair(r.Runner):
                    def commit_effect(self, pending):
                        if pending['kind'] == kind:
                            raise r.Unknown('crash AFTER paired native completion/readback')
                        super().commit_effect(pending)
                runner = CrashPair(self.d, self.e, self.a, self.t, self.root, host_class=SimulatedHost)
                with self.assertRaises(r.Unknown):
                    runner.start()
                cp = r.read_json(runner.checkpoint)
                self.assertEqual(cp['pending']['native_outcome'], 'delivered-success')
                self.runner().start(resume=True)
                self.assertEqual([SimulatedHost.state[k] for k in ('backups', 'persists', 'replacements')], [1, 1, 1])
                self.root = self.root / 'next-pair'
                self.root.mkdir(mode=0o700)
                SimulatedHost.state.update(image=self.d['current']['image_id'], selector=self.d['current']['selector'],
                                           container='old-container', version='old-version', replacements=0, persists=0, backups=0)

    def test_each_committed_crash_window_exact_preimages(self):
        for kind in ('recovery-point', 'persist'):
            with self.subTest(kind=kind):
                class CrashStage(r.Runner):
                    def effect(self, actual, *args):
                        super().effect(actual, *args)
                        if actual == kind:
                            raise r.Unknown('committed before stage advance')
                runner = CrashStage(self.d, self.e, self.a, self.t, self.root, host_class=SimulatedHost)
                with self.assertRaises(r.Unknown):
                    runner.start()
                baseline = copy.deepcopy(SimulatedHost.state)
                for edit in ({'container': 'foreign'}, {'started_at': 'foreign'}, {'restarts': 1}, {'stopped': True}):
                    SimulatedHost.state = dict(baseline, **edit)
                    with self.assertRaises(r.Refuse):
                        self.runner().start(resume=True)
                    self.assertEqual(SimulatedHost.state['replacements'], 0)
                SimulatedHost.state = baseline
                self.runner().start(resume=True)
                self.assertEqual(SimulatedHost.state['replacements'], 1)
                # Independent next case, never delete historical checkpoint/lock.
                self.root = self.root / 'next-case'
                self.root.mkdir(mode=0o700)
                SimulatedHost.state.update(image=self.d['current']['image_id'], selector=self.d['current']['selector'],
                                           container='old-container', version='old-version', replacements=0)

    def test_changed_approvals_refuse_completed_resume(self):
        self.runner().start()
        self.a['approval_id'] = 'different-owner'
        with self.assertRaises(r.Refuse):
            self.runner().start(resume=True)

    def test_attended_stopped_candidate_allows_separate_recovery_only(self):
        runner = self.lost()
        SimulatedHost.state['stopped'] = True
        approval = disposition(runner, 'recover-effective-state')
        result = runner.start(resume=True, disposition=approval)
        self.assertEqual(result['status'], 'attended-recovery-ready')
        self.assertFalse((runner.root / 'receipt.json').exists())
        prior = r.read_bytes(runner.checkpoint)
        self.recovery().start(recovery=True, failed_state=self.root)
        self.assertEqual(r.read_bytes(runner.checkpoint), prior)
        self.assertEqual(SimulatedHost.state['replacements'], 2)

    def test_active_native_pid_and_global_stop_override_attended_assertions(self):
        runner = self.lost()
        cp = r.read_json(runner.checkpoint)
        cp['pending']['native_process'] = {'pid': os.getpid(), 'terminal': False}
        r.atomic(runner.checkpoint, cp)
        approval = disposition(runner)
        with self.assertRaises(r.Refuse):
            runner.start(resume=True, disposition=approval)
        r.atomic(self.lock / 'STOP', {'global_owner_stop': True})
        with self.assertRaises(r.Refuse):
            runner.start(resume=True, disposition=approval)
        self.assertEqual(SimulatedHost.state['replacements'], 1)

    def test_absent_and_created_unstarted_require_identified_recovery(self):
        class Intermediate(SimulatedHost):
            def snapshot(self, allow_stopped=False, allow_absent=False):
                if self.state.get('absent'):
                    r.require(allow_absent, 'absent app')
                    return {'container': None, 'image': None, 'restarts': None, 'started_at': None, 'running': False}
                return super().snapshot(allow_stopped=allow_stopped, allow_absent=allow_absent)
            def replace(self):
                super().replace()
                self.state.pop('absent', None)
        for absent in (True, False):
            with self.subTest(absent=absent):
                runner = self.lost(Intermediate)
                SimulatedHost.state.update(absent=absent, stopped=True, started_at='0001-01-01T00:00:00Z')
                with self.assertRaises(r.Refuse):
                    runner.start(resume=True)
                cp = r.read_json(runner.checkpoint)
                observed = runner.host.snapshot(allow_stopped=True, allow_absent=True)
                # Explicit synthetic inspection of this effective intermediate.
                SimulatedHost.state.pop('absent', None)
                approval = disposition(runner, 'recover-effective-state')
                approval['observed'] = observed
                evidence = r.read_json(approval['evidence_file'])
                evidence['observed'] = observed
                r.atomic(approval['evidence_file'], evidence)
                approval['evidence_sha256_file'] = r.file_digest(approval['evidence_file'])
                SimulatedHost.state['absent'] = absent
                result = runner.start(resume=True, disposition=approval)
                self.assertEqual(result['status'], 'attended-recovery-ready')
                recovery = self.recovery()
                recovery.host_class = Intermediate
                recovery.a['recovery_preimage'] = observed
                recovery.start(recovery=True, failed_state=self.root)
                self.assertEqual(SimulatedHost.state['replacements'], 2)
                self.root = self.root / 'next-intermediate'
                self.root.mkdir(mode=0o700)
                SimulatedHost.state.update(image=self.d['current']['image_id'], selector=self.d['current']['selector'],
                                           container='old-container', version='old-version', replacements=0, stopped=False)

    def test_old_app_stopped_before_replacement_uses_exact_inactive_recovery(self):
        runner = self.lost()
        SimulatedHost.state.update(image=self.d['current']['image_id'], container='old-container', stopped=True)
        observed = runner.host.snapshot(allow_stopped=True)
        result = runner.start(resume=True, disposition=disposition(runner, 'recover-effective-state'))
        self.assertEqual(result['status'], 'attended-recovery-ready')
        recovery = self.recovery()
        recovery.d['current'] = copy.deepcopy(self.d['current'])
        recovery.d['recovery'] = copy.deepcopy(self.d['current'])
        for role in ('current', 'recovery'):
            recovery.e[role]['artifact'] = recovery.d[role]
        recovery.a.update(descriptor_sha256=r.digest(recovery.d), recovery_preimage=observed)
        recovery.start(recovery=True, failed_state=self.root)
        self.assertEqual(SimulatedHost.state['image'], self.d['current']['image_id'])
        self.assertEqual(SimulatedHost.state['replacements'], 2)
        recovery.start(recovery=True, resume=True)
        self.assertEqual(SimulatedHost.state['replacements'], 2)

    def test_attended_changed_protected_target_and_missing_recovery_scope_refuse(self):
        runner = self.lost()
        approval = disposition(runner, 'recover-effective-state')
        SimulatedHost.state['protected_drift'] = True
        with self.assertRaises(r.Refuse):
            runner.start(resume=True, disposition=approval)
        SimulatedHost.state.pop('protected_drift')
        runner.start(resume=True, disposition=approval)
        recovery = self.recovery()
        recovery.a.pop('recovery_preimage')
        with self.assertRaises(r.Refuse):
            recovery.start(recovery=True, failed_state=self.root)
        self.assertEqual(SimulatedHost.state['replacements'], 1)

    def test_public_disk_secret_tmpfs_custody_and_misclassification(self):
        secret = self.root / 'secret-tmpfs'
        secret.mkdir(mode=0o700)
        for name in ('compose', 'env', 'selector', 'config', 'credentials', 'Caddyfile'):
            r.atomic(self.root / name, b'public configuration' if name == 'Caddyfile' else b'x', raw=True)
        public = self.root / 'Caddyfile'
        public.chmod(0o644)
        t = dict(self.t, project='owned', docker_host='unix:///owned.sock', compose_file=str(self.root / 'compose'),
                 compose_sha256=r.file_digest(self.root / 'compose'), compose_env=str(self.root / 'env'),
                 selector_file=str(self.root / 'selector'), secret_work_dir=str(secret), protected_paths=[str(public), str(secret)],
                 protected_public_paths=[str(public)], aws_config=str(self.root / 'config'),
                 aws_credentials=str(self.root / 'credentials'), aws_profile='operator', region='local-test',
                 compose_executable=sys.executable, database_url_file=str(self.root / 'credentials'),
                 journey_account_file=str(self.root / 'credentials'), proxy_secret_file=str(self.root / 'credentials'))
        disk_secret = False
        def run(host, args, **kwargs):
            if args[0] == 'findmnt':
                return b'ext4\n' if disk_secret or args[-1] == str(public) else b'tmpfs\n'
            if args[0] == 'swapon':
                return b''
            return r.Host.run(host, args, **kwargs)
        with patch.object(r, 'aws_inputs'), patch.object(r.Host, 'run', run):
            host = r.Host(dict(self.d, mode='production'), t, self.root)
            host.t.update(daemon_id='owned', compose_version='Docker Compose version v5.5.0')
            with patch.object(host, 'docker', return_value=b'{"ID":"owned"}'), \
                 patch.object(host, 'compose', return_value=b'Docker Compose version v5.6.0'):
                with self.assertRaisesRegex(r.Refuse, 'version drift'):
                    host.prereqs()
            before = host.mount_files()
            public.write_text('changed public config')
            self.assertNotEqual(before, host.mount_files())
            disk_secret = True
            with self.assertRaises(r.Refuse):
                r.Host(dict(self.d, mode='production'), t, self.root)
            disk_secret = False
            for bad in ([t['aws_credentials']], [str(secret)], [str(public), t['proxy_secret_file']]):
                with self.assertRaises(r.Refuse):
                    r.Host(dict(self.d, mode='production'), dict(t, protected_public_paths=bad), self.root)
