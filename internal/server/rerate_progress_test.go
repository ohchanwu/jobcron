package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
)

// captureEmit records SSE events in order for progress-copy assertions.
type captureEmit struct {
	mu     sync.Mutex
	events [][2]string
}

func (c *captureEmit) emit(event, data string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, [2]string{event, data})
}

func (c *captureEmit) snapshot() [][2]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([][2]string(nil), c.events...)
}

func (c *captureEmit) firstIndex(event, prefix string) int {
	for i, ev := range c.snapshot() {
		if ev[0] == event && strings.HasPrefix(ev[1], prefix) {
			return i
		}
	}
	return -1
}

// seedRerateProgress seeds n visible postings on today (SQLite, user 1) and
// returns the server plus a capturing emitter.
func seedRerateProgress(t *testing.T, n int) (*Server, *captureEmit, *AIRuntime) {
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
	for i := 0; i < n; i++ {
		p := listingPosting(fmt.Sprintf("prog-%d", i), "신입 백엔드 개발자")
		p.Description = "서버 개발자를 찾습니다"
		p.FirstSeenAt, p.LastSeenAt = now, now
		if _, _, err := st.UpsertPosting(ctx, p); err != nil {
			t.Fatalf("UpsertPosting: %v", err)
		}
	}
	stub := &ai.StubProvider{
		NameVal: "stub",
		ScoreDeltaFn: func(context.Context, string, string) ([]ai.RawDeltaItem, ai.Usage, error) {
			return []ai.RawDeltaItem{
				{Signal: "백엔드", Kind: ai.KindPresence, Delta: 7, Quote: "서버 개발자를 찾습니다", MatchedGoal: "좋아하는 업무"},
			}, ai.Usage{InputTokens: 50, OutputTokens: 10}, nil
		},
	}
	runtime := testAIRuntime(1, stub, "test-model")
	if _, err := srv.scoreAll(ctx, 1, runtime); err != nil {
		t.Fatalf("scoreAll: %v", err)
	}
	cap := &captureEmit{}
	return srv, cap, runtime
}

// TestRunRerateOpensWithEstimateCopy: the FIRST status event of a press is the
// calm 5–10 minute estimate (with its variability qualifier), not the old
// generic mid-run line, and no progress event may precede it — a user who just
// pressed the button must immediately see how long the wait roughly is.
func TestRunRerateOpensWithEstimateCopy(t *testing.T) {
	srv, cap, runtime := seedRerateProgress(t, 3)
	if _, err := srv.runRerate(context.Background(), "today", cap.emit, 1, runtime); err != nil {
		t.Fatalf("runRerate: %v", err)
	}
	firstStatus := cap.firstIndex("status", "")
	if firstStatus == -1 {
		t.Fatalf("no status event emitted: %v", cap.snapshot())
	}
	got := cap.snapshot()[firstStatus][1]
	for _, want := range []string{"5–10분", "커피", "더 오래 걸릴 수 있어요"} {
		if !strings.Contains(got, want) {
			t.Errorf("first status = %q, want it to contain %q", got, want)
		}
	}
	if strings.Contains(got, "보장") || strings.Contains(got, "정확히") {
		t.Errorf("first status = %q must stay a rough estimate, not a promise", got)
	}
	if prog := cap.firstIndex("progress", ""); prog != -1 && prog < firstStatus {
		t.Errorf("progress event at %d precedes first status at %d — the estimate must lead: %v", prog, firstStatus, cap.snapshot())
	}
}

