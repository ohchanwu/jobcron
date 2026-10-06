# AI rating outcomes and weighting — implementation and combined-release plan

Status: revised draft R2 addressing review120's remaining manifest-metadata correction; awaiting independent re-review, not an execution envelope.
Created: 2026-10-06. Author: jobcron-orchestrator.
Governing [specification](../specs/261006-ai-rating-outcomes-and-weight.md).
Protected baseline: `b7ce9d585f3cdadf3eb12bff9c8b8e690f9a1964` on local main.

## Authority and execution tuple

- Board: jobcron only. Repository: `<REPOSITORY_ROOT>` (the Jobcron checkout).
  Exact private repository/worktree bindings remain on the native card, not in public prose.
- Prospective documents: controller-authored in the dedicated docs worktree,
  .worktrees/jobcron-ai-rating-spec-20261006, branch docs/jobcron-ai-rating-spec-20261006.
- Readiness reviewer: exactly jobcron-reviewer; readiness approval is not application approval.
- Subsequent implementation: exactly jobcron-worker (routine bounded default); jobcron-worker-glm-1
  or jobcron-worker-glm-2 only for a recorded capacity/capability reason on the matching board.
- Implementation worktree/branch must be allocated and recorded from the integrated approved-docs
  baseline before dispatch. Never implement in the docs worktree or dirty canonical checkout.
- Same-card application review: exactly jobcron-reviewer. Full exact source tuple and changed
  files/test evidence must appear in the native handoff; preserve rejected candidates/history.
- Initial implementation runtime allowance: three hours for discovery, vertical TDD slices,
  owned PostgreSQL/browser fixtures, full suite, commit and review handoff. No indefinite run.
- Preview: loopback 127.0.0.1:17777 with --no-open; never hijack the user's browser.
- Current phase authorizes specification/plan preparation and independent review. Local coding
  follows only after design readiness and confirmation that the owner's local bundle approval
  applies to the final reviewed scope. No future live effect is inferred from deployment intent.

Do not create another graph for the already completed cap/progress/diagnosis/signup-readiness
slices. This is a new feature specification, not a reopening of the report-only diagnosis.

## P0 — Review and freeze the small design

1. Independently review the complete S1–S6 and acceptance matrix. Consolidate material corrections
   on the same prospective review card; do not manufacture worker authorship for controller docs.
2. Review parser outcome classification, bounded outcome persistence, manual-versus-automatic
   retry, deterministic duplicate rules, clipped explanations and the selected S6 compatibility
   recovery build. Additive SQL alone does not allow an untouched older binary to reopen the
   ledger. Prefer one minimal outcome record, not a telemetry platform or persistent attempt ledger.
3. Use actual current source, storage migrations and docs index; externally named historical design
   files are absent and their content must not be invented. Record the owner-requested UX/weight
   changes as explicit scoped amendments to legacy prose.
4. Freeze reviewed document hashes, commit approved docs locally after required checks, and preserve
   the main checkout's unrelated AGENTS.md edits and private .superpowers artifacts.
5. Dispatch one cohesive implementation card for parser/gate/outcome/server/UI integration; split
   only if discovery establishes genuinely independently reviewable slices, not to fill lanes.

## P1 — Vertical TDD implementation

Read only active spec/plan, relevant current source and concise decision records. Retain current
pure-Go/provider boundaries and reuse existing renderer, storage and migrations. No new dependency
or scoring interface is assumed.

1. **Honest no-signal tracer:** explicit empty response -> persisted genuine-empty outcome and
   successful score cache -> successful N -> neutral card. Repeat spends zero. Watch the new
   behavior test fail before the minimum implementation and verify it green.
2. **Rejected tracer:** proposed-but-all-invalid / all-gate-rejected -> bounded rejection outcome,
   no success cache/N -> truthful rejection card after reload. Successful-call tokens are debited.
   A deliberate subsequent manual run may retry once; automatic runs and observation cannot.
3. **Partial and failure tracer:** surviving evidence persists atomically with its outcome;
   partial rejection and valid zero-net preserve evidence. Provider/parse/write failures cannot
   masquerade as genuine empty or complete success. Check interruption and cross-user isolation.
