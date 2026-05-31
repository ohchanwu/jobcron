package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ohchanwu/job-scraper/internal/profile"
	"github.com/ohchanwu/job-scraper/internal/storage"
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

func TestArchiveSortsByScoreWithinEachDay(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	ctx := context.Background()

	// MinScore = 0 keeps every row in the main day list — this test is about
	// sort order, not the below-MinScore split (see
	// TestArchiveRoutesBelowMinScoreToExcluded for that).
	zero := 0
	profJSON, _ := profile.Marshal(profile.Profile{MinScore: &zero})
	if _, _, err := st.SaveProfile(ctx, profJSON); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	// Three postings on the same KST day, inserted in an order that does
	// NOT match the score order. Expectation: the day's postings render
	// score-descending, not insertion-order.
	day := time.Date(2026, 5, 23, 6, 0, 0, 0, time.UTC) // mid-day KST

	low := listingPosting("low", "낮은 점수")
	low.FirstSeenAt, low.LastSeenAt = day, day
	highScore := listingPosting("high", "높은 점수")
	highScore.FirstSeenAt, highScore.LastSeenAt = day.Add(time.Minute), day.Add(time.Minute)
	mid := listingPosting("mid", "중간 점수")
	mid.FirstSeenAt, mid.LastSeenAt = day.Add(2*time.Minute), day.Add(2*time.Minute)

	lowID := mustUpsert(t, st, low)
	highID := mustUpsert(t, st, highScore)
	midID := mustUpsert(t, st, mid)

	for id, total := range map[int64]int{lowID: 15, highID: 80, midID: 40} {
		if err := st.UpsertScore(ctx, storage.Score{
			PostingID: id, ProfileHash: "test", Total: total,
			BreakdownJSON: "[]", ComputedAt: time.Now(),
		}); err != nil {
			t.Fatalf("UpsertScore id=%d: %v", id, err)
		}
	}

	view, err := srv.buildArchive(ctx, day.Add(time.Hour))
	if err != nil {
		t.Fatalf("buildArchive: %v", err)
	}
	if len(view.Days) != 1 {
		t.Fatalf("Days = %d, want 1 (all three postings on the same KST day)", len(view.Days))
	}
	got := view.Days[0].Postings
	if len(got) != 3 {
		t.Fatalf("day has %d postings, want 3", len(got))
	}
	wantOrder := []string{"높은 점수", "중간 점수", "낮은 점수"} // 80, 40, 15
	for i, want := range wantOrder {
		if got[i].Posting.Title != want {
			t.Errorf("position %d title = %q, want %q (full order: %v)",
				i, got[i].Posting.Title, want,
				[]string{got[0].Posting.Title, got[1].Posting.Title, got[2].Posting.Title})
		}
	}
}

// TestArchiveRoutesBelowMinScoreToExcluded covers task 1(b): the 관심 공고
// page mirrors the briefing's MinScore split. A posting scoring 35 with
// MinScore = 40 lands in the collapsible Excluded list, not the main
// day-grouped list; a 50 stays in the main list.
func TestArchiveRoutesBelowMinScoreToExcluded(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	ctx := context.Background()

	forty := 40
	profJSON, _ := profile.Marshal(profile.Profile{MinScore: &forty})
	if _, _, err := st.SaveProfile(ctx, profJSON); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	day := time.Date(2026, 5, 29, 6, 0, 0, 0, time.UTC)
	low := listingPosting("low", "낮은 점수 공고")
	low.FirstSeenAt, low.LastSeenAt = day, day
	high := listingPosting("high", "높은 점수 공고")
	high.FirstSeenAt, high.LastSeenAt = day, day
	lowID := mustUpsert(t, st, low)
	highID := mustUpsert(t, st, high)
	for id, total := range map[int64]int{lowID: 35, highID: 50} {
		if err := st.UpsertScore(ctx, storage.Score{
			PostingID: id, ProfileHash: "test", Total: total,
			BreakdownJSON: "[]", ComputedAt: time.Now(),
		}); err != nil {
			t.Fatalf("UpsertScore id=%d: %v", id, err)
		}
	}

	view, err := srv.buildArchive(ctx, day.Add(time.Hour))
	if err != nil {
		t.Fatalf("buildArchive: %v", err)
	}

	if len(view.Excluded) != 1 || view.Excluded[0].Posting.Title != "낮은 점수 공고" {
		t.Errorf("Excluded = %v, want exactly [낮은 점수 공고]", postingTitles(view.Excluded))
	}
	var dayTitles []string
	for _, d := range view.Days {
		for _, p := range d.Postings {
			dayTitles = append(dayTitles, p.Posting.Title)
		}
	}
	if len(dayTitles) != 1 || dayTitles[0] != "높은 점수 공고" {
		t.Errorf("main day list = %v, want exactly [높은 점수 공고]", dayTitles)
	}
	// Total counts both the main and excluded postings.
	if view.Total != 2 {
		t.Errorf("Total = %d, want 2 (main + excluded)", view.Total)
	}
}

