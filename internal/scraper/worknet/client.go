package worknet

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ohchanwu/jobcron/internal/pacing"
)

const (
	// defaultBaseURL is the 워크넷 OpenAPI host. The endpoint serves over
	// plain HTTP (TLS is not offered on this host as of registration).
	defaultBaseURL = "http://openapi.work.go.kr"

	// wantedPath is the single endpoint that serves both listing and detail
	// calls, distinguished by the callTp parameter.
	wantedPath = "/opi/opi/opia/wantedApi.do"

	// userAgent identifies jobcron politely, matching the Jumpit
	// scraper's etiquette.
	userAgent = "jobcron/0.1 (+github.com/ohchanwu/jobcron)"

	// robotsTTL is how long a robots.txt verdict is cached.
	robotsTTL = 24 * time.Hour
)

// client is the low-level 워크넷 HTTP client — rate-limited, robots-aware,
// and key-redacted. The authKey lives only inside this client so other
// packages cannot accidentally log it.
type client struct {
	http    *http.Client
	baseURL string
	authKey string
	pacer   *pacing.Pacer

	robotsMu    sync.Mutex
	robotsCache *robotsEntry
}

// robotsEntry is the cached robots.txt verdict for the API host.
type robotsEntry struct {
	allowed   bool
	expiresAt time.Time
}

// newClient returns a 워크넷 client targeting baseURL with the given key.
// rateLimit is the minimum spacing enforced between requests; pass 0 in
// tests to disable pacing.
func newClient(baseURL, authKey string, rateLimit time.Duration) *client {
	return &client{
		http:    &http.Client{Timeout: 30 * time.Second},
		baseURL: baseURL,
		authKey: authKey,
		pacer:   pacing.New(rateLimit),
	}
}

// call performs one rate-limited GET against the wanted endpoint with the
// given pre-built parameter set. The authKey is injected here so it never
// appears in caller-built strings (and therefore never in error messages
// or logs). errors redact the key from the URL.
func (c *client) call(ctx context.Context, params url.Values) ([]byte, error) {
	if err := c.pacer.Wait(ctx); err != nil {
		return nil, err
	}
	q := url.Values{}
	for k, v := range params {
		q[k] = v
	}
	q.Set("authKey", c.authKey)
	full := c.baseURL + wantedPath + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, fmt.Errorf("worknet: build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/xml,text/xml")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("worknet: GET %s: %w", redact(full, c.authKey), err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("worknet: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("worknet: GET %s: status %d",
			redact(full, c.authKey), resp.StatusCode)
	}
	return body, nil
}

// checkAccess fetches robots.txt for the API host. A non-200 (typical for
// this endpoint, whose robots.txt 404s) is treated as unrestricted per
// RFC 9309 — mirrors the Jumpit handling of jumpit-api.saramin.co.kr.
// Verdicts are cached for robotsTTL.
func (c *client) checkAccess(ctx context.Context) error {
	c.robotsMu.Lock()
	if c.robotsCache != nil && time.Now().Before(c.robotsCache.expiresAt) {
		allowed := c.robotsCache.allowed
		c.robotsMu.Unlock()
		if !allowed {
			return fmt.Errorf("worknet: robots.txt disallows %s", wantedPath)
		}
		return nil
	}
	c.robotsMu.Unlock()

	// We do not need to honor the response shape here — a 404 means "no
	// policy, scraping permitted." A 200 with an explicit Disallow would
	// be the only block case; the public-data portal's API host has never
	// served one. If that ever changes we will see "robots.txt disallows
	// …" surface in the SSE error event and can revisit.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/robots.txt", nil)
	if err != nil {
		return fmt.Errorf("worknet: build robots request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		// Transient robots.txt failure must not brick the user's tool —
		// match Jumpit's behavior.
		c.cacheRobots(true)
		return nil
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	allowed := resp.StatusCode != http.StatusOK // any 200 is suspicious; 404 is the norm
	if resp.StatusCode == http.StatusOK {
		// Conservative: a real robots.txt body would need parsing. The
		// 워크넷 API host has not served one historically, so we treat
		// a 200 here as a future event worth surfacing.
		c.cacheRobots(false)
		return fmt.Errorf("worknet: robots.txt now served — re-check policy before next release")
	}
	c.cacheRobots(allowed)
	return nil
}

func (c *client) cacheRobots(allowed bool) {
	c.robotsMu.Lock()
	c.robotsCache = &robotsEntry{allowed: allowed, expiresAt: time.Now().Add(robotsTTL)}
	c.robotsMu.Unlock()
}

// redact strips the authKey from a URL so it never lands in an error
// message or log line. Returns the original URL unchanged when key is empty.
func redact(s, key string) string {
	if key == "" {
		return s
	}
	return strings.ReplaceAll(s, key, "[REDACTED]")
}
