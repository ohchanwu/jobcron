# PostgreSQL Credential Foundation: Slice 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task by task with the listed review checkpoints.

**Goal:** Make Jobcron storage-ready for one encrypted Anthropic credential per user by adding the PostgreSQL schema, authenticated encryption, master-key handling, configuration validation, and ciphertext-only storage APIs.

**Architecture:** The `credential` package owns provider canonicalization, AES-256-GCM encryption, and master-key loading. PostgreSQL stores only ciphertext, a fresh nonce, and an encryption version, keyed by `(user_id, provider)`. The storage package never accepts plaintext; the server-facing runtime that will combine these pieces is intentionally deferred to Slice 2.

**Tech Stack:** Go 1.26, standard-library `crypto/aes` and `crypto/cipher`, PostgreSQL 18, `database/sql` with pgx, embedded SQL migrations, Go unit/integration tests.

## Global Constraints

- Run every command from the Jobcron repository root.
- Keep commits local. Never push or create a pull request.
- Preserve unrelated working-tree changes, especially
  `docs/superpowers/specs/260713-alpha-launch-human-blocked-steps.md`.
- Use fake, deterministic test keys and credentials only. Never print, commit, or
  log an actual API key, master key, database URL, account identity, or host.
- Do not change the running AI request path, profile handler, scoring tables,
  SQLite importer, local PostgreSQL bootstrap, or production Compose in this
  slice.
- Do not remove `internal/ai/keys.go` or the existing credential volume yet.
- PostgreSQL integration tests must use the existing disposable-schema helper;
  they must never reset `jobcron_dev` or any external database.
- Every implementation task follows red-green-refactor: write the focused test,
  observe the intended failure, add the minimum implementation, rerun the
  focused test, then run the neighboring package tests before committing.

## Slice 1 Completion Contract

At the end of this slice:

- PostgreSQL migration 0014 creates `user_ai_credentials` with one row per
  user/provider and user-delete cascade behavior.
- AES-256-GCM produces a fresh 12-byte nonce and authenticates the user ID,
  provider, and encryption version as additional authenticated data (AAD).
- Production configuration requires a base64-encoded 32-byte master key.
- The local key helper reads or atomically creates a protected key file, but the
  main command does not consume it until the Slice 2 runtime wiring.
- Storage CRUD accepts and returns encrypted values only.
- Focused, PostgreSQL integration, race, vet, formatting, and build gates pass.

This slice does **not** make saved credentials usable by live AI requests. That
observable product change is the first task of Slice 2.

---

### Task 1: Add the PostgreSQL credential table

**Files:**

- Create: `internal/storage/postgres_migrations/0014_user_ai_credentials.sql`
- Modify: `internal/storage/postgres_integration_test.go:8-32`

**Interface produced:**

```sql
user_ai_credentials(
    user_id BIGINT,
    provider TEXT,
    ciphertext BYTEA,
    nonce BYTEA,
    encryption_version SMALLINT,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    PRIMARY KEY (user_id, provider)
)
```

- [ ] **Step 1: Extend the migration smoke test and observe the missing table**

Add `"user_ai_credentials"` to the table list in
`TestPostgresMigrationsCreateCoreTables`.

Start or reuse local PostgreSQL, then set the integration-test URL:

```sh
docker compose -f deploy/local/compose.yaml up -d postgres
docker compose -f deploy/local/compose.yaml exec -T postgres \
  pg_isready -U postgres -d jobcron_dev
export JOBCRON_TEST_POSTGRES_URL='postgres://postgres@127.0.0.1:55432/jobcron_dev?sslmode=disable'
go test ./internal/storage -run TestPostgresMigrationsCreateCoreTables -count=1
```

Expected: FAIL because `user_ai_credentials` does not exist. If the test skips,
stop and fix `JOBCRON_TEST_POSTGRES_URL`; a skipped PostgreSQL test is not proof.

- [ ] **Step 2: Add migration 0014**

