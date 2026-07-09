# Jobcron Production App Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the current read-only demo into a single-user production app with PostgreSQL, owner login, cookie sessions, server-side user state, scheduled daily scraping, manual/opt-in AI scoring, RDS deployment docs, and a human handoff report.

**Architecture:** Keep global job postings shared, but introduce owner/user identity now so Milestone B multi-user support does not require a second schema rewrite. Make PostgreSQL the normal database for production and local development by the end of Milestone A, with Docker Compose providing the local PostgreSQL service. Keep SQLite only as a legacy import source and temporary test compatibility layer while the app moves to PostgreSQL-first behavior.

- **Overseer feedback:** No, we want to be using PostgreSQL for local development as well once Milestone A is complete. The port should be total, unless you can come up with a good objection. We want to make the local development environment as similar to prod as is feasible/advisable/prudent.
- **Overseer feedback response:** Accepted. The revised plan makes PostgreSQL the default local development database at the end of Milestone A. SQLite is retained only where it has a concrete transitional purpose: importing the current `jobs.db` data and keeping narrow compatibility tests until their PostgreSQL equivalents exist.

**Tech Stack:** Go `net/http`, `database/sql`, PostgreSQL via `github.com/jackc/pgx/v5/stdlib`, local PostgreSQL via Docker Compose, SQLite via `modernc.org/sqlite` only for legacy import/test compatibility, password hashing via `golang.org/x/crypto/argon2`, existing embedded templates in `web/`, Docker Compose/Caddy/AWS RDS docs.

## Global Constraints

- Keep commits local until human review. Do not push.
- Make small local commits at meaningful task boundaries.
- Preserve `demo.jobcron.app` behavior for the existing public demo.
- Production target is `jobcron.app` and `www.jobcron.app`.
- Milestone A is single-user production with one owner account.
- Milestone B is public signup and multi-user accounts, but do not implement public signup in this plan.
- Use AWS RDS PostgreSQL for production.
- Use local PostgreSQL for normal development by the end of Milestone A.
- Keep SQLite available only for legacy `jobs.db` import and transitional test coverage.
- Do not upload or commit `ai_keys.json`, `.env`, `jobs.db`, or production secrets.
- Owner account creation is a CLI command, not a first-run browser page.
- Production user state must not use localStorage as source of truth.
- Scheduled scrape must not run fresh AI by default.
- Manual AI run must be available to the logged-in owner.
- Scheduled AI for new postings is opt-in and must show a prominent tip while disabled.
- Default AI budget caps: estimated USD $10/month, USD $0.50/day, and about USD $0.30 per automated run.
- If an AI cap is hit, show a user-facing notification linking to settings.
- Backup design must tolerate the MacBook being closed by using server retention plus MacBook pull sync.
- Do not start implementation until this plan is reviewed.

---

## Execution Workflow

1. Implement tasks locally in order.
2. Commit each task locally.
3. After Task 12, write `docs/reports/production-local-build-report.md` with:
   - what was built locally,
   - test evidence,
   - exact RDS/secrets/DNS/backup actions needed from the human,
   - exact commands for the human to run,
   - what remains blocked until those actions are done.
4. Stop and wait for the human to perform the required AWS/RDS/secrets work.
5. Resume with Task 13 after the human reports the external work is complete.
6. Keep all commits local until the human asks to push.

---

## Planned File Structure

