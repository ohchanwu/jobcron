package server

import (
	"context"
	"testing"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/scraper"
)

func TestRatingManualAdmitsEachRowOnlyOncePerRun(t *testing.T) {
	srv, stub, runtime := seedRerate(t)
	stub.ScoreDeltaFn = func(context.Context, string, string) ([]ai.RawDeltaItem, ai.Usage, error) {
		return []ai.RawDeltaItem{{Signal: "x", MatchedGoal: "목표", Kind: ai.KindPresence, Delta: 4, Quote: "허구의 공고 내용"}}, ai.Usage{}, nil
	}
	ctx := context.Background()
	all, err := srv.store.AllPostings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rows := []scraper.Posting{all[0], all[0], all[0]}
	n, calls, processed, err := srv.rateStage2(ctx, rows, currentProfile(t, srv), 1, runtime, srv.newAIBudget(ctx, 1, runtime), &callCap{max: 200}, noopEmit, true)
	if err != nil || n != 0 || calls != 1 || processed != 1 || stub.ScoreDeltaCalls != 1 {
		t.Fatalf("same-run rejection retried: n=%d calls=%d processed=%d provider=%d err=%v", n, calls, processed, stub.ScoreDeltaCalls, err)
	}
}
