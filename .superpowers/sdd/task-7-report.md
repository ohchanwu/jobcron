# Rename Task 7 Report: Full Verification And Local Handoff

## Result

The jobcron hard rename is verified from baseline `01cdb02` through implementation
HEAD `0a1a8d0`. No implementation defect required a source-code fix. This report
and the progress ledger are the only Task 7 file changes.

The local Git remote was already canonical:

```text
git@github.com:ohchanwu/jobcron.git
```

No push or other remote write was performed.

The ignored local production build report remains part of the handoff at this
exact path:

```text
docs/superpowers/specs/2026-07-10-production-local-build-report.md
```

## Go Verification

All required commands passed from a clean tracked worktree:

```sh
gofmt -w cmd internal
test -z "$(gofmt -l .)"
go list ./...
go build ./cmd/jobcron ./cmd/jobcron-user ./cmd/jobcron-import
go vet ./...
go test ./... -count=1
```

`go list ./...` returned only packages under
`github.com/ohchanwu/jobcron`. Formatting produced no changes.

## PostgreSQL 18 Integration

The local container `local-postgres-1` ran `postgres:18-alpine`, server version
`18.4`, on port `55432`.

A uniquely named database, `jobcron_rename_test_1783735769_54740`, was created,
used only for this command, and dropped successfully by a cleanup trap:

```sh
JOBCRON_TEST_POSTGRES_URL='postgres://postgres@localhost:55432/jobcron_rename_test_1783735769_54740?sslmode=disable' \
  go test ./cmd/jobcron-import ./cmd/jobcron-user ./internal/storage -count=1
```

All three packages passed. The legacy `jobscraper_dev` database and production
data were not accessed or modified.

## Real Command-Path Migration

The real command path ran under temporary home
`/tmp/jobcron-task7-home.Nt1HL6`:

```sh
HOME="$temporary_home" \
JOBCRON_DB="$temporary_home/runtime.db" \
JOBCRON_HOST=127.0.0.1 \
JOBCRON_PORT=17777 \
go run ./cmd/jobcron --no-open
```

The temporary legacy macOS config directory contained:

- `jobs.db`
- `jobs.db-wal`
- `jobs.db-shm`
- `ai_keys.json`

The command started on `127.0.0.1:17777`, renamed the entire legacy directory to
`jobcron`, removed the old directory path, and preserved identical SHA-256
hashes for all four files. The temporary home was removed after verification.

## Frontend QA

The preview uses an isolated PostgreSQL 18 database named
`jobcron_task7_preview_1783735875`, a temporary owner account, one temporary
posting, and a temporary config home. It remains available for local inspection:

```text
http://127.0.0.1:17778
```

The browser workflow used the gstack headless browse CLI. It logged in through
the real production UI and exercised the archive card at both required
viewports:

- Desktop: `1440x900`
- Mobile: `390x844`

The bookmark and hide handlers were exercised with the page's demo-state flag
enabled so the JavaScript used browser storage without changing PostgreSQL
bookmark or hidden-job rows. Evidence:

```text
jobcronDemoBookmarks = ["1"]
jobcronDemoHidden = ["1"]
jobScraperDemoBookmarks = null
jobScraperDemoHidden = null
bookmark aria-pressed = true
hidden card hidden = true
```

No console errors appeared. Every local asset request completed successfully;
the only non-200 responses were expected login redirects. Desktop and mobile
both passed `document.documentElement.scrollWidth <= window.innerWidth + 2`.
Visual inspection found no overlap, clipping, text containment problem, or
responsive regression.

Screenshots:

- `/tmp/jobcron-task7-desktop-bookmarked.png`
- `/tmp/jobcron-task7-desktop-hidden-recapture.png`
- `/tmp/jobcron-task7-mobile-bookmarked.png`

Controller follow-up recaptured the hidden-post state after a two-second settle.
The page had no active animations, the posting card was hidden, the body
background remained `rgb(251, 247, 239)`, and the recapture visually contained
no black rectangle. The original and recaptured PNGs were byte-for-byte
identical (SHA-256
`d460fcd974473b646dd5ec3c2e78b42f986959d75347abbc03f16738cf548454`,
zero differing pixels). Both are opaque 1440x900 PNGs. The reported black region
was therefore a downstream image display/decode artifact, not a product render
or screenshot-timing defect.

## Container, Compose, And Release Checks

Both Linux arm64 images built locally and expose `jobcron` as the entrypoint:

```text
jobcron:task7-production  arch=arm64 os=linux entrypoint=["jobcron"]
jobcron:task7-demo        arch=arm64 os=linux entrypoint=["jobcron"]
```

