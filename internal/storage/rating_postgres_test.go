package storage

import (
	"context"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
)

func TestRatingPostgresOutcomeIsolationAtomicityAndCascade(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	a := insertTestUser(t, st, "rating-a@example.invalid")
	b := insertTestUser(t, st, "rating-b@example.invalid")
	id, _, err := st.UpsertPosting(ctx, samplePosting())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	empty := AIScoreOutcome{State: AIScoreNoSignal, ComputedAt: now}
	rejected := AIScoreOutcome{State: AIScoreRejected, Proposed: 2, ComputedAt: now}
	if err := st.UpsertAIResult(ctx, a, id, "goal", "v2", ai.Delta{}, empty); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := st.UpsertAIScoreFailure(ctx, b, id, "goal", "v2", rejected); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		user  int64
		state string
		cache bool
	}{{a, AIScoreNoSignal, true}, {b, AIScoreRejected, false}} {
		outcomes, err := st.AIScoreOutcomesByPostingID(ctx, tc.user, "goal", "v2")
		if err != nil || len(outcomes) != 1 || outcomes[id].State != tc.state {
			t.Fatalf("cross-user outcome: user=%d %v %v", tc.user, outcomes, err)
		}
		_, ok, err := st.AIScore(ctx, tc.user, id, "goal", "v2")
		if err != nil || ok != tc.cache {
			t.Fatalf("cross-user success: user=%d ok=%v err=%v", tc.user, ok, err)
		}
	}
	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM ai_score_outcomes WHERE user_id=$1`, b).Scan(&n); err != nil || n != 1 {
		t.Fatalf("outcome became attempt ledger: %d %v", n, err)
	}
	for _, key := range [][2]string{{"other-goal", "v2"}, {"goal", "v1"}, {"goal", "other-model"}} {
		outcomes, err := st.AIScoreOutcomesByPostingID(ctx, a, key[0], key[1])
		if err != nil || len(outcomes) != 0 {
			t.Fatalf("stale/model outcome became current: %v %v", outcomes, err)
		}
	}
	if _, err := st.db.Exec(`ALTER TABLE ai_score_outcomes ADD CONSTRAINT fixture_reject_result CHECK (state <> 'rated')`); err != nil {
		t.Fatal(err)
	}
	d := ai.Delta{Items: []ai.DeltaItem{{Kind: ai.KindPresence, Delta: 20, Evidence: "서버 개발자를 찾습니다"}}, NetDelta: 20}
	if err := st.UpsertAIResult(ctx, b, id, "goal", "v2", d, AIScoreOutcome{State: AIScoreRated, Proposed: 1, Accepted: 1, ComputedAt: now}); err == nil {
		t.Fatal("fixture outcome write failure did not fail successful result")
	}
	if _, ok, err := st.AIScore(ctx, b, id, "goal", "v2"); err != nil || ok {
		t.Fatalf("half-committed score after outcome failure: %v %v", ok, err)
	}
	outcomes, err := st.AIScoreOutcomesByPostingID(ctx, b, "goal", "v2")
	if err != nil || outcomes[id].State != AIScoreRejected {
		t.Fatalf("atomic failure erased definite rejection: %v %v", outcomes, err)
	}
	if _, err := st.db.Exec(`DELETE FROM users WHERE id=$1`, a); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM ai_score_outcomes WHERE user_id=$1`, a).Scan(&n); err != nil || n != 0 {
		t.Fatalf("account cascade failed: %d %v", n, err)
	}
	if _, err := st.db.Exec(`DELETE FROM postings WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM ai_score_outcomes`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("posting cascade failed: %d %v", n, err)
	}
}
