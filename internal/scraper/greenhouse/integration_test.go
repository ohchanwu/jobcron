//go:build integration

package greenhouse

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// browserUA impersonates a browser: some careers hosts (team.daangn.com)
// 403 plain HTTP clients.
const browserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

func liveTenants() map[string]*Scraper {
	return map[string]*Scraper{
		"daangn":   Daangn(),
		"krafton":  Krafton(),
		"moloco":   Moloco(),
		"sendbird": Sendbird(),
	}
}

// TestLiveGreenhouse performs one real round trip per tenant against the
// Greenhouse public board API. Gated behind the `integration` build tag:
//
//	go test -tags integration ./internal/scraper/greenhouse/
func TestLiveGreenhouse(t *testing.T) {
	for name, s := range liveTenants() {
		name, s := name, s
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := s.CheckAccess(ctx); err != nil {
				t.Fatalf("CheckAccess: %v", err)
			}
			postings, err := s.FetchListing(ctx, 10)
			if err != nil {
				t.Fatalf("FetchListing: %v", err)
			}
			t.Logf("live %s: %d 신입 dev postings", name, len(postings))
			for _, p := range postings {
				if p.Source != s.t.Source {
					t.Errorf("Source=%q, want %q", p.Source, s.t.Source)
				}
				if p.SourcePostingID == "" || p.Title == "" || p.Company == "" || p.URL == "" {
					t.Errorf("essential fields missing: id=%q title=%q company=%q url=%q",
						p.SourcePostingID, p.Title, p.Company, p.URL)
				}
				if !p.Newcomer {
					t.Errorf("id=%s not flagged Newcomer despite passing the 신입 filter", p.SourcePostingID)
				}
				t.Logf("  %s | %s | %s | %s", p.SourcePostingID, p.Title, p.Location, p.URL)
			}
		})
	}
}

// TestLiveGreenhouseURLsResolve verifies the click-through URL each posting
// carries lands on a page that actually contains the posting id — guarding
// against the dead-absolute_url / wrong-redirect class of bug. One request
// per posting (capped).
func TestLiveGreenhouseURLsResolve(t *testing.T) {
	client := &http.Client{Timeout: 20 * time.Second}
	for name, s := range liveTenants() {
		name, s := name, s
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			if err := s.CheckAccess(ctx); err != nil {
				t.Fatalf("CheckAccess: %v", err)
			}
			postings, err := s.FetchListing(ctx, 3)
			if err != nil {
				t.Fatalf("FetchListing: %v", err)
			}
			if len(postings) == 0 {
				t.Skipf("no 신입 dev postings on %s today", name)
			}
			// The host the click-through must END on after following redirects.
			// 센드버드 (2026-06-08) regressed by 302-redirecting its hosted board
			// URL to sendbird.com's careers FRONT PAGE — which still echoes the
			// gh_jid in its HTML, so the body-contains-id check below passes even
			// though the destination is wrong. Pinning the final host catches that
			// off-board redirect class: a Greenhouse-built URL must stay on
			// Greenhouse; a LinkSite URL must stay on the tenant's own site.
			wantHost := "job-boards.greenhouse.io"
			if s.t.Link == LinkSite {
				u, err := url.Parse(s.t.SiteURL)
				if err != nil {
					t.Fatalf("parse SiteURL %q: %v", s.t.SiteURL, err)
				}
				wantHost = u.Host
			}
			for _, p := range postings {
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
				if err != nil {
					t.Errorf("build request for %s: %v", p.URL, err)
					continue
				}
				req.Header.Set("User-Agent", browserUA)
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
				if gotHost := resp.Request.URL.Host; gotHost != wantHost {
					t.Errorf("destination of %s redirected off-host to %q, want %q — likely a company careers front page, not the posting",
						p.URL, gotHost, wantHost)
				}
				if !strings.Contains(string(body), p.SourcePostingID) {
					t.Errorf("destination of %s lacks posting id %s — likely landed on a generic page",
						p.URL, p.SourcePostingID)
				}
			}
		})
	}
}
