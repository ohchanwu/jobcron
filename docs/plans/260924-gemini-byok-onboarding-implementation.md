# Gemini BYOK onboarding implementation plan

**Status:** Approved for autonomous execution through the live-provider gate

**Spec:** [`../specs/260924-gemini-byok-onboarding.md`](../specs/260924-gemini-byok-onboarding.md)

**Baseline:** `fef481785bfe5bed1795d9ee46f287328c7f95f3`

## Goal

Make Gemini the recommended and preselected AI provider for newly created profiles, explain how to obtain and revoke a key, and preserve the existing encrypted-credential and real-evaluation paths. Do not add a connection-test endpoint or button.

There are no production users. Existing accounts are operator-created test profiles. Their AI provider/model may change if needed, but every non-AI profile field and all saved provider credentials must remain intact.

## Scope and implementation decisions

- Put Gemini first in the server-owned provider registry and label it as recommended.
- Preselect Gemini and its default model only when a profile has not yet been saved. An explicitly saved AI-off, Anthropic, OpenAI, or Gemini choice remains an accurate reflection of that test profile until it is edited; no bulk JSON rewrite is needed.
- Keep `없음 (끄기)`, Anthropic, and OpenAI available.
- Add a compact onboarding callout in the profile form with privacy wording, quota caveats, and links to Google AI Studio and the local setup guide.
- Add `GET /guides/gemini-api-key` as a static, server-rendered guide. The guide must cover creation, paste/save, first real evaluation, expected success/failure, quota or rate-limit errors, and revocation. It must explain that the key goes directly from Jobcron to Google Gemini and is encrypted at rest; it must not imply that Jobcron controls Google retention.
- Use the first real AI evaluation as the connection test. Do not add a synthetic provider request, route, handler, button, or rate limiter.
- Keep the written guide sufficient without video. Add only screenshots that can be captured locally without exposing credentials.
- Preserve all profile fields and credentials when a user edits only AI settings; never render a stored key back to the browser.

## TDD work slices

### 1. Provider order and first-profile defaults

1. Add focused tests that fail because Gemini is not first and a profile with no saved row does not render Gemini/default model as selected.
2. Implement the smallest server/provider changes to pass.
3. Add regression tests proving saved test profiles retain all non-AI fields and explicit AI-off/other-provider selections.
4. Run the focused provider/profile tests before continuing.

### 2. Onboarding callout and privacy copy

1. Add handler/template tests for the recommended badge, AI Studio link, guide link, privacy language, quota caveat, and `noopener noreferrer` protection on external links.
2. Implement the callout and scoped responsive styles.
3. Verify keyboard focus order and that the AI-off option remains selectable.

### 3. Written guide

1. Add a route/handler test for `GET /guides/gemini-api-key` and content assertions for key creation, first evaluation, error handling, revocation, privacy, and deferred video.
2. Add the embedded guide template, route, and minimal styling/navigation.
3. Add a locally captured Jobcron settings screenshot only if it contains no secret or personal data; otherwise keep the guide text-only rather than ship a misleading or sensitive image.

### 4. Safe errors and no alternate test path

1. Extend existing simulated-provider tests for invalid credentials, quota/rate limits, and unavailable model responses; assert actionable, secret-free user messages and offline scoring fallback.
2. Confirm routing tests contain no dedicated connection-test endpoint and the profile template contains no test button.
3. Keep the existing first-evaluation flow unchanged except for guidance copy.

### 5. Browser and repository verification

- Run `gofmt -l .`, focused tests, `go test ./...`, and `go vet ./...`.
- Run PostgreSQL-backed tests where the repository harness makes them available.
- Run `git diff --check`, relative Markdown link checking, and Gitleaks on the staged tree.
- Start or reuse the local preview server and inspect desktop and narrow mobile layouts without entering a real API key.
- Independently review the diff for spec compliance, security, and regressions.
- Commit reviewed work and integrate the approved branch into local `main`, preferably by fast-forward. Do not push or deploy.

## Human gate

Stop after the reviewed implementation is integrated locally and provide the preview URL plus a short checklist. The operator will then:

1. Open the guide and obtain a real Gemini key privately.
2. Paste and save it in Jobcron without sharing it in chat or logs.
3. Run the first real AI evaluation.
4. Confirm success or capture only the redacted error text.
5. Revoke or rotate the test key if desired.

Any production push, deployment, AWS/Cloudflare/DNS change, or public traffic change remains a separate authorization boundary.
