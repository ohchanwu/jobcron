package jumpit

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/scraper"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func TestParseListing(t *testing.T) {
	postings, err := parseListing(readFixture(t, "listing.json"))
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}
	if len(postings) != 3 {
		t.Fatalf("got %d postings, want 3", len(postings))
	}

	kst := time.FixedZone("KST", 9*60*60)

	// Position 0 — a 신입 posting; full listing-level field mapping.
	p0 := postings[0]
	if p0.Source != "jumpit" {
		t.Errorf("Source = %q, want jumpit", p0.Source)
	}
	if p0.SourcePostingID != "53688979" {
		t.Errorf("SourcePostingID = %q, want 53688979", p0.SourcePostingID)
	}
	if p0.URL != "https://jumpit.saramin.co.kr/position/53688979" {
		t.Errorf("URL = %q", p0.URL)
	}
	if p0.Title != "B2B 프로젝트 개발팀 신입" {
		t.Errorf("Title = %q", p0.Title)
	}
	if p0.Company != "에스피에이치" {
		t.Errorf("Company = %q", p0.Company)
	}
	if p0.Location != "서울 마포구" {
		t.Errorf("Location = %q, want 서울 마포구", p0.Location)
	}
	if !p0.Newcomer {
		t.Errorf("Newcomer = false, want true")
	}
	if p0.CareerLevel != "신입" {
		t.Errorf("CareerLevel = %q, want 신입", p0.CareerLevel)
	}
	wantStacks := []string{"Git", "Java", "React", "Spring", "AI/인공지능"}
	if !reflect.DeepEqual(p0.StackTags, wantStacks) {
		t.Errorf("StackTags = %v, want %v", p0.StackTags, wantStacks)
	}
	if p0.AlwaysOpen {
		t.Errorf("AlwaysOpen = true, want false")
	}
	wantClosed := time.Date(2026, 5, 20, 23, 59, 59, 0, kst)
	if p0.ClosedAt == nil || !p0.ClosedAt.Equal(wantClosed) {
		t.Errorf("ClosedAt = %v, want %v", p0.ClosedAt, wantClosed)
	}
	if p0.Description != "" {
		t.Errorf("Description = %q, want empty (listing has no detail text)", p0.Description)
	}
	if p0.RawJSON == "" {
		t.Errorf("RawJSON is empty, want the raw position object")
	}

	// Position 1 — career-range label derivation.
	p1 := postings[1]
	if p1.Newcomer {
		t.Errorf("p1.Newcomer = true, want false")
	}
	if p1.MinCareer != 1 || p1.MaxCareer != 15 {
		t.Errorf("p1 career = %d-%d, want 1-15", p1.MinCareer, p1.MaxCareer)
	}
	if p1.CareerLevel != "1-15년" {
		t.Errorf("p1.CareerLevel = %q, want 1-15년", p1.CareerLevel)
	}

	// Position 2 — alwaysOpen (null closedAt) and multi-location join.
	p2 := postings[2]
	if !p2.AlwaysOpen {
		t.Errorf("p2.AlwaysOpen = false, want true")
	}
	if p2.ClosedAt != nil {
		t.Errorf("p2.ClosedAt = %v, want nil for an alwaysOpen posting", p2.ClosedAt)
	}
	if p2.Location != "서울 강남구, 경기 성남시 분당구" {
		t.Errorf("p2.Location = %q, want the two locations joined", p2.Location)
	}
}

func TestParseListingRejectsInvalidJSON(t *testing.T) {
	if _, err := parseListing([]byte("this is not json")); err == nil {
		t.Fatal("parseListing(invalid) = nil error, want an error")
	}
}

func TestParseDetail(t *testing.T) {
	listing, err := parseListing(readFixture(t, "listing.json"))
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}
	base := listing[0] // posting 53688979 — matches detail.json

	got, err := parseDetail(base, readFixture(t, "detail.json"))
	if err != nil {
		t.Fatalf("parseDetail: %v", err)
	}

	// Identity and listing-derived fields are preserved, not overwritten.
	if got.SourcePostingID != "53688979" || got.Source != "jumpit" ||
		got.URL != base.URL || got.Title != base.Title || got.ClosedAt != base.ClosedAt {
		t.Errorf("listing-level fields not preserved through parseDetail")
	}

	// Location is upgraded to the detail endpoint's full address.
	if got.Location != "서울 마포구 마포대로92, A동 3층" {
		t.Errorf("Location = %q, want the detail full address", got.Location)
	}

	if got.Education == nil || *got.Education != 8 {
		t.Errorf("Education = %v, want 8", got.Education)
	}
	if got.EducationName != "대학교졸업(4년) 이상" {
		t.Errorf("EducationName = %q", got.EducationName)
	}

	kst := time.FixedZone("KST", 9*60*60)
	wantPub := time.Date(2026, 4, 21, 0, 0, 0, 0, kst)
	if got.PublishedAt == nil || !got.PublishedAt.Equal(wantPub) {
		t.Errorf("PublishedAt = %v, want %v", got.PublishedAt, wantPub)
	}

	wantTags := []scraper.Tag{
		{ID: "com_143", Name: "연봉상승률 15% 이상", Category: "salary"},
		{ID: "285", Name: "휴가비 지원", Category: "welfare"},
		{ID: "com_126", Name: "5호선 역세권 기업", Category: "subway"},
		{ID: "com_147", Name: "평균연봉 6,000 이상", Category: "salary"},
	}
	if !reflect.DeepEqual(got.Tags, wantTags) {
		t.Errorf("Tags =\n %+v\nwant\n %+v", got.Tags, wantTags)
	}

	// techStacks normalized from the detail object-array shape into []string.
	wantStacks := []string{"Git", "Java", "React", "Spring", "AI/인공지능"}
	if !reflect.DeepEqual(got.StackTags, wantStacks) {
		t.Errorf("StackTags = %v, want %v", got.StackTags, wantStacks)
	}

	// Description = the five free-text fields joined by \n\n, in fixed order.
	order := []string{
		"Story Place Human", // serviceInfo
		"Google Maps API",   // responsibility
		"RestFul API",       // qualifications
		"Claude Code",       // preferredRequirements
		"법인카드",              // welfares
	}
	for _, sub := range order {
		if !strings.Contains(got.Description, sub) {
			t.Errorf("Description missing %q", sub)
		}
	}
	for i := 1; i < len(order); i++ {
		if strings.Index(got.Description, order[i-1]) >= strings.Index(got.Description, order[i]) {
			t.Errorf("Description out of order: %q must precede %q", order[i-1], order[i])
		}
	}
}

func TestParseDetailRejectsInvalidJSON(t *testing.T) {
	if _, err := parseDetail(scraper.Posting{}, []byte("not json")); err == nil {
		t.Fatal("parseDetail(invalid) = nil error, want an error")
	}
}
