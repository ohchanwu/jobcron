# Human deploy guide for jobcron.app production

> **Alpha launch:** Use the
> [human-assisted alpha deployment specification](../../docs/specs/260923-human-assisted-alpha-deployment.md)
> as the controlling runbook. This document preserves detailed private-runtime
> procedures that may be selected during that attended run; it is not a mandate
> to execute the superseded multi-slice launch end to end.

This guide supports an explicit lean initial deployment and the historical
private Slice 4 replacement-host sequence. Neither authorizes a
public cutover. Use only the approved commit, private controller artifacts, and
short-lived operator credentials. Never put secrets, identifiers, addresses,
personal data, screenshots, or raw logs in Git, issues, chat, or shared command
output.

All host access uses AWS Systems Manager Session Manager. RDS remains private,
the replacement host has no key pair or inbound rule, and runtime values exist
on the host only below `/run/jobcron`. Stop immediately on any mismatch; never
infer or substitute a selector.

## Initial-deployment lean lane

Use this lane only after live/state reconciliation confirms there has never
been a Jobcron production service, production data, prior deployed image, or
legacy rollback host. The managed bootstrap is disposable: no Jobcron service
runs there and it contains no production data. The selected RDS is unused.
Unknown or contrary evidence is a stop condition, not permission to fabricate
legacy selectors. Obsolete ignored selector scripts are retired for this lane;
do not execute them or copy their assertions into the new checkpoint.

Keep a mode-`0700` controller directory outside the clean release checkout and
mode-`0600` regular files owned by the operator. Use `umask 077`. Reconcile the
remote encrypted, lockfile-enabled backend against live resources immediately
before planning. Populate the common exact fields listed in section 1 from
observed facts, with these **only** schema differences:

- `schema_version` is `human-assisted-initial-deployment-v1`.
- `bootstrap_host` is exactly
  `{"disposition":"replace-disposable-bootstrap","human_approved":true}`.
  This approves the proposed disposable-host action, not plan apply.
- Omit `legacy_rollback_host` entirely. It is forbidden even with `retained=false`.
- Add exactly `initial_deployment` with these fields:
  `prior_service_exists=false`, `prior_production_data_exists=false`,
  `prior_image_exists=false`, `legacy_rollback_host_exists=false`,
  `bootstrap_service_running=false`, `bootstrap_contains_production_data=false`,
  `database_unused=true`, and
  `rollback="stop-private-runtime-leave-public-routing-unchanged"`.
- Add exactly `evidence_security` with `state_secret_free=true`,
  `plan_secret_free=true`, and `shared_evidence_secret_free=true`. These are
  private inspection results, never guesses or claims about secret contents.
- `managed_eip.state_presence` may be `present` or `absent`, based on fresh
  reconciliation. A present EIP must remain unattached and unchanged; only an
  absent one may be created unattached. No legacy-host provenance is required.

All other exact fields, freshness (at most 24 hours, never future), clean exact
SHA binding, protected resources, cost ceilings, runtime-secret version count,
RDS protections, recovery bucket protections and no-cutover assertions remain
mandatory. A nonzero runtime-secret version count is valid; do not read secret
contents to establish that count. Inspect state and plan privately for secret
payloads without sharing them. The checker's credential-field/PEM/URL guards
are defense in depth, not an exhaustive secret scanner.

Follow sections 2 and 3 for immutable-image publication, synthetic Compose
validation and exact bootstrap rendering. Generate a **fresh binary saved
plan** with `-replace=aws_instance.replacement_host`, its JSON and fresh aggregate
cost evidence, then invoke directly:

```sh
scripts/check-terraform-slice-4-plan.sh \
  "$TF_SLICE4_PLAN_JSON" \
  "$TF_AGGREGATE_COST_JSON" \
  "$TF_CURRENT_RECONCILIATION_CHECKPOINT_JSON" \
  "$TF_SLICE4_RENDERED_USER_DATA" \
  "$JOBCRON_REVIEWED_SHA" \
  initial-deployment
```

