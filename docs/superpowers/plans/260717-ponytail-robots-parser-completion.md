# Robots Parser Completion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move Rallit onto the reviewed shared robots parser and delete its last local copy.

**Architecture:** This completion batch consumes `internal/scraper.RobotsAllows` exactly as
approved in `PT4-006`. Rallit retains its fetch, cache, host, path, and failure policy; only the
parser call and duplicate parser implementation change.

**Tech Stack:** Go 1.26, shared scraper primitive, Rallit unit and live integration tests.

## Global Constraints

- Batch: `PT4-007`; candidate: `PONY-002`.
- Start from the exact human-reviewed `PT4-006` implementation commit supplied by Mayor.
- Record that commit SHA and require `internal/scraper/robots.go` to exist before editing.
- The reviewed base must contain this plan and its ledger entry marked `planned`.
- Production scope is exactly `internal/scraper/rallit/client.go`.
- Do not edit, move, or redefine the owner created by `PT4-006`.
- Do not change Rallit fetches, status handling, cache TTL, cache key, host, path, or errors.
- Target at least 40 fewer production lines and add no dependency or abstraction.
- Never push, perform neighboring cleanup, or combine this batch with another reduction.

---

### Task 1: Verify the dependency and characterize Rallit

**Files:**

- Test: `internal/scraper/rallit/client_test.go`
- Evidence: `.superpowers/sdd/260717-ponytail/PT4-007-before.md`

**Interfaces:**

- Consumes: `func scraper.RobotsAllows(content []byte, path string) bool` from `PT4-006`.
- Removes: Rallit's local `robotsAllows` and `longestPrefixMatch`.

- [ ] **Step 1: Lock the reviewed dependency**

```sh
git status --short --branch
git log -1 --format='%H %cI %s'
test -f internal/scraper/robots.go
rg -n 'func RobotsAllows|func robotsAllows|func longestPrefix' internal/scraper
```

Expected: Mayor's reviewed `PT4-006` implementation is the exact parent; the shared owner and
Rallit's one remaining local parser are present. Stop on any other base or consumer.

- [ ] **Step 2: Run the existing parser and access tests before editing**

```sh
go test ./internal/scraper -run '^TestRobotsAllows$' -count=1
go test ./internal/scraper/rallit -count=1
git ls-files '*.go' ':(exclude)**/*_test.go' | xargs wc -l
git ls-files '**/*_test.go' | xargs wc -l
go list -m -f '{{if not .Indirect}}{{.Path}} {{.Version}}{{end}}' all
```

Expected: shared characterization and Rallit tests pass. Record output in the ignored before
file. If Rallit lacks a local access assertion, add one before the production edit and see it
pass against the local parser.

### Task 2: Convert the final parser consumer

**Files:**

- Modify: `internal/scraper/rallit/client.go`
- Test: `internal/scraper/rallit/client_test.go`

**Interfaces:**

- Consumes: `scraper.RobotsAllows([]byte, string) bool`.
- Deletes: two private Rallit parser functions.

- [ ] **Step 1: Replace only the parser call**

Replace:

```go
allowed = robotsAllows(body, robotsCheckPath)
```

with:

```go
allowed = scraper.RobotsAllows(body, robotsCheckPath)
```

Add the existing `internal/scraper` package import. Do not move the surrounding access logic.

- [ ] **Step 2: Delete the local parser and helper**

Delete `robotsAllows` and `longestPrefixMatch` from `client.go`. Remove only imports made unused
by those deletions. Do not edit `internal/scraper/robots.go` or its owner test.

- [ ] **Step 3: Run targeted green checks and ownership assertions**

```sh
gofmt -w internal/scraper/rallit/client.go internal/scraper/rallit/client_test.go
go test ./internal/scraper ./internal/scraper/rallit -count=1
test "$(rg -l 'func robotsAllows' internal/scraper || true)" = ""
test "$(rg -l 'func RobotsAllows' internal/scraper)" = \
  "internal/scraper/robots.go"
```

Expected: tests pass, no local parser remains, and the `PT4-006` owner is unchanged.

### Task 3: Run full, live, and measurement gates

**Files:**

- Modify: none
- Evidence: `.superpowers/sdd/260717-ponytail/PT4-007-after.md`

**Interfaces:**

- Consumes: completed five-source parser migration.
- Produces: final correctness, size, and rollback evidence.

- [ ] **Step 1: Run static, build, unit, race, and coverage gates**

```sh
test -z "$(gofmt -l .)"
go vet ./...
go build ./...
go test ./... -count=1
go test -race ./... -count=1
go test ./... -coverprofile=/tmp/jobcron-ponytail-PT4-007.cover -count=1
go tool cover -func=/tmp/jobcron-ponytail-PT4-007.cover
```

Expected: every command passes.

- [ ] **Step 2: Run the relevant live Rallit contract**

```sh
go test -tags integration ./internal/scraper/rallit/ -count=1
```

Expected: the reachable live contract passes. An upstream outage blocks this batch rather than
being waived. PostgreSQL, AI, browser, and deployment gates are not proportional.

- [ ] **Step 3: Capture after metrics and scope**

```sh
git ls-files '*.go' ':(exclude)**/*_test.go' | xargs wc -l
git ls-files '**/*_test.go' | xargs wc -l
go list -m -f '{{if not .Indirect}}{{.Path}} {{.Version}}{{end}}' all
ponytail_metrics_dir=$(mktemp -d)
go build -o "$ponytail_metrics_dir/jobcron" ./cmd/jobcron
go build -o "$ponytail_metrics_dir/jobcron-import" ./cmd/jobcron-import
go build -o "$ponytail_metrics_dir/jobcron-user" ./cmd/jobcron-user
wc -c "$ponytail_metrics_dir"/*
git diff --numstat
git diff --name-only
```

Expected: production delta is at most `-40`, dependencies are unchanged, and only the approved
production file plus necessary test/evidence files changed.

### Task 4: Review, commit, and preserve reverse rollback

**Files:**

- Modify: none
- Evidence: `.superpowers/sdd/260717-ponytail/PT4-007-review.md`

**Interfaces:**

- Produces: one independently reversible completion commit.

- [ ] **Step 1: Run both review lenses**

Run Ponytail review in supported full mode for deletion completeness and owner reuse. Then run a
normal correctness and security review of the full diff. Reject dependency drift, policy moves,
or any edit to the shared owner. Record findings and resolutions.

- [ ] **Step 2: Inspect, scan, and commit exactly this batch**

```sh
git diff --check
git diff --stat
git diff
gitleaks git --redact --no-banner
git status --short
git add internal/scraper/rallit/client.go internal/scraper/rallit/client_test.go
git diff --cached --check
git diff --cached
git commit -m 'refactor(scraper): finish robots parser migration'
```

Expected: one commit, no secret or publication concern, and no unrelated path staged. Omit the
test file if characterization was already sufficient and it remained unchanged.

- [ ] **Step 3: Record rollback order**

This commit is independently reversible while `PT4-006` remains. To roll back the whole robots
cluster, revert `PT4-007` first and then the exact reviewed `PT4-006` implementation commit.
Never revert the foundation first while Rallit imports its owner.

## Completion Criteria

- Rallit consumes the unchanged parser owner created by `PT4-006`.
- No source-local robots parser remains.
- Rallit's observable access behavior is unchanged.
- Production lines fall by at least 40; tests, dependencies, and coverage are recorded.
- Full, race, coverage, and relevant live gates pass.
- One implementation commit exists with the exact message and reverse rollback order.
