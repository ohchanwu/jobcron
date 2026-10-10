#!/usr/bin/env python3
"""App-only, target-host release runner. No cloud transport or arbitrary commands.

An attended operator runs this on the selected host via the existing native SSM
session. Independent evidence, target observations and approvals are trusted
operator inputs, not signatures or assertions made authoritative by this runner.
The local adapter is real Docker/Compose/PostgreSQL, not a simulated provider.
"""
import argparse
import configparser
import contextlib
import fcntl
import hashlib
import html
import http.cookiejar
import json
import math
import os
from pathlib import Path
import re
import selectors
import signal
import ssl
import stat
import subprocess
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid


class Refuse(Exception):
    pass


class Unknown(Refuse):
    """The operation may have been submitted. Never retry automatically."""


class Unsubmitted(Refuse):
    """exec failed before any native process started."""


def require(ok, message):
    if not ok:
        raise Refuse(message)


def encoded(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":")).encode()


def digest(value):
    return hashlib.sha256(encoded(value)).hexdigest()


def file_digest(path):
    p = custody(path)
    result = hashlib.sha256()
    with p.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            result.update(block)
    return result.hexdigest()


def custody(path, directory=False):
    p = Path(path)
    require(p.is_absolute(), "input paths must be absolute")
    # Check every component before following it; reject symlinks and writable
    # foreign parents, including hidden configuration/input routes.
    for ancestor in reversed([p, *p.parents]):
        s = ancestor.lstat()
        require(not stat.S_ISLNK(s.st_mode), "symlink input custody")
        require(not s.st_mode & 0o022, "writable input ancestry")
    s = p.lstat()
    require(s.st_uid == os.getuid(), "foreign input owner")
    require(stat.S_IMODE(s.st_mode) == (0o700 if directory else 0o600), "unsafe input mode")
    require(stat.S_ISDIR(s.st_mode) if directory else stat.S_ISREG(s.st_mode), "unsafe input kind")
    require(directory or s.st_nlink == 1, "hardlinked input")
    return p


def read_bytes(path):
    p = custody(path)
    fd = os.open(p, os.O_RDONLY | os.O_NOFOLLOW)
    try:
        before = p.lstat()
        s = os.fstat(fd)
        require((s.st_dev, s.st_ino) == (before.st_dev, before.st_ino), "input changed during open")
        with os.fdopen(fd, "rb", closefd=False) as stream:
            data = stream.read(8 * 1024 * 1024 + 1)
        require(len(data) <= 8 * 1024 * 1024, "input exceeds bound")
        return data
    finally:
        os.close(fd)


def unique_pairs(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, "duplicate JSON key")
        result[key] = value
    return result


def read_json(path):
    try:
        return json.loads(read_bytes(path), object_pairs_hook=unique_pairs)
    except (ValueError, UnicodeError):
        raise Refuse("invalid JSON input") from None


def atomic(path, value, raw=False):
    p = Path(path)
    custody(p.parent, directory=True)
    if p.exists() or p.is_symlink():
        custody(p)
    fd, temporary = tempfile.mkstemp(prefix=".release-", dir=p.parent)
    try:
        os.fchmod(fd, 0o600)
        with os.fdopen(fd, "wb") as stream:
            stream.write(value if raw else encoded(value) + b"\n")
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, p)
        directory_fd = os.open(p.parent, os.O_RDONLY)
        try:
            os.fsync(directory_fd)
        finally:
            os.close(directory_fd)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


