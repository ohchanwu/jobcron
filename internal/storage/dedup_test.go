package storage

import (
	"context"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/scraper"
)

// dedupPosting builds a sample posting with the given source and id, so a
// test can seed two postings that look almost-identical except for source.
func dedupPosting(source, sourcePostingID, title string, seen time.Time) scraper.Posting {
	return scraper.Posting{
		Source:          source,
		SourcePostingID: sourcePostingID,
		URL:             "https://example.com/" + source + "/" + sourcePostingID,
		Title:           title,
		Company:         "토스",
		Location:        "서울 강남구",
		Newcomer:        true,
		StackTags:       []string{},
		Tags:            []scraper.Tag{},
		Description:     "",
		RawJSON:         `{}`,
		FirstSeenAt:     seen,
		LastSeenAt:      seen,
	}
}

func TestMarkAndClearDuplicate(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	t0 := time.Date(2026, 5, 26, 8, 0, 0, 0, time.UTC)

	canonicalID, _, err := st.UpsertPosting(ctx, dedupPosting("jumpit", "100", "Backend Engineer", t0))
	if err != nil {
		t.Fatalf("upsert canonical: %v", err)
	}
	dupID, _, err := st.UpsertPosting(ctx, dedupPosting("rallit", "200", "Backend Engineer", t0.Add(time.Hour)))
	if err != nil {
		t.Fatalf("upsert dup: %v", err)
	}

	// Fresh inserts: both should be canonical.
	p, _, _ := st.PostingByID(ctx, dupID)
	if p.DuplicateOf != nil {
		t.Fatalf("fresh insert has DuplicateOf = %v, want nil", *p.DuplicateOf)
	}

	if err := st.MarkDuplicate(ctx, dupID, canonicalID); err != nil {
		t.Fatalf("MarkDuplicate: %v", err)
	}

	// After marking: dup points at canonical.
	p, _, _ = st.PostingByID(ctx, dupID)
	if p.DuplicateOf == nil || *p.DuplicateOf != canonicalID {
		t.Errorf("after MarkDuplicate, DuplicateOf = %v, want %d", p.DuplicateOf, canonicalID)
	}
	// Canonical stays canonical.
	p, _, _ = st.PostingByID(ctx, canonicalID)
	if p.DuplicateOf != nil {
		t.Errorf("canonical has DuplicateOf = %v, want nil", *p.DuplicateOf)
	}

	// ClearAllDuplicates undoes the mark.
	if err := st.ClearAllDuplicates(ctx); err != nil {
		t.Fatalf("ClearAllDuplicates: %v", err)
	}
	p, _, _ = st.PostingByID(ctx, dupID)
	if p.DuplicateOf != nil {
		t.Errorf("after ClearAllDuplicates, DuplicateOf = %v, want nil", *p.DuplicateOf)
	}
}

func TestMarkDuplicateRejectsSelfAndMissing(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	t0 := time.Date(2026, 5, 26, 8, 0, 0, 0, time.UTC)

	id, _, err := st.UpsertPosting(ctx, dedupPosting("jumpit", "100", "Backend Engineer", t0))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := st.MarkDuplicate(ctx, id, id); err == nil {
		t.Error("MarkDuplicate(id, id) returned nil, want self-reference error")
	}
	if err := st.MarkDuplicate(ctx, 9999, id); err == nil {
		t.Error("MarkDuplicate of missing id returned nil, want not-found error")
	}
}

