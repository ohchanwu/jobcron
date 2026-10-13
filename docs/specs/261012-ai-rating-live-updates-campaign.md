# AI Rating and Live Posting Updates — Campaign Specification

Created: 2026-10-12. Status: independently design-ready in review 169; owner
DESIGN approved at `119fd7577ebd9b8134512654d0ed50dac2922827`.
The plan was independently ready in review 171. On 2026-10-12 the owner approved
the plan and autonomous LOCAL implementation, superseding the earlier instruction
to write the plan without starting. Slice 1 is locally accepted/integrated at
`8c369d3cf19a4c966fe4745dfa99738c3fe78367`. Slice 2's live stored-view candidate is
implemented locally and awaits separate exact-code implementation acceptance. Live inference,
publication and deployment remain unauthorized.
Companion: [task-level plan](../plans/261012-ai-rating-live-updates-campaign-plan.md),
independently plan-ready; local coding authority is the later owner instruction.
Discovery baseline: `095bc775610656f3cbe74a076c5be495c30d22b5`, tree
`3a963914ba415838a11412a122458111cd144a15`.

## Scope and order

One campaign, two separately accepted future local slices:

1. Gemini pacing and quota classification, under the existing
   [narrow Gemini specification](261009-gemini-pacing-and-quota-classification.md).
2. Live, per-posting stored-state updates on Today (`/briefing`), bookmarks
   (`/bookmarks`) and all postings (`/`, legacy `/archive` redirects there).

A possible single app-only release follows BOTH independently passing slices.
UI implementation/acceptance need not wait for a paid/live Gemini experiment.
The [runner is locally accepted](../decisions/261012-app-only-runner-local-acceptance.md);
that does not approve this payload, production execution or a provider experiment.
Do not dispatch future implementation cards from this proposal.

### Slice 1: retain the narrow contract

The Gemini spec/plan own parsing fixtures and experiment details; this campaign
changes their placement, not their technical scope. Preserve experimental Gemini
**at least 10-second request-start spacing**, actual limiter/cancellation tests,
existing 1.2-second non-Gemini behavior and worker-latency overlap. Cover native
and supported compatible structured errors: minute request/token, explicit daily
including RetryInfo, mixed/conflicting, billing/account and unknown. Bounded
allowlisted classification must not disclose raw errors or invent quota scope.
Preserve budgets, singleflight, successful caches, prompts, models, versions,
evidence rules and complete migration manifests. Correct affected timing copy.
Neither quota safety nor model superiority has been demonstrated.

The proposed live run remains separately gated: at most 20 selected existing
Stage-2 misses, a frozen identity, demonstrable supported admission/stop control,
no forced misses, scrape/Stage-1, automatic retry, new live baseline or paid Haiku
comparison. Stop new admissions after quota/rate failure or ambiguous submission;
reconcile in-flight work. Cancellation is not remote non-submission proof. A
missing supported route blocks the experiment, not the useful local UI slice.

## Slice 2: owner product contract

- Cover manual rerate on all three surfaces, scrape-time AI and existing
  scheduled/background analysis. A page receives its user's changes even when a
  different tab, surface or detached run initiated them. Hidden tabs resync on
  return. Observation never starts, retries or expands analysis.
- Insert each eligible newly scraped posting as soon as its posting transaction
  commits, before extraction/Stage-2 finishes. Show `아직 AI 분석 전` for no current
  Stage-2 result, not a fake zero adjustment, queued state or completion promise.
  Source data plus deterministic rule scoring are sufficient to render it.
  Current threshold, source, mute, date, duplicate and cap rules still apply;
  an eligible filtered-out row appears in its proper section, not forced main.
- Reflect every committed per-posting terminal outcome individually: rated,
  genuine no signal, rejected evidence and definite failed status. Preserve
  [outcome/cache semantics](../../internal/server/RERATE_NOTES.md), faded supported
  old scores and hard exclusions. A committed genuine empty success overrides
  old evidence; rejection/failure creates no new score adjustment. Unknown legacy
  provenance remains unknown. Unadmitted/budget-skipped work gains no outcome.
