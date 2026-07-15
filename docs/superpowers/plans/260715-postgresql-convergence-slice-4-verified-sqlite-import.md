# PostgreSQL Convergence Slice 4: Verified SQLite Import Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the existing copy helper into a safe, repeatable, verified
one-time migration, then remove SQLite from the normal writable runtime.

**Architecture:** The importer locks and snapshots the legacy SQLite source,
fingerprints the immutable snapshot, computes a collision-aware plan, and is a
dry run unless `--apply` is supplied. One PostgreSQL transaction copies all
eight data categories plus an optional encrypted credential and records an
import ledger row. A post-commit readback verifies owner-scoped values. Only
after those gates pass does the normal `jobcron` command become PostgreSQL-only.

**Tech Stack:** Go 1.26, modernc SQLite online backup API, SHA-256, PostgreSQL
18, `database/sql`, pgx, AES-256-GCM, Go integration tests.

## Global Constraints

- Start after Slices 2 and 3 pass locally.
- Treat the exact prior-slice commit named in the bead as authoritative, not
  `origin/main`. A separate worker clone must fetch that commit from the Mayor
  rig and verify its hash before editing.
- Do not use the normal `gt done` path because it submits an MR. Commit locally,
  report the exact tip and verification evidence, then defer so Mayor can fetch
  the clone and integrate without pushing.
- Follow the approved
  [convergence specification](../specs/260714-postgresql-local-convergence-user-ai-credentials.md).
- Preserve Slice 2's operation-scoped AI runtime, scheduler lock ordering,
  single render snapshot, missing-score omission, and rule-only recovery
  behavior. This slice changes persistence activation, not those contracts.
- The source SQLite database and optional `ai_keys.json` are read-only inputs.
  Never modify, rename, truncate, or delete them.
- The importer is dry-run by default. Only `--apply` may write PostgreSQL.
- Preserve the pre-created owner's ID and password hash. Never import sessions,
  passwords, or a replacement owner.
- Obtain the credential master key from the same config rules as the app, never
  from a CLI argument.
- Never print URLs, owner identity, source paths, key bytes, ciphertext, or key
  file contents. Reports may print a source SHA-256 fingerprint and counts.
- There is no force-overwrite flag.
- Use disposable PostgreSQL schemas and temporary SQLite fixtures in tests.
- Keep commits local. Never push.
- Preserve Slice 3's internal `JOBCRON_STRICT_PORT=1` preview contract: the
  preview exports it for both bootstrap and the main child so the requested
  port is attempted exactly once. Do not promote it to a public CLI flag;
  ordinary startup keeps the existing ten-port fallback.

## Slice Completion Contract

This slice is complete when dry-run, atomic apply, same-fingerprint verification,
different-source refusal, credential encryption, post-commit value checks, and
rollback tests pass; then `jobcron` no longer accepts `--db` or opens SQLite and
`jobcron-import --sqlite` is the only legacy SQLite entry point.

---

### Task 1: Add the immutable import ledger

**Files:**

- Create: `internal/storage/postgres_migrations/0016_local_data_imports.sql`
- Modify: `internal/storage/postgres_integration_test.go`

- [ ] **Step 1: Write failing schema and constraint tests**

Add tests for table existence, owner cascade, 64-character fingerprint check,
same owner/fingerprint uniqueness, and the ability for different owners to have
separate ledger rows.

- [ ] **Step 2: Add migration `0016`**

Use this contract:

```sql
CREATE TABLE local_data_imports (
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_sha256   TEXT NOT NULL CHECK (length(source_sha256) = 64),
    source_counts   JSONB NOT NULL,
    imported_counts JSONB NOT NULL,
    completed_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, source_sha256)
);
```

- [ ] **Step 3: Run migration tests and commit**

```sh
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' \
  go test ./internal/storage -run 'LocalDataImports|PostgresMigrations' -count=1
git add internal/storage/postgres_migrations/0016_local_data_imports.sql \
  internal/storage/postgres_integration_test.go
git commit -m "feat(storage): record verified local data imports"
```

