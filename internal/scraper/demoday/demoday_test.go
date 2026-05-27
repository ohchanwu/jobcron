package demoday

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestParseListing exercises the JSON → Posting normalization against
// the captured fixture (testdata/listing_fixture.json, 3 real records
// pulled on 2026-05-27). The assertions pin the fields the scoring
// engine reads.
func TestParseListing(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "listing_fixture.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	postings, err := parseListing(body, defaultSiteURL)
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}
	if len(postings) != 3 {
		t.Fatalf("got %d postings, want 3", len(postings))
	}

	// Posting 0: id=16967, 라라비치, entry → Newcomer=true, MinCareer=0.
	p0 := postings[0]
	if p0.Source != "demoday" {
		t.Errorf("Source = %q, want demoday", p0.Source)
	}
	if p0.SourcePostingID != "16967" {
		t.Errorf("SourcePostingID = %q, want 16967", p0.SourcePostingID)
	}
	if p0.URL != "https://demoday.co.kr/recruits/16967" {
		t.Errorf("URL = %q, want canonical recruits URL", p0.URL)
	}
	if !strings.Contains(p0.Title, "라라비치") {
		t.Errorf("Title %q missing 라라비치", p0.Title)
	}
	if p0.Company != "라라비치" {
		t.Errorf("Company = %q, want 라라비치", p0.Company)
	}
	if !p0.Newcomer {
		t.Error("entry-level posting was not flagged Newcomer")
	}
	if p0.MinCareer != 0 || p0.MaxCareer != 0 {
		t.Errorf("entry-level career range = (%d,%d), want (0,0)", p0.MinCareer, p0.MaxCareer)
	}
	if p0.CareerLevel != "신입" {
		t.Errorf("CareerLevel = %q, want 신입", p0.CareerLevel)
	}
	// Description should be HTML-stripped — no '<' or '>' characters.
	if strings.ContainsAny(p0.Description, "<>") {
		t.Errorf("Description still contains HTML: %q", p0.Description[:80])
	}
	if !strings.Contains(p0.Description, "라라비치") {
		t.Errorf("Description missing 라라비치 from stripped content")
	}
	if p0.RawJSON == "" {
		t.Error("RawJSON not populated")
	}

	// Posting 1: id=16966, 스팩스페이스, 1-3 → Newcomer=false, range 1-3.
	p1 := postings[1]
	if p1.SourcePostingID != "16966" {
		t.Errorf("[1] SourcePostingID = %q, want 16966", p1.SourcePostingID)
	}
	if p1.Newcomer {
		t.Error("[1] 1-3 posting was wrongly flagged Newcomer")
	}
	if p1.MinCareer != 1 || p1.MaxCareer != 3 {
		t.Errorf("[1] career range = (%d,%d), want (1,3)", p1.MinCareer, p1.MaxCareer)
	}
	if p1.CareerLevel != "1-3년" {
		t.Errorf("[1] CareerLevel = %q, want 1-3년", p1.CareerLevel)
	}

	// PublishedAt should parse the created_at ISO timestamp.
	if p0.PublishedAt == nil {
		t.Error("PublishedAt = nil, want parsed time")
	}
}

func TestParseListingRejectsInvalidJSON(t *testing.T) {
	_, err := parseListing([]byte("not json"), defaultSiteURL)
	if err == nil {
		t.Error("parseListing accepted garbage input, want error")
	}
}

func TestExperienceBounds(t *testing.T) {
	cases := []struct {
		level      string
		wantMin    int
		wantMax    int
		wantNewcom bool
	}{
		{"entry", 0, 0, true},
		{"1-3", 1, 3, false},
		{"unknown", 1, 3, false}, // safe default
		{"", 1, 3, false},
		{"  ENTRY  ", 0, 0, true}, // trimming and case-insensitivity
	}
	for _, c := range cases {
		gotMin, gotMax, gotNew := experienceBounds(c.level)
		if gotMin != c.wantMin || gotMax != c.wantMax || gotNew != c.wantNewcom {
			t.Errorf("experienceBounds(%q) = (%d,%d,%v), want (%d,%d,%v)",
				c.level, gotMin, gotMax, gotNew, c.wantMin, c.wantMax, c.wantNewcom)
		}
	}
}

func TestStripHTML(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"<p>hello</p>", "hello"},
		{"<p>hello   world</p>", "hello world"},
		{"<p>line one<br>line two</p>", "line one line two"},
		{"<figure><table><tr><td>cell</td></tr></table></figure>", "cell"},
		{"", ""},
		{"plain text", "plain text"},
		{"<p>한국어 텍스트</p>", "한국어 텍스트"},
	}
	for _, c := range cases {
		got := stripHTML(c.in)
		if got != c.want {
			t.Errorf("stripHTML(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseTimestamp(t *testing.T) {
	cases := []struct {
		in         string
		want       string // formatted as RFC3339 in UTC
		wantParsed bool
	}{
		{"2026-05-27T00:05:19+00:00", "2026-05-27T00:05:19Z", true},
		{"2026-05-27T00:05:19.123456+00:00", "2026-05-27T00:05:19.123456Z", true},
		{"2026-05-27T00:05:19Z", "2026-05-27T00:05:19Z", true},
		{"", "", false},
		{"not a date", "", false},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, ok := parseTimestamp(c.in)
			if ok != c.wantParsed {
				t.Fatalf("ok = %v, want %v", ok, c.wantParsed)
			}
			if !ok {
				return
			}
			gotS := got.Format(time.RFC3339Nano)
			if gotS != c.want && !strings.HasPrefix(gotS, c.want[:10]) {
				t.Errorf("got %q, want %q", gotS, c.want)
			}
		})
	}
}

