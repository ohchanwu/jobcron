package scraper

import (
	"regexp"
	"strconv"
)

// Experience parsing — storage decision.
//
// Parsed values are computed on demand during scoring; nothing is
// persisted on Posting and no migration is added. Three reasons:
//
//  1. Posting stays a faithful mirror of what the source returned.
//     Parsed-out experience years are a *derived* signal — keeping them
//     out of the schema makes "is this field source-of-truth?" obvious
//     to future readers.
//  2. Scoring is already recomputed whenever the profile changes, so
//     there is no separate consistency story to maintain. If the parser
//     improves, the next score pass picks up the new behavior with no
//     migration step.
//  3. The parser is a handful of regexes against title + description.
//     v1 scrapes ~50-300 postings per pass; the per-score cost is
//     negligible. A schema column would only pay off if we ever wanted
//     to filter at the SQL layer, which is parked for v1.x.
//
// Revisit if (a) we want SQL-level experience filters, or (b) the
// parser grows into something expensive (LLM-based, multi-pass NER).

// reExperienceMinimum matches "N년 이상" / "N+년" / "N년+" / "N년 +" —
// a one-sided lower bound. Returns one digit group.
var reExperienceMinimum = regexp.MustCompile(`(\d+)\s*\+\s*년|(\d+)\s*년\s*(?:이상|\+)`)

// reExperienceMinimumWord matches "최소 N년" — same one-sided lower bound,
// alternative phrasing.
var reExperienceMinimumWord = regexp.MustCompile(`최소\s*(\d+)\s*년`)

// reExperienceRange matches "M-N년" / "M~N년" / "M ~ N 년" — an inclusive
// experience range. Returns two digit groups.
var reExperienceRange = regexp.MustCompile(`(\d+)\s*[-~−]\s*(\d+)\s*년`)

// reExperienceShinipRange matches "신입~N년" / "신입-N년" — the source
// admits 신입 explicitly but caps at N years. Returns one digit group.
var reExperienceShinipRange = regexp.MustCompile(`신입\s*[-~−]\s*(\d+)\s*년`)

// reExperienceCareerN matches "경력 N년" — typically used as an exact
// expectation. Conservative: treat as min = max = N.
var reExperienceCareerN = regexp.MustCompile(`경력\s*(\d+)\s*년`)

// reExperienceSenior / reExperienceJunior catch the unquantified labels
// at the bottom of the priority list so they only fire when no numeric
// signal was found.
var reExperienceSenior = regexp.MustCompile(`(?:^|[^a-zA-Z가-힣])시니어(?:[^a-zA-Z가-힣]|$)`)
var reExperienceJunior = regexp.MustCompile(`(?:^|[^a-zA-Z가-힣])주니어(?:[^a-zA-Z가-힣]|$)`)

// experienceUpperOpen is the synthetic upper bound used for "이상"-style
// matches that don't put a ceiling on years. 99 sits comfortably above
// any realistic IT career length and reads as "no upper bound".
const experienceUpperOpen = 99

// ParseExperienceYears reads title and description and tries to extract a
// concrete (minYears, maxYears) experience requirement.
//
// ok is true only when a clear pattern matches. Patterns are intentionally
// conservative — the design doc's failure mode this addresses is
// "경력무관 but actually 2-5년 경력" (i.e. the source tagged a posting
// 신입-friendly but the title contradicts that). False positives here
// silently penalize legitimate 신입 postings, so the matcher only fires
// on explicit experience-year language.
//
// The title is scanned before the description so a clean title signal
// wins over noise in a long JD body.
func ParseExperienceYears(title, description string) (minYears, maxYears int, ok bool) {
	for _, text := range []string{title, description} {
		if a, b, found := parseExperienceText(text); found {
			return a, b, true
		}
	}
	return 0, 0, false
}

// parseExperienceText runs the regex priority ladder against a single
// chunk of text. The order matters: more-specific patterns win so that
// "경력 5년 이상" is read as (5, ∞), not (5, 5).
func parseExperienceText(text string) (minYears, maxYears int, ok bool) {
	if m := reExperienceMinimum.FindStringSubmatch(text); m != nil {
		// One of the two capture groups holds the number; the other is empty.
		raw := m[1]
		if raw == "" {
			raw = m[2]
		}
		if n, err := strconv.Atoi(raw); err == nil {
			return n, experienceUpperOpen, true
		}
	}
	if m := reExperienceMinimumWord.FindStringSubmatch(text); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n, experienceUpperOpen, true
		}
	}
	if m := reExperienceShinipRange.FindStringSubmatch(text); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return 0, n, true
		}
	}
	if m := reExperienceRange.FindStringSubmatch(text); m != nil {
		a, errA := strconv.Atoi(m[1])
		b, errB := strconv.Atoi(m[2])
		if errA == nil && errB == nil && a <= b {
			return a, b, true
		}
	}
	if m := reExperienceCareerN.FindStringSubmatch(text); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n, n, true
		}
	}
	if reExperienceSenior.MatchString(text) {
		return 5, experienceUpperOpen, true
	}
	if reExperienceJunior.MatchString(text) {
		return 1, 3, true
	}
	return 0, 0, false
}
