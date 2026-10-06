package profile

import (
	"strings"
	"testing"
)

func TestAIPerCallCapJSONCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name    string
		json    string
		wantRaw int
		wantCap int
	}{
		{name: "legacy absent", json: `{}`, wantCap: 200},
		{name: "zero", json: `{"ai_per_call_cap":0}`, wantCap: 200},
		{name: "negative", json: `{"ai_per_call_cap":-1}`, wantRaw: -1, wantCap: 200},
		{name: "explicit old default", json: `{"ai_per_call_cap":50}`, wantRaw: 50, wantCap: 50},
		{name: "explicit 100", json: `{"ai_per_call_cap":100}`, wantRaw: 100, wantCap: 100},
		{name: "explicit 200", json: `{"ai_per_call_cap":200}`, wantRaw: 200, wantCap: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Unmarshal(tc.json)
			if err != nil {
				t.Fatal(err)
			}
			if p.AIPerCallCap != tc.wantRaw || p.EffectiveAIPerCallCap() != tc.wantCap {
				t.Errorf("cap raw=%d effective=%d, want raw=%d effective=%d", p.AIPerCallCap, p.EffectiveAIPerCallCap(), tc.wantRaw, tc.wantCap)
			}
			canonical, err := Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantRaw == 0 && strings.Contains(canonical, "ai_per_call_cap") {
				t.Errorf("unset cap must stay omitted: %s", canonical)
			}
			roundTrip, err := Unmarshal(canonical)
			if err != nil {
				t.Fatal(err)
			}
			if roundTrip.AIPerCallCap != tc.wantRaw || roundTrip.EffectiveAIPerCallCap() != tc.wantCap {
				t.Errorf("round-trip cap raw=%d effective=%d, want raw=%d effective=%d", roundTrip.AIPerCallCap, roundTrip.EffectiveAIPerCallCap(), tc.wantRaw, tc.wantCap)
			}
			if AIInputHash(p) != AIInputHash(Profile{}) {
				t.Error("per-call cap must not change the goal-keyed AI cache")
			}
		})
	}
}
