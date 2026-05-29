package storage

import (
	"context"
	"testing"
	"time"
)

const (
	testStaleWindow = 3 * 24 * time.Hour
	testOldWindow   = 90 * 24 * time.Hour
)

func TestSweepRemovesStalePostingsRelativeToMax(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// Fresh posting — was last seen "now"; defines the staleness baseline.
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	fresh := samplePosting()
	fresh.SourcePostingID = "fresh"
	fresh.FirstSeenAt = now.Add(-time.Hour)
	fresh.LastSeenAt = now
	freshID, _, err := st.UpsertPosting(ctx, fresh)
	if err != nil {
		t.Fatalf("UpsertPosting fresh: %v", err)
	}

	// Stale posting — last seen 4 days before the freshest one. Beyond
	// the 3-day window.
	stale := samplePosting()
	stale.SourcePostingID = "stale"
	stale.FirstSeenAt = now.Add(-10 * 24 * time.Hour)
	stale.LastSeenAt = now.Add(-4 * 24 * time.Hour)
	staleID, _, err := st.UpsertPosting(ctx, stale)
	if err != nil {
		t.Fatalf("UpsertPosting stale: %v", err)
	}

	removed, err := st.SweepStalePostings(ctx, now, testStaleWindow, testOldWindow, nil)
	if err != nil {
		t.Fatalf("SweepStalePostings: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
	if _, ok, _ := st.PostingByID(ctx, freshID); !ok {
		t.Error("fresh posting was removed; should have survived")
	}
	if _, ok, _ := st.PostingByID(ctx, staleID); ok {
		t.Error("stale posting still present; should have been removed")
	}
}

// TestSweepThreeDayBoundary pins the exact stale cut: a posting last seen
// just under 3 days before the freshest one survives; one last seen just over
// 3 days is swept. The cut is `last_seen_at < MAX(last_seen_at) - 3d`.
func TestSweepThreeDayBoundary(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)

	fresh := samplePosting()
	fresh.SourcePostingID = "fresh"
	fresh.FirstSeenAt = now.Add(-time.Hour)
	fresh.LastSeenAt = now // defines the baseline
	if _, _, err := st.UpsertPosting(ctx, fresh); err != nil {
		t.Fatalf("UpsertPosting fresh: %v", err)
	}

	underID := upsertSweepProbe(t, st, "under", now.Add(-(testStaleWindow - time.Hour))) // 2d23h → survives
	overID := upsertSweepProbe(t, st, "over", now.Add(-(testStaleWindow + time.Hour)))   // 3d1h  → swept

	removed, err := st.SweepStalePostings(ctx, now, testStaleWindow, testOldWindow, nil)
	if err != nil {
		t.Fatalf("SweepStalePostings: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1 (only the just-over-3-day posting)", removed)
	}
	if _, ok, _ := st.PostingByID(ctx, underID); !ok {
		t.Error("posting last seen 2d23h ago was swept; just-under-3-days must survive")
	}
	if _, ok, _ := st.PostingByID(ctx, overID); ok {
		t.Error("posting last seen 3d1h ago survived; just-over-3-days must be swept")
	}
}

// upsertSweepProbe inserts a posting last seen at lastSeen (first seen well
// before the old-window so only the stale rule is in play) and returns its id.
func upsertSweepProbe(t *testing.T, st *Store, id string, lastSeen time.Time) int64 {
	t.Helper()
	p := samplePosting()
	p.SourcePostingID = id
	p.FirstSeenAt = lastSeen.Add(-24 * time.Hour)
	p.LastSeenAt = lastSeen
	rowID, _, err := st.UpsertPosting(context.Background(), p)
	if err != nil {
		t.Fatalf("UpsertPosting %s: %v", id, err)
	}
	return rowID
}

func TestSweepRemovesPostingsOlderThanOldWindow(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)

	// Recent posting — well under 90 days. Survives.
	recent := samplePosting()
	recent.SourcePostingID = "recent"
	recent.FirstSeenAt = now.Add(-10 * 24 * time.Hour)
	recent.LastSeenAt = now
	recentID, _, err := st.UpsertPosting(ctx, recent)
	if err != nil {
		t.Fatalf("UpsertPosting recent: %v", err)
	}

	// 100-day-old posting still actively re-scraped (last_seen_at == now,
	// so it's not stale). The 3-month rule still removes it.
	old := samplePosting()
	old.SourcePostingID = "old"
	old.FirstSeenAt = now.Add(-100 * 24 * time.Hour)
	old.LastSeenAt = now
	old.AlwaysOpen = false
	oldID, _, err := st.UpsertPosting(ctx, old)
	if err != nil {
		t.Fatalf("UpsertPosting old: %v", err)
	}

	removed, err := st.SweepStalePostings(ctx, now, testStaleWindow, testOldWindow, nil)
	if err != nil {
		t.Fatalf("SweepStalePostings: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1 (the 100-day-old posting)", removed)
	}
	if _, ok, _ := st.PostingByID(ctx, recentID); !ok {
		t.Error("recent posting was removed; should have survived")
	}
	if _, ok, _ := st.PostingByID(ctx, oldID); ok {
		t.Error("old posting still present; should have been removed")
	}
}