### Task 2: Snapshot and fingerprint the SQLite source safely

**Files:**

- Create: `internal/sqlitesnapshot/snapshot.go`
- Create: `internal/sqlitesnapshot/snapshot_test.go`

- [ ] **Step 1: Define the snapshot result**

```go
type Snapshot struct {
    Path   string
    SHA256 string
}

func Create(ctx context.Context, sourcePath, workDir string) (Snapshot, error)
```

`Path` is a private working copy that the caller removes. `SHA256` is lowercase
hex of that copy's complete bytes.

- [ ] **Step 2: Write source-safety tests**

Cover:

```go
func TestCreateRejectsMissingOrNonRegularSource(t *testing.T)
func TestCreateRejectsActiveWriter(t *testing.T)
func TestCreateIncludesCommittedWALRows(t *testing.T)
func TestCreateProducesStableFingerprintForSameLogicalSource(t *testing.T)
func TestCreateSnapshotPassesQuickCheck(t *testing.T)
func TestCreateNeverChangesSourceFiles(t *testing.T)
func TestCreateRemovesPartialSnapshotOnFailure(t *testing.T)
```

Capture source database, `-wal`, and `-shm` hashes before and after each call.

- [ ] **Step 3: Implement a locked online backup**

Use the modernc connection's supported online backup boundary:

```go
type backuper interface {
    NewBackup(string) (*sqlite.Backup, error)
}
```

Acquire a no-wait SQLite write lock before snapshotting so a live writer causes
a refusal rather than a fuzzy copy. Keep the lock for the backup operation,
finish and close the backup, then open only the snapshot and run
`PRAGMA quick_check`. Hash the snapshot after all handles are closed.

Never fall back to raw file copying.

- [ ] **Step 4: Run tests and commit**

```sh
go test ./internal/sqlitesnapshot -count=1
git add internal/sqlitesnapshot
git commit -m "feat(import): create immutable sqlite snapshots"
```

### Task 3: Replace importer flags and build a collision-aware dry run

**Files:**

- Modify: `cmd/jobcron-import/main.go`
- Modify: `cmd/jobcron-import/main_test.go`

- [ ] **Step 1: Replace the option contract**

```go
type importOptions struct {
    sqlitePath  string
    postgresURL string
    ownerEmail  string
    aiKeysPath  string
    apply       bool
    out         io.Writer
}
```

Remove `--dry-run`; dry-run is the default. Add `--apply` and optional
`--ai-keys`. Require explicit `--owner-email` rather than silently targeting a
fallback account.

- [ ] **Step 2: Introduce structured counts and plan output**

```go
type categoryCounts struct {
    Profile       int `json:"profile"`
    Postings      int `json:"postings"`
    Scores        int `json:"scores"`
    Bookmarks     int `json:"bookmarks"`
    NotInterested int `json:"not_interested"`
    AIExtractions int `json:"ai_extractions"`
    AIScores      int `json:"ai_scores"`
    AIUsage       int `json:"ai_usage"`
    AIProviders   int `json:"ai_providers"`
}

type importPlan struct {
    SourceSHA256 string
    Source       categoryCounts
    Target       categoryCounts
    Collisions   categoryCounts
}
```

The output must name all eight categories, credential provider count,
fingerprint, and collisions. It must redact the URL, email, paths, and all
credential material.

- [ ] **Step 3: Write dry-run behavior tests**

Add at minimum:

```go
func TestImportDefaultsToDryRunAndWritesNothing(t *testing.T)
func TestImportDryRunReportsAllCategoriesFingerprintAndCollisions(t *testing.T)
func TestImportRequiresExistingSoleOwner(t *testing.T)
func TestImportRefusesOwnerEmailMismatch(t *testing.T)
func TestImportReportDoesNotContainSecretsOrPrivateInputs(t *testing.T)
```

The owner lookup must find exactly the supplied existing account. It must not
call `ensureOwnerUser` or create any user.

- [ ] **Step 4: Implement planning against the immutable snapshot**

