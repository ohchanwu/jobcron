package rallit

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ohchanwu/job-scraper/internal/scraper"
)

// envelope is the shared response wrapper for every rallit endpoint. We
// reject non-OK responses up front rather than threading the error through
// every parser — the data shape behind a non-OK envelope is unstable.
type envelope struct {
	StatusCode string          `json:"statusCode"`
	Message    string          `json:"message"`
	Data       json.RawMessage `json:"data"`
}

// listingData is the data payload of a listing response.
type listingData struct {
	PageNumber int           `json:"pageNumber"`
	PageSize   int           `json:"pageSize"`
	TotalCount int           `json:"totalCount"`
	TotalPage  int           `json:"totalPage"`
	Items      []listingItem `json:"items"`
}

// listingItem is one summary posting in a listing response. We deliberately
// only decode fields we use — extra keys in the live response are ignored.
type listingItem struct {
	ID               int      `json:"id"`
	Title            string   `json:"title"`
	JobLevel         string   `json:"jobLevel"`
	JobLevels        []string `json:"jobLevels"`
	StartedAt        string   `json:"startedAt"` // YYYY-MM-DD
	EndedAt          string   `json:"endedAt"`   // YYYY-MM-DD; 9999-12-31 = always-open
	CompanyID        int      `json:"companyId"`
	CompanyName      string   `json:"companyName"`
	AddressRegion    string   `json:"addressRegion"`
	URL              string   `json:"url"`
	JobSkillKeywords []string `json:"jobSkillKeywords"`
}

// detailData is the data payload of a detail response — same shape as a
// listing item plus several long-form rich-text fields and the
// isAlwaysHiring boolean.
type detailData struct {
	ID                  int      `json:"id"`
	Title               string   `json:"title"`
	JobLevel            string   `json:"jobLevel"`
	JobLevels           []string `json:"jobLevels"`
	StartedAt           string   `json:"startedAt"`
	EndedAt             string   `json:"endedAt"`
	IsAlwaysHiring      bool     `json:"isAlwaysHiring"`
	CompanyID           int      `json:"companyId"`
	CompanyName         string   `json:"companyName"`
	AddressMain         string   `json:"addressMain"`
	AddressDetail       string   `json:"addressDetail"`
	AddressBuildingName string   `json:"addressBuildingName"`
	// addressRegion is intentionally NOT decoded here — the detail
	// endpoint returns it as a {code,name} object, while the listing
	// returns a flat string. We use the granular address fields anyway.
	JobSkillKeywords        []string `json:"jobSkillKeywords"`
	Description             string   `json:"description"`
	Responsibilities        string   `json:"responsibilities"`
	BasicQualifications     string   `json:"basicQualifications"`
	PreferredQualifications string   `json:"preferredQualifications"`
	Benefits                string   `json:"benefits"`
}

// parseListing decodes a listing response into normalized listing-level
// Postings. PublishedAt is set from startedAt (unless the API's sentinel
// "1970-01-01" appears); ClosedAt and AlwaysOpen are derived from endedAt
// (and its "9999-12-31" sentinel). FirstSeenAt and LastSeenAt are left
// zero for the server to stamp at insert time, matching Jumpit's contract.
func parseListing(body []byte) ([]scraper.Posting, error) {
	env, err := decodeEnvelope(body)
	if err != nil {
		return nil, err
	}
	var data listingData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return nil, fmt.Errorf("rallit: decode listing data: %w", err)
	}
	out := make([]scraper.Posting, 0, len(data.Items))
	for _, it := range data.Items {
		if it.ID == 0 {
			continue
		}
		stack := append([]string{}, it.JobSkillKeywords...)
		p := scraper.Posting{
			Source:          Source,
			SourcePostingID: fmt.Sprintf("%d", it.ID),
			URL:             strings.TrimSpace(it.URL),
			Title:           strings.TrimSpace(it.Title),
			Company:         strings.TrimSpace(it.CompanyName),
			Location:        strings.TrimSpace(it.AddressRegion),
			Newcomer:        true, // listing was filtered server-side by jobLevel=BEGINNER,INTERN,IRRELEVANT
			CareerLevel:     strings.TrimSpace(it.JobLevel),
			StackTags:       stack,
			Tags:            []scraper.Tag{},
			RawJSON:         string(body),
		}
		applyDates(&p, it.StartedAt, it.EndedAt, false)
		out = append(out, p)
	}
	return out, nil
}

