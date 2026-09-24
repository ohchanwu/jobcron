# Human-Assisted Alpha Deployment

**Status:** Active

This specification replaces the autonomous, multi-slice first-production launch
process for the initial Jobcron alpha. The archived process remains useful
historical evidence, but it is not the runbook for this launch.

This document defines safety requirements and approval boundaries. It does not
by itself authorize an AWS, DNS, Cloudflare, registry, database, or public-
traffic change.

## Objective

Deploy Jobcron in one attended working day for an invite-only alpha of fewer
than 200 users. The human operator remains present to inspect the AWS and
Cloudflare consoles, complete authentication, confirm sensitive facts, and
approve external changes.

The goal is a safe small release, not a generalized deployment platform.

## Target architecture

Use the existing supported AWS design selected after live discovery:

- one application host;
- one private PostgreSQL RDS database;
- Caddy as the only public HTTPS entry point;
- one Jobcron application container and one active scheduler;
- access-code-gated signup; and
- an immutable private image pinned by digest.

Do not invent a PaaS deployment or production database on the application host
for this deadline. Keep the application port, PostgreSQL, SSH, and
administrative interfaces off the public Internet.

## Responsibilities

The human operator:

- completes AWS or registry authentication;
- inspects sensitive console values without copying them into tracked output;
- confirms the account, region, resources, costs, DNS, and rollback target;
- supplies or enters secrets through approved private channels; and
- explicitly approves each mutation boundary below.

The assisting agent:

- checks the repository, exact release commit, tests, plans, and value-blind
  evidence;
- guides the ordered runbook and stops on contradictions;
- never infers a private selector or secret; and
- does not push, deploy, change credentials, or enable traffic without the
  corresponding human approval.

## Reconcile current state first

The launch starts with discovery, not an apply:

1. Authenticate and confirm the intended account and region.
2. Identify the stopped EC2 and RDS resources and start only the selected pair.
3. Confirm database encryption, backups, networking, and data integrity.
4. Determine whether the deleted EIP was the Terraform-managed origin EIP.
5. Treat every previous saved plan as stale. Refresh state and generate a fresh
   plan from recovered private controller inputs.
6. Confirm current DNS ownership and whether any record points to a stale
   address.

Do not edit or execute private recovery artifacts in place. Stage only the
required inputs in an owner-only working directory, and never place their
values in Git, chat, logs, screenshots, or Terraform output.

## Release gates

All gates are mandatory:

- The release is one exact, clean commit.
- CI is green through the PostgreSQL-backed tests, full test suite, race suite,
  and target build; an early failure that skips later jobs is not green CI.
- The private image is bound to the release commit and consumed by immutable
  digest.
- Production secrets are absent from Git, image layers, Terraform state,
  command arguments, shared output, and persistent host files.
- The application uses a lower-privilege database role over TLS.
- A pre-change RDS snapshot is available before migration or import.
- Private acceptance passes before public exposure.
- An off-host database dump is checksum-verified and restored successfully into
  a disposable database before cutover.
- Public cutover receives a separate explicit human approval.

A failed or ambiguous gate is a no-go. The deadline never authorizes bypassing
one.

## Attended runbook

### 1. Morning preflight

Reconcile live cloud state, recovered controller inputs, DNS, backups, and the
rollback target. Start the selected resources and wait for healthy status.

Stop the launch if the intended architecture and data cannot be identified
unambiguously by the morning cutoff agreed at the start of the session.

### 2. Freeze the release

Fix only launch-blocking defects. Require green CI, record the exact commit, and
publish one private image by immutable digest.

Stop if a green candidate is not ready by the agreed midday cutoff.

### 3. Review a fresh plan

Generate a fresh saved plan after state refresh. It may create the replacement
EIP and the minimum already-reviewed private runtime resources. It must not
contain an unexplained replacement, destruction, broad ingress rule, database
replacement, unrelated resource change, or secret value.

