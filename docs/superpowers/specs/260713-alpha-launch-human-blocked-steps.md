# Jobcron Alpha Launch: Human-Blocked Steps

**Status:** Superseded; retained temporarily for the human progress record<br>
**Owner:** Human operator<br>
**Implementation prerequisite:** [Verified alpha pre-launch fixes](../archive/2026-07-13-alpha-pre-launch-fixes/260713-alpha-pre-launch-fixes-verification.md)

> **Convergence note (2026-07-15):** The checked boxes and `OF` annotations
> below record the human's launch preparation feedback. Do not execute this
> checklist as the current deployment guide. The approved
> [PostgreSQL convergence specification](260714-postgresql-local-convergence-user-ai-credentials.md)
> and [Slice 5 plan](../plans/260715-postgresql-convergence-slice-5-first-production-deployment.md)
> supersede its credential-volume, import, and rollback instructions. Final
> production has no `jobcron_config` volume, and reverting Git alone is not a
> safe rollback after PostgreSQL accepts newer writes.

## Purpose

This is the production launch checklist for actions an agent cannot safely finish
without the human's accounts, secrets, billing authority, identity choices, or
approval to change external state. It begins only after the repository work in
the linked implementation spec is complete, reviewed, committed, and pushed.

Do not enter real values into this tracked file. Angle-bracket values are
placeholders to replace only in a private terminal, password manager, AWS console,
or other access-controlled operational source.

## Why These Steps Are Human-Blocked

- **AWS EC2, RDS, and security groups:** Require cloud credentials, billing
  authority, and permission to change network exposure.
- **DNS:** Changes the public production route and may affect an existing
  domain.
- **Docker Hub publication:** Requires registry credentials and publishes a
  release artifact externally.
- **Production `.env`:** Contains the database connection string and session
  secret.
- **Owner account:** Requires the human to choose the real owner identity and
  password.
- **Recovered-data import:** Can overwrite production profile and user-scoped
  state, so it needs explicit approval.
- **Anthropic API key:** Is a paid credential that its owner must enter
  directly.
- **Go-live and rollback:** Change public availability and may require restoring
  production data or DNS.

## Secret and Publication Rules

- Never paste a real API key, password, `DATABASE_URL`, session secret, RDS
  endpoint, EC2 address, owner email, backup identifier, or recovery path into
  this file, Git, an issue, a PR, chat, or shared logs.
- Store real values in a password manager or private shell session. Clear shell
  variables when the step finishes.
- Do not make RDS public. The database stays behind its security group, and local
  operator commands use an SSH tunnel through EC2.
- Do not use `docker compose down -v`; it deletes the volume that holds
  `ai_keys.json`.
- Do not send terminal output containing secrets to an agent. Redact first.

## Human Inputs to Prepare Privately

- **Approved release commit SHA** — Not secret; used for the immutable image
  tag and audit trail.
- **Docker Hub repository** — Not secret; used for image publication and EC2
  pull.
- **Docker Hub credentials** — Secret; used by `docker login`.
- **EC2 host/address and SSH key** — Sensitive; used for deployment and the RDS
  tunnel.
- **RDS endpoint, database user, and password** — Secret; used in the
  production `DATABASE_URL`.
- **New `SESSION_SECRET`** — Secret; protects signed-in session integrity.
- **Owner email and password** — Secret; used for the first production login.
- **Recovered SQLite path** — Sensitive; used for the optional local-to-RDS
  import.
- **Anthropic API key** — Secret; used for optional BYOK scoring.
- **Backup/snapshot identifiers** — Sensitive; retained as rollback evidence.

## Preconditions

- [x] The alpha pre-launch implementation spec is complete and its commit is on
      the intended GitHub branch.
- [x] CI and the full local regression suite pass on that exact commit.
- [ ] Production Compose has the explicit `jobcron_config` volume mounted at
      `/root/.config/jobcron`.
- [ ] The human deploy guide contains the verified SSH-tunnel owner/import path.
- [x] The selected release commit contains no real credentials or private
      operational data.
- [x] The human has decided whether to import the recovered SQLite data.
  - **OF** import it.
- [x] A rollback window is available and no other operator is deploying.
  - **OF** rollback can be done by reverting back to a previous commit. No other operator is deploying.

Stop if any precondition is false.

## Phase 1: Verify AWS State and Backups

In the AWS console, verify and record the results privately:

- [x] The EC2 instance is running, uses the expected ARM64 architecture, has
      enough disk space, and retains a stable public address for DNS.
- [x] RDS is available on PostgreSQL 18, is not publicly accessible, and is
      reachable from the EC2 security group on TCP `5432`.
- [ ] EC2 inbound rules allow SSH `22` only from the operator's current IP and
      allow public HTTP `80` and HTTPS `443`.
