package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
)

// rerateStub returns a provider whose ScoreDelta cites a quote that is present
// in the seeded postings, so the gate keeps it. It counts ScoreDelta calls.
func rerateStub() *ai.StubProvider {
	return &ai.StubProvider{
		NameVal: "stub",
		ScoreDeltaFn: func(ctx context.Context, modelText, profileText string) ([]ai.RawDeltaItem, ai.Usage, error) {
			return []ai.RawDeltaItem{
				{Signal: "백엔드", Kind: ai.KindPresence, Delta: 7, Quote: "서버 개발자를 찾습니다", MatchedGoal: "좋아하는 업무"},
			}, ai.Usage{InputTokens: 50, OutputTokens: 10}, nil
		},
	}
}

// seedRerate saves a show-everything profile and two postings first seen today,
// then scores them so they are visible on /today. Returns the server + store.
func seedRerate(t *testing.T) (*Server, *ai.StubProvider) {
	t.Helper()
	srv, st := newTestServer(t, &fakeScraper{})
	ctx := context.Background()
	zero := 0
	prof := profile.Profile{CareerYears: 0, MinScore: &zero, JobLikes: "백엔드 서버 개발"}
	pj, _ := profile.Marshal(prof)
	if _, _, err := st.SaveProfile(ctx, pj); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	now := time.Now().UTC()
	for _, id := range []string{"r1", "r2"} {
		p := listingPosting(id, "신입 백엔드 개발자")
		p.Description = "서버 개발자를 찾습니다"
		p.FirstSeenAt, p.LastSeenAt = now, now
		if _, _, err := st.UpsertPosting(ctx, p); err != nil {
			t.Fatalf("UpsertPosting: %v", err)
		}
	}
	stub := rerateStub()
	srv.SetAIProvider(stub, "test-model")
	if _, err := srv.scoreAll(ctx); err != nil { // initial scores → rows visible on today
		t.Fatalf("scoreAll: %v", err)
	}
	return srv, stub
}

func TestRunRerateRatesVisibleRows(t *testing.T) {
	srv, stub := seedRerate(t)
	ctx := context.Background()

	summary, err := srv.runRerate(ctx, "today", noopEmit)
	if err != nil {
		t.Fatalf("runRerate: %v", err)
	}
	if summary.Analyzed != 2 {
		t.Errorf("rated %d rows, want 2", summary.Analyzed)
	}
	if stub.ScoreDeltaCalls != 2 {
		t.Errorf("ScoreDelta calls = %d, want 2 (one per visible uncached row)", stub.ScoreDeltaCalls)
	}
	// Both postings now carry a fresh ai_scores row.
	deltas, err := srv.store.AIScoresByPostingID(ctx, profile.AIInputHash(currentProfile(t, srv)), srv.aiVersion)
	if err != nil {
		t.Fatalf("AIScoresByPostingID: %v", err)
	}
	if len(deltas) != 2 {
		t.Fatalf("ai_scores rows = %d, want 2", len(deltas))
	}
	// The cited delta survived the gate and landed in the stored score breakdown.
	scores, _ := srv.store.ScoresByPostingID(ctx)
	for id, sc := range scores {
		if sc.Total < 7 {
			t.Errorf("posting %d Total = %d, want >= 7 (AI +7 merged)", id, sc.Total)
		}
	}
}

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

func TestRunRerateBudgetMessagePointsToProfileSettings(t *testing.T) {
	srv, _ := seedRerate(t)
	srv.aiRunTokenCap = 0
	ctx := context.Background()
	var messages []string
	emit := func(event, data string) {
		if event == "status" {
			messages = append(messages, data)
		}
	}

	if _, err := srv.runRerate(ctx, "today", emit); err != nil {
		t.Fatalf("runRerate: %v", err)
	}
	body := strings.Join(messages, "\n")
	if !strings.Contains(body, "오늘 AI 예산을 다 써서") || !strings.Contains(body, "프로필 설정") {
		t.Fatalf("budget cap message should explain the cap and point to settings, got:\n%s", body)
	}
}

