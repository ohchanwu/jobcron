package server

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
)

func TestRatingProviderFailureIsNotCompletedAnalysis(t *testing.T) {
	srv, stub, runtime := seedRerate(t)
	stub.ScoreDeltaFn = func(context.Context, string, string) ([]ai.RawDeltaItem, ai.Usage, error) {
		return nil, ai.Usage{InputTokens: 5}, errors.New("malformed response after a successful provider call")
	}
	ctx := context.Background()
	summary, err := srv.runRerate(ctx, "today", noopEmit, 1, runtime)
	if err == nil || summary.Analyzed != 0 {
		t.Fatalf("failure became success: %+v %v", summary, err)
	}
	var n int
	if err := srv.store.SQLDB().QueryRow(`SELECT COUNT(*) FROM ai_score_outcomes WHERE state='failed'`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("failure provenance n=%d err=%v", n, err)
	}
	assertStage1Usage(t, srv.store, 1, 10)
	b, err := srv.buildBriefingWithRuntime(ctx, time.Now(), 1, runtime)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range b.Today {
		if row.AIOutcome == "no_signal" || row.AIOutcome == "rejected" {
			t.Fatalf("failure manufactured completed/rejected evidence: %+v", row)
		}
	}
	rec := httptest.NewRecorder()
	srv.render(rec, "index.html", b)
	if strings.Count(rec.Body.String(), "AI 분석을 완료하지 못했어요") != 2 {
		t.Fatal("a definite failure must differ from never attempted without looking completed")
	}
	// Never attempted budget skips retain the previous definite explanation.
	prof := currentProfile(t, srv)
	n1 := stub.ScoreDeltaCalls
	_, _, _, err = srv.rateStage2(ctx, postingsOf(b.Today), prof, 1, runtime, srv.newAIBudget(ctx, 1, runtime), &callCap{max: 0}, noopEmit, true)
	if err != nil || stub.ScoreDeltaCalls != n1 {
		t.Fatalf("budget skip attempted provider: %d %v", stub.ScoreDeltaCalls, err)
	}
	outcomes, err := srv.store.AIScoreOutcomesByPostingID(ctx, 1, profile.AIInputHash(prof), runtime.ScoreVersion)
	if err != nil || len(outcomes) != 2 {
		t.Fatalf("budget skip erased failure: %v %v", outcomes, err)
	}
}