- Create `internal/config/config.go`: parse production/runtime configuration from flags and environment.
- Create `internal/storage/dialect.go`: database dialect enum and SQL placeholder helpers.
- Modify `internal/storage/store.go`: open SQLite or PostgreSQL and run dialect-specific migrations.
- Create `internal/storage/postgres_migrations/*.sql`: PostgreSQL schema.
- Create `deploy/local/compose.yaml`: local PostgreSQL service for day-to-day development.
- Create `deploy/local/README.md`: local PostgreSQL setup, reset, migrate, and import workflow.
- Create `cmd/job-scraper-import/main.go`: one-time SQLite to PostgreSQL importer.
- Create `internal/auth/password.go`: Argon2id password hashing and verification.
- Create `internal/auth/session.go`: secure session token generation and hashing helpers.
- Create `internal/storage/users.go`: owner user persistence.
- Create `internal/storage/sessions.go`: session persistence.
- Create `cmd/job-scraper-user/main.go`: CLI owner account creation and password reset.
- Create `internal/server/auth.go`: login/logout/session middleware.
- Modify `internal/server/handlers.go`: add auth routes and protect production write routes.
- Modify `internal/storage/profile.go`, `bookmarks.go`, `not_interested.go`, `scores.go`: add owner/user-scoped operations.
- Modify `internal/server/archive.go`, `bookmarks.go`, `hidden.go`, `server.go`: use current user ID for state.
- Create `internal/server/scheduler.go`: daily scrape scheduler.
- Create `internal/storage/scrape_runs.go`: scrape run history.
- Create `internal/server/admin.go`: owner-only status page or JSON endpoint.
- Modify `internal/server/rerate.go`: manual AI run and cap-hit notification behavior.
- Modify `web/*.html`, `web/*.js`, `web/styles.css`: login, server-backed state, status/tips.
- Modify `deploy/aws/Caddyfile`, `deploy/aws/compose.yaml`, `deploy/aws/README.md`, `deploy/aws/HUMAN_DEPLOY_GUIDE.md`: production hostnames, RDS, env, backup docs.
- Create `deploy/aws/backup-pull-macbook.md`: MacBook pull-based backup runbook.
- Create `docs/reports/production-local-build-report.md`: handoff report after local work.

---

## Task 1: Production Configuration Foundation

**Files:**

- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Modify: `cmd/job-scraper/main.go`

**Interfaces:**

- Produces: `config.Load(args []string, env map[string]string) (config.Config, error)`
- Produces: `Config.DatabaseURL string`
- Produces: `Config.SessionSecret []byte`
- Produces: `Config.Production bool`
- Produces: `Config.SchedulerEnabled bool`
- Produces: `Config.DailyScrapeTime string`
- Consumes: existing flags `--port`, `--host`, `--no-open`, `--demo`, `--db`, `--worknet-api-key`

- [ ] **Step 1: Write failing config tests**

Add tests that lock these behaviors:

```go
func TestLoadProductionRequiresDatabaseURL(t *testing.T) {
	env := map[string]string{"JOBSCRAPER_ENV": "production", "SESSION_SECRET": strings.Repeat("a", 32)}
	_, err := config.Load(nil, env)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("Load error = %v, want DATABASE_URL requirement", err)
	}
}

func TestLoadProductionRequiresSessionSecret(t *testing.T) {
	env := map[string]string{"JOBSCRAPER_ENV": "production", "DATABASE_URL": "postgres://db.example.invalid/jobs"}
	_, err := config.Load(nil, env)
	if err == nil || !strings.Contains(err.Error(), "SESSION_SECRET") {
		t.Fatalf("Load error = %v, want SESSION_SECRET requirement", err)
	}
}

func TestLoadDefaultsSchedulerOffOutsideProduction(t *testing.T) {
	cfg, err := config.Load(nil, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Production {
		t.Fatal("Production = true, want false")
	}
	if cfg.SchedulerEnabled {
		t.Fatal("SchedulerEnabled = true, want false")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config`

Expected: package or symbols missing.

- [ ] **Step 3: Implement `internal/config`**

Implement environment parsing with explicit defaults:

```go
type Config struct {
	Production        bool
	DatabaseURL       string
	SessionSecret     []byte
	SchedulerEnabled  bool
	DailyScrapeTime   string
	AdminToken        string
	WorknetKey        string
	Host              string
	Port              int
	NoOpen            bool
	Demo              bool
	DBPath            string
}
```

Rules:

- `JOBSCRAPER_ENV=production` enables production mode.
- Production requires `DATABASE_URL`.
- Production requires `SESSION_SECRET` with at least 32 bytes.
- `JOBSCRAPER_SCHEDULER_ENABLED=1` enables scheduler.
- Default scrape time is `08:00`.
- CLI flags override equivalent environment defaults where flags already exist.

- [ ] **Step 4: Wire `cmd/job-scraper/main.go` to `config.Load`**

Keep behavior identical in local mode. In production, open storage from `DATABASE_URL` instead of `--db`.

- [ ] **Step 5: Verify**

Run:

```bash
go test ./internal/config ./cmd/job-scraper
go test ./...
```

Expected: all pass.

- [ ] **Step 6: Commit locally**

```bash
git add internal/config cmd/job-scraper/main.go
git commit -m "feat: add production config loading"
```

---

## Task 2: Storage Dialect and PostgreSQL Migration Runner

**Files:**

