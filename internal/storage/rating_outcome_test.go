package storage

import (
	"context"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
)

func TestRatingOutcomeCannotContradictSuccessfulCache(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	id, _, err := st.UpsertPosting(ctx, samplePosting())
	if err != nil {
		t.Fatal(err)
	}
	o := AIScoreOutcome{State: AIScoreNoSignal, ComputedAt: time.Now()}
	if err := st.UpsertAIResult(ctx, 1, id, "goal", "v2", ai.Delta{}, o); err != nil {
		t.Fatal(err)
	}
	err = st.UpsertAIScoreFailure(ctx, 1, id, "goal", "v2", AIScoreOutcome{State: AIScoreRejected, Proposed: 1, ComputedAt: time.Now()})
	if err == nil {
		t.Fatal("failure overwrote provenance of a successful exact cache identity")
	}
	outcomes, err := st.AIScoreOutcomesByPostingID(ctx, 1, "goal", "v2")
	if err != nil || outcomes[id].State != AIScoreNoSignal {
		t.Fatalf("success lost: %v %v", outcomes, err)
	}
}

func TestRatingSQLiteOutcomeRejectsAnotherUser(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	id, _, err := st.UpsertPosting(ctx, samplePosting())
	if err != nil {
		t.Fatal(err)
	}
	// SQLite caches intentionally remain the legacy single-user/demo schema.
	// Reject cross-user writes instead of misbinding user-scoped outcome provenance.
	if err := st.UpsertAIResult(ctx, 2, id, "goal", "v2", ai.Delta{}, AIScoreOutcome{State: AIScoreNoSignal, ComputedAt: time.Now()}); err == nil {
		t.Fatal("SQLite new result API aliased another user into the legacy score cache")
	}
	if _, ok, err := st.AIScore(ctx, 1, id, "goal", "v2"); err != nil || ok {
		t.Fatalf("other user's result leaked: ok=%v err=%v", ok, err)
	}
}
