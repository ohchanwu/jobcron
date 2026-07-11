package web

import (
	"strings"
	"testing"
)

func TestStylesFillViewport(t *testing.T) {
	contents, err := FS.ReadFile("styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}
	css := string(contents)

	for _, declaration := range []string{
		"html { background: var(--bg); }",
		"min-height: 100vh;",
		"min-height: 100dvh;",
	} {
		if !strings.Contains(css, declaration) {
			t.Errorf("styles.css missing %q", declaration)
		}
	}
}
