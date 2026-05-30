package scoring

import (
	"testing"

	"github.com/ohchanwu/job-scraper/internal/profile"
)

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
		if got := nearMissCareerAward(c.w); got != c.want {
			t.Errorf("nearMissCareerAward(%d) = %d, want %d", c.w, got, c.want)
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
		if got := ambiguousSalaryAward(c.w); got != c.want {
			t.Errorf("ambiguousSalaryAward(%d) = %d, want %d", c.w, got, c.want)
		}
	}
}

// TestWeightHints covers the exported entry point the profile form uses: it
// returns the derived awards for a profile's effective weights, sharing the
// same formula the per-posting scorer applies. Defaults (25 / 10) → (10 / 5).
func TestWeightHints(t *testing.T) {
	if career, salary := WeightHints(profile.Profile{CareerWeight: 25, SalaryWeight: 10}); career != 10 || salary != 5 {
		t.Errorf("WeightHints(defaults) = (%d, %d), want (10, 5)", career, salary)
	}
	if career, salary := WeightHints(profile.Profile{CareerWeight: 30, SalaryWeight: 21}); career != 12 || salary != 11 {
		t.Errorf("WeightHints(30, 21) = (%d, %d), want (12, 11)", career, salary)
	}
}