4. **Weight tracer:** align the prompt and example; enforce item +/-30 and aggregate +/-40;
   test strong signed results, cancellation, clipping/explanation agreement, score bounds,
   hard exclusions and AI-off invariance. Implement S4's canonical evidence grouping, conservative
   same-sign selection and opposite-sign suppression, with permutation/non-inflation fixtures.
   Broader semantic paraphrase/category non-duplication is prompt guidance, not a code guarantee.
5. **Version and surfaces tracer:** bump only Stage-2 contract identity; retain old caches as
   previous-setting evidence without inventing old empty provenance. Apply shared UI to Today,
   bookmarks and archive, including low-score/collapsed and stale/current interactions.
6. Update RERATE_NOTES.md, relevant maintained architecture/product explanations and release notes
   when implementation lands. Reconcile affected legacy AGENTS guidance in the isolated feature
   worktree, then integrate without overwriting the owner's concurrent canonical-file edits.
7. **Compatibility recovery artifact:** in a separately allocated exact Jobcron recovery worktree,
   start from b7ce9d5 and add the feature's reviewed new migration files, byte-identical, plus
   exactly the S6 manifest-metadata changes in `internal/storage/store.go`: append their filename/
   SHA256 entries in `pinnedPostgresMigrationDigests` and update `pinnedPostgresMigrationTree`.
   Require matching complete embedded-manifest metadata in both forward and recovery builds.
   Preserve every old migration/digest entry, all baseline application/AI/UI behavior, and all
   validation algorithms. The recovery-source allowlist is only the new migration files and those
   two declarations; no wider framework/storage change is authorized. Record a distinct source
   commit and independently review the complete normalized delta, including metadata-only Go
   changes, with real worker provenance. No recovery code is implemented by the controller and
   no recovery artifact is deployed during local preparation. Stop for a design amendment if
   wider source/schema changes prove necessary.

Each tracer uses RED -> GREEN -> REFACTOR, not all tests then all code. Do not bypass citation
verification to make tests pass. No live provider calls or performance/yield claims are required.

## P2 — Proportional independent verification

Use the exact available Go toolchain. Ordinary broad checks must REMOVE optional database/browser
probe environment activation. Never use shared PostgreSQL port 55432 or a personal database.

Required ordinary gates from the candidate worktree:

```sh
go build ./cmd/jobcron
go test -timeout=8m ./...
go vet ./...
gofmt -l .
git diff --check
node web/testdata/ai-rerate-lifecycle.test.js
node web/testdata/ai-rerate-owner.test.js
node web/testdata/bookmark-lifecycle.test.js
```

Record actual toolchain, environment boundary, exact command, exit and cache/skip semantics.
Run focused parser/gate/scoring/outcome tests and a targeted race regression where concurrency
changed; confirm new tests actually execute rather than succeeding through skips.

For storage migrations, server flows and browser acceptance, reuse the already reviewed owned
fixture approach at its stated scope, adapted only for the new exact candidate and assertions.
First record the uniquely owned container/process/endpoint and provenance. Scope disposable
PostgreSQL activation to inspected targeted package commands, never a broad subprocess suite.
Record cleanup outcomes and verify exact owned-resource absence; do not rerun completed tests or
mutations to make an observer's cleanup false-negative look green.

Before release readiness, actually rehearse the S6 recovery binary on that owned fully migrated
database: validate complete ledger names/digests, retain data written through forward behavior,
exercise representative recovery reads/writes, then return to forward behavior without restoring
an old database. Verify both binaries' schema views and applicable SQLite/demo compatibility.
Keep the existing unknown-version/digest rejection tests and embedded-manifest/pinned-tree identity
controls green after updating metadata; preserve count, filename, digest, unknown-version,
identity, and ledger checks. Retain the source/build identity and observed results for BOTH
artifacts; static source/migration/metadata equality is not runtime proof.

Exercise real embedded assets and mux in Chromium on 17777, --no-open. Verify the new cards,
state labels, no false scores, fresh N/M, keyboard/mobile/theme behavior and the existing live
progress/recovery/no-duplicate-start contract. Use synthetic providers with counted calls,
not live credentials. Preserve raw evidence only locally; publish sanitized summaries.

