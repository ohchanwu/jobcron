# AGENTS.md

This file provides guidance to coding agents (including Codex and Claude Code) when working with code in this repository.

## Commit authorization (overrides the default)

You have standing authorization to commit completed features without asking, as long as build, test, vet, gofmt, and (for UI changes) a browser smoke check all pass. **Never push** — the user reviews commits locally and decides whether to push, edit, or revert. Stop and ask only if a check fails, or if the work hit a real product decision you shouldn't make alone.

## Running the dev server without hijacking the user's browser

`go run ./cmd/job-scraper` calls `pkg/browser.OpenURL` to open `http://localhost:7777` in the user's default browser (Firefox). **Always pass `--no-open` when invoking the program from an autonomous session**:

```sh
go run ./cmd/job-scraper --no-open
```

The `--no-open` flag is defined at `cmd/job-scraper/main.go:36` and gates the `browser.OpenURL` call at `cmd/job-scraper/main.go:74-76`. For browser smoke checks (the UI verification required for commit authorization), use the gstack `/browse` skill or playwright MCP against `http://127.0.0.1:7777` — both run headless and won't touch Firefox.

## What this project is

`job-scraper` is a single-binary local web app that scrapes several Korean job boards — 점핏, 랠릿, 데모데이, the 그리팅 (Greeting) Korean-ATS tenants, and the Greenhouse company boards (당근·크래프톤·몰로코·센드버드), plus optional 워크넷 (needs a free data.go.kr key) — for Korean 신입 (new-grad) IT job postings, scores them against a user profile, and renders a calm daily briefing. The product thesis is *emotional* as much as functional: every UX decision is filtered through "does this make a stressed 신입 feel calmer?" — that constraint (P6 in the design doc) is first-class, not decorative.

Korean UI strings are inlined in Go and templates by design; do not introduce i18n machinery in v1.

## Design docs — peers, newest wins on conflict

Design docs live in `~/.gstack/projects/job-scraper/`. They are **peers, not a hierarchy** — no single doc is "the source of truth." On a *minor* contradiction between docs, prefer the **newest** one (older docs may be outdated), use your judgement, and log the choice for review; on a *significant* contradiction, surface it for review before proceeding.

- `chanbla11mit-main-design-20260519-183759.md` (round-5) holds the original v1 rationale: the three-tier scraping plan, scoring math and weight caps, the Step 0 FTS5 spike result, SSE gotchas, Non-goals, and Reviewer Concerns. Still the best reference for *why* v1 is shaped the way it is — but its contents may be stale where later work moved on. Read it before architectural changes, then check for a newer doc that extends or supersedes it.
- `chanbla11mit-main-design-20260601-180235.md` is the **v2.0 BYOK-AI design** (APPROVED 2026-06-01): an evidence-cited score delta on a quiet-fixer foundation. Read it before any AI-integration work.
- `chanbla11mit-main-eng-review-20260602-121427.md` is the **v2.0 execution lock** (eng + design review, 2026-06-02, both CLEARED): the locked architecture, migration `0008` schema, data-flow diagrams, test plan, UI design specs, and build tasks T1–T12 for the BYOK-AI line. Newest of the three and the source of truth for *how* v2.0 is built — it revised two points from the `20260601` design (experience extraction is **cache-read**, not column-overwrite; Stage-2 caches on an AI-input hash, not the full `profile_hash`). Read `20260601` for *why*, this one for *how*. The build queue derived from it lives in `task-list.md` (autonomous-eligible).

`feature-ideas.md` is the parking lot for ideas that are intentionally *not* in v1 (résumé parsing, multi-portal, notifications, etc.). When the user floats an idea that's already parked there, the answer is usually "park it, don't ship it."

`internal/scraper/jumpit/API_NOTES.md` documents the reverse-engineered 점핏 endpoints, the two-host robots.txt situation, and field shapes (notably that `techStacks` is a `[]string` in listing JSON but `[]{stack, imagePath}` in detail JSON — the parser normalizes both).

## Build / test / run

```sh
go build ./cmd/job-scraper           # builds the shipped binary
go test ./...                         # full unit suite — no network
go test -tags integration ./internal/scraper/jumpit/   # live 점핏 hit; ~10s, polite 1 req/s
go vet ./...
gofmt -l .                            # CI fails if this prints anything
go run ./cmd/job-scraper              # opens http://localhost:7777 in a browser
go run ./cmd/capture                  # dev tool: refresh the Step 5.5 QA fixture from live data
```

