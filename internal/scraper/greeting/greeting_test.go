package greeting

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const testOrigin = "https://cashwalk12.career.greetinghr.com"

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

func parseFixture(t *testing.T, name string, te tenant, origin string) map[string]scraper2Posting {
	t.Helper()
	postings, err := parseBoard(loadFixture(t, name), te, origin)
	if err != nil {
		t.Fatalf("parseBoard(%s): %v", name, err)
	}
	out := make(map[string]scraper2Posting, len(postings))
	for _, p := range postings {
		out[p.SourcePostingID] = scraper2Posting{
			source: p.Source, url: p.URL, title: p.Title, company: p.Company,
			newcomer: p.Newcomer, careerLevel: p.CareerLevel, location: p.Location,
		}
	}
	return out
}

type scraper2Posting struct {
	source, url, title, company, careerLevel, location string
	newcomer                                           bool
}

// --- cashwalk12: structured 개발/데이터 occupation keeps, non-dev rejects ----

func TestParseCashwalkBoard(t *testing.T) {
	got := parseFixture(t, "cashwalk12_board.html", tenant{slug: "cashwalk12", company: "넛지헬스케어"}, testOrigin)

	// dev 신입 keeps: 백엔드/Flutter/iOS 인턴 (occ 개발), 데이터엔지니어 인턴 (occ 데이터), QA 병특 (occ 개발)
	for _, id := range []string{"cashwalk12-78138", "cashwalk12-30813", "cashwalk12-56673", "cashwalk12-170066", "cashwalk12-33761"} {
		if _, ok := got[id]; !ok {
			t.Errorf("expected keep %s missing (kept: %v)", id, keys(got))
		}
	}
	// rejects: Growth Marketing Intern @ Seattle (non-dev + non-Korea), 커뮤니티운영
	// (마케팅), 서비스기획 (기획), 인재등록 (non-job).
	for _, id := range []string{"cashwalk12-174306", "cashwalk12-30821", "cashwalk12-30704", "cashwalk12-135514"} {
		if _, ok := got[id]; ok {
			t.Errorf("%s should be rejected but was kept", id)
		}
	}

	p, ok := got["cashwalk12-78138"]
	if !ok {
		t.Fatal("백엔드개발 채용전환형 인턴 (78138) not kept")
	}
	if p.source != "greeting" {
		t.Errorf("Source=%q, want greeting", p.source)
	}
	if p.url != testOrigin+"/ko/o/78138" {
		t.Errorf("URL=%q, want %s/ko/o/78138", p.url, testOrigin)
	}
	if p.company != "넛지헬스케어" {
		t.Errorf("Company=%q, want 넛지헬스케어", p.company)
	}
	if p.careerLevel != "인턴" {
		t.Errorf("CareerLevel=%q, want 인턴", p.careerLevel)
	}
	if !p.newcomer {
		t.Error("Newcomer=false, want true")
	}
}

// --- realworld: keyword fallback (occ Tech/Product) + Business reject -------

func TestParseRealworldBoard(t *testing.T) {
	got := parseFixture(t, "realworld_board.html", tenant{slug: "realworld", company: "RLWRLD"}, "https://realworld.career.greetinghr.com")
	// AI Research Engineer (occ Tech/Product → kept via HasDevKeyword "Engineer").
	if _, ok := got["realworld-195922"]; !ok {
		t.Errorf("AI Research Engineer (195922) should be kept (kept: %v)", keys(got))
	}
	// AI strategy intern (occ Business → not dev-occupation, no dev keyword).
	if _, ok := got["realworld-168019"]; ok {
		t.Error("AI strategy intern (168019) wrongly kept — not a dev role")
	}
}

// --- estfamily: 개발 occupation + DevOps via keyword (occ 기술) -------------

func TestParseEstfamilyBoard(t *testing.T) {
	got := parseFixture(t, "estfamily_board.html", tenant{slug: "estfamily", company: "이스트소프트"}, "https://estfamily.career.greetinghr.com")
	for _, id := range []string{"estfamily-204268", "estfamily-107367"} {
		if _, ok := got[id]; !ok {
			t.Errorf("expected dev keep %s missing (kept: %v)", id, keys(got))
		}
	}
	if _, ok := got["estfamily-217565"]; ok {
		t.Error("non-dev opening 217565 wrongly kept")
	}
}

// --- supercent: occupation deny-list (사업개발/PM, 영상제작) regression -------

func TestParseSupercentBoard(t *testing.T) {
	got := parseFixture(t, "supercent_board.html", tenant{slug: "supercent", company: "슈퍼센트"}, "https://supercent.career.greetinghr.com")
	// keeps: 데이터 사이언티스트 (occ DS), 데이터 애널리틱스 엔지니어 (occ 데이터 엔지니어)
	for _, id := range []string{"supercent-184859", "supercent-216857"} {
		if _, ok := got[id]; !ok {
			t.Errorf("expected data keep %s missing (kept: %v)", id, keys(got))
		}
	}
	// rejects: occ 영상제작 (모바일 게임 광고 영상 기획자 — bare 모바일 must not win),
	// occ 사업개발/PM (모바일 게임 사업 PM, 퍼블리싱 매니저 — 개발 substring must not win).
	for _, id := range []string{"supercent-196630", "supercent-184787", "supercent-202460"} {
		if _, ok := got[id]; ok {
			t.Errorf("non-dev opening %s wrongly kept", id)
		}
	}
}

