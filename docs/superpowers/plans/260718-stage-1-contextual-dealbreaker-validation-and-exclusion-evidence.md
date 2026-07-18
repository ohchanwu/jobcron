# Stage 1 Contextual Dealbreaker Validation and Exclusion Evidence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Contextually validate deterministic dealbreaker matches before hard exclusion and show
clear, evidence-backed reasons on every excluded posting.

**Architecture:** Keep token-exact matching as the cheap candidate generator, then call a focused
user-scoped Stage 1B validator only for unresolved matches. Store citation-gated verdicts in
PostgreSQL, merge them into the existing pure scorer, and persist the resulting reason list inside
`scores.breakdown_json` so daily and archive templates render the exact decision that was scored.

**Tech Stack:** Go 1.26.3, PostgreSQL through `database/sql` and pgx, Anthropic's existing Messages
API client, Go HTML templates, CSS, and gstack `/browse` for browser QA.

## Global Constraints

- Follow the approved
  [contextual dealbreaker specification](../specs/260718-stage-1-contextual-dealbreaker-validation-and-exclusion-evidence.md).
- Stage 1B runs only after the existing matcher finds a token-exact candidate. Do not add synonym
  discovery, translation, embeddings, or a general classifier.
- A posting clears keyword exclusion only when every deterministic hit has a valid cached
  `not_applicable` verdict.
- `applies`, `uncertain`, missing, invalid, unavailable, or over-budget results retain the current
  hard exclusion. Never weaken an unresolved user-defined constraint.
- Implement the new schema and Stage 1B storage only in PostgreSQL. Do not add a SQLite migration,
  validation table, provider path, or compatibility test.
- Keep the frozen SQLite source schema only for the verified importer and rollback window. The
  importer may map legacy `evidence` to PostgreSQL `career_evidence`; it must not gain Stage 1B.
- Profile save and startup remain provider-free. A paid call occurs only during scrape or the
  user-triggered `AI 평가` flow.
- Preserve Stage 2 weights, goal prompt, cache identity, stale-chip behavior, MinScore semantics,
  bookmark exemptions, and dealbreaker-before-Stage-2 ordering.
- Keep every provider call bound to the resolved `AIRuntime.UserID` and the existing per-user token
  and USD budgets. Do not log profile phrases, prompts, responses, or credentials.
- Persist display reasons in the existing `ScoreResult` JSON. Do not add a render-time query of
  `ai_dealbreaker_validations`.
- Use existing packages and dependencies. Do not add a new module.
- Keep documentation lines and Markdown table rows at or below 100 display columns.
- After implementation, update `docs/architecture.md`, archive the completed spec and plan, and
  update `docs/superpowers/README.md`.
- Commit at each task checkpoint. Keep commits local and never push.

---

### Task 1: Split eligibility evidence and AI cache versions

**Files:**

- Modify: `internal/ai/provider.go`
- Modify: `internal/ai/extract.go`
- Modify: `internal/ai/extract_test.go`
- Modify: `internal/ai/anthropic.go`
- Modify: `internal/ai/stub.go`
- Modify: `internal/ai/stub_test.go`
- Modify: `internal/ai/version.go`
- Modify: `internal/ai/version_test.go`

**Interfaces:**

- Produces `Extraction.CareerEvidence` and `Extraction.EducationEvidence` for storage and scoring.
- Produces task-specific cache-version functions consumed by `AIRuntime` in Task 5.
- Preserves `AIVersion(provider, model)` as the Stage 2 compatibility alias.

- [ ] **Step 0: Record the exact implementation base**

```sh
mkdir -p .superpowers/sdd/260718-contextual-dealbreakers
git rev-parse HEAD > \
  .superpowers/sdd/260718-contextual-dealbreakers/implementation-base.txt
```

This ignored evidence file is the cumulative-diff base used in Task 7. Do not estimate or replace
it with a later commit.

- [ ] **Step 1: Write failing extraction-evidence tests**

Add focused cases with these names:

```go
func TestParseExtractionRequiresRestrictiveCareerEvidence(t *testing.T)
func TestParseExtractionRequiresRestrictiveEducationEvidence(t *testing.T)
func TestExtractionRejectsEvidenceAbsentFromPosting(t *testing.T)
func TestExtractionAcceptsSeparateVerbatimEvidence(t *testing.T)
```

