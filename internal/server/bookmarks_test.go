package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/scraper"
)

func TestBookmarkAddSetsState(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	id := mustUpsert(t, st, listingPosting("1", "백엔드 신입"))

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec,
		httptest.NewRequest(http.MethodPut, "/api/bookmark/"+strconv.FormatInt(id, 10), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := decodeBookmark(t, rec.Body.String()); !got {
		t.Errorf(`response = {"bookmarked": %v}, want true`, got)
	}
	got, err := st.IsBookmarked(context.Background(), id)
	if err != nil || !got {
		t.Errorf("IsBookmarked = %v, %v; want true, nil", got, err)
	}
}

func TestBookmarkRemoveClearsState(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	id := mustUpsert(t, st, listingPosting("1", "백엔드 신입"))
	if err := st.SetBookmark(context.Background(), id, time.Now()); err != nil {
		t.Fatalf("seed bookmark: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec,
		httptest.NewRequest(http.MethodDelete, "/api/bookmark/"+strconv.FormatInt(id, 10), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := decodeBookmark(t, rec.Body.String()); got {
		t.Errorf(`response = {"bookmarked": %v}, want false`, got)
	}
	got, err := st.IsBookmarked(context.Background(), id)
	if err != nil {
		t.Fatalf("IsBookmarked: %v", err)
	}
	if got {
		t.Error("posting still bookmarked after DELETE")
	}
}

func TestBookmarkRejectsBadID(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/bookmark/not-a-number", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an unparseable id", rec.Code)
	}
}

func TestBookmarksPageListsSavedPostings(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	ctx := context.Background()
	savedID := mustUpsert(t, st, listingPosting("saved", "저장한 공고"))
	mustUpsert(t, st, listingPosting("unsaved", "저장 안 한 공고"))
	if err := st.SetBookmark(ctx, savedID, time.Now()); err != nil {
		t.Fatalf("SetBookmark: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/bookmarks", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "저장한 공고") {
		t.Error("/bookmarks does not list the saved posting")
	}
	if strings.Contains(body, "저장 안 한 공고") {
		t.Error("/bookmarks lists a posting that was never bookmarked")
	}
}

func TestBookmarksPageEmptyState(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/bookmarks", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "아직 저장한 공고가 없어요") {
		t.Error("/bookmarks empty state copy missing")
	}
}

func TestDashboardMarksBookmarkedRows(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	ctx := context.Background()

	profileJSON := `{"stacks":[],"location":{"cities":[],"weight":0,"remote_ok":false},"career_years":0,"salary_floor_krw":0,"max_education":0,"dealbreakers":null}`
	if _, _, err := st.SaveProfile(ctx, profileJSON); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	p := listingPosting("today1", "오늘 공고")
	p.FirstSeenAt = time.Now().UTC()
	p.LastSeenAt = p.FirstSeenAt
	id := mustUpsert(t, st, p)
	scoreEach(t, st, map[int64]int{id: 50}) // scored, as scoreAll always does post-scrape (Bug 2B skips unscored)
	if err := st.SetBookmark(ctx, id, time.Now()); err != nil {
		t.Fatalf("SetBookmark: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	body := rec.Body.String()
	if !strings.Contains(body, `class="bookmark on"`) {
		t.Errorf("dashboard does not mark the bookmarked row as on\n--- body ---\n%s", body)
	}
}

// mustUpsert inserts p and returns its row id, failing the test on error.
func mustUpsert(t *testing.T, st interface {
	UpsertPosting(context.Context, scraper.Posting) (int64, bool, error)
}, p scraper.Posting,
) int64 {
	t.Helper()
	if p.FirstSeenAt.IsZero() {
		p.FirstSeenAt = time.Now().UTC()
		p.LastSeenAt = p.FirstSeenAt
	}
	id, _, err := st.UpsertPosting(context.Background(), p)
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}
	return id
}

// decodeBookmark parses a {"bookmarked": bool} response body.
func decodeBookmark(t *testing.T, body string) bool {
	t.Helper()
	var resp struct {
		Bookmarked bool `json:"bookmarked"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode bookmark response %q: %v", body, err)
	}
	return resp.Bookmarked
}
