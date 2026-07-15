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

	"github.com/ohchanwu/jobcron/internal/profile"
)

// decodeNotInterested parses a {"not_interested": bool} response body.
func decodeNotInterested(t *testing.T, body string) bool {
	t.Helper()
	var resp struct {
		NotInterested bool `json:"not_interested"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode not-interested response %q: %v", body, err)
	}
	return resp.NotInterested
}

func TestNotInterestedAddSetsState(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	id := mustUpsert(t, st, listingPosting("1", "백엔드 신입"))

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec,
		httptest.NewRequest(http.MethodPut, "/api/not-interested/"+strconv.FormatInt(id, 10), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := decodeNotInterested(t, rec.Body.String()); !got {
		t.Errorf(`response = {"not_interested": %v}, want true`, got)
	}
	got, err := st.IsNotInterested(context.Background(), id)
	if err != nil || !got {
		t.Errorf("IsNotInterested = %v, %v; want true, nil", got, err)
	}
}

func TestNotInterestedRemoveClearsState(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	id := mustUpsert(t, st, listingPosting("1", "백엔드 신입"))
	if err := st.SetNotInterested(context.Background(), id, time.Now()); err != nil {
		t.Fatalf("seed mute: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec,
		httptest.NewRequest(http.MethodDelete, "/api/not-interested/"+strconv.FormatInt(id, 10), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := decodeNotInterested(t, rec.Body.String()); got {
		t.Errorf(`response = {"not_interested": %v}, want false`, got)
	}
	got, err := st.IsNotInterested(context.Background(), id)
	if err != nil {
		t.Fatalf("IsNotInterested: %v", err)
	}
	if got {
		t.Error("posting still muted after DELETE")
	}
}

func TestNotInterestedRejectsBadID(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/not-interested/not-a-number", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an unparseable id", rec.Code)
	}
}

// TestMutedPostingHiddenFromBriefing verifies a muted posting first seen
// today does not appear on the dashboard, while an un-muted sibling does.
func TestMutedPostingHiddenFromBriefing(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	ctx := context.Background()
	zero := 0
	profJSON, _ := profile.Marshal(profile.Profile{MinScore: &zero})
	if _, _, err := st.SaveProfile(ctx, profJSON); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	shown := listingPosting("shown", "보이는 공고")
	shown.FirstSeenAt = time.Now().UTC()
	shown.LastSeenAt = shown.FirstSeenAt
	// Title avoids the literal "숨긴 공고", which now appears in every page's
	// navbar (the 숨긴 공고 link) — a body substring check would false-match it.
	hidden := listingPosting("hidden", "음소거한 공고")
	hidden.FirstSeenAt = time.Now().UTC()
	hidden.LastSeenAt = hidden.FirstSeenAt
	mustUpsert(t, st, shown)
	hiddenID := mustUpsert(t, st, hidden)
	if _, err := srv.scoreAll(ctx, 0, nil); err != nil {
		t.Fatalf("scoreAll: %v", err)
	}
	if err := st.SetNotInterested(ctx, hiddenID, time.Now()); err != nil {
		t.Fatalf("SetNotInterested: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	body := rec.Body.String()
	if !strings.Contains(body, "보이는 공고") {
		t.Error("dashboard dropped the un-muted posting")
	}
	if strings.Contains(body, "음소거한 공고") {
		t.Error("dashboard still shows a muted posting")
	}
}

// TestMutedBookmarkStaysOnBookmarksPage verifies the one exception: a posting
// that is both bookmarked and muted stays visible on /bookmarks, with its
// mute toggle rendered in the on state.
func TestMutedBookmarkStaysOnBookmarksPage(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	ctx := context.Background()
	id := mustUpsert(t, st, listingPosting("bm", "저장하고 숨긴 공고"))
	scoreEach(t, st, map[int64]int{id: 50})
	if err := st.SetBookmark(ctx, id, time.Now()); err != nil {
		t.Fatalf("SetBookmark: %v", err)
	}
	if err := st.SetNotInterested(ctx, id, time.Now()); err != nil {
		t.Fatalf("SetNotInterested: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/bookmarks", nil))
	body := rec.Body.String()
	if !strings.Contains(body, "저장하고 숨긴 공고") {
		t.Error("/bookmarks dropped a bookmarked-but-muted posting; it should stay visible")
	}
	if !strings.Contains(body, `class="not-interested on"`) {
		t.Error("/bookmarks does not render the muted bookmark's mute toggle in the on state")
	}
}

// TestProfileFormShowsDerivedWeightHints verifies the profile form renders the
// near-miss / ambiguous derived awards next to the weight inputs. Default
// weights (25 / 10) derive to 10 / 5.
func TestProfileFormShowsDerivedWeightHints(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	twenty5, ten := 25, 10
	prof := profile.Profile{CareerWeight: twenty5, SalaryWeight: ten}
	profJSON, _ := profile.Marshal(prof)
	if _, _, err := st.SaveProfile(context.Background(), profJSON); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/profile", nil))
	body := rec.Body.String()
	if !strings.Contains(body, `id="derive-career">10<`) {
		t.Error("/profile career near-miss hint missing or not 10 for CareerWeight=25")
	}
	if !strings.Contains(body, `id="derive-salary">5<`) {
		t.Error("/profile salary ambiguous hint missing or not 5 for SalaryWeight=10")
	}
}
