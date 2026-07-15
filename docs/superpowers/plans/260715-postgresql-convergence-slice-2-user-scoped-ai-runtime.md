# PostgreSQL Convergence Slice 2: User-Scoped AI Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the mutable server-wide AI provider, cache, and usage state
with one encrypted, operation-scoped runtime per user.

**Architecture:** PostgreSQL migration `0015` assigns existing global AI rows
only when ownership is unambiguous, then makes every score and usage query
require `user_id`. The server decrypts a credential once at the start of a
scrape, rerate, render, or rescore operation and passes an immutable `AIRuntime`
through that operation. Profile and credential writes commit together before a
new provider is constructed.

**Tech Stack:** Go 1.26, PostgreSQL 18, `database/sql`, pgx, AES-256-GCM,
`net/http`, Go tests with the race detector.

## Global Constraints

- Start from completed Slice 1, including migration `0014`,
  `credential.Cipher`, master-key validation, and encrypted credential CRUD.
- Follow the approved
  [convergence specification](../specs/260714-postgresql-local-convergence-user-ai-credentials.md).
- Do not add plaintext credential fields to storage structs, logs, errors,
  templates, fixtures, or snapshots.
- Keep `ai_extractions` global. It contains posting-derived facts, not user
  preferences.
- Do not add scheduler fan-out. The scheduler may run paid AI only for exactly
  one owner.
- Preserve the legacy SQLite reader for Slice 4. Do not remove the normal
  SQLite startup fallback in this slice.
- Treat Slice 2 as a non-release migration checkpoint. A legacy SQLite launch
  remains readable and supports rule-based scoring, but paid AI is off because
  SQLite has no positive account ID for the user-scoped runtime. Do not invent
  a `user_id = 0` exception or continue reading `ai_keys.json` in the server.
  Leave the SQLite data and legacy key file untouched for Slice 4 import.
- Use disposable PostgreSQL schemas for integration tests. Never reset
  `jobcron_dev`, RDS, or the retained SQLite source.
- Keep commits local. Never push.

## Slice Completion Contract

This slice is complete when authenticated request paths cannot observe or spend
another user's AI state, the server owns no mutable global provider/model/budget
fields, profile plus credential saves are atomic, and the required storage and
server isolation tests pass against PostgreSQL 18. The retained SQLite fallback
must still render and rescore with rules only, without reading or changing the
legacy key file.

---

### Task 1: Add the safe user-scoped AI-state migration

**Files:**

- Create: `internal/storage/postgres_migrations/0015_user_scoped_ai_state.sql`
- Modify: `internal/storage/postgres_integration_test.go`
- Test: `internal/storage/postgres_integration_test.go`

- [ ] **Step 1: Write failing one-owner and ambiguous-owner migration tests**

Add tests that apply migrations only through `0014`, seed the pre-`0015`
tables, then apply `0015`:

```go
func TestUserScopedAIMigrationAssignsGlobalRowsToSoleOwner(t *testing.T)
func TestUserScopedAIMigrationRejectsRowsWithNoOwner(t *testing.T)
func TestUserScopedAIMigrationRejectsRowsWithMultipleOwners(t *testing.T)
func TestUserScopedAIMigrationAllowsEmptyTablesWithoutOwner(t *testing.T)
```

The success test must compare every seeded `ai_scores` and `ai_usage` value,
not only row counts. Each rejection test must prove the old tables and rows
remain intact and migration version `15` was not recorded.

- [ ] **Step 2: Run the focused tests and confirm the expected failure**

```sh
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' \
  go test ./internal/storage -run 'UserScopedAIMigration' -count=1
```

Expected: failure because migration `0015` does not exist.

- [ ] **Step 3: Implement migration `0015`**

The migration must:

1. count users and detect whether either legacy AI table contains rows;
2. raise an actionable PostgreSQL exception when rows exist and user count is
   not exactly one;
3. create `ai_scores_user_scoped` and `ai_usage_user_scoped` with the schema in
   the approved spec;
