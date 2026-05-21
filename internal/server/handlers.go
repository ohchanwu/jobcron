package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/ohchanwu/job-scraper/internal/profile"
	"github.com/ohchanwu/job-scraper/internal/scoring"
	"github.com/ohchanwu/job-scraper/internal/scraper"
	"github.com/ohchanwu/job-scraper/web"
)

// Handler builds the HTTP routing for the server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleDashboard)
	mux.HandleFunc("GET /profile", s.handleProfileForm)
	mux.HandleFunc("POST /profile", s.handleProfileSave)
	mux.HandleFunc("POST /api/scrape", s.handleScrape)
	mux.Handle("GET /static/", http.StripPrefix("/static/",
		http.FileServer(http.FS(web.FS))))
	return mux
}

// handleScrape runs the full scrape pipeline synchronously and returns the
// result as JSON.
func (s *Server) handleScrape(w http.ResponseWriter, r *http.Request) {
	res, err := s.runScrape(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// dashboardPosting is one row of the daily briefing.
type dashboardPosting struct {
	Posting     scraper.Posting
	Total       int
	Excluded    bool
	Explanation string
}

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
	postings, err := s.dashboardPostings(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "index.html", map[string]any{"Postings": postings})
}

// dashboardPostings loads every posting joined with its stored score, sorted
// by score descending (excluded postings, total -1, fall to the bottom).
func (s *Server) dashboardPostings(ctx context.Context) ([]dashboardPosting, error) {
	postings, err := s.store.AllPostings(ctx)
	if err != nil {
		return nil, err
	}
	scores, err := s.store.ScoresByPostingID(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dashboardPosting, 0, len(postings))
	for _, p := range postings {
		dp := dashboardPosting{Posting: p}
		if sc, ok := scores[p.ID]; ok {
			dp.Total = sc.Total
			dp.Excluded = sc.Total < 0
			var result scoring.ScoreResult
			if json.Unmarshal([]byte(sc.BreakdownJSON), &result) == nil {
				dp.Explanation = scoring.Explain(result)
			}
		}
		out = append(out, dp)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Total > out[j].Total })
	return out, nil
}

// profileForm is the view model for the profile form — flat string/int fields
// matching the HTML inputs.
type profileForm struct {
	CareerYears      int
	SalaryFloorMan   int
	MaxEducation     int
	StacksText       string
	CitiesText       string
	LocationWeight   int
	RemoteOK         bool
	MustHaveText     string
	DealbreakersText string
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
	s.render(w, "profile.html", toProfileForm(p))
}

// handleProfileSave parses the submitted form, stores the profile, re-scores
// every posting against it, and redirects to the dashboard.
func (s *Server) handleProfileSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	p := profile.Profile{
		CareerYears:    atoi(r.FormValue("career_years")),
		SalaryFloorKRW: atoi(r.FormValue("salary_floor_man")) * 10000,
		MaxEducation:   profile.EducationLevel(atoi(r.FormValue("max_education"))),
		Stacks:         parseStacks(r.FormValue("stacks")),
		Location: profile.LocationPref{
			Cities:   parseCSV(r.FormValue("cities")),
			Weight:   atoi(r.FormValue("location_weight")),
			RemoteOK: r.FormValue("remote_ok") != "",
		},
		MustHave:     parseLines(r.FormValue("must_have")),
		Dealbreakers: parseLines(r.FormValue("dealbreakers")),
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
		SalaryFloorMan:   p.SalaryFloorKRW / 10000,
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
