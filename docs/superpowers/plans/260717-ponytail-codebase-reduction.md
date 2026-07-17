# Ponytail Codebase Reduction Campaign Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce duplicated and unnecessary production code without removing
features, weakening safety controls, or changing user-visible behavior.

**Architecture:** Ponytail supplies the deletion-first review discipline and a
ranked whole-repository audit. Go-specific clone, reachability, dependency, and
test evidence independently validate its findings. The campaign first produces
an approved candidate ledger, then implements one semantic cluster per child
plan and reversible commit.

**Tech Stack:** Go 1.26, PostgreSQL 18, embedded HTML/CSS/JavaScript, Ponytail
for Codex and Claude Code, `dupl` v1.1.0,
`golang.org/x/tools/cmd/deadcode` v0.47.0, existing Go tests, and gstack
`/browse`.

## Global Constraints

- Start only from a clean, reviewed `main` commit with no active jobscraper
  polecat, merge-queue, or convoy work.
- Ponytail is a candidate generator, not proof that code is safe to delete.
- Use Ponytail `full`. Do not use `ultra` for this campaign.
- Keep Ponytail's global default `off` during the Jobcron pilot; activate `full`
  only in Jobcron campaign threads. Promote `full` to the global default only
  after the human accepts the completed pilot in Task 7.
- Preserve every supported feature and all user-visible behavior.
- Never reduce trust-boundary validation, data-loss prevention, authentication,
  authorization, encryption, CSRF protection, rate limiting, concurrency
  controls, accessibility, migration safety, or deployment verification.
- Optimize production code and direct dependencies. Do not use test or
  documentation deletion to manufacture a smaller total line count.
- Preserve useful characterization and regression tests. Consolidate test setup
  only when failure messages and scenario clarity remain at least as good.
- Prefer reuse or deletion. Do not create a generic `util`, `helpers`, or new
  interface package merely to make duplicate-detector output disappear.
- A shared helper must have one clear owner and express one shared policy.
- Similar-looking code with different domain policy stays separate.
- Keep `scraper.Scraper`, `ai.Provider`, the concrete `*storage.Store`, and the
  documented package boundaries unless a separate architecture decision says
  otherwise.
- Never make `internal/ai` import `internal/scoring`; that would create a Go
  import cycle because `scoring` already imports `ai`.
- Treat their identical tokenizer and exact-token phrase matcher as a candidate
  for a narrow lower-level package. Share only that stable primitive; keep
  scoring policy and AI evidence-gate policy in their owning packages.
  - **OF — Resolved:** The import cycle blocks direct sharing, but not a narrow
    third package. The new potential-easy-win section records that option and
    the later ledger no longer rejects it in advance.
- Keep raw audit output in ignored `.superpowers/sdd/`. Track only reviewed
  conclusions and executable plans under `docs/superpowers/`.
- Each implementation commit covers one semantic cluster and is independently
  reversible.
- Never push. The human reviews local commits and decides when to push.
- Do not expose credentials, production data, connection strings, or machine
  identifiers in tracked reports.

## Starting Snapshot

The drafting worktree is not an execution base because it has unrelated local
changes and outstanding production work. Re-capture every metric at the clean
campaign base.

The provisional 2026-07-17 inventory is:

- 16,015 production Go lines;
- 23,081 Go test lines across 620 test functions;
- 3,547 tracked web HTML/CSS/JavaScript lines;
- 448 tracked shell lines; and
- 7 direct Go module dependencies.

These numbers describe scale, not targets. Large files and duplicate-detector
matches are leads, not evidence of bad design.

## Potential Easy Wins

These are seeded candidates, not preapproved edits. Each must still pass the
same caller analysis, behavior-locking tests, net-reduction check, and human
review as findings discovered by Ponytail, `dupl`, or `deadcode`.

### 1. Share the exact-token matching primitive

`internal/scoring/match.go` and `internal/ai/score_delta.go` currently duplicate
the same low-level operations: NFC normalization, Unicode letter/digit token
splitting, lowercasing, and exact contiguous token-sequence matching.

