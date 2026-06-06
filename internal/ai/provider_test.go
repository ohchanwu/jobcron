package ai

import (
	"testing"
	"time"
)

func TestSuggestedRateLimit(t *testing.T) {
	cases := map[string]time.Duration{
		"anthropic": 1200 * time.Millisecond, // ~50 req/min — tier-1 entry ceiling
		"openai":    200 * time.Millisecond,  // headroom; the pool becomes the bound
		"unknown":   time.Second,             // conservative fallback
		"":          time.Second,
	}
	for provider, want := range cases {
		if got := SuggestedRateLimit(provider); got != want {
			t.Errorf("SuggestedRateLimit(%q) = %v, want %v", provider, got, want)
		}
	}
}
