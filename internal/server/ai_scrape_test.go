package server

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ohchanwu/job-scraper/internal/ai"
	"github.com/ohchanwu/job-scraper/internal/profile"
	"github.com/ohchanwu/job-scraper/internal/scraper"
)

// newcomerStub is an AI provider that always extracts "신입 OK, 0 years, no
// education requirement" with a fixed token cost.
func newcomerStub() *ai.StubProvider {
	zero := 0
	return &ai.StubProvider{
		NameVal: "stub",
		ExtractFn: func(ctx context.Context, modelText string) (ai.Extraction, ai.Usage, error) {
			return ai.Extraction{MinCareer: 0, MaxCareer: &zero, Newcomer: true, EducationEnum: ai.EduNone, Evidence: "신입 환영"},
				ai.Usage{InputTokens: 100, OutputTokens: 20}, nil
		},
	}
}

func saveSinipProfile(t *testing.T, srv *Server) {
	t.Helper()
	j, _ := profile.Marshal(profile.Profile{CareerYears: 0})
	if _, _, err := srv.store.SaveProfile(context.Background(), j); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
}

// aiExtractionCount counts cached extractions under the server's ai_version
// (one row per posting in these tests).
func aiExtractionCount(t *testing.T, srv *Server) int {
	t.Helper()
	m, err := srv.store.AIExtractionsByPostingID(context.Background(), srv.aiVersion)
	if err != nil {
		t.Fatalf("count ai_extractions: %v", err)
	}
	return len(m)
}

// TestRunScrapeAutoRatesFreshBriefingWithStage2 proves the fix: a scrape now
// runs Stage-2 over the fresh briefing, so new postings carry their evidence-
// cited AI delta WITHOUT a manual 재평가 press.
func TestRunScrapeAutoRatesFreshBriefingWithStage2(t *testing.T) {
	mkDetail := func(id, title string) scraper.Posting {
		p := listingPosting(id, title)
		p.Description = "서버 개발자를 찾습니다" // the quote rerateStub cites → the gate keeps the delta
		return p
	}
	f := &fakeScraper{
		listing: []scraper.Posting{listingPosting("1", "백엔드 신입"), listingPosting("2", "서버 신입")},
		details: map[string]scraper.Posting{
			"1": mkDetail("1", "백엔드 신입"),
			"2": mkDetail("2", "서버 신입"),
		},
	}
	srv, st := newTestServer(t, f)
	srv.SetAIProvider(rerateStub(), "test-model") // ScoreDeltaFn set; Stage-1 backfill errors harmlessly
	ctx := context.Background()
	zero := 0
	pj, _ := profile.Marshal(profile.Profile{CareerYears: 0, MinScore: &zero, JobLikes: "백엔드 서버 개발"})
	if _, _, err := st.SaveProfile(ctx, pj); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	if _, err := srv.runScrape(ctx, noopEmit); err != nil {
		t.Fatalf("runScrape: %v", err)
	}

	// Both fresh, visible postings should have a Stage-2 delta cached against the
	// current goal — no 재평가 needed.
	if n := countAIScores(t, srv); n != 2 {
		t.Fatalf("after scrape: %d Stage-2 deltas cached, want 2 (auto-rated, no manual 재평가)", n)
	}
}

func TestRunScrapeStubProviderExtractsNewPostings(t *testing.T) {
	f := &fakeScraper{
		listing: []scraper.Posting{listingPosting("1", "백엔드 신입"), listingPosting("2", "AI 신입")},
		details: map[string]scraper.Posting{
			"1": listingPosting("1", "백엔드 신입"),
			"2": listingPosting("2", "AI 신입"),
		},
	}
	srv, st := newTestServer(t, f)
	stub := newcomerStub()
	srv.SetAIProvider(stub, "test-model")
	saveSinipProfile(t, srv)
	ctx := context.Background()

	if _, err := srv.runScrape(ctx, noopEmit); err != nil {
		t.Fatalf("runScrape: %v", err)
	}

	// One ai_extractions row per new posting, and exactly one Extract call each.
	if n := aiExtractionCount(t, srv); n != 2 {
		t.Errorf("ai_extractions rows = %d, want 2", n)
	}
	if stub.Calls != 2 {
		t.Errorf("Extract calls = %d, want 2 (one per new posting)", stub.Calls)
	}
	if len(f.detailCalls) != 2 {
		t.Errorf("FetchDetail calls = %d, want 2", len(f.detailCalls))
	}
	// The cached extraction is readable under the run's ai_version.
	exts, err := st.AIExtractionsByPostingID(ctx, srv.aiVersion)
	if err != nil || len(exts) != 2 {
		t.Fatalf("AIExtractionsByPostingID: got %d (err=%v), want 2", len(exts), err)
	}
	for id, e := range exts {
		if !e.Newcomer || e.EducationEnum != ai.EduNone {
			t.Errorf("posting %d extraction = %+v, want newcomer/none", id, e)
		}
	}
}

