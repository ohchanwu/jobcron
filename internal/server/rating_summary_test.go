package server

import (
	"strings"
	"testing"
)

func TestRatingSummaryKeepsStage2CountsWhenContextIsPending(t *testing.T) {
	summary := rerateSummary{
		Processed: 8, Visible: 8, Analyzed: 6, NoSignal: 1, Rejected: 1,
		ContextPendingBefore: 1, ContextPendingAfter: 1,
		ContextFailureMessage: "AI 문맥 확인을 완료하지 못했어요.",
	}
	message := rerateDoneMessage(summary)
	for _, want := range []string{
		"처리 8/8 · 분석 완료 6/8 · 추가 반영 없음 1 · 근거 미확인 1",
		"AI 문맥 확인을 완료하지 못했어요.",
		"AI 문맥 확인 1개가 남았어요.",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("mixed-phase summary %q missing %q", message, want)
		}
	}
}
