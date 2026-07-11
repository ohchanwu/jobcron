# Rename Task 7 Report: Full Verification And Local Handoff

## Result

The jobcron hard rename is verified from baseline `01cdb02` through implementation
HEAD `0a1a8d0`. No implementation defect required a source-code fix. This report
and the progress ledger are the only Task 7 file changes.

The local Git remote was already canonical:

```text
git@github.com:ohchanwu/jobcron.git
```

No push or other remote write was performed.

## Go Verification

All required commands passed from a clean tracked worktree:

```sh
gofmt -w cmd internal
test -z "$(gofmt -l .)"
go list ./...
go build ./cmd/jobcron ./cmd/jobcron-user ./cmd/jobcron-import
go vet ./...
go test ./... -count=1
```

`go list ./...` returned only packages under
`github.com/ohchanwu/jobcron`. Formatting produced no changes.

## PostgreSQL 18 Integration

The local container `local-postgres-1` ran `postgres:18-alpine`, server version
`18.4`, on port `55432`.

A uniquely named database, `jobcron_rename_test_1783735769_54740`, was created,
used only for this command, and dropped successfully by a cleanup trap:

```sh
JOBCRON_TEST_POSTGRES_URL='postgres://postgres@localhost:55432/jobcron_rename_test_1783735769_54740?sslmode=disable' \
  go test ./cmd/jobcron-import ./cmd/jobcron-user ./internal/storage -count=1
```

All three packages passed. The legacy `jobscraper_dev` database and production
data were not accessed or modified.

## Real Command-Path Migration

The real command path ran under temporary home
`/tmp/jobcron-task7-home.Nt1HL6`:

```sh
HOME="$temporary_home" \
JOBCRON_DB="$temporary_home/runtime.db" \
JOBCRON_HOST=127.0.0.1 \
JOBCRON_PORT=17777 \
go run ./cmd/jobcron --no-open
```

The temporary legacy macOS config directory contained:

- `jobs.db`
- `jobs.db-wal`
- `jobs.db-shm`
- `ai_keys.json`

The command started on `127.0.0.1:17777`, renamed the entire legacy directory to
`jobcron`, removed the old directory path, and preserved identical SHA-256
hashes for all four files. The temporary home was removed after verification.

## Frontend QA

The preview uses an isolated PostgreSQL 18 database named
`jobcron_task7_preview_1783735875`, a temporary owner account, one temporary
posting, and a temporary config home. It remains available for local inspection:

```text
http://127.0.0.1:17778
```

The browser workflow used the gstack headless browse CLI. It logged in through
the real production UI and exercised the archive card at both required
viewports:

- Desktop: `1440x900`
- Mobile: `390x844`

The bookmark and hide handlers were exercised with the page's demo-state flag
enabled so the JavaScript used browser storage without changing PostgreSQL
bookmark or hidden-job rows. Evidence:

```text
jobcronDemoBookmarks = ["1"]
jobcronDemoHidden = ["1"]
jobScraperDemoBookmarks = null
jobScraperDemoHidden = null
bookmark aria-pressed = true
hidden card hidden = true
```

No console errors appeared. Every local asset request completed successfully;
the only non-200 responses were expected login redirects. Desktop and mobile
both passed `document.documentElement.scrollWidth <= window.innerWidth + 2`.
Visual inspection found no overlap, clipping, text containment problem, or
responsive regression.

Screenshots:

- `/tmp/jobcron-task7-desktop-bookmarked.png`
- `/tmp/jobcron-task7-desktop-hidden-recapture.png`
- `/tmp/jobcron-task7-mobile-bookmarked.png`

Controller follow-up recaptured the hidden-post state after a two-second settle.
The page had no active animations, the posting card was hidden, the body
background remained `rgb(251, 247, 239)`, and the recapture visually contained
no black rectangle. The original and recaptured PNGs were byte-for-byte
identical (SHA-256
`d460fcd974473b646dd5ec3c2e78b42f986959d75347abbc03f16738cf548454`,
zero differing pixels). Both are opaque 1440x900 PNGs. The reported black region
was therefore a downstream image display/decode artifact, not a product render
or screenshot-timing defect.

## Container, Compose, And Release Checks

Both Linux arm64 images built locally and expose `jobcron` as the entrypoint:

```text
jobcron:task7-production  arch=arm64 os=linux entrypoint=["jobcron"]
jobcron:task7-demo        arch=arm64 os=linux entrypoint=["jobcron"]
```

The demo, local PostgreSQL, and production Compose files all rendered. The
rendered application contracts contained no active `JOBSCRAPER_*` or
`job-scraper` name. Production rendered `JOBCRON_DAILY_SCRAPE_TIME: "05:00"`;
local PostgreSQL rendered `postgres:18-alpine`, `jobcron_dev`, and
`jobcron-postgres18-cluster`.

GoReleaser was not installed globally, so the verification used a temporary
module invocation:

```sh
go run github.com/goreleaser/goreleaser/v2@latest check
go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean
```

GoReleaser `v2.17.0` validated one configuration file and produced the full
snapshot archive matrix with canonical `jobcron_*` names plus checksums. The
tool selected Go `1.26.5` because that GoReleaser version requires Go 1.26.4 or
newer. The snapshot succeeded in 33 seconds and left no tracked changes.

## Repository Scans

The old-name scan found 170 permitted occurrences covered by the design
allowlist and zero uncategorized files. Related allowlist categories are grouped
below:

```text
legacy Docker rollback volume                         1
dated historical evidence                            72
source mappings and deferred Gas Town design          84
legacyDirName app-data migration                       1
rejection tests and migration fixtures                 8
immutable migrations and migration 0013 sentinel       4
```

All historical files containing old names carry the required 2026-07-11 rename
note. The tracked secret scan found one deliberately synthetic pre-existing
test string in `internal/ai/injection_test.go`; the cumulative rename diff added
zero secret-like values. Thirteen local Markdown links were checked and none
were broken.

## Cumulative Review

The cumulative diff from `01cdb02` through `0a1a8d0` was reread with rename
detection and checked against the hard-rename design.

```sh
git diff --check 01cdb02..0a1a8d0
git diff --find-renames 01cdb02..0a1a8d0
git diff --stat 01cdb02..0a1a8d0
git log --reverse --oneline 01cdb02..0a1a8d0
```

The review confirmed the module and commands, runtime variables, app-data
migration, PostgreSQL sentinel migration, browser and HTTP identities,
container/release contracts, and active documentation all match the design.
Append-only migrations, rollback identifiers, historical evidence, and the Gas
Town identity remain unchanged only where explicitly allowed. No user edit was
discarded, no unrelated refactor entered the diff, and `git diff --check`
reported no whitespace error.

## Limitations And Human Handoff

- The browser-storage interaction used the production PostgreSQL UI with the
  page demo-state flag enabled. The deployed public demo is intentionally
  SQLite-only; combining demo mode with PostgreSQL is not a supported runtime
  contract.
- The preview database and server remain running for human inspection. They
  contain only temporary Task 7 fixtures and must not be mistaken for production
  data.
- The Gas Town rig and workspace still use `jobscraper`. Their rename remains a
  separate migration project.
- The EC2 application directory move to `/srv/jobcron` and pulling the canonical
  image remain human-owned deployment actions. No EC2 state or secret was
  changed during Task 7.
- No remote branch was changed and nothing was pushed.