Open only `Snapshot.Path`. Count and validate every category, parse the optional
legacy key file with the narrowly scoped `ai.LoadKeys` reader, normalize known
providers, and compute target collisions scoped to the owner. Reject more than
one legacy credential for the same provider after normalization.

- [ ] **Step 5: Run tests and commit**

```sh
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' \
  go test ./cmd/jobcron-import -run 'DryRun|Requires|Mismatch|Report' -count=1
git add cmd/jobcron-import/main.go cmd/jobcron-import/main_test.go
git commit -m "refactor(import): make verified dry run the default"
```

### Task 4: Make apply atomic and encrypt the optional credential

**Files:**

- Modify: `cmd/jobcron-import/main.go`
- Modify: `cmd/jobcron-import/main_test.go`
- Modify: `internal/ai/keys.go`

- [ ] **Step 1: Update copy helpers to the final schema**

Pass `ownerID` into `copyAIScores` and `copyAIUsage`. Insert their rows with
`user_id` and conflict on the user-scoped primary keys. Keep
`copyAIExtractions` global.

Slice 2 discovery: migration `0015` immediately invalidated the existing
importer's old conflict targets, so the `ownerID` arguments and user-scoped
`ai_scores` / `ai_usage` writes landed as a build-and-test compatibility fix in
Slice 2. In this slice, verify those helpers against the full apply contract;
do not reintroduce the pre-`0015` conflict keys. SQLite fixture setup still
passes a positive sentinel user ID only because the shared storage API now
requires one—the legacy source tables themselves remain unscoped.

- [ ] **Step 2: Load the importer master key without a CLI flag**

Use `JOBCRON_CREDENTIAL_ENCRYPTION_KEY` when present. Outside production, use
`credential.LoadOrCreateLocalMasterKey`; production-mode invocation must fail
closed if the configured key is absent or malformed.

For an import into production RDS, the trusted shell must supply the exact same
`JOBCRON_CREDENTIAL_ENCRYPTION_KEY` that the production app will use. The value
is never passed as a flag or printed.

For each optional legacy provider key, call `Cipher.Seal(ownerID, provider,
plaintext)` and write only `storage.EncryptedAICredential` values.

- [ ] **Step 3: Write transaction failure tests**

Inject a failure after each category, after credential upsert, during count
comparison, and before ledger insert. For every case assert that all target
category counts, owner password hash, credential row, and ledger remain exactly
at their pre-call values.

- [ ] **Step 4: Implement one transaction in dependency order**

The `--apply` path must:

1. lock the owner/import decision against concurrent imports;
2. copy postings before rows that reference postings;
3. copy profile, rule scores, bookmarks, hidden jobs, global extractions,
   user-scoped AI scores, and user-scoped usage;
4. upsert the optional encrypted credential;
5. reset only required PostgreSQL sequences;
6. compare transaction-visible counts and representative values;
7. insert `local_data_imports`; and
8. commit once.

Do not use `GREATEST` merging for one-time usage import; the clean-target plan
must prove no ambiguous collision, then copy exact source totals.

- [ ] **Step 5: Run apply tests and commit**

```sh
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' \
  go test ./cmd/jobcron-import -run 'Apply|Rollback|Credential|Password' \
  -count=1
git add cmd/jobcron-import/main.go cmd/jobcron-import/main_test.go \
  internal/ai/keys.go
git commit -m "feat(import): atomically copy and encrypt legacy state"
```

### Task 5: Add ledger idempotency and post-commit verification

**Files:**

- Modify: `cmd/jobcron-import/main.go`
- Modify: `cmd/jobcron-import/main_test.go`

- [ ] **Step 1: Add exact-value verification**

After commit, reconnect and compare owner-scoped target data to the snapshot.
Verification must include:

- all category counts;
- canonical profile JSON and hash;
- at least one posting's identity fields;
- one rule score total and breakdown;
- one bookmark and one hidden row;
- one global extraction;
- one user-scoped AI score and delta JSON;
- every imported daily usage total; and
- credential provider presence plus successful decrypt equality without
  printing plaintext.

