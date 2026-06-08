package greenhouse

import (
	"html"
	"regexp"
	"strings"

	"github.com/ohchanwu/job-scraper/internal/scraper"
)

// detection is the per-job verdict produced by the tenant's 신입 detector.
type detection struct {
	keep        bool
	newcomer    bool
	minCareer   int
	maxCareer   int
	careerLevel string
}

func classify(strategy DetectStrategy, j ghJob, md map[string]any) detection {
	if strategy == DetectMetadata {
		return classifyMetadata(md)
	}
	return classifyHeuristic(j)
}

// --- 당근 structured-metadata detector -------------------------------------

// classifyMetadata applies 당근's 신입 IT filter. Both conditions must hold:
//
//  1. Engineer == true (yes_no metadata).
//  2. Prior Experience contains "신입" (matches 신입 and 신입/경력, skips 경력).
//
// This is the reliable path 당근's board uniquely supports; see
// API_NOTES.md for why we accept 신입/경력.
func classifyMetadata(md map[string]any) detection {
	if v, ok := md["Engineer"].(bool); !ok || !v {
		return detection{}
	}
	pe, ok := md["Prior Experience"].(string)
	if !ok || !strings.Contains(pe, "신입") {
		return detection{}
	}
	minC, maxC := metadataBounds(pe)
	return detection{
		keep:        true,
		newcomer:    true,
		minCareer:   minC,
		maxCareer:   maxC,
		careerLevel: metadataLabel(pe),
	}
}

// metadataBounds turns 당근's Prior Experience string into the (min, max)
// pair. Pure-신입 maps to (0, 0); the mixed 신입/경력 admits a couple years
// without over-promising the upper bound. The "3" is harmless for the 신입
// target user: scoreCareer awards on (newcomer && years==0) before maxCareer is
// read, and an AI extraction overrides it — so the ceiling only shapes the
// near-miss boundary for experienced users. Validated 2026-06-08; see
// API_NOTES.md.
func metadataBounds(priorExp string) (min, max int) {
	switch strings.TrimSpace(priorExp) {
	case "신입":
		return 0, 0
	case "신입/경력":
		return 0, 3
	default:
		return 0, 0
	}
}

func metadataLabel(priorExp string) string {
	switch strings.TrimSpace(priorExp) {
	case "신입":
		return "신입"
	case "신입/경력":
		return "신입/경력"
	default:
		return strings.TrimSpace(priorExp)
	}
}

// --- Title/description heuristic detector ----------------------------------

// classifyHeuristic decides 신입-eligibility for boards with no structured
// 신입 field (every Greenhouse tenant except 당근). A posting is kept iff it
// is:
//
//  1. Korea-based — these companies are multinational and we only want
//     their Korean roles (the Korea-only boards never trip this).
//  2. A developer role — HasDevKeyword over title+description, covering the
//     broad 신입-dev scope (SWE / 보안 / 데이터 / DevOps·인프라 / AI / QA).
//  3. Newcomer-marked in the TITLE (신입 / 인턴 / junior / …). Titles are
//     clean; matching the title (not the description) avoids false hits on
//     a senior posting that merely mentions mentoring juniors.
//  4. NOT explicitly senior — a 시니어 / Lead / Principal title is rejected
//     even if it carries a newcomer word, and a 2+-year experience floor is
//     rejected unless the role is an internship (an intern JD's "2년 이상"
//     describes research background, not a career floor). This catches the
//     contradictory "Jr. 팀원 (3~6년)" case where a weak junior word rides
//     alongside a real mid-career demand.
//
// Greenhouse carries no structured 신입 flag, so this trades recall for
// precision: an unmarked "Backend Engineer" open to 신입 is missed (the
// Greenhouse 신입-dev count is a documented lower bound), but the briefing is
// not flooded with senior roles. A 2026-06-08 live audit validated keeping the
// title-only check: across all four boards every body-only newcomer hit was
// boilerplate (krafton's "신입일 경우 자기소개서를…" footer; a sendbird "mentoring
// junior engineers" line) — widening to the body would inject ~17 senior
// false positives and zero real 신입 roles. See API_NOTES.md.
func classifyHeuristic(j ghJob) detection {
	location := ""
	if j.Location != nil {
		location = j.Location.Name
	}
	if !isKorea(location) {
		return detection{}
	}
	title := j.Title
	desc := stripHTML(html.UnescapeString(j.Content))
	if hasSeniorMarker(title) {
		return detection{}
	}
	if !hasNewcomerMarker(title) {
		return detection{}
	}
	if !scraper.HasDevKeyword(title + " " + desc) {
		return detection{}
	}
	minY, maxY, ok := scraper.ParseExperienceYears(title, desc)
	if ok && minY >= 2 && !hasInternMarker(title) {
		return detection{}
	}
	minC, maxC := heuristicBounds(ok, minY, maxY)
	return detection{
		keep:        true,
		newcomer:    true,
		minCareer:   minC,
		maxCareer:   maxC,
		careerLevel: heuristicLabel(title),
	}
}

// heuristicBounds picks the career range for a kept posting. A 신입 or
// intern marker dominates, so the floor is 0; an explicit capped range
// like 신입~3년 keeps its ceiling, but a "N년 이상" note on an internship
// (often research experience, not a career floor) collapses to entry.
func heuristicBounds(careerKnown bool, minY, maxY int) (min, max int) {
	if careerKnown && minY == 0 {
		return 0, maxY
	}
	return 0, 0
}

func heuristicLabel(title string) string {
	if hasInternMarker(title) {
		return "인턴"
	}
	return "신입"
}

// --- markers ---------------------------------------------------------------

// newcomerEN is word-boundary anchored so "intern" does not match
// "Internal"/"International" and "associate"/"graduate" stay whole words.
var newcomerEN = regexp.MustCompile(`(?i)\b(intern|interns|internship|junior|jr|entry[\s-]?level|new[\s-]?grad(uate)?|graduate|associate|apprentice|campus)\b`)

// newcomerKO is substring-matched (Korean has no word boundaries). None of
// these is a substring of a common non-newcomer Korean compound.
var newcomerKO = []string{"신입", "인턴", "주니어", "신입사원", "경력무관", "경력 무관", "졸업예정", "수습"}

func hasNewcomerMarker(title string) bool {
	if newcomerEN.MatchString(title) {
		return true
	}
	for _, k := range newcomerKO {
		if strings.Contains(title, k) {
			return true
		}
	}
	return false
}

var internEN = regexp.MustCompile(`(?i)\b(intern|interns|internship)\b`)

func hasInternMarker(title string) bool {
	return internEN.MatchString(title) || strings.Contains(title, "인턴")
}

// seniorEN / seniorKO catch unambiguous seniority so a "Senior … (mentors
// Junior devs)" title is rejected despite the newcomer word. "Manager" is
// deliberately absent — Product/Project Manager is a function, not a level.
var seniorEN = regexp.MustCompile(`(?i)\b(senior|sr|lead|principal|staff|head|director|chief|vp|vice\s*president)\b`)
var seniorKO = []string{"시니어", "수석", "책임", "팀장", "실장", "부장"}

func hasSeniorMarker(title string) bool {
	if seniorEN.MatchString(title) {
		return true
	}
	for _, k := range seniorKO {
		if strings.Contains(title, k) {
			return true
		}
	}
	return false
}

func isKorea(location string) bool {
	return scraper.IsKoreaLocation(location)
}