- Create: `internal/storage/dialect.go`
- Create: `internal/storage/postgres_migrations/0001_initial.sql`
- Create: `internal/storage/postgres_migrations/0002_user_state.sql`
- Create: `deploy/local/compose.yaml`
- Create: `deploy/local/README.md`
- Modify: `internal/storage/store.go`
- Modify: `go.mod`, `go.sum`
- Test: `internal/storage/store_test.go`

**Interfaces:**

- Produces: `storage.OpenPostgres(databaseURL string) (*Store, error)`
- Produces: `storage.OpenSQLiteAt(path string) (*Store, error)`
- Produces: `Store.Dialect() storage.Dialect`
- Keeps: `storage.Open()` and `storage.OpenAt(path)` compatibility for legacy SQLite tests and import only.
- Produces: local PostgreSQL development service at `deploy/local/compose.yaml`.

- [ ] **Step 1: Add failing tests for dialect selection**

Add:

```go
func TestOpenAtUsesSQLiteDialect(t *testing.T) {
	st := newTestStore(t)
	if st.Dialect() != storage.DialectSQLite {
		t.Fatalf("Dialect = %v, want sqlite", st.Dialect())
	}
}
```

- [ ] **Step 2: Add PostgreSQL dependency**

Run:

```bash
go get github.com/jackc/pgx/v5/stdlib
```

- [ ] **Step 3: Implement dialect enum**

```go
type Dialect string

const (
	DialectSQLite   Dialect = "sqlite"
	DialectPostgres Dialect = "postgres"
)
```

Add SQL placeholder helper:

```go
func (d Dialect) Placeholder(n int) string {
	if d == DialectPostgres {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}
```

- [ ] **Step 4: Split open functions**

Keep `OpenAt(path)` as SQLite for transitional callers, but mark new app code to use `OpenPostgres(databaseURL)` when a `DATABASE_URL` is configured. Do not add new feature work that depends on SQLite.

- [ ] **Step 5: Implement PostgreSQL migration runner**

Use a `schema_migrations(version integer primary key, applied_at timestamptz not null)` table, not SQLite `PRAGMA user_version`.

- [ ] **Step 6: Create PostgreSQL initial schema**

Port the current SQLite schema to PostgreSQL:

- `INTEGER PRIMARY KEY AUTOINCREMENT` becomes `BIGSERIAL PRIMARY KEY`.
- `DATETIME` becomes `TIMESTAMPTZ`.
- JSON text columns can stay `TEXT` for first migration.
- SQLite full-text search does not port directly. For Milestone A, add PostgreSQL search using `to_tsvector`/`plainto_tsquery` if the current app has visible search behavior; otherwise document search as unchanged only if no user-facing search exists.

- [ ] **Step 7: Add local PostgreSQL development Compose service**

Create `deploy/local/compose.yaml`:

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_HOST_AUTH_METHOD: trust
      POSTGRES_DB: jobscraper_dev
    ports:
      - "55432:5432"
    volumes:
      - jobscraper-postgres-data:/var/lib/postgresql/data

volumes:
  jobscraper-postgres-data:
```

Create `deploy/local/README.md` with:

- start: `docker compose -f deploy/local/compose.yaml up -d`,
- local `DATABASE_URL`: `postgres://postgres@localhost:55432/jobscraper_dev?sslmode=disable`,
- reset command using `docker compose -f deploy/local/compose.yaml down -v`,
- import command from the old `jobs.db`.

- [ ] **Step 8: Verify PostgreSQL-first storage and legacy SQLite tests**

Run:

```bash
go test ./internal/storage
docker compose -f deploy/local/compose.yaml up -d
JOBSCRAPER_TEST_POSTGRES_URL='postgres://postgres@localhost:55432/jobscraper_dev?sslmode=disable' go test ./internal/storage -run Postgres -count=1
go test ./...
```

Expected: all pass.

- [ ] **Step 9: Commit locally**

```bash
git add go.mod go.sum internal/storage deploy/local
git commit -m "feat: add storage dialect foundation"
```

---

## Task 3: PostgreSQL Local Development and Integration Test Harness

**Files:**

- Create: `internal/storage/postgres_integration_test.go`
- Modify: `cmd/job-scraper/main.go`
- Modify: `README.md` or existing local run docs if present.
- Modify: `docs/plans/production-app-roadmap.md` only if a real plan correction is discovered.

**Interfaces:**

