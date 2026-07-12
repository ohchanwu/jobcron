package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
	"github.com/ohchanwu/jobcron/internal/scoring"
	"github.com/ohchanwu/jobcron/internal/scraper"
	"github.com/ohchanwu/jobcron/internal/storage"
	"github.com/ohchanwu/jobcron/web"
)

// scrapeNewCap bounds how many new postings one scrape will detail-fetch — a
// defensive limit; the 점핏 신입 universe is well under it.
const scrapeNewCap = 50

// Edited-JD refresh (T7). The scrape detail-fetches NEW postings only; an
// already-seen posting whose JD the employer later edits never gets re-fetched,
// so its content_hash — and thus its cached Stage-1 extraction and score — stay
// frozen at first sight. To catch edits without re-fetching everything (cost +
// politeness) or nothing (the staleness), each scrape re-fetches up to
// detailRefreshCap already-seen postings PER SOURCE, choosing the ones whose
// detail is oldest, and only those at least detailRefreshMinAge stale (so a
// same-day re-scrape doesn't re-fetch what it just fetched). Oldest-first
// rotation guarantees every posting's detail is rechecked within a bounded
// number of scrapes; JD edits are infrequent, so a day-plus latency to catch one
// is fine for a calm daily briefing. For full-listing sources (데모데이 /
// Greenhouse / 그리팅, whose FetchDetail is a no-op) the re-fetch is free and
// still picks up the listing's current text; for 점핏 / 랠릿 it is a real
// 1-req/s detail fetch, bounded by the cap.
const (
	detailRefreshCap    = 10
	detailRefreshMinAge = 24 * time.Hour
)

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

func aiRunTokenCapForUSDCents(cents int) int {
	return tokenCapForUSDCents(cents, profile.DefaultAIRunUSDCents, defaultRunTokenCap)
}

func aiDailyTokenCapForUSDCents(cents int) int {
	return tokenCapForUSDCents(cents, profile.DefaultAIDailyUSDCents, profile.DefaultDailyTokenCap)
}

func aiMonthlyTokenCapForUSDCents(cents int) int {
	return tokenCapForUSDCents(cents, profile.DefaultAIDailyUSDCents, profile.DefaultDailyTokenCap)
}

func tokenCapForUSDCents(cents, defaultCents, defaultTokens int) int {
	if cents <= 0 {
		cents = defaultCents
	}
	if defaultCents <= 0 {
		return defaultTokens
	}
	cap := (defaultTokens * cents) / defaultCents
	if cap < 1 {
		return 1
	}
	return cap
}

func minPositive(a, b int) int {
	if a <= 0 {
		return b
	}
	if b <= 0 || a < b {
		return a
	}
	return b
}

