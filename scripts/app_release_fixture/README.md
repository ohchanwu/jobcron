# App-only release runner: invocation and custody

`../app_release.py` is a dependency-free Python 3 POSIX target-host CLI. It runs
fixed native Docker/Compose, PostgreSQL-client and (production only) AWS CLI
operations. Run it inside the existing attended Session Manager session on the
selected host; it is not a remote command broker. It never publishes an image,
starts a tunnel, acquires credentials, migrates, restores the authoritative DB,
changes routing, runs Terraform, or invokes systemd/Compose shutdown.

Status: implemented for local acceptance. The owned rehearsal exercises real
Linux containers/Compose/PostgreSQL and synthetic HTTP authentication. Production
AWS/IAM, EC2 host identity, off-host storage and public routing have NOT been
exercised. Independent same-card review remains an acceptance gate. Neither this
command nor its local fixtures grants production authority.

## Native prerequisites and trust boundary

Use a trusted installed Python 3, Docker CLI and absolute standalone Compose
executable (an installed `docker-compose` CLI-plugin binary can be invoked
directly). Production also needs installed `aws`, `psql`, `pg_dump`, `pg_restore`,
`findmnt`, `swapon`, an EC2 Nitro board-asset identity, disabled swap, and an
attended operator-selected temporary static AWS session. No dependency installer
or registry-login path is present. Current, candidate and recovery images must
already be locally available at their exact digests/platform. Pre-pull through
the supported runtime pull procedure ONLY under separate publication/credential
and registry-read authority, before interrupting the app.

Host root, daemon, installed tools, approved Compose contract and attended
operator custody are trusted. Every input is an absolute owner-owned `0600`
single-link regular non-symlink file; state/lock directories are `0700`. Ancestors
must not be symlinks or writable by other users. Do not use a shared `/tmp`.
Do not modify the live Compose topology to get a pass. Its approved file must
have private custody at its existing path, so relative mount sources resolve
exactly as the current runtime. Non-image render/actual-runtime hashes come from
independent current observations, not the proposed descriptor.

The runner clears inherited environment, stdin, AWS/PG/Docker context and proxy
settings, supplies an empty isolated Docker config, an explicit Compose file and
env file, and no shell commands. AWS profiles admit only the selected static
session: region-only config and access key/secret/session-token credentials. Role
chaining, `credential_process`, endpoint overrides, SSO and hidden config routes
are refused. Native stdout is captured in bounded, non-disk pipes (8 MiB maximum),
not a temporary file in the durable checkpoint directory. Production credential
files, journey account, database URL, protected secret paths and native temporary
workspace must be on verified owner-only tmpfs; payload/password temporaries are removed on
ordinary exits. Credentials never appear in argv, stdout, receipts or raw errors.
An OS kill can leave private tmpfs files; inspect and clean only owned remnants.

The approval/evidence files are supplied independently by the attended trusted
operator after checking the referenced source/CI/review/publication/restore
records. They are not signatures, an anti-replay service or automated permission
proof. The runner checks exact bindings; it does not fetch CI or decide that its
own generated plan/receipt is approval. Fixture-generated local records are
explicitly `local-rehearsal` and cannot satisfy production provenance.

## Input schema (JSON, schema 1)

The descriptor has exactly these fields:

- `schema: 1`, `release_id` (short lowercase identifier), `mode` (`local` or
  `production`), and `target` (independently selected logical target).
- `current`, `candidate`, `recovery`: each contains `source` (full 40-character
  SHA), `selector` (production `ghcr.io/<owner>/jobcron@sha256:<64hex>`; local
  image IDs allowed), `image_id` (actual Docker `sha256:<64hex>`), and `platform`
  (`linux/arm64` or `linux/amd64`). A tag or abbreviated SHA is not sufficient.
- `manifests`: both `postgres` and `sqlite`, each with complete Git `tree` and
  ordered `files: [{name, sha256}, ...]`. All three artifact evidence records must
  match these complete manifests. The actual `schema_migrations` rows must match
  every PostgreSQL version/name/digest, with no missing or extra row. Startup's
  own validation remains intact. A mismatch exits the lane; never migrate/backfill
  or delete ledger entries to make the runner pass.
- `config_sha256`: the independently observed rendered Compose JSON hash with
  only `services.app.image` removed, using the same renderer/version and paths.
- `operation_timeout`: native per-operation watchdog, 1–600 seconds;
  `observe_seconds`, `sample_seconds`: finite positive observation policy with
  initial/intermediate/final samples. Production enforces at least 1800 seconds.
  Local shorter intervals are not production-stability evidence.