- Never display/count a provider reply as a saved success before commit. An
  outcome-write error is a run/storage warning, not a saved failed/rated card.
  Other rows already committed still update when the run later fails/cancels.
- Keep the user's SELECTED sort. Never switch 날짜순 to 점수순 on arrival of a
  score. Apply threshold/membership immediately in either mode. Preserve current
  search/source choice, bookmarks, not-interested, exclusion, dates, counts and
  empty states. No full-page terminal reload, final-batch-only behavior or pinning
  a below-threshold card in the main list.
- When the read card crosses below threshold, move it immediately into its
  filtered-out section; give a subtle notice and sensible focus/scroll handling.
  Do NOT autoexpand the destination. Other disclosure/control state is retained
  where applicable; honor reduced motion.

## Observed native fit and necessary seams

These are source observations at the discovery baseline, not implemented changes:

| Existing seam | Consequence for the proposal |
| --- | --- |
| `internal/server/rerate.go:716–853`: shared pool and `rerateOne` commit result/outcome before return; aggregator has only cached/providerCalled/err and progress | Add posting identity and explicit committed-change/outcome information to the worker result; publish after durable write, not from provider completion or boolean inference |
| `rerate.go:583–699,856–970`: preparation, global contextual-validation candidates, then fixed Stage-2 surface | Freeze the existing preparation cohort and Stage-2 list at their current admission boundaries; live display is NOT the next provider work queue |
| `internal/server/server.go:825–963`: insert precedes extraction, refresh and successful extraction writes; `540–689`: sweep/dedup/user analysis | Publish posting insertion BEFORE the blocking extraction; include global extraction/detail/sweep/dedup changes as well as Stage-2 |
| `internal/server/scheduler.go:71–187`: shared gate, global collection then sequential per-user analysis with noop emit | Hook committed changes below request-specific emit; request SSE alone misses background users |
| `internal/server/rerate_status.go:31–167`: in-memory user/surface run tracker, epoch-qualified token, provider-free status GET (authentication still touches session state) | Reuse run identity/status distinctions, but not as a durable posting revision or cross-surface observation channel |
| `internal/server/server.go:191–268,352–391,1025–1163` | `runtimeForRender` decrypts/constructs a provider; `scoreAll` is provider-call-free but WRITES scores. Neither belongs in read-only observation |
| `internal/server/auth.go:31–98`; `internal/storage/sessions.go:172–197` | Existing `requireAuth`/`stateUserID` ultimately use `UserBySessionToken`, which WRITES session last_seen_at. Observe routes need SELECT-only session validation in the middleware as well as the handler |
| `handlers.go:278–367`, `archive.go:125–246`, `bookmarks.go:48–99` | Builders skip absent/stale materialized score rows. New postings need a common pure read projection, not calling `scoreAll` from an observer |
| `web/ai-rerate.js:197–205,291–350` | Operation-starting GET EventSource and owner-only terminal reload are distinct from proposed observing; keep their no-restart/late-owner guards, replace reload with safe resync |
| `web/source-filter.js:43–62,97–145`; bookmark/mute and `ai-popover.js` delegated handlers | Filter currently captures cards/counts once and search can autoopen exclusions. Refresh its keyed registry/counts; explicit search may open matches, but a live move must not open a destination |

Current templates duplicate card shells across surfaces; `_chips.html` and
`exclusion_reason.html` provide shared escaped subtemplates. Add small keyed
fragment boundaries, not a frontend framework. Initial page render and observation
must share the same pure projection, so a later navigation cannot undo fresh state.

## Chosen boundary: invalidation SSE + authoritative read snapshot

Choose authenticated read-only `GET /api/posting-updates` for small invalidation
notifications, and `GET /api/posting-view?surface=...&sort=...` for a surface
snapshot. These are PROPOSED routes, not available commands. Use existing Go
`net/http`, SSE, `html/template` and vanilla JS; no WebSocket, broker, durable
event log, distributed limiter, new dependency or migration by default.