The demo, local PostgreSQL, and production Compose files all rendered. The
rendered application contracts contained no active `JOBSCRAPER_*` or
`job-scraper` name. Production rendered `JOBCRON_DAILY_SCRAPE_TIME: "05:00"`;
local PostgreSQL rendered `postgres:18-alpine`, `jobcron_dev`, and
`jobcron-postgres18-cluster`.

GoReleaser was not installed globally, so the verification used a temporary
module invocation:

```sh
go run github.com/goreleaser/goreleaser/v2@latest check
go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean
```

GoReleaser `v2.17.0` validated one configuration file and produced the full
snapshot archive matrix with canonical `jobcron_*` names plus checksums. The
tool selected Go `1.26.5` because that GoReleaser version requires Go 1.26.4 or
newer. The snapshot succeeded in 33 seconds and left no tracked changes.

## Repository Scans

The old-name scan found 170 permitted occurrences covered by the design
allowlist and zero uncategorized files. Related allowlist categories are grouped
below:

```text
legacy Docker rollback volume                         1
dated historical evidence                            72
source mappings and deferred Gas Town design          84
legacyDirName app-data migration                       1
rejection tests and migration fixtures                 8
immutable migrations and migration 0013 sentinel       4
```

All historical files containing old names carry the required 2026-07-11 rename
note. The tracked secret scan found one deliberately synthetic pre-existing
test string in `internal/ai/injection_test.go`; the cumulative rename diff added
zero secret-like values. Thirteen local Markdown links were checked and none
were broken.

### Exact Reproducible Scan Script

The old-name categorization, secret scan, cumulative-diff secret scan, and local
Markdown-link validation were rerun from the repository root with context-mode
`ctx_execute`, `language: "javascript"`, using this exact script:

```javascript
const fs = require("fs");
const path = require("path");
const cp = require("child_process");

const files = cp.execSync("git ls-files", { encoding: "utf8" })
  .trim().split("\n").filter(Boolean);

const oldName = /(job-scraper|jobscraper|job_scraper|JOBSCRAPER|JobScraper|jobScraper)/g;
const historical = new Set([
  "docs/superpowers/plans/2026-07-09-jobcron-production-app.md",
  "docs/superpowers/specs/2026-07-10-production-deploy-prep-report.md",
  "internal/scraper/greeting/API_NOTES.md",
]);
const mapping = new Set([
  "docs/superpowers/plans/2026-07-11-jobcron-hard-rename-implementation.md",
  "docs/superpowers/specs/2026-07-11-jobcron-hard-rename-design.md",
]);
const rejection = new Set([
  "internal/config/config_test.go",
  "internal/server/demo_test.go",
  "internal/storage/postgres_integration_test.go",
]);
const migrations = /^internal\/storage\/(?:postgres_)?migrations\/(?:0001|0006|0013)/;
const counts = {}, uncategorized = [], missingNotes = [];
let oldTotal = 0;

for (const file of files) {
  if (file.startsWith(".superpowers/sdd/")) continue;
  let text;
  try { text = fs.readFileSync(file, "utf8"); } catch { continue; }
  const hits = [...text.matchAll(oldName)];
  if (!hits.length) continue;
  oldTotal += hits.length;
  let category = "";
  if (historical.has(file)) {
    category = "historical";
    if (!/Rename note \(2026-07-11\)/.test(text.slice(0, 1200))) missingNotes.push(file);
  } else if (mapping.has(file)) category = "mapping_or_gastown";
  else if (migrations.test(file)) category = "migration";
  else if (rejection.has(file)) category = "rejection_or_fixture";
  else if (file === "internal/appdata/paths.go") category = "legacy_appdata";
  else if (file === "deploy/local/README.md") category = "rollback_volume";
  else uncategorized.push(file);
  counts[category] = (counts[category] || 0) + hits.length;
}

const secretPatterns = [
  ["aws", /\b(?:AKIA|ASIA)[A-Z0-9]{16}\b/g],
  ["private", /-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----/g],
  ["anthropic", /\bsk-ant-[A-Za-z0-9_-]{20,}\b/g],
  ["openai", /\bsk-(?:proj-)?[A-Za-z0-9_-]{20,}\b/g],
  ["github", /\bgh[pousr]_[A-Za-z0-9]{20,}\b/g],
  ["jwt", /\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b/g],
];
const secretHits = [];
for (const file of files) {
  let text;
  try { text = fs.readFileSync(file, "utf8"); } catch { continue; }
  for (const [kind, re] of secretPatterns) {
    for (const match of text.matchAll(re)) {
      if (file === "internal/ai/injection_test.go" &&
          match[0] === ["sk-ant", "super-secret-key"].join("-")) continue;
      secretHits.push(kind + ":" + file);
    }
  }
}
const diff = cp.execSync("git diff 01cdb02..HEAD", {
  encoding: "utf8", maxBuffer: 50 * 1024 * 1024,
});
const diffSecretHits = secretPatterns.flatMap(([kind, re]) =>
  [...diff.matchAll(re)].map(() => kind)
);

let checkedLinks = 0;
const brokenLinks = [];
for (const file of files.filter(file => file.endsWith(".md"))) {
  const text = fs.readFileSync(file, "utf8");
  for (const match of text.matchAll(/\[[^\]]*\]\(([^)]+)\)/g)) {
    let target = match[1].trim().replace(/^<|>$/g, "").split(/\s+["']/)[0];
    if (!target || target.startsWith("#") || /^[a-z]+:/i.test(target) ||
        target.includes("{{") || target.includes("<") ||
        target.includes(">") || target.includes("*")) continue;
    target = decodeURIComponent(target.split("#")[0].split("?")[0]);
    if (!target) continue;
    checkedLinks++;
    const resolved = target.startsWith("/")
      ? target : path.resolve(path.dirname(file), target);
    if (!fs.existsSync(resolved)) brokenLinks.push(file + " -> " + target);
  }
}

console.log("old_name_total=" + oldTotal);
console.log("old_name_categories=" + JSON.stringify(counts));
console.log("old_name_uncategorized=" + uncategorized.length);
console.log("historical_missing_rename_note=" + missingNotes.length);
console.log("tracked_secret_findings=" + secretHits.length);
console.log("cumulative_diff_secret_findings=" + diffSecretHits.length);
console.log("local_markdown_links_checked=" + checkedLinks);
console.log("broken_local_markdown_links=" + brokenLinks.length);
```

