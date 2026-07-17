# Ponytail Reduction Candidate Ledger

Task 3 recorded the independent evidence set. Task 4 rechecked every candidate at Mayor base
`2b3046c170cb1667c880bf5c3b889aaafe8a1469`, applied every acceptance condition, and approved
only the ordered batches recorded below.

## Baseline

- Reviewed base: `efa0ec6d5fd98bcc4225dea3f843dbaea074a229`.
- Ponytail mode: `full`, activated by the supported Codex command and verified before audit.
- Audit scope: 120 tracked production Go, web, and script files; generated, vendored, `dist/`,
  and archive content excluded.
- Scope size: 91 production Go files, 29 web or script files, 20,014 lines total.
- Ponytail audit: eight ranked findings; estimated ceiling of 330 production lines and zero
  direct dependencies removable before semantic review.
- `dupl` v1.1.0: seven production clone families at threshold 100 and three test clone
  families at threshold 150.
- `deadcode` v0.47.0: four findings common to darwin/arm64, linux/amd64, linux/arm64, and
  windows/amd64 when run with tests.
- `go mod tidy -diff`: empty output.
- Direct modules: seven external direct dependencies; each has an owned current capability.
- Ignored evidence:
  - `.superpowers/sdd/260717-ponytail/ponytail-audit.md`
  - `.superpowers/sdd/260717-ponytail/dupl-production.txt`
  - `.superpowers/sdd/260717-ponytail/dupl-tests.txt`
  - `.superpowers/sdd/260717-ponytail/deadcode/`
  - `.superpowers/sdd/260717-ponytail/go-mod-tidy.diff`
  - `.superpowers/sdd/260717-ponytail/direct-modules.txt`

The plan's raw cross-target `GOOS` and `GOARCH` commands cross-compiled the analyzer itself.
Those preserved failures are tool-invocation evidence, not code findings. Corrected scans used
one native darwin/arm64 binary built from the same pinned module, then ran that binary under
each target environment. The ignored evidence records the binary module metadata and SHA-256.

### Direct dependency inventory

- `github.com/jackc/pgx/v5` v5.10.0: pure-Go PostgreSQL driver. Production imports are
  `internal/storage/store.go` and `cmd/jobcron-import/main.go`; PostgreSQL contract tests also
  import its standard-library adapter.
- `github.com/pkg/browser` at `5ac0b6a4141c`: cross-platform default-browser launch from
  `cmd/jobcron/main.go`. Removing it would change the local startup flow.
- `golang.org/x/crypto` v0.54.0: Argon2id password hashing in
  `internal/auth/password.go`.
- `golang.org/x/term` v0.45.0: no-echo terminal password input in
  `cmd/jobcron-user/main.go`.
- `golang.org/x/text` v0.40.0: NFC normalization in AI extraction, AI citation gating,
  profile hashing, and scoring tokenization.
- `gopkg.in/yaml.v3` v3.0.1: structural parsing in deployment, release, local database,
  and CI contract tests. Hand-written YAML parsing would add risk and code.
- `modernc.org/sqlite` v1.50.1: pure-Go SQLite compatibility for storage, snapshot, and
  import paths. It preserves the no-CGO distribution contract.

No direct dependency has a smaller safe standard-library or native replacement.

## Task 3 Accepted-for-Triage Candidates

These findings are admitted to Task 4 for full semantic tracing and acceptance review.

### PONY-002: Share the robots.txt parser primitive

- Evidence: Ponytail `shrink`; `dupl` production clone family.
- Files and symbols:
  - `internal/scraper/demoday/demoday.go`: `robotsAllows`
  - `internal/scraper/greenhouse/greenhouse.go`: `robotsAllows`
  - `internal/scraper/greeting/greeting.go`: `robotsAllows`
  - `internal/scraper/jumpit/client.go`: `robotsAllows`
  - `internal/scraper/rallit/client.go`: `robotsAllows`
- Observable behavior: parse the same wildcard-agent, allow/disallow, longest-match-wins
  RFC 9309 subset. Fetch, failure, host, path, and cache policy stay source-owned.
