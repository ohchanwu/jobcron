package alio

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ohchanwu/job-scraper/internal/scraper"
)

// Source is the stable source identifier persisted on every Posting we
// produce and matched against the user's DisabledSources list.
const Source = "alio"

const (
	defaultBaseURL  = "https://job.alio.go.kr"
	listingPath     = "/recruit.do"
	detailPathFmt   = "/recruitview.do?idx=%s"
	robotsCheckPath = "/recruit.do"
	userAgent       = "job-scraper/0.1 (+github.com/ohchanwu/job-scraper)"
	robotsTTL       = 24 * time.Hour
	defaultPageSize = 100 // 잡알리오 lets us pull the full IT-신입 universe in one page
	maxListingPages = 10  // defensive upper bound
)

// ncsCategoryCodes are the NCS 직무 codes we filter the listing to.
// `R600020` (정보통신) covers the IT cluster. To broaden coverage,
// append more codes here — but each one widens the non-dev surface
// (see API_NOTES.md).
var ncsCategoryCodes = []string{"R600020"}

// careerCodes are the 채용구분 codes we OR together. `R2010` is pure
// 신입; `R2030` is 신입+경력 — many public-sector postings open to
// both new-grads and experienced hires use this single code, so we
// include it to avoid missing legitimate 신입-friendly listings.
var careerCodes = []string{"R2010", "R2030"}

// Scraper is the 잡알리오 implementation of scraper.Scraper.
type Scraper struct {
	client    *http.Client
	baseURL   string
	rateLimit time.Duration
	pageSize  int

	mu          sync.Mutex
	lastRequest time.Time

	robotsMu    sync.Mutex
	robotsCache *robotsEntry
}

type robotsEntry struct {
	allowed   bool
	expiresAt time.Time
}

var _ scraper.Scraper = (*Scraper)(nil)

// New returns a 잡알리오 scraper paced at one request per second.
func New() *Scraper { return newScraper(defaultBaseURL, time.Second) }

func newScraper(baseURL string, rateLimit time.Duration) *Scraper {
	return &Scraper{
		client:    &http.Client{Timeout: 30 * time.Second},
		baseURL:   baseURL,
		rateLimit: rateLimit,
		pageSize:  defaultPageSize,
	}
}

// Source returns the stable source identifier.
func (s *Scraper) Source() string { return Source }

