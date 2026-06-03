package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ohchanwu/job-scraper/internal/ai"
	"github.com/ohchanwu/job-scraper/internal/profile"
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

	n, err := srv.runRerate(ctx, "today", noopEmit)
	if err != nil {
		t.Fatalf("runRerate: %v", err)
	}
	if n != 2 {
		t.Errorf("rated %d rows, want 2", n)
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

	if _, err := srv.runRerate(ctx, "today", noopEmit); err != nil {
		t.Fatalf("first runRerate: %v", err)
	}
	callsAfterFirst := stub.ScoreDeltaCalls

	// A reconnect (second run, same profile) must resume entirely from cache —
	// no provider call, no double-spend (S8).
	n, err := srv.runRerate(ctx, "today", noopEmit)
	if err != nil {
		t.Fatalf("second runRerate: %v", err)
	}
	if n != 2 {
		t.Errorf("second run rated %d, want 2 (from cache)", n)
	}
	if stub.ScoreDeltaCalls != callsAfterFirst {
		t.Errorf("ScoreDelta called again on reconnect (%d → %d); cache must be reused",
			callsAfterFirst, stub.ScoreDeltaCalls)
	}
}

// TestRerateProgressesAcrossPressesUnderBudget proves the behavior a user with a
// large list depends on: when the per-run token budget halts a re-rate partway,
// the NEXT press skips the already-rated rows (Stage-2 cache hits, no re-spend)
// and rates the ones after them. Each posting's ScoreDelta is called exactly
// once across the two presses — never re-rated.
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
	const total = 4
	for i := 0; i < total; i++ {
		p := listingPosting("p"+string(rune('1'+i)), "신입 백엔드 개발자")
		p.Description = "서버 개발자를 찾습니다"
		p.FirstSeenAt, p.LastSeenAt = now, now
		if _, _, err := st.UpsertPosting(ctx, p); err != nil {
			t.Fatalf("UpsertPosting: %v", err)
		}
	}
	stub := rerateStub() // each ScoreDelta debits 60 tokens (50 in + 10 out)
	srv.SetAIProvider(stub, "test-model")
	srv.aiRunTokenCap = 90 // tiny: two ScoreDelta calls per run, then halt
	if _, err := srv.scoreAll(ctx); err != nil {
		t.Fatalf("scoreAll: %v", err)
	}

	// Press 1: the run budget halts after two rows.
	if _, err := srv.runRerate(ctx, "today", noopEmit); err != nil {
		t.Fatalf("press 1: %v", err)
	}
	if stub.ScoreDeltaCalls != 2 {
		t.Fatalf("after press 1: ScoreDelta calls = %d, want 2 (budget halts partway)", stub.ScoreDeltaCalls)
	}
	rated1 := countAIScores(t, srv)
	if rated1 != 2 {
		t.Fatalf("after press 1: %d rows rated, want 2", rated1)
	}

	// Press 2: the two already-rated rows are cache hits (skipped, no re-spend);
	// the budget rates the remaining two.
	if _, err := srv.runRerate(ctx, "today", noopEmit); err != nil {
		t.Fatalf("press 2: %v", err)
	}
	if stub.ScoreDeltaCalls != total {
		t.Fatalf("after press 2: total ScoreDelta calls = %d, want %d (each row rated exactly once, never re-rated)",
			stub.ScoreDeltaCalls, total)
	}
	if rated2 := countAIScores(t, srv); rated2 != total {
		t.Fatalf("after press 2: %d rows rated, want all %d", rated2, total)
	}

	// Press 3 (everything cached): no further provider calls at all.
	if _, err := srv.runRerate(ctx, "today", noopEmit); err != nil {
		t.Fatalf("press 3: %v", err)
	}
	if stub.ScoreDeltaCalls != total {
		t.Fatalf("press 3 made extra ScoreDelta calls (%d > %d) — a fully-rated list must re-spend nothing",
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
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (AI not configured)", rec.Code)
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
