# README Deployment Status Refresh Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish accurate English and Korean deployment messaging, current public-safe screenshots, and a tracked one-line demo scrape helper.

**Architecture:** Keep the root READMEs local-first while distinguishing the live read-only demo from the not-yet-live production app. Capture the current application at the existing theme-aware screenshot paths. Preserve the helper as one line while moving the token-bearing headers from process arguments to curl's stdin.

**Tech Stack:** Markdown, POSIX shell, curl, Go application, gstack `/browse`, Git, Gitleaks

## Global Constraints

- `demo.jobcron.app` is live as a read-only demo.
- `jobcron.app` is not live yet; its deployment configuration is ready and launch is coming soon.
- Keep `deploy/demo/SCRAPE_CMD.sh` as a one-line helper.
- Never commit an API key, admin token, `.env` value, database, private profile text, or machine-specific path.
- Preserve the existing light/dark `<picture>` behavior and screenshot file paths.
- Commit locally, then push `main` only after all verification and publication gates pass.

---

### Task 1: Clarify Deployment Status in Both READMEs

**Files:**
- Modify: `README.md`
- Modify: `README.ko.md`

**Interfaces:**
- Consumes: the approved deployment facts in the design specification
- Produces: matching English and Korean public status messages

- [x] **Step 1: Add the English deployment callout**

Insert this block after the introductory product paragraph and before `<picture>`:

```markdown
> **Deployment status:** The read-only demo is live at
> [demo.jobcron.app](https://demo.jobcron.app). The full production app at
> `jobcron.app` is not publicly available yet; its deployment configuration is
> ready, and launch is coming soon. Until then, run the full app locally.
```

- [x] **Step 2: Add the Korean deployment callout**

Insert the equivalent block in `README.ko.md`:

```markdown
> **배포 상태:** 읽기 전용 데모는
> [demo.jobcron.app](https://demo.jobcron.app)에서 이용할 수 있습니다. 전체 기능을
> 제공하는 프로덕션 앱 `jobcron.app`은 아직 공개되지 않았지만 배포 준비를 마쳤으며,
> 곧 출시할 예정입니다. 그전까지는 전체 앱을 로컬에서 실행할 수 있습니다.
```

- [x] **Step 3: Correct local-only and source-list copy**

Replace the English absolute local-only bullet with:

```markdown
- The full app currently runs locally with no account or telemetry. A read-only
  web demo is live now, and the production web app is coming soon.
```

Update the Korean introduction and source bullet to match the current English
source set: 점핏, 랠릿, 데모데이, 그리팅, 당근, 크래프톤, 몰로코, 센드버드, plus
optional 워크넷. Replace its local-only bullet with the Korean equivalent of the
new English statement.

- [x] **Step 4: Verify README structure and links**

Run:

```bash
git diff --check -- README.md README.ko.md
rg -n 'demo\.jobcron\.app|jobcron\.app|coming soon|곧 출시' README.md README.ko.md
```

Expected: no whitespace errors; each README states both deployment facts.

- [x] **Step 5: Commit**

```bash
git add README.md README.ko.md
git commit -m "docs: clarify current deployment status"
```

---

### Task 2: Capture Current Light and Dark Screenshots

**Files:**
- Modify: `docs/assets/screenshots/dashboard.png`
- Modify: `docs/assets/screenshots/dashboard-dark.png`
- Modify: `README.md`
- Modify: `README.ko.md`

**Interfaces:**
- Consumes: the current committed app served on loopback with public-safe postings
- Produces: the existing theme-aware README screenshot pair

- [x] **Step 1: Confirm the screenshot target**

Use the current app at `http://127.0.0.1:17778/`. Confirm the page is current,
contains no credential or private profile form, and shows the current AI-analysis
chip presentation. Keep AI evidence panels collapsed so private goal text is not
published.

- [x] **Step 2: Capture the light screenshot**

