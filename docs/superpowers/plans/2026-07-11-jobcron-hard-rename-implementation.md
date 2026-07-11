# Jobcron Hard Rename Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename the application, commands, Go module, runtime configuration, build artifacts, deployment contract, and active documentation to `jobcron` while preserving existing MacBook and PostgreSQL data.

**Architecture:** Introduce one focused `internal/appdata` package that migrates the legacy macOS application directory before any database or AI-key access. Rename application interfaces atomically, append a PostgreSQL migration for the legacy import-owner identity, and keep Gas Town operational state outside this implementation. Existing SQL migration history and old Docker data remain intact.

**Tech Stack:** Go 1.24, PostgreSQL 18, SQLite, Docker Compose, GoReleaser, GitHub Actions, Caddy, gstack browser QA.

## Global Constraints

- Keep all commits local. Never push, create a pull request, or run a deploy command that pushes an image.
- Do not rename the Gas Town rig, bead prefix, worktrees, recovery sandboxes, or `/Users/chanbla11mit/gt/jobscraper` workspace path in this plan.
- Do not edit existing SQL migration contents; migration history is append-only.
- Do not add compatibility aliases for `JOBSCRAPER_*`; `JOBCRON_*` is the only supported application prefix after this change.
- Keep `DATABASE_URL` and `SESSION_SECRET` unchanged.
- Do not delete legacy application directories, Docker volumes, backups, or recovery state.
- Preserve historical commands in completed reports; add a rename note instead of rewriting evidence.
- Permit old-name references only under the design's Old-Name Allowlist; they do
  not add a legacy runtime interface.
- Keep Worknet and `JOBCRON_PROXY_SECRET` disabled for the first production pass.
- Production schedule is `05:00` Korea Standard Time.
- Production image is `ohchanwu/jobcron:0.2-linuxarm64`.
- Use test-driven development: observe each new behavioral test fail before writing its implementation.

## File Structure

- `internal/appdata/paths.go`: canonical and legacy application-directory names plus one-time migration logic.
- `internal/appdata/paths_test.go`: migration state-machine tests using temporary directories only.
- `internal/storage/store.go`: canonical default SQLite database path.
- `internal/ai/keys.go`: canonical default AI key path.
- `cmd/jobcron*`: renamed application command packages.
- `internal/config/config.go`: canonical `JOBCRON_*` runtime contract.
- `internal/storage/postgres_migrations/0013_rename_import_owner_email.sql`: append-only sentinel rename.
- `internal/server/auth.go` and `internal/server/csrf.go`: canonical cookie and header identities.
- `web/bookmark.js` and `web/not-interested.js`: canonical demo local-storage identities.
- `.github/workflows/ci.yml`, `.goreleaser.yml`, Dockerfiles, and Compose files: canonical build and deployment contract.
- Active README and deployment documents: canonical commands, URLs, environment variables, and paths.

---

### Task 1: Preserve MacBook Application Data

**Files:**
- Create: `internal/appdata/paths.go`
- Create: `internal/appdata/paths_test.go`
- Modify: `internal/storage/store.go`
- Modify: `internal/storage/store_test.go`
- Modify: `internal/ai/keys.go`
- Modify: `internal/ai/keys_test.go`
- Modify: `cmd/job-scraper/main.go`
- Modify: `cmd/job-scraper/main_test.go`

**Interfaces:**
- Produces: `appdata.Dir(root string) string`
- Produces: `appdata.LegacyDir(root string) string`
- Produces: `appdata.Prepare(root string) error`
- Consumes: a root returned by `os.UserConfigDir()`; tests pass `t.TempDir()`.

- [ ] **Step 1: Write failing directory-migration tests**

Create table-driven tests covering: neither directory exists, canonical-only,
legacy-only, both directories, and a rename failure. The legacy-only fixture must
contain `jobs.db`, `jobs.db-wal`, `jobs.db-shm`, a backup, and `ai_keys.json`.