// Server wires storage, one or more scrapers, and the HTTP handlers together.
type Server struct {
	store        *storage.Store
	sources      []scraper.Scraper
	tmpl         *template.Template
	flight       *singleFlight
	rerates      *rerateTracker
	csrfSecret   []byte
	loginLimiter *loginRateLimiter

	// AI extraction (BYOK, v2.0). ai is nil when no provider is configured —
	// the default — and the pipeline behaves exactly like v1.5 (regex scoring,
	// no provider calls). ReconfigureAI builds the provider from ai_keys.json +
	// the profile's chosen provider/model; main.go calls it at startup and
	// handleProfileSave on every save, so a key entered in the form goes live
	// without a restart.
	ai                ai.Provider
	aiModel           string
	aiVersion         string // ai.AIVersion(ai.Name(), aiModel), precomputed
	aiRunTokenCap     int    // per-run, in-memory token ceiling (hard cap)
	aiDailyTokenCap   int    // rolling daily token ceiling, enforced against the persisted ai_usage ledger (hard cap)
	aiMonthlyTokenCap int    // rolling monthly token ceiling, derived from the persisted ai_usage ledger (hard cap)
	aiPerCallCap      int    // 재평가: not-yet-analyzed rows analyzed per press (legibility knob, not a hard cap)
	aiKeysPath        string // ai_keys.json location; empty = ai.DefaultKeysPath() (tests override)
	demoMode          bool   // read-only public demo mode
	productionMode    bool   // require owner login for protected HTTP routes
	adminToken        string // optional safety token for operator GET mutators in demo mode
	proxySecret       string // optional shared secret that allows Caddy forwarded-client headers
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
// so they never touch the real ~/.../jobcron/ai_keys.json.
func (s *Server) SetAIKeysPath(path string) { s.aiKeysPath = path }

// SetDemoMode makes the HTTP surface read-only. Visitor bookmark/hide state is
// handled by browser localStorage in this mode; no request should mutate the DB.
func (s *Server) SetDemoMode(on bool) { s.demoMode = on }

// SetProductionMode requires cookie-session authentication for protected pages.
func (s *Server) SetProductionMode(on bool) { s.productionMode = on }

// SetSessionSecret makes security tokens derive from the configured production
// SESSION_SECRET. New still creates a random development secret so tests and
// local non-production runs do not need configuration.
func (s *Server) SetSessionSecret(secret []byte) {
	if len(secret) == 0 {
		return
	}
	s.csrfSecret = append([]byte(nil), secret...)
}

// SetAdminToken sets the operator token accepted by /api/scrape in demo mode.
// An empty token means the endpoint is refused like every other mutator.
func (s *Server) SetAdminToken(token string) { s.adminToken = token }

// SetProxySecret allows the app to trust forwarded client-IP headers stamped
// by the configured reverse proxy. Leave empty unless the proxy injects the
// same secret and the app is not directly exposed to the public internet.
func (s *Server) SetProxySecret(secret string) { s.proxySecret = strings.TrimSpace(secret) }

func (s *Server) validAdminToken(r *http.Request) bool {
	if s.adminToken == "" {
		return false
	}
	got := r.URL.Query().Get("token")
	if got == "" {
		got = r.Header.Get("X-Jobcron-Admin-Token")
	}
	wantHash := sha256.Sum256([]byte(s.adminToken))
	gotHash := sha256.Sum256([]byte(got))
	return subtle.ConstantTimeCompare(wantHash[:], gotHash[:]) == 1
}

// keysPath returns the configured ai_keys.json path, falling back to the OS
// default.
func (s *Server) keysPath() (string, error) {
	if s.aiKeysPath != "" {
		return s.aiKeysPath, nil
	}
	return ai.DefaultKeysPath()
}

// ReconfigureAI (re)builds the AI provider from the saved profile + ai_keys.json
// and applies the profile's AI budget caps. It is the single wiring point:
// main.go calls it once at startup, handleProfileSave on every save. AI is left
// OFF (provider nil, silent regex fallback) when the profile selects no provider
// or the selected provider has no saved key. A bad provider name / build error
// also leaves AI off and is returned for the caller to log — never fatal.
func (s *Server) ReconfigureAI(ctx context.Context, userIDOpt ...int64) error {
	prof, ok, err := s.loadProfile(ctx, userIDOpt...)
	if err != nil {
		return err
	}
	if ok {
		s.aiRunTokenCap = aiRunTokenCapForUSDCents(prof.EffectiveAIRunUSDCapCents())
		s.aiDailyTokenCap = minPositive(
			prof.EffectiveAIDailyTokenCap(),
			aiDailyTokenCapForUSDCents(prof.EffectiveAIDailyUSDCapCents()),
		)
		s.aiMonthlyTokenCap = aiMonthlyTokenCapForUSDCents(prof.EffectiveAIMonthlyUSDCapCents())
		s.aiPerCallCap = prof.EffectiveAIPerCallCap()
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
	p, err := ai.New(prof.AIProvider, key, model, ai.SuggestedRateLimit(prof.AIProvider))
	if err != nil {
		s.SetAIProvider(nil, "")
		return err
	}
	s.SetAIProvider(p, model)
	return nil
}

func optionalUserID(userIDOpt []int64) int64 {
	if len(userIDOpt) == 0 {
		return 0
	}
	return userIDOpt[0]
}

func (s *Server) profileJSON(ctx context.Context, userID int64) (string, string, bool, error) {
	if userID == 0 {
		return s.store.Profile(ctx)
	}
	return s.store.ProfileForUser(ctx, userID)
}

func (s *Server) saveProfileJSON(ctx context.Context, userID int64, canonical string) (string, bool, error) {
	if userID == 0 {
		return s.store.SaveProfile(ctx, canonical)
	}
	return s.store.SaveProfileForUser(ctx, userID, canonical)
}

func (s *Server) scoresByPostingID(ctx context.Context, userID int64) (map[int64]storage.Score, error) {
	if userID == 0 {
		return s.store.ScoresByPostingID(ctx)
	}
	return s.store.ScoresByPostingID(ctx, userID)
}

func (s *Server) bookmarkedIDs(ctx context.Context, userID int64) (map[int64]bool, error) {
	if userID == 0 {
		return s.store.BookmarkedIDs(ctx)
	}
	return s.store.BookmarkedIDsForUser(ctx, userID)
}

func (s *Server) notInterestedIDs(ctx context.Context, userID int64) (map[int64]bool, error) {
	if userID == 0 {
		return s.store.NotInterestedIDs(ctx)
	}
	return s.store.NotInterestedIDs(ctx, userID)
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
		store:             store,
		sources:           sources,
		flight:            newSingleFlight(),
		rerates:           newRerateTracker(),
		csrfSecret:        newCSRFSecret(),
		loginLimiter:      newLoginRateLimiter(),
		aiRunTokenCap:     defaultRunTokenCap,
		aiDailyTokenCap:   profile.DefaultDailyTokenCap,
		aiMonthlyTokenCap: aiMonthlyTokenCapForUSDCents(profile.DefaultAIMonthlyUSDCents),
		aiPerCallCap:      profile.DefaultAIPerCallCap,
	}
	funcs := template.FuncMap{
		"sourceLabel":       sourceLabel,
		"registeredSources": srv.allRegisteredSources,
		"sourcePillGroups":  srv.sourcePillGroups,
		"absInt":            absInt,
		"usdCents":          usdCents,
		"demoMode":          func() bool { return srv.demoMode },
		"productionMode":    func() bool { return srv.productionMode },
		"navData":           func(active, csrfToken string) navView { return navView{Active: active, CSRFToken: csrfToken} },
	}
	srv.tmpl = template.Must(template.New("").Funcs(funcs).ParseFS(web.FS, "*.html"))
	return srv
}

// ScrapeResult summarizes one scrape run across every active source.
type ScrapeResult = storage.ScrapeResult

// scrapeAllKey is the singleflight key for a multi-source scrape run. We
// hold one global lock for the whole pipeline rather than one per source —
// the user clicks one button, sees one progress stream, and would be
// confused by partial states. Per-source locks would matter if scrapes were
// triggered independently.
const scrapeAllKey = "_all_"

func (s *Server) runScrapeWithHistory(ctx context.Context, trigger string, emit func(event, data string), userIDOpt ...int64) (result ScrapeResult, err error) {
	run, startErr := s.store.StartScrapeRun(ctx, trigger)
	if startErr != nil {
		return ScrapeResult{}, startErr
	}
	status := storage.ScrapeRunStatusSuccess
	errorSummary := ""
	defer func() {
		if r := recover(); r != nil {
			status = storage.ScrapeRunStatusFailure
			err = fmt.Errorf("server: scrape panic: %v", r)
			errorSummary = err.Error()
		} else if err != nil {
			status = storage.ScrapeRunStatusFailure
			errorSummary = err.Error()
		}
		finishCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if finishErr := s.store.FinishScrapeRun(finishCtx, run.ID, result, status, truncateScrapeRunError(errorSummary)); finishErr != nil && err == nil {
			err = finishErr
		}
	}()
	return s.runScrapeForTrigger(ctx, trigger, emit, userIDOpt...)
}

func truncateScrapeRunError(s string) string {
	const max = 500
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// runScrape executes the full pipeline across every enabled source: robots
// check → listing → detail fetch → upsert → sweep → score. Disabled sources
// are skipped entirely and their data is frozen in the DB so re-enabling a
// source does not require a fresh scrape.
func (s *Server) runScrape(ctx context.Context, emit func(event, data string), userIDOpt ...int64) (ScrapeResult, error) {
	return s.runScrapeForTrigger(ctx, storage.ScrapeTriggerManual, emit, userIDOpt...)
}

func (s *Server) runScrapeForTrigger(ctx context.Context, trigger string, emit func(event, data string), userIDOpt ...int64) (ScrapeResult, error) {
	userID := optionalUserID(userIDOpt)
	prof, profileOK, err := s.loadProfile(ctx, userID)
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
	freshAI := s.freshAIAllowedForTrigger(trigger, profileOK, prof)
	budget := s.newAIBudget(ctx)
	if !freshAI {
		budget = nil
	}
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
		res.Refreshed += sub.Refreshed
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

	// Cold-start banner (D6): if the per-run AI budget ran out mid-scrape during
	// Stage-1 extraction, some postings were scored by regex instead of AI. A
	// mixed briefing is fine — surface it calmly, never as a failure.
	if budget != nil && budget.isDegraded() {
		emit("status", "오늘 AI 예산을 다 써서 일부는 일반 점수로 분석했어요 — 프로필 설정에서 한도를 바꿀 수 있어요.")
	}

	emit("status", "공고에 점수를 매기는 중...")
	scored, err := s.scoreAll(ctx, userID)
	if err != nil {
		return res, err
	}
	res.Scored = scored

	// Auto-rate the fresh briefing with Stage-2 so new postings show their
	// evidence-cited AI chip without a manual 재평가. Runs AFTER the offline
	// scoreAll (so "visible" reflects real scores), over the today surface only,
	// through the same worker pool as 재평가. A fresh run budget gives Stage-2 its
	// own per-run cap; the daily cap still accounts for the scrape's Stage-1 spend
	// (newAIBudget re-reads the ledger). aiPerCallCap bounds the spend per scrape;
	// the rest stays for a manual 재평가.
	if freshAI {
		if vis, verr := s.visibleForRerate(ctx, "today", now, userID); verr == nil && len(vis) > 0 {
			emit("status", "새 공고를 AI로 분석하는 중...")
			rateBudget := s.newAIBudget(ctx)
			rated, provErr := s.rateStage2(ctx, vis, prof, rateBudget, emit)
			if rated > 0 {
				// Merge the fresh Stage-2 deltas into the rendered scores.
				if rescored, rerr := s.scoreAll(ctx, userID); rerr == nil {
					res.Scored = rescored
				}
			}
			if provErr != nil {
				// The auto-rate hit a provider error (bad key/model after a switch).
				// Surface it calmly inline — the scrape itself still succeeds.
				emit("status", providerFailureMessage(provErr))
			}
			if rateBudget != nil && rateBudget.isDegraded() {
				emit("status", "오늘 AI 예산을 다 써서 일부 공고는 아직 분석하지 못했어요 — 프로필 설정에서 한도를 바꿀 수 있어요.")
			}
		}
	}
	return res, nil
}

func (s *Server) freshAIAllowedForTrigger(trigger string, profileOK bool, prof profile.Profile) bool {
	if s.ai == nil || !profileOK {
		return false
	}
	if trigger == storage.ScrapeTriggerScheduled {
		return prof.ScheduledAIEnabled
	}
	return true
}

// aiBudget gates AI spending for one run against two ceilings: a per-run,
// in-memory cap (runCap) and a rolling daily cap (dailyCap) enforced against the
// persisted ai_usage ledger. The daily total at the run's START is read once
// (dailyAtStart); each admitted call's tokens are added to runSpent AND written
// through to the ledger, so the cap holds across process restarts and across
// runs in the same day. (Within a single day, concurrent AI runs are kept from
// double-spending the daily budget by the scrape⟷re-rate exclusion — T7.)
type aiBudget struct {
	store          *storage.Store
	day            string // UTC date, e.g. "2026-06-03"
	month          string // UTC month, e.g. "2026-06"
	runCap         int
	dailyCap       int
	monthlyCap     int
	dailyAtStart   int // ledger total (input+output) when the run began
	monthlyAtStart int // ledger total for the UTC month when the run began

	// mu guards the mutable counters so the concurrent 재평가 worker pool
	// (handleRerateSSE) can canSpend/debit from several goroutines without a
	// data race. The scrape path is sequential, so the lock is uncontended
	// there. Optimistic overshoot is bounded: workers that pass canSpend
	// before an in-flight call debits can collectively exceed the cap by at
	// most (pool size) calls — acceptable for a soft token ceiling.
	mu       sync.Mutex
	runSpent int  // input+output debited this run
	degraded bool // true once either cap was hit mid-run
}

// newAIBudget returns a fresh per-run budget, or nil when AI is off (so the
// scrape loop skips all AI bookkeeping). It reads the day's ledger total once so
// the daily cap accounts for spend from earlier runs (and earlier process
// lifetimes) the same day.
func (s *Server) newAIBudget(ctx context.Context) *aiBudget {
	if s.ai == nil {
		return nil
	}
	now := time.Now().UTC()
	day := now.Format("2006-01-02")
	month := now.Format("2006-01")
	in, out, err := s.store.AIUsageForDay(ctx, day)
	if err != nil {
		in, out = 0, 0 // a ledger read error must not block scoring — start from 0
	}
	monthIn, monthOut, err := s.store.AIUsageForMonth(ctx, month)
	if err != nil {
		monthIn, monthOut = 0, 0
	}
	return &aiBudget{
		store:          s.store,
		day:            day,
		month:          month,
		runCap:         s.aiRunTokenCap,
		dailyCap:       s.aiDailyTokenCap,
		monthlyCap:     s.aiMonthlyTokenCap,
		dailyAtStart:   in + out,
		monthlyAtStart: monthIn + monthOut,
	}
}

// canSpend reports whether either cap still has headroom, marking the run
// degraded the first time it does not (so the caller surfaces the calm banner).
func (b *aiBudget) canSpend() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.runSpent >= b.runCap ||
		b.dailyAtStart+b.runSpent >= b.dailyCap ||
		b.monthlyAtStart+b.runSpent >= b.monthlyCap {
		b.degraded = true
		return false
	}
	return true
}

// debit records a call's token usage: against the in-memory run counter and
// (best-effort) through to the persisted daily ledger. A ledger write failure is
// swallowed — it must never fail a scrape — but the in-memory run cap still
// holds for the rest of this run. The ledger write happens outside the lock so a
// slow disk write does not serialize the worker pool's budget checks.
func (b *aiBudget) debit(ctx context.Context, u ai.Usage) {
	b.mu.Lock()
	b.runSpent += u.InputTokens + u.OutputTokens
	b.mu.Unlock()
	_ = b.store.AddAIUsage(ctx, b.day, u.InputTokens, u.OutputTokens)
}

// isDegraded reports whether either cap was hit during the run.
func (b *aiBudget) isDegraded() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.degraded
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
	seen, err := s.store.SeenDetail(ctx, src.Source())
	if err != nil {
		return ScrapeResult{}, err
	}
	res := ScrapeResult{Listed: len(listing)}

	// refreshCand is an already-seen posting eligible for an edited-JD re-fetch:
	// the listing posting (carries the id/url FetchDetail needs) plus its stored
	// row id and how stale its detail is.
	type refreshCand struct {
		p     scraper.Posting
		id    int64
		detAt time.Time
	}
	var fresh []scraper.Posting
	var cands []refreshCand
	staleBefore := now.Add(-detailRefreshMinAge)
	for _, p := range listing {
		if info, ok := seen[p.SourcePostingID]; ok {
			p.LastSeenAt = now
			if _, _, err := s.store.UpsertPosting(ctx, p); err != nil {
				return res, fmt.Errorf("server: refresh seen posting: %w", err)
			}
			if info.DetailFetchedAt.Before(staleBefore) {
				cands = append(cands, refreshCand{p: p, id: info.ID, detAt: info.DetailFetchedAt})
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

	// Edited-JD refresh (T7): re-fetch the detail of the stalest already-seen
	// postings (oldest detail_fetched_at first, capped per source). A changed JD
	// flows new content_hash → fresh Stage-1 extraction → re-score; an unchanged
	// JD is a cheap no-op (content_hash matches, extraction cache hits). The
	// per-source cap bounds the added 1-req/s detail fetches for 점핏/랠릿; it is
	// free for the no-op-FetchDetail sources.
	sort.Slice(cands, func(i, j int) bool { return cands[i].detAt.Before(cands[j].detAt) })
	if len(cands) > detailRefreshCap {
		cands = cands[:detailRefreshCap]
	}
	for i, c := range cands {
		detailed, err := src.FetchDetail(ctx, c.p)
		if err != nil {
			continue // transient detail failure — try again a later scrape
		}
		detailed.LastSeenAt = now
		if err := s.store.RefreshPostingDetail(ctx, c.id, detailed, now); err != nil {
			return res, fmt.Errorf("server: refresh posting detail: %w", err)
		}
		res.Refreshed++
		emit("progress", fmt.Sprintf("[%s] 기존 공고 새로고침 %d/%d...", label, i+1, len(cands)))
		s.extractStage1(ctx, c.id, detailed, now, budget)
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
func (s *Server) loadProfile(ctx context.Context, userIDOpt ...int64) (profile.Profile, bool, error) {
	userID := optionalUserID(userIDOpt)
	jsonStr, _, ok, err := s.profileJSON(ctx, userID)
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

// RescoreAll re-scores every stored posting against the current profile. main
// calls it once at startup (right after ReconfigureAI) so a posting left
// unscored by an INTERRUPTED scrape is healed on the next boot rather than
// rendering as a blank card. Fix 2A keeps CLIENT navigation from leaving the
// unscored state; this covers PROCESS death — a crash / SIGKILL / OOM / deploy
// restart in the window between UpsertPosting committing a new row and the
// end-of-run scoreAll, which nothing else self-heals. It is the exported entry
// point to scoreAll; it never calls the AI provider (merge-only, D10), so the
// startup pass is a cheap cache-only re-score. No-op when no profile is saved.
func (s *Server) RescoreAll(ctx context.Context) (int, error) {
	return s.scoreAll(ctx)
}

// scoreAll scores every stored posting against the current profile and upserts
// the score rows. It is a no-op when no profile has been saved yet.
func (s *Server) scoreAll(ctx context.Context, userIDOpt ...int64) (int, error) {
	userID := optionalUserID(userIDOpt)
	profJSON, profHash, ok, err := s.profileJSON(ctx, userID)
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
	// Batch-load the Stage-1 extractions and Stage-2 deltas once (no N+1) when
	// AI is enabled; scoreCareer/educationDealbreaker prefer the extractions,
	// and the deltas merge as the "AI 분석" line item. This is merge-only — D10:
	// scoreAll NEVER calls the provider; the provider runs only at scrape time
	// (Stage 1) and on a 재평가 (Stage 2, T7).
	var (
		exts         map[int64]ai.Extraction
		freshDeltas  map[int64]ai.Delta // keyed by the CURRENT goal text (ai_input_hash) + current ai_version
		latestDeltas map[int64]ai.Delta // newest per posting under the current ai_version, any goal text — stale fallback for a goal edit
		anyVerDeltas map[int64]ai.Delta // newest per posting across ANY ai_version — stale fallback for a provider/model switch
	)
	if s.ai != nil {
		exts, err = s.store.AIExtractionsByPostingID(ctx, s.aiVersion)
		if err != nil {
			return 0, err
		}
		aiInputHash := profile.AIInputHash(prof)
		freshDeltas, err = s.store.AIScoresByPostingID(ctx, aiInputHash, s.aiVersion)
		if err != nil {
			return 0, err
		}
		latestDeltas, err = s.store.LatestAIScoresByPostingID(ctx, s.aiVersion)
		if err != nil {
			return 0, err
		}
		// Cross-version fallback: when the user switches provider/model, ai_version
		// rotates and orphans every prior row from the two version-scoped lookups
		// above. Without this, the AI chip would VANISH on a provider switch; with
		// it, the latest prior reading persists faded ("이전 설정 기준") until a
		// 재평가 refreshes it under the new provider/model.
		anyVerDeltas, err = s.store.LatestAIScoresAnyVersionByPostingID(ctx)
		if err != nil {
			return 0, err
		}
	} else if s.demoMode {
		// The public demo runs without ai_keys.json, but the uploaded database
		// already contains tonight's cached Stage-2 deltas. Merge the newest
		// cached row as current so the first server boot does not erase the
		// visible AI chips.
		anyVerDeltas, err = s.store.LatestAIScoresAnyVersionByPostingID(ctx)
		if err != nil {
			return 0, err
		}
	}
	for _, p := range postings {
		var ext *ai.Extraction
		if e, ok := exts[p.ID]; ok {
			ext = &e
		}
		// Prefer a delta computed against the CURRENT goal text + provider/model
		// (fresh). Else fall back, marked Stale (still summed into the Total — T1),
		// so the chip reads "(이전 설정 기준)": first to the latest row under the
		// current ai_version (a goal edit), then to the latest row under ANY
		// ai_version (a provider/model switch, which rotated ai_version and orphaned
		// the prior rows). The cross-version tier is what keeps the chip from
		// vanishing when the user changes provider.
		var delta *ai.Delta
		if d, ok := freshDeltas[p.ID]; ok {
			d.Stale = false
			delta = &d
		} else if d, ok := latestDeltas[p.ID]; ok {
			d.Stale = true
			delta = &d
		} else if d, ok := anyVerDeltas[p.ID]; ok {
			d.Stale = !(s.demoMode && s.ai == nil)
			delta = &d
		}
		result := scoring.Score(p, prof, ext, delta)
		breakdown, err := json.Marshal(result)
		if err != nil {
			return 0, fmt.Errorf("server: marshal score: %w", err)
		}
		sc := storage.Score{
			PostingID:     p.ID,
			ProfileHash:   profHash,
			Total:         result.Total,
			BreakdownJSON: string(breakdown),
			ComputedAt:    time.Now().UTC(),
		}
		if userID == 0 {
			err = s.store.UpsertScore(ctx, sc)
		} else {
			err = s.store.UpsertScoreForUser(ctx, userID, sc)
		}
		if err != nil {
			return 0, fmt.Errorf("server: save score: %w", err)
		}
	}
	return len(postings), nil
}
