package scoring

import "testing"

func TestExplain(t *testing.T) {
	scored := ScoreResult{
		Total: 75,
		Breakdown: []LineItem{
			{Label: "React", Delta: 50, Reason: "기술 스택 일치"},
			{Label: "신입", Delta: 25, Reason: "경력 조건 일치"},
		},
	}
	if got := Explain(scored); got != "React +50 · 신입 +25" {
		t.Errorf("Explain(scored) = %q, want \"React +50 · 신입 +25\"", got)
	}

	keyword := ScoreResult{Total: -1, DealbreakerHit: &DealbreakerHit{Kind: "keyword", Phrase: "병특"}}
	if got := Explain(keyword); got != "병특 ⛔" {
		t.Errorf("Explain(keyword) = %q, want \"병특 ⛔\"", got)
	}

	edu := ScoreResult{Total: -1, DealbreakerHit: &DealbreakerHit{Kind: "education", Phrase: "대학교졸업(4년) 이상"}}
	if got := Explain(edu); got != "대학교졸업(4년) 이상 ⛔" {
		t.Errorf("Explain(edu) = %q, want \"대학교졸업(4년) 이상 ⛔\"", got)
	}
}
