# Jobcron Hard Rename Design

Date: 2026-07-11
Status: Approved direction; Tasks 1-5 integrated; Task 6 documents the completed rename

## Summary

Rename the product, repository contract, commands, runtime configuration, build
artifacts, and active documentation from `job-scraper` / `jobscraper` to
`jobcron` before the first production launch.

This is a hard rename: `jobcron` becomes the only supported application name and
`JOBCRON_*` becomes the only supported application-specific environment prefix.
The implementation will not add temporary `JOBSCRAPER_*` aliases because no
known external deployment depends on them and the production EC2 environment can
be updated directly.

The hard rename has three deliberate compatibility exceptions:

1. Existing MacBook application data is migrated automatically instead of being
   hidden by the new default directory.
2. Already-applied SQL migrations remain immutable; a new migration updates the
   legacy import-owner identity where safe.
3. The Gas Town rig remains named `jobscraper` so beads, polecat worktrees,
   recovery records, and current coordination paths remain valid.

## Goals

- Make `jobcron` the consistent product, command, module, image, and deployment
  name.
- Rename source commands to `./cmd/jobcron`, `./cmd/jobcron-user`, and
  `./cmd/jobcron-import`.
- Rename every supported `JOBSCRAPER_*` variable to `JOBCRON_*`.
- Preserve the existing MacBook SQLite database, write-ahead-log files, backups,
  and AI key file through a one-time directory migration.
- Preserve PostgreSQL data and migration history.
- Keep secrets out of Git.
- Keep all commits local and do not push. The human has completed the GitHub
  repository rename.

## Non-Goals

- Renaming the Gas Town rig, bead prefix, polecats, recovery sandboxes, or
  `/Users/chanbla11mit/gt/jobscraper` workspace path during this implementation
  phase. A separate Gas Town migration phase remains required.
- Providing long-term compatibility for old command names or `JOBSCRAPER_*`
  variables.
- Rewriting historical command output in reports as if it had originally used
  the new name.
- Deleting old application data, Docker volumes, backups, or recovery state.
- Changing `DATABASE_URL` or `SESSION_SECRET`; these are generic production
  contracts, not product-prefixed variables.

## Canonical Naming

| Surface             | Legacy                            | Canonical                         |
| ------------------- | --------------------------------- | --------------------------------- |
| Product             | `job-scraper`                     | `jobcron`                         |
| Go module           | `github.com/ohchanwu/job-scraper` | `github.com/ohchanwu/jobcron`     |
| Main command path   | `./cmd/job-scraper`               | `./cmd/jobcron`                   |
| User command path   | `./cmd/job-scraper-user`          | `./cmd/jobcron-user`              |
| Import command path | `./cmd/job-scraper-import`        | `./cmd/jobcron-import`            |
| Binary              | `job-scraper`                     | `jobcron`                         |
| Image variable      | `JOBSCRAPER_IMAGE`                | `JOBCRON_IMAGE`                   |
| Docker image        | registry-dependent                | `ohchanwu/jobcron:0.2-linuxarm64` |
| EC2 app directory   | `/srv/job-scraper`                | `/srv/jobcron`                    |
| macOS app directory | `job-scraper`                     | `jobcron`                         |
| Runtime prefix      | `JOBSCRAPER_*`                    | `JOBCRON_*`                       |
| Session cookie      | `job_scraper_session`             | `jobcron_session`                 |
| CSRF cookie         | `job_scraper_csrf`                | `jobcron_csrf`                    |
| Demo storage prefix | `jobScraperDemo*`                 | `jobcronDemo*`                    |

The public domain, PostgreSQL database, and PostgreSQL owner already use
`jobcron.app`, `jobcron`, and `jobcron_admin` and do not need renaming.

## Source And Module Rename

Rename all three product command directories and update their package comments,
usage text, log prefixes, version output, tests, Docker builds, CI commands, and
GoReleaser configuration.

Change the module declaration and every internal import in one commit so the Go
package graph is never left in a mixed state. Release archive names become
`jobcron_<os>_<arch>` and active README download URLs point at
`github.com/ohchanwu/jobcron`.

The helper command `cmd/capture` keeps its generic command name but moves its
imports to the new module path. Any comments that describe which product binary
CI builds are updated.

## Environment Contract

Rename all 18 repository references using the `JOBSCRAPER_` prefix:

