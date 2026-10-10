"""Offline fail-closed checks. Native rehearsal is a separate explicit command."""
import copy
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
import sys

spec = importlib.util.spec_from_file_location("app_release", Path(__file__).with_name("app_release.py"))
assert spec is not None and spec.loader is not None
r = importlib.util.module_from_spec(spec)
spec.loader.exec_module(r)


class Contracts(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(dir=r.custody(os.environ["TMPDIR"], directory=True))
        self.root = Path(self.tmp.name)
        self.root.chmod(0o700)

    def tearDown(self):
        self.tmp.cleanup()

    def put(self, name, value):
        p = self.root / name
        r.atomic(p, value)
        return p

    def test_custody(self):
        p = self.put("input", {"ok": True})
        self.assertEqual(r.read_json(p), {"ok": True})
        p.chmod(0o644)
        with self.assertRaises(r.Refuse):
            r.read_json(p)
        p.chmod(0o600)
        link = self.root / "link"
        link.symlink_to(p)
        with self.assertRaises(r.Refuse):
            r.read_json(link)
        p.write_text('{"ok":1,"ok":2}')
        with self.assertRaises(r.Refuse):
            r.read_json(p)

    def test_lock_concurrent_and_stale(self):
        with r.TargetLock(self.root):
            with self.assertRaises(r.Refuse):
                with r.TargetLock(self.root):
                    pass
        self.put("lock.json", {"pid": 99999999, "pending": True})
        # An old advisory record is not deleted on a TTL; state must reconcile.
        with self.assertRaises(r.Refuse):
            with r.TargetLock(self.root):
                pass
        with r.TargetLock(self.root, reconcile=True):
            self.assertTrue((self.root / "lock.json").exists())

    def test_immutable_selector(self):
        for value in ("app:latest", "ghcr.io/acme/jobcron:abc", "sha256:short", "x@sha256:" + "A" * 64):
            with self.assertRaises(r.Refuse):
                r.image_selector(value, "local")
        r.image_selector("sha256:" + "a" * 64, "local")
        r.image_selector("ghcr.io/acme/jobcron@sha256:" + "b" * 64, "production")
        with self.assertRaises(r.Refuse):
            r.image_selector("sha256:" + "a" * 64, "production")

    def test_manifest_complete_and_ledger(self):
        m = {"postgres": {"tree": "a" * 40, "files": [{"name": "0001_initial.sql", "sha256": "b" * 64}]},
             "sqlite": {"tree": "c" * 40, "files": [{"name": "0001_initial.sql", "sha256": "d" * 64}]}}
        r.validate_manifests(m)
        r.validate_ledger(m, [{"version": 1, "name": "0001_initial.sql", "sha256": "b" * 64}])
        for rows in ([], [{"version": 1, "name": "0001_initial.sql", "sha256": "e" * 64}],
                     [{"version": 1, "name": "0001_initial.sql", "sha256": "b" * 64}, {"version": 2}]):
            with self.assertRaises(r.Refuse):
                r.validate_ledger(m, rows)
        changed = copy.deepcopy(m)
        changed["sqlite"]["files"] = []
        with self.assertRaises(r.Refuse):
            r.validate_manifests(changed)

    def test_approval_is_independent_and_exact(self):
        d = {"target": "owned", "mode": "local"}
        a = {"descriptor_sha256": r.digest(d), "target": "owned", "approval_id": "operator-1",
             "effects": ["observe", "persist", "replace", "recovery-point"]}
        r.approve(d, a, "replace")
        for change in ({"target": "foreign"}, {"effects": []}, {"descriptor_sha256": "0" * 64}):
            bad = dict(a, **change)
            with self.assertRaises(r.Refuse):
                r.approve(d, bad, "replace")
        with self.assertRaises(r.Refuse):
            r.approve(d, a, "recover")

    def test_ambiguity_refuses_preimage_and_conflict(self):
        pending = {"kind": "replace", "before": {"container": "old"}, "after": "sha256:" + "a" * 64}
        state = {"container": "new", "image": pending["after"]}
        with self.assertRaises(r.Refuse):
            r.reconcile_effect(pending, state)
        pending.update(native_outcome="delivered-success", result=state)
        self.assertEqual(r.reconcile_effect(pending, state), "committed")
        for observed in ({"container": "old", "image": "sha256:" + "b" * 64}, {},
                         {"container": "other", "image": "sha256:" + "c" * 64}):
            with self.assertRaises(r.Refuse):
                r.reconcile_effect(pending, observed)
        # Selector persistence has its own authoritative version, not just runtime image.
        p = {"kind": "persist", "before": {"version": "v1"}, "after": pending["after"]}
        self.assertEqual(r.reconcile_effect(p, {"selector": pending["after"], "version": "v2"}), "committed")
        with self.assertRaises(r.Refuse):
            r.reconcile_effect(p, {"selector": pending["after"], "version": "v1"})

    def test_subprocess_timeout_and_missing_native(self):
        with self.assertRaises(r.Unknown):
            r.native(["/usr/bin/python3", "-c", "import time; time.sleep(2)"], timeout=0.05)
        with self.assertRaises(r.Unsubmitted):
            r.native(["/definitely/missing/jobcron-native"], timeout=1)

    def test_native_result_bound_and_explicit_output_stream(self):
        with self.assertRaises(r.Refuse):
            r.native([sys.executable, "-c", "import sys; sys.stdout.buffer.write(b'x'*(8*1024*1024+1))"], 5)
        path = self.root / "owned-output"
        with path.open("wb") as stream:
            self.assertEqual(r.native([sys.executable, "-c", "import sys; sys.stdout.write(sys.stdin.read())"], 5,
                                     data=b"synthetic", output=stream), b"")
        self.assertEqual(path.read_bytes(), b"synthetic")

    def test_stale_target_intent_cannot_be_cleared_by_another_state(self):
        lock = self.root / "target"
        lock.mkdir(mode=0o700)
        with self.assertRaises(r.Unknown):
            with r.TargetLock(lock, owner={"state_dir": "owned-first", "descriptor_sha256": "a" * 64}):
                raise r.Unknown("submitted local simulation")
        original = r.read_json(lock / "lock.json")
        with self.assertRaises(r.Refuse):
            with r.TargetLock(lock, reconcile=True, owner={"state_dir": "owned-other", "descriptor_sha256": "b" * 64}):
                self.fail("wrong-state resume acquired target")
        self.assertEqual(r.read_json(lock / "lock.json"), original)
        with r.TargetLock(lock, reconcile=True, owner=original["owner"]):
            pass

    def test_production_observation_cannot_shorten(self):
        r.observation_policy({"mode": "local", "observe_seconds": 0.1, "sample_seconds": 0.05})
        for d in ({"mode": "production", "observe_seconds": 1799, "sample_seconds": 60},
                  {"mode": "production", "observe_seconds": 1800, "sample_seconds": 1800}):
            with self.assertRaises(r.Refuse):
                r.observation_policy(d)
        r.observation_policy({"mode": "production", "observe_seconds": 1800, "sample_seconds": 60})

    def test_docker_metadata_order_and_protected_mutation(self):
        mounts = [{"Destination": "/b", "Source": "/private/b", "RW": False},
                  {"Destination": "/a", "Source": "/private/a", "RW": False}]
        self.assertEqual(r.metadata({"Mounts": mounts}, ("Mounts",)),
                         r.metadata({"Mounts": list(reversed(mounts))}, ("Mounts",)))
        changed = copy.deepcopy(mounts)
        changed[0]["RW"] = True
        self.assertNotEqual(r.metadata({"Mounts": mounts}, ("Mounts",)),
                            r.metadata({"Mounts": changed}, ("Mounts",)))
        self.assertEqual(r.environment(["B=2", "A=1"]), r.environment(["A=1", "B=2"]))
        self.assertNotEqual(r.environment(["A=1"]), r.environment(["A=2"]))
        with self.assertRaises(r.Refuse):
            r.environment(["A=1", "A=2"])

    def test_api_mount_order_preserves_all_options_and_rejects_duplicates(self):
        mounts = [{"Target": "/b", "Source": "/owned/b", "ReadOnly": True, "BindOptions": {"CreateMountpoint": False}},
                  {"Target": "/a", "Source": "/owned/a", "ReadOnly": True}]
        before = {"HostConfig": {"Mounts": mounts, "Privileged": False}}
        after = {"HostConfig": {"Mounts": list(reversed(mounts)), "Privileged": False}}
        self.assertEqual(r.metadata(before, ("HostConfig",)), r.metadata(after, ("HostConfig",)))
        mutated = copy.deepcopy(after)
        mutated["HostConfig"]["Mounts"][1]["BindOptions"]["CreateMountpoint"] = True
        self.assertNotEqual(r.metadata(before, ("HostConfig",)), r.metadata(mutated, ("HostConfig",)))
        self.assertEqual(before["HostConfig"]["Mounts"], mounts)
        with self.assertRaises(r.Refuse):
            r.metadata({"HostConfig": {"Mounts": [mounts[0], mounts[0]]}}, ("HostConfig",))

    def test_subprocess_input_routes_are_not_inherited(self):
        from unittest.mock import patch
        with patch.dict(os.environ, {"AWS_PROFILE": "foreign", "PGHOST": "foreign", "DOCKER_CONTEXT": "foreign",
                                     "PYTHONPATH": "/foreign", "JOBCRON_TEST_POSTGRES_URL": "foreign"}):
            result = r.native([sys.executable, "-c", "import os,sys,json; print(json.dumps({'keys':sorted(os.environ),'stdin':sys.stdin.read()}))"], 5)
        value = json.loads(result)
        self.assertEqual(value["stdin"], "")
        self.assertFalse(set(value["keys"]) & {"AWS_PROFILE", "PGHOST", "DOCKER_CONTEXT", "PYTHONPATH", "JOBCRON_TEST_POSTGRES_URL"})

    def test_static_aws_config_rejects_hidden_execution_and_routes(self):
        config = self.root / "aws-config"
        credentials = self.root / "aws-credentials"
        r.atomic(config, b"[profile operator]\nregion=ap-northeast-2\n", raw=True)
        r.atomic(credentials, b"[operator]\naws_access_key_id=synthetic\naws_secret_access_key=synthetic\naws_session_token=synthetic\n", raw=True)
        target = {"aws_config": str(config), "aws_credentials": str(credentials), "aws_profile": "operator", "region": "ap-northeast-2"}
        r.aws_inputs(target)
        for line in ("credential_process=/bin/false", "role_arn=foreign", "endpoint_url=https://foreign.invalid", "services=foreign"):
            r.atomic(config, ("[profile operator]\nregion=ap-northeast-2\n" + line + "\n").encode(), raw=True)
            with self.assertRaises(r.Refuse):
                r.aws_inputs(target)

    def test_hardlinks_and_relative_inputs_refused(self):
        p = self.put("single-link", {"ok": True})
        os.link(p, self.root / "hardlink")
        with self.assertRaises(r.Refuse):
            r.read_json(p)
        with self.assertRaises(r.Refuse):
            r.custody("relative")

    def test_production_selector_shape_preserves_every_other_field(self):
        # Trusted local transport simulation ONLY, not AWS permission proof.
        h = object.__new__(r.Host)
        h.local, h.root, h.secret_work = False, self.root, self.root
        h.t = {"secret_id": "synthetic-owned-secret"}
        old = {"JOBCRON_IMAGE": "ghcr.io/acme/jobcron@sha256:" + "a" * 64,
               "JOBCRON_STAGE1_SPONSOR_USER_ID": "1", "SESSION_SECRET": "synthetic-only"}
        state = {"selector": old["JOBCRON_IMAGE"], "version": "original", "secret": old}
        h.evidence_secret_digest = r.digest({k: v for k, v in old.items() if k != "JOBCRON_IMAGE"})
        h.selector = lambda: copy.deepcopy(state)
        calls = []
        def aws(*args):
            calls.append(args)
            self.assertEqual(args[:2], ("secretsmanager", "put-secret-value"))
            self.assertEqual(args[2:4], ("--secret-id", "synthetic-owned-secret"))
            payload = r.read_json(args[-1].removeprefix("file://"))
            self.assertEqual({k: v for k, v in payload.items() if k != "JOBCRON_IMAGE"},
                             {k: v for k, v in old.items() if k != "JOBCRON_IMAGE"})
            state.update(selector=payload["JOBCRON_IMAGE"], version=args[5], secret=payload)
        h.aws = aws
        persisted = []
        h.persist_runtime_env = lambda image: persisted.append(image)
        image = "ghcr.io/acme/jobcron@sha256:" + "b" * 64
        h.persist(image, state.copy(), "exact-native-operation")
        self.assertEqual(len(calls), 1)
        self.assertEqual(persisted, [image])
        self.assertFalse((self.root / "selector-payload.json").exists())


if __name__ == "__main__":
    unittest.main()
