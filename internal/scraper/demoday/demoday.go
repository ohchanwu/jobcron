package demoday

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ohchanwu/jobcron/internal/pacing"
	"github.com/ohchanwu/jobcron/internal/scraper"
)

// Source is the stable source identifier persisted on every Posting we
// produce and matched against the user's DisabledSources list.
const Source = "demoday"

const (
	defaultSiteURL    = "https://demoday.co.kr"
	defaultAPIBaseURL = "https://xypsryijdllrhfctnehy.supabase.co"
	listingPath       = "/rest/v1/recruits"
	recruitsRobots    = "/rest/v1/recruits"
	siteRobotsCheck   = "/recruits"
	userAgent         = "jobcron/0.1 (+github.com/ohchanwu/jobcron)"
	robotsTTL         = 24 * time.Hour
	requestTimeout    = 30 * time.Second
)

// bakedInSupabaseAnonKey is the Supabase project's anonymous API key as
// it was when shipped. Embedded in the 데모데이 page bundle and required
// on every REST call (both as the `apikey` header and as a Bearer
// token). It's publicly visible — it's not a credential leak, it's a
// maintainability target: when 데모데이 rotates the key, every user's
// scrape will 401 until the binary is updated.
//
// The scraper picks the key in this order:
//  1. JOBCRON_DEMODAY_ANON_KEY env var, if set and non-empty.
//  2. This baked-in constant otherwise.
//
// The env-var path lets the user paste a fresh key from a current
// demoday.co.kr page bundle and keep the scraper working without
// waiting for a project release. The 401 handler in `get` surfaces
// both paths in the error message.
const bakedInSupabaseAnonKey = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9." +
	"eyJpc3MiOiJzdXBhYmFzZSIsInJlZiI6Inh5cHNyeWlqZGxscmhmY3RuZWh5Iiwicm9sZSI6ImFub24iLCJpYXQiOjE3NDk3MDc4MzcsImV4cCI6MjA2NTI4MzgzN30." +
	"cYTA9nOmjVbwVF784xr8BjGK1pkyFAA4_aQzfV73LhU"

// anonKeyEnvVar is the env var that overrides the baked-in key.
const anonKeyEnvVar = "JOBCRON_DEMODAY_ANON_KEY"

// resolveAnonKey returns the env-var key when set, falling back to the
// baked-in default. Pulled into a function so tests can swap it.
func resolveAnonKey() string {
	if v := strings.TrimSpace(os.Getenv(anonKeyEnvVar)); v != "" {
		return v
	}
	return bakedInSupabaseAnonKey
}

// experienceLevels is the set of `experience_level` enum values the
// scraper accepts as 신입-relevant. `entry` is unambiguously new-grad;
// `1-3` is the closest "junior" bucket. `any` carries no preference and
// passes through the same IT/SWE post-filter as the other buckets. See
// API_NOTES.md for the bucket distribution.
var experienceLevels = []string{"entry", "1-3", "any"}

// itPositionCategories is the set of `position_tags[0]` values 데모데이
// uses for IT/SWE roles. Every record carries an ordered position_tags
// array whose first element is the top-level job family — these three
// values cover ~95% of the SWE-relevant population with very few false
// positives. See tmp/demoday_audit_2026-05-28.md for the 1000-row audit.
var itPositionCategories = map[string]struct{}{
	"개발":    {},
	"게임 제작": {},
	"정보보호":  {},
}

// keepsITSWE reports whether a 데모데이 row should survive the IT/SWE
// post-filter. The rule is shared across all experience buckets (entry,
// 1-3, any) because the recruit table is a general-purpose job board —
// every bucket is dominated by non-SWE roles (marketing, MD, sales, HR,
// hardware engineering) that the user does not want in their briefing.
//
// A row is kept iff:
//
//  1. The title/position do NOT carry an explicit 4+ year experience
//     demand (시니어, 5년 이상, etc.), AND
//  2. EITHER positionTags[0] is one of the IT/SWE job-family categories
//     (개발, 게임 제작, 정보보호 — see itPositionCategories), OR — when
//     positionTags is empty/missing — the title+position match the
//     fallback IT keyword filter.
//
// The position-tags signal is preferred because it's structured upstream
// data: "engineer" / "엔지니어" in the keyword filter matches mechanical,
// RF, and aerospace engineers too. position_tags[0] sorts those into
// "엔지니어링·설계" and out of our scope cleanly.
func keepsITSWE(positionTags []string, title, position string) bool {
	if minY, _, ok := scraper.ParseExperienceYears(title, position); ok && minY >= 4 {
		return false
	}
	if len(positionTags) > 0 && strings.TrimSpace(positionTags[0]) != "" {
		_, ok := itPositionCategories[strings.TrimSpace(positionTags[0])]
		return ok
	}
	return scraper.HasDevKeyword(title + " " + position)
}

