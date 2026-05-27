//go:build integration

package daangn

import (
	"context"
	"testing"
	"time"
)

// TestLiveDaangn performs one real round trip against the Greenhouse
// public board API. Gated behind the `integration` build tag; run with
//
//	go test -tags integration ./internal/scraper/daangn/
func TestLiveDaangn(t *testing.T) {
	s := New()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.CheckAccess(ctx); err != nil {
		t.Fatalf("CheckAccess: %v", err)
	}
	postings, err := s.FetchListing(ctx, 5)
	if err != nil {
		t.Fatalf("FetchListing: %v", err)
	}
	t.Logf("live: %d postings (capped at 5)", len(postings))
	for _, p := range postings {
		if p.Source != "daangn" {
			t.Errorf("Source = %q", p.Source)
		}
		if p.SourcePostingID == "" || p.Title == "" || p.Company == "" {
			t.Errorf("essential fields missing: id=%q title=%q company=%q",
				p.SourcePostingID, p.Title, p.Company)
		}
		if !p.Newcomer {
			t.Errorf("posting id=%s not flagged Newcomer despite passing 신입 filter", p.SourcePostingID)
		}
	}
}
