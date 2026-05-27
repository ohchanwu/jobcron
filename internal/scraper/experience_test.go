package scraper

import "testing"

func TestParseExperienceYears(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		title       string
		description string
		wantMin     int
		wantMax     int
		wantOK      bool
	}{
		// --- Clear positive signals from the design-doc failure mode ---
		{
			name:    "range with hyphen in title",
			title:   "백엔드 개발자 (2-5년)",
			wantMin: 2, wantMax: 5, wantOK: true,
		},
		{
			name:    "range with tilde in title",
			title:   "프론트엔드 엔지니어 3~7년",
			wantMin: 3, wantMax: 7, wantOK: true,
		},
		{
			name:    "minimum with 이상",
			title:   "DevOps 엔지니어 (5년 이상)",
			wantMin: 5, wantMax: experienceUpperOpen, wantOK: true,
		},
		{
			name:    "minimum with 최소",
			title:   "보안 엔지니어 최소 3년 경력",
			wantMin: 3, wantMax: experienceUpperOpen, wantOK: true,
		},
		{
			// "경력 N년" reads as a minimum on real postings ("at least
			// N years"), not an exact value. A user with 6 years still
			// fits, and a 신입 user is still correctly out.
			name:    "career N (open-bounded minimum)",
			title:   "AI 엔지니어 경력 5년",
			wantMin: 5, wantMax: experienceUpperOpen, wantOK: true,
		},
		{
			name:    "+년 suffix",
			title:   "Senior Engineer 5+년",
			wantMin: 5, wantMax: experienceUpperOpen, wantOK: true,
		},
		{
			name:    "이상 wins over 경력 N — '경력 5년 이상' → (5, ∞)",
			title:   "백엔드 개발자 경력 5년 이상",
			wantMin: 5, wantMax: experienceUpperOpen, wantOK: true,
		},

		// --- 신입~N pattern: explicit 신입 friendly but bounded ---
		{
			name:    "shinip range",
			title:   "데이터 엔지니어 신입~3년",
			wantMin: 0, wantMax: 3, wantOK: true,
		},

		// --- Senior / Junior word fallbacks ---
		{
			name:    "senior keyword alone",
			title:   "시니어 백엔드 개발자",
			wantMin: 5, wantMax: experienceUpperOpen, wantOK: true,
		},
		{
			name:    "junior keyword alone",
			title:   "주니어 프론트엔드 개발자",
			wantMin: 1, wantMax: 3, wantOK: true,
		},
		{
			name:    "senior with surrounding text",
			title:   "[채용] 시니어 풀스택 모집",
			wantMin: 5, wantMax: experienceUpperOpen, wantOK: true,
		},

		// --- Title wins over description ---
		{
			name:        "title signal wins over description noise",
			title:       "백엔드 엔지니어 3-5년",
			description: "신입 환영 — 경력 1년 이상도 좋아요",
			wantMin:     3, wantMax: 5, wantOK: true,
		},
		{
			name:        "fallback to description when title is plain",
			title:       "백엔드 엔지니어",
			description: "지원자격: 관련 경력 3년 이상",
			wantMin:     3, wantMax: experienceUpperOpen, wantOK: true,
		},

		// --- Negative cases: no clear signal, do not fire ---
		{
			name:    "no experience signal — plain title",
			title:   "백엔드 개발자 모집",
			wantOK:  false,
			wantMin: 0, wantMax: 0,
		},
		{
			name:   "shinip-only title",
			title:  "신입 백엔드 개발자",
			wantOK: false,
		},
		{
			name:   "newcomer welcome — no number",
			title:  "신입 환영합니다",
			wantOK: false,
		},
		{
			name:        "empty inputs",
			title:       "",
			description: "",
			wantOK:      false,
		},
		{
			name:   "junior as part of a longer word should not match",
			title:  "프로주니어가 아닌 신입 환영", // 주니어 embedded in 프로주니어가
			wantOK: false,
		},
		{
			name:   "senior in english word should not match",
			title:  "Senior level position",
			wantOK: false,
		},

		// --- Conservative: ambiguous wording does not fire ---
		{
			name:   "이내 (within N years) is not 이상 — should not match minimum pattern",
			title:  "3년 이내 졸업자 우대",
			wantOK: false,
		},
		{
			name:   "최소 without 년 — irrelevant '최소 1명' style",
			title:  "최소 1명 채용",
			wantOK: false,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			gotMin, gotMax, gotOK := ParseExperienceYears(c.title, c.description)
			if gotOK != c.wantOK {
				t.Fatalf("ok=%v want %v (min=%d max=%d)", gotOK, c.wantOK, gotMin, gotMax)
			}
			if gotMin != c.wantMin || gotMax != c.wantMax {
				t.Fatalf("range=(%d,%d) want (%d,%d)", gotMin, gotMax, c.wantMin, c.wantMax)
			}
		})
	}
}