A full rendered page reload loses focus/disclosures and is rejected. Sending
provider deltas alone misses membership, exclusions, caps and cross-tab changes.
Sending a complete surface snapshot down each SSE notification multiplies server
reads/render work per listener. Small invalidations plus one coalesced read per
visible page is the smallest native fit: a snapshot includes ordered keyed row
fragments, section/day membership, authoritative counts/empty-state data, current
context and user-scoped run summaries. The browser diffs it by posting ID; it does
NOT replace the page or all cards indiscriminately. Each newly saved row is patched
without waiting for the run terminal event; different rows in a coalesced snapshot
still get individual outcomes. This is current-state reconciliation, not token
streaming or an outcome-history feed. Offline/overflow recovery shows the latest
stored outcome per key, not a fabricated replay of superseded intermediate states.

### Durable authority and revision protocol

1. A successful writer transaction returns before publishing its change. Add a
   lightweight process-local commit/read barrier covering relevant global and
   user-scoped mutations through publication; never hold it over provider/network
   calls. A shared lock order (global before user) avoids deadlock. Snapshot reads
   capture one consistent database view and matching revisions under the barrier;
   use bounded read-only transactions (PostgreSQL repeatable-read, SQLite read
   transaction for compatibility), not unrelated SELECTs mixed across commits.
   Establish the database snapshot and capture revisions under the barrier, then
   release it before rendering/network transfer; a slow browser cannot block commits.
   Existing store methods are not transaction-scoped: factoring this read boundary
   and instrumenting all listed writers is required future work, not an existing API.
2. Server epoch changes on restart. Global posting revision and this user's view
   revision advance monotonically within it. Send only scoped opaque revisions/
   dirty kind, optionally affected posting IDs; no other user's revision, goal,
   outcome, run token or credential information. Global events contain public
   posting changes only, never sponsor/user analysis metadata.
3. Snapshot envelope binds epoch, global/user revisions, an opaque current-context
   token, surface, effective sort and request generation. Context covers profile
   hash, goal identity, provider/model/score/dealbreaker contracts and AI-enabled
   metadata. Do not send raw profile/goal hashes as diagnostics. Derive versions
   from nonsecret configuration functions and SELECT-only credential-presence
   metadata; no ciphertext/nonce read, decryption or provider construction is needed.
   A narrow `SELECT EXISTS` credential-presence read is future work, not a claim
   that the current credential loader is metadata-only. Configured AI is not proof
   of usable credentials/provider readiness; a later deliberate operation validates
   that and may fail safely. Saved evidence does not require secret resolution.
4. Subscribe/register before initial snapshot; then compare its revisions with
   notices received during the read. Only one read in flight per page. If newer
   ordinary notices arrive, apply the coherent snapshot only when not older than
   the last APPLIED revisions and admitted by the latest request generation, then
   drain dirty state with another read. Equal revisions allow a forced stored-state
   resync to detect external writes that bypassed counters; unchanged keyed rows
   still do not flash. Do not discard
   every read merely because continuous writes advanced a dirty hint: that would
   starve live updates until run end. A context-change notice carries the new opaque
   context token and invalidates old-context in-flight reads. Coalescing retains
   the newest context token and strongest resync requirement, never overwriting it
   with a weaker ordinary dirty hint. Validate schema/context/surface/sort/request
   generation before DOM effects; ignore duplicate/older responses. Context changes
   require a new complete projection, not applying an old run's delta to new goals.
   Durable view revisions exclude transient progress; progress uses its own run
   token/phase sequence and cannot certify stored success. An old run token may
   update its own progress only; cards always come from the
   current-context snapshot. Do not compare numeric RunID across epochs.
5. A gap, overflow, reconnect, pageshow/BFCache or visibility return requires a full
   stored-state resync, regardless of Last-Event-ID. No event replay is promised.
   Subscribe only to the observe route, never reopen `/api/rerate` or `/api/scrape`.
   A restart marks lost in-memory run status unknown/idle with honest copy, not
   "completed" or an inference retry; durable per-row results remain readable.
6. Periodic bounded read-only reconciliation catches dropped notices or out-of-band
   operator writes that bypass the process barrier. The supported deployment is one
   application/scheduler, not a multi-writer distributed revision service. Never
   claim notification delivery itself proves freshness or commit.

### Read projection, not render-time mutation

