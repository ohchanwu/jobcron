# Re-rate (재평가) cache semantics

Stage-2 outcomes preserve the difference between a genuine empty response,
unverified proposals, and a failed call. This contract was implemented and its
release accepted at `4701eba23989b2dc4eac93ea12f1896ea16b6066`. That historical
acceptance is not fresh production evidence or authority for another operation.

The [AI-rating/live-update campaign](../../docs/specs/261012-ai-rating-live-updates-campaign.md)
retains these outcome/cache semantics. Gemini Slice 1 is independently locally
accepted/integrated at `8c369d3cf19a4c966fe4745dfa99738c3fe78367`. The Slice 2 local
candidate now reads saved per-posting results on all three surfaces through
read-only observation and keyed reconciliation; its separate exact-code acceptance
is pending. Terminal events request a current snapshot rather than reloading.
None of this authorizes a live provider experiment, publication or release.

## Rated and genuine no-signal results are cached successes

At least one accepted item is **rated**, including partial acceptance and a
nonempty result whose positive/negative items net to zero. It retains evidence
and a signed AI chip. A structurally valid explicit empty `items` array (or the
supported bare empty array) is **no signal**. It renders the grey
`AI 분석 완료 · 추가로 반영할 내용 없음` card, not a fabricated scored LineItem.

`UpsertAIResult` commits the successful `ai_scores` row and its provenance in
`ai_score_outcomes` atomically. Only committed success contributes to fresh N.
Both successes are cache hits on the next press and spend no tokens under the
same identity. A fresh genuine empty result takes precedence over old scored
evidence. Neither card expresses a negative judgment of the applicant.

## Rejected proposals are not an empty success

Nonempty responses whose proposals all fail item validation, citation checks,
or canonical evidence grouping are **rejected**, not genuine no signal.
The parser preserves proposed-item provenance instead of turning invalid arrays
into a valid empty response. Malformed JSON and missing/wrong-type `items` are
parse failures. Partial acceptance still succeeds.

Rejected work writes one bounded user/posting/goal/ScoreVersion status containing
only state, proposed/accepted counts and time. It writes no successful score row
and adds nothing to fresh N. The amber `AI 분석 · 근거를 확인하지 못했어요` card
explains that no NEW adjustment was applied. A prior supported score may remain
faded as `이전 설정 기준`, alongside the current rejection; it does not count as fresh.
Legacy empty rows remain unknown and are never backfilled with guessed provenance.

A later deliberate manual rerate may retry a rejected row once per run, within
the existing shared cap, pacing and token budgets. Automatic scrape/scheduled
Stage-2 callers skip known same-identity rejections. Reload, polling and reconnect
never initiate a paid retry. A budget skip retains the earlier explanation.

## Failed or never reached — no successful cache row

The analysis did **not** complete: a provider error (timeout, 5xx, 429/529
overload, malformed JSON that fails parsing), a failed cache write, OR the listing
was never reached this press because the user's per-call cap
(`AIRuntime.PerCallCap`) or the
token budget halted first.

None writes a successful `ai_scores` row. A definite provider/parse failure may
persist a bounded failed status; never-attempted/budget-skipped work does not
manufacture an outcome. A failed outcome write is not counted as analysis.
Consequences:

- **Not counted in N** (no row).
- **Retried on the next press** — the cache check misses, so control falls through
  to the spend path, subject to that press's own cap/budget.

This is what makes a second 재평가 press *advance* the counter: it picks up missing
successes, including known rejections eligible for a deliberate retry. Intermittent
`ScoreDelta` failures can recover on a later press.

**Provider errors are not silent.** A `ScoreDelta` error propagates out
of `rerateOne`. `rateStage2`
keeps the first such error and `runRerate` surfaces it: if **every** attempted row
failed (`analyzed == 0`), the SSE terminal is a calm, classified `failed` event —
`providerFailureMessage` maps a 401/403 to "AI 키를 확인해주세요", a 400/404 to
"선택한 모델이 이 제공자와 맞지 않아요" (the mismatched-model trap a provider switch
leaves behind), a 429 to a usage-cap line — instead of a hollow `done` with `0/M`.
A *partial* failure retains individually committed results and emits a
status note, retaining the same classified cause in the terminal and
recovered message. Missing successes alone do not establish a cap/budget skip:
called failures and rejected proposals were processed. Actual uncalled cap/budget
skips, rejected evidence and contextual warnings can coexist and are reported
independently. The cache behavior above is unchanged: a failed row writes no
`ai_scores` row and is retried on the next press.

## Counts and progress

Fresh N counts current-version rated and genuine no-signal successes, not chips,
calls, old-version rows, rejections or failures. The live counter's M is the current
eligible stored-view surface, independent of quick search/source filters. Run phase
processed/target/skipped denominators stay frozen at their original admission
boundaries, including Stage 2 AFTER preparation, validation and rule merging.
Live reordering, membership changes and new arrivals never expand that queue.
Processing progress can advance on failure/rejection;
terminal copy separately reports processed, successfully analyzed, no-signal and
rejected counts, including when contextual-validation warnings also need display.
The numeric progress, provider-dependent wait copy, single observation owner and
interruption recovery remain in place. Hard exclusions never acquire a new AI card.