- [ ] EC2 can reach RDS and Docker Hub outbound.
- [ ] RDS automated backups have a deliberate retention period.
- [ ] If data import is planned, create a manual RDS snapshot immediately before
      import and store its identifier privately.
- [ ] EC2 storage is encrypted. Decide whether host-level backup is required for
      the Docker volume or whether Anthropic key re-entry after host loss is the
      accepted recovery path.

Do not continue if RDS is public, PostgreSQL is open to the internet, or no
rollback point exists for an approved import.

## Phase 2: Verify DNS

At the DNS provider:

- [ ] The root `jobcron.app` A record targets the stable EC2 public address.
- [ ] `www.jobcron.app` is a CNAME to `jobcron.app`.
- [ ] Cloudflare proxying remains off for the first launch so Caddy can obtain and
      renew certificates directly.
- [ ] The operator has recorded the previous DNS values privately for rollback.

Do not put the EC2 address into this tracked checklist.

## Phase 3: Build and Publish an Immutable Image

From the approved local repository checkout:

```sh
git status --short
git rev-parse HEAD

RELEASE_TAG="sha-$(git rev-parse --short=12 HEAD)"
IMAGE="<dockerhub-user>/jobcron:${RELEASE_TAG}"

docker login
docker buildx build \
  --platform linux/arm64 \
  -f deploy/production/Dockerfile \
  -t "$IMAGE" \
  --push .
docker buildx imagetools inspect "$IMAGE"
```

- [ ] `git status --short` is empty before the build.
- [ ] The full SHA is the reviewed release commit.
- [ ] The pushed manifest contains `linux/arm64`.
- [ ] Record the immutable image name privately and use that exact value in
      production. Do not substitute `latest` or an assumed version tag.

This step publishes externally. Stop if the commit or image destination is not
the intended one.

## Phase 4: Prepare EC2 Deployment Files and Secrets

SSH to EC2. Update the existing deployment-file checkout with a fast-forward-only
pull, or copy the reviewed `deploy/production/` directory through the established
operator channel. The app itself still runs from the Docker image; it is not built
on EC2.

In `/srv/jobcron/deploy/production`:

```sh
cp .env.example .env
chmod 600 .env
openssl rand -base64 48
```

Privately edit `.env` with exactly:

```text
JOBCRON_IMAGE=<immutable-image-from-phase-3>
DATABASE_URL=<private-rds-postgresql-url-with-sslmode-require>
SESSION_SECRET=<new-random-secret>
```

- [ ] `.env` is owned by the deployment user and has mode `0600`.
- [ ] `JOBCRON_DEMO`, `JOBCRON_ADMIN_TOKEN`, `JOBCRON_PROXY_SECRET`, and
      `JOBCRON_WORKNET_KEY` are absent.
- [ ] The daily schedule remains the approved `05:00` KST value unless the human
      deliberately changes the product schedule.
- [ ] Validate without printing the rendered secrets:

  ```sh
  docker compose --env-file .env config --quiet
  ```

Do not paste `.env` or rendered Compose output into chat or a ticket.

## Phase 5: Pull, Start, and Apply Migrations

On EC2:

```sh
cd /srv/jobcron/deploy/production
docker compose --env-file .env pull
docker compose --env-file .env up -d
docker compose --env-file .env ps
docker compose --env-file .env logs --tail 200 app
```

- [ ] The app image matches the immutable tag from Phase 3.
- [ ] The app starts without a PostgreSQL or migration error.
- [ ] Caddy starts and can reach `app:7777` on the private Docker network.
- [ ] `docker volume inspect jobcron_config` succeeds.
- [ ] Review logs locally for errors, but do not publish raw production logs.

The app must start successfully once before owner creation or import because app
startup applies database migrations.

## Phase 6: Create the Owner Through a Private Tunnel

On the operator's Mac, open a dedicated terminal and keep it running:

```sh
ssh -o ExitOnForwardFailure=yes -N \
  -L 127.0.0.1:15432:<rds-endpoint>:5432 \
  ec2-user@<ec2-public-host>
```

In a second terminal, inside the approved source checkout, create private shell
variables. The tunneled URL uses `127.0.0.1:15432` and includes
`sslmode=require`:

```sh
export TUNNELED_DATABASE_URL='<private-tunneled-postgresql-url>'
export OWNER_EMAIL='<owner-email>'

go run ./cmd/jobcron-user create-owner \
  --database-url "$TUNNELED_DATABASE_URL" \
  --email "$OWNER_EMAIL"
```

Let `jobcron-user` prompt for the password. Do not place the password in the
command line or shell history.

- [ ] The command reports one owner created for the intended email.
- [ ] Do not repeat `create-owner` after success. Use the documented
      `reset-password` command if a password reset is later required.