- Callers and non-Go consumers: each source's access check; no template, JavaScript, SQL,
  migration, reflection, or configuration consumer.
- Owner: the shared scraper package, limited to the stable parser primitive.
- Behavior locks: `internal/scraper/jumpit/client_test.go:TestRobotsAllows` plus each
  source's access tests. Task 4 must add cross-package characterization before moving code.
- Expected reduction: about 140 production lines; zero dependencies.
- Risk and rollback: medium because robots decisions gate network access. One reversible
  commit; keep every source policy outside the shared primitive.
- Status: `accepted` for Task 4 evidence because five current consumers implement the same
  stable primitive.

### PONY-003: Share request-start pacing

- Evidence: Ponytail `shrink`; `dupl` production clone family.
- Files and symbols: `waitForRateLimit` in `internal/ai/client.go` and the Demoday,
  Greenhouse, Greeting, Jumpit, Rallit, and Worknet scraper clients.
- Observable behavior: reserve the next request start under a mutex, release the lock before
  waiting, and return promptly on context cancellation.
- Callers and non-Go consumers: provider completion and scraper HTTP helpers; no non-Go
  consumers.
- Owner: a narrow request-pacing package or an existing owner chosen by Task 4. Timing values
  and provider/source error policy remain local.
- Behavior locks: `internal/scraper/jumpit/client_test.go:TestClientGetRateLimits`, provider
  pacing tests, and source client tests. Task 4 must characterize concurrent callers.
- Expected reduction: about 95 production lines; zero dependencies.
- Risk and rollback: high because request pacing protects external rate limits and concurrent
  calls. One reversible semantic-cluster commit after a child plan.
- Status: `accepted` for Task 4 evidence because seven live consumers share the same algorithm.

### PONY-004: Reuse one storage posting-row scanner

- Evidence: Ponytail `shrink`; `dupl` production clone families in user-state listings.
- Files and symbols: four `scanPosting` row loops in `internal/storage/bookmarks.go` and
  `internal/storage/not_interested.go`; existing primitive `scanPosting` lives in
  `internal/storage/postings.go`.
- Observable behavior: scan all rows in order, stop on the first scan error, and return the
  accumulated postings with `rows.Err()`.
- Callers and non-Go consumers: bookmark and hidden-posting list methods for SQLite and
  PostgreSQL; SQL remains in each method. No non-Go consumer.
- Owner: `internal/storage/postings.go`, beside `scanPosting`; do not parameterize table names.
- Behavior locks: ordered-list tests in `internal/storage/bookmarks_test.go` and
  `internal/storage/not_interested_test.go`, plus PostgreSQL user-scope tests.
- Expected reduction: about 30 production lines; zero dependencies.
- Risk and rollback: low if queries and error text stay local. One reversible storage commit.
- Status: `accepted` for Task 4 evidence because the repeated row-consumption policy is exact.

### PONY-005: Remove the unused scheduler handle

- Evidence: Ponytail `yagni`; all-target `deadcode` finding for `Scheduler.Done`.
- Files and symbols: `Scheduler`, `Scheduler.done`, `Scheduler.Done`, and `StartScheduler` in
  `internal/server/scheduler.go`; ignored return handle in `cmd/jobcron/main.go`.
- Observable behavior: callers use only startup validation and the background loop. No caller
  observes the returned handle or channel.
- Callers and non-Go consumers: `cmd/jobcron/main.go` and two scheduler tests; no non-Go
  consumer.
- Owner: `internal/server` scheduler startup.
- Behavior locks: `TestStartSchedulerRunsScheduledScrapeAfterSleep` and
  `TestStartSchedulerRecordsSkippedRunWhenScrapeLockBusy`.
- Expected reduction: about 10 production lines; zero dependencies.
- Risk and rollback: low, but the exported return signature changes inside this module. One
  reversible commit including caller and test updates.
- Status: `accepted` for Task 4 evidence because the handle is ignored by every caller.

### PONY-006: Remove `buildModelText`

