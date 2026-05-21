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

// Server wires storage, a scraper, and the HTTP handlers together.
type Server struct {
	store *storage.Store
	scr   scraper.Scraper
	tmpl  *template.Template
}

// New builds a Server over the given storage and scraper. It parses the
// embedded HTML templates, panicking on a malformed template (a developer
// error caught at startup).
func New(store *storage.Store, scr scraper.Scraper) *Server {
	return &Server{
		store: store,
		scr:   scr,
		tmpl:  template.Must(template.ParseFS(web.FS, "*.html")),
	}
}

// ScrapeResult summarizes one scrape run.
type ScrapeResult struct {
	Listed int `json:"listed"`
	New    int `json:"new"`
	Scored int `json:"scored"`
}

// runScrape executes the full synchronous pipeline: robots check, listing
// fetch, a detail fetch for each posting new to the DB, persistence, then
// scoring of every stored posting against the current profile.
func (s *Server) runScrape(ctx context.Context) (ScrapeResult, error) {
	if err := s.scr.CheckAccess(ctx); err != nil {
		return ScrapeResult{}, fmt.Errorf("server: scrape access denied: %w", err)
	}
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
	for _, p := range listing {
		if known[p.SourcePostingID] {
			p.LastSeenAt = now
			if _, _, err := s.store.UpsertPosting(ctx, p); err != nil {
				return res, fmt.Errorf("server: refresh seen posting: %w", err)
			}
			continue
		}
		if res.New >= scrapeNewCap {
			break
		}
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
	}

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
