# PostgreSQL Local Convergence And Per-User AI Credentials

**Date:** 2026-07-14
**Status:** Draft for approval
**Scope:** Writable local app, production credential storage, and user-scoped AI state

## Context

Jobcron currently has two writable persistence modes. A normal local launch falls
back to SQLite, while production uses PostgreSQL when `DATABASE_URL` is present.
AI credentials live outside both databases in one global `ai_keys.json`, and the
production container preserves that file with the `jobcron_config` Docker volume.

That split no longer matches the hosted-first product. Account data and AI results
must follow the authenticated user across devices, and one server-wide key file
cannot safely support more than one account. This specification makes PostgreSQL
the only writable application database, migrates existing local state once, and
stores one encrypted AI credential per user and provider.

This work starts before public launch. It supersedes only the timing in
[`260714-hosted-first-local-database-convergence.md`](../decisions/260714-hosted-first-local-database-convergence.md):
the accepted convergence outcome remains, but it is no longer deferred until
after launch.

## Locked Product Decisions

1. The current owner-only account UI remains. Public signup, account recovery,
   organizations, and team accounts are not added here.
2. PostgreSQL storage must nevertheless be safe for multiple user rows. Two test
   users must not share credentials, AI scores, usage budgets, profiles, bookmarks,
   or hidden state.
3. All existing local SQLite state is preserved through a verified one-time
   migration: profile, postings, rule scores, bookmarks, hidden jobs, AI
   extractions, AI scores, AI usage, and the existing AI credential when present.
4. Local PostgreSQL 18 starts automatically through Docker Compose when a normal
   local run has no explicit `DATABASE_URL`.
5. Each user may have at most one encrypted credential for each provider. The UI
   initially supports only `anthropic`; the schema must not require a migration to
   add another validated provider later.

## Verified Current State

Verified against `main` on 2026-07-14.

### Runtime selection

- **Current:** `cmd/jobcron/main.go:144-153` opens PostgreSQL only when
  `DATABASE_URL` is set; `--db` and the default path open SQLite.
- **Gap:** A normal writable launch silently remains on SQLite.

### Local database

- **Current:** `deploy/local/compose.yaml:1-13` provides PostgreSQL 18 on port
  `55432`, but the operator must start it and export the URL manually.
- **Gap:** Local startup is not automatic and has no health check.

### Local preview

- **Current:** `scripts/preview-interactive.sh:12-39` creates a temporary SQLite
  database.
- **Gap:** Preview does not exercise the production persistence backend.

### User state

- **Current:** PostgreSQL migration `0006_user_scoped_state.sql` scopes profiles,
  scores, bookmarks, and hidden state by `user_id`.
- **Gap:** The older global AI tables were not included.

### AI credentials

- **Current:** `internal/ai/keys.go:16-86` stores a provider-to-key JSON map on
  disk. `internal/server/handlers.go:617-635` writes it without a user ID.
- **Gap:** Every account on one server would share and overwrite the same key.

### AI runtime

- **Current:** `internal/server/server.go:114-149` keeps one mutable provider,
  model, version, and budget configuration on `Server`.
- **Gap:** Saving one user's profile changes the AI runtime for every request.

### AI score cache

- **Current:** `internal/storage/ai_scores.go:14-92` keys cached deltas by posting,
  goal hash, and AI version, and prunes without `user_id`.
- **Gap:** One user's stale fallback or pruning can expose or delete another
  user's profile-derived result.

### AI usage

- **Current:** `internal/storage/ai_usage.go:11-62` aggregates usage by day only.
- **Gap:** Token and cost limits are server-global rather than account-scoped.

### Existing importer

- **Current:** `cmd/jobcron-import/main.go` copies eight SQLite data categories in
  one PostgreSQL transaction. Integration tests cover representative data,
  rollback, idempotent upserts, and owner-password preservation.
- **Gap:** It does not create a consistent source snapshot, record a completed
  import, migrate credentials, or verify post-commit counts.

