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
