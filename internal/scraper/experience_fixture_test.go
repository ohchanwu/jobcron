package scraper

import "testing"

// TestParseExperienceYearsRealTitles is the failure-mode fixture: real
// posting titles captured from the local jobs.db on 2026-05-26 where the
// source had tagged the posting newcomer=1 (the misleading "경력무관"
// state the design doc calls out) but the title carries an explicit
// experience requirement. The fixture also includes false-positive
// guards (year references that look like a year-count but aren't) so a
// regex change that broadens matching gets caught.
//
// Each entry's ID is the row's `id` column in the working DB at capture
// time, kept here only as a breadcrumb — these tests don't read the DB.
func TestParseExperienceYearsRealTitles(t *testing.T) {
	t.Parallel()

	type fixtureCase struct {
		postingID int    // DB row id at capture time, for traceability
		title     string // real title pulled from jobs.db
		wantMin   int
		wantMax   int
		wantOK    bool
	}

	// --- Should fire: title contradicts the source's newcomer=1 tag ---
	hits := []fixtureCase{
		{63, "콘텐츠 마케팅 PD (3년 이상)", 3, experienceUpperOpen, true},
		{141, "[시니어] 마케터 (선순환 기획), 서울", 5, experienceUpperOpen, true},
		{148, "[시니어] Product Manager (PM)", 5, experienceUpperOpen, true},
		{156, "[시니어] QA Engineer", 5, experienceUpperOpen, true},
		{212, "[시니어] 마케터 (선순환 기획), 부산", 5, experienceUpperOpen, true},
		{213, "주니어 마케터 (정규직)", 1, 3, true},
		{217, "Frontend Engineer(5년 이상)", 5, experienceUpperOpen, true},
		{239, "B2B 세일즈 매니저 (2~5년차)", 2, 5, true},
		{242, "[뷰메진]로보틱스 엔지니어 Lead (경력 3년 이상)", 3, experienceUpperOpen, true},
	}

	// --- Should NOT fire: contains a number-year token but it's not an
	// experience requirement (calendar year, contract length, growth time). ---
	misses := []fixtureCase{
		{83, "[핏펫] 미디어 커머스 마케터(인턴/1년 계약직)", 0, 0, false},                     // "1년 계약직" = 1-year contract
		{163, "2026년 위촉직(사업수행인력) 근로자 채용 공고", 0, 0, false},                    // "2026년" = calendar year
		{224, "[정규직 마케터 모집] 1년 만에 6배 성장한 MAU 1,400명 후불제 소개팅 앱", 0, 0, false}, // "1년 만에" = within a year
		{253, "한전KDN(주) 울산지사 26년 배전지능화 단순정비 업무보조원 모집공고", 0, 0, false},        // "26년" = abbreviated calendar year
		{259, "[중앙보훈병원] 2026년 상반기 정규직 직원 채용공고", 0, 0, false},                 // "2026년" = calendar year
	}

	for _, c := range hits {
		c := c
		t.Run(c.title, func(t *testing.T) {
			t.Parallel()
			min, max, ok := ParseExperienceYears(c.title, "")
			if !ok {
				t.Fatalf("id=%d parser did NOT fire on %q; want (min=%d, max=%d, ok=true)",
					c.postingID, c.title, c.wantMin, c.wantMax)
			}
			if min != c.wantMin || max != c.wantMax {
				t.Errorf("id=%d %q parsed = (%d, %d), want (%d, %d)",
					c.postingID, c.title, min, max, c.wantMin, c.wantMax)
			}
		})
	}

	for _, c := range misses {
		c := c
		t.Run("no-match/"+c.title, func(t *testing.T) {
			t.Parallel()
			min, max, ok := ParseExperienceYears(c.title, "")
			if ok {
				t.Errorf("id=%d FALSE POSITIVE on %q — parser fired with (min=%d, max=%d), expected no match",
					c.postingID, c.title, min, max)
			}
		})
	}
}