- Consumes: `JOBSCRAPER_TEST_POSTGRES_URL`
- Produces: integration tests skipped unless `JOBSCRAPER_TEST_POSTGRES_URL` is set.
- Produces: local app startup path that uses PostgreSQL when `DATABASE_URL` is set.

- [ ] **Step 1: Write integration test**

Add a test that opens PostgreSQL, applies migrations, and verifies core tables exist.

```go
func TestPostgresMigrationsCreateCoreTables(t *testing.T) {
	url := os.Getenv("JOBSCRAPER_TEST_POSTGRES_URL")
	if url == "" {
		t.Skip("JOBSCRAPER_TEST_POSTGRES_URL not set")
	}
	st, err := storage.OpenPostgres(url)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for _, table := range []string{"users", "sessions", "postings", "profiles", "bookmarks", "not_interested", "scores", "scrape_runs"} {
		var exists bool
		err := st.DBForTest().QueryRowContext(context.Background(), `SELECT EXISTS (
			SELECT 1 FROM information_schema.tables WHERE table_name = $1
		)`, table).Scan(&exists)
		if err != nil || !exists {
			t.Fatalf("table %s exists=%v err=%v", table, exists, err)
		}
	}
}
```

If needed, add `DBForTest()` behind a test-only file.

- [ ] **Step 2: Verify skip behavior**

Run:

```bash
go test ./internal/storage -run Postgres
```

Expected: skipped when env var absent.

- [ ] **Step 3: Verify with local Docker PostgreSQL**

Run:

```bash
docker compose -f deploy/local/compose.yaml up -d
JOBSCRAPER_TEST_POSTGRES_URL='postgres://postgres@localhost:55432/jobscraper_dev?sslmode=disable' go test ./internal/storage -run Postgres -count=1
```

Expected: test passes.

- [ ] **Step 4: Make PostgreSQL the documented local development path**

Update startup wiring so:

- `DATABASE_URL` opens PostgreSQL,
- absence of `DATABASE_URL` may still use SQLite only for compatibility/demo commands,
- local development docs instruct developers to start PostgreSQL and run the app with `DATABASE_URL`.

Run:

```bash
docker compose -f deploy/local/compose.yaml up -d
DATABASE_URL='postgres://postgres@localhost:55432/jobscraper_dev?sslmode=disable' ./job-scraper --no-open --port 7777
```

Expected: app starts against PostgreSQL.

- [ ] **Step 5: Commit locally**

```bash
git add internal/storage/postgres_integration_test.go internal/storage cmd/job-scraper README.md deploy/local
git commit -m "test: add postgres migration integration check"
```

---

## Task 4: SQLite to PostgreSQL Importer

**Files:**

- Create: `cmd/job-scraper-import/main.go`
- Create: `cmd/job-scraper-import/main_test.go`
- Modify: `internal/storage/store.go` only if export helpers are required.

**Interfaces:**

- Produces command:
  - `job-scraper-import --sqlite /path/jobs.db --postgres "$DATABASE_URL" --dry-run`
  - `job-scraper-import --sqlite /path/jobs.db --postgres "$DATABASE_URL"`

- [ ] **Step 1: Write importer tests**

Add two tests:

- a dry-run test using a temporary SQLite store with sample data that asserts the importer reports table counts without writing to PostgreSQL,
- a PostgreSQL integration test, gated by `JOBSCRAPER_TEST_POSTGRES_URL`, that imports sample SQLite data into PostgreSQL and verifies representative postings, user state, and scores exist in PostgreSQL.

- [ ] **Step 2: Implement importer structure**

Implement ordered copy:

1. profiles/users bootstrap,
2. postings,
3. scores,
4. bookmarks,
5. not_interested,
6. ai_extractions,
7. ai_scores,
8. ai_usage.

For Milestone A, create one owner user during import if no owner exists, then attach single-row profile/bookmarks/hidden/scores to that user.

- [ ] **Step 3: Add conflict handling**

Use PostgreSQL `ON CONFLICT` for idempotent retries. Importing twice should not duplicate postings or state.

- [ ] **Step 4: Verify**

Run:

```bash
docker compose -f deploy/local/compose.yaml up -d
JOBSCRAPER_TEST_POSTGRES_URL='postgres://postgres@localhost:55432/jobscraper_dev?sslmode=disable' go test ./cmd/job-scraper-import ./internal/storage
go test ./...
```

- [ ] **Step 5: Commit locally**

```bash
git add cmd/job-scraper-import internal/storage
git commit -m "feat: add sqlite to postgres importer"
```