func TestParseDateAlwaysOpen(t *testing.T) {
	// "" yields false → caller marks AlwaysOpen=true.
	if _, ok := parseDate(""); ok {
		t.Error("parseDate(empty) returned ok, want false")
	}
	// Real date parses to UTC midnight of the KST day.
	got, ok := parseDate("2026-05-29")
	if !ok {
		t.Fatal("parseDate(real date) returned !ok")
	}
	// 2026-05-29 KST midnight = 2026-05-28 15:00 UTC.
	want := time.Date(2026, 5, 28, 15, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parseDate = %v, want %v", got, want)
	}
}

// TestFetchListingAgainstFakeSupabase exercises the full HTTP path —
// header construction, query string, JSON decode — against a fake
// Supabase server that returns the captured fixture.
func TestFetchListingAgainstFakeSupabase(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "listing_fixture.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var gotPath, gotQuery, gotKey, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotKey = r.Header.Get("apikey")
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	s := newScraper(defaultSiteURL, srv.URL, 0)
	postings, err := s.FetchListing(context.Background(), 0)
	if err != nil {
		t.Fatalf("FetchListing: %v", err)
	}

	if gotPath != "/rest/v1/recruits" {
		t.Errorf("path = %q, want /rest/v1/recruits", gotPath)
	}
	if !strings.Contains(gotQuery, "experience_level=in.%28entry%2C1-3%2Cany%29") {
		t.Errorf("query missing experience_level filter (expected entry,1-3,any): %s", gotQuery)
	}
	if !strings.Contains(gotQuery, "status=eq.published") {
		t.Errorf("query missing status filter: %s", gotQuery)
	}
	if gotKey == "" {
		t.Error("apikey header not set")
	}
	if !strings.HasPrefix(gotAuth, "Bearer ") {
		t.Errorf("Authorization header = %q, want Bearer prefix", gotAuth)
	}
	if len(postings) != 3 {
		t.Errorf("got %d postings, want 3", len(postings))
	}
}

func TestFetchListingRespectsLimit(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "listing_fixture.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	s := newScraper(defaultSiteURL, srv.URL, 0)
	postings, err := s.FetchListing(context.Background(), 2)
	if err != nil {
		t.Fatalf("FetchListing: %v", err)
	}
	if len(postings) != 2 {
		t.Errorf("limit=2 returned %d, want 2", len(postings))
	}
}

func TestFetchListingHandles206PartialContent(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "listing_fixture.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	s := newScraper(defaultSiteURL, srv.URL, 0)
	if _, err := s.FetchListing(context.Background(), 0); err != nil {
		t.Errorf("FetchListing rejected 206 Partial Content: %v", err)
	}
}

func TestCheckAccessHonorsSiteDisallow(t *testing.T) {
	// Site host serves robots that disallows /recruits — scraper must
	// refuse to proceed even though the API host is unrestricted.
	siteRobots := "User-Agent: *\nDisallow: /recruits\n"
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			_, _ = w.Write([]byte(siteRobots))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer site.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound) // 404 = unrestricted
	}))
	defer api.Close()

	s := newScraper(site.URL, api.URL, 0)
	err := s.CheckAccess(context.Background())
	if err == nil {
		t.Fatal("CheckAccess allowed scraping despite site Disallow")
	}
	if !strings.Contains(err.Error(), "robots") {
		t.Errorf("error %q does not mention robots", err)
	}
}

func TestCheckAccessAllowsWhenBothHostsAreClean(t *testing.T) {
	siteRobots := "User-Agent: *\nAllow: /\nDisallow: /api/\nDisallow: /_next/\n"
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			_, _ = w.Write([]byte(siteRobots))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer site.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer api.Close()

	s := newScraper(site.URL, api.URL, 0)
	if err := s.CheckAccess(context.Background()); err != nil {
		t.Errorf("CheckAccess returned %v, want nil — /recruits is allowed", err)
	}
}

// TestApplicationDeadlineNullMeansAlwaysOpen confirms the
// AlwaysOpen-from-null-deadline semantics matches what the rest of
// the app expects (sweep treats AlwaysOpen specially).
func TestApplicationDeadlineNullMeansAlwaysOpen(t *testing.T) {
	row := map[string]any{
		"id":                   1,
		"title":                "test",
		"company_name":         "company",
		"experience_level":     "entry",
		"created_at":           "2026-05-27T00:00:00+00:00",
		"application_deadline": nil,
	}
	rawArr, _ := json.Marshal([]any{row})
	postings, err := parseListing(rawArr, defaultSiteURL)
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}
	if !postings[0].AlwaysOpen {
		t.Error("null application_deadline did not produce AlwaysOpen=true")
	}
	if postings[0].ClosedAt != nil {
		t.Errorf("AlwaysOpen posting has ClosedAt = %v, want nil", postings[0].ClosedAt)
	}
}

