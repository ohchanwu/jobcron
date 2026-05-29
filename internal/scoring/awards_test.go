package scoring

import "testing"

// TestNearMissCareerAward locks the round(w × 2/5) derivation the profile UI
// previews and the scorer applies. The two must never drift.
func TestNearMissCareerAward(t *testing.T) {
	cases := []struct{ w, want int }{
		{0, 0},
		{10, 4},  // 20/5
		{22, 9},  // round(8.8)
		{23, 9},  // round(9.2)
		{25, 10}, // the default weight → historical 25→10
		{40, 16},
	}
	for _, c := range cases {
		if got := NearMissCareerAward(c.w); got != c.want {
			t.Errorf("NearMissCareerAward(%d) = %d, want %d", c.w, got, c.want)
		}
	}
}

// TestAmbiguousSalaryAward locks the round(w ÷ 2) derivation.
func TestAmbiguousSalaryAward(t *testing.T) {
	cases := []struct{ w, want int }{
		{0, 0},
		{7, 4},  // round(3.5)
		{9, 5},  // round(4.5)
		{10, 5}, // the default weight → historical 10→5
		{11, 6},
		{20, 10},
	}
	for _, c := range cases {
		if got := AmbiguousSalaryAward(c.w); got != c.want {
			t.Errorf("AmbiguousSalaryAward(%d) = %d, want %d", c.w, got, c.want)
		}
	}
}
