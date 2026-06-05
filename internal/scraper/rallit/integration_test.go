//go:build integration

package rallit

import (
	"context"
	"testing"
	"time"
)

// TestLiveDeveloperFilterReturnsPostings is a guard against the silent-zero
// footgun introduced by the jobGroup=DEVELOPER filter. 랠릿 returns a normal
// envelope (statusCode "OK", totalCount 0, empty items) for an UNKNOWN jobGroup
// value rather than an error — so if 랠릿 ever renames or splits the DEVELOPER
// group, FetchListing would silently yield zero postings with no failure, and
// the briefing would quietly go stale (existing 랠릿 rows linger up to ~90 days
// because the staleness baseline never advances). This live test trips loudly
// instead. Excluded from normal runs by the `integration` build tag:
//
//	go test -tags integration ./internal/scraper/rallit/
//
// It honors the polite 1s rate limit, so it takes a few seconds.
func TestLiveDeveloperFilterReturnsPostings(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	s := New()
	if err := s.CheckAccess(ctx); err != nil {
		t.Fatalf("CheckAccess: %v", err)
	}
	postings, err := s.FetchListing(ctx, 0)
	if err != nil {
		t.Fatalf("FetchListing: %v", err)
	}
	if len(postings) == 0 {
		t.Fatal("랠릿 jobGroup=DEVELOPER 신입 listing returned ZERO postings — the jobGroup " +
			"enum may have been renamed/split. Re-run the totalCount probe documented in " +
			"API_NOTES.md (\"직군 (job group) filter\") and update devJobGroup in rallit.go.")
	}
	t.Logf("랠릿 DEVELOPER 신입 listing returned %d postings", len(postings))
}