4. copy legacy rows to the sole user's ID when one exists;
5. compare source and destination counts before dropping anything;
6. drop the old tables, rename replacements, and create
   `idx_ai_scores_user_latest`; and
7. leave empty replacement tables when no users and no legacy rows exist.

- [ ] **Step 4: Run migration tests and the PostgreSQL storage suite**

```sh
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' \
  go test ./internal/storage -run 'UserScopedAIMigration|PostgresMigrations' \
  -count=1
```

Expected: PASS with no skipped PostgreSQL tests.

- [ ] **Step 5: Commit the migration**

```sh
git add internal/storage/postgres_migrations/0015_user_scoped_ai_state.sql \
  internal/storage/postgres_integration_test.go
git commit -m "feat(storage): scope postgres AI state by user"
```

### Task 2: Require user IDs in AI score and usage storage APIs

**Files:**

- Modify: `internal/storage/ai_scores.go`
- Modify: `internal/storage/ai_scores_test.go`
- Modify: `internal/storage/ai_usage.go`
- Modify: `internal/storage/ai_usage_test.go`

- [ ] **Step 1: Change tests to the target public contracts**

Every public method must accept a positive `userID` immediately after `ctx`:

```go
func (s *Store) UpsertAIScore(
    ctx context.Context,
    userID, postingID int64,
    aiInputHash, aiVersion string,
    delta ai.Delta,
    computedAt time.Time,
) error

func (s *Store) AIScore(
    ctx context.Context,
    userID, postingID int64,
    aiInputHash, aiVersion string,
) (ai.Delta, bool, error)

func (s *Store) LatestAIScore(
    ctx context.Context,
    userID, postingID int64,
    aiVersion string,
) (ai.Delta, bool, error)

func (s *Store) AIScoresByPostingID(
    ctx context.Context,
    userID int64,
    aiInputHash, aiVersion string,
) (map[int64]ai.Delta, error)

func (s *Store) LatestAIScoresByPostingID(
    ctx context.Context,
    userID int64,
    aiVersion string,
) (map[int64]ai.Delta, error)

func (s *Store) LatestAIScoresAnyVersionByPostingID(
    ctx context.Context,
    userID int64,
) (map[int64]ai.Delta, error)

func (s *Store) AddAIUsage(
    ctx context.Context,
    userID int64,
    day string,
    inputTokens, outputTokens int,
) error

func (s *Store) AIUsageForDay(
    ctx context.Context,
    userID int64,
    day string,
) (inputTokens, outputTokens int, err error)

func (s *Store) AIUsageForMonth(
    ctx context.Context,
    userID int64,
    month string,
) (inputTokens, outputTokens int, err error)
```

Retain the existing latest-score pruning limit, but include `user_id` in its
partition, selection, and deletion predicates.

- [ ] **Step 2: Add cross-user isolation tests**

Add at minimum:

```go
func TestAIScoresAreIsolatedByUser(t *testing.T)
func TestAIScorePruningDoesNotDeleteAnotherUsersRows(t *testing.T)
func TestAIUsageIsIsolatedByUserAndMonth(t *testing.T)
func TestAIStorageRejectsNonPositiveUserID(t *testing.T)
```

Use two real PostgreSQL user rows and the same posting/hash/version/day. Assert
exact values for both users after writes and pruning.

- [ ] **Step 3: Run the focused tests and confirm compile failures**

```sh
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' \
  go test ./internal/storage -run 'AIScore|AIUsage' -count=1
```

Expected: compile failures until method signatures and SQL predicates change.

- [ ] **Step 4: Implement the user-scoped queries**

Reject `userID <= 0`. Include `user_id` in every insert conflict key, lookup,
latest fallback, prune, daily debit, and monthly sum. Do not use
`firstUserID` inside these methods.

The SQLite branch may exist only as a narrow legacy-source compatibility path
until Slice 4; it must never be selected by a PostgreSQL store or hide a missing
PostgreSQL `user_id` predicate.

