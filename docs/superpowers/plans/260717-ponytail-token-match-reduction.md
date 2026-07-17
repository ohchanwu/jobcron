# Token Match Reduction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give scoring and AI citation gates one exact-token primitive without merging their
different policies or creating an import cycle.

**Architecture:** New package `internal/tokenmatch` owns only Unicode tokenization and contiguous
phrase matching. Scoring keeps `tokenize` and `textContains` as one-line policy-facing delegates;
AI keeps `gateTokenize` and `tokenSubsequence` as one-line gate-facing delegates.

**Tech Stack:** Go 1.26, Unicode NFC via `golang.org/x/text`, `slices`, scoring and AI tests.

## Global Constraints

- Batch: `PT4-005`; candidate: `PONY-001`.
- Start from the human-reviewed Task 5 tip supplied by Mayor, with clean porcelain.
- The reviewed base must contain this plan and the ledger entry marked `planned`.
- Production scope is exactly three files: the new owner, scoring match, and AI score delta.
- Test scope is the new owner test plus existing scoring and AI tests.
- Preserve scoring awards/dealbreakers and AI citation-gate policy in their current packages.
- Preserve NFC, maximal letter/digit runs, lowercase, contiguity, order, and empty-phrase false.
- Never make `internal/ai` import `internal/scoring` or the reverse.
- Target at least 25 net fewer production lines and zero dependency change.
- Never push, perform neighboring cleanup, or combine this batch with another reduction.

---

### Task 1: Record paired behavior and before metrics

**Files:**

- Modify: none
- Test: `internal/scoring/match_test.go`
- Test: `internal/ai/score_delta_test.go`
- Evidence: `.superpowers/sdd/260717-ponytail/PT4-005-before.md`

**Interfaces:**

- Consumes: scoring `tokenize`/`textContains` and AI `gateTokenize`/`tokenSubsequence`.
- Produces: a shared invariant baseline; no source change.

- [ ] **Step 1: Verify every current consumer and import direction**

```sh
git status --short --branch
git log -1 --format='%H %cI %s'
rg -n 'tokenize\(|textContains\(|gateTokenize\(|tokenSubsequence\(' \
  internal/scoring internal/ai
go list -deps ./internal/scoring ./internal/ai | \
  rg 'internal/(scoring|ai)$'
```

Expected: scoring imports AI today; AI does not import scoring. All policy callers remain behind
the four local unexported names.

- [ ] **Step 2: Run existing paired behavior locks**

```sh
go test ./internal/scoring -run '^(TestTokenize|TestTextContains)$' -count=1
go test ./internal/ai -run \
  '^(TestGateTokenizeInvariants|TestGateDeltaPresence|TestGateDeltaAbsence)$' -count=1
go test ./internal/scoring -run \
  '^(TestScoreDealbreakerKeywordIsTokenExact|TestAreDuplicates)$' -count=1
```

Expected: all tests pass on the two current copies.

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
go test ./internal/scoring ./internal/ai \
  -coverprofile="$ponytail_metrics_dir/token-before.cover" -count=1
go tool cover -func="$ponytail_metrics_dir/token-before.cover"
```

Expected: all commands pass; record exact results in the ignored before file.

### Task 2: Create the narrow owner test-first

**Files:**

- Create: `internal/tokenmatch/tokenmatch.go`
- Create: `internal/tokenmatch/tokenmatch_test.go`

**Interfaces:**

- Produces: `func Tokenize(text string) []string`.
- Produces: `func Contains(text, phrase string) bool`.

- [ ] **Step 1: Write owner tests from both current invariant suites**

Create table tests covering Korean token exactness, attached particles, punctuation, NFC,
ASCII case folding, digit tokens, contiguous order, newlines, and empty phrases. Use:

```go
func TestTokenize(t *testing.T) {
	got := Tokenize("React, 백엔드 개발자 3")
	want := []string{"react", "백엔드", "개발자", "3"}
	if !slices.Equal(got, want) {
		t.Fatalf("Tokenize = %v, want %v", got, want)
	}
}