Independent evidence contains `target`, `mode`, and one record per artifact role:
`artifact`, `manifests`, and `ci`, `review`, `publication` records. Each gate has
`source`, `result: accepted`, `kind` (`ci`, `review`, `publication` respectively in
production; `local-rehearsal` only in local mode), and a nonempty independent
`record` reference. Publication additionally binds `selector` and `platform`.
`restore` binds `accepted: true`, `target`, the exact `manifests`, and the accepted
disposable-restore `record`. Production additionally binds
`secret_nonimage_sha256`, computed privately from the exact current secret JSON
with only `JOBCRON_IMAGE` removed. Do not publish that private hash or its values.
Reuse restore evidence only while its schema/tool/target applicability remains
valid; `pg_restore --list` is not a disposable restore.

Approvals contain `descriptor_sha256` (canonical sorted compact JSON SHA-256),
exact `target`, a non-secret `approval_id`, and named `effects` from `observe`,
`recovery-point`, `persist`, `replace`, `recover`. `recover` is separately required
for recovery; a failed release does not grant it. `observe` includes the approved
native/provider reads and HTTP authentication/session creation/logout used by the
core journey. Publication and credential acquisition stay separately attended.

The owner-only target observation contains:

- `target`, `project`, exact local `docker_host` Unix socket, observed `daemon_id`,
  `compose_executable`, `compose_file`, `compose_sha256`, `compose_env`,
  `selector_file`, stable per-target `lock_dir`, `config_sha256`.
- `protected_sha256`: hash of complete protected actual runtime metadata (proxy
  ID/image/config/host config/mounts; app host config/mounts, non-image env and
  command; protected path inode/mode/content identities; local DB equivalent).
  Use `Host.snapshot`'s metadata shape. Inspect mounts are sorted by unique
  destination and Engine API `HostConfig.Mounts` by unique target,
  and app env by unique name, since Docker's array ordering is unspecified;
  no fields/values are dropped or rewritten. All other differences fail closed.
- `protected_paths`: secret directories, CA/Caddy configuration and other
  protected mount inputs; include every relevant non-image runtime input.
- `journey_url` (exact HTTPS origin), `journey_ca_file`,
  `journey_account_file: {email,password}`, `proxy_secret_file`. Supply an approved
  journey account in private custody, never in shell arguments or shared output.
  Redirects must stay on that exact origin. The fixed journey checks login,
  dashboard, briefing, bookmarks, profile, account, logout and session revocation;
  it never calls scrape/re-rate. Fixture journeys use synthetic accounts only.
- Production additionally: `account`, `actor_arn`, `region`, `instance_id`, independently
  obtained `instance_identity_file`, `aws_config`, `aws_credentials`,
  `aws_profile`, `secret_id`, `secret_work_dir` (private tmpfs),
  `database_url_file` (current runtime URL), `database_ca_file`, `recovery_bucket`.
  The runtime URL must use `postgres`, password file custody and only
  `sslmode=verify-full&sslrootcert=...`; libpq receives a private passfile and exact
  host/database/user/CA settings, not a password in argv/environment.

These private observation/evidence packets are reviewed inputs, not public
fixtures. Do not manufacture production packets from the local harness.

## Commands and recovery

From the reviewed checkout, select existing owner-only packet/state directories:

```sh
umask 077
set -- --descriptor "$PACKET/descriptor.json" \
  --evidence "$PACKET/evidence.json" --approvals "$PACKET/approvals.json" \
  --target "$PACKET/target.json" --state-dir "$STATE"
python3 scripts/app_release.py "$@"            # default input-only prepare
python3 scripts/app_release.py preflight "$@"  # named observe authority required
python3 scripts/app_release.py release "$@"    # all forward effect approvals
```

Prepare validates only local inputs and submits nothing. Preflight locks the
selected target, verifies native availability/identity/protected state/ledger and
current selector; it does not mutate the selector/app or create a backup. Release
reobserves under the same stable target lock, creates an owned recovery dump
(and in production uploads/downloads it and verifies its digest), changes only
`JOBCRON_IMAGE` in the authoritative secret plus runtime selector file, and runs
exactly `Compose up --detach --no-deps --no-build --pull never --force-recreate
--scale app=1 app`. It never calls runtime `prepare`/`cleanup`, which would remove
mounted directories. Caddy/database/mount/config identities are rechecked.
Compose singleton stop-before-create permits a short interruption, not overlap.
The receipt's measured replacement-to-authenticated-readiness time is a
conservative interruption upper bound, not a zero-downtime claim.

`checkpoint.json`, `receipt.json`, lock inode and dumps remain owner-only.
Kernel locking plus a durable marker rejects concurrent/stale runs. Do not
resume a different release state to clear a stale marker: it is bound to the
exact state-directory/descriptor pair, including for completed checkpoints.
Pre-binding legacy markers refuse and require explicit owner reconciliation.
Do not unlink the kernel-lock inode, expire a marker on a TTL, or overwrite a checkpoint.
Read-only failures release their marker with an owner-only history record. A
crashed read-only marker can be reobserved by the same packet/state's `preflight`;
that route cannot clear mutation intent or another state's marker. History records
retain every transferred marker and completed/failed-before-effects disposition.
A submitted operation with a lost reply is unknown even if the process failed.
Use the same unchanged packet/state with `resume`; authoritative committed
selector versions/container identities are reconciled without resubmission.
Conflicting/preimage/unprovable observations stop. Submission is accounted at the
actual mutating boundary, not around an entire multi-command callback: a missing
late readback helper leaves earlier submission unknown. Only proven no-submission
permits one deliberate resume retry, never an ambiguous native effect. Backup
artifact creation is itself a mutation; a later missing dump/upload helper cannot
make that partial artifact unsubmitted. Ambiguous recovery-point creation needs attended archive
reconciliation and is deliberately not automatically replayed.

