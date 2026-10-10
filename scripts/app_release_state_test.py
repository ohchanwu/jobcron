"""State-machine tests use a trusted local simulation, not provider proof."""
import copy
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest

s = importlib.util.spec_from_file_location("r", Path(__file__).with_name("app_release.py"))
assert s is not None and s.loader is not None
r = importlib.util.module_from_spec(s)
s.loader.exec_module(r)


def inputs():
    manifests = {k: {"tree": "a" * 40, "files": [{"name": "0001_initial.sql", "sha256": "b" * 64}]} for k in ("postgres", "sqlite")}
    artifacts = {k: {"source": "c" * 40, "selector": "sha256:" + v * 64, "image_id": "sha256:" + v * 64, "platform": "linux/arm64"} for k, v in (("current", "d"), ("candidate", "e"), ("recovery", "d"))}
    d = {"schema": 1, "release_id": "unit-local", "mode": "local", "target": "owned-test", **artifacts, "manifests": manifests,
         "config_sha256": "f" * 64, "operation_timeout": 1, "observe_seconds": 0.5, "sample_seconds": 0.05}
    e = {"target": d["target"], "mode": "local", "restore": {"accepted": True, "manifests": manifests, "target": d["target"], "record": "local restore test"}}
    for role in artifacts:
        e[role] = {"artifact": artifacts[role], "manifests": manifests}
        for gate in ("ci", "review", "publication"):
            e[role][gate] = {"source": artifacts[role]["source"], "result": "accepted", "kind": "local-rehearsal", "record": "trusted local simulation"}
        e[role]["publication"].update(selector=artifacts[role]["selector"], platform="linux/arm64")
    a = {"descriptor_sha256": r.digest(d), "target": d["target"], "approval_id": "local-owner", "effects": ["observe", "persist", "replace", "recovery-point", "recover"]}
    return d, e, a


class SimulatedHost:
    local = True
    state = {}
    def __init__(self, d, t, root):
        self.d = d
    def prereqs(self):
        r.require(not self.state.get("missing"), "missing native prerequisites")
    def snapshot(self):
        r.require(not self.state.get("protected_drift"), "protected mutation")
        return {"image": self.state["image"], "container": self.state["container"], "restarts": self.state.get("restarts", 0)}
    def selector(self):
        return {"selector": self.state["selector"], "version": self.state["version"]}
    def backup(self, op):
        self.state["backups"] += 1
        return {"operation_id": op, "off_host_verified": False, "dump_sha256": "1" * 64}
    def persist(self, image, before, op):
        self.state["persists"] += 1
        self.state.update(selector=image, version=op)
        if self.state.pop("lost_persist", False):
            raise r.Unknown("locally lost reply")
    def persist_runtime_env(self, image):
        r.require(image == self.state["selector"], "runtime selector mismatch")
    def replace(self):
        self.state["replacements"] += 1
        self.state.update(image=self.state["selector"], container="new-container")
        if self.state.pop("lost_replace", False):
            raise r.Unknown("locally lost reply")
    def journey(self):
        return {"authenticated_core": True, "tls": True, "scrape_or_rerate": False}


class StateTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(dir=r.custody(os.environ["TMPDIR"], directory=True))
        self.root = Path(self.temp.name)
        self.root.chmod(0o700)
        self.lock = self.root / "target-lock"
        self.lock.mkdir(mode=0o700)
        self.d, self.e, self.a = inputs()
        self.t = {"target": self.d["target"], "config_sha256": self.d["config_sha256"], "lock_dir": str(self.lock)}
        SimulatedHost.state = {"image": self.d["current"]["image_id"], "selector": self.d["current"]["selector"],
                               "container": "old-container", "version": "old-version", "backups": 0, "persists": 0, "replacements": 0}
    def tearDown(self):
        self.temp.cleanup()
    def runner(self):
        return r.Runner(self.d, self.e, self.a, self.t, self.root, host_class=SimulatedHost)
    def test_prepare_is_input_only(self):
        self.assertEqual(self.runner().prepare()["status"], "prepared-local-only")
        self.assertEqual(list(self.root.iterdir()), [self.lock])
        self.assertEqual(SimulatedHost.state["backups"], 0)
    def test_missing_evidence_and_manifest_equality(self):
        for edit in (lambda e: e["candidate"].pop("review"), lambda e: e["recovery"]["manifests"].update(sqlite={"files": []})):
            original = copy.deepcopy(self.e)
            self.e = copy.deepcopy(self.e)
            edit(self.e)
            with self.assertRaises(r.Refuse):
                self.runner()
            self.e = original
    def test_wrong_target_and_current_digest(self):
        self.t["target"] = "foreign"
        with self.assertRaises(r.Refuse):
            self.runner()
        self.t["target"] = self.d["target"]
        SimulatedHost.state["image"] = "wrong"
        with self.assertRaises(r.Refuse):
            self.runner().start()
        self.assertEqual(SimulatedHost.state["persists"], 0)
    def test_protected_drift_and_missing_native_fail_before_effect(self):
        for flag in ("protected_drift", "missing"):
            SimulatedHost.state[flag] = True
            with self.assertRaises(r.Refuse):
                self.runner().start(resume=(self.lock / "lock.json").exists())
            SimulatedHost.state.pop(flag)
        self.assertEqual(SimulatedHost.state["backups"], 0)
    def test_release_and_completed_resume_never_repeat_effect(self):
        receipt = self.runner().start()
        self.assertEqual(receipt["status"], "local-rehearsal-verified")
        self.runner().start(resume=True)
        self.assertEqual([SimulatedHost.state[k] for k in ("backups", "persists", "replacements")], [1, 1, 1])
    def test_lost_reply_each_effect_reconciles_without_replay(self):
        for flag in ("lost_persist", "lost_replace"):
            with self.subTest(flag=flag):
                with tempfile.TemporaryDirectory(dir=self.root) as directory:
                    state = Path(directory)
                    state.chmod(0o700)
                    SimulatedHost.state.update(image=self.d["current"]["image_id"], selector=self.d["current"]["selector"], container="old-container", version="old-version", backups=0, persists=0, replacements=0)
                    SimulatedHost.state[flag] = True
                    runner = r.Runner(self.d, self.e, self.a, self.t, state, host_class=SimulatedHost)
                    with self.assertRaises(r.Unknown):
                        runner.start(resume=(self.lock / "lock.json").exists())
                    runner.start(resume=True)
                    self.assertEqual([SimulatedHost.state[k] for k in ("backups", "persists", "replacements")], [1, 1, 1])
    def test_recovery_requires_separate_approval(self):
        self.a["effects"].remove("recover")
        with self.assertRaises(r.Refuse):
            self.runner().start(recovery=True)
        self.assertEqual(SimulatedHost.state["backups"], 0)
    def test_global_stop_applies_across_state_directories(self):
        r.atomic(self.lock / "STOP", {"owner_stopped": True})
        with self.assertRaises(r.Refuse):
            self.runner().start()
        self.assertEqual(SimulatedHost.state["persists"], 0)
    def test_committed_receipt_status_is_not_unknown(self):
        receipt = self.runner().start()
        self.assertTrue(all(e["status"] == "committed-verified" for e in receipt["stage_outcomes"]))

    def test_preflight_submits_no_backup_selector_or_app_effect(self):
        self.assertEqual(self.runner().preflight()["status"], "preflight-observed")
        self.assertFalse((self.root / "checkpoint.json").exists())
        self.assertEqual([SimulatedHost.state[k] for k in ("backups", "persists", "replacements")], [0, 0, 0])

    def test_only_one_proven_unsubmitted_retry(self):
        class BeforeSubmit(SimulatedHost):
            def persist(self, image, before, op):
                raise r.Unsubmitted("exec never started")
        runner = r.Runner(self.d, self.e, self.a, self.t, self.root, host_class=BeforeSubmit)
        with self.assertRaises(r.Unsubmitted):
            runner.start()
        self.assertEqual(r.read_json(self.root / "checkpoint.json")["pending"]["status"], "proven-unsubmitted")
        with self.assertRaises(r.Unsubmitted):
            runner.start(resume=True)
        with self.assertRaises(r.Refuse):
            runner.start(resume=True)
        self.assertEqual(SimulatedHost.state["backups"], 1)
        self.assertEqual(SimulatedHost.state["replacements"], 0)

    def test_noop_refused_before_recovery_point(self):
        self.d["candidate"] = self.d["current"]
        self.e["candidate"] = self.e["current"]
        self.a["descriptor_sha256"] = r.digest(self.d)
        with self.assertRaises(r.Refuse):
            self.runner().start()
        self.assertEqual(SimulatedHost.state["backups"], 0)

    def test_cli_invalid_inputs_never_echo_private_values(self):
        import subprocess
        import sys
        secret = "synthetic-private-value-not-for-output"
        descriptor = dict(self.d, forbidden=secret)
        for name, value in (("descriptor", descriptor), ("evidence", self.e), ("approvals", self.a), ("target", self.t)):
            r.atomic(self.root / (name + ".json"), value)
        args = [sys.executable, str(Path(__file__).with_name("app_release.py"))]
        for name in ("descriptor", "evidence", "approvals", "target"):
            args += ["--" + name, str(self.root / (name + ".json"))]
        args += ["--state-dir", str(self.root)]
        p = subprocess.run(args, env={"PATH": "/usr/bin:/bin", "HOME": "/nonexistent"}, stdin=subprocess.DEVNULL,
                           capture_output=True, timeout=5)
        self.assertEqual(p.returncode, 1)
        self.assertNotIn(secret.encode(), p.stdout + p.stderr)
        self.assertEqual(json.loads(p.stdout)["status"], "refused-or-unknown")


if __name__ == "__main__":
    unittest.main()