---

## Task 5: Owner User and CLI Account Creation

**Files:**

- Create: `internal/auth/password.go`
- Create: `internal/auth/password_test.go`
- Create: `internal/storage/users.go`
- Create: `internal/storage/users_test.go`
- Create: `cmd/job-scraper-user/main.go`
- Create: `cmd/job-scraper-user/main_test.go`
- Modify: `go.mod`, `go.sum`

**Interfaces:**

- Produces: `auth.HashPassword(password string) (string, error)`
- Produces: `auth.VerifyPassword(encodedHash, password string) (bool, error)`
- Produces: `Store.CreateOwnerUser(ctx, email, passwordHash string) (User, error)`
- Produces CLI:
  - `job-scraper-user create-owner --database-url "$DATABASE_URL" --email you@example.com`
  - password read from terminal prompt or `JOBSCRAPER_OWNER_PASSWORD` for scripted setup.

- [ ] **Step 1: Add Argon2id dependency**

Run:

```bash
go get golang.org/x/crypto/argon2
```

- [ ] **Step 2: Write password tests**

Test:

- hash verifies with original password,
- hash rejects wrong password,
- encoded hash includes parameters,
- malformed hash returns an error.

- [ ] **Step 3: Implement Argon2id hashing**

Parameters for first pass:

- memory: 64 MiB,
- iterations: 3,
- parallelism: 2,
- salt: 16 bytes,
- key length: 32 bytes.

If local memory profile is too high during tests, reduce memory to 32 MiB and document why in code.

- [ ] **Step 4: Implement user storage**

User fields:

```go
type User struct {
	ID           int64
	Email        string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
```

Milestone A should enforce at most one owner user.

- [ ] **Step 5: Implement CLI command**

The command should fail if an owner already exists unless `reset-password` is used.

- [ ] **Step 6: Verify**

Run:

```bash
go test ./internal/auth ./internal/storage ./cmd/job-scraper-user
go test ./...
```

- [ ] **Step 7: Commit locally**

```bash
git add go.mod go.sum internal/auth internal/storage/users.go internal/storage/users_test.go cmd/job-scraper-user
git commit -m "feat: add owner account cli"
```

---

## Task 6: Cookie Sessions and Login Flow

**Files:**

- Create: `internal/auth/session.go`
- Create: `internal/auth/session_test.go`
- Create: `internal/storage/sessions.go`
- Create: `internal/storage/sessions_test.go`
- Create: `internal/server/auth.go`
- Create: `internal/server/auth_test.go`
- Create: `web/login.html`
- Modify: `internal/server/handlers.go`
- Modify: `web/*.html` as needed for login/logout links.

**Interfaces:**

- Produces: `Store.CreateSession(ctx, userID int64, tokenHash string, expiresAt time.Time) error`
- Produces: `Store.UserBySessionToken(ctx, token string) (User, bool, error)`
- Produces routes:
  - `GET /login`
  - `POST /login`
  - `POST /logout`

- [ ] **Step 1: Write session token tests**

Test:

- generated tokens are at least 32 random bytes before encoding,
- only token hashes are stored,
- expired sessions are rejected.

- [ ] **Step 2: Implement session storage**

Store SHA-256 token hash, user ID, expiry, created time, last seen time.

- [ ] **Step 3: Add auth middleware**

Middleware rules:

- In production mode, protected pages redirect anonymous users to `/login`.
- In demo mode, current public behavior stays unchanged.
- Static assets and `/login` remain public.

- [ ] **Step 4: Add login/logout handlers**

Login failure must use generic error copy. Do not reveal whether email exists.

- [ ] **Step 5: Add secure cookie flags**

Cookie:

- `HttpOnly: true`
- `Secure: true` in production
- `SameSite: http.SameSiteLaxMode`
- path `/`

- [ ] **Step 6: Verify**

Run:

```bash
go test ./internal/auth ./internal/storage ./internal/server
go test ./...
```

- [ ] **Step 7: Commit locally**

```bash
git add internal/auth internal/storage/sessions.go internal/storage/sessions_test.go internal/server/auth.go internal/server/auth_test.go internal/server/handlers.go web/login.html web
git commit -m "feat: add owner login sessions"
```

---

## Task 7: User-Scoped Profile, Bookmarks, Hidden State, and Scores

**Files:**

