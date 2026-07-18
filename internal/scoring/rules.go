package scoring

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
	"github.com/ohchanwu/jobcron/internal/scraper"
)

// careerUpperOpen mirrors scraper.experienceUpperOpen (99): a cached AI
// extraction with a nil max_career means "no upper bound" and reads as this.
const careerUpperOpen = 99

// aiEducationOrdinal maps the raw AI education enum to a profile education
// ordinal and a short Korean label for the dealbreaker chip. master and
// doctorate both collapse to Graduate (the profile has no finer level), but
// ai_extractions keeps the raw enum for a future split.
var aiEducationOrdinal = map[string]struct {
	level profile.EducationLevel
	label string
}{
	ai.EduNone:       {profile.EducationAny, ""},
	ai.EduHighSchool: {profile.EducationHighSchool, "고졸"},
	ai.EduAssociate:  {profile.EducationAssociate, "초대졸"},
	ai.EduBachelor:   {profile.EducationBachelor, "대졸(4년)"},
	ai.EduMaster:     {profile.EducationGraduate, "석사"},
	ai.EduDoctorate:  {profile.EducationGraduate, "박사"},
}

// Scoring point caps. Stack and location award user-assigned weights bounded
// by their caps; career and salary award the per-profile weights
// (Profile.CareerWeight / Profile.SalaryWeight) — defaulting to 25/10 when
// the user has not customized them, which preserves the historical
// fixed-cap math.
const (
	maxTotal = 100
	stackCap = 50

	// careerNearMissNum/Den derive the near-miss award from the career
	// weight: near = round(weight * Num/Den). The 2/5 ratio reproduces the
	// old 25→10 mapping exactly.
	careerNearMissNum = 2
	careerNearMissDen = 5

	locationCap = 15

	// salaryAmbiguousNum/Den derive the ambiguous-salary award from
	// SalaryWeight (the clear-salary award). 1/2 reproduces the old 10→5.
	salaryAmbiguousNum = 1
	salaryAmbiguousDen = 2
)

// careerExactAward returns the chip Delta for an exact career match: the
// user's CareerWeight, defaulted via EffectiveCareerWeight.
func careerExactAward(prof profile.Profile) int { return prof.EffectiveCareerWeight() }

// careerNearMissAward returns the near-miss award (one bracket off): a
// fraction of the exact award, rounded to keep totals integer.
func careerNearMissAward(prof profile.Profile) int {
	return nearMissCareerAward(careerExactAward(prof))
}

// nearMissCareerAward derives the career near-miss award from a career weight
// w, using the scorer's round-half-up rule (round(w × 2/5)). The profile form's
// preview reaches it through WeightHints, so the UI hint and the scorer share
// this one formula with no duplication.
func nearMissCareerAward(w int) int {
	return (w*careerNearMissNum + careerNearMissDen/2) / careerNearMissDen
}

// salaryClearAward / salaryAmbiguousAward mirror the career awards for
// the salary category.
func salaryClearAward(prof profile.Profile) int { return prof.EffectiveSalaryWeight() }
func salaryAmbiguousAward(prof profile.Profile) int {
	return ambiguousSalaryAward(salaryClearAward(prof))
}

// ambiguousSalaryAward derives the ambiguous-salary award from a salary weight
// w (round(w ÷ 2)), the salary counterpart of nearMissCareerAward.
func ambiguousSalaryAward(w int) int {
	return (w*salaryAmbiguousNum + salaryAmbiguousDen/2) / salaryAmbiguousDen
}

// WeightHints returns the derived award values the profile form previews next
// to the weight inputs: the career near-miss award and the ambiguous-salary
// award for the profile's effective weights. Exposed so the form hint and the
// scorer share one rounding formula rather than duplicating it in the server.
func WeightHints(prof profile.Profile) (careerNearMiss, salaryAmbiguous int) {
	return careerNearMissAward(prof), salaryAmbiguousAward(prof)
}

