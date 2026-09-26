"""Isolated operator-only D1 register writer. Never imported by the extractor.

Only default-orchestrator may invoke this helper after documented independent
review/human disposition. Role enums record that disposition; they are not an
authentication system against a compromised owner account.
"""
import fcntl
import hashlib
import os
import sys
import uuid

if __name__ == "__main__":
    sys.path.insert(0, os.path.dirname(os.path.realpath(__file__)))

from private_plan_shape import ShapeError, encode, parse, require, validate_manifest
from private_plan_shape_io import Directory, leaf, read_private
from private_plan_shape_review import REGISTER_SCHEMA, validate


def append(register_path, manifest_path, expected_sequence, state, reason, reviewer, writer, timestamp):
    manifest_bytes = read_private(manifest_path)
    manifest = parse(manifest_bytes)
    validate_manifest(manifest)
    require(manifest_bytes == encode(manifest), "noncanonical_manifest")
    parent, name = os.path.split(register_path)
    leaf(name)
    with Directory(parent) as directory:
        fcntl.flock(directory.fd, fcntl.LOCK_EX)
        try:
            try:
                before = directory.read(name)
                register = parse(before)
                validate(register)
            except FileNotFoundError:
                before = None
                register = dict(schema=REGISTER_SCHEMA, events=[])
            previous = register["events"][-1]["sequence"] if register["events"] else 0
            require(type(expected_sequence) is int and expected_sequence == previous, "stale_sequence")
            event = dict(sequence=previous + 1, status=state, reason=reason,
                         binding=manifest["binding"], manifest_sha256=hashlib.sha256(manifest_bytes).hexdigest(),
                         reviewer=reviewer, writer=writer, timestamp=timestamp)
            register["events"].append(event)
            validate(register)
            payload = encode(register)
            staging = "register-" + uuid.uuid4().hex
            directory.write_new(staging, payload)
            require(directory.read(staging) == payload, "write_failed")
            require(read_private(manifest_path) == manifest_bytes, "source_changed")
            directory.unchanged()
            try:
                current = directory.read(name)
            except FileNotFoundError:
                current = None
            require(current == before, "register_changed")
            os.replace(staging, name, src_dir_fd=directory.fd, dst_dir_fd=directory.fd)
            os.fsync(directory.fd)
            require(directory.read(name) == payload, "write_failed")
        finally:
            fcntl.flock(directory.fd, fcntl.LOCK_UN)


def main(arguments):
    os.umask(0o077)
    # No argparse diagnostics: malformed arguments may themselves contain data.
    require(len(arguments) == 9 and arguments[0] == "operator-disposition-confirmed", "invalid_invocation")
    register, manifest, sequence, state, reason, reviewer, writer, timestamp = arguments[1:]
    require(sequence.isascii() and sequence.isdecimal() and len(sequence) <= 12, "invalid_sequence")
    append(register, manifest, int(sequence), state, reason, reviewer, writer, timestamp)
    print("register_event_recorded")


if __name__ == "__main__":
    try:
        main(sys.argv[1:])
    except Exception:
        print("register_write_blocked", file=sys.stderr)
        sys.exit(1)