- Modify: `internal/storage/profile.go`, `profile_test.go`
- Modify: `internal/storage/bookmarks.go`, `bookmarks_test.go`
- Modify: `internal/storage/not_interested.go`, `not_interested_test.go`
- Modify: `internal/storage/scores.go`, `scores_test.go`
- Modify: `internal/server/server.go`, `archive.go`, `bookmarks.go`, `hidden.go`, `handlers.go`
- Modify: `internal/server/*_test.go` that assumes global state.
- Modify: `web/bookmark.js`, `web/not-interested.js`, `web/*.html`

**Interfaces:**

- Produces user-scoped storage methods:
  - `SaveProfileForUser(ctx, userID, canonicalJSON string)`
  - `ProfileForUser(ctx, userID int64)`
  - `AddBookmark(ctx, userID, postingID int64)`
  - `NotInterestedIDs(ctx, userID int64)`
  - `ScoresByPostingID(ctx, userID int64)`

- [ ] **Step 1: Add failing user isolation tests**

For each state type:

- create two users,
- save different state for each,
- assert user A cannot see user B state.

- [ ] **Step 2: Add PostgreSQL user-scoped state migration**

Add a migration that:

- creates `users`,
- creates `sessions`,
- renames or rebuilds `profile` into `profiles`,
- adds `user_id` to bookmarks, not_interested, and scores,
- backfills imported single-user data to the owner user.

- [ ] **Step 3: Keep SQLite compatibility narrow**

Do not add broad new SQLite behavior. Keep only the compatibility needed for:

- reading the old `jobs.db` import source,
- existing unit tests that have not yet moved to PostgreSQL,
- demo mode if the current demo deployment still depends on SQLite.

Any compatibility wrapper must be documented as transitional.

- [ ] **Step 4: Update storage methods**

Make user-scoped methods primary. Keep compatibility wrappers only where tests or demo mode still need them.

- [ ] **Step 5: Update server state reads/writes**

All production state-changing handlers must use current authenticated user ID. Demo mode may continue localStorage behavior on `demo.jobcron.app`.

- [ ] **Step 6: Remove production localStorage source of truth**

Bookmark/hide JavaScript should call server endpoints when logged in. It may keep localStorage behavior only when the page has `data-demo="true"`.

- [ ] **Step 7: Verify**

Run:

```bash
go test ./internal/storage ./internal/server
go test ./...
```

- [ ] **Step 8: Commit locally**

```bash
git add internal/storage internal/server web
git commit -m "feat: scope user state to owner account"
```

---

## Task 8: CSRF Protection and Login Rate Limiting

**Files:**

- Create: `internal/server/csrf.go`
- Create: `internal/server/csrf_test.go`
- Create: `internal/server/rate_limit.go`
- Create: `internal/server/rate_limit_test.go`
- Modify: `internal/server/handlers.go`
- Modify: `web/*.html` forms.

**Interfaces:**

- Produces: CSRF token generation and validation for POST/PUT/DELETE form/API calls.
- Produces: in-memory login rate limiter keyed by IP and email.

- [ ] **Step 1: Write CSRF tests**

Test:

- missing token is rejected,
- wrong token is rejected,
- valid token passes,
- GET requests are not blocked.

- [ ] **Step 2: Implement CSRF**

Use session-bound CSRF token stored server-side or signed with `SESSION_SECRET`.

- [ ] **Step 3: Write rate limit tests**

Test five failed login attempts from same IP/email; sixth should get HTTP 429 until window expires.

- [ ] **Step 4: Implement rate limiter**

In-memory is acceptable for Milestone A single instance.

- [ ] **Step 5: Verify**

Run:

```bash
go test ./internal/server
go test ./...
```

- [ ] **Step 6: Commit locally**

```bash
git add internal/server web
git commit -m "feat: add csrf and login rate limiting"
```

---

## Task 9: Scrape Run History

**Files:**

- Create: `internal/storage/scrape_runs.go`
- Create: `internal/storage/scrape_runs_test.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/sse.go` only if status serialization needs reuse.

**Interfaces:**

- Produces: `Store.StartScrapeRun(ctx, trigger string) (ScrapeRun, error)`
- Produces: `Store.FinishScrapeRun(ctx, id int64, result ScrapeResult, status string, errorSummary string) error`
- Produces trigger values: `manual`, `scheduled`

- [ ] **Step 1: Write storage tests**

Test:

- start creates running row,
- finish records counts and status,
- latest returns newest run.

