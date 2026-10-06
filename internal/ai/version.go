package ai

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const (
	extractionContractRevision = "1"
	DealbreakerPromptVersion   = "2"
	ScorePromptVersion         = "2"
)

func taskVersion(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])[:12]
}

func ExtractionContractVersion() string {
	return taskVersion("extraction-contract", extractionContractRevision)
}

func DealbreakerVersion(provider, model string) string {
	return taskVersion(provider, model, "dealbreaker", DealbreakerPromptVersion)
}

// ScoreVersion keys the Stage-2 contract independently of extraction/dealbreakers.
func ScoreVersion(provider, model string) string {
	return taskVersion(provider, model, ScorePromptVersion)
}

// AIVersion is the Stage-2 compatibility alias.
func AIVersion(provider, model string) string {
	return ScoreVersion(provider, model)
}