- Evidence: Ponytail `delete`; all-target `deadcode` finding.
- Files and symbols: `internal/ai/extract.go:buildModelText`; two build-tagged uses in
  `internal/ai/spike_test.go`.
- Observable behavior: duplicate `ModelInput` text assembly and truncation while discarding
  its content hash.
- Callers and non-Go consumers: optional local-AI spike tests only; no production or non-Go
  caller.
- Owner: `internal/ai.ModelInput`.
- Behavior locks: `TestBuildModelTextTruncationAndHashStability`; optional spike tests can
  ignore the extra `ModelInput` result.
- Expected reduction: about 13 production lines; zero dependencies.
- Risk and rollback: low. One reversible AI commit with tagged-test compilation.
- Status: `accepted` for Task 4 evidence because the existing owner is behavior-equivalent.

### PONY-007: Remove `DefaultKeysPath`

- Evidence: Ponytail `delete`; all-target `deadcode` finding.
- Files and symbols: `internal/ai/keys.go:DefaultKeysPath`.
- Observable behavior: exported wrapper around private `defaultKeysPath(os.UserConfigDir)`.
  The current importer accepts an explicit legacy key path and has no default-path caller.
- Callers and non-Go consumers: none in tracked Go, templates, JavaScript, scripts,
  configuration, migrations, or reflection.
- Owner: explicit legacy import wiring and private `defaultKeysPath` test seam.
- Behavior locks: `TestDefaultKeysPathUsesCanonicalApplicationDirectory` covers the private
  path builder.
- Expected reduction: six production lines; zero dependencies.
- Risk and rollback: low because `internal/ai` cannot be imported outside the parent module.
  One reversible AI commit.
- Status: `accepted` for Task 4 evidence because the wrapper has no reachable consumer.

### PONY-008: Remove `Server.registeredSources`

- Evidence: Ponytail `delete`; all-target `deadcode` finding.
- Files and symbols: `internal/server/sources.go:Server.registeredSources`.
- Observable behavior: return the internal scraper slice unchanged.
- Callers and non-Go consumers: none. The template function named `registeredSources` binds
  to `Server.allRegisteredSources`, not this method.
- Owner: `Server.allRegisteredSources` for template options and `Server.sources` internally.
- Behavior locks: server template rendering and source-filter tests; Task 4 should add a
  narrow name-binding assertion if needed.
- Expected reduction: four production lines; zero dependencies.
- Risk and rollback: low. One reversible server commit.
- Status: `accepted` for Task 4 evidence because the similarly named template binding is a
  different live symbol.

## Task 3 Rejected Candidates

### PONY-009: Merge bookmark and not-interested HTTP handlers

- Evidence: `dupl` production and test clone families.
- Files and symbols: add/remove handlers and JSON writers in
  `internal/server/bookmarks.go` and `internal/server/not_interested.go`.
- Observable behavior: bookmark state remains visible while muted state filters most lists;
  JSON field names and storage calls differ.
- Callers and non-Go consumers: separate routes, JavaScript state handlers, and templates.
- Owner: each user-state domain.
- Behavior locks: bookmark, not-interested, hidden, archive, and production user-scope tests.
- Expected reduction: fewer than 15 production lines after callbacks and parameters; zero
  dependencies.
- Risk and rollback: medium; one server commit could revert it.
- Status: `rejected` because same shape serves different user policy and a generic handler
  would hide those differences.

### PONY-010: Parameterize bookmark and not-interested storage tables

- Evidence: `dupl` production clone families.
- Files and symbols: ID, existence, and joined-list methods in
  `internal/storage/bookmarks.go` and `internal/storage/not_interested.go`.
- Observable behavior: separate tables, timestamp columns, error language, visibility policy,
  and public methods across SQLite and PostgreSQL.
- Callers and non-Go consumers: server state surfaces and migration-owned table names.
- Owner: each storage domain; only the row loop in PONY-004 is safely shared.
- Behavior locks: both storage suites, user isolation, sweep exemptions, and PostgreSQL tests.
- Expected reduction: uncertain and likely offset by dynamic SQL plumbing; zero dependencies.
- Risk and rollback: high because identifier parameterization weakens query clarity and safety.
- Status: `rejected`; PONY-004 captures the stable primitive without a catch-all abstraction.