## When IS a cached-empty listing re-analyzed?

The PostgreSQL Stage-2 cache/outcome key is `(user_id, posting_id, ai_input_hash, ScoreVersion)`.
Legacy SQLite's successful-score cache is sole-user; new outcomes enforce user 1. An
empty-cached listing becomes a fresh miss — and is re-analyzed — only when one
of these rotates:

- **You edit a goal field** (`job_likes` / `job_dislikes` / `short_term_goals` /
  `long_term_goals`). `profile.AIInputHash` hashes only the goal text
  (NFC-normalized), so a goal edit rotates `ai_input_hash`. Weight / MinScore
  tweaks do **not** (by design — they must not churn the AI cache).
- **You switch provider/model or the Stage-2 contract changes.** `ScoreVersion`
  rotates. Stage-2 prompt version 2 does not rotate extraction/dealbreaker identities.

Practical upshot: if the AI is *wrongly* finding nothing on a batch of listings,
re-pressing won't help — reword your goals (give the model different things to
match against) or switch models. A repeat press recovers rejected, failed or
never-run listings, not genuine empty or rated successes. Rendering and startup
never call the provider; upgrading the contract can cause bounded fresh cache
misses and spend on the next existing authorized analysis trigger, not at startup.

Ordinary cross-model pruning retains current-model rows and one prior-model score.
After A → B → C, A's older score may therefore be gone on a return to A. Its old
successful outcome is not an independent cache hit or a current completed card:
successful provenance is readable only while its exact score row still exists.
A definite admitted rejection/failure can replace that orphan provenance, while
an actual successful score (including a legacy row without outcome detail) remains
protected. PostgreSQL score/failure transactions serialize on the posting before
cache admission; user/goal/model keys still decide which result is protected.
This also handles pruning performed through the compatible recovery binary,
without deleting old outcomes or changing the existing stale-score retention.

## Token-accounting footnote

`budget.debit` charges reported usage even when item/citation verification rejects
all proposals or parsing fails after a billable response. Failed calls with no
reported usage cannot be locally accounted for; the provider may still bill them.
Retries do not reset the daily/run ledger or get a free-call exemption.

## Design rationale

This is the token-saving contract: analyze each listing **once** per
`(goal, model)`, cache the result — even an empty one — and let repeat presses
drain a long surface a cap-sized chunk at a time without ever re-spending on a
success. A dropped or failed run resumes from cache with no double-spend (the
per-row commit lands before success is reported — S8). Explicit provenance keeps
neutral success distinct from unverified proposals without retaining raw model output.

---

## Evidence and magnitude

The prompt requires contiguous posting-only presence quotes with at least six
Unicode characters AND two existing-tokenizer tokens plus an explicit matched
goal. Presence verifies against sent text; absence checks the full description.
Per-item deltas clamp to +/-30 and net to +/-40. Evidence disclosures explain
net clipping; total scores remain 0..100 and hard exclusions short-circuit first.
Token-canonical identical evidence is grouped conservatively: minimum same-sign
magnitude, opposite-sign suppression, deterministic representative/order. This
is not a semantic paraphrase detector and does not guarantee higher chip yield.

The additive outcome migrations are SQLite 0013 and PostgreSQL 0020. A distinct
baseline-behavior compatibility recovery build must carry those exact files and
the matching complete pinned manifest; the untouched prior binary is not the
post-upgrade recovery artifact. Preserve the authoritative ledger and user writes.
See the [maintained outcome/release contract](../../docs/decisions/261009-ai-rating-and-release-contract.md)
and [completed historical specification/plan](../../docs/archive/2026-10-09-alpha-launch-and-ai-rating-release/README.md).
The [Gemini follow-up](../../docs/specs/261009-gemini-pacing-and-quota-classification.md)
is locally accepted as Slice 1 at `8c369d3cf19a4c966fe4745dfa99738c3fe78367`. Gemini starts
are spaced at least 10 seconds within one provider/pacer; other providers retain
1.2 seconds. Extraction and scoring share that instance; unrelated runtimes and
other project clients do not share a distributed quota guarantee. Explicit Gemini
minute request/token, daily, mixed and account restrictions get distinct calm copy;
generic 429/QuotaFailure/RetryInfo remain unknown in scope. Validated retry metadata
does not trigger retries. Short waits cannot be promised to restore daily/account
limits. Active progress explains provider-dependent waits, not a fixed 5–10 minutes.
Outcome/cache/prompt/version/migration contracts are unchanged. The local live-view
candidate uses SELECT-only cookie validation and credential
presence, one coherent read transaction, and no runtime/provider/decryption on
page-document and new snapshot/observe paths. Owner option 2 preserves only the
two legacy status GETs' existing `sessions.last_seen_at` activity writes; the full
browser bootstrap is not zero-write. Saved outcomes publish at successful commit,
even for detached/scheduled/noop callers. A persistence error publishes no saved
outcome and retains a run warning. Reconnect/poll/resync never starts or retries
analysis. The live-view candidate still needs independent acceptance. Local checks do not
authorize publication, production migration, deployment or live inference.