Create the migration with this exact schema:

```sql
-- 0014_user_ai_credentials.sql — encrypted per-user provider credentials.

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

Do not add a PostgreSQL enum or a provider check constraint. Go validates the
provider, while the table remains extensible without another schema migration.

- [ ] **Step 3: Prove constraints and cascade behavior at the migration layer**

Add `TestPostgresCredentialMigrationConstraintsAndCascade` to
`postgres_integration_test.go`. The test must:

1. Insert a fake user and capture its ID.
2. Insert one valid credential row using a 17-byte-or-longer fake ciphertext and
   a 12-byte fake nonce.
3. Assert a second row with the same `(user_id, provider)` fails.
4. Assert an empty provider, 16-byte ciphertext, 11-byte nonce, and version zero
   each fail.
5. Delete the user and assert the credential row count becomes zero.

Use subtests so a failed constraint names the violated invariant. Keep every
fixture synthetic; do not use a string shaped like a real provider API key.

- [ ] **Step 4: Run the migration tests**

```sh
go test ./internal/storage \
  -run 'TestPostgres(MigrationsCreateCoreTables|CredentialMigrationConstraintsAndCascade)$' \
  -count=1
```

Expected: PASS with both tests executed, not skipped.

- [ ] **Step 5: Commit the schema checkpoint**

```sh
git add internal/storage/postgres_migrations/0014_user_ai_credentials.sql \
  internal/storage/postgres_integration_test.go
git diff --cached --check
git commit -m 'feat(storage): add encrypted user credential schema'
```

---

### Task 2: Implement provider-bound AES-256-GCM

**Files:**

- Create: `internal/credential/cipher.go`
- Create: `internal/credential/cipher_test.go`

**Interfaces produced:**

```go
const EncryptionVersionAES256GCM int16 = 1

type Cipher interface {
    Seal(userID int64, provider, plaintext string) (ciphertext, nonce []byte, version int16, err error)
    Open(userID int64, provider string, ciphertext, nonce []byte, version int16) (string, error)
}

func NewAESGCMCipher(masterKey []byte) (*AESGCMCipher, error)
func NormalizeProvider(provider string) (string, error)
```

- [ ] **Step 1: Write failing provider and constructor tests**

Create table-driven tests covering:

- `NormalizeProvider(" Anthropic ") == "anthropic"`.
- lowercase letters, digits, `_`, and `-` remain valid so future providers do
  not need a database migration.
- empty input and characters outside `[a-z0-9_-]` fail.
- `NewAESGCMCipher` accepts exactly 32 bytes and rejects 0, 16, 31, and 33 bytes.
- returned errors contain neither the supplied key bytes nor their base64 form.

Use a fixture such as `bytes.Repeat([]byte{0x42}, 32)`, never an environment
value.

Run:

```sh
go test ./internal/credential \
  -run 'Test(NormalizeProvider|NewAESGCMCipher)' -count=1
```

Expected: FAIL because `internal/credential` and its API do not exist.

- [ ] **Step 2: Implement canonicalization and cipher construction**

Use a package-level provider pattern and copy the caller's key before building
the AEAD:

```go
var providerPattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

