package storage

import (
	"context"
	"testing"
	"time"

	"github.com/ohchanwu/job-scraper/internal/ai"
)

func sampleDelta(net int) ai.Delta {
	return ai.Delta{
		Items: []ai.DeltaItem{
			{Signal: "백엔드", Kind: ai.KindPresence, Delta: net, Evidence: "서버 개발자를 찾습니다", MatchedGoal: "좋아하는 업무"},
		},
		NetDelta: net,
	}
}

func TestUpsertAIScoreRoundTrip(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	id, _, err := st.UpsertPosting(ctx, samplePosting())
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}
	const (
		inputHash = "goalhash0001"
		ver       = "aiver0000001"
	)
	now := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)

	t.Run("round-trips items and net", func(t *testing.T) {
		if err := st.UpsertAIScore(ctx, id, inputHash, ver, sampleDelta(6), now); err != nil {
			t.Fatalf("UpsertAIScore: %v", err)
		}
		got, ok, err := st.AIScore(ctx, id, inputHash, ver)
		if err != nil || !ok {
			t.Fatalf("AIScore: ok=%v err=%v", ok, err)
		}
		if got.NetDelta != 6 || len(got.Items) != 1 || got.Items[0].Evidence != "서버 개발자를 찾습니다" {
			t.Fatalf("round trip = %+v", got)
		}
		if got.Stale {
			t.Fatal("a fresh AIScore read must not be marked stale")
		}
	})

	t.Run("empty items round-trips as empty slice", func(t *testing.T) {
		if err := st.UpsertAIScore(ctx, id, "emptyhash001", ver, ai.Delta{NetDelta: 0}, now); err != nil {
			t.Fatalf("UpsertAIScore empty: %v", err)
		}
		got, ok, _ := st.AIScore(ctx, id, "emptyhash001", ver)
		if !ok || len(got.Items) != 0 || got.NetDelta != 0 {
			t.Fatalf("empty delta round trip = %+v", got)
		}
	})

	t.Run("PK conflict updates in place", func(t *testing.T) {
		if err := st.UpsertAIScore(ctx, id, inputHash, ver, sampleDelta(-3), now.Add(time.Hour)); err != nil {
			t.Fatalf("re-upsert: %v", err)
		}
		got, _, _ := st.AIScore(ctx, id, inputHash, ver)
		if got.NetDelta != -3 {
			t.Fatalf("conflict update net = %d, want -3", got.NetDelta)
		}
		var n int
		st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ai_scores WHERE posting_id=? AND ai_input_hash=? AND ai_version=?`, id, inputHash, ver).Scan(&n)
		if n != 1 {
			t.Fatalf("conflict produced %d rows, want 1 (upsert, not duplicate)", n)
		}
	})

	t.Run("miss returns ok=false", func(t *testing.T) {
		_, ok, err := st.AIScore(ctx, id, "no-such-hash", ver)
		if err != nil || ok {
			t.Fatalf("miss: ok=%v err=%v", ok, err)
		}
	})
}

func TestLatestAIScorePrefersNewestAcrossInputHashes(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	id, _, _ := st.UpsertPosting(ctx, samplePosting())
	const ver = "aiver0000002"
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	// Two deltas for the same posting under different goal-text hashes; the
	// newer one (different input hash) is what the stale fallback should return.
	if err := st.UpsertAIScore(ctx, id, "oldgoalhash1", ver, sampleDelta(4), t0); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertAIScore(ctx, id, "newgoalhash1", ver, sampleDelta(9), t0.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}

	got, ok, err := st.LatestAIScore(ctx, id, ver)
	if err != nil || !ok {
		t.Fatalf("LatestAIScore: ok=%v err=%v", ok, err)
	}
	if got.NetDelta != 9 {
		t.Fatalf("latest net = %d, want 9 (the newer computed_at)", got.NetDelta)
	}

	t.Run("a different ai_version does not leak", func(t *testing.T) {
		if err := st.UpsertAIScore(ctx, id, "newgoalhash1", "otherversion", sampleDelta(99), t0.Add(72*time.Hour)); err != nil {
			t.Fatal(err)
		}
		got, _, _ := st.LatestAIScore(ctx, id, ver)
		if got.NetDelta != 9 {
			t.Fatalf("latest net = %d, want 9 — the other-version row must not leak", got.NetDelta)
		}
	})
}

func TestAIScoresByPostingIDBatched(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const (
		inputHash = "freshhash001"
		ver       = "aiver0000003"
	)
	t0 := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)

	id1, _, _ := st.UpsertPosting(ctx, samplePosting())
	p2 := samplePosting()
	p2.SourcePostingID = "888"
	id2, _, _ := st.UpsertPosting(ctx, p2)

	if err := st.UpsertAIScore(ctx, id1, inputHash, ver, sampleDelta(6), t0); err != nil {
		t.Fatal(err)
	}
	// id2 has a row under a DIFFERENT input hash → must NOT appear in the fresh map.
	if err := st.UpsertAIScore(ctx, id2, "stalehash002", ver, sampleDelta(3), t0); err != nil {
		t.Fatal(err)
	}

	fresh, err := st.AIScoresByPostingID(ctx, inputHash, ver)
	if err != nil {
		t.Fatalf("AIScoresByPostingID: %v", err)
	}
	if len(fresh) != 1 {
		t.Fatalf("fresh map has %d entries, want 1 (only the matching input hash)", len(fresh))
	}
	if _, ok := fresh[id1]; !ok {
		t.Fatalf("fresh map missing id1")
	}
	if _, ok := fresh[id2]; ok {
		t.Fatalf("id2 (different input hash) must not be in the fresh map")
	}

	t.Run("latest batch returns newest per posting across hashes", func(t *testing.T) {
		// id1 gets a newer row under a different hash; the latest batch must pick it.
		if err := st.UpsertAIScore(ctx, id1, "newest_hash1", ver, sampleDelta(11), t0.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		latest, err := st.LatestAIScoresByPostingID(ctx, ver)
		if err != nil {
			t.Fatalf("LatestAIScoresByPostingID: %v", err)
		}
		if len(latest) != 2 {
			t.Fatalf("latest map has %d entries, want 2", len(latest))
		}
		if latest[id1].NetDelta != 11 {
			t.Fatalf("id1 latest net = %d, want 11 (newest computed_at)", latest[id1].NetDelta)
		}
		if latest[id2].NetDelta != 3 {
			t.Fatalf("id2 latest net = %d, want 3", latest[id2].NetDelta)
		}
	})
}

func TestAIScoreCascadeOnPostingDelete(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const ver = "cascadescore"
	id, _, _ := st.UpsertPosting(ctx, samplePosting())
	if err := st.UpsertAIScore(ctx, id, "h", ver, sampleDelta(5), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM postings WHERE id = ?`, id); err != nil {
		t.Fatalf("delete posting: %v", err)
	}
	var n int
	st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ai_scores WHERE posting_id = ?`, id).Scan(&n)
	if n != 0 {
		t.Fatalf("ai_scores row outlived its posting (%d rows) — ON DELETE CASCADE not engaged", n)
	}
}