Use one fixture where `newcomer=false` cites `경력 2년 이상`, one where `education_enum` is
`bachelor` and cites `학사 이상`, and one forged quote absent from `modelText`. The forged and
missing restrictive evidence cases must return an error so the caller keeps the regex/source
fallback and stores no extraction.

- [ ] **Step 2: Write failing version-partition tests**

Replace the single-version assumptions with this contract:

```go
func TestTaskVersionsAreStableAndPartitioned(t *testing.T) {
    score := ScoreVersion("anthropic", "claude-x")
    if score != AIVersion("anthropic", "claude-x") {
        t.Fatal("Stage 2 cache identity changed")
    }
    if EligibilityVersion("anthropic", "claude-x") == score {
        t.Fatal("eligibility and score versions must be separate")
    }
    if DealbreakerVersion("anthropic", "claude-x") == score {
        t.Fatal("dealbreaker and score versions must be separate")
    }
}
```

Also assert that provider, model, task name, and that task's prompt version each rotate only the
corresponding version.

- [ ] **Step 3: Run the focused tests and confirm the red state**

```sh
go test ./internal/ai \
  -run 'Extraction.*Evidence|TaskVersionsAreStableAndPartitioned' -count=1
```

Expected: compile or assertion failures because the split fields and version functions do not yet
exist.

- [ ] **Step 4: Implement the minimal extraction and version contracts**

Use this public shape:

```go
type Extraction struct {
    MinCareer        int
    MaxCareer        *int
    Newcomer         bool
    EducationEnum    string
    CareerEvidence   string
    EducationEvidence string
}

const (
    EligibilityPromptVersion = "2"
    DealbreakerPromptVersion = "1"
    ScorePromptVersion       = "1"
)

func EligibilityVersion(provider, model string) string
func DealbreakerVersion(provider, model string) string
func ScoreVersion(provider, model string) string
func AIVersion(provider, model string) string
```

Implement all three through one unexported SHA-256 helper over NUL-separated parts.
`ScoreVersion` must hash provider, model, and `ScorePromptVersion` in exactly the old order.
Eligibility and dealbreaker versions additionally include their task name. `AIVersion` must return
`ScoreVersion` so every existing Stage 2 row remains addressable.

Update the extraction prompt and parser to require `career_evidence` and `education_evidence`.
After parsing, NFC-normalize posting input and each quote, then require every non-empty quote to
occur verbatim. Restrictive career and education results require their corresponding quote.

- [ ] **Step 5: Update provider and stub fixtures, then run the package**

Update existing fixtures from `Evidence` to the correct split field. Do not retain a third
ambiguous evidence field.

```sh
gofmt -w internal/ai
go test ./internal/ai -count=1
```

Expected: all `internal/ai` tests pass and Stage 2 version tests prove backward compatibility.

- [ ] **Step 6: Commit Task 1**

```sh
git add internal/ai
git diff --cached --check
git commit -m "refactor(ai): split eligibility evidence and cache versions"
```

### Task 2: Add the focused Stage 1B provider contract and validation gate

**Files:**

- Create: `internal/ai/dealbreakers.go`
- Create: `internal/ai/dealbreakers_test.go`
- Modify: `internal/ai/provider.go`
- Modify: `internal/ai/anthropic.go`
- Modify: `internal/ai/provider_test.go`
- Modify: `internal/ai/stub.go`
- Modify: `internal/ai/stub_test.go`
- Modify: `internal/ai/injection_test.go`

**Interfaces:**

- Consumes the existing HTTP completion client and `internal/tokenmatch` tokenizer.
- Produces `Provider.ValidateDealbreakers` for Task 5.
- Produces citation-gated `DealbreakerValidation` values for Tasks 3 and 4.

- [ ] **Step 1: Write failing parser and citation-gate tests**

Cover this exact matrix:

```go
func TestParseDealbreakerValidationsAcceptsAllVerdicts(t *testing.T)
func TestParseDealbreakerValidationsRejectsUnknownID(t *testing.T)
func TestParseDealbreakerValidationsRejectsDuplicateID(t *testing.T)
func TestParseDealbreakerValidationsTreatsMissingIDAsUnresolved(t *testing.T)
func TestParseDealbreakerValidationsRejectsForgedEvidence(t *testing.T)
func TestParseDealbreakerValidationsRejectsEvidenceWithoutCandidate(t *testing.T)
func TestParseDealbreakerValidationsAllowsUncertainWithoutEvidence(t *testing.T)
func TestDealbreakerPromptKeepsPostingAndPhrasesAsData(t *testing.T)
```