func TestContains(t *testing.T) {
	if !Contains("백엔드 신입 개발자", "신입 개발자") {
		t.Fatal("contiguous ordered phrase did not match")
	}
	if Contains("개발자를 찾습니다", "개발") || Contains("text", "   ") {
		t.Fatal("token-exact or empty-phrase contract changed")
	}
}
```

- [ ] **Step 2: Run the new tests red**

```sh
go test ./internal/tokenmatch -count=1
```

Expected: FAIL because `Tokenize` and `Contains` do not exist.

- [ ] **Step 3: Implement only the stable primitive**

Move the current tokenizer algorithm into `Tokenize`. Implement `Contains` by tokenizing both
arguments, returning false for an empty needle, and using `slices.Equal` over each contiguous
window. Do not add policy, configuration, interfaces, or callbacks.

- [ ] **Step 4: Run owner tests green**

```sh
gofmt -w internal/tokenmatch/tokenmatch.go internal/tokenmatch/tokenmatch_test.go
go test ./internal/tokenmatch -count=1
```

Expected: PASS.

### Task 3: Delegate both policy packages to the owner

**Files:**

- Modify: `internal/scoring/match.go`
- Modify: `internal/ai/score_delta.go`
- Test: existing scoring and AI tests

**Interfaces:**

- Consumes: `tokenmatch.Tokenize` and `tokenmatch.Contains`.
- Produces: unchanged local unexported signatures for every existing caller.

- [ ] **Step 1: Replace scoring implementations with delegates**

```go
func tokenize(text string) []string { return tokenmatch.Tokenize(text) }

func textContains(text, phrase string) bool {
	return tokenmatch.Contains(text, phrase)
}
```

Keep `normalizeText` local. Remove scoring's now-unused `slices` and `unicode` imports; keep
`strings` and `norm` for `normalizeText`.

- [ ] **Step 2: Replace AI implementations with delegates**

```go
func gateTokenize(text string) []string { return tokenmatch.Tokenize(text) }

func tokenSubsequence(text, phrase string) bool {
	return tokenmatch.Contains(text, phrase)
}
```

Remove AI's now-unused `slices`, `unicode`, and `norm` imports. Keep every citation gate caller
and policy branch unchanged.

- [ ] **Step 3: Run targeted green checks**

```sh
gofmt -w internal/tokenmatch internal/scoring/match.go internal/ai/score_delta.go
go test ./internal/tokenmatch ./internal/scoring ./internal/ai -count=1
go test ./internal/scoring -run '^TestScoreDealbreakerKeywordIsTokenExact$' -count=1
go test ./internal/ai -run '^TestGateTokenizeInvariants$' -count=1
go list -deps ./internal/ai | rg 'internal/tokenmatch$'
! go list -deps ./internal/ai | rg 'internal/scoring$'
```

Expected: all tests pass; AI imports only downward to `tokenmatch`, never to scoring.

### Task 4: Run full gates and measurements

**Files:**

- Modify: none
- Evidence: `.superpowers/sdd/260717-ponytail/PT4-005-after.md`

**Interfaces:**

- Consumes: the three production files and owner tests.
- Produces: full correctness and reduction evidence.

- [ ] **Step 1: Run static, build, unit, race, and coverage gates**

```sh
test -z "$(gofmt -l .)"
go vet ./...
go build ./...
go test ./... -count=1
go test -race ./... -count=1
go test ./... -coverprofile=/tmp/jobcron-ponytail-PT4-005.cover -count=1
go tool cover -func=/tmp/jobcron-ponytail-PT4-005.cover
```

Expected: every command passes. PostgreSQL, scraper, browser, and deployment gates are not
proportional because storage, HTTP, templates, and configuration do not change.

- [ ] **Step 2: Cross-build and repeat Task 1 measurements**

```sh
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./...
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./...
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...
```

Repeat Task 1 Step 3 with `token-after.cover`. Expected: net production lines decrease by at
least 25; direct dependencies stay fixed; tests, coverage, hashes, and binaries do not regress.

### Task 5: Review and commit the implementation batch

**Files:**

- Create: `internal/tokenmatch/tokenmatch.go`
- Create: `internal/tokenmatch/tokenmatch_test.go`
- Modify: `internal/scoring/match.go`
- Modify: `internal/ai/score_delta.go`

**Interfaces:**

- Consumes: all passing gates.
- Produces: one independently reversible exact-token commit.

- [ ] **Step 1: Run Ponytail and correctness/security review**

```sh
git diff -- internal/tokenmatch internal/scoring/match.go internal/ai/score_delta.go
git diff --check
git status --short
```

Expected: only the stable primitive moves. Confirm scoring policy, AI citation policy, import
direction, Unicode semantics, empty-input behavior, and trust-boundary gates remain unchanged.

- [ ] **Step 2: Commit exactly this batch**

```sh
git add internal/tokenmatch internal/scoring/match.go internal/ai/score_delta.go
git diff --cached --check
git commit -m "refactor(text): share exact token matching"
```

Expected: one commit. Rollback is `git revert <PT4-005-commit>` and restores both local copies.