The allowlist is one requested destroy-then-create of the disposable managed
bootstrap, plus **only when absent** one VPC EIP create. All other resources in
the exact controller inventory must be known no-ops; the only output transition
is the sensitive replacement instance ID. The checker retains the reviewed
bootstrap hash, unchanged AMI/type/subnet/profile/security-group binding, IMDSv2,
encrypted 8 GiB root disk, keyless host and private-phase checks. Updating EC2
user data does not replay cloud-init, so replacement intentionally discards the
unused bootstrap root volume to install the reviewed runtime. It is not a claim
that an old service is being replaced. No additional deletion/replacement,
database replacement, public ingress, EIP association, DNS/Cloudflare change,
secret version or unrelated drift is allowed. Refresh may report only the
already-reconciled missing EIP deletion; it is not an extra planned destruction.

Record the binary plan's SHA-256 digest privately. Independent review binds that
digest and the exact clean release with green CI and immutable private image.
Regenerating the plan or bootstrap invalidates review. Obtain separate explicit
apply approval before section 4; a checker `PASS` is not approval. Keep existing
automatic Git/PATH protections, but additional adversarial environment/object
audits and full-toolchain manifests are deferred, not new launch prerequisites.

For initial deployment only, select these branches in the remaining procedure:

- Section 6: initialize the empty RDS schema using `jobcron-user` built from the
  exact clean reviewed release, not the historical lineage/backfill recipe.
  Build `./cmd/jobcron-user` with `GOTOOLCHAIN=local CGO_ENABLED=0 go build
  -mod=readonly -trimpath -o "$migration_bin" ./cmd/jobcron-user` on the trusted
  operator machine; keep the binary outside the checkout. Check the checkout's
  full SHA/clean status before and after building and verify `go version -m`
  reports that exact `vcs.revision` and `vcs.modified=false`. Record its SHA-256
  digest privately. Run `"$migration_bin" migrate --database-url
  "$JOBCRON_MASTER_DATABASE_URL"` through the section-6 TLS tunnel and silent
  password prompt, with `JOBCRON_DATABASE_PASSWORD` unset. **Do not supply**
  `--backfill-legacy-migration-tree`, a previous-image commit, or a full-toolchain
  manifest. A legacy migration ledger or existing data contradicts this lane:
  stop rather than backfill it. Snapshotting the verified empty unused DB is
  deferred; an existing-data migration still requires its snapshot.
- Section 7: no production SQLite import; create the initial owner through the
  private `jobcron-user create-owner` path. Keep least-privilege role setup,
  private RDS, TLS, and credential handling unchanged.
- Sections 9–11: no prior image or old stack must be invented. Keep the current
  immutable image, fail-closed startup, private functional acceptance,
  access-code rejection, exactly one scheduler, persistence after container
  recreation and reboot checks. Record sanitized acceptance results.
- Section 12: after initial schema/data exists, require **one off-host
  `database.dump` plus `database.dump.sha256`, checksum verification and a
  successful disposable restore**, comparing schema and representative row
  counts. The existing archive service may generate these; copy the two exact
  objects privately to the trusted Mac, validate the checksum manifest names
  only `database.dump`, and verify with `shasum -a 256 -c database.dump.sha256`.
  Do not mark the whole six-object set `macbook-copy=verified` after a partial
  copy. The exhaustive six-object/log-retention matrix is deferred. Do not
  publish dumps/logs or skip the off-host restore; preserve the
  credential-encryption key separately using an approved private backup path.
- Section 13: preserve the current DB, image and recovery material. There is no
  legacy host, prior image or prior production data to preserve. On failure stop
  privately and leave public routing unchanged. Public writes require separate
  attended EIP/DNS/Cloudflare/cutover approval; after writes, PostgreSQL remains
  authoritative, and a failure may require unavailability and recovery rather
  than a nonexistent old service.

