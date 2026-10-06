package scoring

import (
	"strings"
	"testing"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
	"github.com/ohchanwu/jobcron/internal/scraper"
)

func TestRatingClippedEvidenceExplainsAppliedNet(t *testing.T) {
	for _, sign := range []int{1, -1} {
		d := ai.GateDelta([]ai.RawDeltaItem{
			{Signal: "서버", MatchedGoal: "업무", Kind: ai.KindPresence, Quote: "서버 개발자를 찾습니다", Delta: sign * 30},
			{Signal: "협업", MatchedGoal: "문화", Kind: ai.KindPresence, Quote: "함께 협업하는 팀입니다", Delta: sign * 30},
		}, "서버 개발자를 찾습니다. 함께 협업하는 팀입니다.", "")
		r := Score(scraper.Posting{}, profile.Profile{}, nil, &d, nil)
		var line *LineItem
		for i := range r.Breakdown {
			if r.Breakdown[i].Label == aiLineLabel {
				line = &r.Breakdown[i]
			}
		}
		if line == nil || line.Delta != sign*40 || len(line.Evidence) != 2 || !strings.Contains(line.Reason, "항목 합계") || !strings.Contains(line.Reason, "40점") {
			t.Fatalf("clipping not explained consistently: %+v", line)
		}
	}
}
