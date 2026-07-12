# Alpha Milestone A Polishes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make AI evaluation visibly active, recover its progress when the user returns through browser history, clearly explain zero-token no-op runs, give AI evidence its approved Electric-indigo treatment, and default Demoday off.

**Architecture:** Keep `/api/rerate` as the existing SSE start-and-stream path. Add a process-local, per-user/per-surface tracker and a read-only JSON status endpoint; the original page consults that endpoint only after back/forward restoration and polls while the detached run remains active. Successful runs pass a one-time notice through `sessionStorage` before a single reload, while a monotonic run ID prevents stale terminal state from triggering repeated reloads.

**Tech Stack:** Go 1.22+ standard library, embedded Go HTML templates and assets, vanilla JavaScript (`EventSource`, `fetch`, `pageshow`, `sessionStorage`), CSS custom properties, SQLite/PostgreSQL-compatible profile storage, Go tests, and gstack `/browse` for browser QA.

## Global Constraints

- Keep commits local. Never push, create a pull request, or run a deployment skill.
- Keep the current SSE flow for a run started on the visible page; use status polling only to repair back/forward restoration.
- Do not show rerate progress on the page the user navigated to. Restore it only when the original history entry becomes active again.
- Keep rerate state in memory only. It does not survive application restart and must not add a database table, queue, worker service, or external dependency.
- Preserve the detached `context.Background()` timeout so navigation never cancels paid AI work or the terminal `scoreAll`.
- Render the active copy exactly as `AI로 다시 분석하는 중이에요 — 여러 공고를 한 번에 살펴보고 있어요. ☕` and progress as `공고 N/M 분석 중...`.
- After a run completes while the user is away, reload the original page exactly once and show `AI 평가가 완료됐어요. 새로운 평가 결과를 반영했습니다.` exactly once.
- A fully cached press must make zero provider calls and show `이미 모든 공고가 AI로 평가됐습니다. 추가 토큰은 사용하지 않았어요.`.
- Rename the visible control from `재평가` to `AI 평가`; stale counts remain appended as `AI 평가 ·N`.
- Respect `prefers-reduced-motion: reduce`; animation must not hide content, animate live-region text, or shift the page layout.
- Apply Electric indigo only to the AI analysis chip, its evidence panel, values, border, caret, and focus ring. Never tint the surrounding listing card.
- Electric-indigo light tokens: chip `#eeeafe`, panel `#f3f0ff`, border `#c9bdfa`, text `#3f307c`, accent/focus `#6748c7`.
- Electric-indigo dark tokens: chip `#29233f`, panel `#211c34`, border `#5b4d8d`, text `#ede8ff`, accent/focus `#b7a7ff`.
- Preserve the AI chip's keyboard behavior, expanded/collapsed semantics, existing shape and spacing, and 44px mobile touch target.
- Default Demoday off for a brand-new profile and update the current alpha user's saved profile once; preserve the ability to re-enable Demoday.
- Do not change the existing USD-cent AI budget fields, storage, labels, or token-cap calculations.
- Use Tier C frontend verification: every UI page, light and dark themes, desktop and mobile, real interactions, no console errors, and no horizontal overflow.
- Use `/browse` for all browser work and never open the user's default browser.

---

## Planned File Structure

- Create `internal/server/rerate_status.go`: thread-safe process-local rerate snapshots and `GET /api/rerate/status`.
- Create `internal/server/rerate_status_test.go`: state-transition, stale-run, validation, and JSON endpoint coverage.
- Modify `internal/server/server.go`: own and initialize the tracker; adapt the scrape caller to the richer rerate result.
- Modify `internal/server/handlers.go`: register the status route and apply source defaults only when no profile exists.
- Modify `internal/server/rerate.go`: publish every status/progress/terminal event to the tracker and count provider attempts.
- Modify `internal/server/ai_rerate_test.go`: cover terminal snapshots, no-op copy, zero provider calls, and button copy.
- Modify `internal/server/ai_config_test.go` and `internal/server/ai_injection_test.go`: adapt existing `runRerate` calls to the result struct.
- Modify `internal/server/server_test.go`: make the scraper test double source-configurable and cover new/existing Demoday behavior.
- Modify `internal/server/sources.go`: centralize the Demoday default and preserve explicit saved settings.
- Modify `web/_rerate.html`: rename the control and add non-live-region activity markup.
- Replace `web/ai-rerate.js`: separate SSE start, history recovery, terminal notice, and UI rendering responsibilities.
- Modify `web/styles.css`: add activity animation and Electric-indigo semantic tokens/styles.
- Create `web/styles_test.go`: pin the approved color tokens and reduced-motion rule.
- Create `docs/superpowers/archive/2026-07-12-alpha-milestone-a-polishes/260712-alpha-milestone-a-polishes-verification.md`: final test and browser evidence.
- Modify `docs/superpowers/README.md`: keep active links correct during implementation, then point to archived records after completion.
- Modify `docs/README.md`: link the active plan, then replace it with the archived verification record after completion.

---

### Task 1: Process-Local Rerate Status Tracker

**Files:**

- Create: `internal/server/rerate_status.go`
- Create: `internal/server/rerate_status_test.go`
- Modify: `internal/server/server.go:105-131,298-324`
- Modify: `internal/server/handlers.go:23-47`
- Modify: `internal/server/auth_test.go:45-52`

**Interfaces:**

- Produces: `newRerateTracker() *rerateTracker`
- Produces: `(*rerateTracker).start(userID int64, surface string) rerateStatus`
- Produces: `(*rerateTracker).record(userID int64, surface string, runID uint64, event, data string)`
- Produces: `(*rerateTracker).snapshot(userID int64, surface string) (rerateStatus, bool)`
- Produces: `GET /api/rerate/status?surface=<today|bookmarks|archive>` returning `rerateStatus` JSON with `Cache-Control: no-store`
- Consumes: `validRerateSurface`, `stateUserID`, and the existing production authentication middleware

- [ ] **Step 1: Write tracker state-transition tests**

Create `internal/server/rerate_status_test.go` with the package and imports below, then add the first two tests:

