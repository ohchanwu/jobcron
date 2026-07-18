# Stage 1 Contextual Dealbreaker Validation and Exclusion Evidence

**Status:** Approved design, awaiting implementation

**Created:** 2026-07-18

**Verified against:** `main` at `9491d0d2ad02c015d90844393016a7fc0306fa32`

## Context

Jobcron currently treats a token-exact dealbreaker match as conclusive. The matcher cannot
understand negation or whether a phrase describes the actual role. A profile dealbreaker such as
`리서치` therefore excludes a posting containing `리서치 아님`, even though the sentence says the
opposite of the user's concern.

This is a trust problem as well as a scoring problem. The collapsed
`관심 밖으로 분류된 공고` list currently shows a dash and a compact explanation, but not the
supporting job-description text. Users cannot quickly tell whether Jobcron understood the posting
or hid a good opportunity by mistake.

The approved change adds contextual AI validation to Stage 1 and makes every excluded card explain
its classification with evidence. It preserves deterministic scoring when AI is unavailable.

## Goals

1. Use AI to decide whether a deterministic dealbreaker hit actually applies in context.
2. Prevent confidently negated or irrelevant mentions from causing a hard exclusion.
3. Preserve the existing exclusion when AI is uncertain, unavailable, over budget, or invalid.
4. Give every excluded listing a clear reason and the best available supporting evidence.
5. Keep user-specific judgments isolated and cacheable without changing the global eligibility
   cache.
6. Preserve rule-only operation, Stage 2 scoring, score weights, and MinScore behavior.

## Non-Goals

- Semantic discovery of synonyms or translations that the deterministic matcher did not find.
- AI validation of positive stack, location, salary, or ordinary score contributions.
- Changing the free-form Stage 2 `job_dislikes` behavior.
- Letting Stage 2 override a confirmed or conservatively retained hard dealbreaker.
- Adding manual classification overrides or feedback-training controls.
- Writing the repository's missing overall architecture document.

## Verified Current State

Verified on 2026-07-18.

### Stage 1

`internal/ai/extract.go` asks the model only for career and education facts. The result contains one
general evidence string. `internal/server/server.go:779` caches it globally by posting, content
hash, and AI version. Failures silently fall back to source fields and regexes.

The global cache is correct for posting-derived eligibility facts. It must not store a judgment
that depends on one user's dealbreaker list.

### Deterministic dealbreakers

`internal/scoring/rules.go:366` checks the profile's dealbreakers before any score contribution.
`internal/scoring/match.go:25` uses contiguous, ordered, token-exact matching. It does not interpret
negation or surrounding meaning.

`internal/scoring/qa_test.go:27` already documents the known false-positive class: `야근` appears
inside positive or negated phrases such as `야근강요 안함` and `야근수당`.

### Scoring and Stage 2

`internal/scoring/engine.go:57` short-circuits a keyword or education dealbreaker to `Total = -1`
before merging a Stage 2 delta. This ordering is intentional and remains unchanged.

`internal/server/server.go:856` batch-loads cached AI state and never calls a provider. Provider
calls occur in explicit paid-AI phases, not inside the pure score merge.

### Excluded-list UI

`web/index.html:123` and `web/archive.html:117` render collapsed excluded lists. Each row currently
shows `—`, the company, and `scoring.Explain`, but no evidence card.

`web/styles.css:787` applies `opacity: 0.5` to the entire excluded row. That treatment is too faint
for evidence that exists specifically to earn user trust.

## Root Cause

The deterministic matcher answers only this question:

> Do the profile phrase's tokens occur together in the posting text?

The product needs a second answer:

> Does the posting say this unwanted condition actually applies to the role?

Stage 1 does not currently receive candidate dealbreakers, and its global cache cannot safely hold
profile-dependent answers. Stage 2 sees free-form goals, but it runs after dealbreaker exclusion
and therefore cannot repair the false positive.

