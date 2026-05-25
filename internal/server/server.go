package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"time"

	"github.com/ohchanwu/job-scraper/internal/profile"
	"github.com/ohchanwu/job-scraper/internal/scoring"
	"github.com/ohchanwu/job-scraper/internal/scraper"
	"github.com/ohchanwu/job-scraper/internal/storage"
	"github.com/ohchanwu/job-scraper/web"
)

// scrapeNewCap bounds how many new postings one scrape will detail-fetch — a
// defensive limit; the 점핏 신입 universe is well under it.
const scrapeNewCap = 50

// Sweep windows. Postings that have not been seen in any scrape for
// sweepStaleWindow, OR were first seen more than sweepOldWindow ago AND
// are not always_open, get hard-deleted at the end of every scrape.
// Bookmarked postings are exempt from both rules.
const (
	sweepStaleWindow = 3 * 24 * time.Hour
	sweepOldWindow   = 90 * 24 * time.Hour
)

// Server wires storage, one or more scrapers, and the HTTP handlers together.
type Server struct {
	store   *storage.Store
	sources []scraper.Scraper
	tmpl    *template.Template
	flight  *singleFlight
}

// New builds a Server over the given storage and one or more scrapers. The
// scrape pipeline iterates sources in the order they are registered, so the
// most-trusted source should come first. It parses the embedded HTML
// templates, panicking on a malformed template (a developer error caught at
// startup).
func New(store *storage.Store, sources ...scraper.Scraper) *Server {
	if len(sources) == 0 {
		panic("server.New: at least one scraper is required")
	}
	funcs := template.FuncMap{
		"sourceLabel": sourceLabel,
	}
	tmpl := template.Must(template.New("").Funcs(funcs).ParseFS(web.FS, "*.html"))
	return &Server{
		store:   store,
		sources: sources,
		tmpl:    tmpl,
		flight:  newSingleFlight(),
	}
}

// ScrapeResult summarizes one scrape run across every active source.
type ScrapeResult struct {
	Listed  int `json:"listed"`
	New     int `json:"new"`
	Scored  int `json:"scored"`
	Removed int `json:"removed"` // postings hard-deleted by the staleness sweep
}

// scrapeAllKey is the singleflight key for a multi-source scrape run. We
// hold one global lock for the whole pipeline rather than one per source —
// the user clicks one button, sees one progress stream, and would be
// confused by partial states. Per-source locks would matter if scrapes were
// triggered independently.
const scrapeAllKey = "_all_"

// runScrape executes the full pipeline across every enabled source: robots
// check → listing → detail fetch → upsert → sweep → score. Disabled sources
// are skipped entirely and their data is frozen in the DB so re-enabling a
// source does not require a fresh scrape.
func (s *Server) runScrape(ctx context.Context, emit func(event, data string)) (ScrapeResult, error) {
	prof, profileOK, err := s.loadProfile(ctx)
	if err != nil {
		return ScrapeResult{}, err
	}

	var active []scraper.Scraper
	for _, src := range s.sources {
		if !profileOK || prof.SourceEnabled(src.Source()) {
			active = append(active, src)
		}
	}
	if len(active) == 0 {
		emit("status", "활성화된 공고 출처가 없어요 — 프로필 설정에서 켜주세요.")
		return ScrapeResult{}, nil
	}

	now := time.Now().UTC()
	var res ScrapeResult
	for _, src := range active {
		sub, err := s.runScrapeSource(ctx, src, now, emit)
		if err != nil {
			return res, err
		}
		res.Listed += sub.Listed
		res.New += sub.New
	}

	activeIDs := make([]string, 0, len(active))
	for _, src := range active {
		activeIDs = append(activeIDs, src.Source())
	}
	removed, err := s.store.SweepStalePostings(ctx, now, sweepStaleWindow, sweepOldWindow, activeIDs)
	if err != nil {
		return res, fmt.Errorf("server: sweep stale postings: %w", err)
	}
	res.Removed = removed
	if removed > 0 {
		emit("status", fmt.Sprintf("오래된 공고 %d개를 정리했어요", removed))
	}

	emit("status", "공고에 점수를 매기는 중...")
	scored, err := s.scoreAll(ctx)
	if err != nil {
		return res, err
	}
	res.Scored = scored
	return res, nil
}

