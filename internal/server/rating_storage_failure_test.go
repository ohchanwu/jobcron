package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/storage"
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
	var warning *rerateStorageError
	if !errors.As(err, &warning) {
		t.Fatal("cache read failure must be a storage warning across automatic callers", err)
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

func TestPostingPersistenceWarningIsNotProviderFailure(t *testing.T) {
	message := analysisFailureMessage(&rerateStorageError{errors.New("private SQL diagnostic")})
	if message != "분석 결과를 저장하지 못했어요 — 저장된 결과만 보여드릴게요." {
		t.Fatalf("storage failure classification=%q", message)
	}
}

func TestPostingAutomaticPersistenceFailureRetainsRunWarning(t *testing.T) {
	for _, trigger := range []string{storage.ScrapeTriggerManual, storage.ScrapeTriggerScheduled} {
		t.Run(trigger, func(t *testing.T) {
			srv, stub, runtime := seedRerate(t)
			ctx := context.Background()
			prof := currentProfile(t, srv)
			prof.ScheduledAIEnabled = true
			stub.ScoreDeltaFn = func(context.Context, string, string) ([]ai.RawDeltaItem, ai.Usage, error) {
				return nil, ai.Usage{}, nil
			}
			if _, err := srv.store.SQLDB().Exec(`CREATE TRIGGER deny_auto_outcome BEFORE INSERT ON ai_score_outcomes BEGIN SELECT RAISE(ABORT, 'synthetic denied'); END`); err != nil {
				t.Fatal(err)
			}
			_, err := srv.analyzeUserCollection(ctx, trigger, noopSchedulerEmit, runtime.UserID, prof, runtime, srv.newAIBudget(ctx, runtime.UserID, runtime), scrapeCollection{now: time.Now()})
			if err != nil {
				t.Fatal("collection should retain its rule fallback", err)
			}
			status, ok := srv.rerates.snapshot(runtime.UserID, "today")
			if !ok || status.Message != "분석 결과를 저장하지 못했어요 — 저장된 결과만 보여드릴게요." {
				t.Fatal("unsaved automatic result claimed normal completion", status)
			}
			var outcomes int
			if err := srv.store.SQLDB().QueryRow(`SELECT COUNT(*) FROM ai_score_outcomes`).Scan(&outcomes); err != nil || outcomes != 0 {
				t.Fatal(outcomes, err)
			}
		})
	}
}