// --- classify gates ---------------------------------------------------------

func pos(careerType, occ, place string) position {
	p := position{}
	p.JobPositionCareer = &struct {
		CareerFrom *int   `json:"careerFrom"`
		CareerTo   *int   `json:"careerTo"`
		CareerType string `json:"careerType"`
	}{CareerType: careerType}
	p.WorkspaceOccupation = &struct {
		Occupation string `json:"occupation"`
	}{Occupation: occ}
	pl := place
	p.WorkspacePlace = &struct {
		Place       *string `json:"place"`
		Location    *string `json:"location"`
		DetailPlace *string `json:"detailPlace"`
	}{Place: &pl}
	return p
}

func TestQualifiesGates(t *testing.T) {
	cases := []struct {
		name       string
		title, occ string
		ct, place  string
		want       bool
	}{
		{"newcomer dev seoul", "백엔드개발 인턴", "개발", careerNewComer, "서울특별시 강남구", true},
		{"notmatter data seoul", "데이터엔지니어", "데이터", careerNotMatter, "대한민국 서울", true},
		{"experienced dev rejected", "백엔드 개발자", "개발", careerExperienced, "서울", false},
		{"newcomer marketing rejected", "그로스 마케터", "마케팅", careerNewComer, "서울", false},
		{"newcomer dev non-korea rejected", "ML Engineer Intern", "개발", careerNewComer, "Seattle, WA", false},
		{"tech occ keyword fallback", "AI Research Engineer", "Tech/Product", careerNotMatter, "서울", true},
	}
	for _, c := range cases {
		if got := qualifies(c.title, pos(c.ct, c.occ, c.place)); got != c.want {
			t.Errorf("%s: qualifies=%v, want %v", c.name, got, c.want)
		}
	}
}

func TestIsNonJobPosting(t *testing.T) {
	for _, s := range []string{"[넛지헬스케어] 인재등록", "S/W 개발자 지인 추천제도", "상시 인재풀 등록"} {
		if !isNonJobPosting(s) {
			t.Errorf("isNonJobPosting(%q)=false, want true", s)
		}
	}
	if isNonJobPosting("백엔드개발 채용전환형 인턴") {
		t.Error("real opening wrongly flagged as non-job")
	}
}

func TestNormalizeLocation(t *testing.T) {
	cases := map[string]string{
		"대한민국 서울특별시 강남구 역삼로1길 8": "서울특별시 강남구",
		"서울특별시 강남구 역삼로 109":      "서울특별시 강남구",
		"Seoul, Korea": "Seoul, Korea",
		"":             "",
	}
	for in, want := range cases {
		if got := normalizeLocation(in); got != want {
			t.Errorf("normalizeLocation(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestOriginOf(t *testing.T) {
	if got := originOf("https://cashwalk12.career.greetinghr.com/ko/home", "x"); got != "https://cashwalk12.career.greetinghr.com" {
		t.Errorf("originOf landing=%q", got)
	}
	if got := originOf("https://www.musinsacareers.com/ko/home", "x"); got != "https://www.musinsacareers.com" {
		t.Errorf("originOf custom-domain=%q", got)
	}
	if got := originOf("::bad::", "fallback.host"); got != "https://fallback.host" {
		t.Errorf("originOf fallback=%q", got)
	}
}

// --- FetchListing: slug loop, aggregation, origin-derived URLs --------------

func TestFetchListingAggregatesAndBuildsURLs(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		_, _ = w.Write(loadFixture(t, "cashwalk12_board.html"))
	}))
	defer srv.Close()

	s := newScraper([]tenant{{slug: "cashwalk12", company: "넛지헬스케어"}}, 0)
	s.origin = func(tenant) string { return srv.URL }

	postings, err := s.FetchListing(context.Background(), 0)
	if err != nil {
		t.Fatalf("FetchListing: %v", err)
	}
	if len(postings) != 5 {
		t.Errorf("got %d postings, want 5 dev 신입", len(postings))
	}
	if gotUA != userAgent {
		t.Errorf("User-Agent=%q, want %q", gotUA, userAgent)
	}
	for _, p := range postings {
		if p.URL != srv.URL+"/ko/o/"+after(p.SourcePostingID, "cashwalk12-") {
			t.Errorf("URL %q not built from final origin", p.URL)
		}
	}
}

func TestCheckAccessAllowsBoardPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			_, _ = w.Write([]byte("User-agent: *\nAllow: /\nDisallow: /m/*\nDisallow: /a/*\n"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	s := newScraper([]tenant{{slug: "cashwalk12"}}, 0)
	s.origin = func(tenant) string { return srv.URL }
	if err := s.CheckAccess(context.Background()); err != nil {
		t.Errorf("CheckAccess: %v", err)
	}
}

func after(s, prefix string) string {
	if len(s) > len(prefix) {
		return s[len(prefix):]
	}
	return s
}

func keys(m map[string]scraper2Posting) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
