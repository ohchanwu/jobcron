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

- Public or invite-only signup.
- Separate profile per user.
- Separate bookmark and hidden state per user.
- User-specific scores, because scoring depends on the user's profile.
- Per-user AI settings or a controlled shared AI budget.
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
- Keep localStorage only for anonymous demo behavior, or remove demo localStorage once production login exists.
- Make profile state user-scoped.
- Make bookmarks user-scoped.
- Make hidden postings user-scoped.
- Make score rows user-scoped or keyed by profile hash plus user identity.
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

Open question: AI scoring during automated scrape.

We need a deliberate decision before implementation:

- Option 1: scheduled scrape does no fresh AI work. It only scrapes and uses cached AI results. Lowest cost and safest, but new postings may lack AI chips until a manual AI run happens.
- Option 2: scheduled scrape runs AI only for new postings, bounded by strict daily token caps. Best product experience, but requires real Anthropic spend and careful failure handling.
- Option 3: scheduled scrape runs normal scrape first, then queues AI scoring as a separate background job. Most production-friendly, but more moving parts.

Recommendation for Milestone A: start with Option 2 if budget caps are reliable and visible in admin status. If we want the safest launch, start with Option 1 and add Option 3 later.

### 5. Production deployment

Move from demo deployment to production deployment.

To do:

- Add PostgreSQL to deployment.
- Prefer AWS RDS PostgreSQL if we want managed backups and fewer server-disk risks.
- Use Docker Compose PostgreSQL on the EC2 instance only if cost is the overriding concern.
- Add backup process:
  - RDS snapshots if using RDS.
  - Scheduled `pg_dump` if self-hosted.
- Add persistent storage for Postgres if self-hosted.
- Update Caddy config only if production hostnames change.
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
9. Decide and implement automated AI scoring behavior.
10. Add scrape run history and admin status.
11. Update AWS deployment for PostgreSQL and backups.
12. Run full browser QA on login, profile, daily briefing, archive, bookmarks, hidden postings, and admin status.

## Product decisions still needed

1. Should Milestone A use RDS PostgreSQL or self-host PostgreSQL in Docker Compose?
2. Should scheduled scrape run AI automatically?
3. If AI runs automatically, what is the daily budget ceiling?
4. Should the first owner account be created by CLI command, environment variables, or first-run setup page?
5. Should anonymous demo mode remain available after production login exists?
6. Should Milestone B be public signup, invite-only signup, or admin-created users?

## Acceptance criteria for Milestone A

- The app runs from PostgreSQL in production.
- A single owner can log in and log out.
- Session cookies survive refresh and expire correctly.
- Profile edits persist server-side.
- Bookmarks persist server-side.
- Hidden postings persist server-side.
- Daily scrape runs automatically without manual `curl`.
- Manual scrape remains available to the owner or operator.
- Scrape run history shows success or failure.
- AI behavior during scheduled scrape is explicit and bounded.
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

These are Milestone B or later.