The UI gap has the same root cause at a different boundary: scoring persists the result, but not a
structured, evidence-backed explanation suitable for an excluded card.

## Approved Architecture

Treat the pre-scoring AI layer as two focused parts:

```text
Posting text
  |
  +--> Stage 1A: global eligibility extraction
  |      career + education + exact evidence
  |
  +--> deterministic dealbreaker candidate matching
         |
         +--> Stage 1B: user-scoped contextual validation
                  applies | not_applicable | uncertain
                  + exact evidence
                         |
                         v
                  deterministic scoring
                         |
                         v
                  persisted exclusion reasons
                         |
                         v
                  trusted excluded-card UI
```

Stage 1B is targeted. It calls AI only when the existing matcher finds at least one candidate. This
keeps the deterministic matcher as the cheap first filter and avoids sending every profile keyword
for every posting.

### Why not extend the existing global extraction row?

Career and education are properties of a posting. Whether `리서치` violates a particular profile
is user-specific. Mixing both into `ai_extractions` would either leak one user's input into another
user's cache or force all eligibility extraction to become needlessly user-scoped.

### Why not add negation regexes?

Local patterns could catch a few phrases such as `아님`, but they would remain brittle across
Korean, English, clause boundaries, quoted text, benefits, comparisons, and indirect wording. The
requested value is contextual judgment, not a growing exception list.

## Stage 1A: Eligibility Evidence

Replace the single ambiguous evidence field with separate career and education evidence:

```go
type Extraction struct {
    MinCareer        int
    MaxCareer        *int
    Newcomer         bool
    EducationEnum    string
    CareerEvidence   string
    EducationEvidence string
}
```

Formatting may align the field names, but the public meanings are fixed.

The extraction prompt must request short verbatim quotes. A restrictive career result
(`newcomer=false` or `min_career>0`) requires `career_evidence`. A restrictive education result
requires `education_evidence`.

Before caching, each non-empty quote must be found in the full normalized posting input. A quote
that exists only in the model response is invalid. An invalid restrictive quote rejects the whole
extraction and preserves the existing regex/source fallback.

Old single-evidence rows must not be mistaken for the new contract. Introduce task-specific cache
versions instead of bumping the shared version and invalidating unrelated Stage 2 scores:

```go
const (
    EligibilityPromptVersion = "2"
    DealbreakerPromptVersion = "1"
    ScorePromptVersion       = "1"
)
```

`ScoreVersion` must preserve the current Stage 2 cache identity. `EligibilityVersion` and
`DealbreakerVersion` add their task name and prompt version to the provider/model hash. The runtime
therefore resolves three explicit versions instead of one ambiguous `Version` field.

## Stage 1B: Contextual Dealbreaker Validation

### Provider contract

Add one focused provider method rather than widening Stage 2:

```go
type DealbreakerCandidate struct {
    ID     string
    Phrase string
}

type DealbreakerVerdict string

const (
    DealbreakerApplies       DealbreakerVerdict = "applies"
    DealbreakerNotApplicable DealbreakerVerdict = "not_applicable"
    DealbreakerUncertain     DealbreakerVerdict = "uncertain"
)

type DealbreakerValidation struct {
    CandidateID string
    Verdict     DealbreakerVerdict
    Evidence    string
}

ValidateDealbreakers(
    ctx context.Context,
    modelText string,
    candidates []DealbreakerCandidate,
) ([]DealbreakerValidation, Usage, error)
```

The candidate ID is the full SHA-256 hex digest of the canonical token sequence. The raw phrase is
sent to the provider but is not stored in the validation cache.

### Prompt contract

The prompt must define `applies` as: the posting says the role requires, performs, expects, or
meaningfully includes the unwanted condition.

It must define `not_applicable` as: the phrase is negated, explicitly absent, merely quoted,
describes something the company does not require, appears only as a benefit label, or otherwise
does not assert that the condition applies to the role.

It must use `uncertain` when the posting does not support either conclusion.

The model returns only this shape:

```json
{
  "checks": [
    {
      "candidate_id": "<input id>",
      "verdict": "applies|not_applicable|uncertain",
      "evidence": "<short verbatim quote or empty for uncertain>"
    }
  ]
}
```

The existing prompt-injection boundary remains: posting text and candidate phrases are untrusted
data, never instructions.

### Validation gate

The parser must reject unknown verdicts, unknown IDs, duplicate IDs, and malformed JSON. A missing
candidate result is treated as unresolved for that operation.

`applies` and `not_applicable` require evidence that:

1. occurs verbatim after NFC normalization in the full posting input;
2. contains the matched candidate token sequence; and
3. stays within a bounded quote length.

An invalid conclusive result is not cached and falls back conservatively. A valid `uncertain`
result may be cached with empty evidence so repeated runs do not repeatedly spend money on the same
ambiguity. Transport, provider, and budget failures are not cached and may retry later.

### Multiple matches

Validate every deterministic candidate match. A posting avoids keyword exclusion only when every
matched phrase has a cached `not_applicable` verdict.

One `applies`, `uncertain`, missing, or unavailable verdict retains the hard exclusion. The reason
panel lists every retained reason in profile order so a later match cannot be hidden behind the
first one.

## Persistence

### PostgreSQL

Add `internal/storage/postgres_migrations/0017_contextual_dealbreakers.sql`:

```sql
ALTER TABLE ai_extractions
    RENAME COLUMN evidence TO career_evidence;

ALTER TABLE ai_extractions
    ADD COLUMN education_evidence TEXT NOT NULL DEFAULT '';

CREATE TABLE ai_dealbreaker_validations (
    user_id       BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    posting_id    BIGINT NOT NULL REFERENCES postings(id) ON DELETE CASCADE,
    content_hash  TEXT NOT NULL,
    ai_version    TEXT NOT NULL,
    keyword_hash  TEXT NOT NULL,
    verdict       TEXT NOT NULL CHECK (
        verdict IN ('applies', 'not_applicable', 'uncertain')
    ),
    evidence      TEXT NOT NULL,
    computed_at   TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (
        user_id,
        posting_id,
        content_hash,
        ai_version,
        keyword_hash
    )
);
```

The table is user-scoped even though a judgment can be objectively reusable. This avoids turning
profile phrases into shared cross-account state and matches the existing ownership boundary for
preference-derived AI data.

### Legacy SQLite compatibility

Add the equivalent additive table and extraction columns in SQLite migration `0013`. Normal
application startup remains PostgreSQL-only. SQLite support exists only for the verified legacy
reader and isolated compatibility tests; it must not enable paid AI or invent `user_id = 0` state.

### Store methods

Add explicit user-scoped methods:

```go
UpsertAIDealbreakerValidation(
    ctx context.Context,
    userID int64,
    postingID int64,
    contentHash string,
    aiVersion string,
    keywordHash string,
    validation ai.DealbreakerValidation,
    computedAt time.Time,
) error

AIDealbreakerValidationsByPostingID(
    ctx context.Context,
    userID int64,
    aiVersion string,
) (map[int64]map[string]storage.AIDealbreakerValidation, error)
```

The batched read must not introduce an N+1 query. Scoring ignores rows whose content hash no longer
matches the posting.

## Runtime Flow

### New and changed postings

For each detailed posting during a manual or scheduled scrape:

1. run or reuse Stage 1A;
2. find deterministic dealbreaker candidates for the active user;
3. batch one Stage 1B request for the unresolved candidates;
4. citation-gate and persist valid results;
5. debit the same user-scoped AI budget and usage ledger;
6. run the normal score merge; and
7. run automatic Stage 2 only after corrected exclusions are known.

A Stage 1B cache hit makes no provider call and spends no tokens.

### Explicit AI re-rate

The re-rate operation must process Stage 1B before selecting the Stage 2 `Today` set. It must
include deterministic candidate hits from currently excluded postings; otherwise the false
positive can never re-enter the main list.

