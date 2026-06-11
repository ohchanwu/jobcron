package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/ohchanwu/job-scraper/internal/scoring"
)

// bookmarksView is the view model for the /bookmarks page.
type bookmarksView struct {
	Postings []dashboardPosting
	Date     string      // today's date, for header symmetry with the briefing
	Rerate   *rerateInfo // re-rate button state; nil = no AI key (button hidden)
}

// handleBookmarks renders the user's saved postings, most recently saved
// first. Expired postings stay visible so the user can decide whether to
// clear them out.
func (s *Server) handleBookmarks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	view, err := s.buildBookmarks(ctx, time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "bookmarks.html", view)
}

// buildBookmarks assembles the bookmarks view: every bookmarked posting,
// each carrying its current score chips and a deadline badge. Bookmark
// order is most-recently-saved first (storage layer does the sort).
func (s *Server) buildBookmarks(ctx context.Context, now time.Time) (bookmarksView, error) {
	postings, err := s.store.BookmarkedPostings(ctx)
	if err != nil {
		return bookmarksView{}, err
	}
	scores, err := s.store.ScoresByPostingID(ctx)
	if err != nil {
		return bookmarksView{}, err
	}
	// A bookmarked posting can also be muted ("관심 없음"); it stays visible
	// here (unlike on the briefing / 전체 공고 list) but renders its mute
	// toggle in the on state so the user can un-mute it from here too.
	muted, err := s.store.NotInterestedIDs(ctx)
	if err != nil {
		return bookmarksView{}, err
	}
	view := bookmarksView{Date: now.In(kstZone).Format("2006 / 01 / 02")}
	for _, p := range postings {
		dp := dashboardPosting{
			Posting:       p,
			Bookmarked:    true, // every row here is bookmarked by definition
			NotInterested: muted[p.ID],
			Deadline:      deadlineBadge(p.ClosedAt, p.AlwaysOpen, now),
		}
		if sc, ok := scores[p.ID]; ok {
			dp.Total = sc.Total
			dp.Excluded = sc.Total < 0
			var result scoring.ScoreResult
			if json.Unmarshal([]byte(sc.BreakdownJSON), &result) == nil {
				dp.Explanation = scoring.Explain(result)
				dp.Breakdown = result.Breakdown
			}
		}
		view.Postings = append(view.Postings, dp)
	}
	prof, _, err := s.loadProfile(ctx)
	if err != nil {
		return bookmarksView{}, err
	}
	view.Rerate = s.buildRerateInfo(ctx, prof, "bookmarks", view.Postings)
	return view, nil
}

// handleBookmarkAdd saves a posting to the user's bookmarks. Idempotent —
// a repeat PUT does not advance bookmarked_at.
func (s *Server) handleBookmarkAdd(w http.ResponseWriter, r *http.Request) {
	id, ok := postingID(w, r)
	if !ok {
		return
	}
	if err := s.store.SetBookmark(r.Context(), id, time.Now()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeBookmarkState(w, true)
}

// handleBookmarkRemove clears the bookmark for a posting. Idempotent —
// removing a never-bookmarked posting is a no-op success.
func (s *Server) handleBookmarkRemove(w http.ResponseWriter, r *http.Request) {
	id, ok := postingID(w, r)
	if !ok {
		return
	}
	if err := s.store.ClearBookmark(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeBookmarkState(w, false)
}

// postingID extracts the {id} path value as a positive int64, writing
// 400 Bad Request and returning ok=false when the value is missing or
// unparseable.
func postingID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := r.PathValue("id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid posting id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

// writeBookmarkState replies with the new bookmark state as JSON so the
// client can mirror its UI to the source of truth without re-reading.
func writeBookmarkState(w http.ResponseWriter, bookmarked bool) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]bool{"bookmarked": bookmarked})
}