Use `리서치 아님` for a valid `not_applicable` result and a sentence asserting research duties for
a valid `applies` result. A conclusive quote must be short, occur in normalized posting text, and
contain the candidate's contiguous token sequence.

- [ ] **Step 2: Run the tests and confirm they fail**

```sh
go test ./internal/ai -run 'Dealbreaker|PromptKeepsPosting' -count=1
```

Expected: compile failures because the contract and parser do not exist.

- [ ] **Step 3: Add the provider contract**

Use the approved public types without a general-purpose classification abstraction:

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

type Provider interface {
    Name() string
    Extract(context.Context, string) (Extraction, Usage, error)
    ValidateDealbreakers(
        context.Context,
        string,
        []DealbreakerCandidate,
    ) ([]DealbreakerValidation, Usage, error)
    ScoreDelta(context.Context, string, string) ([]RawDeltaItem, Usage, error)
}
```

Add only the focused prompt, request builder, parser, and evidence gate in
`internal/ai/dealbreakers.go`. Reject malformed JSON, unknown verdicts, unknown IDs, duplicate IDs,
overlong evidence, and invalid conclusive evidence. Return valid partial results; callers treat
missing IDs as unresolved and do not invent a verdict.

- [ ] **Step 4: Wire the HTTP provider and stub**

Implement:

```go
func (p *httpProvider) ValidateDealbreakers(
    ctx context.Context,
    modelText string,
    candidates []DealbreakerCandidate,
) ([]DealbreakerValidation, Usage, error)
```

Return immediately without a provider call when `candidates` is empty. Update the existing stub
with one optional function field matching the new method, and return `ErrNotImplemented` when it is
unset.

- [ ] **Step 5: Run package, race, and injection tests**

```sh
gofmt -w internal/ai
go test ./internal/ai -count=1
go test -race ./internal/ai -count=1
```

Expected: all parser, provider, citation, injection, and race tests pass.

- [ ] **Step 6: Commit Task 2**

```sh
git add internal/ai
git diff --cached --check
git commit -m "feat(ai): validate contextual dealbreaker matches"
```

### Task 3: Add the PostgreSQL cache and preserve the legacy import boundary

**Files:**

- Create: `internal/storage/postgres_migrations/0017_contextual_dealbreakers.sql`
- Create: `internal/storage/ai_dealbreakers.go`
- Create: `internal/storage/ai_dealbreakers_test.go`
- Modify: `internal/storage/ai_extractions.go`
- Modify: `internal/storage/ai_extractions_test.go`
- Modify: `internal/storage/postgres_integration_test.go`
- Modify: `cmd/jobcron-import/main.go`
- Modify: `cmd/jobcron-import/main_test.go`

**Interfaces:**

- Consumes Task 1's split `Extraction` and Task 2's validation types.
- Produces one batched, user-scoped cache read for `scoreAll` in Task 5.
- Preserves the frozen SQLite source schema while adapting its PostgreSQL destination mapping.

- [ ] **Step 1: Write the migration and isolation tests first**

Add tests with these contracts:

```go
func TestContextualDealbreakerMigrationSplitsExtractionEvidence(t *testing.T)
func TestAIDealbreakerValidationRoundTrip(t *testing.T)
func TestAIDealbreakerValidationIsUserScoped(t *testing.T)
func TestAIDealbreakerValidationKeyChangesMiss(t *testing.T)
func TestAIDealbreakerValidationCascades(t *testing.T)
func TestAIDealbreakerValidationsUseOneBatchQuery(t *testing.T)
```

Seed migration `0016` with one extraction row, apply `0017`, and assert the prior quote moved to
`career_evidence` while `education_evidence` is empty. In cache tests, use two users and the same
posting, then verify neither user can read or overwrite the other's row. Delete the posting and one
user to prove both cascades.

- [ ] **Step 2: Add the PostgreSQL migration**

Create exactly this schema change:

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

Do not create `internal/storage/migrations/0013_*.sql` or any SQLite equivalent.

- [ ] **Step 3: Add the storage row and methods**

Use this storage type and the approved signatures:

```go
type AIDealbreakerValidation struct {
    PostingID   int64
    ContentHash string
    AIVersion   string
    KeywordHash string
    Validation  ai.DealbreakerValidation
    ComputedAt  time.Time
}

