//go:build integration

package jumpit

import (
	"context"
	"testing"
	"time"
)

// TestLiveJumpit exercises the real 점핏 API end to end. It is excluded from
// normal test runs by the `integration` build tag — run it deliberately with:
//
//	go test -tags integration ./internal/scraper/jumpit/
//
// It honors the polite 1s rate limit, so it takes several seconds.
func TestLiveJumpit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	s := New()

	if err := s.CheckAccess(ctx); err != nil {
		t.Fatalf("CheckAccess: %v", err)
	}

	postings, err := s.FetchListing(ctx, 3)
	if err != nil {
		t.Fatalf("FetchListing: %v", err)
	}
	if len(postings) == 0 {
		t.Fatal("FetchListing returned no 신입 postings")
	}
	for _, p := range postings {
		if p.Source != "jumpit" || p.SourcePostingID == "" || p.Title == "" {
			t.Errorf("listing posting looks unpopulated: %+v", p)
		}
	}

	detailed, err := s.FetchDetail(ctx, postings[0])
	if err != nil {
		t.Fatalf("FetchDetail: %v", err)
	}
	if detailed.Description == "" {
		t.Errorf("FetchDetail produced an empty Description for posting %s", detailed.SourcePostingID)
	}
	t.Logf("live: %d listing postings; detail for %s has a %d-char description",
		len(postings), detailed.SourcePostingID, len(detailed.Description))
}
