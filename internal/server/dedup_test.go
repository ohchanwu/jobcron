package server

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ohchanwu/job-scraper/internal/scraper"
	"github.com/ohchanwu/job-scraper/internal/storage"
)

// makePosting builds a minimal canonical-friendly posting for dedup tests.
func makePosting(source, sourceID, title, company, location string, firstSeen time.Time) scraper.Posting {
	return scraper.Posting{
		Source:          source,
		SourcePostingID: sourceID,
		URL:             "https://example.com/" + source + "/" + sourceID,
		Title:           title,
		Company:         company,
		Location:        location,
		Newcomer:        true,
		StackTags:       []string{},
		Tags:            []scraper.Tag{},
		Description:     "",
		RawJSON:         `{}`,
		FirstSeenAt:     firstSeen,
		LastSeenAt:      firstSeen,
	}
}

// newServerWithStore builds a Server backed by a fresh on-disk DB and a
// no-op fake scraper. Dedup orchestration tests don't need real scrape
// data — they seed postings directly through the store.
func newServerWithStore(t *testing.T) *Server {
	t.Helper()
	st, err := storage.OpenAt(filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return New(st, &fakeScraper{})
}

func TestMarkCrossPortalDuplicatesCollapsesMatchingPair(t *testing.T) {
	srv := newServerWithStore(t)
	ctx := context.Background()
	t0 := time.Date(2026, 5, 26, 8, 0, 0, 0, time.UTC)

	canonicalID, _, err := srv.store.UpsertPosting(ctx,
		makePosting("jumpit", "100", "Backend Engineer", "토스", "서울 강남구", t0))
	if err != nil {
		t.Fatalf("seed canonical: %v", err)
	}
	dupID, _, err := srv.store.UpsertPosting(ctx,
		makePosting("rallit", "200", "Backend Engineer", "토스", "서울 강남구", t0.Add(time.Hour)))
	if err != nil {
		t.Fatalf("seed dup: %v", err)
	}

	n, err := srv.markCrossPortalDuplicates(ctx)
	if err != nil {
		t.Fatalf("markCrossPortalDuplicates: %v", err)
	}
	if n != 1 {
		t.Errorf("returned %d, want 1", n)
	}
	dup, _, _ := srv.store.PostingByID(ctx, dupID)
	if dup.DuplicateOf == nil || *dup.DuplicateOf != canonicalID {
		t.Errorf("dup.DuplicateOf = %v, want %d", dup.DuplicateOf, canonicalID)
	}
	canon, _, _ := srv.store.PostingByID(ctx, canonicalID)
	if canon.DuplicateOf != nil {
		t.Errorf("canon.DuplicateOf = %v, want nil", *canon.DuplicateOf)
	}
}

func TestMarkCrossPortalDuplicatesPicksEarliestAsCanonical(t *testing.T) {
	srv := newServerWithStore(t)
	ctx := context.Background()
	t0 := time.Date(2026, 5, 26, 8, 0, 0, 0, time.UTC)

	// Insert in reverse-time order — rallit seen first, jumpit later.
	rallitID, _, _ := srv.store.UpsertPosting(ctx,
		makePosting("rallit", "200", "Backend Engineer", "토스", "서울 강남구", t0))
	jumpitID, _, _ := srv.store.UpsertPosting(ctx,
		makePosting("jumpit", "100", "Backend Engineer", "토스", "서울 강남구", t0.Add(time.Hour)))

	if _, err := srv.markCrossPortalDuplicates(ctx); err != nil {
		t.Fatalf("markCrossPortalDuplicates: %v", err)
	}

	// rallit was first → canonical. jumpit → duplicate.
	canon, _, _ := srv.store.PostingByID(ctx, rallitID)
	if canon.DuplicateOf != nil {
		t.Errorf("expected rallit (first seen) to be canonical, got duplicate_of=%v", *canon.DuplicateOf)
	}
	dup, _, _ := srv.store.PostingByID(ctx, jumpitID)
	if dup.DuplicateOf == nil || *dup.DuplicateOf != rallitID {
		t.Errorf("expected jumpit to be duplicate_of rallit, got %v", dup.DuplicateOf)
	}
}

func TestMarkCrossPortalDuplicatesClearsStaleMarks(t *testing.T) {
	srv := newServerWithStore(t)
	ctx := context.Background()
	t0 := time.Date(2026, 5, 26, 8, 0, 0, 0, time.UTC)

	// Seed two postings that DO match.
	idA, _, _ := srv.store.UpsertPosting(ctx,
		makePosting("jumpit", "100", "Backend Engineer", "토스", "서울 강남구", t0))
	idB, _, _ := srv.store.UpsertPosting(ctx,
		makePosting("rallit", "200", "Backend Engineer", "토스", "서울 강남구", t0.Add(time.Hour)))

	// Stale mark from a prior pass: claim a different pairing.
	if err := srv.store.MarkDuplicate(ctx, idA, idB); err != nil {
		t.Fatalf("seed stale mark: %v", err)
	}

	// Re-run dedup. The stale mark should be cleared and re-derived correctly.
	if _, err := srv.markCrossPortalDuplicates(ctx); err != nil {
		t.Fatalf("markCrossPortalDuplicates: %v", err)
	}
	pA, _, _ := srv.store.PostingByID(ctx, idA)
	pB, _, _ := srv.store.PostingByID(ctx, idB)
	if pA.DuplicateOf != nil {
		t.Errorf("idA (earlier) should be canonical, got duplicate_of=%v", *pA.DuplicateOf)
	}
	if pB.DuplicateOf == nil || *pB.DuplicateOf != idA {
		t.Errorf("idB should be duplicate_of idA=%d, got %v", idA, pB.DuplicateOf)
	}
}

func TestMarkCrossPortalDuplicatesIgnoresNonMatchingPairs(t *testing.T) {
	srv := newServerWithStore(t)
	ctx := context.Background()
	t0 := time.Date(2026, 5, 26, 8, 0, 0, 0, time.UTC)

	// Two postings, same company, DIFFERENT locations — should not match.
	idA, _, _ := srv.store.UpsertPosting(ctx,
		makePosting("jumpit", "100", "Backend Engineer", "토스", "서울 강남구", t0))
	idB, _, _ := srv.store.UpsertPosting(ctx,
		makePosting("rallit", "200", "Backend Engineer", "토스", "판교 분당구", t0.Add(time.Hour)))

	n, err := srv.markCrossPortalDuplicates(ctx)
	if err != nil {
		t.Fatalf("markCrossPortalDuplicates: %v", err)
	}
	if n != 0 {
		t.Errorf("marked %d, want 0", n)
	}
	for _, id := range []int64{idA, idB} {
		p, _, _ := srv.store.PostingByID(ctx, id)
		if p.DuplicateOf != nil {
			t.Errorf("id %d should be canonical, got duplicate_of=%v", id, *p.DuplicateOf)
		}
	}
}