class TargetLock:
    """Kernel lock plus durable stale marker; no TTL-based lock deletion."""
    def __init__(self, root, reconcile=False, owner=None, handoff=None, read_only=False):
        self.root, self.reconcile = Path(root), reconcile
        self.owner = owner
        self.handoff, self.read_only = handoff, read_only
        self.fd = None

    def __enter__(self):
        custody(self.root, directory=True)
        p = self.root / "lock"
        if p.exists() or p.is_symlink():
            custody(p)
        self.fd = os.open(p, os.O_RDWR | os.O_CREAT | os.O_NOFOLLOW, 0o600)
        try:
            fcntl.flock(self.fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
            marker = self.root / "lock.json"
            require(not marker.exists() or self.reconcile or self.handoff, "stale release lock: use resume and authoritative reconciliation")
            if marker.exists():
                previous = read_json(marker)
                if self.handoff:
                    self.handoff(previous)
                else:
                    require(previous.get("owner") == self.owner, "stale intent belongs to another release state")
                    require(not self.read_only or previous.get("read_only") is True, "read-only preflight cannot reconcile mutation intent")
                atomic(self.root / ("history-" + str(uuid.uuid4()) + ".json"), previous)
            elif self.handoff:
                self.handoff(None)
            atomic(marker, {"pid": os.getpid(), "pending": True, "owner": self.owner, "read_only": self.read_only})
            return self
        except (BlockingIOError, Refuse):
            os.close(self.fd)
            self.fd = None
            raise Refuse("release lock occupied or stale; reconciliation required") from None
        except BaseException:
            os.close(self.fd)
            self.fd = None
            raise

    def __exit__(self, typ, value, traceback):
        # Keep stale marker on interruption/ambiguous failure. Never unlink the
        # kernel lock inode, which could allow different processes to hold locks.
        # Read-only failures have no native mutation to reconcile. Preserve their
        # disposition without stranding the target. intent() turns this off before
        # publishing an effect checkpoint.
        if typ is None or self.read_only:
            atomic(self.root / ("history-" + str(uuid.uuid4()) + ".json"),
                   {"owner": self.owner, "read_only": self.read_only, "outcome": "returned" if typ is None else "failed-before-effects"})
            (self.root / "lock.json").unlink()
        if self.fd is not None:
            os.close(self.fd)

    def intent(self):
        self.read_only = False
        atomic(self.root / "lock.json", {"pid": os.getpid(), "pending": True, "owner": self.owner, "read_only": False})


def native(args, timeout, env=None, data=None, output=None):
    """No shell, inherited credentials, stdin/config discovery or raw diagnostics.

    Only an exec-before-submission failure is eligible for one explicit retry.
    All process failures/timeouts are conservatively unknown at an effect boundary.
    """
    clean = {"PATH": "/usr/local/bin:/usr/local/sbin:/opt/homebrew/bin:/usr/bin:/usr/sbin:/bin:/sbin", "LANG": "C.UTF-8",
             "HOME": "/nonexistent", "AWS_EC2_METADATA_DISABLED": "true",
             "AWS_MAX_ATTEMPTS": "1", "AWS_RETRY_MODE": "standard"}
    clean.update(env or {})
    scratch = custody(clean.get("TMPDIR") or os.environ["TMPDIR"], directory=True)
    clean["TMPDIR"] = str(scratch)
    # Secret-bearing output NEVER reaches a temporary disk file. Drain bounded
    # pipes while feeding stdin concurrently (large pg_restore inputs must not
    # deadlock). Only the explicitly approved backup stream writes data to disk.
    try:
        process = subprocess.Popen(args, stdin=subprocess.PIPE if data is not None else subprocess.DEVNULL,
                                   stdout=output if output is not None else subprocess.PIPE, stderr=subprocess.DEVNULL,
                                   env=clean, start_new_session=True)
    except OSError:
        raise Unsubmitted("native executable unavailable; no submission") from None
    result = bytearray()
    deadline = time.monotonic() + timeout
    offset = 0
    with selectors.DefaultSelector() as pipes:
        try:
            if process.stdout is not None:
                os.set_blocking(process.stdout.fileno(), False)
                pipes.register(process.stdout, selectors.EVENT_READ)
            if process.stdin is not None:
                if data:
                    os.set_blocking(process.stdin.fileno(), False)
                    pipes.register(process.stdin, selectors.EVENT_WRITE)
                else:
                    process.stdin.close()
            while pipes.get_map() or process.poll() is None:
                remaining = deadline - time.monotonic()
                require(remaining > 0, "native operation timeout")
                for key, event in pipes.select(min(0.2, remaining)):
                    if event & selectors.EVENT_READ:
                        block = os.read(key.fd, min(65536, 8 * 1024 * 1024 + 1 - len(result)))
                        if not block:
                            pipes.unregister(key.fileobj)
                            key.fileobj.close()
                        else:
                            result.extend(block)
                            require(len(result) <= 8 * 1024 * 1024, "native result exceeds bound")
                    else:
                        try:
                            offset += os.write(key.fd, data[offset:offset + 65536])
                        except BrokenPipeError:
                            offset = len(data)
                        if offset == len(data):
                            pipes.unregister(key.fileobj)
                            key.fileobj.close()
            process.wait(timeout=max(0.01, deadline - time.monotonic()))
        except BaseException:
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            process.wait()
            raise Unknown("native operation interrupted/timed out; reconcile before any retry") from None
        finally:
            for stream in (process.stdin, process.stdout):
                if stream is not None:
                    stream.close()
        if process.returncode != 0:
            raise Unknown("native process returned failure; reconcile any submitted effect")
        return bytes(result)


def image_selector(value, mode):
    pattern = r"ghcr\.io/[a-z0-9][a-z0-9-]*/jobcron@sha256:[0-9a-f]{64}"
    if mode == "local" and re.fullmatch(r"sha256:[0-9a-f]{64}", value):
        return
    require(re.fullmatch(pattern, value), "immutable image selector required")


def validate_manifests(manifests):
    require(set(manifests) == {"postgres", "sqlite"}, "both complete migration manifests required")
    for manifest in manifests.values():
        require(set(manifest) == {"tree", "files"} and re.fullmatch(r"[0-9a-f]{40}", manifest["tree"]), "migration tree required")
        files = manifest["files"]
        require(isinstance(files, list) and files, "empty migration manifest")
        names, versions = [], []
        for item in files:
            require(set(item) == {"name", "sha256"}, "invalid migration entry")
            require(re.fullmatch(r"[0-9]{4}_[a-z0-9_]+\.sql", item["name"]) and
                    re.fullmatch(r"[0-9a-f]{64}", item["sha256"]), "invalid migration identity")
            names.append(item["name"])
            versions.append(int(item["name"][:4]))
        require(names == sorted(set(names)) and len(set(versions)) == len(versions), "duplicate/unordered migration manifest")


def validate_ledger(manifests, ledger):
    expected = [{"version": int(f["name"][:4]), **f} for f in manifests["postgres"]["files"]]
    require(ledger == expected, "exact complete applied ledger mismatch; outside app-only lane")


def approve(descriptor, approvals, effect):
    require(re.fullmatch(r"[A-Za-z0-9_-]{1,120}", approvals.get("approval_id", "")) and
            set(approvals.get("effects", [])) <= {"observe", "recovery-point", "persist", "replace", "recover"}, "invalid named approval scope")
    require(approvals.get("descriptor_sha256") == digest(descriptor) and
            approvals.get("target") == descriptor["target"] and approvals.get("approval_id") and
            effect in approvals.get("effects", []), "missing exact independent approval: " + effect)


def observation_policy(d):
    duration, spacing = d["observe_seconds"], d["sample_seconds"]
    require(isinstance(duration, (int, float)) and isinstance(spacing, (int, float)) and
            math.isfinite(duration) and math.isfinite(spacing) and 0 < spacing <= duration / 2,
            "initial/intermediate/final observation required")
    require(d["mode"] == "local" or duration >= 1800, "production observation requires at least 1800 seconds")


def reconcile_effect(pending, state):
    if pending["kind"] == "persist":
        require(state.get("selector") == pending["after"] and state.get("version") and
                state["version"] != pending["before"]["version"], "selector submission unprovable/conflicting; stop")
    elif pending["kind"] == "replace":
        require(state.get("image") == pending["after"] and state.get("container") and
                state["container"] != pending["before"]["container"], "app submission unprovable/conflicting; stop")
    else:
        raise Refuse("submitted recovery-point operation requires attended reconciliation")
    return "committed"


LEDGER_SQL = "SELECT COALESCE(json_agg(t ORDER BY version), '[]'::json) FROM (SELECT version, name, sha256 FROM schema_migrations) t;"


def metadata(container, keys):
    result = {k: container[k] for k in keys}
    if "Mounts" in result:
        # Docker's inspect array order is unspecified. Destinations are unique;
        # sorting preserves every field and never admits a changed mount.
        mounts = result["Mounts"]
        require(len({m["Destination"] for m in mounts}) == len(mounts), "duplicate mount destination")
        result["Mounts"] = sorted(mounts, key=lambda m: m["Destination"])
    if "HostConfig" in result and result["HostConfig"].get("Mounts") is not None:
        # Compose's Engine API mount array is also unordered, but uses Target
        # rather than inspect's Destination. Preserve nested options verbatim.
        result["HostConfig"] = dict(result["HostConfig"])
        mounts = result["HostConfig"]["Mounts"]
        require(len({m["Target"] for m in mounts}) == len(mounts), "duplicate API mount target")
        result["HostConfig"]["Mounts"] = sorted(mounts, key=lambda m: m["Target"])
    return result


def environment(values):
    require(all(isinstance(v, str) and "=" in v for v in values), "invalid runtime environment")
    require(len({v.split("=", 1)[0] for v in values}) == len(values), "duplicate runtime environment name")
    return sorted(values)


def aws_inputs(target):
    # AWS config is an additional executable input route (credential_process,
    # role chaining, endpoint/services overrides). Admit static selected-session
    # files only, never default profile discovery or a credential broker.
    config = configparser.ConfigParser(interpolation=None)
    credentials = configparser.ConfigParser(interpolation=None)
    try:
        config.read_string(read_bytes(target["aws_config"]).decode())
        credentials.read_string(read_bytes(target["aws_credentials"]).decode())
    except configparser.Error:
        raise Refuse("invalid operator configuration") from None
    profile = target["aws_profile"]
    section = "default" if profile == "default" else "profile " + profile
    require(not config.defaults() and config.sections() == [section] and
            dict(config[section]) == {"region": target["region"]}, "unsupported AWS configuration route")
    require(not credentials.defaults() and credentials.sections() == [profile] and
            set(credentials[profile]) == {"aws_access_key_id", "aws_secret_access_key", "aws_session_token"} and
            all(v and not any(c in v for c in "\r\n\x00") for v in credentials[profile].values()),
            "explicit static operator session credentials required")


class Host:
    """Fixed native target-host operations; local Unix Docker only.

    Production execution is on the exact host inside an attended SSM session.
    AWS uses explicit operator-selected files and disables retries/metadata.
    No remote shell, arbitrary command, login, discovery, down or systemd route.
    """
    def __init__(self, descriptor, target, root):
        self.d, self.t, self.root = descriptor, target, Path(root)
        self.timeout = descriptor["operation_timeout"]
        self.local = descriptor["mode"] == "local"
        self.env = {"DOCKER_HOST": target["docker_host"], "DOCKER_CONFIG": str(self.root / "docker"),
                    "TMPDIR": str(self.root)}
        p = self.root / "docker"
        if not p.exists():
            p.mkdir(mode=0o700)
        custody(p, directory=True)
        require(not any(p.iterdir()), "Docker config must be empty; no credentials/keychain helpers")
        require(target["docker_host"].startswith("unix:///"), "local Unix Docker required")
        custody(target["compose_env"])
        custody(target["selector_file"])
        # Compose is executable configuration; trusted operator pins its bytes.
        require(file_digest(target["compose_file"]) == target["compose_sha256"], "Compose contract drift")
        if not self.local:
            aws_inputs(target)
            self.secret_work = custody(target["secret_work_dir"], directory=True)
            for path in [self.secret_work, *map(Path, target["protected_paths"]),
                         *[Path(target[k]) for k in ("aws_credentials", "database_url_file", "journey_account_file", "proxy_secret_file")]]:
                custody(path, directory=path.is_dir())
                require(self.run(["findmnt", "-n", "-o", "FSTYPE", "--target", str(path)]).strip() == b"tmpfs", "production secret inputs require tmpfs")
            require(not self.run(["swapon", "--noheadings", "--show=NAME"]).strip(), "production swap must be disabled")
            # Native stdout uses bounded non-disk pipes. Native tools' own scratch
            # and credential/payload/passfile routes use verified private tmpfs.
            # The durable root holds sanitized checkpoints/approved archives.
            self.env["TMPDIR"] = str(self.secret_work)
            self.secret_output_verified = True
            self.aws_env = {"AWS_CONFIG_FILE": target["aws_config"],
                            "AWS_SHARED_CREDENTIALS_FILE": target["aws_credentials"],
                            "AWS_PROFILE": target["aws_profile"], "AWS_CLI_AUTO_PROMPT": "off"}
        require(Path(target["compose_executable"]).is_absolute() and os.access(target["compose_executable"], os.X_OK),
                "native Compose executable unavailable")
        self.compose_prefix = [target["compose_executable"], "--project-name", target["project"],
                               "--project-directory", str(Path(target["compose_file"]).parent),
                               "--file", target["compose_file"], "--env-file", target["compose_env"]]

    def run(self, args, data=None, output=None):
        return native(args, self.timeout, env=self.env, data=data, output=output)

    def docker(self, *args, data=None):
        return self.run(["docker", *args], data=data)

    def compose(self, *args):
        return self.run([*self.compose_prefix, *args])

    def aws(self, *args):
        require(not self.local, "provider operation disabled in local mode")
        require(self.secret_output_verified and self.env["TMPDIR"] == str(self.secret_work), "unverified secret output route")
        return native(["aws", "--no-cli-pager", "--region", self.t["region"], *args], self.timeout,
                      env={**self.env, **self.aws_env})

    def mutate(self, operation):
        # Precisely the mutating operation, not surrounding preflight/readback.
        boundary = getattr(self, "submission", None)
        return boundary(operation) if boundary else operation()

    def expected_selector(self, image, operation_id):
        if not self.local:
            return {"selector": image, "version": operation_id}
        values = self.env_values(read_bytes(self.t["selector_file"]))
        values["JOBCRON_IMAGE"] = image
        raw = "".join(f"{k}={v}\n" for k, v in values.items()).encode()
        return {"selector": image, "version": hashlib.sha256(raw).hexdigest()}

    def prereqs(self):
        info = json.loads(self.docker("info", "--format", "{{json .}}"))
        require(info["ID"] == self.t["daemon_id"], "wrong Docker daemon/target")
        self.compose("version")
        for role in ("current", "candidate", "recovery"):
            artifact = self.d[role]
            # Preavailability only: never pull/login implicitly. Operator may
            # pre-pull separately with explicit credentials/publication authority.
            image = json.loads(self.docker("image", "inspect", artifact["selector"]))[0]
            require(image["Id"] == artifact["image_id"] and
                    image["Os"] + "/" + image["Architecture"] == artifact["platform"], "artifact digest/platform unavailable")
            if not self.local:
                require(artifact["selector"] in image["RepoDigests"], "publication digest unavailable")
            require(image["Config"].get("Entrypoint") == ["jobcron"], "unsupported app entrypoint")
        if not self.local:
            # Explicit account+instance identity: descriptor cannot select itself.
            caller = json.loads(self.aws("sts", "get-caller-identity"))
            require(caller["Account"] == self.t["account"] and caller["Arn"] == self.t["actor_arn"], "wrong provider actor")
            identity = read_json(self.t["instance_identity_file"])
            require(identity["instanceId"] == self.t["instance_id"] and identity["accountId"] == self.t["account"] and
                    identity["region"] == self.t["region"], "wrong authoritative host identity")
            require(Path("/sys/class/dmi/id/board_asset_tag").read_text().strip() == self.t["instance_id"],
                    "executing host does not match selected EC2 instance")

    def render(self):
        configuration = json.loads(self.compose("config", "--format", "json"))
        app = configuration["services"]["app"]
        app.pop("image")
        for name in ("build", "scale", "container_name", "env_file"):
            require(name not in app, "unsupported app Compose field")
        require(app.get("pull_policy") == "never" and app.get("environment", {}).get("JOBCRON_SCHEDULER_ENABLED") == "1",
                "one fixed scheduler/pull-never contract required")
        require("--no-open" in app["command"], "no-open required")
        for name in ("DATABASE_URL", "SESSION_SECRET", "JOBCRON_CREDENTIAL_ENCRYPTION_KEY", "JOBCRON_SIGNUP_ACCESS_CODE", "JOBCRON_PROXY_SECRET"):
            require(name not in app["environment"] and name + "_FILE" in app["environment"], "secret file routes required")
        return digest(configuration)

    def selector(self):
        if self.local:
            raw = read_bytes(self.t["selector_file"])
            values = self.env_values(raw)
            return {"selector": values["JOBCRON_IMAGE"], "version": hashlib.sha256(raw).hexdigest()}
        result = json.loads(self.aws("secretsmanager", "get-secret-value", "--secret-id", self.t["secret_id"]))
        secret = json.loads(result["SecretString"], object_pairs_hook=unique_pairs)
        return {"selector": secret["JOBCRON_IMAGE"], "version": result["VersionId"], "secret": secret}

    @staticmethod
    def env_values(raw):
        result = {}
        for line in raw.decode().splitlines():
            require(re.fullmatch(r"[A-Z0-9_]+=[^\r\n\x00]+", line), "unsafe selector file")
            name, value = line.split("=", 1)
            require(name not in result, "duplicate selector field")
            result[name] = value
        require(set(result) == {"JOBCRON_IMAGE", "JOBCRON_STAGE1_SPONSOR_USER_ID"}, "selector file scope mismatch")
        return result

    def persist(self, image, before, operation_id):
        current = self.selector()
        require(current["version"] == before["version"] and current["selector"] == before["selector"], "selector preimage drift")
        if not self.local:
            protected = dict(current["secret"])
            protected.pop("JOBCRON_IMAGE")
            require(digest(protected) == self.evidence_secret_digest, "protected secret preimage drift")
        if self.local:
            raw = read_bytes(self.t["selector_file"])
            values = self.env_values(raw)
            values["JOBCRON_IMAGE"] = image
            self.mutate(lambda: atomic(self.t["selector_file"], "".join(f"{k}={v}\n" for k, v in values.items()).encode(), raw=True))
        else:
            secret = dict(current["secret"], JOBCRON_IMAGE=image)
            payload = self.secret_work / "selector-payload.json"
            atomic(payload, secret)
            try:
                # Reobserve immediately before put. Secrets Manager has no CAS;
                # exclusive attended operator custody remains an explicit boundary.
                require(self.selector()["version"] == before["version"], "secret preimage changed")
                self.mutate(lambda: self.aws("secretsmanager", "put-secret-value", "--secret-id", self.t["secret_id"],
                         "--client-request-token", operation_id, "--secret-string", "file://" + str(payload)))
                after = self.selector()
                require(after["secret"] == secret and after["version"] == operation_id, "secret authoritative readback mismatch")
            finally:
                payload.unlink()
        # Persist runtime image only; do not call runtime prepare/cleanup, which
        # recreates mounted directories. Compare all other bytes/fields.
        self.persist_runtime_env(image)

    def persist_runtime_env(self, image):
        p = self.t["compose_env"]
        raw = read_bytes(p)
        values = self.env_values(raw)
        values["JOBCRON_IMAGE"] = image
        desired = "".join(f"{k}={v}\n" for k, v in values.items()).encode()
        if desired != raw:
            self.mutate(lambda: atomic(p, desired, raw=True))
        require(self.env_values(read_bytes(p)) == values, "runtime selector readback failed")
        require(self.render() == self.t["config_sha256"], "non-image configuration changed")

    def inspect(self, container):
        return json.loads(self.docker("inspect", container))[0]

    def service(self, name, allow_stopped=False):
        ids = self.docker("ps", "-a", "--filter", "label=com.docker.compose.project=" + self.t["project"],
                          "--filter", "label=com.docker.compose.service=" + name, "--format", "{{.ID}}").decode().split()
        require(len(ids) == 1, "exactly one identifiable " + name + " required")
        container = self.inspect(ids[0])
        require(allow_stopped or container["State"]["Running"], "running service required")
        return container

    def snapshot(self, allow_stopped=False):
        require(self.render() == self.t["config_sha256"], "non-image Compose drift")
        app, proxy = self.service("app", allow_stopped=allow_stopped), self.service("caddy")
        require(app["Config"]["Env"].count("JOBCRON_SCHEDULER_ENABLED=1") == 1 and
                (allow_stopped or (app["State"]["Running"] and app["State"]["Pid"] > 0)), "active scheduler contract invalid")
        require(not app["State"].get("Paused") and not app["State"].get("Restarting"), "indeterminate app lifecycle")
        # Snapshot only metadata, never container logs or secret values.
        protected = {"proxy": metadata(proxy, ("Id", "Image", "Mounts", "Config", "HostConfig")),
                     "app": metadata(app, ("Mounts", "HostConfig")),
                     "app_env": environment(app["Config"]["Env"]), "app_cmd": app["Config"]["Cmd"],
                     "secrets": self.mount_files()}
        # Docker injects the image selector into log tag configuration on some
        # versions; unsupported differences remain fail-closed, not normalized.
        if self.local:
            db = self.service("db")
            protected["database"] = metadata(db, ("Id", "Image", "Mounts", "Config", "HostConfig"))
        require(digest(protected) == self.t["protected_sha256"], "protected runtime/mount/database/proxy drift")
        validate_ledger(self.d["manifests"], self.ledger())
        return {"container": app["Id"], "image": app["Image"], "restarts": app["RestartCount"],
                "started_at": app["State"]["StartedAt"], "running": app["State"]["Running"], "protected_sha256": digest(protected)}

    def mount_files(self):
        records = []
        for path in self.t["protected_paths"]:
            p = custody(path, directory=Path(path).is_dir())
            files = [p, *sorted(p.iterdir())] if p.is_dir() else [p]
            for f in files:
                s = f.lstat()
                require(not f.is_symlink(), "symlink secret mount")
                record = {"path": str(f), "device": s.st_dev, "inode": s.st_ino, "mode": stat.S_IMODE(s.st_mode)}
                if f.is_file():
                    record["digest"] = file_digest(f)
                else:
                    require(f == p and f.is_dir(), "nested secret mounts unsupported")
                records.append(record)
        return records

    @contextlib.contextmanager
    def pg_environment(self):
        # Existing file-input custody, password never in argv/process environment.
        url = urllib.parse.urlsplit(read_bytes(self.t["database_url_file"]).decode())
        require(url.scheme == "postgres" and url.username and url.password and url.hostname and url.port, "invalid database input")
        assert url.username and url.password and url.hostname and url.port
        require(not url.fragment and not any(c in urllib.parse.unquote(url.password) for c in "\n\r\x00"), "unsafe database input")
        passfile = self.secret_work / "pgpass"
        def escape(text):
            return text.replace("\\", "\\\\").replace(":", "\\:")
        fields = [url.hostname, str(url.port), url.path[1:], urllib.parse.unquote(url.username), urllib.parse.unquote(url.password)]
        query = urllib.parse.parse_qs(url.query)
        require(set(query) == {"sslmode", "sslrootcert"} and query["sslmode"] == ["verify-full"] and
                len(query["sslrootcert"]) == 1, "production PostgreSQL verify-full required")
        ca = custody(self.t["database_ca_file"])
        atomic(passfile, (":".join(map(escape, fields)) + "\n").encode(), raw=True)
        env = {"PGHOST": fields[0], "PGPORT": fields[1], "PGDATABASE": fields[2], "PGUSER": fields[3],
               "PGPASSFILE": str(passfile), "PGSSLMODE": "verify-full", "PGSSLROOTCERT": str(ca), "PGCONNECT_TIMEOUT": "10"}
        try:
            yield env
        finally:
            passfile.unlink()

    def ledger(self):
        if self.local:
            db = self.service("db")
            result = self.docker("exec", "-i", db["Id"], "psql", "-X", "-U", "postgres", "-d", "jobcron", "-At", "-v", "ON_ERROR_STOP=1", "-c", LEDGER_SQL)
        else:
            with self.pg_environment() as pg:
                result = native(["psql", "-X", "-At", "-v", "ON_ERROR_STOP=1", "-c", LEDGER_SQL], self.timeout,
                                env={**self.env, **pg})
        return json.loads(result)

    def backup(self, operation_id):
        # Own a new artifact; no migrations/restores or old database rollback.
        p = self.root / (operation_id + ".dump")
        require(not p.exists(), "recovery artifact already exists; reconcile")
        # Resolve read-only prerequisites before artifact creation. Once a file
        # may exist, a later missing dump/upload/helper cannot prove the whole
        # backup unsubmitted.
        db = self.service("db") if self.local else None
        stream = self.mutate(lambda: p.open("xb"))
        with stream:
            os.chmod(p, 0o600)
            if self.local:
                self.run(["docker", "exec", db["Id"], "pg_dump", "-U", "postgres", "-d", "jobcron", "-Fc"], output=stream)
            else:
                with self.pg_environment() as pg:
                    native(["pg_dump", "-Fc"], self.timeout, env={**self.env, **pg}, output=stream)
            stream.flush()
            os.fsync(stream.fileno())
        require(p.stat().st_size > 0, "empty recovery artifact")
        if self.local:
            self.docker("exec", "-i", self.service("db")["Id"], "pg_restore", "--list", data=read_bytes(p))
        else:
            native(["pg_restore", "--list", str(p)], self.timeout, env=self.env)
            remote = "s3://" + self.t["recovery_bucket"] + "/app-release/" + operation_id + ".dump"
            self.aws("s3", "cp", str(p), remote, "--only-show-errors")
            copy = self.root / (operation_id + ".readback")
            # Pre-create private output; aws writes only to the selected path.
            atomic(copy, b"", raw=True)
            self.aws("s3", "cp", remote, str(copy), "--only-show-errors")
            require(file_digest(copy) == file_digest(p), "off-host recovery readback mismatch")
            copy.unlink()
        return {"operation_id": operation_id, "dump_sha256": file_digest(p), "off_host_verified": not self.local}

    def replace(self):
        # Compose stop-before-create replaces one singleton, never blue/green,
        # never down, proxy recreate, mounted-dir cleanup or systemd restart.
        self.mutate(lambda: self.compose("up", "--detach", "--no-deps", "--no-build", "--pull", "never", "--force-recreate", "--scale", "app=1", "app"))

    def journey(self):
        account = read_json(self.t["journey_account_file"])
        base = self.t["journey_url"]
        parsed = urllib.parse.urlsplit(base)
        require(parsed.scheme == "https" and not parsed.username and not parsed.query and not parsed.fragment, "verified HTTPS journey required")
        if self.local:
            require(parsed.hostname == "127.0.0.1", "local journey must use owned loopback")
        context = ssl.create_default_context(cafile=str(custody(self.t["journey_ca_file"])))
        proxy = read_bytes(self.t["proxy_secret_file"]).decode()
        jar = http.cookiejar.CookieJar()
        class SameOrigin(urllib.request.HTTPRedirectHandler):
            def redirect_request(self, req, fp, code, msg, headers, newurl):
                require(urllib.parse.urlsplit(newurl).netloc == parsed.netloc and newurl.startswith(base + "/"), "journey redirected away from exact target")
                return super().redirect_request(req, fp, code, msg, headers, newurl)
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(jar),
                                            urllib.request.HTTPSHandler(context=context), SameOrigin())
        def request(path, form=None):
            req = urllib.request.Request(base + path, data=urllib.parse.urlencode(form).encode() if form is not None else None,
                                         headers={"X-Jobcron-Proxy": proxy})
            with opener.open(req, timeout=min(15, self.timeout)) as response:
                body = response.read(2 * 1024 * 1024).decode()
                require(response.status == 200, "journey response failure")
                return body, response.geturl()
        def token(body):
            found = re.search(r'name="csrf_token" value="([^"]+)"', body)
            require(found, "journey CSRF token absent")
            return html.unescape(found.group(1))
        login, _ = request("/login")
        _, location = request("/login", {"email": account["email"], "password": account["password"], "csrf_token": token(login)})
        require("/login" not in location, "journey authentication failed")
        for path in ("/", "/briefing", "/bookmarks", "/profile", "/account"):
            body, location = request(path)
            require("/login" not in location and "<!doctype html>" in body.lower(), "authenticated core journey failed")
        body, _ = request("/account")
        request("/logout", {"csrf_token": token(body)})
        _, location = request("/account")
        require("/login" in location, "logout/session reuse failed")
        return {"tls": True, "authenticated_core": True, "logout": True, "scrape_or_rerate": False}