Extract the deterministic composition inside `scoreAll` into a pure function used
by writers AND snapshot/page reads. Feed it committed posting, this user's current
profile, global current-content extraction, user-scoped contextual validations,
fresh/latest-supported deltas and bounded outcome state from the same read view.
Preserve exact stale fallback, genuine empty precedence, exclusion short-circuit,
score clamps and citation rules. Missing materialized score rows use the real rule
calculation; do not invent zero or display another-profile total. Reads never write
scores, usage, outcomes, profiles, run state or credentials and never resolve an
`AIRuntime`. `buildRerateInfo` needs a metadata-only equivalent rather than passing
an artificially constructed runtime. Propagate cache/read errors as unavailable
snapshot/counts, not a guessed fresh N=0 or a fabricated successful card. On failed
reads keep last good UI marked stale until reconciliation succeeds.

All three initial page builders adopt this read projection as part of the future
slice, while admission keeps its existing fixed phase lists. Observer templates
use ordinary escaped text/URLs and existing chip/evidence rules; no raw model HTML,
provider errors, JD export, executable markup or template.HTML coercion.

### Writer coverage and scope

| Source of change | Invalidation scope and display |
| --- | --- |
| Posting insert/detail refresh, global Stage-1A extraction, dedup and sweep | Global public posting dirty signal; each listener recomputes only its own profile/filter view; inserts before AI, update/remove canonical siblings after commit |
| Stage-1B validation and Stage-2 result/outcome, including scheduled/noop-emit callers | That user only, all listing surfaces; fresh N and card/status change after commit |
| Rule score writes / rescore recovery | User view dirty only on meaningful stored-state change; pure projection avoids waiting for terminal whole-table rescore |
| Profile/goals/weights/threshold/source/model/provider or credential presence changes | That user context invalidation after commit; derive new current view, retain supported stale evidence, no inference on observe |
| Bookmark/not-interested add/remove | That user's surfaces; reconcile membership, bookmark soft-threshold exemption and mute rules after commit |
| Run progress/terminal/partial/storage warning; committed account/session revocation | User-scoped bounded transient status separate from durable view revisions; saved cards survive later failure; close invalid sessions, never turn unsaved replies into outcome rows |

Successful unchanged cache hits require no card flash or pretend-new result.
`UpsertAIResult` atomic score+outcome writes and `UpsertAIScoreFailure` successful
writes are authoritative. A storage-error worker result must explicitly mean no
saved-outcome notification; callers must not classify it as provider success.
Stage-1A best-effort write failures likewise publish nothing. A useful status may
report incomplete persistence without claiming a per-row saved failure.

Run admission remains independent: freeze preparation candidates, the established
surface-first/global Stage-1B cohort, and the unique Stage-2 target/denominator at
their existing phase boundaries. Keep that ordering even as the display reorders,
filters or receives new rows. Never add targets, raise caps, bypass the shared gate,
re-spend cache hits or change rejected automatic-vs-manual retry rules. Browser
quick source/search filters currently narrow display, not provider admission; this
proposal does not silently change that contract. Display fresh N/current-view M
separately from frozen run processed/target/skipped counts, labeled by phase;
failed/rejected calls are processed, not cap skips. A finished display snapshot
is not proof that every admitted provider call completed.

## Sorting, membership and calm reconciliation

- Only archive/all-postings currently has a sort selector. Preserve its effective
  per-page mode (explicit query then remembered cookie, default date). Snapshot
  requests send the current page's mode, not a new cookie from another tab.
  Today stays score-descending with its existing cap; bookmarks stays saved-date
  order. No new sort selector is required on either.
- 날짜순 archive keeps first-seen KST day groups newest first and existing
  score-descending order WITHIN each day. Existing stable first-seen tie order is
  retained. Thus an updated row can move inside its day, never across dates or
  into a date-headerless global ranking. 점수순 immediately recomputes the flat
  descending-score ranking with the same underlying stable ties. Excluded order
  follows the existing builders. Do not rewrite dates on score completion.
- Apply threshold and hard exclusions immediately. Bookmark exempts soft MinScore
  on Today/archive, not hard exclusions. Expired rows leave Today, collapse in
  archive, remain on bookmarks; muted rows leave Today/archive but remain marked
  on bookmarks. Disabled sources/duplicates, Today cap and canonical replacement
  follow the current surface rules. Insertion/removal must also bring in the next
  eligible capped row and update day headers, counts and empty states.