// fmt import used only when debugging during test development.
var _ = fmt.Sprintf

func TestResolveAnonKeyFallsBackToBakedIn(t *testing.T) {
	t.Setenv(anonKeyEnvVar, "")
	if got := resolveAnonKey(); got != bakedInSupabaseAnonKey {
		t.Errorf("with empty env var, resolveAnonKey() = %q, want bakedInSupabaseAnonKey", got)
	}
}

func TestResolveAnonKeyOverridesViaEnvVar(t *testing.T) {
	t.Setenv(anonKeyEnvVar, "  overridden.key  ")
	if got := resolveAnonKey(); got != "overridden.key" {
		t.Errorf("with env var set, resolveAnonKey() = %q, want %q (trimmed)", got, "overridden.key")
	}
}

func TestAnyBucketKeepsITWithNoExperienceDemand(t *testing.T) {
	cases := []struct {
		name     string
		title    string
		position string
		want     bool
	}{
		// Clear IT signals — kept.
		{name: "Korean 개발자 in title", title: "Django 백엔드 개발자 모집", want: true},
		{name: "English engineer in title", title: "Frontend Engineer (full-time)", want: true},
		{name: "data scientist", title: "데이터 사이언티스트 채용", want: true},
		// IT signal but with 5+ year demand — dropped.
		{name: "engineer + 5년 이상", title: "백엔드 엔지니어 (5년 이상)", want: false},
		{name: "engineer + 시니어", title: "시니어 백엔드 엔지니어", want: false},
		{name: "engineer + 경력 5년", title: "프론트엔드 개발자 경력 5년", want: false},
		// IT signal + 경력 3년 — kept (parsed min=3 < 4).
		{name: "engineer + 경력 3년", title: "백엔드 개발자 경력 3년", want: true},
		// No IT signal — dropped, even with no experience demand.
		{name: "non-IT designer", title: "Office Interior Designer (Lead)", want: false},
		{name: "marketing role", title: "그로스 마케터 신입 모집", want: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := anyBucketKeeps(c.title, c.position); got != c.want {
				t.Errorf("anyBucketKeeps(%q, %q) = %v, want %v", c.title, c.position, got, c.want)
			}
		})
	}
}

func TestParseListingDropsAnyBucketWhenFilterFails(t *testing.T) {
	// Three any-bucket rows; only the one with a clear IT signal AND
	// no 5+ year demand should survive.
	body := []byte(`[
		{"id":1, "title":"Frontend Engineer (intern)", "position":"Frontend",
		 "experience_level":"any", "company_name":"Acme",
		 "created_at":"2026-05-26T00:00:00+00:00"},
		{"id":2, "title":"Office Interior Designer (Lead)", "position":"Design",
		 "experience_level":"any", "company_name":"BCorp",
		 "created_at":"2026-05-26T00:00:00+00:00"},
		{"id":3, "title":"Backend Engineer (5년 이상)", "position":"Backend",
		 "experience_level":"any", "company_name":"DCorp",
		 "created_at":"2026-05-26T00:00:00+00:00"}
	]`)
	postings, err := parseListing(body, defaultSiteURL)
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}
	if len(postings) != 1 {
		t.Fatalf("got %d postings, want 1 (only the clean IT any-bucket row should survive)", len(postings))
	}
	if postings[0].Title != "Frontend Engineer (intern)" {
		t.Errorf("wrong survivor: %q", postings[0].Title)
	}
	// `any` bucket should be tagged with "경력 무관" and newcomer-friendly.
	if !postings[0].Newcomer {
		t.Error("any-bucket survivor should be Newcomer=true")
	}
	if postings[0].CareerLevel != "경력 무관" {
		t.Errorf("CareerLevel = %q, want 경력 무관", postings[0].CareerLevel)
	}
	if postings[0].MaxCareer != anyBucketMaxYears {
		t.Errorf("MaxCareer = %d, want anyBucketMaxYears (%d)", postings[0].MaxCareer, anyBucketMaxYears)
	}
}

func TestFetchListing401ReportsRotatedKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"invalid api key"}`))
	}))
	defer srv.Close()

	s := newScraper(defaultSiteURL, srv.URL, 0)
	_, err := s.FetchListing(context.Background(), 0)
	if err == nil {
		t.Fatal("FetchListing did not error on 401")
	}
	msg := err.Error()
	for _, want := range []string{"401", "rotated", anonKeyEnvVar, "bakedInSupabaseAnonKey"} {
		if !strings.Contains(msg, want) {
			t.Errorf("401 error message missing %q\nfull message: %s", want, msg)
		}
	}
}
