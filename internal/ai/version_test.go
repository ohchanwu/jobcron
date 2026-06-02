package ai

import (
	"regexp"
	"testing"
)

var hex12 = regexp.MustCompile(`^[0-9a-f]{12}$`)

func TestAIVersionStableAndSensitive(t *testing.T) {
	base := AIVersion("anthropic", "claude-x")
	if !hex12.MatchString(base) {
		t.Fatalf("AIVersion = %q, want 12 lowercase hex chars", base)
	}
	if again := AIVersion("anthropic", "claude-x"); again != base {
		t.Fatalf("AIVersion not deterministic: %q vs %q", base, again)
	}
	if AIVersion("openai", "claude-x") == base {
		t.Fatal("changing provider must change AIVersion")
	}
	if AIVersion("anthropic", "claude-y") == base {
		t.Fatal("changing model must change AIVersion")
	}
	// Separator guard: ("ab","c") must differ from ("a","bc").
	if AIVersion("ab", "c") == AIVersion("a", "bc") {
		t.Fatal("AIVersion must not collide across the provider/model boundary")
	}
}