// TestArchiveRoutesExpiredToExcluded covers the expired-listings task: a
// posting past its closing date drops into the 관심 밖 collapsible regardless
// of score (it's closed, so it leaves the live day list but stays findable),
// and carries the "마감" badge. A still-open posting with the same score stays
// in the main day list.
func TestArchiveRoutesExpiredToExcluded(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	ctx := context.Background()

	zero := 0
	profJSON, _ := profile.Marshal(profile.Profile{MinScore: &zero})
	if _, _, err := st.SaveProfile(ctx, profJSON); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	day := time.Date(2026, 5, 29, 6, 0, 0, 0, time.UTC)
	now := day.Add(time.Hour)

	open := listingPosting("open", "열린 공고")
	open.FirstSeenAt, open.LastSeenAt = day, day
	openClose := now.Add(72 * time.Hour)
	open.ClosedAt = &openClose

	past := listingPosting("past", "마감된 공고")
	past.FirstSeenAt, past.LastSeenAt = day, day
	pastClose := now.Add(-24 * time.Hour)
	past.ClosedAt = &pastClose

	openID := mustUpsert(t, st, open)
	pastID := mustUpsert(t, st, past)
	for id, total := range map[int64]int{openID: 80, pastID: 80} {
		if err := st.UpsertScore(ctx, storage.Score{
			PostingID: id, ProfileHash: "test", Total: total,
			BreakdownJSON: "[]", ComputedAt: time.Now(),
		}); err != nil {
			t.Fatalf("UpsertScore id=%d: %v", id, err)
		}
	}

	view, err := srv.buildArchive(ctx, now)
	if err != nil {
		t.Fatalf("buildArchive: %v", err)
	}

	if len(view.Excluded) != 1 || view.Excluded[0].Posting.Title != "마감된 공고" {
		t.Fatalf("Excluded = %v, want exactly [마감된 공고]", postingTitles(view.Excluded))
	}
	if got := view.Excluded[0].Deadline; got.Text != "마감" || got.Kind != "urgent" {
		t.Errorf("expired row badge = {Text:%q Kind:%q}, want {마감 urgent}", got.Text, got.Kind)
	}
	var dayTitles []string
	for _, d := range view.Days {
		dayTitles = append(dayTitles, postingTitles(d.Postings)...)
	}
	if len(dayTitles) != 1 || dayTitles[0] != "열린 공고" {
		t.Errorf("main day list = %v, want exactly [열린 공고]", dayTitles)
	}
}

// TestArchiveHidesMutedPostings covers task 1(c): a muted ("관심 없음")
// posting vanishes from 관심 공고 entirely — not into the Excluded
// collapsible, but gone from both lists.
func TestArchiveHidesMutedPostings(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	ctx := context.Background()

	zero := 0
	profJSON, _ := profile.Marshal(profile.Profile{MinScore: &zero})
	if _, _, err := st.SaveProfile(ctx, profJSON); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	day := time.Date(2026, 5, 29, 6, 0, 0, 0, time.UTC)
	shown := listingPosting("shown", "보이는 공고")
	shown.FirstSeenAt, shown.LastSeenAt = day, day
	hidden := listingPosting("hidden", "숨긴 공고")
	hidden.FirstSeenAt, hidden.LastSeenAt = day, day
	mustUpsert(t, st, shown)
	hiddenID := mustUpsert(t, st, hidden)
	if err := st.SetNotInterested(ctx, hiddenID, time.Now()); err != nil {
		t.Fatalf("SetNotInterested: %v", err)
	}

	view, err := srv.buildArchive(ctx, day.Add(time.Hour))
	if err != nil {
		t.Fatalf("buildArchive: %v", err)
	}
	if view.Total != 1 {
		t.Errorf("Total = %d, want 1 (the muted posting is gone)", view.Total)
	}
	all := postingTitles(view.Excluded)
	for _, d := range view.Days {
		all = append(all, postingTitles(d.Postings)...)
	}
	for _, title := range all {
		if title == "숨긴 공고" {
			t.Errorf("muted posting still present in archive view: %v", all)
		}
	}
}

// postingTitles is a test helper: pull the titles out of a slice of rows.
func postingTitles(rows []dashboardPosting) []string {
	out := make([]string, 0, len(rows))
	for _, p := range rows {
		out = append(out, p.Posting.Title)
	}
	return out
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
