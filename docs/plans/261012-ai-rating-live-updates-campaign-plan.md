# AI Rating and Live Posting Updates — Phased Plan

Created: 2026-10-12. Status: draft, **not approved for execution**.
Governing [campaign specification](../specs/261012-ai-rating-live-updates-campaign.md).
Discovery baseline: `095bc775610656f3cbe74a076c5be495c30d22b5`, tree
`3a963914ba415838a11412a122458111cd144a15`.

## Phase 0 — this documentation-only card

- Inspect actual writers, render helpers, sort/filter/client lifecycle and privacy
  boundaries read-only; settle the observation/projection/ordering proposal.
- Close out the already accepted runner's docs, not its implementation/rehearsal.
  Archive useful completed records; preserve compatibility pointers and maintained
  operator procedures. Record exact local acceptance and its live-proof limits.
- Check Markdown destinations, scope/status/owner-decision coverage, existing
  docs-consuming tests, staged secret/publication safety and local commit.
- Request same-card DESIGN/READINESS review from `jobcron-reviewer` on board
  `jobcron`, with exact candidate/tree/baseline and actual sanitized evidence.
  Resolve routine docs findings on this card. The controller integrates only the
  independently approved local documentation candidate.

Documentation readiness/integration does not approve coding. Stop after that
handoff; do not create or dispatch application cards, restart old runner work,
acquire credentials or perform publication/provider/deployment operations.

## Phase 1 — future Gemini slice, only after explicit coding approval

Use the [Gemini plan](261009-gemini-pacing-and-quota-classification-plan.md) for
its narrow implementation/test details. Preserve the original 2026-10-09 dates.
Implement Gemini-only experimental request-start pacing and bounded quota-scope
classification, with real limiter/cancellation/provider-isolation tests and honest
Korean timing copy. Keep prompts/models/cache/evidence/budget/migration identities.

Before any future dispatch, inspect the board for duplicate work and bind the
exact approved baseline, profile, board `jobcron`, isolated worktree/branch,
allowed files, owned test routes and `local-only` contract. Preflight actual
worker/reviewer skills; do not bypass missing capabilities. Same-card reviewer is
`jobcron-reviewer` only. Workers have no publication or live authority.

Run focused AI/server tests, clean-env full build/test/vet/format, controlled
browser smoke for affected copy and publication/link checks. Review/integrate the
exact passing local payload independently. Slice 1 acceptance proves only local
behavior, not quota safety or quality improvement. A live experiment is optional,
separately authorized and NOT a prerequisite for Phase 2.

## Phase 2 — future live-update slice, after Phase 1 local acceptance and approval

Use small native vertical steps, each tested against controlled stored state:

1. Factor pure score composition from `scoreAll`. Add one transaction-consistent,
   metadata-only user read projection used by initial pages and snapshots. Verify
   fresh/no-signal/stale/rejected/failed and missing-score rendering parity without
   writes, credential decryption, runtime resolution or provider construction.
2. Add post-commit scoped revisions/invalidation and the separate authenticated
   observe/snapshot endpoints, with SELECT-only session middleware (the current
   session resolver writes last_seen_at) and credential-presence metadata.
   Instrument global collection/extraction/detail/
   sweep/dedup and each user's validation/Stage-2/profile/bookmark/mute writers,
   including scheduled noop-emit and detached paths. Extend worker result identity
   and committed-state reporting; no notification after a failed persistence call.
3. Add keyed template fragments and incremental vanilla-JS reconciliation on all
   three surfaces. Keep selected archive sort, group/tie/current membership rules,
   current quick filters and controls. Show new eligible postings immediately as
   not yet AI-analyzed; make all stored terminal outcome distinctions visible.
4. Replace terminal reload with snapshot reconciliation, retaining initiating-run
   owner/epoch/late-response guards. Add other-tab/background updates, visibility/
   BFCache/restart resync and degraded read-only polling, never analysis restart.