func TestRunScrapeProviderFailsScrapeContinues(t *testing.T) {
	f := &fakeScraper{
		listing: []scraper.Posting{listingPosting("1", "신입 공고")},
		details: map[string]scraper.Posting{"1": listingPosting("1", "신입 공고")},
	}
	srv, st := newTestServer(t, f)
	srv.SetAIProvider(&ai.StubProvider{NameVal: "stub", ExtractFn: func(ctx context.Context, _ string) (ai.Extraction, ai.Usage, error) {
		return ai.Extraction{}, ai.Usage{}, errors.New("provider down")
	}}, "test-model")
	saveSinipProfile(t, srv)
	ctx := context.Background()

	res, err := srv.runScrape(ctx, noopEmit)
	if err != nil {
		t.Fatalf("runScrape must not fail when the provider errors: %v", err)
	}
	if res.New != 1 || res.Scored != 1 {
		t.Errorf("ScrapeResult = %+v, want the posting still inserted + scored", res)
	}
	if n := aiExtractionCount(t, srv); n != 0 {
		t.Errorf("ai_extractions rows = %d, want 0 (provider failed, no cache row)", n)
	}
	// Posting is present and scored via the regex path.
	if ps, _ := st.AllPostings(ctx); len(ps) != 1 {
		t.Errorf("postings = %d, want 1", len(ps))
	}
}

func TestRunScrapeNoExtractorMakesNoAICalls(t *testing.T) {
	f := &fakeScraper{
		listing: []scraper.Posting{listingPosting("1", "신입")},
		details: map[string]scraper.Posting{"1": listingPosting("1", "신입")},
	}
	srv, _ := newTestServer(t, f) // no SetAIProvider — AI off (the default)
	saveSinipProfile(t, srv)
	if _, err := srv.runScrape(context.Background(), noopEmit); err != nil {
		t.Fatalf("runScrape: %v", err)
	}
	if n := aiExtractionCount(t, srv); n != 0 {
		t.Errorf("ai_extractions rows = %d, want 0 (AI off)", n)
	}
}

func TestRunScrapeAlreadySeenDoesNotExtract(t *testing.T) {
	f := &fakeScraper{
		listing: []scraper.Posting{listingPosting("1", "기존 공고")},
		details: map[string]scraper.Posting{"1": listingPosting("1", "기존 공고")},
	}
	srv, st := newTestServer(t, f)
	stub := newcomerStub()
	srv.SetAIProvider(stub, "test-model")
	saveSinipProfile(t, srv)
	ctx := context.Background()

	// Pre-insert the posting so it's "known" — the bump path must skip detail
	// AND extraction.
	known := listingPosting("1", "기존 공고")
	known.FirstSeenAt = time.Now().UTC()
	known.LastSeenAt = time.Now().UTC()
	if _, _, err := st.UpsertPosting(ctx, known); err != nil {
		t.Fatalf("seed posting: %v", err)
	}

	if _, err := srv.runScrape(ctx, noopEmit); err != nil {
		t.Fatalf("runScrape: %v", err)
	}
	if len(f.detailCalls) != 0 {
		t.Errorf("FetchDetail called %v for a known posting; want none", f.detailCalls)
	}
	if stub.Calls != 0 {
		t.Errorf("Extract called %d times for a known posting; want 0 (no steady-state token burn)", stub.Calls)
	}
	if n := aiExtractionCount(t, srv); n != 0 {
		t.Errorf("ai_extractions rows = %d, want 0", n)
	}
}