func (s *Store) UpsertAIDealbreakerValidation(
    ctx context.Context,
    userID int64,
    postingID int64,
    contentHash string,
    aiVersion string,
    keywordHash string,
    validation ai.DealbreakerValidation,
    computedAt time.Time,
) error

func (s *Store) AIDealbreakerValidationsByPostingID(
    ctx context.Context,
    userID int64,
    aiVersion string,
) (map[int64]map[string]AIDealbreakerValidation, error)
```

Reject non-positive user IDs and non-PostgreSQL stores before issuing SQL. Query the full user's
current AI version once and group rows in Go. Key the inner map by
`contentHash + "\x00" + keywordHash`; this preserves historical content rows while allowing the
caller holding the current posting to perform one exact lookup. Reject an upsert when
`validation.CandidateID != keywordHash`; cache identity and returned payload must agree.

- [ ] **Step 4: Update extraction storage and importer mapping**

Update PostgreSQL extraction reads and writes to use `career_evidence` and `education_evidence`.
Keep the SQLite schema frozen. The existing SQLite storage adapter remains import-fixture-only:
map its single `evidence` value to `CareerEvidence`, leave `EducationEvidence` empty, and reject a
SQLite write that would discard non-empty education evidence. Do not add SQLite columns or use
this adapter in application runtime.

In `copyAIExtractions`, continue selecting legacy SQLite `evidence`, but write the PostgreSQL
destination as:

```sql
INSERT INTO ai_extractions (
    posting_id,
    content_hash,
    ai_version,
    min_career,
    max_career,
    newcomer,
    education_enum,
    career_evidence,
    education_evidence,
    computed_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'',$9)
```

Update the representative-row comparison accordingly. Do not alter SQLite migrations or write
Stage 1B rows during import; old databases cannot contain them.

- [ ] **Step 5: Run PostgreSQL storage and importer tests**

```sh
JOBCRON_TEST_POSTGRES_URL="$DATABASE_URL" \
  go test ./internal/storage ./cmd/jobcron-import \
  -run 'ContextualDealbreaker|AIDealbreaker|AIExtraction|Import' -count=1
```

Expected: migration, isolation, cascade, batch-read, extraction, dry-run, apply, rollback, and
representative-row verification tests pass.

- [ ] **Step 6: Commit Task 3**

```sh
git add internal/storage cmd/jobcron-import
git diff --cached --check
git commit -m "feat(storage): persist contextual dealbreaker verdicts"
```

### Task 4: Merge every candidate verdict into one persisted scoring decision

**Files:**

- Modify: `internal/scoring/match.go`
- Modify: `internal/scoring/rules.go`
- Modify: `internal/scoring/engine.go`
- Modify: `internal/scoring/explain.go`
- Modify: `internal/scoring/engine_test.go`
- Modify: `internal/scoring/ai_cache_test.go`
- Modify: `internal/scoring/qa_test.go`
- Modify: `internal/profile/profile.go`
- Modify: `internal/profile/profile_test.go`

**Interfaces:**

- Consumes Task 2's candidate and validation types.
- Produces `DealbreakerCandidates`, `ExclusionReasons`, and the normalized dealbreaker hash used by
  Task 5.
- Keeps `DealbreakerHit` so existing JSON and compact explanations remain readable.

- [ ] **Step 1: Write failing candidate and fallback tests**

Add cases proving:

```go
func TestDealbreakerCandidatesReturnsEveryMatchInProfileOrder(t *testing.T)
func TestScoreSuppressesHitOnlyWhenNotApplicable(t *testing.T)
func TestScoreRetainsAppliesUncertainMissingAndUnavailableHits(t *testing.T)
func TestScoreDoesNotHideLaterApplicableHit(t *testing.T)
func TestScorePreservesDealbreakerBeforeStage2(t *testing.T)
```

The multi-hit fixture must contain both `리서치 아님` and an affirmative `야근` statement. Give
the first candidate `not_applicable` and the second `applies`; the result must remain `Total = -1`
and show only the retained `야근` reason.

- [ ] **Step 2: Write failing structured-reason and compatibility tests**

Cover keyword, education, career, and MinScore reasons, including exact evidence and confidence.
Also keep this compatibility check:

```go
func TestScoreResultUnmarshalsWithoutExclusionReasons(t *testing.T) {
    var got ScoreResult
    err := json.Unmarshal(
        []byte(`{"Total":-1,"Breakdown":[],"DealbreakerHit":{"Kind":"keyword","Phrase":"리서치"}}`),
        &got,
    )
    if err != nil || got.ExclusionReasons != nil {
        t.Fatalf("legacy score JSON: result=%+v err=%v", got, err)
    }
}
```

- [ ] **Step 3: Implement candidate identity and collection**

Expose one focused helper:

```go
func DealbreakerCandidates(
    p scraper.Posting,
    prof profile.Profile,
) []ai.DealbreakerCandidate
```

For each matched phrase in profile order, tokenize with `internal/tokenmatch`, join the canonical
tokens with NUL bytes, and use the full lowercase SHA-256 hex digest as `Candidate.ID`. Skip empty
phrases and preserve every real match; do not short-circuit after the first.

- [ ] **Step 4: Add the scoring reason contract**

Use the spec's stable shape with JSON field names:

```go
type ExclusionReason struct {
    Kind       string `json:"kind"`
    Label      string `json:"label"`
    Phrase     string `json:"phrase,omitempty"`
    Evidence   string `json:"evidence,omitempty"`
    Confidence string `json:"confidence"`
}

