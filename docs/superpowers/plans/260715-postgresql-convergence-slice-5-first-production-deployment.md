# PostgreSQL Convergence Slice 5: First Production Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prepare and perform the first production deployment with encrypted
database-backed credentials, verified migrated state, durable container
recreation, and accurate public documentation and screenshots.

**Architecture:** Production Compose passes RDS, session, and credential master
secrets as environment variables and mounts only Caddy state. Before the first
app start, a trusted local checkout reaches private RDS through a localhost-only
SSH tunnel, creates the sole owner, snapshots RDS, and runs the verified
importer. Browser journeys then prove the migrated state and encrypted
credential survive app-container recreation.

**Tech Stack:** Docker Compose v2, AWS EC2, AWS RDS PostgreSQL 18, Caddy, Go
operator commands, SSH local forwarding, gstack `/browse`, Gitleaks.

## Global Constraints

- Start only after Slices 2 through 4 pass on the exact candidate commit.
- Treat the exact prior-slice commit named in the bead as authoritative, not
  `origin/main`. A separate worker clone must fetch that commit from the Mayor
  rig and verify its hash before editing.
- Do not use the normal `gt done` path because it submits an MR. Commit locally,
  report the exact tip and verification evidence, then defer so Mayor can fetch
  the clone and integrate without pushing.
- Follow the approved
  [convergence specification](../specs/260714-postgresql-local-convergence-user-ai-credentials.md).
- Current production reality is a blank application host: the EC2 instance has
  the application `.env`, but no Docker installation, app deployment, owner, or
  legacy credential volume.
- Preserve the existing `DATABASE_URL`, `SESSION_SECRET`, `JOBCRON_ENV`, host,
  port, no-open, and daily scrape values. Add only the missing image reference
  and the credential-encryption key.
- Never put real secrets, host addresses, database identifiers, account
  identity, source paths, or screenshot user data in Git, chat, issue text, or
  captured logs.
- Production migration runs from a trusted local checkout through a
  localhost-only tunnel. Do not add an importer service or copy SQLite/key files
  to EC2.
- Do not inspect or clean a nonexistent credential volume. The relevant test is
  that final Compose does not declare or mount one.
- Do not mutate AWS, DNS, a registry, RDS, or EC2 until the human explicitly
  authorizes deployment execution. Code and documentation preparation may be
  committed locally before that authorization.
- Keep Git commits local. Never run `git push` or create a pull request.

## Slice Completion Contract

This slice is complete only when the final Compose contract passes, the exact
candidate image is available, the sole owner and verified import exist in RDS,
the app is deployed, authenticated browser journeys pass, one approved paid AI
rating succeeds, app-container recreation preserves all state, and sanitized
light/dark README screenshots and English/Korean docs match the deployed
behavior.

---

### Task 1: Remove filesystem credential storage from production Compose

**Files:**

- Modify: `deploy/production/compose.yaml`
- Modify: `deploy/production/.env.example`
- Create: `deploy/production/compose_test.go`

- [ ] **Step 1: Write a failing Compose contract test**

Render `docker compose config` with synthetic values and assert:

```go
func TestProductionComposeRequiresCredentialEncryptionKey(t *testing.T)
func TestProductionComposeHasNoJobcronConfigVolumeOrMount(t *testing.T)
func TestProductionComposeRetainsDatabaseSessionAndCaddyState(t *testing.T)
func TestProductionComposeUsesImmutableImageReference(t *testing.T)
```

The absence test must inspect parsed rendered services and top-level volumes;
do not pass by grepping comments alone.

- [ ] **Step 2: Update the production service**

Remove:

```yaml
volumes:
  - jobcron_config:/root/.config/jobcron
```

and the top-level `jobcron_config` declaration. Add:

```yaml
JOBCRON_CREDENTIAL_ENCRYPTION_KEY: >-
  ${JOBCRON_CREDENTIAL_ENCRYPTION_KEY:?set JOBCRON_CREDENTIAL_ENCRYPTION_KEY in .env}
```

