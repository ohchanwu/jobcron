package ai

import (
	"context"
	"errors"
	"time"
)

// ErrNotImplemented is returned by a provider method whose body has not been
// wired yet (e.g. a stub with no func set, or ScoreDelta before Stage 2).
var ErrNotImplemented = errors.New("ai: not implemented")

// ErrUnknownProvider is returned by New for an unrecognized provider name.
var ErrUnknownProvider = errors.New("ai: unknown provider")

// Provider is the seam for a BYOK AI backend. It is data-in/data-out: a
// posting's model text goes in, structured facts come out. No tool-use, no
// streaming — that contract is what lets the one-host egress pin hold.
type Provider interface {
	// Name is the stable provider id, e.g. "anthropic" or "openai".
	Name() string

	// Extract reads one posting's assembled model text and returns the
	// structured career/education extraction. A non-nil error means the
	// caller must fall back to the offline regex path and persist no cache
	// row. Usage carries the token counts the ledger debits.
	Extract(ctx context.Context, modelText string) (Extraction, Usage, error)

	// ScoreDelta weighs one posting (modelText) against the applicant's
	// free-form goals (profileText) and returns the raw, un-gated per-signal
	// items. A non-nil error means the caller applies no delta for that
	// posting. The returned items are NOT citation-gated — the caller passes
	// them through GateDelta before trusting any of them.
	ScoreDelta(ctx context.Context, modelText, profileText string) ([]RawDeltaItem, Usage, error)
}

// Extraction is the validated Stage-1 result, mirroring the ai_extractions
// columns. The caller supplies posting id / content_hash / ai_version; the
// model supplies these fields.
type Extraction struct {
	MinCareer     int    // years, lower bound; >= 0
	MaxCareer     *int   // nil = open upper bound (read maps nil -> experienceUpperOpen 99)
	Newcomer      bool   // the model's 신입-eligible judgment
	EducationEnum string // raw enum: none|highschool|associate|bachelor|master|doctorate
	Evidence      string // short cited quote (stored; not gated in Stage 1)
}

// DeltaItem is one gated per-signal contribution from the Stage-2 ScoreDelta
// path. Defined in Stage 1 so scoring.Score's signature can reference
// *ai.Delta; produced in Stage 2 (T5).
type DeltaItem struct {
	Signal      string `json:"signal"`
	Kind        string `json:"kind"` // "presence" | "absence"
	Delta       int    `json:"delta"`
	Evidence    string `json:"evidence,omitempty"`
	MatchedGoal string `json:"matched_goal,omitempty"`
}

// Delta is the Stage-2 AI score delta: the surviving gated items and their
// net sum. Defined in Stage 1 (type only); filled by ScoreDelta in Stage 2.
//
// Stale marks a delta computed against a PRIOR profile (the scoreAll merge
// sets it when it falls back to the latest cached row because no row matches
// the current ai_input_hash). A stale delta is still summed into the Total;
// the chip is just labelled "(이전 프로필 기준)". scoreCareer's merge reads it
// onto the AI LineItem (T6 renders the stale chrome).
type Delta struct {
	Items    []DeltaItem
	NetDelta int
	Stale    bool
}

// Usage is the token accounting returned by every provider call. Stage 1
// returns it; T9's ai_usage ledger debits it.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// New constructs a live provider for the given name ("anthropic" | "openai")
// with the user's API key and chosen model. rateLimit is the minimum spacing
// between requests (pass 0 in tests to disable pacing). It returns
// ErrUnknownProvider for any other name. The returned provider is pinned to
// that provider's single API host.
func New(providerName, apiKey, model string, rateLimit time.Duration) (Provider, error) {
	spec, ok := specByName[providerName]
	if !ok {
		return nil, ErrUnknownProvider
	}
	return newHTTPProvider(spec, apiKey, model, spec.defaultBaseURL, rateLimit)
}

// defaultModelByProvider is the model used when the user selects a provider but
// leaves the model field blank. Both default to the provider's small, cheap tier
// — the extraction/scoring task is short, and BYOK users pay per token. The user
// can override either in the profile form.
var defaultModelByProvider = map[string]string{
	"anthropic": "claude-haiku-4-5-20251001",
	"openai":    "gpt-4o-mini",
}

// DefaultModel returns the fallback model id for a provider, or "" for an
// unknown provider name. Used by the server when the profile sets a provider but
// no explicit model.
func DefaultModel(providerName string) string {
	return defaultModelByProvider[providerName]
}

// Providers lists the selectable provider ids for the settings UI, in display
// order.
func Providers() []string { return []string{"anthropic", "openai"} }

// suggestedRateLimit is the per-provider minimum spacing between live requests,
// tuned to each provider's entry-tier requests-per-minute ceiling so a BYOK user
// on a fresh key doesn't trip 429s:
//
//   - anthropic: ~50 req/min on the tier-1 (entry) Claude limits → 1.2s spacing.
//   - openai: hundreds of req/min on entry tiers → 200ms, so the 재평가 worker
//     pool (rerateWorkers), not the limiter, becomes the throughput bound.
//
// The limiter only spaces request STARTS — waitForRateLimit releases its lock
// before sleeping — so concurrent pool calls still overlap. A 429 is not fatal:
// the caller falls back to the row's regex score and retries on the next press.
var suggestedRateLimit = map[string]time.Duration{
	"anthropic": 1200 * time.Millisecond,
	"openai":    200 * time.Millisecond,
}

// SuggestedRateLimit returns the default request spacing for a provider, falling
// back to a conservative 1s for an unknown name. The server passes this to New
// so pacing matches the chosen provider's rate limits.
func SuggestedRateLimit(providerName string) time.Duration {
	if d, ok := suggestedRateLimit[providerName]; ok {
		return d
	}
	return time.Second
}