The exact output was:

```text
old_name_total=170
old_name_categories={"rollback_volume":1,"historical":72,"mapping_or_gastown":84,"legacy_appdata":1,"rejection_or_fixture":8,"migration":4}
old_name_uncategorized=0
historical_missing_rename_note=0
tracked_secret_findings=0
cumulative_diff_secret_findings=0
local_markdown_links_checked=13
broken_local_markdown_links=0
```

## Cumulative Review

The cumulative diff from `01cdb02` through `0a1a8d0` was reread with rename
detection and checked against the hard-rename design.

```sh
git diff --check 01cdb02..0a1a8d0
git diff --find-renames 01cdb02..0a1a8d0
git diff --stat 01cdb02..0a1a8d0
git log --reverse --oneline 01cdb02..0a1a8d0
```

The review confirmed the module and commands, runtime variables, app-data
migration, PostgreSQL sentinel migration, browser and HTTP identities,
container/release contracts, and active documentation all match the design.
Append-only migrations, rollback identifiers, historical evidence, and the Gas
Town identity remain unchanged only where explicitly allowed. No user edit was
discarded, no unrelated refactor entered the diff, and `git diff --check`
reported no whitespace error.

## Final Mayor Inbox Evidence

The controller ran this exact command from `/Users/chanbla11mit/gt/mayor`:

```sh
gt mail inbox --unread
```

Final output:

```text
Inbox: mayor/ (59 messages, 0 unread)
(no messages)
```

This read-only inbox check is the only Gas Town operation included as Task 7
evidence. Task 7 itself did not mutate Gas Town state.

### Independent Mayor Operational Duty

High escalation `hq-wisp-2tu5mv` was handled separately after Task 7 because a
high-priority user/system alert required the Mayor to process it before going
idle. It was not rename verification and is not part of Task 7. The escalation
was read, live verification showed the shared Dolt server healthy on port
`33327`, and it was acknowledged without a restart or broad repair command.

## Exact Human Deployment Handoff

Run these steps in order. This autonomous run did not and will not push. A human
must review and push the local commits, prove that the reviewed commit and
deployment files are present on the remote, and prove that the reviewed image is
present in the registry before stopping the current production stack. The image
is built on the local MacBook and pushed to the registry; EC2 only pulls the
prebuilt image.

1. On the MacBook, review the complete local rename history and choose the exact
   commit approved for deployment:

   ```sh
   cd /path/to/jobcron
   git status --short
   git log --oneline 01cdb02..HEAD
   git diff --check 01cdb02..HEAD
   git diff --stat 01cdb02..HEAD
   REVIEWED_COMMIT=<full-reviewed-commit-sha>
   test "$(git rev-parse "$REVIEWED_COMMIT")" = "$REVIEWED_COMMIT"
   ```

2. After human approval, the human pushes the reviewed branch. This command is
   intentionally not run by the autonomous agent:

   ```sh
   git push origin main
   ```

