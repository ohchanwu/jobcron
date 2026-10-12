# AI Rating and Live Posting Updates — Task-Level Implementation Plan

Created: 2026-10-12. Status: implementation-plan documentation awaiting independent
PLAN-READINESS review. **DESIGN APPROVED; APPLICATION IMPLEMENTATION ON HOLD.**
The owner instructed: “Approved. Write the impl plan. Don’t start impl.”
This document does not authorize coding, feature/test scaffolding, dispatch,
runtime experiments, paid inference, publication or deployment.

Governing [campaign specification](../specs/261012-ai-rating-live-updates-campaign.md)
was independently design-ready in review 169 and owner design-approved at
`119fd7577ebd9b8134512654d0ed50dac2922827`, tree
`28ec52d90466438ed599c9c3f060357bc12d0b06`. That is this plan's source-grounding
baseline; the specification's earlier discovery baseline remains historical.
This replaces the phased summary at the same path, not a second competing plan.
The [narrow Gemini spec](../specs/261009-gemini-pacing-and-quota-classification.md)
and [experiment reference](261009-gemini-pacing-and-quota-classification-plan.md)
retain their original 2026-10-09 creation dates and substantive constraints.

## Authority, sequencing and future dispatch

All T01–T14 below are logical future tasks in plan text, NOT Kanban cards. None
is started. After a future explicit owner start instruction, execute two ordered
local payloads: T01–T04 Gemini FIRST; independently accept/integrate its exact code;
then T05–T14 live UI SECOND under applicable coding authority. Obtain separate
exact-code independent acceptance for each slice. An optional paid/live Gemini
experiment is not a dependency of UI work. At most one separately authorized
combined release may follow both local acceptances; no intermediate release,
launch/migration repetition or old runner rehearsal is required.

Future controller dispatch must:

- Use board/tenant `jobcron`, the Jobcron repository, isolated pinned worktrees,
  explicit allowed paths, full clean baseline/tree and exact new candidate/tree,
  with `local-only` completion. Check current cards for duplicates/collisions.
- Select only `jobcron-worker`, `jobcron-worker-glm-1` or `jobcron-worker-glm-2`;
  reviewer is `jobcron-reviewer` only. Recommend one primary `jobcron-worker` for
  each whole slice because server/projection/client seams overlap. A second pool
  worker is useful only for a genuinely independent, disjoint bounded task, not
  to occupy workers. No legacy profiles or anonymous coding delegation.
- Preflight the actual assigned profile/model, inherited task/domain skills and
  automatic SDLC skill resolution on the actual card. Record real process PID,
  start and cwd receipts after normal claim, not merely desired configuration.
  Stop on profile/tenant/workspace/baseline mismatch or unavailable capability.
- Size run/subprocess limits from whole measured work, including fixture startup,
  full suites, browser checks, review and repairs; retain rejected candidates and
  all repair history. A limit is not a campaign deadline or authorization expiry.
  Do not invent autonomous scope, budgets or owner approvals now.
- Keep reviewer corrections on the same slice card. Flag shared-file hotspots
  before concurrent work. Exact approval permits controller local integration,
  never worker push/PR/cloud/credential/production effects.

Serial dependency spine: T01/T02 → T03 → T04 → T05 → T07 → T08 → T09 → T10 →
T11 → T12 → T13 → T14. T06 may follow T04 independently of T05; both T05/T06
must finish before T07. T01 and T02 can be studied in parallel, but both touch
AI tests; prefer serial implementation by the primary. T08 and T09 share writer
contracts and must not race each other. Client work starts only after endpoint/
fragment contracts settle. No dispatch is authorized by this recommendation.

## Native grounding and proposed boundaries

Paths/functions in this table exist at the baseline. Names explicitly labeled
“proposed” later are implementation suggestions, not available APIs/tests/routes.

| Existing scope | Relevant facts and task |
| --- | --- |
| `internal/ai/provider.go`, `client.go`, `openai_compatible.go`; `internal/pacing/pacing.go` | `SuggestedRateLimit` is uniformly 1.2s; `httpProvider.complete` calls shared `Pacer.Wait` then HTTP; Gemini uses the compatible chat-completions spec. Native envelope support here means error parsing, NOT adding a native inference API. T01–T03 |
| `internal/server/rerate.go` | `providerErrorSignals`/`providerFailureMessage` conflate generic quota and retry markers; `rerateActiveStatus` promises 5–10 minutes. `rateStage2` local result lacks posting/commit identity; `rerateOne` persists before returning. T01, T03, T09 |
| `server.go:1027–1163`, `internal/scoring/engine.go` | `scoreAll` composes cached extraction/validation/deltas, then writes scores. Pure `scoring.Score` already exists, but cache selection is not a shared pure read API. T05 |
| `server.go:352–391`, `handlers.go:278–367`, `archive.go:125–246`, `bookmarks.go:48–99` | Current builders discard absent/other-profile materialized scores; runtime render helper decrypts credentials. T06/T07 must replace this read path, not call `scoreAll` from snapshots |
| `internal/storage/profile.go`, `ai_extractions.go`, `ai_scores.go`, `ai_score_outcomes.go`, `ai_dealbreakers.go`, `postings.go`, `bookmarks.go`, `not_interested.go` | Concrete Store uses unscoped DB reads; narrow `queryExecer` lacks multirow reads. Factor transaction-bound reads without a repository interface. T07 |
| `internal/server/auth.go`, `internal/storage/sessions.go`, `ai_credentials.go` | `requireAuth`/`stateUserID` use `UserBySessionToken`, which updates last_seen_at. Credential loader returns ciphertext/nonce. Neither is an observation helper. T06 |
| `server.go:540–689,825–964`, `dedup.go`, `scheduler.go:71–187`, `rerate.go:165–258` | Shared collection, sponsor Stage-1A, per-user Stage-1B/Stage-2, detached/manual and scheduled noop emit paths require hooks below request emitters. T08/T09 |
| `handlers.go`, `bookmarks.go`, `not_interested.go`, `account.go`, `auth.go` | Profile/credential, bookmark/mute and account/session mutations are other writers. T08 |
| `rerate_status.go`, `web/ai-rerate.js`, `_rerate.html` | User/surface tracker has epoch-qualified run tokens; client has late-owner guards but calls `rememberAndReload`. Progress is not a stored-view revision. T08/T12 |
| `web/index.html`, `archive.html`, `bookmarks.html`, `_chips.html`, `exclusion_reason.html`, `embed.go` | Existing escaped shells/subtemplates; use small shared keyed fragments. T10 |
| `web/source-filter.js`, `bookmark.js`, `not-interested.js`, `ai-popover.js`, `styles.css` | Filters capture cards/counts once; explicit search autoopens excluded details; mutators and popovers are delegated. T11/T12 |

