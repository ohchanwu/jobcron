package naver

import (
	"os"
	"path/filepath"
	"testing"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

func TestParseListingFiltersToNewcomerCodes(t *testing.T) {
	got, err := parseListing(readFixture(t, "listing.json"))
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}
	// Every kept posting must have an entTypeCd of either 0010 (신입) or
	// 0030 (무관). Other codes (0020 경력) must be filtered out.
	for _, p := range got {
		if p.CareerLevel != "신입" && p.CareerLevel != "무관" {
			t.Errorf("kept a posting with CareerLevel %q; expected 신입 or 무관", p.CareerLevel)
		}
		if p.Source != Source {
			t.Errorf("Source = %q, want %q", p.Source, Source)
		}
		if !p.Newcomer {
			t.Errorf("Newcomer = false on %q; should be true after filter", p.Title)
		}
	}
}

func TestParseListingRejectsNonOKResult(t *testing.T) {
	body := []byte(`{"result":"N","totalSize":0,"list":[]}`)
	if _, err := parseListing(body); err == nil {
		t.Fatal("parseListing accepted result != Y")
	}
}

func TestParseListingHandlesEmptyList(t *testing.T) {
	body := []byte(`{"result":"Y","totalSize":0,"list":[]}`)
	got, err := parseListing(body)
	if err != nil {
		t.Fatalf("parseListing empty: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestParseListingDatesArriveInUTC(t *testing.T) {
	body := []byte(`{"result":"Y","totalSize":1,"list":[{
		"annoId":1,"annoSubject":"신입 채용","sysCompanyCdNm":"NAVER",
		"entTypeCd":"0010","entTypeCdNm":"신입",
		"classCdNm":"Tech","subJobCdNm":"Software Engineering",
		"staYmd":"20260520","endYmd":"20260620",
		"jobDetailLink":"https://x"
	}]}`)
	got, err := parseListing(body)
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	p := got[0]
	if p.PublishedAt == nil || p.PublishedAt.UTC().Format("20060102") != "20260519" {
		t.Errorf("PublishedAt = %v, expected 2026-05-19 UTC (20260520 KST)", p.PublishedAt)
	}
	if p.ClosedAt == nil || p.ClosedAt.UTC().Format("20060102") != "20260619" {
		t.Errorf("ClosedAt = %v", p.ClosedAt)
	}
}

func TestParseListingMalformedJSONErrors(t *testing.T) {
	if _, err := parseListing([]byte("not json")); err == nil {
		t.Fatal("parseListing accepted invalid JSON")
	}
}

func TestComposeStubDescriptionJoinsAndSkipsBlanks(t *testing.T) {
	r := wantedRow{
		ClassCdNm:   "Tech",
		SubJobCdNm:  "Frontend Development",
		EmpTypeCdNm: "정규직",
		EntTypeCdNm: "신입",
	}
	got := composeStubDescription(r)
	want := "Tech · Frontend Development · 정규직 · 신입"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
