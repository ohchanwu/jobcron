package daangn

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestParseListingFiltersToShinipIT asserts the filter behavior against
// the captured 42-job fixture (testdata/listing_fixture.json, pulled
// 2026-05-27): exactly 4 jobs should pass (Engineer=true AND Prior
// Experience contains 신입).
func TestParseListingFiltersToShinipIT(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "listing_fixture.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	postings, err := parseListing(body)
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}
	if len(postings) != 4 {
		t.Fatalf("filter passed %d postings, want 4", len(postings))
	}
	gotIDs := map[string]bool{}
	for _, p := range postings {
		gotIDs[p.SourcePostingID] = true
	}
	wantIDs := []string{"7562298003", "7689088003", "7668837003", "5248527003"}
	for _, id := range wantIDs {
		if !gotIDs[id] {
			t.Errorf("posting id=%s missing from filtered output", id)
		}
	}

	// Common assertions on every survivor.
	for i, p := range postings {
		if p.Source != "daangn" {
			t.Errorf("[%d] Source = %q, want daangn", i, p.Source)
		}
		if !p.Newcomer {
			t.Errorf("[%d] Newcomer = false, want true for 신입-relevant posting", i)
		}
		if p.Company == "" {
			t.Errorf("[%d] Company is empty", i)
		}
		if p.Location == "" {
			t.Errorf("[%d] Location is empty", i)
		}
		if p.URL == "" {
			t.Errorf("[%d] URL is empty", i)
		}
		// Description should be HTML-stripped — no '<' or '>' characters.
		if strings.ContainsAny(p.Description, "<>") {
			t.Errorf("[%d] Description still contains HTML: %q", i, p.Description[:80])
		}
		// PublishedAt must parse for every record.
		if p.PublishedAt == nil {
			t.Errorf("[%d] PublishedAt is nil", i)
		}
	}
}

func TestIsShinipIT(t *testing.T) {
	cases := []struct {
		name     string
		md       map[string]any
		wantPass bool
	}{
		{
			name:     "engineer + 신입",
			md:       map[string]any{"Engineer": true, "Prior Experience": "신입"},
			wantPass: true,
		},
		{
			name:     "engineer + 신입/경력",
			md:       map[string]any{"Engineer": true, "Prior Experience": "신입/경력"},
			wantPass: true,
		},
		{
			name:     "engineer + 경력 only",
			md:       map[string]any{"Engineer": true, "Prior Experience": "경력"},
			wantPass: false,
		},
		{
			name:     "non-engineer + 신입",
			md:       map[string]any{"Engineer": false, "Prior Experience": "신입"},
			wantPass: false,
		},
		{
			name:     "missing Engineer field",
			md:       map[string]any{"Prior Experience": "신입"},
			wantPass: false,
		},
		{
			name:     "missing Prior Experience",
			md:       map[string]any{"Engineer": true},
			wantPass: false,
		},
		{
			name:     "Engineer as string (not yes_no)",
			md:       map[string]any{"Engineer": "yes", "Prior Experience": "신입"},
			wantPass: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isShinipIT(c.md); got != c.wantPass {
				t.Errorf("isShinipIT = %v, want %v", got, c.wantPass)
			}
		})
	}
}

func TestExperienceBounds(t *testing.T) {
	cases := []struct {
		priorExp string
		wantMin  int
		wantMax  int
	}{
		{"신입", 0, 0},
		{"신입/경력", 0, 3},
		{"경력", 0, 0}, // safe default, never reached in practice
		{"  신입  ", 0, 0},
	}
	for _, c := range cases {
		gotMin, gotMax := experienceBounds(c.priorExp)
		if gotMin != c.wantMin || gotMax != c.wantMax {
			t.Errorf("experienceBounds(%q) = (%d,%d), want (%d,%d)",
				c.priorExp, gotMin, gotMax, c.wantMin, c.wantMax)
		}
	}
}