### PONY-011: Generalize legacy import row copies

- Evidence: `dupl` production clone family for `copyBookmarks` and `copyNotInterested`.
- Files and symbols: `cmd/jobcron-import/main.go` copy functions.
- Observable behavior: move distinct tables and timestamp columns inside a verified,
  rollback-safe SQLite-to-PostgreSQL transaction.
- Callers and non-Go consumers: import apply pipeline and operational recovery contract.
- Owner: each migration category.
- Behavior locks: representative import, rollback-at-every-boundary, fingerprint, and
  post-commit verification tests.
- Expected reduction: about 15 production lines only with dynamic identifiers; zero
  dependencies.
- Risk and rollback: high because migration safety is a global no-reduction boundary.
- Status: `rejected` because explicit table copies are safer than generic migration plumbing.

### PONY-012: Merge complete scraper HTTP and robots-cache helpers

- Evidence: `dupl` production clone families beyond the primitives in PONY-002 and PONY-003.
- Files and symbols: per-source `get`, `checkRobotsHost`, `cacheRobots`, and access methods.
- Observable behavior: source-specific hosts, headers, paths, status handling, failure posture,
  cache keys, and error messages.
- Callers and non-Go consumers: every scraper implementation; no direct non-Go consumer.
- Owner: each concrete scraper package.
- Behavior locks: source integration and client tests.
- Expected reduction: superficially large but offset by configuration and callbacks; zero
  dependencies.
- Risk and rollback: high across network access policy.
- Status: `rejected`; share only the stable parser and pacing primitives already isolated.

### PONY-013: Consolidate cloned test scenarios

- Evidence: three `dupl` test clone families across server and storage user-state tests.
- Files and symbols: bookmark, hidden, and not-interested test setup and assertions.
- Observable behavior: each scenario names a distinct user action and failure surface.
- Callers and non-Go consumers: test runner only.
- Owner: each behavior suite.
- Behavior locks: the tests themselves.
- Expected reduction: test lines only; zero production lines and dependencies.
- Risk and rollback: low, one test commit.
- Status: `rejected` because the campaign forbids manufacturing reduction from tests and the
  duplicated setup keeps failures local and legible.

## Task 3 Separate-Decision Candidate

### PONY-001: Share the exact-token matching primitive

- Evidence: tracked plan seed; Ponytail `shrink`; source comparison.
- Files and symbols:
  - `internal/scoring/match.go`: `tokenize`, `textContains`
  - `internal/ai/score_delta.go`: `gateTokenize`, `tokenSubsequence`
- Package guidance and import graph: `internal/scoring` imports `internal/ai`; therefore AI
  cannot import scoring. A narrow third package can be imported by both without a cycle.
- Observable behavior: NFC-normalize, split maximal Unicode letter/digit runs, lowercase,
  then match an exact contiguous token sequence. Empty phrases never match.
- Callers and non-Go consumers:
  - scoring keyword rules call `textContains`;
  - scoring title and dedup rules call `tokenize`;
  - AI presence and absence citation gates call `gateTokenize` and `tokenSubsequence`.
  - FTS5 schema and user documentation define the same token-exact contract;
  - no template or JavaScript calls the Go functions.
- Policy boundary: scoring decides awards and dealbreakers. AI decides whether quoted or
  absent evidence survives the citation gate. Only tokenization and contiguous matching are
  shared; those policies stay separate.
- Owner: a narrow lower-level exact-token package, not `util`, `helpers`, scoring, or AI.
- Behavior locks: `TestTokenize`, `TestTextContains`, `TestGateTokenizeInvariants`,
  `TestGateDeltaPresence`, `TestGateDeltaAbsence`, and token-exact scoring tests.
- Expected reduction: about 25 net production lines after the new owner and imports; zero
  dependencies.
- Risk and rollback: medium because divergence could change scores or citation acceptance.
  One reversible architecture commit after paired characterization tests pass.