func NormalizeProvider(provider string) (string, error) {
    normalized := strings.ToLower(strings.TrimSpace(provider))
    if !providerPattern.MatchString(normalized) {
        return "", errors.New("credential: invalid provider")
    }
    return normalized, nil
}
```

`NewAESGCMCipher` must require `len(masterKey) == 32`, call `aes.NewCipher`, then
`cipher.NewGCM`. Do not retain or include the key in any returned error.

- [ ] **Step 3: Write failing authenticated-encryption tests**

Add tests for:

- round trip for user `101`, provider `anthropic`, and a synthetic plaintext;
- two seals of the same input produce different nonces and ciphertexts;
- nonce length is 12 and version is 1;
- a different 32-byte key cannot decrypt;
- changing only the user ID cannot decrypt;
- changing only the provider cannot decrypt;
- an unsupported encryption version cannot decrypt;
- truncated ciphertext and wrong nonce length fail safely;
- every failure string omits plaintext, ciphertext, nonce, and key material.

The wrong-provider test must use another syntactically valid provider such as
`openai`. This proves the provider is part of AAD, not merely rejected by input
validation.

- [ ] **Step 4: Implement `Seal` and `Open`**

Use a versioned, domain-separated AAD value:

```go
func aad(userID int64, provider string, version int16) []byte {
    return []byte(fmt.Sprintf(
        "jobcron:user-ai-credential:v%d:%d:%s",
        version,
        userID,
        provider,
    ))
}
```

`Seal` must:

1. Require `userID > 0`, a normalized provider, and non-empty plaintext.
2. Allocate `aead.NonceSize()` bytes.
3. Fill the nonce with `crypto/rand.Read` for every call.
4. Call `aead.Seal(nil, nonce, []byte(plaintext), aad(...))`.
5. Return version 1.

`Open` must validate user/provider/version/nonce size before calling
`aead.Open`. Convert every AEAD authentication failure into a stable generic
error such as `credential: decrypt failed`; do not wrap secret-bearing buffers.

- [ ] **Step 5: Run focused and package tests**

```sh
go test ./internal/credential -count=1
go test -race ./internal/credential -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit the cipher checkpoint**

```sh
git add internal/credential/cipher.go internal/credential/cipher_test.go
git diff --cached --check
git commit -m 'feat(credential): add user-bound AES-GCM cipher'
```

---

### Task 3: Add master-key parsing and protected local persistence

**Files:**

- Create: `internal/credential/key.go`
- Create: `internal/credential/key_test.go`
- Reference: `internal/appdata/paths.go:16-24`
- Reference: `internal/ai/keys.go:12-86`

**Interfaces produced:**

```go
const MasterKeyBytes = 32

func ParseMasterKey(encoded string) ([]byte, error)
func DefaultMasterKeyPath() (string, error)
func LoadOrCreateLocalMasterKey() ([]byte, error)
func LoadOrCreateLocalMasterKeyAt(path string) ([]byte, error)
```

- [ ] **Step 1: Write failing parse and path tests**

Test that:

- standard padded base64 containing exactly 32 bytes parses successfully;
- surrounding whitespace is ignored;
- malformed base64 fails;
- decoded lengths 0, 16, 31, and 33 fail;
- error strings do not contain the encoded input or decoded bytes;
- the default path is
  `<user-config-dir>/jobcron/credential-encryption.key`.

Follow the dependency-injection pattern used by `internal/ai/keys_test.go`: keep
an unexported `defaultMasterKeyPath(userConfigDir func() (string, error))` helper
so the path test never reads the developer's machine configuration.

Run:

```sh
go test ./internal/credential -run 'Test(Parse|DefaultMasterKeyPath)' -count=1
```

Expected: FAIL because the key API is absent.

- [ ] **Step 2: Implement strict key parsing and path resolution**

`ParseMasterKey` must use `base64.StdEncoding.DecodeString` after
`strings.TrimSpace`, then require exactly 32 decoded bytes. Return a newly
allocated byte slice and a generic error. The intended operator generation
format is compatible with `openssl rand -base64 32`, but no operator command or
real value belongs in tests.

Build the default path with:

```go
filepath.Join(appdata.Dir(root), "credential-encryption.key")
```

- [ ] **Step 3: Write failing local file lifecycle tests**

Use a path below `t.TempDir()` and an unexported injectable helper:

```go
func loadOrCreateLocalMasterKeyAt(path string, random io.Reader) ([]byte, error)
```

Cover these cases:

1. Missing parent directories are created with owner-only permissions.
2. A missing file is created with mode `0600` and contains base64, not raw key
   bytes.
