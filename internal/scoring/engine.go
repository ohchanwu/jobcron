package scoring

import (
	"sort"

	"github.com/ohchanwu/job-scraper/internal/profile"
	"github.com/ohchanwu/job-scraper/internal/scraper"
)

// LineItem is one contribution to a posting's score.
type LineItem struct {
	Label  string // e.g. a stack name, a city, "신입"
	Delta  int    // signed point contribution
	Reason string // short Korean explanation
}

// DealbreakerHit records why a posting was excluded from scoring.
type DealbreakerHit struct {
	Kind   string // "keyword" | "must_have_missing" | "education"
	Phrase string // the offending or missing phrase
}

// ScoreResult is the outcome of scoring one posting against a profile.
type ScoreResult struct {
	Total          int        // 0..100, or -1 on a dealbreaker hit
	Breakdown      []LineItem // contributions, ordered by Delta descending
	DealbreakerHit *DealbreakerHit
}

// Score evaluates a posting against a user profile, producing a 0..100 total
// with an explained breakdown.
func Score(p scraper.Posting, prof profile.Profile) ScoreResult {
	if hit := checkDealbreakers(p, prof); hit != nil {
		return ScoreResult{Total: -1, DealbreakerHit: hit}
	}

	var breakdown []LineItem
	breakdown = append(breakdown, scoreStacks(p, prof)...)
	if li, ok := scoreCareer(p, prof); ok {
		breakdown = append(breakdown, li)
	}
	if li, ok := scoreLocation(p, prof); ok {
		breakdown = append(breakdown, li)
	}
	if li, ok := scoreSalary(p, prof); ok {
		breakdown = append(breakdown, li)
	}

	sort.SliceStable(breakdown, func(i, j int) bool {
		return breakdown[i].Delta > breakdown[j].Delta
	})

	total := 0
	for _, li := range breakdown {
		total += li.Delta
	}
	if total > maxTotal {
		total = maxTotal
	}
	return ScoreResult{Total: total, Breakdown: breakdown}
}