// TestRerateProgressesAcrossPressesUnderBudget proves the behavior a user with a
// large list depends on: when the per-run token budget halts a re-rate partway,
// each following press skips the already-rated rows (Stage-2 cache hits, no
// re-spend) and rates the ones after them, until the whole list is done. Each
// posting's ScoreDelta is called exactly once across all presses — never
// re-rated — even though the concurrent worker pool can overshoot the per-run
// cap by a bounded number of calls.
func TestRerateProgressesAcrossPressesUnderBudget(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	ctx := context.Background()
	zero := 0
	prof := profile.Profile{CareerYears: 0, MinScore: &zero, JobLikes: "백엔드 서버 개발"}
	pj, _ := profile.Marshal(prof)
	if _, _, err := st.SaveProfile(ctx, pj); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	now := time.Now().UTC()
	const total = 24
	for i := 0; i < total; i++ {
		p := listingPosting("p"+string(rune('a'+i)), "신입 백엔드 개발자")
		p.Description = "서버 개발자를 찾습니다"
		p.FirstSeenAt, p.LastSeenAt = now, now
		if _, _, err := st.UpsertPosting(ctx, p); err != nil {
			t.Fatalf("UpsertPosting: %v", err)
		}
	}
	stub := rerateStub() // each ScoreDelta debits 60 tokens (50 in + 10 out)
	srv.SetAIProvider(stub, "test-model")
	// Per-run cap ≈ 8 calls (8×60=480), so the run halts well short of `total`.
	// The concurrent worker pool can overshoot the cap by at most rerateWorkers
	// calls (workers that pass canSpend before in-flight debits land), so one
	// press rates somewhere in [8, 8+rerateWorkers] rows — always strictly fewer
	// than `total`. The daily cap is high so only the run cap binds.
	srv.aiRunTokenCap = 480
	srv.aiDailyTokenCap = 1_000_000
	if _, err := srv.scoreAll(ctx); err != nil {
		t.Fatalf("scoreAll: %v", err)
	}

	// Press 1 halts partway: the run budget stops it before the whole list.
	if _, err := srv.runRerate(ctx, "today", noopEmit); err != nil {
		t.Fatalf("press 1: %v", err)
	}
	rated1 := countAIScores(t, srv)
	if rated1 == 0 || rated1 >= total {
		t.Fatalf("press 1 rated %d rows, want a partial run (0 < n < %d): the budget must halt it partway", rated1, total)
	}
	if stub.ScoreDeltaCalls != rated1 {
		t.Fatalf("press 1: %d ScoreDelta calls but %d rows rated — every successful call must cache exactly one row",
			stub.ScoreDeltaCalls, rated1)
	}

	// Keep pressing: each press resumes on the still-uncached rows (cache hits
	// skip the already-rated ones, no re-spend) until the whole list is rated.
	for press := 2; countAIScores(t, srv) < total; press++ {
		if press > 20 {
			t.Fatalf("re-rate did not finish the list within 20 presses (rated %d/%d)", countAIScores(t, srv), total)
		}
		if _, err := srv.runRerate(ctx, "today", noopEmit); err != nil {
			t.Fatalf("press %d: %v", press, err)
		}
	}

	// Every row rated exactly once across all presses — never re-rated.
	if stub.ScoreDeltaCalls != total {
		t.Fatalf("total ScoreDelta calls = %d, want %d (each row rated exactly once, never re-rated)",
			stub.ScoreDeltaCalls, total)
	}

	// A press over a fully-rated list spends nothing.
	if _, err := srv.runRerate(ctx, "today", noopEmit); err != nil {
		t.Fatalf("final press: %v", err)
	}
	if stub.ScoreDeltaCalls != total {
		t.Fatalf("final press made extra ScoreDelta calls (%d > %d) — a fully-rated list must re-spend nothing",
			stub.ScoreDeltaCalls, total)
	}
}

// countAIScores counts rows in ai_scores under the server's ai_version for the
// current profile's goal text (the fresh, current-goal deltas).
func countAIScores(t *testing.T, srv *Server) int {
	t.Helper()
	m, err := srv.store.AIScoresByPostingID(context.Background(),
		profile.AIInputHash(currentProfile(t, srv)), srv.aiVersion)
	if err != nil {
		t.Fatalf("AIScoresByPostingID: %v", err)
	}
	return len(m)
}

func TestRerateMutuallyExclusiveWithScrape(t *testing.T) {
	srv, _ := seedRerate(t)

	t.Run("re-rate is rejected while a scrape holds the lock", func(t *testing.T) {
		if !srv.flight.tryAcquire(scrapeAllKey) {
			t.Fatal("could not acquire the scrape lock for the test")
		}
		defer srv.flight.release(scrapeAllKey)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/rerate?surface=today", nil))
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409 (scrape in progress)", rec.Code)
		}
	})

	t.Run("a scrape is rejected while a re-rate holds the lock", func(t *testing.T) {
		if !srv.flight.tryAcquire(scrapeAllKey) {
			t.Fatal("could not acquire the shared lock for the test")
		}
		defer srv.flight.release(scrapeAllKey)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/scrape", nil))
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409 (shared scrape⟷re-rate lock)", rec.Code)
		}
	})
}

func TestRerateRejectedWhenAIOff(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{}) // no SetAIProvider
	saveSinipProfile(t, srv)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/rerate?surface=today", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (AI not configured)", rec.Code)
	}
}

