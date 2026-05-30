package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ohchanwu/job-scraper/internal/profile"
	"github.com/ohchanwu/job-scraper/internal/scoring"
	"github.com/ohchanwu/job-scraper/internal/scraper"
	"github.com/ohchanwu/job-scraper/web"
)

// Handler builds the HTTP routing for the server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleDashboard)
	mux.HandleFunc("GET /archive", s.handleArchive)
	mux.HandleFunc("GET /bookmarks", s.handleBookmarks)
	mux.HandleFunc("GET /hidden", s.handleHidden)
	mux.HandleFunc("GET /profile", s.handleProfileForm)
	mux.HandleFunc("POST /profile", s.handleProfileSave)
	mux.HandleFunc("GET /api/scrape", s.handleScrapeSSE)
	mux.HandleFunc("PUT /api/bookmark/{id}", s.handleBookmarkAdd)
	mux.HandleFunc("DELETE /api/bookmark/{id}", s.handleBookmarkRemove)
	mux.HandleFunc("PUT /api/not-interested/{id}", s.handleNotInterestedAdd)
	mux.HandleFunc("DELETE /api/not-interested/{id}", s.handleNotInterestedRemove)
	// Browsers request /favicon.ico at the root regardless of the <link>
	// tags; redirect those to the embedded asset so they stop 404ing.
	mux.Handle("GET /favicon.ico", http.RedirectHandler("/static/favicon.ico", http.StatusMovedPermanently))
	mux.Handle("GET /static/", http.StripPrefix("/static/",
		http.FileServer(http.FS(web.FS))))
	return mux
}

// handleScrapeSSE runs the scrape pipeline, streaming progress to the client
// as Server-Sent Events. A second concurrent scrape is rejected with 409.
// The lock is global rather than per-source because the user clicks one
// button, sees one SSE stream, and would be confused by partial states.
func (s *Server) handleScrapeSSE(w http.ResponseWriter, r *http.Request) {
	if !s.flight.tryAcquire(scrapeAllKey) {
		http.Error(w, "이미 스크랩이 진행 중이에요. 잠시만 기다려 주세요.", http.StatusConflict)
		return
	}
	defer s.flight.release(scrapeAllKey)

	sw, err := newSSEWriter(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	res, err := s.runScrape(r.Context(), sw.event)
	if err != nil {
		sw.event("failed", "스크랩에 실패했어요. 잠시 후 다시 시도해 주세요.")
		return
	}
	sw.event("done", fmt.Sprintf("브리핑이 준비됐어요 — 새 공고 %d개", res.New))
}

// dashboardPosting is one row of the daily briefing.
type dashboardPosting struct {
	Posting          scraper.Posting
	Total            int
	Excluded         bool
	Bookmarked       bool               // user has saved this posting
	NotInterested    bool               // user has muted this posting ("관심 없음")
	Explanation      string             // "React +20 · 신입 +25 ..." (used for excluded rows)
	Breakdown        []scoring.LineItem // structured line items, rendered as chips
	Deadline         string             // "오늘 마감" | "마감 D-2" | ""
	DuplicateSources []string           // sources of cross-portal duplicates collapsed into this canonical
}

// briefing is the daily-briefing view model: postings first seen today, split
// into the scored list and the dealbreaker-excluded list.
type briefing struct {
	Today    []dashboardPosting
	Excluded []dashboardPosting
	Date     string // "2026 / 05 / 23" (KST)
}

// briefingCap bounds how many postings the daily briefing lists.
const briefingCap = 50

// kstZone is Korea Standard Time (UTC+9, no DST).
var kstZone = time.FixedZone("KST", 9*60*60)

// handleDashboard renders the daily briefing. On first run (no profile saved)
// it redirects to the profile form.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, _, hasProfile, err := s.store.Profile(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !hasProfile {
		http.Redirect(w, r, "/profile", http.StatusSeeOther)
		return
	}
	b, err := s.buildBriefing(ctx, time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "index.html", b)
}