// parseDetail enriches a listing-level posting with detail-call fields.
func parseDetail(base scraper.Posting, body []byte) (scraper.Posting, error) {
	env, err := decodeEnvelope(body)
	if err != nil {
		return scraper.Posting{}, err
	}
	var d detailData
	if err := json.Unmarshal(env.Data, &d); err != nil {
		return scraper.Posting{}, fmt.Errorf("rallit: decode detail data: %w", err)
	}

	enriched := base
	if t := strings.TrimSpace(d.Title); t != "" {
		enriched.Title = t
	}
	if c := strings.TrimSpace(d.CompanyName); c != "" {
		enriched.Company = c
	}
	if loc := joinAddress(d.AddressMain, d.AddressDetail, d.AddressBuildingName); loc != "" {
		enriched.Location = loc
	}
	if lv := strings.TrimSpace(d.JobLevel); lv != "" {
		enriched.CareerLevel = lv
	}
	if len(d.JobSkillKeywords) > 0 {
		enriched.StackTags = append([]string{}, d.JobSkillKeywords...)
	}
	enriched.Description = composeDescription(d)
	enriched.RawJSON = string(body)
	applyDates(&enriched, d.StartedAt, d.EndedAt, d.IsAlwaysHiring)
	return enriched, nil
}

// decodeEnvelope unwraps the standard rallit response envelope, returning an
// error when statusCode is not OK.
func decodeEnvelope(body []byte) (envelope, error) {
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return envelope{}, fmt.Errorf("rallit: decode envelope: %w", err)
	}
	if env.StatusCode != "OK" {
		return envelope{}, fmt.Errorf("rallit: status %s: %s", env.StatusCode, env.Message)
	}
	return env, nil
}

// joinAddress concatenates the three address fields, skipping blanks.
func joinAddress(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			kept = append(kept, t)
		}
	}
	return strings.Join(kept, " ")
}

// composeDescription concatenates the rich-text detail fields and strips
// HTML tags so the result feeds cleanly into the FTS index. The strip is
// intentionally crude — we index for keyword matching, not for display.
func composeDescription(d detailData) string {
	parts := []string{
		d.Description, d.Responsibilities,
		d.BasicQualifications, d.PreferredQualifications, d.Benefits,
	}
	var joined []string
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			joined = append(joined, t)
		}
	}
	return collapseWhitespace(stripHTMLTags(strings.Join(joined, "\n")))
}

// htmlTag matches an HTML tag, including its attributes. We replace
// matches with a single space so adjacent words stay separated.
var htmlTag = regexp.MustCompile(`<[^>]*>`)

func stripHTMLTags(s string) string {
	return htmlTag.ReplaceAllString(s, " ")
}

// whitespaceRun matches any run of whitespace; we collapse to single spaces.
var whitespaceRun = regexp.MustCompile(`\s+`)

func collapseWhitespace(s string) string {
	return strings.TrimSpace(whitespaceRun.ReplaceAllString(s, " "))
}

// applyDates sets PublishedAt, ClosedAt, and AlwaysOpen from the YYYY-MM-DD
// strings the rallit API uses. The 9999-12-31 endedAt sentinel and the
// explicit isAlwaysHiring flag both mean always-open; either is enough.
// 1970-01-01 in startedAt is the API's "unknown" sentinel and is dropped.
func applyDates(p *scraper.Posting, startedAt, endedAt string, alwaysHiring bool) {
	if t, ok := parseRallitDate(startedAt); ok && !isEpochSentinel(startedAt) {
		p.PublishedAt = &t
	}
	if alwaysHiring || strings.TrimSpace(endedAt) == alwaysOpenSentinel {
		p.AlwaysOpen = true
		p.ClosedAt = nil
		return
	}
	if t, ok := parseRallitDate(endedAt); ok {
		p.ClosedAt = &t
	}
}

func isEpochSentinel(s string) bool {
	return strings.TrimSpace(s) == "1970-01-01"
}

const alwaysOpenSentinel = "9999-12-31"

// parseRallitDate reads a YYYY-MM-DD string as KST midnight, returning UTC.
func parseRallitDate(s string) (time.Time, bool) {
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

var kstZone = time.FixedZone("KST", 9*60*60)