### Production Compose

- **Current:** `deploy/production/compose.yaml:29-30,47-48` mounts the named
  `jobcron_config` volume.
- **Gap:** Credential durability is host-local and not account-scoped.

No similar open GitHub issue was found using the terms `PostgreSQL SQLite AI
credentials`.

## Goals

- Give local, production, and test code one writable SQL behavior: PostgreSQL.
- Preserve the current local user's data without silently mutating the source.
- Store AI credentials encrypted at rest and keyed by the authenticated user.
- Make AI score caches and usage budgets user-scoped.
- Remove `jobcron_config` from the final production application service.
- Keep offline scoring functional when a credential is missing or unreadable.
- Provide a cutover and rollback procedure that never deletes the only copy of
  user data or credentials.

## Non-Goals

- Public signup, multiple owners, password recovery, social login, organizations,
  roles, or an account-management UI.
- Billing users for AI usage or providing a shared Jobcron-funded AI key.
- Supporting more than one credential for the same user and provider.
- Synchronizing two independent self-hosted PostgreSQL installations.
- Automatic master-key rotation or AWS KMS integration. The schema is versioned
  so rotation can be added without changing credential ownership.
- Secure deletion guarantees for an old SQLite or `ai_keys.json` file on SSDs.

## Target Architecture

```text
normal local run ──> ensure PostgreSQL 18 Compose service ──┐
explicit URL ────────────────────────────────────────────────┤
production RDS ─────────────────────────────────────────────┤
                                                           v
                                                  PostgreSQL Store
                                             ┌─────────────┼─────────────┐
                                             v             v             v
                                      user state     AI state      encrypted BYOK
                                      user_id        user_id       user_id+provider
                                             \             |             /
                                              \            |            /
                                               user-scoped AI runtime
                                                         |
                                                         v
                                               authenticated browser
```

The credential encryption master key stays outside PostgreSQL. PostgreSQL holds
only ciphertext. A database dump or RDS snapshot is therefore insufficient to
recover an API key without the separately protected master key.

## Proposed Change

### 1. Make PostgreSQL The Writable Runtime

`openConfiguredStore` must use these rules. The `--db` flag is removed in every
mode.

- **Production:** `DATABASE_URL` is required. Open the supplied PostgreSQL URL.
- **Normal local with `DATABASE_URL`:** Open the supplied PostgreSQL URL and
  never invoke Docker.
- **Normal local without `DATABASE_URL`:** Start or verify the embedded
  PostgreSQL Compose service, then open the fixed local URL.

The normal local URL is:

```text
postgres://postgres@127.0.0.1:55432/jobcron_dev?sslmode=disable
```

Add `internal/localdb` with the canonical PostgreSQL 18 Compose definition and an
`Ensure(ctx)` function. `Ensure` must:

1. run only for a non-production process with no explicit URL;
2. invoke `docker compose` with project name `jobcron-local`;
3. use an explicitly named `jobcron-postgres18-cluster` volume;
4. start the service with `up -d --wait` and a `pg_isready` health check;
5. time out after 60 seconds;
6. return the fixed local URL only after PostgreSQL accepts connections; and
7. return an actionable error when Docker, Compose, the daemon, port `55432`, or
   the health check is unavailable. The error must also explain that setting
   `DATABASE_URL` bypasses managed local startup.

`deploy/local/compose.yaml` remains the human-readable lifecycle surface. A
contract test must render it and the embedded definition and assert identical
image, port, database name, volume name, and health check so the two definitions
cannot drift.

The local PostgreSQL cluster survives ordinary app exits and container
replacement. Only an explicit documented reset may remove its volume.

### 2. Add The Per-User Credential Schema

Add PostgreSQL migration `0014_user_ai_credentials.sql` with this table:

