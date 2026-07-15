# Multi-User Account Expansion Follow-Up

**Date:** 2026-07-15
**Status:** Deferred; not approved for implementation
**Depends on:**
[`260714-postgresql-local-convergence-user-ai-credentials.md`](260714-postgresql-local-convergence-user-ai-credentials.md)

## Purpose

The PostgreSQL convergence work makes storage and AI execution safe for multiple
user rows while retaining the owner-only account UI. This file records the work
explicitly deferred until Jobcron adds user account creation.

## Prerequisites

- PostgreSQL is the only writable database.
- Profiles, saved-job state, AI scores, usage, and credentials are user-scoped.
- Every AI operation uses one explicit user's runtime and credential.
- Two authenticated sessions pass isolation tests.

## Deferred Work

### Accounts

- Choose invite-only accounts or public signup.
- Add account creation, password recovery, and any approved social-login flow.
- Decide whether accounts are independent or belong to organizations and teams.
- Define owner, administrator, and member roles only if teams are selected.
- Add the account-management UI required by the chosen model.

### AI credentials

- Add a delete-key UI with confirmation.
- Keep blank input as "preserve the existing key."
- Ensure create, replace, and delete operations affect only the authenticated
  user's provider row.
- Decide whether every user brings a key or Jobcron funds shared usage.

### Scheduled work

- Replace the sole-owner scheduler lookup with explicit per-user fan-out.
- Decide whether schedules belong to a user, an organization, or the server.
- Apply token and cost limits separately to each user.
- A missing or broken credential skips paid AI only for the affected user.

### Isolation

- Every authenticated profile, scrape, rerate, rescore, cache, and usage path
  continues to receive an explicit `userID` from the session.
- Users cannot read, overwrite, prune, or debit one another's state.
- `ai_extractions` remains global only while it contains public posting facts and
  no profile or account data.

## Decisions Required Before Planning

1. Invite-only or public signup?
2. Independent accounts or organizations?
3. Which roles exist?
4. User-provided keys only, or shared funded usage?
5. Per-user schedules or one shared scrape with user-specific scoring?
6. What happens to private state and credentials when an account is deleted?

## Re-entry Trigger

Return here after the owner-only alpha is stable and the human is ready to choose
the account and tenancy model. Resolve the six decisions, then turn the selected
scope into an implementation plan.
