package worknet

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ohchanwu/job-scraper/internal/scraper"
)

// Source is the stable source identifier persisted on every Posting we
// produce and matched against the user's DisabledSources list.
const Source = "worknet"

// occupationCodes are the KECO 직종코드 values we ask the API to filter
// the listing to. The 22xx series covers application software developers,
// information-systems specialists, data professionals, and information
// security — i.e. the bulk of 신입 IT roles. See API_NOTES.md for the
// full table and the rationale.
var occupationCodes = []string{"2211", "2212", "2213", "2214"}

const (
	defaultPageSize = 100 // API caps display at 100 per page
	maxListingPages = 20  // defensive upper bound
)

// Scraper is the 워크넷 implementation of scraper.Scraper.
type Scraper struct {
	client   *client
	pageSize int
}

// Scraper must satisfy the scraper.Scraper contract.
var _ scraper.Scraper = (*Scraper)(nil)

// New returns a 워크넷 scraper authenticated with the given OpenAPI key,
// paced at one request per second to match the Jumpit scraper's etiquette.
// Returns an error when authKey is empty — registration must be intentional.
func New(authKey string) (*Scraper, error) {
	if strings.TrimSpace(authKey) == "" {
		return nil, errors.New("worknet: authKey is required")
	}
	return newScraper(defaultBaseURL, authKey, time.Second), nil
}

// newScraper builds a 워크넷 scraper against baseURL with the given rate
// limit. Tests pass a test-server URL and a zero rate limit.
func newScraper(baseURL, authKey string, rateLimit time.Duration) *Scraper {
	return &Scraper{
		client:   newClient(baseURL, authKey, rateLimit),
		pageSize: defaultPageSize,
	}
}

// Source returns the stable source identifier.
func (s *Scraper) Source() string { return Source }

// Kind reports that 워크넷 is a multi-company aggregator (public-sector).
func (s *Scraper) Kind() scraper.SourceKind { return scraper.SourceKindAggregator }

// CheckAccess verifies that scraping is currently permitted (robots.txt).
func (s *Scraper) CheckAccess(ctx context.Context) error {
	return s.client.checkAccess(ctx)
}

// FetchListing pages through the 신입-filtered IT listing. Stops at the
// first short page, at limit postings (when limit > 0), or at the page cap.
func (s *Scraper) FetchListing(ctx context.Context, limit int) ([]scraper.Posting, error) {
	var all []scraper.Posting
	for page := 1; page <= maxListingPages; page++ {
		params := url.Values{}
		params.Set("callTp", "L")
		params.Set("returnType", "XML")
		params.Set("startPage", strconv.Itoa(page))
		params.Set("display", strconv.Itoa(s.pageSize))
		params.Set("occupation", strings.Join(occupationCodes, "|"))
		params.Set("minCareer", "0")
		params.Set("maxCareer", "0")

		body, err := s.client.call(ctx, params)
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
			break // short page = last page
		}
	}
	return all, nil
}

// FetchDetail enriches a listing-level posting with the detail-call fields.
func (s *Scraper) FetchDetail(ctx context.Context, p scraper.Posting) (scraper.Posting, error) {
	if strings.TrimSpace(p.SourcePostingID) == "" {
		return scraper.Posting{}, fmt.Errorf("worknet: detail: missing wantedAuthNo")
	}
	params := url.Values{}
	params.Set("callTp", "D")
	params.Set("returnType", "XML")
	params.Set("wantedAuthNo", p.SourcePostingID)
	body, err := s.client.call(ctx, params)
	if err != nil {
		return scraper.Posting{}, err
	}
	return parseDetail(p, body)
}