func TestCanonicalPostingsFiltersDuplicates(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	t0 := time.Date(2026, 5, 26, 8, 0, 0, 0, time.UTC)

	canonicalID, _, _ := st.UpsertPosting(ctx, dedupPosting("jumpit", "100", "Backend Engineer", t0))
	dupID, _, _ := st.UpsertPosting(ctx, dedupPosting("rallit", "200", "Backend Engineer", t0.Add(time.Hour)))
	soloID, _, _ := st.UpsertPosting(ctx, dedupPosting("jumpit", "300", "Frontend Engineer", t0.Add(2*time.Hour)))

	if err := st.MarkDuplicate(ctx, dupID, canonicalID); err != nil {
		t.Fatalf("MarkDuplicate: %v", err)
	}

	all, err := st.AllPostings(ctx)
	if err != nil {
		t.Fatalf("AllPostings: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("AllPostings returned %d, want 3", len(all))
	}

	canon, err := st.CanonicalPostings(ctx)
	if err != nil {
		t.Fatalf("CanonicalPostings: %v", err)
	}
	if len(canon) != 2 {
		t.Fatalf("CanonicalPostings returned %d, want 2", len(canon))
	}
	got := map[int64]bool{canon[0].ID: true, canon[1].ID: true}
	if !got[canonicalID] || !got[soloID] {
		t.Errorf("CanonicalPostings returned ids %v, want {%d, %d}", got, canonicalID, soloID)
	}
	if got[dupID] {
		t.Errorf("CanonicalPostings included duplicate id %d", dupID)
	}
}

func TestDuplicatesOfReturnsSiblings(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	t0 := time.Date(2026, 5, 26, 8, 0, 0, 0, time.UTC)

	canonicalID, _, _ := st.UpsertPosting(ctx, dedupPosting("jumpit", "100", "Backend Engineer", t0))
	dupA, _, _ := st.UpsertPosting(ctx, dedupPosting("rallit", "200", "Backend Engineer", t0.Add(time.Hour)))
	dupB, _, _ := st.UpsertPosting(ctx, dedupPosting("naver", "300", "Backend Engineer", t0.Add(2*time.Hour)))

	if err := st.MarkDuplicate(ctx, dupA, canonicalID); err != nil {
		t.Fatalf("MarkDuplicate A: %v", err)
	}
	if err := st.MarkDuplicate(ctx, dupB, canonicalID); err != nil {
		t.Fatalf("MarkDuplicate B: %v", err)
	}

	dupes, err := st.DuplicatesOf(ctx, canonicalID)
	if err != nil {
		t.Fatalf("DuplicatesOf: %v", err)
	}
	if len(dupes) != 2 {
		t.Fatalf("DuplicatesOf returned %d, want 2", len(dupes))
	}
	got := map[string]bool{dupes[0].Source: true, dupes[1].Source: true}
	if !got["rallit"] || !got["naver"] {
		t.Errorf("DuplicatesOf sources = %v, want {rallit, naver}", got)
	}

	// Canonical with no duplicates returns empty (no error).
	soloID, _, _ := st.UpsertPosting(ctx, dedupPosting("jumpit", "400", "Frontend Engineer", t0.Add(3*time.Hour)))
	dupes, err = st.DuplicatesOf(ctx, soloID)
	if err != nil {
		t.Fatalf("DuplicatesOf solo: %v", err)
	}
	if len(dupes) != 0 {
		t.Errorf("DuplicatesOf solo returned %d, want 0", len(dupes))
	}
}

// TestSweepCanonicalCascadesDuplicateOfToNull confirms the ON DELETE SET
// NULL behavior on duplicate_of: when the canonical row is deleted (e.g.
// by sweep), its former duplicates lose their duplicate_of pointer and
// re-enter the canonical list rather than disappearing alongside it.
// TestSweepCanonicalCascadesDuplicateOfToNull confirms ON DELETE SET
// NULL: when the canonical row is swept (it became stale in its own
// source), its former duplicates lose their duplicate_of pointer and
// re-enter the canonical list rather than vanishing.
func TestSweepCanonicalCascadesDuplicateOfToNull(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	t0 := time.Date(2026, 5, 26, 8, 0, 0, 0, time.UTC)

	// Seed:
	//   jumpit canonical at t0 (about to go stale)
	//   jumpit baseline-bumper at t0+10d (advances jumpit's max last_seen)
	//   rallit duplicate at t0+10d (rallit baseline matches, so dup is fresh)
	canonicalID, _, _ := st.UpsertPosting(ctx, dedupPosting("jumpit", "100", "Backend Engineer", t0))
	st.UpsertPosting(ctx, dedupPosting("jumpit", "999", "Stale-baseline-bump", t0.Add(10*24*time.Hour)))
	dupID, _, _ := st.UpsertPosting(ctx, dedupPosting("rallit", "200", "Backend Engineer", t0.Add(10*24*time.Hour)))

	if err := st.MarkDuplicate(ctx, dupID, canonicalID); err != nil {
		t.Fatalf("MarkDuplicate: %v", err)
	}

	staleWindow := 3 * 24 * time.Hour
	oldWindow := 365 * 24 * time.Hour
	now := t0.Add(11 * 24 * time.Hour)
	if _, err := st.SweepStalePostings(ctx, now, staleWindow, oldWindow, nil); err != nil {
		t.Fatalf("SweepStalePostings: %v", err)
	}

	// Canonical should be gone.
	if _, ok, _ := st.PostingByID(ctx, canonicalID); ok {
		t.Error("canonical was not swept, want gone")
	}
	// Duplicate should still exist with duplicate_of cleared.
	p, ok, err := st.PostingByID(ctx, dupID)
	if err != nil {
		t.Fatalf("PostingByID dup: %v", err)
	}
	if !ok {
		t.Fatal("former duplicate was deleted along with canonical, want preserved")
	}
	if p.DuplicateOf != nil {
		t.Errorf("former duplicate still points at deleted canonical: %v", *p.DuplicateOf)
	}
}