The human must approve this exact private-infrastructure plan before apply.
Regenerating the plan invalidates that approval.

Replacement and combined-recovery plan review must consume a fresh (at most 24
hours old), exact-key, value-blind current reconciliation checkpoint rather
than the superseded Slice 3 completion checkpoint. It binds the clean exact
commit; the selected Terraform-managed bootstrap host and database; the human-
approved host replacement and retained legacy rollback host; zero origin
ingress in the private phase; available private encrypted deletion-protected
RDS with backups and observed restorable metadata; the existing runtime-secret
container as unmanaged-by-Terraform with a numeric observed version count; safe
recovery bucket and remote encrypted locked state backend; and no approved or
performed public cutover. Replacement-only requires the managed EIP present
and unattached; combined recovery requires it absent so the reviewed plan may
create it unattached. The checkpoint never asserts secret contents and must not
require a zero version count. The historical create-mode contract remains
unchanged for compatibility. Any missing, false, stale, renamed, malformed, or
unexpected checkpoint field is a no-go.

### 4. Deploy privately

Apply only the approved plan. Run schema migration through the private operator
path, create or verify the lower-privilege runtime role, populate the runtime
secret privately, and start the pinned image. Keep the EIP unattached and public
traffic disabled.

Private acceptance covers login, logout, rejected session reuse, profile
read/write, persistence after container recreation, access-code rejection, one
representative scrape/evaluation path, one active scheduler, and absence of
unexpected public listeners.

Stop if private acceptance is not complete by the agreed afternoon cutoff.

### 5. Prove recovery

Create an off-host PostgreSQL dump, verify its checksum or manifest, restore it
into a disposable database, and compare the schema and representative row
counts. Keep the credential-encryption key backed up separately from the
host and database.

A successful backup command without a successful restore is insufficient.

### 6. Approve and perform cutover

Before cutover, present the release commit and image digest, plan summary,
private acceptance result, restore result, intended EIP/DNS/Cloudflare changes,
rollback commands, and remaining risks.

Only an explicit human approval authorizes EIP association, DNS changes,
Cloudflare changes, or public traffic.

After approval, expose only Caddy HTTPS, use strict origin TLS verification, and
run the critical user journey through the public hostname. Observe health,
logs, resource use, database connections, and one post-cutover recovery
artifact. Freeze unrelated changes.

## Data migration

Migrate existing alpha data only when it is required for launch:

1. preserve the source database and associated recovery files;
2. wait for the pre-import RDS snapshot;
3. run the importer in dry-run mode;
4. review its fingerprint, counts, and collisions;
5. apply the exact reviewed import; and
6. rerun it to verify idempotence.

Do not build dual writes or incremental synchronization for this launch. After
public PostgreSQL writes begin, PostgreSQL is authoritative; do not route users
back to a writable stale SQLite database.

## Rollback

Before public writes, stop the new runtime or leave it isolated. If migration or
import is wrong, restore the pre-change snapshot before accepting traffic.

After public writes, retain PostgreSQL and roll the application back only to a
schema-compatible immutable image. For database failure, take the service
unavailable and recover using RDS point-in-time recovery, the retained snapshot,
or the verified dump. Preserve prior hosts, databases, images, source data, and
recovery material until the human closes the rollback window.

## Deferred until after alpha launch

- scheduled Cloudflare prefix-list refresh and edge automation;
- HA, Multi-AZ conversion, autoscaling, ALB, ECS, Kubernetes, and PaaS migration;
- broad observability-platform work and exhaustive failure injection;
- infrastructure cleanup or destruction;
- open signup, Worknet, and optional paid-AI acceptance; and
- unrelated product, schema, dependency, or infrastructure refactoring.

Current Cloudflare-origin restrictions must still be reviewed and safe; only
the refresh automation is deferred.

## Superseded records

The earlier multi-slice specifications, plans, and authorization model are
preserved in the [simplified alpha deployment archive](../archive/2026-09-23-simplified-alpha-deployment/README.md).
