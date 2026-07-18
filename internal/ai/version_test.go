package ai

import (
	"regexp"
	"testing"
)

var hex12 = regexp.MustCompile(`^[0-9a-f]{12}$`)

func TestTaskVersionsAreStableAndPartitioned(t *testing.T) {
	score := ScoreVersion("anthropic", "claude-x")
	if !hex12.MatchString(score) {
		t.Fatalf("ScoreVersion = %q, want 12 lowercase hex chars", score)
	}
	if score != "925859b252bb" {
		t.Fatalf("ScoreVersion = %q, want the pre-split Stage 2 identity", score)
	}
	if score != AIVersion("anthropic", "claude-x") {
		t.Fatal("Stage 2 cache identity changed")
	}
	if EligibilityVersion("anthropic", "claude-x") == score {
		t.Fatal("eligibility and score versions must be separate")
	}
	if DealbreakerVersion("anthropic", "claude-x") == score {
		t.Fatal("dealbreaker and score versions must be separate")
	}
	eligibility := EligibilityVersion("anthropic", "claude-x")
	if eligibility != taskVersion("anthropic", "claude-x", "eligibility", EligibilityPromptVersion) {
		t.Fatal("EligibilityVersion must use the eligibility task name and prompt version")
	}
	dealbreaker := DealbreakerVersion("anthropic", "claude-x")
	if dealbreaker != taskVersion("anthropic", "claude-x", "dealbreaker", DealbreakerPromptVersion) {
		t.Fatal("DealbreakerVersion must use the dealbreaker task name and prompt version")
	}
	if eligibility == dealbreaker {
		t.Fatal("eligibility and dealbreaker versions must be separate")
	}
	if ScoreVersion("anthropic", "claude-x") != score {
		t.Fatal("score version must be stable")
	}

	versions := []struct {
		name string
		fn   func(string, string) string
	}{
		{"eligibility", EligibilityVersion},
		{"dealbreaker", DealbreakerVersion},
		{"score", ScoreVersion},
	}
	for _, version := range versions {
		base := version.fn("anthropic", "claude-x")
		if version.fn("openai", "claude-x") == base {
			t.Errorf("%s version did not rotate with provider", version.name)
		}
		if version.fn("anthropic", "claude-y") == base {
			t.Errorf("%s version did not rotate with model", version.name)
		}
	}

	if taskVersion("anthropic", "claude-x", "eligibility", EligibilityPromptVersion) ==
		taskVersion("anthropic", "claude-x", "other", EligibilityPromptVersion) {
		t.Fatal("task name must rotate a task-specific version")
	}
	if taskVersion("anthropic", "claude-x", "eligibility", EligibilityPromptVersion) ==
		taskVersion("anthropic", "claude-x", "eligibility", "other") {
		t.Fatal("task prompt version must rotate its corresponding version")
	}
	if ScoreVersion("ab", "c") == ScoreVersion("a", "bc") {
		t.Fatal("version parts must be NUL-separated")
	}
}
