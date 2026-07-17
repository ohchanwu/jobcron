package jumpit

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/ohchanwu/jobcron/internal/scraper"
)

const (
	// defaultBaseURL is the 점핏 API host (a separate subdomain from the
	// user-facing site).
	defaultBaseURL = "https://jumpit-api.saramin.co.kr"

	// userAgent identifies jobcron politely, per the design's operational
	// commitments.
	userAgent     = "jobcron/0.1 (+github.com/ohchanwu/jobcron)"
	originHeader  = "https://jumpit.saramin.co.kr"
	refererHeader = "https://jumpit.saramin.co.kr/"
)

// robotsTTL is how long a robots.txt verdict is cached per host.
const robotsTTL = 24 * time.Hour

// robotsCheckPath is the representative path checked against robots.txt; both
// scrape endpoints (/api/positions and /api/position/{id}) sit under /api/.
const robotsCheckPath = "/api/positions"

// defaultRobotsHosts are the hosts whose robots.txt the scraper honors: the
// API host it fetches from and the user-facing host it links to.
var defaultRobotsHosts = []string{
	"https://jumpit-api.saramin.co.kr",
	"https://jumpit.saramin.co.kr",
}

// client is the low-level 점핏 HTTP client. It paces requests so consecutive
// calls are at least rateLimit apart and caches robots.txt verdicts per host.
type client struct {
	http      *http.Client
	baseURL   string
	rateLimit time.Duration

	mu          sync.Mutex
	lastRequest time.Time

	robotsHosts []string
	robotsMu    sync.Mutex
	robotsCache map[string]robotsEntry
}

// robotsEntry is a cached per-host robots.txt verdict.
type robotsEntry struct {
	allowed   bool
	expiresAt time.Time
}

// newClient returns a client targeting baseURL. rateLimit is the minimum
// spacing enforced between requests; pass 0 in tests to disable pacing.
func newClient(baseURL string, rateLimit time.Duration) *client {
	return &client{
		http:        &http.Client{Timeout: 30 * time.Second},
		baseURL:     baseURL,
		rateLimit:   rateLimit,
		robotsHosts: defaultRobotsHosts,
		robotsCache: map[string]robotsEntry{},
	}
}

// waitForRateLimit blocks until at least rateLimit has elapsed since the
// previous request began, or ctx is cancelled. It reserves the next slot
// under the mutex so callers stay correctly paced.
func (c *client) waitForRateLimit(ctx context.Context) error {
	c.mu.Lock()
	var wait time.Duration
	if !c.lastRequest.IsZero() {
		if elapsed := time.Since(c.lastRequest); elapsed < c.rateLimit {
			wait = c.rateLimit - elapsed
		}
	}
	c.lastRequest = time.Now().Add(wait)
	c.mu.Unlock()

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

// fetch performs a rate-limited GET of fullURL with the polite 점핏 headers
// and returns the response body and HTTP status code.
func (c *client) fetch(ctx context.Context, fullURL string) ([]byte, int, error) {
	if err := c.waitForRateLimit(ctx); err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("jumpit: build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Origin", originHeader)
	req.Header.Set("Referer", refererHeader)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("jumpit: GET %s: %w", fullURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("jumpit: read %s: %w", fullURL, err)
	}
	return body, resp.StatusCode, nil
}

// get fetches a path under baseURL (e.g. "/api/positions?...") and requires a
// 200 response.
func (c *client) get(ctx context.Context, path string) ([]byte, error) {
	body, status, err := c.fetch(ctx, c.baseURL+path)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("jumpit: GET %s: status %d", path, status)
	}
	return body, nil
}

// checkAccess reports whether scraping is currently permitted by robots.txt
// for every configured host. Verdicts are cached per host for robotsTTL. A
// host whose robots.txt cannot be fetched is treated as allowed — a transient
// robots.txt failure must not brick the user's tool.
func (c *client) checkAccess(ctx context.Context) error {
	for _, host := range c.robotsHosts {
		allowed, err := c.hostAllows(ctx, host)
		if err != nil {
			continue // could not determine — proceed
		}
		if !allowed {
			return fmt.Errorf("jumpit: %s robots.txt disallows %s", host, robotsCheckPath)
		}
	}
	return nil
}

// hostAllows reports whether host's robots.txt permits robotsCheckPath, using
// the per-host 24h cache. A non-200 response (e.g. a 404) means there is no
// enforceable policy, so scraping is allowed.
func (c *client) hostAllows(ctx context.Context, host string) (bool, error) {
	c.robotsMu.Lock()
	if e, ok := c.robotsCache[host]; ok && time.Now().Before(e.expiresAt) {
		c.robotsMu.Unlock()
		return e.allowed, nil
	}
	c.robotsMu.Unlock()

	body, status, err := c.fetch(ctx, host+"/robots.txt")
	if err != nil {
		return false, err
	}
	allowed := true
	if status == http.StatusOK {
		allowed = scraper.RobotsAllows(body, robotsCheckPath)
	}
	c.robotsMu.Lock()
	c.robotsCache[host] = robotsEntry{allowed: allowed, expiresAt: time.Now().Add(robotsTTL)}
	c.robotsMu.Unlock()
	return allowed, nil
}
