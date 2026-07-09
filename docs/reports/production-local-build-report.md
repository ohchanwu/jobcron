# Production Local Build Report

Date: 2026-07-10 KST

## Scope

This report covers the local state after the production-app Task 1-11 line and Task 12 verification. I did not start AWS-dependent work. RDS, production secrets, DNS, backup policy, and production Anthropic ownership still need human input before deployment continues.

## Local Commits Completed

The local integrated baseline used for this report is `02c314e` from `/Users/chanbla11mit/gt/jobscraper/refinery/rig`, as requested by `jobs-e2a`.

Recent production-app commits present in that integrated line:

- `02c314e` / `b51e9c4` - manual AI re-rate controls and opt-in scheduled AI controls.
- `b519655` - daily scrape scheduler.
- `97d5fff`, `0527155`, `d2b3e8b` - scrape run history and PostgreSQL migration/runtime coverage.
- `27ed9db`, `a9cd79c`, `29fd1bd`, `4d53253` - CSRF, login rate limiting, and proxy-bound hardening.
- `8ba8e08`, `54629ee`, `cf5616c` - owner login sessions, logout/API auth hardening, and owner-profile AI reconfiguration.
- `ca7d432`, `86dc414` - owner account CLI and email-safe password reset.
- `b6ad137`, `62c111d`, `67f1ee5` - SQLite-to-PostgreSQL importer and deterministic owner/import behavior.
- `5d5ab93`, `64ac0e7`, `f9cb173`, `3e952fd` - storage dialect foundation, PostgreSQL runtime queries, isolated PostgreSQL integration tests, and migration checks.
- `15822e7`, `3399092`, `922986f`, `a3b5390` - production config loading and production-app planning/docs.

Task 12 adds this report only.

## Local Verification

Commands run from the `jobs-e2a` branch:

```sh
gofmt -l .
go build ./cmd/job-scraper
go vet ./...
JOBSCRAPER_TEST_POSTGRES_URL='postgres://postgres@localhost:55432/jobscraper_dev?sslmode=disable' go test ./internal/storage -run Postgres -count=1
go test ./...
```

Results:

- `gofmt -l .` printed nothing.
- `go build ./cmd/job-scraper` passed.
- `go vet ./...` passed.
- PostgreSQL integration test passed: `ok github.com/ohchanwu/job-scraper/internal/storage`.
- Full Go test suite passed.

## Local PostgreSQL App Run

Local PostgreSQL was already running through Docker Compose:

```sh
docker compose -f deploy/local/compose.yaml ps
```

The app was run against a clean local PostgreSQL database:

```sh
docker exec local-postgres-1 psql -U postgres -c "DROP DATABASE IF EXISTS jobscraper_task12"
docker exec local-postgres-1 psql -U postgres -c "CREATE DATABASE jobscraper_task12"
DATABASE_URL='postgres://postgres@localhost:55432/jobscraper_task12?sslmode=disable' go run ./cmd/job-scraper --no-open
```

Browser verification used gstack browse against `http://127.0.0.1:7777`.

Observed result:

- `/` returned `303` and redirected to `/profile`.
- `/profile` returned `200`.
- Static assets returned `200`.
- Browser console had no messages after resetting the browse daemon.
- The first-run profile form rendered.

One local-data warning: the documented `jobscraper_dev` database currently returns `500` on `/` because it contains an older profile JSON shape:

```text
profile: unmarshal: json: cannot unmarshal string into Go struct field Profile.stacks of type profile.StackPref
```

The clean database path passed. Treat the `jobscraper_dev` failure as stale local data or import/migration follow-up before relying on that specific database.

## Legacy SQLite Status

Runtime production storage should use PostgreSQL through `DATABASE_URL`.

Legacy SQLite still exists for compatibility and local tooling in these places:

- Import/source compatibility: the SQLite-to-PostgreSQL importer still reads old local/demo SQLite databases so existing data can be migrated.
- Demo/local compatibility: developer/demo runs can still use a local SQLite file when `DATABASE_URL` is not set.
- Tests: SQLite remains in unit and compatibility tests where it is useful to exercise storage behavior without PostgreSQL.

I did not find or verify any production path that should intentionally use SQLite after `DATABASE_URL` is configured. Production deploy should treat a missing `DATABASE_URL` as a configuration error to catch accidental SQLite fallback before traffic reaches the app.

## Owner Account Command

After production `DATABASE_URL` is ready and migrations have run, create the owner account from a checked-out repo or build host with Go available:

```sh
export DATABASE_URL='postgres://<user>:<password>@<rds-endpoint>:5432/<database>?sslmode=require'
export JOBSCRAPER_OWNER_PASSWORD='<temporary-owner-password>'
go run ./cmd/job-scraper-user create-owner \
  --database-url "$DATABASE_URL" \
  --email '<owner-email@example.com>'
unset JOBSCRAPER_OWNER_PASSWORD
```