Evaluate extracting only those operations into a narrowly named lower-level
package such as `internal/textmatch`. Keep these policies outside it:

- `scoring`: dealbreaker matching, intern classification, and title-similarity
  deduplication;
- `ai`: minimum evidence length, minimum evidence token count, sent-text versus
  full-description selection, and fail-safe presence/absence handling.

The child plan must prove identical observable behavior, preserve focused tests
for both consumers, avoid an import cycle, and reduce net production code. It
must also update the documented package-boundary guidance if the candidate is
accepted. Future linguistic matching changes may remain in `scoring` without
weakening the stricter AI evidence gate.

## Success Criteria

1. The clean baseline and final commit pass the same unit, integration, race,
   cross-build, and browser user-flow gates.
2. No supported command, route, scraper, storage backend, profile field, AI
   behavior, authentication behavior, or deployment contract is removed.
3. Every deletion is backed by caller analysis and either existing coverage or
   a behavior-locking characterization test.
4. Production Go/web/shell lines or direct dependency count decrease. Test and
   documentation lines are reported separately.
5. No accepted batch introduces a catch-all helper package, speculative
   abstraction, or dependency.
6. Every accepted, rejected, and separate-decision Ponytail finding has a
   recorded reason.
7. If the evidence finds no safe cuts, `Lean already. Ship.` is a successful
   outcome. The campaign must not force a deletion quota.

---

### Task 1: Freeze a Clean Behavioral Baseline

**Files:**

- Create ignored evidence:
  `.superpowers/sdd/260717-ponytail/baseline.md`
- Create ignored browser evidence under:
  `.superpowers/sdd/260717-ponytail/browser-baseline/`
- Modify tracked files: none

**Interfaces:**

- Consumes: a reviewed `main` commit and a disposable PostgreSQL 18 test
  database exposed through `JOBCRON_TEST_POSTGRES_URL`.
- Produces: the exact base commit, test results, user-flow observations, package
  coverage, production line counts, direct dependency count, and binary sizes
  used by every later comparison.

- [ ] **Step 1: Verify that the campaign may start**

Run from the repository root:

```bash
git status --short --branch
gt rig status jobscraper
gt polecat list jobscraper
gt mq list jobscraper
gt convoy list
```

Expected: the worktree is clean and no jobscraper work is active or awaiting
integration. Stop instead of stashing, deleting, or absorbing unrelated work.

- [ ] **Step 2: Record the immutable baseline identity**

Run:

```bash
git rev-parse HEAD
git log -1 --format='%H %cI %s'
go version
git status --porcelain=v1
```

Expected: one reviewed commit, the intended Go toolchain, and empty porcelain
status. Record the values in the ignored baseline file.

- [ ] **Step 3: Record separate production and test size metrics**

Use tracked files only. Record at least:

```bash
git ls-files '*.go' ':(exclude)**/*_test.go' | xargs wc -l
git ls-files '**/*_test.go' | xargs wc -l
git ls-files 'web/*.html' 'web/*.css' 'web/*.js' | xargs wc -l
go list -m -f '{{if not .Indirect}}{{.Path}} {{.Version}}{{end}}' all
```

Also record the sizes of clean builds for `jobcron`, `jobcron-import`, and
`jobcron-user`. Do not count `dist/`, generated binaries, archived plans, or
untracked files as source reduction.

- [ ] **Step 4: Run the clean static and unit gates**

Run:

```bash
test -z "$(gofmt -l .)"
go vet ./...
go build ./...
go test ./... -count=1
go test -race ./... -count=1
go test ./... -coverprofile=/tmp/jobcron-ponytail-before.cover -count=1
go tool cover -func=/tmp/jobcron-ponytail-before.cover
```

Expected: all commands pass. Coverage is a comparison signal, not deletion
permission.

- [ ] **Step 5: Run PostgreSQL-backed tests without skips**

Export `JOBCRON_TEST_POSTGRES_URL` to a disposable PostgreSQL 18 database, then
run:

```bash
go test ./internal/storage ./cmd/jobcron-import ./cmd/jobcron-user -count=1
go test -race ./internal/storage ./cmd/jobcron-import ./cmd/jobcron-user -count=1
```

Expected: PostgreSQL tests execute and pass. A skipped database test blocks the
baseline.

- [ ] **Step 6: Run live scraper contracts**

Run each source separately so an upstream failure identifies one source:

```bash
go test -tags integration ./internal/scraper/jumpit/ -count=1
go test -tags integration ./internal/scraper/rallit/ -count=1
go test -tags integration ./internal/scraper/demoday/ -count=1
go test -tags integration ./internal/scraper/greeting/ -count=1
go test -tags integration ./internal/scraper/greenhouse/ -count=1
```

Expected: each reachable live contract passes. If an external service is down,
record the exact baseline failure and postpone changes to that scraper until the
same contract can be observed again.

- [ ] **Step 7: Walk the primary web flow in a real headless browser**

Start the application with a disposable PostgreSQL database and `--no-open`.
Use gstack `/browse`, not `curl`, to exercise:

1. owner login;
2. dashboard rendering;
3. profile load and save without entering a replacement AI key;
4. bookmarks and hidden/not-interested state;
5. archive navigation;
6. manual scrape status; and
7. rerate status with the controlled test setup.

Capture desktop and mobile screenshots plus console errors in the ignored
browser-baseline directory. Do not use the production database or paid provider
calls for this baseline.

- [ ] **Step 8: Commit nothing**

Task 1 creates ignored evidence only. Confirm `git status --short` is still
empty before proceeding.

---

### Task 2: Install and Scope Ponytail for Codex and Claude Code

**Files:**

- Modify repository files: none
- External tool state: the Codex and Claude Code Ponytail plugins and lifecycle
  hooks
- External configuration: `~/.config/ponytail/config.json`

- **OF — Resolved:** Install and verify Ponytail in both Codex and Claude Code.

**Interfaces:**

- Consumes: Node.js on the non-interactive `PATH` plus current Codex and Claude
  Code CLIs.
- Produces: Ponytail's mode, audit, and review commands in fresh Codex and Claude
  Code threads without making Ponytail active in unrelated projects during the
  Jobcron pilot.

- [ ] **Step 1: Verify the hook runtime**

Run:

```bash
node --version
codex plugin list
claude plugin list
```

Expected: Node.js is available. Ponytail was verified absent from both hosts
while this plan was revised on 2026-07-17. Check again at execution time so a
resumed or partially completed setup remains idempotent.

- **OF — Resolved:** The current absence is recorded, while the runtime check is
  retained because a resumable plan must tolerate prior partial installation.

- [ ] **Step 2: Install the official plugin when absent**

Run:

```bash
codex plugin marketplace add DietrichGebert/ponytail
codex plugin add ponytail@ponytail
```

In Claude Code, send these as two separate prompts:

```text
/plugin marketplace add DietrichGebert/ponytail
/plugin install ponytail@ponytail
```

Expected: the official marketplace and plugin are registered in both hosts
without changing the Jobcron repository.

- [ ] **Step 3: Keep the global default off during the Jobcron pilot**

Inspect `~/.config/ponytail/config.json`. Preserve any existing fields and set:

```json
{
  "defaultMode": "off"
}
```

Expected: Ponytail remains available but does not silently govern unrelated
repositories while Jobcron establishes the baseline.

- **OF — Resolved:** Jobcron is the explicit pilot. Task 7 changes
  `defaultMode` to `full` only after the human accepts the pilot, making
  Ponytail the default for later work in other repositories.

- [ ] **Step 4: Trust the hooks and start a fresh thread**

Start Codex, open `/hooks`, review Ponytail's two lifecycle hooks, and trust them
only after confirming they come from the installed plugin. Confirm the plugin
is enabled in Claude Code. Start fresh Jobcron threads in the host or hosts that
will execute the campaign so hook activation is deterministic.

- [ ] **Step 5: Select the campaign mode**

Invoke the command for the active host:

```text
# Codex
@ponytail full

# Claude Code
/ponytail full
```

