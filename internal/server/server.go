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

// defaultRunTokenCap is the per-run AI token ceiling (input + output). A
// generous in-memory limit so one scrape or re-rate can't run away on cost.
// Exceeding it halts AI for the rest of the run and falls back to regex scoring.
//
// The ceiling is SOFT: a call is admitted while there is headroom and debited
// after it returns, so the last admitted call can overspend by up to its own
// cost before the next canSpend() halts. That's fine for a generous cap; the
// hard, persisted, cross-run enforcement is the ai_usage daily ledger below.
// (Renamed from maxRunInputTokensDefault — it counts input+output, not just
// input.)
const defaultRunTokenCap = 150_000

// aiRateLimit paces live provider calls, mirroring the scrapers' 1-req/s
// politeness. Tests pass 0 to the provider constructor to disable pacing.
const aiRateLimit = time.Second

// Server wires storage, one or more scrapers, and the HTTP handlers together.
type Server struct {
	store   *storage.Store
	sources []scraper.Scraper
	tmpl    *template.Template
	flight  *singleFlight

	// AI extraction (BYOK, v2.0). ai is nil when no provider is configured —
	// the default — and the pipeline behaves exactly like v1.5 (regex scoring,
	// no provider calls). ReconfigureAI builds the provider from ai_keys.json +
	// the profile's chosen provider/model; main.go calls it at startup and
	// handleProfileSave on every save, so a key entered in the form goes live
	// without a restart.
	ai              ai.Provider
	aiModel         string
	aiVersion       string // ai.AIVersion(ai.Name(), aiModel), precomputed
	aiRunTokenCap   int    // per-run, in-memory
	aiDailyTokenCap int    // rolling daily, enforced against the persisted ai_usage ledger
	aiKeysPath      string // ai_keys.json location; empty = ai.DefaultKeysPath() (tests override)
}

// SetAIProvider enables AI with the given provider and model. A nil provider
// (the default) leaves AI off. Called after New (the constructor is variadic
// over scrapers, so AI config rides a setter) and by ReconfigureAI. Tests call
// it directly with a stub.
func (s *Server) SetAIProvider(p ai.Provider, model string) {
	s.ai = p
	s.aiModel = model
	if p != nil {
		s.aiVersion = ai.AIVersion(p.Name(), model)
	}
}

// SetAIKeysPath overrides where ReconfigureAI reads/writes the BYOK key file.
// Empty (the default) uses ai.DefaultKeysPath(). Tests point this at a temp dir
// so they never touch the real ~/.../job-scraper/ai_keys.json.
func (s *Server) SetAIKeysPath(path string) { s.aiKeysPath = path }

// keysPath returns the configured ai_keys.json path, falling back to the OS
// default.
func (s *Server) keysPath() (string, error) {
	if s.aiKeysPath != "" {
		return s.aiKeysPath, nil
	}
	return ai.DefaultKeysPath()
}

// ReconfigureAI (re)builds the AI provider from the saved profile + ai_keys.json
// and applies the profile's daily token cap. It is the single wiring point:
// main.go calls it once at startup, handleProfileSave on every save. AI is left
// OFF (provider nil, silent regex fallback) when the profile selects no provider
// or the selected provider has no saved key. A bad provider name / build error
// also leaves AI off and is returned for the caller to log — never fatal.
func (s *Server) ReconfigureAI(ctx context.Context) error {
	prof, ok, err := s.loadProfile(ctx)
	if err != nil {
		return err
	}
	if ok && prof.EffectiveAIDailyTokenCap() > 0 {
		s.aiDailyTokenCap = prof.EffectiveAIDailyTokenCap()
	}
	if !ok || prof.AIProvider == "" {
		s.SetAIProvider(nil, "")
		return nil
	}
	path, err := s.keysPath()
	if err != nil {
		s.SetAIProvider(nil, "")
		return err
	}
	keys, err := ai.LoadKeys(path)
	if err != nil {
		s.SetAIProvider(nil, "")
		return err
	}
	key := keys[prof.AIProvider]
	if key == "" {
		// Provider chosen but no key yet → silent regex fallback (D4).
		s.SetAIProvider(nil, "")
		return nil
	}
	model := prof.AIModel
	if model == "" {
		model = ai.DefaultModel(prof.AIProvider)
	}
	p, err := ai.New(prof.AIProvider, key, model, aiRateLimit)
	if err != nil {
		s.SetAIProvider(nil, "")
		return err
	}
	s.SetAIProvider(p, model)
	return nil
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
		store:           store,
		sources:         sources,
		flight:          newSingleFlight(),
		aiRunTokenCap:   defaultRunTokenCap,
		aiDailyTokenCap: profile.DefaultDailyTokenCap,
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
	budget := s.newAIBudget(ctx)
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

// aiBudget gates AI spending for one run against two ceilings: a per-run,
// in-memory cap (runCap) and a rolling daily cap (dailyCap) enforced against the
// persisted ai_usage ledger. The daily total at the run's START is read once
// (dailyAtStart); each admitted call's tokens are added to runSpent AND written
// through to the ledger, so the cap holds across process restarts and across
// runs in the same day. (Within a single day, concurrent AI runs are kept from
// double-spending the daily budget by the scrape⟷re-rate exclusion — T7.)
type aiBudget struct {
	store        *storage.Store
	day          string // UTC date, e.g. "2026-06-03"
	runCap       int
	dailyCap     int
	dailyAtStart int  // ledger total (input+output) when the run began
	runSpent     int  // input+output debited this run
	degraded     bool // true once either cap was hit mid-run
}

// newAIBudget returns a fresh per-run budget, or nil when AI is off (so the
// scrape loop skips all AI bookkeeping). It reads the day's ledger total once so
// the daily cap accounts for spend from earlier runs (and earlier process
// lifetimes) the same day.
func (s *Server) newAIBudget(ctx context.Context) *aiBudget {
	if s.ai == nil {
		return nil
	}
	day := time.Now().UTC().Format("2006-01-02")
	in, out, err := s.store.AIUsageForDay(ctx, day)
	if err != nil {
		in, out = 0, 0 // a ledger read error must not block scoring — start from 0
	}
	return &aiBudget{
		store:        s.store,
		day:          day,
		runCap:       s.aiRunTokenCap,
		dailyCap:     s.aiDailyTokenCap,
		dailyAtStart: in + out,
	}
}

// canSpend reports whether either cap still has headroom, marking the run
// degraded the first time it does not (so the caller surfaces the calm banner).
func (b *aiBudget) canSpend() bool {
	if b.runSpent >= b.runCap || b.dailyAtStart+b.runSpent >= b.dailyCap {
		b.degraded = true
		return false
	}
	return true
}

// debit records a call's token usage: against the in-memory run counter and
// (best-effort) through to the persisted daily ledger. A ledger write failure is
// swallowed — it must never fail a scrape — but the in-memory run cap still
// holds for the rest of this run.
func (b *aiBudget) debit(ctx context.Context, u ai.Usage) {
	b.runSpent += u.InputTokens + u.OutputTokens
	_ = b.store.AddAIUsage(ctx, b.day, u.InputTokens, u.OutputTokens)
}

// runScrapeSource scrapes one source, emitting source-prefixed status events
// so the user can tell which source is currently active in the stream.
func (s *Server) runScrapeSource(
	ctx context.Context, src scraper.Scraper, now time.Time, budget *aiBudget, emit func(event, data string),
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
func (s *Server) extractStage1(ctx context.Context, id int64, p scraper.Posting, now time.Time, budget *aiBudget) {
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
	budget.debit(ctx, usage)
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
