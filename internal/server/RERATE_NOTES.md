# Re-rate (재평가) cache semantics

Why a listing can be *analyzed* yet show no `AI 분석` card, and what a repeat
재평가 press does — and does not — re-spend on. This trips people up because two
opposite states look identical on the page.

## The two reasons a listing shows no AI 분석 card

A visible, non-dealbroken row carries no `AI 분석` chip in one of two states. They
render identically but behave oppositely on a repeat press.

### A. Analyzed, but empty — cached, never re-analyzed (under the same goal)

The Stage-2 `ScoreDelta` provider call **succeeded**, but the citation gate
(`ai.GateDelta`) stripped every signal — either the model returned no signals, or
none of its quotes survived presence/absence verification. `GateDelta` returns a
real, *empty* `Delta` (`Items: []`, `NetDelta: 0`), not an error.

`rerateOne` (`rerate.go`) stores this **unconditionally** — there is no "skip if
empty" guard. `UpsertAIScore` (`internal/storage/ai_scores.go`) writes a row with
`items_json = "[]"`, `net_delta = 0`.

Consequences:

- **No card.** `scoring.Score` appends the `AI 분석` line only when
  `len(delta.Items) > 0` — the §c "no empty chips" rule (`internal/scoring/engine.go`).
  An empty delta renders nothing.
- **Counted as analyzed.** `buildRerateInfo` computes the `AI 분석 N/M` indicator's
  N from `AIScoresByPostingID`, keyed on **row presence, not chip presence**. An
  empty-items row is present, so it is inside N.
- **Never re-analyzed on a repeat press.** `rerateOne` checks the Stage-2 cache
  *first*; `AIScore` returns `ok=true` for the empty row (it reports a miss only on
  `sql.ErrNoRows`), so `rerateOne` returns `(cached: true, called: false)` and
  never calls the provider again.

An empty result is therefore **permanent under the same configuration** — the
system considers it done and has nothing more to say.

### B. Failed or never reached — no row, retried next press

The analysis did **not** complete: a provider error (timeout, 5xx, 429/529
overload, malformed JSON that fails parsing), a failed cache write, OR the listing
was never reached this press because the user's per-call cap
(`AIRuntime.PerCallCap`) or the
token budget halted first.

None of these write an `ai_scores` row (`rerateOne` returns before the upsert).
Consequences:

- **Not counted in N** (no row).
- **Retried on the next press** — the cache check misses, so control falls through
  to the spend path, subject to that press's own cap/budget.

This is what makes a second 재평가 press *advance* the counter: it picks up exactly
the Case-B rows. Intermittent `ScoreDelta` failures (seen live against real
providers) land here and recover on a later press.

**Provider errors are no longer silent.** A `ScoreDelta` error now propagates out
of `rerateOne` (it returns `(false, err)`, not a swallowed `false`). `rateStage2`
keeps the first such error and `runRerate` surfaces it: if **every** attempted row
failed (`analyzed == 0`), the SSE terminal is a calm, classified `failed` event —
`providerFailureMessage` maps a 401/403 to "AI 키를 확인해주세요", a 400/404 to
"선택한 모델이 이 제공자와 맞지 않아요" (the mismatched-model trap a provider switch
leaves behind), a 429 to a usage-cap line — instead of a hollow `done` with `0/M`.
A *partial* failure still reloads (the rows that succeeded render) but emits a
status note first. The cache behavior above is unchanged: a failed row writes no
`ai_scores` row and is retried on the next press.

## The N/M indicator is the signal that separates them

On the page, Case A and Case B both show no card. The `AI 분석 N/M` indicator is
the only thing that tells them apart: Case A is **inside** N, Case B is not. When
N reaches M, every visible listing has been successfully analyzed (some just had
nothing to say) and further presses re-spend nothing. This is the whole reason the
indicator counts the cache instead of the chips.

## When IS a cached-empty listing re-analyzed?

The Stage-2 cache key is `(posting_id, ai_input_hash, ai_version)`. An
empty-cached listing becomes a fresh miss — and is re-analyzed — only when one
of these rotates:

- **You edit a goal field** (`job_likes` / `job_dislikes` / `short_term_goals` /
  `long_term_goals`). `profile.AIInputHash` hashes only the goal text
  (NFC-normalized), so a goal edit rotates `ai_input_hash`. Weight / MinScore
  tweaks do **not** (by design — they must not churn the AI cache).
- **You switch provider or model.** `ai_version = ai.AIVersion(provider, model)`
  rotates.

Practical upshot: if the AI is *wrongly* finding nothing on a batch of listings,
re-pressing won't help — reword your goals (give the model different things to
match against) or switch models. A repeat press only recovers failed / never-run
listings.

## Token-accounting footnote

`budget.debit` (the per-run + daily `ai_usage` ledger) runs **only on the success
path**, after `ScoreDelta` returns a nil error. A failed call never debits the
local ledger — though the provider may still bill for input tokens on a
200-with-bad-JSON. So the `ai_usage` ledger can slightly under-count true provider
spend during a flaky window.

## Design rationale

This is the token-saving contract: analyze each listing **once** per
`(goal, model)`, cache the result — even an empty one — and let repeat presses
drain a long surface a cap-sized chunk at a time without ever re-spending on a
success. A dropped or failed run resumes from cache with no double-spend (the
per-row commit lands before the next provider call — S8). Caching empty results
as "analyzed-but-silent" is what the honest N/M indicator exists to disambiguate.

---

*Verified against the code 2026-06-03 (three independent traces + three
adversarial reviewers, unanimous). Locked by `TestRerateInfoCountsCacheNotChips`
and `TestRerateProgressesAcrossPressesUnderBudget` in
`internal/server/ai_rerate_test.go`.*