Keep `DATABASE_URL`, `SESSION_SECRET`, the immutable `JOBCRON_IMAGE`, and both
Caddy volumes.

- [ ] **Step 3: Update the environment template**

Add an empty placeholder and a safe generation command:

```dotenv
# Base64 for exactly 32 random bytes; keep it separately backed up.
JOBCRON_CREDENTIAL_ENCRYPTION_KEY=
```

Do not add any real value. Document generation with `openssl rand -base64 32`
and validation without echoing the result.

- [ ] **Step 4: Run contract tests and commit**

```sh
go test ./deploy/production -run ProductionCompose -count=1
JOBCRON_IMAGE='example.invalid/jobcron:sha-test' \
DATABASE_URL='postgres://user:pass@db.example.invalid:5432/jobcron?sslmode=require' \
SESSION_SECRET='synthetic-session-secret-at-least-32-bytes' \
JOBCRON_CREDENTIAL_ENCRYPTION_KEY='MDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDA=' \
  docker compose -f deploy/production/compose.yaml config >/tmp/jobcron-compose.yaml
! rg -n 'jobcron_config|/root/.config/jobcron' /tmp/jobcron-compose.yaml
git add deploy/production/compose.yaml deploy/production/.env.example \
  deploy/production/compose_test.go
git diff --cached --check
git diff --cached
gitleaks git --staged --redact --no-banner
git commit -m "feat(deploy): use database-backed production credentials"
```

### Task 2: Rewrite the first-deployment guide to match the blank host

**Files:**

- Modify: `deploy/production/HUMAN_DEPLOY_GUIDE.md`
- Modify: `deploy/production/README.md`

- [ ] **Step 1: Replace stale sequencing and volume guidance**

The guide must state:

- no app or Docker stack is currently deployed;
- the existing EC2 `.env` already contains the configured database URL,
  session secret, environment, host, port, no-open setting, and daily time;
- the operator adds `JOBCRON_IMAGE` and
  `JOBCRON_CREDENTIAL_ENCRYPTION_KEY` without replacing existing secrets;
- owner creation and import happen from the Mac through the tunnel before the
  first app start; and
- production has no legacy credential volume or migration container.

Delete instructions that start the app to apply migrations before owner
creation; `jobcron-user` opens PostgreSQL and applies embedded migrations.

- [ ] **Step 2: Document the exact private-RDS sequence**

Use placeholders and this order:

1. open `ssh -o ExitOnForwardFailure=yes -N -L
   127.0.0.1:15432:<private-rds-host>:5432 <ec2-user>@<ec2-host>`;
2. export the tunneled URL, owner email, and the production credential master
   key only in the current trusted shell;
3. run `go run ./cmd/jobcron-user create-owner`;
4. create the pre-import RDS snapshot;
5. run `go run ./cmd/jobcron-import` without `--apply`;
6. review fingerprint, eight category counts, provider count, and collisions;
7. rerun the identical command with `--apply`; and
8. unset variables and close the tunnel after verification.

The guide must say the importer preserves the new password hash and does not
import sessions or passwords.

- [ ] **Step 3: Document rollback by deployment phase**

- Before import commit: fix the cause and rerun dry-run.
- After import but before new writes: restore the pre-import RDS snapshot.
- After new writes: keep PostgreSQL authoritative and roll back application
  code or restore PostgreSQL; never return to SQLite.

Keep the original SQLite snapshot, legacy key file, master key backup, and RDS
snapshot until the human closes the rollback window.

- [ ] **Step 4: Check public-safety language and commit**

```sh
rg -n 'jobcron_config|ai_keys\.json|first app start|create-owner|--apply' \
  deploy/production
git diff --check
git add deploy/production/HUMAN_DEPLOY_GUIDE.md deploy/production/README.md
git diff --cached
gitleaks git --staged --redact --no-banner
git commit -m "docs(deploy): define blank-slate first production rollout"
```

