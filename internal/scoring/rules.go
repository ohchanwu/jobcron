package scoring

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ohchanwu/job-scraper/internal/profile"
	"github.com/ohchanwu/job-scraper/internal/scraper"
)

// Scoring point caps. Stack and location award user-assigned weights bounded
// by their caps; career and salary award fixed points.
const (
	maxTotal = 100
	stackCap = 50

	careerExact    = 25
	careerNearMiss = 10

	locationCap = 15

	salaryClear     = 10
	salaryAmbiguous = 5
)

// Dealbreaker kinds, recorded on DealbreakerHit.
const (
	dealbreakerKeyword   = "keyword"
	dealbreakerMustHave  = "must_have_missing"
	dealbreakerEducation = "education"
)

const (
	stackReason           = "기술 스택 일치"
	careerReason          = "경력 조건 일치"
	careerNearReason      = "경력 조건 근접"
	locationReason        = "희망 근무지"
	remoteReason          = "재택근무 가능"
	salaryReason          = "연봉 정보 충족"
	salaryAmbiguousReason = "연봉 정보 있음"
)

// scoreStacks awards each profile stack found in the posting's tags its
// user-assigned weight, with the category total capped at stackCap.
func scoreStacks(p scraper.Posting, prof profile.Profile) []LineItem {
	var items []LineItem
	used := 0
	for _, sp := range prof.Stacks {
		tag, ok := matchStackTag(p.StackTags, sp.Name)
		if !ok {
			continue
		}
		delta := sp.Weight
		if used+delta > stackCap {
			delta = stackCap - used
		}
		if delta <= 0 {
			continue
		}
		used += delta
		items = append(items, LineItem{Label: tag, Delta: delta, Reason: stackReason})
	}
	return items
}

// matchStackTag returns the posting tag that case-insensitively equals name.
func matchStackTag(tags []string, name string) (string, bool) {
	for _, t := range tags {
		if strings.EqualFold(t, name) {
			return t, true
		}
	}
	return "", false
}

// scoreCareer awards career-fit points from the posting's structured
// newcomer / min-max-career fields: careerExact when the profile's experience
// falls inside the posting's required range (or both are 신입), and
// careerNearMiss for an immediately adjacent bracket.
func scoreCareer(p scraper.Posting, prof profile.Profile) (LineItem, bool) {
	years := prof.CareerYears
	switch {
	case (p.Newcomer && years == 0) || (years >= p.MinCareer && years <= p.MaxCareer):
		return LineItem{Label: careerLabel(years), Delta: careerExact, Reason: careerReason}, true
	case years == p.MinCareer-1 || years == p.MaxCareer+1:
		return LineItem{Label: careerLabel(years), Delta: careerNearMiss, Reason: careerNearReason}, true
	default:
		return LineItem{}, false
	}
}

// careerLabel is the breakdown label for a profile's experience level.
func careerLabel(years int) string {
	if years == 0 {
		return "신입"
	}
	return fmt.Sprintf("경력 %d년", years)
}

// scoreLocation awards the profile's location weight (clamped to locationCap)
// when any preferred city appears in the posting's address, or when the
// profile allows remote work and the posting carries a 재택/원격 tag.
func scoreLocation(p scraper.Posting, prof profile.Profile) (LineItem, bool) {
	weight := prof.Location.Weight
	if weight > locationCap {
		weight = locationCap
	}
	if weight <= 0 {
		return LineItem{}, false
	}

	postingLoc := normalizeText(p.Location)
	for _, city := range prof.Location.Cities {
		if city != "" && strings.Contains(postingLoc, normalizeText(city)) {
			return LineItem{Label: city, Delta: weight, Reason: locationReason}, true
		}
	}
	if prof.Location.RemoteOK && hasRemoteTag(p.Tags) {
		return LineItem{Label: "재택", Delta: weight, Reason: remoteReason}, true
	}
	return LineItem{}, false
}

// hasRemoteTag reports whether any posting tag signals remote/재택 work.
func hasRemoteTag(tags []scraper.Tag) bool {
	for _, t := range tags {
		name := normalizeText(t.Name)
		if strings.Contains(name, "재택") || strings.Contains(name, "원격") {
			return true
		}
	}
	return false
}