## 1. Record the current reconciliation checkpoint

For replacement and combined-recovery plans, create fresh value-blind evidence
in the protected controller directory. The checker accepts only
`schema_version = "human-assisted-reconciliation-v1"`, rejects unexpected or
renamed keys at every object boundary, and requires `checked_at` to be a valid
UTC timestamp no more than 24 hours old and not in the future. Record only:

- `commit`: a 40-lowercase-hex `sha` with `exact` and `clean` both true;
- `selected_state_resources`: the managed Terraform addresses
  `aws_instance.replacement_host` and `aws_db_instance.production`;
- `bootstrap_host`: disposition `replace` with `human_approved = true`, plus a
  retained `legacy_rollback_host`;
- `managed_eip`: address `aws_eip.origin`, `unattached = true`, and
  `state_presence = "present"` for replacement-only or `"absent"` for
  combined recovery;
- `origin`: zero ingress rules and phase `private`;
- `rds`: status `available`, not publicly accessible, encrypted,
  deletion-protected, positive integral backup retention, and observed latest-
  restorable-time metadata;
- `runtime_secret`: the container exists, Terraform does not manage versions,
  and `observed_version_count` is a nonnegative integer. The count may be
  nonzero and proves no secret content;
- `recovery_bucket`: encrypted, versioned, public-blocked, TLS-only, with its
  lifecycle verified;
- `state_backend`: remote, encrypted, and lockfile-enabled; and
- `public_cutover`: neither approved nor performed.

Do not include account IDs, resource IDs, endpoints, secret values, or
restorable timestamps. A false, missing, stale, malformed, renamed, or extra
field is a stop condition. In these historical recovery modes, preserve the
selected legacy host, database, prior image, and recovery materials. Initial
deployment instead uses the exact differences above; never reuse this legacy
checkpoint unchanged.

The three-argument create mode is a compatibility boundary for the historical
Slice 3 launch path and still requires its original exact checkpoint, including
an empty zero-version secret container. Do not use that historical checkpoint
for replacement or combined recovery.

## 2. Publish the immutable private image

From the exact clean implementation commit, dispatch
`.github/workflows/publish-production-image.yml`. The workflow must use only
`GITHUB_TOKEN`, publish one private `linux/arm64` package, and record the
approved commit and immutable digest in the private `image.json` evidence.

Verify package visibility, platform, commit label, and digest without copying
private package metadata into shared output. The host must consume
`ghcr.io/<owner>/jobcron@sha256:<digest>`, never a mutable tag.

## 3. Generate and review the saved plan

Before loading private inputs, render Compose with `.env.example` and inspect
the private temporary file fail-closed:

```sh
cd deploy/production
umask 077
rendered_compose="$(mktemp)"
trap 'rm -f "$rendered_compose"' EXIT HUP INT TERM
docker compose --env-file .env.example config \
  2>/dev/null >"$rendered_compose"
sh ../../scripts/inspect-production-compose-render.sh \
  "$rendered_compose"
rm -f "$rendered_compose"
trap - EXIT HUP INT TERM
cd ../..
```

Stop if rendering or inspection fails. The temporary file contains synthetic
values only and is removed on success, failure, or interruption.

Load the ignored `controller.env` and private Terraform inputs without printing
them. Create `slice-4.tfplan`, its JSON form, and the aggregate cost evidence in
the protected controller directory. Run:

```sh
scripts/check-terraform-slice-4-plan.sh \
  "$TF_SLICE4_PLAN_JSON" \
  "$TF_AGGREGATE_COST_JSON" \
  "$TF_SLICE3_CHECKPOINT_JSON"
```

Require the exact five creates, one sensitive output create, no other action,
fresh aggregate cost within both ceilings, and a passing Slice 3 checkpoint.
An independent reviewer must approve the exact commit and saved-plan digest.
Regenerating the plan invalidates that review. This historical create mode is
the only three-argument invocation.

