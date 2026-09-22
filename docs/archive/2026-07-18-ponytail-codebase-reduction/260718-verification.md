# Ponytail Codebase Reduction Verification

Date: 2026-07-18

Status: DONE

## Outcome

The campaign preserved Jobcron behavior while removing 437 production Go lines. All ten
accepted batches were independently reviewed, integrated locally, and verified again as one
final codebase. No dependency or user-visible behavior changed.

Baseline source: `047102ab3540daff17633a5501c14a9f70fda46a`

Final semantic source: `9fc13164de9f1736a0fb84a50d0f0019846dd17d`

Final pre-archive documentation checkpoint:
`2e93d937423275cd9e1478086c496619bcb82807`

## Measured Change

- Production Go: 15,924 to 15,487 lines; 437 lines removed.
- Production Go files: 91 to 94; three narrow behavior-owner files added.
- Go tests: 22,982 to 23,305 lines; 323 lines added.
- Go test files: 99 to 104; five behavior-lock files added.
- Top-level Go tests: 620 to 631; 11 tests added.
- Direct dependencies: 7 to 7; unchanged.
- Web source: 3,526 lines; unchanged.
- Shell source: 444 lines; unchanged.
- Statement coverage: 61.2 to 61.6 percent; up 0.4 percentage points.
- Reproducible `jobcron` binary: 28,214,786 to 28,197,266 bytes; 17,520 bytes
  removed.
- Reproducible importer and user binaries: byte-unchanged.

The baseline and final binaries were built sequentially at one checkout path with Go 1.26.3,
`darwin/arm64`, `CGO_ENABLED=0`, trimpath, VCS metadata disabled, and an empty build ID. A
second final build matched every final binary byte-for-byte.

## Final Behavior Gates

- Repository formatting, `go vet ./...`, and `go build ./...`: pass.
- Full unit and race suites: pass.
- Full coverage suite: pass at 61.6 percent.
- PostgreSQL 18 storage, importer, and user suites: pass normally and with race detection.
- PostgreSQL JSON audit: zero skips, 182 test-pass events, and three package passes in each
  run.
- Live Jumpit, Rallit, Demoday, Greeting, and Greenhouse contracts: pass.
- All three commands cross-build with `CGO_ENABLED=0` for `darwin/arm64`, `linux/amd64`,
  `linux/arm64`, and `windows/amd64`.
- Full-history Gitleaks and documentation publication review: pass.

## Browser User Path

The production-mode binary ran directly against a disposable PostgreSQL 18 database with no
paid AI provider configured. gstack's persistent headless browser verified:

1. desktop owner login;
2. dashboard and archive rendering;
3. profile load and save without a replacement AI key;
4. an authenticated manual scrape through its terminal browser SSE event;
5. bookmark and hidden-state controls plus their dedicated pages;
6. logout and mobile relogin persistence;
7. idle rerate status and the intended no-key 503 guard;
8. every local page on 1440 by 900 and 375 by 812 viewports; and
9. the exact Sendbird posting destination, matching title, and public role identifier.

The manual scrape completed with 162 new postings. Every local page reported zero console
errors after expected endpoint-control and external-site entries were cleared. All eight
screenshots were visually inspected; no clipping, overlap, horizontal overflow, or unreadable
controls were found. Browser, app, credential material, and disposable database cleanup
completed.

## Finding Disposition

- Accepted findings: 8.
- Rejected findings: 5.
- Separate-decision findings: 0.
- Reversible implementation batches: 10.

The campaign intentionally retained duplication where consolidation would mix policy or add
speculative flexibility:

- bookmark and hidden HTTP handlers;
- bookmark and hidden storage-table statements;
- legacy importer row-copy paths;
- complete scraper HTTP, cache, and robots-policy helpers; and
- cloned tests whose scenarios should remain explicit.

## Local Semantic Commits

- `PT4-001` `03c8fd4`: remove the unused default AI-key path wrapper.
- `PT4-002` `26083aa`: remove the unused registered-source method.
- `PT4-003` `99f395a`: reuse `ModelInput` instead of `buildModelText`.
- `PT4-004` `6f2f19c`: reuse one storage posting-row collector.
- `PT4-005` `d944eb2`: share narrow exact-token matching.
- `PT4-006` `c8905de`: add the shared robots parser and four consumers.
- `PT4-007` `8541429`: convert the final robots-parser consumer.
- `PT4-008` `6619d50`: add the request pacer and four consumers.
- `PT4-009` `ffbe1ca`: convert the final request-pacer consumers.
- `PT4-010` `9fc1316`: remove the unused scheduler handle.

Documentation, evidence corrections, and review checkpoints remain in the intervening local
history. Nothing was pushed.

## Global Ponytail Default

`~/.config/ponytail/config.json` now sets `defaultMode` to `full`. An isolated run of the
official activation and mode-tracker hooks verified that a new session starts in `full`,
`@ponytail off` disables it for the session, and `@ponytail full` restores it. Other
configuration fields were preserved.

## Independent Decisions Worth Review

- Chosen: treat the user's direction to proceed with all remaining items as final campaign
  acceptance and complete the documented archive/default steps. Alternative: stop for a
  second acceptance prompt. Rollback: move the records back and set the default to `off`.
- Chosen: keep paid AI disabled in browser verification and exercise the explicit unavailable
  contract. Alternative: use a real provider credential. Rollback cost: none.
- Chosen: retain all five rejected duplication clusters. Alternative: broaden the campaign
  into policy-bearing abstractions. Rollback cost: none because no rejected change landed.
