package scoring

import (
	"fmt"
	"strings"
)

// Explain renders a ScoreResult as a compact one-line summary for display,
// e.g. "React +50 · 신입 +25 · 서울 +15", or "병특 ⛔" when the posting was
// excluded by a dealbreaker.
func Explain(r ScoreResult) string {
	if r.DealbreakerHit != nil {
		if r.DealbreakerHit.Kind == dealbreakerMustHave {
			return r.DealbreakerHit.Phrase + " 누락 ⛔"
		}
		return r.DealbreakerHit.Phrase + " ⛔"
	}
	parts := make([]string, 0, len(r.Breakdown))
	for _, li := range r.Breakdown {
		parts = append(parts, fmt.Sprintf("%s +%d", li.Label, li.Delta))
	}
	return strings.Join(parts, " · ")
}
