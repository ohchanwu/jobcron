# Alpha Pre-Launch Fixes

**Status:** Ready for implementation<br>
**Verified:** 2026-07-13 against `main` at `189a736`

## Context

Two visible details should be corrected before the alpha launch:

1. The root README files show the current 전체 공고 page in 날짜순 mode, even
   though 점수순 better communicates Jobcron's main value: ranking jobs by fit.
2. Removing a bookmark while viewing `/bookmarks` updates the stored state but
   leaves the card on screen until the user reloads. The same action should feel
   complete immediately, like the existing smooth card removal after pressing
   the 관심 없음 eye on the briefing or 전체 공고 page.

These changes affect prospective users evaluating the project from GitHub and
signed-in or demo users managing saved jobs. They are launch polish, but the
bookmark issue also creates a short-lived mismatch between the page and the
stored data.

## Goals

- Make both README screenshots show the 전체 공고 page with 점수순 active.
- Remove a successfully unbookmarked card from `/bookmarks` without a page
  reload, using the existing posting-card exit motion.
- Keep the bookmark count, empty state, source filter, and search results in sync
  after a card leaves the page.
- Preserve rollback behavior when the bookmark request fails.

## Current State

| Surface | Verified behavior | Gap |
|---|---|---|
| `README.md:21-24` | Uses `dashboard.png` and `dashboard-dark.png` for the light and dark 전체 공고 screenshots. | The captured page has 날짜순 active. |
| `README.ko.md:21-24` | Uses the same two assets and the same English alternative text. | The screenshot does not present the requested 점수순 view, and the alternative text does not name the sort mode. |
| `web/archive.html:55-57` | Provides 날짜순 and 점수순 links. `/?sort=score` activates the flat, descending score view. | No application behavior change is needed. |
| `web/bookmark.js:59-100` | Optimistically flips the bookmark icon, calls `/api/bookmark/{id}`, reconciles to the JSON response, and rolls back the icon on failure. | A successful `bookmarked: false` response does not remove the card on `/bookmarks`. |
| `web/bookmark.js:35-57` | Demo mode filters rendered cards from `localStorage` and updates its count and empty state. | An unbookmarked demo card is hidden immediately, without the existing exit transition. |
| `web/not-interested.js:70-83` and `web/styles.css:765-770` | Adds `.posting.removing`, waits for the 0.22-second opacity transition, and falls back to removal after 260 ms. | The bookmark flow does not reuse this behavior. |
| `web/bookmarks.html:71-116` | Renders cards when bookmarks exist and an empty state when the initial server response has none. Only demo mode gets a hidden empty-state element beside a non-empty list. | The signed-in page cannot reveal an empty state after JavaScript removes the last card. |
| `web/source-filter.js:43-53` | Caches the initial card nodes for source and text filtering. | Removed nodes remain in the cache and can produce a false non-empty result unless disconnected nodes are ignored. |

### Root Cause

The screenshots were captured while the archive's default 날짜순 mode was active.
The bookmark client was written as a cross-page icon toggle, so its successful
response handler reconciles only the button state. It has no `/bookmarks`-specific
card lifecycle. Demo mode later added page filtering, but it uses `hidden`
immediately. The live template also assumes the empty state is decided only at
server-render time.

## Proposed Change

### P0: Make unbookmarking finish on the bookmarks page

Use the existing `.posting.removing` transition and the same 260 ms hard-removal
fallback as the 관심 없음 flow. Do not add a second animation style.

The final lifecycle is:

```text
Unbookmark click
  ├─ signed-in app: DELETE succeeds and returns {"bookmarked": false}
  └─ demo: the posting ID is absent from the saved localStorage set
                         |
                         v
              add .posting.removing
                         |
              transitionend or 260 ms
                         |
                         v
                 remove card node
                         |
                         v
       update count, empty state, and active filters

Request failure or a final bookmarked=true state
                         |
                         v
       keep card and reconcile the bookmark button
```

Implementation requirements:

1. Add a small `fadeRemove` helper to `web/bookmark.js` that matches
   `web/not-interested.js:70-83` and reuses `.posting.removing`.
2. On the signed-in app, start card removal only after a successful response whose
   final `bookmarked` value is `false`, and only when `location.pathname` is
   `/bookmarks`. A failed request must keep the card and restore the pre-click
   button state.
3. In demo mode, preserve the initial `localStorage` filtering behavior, but do
   not set the clicked visible card to `hidden` before its exit transition. The
   card must visibly receive `.removing` first.
4. After the card is removed, update the `저장된 공고` count from the remaining
   saved cards. When the last saved card leaves, show the correct empty-state copy
   for signed-in or demo mode and hide the now-empty list.