Expected: legacy file references describe the retained local import source only;
no text claims EC2 stores it.

### Task 3: Add the production verification harness

**Files:**

- Create: `scripts/verify-production-compose.sh`
- Create: `scripts/verify_production_compose_test.go`
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Write the harness tests first**

The script must accept only synthetic environment values in CI and verify:

- required variables fail closed;
- rendered Compose has no app filesystem mount;
- app is not published directly on host port `7777`;
- Caddy alone publishes `80` and `443`;
- database, session, credential key, production mode, and scheduler settings
  reach the app; and
- image reference is an immutable `sha-<12-hex>` tag or approved digest.

- [ ] **Step 2: Implement a non-secret render verifier**

Write rendered output to a private temporary file, inspect structured fields,
and remove the file on exit. On failure, report the missing contract field, not
the environment value.

- [ ] **Step 3: Add CI coverage and run it**

```sh
sh -n scripts/verify-production-compose.sh
go test ./scripts -run ProductionCompose -count=1
sh scripts/verify-production-compose.sh
```

CI supplies only documented synthetic placeholders.

- [ ] **Step 4: Commit the harness**

```sh
git add scripts/verify-production-compose.sh \
  scripts/verify_production_compose_test.go .github/workflows/ci.yml
git commit -m "test(deploy): enforce production compose contract"
```

### Task 4: Build and publish the exact candidate image

**Files:**

- Verify: `deploy/production/Dockerfile`
- Verify: `deploy/production/compose.yaml`

This task changes registry state. Execute only after explicit human deployment
authorization.

- [ ] **Step 1: Prove the candidate commit is clean and fully verified**

```sh
git status --short
git rev-parse HEAD
test -z "$(gofmt -l .)"
go vet ./...
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' go test ./... -count=1
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' \
  go test -race ./... -count=1
goreleaser check
sh scripts/verify-production-compose.sh
```

Stop if the tree is dirty, tests skip PostgreSQL coverage, or any gate fails.

- [ ] **Step 2: Build an immutable arm64 image**

```sh
RELEASE_COMMIT="$(git rev-parse HEAD)"
RELEASE_TAG="sha-$(git rev-parse --short=12 HEAD)"
IMAGE="<registry-user>/jobcron:${RELEASE_TAG}"
docker buildx build --platform linux/arm64 \
  -f deploy/production/Dockerfile -t "$IMAGE" --push .
docker buildx imagetools inspect "$IMAGE"
```

Verify the manifest architecture and record the digest in the access-controlled
deployment log. Do not paste registry credentials or private metadata into Git.

### Task 5: Prepare EC2 and preserve the existing environment

**Files:** None tracked.

This task changes EC2. Execute only after explicit human deployment
authorization.

- [ ] **Step 1: Install Docker and Compose on the blank host**

Confirm the host OS, then use the matching documented package manager. For the
expected Amazon Linux 2023 host:

```sh
sudo dnf install -y docker
sudo systemctl enable --now docker
sudo usermod -aG docker "$USER"
```

Reconnect once, install the Compose v2 plugin for the host architecture, and
verify `docker version` plus `docker compose version`.

- [ ] **Step 2: Place the approved deployment files**

Create the application directory, obtain the exact approved commit without
rewriting history, and confirm:

```sh
git rev-parse HEAD
git status --short
```

The commit must equal `RELEASE_COMMIT` and the tree must be clean.

- [ ] **Step 3: Add only the missing environment entries**

Back up `.env` with owner-only permissions. Preserve all existing values. Add:

```dotenv
JOBCRON_IMAGE=<immutable-image-from-task-4>
JOBCRON_CREDENTIAL_ENCRYPTION_KEY=<base64-32-byte-master-key>
```

Generate the key on a trusted machine, store its backup separately from RDS,
and validate decoded length without printing it. Do not replace the existing
database URL or session secret.

- [ ] **Step 4: Validate Compose before starting anything**