type ScoreResult struct {
    Total            int
    Breakdown        []LineItem
    DealbreakerHit   *DealbreakerHit
    ExclusionReasons []ExclusionReason `json:"exclusion_reasons,omitempty"`
}

func Score(
    p scraper.Posting,
    prof profile.Profile,
    ext *ai.Extraction,
    delta *ai.Delta,
    validations map[string]ai.DealbreakerValidation,
) ScoreResult
```

Map candidate IDs to verdicts. Suppress only `not_applicable`; retain every other hit with
`confirmed`, `uncertain`, or `unverified` confidence. Then append education, career, and MinScore
reasons in the approved priority order. Never attach a Stage 2 line to a retained hard
dealbreaker, and never fabricate evidence for the generic MinScore reason.

- [ ] **Step 5: Add the normalized profile hash**

Implement:

```go
func DealbreakerInputHash(p Profile) string
```

Hash the ordered canonical token sequences with unambiguous separators. Whitespace, punctuation,
case, and Unicode-composition-only changes must keep the hash stable; phrase order and semantic
token changes must rotate it.

- [ ] **Step 6: Run scoring and profile tests**

```sh
gofmt -w internal/scoring internal/profile
go test ./internal/scoring ./internal/profile -count=1
go test -race ./internal/scoring ./internal/profile -count=1
```

Expected: all prior weights and matching tests pass alongside the contextual and JSON tests.

- [ ] **Step 7: Commit Task 4**

```sh
git add internal/scoring internal/profile
git diff --cached --check
git commit -m "feat(scoring): persist contextual exclusion reasons"
```

### Task 5: Orchestrate Stage 1B in scrape, re-rate, profile, and startup flows

**Files:**

- Modify: `internal/server/server.go`
- Modify: `internal/server/rerate.go`
- Modify: `internal/server/rerate_status.go`
- Modify: `internal/server/handlers.go`
- Modify: `internal/server/ai_runtime_test.go`
- Modify: `internal/server/ai_config_test.go`
- Modify: `internal/server/ai_scrape_test.go`
- Modify: `internal/server/ai_rerate_test.go`
- Modify: `internal/server/rerate_status_test.go`
- Modify: `internal/server/production_user_scope_test.go`
- Modify: `internal/server/scheduler_test.go`
- Modify: `internal/server/unscored_test.go`

**Interfaces:**

- Consumes Tasks 1 through 4.
- Produces cache-only inputs for `scoreAll` and paid Stage 1B work only in scrape and `AI 평가`.
- Preserves the existing sole-owner scheduler and authenticated-user boundaries.

- [ ] **Step 1: Write failing runtime-version and user-isolation tests**

Replace `AIRuntime.Version` expectations with:

```go
type AIRuntime struct {
    UserID             int64
    Provider           ai.Provider
    EligibilityVersion string
    DealbreakerVersion string
    ScoreVersion       string
    RunTokenCap        int
    DailyTokenCap      int
    MonthlyTokenCap    int
    PerCallCap         int
}
```

Assert that runtime construction derives all three versions from the same user's provider and
model. Add a two-user test where identical posting text and different dealbreaker profiles produce
separate validation rows and explanations.

- [ ] **Step 2: Write failing scrape-order and cache-hit tests**

Add tests proving this sequence for each detailed posting:

```text
Stage 1A -> deterministic candidates -> Stage 1B -> scoreAll -> Stage 2
```

Verify a current Stage 1B cache row makes zero provider calls and spends zero tokens. Verify a
provider error, malformed response, invalid quote, missing credential, and exhausted budget each
retain the deterministic exclusion and do not abort the scrape.

- [ ] **Step 3: Implement the focused Stage 1B runner**

Add one server helper rather than putting provider calls inside `scoring.Score`:

```go
func (s *Server) validateDealbreakers(
    ctx context.Context,
    userID int64,
    postings []scraper.Posting,
    prof profile.Profile,
    runtime *AIRuntime,
    budget *aiBudget,
    calls *callCap,
    emit func(event, data string),
) (providerCalls int, providerErr error)
```

For each posting, compute `ai.ModelInput`, obtain every deterministic candidate, discard exact
cache hits, and send all unresolved candidates for that posting in one provider request. Before
each paid call, verify runtime user, budget, and call cap. Debit usage for every completed call,
including `uncertain`; persist only citation-gated results. Do not cache transport, budget, or
invalid-evidence failures.

- [ ] **Step 4: Merge cached validations in `scoreAll`**

Batch-load extractions, Stage 1B rows, and Stage 2 deltas once each. For every posting, ignore a
validation whose stored content hash differs from the current `ai.ModelInput` hash. Pass the
remaining candidate-ID map into `scoring.Score`, marshal the complete `ScoreResult` once, and write
it through the existing user-scoped score method.

`RescoreAll`, `RescoreSoleOwner`, startup, and profile save may call this merge but must never call
`validateDealbreakers`.

- [ ] **Step 5: Put Stage 1B before Stage 2 in both paid flows**

During scrape, run the focused validator after Stage 1A and before the final score merge and
automatic Stage 2 selection.

During user-triggered `GET /api/rerate`, load all candidate postings, including rows currently
stored as `Total = -1`; run Stage 1B, re-score all postings, rebuild the eligible `Today` set, then
run the existing Stage 2 work. Share the operation's `aiBudget` and `callCap` so Stage 1B cannot
bypass existing spend controls.

- [ ] **Step 6: Make profile changes cache-aware and provider-free**

When saving a profile, compare `profile.DealbreakerInputHash` values. Commit the profile, then call
only cache-backed `scoreAll`. Treat a changed hash or a missing current validation as pending
`AI 평가` work in `rerateInfo`; do not call the provider during the request.

Startup follows the same cache-only rule. Scheduled scrape retains the existing sole-owner policy:
exactly one owner resolves one runtime; zero or multiple owners skip paid AI and record the existing
operator error.

- [ ] **Step 7: Run server integration and race tests**

```sh
JOBCRON_TEST_POSTGRES_URL="$DATABASE_URL" \
  go test ./internal/server \
  -run 'AIRuntime|Dealbreaker|Scrape|Rerate|Profile|Startup|Scheduler|UserScope' \
  -count=1
