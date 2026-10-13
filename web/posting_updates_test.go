package web

import (
	"os/exec"
	"testing"
)

func TestPostingSnapshotAdmissionBehavior(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("node is required for posting update behavioral tests")
	}
	out, err := exec.Command(node, "testdata/posting-updates.test.js").CombinedOutput()
	if err != nil {
		t.Fatalf("posting updates: %v\n%s", err, out)
	}
}

func TestPostingObserverLifecycleBehavior(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("node is required for posting lifecycle behavioral tests")
	}
	out, err := exec.Command(node, "testdata/posting-lifecycle.test.js").CombinedOutput()
	if err != nil {
		t.Fatalf("posting lifecycle: %v\n%s", err, out)
	}
}

func TestPostingMutatorsIgnoreLateReplies(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("node is required for mutation behavioral tests")
	}
	out, err := exec.Command(node, "testdata/posting-mutators.test.js").CombinedOutput()
	if err != nil {
		t.Fatalf("posting mutators: %v\n%s", err, out)
	}
}
