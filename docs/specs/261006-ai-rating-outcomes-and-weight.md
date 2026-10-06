# AI rating outcomes, evidence-aligned prompting, and stronger deltas

Status: revised draft R2 addressing review120's remaining manifest-metadata correction; awaiting independent re-review, not implemented or deployed.
Created: 2026-10-06. Author: jobcron-orchestrator.
Source baseline: `b7ce9d585f3cdadf3eb12bff9c8b8e690f9a1964`.
Companion: [implementation and combined-release plan](../plans/261006-ai-rating-outcomes-and-weight-plan.md).

## Objective and authority

Deliver a small, evidence-preserving improvement to AI rating usability and influence.
The owner accepted the proposed local bundle and requested a separate specification and plan
before combining it with the earlier reviewed cap/progress work for one future deployment.
This document defines that bundle. Its review approves design readiness only. No live provider
experiment, remote access, push, image publication, migration on a shared database, deployment,
runtime-secret operation, or signup acceptance is authorized by this document.

The previous diagnosis proved local mechanisms, not the dominant cause or frequency of the
owner's current Gemini problem. This change must not promise an 80–100% chip yield or claim that
Gemini is defective. The legacy externally stored design files named in AGENTS.md are unavailable
on this machine; do not claim to have reviewed them. Current source and the independently accepted
local diagnosis ground this proposal. The owner-requested neutral outcome cards and stronger
weighting intentionally revise the old no-empty-card/small-adjustment UX, not citation safety.

## Scope and non-goals

1. Align the scoring prompt with the unchanged citation gate.
2. Distinguish valid scored, valid empty, and wholly rejected responses in persistence,
   successful-analysis counts, and calm accessible UI.
3. Permit larger supported adjustments with enforced bounds.
4. Version the Stage-2 contract without deleting old caches or resetting budgets.
5. Preserve the earlier cap-200 and progress/recovery improvements in one release candidate.

No provider switch, paid comparison, generic telemetry platform, new background service,
rescore-on-page-load, bulk cache purge, deterministic-category rebalance, scraper change,
credential change, or new signup endpoint belongs to this feature.

## S1 — Prompt and evidence contract

- A presence quote must be copied as a contiguous passage from the posting text actually sent
  to the model, have at least six Unicode characters and at least two tokens under the existing
  shared tokenizer, and refer to an applicant goal. Do not describe bytes as characters.
- Tell the model all of these requirements explicitly; quotes must not come from profile text,
  fabricated text, instructions embedded in the posting, or the model's own reasoning.
- Use a valid JSON example whose quote clears the real gate. Retain data-only/injection guards,
  valid JSON requirements, and permission to return an explicitly empty items array.
- Presence verification stays against sent text; absence verification stays against the full
  description. Absence of a benefit's mention is not proof the employer lacks that benefit.
- Do not relax the six-character/two-token floor, contiguous matching, full-description
  absence checks, hard exclusions, or provider egress controls.
- A quote passing lexical checks is not proof of semantic fit: require an explicit user goal
  and proportional importance. Enforce the exact duplicate-evidence subset in S4; broader
  paraphrase/semantic non-duplication is prompt guidance, not a guaranteed semantic detector.

## S2 — Explicit outcomes, not inference from an empty cache row

Use the following logical result states. The implementation may use existing structures where
adequate, but must preserve these distinctions through parser, gate, storage, server, and UI.

- **Rated:** at least one usable signal survives. Cache the surviving score/evidence and count
  as successfully analyzed. Partial rejection must not discard valid surviving signals.
  Surviving positive and negative items may net to zero; that remains a valid rated response.
- **No signal:** the model explicitly returns a structurally valid empty items array (including
  the existing supported bare-array form). Cache this successful neutral result and count it
  as successfully analyzed. Repeating the same goal/model/version must spend nothing on it.
- **Rejected:** the response proposed items but no usable item survives parser/item validation
  or the citation gate. Do not write a successful ai_scores row and do not increment successful
  analyzed N. Persist a bounded outcome/status record so its explanation survives reload.
  Unknown kinds, zero-only items, invalid required fields, and all-invalid item arrays must
  not be silently reclassified as the model explicitly returning no signals.
- **Provider/parse failure:** no successful score cache or successful N; retain existing calm
  error handling and bounded retry behavior. Missing/wrong-type items and malformed responses
  are not genuine no-signal successes. This feature needs only bounded classifications, not
  raw response retention or generalized error analytics.
