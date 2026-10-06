package server

import (
	"context"
	"testing"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
)

func TestRatingRejectedPersistsWithoutSuccessfulCache(t *testing.T) {
	srv, stub, runtime := seedRerate(t)
	stub.ScoreDeltaFn = func(context.Context, string, string) ([]ai.RawDeltaItem, ai.Usage, error) {
		return []ai.RawDeltaItem{{Signal: "unsupported", Kind: ai.KindPresence, Delta: 25, Quote: "지어낸 회사 업무", MatchedGoal: "목표"}}, ai.Usage{InputTokens: 5}, nil
	}
	ctx := context.Background()
	summary, err := srv.runRerate(ctx, "today", noopEmit, 1, runtime)
	if err != nil || summary.Analyzed != 0 || stub.ScoreDeltaCalls != 2 {
		t.Fatalf("rejection is processed, not a success or provider failure: summary=%+v calls=%d err=%v", summary, stub.ScoreDeltaCalls, err)
	}
	var n int
	if err := srv.store.SQLDB().QueryRow(`SELECT COUNT(*) FROM ai_score_outcomes WHERE state='rejected' AND proposed=1 AND accepted=0`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("persisted rejection n=%d err=%v", n, err)
	}
	fresh, err := srv.store.AIScoresByPostingID(ctx, 1, profile.AIInputHash(currentProfile(t, srv)), runtime.ScoreVersion)
	if err != nil || len(fresh) != 0 {
		t.Fatalf("rejected rows must not be successful cache hits: %v %v", fresh, err)
	}
	assertStage1Usage(t, srv.store, 1, 10)
	stub.ScoreDeltaFn = rerateStub().ScoreDeltaFn
	summary, err = srv.runRerate(ctx, "today", noopEmit, 1, runtime)
	if err != nil || summary.Analyzed != 2 || stub.ScoreDeltaCalls != 4 {
		t.Fatalf("later manual retry: summary=%+v calls=%d err=%v", summary, stub.ScoreDeltaCalls, err)
	}
}
