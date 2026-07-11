package jumpit

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ohchanwu/jobcron/internal/scraper"
)

// Source is the stable source identifier for 점핏 (Jumpit) postings.
const Source = "jumpit"

// postingURLPrefix builds the user-facing posting URL: prefix + position id.
const postingURLPrefix = "https://jumpit.saramin.co.kr/position/"

// kst is the Korea Standard Time zone. Korea has not observed DST since 1988,
// so a fixed +09:00 offset is correct and avoids a tzdata dependency. 점핏
// timestamps carry no offset and are KST.
var kst = time.FixedZone("KST", 9*60*60)

// listingEnvelope is the 점핏 /api/positions response shape. Positions are
// kept raw so each can also be retained verbatim as Posting.RawJSON.
type listingEnvelope struct {
	Result struct {
		Positions []json.RawMessage `json:"positions"`
	} `json:"result"`
}

// listingPosition is one position object from the listing endpoint.
type listingPosition struct {
	ID          int64    `json:"id"`
	Title       string   `json:"title"`
	CompanyName string   `json:"companyName"`
	TechStacks  []string `json:"techStacks"`
	Newcomer    bool     `json:"newcomer"`
	MinCareer   int      `json:"minCareer"`
	MaxCareer   int      `json:"maxCareer"`
	Locations   []string `json:"locations"`
	AlwaysOpen  bool     `json:"alwaysOpen"`
	ClosedAt    string   `json:"closedAt"` // "" when JSON null (alwaysOpen)
}

// parseListing converts a 점핏 listing response into listing-level postings.
// The detail-only fields (Description, Tags, Education, PublishedAt) are left
// zero — FetchDetail fills them in.
func parseListing(data []byte) ([]scraper.Posting, error) {
	var env listingEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("jumpit: parse listing: %w", err)
	}
	postings := make([]scraper.Posting, 0, len(env.Result.Positions))
	for i, raw := range env.Result.Positions {
		var lp listingPosition
		if err := json.Unmarshal(raw, &lp); err != nil {
			return nil, fmt.Errorf("jumpit: parse listing position %d: %w", i, err)
		}
		closedAt, err := parseJumpitTime(lp.ClosedAt)
		if err != nil {
			return nil, fmt.Errorf("jumpit: listing position %d closedAt: %w", i, err)
		}
		id := strconv.FormatInt(lp.ID, 10)
		postings = append(postings, scraper.Posting{
			Source:          Source,
			SourcePostingID: id,
			URL:             postingURLPrefix + id,
			Title:           lp.Title,
			Company:         lp.CompanyName,
			Location:        strings.Join(lp.Locations, ", "),
			Newcomer:        lp.Newcomer,
			MinCareer:       lp.MinCareer,
			MaxCareer:       lp.MaxCareer,
			CareerLevel:     careerLevel(lp.Newcomer, lp.MinCareer, lp.MaxCareer),
			StackTags:       lp.TechStacks,
			AlwaysOpen:      lp.AlwaysOpen,
			ClosedAt:        closedAt,
			RawJSON:         string(raw),
		})
	}
	return postings, nil
}

// careerLevel derives the human-readable career label from the structured
// newcomer / min-max-career fields (no string parsing of the title).
func careerLevel(newcomer bool, minCareer, maxCareer int) string {
	if newcomer || (minCareer == 0 && maxCareer == 0) {
		return "신입"
	}
	return fmt.Sprintf("%d-%d년", minCareer, maxCareer)
}

// detailEnvelope is the 점핏 /api/position/{id} response shape.
type detailEnvelope struct {
	Result json.RawMessage `json:"result"`
}

// detailPosition is the detail-endpoint position object — only the fields the
// scraper consumes.
type detailPosition struct {
	ServiceInfo           string        `json:"serviceInfo"`
	Responsibility        string        `json:"responsibility"`
	Qualifications        string        `json:"qualifications"`
	PreferredRequirements string        `json:"preferredRequirements"`
	Welfares              string        `json:"welfares"`
	Location              string        `json:"location"`
	Education             *int          `json:"education"`
	EducationName         string        `json:"educationName"`
	PublishedAt           string        `json:"publishedAt"`
	TechStacks            []detailStack `json:"techStacks"`
	Tags                  []detailTag   `json:"tags"`
}

type detailStack struct {
	Stack string `json:"stack"`
}

type detailTag struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Emoticon string `json:"emoticon"`
}

// parseDetail enriches a listing-level posting p with its detail-page fields:
// the composed Description, structured Tags, Education, PublishedAt, the full
// Location address, and the detail-shape tech stacks. Identity and
// listing-derived fields (Source, URL, Title, Company, career, ClosedAt) are
// left untouched.
func parseDetail(p scraper.Posting, data []byte) (scraper.Posting, error) {
	var env detailEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return scraper.Posting{}, fmt.Errorf("jumpit: parse detail: %w", err)
	}
	var dp detailPosition
	if err := json.Unmarshal(env.Result, &dp); err != nil {
		return scraper.Posting{}, fmt.Errorf("jumpit: parse detail result: %w", err)
	}
	publishedAt, err := parseJumpitTime(dp.PublishedAt)
	if err != nil {
		return scraper.Posting{}, fmt.Errorf("jumpit: detail publishedAt: %w", err)
	}

	// Description join order is fixed so dealbreaker matching is deterministic:
	// serviceInfo, responsibility, qualifications, preferredRequirements,
	// welfares (welfares is where 병특/재택/야근 dealbreakers usually live).
	p.Description = strings.Join([]string{
		dp.ServiceInfo,
		dp.Responsibility,
		dp.Qualifications,
		dp.PreferredRequirements,
		dp.Welfares,
	}, "\n\n")
	p.Tags = detailTags(dp.Tags)
	p.StackTags = detailStacks(dp.TechStacks)
	p.Education = dp.Education
	p.EducationName = dp.EducationName
	p.PublishedAt = publishedAt
	if dp.Location != "" {
		p.Location = dp.Location // full address — richer than the listing's
	}
	p.RawJSON = string(env.Result)
	return p, nil
}

// detailTags maps the detail payload's tag objects into domain Tags, using
// the upstream `emoticon` field as the tag Category.
func detailTags(tags []detailTag) []scraper.Tag {
	out := make([]scraper.Tag, 0, len(tags))
	for _, t := range tags {
		out = append(out, scraper.Tag{ID: t.ID, Name: t.Name, Category: t.Emoticon})
	}
	return out
}

// detailStacks normalizes the detail endpoint's object-array tech stacks
// ([{stack,imagePath}]) into the []string shape used everywhere else.
func detailStacks(stacks []detailStack) []string {
	out := make([]string, 0, len(stacks))
	for _, s := range stacks {
		out = append(out, s.Stack)
	}
	return out
}

// parseJumpitTime parses a 점핏 timestamp as KST. The listing endpoint uses a
// "T" separator and the detail endpoint a space; neither carries an offset. An
// empty string (a JSON null/absent value) yields a nil time.
func parseJumpitTime(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, kst); err == nil {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("unrecognized time %q", s)
}
