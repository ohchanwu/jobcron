package ai

import (
	"math"
	"testing"
)

func TestRatingBoundedItemsAndSymmetricNet(t *testing.T) {
	for _, tc := range []struct {
		name   string
		deltas []int
		want   int
	}{
		{"positive", []int{math.MaxInt, 30}, 40},
		{"negative", []int{math.MinInt, -30}, -40},
		{"cancel", []int{math.MaxInt, math.MinInt, 25}, 25},
		{"reverse cancel", []int{25, math.MinInt, math.MaxInt}, 25},
	} {
		t.Run(tc.name, func(t *testing.T) {
			quotes := []string{"첫번째 개발 업무", "두번째 개발 업무", "세번째 개발 업무"}
			raw := make([]RawDeltaItem, 0, len(tc.deltas))
			for i, n := range tc.deltas {
				raw = append(raw, RawDeltaItem{Signal: "업무", MatchedGoal: "목표", Kind: KindPresence, Delta: n, Quote: quotes[i]})
			}
			d := GateDelta(raw, "첫번째 개발 업무. 두번째 개발 업무. 세번째 개발 업무.", "")
			if d.NetDelta != tc.want || len(d.Items) != len(raw) {
				t.Fatalf("bounded result=%+v want net %d", d, tc.want)
			}
			for _, it := range d.Items {
				if it.Delta < -30 || it.Delta > 30 {
					t.Fatalf("unbounded item: %+v", it)
				}
			}
		})
	}
}
