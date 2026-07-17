# AI Model Input Reduction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove duplicate `buildModelText` assembly and route its two tagged test callers
through the existing `ModelInput` owner without changing model text or truncation.

**Architecture:** `rawModelText` remains the normalized assembler and `ModelInput` remains the
single public text/hash/truncation entry point. The `aispike` harness ignores the hash it does
not need rather than retaining a second production wrapper.

**Tech Stack:** Go 1.26, `internal/ai`, SHA-256 content hashes, `aispike` build tag.

## Global Constraints

- Batch: `PT4-003`; candidate: `PONY-006`.
- Start from the human-reviewed Task 5 tip supplied by Mayor, with clean porcelain.
- The reviewed base must contain this plan and the ledger entry marked `planned`.
- Production scope is only `internal/ai/extract.go`.
- Test scope is only `internal/ai/spike_test.go`; do not run its paid live benchmark.
- Preserve NFC normalization, rune-boundary truncation, the 12-character pre-truncation hash,
  prompts, parsers, citation gates, and provider behavior.
- Target at least 13 fewer production lines and zero dependency change.
- Never push, perform neighboring cleanup, or combine this batch with another reduction.

---

### Task 1: Lock model-input equivalence and baseline metrics

**Files:**

- Modify: none
- Test: `internal/ai/extract_test.go`
- Evidence: `.superpowers/sdd/260717-ponytail/PT4-003-before.md`

**Interfaces:**

- Consumes: `func buildModelText(scraper.Posting) (string, bool)` and
  `func ModelInput(scraper.Posting) (string, string, bool)`.
- Produces: a before snapshot; no source change.

- [ ] **Step 1: Verify definitions, callers, and build tag**

```sh
git status --short --branch
git log -1 --format='%H %cI %s'
rg -n 'buildModelText|ModelInput|go:build aispike' internal/ai
deadcode_output=$(
  GOTOOLCHAIN=go1.26.3 go run golang.org/x/tools/cmd/deadcode@v0.47.0 -test ./...
) &&
  printf '%s\n' "$deadcode_output" | rg 'internal/ai/extract.go.*buildModelText'
```

Expected: `buildModelText` is found only in production declaration and two `aispike` calls;
deadcode reports it because ordinary all-target tests exclude that tag.

- [ ] **Step 2: Run existing behavior locks on the reviewed base**

```sh
go test ./internal/ai -run '^TestBuildModelTextTruncationAndHashStability$' -count=1
go test -tags aispike ./internal/ai -run '^$' -count=1
```

Expected: the model-input test passes and tagged tests compile without executing the live spike.

- [ ] **Step 3: Record before measurements**

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

Expected: all commands pass; record exact results in the ignored before file.

### Task 2: Replace tagged callers and delete the duplicate wrapper

**Files:**

- Modify: `internal/ai/extract.go:93-107`
- Test: `internal/ai/spike_test.go:196,422`

**Interfaces:**

- Consumes: `ModelInput(p scraper.Posting) (text, contentHash string, truncated bool)`.
- Produces: identical `modelText` values in both spike loops.

- [ ] **Step 1: Replace both tagged call sites**

At both spike locations, replace:

```go
modelText, _ := buildModelText(p)
```

with:

```go
modelText, _, _ := ModelInput(p)
```

The ignored values are the stable content hash and truncation flag; the spike used neither.

- [ ] **Step 2: Delete `buildModelText` exactly**

Remove its comment and function body from `internal/ai/extract.go`. Keep `rawModelText`,
`ModelInput`, `maxModelTextRunes`, SHA-256 hashing, and all imports used by them unchanged.

- [ ] **Step 3: Run targeted green checks**

```sh
gofmt -w internal/ai/extract.go internal/ai/spike_test.go
go test ./internal/ai -run '^TestBuildModelTextTruncationAndHashStability$' -count=1
go test -tags aispike ./internal/ai -run '^$' -count=1
! rg -n 'buildModelText' internal/ai
```

Expected: regular behavior passes, tagged code compiles without network calls, and the duplicate
symbol has no remaining definition or caller.

### Task 3: Run full verification and compare measurements

**Files:**

- Modify: none
- Evidence: `.superpowers/sdd/260717-ponytail/PT4-003-after.md`

**Interfaces:**

- Consumes: the production deletion and two tagged call replacements.
- Produces: full correctness and reduction evidence.

- [ ] **Step 1: Run static, build, unit, race, and coverage gates**

```sh
test -z "$(gofmt -l .)"
go vet ./...
go build ./...
go test ./... -count=1
go test -race ./... -count=1
go test ./... -coverprofile=/tmp/jobcron-ponytail-PT4-003.cover -count=1
go tool cover -func=/tmp/jobcron-ponytail-PT4-003.cover
go test -tags aispike ./internal/ai -run '^$' -count=1
```

Expected: all commands pass. Do not run `TestLocalVsHaikuSpike`; it uses local Ollama and a
paid Anthropic balance. PostgreSQL, scraper, browser, and deployment gates are not proportional.

- [ ] **Step 2: Repeat Task 1 metrics after the edit**

Repeat Task 1 Step 3 with `ai-after.cover`. Expected: production lines decrease by at least 13;
test lines remain stable; dependencies, model text, hash behavior, binaries, and coverage do not
regress.

### Task 4: Review and commit the implementation batch

**Files:**

- Modify: `internal/ai/extract.go`
- Test: `internal/ai/spike_test.go`

**Interfaces:**

- Consumes: all passing evidence.
- Produces: one independently reversible AI model-input commit.

- [ ] **Step 1: Run Ponytail and correctness/security review**

```sh
git diff -- internal/ai/extract.go internal/ai/spike_test.go
git diff --check
git status --short
```

Expected: one wrapper deletion and two three-value calls. Confirm prompts, model text, hashes,
truncation, provider egress, secret handling, and ordinary tests are unchanged.

- [ ] **Step 2: Commit exactly this batch**

```sh
git add internal/ai/extract.go internal/ai/spike_test.go
git diff --cached --check
git commit -m "refactor(ai): reuse model input builder"
```

Expected: one commit. Rollback is `git revert <PT4-003-commit>` and restores the tagged wrapper
without affecting another approved batch.
