package daangn

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ohchanwu/job-scraper/internal/scraper"
)

// Source is the stable source identifier persisted on every Posting we
// produce and matched against the user's DisabledSources list.
const Source = "daangn"

const (
	defaultSiteURL    = "https://team.daangn.com"
	defaultAPIBaseURL = "https://boards-api.greenhouse.io"
	boardSlug         = "daangn"
	listingPath       = "/v1/boards/" + boardSlug + "/jobs?content=true"
	siteRobotsCheck   = "/jobs"
	apiRobotsCheck    = "/v1/boards/"
	userAgent         = "job-scraper/0.1 (+github.com/ohchanwu/job-scraper)"
	robotsTTL         = 24 * time.Hour
	requestTimeout    = 30 * time.Second
)

// Scraper is the 당근 implementation of scraper.Scraper.
type Scraper struct {
	client     *http.Client
	siteURL    string
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

// New returns a 당근 scraper paced at one request per second.
func New() *Scraper { return newScraper(defaultSiteURL, defaultAPIBaseURL, time.Second) }

func newScraper(siteURL, apiBaseURL string, rateLimit time.Duration) *Scraper {
	return &Scraper{
		client:     &http.Client{Timeout: requestTimeout},
		siteURL:    siteURL,
		apiBaseURL: apiBaseURL,
		rateLimit:  rateLimit,
	}
}

// Source returns the stable source identifier.
func (s *Scraper) Source() string { return Source }

// CheckAccess verifies robots.txt on BOTH hosts: team.daangn.com (the
// public-facing careers page that semantically corresponds to this
// scraper) and boards-api.greenhouse.io (where the actual requests
// land). Both are friendly to general crawling — the API host only
// disallows `/embed/`, which we never request.
func (s *Scraper) CheckAccess(ctx context.Context) error {
	if err := s.checkRobotsHost(ctx, s.siteURL, siteRobotsCheck, &s.siteRobotsMu, &s.siteRobots); err != nil {
		return fmt.Errorf("daangn: site robots: %w", err)
	}
	if err := s.checkRobotsHost(ctx, s.apiBaseURL, apiRobotsCheck, &s.robotsMu, &s.robotsCache); err != nil {
		return fmt.Errorf("daangn: api robots: %w", err)
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

// FetchListing pulls every 신입 IT-flagged 당근 posting in one call.
// The Greenhouse board API supports `?content=true` so we get the full
// HTML body alongside metadata in a single round trip.
func (s *Scraper) FetchListing(ctx context.Context, limit int) ([]scraper.Posting, error) {
	body, err := s.get(ctx, listingPath)
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

// FetchDetail is a no-op for 당근 — FetchListing already pulled every
// field this scraper reads thanks to `?content=true`.
func (s *Scraper) FetchDetail(_ context.Context, p scraper.Posting) (scraper.Posting, error) {
	return p, nil
}

func (s *Scraper) get(ctx context.Context, path string) ([]byte, error) {
	if err := s.waitForRateLimit(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.apiBaseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("daangn: build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("daangn: GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("daangn: read %s: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daangn: GET %s: status %d", path, resp.StatusCode)
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

// --- Parsing ---------------------------------------------------------------

type listingResponse struct {
	Jobs []json.RawMessage `json:"jobs"`
}

type ghJob struct {
	ID                  int64       `json:"id"`
	Title               string      `json:"title"`
	AbsoluteURL         string      `json:"absolute_url"`
	Location            *ghLocation `json:"location"`
	Content             string      `json:"content"`
	FirstPublished      string      `json:"first_published"`
	UpdatedAt           string      `json:"updated_at"`
	ApplicationDeadline *string     `json:"application_deadline"`
	Metadata            []ghMeta    `json:"metadata"`
}

type ghLocation struct {
	Name string `json:"name"`
}

// ghMeta value is either a string (single_select / short_text) or a
// bool (yes_no), so we keep it as `any` and type-switch when reading.
type ghMeta struct {
	Name      string `json:"name"`
	Value     any    `json:"value"`
	ValueType string `json:"value_type"`
}

// parseListing decodes the Greenhouse response, filters to 신입 IT, and
// converts each survivor to a Posting.
func parseListing(body []byte) ([]scraper.Posting, error) {
	var resp listingResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse listing JSON: %w", err)
	}
	out := make([]scraper.Posting, 0, len(resp.Jobs))
	for _, raw := range resp.Jobs {
		var j ghJob
		if err := json.Unmarshal(raw, &j); err != nil {
			return nil, fmt.Errorf("parse job row: %w", err)
		}
		md := indexMetadata(j.Metadata)
		if !isShinipIT(md) {
			continue
		}
		out = append(out, normalizeJob(j, md, raw))
	}
	return out, nil
}

// indexMetadata flattens the metadata array into a name→value map for
// O(1) lookup downstream. Values keep their original `any` type so
// callers can distinguish bool from string.
func indexMetadata(meta []ghMeta) map[string]any {
	out := make(map[string]any, len(meta))
	for _, m := range meta {
		out[m.Name] = m.Value
	}
	return out
}

// isShinipIT applies the 신입 IT filter to one job. Both conditions
// must hold:
//
//  1. `Engineer == true` (yes_no metadata field).
//  2. `Prior Experience` contains "신입" (matches `신입` and `신입/경력`,
//     skips `경력`).
//
// See API_NOTES.md for why we accept `신입/경력`.
func isShinipIT(md map[string]any) bool {
	if v, ok := md["Engineer"].(bool); !ok || !v {
		return false
	}
	pe, ok := md["Prior Experience"].(string)
	if !ok {
		return false
	}
	return strings.Contains(pe, "신입")
}

// normalizeJob maps a Greenhouse job into the project's shared Posting
// model. `raw` is the original JSON bytes — stored on RawJSON so a
// future parser can lift additional fields without re-scraping.
func normalizeJob(j ghJob, md map[string]any, raw json.RawMessage) scraper.Posting {
	id := strconv.FormatInt(j.ID, 10)

	location := ""
	if j.Location != nil {
		location = normalizeLocation(j.Location.Name)
	}

	url := j.AbsoluteURL
	if url == "" {
		url = "https://about.daangn.com?gh_jid=" + id
	}

	company := stringMeta(md, "Corporate")
	if company == "" {
		company = "당근"
	}

	pe, _ := md["Prior Experience"].(string)
	minC, maxC := experienceBounds(pe)

	out := scraper.Posting{
		Source:          Source,
		SourcePostingID: id,
		URL:             url,
		Title:           strings.TrimSpace(j.Title),
		Company:         company,
		Location:        location,
		Newcomer:        true, // every survivor has Prior Experience containing 신입
		MinCareer:       minC,
		MaxCareer:       maxC,
		CareerLevel:     careerLabel(pe),
		StackTags:       []string{},
		Tags:            buildTags(md),
		Description:     composeDescription(j, md),
		RawJSON:         string(raw),
	}
	if t, ok := parseTimestamp(j.FirstPublished); ok {
		out.PublishedAt = &t
	}
	if j.ApplicationDeadline == nil || *j.ApplicationDeadline == "" {
		out.AlwaysOpen = true
	} else if t, ok := parseTimestamp(*j.ApplicationDeadline); ok {
		out.ClosedAt = &t
	}
	return out
}

// experienceBounds turns the `Prior Experience` string into the
// (min, max) pair Posting carries. Pure-신입 maps to (0, 0); the
// mixed `신입/경력` admits up to a couple years of experience without
// over-promising the upper bound.
func experienceBounds(priorExp string) (min, max int) {
	switch strings.TrimSpace(priorExp) {
	case "신입":
		return 0, 0
	case "신입/경력":
		return 0, 3
	default:
		return 0, 0
	}
}

func careerLabel(priorExp string) string {
	switch strings.TrimSpace(priorExp) {
	case "신입":
		return "신입"
	case "신입/경력":
		return "신입/경력"
	default:
		return strings.TrimSpace(priorExp)
	}
}

func stringMeta(md map[string]any, name string) string {
	if v, ok := md[name].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func boolMeta(md map[string]any, name string) bool {
	v, _ := md[name].(bool)
	return v
}

// buildTags exposes the structured Greenhouse metadata fields that
// downstream scoring or display can use. Employment Type lands as a
// dedicated tag; `Alternative Civilian Service = true` registers as
// a welfare-category `병역특례 가능` tag so the project's existing
// `병특` dealbreaker matcher reacts to it consistently.
func buildTags(md map[string]any) []scraper.Tag {
	tags := []scraper.Tag{}
	if et := stringMeta(md, "Employment Type"); et != "" {
		tags = append(tags, scraper.Tag{Name: et, Category: "employment_type"})
	}
	if boolMeta(md, "Alternative Civilian Service") {
		tags = append(tags, scraper.Tag{Name: "병역특례 가능", Category: "welfare"})
	}
	return tags
}

// composeDescription folds the HTML-stripped content plus a handful of
// structured metadata into a single FTS-indexable blob. The content is
// the primary signal; metadata is appended at the end so it shows up
// in matches without dominating the first paragraph.
func composeDescription(j ghJob, md map[string]any) string {
	var parts []string
	if j.Content != "" {
		parts = append(parts, stripHTML(html.UnescapeString(j.Content)))
	}
	if v := stringMeta(md, "Corporate"); v != "" {
		parts = append(parts, "소속: "+v)
	}
	if v := stringMeta(md, "Employment Type"); v != "" {
		parts = append(parts, "고용형태: "+v)
	}
	if v := stringMeta(md, "Keywords"); v != "" {
		parts = append(parts, "키워드: "+v)
	}
	if v := stringMeta(md, "동료의 한마디"); v != "" {
		parts = append(parts, "동료의 한마디: "+v)
	}
	if boolMeta(md, "Alternative Civilian Service") {
		parts = append(parts, "병역특례 가능")
	}
	return strings.Join(parts, "\n\n")
}

// normalizeLocation maps Greenhouse's locations to short Korean labels
// the rest of the app expects ("서울 강남구"-style). 당근 currently
// returns "SEOUL" for every posting; we collapse that to "서울" since
// the Greenhouse field carries no district granularity.
func normalizeLocation(s string) string {
	s = strings.TrimSpace(s)
	switch strings.ToUpper(s) {
	case "SEOUL":
		return "서울"
	default:
		return s
	}
}

// --- Dates -----------------------------------------------------------------

// parseTimestamp reads Greenhouse's ISO8601 timestamps (with timezone
// or trailing Z) and returns UTC.
func parseTimestamp(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05-07:00",
		"2006-01-02T15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// --- HTML stripping --------------------------------------------------------

var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)
var htmlWhitespacePattern = regexp.MustCompile(`\s+`)

// stripHTML drops tags and collapses whitespace. Greenhouse content
// arrives HTML-escaped (`&lt;`, `&amp;`, `&nbsp;`); callers should
// run html.UnescapeString first so the tag-strip sees real angle
// brackets to remove.
func stripHTML(s string) string {
	noTags := htmlTagPattern.ReplaceAllString(s, " ")
	collapsed := htmlWhitespacePattern.ReplaceAllString(noTags, " ")
	return strings.TrimSpace(collapsed)
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
