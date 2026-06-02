package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"time"

	"github.com/ohchanwu/job-scraper/internal/ai"
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

// maxRunInputTokensDefault is the per-run AI token budget. It is a generous
// in-memory ceiling so one scrape can't run away on cost; the persisted daily
// ledger and a user-configurable cap are T9. Exceeding it halts AI for the
// rest of the run and falls back to regex scoring.
const maxRunInputTokensDefault = 150_000

// Server wires storage, one or more scrapers, and the HTTP handlers together.
type Server struct {
	store   *storage.Store
	sources []scraper.Scraper
	tmpl    *template.Template
	flight  *singleFlight

	// AI extraction (BYOK, v2.0 Stage 1). ai is nil when no provider is
	// configured — the default — and the pipeline behaves exactly like v1.5
	// (regex scoring, no provider calls). Wiring a real provider from
	// ai_keys.json + a chosen model is the /profile settings work in T9.
	ai            ai.Provider
	aiModel       string
	aiVersion     string // ai.AIVersion(ai.Name(), aiModel), precomputed
	aiRunTokenCap int
}

// SetAIProvider enables AI extraction with the given provider and model. A nil
// provider (the default) leaves AI off. Called after New (the constructor is
// variadic over scrapers, so AI config rides a setter).
func (s *Server) SetAIProvider(p ai.Provider, model string) {
	s.ai = p
	s.aiModel = model
	if p != nil {
		s.aiVersion = ai.AIVersion(p.Name(), model)
	}
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
	srv := &Server{
		store:         store,
		sources:       sources,
		flight:        newSingleFlight(),
		aiRunTokenCap: maxRunInputTokensDefault,
	}
	funcs := template.FuncMap{
		"sourceLabel":       sourceLabel,
		"registeredSources": srv.allRegisteredSources,
		"sourcePillGroups":  srv.sourcePillGroups,
	}
	srv.tmpl = template.Must(template.New("").Funcs(funcs).ParseFS(web.FS, "*.html"))
	return srv
}

