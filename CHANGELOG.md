# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

This changelog begins at the **v1.0 epoch** — the baseline that already shipped
(multi-source scraping, cross-portal dedup, scoring engine, daily-briefing UI)
before tagging began. Earlier development is captured in git history. The first
tagged release is **v1.5.0**, which gathers the refinements layered on the 1.0
baseline. The BYOK AI integration is the **v2.0** line — a separate major, not
part of the 1.x series.

## How to cut a release

1. Move entries out of `[Unreleased]` into a new section dated today,
   e.g. `## [1.0.0] - 2026-06-15`.
2. Open a new empty `[Unreleased]` block at the top.
3. `git tag -a vX.Y.Z -m "Release X.Y.Z"` and `git push origin vX.Y.Z`.
   GoReleaser (`.goreleaser.yml`) reads the tag and bakes it into the binary
   via `-ldflags "-X main.version={{ .Version }}"`; the `--version` CLI flag
   then surfaces it.

## [Unreleased]

## [2.0.0-alpha3] - 2026-06-06

The BYOK-AI line matures and the source set widens. The headline is a reversal:
**OpenAI is removed as a provider** — live testing made it concrete that it can't
serve this app's audience (see Removed). Anthropic is now the single supported
provider, at a calm self-imposed 1 request/second. Alongside that: two new
sources, an automatic AI rating pass at scrape time, and a batch of
provider-switch and scoring fixes.

### Added
- **More sources (eight defaults now).** 그리팅 (Greeting) Korean-ATS aggregator and
  a multi-tenant Greenhouse source (당근·크래프톤·몰로코·센드버드) join
  점핏·랠릿·데모데이. The browser-gated portals (원티드/카카오/쿠팡/그룹바이) were evaluated
  and deliberately not built.
- **AI rates new postings automatically.** A scrape now runs the Stage-2 re-rate
  over the fresh briefing at the end, so new postings show their AI 분석 chip
  without a manual 재평가 — bounded by the same per-call and token caps.
- **관심 공고 sort + remembered filters.** A 점수순 sort toggle on the 관심 공고 page,
  and the source-filter pills + sort choice now persist across visits.
- **Legible re-rate.** Clearer 재평가 progress, and a user-settable "how many
  postings to analyze per press" cap on the profile form.

### Changed
- **AI request pacing is back to a self-imposed 1 request/second** — the original
  limit, restored after a brief provider-aware experiment. It spaces only request
  starts, so the concurrent re-rate worker pool (~4× faster than the old
  one-at-a-time loop) still overlaps call latency.

### Removed
- **OpenAI is no longer a selectable provider; Anthropic is the only one.** It was
  removed after live testing made the problem concrete: a bring-your-own-key user
  on OpenAI's free / entry tier cannot sustain the re-rate workload. Each listing
  costs two model calls (a Stage-1 extraction and a Stage-2 score), and a re-rate
  fans several out at once — past an entry-tier ceiling that can be as low as a few
  requests per minute. In practice OpenAI returned a rate-limit error on nearly
  every call: only about one listing in five got through per press, the rest
  failing. Pacing the calls much more gently still got zero of five through — the
  account tier, not our pacing, is the wall. We will not ask users to pay for a
  higher OpenAI tier just to use this app, so OpenAI is out; Anthropic's entry tier
  handles the 1 req/s pace comfortably. The provider abstraction stays, so a viable
  provider could be added back later, and the OpenAI implementation lives on in git
  history.

### Fixed
- **AI 분석 chips no longer vanish when you change the AI model** (or, before its
  removal, the provider). The change rotated an internal cache version and orphaned
  every prior rating, so the chips disappeared; they now persist, faded and
  labelled "(이전 설정 기준)", until a 재평가 refreshes them.
- **A failing re-rate now says why.** A bad key, a wrong model, or a provider rate
  limit used to be swallowed — the press looked like it did nothing. The re-rate
  now ends in a calm, specific message (check your key / check the model /
  rate-limited, try again / check billing) instead of a hollow "0 analyzed".
- **인턴 (intern) roles keep their 신입 chip when AI is on** — resolving the alpha2
  known issue. The source's correct new-grad flag is no longer overridden by a
  model that misreads an entry-level intern role as requiring experience.
- **랠릿 listings are filtered to the developer 직군**, so non-dev roles no longer
  enter the briefing from that source going forward.
- Scraped postings are never left unscored / blank-carded; the 관심 공고 sort no
  longer flashes 점수순 on load; orphaned AI cache rows are purged with their
  posting (migration 0009).

### Known issues
- The live provider integration tests remain opt-in; this release was verified by
  the manual Anthropic/OpenAI checks run during the OpenAI investigation, not the
  full live suite.

## [2.0.0-alpha2] - 2026-06-03

Stage 2 + hardening of the **v2.0 BYOK-AI** line. AI is now a real, end-to-end
feature: enable it on the profile form, and the briefing gains evidence-cited
score adjustments you can re-run on demand. Still fully optional — with no key
configured the app behaves exactly like v1.5.

