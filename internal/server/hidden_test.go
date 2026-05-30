package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestHiddenPageListsMutedPostings verifies /hidden lists every muted posting
// and nothing else.
func TestHiddenPageListsMutedPostings(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	ctx := context.Background()
	mutedID := mustUpsert(t, st, listingPosting("muted", "숨긴 공고 제목"))
	mustUpsert(t, st, listingPosting("visible", "안 숨긴 공고 제목"))
	if err := st.SetNotInterested(ctx, mutedID, time.Now()); err != nil {
		t.Fatalf("SetNotInterested: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/hidden", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "숨긴 공고 제목") {
		t.Error("/hidden does not list the muted posting")
	}
	if strings.Contains(body, "안 숨긴 공고 제목") {
		t.Error("/hidden lists a posting that was never muted")
	}
}

// TestHiddenPageRendersEyeOn verifies the mute toggle on /hidden renders in the
// on state, so clicking it un-hides.
func TestHiddenPageRendersEyeOn(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	ctx := context.Background()
	id := mustUpsert(t, st, listingPosting("muted", "숨긴 공고 제목"))
	if err := st.SetNotInterested(ctx, id, time.Now()); err != nil {
		t.Fatalf("SetNotInterested: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/hidden", nil))
	body := rec.Body.String()
	if !strings.Contains(body, `class="not-interested on"`) {
		t.Error("/hidden does not render the mute toggle in the on state")
	}
}

// TestHiddenPageMarksBookmarkedRows verifies a muted posting that is also
// bookmarked renders its bookmark toggle in the on state on /hidden.
func TestHiddenPageMarksBookmarkedRows(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	ctx := context.Background()
	id := mustUpsert(t, st, listingPosting("both", "숨기고 저장한 공고"))
	if err := st.SetNotInterested(ctx, id, time.Now()); err != nil {
		t.Fatalf("SetNotInterested: %v", err)
	}
	if err := st.SetBookmark(ctx, id, time.Now()); err != nil {
		t.Fatalf("SetBookmark: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/hidden", nil))
	body := rec.Body.String()
	if !strings.Contains(body, `class="bookmark on"`) {
		t.Error("/hidden does not mark the bookmarked row's bookmark toggle as on")
	}
}

// TestHiddenPageEmptyState verifies the quiet empty state shows when nothing is
// hidden.
func TestHiddenPageEmptyState(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/hidden", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "숨긴 공고가 없어요") {
		t.Error("/hidden empty-state copy missing")
	}
}

// TestHiddenPageHasNavLink verifies the 숨긴 공고 nav link is present and marked
// active on /hidden.
func TestHiddenPageHasNavLink(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/hidden", nil))
	if !strings.Contains(rec.Body.String(), `<a href="/hidden" class="active">숨긴 공고</a>`) {
		t.Error("/hidden missing the active 숨긴 공고 nav link")
	}
}