## Phase 7: Optional Recovered-Data Import

Skip this phase if the human decided to start with an empty production database.

Before import:

- [ ] The manual RDS snapshot from Phase 1 exists.
- [ ] The owner from Phase 6 exists.
- [ ] `OWNER_EMAIL` is exactly the same email used in Phase 6.
- [ ] The recovered SQLite file is readable locally and is not being modified by
      a running local app.

Run the dry-run first:

```sh
go run ./cmd/jobcron-import \
  --sqlite '<recovered-sqlite-path>' \
  --postgres "$TUNNELED_DATABASE_URL" \
  --owner-email "$OWNER_EMAIL" \
  --dry-run
```

Review and privately record the reported counts for profile, postings, scores,
bookmarks, not-interested state, AI extractions, AI scores, and AI usage.

Stop if any count is surprising. If the counts are approved, run the same command
without `--dry-run`:

```sh
go run ./cmd/jobcron-import \
  --sqlite '<recovered-sqlite-path>' \
  --postgres "$TUNNELED_DATABASE_URL" \
  --owner-email "$OWNER_EMAIL"
```

- [ ] The transaction completes without error.
- [ ] The importer does not copy `ai_keys.json`, sessions, owner passwords, or
      production secrets.
- [ ] Close the SSH tunnel after owner/import work and clear private variables:

  ```sh
  unset TUNNELED_DATABASE_URL OWNER_EMAIL
  ```

## Phase 8: Enter the Anthropic Key and Prove Durability

In a real browser, sign in to `https://jobcron.app` and enter the Anthropic key
through the profile UI. Do not copy the key through SSH, Compose, logs, or chat.

After the UI confirms the key was saved, recreate only the app container on EC2:

```sh
cd /srv/jobcron/deploy/production
docker compose --env-file .env up -d --force-recreate app
docker compose --env-file .env ps
```

- [ ] Sign in again and confirm the app still reports Anthropic as configured.
- [ ] Trigger only the minimum approved AI action needed to prove the saved key
      works after recreation.
- [ ] Do not print or inspect the key file contents.

This proves same-host container durability. It does not prove recovery after EC2
host loss.

## Phase 9: Walk the Production User Path

Use a real browser, not HTTP-only probes:

- [ ] `http://jobcron.app/` redirects to HTTPS.
- [ ] `https://www.jobcron.app/` redirects to `https://jobcron.app/`.
- [ ] The certificate is valid for both hostnames.
- [ ] Unauthenticated access reaches login rather than private data.
- [ ] The owner can log in and log out.
- [ ] Profile data is correct, whether newly entered or imported.
- [ ] 전체 공고, 오늘의 브리핑, 북마크, and 숨긴 공고 load correctly.
- [ ] If data was imported, sample at least one posting, score, bookmark, hidden
      item, and AI result against the local source data.
- [ ] Run one manual scrape only if the human approves its network and Anthropic
      cost; otherwise verify the `05:00` KST schedule from the app and logs.
- [ ] No browser console error appears on the walked pages.
- [ ] `demo.jobcron.app` remains unchanged and reachable independently.

## Phase 10: Record Launch Evidence Privately

Record these items in an access-controlled operational note, not this public spec:

- Release commit SHA and immutable image tag
- Deployment time and operator
- RDS snapshot or backup evidence
- EC2 and RDS health confirmation
- DNS and certificate confirmation
- Owner creation and optional import outcome
- Source and target import counts, if applicable
- API-key durability result without the key value
- Browser-flow result and any approved exceptions
- Previous image tag and DNS values for rollback

## Rollback

### Application-only rollback

1. Set `JOBCRON_IMAGE` in the private EC2 `.env` to the previous immutable tag.
2. Run `docker compose --env-file .env pull` and
   `docker compose --env-file .env up -d`.
3. Keep `jobcron_config` and RDS intact. Do not use `down -v`.
4. Repeat the production browser smoke path.

### Import rollback

If import wrote incorrect data, stop public use and restore the pre-import RDS
snapshot to a safe replacement instance according to the AWS recovery plan. Do
not improvise destructive SQL cleanup. Update the private `DATABASE_URL`, restart
the app, and re-run verification.

### DNS rollback

Restore the privately recorded previous DNS values and wait for DNS propagation.
Do not delete the current EC2/RDS state until rollback is verified.

### Key recovery

If the Docker volume is lost, revoke the lost key if exposure is possible, create
a replacement key, and re-enter it through the UI. Do not reconstruct it from
logs, shell history, or Git.

## Human Definition of Done

The alpha launch is complete only when every applicable checkbox above is
verified, skipped items have a written private rationale, rollback evidence
exists, no secret has entered a public artifact, `jobcron.app` passes the real
signed-in user path, and `demo.jobcron.app` remains unaffected.
