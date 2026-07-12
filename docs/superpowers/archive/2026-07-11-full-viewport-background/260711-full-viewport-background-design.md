# Full-Viewport Background Design

## Problem

Short pages let the `body` element end at its content height. Browser extensions
such as Dark Reader may then render the exposed `html` canvas with a different
background, producing a visible band below the application.

## Approved Design

- Apply `var(--bg)` to `html` as well as `body`, so the root canvas and page use
  the same application background.
- Give `body` a `100vh` minimum height with `100dvh` as the modern viewport-unit
  override.
- Do not change `.wrap`, footer spacing, page templates, or theme behavior.

## Verification

- Add a stylesheet contract test for the root background and body minimum
  height.
- Inspect empty bookmarks, hidden postings, profile, and dashboard routes at
  desktop, tablet, and mobile dimensions.
- Simulate different extension colors on the root and verify no exposed band,
  horizontal overflow, console error, or failed local asset.
