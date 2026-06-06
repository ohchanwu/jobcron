//go:build integration

package greeting

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestLiveGreeting fetches every curated tenant board for real. Gated behind
// the `integration` build tag:
//
//	go test -tags integration ./internal/scraper/greeting/
func TestLiveGreeting(t *testing.T) {
	s := New()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if err := s.CheckAccess(ctx); err != nil {
		t.Fatalf("CheckAccess: %v", err)
	}
	postings, err := s.FetchListing(ctx, 0)
	if err != nil {
		t.Fatalf("FetchListing: %v", err)
	}
	t.Logf("live 그리팅: %d 신입 dev postings across %d tenants", len(postings), len(s.tenants))
	if len(postings) == 0 {
		t.Skip("no 그리팅 신입 dev postings today")
	}
	for _, p := range postings {
		if p.Source != "greeting" {
			t.Errorf("Source=%q, want greeting", p.Source)
		}
		if p.SourcePostingID == "" || p.Title == "" || p.Company == "" || p.URL == "" {
			t.Errorf("essential fields missing: id=%q title=%q company=%q url=%q",
				p.SourcePostingID, p.Title, p.Company, p.URL)
		}
		if !p.Newcomer {
			t.Errorf("id=%s not flagged Newcomer", p.SourcePostingID)
		}
		t.Logf("  %s | %s | %s | %s", p.SourcePostingID, p.Title, p.Location, p.URL)
	}
}

// TestLiveGreetingURLsResolve verifies each posting's URL lands on a page
// that contains the opening id — guarding against the landing-redirect /
// custom-domain class of bug. One request per posting (capped).
func TestLiveGreetingURLsResolve(t *testing.T) {
	s := New()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if err := s.CheckAccess(ctx); err != nil {
		t.Fatalf("CheckAccess: %v", err)
	}
	postings, err := s.FetchListing(ctx, 6)
	if err != nil {
		t.Fatalf("FetchListing: %v", err)
	}
	if len(postings) == 0 {
		t.Skip("no postings to verify today")
	}
	client := &http.Client{Timeout: 20 * time.Second}
	for _, p := range postings {
		// SourcePostingID is "{slug}-{openingId}"; the opening id must appear
		// on the destination page.
		openingID := p.SourcePostingID
		if i := strings.LastIndexByte(openingID, '-'); i >= 0 {
			openingID = openingID[i+1:]
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
		if err != nil {
			t.Errorf("build request for %s: %v", p.URL, err)
			continue
		}
		req.Header.Set("User-Agent", userAgent)
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
		if !strings.Contains(string(body), openingID) {
			t.Errorf("destination of %s lacks opening id %s — likely a generic page", p.URL, openingID)
		}
	}
}