// TestRunRerateCountsStage1APreparationProgress: the Stage-1A backfill loop —
// previously fully silent — must emit per-row numeric progress (phase-labeled
// 정보 확인, deliberately distinct from the AI 분석 stage) before Stage 2's
// first 분석 중 line.
func TestRunRerateCountsStage1APreparationProgress(t *testing.T) {
	srv, cap, runtime := seedRerateProgress(t, 3)
	if _, err := srv.runRerate(context.Background(), "today", cap.emit, 1, runtime); err != nil {
		t.Fatalf("runRerate: %v", err)
	}
	firstPrep := cap.firstIndex("progress", "공고 정보 확인 ")
	if firstPrep == -1 {
		t.Fatalf("no 공고 정보 확인 progress emitted: %v", cap.snapshot())
	}
	if !strings.Contains(cap.snapshot()[firstPrep][1], "0/3") {
		t.Errorf("first prep progress = %q, want it to open at the honest 0/3 before the first blocking call", cap.snapshot()[firstPrep][1])
	}
	lastPrep := -1
	for i, ev := range cap.snapshot() {
		if ev[0] == "progress" && strings.HasPrefix(ev[1], "공고 정보 확인 ") {
			lastPrep = i
		}
	}
	if lastPrep == -1 || !strings.Contains(cap.snapshot()[lastPrep][1], "3/3") {
		t.Errorf("prep progress never reached 3/3: %v", cap.snapshot())
	}
	firstStage2 := cap.firstIndex("progress", "공고 ")
	if firstStage2 == -1 {
		t.Fatalf("no stage-2 progress emitted: %v", cap.snapshot())
	}
	// The stage-2 counter ("공고 N/M 분석 중...") must not start before the
	// preparation counter finished — otherwise two N/M counters interleave.
	for i, ev := range cap.snapshot() {
		if ev[0] == "progress" && strings.HasPrefix(ev[1], "공고 ") && strings.Contains(ev[1], "분석 중") {
			if i < lastPrep {
				t.Errorf("stage-2 progress %q at %d interleaves with prep progress ending at %d", ev[1], i, lastPrep)
			}
			break
		}
	}
}

// activeStatusCarriers returns, in order, every status event a full press
// emits. The user-requested approximate 5–10분/coffee copy must survive the
// WHOLE active wait: every status during the run (except deliberate, meaningful
// failure/budget notices) carries the estimate tokens, so a mid-run line can
// never overwrite the estimate with a generic "analyzing" phrase.
func activeStatusCarriers(cap *captureEmit) []string {
	var out []string
	for _, ev := range cap.snapshot() {
		if ev[0] != "status" {
			continue
		}
		// Meaningful notices that must be allowed to replace the estimate:
		// failure/budget/degradation guidance, and the FINAL rescoring line —
		// claiming "약 5–10분" during the seconds-long terminal rescore (after
		// every provider call is done) would be dishonest, not calming.
		if strings.Contains(ev[1], "예산") || strings.Contains(ev[1], "확인해 주세요") ||
			strings.Contains(ev[1], "확인해주세요") || strings.Contains(ev[1], "다시 시도") ||
			strings.Contains(ev[1], "없이 일반 점수") || strings.Contains(ev[1], "점수를 다시 매기는") {
			continue
		}
		out = append(out, ev[1])
	}
	return out
}

func estimateCopyTokens() []string {
	return []string{"5–10분", "커피", "더 오래 걸릴 수 있어요"}
}

// TestRunRerateEstimateSurvivesFullRun: after the opening estimate, the
// mid-run status lines must RE-ANCHOR the same approximate wait (coffee copy),
// not fall back to the old generic phrase — and the terminal rescoring line
// must not drop it either. Proven on the full real event sequence.
func TestRunRerateEstimateSurvivesFullRun(t *testing.T) {
	srv, cap, runtime := seedRerateProgress(t, 3)
	if _, err := srv.runRerate(context.Background(), "today", cap.emit, 1, runtime); err != nil {
		t.Fatalf("runRerate: %v", err)
	}
	carriers := activeStatusCarriers(cap)
	if len(carriers) == 0 {
		t.Fatalf("no active status events: %v", cap.snapshot())
	}
	for i, got := range carriers {
		for _, want := range estimateCopyTokens() {
			if !strings.Contains(got, want) {
				t.Errorf("active status #%d = %q, want it to keep the estimate token %q", i, got, want)
			}
		}
	}
}

