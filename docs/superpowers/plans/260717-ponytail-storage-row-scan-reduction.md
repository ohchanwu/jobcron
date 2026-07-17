# Storage Row Scan Reduction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace four identical bookmark and mute row-consumption loops with one storage-owned
collector while preserving every SQL query, order, error, and user-scope rule.

**Architecture:** `scanPosting` remains the decoder for one posting row. A concrete
`scanPostings(*sql.Rows)` collector beside it owns only iteration, append order, scan failure,
and `rows.Err`; bookmark and not-interested methods continue to own their queries and errors.

**Tech Stack:** Go 1.26, `database/sql`, SQLite, PostgreSQL 18, existing storage tests.

## Global Constraints

- Batch: `PT4-004`; candidate: `PONY-004`.
- Start from the human-reviewed Task 5 tip supplied by Mayor, with clean porcelain.
- The reviewed base must contain this plan and the ledger entry marked `planned`.
- Production scope is exactly `postings.go`, `bookmarks.go`, and `not_interested.go`.
- Do not parameterize SQL identifiers, table names, timestamp columns, or error strings.
- Preserve SQLite fallback, PostgreSQL user validation, ordering, and `rows.Close` ownership.
- Do not add a storage interface or dependency.
- Target at least 30 fewer production lines and zero dependency change.
- Compare binary deltas by building both SHAs sequentially at one checkout path with the same
  Go toolchain and environment and explicit temporary `-o` outputs.
- Never push, perform neighboring loop cleanup, or combine this batch with another reduction.

---

### Task 1: Lock the four current list behaviors and before metrics

**Files:**

- Modify: none
- Test: `internal/storage/bookmarks_test.go`
- Test: `internal/storage/not_interested_test.go`
- Test: `internal/storage/postgres_integration_test.go`
- Evidence: `.superpowers/sdd/260717-ponytail/PT4-004-before.md`

**Interfaces:**

- Consumes: four methods returning `([]scraper.Posting, error)`.
- Produces: an immutable success-order and PostgreSQL-scope baseline.

- [ ] **Step 1: Reconfirm exact loops and allowed consumers**

```sh
git status --short --branch
git log -1 --format='%H %cI %s'
rg -n 'rows.Next|scanPosting\(rows\)|rows.Err' \
  internal/storage/postings.go internal/storage/bookmarks.go \
  internal/storage/not_interested.go
```

Expected: the approved four loops are in bookmark and not-interested list methods. Other
`postings.go` loops are outside this batch and remain unchanged.

- [ ] **Step 2: Run existing SQLite behavior locks**

```sh
go test ./internal/storage \
  -run '^TestBookmarkedPostingsOrderedByBookmarkedAtDesc$' -count=1
go test ./internal/storage \
  -run '^TestNotInterestedPostingsOrderedByMutedAtDesc$' -count=1
```

Expected: PASS; ordering and decoded posting contents are locked before refactoring.

- [ ] **Step 3: Run PostgreSQL user-scope locks without skips**

```sh
test -n "$JOBCRON_TEST_POSTGRES_URL"
go test ./internal/storage -run 'Postgres|UserScoped|UserState' -count=1
```

Expected: relevant PostgreSQL tests execute and pass. A skip or missing disposable PostgreSQL
18 database blocks implementation.

- [ ] **Step 4: Record before measurements**

```sh
mkdir -p .superpowers/sdd/260717-ponytail
git ls-files '*.go' ':(exclude)**/*_test.go' | xargs wc -l
git ls-files '**/*_test.go' | xargs wc -l
go list -m -f '{{if not .Indirect}}{{.Path}} {{.Version}}{{end}}' all
ponytail_metrics_dir=$(mktemp -d)
go test ./internal/storage \
  -coverprofile="$ponytail_metrics_dir/storage-before.cover" -count=1
go tool cover -func="$ponytail_metrics_dir/storage-before.cover"
```

Expected: all commands pass; record exact output in the ignored before file.

### Task 2: Add the narrow collector and replace four loops

**Files:**

- Modify: `internal/storage/postings.go` beside `scanPosting`
- Modify: `internal/storage/bookmarks.go:182-237`
- Modify: `internal/storage/not_interested.go:185-240`

**Interfaces:**

- Consumes: `func scanPosting(rowScanner) (scraper.Posting, error)`.
- Produces: `func scanPostings(rows *sql.Rows) ([]scraper.Posting, error)`.

- [ ] **Step 1: Add the exact collector beside `scanPosting`**

```go
func scanPostings(rows *sql.Rows) ([]scraper.Posting, error) {
	var postings []scraper.Posting
	for rows.Next() {
		p, err := scanPosting(rows)
		if err != nil {
			return nil, err
		}
		postings = append(postings, p)
	}
	return postings, rows.Err()
}
```

Use concrete `*sql.Rows`; do not introduce a rows interface, callback, generic, or query helper.

- [ ] **Step 2: Replace each approved loop**

