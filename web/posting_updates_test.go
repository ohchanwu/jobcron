package web

import (
	"bytes"
	"html/template"
	"os/exec"
	"strings"
	"testing"
)

func TestRerateOffKeepsHiddenLiveShell(t *testing.T) {
	tmpl, err := template.ParseFS(FS, "_rerate.html")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := tmpl.ExecuteTemplate(&out, "rerateButton", nil); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`class="rerate-row" hidden`, `id="rerate"`, `disabled`, `class="rerate-count"`, `class="rerate-feedback" hidden`, `id="rerate-log"`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("AI-off shell missing %s: %s", want, out.String())
		}
	}
}

func TestRerateLiveContextBehavior(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("node is required for AI context behavioral tests")
	}
	if out, err := exec.Command(node, "testdata/ai-rerate-context.test.js").CombinedOutput(); err != nil {
		t.Fatalf("AI live context: %v\n%s", err, out)
	}
}

func TestRerateHiddenShellOverridesFlexLayout(t *testing.T) {
	css, err := FS.ReadFile("styles.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), ".rerate-row[hidden], .rerate-feedback[hidden] { display: none; }") {
		t.Fatal("hidden AI shells must override their explicit flex layout")
	}
}

func TestPostingScopedTerminalSummaryBehavior(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("node is required for scoped summary behavioral tests")
	}
	if out, err := exec.Command(node, "testdata/posting-summary.test.js").CombinedOutput(); err != nil {
		t.Fatalf("posting terminal summary: %v\n%s", err, out)
	}
}

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
