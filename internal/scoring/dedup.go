package scoring

import (
	"regexp"
	"strings"

	"github.com/ohchanwu/jobcron/internal/scraper"
)

// Cross-portal deduplication — design notes.
//
// What we're doing here. Same job posting can show up on multiple
// source portals (the same role on 점핏 AND 랠릿, or on 잡알리오 AND
// 데모데이 once those scrapers ship). The user sees the same job
// twice in their briefing, which breaks the calm-list principle. The
// match rule below identifies pairs that are almost-certainly the
// same posting so the storage layer can mark the second-seen one
// duplicate_of = the canonical.
//
// UX decision: hide-and-badge, not visual-group.
//
//	Two options were on the table. (A) Hide non-canonical rows from
//	the list and stamp the canonical with an "also on 랠릿, 잡알리오"
//	badge. (B) Show every copy but visually group them.
//
//	Option A wins for v1. The product thesis (P6: calmness) treats
//	every extra row as cost. A duplicate adds zero new information
//	the user can act on — they'll click whichever URL they prefer,
//	and the source is the only differentiator. Option B doubles the
//	row count to preserve a signal the user doesn't read.
//
//	Trade-off accepted: the canonical's "first seen" version may be
//	an older snapshot of the description. We re-fetch detail and
//	bump last_seen_at across all sources during scrape, so the
//	canonical's data still freshens; only the picked URL is sticky.
//
// Forward-looking note. The current local DB (291 postings on
// 2026-05-26) has exactly one company posting on 2+ portals
// (페이타랩 on 점핏 and 랠릿) and that company cross-posts *different*
// roles per portal — so the matcher returns zero positives against
// current data, which is correct. Dedup pays off once 데모데이 and
// 그룹바이 ship (both heavily overlap 랠릿 on the startup-dev cohort)
// and once any direct-company-page scrapers add a second route to
// the same posting.
//
// Why this match rule is conservative. False positives here are
// permanent: a wrongly-merged pair hides one of the postings from
// the user forever. False negatives are recoverable: a missed pair
// shows two rows instead of one. Tuned for low recall over high
// precision — see ApproxDuplicateThreshold below.

// ApproxDuplicateThreshold is the Jaccard-overlap floor on title
// tokens for a pair to be considered duplicates of each other.
// Conservative: 0.7 means roughly 70% of distinct tokens must
// overlap — enough to catch "Windows 개발자" vs "Windows 개발자 (C#)"
// but not "Android Developer" vs "Windows Developer".
const ApproxDuplicateThreshold = 0.7

// companyStripPatterns are the legal-form suffixes/prefixes commonly
// attached to Korean (and Korean-context English) company names. The
// matcher strips them before equality so 토스와 토스 주식회사 와
// (주)토스 all collapse to "토스".
var companyStripPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\(주\)`),
	regexp.MustCompile(`주식회사`),
	regexp.MustCompile(`(?i)\binc\.?\b`),
	regexp.MustCompile(`(?i)\bco\.?\b`),
	regexp.MustCompile(`(?i)\bltd\.?\b`),
	regexp.MustCompile(`(?i)\bcorp\.?\b`),
	regexp.MustCompile(`(?i)\bllc\.?\b`),
}

// normalizeCompany strips legal-form noise and lowercases, leaving
// the bare brand. It's deliberately greedy on punctuation/whitespace
// so "Toss, Inc." and "토스(주)" both end up "toss" and "토스".
func normalizeCompany(s string) string {
	s = strings.ToLower(s)
	for _, re := range companyStripPatterns {
		s = re.ReplaceAllString(s, " ")
	}
	// Collapse all remaining whitespace and punctuation runs.
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '.' || r == ',' || r == '-' || r == '_' {
			if !prevSpace {
				b.WriteRune(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	return strings.TrimSpace(b.String())
}

// normalizeLocation lowercases and collapses whitespace so trivial
// formatting differences ("서울 강남구" vs "서울  강남구") don't break
// equality. Address-level normalization (e.g. dropping "지점", suite
// numbers) is intentionally NOT done — different addresses for the
// same company mean different roles, which the user wants to keep
// separate.
func normalizeLocation(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.Join(strings.Fields(s), " ")
}

// titleTokensSet returns the unique tokens of title — duplicated
// runs collapse so the Jaccard ratio reflects vocabulary overlap,
// not word frequency.
func titleTokensSet(title string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, t := range tokenize(title) {
		set[t] = struct{}{}
	}
	return set
}

// jaccard returns |a ∩ b| / |a ∪ b| for two token sets. Returns 0
// when either side is empty.
func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for tok := range a {
		if _, ok := b[tok]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// AreDuplicates reports whether two postings are almost-certainly the
// same job (the canonical-and-duplicate relationship is symmetric,
// so call order doesn't matter). The rule:
//
//   - Different sources. Two postings from the same source already
//     can't collide (UNIQUE (source, source_posting_id) in the DB).
//   - Normalized company names equal.
//   - Normalized locations equal.
//   - Title-token Jaccard at or above ApproxDuplicateThreshold.
//
// All four conditions must hold. Locations not matching is a strong
// signal of "different role at different office," which the user
// wants kept separate even if the title coincides.
func AreDuplicates(a, b scraper.Posting) bool {
	if a.Source == b.Source {
		return false
	}
	if normalizeCompany(a.Company) != normalizeCompany(b.Company) {
		return false
	}
	if normalizeLocation(a.Location) != normalizeLocation(b.Location) {
		return false
	}
	return jaccard(titleTokensSet(a.Title), titleTokensSet(b.Title)) >= ApproxDuplicateThreshold
}
