//go:build integration

package daangn

import (
	"context"
	"io"
	"net/http"
	"strings"
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

// TestLiveDaangnURLResolves verifies that the URL each posting carries
// actually lands on a real job page — not the marketing home, not a
// generic redirect. This regression-guards against the "Greenhouse
// absolute_url was dead and we never noticed" class of bug surfaced
// by the 2026-05-27 link audit. One request per posting.
func TestLiveDaangnURLResolves(t *testing.T) {
	s := New()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := s.CheckAccess(ctx); err != nil {
		t.Fatalf("CheckAccess: %v", err)
	}
	postings, err := s.FetchListing(ctx, 3)
	if err != nil {
		t.Fatalf("FetchListing: %v", err)
	}
	if len(postings) == 0 {
		t.Skip("no 신입 IT 당근 postings to verify today")
	}
	client := &http.Client{Timeout: 15 * time.Second}
	for _, p := range postings {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
		if err != nil {
			t.Errorf("build request for %s: %v", p.URL, err)
			continue
		}
		// team.daangn.com 403s plain HTTP clients; impersonate a browser.
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		resp, err := client.Do(req)
		if err != nil {
			t.Errorf("GET %s: %v", p.URL, err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: status %d", p.URL, resp.StatusCode)
			continue
		}
		// A real job page contains the posting id somewhere (next-data
		// blob, asset path, etc.). The marketing home page does not.
		if !strings.Contains(string(body), p.SourcePostingID) {
			t.Errorf("destination of %s does not contain posting id %s — likely landed on a generic page",
				p.URL, p.SourcePostingID)
		}
	}
}