```sh
cd <production-compose-directory>
docker compose --env-file .env config --quiet
docker compose --env-file .env config > /tmp/jobcron-production-compose.yaml
! rg -n 'jobcron_config|/root/.config/jobcron' \
  /tmp/jobcron-production-compose.yaml
rm -f /tmp/jobcron-production-compose.yaml
```

Do not start the app yet.

### Task 6: Create the owner, snapshot RDS, and run the verified import

**Files:** None tracked.

This task changes RDS. Execute only after explicit human import authorization
and approval of the retained local source and owner identity.

- [ ] **Step 1: Open the localhost-only SSH tunnel**

Use `ExitOnForwardFailure`, bind only `127.0.0.1:15432`, and verify the tunnel
from the trusted local checkout. Never make RDS public.

Export `JOBCRON_CREDENTIAL_ENCRYPTION_KEY` from the same protected value added
to EC2 `.env`. Validate its decoded length without printing it.

- [ ] **Step 2: Create exactly one owner**

```sh
go run ./cmd/jobcron-user create-owner \
  --database-url "$TUNNELED_DATABASE_URL" \
  --email "$OWNER_EMAIL"
```

Use the secure password prompt. Query only the user count afterward; expected
count is exactly one.

- [ ] **Step 3: Create and wait for the pre-import RDS snapshot**

Create a manual snapshot through the approved AWS operator surface and wait
until its status is `available`. Record its identifier only in the
access-controlled deployment log.

- [ ] **Step 4: Run and approve dry-run**

```sh
go run ./cmd/jobcron-import \
  --sqlite '<immutable-sqlite-snapshot>' \
  --postgres "$TUNNELED_DATABASE_URL" \
  --owner-email "$OWNER_EMAIL" \
  --ai-keys '<optional-legacy-key-file>'
```

Review fingerprint, all eight categories, credential provider count, and zero
unapproved collisions. Stop on any mismatch.

- [ ] **Step 5: Apply and independently verify**

Rerun the identical command with `--apply`. Then rerun it once more without
`--apply`; expected output is `already imported` plus successful verification.
Confirm the owner password hash did not change and PostgreSQL contains no
plaintext key.

### Task 7: Start production and walk the authenticated browser journeys

**Files:** None tracked until sanitized screenshots are captured in Task 8.

This task changes production. Use the gstack `/browse` skill for every browser
step; do not open the user's default browser.

- [ ] **Step 1: Pull and start the exact image**

```sh
docker compose --env-file .env pull
docker compose --env-file .env up -d
docker compose --env-file .env ps
docker compose --env-file .env logs --no-color --tail=200 app caddy
```

Expected: both services healthy/running, migrations succeed, and logs contain
no secrets or credential errors.

- [ ] **Step 2: Verify migrated state through the real browser path**

Sign in as the owner and verify:

- profile field values;
- at least one known rule score;
- at least one known AI score and visible delta chips;
- one bookmark and one hidden job; and
- masked credential state without plaintext.

Follow at least one job link and confirm the destination identifies the expected
posting, not merely an HTTP success.

- [ ] **Step 3: Run one approved paid AI rating**

Choose one known posting, start rerating through the UI, wait for the terminal
event, and verify the new AI delta and user-scoped usage debit in the app. Do
not expose provider response bodies.

- [ ] **Step 4: Prove container-recreation durability**

```sh
docker compose --env-file .env up -d --force-recreate app
docker compose --env-file .env ps
```

Sign in again and repeat the profile, score, bookmark, hidden state, masked key,
and AI-rating checks. This proves RDS plus the separate master key are the
durability boundary; no app filesystem volume is involved.

- [ ] **Step 5: Prove two-session isolation outside production**

Against a disposable PostgreSQL database, seed two synthetic users and use two
independent browser cookie jars. Give each user a distinct synthetic profile
and encrypted fake provider credential, then verify each session sees only its
own profile, scores, usage, masked-key state, bookmarks, and hidden jobs. This
journey must not run against RDS and must not use a paid provider.

