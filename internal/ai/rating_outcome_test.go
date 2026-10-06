package ai

import "testing"

func TestScoreDeltaRequiresExplicitArray(t *testing.T) {
	for _, raw := range []string{`{}`, `{"items":null}`, `{"items":{}}`, `{"items":""}`} {
		t.Run(raw, func(t *testing.T) {
			if _, err := parseScoreDelta([]byte(raw)); err == nil {
				t.Fatal("missing or non-array items must fail, not become a genuine empty success")
			}
		})
	}
	for _, raw := range []string{`{"items":[]}`, `[]`} {
		items, err := parseScoreDelta([]byte(raw))
		if err != nil || len(items) != 0 {
			t.Fatalf("explicit empty %s: items=%v err=%v", raw, items, err)
		}
	}
}

func TestScoreDeltaPreservesRejectedProposalProvenance(t *testing.T) {
	for _, raw := range []string{
		`{"items":[{"kind":"unknown","delta":9}]}`,
		`{"items":[{"signal":"zero","kind":"presence","delta":0,"quote":"서버 개발자를 찾습니다","matched_goal":"업무"}]}`,
		`{"items":[null]}`,
		`{"items":[{"delta":"wrong type"}]}`,
	} {
		t.Run(raw, func(t *testing.T) {
			items, err := parseScoreDelta([]byte(raw))
			if err != nil || len(items) != 1 {
				t.Fatalf("invalid proposal must reach the gate, never become explicit empty: items=%+v err=%v", items, err)
			}
			if d := GateDelta(items, "서버 개발자를 찾습니다", ""); len(d.Items) != 0 {
				t.Fatalf("invalid proposal survived: %+v", d)
			}
		})
	}
}