- [ ] **Step 5: Run storage tests and commit**

```sh
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' \
  go test ./internal/storage -run 'AIScore|AIUsage' -count=1
git add internal/storage/ai_scores.go internal/storage/ai_scores_test.go \
  internal/storage/ai_usage.go internal/storage/ai_usage_test.go
git commit -m "refactor(storage): require users for AI scores and usage"
```

### Task 3: Add atomic profile-and-credential persistence

**Files:**

- Modify: `internal/storage/profile.go`
- Modify: `internal/storage/ai_credentials.go`
- Create: `internal/storage/profile_credentials_test.go`

- [ ] **Step 1: Write transaction tests**

Add these target cases:

```go
func TestSaveProfileAndCredentialForUserCommitsBoth(t *testing.T)
func TestSaveProfileAndCredentialForUserBlankCredentialKeepsExisting(t *testing.T)
func TestSaveProfileAndCredentialForUserRollsBackBothOnProfileFailure(t *testing.T)
func TestSaveProfileAndCredentialForUserRollsBackBothOnCredentialFailure(t *testing.T)
```

The rollback assertions must compare the pre-call profile JSON/hash and the
exact pre-call ciphertext/nonce/version.

- [ ] **Step 2: Introduce the storage transaction contract**

```go
func (s *Store) SaveProfileAndCredentialForUser(
    ctx context.Context,
    userID int64,
    canonicalJSON string,
    credential *EncryptedAICredential,
) (hash string, changed bool, err error)
```

`credential == nil` means leave the existing credential row untouched. A
non-nil credential must match `userID`. Refactor the existing SQL bodies into
transaction-capable private helpers rather than nesting transactions.

- [ ] **Step 3: Run tests, implement, and rerun**

```sh
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' \
  go test ./internal/storage -run 'ProfileAndCredential' -count=1
```

Expected before implementation: undefined method. Expected after: PASS.

- [ ] **Step 4: Commit the atomic save boundary**

```sh
git add internal/storage/profile.go internal/storage/ai_credentials.go \
  internal/storage/profile_credentials_test.go
git commit -m "feat(storage): save profiles and AI credentials atomically"
```

### Task 4: Replace global AI configuration with `AIRuntime`

**Files:**

- Modify: `internal/server/server.go`
- Modify: `internal/server/server_test.go`
- Modify: `internal/server/ai_config_test.go`
- Modify: `cmd/jobcron/main.go`
- Modify: `cmd/jobcron/main_test.go`

- [ ] **Step 1: Add runtime-resolution tests**

Cover two users with different encrypted Anthropic keys, AI disabled, missing
credential, wrong master key, moved ciphertext, unknown encryption version,
and provider-construction failure. Provider test doubles must expose only a
non-secret key fingerprint so assertions never print keys.

- [ ] **Step 2: Define the immutable operation runtime**

```go
type AIRuntime struct {
    UserID          int64
    Provider        ai.Provider
    Version         string
    RunTokenCap     int
    DailyTokenCap   int
    MonthlyTokenCap int
    PerCallCap      int
}

func (s *Server) aiRuntimeForUser(
    ctx context.Context,
    userID int64,
) (*AIRuntime, error)
```

The `Server` may retain one immutable `credential.Cipher`. Add the new resolver
without switching call sites yet; Task 5 removes `ai`, `aiModel`, `aiVersion`,
all four global budget fields, `aiKeysPath`, `SetAIKeysPath`, `keysPath`, and
`ReconfigureAI` in one buildable cutover.

Add an explicit cipher injection point used once during process construction:

```go
func (s *Server) SetCredentialCipher(c credential.Cipher)
```

Production and local startup must set it before handlers or the scheduler run.
Tests that never resolve AI may omit it; resolving an encrypted row without it
returns a stable non-secret error.

- [ ] **Step 3: Implement single-decrypt runtime resolution**