// TestExtractStage1CacheHitSkipsProvider exercises the cache-hit branch
// directly: a pre-seeded extraction for the posting's content_hash means no
// provider call.
func TestExtractStage1CacheHitSkipsProvider(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	stub := newcomerStub()
	srv.SetAIProvider(stub, "test-model")
	ctx := context.Background()

	p := listingPosting("1", "백엔드 신입")
	p.Description = "서버 개발자를 찾습니다"
	p.FirstSeenAt, p.LastSeenAt = time.Now().UTC(), time.Now().UTC()
	id, _, err := st.UpsertPosting(ctx, p)
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}
	_, contentHash, _ := ai.ModelInput(p)
	seeded := ai.Extraction{MinCareer: 0, Newcomer: true, EducationEnum: ai.EduNone, Evidence: "seeded"}
	if err := st.UpsertAIExtraction(ctx, id, contentHash, srv.aiVersion, seeded, time.Now()); err != nil {
		t.Fatalf("seed extraction: %v", err)
	}

	srv.extractStage1(ctx, id, p, time.Now().UTC(), srv.newAIBudget(ctx))
	if stub.Calls != 0 {
		t.Errorf("Extract called %d times on a cache hit; want 0", stub.Calls)
	}
}

func TestRunScrapePerRunBudgetHalts(t *testing.T) {
	f := &fakeScraper{
		listing: []scraper.Posting{listingPosting("1", "a"), listingPosting("2", "b"), listingPosting("3", "c")},
		details: map[string]scraper.Posting{
			"1": listingPosting("1", "a"), "2": listingPosting("2", "b"), "3": listingPosting("3", "c"),
		},
	}
	srv, _ := newTestServer(t, f)
	stub := newcomerStub() // each Extract spends 120 tokens (100 in + 20 out)
	srv.SetAIProvider(stub, "test-model")
	srv.aiRunTokenCap = 5 // tiny: the first call exhausts it
	saveSinipProfile(t, srv)

	var events []string
	emit := func(ev, data string) { events = append(events, ev+"\x00"+data) }

	if _, err := srv.runScrape(context.Background(), emit); err != nil {
		t.Fatalf("runScrape: %v", err)
	}
	if stub.Calls != 1 {
		t.Errorf("Extract calls = %d, want 1 (budget halts after the first)", stub.Calls)
	}
	if n := aiExtractionCount(t, srv); n != 1 {
		t.Errorf("ai_extractions rows = %d, want 1", n)
	}
	var sawColdStart bool
	for _, e := range events {
		if strings.HasPrefix(e, "status\x00") && strings.Contains(e, "AI 예산") {
			sawColdStart = true
		}
	}
	if !sawColdStart {
		t.Errorf("expected a calm cold-start status when the budget halted; events=%v", events)
	}
}

// TestRunScrapeEndToEndScoreCorrection ties T4 (extraction wired) to T3
// (cache-read scoring): a posting whose body reads "경력 5년 이상" (regex would
// score a 신입 user 0) is corrected by the AI extraction to newcomer=true and
// scores the full newcomer award.
func TestRunScrapeEndToEndScoreCorrection(t *testing.T) {
	body := listingPosting("1", "백엔드 엔지니어")
	body.Description = "경력 5년 이상 우대"
	body.Newcomer = false
	body.MinCareer = 5
	f := &fakeScraper{
		listing: []scraper.Posting{listingPosting("1", "백엔드 엔지니어")},
		details: map[string]scraper.Posting{"1": body},
	}
	srv, st := newTestServer(t, f)
	srv.SetAIProvider(newcomerStub(), "test-model")
	saveSinipProfile(t, srv) // 신입 (CareerYears 0)
	ctx := context.Background()

	if _, err := srv.runScrape(ctx, noopEmit); err != nil {
		t.Fatalf("runScrape: %v", err)
	}
	scores, err := st.ScoresByPostingID(ctx)
	if err != nil || len(scores) != 1 {
		t.Fatalf("ScoresByPostingID: got %d (err=%v), want 1", len(scores), err)
	}
	for _, sc := range scores {
		// 신입 award is the default career weight (25). Without the AI
		// correction the regex override would score this 0.
		if sc.Total != profile.DefaultCareerWeight {
			t.Errorf("Total = %d, want %d (AI-corrected full newcomer award)", sc.Total, profile.DefaultCareerWeight)
		}
	}
}