Expected: `full` is active. Do not use `ultra`; the campaign values verified
behavior preservation over the maximum possible deletion count.

---

### Task 3: Build the Independent Reduction Evidence Set

**Files:**

- Create ignored Ponytail output:
  `.superpowers/sdd/260717-ponytail/ponytail-audit.md`
- Create ignored clone output:
  `.superpowers/sdd/260717-ponytail/dupl-production.txt`
- Create ignored test-clone output:
  `.superpowers/sdd/260717-ponytail/dupl-tests.txt`
- Create ignored reachability output under:
  `.superpowers/sdd/260717-ponytail/deadcode/`
- Create tracked ledger:
  `docs/superpowers/specs/260717-ponytail-reduction-ledger.md`
- Modify: `docs/superpowers/README.md`
- Modify: `docs/README.md`

**Interfaces:**

- Consumes: Task 1's baseline and Task 2's Ponytail installation.
- Produces: one deduplicated candidate ledger. It contains findings only and
  authorizes no source changes.

- [ ] **Step 1: Run Ponytail's whole-repository audit**

Invoke `@ponytail-audit` in Codex or `/ponytail-audit` in Claude Code from the
clean repository root. Ask it to scan tracked production Go, web assets,
scripts, and direct dependencies while excluding generated files, `dist/`,
archives, and vendored content.

Record its complete ranked output in the ignored audit file. Preserve its
`delete`, `stdlib`, `native`, `yagni`, and `shrink` tags.

- [ ] **Step 2: Detect production Go clones mechanically**

Run the pinned abstract syntax tree (AST) clone detector on non-test Go files.
An AST represents code as structural nodes such as functions, calls, loops, and
expressions rather than as raw text:

- **OF — Resolved:** AST is expanded and defined above. Structural comparison
  can find clones whose variable names or literal values differ.

```bash
git ls-files 'cmd/**/*.go' 'internal/**/*.go' \
  | rg -v '_test\.go$' \
  | go run github.com/mibk/dupl@v1.1.0 -files -plumbing -t 100
```

Record the output in the ignored production clone report. `dupl` ignores AST
values and can report false positives, so a match is never automatic permission
to share code.

- [ ] **Step 3: Inspect test clones separately**

Run:

```bash
git ls-files '**/*_test.go' \
  | go run github.com/mibk/dupl@v1.1.0 -files -plumbing -t 150
```

Record the output separately. Test duplication receives lower priority than
production duplication and is accepted when it keeps each scenario legible.

- [ ] **Step 4: Find unreachable Go functions across supported targets**

Run `deadcode` with tests for each shipped target:

```bash
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 \
  go run golang.org/x/tools/cmd/deadcode@v0.47.0 -test ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go run golang.org/x/tools/cmd/deadcode@v0.47.0 -test ./...
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
  go run golang.org/x/tools/cmd/deadcode@v0.47.0 -test ./...
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go run golang.org/x/tools/cmd/deadcode@v0.47.0 -test ./...
```

Record each target separately. Only findings shared by all relevant targets are
eligible, and even that intersection still requires reflection, template,
interface, build-tag, and migration review.

- [ ] **Step 5: Check module and standard-library opportunities**

Run:

```bash
go mod tidy -diff
go list -m -f '{{if not .Indirect}}{{.Path}} {{.Version}}{{end}}' all
```

For each direct dependency, record its import sites and the capability it owns.
Do not replace a small, maintained dependency with more hand-written code merely
to lower the dependency count.

- [ ] **Step 6: Create the tracked candidate ledger**

Create the ledger with these exact sections:

```markdown
# Ponytail Reduction Candidate Ledger

## Baseline

## Accepted Candidates

## Rejected Candidates

## Needs Separate Product or Architecture Decision

## Batch Order

## Final Comparison
```

Every candidate entry must record:

- a stable `PONY-NNN` identifier;
- Ponytail/mechanical evidence source;
- exact files and symbols;
- current observable behavior;
- all callers and non-Go consumers;
- owning package;
- behavior-locking tests;
- expected production lines and dependencies removed;
- risk level and rollback boundary; and
- `accepted`, `rejected`, or `separate decision`, with the reason.