If an independently approved recovery must replace an already-created host,
do not reuse the create-plan verdict and do not infer that updating `user_data`
will replay cloud-init. Render the exact evaluated bootstrap privately:

```sh
umask 077
render_json="$(mktemp)"
trap 'rm -f "$render_json" "$TF_SLICE4_RENDERED_USER_DATA"' EXIT HUP INT TERM
terraform -chdir=infra/terraform/production console >"$render_json" <<'EOF'
jsonencode(local.replacement_user_data)
EOF
jq -ejr 'fromjson | if type == "string" and length > 0 then . else error("invalid") end' \
  "$render_json" >"$TF_SLICE4_RENDERED_USER_DATA"
chmod 0600 "$TF_SLICE4_RENDERED_USER_DATA"
rm -f "$render_json"
```

Create the saved plan with the explicit
`-replace=aws_instance.replacement_host` option, render its JSON without
printing it, then run the checker with the current reconciliation checkpoint,
private rendered bootstrap, and independently reviewed full lowercase 40-hex
commit as the third through fifth arguments:

```sh
scripts/check-terraform-slice-4-plan.sh \
  "$TF_SLICE4_PLAN_JSON" \
  "$TF_AGGREGATE_COST_JSON" \
  "$TF_CURRENT_RECONCILIATION_CHECKPOINT_JSON" \
  "$TF_SLICE4_RENDERED_USER_DATA" \
  "$JOBCRON_REVIEWED_SHA"
```

This mode requires exactly one destroy-then-create action for the replacement
instance with explicit replace-by-request provenance, every other protected
resource as a no-op, the sensitive instance-ID output transition, unchanged
AMI, instance type, subnet, instance profile, and security-group binding,
keyless public-IP behavior before and after, IMDSv2, the encrypted 8 GiB root
volume, and the exact tracked bootstrap asset digests. It rejects extra
actions, diagnostics, unknown recovery or security controls, stale assets, or
a rendered bootstrap that is not a regular mode-`0600` file. Remove the
rendered file after the exact saved-plan digest receives independent approval;
regenerating either artifact invalidates that approval.

The checkpoint commit SHA must equal the explicit reviewed SHA exactly. The
checker also requires that SHA to be the checkout's literal `HEAD`, a clean
tracked/staged/untracked state, no replacement refs or hostile local Git
configuration, and the checker plus every bootstrap asset at the script's own
repository root as tracked files in that checkout. Git replacement objects,
hooks, ambient configuration, external attributes, and excludes are disabled
or rejected for this boundary, as are enabled Git extensions, per-worktree
configuration files, promisor remotes, and untracked-file hiding. Failures
emit only the generic contract error.

Execute `scripts/check-terraform-slice-4-plan.sh` directly as shown, not by
sourcing it or passing it to an ambient shell. Its `/bin/bash -p` launcher
ignores exported functions, startup files (`BASH_ENV`/`ENV`), and inherited
shell options before unconditionally starting the adjacent tracked
`check-terraform-slice-4-plan-body.sh` with a cleared environment. Both files
belong to the reviewed checkout. Validation restricts `PATH` to root-owned,
non-group- or other-writable system tool directories (`/usr/bin`, `/bin`, and
`/usr/local/bin` only when trusted). Caller-supplied functions and PATH wrappers
cannot forge a verdict; a missing required trusted tool fails closed. Both
stages require Bash at `/bin/bash`; do not invoke the internal body directly.

Every plan, cost, and checkpoint file must contain exactly one valid JSON
document. Empty, whitespace-only, malformed, or multiple-document files fail
with the same generic error, including on jq 1.6. The document-count boundary
and semantic checks use the same parse; no input values or parser errors are
printed. Keep these artifacts in the protected controller directory and do
not modify them during review.

