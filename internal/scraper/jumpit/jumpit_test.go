package jumpit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ohchanwu/job-scraper/internal/scraper"
)

// listingJSON builds a minimal listing envelope with the given position ids.
func listingJSON(ids ...int64) string {
	var b strings.Builder
	b.WriteString(`{"result":{"positions":[`)
	for i, id := range ids {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":%d}`, id)
	}
	b.WriteString(`]}}`)
	return b.String()
}

func TestScraperSource(t *testing.T) {
	if got := New().Source(); got != "jumpit" {
		t.Errorf("Source() = %q, want jumpit", got)
	}
}

func TestScraperCheckAccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	s := newScraper(srv.URL, 0)
	s.client.robotsHosts = []string{srv.URL}
	if err := s.CheckAccess(context.Background()); err != nil {
		t.Errorf("CheckAccess = %v, want nil", err)
	}
}

func TestFetchListing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(readFixture(t, "listing.json"))
	}))
	defer srv.Close()

	s := newScraper(srv.URL, 0)
	postings, err := s.FetchListing(context.Background(), 0)
	if err != nil {
		t.Fatalf("FetchListing: %v", err)
	}
	if len(postings) != 3 {
		t.Fatalf("got %d postings, want 3", len(postings))
	}
	if postings[0].SourcePostingID != "53688979" {
		t.Errorf("postings[0].SourcePostingID = %q, want 53688979", postings[0].SourcePostingID)
	}
}

func TestFetchListingRespectsLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(readFixture(t, "listing.json"))
	}))
	defer srv.Close()

	s := newScraper(srv.URL, 0)
	postings, err := s.FetchListing(context.Background(), 2)
	if err != nil {
		t.Fatalf("FetchListing: %v", err)
	}
	if len(postings) != 2 {
		t.Errorf("got %d postings, want 2 (capped by limit)", len(postings))
	}
}

func TestFetchListingPaginates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "1":
			w.Write([]byte(listingJSON(201, 202)))
		case "2":
			w.Write([]byte(listingJSON(203)))
		default:
			w.Write([]byte(listingJSON()))
		}
	}))
	defer srv.Close()

	s := newScraper(srv.URL, 0)
	s.pageSize = 2 // a full page, so the walk continues to page 2
	postings, err := s.FetchListing(context.Background(), 0)
	if err != nil {
		t.Fatalf("FetchListing: %v", err)
	}
	if len(postings) != 3 {
		t.Fatalf("got %d postings across pages, want 3", len(postings))
	}
	want := []string{"201", "202", "203"}
	for i, w := range want {
		if postings[i].SourcePostingID != w {
			t.Errorf("posting %d id = %q, want %q", i, postings[i].SourcePostingID, w)
		}
	}
}

func TestFetchDetail(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write(readFixture(t, "detail.json"))
	}))
	defer srv.Close()

	s := newScraper(srv.URL, 0)
	base := scraper.Posting{Source: "jumpit", SourcePostingID: "53688979"}
	got, err := s.FetchDetail(context.Background(), base)
	if err != nil {
		t.Fatalf("FetchDetail: %v", err)
	}
	if gotPath != "/api/position/53688979" {
		t.Errorf("requested path = %q, want /api/position/53688979", gotPath)
	}
	if got.Education == nil || *got.Education != 8 {
		t.Errorf("Education = %v, want 8 (detail parsed into the posting)", got.Education)
	}
	if got.Description == "" {
		t.Error("Description is empty, want the composed detail text")
	}
}