3. Verify that GitHub/origin contains the exact reviewed commit and its
   production deployment files:

   ```sh
   git fetch origin
   test "$(git rev-parse origin/main)" = "$REVIEWED_COMMIT"
   git ls-remote origin refs/heads/main | awk '{print $1}' | grep -Fx "$REVIEWED_COMMIT"
   git show "origin/main:deploy/production/compose.yaml" >/dev/null
   git show "origin/main:deploy/production/Caddyfile" >/dev/null
   git show "origin/main:deploy/production/HUMAN_DEPLOY_GUIDE.md" >/dev/null
   ```

4. On the MacBook, build and push the canonical Linux arm64 image from that
   reviewed commit, then verify the registry contains it:

   ```sh
   cd /path/to/jobcron
   test "$(git rev-parse HEAD)" = "$REVIEWED_COMMIT"
   IMAGE=ohchanwu/jobcron:0.2-linuxarm64
   docker buildx build --platform linux/arm64 \
     -f deploy/production/Dockerfile \
     -t "$IMAGE" \
     --push .
   docker buildx imagetools inspect "$IMAGE"
   ```

   Do not continue to EC2 until steps 1-4 all pass. In particular, do not stop
   the current production stack until both the reviewed remote commit and the
   registry image are confirmed.

5. On EC2, stop the old production stack before moving its checkout and
   server-only `.env` together:

   ```sh
   test -d /srv/job-scraper
   test ! -e /srv/jobcron
   cd /srv/job-scraper/deploy/production
   docker compose --env-file .env down
   cd /srv
   sudo mv job-scraper jobcron
   sudo chown -R ec2-user:ec2-user /srv/jobcron
   ```

6. Update the moved checkout from the canonical repository and verify EC2
   checked out the same reviewed commit:

   ```sh
   REVIEWED_COMMIT=<full-reviewed-commit-sha>
   cd /srv/jobcron
   git remote set-url origin git@github.com:ohchanwu/jobcron.git
   git pull --ff-only
   test "$(git rev-parse HEAD)" = "$REVIEWED_COMMIT"
   ```

7. Validate the server-only production environment without printing secrets:

   ```sh
   cd /srv/jobcron/deploy/production
   test -f .env
   chmod 600 .env
   grep -Fx 'JOBCRON_IMAGE=ohchanwu/jobcron:0.2-linuxarm64' .env
   grep -q '^DATABASE_URL=' .env
   grep -q '^SESSION_SECRET=' .env
   ! grep -Eq '^(JOBSCRAPER_|JOBCRON_DEMO=|JOBCRON_ADMIN_TOKEN=|JOBCRON_PROXY_SECRET=|JOBCRON_WORKNET_KEY=)' .env
   ```

   Keep real database, session, owner-password, and API-key values only in the
   server `.env` or interactive owner command. Never add them to Git. Worknet
   remains off. Neither `JOBCRON_PROXY_SECRET` nor
   `JOBSCRAPER_PROXY_SECRET` is present.

8. Authenticate to the registry if required, then pull and start the prebuilt
   image. Do not build on EC2:

   ```sh
   docker login <registry>  # only when the registry requires it
   cd /srv/jobcron/deploy/production
   docker compose --env-file .env pull
   docker compose --env-file .env up -d
   docker compose ps
   docker compose logs --tail=200 app caddy
   ```

   Do not run `docker compose build` or `docker compose up --build` on EC2.

9. Create the owner account from a source checkout with Go and RDS network
   access, entering the temporary password outside Git:

   ```sh
   export DATABASE_URL='postgres://jobcron_admin:<database-password>@<rds-endpoint>:5432/jobcron?sslmode=require'
   export JOBCRON_OWNER_PASSWORD='<temporary-owner-password>'
   go run ./cmd/jobcron-user create-owner \
     --database-url "$DATABASE_URL" \
     --email 'ohchanwu@gmail.com'
   unset JOBCRON_OWNER_PASSWORD
   ```

10. In a real browser, verify HTTPS, redirects, owner login, PostgreSQL-backed
   data, and the `05:00` Korea Standard Time schedule:

   ```text
   https://jobcron.app/
   https://www.jobcron.app/
   ```

   Caddy must be the only public entry point on ports `80` and `443`. The app
   remains private inside the Compose network as `app:7777`; no app port is
   published directly and no shared proxy secret is used.

## Limitations And Human Handoff

- The browser-storage interaction used the production PostgreSQL UI with the
  page demo-state flag enabled. The deployed public demo is intentionally
  SQLite-only; combining demo mode with PostgreSQL is not a supported runtime
  contract.
- The preview database and server remain running for human inspection. They
  contain only temporary Task 7 fixtures and must not be mistaken for production
  data.
- The Gas Town rig and workspace still use `jobscraper`. Their rename remains a
  separate migration project.
- The ordered `/srv/jobcron` migration and image-pull procedure above remains a
  human-owned deployment action. No EC2 state or secret was changed during Task
  7.
- No remote branch was changed and nothing was pushed.