If current reconciliation confirms that the deleted address was exactly the
Terraform-managed `aws_eip.origin`, and the approved saved plan combines its
recovery with the explicit host replacement, set `managed_eip.state_presence`
to `"absent"` and add the separate exact sixth argument `combined-recovery`
after the reviewed SHA:

```sh
scripts/check-terraform-slice-4-plan.sh \
  "$TF_SLICE4_PLAN_JSON" \
  "$TF_AGGREGATE_COST_JSON" \
  "$TF_CURRENT_RECONCILIATION_CHECKPOINT_JSON" \
  "$TF_SLICE4_RENDERED_USER_DATA" \
  "$JOBCRON_REVIEWED_SHA" \
  combined-recovery
```

This fail-closed mode accepts exactly the unattached VPC-scoped
`aws_eip.origin` create and the explicitly requested replacement-host
destroy-then-create. Every other protected resource must be a no-op. Do not use
this mode for an EIP import, EIP replacement, association, any second create or
replacement, or any plan with another drift action.

## 4. Apply only the reviewed replacement-host plan

Obtain separate explicit apply approval. Recheck the saved-plan digest, then
apply the binary plan exactly once:

```sh
terraform -chdir=infra/terraform/production apply -input=false \
  "$TF_SLICE4_PLAN"
```

Verify value-blind that Session Manager sees the host; port `22`, a key pair,
and inbound rules are absent; IMDSv2 and the encrypted 8 GiB root volume are
enforced; IAM is limited to runtime-secret read and new recovery-object writes;
the reserved EIP is unattached; other protected resources are unchanged; and
`jobcron.service` is installed but stopped. A fresh Terraform plan must then be
clean.

## 5. Use Session Manager for host and RDS access

Load the private instance selector and forwarding parameters, then open the
localhost-only RDS tunnel in a dedicated trusted-Mac terminal:

```sh
aws ssm start-session \
  --target "$REPLACEMENT_INSTANCE_ID" \
  --document-name AWS-StartPortForwardingSessionToRemoteHost \
  --parameters "$SSM_FORWARD_PARAMETERS"
```

Confirm the listener is bound to `127.0.0.1` and RDS is still private. Use
separate Session Manager sessions for host operations and later private
verification. Do not capture the commands or resolved parameters in tracked
evidence.

## 6. Apply schema migrations through the private tunnel

Initial deployment uses the no-lineage branch above. The following full recipe
is only for a deployment with an existing version-only migration ledger and a
real previous deployed image; do not execute it for an unused initial database.

Before the first runtime start, bind the migration binary to the full exact SHA
approved by independent review. Refuse a different HEAD, any tracked or
untracked worktree change, or any stash:

```sh
(
set -eu
umask 077
PATH=/usr/bin:/bin
export PATH
export JOBCRON_REVIEWED_SHA='<approved-40-hex-commit>'
export JOBCRON_PREVIOUS_IMAGE_COMMIT='<commit from the private deployed image.json>'
export JOBCRON_GO_BINARY='<approved-absolute-go-binary>'
export JOBCRON_GO_TOOLCHAIN_DIGEST='<approved-64-hex-complete-goroot-manifest-digest>'
export GIT_NO_REPLACE_OBJECTS=1
printf '%s\n' "$JOBCRON_REVIEWED_SHA" | grep -Eq '^[0-9a-f]{40}$'
printf '%s\n' "$JOBCRON_PREVIOUS_IMAGE_COMMIT" | grep -Eq '^[0-9a-f]{40}$'
printf '%s\n' "$JOBCRON_GO_TOOLCHAIN_DIGEST" | grep -Eq '^[0-9a-f]{64}$'
case $JOBCRON_GO_BINARY in /*) ;; *) exit 1 ;; esac
test -x "$JOBCRON_GO_BINARY"
go_dir=$(CDPATH= cd -P "${JOBCRON_GO_BINARY%/*}" && pwd)
go_root=$(CDPATH= cd -P "$go_dir/.." && pwd)
toolchain_manifest_dir=$(mktemp -d)
toolchain_unsupported="$toolchain_manifest_dir/unsupported"
toolchain_paths="$toolchain_manifest_dir/paths"
toolchain_sorted_paths="$toolchain_manifest_dir/sorted-paths"
toolchain_manifest="$toolchain_manifest_dir/manifest"
cleanup_toolchain_manifest() {
  rm -f -- "$toolchain_unsupported" "$toolchain_paths" \
    "$toolchain_sorted_paths" "$toolchain_manifest"
  rmdir "$toolchain_manifest_dir"
}
trap cleanup_toolchain_manifest EXIT
trap 'exit 1' HUP INT TERM
(cd "$go_root" && find . ! -type d ! -type f -print0 >"$toolchain_unsupported")
test ! -s "$toolchain_unsupported"
(cd "$go_root" && find . -type f -print0 >"$toolchain_paths")
test -s "$toolchain_paths"
LC_ALL=C sort -z <"$toolchain_paths" >"$toolchain_sorted_paths"
(cd "$go_root" && xargs -0 shasum -a 256 <"$toolchain_sorted_paths" >"$toolchain_manifest")
computed_go_toolchain_digest=$(shasum -a 256 "$toolchain_manifest")
computed_go_toolchain_digest=${computed_go_toolchain_digest%% *}
test "$computed_go_toolchain_digest" = "$JOBCRON_GO_TOOLCHAIN_DIGEST"
cleanup_toolchain_manifest
trap - EXIT HUP INT TERM
run_git() {
  env -i PATH=/usr/bin:/bin HOME=/var/empty GIT_CONFIG_NOSYSTEM=1 \
    GIT_CONFIG_GLOBAL=/dev/null GIT_NO_REPLACE_OBJECTS=1 \
    /usr/bin/git -c core.hooksPath=/dev/null -c core.attributesFile=/dev/null \
      -c core.excludesFile=/dev/null "$@"
}
test "$(run_git rev-parse HEAD)" = "$JOBCRON_REVIEWED_SHA"
test -z "$(run_git status --porcelain=v1 --untracked-files=all)"
test -z "$(run_git stash list)"
run_git cat-file -e "$JOBCRON_PREVIOUS_IMAGE_COMMIT^{commit}"
previous_migration_tree=$(run_git rev-parse \
  "$JOBCRON_PREVIOUS_IMAGE_COMMIT:internal/storage/postgres_migrations")
reviewed_migration_tree=$(run_git rev-parse \
  "$JOBCRON_REVIEWED_SHA:internal/storage/postgres_migrations")
test "$previous_migration_tree" = "$reviewed_migration_tree"
migration_dir=$(mktemp -d)
migration_bin="$migration_dir/jobcron-user-$JOBCRON_REVIEWED_SHA"
migration_builder="$migration_dir/build-reviewed-jobcron-user"
cleanup_migration_binary() {
  rm -f -- "$migration_bin" "$migration_builder"
  rmdir "$migration_dir"
}
trap cleanup_migration_binary EXIT
trap 'exit 1' HUP INT TERM
repo_root=$(run_git rev-parse --show-toplevel)
run_git show "${JOBCRON_REVIEWED_SHA}:scripts/build-reviewed-jobcron-user.sh" >"$migration_builder"
chmod 500 "$migration_builder"
"$migration_builder" "$repo_root" "$JOBCRON_REVIEWED_SHA" "$migration_bin" "$JOBCRON_GO_BINARY" "$JOBCRON_GO_TOOLCHAIN_DIGEST"
test "$(run_git rev-parse HEAD)" = "$JOBCRON_REVIEWED_SHA"
test -z "$(run_git status --porcelain=v1 --untracked-files=all)"
test -z "$(run_git stash list)"
unset JOBCRON_DATABASE_PASSWORD
"$migration_bin" migrate \
  --database-url "$JOBCRON_MASTER_DATABASE_URL" \
  --backfill-legacy-migration-tree "$previous_migration_tree"
)
```

