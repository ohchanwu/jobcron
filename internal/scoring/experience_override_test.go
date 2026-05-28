package scoring

import (
	"strings"
	"testing"

	"github.com/ohchanwu/job-scraper/internal/profile"
	"github.com/ohchanwu/job-scraper/internal/scraper"
)

// TestScoreCareerOverride exercises the path where the parsed experience
// requirement contradicts the source's claim. The reference scenario from
// the design doc: source tags a posting 신입 (Newcomer=true, MinCareer=0,
// MaxCareer=0) but the title says "2-5년".
func TestScoreCareerOverride(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		title       string
		description string
		newcomer    bool
		min, max    int
		years       int
		wantTotal   int
		// wantLabelPrefix is matched against the career chip's label after
		// scoring. Empty string means no career chip is expected at all.
		wantLabelPrefix string
	}{
		{
			name:     "신입 user, posting tagged 신입 but title says 2-5년 — no points + override chip",
			title:    "백엔드 개발자 (2-5년)",
			newcomer: true, min: 0, max: 0,
			years:     0,
			wantTotal: 0,
			// Override fires; user doesn't match. Emit a 0-delta override chip.
			wantLabelPrefix: "본문 2-5년",
		},
		{
			name:     "3년 user, posting tagged 신입 but title says 2-5년 — override gives +25",
			title:    "백엔드 개발자 (2-5년)",
			newcomer: true, min: 0, max: 0,
			years:           3,
			wantTotal:       25,
			wantLabelPrefix: "본문 2-5년",
		},
		{
			name:     "신입 user, posting tagged 신입 + title 'senior' — silent override penalty",
			title:    "시니어 백엔드 개발자",
			newcomer: true, min: 0, max: 0,
			years:           0,
			wantTotal:       0,
			wantLabelPrefix: "본문 5년 이상",
		},
		{
			name:     "1년 user, posting tagged 신입 + '5년 이상' — near miss across the parsed bound",
			title:    "DevOps Engineer (5년 이상)",
			newcomer: true, min: 0, max: 0,
			years:           4,
			wantTotal:       10, // years == minC-1 → near miss
			wantLabelPrefix: "본문 5년 이상",
		},
		{
			name:     "신입 user, posting tagged 신입 + 신입~3년 — parsed range still admits 신입",
			title:    "주니어 데이터 분석가 신입~3년",
			newcomer: true, min: 0, max: 0,
			years:           0,
			wantTotal:       25, // parsed (0, 3) includes 0 → exact match still
			wantLabelPrefix: "본문 ~3년",
		},
		{
			name:     "no parser signal — source values used unchanged",
			title:    "백엔드 개발자 모집",
			newcomer: true, min: 0, max: 0,
			years:           0,
			wantTotal:       25, // normal newcomer match
			wantLabelPrefix: "신입",
		},
		{
			name:        "parser fires in description, agrees with source — no override",
			title:       "백엔드 개발자 (신입)",
			description: "지원자격: 경력 0년 이상", // pMin=0, but pMax=99 — differs from source max=0
			newcomer:    true, min: 0, max: 0,
			years:     0,
			wantTotal: 25,
			// Parsed range (0, 99) differs from source (0, 0) so override fires.
			// User (0 years) still falls inside [0, 99] → +25 with override label.
			wantLabelPrefix: "본문 0년 이상",
		},
		{
			name: "신입 user vs '경력 3년' (parser → 3,∞) — not matched, not dealbroken, 0-delta override chip",
			// Guard for the 2026-05-27 parser change in commit 08b8d5d that
			// flipped "경력 N년" from (N, N) to (N, ∞). The intent was that
			// a 신입 user no longer near-misses a posting demanding ≥N years.
			// This case asserts the new behavior end-to-end through the
			// scoring engine: parser returns (3, 99); override fires
			// (differs from the source's (0, 0) + Newcomer=true); user with
			// 0 years falls outside [3, 99] and outside the near-miss
			// neighborhood (minC-1=2 ≠ 0, maxC+1=100 ≠ 0); so the result
			// is the override 0-delta chip and Total = 0. No dealbreaker,
			// no misleading near-miss, no silent +25 from the source's
			// stale Newcomer flag.
			title:    "공공기관 시스템 개발자 (경력 3년)",
			newcomer: true, min: 0, max: 0,
			years:           0,
			wantTotal:       0,
			wantLabelPrefix: "본문 3년 이상",
		},
		{
			name: "3년 user vs '경력 3년' — exact match through the parser, +25",
			// Same posting shape as above, different user: a 3-year user
			// hits the lower bound of the parsed range. Confirms the open-
			// upper-bound doesn't accidentally exclude in-range users.
			title:    "공공기관 시스템 개발자 (경력 3년)",
			newcomer: true, min: 0, max: 0,
			years:           3,
			wantTotal:       25,
			wantLabelPrefix: "본문 3년 이상",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			p := scraper.Posting{
				Title:       c.title,
				Description: c.description,
				Newcomer:    c.newcomer,
				MinCareer:   c.min,
				MaxCareer:   c.max,
			}
			prof := profile.Profile{CareerYears: c.years}
			r := Score(p, prof)
			if r.Total != c.wantTotal {
				t.Errorf("Total = %d, want %d (breakdown: %+v)", r.Total, c.wantTotal, r.Breakdown)
			}
			// Find the career-related chip (any item with the careerReason or
			// careerNearReason string in Reason).
			var careerChip *LineItem
			for i := range r.Breakdown {
				if r.Breakdown[i].Reason == careerReason || r.Breakdown[i].Reason == careerNearReason {
					careerChip = &r.Breakdown[i]
					break
				}
			}
			if c.wantLabelPrefix == "" {
				if careerChip != nil {
					t.Errorf("got career chip %+v, want none", careerChip)
				}
				return
			}
			if careerChip == nil {
				t.Fatalf("no career chip in breakdown %+v, want label starting with %q", r.Breakdown, c.wantLabelPrefix)
			}
			if !strings.HasPrefix(careerChip.Label, c.wantLabelPrefix) {
				t.Errorf("career chip label = %q, want prefix %q", careerChip.Label, c.wantLabelPrefix)
			}
		})
	}
}

// TestFormatExperienceRange checks the standalone label formatter.
func TestFormatExperienceRange(t *testing.T) {
	cases := []struct {
		min, max int
		want     string
	}{
		{5, 99, "5년 이상"},
		{3, 7, "3-7년"},
		{0, 3, "~3년"},
		{5, 5, "5년"},
		{1, 3, "1-3년"},
	}
	for _, c := range cases {
		got := formatExperienceRange(c.min, c.max)
		if got != c.want {
			t.Errorf("formatExperienceRange(%d, %d) = %q, want %q", c.min, c.max, got, c.want)
		}
	}
}
