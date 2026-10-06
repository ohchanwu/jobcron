package storage

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
)

func TestRatingFailureProtectsActualCacheWithoutProvenance(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			st := newTestStore(t)
			uid := int64(1)
			if dialect == "postgres" {
				st = newPostgresTestStore(t)
				uid = insertTestUser(t, st, "actual-cache@example.invalid")
			}
			ctx := context.Background()
			id, _, err := st.UpsertPosting(ctx, samplePosting())
			if err != nil {
				t.Fatal(err)
			}
			if err := st.UpsertAIScore(ctx, uid, id, "goal", "model", ai.Delta{}, time.Now()); err != nil {
				t.Fatal(err)
			}
			for _, state := range []string{AIScoreRejected, AIScoreFailed} {
				o := AIScoreOutcome{State: state, Proposed: 1, ComputedAt: time.Now()}
				if err := st.UpsertAIScoreFailure(ctx, uid, id, "goal", "model", o); err == nil {
					t.Fatalf("%s contradicted an actual successful cache without an outcome", state)
				}
			}
			out, err := st.AIScoreOutcomesByPostingID(ctx, uid, "goal", "model")
			if err != nil || len(out) != 0 {
				t.Fatalf("manufactured failure for actual cache: %v %v", out, err)
			}
			if _, ok, err := st.AIScore(ctx, uid, id, "goal", "model"); err != nil || !ok {
				t.Fatalf("cache lost: %v %v", ok, err)
			}
		})
	}
}

func TestRatingPostgresConcurrentFailureCannotOverwriteSuccess(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	uid := insertTestUser(t, st, "concurrent-result@example.invalid")
	other := insertTestUser(t, st, "concurrent-other@example.invalid")
	id, _, err := st.UpsertPosting(ctx, samplePosting())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		// Distinct goal identities exercise insertion as well as conflict update.
		hash := time.Unix(int64(i), 0).Format(time.RFC3339)
		start := make(chan struct{})
		var wg sync.WaitGroup
		var successErr, failureErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			successErr = st.UpsertAIResult(ctx, uid, id, hash, "model", ai.Delta{}, AIScoreOutcome{State: AIScoreNoSignal, ComputedAt: time.Now()})
		}()
		go func() {
			defer wg.Done()
			<-start
			failureErr = st.UpsertAIScoreFailure(ctx, uid, id, hash, "model", AIScoreOutcome{State: AIScoreRejected, Proposed: 1, ComputedAt: time.Now()})
		}()
		close(start)
		wg.Wait()
		if successErr != nil {
			t.Fatal(successErr)
		}
		// Either failure admitted first, or rejected after the real success. In
		// both serial orders the committed success must own the final provenance.
		out, err := st.AIScoreOutcomesByPostingID(ctx, uid, hash, "model")
		if err != nil || out[id].State != AIScoreNoSignal {
			t.Fatalf("contradictory concurrent result: %v %v (failure=%v)", out, err, failureErr)
		}
		if err := st.UpsertAIScoreFailure(ctx, other, id, hash, "model", AIScoreOutcome{State: AIScoreRejected, Proposed: 1, ComputedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
		otherOut, err := st.AIScoreOutcomesByPostingID(ctx, other, hash, "model")
		if err != nil || otherOut[id].State != AIScoreRejected {
			t.Fatalf("other user was protected by wrong cache: %v %v", otherOut, err)
		}
	}
}
