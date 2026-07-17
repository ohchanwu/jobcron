# Ponytail Reduction Candidate Ledger

This ledger records evidence only. An entry in Accepted Candidates is accepted for Task 4
triage, not approved for implementation. Task 4 must still apply the campaign acceptance gate.

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

## Accepted Candidates

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

## Rejected Candidates

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

## Needs Separate Product or Architecture Decision

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

## Batch Order

Task 4 has not approved implementation batches. The evidence-first order is:

1. PONY-006, PONY-007, and PONY-008 as independent unreachable-code checks.
2. PONY-005 as a scheduler API simplification.
3. PONY-004 as storage-owned row-loop reuse.
4. PONY-002 and PONY-003 only after cross-consumer characterization.
5. PONY-001 only after the separate ownership decision.

Rejected candidates do not enter a batch.

## Final Comparison

Task 3 records a possible ceiling, not an achieved reduction:

- production lines removed: 0;
- direct dependencies removed: 0;
- accepted-for-triage findings: 7;
- rejected findings: 5;
- separate-decision findings: 1; and
- source or user-visible behavior changes: none.