With `/browse`, set a `1440x900` viewport, select light theme, reload, and capture
the viewport to:

```text
docs/assets/screenshots/dashboard.png
```

- [x] **Step 3: Capture the dark screenshot**

Select dark theme without changing route or state, reload, and capture the same
viewport to:

```text
docs/assets/screenshots/dashboard-dark.png
```

- [x] **Step 4: Inspect and verify**

Inspect both PNGs at full resolution. Verify readable text, no overlap or crop,
no expanded private evidence, no horizontal overflow, no console errors, and no
failed first-party asset requests. Click one representative source filter and
confirm the listing changes, then restore the all-sources view before the final
capture.

- [x] **Step 5: Update alt text and commit**

Use matching English alt text in both READMEs that names the all-postings page,
source filters, AI analysis chips, and light/dark themes. Then run:

```bash
git add README.md README.ko.md docs/assets/screenshots/dashboard.png docs/assets/screenshots/dashboard-dark.png
git commit -m "docs: refresh current application screenshots"
```

---

### Task 3: Track the One-Line Demo Scrape Helper Safely

**Files:**
- Add: `deploy/demo/SCRAPE_CMD.sh`

**Interfaces:**
- Consumes: `.env` variables `JOBCRON_ADMIN_TOKEN` or legacy `JOBSCRAPER_ADMIN_TOKEN`
- Produces: one authenticated streaming request to `https://demo.jobcron.app/api/scrape`

- [x] **Step 1: Preserve one-line behavior while removing token argv exposure**

Keep the file as exactly one shell line. Source `.env`, resolve the compatible
token variable, and pipe the two header lines to `curl -H @-` instead of expanding
the token inside `curl -H "..."` process arguments. Keep the existing missing-token
message and public endpoint.

- [x] **Step 2: Verify syntax without making a network request**

Run:

```bash
test "$(wc -l < deploy/demo/SCRAPE_CMD.sh | tr -d ' ')" = "1"
sh -n deploy/demo/SCRAPE_CMD.sh
! rg -n 'sk-ant-|sk-proj-|Bearer [A-Za-z0-9]' deploy/demo/SCRAPE_CMD.sh
```

Expected: every command exits 0 and the file remains one line.

- [x] **Step 3: Commit**

```bash
git add deploy/demo/SCRAPE_CMD.sh
git commit -m "chore: track demo scrape helper"
```

---

### Task 4: Verify, Archive, and Push

**Files:**
- Move: `docs/superpowers/specs/260713-readme-deployment-status-refresh.md`
- Move: `docs/superpowers/plans/260713-readme-deployment-status-refresh-implementation-plan.md`
- Modify: `docs/superpowers/README.md`

**Interfaces:**
- Consumes: Tasks 1-3
- Produces: public-safe committed documentation and synchronized `origin/main`

- [x] **Step 1: Run project and publication gates**

Run formatting, vet, focused tests, Node lifecycle tests, the exact race selector,
the full Go suite, build, Markdown diff checks, `sh -n`, and Gitleaks across the
unpublished range. Every command must exit 0.

- [x] **Step 2: Review public artifacts manually**

Review both README diffs, both full-resolution PNGs, and the helper. Confirm there
are no credentials, private profile details, private infrastructure identifiers,
machine paths, databases, generated QA screenshots, or unrelated files.

- [x] **Step 3: Archive the completed design and plan**

Move both records into:

```text
docs/superpowers/archive/2026-07-13-readme-deployment-status-refresh/
```

Set `docs/superpowers/README.md` Active Work to `None.` and add both records under
Recently Archived.

- [x] **Step 4: Commit lifecycle documentation**

```bash
git add docs/superpowers
git commit -m "docs: archive README deployment refresh"
```

- [x] **Step 5: Push and verify**

Fetch `origin`, confirm `0` remote-only commits, push `main`, fetch again, and
verify local `HEAD` equals `origin/main`. Do not create a PR or deploy the app.
