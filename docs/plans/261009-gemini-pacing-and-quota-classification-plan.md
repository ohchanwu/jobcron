# Gemini Pacing and Quota Classification — Follow-Up Plan

Created: 2026-10-09. Status: draft, **not approved for execution**.
Governing [specification](../specs/261009-gemini-pacing-and-quota-classification.md).
Prerequisite: independently accepted/integrated
[app-only runner](261009-app-only-release-runner-plan.md).

## 0. Review the documentation, then wait

Review the narrow amendment against current AI outcome/cache and provider error
contracts. Confirm experimental Gemini-only 10-second starts, non-Gemini behavior,
quota-scope distinctions, redaction and honest timing. No provider/model or prompt
change is part of this experiment. Obtain owner disposition of the final scope.
Documentation approval is not permission to start coding or spend tokens.

## 1. Future local implementation, after runner acceptance and explicit approval

Reuse the existing Jobcron pool and same-card `jobcron-reviewer`. Inspect current
cards first. Verify forced skills and record exact board `jobcron`, selected
implementation profile, repository, isolated worktree, branch, full integrated
baseline, allowed files, inspected test/fixture routes and `local-only` completion
contract before dispatch. Exact private paths live on that native card, not here.
Never allocate/dispatch this phase merely because the runner plan names it.

Small vertical slices:

1. Test and implement bounded structured quota parsing/classification against
   sanitized native and compatible envelopes. Unknown and mixed cases must remain
   honest; retry hints cannot erase explicit daily restrictions.
2. Test and implement provider-specific spacing using the existing limiter.
   Verify request starts, cancellation, extraction/Stage-2 wiring and independent
   non-Gemini behavior without real provider calls or lengthy sleeping tests.
3. Integrate calm failure/progress copy and allowlisted diagnostics without new
   schema, raw error disclosure, outcome semantics or cache-identity churn.
4. Verify unchanged prompt/model/evidence inputs and complete migration manifests.
   Update maintained provider/re-rate/operator notes only for behavior actually
   implemented; future experimental results must not be written as established.

Inspect any test subprocess/helper before supplying a URL. Use owned disposable
fixtures only, never the shared local database or production. Broad tests unset
`JOBCRON_TEST_POSTGRES_URL`, `JOBCRON_RERATE_BROWSER_PROBE` and
`JOBCRON_RERATE_PROBE_OUT`; standard preview is loopback `17777 --no-open` unless
an alternative is durably allocated. Live provider integration tests remain off.

## 2. Verify and review the exact payload

- Focused AI parsing/classification and server limiter/SSE/count/cache tests.
- Explicit minute-request, minute-token, daily+RetryInfo, daily without retry,
  mixed/unknown/unscoped 429, malformed/oversized detail and header, compatible
  wrapper, billing/account, transport, parse and non-quota regression fixtures.
- Redaction tests with synthetic leakage canaries; no real errors/keys/JDs in Git.
- Clean-env build of `./cmd/jobcron` and `./cmd/jobcron-user`, full `go test ./...`,
  `go vet ./...`, literal `gofmt -l .` plus indexed-file format verification.
- Owned browser smoke of affected Korean messages, progress/longer-run copy and
  unchanged outcome distinctions. No scrape/re-rate network/provider request.
- Documentation/link checks and secret/manual publication review.

Commit only passing local work, then hand the same card to `jobcron-reviewer` with
exact candidate/baseline, full diff and real output. Return actionable findings to
the same implementation card. The controller integrates the exact approved code.
None of these checks establishes the owner's actual quota or a live improvement.

## 3. Separately gated first runner release

The controller binds green exact-head CI, published immutable artifact/platform,
current target and compatible recovery with unchanged complete manifests.
Publication/push, credentials/live observations, image-selector persistence and
app replacement require explicit applicable authority; no worker may perform them.
Use the accepted runner's prepare/reconcile/app-only flow, protected-state and
journey checks, at least 30-minute stability and sanitized final receipt. Retain
valid earlier evidence where unaffected; do not rerun the completed migration.

A release result is distinct from an experiment result. Do not invoke a paid
analysis merely to prove the deployment passed. Recovery is separately approved,
retains all current writes, and does not silently change model settings.

## 4. Prepare the bounded live run without submitting inference

Under separately approved nonmutating observation scope:

- Freeze exact target/user/goals/model/prompt/rules and choose up to 20 eligible
  existing Stage-2 cache misses; bind private ID/content inventory and exact code.
- Verify a supported surface/cap or approved owned fixture actually limits this
  route to the batch and implements the stated admission/stop behavior. There is
  no assumed arbitrary-ID endpoint. Temporary cap changes/restoration or fixture
  preparation need named authority; unresolved control is a live-phase blocker,
  not permission to add a benchmark platform or claim remote cancellation.
- Prefer a paused/quiet scheduling window that already exists. Any scheduler,
  runtime configuration or credential change is a separately approved effect,
  not an implicit experiment prerequisite. If concurrent activity cannot be
  excluded, report the limitation rather than claim isolated quota conditions.
- Preserve success caches and saved goals/model. Record existing budgets and, if
  authorized/available, the project's current quota/usage scope. No key goes into
  chat or tracked files. Public docs do not substitute for the active allowance.
- Present one grouped live gate naming maximum submissions, selected batch,
  per-run/daily token and spend ceilings, one-run scope and stop/reconcile rules.
  Missing decision/credentials block that live phase, not useful local preparation.

## 5. Execute only the separately approved experiment

Submit one deliberate accepted-app Stage-2 run; no forced cache misses, baseline
arm, Haiku arm or in-run replay. Enforce up to 20 submissions and existing budgets.
Halt new admissions at a rate/quota failure or ambiguous submission, reconcile
in-flight/cache/outcome state before any later owner-authorized run, and do not
call a locally timed-out request definitely unsubmitted.

Record measured start spacing, actual counts/outcomes, known quota scope/retry
hints, reported token usage and elapsed time in owner-only evidence. Keep rated,
neutral no-signal, evidence rejection and provider/parse/storage failure separate.
Missing provider usage is unknown, not a zero-bill claim. No raw errors or personal
posting/goal data in shared reports. Published results contain only needed
sanitized aggregates and limits.

Report feasibility and limitations, not causality or model superiority. Reuse a
valid comparable prior run if one exists; otherwise explicitly state there was
no controlled baseline. Further pacing changes, another live run, a faster arm or
a paid model comparison need a new applicable owner gate after reconciliation.

## Completion and archival

Local payload acceptance, verified deployment and measured live experiment are
three separate milestones. A denied/unavailable provider response is evidence,
not fabricated success. Distill only verified behavior/results into maintained
notes or a concise decision, then archive this execution plan when its authorized
scope is actually complete. Keep the runner and completed prior release separate.
