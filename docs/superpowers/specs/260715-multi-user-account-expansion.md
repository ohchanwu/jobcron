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

The database can safely hold multiple users, but two owner-era entry points still
prevent the app from operating that way. The `jobcron-user create-owner` command
refuses to create a second account, and the daily scheduler refuses to run when
more than one user exists. This milestone removes both restrictions.

## Locked Product Decisions

1. Accounts are independent. There are no organizations, teams, application
   roles, or social-login providers.
2. The cohort gets self-service email-and-password signup protected by one
   server-side access code. Signup is not yet open to the general public.
3. Passwords reuse the existing Argon2id implementation. Cohort users can change
   their password while signed in. Recovery for a forgotten password and email
   verification are deferred; the operator handles cohort recovery through the
   user-management CLI.
4. AI access is bring-your-own-key. The UI supports Anthropic, OpenAI, and
   Gemini. Shared Jobcron-funded usage is out of scope.
5. The server performs one global scrape every day at 05:00 KST, then scores the
   shared postings separately for each user.
6. Per-user scoring runs sequentially at first. Concurrency or a job queue is
   added only if measured run duration requires it.
7. A missing, invalid, or exhausted AI credential affects only that user.
   Deterministic scoring continues and other users still complete.
8. Deleting an account removes its sessions, profile, saved-job state, AI
   scores, usage, contextual validations, import records, and credentials.
   Global postings, public `ai_extractions`, and global scrape history remain.
9. Cohort users can permanently delete their own account after re-authentication
   and explicit confirmation. Operator CLI commands remain available for support.

## Milestone Scope

### Account creation

- Add a general `Store.CreateUser` Go storage method that can create more than
  one account. Keep the existing `jobcron-user create-owner` CLI command for
  creating the first production account before web signup is enabled; this
  one-time setup is the “owner bootstrap.”
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

- Keep the existing server-side session design. Jobcron does not use JWTs: the
  browser receives an opaque random cookie, while PostgreSQL stores only its
  hash. Deleting session rows invalidates those cookies immediately.
- Let an authenticated user change their password after confirming the current
  password, then revoke their other sessions.
- Let an authenticated user permanently delete their account after confirming
  the current password and accepting an explicit destructive confirmation.
- Generalize the existing password-reset CLI to select exactly one user by
  normalized email.
- Add an operator CLI command that deletes exactly one user after explicit
  confirmation.
- Revoke all sessions as part of account deletion through the existing foreign
  key cascade, then expire the browser cookie.
- Verify that deleting a user removes every private row but preserves global
  posting facts and scrape history.
- State clearly in the signup UI that the cohort release does not verify email
  ownership and that forgotten-password recovery requires contacting the operator.

### AI providers and credentials

- Support Anthropic, OpenAI, and Gemini through the existing `ai.Provider`
  interface and shared HTTP-provider chassis. Do not add provider SDK dependencies.
- Give each provider a small static model allowlist and one inexpensive default.
  Keep volatile model identifiers in the code and tests rather than locking them
  into this specification.
- Let each user store at most one encrypted key per provider and select one active
  provider and model for scoring.
- Add authenticated create, replace, and delete-key actions for each supported
  provider, with confirmation before deletion.
- Keep blank key input as “preserve the existing key” for the selected provider.
- Keep create, replace, read, and delete operations scoped to the authenticated
  user's selected provider row.
- Implement OpenAI with its Chat Completions REST contract. Implement Gemini with
  its documented OpenAI-compatible REST endpoint, reusing the OpenAI-shaped body
  and response parser while keeping separate provider identity, authentication,
  base host, model allowlist, errors, usage accounting, and egress pin.
- Keep Anthropic's existing Messages API adapter unchanged except where the
  shared three-provider UI or tests require it.
- Add offline HTTP contract tests for all three providers and opt-in live contract
  tests that run only when the corresponding test API key is present.
- Correct the profile copy that still describes a local `0600` key file. In the
  hosted app, the credential is encrypted before storage in PostgreSQL.
- Do not add providers beyond these three, dynamic provider plugins, or remote
  model discovery.

Implementation references:

- [OpenAI Chat Completions API](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)
- [Gemini OpenAI compatibility](https://ai.google.dev/gemini-api/docs/openai)

### Scheduled work

- Align the application default and production configuration on 05:00 KST.
- Replace `SoleOwnerUserID` in scheduled work. Multiple users must no longer make
  the scheduled run ambiguous or cause it to be skipped.
- Fetch, normalize, deduplicate, and upsert shared postings once without using a
  user's paid AI credential.
- Enumerate current users and perform profile-dependent scoring sequentially.
  Users without a saved profile are skipped until they finish setup.
- Resolve each user's selected provider, AI runtime, credential, and usage budget
  independently. Paid AI calls occur only inside that user's scoring pass and are
  charged only to that user's usage ledger.
- Reuse global cached `ai_extractions` when available. An uncached extraction is
  paid for by the user whose scoring pass requested it; later cache hits make no
  paid call. Revisit user-scoped extraction caches only if this simple cohort
  policy causes a material cost-fairness problem.
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
- The 05:00 shared fetch must not impersonate the first database user or spend
  that user's API key on behalf of everyone. Paid AI starts only inside an
  explicitly identified user's scoring pass with that user's selected key.

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

### Complete public support and policy

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
- Verify that signup with a missing, incorrect, or unconfigured cohort access code
  creates neither an account nor a session. Also test signup throttling,
  enumeration-safe duplicate handling, CSRF, auto-login, and unauthenticated access.
- Verify the 05:00 scheduler calculation, then invoke the scheduled workflow
  directly in tests rather than waiting for the clock. With at least two users,
  assert one shared fetch, separate user scoring, one broken key that does not
  stop another user, and one missing profile that is skipped.
- Add offline provider contract tests for Anthropic, OpenAI, and Gemini, including
  authentication, request and response shape, usage accounting, model validation,
  API errors, and host pinning.
- Verify in a real browser that two accounts cannot see or change one another's
  profile, saved jobs, hidden jobs, scores, usage, or AI credential.
- Walk signup, login, profile setup, provider switching, key replacement, key
  deletion, password change, account deletion, logout, and failure paths on
  desktop and mobile viewports with no console errors.
- Update `docs/architecture.md`, deployment documentation, and the active
  Terraform specification with the final runtime-secret interface.

## Non-Goals

- Organizations, teams, invitations tied to individual email addresses, or
  application roles.
- Social login, multi-factor authentication, or public password recovery in the
  cohort milestone.
- Providers beyond Anthropic, OpenAI, and Gemini; dynamic provider plugins;
  remote model discovery; or shared funded AI usage.
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
