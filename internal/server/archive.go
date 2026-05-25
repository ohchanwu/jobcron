package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/ohchanwu/job-scraper/internal/scoring"
	"github.com/ohchanwu/job-scraper/internal/scraper"
)

// archiveDay is one calendar day's worth of postings on the archive page,
// grouped so the rendered list has visible date boundaries (a flat
// hundreds-of-rows list is hard to navigate).
type archiveDay struct {
	Date     string // "2026 / 05 / 23"
	IsToday  bool
	Postings []dashboardPosting
}

// archiveView is the view model for the /archive page.
type archiveView struct {
	Today string // header date, mirroring the briefing view's Date field
	Days  []archiveDay
	Total int // total posting count across all days, for the header counter
}

// handleArchive renders every posting the scraper has ever stored, grouped
// by the day it was first seen, most recent day first.
func (s *Server) handleArchive(w http.ResponseWriter, r *http.Request) {
	view, err := s.buildArchive(r.Context(), time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "archive.html", view)
}

// buildArchive groups every stored posting by its first-seen KST day. The
// SQL ORDER BY first_seen_at DESC means we can walk the list once and
// open a new day group every time the KST date changes.
func (s *Server) buildArchive(ctx context.Context, now time.Time) (archiveView, error) {
	allPostings, err := s.store.AllPostings(ctx)
	if err != nil {
		return archiveView{}, err
	}
	scores, err := s.store.ScoresByPostingID(ctx)
	if err != nil {
		return archiveView{}, err
	}
	bookmarks, err := s.store.BookmarkedIDs(ctx)
	if err != nil {
		return archiveView{}, err
	}
	prof, _, err := s.loadProfile(ctx)
	if err != nil {
		return archiveView{}, err
	}
	disabled := s.disabledSourceSet(prof.DisabledSources)

	postings := make([]scraper.Posting, 0, len(allPostings))
	for _, p := range allPostings {
		if !disabled[p.Source] {
			postings = append(postings, p)
		}
	}

	view := archiveView{
		Today: now.In(kstZone).Format("2006 / 01 / 02"),
		Total: len(postings),
	}

	var currentKey string // YYYY-MM-DD in KST
	for _, p := range postings {
		dp := dashboardPosting{
			Posting:    p,
			Bookmarked: bookmarks[p.ID],
			Deadline:   deadlineBadge(p.ClosedAt, p.AlwaysOpen, now),
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

		seenKST := p.FirstSeenAt.In(kstZone)
		key := seenKST.Format("2006-01-02")
		if key != currentKey {
			view.Days = append(view.Days, archiveDay{
				Date:    seenKST.Format("2006 / 01 / 02"),
				IsToday: sameKSTDay(p.FirstSeenAt, now),
			})
			currentKey = key
		}
		last := &view.Days[len(view.Days)-1]
		last.Postings = append(last.Postings, dp)
	}
	return view, nil
}