// Scraper is the 데모데이 implementation of scraper.Scraper.
type Scraper struct {
	client     *http.Client
	siteURL    string
	apiBaseURL string
	anonKey    string
	pacer      *pacing.Pacer

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

// New returns a 데모데이 scraper paced at one request per second.
func New() *Scraper { return newScraper(defaultSiteURL, defaultAPIBaseURL, time.Second) }

func newScraper(siteURL, apiBaseURL string, rateLimit time.Duration) *Scraper {
	return &Scraper{
		client:     &http.Client{Timeout: requestTimeout},
		siteURL:    siteURL,
		apiBaseURL: apiBaseURL,
		anonKey:    resolveAnonKey(),
		pacer:      pacing.New(rateLimit),
	}
}

// Source returns the stable source identifier.
func (s *Scraper) Source() string { return Source }

// Kind reports that 데모데이 is a multi-company aggregator (startups).
func (s *Scraper) Kind() scraper.SourceKind { return scraper.SourceKindAggregator }

// CheckAccess verifies robots.txt on BOTH hosts the scraper touches: the
// public demoday.co.kr site (whose preferences this scraper effectively
// represents from the user's mental model) and the Supabase API host
// (whose paths the scraper actually requests). Mirrors the 점핏 pattern
// of per-host robots checks.
//
// demoday.co.kr disallows `/api/` and `/_next/` for `User-Agent: *`, but
// `/recruits` (the public-facing listings page our requests semantically
// correspond to) is unrestricted. The Supabase host's robots.txt
// historically 404s, which RFC 9309 reads as unrestricted.
func (s *Scraper) CheckAccess(ctx context.Context) error {
	if err := s.checkRobotsHost(ctx, s.siteURL, siteRobotsCheck, &s.siteRobotsMu, &s.siteRobots); err != nil {
		return fmt.Errorf("demoday: site robots: %w", err)
	}
	if err := s.checkRobotsHost(ctx, s.apiBaseURL, recruitsRobots, &s.robotsMu, &s.robotsCache); err != nil {
		return fmt.Errorf("demoday: api robots: %w", err)
	}
	return nil
}

// checkRobotsHost fetches and parses a single host's robots.txt, caching
// the allow/deny verdict for 24h. A network error returns nil with a
// cached "allow" — same posture as every other scraper in the project,
// where a transient failure must not brick the user's daily briefing.
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
		allowed = scraper.RobotsAllows(body, path)
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

// FetchListing pulls 신입-relevant postings in one Supabase query.
// Because the response carries the full record, FetchDetail is a no-op.
// The `limit` parameter clamps the returned slice for testing only;
// production callers pass 0 to mean "no cap."
func (s *Scraper) FetchListing(ctx context.Context, limit int) ([]scraper.Posting, error) {
	q := url.Values{}
	q.Set("select", "*")
	q.Set("status", "eq.published")
	q.Set("is_active", "eq.true")
	q.Set("experience_level", "in.("+strings.Join(experienceLevels, ",")+")")
	q.Set("order", "created_at.desc")

	body, err := s.get(ctx, listingPath+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	postings, err := parseListing(body, s.siteURL)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(postings) > limit {
		postings = postings[:limit]
	}
	return postings, nil
}

// FetchDetail is a no-op for 데모데이 — FetchListing already pulled every
// field this scraper reads. The bulk Supabase response is cheap enough
// that a per-posting detail round trip would add latency without value.
func (s *Scraper) FetchDetail(_ context.Context, p scraper.Posting) (scraper.Posting, error) {
	return p, nil
}

// get is the rate-limited GET helper. Adds Supabase's required auth
// headers (anon key in both `apikey` and `Authorization`) plus the
// `Accept-Profile: public` header that pins the request to the public
// PostgREST schema.
func (s *Scraper) get(ctx context.Context, path string) ([]byte, error) {
	if err := s.pacer.Wait(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.apiBaseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("demoday: build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Profile", "public")
	req.Header.Set("apikey", s.anonKey)
	req.Header.Set("Authorization", "Bearer "+s.anonKey)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("demoday: GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("demoday: read %s: %w", path, err)
	}
	// Supabase returns 200 for "full result" and 206 for "partial content"
	// when the result was capped by its server-side row limit. Both are
	// success cases for us — we only requested a small set and use the
	// first 1000 rows in either case.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf(
				"demoday: GET %s: status 401 — 데모데이's Supabase anon key likely rotated. "+
					"Refresh it by either (a) setting %s to the new key (visible in demoday.co.kr's page bundle), "+
					"or (b) updating bakedInSupabaseAnonKey in internal/scraper/demoday/demoday.go and rebuilding.",
				path, anonKeyEnvVar)
		}
		return nil, fmt.Errorf("demoday: GET %s: status %d", path, resp.StatusCode)
	}
	return body, nil
}