- `JOBCRON_ADMIN_TOKEN`
- `JOBCRON_AI_MODEL`
- `JOBCRON_ANTHROPIC_KEY`
- `JOBCRON_DAILY_SCRAPE_TIME`
- `JOBCRON_DB`
- `JOBCRON_DEMO`
- `JOBCRON_DEMODAY_ANON_KEY`
- `JOBCRON_ENV`
- `JOBCRON_HOST`
- `JOBCRON_IMAGE`
- `JOBCRON_NO_OPEN`
- `JOBCRON_OWNER_PASSWORD`
- `JOBCRON_PORT`
- `JOBCRON_PROXY_SECRET`
- `JOBCRON_SCHEDULER_ENABLED`
- `JOBCRON_TEST_POSTGRES_URL`
- `JOBCRON_USE_HEADLESS`
- `JOBCRON_WORKNET_KEY`

Configuration errors, help text, integration-test opt-ins, Compose interpolation,
and documentation must all use the canonical names. Old names are rejected
rather than silently accepted.

The production schedule changes from `09:00` to `05:00`. The scheduler already
calculates the next run in Korea Standard Time, so this does not depend on the
EC2 system timezone.

## MacBook Data Migration

The current MacBook contains
`~/Library/Application Support/job-scraper/` with `jobs.db`, SQLite sidecar
files, backups, and `ai_keys.json`. A simple default-path rename would make the
application appear empty.

Add one application-data preparation helper and run it before opening the
database or AI key store.

Migration rules:

1. If neither legacy nor canonical directory exists, continue normally; the
   canonical directory is created on demand.
2. If only the canonical directory exists, continue normally.
3. If only the legacy directory exists, atomically rename the whole directory
   from `job-scraper` to `jobcron`. Moving the entire directory keeps the SQLite
   database, write-ahead log, shared-memory file, backups, and AI keys together.
4. If both directories exist, stop with an actionable collision error. Do not
   merge, overwrite, or delete either directory automatically.
5. If the rename fails, return the error and leave the legacy directory intact.

The migration assumes only one application process is running. Documentation
must tell the human to stop the old binary before first starting `jobcron`.
Rollback is: stop `jobcron`, confirm no process holds the database, and rename
the directory back to `job-scraper`.

Tests cover all four directory states, preservation of nested files, rename
failure, collision behavior, and idempotent repeated startup.

## PostgreSQL And Import Identity

Do not modify the applied `0006_user_scoped_state.sql` migration. It contains
the historical sentinel user `sqlite-import-owner@job-scraper.local`, and
changing migration history would not update databases that already recorded
version 6.

Add a new PostgreSQL migration that changes the sentinel email to
`sqlite-import-owner@jobcron.local` only when:

- the old sentinel email exists,
- its password hash is still `imported-sqlite-no-login`, and
- no user already has the new sentinel email.

Then change the import command's default owner email to the new sentinel. Real
owner accounts and arbitrary user email addresses are not modified.

Migration tests must cover a legacy sentinel, an already-renamed sentinel, an
existing canonical-email collision, and a real owner account that must remain
unchanged.

## Browser And HTTP Identity

Rename the session and CSRF cookie names and the demo bookmark/hidden-job local
storage keys. Existing browser sessions and demo-only browser state will not be
migrated. This is acceptable before the first production launch and avoids
carrying compatibility code indefinitely.

Rename product-specific HTTP header names such as
`X-JobScraper-Admin-Token` to `X-Jobcron-Admin-Token`. The existing
`X-Jobcron-Proxy` header is already canonical.

## Docker And Deployment

Update demo and production Dockerfiles to build `./cmd/jobcron`, copy a
`jobcron` binary, and use `ENTRYPOINT ["jobcron"]`.

Production Compose uses `JOBCRON_IMAGE` with
`ohchanwu/jobcron:0.2-linuxarm64`, the `05:00` Korea Standard Time schedule, and
the renamed runtime variables. Worknet and proxy-secret behavior remain
unchanged: both stay disabled for the first production pass, and Caddy remains
the only public entry point.

Human-owned deployment actions:

- [x] Rename the GitHub repository to `ohchanwu/jobcron`.
  - **overseer feedback - settled:** The repository rename is complete. The
    canonical URLs are `git@github.com:ohchanwu/jobcron.git` for SSH and
    `https://github.com/ohchanwu/jobcron.git` for HTTPS.
- [x] Update each independent local clone's configured `origin` URL, beginning
  with this checkout.
  - **overseer feedback - settled:** GitHub redirects the old repository URL but
    cannot rewrite local `.git/config` files. This checkout now uses
    `git@github.com:ohchanwu/jobcron.git` for both fetch and push. Linked
    worktrees sharing its Git common directory inherit that update; independent
    clones still need their own update.