JOBCRON_TEST_POSTGRES_URL="$DATABASE_URL" \
  go test -race ./internal/server -count=1
```

Expected: scrape order, cache hits, conservative fallback, per-user isolation, re-entry into
`Today`, provider-free save/startup, scheduler, and existing Stage 2 behavior all pass.

- [ ] **Step 8: Commit Task 5**

```sh
git add internal/server
git diff --cached --check
git commit -m "feat(server): run contextual validation before scoring"
```

### Task 6: Render the excluded-card trust surface

**Files:**

- Create: `internal/server/exclusion_reason.go`
- Create: `internal/server/exclusion_reason_test.go`
- Create: `web/exclusion_reason.html`
- Modify: `internal/server/handlers.go`
- Modify: `internal/server/archive.go`
- Modify: `internal/server/server_test.go`
- Modify: `internal/server/archive_test.go`
- Modify: `internal/server/ai_injection_test.go`
- Modify: `web/index.html`
- Modify: `web/archive.html`
- Modify: `web/styles.css`
- Modify: `docs/assets/screenshots/dashboard.png`
- Modify: `docs/assets/screenshots/dashboard-dark.png`

**Interfaces:**

- Consumes `ScoreResult.ExclusionReasons` from Tasks 4 and 5.
- Produces one shared template partial for daily and archive excluded lists.
- Does not query the Stage 1B table or recalculate scoring policy while rendering.

- [ ] **Step 1: Write failing view-model and escaping tests**

Add these cases:

```go
func TestExclusionReasonViewShowsEveryReasonInOrder(t *testing.T)
func TestExclusionReasonViewSplitsMarkedKeywordWithoutHTML(t *testing.T)
func TestExcludedReasonEscapesProviderOutput(t *testing.T)
func TestDailyAndArchiveRenderExclusionReasons(t *testing.T)
```

Use evidence containing `<script>alert(1)</script>` and assert it renders as escaped text. Verify
the matched keyword alone is wrapped in `<mark>`, while all surrounding segments remain escaped.
Do not use `template.HTML`.

- [ ] **Step 2: Build the narrow view model**

Use plain strings and pre-split segments:

```go
type exclusionTextSegment struct {
    Text   string
    Marked bool
}