Seed `PONY-001` in the separate-decision section with the shared exact-token
matching candidate from Potential Easy Wins. Record the current package
guidance, import graph, all callers, policy-specific behavior, tests, estimated
net production-line reduction, and rollback boundary. Task 4 may move it to the
accepted or rejected section only after applying the full acceptance gate.

- [ ] **Step 7: Index and commit the evidence synthesis**

Add the ledger to both documentation indexes. Inspect the complete staged diff,
run Gitleaks, and manually check for credentials, private paths, and production
details before committing:

```bash
git add docs/superpowers/specs/260717-ponytail-reduction-ledger.md \
  docs/superpowers/README.md docs/README.md
git diff --cached --check
gitleaks git --no-banner
git commit -m "docs(plan): record ponytail reduction candidates"
```

Commit only the ledger and index changes. Keep raw reports ignored.

---

### Task 4: Triage Candidates into Safe Semantic Batches

**Files:**

- Modify:
  `docs/superpowers/specs/260717-ponytail-reduction-ledger.md`
- Modify: `docs/superpowers/README.md` only if active-work links change
- Modify: `docs/README.md` only if active-work links change

**Interfaces:**

- Consumes: the complete Task 3 evidence set.
- Produces: an approved ordered batch list or a documented lean-codebase result.

- [ ] **Step 1: Trace every candidate before judging it**

For each candidate, inspect definitions, Go callers, template references,
JavaScript references, SQL/migration references, reflection, build tags,
configuration, tests, and relevant Git history. A text search with no hits is
not enough when registration or reflection may be involved.

- [ ] **Step 2: Classify duplicate findings by semantics**

Use exactly these classes:

1. same policy and same behavior: consolidate under the existing owning package;
2. same shape but different policy: keep separate and record why;
3. existing helper already owns the behavior: reuse it and delete the duplicate;
4. same stable primitive but different surrounding policies: consider a narrow
   lower-level owner while keeping policy in each consumer;
5. intentional boundary duplication: keep it and preserve behavior-locking
   tests; or
6. test-only setup duplication: consolidate only if individual scenarios remain
   clearer after the change.

- [ ] **Step 3: Apply the acceptance gate**

Accept a candidate only when all of these are true:

- the behavior contract is named;
- all consumers are known;
- coverage exists or a characterization test can lock the behavior;
- the existing owner is obvious, or a narrow new owner is justified by multiple
  current consumers with the same stable contract;
- the result removes production code or a dependency;
- no speculative or catch-all abstraction or import cycle is introduced; and
- the change can be reverted in one commit.

Reject it otherwise. Move feature removals and architectural changes to the
separate-decision section instead of smuggling them into cleanup work.

- [ ] **Step 4: Order the accepted batches**

Use this order:

1. unreachable private code and unused configuration;
2. reuse of an existing helper or standard-library feature;
3. exact same-policy production clones;
4. dependency removal;
5. wrapper/interface simplification with proven behavior; and
6. test-helper cleanup.

Each batch covers one domain, changes no more than five production files, and
has a target of negative production lines. Split any larger cluster.

- [ ] **Step 5: Stop cleanly when no candidates pass**

If nothing passes the gate, record `Lean already. Ship.`, commit the ledger
conclusion, and proceed directly to Task 7. Do not weaken the gate to create
work.

---

### Task 5: Write One Exact Child Plan per Accepted Batch

**Files:**

- Create one dated plan per accepted semantic cluster under:
  `docs/superpowers/plans/`
- Modify:
  `docs/superpowers/specs/260717-ponytail-reduction-ledger.md`
- Modify: `docs/superpowers/README.md`
- Modify: `docs/README.md`

**Interfaces:**

- Consumes: one accepted ledger batch with known symbols and consumers.
- Produces: an implementation plan with exact files, signatures, tests,
  commands, expected outputs, and one rollback commit boundary.

- [ ] **Step 1: Name the plan after the behavior owner**