// scoreSalary awards salary points from the posting's salary-category tags:
// salaryClear when an advertised figure meets the profile's floor,
// salaryAmbiguous when a salary tag exists but carries no comparable figure
// (e.g. a rate-only tag). No salary tag, or a figure below the floor, scores
// nothing.
func scoreSalary(p scraper.Posting, prof profile.Profile) (LineItem, bool) {
	krw, hasSalaryTag := salaryFromTags(p.Tags)
	switch {
	case !hasSalaryTag:
		return LineItem{}, false
	case krw == 0:
		return LineItem{Label: "연봉", Delta: salaryAmbiguous, Reason: salaryAmbiguousReason}, true
	case krw >= prof.SalaryFloorKRW:
		return LineItem{Label: "연봉", Delta: salaryClear, Reason: salaryReason}, true
	default:
		return LineItem{}, false
	}
}

// salaryFromTags returns the highest absolute annual-salary figure (KRW)
// advertised across the posting's salary-category tags, and whether the
// posting has any salary tag at all.
func salaryFromTags(tags []scraper.Tag) (krw int, hasSalaryTag bool) {
	for _, t := range tags {
		if !strings.EqualFold(t.Category, "salary") {
			continue
		}
		hasSalaryTag = true
		if v := parseManwon(t.Name); v > krw {
			krw = v
		}
	}
	return krw, hasSalaryTag
}

// parseManwon extracts the largest absolute KRW figure from a tag name,
// reading bare numbers as 만원 (×10,000). A number immediately followed by
// "%" is a rate, not a salary, and is skipped.
func parseManwon(s string) int {
	best := 0
	runes := []rune(s)
	for i := 0; i < len(runes); {
		if !isASCIIDigit(runes[i]) {
			i++
			continue
		}
		var digits []rune
		j := i
		for j < len(runes) && (isASCIIDigit(runes[j]) || runes[j] == ',') {
			if isASCIIDigit(runes[j]) {
				digits = append(digits, runes[j])
			}
			j++
		}
		if j < len(runes) && runes[j] == '%' {
			i = j + 1 // a percentage — not a salary figure
			continue
		}
		if n, err := strconv.Atoi(string(digits)); err == nil && n*10000 > best {
			best = n * 10000
		}
		i = j
	}
	return best
}

func isASCIIDigit(r rune) bool { return r >= '0' && r <= '9' }

// checkDealbreakers returns the first reason the posting must be excluded, or
// nil. Checks run in order: dealbreaker keywords, missing must-haves, then an
// unmet education requirement.
func checkDealbreakers(p scraper.Posting, prof profile.Profile) *DealbreakerHit {
	text := p.Title + " " + p.Company + " " + p.Description

	for _, phrase := range prof.Dealbreakers {
		if textContains(text, phrase) {
			return &DealbreakerHit{Kind: dealbreakerKeyword, Phrase: phrase}
		}
	}
	for _, phrase := range prof.MustHave {
		if !textContains(text, phrase) {
			return &DealbreakerHit{Kind: dealbreakerMustHave, Phrase: phrase}
		}
	}
	if req, ok := educationDealbreaker(p, prof); ok {
		return &DealbreakerHit{Kind: dealbreakerEducation, Phrase: req}
	}
	return nil
}

// educationDealbreaker reports whether the posting demands a higher education
// level than the profile holds. It is inert when the user leaves MaxEducation
// at EducationAny (학력 무관).
func educationDealbreaker(p scraper.Posting, prof profile.Profile) (string, bool) {
	if prof.MaxEducation == profile.EducationAny {
		return "", false
	}
	if postingEducationLevel(p.EducationName) > prof.MaxEducation {
		return p.EducationName, true
	}
	return "", false
}

// postingEducationLevel maps a 점핏 educationName string to an ordinal level
// by keyword. It is a v1 heuristic — education is a soft, optional filter.
func postingEducationLevel(name string) profile.EducationLevel {
	n := normalizeText(name)
	switch {
	case strings.Contains(n, "박사"), strings.Contains(n, "석사"):
		return profile.EducationGraduate
	case strings.Contains(n, "대학교"), strings.Contains(n, "4년"):
		return profile.EducationBachelor
	case strings.Contains(n, "대학"), strings.Contains(n, "전문대"), strings.Contains(n, "초대졸"):
		return profile.EducationAssociate
	case strings.Contains(n, "고등학교"), strings.Contains(n, "고졸"):
		return profile.EducationHighSchool
	default:
		return profile.EducationAny
	}
}