Native implementation handoff must include exact new commit, base/common repository, changed
files, test/fixture/browser output paths and cleanup receipts. jobcron-reviewer reviews the actual
candidate and affected integration paths, not just worker claims. Failure findings remain on the
same card with true implementation provenance and bounded repair accounting when opted in.

## P3 — Integrate ONE application release candidate

1. Integrate independently approved feature code locally, preserving exact reviewed ancestry and
   unaffected cap/progress commits. The new combined release must descend from baseline b7ce9d5.
2. Run affected combined gates on that candidate. Reuse valid earlier evidence for unchanged
   surfaces; do not reopen the entire provider diagnosis or replicate old checker campaigns.
3. Verify explicit cap 100/200 and unset/effective 200; neutral/rejected/rated/stale UI; live
   progress, coffee-break copy, interrupted-stream recovery, no duplicate calls; and schema
   startup/read-write compatibility of the S6 recovery build with the migrated ledger and new
   data. Record forward reviewed/integrated revisions and the distinct recovery revision.
4. Freeze one exact clean application revision and later one immutable release artifact digest
   for the production host's independently established target platform. No mutable-tag deployment.
   Prepare the separately reviewed compatibility recovery artifact/digest as well; it is not the
   untouched old binary and not another forward deployment. Neither image is published here.

Deliverable is a reviewed, tested local combined application candidate plus concise release notes.
Documentation approval alone does not meet that deliverable.

## P4 — Later combined production release, not three deployments

The owner wants the changes deployed together after completion. Prepare one concise release and
rollback handoff using the existing supported production path and preserved valid evidence.
Do not build a new deployment controller or recheck unrelated infrastructure by default.

Before live execution, identify the actual candidate/ref/digest, current runtime/rollback identity,
maintenance scope/window, exact commands/effects, usable credential context, backup/recovery and
the reviewed S6 recovery artifact plus rehearsed migration compatibility, then obtain fresh
applicable gate-specific authority and
independent exact-command review. Prior consumed/expired packets are immutable and unusable.
Push, image publication, migration and runtime replacement are distinct effects; this plan does
not provide their missing call bounds or time windows.

Release contents:

- Earlier reviewed default analysis cap 200, preserving explicitly saved settings.
- Earlier reviewed live progress, lifecycle/recovery and approximate coffee-break guidance.
- This feature's evidence-aligned prompt, truthful outcomes/UI and bounded stronger deltas.

Signup-code activation remains a SEPARATE runtime-secret checkpoint, not a value in source/image.
Reuse its existing stopped remaining-phase preparation without replaying already completed or
uncertain secret publication. If the owner wants it coordinated with this release, review the
current remaining-only effects and ordering to avoid an unnecessary second restart; require fresh
secret-operation authority and effective runtime acceptance. Successful signup testing can create
an account/session and needs its own explicit creation/cleanup scope. A GET page or rejected form
is not proof that the intended new code is accepted. Preserve unrelated secret fields and accounts.

Production acceptance after the combined release covers HTTPS/session basics, all affected AI
states and progress, saved-cap persistence, actual application revision and migration identity,
service stability and any separately authorized signup acceptance. Do not automatically initiate
paid AI requests under ordinary HTTP/browser acceptance. Distinguish synthetic fixture proof,
automated public checks and genuinely exercised production behavior.

Post-migration recovery selects the separately reviewed compatibility recovery artifact carrying
the complete new ledger manifest and baseline behavior; the untouched old binary is not usable
against that ledger. Bind its exact source/digest and successful owned-fixture rehearsal before
the release mutation. Retain the authoritative current database/new user writes and outcome data;
do not delete ledger records, weaken validation, or restore a stale writable database. Retain
runtime-secret recovery material and its separate semantics. One forward deployment remains the
intent; publishing/using the recovery artifact requires its own applicable live authority. No DNS,
ingress, topology, host replacement, unrelated cleanup or provider switch is implicit.

## Completion boundaries

- Design phase done: independently reviewed spec/plan, stable local files and recorded exact hashes.
- Implementation done: independently approved exact application candidate and observed checks.
- Local bundle done: combined candidate contains all reviewed edits and affected checks pass.
- Deployed bundle done: authorized immutable artifact is running, applicable migration/recovery,
  functional acceptance and stability gates are actually verified. Secret activation is separately
  reported; no app-release completion may imply it succeeded.