Derive the filename from the actual package or behavior owner. For example, a
server-owned batch created on 2026-07-18 is
`260718-ponytail-server-reduction.md`. Do not create one repository-wide rewrite
plan.

- [ ] **Step 2: Specify the behavior lock before the refactor**

Each child plan must name the current behavior and the exact existing tests that
lock it. Where coverage is missing, add a characterization test that passes on
the old implementation before changing production code.

- [ ] **Step 3: Specify the smallest production edit**

The child plan must state exactly what disappears, which existing owner replaces
it, and why the result has identical semantics. It must show actual code for new
or changed helper signatures. It may not say “deduplicate similar code” without
the concrete call sites.

- [ ] **Step 4: Specify proportional verification**

Every child plan includes targeted package tests, full unit and race suites, and
the relevant PostgreSQL, scraper, AI, browser, cross-build, or deployment gates
from Task 1.

- [ ] **Step 5: Review and commit each child plan**

Inspect staged documentation, run Gitleaks, update both indexes, and commit the
plan separately from implementation. The human may reject one batch without
blocking the others.

---

### Task 6: Execute and Review Each Reduction Batch

**Files:**

- Modify only the exact source and test files named by the approved child plan.
- Modify the candidate ledger after the batch passes.

**Interfaces:**

- Consumes: one approved Task 5 child plan.
- Produces: one behavior-preserving, net-negative local commit and an updated
  ledger entry containing its measured result.

- [ ] **Step 1: Reconfirm the batch base**

Verify the worktree is clean, no conflicting Gas Town work has started, and the
batch is based on the expected reviewed commit. Stop on drift and update the
child plan before editing.

- [ ] **Step 2: Run the pre-refactor behavior lock**

Run the exact targeted tests from the child plan. A new characterization test
must pass before the refactor because its purpose is to freeze existing
behavior, not introduce new behavior.

- [ ] **Step 3: Make the smallest planned edit**

Delete or consolidate only the approved cluster. Reuse the existing owner; do
not expand the change into neighboring cleanup discovered along the way. Add new
findings to the ledger for later triage.

- [ ] **Step 4: Run the targeted tests and static checks**

Run the child plan's package tests, `gofmt`, `go vet`, and any focused database,
scraper, AI, web, or deployment contract tests.

- [ ] **Step 5: Run Ponytail and correctness reviews on the diff**

Invoke `@ponytail-review` in Codex or `/ponytail-review` in Claude Code to check
whether the diff can shrink further. Then run the normal correctness/security
review because Ponytail explicitly excludes correctness, security, and
performance findings.

Reject a suggested reduction when it crosses any Global Constraint.

- [ ] **Step 6: Run the full regression gates**

At minimum, run:

```bash
test -z "$(gofmt -l .)"
go vet ./...
go build ./...
go test ./... -count=1
go test -race ./... -count=1
```

Also run every proportional Task 1 integration and browser gate named by the
child plan. UI-affecting edits require desktop and mobile browser verification
with zero console errors.

- [ ] **Step 7: Measure and commit one reversible batch**

Record production lines, test lines, direct dependencies, binary size, and test
coverage before and after. Inspect the cumulative diff, then commit only the
approved batch:

```bash
git diff --check
git diff --stat
git diff --numstat
```

Use the exact `git commit` command written in the approved child plan. Never
push.

- [ ] **Step 8: Update the ledger**

Record the commit, actual production lines/dependencies removed, tests run, and
any rejected Ponytail follow-ups. Commit the documentation update separately
after the publication-security gate.

Repeat Task 6 only for the next approved child plan.

---

### Task 7: Prove the Final Codebase Preserves Functionality

**Files:**

- Modify:
  `docs/superpowers/specs/260717-ponytail-reduction-ledger.md`
- Create ignored final browser evidence under:
  `.superpowers/sdd/260717-ponytail/browser-final/`
- Move completed tracked records to a dated `docs/superpowers/archive/`
  workstream after human acceptance.
- Update: `docs/superpowers/README.md`
- Update: `docs/README.md`
- External configuration after human acceptance:
  `~/.config/ponytail/config.json`

