package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ohchanwu/job-scraper/internal/profile"
	"github.com/ohchanwu/job-scraper/internal/scraper"
	"github.com/ohchanwu/job-scraper/internal/storage"
)

// fakeScraper is a test double for scraper.Scraper — it returns canned data
// instead of hitting the live 점핏 API.
type fakeScraper struct {
	listing     []scraper.Posting
	details     map[string]scraper.Posting
	accessErr   error
	detailCalls []string // SourcePostingIDs FetchDetail was called for
}

func (f *fakeScraper) Source() string                        { return "jumpit" }
func (f *fakeScraper) CheckAccess(ctx context.Context) error { return f.accessErr }

func (f *fakeScraper) FetchListing(ctx context.Context, limit int) ([]scraper.Posting, error) {
	return f.listing, nil
}

func (f *fakeScraper) FetchDetail(ctx context.Context, p scraper.Posting) (scraper.Posting, error) {
	f.detailCalls = append(f.detailCalls, p.SourcePostingID)
	if d, ok := f.details[p.SourcePostingID]; ok {
		return d, nil
	}
	return p, nil
}

// noopEmit is an SSE emit callback that discards events.
func noopEmit(event, data string) {}

func listingPosting(sourceID, title string) scraper.Posting {
	return scraper.Posting{
		Source: "jumpit", SourcePostingID: sourceID, Title: title,
		Company: "테스트회사", URL: "https://example.test/" + sourceID,
	}
}

func newTestServer(t *testing.T, f *fakeScraper) (*Server, *storage.Store) {
	t.Helper()
	st, err := storage.OpenAt(filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return New(st, f), st
}

func TestRunScrapeStoresAndScoresPostings(t *testing.T) {
	f := &fakeScraper{
		listing: []scraper.Posting{listingPosting("1", "백엔드 신입"), listingPosting("2", "AI 신입")},
		details: map[string]scraper.Posting{
			"1": listingPosting("1", "백엔드 신입"),
			"2": listingPosting("2", "AI 신입"),
		},
	}
	srv, st := newTestServer(t, f)
	ctx := context.Background()

	profJSON, _ := profile.Marshal(profile.Profile{CareerYears: 0})
	if _, _, err := st.SaveProfile(ctx, profJSON); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	res, err := srv.runScrape(ctx, noopEmit)
	if err != nil {
		t.Fatalf("runScrape: %v", err)
	}
	if res.Listed != 2 || res.New != 2 || res.Scored != 2 {
		t.Errorf("ScrapeResult = %+v, want {Listed:2 New:2 Scored:2}", res)
	}

	postings, err := st.AllPostings(ctx)
	if err != nil || len(postings) != 2 {
		t.Fatalf("AllPostings: got %d (err=%v), want 2", len(postings), err)
	}
	scores, err := st.ScoresByPostingID(ctx)
	if err != nil || len(scores) != 2 {
		t.Fatalf("ScoresByPostingID: got %d (err=%v), want 2", len(scores), err)
	}
}

func TestRunScrapeSkipsDetailForKnownPostings(t *testing.T) {
	f := &fakeScraper{
		listing: []scraper.Posting{listingPosting("1", "기존 공고"), listingPosting("2", "새 공고")},
	}
	srv, st := newTestServer(t, f)
	ctx := context.Background()

	seen := listingPosting("1", "기존 공고")
	seen.FirstSeenAt = time.Now().Add(-48 * time.Hour).UTC()
	seen.LastSeenAt = seen.FirstSeenAt
	if _, _, err := st.UpsertPosting(ctx, seen); err != nil {
		t.Fatalf("seed UpsertPosting: %v", err)
	}

	if _, err := srv.runScrape(ctx, noopEmit); err != nil {
		t.Fatalf("runScrape: %v", err)
	}
	if len(f.detailCalls) != 1 || f.detailCalls[0] != "2" {
		t.Errorf("FetchDetail called for %v, want only [2] — posting 1 is already known", f.detailCalls)
	}
}

func TestRunScrapeFailsWhenAccessDenied(t *testing.T) {
	f := &fakeScraper{accessErr: errors.New("robots.txt disallows")}
	srv, _ := newTestServer(t, f)
	if _, err := srv.runScrape(context.Background(), noopEmit); err == nil {
		t.Error("runScrape = nil error, want an error when CheckAccess fails")
	}
}

func TestHandleScrapeStreamsSSE(t *testing.T) {
	f := &fakeScraper{listing: []scraper.Posting{listingPosting("1", "신입 공고")}}
	srv, st := newTestServer(t, f)
	profJSON, _ := profile.Marshal(profile.Profile{})
	if _, _, err := st.SaveProfile(context.Background(), profJSON); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/scrape", nil))

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{"event: status", "event: count", "event: done"} {
		if !strings.Contains(body, want) {
			t.Errorf("SSE stream missing %q\n--- body ---\n%s", want, body)
		}
	}
}

func TestHandleScrapeRejectsConcurrentScrape(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	srv.flight.tryAcquire("jumpit") // a scrape is already in progress

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/scrape", nil))
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 when a scrape is already running", rec.Code)
	}
}

func TestHandleProfileSaveThenLoad(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})

	form := url.Values{}
	form.Set("career_years", "0")
	form.Set("salary_floor_man", "5000")
	form.Set("max_education", "3") // bachelor
	form.Set("stacks", "React,20\nGo,30")
	form.Set("cities", "서울, 판교")
	form.Set("location_weight", "15")
	form.Set("remote_ok", "on")
	form.Set("must_have", "React")
	form.Set("dealbreakers", "병역특례")

	req := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("save status = %d, want 303; body=%s", rec.Code, rec.Body)
	}

	jsonStr, _, ok, err := st.Profile(context.Background())
	if err != nil || !ok {
		t.Fatalf("Profile: ok=%v err=%v", ok, err)
	}
	p, err := profile.Unmarshal(jsonStr)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(p.Stacks) != 2 || p.Stacks[0].Name != "React" || p.Stacks[0].Weight != 20 {
		t.Errorf("stacks = %+v", p.Stacks)
	}
	if p.MaxEducation != profile.EducationBachelor {
		t.Errorf("max education = %v, want bachelor", p.MaxEducation)
	}
	if p.SalaryFloorKRW != 50_000_000 {
		t.Errorf("salary floor = %d, want 50000000", p.SalaryFloorKRW)
	}
	if len(p.Dealbreakers) != 1 || p.Dealbreakers[0] != "병역특례" {
		t.Errorf("dealbreakers = %v", p.Dealbreakers)
	}

	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/profile", nil))
	if rec2.Code != http.StatusOK || !strings.Contains(rec2.Body.String(), "병역특례") {
		t.Errorf("GET /profile: code=%d, body missing saved data", rec2.Code)
	}
}

func TestFirstRunRedirectsToProfile(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/profile" {
		t.Errorf("first run: code=%d loc=%q, want 303 -> /profile",
			rec.Code, rec.Header().Get("Location"))
	}
}

func TestDashboardShowsScoredPostings(t *testing.T) {
	f := &fakeScraper{listing: []scraper.Posting{listingPosting("1", "백엔드 신입 개발자")}}
	srv, st := newTestServer(t, f)
	ctx := context.Background()
	profJSON, _ := profile.Marshal(profile.Profile{})
	if _, _, err := st.SaveProfile(ctx, profJSON); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	if _, err := srv.runScrape(ctx, noopEmit); err != nil {
		t.Fatalf("runScrape: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "백엔드 신입 개발자") {
		t.Errorf("dashboard body does not list the scraped posting")
	}
}
