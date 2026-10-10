"""Review153 regressions: simulated transport is not native/provider proof."""
import copy
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

from app_release_state_test import r, inputs, SimulatedHost


class RecoveryTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(dir=r.custody(os.environ['TMPDIR'], directory=True))
        self.root = Path(self.tmp.name)
        self.lock = self.root / 'target-lock'
        self.lock.mkdir(mode=0o700)
        self.d, self.e, self.a = inputs()
        self.t = {'target': self.d['target'], 'config_sha256': self.d['config_sha256'], 'lock_dir': str(self.lock)}
        SimulatedHost.state = {'image': self.d['current']['image_id'], 'selector': self.d['current']['selector'],
                               'container': 'old-container', 'version': 'old-version', 'backups': 0, 'persists': 0, 'replacements': 0}

    def tearDown(self):
        self.tmp.cleanup()

    def runner(self, host=SimulatedHost):
        return r.Runner(self.d, self.e, self.a, self.t, self.root, host_class=host)

    def recovery(self):
        root = self.root / 'compatible-recovery'
        root.mkdir(mode=0o700)
        d, e = copy.deepcopy(self.d), copy.deepcopy(self.e)
        d.update(release_id='approved-recovery', current=d['candidate'])
        e['current'] = e['candidate']
        a = dict(self.a, descriptor_sha256=r.digest(d), recovery_preimage=SimulatedHost(d, self.t, root).snapshot(allow_stopped=True))
        return r.Runner(d, e, a, self.t, root, host_class=SimulatedHost)

    def test_readonly_failure_releases_lock_preserving_audit(self):
        SimulatedHost.state['missing'] = True
        with self.assertRaises(r.Refuse):
            self.runner().preflight()
        SimulatedHost.state.pop('missing')
        self.runner().preflight()
        self.assertFalse((self.root / 'checkpoint.json').exists())
        self.assertTrue(list(self.lock.glob('history-*.json')))
        self.assertEqual(SimulatedHost.state['backups'], 0)

    def test_stale_readonly_marker_reconciles_without_mutation(self):
        owner = {'state_dir': str(self.root), 'descriptor_sha256': r.digest(self.d)}
        r.atomic(self.lock / 'lock.json', {'owner': owner, 'read_only': True, 'pending': True})
        self.runner().preflight()
        self.assertEqual(SimulatedHost.state['backups'], 0)
        self.assertTrue(list(self.lock.glob('history-*.json')))

    def test_lost_replace_then_stopped_requires_attended_recovery(self):
        SimulatedHost.state['lost_replace'] = True
        with self.assertRaises(r.Unknown):
            self.runner().start()
        SimulatedHost.state['stopped'] = True
        with self.assertRaises(r.Refuse):
            self.runner().start(resume=True)
        cp = r.read_json(self.root / 'checkpoint.json')
        self.assertEqual(cp['pending']['status'], 'submitted-unknown')
        with self.assertRaises(r.Refuse):
            self.recovery().start(recovery=True, failed_state=self.root)
        self.assertEqual(SimulatedHost.state['replacements'], 1)

    def test_failed_journey_stopped_candidate_explicit_recovery(self):
        class FailedJourney(SimulatedHost):
            def journey(self):
                raise r.Refuse('synthetic acceptance failure')
        with self.assertRaises(r.Refuse):
            self.runner(FailedJourney).start()
        original = r.read_bytes(self.root / 'checkpoint.json')
        SimulatedHost.state['stopped'] = True
        recovery = self.recovery()
        with self.assertRaises(r.Refuse):
            recovery.start(recovery=True)
        receipt = recovery.start(recovery=True, failed_state=self.root)
        self.assertTrue(receipt['recovery'])
        self.assertEqual(r.read_bytes(self.root / 'checkpoint.json'), original)
        self.assertEqual(SimulatedHost.state['replacements'], 2)
        self.assertTrue((recovery.root / 'handoff.json').exists())
        with self.assertRaises(r.Refuse):
            self.runner().start(resume=True)

    def test_recovery_handoff_refuses_stop_conflict_unknown_and_no_approval(self):
        class FailedJourney(SimulatedHost):
            def journey(self):
                raise r.Refuse('synthetic failure')
        with self.assertRaises(r.Refuse):
            self.runner(FailedJourney).start()
        recovery = self.recovery()
        original = r.read_bytes(self.lock / 'lock.json')
        recovery.a['effects'] = ['observe', 'persist', 'replace', 'recovery-point']
        with self.assertRaises(r.Refuse):
            recovery.start(recovery=True, failed_state=self.root)
        recovery.a['effects'].append('recover')
        cp = r.read_json(self.root / 'checkpoint.json')
        r.atomic(self.root / 'checkpoint.json', dict(cp, pending={'kind': 'recovery-point', 'status': 'submitted-unknown'}))
        with self.assertRaises(r.Refuse):
            recovery.start(recovery=True, failed_state=self.root)
        r.atomic(self.root / 'checkpoint.json', cp)
        SimulatedHost.state['version'] = 'foreign-version'
        with self.assertRaises(r.Refuse):
            recovery.start(recovery=True, failed_state=self.root)
        self.assertEqual(r.read_bytes(self.lock / 'lock.json'), original)
        r.atomic(self.lock / 'STOP', {'global': True})
        with self.assertRaises(r.Refuse):
            recovery.start(recovery=True, failed_state=self.root)
        self.assertEqual(SimulatedHost.state['replacements'], 1)

    def test_late_helper_exec_failure_never_proves_effect_unsubmitted(self):
        for kind in ('persist', 'replace', 'backup'):
            with self.subTest(kind=kind), tempfile.TemporaryDirectory(dir=self.root) as directory:
                state = Path(directory)
                SimulatedHost.state.update(image=self.d['current']['image_id'], selector=self.d['current']['selector'],
                                           container='old-container', version='old-version', backups=0, persists=0, replacements=0)
                class LateMissing(SimulatedHost):
                    pass
                original = getattr(SimulatedHost, kind)
                def late(host, *args):
                    result = original(host, *args)
                    r.native(['/definitely/missing/local-readback'], 1)
                    return result
                setattr(LateMissing, kind, late)
                runner = r.Runner(self.d, self.e, self.a, self.t, state, host_class=LateMissing)
                with self.assertRaises(r.Unsubmitted):
                    runner.start()
                cp = r.read_json(state / 'checkpoint.json')
                self.assertEqual(cp['pending']['status'], 'submitted-unknown')
                if kind == 'backup':
                    with self.assertRaises(r.Refuse):
                        runner.start(resume=True)
                elif kind == 'persist':
                    runner.host_class = SimulatedHost
                    runner.start(resume=True)
                else:
                    with self.assertRaises(r.Refuse):
                        runner.start(resume=True)
                self.assertEqual(SimulatedHost.state[{'persist': 'persists', 'replace': 'replacements', 'backup': 'backups'}[kind]], 1)
                # A backup ambiguity leaves a durable target marker; preserve it,
                # then use another independently owned lock for the next subcase.
                self.t['lock_dir'] = str(self.root / ('lock-after-' + kind))
                Path(self.t['lock_dir']).mkdir(mode=0o700)

    def test_completed_resume_refuses_each_runtime_or_selector_drift(self):
        self.runner().start()
        state = copy.deepcopy(SimulatedHost.state)
        for change in ({'container': 'foreign'}, {'restarts': 2}, {'started_at': 'different'},
                       {'version': 'foreign'}, {'protected_drift': True}):
            SimulatedHost.state = dict(state, **change)
            with self.assertRaises(r.Refuse):
                self.runner().start(resume=True)
        SimulatedHost.state = state
        result = self.runner().start(resume=True)
        self.assertIn('resume_readback', result)
        self.assertEqual(SimulatedHost.state['replacements'], 1)

    def test_committed_persist_crash_window_rejects_unrelated_version(self):
        self.runner().start()
        cp = r.read_json(self.root / 'checkpoint.json')
        cp.update(stage='persist', effects=[x for x in cp['effects'] if x['kind'] != 'replace'])
        SimulatedHost.state.update(image=self.d['current']['image_id'], container='old-container', version='unrelated')
        r.atomic(self.root / 'checkpoint.json', cp)
        with self.assertRaises(r.Refuse):
            self.runner().start(resume=True)
        self.assertEqual(SimulatedHost.state['replacements'], 1)

    def test_pre_submit_failures_one_retry_for_each_effect(self):
        for kind in ('persist', 'replace', 'backup'):
            with self.subTest(kind=kind), tempfile.TemporaryDirectory(dir=self.root) as directory:
                state = Path(directory)
                SimulatedHost.state.update(image=self.d['current']['image_id'], selector=self.d['current']['selector'],
                                           container='old-container', version='old-version', backups=0, persists=0, replacements=0)
                class BeforeSubmission(SimulatedHost):
                    pass
                def missing(host, *args):
                    host.submission(lambda: r.native(['/definitely/missing/native-mutator'], 1))
                setattr(BeforeSubmission, kind, missing)
                runner = r.Runner(self.d, self.e, self.a, self.t, state, host_class=BeforeSubmission)
                with self.assertRaises(r.Unsubmitted):
                    runner.start()
                self.assertEqual(r.read_json(state / 'checkpoint.json')['pending']['status'], 'proven-unsubmitted')
                runner.host = None
                runner.host_class = SimulatedHost
                runner.start(resume=True)
                self.assertEqual([SimulatedHost.state[k] for k in ('backups', 'persists', 'replacements')], [1, 1, 1])

    def test_committed_and_completed_protected_secret_drift_refused(self):
        self.runner().start()
        cp = r.read_json(self.root / 'checkpoint.json')
        class SecretTransport(SimulatedHost):
            local = False
            def selector(self):
                return dict(super().selector(), secret={'JOBCRON_IMAGE': self.state['selector'], 'SECRET': 'changed-synthetic'})
        self.e['secret_nonimage_sha256'] = r.digest({'SECRET': 'original-synthetic'})
        cp['evidence_sha256'] = r.digest(self.e)
        for stage in ('complete', 'persist'):
            checkpoint = dict(cp, stage=stage)
            if stage == 'persist':
                checkpoint['effects'] = [e for e in cp['effects'] if e['kind'] != 'replace']
            r.atomic(self.root / 'checkpoint.json', checkpoint)
            with self.assertRaises(r.Refuse):
                self.runner(SecretTransport).start(resume=True)
        self.assertEqual(SimulatedHost.state['replacements'], 1)

    def test_production_secret_stdout_uses_verified_workspace_not_state(self):
        secret = self.root / 'secret-tmpfs'
        secret.mkdir(mode=0o700)
        for name in ('compose', 'env', 'selector', 'config', 'credentials'):
            r.atomic(self.root / name, b'x', raw=True)
        t = dict(self.t, project='owned', docker_host='unix:///owned.sock', compose_file=str(self.root / 'compose'),
                 compose_sha256=r.file_digest(self.root / 'compose'), compose_env=str(self.root / 'env'),
                 selector_file=str(self.root / 'selector'), secret_work_dir=str(secret), protected_paths=[],
                 aws_config=str(self.root / 'config'), aws_credentials=str(self.root / 'credentials'),
                 aws_profile='operator', region='local-test', compose_executable=sys.executable)
        t.update(database_url_file=str(self.root / 'credentials'), journey_account_file=str(self.root / 'credentials'),
                 proxy_secret_file=str(self.root / 'credentials'))
        spools = []
        actual_spool = r.tempfile.TemporaryFile
        def spool(*args, **kwargs):
            spools.append(Path(kwargs['dir']))
            return actual_spool(*args, **kwargs)
        def run(host, args, **kwargs):
            if args[0] == 'findmnt':
                return b'tmpfs\n'
            if args[0] == 'swapon':
                return b''
            return r.Host.run(host, args, **kwargs)
        with patch.object(r, 'aws_inputs'), patch.object(r.Host, 'run', run):
            h = r.Host(dict(self.d, mode='production'), t, self.root)
        with patch.object(r.tempfile, 'TemporaryFile', spool):
            h.run([sys.executable, '-c', 'print("synthetic-secret-response")'])
        self.assertEqual(spools, [])  # Bounded pipes, no secret spool on any disk.
        self.assertNotEqual(h.env['TMPDIR'], str(self.root))
        with patch.object(r, 'aws_inputs'), patch.object(r.Host, 'run', return_value=b'ext4\n'):
            with self.assertRaises(r.Refuse):
                r.Host(dict(self.d, mode='production'), t, self.root)


if __name__ == '__main__':
    unittest.main()
