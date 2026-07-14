# PostgreSQL Credential Foundation: Slice 1 Verification

## Outcome

Slice 1 is complete and storage-ready. Jobcron now has:

- PostgreSQL migration 0014 for one encrypted credential per user/provider;
- AES-256-GCM with fresh 12-byte nonces and authenticated user/provider/version
  binding;
- strict base64 parsing for a 32-byte server master key;
- atomic, owner-only local master-key persistence with concurrent first-start
  convergence;
- production configuration validation for the encryption key;
- ciphertext-only PostgreSQL create/read/update/delete storage methods;
- an integration test proving plaintext does not cross the storage boundary and
  copied rows fail authentication for another user or provider.

Live AI requests do not consume these rows yet. Per-user runtime resolution is
intentionally deferred to Slice 2.

## Local Commits

- `57be5cf` — encrypted per-user credential schema
- `a8f6038` — user/provider-bound AES-GCM cipher
- `f74cf8f` — protected master-key parsing and local persistence
- `28d9ddb` — production master-key configuration validation
- `8a5c78f` — encrypted credential storage CRUD
- `3030b0f` — cipher-to-PostgreSQL security boundary test

## Verification Evidence

- `test -z "$(gofmt -l .)"`: PASS
- `git diff --check`: PASS
- `go vet ./...`: PASS
- `go test ./... -count=1` with PostgreSQL integration enabled: PASS across 22
  package results
- `go test -race ./... -count=1` with PostgreSQL integration enabled: PASS
  across 22 package results
- `go build ./...`: PASS
- Explicit verbose execution of the migration table, migration constraint, and
  encrypted round-trip PostgreSQL tests: PASS; none skipped
- Gitleaks across all six implementation commits: PASS, no leaks found
- Markdown publication checks for the archived plan and report: PASS

No browser QA was required because Slice 1 changes only backend configuration,
cryptography, migrations, and storage tests; it does not change HTML, CSS,
JavaScript, routes, or user-visible behavior.

## Security Review

- The database schema and storage API contain no plaintext credential field.
- AES-GCM additional authenticated data contains the encryption version, user
  ID, and normalized provider, so ciphertext cannot be reassigned safely.
- Authentication failures are generic and omit plaintext, key, nonce, and
  ciphertext material.
- Master-key files are base64 encoded, mode `0600`, and installed atomically
  without replacing a concurrent winner.
- Production stores only decoded key bytes in `Config`; it does not retain the
  original environment string.
- All committed values are deterministic synthetic fixtures or placeholders.

## Independent Decisions Worth Review

- **Chosen:** install the local key temp file with an atomic hard link, treating
  an existing destination as the concurrent winner. **Rejected:** plain rename,
  which can replace another process's new key. **Rollback cost:** low; this is
  isolated to the local key loader.
- **Chosen:** normalize any syntactically safe provider in the cipher/storage
  foundation while leaving application-level provider support to Slice 2.
  **Rejected:** a PostgreSQL provider enum, which would require a migration for
  every future provider. **Rollback cost:** low before additional providers are
  introduced.
- **Chosen:** validate and retain decoded master-key bytes in configuration.
  **Rejected:** carrying the base64 environment string into later layers, which
  creates more opportunities to log it accidentally. **Rollback cost:** low.

## Remaining Boundary

Do not claim account-synced AI credentials are usable from the application yet.
Slice 2 must resolve and decrypt the authenticated user's row for each AI
operation and preserve rule-based fallback behavior when no usable credential
exists.
