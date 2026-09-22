# PostgreSQL Convergence Slice 2 Verification

## Outcome

Slice 2 is integrated locally. PostgreSQL AI scores, usage, credentials, and
runtime state are user-scoped. SQLite remains a temporary rule-only legacy
reader for the verified import in Slice 4.

## Integrated Commits

- `4afc0db` scopes PostgreSQL AI state by user.
- `ce0f68d` requires user IDs in AI score and usage storage methods.
- `1058051` saves profiles and optional encrypted credentials atomically.
- `79552d8` replaces mutable global AI configuration with operation runtimes.
- `8124149` hardens fallback, scheduler, and scored-surface behavior.

## Verified Contracts

- Migration `0015` copies legacy AI rows only for one unambiguous owner and
  leaves the old schema intact on rejected zero-owner or multi-owner cases.
- Every PostgreSQL AI score and usage query is user-scoped, including pruning.
- Profile and optional credential writes commit or roll back together.
- One encrypted credential is decrypted while resolving one user's immutable
  operation runtime; plaintext is not cached on `Server`.
- Authenticated scrape, rerate, profile, render, and rescore paths carry the
  explicit user ID.
- The scheduler acquires its operation lock before owner and runtime
  resolution.
- Runtime failure still permits rule-only startup recovery.
- PostgreSQL and retained SQLite rendering use one profile/hash snapshot and
  omit missing or stale scores instead of presenting fabricated zero values.
- The legacy SQLite path neither reads nor modifies `ai_keys.json`.

## Fresh Verification

The integrated Mayor checkout passed:

```sh
test -z "$(gofmt -l .)"
git diff --check 65a1cf6..HEAD
go vet ./...
go build ./...
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./cmd/jobcron
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./cmd/jobcron
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' go test ./... -count=1
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' \
  go test -race ./... -count=1
gitleaks git --log-opts='65a1cf6..HEAD' --no-banner --redact
```

Focused PostgreSQL tests also passed for migration locking and ambiguity,
runtime fallback, scheduler/runtime interleaving, render snapshot consistency,
SQLite profile-save/render interleaving, and missing-score behavior across all
scored surfaces.

## Handoff To Later Slices

- Slice 3 must resolve a stable positive local owner before any
  PostgreSQL-backed request reaches `Server`.
- Slice 4 removes normal SQLite startup and confines all legacy reads to the
  verified importer.
- Slice 4 must retain the user-scoped conflict keys already required by
  migration `0015`.
