# Full-Viewport Background Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ensure every Jobcron page paints the application background through the full viewport.

**Architecture:** Keep the existing page layout unchanged. Extend the shared global CSS contract so the root canvas shares the application background and `body` cannot be shorter than the viewport.

**Tech Stack:** Embedded CSS, Go `embed.FS` regression test, gstack browser QA.

## Global Constraints

- Preserve existing light/dark theme variables and page spacing.
- Do not add JavaScript or page-specific selectors.
- Verify desktop, tablet, and mobile layouts.

---

### Task 1: Full-viewport page canvas

**Files:**
- Create: `web/embed_test.go`
- Modify: `web/styles.css`

**Interfaces:**
- Consumes: `web.FS` and the existing `--bg` theme variable.
- Produces: a shared root/body background and viewport-height contract.

- [x] **Step 1: Write the failing stylesheet contract test**

Assert that embedded `styles.css` gives `html` the application background and
gives `body` both `100vh` and `100dvh` minimum-height declarations.

- [x] **Step 2: Run the test and confirm the expected failure**

Run: `go test ./web -run TestStylesFillViewport -count=1`

Expected: FAIL because the declarations are absent.

- [x] **Step 3: Add the minimal global CSS declarations**

Set `html { background: var(--bg); }`, then add `min-height: 100vh` and
`min-height: 100dvh` to `body`.

- [x] **Step 4: Verify code and browser behavior**

Run the focused test, `go test ./... -count=1`, `go vet ./...`, and browser QA
at `1440x900`, `1024x1366`, and `390x844` on short and long routes.

- [x] **Step 5: Commit**

Commit message: `fix: fill viewport background on short pages`
