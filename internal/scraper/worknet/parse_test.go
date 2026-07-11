package worknet

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/scraper"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

func TestParseListingExtractsCoreFields(t *testing.T) {
	got, err := parseListing(readFixture(t, "listing.xml"))
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	p := got[0]
	if p.Source != Source {
		t.Errorf("Source = %q, want %q", p.Source, Source)
	}
	if p.SourcePostingID != "K2026050001" {
		t.Errorf("SourcePostingID = %q", p.SourcePostingID)
	}
	if p.Title != "로봇/로보틱스 S/W 엔지니어 신입 모집" {
		t.Errorf("Title = %q", p.Title)
	}
	if p.Company != "테솔로" {
		t.Errorf("Company = %q", p.Company)
	}
	if p.Location != "인천 연수구" {
		t.Errorf("Location = %q", p.Location)
	}
	if !p.Newcomer {
		t.Error("Newcomer = false, want true (listing was 신입-filtered server-side)")
	}
	if p.EducationName != "대학교 졸업" {
		t.Errorf("EducationName = %q", p.EducationName)
	}
	if p.URL == "" {
		t.Error("URL is empty")
	}
}

func TestParseListingHandlesAlwaysOpenSentinel(t *testing.T) {
	got, err := parseListing(readFixture(t, "listing.xml"))
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}
	// K2026050002 has closeDt = 99991231 — must map to AlwaysOpen.
	var found bool
	for _, p := range got {
		if p.SourcePostingID == "K2026050002" {
			found = true
			if !p.AlwaysOpen {
				t.Error("AlwaysOpen = false, want true for 99991231")
			}
			if p.ClosedAt != nil {
				t.Errorf("ClosedAt = %v, want nil for 99991231", p.ClosedAt)
			}
		}
	}
	if !found {
		t.Fatal("did not find K2026050002 in parsed listing")
	}
}

func TestParseListingDatesArriveInUTC(t *testing.T) {
	got, err := parseListing(readFixture(t, "listing.xml"))
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}
	p := got[0]
	if p.PublishedAt == nil {
		t.Fatal("PublishedAt is nil; expected 20260520 parsed")
	}
	// 20260520 KST = 20260519 15:00 UTC
	want := time.Date(2026, 5, 19, 15, 0, 0, 0, time.UTC)
	if !p.PublishedAt.Equal(want) {
		t.Errorf("PublishedAt = %v, want %v", p.PublishedAt, want)
	}
	if p.ClosedAt == nil {
		t.Fatal("ClosedAt is nil; expected 20260620 parsed")
	}
}

func TestParseListingSkipsEntriesWithoutAuthNo(t *testing.T) {
	body := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<wantedRoot>
  <total>2</total>
  <wanted>
    <wantedAuthNo></wantedAuthNo>
    <title>blank-id row</title>
  </wanted>
  <wanted>
    <wantedAuthNo>K2026050099</wantedAuthNo>
    <title>good row</title>
    <company>OK</company>
  </wanted>
</wantedRoot>`)
	got, err := parseListing(body)
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (blank-id row should be skipped)", len(got))
	}
	if got[0].SourcePostingID != "K2026050099" {
		t.Errorf("kept the wrong row: %q", got[0].SourcePostingID)
	}
}

func TestParseDetailEnrichesDescription(t *testing.T) {
	base := scraper.Posting{
		Source:          Source,
		SourcePostingID: "K2026050001",
		Title:           "로봇/로보틱스 S/W 엔지니어 신입 모집",
		Company:         "테솔로",
		Location:        "인천 연수구",
	}
	got, err := parseDetail(base, readFixture(t, "detail.xml"))
	if err != nil {
		t.Fatalf("parseDetail: %v", err)
	}
	if got.Description == "" {
		t.Fatal("Description is empty; expected jobcont contents")
	}
	if !contains(got.Description, "ROS") {
		t.Errorf("Description missing expected substring: %q", got.Description)
	}
	// Detail address should overwrite the listing's coarser Location.
	if got.Location != "인천 연수구 컨벤시아대로165, 26층 브이454" {
		t.Errorf("Location did not pick up detail address: %q", got.Location)
	}
}

func TestParseListingMalformedXMLIsAnError(t *testing.T) {
	_, err := parseListing([]byte("<wantedRoot><wanted></<broken/>"))
	if err == nil {
		t.Fatal("parseListing accepted malformed XML")
	}
}

// contains is a tiny strings.Contains shim to avoid yet another import in
// test files; keeps the assertion read like English.
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