After Stage 1B completes, re-score all of that user's postings, rebuild the eligible set, then run
Stage 2 over its normal scope. Dealbreaker changes count as an AI-input change for re-rate readiness
even though they remain separate from the Stage 2 goal hash.

### Profile save and startup

Profile save remains fast and provider-free. It commits the profile, re-scores from existing
caches, and marks re-rate work pending when the normalized dealbreaker hash changed or validations
are missing. The user can then run the existing AI re-rate flow.

Startup remains provider-free. It may merge cached validations and use the conservative fallback,
but it must not create surprise paid calls.

### Conservative fallback

The approved fallback is fixed:

- `applies`: retain the deterministic hard exclusion and mark it AI-confirmed.
- `not_applicable`: suppress that keyword hit.
- `uncertain`: retain the deterministic hard exclusion and mark it uncertain.
- no runtime, no key, provider failure, invalid evidence, budget exhaustion, or cache miss: retain
  the deterministic hard exclusion and mark it unverified.

Rule-only users therefore keep today's behavior. AI improves confident false positives but never
silently weakens an unresolved user-defined hard constraint.

## Scoring Result Contract

Persist structured reasons inside the existing per-user score JSON. Do not add a second render-time
join for data already known during scoring.

```go
type ExclusionReason struct {
    Kind       string // keyword | education | career | min_score
    Label      string
    Phrase     string
    Evidence   string
    Confidence string // confirmed | uncertain | unverified | deterministic
}

type ScoreResult struct {
    Total            int
    Breakdown        []LineItem
    DealbreakerHit   *DealbreakerHit
    ExclusionReasons []ExclusionReason
}
```

Keep `DealbreakerHit` during this change so existing score JSON and callers remain backward
compatible. `ExclusionReasons` becomes the rendering contract.

Reason priority is:

1. retained keyword dealbreakers, in profile order;
2. education requirement above the user's education;
3. career ineligibility, including `신입 지원 불가`;
4. generic `기준 점수 미달: <score>점 / 기준 <minimum>점`.

The first three use exact evidence when available. The generic MinScore reason must not invent a JD
quote. A below-MinScore listing may show both a specific missed requirement and the score threshold
when both explain the classification.

Bookmarked postings retain their existing MinScore exemption. Bookmarks do not override a hard
dealbreaker. No score weight or threshold changes in this feature.

## Excluded-Card Trust Surface

Render a shared exclusion-reason partial on the daily briefing and archive excluded lists. The
partial may also be reused on bookmarked or hidden hard-dealbreaker rows, but those pages must not
change classification behavior.

Each excluded listing shows one visible reason panel below its metadata:

```text
! 제외 이유
  신입 지원 불가
  "경력 2년 이상의 백엔드 개발자를 찾습니다"
  AI 문맥 확인
```

or:

```text
! 제외 이유
  제외 키워드: 리서치
  "본 포지션은 사용자 리서치를 직접 수행합니다"
  AI 문맥 확인
```

An unverified fallback says `규칙 기반 · AI 문맥 확인 없음`. An uncertain cached result says
`AI 문맥 확인 불확실`. The status is visible text, not a tooltip.

### Visual requirements

- Add semantic `--danger`, `--danger-soft`, and `--danger-border` tokens for light and dark themes.
- Use a red danger treatment for the reason panel, but never rely on color alone.
- Include a text heading and icon or symbol with an accessible label.
- Give evidence a distinct quoted treatment.
- Highlight the exact matched keyword inside keyword evidence with `<mark>` when present.
- Build highlighted segments as escaped strings; never pass model output through `template.HTML`.
- Remove whole-row opacity from `.excluded-box .posting` so all text meets contrast targets.
- Keep the collapsed summary and posting actions unchanged.
- Support desktop and mobile without horizontal scrolling or clipped evidence.

## Failure Handling

