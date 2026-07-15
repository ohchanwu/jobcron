# PostgreSQL Convergence Slice 3: Local Bootstrap And Preview Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make PostgreSQL 18 start automatically for ordinary local runs and
move the writable preview to an isolated disposable PostgreSQL database.

**Architecture:** A small `internal/localdb` package owns an embedded Compose
definition, invokes Docker Compose under the fixed `jobcron-local` project,
waits for a healthy database, and returns one fixed local URL. Explicit
`DATABASE_URL` bypasses the package. The preview creates a unique database and
temporary encryption key, then removes both unless the operator asks to keep
them.

**Tech Stack:** Go 1.26, PostgreSQL 18 Alpine, Docker Compose v2, POSIX shell,
`pg_isready`, Go process and contract tests.

## Global Constraints

- Start after Slice 2 is merged locally and passing.
- Treat the exact prior-slice commit named in the bead as authoritative, not
  `origin/main`. A separate worker clone must fetch that commit from the Mayor
  rig and verify its hash before editing.
- Do not use the normal `gt done` path because it submits an MR. Commit locally,
  report the exact tip and verification evidence, then defer so Mayor can fetch
  the clone and integrate without pushing.
- Follow the approved
  [convergence specification](../specs/260714-postgresql-local-convergence-user-ai-credentials.md).
- PostgreSQL AI score, usage, and runtime paths now require a positive user ID.
  The temporary `userID == 0` compatibility path belongs only to retained
  SQLite reads. Resolve the stable local owner before a PostgreSQL-backed
  request reaches `Server`.
- Preserve the Slice 2 operation boundaries: acquire the scrape flight lock
  before resolving an AI runtime, pass one runtime through an operation, render
  from one profile/hash snapshot, omit missing or stale score rows, and run
  rule-based startup recovery even when AI runtime resolution fails.
- Do not remove the SQLite fallback or `--db` in this slice. Slice 4 owns that
  activation after the verified importer lands.
- Never run `docker compose down -v` from tests or automatic startup.
- Never connect preview tests to `jobcron_dev`, RDS, or the retained SQLite
  file.
- An explicit `DATABASE_URL` must not probe Docker, Compose, port `55432`, or
  the managed local project.
- Keep the local master key outside Docker volumes at the protected OS config
  path created by Slice 1.
- Keep commits local. Never push.

## Slice Completion Contract

This slice is complete when the managed PostgreSQL service has a deterministic
project, URL, health check, and durable volume; explicit URLs bypass it; the
preview uses and cleans a unique PostgreSQL database and key; and the runtime
cutover is ready for Slice 4 without yet making existing SQLite data
unreachable.

---

### Task 1: Create the canonical embedded local PostgreSQL definition

**Files:**

- Create: `internal/localdb/compose.yaml`
- Create: `internal/localdb/contract.go`
- Create: `internal/localdb/contract_test.go`
- Modify: `deploy/local/compose.yaml`

- [ ] **Step 1: Write the contract test first**

The test must compare these exact fields in the embedded and human-readable
definitions:

```text
service: postgres
image: postgres:18-alpine
host port: 55432
container port: 5432
database: jobcron_dev
volume name: jobcron-postgres18-cluster
mount: /var/lib/postgresql
health command: pg_isready -U postgres -d jobcron_dev
```

Add `gopkg.in/yaml.v3` only if a pure standard-library comparison would be
brittle. Compare decoded fields, not whitespace.

- [ ] **Step 2: Run the test and confirm the mismatch**

```sh
go test ./internal/localdb -run ComposeContract -count=1
```

Expected: failure because the embedded definition does not exist and the
current deploy file lacks an explicit volume `name` and health check.

- [ ] **Step 3: Add matching Compose definitions**

Both files must declare the durable volume explicitly:

```yaml
volumes:
  jobcron-postgres18-cluster:
    name: jobcron-postgres18-cluster
```

Add a `pg_isready` health check with a short interval and enough retries for the
60-second startup ceiling. Keep PostgreSQL data mounted at
`/var/lib/postgresql` for the PostgreSQL 18 image layout.