To rotate the password later:

```sh
export JOBSCRAPER_OWNER_PASSWORD='<new-owner-password>'
go run ./cmd/job-scraper-user reset-password \
  --database-url "$DATABASE_URL" \
  --email '<owner-email@example.com>'
unset JOBSCRAPER_OWNER_PASSWORD
```

Current deploy note: `deploy/aws/Dockerfile` only packages `/usr/local/bin/job-scraper`; it does not package `job-scraper-user`. Either run the command from a source checkout with Go installed, or update the deployment image before expecting this helper inside the container.

## Production Env Template

Use this as the starting production `.env` template. Values in angle brackets are human-provided.

Important deploy-file warning: the current `deploy/aws/compose.yaml` is still the demo-era shape. It runs the app in demo mode, mounts `/data/jobs.db`, and expects `JOBSCRAPER_DEMO` / `JOBSCRAPER_ADMIN_TOKEN`. Before production deployment, update the AWS compose/config to consume the PostgreSQL production variables below instead of the demo SQLite setup.

```sh
JOBSCRAPER_ENV=production
JOBSCRAPER_HOST=0.0.0.0
JOBSCRAPER_PORT=7777
JOBSCRAPER_NO_OPEN=1

DATABASE_URL=postgres://<user>:<password>@<rds-endpoint>:5432/<database>?sslmode=require
SESSION_SECRET=<base64-or-hex-secret-at-least-32-bytes>

JOBSCRAPER_SCHEDULER_ENABLED=1
JOBSCRAPER_DAILY_SCRAPE_TIME=09:00

# Optional source/API keys.
JOBSCRAPER_WORKNET_KEY=<optional-worknet-data-go-kr-key>

# Needed only if Caddy/reverse-proxy trusted client IP handling is used.
JOBSCRAPER_PROXY_SECRET=<random-proxy-secret>

# Keep unset for private production owner app unless intentionally running the read-only demo mode.
# JOBSCRAPER_DEMO=1
# JOBSCRAPER_ADMIN_TOKEN=<demo-admin-token>
```

Generate a session secret with:

```sh
openssl rand -base64 48
```

Do not put Anthropic keys in this `.env` unless the product decision is that the production operator owns the AI key. The current app supports bring-your-own-key AI settings through the profile flow.

## Backup Pull Plan

After RDS is created, define a MacBook backup destination before the first production scrape. The path still needs human confirmation.

Proposed local path:

```sh
/Users/chanbla11mit/backups/jobcron/rds/
```

Initial pull command shape:

```sh
mkdir -p /Users/chanbla11mit/backups/jobcron/rds
pg_dump "$DATABASE_URL" \
  --format=custom \
  --no-owner \
  --file "/Users/chanbla11mit/backups/jobcron/rds/jobcron-$(date +%Y%m%d-%H%M%S).dump"
```

First verification command:

```sh
pg_restore --list /Users/chanbla11mit/backups/jobcron/rds/<dump-file>.dump >/tmp/jobcron-backup-restore-list.txt
```

This only verifies that the dump is readable. A stronger first backup check is to restore into a disposable local PostgreSQL database and run the app against that restored database.

## DNS and Caddy Requirements

DNS:

- `jobcron.app` should resolve to the EC2 instance public DNS name or public IP using a DNS-only record while the instance does not use an Elastic IP.
- `www.jobcron.app` should point to the same target, usually with a CNAME when the DNS provider allows it.
- Cloudflare proxy should stay off unless Caddy is configured and verified behind Cloudflare; otherwise TLS/debugging is harder and client IP handling can be misleading.

Caddy:

- Caddy should terminate HTTPS for `jobcron.app` and `www.jobcron.app`.
- Caddy should reverse-proxy to the app container on `127.0.0.1:7777` or the Docker network service name, depending on the final compose layout.
- The app container should bind `JOBSCRAPER_HOST=0.0.0.0`; Caddy handles public HTTPS.
- If the app trusts proxy-provided client IPs, set and pass `JOBSCRAPER_PROXY_SECRET` consistently between Caddy and the app. If that proxy-secret integration is not enabled in the final Caddy config, leave client-IP trust disabled rather than accepting spoofable headers.

## Human Inputs Still Needed

- RDS PostgreSQL endpoint, database name, username, and password.
- Production `SESSION_SECRET`.
- Owner email and password setup method for the first account.
- Production domain/DNS confirmation for `jobcron.app` and `www.jobcron.app`.
- MacBook backup destination path, backup cadence, and whether the first restore test should be list-only or a disposable local restore.
- Decision: Anthropic key is operator-owned in production or only user-provided later.
- Decision or image update: where `job-scraper-user` will run after deploy.