- A Stage 1B failure cannot abort a scrape, profile save, startup, or rule-based rescore.
- A failed or partial batch records no false conclusive verdicts.
- Valid results from a partial batch may commit independently; unresolved candidates fall back.
- Provider errors use the existing classified, non-secret operator and user messages.
- Usage is debited for every completed provider call, including an `uncertain` result.
- No profile phrase, API key, prompt body, or model response may enter logs.
- Storage and rendering errors remain ordinary operation failures; they must not silently fabricate
  a reason.

## Security and Privacy

- Treat the posting and every profile phrase as untrusted prompt data.
- Store only the keyword hash in the validation table.
- Keep the raw phrase only in the user-scoped profile and score JSON where it is already needed for
  display.
- Validate every displayed AI quote against the full posting input.
- Let Go templates escape all reason and evidence fields.
- Preserve the existing user ID and AI-runtime match checks before every paid call.
- Preserve per-user token and USD caps.

## Do Not Touch

- Global `ai_extractions` ownership for posting-derived facts.
- Source posting columns as the faithful scraper mirror.
- Stage 2 goal scoring, citation gate, cache keys, and stale-chip behavior.
- Dealbreaker-before-Stage-2 ordering.
- Score weights, MinScore, bookmark exemptions, and hidden-posting behavior.
- Sole-owner scheduler policy and user-scoped credentials.
- PostgreSQL-only normal startup and the read-only purpose of legacy SQLite.

## Testing Plan

### Unit tests

- Parse all three verdicts and reject unknown or duplicate candidate IDs.
- Reject conclusive evidence absent from the full posting text.
- Confirm `리서치 아님` and `야근강요 안함` can return `not_applicable`.
- Confirm an affirmative responsibility can return `applies`.
- Confirm `uncertain`, missing, and invalid results use the conservative fallback.
- Confirm multiple candidates require every hit to be `not_applicable` before suppression.
- Confirm career and education evidence gates.
- Confirm old score JSON without `ExclusionReasons` still unmarshals.

### Storage tests

- PostgreSQL migration `0017` applies and is idempotent through the migration runner.
- User A cannot read or overwrite user B's validation rows.
- Content, AI-version, and keyword-hash changes miss the cache.
- Posting and user deletion cascade to validation rows.
- Batch reads return current rows without N+1 queries.
- The legacy SQLite compatibility schema opens without enabling paid AI.

### Server integration tests

- Scrape order is Stage 1A, Stage 1B, score merge, then Stage 2.
- A cached validation produces zero provider calls.
- Explicit re-rate checks currently excluded candidate postings before rebuilding `Today`.
- A dealbreaker profile change marks re-rate pending.
- Budget exhaustion retains deterministic exclusions and truthful unverified labels.
- Startup performs no provider call.
- Two users with different dealbreakers receive independent classifications and evidence.

### Rendering and browser QA

- Daily and archive excluded rows render the reason, status, and evidence.
- Career mismatch renders `신입 지원 불가` when applicable.
- Generic MinScore exclusion renders the score and threshold without fake evidence.
- AI output is escaped and cannot inject markup.
- Matched keyword highlighting does not break Korean or English text.
- Walk the affected flows in a real headless browser on desktop and mobile.
- Verify light and dark themes, disclosure toggling, bookmarks, mute actions, and outbound links.
- Verify no console errors and capture updated screenshots at the repository's required dimensions.

### Live provider gate

With an explicitly configured test key, validate at least these two synthetic postings end to end:

1. a matched keyword inside a clear negation is not hard-excluded;
2. the same keyword stated as an actual responsibility is excluded with an exact quote.

The live gate must debit a disposable user's usage ledger and must not print the credential or raw
provider response.

## Acceptance Criteria

1. `리서치 아님` does not hard-exclude a posting when Stage 1B returns a citation-gated
   `not_applicable` verdict for the matched `리서치` candidate.
2. An affirmative `리서치` responsibility remains `Total = -1` when Stage 1B returns `applies`.
3. `uncertain`, missing, invalid, unavailable, or over-budget validation retains the deterministic
   exclusion and displays the correct non-confirmed status.