// CheckAccess verifies robots.txt currently permits scraping. The host's
// robots.txt 404s historically (RFC 9309 unrestricted), matching how
// `jumpit-api.saramin.co.kr` and `recruit.navercorp.com` are handled.
func (s *Scraper) CheckAccess(ctx context.Context) error {
	s.robotsMu.Lock()
	if s.robotsCache != nil && time.Now().Before(s.robotsCache.expiresAt) {
		allowed := s.robotsCache.allowed
		s.robotsMu.Unlock()
		if !allowed {
			return fmt.Errorf("alio: robots.txt disallows %s", robotsCheckPath)
		}
		return nil
	}
	s.robotsMu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		s.baseURL+"/robots.txt", nil)
	if err != nil {
		return fmt.Errorf("alio: build robots request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := s.client.Do(req)
	if err != nil {
		// Transient failure must not brick the user's tool.
		s.cacheRobots(true)
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	allowed := true
	if resp.StatusCode == http.StatusOK {
		allowed = robotsAllows(body, robotsCheckPath)
	}
	s.cacheRobots(allowed)
	if !allowed {
		return fmt.Errorf("alio: robots.txt disallows %s", robotsCheckPath)
	}
	return nil
}

func (s *Scraper) cacheRobots(allowed bool) {
	s.robotsMu.Lock()
	s.robotsCache = &robotsEntry{allowed: allowed, expiresAt: time.Now().Add(robotsTTL)}
	s.robotsMu.Unlock()
}

// FetchListing pages through the IT 신입-friendly listing. With
// pageSize=100 the full IT-신입 universe (~typically 30-100 active
// postings) fits in a single fetch, so this almost always exits at
// the first short page.
func (s *Scraper) FetchListing(ctx context.Context, limit int) ([]scraper.Posting, error) {
	var all []scraper.Posting
	for page := 1; page <= maxListingPages; page++ {
		q := url.Values{}
		q.Set("pageNo", strconv.Itoa(page))
		q.Set("pageSet", strconv.Itoa(s.pageSize))
		for _, c := range ncsCategoryCodes {
			q.Add("detail_code", c)
		}
		for _, c := range careerCodes {
			q.Add("career", c)
		}
		body, err := s.get(ctx, listingPath+"?"+q.Encode())
		if err != nil {
			return nil, err
		}
		postings, err := parseListing(body, s.baseURL)
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

// FetchDetail is a no-op for 잡알리오 — the listing carries every field
// we use, and the detail page's added value (NCS labels + PDF
// attachments) is not worth a per-posting HTTP round trip.
func (s *Scraper) FetchDetail(_ context.Context, p scraper.Posting) (scraper.Posting, error) {
	return p, nil
}

// get is the rate-limited GET helper.
func (s *Scraper) get(ctx context.Context, path string) ([]byte, error) {
	if err := s.waitForRateLimit(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("alio: build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,*/*")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("alio: GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("alio: read %s: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("alio: GET %s: status %d", path, resp.StatusCode)
	}
	return body, nil
}

func (s *Scraper) waitForRateLimit(ctx context.Context) error {
	s.mu.Lock()
	var wait time.Duration
	if !s.lastRequest.IsZero() {
		if elapsed := time.Since(s.lastRequest); elapsed < s.rateLimit {
			wait = s.rateLimit - elapsed
		}
	}
	s.lastRequest = time.Now().Add(wait)
	s.mu.Unlock()
	if wait <= 0 {
		return nil
	}
	select {
	case <-time.After(wait):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// detailURL returns the public posting URL for a given 잡알리오 idx.
func detailURL(baseURL, idx string) string {
	return baseURL + fmt.Sprintf(detailPathFmt, idx)
}

// --- Parsing ---------------------------------------------------------------

// rowAnchorPattern matches the per-posting anchor that opens the listing
// row. The capture groups are (idx, title). The title may contain whitespace
// and Korean characters; we trim aggressively at extract time.
var rowAnchorPattern = regexp.MustCompile(`<a href="/recruitview\.do\?idx=(\d+)"[^>]*>([^<]+)</a>`)

// tdPattern matches a single `<td ...>...</td>` cell. The legacy JSP
// indents heavily; whitespace is collapsed at extract time. `(?s)` lets
// `.` cross newlines.
var tdPattern = regexp.MustCompile(`(?s)<td[^>]*>(.*?)</td>`)

// trPattern matches a single `<tr>...</tr>` row.
var trPattern = regexp.MustCompile(`(?s)<tr[^>]*>(.*?)</tr>`)

// tagPattern matches any HTML tag — used to strip inline markup from cell
// contents (e.g. <span class="orange">D-6</span>).
var tagPattern = regexp.MustCompile(`<[^>]+>`)

// wsPattern collapses any run of whitespace (including HTML-encoded
// non-breaking spaces) into a single space.
var wsPattern = regexp.MustCompile(`(?:\s|&nbsp;)+`)

// parseListing extracts postings from a 잡알리오 listing page. baseURL is
// the absolute host used to materialize the public URL on each posting.
func parseListing(body []byte, baseURL string) ([]scraper.Posting, error) {
	rows := trPattern.FindAllSubmatch(body, -1)
	out := make([]scraper.Posting, 0, len(rows))
	for _, r := range rows {
		rowHTML := r[1]
		anchor := rowAnchorPattern.FindSubmatch(rowHTML)
		if anchor == nil {
			continue // not a posting row (header, footer, pagination, etc.)
		}
		idx := string(anchor[1])
		title := cleanText(anchor[2])
		if idx == "" || title == "" {
			continue
		}

		cells := extractCells(rowHTML)
		// Row shape from API_NOTES.md:
		//   [0] row index (page-local, not the posting id)
		//   [1] title (we already pulled this from the anchor)
		//   [2] company
		//   [3] location
		//   [4] employment type
		//   [5] posted date (YYYY.MM.DD)
		//   [6] closing date (YY.MM.DD plus D-N badge)
		//   [7] status
		var company, location, empType, postedRaw, closedRaw string
		if len(cells) > 2 {
			company = cells[2]
		}
		if len(cells) > 3 {
			location = cells[3]
		}
		if len(cells) > 4 {
			empType = cells[4]
		}
		if len(cells) > 5 {
			postedRaw = cells[5]
		}
		if len(cells) > 6 {
			closedRaw = cells[6]
		}

		p := scraper.Posting{
			Source:          Source,
			SourcePostingID: idx,
			URL:             detailURL(baseURL, idx),
			Title:           title,
			Company:         company,
			Location:        location,
			Newcomer:        true, // listing was server-side filtered to R2010+R2030
			CareerLevel:     "신입",
			StackTags:       []string{},
			Tags:            []scraper.Tag{},
			Description:     composeStubDescription(title, company, location, empType),
			RawJSON:         "", // 잡알리오 is HTML; we do not retain the page bytes per row
		}
		if t, ok := parsePostedDate(postedRaw); ok {
			p.PublishedAt = &t
		}
		if t, ok := parseClosedDate(closedRaw); ok {
			p.ClosedAt = &t
		}
		out = append(out, p)
	}
	return out, nil
}

// extractCells returns the inner-text of each `<td>` cell in a row, in
// document order. HTML tags inside cells are stripped; whitespace is
// collapsed to single spaces and trimmed.
func extractCells(rowHTML []byte) []string {
	matches := tdPattern.FindAllSubmatch(rowHTML, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, cleanText(m[1]))
	}
	return out
}

// cleanText strips HTML tags from raw cell bytes and collapses
// whitespace. Empty after trimming becomes "".
func cleanText(raw []byte) string {
	noTags := tagPattern.ReplaceAll(raw, []byte(" "))
	collapsed := wsPattern.ReplaceAll(noTags, []byte(" "))
	return strings.TrimSpace(string(collapsed))
}

// composeStubDescription joins the structured fields so FTS has more than
// the title to match on. 잡알리오 listings have no long-form description
// field (the JD lives in a PDF attachment on the detail page).
func composeStubDescription(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			kept = append(kept, t)
		}
	}
	return strings.Join(kept, " · ")
}

// --- Dates -----------------------------------------------------------------

var kstZone = time.FixedZone("KST", 9*60*60)

// parsePostedDate reads a `YYYY.MM.DD` posted-date string as KST midnight,
// returning UTC.
func parsePostedDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("2006.01.02", s, kstZone)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

// parseClosedDate reads the 잡알리오 closing cell. The visible date is
// `YY.MM.DD` but the cell ALSO contains a D-N badge after a `<br>`; we
// already stripped tags upstream, so the input here looks like
// `26.06.02 D-6`. We pull off the first space-separated token and parse
// the 2-digit year, mapping ≥50 to 19xx (a far-future safety boundary).
func parseClosedDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if i := strings.IndexByte(s, ' '); i > 0 {
		s = s[:i]
	}
	t, err := time.ParseInLocation("06.01.02", s, kstZone)
	if err != nil {
		return time.Time{}, false
	}
	// Go's "06" interprets 00-68 as 20xx and 69-99 as 19xx. That works
	// for our use case (anything before 2069 is a current or recent
	// posting); we leave it as-is.
	return t.UTC(), true
}

// --- robots.txt ------------------------------------------------------------

// robotsAllows is the same pragmatic subset of RFC 9309 used by every
// other scraper in this project — wildcard user-agent, prefix-match,
// longest-match-wins.
func robotsAllows(content []byte, path string) bool {
	var disallow, allow []string
	inStar := false
	sc := bufio.NewScanner(bytes.NewReader(content))
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "user-agent":
			inStar = value == "*"
		case "disallow":
			if inStar && value != "" {
				disallow = append(disallow, value)
			}
		case "allow":
			if inStar && value != "" {
				allow = append(allow, value)
			}
		}
	}
	blocked := longestPrefix(disallow, path)
	if blocked == 0 {
		return true
	}
	return longestPrefix(allow, path) >= blocked
}

func longestPrefix(rules []string, path string) int {
	best := 0
	for _, r := range rules {
		if len(r) > best && strings.HasPrefix(path, r) {
			best = len(r)
		}
	}
	return best
}