func TestSweepExemptsAlwaysOpenFromOldRule(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)

	p := samplePosting()
	p.SourcePostingID = "always-open"
	p.FirstSeenAt = now.Add(-200 * 24 * time.Hour) // very old
	p.LastSeenAt = now                             // still actively scraped
	p.AlwaysOpen = true
	p.ClosedAt = nil
	id, _, err := st.UpsertPosting(ctx, p)
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}

	removed, err := st.SweepStalePostings(ctx, now, testStaleWindow, testOldWindow, nil)
	if err != nil {
		t.Fatalf("SweepStalePostings: %v", err)
	}
	if removed != 0 {
		t.Errorf("removed = %d, want 0 — always_open should ignore the 3-month rule", removed)
	}
	if _, ok, _ := st.PostingByID(ctx, id); !ok {
		t.Error("always_open posting was removed; should have survived the old-rule")
	}
}

func TestSweepRemovesAlwaysOpenWhenStale(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)

	// Fresh anchor so the staleness baseline is "now".
	fresh := samplePosting()
	fresh.SourcePostingID = "fresh-anchor"
	fresh.FirstSeenAt = now
	fresh.LastSeenAt = now
	if _, _, err := st.UpsertPosting(ctx, fresh); err != nil {
		t.Fatalf("UpsertPosting fresh: %v", err)
	}

	// always_open posting that the source dropped 5 days ago. The
	// always_open flag exempts it from the 3-month rule but NOT from the
	// stale rule — if the source no longer lists it, it is gone.
	stale := samplePosting()
	stale.SourcePostingID = "always-open-stale"
	stale.FirstSeenAt = now.Add(-30 * 24 * time.Hour)
	stale.LastSeenAt = now.Add(-5 * 24 * time.Hour)
	stale.AlwaysOpen = true
	stale.ClosedAt = nil
	staleID, _, err := st.UpsertPosting(ctx, stale)
	if err != nil {
		t.Fatalf("UpsertPosting stale: %v", err)
	}

	removed, err := st.SweepStalePostings(ctx, now, testStaleWindow, testOldWindow, nil)
	if err != nil {
		t.Fatalf("SweepStalePostings: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
	if _, ok, _ := st.PostingByID(ctx, staleID); ok {
		t.Error("source-removed always_open posting still present")
	}
}

func TestSweepExemptsBookmarkedFromBothRules(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)

	// Fresh anchor.
	fresh := samplePosting()
	fresh.SourcePostingID = "fresh"
	fresh.FirstSeenAt = now
	fresh.LastSeenAt = now
	if _, _, err := st.UpsertPosting(ctx, fresh); err != nil {
		t.Fatalf("UpsertPosting fresh: %v", err)
	}

	// One stale-and-bookmarked.
	staleBmarked := samplePosting()
	staleBmarked.SourcePostingID = "stale-bmarked"
	staleBmarked.FirstSeenAt = now.Add(-30 * 24 * time.Hour)
	staleBmarked.LastSeenAt = now.Add(-10 * 24 * time.Hour)
	sbID, _, err := st.UpsertPosting(ctx, staleBmarked)
	if err != nil {
		t.Fatalf("UpsertPosting stale-bmarked: %v", err)
	}
	if err := st.SetBookmark(ctx, sbID, now); err != nil {
		t.Fatalf("SetBookmark stale-bmarked: %v", err)
	}

	// One old-and-bookmarked.
	oldBmarked := samplePosting()
	oldBmarked.SourcePostingID = "old-bmarked"
	oldBmarked.FirstSeenAt = now.Add(-200 * 24 * time.Hour)
	oldBmarked.LastSeenAt = now
	oldBmarked.AlwaysOpen = false
	obID, _, err := st.UpsertPosting(ctx, oldBmarked)
	if err != nil {
		t.Fatalf("UpsertPosting old-bmarked: %v", err)
	}
	if err := st.SetBookmark(ctx, obID, now); err != nil {
		t.Fatalf("SetBookmark old-bmarked: %v", err)
	}

	removed, err := st.SweepStalePostings(ctx, now, testStaleWindow, testOldWindow, nil)
	if err != nil {
		t.Fatalf("SweepStalePostings: %v", err)
	}
	if removed != 0 {
		t.Errorf("removed = %d, want 0 — bookmarks must exempt both rules", removed)
	}
	for _, id := range []int64{sbID, obID} {
		if _, ok, _ := st.PostingByID(ctx, id); !ok {
			t.Errorf("bookmarked posting %d was removed", id)
		}
	}
}