type exclusionReasonView struct {
    Label      string
    Status     string
    Evidence   []exclusionTextSegment
    HasEvidence bool
}
```

Map confidence to visible Korean text:

```text
confirmed    -> AI 문맥 확인
uncertain    -> AI 문맥 확인 불확실
unverified   -> 규칙 기반 · AI 문맥 확인 없음
deterministic -> 규칙 기반
```

Add `ExclusionReasons []exclusionReasonView` to `dashboardPosting`. Populate it only by
unmarshalling `scores.breakdown_json`; do not read `ai_dealbreaker_validations` during rendering.

- [ ] **Step 3: Add and reuse the template partial**

Define one `exclusion-reasons` template in `web/exclusion_reason.html` and call it from excluded
rows in `web/index.html` and `web/archive.html`. Each row must show the `제외 이유` heading, reason
label, optional verbatim evidence, and visible status text. Keep disclosure summaries, bookmark,
mute, source, deadline, and outbound-link markup unchanged.

- [ ] **Step 4: Add semantic danger styling and restore row contrast**

Add light/dark `--danger`, `--danger-soft`, and `--danger-border` tokens. Style the reason panel,
quoted evidence, and `<mark>` with these tokens, but retain text and an accessible warning symbol
so color is not the only signal. Remove this rule:

```css
.excluded-box .posting { opacity: 0.5; }
```

Keep the panel readable without horizontal scrolling or clipped evidence at desktop and mobile
widths.

- [ ] **Step 5: Run rendering tests**

```sh
gofmt -w internal/server/exclusion_reason.go \
  internal/server/exclusion_reason_test.go
go test ./internal/server \
  -run 'ExclusionReason|ExcludedReason|DailyAndArchive|Injection' -count=1
