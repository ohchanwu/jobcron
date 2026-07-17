# AI Key Path Reduction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Delete the unreachable exported `ai.DefaultKeysPath` wrapper without changing legacy
key import, canonical path construction, or key-file safety behavior.

**Architecture:** `internal/ai.defaultKeysPath` remains the tested path-construction owner.
Runtime and import callers continue to pass explicit paths to `LoadKeys`; no replacement API is
introduced.

**Tech Stack:** Go 1.26, `internal/ai`, pinned `deadcode` v0.47.0, existing Go tests.

## Global Constraints

- Batch: `PT4-001`; candidate: `PONY-007`.
- Start from the human-reviewed Task 5 tip supplied by Mayor, with clean porcelain.
- The reviewed base must contain this plan and the ledger entry marked `planned`.
- Change only `internal/ai/keys.go`; tests are read-only because coverage already exists.
- Do not alter key-file permissions, parsing, normalization, encryption, or importer behavior.
- Do not add an interface, replacement wrapper, dependency, configuration, or migration.
- Target at least six fewer production lines and zero dependency change.
- Never push, perform neighboring cleanup, or combine this batch with another reduction.

---

### Task 1: Reconfirm reachability and record before metrics

**Files:**

- Modify: none
- Evidence: `.superpowers/sdd/260717-ponytail/PT4-001-before.md`

**Interfaces:**

- Consumes: `func DefaultKeysPath() (string, error)` and
  `func defaultKeysPath(func() (string, error)) (string, error)`.
- Produces: an immutable before snapshot; no source change.

- [ ] **Step 1: Verify the reviewed base and allowed scope**

```sh
git status --short --branch
git log -1 --format='%H %cI %s'
rg -n 'DefaultKeysPath|defaultKeysPath' . \
  -g '!docs/superpowers/archive/**' -g '!.superpowers/**'
```

Expected: clean status; the exported symbol appears only in `keys.go` and the Task 3/4 docs;
the private seam appears in `keys.go` and `keys_test.go`.

- [ ] **Step 2: Run the deletion signal before editing**

```sh
go run golang.org/x/tools/cmd/deadcode@v0.47.0 -test ./... | \
  rg 'internal/ai/keys.go.*DefaultKeysPath'
```

Expected: one `DefaultKeysPath` finding. This is the red reachability signal.

- [ ] **Step 3: Record source, dependency, binary, and coverage metrics**

```sh
mkdir -p .superpowers/sdd/260717-ponytail
git ls-files '*.go' ':(exclude)**/*_test.go' | xargs wc -l
git ls-files '**/*_test.go' | xargs wc -l
go list -m -f '{{if not .Indirect}}{{.Path}} {{.Version}}{{end}}' all
ponytail_metrics_dir=$(mktemp -d)
go build -o "$ponytail_metrics_dir/jobcron" ./cmd/jobcron
go build -o "$ponytail_metrics_dir/jobcron-import" ./cmd/jobcron-import
go build -o "$ponytail_metrics_dir/jobcron-user" ./cmd/jobcron-user
wc -c "$ponytail_metrics_dir"/*
go test ./internal/ai -coverprofile="$ponytail_metrics_dir/ai-before.cover" -count=1
go tool cover -func="$ponytail_metrics_dir/ai-before.cover"
```

Expected: all commands pass. Record output in the ignored before file, including the temporary
directory path so the after comparison uses the same binaries and coverage scope.

### Task 2: Delete only the unreachable wrapper

**Files:**

- Modify: `internal/ai/keys.go:19-24`
- Test: `internal/ai/keys_test.go:108-120` (unchanged behavior lock)

**Interfaces:**

- Consumes: the private `defaultKeysPath` seam.
- Produces: no replacement API; `LoadKeys(path string)` remains unchanged.

- [ ] **Step 1: Run the existing behavior lock before editing**

```sh
go test ./internal/ai -run '^TestDefaultKeysPathUsesCanonicalApplicationDirectory$' -count=1
```

Expected: PASS on the reviewed base.

- [ ] **Step 2: Remove the exact dead declaration**

Delete this block and nothing else:

```go
// DefaultKeysPath is the legacy BYOK key-store path under the user's OS config
// directory. Normal runtime credentials now live encrypted in PostgreSQL; this
// path remains only for explicit legacy import and compatibility code.
func DefaultKeysPath() (string, error) {
	return defaultKeysPath(os.UserConfigDir)
}
```

Keep the `os` import because `LoadKeys`, `SaveKeys`, and file-mode code still use it.

- [ ] **Step 3: Run the targeted green checks**

```sh
test -z "$(gofmt -l internal/ai/keys.go)"
go test ./internal/ai -run '^TestDefaultKeysPathUsesCanonicalApplicationDirectory$' -count=1
! rg -n 'func DefaultKeysPath' internal/ai/keys.go
! go run golang.org/x/tools/cmd/deadcode@v0.47.0 -test ./... | \
  rg 'internal/ai/keys.go.*DefaultKeysPath'
```

Expected: formatting and the test pass; both absence checks succeed.

### Task 3: Run proportional and full verification

**Files:**

- Modify: none
- Evidence: `.superpowers/sdd/260717-ponytail/PT4-001-after.md`

**Interfaces:**

- Consumes: the one-file deletion.
- Produces: verification and before/after measurements for review.

- [ ] **Step 1: Run static, build, unit, race, and coverage gates**

```sh
test -z "$(gofmt -l .)"
go vet ./...
go build ./...
go test ./... -count=1
go test -race ./... -count=1
go test ./... -coverprofile=/tmp/jobcron-ponytail-PT4-001.cover -count=1
go tool cover -func=/tmp/jobcron-ponytail-PT4-001.cover
```

Expected: every command passes. PostgreSQL, scraper integration, browser, and deployment gates
are not proportional because the deleted function has no runtime consumer.

- [ ] **Step 2: Cross-build the shipped package graph**

```sh
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./...
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./...
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...
```

Expected: all four builds pass; deleting an OS-path wrapper does not change portability.

- [ ] **Step 3: Repeat measurements and prove the negative delta**

Repeat Task 1 Step 3 with an `ai-after.cover` file. Expected: production Go lines decrease by
at least six; test lines, direct dependencies, binary behavior, and package coverage do not
regress. Record any binary-size noise without claiming it as source reduction.

### Task 4: Review and commit the implementation batch

**Files:**

- Modify: `internal/ai/keys.go`

**Interfaces:**

- Consumes: all passing evidence above.
- Produces: one independently reversible implementation commit.

- [ ] **Step 1: Run Ponytail and normal review**

```sh
git diff -- internal/ai/keys.go
git diff --check
git status --short
```

Expected: one deleted wrapper, no unrelated edits, no secret or personal data, and no safety
behavior reduction. Confirm Ponytail reports deletion rather than replacement abstraction.

- [ ] **Step 2: Commit exactly the batch boundary**

```sh
git add internal/ai/keys.go
git diff --cached --check
git commit -m "refactor(ai): remove unused default key path"
```

Expected: one commit. Rollback is `git revert <PT4-001-commit>`; no later batch depends on the
deleted symbol.
