package scoring

import (
	"sort"

	"github.com/ohchanwu/job-scraper/internal/ai"
	"github.com/ohchanwu/job-scraper/internal/profile"
	"github.com/ohchanwu/job-scraper/internal/scraper"
)

// aiLineLabel is the chip label for the merged Stage-2 AI delta.
const aiLineLabel = "AI 분석"

// LineItem is one contribution to a posting's score. The json tags keep the
// persisted breakdown_json stable; Evidence/Stale are omitempty so non-AI
// line items (and breakdown_json written before v2.0) stay byte-minimal and
// still unmarshal.
type LineItem struct {
	Label    string         `json:"Label"`
	Delta    int            `json:"Delta"`
	Reason   string         `json:"Reason"`
	Evidence []EvidenceItem `json:"evidence,omitempty"` // AI 분석 popover (Stage 2)
	Stale    bool           `json:"stale,omitempty"`    // delta computed vs a prior profile
}

// EvidenceItem is one cited signal behind the AI delta — rendered in the
// Evidence popover (T6). Quote is the model's cited text for a presence
// signal; absence signals carry no quote.
type EvidenceItem struct {
	Kind        string `json:"kind"` // "presence" | "absence"
	Delta       int    `json:"delta"`
	Quote       string `json:"quote,omitempty"`
	MatchedGoal string `json:"matched_goal,omitempty"`
}

// DealbreakerHit records why a posting was excluded from scoring.
type DealbreakerHit struct {
	Kind   string // "keyword" | "education"
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
//
// ext is the cached Stage-1 AI extraction (nil = none): when present,
// scoreCareer and educationDealbreaker prefer the AI career/education facts
// over the regex path. delta is the cached Stage-2 AI score delta (nil = none):
// when present it is merged as one "AI 분석" line item.
//
// A dealbreaker short-circuits FIRST, before any AI delta is merged — a
// dealbroken posting (Total: -1) never carries an AI line item (D3/S4).
func Score(p scraper.Posting, prof profile.Profile, ext *ai.Extraction, delta *ai.Delta) ScoreResult {
	if hit := checkDealbreakers(p, prof, ext); hit != nil {
		return ScoreResult{Total: -1, DealbreakerHit: hit}
	}

	var breakdown []LineItem
	breakdown = append(breakdown, scoreStacks(p, prof)...)
	if li, ok := scoreCareer(p, prof, ext); ok {
		breakdown = append(breakdown, li)
	}
	if li, ok := scoreLocation(p, prof); ok {
		breakdown = append(breakdown, li)
	}
	if li, ok := scoreSalary(p, prof); ok {
		breakdown = append(breakdown, li)
	}
	// Append the AI line only when at least one signal survived the citation
	// gate. A delta with no surviving items contributes nothing and must show no
	// chip — the calm surface stays silent rather than rendering an empty
	// "AI 분석" chip (design §c).
	if delta != nil && len(delta.Items) > 0 {
		breakdown = append(breakdown, aiLineItem(*delta))
	}

	sort.SliceStable(breakdown, func(i, j int) bool {
		return breakdown[i].Delta > breakdown[j].Delta
	})

	total := 0
	for _, li := range breakdown {
		total += li.Delta
	}
	// Clamp to [0,100]: a negative net AI delta must floor at 0, not collide
	// with the -1 dealbreaker sentinel (D3).
	if total > maxTotal {
		total = maxTotal
	}
	if total < 0 {
		total = 0
	}
	return ScoreResult{Total: total, Breakdown: breakdown}
}

// aiLineItem folds an AI Delta into one "AI 분석" line item: the net delta as
// the chip value, the surviving signals as Evidence for the popover, and the
// stale flag passed through.
func aiLineItem(d ai.Delta) LineItem {
	evidence := make([]EvidenceItem, 0, len(d.Items))
	for _, it := range d.Items {
		evidence = append(evidence, EvidenceItem{
			Kind:        it.Kind,
			Delta:       it.Delta,
			Quote:       it.Evidence,
			MatchedGoal: it.MatchedGoal,
		})
	}
	li := LineItem{Label: aiLineLabel, Delta: d.NetDelta, Reason: aiLineLabel, Stale: d.Stale}
	if len(evidence) > 0 {
		li.Evidence = evidence
	}
	return li
}