```go
func TestPrepareMigratesLegacyDirectoryAtomically(t *testing.T) {
	root := t.TempDir()
	legacy := LegacyDir(root)
	if err := os.MkdirAll(legacy, 0o700); err != nil { t.Fatal(err) }
	for _, name := range []string{"jobs.db", "jobs.db-wal", "jobs.db-shm", "jobs.db.bak", "ai_keys.json"} {
		if err := os.WriteFile(filepath.Join(legacy, name), []byte(name), 0o600); err != nil { t.Fatal(err) }
	}
	if err := Prepare(root); err != nil { t.Fatalf("Prepare: %v", err) }
	if _, err := os.Stat(legacy); !errors.Is(err, os.ErrNotExist) { t.Fatalf("legacy still exists: %v", err) }
	for _, name := range []string{"jobs.db", "jobs.db-wal", "jobs.db-shm", "jobs.db.bak", "ai_keys.json"} {
		if got, err := os.ReadFile(filepath.Join(Dir(root), name)); err != nil || string(got) != name {
			t.Fatalf("%s: got=%q err=%v", name, got, err)
		}
	}
}

func TestPrepareRejectsDirectoryCollision(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{LegacyDir(root), Dir(root)} {
		if err := os.MkdirAll(dir, 0o700); err != nil { t.Fatal(err) }
	}
	if err := Prepare(root); err == nil || !strings.Contains(err.Error(), "both") {
		t.Fatalf("Prepare error = %v, want collision", err)
	}
}
```

- [ ] **Step 2: Run the new tests and verify failure**

Run: `go test ./internal/appdata -run TestPrepare -count=1 -v`

Expected: FAIL because `Dir`, `LegacyDir`, and `Prepare` do not exist.

- [ ] **Step 3: Implement the minimal migration helper**

```go
package appdata

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	dirName       = "jobcron"
	legacyDirName = "job-scraper"
)

func Dir(root string) string       { return filepath.Join(root, dirName) }
func LegacyDir(root string) string { return filepath.Join(root, legacyDirName) }

func Prepare(root string) error { return prepare(root, os.Rename) }

func prepare(root string, rename func(string, string) error) error {
	legacy, canonical := LegacyDir(root), Dir(root)
	_, legacyErr := os.Stat(legacy)
	_, canonicalErr := os.Stat(canonical)
	legacyExists := legacyErr == nil
	canonicalExists := canonicalErr == nil
	if legacyErr != nil && !errors.Is(legacyErr, os.ErrNotExist) { return fmt.Errorf("appdata: inspect legacy directory: %w", legacyErr) }
	if canonicalErr != nil && !errors.Is(canonicalErr, os.ErrNotExist) { return fmt.Errorf("appdata: inspect canonical directory: %w", canonicalErr) }
	if legacyExists && canonicalExists { return fmt.Errorf("appdata: both legacy %q and canonical %q directories exist", legacy, canonical) }
	if !legacyExists { return nil }
	if err := rename(legacy, canonical); err != nil { return fmt.Errorf("appdata: rename %q to %q: %w", legacy, canonical, err) }
	return nil
}
```

- [ ] **Step 4: Point database and AI-key defaults at the canonical directory**

In both default-path functions, obtain `os.UserConfigDir()` and join through
`appdata.Dir(dir)`. In `main`, call `appdata.Prepare(configRoot)` immediately
after configuration parsing and before opening storage or loading AI keys.

- [ ] **Step 5: Run focused tests**

Run: `go test ./internal/appdata ./internal/storage ./internal/ai ./cmd/job-scraper -count=1`

Expected: PASS. No test may read or rename the real home directory.

- [ ] **Step 6: Commit the migration checkpoint**

```bash
git add internal/appdata internal/storage/store.go internal/storage/store_test.go internal/ai/keys.go internal/ai/keys_test.go cmd/job-scraper/main.go cmd/job-scraper/main_test.go
git commit -m "feat: migrate application data to jobcron directory"
```

---

### Task 2: Rename Go Module, Commands, And Environment Contract

**Files:**
- Rename: `cmd/job-scraper` to `cmd/jobcron`
- Rename: `cmd/job-scraper-user` to `cmd/jobcron-user`
- Rename: `cmd/job-scraper-import` to `cmd/jobcron-import`
- Modify: `go.mod`
- Modify: all tracked `*.go` imports under `cmd/` and `internal/`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `.gitignore`

