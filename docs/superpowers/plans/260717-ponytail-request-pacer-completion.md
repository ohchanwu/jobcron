# Request Pacer Completion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move Jumpit, Rallit, and Worknet onto the reviewed request pacer and delete the last
three local pacing implementations.

**Architecture:** This batch consumes `internal/pacing.Pacer` exactly as approved in
`PT4-008`. Each client keeps its rate value, constructor, HTTP request, source error policy,
and robots cache; only pacing state and calls move to the existing owner.

**Tech Stack:** Go 1.26, shared request-start pacer, three scraper clients and live contracts.

## Global Constraints

- Batch: `PT4-009`; candidate: `PONY-003`.
- Start from the exact human-reviewed `PT4-008` implementation commit supplied by Mayor.
- Record that commit SHA and require `internal/pacing/pacing.go` before editing.
- The reviewed base must contain this plan and its ledger entry marked `planned`.
- Production scope is exactly Jumpit, Rallit, and Worknet client files.
- Test scope is Jumpit's timing test and focused Rallit and Worknet client tests.
- Do not edit, move, widen, or redefine the owner created by `PT4-008`.
- Preserve each client's spacing input, zero-spacing tests, cancellation, and request policy.
- Target at least 50 fewer production lines and add no dependency or abstraction.
- Compare binary deltas by building both SHAs sequentially at one checkout path with the same
  Go toolchain and environment and explicit temporary `-o` outputs.
- Never push, perform neighboring cleanup, or combine this batch with another reduction.

---

### Task 1: Lock the dependency and characterize remaining clients

**Files:**

- Test: `internal/scraper/jumpit/client_test.go`
- Create: `internal/scraper/rallit/client_test.go`
- Modify: `internal/scraper/worknet/client_test.go`
- Evidence: `.superpowers/sdd/260717-ponytail/PT4-009-before.md`

**Interfaces:**

- Consumes: `pacing.New(time.Duration) *pacing.Pacer` from `PT4-008`.
- Removes: three private `waitForRateLimit(context.Context) error` methods.

- [ ] **Step 1: Verify the reviewed owner and remaining copies**

```sh
git status --short --branch
git log -1 --format='%H %cI %s'
test -f internal/pacing/pacing.go
rg -n 'type Pacer|func New|func \(p \*Pacer\) Wait' internal/pacing
rg -n 'waitForRateLimit|rateLimit|lastRequest' \
  internal/scraper/jumpit/client.go internal/scraper/rallit/client.go \
  internal/scraper/worknet/client.go
```

Expected: Mayor's reviewed `PT4-008` commit is the exact parent, its owner is unchanged, and
the three approved clients are the only local pacing definitions.

- [ ] **Step 2: Add Jumpit's missing concurrent-start characterization**

Add `sort` to `internal/scraper/jumpit/client_test.go`, then add:

```go
func TestClientGetRateLimitsConcurrent(t *testing.T) {
	const spacing = 40 * time.Millisecond
	var (
		mu    sync.Mutex
		times []time.Time
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newClient(srv.URL, spacing)
	start := make(chan struct{})
	errs := make(chan error, 3)
	for range 3 {
		go func() {
			<-start
			_, err := c.get(context.Background(), "/api/positions")
			errs <- err
		}()
	}
	close(start)
	for range 3 {
		if err := <-errs; err != nil {
			t.Fatalf("get: %v", err)
		}
	}

	mu.Lock()
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	mu.Unlock()
	for i := 1; i < len(times); i++ {
		if gap := times[i].Sub(times[i-1]); gap < 30*time.Millisecond {
			t.Fatalf("concurrent request gap %d = %v, want at least 30ms", i, gap)
		}
	}
}
```

This is the approved Jumpit concurrency lock; the older sequential timing test remains.

- [ ] **Step 3: Add Rallit's passing timing characterization**

Create `internal/scraper/rallit/client_test.go` with the complete file:

```go
package rallit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestClientGetRateLimits(t *testing.T) {
	var (
		mu    sync.Mutex
		times []time.Time
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := newClient(srv.URL, 40*time.Millisecond)
	for range 2 {
		if _, err := c.get(context.Background(), "/jobs"); err != nil {
			t.Fatalf("get: %v", err)
		}
	}
	if gap := times[1].Sub(times[0]); gap < 30*time.Millisecond {
		t.Fatalf("request gap = %v, want at least 30ms", gap)
	}
}
```

- [ ] **Step 4: Add Worknet's passing timing characterization**

Add `sync` and `time` to `internal/scraper/worknet/client_test.go`, then add:

```go
func TestCallRateLimits(t *testing.T) {
	var (
		mu    sync.Mutex
		times []time.Time
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := newClient(srv.URL, "test-key", 40*time.Millisecond)
	for range 2 {
		if _, err := c.call(context.Background(), url.Values{}); err != nil {
			t.Fatalf("call: %v", err)
		}
	}
	if gap := times[1].Sub(times[0]); gap < 30*time.Millisecond {
		t.Fatalf("request gap = %v, want at least 30ms", gap)
	}
}
```

- [ ] **Step 5: Run all timing locks before production edits**

```sh
gofmt -w internal/scraper/rallit/client_test.go \
  internal/scraper/worknet/client_test.go
go test ./internal/pacing -run '^TestPacer' -count=10
go test ./internal/scraper/jumpit -run '^TestClientGetRateLimits' -count=10
go test ./internal/scraper/rallit -run '^TestClientGetRateLimits$' -count=10
go test ./internal/scraper/worknet -run '^TestCallRateLimits$' -count=10
go test ./... -coverprofile=/tmp/jobcron-ponytail-PT4-009-before.cover -count=1
go tool cover -func=/tmp/jobcron-ponytail-PT4-009-before.cover
ponytail_metrics_dir=$(mktemp -d)
go build -o "$ponytail_metrics_dir/jobcron" ./cmd/jobcron
go build -o "$ponytail_metrics_dir/jobcron-import" ./cmd/jobcron-import
go build -o "$ponytail_metrics_dir/jobcron-user" ./cmd/jobcron-user
wc -c "$ponytail_metrics_dir"/*
git ls-files '*.go' ':(exclude)**/*_test.go' | xargs wc -l
git ls-files '**/*_test.go' | xargs wc -l
go list -m -f '{{if not .Indirect}}{{.Path}} {{.Version}}{{end}}' all
```

Expected: all tests pass against the reviewed owner or current local implementations. Record
the exact base, timings, source/test lines, dependencies, binary sizes, and coverage.

### Task 2: Convert exactly three remaining consumers

**Files:**

- Modify: `internal/scraper/jumpit/client.go`
- Modify: `internal/scraper/rallit/client.go`
- Modify: `internal/scraper/worknet/client.go`
- Test: the three focused client test files from Task 1

**Interfaces:**

- Consumes: unchanged `pacing.Pacer` from `PT4-008`.
- Produces: no source-local request pacer.

- [ ] **Step 1: Replace each client's local pacing state**

Replace:

```go
rateLimit   time.Duration
mu          sync.Mutex
lastRequest time.Time
```

with:

```go
pacer *pacing.Pacer
```

Keep each robots mutex. In `newClient`, replace `rateLimit: rateLimit` with
`pacer: pacing.New(rateLimit)`. Do not change constructor signatures or scraper callers.

- [ ] **Step 2: Route request helpers through the reviewed owner**

Replace the first line in Jumpit's `fetch`, Rallit's `get`, and Worknet's `call`:

```go
if err := c.waitForRateLimit(ctx); err != nil {
```

with:

```go
if err := c.pacer.Wait(ctx); err != nil {
```

Delete only the three private pacing methods. Keep HTTP methods, URLs, keys, headers, status
handling, errors, response parsing, robots caches, and source timing constants unchanged.

- [ ] **Step 3: Run targeted green checks and ownership assertions**

```sh
gofmt -w internal/scraper/jumpit/client.go internal/scraper/jumpit/client_test.go \
  internal/scraper/rallit/client.go internal/scraper/rallit/client_test.go \
  internal/scraper/worknet/client.go internal/scraper/worknet/client_test.go
go test ./internal/pacing ./internal/scraper/jumpit \
  ./internal/scraper/rallit ./internal/scraper/worknet -count=1
test "$(rg -l 'func .*waitForRateLimit' internal || true)" = ""
test "$(rg -l 'type Pacer' internal)" = "internal/pacing/pacing.go"
```

