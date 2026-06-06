package greeting

import (
	"strings"

	"github.com/ohchanwu/job-scraper/internal/scraper"
)

// careerType enum values from jobPositionCareer.careerType.
const (
	careerNewComer    = "NEW_COMER"   // 신입
	careerNotMatter   = "NOT_MATTER"  // 경력무관 (open to 신입 + 경력)
	careerExperienced = "EXPERIENCED" // 경력 — excluded
)

// employmentType enum values from jobPositionEmployment.employmentType.
const (
	empIntern   = "INTERN_WORKER"              // 채용전환형 인턴
	empFullTime = "FULL_TIME_WORKER"           // 정규직
	empMilitary = "MILITARY_SERVICE_EXCEPTION" // 병역특례 (산업기능요원/전문연구요원)
)

// pickQualifying returns the first position of an opening that is a 신입 dev
// role we want, or ok=false if none qualifies. An opening can bundle several
// positions; one qualifying position is enough to surface the posting, and
// its fields drive the Posting.
func pickQualifying(o opening) (position, bool) {
	if isNonJobPosting(o.Title) {
		return position{}, false
	}
	for _, p := range o.OpeningJobPosition.OpeningJobPositions {
		if qualifies(o.Title, p) {
			return p, true
		}
	}
	return position{}, false
}

// qualifies applies the three gates: structured 신입-eligibility, Korea
// location, and dev-role.
func qualifies(title string, p position) bool {
	c := p.JobPositionCareer
	if c == nil {
		return false
	}
	if c.CareerType != careerNewComer && c.CareerType != careerNotMatter {
		return false // EXPERIENCED or unknown
	}
	if !scraper.IsKoreaLocation(placeOf(p)) {
		return false
	}
	return isDevRole(title, p)
}

// isDevRole decides whether a position is a developer role, mirroring
// 데모데이's keepsITSWE shape: the structured occupation family is the primary
// signal, with the shared dev-keyword classifier as the fallback for the
// many tenant-specific occupation labels (테크, 기술, Tech/Product, DS, …).
//
// The structured allow check matters because a clean job like "Flutter개발"
// (occupation 개발) carries no token HasDevKeyword recognizes — the
// occupation family is what flags it. HasDevKeyword (which excludes bare 개발
// and bare AI) then keeps precision on the fallback path: it correctly drops
// a "사업개발팀 영업" sales role and an "AI 전략 인턴" strategy role.
func isDevRole(title string, p position) bool {
	occ, job := occJob(p)
	// Deny first: a clearly non-dev occupation family wins even if its label
	// contains a dev substring (e.g. "사업개발/PM" contains 개발 but is biz-dev)
	// and blocks the keyword fallback from over-matching a stray token like
	// "모바일" in "모바일 게임 광고 영상 기획자".
	if nonDevOccupation(occ) {
		return false
	}
	if devOccupation(occ) {
		return true
	}
	return scraper.HasDevKeyword(title + " " + job + " " + occ)
}

// nonDevOccupation reports whether the structured occupation family is a
// clearly non-developer one. "Product" / "Tech/Product" are deliberately
// absent — some tenants use them as a catch-all that includes backend/AI.
func nonDevOccupation(occ string) bool {
	for _, k := range []string{
		"사업개발", "영상", "마케팅", "광고", "영업", "기획", "디자인", "인사",
		"재무", "회계", "퍼블리싱", "콘텐츠", "브랜드", "홍보", "세일즈", "총무",
		"구매", "물류", "고객", "경영",
	} {
		if strings.Contains(occ, k) {
			return true
		}
	}
	l := strings.ToLower(occ)
	for _, k := range []string{"sales", "marketing", "design", "finance", "accounting", "content", "brand", "business"} {
		if strings.Contains(l, k) {
			return true
		}
	}
	return false
}

// devOccupation reports whether the structured occupation family is clearly
// a developer family. Korean 개발/데이터 and the unambiguous English tech
// families; everything else defers to the keyword fallback.
func devOccupation(occ string) bool {
	if strings.Contains(occ, "개발") || strings.Contains(occ, "데이터") {
		return true
	}
	l := strings.ToLower(occ)
	for _, k := range []string{"data", "engineering", "software", "devops"} {
		if strings.Contains(l, k) {
			return true
		}
	}
	return false
}

// isNonJobPosting screens out board entries that aren't real openings:
// referral programs and standing talent-pool registrations.
func isNonJobPosting(title string) bool {
	for _, k := range []string{"추천제도", "지인 추천", "지인추천", "인재등록", "인재 등록", "인재풀", "인재 풀"} {
		if strings.Contains(title, k) {
			return true
		}
	}
	return false
}

func occJob(p position) (occ, job string) {
	if p.WorkspaceOccupation != nil {
		occ = strings.TrimSpace(p.WorkspaceOccupation.Occupation)
	}
	if p.WorkspaceJob != nil {
		job = strings.TrimSpace(p.WorkspaceJob.Job)
	}
	return occ, job
}
