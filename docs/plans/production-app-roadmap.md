# Plan: turn jobcron.app from demo into production app

**Status:** planning document, no implementation started  
**Date:** 2026-07-09  
**Decision:** build Option A first: a single-user production app. Keep Option B, multi-user accounts, as the next product milestone.

## Goal

The AWS demo proves the product works as a public read-only briefing. The next step is to make it a production app that can run every day without manual operator work, persist state safely on the server, and support a real login session.

The first production version should still be scoped for one primary user. That keeps the migration focused on reliability:

- PostgreSQL instead of SQLite for production.
- Login and cookie-based sessions.
- Profile, bookmark, and hidden-posting state stored server-side.
- Automated daily scrape.
- Clear handling of whether AI scoring runs automatically.

Multi-user support comes after that foundation is stable.

## Current state

- The app is a Go web app with embedded HTML/CSS/JS.
- Storage is a SQLite file opened through `internal/storage`.
- Schema migrations are embedded SQL files under `internal/storage/migrations`.
- Profile state is currently a single global profile row.
- Bookmarks and hidden postings exist in the database for the local app.
- Public demo mode blocks normal visitor writes.
- Demo bookmark and hide state are handled in browser localStorage so visitors cannot mutate the database.
- Scraping is triggered manually through `GET /api/scrape` with an admin token in demo mode.
- AI keys live in `ai_keys.json`, not in the database.
- AI scoring can run during scrape when AI is configured, and cached AI results are stored in the database.

## Milestone A: single-user production app

This is the recommended first milestone.

The app should behave like a private production tool for one owner:

- One login account.
- One saved profile.
- One set of bookmarks and hidden postings.
- One production PostgreSQL database.
- Daily scrape runs automatically.
- Manual scrape remains available as an operator fallback.

This milestone deliberately avoids public signup, password reset, per-user billing, user administration, and abuse controls. Those become part of Milestone B.

## Milestone B: multi-user app

This is the next milestone after A.

The app should support multiple independent users:

- **overseer feedback:** Public signup, not invite-only signup, is the target for Milestone B.
- Separate profile per user.
- Separate bookmark and hidden state per user.
- User-specific scores, because scoring depends on the user's profile.
- **overseer feedback:** Per-user AI settings should use bring-your-own-key (BYOK). The app should make it easy for users to find provider setup pages:
  - Claude / Anthropic Console: <https://console.anthropic.com/>
  - Anthropic API docs: <https://platform.claude.com/docs/en/get-started>
  - OpenAI API keys: <https://platform.openai.com/api-keys>
  - OpenAI API docs: <https://developers.openai.com/api/docs/quickstart>
  Current app support should stay Anthropic-first unless an OpenAI provider is reintroduced later. Budget copy should tell users that expected usage is roughly around USD $5/month for this product shape, with the exact cost depending on model, scrape volume, and how often they request AI scoring.
- Stronger abuse controls around login, scrape visibility, AI spend, and account creation.

Milestone A should avoid designs that block Milestone B. For example, even if A has only one user, the database should introduce `users` and `user_id` columns early so the later migration is smaller.

## Major workstreams

### 1. PostgreSQL storage

Add PostgreSQL as the production database while keeping SQLite usable for local development and tests during the transition.

To do:

- Add a storage driver abstraction or dialect layer around `database/sql`.
- Add PostgreSQL migrations equivalent to the existing SQLite migrations.
- Replace SQLite-specific SQL where needed.
- Add `DATABASE_URL` config.
- Make production startup refuse to run without `DATABASE_URL`.
- Add a one-time import command from existing `jobs.db` to PostgreSQL.
- Preserve current postings, scores, bookmarks, hidden postings, profile, AI extraction cache, AI score cache, and AI usage rows during import.
- Decide whether SQLite remains supported long term as local-only development storage.

PostgreSQL tables should include:

- `users`
- `sessions`
- `postings`
- `scores`
- `profiles`
- `bookmarks`
- `not_interested`
- `ai_extractions`
- `ai_scores`
- `ai_usage`
- `scrape_runs`

### 2. Login and cookie sessions

Move production identity into server-managed login and secure cookies.

To do:

- Add a `users` table.
- Add one owner account for Milestone A.
- Store password hashes using Argon2id or bcrypt.
  - **overseer feedback:** Prefer Argon2id unless deployment constraints force bcrypt. Argon2id is the current stronger default for new systems because it is memory-hard, making large offline cracking attempts more expensive. Its downside is slightly more tuning work and more memory use per login. Bcrypt is older, widely available, and operationally simple, but it is CPU-hard rather than memory-hard and has awkward password length behavior. For this app, Argon2id is the better first choice; bcrypt is the fallback if the chosen Go dependency or deployment memory profile causes problems.
- Add `sessions` table with hashed session tokens.
- Set secure, HTTP-only, SameSite cookies.
- Add login and logout routes.
- Add middleware that loads the current user from the session cookie.
- Protect profile, bookmark, hidden, AI settings, scrape, and admin endpoints.
- Add CSRF protection for state-changing requests.
- Add login rate limiting.

Milestone A can avoid signup. Account creation can be done with a CLI command or one-time admin setup command.

### 3. Move user state out of localStorage

Production user state should live in PostgreSQL.

To do:

- Remove localStorage as the source of truth for bookmarks and hidden postings in production mode.
- **overseer feedback:** Remove localStorage from the production app once login exists. Keep the current anonymous demo behavior live at `demo.jobcron.app` for backward compatibility even after the production version launches at `jobcron.app`.
- Make profile state user-scoped.
- Make bookmarks user-scoped.
- Make hidden postings user-scoped.
- **overseer feedback:** Choose the easiest maintainable score ownership model. The likely implementation is user-scoped scores keyed by `user_id`, `posting_id`, and `profile_hash`, because that is explicit and prepares the schema for Milestone B without requiring clever lookup rules.
  - **overseer feedback:** This is also the most easily scalable option for the expected path. A composite key on `(user_id, posting_id, profile_hash)` keeps reads simple, lets each user's profile produce independent scores, and supports targeted indexes for user pages. If score volume becomes large later, PostgreSQL can still handle the table with normal indexes first, then partition by `user_id` or score date only if measurement proves it is needed.
- Update `/bookmarks`, `/hidden`, daily briefing, and archive views to read the logged-in user's state.

For Milestone A, this still means adding `user_id` even though there is only one account. That keeps the schema ready for Milestone B.

### 4. Automated daily scrape

The app should scrape without a manual `curl`.

To do:

- Add an internal scheduler that runs once per day in Korea Standard Time.
- Keep the existing manual scrape endpoint as an operator fallback.
- Reuse the existing scrape singleflight lock so scheduled and manual scrape cannot overlap.
- Store scrape run history in `scrape_runs`.
- Record started time, finished time, status, source counts, new posting count, and error summaries.
- Add an admin status page or endpoint showing last scrape, next scrape, and last failure.
- Add environment config for schedule time, for example `JOBSCRAPER_DAILY_SCRAPE_TIME=08:00`.
- Add a way to disable the scheduler in local development and tests.

AI scoring during automated scrape.

The available choices are:

- Option 1: scheduled scrape does no fresh AI work. It only scrapes and uses cached AI results. Lowest cost and safest, but new postings may lack AI chips until a manual AI run happens.
- Option 2: scheduled scrape runs AI only for new postings, bounded by strict daily token caps. Best product experience, but requires real Anthropic spend and careful failure handling.
- Option 3: scheduled scrape runs normal scrape first, then queues AI scoring as a separate background job. Most production-friendly, but more moving parts.