Expected: all focused suites pass, no local pacing method remains, and the owner is unchanged.

### Task 3: Run full, live, and measurement gates

**Files:**

- Modify: none
- Evidence: `.superpowers/sdd/260717-ponytail/PT4-009-after.md`

**Interfaces:**

- Consumes: all seven converted pacing consumers.
- Produces: final correctness, size, and reverse-rollback evidence.

- [ ] **Step 1: Run static, build, unit, race, and coverage gates**

```sh
test -z "$(gofmt -l .)"
go vet ./...
go build ./...
go test ./... -count=1
go test -race ./... -count=1
go test ./... -coverprofile=/tmp/jobcron-ponytail-PT4-009.cover -count=1
go tool cover -func=/tmp/jobcron-ponytail-PT4-009.cover
```

Expected: every command passes.

- [ ] **Step 2: Run the reachable live scraper contracts**

```sh
go test -tags integration ./internal/scraper/jumpit/ -count=1
go test -tags integration ./internal/scraper/rallit/ -count=1
```

Expected: both Task 1 live contracts pass. Worknet was not in the Task 1 live baseline because
it requires an optional data.go.kr key; its deterministic HTTP client test is mandatory.
PostgreSQL, AI, browser, deployment, and cross-build gates are not proportional.

- [ ] **Step 3: Capture after metrics and scope**

```sh
ponytail_metrics_dir=$(mktemp -d)
go build -o "$ponytail_metrics_dir/jobcron" ./cmd/jobcron
go build -o "$ponytail_metrics_dir/jobcron-import" ./cmd/jobcron-import
go build -o "$ponytail_metrics_dir/jobcron-user" ./cmd/jobcron-user
wc -c "$ponytail_metrics_dir"/*
git ls-files '*.go' ':(exclude)**/*_test.go' | xargs wc -l
git ls-files '**/*_test.go' | xargs wc -l
go list -m -f '{{if not .Indirect}}{{.Path}} {{.Version}}{{end}}' all
git diff --numstat
git diff --name-only
```

Expected: at least 50 fewer production lines, no dependency drift, and only approved source,
test, and ignored evidence paths change. Record binary and coverage differences.

### Task 4: Review, commit, and preserve reverse rollback

**Files:**

- Modify: the three approved clients and focused tests
- Evidence: `.superpowers/sdd/260717-ponytail/PT4-009-review.md`

**Interfaces:**

- Produces: one independently reversible completion commit.

- [ ] **Step 1: Run both review lenses**

Run Ponytail review in supported full mode for deletion completeness and owner reuse. Then run a
normal correctness and security review. Reject owner edits, timing changes, missing pacing calls,
request-policy changes, optional-key disclosure, and dependency drift.

- [ ] **Step 2: Inspect, scan, and commit exactly this batch**

```sh
git diff --check
git diff --stat
git diff
gitleaks git --redact --no-banner
git add internal/scraper/jumpit/client.go internal/scraper/jumpit/client_test.go \
  internal/scraper/rallit/client.go internal/scraper/rallit/client_test.go \
  internal/scraper/worknet/client.go internal/scraper/worknet/client_test.go
git diff --cached --check
git diff --cached
git commit -m 'refactor(scraper): finish request pacer migration'
```

Expected: one commit and no unrelated path staged.

- [ ] **Step 3: Record rollback order**

This commit is independently reversible while `PT4-008` remains. To roll back the whole pacing
cluster, revert `PT4-009` first and then the exact reviewed `PT4-008` implementation commit.
Never revert the foundation first while these clients import its owner.

## Completion Criteria

- All seven live consumers use the unchanged owner created by `PT4-008`.
- No source-local request pacing method remains.
- Concurrent timing and cancellation behavior stay locked.
- Production lines fall by at least 50; dependencies, binaries, tests, and coverage are recorded.
- Full, race, coverage, focused client, and reachable live gates pass.
- One implementation commit exists with the exact message and reverse rollback order.
