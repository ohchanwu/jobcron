package greenhouse

// Tenant describes one Greenhouse board this scraper pulls from. Each
// registered Scraper wraps exactly one Tenant, so Source(), the source
// badge, and the profile toggle stay per-company.
type Tenant struct {
	// Source is the stable identifier persisted on every Posting and
	// matched against the user's DisabledSources (e.g. "daangn", "krafton").
	Source string
	// Token is the Greenhouse board slug in the API path
	// (/v1/boards/{Token}/jobs).
	Token string
	// Company is the display company name. Used as-is for heuristic
	// tenants; 당근 overrides it per-posting from its "Corporate" metadata.
	Company string
	// SiteURL is the public careers host. Used for the site robots.txt
	// check and, when Link == LinkSite, to build the click-through URL.
	SiteURL string
	// Link selects how the user-facing posting URL is built.
	Link LinkStrategy
	// Detect selects how 신입-eligibility is decided for this board.
	Detect DetectStrategy
}

// LinkStrategy controls how a posting's click-through URL is built.
type LinkStrategy int

const (
	// LinkAbsolute uses Greenhouse's own absolute_url. Correct for boards
	// whose absolute_url points at the hosted job page
	// (job-boards.greenhouse.io/{token}/jobs/{id}) — krafton, moloco.
	LinkAbsolute LinkStrategy = iota
	// LinkSite builds {SiteURL}/jobs/{id}/ and ignores absolute_url. For
	// 당근, whose absolute_url is a dead about.daangn.com marketing link.
	LinkSite
	// LinkBoard builds the canonical hosted board URL
	// job-boards.greenhouse.io/{token}/jobs/{id}. For boards whose
	// absolute_url points at a custom careers page that does not deep-link
	// to the specific posting.
	LinkBoard
)

// DetectStrategy controls how 신입-eligibility is decided.
type DetectStrategy int

const (
	// DetectHeuristic uses a title/description 신입-dev heuristic — the
	// general Greenhouse case, where the board carries no structured 신입
	// field. See classify.go.
	DetectHeuristic DetectStrategy = iota
	// DetectMetadata uses 당근's structured Greenhouse metadata (Engineer +
	// Prior Experience). Reliable but specific to 당근's board config.
	DetectMetadata
)

// link builds the click-through URL for a posting on this tenant's board.
func (t Tenant) link(id, absoluteURL string) string {
	switch t.Link {
	case LinkSite:
		return trimSlash(t.SiteURL) + "/jobs/" + id + "/"
	case LinkBoard:
		return "https://job-boards.greenhouse.io/" + t.Token + "/jobs/" + id
	default: // LinkAbsolute
		return absoluteURL
	}
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

// --- Curated tenant list ---------------------------------------------------
//
// The tokens below were verified live on 2026-06-06 (click-through checked
// in a browser). 토스 is intentionally absent — it runs a custom host
// (api-public.toss.im), not the standard boards-api.greenhouse.io, so it
// needs separate handling. Senior-only Korean boards (coupang /
// coupanginternal / seoulrobotics) are excluded — ~0 신입 dev.

// Daangn returns the 당근 careers scraper. It keeps Source "daangn" and the
// structured-metadata detector that 당근's board uniquely supports, so its
// badge, profile toggle, and stored postings are unchanged by the move
// from the standalone daangn package into this shared adapter.
func Daangn() *Scraper {
	return New(Tenant{
		Source:  "daangn",
		Token:   "daangn",
		Company: "당근",
		SiteURL: "https://team.daangn.com",
		Link:    LinkSite,
		Detect:  DetectMetadata,
	})
}

// Krafton returns the 크래프톤 careers scraper (AI Research interns + 신입
// tracks). absolute_url is the hosted Greenhouse board, which deep-links
// correctly.
func Krafton() *Scraper {
	return New(Tenant{
		Source:  "krafton",
		Token:   "krafton",
		Company: "크래프톤",
		SiteURL: "https://careers.krafton.com",
		Link:    LinkAbsolute,
		Detect:  DetectHeuristic,
	})
}

// Moloco returns the 몰로코 careers scraper (Seoul ML/SWE interns).
func Moloco() *Scraper {
	return New(Tenant{
		Source:  "moloco",
		Token:   "moloco",
		Company: "몰로코",
		SiteURL: "https://www.moloco.com",
		Link:    LinkAbsolute,
		Detect:  DetectHeuristic,
	})
}

// Sendbird returns the 센드버드 careers scraper (Seoul AI engineer interns).
// Its absolute_url is a custom careers page that does not deep-link to the
// posting, so it uses the canonical hosted board URL instead.
func Sendbird() *Scraper {
	return New(Tenant{
		Source:  "sendbird",
		Token:   "sendbird",
		Company: "센드버드",
		SiteURL: "https://sendbird.com",
		Link:    LinkBoard,
		Detect:  DetectHeuristic,
	})
}
