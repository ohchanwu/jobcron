# Scheduler API Reduction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the unused scheduler handle and return only startup validation errors while
preserving the same background scheduler loop.

**Architecture:** `internal/server.StartScheduler` remains the lifecycle owner. It validates
configuration, starts the existing context-owned goroutine, and returns `error`; the unobserved
`Scheduler`, `done` channel, and `Done` method disappear without a replacement abstraction.

**Tech Stack:** Go 1.26, `context`, Jobcron server scheduler and command startup.

## Global Constraints

- Batch: `PT4-010`; candidate: `PONY-005`.
- Start from the human-reviewed Task 5 tip supplied by Mayor, with clean porcelain.
- The reviewed base must contain this plan and the ledger entry marked `planned`.
- Production scope is exactly `internal/server/scheduler.go` and `cmd/jobcron/main.go`.
- Test scope is exactly `internal/server/scheduler_test.go` unless evidence proves a gap.
- Preserve startup validation, KST calculation, injectable clock/sleep, loop, and cancellation.
- Preserve scheduler enablement, logging, scrape serialization, owner resolution, and AI policy.
- Remove the handle; do not replace it with another type, channel, callback, or interface.
- Target at least 10 fewer production lines and add no dependency or configuration.
- Compare binary deltas by building both SHAs sequentially at one checkout path with the same
  Go toolchain and environment and explicit temporary `-o` outputs.
- Never push, perform neighboring cleanup, or combine this batch with another reduction.

---

### Task 1: Prove the handle is unobserved and lock behavior

**Files:**

- Test: `internal/server/scheduler_test.go`
- Evidence: `.superpowers/sdd/260717-ponytail/PT4-010-before.md`

**Interfaces:**

- Consumes: current `func StartScheduler(context.Context, SchedulerConfig) (*Scheduler, error)`.
- Removes: `Scheduler`, `Scheduler.done`, and `func (*Scheduler) Done() <-chan struct{}`.

- [ ] **Step 1: Trace every definition and consumer**

```sh
git status --short --branch
git log -1 --format='%H %cI %s'
rg -n 'type Scheduler|func \(s \*Scheduler\) Done|StartScheduler\(|\.Done\(' \
  cmd internal --glob '*.go'
rg -n 'JOBCRON_SCHEDULER_ENABLED|DailyScrapeTime|SchedulerEnabled' \
  cmd internal deploy scripts README.md README.ko.md
git log --oneline -S'StartScheduler' -- internal/server/scheduler.go cmd/jobcron/main.go
```

Expected: one definition, one command caller, and two tests. Every caller discards the handle;
no template, JavaScript, SQL, migration, reflection, build-tagged, or external module consumer.

- [ ] **Step 2: Run the existing behavior locks before editing**

```sh
go test ./internal/server \
  -run '^TestStartSchedulerRunsScheduledScrapeAfterSleep$' -count=10
go test ./internal/server \
  -run '^TestStartSchedulerRecordsSkippedRunWhenScrapeLockBusy$' -count=10
go test -race ./internal/server \
  -run '^TestStartSchedulerRunsScheduledScrapeAfterSleep$' -count=10
go test -race ./internal/server \
  -run '^TestStartSchedulerRecordsSkippedRunWhenScrapeLockBusy$' -count=10
```

Expected: scheduled execution and busy-lock recording pass repeatedly. Existing coverage is
sufficient because both tests drive the live goroutine through injected `Now` and `Sleep`.

- [ ] **Step 3: Record complete before measurements**

```sh
go test ./... -coverprofile=/tmp/jobcron-ponytail-PT4-010-before.cover -count=1
go tool cover -func=/tmp/jobcron-ponytail-PT4-010-before.cover
git ls-files '*.go' ':(exclude)**/*_test.go' | xargs wc -l
git ls-files '**/*_test.go' | xargs wc -l
go list -m -f '{{if not .Indirect}}{{.Path}} {{.Version}}{{end}}' all
```

