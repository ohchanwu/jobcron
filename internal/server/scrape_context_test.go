package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ohchanwu/job-scraper/internal/profile"
	"github.com/ohchanwu/job-scraper/internal/scraper"
)

// TestScrapeCompletesDespiteRequestCancellation locks in Bug 2A: a scrape must
// NOT be tied to the HTTP request context. Navigating away from the briefing
// mid-scrape tears down the SSE EventSource and cancels the request; if the
// scrape used that context it would abort, leaving postings inserted but
// unscored (the end-of-run scoreAll never reached) — the blank-card bug.
//
// We drive the worst case: a request whose context is ALREADY cancelled. The
// scrape must still run to completion against its own detached context and
// score everything. (With the old r.Context() wiring the first ctx-aware DB op
// errors out, so nothing is persisted at all.)
func TestScrapeCompletesDespiteRequestCancellation(t *testing.T) {
	fs := &fakeScraper{listing: []scraper.Posting{listingPosting("p1", "신입 개발자 공고")}}
	srv, st := newTestServer(t, fs)
	ctx := context.Background()
	profJSON, _ := profile.Marshal(profile.Profile{})
	if _, _, err := st.SaveProfile(ctx, profJSON); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	reqCtx, cancel := context.WithCancel(context.Background())
	cancel() // user navigated away the instant the scrape began
	req := httptest.NewRequest(http.MethodGet, "/api/scrape", nil).WithContext(reqCtx)
	srv.handleScrapeSSE(httptest.NewRecorder(), req)

	postings, err := st.AllPostings(ctx)
	if err != nil {
		t.Fatalf("AllPostings: %v", err)
	}
	if len(postings) == 0 {
		t.Fatal("scrape persisted no postings — it was cancelled with the request instead of detaching")
	}
	scores, err := st.ScoresByPostingID(ctx)
	if err != nil {
		t.Fatalf("ScoresByPostingID: %v", err)
	}
	for _, p := range postings {
		if _, ok := scores[p.ID]; !ok {
			t.Fatalf("posting %d (%q) left unscored — the end-of-run scoreAll never reached it", p.ID, p.Title)
		}
	}
}
