# Daangn Canonical Role URLs Implementation Plan

> **Completed 2026-07-17.** Source commit
> `0fd9295ef19fa8b5391a216be41cad809ec7cb81` has parent
> `bb29695851169ce8b866476fed2336c1214683fb`, a documentation-only descendant of the planned
> base. Review `jobs-o3g` found no issues after live-browser, integration, race, coverage,
> Gitleaks, and clean-tree gates. Mayor integrated the exact source commit.

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`)
> syntax for tracking.

**Goal:** Emit Daangn's canonical careers URLs and restore the Greenhouse live URL gate.

**Architecture:** Keep Daangn on its existing `LinkSite` strategy, which has no other consumer.
Change that strategy's path and Daangn's site host together; preserve listing, filtering, robots,
and storage behavior.

**Tech Stack:** Go 1.26, Greenhouse scraper, Go integration tests, gstack `/browse`.

## Global Constraints

- Start from exact Mayor commit `95a7982078f3d82cf6010ead02dacaedc597813f`.
- Change no source registration, classification, robots policy, storage schema, or source ID.
- Add no dependency, configuration, link strategy, redirect exception, or migration.
- Keep the repair separate from Ponytail batch `PT4-006`.
- Never push, merge, clean up the polecat, close the bead, or run `gt done`.

---

### Task 1: Lock the canonical URL

**Files:**

- Modify: `internal/scraper/greenhouse/greenhouse_test.go`
- Test: `internal/scraper/greenhouse/greenhouse_test.go`

**Interfaces:**

- Consumes: `Tenant.link(id, absoluteURL string) string`.
- Produces: a failing regression test for the canonical Daangn URL.

- [ ] **Step 1: Change the fixture expectation**

Expect each Daangn posting URL to equal:

```go
want := "https://careers.daangn.com/jobs/role/" + id + "/"
```

- [ ] **Step 2: Prove the test is red**

```bash
go test ./internal/scraper/greenhouse -run '^TestParseDaangnMetadataKeepsFourShinip$' -count=1
```

Expected: FAIL because production still emits `team.daangn.com/jobs/{id}/`.

### Task 2: Emit and document the canonical URL

**Files:**

- Modify: `internal/scraper/greenhouse/tenant.go`
- Modify: `internal/scraper/greenhouse/integration_test.go`
- Modify: `internal/scraper/greenhouse/API_NOTES.md`
- Modify: `README.md`
- Modify: `README.ko.md`
- Modify: `docs/scraping/source-catalog.md`

**Interfaces:**

- Produces: `https://careers.daangn.com/jobs/role/{id}/` for `LinkSite`.

- [ ] **Step 1: Make the minimal source change**

Change `LinkSite` to append `/jobs/role/{id}/` and set Daangn's `SiteURL` to
`https://careers.daangn.com`. Update only comments and durable docs that name the legacy host or
path.

- [ ] **Step 2: Run focused and live tests**

```bash
gofmt -w internal/scraper/greenhouse/tenant.go \
  internal/scraper/greenhouse/greenhouse_test.go \
  internal/scraper/greenhouse/integration_test.go
go test ./internal/scraper/greenhouse -count=1
go test -tags integration ./internal/scraper/greenhouse -count=1
```

Expected: unit and live tests pass without a Daangn cross-host redirect.

- [ ] **Step 3: Verify representative destinations in a real browser**

Use gstack `/browse` to click or open three emitted Daangn URLs. Confirm each final URL retains
its source posting ID and the rendered page contains the matching role title. Raw HTTP checks do
not satisfy this step.

### Task 3: Run regression, security, and commit gates

**Files:**

- Modify: none

**Interfaces:**

- Produces: one independently reversible prerequisite commit.

- [ ] **Step 1: Run full gates**

```bash
test -z "$(gofmt -l .)"
go vet ./...
go build ./...
go test ./... -count=1
go test -race ./... -count=1
go test ./... -coverprofile=/tmp/jobcron-daangn-url.cover -count=1
go tool cover -func=/tmp/jobcron-daangn-url.cover
gitleaks git --redact --no-banner
git diff --check
```

Expected: every gate passes; expected general-suite integration skips remain unchanged.

- [ ] **Step 2: Review and commit the exact repair**

Inspect the cumulative diff for credentials, private data, unrelated changes, and publication
risk. Then commit only the files named above:

```bash
git add internal/scraper/greenhouse README.md README.ko.md docs/scraping/source-catalog.md
git diff --cached --check
git commit -m 'fix(scraper): use canonical Daangn role URLs'
```

Expected: one local commit. Rollback is `git revert <commit>`.
