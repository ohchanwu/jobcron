# Interactive Preview And Navigation Verification

Verified 2026-07-11 against Mayor-controlled local `main` commit `8967a5e`.
All work and verification commits remained local; nothing was pushed.

## Automated gates

- `gofmt -l .`: clean.
- `go build ./...`: passed.
- `go vet ./...`: passed.
- `go test ./... -count=1`: passed.
- PostgreSQL integration: passed the full uncached suite with
  `JOBCRON_TEST_POSTGRES_URL` against PostgreSQL 18.4 in a uniquely named
  throwaway database. Teardown dropped the database and a catalog query
  confirmed that zero matching databases remained.
- Hosted demo mutation protection: `TestDemoModeRejectsWriteRoutes`,
  `TestDemoModeScrapeRequiresAdminToken`, and
  `TestDemoModeRerateAlwaysRefused` passed. Together they reject profile,
  bookmark, hidden-post, unauthenticated visitor scrape, and rerate writes.

## Frontend QA

The headless gstack browser exercised `/`, `/briefing`, `/bookmarks`,
`/hidden`, `/profile`, and `/login` in light and dark themes at `1440x900`,
`1024x1366`, and `390x844` (36 route/theme/viewport cases).

- 36 viewport screenshots were captured outside the repository and visually
  reviewed as six contact sheets.
- All routes rendered the expected title and page heading.
- All 36 cases had no horizontal overflow or off-viewport content.
- A clean second matrix recorded zero console errors and zero failed requests
  across 840 first-party requests.
- Header, navigation, main, and heading geometry remained stable from load to
  network idle in all 36 cases.
- The demo fixture has no current-day briefing, so the notification dot's
  visible presentation state was exercised directly. Desktop light and mobile
  dark screenshots plus bounding-box measurements confirmed that the dot does
  not overlap the navigation text and does not introduce page overflow.

Generated QA screenshots were kept under `/tmp/jobcron-task4-qa` and were not
added to Git.