- [ ] **Step 4: Run the contract test and render the deploy file**

```sh
go test ./internal/localdb -run ComposeContract -count=1
docker compose -p jobcron-local -f deploy/local/compose.yaml config
```

Expected: PASS; rendered output contains the exact image, port, database,
health check, and named volume above.

- [ ] **Step 5: Commit the Compose contract**

```sh
git add go.mod go.sum internal/localdb/compose.yaml \
  internal/localdb/contract.go internal/localdb/contract_test.go \
  deploy/local/compose.yaml
git commit -m "feat(localdb): define canonical postgres compose service"
```

### Task 2: Implement managed local PostgreSQL startup

**Files:**

- Create: `internal/localdb/ensure.go`
- Create: `internal/localdb/ensure_test.go`

- [ ] **Step 1: Define the testable command boundary**

Use these public constants and entry point:

```go
const (
    ProjectName = "jobcron-local"
    DatabaseURL = "postgres://postgres@127.0.0.1:55432/jobcron_dev?sslmode=disable"
)

func Ensure(ctx context.Context) (string, error)
```

Keep command execution behind an unexported interface so tests can capture
arguments and inject errors without invoking Docker.

- [ ] **Step 2: Write failing decision and failure-path tests**

Add at minimum:

```go
func TestEnsureUsesFixedProjectAndEmbeddedCompose(t *testing.T)
func TestEnsureReturnsURLOnlyAfterComposeWaitSucceeds(t *testing.T)
func TestEnsureExplainsMissingDockerAndDatabaseURLBypass(t *testing.T)
func TestEnsureExplainsMissingComposePlugin(t *testing.T)
func TestEnsureExplainsDaemonFailure(t *testing.T)
func TestEnsureRefusesForeignPortOwner(t *testing.T)
func TestEnsurePreservesContainerStateOnHealthTimeout(t *testing.T)
func TestEnsureHonorsCallerCancellation(t *testing.T)
```

Every error assertion must require the failing component and the phrase
`DATABASE_URL` without leaking command environment values.

- [ ] **Step 3: Implement preflight and startup**

`Ensure` must:

1. locate `docker` and validate `docker compose version`;
2. validate daemon access with a non-mutating command;
3. determine whether the managed `postgres` service already owns port `55432`;
4. refuse a foreign listener instead of attaching blindly;
5. materialize the embedded Compose bytes in a private temporary directory;
6. run `docker compose -p jobcron-local -f <temp-file> up -d --wait
   --wait-timeout 60`;
7. preserve service state on failure and print the exact `ps` and `logs`
   diagnostic commands; and
8. return `DatabaseURL` only after the command succeeds.

Do not invoke `down`, remove a container, or remove a volume in an error path.

- [ ] **Step 4: Run unit tests and a real user-path smoke test**

```sh
go test ./internal/localdb -count=1
docker compose -p jobcron-local -f deploy/local/compose.yaml up -d --wait
docker compose -p jobcron-local -f deploy/local/compose.yaml ps
```

Expected: unit PASS; the real `postgres` service reports healthy. If Docker is
not available, report that environment limitation and do not fake the smoke
result.

- [ ] **Step 5: Commit startup support**

```sh
git add internal/localdb/ensure.go internal/localdb/ensure_test.go
git commit -m "feat(localdb): ensure managed postgres is healthy"
```

### Task 3: Add the runtime database and master-key decision seam

**Files:**

- Modify: `cmd/jobcron/main.go`
- Modify: `cmd/jobcron/main_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/storage/users.go`
- Modify: `internal/storage/users_test.go`
- Modify: `internal/server/auth.go`
- Modify: `internal/server/auth_test.go`

- [ ] **Step 1: Write the target decision tests**

Add tests for:

```go
func TestResolveRuntimeExplicitURLBypassesLocalEnsure(t *testing.T)
func TestResolveRuntimeProductionRequiresExplicitURL(t *testing.T)
func TestResolveRuntimeLocalWithoutURLUsesManagedPostgres(t *testing.T)
func TestResolveRuntimeLocalLoadsProtectedMasterKey(t *testing.T)
func TestResolveRuntimeProductionNeverCreatesLocalMasterKey(t *testing.T)
func TestResolveRuntimeManagedLocalCreatesStablePositiveUser(t *testing.T)
func TestResolveRuntimeExplicitURLRequiresSolePositiveUser(t *testing.T)
```

Inject `ensureLocalPostgres` and `loadLocalMasterKey` function variables in the
test package. Assert call counts, selected URLs, and key lengths; never print
the key bytes.

- [ ] **Step 2: Introduce one resolved PostgreSQL startup value**

```go
type runtimeStorage struct {
    DatabaseURL             string
    CredentialEncryptionKey []byte
    ManagedLocal            bool
    UserID                  int64
}

func resolvePostgresRuntime(
    ctx context.Context,
    cfg config.Config,
) (runtimeStorage, error)
```

Rules:

- production uses the configured URL and configured key only;
- local with an explicit URL bypasses `localdb.Ensure`; and
- local without a URL calls `localdb.Ensure`.

Slice 2 already loads the protected local key. This helper owns database
selection and returns that already-resolved key beside the URL so Slice 4 can
activate both values together.

For the managed local project, preserve the existing no-login local UX without
using user ID `0`: create or resolve exactly one synthetic local owner with a
stable `example.invalid` address and an unusable password hash, then return its
positive ID. For an explicit non-production URL, require exactly one existing
user and refuse zero or multiple users instead of creating an account on an
unknown database.

- [ ] **Step 3: Keep the activation seam buildable but inactive**

Add a PostgreSQL-only opener that accepts `runtimeStorage`, and use it for the
existing production and explicit-URL paths. Do not call
`resolvePostgresRuntime` for an ordinary no-URL launch yet. Do not delete
`DBPath`, `--db`, `storage.Open`, or `storage.OpenAt`; Slice 4 activates the
managed no-URL path and deletes the fallback in the same commit after importer
verification exists.

Add an immutable local-user injection point on `Server` and make non-production
PostgreSQL request paths return that positive ID instead of `0`. Production
continues to resolve the authenticated session. Tests must prove the synthetic
local owner is created only for the fixed managed local database, is reused on
restart, and is never created for production or an explicit URL. Keep the
existing scheduler lock ordering, operation-scoped runtime, render snapshot,
and rule-recovery regression tests passing while this startup seam changes.

- [ ] **Step 4: Run tests and commit the seam**

```sh
go test ./cmd/jobcron ./internal/config ./internal/localdb -count=1
git add cmd/jobcron/main.go cmd/jobcron/main_test.go \
  internal/config/config.go internal/config/config_test.go \
  internal/storage/users.go internal/storage/users_test.go \
  internal/server/auth.go internal/server/auth_test.go
git commit -m "refactor(jobcron): resolve local postgres and master key"
```

### Task 4: Convert the interactive preview to disposable PostgreSQL

**Files:**

- Modify: `scripts/preview-interactive.sh`
- Modify: `scripts/preview_interactive_test.go`

- [ ] **Step 1: Write failing preview lifecycle tests**

Add these cases:

```go
func TestPreviewUsesUniquePostgresDatabase(t *testing.T)
func TestPreviewWritesDoNotReachJobcronDevOrSecondPreview(t *testing.T)
func TestPreviewDropsDatabaseAndKeyOnExit(t *testing.T)
func TestPreviewKeepRetainsAndPrintsDatabaseAndKeyLocations(t *testing.T)
func TestPreviewRejectsProductionDatabaseURL(t *testing.T)
```

Tests must skip with a clear reason when Docker is unavailable. They must use a
reserved loopback HTTP port and query PostgreSQL to verify actual rows, not only
inspect environment strings.

- [ ] **Step 2: Define the preview lifecycle**

The script must:

1. validate the optional HTTP port;
2. ensure `jobcron-local` is healthy through `deploy/local/compose.yaml`;
3. create a database named `jobcron_preview_<random-safe-suffix>`;
4. create a private temporary state directory and base64 32-byte master key;
5. export the preview `DATABASE_URL`, encryption key, `JOBCRON_NO_OPEN=1`, and
   loopback host;
