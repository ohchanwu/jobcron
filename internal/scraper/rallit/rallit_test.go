package rallit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestFetchListingFiltersToDeveloperJobGroup locks in the server-side 직군
// filter. Without jobGroup=DEVELOPER, 랠릿 returns every 신입-level role
// regardless of function (marketing / design / PM / HR), which leaked non-dev
// 모두닥 postings into the briefing. The DEVELOPER group is 랠릿's umbrella for
// all tech roles (SWE + 데이터 + AI + 보안 + DevOps + QA).
func TestFetchListingFiltersToDeveloperJobGroup(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"statusCode":"OK","data":{"items":[],"totalCount":0,"pageNumber":1,"pageSize":50,"totalPage":0}}`))
	}))
	defer srv.Close()

	s := newScraper(srv.URL, 0)
	if _, err := s.FetchListing(context.Background(), 0); err != nil {
		t.Fatalf("FetchListing: %v", err)
	}
	if !strings.Contains(gotQuery, "jobGroup=DEVELOPER") {
		t.Errorf("listing query missing the dev 직군 filter jobGroup=DEVELOPER; got %q", gotQuery)
	}
	// Regression guard: the career-level filter must still be present.
	if !strings.Contains(gotQuery, "jobLevel=") {
		t.Errorf("listing query unexpectedly dropped jobLevel; got %q", gotQuery)
	}
}
