# Server Source Reduction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Delete the unreachable `Server.registeredSources` method while preserving the live
template binding to `Server.allRegisteredSources` and registration order.

**Architecture:** `Server.sources` remains the internal scraper list. Templates continue to
resolve `registeredSources` through the explicit function-map entry bound to
`allRegisteredSources`; a characterization test locks that name distinction before deletion.

**Tech Stack:** Go 1.26, `html/template`, embedded web templates, `httptest`, gstack `/browse`.

## Global Constraints

- Batch: `PT4-002`; candidate: `PONY-008`.
- Start from the human-reviewed Task 5 tip supplied by Mayor, with clean porcelain.
- The reviewed base must contain this plan and the ledger entry marked `planned`.
- Production scope is only `internal/server/sources.go`.
- Test scope is only `internal/server/server_test.go`.
- Preserve source order, labels, kinds, enabled state, profile fields, and template names.
- Do not change `Server.sources`, `allRegisteredSources`, `sourceOptions`, or `sourcePillGroups`.
- Target at least four fewer production lines and zero dependency change.
- Never push, perform neighboring cleanup, or combine this batch with another reduction.

---

### Task 1: Lock the live template binding before deletion

**Files:**

- Modify: `internal/server/server_test.go`
- Test: `internal/server/server_test.go`
- Evidence: `.superpowers/sdd/260717-ponytail/PT4-002-before.md`

**Interfaces:**

- Consumes: template function name `registeredSources` bound to
  `func() []sourceOption` through `srv.allRegisteredSources`.
- Produces: `TestProfileFormRegisteredSourcesPreservesRegistrationOrder`.

- [ ] **Step 1: Verify base identity and symbol reachability**

```sh
git status --short --branch
git log -1 --format='%H %cI %s'
rg -n 'registeredSources|allRegisteredSources' internal/server web
deadcode_output=$(
  GOTOOLCHAIN=go1.26.3 go run golang.org/x/tools/cmd/deadcode@v0.47.0 -test ./...
) &&
  printf '%s\n' "$deadcode_output" | rg 'internal/server/sources.go.*registeredSources'
```

Expected: the private method is one deadcode finding; `server.go` binds the same template name
to `allRegisteredSources`; templates consume the function-map name.

- [ ] **Step 2: Add the characterization test**

Add this test beside `TestProfileFormDefaultsDemodayOffOnlyBeforeFirstSave`:

```go
func TestProfileFormRegisteredSourcesPreservesRegistrationOrder(t *testing.T) {
	st, err := storage.OpenSQLiteAt(filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	srv := New(st,
		&fakeScraper{source: "jumpit"},
		&fakeScraper{source: "demoday"},
	)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/profile", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /profile = %d", rec.Code)
	}
	body := rec.Body.String()
	jumpit := strings.Index(body, `name="source_jumpit"`)
	demoday := strings.Index(body, `name="source_demoday"`)
	if jumpit < 0 || demoday < 0 || jumpit >= demoday {
		t.Fatalf("registered source order: jumpit=%d demoday=%d", jumpit, demoday)
	}
}
```

- [ ] **Step 3: Run the characterization on the old implementation**

```sh
gofmt -w internal/server/server_test.go
go test ./internal/server \
  -run '^TestProfileFormRegisteredSourcesPreservesRegistrationOrder$' -count=1
```

Expected: PASS before production deletion, proving the test observes the live template binding.

### Task 2: Remove the dead method only

**Files:**

- Modify: `internal/server/sources.go:91-94`
- Test: `internal/server/server_test.go`

**Interfaces:**

- Consumes: the behavior lock from Task 1.
- Produces: no replacement method or function-map change.

- [ ] **Step 1: Delete the exact unreachable declaration**

Delete this block:

```go
// registeredSources exposes the source identifiers in registration order —
// used by tests and any caller that needs to enumerate without going
// through the full Scraper interface.
func (s *Server) registeredSources() []scraper.Scraper { return s.sources }
```

Do not remove the `scraper` import; the file still uses `scraper.SourceKind` and
`scraper.Scraper`.

