"""Independent synthetic tests; never consume deployment artifacts."""
import importlib.util
from pathlib import Path
import unittest
import os
import tempfile
import hashlib
import shutil
import json
import copy
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor
from unittest import mock


class InventoryTest(unittest.TestCase):
    def test_move_import_and_replacement_paths_cannot_disappear_into_scalars(self):
        import private_plan_shape as shape
        plan = dict(format_version="1.2", terraform_version="1.15.8", resource_changes=[{
            "address": "aws_instance.replacement_host", "type": "aws_eip", "mode": "data",
            "previous_address": "CANARY_ADDRESS", "change": {"actions": ["delete", "create"],
            "replace_paths": [["domain"], ["CANARY_PATH"]], "importing": {"id": "CANARY_IMPORT"}}}])
        result = shape.inventory(plan)
        self.assertTrue(any(n.get("path_step") == "domain" for n in result["nodes"]))
        self.assertTrue(any(d["category"] == "type" and d["rule"] == "resource_identity" for d in result["shape_diff"]))
        self.assertTrue(any(d.get("expected") == "action_reason" for d in result["shape_diff"]))
        self.assertTrue(any(d["rule"] == "move_import" for d in result["shape_diff"]))
        self.assertNotIn("CANARY", shape.encode(result).decode())

    def test_provider_schema_and_missing_change_fields_are_classified(self):
        import private_plan_shape as shape
        plan = dict(format_version="1.2", terraform_version="1.15.8",
                    planned_values={"root_module": {"resources": [{"address": "aws_eip.origin",
                        "type": "aws_eip", "mode": "managed", "schema_version": 999,
                        "provider_name": "CANARY_PROVIDER", "values": {}}]}},
                    resource_changes=[{"address": "aws_eip.origin", "type": "aws_eip", "mode": "managed",
                                       "change": {"actions": ["create"]}}])
        result = shape.inventory(plan)
        self.assertTrue(any(d["rule"] == "provider_schema" for d in result["shape_diff"]))
        self.assertTrue(any(d.get("expected") == "after" for d in result["shape_diff"]))
        self.assertNotIn("CANARY", shape.encode(result).decode())

    def test_register_mutations_are_closed_and_latest_invalidation_wins(self):
        import private_plan_shape_review as review
        binding = {key: "a" * 40 for key in review.COMMITS}
        binding.update(binary_plan_sha256="b" * 64, **review.TOOLS)
        event = dict(sequence=1, status="approved", binding=binding,
                     manifest_sha256=hashlib.sha256(b"manifest").hexdigest(), reviewer="reviewer-sol",
                     writer="default-orchestrator", timestamp="2026-09-26T00:00:00Z", reason="independent_review")
        base = dict(schema=review.REGISTER_SCHEMA, events=[event])
        for field, bad in (("sequence", True), ("sequence", 0), ("status", "active"),
                           ("reviewer", "worker-astra"), ("writer", "worker-astra"),
                           ("timestamp", "CANARY_TIME"), ("timestamp", "2026-99-99T00:00:00Z"),
                           ("manifest_sha256", "CANARY"), ("reason", "CANARY_REASON")):
            candidate = copy.deepcopy(base)
            candidate["events"][0][field] = bad
            with self.subTest(field=field, bad=bad), self.assertRaises(review.ShapeError):
                review.validate(candidate)
        for key in review.COMMITS | set(review.TOOLS) | {"binary_plan_sha256"}:
            candidate = copy.deepcopy(base)
            candidate["events"][0]["binding"][key] = "wrong"
            with self.subTest(binding=key), self.assertRaises(review.ShapeError):
                review.validate(candidate)
        current = dict(schema=review.REGISTER_SCHEMA,
                       events=[event, dict(event, sequence=2, status="invalidated", reason="freshness_failed")])
        self.assertEqual(review.status(current, binding, b"manifest")[0], "invalidated")
        retired_then_invalidated = dict(schema=review.REGISTER_SCHEMA, events=[
            event, dict(event, sequence=2, status="retired", reason="superseded"),
            dict(event, sequence=3, status="invalidated", reason="custody_lost")])
        self.assertEqual(review.status(retired_then_invalidated, binding, b"manifest")[0], "invalidated")
        with self.assertRaises(review.ShapeError):
            review.validate(dict(schema=review.REGISTER_SCHEMA, events=[event, event]))

    def test_manifest_mutation_and_time_node_byte_budgets(self):
        import private_plan_shape as shape
        import private_plan_shape_review as review
        binding = {key: "a" * 40 for key in review.COMMITS}
        binding.update(binary_plan_sha256="b" * 64, **review.TOOLS)
        plan = dict(format_version="1.2", terraform_version="1.15.8", diagnostics=[{"detail": "synthetic"}])
        manifest = dict(schema=shape.SCHEMA, mode="diagnostic-only", binding=binding, **shape.inventory(plan))
        for section in ("nodes", "sections", "shape_diff"):
            mutant = copy.deepcopy(manifest)
            mutant[section][0]["CANARY_EXTRA"] = "CANARY_VALUE"
            with self.subTest(section=section), self.assertRaises(shape.ShapeError):
                shape.validate_manifest(mutant)
        mutant = copy.deepcopy(manifest)
        mutant["coverage"]["visited"] += 1
        with self.assertRaises(shape.ShapeError):
            shape.validate_manifest(mutant)
        with mock.patch.object(shape, "MAX_BYTES", 4), self.assertRaises(shape.ShapeError):
            shape.parse(b'{"large":1}')
        with mock.patch.object(shape, "MAX_NODES", 1), self.assertRaises(shape.ShapeError):
            shape.inventory(plan)
        with mock.patch.object(shape.time, "monotonic", side_effect=[0, 11]), self.assertRaises(shape.ShapeError):
            shape.inventory(plan)

    def test_isolated_cli_errors_never_echo_arguments_or_paths(self):
        for name, message in (("private_plan_shape_cli.py", b"diagnostic_blocked\n"),
                              ("private_plan_shape_register.py", b"register_write_blocked\n")):
            result = subprocess.run([sys.executable, "-I", "-B", str(Path(__file__).with_name(name)),
                                     "CANARY_ARGUMENT"], capture_output=True)
            self.assertEqual(result.returncode, 1)
            self.assertEqual(result.stdout, b"")
            self.assertEqual(result.stderr, message)

    def test_filesystem_adversaries_and_concurrent_collision(self):
        import private_plan_shape_io as secure
        with tempfile.TemporaryDirectory(dir=Path(__file__).resolve().parent) as root:
            file = Path(root) / "file"
            file.write_bytes(b"synthetic")
            file.chmod(0o600)
            with secure.Directory(root) as directory:
                os.symlink(file, Path(root) / "link")
                with self.assertRaises((OSError, secure.ShapeError)):
                    directory.read("link")
                os.link(file, Path(root) / "hardlink")
                with self.assertRaises(secure.ShapeError):
                    directory.read("file")
                os.unlink(Path(root) / "hardlink")
                file.chmod(0o644)
                with self.assertRaises(secure.ShapeError):
                    directory.read("file")
                file.chmod(0o600)
                os.mkfifo(Path(root) / "fifo", 0o600)
                with self.assertRaises(secure.ShapeError):
                    directory.read("fifo")
                with self.assertRaises(secure.ShapeError):
                    directory.read("../file")
                with mock.patch.object(secure.Directory, "write_new", side_effect=OSError("synthetic_crash")):
                    with self.assertRaises(OSError):
                        secure.publish(directory, "crashed", {"manifest.json": b"{}"})
                self.assertFalse((Path(root) / "crashed").exists())
            def publish():
                try:
                    with secure.Directory(root) as directory:
                        secure.publish(directory, "concurrent", {"manifest.json": b"{}"})
                    return "published"
                except secure.ShapeError:
                    return "collision"
            with ThreadPoolExecutor(max_workers=2) as pool:
                self.assertEqual(sorted(pool.map(lambda _: publish(), range(2))), ["collision", "published"])

    def test_owner_directory_effective_acl_and_parent_substitution(self):
        import private_plan_shape_io as secure
        with tempfile.TemporaryDirectory(dir=Path(__file__).resolve().parent) as outer:
            root = Path(outer) / "private"
            root.mkdir(mode=0o700)
            with secure.Directory(str(root)) as directory:
                root.rename(Path(outer) / "old")
                root.mkdir(mode=0o700)
                with self.assertRaises(secure.ShapeError):
                    directory.unchanged()
            if sys.platform == "darwin":
                subprocess.run(["/bin/chmod", "+a", "everyone allow read", str(root)], check=True,
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                with self.assertRaises(secure.ShapeError):
                    secure.Directory(str(root))

    def test_same_shape_semantic_pairs_and_canaries_cannot_be_certified(self):
        import private_plan_shape as shape
        plan = dict(format_version="1.2", terraform_version="1.15.8", resource_changes=[{
            "address": "aws_instance.replacement_host", "type": "aws_instance", "mode": "managed",
            "change": {"actions": ["delete", "create"], "after": {"ami": "ami-synthetic-safe",
                       "user_data": "safe", "tags": {"CANARY_KEY": "CANARY_VALUE"},
                       "CANARY_FIELD": ["CANARY_PAYLOAD"]}}}], diagnostics=[{"detail": "CANARY_DIAGNOSTIC"}])
        unsafe = copy.deepcopy(plan)
        unsafe["resource_changes"][0]["change"]["after"]["ami"] = "ami-synthetic-wrong"
        unsafe["resource_changes"][0]["change"]["after"]["user_data"] = "postgres://name:CANARY_SECRET@host"
        self.assertEqual(shape.inventory(plan), shape.inventory(unsafe))
        output = shape.encode(shape.inventory(unsafe))
        self.assertNotIn(b"CANARY", output)
        self.assertNotIn(b"approval", output)
        self.assertNotIn(hashlib.sha256(b"CANARY_SECRET").hexdigest().encode(), output)

    def test_missing_required_security_structure_is_not_hidden_by_traversal_counts(self):
        import private_plan_shape as shape
        result = shape.inventory(dict(format_version="1.2", terraform_version="1.15.8",
            resource_changes=[{"address": "aws_instance.replacement_host", "type": "aws_instance",
                               "mode": "managed", "change": {"actions": ["delete", "create"],
                               "after": {"metadata_options": []}}}]))
        self.assertTrue(any(d.get("expected") == "root_block_device" for d in result["shape_diff"]))
        self.assertTrue(any(d["category"] == "cardinality" for d in result["shape_diff"]))
        self.assertEqual(result["coverage"]["source_companions"], "missing")

    def test_parser_and_walk_budgets_fail_closed(self):
        import private_plan_shape as shape
        for raw in (b'{"x":1,"x":2}', b'{} {}', b'{"x":NaN}', b'{"x":1e999}', b'\xff'):
            with self.subTest(raw=raw), self.assertRaises(shape.ShapeError):
                shape.parse(raw)
        deep = {}
        for _ in range(shape.MAX_DEPTH + 1):
            deep = {"CANARY": deep}
        with self.assertRaises(shape.ShapeError):
            shape.inventory(dict(format_version="1.2", terraform_version="1.15.8", diagnostics=deep))

    def test_unknown_masks_and_action_reason_remain_distinguishable(self):
        import private_plan_shape as shape
        plan = dict(format_version="1.2", terraform_version="1.15.8", resource_changes=[{
            "address": "aws_instance.replacement_host", "type": "aws_instance", "mode": "managed",
            "action_reason": "replace_by_request", "change": {"actions": ["delete", "create"],
                "after_unknown": {"root_block_device": [{"volume_id": True, "encrypted": False}]}}}])
        before = shape.inventory(plan)
        self.assertTrue(any(n.get("reason") == "replace_by_request" for n in before["nodes"]))
        self.assertEqual({n["mask"] for n in before["nodes"] if "mask" in n}, {"true", "false"})
        plan["resource_changes"][0]["change"]["after_unknown"]["root_block_device"][0]["encrypted"] = None
        after = shape.inventory(plan)
        self.assertTrue(any(d["category"] == "mask" for d in after["shape_diff"]))

    def test_driver_rejects_wrong_binding_without_decoder_or_writer(self):
        path = Path(__file__).with_name("private_plan_shape_cli.py")
        self.assertTrue(path.exists(), "D1 bound command interface is not implemented")
        import private_plan_shape_cli as cli
        import private_plan_shape as shape
        import private_plan_shape_review as review
        binding = {key: "0" * 40 for key in review.COMMITS}
        binding.update(binary_plan_sha256="b" * 64, **review.TOOLS)
        with self.assertRaises(shape.ShapeError):
            cli.verify_checkout(binding)
        self.assertNotIn("private_plan_shape_register", path.read_text())

    def test_isolated_register_writer_preserves_events_and_rejects_stale_sequence(self):
        path = Path(__file__).with_name("private_plan_shape_register.py")
        self.assertTrue(path.exists(), "D1 isolated writer is not implemented")
        import private_plan_shape_register as writer
        import private_plan_shape_review as review
        import private_plan_shape as shape
        binding = {key: "a" * 40 for key in review.COMMITS}
        binding.update(binary_plan_sha256="b" * 64, **review.TOOLS)
        manifest = dict(schema=shape.SCHEMA, mode="diagnostic-only", binding=binding,
                        **shape.inventory({"format_version": "1.2", "terraform_version": "1.15.8"}))
        with tempfile.TemporaryDirectory(dir=Path(__file__).resolve().parent) as root:
            manifest_path = str(Path(root) / "manifest.json")
            Path(manifest_path).write_bytes(shape.encode(manifest))
            os.chmod(manifest_path, 0o600)
            register_path = str(Path(root) / "register.json")
            writer.append(register_path, manifest_path, 0, "approved", "independent_review",
                          "reviewer-sol", "default-orchestrator", "2026-09-26T00:00:00Z")
            first = shape.parse(Path(register_path).read_bytes())
            with self.assertRaises(shape.ShapeError):
                writer.append(register_path, manifest_path, 0, "retired", "superseded",
                              "reviewer-sol", "default-orchestrator", "2026-09-26T00:01:00Z")
            writer.append(register_path, manifest_path, 1, "retired", "superseded",
                          "reviewer-sol", "default-orchestrator", "2026-09-26T00:01:00Z")
            final = shape.parse(Path(register_path).read_bytes())
            self.assertEqual(final["events"][:-1], first["events"])
            self.assertEqual(review.status(final, binding, Path(manifest_path).read_bytes())[0], "retired")

    def test_real_offline_binary_decode_and_digest_binding(self):
        path = Path(__file__).with_name("private_plan_shape_decode.py")
        self.assertTrue(path.exists(), "D1 bound offline decoder is not implemented")
        import private_plan_shape_decode as decoder
        import private_plan_shape as shape
        repository = Path(__file__).resolve().parents[1]
        provider_dir = Path(os.environ.get("D1_TEST_PROVIDER_DIR", str(repository / "infra/terraform/production/.terraform/providers")))
        self.assertTrue(provider_dir.is_dir(), "pinned offline provider installation missing; no network fallback")
        with tempfile.TemporaryDirectory(dir=Path(__file__).resolve().parent) as root:
            executable = shutil.which("terraform")
            self.assertIsNotNone(executable)
            assert executable is not None
            staged_tool = str(Path(root) / "terraform")
            shutil.copyfile(executable, staged_tool)
            os.chmod(staged_tool, 0o700)
            Path(root, "main.tf").write_text('''terraform {
  required_version = "= 1.15.8"
  required_providers {
    aws = {
      source = "hashicorp/aws"
      version = "= 6.33.0"
    }
  }
}
provider "aws" {
  region = "us-east-1"
  access_key = "synthetic"
  secret_key = "synthetic"
  skip_credentials_validation = true
  skip_requesting_account_id = true
  skip_metadata_api_check = true
  skip_region_validation = true
}
resource "aws_eip" "synthetic" {
  domain = "vpc"
}
''')
            shutil.copyfile(repository / "infra/terraform/production/.terraform.lock.hcl", Path(root, ".terraform.lock.hcl"))
            decoder.run(staged_tool, ["init", "-backend=false", "-input=false", "-lockfile=readonly",
                                     "-plugin-dir=" + str(provider_dir)], root, str(Path(root, "init.json")))
            decoder.run(staged_tool, ["plan", "-refresh=false", "-input=false", "-out=source.tfplan"],
                        root, str(Path(root, "plan.json")))
            source_copy = Path(root) / "source.tfplan"
            self.assertEqual(source_copy.stat().st_mode & 0o777, 0o600)
            digest = hashlib.sha256(source_copy.read_bytes()).hexdigest()
            result = decoder.decode(str(source_copy), digest, root, staged_tool, root)
            self.assertEqual(result["terraform_version"], "1.15.8")
            self.assertEqual(result["format_version"], "1.2")
            self.assertEqual(hashlib.sha256(source_copy.read_bytes()).hexdigest(), digest)
            with self.assertRaises(shape.ShapeError):
                decoder.decode(str(source_copy), "0" * 64, root, staged_tool, root)
            real_run = decoder.run
            def mutate(*arguments):
                real_run(*arguments)
                if arguments[1][0] == "show":
                    with source_copy.open("ab") as stream:
                        stream.write(b"synthetic mutation")
            with mock.patch.object(decoder, "run", side_effect=mutate), self.assertRaises(shape.ShapeError):
                decoder.decode(str(source_copy), digest, root, staged_tool, root)

    def test_manifest_has_closed_value_free_schema(self):
        import private_plan_shape as shape
        self.assertTrue(hasattr(shape, "validate_manifest"), "D1 closed output schema is not implemented")
        import private_plan_shape_review as review
        binding = {key: "a" * 40 for key in review.COMMITS}
        binding.update(binary_plan_sha256="b" * 64, **review.TOOLS)
        result = dict(schema=shape.SCHEMA, mode="diagnostic-only", binding=binding,
                      **shape.inventory({"format_version": "1.2", "terraform_version": "1.15.8"}))
        shape.validate_manifest(result)
        result["nodes"][0]["payload"] = "CANARY"
        with self.assertRaises(shape.ShapeError):
            shape.validate_manifest(result)

    def test_lifecycle_is_bound_and_does_not_claim_rollback_detection(self):
        path = Path(__file__).with_name("private_plan_shape_review.py")
        self.assertTrue(path.exists(), "D1 lifecycle consumer is not implemented")
        import private_plan_shape_review as review
        binding = {key: "a" * 40 for key in review.COMMITS}
        binding.update(binary_plan_sha256="b" * 64, **review.TOOLS)
        event = dict(sequence=1, status="approved", binding=binding,
                     manifest_sha256=hashlib.sha256(b"manifest").hexdigest(),
                     reviewer="reviewer-sol", writer="default-orchestrator",
                     timestamp="2026-09-26T00:00:00Z", reason="independent_review")
        old = dict(schema=review.REGISTER_SCHEMA, events=[event])
        self.assertEqual(review.status(old, binding, b"manifest"),
                         ("approved", "rollback_detection_unavailable"))
        current = dict(schema=review.REGISTER_SCHEMA,
                       events=[event, dict(event, sequence=2, status="retired", reason="superseded")])
        self.assertEqual(review.status(current, binding, b"manifest")[0], "retired")
        self.assertEqual(review.status(old, binding, b"manifest")[1], "rollback_detection_unavailable")
        self.assertEqual(review.status(current, binding, b"substituted")[0], "unreviewed")
        with self.assertRaises(review.ShapeError):
            review.status(dict(old, extra="CANARY"), binding, b"manifest")

    def test_missing_inventory_and_forbidden_action_are_reported_together(self):
        import private_plan_shape as shape
        result = shape.inventory({"format_version": "1.2", "terraform_version": "1.15.8",
            "resource_changes": [{"address": "aws_db_instance.production", "type": "aws_db_instance",
                                  "mode": "managed", "change": {"actions": ["delete"]}}]})
        self.assertTrue(any(d["category"] == "action" for d in result["shape_diff"]))
        self.assertTrue(any(d["rule"] == "required_resources" for d in result["shape_diff"]))

    def test_owner_only_atomic_publication_and_collision(self):
        path = Path(__file__).with_name("private_plan_shape_io.py")
        self.assertTrue(path.exists(), "D1 secure publication is not implemented")
        import private_plan_shape_io as secure
        with tempfile.TemporaryDirectory(dir=Path(__file__).resolve().parent) as root:
            os.chmod(root, 0o700)
            with secure.Directory(root) as directory:
                secure.publish(directory, "generation", {"manifest.json": b"{}\n"})
                with self.assertRaises(secure.ShapeError):
                    secure.publish(directory, "generation", {"manifest.json": b"changed"})
            self.assertEqual((Path(root) / "generation/manifest.json").read_bytes(), b"{}\n")
            self.assertEqual((Path(root) / "generation").stat().st_mode & 0o777, 0o700)
            self.assertEqual((Path(root) / "generation/manifest.json").stat().st_mode & 0o777, 0o600)
    def test_scalar_values_and_dynamic_names_are_not_exported(self):
        path = Path(__file__).with_name("private_plan_shape.py")
        self.assertTrue(path.exists(), "D1 structural extractor is not implemented")
        spec = importlib.util.spec_from_file_location("shape", path)
        assert spec is not None and spec.loader is not None
        shape = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(shape)
        left = {"format_version": "1.2", "terraform_version": "1.15.8",
                "variables": {"CANARY_NAME": {"value": "CANARY_VALUE"}},
                "diagnostics": [{"detail": "CANARY_DETAIL"}]}
        right = {"format_version": "1.2", "terraform_version": "1.15.8",
                 "variables": {"OTHER_NAME": {"value": "OTHER_VALUE"}},
                 "diagnostics": [{"detail": "OTHER_DETAIL"}]}
        a, b = shape.inventory(left), shape.inventory(right)
        self.assertEqual(a, b)
        self.assertNotIn("CANARY", shape.encode(a).decode())
        self.assertEqual(a["coverage"]["visited"], len(a["nodes"]))
        self.assertEqual(a["coverage"]["visited"], sum(a["coverage"][key]
                         for key in ("classified", "opaque", "unsupported")))
        self.assertTrue(a["shape_diff"])

    def test_configuration_references_are_structural_not_scalar_equality(self):
        import private_plan_shape as shape
        plan = {"format_version": "1.2", "terraform_version": "1.15.8",
                "configuration": {"root_module": {"resources": [{
                    "address": "aws_iam_instance_profile.replacement_host",
                    "type": "aws_iam_instance_profile", "mode": "managed",
                    "expressions": {"role": {"references": [
                        "aws_iam_role.replacement_host.name", "CANARY_REFERENCE"]}}
                }]}}}
        result = shape.inventory(plan)
        self.assertEqual(len(result["relations"]), 2)
        self.assertNotIn("CANARY", shape.encode(result).decode())
        self.assertEqual(result["relations"][0]["target"], "aws_iam_role.replacement_host")
        self.assertEqual(result["relations"][1]["target"], "unresolved")


if __name__ == "__main__":
    unittest.main()