```go
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRerateTrackerRecordsLifecycle(t *testing.T) {
	tracker := newRerateTracker()
	started := tracker.start(7, "today")
	tracker.record(7, "today", started.RunID, "status", "AI로 다시 분석하는 중이에요")
	tracker.record(7, "today", started.RunID, "progress", "공고 3/9 분석 중...")

	running, ok := tracker.snapshot(7, "today")
	if !ok || running.State != rerateStateRunning {
		t.Fatalf("running snapshot = %+v, ok=%v", running, ok)
	}
	if running.Status != "AI로 다시 분석하는 중이에요" || running.Progress != "공고 3/9 분석 중..." {
		t.Fatalf("running copy = %+v", running)
	}

	tracker.record(7, "today", started.RunID, "done", "완료")
	done, _ := tracker.snapshot(7, "today")
	if done.State != rerateStateDone || done.Message != "완료" {
		t.Fatalf("done snapshot = %+v", done)
	}
}

func TestRerateTrackerIgnoresStaleRunUpdates(t *testing.T) {
	tracker := newRerateTracker()
	old := tracker.start(7, "today")
	current := tracker.start(7, "today")
	tracker.record(7, "today", old.RunID, "done", "stale")

	got, _ := tracker.snapshot(7, "today")
	if got.RunID != current.RunID || got.State != rerateStateRunning || got.Message != "" {
		t.Fatalf("snapshot accepted stale update: %+v", got)
	}
}
```

- [ ] **Step 2: Run the tracker tests and verify the symbols are missing**

Run:

```bash
go test ./internal/server -run 'TestRerateTracker' -count=1
```

Expected: FAIL because `newRerateTracker`, `rerateStateRunning`, and `rerateStateDone` do not exist.

- [ ] **Step 3: Implement the complete tracker and JSON handler**

Create `internal/server/rerate_status.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"sync"
)

type rerateState string

const (
	rerateStateIdle    rerateState = "idle"
	rerateStateRunning rerateState = "running"
	rerateStateDone    rerateState = "done"
	rerateStateFailed  rerateState = "failed"
)

type rerateKey struct {
	userID  int64
	surface string
}

type rerateStatus struct {
	RunID    uint64      `json:"run_id"`
	State    rerateState `json:"state"`
	Status   string      `json:"status,omitempty"`
	Progress string      `json:"progress,omitempty"`
	Message  string      `json:"message,omitempty"`
}

type rerateTracker struct {
	mu     sync.RWMutex
	nextID uint64
	runs   map[rerateKey]rerateStatus
}

func newRerateTracker() *rerateTracker {
	return &rerateTracker{runs: make(map[rerateKey]rerateStatus)}
}

func (t *rerateTracker) start(userID int64, surface string) rerateStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nextID++
	status := rerateStatus{RunID: t.nextID, State: rerateStateRunning}
	t.runs[rerateKey{userID: userID, surface: surface}] = status
	return status
}

func (t *rerateTracker) record(userID int64, surface string, runID uint64, event, data string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	key := rerateKey{userID: userID, surface: surface}
	status, ok := t.runs[key]
	if !ok || status.RunID != runID {
		return
	}
	switch event {
	case "status":
		status.Status = data
	case "progress":
		status.Progress = data
	case "done":
		status.State = rerateStateDone
		status.Message = data
	case "failed":
		status.State = rerateStateFailed
		status.Message = data
	default:
		return
	}
	t.runs[key] = status
}

func (t *rerateTracker) snapshot(userID int64, surface string) (rerateStatus, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	status, ok := t.runs[rerateKey{userID: userID, surface: surface}]
	return status, ok
}

func (s *Server) handleRerateStatus(w http.ResponseWriter, r *http.Request) {
	surface := r.URL.Query().Get("surface")
	if !validRerateSurface(surface) {
		http.Error(w, "알 수 없는 화면이에요.", http.StatusBadRequest)
		return
	}
	userID, err := s.stateUserID(r.Context(), r)
	if err != nil {
		writeAuthUnauthorized(w)
		return
	}
	status, ok := s.rerates.snapshot(userID, surface)
	if !ok {
		status = rerateStatus{State: rerateStateIdle}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
```

- [ ] **Step 4: Initialize the tracker and register the status route**

Add the field and constructor initialization in `internal/server/server.go`:

```go
type Server struct {
	store        *storage.Store
	sources      []scraper.Scraper
	tmpl         *template.Template
	flight       *singleFlight
	rerates      *rerateTracker
	csrfSecret   []byte
	loginLimiter *loginRateLimiter
	// Existing AI and runtime fields remain unchanged.
}
```

```go
srv := &Server{
	store:             store,
	sources:           sources,
	flight:            newSingleFlight(),
	rerates:           newRerateTracker(),
	csrfSecret:        newCSRFSecret(),
	loginLimiter:      newLoginRateLimiter(),
	aiRunTokenCap:     defaultRunTokenCap,
	aiDailyTokenCap:   profile.DefaultDailyTokenCap,
	aiMonthlyTokenCap: aiMonthlyTokenCapForUSDCents(profile.DefaultAIMonthlyUSDCents),
	aiPerCallCap:      profile.DefaultAIPerCallCap,
}
```

Register the read-only route in `internal/server/handlers.go` directly after `/api/rerate`:

```go
mux.HandleFunc("GET /api/rerate", s.handleRerateSSE)
mux.HandleFunc("GET /api/rerate/status", s.handleRerateStatus)
```

Add the endpoint to the protected-route table in `internal/server/auth_test.go`:

```go
{name: "rerate status", method: http.MethodGet, target: "/api/rerate/status?surface=today"},
```

- [ ] **Step 5: Add endpoint validation and JSON tests**

Append to `internal/server/rerate_status_test.go`:

```go
func TestRerateStatusEndpoint(t *testing.T) {
	srv, _ := seedRerate(t)
	started := srv.rerates.start(0, "today")
	srv.rerates.record(0, "today", started.RunID, "progress", "공고 2/7 분석 중...")

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/rerate/status?surface=today", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	var got rerateStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.RunID != started.RunID || got.State != rerateStateRunning || got.Progress != "공고 2/7 분석 중..." {
		t.Fatalf("response = %+v", got)
	}
}

func TestRerateStatusEndpointRejectsUnknownSurface(t *testing.T) {
	srv, _ := seedRerate(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/rerate/status?surface=hidden", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
```