Load `ProfileForUser`, normalize the selected provider, read exactly that
`user_ai_credentials` row, decrypt once, build `ai.Provider`, derive limits,
and return the runtime. Return `nil, nil` when AI is off or no row exists.
Never cache plaintext or the provider on `Server`.

- [ ] **Step 4: Wire the master cipher in `cmd/jobcron`**

In production, construct the cipher from `cfg.CredentialEncryptionKey`. Outside
production, use that configured key when present or call
`credential.LoadOrCreateLocalMasterKey` before constructing the cipher. Pass the
cipher to the server. Keep startup `ReconfigureAI` only until Task 5 performs
the complete runtime cutover.

- [ ] **Step 5: Run focused tests and commit**

```sh
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' \
  go test ./internal/server ./cmd/jobcron -run 'AIRuntime|ConfiguredStore' \
  -count=1
git add internal/server/server.go internal/server/server_test.go \
  internal/server/ai_config_test.go cmd/jobcron/main.go cmd/jobcron/main_test.go
git commit -m "refactor(server): resolve AI runtime per user"
```

### Task 5: Cut over every AI call site to one operation runtime

**Files:**

- Modify: `internal/server/server.go`
- Modify: `internal/server/rerate.go`
- Modify: `internal/server/handlers.go`
- Modify: `internal/server/archive.go`
- Modify: `internal/server/bookmarks.go`
- Modify: `internal/server/rerate_status.go`
- Modify: `internal/server/ai_scrape_test.go`
- Modify: `internal/server/ai_rerate_test.go`
- Modify: `internal/server/production_user_scope_test.go`

- [ ] **Step 1: Make operation boundaries explicit**

Change internal entry points so `userID` is required and `runtime` is resolved
once, then passed downward. Representative target shapes:

```go
func (s *Server) runScrapeWithHistory(
    ctx context.Context,
    trigger string,
    emit func(string, string),
    userID int64,
    runtime *AIRuntime,
) (ScrapeResult, error)

func (s *Server) runRerate(
    ctx context.Context,
    surface string,
    emit func(string, string),
    userID int64,
    runtime *AIRuntime,
) (rerateSummary, error)

func (s *Server) scoreAll(
    ctx context.Context,
    userID int64,
    runtime *AIRuntime,
) (int, error)
```

Remove variadic user IDs from AI-capable helpers. Make `aiBudget` carry
`userID` and limits from `AIRuntime`; usage reads and debits must use that ID.

At the end of this step, remove all global provider/model/budget fields and
`ReconfigureAI` together. Replace startup global configuration and rescoring
by temporarily skipping the cache-only startup heal. Task 6 restores that heal
through the exact-owner lookup at the same time as scheduler ownership. Do not
retain a first-row fallback merely to keep startup rescoring active.

- [ ] **Step 2: Update cache and provider call sites**

Use `runtime.Provider` and `runtime.Version` only when runtime is non-nil. Pass
`userID` into every AI-score and usage method. Keep global
`AIExtractionsByPostingID` calls unchanged.

For the transitional SQLite path, resolve no `AIRuntime`, make no provider call,
and preserve the existing cached non-secret rows. Add a regression test proving
the app can render and perform rule-based rescoring without reading or changing
the legacy `ai_keys.json` file.

- [ ] **Step 3: Add an exact decrypt-count test**

Wrap the cipher in a test spy and perform a multi-row rerate. Assert `Open` was
called once for the complete operation, not once per row or helper.

- [ ] **Step 4: Add two-user concurrency tests**

Run user A and B rerates concurrently with different provider spies. Prove:

- each spy receives only its user's calls;
- scores for the same posting remain distinct;
- daily and monthly usage remain distinct; and
- race detection reports no server field race.

- [ ] **Step 5: Run server tests and commit**

```sh
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' \
  go test -race ./internal/server -run 'AI|UserScope|Rerate|Scrape' -count=1
git add internal/server
git commit -m "refactor(server): pass user AI runtime through operations"
```

### Task 6: Make profile save and scheduler ownership safe

**Files:**