func TestSweepIsNoopOnEmptyTable(t *testing.T) {
	st := newTestStore(t)
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	removed, err := st.SweepStalePostings(context.Background(), now, testStaleWindow, testOldWindow, nil)
	if err != nil {
		t.Fatalf("SweepStalePostings: %v", err)
	}
	if removed != 0 {
		t.Errorf("removed = %d, want 0 on an empty table", removed)
	}
}

func TestSweepCascadesScores(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)

	// Anchor + a stale posting with an attached score.
	fresh := samplePosting()
	fresh.SourcePostingID = "fresh"
	fresh.FirstSeenAt = now
	fresh.LastSeenAt = now
	if _, _, err := st.UpsertPosting(ctx, fresh); err != nil {
		t.Fatalf("UpsertPosting fresh: %v", err)
	}

	stale := samplePosting()
	stale.SourcePostingID = "stale-with-score"
	stale.FirstSeenAt = now.Add(-30 * 24 * time.Hour)
	stale.LastSeenAt = now.Add(-10 * 24 * time.Hour)
	id, _, err := st.UpsertPosting(ctx, stale)
	if err != nil {
		t.Fatalf("UpsertPosting stale: %v", err)
	}
	if err := st.UpsertScore(ctx, Score{
		PostingID: id, ProfileHash: "abc", Total: 60,
		BreakdownJSON: `[]`, ComputedAt: now,
	}); err != nil {
		t.Fatalf("UpsertScore: %v", err)
	}

	if _, err := st.SweepStalePostings(ctx, now, testStaleWindow, testOldWindow, nil); err != nil {
		t.Fatalf("SweepStalePostings: %v", err)
	}

	var scoreCount int
	if err := st.db.QueryRow(`SELECT count(*) FROM scores WHERE posting_id = ?`, id).Scan(&scoreCount); err != nil {
		t.Fatalf("count scores: %v", err)
	}
	if scoreCount != 0 {
		t.Errorf("score row survived posting deletion; ON DELETE CASCADE not engaged")
	}
}

