# No Browser-Driven Scraping For v1.x

## Decision

Do not add Playwright, chromedp, or browser-fingerprint bypass infrastructure to
Jobcron in v1.x. Continue prioritizing public, no-auth ATS APIs that fit the
pure-Go single-binary product constraint.

## Why

- Wanted's browser challenge is paired with terms that prohibit automated
  scraping and circumvention. Its legitimate OpenAPI is partner-key-gated.
- Kakao's continuous board is experienced-only; newcomer hiring is seasonal,
  fragmented, and already syndicated elsewhere.
- Coupang's actual listings are available through its public Greenhouse board,
  which Jobcron already understands; the remaining gap is relevance, not access.
- GroupBy has a possible pure-Go uTLS path, but its fingerprint wall is an
  operator no-bots signal and its experienced-skewed inventory overlaps current
  sources.
- A browser runtime would materially increase packaging, maintenance, and
  operational complexity for little incremental newcomer-developer coverage.

## Reconsider When

Revisit only if a source offers explicit automation permission or a stable
official API and measured newcomer-developer yield justifies the added
integration. The detailed source evidence and contingency architecture remain
in [the research record](../../research/2026-06-06-browser-driven-scrapers.md).
