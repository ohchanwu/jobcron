package ai

import (
	"strings"
	"testing"
)

func TestRatingPromptMatchesGateAndContainsValidExample(t *testing.T) {
	for _, want := range []string{"6 Unicode characters", "2 tokens", "contiguous", "posting text actually sent", "[-30, +30]", "[-40, +40]", "explicit supported conflict", "data"} {
		if !strings.Contains(scoreDeltaSystemPrompt, want) {
			t.Errorf("prompt missing gate/calibration contract %q", want)
		}
	}
	raw, err := parseScoreDelta([]byte(scoreDeltaSystemPrompt))
	if err != nil {
		t.Fatalf("prompt example must be valid JSON: %v", err)
	}
	d := GateDelta(raw, "서버 개발자를 찾습니다", "")
	if len(d.Items) != 1 {
		t.Fatalf("prompt example fails the actual gate: %+v", d)
	}
}

func TestRatingContractOnlyRotatesStage2(t *testing.T) {
	if ScoreVersion("anthropic", "claude-x") == taskVersion("anthropic", "claude-x", "1") {
		t.Fatal("Stage-2 contract must miss old-version caches")
	}
	if ExtractionContractVersion() != taskVersion("extraction-contract", "1") {
		t.Fatal("extraction contract rotated")
	}
	if DealbreakerVersion("anthropic", "claude-x") != taskVersion("anthropic", "claude-x", "dealbreaker", "2") {
		t.Fatal("dealbreaker contract rotated")
	}
}
