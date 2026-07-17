# Request Pacer Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add one request-start pacer and migrate AI, Demoday, Greenhouse, and Greeting
without changing request timing, cancellation, transport, or source policy.

**Architecture:** A concrete `internal/pacing.Pacer` owns only mutex-protected request-start
reservation and context-aware waiting. Each consumer still chooses its spacing and retains
HTTP construction, hosts, headers, errors, retries, robots policy, and response handling.

**Tech Stack:** Go 1.26, `context`, `sync`, `time`, AI and scraper HTTP clients.

## Global Constraints

- Batch: `PT4-008`; candidate: `PONY-003`.
- Start from the human-reviewed Task 5 tip supplied by Mayor, with clean porcelain.
- The reviewed base must contain this plan and the ledger entry marked `planned`.
- Production scope is exactly the new owner plus AI, Demoday, Greenhouse, and Greeting.
- Test scope is the new owner test plus existing AI and converted scraper tests.
- Do not convert Jumpit, Rallit, or Worknet; dependent batch `PT4-009` owns them.
- Keep each caller's current spacing value, constructor input, and zero-spacing test behavior.
- Keep the foundation net-negative while adding the owner; target at least 45 fewer lines.
- Add no interface, callback, generic, configuration, dependency, or HTTP abstraction.
- Never push, perform neighboring cleanup, or combine this batch with another reduction.

---

### Task 1: Characterize concurrent starts and cancellation

**Files:**

- Create: `internal/pacing/pacing_test.go`
- Modify: `internal/ai/client_test.go`
- Test: `internal/scraper/demoday/demoday_test.go`
- Test: `internal/scraper/greenhouse/greenhouse_test.go`
- Test: `internal/scraper/greeting/greeting_test.go`
- Evidence: `.superpowers/sdd/260717-ponytail/PT4-008-before.md`

**Interfaces:**

- Consumes: four current private `waitForRateLimit(context.Context) error` methods.
- Produces: passing concurrency and cancellation locks against the current AI copy, then moves
  those same locks to `pacing.Pacer`.

- [ ] **Step 1: Verify definitions, callers, and local policies**

```sh
git status --short --branch
git log -1 --format='%H %cI %s'
rg -n 'waitForRateLimit|rateLimit|lastRequest' \
  internal/ai/client.go internal/scraper/demoday/demoday.go \
  internal/scraper/greenhouse/greenhouse.go internal/scraper/greeting/greeting.go
rg -n 'waitForRateLimit|rateLimit|lastRequest' \
  internal/scraper/jumpit internal/scraper/rallit internal/scraper/worknet
```

Expected: exactly seven live pacing copies. Four are in this foundation and three remain for
`PT4-009`. No template, JavaScript, SQL, migration, reflection, or configuration consumer.

- [ ] **Step 2: Characterize the current AI copy before production edits**

Add `sort`, `sync`, and `time` to `internal/ai/client_test.go`, then add these tests:

```go
func TestWaitForRateLimitSpacesConcurrentStarts(t *testing.T) {
	const spacing = 40 * time.Millisecond
	p := &httpProvider{rateLimit: spacing}
	ready := make(chan struct{})
	times := make(chan time.Time, 3)
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ready
			if err := p.waitForRateLimit(context.Background()); err != nil {
				t.Errorf("waitForRateLimit: %v", err)
				return
			}
			times <- time.Now()
		}()
	}
	close(ready)
	wg.Wait()
	close(times)
	var got []time.Time
	for at := range times {
		got = append(got, at)
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Before(got[j]) })
	for i := 1; i < len(got); i++ {
		if gap := got[i].Sub(got[i-1]); gap < 30*time.Millisecond {
			t.Fatalf("start gap = %v, want at least 30ms", gap)
		}
	}
}

func TestWaitForRateLimitHonorsCancellation(t *testing.T) {
	p := &httpProvider{rateLimit: time.Hour}
	if err := p.waitForRateLimit(context.Background()); err != nil {
		t.Fatalf("first waitForRateLimit: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.waitForRateLimit(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForRateLimit error = %v, want context.Canceled", err)
	}
}
```

- [ ] **Step 3: Run both characterizations green on the unchanged copy**

