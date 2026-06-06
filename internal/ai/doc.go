// Package ai is the BYOK (bring-your-own-key) AI provider layer for v2.0.
//
// It is deliberately thin: a Provider interface, a hand-rolled net/http client
// for Anthropic (no SDK, to keep the pure-Go CGO-free single-binary build
// intact), a one-host egress pin enforced in Transport.DialContext, a 0600 key
// store, and a stub provider for the offline test suite. (OpenAI was a second
// provider behind the same providerSpec chassis; it was removed because its low
// free-tier rate limit couldn't sustain the re-rate workload — git history has
// the spec.)
//
// Layering: ai imports only the standard library, golang.org/x/text, and
// internal/scraper (for the Posting domain type that buildModelText reads).
// It MUST NOT import internal/scoring — scoring imports ai (Score takes
// *ai.Extraction / *ai.Delta), so the reverse would be an import cycle.
//
// Stage 1 (v2.0.0-alpha) ships Extract; ScoreDelta is added to the Provider
// interface in Stage 2 (T5).
package ai
