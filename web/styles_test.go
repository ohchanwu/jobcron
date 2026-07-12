package web

import (
	"strings"
	"testing"
)

func TestAIAnalysisUsesApprovedIndigoTokens(t *testing.T) {
	b, err := FS.ReadFile("styles.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)
	for _, want := range []string{
		"--ai-chip:     light-dark(#eeeafe, #29233f)",
		"--ai-panel:    light-dark(#f3f0ff, #211c34)",
		"--ai-border:   light-dark(#c9bdfa, #5b4d8d)",
		"--ai-text:     light-dark(#3f307c, #ede8ff)",
		"--ai-accent:   light-dark(#6748c7, #b7a7ff)",
		"background: var(--ai-chip)",
		"background: var(--ai-panel)",
		"outline: 2px solid var(--ai-accent)",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("styles.css missing %q", want)
		}
	}
}

func TestAIAnalysisUsesIndigoEvidenceTextAndReducedMotion(t *testing.T) {
	b, err := FS.ReadFile("styles.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)
	if strings.Contains(css, ".ai-evidence-text { color: var(--ink); }") {
		t.Error("AI evidence text must not override the panel's approved indigo text token")
	}
	for _, want := range []string{
		".ai-evidence-text { color: var(--ai-text); }",
		"@media (prefers-reduced-motion: reduce) {\n  .chip-ai .chip-caret { transition: none; }\n}",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("styles.css missing %q", want)
		}
	}
}