- [ ] **Step 2: Add idempotency and mismatch tests**

```go
func TestImportSameFingerprintVerifiesWithoutWrites(t *testing.T)
func TestImportDifferentFingerprintForImportedOwnerIsRefused(t *testing.T)
func TestImportPostCommitMismatchFailsVerification(t *testing.T)
func TestImportDoesNotModifySourceOrLegacyKeyFile(t *testing.T)
```

The same-fingerprint path must emit `already imported`, perform readback only,
and preserve target row timestamps. The different-fingerprint path must fail
before any copy call.

- [ ] **Step 3: Implement ledger decision rules**

- Existing `(owner, fingerprint)`: verify and exit success.
- Any different ledger fingerprint for owner: refuse.
- Owner has nonempty target state without a ledger: report collisions and
  refuse apply.
- Empty target and no ledger: allow apply.

If post-commit verification fails, return a distinct error telling the operator
to preserve both systems and restore the documented PostgreSQL backup. Never
attempt an automatic compensating delete.

- [ ] **Step 4: Run importer integration tests and commit**

```sh
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' \
  go test ./cmd/jobcron-import -count=1
git add cmd/jobcron-import/main.go cmd/jobcron-import/main_test.go
git commit -m "feat(import): verify and ledger completed imports"
```

### Task 6: Activate the PostgreSQL-only writable runtime

**Files:**

- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `cmd/jobcron/main.go`
- Modify: `cmd/jobcron/main_test.go`
- Modify: `internal/storage/store.go`
- Modify: `internal/storage/store_test.go`
- Modify: `scripts/preview-interactive.sh`
- Modify: `README.md`
- Modify: `README.ko.md`
- Modify: `deploy/local/README.md`

- [ ] **Step 1: Write the final startup contract tests**

Cover:

```go
func TestLoadRejectsDBFlag(t *testing.T)
func TestLoadIgnoresNoLegacyDatabaseEnvironment(t *testing.T)
func TestOpenConfiguredStoreAlwaysUsesPostgres(t *testing.T)
func TestNormalLocalStartupDoesNotCreateSQLite(t *testing.T)
func TestExplicitDatabaseURLNeverInvokesDocker(t *testing.T)
func TestManagedLocalStartupUsesStablePositiveUserID(t *testing.T)
func TestManagedLocalStartupReusesImportedOwner(t *testing.T)
func TestExplicitURLRefusesAmbiguousUsers(t *testing.T)
func TestManagedLocalStartupRequiresHostTCPReachability(t *testing.T)
```

Use a temporary OS config root and assert no `jobs.db`, SQLite journal, WAL, or
SHM file appears.

- [ ] **Step 2: Remove normal SQLite selection**

Delete `Config.DBPath`, the `--db` flag, `JOBCRON_DB` handling,
`openConfiguredStore` SQLite branches, and normal calls to `storage.Open` or
`storage.OpenAt`. Keep `storage.OpenSQLiteAt` only for the importer and its
fixtures. Stop moving or preparing legacy application data during normal app
startup. Activate Slice 3's `resolvePostgresRuntime` for the ordinary no-URL
local launch in this same commit and pass its positive local owner ID into the
server. For the fixed managed database, inspect `users` after migrations: create
the fixed synthetic local owner only when the table is empty; reuse one existing
positive owner unchanged (especially the verified imported owner); and refuse
multiple users. An explicit URL must already contain exactly one user and may
never create one.

The managed startup gate must prove a real host TCP connection to
`127.0.0.1:55432`; Compose health, container labels, or Docker `HostConfig` are
not sufficient. Preserve state and print actionable `ps` and `logs postgres`
diagnostics on failure. The startup helper itself must never run `down`, remove
or recreate a container, or remove a volume.

If the first Compose create ran while `55432` was occupied, the canonical
`jobcron-local-postgres-1` container may exist without a usable published port.
After the port is free, the documented operator remediation must recreate only
`jobcron-local-postgres-1`. It must preserve the older `local-postgres-1`
container, `local_jobcron-postgres18-cluster`, and
`jobcron-postgres18-cluster`.

