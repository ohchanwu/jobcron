package scoring

import (
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// tokenize splits text the way SQLite FTS5's unicode61 tokenizer does: it
// NFC-normalizes the text, breaks it into maximal runs of letters and digits
// (every other character is a separator), and lowercases each token.
//
// This is the v1 matching strategy — see the design doc's Matching Semantics
// and the user's Step 5 decision. It deliberately reproduces FTS5's token-
// exact behavior: "개발" and "개발자" are distinct tokens.
func tokenize(text string) []string {
	text = norm.NFC.String(text)
	var tokens []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			tokens = append(tokens, strings.ToLower(b.String()))
			b.Reset()
		}
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return tokens
}

// normalizeText NFC-normalizes and lowercases a string — used for whole-string
// (non-tokenized) comparisons such as location matching.
func normalizeText(s string) string {
	return strings.ToLower(norm.NFC.String(s))
}

// textContains reports whether phrase occurs in text as a contiguous run of
// tokens — the same token-exact, phrase-ordered semantics as an FTS5 quoted
// phrase MATCH. An empty phrase (one that tokenizes to nothing) matches
// nothing.
func textContains(text, phrase string) bool {
	needle := tokenize(phrase)
	if len(needle) == 0 {
		return false
	}
	haystack := tokenize(text)
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if slices.Equal(haystack[i:i+len(needle)], needle) {
			return true
		}
	}
	return false
}
