# Multi-User Account Expansion

- **Date:** 2026-07-15
- **Last updated:** 2026-07-22
- **Status:** Active; product decisions approved, implementation plan pending
- **Depends on:**
  [`260714-postgresql-local-convergence-user-ai-credentials.md`](260714-postgresql-local-convergence-user-ai-credentials.md)
- **Related:**
  [`260719-terraform-aws-foundation-and-cloudflare-ingress-automation.md`](260719-terraform-aws-foundation-and-cloudflare-ingress-automation.md)

## Purpose

Prepare Jobcron for its first multi-user demo cohort while the overseer studies
AWS, IAM, Terraform, and the production deployment flow. Application work may
proceed locally against PostgreSQL; AWS resource changes and production cutover
remain separately reviewed, human-gated work.

The first users are the overseer's SSAFY classmates. They receive self-service
account creation behind one shared cohort access code. Removing that gate and
opening signup to the general public is a later security and operations milestone.

## Current Repository Baseline

The PostgreSQL convergence dependency has completed its repository-owned work.
The remaining dependency work is the external production rollout, not an
application prerequisite for this milestone.

The current repository already provides:

- PostgreSQL as the only writable application database;
- user-scoped profiles, saved-job state, AI scores, usage, and credentials;
- explicit user identity for AI runtime and credential resolution;
- two-user isolation coverage;
- Argon2id password hashing, hashed session tokens, CSRF protection, secure
  production cookies, and login throttling;
- encrypted, per-user Anthropic credential storage; and
- foreign-key cascades for private user-owned state.

The owner-only creation and scheduler paths remain incompatible with multiple
users and are part of this milestone.

## Locked Product Decisions

1. Accounts are independent. There are no organizations, teams, application
   roles, or social-login providers.
2. The cohort gets self-service email-and-password signup protected by one
   server-side access code. Signup is not yet open to the general public.
3. Passwords reuse the existing Argon2id implementation. Password recovery and
   email verification are deferred; the operator handles cohort resets through
   the user-management CLI.
4. AI access is bring-your-own-key. The UI supports Anthropic only. OpenAI,
   Gemini, and shared Jobcron-funded usage are out of scope.
5. The server performs one global scrape every day at 05:00 KST, then scores the
   shared postings separately for each user.
6. Per-user scoring runs sequentially at first. Concurrency or a job queue is
   added only if measured run duration requires it.
7. A missing, invalid, or exhausted AI credential affects only that user.
   Deterministic scoring continues and other users still complete.
8. Deleting an account removes its sessions, profile, saved-job state, AI
   scores, usage, contextual validations, import records, and credentials.
   Global postings, public `ai_extractions`, and global scrape history remain.
9. Cohort account deletion is operator-assisted. Truly public signup requires
   self-service deletion before launch.

## Milestone Scope

### Account creation

- Replace owner-only storage assumptions with a general `CreateUser` path while
  preserving the owner bootstrap command for deployment.
- Canonicalize email identity consistently at creation, login, reset, and
  deletion. Store and compare the trimmed lowercase address.
- Keep the database uniqueness constraint as the final duplicate-account guard.
- Add signup pages using the existing templates, CSRF protection, Argon2id
  hashing, session creation, and secure cookie settings.
- Require password confirmation, a minimum of 15 characters, and support long
  passphrases without composition rules.
- Return an enumeration-safe response for duplicate email addresses rather than
  revealing whether an account already exists.
- Rate-limit signup attempts by client address and normalized email.
- Read the cohort gate from `JOBCRON_SIGNUP_ACCESS_CODE`. Never store it in the
  repository, database, URL, logs, or rendered page. Signup fails closed when it
  is unset, and comparisons are constant-time.
- Sign the user in after successful account creation and direct them to profile
  setup.
- Do not add application roles. Operator-only actions remain CLI operations.

### Cohort account lifecycle

- Generalize the existing password-reset CLI to select exactly one user by
  normalized email.
- Add an operator CLI command that deletes exactly one user after explicit
  confirmation.
- Revoke all sessions as part of account deletion through the existing foreign
  key cascade.
- Verify that deleting a user removes every private row but preserves global
  posting facts and scrape history.
- State clearly in the signup UI that the cohort release does not verify email
  ownership and that password reset or deletion requires contacting the operator.

### AI credentials

- Add an authenticated Anthropic delete-key action with confirmation.
- Keep blank key input as “preserve the existing key.”
- Keep create, replace, read, and delete operations scoped to the authenticated
  user's Anthropic row.
- Correct the profile copy that still describes a local `0600` key file. In the
  hosted app, the credential is encrypted before storage in PostgreSQL.
- Do not add provider abstractions or UI for providers not in this milestone.

### Scheduled work

- Align the application default and production configuration on 05:00 KST.
- Replace `SoleOwnerUserID` in scheduled work. Multiple users must no longer make
  the scheduled run ambiguous or cause it to be skipped.