- Status: `separate decision` because creating a new lower-level package changes ownership even
  though the primitive is stable and already duplicated.

## Task 4 Semantic Triage

The recheck covered definitions, Go callers, templates, JavaScript, SQL and migrations,
reflection and registration, build tags, configuration, tests, and focused Git history.
No consumer beyond the Task 3 inventory was found. The six semantic classes below are the
classes defined by the campaign plan; reachability-only PONY-005 is not a duplicate candidate.

The gate labels are: contract, consumers, coverage, owner, reduction, design, and rollback.
`Design` passes only when the change avoids a speculative or catch-all abstraction and an
import cycle. Batch-shape constraints are applied after the seven-condition gate.

### PONY-001 gate: approved

- Semantic class: 4, one stable primitive with separate scoring and AI policies.
- Contract: pass — exact contiguous matching over normalized Unicode letter/digit tokens.
- Consumers: pass — scoring, deduplication, AI citation gates, FTS5, and docs are traced.
- Coverage: pass — tokenizer, phrase, citation, scoring, and deduplication tests lock behavior.
- Owner: pass — `internal/tokenmatch` is a narrow owner justified by two current packages.
- Reduction: pass — about 25 net production lines and no dependency change.
- Design: pass — both packages import down to the owner; neither imports the other.
- Rollback: pass — one three-production-file semantic commit restores both local copies.
- Decision: approve as `PT4-005`; the Task 3 ownership question is resolved narrowly.

### PONY-002 gate: approved in two batches

- Semantic class: 4, one parser primitive with source-owned fetch and failure policies.
- Contract: pass — all five copies implement the same RFC 9309 subset.
- Consumers: pass — five source access checks are the only consumers.
- Coverage: pass — source access tests exist and shared characterization is practical.
- Owner: pass — the existing `internal/scraper` package owns cross-source primitives.
- Reduction: pass — about 140 production lines and no dependency change.
- Design: pass — only parsing moves; hosts, caches, paths, and error policy remain local.
- Rollback: pass — each ordered conversion is one commit; full rollback runs in reverse order.
- Decision: approve as `PT4-006` and `PT4-007`. The first batch adds the owner and converts
  four consumers with a negative delta; the remaining one-file conversion is also negative.

### PONY-003 gate: approved in two batches

- Semantic class: 4, one request-start primitive with caller-owned timing and error policy.
- Contract: pass — all seven copies reserve starts under a mutex and honor cancellation.
- Consumers: pass — one AI provider and six scraper clients are the only consumers.
- Coverage: pass — existing timing tests plus concurrent characterization can lock behavior.
- Owner: pass — seven current consumers justify a narrow `internal/pacing` owner.
- Reduction: pass — about 95 production lines and no dependency change.
- Design: pass — a concrete pacer needs no interface, callback, or catch-all helper.
- Rollback: pass — each ordered conversion is one commit; full rollback runs in reverse order.
- Decision: approve as `PT4-008` and `PT4-009`. The first batch adds the owner and converts
  four consumers with a negative delta; the remaining three-file conversion is also negative.

### PONY-004 gate: approved

- Semantic class: 3, the existing storage scanner already owns row decoding.
- Contract: pass — scan rows in order, stop on scan failure, then return `rows.Err()`.
- Consumers: pass — four bookmark and not-interested listing loops are traced.
- Coverage: pass — ordered-list and PostgreSQL user-scope tests lock the behavior.
- Owner: pass — `internal/storage/postings.go` already owns `scanPosting`.
- Reduction: pass — about 30 production lines and no dependency change.
- Design: pass — SQL, table names, timestamps, and error policy stay in each domain.
- Rollback: pass — one three-production-file storage commit is self-contained.
- Decision: approve as `PT4-004`.

### PONY-005 gate: approved

- Semantic class: not applicable; this is a reachability and API-surface candidate.
- Contract: pass — start validation and the background scheduler loop are preserved.
- Consumers: pass — the command and two scheduler tests ignore the returned handle.
- Coverage: pass — scheduled-run and busy-lock tests exercise the live behavior.
- Owner: pass — `internal/server` owns scheduler startup and lifecycle.
- Reduction: pass — about 10 production lines and no dependency change.
- Design: pass — deletion removes an unused wrapper instead of adding an abstraction.
- Rollback: pass — one two-production-file commit restores the old signature and handle.
- Decision: approve as `PT4-010`.