### Task 8: Refresh screenshots and public documentation

**Files:**

- Modify: `README.md`
- Modify: `README.ko.md`
- Modify: `docs/assets/screenshots/dashboard.png`
- Modify: `docs/assets/screenshots/dashboard-dark.png`
- Modify: `deploy/production/HUMAN_DEPLOY_GUIDE.md`
- Modify: `deploy/production/README.md`

- [ ] **Step 1: Prepare sanitized screenshot data**

Use non-sensitive posting/profile content. Ensure the dashboard is sorted by
score and at least one visible result contains applied AI delta details. Capture
the same viewport and state in light and dark themes.

- [ ] **Step 2: Capture and inspect both images with `/browse`**

Verify at desktop and mobile widths before selecting the README desktop assets.
Check clipping, score order, active sort, chip legibility, themes, and console
errors. Do not retain authentication cookies or raw production captures inside
the repository.

- [ ] **Step 3: Make English and Korean docs final-state accurate**

Remove every statement that describes SQLite as a normal writable database or
filesystem storage as production credential persistence. Document PostgreSQL,
automatic local bootstrap, encrypted per-user credentials, master-key recovery,
and the one-time importer.

- [ ] **Step 4: Run asset and documentation checks**

```sh
file docs/assets/screenshots/dashboard.png \
  docs/assets/screenshots/dashboard-dark.png
rg -n 'SQLite|sqlite|jobcron_config|ai_keys\.json|PostgreSQL|credential' \
  README.md README.ko.md deploy/production
git diff --check
```

Remaining SQLite/key-file references must describe the retained migration
source or rollback window only.

- [ ] **Step 5: Commit the sanitized final documentation**

```sh
git add README.md README.ko.md docs/assets/screenshots/dashboard.png \
  docs/assets/screenshots/dashboard-dark.png \
  deploy/production/HUMAN_DEPLOY_GUIDE.md deploy/production/README.md
git diff --cached --check
git diff --cached
gitleaks git --staged --redact --no-banner
git commit -m "docs: publish verified postgres deployment state"
```

### Task 9: Run the final security and completion gate

**Files:** All changed files in Slices 1 through 5.

- [ ] **Step 1: Run all automated gates on the deployed commit**

```sh
test -z "$(gofmt -l .)"
go vet ./...
go build ./...
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' go test ./... -count=1
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' \
  go test -race ./... -count=1
goreleaser check
sh scripts/verify-production-compose.sh
```

- [ ] **Step 2: Review the cumulative public diff**

```sh
git diff <slice-1-base-commit>..HEAD -- . \
  ':(exclude)docs/superpowers/archive/**'
```

Manually review for credentials, personal data, production identifiers, raw
logs, source paths, and misleading deployment claims.

- [ ] **Step 3: Scan tracked content**

```sh
gitleaks git --redact
git status --short
```

Investigate findings; do not suppress them merely to pass.

- [ ] **Step 4: Record sanitized verification evidence**

Create the completion report required by the documentation lifecycle, archive
completed Slice plans/specification, and update both documentation indexes.
Keep exact RDS, EC2, registry, account, snapshot, and secret evidence only in
the access-controlled operator log.

- [ ] **Step 5: Close the rollback window only on human approval**

Until approval, retain the original SQLite snapshot, legacy local key file,
credential master-key backup, and pre-import RDS snapshot. When approved, remove
the plaintext legacy key through the operator's secure process; the importer
must never claim secure erasure.

## Rollback Boundary

Before first app start, restore the pre-import RDS snapshot if import state is
wrong. After the app accepts PostgreSQL-only writes, do not return to SQLite.
Roll back the image to a schema-compatible commit while keeping PostgreSQL, or
restore an approved RDS snapshot and replay explicitly approved changes. Loss
of EC2 with surviving RDS requires restoration of the separately protected
master key; without it, users must replace their stored credentials.
