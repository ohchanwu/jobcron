package scraper

import "context"

// SourceKind classifies a scraper by what kind of board it pulls from.
// The UI uses this to group aggregator pills (jobs from many companies)
// separately from company pills (jobs from a single company).
type SourceKind int

const (
	// SourceKindAggregator is a multi-company job board (점핏, 랠릿,
	// 데모데이, public-sector aggregators, etc.). Most scrapers are
	// aggregators; new scrapers default to this kind unless explicitly
	// company-scoped.
	SourceKindAggregator SourceKind = iota

	// SourceKindCompany is a direct company careers page that only
	// lists jobs at one organization (네이버 careers, 당근 careers, etc.).
	SourceKindCompany
)

// Scraper fetches job postings from a single source.
//
// The two-phase shape (a cheap listing, then a per-posting detail fetch) lets
// the server fetch detail pages only for postings it has not seen before —
// see the design doc's scrape flow. A single-phase source can implement
// FetchDetail as a no-op that returns its argument unchanged.
type Scraper interface {
	// Source is the stable source identifier, e.g. "jumpit".
	Source() string

	// Kind classifies the source as a multi-company aggregator or a
	// single-company careers page. Used by the UI to group pills.
	Kind() SourceKind

	// CheckAccess reports whether scraping is currently permitted (robots.txt
	// and similar). A non-nil error means scraping must not proceed.
	CheckAccess(ctx context.Context) error

	// FetchListing returns listing-level postings — without the detail-only
	// fields (Description, Tags, Education, PublishedAt). limit caps the
	// number returned; limit <= 0 means no cap.
	FetchListing(ctx context.Context, limit int) ([]Posting, error)

	// FetchDetail enriches one listing-level posting with its detail-page
	// fields and returns the enriched copy.
	FetchDetail(ctx context.Context, p Posting) (Posting, error)
}
