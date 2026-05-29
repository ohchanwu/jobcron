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