**Interfaces:**
- Produces: Go module `github.com/ohchanwu/jobcron`
- Produces: command packages `./cmd/jobcron`, `./cmd/jobcron-user`, `./cmd/jobcron-import`
- Produces: the 18 `JOBCRON_*` names listed in the approved design.

- [ ] **Step 1: Change configuration tests first**

Replace test inputs with `JOBCRON_*` and add a rejection test proving an old
variable no longer configures the application.

```go
func TestLoadIgnoresLegacyEnvironmentPrefix(t *testing.T) {
	cfg, err := config.Load(nil, map[string]string{
		"JOBSCRAPER_ENV":  "production",
		"JOBSCRAPER_PORT": "9000",
	})
	if err != nil { t.Fatalf("Load: %v", err) }
	if cfg.Production { t.Fatal("legacy JOBSCRAPER_ENV enabled production") }
	if cfg.Port == 9000 { t.Fatal("legacy JOBSCRAPER_PORT changed port") }
}
```

- [ ] **Step 2: Run the rejection test and verify failure**

Run: `go test ./internal/config -run TestLoadIgnoresLegacyEnvironmentPrefix -count=1 -v`

Expected: FAIL because the current loader still reads `JOBSCRAPER_*`.

- [ ] **Step 3: Rename command directories and module path atomically**

Run:

```bash
git mv cmd/job-scraper cmd/jobcron
git mv cmd/job-scraper-user cmd/jobcron-user
git mv cmd/job-scraper-import cmd/jobcron-import
go mod edit -module github.com/ohchanwu/jobcron
```

Replace every internal import prefix
`github.com/ohchanwu/job-scraper/` with `github.com/ohchanwu/jobcron/`.

- [ ] **Step 4: Rename the complete environment contract**

Replace the complete application-prefixed contract:

- `JOBSCRAPER_ADMIN_TOKEN` -> `JOBCRON_ADMIN_TOKEN`
- `JOBSCRAPER_AI_MODEL` -> `JOBCRON_AI_MODEL`
- `JOBSCRAPER_ANTHROPIC_KEY` -> `JOBCRON_ANTHROPIC_KEY`
- `JOBSCRAPER_DAILY_SCRAPE_TIME` -> `JOBCRON_DAILY_SCRAPE_TIME`
- `JOBSCRAPER_DB` -> `JOBCRON_DB`
- `JOBSCRAPER_DEMO` -> `JOBCRON_DEMO`
- `JOBSCRAPER_DEMODAY_ANON_KEY` -> `JOBCRON_DEMODAY_ANON_KEY`
- `JOBSCRAPER_ENV` -> `JOBCRON_ENV`
- `JOBSCRAPER_HOST` -> `JOBCRON_HOST`
- `JOBSCRAPER_IMAGE` -> `JOBCRON_IMAGE`
- `JOBSCRAPER_NO_OPEN` -> `JOBCRON_NO_OPEN`
- `JOBSCRAPER_OWNER_PASSWORD` -> `JOBCRON_OWNER_PASSWORD`
- `JOBSCRAPER_PORT` -> `JOBCRON_PORT`
- `JOBSCRAPER_PROXY_SECRET` -> `JOBCRON_PROXY_SECRET`
- `JOBSCRAPER_SCHEDULER_ENABLED` -> `JOBCRON_SCHEDULER_ENABLED`
- `JOBSCRAPER_TEST_POSTGRES_URL` -> `JOBCRON_TEST_POSTGRES_URL`
- `JOBSCRAPER_USE_HEADLESS` -> `JOBCRON_USE_HEADLESS`
- `JOBSCRAPER_WORKNET_KEY` -> `JOBCRON_WORKNET_KEY`

Update configuration errors, CLI messages, live-test opt-ins, and test fixtures.
Keep `DATABASE_URL`, `SESSION_SECRET`, and `AISPIKE_SKIP_HAIKU` unchanged.

- [ ] **Step 5: Rename command-visible strings**

Update command package comments, usage strings, log prefixes, `--version`
output, default flag-set name, and Worknet instructions from `job-scraper` to
`jobcron`. Update binary ignore entries to `/jobcron` and `/jobcron.exe`.

- [ ] **Step 6: Run module and command tests**

Run:

```bash
go list ./...
go test ./internal/config ./cmd/jobcron ./cmd/jobcron-user ./cmd/jobcron-import -count=1
go build ./cmd/jobcron ./cmd/jobcron-user ./cmd/jobcron-import
```

Expected: all commands exit 0 and every listed package starts with
`github.com/ohchanwu/jobcron`.

- [ ] **Step 7: Commit the canonical code contract**

```bash
git add go.mod cmd internal .gitignore
git commit -m "refactor: rename application contract to jobcron"
```

---

### Task 3: Rename The Legacy Import Owner Safely

**Files:**
- Create: `internal/storage/postgres_migrations/0013_rename_import_owner_email.sql`
- Modify: `cmd/jobcron-import/main.go`
- Modify: `cmd/jobcron-import/main_test.go`
- Modify: `internal/storage/postgres_integration_test.go`

**Interfaces:**
- Produces: fallback import owner `sqlite-import-owner@jobcron.local`
- Preserves: existing `0006_user_scoped_state.sql` bytes.

- [ ] **Step 1: Write failing importer and PostgreSQL migration tests**

The importer default test must expect `sqlite-import-owner@jobcron.local`.
PostgreSQL tests use the existing `newPostgresTestStore(t)` helper, insert test
rows after the normal migration sequence, read migration 13 through the package's
existing `postgresMigrationsFS`, execute it explicitly, and assert the result.
This avoids inventing a second migration runner solely for the test.

```go
func TestRenameImportOwnerMigrationPreservesRealOwner(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	_, err := st.SQLDB().ExecContext(ctx, `DELETE FROM users`)
	if err != nil { t.Fatal(err) }
	_, err = st.SQLDB().ExecContext(ctx, `
INSERT INTO users (email, password_hash, created_at, updated_at)
VALUES ('owner@example.com', 'real-hash', now(), now())`)
	if err != nil { t.Fatal(err) }
	sqlBytes, err := postgresMigrationsFS.ReadFile("postgres_migrations/0013_rename_import_owner_email.sql")
	if err != nil { t.Fatal(err) }
	if _, err := st.SQLDB().ExecContext(ctx, string(sqlBytes)); err != nil { t.Fatal(err) }
	var count int
	if err := st.SQLDB().QueryRowContext(ctx, `SELECT count(*) FROM users WHERE email = 'owner@example.com' AND password_hash = 'real-hash'`).Scan(&count); err != nil { t.Fatal(err) }
	if count != 1 { t.Fatalf("real owner count = %d, want 1", count) }
}
```

- [ ] **Step 2: Run focused tests and verify failure**

Run: `go test ./cmd/jobcron-import ./internal/storage -run 'ImportOwner|RenameImportOwner' -count=1 -v`

Expected: FAIL because migration 13 and the canonical fallback do not exist.

- [ ] **Step 3: Add the append-only migration**

```sql
UPDATE users
   SET email = 'sqlite-import-owner@jobcron.local',
       updated_at = now()
 WHERE email = 'sqlite-import-owner@job-scraper.local'
   AND password_hash = 'imported-sqlite-no-login'
   AND NOT EXISTS (
       SELECT 1 FROM users
        WHERE email = 'sqlite-import-owner@jobcron.local'
   );
```

Do not edit migration 0006. Change the importer's default owner email to the
canonical value. An explicit `--owner-email ohchanwu@gmail.com` continues to
attach imported data directly to the real owner account.

- [ ] **Step 4: Run importer and PostgreSQL tests**

Run: `go test ./cmd/jobcron-import ./internal/storage -count=1`

Expected: PASS, with PostgreSQL integration tests skipping only when
`JOBCRON_TEST_POSTGRES_URL` is absent.

- [ ] **Step 5: Commit the database identity checkpoint**

```bash
git add cmd/jobcron-import internal/storage/postgres_migrations/0013_rename_import_owner_email.sql internal/storage/postgres_integration_test.go
git commit -m "feat: rename legacy import owner to jobcron"
```

---

### Task 4: Rename Browser, Cookie, And HTTP Identities