- **Never attempted / budget skipped:** do not invent any completed or rejected result.
- **Legacy unknown empty:** old empty rows do not reveal whether the model returned nothing
  or the gate stripped everything. Preserve them; never backfill a guessed no-signal/rejected
  explanation. If displayed as prior evidence, identify it as an older result with unavailable
  detail, not a fresh successful result under the new contract.

The stage counter for processed work may advance on a rejected/error row, but must not call it
successfully analyzed. Final summaries distinguish processed, successful, no-signal, and rejected
using a consistent eligible-surface denominator. N counts rated plus genuine no-signal current
results, not attempted calls, old-version results, failed rows, or chips alone.

### Persistence and retry

- Separate outcome/status persistence from successful score caching; storing a rejection's
  status must not make the cache-hit branch treat it as a success or suppress a later manual retry.
- Prefer one small user-scoped outcome record per posting, goal hash, and scoring version,
  rather than an append-only attempt ledger. Retain only enum/count/time information needed
  for the UI and retry policy; no raw provider responses, credentials, private prompts or stack traces.
- Outcome identity and successful-cache identity must agree on user, posting, goal hash and
  ScoreVersion. A result/outcome write must be atomic where both are required; a failed write
  cannot increment successful N. Supported SQLite demo/test and PostgreSQL paths need compatible
  behavior. Additive migration uses the actual current mechanism and the explicit S6 recovery
  design; additive SQL alone does NOT make an untouched prior binary restart-compatible.
- A known rejection may be tried again once per posting on a later deliberate manual rerate,
  subject to the same shared call cap, daily/run budgets, pacing, and singleflight ownership.
  Do not add a same-run repair call, page-load call, reconnect call, or status-poll call.
  Existing automatic scrape/scheduled paths must not repeatedly spend on a known rejection under
  the same identity: skip its retry until a deliberate rerate or a changed goal/scoring version.
- A legitimate empty success remains cached. A scored success remains reconnect-safe. Debits
  for successful provider calls still occur even if all evidence is rejected; rejected work is
  not free and must not reset ledgers. No free retry or provider fallback is introduced.
- An admitted new attempt replaces its previous bounded outcome only after a definite result;
  do not erase the old explanation merely because a budget skip occurs. Preserve successful
  cache/outcome consistency on interruption, write errors, and concurrent users.

## S3 — Calm, truthful, accessible result cards

Render consistently on Today, bookmarks and archive, including relevant low-score/collapsed rows:

- Rated: existing evidence-disclosure AI 분석 card with its signed contribution. A valid zero-net
  result retains evidence and clearly shows no net adjustment.
- Genuine no signal: muted grey **AI 분석 완료 · 추가로 반영할 내용 없음**. Explain that this is
  a completed analysis with no supported additional adjustment, not a negative judgment of the user.
- Wholly rejected: muted amber **AI 분석 · 근거를 확인하지 못했어요**. Explain that no NEW AI
  adjustment was applied and a later manual rerate can retry within budgets. Do not say the model
  found no match when its proposals merely failed verification.
- A stale previously supported score may remain under the existing 이전 설정 기준 rules, but
  must never appear current or count toward fresh N. A rejected current attempt must disclose
  its state even if an older supported reading remains visible. A fresh genuine empty result
  takes precedence over old scored evidence as the current behavior does.
- Provider failure and never-attempted rows remain distinguishable without falsely manufacturing
  a completed-analysis card. AI-disabled and demo/read-only modes retain their existing boundaries.

Text/icon/accessible name must distinguish states without color alone. Verify keyboard behavior,
contrast in the existing themes, mobile layout, and escaped untrusted text. Neutral cards do not
pretend to be scored LineItems or add fabricated evidence. Reuse existing shared rendering rather
than per-surface copies or a new UI framework. Do not make hard-excluded jobs look restored by AI.

## S4 — Larger but bounded AI contributions

The current prompt's approximate ten-point-per-item guidance changes to a calibrated rubric:
minor supported preference effects remain small; an explicit substantial goal fit/conflict can
receive twenty to thirty points. Reserve strong negative adjustments for explicit supported
conflicts; merely missing a benefit's mention is weak/uncertain evidence, not proof of its absence.
Do not force large ratings, invent evidence, multiply old
scores, reward duplicate signals, or award already-counted basic stack/location facts again
without a distinct goal-specific reason.