Verify these seams again at the future coding baseline. Changes to architecture,
locked behavior or supported full-surface approach require a design finding,
not silent widening of this plan. No schema, dependency, broker, framework,
durable event log, distributed limiter, new scheduler or token streaming is needed.

## Task execution/evidence convention

Every future task starts with focused RED tests against the actual existing seam
(or a proposed seam introduced in that task), implements the smallest GREEN
change, then refactors only after those assertions pass. Proposed test names are
illustrative, not existing commands. Prefer synthetic in-code tables and the
existing stub/owned Store fixtures; no real keys, raw errors or private JDs.
Run the focused packages after refactor and retain RED/GREEN output, source diff,
fixture identity, elapsed time and failure history. A task's checkpoint is a
locally passing bounded change, not release permission; only T04/T14 are whole-
slice independent acceptance gates. On failure, stop subsequent tasks and repair
the local candidate. Local rollback means a new local correction/revert retaining
evidence, not shared-history rewrite, production rollback or DB/cache deletion.

## Slice 1 — Gemini pacing and quota

### T01 — bounded structured classification (no new retry behavior)

Dependency: future owner coding approval. Purpose: accurate explanatory categories
without raw-body leakage or provider contract crossover.

Scope: existing `internal/ai/client.go` `APIError`/`complete`, compatible spec;
`internal/server/rerate.go` error helpers and `ai_provider_switch_test.go`.
Propose a small `internal/ai/quota_error.go` typed classifier and associated unit
test, plus table-driven server copy tests. APIError may carry only validated retry
metadata from response headers; do not retain/export all headers. Do not change
HTTP request, prompt, success parsing, provider registry/model or score versions.

RED fixture matrix (synthetic native `error.status/details` and supported compatible
`error` envelopes):

- Explicit minute request metric/ID and explicit minute token metric/ID, with/without
  well-formed RetryInfo and Retry-After. Waiting MAY help; no exact safe-delay promise.
- Explicit daily metric/ID with no retry hint and with RetryInfo/header; daily remains
  persistent. Minute+daily and conflicting recognized metric/ID scopes become mixed,
  never transient merely because a retry delay is present.
- Explicit billing/account prerequisite versus rate restriction; key/model/region
  400/401/403/404 retain their distinctions. Non-Gemini Anthropic/OpenAI billing/rate
  fixtures keep their own contract even with Gemini-looking detail strings.
- Generic QuotaFailure/quota text/RESOURCE_EXHAUSTED and unscoped 429 stay unknown;
  unknown metric, unsupported wrapper, missing/wrong types and malformed JSON do
  not become inferred RPM/RPD. RetryInfo alone cannot establish quota scope.
- Truncated/oversized body, excessive nesting/detail count, long fields, duplicate
  or contradictory fields, malformed/negative/overflow retry durations and headers,
  5xx/timeout/transport/parse errors. All finish bounded with safe fallback.
- Synthetic canaries in messages/URLs/metadata/project/key/JD/goal fields never appear
  in diagnostics, status, stored outcome or logs introduced by this slice.

GREEN: parse closed supported shapes, a bounded metric/ID scope allowlist and
validated retry delay into enums, not arbitrary text. Bound body traversal and
field/detail/depth work explicitly (choose constants alongside adversarial tests;
respect the existing 1MiB transport bound and reject truncated classification input).
Unknown shape fails safe. Preserve daily/billing restrictions when retry hints
conflict; explanatory classification grants no retries or successful cache rows.
Refactor duplicate parsing only within these helpers. Evidence: matrix results,
bounded-input tests and allowlisted output/canary audit. Exit: every category and
provider isolation passes; checkpoint/revert classifier only if not.

### T02 — enforce actual Gemini request START spacing

Dependency: future coding approval; integrate with T01 before T03. Purpose: Gemini
minimum 10s starts without serializing response latency or changing other pacing.

Scope: `internal/ai/provider.go` `SuggestedRateLimit`, `client.go` `complete`,
`internal/pacing/pacing.go` `Pacer.Wait`, existing `provider_test.go`, `client_test.go`,
`openai_compatible_test.go`, `internal/pacing/pacing_test.go`. Propose a narrow
controlled-clock/timer seam or use Go's available deterministic time testing;
no general simulation framework. Other scraper users of Pacer must be checked
before changing its implementation; do not silently alter their intended spacing.

RED: duration-selection tests fail the old uniform Gemini value; record actual
transport starts for concurrent Extract/ScoreDelta calls sharing ONE provider/
pacer, not just Wait reservations. Advance controlled time: no second Gemini
start before 10s; several queued workers start >=10s apart; first response remains
blocked while a later request starts after its interval (latency overlap). Delayed
wakeup/clock scheduling must not release overdue reservations as a simultaneous
burst. Test cancellation while waiting, cancellation before the first call and
at the start boundary: canceled requests never reach transport and waits release
resources. Cancellation does not prove remote non-submission after transport begins.
Keep non-Gemini actual 1.2s start behavior and no-pacing synthetic transports;
regress the existing scraper/pacing consumers with short/controlled tests.

GREEN: provider-specific suggested duration; enforce the minimum at actual request
admission rather than assuming future reservations prove actual start spacing.
Keep provider response waits outside limiter locks and retain pool size. Test any
necessary Pacer fix against ALL its callers; canceled reservations must not permit
a burst or leak waiters. Do not add retry/backoff, a process/project-wide singleton
or distributed quota guarantee. Scope is the existing provider instance/pacer,
including operations sharing it, not unrelated runtimes/projects/clients. Evidence:
timestamped synthetic transport trace and cancellation/overlap/provider regression
results. Exit: real start trace satisfies minimum under controlled delayed wakeup;
if scope/clock feasibility fails, stop for review, not a long real-sleep workaround.