**overseer feedback:** Decision for Milestone A: default to Option 1. Scheduled scrape should not run fresh AI by default. Instead, the logged-in owner gets a manual AI run button. Option 2 should be an explicit opt-in setting: scheduled scrape can run AI for new postings only after the user enables it. The front page should show a prominent tip when scheduled AI is off, so the user knows they can enable automatic AI scoring.

**overseer feedback:** Initial AI budget defaults should be USD $10/month estimated budget, USD $0.50/day hard cap, and a per-run cap equivalent to roughly USD $0.30. The UI should show these as estimates, because actual cost depends on provider pricing, selected model, number of new postings, and prompt size. If the user hits a cap, show a clear notification explaining that AI scoring paused and that the limits can be changed in settings.

### 5. Production deployment

Move from demo deployment to production deployment.

To do:

- Add PostgreSQL to deployment.
- **overseer feedback:** Use AWS RDS PostgreSQL for Milestone A.
- Keep Docker Compose PostgreSQL documented only as a rejected lower-cost alternative.
- **overseer feedback:** Rough cost estimate, assuming a small single-AZ PostgreSQL deployment:
  - RDS PostgreSQL: a `db.t4g.micro`-class instance is roughly USD $0.016/hour in common US regions, or about USD $12/month before storage, backup, and regional differences. With small storage and backup overhead, plan for roughly USD $15-25/month unless AWS Free Tier applies. Final numbers should be checked against the AWS RDS pricing page before deployment: <https://aws.amazon.com/rds/postgresql/pricing/>.
  - Docker Compose PostgreSQL on the existing EC2 instance: no extra database compute charge. The main incremental cost is EBS storage and backups, likely a few dollars/month at this project size.
  - For about 20 daily active users in the first month, either option is technically fine. For about 200 daily active users later, RDS becomes more attractive because backups, recovery, monitoring, and database resource isolation matter more than the small monthly savings.
  - Migration from self-hosted Docker PostgreSQL to RDS later should be moderate, not hard, if Milestone A uses `DATABASE_URL`, standard PostgreSQL migrations, and no local filesystem assumptions. The migration path is `pg_dump` from Docker PostgreSQL, restore into RDS, point `DATABASE_URL` at RDS, run verification, then restart the app. Expect a short planned maintenance window for early scale.
- Add backup process:
  - RDS snapshots for short-term restore points.
    - **overseer feedback:** Do not let snapshots accumulate indefinitely in RDS or on the EBS volume. Add a backup export pipeline that rotates recent server-side backups and sends older backup archives to the MacBook.
    - **overseer feedback:** If the MacBook is closed when a server cron job tries to send a backup, the transfer fails or times out. The safe design is therefore not "server pushes once and deletes." Use a two-sided backup flow: the server creates compressed `pg_dump` files and keeps a short retention window; the MacBook runs a launchd job when it is awake to pull any missing backup archives; the server deletes an archive only after the MacBook-side sync has confirmed receipt or after a deliberately generous retention period. This makes a closed MacBook a delayed backup, not data loss.
  - Scheduled `pg_dump` exports for MacBook archival backups.
- Keep self-hosted PostgreSQL persistent-volume notes only in the rejected-alternative section.
- **overseer feedback:** Update Caddy for production hostnames `jobcron.app` and `www.jobcron.app`, not only `demo.jobcron.app`. Caddy should request certificates for both names and redirect the less-preferred host to the canonical production host.
- Add production `.env` documentation:
  - `DATABASE_URL`
  - `SESSION_SECRET`
  - `JOBSCRAPER_ADMIN_TOKEN`
  - `JOBSCRAPER_DAILY_SCRAPE_TIME`
  - `JOBSCRAPER_SCHEDULER_ENABLED`
  - `JOBSCRAPER_WORKNET_KEY`
  - Anthropic key configuration, depending on the AI decision
- Add a health endpoint that checks app and database connectivity.

### 6. Security hardening

Milestone A is private, but it is still internet-facing.

To do:

- Hash passwords with Argon2id or bcrypt.
- Store session tokens hashed in the database.
- Use secure cookie flags.
- Add CSRF protection.
- Add login rate limiting.
- Make admin scrape auth header-only. Avoid query-token examples.
- Ensure production secrets are not logged.
- Ensure AI keys are never committed, uploaded accidentally, or shown in UI after save.
- Add production startup checks for required secrets.

### 7. Observability and operations

The app should tell us whether it is healthy.

To do:

- Add scrape run history view.
- Add last error summary per source.
- Add database migration version to status.
- Add scheduler status: enabled, next run, last run.
- Add AI status: enabled, provider, model, daily budget used, daily budget cap.
- Add logs that are useful in Docker without being noisy.

Alerting can wait until Milestone B, but the app should expose enough state that a human can diagnose failures quickly.

## Suggested implementation order

1. Add production config and environment validation.
2. Add PostgreSQL support and migrations.
3. Add SQLite-to-PostgreSQL importer.
4. Add `users` and owner account setup.
5. Add cookie session login/logout.
6. User-scope profile, bookmarks, hidden postings, and scores.
7. Replace production localStorage state with server-backed state.
8. Add scheduled daily scrape.
9. Implement manual AI run and opt-in scheduled AI behavior.
10. Add scrape run history and admin status.
11. Update AWS deployment for PostgreSQL and backups.
12. Run full browser QA on login, profile, daily briefing, archive, bookmarks, hidden postings, and admin status.

## Product decisions

Resolved decisions:

1. **overseer feedback:** Scheduled scrape should not run AI automatically by default. The user can enable scheduled AI later through a prominent opt-in setting.
2. **overseer feedback:** Initial AI budget defaults should be set by the app: estimated USD $10/month, USD $0.50/day hard cap, and about USD $0.30 per automated run. When a cap is reached, notify the user and link to settings.
3. Should the first owner account be created by CLI command, environment variables, or first-run setup page?
   - **overseer feedback:** A first-run setup page means a temporary browser page shown only when no owner account exists, allowing the first visitor to create the owner account. For this app, avoid that for Milestone A because a public server briefly exposing first-owner setup is easy to misconfigure. Prefer a CLI command or one-time environment bootstrap instead.
   - **overseer feedback:** Use a CLI command for Milestone A owner account creation.
4. Should anonymous demo mode remain available after production login exists?
   - **overseer feedback:** Yes. Keep `demo.jobcron.app` for backward compatibility after `jobcron.app` launches.
5. Should Milestone B be public signup, invite-only signup, or admin-created users?
   - **overseer feedback:** Public signup.
6. Should Milestone A use RDS PostgreSQL or self-host PostgreSQL in Docker Compose?
   - **overseer feedback:** Use RDS PostgreSQL.

## Acceptance criteria for Milestone A

- The app runs from PostgreSQL in production.
- **overseer feedback:** The production database is AWS RDS PostgreSQL.
- A single owner can log in and log out.
- **overseer feedback:** The first owner account can be created by CLI command.
- Session cookies survive refresh and expire correctly.
- Profile edits persist server-side.
- Bookmarks persist server-side.
- Hidden postings persist server-side.
- Daily scrape runs automatically without manual `curl`.
- Manual scrape remains available to the owner or operator.
- Scrape run history shows success or failure.
- AI behavior during scheduled scrape is explicit and bounded.
- **overseer feedback:** AI budget caps default to USD $10/month, USD $0.50/day, and about USD $0.30 per automated run, with user-facing notifications when a cap is hit.
- Production deploy docs cover database, secrets, scheduler, backups, and recovery.
- Existing demo safeguards still protect public visitor access if demo mode remains enabled.

## Out of scope for Milestone A

- Public signup.
- Password reset emails.
- Teams or shared workspaces.
- Billing.
- Per-user AI billing.
- Admin user management.
- Multiple production app instances.
- Full alerting pipeline.
- **overseer feedback:** A full control dashboard with alarms and operational controls.

These are Milestone B or later.
