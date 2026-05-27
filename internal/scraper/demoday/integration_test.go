//go:build integration

package demoday

import (
	"context"
	"testing"
	"time"
)

// TestLiveDemoday performs one real round trip against the demoday
// Supabase host. Gated behind the `integration` build tag; run with
//
//	go test -tags integration ./internal/scraper/demoday/
//
// Pacing is the production 1 req/s.
func TestLiveDemoday(t *testing.T) {
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
	if len(postings) == 0 {
		t.Skip("live demoday returned 0 신입-relevant postings — site state varies")
	}
	t.Logf("live: %d postings (capped at 5)", len(postings))
	p := postings[0]
	if p.Source != "demoday" {
		t.Errorf("Source = %q", p.Source)
	}
	if p.SourcePostingID == "" || p.Title == "" || p.Company == "" {
		t.Errorf("essential fields missing: id=%q title=%q company=%q",
			p.SourcePostingID, p.Title, p.Company)
	}
	if p.URL == "" {
		t.Error("URL not set")
	}
}