// --- Parsing ---------------------------------------------------------------

// supabaseRecruit is the wire shape we read out of the `recruits` row.
// Only the fields the scraper actually uses are spelled out; everything
// else stays in the raw JSON we keep on Posting.RawJSON for forward
// compatibility.
type supabaseRecruit struct {
	ID                  int64    `json:"id"`
	Title               string   `json:"title"`
	Position            string   `json:"position"`
	Content             string   `json:"content"`
	Excerpt             string   `json:"excerpt"`
	CompanyName         string   `json:"company_name"`
	Location            string   `json:"location"`
	ExperienceLevel     string   `json:"experience_level"`
	ApplicationDeadline *string  `json:"application_deadline"`
	CreatedAt           string   `json:"created_at"`
	SkillTags           []string `json:"skill_tags"`
	PositionTags        []string `json:"position_tags"`
	LocationTags        []string `json:"location_tags"`
	EmploymentType      string   `json:"employment_type"`
}

// parseListing converts a Supabase JSON array response into Postings.
// Each raw record's full JSON is preserved on the Posting so a future
// parser can lift additional fields without re-scraping.
func parseListing(body []byte, siteURL string) ([]scraper.Posting, error) {
	var raws []json.RawMessage
	if err := json.Unmarshal(body, &raws); err != nil {
		return nil, fmt.Errorf("parse listing JSON: %w", err)
	}
	out := make([]scraper.Posting, 0, len(raws))
	for _, raw := range raws {
		var r supabaseRecruit
		if err := json.Unmarshal(raw, &r); err != nil {
			return nil, fmt.Errorf("parse recruit row: %w", err)
		}
		// Every bucket (entry / 1-3 / any) gets the same IT/SWE filter —
		// each is dominated by non-SWE roles (marketing, MD, sales,
		// hardware engineering) the user doesn't want. See keepsITSWE
		// for the rule (position_tags[0] primary, keyword fallback).
		if !keepsITSWE(r.PositionTags, r.Title, r.Position) {
			continue
		}
		p := normalizeRecruit(r, raw, siteURL)
		out = append(out, p)
	}
	return out, nil
}

// normalizeRecruit maps a parsed Supabase row to the project's shared
// Posting model.
func normalizeRecruit(r supabaseRecruit, raw json.RawMessage, siteURL string) scraper.Posting {
	id := strconv.FormatInt(r.ID, 10)
	min, max, newcomer := experienceBounds(r.ExperienceLevel)
	p := scraper.Posting{
		Source:          Source,
		SourcePostingID: id,
		URL:             siteURL + "/recruits/" + id,
		Title:           strings.TrimSpace(r.Title),
		Company:         strings.TrimSpace(r.CompanyName),
		Location:        strings.TrimSpace(r.Location),
		Newcomer:        newcomer,
		MinCareer:       min,
		MaxCareer:       max,
		CareerLevel:     careerLevelLabel(r.ExperienceLevel),
		StackTags:       nonEmptyStrings(r.SkillTags),
		Tags:            buildTags(r),
		Description:     composeDescription(r),
		RawJSON:         string(raw),
	}
	if t, ok := parseTimestamp(r.CreatedAt); ok {
		p.PublishedAt = &t
	}
	if r.ApplicationDeadline == nil || *r.ApplicationDeadline == "" {
		p.AlwaysOpen = true
	} else if t, ok := parseDate(*r.ApplicationDeadline); ok {
		p.ClosedAt = &t
	}
	return p
}

