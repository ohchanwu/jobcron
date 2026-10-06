package server

import (
	"context"
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
	if !strings.Contains(cap.snapshot()[firstPrep][1], "1/3") {
		t.Errorf("first prep progress = %q, want it to start at 1/3", cap.snapshot()[firstPrep][1])
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