// buildBriefing assembles today's briefing: postings first seen today and not
// yet past their closing date, each joined with its score, split into the
// scored list (sorted high to low, capped) and the dealbreaker-excluded list.
// Postings from sources the user has disabled in their profile are filtered
// out — disabled sources are invisible everywhere except /bookmarks. Cross-
// portal duplicates are also filtered (only the canonical row is rendered;
// the dashboardPosting carries its sibling sources via DuplicateSources for
// the "also on …" badge).
func (s *Server) buildBriefing(ctx context.Context, now time.Time) (briefing, error) {
	postings, err := s.store.CanonicalPostings(ctx)
	if err != nil {
		return briefing{}, err
	}
	scores, err := s.store.ScoresByPostingID(ctx)
	if err != nil {
		return briefing{}, err
	}
	bookmarks, err := s.store.BookmarkedIDs(ctx)
	if err != nil {
		return briefing{}, err
	}
	muted, err := s.store.NotInterestedIDs(ctx)
	if err != nil {
		return briefing{}, err
	}
	dupSources, err := s.store.DuplicateSourcesByCanonical(ctx)
	if err != nil {
		return briefing{}, err
	}
	prof, _, err := s.loadProfile(ctx)
	if err != nil {
		return briefing{}, err
	}
	disabled := s.disabledSourceSet(prof.DisabledSources)
	b := briefing{Date: now.In(kstZone).Format("2006 / 01 / 02")}
	for _, p := range postings {
		if disabled[p.Source] || muted[p.ID] {
			continue
		}
		if !sameKSTDay(p.FirstSeenAt, now) || expired(p, now) {
			continue
		}
		dp := dashboardPosting{
			Posting:          p,
			Bookmarked:       bookmarks[p.ID],
			Deadline:         deadlineBadge(p.ClosedAt, p.AlwaysOpen, now),
			DuplicateSources: dupSources[p.ID],
		}
		if sc, ok := scores[p.ID]; ok {
			dp.Total = sc.Total
			// Dealbreaker hits (Total = -1) are always excluded. The
			// MinScore knob collapses additional low-scoring rows out of
			// the main "Today" list — the user can still find them under
			// the expandable "제외된 공고" section. MinScore = 0 disables
			// the soft-hide entirely. A bookmarked posting is exempt from
			// the soft MinScore hide (the user deliberately saved it) but
			// NOT from the dealbreaker hide — Total < 0 stays unconditional.
			dp.Excluded = sc.Total < 0 || (sc.Total < prof.EffectiveMinScore() && !bookmarks[p.ID])
			var result scoring.ScoreResult
			if json.Unmarshal([]byte(sc.BreakdownJSON), &result) == nil {
				dp.Explanation = scoring.Explain(result)
				dp.Breakdown = result.Breakdown
			}
		}
		if dp.Excluded {
			b.Excluded = append(b.Excluded, dp)
		} else {
			b.Today = append(b.Today, dp)
		}
	}
	sort.SliceStable(b.Today, func(i, j int) bool { return b.Today[i].Total > b.Today[j].Total })
	if len(b.Today) > briefingCap {
		b.Today = b.Today[:briefingCap]
	}
	return b, nil
}

// sameKSTDay reports whether a and b fall on the same calendar day in KST.
func sameKSTDay(a, b time.Time) bool {
	ak, bk := a.In(kstZone), b.In(kstZone)
	return ak.Year() == bk.Year() && ak.YearDay() == bk.YearDay()
}

// expired reports whether a posting is past its closing date.
func expired(p scraper.Posting, now time.Time) bool {
	return !p.AlwaysOpen && p.ClosedAt != nil && p.ClosedAt.Before(now)
}

// deadlineBadge returns a gentle closing-soon badge for a posting closing
// within three calendar days (KST), or "" otherwise.
func deadlineBadge(closedAt *time.Time, alwaysOpen bool, now time.Time) string {
	if alwaysOpen || closedAt == nil {
		return ""
	}
	c, n := closedAt.In(kstZone), now.In(kstZone)
	closeDay := time.Date(c.Year(), c.Month(), c.Day(), 0, 0, 0, 0, kstZone)
	today := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, kstZone)
	switch days := int(closeDay.Sub(today).Hours() / 24); {
	case days == 0:
		return "오늘 마감"
	case days >= 1 && days <= 3:
		return fmt.Sprintf("마감 D-%d", days)
	default:
		return ""
	}
}

// profileForm is the view model for the profile form — flat string/int fields
// matching the HTML inputs.
type profileForm struct {
	CareerYears      int
	CareerWeight     int
	CareerNearMiss   int // derived: round(CareerWeight × 2/5), shown as a hint
	SalaryFloorMan   int
	SalaryWeight     int
	SalaryAmbiguous  int // derived: round(SalaryWeight ÷ 2), shown as a hint
	MinScore         int
	MaxEducation     int
	StacksText       string
	CitiesText       string
	LocationWeight   int
	RemoteOK         bool
	MustHaveText     string
	DealbreakersText string
	Sources          []sourceOption // one row per registered scraper
	Muted            []mutedPosting // postings the user marked 관심 없음
}