// Dealbreaker kinds, recorded on DealbreakerHit.
const (
	dealbreakerKeyword   = "keyword"
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
// newcomer / min-max-career fields, with one twist: if the title or
// description contains an explicit experience requirement that
// contradicts those fields, the parsed values are treated as
// authoritative. This is the "경력무관 but actually 2-5년 경력"
// failure mode the design doc flags — sources sometimes tag a posting
// 신입-friendly while the title says otherwise.
//
// When the override fires the function always emits a chip (even at
// Delta=0) so the user can see *why* the career score landed where it
// did — silently dropping the score in that case would look like a bug.
func scoreCareer(p scraper.Posting, prof profile.Profile, ext *ai.Extraction) (LineItem, bool) {
	years := prof.CareerYears
	minC, maxC, newcomer, override := careerFacts(p, ext)

	switch {
	case (newcomer && years == 0) || (years >= minC && years <= maxC):
		return LineItem{Label: careerLabel(years, override, minC, maxC), Delta: careerExactAward(prof), Reason: careerReason}, true
	case years == minC-1 || years == maxC+1:
		return LineItem{Label: careerLabel(years, override, minC, maxC), Delta: careerNearMissAward(prof), Reason: careerNearReason}, true
	case override:
		// Parser contradicted the source category but the user doesn't fit the
		// parsed range either. Surface a 0-delta chip so the missing career
		// bonus is explainable instead of mysterious.
		return LineItem{Label: careerLabel(years, true, minC, maxC), Delta: 0, Reason: careerReason}, true
	default:
		return LineItem{}, false
	}
}

func careerFacts(p scraper.Posting, ext *ai.Extraction) (minC, maxC int, newcomer, override bool) {
	minC, maxC, newcomer = p.MinCareer, p.MaxCareer, p.Newcomer
	if ext != nil {
		// Cache-read (D2): the AI extraction is authoritative for career fit.
		// Use its min/max/newcomer and SKIP the regex override entirely. A nil
		// max_career is an open upper bound. override stays false, so the chip
		// shows the user's level ("신입"/"경력 N년"), not a "본문 …" range.
		minC, newcomer = ext.MinCareer, ext.Newcomer
		if ext.MaxCareer != nil {
			maxC = *ext.MaxCareer
		} else {
			maxC = careerUpperOpen
		}
		// 신입-eligibility guard (scoped to 인턴/internship roles). An intern
		// posting is new-grad-eligible by definition, so when the model misjudges
		// it as experienced (newcomer=false / min_career>0) the inclusive reading
		// wins — wrongly excluding an eligible 신입 costs more than keeping a
		// borderline role (design §"source-vs-AI 신입-eligibility disagreement").
		// Deliberately NOT applied to non-intern roles: there D2 must still be
		// able to correct a source false-positive ("경력무관 but actually 2–5년"),
		// which is the whole reason the AI career read exists.
		if isInternRole(p) {
			newcomer = true
			if minC > 0 {
				minC = 0
			}
		}
	} else if pMin, pMax, parsedOK := scraper.ParseExperienceYears(p.Title, p.Description); parsedOK {
		if pMin != minC || pMax != maxC {
			override = true
			minC, maxC = pMin, pMax
			// Once we are reading off a parsed range, the source's 신입 tag
			// is no longer trustworthy — fall back to the range check alone.
			newcomer = false
		}
	}
	return minC, maxC, newcomer, override
}

// isInternRole reports whether a posting's TITLE marks it an 인턴/internship
// role. Korean "인턴" is matched as a substring — it appears only in
// intern-related words (인턴, 인턴십, 인턴직), never inside an unrelated Korean
// word, so a substring test is safe and catches every form. English is matched
// token-exact ("intern"/"internship") so it never fires on internal /
// international / internet. Title-only by design: an intern role announces
// itself in the title, while a senior posting that merely mentions interns in
// its body must not be reclassified.
func isInternRole(p scraper.Posting) bool {
	if strings.Contains(normalizeText(p.Title), "인턴") {
		return true
	}
	for _, tok := range tokenize(p.Title) {
		if tok == "intern" || tok == "internship" {
			return true
		}
	}
	return false
}

// careerLabel renders the chip text for the career line item. The
// override path shows what the posting actually requires (so the user
// can compare it to the source's category badge), while the normal
// path shows the user's level ("신입" / "경력 3년").
func careerLabel(years int, override bool, minC, maxC int) string {
	if override {
		return "본문 " + formatExperienceRange(minC, maxC)
	}
	if years == 0 {
		return "신입"
	}
	return fmt.Sprintf("경력 %d년", years)
}

// formatExperienceRange renders a parsed (min, max) pair as Korean
// text. An open-ended upper bound (max >= 99) reads as "N년 이상";
// equal min and max reads as a single year; otherwise it's the
// hyphen range. A min of 0 with a positive max means the posting is
// newcomer-OK but bounded — render as "~N년".
func formatExperienceRange(minC, maxC int) string {
	const upperOpen = 99
	switch {
	case maxC >= upperOpen:
		return fmt.Sprintf("%d년 이상", minC)
	case minC == 0:
		return fmt.Sprintf("~%d년", maxC)
	case minC == maxC:
		return fmt.Sprintf("%d년", minC)
	default:
		return fmt.Sprintf("%d-%d년", minC, maxC)
	}
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
		return LineItem{Label: "연봉", Delta: salaryAmbiguousAward(prof), Reason: salaryAmbiguousReason}, true
	case krw >= prof.SalaryFloorKRW:
		return LineItem{Label: "연봉", Delta: salaryClearAward(prof), Reason: salaryReason}, true
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

// educationDealbreaker reports whether the posting demands a higher education
// level than the profile holds. It is inert when the user leaves MaxEducation
// at EducationAny (학력 무관). When a cached AI extraction is present, the
// posting's required level comes from the AI education enum (mapped to an
// ordinal — NOT fed to postingEducationLevel, which keyword-matches Korean
// strings and would silently read the English enum as 학력 무관); otherwise it
// comes from the source's EducationName.
func educationDealbreaker(p scraper.Posting, prof profile.Profile, ext *ai.Extraction) (string, bool) {
	if prof.MaxEducation == profile.EducationAny {
		return "", false
	}
	var level profile.EducationLevel
	var label string
	if ext != nil {
		e := aiEducationOrdinal[ext.EducationEnum] // unknown enum -> zero value (EducationAny, "")
		level, label = e.level, e.label
	} else {
		level, label = postingEducationLevel(p.EducationName), p.EducationName
	}
	if level > prof.MaxEducation {
		return label, true
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
