"""Attended D1 diagnostic entry point. No semantic-checker or approval writer.

Invoke with an isolated trusted Python interpreter (-I -B); CLI startup
adds only this reviewed scripts directory to sys.path. All paths
are private command inputs; every public outcome is a fixed value-free code.
"""
import os
from pathlib import Path
import re
import subprocess
import sys

if __name__ == "__main__":
    sys.path.insert(0, str(Path(__file__).resolve().parent))

from private_plan_shape import MAX_BYTES, SCHEMA, ShapeError, encode, inventory, parse, require, validate_manifest
from private_plan_shape_decode import decode
from private_plan_shape_io import Directory, publish, read_private
from private_plan_shape_review import binding_valid, status

RELEASE = "e8e1053319ce03d75f5ac3e0c55e9c122dfec1e9"
CHECKER = "9963a4cec8a5e91977bf7802bb915c583a67f3f4"
EXTRACTOR_FILES = ("private_plan_shape.py", "private_plan_shape_io.py", "private_plan_shape_review.py",
                   "private_plan_shape_decode.py", "private_plan_shape_cli.py")
GIT_ENV = {"PATH": "/usr/bin:/bin", "HOME": "/dev/null", "GIT_CONFIG_NOSYSTEM": "1",
           "GIT_CONFIG_SYSTEM": "/dev/null", "GIT_CONFIG_GLOBAL": "/dev/null",
           "GIT_NO_REPLACE_OBJECTS": "1", "GIT_NO_LAZY_FETCH": "1"}


def git(root, *arguments):
    result = subprocess.run(["/usr/bin/git", "-c", "core.hooksPath=/dev/null",
                             "-c", "core.attributesFile=/dev/null", "-c", "core.excludesFile=/dev/null",
                             "-c", "core.fsmonitor=false", "-C", str(root), *arguments],
                            env=GIT_ENV, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                            stderr=subprocess.DEVNULL, timeout=30)
    require(result.returncode == 0, "checkout_binding_failed")
    return result.stdout


def verify_checkout(binding):
    binding_valid(binding)
    require(binding["checker_commit"] == CHECKER and binding["approved_release_commit"] == RELEASE,
            "checkout_binding_failed")
    root = Path(__file__).resolve().parent.parent
    config_keys = git(root, "config", "--local", "--name-only", "--list").decode("utf-8").splitlines()
    require(not any(re.match(r"^(alias\.|include\.|includeif\.|extensions\.|filter\.|diff\.external$|"
                             r"core\.(worktree|fsmonitor|hookspath|attributesfile|excludesfile|sshcommand)$|"
                             r"status\.showuntrackedfiles$|remote\..*\.(promisor|partialclonefilter)$)", key, re.I)
                    for key in config_keys), "checkout_binding_failed")
    git_directory = git(root, "rev-parse", "--absolute-git-dir").decode("utf-8").strip()
    require(not Path(git_directory, "config.worktree").exists(), "checkout_binding_failed")
    require(git(root, "rev-parse", "--show-toplevel").strip() == os.fsencode(root), "checkout_binding_failed")
    head = git(root, "rev-parse", "HEAD").decode("ascii").strip()
    require(binding["extractor_commit"] == head and binding["shape_contract_commit"] == head, "checkout_binding_failed")
    require(not git(root, "status", "--porcelain=v1", "--untracked-files=all"), "checkout_binding_failed")
    require(not git(root, "for-each-ref", "--format=%(refname)", "refs/replace"), "checkout_binding_failed")
    for filename in EXTRACTOR_FILES:
        path = root / "scripts" / filename
        require(not path.is_symlink() and path.is_file(), "checkout_binding_failed")
        require(path.read_bytes() == git(root, "cat-file", "blob", head + ":scripts/" + filename),
                "checkout_binding_failed")
    for filename in ("check-terraform-slice-4-plan.sh", "check-terraform-slice-4-plan-body.sh"):
        path = root / "scripts" / filename
        require(not path.is_symlink() and path.read_bytes() ==
                git(root, "cat-file", "blob", CHECKER + ":scripts/" + filename), "checkout_binding_failed")
    # No release/image/infra drift is admitted by this diagnostic lane.
    for directory in ("infra/terraform/production", "internal", "cmd", "pkg", "deploy", ".github",
                      "Dockerfile", "go.mod", "go.sum", ".dockerignore", ".goreleaser.yaml"):
        require(not git(root, "diff", "--no-ext-diff", RELEASE, "HEAD", "--", directory), "release_binding_failed")


def generate(source, binding_path, output_root, generation, terraform, tooling):
    binding_bytes = read_private(binding_path)
    binding = parse(binding_bytes)
    verify_checkout(binding)
    plan = decode(source, binding["binary_plan_sha256"], output_root, terraform, tooling)
    result = dict(schema=SCHEMA, mode="diagnostic-only", binding=binding, **inventory(plan))
    validate_manifest(result)
    payload = encode(result)
    require(len(payload) <= MAX_BYTES, "output_budget")
    require(read_private(binding_path) == binding_bytes, "source_changed")
    verify_checkout(binding)
    with Directory(output_root) as root:
        publish(root, generation, {"manifest.json": payload})
    # No register consulted, created, activated, or written during generation.
    return "inventory_complete_unreviewed"


def review_status(generation_path, binding_path):
    binding = parse(read_private(binding_path))
    verify_checkout(binding)
    with Directory(generation_path) as directory:
        raw = directory.read("manifest.json")
    manifest = parse(raw)
    validate_manifest(manifest)
    require(raw == encode(manifest) and manifest["binding"] == binding, "manifest_binding_failed")
    register_path = os.environ.get("TF_PLAN_SHAPE_REVIEW_REGISTER_JSON", "")
    require(bool(register_path), "unreviewed")
    register = parse(read_private(register_path))
    state, limitation = status(register, binding, raw)
    require(state == "approved", "review_not_current")
    require(limitation == "rollback_detection_unavailable", "invalid_register")
    return "diagnostic_review_current_rollback_detection_unavailable"


def main(arguments):
    os.umask(0o077)
    if len(arguments) == 7 and arguments[0] == "inventory":
        outcome = generate(*arguments[1:])
    elif len(arguments) == 3 and arguments[0] == "review-status":
        outcome = review_status(*arguments[1:])
    else:
        raise ShapeError("invalid_invocation")
    print(outcome)


if __name__ == "__main__":
    try:
        main(sys.argv[1:])
    except Exception:
        print("diagnostic_blocked", file=sys.stderr)
        sys.exit(1)