class Runner:
    def __init__(self, descriptor, evidence, approvals, target, root, host_class=Host):
        self.d, self.e, self.a, self.t = descriptor, evidence, approvals, target
        self.root = custody(root, directory=True)
        self.checkpoint = self.root / "checkpoint.json"
        self.validate()
        self.host_class = host_class
        self.host = None
        self.cp = None

    def validate(self):
        d = self.d
        required = {"schema", "release_id", "mode", "target", "current", "candidate", "recovery", "manifests",
                    "config_sha256", "operation_timeout", "observe_seconds", "sample_seconds"}
        require(set(d) == required and d["schema"] == 1 and d["mode"] in ("local", "production"), "invalid descriptor schema")
        require(re.fullmatch(r"[a-z0-9-]{1,80}", d["release_id"]) and 1 <= d["operation_timeout"] <= 600, "invalid release/time bound")
        require(d["target"] == self.t["target"] and d["config_sha256"] == self.t["config_sha256"], "independent target/config mismatch")
        validate_manifests(d["manifests"])
        observation_policy(d)
        require(self.e.get("target") == d["target"] and self.e.get("mode") == d["mode"], "independent evidence target mismatch")
        for role in ("current", "candidate", "recovery"):
            artifact = d[role]
            require(set(artifact) == {"source", "selector", "image_id", "platform"} and
                    re.fullmatch(r"[0-9a-f]{40}", artifact["source"]) and
                    re.fullmatch(r"sha256:[0-9a-f]{64}", artifact["image_id"]) and
                    artifact["platform"] in ("linux/arm64", "linux/amd64"), "invalid full artifact identity")
            image_selector(artifact["selector"], d["mode"])
            evidence = self.e.get(role, {})
            require(evidence.get("artifact") == artifact and evidence.get("manifests") == d["manifests"], "independent artifact/manifest mismatch")
            for gate in ("ci", "review", "publication"):
                entry = evidence.get(gate, {})
                require(entry.get("source") == artifact["source"] and entry.get("result") == "accepted" and entry.get("record"), "absent exact provenance: " + gate)
                require(entry.get("kind") == ("local-rehearsal" if d["mode"] == "local" else gate), "local provenance cannot approve production")
            require(evidence["publication"].get("selector") == artifact["selector"] and
                    evidence["publication"].get("platform") == artifact["platform"], "publication identity mismatch")
        require(self.e.get("restore", {}).get("accepted") is True and
                self.e["restore"].get("manifests") == d["manifests"] and self.e["restore"].get("target") == d["target"] and
                self.e["restore"].get("record"), "independent compatible restore evidence required")
        approve(d, self.a, "observe")

    def stop(self):
        require(not (self.root / "STOP").exists() and not (Path(self.t["lock_dir"]) / "STOP").exists(),
                "global owner stop; no further campaign effects")

    def save(self):
        atomic(self.checkpoint, self.cp)

    def observe(self, expected_image, allow_stopped=False):
        self.stop()
        state = self.host.snapshot(allow_stopped=True) if allow_stopped else self.host.snapshot()
        require(state["image"] == expected_image, "wrong authoritative current image")
        return state

    def selector_state(self, expected=None):
        state = self.host.selector()
        if not self.host.local:
            protected = dict(state["secret"])
            protected.pop("JOBCRON_IMAGE")
            require(digest(protected) == self.e.get("secret_nonimage_sha256"), "protected secret value drift")
        result = {k: state[k] for k in ("selector", "version")}
        require(expected is None or result == expected, "exact native selector/version drift")
        return result

    def recovery_handoff(self, failed_state, marker):
        # Called while holding the SAME stable kernel lock, before marker transfer.
        # This is not authority inferred from failure: the new descriptor's
        # compatible provenance and explicit recover approval were already checked.
        old_root = custody(failed_state, directory=True)
        require(old_root != self.root and not (old_root / "STOP").exists(), "invalid/stopped failed release state")
        cp = read_json(old_root / "checkpoint.json")
        require(cp["target"] == self.d["target"] and cp["target_sha256"] == digest(self.t) and
                cp["wanted"] == self.d["current"], "failed-state target/artifact mismatch")
        if marker:
            old_owner = {"state_dir": str(old_root), "descriptor_sha256": cp["descriptor_sha256"]}
            new_owner = {"state_dir": str(self.root), "descriptor_sha256": digest(self.d)}
            require(marker.get("owner") == old_owner or
                    (marker.get("owner") == new_owner and (self.root / "handoff.json").exists()), "handoff marker belongs to another state")
        require(cp["stage"] in ("replace", "verify", "complete") and not cp["pending"],
                "unresolved failed-state effects; use original resume before recovery")
        effects = {e["kind"]: e for e in cp["effects"]}
        require(set(effects) == {"recovery-point", "persist", "replace"} and len(cp["effects"]) == 3 and cp.get("backup"),
                "failed release not fully committed/reconciled")
        require(all(e["status"] in ("committed-verified", "committed-reconciled") for e in effects.values()), "unproven failed-state effects")
        self.selector_state(effects["persist"]["result"])
        actual = self.observe(self.d["current"]["image_id"], allow_stopped=True)
        committed = effects["replace"]["result"]
        # Failure may stop/restart the SAME container. Do not adopt an unrelated
        # container/image or discard protected-state predicates to enable recovery.
        require(actual["container"] == committed["container"] and
                actual.get("protected_sha256") == committed.get("protected_sha256"), "failed candidate identity drift")
        record = {"schema": 1, "failed_state": str(old_root), "failed_checkpoint_sha256": file_digest(old_root / "checkpoint.json"),
                  "failed_descriptor_sha256": cp["descriptor_sha256"], "new_descriptor_sha256": digest(self.d),
                  "approval_id": self.a["approval_id"], "observed": actual, "selector": self.selector_state(),
                  "failed_acceptance_not_promoted": True}
        if (self.root / "handoff.json").exists():
            require(read_json(self.root / "handoff.json") == record, "handoff observation changed; preserve and reconcile")
        else:
            atomic(self.root / "handoff.json", record)
        self.handoff_observed = actual
        # Preserve old checkpoint/receipts; a separate terminal marker prevents
        # their future resume from representing superseded acceptance as current.
        if (old_root / "superseded.json").exists():
            previous = read_json(old_root / "superseded.json")
            require(previous["new_descriptor_sha256"] == digest(self.d) and
                    previous["failed_checkpoint_sha256"] == record["failed_checkpoint_sha256"], "failed-state handoff already belongs elsewhere")
        else:
            atomic(old_root / "superseded.json", record)

    def prepare(self):
        # Local input-only validation: no provider/Docker reads, lock or writes.
        return {"status": "prepared-local-only", "mode": self.d["mode"],
                "descriptor_sha256": digest(self.d), "effects": ["observe", "recovery-point", "persist", "replace"],
                "recovery_requires_separate_approval": True, "production_state_verified": False}

    def preflight(self):
        self.stop()
        approve(self.d, self.a, "observe")
        with TargetLock(custody(self.t["lock_dir"], directory=True),
                        owner={"state_dir": str(self.root), "descriptor_sha256": digest(self.d)}, read_only=True, reconcile=True):
            self.host = self.host_class(self.d, self.t, self.root)
            self.host.prereqs()
            state = self.observe(self.d["current"]["image_id"])
            require(self.selector_state()["selector"] == self.d["current"]["selector"], "persisted current selector mismatch")
            return {"status": "preflight-observed", "mode": self.d["mode"], "image_id": state["image"],
                    "protected_state_preserved": True, "exact_ledger": True, "effects_submitted": False}

    def effect(self, kind, operation, before, after):
        self.stop()
        approve(self.d, self.a, kind)
        pending = {"kind": kind, "operation_id": str(uuid.uuid4()), "before": before, "after": after, "status": "proven-unsubmitted"}
        if kind == "persist":
            pending["expected"] = self.host.expected_selector(after, pending["operation_id"])
        # Durable intent precedes invoking the native operation. Once this record
        # exists, the precise mutation boundary accounts for submission. A late
        # missing readback helper cannot relabel an earlier mutation unsubmitted.
        self.active_lock.intent()
        self.cp["pending"] = pending
        self.save()
        def submission(call):
            self.stop()
            previous = pending["status"]
            pending["status"] = "submitted-unknown"
            self.save()
            try:
                return call()
            except Unsubmitted:
                if previous == "proven-unsubmitted":
                    pending["status"] = "proven-unsubmitted"
                    self.save()
                raise
        self.host.submission = submission
        try:
            result = operation(pending["operation_id"])
        finally:
            self.host.submission = None
        self.stop()
        require(pending["status"] == "submitted-unknown", "adapter did not account for mutation submission")
        pending["result"] = result
        pending["status"] = "committed-verified"
        self.cp["effects"].append(pending)
        self.cp["pending"] = None
        self.save()

    def start(self, recovery=False, resume=False, failed_state=None):
        self.stop()
        require(not (self.root / "superseded.json").exists(), "release state was handed to approved recovery; historical only")
        require(failed_state is None or (recovery and not resume), "failed-state handoff requires new approved recovery")
        if failed_state is not None:
            require(not self.checkpoint.exists(), "handoff state already has a checkpoint: resume it, never overwrite")
        wanted = self.d["recovery" if recovery else "candidate"]
        require(wanted["image_id"] != self.d["current"]["image_id"], "no-op image release refused before effects")
        for gate in ("observe", "persist", "replace", "recovery-point"):
            approve(self.d, self.a, gate)
        if recovery:
            approve(self.d, self.a, "recover")
        # Handoff needs real readback UNDER the acquired target kernel lock.
        def handoff(marker):
            self.host = self.host_class(self.d, self.t, self.root)
            self.host.evidence_secret_digest = self.e.get("secret_nonimage_sha256")
            self.host.prereqs()
            self.recovery_handoff(failed_state, marker)
        with TargetLock(custody(self.t["lock_dir"], directory=True), reconcile=resume,
                        owner={"state_dir": str(self.root), "descriptor_sha256": digest(self.d)},
                        handoff=handoff if failed_state is not None else None,
                        read_only=not resume and failed_state is None) as lock:
            self.active_lock = lock
            self.host = self.host or self.host_class(self.d, self.t, self.root)
            self.host.evidence_secret_digest = self.e.get("secret_nonimage_sha256")
            self.host.prereqs()
            if resume:
                self.cp = read_json(self.checkpoint)
                require(self.cp["descriptor_sha256"] == digest(self.d) and self.cp["recovery"] == recovery and
                        self.cp["target"] == self.d["target"], "checkpoint identity mismatch")
                require(self.cp["evidence_sha256"] == digest(self.e) and self.cp["target_sha256"] == digest(self.t), "checkpoint evidence/target changed")
                self.resume_pending(wanted)
            else:
                require(not self.checkpoint.exists(), "checkpoint exists: resume, never overwrite recovery evidence")
                initial = self.observe(self.d["current"]["image_id"], allow_stopped=recovery)
                selector = self.selector_state()
                require(selector["selector"] == self.d["current"]["selector"], "wrong authoritative persisted current selector")
                self.cp = {"schema": 1, "target": self.d["target"], "descriptor_sha256": digest(self.d),
                           "evidence_sha256": digest(self.e), "target_sha256": digest(self.t), "recovery": recovery,
                           "initial": initial, "wanted": wanted, "selector_before": selector,
                           "effects": [], "pending": None, "stage": "preflight", "samples": []}
                self.save()
            if self.cp["stage"] == "preflight":
                self.observe(self.d["current"]["image_id"], allow_stopped=recovery)
                def backup(op):
                    self.cp["backup"] = self.host.backup(op)
                    return self.cp["backup"]
                self.effect("recovery-point", backup, self.cp["initial"], wanted["image_id"])
                self.cp["stage"] = "persist"
                self.save()
            if self.cp["stage"] == "persist":
                self.observe(self.d["current"]["image_id"], allow_stopped=recovery)
                before = self.selector_state(self.cp["selector_before"])
                require(before["selector"] == self.d["current"]["selector"] and before["version"] == self.cp["selector_before"]["version"], "selector preimage drift")
                def persist(op):
                    assert self.host is not None
                    self.host.persist(wanted["selector"], before, op)
                    return self.selector_state(self.cp["pending"]["expected"])
                self.effect("persist", persist,
                            {k: before[k] for k in ("selector", "version")}, wanted["selector"])
                self.cp["selector_after"] = self.cp["effects"][-1]["result"]
                self.cp["stage"] = "replace"
                self.save()
            if self.cp["stage"] == "replace":
                current = self.observe(self.d["current"]["image_id"], allow_stopped=recovery)
                selector = self.selector_state(self.cp["selector_after"])
                require(selector["selector"] == wanted["selector"] and selector["version"] == self.cp["selector_after"]["version"], "persisted selector changed before replacement")
                self.cp["replacement_started"] = time.time()
                def replace(op):
                    assert self.host is not None
                    self.host.replace()
                    after = self.observe(wanted["image_id"])
                    require(after["container"] != current["container"], "replacement not committed")
                    return after
                self.effect("replace", replace, current, wanted["image_id"])
                self.cp["accepted_state"] = self.cp["effects"][-1]["result"]
                self.cp["stage"] = "verify"
                self.save()
            if self.cp["stage"] == "verify":
                self.verify(wanted)
            require(self.observe(wanted["image_id"]) == self.cp["accepted_state"], "completed runtime identity drift")
            self.selector_state(self.cp["selector_after"])
            receipt = read_json(self.root / "receipt.json")
            if resume:
                journey = self.host.journey()
                require(self.observe(wanted["image_id"]) == self.cp["accepted_state"], "runtime drift during resumed journey")
                self.selector_state(self.cp["selector_after"])
                readback = {"verified_at": time.time(), "container": self.cp["accepted_state"]["container"],
                            "journey": journey, "historical_observation_not_restarted": True, "effects_replayed": False}
                atomic(self.root / ("resume-readback-" + str(uuid.uuid4()) + ".json"), readback)
                return dict(receipt, resume_readback=readback)
            return receipt

    def resume_pending(self, wanted):
        pending = self.cp["pending"]
        if pending:
            if pending["status"] == "proven-unsubmitted":
                # At most one deliberate resume retry. Reobserve all state before
                # continuing. No infrastructure retry after any submitted process.
                require(not self.cp.get("unsubmitted_retry_used"), "pre-submission retry exhausted")
                require(self.observe(self.d["current"]["image_id"], allow_stopped=self.cp["recovery"]) == self.cp["initial"],
                        "pre-submit runtime preimage drift")
                self.selector_state(self.cp.get("selector_after", self.cp["selector_before"]))
                self.cp["unsubmitted_retry_used"] = True
                self.cp["pending"] = None
                self.save()
                return
            if pending["kind"] == "persist":
                state = self.selector_state(pending["expected"])
                reconcile_effect(pending, state)
                self.host.persist_runtime_env(wanted["selector"])
                require(self.observe(self.d["current"]["image_id"], allow_stopped=self.cp["recovery"]) == self.cp["initial"], "ambiguous selector runtime drift")
                pending["result"] = state
                self.cp["selector_after"] = state
                self.cp["stage"] = "replace"
            elif pending["kind"] == "replace":
                # An identifiable stopped candidate can reconcile its committed
                # replacement, but it cannot pass verification. This records the
                # failure truthfully and enables separately approved recovery.
                state = self.observe(wanted["image_id"], allow_stopped=True)
                reconcile_effect(pending, state)
                self.selector_state(self.cp["selector_after"])
                require(state["restarts"] == 0, "ambiguous replacement restarted")
                pending["result"] = state
                self.cp["accepted_state"] = state
                self.cp["stage"] = "verify"
            else:
                self.observe(self.d["current"]["image_id"])
                # Observe artifact custody without treating a possibly partial
                # archive/listable TOC as proof the submitted backup completed.
                archive = self.root / (pending["operation_id"] + ".dump")
                if archive.exists():
                    custody(archive)
                raise Refuse("ambiguous recovery point: operator must reconcile native archive before proceeding")
            pending["status"] = "committed-reconciled"
            self.cp["effects"].append(pending)
            self.cp["pending"] = None
            self.save()
        # The process may die between effect readback and stage advancement.
        kinds = [e["kind"] for e in self.cp["effects"]]
        if self.cp["stage"] == "preflight" and "recovery-point" in kinds:
            require(self.cp.get("backup"), "recovery point evidence missing")
            self.cp["stage"] = "persist"
        if self.cp["stage"] == "persist" and "persist" in kinds:
            effect = next(e for e in self.cp["effects"] if e["kind"] == "persist")
            state = self.selector_state(effect["result"])
            require(state == effect["expected"], "committed native operation identity mismatch")
            self.cp["selector_after"] = state
            self.cp["stage"] = "replace"
        if self.cp["stage"] == "replace" and "replace" in kinds:
            effect = next(e for e in self.cp["effects"] if e["kind"] == "replace")
            require(self.observe(wanted["image_id"]) == effect["result"], "committed runtime identity drift")
            self.selector_state(self.cp["selector_after"])
            self.cp["accepted_state"] = effect["result"]
            self.cp["stage"] = "verify"
        self.save()

    def verify(self, wanted):
        # Any interrupted observation restarts this local interval only. It does
        # not replay selector/replacement or refresh a completed recovery point.
        first = self.observe(wanted["image_id"])
        require(first == self.cp["accepted_state"], "verification runtime differs from committed replacement")
        require(first["restarts"] == 0, "unexpected app restart")
        journey = self.host.journey()
        started = time.monotonic()
        deadline = started + self.d["observe_seconds"]
        self.cp["readiness_verified_at"] = time.time()
        self.cp["samples"] = [{"elapsed": 0, **first}]
        self.save()
        while time.monotonic() < deadline:
            self.stop()
            time.sleep(min(self.d["sample_seconds"], max(0, deadline - time.monotonic())))
            current = self.observe(wanted["image_id"])
            require(current == first, "unstable app identity/restart/protected state")
            self.cp["samples"].append({"elapsed": time.monotonic() - started, **current})
            self.save()
        require(len(self.cp["samples"]) >= 3, "insufficient stability samples")
        self.host.journey()
        require(self.observe(wanted["image_id"]) == first, "runtime drift during final journey")
        selector = self.selector_state(self.cp["selector_after"])
        require(selector["selector"] == wanted["selector"] and selector["version"] == self.cp["selector_after"]["version"], "final authoritative selector drift")
        receipt = {"schema": 1, "release_id": self.d["release_id"], "mode": self.d["mode"],
                   "status": "local-rehearsal-verified" if self.host.local else "production-verified",
                   "descriptor_sha256": digest(self.d), "source": wanted["source"], "image_id": wanted["image_id"],
                   "platform": wanted["platform"], "container": first["container"],
                   "approval_id": self.a["approval_id"], "approved_effects": self.a["effects"],
                   "stage_outcomes": [{"kind": e["kind"], "operation_id": e["operation_id"], "status": e["status"]} for e in self.cp["effects"]],
                   "recovery": self.cp["recovery"], "recovery_point": self.cp["backup"], "journey": journey,
                   "observation_seconds": self.cp["samples"][-1]["elapsed"], "samples": len(self.cp["samples"]),
                   "unexpected_restarts": 0, "protected_state_preserved": True, "exact_ledger": True,
                   "interruption_upper_bound_seconds": max(0, self.cp["readiness_verified_at"] - self.cp["replacement_started"]),
                   "production_stability_proven": not self.host.local,
                   "cleanup": "runner checkpoint/evidence retained; no resource cleanup or credential mutation"}
        atomic(self.root / "receipt.json", receipt)
        self.cp["stage"] = "complete"
        self.save()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("stage", nargs="?", default="prepare", choices=("prepare", "preflight", "release", "recover", "resume"))
    for name in ("descriptor", "evidence", "approvals", "target", "state_dir"):
        parser.add_argument("--" + name.replace("_", "-"), required=True, type=Path)
    parser.add_argument("--resume-recovery", action="store_true")
    parser.add_argument("--failed-state-dir", type=Path, help="explicit approved recovery handoff from this preserved release state")
    args = parser.parse_args()
    os.umask(0o077)
    inputs = None
    def stopped(sig, frame):
        if inputs is not None:
            atomic(Path(inputs[3]["lock_dir"]) / "STOP", {"global_owner_stop": True})
        raise Refuse("global owner stop/interruption")
    signal.signal(signal.SIGINT, stopped)
    signal.signal(signal.SIGTERM, stopped)
    try:
        inputs = [read_json(getattr(args, key)) for key in ("descriptor", "evidence", "approvals", "target")]
        runner = Runner(*inputs, args.state_dir)
        if args.stage == "prepare":
            result = runner.prepare()
        elif args.stage == "preflight":
            result = runner.preflight()
        else:
            result = runner.start(recovery=args.stage == "recover" or args.resume_recovery, resume=args.stage == "resume", failed_state=args.failed_state_dir)
        print(json.dumps(result, sort_keys=True))
    except (Refuse, OSError, KeyError, TypeError, ValueError, urllib.error.URLError):
        # Never print raw provider/subprocess/HTTP errors, private identifiers or
        # credentials. Full private checkpoint provides operation reconciliation.
        print(json.dumps({"status": "refused-or-unknown", "action": "inspect owner-only checkpoint; reconcile before retry"}))
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
