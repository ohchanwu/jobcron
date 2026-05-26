package rallit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ohchanwu/job-scraper/internal/scraper"
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
	got, err := parseListing(readFixture(t, "listing.json"))
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("parseListing returned no postings")
	}
	first := got[0]
	if first.Source != Source {
		t.Errorf("Source = %q, want %q", first.Source, Source)
	}
	if first.SourcePostingID == "" {
		t.Error("SourcePostingID is empty")
	}
	if first.Title == "" {
		t.Error("Title is empty")
	}
	if first.Company == "" {
		t.Error("Company is empty")
	}
	if first.URL == "" {
		t.Error("URL is empty")
	}
	if !first.Newcomer {
		t.Error("Newcomer = false; listing was 신입-filtered server-side")
	}
}

func TestParseListingHandlesAlwaysOpenEndedSentinel(t *testing.T) {
	got, err := parseListing(readFixture(t, "listing.json"))
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}
	// Synthesize the always-open case via a deliberate fixture, since the
	// captured listing may or may not include one.
	body := []byte(`{
		"statusCode":"OK","message":"","errorCode":"UNKNOWN_ERROR",
		"data":{
			"pageNumber":1,"pageSize":1,"totalCount":1,"totalPage":1,
			"items":[{
				"id":99999,"title":"always open","companyName":"X","companyId":1,
				"jobLevels":["BEGINNER"],
				"startedAt":"2026-01-01","endedAt":"9999-12-31",
				"addressRegion":"GANGNAM","url":"https://www.rallit.com/positions/99999",
				"jobSkillKeywords":["Go"]
			}]
		}
	}`)
	out, err := parseListing(body)
	if err != nil {
		t.Fatalf("parseListing synthetic: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1", len(out))
	}
	if !out[0].AlwaysOpen {
		t.Error("AlwaysOpen = false, want true for 9999-12-31")
	}
	if out[0].ClosedAt != nil {
		t.Error("ClosedAt should be nil when AlwaysOpen")
	}
	// And just touch the real fixture so the test exercises both paths.
	_ = got
}

func TestParseListingDropsEpochStartedAtSentinel(t *testing.T) {
	body := []byte(`{
		"statusCode":"OK","message":"","errorCode":"UNKNOWN_ERROR",
		"data":{"pageNumber":1,"pageSize":1,"totalCount":1,"totalPage":1,
		"items":[{"id":1,"title":"t","companyName":"c","companyId":1,
		"startedAt":"1970-01-01","endedAt":"2026-06-01","url":"u",
		"jobSkillKeywords":[]}]}}`)
	out, err := parseListing(body)
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}
	if out[0].PublishedAt != nil {
		t.Errorf("PublishedAt = %v, want nil for the epoch sentinel", out[0].PublishedAt)
	}
}

func TestParseListingDatesArriveInUTC(t *testing.T) {
	body := []byte(`{
		"statusCode":"OK","message":"","errorCode":"UNKNOWN_ERROR",
		"data":{"pageNumber":1,"pageSize":1,"totalCount":1,"totalPage":1,
		"items":[{"id":1,"title":"t","companyName":"c","companyId":1,
		"startedAt":"2026-05-20","endedAt":"2026-06-20","url":"u",
		"jobSkillKeywords":[]}]}}`)
	out, err := parseListing(body)
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}
	if out[0].PublishedAt == nil {
		t.Fatal("PublishedAt is nil")
	}
	want := time.Date(2026, 5, 19, 15, 0, 0, 0, time.UTC) // 2026-05-20 KST = 15:00 UTC prior day
	if !out[0].PublishedAt.Equal(want) {
		t.Errorf("PublishedAt = %v, want %v", out[0].PublishedAt, want)
	}
}

func TestParseListingRejectsNonOKEnvelope(t *testing.T) {
	body := []byte(`{"statusCode":"BAD_PARAMETER","message":"oops","data":{},"errorCode":"UNKNOWN_ERROR"}`)
	if _, err := parseListing(body); err == nil {
		t.Fatal("parseListing accepted a non-OK envelope")
	}
}

func TestParseDetailComposesDescriptionAndStripsHTML(t *testing.T) {
	base := scraper.Posting{
		Source:          Source,
		SourcePostingID: "4210",
	}
	got, err := parseDetail(base, readFixture(t, "detail.json"))
	if err != nil {
		t.Fatalf("parseDetail: %v", err)
	}
	if got.Description == "" {
		t.Fatal("Description empty; expected composed HTML-stripped text")
	}
	if strings.Contains(got.Description, "<") || strings.Contains(got.Description, ">") {
		t.Errorf("Description still contains HTML tags: %q", got.Description[:120])
	}
	if got.Title == "" {
		t.Error("Title not populated from detail")
	}
}

func TestParseDetailIsAlwaysHiringRespectsFlag(t *testing.T) {
	body := []byte(`{"statusCode":"OK","message":"","data":{
		"id":1,"title":"t","companyName":"c","companyId":1,
		"isAlwaysHiring":true,"startedAt":"2026-05-01","endedAt":"2026-06-01",
		"description":"<p>hi</p>","jobSkillKeywords":[]
	}}`)
	got, err := parseDetail(scraper.Posting{Source: Source}, body)
	if err != nil {
		t.Fatalf("parseDetail: %v", err)
	}
	if !got.AlwaysOpen {
		t.Error("AlwaysOpen = false, want true when isAlwaysHiring set")
	}
	if got.ClosedAt != nil {
		t.Error("ClosedAt should be nil when AlwaysOpen, regardless of endedAt")
	}
}

func TestStripHTMLTagsCollapsesAdjacentRuns(t *testing.T) {
	in := "<p>hello</p><br><strong>world</strong>"
	out := collapseWhitespace(stripHTMLTags(in))
	if out != "hello world" {
		t.Errorf("got %q, want %q", out, "hello world")
	}
}