- [ ] **Step 2: Add migrations**

Fields:

- id,
- trigger,
- status,
- started_at,
- finished_at,
- listed,
- new_count,
- refreshed,
- scored,
- removed,
- duplicates,
- failed,
- error_summary.

- [ ] **Step 3: Wrap `runScrape` callers**

Manual and scheduled scrape should both record a run. A panic or error should end with failure status.

- [ ] **Step 4: Verify**

Run:

```bash
go test ./internal/storage ./internal/server
go test ./...
```

- [ ] **Step 5: Commit locally**

```bash
git add internal/storage internal/server
git commit -m "feat: record scrape run history"
```

---

## Task 10: Daily Scheduler

**Files:**

- Create: `internal/server/scheduler.go`
- Create: `internal/server/scheduler_test.go`
- Modify: `cmd/job-scraper/main.go`
- Modify: `internal/config/config.go`

**Interfaces:**

- Produces: `server.Scheduler`
- Produces: `server.StartScheduler(ctx, cfg SchedulerConfig)`
- Consumes: `JOBSCRAPER_DAILY_SCRAPE_TIME`
- Consumes: `JOBSCRAPER_SCHEDULER_ENABLED`

- [ ] **Step 1: Write scheduler calculation tests**

Test:

- at 07:00 KST with `08:00`, next run is today 08:00,
- at 09:00 KST with `08:00`, next run is tomorrow 08:00,
- invalid time string returns clear error.

- [ ] **Step 2: Implement next-run calculation with injectable clock**

Use KST explicitly, not local system timezone.

- [ ] **Step 3: Implement scheduler loop**

The loop sleeps until next run, then calls scheduled scrape if lock is free. If manual scrape is running, record a skipped scheduled run with reason.

- [ ] **Step 4: Wire startup**

Start scheduler only when enabled.

- [ ] **Step 5: Verify**

Run:

```bash
go test ./internal/server ./internal/config
go test ./...
```

- [ ] **Step 6: Commit locally**

```bash
git add internal/server/scheduler.go internal/server/scheduler_test.go internal/config cmd/job-scraper/main.go
git commit -m "feat: add daily scrape scheduler"
```

---

## Task 11: Manual AI Run and Opt-In Scheduled AI

**Files:**

- Modify: `internal/profile/profile.go`
- Modify: `internal/server/rerate.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/ai_rerate_test.go`
- Modify: `web/index.html`, `web/profile.html`, `web/styles.css`

**Interfaces:**

- Produces profile fields:
  - `scheduled_ai_enabled bool`
  - `ai_monthly_usd_cap string` or integer cents
  - `ai_daily_usd_cap string` or integer cents
  - `ai_run_usd_cap string` or integer cents
- Keeps scheduled scrape no-AI by default.

- [ ] **Step 1: Write profile default tests**

Assert defaults:

- scheduled AI disabled,
- monthly cap USD 10,
- daily cap USD 0.50,
- run cap USD 0.30.

- [ ] **Step 2: Disable scrape-time AI by default**

Modify `runScrape` so scheduled scrape does not call fresh AI unless profile setting enables it. Manual AI run remains available.

- [ ] **Step 3: Add front-page tip**

When scheduled AI is off and user is logged in, show a visible tip that automatic AI scoring can be enabled in settings.

- [ ] **Step 4: Add cap-hit notification**

When budget blocks AI, show a clear message and settings link.

- [ ] **Step 5: Verify**

Run:

```bash
go test ./internal/profile ./internal/server
go test ./...
```

- [ ] **Step 6: Commit locally**

```bash
git add internal/profile internal/server web
git commit -m "feat: add manual and opt-in scheduled ai controls"
```

---

## Task 12: Local Production Build Report

**Files:**

- Create: `docs/reports/production-local-build-report.md`

**Interfaces:**

- Produces handoff report for human external work.

- [ ] **Step 1: Create report directory**

Run:

```bash
mkdir -p docs/reports
```

- [ ] **Step 2: Write report**

Report must include:

- local commits completed,
- exact tests run,
- whether PostgreSQL integration test passed locally,
- whether the app was run locally against PostgreSQL,
- whether legacy SQLite is still used anywhere outside importer/demo/test compatibility,
- RDS values needed,
- owner account CLI command to run after deploy,
- production `.env` template,
- backup pull plan and MacBook path needed,
- DNS/Caddy requirements,
- remaining work blocked on human actions.

