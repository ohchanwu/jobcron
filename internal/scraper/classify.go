package scraper

import (
	"regexp"
	"strings"
)

// HasDevKeyword reports whether text carries a clear IT/dev signal. It is
// the shared keyword classifier used by sources that have no structured
// job-family field and must infer "is this a developer role?" from the
// title and description (데모데이's keyword fallback and every Greenhouse
// board). The keyword sets target the broad GitHub-entry-level audience
// defined in scraper-list.md — software engineering plus 보안 / 데이터 /
// DevOps·인프라 / AI·ML / QA / 임베디드 — which the bare "engineer" and
// "엔지니어" tokens already cover (Security Engineer, Cloud Engineer, Data
// Engineer, QA Engineer all match).
//
// Bare "개발" is deliberately NOT a keyword: it substring-matches non-dev
// compounds like 사업개발 (business development), 고객개발 (customer
// development), 연구개발 (R&D), 조직개발 (org development). The explicit dev
// compounds we DO want ("프론트 개발", "백엔드 개발", …) are listed instead.
func HasDevKeyword(text string) bool {
	if devKeywordEN.MatchString(text) {
		return true
	}
	for _, k := range devKeywordKO {
		if strings.Contains(text, k) {
			return true
		}
	}
	return false
}

// devKeywordEN is a case-insensitive regex of IT/dev signals in English.
// Bounded by word boundaries so "engineer" matches "SW Engineer" but
// "interior designer" does not.
var devKeywordEN = regexp.MustCompile(
	`(?i)\b(developer|engineer|programmer|frontend|front-end|backend|back-end|fullstack|full-stack|devops|sre|software|mobile|android|ios|react|vue|angular|svelte|django|spring|flask|fastapi|nodejs|node\.js|typescript|javascript|kotlin|swift|golang|python|machine\s*learning|data\s*(scientist|engineer|analyst))\b`)

// devKeywordKO is the substring-matched Korean token set. Korean words
// don't have word boundaries, so this is straight strings.Contains.
var devKeywordKO = []string{
	"개발자", "엔지니어",
	"프론트엔드", "백엔드", "풀스택",
	"프론트 개발", "백엔드 개발", "풀스택 개발",
	"앱 개발", "웹 개발", "서버 개발", "게임 개발", "AI 개발",
	"임베디드", "딥러닝", "프로그래머",
	"데이터 사이언티스트", "데이터 엔지니어",
	"머신러닝",
	"모바일", "안드로이드",
}