- [ ] Rename or recreate the EC2 app directory as `/srv/jobcron` and move the
  existing server-only `.env` without exposing its values.
- [x] Rename all application-prefixed variables in the EC2 `.env`.
  - **overseer feedback - settled:** The EC2 environment-variable rename is
    complete.
- [x] Set the EC2 scrape schedule to `05:00` Korea Standard Time.
  - **overseer feedback - settled:** The production schedule is updated on the
    EC2 instance.
- [ ] Pull the canonical image after it is built and pushed.

No secret value is added to repository files or reports.

For local PostgreSQL, use canonical Compose names and a fresh canonical
development database/volume while leaving the old Docker volume intact for
rollback. Do not delete or repurpose the old volume automatically. Historical
reports may continue to reference the old local database names used during past
verification.

## Documentation Policy

Update active interfaces and instructions:

- README files
- deployment guides and environment templates
- current production deployment report
- CI and release instructions
- agent-facing architecture documentation where it describes current commands

Historical plans and verification reports should retain commands and paths that
were actually used. Add a short note that the application was later renamed to
`jobcron` instead of rewriting historical evidence.

Mark the human's completed EC2 `.env` feedback as settled in the production
deployment report. Consolidate duplicate naming feedback, set the schedule to
`05:00`, and record `JOBCRON_IMAGE=ohchanwu/jobcron:0.2-linuxarm64` without
including secrets.

## Gas Town Boundary

Keep the rig name and path as `jobscraper` during the application hard-rename
implementation. Do not rename beads, branches, polecats, recovery sandboxes, or
the workspace directory as part of the code and deployment change.

**overseer feedback - deferred:** The Gas Town rig, workspace path, bead-facing
identity, and related operational names should eventually become `jobcron` too.
That work is intentionally deferred, not rejected. It needs its own live-state
inventory, recovery plan, and migration procedure after the application rename
is verified. Recovery sandboxes remain intact unless live recovery checks later
authorize cleanup.

The GitHub repository rename and local Git `origin` update do not require the Gas
Town rig to be renamed at the same time. Linked worktrees sharing one Git common
directory inherit a single `origin` update; independent clones do not.

## Verification

Implementation is complete only after all of the following pass:

- `gofmt -l .` prints nothing.
- `go build ./cmd/jobcron ./cmd/jobcron-user ./cmd/jobcron-import` passes.
- `go vet ./...` passes.
- `go test ./...` passes.
- PostgreSQL integration tests pass with `JOBCRON_TEST_POSTGRES_URL`.
- GoReleaser configuration validates and produces canonical archive names.
- Both Dockerfiles build for Linux arm64.
- Demo and production Compose configurations render with only `JOBCRON_*`
  application variables.
- Repository scanning finds no active-interface `JOBSCRAPER_*`, `job-scraper`,
  or `job_scraper` references outside approved historical and Gas Town contexts.
- MacBook data migration tests pass for empty, legacy-only, canonical-only, and
  collision states.
- A temporary-directory migration test proves database sidecars, backups, and
  AI keys move together.
- A clean PostgreSQL database applies every migration, including the sentinel
  rename.
- The production scheduler configuration resolves to `05:00` Korea Standard
  Time.

Because this work changes runtime paths and deployment configuration, commit at
meaningful checkpoints but do not push. The human reviews the local commits and
performs the remote and EC2 changes.

## Rollback

The GitHub repository is already renamed. GitHub's repository redirect can
temporarily serve old clone URLs, but the human would need to restore the old
repository name if a full naming rollback were required. Application rollback
is a normal Git revert plus renaming the MacBook application directory back
after stopping the app. The old Docker volume and historical application-data
backups remain untouched, providing data-level rollback without destructive
cleanup.

## Acceptance Criteria

- New builds, logs, commands, runtime variables, images, active docs, and public
  URLs consistently say `jobcron`.
- The existing MacBook application starts with its prior data and AI key file
  after the one-time migration.
- Existing PostgreSQL data remains accessible and migration history stays
  append-only.
- Production deployment uses `05:00` Korea Standard Time and
  `ohchanwu/jobcron:0.2-linuxarm64`.
- No secrets enter Git.
- Gas Town continues operating under the `jobscraper` rig without recovery or
  bead disruption during this phase, with its eventual rename recorded as a
  separate migration project.
