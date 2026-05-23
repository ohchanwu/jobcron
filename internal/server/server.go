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

// Server wires storage, a scraper, and the HTTP handlers together.
type Server struct {
	store  *storage.Store
	scr    scraper.Scraper
	tmpl   *template.Template
	flight *singleFlight
}

// New builds a Server over the given storage and scraper. It parses the
// embedded HTML templates, panicking on a malformed template (a developer
// error caught at startup).
func New(store *storage.Store, scr scraper.Scraper) *Server {
	return &Server{
		store:  store,
		scr:    scr,
		tmpl:   template.Must(template.ParseFS(web.FS, "*.html")),
		flight: newSingleFlight(),
	}
}

// ScrapeResult summarizes one scrape run.
type ScrapeResult struct {
	Listed  int `json:"listed"`
	New     int `json:"new"`
	Scored  int `json:"scored"`
	Removed int `json:"removed"` // postings hard-deleted by the staleness sweep
}

// runScrape executes the full pipeline — robots check, listing fetch, a
// detail fetch for each posting new to the DB, persistence, then scoring —
// reporting progress through emit (status / count / progress events).
func (s *Server) runScrape(ctx context.Context, emit func(event, data string)) (ScrapeResult, error) {
	emit("status", "점핏 robots.txt 확인 중...")
	if err := s.scr.CheckAccess(ctx); err != nil {
		return ScrapeResult{}, fmt.Errorf("server: scrape access denied: %w", err)
	}
	emit("status", "✓ 허용됐어요 — 공고 목록을 가져오는 중...")
	listing, err := s.scr.FetchListing(ctx, 0)
	if err != nil {
		return ScrapeResult{}, fmt.Errorf("server: fetch listing: %w", err)
	}
	known, err := s.store.KnownSourceIDs(ctx, s.scr.Source())
	if err != nil {
		return ScrapeResult{}, err
	}

	now := time.Now().UTC()
	res := ScrapeResult{Listed: len(listing)}

	// Split the listing: already-seen postings just get last_seen_at bumped;
	// new ones need a detail fetch.
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
	emit("count", fmt.Sprintf("✓ 새로운 공고 %d개를 찾았어요", len(fresh)))

	for _, p := range fresh {
		detailed, err := s.scr.FetchDetail(ctx, p)
		if err != nil {
			continue // skip a posting whose detail fetch failed
		}
		detailed.FirstSeenAt = now
		detailed.LastSeenAt = now
		if _, _, err := s.store.UpsertPosting(ctx, detailed); err != nil {
			return res, fmt.Errorf("server: insert new posting: %w", err)
		}
		res.New++
		emit("progress", fmt.Sprintf("공고 %d/%d 가져오는 중...", res.New, len(fresh)))
	}

	removed, err := s.store.SweepStalePostings(ctx, now, sweepStaleWindow, sweepOldWindow)
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