```sh
gofmt -w internal/ai/client_test.go
go test ./internal/ai -run '^TestWaitForRateLimit' -count=10
```

Expected: PASS repeatedly before any production edit. A failure blocks extraction because the
shared owner would not yet have a trustworthy behavior lock.

- [ ] **Step 4: Record before measurements and existing locks**

```sh
go test ./internal/ai -run 'TestAnthropicClient|TestCompleteNon200' -count=1
go test ./internal/scraper/demoday ./internal/scraper/greenhouse \
  ./internal/scraper/greeting -count=1
git ls-files '*.go' ':(exclude)**/*_test.go' | xargs wc -l
git ls-files '**/*_test.go' | xargs wc -l
go list -m -f '{{if not .Indirect}}{{.Path}} {{.Version}}{{end}}' all
```

Expected: existing client suites pass. Record all output in the ignored before file.

### Task 2: Move the locks and add the narrow pacing owner

**Files:**

- Create: `internal/pacing/pacing.go`
- Create: `internal/pacing/pacing_test.go`

**Interfaces:**

- Produces: `func New(spacing time.Duration) *Pacer`.
- Produces: `func (p *Pacer) Wait(ctx context.Context) error`.

- [ ] **Step 1: Move the characterizations to the owner and verify red**

Move the two tests from `internal/ai/client_test.go` into `internal/pacing/pacing_test.go`.
Rename them to `TestPacerSpacesConcurrentStarts` and `TestPacerWaitHonorsCancellation`, change
the first construction to `p := New(spacing)`, the second to `p := New(time.Hour)`, and all calls
to `p.Wait(...)`. Remove imports made unused in the AI test file.

```sh
go test ./internal/pacing -run '^TestPacer' -count=1
```

Expected: FAIL because `New` and `Pacer.Wait` do not exist. The failure must come from the
wished-for API, not a typo or missing test import.

- [ ] **Step 2: Implement the exact shared algorithm**

```go
package pacing

import (
	"context"
	"sync"
	"time"
)

type Pacer struct {
	spacing   time.Duration
	mu        sync.Mutex
	lastStart time.Time
}

func New(spacing time.Duration) *Pacer {
	return &Pacer{spacing: spacing}
}

func (p *Pacer) Wait(ctx context.Context) error {
	p.mu.Lock()
	var wait time.Duration
	if !p.lastStart.IsZero() {
		if elapsed := time.Since(p.lastStart); elapsed < p.spacing {
			wait = p.spacing - elapsed
		}
	}
	p.lastStart = time.Now().Add(wait)
	p.mu.Unlock()
	if wait <= 0 {
		return nil
	}
	select {
	case <-time.After(wait):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
```

Do not add clock injection, an interface, callbacks, reset, metrics, or policy fields.

- [ ] **Step 3: Run owner tests green**

```sh
gofmt -w internal/pacing/pacing.go internal/pacing/pacing_test.go
go test ./internal/pacing -run '^TestPacer' -count=10
go test -race ./internal/pacing -run '^TestPacer' -count=10
```

Expected: both behaviors pass repeatedly and under the race detector.

### Task 3: Convert exactly four consumers

**Files:**

- Modify: `internal/ai/client.go`
- Modify: `internal/scraper/demoday/demoday.go`
- Modify: `internal/scraper/greenhouse/greenhouse.go`
- Modify: `internal/scraper/greeting/greeting.go`

**Interfaces:**

- Consumes: `pacing.New(time.Duration) *pacing.Pacer` and `Pacer.Wait(context.Context)`.
- Produces: four converted request helpers; three local implementations remain.

- [ ] **Step 1: Replace local pacing state in each consumer**

Replace each `rateLimit`, pacing mutex, and `lastRequest` field with:

```go
pacer *pacing.Pacer
```

Keep unrelated robots mutexes. In each existing constructor, replace `rateLimit: rateLimit`
with `pacer: pacing.New(rateLimit)`. Constructor signatures and callers remain unchanged.

- [ ] **Step 2: Route each request helper through the owner**

Replace calls such as:

```go
if err := p.waitForRateLimit(ctx); err != nil {
```

with the receiver-appropriate form:

```go
if err := p.pacer.Wait(ctx); err != nil {
```

