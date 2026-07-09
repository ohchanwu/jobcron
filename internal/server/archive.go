package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
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

// archiveView is the view model for the /archive (전체 공고) page.
type archiveView struct {
	Today    string // header date, mirroring the briefing view's Date field
	Days     []archiveDay
	Excluded []dashboardPosting // below-MinScore / dealbreaker rows, collapsed
	Total    int                // total posting count (main + excluded), for the header counter
	Rerate   *rerateInfo        // re-rate button state; nil = no AI key (button hidden)
	SortMode string             // "date" (day-grouped, default) | "score" (one flat fit ranking)
}

// Archive sort modes. 날짜순 (date) is the default day-grouped view; 점수순
// (score) flattens every kept row into one list ranked by fit, regardless of
// when it was first seen.
const (
	archiveSortDate  = "date"
	archiveSortScore = "score"
)

// normalizeArchiveSort maps the ?sort= query value to a known mode, defaulting
// to date for anything unrecognized (incl. empty).
func normalizeArchiveSort(v string) string {
	if v == archiveSortScore {
		return archiveSortScore
	}
	return archiveSortDate
}

// applyScoreSort collapses the day-grouped Days into a single, date-headerless
// group sorted by Total descending — the 점수순 (by-fit) view. Excluded rows
// (dealbreaker / below-MinScore) are untouched; they still collapse into the
// 관심 밖 section exactly as in 날짜순. A no-op when there are no kept rows.
func (v *archiveView) applyScoreSort() {
	var all []dashboardPosting
	for _, day := range v.Days {
		all = append(all, day.Postings...)
	}
	sort.SliceStable(all, func(a, b int) bool { return all[a].Total > all[b].Total })
	if len(all) == 0 {
		v.Days = nil
		return
	}
	v.Days = []archiveDay{{Postings: all}} // Date "" → the template omits the day header
}