3. The returned key is exactly 32 bytes.
4. A second load returns the same key without rewriting the file.
5. A pre-existing valid key is reused.
6. Malformed or wrong-length file contents fail without replacement.
7. On non-Windows systems, a pre-existing file readable by group or others is
   rejected.
8. Two concurrent first loads converge on the same stored key.
9. Errors do not contain file contents or generated key bytes.

Use deterministic readers only inside tests. Do not weaken production randomness
to make tests predictable.

- [ ] **Step 4: Implement atomic, no-clobber creation**

The implementation must:

1. `MkdirAll(filepath.Dir(path), 0700)`.
2. Read and validate an existing file before generating anything.
3. Generate 32 bytes with `crypto/rand.Reader` when missing.
4. Write base64 plus a trailing newline to a mode-`0600` temp file in the same
   directory, `Sync`, and close it.
5. Atomically install without replacing a concurrent winner. A same-directory
   hard link from the temp file to the final path is the cross-process
   no-clobber primitive; if the final path already exists, remove the temp file
   and read the winner.
6. Remove the temp file on every exit path.
7. On non-Windows systems, require final permission bits to equal `0600`.

Do not use a plain `os.Rename` that can overwrite another process's newly
created master key. Two processes starting together must not leave one running
with a key different from the persisted winner.

- [ ] **Step 5: Run focused, race, and package tests**

```sh
go test ./internal/credential -run 'Test(LoadOrCreate|Parse|Default)' -count=1
go test -race ./internal/credential -count=1
```

Expected: PASS, including the concurrent first-load test.

- [ ] **Step 6: Commit the key checkpoint**

```sh
git add internal/credential/key.go internal/credential/key_test.go
git diff --cached --check
git commit -m 'feat(credential): add protected master key loading'
```

---

### Task 4: Validate the production master-key configuration

**Files:**

- Modify: `internal/config/config.go:20-88`
- Modify: `internal/config/config_test.go`

**Interface change:**

```go
type Config struct {
    // existing fields...
    CredentialEncryptionKey []byte
}
```

- [ ] **Step 1: Add a valid production fixture helper**

Several existing tests construct production configuration. Add a helper instead
of copying a base64 fixture through the file:

```go
func validProductionEnv() map[string]string {
    return map[string]string{
        "JOBCRON_ENV": "production",
        "DATABASE_URL": "postgres://db.example.invalid/jobs",
        "SESSION_SECRET": strings.Repeat("s", 32),
        "JOBCRON_CREDENTIAL_ENCRYPTION_KEY": base64.StdEncoding.EncodeToString(
            bytes.Repeat([]byte{0x42}, credential.MasterKeyBytes),
        ),
    }
}
```

Use `.invalid` hostnames only. Update existing tests that intend to reach a
later production validation branch to start from this helper.

- [ ] **Step 2: Write failing configuration tests**

Add tests proving:

- production with no `JOBCRON_CREDENTIAL_ENCRYPTION_KEY` fails and names the
  variable;
- production with malformed base64 fails;
- production with decoded lengths 31 and 33 fails;
- valid production configuration exposes exactly 32 parsed bytes in
  `Config.CredentialEncryptionKey`;
- a non-production environment may omit the value and receives `nil`;
- a non-production environment that explicitly supplies an invalid value still
  fails, preventing a typo from silently selecting another key;
- errors never echo the supplied encoded value;
- `--version` remains usable without production secrets, preserving the current
  early-return behavior.

Run:

```sh
go test ./internal/config -run 'TestLoad.*Credential|TestLoadProduction' -count=1
```

Expected: FAIL on the new credential-key expectations.

- [ ] **Step 3: Parse and validate the environment value**

Import `internal/credential`. During `Load`, read
`JOBCRON_CREDENTIAL_ENCRYPTION_KEY`; when non-empty, parse it with
`credential.ParseMasterKey` and store the decoded bytes. After flag parsing and
the existing `ShowVersion` early return, add the production missing-value check.

