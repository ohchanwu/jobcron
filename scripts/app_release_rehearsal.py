#!/usr/bin/env python3
"""EXPLICIT owned local rehearsal; never a production adapter/approval generator.

This entry point is intentionally fixed to the card-allocated Compose project,
loopback ports and local Unix Docker transport. It refuses nonempty ownership
preimages. Secrets are generated only into the caller's owner-only scratch.
It keeps resources/evidence until an explicit exact-owned cleanup invocation.
"""
import argparse
import base64
import importlib.util
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import subprocess
import time

spec = importlib.util.spec_from_file_location("app_release", Path(__file__).with_name("app_release.py"))
assert spec is not None and spec.loader is not None
r = importlib.util.module_from_spec(spec)
spec.loader.exec_module(r)
PROJECT = "jobcron-runner-20261010-local1"
CARD = "t_6e112520"
BASELINE = "37b589bc671fe33e9928c492a1a5d29170a29484"
REPO = Path(__file__).resolve().parent.parent
WORKTREE = Path(os.environ.get("HERMES_KANBAN_WORKSPACE", "/nonexistent"))
SCRATCH = Path.home() / ".hermes/profiles/jobcron-worker/cache/scratch"
PG_IMAGE = "postgres@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15"
CADDY_IMAGE = "caddy@sha256:5f5c8640aae01df9654968d946d8f1a56c497f1dd5c5cda4cf95ab7c14d58648"
GO_IMAGE = "golang@sha256:a688600ca24f8a4d3ca77f95b0dd40704a9fc787c826660eb7ba0b641b8b175d"


