package naver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ohchanwu/job-scraper/internal/scraper"
)

// Source is the stable source identifier persisted on every Posting we
// produce and matched against the user's DisabledSources list.
const Source = "naver"

const (
	defaultBaseURL  = "https://recruit.navercorp.com"
	listingPath     = "/rcrt/loadJobList.do"
	robotsCheckPath = "/rcrt/loadJobList.do"
	userAgent       = "job-scraper/0.1 (+github.com/ohchanwu/job-scraper)"
	robotsTTL       = 24 * time.Hour
)

// newcomerCodes are the entTypeCd values we treat as 신입-friendly:
// 0010 (신입) and 0030 (무관 — any-experience listings are usually open
// to new grads, especially in NAVER LABS / Cloud lines).
var newcomerCodes = map[string]bool{
	"0010": true,
	"0030": true,
}

// Scraper is the Naver implementation of scraper.Scraper. Single-phase —
// the listing endpoint carries every field we use, so FetchDetail is a
// no-op.
type Scraper struct {
	client    *http.Client
	baseURL   string
	rateLimit time.Duration

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

// New returns a Naver scraper paced at one request per second.
func New() *Scraper { return newScraper(defaultBaseURL, time.Second) }

func newScraper(baseURL string, rateLimit time.Duration) *Scraper {
	return &Scraper{
		client:    &http.Client{Timeout: 30 * time.Second},
		baseURL:   baseURL,
		rateLimit: rateLimit,
	}
}

// Source returns the stable source identifier.
func (s *Scraper) Source() string { return Source }

// Kind reports that 네이버 careers is a single-company source.
func (s *Scraper) Kind() scraper.SourceKind { return scraper.SourceKindCompany }

// CheckAccess verifies robots.txt currently permits scraping. The Naver
// recruit host's robots.txt 404s historically; per RFC 9309 that is
// unrestricted (mirrors Jumpit's API-host handling). A 200 with an
// explicit Disallow would block, so we surface that case rather than
// silently allowing it.
func (s *Scraper) CheckAccess(ctx context.Context) error {
	s.robotsMu.Lock()
	if s.robotsCache != nil && time.Now().Before(s.robotsCache.expiresAt) {
		allowed := s.robotsCache.allowed
		s.robotsMu.Unlock()
		if !allowed {
			return fmt.Errorf("naver: robots.txt disallows %s", robotsCheckPath)
		}
		return nil
	}
	s.robotsMu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		s.baseURL+"/robots.txt", nil)
	if err != nil {
		return fmt.Errorf("naver: build robots request: %w", err)
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
		return fmt.Errorf("naver: robots.txt disallows %s", robotsCheckPath)
	}
	return nil
}

func (s *Scraper) cacheRobots(allowed bool) {
	s.robotsMu.Lock()
	s.robotsCache = &robotsEntry{allowed: allowed, expiresAt: time.Now().Add(robotsTTL)}
	s.robotsMu.Unlock()
}

// FetchListing returns all 신입-friendly postings across the Naver group.
// The endpoint returns the full open universe (~25-30 postings typically),
// which we filter client-side to entTypeCd ∈ {0010 신입, 0030 무관} — the
// server-side entTypeCdArr filter does not honor multi-value queries.
// limit caps the returned slice (post-filter); pass 0 for no cap.
func (s *Scraper) FetchListing(ctx context.Context, limit int) ([]scraper.Posting, error) {
	body, err := s.get(ctx, listingPath+"?firstIndex=0")
	if err != nil {
		return nil, err
	}
	postings, err := parseListing(body)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(postings) > limit {
		postings = postings[:limit]
	}
	return postings, nil
}