**Interfaces:**

- Consumes: every accepted reduction commit and Task 1's immutable baseline.
- Produces: a final behavior comparison, measured reduction, and archive-ready
  campaign record.

- [ ] **Step 1: Rerun the complete clean baseline**

Repeat every Task 1 static, unit, race, PostgreSQL, live scraper, coverage,
cross-build, and browser command on the final commit. Do not substitute HTTP
status checks for browser user flows.

- [ ] **Step 2: Cross-build all shipped commands**

Build `jobcron`, `jobcron-import`, and `jobcron-user` with `CGO_ENABLED=0` for:

- darwin/arm64;
- linux/amd64;
- linux/arm64; and
- windows/amd64.

Run:

```bash
for target in darwin/arm64 linux/amd64 linux/arm64 windows/amd64; do
  os=${target%/*}
  arch=${target#*/}
  out="/tmp/jobcron-cross/${os}-${arch}/"
  mkdir -p "$out"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -o "$out" \
    ./cmd/jobcron ./cmd/jobcron-import ./cmd/jobcron-user
done
```

Expected: every supported target builds successfully.

- [ ] **Step 3: Compare behavior and metrics**

Record baseline versus final:

- production Go/web/shell lines;
- test lines and test count separately;
- direct module dependencies;
- clean binary sizes;
- package coverage;
- unit, race, PostgreSQL, and live integration results; and
- each browser-flow result and console-error count.

No test-count or coverage decrease is silently accepted. Explain any metric
movement in the ledger and require human review when the evidence is ambiguous.

- [ ] **Step 4: Run the final security and publication gate**

Run:

```bash
gitleaks git --no-banner
git diff --check
git status --short --branch
```

Manually inspect tracked documentation for credentials, personal information,
production identifiers, and raw operational output.

- [ ] **Step 5: Present the result before archiving**

Report:

- accepted, rejected, and separate-decision finding counts;
- production lines and dependencies removed;
- behavior gates executed;
- any intentional duplication retained; and
- local commit list.

The human reviews the result before Ponytail becomes the global default and
before the campaign is archived or pushed.

- [ ] **Step 6: Promote the accepted pilot to the global default**

Only after the human accepts the Jobcron result, preserve the other fields in
`~/.config/ponytail/config.json` and set:

```json
{
  "defaultMode": "full"
}
```

Start fresh Codex and Claude Code threads outside Jobcron. Confirm Ponytail
reports `full` without a per-thread activation command and confirm each host can
still switch to `off`. This is a machine-wide agent configuration change, not a
Jobcron repository change.

- [ ] **Step 7: Archive completed campaign records**

After acceptance, move the completed parent plan, child plans, and candidate
ledger into one dated archive workstream. Move sanitized raw evidence to the
ignored `.superpowers/archive/` tree, update both indexes, pass the documentation
security gate, and commit the lifecycle update locally.

## Source Notes

- Ponytail's official README documents the separate Codex and Claude Code
  installation commands plus the shared global default-mode configuration:
  <https://github.com/DietrichGebert/ponytail#install>
- Ponytail's core ladder prioritizes existing code, standard library, native
  platform behavior, and minimal working changes while preserving security,
  validation, data-loss handling, and accessibility:
  <https://github.com/DietrichGebert/ponytail/blob/main/skills/ponytail/SKILL.md>
- `ponytail-audit` is a ranked, report-only whole-repository audit and excludes
  correctness, security, and performance review:
  <https://github.com/DietrichGebert/ponytail/blob/main/skills/ponytail-audit/SKILL.md>
- `ponytail-review` reviews a diff for over-engineering but does not apply fixes:
  <https://github.com/DietrichGebert/ponytail/blob/main/skills/ponytail-review/SKILL.md>
- `dupl` compares serialized Go ASTs, ignores node values, and can produce false
  positives:
  <https://github.com/mibk/dupl>
- Go's `deadcode` uses whole-program reachability, is configuration-specific,
  and still requires judgment before deletion:
  <https://pkg.go.dev/golang.org/x/tools/cmd/deadcode>