class Fixture:
    def __init__(self, root):
        self.root = r.custody(root, directory=True)
        r.require(REPO == WORKTREE and self.root.parent == SCRATCH and
                  os.environ.get("HERMES_PROFILE") == "jobcron-worker" and
                  os.environ.get("HERMES_KANBAN_BOARD") == "jobcron" and os.environ.get("HERMES_TENANT") == "jobcron",
                  "exact worker worktree/scratch/board/profile/tenant required")
        self.docker_dir = self.directory("docker")
        r.require(not any(self.docker_dir.iterdir()), "fixture Docker config must be empty")
        self.docker_host = "unix://" + str(Path.home() / ".docker/run/docker.sock")
        self.env = {"DOCKER_HOST": self.docker_host, "DOCKER_CONFIG": str(self.docker_dir), "TMPDIR": str(self.root)}
        self.owner_path = self.root / "ownership.json"

    def directory(self, name):
        p = self.root / name
        if not p.exists():
            p.mkdir(mode=0o700)
        r.custody(p, directory=True)
        return p

    def run(self, args, data=None, timeout=120, env=None):
        return r.native(args, timeout, env={**self.env, **(env or {})}, data=data)

    def docker(self, *args, data=None):
        return self.run(["docker", *args], data=data)

    def compose(self, *args):
        return self.run(["/usr/local/bin/docker-compose", "--project-name", PROJECT, "--file", str(self.root / "compose.json"),
                         "--env-file", str(self.root / "compose.env"), *args])

    def inventories(self):
        result = {}
        for kind, command, field in (("containers", ["ps", "-a"], "{{.ID}}"),
                                     ("volumes", ["volume", "ls"], "{{.Name}}"),
                                     ("networks", ["network", "ls"], "{{.ID}}")):
            result[kind] = self.docker(*command, "--filter", "label=com.docker.compose.project=" + PROJECT,
                                      "--format", field).decode().split()
        return result

    def db_id(self):
        ids = self.compose("ps", "-q", "db").decode().split()
        r.require(len(ids) == 1, "one owned DB required")
        return ids[0]

    def sql(self, text):
        return self.docker("exec", "-i", self.db_id(), "psql", "-X", "-U", "postgres", "-d", "jobcron",
                           "-At", "-v", "ON_ERROR_STOP=1", data=text.encode())

    def setup(self):
        r.require(not self.owner_path.exists(), "existing fixture: do not recreate or overwrite evidence")
        r.require(not any(self.inventories().values()), "owned project preimage not empty")
        # Refuse exact name collisions even if malicious/missing labels obscure them.
        for name in (PROJECT + "_data", PROJECT + "_runtime"):
            command = ["docker", "volume" if name.endswith("data") else "network", "inspect", name]
            check = subprocess.run(command, stdin=subprocess.DEVNULL, capture_output=True,
                                   env={"PATH": "/usr/local/bin:/usr/bin:/bin", "HOME": "/nonexistent", **self.env}, timeout=20)
            missing = b"no such volume" if name.endswith("data") else b"not found"
            r.require(check.returncode != 0 and missing in check.stderr.lower(), "fixture name collision or unprovable preimage")
        for port in (17777, 17780):
            with socket.socket() as probe:
                probe.bind(("127.0.0.1", port))
        info = json.loads(self.docker("info", "--format", "{{json .}}"))
        r.require(info["OSType"] == "linux" and info["Architecture"] == "aarch64", "allocated Linux arm64 daemon required")
        ownership = {"schema": 1, "card": CARD, "project": PROJECT, "daemon_id": info["ID"],
                     "docker_host": self.docker_host, "root": str(self.root), "preimages": self.inventories(),
                     "ports": [17777, 17780], "labels": {"jobcron.card": CARD}}
        r.atomic(self.owner_path, ownership)
        for image in (PG_IMAGE, CADDY_IMAGE, GO_IMAGE):
            observed = json.loads(self.docker("image", "inspect", image))[0]
            r.require(image in observed["RepoDigests"] and observed["Architecture"] == "arm64", "cached immutable fixture image unavailable")
        inputs, tls, binaries = self.directory("input"), self.directory("tls"), self.directory("binaries")
        runtime_secrets = self.directory("secrets")
        pg_password, runtime_password, owner_password = [secrets.token_urlsafe(36) for _ in range(3)]
        for name, value in {
            "pg-password": pg_password, "owner-password": owner_password, "owner-email": "runner-fixture@example.invalid",
            "operator-url": "postgres://postgres@127.0.0.1:5432/jobcron?sslmode=require",
            "operator-owner-url": "postgres://postgres:" + pg_password + "@127.0.0.1:5432/jobcron?sslmode=require",
        }.items():
            r.atomic(inputs / name, value.encode(), raw=True)
        for name, value in {
            "DATABASE_URL": "postgres://fixture_runtime:" + runtime_password + "@db:5432/jobcron?sslmode=verify-full&sslrootcert=/run/jobcron/tls/origin.crt",
            "SESSION_SECRET": secrets.token_urlsafe(36), "JOBCRON_CREDENTIAL_ENCRYPTION_KEY": base64.b64encode(secrets.token_bytes(32)).decode(),
            "JOBCRON_SIGNUP_ACCESS_CODE": secrets.token_urlsafe(24), "JOBCRON_PROXY_SECRET": secrets.token_urlsafe(32),
        }.items():
            r.atomic(runtime_secrets / name, value.encode(), raw=True)
        r.atomic(inputs / "account.json", {"email": "runner-fixture@example.invalid", "password": owner_password})
        # Local self-signed CA/cert, not any real origin certificate/account.
        self.run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "2", "-subj", "/CN=runner-fixture",
                  "-addext", "subjectAltName=IP:127.0.0.1,DNS:db", "-keyout", str(tls / "origin.key"), "-out", str(tls / "origin.crt")])
        for p in tls.iterdir():
            p.chmod(0o600)
        self.build()

    def build(self):
        # Explicit continuation after a proven local, pre-resource build failure.
        # Keep ownership, synthetic inputs and rejected build evidence intact.
        owner = r.read_json(self.owner_path)
        r.require(owner["card"] == CARD and owner["root"] == str(self.root) and owner["project"] == PROJECT,
                  "fixture build continuation ownership mismatch")
        r.require(not any(self.inventories().values()) and not (self.root / "artifacts.json").exists(),
                  "fixture resources/artifacts already exist; do not repeat setup")
        inputs, tls, binaries = [self.root / n for n in ("input", "tls", "binaries")]
        runtime_secrets = self.root / "secrets"
        import urllib.parse
        runtime_password = urllib.parse.urlsplit(r.read_bytes(runtime_secrets / "DATABASE_URL").decode()).password
        r.require(runtime_password, "synthetic runtime input missing")
        # The app source is unchanged in this tooling slice. Both independent
        # binaries come from the exact protected Go baseline; only the explicit
        # version build parameter differs. This is not three aliases of one image.
        source = self.run(["git", "rev-parse", "HEAD"]).decode().strip()
        self.run(["git", "merge-base", "--is-ancestor", BASELINE, source])
        r.require(not self.run(["git", "diff", BASELINE, "--", "*.go", "go.mod", "go.sum", "internal/storage/migrations", "internal/storage/postgres_migrations"]), "application source must remain protected baseline")
        go = str(Path.home() / "go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.3.darwin-arm64/bin/go")
        r.require(self.run([go, "version"]).decode().strip() == "go version go1.26.3 darwin/arm64", "native toolchain mismatch")
        build_env = {"HOME": str(Path.home()), "CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": "arm64", "GOTOOLCHAIN": "local",
                     "GOPROXY": "off", "GOSUMDB": "off"}
        build_records = []
        for role in ("current", "candidate"):
            binary = binaries / ("jobcron-" + role)
            self.run([go, "build", "-ldflags", "-X main.version=runner-rehearsal-" + role,
                      "-o", str(binary), "./cmd/jobcron"], timeout=300, env=build_env)
            binary.chmod(0o600)
            build_records.append({"role": role, "source": source, "binary_sha256": r.file_digest(binary),
                                  "build_parameter": "main.version=runner-rehearsal-" + role})
        self.run([go, "build", "-o", str(binaries / "jobcron-user"), "./cmd/jobcron-user"], timeout=300, env=build_env)
        (binaries / "jobcron-user").chmod(0o600)
        artifacts = {}
        for role in ("current", "candidate"):
            context = self.directory("image-" + role)
            shutil.copyfile(binaries / ("jobcron-" + role), context / "jobcron")
            (context / "jobcron").chmod(0o755)
            # CGO-free fixture binaries need no base layers, pulls or anonymous
            # image-declared volumes. Trust roots come from the owned TLS mount.
            dockerfile = ("FROM scratch\nCOPY jobcron /usr/local/bin/jobcron\nENV HOME=/root PATH=/usr/local/bin\n"
                          "ENTRYPOINT [\"jobcron\"]\nLABEL jobcron.card=" + CARD + "\n")
            r.atomic(context / "Dockerfile", dockerfile.encode(), raw=True)
            self.run(["docker", "build", "--pull=false", "--network=none", "--tag", PROJECT + ":" + role, str(context)], timeout=300)
            image = json.loads(self.docker("image", "inspect", PROJECT + ":" + role))[0]
            artifacts[role] = {"source": source, "selector": image["Id"], "image_id": image["Id"], "platform": image["Os"] + "/" + image["Architecture"]}
        artifacts["recovery"] = artifacts["current"]
        r.require(artifacts["candidate"]["image_id"] != artifacts["current"]["image_id"], "independent candidate build required")
        r.atomic(self.root / "artifacts.json", artifacts)
        r.atomic(self.root / "build-records.json", build_records)
        r.atomic(self.root / "compose.env", ("JOBCRON_IMAGE=" + artifacts["current"]["selector"] + "\nJOBCRON_STAGE1_SPONSOR_USER_ID=1\n").encode(), raw=True)
        r.atomic(self.root / "proxy-header", b"header_up X-Jobcron-Proxy " + r.read_bytes(runtime_secrets / "JOBCRON_PROXY_SECRET") + b"\n", raw=True)
        r.atomic(self.root / "Caddyfile", b"{\n auto_https off\n}\n:8443 {\n tls /fixture-tls/origin.crt /fixture-tls/origin.key\n reverse_proxy app:7777 {\n import /fixture-proxy-header\n }\n}\n:8080 {\n reverse_proxy app:7777 {\n import /fixture-proxy-header\n }\n}\n", raw=True)
        # Copy fixture shell into private input; container invokes /bin/sh, never
        # an executable bit or foreign script. No host credentials mounted.
        r.atomic(inputs / "postgres.sh", (REPO / "scripts/app_release_fixture/postgres.sh").read_bytes(), raw=True)
        labels = {"jobcron.card": CARD}
        def bind(source, destination):
            return {"type": "bind", "source": str(source), "target": destination, "read_only": True, "bind": {"create_host_path": False}}
        environment = {"JOBCRON_ENV": "production", "JOBCRON_SCHEDULER_ENABLED": "1", "JOBCRON_DAILY_SCRAPE_TIME": "23:59",
                       "JOBCRON_STAGE1_SPONSOR_USER_ID": "${JOBCRON_STAGE1_SPONSOR_USER_ID}", "JOBCRON_NO_OPEN": "1", "AWS_EC2_METADATA_DISABLED": "true"}
        for name in ("DATABASE_URL", "SESSION_SECRET", "JOBCRON_CREDENTIAL_ENCRYPTION_KEY", "JOBCRON_SIGNUP_ACCESS_CODE", "JOBCRON_PROXY_SECRET"):
            environment[name + "_FILE"] = "/run/jobcron/secrets/" + name
        compose = {"services": {
            "db": {"image": PG_IMAGE, "pull_policy": "never", "entrypoint": ["/bin/sh", "/fixture-input/postgres.sh"], "labels": labels,
                   "environment": {"POSTGRES_PASSWORD_FILE": "/fixture-input/pg-password", "POSTGRES_DB": "jobcron"},
                   "volumes": [bind(inputs, "/fixture-input"), bind(tls, "/fixture-tls"), bind(binaries, "/fixture-binaries"),
                               {"type": "volume", "source": "data", "target": "/var/lib/postgresql"}],
                   "networks": ["runtime"]},
            "app": {"image": "${JOBCRON_IMAGE}", "pull_policy": "never", "command": ["--no-open", "--host", "0.0.0.0", "--port", "7777"],
                    "environment": environment, "volumes": [bind(runtime_secrets, "/run/jobcron/secrets"), bind(tls, "/run/jobcron/tls")],
                    "labels": labels, "networks": ["runtime"], "restart": "no", "ulimits": {"core": {"hard": 0, "soft": 0}}},
            "caddy": {"image": CADDY_IMAGE, "pull_policy": "never", "labels": labels,
                      "ports": [{"target": 8443, "published": "17780", "host_ip": "127.0.0.1", "protocol": "tcp"},
                                {"target": 8080, "published": "17777", "host_ip": "127.0.0.1", "protocol": "tcp"}],
                      "volumes": [bind(self.root / "Caddyfile", "/etc/caddy/Caddyfile"), bind(tls, "/fixture-tls"),
                                  bind(self.root / "proxy-header", "/fixture-proxy-header")],
                      "networks": ["runtime", "ingress"], "restart": "no"}},
            "networks": {"runtime": {"internal": True, "labels": labels}, "ingress": {"labels": labels}}, "volumes": {"data": {"labels": labels}}}
        r.atomic(self.root / "compose.json", compose)
        self.bootstrap()

    def bootstrap(self):
        owner = r.read_json(self.owner_path)
        r.require(owner["card"] == CARD and owner["root"] == str(self.root) and
                  owner["daemon_id"] == json.loads(self.docker("info", "--format", "{{json .}}"))["ID"], "bootstrap ownership mismatch")
        r.require(not (self.root / "setup-complete.json").exists() and
                  not (self.root / "bootstrap-submitted.json").exists(), "bootstrap already submitted; reconcile, never replay")
        for identifier in self.inventories()["containers"]:
            obj = json.loads(self.docker("inspect", identifier))[0]
            r.require(obj["Config"]["Labels"].get("jobcron.card") == CARD and
                      obj["Config"]["Labels"].get("com.docker.compose.service") == "db", "foreign or app fixture already exists")
        inputs = self.root / "input"
        old = r.read_bytes(inputs / "postgres.sh")
        if old != (REPO / "scripts/app_release_fixture/postgres.sh").read_bytes():
            r.atomic(self.root / ("rejected-postgres-" + __import__("hashlib").sha256(old).hexdigest() + ".sh"), old, raw=True)
            r.atomic(inputs / "postgres.sh", (REPO / "scripts/app_release_fixture/postgres.sh").read_bytes(), raw=True)
        self.compose("up", "--detach", "--no-build", "--pull", "never", "db")
        deadline = time.monotonic() + 60
        while True:
            try:
                self.docker("exec", self.db_id(), "pg_isready", "-U", "postgres", "-d", "jobcron")
                break
            except r.Refuse:
                r.require(time.monotonic() < deadline, "owned DB readiness timeout")
                time.sleep(1)
        exists = self.docker("exec", self.db_id(), "psql", "-X", "-U", "postgres", "-At", "-c", "SELECT count(*) FROM pg_database WHERE datname='jobcron';")
        if exists.strip() == b"0":
            self.docker("exec", self.db_id(), "createdb", "-U", "postgres", "jobcron")
        r.require(self.sql("SELECT count(*) FROM pg_tables WHERE schemaname='public';").strip() == b"0", "nonempty bootstrap database; reconcile")
        r.atomic(self.root / "bootstrap-submitted.json", {"card": CARD, "status": "submitted-unknown"})
        # Native supported jobcron-user operations on the owned container only.
        # Copy executable to its writable /tmp; binaries mount remains 0600.
        self.docker("exec", self.db_id(), "sh", "-c", "cp /fixture-binaries/jobcron-user /tmp/jobcron-user && chmod 700 /tmp/jobcron-user")
        self.docker("exec", "-e", "DATABASE_URL_FILE=/fixture-input/operator-url", "-e", "JOBCRON_DATABASE_PASSWORD_FILE=/fixture-input/pg-password",
                    self.db_id(), "/tmp/jobcron-user", "migrate")
        self.docker("exec", "-e", "DATABASE_URL_FILE=/fixture-input/operator-owner-url", "-e", "JOBCRON_OWNER_PASSWORD_FILE=/fixture-input/owner-password",
                    "-e", "JOBCRON_OWNER_EMAIL_FILE=/fixture-input/owner-email", self.db_id(), "/tmp/jobcron-user", "create-owner")
        import urllib.parse
        runtime_password = urllib.parse.urlsplit(r.read_bytes(self.root / "secrets/DATABASE_URL").decode()).password
        self.sql("CREATE ROLE fixture_runtime LOGIN PASSWORD '" + runtime_password + "';\n"
                 "REVOKE ALL ON SCHEMA public FROM PUBLIC; GRANT USAGE ON SCHEMA public TO fixture_runtime;\n"
                 "GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO fixture_runtime;\n"
                 "REVOKE INSERT,UPDATE,DELETE ON schema_migrations FROM fixture_runtime;\n"
                 "GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA public TO fixture_runtime;\n")
        # Real disposable restore, distinct from pg_restore --list or a claim.
        dump = self.docker("exec", self.db_id(), "pg_dump", "-U", "postgres", "-d", "jobcron", "-Fc")
        r.atomic(self.root / "accepted-restore.dump", dump, raw=True)
        self.docker("exec", self.db_id(), "createdb", "-U", "postgres", "restore_check")
        self.docker("exec", "-i", self.db_id(), "pg_restore", "-U", "postgres", "-d", "restore_check", "--exit-on-error", data=dump)
        restored = self.docker("exec", self.db_id(), "psql", "-X", "-U", "postgres", "-d", "restore_check", "-At", "-c", r.LEDGER_SQL)
        r.require(json.loads(restored) == json.loads(self.sql(r.LEDGER_SQL)), "restored exact ledger mismatch")
        self.docker("exec", self.db_id(), "dropdb", "-U", "postgres", "restore_check")
        self.compose("up", "--detach", "--no-build", "--pull", "never", "app", "caddy")
        self.wait_http()
        self.make_target()
        r.atomic(self.root / "setup-complete.json", {"status": "owned-local-only", "real_disposable_restore": True,
                 "database_host_publication": False, "network_internal": True, "application_sources": "same protected baseline; independent version-parameter builds"})

    def wait_http(self):
        import urllib.request
        deadline = time.monotonic() + 60
        while True:
            try:
                request = urllib.request.Request("http://127.0.0.1:17777/login", headers={"X-Jobcron-Proxy": r.read_bytes(self.root / "secrets/JOBCRON_PROXY_SECRET").decode()})
                opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
                with opener.open(request, timeout=3) as response:
                    r.require(response.status == 200, "fixture HTTP not ready")
                return
            except Exception:
                r.require(time.monotonic() < deadline, "fixture HTTP readiness timeout")
                time.sleep(1)

    def manifests(self):
        result = {}
        for key, directory in (("postgres", "postgres_migrations"), ("sqlite", "migrations")):
            path = "internal/storage/" + directory
            result[key] = {"tree": self.run(["git", "rev-parse", BASELINE + ":" + path]).decode().strip(),
                           "files": [{"name": p.name, "sha256": __import__("hashlib").sha256(p.read_bytes()).hexdigest()}
                                     for p in sorted((REPO / path).glob("*.sql"))]}
        r.validate_manifests(result)
        return result

    def make_target(self):
        target = {"target": PROJECT, "project": PROJECT, "docker_host": self.docker_host,
                  "daemon_id": r.read_json(self.owner_path)["daemon_id"], "compose_file": str(self.root / "compose.json"),
                  "compose_executable": "/usr/local/bin/docker-compose",
                  "compose_sha256": r.file_digest(self.root / "compose.json"), "compose_env": str(self.root / "compose.env"),
                  "selector_file": str(self.root / "compose.env"), "lock_dir": str(self.directory("target-lock")),
                  "config_sha256": "", "protected_sha256": "", "protected_paths": [str(self.root / n) for n in ("secrets", "tls", "Caddyfile", "proxy-header")],
                  "journey_url": "https://127.0.0.1:17780", "journey_ca_file": str(self.root / "tls/origin.crt"),
                  "journey_account_file": str(self.root / "input/account.json"), "proxy_secret_file": str(self.root / "secrets/JOBCRON_PROXY_SECRET")}
        d = {"mode": "local", "operation_timeout": 120, "manifests": self.manifests()}
        h = r.Host(d, target, self.directory("target-read"))
        target["config_sha256"] = h.render()
        app, proxy, db = [h.service(n) for n in ("app", "caddy", "db")]
        protected = {"proxy": r.metadata(proxy, ("Id", "Image", "Mounts", "Config", "HostConfig")),
                     "app": r.metadata(app, ("Mounts", "HostConfig")), "app_env": r.environment(app["Config"]["Env"]), "app_cmd": app["Config"]["Cmd"],
                     "secrets": h.mount_files(), "database": r.metadata(db, ("Id", "Image", "Mounts", "Config", "HostConfig"))}
        target["protected_sha256"] = r.digest(protected)
        r.atomic(self.root / "initial-protected.json", protected)
        # Confinement proves no external scrape/provider route; no probe of a
        # real destination is necessary or permitted. The only app network is
        # owned+internal and no privileged/host/network namespace exists.
        networks = self.docker("network", "inspect", PROJECT + "_runtime")
        r.require(json.loads(networks)[0]["Internal"] is True and set(app["NetworkSettings"]["Networks"]) == {PROJECT + "_runtime"}, "egress confinement failed")
        r.require(not db["HostConfig"]["PortBindings"], "DB host publication prohibited")
        r.atomic(self.root / "target.json", target)
        self.original = h.snapshot()
        r.atomic(self.root / "initial-readback.json", {"app": self.original, "proxy_container": proxy["Id"], "db_container": db["Id"],
                                                     "internal_network": True, "one_scheduler": True})

    def inputs(self, name, current, candidate, recovery):
        root = self.directory(name)
        artifacts = r.read_json(self.root / "artifacts.json")
        target = r.read_json(self.root / "target.json")
        manifests = self.manifests()
        d = {"schema": 1, "release_id": name, "mode": "local", "target": PROJECT, "current": artifacts[current],
             "candidate": artifacts[candidate], "recovery": artifacts[recovery], "manifests": manifests,
             "config_sha256": target["config_sha256"], "operation_timeout": 120, "observe_seconds": 2, "sample_seconds": 0.5}
        evidence = {"target": PROJECT, "mode": "local", "restore": {"accepted": True, "manifests": manifests, "target": PROJECT,
                    "record": "owned real pg_restore+exact-ledger readback; accepted-restore.dump"}}
        for role in ("current", "candidate", "recovery"):
            artifact = d[role]
            evidence[role] = {"artifact": artifact, "manifests": manifests}
            for gate in ("ci", "review", "publication"):
                evidence[role][gate] = {"source": artifact["source"], "result": "accepted", "kind": "local-rehearsal",
                                       "record": "local fixture authority only; build-records.json; not independent production CI/review"}
            evidence[role]["publication"].update({"selector": artifact["selector"], "platform": artifact["platform"]})
        approval = {"descriptor_sha256": r.digest(d), "target": PROJECT, "approval_id": CARD + "-owner-local-rehearsal-" + name,
                    "effects": ["observe", "recovery-point", "persist", "replace", "recover"]}
        for filename, value in (("descriptor", d), ("evidence", evidence), ("approvals", approval), ("target", target)):
            path = root / (filename + ".json")
            if path.exists():
                r.require(r.read_json(path) == value, "existing rehearsal inputs changed; preserve and reconcile")
            else:
                r.atomic(path, value)
        return root, d, evidence, approval, target

    def sequence(self):
        receipts = []
        stages = (
            ("forward-1", "current", "candidate", "recovery", False),
            ("compatible-recovery", "candidate", "candidate", "recovery", True),
            ("forward-2", "recovery", "candidate", "recovery", False))
        for index, (name, current, candidate, recovery, is_recovery) in enumerate(stages):
            root, d, e, a, t = self.inputs(name, current, candidate, recovery)
            r.atomic(root / "prepare.json", r.Runner(d, e, a, t, root).prepare())
            if (root / "checkpoint.json").exists():
                checkpoint = r.read_json(root / "checkpoint.json")
                if checkpoint["stage"] == "complete" and any((self.root / later[0] / "checkpoint.json").exists() for later in stages[index + 1:]):
                    # Later stages deliberately changed the current image. Keep
                    # the earlier operation's historical receipt, never replay
                    # it. The active/final stage reobserves actual native state.
                    receipt = r.read_json(root / "receipt.json")
                    r.require(receipt["descriptor_sha256"] == r.digest(d), "historical receipt binding changed")
                else:
                    receipt = r.Runner(d, e, a, t, root).start(recovery=is_recovery, resume=True)
                if name == "forward-1":
                    r.require(r.read_json(root / "lost-reply-readback.json")["same_container_before_after_resume"], "lost reply evidence missing")
                r.require(self.sql("SELECT profile_json FROM profiles WHERE user_id=1;").decode().strip() == '{"min_score":17}', "post-change write lost")
            elif name == "forward-1":
                # The native Compose effect commits. Only its local reply is
                # discarded here, in the owned fixture subclass (not production).
                class LostReply(r.Host):
                    def replace(self):
                        super().replace()
                        raise r.Unknown("injected locally lost native Compose reply")
                try:
                    r.Runner(d, e, a, t, root, host_class=LostReply).start()
                    raise AssertionError("lost reply injection did not stop")
                except r.Unknown:
                    checkpoint = r.read_json(root / "checkpoint.json")
                    r.require(checkpoint["pending"]["kind"] == "replace", "lost native reply not retained")
                self.wait_http()
                before_resume = r.Host(d, t, self.directory("resume-read")).snapshot()
                receipt = r.Runner(d, e, a, t, root).start(resume=True)
                after_resume = r.Host(d, t, self.directory("resume-read")).snapshot()
                r.require(before_resume == after_resume, "resume duplicated committed replacement")
                r.atomic(root / "lost-reply-readback.json", {"same_container_before_after_resume": True, "container": after_resume["container"],
                         "native_effect": "Compose up --no-deps app actually committed", "transport_fault": "local reply discarded", "provider_permissions_proven": False})
                # A post-change valuable write, not restored from an old dump.
                self.sql("INSERT INTO profiles(user_id,profile_json,profile_hash,updated_at) VALUES(1,'{\"min_score\":17}', 'fixture-post-change', now());")
            else:
                receipt = r.Runner(d, e, a, t, root).start(recovery=is_recovery)
                r.require(self.sql("SELECT profile_json FROM profiles WHERE user_id=1;").decode().strip() == '{"min_score":17}', "post-change write lost across recovery")
            receipts.append(receipt)
        self.unknown_refusal()
        # Full receipts deliberately contain no private selector paths/config/
        # secret digests or credential/cookie values. Inputs remain owner-only.
        r.atomic(self.root / "rehearsal-summary.json", {"card": CARD, "mode": "local-only", "receipts": receipts,
                 "sequence": "current -> candidate -> compatible recovery -> candidate", "post_change_write_preserved": True,
                 "lost_reply_reconciled_without_replay": True, "unknown_state_refused": True,
                 "same_application_source_baseline": BASELINE, "independent_binary_build_parameters": True,
                 "production_stability_or_IAM_proven": False, "resources": "retained for review; exact owned cleanup only"})

    def unknown_refusal(self):
        root, d, e, a, t = self.inputs("unknown-refusal", "candidate", "current", "recovery")
        # Create submitted intent against an unchanged real app. Its readback is
        # preimage, so the runner must not replay or declare no submission.
        host = r.Host(d, t, self.directory("unknown-read"))
        before = host.snapshot()
        selector = host.selector()
        checkpoint = {"schema": 1, "target": PROJECT, "descriptor_sha256": r.digest(d), "evidence_sha256": r.digest(e),
                      "target_sha256": r.digest(t), "recovery": False, "stage": "replace", "effects": [],
                      "initial": before, "selector_before": selector,
                      "pending": {"kind": "replace", "operation_id": "locally-ambiguous-not-noop", "status": "submitted-unknown",
                                  "before": before, "after": d["candidate"]["image_id"]}}
        if (root / "checkpoint.json").exists():
            r.require(r.read_json(root / "checkpoint.json") == checkpoint, "existing unknown intent changed; do not overwrite")
        else:
            r.atomic(root / "checkpoint.json", checkpoint)
        try:
            r.Runner(d, e, a, t, root).start(resume=True)
            raise AssertionError("unknown state accepted")
        except r.Refuse:
            r.require(host.snapshot() == before and host.selector() == selector, "unknown-state refusal performed effect")
            r.atomic(root / "refusal.json", {"refused": True, "native_state_unchanged": True, "effect_replayed": False})

    def cleanup(self):
        owner = r.read_json(self.owner_path)
        r.require(owner["card"] == CARD and owner["project"] == PROJECT and owner["root"] == str(self.root) and
                  owner["docker_host"] == self.docker_host and owner["daemon_id"] == json.loads(self.docker("info", "--format", "{{json .}}"))["ID"], "cleanup ownership mismatch")
        inventory = self.inventories()
        for identifier in inventory["containers"]:
            obj = json.loads(self.docker("inspect", identifier))[0]
            r.require(obj["Config"]["Labels"].get("jobcron.card") == CARD, "foreign container cleanup refused")
        for kind in ("volumes", "networks"):
            for identifier in inventory[kind]:
                obj = json.loads(self.docker("volume" if kind == "volumes" else "network", "inspect", identifier))[0]
                r.require(obj["Labels"].get("jobcron.card") == CARD, "foreign resource cleanup refused")
        # Exact owned IDs only. This is fixture cleanup, never release recovery.
        anonymous = []
        for identifier in inventory["containers"]:
            obj = json.loads(self.docker("inspect", identifier))[0]
            anonymous.extend(m["Name"] for m in obj["Mounts"] if m["Type"] == "volume" and m["Name"] not in inventory["volumes"])
        r.atomic(self.root / "cleanup-owned-preimage.json", {"card": CARD, "inventory": inventory, "anonymous_attached_to_owned_containers": anonymous})
        for identifier in inventory["containers"]:
            self.docker("rm", "--force", "--volumes", identifier)
        for identifier in inventory["networks"]:
            self.docker("network", "rm", identifier)
        for identifier in inventory["volumes"]:
            self.docker("volume", "rm", identifier)
        r.require(not any(self.inventories().values()), "owned cleanup readback nonempty")
        for name in anonymous:
            check = subprocess.run(["docker", "volume", "inspect", name], stdin=subprocess.DEVNULL, capture_output=True,
                                   env={"PATH": "/usr/local/bin:/usr/bin:/bin", "HOME": "/nonexistent", **self.env}, timeout=20)
            r.require(check.returncode != 0 and b"no such volume" in check.stderr.lower(), "owned anonymous volume cleanup unproven")
        r.atomic(self.root / "cleanup.json", {"card": CARD, "containers_volumes_networks_removed": True,
                 "anonymous_volumes_removed_with_owned_containers": len(anonymous), "owned_images_retained_for_review": True, "evidence_retained": True})


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("stage", choices=("setup", "build", "bootstrap", "sequence", "cleanup"))
    p.add_argument("--root", required=True, type=Path)
    args = p.parse_args()
    os.umask(0o077)
    fixture = Fixture(args.root)
    getattr(fixture, args.stage)()
    print(json.dumps({"stage": args.stage, "status": "passed", "mode": "owned-local-only"}))


if __name__ == "__main__":
    try:
        main()
    except (r.Refuse, OSError, ValueError, KeyError) as error:
        import traceback
        print(json.dumps({"status": "fixture-refused-or-unknown", "action": "inspect private inputs; preserve ownership/evidence",
                          "reason": str(error) if isinstance(error, r.Refuse) else type(error).__name__,
                          "frames": [{"function": f.name, "line": f.lineno} for f in traceback.extract_tb(error.__traceback__)]}))
        raise SystemExit(1)