- [ ] **Step 6: Run the focused tests**

Run:

```bash
gofmt -w internal/server/rerate_status.go internal/server/rerate_status_test.go internal/server/server.go internal/server/handlers.go internal/server/auth_test.go
go test ./internal/server -run 'TestRerateTracker|TestRerateStatusEndpoint|TestProductionAuthRejectsAnonymousAPI' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit the tracker**

```bash
git add internal/server/rerate_status.go internal/server/rerate_status_test.go internal/server/server.go internal/server/handlers.go internal/server/auth_test.go
git commit -m "feat: track active AI evaluation status"
```

---

### Task 2: Publish Rerate Lifecycle and Detect Zero-Token No-Ops

**Files:**

- Modify: `internal/server/rerate.go:141-402`
- Modify: `internal/server/server.go:470-490`
- Modify: `internal/server/ai_rerate_test.go`
- Modify: `internal/server/ai_config_test.go`
- Modify: `internal/server/ai_injection_test.go`

**Interfaces:**

- Consumes: `Server.rerates`, `rerateTracker.start`, and `rerateTracker.record` from Task 1
- Produces: `rerateSummary{Analyzed int, Visible int, ProviderCalls int}`
- Produces: `runRerate(...) (rerateSummary, error)`
- Produces: `rateStage2(...) (analyzed int, providerCalls int, provErr error)`
- Produces: `rerateOne(...) (cached bool, providerCalled bool, err error)`
- Produces: `rerateDoneMessage(rerateSummary) string`

- [ ] **Step 1: Write the zero-token no-op test**

Replace the reconnect test's tuple-based assertions in `internal/server/ai_rerate_test.go` with a result assertion and add the exact no-op copy check:

```go
func TestRunRerateReconnectReusesCache(t *testing.T) {
	srv, stub := seedRerate(t)
	ctx := context.Background()

	first, err := srv.runRerate(ctx, "today", noopEmit)
	if err != nil {
		t.Fatalf("first runRerate: %v", err)
	}
	if first.ProviderCalls != 2 {
		t.Fatalf("first ProviderCalls = %d, want 2", first.ProviderCalls)
	}
	callsAfterFirst := stub.ScoreDeltaCalls

	second, err := srv.runRerate(ctx, "today", noopEmit)
	if err != nil {
		t.Fatalf("second runRerate: %v", err)
	}
	if second.Analyzed != 2 || second.ProviderCalls != 0 {
		t.Fatalf("second summary = %+v, want analyzed cache hits and zero calls", second)
	}
	if stub.ScoreDeltaCalls != callsAfterFirst {
		t.Fatalf("provider calls changed %d -> %d", callsAfterFirst, stub.ScoreDeltaCalls)
	}
	if got := rerateDoneMessage(second); got != "이미 모든 공고가 AI로 평가됐습니다. 추가 토큰은 사용하지 않았어요." {
		t.Fatalf("no-op copy = %q", got)
	}
}
```

- [ ] **Step 2: Run the no-op test and verify the result type is missing**

Run:

```bash
go test ./internal/server -run TestRunRerateReconnectReusesCache -count=1
```

Expected: FAIL because `runRerate` still returns integer tuples and `ProviderCalls` does not exist.

- [ ] **Step 3: Introduce the result type and provider-call accounting**

Add this type above `handleRerateSSE` in `internal/server/rerate.go`:

```go
type rerateSummary struct {
	Analyzed      int
	Visible       int
	ProviderCalls int
}
```

Change `rerateOne` to return whether a Stage-2 provider call was attempted. Cache, budget, and cap exits return `providerCalled=false`; every path after `calls.tryReserve()` returns `providerCalled=true`:

```go
func (s *Server) rerateOne(
	ctx context.Context, p scraper.Posting, aiInputHash, profileText string, now time.Time, budget *aiBudget, calls *callCap,
) (cached bool, providerCalled bool, err error) {
	if _, ok, e := s.store.AIScore(ctx, p.ID, aiInputHash, s.aiVersion); e == nil && ok {
		return true, false, nil
	}
	if budget == nil || !budget.canSpend() {
		return false, false, nil
	}
	if !calls.tryReserve() {
		return false, false, nil
	}
	s.extractStage1(ctx, p.ID, p, now, budget)
	sent, _, _ := ai.ModelInput(p)
	raw, usage, err := s.ai.ScoreDelta(ctx, sent, profileText)
	if err != nil {
		return false, true, err
	}
	budget.debit(ctx, usage)
	delta := ai.GateDelta(raw, sent, p.Description)
	if err := s.store.UpsertAIScore(ctx, p.ID, aiInputHash, s.aiVersion, delta, now); err != nil {
		return false, true, err
	}
	return true, true, nil
}
```

Change the `rateStage2` return list to `(analyzed int, providerCalls int, provErr error)`, then update its worker result and aggregation:

```go
type rerateResult struct {
	cached        bool
	providerCalled bool
	err           error
}
```

```go
cached, providerCalled, err := s.rerateOne(ctx, p, aiInputHash, profileText, now, budget, calls)
results <- rerateResult{cached: cached, providerCalled: providerCalled, err: err}
```

```go
for r := range results {
	completed++
	if r.cached {
		analyzed++
	}
	if r.providerCalled {
		providerCalls++
	}
	if r.err != nil && provErr == nil {
		provErr = r.err
	}
	emit("progress", fmt.Sprintf("공고 %d/%d 분석 중...", completed, total))
}
return analyzed, providerCalls, provErr
```

- [ ] **Step 4: Return `rerateSummary` from `runRerate` and update its callers**

Change `runRerate` to build and return one summary:

```go
func (s *Server) runRerate(ctx context.Context, surface string, emit func(event, data string), userIDOpt ...int64) (summary rerateSummary, err error) {
	userID := optionalUserID(userIDOpt)
	prof, ok, err := s.loadProfile(ctx, userID)
	if err != nil || !ok {
		return summary, err
	}
	postings, err := s.visibleForRerate(ctx, surface, time.Now(), userID)
	if err != nil {
		return summary, err
	}
	summary.Visible = len(postings)
	if summary.Visible == 0 {
		return summary, nil
	}
	emit("status", "AI로 다시 분석하는 중이에요 — 여러 공고를 한 번에 살펴보고 있어요. ☕")
	budget := s.newAIBudget(ctx)
	var provErr error
	summary.Analyzed, summary.ProviderCalls, provErr = s.rateStage2(ctx, postings, prof, budget, emit)
	if budget != nil && budget.isDegraded() {
		emit("status", "오늘 AI 예산을 다 써서 일부는 다시 분석하지 못했어요 — 프로필 설정에서 한도를 바꿀 수 있어요.")
	}
	if provErr != nil && summary.Analyzed > 0 {
		emit("status", providerFailureMessage(provErr))
	}
	emit("status", "점수를 다시 매기는 중...")
	if _, err := s.scoreAll(ctx, userID); err != nil {
		return summary, err
	}
	if summary.Analyzed == 0 && provErr != nil {
		return summary, &providerCallError{err: provErr}
	}
	return summary, nil
}
```

At the scrape caller in `internal/server/server.go`, discard the new count explicitly:

```go
rated, _, provErr := s.rateStage2(ctx, vis, prof, rateBudget, emit)
```

Update every test call found by `rg -n 'runRerate\(' internal/server --glob '*.go'` to receive `summary, err` and use `summary.Analyzed` or `summary.Visible`.

- [ ] **Step 5: Publish lifecycle events through the tracker**

After `newSSEWriter` succeeds in `handleRerateSSE`, start a run and wrap event delivery so tracker writes happen even after the browser disconnects:

```go
run := s.rerates.start(userID, surface)
emit := func(event, data string) {
	s.rerates.record(userID, surface, run.RunID, event, data)
	sw.event(event, data)
}
emit("run", fmt.Sprintf("%d", run.RunID))
```

Use `emit` for both terminal paths and for `runRerate`:

```go
done := false
failMsg := "AI 평가에 실패했어요. 잠시 후 다시 시도해 주세요."
defer func() {
	if !done {
		emit("failed", failMsg)
	}
}()