```sql
CREATE TABLE user_ai_credentials (
    user_id             BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider            TEXT NOT NULL,
    ciphertext          BYTEA NOT NULL,
    nonce               BYTEA NOT NULL,
    encryption_version  SMALLINT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, provider),
    CHECK (provider <> ''),
    CHECK (octet_length(ciphertext) > 16),
    CHECK (octet_length(nonce) = 12),
    CHECK (encryption_version > 0)
);
```

Provider identifiers are normalized and validated in Go. The database permits a
future provider without a schema migration, while the current application still
accepts only `anthropic`.

Add storage operations whose public shapes contain ciphertext but never plaintext:

```go
type EncryptedAICredential struct {
    UserID            int64
    Provider          string
    Ciphertext        []byte
    Nonce             []byte
    EncryptionVersion int16
    UpdatedAt         time.Time
}

func (s *Store) UpsertUserAICredential(ctx context.Context, c EncryptedAICredential) error
func (s *Store) UserAICredential(ctx context.Context, userID int64, provider string) (EncryptedAICredential, bool, error)
func (s *Store) DeleteUserAICredential(ctx context.Context, userID int64, provider string) error
```

Blank credential input keeps the existing row, matching the current profile
form. Turning AI off keeps the encrypted row so re-enabling does not require
re-entry. Explicit credential deletion is an internal storage operation in this
scope; a new delete-key UI is deferred.

### 3. Encrypt Credentials At The Application Boundary

Add `internal/credential` with AES-256-GCM encryption. AES-GCM encrypts and also
detects ciphertext tampering. Its contract is:

```go
type Cipher interface {
    Seal(userID int64, provider, plaintext string) (ciphertext, nonce []byte, version int16, err error)
    Open(userID int64, provider string, ciphertext, nonce []byte, version int16) (string, error)
}
```

Requirements:

- The master key is exactly 32 random bytes, represented as base64 when supplied
  through configuration.
- Production requires `JOBCRON_CREDENTIAL_ENCRYPTION_KEY`. Startup fails before
  serving requests when it is missing or malformed.
- A normal local run reads or creates
  `<OS config directory>/jobcron/credential-encryption.key` with mode `0600`.
  This server-level master key is not an AI credential and is never stored in
  PostgreSQL or a Docker volume.
- Every save uses a fresh 12-byte cryptographic nonce.
- Authenticated metadata binds the ciphertext to
  `jobcron:user-ai-credential:v1:<user_id>:<provider>`. Copying a row to another
  user or provider must therefore fail decryption.
- Version `1` means AES-256-GCM with this metadata format. Unknown versions fail
  closed without deleting or replacing the row.
- Plaintext keys must never appear in logs, errors, templates, telemetry, test
  snapshots, SQL parameters captured by assertions, or command output.
- A decryption failure disables paid AI for that user and returns a stable
  operator-visible error. Rule-based scoring and access to previously stored
  non-secret data continue to work.

The master key must be backed up separately from RDS. Losing it makes stored API
keys intentionally unrecoverable; users can restore service by entering a new
key after the credential row is replaced.

### 4. Make AI Runtime State User-Scoped

Remove `Server.ai`, `aiModel`, `aiVersion`, the four global budget fields,
`aiKeysPath`, `SetAIKeysPath`, and startup `ReconfigureAI`.

Replace them with an operation-scoped value:

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

