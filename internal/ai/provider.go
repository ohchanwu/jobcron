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
//
// Stage 1 defines Name + Extract. ScoreDelta (Stage 2 / T5) is added to this
// interface when the per-signal delta path lands.
type Provider interface {
	// Name is the stable provider id, e.g. "anthropic" or "openai".
	Name() string

	// Extract reads one posting's assembled model text and returns the
	// structured career/education extraction. A non-nil error means the
	// caller must fall back to the offline regex path and persist no cache
	// row. Usage carries the token counts the (deferred) ledger debits.
	Extract(ctx context.Context, modelText string) (Extraction, Usage, error)
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
type Delta struct {
	Items    []DeltaItem
	NetDelta int
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