// TestRunRerateEmitsZeroProgressBeforeBlockingWork: a numeric, phase-labeled
// 0/N must reach the client BEFORE the first blocking provider call of each
// phase — the press may not sit on a text-only placeholder while the first
// paced call runs. Proven with a FUNDED delayed Stage-1A sponsor (real
// Postgres, real extract latency): the 0/N must be recorded before Extract
// returns for the first candidate.
func TestRunRerateEmitsZeroProgressBeforeBlockingWork(t *testing.T) {
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	ctx := context.Background()
	userID := insertAIRuntimeTestUser(t, st, "rerate-zero-progress@example.invalid")
	sponsorID := insertAIRuntimeTestUser(t, st, "rerate-zero-sponsor@example.invalid")
	zero := 0
	prof := profile.Profile{CareerYears: 0, MinScore: &zero, JobLikes: "백엔드 서버 개발"}
	saveAIRuntimeProfile(t, st, userID, prof)

	now := time.Now().UTC()
	for i := 0; i < 2; i++ {
		p := listingPosting(fmt.Sprintf("zero-%d", i), "신입 백엔드 개발자")
		p.Description = "서버 개발자를 찾습니다"
		p.FirstSeenAt, p.LastSeenAt = now, now
		if _, _, err := st.UpsertPosting(ctx, p); err != nil {
			t.Fatalf("UpsertPosting: %v", err)
		}
	}

	// A funded sponsor whose Extract BLOCKS until the test observes the 0/N
	// event — proving the event fires before the first provider call returns.
	extractEntered := make(chan struct{}, 1)
	extractReturn := make(chan struct{})
	sponsor := &ai.StubProvider{
		NameVal: "anthropic",
		ExtractFn: func(context.Context, string) (ai.Extraction, ai.Usage, error) {
			select {
			case extractEntered <- struct{}{}:
			default:
			}
			<-extractReturn
			return ai.Extraction{Newcomer: true, EducationEnum: ai.EduNone}, ai.Usage{InputTokens: 2}, nil
		},
	}
	cipher := newAIRuntimeTestCipher(t, 0x5A)
	srv.SetCredentialCipher(cipher)
	saveAIRuntimeProfile(t, st, sponsorID, profile.Profile{
		CareerYears: 0,
		AIProvider:  "anthropic",
		AIModel:     "sponsor-model",
	})
	saveAIRuntimeCredential(t, st, cipher, sponsorID, "anthropic", stage1SponsorFixture)
	srv.newAIProvider = func(_ string, key string, _ string, _ time.Duration) (ai.Provider, error) {
		if key != stage1SponsorFixture {
			return nil, errors.New("unexpected synthetic credential")
		}
		return sponsor, nil
	}
	srv.stage1SponsorUserID = sponsorID

	userProvider := &ai.StubProvider{
		NameVal: "stub",
		ScoreDeltaFn: func(context.Context, string, string) ([]ai.RawDeltaItem, ai.Usage, error) {
			return []ai.RawDeltaItem{
				{Signal: "백엔드", Kind: ai.KindPresence, Delta: 7, Quote: "서버 개발자를 찾습니다", MatchedGoal: "좋아하는 업무"},
			}, ai.Usage{InputTokens: 50, OutputTokens: 10}, nil
		},
	}
	runtime := testAIRuntime(userID, userProvider, "test-model")
	if _, err := srv.scoreAll(ctx, userID, runtime); err != nil {
		t.Fatalf("scoreAll: %v", err)
	}

	cap := &captureEmit{}
	runDone := make(chan error, 1)
	go func() {
		_, err := srv.runRerate(ctx, "today", cap.emit, userID, runtime)
		runDone <- err
	}()
	<-extractEntered // the first Stage-1A Extract call is now blocking

	// The 0/N preparation event must already be recorded — emitted before the
	// loop's first blocking call, not after it returns.
	firstPrep := cap.firstIndex("progress", "공고 정보 확인 0/")
	if firstPrep == -1 {
		t.Fatalf("no 공고 정보 확인 0/N before the first blocking extract: %v", cap.snapshot())
	}
	firstStatus := cap.firstIndex("status", "")
	if firstStatus == -1 || firstStatus > firstPrep {
		t.Fatalf("estimate status must precede the first prep progress: %v", cap.snapshot())
	}
	close(extractReturn)
	if err := <-runDone; err != nil {
		t.Fatalf("runRerate: %v", err)
	}
}

