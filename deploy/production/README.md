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
`JOBCRON_USER_PASSWORD_FILE` for their respective password prompts. The migration
URL must still be password-free, localhost-only and TLS-required; its password
comes from the separate file or silent prompt. Existing direct inputs remain for
controlled non-production tooling, not the production operator procedure.
Do not supply credential-bearing URLs in command arguments.

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