### T03 — wire extraction/Stage-2 and calm copy without identity churn

Dependency: T01/T02. Scope: `server.go` runtime factory/resolveStage1Funding/
extractStage1, `stage1_sponsor_test.go`, `rerate.go` preparation/validation/Stage-2/error/progress copy,
`web/ai-rerate.js`, `_rerate.html`, current AI/server/web tests. Propose focused
wiring tests, not a live provider fixture.

RED: factory spies verify Gemini duration at sponsor Stage-1A and user Stage-1B/
Stage-2 resolution; compatible Extract/ScoreDelta hit the same pacer. Frozen
successful current/no-signal caches spend zero; pacing changes do not rotate
ExtractionContractVersion, DealbreakerVersion, ScoreVersion or AIInputHash. Preserve
source/profile/payer separation, shared singleflight, per-call/run/daily/monthly
ceilings, admission and automatic rejection skip/manual later retry behavior.
Test classified failures do not become no-signal or fresh successes. Copy tests
reject obsolete fixed 5–10-minute/coffee-break claims for slowed AI work and
confusing source-scrape 1/s with AI pacing. Preserve phase-specific numeric progress
from 0/N before the first blocking call; failed/rejected versus skipped versus
fresh-success counts remain distinct.

GREEN: wire through existing runtime factory (no prompt/model change), expose calm
minute/daily/mixed/billing/unknown explanations and bounded enum/retry metadata
only. State work may take longer depending on postings/provider, not a completion
time or quota-safe promise. Do not change unrelated UX. Evidence: wiring/cache/
admission spy tables, controlled browser copy smoke with synthetic results, exact
unchanged prompt/version/model and complete migration manifests. Exit: no extra
requests on cache hits or read/recovery paths; checkpoint before T04.

### T04 — independent exact-code acceptance of Slice 1

Dependency: T01–T03. Run the focused and broad future verification below, affected
copy browser smoke, diff/privacy/links and complete staged-diff scanner. Commit
passing local code; same-card `jobcron-reviewer` independently checks full baseline,
candidate/tree, real start trace, categories, wiring/cache/budget semantics and
unchanged manifests. Ordinary findings repair the same card, preserving history.
Controller integrates only the exact independently approved candidate. Exit:
Slice 1 locally accepted; no quota-safety/model-quality/release claim. This is the
checkpoint required before Slice 2, NOT a paid experiment dependency.

## Slice 2 — authoritative stored-state live UI

### T05 — common pure score composition and outcome policy

Dependency: T04 plus applicable owner start authority. Scope: `server.go` scoreAll,
`internal/scoring/engine.go`, existing `ai_cache_test.go`, `ai_provider_switch_test.go`,
`rating_outcome_test.go`, `rating_cards_test.go`, `unscored_test.go`.
Propose `internal/server/score_projection.go` and `composeStoredScore` with explicit
immutable inputs; no Store/provider/runtime access inside the pure function.

RED: compare old scoreAll composition and proposed composition over source posting,
current profile, matching-content global extraction, current-content/candidate
user validation, fresh delta, latest current-version and supported cross-version
fallback. Assert extraction/source newcomer preference, contextual exclusion,
citation/evidence/clamp/signed lines, hard exclusion short-circuit and deterministic
stable output. Include missing materialized score and other-profile stale score:
derive real rule result, never zero/another-profile total. AI disabled/no profile,
supported saved stale evidence, goal/provider/model changes and existing demo cache
policy need explicit cases. Missing current outcome provenance stays unknown.

Rated includes accepted net-zero/partial evidence; genuine current empty success
supersedes old delta and counts fresh without an invented scored chip. Rejected/
failed bounded saved outcome adds no new adjustment and retains supported faded
evidence; unadmitted/skipped has no new outcome. Hard-excluded rows have no AI line.
Do not label absence of a current result queued: use `아직 AI 분석 전` where the
current outcome is absent, with supported old evidence separately faded and known
failed/rejected/no-signal copy taking precedence. No completion promise.

GREEN: extract cache selection/composition, call it from scoreAll writers and later
read projection; leave scoring math and cache identity untouched. Existing missing-
score omission tests must be deliberately rewritten in T07 to require pure computed
parity, not deleted as inconvenient. Evidence: deterministic equivalence tables,
fresh/stale/outcome matrix and zero-side-effect spies. Exit: composition is pure,
all semantic cases locked; checkpoint before read architecture integration.

### T06 — SELECT-only exact-session and AI-presence metadata

Dependency: T04; may be developed separately from T05 in disjoint files. Scope:
`internal/storage/sessions.go`, `ai_credentials.go`, `internal/server/auth.go`,
`handlers.go` middleware, account/session tests and `production_user_scope_test.go`.
Propose `SessionIdentityReadOnly` (return minimum identity/expiry, no email/password)
and `HasUserAICredential` (`SELECT EXISTS`), plus `requireObservationAuth`.

RED: production cookie hash/expiry/existing-user predicates match current session
semantics for valid, empty, expired, revoked, deleted-user and wrong-user sessions.
Trace middleware+handler+heartbeat SQL: no UPDATE last_seen_at, INSERT, DELETE,
session extension or other writes. Return API 401 rather than login HTML; reject
demo observation, foreign origin/user selector and credential URL/CORS bypass.
Fixed SQLite local user and explicit PostgreSQL local user expose only their
identity. Metadata query never reads ciphertext/nonce; fail credential decryptor,
provider factory and mutating session helpers if called by observation.

GREEN: route the TWO proposed observer endpoints around the mutating requireAuth
path without bypassing authentication/CSRF policy for other routes. Credential
presence/configuration supplies enabled metadata; derive default model/nonsecret
contract versions via existing `ai` functions. Presence is NOT usable-key/provider
readiness. Never manufacture an AIRuntime for rendering/counts. Evidence: query
traces, unchanged session rows, decrypt/factory counters and two-user/401 tests on
owned PostgreSQL; SQLite local compatibility tests. Exit: no secret resolution or
write on full read path; metadata read errors remain unavailable, not “AI off” guesses.