// runScrapeSource scrapes one source, emitting source-prefixed status events
// so the user can tell which source is currently active in the stream.
func (s *Server) runScrapeSource(
	ctx context.Context, src scraper.Scraper, now time.Time, emit func(event, data string),
) (ScrapeResult, error) {
	label := sourceLabel(src.Source())
	emit("status", fmt.Sprintf("[%s] robots.txt 확인 중...", label))
	if err := src.CheckAccess(ctx); err != nil {
		return ScrapeResult{}, fmt.Errorf("server: %s access denied: %w", src.Source(), err)
	}
	emit("status", fmt.Sprintf("[%s] ✓ 허용됐어요 — 공고 목록을 가져오는 중...", label))
	listing, err := src.FetchListing(ctx, 0)
	if err != nil {
		return ScrapeResult{}, fmt.Errorf("server: %s fetch listing: %w", src.Source(), err)
	}
	known, err := s.store.KnownSourceIDs(ctx, src.Source())
	if err != nil {
		return ScrapeResult{}, err
	}
	res := ScrapeResult{Listed: len(listing)}

	var fresh []scraper.Posting
	for _, p := range listing {
		if known[p.SourcePostingID] {
			p.LastSeenAt = now
			if _, _, err := s.store.UpsertPosting(ctx, p); err != nil {
				return res, fmt.Errorf("server: refresh seen posting: %w", err)
			}
			continue
		}
		fresh = append(fresh, p)
	}
	if len(fresh) > scrapeNewCap {
		fresh = fresh[:scrapeNewCap]
	}
	emit("count", fmt.Sprintf("[%s] ✓ 새로운 공고 %d개를 찾았어요", label, len(fresh)))

	for _, p := range fresh {
		detailed, err := src.FetchDetail(ctx, p)
		if err != nil {
			continue
		}
		detailed.FirstSeenAt = now
		detailed.LastSeenAt = now
		if _, _, err := s.store.UpsertPosting(ctx, detailed); err != nil {
			return res, fmt.Errorf("server: insert new posting: %w", err)
		}
		res.New++
		emit("progress", fmt.Sprintf("[%s] 공고 %d/%d 가져오는 중...", label, res.New, len(fresh)))
	}
	return res, nil
}

// loadProfile fetches the saved profile, returning ok=false when none has
// been saved yet. Returning ok=false instead of an error keeps the scrape
// pipeline from blowing up before the user even has a chance to set up.
func (s *Server) loadProfile(ctx context.Context) (profile.Profile, bool, error) {
	jsonStr, _, ok, err := s.store.Profile(ctx)
	if err != nil {
		return profile.Profile{}, false, err
	}
	if !ok {
		return profile.Profile{}, false, nil
	}
	p, err := profile.Unmarshal(jsonStr)
	if err != nil {
		return profile.Profile{}, false, fmt.Errorf("server: decode profile: %w", err)
	}
	return p, true, nil
}

// scoreAll scores every stored posting against the current profile and upserts
// the score rows. It is a no-op when no profile has been saved yet.
func (s *Server) scoreAll(ctx context.Context) (int, error) {
	profJSON, profHash, ok, err := s.store.Profile(ctx)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, nil
	}
	prof, err := profile.Unmarshal(profJSON)
	if err != nil {
		return 0, fmt.Errorf("server: decode profile: %w", err)
	}
	postings, err := s.store.AllPostings(ctx)
	if err != nil {
		return 0, err
	}
	for _, p := range postings {
		result := scoring.Score(p, prof)
		breakdown, err := json.Marshal(result)
		if err != nil {
			return 0, fmt.Errorf("server: marshal score: %w", err)
		}
		if err := s.store.UpsertScore(ctx, storage.Score{
			PostingID:     p.ID,
			ProfileHash:   profHash,
			Total:         result.Total,
			BreakdownJSON: string(breakdown),
			ComputedAt:    time.Now().UTC(),
		}); err != nil {
			return 0, fmt.Errorf("server: save score: %w", err)
		}
	}
	return len(postings), nil
}
