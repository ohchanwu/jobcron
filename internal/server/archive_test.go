package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestArchivePageListsEveryPosting(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})

	today := listingPosting("today1", "오늘 공고")
	today.FirstSeenAt = time.Now().UTC()
	today.LastSeenAt = today.FirstSeenAt

	yesterday := listingPosting("yest1", "어제 공고")
	yesterday.FirstSeenAt = time.Now().Add(-26 * time.Hour).UTC()
	yesterday.LastSeenAt = yesterday.FirstSeenAt

	old := listingPosting("old1", "오래된 공고")
	old.FirstSeenAt = time.Now().Add(-30 * 24 * time.Hour).UTC()
	old.LastSeenAt = old.FirstSeenAt

	mustUpsert(t, st, today)
	mustUpsert(t, st, yesterday)
	mustUpsert(t, st, old)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/archive", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"오늘 공고", "어제 공고", "오래된 공고"} {
		if !strings.Contains(body, want) {
			t.Errorf("/archive missing %q", want)
		}
	}
	if !strings.Contains(body, "오늘") {
		t.Error("/archive missing the 오늘 day marker for today's group")
	}
}

func TestArchiveGroupsByKSTDay(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	ctx := context.Background()

	// Two postings on the same KST day, one on a different day.
	dayA := time.Date(2026, 5, 22, 14, 0, 0, 0, time.UTC) // 23:00 KST May 22
	dayB := time.Date(2026, 5, 22, 16, 0, 0, 0, time.UTC) // 01:00 KST May 23 (different day in KST)

	pA1 := listingPosting("a1", "A1")
	pA1.FirstSeenAt, pA1.LastSeenAt = dayA, dayA
	pA2 := listingPosting("a2", "A2")
	pA2.FirstSeenAt, pA2.LastSeenAt = dayA.Add(time.Minute), dayA.Add(time.Minute)
	pB := listingPosting("b1", "B1")
	pB.FirstSeenAt, pB.LastSeenAt = dayB, dayB

	mustUpsert(t, st, pA1)
	mustUpsert(t, st, pA2)
	mustUpsert(t, st, pB)

	now := time.Date(2026, 5, 23, 6, 0, 0, 0, time.UTC) // mid-day May 23 KST
	view, err := srv.buildArchive(ctx, now)
	if err != nil {
		t.Fatalf("buildArchive: %v", err)
	}
	if view.Total != 3 {
		t.Errorf("Total = %d, want 3", view.Total)
	}
	if len(view.Days) != 2 {
		t.Fatalf("Days = %d, want 2 (one per distinct KST date)", len(view.Days))
	}
	if !view.Days[0].IsToday {
		t.Error("Days[0].IsToday = false; the most recent group should be marked today")
	}
	if len(view.Days[0].Postings) != 1 || len(view.Days[1].Postings) != 2 {
		t.Errorf("group sizes = [%d, %d], want [1, 2]",
			len(view.Days[0].Postings), len(view.Days[1].Postings))
	}
}

func TestArchiveEmptyState(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/archive", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "아직 스크랩한 공고가 없어요") {
		t.Error("/archive missing the empty-state copy")
	}
}

func TestArchiveMarksBookmarkedRows(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	ctx := context.Background()

	p := listingPosting("p1", "저장된 공고")
	p.FirstSeenAt = time.Now().UTC()
	p.LastSeenAt = p.FirstSeenAt
	id := mustUpsert(t, st, p)
	if err := st.SetBookmark(ctx, id, time.Now()); err != nil {
		t.Fatalf("SetBookmark: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/archive", nil))
	if !strings.Contains(rec.Body.String(), `class="bookmark on"`) {
		t.Error("/archive does not mark the bookmarked row as on")
	}
}
