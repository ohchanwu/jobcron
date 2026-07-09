package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/ohchanwu/job-scraper/internal/scoring"
	"github.com/ohchanwu/job-scraper/internal/scraper"
)

// hiddenView is the view model for the /hidden (숨긴 공고) page: every posting
// the user manually muted ("관심 없음"), most recently muted first. This is the
// real home for muted jobs — the eye button hides them from / and /archive
// entirely, and this page is where the user sees and un-hides them.
type hiddenView struct {
	Postings  []dashboardPosting
	Date      string // today's date, for header symmetry with the other pages
	CSRFToken string
}

// handleHidden renders the user's manually-muted postings.
func (s *Server) handleHidden(w http.ResponseWriter, r *http.Request) {
	userID, err := s.stateUserID(r.Context(), r)
	if err != nil {
		writeAuthUnauthorized(w)
		return
	}
	view, err := s.buildHidden(r.Context(), time.Now(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderWithRequest(w, r, "hidden.html", view)
}

// buildHidden assembles the 숨긴 공고 view: every muted posting (ordered
// most-recently-muted first by the storage layer), each carrying its current
// score chips, a deadline badge, and its bookmark state. The mute toggle is
// rendered in the on state (every row here is muted by definition) so clicking
// it un-hides. Muted postings from a source the user later disabled still show
// here, mirroring /bookmarks — this is the only place to un-hide them.
func (s *Server) buildHidden(ctx context.Context, now time.Time, userIDOpt ...int64) (hiddenView, error) {
	userID := optionalUserID(userIDOpt)
	postings, err := s.notInterestedPostings(ctx, userID)
	if err != nil {
		return hiddenView{}, err
	}
	if s.demoMode {
		postings, err = s.store.AllPostings(ctx)
		if err != nil {
			return hiddenView{}, err
		}
	}
	scores, err := s.scoresByPostingID(ctx, userID)
	if err != nil {
		return hiddenView{}, err
	}
	bookmarks, err := s.bookmarkedIDs(ctx, userID)
	if err != nil {
		return hiddenView{}, err
	}
	if s.demoMode {
		bookmarks = map[int64]bool{}
	}
	view := hiddenView{Date: now.In(kstZone).Format("2006 / 01 / 02")}
	for _, p := range postings {
		dp := dashboardPosting{
			Posting:       p,
			Bookmarked:    bookmarks[p.ID],
			NotInterested: !s.demoMode, // demo localStorage decides which rows are hidden
			Deadline:      deadlineBadge(p.ClosedAt, p.AlwaysOpen, now),
		}
		if sc, ok := scores[p.ID]; ok {
			dp.Total = sc.Total
			dp.Excluded = sc.Total < 0 // dealbreaker hits render dimmed with "—"
			var result scoring.ScoreResult
			if json.Unmarshal([]byte(sc.BreakdownJSON), &result) == nil {
				dp.Explanation = scoring.Explain(result)
				dp.Breakdown = result.Breakdown
			}
		}
		view.Postings = append(view.Postings, dp)
	}
	return view, nil
}

func (s *Server) notInterestedPostings(ctx context.Context, userID int64) ([]scraper.Posting, error) {
	if userID == 0 {
		return s.store.NotInterestedPostings(ctx)
	}
	return s.store.NotInterestedPostingsForUser(ctx, userID)
}