- Fetch, normalize, deduplicate, upsert, and extract global posting facts once.
- Enumerate current users and perform profile-dependent scoring sequentially.
  Users without a saved profile are skipped until they finish setup.
- Resolve each user's AI runtime, credential, and usage budget independently.
- Record a bounded per-user failure summary without logging credentials or
  allowing one user's error to abort the remaining users.
- Keep the existing single global scrape lock. Replace it only if production
  measurements show that it prevents required work.

### Isolation

- Every authenticated profile, bookmark, hidden-job, rerate, rescore, cache,
  usage, and credential path continues to receive an explicit session `userID`.
- Users cannot read, overwrite, prune, or debit one another's state.
- `ai_extractions` remains global only while it contains public posting facts and
  no profile, credential, or account data.
- Global scrape work never selects an arbitrary user for paid AI execution.

## Truly Public Signup Follow-Up

The cohort access code is a deliberate boundary, not sufficient protection for
open internet signup. The following work is required before removing it.

### Prove email ownership and recovery

- Add transactional email delivery and verify each address with a signed,
  single-use, expiring token before activating the account.
- Add resend limits, generic responses, and safe token replacement so these
  endpoints cannot enumerate accounts or generate unbounded email.
- Add self-service password reset using the same single-use token properties.
- Configure the sending domain, SPF, DKIM, and DMARC, and handle delivery
  failures, bounces, and provider complaints.

### Resist automated abuse

- Remove the cohort code only after signup and recovery endpoints have both
  per-address and per-network throttling.
- Add a privacy-preserving bot challenge such as Cloudflare Turnstile when
  signup is opened publicly.
- Bound expensive work per account. A new user cannot trigger unlimited scraper,
  scoring, AI, email, or storage consumption.
- Add an operator mechanism to disable an abusive account without introducing
  application roles or a web administration console.

### Complete self-service account lifecycle

- Let an authenticated user change their password after confirming the current
  password, then revoke their other sessions.
- Let a user permanently delete their account after re-authentication and an
  explicit destructive confirmation.
- Publish clear privacy, retention, deletion, and acceptable-use terms before
  collecting accounts from the general public.
- Provide a support path for lost access, disputed email ownership, and deletion
  requests without exposing whether an unrelated account exists.

### Harden public operations

- Monitor signup, verification, recovery, throttling, delivery failures, and
  account deletion without logging passwords, tokens, API keys, or unnecessary
  personal data.
- Exercise enumeration, replay, expired-token, duplicate-email, rate-limit,
  session-revocation, and deletion failure paths.
- Store email-provider, Turnstile, and signup secrets in the production secret
  store and inject them through the reviewed Terraform and runtime configuration.
- Grant the application only the IAM permissions required to read its runtime
  secrets and use the selected email service.
- Verify the complete public journey in a real browser: signup, email
  verification, login, password reset, credential management, and account
  deletion.

Public signup is ready only when an unknown internet user can complete that
journey safely without operator intervention and the operator can contain abuse
without editing the database manually.

## Verification Requirements

- Run the complete Go test suite and the PostgreSQL integration suite.
- Add storage tests for normalized email uniqueness, concurrent duplicate
  creation, password reset targeting, and account-deletion cascades.
- Add server tests for access-code failure, signup throttling, enumeration-safe
  duplicate handling, CSRF, auto-login, and unauthenticated access.
- Add scheduler tests with at least two users, distinct profiles, one broken AI
  key, one missing profile, and one successful user.
- Verify in a real browser that two accounts cannot see or change one another's
  profile, saved jobs, hidden jobs, scores, usage, or AI credential.
- Walk signup, login, profile setup, key replacement, key deletion, logout, and
  failure paths on desktop and mobile viewports with no console errors.
- Update `docs/architecture.md`, deployment documentation, and the active
  Terraform specification with the final runtime-secret interface.

## Non-Goals

- Organizations, teams, invitations tied to individual email addresses, or
  application roles.
- Social login, multi-factor authentication, or public password recovery in the
  cohort milestone.
- OpenAI, Gemini, shared funded AI usage, or provider-selection abstractions.
- Per-user scraping schedules, duplicate source fetches, concurrent fan-out,
  queues, workers, or distributed scheduling.
- Applying Terraform, modifying live AWS resources, changing DNS, or performing
  the production cutover as part of this application milestone.

## Deployment And Learning Gate

The application milestone may be implemented and verified locally before the
overseer finishes the AWS and IaC study path. It must not silently broaden into a
production infrastructure change.

Before production deployment, the overseer reviews and can explain:

- the Terraform plan and which AWS resources it creates, changes, or destroys;
- the IAM principal and least-privilege permissions used for planning, applying,
  deployment, and runtime secret access;
- Terraform state storage, locking, backup, and recovery;
- how `JOBCRON_SIGNUP_ACCESS_CODE` and existing secrets reach the container
  without entering Git or Terraform state as plaintext;
- the database migration, rollback, credential-rotation, and account bootstrap
  procedures; and
- the expected recurring AWS and third-party costs.

Only then does the separate production deployment plan proceed.