// TestRunReratePhaseProgressCountsHonestRows: through the whole run, each
// phase's counter must advance only for rows actually processed in that phase,
// and no phase may present skipped/failed rows as 분석 (analyzed):
//   - Stage-1A 정보 확인 counts every iterated candidate (provider-funded or not).
//   - Stage-1B 문맥 확인 counts every row the phase examined — including rows
//     skipped by budget/cap and rows whose provider call FAILED — because those
//     were processed-by-skipping, never as analyzed.
//   - Stage-2 분석 중 counts each completed row once, out-of-order-safe, and
//     its first event is an honest 0/M before any provider call completes.
func TestRunReratePhaseProgressCountsHonestRows(t *testing.T) {
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	ctx := context.Background()
	userID := insertAIRuntimeTestUser(t, st, "rerate-phase-honest@example.invalid")
	zero := 0
	prof := profile.Profile{CareerYears: 0, MinScore: &zero, Dealbreakers: []string{"리서치"}, JobLikes: "백엔드 서버 개발"}
	saveAIRuntimeProfile(t, st, userID, prof)

	now := time.Now().UTC()
	const rows = 4
	for i := 0; i < rows; i++ {
		p := listingPosting(fmt.Sprintf("phase-%d", i), "신입 리서치 개발자")
		p.Description = "리서치 아님. 서버 개발자를 찾습니다"
		p.FirstSeenAt, p.LastSeenAt = now, now
		if _, _, err := st.UpsertPosting(ctx, p); err != nil {
			t.Fatalf("UpsertPosting: %v", err)
		}
	}

	// Stage-1B: first two rows resolve, third FAILS against the provider,
	// fourth is skipped by the per-call cap. Stage-2: first row fails, the
	// rest rate fine — a partial provider failure must not corrupt counts.
	var contextCalls, scoreCalls int
	provider := &ai.StubProvider{
		NameVal: "stub",
		ValidateDealbreakersFn: func(_ context.Context, _ string, candidates []ai.DealbreakerCandidate) ([]ai.DealbreakerValidation, ai.Usage, error) {
			n := contextCalls
			contextCalls++
			if n == 2 {
				return nil, ai.Usage{}, errors.New("synthetic provider failure")
			}
			return []ai.DealbreakerValidation{{
				CandidateID: candidates[0].ID,
				Verdict:     ai.DealbreakerNotApplicable,
				ReasonCode:  ai.DealbreakerReasonExplicitlyNegated,
			}}, ai.Usage{InputTokens: 2}, nil
		},
		ScoreDeltaFn: func(context.Context, string, string) ([]ai.RawDeltaItem, ai.Usage, error) {
			n := scoreCalls
			scoreCalls++
			if n == 0 {
				return nil, ai.Usage{}, errors.New("synthetic stage-2 failure")
			}
			return []ai.RawDeltaItem{
				{Signal: "백엔드", Kind: ai.KindPresence, Delta: 7, Quote: "서버 개발자를 찾습니다", MatchedGoal: "좋아하는 업무"},
			}, ai.Usage{InputTokens: 50, OutputTokens: 10}, nil
		},
	}
	runtime := testAIRuntime(userID, provider, "test-model")
	// Shared cap = rows+2: Stage-1B's four attempts (three resolving, one
	// failing) consume four slots, leaving exactly TWO for Stage-2's four
	// visible rows — the first ScoreDelta FAILS, the second succeeds, and the
	// last two rows are processed as cap-skips. Every phase counter must
	// still complete (each row was processed), while 분석 counts stay honest
	// (analyzed=1, not 4).
	runtime.PerCallCap = rows + 2
	if _, err := srv.scoreAll(ctx, userID, runtime); err != nil {
		t.Fatalf("scoreAll: %v", err)
	}

	cap := &captureEmit{}
	summary, err := srv.runRerate(ctx, "today", cap.emit, userID, runtime)
	if err != nil {
		t.Fatalf("runRerate: %v", err)
	}

	events := cap.snapshot()
	// The row whose Stage-1B provider call FAILED stays dealbreaker-excluded,
	// so Stage 2 sees rows-1 visible rows — its denominator is the visible
	// set, deliberately distinct from the preparation/context denominators.
	const stage2Total = rows - 1
	var sawStage2Zero bool
	var stage2Counts []int
	lastContext := -1
	for _, ev := range events {
		if ev[0] != "progress" {
			continue
		}
		var n, m int
		switch {
		case strings.HasPrefix(ev[1], "공고 정보 확인 "):
			// Stage-1A: every candidate counted exactly once, 0/N leading.
			if _, err := fmt.Sscanf(ev[1], "공고 정보 확인 %d/%d...", &n, &m); err != nil {
				t.Fatalf("malformed prep progress %q: %v", ev[1], err)
			}
			if m != rows {
				t.Errorf("prep progress %q denominator = %d, want %d", ev[1], m, rows)
			}
		case strings.HasPrefix(ev[1], "공고 문맥 확인 "):
			if _, err := fmt.Sscanf(ev[1], "공고 문맥 확인 %d/%d...", &n, &m); err != nil {
				t.Fatalf("malformed context progress %q: %v", ev[1], err)
			}
			if m != rows {
				t.Errorf("context progress %q denominator = %d, want %d", ev[1], m, rows)
			}
			if n != lastContext+1 {
				t.Errorf("context progress %q advances %d → %d, want strictly sequential", ev[1], lastContext, n)
			}
			lastContext = n
		case strings.HasPrefix(ev[1], "공고 ") && strings.Contains(ev[1], "분석 중"):
			if _, err := fmt.Sscanf(ev[1], "공고 %d/%d 분석 중...", &n, &m); err != nil {
				t.Fatalf("malformed stage-2 progress %q: %v", ev[1], err)
			}
			if m != stage2Total {
				t.Errorf("stage-2 progress %q denominator = %d, want the visible count %d", ev[1], m, stage2Total)
			}
			if n == 0 {
				sawStage2Zero = true
				continue // the honest 0/M opener, not a completed row
			}
			stage2Counts = append(stage2Counts, n)
		}
	}
	if lastContext != rows {
		t.Errorf("context counter finished at %d, want %d (every examined row counted, incl. failed + cap-skipped)", lastContext, rows)
	}
	if !sawStage2Zero {
		t.Errorf("stage-2 never emitted an honest 0/%d before worker results: %v", rows, events)
	}
	// After the 0/M opener, each completed row counts exactly once — provider
	// failures and cap-skips still COMPLETE a row, so the counter reaches M.
	for i, n := range stage2Counts {
		if n != i+1 {
			t.Errorf("stage-2 count #%d = %d, want %d (each completed row counted exactly once)", i, n, i+1)
		}
	}
	if len(stage2Counts) != stage2Total {
		t.Errorf("stage-2 emitted %d counts after the opener, want %d (failed + cap-skipped rows still complete)", len(stage2Counts), stage2Total)
	}
	// The 분석 count stays honest: only the row whose provider call SUCCEEDED
	// is analyzed — the failed call and the cap-skips must not inflate it.
	if summary.Analyzed != 1 || summary.Visible != stage2Total {
		t.Errorf("summary analyzed=%d visible=%d, want 1/%d (failures and skips are never counted as 분석)", summary.Analyzed, summary.Visible, stage2Total)
	}
	if provider.ScoreDeltaCalls != 2 {
		t.Errorf("ScoreDelta calls = %d, want 2 (the shared cap bound)", provider.ScoreDeltaCalls)
	}
}