### T07 — transaction-consistent read projection shared by initial pages

Dependency: T05/T06. Scope: concrete `internal/storage` reads listed above,
`store.go`, `internal/server/server.go` render helpers, all three builders and
`rerate.go` buildRerateInfo. Propose narrow `internal/storage/posting_view.go`
transaction-bound read bundle, and `internal/server/posting_view.go` pure surface
projection/metadata-only rerate-info builder. Do not add a repository interface.

RED: one coherent view includes profile/hash/config/presence, postings/content,
global extraction, this user's validations/deltas/outcomes/bookmarks/mutes,
canonical duplicate/source metadata and current time. Force a writer between
SELECTs; neither PostgreSQL nor SQLite may return a mixed before/after bundle.
Initially use a test barrier; T08 installs real commit/read coordination. Test
rollback/cancellation/read deadline and read-query errors: unavailable whole
snapshot/counts, never guessed fresh N=0 or partial success. Query shape is batched
and bounded, not per-card runtime/decryption or unbounded N+1 work.

GREEN: PostgreSQL read-only repeatable-read transaction; SQLite explicit read
transaction. `BeginTx` alone is NOT snapshot establishment: execute a real first
read under the revision barrier before capture/release. Thread the SAME tx through
all reads, with dialect-specific binding; load data and end transaction before
templates/network output. Compute scores using T05 even absent/stale materialized
rows. Initial `/briefing`, `/bookmarks`, `/` use the same projection as observer;
no navigation can revert a newly saved result. Keep `/archive` redirect. Writers
may continue materializing scores; reads never call scoreAll/RescoreUsers/runtime.
Do not trigger startup cache recovery from observe/resync.

Preserve surface policy: Today first-seen current KST day, nonexpired canonical
enabled/unmuted rows, stable score-descending and existing briefingCap; bookmark
soft-MinScore exemption not hard exclusion. Archive canonical enabled/unmuted,
expired rows excluded, date groups newest first with stable descending score within
day; score mode flat descending with existing stable ties. Excluded order remains
builder order. Bookmarks retains saved-at order, disabled-source/expired/muted
saved rows and hard-exclusion marking. No new Today/bookmarks sort selector.
Test cap removal/insertion backfill, canonical sibling replacement, source/mute/
expiry/minScore/zero-empty states. Current-view fresh N/M is metadata from this
view, not frozen run progress or the client quick search/source selection.

Evidence: both-dialect coherent-read interleavings, pure missing/stale-score page
parity and golden membership/order/outcome tables. Exit: all initial/read views
agree, zero side effects and truthful error policy; preserve no-profile guidance
and public demo behavior without enabling private observation.

### T08 — scoped revisions, short commit/read barrier and writer coverage

Dependency: T07; integrate serially with T09. Scope: propose small
`internal/server/posting_updates.go` hub/barrier, existing global/user writers,
profile/key-delete/bookmark/mute/account/logout handlers, scheduler and run tracker.
No persistent revision schema or event-history product.

RED: gate a transaction before commit, between commit/publication and at snapshot
first SELECT. Assert no revision labels old data as new, and committed rows cannot
hide until a provider completes. Block provider/slow listener/renderer and prove
writers are not held by network/DOM work. Cancellation/failed write/rollback emits
no saved-change revision. An unchanged cache hit has no flash/pretend-new result.
Progress does not advance durable revisions. Two users see only their own changes.

GREEN lock protocol: relevant writer takes global barrier before user barrier
(global mutation uses global exclusive; user mutation holds global read then that
user exclusive), performs only bounded DB mutation/commit, advances matching
revision and enqueues scoped dirty notice before release. Acquire BEFORE the
commit-sensitive write, not after commit (otherwise a reader can see an unpublished
write). Snapshot acquires global read then user read, starts transaction and first
actual DB read, captures epoch/global/user/context, then releases barriers; remaining
SELECTs use that established snapshot. Release tx before rendering/transfer. Never
hold these locks over provider calls, budget sleeps or whole runs. Existing
singleflight is acquired BEFORE view barriers; never acquire flight/budget/tracker
locks while holding the view barrier. Copy run metadata independently using its
own lock; enqueue must be nonblocking, with no callback/network write under locks.
Document all acquisition paths and test deadlock/race stress on both dialects.

Maintain new epoch per process, monotonic global/user revisions only within it;
derive opaque context covering current profile/goal/provider/model/score and
dealbreaker contracts/presence. Context-change notices carry new token. No raw
goal/profile hashes as diagnostics. Active listener/run maps are bounded/evicted;
eviction/re-registration must force a fresh baseline without rolling a user's
revision backward within an epoch. Test this alongside idle-state cleanup.
Uninstrumented external/CLI writes are NOT covered by these process counters;
periodic forced stored-state resync and exact-session rechecks handle them. Do not
claim distributed coherence or old epoch run-number ordering.

Required writer ledger (audit definitions AND usages; no request-emitter-only hook):

| Writer seam | Scope, publication and test |
| --- | --- |
| runScrapeSource UpsertPosting new/seen; RefreshPostingDetail | Global public dirty after successful actual write, BEFORE extractStage1. Block extraction and observe new rule-scored card now; seen no meaningful view change need not flash |
| extractStage1 UpsertAIExtraction, including preparation/sponsor backfill | Global current-content dirty only on successful write; best-effort failure publishes nothing and regex remains truthful; no sponsor/payer metadata in global event |
| collectPostings SweepStalePostings; dedup.go markCrossPortalDuplicates | Global removal/canonical change after each successful mutation; update sibling sources and cap backfill; failed source sweep policy unchanged |
| validateDealbreakers persisted Stage-1B checks | Only analyzed user's dirty notice, including scheduled/manual paths; failed persistence cannot claim resolved exclusion |
| scoreAll UpsertScore/UpsertScoreForUser and cache-only recovery | Meaningful user stored-view changes only; pure reads do not wait for terminal full rescore or ComputedAt-only churn |
| T09 Stage-2 UpsertAIResult/UpsertAIScoreFailure | Only that user's committed result/outcome; same behavior with noop emit, detached/manual/scrape/scheduled callers |
| handleProfileSave/SaveProfileAndCredentialForUser; handleAIKeyDelete | Context invalidation immediately after profile/credential commit, BEFORE optional best-effort rescore; old responses become inadmissible; failure leaves old context |
| bookmark/mute PUT/DELETE handlers | Only that user's membership changes; retain idempotent saved ordering; failed mutation no revision; mutation replies cannot undo newer snapshots |
| handleLogout; account password/delete handlers and session revocation | Close exact revoked-session listeners after commit; password keeps current session per existing contract, deletion all relevant listeners. Missed/external revocation fails next SELECT-only validity check |
| rerate tracker; scrape/analyzeUserCollection; runScheduledScrapeWithHistory/noopSchedulerEmit | Bounded user-specific progress/terminal/storage-warning channel separate from durable revisions. Global collection notices contain no other user's paid progress; scheduled analysis status recorded below noop emit |

