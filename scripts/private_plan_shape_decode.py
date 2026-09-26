"""Offline, digest-bound saved-binary decoder (macOS attended workstation).

Never runs init/plan/refresh/apply. Tooling directory is an operator-selected,
preinitialized public checkout with the pinned AWS plugin already installed.
It is trusted code/tooling, NOT a candidate-plan input. Network is denied at
process launch; plugin Unix-socket IPC alone is allowed. Raw stderr is discarded.
"""
import hashlib
import os
import resource
import signal
import stat
import subprocess
import sys
import uuid

from private_plan_shape import MAX_BYTES, ShapeError, parse, require
from private_plan_shape_io import Directory, FILE_FLAGS, check_file, identity, leaf
from private_plan_shape_review import digest

SANDBOX = "(version 1) (allow default) (deny network*) (allow network* (local unix-socket))"
ENV = {"PATH": "/usr/bin:/bin", "CHECKPOINT_DISABLE": "1", "TF_IN_AUTOMATION": "1",
       "HOME": "/dev/null", "TF_CLI_CONFIG_FILE": "/dev/null", "AWS_EC2_METADATA_DISABLED": "true"}


def limits():
    os.umask(0o077)
    resource.setrlimit(resource.RLIMIT_FSIZE, (MAX_BYTES, MAX_BYTES))
    resource.setrlimit(resource.RLIMIT_CORE, (0, 0))


def run(executable, arguments, cwd, output):
    require(sys.platform == "darwin", "offline_decode_unavailable")
    with open(output, "xb", opener=lambda path, flags: os.open(path, flags, 0o600)) as stream:
        process = subprocess.Popen(["/usr/bin/sandbox-exec", "-p", SANDBOX, executable, *arguments],
                                   cwd=cwd, env=ENV, stdin=subprocess.DEVNULL,
                                   stdout=stream, stderr=subprocess.DEVNULL,
                                   start_new_session=True, preexec_fn=limits)
        try:
            require(process.wait(timeout=60) == 0, "offline_decode_failed")
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait()
            raise ShapeError("offline_decode_timeout") from None
        finally:
            # No provider process may outlive a finished or failed decoder.
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
        stream.flush()
        os.fsync(stream.fileno())


def trusted_tool(executable):
    require(type(executable) is str and os.path.isabs(executable), "untrusted_tool")
    executable = os.path.realpath(executable)
    info = os.stat(executable, follow_symlinks=False)
    require(stat.S_ISREG(info.st_mode) and info.st_uid in (0, os.geteuid()) and
            not info.st_mode & 0o022 and info.st_mode & 0o111, "untrusted_tool")
    parent = os.path.dirname(executable)
    while parent != "/":
        info = os.stat(parent, follow_symlinks=False)
        require(stat.S_ISDIR(info.st_mode) and info.st_uid in (0, os.geteuid()) and
                not info.st_mode & 0o022, "untrusted_tool")
        parent = os.path.dirname(parent)
    return executable


def decode(source, expected_digest, private_root, terraform, tooling_directory):
    digest(expected_digest, 64)
    executable = trusted_tool(terraform)
    require(os.path.isabs(tooling_directory) and os.path.realpath(tooling_directory) == tooling_directory,
            "untrusted_tooling")
    parent, filename = os.path.split(source)
    leaf(filename)
    with Directory(parent) as source_dir, Directory(private_root) as root:
        fd = os.open(filename, FILE_FLAGS, dir_fd=source_dir.fd)
        staging = "decode-" + uuid.uuid4().hex
        try:
            original = check_file(fd)
            raw = source_dir.read(filename)
            require(hashlib.sha256(raw).hexdigest() == expected_digest, "digest_mismatch")
            root.unchanged()
            os.mkdir(staging, 0o700, dir_fd=root.fd)
            workpath = root.path + "/" + staging
            with Directory(workpath) as work:
                work.write_new("source.tfplan", raw)
                run(executable, ["version", "-json"], tooling_directory, workpath + "/version.json")
                version = parse(work.read("version.json"))
                require(version.get("terraform_version") == "1.15.8" and
                        version.get("provider_selections") == {"registry.terraform.io/hashicorp/aws": "6.33.0"},
                        "unsupported_toolchain")
                run(executable, ["show", "-json", workpath + "/source.tfplan"],
                    tooling_directory, workpath + "/decoded.json")
                decoded = parse(work.read("decoded.json"))
                require(decoded.get("format_version") == "1.2" and
                        decoded.get("terraform_version") == "1.15.8", "unsupported_format")
                require(hashlib.sha256(work.read("source.tfplan")).hexdigest() == expected_digest,
                        "source_changed")
                require(identity(check_file(fd)) == identity(original), "source_changed")
                require(os.path.samestat(original, os.stat(filename, dir_fd=source_dir.fd, follow_symlinks=False)),
                        "source_changed")
                require(hashlib.sha256(source_dir.read(filename)).hexdigest() == expected_digest,
                        "source_changed")
                source_dir.unchanged()
                root.unchanged()
                # Only our own known staged files; never follow a source link
                # or clean unrelated/stale recovery generations.
                for name in ("source.tfplan", "version.json", "decoded.json"):
                    os.unlink(name, dir_fd=work.fd)
                os.fsync(work.fd)
            os.rmdir(staging, dir_fd=root.fd)
            os.fsync(root.fd)
            return decoded
        finally:
            os.close(fd)
            # Failure stages are owner-only and intentionally preserved. They
            # are never complete inventories and must not be shared or reused.
