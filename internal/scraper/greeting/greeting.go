// Package greeting scrapes 신입 (new-grad) IT postings from Korean
// companies that host their careers board on 그리팅 (Greeting, by 두들린) —
// the leading Korean-native ATS. Each tenant runs a server-rendered Next.js
// board at {slug}.career.greetinghr.com whose openings are inlined in the
// page's __NEXT_DATA__ JSON.
//
// Unlike the Greenhouse boards, Greeting carries a structured 신입 signal
// (jobPositionCareer.careerType ∈ NEW_COMER / NOT_MATTER / EXPERIENCED) and
// a structured job-family (workspaceOccupation.occupation), so detection is
// far more reliable than a title heuristic.
//
// All tenants share one Source ("greeting"): the per-company name comes from
// each opening's group.name, and a single profile toggle covers the whole
// curated slug list (which would otherwise bloat the toggle list). See
// API_NOTES.md for the __NEXT_DATA__ shape, the curated slug list, and the
// per-tenant landing-redirect / custom-domain handling.
package greeting

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/ohchanwu/jobcron/internal/scraper"
)

// Source is the stable identifier persisted on every Posting and matched
// against the user's DisabledSources list.
const Source = "greeting"

const (
	userAgent       = "jobcron/0.1 (+github.com/ohchanwu/jobcron)"
	robotsCheckPath = "/ko/"
	robotsTTL       = 24 * time.Hour
	requestTimeout  = 30 * time.Second
)

// Scraper is the 그리팅 implementation of scraper.Scraper. One instance
// covers the whole curated tenant list.
type Scraper struct {
	tenants   []tenant
	client    *http.Client
	rateLimit time.Duration

	// origin returns the scheme://host base for a tenant's board. Default
	// is https://{slug}.career.greetinghr.com; overridable in tests.
	origin func(tenant) string

	mu          sync.Mutex
	lastRequest time.Time

	robotsMu    sync.Mutex
	robotsCache map[string]robotsEntry // keyed by origin
}

type robotsEntry struct {
	allowed   bool
	expiresAt time.Time
}

var _ scraper.Scraper = (*Scraper)(nil)

// New returns a 그리팅 scraper over the curated tenant list, paced at one
// request per second.
func New() *Scraper { return newScraper(curatedTenants, time.Second) }

func newScraper(tenants []tenant, rateLimit time.Duration) *Scraper {
	// The default http.Client follows redirects, which we rely on to resolve
	// each tenant's landing path (/ko/home, /ko/main, …) and any custom domain.
	return &Scraper{
		tenants:     tenants,
		client:      &http.Client{Timeout: requestTimeout},
		rateLimit:   rateLimit,
		origin:      func(t tenant) string { return "https://" + t.host() },
		robotsCache: make(map[string]robotsEntry),
	}
}

// Source returns the stable source identifier.
func (s *Scraper) Source() string { return Source }

// Kind reports that 그리팅 is a multi-company aggregator.
func (s *Scraper) Kind() scraper.SourceKind { return scraper.SourceKindAggregator }

// CheckAccess verifies robots.txt on each tenant host. The boards share a
// Cloudflare-served robots.txt that allows the /ko/ board path for
// User-agent: * (only /m/*, /a/*, and /o/*/apply are disallowed).
func (s *Scraper) CheckAccess(ctx context.Context) error {
	for _, t := range s.tenants {
		if err := s.checkRobots(ctx, s.origin(t)); err != nil {
			return fmt.Errorf("greeting: %s: %w", t.slug, err)
		}
	}
	return nil
}

func (s *Scraper) checkRobots(ctx context.Context, origin string) error {
	s.robotsMu.Lock()
	if e, ok := s.robotsCache[origin]; ok && time.Now().Before(e.expiresAt) {
		allowed := e.allowed
		s.robotsMu.Unlock()
		if !allowed {
			return fmt.Errorf("robots.txt disallows %s", robotsCheckPath)
		}
		return nil
	}
	s.robotsMu.Unlock()

	body, status, err := s.fetch(ctx, origin+"/robots.txt")
	if err != nil {
		// Transient robots.txt failure must not brick the user's tool.
		s.cacheRobots(origin, true)
		return nil
	}
	allowed := true
	if status == http.StatusOK {
		allowed = scraper.RobotsAllows(body, robotsCheckPath)
	}
	s.cacheRobots(origin, allowed)
	if !allowed {
		return fmt.Errorf("robots.txt disallows %s", robotsCheckPath)
	}
	return nil
}

func (s *Scraper) cacheRobots(origin string, allowed bool) {
	s.robotsMu.Lock()
	s.robotsCache[origin] = robotsEntry{allowed: allowed, expiresAt: time.Now().Add(robotsTTL)}
	s.robotsMu.Unlock()
}

// FetchListing pulls every 신입 dev opening across all curated tenants. Each
// tenant board is one fetch (the openings are inline in __NEXT_DATA__), so
// FetchDetail is a no-op.
func (s *Scraper) FetchListing(ctx context.Context, limit int) ([]scraper.Posting, error) {
	var out []scraper.Posting
	for _, t := range s.tenants {
		body, _, finalURL, err := s.get(ctx, s.origin(t)+"/")
		if err != nil {
			// One dead tenant must not sink the whole source — skip it.
			continue
		}
		origin := originOf(finalURL, t.host())
		postings, err := parseBoard(body, t, origin)
		if err != nil {
			continue
		}
		out = append(out, postings...)
		if limit > 0 && len(out) >= limit {
			return out[:limit], nil
		}
	}
	return out, nil
}

// FetchDetail is a no-op — FetchListing already pulled every field this
// scraper reads from the board's __NEXT_DATA__.
func (s *Scraper) FetchDetail(_ context.Context, p scraper.Posting) (scraper.Posting, error) {
	return p, nil
}

// get fetches a URL (following redirects) and returns the body, status, and
// the final URL after redirects — needed to resolve each tenant's landing
// path and custom domain for building job URLs.
func (s *Scraper) get(ctx context.Context, rawURL string) ([]byte, int, string, error) {
	if err := s.waitForRateLimit(ctx); err != nil {
		return nil, 0, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, "", fmt.Errorf("GET %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, "", fmt.Errorf("read %s: %w", rawURL, err)
	}
	final := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		final = resp.Request.URL.String()
	}
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, final, fmt.Errorf("GET %s: status %d", rawURL, resp.StatusCode)
	}
	return body, resp.StatusCode, final, nil
}

// fetch is the robots.txt helper — a GET that tolerates non-200 without
// erroring on the status, returning (body, status).
func (s *Scraper) fetch(ctx context.Context, rawURL string) ([]byte, int, error) {
	body, status, _, err := s.get(ctx, rawURL)
	if err != nil && status != 0 {
		// Non-200 with a status is fine for robots (caller decides).
		return body, status, nil
	}
	return body, status, err
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

// originOf returns the scheme://host origin of a final (post-redirect) URL,
// falling back to the tenant's greetinghr host if the URL can't be parsed.
func originOf(finalURL, fallbackHost string) string {
	if u, err := url.Parse(finalURL); err == nil && u.Scheme != "" && u.Host != "" {
		return u.Scheme + "://" + u.Host
	}
	return "https://" + fallbackHost
}
