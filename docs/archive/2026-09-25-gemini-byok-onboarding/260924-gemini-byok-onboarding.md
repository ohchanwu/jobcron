# Gemini BYOK Onboarding

**Date:** 2026-09-24
**Status:** Completed and integrated 2026-09-25

## Goal

Make Gemini the easiest recommended AI provider for Jobcron users without
promising permanent free access or a provider-independent fixed quota.

## Final contract

- Gemini is shown first and is the recommended provider. An unsaved profile
  preselects `gemini-3.5-flash-lite`; saved provider, model, AI-off, and other
  profile choices remain truthful and are not rewritten.
- The profile callout links to Google AI Studio and the local setup guide. AI
  remains optional, and deterministic scoring continues without a key.
- `/guides/gemini-api-key` is a concise Korean guide with exactly six numbered
  steps: sign in, create a key, copy it, paste it into Jobcron, choose
  Flash-Lite and save, then run the first real AI evaluation.
- The guide says that Flash-Lite supports approximately 500 requests per day,
  scoped to Google's project/model limits. It also warns that other uses of the
  same Google project share that limit, and points users to wait for reset or
  use a paid-tier project if necessary.
- The guide includes Google's unpaid-tier data-use warning, sensitive-data
  guidance, and short troubleshooting for invalid keys, HTTP 429/quota,
  model mismatch, and regional restrictions.
- Project selection is an unnumbered note under key creation, and key handling
  is a short copy warning. A standalone key storage/replacement/revocation
  section and verbose connection-test explanation are intentionally omitted.
- The first real AI evaluation is the only connection check. No synthetic test
  route or button exists. A setup video is deferred.

## Verification contract

The integrated provider/profile and guide tests cover recommendation order,
unsaved defaults, preservation of saved profiles, the six-step structure,
project-scoped quota wording, safe external links, troubleshooting copy, and
the absence of synthetic test routes. Browser users can return to the profile
from the guide, and the guide remains text-only so it contains no credentials
or private data.
