package scoring

import (
	"testing"

	"github.com/ohchanwu/jobcron/internal/profile"
	"github.com/ohchanwu/jobcron/internal/scraper"
)

// basePosting scores 0 in every category: senior career, no stacks, no tags,
// no location. Tests add only the field(s) they exercise.
func basePosting() scraper.Posting {
	return scraper.Posting{MinCareer: 10, MaxCareer: 20}
}

// baseProfile scores 0 in every category: 신입, no stacks/location/salary, no
// keywords.
func baseProfile() profile.Profile {
	return profile.Profile{CareerYears: 0}
}

func lineDelta(r ScoreResult, label string) (int, bool) {
	for _, li := range r.Breakdown {
		if li.Label == label {
			return li.Delta, true
		}
	}
	return 0, false
}

func TestScoreStacksSumsMatchedWeights(t *testing.T) {
	p := basePosting()
	p.StackTags = []string{"React", "TypeScript", "Java"}
	prof := baseProfile()
	prof.Stacks = []profile.StackPref{
		{Name: "react", Weight: 20}, // case-insensitive match
		{Name: "TypeScript", Weight: 10},
		{Name: "Go", Weight: 30}, // not tagged on the posting
	}

	r := scoreNoAI(p, prof)
	if r.Total != 30 {
		t.Errorf("Total = %d, want 30 (20 + 10 matched stacks)", r.Total)
	}
	if d, ok := lineDelta(r, "React"); !ok || d != 20 {
		t.Errorf("React line item = (%d,%v), want (20,true)", d, ok)
	}
	if d, ok := lineDelta(r, "TypeScript"); !ok || d != 10 {
		t.Errorf("TypeScript line item = (%d,%v), want (10,true)", d, ok)
	}
	if _, ok := lineDelta(r, "Go"); ok {
		t.Error("Go line item present, want absent (no matching tag)")
	}
}

func TestScoreStacksCapsAt50(t *testing.T) {
	p := basePosting()
	p.StackTags = []string{"A", "B", "C"}
	prof := baseProfile()
	prof.Stacks = []profile.StackPref{
		{Name: "A", Weight: 40},
		{Name: "B", Weight: 40},
		{Name: "C", Weight: 40},
	}
	if r := scoreNoAI(p, prof); r.Total != 50 {
		t.Errorf("Total = %d, want 50 (stack sum 120 capped at 50)", r.Total)
	}
}

// TestScoreCareerHonorsCustomWeight is the regression guard for the
// 2026-05-28 per-category-weights change: a profile with no CareerWeight
// set still scores the historical 25/10, and bumping CareerWeight to 40
// scales both the exact and near-miss awards by the same ratio.
func TestScoreCareerHonorsCustomWeight(t *testing.T) {
	p := basePosting()
	p.Newcomer, p.MinCareer, p.MaxCareer = true, 0, 0
	prof := baseProfile() // CareerWeight=0 → Effective=25
	if r := scoreNoAI(p, prof); r.Total != 25 {
		t.Errorf("default CareerWeight → Total = %d, want 25", r.Total)
	}

	prof.CareerWeight = 40
	if r := scoreNoAI(p, prof); r.Total != 40 {
		t.Errorf("CareerWeight=40 → Total = %d, want 40 (exact match award)", r.Total)
	}

	// Near-miss: 신입 profile, 1-3년 posting (adjacent). Near-miss
	// award is round(weight * 2/5): 40 * 2/5 = 16.
	p.Newcomer, p.MinCareer, p.MaxCareer = false, 1, 3
	if r := scoreNoAI(p, prof); r.Total != 16 {
		t.Errorf("CareerWeight=40 near-miss → Total = %d, want 16 (40 * 2/5)", r.Total)
	}
}

// TestScoreSalaryHonorsCustomWeight mirrors TestScoreCareerHonorsCustomWeight
// for the salary category. Clear-award is the user's SalaryWeight;
// ambiguous-award is half (round-half-up).
func TestScoreSalaryHonorsCustomWeight(t *testing.T) {
	p := basePosting()
	p.Tags = []scraper.Tag{{Category: "salary", Name: "평균연봉 5,000 이상"}}

	prof := baseProfile() // SalaryWeight=0 → Effective=10
	prof.SalaryFloorKRW = 40_000_000
	if r := scoreNoAI(p, prof); r.Total != 10 {
		t.Errorf("default SalaryWeight → Total = %d, want 10", r.Total)
	}

	prof.SalaryWeight = 30
	if r := scoreNoAI(p, prof); r.Total != 30 {
		t.Errorf("SalaryWeight=30 → Total = %d, want 30 (clear-award)", r.Total)
	}

	// Ambiguous (rate-only) — half of clear, rounded: 30 / 2 = 15.
	p.Tags = []scraper.Tag{{Category: "salary", Name: "연봉상승률 15% 이상"}}
	if r := scoreNoAI(p, prof); r.Total != 15 {
		t.Errorf("SalaryWeight=30 ambiguous → Total = %d, want 15 (30 / 2)", r.Total)
	}
}