### PONY-006 gate: approved

- Semantic class: 3, `ModelInput` already owns text assembly, truncation, and hashing.
- Contract: pass — callers receive identical model text while ignoring the extra hash.
- Consumers: pass — only two `aispike`-tagged test calls use `buildModelText`.
- Coverage: pass — `TestBuildModelTextTruncationAndHashStability` locks the owner.
- Owner: pass — `internal/ai.ModelInput` is the existing behavior owner.
- Reduction: pass — about 13 production lines and no dependency change.
- Design: pass — reuse deletes a duplicate wrapper without new plumbing.
- Rollback: pass — one production file plus tagged tests forms one reversible AI commit.
- Decision: approve as `PT4-003`.

### PONY-007 gate: approved

- Semantic class: 3, the private `defaultKeysPath` seam already owns path construction.
- Contract: pass — the exported wrapper only supplies `os.UserConfigDir`.
- Consumers: pass — no Go, non-Go, reflection, configuration, or migration consumer exists.
- Coverage: pass — the private path-builder test locks the canonical directory contract.
- Owner: pass — explicit importer wiring and the private test seam remain authoritative.
- Reduction: pass — about six production lines and no dependency change.
- Design: pass — deletion removes an unreachable wrapper and adds nothing.
- Rollback: pass — one production-file commit restores the wrapper.
- Decision: approve as `PT4-001`.

### PONY-008 gate: approved

- Semantic class: 3, `Server.sources` and `allRegisteredSources` already own live behavior.
- Contract: pass — the dead method returns the internal scraper slice unchanged.
- Consumers: pass — no caller exists; the same-named template binding uses another method.
- Coverage: pass — a narrow function-map binding assertion can lock the name distinction.
- Owner: pass — the field owns internal iteration and `allRegisteredSources` owns templates.
- Reduction: pass — about four production lines and no dependency change.
- Design: pass — deletion adds no abstraction and preserves registration order.
- Rollback: pass — one production-file server commit restores the method.
- Decision: approve as `PT4-002`.

### PONY-009 gate: rejected

- Semantic class: 2, the HTTP handlers share shape but enforce different user policy.
- Contract: fail — bookmark visibility and mute filtering are not one behavior contract.
- Consumers: pass — routes, JavaScript, templates, storage calls, and tests are traced.
- Coverage: pass — bookmark, hidden, archive, and user-scope suites lock both policies.
- Owner: fail — each user-state domain is the clear owner; no shared owner exists.
- Reduction: pass — callbacks could remove fewer than 15 production lines.
- Design: fail — a generic callback handler would hide JSON and visibility differences.
- Rollback: pass — a two-production-file server change could be reverted in one commit.
- Decision: reject; preserve the separate handlers.

### PONY-010 gate: rejected

- Semantic class: 2, storage methods share shape but own different state policy.
- Contract: fail — tables, timestamps, errors, visibility, and public methods differ.
- Consumers: pass — server surfaces, SQLite, PostgreSQL, and migrations are traced.
- Coverage: pass — storage, isolation, sweep, and PostgreSQL tests lock both domains.
- Owner: fail — each table domain owns its operations; only PONY-004 has a shared owner.
- Reduction: fail — dynamic SQL plumbing offsets an uncertain deletion.
- Design: fail — identifier parameterization weakens query clarity and safety.
- Rollback: pass — a two-production-file storage change could be reverted in one commit.
- Decision: reject; share only row consumption through PONY-004.

### PONY-011 gate: rejected