```

Expected: reason order, Korean status copy, HTML escaping, shared rendering, and adjacent actions
pass.

- [ ] **Step 6: Perform Tier C browser QA with `/browse`**

Start or reuse a no-open PostgreSQL-backed preview. Use the `frontend-qa` skill and gstack
`/browse`, never the user's default browser. At desktop and mobile widths, in light and dark
themes:

1. open the daily and archive pages;
2. expand `관심 밖으로 분류된 공고`;
3. inspect confirmed, uncertain, unverified, education, career, and MinScore reasons;
4. toggle bookmark and mute controls;
5. open one outbound posting and verify its unique title or identifier at the destination;
6. confirm disclosure toggling and adjacent scored cards still work; and
7. verify no console errors, clipping, horizontal scrolling, or contrast loss.

Update the sanitized README screenshots at exactly `2716x1720` pixels. Do not capture private
profile data, credentials, production identifiers, or authentication material.

- [ ] **Step 7: Commit Task 6**

```sh
git add internal/server web docs/assets/screenshots
git diff --cached --check
gitleaks git --staged --redact --no-banner
git commit -m "feat(web): explain every excluded posting"
```

### Task 7: Prove the complete feature and close the documentation lifecycle

**Files:**

- Modify: `docs/architecture.md`
- Modify: `docs/superpowers/README.md`
- Move after all gates pass:
  `docs/superpowers/specs/260718-stage-1-contextual-dealbreaker-validation-and-exclusion-evidence.md`
- Move after all gates pass:
  `docs/superpowers/plans/260718-stage-1-contextual-dealbreaker-validation-and-exclusion-evidence.md`

**Interfaces:**

- Consumes every prior task on one exact candidate commit.
- Produces verified architecture documentation and archived completed implementation knowledge.

- [ ] **Step 1: Run the full local verification matrix**

```sh
test -z "$(gofmt -l .)"
JOBCRON_TEST_POSTGRES_URL="$DATABASE_URL" go test ./... -count=1
JOBCRON_TEST_POSTGRES_URL="$DATABASE_URL" go test -race ./... -count=1
go vet ./...
go build ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -o /tmp/jobcron-linux-amd64 ./cmd/jobcron
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
  go build -o /tmp/jobcron-linux-arm64 ./cmd/jobcron
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 \
  go build -o /tmp/jobcron-darwin-arm64 ./cmd/jobcron
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go build -o /tmp/jobcron-windows-amd64.exe ./cmd/jobcron
```

Expected: all unit, PostgreSQL integration, race, vet, build, and cross-build gates pass. Confirm
there is no `0013_contextual_*` SQLite migration and no Stage 1B SQLite branch.

- [ ] **Step 2: Run the explicit live-provider gate**

Only with an explicitly configured disposable test credential, run two synthetic postings end to
end:

1. `리서치 아님` returns citation-gated `not_applicable`, re-enters `Today`, and spends from the
   disposable user's ledger; and
2. an actual research responsibility returns `applies`, stays excluded, and shows the exact quote.

Verify the cache-hit rerun makes zero provider calls and spends zero tokens. Do not print the key,
prompt, response, connection string, or raw production data.

- [ ] **Step 3: Re-run the complete browser journey on the candidate commit**

Repeat Task 6's Tier C browser matrix against the exact verified commit. Include daily, archive,
bookmarks, hidden postings, profile save, manual scrape, `AI 평가`, light/dark, desktop/mobile,
outbound destinations, and console errors. The user-facing path, not `curl`, is the acceptance
gate.

- [ ] **Step 4: Update the architecture document**

Document:

- Stage 1A global eligibility extraction and its separate evidence/version;
- deterministic token-exact candidate generation;
- user-scoped Stage 1B validation and PostgreSQL cache identity;
- conservative fallback and paid-call boundaries;
- persisted `ScoreResult.ExclusionReasons` and the shared trust surface; and
- the frozen SQLite importer boundary and its eventual removal after rollback closes.

Link to the active spec and plan until the archive move, then update links to their final archived
paths.

- [ ] **Step 5: Archive the completed spec and plan**

Create one dated workstream directory:

```text
docs/superpowers/archive/2026-07-18-contextual-dealbreaker-validation/
```

Move the spec and this plan into it only after every implementation and verification gate passes.
Remove them from Active Work and add one concise Recently Archived entry in
`docs/superpowers/README.md`. Do not copy raw logs, secrets, or unsanitized screenshots into Git.

- [ ] **Step 6: Run the documentation publication gate and commit**

```sh
git diff --check
awk '/^[[:space:]]*\|/ && length($0) > 100 { \
  print FNR ":" length($0) ":" $0 \
}' \
  docs/architecture.md docs/superpowers/README.md \
  docs/superpowers/archive/2026-07-18-contextual-dealbreaker-validation/*.md
git add docs
git diff --cached --check
git diff --cached
gitleaks git --staged --redact --no-banner
git commit -m "docs: record contextual exclusion architecture"
```

Expected: no over-100-column Markdown table rows, secrets, private data, stale active links, or
publication-safety findings.

- [ ] **Step 7: Inspect the cumulative candidate**

```sh
BASE_SHA=$(cat \
  .superpowers/sdd/260718-contextual-dealbreakers/implementation-base.txt)
git status --short
git log --oneline --decorate -8
git diff "$BASE_SHA"...HEAD --stat
git diff "$BASE_SHA"...HEAD --check
gitleaks git --redact --no-banner
```

Review the complete diff for scope, user isolation, fallback truthfulness, paid-call placement, UI
escaping, legacy import preservation, and unrelated changes. Do not push.