func (s *Server) aiRuntimeForUser(ctx context.Context, userID int64) (*AIRuntime, error)
```

`aiRuntimeForUser` loads that user's profile, loads and decrypts that user's
credential, constructs the selected provider, and derives that user's limits.
It returns `nil, nil` when AI is off or no credential exists.

One resolved runtime must be passed through a complete scrape or rerate operation;
helpers must not repeatedly decrypt the credential. Authenticated manual scrape,
rerate, profile save, page rendering, and rescore paths must pass their explicit
`userID`. The existing owner-only scheduler resolves the sole owner at the start
of its run. If zero or more than one owner exists, it skips paid AI and records an
operator error rather than guessing. Multi-user scheduler fan-out is out of scope.

Profile save must resolve the authenticated user before handling the key. It then:

1. validates and encrypts a newly entered key;
2. upserts `(user_id, provider)`;
3. saves the non-secret profile;
4. resolves a fresh runtime for that same user; and
5. rerates only that user's score rows.

If steps 1 or 3 fail, no partial credential/profile combination may be presented
as successfully saved. Use a PostgreSQL transaction for the credential row and
profile row; provider construction and paid calls occur only after commit.

### 5. Scope AI Scores And Usage By User

`ai_extractions` remains global. It derives facts only from public posting content
and is reusable when content hash and AI version match.

Add PostgreSQL migration `0015_user_scoped_ai_state.sql` that replaces the other
two global tables with:

```sql
CREATE TABLE ai_scores_user_scoped (
    user_id       BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    posting_id    BIGINT NOT NULL REFERENCES postings(id) ON DELETE CASCADE,
    ai_input_hash TEXT NOT NULL,
    ai_version    TEXT NOT NULL,
    items_json    TEXT NOT NULL,
    net_delta     INTEGER NOT NULL,
    computed_at   TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, posting_id, ai_input_hash, ai_version)
);

CREATE INDEX idx_ai_scores_user_latest
    ON ai_scores_user_scoped(user_id, posting_id, ai_version, computed_at DESC);

CREATE TABLE ai_usage_user_scoped (
    user_id       BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    day           DATE NOT NULL,
    input_tokens  BIGINT NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens BIGINT NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    PRIMARY KEY (user_id, day)
);
```

The migration may assign existing global rows only when exactly one user exists.
If global rows exist with zero or multiple users, the migration must abort with
an actionable error instead of duplicating or guessing ownership.

Every AI-score lookup, upsert, stale fallback, and prune query must require
`user_id`. Pruning one user's provider/model history must not touch another
user's rows. Every usage read and debit must require `user_id`; daily and monthly
caps are calculated per user.

### 6. Upgrade The One-Time SQLite Import

Keep `cmd/jobcron-import` as the explicit operator tool. Normal app startup must
never import data silently.

Add PostgreSQL migration `0016_local_data_imports.sql`:

```sql
CREATE TABLE local_data_imports (
    user_id            BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_sha256      TEXT NOT NULL CHECK (length(source_sha256) = 64),
    source_counts      JSONB NOT NULL,
    imported_counts    JSONB NOT NULL,
    completed_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, source_sha256)
);
```

The importer workflow is:

1. Refuse to proceed while the legacy local app process holds the source open.
2. Create a consistent SQLite snapshot with SQLite's own backup/VACUUM mechanism;
   do not copy only `jobs.db` while WAL data may exist.
3. Compute SHA-256 over that immutable snapshot. Never store its private path.
4. Dry-run by default. Print source counts, target collision counts, the owner
   email placeholder supplied by the operator, credential provider names, and the
   fingerprint. Never print profile JSON, key bytes, endpoints, or passwords.
5. Require `--apply` for writes. Preserve the existing owner's password hash.
6. In one PostgreSQL transaction, import postings, profile, user scores,
   bookmarks, hidden jobs, AI extractions, user-scoped AI scores, user-scoped AI
   usage, and an encrypted credential from an optional legacy `ai_keys.json`.
7. Insert `local_data_imports` only after all writes and count checks succeed.
8. After commit, re-read target rows scoped to the owner and compare every source
   category. A mismatch is a failed verification even though the transaction
   committed; the operator must use the documented PostgreSQL rollback rather
   than rerun blindly.
9. A repeated `--apply` with the same owner and fingerprint performs verification
   only and exits successfully with `already imported`; it performs no writes.
10. A different snapshot targeting an owner who already has imported state is
    refused unless the target has been explicitly reset. There is no `--force`
    overwrite flag.

Required command shape:

```sh
jobcron-import \
  --sqlite '<immutable-sqlite-snapshot>' \
  --postgres '<postgresql-url>' \
  --owner-email '<owner-email>' \
  --ai-keys '<optional-legacy-ai-keys-file>'

