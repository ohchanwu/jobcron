package ai

import "testing"

func TestRatingGateRequiresSignalAndGoal(t *testing.T) {
	for _, raw := range []string{
		`{"items":[{"kind":"presence","delta":8,"quote":"서버 개발자를 찾습니다","matched_goal":"업무"}]}`,
		`{"items":[{"signal":"백엔드","kind":"presence","delta":8,"quote":"서버 개발자를 찾습니다"}]}`,
		`{"items":[{"signal":" ","kind":"presence","delta":8,"quote":"서버 개발자를 찾습니다","matched_goal":"업무"}]}`,
	} {
		items, err := parseScoreDelta([]byte(raw))
		if err != nil || len(items) != 1 {
			t.Fatalf("proposal lost: %v %v", items, err)
		}
		if d := GateDelta(items, "서버 개발자를 찾습니다", ""); len(d.Items) != 0 {
			t.Fatalf("required fields missing but accepted: %+v", d)
		}
	}
}
