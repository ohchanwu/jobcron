package server

import (
	"context"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
)

func TestRatingAutomaticDoesNotRetryKnownRejection(t *testing.T) {
	srv, stub, runtime := seedRerate(t)
	stub.ScoreDeltaFn = func(context.Context, string, string) ([]ai.RawDeltaItem, ai.Usage, error) {
		return []ai.RawDeltaItem{{Kind: ai.KindPresence, Signal: "unverified", MatchedGoal: "업무", Quote: "지어낸 공고 문장", Delta: 5}}, ai.Usage{InputTokens: 4}, nil
	}
	ctx := context.Background()
	if _, err := srv.runRerate(ctx, "today", noopEmit, 1, runtime); err != nil {
		t.Fatal(err)
	}
	postings, err := srv.visibleForRerate(ctx, "today", time.Now(), 1, runtime)
	if err != nil {
		t.Fatal(err)
	}
	prof := currentProfile(t, srv)
	n, calls, _, err := srv.rateStage2(ctx, postings, prof, 1, runtime, srv.newAIBudget(ctx, 1, runtime), &callCap{max: 200}, noopEmit, false)
	if err != nil || n != 0 || calls != 0 || stub.ScoreDeltaCalls != 2 {
		t.Fatalf("automatic pass retried known rejection: n=%d calls=%d provider=%d err=%v", n, calls, stub.ScoreDeltaCalls, err)
	}
	// Observation and ordinary merge are never triggers.
	if _, err := srv.buildBriefingWithRuntime(ctx, time.Now(), 1, runtime); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.scoreAll(ctx, 1, runtime); err != nil {
		t.Fatal(err)
	}
	if stub.ScoreDeltaCalls != 2 {
		t.Fatal("render/merge re-spent")
	}
}