Use `s.pacer.Wait(ctx)` in scraper receivers. Delete only the four private pacing methods and
remove `sync` only where no robots mutex still needs it. Keep `time` for spacing and TTLs.

- [ ] **Step 3: Run targeted green checks and ownership assertions**

```sh
gofmt -w internal/pacing/pacing.go internal/pacing/pacing_test.go \
  internal/ai/client.go internal/scraper/demoday/demoday.go \
  internal/scraper/greenhouse/greenhouse.go internal/scraper/greeting/greeting.go
go test ./internal/pacing ./internal/ai ./internal/scraper/demoday \
  ./internal/scraper/greenhouse ./internal/scraper/greeting -count=1
test "$(rg -l 'func .*waitForRateLimit' internal | sort)" = \
  $'internal/scraper/jumpit/client.go\ninternal/scraper/rallit/client.go\n'\
$'internal/scraper/worknet/client.go'
```

Expected: converted packages pass and only the three deferred copies remain.

### Task 4: Run full, live, and measurement gates

**Files:**

- Modify: none
- Evidence: `.superpowers/sdd/260717-ponytail/PT4-008-after.md`

**Interfaces:**

- Consumes: the owner plus four conversions.
- Produces: the exact reviewed implementation base required by `PT4-009`.

- [ ] **Step 1: Run static, build, unit, race, and coverage gates**

```sh
test -z "$(gofmt -l .)"
go vet ./...
go build ./...
go test ./... -count=1
go test -race ./... -count=1
go test ./... -coverprofile=/tmp/jobcron-ponytail-PT4-008.cover -count=1
go tool cover -func=/tmp/jobcron-ponytail-PT4-008.cover
```

Expected: every command passes.

- [ ] **Step 2: Run proportional AI and live scraper contracts**

```sh
go test ./internal/ai -run 'TestAnthropicClient|TestCompleteNon200|HTTPProvider' -count=1
go test -tags integration ./internal/scraper/demoday/ -count=1
go test -tags integration ./internal/scraper/greenhouse/ -count=1
go test -tags integration ./internal/scraper/greeting/ -count=1
```

Expected: local AI HTTP contracts and reachable live source contracts pass. Do not make a paid
provider call. PostgreSQL, browser, deployment, and cross-build gates are not proportional.

- [ ] **Step 3: Compare complete measurements**

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
```

Expected: at least 45 fewer production lines, unchanged direct dependencies, and recorded test
lines, binary sizes, and coverage.

### Task 5: Review and commit the foundation

**Files:**

- Create: `internal/pacing/pacing.go`
- Create: `internal/pacing/pacing_test.go`
- Modify: the four approved consumer files
- Evidence: `.superpowers/sdd/260717-ponytail/PT4-008-review.md`

**Interfaces:**

- Produces: one independently reversible pacer-owner commit.

- [ ] **Step 1: Run Ponytail and correctness/security review**

```sh
git diff -- internal/pacing internal/ai/client.go internal/scraper/demoday \
  internal/scraper/greenhouse internal/scraper/greeting
git diff --check
git status --short
```

Confirm only pacing moved. Reject changes to spacing values, start reservation, cancellation,
requests, hosts, headers, robots policy, retries, status handling, or errors.

- [ ] **Step 2: Scan and commit exactly the foundation**

```sh
gitleaks git --redact --no-banner
git add internal/pacing/pacing.go internal/pacing/pacing_test.go \
  internal/ai/client.go internal/scraper/demoday/demoday.go \
  internal/scraper/greenhouse/greenhouse.go internal/scraper/greeting/greeting.go
git diff --cached --check
git diff --cached
git commit -m 'refactor: add shared request pacer'
```

Expected: one commit. `PT4-009` must name this reviewed commit as its base. Full rollback after
both batches reverts `PT4-009` first, then this foundation commit.

## Completion Criteria

- AI, Demoday, Greenhouse, and Greeting use one concrete request-start pacer.
- Jumpit, Rallit, and Worknet remain local for `PT4-009`.
- Concurrent starts and cancellation retain executable behavior locks.
- Production lines fall by at least 45 with no dependency or policy change.
- Full, race, coverage, local AI, and relevant live scraper gates pass.
- One implementation commit exists with the exact message and reverse rollback order.
