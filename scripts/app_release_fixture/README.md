# App-only release runner: invocation and custody

`../app_release.py` is a dependency-free Python 3 POSIX target-host CLI. It runs
fixed native Docker/Compose, PostgreSQL-client and (production only) AWS CLI
operations. Run it inside the existing attended Session Manager session on the
selected host; it is not a remote command broker. It never publishes an image,
starts a tunnel, acquires credentials, migrates, restores the authoritative DB,
changes routing, runs Terraform, or invokes systemd/Compose shutdown.

Status: independently locally accepted/integrated at
`095bc775610656f3cbe74a076c5be495c30d22b5` (worker 166 / reviewer 167).
See [acceptance and evidence limits](../../docs/decisions/261012-app-only-runner-local-acceptance.md).
The owned rehearsal exercised real Linux containers/Compose/PostgreSQL and synthetic
HTTP authentication; native app artifacts were source `689129b`, not `095bc775`.
Production AWS/IAM, EC2 host identity, off-host storage, public routing, production
no-swap positive and the real 1800-second stability have NOT been exercised.
Neither this command nor its local fixtures grants production authority.

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
operator custody are trusted. Every private input is an absolute owner-owned `0600`
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
workspace must be on verified owner-only tmpfs with swap disabled. The existing
disk-backed public Caddyfile may be `0600` or `0644`; it remains owner-owned,
single-link, non-symlink and immutable by inode/content/mount identity. It is not
relocated onto tmpfs. Payload/password temporaries are removed on
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
  `compose_executable`, exact independently observed `compose_version`,
  `compose_file`, `compose_sha256`, `compose_env`,
  `selector_file`, stable per-target `lock_dir`, `config_sha256`.
- `protected_sha256`: hash of complete protected actual runtime metadata (proxy
  ID/image/config/host config/mounts; app host config/mounts, non-image env and
  command; protected path inode/mode/content identities; local DB equivalent).
  Use `Host.snapshot`'s metadata shape. Inspect mounts are sorted by unique
  destination and Engine API `HostConfig.Mounts` by unique target,
  and app env by unique name, since Docker's array ordering is unspecified;
  no fields/values are dropped or rewritten. All other differences fail closed.
- `protected_paths`: secret directories, CA/Caddy configuration and other
  protected mount inputs; include every relevant non-image runtime input and all
  app/Caddy bind sources. `protected_public_paths` explicitly classifies only the
  existing Compose-relative `Caddyfile` as public. Secret-file classification,
  public directories, inline proxy secrets/private keys, missing bind-input
  coverage and public config mounted as a secret refuse. Actual secret mounts and
  scratch still require private tmpfs/no-swap in production. Both classes retain
  owner/mode/inode/content checks.
- `protected_metadata_file`: original owner-only `Host.snapshot` protected
  metadata, whose digest must equal `protected_sha256`. Required for an explicitly
  identified absent-app recovery; current proxy/DB/files/ledger are independently
  reread and must still match it. It never authorizes changed protected values.
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
Use the same unchanged packet/state with `resume`. Before EVERY write, including
backup/selector committed-before-stage-advance windows, the entire recorded app
CID/image/start/restarts/running preimage, selector/version, protected state and
ledger must match. Runtime state is never refreshed from a convenient observation.
Every durable native success/exact-readback pair is saved before clearing intent;
automatic replacement resume requires that complete pair AND unchanged exact
identity. Compose returns a completed command, NOT an Engine create-ID receipt.
Matching image/name, a local operation UUID or zero restarts never prove a lost
command created that container. A crash between native success and durable first
postidentity remains unknown, even if the app currently looks healthy.
Conflicting/preimage/unprovable observations stop. Submission is accounted at the
actual mutating boundary, not around an entire multi-command callback: a missing
late readback helper leaves earlier submission unknown. Only proven no-submission
permits one deliberate resume retry, never an ambiguous native effect. Backup
artifact creation is itself a mutation; a later missing dump/upload helper cannot
make that partial artifact unsubmitted. Ambiguous recovery-point creation needs attended archive
reconciliation and is deliberately not automatically replayed.

### Explicit attended replacement disposition

For unbound replacement intent only, a trusted operator may supply a separate
owner-only input through the existing attended custody channel:

```sh
python3 scripts/app_release.py resume "$@" \
  --disposition-file "$PACKET/attended-disposition.json"
```

The ORIGINAL packet/state/target lock remains selected by `$@`. This is not a
permission to replay the ambiguous command or automatically recover. The runner
never generates production dispositions. The operator must independently prove
the client invocation terminated, all native/daemon effects settled, and exclusive
effective-host state before supplying any disposition. A possibly in-flight
mutation or transient snapshot is insufficient; if settlement is unprovable, stop.
The runner also refuses a recorded still-existing native PID, then rechecks state
under the same lock. Operator/installed-tool custody is trusted, not a new
signature, daemon transaction or hostile-administrator defense.

Disposition schema 1 binds `target`, `descriptor_sha256`, `evidence_sha256`,
`target_sha256`, ORIGINAL `checkpoint_sha256` and `sequence`, pending
`operation_id`, complete intended `artifact`, independently observed complete
`observed` runtime tuple (or verified absence), and exact `selector`/native version.
It names `approval_id`, `disposition` and matching `authority`, plus independent
owner-only `evidence_file` and its `evidence_sha256_file`. That evidence repeats all
bindings, schema 1, a nonempty `record` reference and explicit true
`native_invocation_terminated`, `native_effects_settled`, `exclusive_operator`.
Missing/stale/conflicting bindings refuse. Artifact/manifests and protected-state
readback keep host/DB/target/schema/routing drift outside this lane, regardless of
a runtime-identity disposition.

