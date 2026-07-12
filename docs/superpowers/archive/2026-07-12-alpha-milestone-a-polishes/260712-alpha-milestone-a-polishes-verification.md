# Alpha Milestone A Polishes Verification

## Automated Gates

- `test -z "$(gofmt -l internal/server web)"`: PASS
- `go vet ./...`: PASS
- `go test ./internal/server ./web -count=1`: PASS
- `go test -race ./internal/server -run 'TestRunRerate|TestRerateTracker|TestRerateStatus|TestProfileFormDefaultsDemodayOff' -count=1`: PASS
- `go test ./... -count=1`: PASS
- `go build ./cmd/jobcron`: PASS
- `git diff --check`: PASS
- Node-backed executable JavaScript lifecycle harness: PASS

## Browser QA

- Back/forward recovery: PASS
- Completion while away, one reload, one-time notice: PASS
- Fully cached evaluation with unchanged token usage: PASS
- Electric-indigo chip/panel/focus, light and dark: PASS
- Desktop 1440x900 and mobile 390x844: PASS
- Reduced motion: PASS
- All UI routes, console, network, and overflow: PASS
- Current alpha profile has Demoday disabled and can re-enable it: PASS

## Verification Method

The live provider flow used a private runtime configuration against a controlled
QA copy of the local database. The canonical database and saved profile were not
modified. Real existing postings supplied the visible briefing rows; only the QA
copy's visibility timestamps, bookmarks, and AI cache rows were adjusted to make
the lifecycle deterministic. Credentials, posting identifiers, posting titles,
database paths, screenshots, and machine-specific recovery details were not
included in tracked documentation.

The browser check observed current progress after returning through history,
then observed that value change again. Completion while away produced one Back
navigation plus exactly one automatic reload, fresh AI results, and the exact
one-time notice. A following manual reload cleared the notice. A fully cached
press showed the exact zero-token explanation and left the displayed token usage
unchanged.

## Remaining Risk

Rerate recovery is intentionally process-local. Restarting Jobcron during an
active evaluation clears the status snapshot; cached per-row AI results remain
safe and reusable.