func TestScoreCareer(t *testing.T) {
	cases := []struct {
		name     string
		years    int
		newcomer bool
		min, max int
		want     int
	}{
		{"신입 profile, newcomer posting", 0, true, 0, 0, 25},
		{"신입 profile, 1-3년 posting (adjacent)", 0, false, 1, 3, 10},
		{"신입 profile, senior posting", 0, false, 5, 10, 0},
		{"3년 profile, 1-5년 posting (in range)", 3, false, 1, 5, 25},
		{"3년 profile, 5-10년 posting (too far)", 3, false, 5, 10, 0},
		{"2년 profile, 3-6년 posting (adjacent)", 2, false, 3, 6, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := basePosting()
			p.Newcomer, p.MinCareer, p.MaxCareer = tc.newcomer, tc.min, tc.max
			prof := baseProfile()
			prof.CareerYears = tc.years
			if r := scoreNoAI(p, prof); r.Total != tc.want {
				t.Errorf("Total = %d, want %d", r.Total, tc.want)
			}
		})
	}
}

func TestScoreSalary(t *testing.T) {
	salaryTag := func(name string) scraper.Tag {
		return scraper.Tag{Category: "salary", Name: name}
	}
	cases := []struct {
		name  string
		tags  []scraper.Tag
		floor int
		want  int
	}{
		{"no salary tag", []scraper.Tag{{Category: "welfare", Name: "휴가비 지원"}}, 0, 0},
		{"no tags at all", nil, 50_000_000, 0},
		{"salary tag clears the floor", []scraper.Tag{salaryTag("평균연봉 6,000 이상")}, 50_000_000, 10},
		{"salary tag below the floor", []scraper.Tag{salaryTag("평균연봉 3,000 이상")}, 50_000_000, 0},
		{"rate-only tag is ambiguous", []scraper.Tag{salaryTag("연봉상승률 15% 이상")}, 50_000_000, 5},
		{"salary tag with no floor set", []scraper.Tag{salaryTag("평균연봉 4,000 이상")}, 0, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := basePosting()
			p.Tags = tc.tags
			prof := baseProfile()
			prof.SalaryFloorKRW = tc.floor
			if r := scoreNoAI(p, prof); r.Total != tc.want {
				t.Errorf("Total = %d, want %d", r.Total, tc.want)
			}
		})
	}
}

func TestScoreSortsBreakdownByDelta(t *testing.T) {
	// A low stack weight makes the natural append order (stack, career,
	// location, salary) NOT delta-descending, so this catches a missing sort.
	p := scraper.Posting{
		Newcomer:  true,
		StackTags: []string{"React"},
		Location:  "서울 강남구",
		Tags:      []scraper.Tag{{Category: "salary", Name: "평균연봉 6,000 이상"}},
	}
	prof := profile.Profile{
		CareerYears:    0,
		Stacks:         []profile.StackPref{{Name: "React", Weight: 5}},
		Location:       profile.LocationPref{Cities: []string{"서울"}, Weight: 15},
		SalaryFloorKRW: 50_000_000,
	}
	r := scoreNoAI(p, prof)
	if r.Total != 55 {
		t.Errorf("Total = %d, want 55 (5 + 25 + 15 + 10)", r.Total)
	}
	for i := 1; i < len(r.Breakdown); i++ {
		if r.Breakdown[i-1].Delta < r.Breakdown[i].Delta {
			t.Fatalf("breakdown not sorted by delta desc: %+v", r.Breakdown)
		}
	}
	if r.Breakdown[0].Delta != 25 {
		t.Errorf("breakdown[0].Delta = %d, want 25 (career, the largest)", r.Breakdown[0].Delta)
	}
}

func TestScorePerfectPostingScores100(t *testing.T) {
	p := scraper.Posting{
		Newcomer:  true,
		StackTags: []string{"React"},
		Location:  "서울 강남구",
		Tags:      []scraper.Tag{{Category: "salary", Name: "평균연봉 6,000 이상"}},
	}
	prof := profile.Profile{
		CareerYears:    0,
		Stacks:         []profile.StackPref{{Name: "React", Weight: 50}},
		Location:       profile.LocationPref{Cities: []string{"서울"}, Weight: 15},
		SalaryFloorKRW: 50_000_000,
	}
	r := scoreNoAI(p, prof)
	if r.Total != 100 {
		t.Errorf("Total = %d, want 100 (50 + 25 + 15 + 10)", r.Total)
	}
	if len(r.Breakdown) != 4 {
		t.Errorf("breakdown has %d items, want 4", len(r.Breakdown))
	}
}