6. build and run `cmd/jobcron` without `--db`;
7. terminate the child, drop only its unique database, and remove the key/state
   on normal exit or signals; and
8. with `JOBCRON_PREVIEW_KEEP=1`, retain state and print explicit manual cleanup
   commands without executing them.

Reject inherited database URLs that are not the generated loopback preview URL.
Never call Compose `down` because the shared local cluster should remain up.

- [ ] **Step 3: Exercise the real preview user path**

Start the script, submit a profile through the same HTTP form a user uses, then
assert:

- the profile exists only in the generated preview database;
- `jobcron_dev` does not contain that profile; and
- a second simultaneous preview does not contain it.

After stopping the first process, prove its database no longer exists and its
temporary key file is gone.

- [ ] **Step 4: Run tests and commit**

```sh
sh -n scripts/preview-interactive.sh
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' \
  go test ./scripts -run Preview -count=1
git add scripts/preview-interactive.sh scripts/preview_interactive_test.go
git commit -m "feat(preview): use disposable postgres databases"
```

### Task 5: Document managed local lifecycle without activating cutover

**Files:**

- Modify: `deploy/local/README.md`
- Modify: `README.md`
- Modify: `README.ko.md`

- [ ] **Step 1: Update local operator documentation**

Document:

- automatic `jobcron-local` startup;
- the fixed loopback URL and named volume;
- explicit `DATABASE_URL` bypass;
- Docker/Compose prerequisites and diagnostics;
- ordinary stop versus explicit reset; and
- preview isolation and `JOBCRON_PREVIEW_KEEP=1` cleanup.

Do not yet claim SQLite is unreachable or remove the migration warning. State
that the final writable cutover is gated on Slice 4's verified import.

- [ ] **Step 2: Verify English/Korean statements agree**

```sh
rg -n '55432|jobcron-local|DATABASE_URL|preview|미리보기' \
  README.md README.ko.md deploy/local/README.md
```

Manually compare the prerequisites, bypass rule, and reset warning.

- [ ] **Step 3: Commit local documentation**

```sh
git add README.md README.ko.md deploy/local/README.md
git diff --cached --check
git diff --cached
gitleaks git --staged --redact --no-banner
git commit -m "docs: explain managed local postgres lifecycle"
```

### Task 6: Prove the Slice 3 completion contract

**Files:**

- Modify as needed: `.github/workflows/ci.yml`

- [ ] **Step 1: Add CI coverage for contracts and preview cleanup**

Start PostgreSQL 18 on `55432`, set `JOBCRON_TEST_POSTGRES_URL`, run localdb
contract tests, and run the preview isolation test. Use unique databases; do not
drop the shared service volume.

- [ ] **Step 2: Run full verification**

```sh
test -z "$(gofmt -l .)"
go vet ./...
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' go test ./... -count=1
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' \
  go test -race ./... -count=1
docker compose -p jobcron-local -f deploy/local/compose.yaml config
```

Expected: PASS and rendered Compose contract matches the embedded definition.

- [ ] **Step 3: Confirm the activation gate remains intact**

```sh
go run ./cmd/jobcron --help 2>&1 | rg -- '--db'
rg -n 'OpenSQLiteAt|storage\.Open\(|storage\.OpenAt' cmd internal/storage
```

Expected in Slice 3: the legacy path is still present solely because Slice 4
has not completed. Record this as a dependency, not as completed convergence.

- [ ] **Step 4: Review and commit CI changes**

```sh
git diff --check
git add .github/workflows/ci.yml
git commit -m "ci: verify managed postgres and preview isolation"
```

Skip the commit if CI already covers every required command.

## Rollback Boundary

Stopping the app does not stop or delete PostgreSQL. A code rollback may ignore
the managed service while existing SQLite remains the fallback. Never remove
`jobcron-postgres18-cluster` automatically. If a test preview is interrupted,
drop only the printed `jobcron_preview_*` database after confirming its exact
name.
