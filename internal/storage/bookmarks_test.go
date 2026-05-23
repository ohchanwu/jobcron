package storage

import (
	"context"
	"testing"
	"time"
)

func TestSetBookmarkInsertsRow(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	id, _, err := st.UpsertPosting(ctx, samplePosting())
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}

	at := time.Date(2026, 5, 23, 10, 0, 0, 0, time.UTC)
	if err := st.SetBookmark(ctx, id, at); err != nil {
		t.Fatalf("SetBookmark: %v", err)
	}

	got, err := st.IsBookmarked(ctx, id)
	if err != nil || !got {
		t.Errorf("IsBookmarked = %v, %v; want true, nil", got, err)
	}
}

func TestSetBookmarkIsIdempotent(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	id, _, err := st.UpsertPosting(ctx, samplePosting())
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}

	first := time.Date(2026, 5, 23, 10, 0, 0, 0, time.UTC)
	if err := st.SetBookmark(ctx, id, first); err != nil {
		t.Fatalf("first SetBookmark: %v", err)
	}
	// A second SetBookmark on the same posting must not overwrite the
	// original bookmarked_at — the user's mental model is "the moment I
	// saved it," not "the most recent click."
	later := first.Add(48 * time.Hour)
	if err := st.SetBookmark(ctx, id, later); err != nil {
		t.Fatalf("second SetBookmark: %v", err)
	}

	var ts time.Time
	if err := st.db.QueryRow(
		`SELECT bookmarked_at FROM bookmarks WHERE posting_id = ?`, id).Scan(&ts); err != nil {
		t.Fatalf("read bookmarked_at: %v", err)
	}
	if !ts.Equal(first) {
		t.Errorf("bookmarked_at = %v, want %v (must not advance on a repeat set)", ts, first)
	}
}

func TestClearBookmarkRemovesRow(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	id, _, err := st.UpsertPosting(ctx, samplePosting())
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}
	if err := st.SetBookmark(ctx, id, time.Now()); err != nil {
		t.Fatalf("SetBookmark: %v", err)
	}
	if err := st.ClearBookmark(ctx, id); err != nil {
		t.Fatalf("ClearBookmark: %v", err)
	}
	got, err := st.IsBookmarked(ctx, id)
	if err != nil {
		t.Fatalf("IsBookmarked: %v", err)
	}
	if got {
		t.Error("IsBookmarked = true after ClearBookmark")
	}
}

func TestClearBookmarkIsNoopWhenAbsent(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	id, _, err := st.UpsertPosting(ctx, samplePosting())
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}
	if err := st.ClearBookmark(ctx, id); err != nil {
		t.Errorf("ClearBookmark on never-bookmarked posting: %v", err)
	}
}

func TestBookmarkedIDsReturnsSet(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ids := upsertN(t, st, 3)

	for _, id := range ids[:2] { // bookmark only the first two
		if err := st.SetBookmark(ctx, id, time.Now()); err != nil {
			t.Fatalf("SetBookmark %d: %v", id, err)
		}
	}

	got, err := st.BookmarkedIDs(ctx)
	if err != nil {
		t.Fatalf("BookmarkedIDs: %v", err)
	}
	if len(got) != 2 || !got[ids[0]] || !got[ids[1]] || got[ids[2]] {
		t.Errorf("BookmarkedIDs = %v, want {%d, %d}", got, ids[0], ids[1])
	}
}

func TestBookmarkedPostingsOrderedByBookmarkedAtDesc(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ids := upsertN(t, st, 3)

	// Bookmark order: ids[1] first, then ids[0], then ids[2]; the listing
	// must come back in reverse order — most recently saved first.
	t0 := time.Date(2026, 5, 23, 10, 0, 0, 0, time.UTC)
	if err := st.SetBookmark(ctx, ids[1], t0); err != nil {
		t.Fatalf("SetBookmark[1]: %v", err)
	}
	if err := st.SetBookmark(ctx, ids[0], t0.Add(time.Hour)); err != nil {
		t.Fatalf("SetBookmark[0]: %v", err)
	}
	if err := st.SetBookmark(ctx, ids[2], t0.Add(2*time.Hour)); err != nil {
		t.Fatalf("SetBookmark[2]: %v", err)
	}

	postings, err := st.BookmarkedPostings(ctx)
	if err != nil {
		t.Fatalf("BookmarkedPostings: %v", err)
	}
	if len(postings) != 3 {
		t.Fatalf("len = %d, want 3", len(postings))
	}
	wantOrder := []int64{ids[2], ids[0], ids[1]}
	for i, want := range wantOrder {
		if postings[i].ID != want {
			t.Errorf("postings[%d].ID = %d, want %d", i, postings[i].ID, want)
		}
	}
}

func TestBookmarkCascadesOnPostingDelete(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	id, _, err := st.UpsertPosting(ctx, samplePosting())
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}
	if err := st.SetBookmark(ctx, id, time.Now()); err != nil {
		t.Fatalf("SetBookmark: %v", err)
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM postings WHERE id = ?`, id); err != nil {
		t.Fatalf("delete posting: %v", err)
	}
	got, err := st.IsBookmarked(ctx, id)
	if err != nil {
		t.Fatalf("IsBookmarked: %v", err)
	}
	if got {
		t.Error("bookmark row outlived its posting — FK cascade failed")
	}
}

// upsertN inserts n distinct postings and returns their row ids in
// insertion order.
func upsertN(t *testing.T, st *Store, n int) []int64 {
	t.Helper()
	ctx := context.Background()
	ids := make([]int64, n)
	for i := 0; i < n; i++ {
		p := samplePosting()
		p.SourcePostingID = "p" + time.Now().Format("150405.000") + "-" + string(rune('a'+i))
		p.URL = "https://example.test/" + p.SourcePostingID
		id, _, err := st.UpsertPosting(ctx, p)
		if err != nil {
			t.Fatalf("UpsertPosting #%d: %v", i, err)
		}
		ids[i] = id
	}
	return ids
}