- [ ] **Step 2: Run targeted red/green checks**

```sh
test -z "$(gofmt -l internal/server/sources.go internal/server/server_test.go)"
go test ./internal/server \
  -run '^TestProfileFormRegisteredSourcesPreservesRegistrationOrder$' -count=1
go test ./internal/server \
  -run '^TestProfileFormDefaultsDemodayOffOnlyBeforeFirstSave$' -count=1
! rg -n 'func \(s \*Server\) registeredSources' internal/server/sources.go
deadcode_output=$(
  GOTOOLCHAIN=go1.26.3 go run golang.org/x/tools/cmd/deadcode@v0.47.0 -test ./...
) &&
  ! printf '%s\n' "$deadcode_output" | rg 'internal/server/sources.go.*registeredSources'
```

Expected: both tests pass and the dead method disappears from source and deadcode output.

### Task 3: Run full and browser verification

**Files:**

- Modify: none
- Evidence: `.superpowers/sdd/260717-ponytail/PT4-002-after.md`

**Interfaces:**

- Consumes: one production deletion and one characterization test.
- Produces: reviewable behavior and measurement evidence.

- [ ] **Step 1: Run static, build, unit, race, and coverage gates**

```sh
test -z "$(gofmt -l .)"
go vet ./...
go build ./...
go test ./... -count=1
go test -race ./... -count=1
go test ./... -coverprofile=/tmp/jobcron-ponytail-PT4-002.cover -count=1
go tool cover -func=/tmp/jobcron-ponytail-PT4-002.cover
```

Expected: every command passes. PostgreSQL, scraper integration, AI, and deployment gates are
not proportional because no data, network, or AI policy changes.

- [ ] **Step 2: Walk the affected user path in a real headless browser**

```sh
scripts/preview-interactive.sh 7777
```

Use gstack `/browse` against `http://127.0.0.1:7777/profile`. Verify the Jumpit and Demoday
source controls both render in registration order on desktop and mobile, and record zero console
errors. The preview script creates and cleans up a disposable PostgreSQL database. Do not use
`curl`, the user's browser, production data, or a paid provider.

- [ ] **Step 3: Compare source, test, dependency, binary, and coverage metrics**

Before and after the edit, record:

```sh
git ls-files '*.go' ':(exclude)**/*_test.go' | xargs wc -l
git ls-files '**/*_test.go' | xargs wc -l
go list -m -f '{{if not .Indirect}}{{.Path}} {{.Version}}{{end}}' all
ponytail_metrics_dir=$(mktemp -d)
go build -o "$ponytail_metrics_dir/jobcron" ./cmd/jobcron
go build -o "$ponytail_metrics_dir/jobcron-import" ./cmd/jobcron-import
go build -o "$ponytail_metrics_dir/jobcron-user" ./cmd/jobcron-user
wc -c "$ponytail_metrics_dir"/*
go test ./internal/server -coverprofile="$ponytail_metrics_dir/server.cover" -count=1
go tool cover -func="$ponytail_metrics_dir/server.cover"
```

Expected: production lines decrease by at least four; test lines increase only by the behavior
lock; dependencies, rendered behavior, binary behavior, and coverage do not regress.

### Task 4: Review and commit the implementation batch

**Files:**

- Modify: `internal/server/sources.go`
- Test: `internal/server/server_test.go`

**Interfaces:**

- Consumes: all passing gates.
- Produces: one independently reversible server-source commit.

- [ ] **Step 1: Review the exact diff**

```sh
git diff -- internal/server/sources.go internal/server/server_test.go
git diff --check
git status --short
```

Expected: one dead method deletion and one focused characterization test. Confirm no template,
route, source policy, authentication, CSRF, or accessibility behavior changed.

- [ ] **Step 2: Commit the batch**

```sh
git add internal/server/sources.go internal/server/server_test.go
git diff --cached --check
git commit -m "refactor(server): remove unused registered sources method"
```

Expected: one commit. Rollback is `git revert <PT4-002-commit>`; no other approved batch uses
the deleted method.