5. Implement the read-card threshold move, polite notice, no destination autoopen,
   anchor/focus/disclosure preservation, in-flight control reconciliation and
   reduced-motion movement. Refresh the filter registry/counts on keyed changes;
   do not let search-driven autoopen override a live-move decision.
6. Verify revision/context/generation races, two-user isolation, auth revocation,
   commit/read coherence, listener/queue/read bounds, overflow and complete large
   archive snapshots with ongoing writes (no discard-on-every-dirty-hint starvation).
   If the bounded snapshot strategy cannot converge during the
   existing writer workload, stop for design review, not silent truncation.

The campaign spec's acceptance matrix is mandatory, not just a smoke check. Use
blocking fake providers, injected persistence errors and owned browser fixtures,
never real keys, the shared local database or a provider test endpoint. Confirm
all provider admission targets, denominators, caps, cache-hit behavior, budgets,
automatic rejection skips and later manual retry eligibility remain unchanged.
The observation path is not the new provider queue; current quick filters do not
silently redefine existing admission. Processed/skipped run totals remain frozen
per phase while current-view N/M may change independently.

Run focused server/storage/scoring/web tests, clean-env full `go test ./...`,
`go build ./cmd/jobcron ./cmd/jobcron-user`, `go vet ./...`, literal `gofmt -l .`
and tracked-file formatting verification. Record preexisting ignored historical
format findings without editing private evidence. Browser smoke must cover all
three surfaces, both archive sorts, individual arrival before terminal, another
tab, scheduled/detached writers, focused read-card movement and hidden-tab return.
Standard preview is loopback `17777 --no-open`; allocate a different port durably
only if needed. Unset `JOBCRON_TEST_POSTGRES_URL`, `JOBCRON_RERATE_BROWSER_PROBE`
and `JOBCRON_RERATE_PROBE_OUT` in broad tests; supply optional routes only after
inspection and exact owned-fixture binding. Keep live integrations off.

Commit passing local work; request same-card independent implementation acceptance,
not reuse this documentation review as code approval. Integrate the exact approved
slice. Update maintained behavior docs/architecture only after implementation.

## Phase 3 — possible single release after BOTH local acceptances

A separate controller/owner gate may combine both independently accepted payloads
into one release. No release must occur between the slices merely to run the UI
checks. Bind exact integrated source, exact-head green CI, immutable published
artifact/platform, compatible recovery and unchanged complete migration manifests.
Any manifest/schema/non-image configuration change exits the app-only lane.

Publication/push, credential acquisition, live observations, selector persistence,
app replacement and compatible recovery are separately approved effects. Use the
[accepted runner/operator reference](../../scripts/app_release_fixture/README.md)
and [existing-service safety contract](../decisions/261009-ai-rating-and-release-contract.md):
prepare, authorized reconciliation/preflight, recovery point, image-only selector,
app-only replacement, exact journey/protected-state readback, at least 30-minute
stability and sanitized receipt. Preserve data/new writes and one scheduler;
no migrations, infrastructure, DNS/public cutover or provider calls as smoke checks.
Local fixtures cannot prove production no-swap custody, IAM or public acceptance.

## Phase 4 — optional separately gated Gemini experiment

Use Gemini plan Phases 4/5 only under their own approval. Select at most 20 existing
Stage-2 misses with supported controls, frozen identities/budgets and demonstrable
admission/stop semantics; stop/reconcile quota or ambiguous submissions, including
in-flight work. Do not invent an arbitrary-ID endpoint or remote cancellation.
No old-pacing live arm, forced misses or paid Haiku comparison is authorized.
Record actual submitted/completed/never-reached/outcome/usage/elapsed evidence,
including unknown billability. Report feasibility and limits, not causality or
model superiority. An unavailable experiment does not invalidate local UI acceptance.

## Completion labels

Docs checked, independently design-ready, coding-approved, Slice 1 locally accepted,
Slice 2 locally accepted, release verified and experiment measured are different
milestones. Only observed evidence can advance each one. Distill verified behavior
into maintained references and archive completed plans after their actual scope
finishes. Unexecuted release/experiment phases stay conditional, not marked done.
