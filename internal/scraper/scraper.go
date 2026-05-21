package scraper

import "context"

// Scraper fetches job postings from a single source.
//
// The two-phase shape (a cheap listing, then a per-posting detail fetch) lets
// the server fetch detail pages only for postings it has not seen before —
// see the design doc's scrape flow. A single-phase source can implement
// FetchDetail as a no-op that returns its argument unchanged.
type Scraper interface {
	// Source is the stable source identifier, e.g. "jumpit".
	Source() string

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