### Added
- **AI is now user-facing.** A new "AI 분석" section on the profile form picks a
  provider (Anthropic / OpenAI), takes your API key (stored only in a local 0600
  file, never the database, shown as "•••• 저장됨" once saved), and sets a daily
  token budget with a live "오늘 사용 / 남은 예산" readout. The app wires the
  provider at startup and the moment you save a key — no restart.
- **Evidence-cited AI score adjustment.** When AI is on, each posting can gain an
  `AI 분석` chip — gold `+N` or muted `−N` — that you click to see the exact quote
  from the posting backing each adjustment (or a code-verified "관련 언급 없음" for
  something a goal needed but the posting lacks). Every shown adjustment is
  backed by a real citation; nothing unjustified appears.
- **재평가 (re-rate).** A per-page button (briefing / 관심 공고 / 북마크) re-scores
  the rows you can see against your goals, streaming progress. It only spends
  tokens on rows not already analyzed under your current goals, so pressing it
  again continues where it left off instead of redoing work. Hidden entirely when
  no key is set; shows a count when some rows were scored against an older profile.
- **Rolling daily token ledger + caps.** A per-run and a per-day token ceiling
  (default ~1M/day) cap spend; when the budget runs out the briefing finishes on
  the regular score with a calm note, never an error.

### Changed
- A posting's "(이전 프로필 기준)" stale AI chip stays counted in the total and is
  shown faded, so editing your goals never silently changes a score without
  telling you it's now out of date.

### Security
- The AI is treated as untrusted: a posting that tries to hijack the model (hidden
  "ignore instructions / dump your key" text) gets its output rejected by the JSON
  and citation gates, applies no score, and cannot exfiltrate the key (which never
  enters the prompt; egress is pinned to the one provider host).

### Known issues
- An `인턴` (internship) posting can lose its `신입` chip when AI is on: the model
  sometimes reads an entry-level intern role as requiring experience, and the
  score trusts that over the source's correct new-grad flag. Fix pending.
- The live provider integration tests ship but are opt-in and were not run this
  release (no API key available).

## [2.0.0-alpha] - 2026-06-02

The **v2.0 BYOK-AI** line begins here (a separate major, not part of 1.x). Stage 1
below is the foundation — committed but **dormant**: there is no key-entry UI yet
(a later stage), so the running app behaves exactly like v1.5 until a provider is
configured.

### Added
- **Bring-your-own-key AI provider layer (`internal/ai`).** A provider interface
  with hand-rolled Anthropic and OpenAI HTTP clients (no SDK, to keep the pure-Go
  single-binary build), a one-host egress pin, a 0600 `ai_keys.json` key store,
  and a stub provider for the offline test suite.
- **Stage-1 extraction + cache.** With AI enabled, each new posting's
  career/education requirements are read by the model and cached in a new
  `ai_extractions` table (migration `0008`). Scoring prefers these cached AI facts
  over the regex heuristic — so a posting whose body says "경력 5년 이상" but is
  actually 신입-friendly scores the full new-grad award instead of 0. Any AI
  failure falls back silently to the existing regex scoring.
- **Profile AI goal fields.** Four optional free-text fields (좋아하는 업무,
  피하고 싶은 업무, 단기/장기 목표) under a new "AI 분석 (선택)" section, for the
  upcoming AI analysis.

### Removed
- **필수 키워드 (must-have keywords).** The must-have list is gone. Use dealbreaker
  keywords (any match excludes a posting) instead; re-enter past must-haves as
  goal fields or dealbreakers.

### Changed
- **Scores clamp to `[0, 100]`** (a floor was added) and the score breakdown
  renders signed deltas. No effect on existing scores while AI is off.

## [1.5.0] - 2026-05-29

The accumulated refinements on the 1.0 baseline, cut as the first tagged
release. Everything here is keyword-matched scoring and UI — no LLM; AI is v2.0.

### Added
- **관심 공고 view.** The former "전체 공고" page is now a curated list. Postings
  below your minimum score collapse into a "관심 밖으로 분류된 공고" section
  instead of crowding the day list, mirroring the daily briefing. (URL stays
  `/archive`.)
- **관심 없음 manual mute.** Hide a posting from the briefing and 관심 공고 with one
  click. A bookmarked-but-muted posting stays on the bookmarks page, and an
  unmute list in profile settings brings any of them back.
- **Per-category scoring weights.** Set how much career fit and salary fit are
  worth; defaults preserve the original 25/10 scoring. The profile form previews
  the derived near-miss and ambiguous-salary awards as you change a weight.
- **Minimum-score threshold.** Hide low-scoring postings from the daily briefing
  (default 40; set to 0 to show everything).
- **Theme switcher.** Light / dark / auto, with monitor·sun·moon icons.
- **Favicon.** A gold sunrise mark — SVG with a multi-resolution ICO fallback.

### Changed
- Location matching treats 강남 as 강남구 (and similar district forms), so more
  서울 postings earn their location bonus.

### Fixed
- demoday: the IT/SWE filter applies to every position bucket, and
  프로그래머-titled roles are kept rather than dropped.
- Stale-posting sweep: the 3-day window is now pinned by a boundary test
  (just-under survives, just-over is removed on the next scrape).