- Key every card/fragment by posting ID and retain nodes when unchanged. Before a
  patch, save the first visible retained card's viewport offset, focused logical
  control and open evidence/exclusion disclosures. Patch changed parts, reparent
  for sorting/membership, refresh the source-filter search registry and counts,
  then compensate scroll against that anchor. Use short transform-only movement
  for retained visible cards; no attention-grabbing flash on cache hits. Disable
  animations for reduced motion and cancel obsolete animations on new revisions.
- A read card moving below threshold moves now. Keep destination open/closed as
  it was, even during an active search: live reconciliation may recompute search
  matches/counts but cannot use the current filter's search-autoopen behavior to
  expand it. Explicit user search input still has its established autoopen rule.
  Say quietly in a polite live region: `점수가 바뀌어 관심 밖으로 옮겼어요.`
  Keep focus on a surviving equivalent control only if visible; otherwise focus
  the next visible card's title/control, then previous, then section summary or
  empty-state heading with preventScroll. Close a now-hidden popover; never leave
  focus trapped behind closed details or force focus for unrelated updates.
- Pending bookmark/mute controls retain disabled/in-flight state during patching.
  Their responses must not undo a newer committed snapshot; reconcile using the
  same revision/generation gate (or immediately request current snapshot when the
  mutation response has no revision). Do not replay a failed mutator to refresh.

## Authentication, bounds and fallback

Production observation uses the existing cookie-session identity, same-origin
only, no user ID selector, credential in URL or permissive CORS. Reject anonymous
API reads with 401, not login HTML masquerading as a snapshot. Do not expose the
authenticated stream to public demo visitors. Fixed local-user reads use the
existing local identity only, without another user's metadata.

The existing authentication middleware is not read-only: `UserBySessionToken`
updates last_seen_at. Add a SELECT-only exact-session/expiry/user-existence lookup
for these two observation routes, including their initial middleware admission
and heartbeat checks; do not call the mutating helper before a supposedly read-only
handler. Keep the same hashed-token, expiry and revocation predicates and API 401
behavior, with no privilege bypass or session extension. Other route activity
tracking is outside this slice. Verify no SQL writes from observation authentication.

Owner amendment (2026-10-12, option 2): zero SQL/storage/session mutations applies
to the page-document/read-projection path and the new observer endpoints, including
their reconnect, polling and resync after restart. The two existing bootstrap
status GETs, `/api/briefing-status` and `/api/rerate/status`, retain their existing
`sessions.last_seen_at` activity writes and exact-session authentication. Real
browser verification must allow and measure those unsuppressed requests separately;
the complete page bootstrap is NOT zero-write. This exception permits no credential,
usage, cache, score, outcome, profile or session-extension write, decryption,
provider construction/call or paid restart. It does not weaken new observer or
document read-only guarantees. Original whole-bootstrap failing evidence remains
in the same task's recovery history, not relabeled as a passing old test.

Recheck exact session validity before each private emission/read and on heartbeat;
explicit logout/password/session revocation closes relevant listeners immediately.
A missed notification must still fail the next validity check; revalidate before
writing a fragment after a lengthy read. On 401 clear private live content, close
listeners and present sign-in guidance; do not auto-start work after login.

Use `Cache-Control: no-store, private`, no shared fragment cache, existing SSE
content type/flush and `X-Accel-Buffering: no`; no compression/buffering on SSE.
Send bounded JSON invalidations, not multiline HTML via the single-line SSE writer.
HTTP snapshot JSON safely encodes escaped template fragments. Avoid leaking
private error bodies or user IDs into logs and validate all supplied surface/sort
values against a closed allowlist.

Proposed resource limits for implementation tests: one observer per visible page,
maximum 8 listeners per user / 64 total, one pending dirty notification slot per listener,
15-second heartbeat/revocation check and 5-second server write timeout. A full
slot coalesces to latest revisions with resync-required, never blocks a writer;
disconnect stalled readers. Close/unregister on request cancellation/pagehide;
hidden pages close observation and reopen/resync on visibility return. Keep maps
only for active listeners/runs and evict idle state; no unbounded per-outcome list.