**Files:**
- Modify: `internal/server/auth.go`
- Modify: `internal/server/auth_test.go`
- Modify: `internal/server/csrf.go`
- Modify: `internal/server/csrf_test.go`
- Modify: `internal/server/server.go`
- Modify: related server tests that set the admin header
- Modify: `web/bookmark.js`
- Modify: `web/not-interested.js`

**Interfaces:**
- Produces: cookies `jobcron_session` and `jobcron_csrf`
- Produces: header `X-Jobcron-Admin-Token`
- Produces: local-storage keys `jobcronDemoBookmarks` and `jobcronDemoHidden`

- [ ] **Step 1: Update tests to assert canonical identities**

Add explicit constant-level or response-cookie assertions so tests fail while
the old names remain.

```go
func TestProductionCookieNamesUseJobcronPrefix(t *testing.T) {
	if sessionCookieName != "jobcron_session" { t.Fatalf("session cookie = %q", sessionCookieName) }
	if csrfCookieName != "jobcron_csrf" { t.Fatalf("csrf cookie = %q", csrfCookieName) }
}
```

- [ ] **Step 2: Run focused tests and verify failure**

Run: `go test ./internal/server -run 'CookieNames|AdminToken|CSRF' -count=1 -v`

Expected: FAIL on old cookie/header values.

- [ ] **Step 3: Rename server and browser identities**

Change the two cookie constants, every `X-JobScraper-Admin-Token` read/write,
and both demo local-storage keys. Do not add fallback reads for old keys; the
approved pre-launch policy accepts session and demo-state reset.

- [ ] **Step 4: Run server tests**

Run: `go test ./internal/server -count=1`

Expected: PASS.

- [ ] **Step 5: Commit browser and HTTP identity changes**

```bash
git add internal/server web/bookmark.js web/not-interested.js
git commit -m "refactor: rename browser and HTTP identities to jobcron"
```

---

### Task 5: Rename Build, Release, And Deployment Contracts

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `.goreleaser.yml`
- Modify: `deploy/demo/Dockerfile`
- Modify: `deploy/demo/compose.yaml`
- Modify: `deploy/demo/Caddyfile`
- Modify: `deploy/demo/README.md`
- Modify: `deploy/demo/HUMAN_DEPLOY_GUIDE.md`
- Modify: `deploy/local/compose.yaml`
- Modify: `deploy/local/README.md`
- Modify: `deploy/production/Dockerfile`
- Modify: `deploy/production/compose.yaml`
- Modify: `deploy/production/Caddyfile`
- Modify: `deploy/production/.env.example`
- Modify: `deploy/production/README.md`
- Modify: `deploy/production/HUMAN_DEPLOY_GUIDE.md`
- Test: `deploy/production/compose.yaml` through exact rendered-config assertions

**Interfaces:**
- Produces: image `ohchanwu/jobcron:0.2-linuxarm64`
- Produces: Linux arm64 container entry point `jobcron`
- Produces: production schedule `JOBCRON_DAILY_SCRAPE_TIME=05:00`
- Produces: EC2 path `/srv/jobcron`

- [ ] **Step 1: Capture the current rendered Compose contract**

Run this exact assertion before editing:

```bash
rendered="$(JOBSCRAPER_IMAGE=legacy/job-scraper:test DATABASE_URL='postgres://jobcron_admin:dummy@example.invalid:5432/jobcron?sslmode=require' SESSION_SECRET=dummy-session-secret docker compose -f deploy/production/compose.yaml config)"
printf '%s\n' "$rendered" | rg 'JOBSCRAPER_IMAGE|JOBSCRAPER_ENV|JOBSCRAPER_SCHEDULER_ENABLED|JOBSCRAPER_DAILY_SCRAPE_TIME'
```

Expected: the command finds the old contract, establishing the pre-change state.

- [ ] **Step 2: Verify current deployment contract fails canonical checks**

Run:

```bash
JOBCRON_IMAGE=ohchanwu/jobcron:0.2-linuxarm64 DATABASE_URL='postgres://jobcron_admin:dummy@example.invalid:5432/jobcron?sslmode=require' SESSION_SECRET=dummy-session-secret docker compose -f deploy/production/compose.yaml config
```

Expected: FAIL because the current Compose file still requires
`JOBSCRAPER_IMAGE`.

