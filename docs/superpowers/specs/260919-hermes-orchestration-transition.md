# Hermes Orchestration Transition

**Status:** approved operating specification

## Scope

This specification moves Jobcron implementation work to Hermes Kanban without
expanding authority beyond local repository work. It is public-safe: it records
roles and controls, never credentials, account identifiers, or machine-local
paths.

## Approved roster and routing

| Role | Profiles | Responsibility |
| --- | --- | --- |
| Coordinator | `default` | Creates and prioritizes Kanban work; settles decisions and human gates. |
| Reviewer | `reviewer-sol` | Independently reviews every implementation card before approval. |
| Terra implementation | `worker-terra-1`, `worker-terra-2` | Executes assigned implementation cards. |
| Luna implementation | `worker-luna-1`, `worker-luna-2` | Executes assigned implementation cards. |

The dispatcher, not a worker, selects the assigned Terra or Luna profile.
Terra and Luna implement only the card they receive; neither self-routes work
nor silently takes a sibling's scope. Jobcron code and documentation cards can
be assigned to either implementation lane. All four implementation profiles
run Ponytail in `full` mode for every task, including Jobcron work.

## Authorization and workspace boundary

A worker may start implementation only when its Kanban card contains both:

1. an approved specification or plan that defines the requested change; and
2. explicit authorization to run autonomously.

If either is absent, or a required product decision is unresolved, the worker
blocks the card for human input rather than inferring authority. Each card uses
an isolated worktree. Before editing, the worker records a clean or understood
baseline with the current branch, status, and rollback commit. It preserves a
verified rollback point before any change and, after a successful local commit,
records the resulting commit as the next rollback point.

Workers may commit their verified local card changes. Integration authority is
local-only: a coordinator or explicitly authorized local integrator may merge
reviewed work locally after verifying the target state. A local merge is not
permission to publish anything.

## Required execution and review flow

1. Read the card, repository instructions, approved spec or plan, and relevant
   existing code before editing.
2. Work only in the card's isolated worktree and keep the diff within card
   scope.
3. Run the focused checks required by the repository and inspect the resulting
   diff. A passing check and clean intended diff are required before commit.
4. Commit the verified change locally with an accurate message.
5. Request mandatory independent review from `reviewer-sol` through Kanban.
   The reviewer approves, returns concrete changes, or blocks only for an
   external decision or dependency.
6. Do not treat review approval as publication authority. Complete the local
   integration step only when it is separately assigned and verified.

## Human-only gates

The following actions require a human's explicit, current authorization. A
worker must not perform them merely because related local work was approved:

- push, create or update a pull request, or otherwise publish Git state;
- deploy, modify production or cloud resources, or run a production migration;
- obtain, enter, rotate, reveal, or transmit credentials;
- send messages, create tickets, change third-party records, or make any other
  external write.

## Telegram milestone protocol

Kanban is the workflow record. When Telegram delivery is configured, it mirrors
only these lifecycle milestones: card accepted, human gate or blocker raised,
review requested, review returned or approved, and verified local integration
outcome. Messages are concise and public-safe: they identify the card and
state, never secrets, machine-local paths, raw logs, or unreviewed claims.
Telegram is notification-only; a reply there does not alter authorization until
it is recorded on the Kanban card.

## Cross-session sources of truth

In descending operational priority, workers use:

1. the current Kanban card, its parent handoffs, comments, and lifecycle state;
2. the approved specification or plan linked by that card;
3. repository instructions and tracked documentation;
4. the local Git branch, status, commits, and verification output.

Telegram notifications, chat recollections, and transient terminal output are
not sources of truth. Conflicts are recorded on the card and sent to a human
rather than resolved by guessing.

## Jobcron execution constraint

Autonomous Jobcron runs must not open a user's browser. Invoke the application
with `--no-open`; browser smoke checks, when required, use a headless browser
against the local service. No autonomous task may use a browser session to
perform an external write.

Jobcron v1.x does not permit browser-driven scraping or browser-fingerprint
bypass. Autonomous implementation must not add or use Playwright, chromedp, or
similar browser automation for scraping; the headless local UI-smoke exception
above does not change that product constraint.

## Current transition checklist

- [x] Approved roster and Terra/Luna dispatch routing are recorded.
- [x] Ponytail `full` is required for all four implementation profiles.
- [x] Approved-spec and autonomous-run gates are explicit.
- [x] Isolated worktrees, rollback points, local-only integration, and
  `reviewer-sol` review are required.
- [x] Human-only external-action gates and Telegram notification limits are
  recorded.
- [x] Jobcron's no-browser-driven-scraping and `--no-open` rules are carried
  into the workflow.
- [ ] Apply this contract to each newly created implementation card and revise
  it only through an approved, reviewed documentation change.

## Acceptance and rollback

The transition is accepted when a newly assigned implementation card can trace
its authority, isolated worktree, baseline rollback commit, verification,
local commit, and `reviewer-sol` review through Kanban without relying on chat
or Telegram. If any control fails, stop the card, return the worktree to its
recorded rollback commit, and record the failure and recovery on the card.
Changes to this specification follow the same local verification and mandatory
review flow; a human may revert the documentation commit to restore the prior
workflow record.
