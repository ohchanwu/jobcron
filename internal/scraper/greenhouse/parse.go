package greenhouse

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ohchanwu/job-scraper/internal/scraper"
)

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

// ghMeta value is either a string (single_select / short_text) or a bool
// (yes_no), so we keep it as `any` and type-switch when reading.
type ghMeta struct {
	Name      string `json:"name"`
	Value     any    `json:"value"`
	ValueType string `json:"value_type"`
}

// parseListing decodes the Greenhouse response, keeps the 신입 IT postings
// per the tenant's detection strategy, and converts each survivor to a
// Posting.
func parseListing(body []byte, t Tenant) ([]scraper.Posting, error) {
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
		det := classify(t.Detect, j, md)
		if !det.keep {
			continue
		}
		out = append(out, normalizeJob(j, md, raw, t, det))
	}
	return out, nil
}

// indexMetadata flattens the metadata array into a name→value map for O(1)
// lookup. Values keep their original `any` type so callers can distinguish
// bool from string.
func indexMetadata(meta []ghMeta) map[string]any {
	out := make(map[string]any, len(meta))
	for _, m := range meta {
		out[m.Name] = m.Value
	}
	return out
}

// normalizeJob maps a Greenhouse job into the project's shared Posting
// model. raw is the original JSON bytes — stored on RawJSON so a future
// parser can lift additional fields without re-scraping.
func normalizeJob(j ghJob, md map[string]any, raw json.RawMessage, t Tenant, det detection) scraper.Posting {
	id := strconv.FormatInt(j.ID, 10)

	location := ""
	if j.Location != nil {
		location = normalizeLocation(j.Location.Name)
	}

	// 당근 carries a per-posting sub-company in "Corporate" metadata; the
	// heuristic tenants do not, so they fall back to the tenant's name.
	company := t.Company
	if c := stringMeta(md, "Corporate"); c != "" {
		company = c
	}

	out := scraper.Posting{
		Source:          t.Source,
		SourcePostingID: id,
		URL:             t.link(id, j.AbsoluteURL),
		Title:           strings.TrimSpace(j.Title),
		Company:         company,
		Location:        location,
		Newcomer:        det.newcomer,
		MinCareer:       det.minCareer,
		MaxCareer:       det.maxCareer,
		CareerLevel:     det.careerLevel,
		StackTags:       []string{},
		Tags:            buildTags(md),
		Description:     composeDescription(j, md),
		RawJSON:         string(raw),
	}
	if pub, ok := parseTimestamp(j.FirstPublished); ok {
		out.PublishedAt = &pub
	}
	if j.ApplicationDeadline == nil || *j.ApplicationDeadline == "" {
		out.AlwaysOpen = true
	} else if cl, ok := parseTimestamp(*j.ApplicationDeadline); ok {
		out.ClosedAt = &cl
	}
	return out
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

// buildTags exposes the structured Greenhouse metadata fields downstream
// scoring or display can use. Both fields are 당근-specific; on boards that
// lack them this returns an empty slice. `Alternative Civilian Service =
// true` registers as a welfare-category 병역특례 가능 tag so the project's
// existing 병특 matcher reacts to it consistently.
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
// structured metadata fields into a single FTS-indexable blob. The content
// is the primary signal; metadata (all 당근-specific, absent elsewhere) is
// appended so it shows up in matches without dominating the first
// paragraph.
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

// normalizeLocation maps Greenhouse's locations to short Korean labels the
// rest of the app expects. 당근 returns "SEOUL"; the heuristic tenants
// return richer strings like "Seoul, Korea" which we pass through after
// collapsing the common SEOUL-only case.
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

// parseTimestamp reads Greenhouse's ISO8601 timestamps (with timezone or
// trailing Z) and returns UTC.
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
// arrives HTML-escaped (&lt;, &amp;, &nbsp;); callers should run
// html.UnescapeString first so the tag-strip sees real angle brackets.
func stripHTML(s string) string {
	noTags := htmlTagPattern.ReplaceAllString(s, " ")
	collapsed := htmlWhitespacePattern.ReplaceAllString(noTags, " ")
	return strings.TrimSpace(collapsed)
}