Expected: record source/test lines, direct dependencies, and coverage in the ignored before
file.

### Task 2: Delete the handle and narrow the startup API

**Files:**

- Modify: `internal/server/scheduler.go`
- Modify: `cmd/jobcron/main.go`
- Modify: `internal/server/scheduler_test.go`

**Interfaces:**

- Produces: `func StartScheduler(ctx context.Context, cfg SchedulerConfig) error`.
- Preserves: `SchedulerConfig` and the context-owned background loop.

- [ ] **Step 1: Remove the unused type and change the signature**

Delete:

```go
type Scheduler struct {
	done chan struct{}
}

func (s *Scheduler) Done() <-chan struct{} { return s.done }
```

Change the signature to:

```go
func StartScheduler(ctx context.Context, cfg SchedulerConfig) error
```

Change the nil-server branch from:

```go
return nil, fmt.Errorf("scheduler: server is required")
```

to:

```go
return fmt.Errorf("scheduler: server is required")
```

Change `return nil, err` after `nextScheduledRun` to `return err`. Keep the validation order
and error text unchanged.

- [ ] **Step 2: Start the identical goroutine without a completion channel**

Replace:

```go
s := &Scheduler{done: make(chan struct{})}
go func() {
	defer close(s.done)
	for {
		now := cfg.Now()
		next, err := nextScheduledRun(now, cfg.DailyScrapeTime)
		if err != nil {
			return
		}
		delay := next.Sub(now.In(kstLocation()))
		if delay < 0 {
			delay = 0
		}
		if err := cfg.Sleep(ctx, delay); err != nil {
			return
		}
		cfg.Server.runScheduledScrape(ctx)
	}
}()
return s, nil
```

with:

```go
go func() {
	for {
		now := cfg.Now()
		next, err := nextScheduledRun(now, cfg.DailyScrapeTime)
		if err != nil {
			return
		}
		delay := next.Sub(now.In(kstLocation()))
		if delay < 0 {
			delay = 0
		}
		if err := cfg.Sleep(ctx, delay); err != nil {
			return
		}
		cfg.Server.runScheduledScrape(ctx)
	}
}()
return nil
```

Do not change loop exits, delay calculation, context propagation, or `runScheduledScrape`.

- [ ] **Step 3: Run the expected compile-red boundary**

```sh
gofmt -w internal/server/scheduler.go
go test ./internal/server ./cmd/jobcron -count=1
```

Expected: FAIL with assignment-mismatch errors because three callers still expect two return
values. Any scheduler behavior failure before caller updates indicates an unintended edit.

- [ ] **Step 4: Update the command and two tests**

In `cmd/jobcron/main.go`, replace:

```go
if _, err := server.StartScheduler(appCtx, server.SchedulerConfig{
```

with:

```go
if err := server.StartScheduler(appCtx, server.SchedulerConfig{
```

In both scheduler tests, replace `_, err := StartScheduler(` with
`err := StartScheduler(`. Do not change their clocks, sleeps, assertions, or cleanup.

- [ ] **Step 5: Run targeted green checks and deletion assertions**

```sh
gofmt -w internal/server/scheduler.go internal/server/scheduler_test.go \
  cmd/jobcron/main.go
go test ./internal/server \
  -run '^TestStartSchedulerRunsScheduledScrapeAfterSleep$' -count=10
go test ./internal/server \
  -run '^TestStartSchedulerRecordsSkippedRunWhenScrapeLockBusy$' -count=10
go test -race ./internal/server \
  -run '^TestStartSchedulerRunsScheduledScrapeAfterSleep$' -count=10
go test -race ./internal/server \
  -run '^TestStartSchedulerRecordsSkippedRunWhenScrapeLockBusy$' -count=10
test "$(rg -n 'type Scheduler struct|\.Done\(\)|&Scheduler|s\.done' \
  internal/server cmd/jobcron || true)" = ""
test "$(rg -l 'StartScheduler\(' internal/server cmd/jobcron | sort)" = \
  $'cmd/jobcron/main.go\ninternal/server/scheduler.go\ninternal/server/scheduler_test.go'
```