Evidence: completed caller ledger, commit timing traces, lock-order tests, scoped
event/privacy tests. Exit: every listed writer has a success/failure assertion;
if any scope cannot be instrumented safely, stop acceptance instead of relying
only on foreground SSE or production observation.

### T09 — explicit committed worker outcomes and frozen admission

Dependency: T08; scope `rerate.go` rateStage2/rerateOne/validateDealbreakers,
`server.go` analyzeUserCollection/extractStage1, `scheduler.go`, storage atomic
outcome helpers and existing rating_storage_failure/rating_outcome/ai_rerate tests.
Propose explicit worker result fields for posting ID, analyzed user, goal/version
identity, cached/provider-called, committed-change and committed outcome; use a
closed status instead of inferring commit from boolean combinations. Keep old
context identity internal; current cards always come from the T07 projection.

RED: block all but one synthetic worker; release one rated/no-signal/rejected/failed
reply at a time and observe durable row/outcome before run terminal, also with noop
emit. Inject atomic result+outcome write failure and failed/rejected-outcome write
failure: no success/fresh increment, no saved-outcome notice, no half-commit. Partial
siblings update even when another row fails or run cancels. Storage error is a run
warning, not provider success or a saved failed card; automatic/scheduled callers
must not pass storage error through providerFailureMessage as if provider failed.
Global Stage-1A failed write likewise publishes nothing.

Freeze existing admission lists at their current boundaries: preparation candidates;
surface-first then global unique Stage-1B cohort; unique visible Stage-2 list AFTER
preparation/validation/rule merge. Do not freeze Stage-2 too early. Live display
sort/search/source changes, threshold moves, cross-tab changes and new arrivals
never append/reorder the active provider queue or raise cap. Record exact admitted
IDs/phase targets and spy calls, budgets and skip/processed counts for manual three-
surface, scrape auto-rate and scheduled paths. Cached rated/empty spend zero;
automatic rejected skip stays, later deliberate manual retry stays, no new in-run
retry. Failed/rejected calls count processed, not cap skips. Quick filters remain
display-only; do not silently change provider admission semantics. Show current
view fresh N/M separately from run phase processed/target/skipped.

GREEN: advance revision/publish at the persistence boundary inside T08's bounded
commit/publication window, then return explicit committed metadata to the aggregator;
do not defer publication until result-channel consumption. Keep request ResponseWriter
on its owning goroutine.
No duplicate publication or raw provider delta event. Separate persistence warning
from definite committed failed outcome across all callers. Evidence: blocked-run
timelines, injected SQL failure/rollback checks, exact-call cohort ledger. Exit:
all outcomes before terminal with failed-save exclusion and unchanged spend/admission.

### T10 — bounded read-only SSE/snapshot routes and escaped keyed fragments

Dependency: T06–T09. Scope `handlers.go` registration, `auth.go`, `sse.go`, proposed
posting_updates/posting_view handlers, `web/embed.go`, three shells plus shared
chip/evidence templates. Propose `web/_posting.html` small keyed shell/regions,
not model HTML or full-page replacement.

Proposed contract (not routes available today):

- `GET /api/posting-updates`: same-origin authenticated cookie/local identity;
  bounded JSON SSE dirty/resync/context notices with epoch/global/user revisions,
  optionally public posting IDs. User progress carries run token/phase sequence
  separately. No JD/goal/errors/credential/other-user data. Subscribe/register
  before initial read; no durable replay even when Last-Event-ID is supplied.