4. Every excluded daily and archive listing renders at least one structured reason.
5. Keyword, career, and education reasons show exact evidence when valid evidence exists.
6. Generic MinScore exclusion shows the actual score and threshold and never fabricates a quote.
7. Every displayed AI quote is proven present in the full normalized posting input.
8. Multiple keyword hits suppress exclusion only when every hit is `not_applicable`.
9. Validation cache rows are isolated by user and invalidated by posting content, keyword, model,
   provider, or prompt-version changes.
10. Cache hits spend no tokens; completed provider calls debit the existing per-user ledger.
11. Stage 2 cannot override a retained hard dealbreaker.
12. Startup and rule-only operation perform no paid AI calls and preserve current classifications.
13. Excluded evidence remains readable in light and dark themes at desktop and mobile widths.
14. Existing bookmark, mute, source-filter, archive, and outbound-link behavior remains intact.
15. The full Go tests, race tests, static checks, builds, PostgreSQL integration tests,
    live AI gate, and browser QA pass on the same candidate commit.

## Implementation Order

```text
A. AI contracts and citation gates
        |
        +--> B. additive schema and user-scoped cache
        |
        +--> C. scoring reason contract
                    |
                    v
D. scrape and re-rate orchestration
        |
        v
E. shared reason panel and theme tokens
        |
        v
F. full integration, live-provider, and browser verification
```

The contract and schema land first because orchestration must have stable inputs and cache keys.
The reason contract lands before the UI so templates receive one explicit view model rather than
reconstructing scoring policy. Browser work comes after the backend can produce every status.

## File Impact

### AI contracts

- `internal/ai/provider.go`: candidate, verdict, validation, and provider method.
- `internal/ai/extract.go`: split eligibility evidence and citation gating.
- New focused dealbreaker-validation prompt and parser beside existing AI contracts.
- `internal/ai/version.go`: task-specific cache versions without Stage 2 cache churn.

### Scoring and profile

- `internal/scoring/match.go`: expose candidate matches and safe evidence spans.
- `internal/scoring/rules.go`: merge contextual verdicts and career evidence.
- `internal/scoring/engine.go`: persist structured exclusion reasons.
- `internal/scoring/explain.go`: preserve compact legacy explanations.
- `internal/profile/profile.go`: normalized dealbreaker input hash.

### Storage and server

- PostgreSQL migration `0017` and SQLite compatibility migration `0013`.
- New storage methods for user-scoped validation rows.
- `internal/server/server.go`: scrape Stage 1B and cache merge.
- `internal/server/rerate.go`: excluded-candidate backfill before Stage 2.
- `internal/server/handlers.go`: reason view model and re-rate readiness.
- `internal/server/archive.go`, bookmarks, and hidden views: reuse stored reasons where applicable.

### Web

- New shared exclusion-reason template partial.
- `web/index.html` and `web/archive.html`: render the partial.
- `web/styles.css`: semantic danger tokens, reason panel, evidence, and contrast fix.
- Existing web tests plus browser screenshots.

## Effort Estimate

- AI contracts, prompts, parsers, and citation gates: 0.5 human day.
- Schema, storage methods, and isolation tests: 0.5 human day.
- Scoring, scrape, re-rate, and fallback orchestration: 1 human day.
- Shared UI, accessibility, responsive styling, and template tests: 0.5 human day.
- Live-provider, PostgreSQL, race, browser, and documentation verification: 0.5 human day.

Estimated total: 3 human engineering days, or one deliberate AI-assisted implementation campaign
with independent review checkpoints.

## Rollback

Revert the implementation commits and restore the prior prompt-template version. The new table and
additive evidence column may remain empty without affecting the previous runtime. Existing score
rows are regenerated by the startup rescore path, so rollback does not require rewriting user data.

Do not drop the new table during an emergency rollback. Removing unused additive state can happen
later in a normal migration after the reverted release is stable.
