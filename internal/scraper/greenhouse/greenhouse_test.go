package greenhouse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

func parseFixture(t *testing.T, name string, tenant Tenant) map[string]postingView {
	t.Helper()
	postings, err := parseListing(loadFixture(t, name), tenant)
	if err != nil {
		t.Fatalf("parseListing(%s): %v", name, err)
	}
	out := make(map[string]postingView, len(postings))
	for _, p := range postings {
		out[p.SourcePostingID] = postingView{
			source: p.Source, url: p.URL, title: p.Title, company: p.Company,
			newcomer: p.Newcomer, careerLevel: p.CareerLevel,
		}
	}
	return out
}

type postingView struct {
	source, url, title, company, careerLevel string
	newcomer                                 bool
}

// --- 당근 metadata path (regression: must match the old daangn scraper) ----

func TestParseDaangnMetadataKeepsFourShinip(t *testing.T) {
	got := parseFixture(t, "daangn_jobs.json", Daangn().t)
	if len(got) != 4 {
		t.Fatalf("daangn: want 4 신입 survivors, got %d (%v)", len(got), keys(got))
	}
	for id, p := range got {
		if p.source != "daangn" {
			t.Errorf("daangn id=%s: Source=%q, want daangn", id, p.source)
		}
		if !p.newcomer {
			t.Errorf("daangn id=%s: Newcomer=false, want true", id)
		}
		want := "https://team.daangn.com/jobs/" + id + "/"
		if p.url != want {
			t.Errorf("daangn id=%s: URL=%q, want %q", id, p.url, want)
		}
		if p.company == "" {
			t.Errorf("daangn id=%s: empty Company", id)
		}
	}
}

// --- krafton heuristic path -------------------------------------------------

func TestParseKraftonHeuristic(t *testing.T) {
	got := parseFixture(t, "krafton_jobs.json", Krafton().t)

	// The three AI-Research intern/engineer roles are dev + 신입-marked + Seoul.
	for _, id := range []string{"8125444002", "8574562002", "8401231002"} {
		if _, ok := got[id]; !ok {
			t.Errorf("krafton: expected keep id=%s missing (kept: %v)", id, keys(got))
		}
	}
	// Clear rejects: senior-years dev, 7년 dev, HR, Amsterdam, and the
	// contradictory "Jr. 팀원 (3~6년)" ops role (weak junior word + a real
	// mid-career floor + non-dev role).
	for _, id := range []string{"8360142002", "8524447002", "8480713002", "8561970002", "8505415002"} {
		if _, ok := got[id]; ok {
			t.Errorf("krafton: id=%s should be rejected but was kept", id)
		}
	}
	// "[Infra Div.] IT Engineer (경력 무관 / 계약직)" — 경력무관 newcomer marker +
	// Engineer + Seoul: a legitimate 신입-eligible infra keep under the broad scope.
	if _, ok := got["8503567002"]; !ok {
		t.Errorf("krafton: IT Engineer 경력무관 (8503567002) should be kept (kept: %v)", keys(got))
	}

	p, ok := got["8574562002"]
	if !ok {
		t.Fatal("krafton 8574562002 (Research Engineer intern) not kept")
	}
	if p.source != "krafton" {
		t.Errorf("Source=%q, want krafton", p.source)
	}
	// LinkAbsolute → Greenhouse's hosted-board absolute_url.
	wantURL := "https://job-boards.greenhouse.io/krafton/jobs/8574562002"
	if p.url != wantURL {
		t.Errorf("URL=%q, want %q", p.url, wantURL)
	}
	if p.company != "크래프톤" {
		t.Errorf("Company=%q, want 크래프톤", p.company)
	}
	if p.careerLevel != "인턴" {
		t.Errorf("CareerLevel=%q, want 인턴", p.careerLevel)
	}
}

// --- moloco: the intern-substring trap regression ---------------------------

func TestParseMolocoKeepsInternRejectsInternal(t *testing.T) {
	got := parseFixture(t, "moloco_jobs.json", Moloco().t)
	if _, ok := got["7635045003"]; !ok {
		t.Errorf("moloco: ML Engineer Intern (7635045003) should be kept (kept: %v)", keys(got))
	}
	// "Director, Internal Communications" — 'Internal' must NOT match the
	// intern marker, and it's a foreign senior non-dev role anyway.
	if _, ok := got["7715502003"]; ok {
		t.Error("moloco: 'Director, Internal Communications' wrongly kept (intern-substring trap)")
	}
}

