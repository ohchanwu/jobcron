# PostgreSQL Convergence Slice 3 Verification

## Outcome

Slice 3 is integrated locally. Jobcron now owns one canonical PostgreSQL 18
Compose lifecycle for local work, and the interactive preview uses isolated
PostgreSQL databases without removing shared containers or volumes. Ordinary
no-URL startup intentionally remains on SQLite until Slice 4 completes the
verified import and writable cutover.

## Integrated Commits

- `e33020d` defines the canonical local PostgreSQL Compose service.
- `dda1393` waits for Compose health and real host TCP reachability.
- `2dd67f5` resolves local PostgreSQL, the master key, and a positive owner.
- `582ded2` moves interactive previews to disposable PostgreSQL databases.
- `5367f84` documents the managed local lifecycle and migration boundary.
- `93bfd7d` adds PostgreSQL and preview-isolation coverage to CI.
- `b69da96` hardens cleanup and restricts the trust-auth database to loopback.
- `f14afe1` keeps PostgreSQL demo state user-scoped and makes preview ports exact.

## Verified Contracts

- The canonical service is `jobcron-local-postgres-1`, publishes only on
  `127.0.0.1:55432`, and uses `jobcron-postgres18-cluster`.
- Startup requires Docker, Compose, daemon access, Compose health, and a real
  TCP connection. Failures preserve state and print narrow diagnostics.
- Managed local startup creates a fixed no-login owner only for an empty users
  table, reuses one existing positive owner, and refuses ambiguity.
- An explicit database URL bypasses managed Docker startup and must already
  contain exactly one positive user.
- Preview launch owns an atomic per-user/per-port lock, refuses an unrelated
  listener, and starts on exactly the requested port without fallback.
- Each preview receives a unique database, private state directory, and master
  key. Normal exit removes only owned state; keep mode prints executable cleanup.
- Failed database or lock cleanup returns nonzero, and stale locks receive a
  conservative manual-remediation diagnostic without being stolen.
- The legacy SQLite startup and `--db` path remain available only for the Slice
  4 transition. The activation probe creates no SQLite file when using
  `--version`.

## Fresh Verification

The exact integrated tip `f14afe1e51052c70a86fe4a4df539d70bf248005`
passed against a unique disposable PostgreSQL database:

```sh
test -z "$(gofmt -l .)"
go vet ./...
go build ./...
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' go test ./... -count=1
JOBCRON_TEST_POSTGRES_URL='<disposable-postgres-url>' \
  go test -race ./... -count=1
```

The retained-path activation probe, both rendered Compose contracts, preview
normal and race suites, real-browser preview smoke, diff check, and Gitleaks
also passed. Cleanup checks found no preview databases or live preview locks.

The live local state was verified after the gate:

- `jobcron-local-postgres-1` is healthy on `127.0.0.1:55432`.
- The older `local-postgres-1` remains stopped and preserved.
- `jobcron-postgres18-cluster` and
  `local_jobcron-postgres18-cluster` both remain preserved.

## Handoff To Slice 4

- Preserve the resolved positive-owner rules while removing normal SQLite
  startup.
- Keep `JOBCRON_STRICT_PORT=1` internal to the preview; ordinary startup retains
  its ten-port fallback.
- Keep previews isolated and never run shared Compose `down` during cleanup.
- Confine `storage.OpenSQLiteAt` to the verified importer and its fixtures.

Nothing was pushed and no pull request or merge-queue entry was created.