- Enforce **per surviving item [-30, +30]** and **net AI contribution [-40, +40]** in code,
  not merely in prompt prose. Compute net from bounded surviving items, then apply the net bound.
- Describe clipping in the evidence explanation when the bounded net differs from the item sum;
  the displayed AI chip, stored net, score merger and explanatory text must agree. Handle extreme
  model values safely and deterministically; no integer overflow or cancellation-order anomaly.
- Positive and negative strong effects are symmetric. AI may not bypass a dealbreaker, and
  total scores stay clamped to [0,100]; deterministic scoring with AI off is unchanged.
- A larger AI influence can reorder jobs and move them across MinScore. Treat this as an intended
  user-visible product change, not a guarantee that more jobs receive chips.

### Deterministic duplicate-evidence subset

Apply this small rule to evidence-valid, nonzero items before summing. Signal wording and
matched-goal wording are deliberately NOT part of duplicate identity:

- Presence identity: kind plus the existing tokenizer's ordered token sequence for the quote.
  Case/normalization/whitespace/punctuation differences that tokenize identically are one group.
- Absence identity: kind plus the sorted, unique collection of each form's ordered token sequence.
  Form ordering and repeated forms do not create a new identity. Preserve form boundaries.
- Clamp each evidence-valid item's delta to [-30,+30]. For same-identity, same-sign proposals,
  keep one proposal with the smallest absolute bounded delta. Adding repetitions cannot increase
  that group's magnitude. If both signs occur in a group, suppress the entire conflicting group.
- Pick an explanation deterministically among tied candidates by lexicographic ordering of their
  signal, matched-goal, quote and sorted forms; order groups by canonical identity. Preserve a
  real accepted quote, not synthetic reconstructed token text, for the displayed evidence.
- If proposed items exist but no group remains usable, classify the result as rejected, not
  explicit no-signal. If distinct usable groups cancel to zero, it remains rated with evidence.

This guarantees suppression for identical canonical evidence even if signal/goal wording changes.
Distinct passages or different synonym sets are not assumed semantically equivalent; broader
paraphrase duplication and double-counting deterministic categories remain explicit prompt
guidance, not a promised code guarantee. No semantic model or general deduplication framework.

## S5 — Versioning and old results

Advance the Stage-2 scoring prompt/contract version for prompt, magnitude and outcome semantics.
Do not rotate extraction or dealbreaker versions for a Stage-2-only change. Preserve old records
and the established stale-display contract; old empty records have unknown provenance. New-version
fresh analysis happens only through existing bounded authorized analysis triggers, never render.
Expect fresh cache misses and possible token spend on the next analysis; disclose this in release
notes. Do not automatically rerate every saved posting at startup or clear caches to raise yield.

## Acceptance matrix

- Prompt example and boundary fixtures: <6 characters, single token, invented/non-contiguous
  quote, profile-only quote, actual valid Korean span, full-description absence past truncation,
  and injection text. Existing rejected cases still fail; aligned valid cases survive.
- Parser-to-storage outcome fixtures: explicit empty, all-item-invalid, all-gate-rejected,
  partially accepted, nonempty zero-net, malformed/missing fields, provider errors and write errors.
- Repeat/cache fixtures: genuine-empty and rated repeats spend zero; rejected N remains unchanged;
  a later manual retry can succeed exactly once per row/run; auto-run, reload, reconnect and polling
  do not retry known rejected rows. Rejected success-path usage remains debited.
- User isolation, atomic persistence, per-surface N/M and summaries, legacy unknown empties,
  new-version fresh misses, unchanged Stage-1/dealbreaker identities and stale fallback.
- Magnitude fixtures prove supported +30/-30 items, aggregate abs(net)>30, both bounds, mixed-sign
  cancellation, invalid evidence contributing zero, consistent clipped explanation, total-score
  bounds, dealbreaker short-circuit and byte-identical AI-off behavior.
