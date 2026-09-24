# Gemini BYOK Onboarding

**Date:** 2026-09-24
**Status:** Approved; implementation pending

## Goal

Make Gemini the easiest recommended AI provider for Jobcron's developer-focused users without promising permanent free access or a fixed quota.

## Product Changes

### 1. Profile AI settings

Show Gemini first and preselect `gemini-3.5-flash-lite`. Add a visually prominent recommendation above the provider controls:

> **Recommended: Google Gemini**
>
> Gemini offers an unpaid API tier suitable for getting started. Setup usually takes 2–5 minutes. Google controls availability and usage limits.

Provide two actions:

- **Get a Gemini API key** — opens Google AI Studio in a new tab.
- **View setup guide** — opens Jobcron's guide.

Keep Anthropic and OpenAI selectable. AI remains optional and deterministic scoring continues without a key.

There are no production users yet. Existing accounts are operator-created test
profiles and may be moved to the Gemini/Flash-Lite default. Preserve every
other profile field and all associated postings, scores, bookmarks, hidden
state, usage records, and saved provider credentials so the operator does not
have to recreate test data. New profiles receive the same Gemini/Flash-Lite
default; a user may still select another provider or turn AI off.

### 2. Setup guide

Add a Jobcron page at `/guides/gemini-api-key` with current screenshots and these steps:

1. Open Google AI Studio and sign in.
2. Create or select a project.
3. Create an API key.
4. Copy the key.
5. In Jobcron, select Gemini and paste the key.
6. Select Flash-Lite, save the settings, and run the first AI evaluation. The
   real evaluation is the connection test: it verifies the saved key, selected
   model, provider endpoint, and normal Jobcron request path together.

Include short troubleshooting for invalid keys, quota exhaustion/HTTP 429, model mismatch, regional restrictions, and key replacement or revocation.

Do not add a separate connection-test button or endpoint for the alpha. It
would duplicate only part of the real request path and expand the pre-deployment
surface. The human operator will manually verify this first-evaluation flow.

### 3. Video (deferred)

Do not produce or embed a setup video in this implementation. Ship the complete
written guide first; a short video may be added later if user feedback shows it
would materially improve onboarding. The written guide must remain sufficient
on its own because Google's UI can change before a video is updated.

### 4. Safety and wording

- Say **"unpaid API tier"**, not "free forever" or a fixed request count.
- Link to Google AI Studio's quota dashboard.
- Explain that unpaid-tier prompts and responses may be used by Google to improve its products.
- Tell users not to enter résumés or sensitive, confidential, or identifying information in AI profile fields.
- State that Jobcron encrypts saved provider keys and that users can revoke them through Google AI Studio.

## Verification

- A new user can reach Google AI Studio, create a key, save it, and complete a
  first real AI evaluation using only the guide.
- Settings render Gemini as recommended while preserving all providers and the AI-off path.
- Existing test profiles adopt the Gemini/Flash-Lite default without changing
  any non-AI profile information or associated user data.
- During that real evaluation, invalid, exhausted, and valid keys produce
  distinct, actionable messages.
- Keyboard and mobile users can complete the guide and settings flow.
- No copy promises a fixed quota, permanent free access, or privacy terms Google does not provide.
