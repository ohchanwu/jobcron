package ai

import (
	"crypto/sha256"
	"encoding/hex"
)

// PromptTemplateVersion is bumped by hand whenever the extraction or scoring
// prompt text changes in a way that should invalidate cached AI output. It is
// one of the three inputs to AIVersion, so bumping it makes every prior cache
// row (keyed on ai_version) a clean miss that recomputes under the new prompt.
const PromptTemplateVersion = "1"

// AIVersion returns the cache-partitioning version string for a (provider,
// model, prompt-template) combination: the first 12 hex chars of
// sha256(provider \x00 model \x00 promptTemplateVersion). It is part of the
// primary key of both ai_extractions and ai_scores, so any change to provider,
// model, or PromptTemplateVersion correctly invalidates stale cached output.
//
// The \x00 separators keep ("ab","c") distinct from ("a","bc"). The [:12]
// truncation matches storage.profileHash's convention.
func AIVersion(provider, model string) string {
	sum := sha256.Sum256([]byte(provider + "\x00" + model + "\x00" + PromptTemplateVersion))
	return hex.EncodeToString(sum[:])[:12]
}
