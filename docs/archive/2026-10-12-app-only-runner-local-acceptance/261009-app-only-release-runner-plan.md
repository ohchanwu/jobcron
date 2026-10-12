# App-Only Release Runner — Phased Plan

Created: 2026-10-09. Closed: 2026-10-12. Status: local Phases 0–2 completed and
independently accepted on `t_6e112520`; not an active implementation plan.
Governing [historical specification](261009-app-only-release-runner.md).
Original baseline: `4701eba23989b2dc4eac93ea12f1896ea16b6066`.
Accepted integrated source: `095bc775610656f3cbe74a076c5be495c30d22b5`, tree
`3a963914ba415838a11412a122458111cd144a15`; worker 166 / reviewer 167.
See [acceptance and limits](../../decisions/261012-app-only-runner-local-acceptance.md).
Phases 3–5 below are historical conditional proposals, NOT executed milestones.
Their current placement is the [new campaign plan](../../plans/261012-ai-rating-live-updates-campaign-plan.md).
Do not reopen the runner or infer production/live authority from this archive.

The pinned implementation baseline is
`37b589bc671fe33e9928c492a1a5d29170a29484`. The native Python CLI and owned
fixture are documented in [actual invocation/custody](../../../scripts/app_release_fixture/README.md).
Real local current -> candidate -> compatible recovery -> candidate checks,
authenticated HTTPS/logout, seeded post-change data retention and lost-reply
readback ran; exact local candidate review is now accepted. Historical crash and
fixture correction evidence is retained, not reset. No hosted publication,
provider/IAM read, production deployment or Gemini experiment was performed.
Review153 requested consolidated safety corrections, not approval. Same-card
correction retains the rejected candidate/evidence and adds permanent regressions
and real failed/stopped-candidate recovery, with exact resume identities,
mutation-boundary submission accounting and bounded non-disk output custody.
These were historical review findings; final reviewer 167 approved the corrections.

Review155 rejected inferred-poststate continuation and public/secret custody
layout. That approach and both candidates remain historical. Independent
prospective review161 found the supported Compose plus explicit attended
reconciliation alternative eligible; controller activation allows local run163,
not code approval. Replacement now records delivered-outcome/postimage pairs,
checks complete preimages on every transition, refuses unbound lost replies and
uses separate settled-exclusive operation-scoped attended dispositions when
needed. Same-lock failed/stopped/identified partial recovery preserves unknown
history and post-change writes. Public disk configuration and actual secret tmpfs
custody are distinct. Complete regressions and new owned native rehearsal
were required for the corrected candidate; no prospective review reset the budget
or authorized production effects. Final same-card acceptance is now complete.

Review164 requested the first consolidated approach2 correction, not approval.
Controller reassessment authorizes the same strategy's omitted pending-pair ->
later failed/stopped/restarted SAME container -> named same-lock compatible
recovery transition, and completed-receipt digest/semantic acceptance binding.
Original pending evidence remains byte-preserved; failed release is not accepted.
Receipt-ready publication windows refuse missing/conflicting historical proof
rather than regenerating it. Permanent offline transition/receipt negatives and
actual owned interrupted-pair/stopped-app recovery supplement the original
complete checks. Reviewer 167 accepted the corrected exact candidate and evidence.

## Phase 0 — documentation and readiness

- Reconcile completed launch/release status from accepted evidence; archive old
  execution documents and retain concise [current contracts](../../decisions/261009-ai-rating-and-release-contract.md).
- Review the runner scope and the separate
  [Gemini experiment proposal](../../specs/261009-gemini-pacing-and-quota-classification.md).
  Check that existing native routes can implement the stated lane without a new
  orchestration platform or weakening a protected gate.
- Run independent documentation/design-readiness review automatically as part of
  documentation closeout, and resolve findings within the authorized docs scope.
  Do not ask the owner to reauthorize that routine review. Obtain owner disposition
  of the final scope afterward. Neither these docs nor readiness review permits
  coding. **Stop here until the owner separately authorizes local implementation.**

## Phase 1 — future local implementation, only after authorization

Before dispatch, inspect existing Jobcron cards; do not recreate prior graphs.
Load orchestration policy and verify exact profiles, board, repository and forced
skills. Implementation may be assigned only to `jobcron-worker`,
`jobcron-worker-glm-1` or `jobcron-worker-glm-2`; choose and durably record one exact
profile, not an unresolved pool. Same-card review is `jobcron-reviewer` only.

The native card binds board `jobcron`, exact profile, repository, isolated worktree,
branch, full approved baseline, allowed files, test commands, owned fixture routes
and `local-only` completion contract. Public docs use `<REPOSITORY_ROOT>` and
`<WORKTREE>`; exact private paths belong to that card. A worker may not acquire
production credentials, read/mutate cloud state, push, publish or deploy.

