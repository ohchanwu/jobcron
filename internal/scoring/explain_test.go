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

	missing := ScoreResult{Total: -1, DealbreakerHit: &DealbreakerHit{Kind: "must_have_missing", Phrase: "재택"}}
	if got := Explain(missing); got != "재택 누락 ⛔" {
		t.Errorf("Explain(missing) = %q, want \"재택 누락 ⛔\"", got)
	}
}