- Semantic class: 5, explicit duplication preserves the import transaction boundary.
- Contract: fail — the two categories own different tables, timestamps, and verification.
- Consumers: pass — apply, rollback, fingerprint, and post-commit paths are traced.
- Coverage: pass — representative and every-boundary rollback tests lock import safety.
- Owner: fail — each migration category is intentionally explicit.
- Reduction: pass — dynamic identifiers could remove about 15 production lines.
- Design: fail — generic migration plumbing weakens a no-reduction safety boundary.
- Rollback: pass — the importer change could be reverted in one commit.
- Decision: reject; retain the explicit copies.

### PONY-012 gate: rejected

- Semantic class: 2, HTTP helpers share shape but enforce source-specific policy.
- Contract: fail — hosts, headers, status handling, failure posture, and cache keys differ.
- Consumers: pass — every concrete scraper and source test is traced.
- Coverage: pass — client and integration suites lock the source policies.
- Owner: fail — each concrete scraper owns its network policy.
- Reduction: fail — configuration and callbacks offset the apparent clone deletion.
- Design: fail — a complete shared helper would be a catch-all network abstraction.
- Rollback: pass — a semantic scraper commit could restore the local helpers.
- Decision: reject; PONY-002 and PONY-003 already isolate the only stable primitives.

### PONY-013 gate: rejected

- Semantic class: 6, test-only setup duplication across distinct scenarios.
- Contract: pass — each test clearly names its user action and failure surface.
- Consumers: pass — only the test runner consumes the setup.
- Coverage: pass — the duplicated tests are the behavior locks.
- Owner: fail — each behavior suite is clearer as its own owner.
- Reduction: fail — the proposal removes no production code or dependency.
- Design: fail — a shared test helper would hide scenario-specific setup and failures.
- Rollback: pass — a test-only commit could be reverted independently.
- Decision: reject; test deletion cannot manufacture campaign reduction.

## Approved Ordered Batches

### PT4-001: remove the unused default AI-key path wrapper

- Candidate: PONY-007.
- Behavior owner: private `defaultKeysPath` plus explicit importer path wiring.
- Production files: `internal/ai/keys.go`.
- Behavior lock: `TestDefaultKeysPathUsesCanonicalApplicationDirectory`.
- Estimated delta: minus six production lines; zero dependencies.
- Rollback boundary: one AI-key-path commit restoring one wrapper.
- Reversibility: no later batch depends on the exported wrapper or this deletion.

### PT4-002: remove the unused registered-source method

- Candidate: PONY-008.
- Behavior owner: `Server.sources` and `Server.allRegisteredSources`.
- Production files: `internal/server/sources.go`.
- Behavior lock: a function-map binding assertion plus existing profile rendering tests.
- Estimated delta: minus four production lines; zero dependencies.
- Rollback boundary: one server-source commit restoring one private method.
- Reversibility: no consumer or later batch depends on the deleted method.

### PT4-003: replace `buildModelText` with `ModelInput`

- Candidate: PONY-006.
- Behavior owner: `internal/ai.ModelInput`.
- Production files: `internal/ai/extract.go`.
- Behavior lock: `TestBuildModelTextTruncationAndHashStability` and `aispike` compilation.
- Estimated delta: minus 13 production lines; zero dependencies.
- Rollback boundary: one AI model-input commit with its tagged-test call-site updates.
- Reversibility: no other approved batch changes model-input assembly.

### PT4-004: reuse one storage posting-row collector

- Candidate: PONY-004.
- Behavior owner: `internal/storage/postings.go` beside `scanPosting`.
- Production files: `internal/storage/postings.go`, `internal/storage/bookmarks.go`, and
  `internal/storage/not_interested.go`.
- Behavior lock: ordered bookmark and mute tests plus PostgreSQL user-scope tests.
- Estimated delta: minus 30 production lines; zero dependencies.
- Rollback boundary: one storage-row-consumption commit; SQL remains untouched.
- Reversibility: no other approved batch changes storage queries or row scanning.

### PT4-005: share exact-token matching below scoring and AI

- Candidate: PONY-001.
- Behavior owner: new narrow `internal/tokenmatch/tokenmatch.go`.
- Production files: `internal/tokenmatch/tokenmatch.go`, `internal/scoring/match.go`, and
  `internal/ai/score_delta.go`.
