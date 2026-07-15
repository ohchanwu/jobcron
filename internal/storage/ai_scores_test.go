package storage

import (
	"context"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
)

func sampleDelta(net int) ai.Delta {
	return ai.Delta{
		Items: []ai.DeltaItem{
			{Signal: "백엔드", Kind: ai.KindPresence, Delta: net, Evidence: "서버 개발자를 찾습니다", MatchedGoal: "좋아하는 업무"},
		},
		NetDelta: net,
	}
}

const testAIUserID int64 = 1

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
		if err := st.UpsertAIScore(ctx, testAIUserID, id, inputHash, ver, sampleDelta(6), now); err != nil {
			t.Fatalf("UpsertAIScore: %v", err)
		}
		got, ok, err := st.AIScore(ctx, testAIUserID, id, inputHash, ver)
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
		if err := st.UpsertAIScore(ctx, testAIUserID, id, "emptyhash001", ver, ai.Delta{NetDelta: 0}, now); err != nil {
			t.Fatalf("UpsertAIScore empty: %v", err)
		}
		got, ok, _ := st.AIScore(ctx, testAIUserID, id, "emptyhash001", ver)
		if !ok || len(got.Items) != 0 || got.NetDelta != 0 {
			t.Fatalf("empty delta round trip = %+v", got)
		}
	})

	t.Run("PK conflict updates in place", func(t *testing.T) {
		if err := st.UpsertAIScore(ctx, testAIUserID, id, inputHash, ver, sampleDelta(-3), now.Add(time.Hour)); err != nil {
			t.Fatalf("re-upsert: %v", err)
		}
		got, _, _ := st.AIScore(ctx, testAIUserID, id, inputHash, ver)
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
		_, ok, err := st.AIScore(ctx, testAIUserID, id, "no-such-hash", ver)
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
	if err := st.UpsertAIScore(ctx, testAIUserID, id, "oldgoalhash1", ver, sampleDelta(4), t0); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertAIScore(ctx, testAIUserID, id, "newgoalhash1", ver, sampleDelta(9), t0.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}

	got, ok, err := st.LatestAIScore(ctx, testAIUserID, id, ver)
	if err != nil || !ok {
		t.Fatalf("LatestAIScore: ok=%v err=%v", ok, err)
	}
	if got.NetDelta != 9 {
		t.Fatalf("latest net = %d, want 9 (the newer computed_at)", got.NetDelta)
	}

	t.Run("a different ai_version does not leak", func(t *testing.T) {
		if err := st.UpsertAIScore(ctx, testAIUserID, id, "newgoalhash1", "otherversion", sampleDelta(99), t0.Add(72*time.Hour)); err != nil {
			t.Fatal(err)
		}
		got, _, _ := st.LatestAIScore(ctx, testAIUserID, id, ver)
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

	if err := st.UpsertAIScore(ctx, testAIUserID, id1, inputHash, ver, sampleDelta(6), t0); err != nil {
		t.Fatal(err)
	}
	// id2 has a row under a DIFFERENT input hash → must NOT appear in the fresh map.
	if err := st.UpsertAIScore(ctx, testAIUserID, id2, "stalehash002", ver, sampleDelta(3), t0); err != nil {
		t.Fatal(err)
	}

	fresh, err := st.AIScoresByPostingID(ctx, testAIUserID, inputHash, ver)
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
		if err := st.UpsertAIScore(ctx, testAIUserID, id1, "newest_hash1", ver, sampleDelta(11), t0.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		latest, err := st.LatestAIScoresByPostingID(ctx, testAIUserID, ver)
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

// TestLatestAIScoresAnyVersionCrossesProviderSwitch is the Bug 1 regression: when
// the user switches provider/model, ai_version rotates and the version-scoped
// lookups (AIScoresByPostingID / LatestAIScoresByPostingID) can no longer reach
// the prior rows. The cross-version fallback must still find a posting's latest
// delta so the chip persists faded instead of vanishing.
func TestLatestAIScoresAnyVersionCrossesProviderSwitch(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const (
		anthropicVer = "anthropicver"
		openaiVer    = "openai_ver01"
	)
	t0 := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)

	id1, _, _ := st.UpsertPosting(ctx, samplePosting())
	p2 := samplePosting()
	p2.SourcePostingID = "777"
	id2, _, _ := st.UpsertPosting(ctx, p2)

	// id1: a single delta rated under the OLD provider's ai_version.
	if err := st.UpsertAIScore(ctx, testAIUserID, id1, "goalhash0001", anthropicVer, sampleDelta(7), t0); err != nil {
		t.Fatal(err)
	}
	// id2: rated under the old version, then re-rated more recently under a NEW
	// version (e.g. switched provider and back) — newest computed_at should win.
	if err := st.UpsertAIScore(ctx, testAIUserID, id2, "goalhash0001", anthropicVer, sampleDelta(3), t0); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertAIScore(ctx, testAIUserID, id2, "goalhash0001", openaiVer, sampleDelta(8), t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	// The version-scoped lookup under a THIRD, current ai_version finds nothing —
	// this is exactly the vanish condition the fallback exists to rescue.
	scoped, err := st.LatestAIScoresByPostingID(ctx, testAIUserID, "currentver01")
	if err != nil {
		t.Fatalf("LatestAIScoresByPostingID: %v", err)
	}
	if len(scoped) != 0 {
		t.Fatalf("version-scoped lookup leaked %d rows for an unseen version", len(scoped))
	}

	anyVer, err := st.LatestAIScoresAnyVersionByPostingID(ctx, testAIUserID)
	if err != nil {
		t.Fatalf("LatestAIScoresAnyVersionByPostingID: %v", err)
	}
	if len(anyVer) != 2 {
		t.Fatalf("any-version map has %d entries, want 2", len(anyVer))
	}
	if anyVer[id1].NetDelta != 7 {
		t.Fatalf("id1 any-version net = %d, want 7 (the old-provider row, still reachable)", anyVer[id1].NetDelta)
	}
	if anyVer[id2].NetDelta != 8 {
		t.Fatalf("id2 any-version net = %d, want 8 (newest computed_at across versions)", anyVer[id2].NetDelta)
	}
}

// TestUpsertAIScorePrunePreservesCrossVersionStale is the T5 prune-on-write
// regression. UpsertAIScore prunes a posting's accumulated dead rows but must
// keep (a) every row under the just-written ai_version and (b) the single
// most-recent OTHER-version row — the load-bearing row the faded "이전 설정 기준"
// chip and a provider round-trip both depend on.
func TestUpsertAIScorePrunePreservesCrossVersionStale(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	id, _, _ := st.UpsertPosting(ctx, samplePosting())
	const (
		h  = "goalhash0001"
		vA = "ver_anthropic" // first provider
		vB = "ver_openai_01" // switched once
		vC = "ver_anthro_v2" // switched twice
	)
	t1 := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	t3 := t2.Add(time.Hour)

	rowCount := func() int {
		var n int
		st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ai_scores WHERE posting_id=?`, id).Scan(&n)
		return n
	}

	// Rate under vA, then switch to vB (rotate once). The vB write prunes other
	// versions but keeps the single most-recent — vA, the only other row.
	if err := st.UpsertAIScore(ctx, testAIUserID, id, h, vA, sampleDelta(-7), t1); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertAIScore(ctx, testAIUserID, id, h, vB, sampleDelta(5), t2); err != nil {
		t.Fatal(err)
	}
	// Provider round-trip: switching back to vA must still find its row FRESH —
	// the prune kept it, so the prior provider's score returns without re-rating.
	if got, ok, _ := st.AIScore(ctx, testAIUserID, id, h, vA); !ok || got.NetDelta != -7 {
		t.Fatalf("after one rotation, vA row should survive fresh: ok=%v net=%d (want -7)", ok, got.NetDelta)
	}
	if rowCount() != 2 {
		t.Fatalf("after one rotation: %d rows, want 2 (vA + vB)", rowCount())
	}

	// Rotate twice: the vC write prunes other versions to the single most-recent
	// (vB at t2), deleting the older vA row.
	if err := st.UpsertAIScore(ctx, testAIUserID, id, h, vC, sampleDelta(9), t3); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := st.AIScore(ctx, testAIUserID, id, h, vA); ok {
		t.Fatal("after two rotations, the oldest (vA) row should have been pruned")
	}
	if _, ok, _ := st.AIScore(ctx, testAIUserID, id, h, vB); !ok {
		t.Fatal("vB is the most-recent OTHER-version row — it must survive the prune")
	}
	if _, ok, _ := st.AIScore(ctx, testAIUserID, id, h, vC); !ok {
		t.Fatal("vC is the current version — it must survive")
	}
	if rowCount() != 2 {
		t.Fatalf("after two rotations: %d rows, want 2 (vB + vC)", rowCount())
	}

	// The cross-version stale chip still resolves: under a brand-new current
	// version (vD, never rated) the version-scoped lookup misses, but the
	// any-version fallback returns the newest surviving row (vC).
	if _, ok, _ := st.LatestAIScore(ctx, testAIUserID, id, "ver_unseen_01"); ok {
		t.Fatal("an unseen version must miss the version-scoped lookup")
	}
	anyVer, err := st.LatestAIScoresAnyVersionByPostingID(ctx, testAIUserID)
	if err != nil {
		t.Fatal(err)
	}
	if d, ok := anyVer[id]; !ok || d.NetDelta != 9 {
		t.Fatalf("cross-version stale chip must resolve to the newest surviving row: ok=%v net=%d (want 9)", ok, d.NetDelta)
	}

	// "Keep ALL current-version rows": a second goal hash under the current
	// version must coexist (the prune only ever targets OTHER versions).
	if err := st.UpsertAIScore(ctx, testAIUserID, id, "goalhash0002", vC, sampleDelta(2), t3.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := st.AIScore(ctx, testAIUserID, id, h, vC); !ok {
		t.Fatal("first current-version row must survive a same-version write")
	}
	if _, ok, _ := st.AIScore(ctx, testAIUserID, id, "goalhash0002", vC); !ok {
		t.Fatal("second current-version row must survive")
	}
	if rowCount() != 3 {
		t.Fatalf("expected 3 rows (vB + two vC), got %d", rowCount())
	}
}

func TestAIScoreCascadeOnPostingDelete(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const ver = "cascadescore"
	id, _, _ := st.UpsertPosting(ctx, samplePosting())
	if err := st.UpsertAIScore(ctx, testAIUserID, id, "h", ver, sampleDelta(5), time.Now()); err != nil {
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

func TestAIScoresAreIsolatedByUser(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	userA := insertAIStorageTestUser(t, st, "ai-score-a@example.invalid")
	userB := insertAIStorageTestUser(t, st, "ai-score-b@example.invalid")
	postingID := insertMigrationTestPosting(t, st, "score-isolation")
	computedAt := time.Date(2026, 7, 15, 6, 0, 0, 0, time.UTC)

	if err := st.UpsertAIScore(ctx, userA, postingID, "shared-hash", "shared-version", sampleDelta(8), computedAt); err != nil {
		t.Fatalf("UpsertAIScore user A: %v", err)
	}
	if err := st.UpsertAIScore(ctx, userB, postingID, "shared-hash", "shared-version", sampleDelta(-4), computedAt); err != nil {
		t.Fatalf("UpsertAIScore user B: %v", err)
	}

	for _, test := range []struct {
		name   string
		userID int64
		want   int
	}{
		{name: "user A", userID: userA, want: 8},
		{name: "user B", userID: userB, want: -4},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok, err := st.AIScore(ctx, test.userID, postingID, "shared-hash", "shared-version")
			if err != nil || !ok || got.NetDelta != test.want {
				t.Fatalf("AIScore = net %d ok=%v err=%v, want net %d", got.NetDelta, ok, err, test.want)
			}
			fresh, err := st.AIScoresByPostingID(ctx, test.userID, "shared-hash", "shared-version")
			if err != nil || len(fresh) != 1 || fresh[postingID].NetDelta != test.want {
				t.Fatalf("AIScoresByPostingID = %+v err=%v, want only net %d", fresh, err, test.want)
			}
			latest, err := st.LatestAIScoresByPostingID(ctx, test.userID, "shared-version")
			if err != nil || len(latest) != 1 || latest[postingID].NetDelta != test.want {
				t.Fatalf("LatestAIScoresByPostingID = %+v err=%v, want only net %d", latest, err, test.want)
			}
			anyVersion, err := st.LatestAIScoresAnyVersionByPostingID(ctx, test.userID)
			if err != nil || len(anyVersion) != 1 || anyVersion[postingID].NetDelta != test.want {
				t.Fatalf("LatestAIScoresAnyVersionByPostingID = %+v err=%v, want only net %d", anyVersion, err, test.want)
			}
		})
	}
}

func TestAIScorePruningDoesNotDeleteAnotherUsersRows(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	userA := insertAIStorageTestUser(t, st, "ai-prune-a@example.invalid")
	userB := insertAIStorageTestUser(t, st, "ai-prune-b@example.invalid")
	postingID := insertMigrationTestPosting(t, st, "score-pruning")
	t0 := time.Date(2026, 7, 15, 7, 0, 0, 0, time.UTC)

	for index, version := range []string{"user-b-v1", "user-b-v2", "user-b-v3"} {
		if _, err := st.db.ExecContext(ctx, `
INSERT INTO ai_scores
    (user_id, posting_id, ai_input_hash, ai_version, items_json, net_delta, computed_at)
VALUES ($1, $2, 'shared-hash', $3, '[]', $4, $5)`,
			userB, postingID, version, index+1, t0.Add(time.Duration(index)*time.Minute)); err != nil {
			t.Fatalf("seed user B score %s: %v", version, err)
		}
	}

	for index, version := range []string{"user-a-v1", "user-a-v2", "user-a-v3"} {
		if err := st.UpsertAIScore(ctx, userA, postingID, "shared-hash", version, sampleDelta(index+10), t0.Add(time.Duration(index)*time.Hour)); err != nil {
			t.Fatalf("UpsertAIScore user A %s: %v", version, err)
		}
	}

	var userBCount int
	if err := st.db.QueryRowContext(ctx, `
SELECT count(*) FROM ai_scores WHERE user_id = $1 AND posting_id = $2`, userB, postingID).Scan(&userBCount); err != nil {
		t.Fatalf("count user B scores: %v", err)
	}
	if userBCount != 3 {
		t.Fatalf("user B score count after user A pruning = %d, want 3", userBCount)
	}
	for index, version := range []string{"user-b-v1", "user-b-v2", "user-b-v3"} {
		got, ok, err := st.AIScore(ctx, userB, postingID, "shared-hash", version)
		if err != nil || !ok || got.NetDelta != index+1 {
			t.Fatalf("user B %s after user A pruning = net %d ok=%v err=%v, want %d", version, got.NetDelta, ok, err, index+1)
		}
	}
}

func TestAIStorageRejectsNonPositiveUserID(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	when := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)

	if err := st.UpsertAIScore(ctx, 0, 1, "hash", "version", sampleDelta(1), when); err == nil {
		t.Fatal("UpsertAIScore accepted userID 0")
	}
	if _, _, err := st.AIScore(ctx, -1, 1, "hash", "version"); err == nil {
		t.Fatal("AIScore accepted a negative userID")
	}
	if _, _, err := st.LatestAIScore(ctx, 0, 1, "version"); err == nil {
		t.Fatal("LatestAIScore accepted userID 0")
	}
	if _, err := st.AIScoresByPostingID(ctx, 0, "hash", "version"); err == nil {
		t.Fatal("AIScoresByPostingID accepted userID 0")
	}
	if _, err := st.LatestAIScoresByPostingID(ctx, 0, "version"); err == nil {
		t.Fatal("LatestAIScoresByPostingID accepted userID 0")
	}
	if _, err := st.LatestAIScoresAnyVersionByPostingID(ctx, 0); err == nil {
		t.Fatal("LatestAIScoresAnyVersionByPostingID accepted userID 0")
	}
}

func insertAIStorageTestUser(t *testing.T, st *Store, email string) int64 {
	t.Helper()
	var userID int64
	if err := st.db.QueryRow(`
INSERT INTO users (email, password_hash, created_at, updated_at)
VALUES ($1, 'synthetic-password-hash', now(), now())
RETURNING id`, email).Scan(&userID); err != nil {
		t.Fatalf("insert AI storage test user: %v", err)
	}
	return userID
}
