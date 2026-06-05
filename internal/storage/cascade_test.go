package storage

import (
	"context"
	"testing"
	"time"
)

// TestDeletePostingCascadesScore proves the scores ON DELETE CASCADE actually
// fires — i.e. foreign keys are enforced. If this ever fails, swept postings
// leave orphan score rows behind (the 2026-06-05 audit found 288 such orphans;
// migration 0009 purges the historical residue, but this test guards that the
// cause is not still live).
func TestDeletePostingCascadesScore(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	id, _, err := st.UpsertPosting(ctx, samplePosting())
	if err != nil {
		t.Fatalf("upsert posting: %v", err)
	}
	if err := st.UpsertScore(ctx, Score{
		PostingID:     id,
		ProfileHash:   "hash",
		Total:         42,
		BreakdownJSON: "{}",
		ComputedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert score: %v", err)
	}
	if _, err := st.db.ExecContext(ctx, "DELETE FROM postings WHERE id = ?", id); err != nil {
		t.Fatalf("delete posting: %v", err)
	}
	var n int
	if err := st.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM scores WHERE posting_id = ?", id).Scan(&n); err != nil {
		t.Fatalf("count scores: %v", err)
	}
	if n != 0 {
		t.Fatalf("FK cascade broken: score row survived posting delete; want 0 orphans, got %d", n)
	}
}
