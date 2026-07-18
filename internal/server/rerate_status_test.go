package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
)

func TestRerateTrackerRecordsLifecycle(t *testing.T) {
	tracker := newRerateTracker()
	started := tracker.start(7, "today", "entry-token-00000001")
	tracker.record(7, "today", started.RunID, "status", "AI로 다시 분석하는 중이에요")
	tracker.record(7, "today", started.RunID, "progress", "공고 3/9 분석 중...")

	running, ok := tracker.snapshot(7, "today")
	if !ok || running.State != rerateStateRunning {
		t.Fatalf("running snapshot = %+v, ok=%v", running, ok)
	}
	if running.Status != "AI로 다시 분석하는 중이에요" || running.Progress != "공고 3/9 분석 중..." {
		t.Fatalf("running copy = %+v", running)
	}

	tracker.complete(7, "today", started.RunID, rerateOutcomeChanged, "완료")
	done, _ := tracker.snapshot(7, "today")
	if done.State != rerateStateDone || done.Outcome != rerateOutcomeChanged || done.Message != "완료" {
		t.Fatalf("done snapshot = %+v", done)
	}
}

func TestRerateTrackerIgnoresStaleRunUpdates(t *testing.T) {
	tracker := newRerateTracker()
	old := tracker.start(7, "today", "entry-token-00000001")
	current := tracker.start(7, "today", "entry-token-00000002")
	tracker.complete(7, "today", old.RunID, rerateOutcomeChanged, "stale")

	got, _ := tracker.snapshot(7, "today")
	if got.RunID != current.RunID || got.State != rerateStateRunning || got.Message != "" {
		t.Fatalf("snapshot accepted stale update: %+v", got)
	}
}

func TestRerateTrackerInvalidatesOneUser(t *testing.T) {
	tracker := newRerateTracker()
	for _, key := range []rerateKey{{userID: 7, surface: "today"}, {userID: 7, surface: "bookmarks"}, {userID: 8, surface: "today"}} {
		run := tracker.start(key.userID, key.surface, "entry-token-00000001")
		tracker.complete(key.userID, key.surface, run.RunID, rerateOutcomeCached, "cached")
	}
	tracker.invalidateUser(7)
	if _, ok := tracker.snapshot(7, "today"); ok {
		t.Fatal("user 7 today status survived invalidation")
	}
	if _, ok := tracker.snapshot(7, "bookmarks"); ok {
		t.Fatal("user 7 bookmarks status survived invalidation")
	}
	if got, ok := tracker.snapshot(8, "today"); !ok || got.State != rerateStateDone {
		t.Fatalf("user 8 status changed: %+v ok=%v", got, ok)
	}
}

func TestRerateTrackerRunTokensDoNotRepeatAcrossProcesses(t *testing.T) {
	first := newRerateTracker().start(7, "today", "entry-token-00000001")
	second := newRerateTracker().start(7, "today", "entry-token-00000001")
	if first.RunToken == "" || second.RunToken == "" || first.RunToken == second.RunToken {
		t.Fatalf("process run tokens = %q, %q; want distinct non-empty values", first.RunToken, second.RunToken)
	}
}

func TestRerateStatusEndpoint(t *testing.T) {
	srv, _, _ := seedRerate(t)
	started := srv.rerates.start(0, "today", "entry-token-00000001")
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
	srv, _, _ := seedRerate(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/rerate/status?surface=hidden", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestRerateInfoMarksMissingDealbreakerValidationPending(t *testing.T) {
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	ctx := context.Background()
	userID := insertAIRuntimeTestUser(t, st, "rerate-pending@example.invalid")
	prof := profile.Profile{Dealbreakers: []string{"리서치"}}
	saveAIRuntimeProfile(t, st, userID, prof)
	p := listingPosting("rerate-pending", "신입 리서치 개발자")
	p.Description = "리서치 업무를 수행합니다"
	p.FirstSeenAt, p.LastSeenAt = time.Now().UTC(), time.Now().UTC()
	p.ID = mustUpsert(t, st, p)
	runtime := testAIRuntime(userID, &ai.StubProvider{NameVal: "stub"}, "shared-model")
	if _, err := srv.scoreAll(ctx, userID, runtime); err != nil {
		t.Fatal(err)
	}
	briefing, err := srv.buildBriefingWithRuntime(ctx, time.Now(), userID, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if briefing.Rerate == nil || briefing.Rerate.StaleCount != 1 {
		t.Fatalf("rerate info = %+v, want one pending contextual validation", briefing.Rerate)
	}
}

func TestRerateStreamRequiresValidatedHistoryEntry(t *testing.T) {
	for _, target := range []string{
		"/api/rerate?surface=today",
		"/api/rerate?surface=today&entry=bad!",
	} {
		srv, _, _ := seedRerate(t)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s = %d, want 400", target, rec.Code)
		}
	}
}
