package server

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRatingCacheReadFailureNeverStartsProvider(t *testing.T) {
	srv, stub, runtime := seedRerate(t)
	ctx := context.Background()
	rows, err := srv.store.AllPostings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.store.SQLDB().Exec(`DROP TABLE ai_scores`); err != nil {
		t.Fatal(err)
	}
	n, calls, _, err := srv.rateStage2(ctx, rows, currentProfile(t, srv), 1, runtime, srv.newAIBudget(ctx, 1, runtime), &callCap{max: 200}, noopEmit, true)
	if err == nil || n != 0 || calls != 0 || stub.ScoreDeltaCalls != 0 {
		t.Fatalf("uncertain cache read started spending: n=%d calls=%d provider=%d err=%v", n, calls, stub.ScoreDeltaCalls, err)
	}
}

func TestRatingAtomicOutcomeWriteFailureCannotIncrementSuccess(t *testing.T) {
	srv, stub, runtime := seedRerate(t)
	ctx := context.Background()
	if _, err := srv.store.SQLDB().Exec(`CREATE TRIGGER deny_ai_outcome BEFORE INSERT ON ai_score_outcomes BEGIN SELECT RAISE(ABORT, 'fixture outcome write denied'); END`); err != nil {
		t.Fatal(err)
	}
	summary, err := srv.runRerate(ctx, "today", noopEmit, 1, runtime)
	if err == nil || summary.Analyzed != 0 || stub.ScoreDeltaCalls != 2 {
		t.Fatalf("write failure success: %+v calls=%d err=%v", summary, stub.ScoreDeltaCalls, err)
	}
	var providerErr *providerCallError
	if errors.As(err, &providerErr) {
		t.Fatal("persistence failure was misclassified as a provider failure")
	}
	for _, table := range []string{"ai_scores", "ai_score_outcomes"} {
		var n int
		if err := srv.store.SQLDB().QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("half-committed %s n=%d err=%v", table, n, err)
		}
	}
	assertStage1Usage(t, srv.store, 1, 120)
	b, err := srv.buildBriefingWithRuntime(ctx, time.Now(), 1, runtime)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range b.Today {
		if p.AIOutcome != "" {
			t.Fatalf("failed write manufactured provenance: %+v", p)
		}
	}
}
