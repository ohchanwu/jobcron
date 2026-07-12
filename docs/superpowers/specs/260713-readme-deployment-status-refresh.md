# README Deployment Status Refresh

**Status:** Approved design, awaiting implementation

## Goal

Refresh the English and Korean root READMEs so they describe the current
deployment state accurately and show the current application UI.

## Public Message

- The read-only demo is live at <https://demo.jobcron.app>.
- The full production application at `jobcron.app` is not live yet.
- Production deployment configuration is ready and the launch is coming soon.
- Until production launches, users can run the full application locally.

The message should appear near the README introduction and should replace any
absolute claim that the project has no server or web deployment.

## Screenshot

Replace both README theme images at their existing paths:

- `docs/assets/screenshots/dashboard.png`
- `docs/assets/screenshots/dashboard-dark.png`

Capture the latest committed application in light and dark themes at a desktop
viewport. Use public-safe data, show the current AI-analysis presentation, and
exclude credentials, private profile text, machine paths, and browser chrome.
Keep the existing `<picture>` structure so GitHub selects the image matching the
reader's color scheme.

## Demo Scrape Helper

Track `deploy/demo/SCRAPE_CMD.sh` exactly as the existing working one-line helper.
It may reference environment-variable names and the public demo URL, but it must
not contain an actual token or other credential. Do not reformat or redesign it.

## Verification

- Inspect both screenshots at full resolution.
- Verify the updated README image paths and links.
- Render both READMEs on GitHub-compatible Markdown or inspect the corresponding
  HTML structure.
- Run `sh -n deploy/demo/SCRAPE_CMD.sh` without executing the remote scrape.
- Run Markdown diff checks and Gitleaks before committing.
- Confirm only the intended README, screenshot, helper, and lifecycle-document
  files are included in the final push.