Snapshot reads have a 10-second deadline and one in-flight read per page, maximum
one read start per second during bursts. No terminal batching: a healthy visible
page reconciles each saved row on the next bounded read, independently of run end.
One complete surface snapshot is the initial boundary, matching the existing
all-postings surface rather than inventing a new pagination product. Bound server
reads by the deadline and concurrency (one per page, at most two concurrent snapshot
reads per user / eight total); release the consistent read transaction on completion/cancellation.
Measure full archive payload/DOM reconciliation with a representative large fixture
and ongoing writes. Never silently cap/truncate rows or restart coherent reads on
every dirty hint; newer-than-applied snapshots advance the UI during continuous work.
An exceeded deadline/resource admission returns unavailable, not a partial success;
preserve last good state and the explicit fallback. If the existing full-surface
approach cannot meet acceptance, surface a design finding before implementation
acceptance instead of adding an unreviewed cursor/event-history service.

On observe error close that observe connection and use read-only snapshot polling
(every 5 seconds while visible, exponential transport backoff up to 30 seconds).
A periodic visible resync at most every 30 seconds also covers lost notifications
and KST-day/deadline transitions. Pause offline/hidden; retry on online/visibility.
Each fetch includes body-read deadline. After 5 consecutive failed reads, show
`새 결과를 아직 확인하지 못했어요.` and a read-only `결과 다시 확인` control;
keep last good state explicitly stale, do not claim completion or reload the page.
The initiating run's progress/status recovery remains separate and cannot restart
its operation-starting EventSource. Terminal done/failed requests one current
snapshot, preserves owner-specific notice semantics and never reloads. Polling is
only a degraded transport path, not the default final-batch implementation.

## Future implementation acceptance

Slice 1 must pass its narrow local acceptance independently. Slice 2 needs:

1. Controlled provider/DB tests show insertion before blocked AI, and each
   committed rated/no-signal/rejected/failed row updates before run terminal on all
   three surfaces. Include partial/cancelled run and both result/outcome-write
   failures: no saved card/fresh count on failed write, committed siblings survive.
2. Manual, scrape-time, detached and scheduled/background writers reach another
   open tab/surface of the SAME user. Two users cannot read each other's profile,
   outcomes/status/fragments or infer another user's paid progress. Observation
   requests, including reconnect/poll/visibility/resync AFTER server restart, have
   ZERO provider calls and ZERO storage/credential mutations/decryptions; assert
   those seams explicitly. Existing startup cache-only recovery is separate and
   is not initiated or replayed by observation.
3. Date-vs-score browser fixtures lock group/tie rules; score rises/falls, threshold
   crossings, saved exemptions, mute/expiry/source/search/duplicate/cap changes,
   section counts and zero-row/large surfaces converge. Read-card movement must
   not autoexpand its destination, defer filtering or lose keyboard/scroll state.
4. Duplicate/out-of-order/gap/late run, changed goals/provider/model/profile,
   old network/mutator responses, restart epochs and snapshot read races cannot
   overwrite current UI. Reconnect snapshot closes the subscribe/read race; slow
   listener overflow and dropped hints resync without a provider operation.
5. Auth/session expiry/revocation/account deletion, no-store/escaping/SSE flush,
   resource/deadline/backpressure/cleanup and no-compression contracts pass.
6. Controlled browser evidence includes hidden/visibility/BFCache/online recovery,
   reduced motion, expanded evidence, focused control/read-card removal and no
   terminal reload. Freeze provider admission IDs/phase denominators and prove
   reorder/filter/new inserts neither broaden calls nor alter retry/cap/budgets.
7. Focused plus broad build/test/vet/tracked-format gates, owned loopback browser
   smoke, public-doc/link/privacy checks and exact-candidate independent acceptance.
   Synthetic evidence is not live provider quota proof or release approval.

No model comparison, token streaming, push notification, new scheduler, schema
change or event-history product is included. Separate approval governs coding,
live experiments and any future release; unresolved requirements cannot be traded
for a reload-only or single-surface shortcut.
