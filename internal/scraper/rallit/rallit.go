package rallit

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ohchanwu/jobcron/internal/scraper"
)

// Source is the stable source identifier persisted on every Posting we
// produce and matched against the user's DisabledSources list.
const Source = "rallit"

const (
	listingPath      = "/api/v1/position"
	detailPathPrefix = "/api/v1/position/"
	defaultPageSize  = 50
	maxListingPages  = 20 // defensive upper bound; ~1000 신입 postings would be unusual
)

// newcomerLevels are the rallit jobLevel enum values we treat as
// 신입-friendly: BEGINNER (신입), INTERN (인턴), and IRRELEVANT (경력 무관,
// implicitly welcoming new grads). The server-side filter does an "ANY
// match" so some returned postings will additionally list JUNIOR/MIDDLE
// levels — accepted as legitimate signal; the scoring stage handles refinement.
var newcomerLevels = []string{"BEGINNER", "INTERN", "IRRELEVANT"}

// devJobGroup restricts the listing to 랠릿's DEVELOPER 직군 (job group). On
// 랠릿 this single umbrella group covers every tech role — backend / frontend /
// mobile SWE plus 데이터(분석·엔지니어링·사이언스), AI/ML, 보안, DevOps·인프라·
// 클라우드, QA, 임베디드 — while excluding the marketing / design / PM / HR /
// sales 신입 roles that otherwise flood the briefing (recon 2026-06-05: 135
// 신입 postings total → 69 with this filter; the excluded 66 were all non-dev,
// e.g. 모두닥's 마케터 / 사무보조 / 채용운영(TA)). The param takes a SINGLE
// value — comma-OR is NOT supported here (unlike jobLevel), and an unknown
// value silently returns zero rows, so DEVELOPER must stay exact. See
// API_NOTES.md "직군 (job group) filter".
const devJobGroup = "DEVELOPER"

// Scraper is the 랠릿 implementation of scraper.Scraper.
type Scraper struct {
	client   *client
	pageSize int
}

// Scraper must satisfy the scraper.Scraper contract.
var _ scraper.Scraper = (*Scraper)(nil)

// New returns a 랠릿 scraper, paced at one request per second to match
// the etiquette used by the 점핏 / 워크넷 scrapers.
func New() *Scraper {
	return newScraper(defaultBaseURL, time.Second)
}

// newScraper builds a 랠릿 scraper against baseURL with the given rate
// limit. Tests pass a test-server URL and a zero rate limit.
func newScraper(baseURL string, rateLimit time.Duration) *Scraper {
	return &Scraper{
		client:   newClient(baseURL, rateLimit),
		pageSize: defaultPageSize,
	}
}

// Source returns the stable source identifier.
func (s *Scraper) Source() string { return Source }

// Kind reports that 랠릿 is a multi-company aggregator.
func (s *Scraper) Kind() scraper.SourceKind { return scraper.SourceKindAggregator }

// CheckAccess verifies that robots.txt currently permits scraping rallit.
func (s *Scraper) CheckAccess(ctx context.Context) error {
	return s.client.checkAccess(ctx)
}

// FetchListing pages through the 신입-filtered listing. Stops at the first
// short page, at limit postings (when limit > 0), or at the page cap.
func (s *Scraper) FetchListing(ctx context.Context, limit int) ([]scraper.Posting, error) {
	var all []scraper.Posting
	for page := 1; page <= maxListingPages; page++ {
		q := url.Values{}
		q.Set("pageNumber", strconv.Itoa(page))
		q.Set("pageSize", strconv.Itoa(s.pageSize))
		q.Set("jobLevel", strings.Join(newcomerLevels, ","))
		q.Set("jobGroup", devJobGroup)

		body, err := s.client.get(ctx, listingPath+"?"+q.Encode())
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
			break
		}
	}
	return all, nil
}

// FetchDetail enriches a listing-level posting with the detail call.
func (s *Scraper) FetchDetail(ctx context.Context, p scraper.Posting) (scraper.Posting, error) {
	if strings.TrimSpace(p.SourcePostingID) == "" {
		return scraper.Posting{}, fmt.Errorf("rallit: detail: missing posting id")
	}
	body, err := s.client.get(ctx, detailPathPrefix+p.SourcePostingID)
	if err != nil {
		return scraper.Posting{}, err
	}
	return parseDetail(p, body)
}