Implement the smallest runnable entry point fitting existing tools:

1. Parse the release/approval input, validate independent provenance and scope,
   and implement nonmutating prepare plus explicit native-operation boundaries.
2. Implement lock/checkpoint, bounded submit/observe/reconcile semantics and
   file-input custody. Cover refusal and a lost reply before extending automation.
3. Connect supported pre-pull/recovery-point, image-only persistence and app-only
   replacement routes. Keep the remote adapter real, but exercise it only against
   owned local routes during this card. No self-approving production adapter test.
4. Connect state/readiness/journey/ledger/protected-state observation, the required
   stability gate, sanitized receipt and explicitly authorized recovery.
5. Update actual operator invocation docs only as commands become runnable.

Use vertical test-first changes rather than a large stub backend. Select the
language/location by fit with current runtime/operator code; do not introduce a
framework or dependency merely to satisfy this plan. Surface a real custody,
identity or product decision before widening the design.

## Phase 2 — owned rehearsal and same-card review

Allocate unique disposable Compose resources, exact owned database/volumes and
nonconflicting loopback ports. Never touch the shared local PostgreSQL or user
state. Preview uses `127.0.0.1:17777 --no-open` unless the card allocates another
port. Inventory subprocess input routes before running anything. Do not treat
removing an environment variable as proof a helper cannot read a shared fixture.

Execute the real current -> candidate -> compatible recovery -> candidate
sequence, preserve seeded new writes, verify unchanged proxy container/mounts,
confirm one scheduler and a real app journey. Inject a representative ambiguous
reply locally, reconcile without replay, and verify safe refusal for unknown state.
Retain sanitized local evidence and remove only exact owned resources.

Required future checks:

- Focused runner tests; applicable shell/Python syntax and Linux command shape.
- Clean-env `go build ./cmd/jobcron ./cmd/jobcron-user`, `go test ./...`,
  `go vet ./...`, `gofmt -l .` and indexed-Go formatting inspection.
- Broad processes unset `JOBCRON_TEST_POSTGRES_URL`,
  `JOBCRON_RERATE_BROWSER_PROBE` and `JOBCRON_RERATE_PROBE_OUT`. Targeted optional
  endpoints are allowed only after inspecting the test and binding an owned fixture.
- Browser smoke for changed UI/real journey and documentation/link/secret checks.
  Report nested ignored-worktree format findings honestly; do not repair private
  historical files to obtain an empty listing.

Commit locally only after applicable checks pass. Hand the same card to
`jobcron-reviewer` with exact baseline/candidate, complete diff and observed
rehearsal/check output. A finding returns to the same card; no duplicate review or
helper graph. The controller integrates only the exact approved candidate while
preserving owner edits. Local acceptance does not claim a production rehearsal.

## Phase 3 — first payload, still separate from release authority

Only after runner acceptance/integration, and separate owner coding approval,
implement the small Gemini pacing/classification payload under its
[own plan](../../plans/261009-gemini-pacing-and-quota-classification-plan.md).
Review that exact candidate and all affected regressions. Keep model/prompt/cache
contracts and complete migration manifests unchanged. Runner tooling acceptance
and payload acceptance are independent evidence.

## Phase 4 — conditional publication and deployment

The controller obtains explicit applicable authority for push/CI publication,
credential/provider reads, exact-target selector persistence and app replacement.
Bind exact source/green CI/immutable artifacts and compatible recovery. Reuse
valid prior evidence; reobserve only affected target/preimages before their
writes. A credentials login or an old release grant is not new mutation authority.

Use the implemented runner: prepare -> authorized preflight -> image-only selector
persistence -> app-only replacement -> actual verification -> at least 30-minute
stability -> sanitized receipt. Production recovery is separately named/approved.
No new migrations, DNS, infrastructure or protected-value change is implicit.
Do not mark this phase done on local-fixture evidence or a successful API reply.

## Phase 5 — conditional live Gemini experiment

Only after the separately accepted release and a distinct exact-model/batch/budget
approval, run the bounded experiment in the Gemini plan. No live call is required
to complete the runner or its local implementation card. Complete useful already-
authorized preparation without manufacturing replacement policy artifacts or
reopening the completed launch campaign.

## Completion labels

Docs ready means written and checked, not approved. Design ready requires recorded
review and owner disposition. Runner locally accepted means runnable native
implementation, real owned rehearsal and exact-candidate review. Release verified
requires actual authorized target readback, journey, preservation and observation.
Experiment complete requires actual authorized call evidence; unknown/denied
results remain unknown/denied, not a substitute success.
