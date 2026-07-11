package jumpit

import (
	"context"
	"fmt"
	"time"

	"github.com/ohchanwu/jobcron/internal/scraper"
)

const (
	listingPath      = "/api/positions"
	detailPathPrefix = "/api/position/"
	defaultPageSize  = 500 // ~9x headroom over the current 신입 universe (~57)
	maxListingPages  = 20  // defensive bound; steady state exits after page 1
)

// Scraper is the 점핏 (Jumpit) implementation of scraper.Scraper.
type Scraper struct {
	client   *client
	pageSize int
}

// Scraper must satisfy the scraper.Scraper contract.
var _ scraper.Scraper = (*Scraper)(nil)

// New returns a 점핏 scraper that talks to the live API, paced at one request
// per second.
func New() *Scraper {
	return newScraper(defaultBaseURL, time.Second)
}

// newScraper builds a 점핏 scraper against baseURL with the given request
// rate limit (tests pass a test-server URL and a zero rate limit).
func newScraper(baseURL string, rateLimit time.Duration) *Scraper {
	return &Scraper{
		client:   newClient(baseURL, rateLimit),
		pageSize: defaultPageSize,
	}
}

// Source returns the stable source identifier.
func (s *Scraper) Source() string { return Source }

// Kind reports that 점핏 is a multi-company aggregator.
func (s *Scraper) Kind() scraper.SourceKind { return scraper.SourceKindAggregator }

// CheckAccess verifies robots.txt currently permits scraping 점핏.
func (s *Scraper) CheckAccess(ctx context.Context) error {
	return s.client.checkAccess(ctx)
}

// FetchListing walks the 점핏 신입 listing (career=0) and returns listing-level
// postings. It stops at the first short page, at limit postings (when
// limit > 0), or at the page cap — in steady state the whole 신입 universe
// fits in a single page.
func (s *Scraper) FetchListing(ctx context.Context, limit int) ([]scraper.Posting, error) {
	var all []scraper.Posting
	for page := 1; page <= maxListingPages; page++ {
		path := fmt.Sprintf("%s?career=0&size=%d&sort=popular&highlight=false&page=%d",
			listingPath, s.pageSize, page)
		body, err := s.client.get(ctx, path)
		if err != nil {
			return nil, err
		}
		postings, err := parseListing(body)
		if err != nil {
			return nil, err
		}
		all = append(all, postings...)
		if limit > 0 && len(all) >= limit {
			return all[:limit], nil
		}
		if len(postings) < s.pageSize {
			break // a short page is the last page
		}
	}
	return all, nil
}

// FetchDetail enriches a listing-level posting with its 점핏 detail page.
func (s *Scraper) FetchDetail(ctx context.Context, p scraper.Posting) (scraper.Posting, error) {
	body, err := s.client.get(ctx, detailPathPrefix+p.SourcePostingID)
	if err != nil {
		return scraper.Posting{}, err
	}
	return parseDetail(p, body)
}