// handleArchive renders every posting the scraper has ever stored, grouped
// by the day it was first seen, most recent day first.
func (s *Server) handleArchive(w http.ResponseWriter, r *http.Request) {
	userID, err := s.stateUserID(r.Context(), r)
	if err != nil {
		writeAuthUnauthorized(w)
		return
	}
	view, err := s.buildArchive(r.Context(), time.Now(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	view.SortMode = resolveArchiveSort(w, r)
	if view.SortMode == archiveSortScore {
		view.applyScoreSort()
	}
	s.render(w, "archive.html", view)
}

// archiveSortCookie remembers the 전체 공고 sort per-browser.
const archiveSortCookie = "archive_sort"

// resolveArchiveSort picks the 전체 공고 sort and remembers it. An explicit
// ?sort= wins and is written to a per-browser cookie; otherwise the remembered
// cookie applies; otherwise the 날짜순 default. Doing this server-side (rather
// than a client-side localStorage redirect) means the right sort renders on the
// FIRST load — no second round trip, so no white flash on return visits.
func resolveArchiveSort(w http.ResponseWriter, r *http.Request) string {
	if q := r.URL.Query().Get("sort"); q != "" {
		mode := normalizeArchiveSort(q)
		http.SetCookie(w, &http.Cookie{
			Name:     archiveSortCookie,
			Value:    mode,
			Path:     "/archive",
			MaxAge:   60 * 60 * 24 * 365, // a year
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		return mode
	}
	if c, err := r.Cookie(archiveSortCookie); err == nil {
		return normalizeArchiveSort(c.Value)
	}
	return archiveSortDate
}

// buildArchive groups every stored posting by its first-seen KST day. The
// SQL ORDER BY first_seen_at DESC means we can walk the list once and
// open a new day group every time the KST date changes. Cross-portal
// duplicates are hidden — only the canonical row appears — matching the
// briefing's behavior.
func (s *Server) buildArchive(ctx context.Context, now time.Time, userIDOpt ...int64) (archiveView, error) {
	userID := optionalUserID(userIDOpt)
	allPostings, err := s.store.CanonicalPostings(ctx)
	if err != nil {
		return archiveView{}, err
	}
	scores, err := s.scoresByPostingID(ctx, userID)
	if err != nil {
		return archiveView{}, err
	}
	bookmarks, err := s.bookmarkedIDs(ctx, userID)
	if err != nil {
		return archiveView{}, err
	}
	if s.demoMode {
		bookmarks = map[int64]bool{}
	}
	muted, err := s.notInterestedIDs(ctx, userID)
	if err != nil {
		return archiveView{}, err
	}
	if s.demoMode {
		muted = map[int64]bool{}
	}
	dupSources, err := s.store.DuplicateSourcesByCanonical(ctx)
	if err != nil {
		return archiveView{}, err
	}
	prof, _, err := s.loadProfile(ctx, userID)
	if err != nil {
		return archiveView{}, err
	}
	disabled := s.disabledSourceSet(prof.DisabledSources)
	minScore := prof.EffectiveMinScore()

	// Muted ("관심 없음") postings vanish from this page entirely, just like
	// the briefing — they are not merely demoted into the 관심 밖 collapsible.
	postings := make([]scraper.Posting, 0, len(allPostings))
	for _, p := range allPostings {
		if !disabled[p.Source] && !muted[p.ID] {
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
			Posting:          p,
			Bookmarked:       bookmarks[p.ID],
			Deadline:         deadlineBadge(p.ClosedAt, p.AlwaysOpen, now),
			DuplicateSources: dupSources[p.ID],
		}
		if sc, ok := scores[p.ID]; ok {
			dp.Total = sc.Total
			// Mirror the briefing's split: dealbreaker hits (Total < 0) and
			// rows below the MinScore threshold drop out of the day-grouped
			// list into a single "관심 밖으로 분류된 공고" collapsible, instead
			// of the old inline-dimmed treatment. MinScore = 0 keeps every
			// non-dealbreaker row in the main list. A bookmarked posting is
			// exempt from the soft MinScore hide (the user saved it) but not
			// from the dealbreaker hide — Total < 0 stays unconditional.
			dp.Excluded = sc.Total < 0 || (sc.Total < minScore && !bookmarks[p.ID])
			var result scoring.ScoreResult
			if json.Unmarshal([]byte(sc.BreakdownJSON), &result) == nil {
				dp.Explanation = scoring.Explain(result)
				dp.Breakdown = result.Breakdown
			}
		}

		// Past-deadline postings drop into the 관심 밖 collapsible regardless of
		// score — they're closed, so they leave the live day list but stay
		// findable here (and badged "마감"). Mirrors the briefing, which filters
		// expired postings out of the daily list entirely. 데모데이 keeps
		// re-serving published-but-past rows, so without this they would sit in
		// the main 전체 공고 list with a normal score and no closure cue.
		if expired(p, now) {
			dp.Excluded = true
		}

		if dp.Excluded {
			view.Excluded = append(view.Excluded, dp)
			continue
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

	// Within each day group, sort by score descending so the most relevant
	// postings rise to the top of the day. Matches the dashboard's sort
	// rule (handlers.go). SliceStable preserves the SQL-layer
	// `first_seen_at DESC` order as a tie-breaker — for two postings with
	// the same score, the more recently-found one shows first. Excluded
	// postings (Total < 0) sink to the bottom of their day naturally.
	for i := range view.Days {
		day := &view.Days[i]
		sort.SliceStable(day.Postings, func(a, b int) bool {
			return day.Postings[a].Total > day.Postings[b].Total
		})
	}
	lists := make([][]dashboardPosting, 0, len(view.Days))
	for _, day := range view.Days {
		lists = append(lists, day.Postings)
	}
	view.Rerate = s.buildRerateInfo(ctx, prof, "archive", lists...)
	return view, nil
}