jobcron-import \
  --sqlite '<immutable-sqlite-snapshot>' \
  --postgres '<postgresql-url>' \
  --owner-email '<owner-email>' \
  --ai-keys '<optional-legacy-ai-keys-file>' \
  --apply
```

The encryption master key is obtained through the same production/local
configuration rules as the app, never from a command-line argument.

The original SQLite snapshot and `ai_keys.json` remain untouched until browser
verification passes. Documentation then tells the operator how to remove the
plaintext key file. The tool must not claim secure erasure.

### 7. Convert The Writable Preview

Update `scripts/preview-interactive.sh` to:

1. ensure the local PostgreSQL Compose service is healthy;
2. create a uniquely named temporary PostgreSQL database;
3. create a temporary local credential-encryption key;
4. run the app with that database and key;
5. drop the database and key on exit; and
6. retain and print their locations only when `JOBCRON_PREVIEW_KEEP=1`.

The preview must not connect to `jobcron_dev`, production RDS, or the legacy
SQLite file. Its test must prove isolation by writing a profile and asserting
that neither the normal local database nor a second preview contains it.

### 8. Remove The Production Credential Volume Safely

The final `deploy/production/compose.yaml` must:

- remove the app mount at `/root/.config/jobcron`;
- remove the top-level `jobcron_config` declaration;
- require `JOBCRON_CREDENTIAL_ENCRYPTION_KEY`; and
- retain the RDS URL, session secret, and existing Caddy volumes.

The production image must include `jobcron-import`. Add a separate
`deploy/production/compose.migrate.yaml` that is used only for cutover. Its
migration service mounts the existing `jobcron_config` volume read-only at
`/legacy-config`, reads `/legacy-config/ai_keys.json`, and writes the encrypted
credential through the production database connection. The normal app service
must never mount that volume after cutover.

Production sequence:

1. Snapshot RDS and record the current image tag privately.
2. Pull the migration-capable image without replacing the running app.
3. Run importer dry-run through the migration override.
4. Run importer `--apply` and verify the encrypted row can be decrypted and used
   for the owner without printing or making an unapproved paid call.
5. Deploy the final Compose configuration without `jobcron_config`.
6. Sign in, confirm masked-key state, run one approved AI rating, recreate the app
   container, and confirm the rating and key still work.
7. Keep the old Docker volume through the rollback window. Remove it only after
   the human explicitly closes the rollback window.

Neither Compose nor the importer automatically deletes the old volume.

## Failure Modes And Required Behavior

- **Docker CLI or daemon missing locally:** Exit before app startup with
  installation and `DATABASE_URL` bypass guidance.
- **Port `55432` already belongs to another process:** Do not attach to it
  blindly; identify the conflict and refuse managed startup.
- **PostgreSQL health check times out:** Leave diagnostic container state intact
  and return the Compose service and status commands.
- **Legacy SQLite source is live or inconsistent:** Refuse import; do not modify
  source or target.
- **Import copy or count check fails:** Roll back the entire PostgreSQL
  transaction; do not write the import ledger row.
- **Import post-commit verification fails:** Stop. Preserve source and target;
  restore the pre-import PostgreSQL snapshot or volume.
- **Existing global AI rows have ambiguous ownership:** Abort schema migration
  before changing the old tables.
- **Encryption master key is missing or malformed:** Production startup fails
  closed; local startup creates a protected local key only outside production.
- **Credential ciphertext, nonce, version, user, or provider is wrong:** Disable
  paid AI for that user, preserve the row, and surface a non-secret error.
- **Credential save succeeds but profile save fails:** Roll back both database
  changes.
- **Provider returns unauthorized:** Keep the encrypted credential, show a safe
  re-entry message, and do not expose provider response bodies.
- **One user changes provider, model, or key:** Invalidate only that user's
  runtime and caches; other users are unchanged.
- **Old Docker volume is absent:** Continue if the credential table already
  verifies; otherwise require key re-entry.
- **EC2 is lost but RDS survives:** Restore the separately protected master key
  or require users to replace credentials.

## Dependency Graph

```text
0014 credential schema + cipher ───────> user-scoped credential service ──┐
                                                                          ├─> profile/runtime refactor
