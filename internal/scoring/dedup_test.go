package scoring

import (
	"testing"

	"github.com/ohchanwu/job-scraper/internal/scraper"
)

func TestNormalizeCompany(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"토스", "토스"},
		{"토스 주식회사", "토스"},
		{"주식회사 토스", "토스"},
		{"(주)토스", "토스"},
		{"토스(주)", "토스"},
		{"Toss, Inc.", "toss"},
		{"Toss Inc", "toss"},
		{"Toss Co., Ltd.", "toss"},
		{"NAVER Corp", "naver"},
		{"네이버 (NAVER)", "네이버 (naver)"}, // we don't strip parens-with-content
		{"  토스  ", "토스"},
		{"카카오 페이", "카카오 페이"},
		{"", ""},
	}
	for _, c := range cases {
		got := normalizeCompany(c.in)
		if got != c.want {
			t.Errorf("normalizeCompany(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeLocation(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"서울 강남구", "서울 강남구"},
		{"서울  강남구", "서울 강남구"},
		{"  서울 강남구  ", "서울 강남구"},
		{"서울\t강남구", "서울 강남구"},
		{"부산 부산진구 서면로 39", "부산 부산진구 서면로 39"},
		{"", ""},
	}
	for _, c := range cases {
		got := normalizeLocation(c.in)
		if got != c.want {
			t.Errorf("normalizeLocation(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestJaccard(t *testing.T) {
	set := func(toks ...string) map[string]struct{} {
		m := make(map[string]struct{}, len(toks))
		for _, t := range toks {
			m[t] = struct{}{}
		}
		return m
	}
	cases := []struct {
		name string
		a, b map[string]struct{}
		want float64
	}{
		{"identical", set("a", "b", "c"), set("a", "b", "c"), 1.0},
		{"disjoint", set("a", "b"), set("c", "d"), 0.0},
		{"half overlap", set("a", "b"), set("a", "c"), 1.0 / 3.0},
		{"empty left", set(), set("a", "b"), 0.0},
		{"empty right", set("a"), set(), 0.0},
		{"both empty", set(), set(), 0.0},
		{"superset (50%)", set("a", "b"), set("a", "b", "c", "d"), 0.5},
	}
	for _, c := range cases {
		got := jaccard(c.a, c.b)
		if abs(got-c.want) > 1e-9 {
			t.Errorf("%s: jaccard = %v, want %v", c.name, got, c.want)
		}
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// TestAreDuplicates exercises the full match rule with realistic
// posting pairs. The "real-DB" cases use the actual 페이타랩 cross-
// portal data we saw on 2026-05-26 — none of those pairs is a true
// duplicate, so they're all NEGATIVE controls (the matcher must
// reject them). Positive cases are synthetic but follow the shapes
// we expect to see once 데모데이 / 그룹바이 ship.
func TestAreDuplicates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		a, b     scraper.Posting
		wantDupe bool
	}{
		// --- POSITIVE: synthetic clear duplicates ---
		{
			name: "same title, same company, same location, different source — DUPE",
			a: scraper.Posting{
				Source: "jumpit", Title: "Backend Engineer (신입)",
				Company: "토스", Location: "서울 강남구",
			},
			b: scraper.Posting{
				Source: "rallit", Title: "Backend Engineer (신입)",
				Company: "토스", Location: "서울 강남구",
			},
			wantDupe: true,
		},
		{
			name: "company legal-form difference doesn't block match — DUPE",
			a: scraper.Posting{
				Source: "jumpit", Title: "백엔드 엔지니어 (신입)",
				Company: "주식회사 토스", Location: "서울 강남구",
			},
			b: scraper.Posting{
				Source: "rallit", Title: "백엔드 엔지니어 (신입)",
				Company: "토스(주)", Location: "서울 강남구",
			},
			wantDupe: true,
		},
		{
			name: "minor title decoration (parenthesis) — DUPE if Jaccard >= 0.7",
			a: scraper.Posting{
				Source: "jumpit", Title: "Frontend Engineer (React)",
				Company: "당근", Location: "서울 서초구",
			},
			b: scraper.Posting{
				Source: "rallit", Title: "Frontend Engineer React",
				Company: "당근", Location: "서울 서초구",
			},
			wantDupe: true,
		},

		// --- NEGATIVE: real 페이타랩 cross-portal pairs from jobs.db (2026-05-26) ---
		{
			name: "real 페이타랩 jumpit#14 vs rallit#135 — different locations",
			a: scraper.Posting{
				Source: "jumpit", Title: "Windows개발자 (신입)",
				Company: "페이타랩", Location: "부산 부산진구 서면로39, 6층 쿨리지코너부산센터 601호, 602호, 603호",
			},
			b: scraper.Posting{
				Source: "rallit", Title: "Windows 개발자 (C#, WPF)",
				Company: "페이타랩", Location: "서울 강남구 대치동 944-21 스파크플러스 삼성2호점 6층",
			},
			wantDupe: false,
		},
		{
			name: "real 페이타랩 rallit#214 vs rallit#221 — same source — not a DUPE",
			a: scraper.Posting{
				Source: "rallit", Title: "HR Generalist (서울)",
				Company: "페이타랩", Location: "서울 강남구 대치동",
			},
			b: scraper.Posting{
				Source: "rallit", Title: "HR Generalist (부산)",
				Company: "페이타랩", Location: "부산 부산진구",
			},
			wantDupe: false,
		},

		// --- NEGATIVE: synthetic adversarial pairs ---
		{
			name: "same company, same location, totally different role — NOT DUPE",
			a: scraper.Posting{
				Source: "jumpit", Title: "Android Developer",
				Company: "토스", Location: "서울 강남구",
			},
			b: scraper.Posting{
				Source: "rallit", Title: "DevOps Engineer",
				Company: "토스", Location: "서울 강남구",
			},
			wantDupe: false,
		},
		{
			name: "same title + same source — different SourcePostingIDs, NOT a dedup target",
			a: scraper.Posting{
				Source: "jumpit", Title: "Backend Engineer",
				Company: "당근", Location: "서울 서초구",
			},
			b: scraper.Posting{
				Source: "jumpit", Title: "Backend Engineer",
				Company: "당근", Location: "서울 서초구",
			},
			wantDupe: false, // same source can't collide; DB unique constraint covers it
		},
		{
			name: "different company, same title and location — NOT DUPE",
			a: scraper.Posting{
				Source: "jumpit", Title: "Backend Engineer",
				Company: "토스", Location: "서울 강남구",
			},
			b: scraper.Posting{
				Source: "rallit", Title: "Backend Engineer",
				Company: "당근", Location: "서울 강남구",
			},
			wantDupe: false,
		},
		{
			name: "same company, near-identical title, different location — NOT DUPE",
			a: scraper.Posting{
				Source: "jumpit", Title: "Backend Engineer 신입",
				Company: "토스", Location: "서울 강남구",
			},
			b: scraper.Posting{
				Source: "rallit", Title: "Backend Engineer 신입",
				Company: "토스", Location: "판교 분당구",
			},
			wantDupe: false,
		},
		{
			name: "single-token titles with full overlap — DUPE",
			a: scraper.Posting{
				Source: "jumpit", Title: "Frontend",
				Company: "당근", Location: "서울",
			},
			b: scraper.Posting{
				Source: "rallit", Title: "Frontend",
				Company: "당근", Location: "서울",
			},
			wantDupe: true,
		},
		{
			name:     "empty companies — collapse to '' and would match — but empty title set blocks it",
			a:        scraper.Posting{Source: "jumpit", Title: "", Company: "", Location: ""},
			b:        scraper.Posting{Source: "rallit", Title: "", Company: "", Location: ""},
			wantDupe: false, // jaccard returns 0 for empty token sets → below threshold
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := AreDuplicates(c.a, c.b)
			if got != c.wantDupe {
				t.Errorf("AreDuplicates = %v, want %v\n  a: %+v\n  b: %+v",
					got, c.wantDupe, c.a, c.b)
			}
			// Symmetric: swap arguments, expect same answer.
			if rev := AreDuplicates(c.b, c.a); rev != got {
				t.Errorf("AreDuplicates is not symmetric: a->b=%v, b->a=%v", got, rev)
			}
		})
	}
}
