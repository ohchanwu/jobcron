package tokenmatch

import (
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Tokenize NFC-normalizes text, lowercases it, and splits it into maximal
// letter-or-digit runs.
func Tokenize(text string) []string {
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

// Contains reports whether phrase occurs in text as contiguous ordered tokens.
func Contains(text, phrase string) bool {
	needle := Tokenize(phrase)
	if len(needle) == 0 {
		return false
	}
	haystack := Tokenize(text)
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if slices.Equal(haystack[i:i+len(needle)], needle) {
			return true
		}
	}
	return false
}
