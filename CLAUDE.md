# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this project is

`job-scraper` is a single-binary local web app that scrapes 점핏 (Jumpit) for Korean 신입 (new-grad) IT job postings, scores them against a user profile, and renders a calm daily briefing. The product thesis is *emotional* as much as functional: every UX decision is filtered through "does this make a stressed 신입 feel calmer?" — that constraint (P6 in the design doc) is first-class, not decorative.

Korean UI strings are inlined in Go and templates by design; do not introduce i18n machinery in v1.

## The design doc is the source of truth

`~/.gstack/projects/job-scraper/chanbla11mit-main-design-20260519-183759.md` is the round-5 design doc with full rationale: the three-tier scraping plan, scoring math and weight caps, the Step 0 FTS5 spike result, SSE gotchas, Non-goals, and Reviewer Concerns. Read it before architectural changes — it pre-resolves a lot of "should we add X?" questions.

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
- **`scraper.Scraper` IS an interface.** This is the seam for v1.1+ adding 원티드 / 프로그래머스 / company pages. New sources go under `internal/scraper/<name>/` and register themselves through `Scraper`. The `internal/scraper` package owns the shared `Posting` and `Tag` domain types.
- **One scrape at a time, per source.** `internal/server/singleflight.go` holds a mutex per source key; concurrent POSTs to `/api/scrape` return `409 Conflict`. The HTMX button uses `hx-disabled-elt` for the client side.

## Korean matching semantics (do not casually change)

`internal/scoring/match.go` reproduces FTS5's `unicode61` tokenization in Go — NFC-normalize, split on non-letter/digit runs, lowercase. Phrase match is token-exact and order-preserving. The user-facing consequence (documented in README and the profile form helper text):

- "개발" does **NOT** match "개발자" — different tokens by design. This is what stops "병특" matching "병특혜택없음".
- The matcher cannot distinguish "야근 없음" from "야근". Users enter short root-form keywords.
- Korean particle attachment ("야근이"/"야근을") will miss "야근" — the Step 0 addendum flags this as something to measure with the 20-posting QA fixture (`internal/scoring/testdata/qa_postings.json`) before deciding on a mitigation. Success Criterion #5 requires zero false negatives on dealbreakers.

Stack matching uses a separate code path (`scoreStacks`) — case-insensitive exact match against `Posting.StackTags`, **not** FTS — because punctuated stack names like `.NET`, `Next.js`, `Spring Boot` would tokenize badly.

## Scoring math

Categories cap at fixed maxes (Stack 50, Career 25, Location 15, Salary 10), sum to ≤ 100. A dealbreaker hit short-circuits to `Total: -1` regardless of other deltas. Education is a *soft filter* — when `MaxEducation` is set and a posting demands more, that's a `DealbreakerHit{Kind: "education"}`, not a scored line item. See `internal/scoring/rules.go` for the per-category logic.

When the profile changes, scores become stale (their `profile_hash` no longer matches the current one). `handleProfileSave` re-scores every posting in the same request; on dashboard render, mismatched-hash scores are recomputed. There is no score history — old values are overwritten by design.

## SSE conventions

`internal/server/sse.go` sets `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`, `X-Accel-Buffering: no` and flushes after each event. **Do not wrap `/api/scrape` in any compression middleware** — gzip will buffer and the client sees nothing. Newlines in the `data:` payload are collapsed to spaces because each event sends one-line status text; if you ever need to send a multi-line HTML fragment, switch to the `data: `-per-line encoding (the design doc spells out the gotcha).

The event names the client listens for: `status`, `count`, `progress`, `done`, `failed`. The HTMX `sse-connect` attribute must be removed by the `done` payload to prevent auto-reconnect (htmx will otherwise start a second scrape).

## Scrape pipeline

`Server.runScrape` (in `internal/server/server.go`) is the orchestrator:
1. `CheckAccess` — robots.txt with 24h cache, checks **both** `jumpit.saramin.co.kr` and `jumpit-api.saramin.co.kr` (the API host's robots.txt 404s, which RFC 9309 reads as unrestricted).
2. `FetchListing` against `/api/positions?career=0&size=500&...` — `career=0` filters 신입 server-side. Pagination is a defensive fallback; the 신입 universe (~57 postings) fits in one page.
3. For already-seen postings, just bump `last_seen_at`. For new postings (up to `scrapeNewCap = 50`), fetch detail at 1 req/s and persist.
4. Re-score everything against the current profile.

The 1 req/s pacing is `time.Sleep` in `internal/scraper/jumpit/client.go` — deliberately simpler than `x/time/rate`.

## Storage layout

The DB lives at `os.UserConfigDir() + "/job-scraper/jobs.db"` (overridable with `--db`). Migrations are embedded via `embed.FS`, named `NNNN_description.sql`, and tracked via `PRAGMA user_version`. The current schema is in `migrations/0001_initial.sql`; `raw_json` is kept on every posting for forward compatibility with v1.1+ parsers.

## Things the design doc explicitly rules out for v1

Don't add (parked in `feature-ideas.md`):
- Multi-portal scraping, notifications, background scheduling, LLM anything, résumé parsing.
- An architecture diagram in README (the code is small enough to be self-evident at v1 size).
- Score history, absence-based closed-posting tracking (deadline-based via `closed_at` is in).
- Code signing/notarization, Homebrew/Docker/auto-update.
- A `storage.Storage` interface for testability.

If a change request implies any of these, surface that it's a v1.1+ item before implementing.