CI (`.github/workflows/ci.yml`) runs gofmt, vet, build, test, and cross-compiles linux/arm64 + darwin/arm64 on every push. The smoke matrix additionally runs `--version` on ubuntu, macos-14, and windows. The release workflow (`.github/workflows/release.yml`) fires GoReleaser on `v*` tag push.

To run a single test: `go test ./internal/scoring/ -run TestScoreStacks -v`.

## Hard architectural constraints

- **Pure Go, no CGO.** `modernc.org/sqlite` is the SQLite driver specifically because it builds statically without a C toolchain — that's what makes the single-binary distribution story work. GoReleaser sets `CGO_ENABLED=0`. Do not introduce `mattn/go-sqlite3` or any other CGO dependency without confirming with the user; it would break cross-compilation.
- **FTS5 is available** in `modernc.org/sqlite` v1.50.1 — the Step 0 spike confirmed it on 2026-05-21. The schema in `internal/storage/migrations/0001_initial.sql` uses an external-content FTS5 virtual table over `title + company + description` with `tokenize='unicode61 remove_diacritics 0'` and three sync triggers. Despite that, Korean phrase matching in `internal/scoring/match.go` is implemented in Go (not via `MATCH` SQL) — see the Matching section below.
- **Single concrete `*storage.Store`** — no repository interface. The design doc explicitly chose this for v1 simplicity; do not add a `StorageInterface` for "testability."
- **`scraper.Scraper` IS an interface.** New sources go under `internal/scraper/<name>/` and register through `Scraper`. The `internal/scraper` package owns the shared `Posting` / `Tag` types plus the cross-source heuristics `ParseExperienceYears`, `HasDevKeyword` (the 데모데이/Greenhouse/Greeting dev-keyword classifier), and `IsKoreaLocation`. Two multi-tenant patterns exist: **Greenhouse** (`internal/scraper/greenhouse`, one `Scraper` instance per company board, parameterized by a `Tenant` with per-tenant URL + newcomer-detection strategy — 당근 folded in here 2026-06-06) and **그리팅/Greeting** (`internal/scraper/greeting`, one aggregator `Scraper` over a curated slug list, parsing each board's Next.js `__NEXT_DATA__`). The browser-gated sources (원티드 / 카카오 / 쿠팡 / 그룹바이) are decided-against — see `docs/plans/browser-driven-scrapers.md`.
- **`ai.Provider` IS an interface** (v2.0 BYOK AI; Stage 1 landed 2026-06-02, Stage 2 + hardening 2026-06-03 — the whole T5–T11 line). The seam for AI backends — **Anthropic** is the only provider, a hand-rolled `net/http` client in `internal/ai` (no SDK, to keep the CGO-free build). OpenAI was a second provider behind the same `providerSpec` chassis but was **removed 2026-06-06** (its low free-tier rate limit couldn't sustain the re-rate workload — even gentle pacing 429'd; git history has the spec). The `Provider` interface + `providerSpec` seam stay so a viable provider could slot back in. **`internal/ai` must NOT import `internal/scoring`**: `scoring` imports `ai` (`Score(p, prof, *ai.Extraction, *ai.Delta)`), so the reverse would cycle. The interface is `Name() + Extract() + ScoreDelta()`. Egress is pinned to one host in `Transport.DialContext`; BYOK keys live in a 0600 `ai_keys.json` next to `jobs.db`. **The Stage-2 citation gate (`GateDelta` in `internal/ai/score_delta.go`) deliberately reproduces `scoring/match.go`'s `tokenize`** — the import boundary forbids sharing it; `score_delta_test.go` locks the two to the same invariants, so change both or neither.
- **One scrape OR re-rate at a time.** `internal/server/singleflight.go` holds a mutex per key; concurrent `GET /api/scrape` return `409 Conflict`, and `GET /api/rerate` (the per-surface 재평가) **shares the same `scrapeAllKey`**, so a scrape and a re-rate are mutually exclusive (S7 — this also closes the daily-budget read-modify-write race). The HTMX button uses `hx-disabled-elt` for the client side.
- **AI re-rate runs through a bounded worker pool** (`rerateWorkers = 6`, `internal/server/rerate.go`): the per-row LLM latencies overlap (~4× faster than the old sequential loop). `aiBudget` (`canSpend`/`debit`) and the per-call `callCap` are mutex-guarded, and each worker commits its `ai_scores` row before the next call (S8 reconnect-safe). The provider request pacing is a self-imposed **1 request/second** (`ai.SuggestedRateLimit` → `aiRequestSpacing = time.Second`; reinstated 2026-06-06, the original `aiRateLimit`, well under Anthropic's ~50 req/min tier-1 ceiling) — it spaces only request STARTS (the limiter releases its lock before sleeping), so the pool's latencies still overlap. The same pool (`rateStage2`) auto-rates the fresh briefing at the end of a scrape.

## Korean matching semantics (do not casually change)

`internal/scoring/match.go` reproduces FTS5's `unicode61` tokenization in Go — NFC-normalize, split on non-letter/digit runs, lowercase. Phrase match is token-exact and order-preserving. The user-facing consequence (documented in README and the profile form helper text):

- "개발" does **NOT** match "개발자" — different tokens by design. This is what stops "병특" matching "병특혜택없음".
- The matcher cannot distinguish "야근 없음" from "야근". Users enter short root-form keywords.
- Korean particle attachment ("야근이"/"야근을") will miss "야근" — the Step 0 addendum flags this as something to measure with the 20-posting QA fixture (`internal/scoring/testdata/qa_postings.json`) before deciding on a mitigation. Success Criterion #5 requires zero false negatives on dealbreakers.

Stack matching uses a separate code path (`scoreStacks`) — case-insensitive exact match against `Posting.StackTags`, **not** FTS — because punctuated stack names like `.NET`, `Next.js`, `Spring Boot` would tokenize badly.

## Scoring math

Categories cap at maxes (Stack 50, Career = `Profile.CareerWeight` default 25, Location = `Profile.Location.Weight` clamped to 15, Salary = `Profile.SalaryWeight` default 10). Sum to ≤ 100 in the default configuration. A dealbreaker hit short-circuits to `Total: -1` regardless of other deltas. Education is a *soft filter* — when `MaxEducation` is set and a posting demands more, that's a `DealbreakerHit{Kind: "education"}`, not a scored line item. See `internal/scoring/rules.go` for the per-category logic.

`CareerWeight` and `SalaryWeight` were added 2026-05-28 as part of the per-category weights effort. The old fixed constants are gone — they live now as `DefaultCareerWeight` and `DefaultSalaryWeight` in `internal/profile/profile.go`, applied by `Profile.EffectiveCareerWeight` / `EffectiveSalaryWeight` when the user hasn't set them (which preserves scores byte-identical for any saved profile predating the change). Near-miss/ambiguous awards scale proportionally: career near-miss = round(weight × 2/5), salary ambiguous = round(weight ÷ 2), keeping the historical 25→10 and 10→5 ratios exact when the user accepts defaults.

`Profile.MinScore` (also added 2026-05-28) hides low-scoring rows from the main "Today" briefing — postings with `Total < EffectiveMinScore()` land in the collapsible `관심 밖으로 분류된 공고` section instead of the main list. Default `DefaultMinScore = 40`; `MinScore = 0` shows everything (no hiding). The field is a pointer (`*int`) so `nil` (profile predates the field) differs from explicit 0 (user opted in to "show everything"). The 0005_min_score migration backfills 40 into the profile JSON for old profiles — Go's `EffectiveMinScore` would default to 40 anyway, but the migration keeps the persisted state self-describing. Low-score postings stay in the DB (still visible on `/archive`) — MinScore only collapses the briefing surface.

When the profile changes, scores become stale (their `profile_hash` no longer matches the current one). `handleProfileSave` re-scores every posting in the same request; on dashboard render, mismatched-hash scores are recomputed. There is no score history — old values are overwritten by design.

**v2.0 AI scoring (cache-read; Stage 1 + Stage 2 both live).** `Score` takes `*ai.Extraction` and `*ai.Delta`. When an extraction is cached (`ai_extractions`), `scoreCareer`/`educationDealbreaker` prefer the AI career/education facts and **skip the regex `ParseExperienceYears` override** (decision D2 — the extraction lives only in the cache, never written into the `postings` columns); with no extraction the regex path is byte-identical to v1.5. The Stage-2 delta merges as one `AI 분석` `LineItem` (`scoreAll` looks it up: fresh by `ai_input_hash` + `ai_version`; else the latest cached delta marked `Stale` — first under the current `ai_version` (a goal edit), then across **any** `ai_version` (`LatestAIScoresAnyVersionByPostingID`) so a provider/model switch — which rotates `ai_version` — shows the prior reading faded (`이전 설정 기준`) rather than vanishing; **merge-only — `scoreAll` never calls the provider**). Provider calls happen at scrape time (Stage-1 extraction per new posting **and** a Stage-2 auto-rate of the fresh today briefing — `runScrape` → `rateStage2`, added 2026-06-06 so new postings show their AI chip without a manual press) and on a 재평가 (Stage-2 over a surface's visible rows). Both Stage-2 paths share the same worker pool and are bounded by `aiPerCallCap` + the token budget. An AI line is appended only when ≥1 signal survived the citation gate (no empty chips — §c). The total clamps to **`[0,100]`** (a negative AI delta floors at 0, not the `-1` dealbreaker sentinel), and `Explain` renders **signed** deltas. The dealbreaker short-circuit returns first, so a `Total:-1` posting never carries an AI line. AI is **off by default** (`Server.ai == nil`); when off, scoring is byte-identical to v1.5. **`ReconfigureAI` (server.go) is the single wiring point** — `main.go` calls it at startup and `handleProfileSave` on every save, building the provider from `ai_keys.json` + the profile's `AIProvider`/`AIModel` (non-secret AI settings live on `Profile`, `omitempty`, and are excluded from `BuildStage2ProfileText` so they never churn the goal-keyed cache).

**Source-vs-AI 신입 disagreement (fixed 2026-06-03, R3):** when the source marks a posting new-grad-eligible (점핏 `career=0`, 데모데이 `entry`/`any`, 그리팅 `NEW_COMER`/`NOT_MATTER`, Greenhouse heuristic) but a cached AI extraction says `newcomer=false` / `min_career>0`, `scoreCareer` keeps the inclusive source reading (`internal/scoring/rules.go:173-191`). A bad extraction can no longer silently strip an eligible posting's 신입 award — wrongly *excluding* a 신입 costs more than keeping a borderline senior role.

## SSE conventions

`internal/server/sse.go` sets `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`, `X-Accel-Buffering: no` and flushes after each event. **Do not wrap `/api/scrape` in any compression middleware** — gzip will buffer and the client sees nothing. Newlines in the `data:` payload are collapsed to spaces because each event sends one-line status text; if you ever need to send a multi-line HTML fragment, switch to the `data: `-per-line encoding (the design doc spells out the gotcha).

The event names the client listens for: `status`, `count`, `progress`, `done`, `failed`. The HTMX `sse-connect` attribute must be removed by the `done` payload to prevent auto-reconnect (htmx will otherwise start a second scrape).

`/api/rerate` (the per-surface 재평가) is the second SSE endpoint and shares this contract. Two extra rules it follows (S8): it emits its terminal event (`done`|`failed`) on **every** exit path via a `defer` (so a mid-stream error never leaves the client hanging or auto-reconnecting into a double-spend), and it **commits each posting's `ai_scores` row before the next provider call**, so a dropped connection resumes from cache with no re-spend. Both the scrape and re-rate clients live in static JS (`ai-rerate.js` mirrors the inline scrape `EventSource` flow), not htmx.

**Re-rate cache semantics** — why a listing can be analyzed yet show no `AI 분석` card, and what a repeat press re-spends on — are written up in `internal/server/RERATE_NOTES.md`. Short version: a successful-but-empty delta is cached (`items_json = "[]"`), counted in the `N/M` indicator but rendered as no chip, and **never** re-analyzed under the same goal text + model; only failed or never-reached listings lack a cache row and get retried on the next press.

## Scrape pipeline

`Server.runScrape` (in `internal/server/server.go`) is the orchestrator:
1. `CheckAccess` — robots.txt with 24h cache, checks **both** `jumpit.saramin.co.kr` and `jumpit-api.saramin.co.kr` (the API host's robots.txt 404s, which RFC 9309 reads as unrestricted).
2. `FetchListing` against `/api/positions?career=0&size=500&...` — `career=0` filters 신입 server-side. Pagination is a defensive fallback; the 신입 universe (~57 postings) fits in one page.
3. For already-seen postings, just bump `last_seen_at`. For new postings (up to `scrapeNewCap = 50`), fetch detail at 1 req/s and persist. **Edited-JD refresh (T7):** each scrape also re-fetches the detail of up to `detailRefreshCap = 10` already-seen postings *per source* whose `detail_fetched_at` is the oldest and at least `detailRefreshMinAge = 24h` stale (oldest-first rotation), via `RefreshPostingDetail`, then re-extracts Stage-1 — so an employer's later JD edit changes `content_hash` and the cached extraction/score refresh. Free for no-op-`FetchDetail` sources (데모데이/Greenhouse/그리팅); a bounded 1-req/s cost for 점핏/랠릿. The seen/new split and the refresh candidates both come from `SeenDetail`. (The Stage-2 AI *delta* is goal-keyed, not content-keyed, so it is NOT recomputed on a JD edit — a known limitation parked in `feature-ideas.md`.)
4. **Sweep stale postings** — `SweepStalePostings` hard-deletes anything not seen in `sweepStaleWindow=3d` OR first seen more than `sweepOldWindow=90d` ago (the latter only if `always_open=0`). Bookmarked postings are exempt from both rules.
5. Re-score everything against the current profile.

The 1 req/s pacing is `time.Sleep` in `internal/scraper/jumpit/client.go` — deliberately simpler than `x/time/rate`.

**Sweep semantics worth knowing** — staleness is measured relative to `MAX(last_seen_at)` across the whole table, not wall-clock `now()`. If the user goes on vacation and doesn't scrape for two weeks, nothing becomes stale during the gap because the baseline doesn't move. On the next scrape, the baseline jumps forward and any posting not re-found becomes stale. This avoids needing a `scrape_runs` audit table.

## Storage layout

The DB lives at `os.UserConfigDir() + "/job-scraper/jobs.db"` (overridable with `--db`). Migrations are embedded via `embed.FS`, named `NNNN_description.sql`, and tracked via `PRAGMA user_version`. `raw_json` is kept on every posting for forward compatibility with v1.6+ parsers. (`0006` is intentionally skipped — the runner keys on the 4-digit filename prefix, not contiguity.) Latest is `0010_detail_fetched_at.sql` (adds `postings.detail_fetched_at` for the T7 edited-JD refresh, backfilled from `first_seen_at`); `0009_purge_orphan_scores.sql` is a one-time orphan cleanup.

Migration `0008_byok_ai.sql` (v2.0) adds the full AI schema in one file: `ai_extractions` (Stage-1 career/education cache, keyed `posting_id + content_hash + ai_version`), `ai_scores` (Stage-2 delta cache, keyed `posting_id + ai_input_hash + ai_version` + `computed_at`), and `ai_usage` (rolling daily token ledger, one row per UTC day). Both AI caches `REFERENCES postings(id) ON DELETE CASCADE`, so the staleness sweep cleans them with the posting. All three tables are written: `ai_extractions` by Stage-1 extraction at scrape time; `ai_scores` by Stage-2 — at scrape time (the auto-rate of the fresh briefing, since 2026-06-06) and on a 재평가; `ai_usage` (the token ledger) debited by both stages. The token budget has two ceilings — a per-run in-memory cap and a rolling daily cap enforced against `ai_usage` (read once at run start, written through on each debit, so it holds across restarts). The BYOK key file (`ai_keys.json`, 0600) lives in the same config dir but is **not** in the DB.

**modernc.org/sqlite DATETIME quirk** — when you bind a `time.Time` parameter, the driver serializes via Go's default `time.Time.String()` format (`"2006-01-02 15:04:05.999999999 -0700 MST"`), and that is what lands in the column on disk. For a named DATETIME column, `SELECT col` round-trips back to `time.Time` cleanly. But aggregates like `MAX(col)` lose the DATETIME column tag — `sql.NullTime.Scan` will fail. Read the aggregate into a `sql.NullString` and parse with `timeStoreFormat` in `internal/storage/postings.go` (the constant exists for exactly this).

## Things the design doc explicitly rules out for v1

Don't add (parked in `feature-ideas.md`):
- Notifications, background scheduling, résumé parsing. (Two former non-goals have since shipped: **multi-portal scraping** — 8 default sources across 점핏·랠릿·데모데이·그리팅 + the Greenhouse company boards, with the browser-gated portals 원티드/카카오/쿠팡/그룹바이 deliberately *not* built, see `docs/plans/browser-driven-scrapers.md`; and **LLM/AI** — the v2.0 BYOK-AI line, fully built (Stage 1 2026-06-02, Stage 2 + hardening 2026-06-03, scrape-time auto-rate 2026-06-06). The live `-tags integration` provider tests still need a real key to run; open polish items are in `task-list.md`.)
- An architecture diagram in README (the code is small enough to be self-evident at v1 size).
- Score history, absence-based closed-posting tracking (deadline-based via `closed_at` is in).
- Code signing/notarization, Homebrew/Docker/auto-update.
- A `storage.Storage` interface for testability.

If a change request implies any of these, surface that it's a later-version item — v2.0 for the AI/LLM line, otherwise a future v1.x minor (v1.6+) — before implementing.