- [ ] **Step 3: Commit locally**

```bash
git add docs/reports/production-local-build-report.md
git commit -m "docs: add production local build handoff"
```

- [ ] **Step 4: Stop**

Do not continue to AWS-dependent tasks until the human reports RDS/secrets/backup details are ready.

---

## Human Handoff Requirements

The human must provide or complete:

- Create AWS RDS PostgreSQL instance.
- Provide `DATABASE_URL` or its components.
- Provide production `SESSION_SECRET`.
- Choose owner email and owner password setup method.
- Confirm production domain DNS for `jobcron.app` and `www.jobcron.app`.
- Confirm MacBook backup destination path.
- Confirm whether Anthropic production key is operator-owned or only BYOK for later users.

---

## Task 13: AWS/RDS Deployment Docs and Compose Update

**Blocked until:** Human provides RDS details, production secrets, and backup destination.

**Files:**

- Modify: `deploy/aws/Caddyfile`
- Modify: `deploy/aws/compose.yaml`
- Modify: `deploy/aws/README.md`
- Modify: `deploy/aws/HUMAN_DEPLOY_GUIDE.md`
- Create: `deploy/aws/backup-pull-macbook.md`

**Interfaces:**

- Consumes production RDS `DATABASE_URL`.
- Consumes `SESSION_SECRET`.
- Consumes `JOBSCRAPER_IMAGE`.

- [ ] **Step 1: Update Caddyfile**

Caddy should serve `jobcron.app` and `www.jobcron.app`; redirect `www.jobcron.app` to canonical `jobcron.app`.

- [ ] **Step 2: Update Compose**

App container should receive:

- `DATABASE_URL`,
- `SESSION_SECRET`,
- scheduler env,
- admin token,
- image tag.

No PostgreSQL container should run in production compose when using RDS.

- [ ] **Step 3: Write RDS deployment docs**

Include security group guidance: EC2 can reach RDS; public internet cannot reach RDS.

- [ ] **Step 4: Write MacBook backup pull runbook**

Document:

- server `pg_dump` location and retention,
- MacBook `launchd` pull job,
- behavior when MacBook is closed,
- verification command to list latest local backup.

- [ ] **Step 5: Commit locally**

```bash
git add deploy/aws
git commit -m "docs: update production rds deployment"
```

---

## Task 14: Final Local Verification

**Files:**

- No required source changes unless verification finds a bug.

- [ ] **Step 1: Run full Go tests**

Run:

```bash
go test ./...
```

Expected: all pass.

- [ ] **Step 2: Run PostgreSQL integration tests**

Run with the local Docker PostgreSQL database from Task 3, or with a disposable RDS test database:

```bash
JOBSCRAPER_TEST_POSTGRES_URL='postgres://postgres@localhost:55432/jobscraper_dev?sslmode=disable' go test ./internal/storage -run Postgres -count=1
```

Expected: pass.

- [ ] **Step 3: Build binaries**

Run:

```bash
go build ./cmd/job-scraper
go build ./cmd/job-scraper-user
go build ./cmd/job-scraper-import
```

Expected: all build.

- [ ] **Step 4: Run local browser QA against PostgreSQL**

Start local PostgreSQL and run the app against it:

```bash
docker compose -f deploy/local/compose.yaml up -d
DATABASE_URL='postgres://postgres@localhost:55432/jobscraper_dev?sslmode=disable' ./job-scraper --no-open --port 7777
```

Walk:

- login redirect behavior in production-like mode,
- profile save,
- bookmark add/remove,
- hide add/remove,
- daily briefing,
- archive,
- bookmarks,
- hidden,
- admin scrape status.

- [ ] **Step 5: Write final report**

Update `docs/reports/production-local-build-report.md` with final verification.

- [ ] **Step 6: Commit locally**

```bash
git add docs/reports/production-local-build-report.md
git commit -m "docs: update production verification report"
```

---

## Self-Review Checklist

- Spec coverage: PostgreSQL, owner login, cookie sessions, server-side state, scheduled scrape, manual/opt-in AI, RDS, backup to MacBook, demo compatibility, local-only commits, human handoff.
- Placeholder scan: no task may contain unresolved placeholders in implementation steps. Human-provided runtime values are listed only in the handoff section.
- Type consistency: storage/user/session/config names are introduced before use by later tasks.
- Scope check: public signup, password reset, billing, control dashboard, multi-instance deployment, and full alerting stay out of Milestone A.
