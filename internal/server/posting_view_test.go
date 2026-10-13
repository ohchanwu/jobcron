package server

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/profile"
	"github.com/ohchanwu/jobcron/internal/scoring"
)

func TestPostingProjectionAllSurfacesComputeMissingScores(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	ctx := context.Background()
	zero := 0
	prof := profile.Profile{CareerYears: 0, MinScore: &zero}
	pj, _ := profile.Marshal(prof)
	if _, _, err := st.SaveProfile(ctx, pj); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	p := listingPosting("projection-new", "새 신입 공고")
	p.FirstSeenAt = now
	p.LastSeenAt = now
	id := mustUpsert(t, st, p)
	if err := st.SetBookmark(ctx, id, now); err != nil {
		t.Fatal(err)
	}
	b, err := srv.buildBriefing(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	a, err := srv.buildArchive(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	book, err := srv.buildBookmarks(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	want := scoring.Score(p, prof, nil, nil, nil)
	if len(b.Today) != 1 || len(a.Days) != 1 || len(a.Days[0].Postings) != 1 || len(book.Postings) != 1 {
		t.Fatalf("missing score omitted: briefing=%d days=%d bookmarks=%d", len(b.Today), len(a.Days), len(book.Postings))
	}
	for _, got := range []dashboardPosting{b.Today[0], a.Days[0].Postings[0], book.Postings[0]} {
		if got.Total != want.Total || len(got.Breakdown) == 0 {
			t.Fatalf("computed score=%+v want=%+v", got, want)
		}
	}
	scores, err := st.ScoresByPostingID(ctx)
	if err != nil || len(scores) != 0 {
		t.Fatal("read materialized scores")
	}
}

func TestPostingInitialPagesDoNotTouchSession(t *testing.T) {
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	uid, cookie := createSessionUser(t, st, "page-observer@example.invalid", "page-observer-session")
	saveAIRuntimeProfile(t, st, uid, profile.Profile{CareerYears: 0})
	var before, after string
	if err := st.SQLDB().QueryRow(`SELECT last_seen_at::text FROM sessions WHERE user_id=$1`, uid).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/briefing", "/bookmarks"} {
		r := httptest.NewRequest("GET", path, nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s status=%d", path, w.Code)
		}
	}
	if err := st.SQLDB().QueryRow(`SELECT last_seen_at::text FROM sessions WHERE user_id=$1`, uid).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("initial page authentication wrote session activity")
	}
}

// Owner option 2 (2026-10-12): document/projection and new observation requests
// are SELECT-only. The two existing bootstrap status endpoints deliberately
// retain activity tracking. Run175's original whole-bootstrap RED is preserved
// in the task recovery attachment; this is the explicitly amended contract.
func TestPostingLegacyStatusRequestsRetainSessionActivity(t *testing.T) {
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	uid, cookie := createSessionUser(t, st, "bootstrap-observer@example.invalid", "bootstrap-observer-session")
	saveAIRuntimeProfile(t, st, uid, profile.Profile{CareerYears: 0})
	for _, path := range []string{"/api/briefing-status", "/api/rerate/status?surface=today", "/api/rerate/status?surface=bookmarks", "/api/rerate/status?surface=archive"} {
		var before, after string
		if err := st.SQLDB().QueryRow(`SELECT last_seen_at::text FROM sessions WHERE user_id=$1`, uid).Scan(&before); err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("GET", path, nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("bootstrap %s status=%d", path, w.Code)
		}
		if err := st.SQLDB().QueryRow(`SELECT last_seen_at::text FROM sessions WHERE user_id=$1`, uid).Scan(&after); err != nil {
			t.Fatal(err)
		}
		if before == after {
			t.Fatalf("legacy status %s lost activity tracking", path)
		}
		r = httptest.NewRequest("GET", path, nil)
		w = httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatalf("anonymous status %s=%d", path, w.Code)
		}
	}
}

func TestPostingHardExclusionDimIsBookmarkOnly(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	ctx := context.Background()
	pj, _ := profile.Marshal(profile.Profile{Dealbreakers: []string{"야근"}})
	st.SaveProfile(ctx, pj)
	p := listingPosting("contrast", "야근 개발자")
	p.FirstSeenAt = time.Now()
	p.LastSeenAt = p.FirstSeenAt
	id := mustUpsert(t, st, p)
	st.SetBookmark(ctx, id, time.Now())
	b, _ := srv.buildBriefing(ctx, time.Now())
	a, _ := srv.buildArchive(ctx, time.Now())
	book, _ := srv.buildBookmarks(ctx, time.Now())
	for i, row := range []dashboardPosting{b.Excluded[0], a.Excluded[0], book.Postings[0]} {
		var fragment bytes.Buffer
		if err := srv.tmpl.ExecuteTemplate(&fragment, "posting", row); err != nil {
			t.Fatal(err)
		}
		if dim := strings.Contains(fragment.String(), `class="posting posting-dim"`); dim != (i == 2) {
			t.Fatalf("surface %d contrast changed: dim=%v", i, dim)
		}
	}
}