// experienceBounds translates a `experience_level` enum value into the
// (min, max, newcomer) triple Posting carries.
//
//	"entry" → newcomer-true, 0-0
//	"1-3"   → newcomer-false, 1-3 (a 신입 user near-misses; the scoring
//	          engine already handles the adjacent bracket)
//	"any"   → newcomer-true, 0-∞ (no preference; the bucket is gated to
//	          IT/SWE rows by keepsITSWE before we get here)
//
// Other values would have been filtered out at query time; if a new
// enum value slips through, we conservatively flag it as 1-3.
func experienceBounds(level string) (min, max int, newcomer bool) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "entry":
		return 0, 0, true
	case "any":
		return 0, anyBucketMaxYears, true
	case "1-3":
		return 1, 3, false
	default:
		return 1, 3, false
	}
}

// anyBucketMaxYears is the synthetic upper bound for `any`-bucket rows.
// Identical in spirit to the experienceUpperOpen const used in the
// general parser — chosen to read as "no upper bound" while staying a
// finite integer the scoring engine handles cleanly.
const anyBucketMaxYears = 99

// careerLevelLabel renders the source's experience_level as a short
// Korean label for the CareerLevel field. Kept terse — the UI shows
// "신입" or a year range, not the raw enum.
func careerLevelLabel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "entry":
		return "신입"
	case "1-3":
		return "1-3년"
	case "any":
		return "경력 무관"
	default:
		return level
	}
}

// buildTags turns the structured Supabase tag arrays into the project's
// Tag type. position_tags are the most useful signal for matching;
// location_tags duplicate the Location field but are kept for symmetry
// with the source data.
func buildTags(r supabaseRecruit) []scraper.Tag {
	tags := make([]scraper.Tag, 0,
		len(r.PositionTags)+len(r.LocationTags))
	for _, t := range r.PositionTags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		tags = append(tags, scraper.Tag{Name: t, Category: "position"})
	}
	for _, t := range r.LocationTags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		tags = append(tags, scraper.Tag{Name: t, Category: "location"})
	}
	if r.EmploymentType != "" {
		tags = append(tags, scraper.Tag{
			Name:     r.EmploymentType,
			Category: "employment_type",
		})
	}
	return tags
}

// composeDescription concatenates the human-readable fields into a
// single FTS-indexable blob. HTML content and excerpt are stripped to
// plain text first; double newlines mark soft section boundaries.
func composeDescription(r supabaseRecruit) string {
	var parts []string
	if r.Position != "" {
		parts = append(parts, "직무: "+strings.TrimSpace(r.Position))
	}
	if r.Excerpt != "" {
		parts = append(parts, stripHTML(r.Excerpt))
	}
	if r.Content != "" {
		parts = append(parts, stripHTML(r.Content))
	}
	return strings.Join(parts, "\n\n")
}

// nonEmptyStrings returns a new slice with empty/blank entries removed.
// Used because Supabase tag arrays occasionally include empty strings.
func nonEmptyStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// --- Dates -----------------------------------------------------------------

var kstZone = time.FixedZone("KST", 9*60*60)

// parseTimestamp reads a Supabase ISO timestamp ("2026-05-27T00:05:19+00:00"
// or "2026-05-27T00:05:19.123456+00:00") and returns UTC.
func parseTimestamp(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999-07:00",
		"2006-01-02T15:04:05-07:00",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// parseDate reads a "YYYY-MM-DD" Supabase date column as KST midnight,
// returning UTC. application_deadline is stored as a date-only column.
func parseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("2006-01-02", s, kstZone)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

// --- HTML stripping --------------------------------------------------------

var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)
var htmlWhitespacePattern = regexp.MustCompile(`\s+`)

// stripHTML drops every HTML tag and collapses runs of whitespace. Used
// on `content` and `excerpt`, which arrive as CKEditor-style HTML. Very
// simple — no entity decoding, no script removal — because we only feed
// the result into FTS-style token matching downstream.
func stripHTML(s string) string {
	noTags := htmlTagPattern.ReplaceAllString(s, " ")
	collapsed := htmlWhitespacePattern.ReplaceAllString(noTags, " ")
	return strings.TrimSpace(collapsed)
}