// FetchDetail is a no-op for Naver — the listing carries every field we use.
// Re-stamping LastSeenAt would be the caller's job; we return the input as-is.
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
		return nil, fmt.Errorf("naver: build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json,*/*")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("naver: GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("naver: read %s: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("naver: GET %s: status %d", path, resp.StatusCode)
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

// listingResponse mirrors loadJobList.do's JSON envelope. We only decode
// fields we use — extra keys in the live response are ignored by Go's
// JSON decoder.
type listingResponse struct {
	Result    string      `json:"result"`
	TotalSize int         `json:"totalSize"`
	List      []wantedRow `json:"list"`
}

type wantedRow struct {
	AnnoID         int64  `json:"annoId"`
	AnnoSubject    string `json:"annoSubject"`
	SysCompanyCdNm string `json:"sysCompanyCdNm"`
	EntTypeCd      string `json:"entTypeCd"`
	EntTypeCdNm    string `json:"entTypeCdNm"`
	ClassCdNm      string `json:"classCdNm"`
	SubJobCdNm     string `json:"subJobCdNm"`
	EmpTypeCdNm    string `json:"empTypeCdNm"`
	StaYmd         string `json:"staYmd"` // YYYYMMDD
	EndYmd         string `json:"endYmd"` // YYYYMMDD
	JobDetailLink  string `json:"jobDetailLink"`
}

// parseListing decodes the loadJobList.do payload, filters to 신입+무관,
// and produces normalized Posting values.
func parseListing(body []byte) ([]scraper.Posting, error) {
	var resp listingResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("naver: decode listing: %w", err)
	}
	if resp.Result != "Y" {
		return nil, fmt.Errorf("naver: listing result = %q (expected Y)", resp.Result)
	}
	out := make([]scraper.Posting, 0, len(resp.List))
	for _, r := range resp.List {
		if r.AnnoID == 0 || !newcomerCodes[strings.TrimSpace(r.EntTypeCd)] {
			continue
		}
		p := scraper.Posting{
			Source:          Source,
			SourcePostingID: fmt.Sprintf("%d", r.AnnoID),
			URL:             strings.TrimSpace(r.JobDetailLink),
			Title:           strings.TrimSpace(r.AnnoSubject),
			Company:         strings.TrimSpace(r.SysCompanyCdNm),
			Newcomer:        true,
			CareerLevel:     strings.TrimSpace(r.EntTypeCdNm),
			StackTags:       []string{},
			Tags:            []scraper.Tag{},
			// Description synthesized from category + employment-type
			// fields — Naver has no JSON detail endpoint, and FTS needs
			// SOMETHING to keyword-match against beyond the title.
			Description: composeStubDescription(r),
			RawJSON:     string(body),
		}
		applyDates(&p, r.StaYmd, r.EndYmd)
		out = append(out, p)
	}
	return out, nil
}

// composeStubDescription joins the structured category fields so FTS
// matching has more than just the title to chew on. Empty fields are
// silently skipped.
func composeStubDescription(r wantedRow) string {
	parts := []string{r.ClassCdNm, r.SubJobCdNm, r.EmpTypeCdNm, r.CareerNote()}
	var kept []string
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			kept = append(kept, t)
		}
	}
	return strings.Join(kept, " · ")
}

// CareerNote returns a short label like "신입" / "무관" for display.
func (r wantedRow) CareerNote() string { return strings.TrimSpace(r.EntTypeCdNm) }

// applyDates parses YYYYMMDD KST dates into UTC PublishedAt / ClosedAt.
// Empty or malformed values are silently dropped.
func applyDates(p *scraper.Posting, staYmd, endYmd string) {
	if t, ok := parseYmd(staYmd); ok {
		p.PublishedAt = &t
	}
	if t, ok := parseYmd(endYmd); ok {
		p.ClosedAt = &t
	}
}

var kstZone = time.FixedZone("KST", 9*60*60)

func parseYmd(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("20060102", s, kstZone)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

// robotsAllows is the same pragmatic subset of RFC 9309 used by jumpit
// and rallit — wildcard user-agent, prefix-match, longest-match-wins.
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