ctx, cancel := context.WithTimeout(context.Background(), scrapeMaxDuration)
defer cancel()
summary, err := s.runRerate(ctx, surface, emit, userID)
if err != nil {
	var pce *providerCallError
	if errors.As(err, &pce) {
		failMsg = providerFailureMessage(pce.err)
	}
	return
}
done = true
emit("done", rerateDoneMessage(summary))
```

- [ ] **Step 6: Implement exact terminal copy**

Replace `rerateDoneMessage` with:

```go
func rerateDoneMessage(summary rerateSummary) string {
	switch {
	case summary.Visible == 0:
		return "지금 화면에 분석할 공고가 없어요."
	case summary.ProviderCalls == 0 && summary.Analyzed >= summary.Visible:
		return "이미 모든 공고가 AI로 평가됐습니다. 추가 토큰은 사용하지 않았어요."
	case summary.Analyzed >= summary.Visible:
		return fmt.Sprintf("공고 %d개를 모두 AI로 분석했어요.", summary.Visible)
	default:
		return fmt.Sprintf(
			"공고 %d/%d개를 AI로 분석했어요 — 토큰을 아끼려고 한 번에 일정 개수만 분석해요. 더 보려면 다시 눌러주세요.",
			summary.Analyzed, summary.Visible)
	}
}
```

- [ ] **Step 7: Add a terminal snapshot endpoint test**

Append to `internal/server/ai_rerate_test.go`:

```go
func TestRerateSSEPublishesTerminalStatus(t *testing.T) {
	srv, _ := seedRerate(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/rerate?surface=today", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("rerate status = %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "event: run") || !strings.Contains(body, "event: done") {
		t.Fatalf("SSE lifecycle events missing:\n%s", body)
	}

	statusRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(statusRec, httptest.NewRequest(http.MethodGet, "/api/rerate/status?surface=today", nil))
	var status rerateStatus
	if err := json.Unmarshal(statusRec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.State != rerateStateDone || status.RunID == 0 || status.Message == "" {
		t.Fatalf("terminal status = %+v", status)
	}
}
```

- [ ] **Step 8: Run server tests and the race detector**

Run:

```bash
gofmt -w internal/server/rerate.go internal/server/server.go internal/server/ai_rerate_test.go internal/server/ai_config_test.go internal/server/ai_injection_test.go
go test ./internal/server -run 'TestRunRerate|TestRerate' -count=1
go test -race ./internal/server -run 'TestRunRerate|TestRerateTracker|TestRerateStatus' -count=1
```

Expected: PASS with no data races.

- [ ] **Step 9: Commit lifecycle tracking and no-op semantics**

```bash
git add internal/server/rerate.go internal/server/server.go internal/server/ai_rerate_test.go internal/server/ai_config_test.go internal/server/ai_injection_test.go
git commit -m "feat: recover AI evaluation lifecycle state"
```

---

### Task 3: History Restoration, Completion Notice, and Activity Animation

**Files:**

- Modify: `web/_rerate.html:1-17`
- Replace: `web/ai-rerate.js`
- Modify: `web/styles.css` near the existing rerate and scrape-log rules
- Modify: `internal/server/ai_rerate_test.go`

**Interfaces:**

- Consumes: `GET /api/rerate/status` and its `rerateStatus` JSON from Task 1
- Consumes: the terminal `done` and `failed` snapshots published in Task 2
- Produces: session keys `jobcron:rerate-notice:<surface>` and `jobcron:rerate-handled:<surface>`
- Produces: DOM IDs `rerate-activity`, `rerate-status`, and `rerate-progress`
- Produces: a `pageshow` recovery path that runs only for back/forward restoration

- [ ] **Step 1: Pin the button copy and activity markup in a failing render test**

Extend `TestRerateButtonHiddenWithoutKey` in `internal/server/ai_rerate_test.go` after the AI-on response:

```go
body := rec2.Body.String()
for _, want := range []string{
	`id="rerate"`,
	`>AI 평가<`,
	`id="rerate-activity"`,
	`aria-hidden="true"`,
} {
	if !strings.Contains(body, want) {
		t.Fatalf("rendered rerate control missing %q", want)
	}
}
if strings.Contains(body, `>재평가<`) {
		t.Fatal("legacy 재평가 button copy is still rendered")
}
```

- [ ] **Step 2: Run the render test and verify the new copy is absent**

Run:

```bash
go test ./internal/server -run TestRerateButtonHiddenWithoutKey -count=1
```

Expected: FAIL because the button still says `재평가` and has no activity indicator.

- [ ] **Step 3: Update the shared rerate template**

Replace the control block in `web/_rerate.html` with:

```html
<div class="rerate-row">
  <button id="rerate" type="button" class="btn-ghost{{if .StaleCount}} has-stale{{end}}" data-surface="{{.Surface}}">AI 평가{{if .StaleCount}} ·{{.StaleCount}}{{end}}</button>
  <span class="rerate-hint">AI로 화면에 보이는 공고를 분석해요{{if .StaleCount}} · 프로필이 바뀐 공고 {{.StaleCount}}개{{end}}</span>
  {{if .Visible}}<span class="rerate-count{{if ge .Analyzed .Visible}} complete{{end}}" title="화면에 보이는 공고 {{.Visible}}개 중 {{.Analyzed}}개를 AI로 분석했어요. 토큰을 아끼려고 한 번에 일정 개수만 분석하고, 더 보려면 AI 평가를 다시 누르면 돼요.">AI 분석 {{.Analyzed}}/{{.Visible}}</span>{{end}}
</div>
<div class="rerate-feedback">
  <span id="rerate-activity" class="rerate-activity" aria-hidden="true" hidden><i></i><i></i><i></i></span>
  <div id="rerate-log" class="scrape-log" aria-live="polite"></div>
</div>
```

Also update the template comment from `재평가` to `AI 평가`.

- [ ] **Step 4: Replace `web/ai-rerate.js` with separated lifecycle helpers**

Implement these exact responsibilities in one IIFE:

```javascript
(function () {
  var btn = document.getElementById('rerate');
  var log = document.getElementById('rerate-log');
  var activity = document.getElementById('rerate-activity');
  if (!btn || !log || !activity) return;

  var surface = btn.dataset.surface;
  if (!surface) return;
  var eventSource = null;
  var pollTimer = null;
  var activeRunID = 0;
  var noticeKey = 'jobcron:rerate-notice:' + surface;
  var handledKey = 'jobcron:rerate-handled:' + surface;
  var activeCopy = 'AI로 다시 분석하는 중이에요 — 여러 공고를 한 번에 살펴보고 있어요. ☕';
  var completedAwayCopy = 'AI 평가가 완료됐어요. 새로운 평가 결과를 반영했습니다.';

  function messageElement(id) {
    var node = document.getElementById(id);
    if (!node) {
      node = document.createElement('p');
      node.id = id;
      log.appendChild(node);
    }
    return node;
  }

  function setMessage(node, msg) {
    node.textContent = '';
    var settingsText = '프로필 설정';
    var index = msg.indexOf(settingsText);
    if (index === -1) {
      node.textContent = msg;
      return;
    }
    node.appendChild(document.createTextNode(msg.slice(0, index)));
    var link = document.createElement('a');
    link.href = '/profile';
    link.className = 'budget-settings-link';
    link.textContent = settingsText;
    node.appendChild(link);
    node.appendChild(document.createTextNode(msg.slice(index + settingsText.length)));
  }

  function showStatus(msg) {
    if (msg) setMessage(messageElement('rerate-status'), msg);
  }

  function showProgress(msg) {
    if (msg) messageElement('rerate-progress').textContent = msg;
  }

  function setRunning(running) {
    btn.disabled = running;
    activity.hidden = !running;
  }

  function stopTransport() {
    if (eventSource) {
      eventSource.close();
      eventSource = null;
    }
    if (pollTimer) {
      clearTimeout(pollTimer);
      pollTimer = null;
    }
  }

  function rememberAndReload(message, runID) {
    if (runID) sessionStorage.setItem(handledKey, String(runID));
    sessionStorage.setItem(noticeKey, message);
    location.reload();
  }

  function showStoredNotice() {
    var message = sessionStorage.getItem(noticeKey);
    if (!message) return;
    sessionStorage.removeItem(noticeKey);
    showStatus(message);
  }

  function pollStatus() {
    fetch('/api/rerate/status?surface=' + encodeURIComponent(surface), {
      headers: { 'Accept': 'application/json' },
      cache: 'no-store'
    }).then(function (response) {
      if (!response.ok) throw new Error('status ' + response.status);
      return response.json();
    }).then(function (status) {
      var handled = status.run_id && sessionStorage.getItem(handledKey) === String(status.run_id);
      if (status.state === 'running') {
        setRunning(true);
        showStatus(status.status || activeCopy);
        showProgress(status.progress || '공고 분석을 준비하는 중...');
        pollTimer = setTimeout(pollStatus, 750);
        return;
      }
      setRunning(false);
      if (status.state === 'done' && !handled) {
        rememberAndReload(completedAwayCopy, status.run_id);
        return;
      }
      if (status.state === 'failed' && !handled) {
        sessionStorage.setItem(handledKey, String(status.run_id));
        showStatus(status.message || 'AI 평가에 실패했어요.');
      }
    }).catch(function () {
      setRunning(false);
      showStatus('진행 상태를 다시 불러오지 못했어요. 잠시 후 다시 시도해 주세요.');
    });
  }

  function isHistoryReturn(event) {
    if (event && event.persisted) return true;
    var entries = performance.getEntriesByType ? performance.getEntriesByType('navigation') : [];
    return entries.length > 0 && entries[0].type === 'back_forward';
  }

  btn.addEventListener('click', function () {
    stopTransport();
    log.textContent = '';
    setRunning(true);
    showStatus(activeCopy);
    eventSource = new EventSource('/api/rerate?surface=' + encodeURIComponent(surface));
    eventSource.addEventListener('run', function (event) { activeRunID = Number(event.data) || 0; });
    eventSource.addEventListener('status', function (event) { showStatus(event.data); });
    eventSource.addEventListener('progress', function (event) { showProgress(event.data); });
    eventSource.addEventListener('done', function (event) {
      stopTransport();
      setRunning(false);
      rememberAndReload(event.data, activeRunID);
    });
    eventSource.addEventListener('failed', function (event) {
      stopTransport();
      setRunning(false);
      showStatus(event.data || 'AI 평가에 실패했어요.');
    });
    eventSource.addEventListener('error', function () {
      if (document.visibilityState === 'hidden') return;
      stopTransport();
      setRunning(false);
      showStatus('연결이 끊겼어요. 잠시 후 다시 시도해 주세요.');
    });
  });

  window.addEventListener('pagehide', stopTransport);
  window.addEventListener('pageshow', function (event) {
    showStoredNotice();
    if (isHistoryReturn(event)) pollStatus();
  });
  showStoredNotice();
})();
```

- [ ] **Step 5: Add the three-dot animation with reduced-motion behavior**

Add near the rerate styles in `web/styles.css`:

```css
.rerate-feedback {
  display: flex;
  align-items: flex-start;
  gap: 0.45rem;
}
.rerate-activity {
  flex: 0 0 1.3rem;
  display: inline-flex;
  align-items: center;
  justify-content: space-between;
  min-height: 1.5rem;
  padding-top: 0.15rem;
}
.rerate-activity[hidden] { display: none; }
.rerate-activity i {
  width: 0.22rem;
  height: 0.22rem;
  border-radius: 50%;
  background: var(--accent);
  animation: rerate-pulse 0.9s ease-in-out infinite alternate;
}
.rerate-activity i:nth-child(2) { animation-delay: 0.15s; }
.rerate-activity i:nth-child(3) { animation-delay: 0.3s; }
@keyframes rerate-pulse {
  from { opacity: 0.3; transform: translateY(0); }
  to { opacity: 1; transform: translateY(-0.16rem); }
}
@media (prefers-reduced-motion: reduce) {
  .rerate-activity i {
    animation: none;
    opacity: 0.75;
    transform: none;
  }
}
```

- [ ] **Step 6: Run focused template and server tests**

Run:

```bash
go test ./internal/server -run 'TestRerateButton|TestRerateSSE|TestRerateStatus' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit the restored UI lifecycle**

```bash
git add web/_rerate.html web/ai-rerate.js web/styles.css internal/server/ai_rerate_test.go
git commit -m "feat: restore AI evaluation progress after navigation"
```

---

### Task 4: Electric-Indigo AI Analysis Surface

**Files:**

- Modify: `web/styles.css:41-49,522-596`
- Create: `web/styles_test.go`

**Interfaces:**

- Produces: `--ai-chip`, `--ai-panel`, `--ai-border`, `--ai-text`, and `--ai-accent`
- Consumes: existing `.chip-ai`, `.ai-evidence`, `.chip-caret`, `.v`, `.v.neg`, and `:focus-visible` markup
- Preserves: `.posting`, ordinary `.chip`, deadline colors, and the 44px mobile rule

- [ ] **Step 1: Write a failing token-presence test**

Create `web/styles_test.go`:

```go
package web

import (
	"strings"
	"testing"
)

func TestAIAnalysisUsesApprovedIndigoTokens(t *testing.T) {
	b, err := FS.ReadFile("styles.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)
	for _, want := range []string{
		"--ai-chip:     light-dark(#eeeafe, #29233f)",
		"--ai-panel:    light-dark(#f3f0ff, #211c34)",
		"--ai-border:   light-dark(#c9bdfa, #5b4d8d)",
		"--ai-text:     light-dark(#3f307c, #ede8ff)",
		"--ai-accent:   light-dark(#6748c7, #b7a7ff)",
		"background: var(--ai-chip)",
		"background: var(--ai-panel)",
		"outline: 2px solid var(--ai-accent)",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("styles.css missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Run the style test and verify the tokens are absent**

Run:

```bash
go test ./web -run TestAIAnalysisUsesApprovedIndigoTokens -count=1
```

Expected: FAIL for the first missing token.

- [ ] **Step 3: Add semantic Electric-indigo variables**

Add these variables to the existing `:root` token block in `web/styles.css`:

```css
--ai-chip:     light-dark(#eeeafe, #29233f);
--ai-panel:    light-dark(#f3f0ff, #211c34);
--ai-border:   light-dark(#c9bdfa, #5b4d8d);
--ai-text:     light-dark(#3f307c, #ede8ff);
--ai-accent:   light-dark(#6748c7, #b7a7ff);
```

- [ ] **Step 4: Scope the new tokens to AI elements only**

Update the AI-specific rules without changing `.posting` or the base `.chip` rule:

```css
.chip-ai {
  font: inherit;
  font-size: 0.73rem;
  letter-spacing: 0.01em;
  background: var(--ai-chip);
  color: var(--ai-text);
  border: 1px solid var(--ai-border);
  border-radius: 3px;
  padding: 0.12rem 0.55rem;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
}
.chip-ai .v,
.chip-ai .v.neg,
.ai-evidence .v,
.ai-evidence .v.neg { color: var(--ai-accent); }
.chip-ai .chip-caret { color: var(--ai-accent); }
.chip-ai:focus-visible { outline: 2px solid var(--ai-accent); outline-offset: 2px; }
.ai-evidence {
  flex-basis: 100%;
  width: 100%;
  margin-top: 0.4rem;
  padding: 0.5rem 0.7rem;
  color: var(--ai-text);
  background: var(--ai-panel);
  border: 1px solid var(--ai-border);
  border-radius: 4px;
}
```

Retain the existing `.chip-ai.is-stale`, `.chip-stale-note`, `.ai-evidence-item`, and mobile 44px rules. Update the nearby comments so they describe indigo rather than the old gold/neutral treatment.

- [ ] **Step 5: Run the web and server rendering tests**

Run:

```bash
gofmt -w web/styles_test.go
go test ./web ./internal/server -run 'TestAIAnalysisUsesApprovedIndigoTokens|TestRerateInfoCountsCacheNotChips|TestRerateButton' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit the approved color system**

```bash
git add web/styles.css web/styles_test.go
git commit -m "feat: style AI analysis with electric indigo"
```

---

### Task 5: Demoday Default-Off and Existing Alpha Profile Update

**Files:**

- Modify: `internal/server/sources.go:31-65`
- Modify: `internal/server/handlers.go:438-461`
- Modify: `internal/server/server_test.go:21-68`
- Runtime data change: current alpha user's saved profile through the `/profile` UI

**Interfaces:**

- Produces: `defaultDisabledSources() []string` returning `[]string{"demoday"}`
- Produces: `sourceOptions(disabled []string, applyDefaults bool) []sourceOption`
- Consumes: the existing `ok` result from `profileJSON`
- Preserves: explicit saved source choices, including a later user decision to re-enable Demoday

- [ ] **Step 1: Make the scraper test double source-configurable**

Add `source string` to `fakeScraper` in `internal/server/server_test.go` and update `Source` without changing existing fixtures:

```go
type fakeScraper struct {
	source       string
	listing      []scraper.Posting
	details      map[string]scraper.Posting
	accessErr    error
	listingPanic string
	detailCalls  []string
}

func (f *fakeScraper) Source() string {
	if f.source != "" {
		return f.source
	}
	return "jumpit"
}
```

- [ ] **Step 2: Write new-profile and saved-profile source tests**

Add to `internal/server/server_test.go`:

```go
func TestProfileFormDefaultsDemodayOffOnlyBeforeFirstSave(t *testing.T) {
	st, err := storage.OpenAt(filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	srv := New(st, &fakeScraper{}, &fakeScraper{source: "demoday"})

	render := func() string {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/profile", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /profile = %d", rec.Code)
		}
		return rec.Body.String()
	}

	newBody := render()
	if !strings.Contains(newBody, `name="source_jumpit" checked`) {
		t.Fatal("Jumpit should default on")
	}
	if strings.Contains(newBody, `name="source_demoday" checked`) {
		t.Fatal("Demoday should default off before the first profile save")
	}

	saveTestProfile(t, st, profile.Profile{})
	savedBody := render()
	if !strings.Contains(savedBody, `name="source_demoday" checked`) {
		t.Fatal("an explicitly saved all-enabled profile must keep Demoday enabled")
	}
}
```

- [ ] **Step 3: Run the profile-form test and verify Demoday currently defaults on**

Run:

```bash
go test ./internal/server -run TestProfileFormDefaultsDemodayOffOnlyBeforeFirstSave -count=1
```

Expected: FAIL because `sourceOptions` currently treats every unlisted source as enabled.

- [ ] **Step 4: Implement the new-profile default without changing saved semantics**

Add to `internal/server/sources.go`:

```go
const demodaySourceID = "demoday"

func defaultDisabledSources() []string {
	return []string{demodaySourceID}
}
```

Change `sourceOptions` to apply defaults only when instructed:

```go
func (s *Server) sourceOptions(disabled []string, applyDefaults bool) []sourceOption {
	disabledSet := make(map[string]bool, len(disabled)+1)
	for _, id := range disabled {
		disabledSet[id] = true
	}
	if applyDefaults {
		for _, id := range defaultDisabledSources() {
			disabledSet[id] = true
		}
	}
	opts := make([]sourceOption, 0, len(s.sources))
	for _, src := range s.sources {
		id := src.Source()
		opts = append(opts, sourceOption{
			ID: id, Label: sourceLabel(id), Enabled: !disabledSet[id], Kind: src.Kind(),
		})
	}
	return opts
}
```

In `handleProfileForm`, use the existing `ok` flag:

```go
form.Sources = s.sourceOptions(p.DisabledSources, !ok)
```

- [ ] **Step 5: Run focused source and profile tests**

Run:

```bash
gofmt -w internal/server/sources.go internal/server/handlers.go internal/server/server_test.go
go test ./internal/server -run 'TestProfileFormDefaultsDemodayOffOnlyBeforeFirstSave|TestBriefingStatus' -count=1
```

Expected: PASS.

- [ ] **Step 6: Update the current alpha profile through the real UI**

Back up the current local database before changing the saved profile:

```bash
test -n "$JOBSCRAPER_DB"
cp "$JOBSCRAPER_DB" "$JOBSCRAPER_DB.before-demoday-$(date +%Y%m%d%H%M%S)"
```

Start or reuse the app with `--no-open`, then use `/browse` against its real local URL:

```bash
B="$HOME/.Codex/skills/gstack/browse/dist/browse"
APP_URL=http://127.0.0.1:17778
"$B" goto "$APP_URL/profile"
"$B" is checked 'input[name="source_demoday"]'
"$B" click 'input[name="source_demoday"]'
"$B" click 'button[type="submit"]'
"$B" goto "$APP_URL/profile"
"$B" is checked 'input[name="source_demoday"]'
```

Expected: the first check is `true`; the final check is `false`. Confirm Jumpit, Rallit, and other intended sources remain checked before submitting.

- [ ] **Step 7: Commit the default behavior**

```bash
git add internal/server/sources.go internal/server/handlers.go internal/server/server_test.go
git commit -m "feat: default Demoday source off"
```

Do not commit the database or its backup.

---

### Task 6: Integrated Tests, Tier C Browser QA, and Documentation Archive

**Files:**

- Create: `docs/superpowers/archive/2026-07-12-alpha-milestone-a-polishes/260712-alpha-milestone-a-polishes-verification.md`
- Move after verification: `docs/superpowers/specs/260712-alpha-milestone-a-polishes.md`
- Move after verification: `docs/superpowers/plans/260712-alpha-milestone-a-polishes-implementation-plan.md`
- Modify: `docs/superpowers/README.md`
- Modify: `docs/README.md`

**Interfaces:**

- Consumes: all implementation tasks
- Produces: fresh unit, race, full-suite, vet, browser, accessibility-visible, and responsive evidence
- Produces: archived spec, plan, and verification report with no active stale links

- [ ] **Step 1: Run formatting and static checks**

Run:

```bash
test -z "$(gofmt -l internal/server web)"
go vet ./...
git diff --check
```

Expected: all commands exit 0 with no output from `gofmt -l` or `git diff --check`.

- [ ] **Step 2: Run focused and race tests**

Run:

```bash
go test ./internal/server ./web -count=1
go test -race ./internal/server -run 'TestRunRerate|TestRerateTracker|TestRerateStatus|TestProfileFormDefaultsDemodayOff' -count=1
```

Expected: PASS with no race report.

- [ ] **Step 3: Run the complete project suite**

Run:

```bash
go test ./... -count=1
```

Expected: PASS for every package.

- [ ] **Step 4: Start or reuse the real local app without opening a browser**

Use the project binary or interactive preview with an explicit port and `--no-open`:

```bash
go run ./cmd/jobcron --host 127.0.0.1 --port 17778 --no-open
```

Expected: the server remains available at `http://127.0.0.1:17778`. If an existing server already reflects the current build, reuse it instead of starting a second copy.

- [ ] **Step 5: Verify the primary AI evaluation flow in a real browser**

Using `/browse`, perform this sequence on `/briefing`:

1. Record today's token usage from `/profile`.
2. Click `AI 평가` and assert the button becomes disabled.
3. Assert `#rerate-activity` is visible and `#rerate-status` contains the exact active copy.
4. Wait for `#rerate-progress` to contain `공고 N/M 분석 중...`.
5. Navigate to `/`, then execute browser Back.
6. Assert the original page again shows a disabled button, visible animation, and a current progress value.
7. Wait long enough to observe the progress value change, proving it is not a stale restored DOM value.
8. Navigate away until the run completes, then go Back.
9. Assert one automatic reload occurs, fresh AI scores render, and the exact one-time completion message appears.
10. Reload manually and assert the one-time completion message is gone.

Do not substitute HTTP requests for these browser assertions.

- [ ] **Step 6: Verify the fully cached zero-token path**

With all visible postings evaluated:

1. Record today's token usage from `/profile`.
2. Return to the same listing surface and click `AI 평가`.
3. Assert the page shows `이미 모든 공고가 AI로 평가됐습니다. 추가 토큰은 사용하지 않았어요.` after its reload.
4. Return to `/profile` and assert the displayed token usage is unchanged.

This user-path check proves the no-op claim rather than inferring it from a cached row count.

- [ ] **Step 7: Verify the Electric-indigo chip and interaction states**

At desktop `1440x900` and mobile `390x844`, in both light and dark themes:

1. Capture a page screenshot containing an AI chip.
2. Click the AI chip and capture its expanded evidence panel.
3. Keyboard-focus the chip and capture its Electric-indigo focus ring.
4. Assert the listing card itself retains its normal background.
5. Assert the mobile chip remains at least 44px high.
6. Assert no horizontal overflow and no console errors.

The minimum computed text/background contrast must remain at least 4.5:1; the approved mockup measured 8.09:1 or better.

- [ ] **Step 8: Walk every UI page under Tier C**

Visit `/`, `/briefing`, `/bookmarks`, `/hidden`, and `/profile` at desktop and mobile widths in light and dark themes. On each page:

- capture a screenshot,
- inspect the changed chip/button or its closest sibling pattern,
- confirm no text overlap or horizontal overflow,
- check browser console and failed network requests,
- click one representative interactive control and verify its real result.

Also enable reduced motion in the browser context and verify the three dots remain visible but static while evaluation is running.

- [ ] **Step 9: Write the verification report**

Create `docs/superpowers/archive/2026-07-12-alpha-milestone-a-polishes/260712-alpha-milestone-a-polishes-verification.md` with:

```markdown
# Alpha Milestone A Polishes Verification

## Automated Gates

- `go vet ./...`: PASS
- `go test ./internal/server ./web -count=1`: PASS
- `go test -race ./internal/server -run 'TestRunRerate|TestRerateTracker|TestRerateStatus|TestProfileFormDefaultsDemodayOff' -count=1`: PASS
- `go test ./... -count=1`: PASS
- `git diff --check`: PASS

## Browser QA

- Back/forward recovery: PASS
- Completion while away, one reload, one-time notice: PASS
- Fully cached evaluation with unchanged token usage: PASS
- Electric-indigo chip/panel/focus, light and dark: PASS
- Desktop 1440x900 and mobile 390x844: PASS
- Reduced motion: PASS
- All UI routes, console, network, and overflow: PASS
- Current alpha profile has Demoday disabled and can re-enable it: PASS

## Remaining Risk

Rerate recovery is intentionally process-local. Restarting Jobcron during an active evaluation clears the status snapshot; cached per-row AI results remain safe and reusable.
```

Replace any failed line with the exact failure and stop before archiving or committing.

- [ ] **Step 10: Archive completed work and update the index**

After every gate passes, move the records:

```bash
mkdir -p docs/superpowers/archive/2026-07-12-alpha-milestone-a-polishes
git mv docs/superpowers/specs/260712-alpha-milestone-a-polishes.md docs/superpowers/archive/2026-07-12-alpha-milestone-a-polishes/
git mv docs/superpowers/plans/260712-alpha-milestone-a-polishes-implementation-plan.md docs/superpowers/archive/2026-07-12-alpha-milestone-a-polishes/
```

Change `docs/superpowers/README.md` so `Active Work` says `None.` and `Recently Archived` links to the archived spec, implementation plan, and verification report. Replace the active-plan link in `docs/README.md` with a link to the archived verification report.

- [ ] **Step 11: Review the cumulative diff and commit locally**

Run:

```bash
git diff HEAD~5 --stat
git diff HEAD~5 -- internal/server web docs/superpowers
git status --short
```

Confirm the diff contains no API keys, database files, backups, generated screenshots, or machine-specific paths. Then commit:

```bash
git add docs/README.md docs/superpowers/README.md docs/superpowers/archive/2026-07-12-alpha-milestone-a-polishes
git commit -m "docs: archive alpha milestone A verification"
```

Do not push.

---

## Implementation Completion Criteria

- The original page's AI evaluation UI recovers after back/forward navigation and continues showing current progress.
- The page visited in between never displays the other page's evaluation UI.
- Completion while away causes one reload, renders fresh scores, and shows one one-time completion message.
- A second manual reload does not repeat the completion notice.
- Fully cached evaluation makes no provider call, does not change displayed token usage, and explains that outcome.
- The control says `AI 평가` on every shared surface.
- The activity indicator is visible during work and static under reduced motion.
- Electric indigo is limited to AI analysis elements and passes contrast, focus, theme, and mobile checks.
- New profiles start with Demoday off; the current profile is updated once; re-enabling remains possible.
- All automated, race, full-suite, and Tier C browser gates pass.
- All commits remain local and the completed records are archived and indexed.
