package rallit

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
)

const (
	// defaultBaseURL is the 랠릿 site host. The API lives under /api/v1.
	defaultBaseURL = "https://www.rallit.com"

	// userAgent matches the etiquette used by the 점핏 / 워크넷 clients.
	userAgent = "job-scraper/0.1 (+github.com/ohchanwu/jobcron)"

	// robotsTTL caches the robots.txt verdict for one day.
	robotsTTL = 24 * time.Hour

	// robotsCheckPath is the path we validate against robots.txt; both
	// endpoints we hit (listing + detail) sit under /api/v1/position.
	robotsCheckPath = "/api/v1/position"
)

// client is the low-level 랠릿 HTTP client — rate-limited and robots-aware.
type client struct {
	http      *http.Client
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

// newClient returns a 랠릿 client targeting baseURL. rateLimit is the
// minimum spacing enforced between requests; pass 0 in tests.
func newClient(baseURL string, rateLimit time.Duration) *client {
	return &client{
		http:      &http.Client{Timeout: 30 * time.Second},
		baseURL:   baseURL,
		rateLimit: rateLimit,
	}
}

// waitForRateLimit blocks until at least rateLimit has elapsed since the
// previous request began, or ctx is cancelled. Reserves the next slot
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

// get fetches path (e.g. "/api/v1/position?pageNumber=1") and requires a
// 200 response.
func (c *client) get(ctx context.Context, path string) ([]byte, error) {
	if err := c.waitForRateLimit(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("rallit: build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rallit: GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("rallit: read %s: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rallit: GET %s: status %d", path, resp.StatusCode)
	}
	return body, nil
}

// checkAccess verifies that robots.txt permits robotsCheckPath. A non-200
// robots response is treated as unrestricted, matching Jumpit's handling.
// Cached for robotsTTL.
func (c *client) checkAccess(ctx context.Context) error {
	c.robotsMu.Lock()
	if c.robotsCache != nil && time.Now().Before(c.robotsCache.expiresAt) {
		allowed := c.robotsCache.allowed
		c.robotsMu.Unlock()
		if !allowed {
			return fmt.Errorf("rallit: robots.txt disallows %s", robotsCheckPath)
		}
		return nil
	}
	c.robotsMu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/robots.txt", nil)
	if err != nil {
		return fmt.Errorf("rallit: build robots request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		// Transient robots.txt failure must not brick the user's tool.
		c.cacheRobots(true)
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	allowed := true
	if resp.StatusCode == http.StatusOK {
		allowed = robotsAllows(body, robotsCheckPath)
	}
	c.cacheRobots(allowed)
	if !allowed {
		return fmt.Errorf("rallit: robots.txt disallows %s", robotsCheckPath)
	}
	return nil
}

func (c *client) cacheRobots(allowed bool) {
	c.robotsMu.Lock()
	c.robotsCache = &robotsEntry{allowed: allowed, expiresAt: time.Now().Add(robotsTTL)}
	c.robotsMu.Unlock()
}

// robotsAllows reports whether path is allowed for our scraper by the given
// robots.txt content. It honors the "User-agent: *" group with prefix-match
// Disallow/Allow rules and longest-match-wins — a pragmatic subset of
// RFC 9309. Identical semantics to the Jumpit scraper's implementation.
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
	blocked := longestPrefixMatch(disallow, path)
	if blocked == 0 {
		return true
	}
	return longestPrefixMatch(allow, path) >= blocked
}

// longestPrefixMatch returns the length of the longest rule that is a prefix
// of path, or 0 when none match.
func longestPrefixMatch(rules []string, path string) int {
	best := 0
	for _, r := range rules {
		if len(r) > best && strings.HasPrefix(path, r) {
			best = len(r)
		}
	}
	return best
}
