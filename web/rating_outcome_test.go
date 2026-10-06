package web

import (
	"strings"
	"testing"
)

func TestRatingOutcomesHaveDistinctThemeAwarePalette(t *testing.T) {
	contents, err := FS.ReadFile("styles.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(contents)
	for _, want := range []string{
		"--ai-neutral-bg:", "--ai-neutral-text:", "--ai-rejected-bg:", "--ai-rejected-text:",
		".ai-outcome-neutral {", ".ai-outcome-rejected {", "overflow-wrap: anywhere;",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("missing outcome theme/wrapping rule %q", want)
		}
	}
}
