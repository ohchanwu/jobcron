package alio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

const testBase = "https://job.alio.go.kr"

func TestParseListingExtractsRealPostings(t *testing.T) {
	got, err := parseListing(readFixture(t, "listing.html"), testBase)
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("parseListing returned no postings")
	}
	for _, p := range got {
		if p.Source != Source {
			t.Errorf("Source = %q, want %q", p.Source, Source)
		}
		if p.SourcePostingID == "" {
			t.Error("SourcePostingID is empty")
		}
		if p.Title == "" {
			t.Error("Title is empty")
		}
		if !p.Newcomer {
			t.Errorf("Newcomer = false on %q; server-side filter should have implied true", p.Title)
		}
		if p.CareerLevel != "신입" {
			t.Errorf("CareerLevel = %q, want 신입", p.CareerLevel)
		}
		if !strings.HasPrefix(p.URL, testBase+"/recruitview.do?idx=") {
			t.Errorf("URL = %q does not point at the public detail page", p.URL)
		}
		if !strings.HasSuffix(p.URL, p.SourcePostingID) {
			t.Errorf("URL %q does not end with the posting id %q", p.URL, p.SourcePostingID)
		}
	}
}

func TestParseListingPullsTheRowFieldsInOrder(t *testing.T) {
	// A hand-built minimal row exercising the exact <tr><td>...</td></tr>
	// shape we depend on. If this breaks, the upstream HTML changed.
	body := []byte(`
<table><tbody>
<tr>
  <td>2147</td>
  <td class="left"><a href="/recruitview.do?idx=300736" target="_blank"/>2026년 한국폴리텍대학 신입 채용</a></td>
  <td>학교법인한국폴리텍</td>
  <td>   경북 </td>
  <td>   무기계약직 </td>
  <td>2026.05.26</td>
  <td>26.06.02<br/><span class="orange" style="font-weight: bold;">D-6</span></td>
  <td><span class="orange"> 진행중</span></td>
</tr>
</tbody></table>`)
	got, err := parseListing(body, testBase)
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	p := got[0]
	if p.SourcePostingID != "300736" {
		t.Errorf("SourcePostingID = %q", p.SourcePostingID)
	}
	if p.Title != "2026년 한국폴리텍대학 신입 채용" {
		t.Errorf("Title = %q", p.Title)
	}
	if p.Company != "학교법인한국폴리텍" {
		t.Errorf("Company = %q", p.Company)
	}
	if p.Location != "경북" {
		t.Errorf("Location = %q (whitespace not collapsed)", p.Location)
	}
	if p.PublishedAt == nil {
		t.Fatal("PublishedAt is nil")
	}
	// 2026.05.26 KST midnight = 2026.05.25 15:00 UTC
	wantPosted := time.Date(2026, 5, 25, 15, 0, 0, 0, time.UTC)
	if !p.PublishedAt.Equal(wantPosted) {
		t.Errorf("PublishedAt = %v, want %v", p.PublishedAt, wantPosted)
	}
	if p.ClosedAt == nil {
		t.Fatal("ClosedAt is nil")
	}
	// 26.06.02 KST midnight = 2026.06.01 15:00 UTC
	wantClosed := time.Date(2026, 6, 1, 15, 0, 0, 0, time.UTC)
	if !p.ClosedAt.Equal(wantClosed) {
		t.Errorf("ClosedAt = %v, want %v", p.ClosedAt, wantClosed)
	}
	if !strings.Contains(p.Description, "무기계약직") {
		t.Errorf("Description %q does not include the employment type stub", p.Description)
	}
}

func TestParseListingSkipsRowsWithoutAnchor(t *testing.T) {
	body := []byte(`
<table><tbody>
<tr><th>row#</th><th>title</th></tr>
<tr><td>1</td><td>not a posting (no anchor)</td></tr>
<tr><td>2</td><td><a href="/recruitview.do?idx=99">good</a></td><td>Acme</td></tr>
</tbody></table>`)
	got, err := parseListing(body, testBase)
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (the row without anchor must be skipped)", len(got))
	}
	if got[0].SourcePostingID != "99" {
		t.Errorf("kept wrong row: %q", got[0].SourcePostingID)
	}
}

func TestParseListingHandlesEmptyBody(t *testing.T) {
	got, err := parseListing([]byte(""), testBase)
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0 on empty input", len(got))
	}
}

func TestParseClosedDateHandlesTrailingBadge(t *testing.T) {
	// After cleanText strips tags and collapses whitespace, the closing
	// cell looks like "26.06.02 D-6". The badge must not break parsing.
	t1, ok := parseClosedDate("26.06.02 D-6")
	if !ok {
		t.Fatal("parseClosedDate failed on badge-prefixed input")
	}
	t2, ok := parseClosedDate("26.06.02")
	if !ok || !t1.Equal(t2) {
		t.Errorf("badge handling diverged from plain date: %v vs %v", t1, t2)
	}
}

func TestParseClosedDateRejectsGarbage(t *testing.T) {
	for _, s := range []string{"", "not a date", "999-999-999"} {
		if _, ok := parseClosedDate(s); ok {
			t.Errorf("parseClosedDate accepted %q", s)
		}
	}
}

func TestParsePostedDateUTCConversion(t *testing.T) {
	got, ok := parsePostedDate("2026.05.20")
	if !ok {
		t.Fatal("parsePostedDate failed on valid input")
	}
	// 2026.05.20 KST midnight = 2026.05.19 15:00 UTC
	want := time.Date(2026, 5, 19, 15, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExtractCellsStripsInnerTags(t *testing.T) {
	row := []byte(`<tr><td>simple</td><td><span class="x">  wrapped &nbsp; text  </span></td></tr>`)
	cells := extractCells(row)
	if len(cells) != 2 {
		t.Fatalf("got %d cells, want 2", len(cells))
	}
	if cells[0] != "simple" {
		t.Errorf("cell[0] = %q", cells[0])
	}
	if cells[1] != "wrapped text" {
		t.Errorf("cell[1] = %q (whitespace + nbsp not collapsed)", cells[1])
	}
}
