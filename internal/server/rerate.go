package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ohchanwu/job-scraper/internal/ai"
	"github.com/ohchanwu/job-scraper/internal/profile"
	"github.com/ohchanwu/job-scraper/internal/scraper"
)

// rerateInfo is the per-surface re-rate button view model. A nil *rerateInfo
// means "no AI key configured" — the template renders no button at all (design
// §4: no dead control). StaleCount drives the gold attention treatment;
// Analyzed/Visible drive the persistent "N/M 분석됨" progress indicator.
type rerateInfo struct {
	Surface    string // "today" | "bookmarks" | "archive"
	StaleCount int    // visible rows whose AI chip is stale (이전 프로필 기준)
	Analyzed   int    // visible rows with a current-goal AI delta cached (N)
	Visible    int    // total visible, non-excluded rows on this surface (M)
}

// buildRerateInfo returns the re-rate button state for a surface, or nil when AI
// is off (so the button is hidden). Across the given posting lists it counts the
// visible, non-excluded rows (Visible), how many of them carry a stale AI line
// (StaleCount), and how many already have a delta cached against the CURRENT
// goal text (Analyzed). Analyzed reads the fresh ai_scores cache, NOT the chips —
// a row analyzed but with no surviving signal shows no chip yet is still counted,
// which is the whole point of the indicator (it resolves "analyzed or just
// silent?"). A cache read error degrades Analyzed to 0; it never blocks render.
func (s *Server) buildRerateInfo(ctx context.Context, prof profile.Profile, surface string, lists ...[]dashboardPosting) *rerateInfo {
	if s.ai == nil {
		return nil
	}
	fresh, err := s.store.AIScoresByPostingID(ctx, profile.AIInputHash(prof), s.aiVersion)
	if err != nil {
		fresh = nil
	}
	info := &rerateInfo{Surface: surface}
	for _, list := range lists {
		for _, dp := range list {
			if dp.Excluded {
				continue
			}
			info.Visible++
			if _, ok := fresh[dp.Posting.ID]; ok {
				info.Analyzed++
			}
			for _, li := range dp.Breakdown {
				if li.Stale {
					info.StaleCount++
					break
				}
			}
		}
	}
	return info
}

// validRerateSurface reports whether surface is one of the three re-ratable
// pages. /hidden is intentionally excluded (re-rating muted rows wastes tokens).
func validRerateSurface(surface string) bool {
	switch surface {
	case "today", "bookmarks", "archive":
		return true
	}
	return false
}