func TestRerateRejectsUnknownSurface(t *testing.T) {
	srv, _ := seedRerate(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/rerate?surface=hidden", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (unknown surface)", rec.Code)
	}
}

func TestRerateSSEEmitsDone(t *testing.T) {
	srv, _ := seedRerate(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/rerate?surface=today", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: done") {
		t.Fatalf("SSE stream missing a terminal done event:\n%s", body)
	}
}

func TestRerateButtonHiddenWithoutKey(t *testing.T) {
	// AI off → no button rendered (design §4: no dead control).
	srv, _ := newTestServer(t, &fakeScraper{})
	saveSinipProfile(t, srv)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if strings.Contains(rec.Body.String(), `id="rerate"`) {
		t.Fatal("the re-rate button must be hidden when no AI key is configured")
	}

	// AI on → button present.
	srv2, _ := seedRerate(t)
	rec2 := httptest.NewRecorder()
	srv2.Handler().ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(rec2.Body.String(), `id="rerate"`) {
		t.Fatal("the re-rate button must be present when AI is configured")
	}
}

// TestRerateButtonShowsStaleCount: a visible row whose delta was computed
// against a prior profile gives the button the gold attention treatment with a
// count ("재평가 ·1" + has-stale).
func TestRerateButtonShowsStaleCount(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	ctx := context.Background()
	zero := 0
	prof := profile.Profile{CareerYears: 0, MinScore: &zero, JobLikes: "백엔드 서버 개발"}
	pj, _ := profile.Marshal(prof)
	if _, _, err := srv.store.SaveProfile(ctx, pj); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	now := time.Now().UTC()
	p := listingPosting("s1", "신입 백엔드")
	p.Description = "서버 개발자를 찾습니다"
	p.FirstSeenAt, p.LastSeenAt = now, now
	id, _, _ := srv.store.UpsertPosting(ctx, p)
	srv.SetAIProvider(rerateStub(), "test-model")

	// Seed a delta under a NON-current input hash → the merge falls back to it
	// and marks it stale (no fresh row for the profile's current goal text).
	if err := srv.store.UpsertAIScore(ctx, id, "stalehash9999", srv.aiVersion,
		ai.Delta{NetDelta: -3, Items: []ai.DeltaItem{{Signal: "x", Kind: ai.KindPresence, Delta: -3, Evidence: "서버 개발자를 찾습니다"}}}, now); err != nil {
		t.Fatalf("seed stale ai_score: %v", err)
	}
	if _, err := srv.scoreAll(ctx); err != nil {
		t.Fatalf("scoreAll: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	body := rec.Body.String()
	if !strings.Contains(body, "재평가 ·1") {
		t.Fatalf("expected the stale-count badge '재평가 ·1' on the button")
	}
	if !strings.Contains(body, "has-stale") {
		t.Fatalf("expected the has-stale attention class on the button")
	}
}

// TestRerateRespectsPerCallCap proves the user-adjustable per-call cap (R1):
// one press analyzes at most s.aiPerCallCap NOT-yet-cached rows even when the
// token budget has ample headroom, and a later press resumes on the rest. This
// is a legibility knob distinct from the hard token caps.
func TestRerateRespectsPerCallCap(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	ctx := context.Background()
	zero := 0
	prof := profile.Profile{CareerYears: 0, MinScore: &zero, JobLikes: "백엔드 서버 개발"}
	pj, _ := profile.Marshal(prof)
	if _, _, err := st.SaveProfile(ctx, pj); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	now := time.Now().UTC()
	const total = 5
	for i := 0; i < total; i++ {
		p := listingPosting("c"+string(rune('1'+i)), "신입 백엔드 개발자")
		p.Description = "서버 개발자를 찾습니다"
		p.FirstSeenAt, p.LastSeenAt = now, now
		if _, _, err := st.UpsertPosting(ctx, p); err != nil {
			t.Fatalf("UpsertPosting: %v", err)
		}
	}
	stub := rerateStub()
	srv.SetAIProvider(stub, "test-model")
	srv.aiRunTokenCap = 1_000_000 // generous: the per-call cap, not tokens, is the limiter
	srv.aiPerCallCap = 2          // analyze at most 2 fresh rows per press
	if _, err := srv.scoreAll(ctx); err != nil {
		t.Fatalf("scoreAll: %v", err)
	}

	// Press 1: 2 fresh rows analyzed, 3 left.
	summary, err := srv.runRerate(ctx, "today", noopEmit)
	if err != nil {
		t.Fatalf("press 1: %v", err)
	}
	if summary.Visible != total {
		t.Fatalf("press 1 visible = %d, want %d", summary.Visible, total)
	}
	if summary.Analyzed != 2 || stub.ScoreDeltaCalls != 2 {
		t.Fatalf("press 1: analyzed=%d ScoreDelta=%d, want 2 and 2 (per-call cap)", summary.Analyzed, stub.ScoreDeltaCalls)
	}
	// Press 2: 2 cache hits (free) + 2 new = 4 analyzed cumulatively, 4 calls total.
	summary, err = srv.runRerate(ctx, "today", noopEmit)
	if err != nil {
		t.Fatalf("press 2: %v", err)
	}
	if summary.Analyzed != 4 || stub.ScoreDeltaCalls != 4 {
		t.Fatalf("press 2: analyzed=%d ScoreDelta=%d, want 4 and 4", summary.Analyzed, stub.ScoreDeltaCalls)
	}
	// Press 3: the last row. All analyzed, no further presses needed.
	summary, err = srv.runRerate(ctx, "today", noopEmit)
	if err != nil {
		t.Fatalf("press 3: %v", err)
	}
	if summary.Analyzed != total || stub.ScoreDeltaCalls != total {
		t.Fatalf("press 3: analyzed=%d ScoreDelta=%d, want %d and %d", summary.Analyzed, stub.ScoreDeltaCalls, total, total)
	}
}

// TestRerateInfoCountsCacheNotChips proves the N/M indicator is honest about a
// row that was analyzed but produced no surviving signal (R1): such a row has a
// cached delta but renders NO AI chip. The indicator counts the cache, so it
// reads "1/1", not "0/1" — resolving the "analyzed or just silent?" ambiguity.
func TestRerateInfoCountsCacheNotChips(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	ctx := context.Background()
	zero := 0
	prof := profile.Profile{CareerYears: 0, MinScore: &zero, JobLikes: "백엔드 서버 개발"}
	pj, _ := profile.Marshal(prof)
	if _, _, err := srv.store.SaveProfile(ctx, pj); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	now := time.Now().UTC()
	p := listingPosting("e1", "신입 백엔드 개발자")
	p.Description = "서버 개발자를 찾습니다"
	p.FirstSeenAt, p.LastSeenAt = now, now
	id, _, err := srv.store.UpsertPosting(ctx, p)
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}
	srv.SetAIProvider(rerateStub(), "test-model")

	// An EMPTY current-goal delta: analyzed, but nothing survived the gate → no chip.
	hash := profile.AIInputHash(currentProfile(t, srv))
	if err := srv.store.UpsertAIScore(ctx, id, hash, srv.aiVersion, ai.Delta{}, now); err != nil {
		t.Fatalf("seed empty delta: %v", err)
	}
	if _, err := srv.scoreAll(ctx); err != nil {
		t.Fatalf("scoreAll: %v", err)
	}

	b, err := srv.buildBriefing(ctx, time.Now())
	if err != nil {
		t.Fatalf("buildBriefing: %v", err)
	}
	if b.Rerate == nil {
		t.Fatal("Rerate info nil; want analyzed/visible counts when AI is on")
	}
	if b.Rerate.Analyzed != 1 || b.Rerate.Visible != 1 {
		t.Fatalf("indicator = %d/%d, want 1/1 (the empty-delta cache row counts as analyzed)",
			b.Rerate.Analyzed, b.Rerate.Visible)
	}
	// The rendered page must NOT carry an AI chip for this row (chip-ai class),
	// yet the indicator still shows it analyzed.
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	body := rec.Body.String()
	if strings.Contains(body, "chip-ai") {
		t.Fatal("an empty-delta row rendered an AI chip; it should be silent (no surviving signal)")
	}
	if !strings.Contains(body, "AI 분석 1/1") {
		t.Fatalf("expected the persistent indicator 'AI 분석 1/1' on the page")
	}
}

func TestRerateDoneMessage(t *testing.T) {
	if got := rerateDoneMessage(rerateSummary{}); got != "지금 화면에 분석할 공고가 없어요." {
		t.Errorf("empty: %q", got)
	}
	if got := rerateDoneMessage(rerateSummary{Analyzed: 5, Visible: 5, ProviderCalls: 5}); got != "공고 5개를 모두 AI로 분석했어요." {
		t.Errorf("complete: %q", got)
	}
	got := rerateDoneMessage(rerateSummary{Analyzed: 3, Visible: 7, ProviderCalls: 3})
	if !strings.Contains(got, "3/7") || !strings.Contains(got, "다시 눌러") {
		t.Errorf("partial copy missing N/M or the press-again cue: %q", got)
	}
}

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

func currentProfile(t *testing.T, srv *Server) profile.Profile {
	t.Helper()
	j, _, ok, err := srv.store.Profile(context.Background())
	if err != nil || !ok {
		t.Fatalf("load profile: ok=%v err=%v", ok, err)
	}
	p, err := profile.Unmarshal(j)
	if err != nil {
		t.Fatalf("unmarshal profile: %v", err)
	}
	return p
}
