# AWS production deploy: jobcron.app

> **Alpha launch:** The active end-to-end runbook is the concise
> [human-assisted alpha deployment specification](../../docs/specs/260923-human-assisted-alpha-deployment.md).
> This directory remains the technical runtime reference; its earlier Slice 4
> sequencing is not independent authorization to execute every step.

## Deployment modes

This directory now implements the replacement-host runtime, but running it is a
separate human-authorized operation. For the first deployment use the
[initial-deployment lean lane](HUMAN_DEPLOY_GUIDE.md#initial-deployment-lean-lane):
fresh `human-assisted-initial-deployment-v1` reconciliation, no prior service or
data, no invented legacy host, and explicit replacement of the disposable
managed bootstrap. The checker permits only that action and, if reconciled
absent, creation of the unattached EIP; all other protected resources are no-ops.
Replacement and combined-recovery modes retain their existing legacy-host
requirements. The three-argument historical create mode still uses the exact
Slice 3 checkpoint. Every mode must publish the approved private `linux/arm64`
image through the repository workflow, and record its immutable digest in private
evidence.
Terraform may apply only the independently reviewed saved plan accepted by
`scripts/check-terraform-slice-4-plan.sh`.

The private foundation provides private database subnets, a
VPC-local-only database route table, security-group-only PostgreSQL access, an
encrypted private RDS instance, a runtime-secret container, and a
protected recovery bucket. Only historical create mode requires an empty
container; current reconciliation records its observed numeric version count.
Secret versions are always written outside Terraform. Verified off-cloud
recovery objects expire after 14 days, all current versions expire after 90
days, and the resulting noncurrent data version expires one day later.

The replacement host has no key pair or inbound rule. All host and database
access uses AWS Systems Manager Session Manager. RDS stays private, and the app
uses a lower-privilege database role created through a localhost-only tunnel.
The reserved EIP remains unattached, and the operator stops before any
Cloudflare, DNS, or public-traffic action.

## Runtime contract

Production secrets do not live in the repository, Terraform state, or a host
environment file. `jobcron.service` retrieves the approved Secrets Manager
version into `/run/jobcron`, validates the exact key set, pulls only the
approved digest, and starts Compose through systemd. The one-shot
`read:packages` registry token is separate from the runtime secret and is
deleted with its temporary Docker configuration after the pull.

The runtime is fail-closed: incomplete secrets, unexpected values, bad file
modes, or a failed pull leave both containers stopped and no partial runtime
files. On reboot, the memory-backed `/run/jobcron` directory is empty and
systemd recreates it from the approved secret. `verify-local-state` reports
value-blind booleans and counts for file modes, log rotation, digest retention,
Docker credentials, and free disk space.

Secrets are file inputs, never Compose environment values. The helper first
requires tmpfs custody and disabled swap, then creates `0700` directories and
`0600` files. `compose.env` contains only the image and sponsor selector.
The app receives fixed `*_FILE=/run/jobcron/secrets/NAME` references through a
read-only bind mount. Caddy imports a restricted `proxy-header` snippet directly
from `/run/jobcron/caddy`; no startup environment expansion is needed. Its admin
API and config persistence are disabled, its root filesystem is read-only, and
`/config`, `/data`, and `/tmp` are tmpfs (there are no persistent Caddy volumes).
Do not enable swap, core dumps, Caddy debug/config logging, or secret-valued
container environment overrides. Host root and the Docker daemon remain trusted.

App file inputs cover `DATABASE_URL`, `SESSION_SECRET`,
`JOBCRON_CREDENTIAL_ENCRYPTION_KEY`, `JOBCRON_SIGNUP_ACCESS_CODE`,
`JOBCRON_PROXY_SECRET`, `JOBCRON_ADMIN_TOKEN`, and `JOBCRON_WORKNET_KEY`.
Direct values are rejected in production; non-production retains the existing
environment contract. A value and its `_FILE` key together are ambiguous even
when empty. Files must be absolute, non-symlink, single-link regular files owned
by the effective user, mode `0400` or `0600`, non-empty and at most 64 KiB.
One terminal LF is removed; CR, NUL and embedded LF are rejected. Other bytes,
including spaces, are preserved. Windows rejects file inputs because these Unix
custody guarantees cannot be established. Errors never include paths or values.

`jobcron-user` accepts `DATABASE_URL_FILE` for all database commands and
`JOBCRON_DATABASE_PASSWORD_FILE`, `JOBCRON_OWNER_PASSWORD_FILE`, and
`JOBCRON_USER_PASSWORD_FILE` for their respective password prompts. Production
operator URLs are password-free: use the real RDS hostname, the explicit local
SSM tunnel port, and exactly `sslmode=verify-full`, `hostaddr=127.0.0.1` and
`sslrootcert=<absolute approved CA file>`. The CA and its directories must be
owned by root/the operator, non-symlink and not writable by other users (a
root-owned sticky temporary ancestor is allowed). The existing pgx stdlib
registration retains RDS `ServerName`/RootCAs but permits only the exact
loopback TCP dial; no remote DNS, alternate dial or plaintext fallback.
All production operator subcommands use a separate database password file or
silent prompt, and owner/reset passwords retain their own inputs. Unset ambient
`PG*` settings. Errors never disclose the URL, CA path or password. Legacy
loopback `sslmode=require` migration remains non-production-only and is not
certificate/hostname verification. Do not put production URLs in argv.

## Verified RDS TLS and controller command brief

This is a local compatibility contract, not authorization or deployment PASS.
The controller retains all exact-operation, recovery and cutover gates.

- Certificate: independently approve the official regional RDS CA bundle and
  its digest. Stage public PEM bytes as root-owned `/etc/jobcron/rds-ca.pem`
  (`0600`, directory `0700`, no symlinks). Existing native `openssl` must be
  available for PEM validation. This public trust anchor is not a new secret,
  runtime JSON key, credential rotation or IAM permission.
- Tunnel/operator: the native SSM TCP session must forward only the selected
  RDS endpoint to an explicitly allocated `127.0.0.1` port. Put the password-free
  real-host/loopback-hostaddr URI above in an owner-only file; invoke the locally
  built exact reviewed `jobcron-user` binary with production `_FILE` inputs.
  Keep this operator SHA/binary digest distinct from the accepted app release.
  Verify certificate and hostname failures stop the operation; reconcile and
  terminate the exact native session handle afterwards.
- Role helper: `JOBCRON_MASTER_DATABASE_URL` retains the existing password-free
  loopback coordinate syntax (`sslmode=require` or `verify-full`); it is never
  used as the effective connection. Set `JOBCRON_RDS_CA_FILE` to the approved
  owner-only absolute CA file and `JOBCRON_PRIVATE_DATABASE_ENDPOINT` to the
  selected real RDS host/port. The helper constructs a `verify-full` libpq URI
  with real host, loopback `hostaddr`, tunnel port and that CA. Passwords remain
  two stdin lines, never argv. The runtime URL it writes uses the real private
  endpoint and exactly `sslmode=verify-full&sslrootcert=/run/jobcron/rds-ca.pem`.
- Runtime: preserve the exact nine existing JSON keys and existing secrets/key;
  bind the accepted image and actual owner ID privately. `prepare` rejects
  downgraded/wrong-CA URLs, copies the approved public CA into root-owned tmpfs
  `/run/jobcron/rds-ca.pem`, and Compose binds that file read-only at the same
  path with `create_host_path: false`. Cleanup removes only the runtime copy.
  Reprepare must not nest a secrets directory or destroy unexpected evidence.
- Backup: prefer the controller's already available native PostgreSQL 18 client
  over installing host tools for the initial off-host dump/checksum/isolated
  restore. Use libpq's real host plus loopback `hostaddr` and the approved CA
  with `verify-full`, password-free `--dbname` and private child password input.
  Actual client compatibility, encrypted off-host retention, separate encryption
  key backup and disposable restore/schema-count comparison remain live gates.
  Host `archive` preserves the verified URI and keeps its password off argv;
  `JOBCRON_PG_DUMP` may select one already installed absolute executable path
  ending in `/pg_dump` (no command string), otherwise PATH's `pg_dump` is used.
  It does not install tools or assert a client-version PASS.
- Recovery timer: before enabling, the controller must bind the existing
  non-secret bucket with a root-owned systemd drop-in for
  `jobcron-recovery.service`: `Environment=JOBCRON_RECOVERY_BUCKET=<existing
  approved bucket>`, and, only if needed, `Environment=JOBCRON_PG_DUMP=<installed
  compatible absolute path>`. Do not put secrets in this drop-in or add IAM.
  Reload/read back the binding and require a successful real archive before
  enabling the timer. A host pg_dump 15 against RDS 18 cannot satisfy that gate;
  leave the timer disabled until compatible, even if the controller backup PASSes.

The published application image is unchanged: no `cmd/jobcron`, application
packages, Go modules, migrations or production Dockerfile behavior changed.
Its existing pgx runtime already accepts `verify-full`/`sslrootcert`; only the
external trust file/mount was missing. Reuse the accepted immutable arm64 image
and its existing CI/provenance evidence, not a new operational-only publication.

## Files

- `compose.yaml` consumes `/run/jobcron/compose.env`, pulls the approved
  immutable image and reaches private RDS through `DATABASE_URL_FILE`.
- `Caddyfile` uses transient Origin CA material from `/run/jobcron/caddy`,
  redirects `www.jobcron.app`, and keeps app access private.
- `.env.example` contains synthetic local-render inputs only.
- `Dockerfile` builds the release image used by the private publication
  workflow.
- `jobcron-runtime.sh` implements `prepare`, `pull`, `archive`, cleanup,
  `verify-local-state`, and value-blind `verify-secrets` metadata checks.
- `systemd/` contains the fail-closed app service and nightly recovery units.
- `HUMAN_DEPLOY_GUIDE.md` defines the exact-plan, Session Manager, private
  verification, recovery manifest, reboot, and stop-condition sequence.

## Validate Compose locally

Use synthetic values only:

```sh
cd deploy/production
JOBCRON_IMAGE='ghcr.io/example/jobcron@sha256:0000000000000000000000000000000000000000000000000000000000000000' \
JOBCRON_STAGE1_SPONSOR_USER_ID='1' \
docker compose config
```

The rendered config must keep the app on loopback host port `7777` and publish
only Caddy on host TCP `443`. During private deployment the origin security
group has no ingress and the reserved EIP remains unattached; at cutover its
only ingress is Cloudflare-prefix-list TCP `443`. The config must include the
database, session, credential-key, production, no-open, scheduler, and signup
settings as file references; and contain no legacy credential volume.
It must not include demo mode, an admin token, a Worknet key, or a
caller-supplied trusted-proxy header. Caddy and the app receive the same proxy
secret so only Caddy can supply the client address used by authentication rate
limits.

## Private operations and recovery

Private verification uses Session Manager port forwarding to the app on host
port `7777` and Caddy on host port `443`. The operator checks real user
behavior, the Origin CA certificate, lower-privilege RDS access, reboot
recovery, and the absence of public ingress before recording sanitized
evidence.

Slice 5 discovers the origin security group through the fixed
`jobcron:edge-target = origin-security-group` tag and derives the canonical VPC
from that group's `vpc_id`. The canonical VPC intentionally remains untagged.

`jobcron-recovery.service` creates a custom-format database dump, sanitized
container logs, and SHA-256 recovery manifests. The trusted Mac runs
`scripts/pull-production-recovery.sh` to copy missing objects, verify every
manifest, and apply only the `macbook-copy=verified` tag. Restore verification
uses a disposable database and bounded schema and row-count comparisons. Initial
deployment may instead verify one off-host database dump and checksum with a
disposable restore before public writes; the exhaustive six-object/log-retention
matrix is deferred, not the restore gate. Empty-unused-DB snapshots and extra
adversarial toolchain manifests are deferred in that lane; prior-image lineage
and production-data import are N/A. Existing automated safety checks remain.

Stop conditions include any checkpoint mismatch, unapproved saved-plan action,
private value in shared output, public ingress, incomplete runtime secret,
failed private user path, failed recovery verification, or less than 2 GiB free
after normal pruning. Keep the database, reserved EIP, current image and recovery
materials throughout the rollback window; preserve an old host or prior image
only where one exists. Initial deployment failure means stopping privately and
leaving public routing unchanged. Separate exact-plan apply approval and attended
cutover approval remain mandatory.