- Duplicate fixtures prove unchanged net for repetitions with the same bounded delta despite
  changed signal/goal wording, and unchanged/smaller PER-GROUP magnitude for differing same-sign
  deltas; normalized spelling/punctuation variants; reordered/repeated absence forms;
  minimum-magnitude same-sign selection; opposite-sign suppression; stable ties and results
  under item permutations; and contributions from genuinely distinct evidence groups. Suppressing
  a conflicting group may change cancellation with other groups; do not promise monotonic absolute
  NET magnitude for arbitrary new conflicting proposals.
- Owned browser fixture verifies all result cards on all three surfaces, partial failures,
  zero-net evidence, stale/current interaction, hard exclusions, both themes/mobile/keyboard and
  terminal progress behavior. Never use a personal/shared database or live provider for this.

No implementation or production acceptance is established by these proposed tests.

## Combined release boundary

The eventual reviewed release contains this feature plus the already reviewed default-200 and
progress/coffee-break/recovery edits inherited in the source baseline. Preserve explicit saved
100 and 200, live progress, interruption recovery, single ownership, and no duplicate calls.
Reuse valid unaffected receipts; rerun materially affected integration/browser checks on the
combined candidate. Release application code as ONE exact revision / immutable artifact, not
three separately deployed patches.

Signup-code activation is not a source edit or proof of effective runtime adoption. Retain its
existing reviewed stopped remaining-phase procedure and separate checkpoint; the earlier secret
publication and consumed operations must not be replayed. Any coordination with the same release
requires fresh applicable authority, current bindings, preserved unrelated fields/accounts,
correct ordering, effective runtime verification and separately authorized signup acceptance.
Never put the signup code or other secret values in these documents or the image.

Deployment publication/push, private migration, service replacement, secret activation, and
production acceptance remain separately scoped live gates. S6 defines the selected compatible
recovery design; do not defer its feasibility or claim the untouched old artifact can reopen a
new ledger. Bind actual artifacts, migration/backup and rollback commands at the release gate.

## S6 — Selected recovery design: prior behavior with the complete new migration manifest

The baseline runtime rejects any applied migration unknown to its embedded manifest. Therefore
the untouched previous immutable artifact is NOT the post-migration recovery target.

Prepare a separately reviewed **compatibility recovery build** from baseline b7ce9d5, carrying
the new additive migration files byte-identical to the forward feature candidate plus the minimal
matching embedded-manifest metadata. The recovery-source allowlist is those new migration files
and exactly two declarations in `internal/storage/store.go`: append their filename/SHA256 entries
to `pinnedPostgresMigrationDigests` and update `pinnedPostgresMigrationTree` to the complete embedded
PostgreSQL migration tree. Both forward and recovery builds must carry matching complete manifest
metadata; SQL files alone fail the baseline's pinned-file-set validation before ledger checks.
Preserve every existing migration and digest entry byte-for-byte, all validation algorithms, and
all baseline application/AI/UI behavior. Recovery thus retains the earlier cap/progress fixes and
prior scoring/outcome behavior, but knows the new ledger. This narrow metadata allowance does
not authorize a migration-framework refactor or changes to storage/application behavior.
The outcome migration adds separate state only; do not change/drop existing table meanings or
constraints needed by the baseline runtime. If implementation needs a wider change, stop and
revise this design before creating migration-dependent code.

Record distinct source commits and immutable digests for forward and compatibility recovery
artifacts. One forward deployment is still intended; the second artifact is prepared for recovery,
not a separate rollout. It must not be called the unchanged previous artifact. Building/testing
it locally is preparation; publishing either artifact or using recovery in production remains
separately authorized. No feature flag or weaker migration verification is introduced.

Owned-fixture acceptance MUST execute the actual recovery binary against the complete migrated
ledger, verify exact migration name/digest recognition, preserve representative post-upgrade user
writes, and exercise prior-behavior reads/writes and supported demo compatibility. Run baseline
unknown-version/digest-rejection controls unchanged; also keep the existing embedded-manifest
and pinned-tree identity controls green after the metadata update. Do not weaken count, filename,
digest, unknown-version, identity, or ledger checks. Independently review the complete normalized
recovery diff, including the two metadata declarations, with real authorized-worker provenance.
Rehearse switch back to forward behavior against the same preserved data. A source diff alone
is not the recovery rehearsal.

After new user writes, retain the authoritative database. Never delete ledger entries, weaken
unknown-version/digest checks, drop outcome state, or restore an older writable database to make
recovery work. Keep backup/recovery custody and runtime-secret rollback as separate checkpoints.
