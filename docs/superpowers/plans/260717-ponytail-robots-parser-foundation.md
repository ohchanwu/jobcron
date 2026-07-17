# Robots Parser Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add one scraper-owned robots parser and migrate Demoday, Greenhouse, Greeting, and
Jumpit while keeping every source's fetch, cache, host, path, and failure policy local.

**Architecture:** `internal/scraper.RobotsAllows` owns only the shared RFC 9309 subset:
wildcard-agent selection, allow/disallow collection, and longest-prefix precedence. Four source
packages call it; Rallit remains local until dependent batch `PT4-007`.

**Tech Stack:** Go 1.26, `bufio.Scanner`, RFC 9309 subset, five scraper packages, live tests.

## Global Constraints

- Batch: `PT4-006`; candidate: `PONY-002`.
- Start from the human-reviewed Task 5 tip supplied by Mayor, with clean porcelain.
- The reviewed base must contain this plan and the ledger entry marked `planned`.
- Production scope is exactly the new owner plus four approved source files.
- Test scope is the new owner test and converted source tests.
- Do not change robots fetches, status handling, cache TTLs, cache keys, hosts, paths, or errors.
- Do not convert Rallit; `PT4-007` owns that final consumer.
- Keep the foundation net-negative while adding the owner; target at least 100 fewer lines.
- Add no dependency, interface, generic, callback, configuration, or catch-all HTTP helper.
- Never push, perform neighboring cleanup, or combine this batch with another reduction.

---

### Task 1: Characterize the shared parser before source edits

**Files:**

- Create: `internal/scraper/robots_test.go`
- Test: `internal/scraper/jumpit/client_test.go`
- Evidence: `.superpowers/sdd/260717-ponytail/PT4-006-before.md`

**Interfaces:**

- Consumes: five current `func robotsAllows([]byte, string) bool` copies.
- Produces: owner-level behavior cases for `RobotsAllows`.

- [ ] **Step 1: Verify all copies and source-owned policies**

```sh
git status --short --branch
git log -1 --format='%H %cI %s'
rg -n 'func robotsAllows|func longestPrefix|robotsAllows\(' \
  internal/scraper/demoday internal/scraper/greenhouse \
  internal/scraper/greeting internal/scraper/jumpit internal/scraper/rallit
```

Expected: five parser/longest-prefix pairs; four approved foundation consumers and one deferred
Rallit consumer. No template, JavaScript, SQL, migration, reflection, or configuration caller.

- [ ] **Step 2: Copy the Jumpit parser table into owner-level tests**

Create a table-driven `TestRobotsAllows` in package `scraper` covering:

```go
tests := []struct {
	name, robots, path string
	allowed            bool
}{
	{"empty", "", "/api/positions", true},
	{"other agent", "User-agent: bot\nDisallow: /", "/api/positions", true},
	{"wildcard disallow", "User-agent: *\nDisallow: /api", "/api/positions", false},
	{"longer allow", "User-agent: *\nDisallow: /\nAllow: /api", "/api/x", true},
	{"empty disallow", "User-agent: *\nDisallow:", "/api/x", true},
}
```

Call the planned `RobotsAllows([]byte(tc.robots), tc.path)` signature.

- [ ] **Step 3: Run the new owner test red**

```sh
go test ./internal/scraper -run '^TestRobotsAllows$' -count=1
```

Expected: FAIL because `scraper.RobotsAllows` does not exist.

- [ ] **Step 4: Record before measurements and source tests**

```sh
go test ./internal/scraper/jumpit -run '^TestRobotsAllows$' -count=1
go test ./internal/scraper/demoday ./internal/scraper/greenhouse \
  ./internal/scraper/greeting ./internal/scraper/jumpit -count=1
git ls-files '*.go' ':(exclude)**/*_test.go' | xargs wc -l
git ls-files '**/*_test.go' | xargs wc -l
go list -m -f '{{if not .Indirect}}{{.Path}} {{.Version}}{{end}}' all
```

Expected: current source tests pass. Record output in the ignored before file.

### Task 2: Add the narrow parser owner

**Files:**

- Create: `internal/scraper/robots.go`
- Create: `internal/scraper/robots_test.go`

**Interfaces:**

- Produces: `func RobotsAllows(content []byte, path string) bool`.
- Private helper: `func longestPrefix(rules []string, path string) int`.

- [ ] **Step 1: Move the exact stable algorithm into the owner**

Implement the current algorithm once in package `scraper`. Preserve:

```go
func RobotsAllows(content []byte, path string) bool
```

It must scan case-insensitive `User-agent`, `Allow`, and `Disallow` fields; apply rules only
inside the wildcard-agent group; ignore empty values; and allow when the longest allow prefix
is at least as long as the longest disallow prefix. Do not parse fetch or cache policy.

- [ ] **Step 2: Run owner tests green**

```sh
gofmt -w internal/scraper/robots.go internal/scraper/robots_test.go
go test ./internal/scraper -run '^TestRobotsAllows$' -count=1
```

Expected: PASS with the same table that passed against Jumpit's local copy.

### Task 3: Convert exactly four source packages

**Files:**

