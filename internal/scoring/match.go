package scoring

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
	"github.com/ohchanwu/jobcron/internal/scraper"
	"github.com/ohchanwu/jobcron/internal/tokenmatch"
	"golang.org/x/text/unicode/norm"
)

// tokenize splits text the way SQLite FTS5's unicode61 tokenizer does: it
// NFC-normalizes the text, breaks it into maximal runs of letters and digits
// (every other character is a separator), and lowercases each token.
//
// This is the v1 matching strategy — see the design doc's Matching Semantics
// and the user's Step 5 decision. It deliberately reproduces FTS5's token-
// exact behavior: "개발" and "개발자" are distinct tokens.
func tokenize(text string) []string { return tokenmatch.Tokenize(text) }

// normalizeText NFC-normalizes and lowercases a string — used for whole-string
// (non-tokenized) comparisons such as location matching.
func normalizeText(s string) string {
	return strings.ToLower(norm.NFC.String(s))
}

// textContains reports whether phrase occurs in text as a contiguous run of
// tokens — the same token-exact, phrase-ordered semantics as an FTS5 quoted
// phrase MATCH. An empty phrase (one that tokenizes to nothing) matches
// nothing.
func textContains(text, phrase string) bool { return tokenmatch.Contains(text, phrase) }

// DealbreakerCandidates returns every matched profile phrase in profile order.
func DealbreakerCandidates(p scraper.Posting, prof profile.Profile) []ai.DealbreakerCandidate {
	text := p.Title + " " + p.Company + " " + p.Description
	var candidates []ai.DealbreakerCandidate
	for _, phrase := range prof.Dealbreakers {
		tokens := tokenize(phrase)
		if len(tokens) == 0 || !textContains(text, phrase) {
			continue
		}
		sum := sha256.Sum256([]byte(strings.Join(tokens, "\x00")))
		candidates = append(candidates, ai.DealbreakerCandidate{
			ID:     hex.EncodeToString(sum[:]),
			Phrase: phrase,
		})
	}
	return candidates
}