SIGINT/SIGTERM writes the stable target-lock `STOP` marker. An owner-written marker
there stops every release-state directory for that target; do not remove it just
to continue one slice. Explicit owner resumption/reconciliation is required.
Native watchdog timeouts stop the process group and preserve unknown intent;
readiness/observation failures retain the app/DB and never automatically recover.

Recovery uses a NEW descriptor whose current identity is the actual failed/new
image, independently compatible recovery provenance and separately named
`recover` approval, plus a NEW state directory. Invoke `recover`, or `resume
--resume-recovery` for its interrupted checkpoint. It preserves the same
writable DB and all post-change writes. Never restore a pre-change writable DB.
When a prior failed acceptance still owns the target marker, invoke:

```sh
python3 scripts/app_release.py recover "$@" --failed-state-dir "$FAILED_STATE"
```

Here `$@` must select the NEW recovery packet/state, not the failed packet. The
handoff holds the same target kernel lock, reads the preserved failed checkpoint,
binds its wanted artifact to the new current artifact, checks exact native
selector version/protected values, container identity, protected runtime and
ledger, then transfers ownership with a separate `handoff.json`. The failed
checkpoint and receipt remain unchanged; `superseded.json` makes their resume
historical-only. A stopped/crashed but uniquely identifiable candidate is allowed
only for recovery/reconciliation, never successful acceptance. Pending replacement
can first be reconciled via the ORIGINAL `resume`, even if stopped; verification
still refuses, after which approved handoff is possible. Unresolved backup or
selector intent, foreign/unidentifiable containers, concurrent owners and global
STOP remain fail-closed. Do not manually clear a mutation marker.
No-op image releases are refused before effects. A completed resume rechecks
exact accepted container/start/restart/protected identity and native selector
version, repeats the authenticated journey and records a fresh `resume_readback`.
It does not recreate the app or relabel the historical stability interval as a
new production observation. Interrupted committed-stage advancement applies the
same selector operation/version and protected-value checks as pending intent.
Older checkpoints without these result/expected/wanted bindings refuse; preserve
them for attended reconciliation rather than editing their history to pass.

## Explicit owned local harness

`../app_release_rehearsal.py` is a card-allocated macOS worker fixture, not a
production packet/approval generator. It refuses other worktrees/profiles/boards,
nonempty project preimages and collisions. It uses project
`jobcron-runner-20261010-local1`, ports `127.0.0.1:17777` and `:17780`, an internal
app/DB network and ingress ONLY on Caddy (never the DB). Thus the live scheduler
exists but cannot reach scraper/AI providers. Do not press scrape/re-rate.

It independently builds current/candidate CGO-free binaries from the protected
unchanged application source, with distinct version parameters and scratch
images; recovery reuses the known-compatible current build. This tooling-only
slice changes no application source. Actual container/image/platform readback
binds both builds, not three aliases of one candidate image. Local gate records
are explicitly fixture authority, not green hosted CI or a production review.

Under the allocated worker environment, create a fresh `0700` directory directly
under that profile's scratch home, then run `setup --root "$FIXTURE"`,
`sequence --root "$FIXTURE"`, and after review/use `cleanup --root "$FIXTURE"`
with `python3 scripts/app_release_rehearsal.py`. Missing cached images/toolchain
are a capability refusal, never implicit pulls or login. Preserve failed
bootstrap inputs; `bootstrap` is only an explicit empty-DB continuation and
refuses previously submitted bootstrap/account/migration work. The sequence
includes a locally discarded reply from a REAL Compose replacement and an
injected unknown intent that must refuse without effects. It also injects a
read-only prerequisite/acceptance failure, stops only the exact owned candidate,
then invokes real CLI approved compatible recovery with new-write/proxy/DB/mount/
ledger preservation and unchanged completed-resume controls. Transport/acceptance fault
injection is local, not a provider permission test. Cleanup checks the exact
owned daemon/card/resource labels, removes only owned container/volume/network
resources, and retains images, checkpoints and sanitized evidence for review.

Focused offline checks:
`TMPDIR="$STATE" python3 -m unittest discover -s scripts -p 'app_release*test.py' -v`.
Use an owner-only TMPDIR outside shared system temporary directories. Full app
checks must unset optional PostgreSQL/browser-probe variables. Local elapsed
observations never satisfy the 30-minute production gate.