// --- sendbird: LinkBoard URL strategy + non-dev intern reject ---------------

func TestParseSendbirdHeuristic(t *testing.T) {
	got := parseFixture(t, "sendbird_jobs.json", Sendbird().t)
	p, ok := got["8276676002"]
	if !ok {
		t.Fatalf("sendbird: AI Agent Engineer Intern (8276676002) should be kept (kept: %v)", keys(got))
	}
	// LinkBoard → canonical hosted-board URL (sendbird's absolute_url is a
	// custom careers page that does not deep-link).
	wantURL := "https://job-boards.greenhouse.io/sendbird/jobs/8276676002"
	if p.url != wantURL {
		t.Errorf("sendbird URL=%q, want %q", p.url, wantURL)
	}
	// "Forward Deployed Product Manager, Intern" — 신입-marked but not a dev role.
	if _, ok := got["8276047002"]; ok {
		t.Error("sendbird: Product Manager intern wrongly kept (not a dev role)")
	}
}

// --- marker unit tests ------------------------------------------------------

func TestHasNewcomerMarker(t *testing.T) {
	cases := []struct {
		title string
		want  bool
	}{
		{"Director, Internal Communications", false}, // intern ⊄ Internal
		{"International Marketing Lead", false},      // intern ⊄ International
		{"AI Agent Engineer, Intern", true},
		{"[AI Research Div.] Research Scientist Intern", true},
		{"Junior Backend Developer", true},
		{"주니어 데이터 엔지니어", true},
		{"신입 백엔드 개발자", true},
		{"Server Programmer (5년 이상)", false},
		{"Senior Software Engineer", false},
		{"Associate Software Engineer", true},
	}
	for _, c := range cases {
		if got := hasNewcomerMarker(c.title); got != c.want {
			t.Errorf("hasNewcomerMarker(%q)=%v, want %v", c.title, got, c.want)
		}
	}
}

func TestHasSeniorMarker(t *testing.T) {
	cases := []struct {
		title string
		want  bool
	}{
		{"Senior Software Engineer", true},
		{"Staff ML Engineer", true},
		{"시니어 백엔드 개발자", true},
		{"수석 연구원", true},
		{"Backend Engineer Intern", false},
		{"신입 개발자", false},
	}
	for _, c := range cases {
		if got := hasSeniorMarker(c.title); got != c.want {
			t.Errorf("hasSeniorMarker(%q)=%v, want %v", c.title, got, c.want)
		}
	}
}

func TestIsKorea(t *testing.T) {
	cases := []struct {
		loc  string
		want bool
	}{
		{"Seoul, Korea", true},
		{"Seoul, South Korea", true},
		{"Pangyo", true},
		{"서울", true},
		{"Menlo Park, California, United States", false},
		{"Amsterdam", false},
		{"Menlo Park, California, United States; Seoul, Korea", true},
		{"", false},
	}
	for _, c := range cases {
		if got := isKorea(c.loc); got != c.want {
			t.Errorf("isKorea(%q)=%v, want %v", c.loc, got, c.want)
		}
	}
}

// --- FetchListing: path / token / headers -----------------------------------

func TestFetchListingHitsTokenPathWithHeaders(t *testing.T) {
	var gotPath, gotUA, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUA = r.Header.Get("User-Agent")
		gotAccept = r.Header.Get("Accept")
		_, _ = w.Write(loadFixture(t, "krafton_jobs.json"))
	}))
	defer srv.Close()

	s := newScraper(Krafton().t, srv.URL, 0)
	postings, err := s.FetchListing(context.Background(), 0)
	if err != nil {
		t.Fatalf("FetchListing: %v", err)
	}
	if gotPath != "/v1/boards/krafton/jobs" {
		t.Errorf("path=%q, want /v1/boards/krafton/jobs", gotPath)
	}
	if gotUA != userAgent {
		t.Errorf("User-Agent=%q, want %q", gotUA, userAgent)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept=%q, want application/json", gotAccept)
	}
	if len(postings) == 0 {
		t.Error("FetchListing returned no postings")
	}
}

func TestCheckAccessAllowsOpenRobots(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /embed/\n"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	s := newScraper(Krafton().t, srv.URL, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.CheckAccess(ctx); err != nil {
		t.Errorf("CheckAccess: %v", err)
	}
}

func keys(m map[string]postingView) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
