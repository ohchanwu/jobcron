package ai

import (
	"testing"
	"time"
)

func TestModelsForProvider(t *testing.T) {
	for _, prov := range Providers() {
		models := ModelsForProvider(prov)
		if len(models) == 0 {
			t.Fatalf("ModelsForProvider(%q) is empty — the dropdown would offer nothing", prov)
		}
		// The dropdown's first model must equal the provider's default, so the
		// "기본값" (empty) choice and the first explicit option agree.
		if models[0] != DefaultModel(prov) {
			t.Errorf("ModelsForProvider(%q)[0] = %q, want the default %q first", prov, models[0], DefaultModel(prov))
		}
	}
	if ModelsForProvider("groq") != nil {
		t.Error("an unknown provider must return nil models")
	}
}

func TestModelsByProviderReturnsACopy(t *testing.T) {
	m := ModelsByProvider()
	if _, ok := m["anthropic"]; !ok {
		t.Fatal("ModelsByProvider missing anthropic")
	}
	// Mutating the returned map/slices must not corrupt the package's source of
	// truth — the form marshals this map to JSON every render.
	m["anthropic"][0] = "tampered"
	delete(m, "openai")
	if ModelsForProvider("anthropic")[0] == "tampered" {
		t.Error("ModelsByProvider leaked a shared slice — caller mutation reached package state")
	}
	if ModelsForProvider("openai") == nil {
		t.Error("ModelsByProvider leaked a shared map — caller deletion reached package state")
	}
}

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