func TestScoreDealbreakerKeyword(t *testing.T) {
	p := basePosting()
	p.Description = "백엔드 개발자 모집. 병특 지원은 받지 않습니다."
	p.StackTags = []string{"React"} // would otherwise contribute points
	prof := baseProfile()
	prof.Stacks = []profile.StackPref{{Name: "React", Weight: 30}}
	prof.Dealbreakers = []string{"병특"}

	r := scoreNoAI(p, prof)
	if r.Total != -1 {
		t.Errorf("Total = %d, want -1 (dealbreaker keyword hit)", r.Total)
	}
	if r.DealbreakerHit == nil || r.DealbreakerHit.Kind != "keyword" || r.DealbreakerHit.Phrase != "병특" {
		t.Errorf("DealbreakerHit = %+v, want {keyword 병특}", r.DealbreakerHit)
	}
	if r.Breakdown != nil {
		t.Errorf("Breakdown = %+v, want nil on a dealbreaker hit", r.Breakdown)
	}
}

func TestScoreDealbreakerKeywordIsTokenExact(t *testing.T) {
	// "병특" must NOT match the single token "병특혜택없음" — the same
	// token-exact behavior the Step 0 spike validated.
	p := basePosting()
	p.Description = "복지: 병특혜택없음"
	prof := baseProfile()
	prof.Dealbreakers = []string{"병특"}
	if r := scoreNoAI(p, prof); r.Total == -1 {
		t.Error("dealbreaker '병특' wrongly matched the token '병특혜택없음'")
	}
}

func TestScoreEducationDealbreaker(t *testing.T) {
	cases := []struct {
		name    string
		eduName string
		maxEdu  profile.EducationLevel
		wantHit bool
	}{
		{"4년제 required, user has only highschool", "대학교졸업(4년) 이상", profile.EducationHighSchool, true},
		{"4년제 required, user has a bachelor's", "대학교졸업(4년) 이상", profile.EducationBachelor, false},
		{"user 학력무관 — filter off", "대학교졸업(4년) 이상", profile.EducationAny, false},
		{"posting itself is 학력무관", "학력무관", profile.EducationHighSchool, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := basePosting()
			p.EducationName = tc.eduName
			prof := baseProfile()
			prof.MaxEducation = tc.maxEdu
			r := scoreNoAI(p, prof)
			gotHit := r.DealbreakerHit != nil && r.DealbreakerHit.Kind == "education"
			if gotHit != tc.wantHit {
				t.Errorf("education dealbreaker = %v, want %v (hit %+v)", gotHit, tc.wantHit, r.DealbreakerHit)
			}
		})
	}
}

func TestScoreLocation(t *testing.T) {
	remoteTag := scraper.Tag{ID: "w1", Name: "재택근무 가능", Category: "welfare"}
	cases := []struct {
		name     string
		location string
		tags     []scraper.Tag
		cities   []string
		weight   int
		remoteOK bool
		want     int
	}{
		{"city substring match", "서울 강남구 테헤란로", nil, []string{"서울"}, 15, false, 15},
		// User-reported gap (2026-05-28): entering 강남 in cities should
		// match a posting in 강남구. Both directions of the prefix should
		// also work — entering 강남구 should match a posting that lists
		// just 강남, and 서울 should match 서울특별시.
		{"강남 matches 강남구", "서울 강남구 역삼동", nil, []string{"강남"}, 15, false, 15},
		{"강남구 matches 강남구", "서울 강남구 역삼동", nil, []string{"강남구"}, 15, false, 15},
		{"서울 matches 서울특별시", "서울특별시 송파구", nil, []string{"서울"}, 15, false, 15},
		// Adjacent district must NOT match — 강남 should not accept 강북.
		{"강남 does NOT match 강북구", "서울 강북구 미아동", nil, []string{"강남"}, 15, false, 0},
		{"no city match", "부산 해운대구", nil, []string{"서울", "판교"}, 15, false, 0},
		{"weight below the cap", "서울 마포구", nil, []string{"서울"}, 10, false, 10},
		{"weight above the cap clamps", "서울 마포구", nil, []string{"서울"}, 99, false, 15},
		{"remote OK + remote tag", "부산 해운대구", []scraper.Tag{remoteTag}, []string{"서울"}, 15, true, 15},
		{"remote tag but RemoteOK off", "부산", []scraper.Tag{remoteTag}, []string{"서울"}, 15, false, 0},
		{"remote OK but no remote tag", "부산", nil, []string{"서울"}, 15, true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := basePosting()
			p.Location = tc.location
			p.Tags = tc.tags
			prof := baseProfile()
			prof.Location = profile.LocationPref{Cities: tc.cities, Weight: tc.weight, RemoteOK: tc.remoteOK}
			if r := scoreNoAI(p, prof); r.Total != tc.want {
				t.Errorf("Total = %d, want %d", r.Total, tc.want)
			}
		})
	}
}