// mutedPosting is a single row in the profile form's "관심 없음" unmute list:
// just enough to identify the posting and target the unmute endpoint.
type mutedPosting struct {
	ID      int64
	Title   string
	Company string
}

// handleProfileForm renders the profile form, pre-filled with any saved profile.
func (s *Server) handleProfileForm(w http.ResponseWriter, r *http.Request) {
	jsonStr, _, ok, err := s.store.Profile(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var p profile.Profile
	if ok {
		if p, err = profile.Unmarshal(jsonStr); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	form := toProfileForm(p)
	form.Sources = s.sourceOptions(p.DisabledSources)
	mutedPostings, err := s.store.NotInterestedPostings(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, mp := range mutedPostings {
		form.Muted = append(form.Muted, mutedPosting{ID: mp.ID, Title: mp.Title, Company: mp.Company})
	}
	s.render(w, "profile.html", form)
}

// handleProfileSave parses the submitted form, stores the profile, re-scores
// every posting against it, and redirects to the dashboard.
func (s *Server) handleProfileSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Sources: the form submits source_<id>=on for each checked box. We
	// invert that into the DisabledSources opt-out list (registered IDs
	// the user did NOT check), so a future scraper added in a later
	// release ships enabled-by-default without a profile rewrite.
	var disabled []string
	for _, src := range s.sources {
		id := src.Source()
		if r.FormValue("source_"+id) == "" {
			disabled = append(disabled, id)
		}
	}
	minScore := atoi(r.FormValue("min_score"))
	p := profile.Profile{
		CareerYears:    atoi(r.FormValue("career_years")),
		CareerWeight:   atoi(r.FormValue("career_weight")),
		SalaryFloorKRW: atoi(r.FormValue("salary_floor_man")) * 10000,
		SalaryWeight:   atoi(r.FormValue("salary_weight")),
		MinScore:       &minScore,
		MaxEducation:   profile.EducationLevel(atoi(r.FormValue("max_education"))),
		Stacks:         parseStacks(r.FormValue("stacks")),
		Location: profile.LocationPref{
			Cities:   parseCSV(r.FormValue("cities")),
			Weight:   atoi(r.FormValue("location_weight")),
			RemoteOK: r.FormValue("remote_ok") != "",
		},
		MustHave:        parseLines(r.FormValue("must_have")),
		Dealbreakers:    parseLines(r.FormValue("dealbreakers")),
		DisabledSources: disabled,
	}
	canonical, err := profile.Marshal(p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, _, err := s.store.SaveProfile(r.Context(), canonical); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := s.scoreAll(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// toProfileForm converts a stored profile into the flat form view model.
func toProfileForm(p profile.Profile) profileForm {
	stacks := make([]string, 0, len(p.Stacks))
	for _, sp := range p.Stacks {
		stacks = append(stacks, sp.Name+","+strconv.Itoa(sp.Weight))
	}
	return profileForm{
		CareerYears:      p.CareerYears,
		CareerWeight:     p.EffectiveCareerWeight(),
		CareerNearMiss:   scoring.NearMissCareerAward(p.EffectiveCareerWeight()),
		SalaryFloorMan:   p.SalaryFloorKRW / 10000,
		SalaryWeight:     p.EffectiveSalaryWeight(),
		SalaryAmbiguous:  scoring.AmbiguousSalaryAward(p.EffectiveSalaryWeight()),
		MinScore:         p.EffectiveMinScore(),
		MaxEducation:     int(p.MaxEducation),
		StacksText:       strings.Join(stacks, "\n"),
		CitiesText:       strings.Join(p.Location.Cities, ", "),
		LocationWeight:   p.Location.Weight,
		RemoteOK:         p.Location.RemoteOK,
		MustHaveText:     strings.Join(p.MustHave, "\n"),
		DealbreakersText: strings.Join(p.Dealbreakers, "\n"),
	}
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// atoi parses an integer form value, treating blank or malformed input as 0.
func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// parseLines splits textarea input into trimmed, non-empty lines.
func parseLines(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// parseCSV splits comma-separated input into trimmed, non-empty values.
func parseCSV(text string) []string {
	var out []string
	for _, part := range strings.Split(text, ",") {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// parseStacks parses one "name,weight" stack preference per line.
func parseStacks(text string) []profile.StackPref {
	var out []profile.StackPref
	for _, line := range parseLines(text) {
		name, weight, _ := strings.Cut(line, ",")
		if name = strings.TrimSpace(name); name == "" {
			continue
		}
		out = append(out, profile.StackPref{Name: name, Weight: atoi(weight)})
	}
	return out
}