`accept-effective-state` proceeds to ordinary journey/stability verification; it
does NOT claim causal provenance or waive acceptance. `recover-effective-state`
records the exact failed/stopped/created-unstarted/absent state and returns
`attended-recovery-ready`, without a release receipt or success. The unknown
checkpoint is retained byte-for-byte as `unknown-checkpoint-<digest>.json` and a
separate `disposition-<digest>.json` appends named evidence with
`original_native_result: unknown` and `causal_container_provenance_claimed: false`.
The active checkpoint uses the same truthful `attended-effective-state` label.
No prior history or unknown marker is manually cleared.

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
historical-only. A durable pending replacement success/postimage pair can also
hand off directly to this separately approved recovery if its SAME recorded
container subsequently stops/fails/restarts. The delivered pair, pending intent
and failed checkpoint are retained verbatim; `handoff.json` additionally records
`original_pending`, the exact new observed failure state and named recovery scope.
A native invocation must have terminal success and no still-existing recorded
PID; unbound/still-in-flight intent cannot use this route. Failed state is never
promoted to release acceptance and the original replacement is not replayed.
A stopped/crashed but uniquely identifiable candidate is allowed
only for recovery/reconciliation, never successful acceptance. Pending replacement
without a durable native-result/postimage pair must first use the ORIGINAL
`resume --disposition-file` and an independently settled-effective-state decision.
An explicit `recover-effective-state` disposition can identify an absent/partial
app without accepting the original release. The NEW recovery approval additionally
binds `recovery_preimage` to that exact current effective state, including CID,
image/start/restarts/running (or absence) and protected digest. Recovery rechecks
that tuple immediately before its writes and retains authoritative DB/new writes.
Changed target/DB/schema/protected configuration cannot be approved here.
Unresolved backup or
selector intent, foreign/unidentifiable containers, concurrent owners and global
STOP remain fail-closed. Do not manually clear a mutation marker.
No-op image releases are refused before effects. The only exception is NEW
separately approved same-lock recovery of an explicitly recorded inactive
intermediate app; it may recreate the already compatible image after the old app
stopped before replacement completion. Both actual image and persisted selector
are independently bound, even when that interrupted intermediate differs from
the intended selector. Ordinary running-image no-ops still refuse.
A completed resume rechecks
exact accepted container/start/restart/protected identity and native selector
version, repeats the authenticated journey and records a fresh `resume_readback`.
Before returning that receipt it validates a durable canonical `receipt_sha256`
and exact semantic bindings to descriptor/evidence/approvals, source/image/platform,
accepted CID/start, selector/version, effect outcomes, recovery point, journey and
observation policy/samples. Missing/copied/mutated receipts refuse, even if the
native runtime is unchanged; historical success is never regenerated to repair a
missing binding. Before publication, the checkpoint records acceptance facts and
requires at least three unchanged samples with a recorded final sample at or
beyond the minimum observation interval, not only a later loop-condition clock.
It then records
the expected digest at `receipt-ready`. An interrupted publication can finalize
ONLY if the existing receipt matches that binding and current identity. Missing
or conflicting receipt-ready evidence remains refused; separately approved
same-lock failed-app recovery is still available without rewriting it.
It does not recreate the app or relabel the historical stability interval as a
new production observation. Interrupted committed-stage advancement applies the
same selector operation/version and protected-value checks as pending intent.
Checkpoints bind unchanged descriptor/evidence/target AND approvals, monotonic
local sequence and `contract: attended-v2`. Older checkpoints without these
result/expected/wanted/contract bindings refuse; preserve
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
includes durable-backup/result and durable-replacement/result crashes with no
replay, plus a locally discarded reply from a REAL Compose replacement. The
unbound reply first refuses automatic adoption without altering the checkpoint,
then uses explicit synthetic LOCAL attended settlement/authority and still runs
the real journey/stability checks. An injected unknown intent refuses without
effects. It also injects a
read-only prerequisite/acceptance failure, stops only the exact owned candidate,
then invokes real CLI approved compatible recovery with new-write/proxy/DB/mount/
ledger preservation and unchanged completed-resume controls. It additionally
interrupts AFTER a real durable replacement-success/postimage pair but BEFORE
clearing pending intent, stops that exact owned app, refuses ordinary acceptance,
then exercises separately approved native same-lock recovery with the original
pending pair/checkpoint byte-preserved and post-change writes retained.
An owned absent-app
fault exercises explicit local intermediate disposition followed by separately
approved compatible recovery; the unknown original is never relabeled accepted.
Transport/acceptance fault
injection is local, not a provider permission test. Cleanup checks the exact
owned daemon/card/resource labels, removes only owned container/volume/network
resources, and retains images, checkpoints and sanitized evidence for review.

Focused offline checks:
`TMPDIR="$STATE" python3 -m unittest discover -s scripts -p 'app_release*test.py' -v`.
Use an owner-only TMPDIR outside shared system temporary directories. Full app
checks must unset optional PostgreSQL/browser-probe variables. Local elapsed
observations never satisfy the 30-minute production gate.