Wrap only the variable name and the generic parser error:

```go
return Config{}, fmt.Errorf("JOBCRON_CREDENTIAL_ENCRYPTION_KEY: %w", err)
```

Do not put the encoded value in `Config`; retaining decoded bytes avoids later
layers repeatedly parsing or accidentally logging the environment string.

- [ ] **Step 4: Run configuration and neighboring tests**

```sh
go test ./internal/config ./internal/credential -count=1
go test ./cmd/jobcron -count=1
```

Expected: PASS. The main command does not yet call
`LoadOrCreateLocalMasterKey`; that runtime composition belongs to Slice 2.

- [ ] **Step 5: Commit the configuration checkpoint**

```sh
git add internal/config/config.go internal/config/config_test.go
git diff --cached --check
git commit -m 'feat(config): require production credential master key'
```

---

### Task 5: Add ciphertext-only credential storage CRUD

**Files:**

- Create: `internal/storage/ai_credentials.go`
- Create: `internal/storage/ai_credentials_test.go`
- Reference: `internal/storage/dialect.go`
- Reference: `internal/storage/store_test.go:323-375`

**Interfaces produced:**

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

- [ ] **Step 1: Write failing validation and backend-boundary tests**

In `ai_credentials_test.go`, first cover the API without requiring PostgreSQL:

- user ID must be positive;
- provider is canonicalized with `credential.NormalizeProvider`;
- ciphertext must be longer than the 16-byte GCM tag;
- nonce must be exactly 12 bytes;
- encryption version must be positive;
- all three operations return a clear PostgreSQL-required error when called on
  the current SQLite test store;
- validation errors do not include ciphertext or nonce bytes.

Run:

```sh
go test ./internal/storage -run 'TestUserAICredentialValidation' -count=1
```

Expected: FAIL because the CRUD API does not exist.

- [ ] **Step 2: Implement the encrypted value type and validation**

The storage package may import `internal/credential` for provider
canonicalization. It must not import any AI client package or accept plaintext.

Use one validation helper shared by upsert and key lookup. The upsert path also
validates ciphertext, nonce, and version. Return generic errors that identify the
invalid field, never its value.

- [ ] **Step 3: Write failing PostgreSQL CRUD tests**

Reuse `newPostgresTestStore(t)`. Add a small test-only helper that inserts users
directly and returns their IDs, because the current public account API permits
only one owner.

Cover:

1. missing lookup returns `(zero, false, nil)`;
2. upsert then lookup round-trips all encrypted fields;
3. provider input with surrounding whitespace and uppercase resolves to the
   canonical stored provider;
4. a second upsert for the same user/provider replaces ciphertext, nonce,
   version, and `updated_at` without creating another row;
5. two users can store different values for the same provider;
6. user A cannot read or delete user B's row;
7. delete removes exactly the requested row and is idempotent;
8. deleting a user cascades to that user's credential only.

Run:

```sh
go test ./internal/storage -run 'TestUserAICredential' -count=1
```

Expected: FAIL at the first unimplemented database operation. If skipped, fix
the integration-test URL before proceeding.

- [ ] **Step 4: Implement PostgreSQL upsert**

Use `s.query` so the statement follows the repository's placeholder convention:

```sql
INSERT INTO user_ai_credentials (
    user_id, provider, ciphertext, nonce, encryption_version, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, now(), now())
ON CONFLICT (user_id, provider) DO UPDATE SET
    ciphertext = EXCLUDED.ciphertext,
    nonce = EXCLUDED.nonce,
    encryption_version = EXCLUDED.encryption_version,
    updated_at = now()
```

Preserve `created_at` on conflict. Do not add plaintext parameters, logging, or
error formatting around the byte slices.

- [ ] **Step 5: Implement lookup and delete**

Lookup must select:

```sql
SELECT user_id, provider, ciphertext, nonce, encryption_version, updated_at
  FROM user_ai_credentials
 WHERE user_id = ? AND provider = ?
```

