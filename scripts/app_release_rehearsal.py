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
                  "compose_executable": "/usr/local/bin/docker-compose", "compose_version": self.compose("version").decode().strip(),
                  "compose_sha256": r.file_digest(self.root / "compose.json"), "compose_env": str(self.root / "compose.env"),
                  "selector_file": str(self.root / "compose.env"), "lock_dir": str(self.directory("target-lock")),
                  "config_sha256": "", "protected_sha256": "", "protected_paths": [str(self.root / n) for n in ("secrets", "tls", "Caddyfile", "proxy-header")],
                  "protected_public_paths": [str(self.root / "Caddyfile")],
                  "protected_metadata_file": str(self.root / "initial-protected.json"),
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

    def attended_inputs(self, root, d, e, t, action="accept-effective-state"):
        # LOCAL fixture operator simulation only. The fault runs AFTER a real,
        # awaited Compose success. No production approval generator.
        r.require(d["mode"] == "local" and t["project"] == PROJECT, "local attended inputs only")
        cp = r.read_json(root / "checkpoint.json")
        process = cp["pending"]["native_process"]
        r.require(process["terminal"] is True and process.get("exit_code") == 0, "fixture native effect not settled")
        try:
            os.kill(process["pid"], 0)
        except ProcessLookupError:
            pass
        else:
            raise r.Refuse("fixture native client still alive")
        host = r.Host(d, t, self.directory("attended-read"))
        observed = host.snapshot(allow_stopped=True, allow_absent=True)
        time.sleep(0.2)
        r.require(host.snapshot(allow_stopped=True, allow_absent=True) == observed, "unstable attended fixture state")
        binding = {"target": d["target"], "descriptor_sha256": r.digest(d), "evidence_sha256": r.digest(e),
                   "target_sha256": r.digest(t), "checkpoint_sha256": r.file_digest(root / "checkpoint.json"),
                   "sequence": cp["sequence"], "operation_id": cp["pending"]["operation_id"], "artifact": cp["wanted"],
                   "observed": observed, "selector": host.selector()}
        evidence = dict(binding, schema=1, native_invocation_terminated=True, native_effects_settled=True, exclusive_operator=True,
                        record="LOCAL synthetic attended inspection: awaited owned native command exit0 before fault injection; terminated PID; stable exclusive owned readback; not production authority")
        path = root / "attended-evidence.json"
        r.atomic(path, evidence)
        approval = dict(binding, schema=1, disposition=action, authority=action, approval_id=CARD + "-local-attended-disposition",
                        evidence_file=str(path), evidence_sha256_file=r.file_digest(path))
        r.atomic(root / "attended-disposition.json", approval)
        return approval

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
                class CrashBackup(r.Runner):
                    def commit_effect(self, pending):
                        if pending["kind"] == "recovery-point":
                            raise r.Unknown("local crash after durable backup outcome/readback")
                        super().commit_effect(pending)
                try:
                    CrashBackup(d, e, a, t, root).start()
                    raise AssertionError("backup pair crash not injected")
                except r.Unknown:
                    backup_op = r.read_json(root / "checkpoint.json")["pending"]["operation_id"]
                try:
                    r.Runner(d, e, a, t, root, host_class=LostReply).start(resume=True)
                    raise AssertionError("lost reply injection did not stop")
                except r.Unknown:
                    checkpoint = r.read_json(root / "checkpoint.json")
                    r.require(checkpoint["pending"]["kind"] == "replace", "lost native reply not retained")
                    r.require(checkpoint["backup"]["operation_id"] == backup_op and
                              len(list(root.glob("*.dump"))) == 1, "durable backup replayed")
                    r.atomic(root / "backup-pair-readback.json", {"durable_pair_reconciled": True, "backup_not_replayed": True})
                self.wait_http()
                before_resume = r.Host(d, t, self.directory("resume-read")).snapshot()
                original = r.file_digest(root / "checkpoint.json")
                try:
                    r.Runner(d, e, a, t, root).start(resume=True)
                    raise AssertionError("unbound lost replacement adopted automatically")
                except r.Refuse:
                    r.require(r.file_digest(root / "checkpoint.json") == original, "refusal rewrote unknown checkpoint")
                approval = self.attended_inputs(root, d, e, t)
                receipt = r.Runner(d, e, a, t, root).start(resume=True, disposition=approval)
                after_resume = r.Host(d, t, self.directory("resume-read")).snapshot()
                r.require(before_resume == after_resume, "resume duplicated committed replacement")
                r.atomic(root / "lost-reply-readback.json", {"same_container_before_after_resume": True, "container": after_resume["container"],
                         "native_effect": "Compose up --no-deps app actually committed", "transport_fault": "local reply discarded", "provider_permissions_proven": False,
                         "automatic_adoption_refused": True, "explicit_local_attended_resolution": True, "causal_container_provenance_claimed": False})
                # A post-change valuable write, not restored from an old dump.
                self.sql("INSERT INTO profiles(user_id,profile_json,profile_hash,updated_at) VALUES(1,'{\"min_score\":17}', 'fixture-post-change', now());")
            else:
                if name == "forward-2":
                    class CrashCommit(r.Runner):
                        def commit_effect(self, pending):
                            if pending["kind"] == "replace":
                                raise r.Unknown("local crash AFTER durable native success/postimage pair")
                            super().commit_effect(pending)
                    try:
                        CrashCommit(d, e, a, t, root).start()
                        raise AssertionError("durable-pair crash did not occur")
                    except r.Unknown:
                        pass
                    before = r.Host(d, t, self.directory("durable-read")).snapshot()
                    receipt = r.Runner(d, e, a, t, root).start(resume=True)
                    r.require(r.Host(d, t, self.directory("durable-read")).snapshot() == before, "durable-pair resume replayed replacement")
                    r.atomic(root / "delivered-pair-readback.json", {"native_success_postimage_pair": True, "same_container_after_resume": True})
                else:
                    receipt = r.Runner(d, e, a, t, root).start(recovery=is_recovery)
                r.require(self.sql("SELECT profile_json FROM profiles WHERE user_id=1;").decode().strip() == '{"min_score":17}', "post-change write lost across recovery")
            receipts.append(receipt)
        failure_receipt = self.failed_recovery()
        pending_pair_receipt = self.pending_pair_recovery()
        intermediate_receipt = self.intermediate_recovery()
        old_stopped_receipt = self.old_stopped_recovery()
        self.unknown_refusal()
        # Full receipts deliberately contain no private selector paths/config/
        # secret digests or credential/cookie values. Inputs remain owner-only.
        r.atomic(self.root / "rehearsal-summary.json", {"card": CARD, "mode": "local-only", "receipts": receipts,
                 "sequence": "current -> candidate -> compatible recovery -> candidate", "post_change_write_preserved": True,
                 "lost_reply_reconciled_without_replay": True, "unknown_state_refused": True,
                 "failed_stopped_candidate_approved_recovery": failure_receipt,
                 "pending_pair_stopped_candidate_approved_recovery": pending_pair_receipt,
                 "absent_intermediate_attended_recovery": intermediate_receipt,
                 "old_app_stopped_before_completion_recovery": old_stopped_receipt,
                 "same_application_source_baseline": BASELINE, "independent_binary_build_parameters": True,
                 "production_stability_or_IAM_proven": False, "resources": "retained for review; exact owned cleanup only"})

    def failed_recovery(self):
        # Exact owned native effects; the acceptance fault is locally injected,
        # not a claim that the real application image itself is defective.
        root, d, e, a, t = self.inputs("failed-acceptance", "candidate", "current", "recovery")
        r.require(not (root / "checkpoint.json").exists(), "failed-acceptance already exercised; retain evidence")
        class MissingPrerequisite(r.Host):
            def prereqs(self):
                self.run(["/definitely/missing/fixture-prerequisite"])
        try:
            r.Runner(d, e, a, t, root, host_class=MissingPrerequisite).preflight()
            raise AssertionError("missing prerequisite accepted")
        except r.Unsubmitted:
            r.require(not (Path(t["lock_dir"]) / "lock.json").exists() and not (root / "checkpoint.json").exists(),
                      "read-only failure stranded lock")
        r.atomic(root / "readonly-failure.json", {"effects_submitted": False, "lock_released_with_history": True})
        r.Runner(d, e, a, t, root).preflight()
        class FailedJourney(r.Host):
            def replace(self):
                super().replace()  # REAL owned Compose commits first.
                self.run(["/definitely/missing/fixture-late-readback-helper"])
            def journey(self):
                super().journey()  # The actual synthetic account/HTTPS path works.
                raise r.Refuse("locally injected acceptance failure after native replacement")
        runner = r.Runner(d, e, a, t, root, host_class=FailedJourney)
        try:
            runner.start()
            raise AssertionError("late missing helper accepted")
        except r.Unsubmitted:
            cp = r.read_json(root / "checkpoint.json")
            r.require(cp["pending"]["kind"] == "replace" and cp["pending"]["status"] == "submitted-unknown",
                      "late missing helper mislabeled committed replacement")
        host = r.Host(d, t, self.directory("failed-read"))
        before_resume = host.snapshot()
        try:
            runner.start(resume=True)
            raise AssertionError("late helper automatically adopted unknown replacement")
        except r.Refuse:
            r.require(host.snapshot() == before_resume, "unknown refusal replayed replacement")
        approval = self.attended_inputs(root, d, e, t)
        try:
            runner.start(resume=True, disposition=approval)
            raise AssertionError("injected failed journey accepted")
        except r.Refuse:
            cp = r.read_json(root / "checkpoint.json")
            r.require(cp["stage"] == "verify" and cp["pending"] is None and len(cp["effects"]) == 3,
                      "failed acceptance not fully committed")
        r.require(host.snapshot() == before_resume, "late-helper reconciliation replayed native replacement")
        r.atomic(root / "late-helper-reconciliation.json", {"real_native_replacement_committed": True,
                 "later_exec_failed_before_that_helper_only": True, "effect_retained_submitted_unknown": True,
                 "resume_same_container": before_resume["container"], "effects_replayed": False})
        self.sql("UPDATE profiles SET profile_json='{\"min_score\":19}', profile_hash='after-failed-candidate' WHERE user_id=1;")
        committed = host.snapshot()
        failed_hash = r.file_digest(root / "checkpoint.json")
        # Stop only the exact recorded owned app, leaving Caddy/DB/mounts intact.
        app = host.inspect(committed["container"])
        r.require(app["Config"]["Labels"].get("jobcron.card") == CARD, "failed app not owned")
        self.docker("stop", "--time", "10", committed["container"])
        stopped = host.snapshot(allow_stopped=True)
        r.require(not stopped["running"] and stopped["container"] == committed["container"], "failed app stop unproven")
        r.atomic(root / "failure-readback.json", {"committed": committed, "stopped": stopped, "fault": "locally injected acceptance failure", "owned_native_stop": True})
        recovery_root, rd, re, ra, rt = self.inputs("failed-compatible-recovery", "current", "candidate", "candidate")
        ra["recovery_preimage"] = stopped
        r.atomic(recovery_root / "approvals.json", ra)
        # Real CLI invocation of the new approved handoff, not a command emitter.
        args = ["/usr/bin/python3", str(REPO / "scripts/app_release.py"), "recover", "--failed-state-dir", str(root),
                "--state-dir", str(recovery_root)]
        for name in ("descriptor", "evidence", "approvals", "target"):
            args += ["--" + name, str(recovery_root / (name + ".json"))]
        result = json.loads(self.run(args, timeout=180))
        r.require(result["recovery"] and result["status"] == "local-rehearsal-verified", "native approved recovery failed")
        r.require(r.file_digest(root / "checkpoint.json") == failed_hash, "failed checkpoint overwritten")
        r.require(self.sql("SELECT profile_json FROM profiles WHERE user_id=1;").decode().strip() == '{"min_score":19}',
                  "new write lost after failed candidate recovery")
        before = r.Host(rd, rt, self.directory("completed-read")).snapshot()
        resumed = r.Runner(rd, re, ra, rt, recovery_root).start(recovery=True, resume=True)
        after = r.Host(rd, rt, self.directory("completed-read")).snapshot()
        r.require(before == after and resumed["resume_readback"]["effects_replayed"] is False, "completed recovery replayed")
        r.atomic(recovery_root / "recovery-preservation.json", {"same_failed_checkpoint": True, "post_change_write_preserved": True,
                 "proxy_database_mounts_ledger_preserved": True, "one_confined_scheduler": True,
                 "completed_resume_same_container": before["container"], "historical_observation_not_current_stability": True})
        return result

    def pending_pair_recovery(self):
        root, d, e, a, t = self.inputs("pending-pair-failed", "candidate", "current", "recovery")
        r.require(not (root / "checkpoint.json").exists(), "pending-pair failure already exercised; preserve evidence")
        class CrashPair(r.Runner):
            def commit_effect(self, pending):
                if pending["kind"] == "replace":
                    raise r.Unknown("LOCAL interruption AFTER native-success/exact-postimage pair, BEFORE clearing intent")
                super().commit_effect(pending)
        try:
            CrashPair(d, e, a, t, root).start()
            raise AssertionError("pending-pair interruption not injected")
        except r.Unknown:
            cp = r.read_json(root / "checkpoint.json")
        pair = cp["pending"]
        r.require(pair["native_outcome"] == "delivered-success" and pair["native_process"]["terminal"] is True and
                  pair["native_process"]["exit_code"] == 0, "native-success pair missing")
        original = r.file_digest(root / "checkpoint.json")
        host = r.Host(d, t, self.directory("pending-pair-read"))
        committed = host.snapshot()
        r.require(committed == pair["result"] and host.inspect(committed["container"])["Config"]["Labels"].get("jobcron.card") == CARD,
                  "pending-pair owned identity unproven")
        self.sql("UPDATE profiles SET profile_json='{\"min_score\":19}', profile_hash='after-pending-pair-failure' WHERE user_id=1;")
        self.docker("stop", "--time", "10", committed["container"])
        stopped = host.snapshot(allow_stopped=True)
        r.require(stopped["container"] == committed["container"] and not stopped["running"], "pending-pair stop unproven")
        try:
            r.Runner(d, e, a, t, root).start(resume=True)
            raise AssertionError("failed pending pair accepted")
        except r.Refuse:
            r.require(r.file_digest(root / "checkpoint.json") == original and host.snapshot(allow_stopped=True) == stopped,
                      "failed pending-pair refusal rewrote evidence or replayed")
        rr, rd, re, ra, rt = self.inputs("pending-pair-compatible-recovery", "current", "candidate", "candidate")
        ra["recovery_preimage"] = stopped
        r.atomic(rr / "approvals.json", ra)
        args = ["/usr/bin/python3", str(REPO / "scripts/app_release.py"), "recover", "--failed-state-dir", str(root), "--state-dir", str(rr)]
        for name in ("descriptor", "evidence", "approvals", "target"):
            args += ["--" + name, str(rr / (name + ".json"))]
        receipt = json.loads(self.run(args, timeout=180))
        r.require(receipt["recovery"] and receipt["status"] == "local-rehearsal-verified" and
                  r.file_digest(root / "checkpoint.json") == original and r.read_json(rr / "handoff.json")["original_pending"] == pair,
                  "pending-pair approved recovery changed original evidence")
        r.require(self.sql("SELECT profile_json FROM profiles WHERE user_id=1;").decode().strip() == '{"min_score":19}', "pending-pair recovery lost new write")
        accepted = host.snapshot()
        recovered = r.Runner(rd, re, ra, rt, rr)
        recovered.start(recovery=True, resume=True)
        r.require(host.snapshot() == accepted, "completed recovery replayed replacement")
        r.atomic(rr / "pending-pair-preservation.json", {"durable_native_success_postimage": committed, "later_same_container_stopped": stopped,
                 "original_pending_and_checkpoint_unchanged": True, "ordinary_resume_refused_without_replay": True,
                 "separate_named_recovery_same_lock": True, "new_write_proxy_db_mounts_ledger_preserved": True, "one_confined_scheduler": True,
                 "completed_resume_same_container": accepted["container"], "original_release_not_accepted": True})
        return receipt

    def intermediate_recovery(self):
        root, d, e, a, t = self.inputs("absent-intermediate", "candidate", "current", "recovery")
        class LostReply(r.Host):
            def replace(self):
                super().replace()
                raise r.Unknown("locally lost reply before first postidentity")
        try:
            r.Runner(d, e, a, t, root, host_class=LostReply).start()
            raise AssertionError("intermediate injection did not stop")
        except r.Unknown:
            pass
        host = r.Host(d, t, self.directory("intermediate-read"))
        committed = host.snapshot()
        app = host.inspect(committed["container"])
        r.require(app["Config"]["Labels"].get("jobcron.card") == CARD, "intermediate app not owned")
        self.docker("stop", "--time", "10", committed["container"])
        self.docker("rm", committed["container"])
        absent = host.snapshot(allow_stopped=True, allow_absent=True)
        r.require(absent["container"] is None, "owned absence unproven")
        try:
            r.Runner(d, e, a, t, root).start(resume=True)
            raise AssertionError("absent unbound runtime automatically recovered")
        except r.Refuse:
            pass
        approval = self.attended_inputs(root, d, e, t, action="recover-effective-state")
        result = r.Runner(d, e, a, t, root).start(resume=True, disposition=approval)
        r.require(result["status"] == "attended-recovery-ready" and not (root / "receipt.json").exists(), "unknown original falsely accepted")
        old_hash = r.file_digest(root / "checkpoint.json")
        rr, rd, re, ra, rt = self.inputs("intermediate-compatible-recovery", "current", "candidate", "candidate")
        ra["recovery_preimage"] = absent
        r.atomic(rr / "approvals.json", ra)
        receipt = r.Runner(rd, re, ra, rt, rr).start(recovery=True, failed_state=root)
        r.require(r.file_digest(root / "checkpoint.json") == old_hash, "unknown intermediate checkpoint rewritten")
        r.require(self.sql("SELECT profile_json FROM profiles WHERE user_id=1;").decode().strip() == '{"min_score":19}', "intermediate recovery lost new write")
        r.atomic(rr / "intermediate-preservation.json", {"verified_absence": True, "trusted_local_attended_resolution": True,
                 "unknown_original_not_accepted": True, "new_writes_and_proxy_db_mounts_ledger_preserved": True})
        return receipt

    def old_stopped_recovery(self):
        root, d, e, a, t = self.inputs("old-app-stopped", "candidate", "current", "recovery")
        class StopOldFault(r.Host):
            def replace(self):
                old = self.service("app")
                self.mutate(lambda: self.docker("stop", "--time", "10", old["Id"]))
                raise r.Unknown("LOCAL fault: original app stopped before Compose completion; result not bound")
        try:
            r.Runner(d, e, a, t, root, host_class=StopOldFault).start()
            raise AssertionError("old-app fault did not stop")
        except r.Unknown:
            pass
        host = r.Host(d, t, self.directory("old-stopped-read"))
        stopped = host.snapshot(allow_stopped=True)
        r.require(stopped["image"] == d["current"]["image_id"] and not stopped["running"], "old stopped runtime unproven")
        approval = self.attended_inputs(root, d, e, t, action="recover-effective-state")
        result = r.Runner(d, e, a, t, root).start(resume=True, disposition=approval)
        r.require(result["status"] == "attended-recovery-ready" and not (root / "receipt.json").exists(), "old-app original falsely accepted")
        old_hash = r.file_digest(root / "checkpoint.json")
        rr, rd, re, ra, rt = self.inputs("old-stopped-compatible-recovery", "candidate", "current", "candidate")
        ra["recovery_preimage"] = stopped
        r.atomic(rr / "approvals.json", ra)
        receipt = r.Runner(rd, re, ra, rt, rr).start(recovery=True, failed_state=root)
        r.require(r.file_digest(root / "checkpoint.json") == old_hash, "old-app unknown history changed")
        r.require(host.snapshot()["image"] == stopped["image"] and host.snapshot()["container"] != stopped["container"], "approved inactive same-image recovery not delivered")
        r.require(self.sql("SELECT profile_json FROM profiles WHERE user_id=1;").decode().strip() == '{"min_score":19}', "old-app recovery lost new write")
        r.atomic(rr / "old-stopped-preservation.json", {"local_stop_before_completion_fault": True, "same_image_inactive_recovery_separately_approved": True,
                 "unknown_original_not_accepted": True, "new_writes_and_proxy_db_mounts_ledger_preserved": True})
        return receipt

    def unknown_refusal(self):
        root, d, e, a, t = self.inputs("unknown-refusal", "candidate", "current", "recovery")
        # Create submitted intent against an unchanged real app. Its readback is
        # preimage, so the runner must not replay or declare no submission.
        host = r.Host(d, t, self.directory("unknown-read"))
        before = host.snapshot()
        selector = host.selector()
        checkpoint = {"schema": 1, "contract": "attended-v2", "sequence": 1, "approvals_sha256": r.digest(a),
                      "wanted": d["candidate"], "selector_after": selector,
                      "target": PROJECT, "descriptor_sha256": r.digest(d), "evidence_sha256": r.digest(e),
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