func TestSweepUsesPerSourceBaseline(t *testing.T) {
	// Two sources scraped at very different cadences: one source's freshness
	// must not stale-out the other source's postings. The per-source MAX
	// baseline isolates each source's clock.
	st := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)

	jumpitFresh := samplePosting()
	jumpitFresh.Source = "jumpit"
	jumpitFresh.SourcePostingID = "j-fresh"
	jumpitFresh.FirstSeenAt = now.Add(-time.Hour)
	jumpitFresh.LastSeenAt = now
	if _, _, err := st.UpsertPosting(ctx, jumpitFresh); err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}

	// Worknet posting last seen 10 days ago. If staleness used the GLOBAL
	// MAX (=jumpit's "now"), this would be deleted as 10 days stale. With
	// per-source baselines, the worknet baseline IS this posting, so the
	// stale-window relative to itself is zero and it survives.
	worknetOld := samplePosting()
	worknetOld.Source = "worknet"
	worknetOld.SourcePostingID = "w-old"
	worknetOld.FirstSeenAt = now.Add(-30 * 24 * time.Hour)
	worknetOld.LastSeenAt = now.Add(-10 * 24 * time.Hour)
	wnID, _, err := st.UpsertPosting(ctx, worknetOld)
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}

	if _, err := st.SweepStalePostings(ctx, now, testStaleWindow, testOldWindow, nil); err != nil {
		t.Fatalf("SweepStalePostings: %v", err)
	}
	if _, ok, _ := st.PostingByID(ctx, wnID); !ok {
		t.Error("worknet posting was swept; per-source baseline should have spared it")
	}
}

func TestSweepSkipsDisabledSources(t *testing.T) {
	// activeSources scopes the sweep — postings from a source not in the
	// active set are frozen in place so re-enabling does not require a
	// fresh scrape.
	st := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)

	// Fresh jumpit anchor so jumpit's per-source baseline is "now".
	jumpitFresh := samplePosting()
	jumpitFresh.Source = "jumpit"
	jumpitFresh.SourcePostingID = "j-fresh"
	jumpitFresh.FirstSeenAt = now
	jumpitFresh.LastSeenAt = now
	if _, _, err := st.UpsertPosting(ctx, jumpitFresh); err != nil {
		t.Fatalf("UpsertPosting jumpit: %v", err)
	}

	// Worknet anchor at the same "now" plus a stale worknet posting 10
	// days old. Without the active-set filter the stale one would be
	// removed; with worknet excluded, it survives.
	worknetAnchor := samplePosting()
	worknetAnchor.Source = "worknet"
	worknetAnchor.SourcePostingID = "w-anchor"
	worknetAnchor.FirstSeenAt = now
	worknetAnchor.LastSeenAt = now
	if _, _, err := st.UpsertPosting(ctx, worknetAnchor); err != nil {
		t.Fatalf("UpsertPosting worknet anchor: %v", err)
	}
	worknetStale := samplePosting()
	worknetStale.Source = "worknet"
	worknetStale.SourcePostingID = "w-stale"
	worknetStale.FirstSeenAt = now.Add(-30 * 24 * time.Hour)
	worknetStale.LastSeenAt = now.Add(-10 * 24 * time.Hour)
	wsID, _, err := st.UpsertPosting(ctx, worknetStale)
	if err != nil {
		t.Fatalf("UpsertPosting worknet stale: %v", err)
	}

	// Active = jumpit only — worknet must be skipped entirely.
	if _, err := st.SweepStalePostings(ctx, now, testStaleWindow, testOldWindow, []string{"jumpit"}); err != nil {
		t.Fatalf("SweepStalePostings: %v", err)
	}
	if _, ok, _ := st.PostingByID(ctx, wsID); !ok {
		t.Error("stale worknet posting was swept; disabled source should be frozen")
	}
}