- Behavior lock: tokenizer, phrase, citation, score, and deduplication tests.
- Estimated delta: minus 25 net production lines; zero dependencies.
- Rollback boundary: one token-primitive commit restores both policy-local copies.
- Reversibility: no other approved batch imports or changes `internal/tokenmatch`.

### PT4-006: add the shared robots parser and convert four sources

- Candidate: PONY-002.
- Behavior owner: new `internal/scraper/robots.go`, limited to parsing and path matching.
- Production files: `internal/scraper/robots.go`, `internal/scraper/demoday/demoday.go`,
  `internal/scraper/greenhouse/greenhouse.go`, `internal/scraper/greeting/greeting.go`, and
  `internal/scraper/jumpit/client.go`.
- Behavior lock: shared parser characterization plus each converted source's access tests.
- Estimated delta: at least minus 100 production lines; zero dependencies.
- Rollback boundary: one parser-owner commit restores the four local copies.
- Reversibility: it is self-contained at its ordered checkpoint; after `PT4-007`, full rollback
  reverts `PT4-007` first and then this single commit.

### PT4-007: convert the remaining robots parser consumer

- Candidate: PONY-002.
- Behavior owner: `internal/scraper/robots.go` from `PT4-006`.
- Production files: `internal/scraper/rallit/client.go`.
- Behavior lock: the shared characterization and Rallit access tests.
- Estimated delta: about minus 40 production lines; zero dependencies.
- Rollback boundary: one Rallit commit restores its local parser and removes its shared call.
- Reversibility: reverting it does not change the owner or the first four consumers.

### PT4-008: add the shared request pacer and convert four consumers

- Candidate: PONY-003.
- Behavior owner: new concrete `internal/pacing/pacing.go`; callers retain timing policy.
- Production files: `internal/pacing/pacing.go`, `internal/ai/client.go`,
  `internal/scraper/demoday/demoday.go`, `internal/scraper/greenhouse/greenhouse.go`, and
  `internal/scraper/greeting/greeting.go`.
- Behavior lock: new concurrent-start and cancellation characterization plus current client
  timing tests.
- Estimated delta: about minus 45 production lines; zero dependencies.
- Rollback boundary: one pacer-owner commit restores the four local implementations.
- Reversibility: it is self-contained at its ordered checkpoint; after `PT4-009`, full rollback
  reverts `PT4-009` first and then this single commit.

### PT4-009: convert the remaining request-pacer consumers

- Candidate: PONY-003.
- Behavior owner: `internal/pacing/pacing.go` from `PT4-008`.
- Production files: `internal/scraper/jumpit/client.go`,
  `internal/scraper/rallit/client.go`, and `internal/scraper/worknet/client.go`.
- Behavior lock: Jumpit concurrency timing plus Rallit and Worknet client tests.
- Estimated delta: about minus 50 production lines; zero dependencies.
- Rollback boundary: one three-client commit restores their local pacers.
- Reversibility: reverting it does not change the owner or the first four consumers.

### PT4-010: remove the unused scheduler handle

- Candidate: PONY-005.
- Behavior owner: `internal/server` scheduler startup.
- Production files: `internal/server/scheduler.go` and `cmd/jobcron/main.go`.
- Behavior lock: `TestStartSchedulerRunsScheduledScrapeAfterSleep` and
  `TestStartSchedulerRecordsSkippedRunWhenScrapeLockBusy`.
- Estimated delta: minus 10 production lines; zero dependencies.
- Rollback boundary: one scheduler-API commit restores the return type and ignored handle.
- Reversibility: no other approved batch changes scheduler startup or lifecycle.

All batches cover one domain, touch at most five production files, target negative production
lines, and have no direct-dependency change. The two split clusters have explicit ordered
rollback. Estimated approved total: minus 323 production lines across ten reversible batches.

## Task 4 Final Comparison

- approved findings: 8;
- rejected findings: 5;
- separate-decision findings: 0;
- approved batches: 10;
- estimated production-line delta: minus 323;
- direct-dependency delta: 0; and
- source or user-visible behavior changes in Task 4: none.