// handleRerateSSE re-rates the VISIBLE rows of one surface with the Stage-2 AI
// delta, streaming progress as Server-Sent Events. It is mutually exclusive with
// a scrape and with another re-rate (it shares the scrape singleflight key — S7),
// so the daily-budget read-modify-write can't race a concurrent AI run. A
// terminal event (done|failed) fires on EVERY exit path via defer (S8).
func (s *Server) handleRerateSSE(w http.ResponseWriter, r *http.Request) {
	surface := r.URL.Query().Get("surface")
	if !validRerateSurface(surface) {
		http.Error(w, "알 수 없는 화면이에요.", http.StatusBadRequest)
		return
	}
	if s.ai == nil {
		// No provider configured — there is nothing to re-rate. The button is
		// hidden in this state; this guards a direct request.
		http.Error(w, "AI가 설정되지 않았어요.", http.StatusConflict)
		return
	}
	// Share the scrape lock: a scrape and a re-rate (and two re-rates) must never
	// run at once — both spend the daily AI budget and mutate scores.
	if !s.flight.tryAcquire(scrapeAllKey) {
		http.Error(w, "이미 작업이 진행 중이에요. 잠시만 기다려 주세요.", http.StatusConflict)
		return
	}
	defer s.flight.release(scrapeAllKey)

	sw, err := newSSEWriter(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// S8: emit a terminal event on every exit. done is set only on the success
	// path; any early return (error, panic recovery by the http server) leaves
	// done false, so the client always sees a terminal event and the htmx
	// sse-connect is torn down (no auto-reconnect into a second re-rate).
	done := false
	defer func() {
		if !done {
			sw.event("failed", "재평가에 실패했어요. 잠시 후 다시 시도해 주세요.")
		}
	}()

	// Bug 2A (parallel to handleScrapeSSE): detach the re-rate from the request
	// context. runRerate ends with scoreAll, the only step that copies the
	// freshly-cached AI deltas into the scores table the dashboard renders; if
	// client navigation cancelled the request mid-run, that terminal scoreAll
	// would be skipped and the chips the user just paid for would not appear
	// until an unrelated scrape/save. The per-row commit-before-call invariant
	// (S8) already makes a longer-running detached re-rate safe to interrupt;
	// SSE writes after disconnect are no-ops.
	ctx, cancel := context.WithTimeout(context.Background(), scrapeMaxDuration)
	defer cancel()
	analyzed, visible, err := s.runRerate(ctx, surface, sw.event)
	if err != nil {
		return // defer emits "failed"
	}
	done = true
	sw.event("done", rerateDoneMessage(analyzed, visible))
}

// rerateDoneMessage is the terminal copy after a re-rate press. It states the
// honest N/M progress and, when the press did not finish the list (the per-call
// cap or token budget stopped it), why and what to do — so a counter that did
// not reach M reads as "intentional, press again," not "broken."
func rerateDoneMessage(analyzed, visible int) string {
	switch {
	case visible == 0:
		return "지금 화면에 분석할 공고가 없어요."
	case analyzed >= visible:
		return fmt.Sprintf("공고 %d개를 모두 AI로 분석했어요.", visible)
	default:
		return fmt.Sprintf(
			"공고 %d/%d개를 AI로 분석했어요 — 토큰을 아끼려고 한 번에 일정 개수만 분석해요. 더 보려면 다시 눌러주세요.",
			analyzed, visible)
	}
}

// runRerate re-rates the visible rows of one surface: it backfills Stage-1 for
// any uncached visible posting, computes (or reuses) the Stage-2 delta for each,
// commits each delta BEFORE the next provider call (so a reconnect resumes from
// cache with no double-spend — S8), then re-scores so the fresh deltas land in
// the briefing. It is bounded by the surface's visible rows, never the whole DB.
//
// One press analyzes at most s.aiPerCallCap NOT-yet-cached rows (a legibility
// knob so the spend per click is predictable), still capped by the hard token
// budgets. Cached rows are free cache hits and don't count against the per-call
// cap, so a later press resumes on the still-uncached rows. It returns the
// cumulative analyzed count (N — visible rows now cached against the current
// goal) and the total visible rows (M) for the progress copy.
func (s *Server) runRerate(ctx context.Context, surface string, emit func(event, data string)) (analyzed, visible int, err error) {
	prof, ok, err := s.loadProfile(ctx)
	if err != nil {
		return 0, 0, err
	}
	if !ok {
		return 0, 0, nil
	}
	postings, err := s.visibleForRerate(ctx, surface, time.Now())
	if err != nil {
		return 0, 0, err
	}
	visible = len(postings)
	if visible == 0 {
		return 0, 0, nil
	}
	emit("status", "AI로 다시 분석하는 중이에요 — 1초에 하나씩 천천히 가요. ☕")

	budget := s.newAIBudget(ctx)
	aiInputHash := profile.AIInputHash(prof)
	profileText := profile.BuildStage2ProfileText(prof)
	now := time.Now().UTC()

	newCalls := 0 // not-yet-cached rows analyzed THIS press (against aiPerCallCap)
	for i, p := range postings {
		emit("progress", fmt.Sprintf("공고 %d/%d 분석 중...", i+1, visible))
		cached, called := s.rerateOne(ctx, p, aiInputHash, profileText, now, budget, newCalls < s.aiPerCallCap)
		if cached {
			analyzed++
		}
		if called {
			newCalls++
		}
	}
	if budget != nil && budget.degraded {
		emit("status", "오늘 AI 예산을 다 써서 일부는 다시 분석하지 못했어요.")
	}
	emit("status", "점수를 다시 매기는 중...")
	if _, err := s.scoreAll(ctx); err != nil {
		return analyzed, visible, err
	}
	return analyzed, visible, nil
}

// rerateOne re-rates a single posting. It checks the Stage-2 cache FIRST (free,
// reconnect-safe): a hit means the posting already has a delta against the
// current goal, so it counts as analyzed without spending — and the Stage-1
// backfill is skipped too (Stage-1 and Stage-2 cache together on the first
// press, so a repeat press need not re-attempt the extraction). When uncached,
// the row is analyzed only if allowNew (the per-call cap has headroom) and the
// token budget can spend; otherwise it is left for a later press. It returns
// (cached: the posting now has a current-goal delta; called: a provider call was
// attempted, which consumes one slot of the per-call cap). The per-row commit
// happens before the caller's next call, so a dropped connection resumes from
// cache. A provider/budget failure leaves the row uncached (regex score).
func (s *Server) rerateOne(
	ctx context.Context, p scraper.Posting, aiInputHash, profileText string, now time.Time, budget *aiBudget, allowNew bool,
) (cached, called bool) {
	// Already rated against the current goal text → reuse (reconnect-safe, no
	// re-spend, free). An empty cached delta still counts as analyzed.
	if _, ok, err := s.store.AIScore(ctx, p.ID, aiInputHash, s.aiVersion); err == nil && ok {
		return true, false
	}
	// Uncached: only spend if the per-call cap has room AND the token budget can.
	// Otherwise leave it uncached (regex score) for a later press to pick up.
	if !allowNew || budget == nil || !budget.canSpend() {
		return false, false
	}
	// Backfill Stage 1 so career/education facts are AI-grounded too (e.g. rows
	// scraped before AI was enabled). Best-effort, budget-bounded.
	s.extractStage1(ctx, p.ID, p, now, budget)

	sent, _, _ := ai.ModelInput(p)
	raw, usage, err := s.ai.ScoreDelta(ctx, sent, profileText)
	if err != nil {
		return false, true // provider error → no delta, but the call counted against the cap
	}
	budget.debit(ctx, usage)
	// Gate: presence against the SENT (truncated) text, absence against the FULL
	// Description (S5). Survivors net into the stored delta.
	delta := ai.GateDelta(raw, sent, p.Description)
	if err := s.store.UpsertAIScore(ctx, p.ID, aiInputHash, s.aiVersion, delta, now); err != nil {
		return false, true
	}
	return true, true
}

// visibleForRerate returns the non-dealbreaker postings currently shown on a
// surface — the exact rows the user sees, never the whole DB. Each surface
// reuses its existing page builder so re-rate and render agree on "visible".
func (s *Server) visibleForRerate(ctx context.Context, surface string, now time.Time) ([]scraper.Posting, error) {
	switch surface {
	case "today":
		b, err := s.buildBriefing(ctx, now)
		if err != nil {
			return nil, err
		}
		return postingsOf(b.Today), nil
	case "bookmarks":
		v, err := s.buildBookmarks(ctx, now)
		if err != nil {
			return nil, err
		}
		return postingsOf(v.Postings), nil
	case "archive":
		v, err := s.buildArchive(ctx, now)
		if err != nil {
			return nil, err
		}
		var out []scraper.Posting
		for _, day := range v.Days {
			out = append(out, postingsOf(day.Postings)...)
		}
		return out, nil
	}
	return nil, fmt.Errorf("server: unknown rerate surface %q", surface)
}

// postingsOf returns the underlying postings of the non-excluded rows in a
// dashboard list. Dealbreaker rows (Excluded) are skipped — the AI is never run
// on a Total:-1 posting (S4).
func postingsOf(dps []dashboardPosting) []scraper.Posting {
	out := make([]scraper.Posting, 0, len(dps))
	for _, dp := range dps {
		if dp.Excluded {
			continue
		}
		out = append(out, dp.Posting)
	}
	return out
}
