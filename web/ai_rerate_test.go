package web

import (
	"os/exec"
	"testing"
)

// TestAIRerateLifecycleBehavior executes the shipped browser client against a
// zero-package Node harness. The production binary stays pure Go; Node is only
// the test runner for real JavaScript lifecycle behavior.
func TestAIRerateLifecycleBehavior(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("node is required for the zero-package ai-rerate lifecycle test")
	}
	cmd := exec.Command(node, "testdata/ai-rerate-lifecycle.test.js")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ai-rerate lifecycle harness: %v\n%s", err, out)
	}
}