- `GET /api/posting-view?surface=today|bookmarks|archive&sort=...&generation=...`:
  closed surface/sort/generation validation; archive sort explicit current page
  date/score (not a different tab's cookie), fixed Today/bookmark policy. JSON
  envelope has schema discriminator, epoch/global/user revisions, opaque context,
  echoed request generation, surface/effective sort, ordered keyed fragments,
  section/KST-day membership, counts/empty states and scoped run summaries. Tokens
  and generation are consistency guards, NEVER authorization/user selectors.

RED: 401/demonstration rejection, no-store/private, escaping hostile synthetic
title/URL/quotes/evidence, no template.HTML coercion, complete keys/counts/empty
sections and initial-page parity. SSE flush/no compression/anti-buffer headers;
single-line writer sends JSON only, not multiline fragments. Exact-session check
before each private emission/read and heartbeat, recheck before sending after a
lengthy read; revoke during read/transfer preparation and ensure no new private
fragment emits. Clear private content on auth loss in T12.

Implement approved bounds: one observer per visible page; 8 listeners/user and
64 total; one pending dirty slot/listener, coalescing latest context/strongest
resync; 15s heartbeat/session recheck and 5s server write timeout. Writers never
wait for listeners; full slot forces resync, stalled reader disconnects. Snapshot
10s deadline; at most two concurrent reads/user and eight total. Cancelled request
releases tx/semaphore/listener. Resource/deadline rejection means unavailable whole
view, not partial rows. No shared fragment cache or private errors/IDs in logs.
Short-lived active maps, eviction and pagehide cleanup are tested. Snapshot/render
is complete surface, no silent truncation. Evidence: auth/transport/bounds tests,
both-dialect read traces, keyed escaped fragment tables. Exit: read routes perform
zero provider construction/calls/decryption/SQL writes, including middleware.

### T11 — keyed incremental reconciliation, selected sorting and calm movement

Dependency: T10. Scope propose `web/posting-updates.js`, three page shells,
`source-filter.js`, `ai-popover.js`, `styles.css`, and future zero-package Node
tests under `web/testdata` with Go wrapper like existing ai_rerate_test.go.

RED unit/DOM harness: keyed patch/insert/reparent/remove; retain unchanged card
nodes and disclosures; authoritative day/section/count/empty-state/cap backfill.
Test archive date mode keeps first-seen KST groups and within-day score/ties, score
mode reorders immediately, Today score/cap and bookmarks saved order. Never
replace selected date mode with global score or rewrite posting date. Test score
rises/falls, hard/soft threshold and saved exemption, expiry, mute, disabled source,
duplicate sibling and cap transitions with active quick source/search filters.

GREEN: compare keyed regions, patch changed safe fragments and reparent retained
nodes to server order; don't replace whole page/all cards. Refresh keyed search
registry and authoritative original counts/pills/day-empty states in source-filter;
live reconciliation recomputes matches but has an explicit no-autoopen mode.
Explicit search INPUT retains established search autoopen and pre-search state.
Live threshold move NEVER opens destination even active matching search. Move
NOW, no pinning/read deferral or terminal batch. Emit polite
`점수가 바뀌어 관심 밖으로 옮겼어요.` only for the affected read card, not focus
steal for unrelated rows.

Before patch capture retained viewport anchor/offset, focused logical control,
evidence/exclusion disclosure state and pending controls. After patch compensate
scroll; focus surviving equivalent only if visible, else next visible title/control,
previous, section summary/empty heading with preventScroll. Close hidden popover,
never focus behind closed details. Short transform-only retained-visible movement;
cancel old animation on newer update, honor reduced motion; no cache-hit flash.
Evidence: Node behavioral tests plus real browser all-surface/both-sort traces,
focused read-card threshold crossing with active search and closed destination,
expanded evidence/scroll/reduced-motion checks. Exit: immediate policy-correct DOM
and calm retained state; screenshots stay owned scratch, never root assets.

### T12 — lifecycle, context/race gates, fallback and NO terminal reload

Dependency: T11. Scope posting-updates client, `ai-rerate.js`, bookmark/mute clients,
existing owner/lifecycle Node harnesses; targeted scrape terminal caller discovered
at future baseline (do not assume Today template owns an inline scrape script).

RED: subscribe-before-read races, notices during body read, duplicate/older response,
schema/surface/sort/generation mismatch, context change followed by weaker dirty
hint, epoch restart/numeric RunID reuse, late owner/run/status and old profile/goal/
provider/model snapshots. Continuous writes must advance coherent snapshots
newer than last APPLIED revision, not discard each because newest dirty > response.
Context notice invalidates old-context read immediately; hold dirty/latest context/
strongest resync until drained. Equal revision forced resync can detect external
writes. Corrupt envelope/read error retains last good UI explicitly stale, not 0/N.

GREEN: one snapshot in flight/page, <=1 read start/s bursts; validate all guards
before DOM effect, coalesce hints and drain next read. Close observer on pagehide/
hidden/offline; register/reopen then full stored-state resync on visibility,
pageshow/BFCache, online, reconnect, gap/overflow or changed epoch regardless of
Last-Event-ID. Never reconnect operation-starting `/api/rerate`/`/api/scrape` to
observe. On observe failure close it and poll snapshot every 5s visible with
exponential transport backoff to 30s; periodic visible resync no more often than
30s catches dropped hints/external writes/KST day/deadline. Fetch deadline includes
body parsing; after five consecutive failed reads show
`새 결과를 아직 확인하지 못했어요.` and read-only `결과 다시 확인`.
No hidden/offline polling or paid retries.

Preserve initiating run owner_entry/epoch-qualified token/admit/late-error guards
and owner-specific notices, separate progress recovery from card projection.
Replace rememberAndReload and any scrape done reload with one current snapshot
request; done/failed cannot reload or replay work. Restart status is honestly
unknown/idle, never completed; stored outcomes remain. Other-tab/background updates
have no initiating-owner notice side effects. Pending bookmark/mute disabled state
survives patches; late mutator reply passes revision/generation gate, or triggers a
read-only snapshot when reply has no revision, never undo newer committed state
or replay a failed mutation. On 401 clear private live content, close connections
and guide sign-in; no automatic paid work after login.

Evidence: deterministic races/late-body harness, network interception proving no
operation GET on fallback, real cross-tab/background/hidden/BFCache/online/restart
walks. Exit: ongoing-write convergence and no terminal reload/paid-observe path;
checkpoint for the integrated risk matrix.

### T13 — integrated storage/privacy/browser and performance validation gate

Dependency: T05–T12. Purpose: mandatory pre-acceptance proof, not production follow-up.
Extend existing owned `internal/server/browser_fixture_test.go`/production scoped
tests or propose a test-only live-view fixture; no new production control routes.
Inspect native helper before use: current newPostgresTestServer creates/drops a
unique schema; browser fixture has synthetic login/role/stats/stop routes and a
blocking stop channel. It is not an all-surface live-update fixture yet. Make any
future fixture bounded/notifying and prove it cannot call a real provider.

Use synthetic two-user cookie sessions, fixed local SQLite, blocked stub transport,
scripted per-row outcomes, injected SQL write failures, deterministic KST dates,
stable ties and representative large archive under ongoing writes. Include zero,
one, cap-plus-backfill and a 2,000-posting multi-day stress fixture with mixed
outcomes/sections and ongoing inserts/score/context changes; 2,000 is a synthetic
test size, not a claim about live volume or a new row limit. Matrix:

| Layer | Mandatory evidence |
| --- | --- |
| Pure/unit/Node | T01–T03 categories/start clocks; T05 cache/outcome equivalence; membership/order/cap rules; revision/context/generation/owner races; filter-autoopen distinction; fallback/no-reload |
| Owned SQLite integration | Insert before blocked extraction; every individual saved outcome before terminal; failed-write no outcome/fresh/notice; coherent read/locks/cancel; local identity; new/missing/stale score parity |
| Owned PostgreSQL integration | Repeatable-read establishment under real concurrent commits; two users and exact-session auth; SELECT-only credential/session; atomic failures; scoped global/user publication; all foreground/detached/manual/scrape/scheduled/noop writer ledger |
| Browser all three surfaces, both archive sorts | Independent saved outcome arrivals while run remains blocked; other tab/surface same user, no other-user data; thresholds/ranking/date/ties/counts/empty/cap backfill; selected quick filters; read-card move/focus/scroll/disclosures/pending controls/reduced motion |
| Lifecycle/security/resource browser + handler | Gap/overflow/dropped hints, late reads/mutators, context changes/restart, hidden-return/BFCache/offline-online; revocation/expiry/deletion during read; no-store/escape/no-compression/flush; all limits/timeouts/cleanup |

Assert provider factory/call/decrypt and SQL mutation counters explicitly around
initial/snapshot/observe/reconnect/poll/resync AFTER restart. Counts must remain
zero; reading encrypted fields is also forbidden. Session activity unchanged.
Intentional synthetic mutation/analysis phases are outside this read-only measurement
window and labeled separately. Existing startup cache-only recovery is not replayed
by observation. Negative tests must not count failure-to-persist as a saved failure.

Measure full-surface rows/payload bytes, read/render/DOM elapsed distributions,
deadline/admission rejections, peak listeners/reads, writer wait/lock duration and
last-applied revision advancement while writes continue. Check every row and final
membership/order/count against authoritative coherent projection; no silent cap,
starvation-until-terminal or unbounded retained state. These results are PENDING,
not claimed performance success. Require complete snapshots within approved 10s
read deadline and continuing useful reconciliation under the representative writer
load; record browser perceptual/focus behavior rather than inventing an unapproved
latency SLO. Failure is a design-validation gate before T14: report measured limit
and request disposition, not an unreviewed cursor/history service, deferred live
production validation or a reload-only/single-surface workaround.

Exit: mandatory matrices pass with actual receipts, owned fixtures stop/clean up
only their resources, failure history remains; otherwise Slice 2 acceptance blocked.

### T14 — broad gates and independent exact-code acceptance of Slice 2

Dependency: T13. Run future focused/broad/race/browser/diff/privacy/links below;
audit full writer ledger and acceptance traceability, current-vs-run counts, no-
paid-read counters and unchanged complete migration manifests. Commit successful
local work; request same-card exact implementation acceptance from jobcron-reviewer,
not reuse design/plan review as code approval. Repair same card; controller integrates
exact APPROVED candidate. Update architecture/maintained behavior only now for
actually shipped local code. Exit: independently locally accepted Slice 2; release
and experiment remain conditional. Preserve earlier slice and recovery evidence.

## Acceptance traceability (nothing deferred to live observation)

The seven numbered acceptance areas in the campaign spec and the narrow Gemini
local acceptance are controlling; this matrix assigns tasks, layers and receipts.

| Approved area | Tasks | Required test/evidence |
| --- | --- | --- |
| Gemini native/compatible minute request/token, daily +/- retry, mixed/billing/unknown, malformed/oversized/nonquota/provider isolation | T01/T03/T04 | Unit classification/canary tables, transport/header bounds and copy browser |
| Gemini actual >=10s starts/cancel/overlap, 1.2s others, cache/admission/version/manifests unchanged | T02–T04 | Controlled clock transport traces, all Pacer callers, sponsor/user factory spies and exact-code diff |
| Campaign 1: insert before blocked AI; rated/no-signal/rejected/failed before terminal; partial/cancel/write failure | T05/T07–T11/T13 | Both-dialect blocked-provider commit timelines and injected atomic/outcome failures; all-surface browser arrivals |
| Campaign 2: manual/scrape/detached/scheduled cross-tab; two-user isolation; zero inference/write/decrypt on reads/restart | T06–T10/T12/T13 | Complete writer ledger, owned PostgreSQL/SQLite query and provider/decrypt counters; two-user/cross-surface/browser walks |
| Campaign 3: date/score/ties, threshold/saved/mute/expiry/source/search/duplicate/cap/empty/large; read-card moves now/no-autoopen | T07/T10/T11/T13 | Golden projection tables, real all-surface/sort/filter/closed-destination/focus/scroll/reduced-motion evidence |
| Campaign 4: stale goal/provider/model/profile/run/response/mutator; epoch/gap/read races and overflow/dropped hints | T07–T08/T10/T12/T13 | Both-dialect first-SELECT/commit barriers, Node race harness and ongoing-write convergence + reconnect/restart browser |
| Campaign 5: expiry/revocation/deletion, no-store/escape/flush, bounds/deadline/backpressure/cleanup/no compression | T06/T08/T10/T12/T13 | Exact-session read traces, revoke-during-read, resource/stalled-listener/cancel tests and privacy/transport browser |
| Campaign 6: hidden/BFCache/online, evidence/focus/pending controls, no reload; frozen calls/phase denominators/caps/cache/retry/budgets | T03/T09/T11–T13 | Node owner/mutator guards, real browser lifecycle, intercepted network and exact admission/call ledger |
| Campaign 7: focused/broad/format/browser/privacy/docs/exact acceptance | T04/T14 | Actual suite counts/skips/timing, staged scanner/manual audit, full candidate/tree/baseline and independent verdict |

## Future verification commands and fixture custody — DO NOT run now

Existing focused packages available today (new tests above still have to be written
after coding approval):

```sh
go test -count=1 ./internal/ai ./internal/pacing ./internal/server ./web
go test -count=1 ./internal/scoring ./internal/storage ./internal/auth ./internal/profile
```

For Slice 1 use the first group and targeted cache/scoring regressions; for Slice 2
use both. Existing useful anchors include TestPacerSpacesConcurrentStarts,
TestPacerWaitHonorsCancellation, TestChatCompletionsPacingCancellation,
TestArchiveGroupsByKSTDay/TestArchiveSortsByScoreWithinEachDay/TestArchiveApplyScoreSort,
TestRatingAtomicOutcomeWriteFailureCannotIncrementSuccess,
TestRatingNoSignalPersistsProvenanceBeforeSuccess and TestAIRerateLifecycleBehavior.
New proposed behavioral tests must be run explicitly at their eventual real names,
not presented as an already available `-run` command. Node is already required by
web Go tests, not a proposed production dependency.

All future execution uses real non-symlink mode-0700 TMPDIR under the executing
profile's scratch, with ownership validated and `PYTHONDONTWRITEBYTECODE=1`.
Do not read credential/env files, use shared local DB, automatic managed-local
preview/bootstrap helpers or uninspected provider/browser probe hooks. Inspect the
specific tests/subprocesses/cleanup before providing an owned disposable loopback
PostgreSQL URL. The existing helper creates/drops schemas: a shared or production
endpoint is forbidden, even for nominally read-only tests. Record fixture identity,
owned endpoints privately, synthetic users and cleanup scope. Broad tests MUST
unset all three optional inputs below; PostgreSQL/browser focused passes are
separate receipts, not silently inherited broad environment. Keep live tags off.

Future clean-environment gates (after loading only nonsecret owned test settings):

```sh
unset JOBCRON_TEST_POSTGRES_URL JOBCRON_RERATE_BROWSER_PROBE JOBCRON_RERATE_PROBE_OUT
export PYTHONDONTWRITEBYTECODE=1
go build ./cmd/jobcron ./cmd/jobcron-user
go test ./...
go vet ./...
gofmt -l .
git diff --check
```

Also check indexed Go sources by `git ls-files -z '*.go'` with a bounded runner
feeding only those files to gofmt; literal tree scan and indexed scan are distinct.
Record inherited ignored formatting findings without reading/repairing private
evidence. Verify full diff scope, complete SQLite/PostgreSQL migration filename/
digest/tree manifests, docs inline/reference links/anchors, Gitleaks on complete
staged diff and manual public/privacy audit. Run focused race checks for touched
concurrent packages (`go test -race ./internal/pacing ./internal/server ./internal/storage`)
under inspected owned synthetic setup; no live integration tags.

Standard future preview: `--host 127.0.0.1 --port 17777 --no-open`, explicit owned
fixture database/configuration only, never default DB/key paths. A different port
requires durable nonconflicting allocation. Test-only listeners must record their
actual owned loopback ports; they are not permission to share preview helpers.
Prefer bounded fixture-based browser smoke with stub providers, not a real scrape.
Real browser smoke covers ALL three surfaces, both archive sorts, saved outcomes
before terminal, cross-tab, detached/scheduled/noop, read-card focus/scroll/search
move, hidden/BFCache/online/restart and zero-paid observation. Include actual
functional execution, not template-string tests alone.

Measure elapsed full suites; if above the foreground tool window, launch bounded
notifying background work and consume its actual exit/output, with heartbeats.
Do not size limits from a tiny focused test or confuse a command submission with
completion. Keep actual provider/subprocess deadlines, hung-process watchdogs,
transport retry backoff and production minimum 1800s stability; no invented latest-
start cutoff, aggregate campaign timer or approval expiry.

## Conditional release and experiment references (not task execution steps)

Only AFTER both exact local acceptances may the owner/controller separately
authorize ONE combined app-only release. Bind exact integrated clean source,
source-specific exact-head green CI (including stages skipped by earlier failure),
immutable published artifact/platform and compatible recovery. Publication/push,
credential acquisition/live reconciliation, selector persistence/app replacement
and recovery are distinct approval effects. Follow the accepted
[native runner/operator reference](../../scripts/app_release_fixture/README.md)
and [existing-service safety contract](../decisions/261009-ai-rating-and-release-contract.md).
Preserve complete unchanged manifests/applied ledger, least-privilege runtime,
pre-change recovery/valid restore evidence, valuable/new writes, protected proxy/
mounts/secrets/DB/routing and ONE scheduler. Any schema/manifest/non-image config
change exits the app-only lane; no migration/launch/rehearsal repetition or whole-
stack restart. Verify exact journey/protected state and >=1800s production stability.
Local tests cannot prove production no-swap/IAM/public/provider state. No provider
inference as release smoke. No worker push, PR, deployment or cloud write.

Optional Gemini experiment follows narrow plan Phases 4/5 only with separate
approval: <=20 selected existing Stage-2 misses, frozen identity/target/budgets and
supported demonstrably bounded admission/stop control. No arbitrary-ID endpoint
is assumed; missing controls block experiment, NOT UI acceptance. No cache deletion,
forced misses, scrape/Stage-1, live old-pacing baseline, paid Haiku comparison,
automatic retry or unapproved scheduler/config/key change. Stop new admissions on
rate/quota failure or ambiguous submission and reconcile in-flight work. Local
cancellation is not remote non-submission. Record actual attempted/submitted/
completed/never-reached/outcome/usage/elapsed with unknown billability distinct from
zero. Report conditional feasibility, not quota safety, causality or model superiority.

## Current documentation closeout and completion labels

This card only expands the existing plan and minimally corrects lifecycle headers/
index. Check source grounding/dependencies/traceability/order/HOLD, allowed Markdown
scope, inline/reference destinations and anchors, diff whitespace, complete staged
Gitleaks/manual privacy audit. Status-only edits do not invalidate prior unchanged-
code design/runner checks: reuse author 168/reviewer 169 docs-consumer evidence
(226 passes, seven optional skips, zero failures) and maintained broad baseline
evidence; do not rerun Go/browser/native suites merely to write plan text. If an
existing docs test consumes a changed assertion/path, inspect and run only that
offline docs-consumer test under safe owned TMPDIR/env. No app experiment/scaffold.

Commit passing local DOCS; hand off full baseline/candidate/tree, changed paths,
actual checks/limitations and absolute sanitized report to same-card jobcron-reviewer
on jobcron for independent PLAN-READINESS review. That reviewer checks concrete
tasks, grounded seams/dependencies and every locked acceptance/HOLD requirement,
not just restating prior design approval. Exact independent approval triggers
controller local DOCS integration only. Stop there; create no future feature cards.

Distinct milestones: design-approved; plan independently ready; coding explicitly
started; Slice 1 accepted; Slice 2 accepted; release verified; experiment measured.
Only evidence/authority appropriate to each may advance it. This writing/review
does not mark future tests, code, performance or operations done. Implementation
remains explicitly ON HOLD until a future owner start instruction. Preserve recovery
and review history; archive this plan only when its actually authorized scope finishes.