After this cutover, no PostgreSQL-backed `Server` path may use the transitional
`userID == 0` SQLite compatibility branch. Keep that branch reachable only from
the importer's legacy-source storage path until its fixtures no longer need it.
Keep `Config.StrictPort` and the strict branch of `cmd/jobcron.listen` while
rewiring startup; preview launch must fail if its requested port becomes busy
after preflight rather than silently serving on a fallback port.

- [ ] **Step 3: Update user documentation to final truth**

English and Korean docs must state that PostgreSQL is the writable database,
ordinary local startup manages PostgreSQL 18, and legacy SQLite is accepted
only by `jobcron-import --sqlite`. Remove instructions that describe SQLite as
the fallback or preview backend. Keep the preview lifecycle accurate: an atomic
per-user/per-port lock, unrelated-listener refusal, a unique disposable database
and key, non-production mode with scheduling disabled, exact manual cleanup when
`JOBCRON_PREVIEW_KEEP=1`, and no shared Compose `down`.

- [ ] **Step 4: Run runtime and documentation checks**

```sh
go run ./cmd/jobcron --help 2>&1 | tee /tmp/jobcron-help.txt
! rg -- '--db' /tmp/jobcron-help.txt
rg -n 'OpenSQLiteAt' cmd internal | sort
rg -n 'SQLite|sqlite|--db|JOBCRON_DB' README.md README.ko.md deploy/local \
  cmd/jobcron internal/config
```

Expected: `OpenSQLiteAt` appears only in importer/legacy tests; remaining
documentation references describe migration, not normal writes.

- [ ] **Step 5: Run tests and commit the cutover**

```sh
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' go test ./... -count=1
git add internal/config cmd/jobcron internal/storage/store.go \
  internal/storage/store_test.go scripts/preview-interactive.sh \
  README.md README.ko.md deploy/local/README.md
git diff --cached --check
git diff --cached
gitleaks git --staged --redact --no-banner
git commit -m "feat: make postgres the only writable runtime"
```

### Task 7: Ship and verify the importer artifact

**Files:**

- Modify: `.goreleaser.yml`
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Add `jobcron-import` to release builds**

Define a separate GoReleaser build ID and binary. Package it beside `jobcron`
for supported platforms. Keep the app version injection scoped correctly; the
importer does not need a duplicated version variable unless it exposes
`--version`.

- [ ] **Step 2: Extend CI**

CI must start PostgreSQL 18, run all importer integration tests, build both
commands, and run the race suite with `JOBCRON_TEST_POSTGRES_URL` set. Add a
test that release configuration includes `jobcron-import`.

- [ ] **Step 3: Run the full completion gate**

```sh
test -z "$(gofmt -l .)"
go vet ./...
go build ./cmd/jobcron ./cmd/jobcron-import ./cmd/jobcron-user
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' go test ./... -count=1
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' \
  go test -race ./... -count=1
goreleaser check
```

Expected: PASS with no PostgreSQL or importer test skipped.

- [ ] **Step 4: Perform the publication and secret review**

Inspect the complete diff and scan staged content. Confirm fixtures contain
only synthetic keys, URLs, identities, and paths.

```sh
git add .goreleaser.yml .github/workflows/ci.yml
git diff --check
git diff --cached
gitleaks git --staged --redact --no-banner
```

- [ ] **Step 5: Commit release and CI changes**

```sh
git commit -m "build: ship and verify the postgres importer"
```

## Rollback Boundary

Before the import transaction commits, rollback is automatic and the original
SQLite source remains authoritative. After import but before new PostgreSQL
writes, restore the clean PostgreSQL snapshot or disposable volume and rerun
from the same immutable source. After PostgreSQL-only writes begin, do not
switch the app back to SQLite; roll back application code while retaining the
PostgreSQL schema, or restore PostgreSQL and replay explicitly approved data.