0015 user-scoped AI scores + usage ───────────────────────────────────────┘             │
                                                                                        v
local PostgreSQL bootstrap ──> PostgreSQL preview                         upgraded importer
                                                                                        │
                                                                                        v
                                                                          production volume cutover
                                                                                        │
                                                                                        v
                                                                            docs + full verification
```

Schema and encryption land first because every later path depends on stable
storage contracts. Runtime isolation lands before credential cutover so a stored
per-user key cannot accidentally feed the existing global provider. The importer
lands before SQLite fallback removal. Production drops the volume last, after the
credential row and rollback evidence exist.

## Acceptance Criteria

1. A normal writable app launch never opens or creates SQLite.
2. With an explicit valid `DATABASE_URL`, startup opens that PostgreSQL database
   and never invokes Docker.
3. Without `DATABASE_URL`, a normal local launch starts PostgreSQL 18 through
   Compose, waits for health, and opens `jobcron_dev` on port `55432`.
4. Production refuses to start without a valid PostgreSQL URL, session secret,
   and 32-byte credential-encryption key.
5. The main `jobcron` command no longer accepts `--db`; only `jobcron-import`
   accepts an explicit legacy SQLite source through `--sqlite`.
6. The writable preview uses a disposable PostgreSQL database and leaves the
   normal local and production databases unchanged.
7. `user_ai_credentials` contains at most one row per user/provider and no
   plaintext API key can be found in PostgreSQL text output, logs, or files under
   the repository.
8. Ciphertext copied to another user or provider fails decryption.
9. Saving a blank key preserves the existing encrypted credential; saving a new
   key replaces only that user's provider row.
10. User A and User B can store different Anthropic keys. A manual AI operation
    for either user constructs a provider with only that user's key.
11. The server has no mutable global AI provider, model, version, or budget state.
12. PostgreSQL AI-score rows, stale fallbacks, pruning, and usage ledgers require
    `user_id`; operations for one user do not read, overwrite, prune, or debit the
    other user's rows.
13. Existing one-owner PostgreSQL AI rows migrate without loss; ambiguous ownership
    aborts before either old table is dropped.
14. The importer dry-run reports all eight existing data categories, credential
    provider count, collisions, and source fingerprint without writing target data.
15. Import `--apply` preserves profile, postings, rule scores, bookmarks, hidden
    jobs, AI extractions, per-user AI scores, per-user AI usage, and the optional
    credential in one transaction.
16. The importer preserves an existing owner's password hash and never imports
    sessions or plaintext passwords.
17. A forced failure in any copied category leaves all target category counts and
    credentials unchanged.
18. Repeating the same completed import performs verification only; a different
    source is refused rather than merged implicitly.
19. The source SQLite snapshot and legacy key file are not modified or deleted by
    the importer.
20. Existing profile values, at least one rule score, one AI score, one bookmark,
    one hidden job, and AI usage totals match between source and target verification.
21. The final production Compose render has no `jobcron_config` declaration or
    `/root/.config/jobcron` mount.
22. The one-time production migration service is the only configuration that can
    mount the legacy volume, and it mounts it read-only.
23. After app-container recreation, the owner remains signed in after a new login,
    sees the migrated profile and ratings, and can use the encrypted credential.
24. Missing credentials or decryption failures fall back to rule-based scoring for
    the affected user without crashing the app or exposing secret material.
25. English and Korean documentation no longer describe SQLite as the normal
    writable database or `jobcron_config` as production credential storage.
26. `gofmt -l .`, `go vet ./...`, `go test -race ./...`, the PostgreSQL integration
    suite, Compose contract tests, import tests, and browser journeys all pass.

## Testing Plan

### Unit

- **Minimum new coverage:** 18 tests.
- **Required cases:** Cipher round trip, nonce uniqueness, wrong key, wrong user,
  wrong provider, wrong version, invalid key size, config validation, and the
  local-startup decision rules.

### Storage

- **Minimum new coverage:** 12 tests.
- **Required cases:** Credential CRUD and isolation, AI-score isolation and
  pruning, usage isolation and monthly queries, one-owner migration, and
  ambiguous-owner rejection.

### Import integration

- **Minimum new coverage:** 8 tests.
- **Required cases:** Dry run, full copy, credential copy, password preservation,
  transaction rollback, same-fingerprint verification, different-source refusal,
  and count mismatch.

### Server

- **Minimum new coverage:** 10 tests.
- **Required cases:** Two-user key isolation, transactional profile save, no-key
  fallback, decrypt failure, provider failure, and user-scoped rerate, scrape,
  score, and budget behavior.

### Compose and process

- **Minimum new coverage:** 4 tests.
- **Required cases:** Automatic start, explicit-URL bypass, health timeout, and
  preview-database cleanup and isolation.

### Browser

- **Minimum new coverage:** 4 journeys.
- **Required cases:** Migrated state, key save and masked state, durability after
  container recreation, and two-session account isolation with seeded users.

### Regression

- **Minimum new coverage:** Existing suites.
- **Required cases:** Full Go tests with the race detector, builds of all shipped
  commands, production and local Compose renders, and English and Korean doc
  checks.

Integration tests must use disposable PostgreSQL schemas or databases. They must
never reset the developer's normal `jobcron_dev`, production RDS, or the retained
legacy SQLite source.

## Rollback Plan

### Before import commit

- The importer transaction rolls back automatically.
- Keep running the old binary against the untouched SQLite source or the old
  production image/volume.

### After import but before PostgreSQL-only writes

- Stop the new app.
- Restore the pre-import RDS snapshot or remove only the disposable local
  PostgreSQL volume.
- Restart the prior version with the untouched SQLite/key source or prior
  production Compose configuration.

### After PostgreSQL-only writes begin

- Do not switch back to SQLite; doing so would discard newer account changes.
- Roll back application code while keeping the PostgreSQL schema compatible, or
  restore a PostgreSQL backup and replay only explicitly approved data.
- Keep the credential master key and old volume through the rollback window.

No rollback command may run `docker compose down -v` against an unidentified
Compose project or delete the source snapshot automatically.

## Implementation Slices And Effort

This is epic-sized and should land in dependency order through reviewable commits.

Slice work:

1. PostgreSQL migrations, credential cipher, configuration, and storage APIs.
2. User-scoped AI runtime, scores, usage, and server tests.
3. Local Compose bootstrap and PostgreSQL preview.
4. Import snapshot, ledger, credential migration, and verification.
5. Production cutover, browser QA, documentation, and security review.

| Slice | Human estimate | AI-agent estimate |
| ----: | -------------- | ----------------- |
| 1 | 1.5-2 days | 3-5 hours |
| 2 | 2-3 days | 6-10 hours |
| 3 | 1-1.5 days | 3-5 hours |
| 4 | 1.5-2 days | 4-7 hours |
| 5 | 1.5-2 days | 4-7 hours |
| **Total** | **7.5-10.5 days** | **20-34 hours** |

## Files Reference

- `cmd/jobcron/main.go`
  - Remove the normal SQLite fallback, resolve local PostgreSQL, and remove the
    startup global AI configuration.
- `cmd/jobcron/main_test.go`
  - Replace SQLite-default expectations with the runtime decision rules.
- `internal/config/config.go`
  - Validate writable PostgreSQL and credential-encryption configuration, and
    restrict `--db`.
- `internal/config/config_test.go`
  - Cover the production key requirement and mode combinations.
- `internal/localdb/ensure.go`
  - Add the automatic Compose startup and health-check package.
- `internal/localdb/compose.yaml`
  - Add the embedded canonical local PostgreSQL 18 service.
- `deploy/local/compose.yaml`
  - Add a stable volume name and health check while remaining
    contract-equivalent to the embedded Compose definition.
- `scripts/preview-interactive.sh`
  - Replace temporary SQLite with a disposable PostgreSQL database and temporary
    master key.
- `scripts/preview_interactive_test.go`
  - Prove PostgreSQL preview isolation and cleanup.
- `internal/storage/postgres_migrations/0014_user_ai_credentials.sql`
  - Add the encrypted per-user credential table.
- `internal/storage/postgres_migrations/0015_user_scoped_ai_state.sql`
  - Scope AI scores and usage to one user and safely migrate global rows.
- `internal/storage/postgres_migrations/0016_local_data_imports.sql`
  - Record verified one-time imports by owner and source fingerprint.
- `internal/storage/ai_credentials.go`
  - Add encrypted credential CRUD.
- `internal/storage/ai_scores.go`
  - Require a user ID in every cache and prune operation.
- `internal/storage/ai_usage.go`
  - Require a user ID for every read and debit.
- `internal/credential/cipher.go`
  - Add the AES-256-GCM implementation and versioned metadata.
- `internal/credential/key.go`
  - Add production-environment and protected local master-key loading.
- `internal/server/server.go`
  - Replace global AI fields with per-user runtime resolution.
- `internal/server/handlers.go`
  - Transactionally save the profile and encrypted credential for the
    authenticated user.
- `internal/server/rerate.go`
  - Pass the per-user runtime through rerate and cache operations.
- `internal/server/scheduler.go`
  - Resolve the sole owner explicitly and fail safely on ambiguous ownership.
- `cmd/jobcron-import/main.go`
  - Add an immutable snapshot, default dry-run, `--apply`, credential import,
    ledger, and post-commit verification.
- `cmd/jobcron-import/main_test.go`
  - Expand PostgreSQL import and failure-path coverage.
- `internal/ai/keys.go`
  - Remove it from runtime; retain only a narrowly scoped legacy reader if the
    importer needs it.
- `deploy/production/Dockerfile`
  - Ship `jobcron-import` beside the app binary.
- `deploy/production/compose.yaml`
  - Remove `jobcron_config` and require the master key.
- `deploy/production/compose.migrate.yaml`
  - Add the one-time read-only legacy-volume migration service.
- `deploy/production/.env.example`
  - Add a placeholder for the encryption master key.
- `deploy/production/README.md`
  - Document final durability and recovery boundaries.
- `deploy/production/HUMAN_DEPLOY_GUIDE.md`
  - Replace key-volume steps with the migration, verification, and rollback
    sequence.
- `README.md` and `README.ko.md`
  - Make PostgreSQL the writable local database and document the Docker
    prerequisite.
- `.goreleaser.yml`
  - Ship the importer and any local Compose artifact required by release users.
- `.github/workflows/ci.yml`
  - Run PostgreSQL-backed unit and integration tests, race checks, Compose checks,
    and import gates.

## Security And Publication Requirements

- Never commit or print a real API key, credential master key, database URL,
  account identity, host address, SQLite source path, or Docker volume contents.
- Tests use deterministic fake keys only and assert that plaintext is absent from
  error strings and captured logs.
- Production documentation uses placeholders and tells the operator to keep the
  master key in the existing access-controlled secret workflow.
- Before committing this specification or implementation documentation, inspect
  the staged diff, run the configured secret scanner, and perform a manual public
  publication review.

## Definition Of Done

All 26 acceptance criteria pass on the exact commit proposed for deployment. The
local SQLite snapshot, production RDS snapshot, old image tag, credential master
key, and legacy Docker volume remain available through the human-approved rollback
window. Only then may the old local plaintext key file and production
`jobcron_config` volume be removed.
