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
func (f *fakeScraper) Kind() scraper.SourceKind              { return scraper.SourceKindAggregator }
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

func TestRunScrapeSweepsStalePostings(t *testing.T) {
	// One pre-existing posting last seen 10 days ago. The scrape returns
	// only a fresh listing, so the pre-existing one is not re-seen and
	// becomes stale relative to the new MAX(last_seen_at).
	f := &fakeScraper{listing: []scraper.Posting{listingPosting("fresh", "갓 올라온 공고")}}
	srv, st := newTestServer(t, f)
	ctx := context.Background()

	stale := listingPosting("stale", "오래 안 보이던 공고")
	stale.FirstSeenAt = time.Now().Add(-15 * 24 * time.Hour).UTC()
	stale.LastSeenAt = time.Now().Add(-10 * 24 * time.Hour).UTC()
	if _, _, err := st.UpsertPosting(ctx, stale); err != nil {
		t.Fatalf("seed stale posting: %v", err)
	}

	var sweepStatus string
	emit := func(event, data string) {
		if event == "status" && strings.Contains(data, "정리했어요") {
			sweepStatus = data
		}
	}
	res, err := srv.runScrape(ctx, emit)
	if err != nil {
		t.Fatalf("runScrape: %v", err)
	}
	if res.Removed != 1 {
		t.Errorf("ScrapeResult.Removed = %d, want 1 (the stale posting)", res.Removed)
	}
	if sweepStatus == "" {
		t.Error("scrape did not emit a sweep status message when removing postings")
	}

	postings, _ := st.AllPostings(ctx)
	if len(postings) != 1 || postings[0].SourcePostingID != "fresh" {
		t.Errorf("postings after sweep = %+v, want only [fresh]", postings)
	}
}

func TestRunScrapeIsolatesPerSourceFailures(t *testing.T) {
	// When the only source fails, runScrape now reports it via the result
	// (Failed=1) and emits a status, rather than returning an error. Real
	// callers wire multiple sources; a single failure should not abort
	// the whole briefing.
	f := &fakeScraper{accessErr: errors.New("robots.txt disallows")}
	srv, _ := newTestServer(t, f)
	res, err := srv.runScrape(context.Background(), noopEmit)
	if err != nil {
		t.Fatalf("runScrape returned unexpected error: %v", err)
	}
	if res.Failed != 1 {
		t.Errorf("res.Failed = %d, want 1 (the access-denied source)", res.Failed)
	}
	if res.New != 0 {
		t.Errorf("res.New = %d, want 0 (failed source produced nothing)", res.New)
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
	srv.flight.tryAcquire(scrapeAllKey) // a scrape is already in progress

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

func TestDeadlineBadge(t *testing.T) {
	kst := time.FixedZone("KST", 9*3600)
	now := time.Date(2026, 5, 22, 15, 0, 0, 0, kst)
	at := func(y int, m time.Month, d int) *time.Time {
		v := time.Date(y, m, d, 23, 59, 59, 0, kst)
		return &v
	}
	cases := []struct {
		name       string
		closedAt   *time.Time
		alwaysOpen bool
		want       string
	}{
		{"closes today", at(2026, 5, 22), false, "오늘 마감"},
		{"closes tomorrow", at(2026, 5, 23), false, "마감 D-1"},
		{"closes in 3 days", at(2026, 5, 25), false, "마감 D-3"},
		{"closes in 10 days", at(2026, 6, 1), false, ""},
		{"always open", nil, true, ""},
		{"no closing date", nil, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deadlineBadge(tc.closedAt, tc.alwaysOpen, now); got != tc.want {
				t.Errorf("deadlineBadge = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDashboardShowsOnlyTodaysPostings(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	ctx := context.Background()
	profJSON, _ := profile.Marshal(profile.Profile{})
	if _, _, err := st.SaveProfile(ctx, profJSON); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	today := listingPosting("today1", "오늘 본 공고")
	today.FirstSeenAt = time.Now().UTC()
	today.LastSeenAt = today.FirstSeenAt
	old := listingPosting("old1", "예전에 본 공고")
	old.FirstSeenAt = time.Now().Add(-72 * time.Hour).UTC()
	old.LastSeenAt = old.FirstSeenAt
	for _, p := range []scraper.Posting{today, old} {
		if _, _, err := st.UpsertPosting(ctx, p); err != nil {
			t.Fatalf("UpsertPosting: %v", err)
		}
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	body := rec.Body.String()
	if !strings.Contains(body, "오늘 본 공고") {
		t.Error("dashboard is missing today's posting")
	}
	if strings.Contains(body, "예전에 본 공고") {
		t.Error("dashboard shows a posting first seen on a previous day")
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