Expected: both behavior locks pass and no handle definition, construction, method, or channel
reference remains.

### Task 3: Run full and measurement gates

**Files:**

- Modify: none
- Evidence: `.superpowers/sdd/260717-ponytail/PT4-010-after.md`

**Interfaces:**

- Consumes: narrowed scheduler startup API.
- Produces: complete verification and rollback evidence.

- [ ] **Step 1: Run static, build, unit, race, and coverage gates**

```sh
test -z "$(gofmt -l .)"
go vet ./...
go build ./...
go test ./... -count=1
go test -race ./... -count=1
go test ./... -coverprofile=/tmp/jobcron-ponytail-PT4-010.cover -count=1
go tool cover -func=/tmp/jobcron-ponytail-PT4-010.cover
```

Expected: every command passes. Scheduler startup and goroutine behavior are fully local; live
scraper, paid AI, PostgreSQL, browser, deployment, and cross-build gates are not proportional.

- [ ] **Step 2: Compare source, dependency, coverage, and API scope metrics**

```sh
git ls-files '*.go' ':(exclude)**/*_test.go' | xargs wc -l
git ls-files '**/*_test.go' | xargs wc -l
go list -m -f '{{if not .Indirect}}{{.Path}} {{.Version}}{{end}}' all
git diff --numstat
git diff --name-only
```

Expected: at least 10 fewer production lines, unchanged tests and direct dependencies, and
only the two approved production files plus the scheduler test and ignored evidence change.

### Task 4: Review and commit the deletion

**Files:**

- Modify: `internal/server/scheduler.go`
- Modify: `cmd/jobcron/main.go`
- Modify: `internal/server/scheduler_test.go`
- Evidence: `.superpowers/sdd/260717-ponytail/PT4-010-review.md`

**Interfaces:**

- Produces: one reversible scheduler-API reduction commit.

- [ ] **Step 1: Run Ponytail and correctness/security review**

```sh
git diff -- internal/server/scheduler.go internal/server/scheduler_test.go \
  cmd/jobcron/main.go
git diff --check
git status --short
```

Confirm only the unobserved handle and discarded return value disappear. Reject any change to
validation, schedule calculation, goroutine exits, scrape locking, owner selection, AI policy,
configuration, logs, errors, or request behavior.

- [ ] **Step 2: Scan and commit exactly this batch**

```sh
gitleaks git --redact --no-banner
git add internal/server/scheduler.go internal/server/scheduler_test.go \
  cmd/jobcron/main.go
git diff --cached --check
git diff --cached
git commit -m 'refactor(server): remove unused scheduler handle'
```

Expected: one commit and no unrelated path staged.

- [ ] **Step 3: Record the rollback boundary**

Reverting this one commit restores the old exported return signature, wrapper type, done
channel, command discard, and test assignments together. No other approved batch depends on
the narrowed scheduler API.

- [ ] **Step 4: Measure the exact base and implementation binaries at one path**

After the implementation commit, use the same-checkout procedure from `PT4-004` Task 4 Step 3.
Substitute this batch's exact Mayor-supplied base SHA, keep the same Go environment and explicit
temporary `-o` outputs, and record per-binary and total deltas. Restore and verify the
implementation branch before handoff; never compare binaries from different paths or runs.

## Completion Criteria

- `StartScheduler` returns only startup validation errors.
- The context-owned background loop and all observable scheduler behavior remain unchanged.
- `Scheduler`, its channel, and `Done` are deleted without replacement.
- Production lines fall by at least 10; dependencies, binaries, tests, and coverage are recorded.
- Full, targeted, race, coverage, and deletion gates pass.
- One implementation commit exists with the exact message and rollback boundary.