Map `sql.ErrNoRows` to `found=false`. Delete by both user ID and provider and
treat zero affected rows as success. Wrap database failures with operation names
only.

- [ ] **Step 6: Run focused, full storage, and race tests**

```sh
go test ./internal/storage -run 'TestUserAICredential' -count=1
go test ./internal/storage -count=1
go test -race ./internal/storage -run 'TestUserAICredential' -count=1
```

Expected: PASS with PostgreSQL tests executed.

- [ ] **Step 7: Commit the storage checkpoint**

```sh
git add internal/storage/ai_credentials.go internal/storage/ai_credentials_test.go
git diff --cached --check
git commit -m 'feat(storage): add encrypted user credential CRUD'
```

---

### Task 6: Prove the cipher-to-storage security boundary

**Files:**

- Modify: `internal/storage/ai_credentials_test.go`
- Modify if needed: `internal/credential/cipher_test.go`
- Modify if needed: `internal/credential/key_test.go`

- [ ] **Step 1: Add an end-to-end storage integration test**

Add `TestUserAICredentialEncryptedRoundTrip` in package `storage`. It must:

1. Construct `credential.NewAESGCMCipher` with a deterministic fake 32-byte
   master key.
2. Encrypt a conspicuous synthetic plaintext marker for user A and provider
   `anthropic`.
3. Store only the returned ciphertext, nonce, and version.
4. Read the row through `UserAICredential` and decrypt it with the same
   user/provider.
5. Assert the plaintext round trip.
6. Assert decryption fails when the retrieved row is reassigned to user B.
7. Assert decryption fails when the provider changes.
8. Query the row directly using PostgreSQL `encode(ciphertext, 'hex')` and
   `encode(nonce, 'hex')`, then assert neither textual value contains the
   plaintext marker.

This test proves the exact boundary: plaintext exists in the caller and cipher,
but the storage API and database row receive encrypted material only.

- [ ] **Step 2: Add a secret-leak regression assertion**

Across cipher, key, config, and storage tests, collect every expected error from
malformed inputs and assert it omits:

- the synthetic plaintext marker;
- fake master-key bytes and their base64 encoding;
- ciphertext bytes;
- nonce bytes.

Do not assert that the source tree lacks the marker—the test fixture must contain
it. Assert against runtime errors and captured database values, which are the
actual leak surfaces in this slice.

- [ ] **Step 3: Run the focused security tests**

```sh
go test ./internal/credential ./internal/config ./internal/storage \
  -run 'Test.*(Credential|Cipher|MasterKey|EncryptedRoundTrip|Secret)' \
  -count=1
```

Expected: PASS with the PostgreSQL integration test executed.

- [ ] **Step 4: Commit the security checkpoint**

```sh
git add internal/storage/ai_credentials_test.go \
  internal/credential/cipher_test.go \
  internal/credential/key_test.go
git diff --cached --check
git commit -m 'test(credential): prove encrypted storage boundary'
```

If one of the listed files did not change, omit it from `git add` rather than
creating a cosmetic edit.

---

### Task 7: Run the Slice 1 verification and publication gate

**Files:**

- Review: every file changed by Tasks 1-6
- Modify: this plan only if execution uncovered a durable correction

- [ ] **Step 1: Re-read the cumulative implementation diff**

```sh
git status --short
git diff HEAD~6 -- \
  internal/credential \
  internal/config \
  internal/storage/ai_credentials.go \
  internal/storage/ai_credentials_test.go \
  internal/storage/postgres_integration_test.go \
  internal/storage/postgres_migrations/0014_user_ai_credentials.sql
git diff --check
```

Adjust `HEAD~6` to the first Slice 1 commit if execution produced a different
number of checkpoints. Confirm manually that:

- no storage method accepts plaintext;
- AAD includes version, user ID, and normalized provider;
- every seal gets a new nonce;
- the production key is required and exactly 32 decoded bytes;
- local creation is mode `0600`, atomic, and no-clobber;
- no error or log statement interpolates secret-bearing input;
- no Slice 2-5 runtime or deployment change slipped into the diff.

- [ ] **Step 2: Run formatting and static analysis**

```sh
test -z "$(gofmt -l .)"
go vet ./...
```

Expected: both commands exit zero with no formatting output.

- [ ] **Step 3: Run all tests with PostgreSQL enabled**

```sh
docker compose -f deploy/local/compose.yaml exec -T postgres \
  pg_isready -U postgres -d jobcron_dev
export JOBCRON_TEST_POSTGRES_URL='postgres://postgres@127.0.0.1:55432/jobcron_dev?sslmode=disable'
go test ./... -count=1
go test -race ./... -count=1
```

Expected: PASS. Inspect the output and confirm the new PostgreSQL tests ran; a
suite that silently skipped them does not satisfy this gate.

- [ ] **Step 4: Build every shipped Go package**

```sh
go build ./...
```

Expected: PASS.

- [ ] **Step 5: Archive the completed plan and write the verification record**

After every implementation gate passes, move this completed plan out of active
work and create a result-only report:

```sh
mkdir -p docs/superpowers/archive/2026-07-14-postgresql-credential-foundation
git mv \
  docs/superpowers/plans/260714-postgresql-credential-foundation-implementation-plan.md \
  docs/superpowers/archive/2026-07-14-postgresql-credential-foundation/
```

Create
`docs/superpowers/archive/2026-07-14-postgresql-credential-foundation/260714-postgresql-credential-foundation-verification.md`
with the actual commit IDs and observed results for formatting, vet, focused
tests, PostgreSQL tests, race tests, build, and secret scan. Do not prefill a
result as PASS before its command succeeds.

Update both indexes:

- remove the Slice 1 plan from `docs/superpowers/README.md` Active Work;
- add the archived plan and verification record under Recently Archived;
- replace the active plan link in `docs/README.md` with the verification link;
- leave the parent credential-convergence specification active for Slices 2-5.

- [ ] **Step 6: Run the publication-security gate and commit the archive**

Stage only the intended lifecycle documentation, inspect the complete staged
diff, then run Gitleaks:

```sh
git add docs/README.md docs/superpowers/README.md \
  docs/superpowers/archive/2026-07-14-postgresql-credential-foundation/
git diff --cached --check
git diff --cached
gitleaks git --staged --redact=100 --no-banner
git diff --cached --name-only
```

Expected staged paths are exactly the two indexes, the archived plan, and the
verification record. Never stage
`docs/superpowers/specs/260713-alpha-launch-human-blocked-steps.md`. Do not
suppress a scanner finding merely to pass, and manually confirm the staged docs
contain no actual credentials, identities, infrastructure addresses, or private
paths.

Commit locally:

```sh
git commit -m 'docs: archive PostgreSQL credential foundation slice'
```

- [ ] **Step 7: Report the slice boundary**

The execution report must include:

- local commit IDs;
- focused, PostgreSQL, race, vet, formatting, build, and secret-scan results;
- explicit confirmation that PostgreSQL tests executed rather than skipped;
- confirmation that no real secret entered git history;
- confirmation that live per-user AI resolution remains intentionally deferred
  to Slice 2;
- any independent decision and its rollback cost.

Do not claim users can save or use account-synced credentials after Slice 1.

## Rollback

Before any Slice 2 code depends on this foundation, rollback is mechanical:

1. Revert the Slice 1 Go commits.
2. Drop `user_ai_credentials` only in disposable development/test databases, or
   restore the pre-0014 database snapshot in an environment where migration 0014
   already ran.
3. Retain any generated local master-key file until the rollback decision is
   final; deleting it would make future ciphertext unrecoverable.

Once real credentials are stored, do not drop the table or replace/delete the
master key as a casual rollback. Preserve both and revert only the code path that
uses them.