The builder itself comes from the reviewed commit. It disables Git replacement
objects, clones the reviewed object into a private repository that has no local
attributes or configuration, and then removes the clone metadata before the
build. The Go command is an explicitly selected absolute local binary and runs
under an environment allowlist with `GOTOOLCHAIN=local`, `CGO_ENABLED=0`, private
caches, module checksum verification, and `-mod=readonly`. Local Git metadata,
ignored files, overlays, ambient build controls, and concurrent worktree edits
cannot enter the privileged binary. The builder rejects symlinks and other
non-regular GOROOT entries, checks every manifest stage explicitly, and verifies
a SHA-256 manifest of every regular file in the selected Go distribution before
allowing the toolchain to run; executable-only or lone-`go` hashes are insufficient.

The previous image commit comes from the private immutable-image evidence, not
from a mutable branch or tag. Exact migration-tree object equality proves that
the version-only production ledger was created by the same migration bytes now
embedded in the reviewed binary. Only after that audit may the explicit
backfill argument add each filename and pinned SHA-256 digest; the binary also
requires that audited tree object to equal its pinned migration tree. Without
the argument, a legacy ledger fails closed; a tree, filename, or digest mismatch
always fails closed.

The URL must contain the master username but no password, use exactly the
`127.0.0.1` Session Manager tunnel, and set only `sslmode=require`.
`verify-full` cannot verify the RDS certificate against a loopback hostname.
For this production run, leave `JOBCRON_DATABASE_PASSWORD` unset and enter the
AWS-managed master password only through the silent stdin prompt. The command's
environment source exists for controlled automation and tests, not this manual
path. It emits only `database_migrations_ready=true`; the master credential
never reaches the host or a command argument. The operation has a two-minute
total deadline and serializes concurrent migrators with a PostgreSQL advisory
lock.

## 7. Create the lower-privilege database role

Set the helper's private inputs to the localhost-only master URL, private RDS
endpoint, application role name, owner-only `database-role.env`, and
owner-only `runtime-secret.json`. Then run:

```sh
scripts/production-rds-role.sh
```

Enter the master and application passwords only through the helper's silent
stdin prompts. It grants connect, schema usage, application-table DML, sequence
usage, and read-only access to `schema_migrations`; the runtime role cannot
forge the migration ledger or create a database, superuser, extension, or
replication role. Existing elevated role attributes are removed, while any role
membership or ownership of the production database, public schema, or public
relations makes the transaction fail closed. Direct and public ledger writes
are revoked, and effective `SELECT`/`INSERT`/`UPDATE`/`DELETE` privileges are
checked before readiness. Verify the catalog grants without printing names or
passwords. The helper stores the lower-privilege TLS `DATABASE_URL` only in the
private runtime JSON and emits only `database_role_ready=true`. Run this helper
after every operator migration so newly created tables and sequences receive
the runtime grants and ledger writes are revoked again.

If the approved legacy SQLite import is still required, keep the source and
optional key file on the trusted Mac. Through the same tunnel, create exactly
one owner, dry-run `jobcron-import`, review the immutable fingerprint, all
category counts and collisions, then repeat the identical approved command
with `--apply`. Never send the RDS master credential or legacy files to the
host.

## 8. Populate the runtime secret outside Terraform

Complete `runtime-secret.json` locally with exactly:

- the lower-privilege TLS database URL;
- session, credential-encryption, proxy, and cohort values;
- the Stage 1 sponsor user ID;
- the approved immutable image digest; and
- the Origin CA certificate and private key.

Validate exact keys, non-empty string values, digest shape, TLS mode, and
certificate/key shape without printing content. Add one Secrets Manager version
using the private file input. Terraform must create no secret version, and its
state and plan must remain unchanged.

Through a Session Manager host session, write the separate classic
`read:packages` token from stdin to `/run/jobcron/registry-token` with mode
`0600`. It must not be `GITHUB_TOKEN`, part of the runtime secret, or retained
after the image pull.

