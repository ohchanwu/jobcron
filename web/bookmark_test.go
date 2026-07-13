package web

import (
	"os/exec"
	"testing"
)

func TestBookmarkLifecycleBehavior(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("node is required for the zero-package bookmark lifecycle test")
	}
	cmd := exec.Command(node, "testdata/bookmark-lifecycle.test.js")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bookmark lifecycle harness: %v\n%s", err, out)
	}
}