- [ ] **Step 3: Rename CI and GoReleaser inputs and outputs**

Build `./cmd/jobcron`, emit binary `jobcron`, and name archives
`jobcron_{{ .Os }}_{{ .Arch }}`. Preserve existing target platforms and release
behavior.

- [ ] **Step 4: Rename both Docker stacks and deployment instructions**

Build `./cmd/jobcron`, copy `/usr/local/bin/jobcron`, and set
`ENTRYPOINT ["jobcron"]`. Rename every application-prefixed variable and
product-specific path. Use image `ohchanwu/jobcron:0.2-linuxarm64` and schedule
`05:00`. Keep Worknet and proxy-secret unset in production.

- [ ] **Step 5: Create a fresh canonical local PostgreSQL identity without deleting old data**

Rename the Compose service configuration to use `jobcron_dev` and a canonical
volume key. Do not remove the legacy Docker volume. Document that the canonical
stack starts clean and the legacy volume remains available for rollback.

- [ ] **Step 6: Validate release and Compose configuration**

Run:

```bash
goreleaser check
JOBCRON_IMAGE=ohchanwu/jobcron:0.2-linuxarm64 DATABASE_URL='postgres://jobcron_admin:dummy@example.invalid:5432/jobcron?sslmode=require' SESSION_SECRET=dummy-session-secret docker compose -f deploy/production/compose.yaml config
docker compose -f deploy/local/compose.yaml config
```

Expected: all commands exit 0; rendered output has no `JOBSCRAPER_` variables.

- [ ] **Step 7: Build both Linux arm64 images locally without pushing**

Run:

```bash
docker build --platform linux/arm64 -f deploy/production/Dockerfile -t jobcron:rename-check .
docker build --platform linux/arm64 -f deploy/demo/Dockerfile -t jobcron:demo-rename-check .
```

Expected: both builds exit 0 and neither command pushes an image.

- [ ] **Step 8: Commit build and deployment changes**

```bash
git add .github/workflows/ci.yml .goreleaser.yml deploy
git commit -m "build: rename release and deployment contract to jobcron"
```

---

### Task 6: Rename Active Documentation And Preserve Historical Evidence

**Files:**
- Modify: `README.md`
- Modify: `README.ko.md`
- Modify: `AGENTS.md`
- Modify: `CLAUDE.md`
- Modify: `feature-ideas.md`
- Modify: `feature-ideas.ko.md`
- Modify: `docs/plans/production-app-roadmap.md`
- Modify: `docs/superpowers/plans/2026-07-09-jobcron-production-app.md`
- Modify: `docs/superpowers/specs/2026-07-10-production-deploy-prep-report.md`
- Modify: `docs/superpowers/specs/2026-07-10-rds-production-settings-recommendation.md`
- Modify: `docs/superpowers/specs/2026-07-11-jobcron-hard-rename-design.md`
- Preserve with rename note: `docs/superpowers/specs/2026-07-10-production-local-build-report.md`
- Preserve with rename note: completed plans and reports whose commands were executed before the rename

**Interfaces:**
- Produces: active documentation using only canonical names.
- Preserves: exact historical command evidence under an explicit rename note.

- [ ] **Step 1: Inventory remaining old-name references by category**

Run a repository scan that confirms every old-name occurrence belongs to one of
the design's eight allowed categories: source-side rename mappings; immutable
pre-rename migrations; the migration 0013 sentinel predicate and migration
fixtures; explicit rejection tests; `legacyDirName`; the legacy Docker volume;
dated rename-noted evidence; or deferred Gas Town context. Do not globally
replace historical evidence.

- [ ] **Step 2: Update active documentation**

Change repository URLs, release downloads, command examples, environment names,
binary names, EC2 paths, and product prose to `jobcron`. Mark repository rename,
EC2 environment rename, and `05:00` schedule feedback as settled.

- [ ] **Step 3: Add one rename note to historical artifacts**

Use this exact note near the top of each retained historical artifact:

```markdown
> Rename note (2026-07-11): This document records commands and paths used before
> the application was renamed from `job-scraper` to `jobcron`. Historical command
> output remains unchanged; current interfaces use `jobcron` and `JOBCRON_*`.
```

