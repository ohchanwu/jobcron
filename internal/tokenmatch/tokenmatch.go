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
	_, _, ok := Find(text, phrase)
	return ok
}

// Find returns the byte span of the first contiguous token-sequence match in
// text. The span indexes the original text, even when matching requires NFC
// normalization or case folding.
func Find(text, phrase string) (start, end int, ok bool) {
	needle := Tokenize(phrase)
	if len(needle) == 0 {
		return 0, 0, false
	}
	haystack := tokenizeSpans(text)
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if slices.EqualFunc(haystack[i:i+len(needle)], needle, func(got spanToken, want string) bool {
			return got.text == want
		}) {
			return haystack[i].start, haystack[i+len(needle)-1].end, true
		}
	}
	return 0, 0, false
}

type spanToken struct {
	text       string
	start, end int
}

func tokenizeSpans(text string) []spanToken {
	var iter norm.Iter
	iter.InitString(norm.NFC, text)
	var tokens []spanToken
	var b strings.Builder
	start, end := -1, 0
	flush := func() {
		if start >= 0 {
			tokens = append(tokens, spanToken{
				text:  strings.ToLower(b.String()),
				start: start,
				end:   end,
			})
			b.Reset()
			start = -1
		}
	}
	for !iter.Done() {
		segmentStart := iter.Pos()
		segment := iter.Next()
		segmentEnd := iter.Pos()
		for _, r := range string(segment) {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				if start < 0 {
					start = segmentStart
				}
				end = segmentEnd
				b.WriteRune(r)
			} else {
				flush()
			}
		}
	}
	flush()
	return tokens
}