After each method's existing `defer rows.Close()`, replace the local slice and loop with:

```go
return scanPostings(rows)
```

Apply this only to `BookmarkedPostings`, `BookmarkedPostingsForUser`,
`NotInterestedPostings`, and `NotInterestedPostingsForUser`.

- [ ] **Step 3: Run targeted green checks**

```sh
gofmt -w internal/storage/postings.go internal/storage/bookmarks.go \
  internal/storage/not_interested.go
go test ./internal/storage \
  -run '^TestBookmarkedPostingsOrderedByBookmarkedAtDesc$' -count=1
go test ./internal/storage \
  -run '^TestNotInterestedPostingsOrderedByMutedAtDesc$' -count=1
test "$(rg -n 'return scanPostings\(rows\)' \
  internal/storage/bookmarks.go internal/storage/not_interested.go | wc -l | tr -d ' ')" = 4
```

Expected: tests pass and exactly four approved methods use the collector.

### Task 3: Run full and PostgreSQL verification

**Files:**

- Modify: none
- Evidence: `.superpowers/sdd/260717-ponytail/PT4-004-after.md`

**Interfaces:**

- Consumes: one collector and four call replacements.
- Produces: complete storage behavior and reduction evidence.

- [ ] **Step 1: Run static, build, unit, race, and coverage gates**

```sh
test -z "$(gofmt -l .)"
go vet ./...
go build ./...
go test ./... -count=1
go test -race ./... -count=1
go test ./... -coverprofile=/tmp/jobcron-ponytail-PT4-004.cover -count=1
go tool cover -func=/tmp/jobcron-ponytail-PT4-004.cover
```

Expected: every command passes.

- [ ] **Step 2: Run proportional PostgreSQL gates without skips**

```sh
test -n "$JOBCRON_TEST_POSTGRES_URL"
go test ./internal/storage ./cmd/jobcron-import ./cmd/jobcron-user -count=1
go test -race ./internal/storage ./cmd/jobcron-import ./cmd/jobcron-user -count=1
```

Expected: all PostgreSQL-backed tests execute and pass. Scraper, AI, browser, cross-build, and
deployment gates are not proportional because queries and external behavior do not change.

- [ ] **Step 3: Repeat Task 1 source, dependency, and coverage metrics after the edit**

Repeat Task 1 Step 4 with `storage-after.cover`. Expected: production lines decrease by at least
30; tests and dependencies remain stable; coverage movement is recorded without being treated
as reduction.

### Task 4: Review and commit the implementation batch

**Files:**

- Modify: `internal/storage/postings.go`
- Modify: `internal/storage/bookmarks.go`
- Modify: `internal/storage/not_interested.go`

**Interfaces:**

- Consumes: all passing gates.
- Produces: one independently reversible storage-row commit.

- [ ] **Step 1: Run Ponytail and correctness/security review**

```sh
git diff -- internal/storage/postings.go internal/storage/bookmarks.go \
  internal/storage/not_interested.go
git diff --check
git status --short
```

Expected: four loops collapse into one collector. Confirm every SQL string, order clause,
timestamp, error string, user check, `defer rows.Close`, and migration remains byte-identical.

- [ ] **Step 2: Commit exactly the storage batch**

```sh
git add internal/storage/postings.go internal/storage/bookmarks.go \
  internal/storage/not_interested.go
git diff --cached --check
git commit -m "refactor(storage): share posting row collector"
```

Expected: one commit. Rollback is `git revert <PT4-004-commit>` and restores all four local
loops without affecting another batch.

- [ ] **Step 3: Measure the exact base and implementation binaries at one path**

After the implementation commit, store its branch and SHA. Switch the same clean checkout to
the exact Mayor-supplied base, build all three binaries to explicit temporary outputs, restore
the implementation branch, verify its SHA, and repeat with the same Go environment:

```sh
ponytail_metrics_dir=$(mktemp -d)
implementation_branch=$(git branch --show-current)
implementation_sha=$(git rev-parse HEAD)
test -n "$implementation_branch"
git switch --detach <exact-base-sha>
go build -o "$ponytail_metrics_dir/base-jobcron" ./cmd/jobcron
go build -o "$ponytail_metrics_dir/base-jobcron-import" ./cmd/jobcron-import
go build -o "$ponytail_metrics_dir/base-jobcron-user" ./cmd/jobcron-user
git switch "$implementation_branch"
test "$(git rev-parse HEAD)" = "$implementation_sha"
go build -o "$ponytail_metrics_dir/final-jobcron" ./cmd/jobcron
go build -o "$ponytail_metrics_dir/final-jobcron-import" ./cmd/jobcron-import
go build -o "$ponytail_metrics_dir/final-jobcron-user" ./cmd/jobcron-user
wc -c "$ponytail_metrics_dir"/*jobcron*
```

Expected: both SHAs build sequentially at the same checkout path. Record per-binary and total
deltas; do not compare these sizes with binaries from another path or run.
