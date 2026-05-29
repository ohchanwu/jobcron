package storage

import (
	"context"
	"testing"
	"time"
)

func TestSetNotInterestedInsertsRow(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	id, _, err := st.UpsertPosting(ctx, samplePosting())
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}

	at := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	if err := st.SetNotInterested(ctx, id, at); err != nil {
		t.Fatalf("SetNotInterested: %v", err)
	}

	got, err := st.IsNotInterested(ctx, id)
	if err != nil || !got {
		t.Errorf("IsNotInterested = %v, %v; want true, nil", got, err)
	}
}

func TestSetNotInterestedIsIdempotent(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	id, _, err := st.UpsertPosting(ctx, samplePosting())
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}

	first := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	if err := st.SetNotInterested(ctx, id, first); err != nil {
		t.Fatalf("first SetNotInterested: %v", err)
	}
	// A repeat mute must not advance muted_at — the timestamp records the
	// first time the user said "관심 없음", which is what the profile list
	// orders by.
	later := first.Add(48 * time.Hour)
	if err := st.SetNotInterested(ctx, id, later); err != nil {
		t.Fatalf("second SetNotInterested: %v", err)
	}

	var ts time.Time
	if err := st.db.QueryRow(
		`SELECT muted_at FROM not_interested WHERE posting_id = ?`, id).Scan(&ts); err != nil {
		t.Fatalf("read muted_at: %v", err)
	}
	if !ts.Equal(first) {
		t.Errorf("muted_at = %v, want %v (must not advance on a repeat set)", ts, first)
	}
}

func TestClearNotInterestedRemovesRow(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	id, _, err := st.UpsertPosting(ctx, samplePosting())
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}
	if err := st.SetNotInterested(ctx, id, time.Now()); err != nil {
		t.Fatalf("SetNotInterested: %v", err)
	}
	if err := st.ClearNotInterested(ctx, id); err != nil {
		t.Fatalf("ClearNotInterested: %v", err)
	}
	got, err := st.IsNotInterested(ctx, id)
	if err != nil {
		t.Fatalf("IsNotInterested: %v", err)
	}
	if got {
		t.Error("IsNotInterested = true after ClearNotInterested")
	}
}

func TestClearNotInterestedIsNoopWhenAbsent(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	id, _, err := st.UpsertPosting(ctx, samplePosting())
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}
	if err := st.ClearNotInterested(ctx, id); err != nil {
		t.Errorf("ClearNotInterested on never-muted posting: %v", err)
	}
}

func TestNotInterestedIDsReturnsSet(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ids := upsertN(t, st, 3)

	for _, id := range ids[:2] { // mute only the first two
		if err := st.SetNotInterested(ctx, id, time.Now()); err != nil {
			t.Fatalf("SetNotInterested %d: %v", id, err)
		}
	}

	got, err := st.NotInterestedIDs(ctx)
	if err != nil {
		t.Fatalf("NotInterestedIDs: %v", err)
	}
	if len(got) != 2 || !got[ids[0]] || !got[ids[1]] || got[ids[2]] {
		t.Errorf("NotInterestedIDs = %v, want {%d, %d}", got, ids[0], ids[1])
	}
}

func TestNotInterestedPostingsOrderedByMutedAtDesc(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ids := upsertN(t, st, 3)

	// Mute order: ids[1] first, then ids[0], then ids[2]; the listing must
	// come back in reverse order — most recently muted first.
	t0 := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	if err := st.SetNotInterested(ctx, ids[1], t0); err != nil {
		t.Fatalf("SetNotInterested[1]: %v", err)
	}
	if err := st.SetNotInterested(ctx, ids[0], t0.Add(time.Hour)); err != nil {
		t.Fatalf("SetNotInterested[0]: %v", err)
	}
	if err := st.SetNotInterested(ctx, ids[2], t0.Add(2*time.Hour)); err != nil {
		t.Fatalf("SetNotInterested[2]: %v", err)
	}

	postings, err := st.NotInterestedPostings(ctx)
	if err != nil {
		t.Fatalf("NotInterestedPostings: %v", err)
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

func TestNotInterestedCascadesOnPostingDelete(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	id, _, err := st.UpsertPosting(ctx, samplePosting())
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}
	if err := st.SetNotInterested(ctx, id, time.Now()); err != nil {
		t.Fatalf("SetNotInterested: %v", err)
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM postings WHERE id = ?`, id); err != nil {
		t.Fatalf("delete posting: %v", err)
	}
	got, err := st.IsNotInterested(ctx, id)
	if err != nil {
		t.Fatalf("IsNotInterested: %v", err)
	}
	if got {
		t.Error("not_interested row outlived its posting — FK cascade failed")
	}
}