- Modify: `internal/server/handlers.go`
- Modify: `internal/server/ai_provider_switch_test.go`
- Modify: `internal/server/scheduler.go`
- Modify: `internal/server/scheduler_test.go`
- Modify: `internal/storage/users.go`
- Modify: `internal/storage/users_test.go`

- [ ] **Step 1: Add sole-owner lookup**

```go
func (s *Store) SoleOwnerUserID(
    ctx context.Context,
) (userID int64, ok bool, err error)
```

Return `ok=false, err=nil` for zero users and a stable error for more than one.
Never fall back to the first row.

- [ ] **Step 2: Rewrite profile save in the required order**

Resolve the authenticated user before reading `ai_key`. If the key is blank,
pass a nil credential. If nonblank, normalize the provider and encrypt before
opening the transaction. Call `SaveProfileAndCredentialForUser`, commit, resolve
a fresh runtime, and rescore only that user.

Delete `saveAIKey` and runtime uses of `internal/ai/keys.go`. Keep `LoadKeys`
available only for Slice 4's legacy importer.

- [ ] **Step 3: Add handler transaction and blank-key tests**

Test successful new key, successful profile-only update, encryption failure,
profile failure after key preparation, and user A save while user B's profile,
credential, scores, and runtime remain unchanged.

- [ ] **Step 4: Make scheduler ownership explicit**

At the start of each scheduled run, call `SoleOwnerUserID`. With exactly one
owner, resolve one runtime and run the scrape. With zero or multiple users,
record a skipped scrape with a non-secret operator error and perform no paid AI
call.

Use the same exact-owner decision for startup cache-only rescoring. One owner is
rescored with one resolved runtime; zero owners is a no-op; ambiguous ownership
logs a non-secret operator error and skips rather than guessing.

- [ ] **Step 5: Run focused tests and commit**

```sh
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' \
  go test -race ./internal/server ./internal/storage \
  -run 'Profile|ProviderSwitch|Scheduler|SoleOwner' -count=1
git add internal/server/handlers.go internal/server/ai_provider_switch_test.go \
  internal/server/scheduler.go internal/server/scheduler_test.go \
  internal/storage/users.go internal/storage/users_test.go
git commit -m "feat(server): save and schedule user AI state safely"
```

### Task 7: Prove the Slice 2 completion contract

**Files:**

- Modify as needed: `internal/server/production_user_scope_test.go`
- Modify as needed: `internal/server/ai_injection_test.go`
- Modify as needed: `internal/storage/postgres_integration_test.go`

- [ ] **Step 1: Run formatting, static analysis, and all PostgreSQL tests**

```sh
test -z "$(gofmt -l .)"
go vet ./...
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' go test ./... -count=1
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' \
  go test -race ./... -count=1
```

Expected: PASS and no PostgreSQL integration skip.

- [ ] **Step 2: Run secret-boundary checks**

```sh
rg -n 'aiKeysPath|SetAIKeysPath|ReconfigureAI|s\.ai(Model|Version|Run|Daily|Monthly|PerCall)?' \
  internal/server cmd/jobcron
rg -n 'ai_keys\.json' internal/server cmd/jobcron
```

Expected: no runtime-global AI configuration and no server key-file access.

- [ ] **Step 3: Re-read the cumulative diff**

```sh
git diff HEAD~6..HEAD -- internal cmd/jobcron
```

Confirm every paid call receives an explicit user/runtime, every user-owned AI
query includes `user_id`, and no error contains key material.

- [ ] **Step 4: Commit any final test-only corrections**

```sh
git add internal cmd/jobcron
git commit -m "test: prove per-user AI runtime isolation"
```

Skip this commit when the tree is already clean.

## Rollback Boundary

Before any production use, application code may roll back while keeping
migration `0015`; the new tables are a compatible superset for one owner. Do not
reverse `0015` by copying rows back when more than one user has written AI
state. Preserve the credential master key and PostgreSQL backup throughout the
rollback window.