- Modify: `internal/scraper/demoday/demoday.go`
- Modify: `internal/scraper/greenhouse/greenhouse.go`
- Modify: `internal/scraper/greeting/greeting.go`
- Modify: `internal/scraper/jumpit/client.go`
- Test: `internal/scraper/jumpit/client_test.go`

**Interfaces:**

- Consumes: `scraper.RobotsAllows([]byte, string) bool`.
- Produces: four converted access checks; Rallit stays local.

- [ ] **Step 1: Replace each approved call site**

Replace only:

```go
allowed = robotsAllows(body, path)
```

or the equivalent `robotsCheckPath` call with:

```go
allowed = scraper.RobotsAllows(body, path)
```

Use the existing path variable at each site. Do not move the surrounding fetch or error branch.

- [ ] **Step 2: Delete four local parser/helper pairs**

Delete `robotsAllows` and `longestPrefix` or `longestPrefixMatch` only from Demoday,
Greenhouse, Greeting, and Jumpit. Remove now-unused imports individually after `gofmt`/compile.
Delete Jumpit's old parser table because the identical owner table now locks that primitive.

- [ ] **Step 3: Run targeted green checks and scope assertions**

```sh
gofmt -w internal/scraper/robots.go internal/scraper/robots_test.go \
  internal/scraper/demoday/demoday.go internal/scraper/greenhouse/greenhouse.go \
  internal/scraper/greeting/greeting.go internal/scraper/jumpit/client.go \
  internal/scraper/jumpit/client_test.go
go test ./internal/scraper ./internal/scraper/demoday \
  ./internal/scraper/greenhouse ./internal/scraper/greeting \
  ./internal/scraper/jumpit -count=1
test "$(rg -l 'func robotsAllows' internal/scraper | sort)" = \
  "internal/scraper/rallit/client.go"
```

Expected: all converted packages pass; Rallit is the only remaining local definition.

### Task 4: Run full, live scraper, and measurement gates

**Files:**

- Modify: none
- Evidence: `.superpowers/sdd/260717-ponytail/PT4-006-after.md`

**Interfaces:**

- Consumes: owner plus four conversions.
- Produces: foundation verification and dependency evidence for `PT4-007`.

- [ ] **Step 1: Run static, build, unit, race, and coverage gates**

```sh
test -z "$(gofmt -l .)"
go vet ./...
go build ./...
go test ./... -count=1
go test -race ./... -count=1
go test ./... -coverprofile=/tmp/jobcron-ponytail-PT4-006.cover -count=1
go tool cover -func=/tmp/jobcron-ponytail-PT4-006.cover
```

Expected: every command passes.

- [ ] **Step 2: Run only converted live scraper contracts**

```sh
go test -tags integration ./internal/scraper/demoday/ -count=1
go test -tags integration ./internal/scraper/greenhouse/ -count=1
go test -tags integration ./internal/scraper/greeting/ -count=1
go test -tags integration ./internal/scraper/jumpit/ -count=1
```

Expected: each reachable live contract passes. An upstream outage blocks this batch rather than
being waived. PostgreSQL, AI, browser, and deployment gates are not proportional.

- [ ] **Step 3: Compare complete measurements**

```sh
ponytail_metrics_dir=$(mktemp -d)
go build -o "$ponytail_metrics_dir/jobcron" ./cmd/jobcron
go build -o "$ponytail_metrics_dir/jobcron-import" ./cmd/jobcron-import
go build -o "$ponytail_metrics_dir/jobcron-user" ./cmd/jobcron-user
wc -c "$ponytail_metrics_dir"/*
git ls-files '*.go' ':(exclude)**/*_test.go' | xargs wc -l
git ls-files '**/*_test.go' | xargs wc -l
go list -m -f '{{if not .Indirect}}{{.Path}} {{.Version}}{{end}}' all
```

Expected: this foundation is net-negative by at least 100 production lines despite adding the
owner. Dependencies stay fixed; test movement, binary noise, and coverage are recorded.

### Task 5: Review and commit the foundation

**Files:**

- Create: `internal/scraper/robots.go`
- Create: `internal/scraper/robots_test.go`
- Modify: the four approved source files and Jumpit parser test

**Interfaces:**

- Consumes: all passing gates.
- Produces: the exact reviewed implementation base required by `PT4-007`.

- [ ] **Step 1: Run Ponytail and network-policy review**

```sh
git diff -- internal/scraper
git diff --check
git status --short
```

Expected: only parsing moves. Confirm hosts, paths, headers, caches, status handling, failure
posture, request pacing, and error messages remain local and unchanged.

- [ ] **Step 2: Commit exactly the foundation**

```sh
git add internal/scraper/robots.go internal/scraper/robots_test.go \
  internal/scraper/demoday/demoday.go internal/scraper/greenhouse/greenhouse.go \
  internal/scraper/greeting/greeting.go internal/scraper/jumpit/client.go \
  internal/scraper/jumpit/client_test.go
git diff --cached --check
git commit -m "refactor(scraper): add shared robots parser"
```

Expected: one commit. `PT4-007` must name this reviewed commit as its base. Full rollback after
both batches reverts `PT4-007` first, then this foundation commit.