5. Dispatch a generic posting-list change event after removal. Update
   `web/source-filter.js` to reapply the active source and text filters on that
   event and to exclude disconnected card nodes from `anyVisible` calculations.
   If the removed card was the last match for an active filter while other
   bookmarks remain, show the existing filter-specific empty message instead of
   the page-level no-bookmarks state.
6. Render a hidden page-level empty-state element in `web/bookmarks.html` even
   when the initial signed-in response contains postings. Keep the current live
   and demo copy distinct.
7. Do not reload or navigate the page to obtain the updated state.

### P1: Replace the README screenshots with 점수순 captures

Capture the existing 전체 공고 page at `/?sort=score` in both themes and replace
the assets in place:

- `docs/assets/screenshots/dashboard.png`
- `docs/assets/screenshots/dashboard-dark.png`

Capture contract:

- Use a 1440 x 900 viewport and keep both files as 1440 x 900 PNG images.
- Show the same 전체 공고 surface and general composition as the current assets.
- Make `점수순` visibly active and ensure the visible kept postings descend by
  score.
- Use the 전체 source pill, clear the text search, and keep the low-score section
  in its normal collapsed state.
- Capture light mode into `dashboard.png` and dark mode into
  `dashboard-dark.png`.
- Use public-safe job data. Do not expose an API key, owner email, profile data,
  session value, database address, or other private configuration.
- Keep the existing `<picture>` light/dark behavior in both README files. Update
  each image's alternative text so it explicitly says the page is score-sorted;
  use English in `README.md` and Korean in `README.ko.md`.

## Dependency Graph

```text
P0 bookmark lifecycle ──> P0 automated tests ──> browser QA

P1 prepare score-sorted page ──> light capture ─┬─> README text check
                              └─> dark capture ─┘

All verification ──> final staged-diff and publication-safety review
```

The two workstreams can be implemented independently. Finish browser behavior
and automated tests before the final screenshot capture so the committed
documentation represents the release candidate being verified.

## Acceptance Criteria

1. `dashboard.png` and `dashboard-dark.png` are both 1440 x 900 PNG files showing
   the root 전체 공고 page with `점수순` visibly active.
2. The kept postings visible in each screenshot are ordered from highest score to
   lowest score.
3. `README.md` and `README.ko.md` still use a theme-aware `<picture>` element and
   both have alternative text that identifies the screenshot as score-sorted.
4. No screenshot contains credentials, private account data, or production
   infrastructure details.
5. On `/bookmarks`, a successful signed-in DELETE response with
   `{"bookmarked": false}` adds `.removing` to the clicked card and removes it
   after `transitionend` or the 260 ms fallback, without a reload.
6. A failed signed-in bookmark request leaves the card in the DOM, restores the
   original icon state, and leaves the count and empty state unchanged.
7. A successful response whose final state is `bookmarked: true` leaves the card
   in the DOM.
8. Unbookmarking on `/`, `/briefing`, `/hidden`, or `/profile` does not remove a
   card through the bookmark script.
9. Demo-mode unbookmarking on `/bookmarks` updates `jobcronDemoBookmarks`, shows
   the same exit transition, and removes the card without a reload.
10. Removing one of two bookmarks changes the header count from 2 to 1 after the
    card leaves the DOM.
11. Removing the last bookmark changes the count to 0, hides the empty list, and
    reveals the correct signed-in or demo empty-state message.
12. With a source pill or text search active, removing the last matching card
    updates the existing filter empty message; disconnected nodes cannot keep the
    filter in a false non-empty state.
13. The existing 관심 없음 removal behavior and its 0.22-second transition remain
    unchanged.
14. The existing bookmark API, per-user storage behavior, CSRF handling, and demo
    read-only server contract remain unchanged.
15. All project tests, static checks, browser flows, and publication-safety checks
    listed below pass.

## Testing Plan

