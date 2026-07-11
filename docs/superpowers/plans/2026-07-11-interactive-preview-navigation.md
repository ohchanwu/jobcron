# Interactive Preview And Navigation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide a safe interactive localhost preview, explain profile-required redirects, make all postings the front page, and show an SNS-style unread indicator for the daily briefing.

**Architecture:** Keep the hosted `--demo` mode read-only. Add a localhost-only preview launcher that runs normal writable mode in an isolated temporary home, move the daily briefing to `/briefing`, and serve all postings at `/`. Use a browser-local last-seen timestamp plus a read-only briefing-status endpoint for the notification dot; no schema migration is required.

**Tech Stack:** Go `net/http`, embedded Go templates/CSS/JavaScript, SQLite preview storage, localStorage, Server-Sent Events, existing Anthropic bring-your-own-key integration.

## Global Constraints

- Do not make `demo.jobcron.app` writable and do not accept visitor API keys in shared demo storage.
- Interactive preview binds to `127.0.0.1`, uses a temporary `HOME` and SQLite database, and never writes a key into Git.
- Preserve `/archive` as a compatibility redirect to `/`.
- Notification state is presentation-only and browser-local under `jobcronBriefingSeenAt`.
- Do not execute this plan until the human approves it.

---

### Task 1: Interactive localhost preview

**Files:**
- Create: `scripts/preview-interactive.sh`
- Create: `scripts/preview_interactive_test.go`
- Modify: `README.md`
- Modify: `README.ko.md`

**Interfaces:**
- Consumes: the existing normal `jobcron` mode, profile form, scrape endpoint, and `ai_keys.json` storage.
- Produces: `scripts/preview-interactive.sh [port]`, defaulting to port `17778`.

- [ ] In `scripts/preview_interactive_test.go`, write a syntax test that runs `sh -n scripts/preview-interactive.sh` and a process test that reserves a free loopback port, starts the launcher with `JOBCRON_PREVIEW_KEEP=1`, and confirms the printed state directory is outside the real home.
- [ ] Confirm the process test fails because the launcher does not exist.
- [ ] Implement the launcher: create a temporary directory, build `./cmd/jobcron`, set temporary `HOME` and `JOBCRON_DB`, bind `127.0.0.1`, omit `--demo`, pass `--no-open`, and remove the directory on exit unless `JOBCRON_PREVIEW_KEEP=1`.
- [ ] Document that this preview permits profile edits, real scraping, and Anthropic-key testing; the key exists only in the temporary preview directory.
- [ ] Verify in a real browser that a profile can be saved, the Anthropic key can be configured, and a scrape can be started without affecting the normal application-data directory.
- [ ] Commit as `feat: add interactive localhost preview`.

### Task 2: All postings at `/` and profile-required guidance

**Files:**
- Modify: `internal/server/handlers.go`
- Modify: `internal/server/archive.go`
- Modify: `internal/server/server_test.go`
- Modify: `web/index.html`
- Modify: `web/archive.html`
- Modify: `web/profile.html`

**Interfaces:**
- Produces: `GET /` for all postings, `GET /briefing` for the daily briefing, and `GET /archive` as a redirect to `/`.

- [ ] Add failing routing tests: `/` renders `전체 공고`; `/briefing` renders the briefing; `/archive` redirects to `/`; a missing profile redirects `/briefing` to `/profile?reason=profile-required`.
- [ ] Add a failing template test requiring the profile page to display `데일리 브리핑에서 새 공고를 스크랩하려면 먼저 프로필을 저장해 주세요.` only when `reason=profile-required` is present.
- [ ] Update the routes and handlers, add a `ProfileRequired bool` field to `profileForm`, and render the message as a calm banner above the form.
- [ ] Update titles, headings, canonical links, and tests so direct visits and browser back/forward navigation preserve the new route meanings.
- [ ] Run focused server tests and commit as `feat: make all postings the front page`.

### Task 3: Shared navigation and daily-briefing notification dot

**Files:**
- Create: `web/_nav.html`
- Create: `web/briefing-notification.js`
- Modify: `web/styles.css`
- Modify: `web/index.html`
- Modify: `web/archive.html`
- Modify: `web/bookmarks.html`
- Modify: `web/hidden.html`
- Modify: `web/profile.html`
- Modify: `internal/server/handlers.go`
- Modify: `internal/server/server_test.go`

**Interfaces:**
- Produces: `GET /api/briefing-status` returning `{ "profile_required": bool, "latest": "<RFC3339 or empty>" }`.
- Produces: localStorage key `jobcronBriefingSeenAt` and a shared `primaryNav` template.

- [ ] Add failing endpoint tests for no profile, no postings, and a newest posting first seen today.
- [ ] Add failing template tests requiring every application page to use the shared nav and the daily-briefing link to target `/briefing`.
- [ ] Implement the read-only status endpoint using the existing briefing-building rules so disabled, expired, and non-today postings do not create a false notification.
- [ ] Implement the shared nav partial with a hidden, accessible dot next to `데일리 브리핑`.
- [ ] Implement JavaScript that compares `latest` with `jobcronBriefingSeenAt`, shows the dot on other pages when newer content exists, and records `latest` only after `/briefing` loads successfully.
- [ ] Add restrained dot CSS that cannot shift nav layout and remains visible in light, dark, desktop, and mobile themes.
- [ ] Browser-test this flow: new posting shows the dot on `/`; clicking `데일리 브리핑` opens `/briefing`; returning to `/` shows the dot cleared; a later posting makes it reappear.
- [ ] Commit as `feat: add daily briefing notification state`.

### Task 4: Complete verification

**Files:**
- Modify: `docs/superpowers/README.md` after implementation to archive this plan and clear active work.

- [ ] Run `gofmt`, `go build ./...`, `go vet ./...`, and `go test ./... -count=1`.
- [ ] Run PostgreSQL 18 integration tests in a throwaway database and drop it afterward.
- [ ] Run frontend QA at `1440x900`, `1024x1366`, and `390x844` across `/`, `/briefing`, `/bookmarks`, `/hidden`, `/profile`, and `/login`, in light and dark themes.
- [ ] Confirm no console errors, failed first-party requests, horizontal overflow, layout shifts, or notification-dot overlap.
- [ ] Confirm the hosted `--demo` mutation tests still reject profile writes, visitor scrapes, rerates, bookmark writes, and hidden-post writes.
- [ ] Commit verification evidence locally and do not push.