- [ ] **Step 4: Verify documentation paths and secret hygiene**

Run path-existence checks for every local Markdown link and scan tracked files
for AWS access keys, private keys, Anthropic keys, literal session secrets, and
database passwords. Expected: no broken local paths and no secrets.

- [ ] **Step 5: Commit documentation changes**

```bash
git add README.md README.ko.md AGENTS.md CLAUDE.md docs deploy
git commit -m "docs: rename active project documentation to jobcron"
```

---

### Task 7: Full Verification And Local Handoff

**Files:**
- Modify only files required to correct verification failures caused by the rename.
- Do not alter Gas Town state, remote branches, or EC2 secrets.

**Interfaces:**
- Consumes: all prior task outputs.
- Produces: verified local commits and a clean tracked worktree.

- [ ] **Step 1: Run formatting, package, build, vet, and test checks**

Run:

```bash
gofmt -w cmd internal
test -z "$(gofmt -l .)"
go list ./...
go build ./cmd/jobcron ./cmd/jobcron-user ./cmd/jobcron-import
go vet ./...
go test ./... -count=1
```

Expected: all commands exit 0 and every local package uses module prefix
`github.com/ohchanwu/jobcron`.

- [ ] **Step 2: Run PostgreSQL 18 integration tests**

Use a clean throwaway database and canonical variable:

```bash
JOBCRON_TEST_POSTGRES_URL='postgres://postgres@localhost:55432/jobcron_rename_test?sslmode=disable' go test ./cmd/jobcron-import ./cmd/jobcron-user ./internal/storage -count=1
```

Expected: PASS. Create and drop only the dedicated throwaway database; do not
modify production RDS or the legacy `jobscraper_dev` database.

- [ ] **Step 3: Verify the data migration through the real command path in a temporary home**

Create a temporary macOS-style config root containing legacy `jobs.db` sidecars
and `ai_keys.json`, run `go run ./cmd/jobcron --no-open` with a temporary home or
explicit test hook, and confirm the canonical directory contains every file.
Never run this verification against the real home directory.

- [ ] **Step 4: Run frontend QA because browser storage JavaScript changed**

Required skill: `frontend-qa`. Start the application against a clean temporary
PostgreSQL database without opening the user's browser. Use gstack `/browse` to
walk the primary desktop and mobile flows, exercise bookmark/hide behavior, and
verify canonical local-storage keys. Confirm no console errors and no overlap or
responsive regression. Include the local preview URL in the final report.

- [ ] **Step 5: Re-run Docker, Compose, GoReleaser, and repository scans**

Confirm both images build locally, both Compose files render, GoReleaser checks,
and active interfaces contain no old names. Every remaining old-name occurrence
must belong to the design's Old-Name Allowlist:

- source-side rename mappings,
- immutable pre-rename migrations, including `0001` and `0006`,
- the migration 0013 old sentinel predicate and migration tests/fixtures,
- explicit rejection tests for old environment/header/cookie identities,
- `legacyDirName` for existing app-data migration,
- the exact legacy Docker rollback volume,
- dated historical evidence under the rename note, or
- deferred Gas Town rig/workspace identity.

- [ ] **Step 6: Re-read the cumulative diff against the pre-rename commit**

Verify every design acceptance criterion, no user edits were lost, no secret was
added, and no unrelated refactor entered the diff.

- [ ] **Step 7: Commit verification-only fixes if any**

```bash
git add .github/workflows/ci.yml .goreleaser.yml .gitignore go.mod cmd internal web deploy docs README.md README.ko.md AGENTS.md CLAUDE.md feature-ideas.md feature-ideas.ko.md
git commit -m "fix: complete jobcron rename verification"
```

Skip this commit when verification required no edits.

- [ ] **Step 8: Update the local Git remote without pushing**

Run:

```bash
git remote set-url origin git@github.com:ohchanwu/jobcron.git
git remote get-url --all origin
```

Expected: the canonical SSH URL is printed. Do not run `git push`.

- [ ] **Step 9: Final status and handoff**

Report local commit hashes, verification results, preview URL, the ignored local
build report path, the still-deferred Gas Town rename, and exact human-only EC2
steps that remain. Verify `gt mail inbox` before going idle.
