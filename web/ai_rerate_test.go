package web

import (
	"strings"
	"testing"
)

func rerateScript(t *testing.T) string {
	t.Helper()
	b, err := FS.ReadFile("ai-rerate.js")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func requireRerateScriptContains(t *testing.T, script string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(script, want) {
			t.Errorf("ai-rerate.js missing lifecycle contract %q", want)
		}
	}
}

func TestAIRerateLifecycleScopesRunToHistoryEntry(t *testing.T) {
	script := rerateScript(t)
	requireRerateScriptContains(t, script,
		"jobcronRerateEntry",
		"history.replaceState",
		"function runOwnerKey(runID)",
		"sessionStorage.setItem(runOwnerKey(runID), entryToken)",
		"function ownsRun(runID)",
		"if (!ownsRun(runID)) return",
		"entry_token: entryToken",
		"notice.entry_token !== entryToken",
	)
}

func TestAIRerateLifecycleAbortsAndInvalidatesStatusPolls(t *testing.T) {
	script := rerateScript(t)
	requireRerateScriptContains(t, script,
		"lifecycleGeneration++",
		"statusController.abort()",
		"new AbortController()",
		"signal: controller.signal",
		"function isCurrent(generation)",
		"if (!isCurrent(generation)) return",
	)
	if got := strings.Count(script, "if (!isCurrent(generation)) return"); got < 5 {
		t.Errorf("generation guard count = %d, want at least 5 continuations guarded", got)
	}
}

func TestAIRerateLifecycleClearsRestoredTerminalState(t *testing.T) {
	script := rerateScript(t)
	requireRerateScriptContains(t, script,
		"function clearStatus()",
		"function clearProgress()",
		"clearProgress();",
		"if (status.state === 'idle')",
		"if (!ownsRun(status.run_id))",
		"if (handled) clearStatus();",
	)
}
