// Package greenhouse scrapes 신입 (new-grad) IT postings from companies
// that host their careers board on Greenhouse's public, no-auth board API
// (boards-api.greenhouse.io/v1/boards/{token}/jobs?content=true).
//
// One Greenhouse board is one company, so each registered Scraper wraps a
// single Tenant: its Source(), source badge, and profile toggle stay
// per-company while the Greenhouse plumbing — HTTP client, robots.txt
// check, 1-req/s pacing, JSON shape, HTML stripping — is shared. 당근 is
// the first tenant (it pioneered this path); krafton / moloco / sendbird
// follow with the title-heuristic 신입 detector because, unlike 당근, their
// boards carry no structured 신입 metadata field.
//
// See API_NOTES.md for the per-tenant URL and detection strategies and the
// curated token list.
package greenhouse

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ohchanwu/jobcron/internal/scraper"
)

const (
	defaultAPIBaseURL = "https://boards-api.greenhouse.io"
	apiRobotsCheck    = "/v1/boards/"
	siteRobotsCheck   = "/jobs"
	userAgent         = "jobcron/0.1 (+github.com/ohchanwu/jobcron)"
	robotsTTL         = 24 * time.Hour
	requestTimeout    = 30 * time.Second
)

// Scraper is a single-company Greenhouse implementation of
// scraper.Scraper, parameterized by its Tenant.
type Scraper struct {
	t          Tenant
	client     *http.Client
	apiBaseURL string
	rateLimit  time.Duration

	mu          sync.Mutex
	lastRequest time.Time

	robotsMu     sync.Mutex
	robotsCache  *robotsEntry
	siteRobotsMu sync.Mutex
	siteRobots   *robotsEntry
}

type robotsEntry struct {
	allowed   bool
	expiresAt time.Time
}

var _ scraper.Scraper = (*Scraper)(nil)

// New returns a Greenhouse scraper for one tenant, paced at one request
// per second.
func New(t Tenant) *Scraper { return newScraper(t, defaultAPIBaseURL, time.Second) }

func newScraper(t Tenant, apiBaseURL string, rateLimit time.Duration) *Scraper {
	return &Scraper{
		t:          t,
		client:     &http.Client{Timeout: requestTimeout},
		apiBaseURL: apiBaseURL,
		rateLimit:  rateLimit,
	}
}

// Source returns the tenant's stable source identifier.
func (s *Scraper) Source() string { return s.t.Source }

// Kind reports that a Greenhouse board is a single-company source.
func (s *Scraper) Kind() scraper.SourceKind { return scraper.SourceKindCompany }

// CheckAccess verifies robots.txt on the Greenhouse API host (where every
// request lands) and, for tenants whose click-through URL points at their
// own careers site (Link == LinkSite, i.e. 당근), that site too. The API
// host only disallows /embed/, which we never request.
func (s *Scraper) CheckAccess(ctx context.Context) error {
	if (s.t.Link == LinkSite || s.t.Link == LinkSiteJob) && s.t.SiteURL != "" {
		if err := s.checkRobotsHost(ctx, s.t.SiteURL, siteRobotsCheck, &s.siteRobotsMu, &s.siteRobots); err != nil {
			return fmt.Errorf("greenhouse %s: site robots: %w", s.t.Source, err)
		}
	}
	if err := s.checkRobotsHost(ctx, s.apiBaseURL, apiRobotsCheck, &s.robotsMu, &s.robotsCache); err != nil {
		return fmt.Errorf("greenhouse %s: api robots: %w", s.t.Source, err)
	}
	return nil
}

func (s *Scraper) checkRobotsHost(
	ctx context.Context, baseURL, path string, mu *sync.Mutex, cache **robotsEntry,
) error {
	mu.Lock()
	if *cache != nil && time.Now().Before((*cache).expiresAt) {
		allowed := (*cache).allowed
		mu.Unlock()
		if !allowed {
			return fmt.Errorf("robots.txt disallows %s on %s", path, baseURL)
		}
		return nil
	}
	mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/robots.txt", nil)
	if err != nil {
		return fmt.Errorf("build robots request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := s.client.Do(req)
	if err != nil {
		// A transient robots.txt failure must not brick the user's tool —
		// same posture as every other scraper in the project.
		cacheRobots(mu, cache, true)
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	allowed := true
	if resp.StatusCode == http.StatusOK {
		allowed = robotsAllows(body, path)
	}
	cacheRobots(mu, cache, allowed)
	if !allowed {
		return fmt.Errorf("robots.txt disallows %s on %s", path, baseURL)
	}
	return nil
}

func cacheRobots(mu *sync.Mutex, cache **robotsEntry, allowed bool) {
	mu.Lock()
	*cache = &robotsEntry{allowed: allowed, expiresAt: time.Now().Add(robotsTTL)}
	mu.Unlock()
}

// FetchListing pulls every 신입 IT posting from this tenant's board in one
// call. The board API supports ?content=true so the full HTML body and
// metadata arrive in a single round trip, which is why FetchDetail is a
// no-op.
func (s *Scraper) FetchListing(ctx context.Context, limit int) ([]scraper.Posting, error) {
	body, err := s.get(ctx, "/v1/boards/"+s.t.Token+"/jobs?content=true")
	if err != nil {
		return nil, err
	}
	postings, err := parseListing(body, s.t)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(postings) > limit {
		postings = postings[:limit]
	}
	return postings, nil
}

// FetchDetail is a no-op — FetchListing already pulled every field this
// scraper reads thanks to ?content=true.
func (s *Scraper) FetchDetail(_ context.Context, p scraper.Posting) (scraper.Posting, error) {
	return p, nil
}

func (s *Scraper) get(ctx context.Context, path string) ([]byte, error) {
	if err := s.waitForRateLimit(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.apiBaseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("greenhouse %s: build request: %w", s.t.Source, err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("greenhouse %s: GET %s: %w", s.t.Source, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("greenhouse %s: read %s: %w", s.t.Source, path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("greenhouse %s: GET %s: status %d", s.t.Source, path, resp.StatusCode)
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
