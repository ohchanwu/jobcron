# Gemini Pacing and Quota Classification — Narrow Follow-Up

Created: 2026-10-09. Updated: 2026-10-12. Status: narrow Slice 1 design under the
independently design-ready, owner design-approved campaign. **IMPLEMENTATION ON
HOLD; no application coding or live model calls authorized.** The owner requested
implementation-plan documentation only, not execution.
Companion: [experiment and implementation plan](../plans/261009-gemini-pacing-and-quota-classification-plan.md).
Original design baseline: `4701eba23989b2dc4eac93ea12f1896ea16b6066`.
Current discovery baseline: `095bc775610656f3cbe74a076c5be495c30d22b5`.
This retains Slice 1's narrow contract under the
[integrated campaign](261012-ai-rating-live-updates-campaign.md), not a parallel
payload campaign or approval to implement it.

## Purpose and placement

Test whether conservative Gemini request-start pacing reduces transient provider
failures, and explain quota errors without conflating per-minute and daily limits.
This is a narrow follow-up to the implemented
[AI-rating contract](../decisions/261009-ai-rating-and-release-contract.md), not a
rewrite of the completed release spec. The accepted
[app-only runner](../decisions/261012-app-only-runner-local-acceptance.md) is now
locally accepted. This proposed Gemini payload comes before the campaign's live
UI slice; a possible single release follows both independent local acceptances.
It is not implementation in the completed runner card. UI acceptance need not
wait for this proposal's separately gated live experiment.

Keep the current outcome/cache/evidence rules in
[re-rate notes](../../internal/server/RERATE_NOTES.md). A failed analysis card is
not proof of bad model judgment; rejection of evidence and valid no-signal results
are different outcomes. No promise of a particular chip yield or model superiority.

## Grounded problem and uncertainty

At the baseline, `SuggestedRateLimit` returns uniform 1.2-second start spacing.
`providerFailureMessage` checks generic `QuotaFailure` / quota text before
`RetryInfo`. Public Gemini examples show minute limits can carry both, so the UI
usage/quota message does **not** establish daily exhaustion. The owner's raw
structured error, active model/project allowance and complete per-run breakdown
are not available to this design. Do not infer them from a screenshot or public
rate tables. Quotas apply across a project, not independently per API key.

Official references, consulted 2026-10-08:

- [Rate limits](https://ai.google.dev/gemini-api/docs/rate-limits)
- [Troubleshooting](https://ai.google.dev/gemini-api/docs/troubleshooting)
- [API errors](https://ai.google.dev/gemini-api/docs/api-errors)
- [OpenAI compatibility](https://ai.google.dev/gemini-api/docs/openai)

Published limits/examples are context; the exact active allowance must come from
a separately authorized authoritative observation. Public text is not an assertion
about this owner's model, tier, usage or failures.

## Proposed code scope

### Provider-specific request-start spacing

- Experimental Gemini spacing: **10 seconds minimum between request starts**;
  Anthropic and other providers retain their existing 1.2-second behavior.
- Apply consistently to Gemini paths using the existing shared limiter, including
  extraction and Stage-2 calls, while preserving worker latency overlap. Test the
  limiter itself, not only the returned duration. Do not serialize completion or
  change pool size to achieve spacing.
- State the real limiter scope. One process/operation cannot promise a project-wide
  quota guarantee across unrelated clients. Do not add a distributed limiter.
- Preserve cancellation, budgets, singleflight and cache-hit behavior. No new
  automatic retry/backoff loop, concurrency bypass or rate-limit exemption.
- Ten seconds is a conservative trial, not a verified quota-safe setting. Token
  limits, daily exhaustion and other project traffic can still cause failure.
- Do not change goals, prompt, model allowlist, score/extraction version or cache
  identities just to test pacing. Existing successful results must remain hits.

### Structured, bounded quota classification

Prefer supported structured status/details, explicit `quotaMetric`/`quotaId` scope
and well-formed `RetryInfo`/`Retry-After` over generic substring labels. Handle
native and supported OpenAI-compatible envelopes; unsupported/malformed shapes
fall back safely rather than inventing a diagnosis.

Required categories and calm copy intent:

- **Minute/request or minute/token throttle:** explicit minute scope, no persistent
  quota/billing contradiction. Explain waiting may help; no exact safe waiting
  time or retry promise without trustworthy evidence.
- **Daily limit:** explicit daily scope. Explain short pacing cannot restore it;
  check quota/usage. `RetryInfo` must not downgrade this to a transient minute case.
- **Mixed limits:** minute plus daily, or conflicting recognized scopes. Preserve
  the persistent restriction and uncertainty; do not claim waiting alone resolves it.
- **Billing/account restriction:** explicit applicable signal, distinct from RPM.
- **Unknown quota/exhaustion:** only generic `QuotaFailure`, generic quota text,
  `RESOURCE_EXHAUSTED` or an unscoped 429. Say the provider limited the request and
  its cause/scope is not established. Generic 429 is not automatically RPM or RPD.
- Preserve existing key/model/region/transport/parsing distinctions. 5xx/timeout,
  malformed JSON, evidence rejection and valid empty success are not quota proof.

Use a bounded allowlist of parsed fields for diagnostics. Accept known scope
categories and validated bounded retry delay, not raw provider text/URLs, key or
project identifiers, JD/goals, credential strings, model responses or stack traces.
Do not introduce persistence/schema changes solely for diagnostics. Classification
is explanatory; it does not create retry authority or successful cache rows.
Generic Gemini scope handling must not rewrite another provider's explicit
rate/billing error contract. Include provider-isolation regression tests.

Update progress/timing copy only where affected. Do not leave a fixed coffee-break
or duration promise that understates the intentionally slower run. Counts retain
processed/failed/rejected/fresh-success distinctions. No unrelated UI redesign.

## Local acceptance

Tests cover explicit minute request and token metrics, daily metrics with and
without retry hints, mixed/conflicting scopes, generic quota text, unknown 429,
malformed details/headers/envelopes, account restrictions and non-quota errors.
Verify bounded parsing, output redaction, existing calm Korean copy, and unchanged
outcome/cache semantics. Test actual minimum start spacing and cancellation using
controlled time/transport; no real sleeps or live provider requests in unit tests.

Check provider independence, extraction/Stage-2 wiring and no additional requests
on cache hit, poll/reload/reconnect. Keep prompts/model/identity and complete
migration manifests unchanged; no schema migration. Run focused AI/server tests,
full clean-env build/test/vet/format gates, owned browser smoke for affected copy,
and exact-candidate independent same-card review. Only then is the payload a
candidate for a separately authorized app-only release.

## Bounded live experiment — separate authority

Proposed batch: **up to 20** named eligible Stage-2 cache misses, one frozen user/
goal/model/prompt/rules tuple, one deliberate run, through accepted application
code. Identify cache misses by nonmutating preparation; do not delete successes,
change goals or rotate versions to manufacture misses. No Stage-1 scrape, scheduled
analysis, parallel owner run, automatic retry or paid Haiku comparison is included.
Use supported existing surface/cap controls or an approved owned fixture; this
draft does not claim an arbitrary-posting-ID endpoint exists. Any temporary cap
change/restoration or fixture/data preparation must be named in its own approval.
Before the live gate, prove that the selected route enforces the submission bound
and stop/admission control. If it cannot, do not run or invent a cancellation
guarantee; seek a narrow owner disposition rather than silently adding tooling.

Before execution, bind exact target, provider/model/project quota observation if
available, selected posting IDs/content identity in private custody, allowed
requests, per-run/daily token ceilings, any spend ceiling and ambiguity stop rules
in a separate owner approval. Existing configured limits remain ceilings, not a
fresh allowance. Optional paid comparison needs its own approval.

Record actual attempted/submitted/completed/never-reached counts, classified quota
scopes, provider/parse/storage failures, rejected proposals, rated and no-signal
successes, fresh-cache counts, reported usage and duration. Separate unknown
billability from zero usage. Stop new admissions after an observed rate/quota
error or ambiguous submission; in-flight calls may finish and must be reconciled.
Cancellation cannot prove the provider did not receive a call. No blind repeat.

A successful slow batch supports feasibility under its observed conditions, not
causal proof versus the old pacing or better model judgment. Use existing prior
batch evidence only when genuinely comparable; label a confounded comparison.
A new baseline arm or Haiku arm is a separate authorized experiment. No old-pacing
live arm is required by this draft.

## Exclusions

No provider switch, prompt/evidence weakening, success-cache deletion, higher caps,
new paid retry, distributed rate controller, model quality benchmark, production
error harvesting or credential acquisition under this document. The completed
runner's local card did not include these changes or live calls. The campaign's
separate live-update scope does not widen this pacing/experiment contract.