| Layer | What | Minimum cases |
|---|---|---:|
| JavaScript lifecycle | Add `web/testdata/bookmark-lifecycle.test.js` using the existing zero-package Node harness pattern. Cover signed-in success, HTTP failure rollback, contradictory `bookmarked: true`, non-bookmarks routes, demo removal, last-card empty state, count update, active-filter recomputation, and the timeout fallback. | 9 |
| Go wrapper | Add `web/bookmark_test.go` to run the JavaScript lifecycle harness through `go test ./...`, following `web/ai_rerate_test.go`. | 1 |
| Server/template | Extend `internal/server/bookmarks_test.go` to prove a non-empty signed-in page includes a hidden empty-state target with live copy and preserves the existing card markup. Keep existing API and empty-page tests. | 2 |
| Existing regression | Run `gofmt -l .`, `go vet ./...`, `go test ./...`, `go build ./cmd/jobcron ./cmd/jobcron-user ./cmd/jobcron-import`, and the direct Node lifecycle tests. | All |
| Browser user path | With `/browse`, unbookmark the first and last cards on `/bookmarks`, verify count and empty states without reload, refresh to confirm persistence, repeat the demo path, and exercise an active source filter and text search. | 6 flows |
| Visual QA | Walk every app page on desktop and mobile in light and dark themes, confirm no console errors, then capture and inspect the two 1440 x 900 README images. | 2 viewports x 2 themes |
| Publication safety | Inspect the complete staged diff, run Gitleaks, and manually inspect both PNGs for private data before commit. | 3 checks |

Browser verification must follow the real click path. HTTP-only checks do not
prove the card transition, count update, empty state, or final screenshot.

## Rollback Plan

- If the bookmark behavior regresses, revert the JavaScript, template, and test
  changes together. The server API and stored bookmarks are unchanged, so no data
  rollback is required.
- If only a screenshot is wrong, restore the previous two PNG assets and their
  previous alternative text. No runtime rollback is required.
- Do not remove or rewrite bookmark rows as part of rollback. A browser refresh
  remains the source-of-truth recovery path if a client-side transition fails.

## Effort Estimate

| Work | Human estimate | Codex + gstack estimate |
|---|---:|---:|
| Bookmark lifecycle, empty state, and filter synchronization | 2-3 hours | 30-45 minutes |
| Automated lifecycle and template tests | 1-1.5 hours | 20-30 minutes |
| Browser QA and screenshot capture | 1-1.5 hours | 20-30 minutes |
| Final documentation and publication review | 30 minutes | 10-15 minutes |
| **Total** | **4.5-6.5 hours** | **80-120 minutes** |

## Files Reference

| File | Change |
|---|---|
| `web/bookmark.js:35-107` | Add context-aware smooth removal, shared page-state updates, and list-change notification for signed-in and demo bookmark flows. |
| `web/bookmarks.html:51-116` | Keep a revealable empty-state target when the initial page contains bookmarks. |
| `web/source-filter.js:43-53,89-121` | Reapply filters after list changes and ignore disconnected cards. |
| `web/styles.css:765-770` | Reuse the existing `.posting.removing` transition; change only if a test exposes a missing reduced-motion fallback. |
| `web/not-interested.js:70-83` | Reference behavior only. Do not change the 관심 없음 lifecycle. |
| `web/testdata/bookmark-lifecycle.test.js` | Add zero-package JavaScript lifecycle coverage. |
| `web/bookmark_test.go` | Run the JavaScript harness from the Go test suite. |
| `internal/server/bookmarks_test.go` | Verify the signed-in empty-state target and retain existing bookmark API/page coverage. |
| `docs/assets/screenshots/dashboard.png` | Replace with the light, score-sorted 1440 x 900 capture. |
| `docs/assets/screenshots/dashboard-dark.png` | Replace with the dark, score-sorted 1440 x 900 capture. |
| `README.md:21-24` | Keep the picture sources and update English alternative text. |
| `README.ko.md:21-24` | Keep the picture sources and update Korean alternative text. |

## What's Working Well: Do Not Touch

- The bookmark API already returns the final server state and persists per-user
  data correctly.
- The optimistic icon update, disabled-button guard, CSRF header, and failure
  rollback in `web/bookmark.js` are correct.
- The 관심 없음 flow already has the requested visual treatment and serves as the
  behavior reference.
- The 전체 공고 sort implementation, remembered sort cookie, and score ordering
  tests are already correct.
- The theme-aware README `<picture>` structure and existing screenshot filenames
  are stable references used by both language versions.

## Out of Scope

- Changing the default 전체 공고 sort from 날짜순 to 점수순.
- Redesigning the README, changing the screenshot dimensions, or adding more
  screenshots.
- Changing bookmark storage, the `/api/bookmark/{id}` contract, authentication,
  or database migrations.
- Removing muted cards from `/bookmarks`; a posting that is both muted and
  bookmarked must continue to remain on the bookmarks page.
- Refactoring all card-removal behavior into a new animation subsystem.
- Deploying `jobcron.app` or changing the demo deployment.

## Definition of Done

The work is complete when every acceptance criterion passes, both new screenshots
have been visually inspected in the rendered English and Korean READMEs, the
signed-in and demo bookmark flows have been exercised in a real browser, and the
final staged diff passes the repository's documentation publication gate.