func TestNormalizeLocation(t *testing.T) {
	cases := []struct{ in, want string }{
		{"SEOUL", "서울"},
		{"  seoul  ", "서울"},
		{"Pangyo", "Pangyo"}, // pass-through for unknown locations
		{"", ""},
	}
	for _, c := range cases {
		if got := normalizeLocation(c.in); got != c.want {
			t.Errorf("normalizeLocation(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBuildTagsExposesAlternativeCivilianService(t *testing.T) {
	md := map[string]any{
		"Employment Type":              "정규직",
		"Alternative Civilian Service": true,
	}
	tags := buildTags(md)
	if len(tags) != 2 {
		t.Fatalf("got %d tags, want 2", len(tags))
	}
	gotEmployment, gotWelfare := false, false
	for _, t := range tags {
		if t.Category == "employment_type" && t.Name == "정규직" {
			gotEmployment = true
		}
		if t.Category == "welfare" && t.Name == "병역특례 가능" {
			gotWelfare = true
		}
	}
	if !gotEmployment {
		t.Error("Employment Type tag missing")
	}
	if !gotWelfare {
		t.Error("병역특례 welfare tag missing despite Alternative Civilian Service = true")
	}
}

func TestBuildTagsOmitsCivilianServiceWhenFalse(t *testing.T) {
	md := map[string]any{
		"Employment Type":              "정규직",
		"Alternative Civilian Service": false,
	}
	tags := buildTags(md)
	for _, tag := range tags {
		if tag.Category == "welfare" {
			t.Errorf("unexpected welfare tag %+v when Alternative Civilian Service = false", tag)
		}
	}
}

// TestFetchListingAgainstFakeGreenhouse exercises the full HTTP path
// against a fake Greenhouse server. Verifies path/User-Agent/Accept
// headers are right and the fake response is decoded correctly.
func TestFetchListingAgainstFakeGreenhouse(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "listing_fixture.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var gotPath, gotUA, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		// content=true is a query parameter; check the full URL.
		if r.URL.RawQuery != "content=true" {
			t.Errorf("query = %q, want content=true", r.URL.RawQuery)
		}
		gotUA = r.Header.Get("User-Agent")
		gotAccept = r.Header.Get("Accept")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	s := newScraper(defaultSiteURL, srv.URL, 0)
	postings, err := s.FetchListing(context.Background(), 0)
	if err != nil {
		t.Fatalf("FetchListing: %v", err)
	}
	if gotPath != "/v1/boards/daangn/jobs" {
		t.Errorf("path = %q, want /v1/boards/daangn/jobs", gotPath)
	}
	if !strings.HasPrefix(gotUA, "job-scraper/") {
		t.Errorf("User-Agent = %q, want job-scraper/* prefix", gotUA)
	}
	if !strings.Contains(gotAccept, "application/json") {
		t.Errorf("Accept = %q, want application/json", gotAccept)
	}
	if len(postings) != 4 {
		t.Errorf("got %d postings, want 4 after filtering", len(postings))
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

func TestParseListingHandlesNullApplicationDeadline(t *testing.T) {
	row := map[string]any{
		"id":                   1,
		"title":                "test",
		"location":             map[string]any{"name": "SEOUL"},
		"first_published":      "2026-05-27T00:00:00+09:00",
		"application_deadline": nil,
		"metadata": []map[string]any{
			{"name": "Engineer", "value": true, "value_type": "yes_no"},
			{"name": "Prior Experience", "value": "신입", "value_type": "single_select"},
			{"name": "Corporate", "value": "당근", "value_type": "single_select"},
		},
	}
	raw, _ := json.Marshal(map[string]any{"jobs": []any{row}})
	postings, err := parseListing(raw)
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}
	if len(postings) != 1 {
		t.Fatalf("got %d postings, want 1", len(postings))
	}
	if !postings[0].AlwaysOpen {
		t.Error("null application_deadline should produce AlwaysOpen=true")
	}
	if postings[0].ClosedAt != nil {
		t.Errorf("AlwaysOpen posting has ClosedAt = %v", postings[0].ClosedAt)
	}
}

func TestParseTimestampAccepts(t *testing.T) {
	cases := []string{
		"2026-05-27T00:05:19+00:00",
		"2026-05-27T00:05:19Z",
		"2026-05-27T00:05:19-08:00",
		"2026-05-27",
	}
	for _, in := range cases {
		got, ok := parseTimestamp(in)
		if !ok {
			t.Errorf("parseTimestamp(%q) returned !ok", in)
			continue
		}
		if got.IsZero() {
			t.Errorf("parseTimestamp(%q) returned zero time", in)
		}
		if got.Location() != time.UTC {
			t.Errorf("parseTimestamp(%q).Location = %v, want UTC", in, got.Location())
		}
	}
}

func TestStripHTMLHandlesGreenhouseEncoding(t *testing.T) {
	in := "<p>Hello   world &amp; friends&nbsp;here</p>"
	got := stripHTML(in)
	// stripHTML doesn't decode entities — caller does that first.
	if !strings.Contains(got, "Hello world") {
		t.Errorf("stripHTML(%q) = %q", in, got)
	}
}

func TestCheckAccessAllowsWhenBothRobotsClean(t *testing.T) {
	siteRobots := "User-Agent: *\nAllow: /\nDisallow: /preview/\n"
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			_, _ = w.Write([]byte(siteRobots))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer site.Close()
	apiRobots := "User-agent: *\nDisallow: /embed/\n"
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			_, _ = w.Write([]byte(apiRobots))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer api.Close()
	s := newScraper(site.URL, api.URL, 0)
	if err := s.CheckAccess(context.Background()); err != nil {
		t.Errorf("CheckAccess = %v, want nil", err)
	}
}

func TestCheckAccessRejectsApiHostDisallow(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("User-Agent: *\nAllow: /\n"))
	}))
	defer site.Close()
	// API robots disallows /v1/boards/.
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /v1/\n"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer api.Close()
	s := newScraper(site.URL, api.URL, 0)
	if err := s.CheckAccess(context.Background()); err == nil {
		t.Error("CheckAccess allowed scraping despite API Disallow")
	}
}