## 9. Prove the systemd runtime fails closed, then start

First select an intentionally incomplete synthetic secret version and start
`jobcron.service`. Confirm the prior stack is stopped, preparation fails,
neither container runs, and `/run/jobcron` contains no partial secret output.
Restore the approved complete secret version.

Start the service again through systemd. It must run `prepare`, digest pull, and
Compose in that order. Confirm:

- the registry token and temporary Docker configuration are removed;
- no home-directory Docker credential exists;
- the current digest (and previous digest, if one exists) remains available;
- both containers are healthy with bounded JSON log rotation;
- the app uses TLS and the lower-privilege role; and
- the app is published only on loopback port `7777`, Caddy is the sole listener
  on host TCP `443`, the origin security group still has no ingress, and the
  reserved EIP remains unattached.

Run `/opt/jobcron/jobcron-runtime.sh verify-local-state` and record only its
value-blind booleans and counts.

## 10. Complete private verification

Forward trusted-Mac ports through Session Manager to host ports `7777` and
`443`. With the required headless browser workflow, walk the login page,
owner login, dashboard, profile read/save, archive, one cohort-safe scrape or
re-rate, logout, and failed-session reuse. Verify expected content and state,
not only an HTTP status.

Separately verify Caddy's Origin CA certificate with the private CA material;
do not bypass certificate verification. Record sanitized results in the private
`private-verification.md`. This is private verification only.

## 11. Prove reboot recovery

After private verification succeeds, enable `jobcron.service` and reboot the
replacement host. Confirm the memory-backed runtime directory was cleared,
systemd recreated complete files with modes `0700` and `0600`, no secret or TLS
key persisted elsewhere, and the already-present approved digest starts without
another registry token.

Any incomplete secret, wrong mode, failed pull, unhealthy container, or failed
user-path check is a stop condition. Unexpected external reachability or
unexpected security-group ingress is also a stop condition. The intentional
Caddy listener on host TCP `443` is not public while the origin security group
has no ingress and the reserved EIP remains unattached.

## 12. Verify recovery manifests and restore

The exhaustive procedure below is for the historical recovery lane. Initial
deployment uses the dump/checksum/disposable-restore minimum above instead.

Run `jobcron-recovery.service` once and enable its timer only after that run
succeeds. The service uploads a custom-format database dump, sanitized Jobcron
and Caddy logs, and one SHA-256 recovery manifest for each artifact.
It accepts only the generated TLS RDS URL, passes a password-free URL as
`pg_dump`'s database argument, and supplies the decoded password only through
the child environment. Confirm the process arguments and sanitized evidence do
not contain either the encoded or decoded database password.

On the trusted Mac, set the private bucket, timestamped prefix, and owner-only
destination, then run:

```sh
scripts/pull-production-recovery.sh
```

The helper copies missing objects, validates the exact six-object set, verifies
all manifests, and only then applies `macbook-copy=verified`. Restore the dump
into a disposable database, compare schema and bounded table counts, and inspect
the logs for headers, cookies, tokens, secrets, and unnecessary personal data.
Verify the reviewed tagged and untagged retention cases without deleting any
object.

Record only sanitized outcomes in `recovery-verification.md`. Failed download,
manifest, tagging, restore, row-count, or sanitization checks are stop
conditions.

## 13. Stop before public cutover

Run a final no-change Terraform plan and confirm the database, reserved EIP,
current image, and recovery materials remain available. For historical recovery
also preserve the old host and prior image. Confirm no
Cloudflare, DNS, public ingress, or public traffic change occurred.

Do not associate the reserved EIP, change edge configuration, or accept public
traffic. Keep the rollback window open. Initial deployment proceeds only under
the active specification's separate attended cutover approval. Historical
Slice 5 may begin only from the exact private checkpoint after every applicable
stop condition above is clear.
