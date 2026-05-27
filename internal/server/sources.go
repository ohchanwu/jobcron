package server

import "github.com/ohchanwu/job-scraper/internal/scraper"

// sourceLabels maps source identifiers to the short Korean display label
// rendered in the UI ("점핏" inline after the company name). When a new
// source is added, give it an entry here; unknown sources fall back to the
// source ID itself rather than throwing, so a freshly added scraper still
// renders something readable until the label is filled in.
var sourceLabels = map[string]string{
	"jumpit":  "점핏",
	"worknet": "워크넷",
	"rallit":  "랠릿",
	"naver":   "네이버",
	"demoday": "데모데이",
	"daangn":  "당근",
	// 잡알리오 was removed as a registered scraper on 2026-05-27 (NCS R600020
	// didn't deliver dev/SWE roles in practice). The label is kept so any
	// existing alio rows in a user's DB render with their proper name until
	// the sweep clears them. Safe to delete once you're sure no user still
	// has 잡알리오 rows older than `sweepStaleWindow` (3 days).
	"alio": "잡알리오",
}

// sourceLabel returns the user-facing display name for a source identifier,
// falling back to the identifier itself when no mapping exists.
func sourceLabel(source string) string {
	if l, ok := sourceLabels[source]; ok {
		return l
	}
	return source
}

// sourceOption is one row of the profile form's source-toggle list — the
// identifier we round-trip on form submit plus the label shown to the user.
type sourceOption struct {
	ID      string
	Label   string
	Enabled bool
}

// sourceOptions returns one option per registered scraper, with each option
// flagged enabled when the given profile permits it. Order follows the
// scraper-registration order so the user sees a stable list.
func (s *Server) sourceOptions(disabled []string) []sourceOption {
	disabledSet := make(map[string]bool, len(disabled))
	for _, d := range disabled {
		disabledSet[d] = true
	}
	opts := make([]sourceOption, 0, len(s.sources))
	for _, src := range s.sources {
		id := src.Source()
		opts = append(opts, sourceOption{
			ID:      id,
			Label:   sourceLabel(id),
			Enabled: !disabledSet[id],
		})
	}
	return opts
}

// disabledSourceSet returns a lookup set of source IDs the user has opted
// out of. Postings whose source is NOT in this set are visible; this means
// data from a source that is no longer registered (e.g. a worknet posting
// after the user removes the key) keeps rendering, which is what we want —
// the user did not ask us to forget it.
func (s *Server) disabledSourceSet(disabled []string) map[string]bool {
	out := make(map[string]bool, len(disabled))
	for _, d := range disabled {
		out[d] = true
	}
	return out
}

// registeredSources exposes the source identifiers in registration order —
// used by tests and any caller that needs to enumerate without going
// through the full Scraper interface.
func (s *Server) registeredSources() []scraper.Scraper { return s.sources }

// allRegisteredSources returns one sourceOption per registered scraper, in
// registration order. Exposed to templates as `registeredSources` so the
// dashboard/archive/bookmarks pages can render the full source-filter
// pill bar regardless of which sources currently have postings visible —
// users want to SEE every source they could filter to, even when one is
// currently empty (matches the "the source exists; it's just quiet today"
// mental model).
func (s *Server) allRegisteredSources() []sourceOption {
	opts := make([]sourceOption, 0, len(s.sources))
	for _, src := range s.sources {
		id := src.Source()
		opts = append(opts, sourceOption{
			ID:      id,
			Label:   sourceLabel(id),
			Enabled: true,
		})
	}
	return opts
}