// ScrapeResult summarizes one scrape run across every active source.
type ScrapeResult struct {
	Listed     int `json:"listed"`
	New        int `json:"new"`
	Scored     int `json:"scored"`
	Removed    int `json:"removed"`    // postings hard-deleted by the staleness sweep
	Duplicates int `json:"duplicates"` // cross-portal duplicates collapsed onto a canonical
	Failed     int `json:"failed"`     // sources that errored and were skipped this run
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

	// Set the expectation up front: the per-source clients pace at 1 req/s
	// so a 50-posting first scrape takes ~a minute. Without this line a
	// user staring at the progress counter might think the tool is hung.
	emit("status", "천천히 가져올게요 — 출처 사이트에 부담을 주지 않으려고 1초에 하나씩 받아와요. ☕")

	now := time.Now().UTC()
	var res ScrapeResult
	// One AI token budget for the whole run (persists across sources). nil
	// when AI is off, so no per-posting budget bookkeeping happens at all.
	budget := s.newAIBudget()
	// succeeded tracks sources that completed without error this run; only
	// they get their data swept. A source that failed cannot tell us what
	// is stale (no fresh baseline this run), so we leave its existing rows
	// untouched until the next successful scrape.
	var succeeded []scraper.Scraper
	for _, src := range active {
		sub, err := s.runScrapeSource(ctx, src, now, budget, emit)
		if err != nil {
			// Per-source fault isolation: one source's failure must not
			// abort the whole briefing. Surface the failure as a status
			// line and move on; the user still gets a working briefing
			// from every source that did succeed.
			emit("status", fmt.Sprintf("[%s] 스크랩에 실패했어요 — 다른 출처를 계속할게요.", sourceLabel(src.Source())))
			res.Failed++
			continue
		}
		succeeded = append(succeeded, src)
		res.Listed += sub.Listed
		res.New += sub.New
	}

	activeIDs := make([]string, 0, len(succeeded))
	for _, src := range succeeded {
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

	duplicates, err := s.markCrossPortalDuplicates(ctx)
	if err != nil {
		return res, fmt.Errorf("server: dedup pass: %w", err)
	}
	res.Duplicates = duplicates
	if duplicates > 0 {
		emit("status", fmt.Sprintf("다른 사이트에 똑같이 올라온 공고 %d개를 묶었어요", duplicates))
	}

	// Cold-start banner (D6): if the per-run AI budget ran out mid-scrape, some
	// postings were scored by regex instead of AI. A mixed briefing is fine —
	// surface it calmly, never as a failure.
	if budget != nil && budget.degraded {
		emit("status", "오늘 AI 예산을 다 써서 일부는 일반 점수로 분석했어요.")
	}

	emit("status", "공고에 점수를 매기는 중...")
	scored, err := s.scoreAll(ctx)
	if err != nil {
		return res, err
	}
	res.Scored = scored
	return res, nil
}

// aiRunBudget is the per-run, in-memory AI token counter (T4). The persisted
// daily ledger (ai_usage) and the user-configurable cap are T9.
type aiRunBudget struct {
	remaining int
	degraded  bool // true once the budget was exhausted mid-run
}

// newAIBudget returns a fresh per-run budget, or nil when AI is off (so the
// scrape loop skips all AI bookkeeping).
func (s *Server) newAIBudget() *aiRunBudget {
	if s.ai == nil {
		return nil
	}
	return &aiRunBudget{remaining: s.aiRunTokenCap}
}

// canSpend reports whether the budget has headroom for another call, marking
// the run degraded the first time it does not.
func (b *aiRunBudget) canSpend() bool {
	if b.remaining <= 0 {
		b.degraded = true
		return false
	}
	return true
}

// debit subtracts a call's token usage from the remaining budget.
func (b *aiRunBudget) debit(u ai.Usage) { b.remaining -= u.InputTokens + u.OutputTokens }

// runScrapeSource scrapes one source, emitting source-prefixed status events
// so the user can tell which source is currently active in the stream.
func (s *Server) runScrapeSource(
	ctx context.Context, src scraper.Scraper, now time.Time, budget *aiRunBudget, emit func(event, data string),
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
		id, _, err := s.store.UpsertPosting(ctx, detailed)
		if err != nil {
			return res, fmt.Errorf("server: insert new posting: %w", err)
		}
		res.New++
		emit("progress", fmt.Sprintf("[%s] 공고 %d/%d 가져오는 중...", label, res.New, len(fresh)))
		// Stage-1 AI extraction (cache-read, D2). Best-effort: any failure
		// leaves no ai_extractions row and the posting is scored by the regex
		// path exactly as v1.5 — the scrape never fails on an AI error.
		s.extractStage1(ctx, id, detailed, now, budget)
	}
	return res, nil
}

// extractStage1 runs and caches the Stage-1 AI extraction for one new posting,
// if AI is enabled and the per-run budget has headroom. It writes only the
// ai_extractions cache row — the postings columns stay a faithful source
// mirror (D2). Every failure path is silent (regex fallback at score time).
func (s *Server) extractStage1(ctx context.Context, id int64, p scraper.Posting, now time.Time, budget *aiRunBudget) {
	if s.ai == nil || budget == nil || !budget.canSpend() {
		return
	}
	sent, contentHash, _ := ai.ModelInput(p)
	// Cache hit (same content already extracted under this ai_version): reuse,
	// no provider call. New postings always miss; this matters for the T7
	// re-rate backfill and idempotent re-runs.
	if _, ok, err := s.store.AIExtraction(ctx, id, contentHash, s.aiVersion); err == nil && ok {
		return
	}
	ext, usage, err := s.ai.Extract(ctx, sent)
	if err != nil {
		return // timeout / 5xx / malformed JSON / out-of-range → regex fallback
	}
	budget.debit(usage)
	// Best-effort cache write; a failure here just means a regex score this pass.
	_ = s.store.UpsertAIExtraction(ctx, id, contentHash, s.aiVersion, ext, now)
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
	// Batch-load the Stage-1 AI extractions once (no N+1) when AI is enabled;
	// scoreCareer/educationDealbreaker prefer them. Stage-2 deltas are not
	// wired yet (the delta arg stays nil until T5–T7).
	var exts map[int64]ai.Extraction
	if s.ai != nil {
		exts, err = s.store.AIExtractionsByPostingID(ctx, s.aiVersion)
		if err != nil {
			return 0, err
		}
	}
	for _, p := range postings {
		var ext *ai.Extraction
		if e, ok := exts[p.ID]; ok {
			ext = &e
		}
		result := scoring.Score(p, prof, ext, nil)
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
