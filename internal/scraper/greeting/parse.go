package greeting

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ohchanwu/job-scraper/internal/scraper"
)

// nextDataPattern pulls the JSON payload out of the SSR Next.js page. The
// buildId in the _next/data route rotates per deploy, so we always parse
// __NEXT_DATA__ from the HTML rather than hitting that route.
var nextDataPattern = regexp.MustCompile(`(?s)<script id="__NEXT_DATA__"[^>]*>(.*?)</script>`)

type nextData struct {
	Props struct {
		PageProps struct {
			DehydratedState struct {
				Queries []query `json:"queries"`
			} `json:"dehydratedState"`
		} `json:"pageProps"`
	} `json:"props"`
}

type query struct {
	QueryKey json.RawMessage `json:"queryKey"`
	State    struct {
		Data json.RawMessage `json:"data"`
	} `json:"state"`
}

type opening struct {
	OpeningID int64   `json:"openingId"`
	Title     string  `json:"title"`
	OpenDate  string  `json:"openDate"`
	DueDate   *string `json:"dueDate"`
	Group     struct {
		Name string `json:"name"`
	} `json:"group"`
	OpeningJobPosition struct {
		OpeningJobPositions []position `json:"openingJobPositions"`
	} `json:"openingJobPosition"`
}

type position struct {
	WorkspaceOccupation *struct {
		Occupation string `json:"occupation"`
	} `json:"workspaceOccupation"`
	WorkspaceJob *struct {
		Job string `json:"job"`
	} `json:"workspaceJob"`
	WorkspacePlace *struct {
		Place       *string `json:"place"`
		Location    *string `json:"location"`
		DetailPlace *string `json:"detailPlace"`
	} `json:"workspacePlace"`
	JobPositionCareer *struct {
		CareerFrom *int   `json:"careerFrom"`
		CareerTo   *int   `json:"careerTo"`
		CareerType string `json:"careerType"`
	} `json:"jobPositionCareer"`
	JobPositionEmployment *struct {
		EmploymentType string `json:"employmentType"`
	} `json:"jobPositionEmployment"`
}

// parseBoard extracts the openings from a tenant board's HTML and converts
// every 신입 dev survivor into a Posting. origin is the scheme://host of the
// board after redirects, used to build click-through URLs.
func parseBoard(html []byte, t tenant, origin string) ([]scraper.Posting, error) {
	openings, err := extractOpenings(html)
	if err != nil {
		return nil, err
	}
	out := make([]scraper.Posting, 0, len(openings))
	for _, raw := range openings {
		var o opening
		if err := json.Unmarshal(raw, &o); err != nil {
			continue // skip a malformed opening rather than failing the board
		}
		pos, ok := pickQualifying(o)
		if !ok {
			continue
		}
		out = append(out, normalizeOpening(o, pos, raw, t, origin))
	}
	return out, nil
}

// extractOpenings finds the ["openings"] react-query entry inside
// __NEXT_DATA__ and returns its data array as raw opening objects.
func extractOpenings(html []byte) ([]json.RawMessage, error) {
	m := nextDataPattern.FindSubmatch(html)
	if m == nil {
		return nil, fmt.Errorf("no __NEXT_DATA__ script in page")
	}
	var nd nextData
	if err := json.Unmarshal(m[1], &nd); err != nil {
		return nil, fmt.Errorf("parse __NEXT_DATA__ JSON: %w", err)
	}
	for _, q := range nd.Props.PageProps.DehydratedState.Queries {
		// The openings query is the one whose queryKey is exactly
		// ["openings"]. Other queryKeys carry object elements, so a []string
		// decode fails for them — exactly the discriminator we want.
		var key []string
		if err := json.Unmarshal(q.QueryKey, &key); err != nil {
			continue
		}
		if len(key) == 1 && key[0] == "openings" {
			var data []json.RawMessage
			if err := json.Unmarshal(q.State.Data, &data); err != nil {
				return nil, fmt.Errorf("parse openings data: %w", err)
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("no [\"openings\"] query in __NEXT_DATA__")
}

// normalizeOpening maps a kept opening + its qualifying position to a
// Posting. raw is the opening's JSON, preserved on RawJSON.
func normalizeOpening(o opening, pos position, raw json.RawMessage, t tenant, origin string) scraper.Posting {
	id := strconv.FormatInt(o.OpeningID, 10)

	company := strings.TrimSpace(o.Group.Name)
	if company == "" {
		company = t.company
	}

	minC, maxC := careerBounds(pos)
	p := scraper.Posting{
		Source:          Source,
		SourcePostingID: t.slug + "-" + id,
		URL:             origin + "/ko/o/" + id,
		Title:           strings.TrimSpace(o.Title),
		Company:         company,
		Location:        normalizeLocation(placeOf(pos)),
		Newcomer:        true,
		MinCareer:       minC,
		MaxCareer:       maxC,
		CareerLevel:     careerLevel(pos),
		StackTags:       []string{},
		Tags:            buildTags(pos),
		Description:     composeDescription(o, pos),
		RawJSON:         string(raw),
	}
	if pub, ok := parseTimestamp(o.OpenDate); ok {
		p.PublishedAt = &pub
	}
	if o.DueDate == nil || strings.TrimSpace(*o.DueDate) == "" {
		p.AlwaysOpen = true
	} else if cl, ok := parseTimestamp(*o.DueDate); ok {
		p.ClosedAt = &cl
	}
	return p
}

// careerBounds derives (min, max) years from the structured career fields.
// NEW_COMER collapses to (0, 0); NOT_MATTER (경력무관) admits any experience
// from 0; explicit careerFrom/careerTo override when present.
func careerBounds(pos position) (min, max int) {
	c := pos.JobPositionCareer
	if c == nil {
		return 0, 0
	}
	if c.CareerFrom != nil {
		min = *c.CareerFrom
	}
	switch {
	case c.CareerTo != nil:
		max = *c.CareerTo
	case c.CareerType == careerNewComer:
		max = 0
	default: // NOT_MATTER with no ceiling
		max = anyMaxYears
	}
	return min, max
}

const anyMaxYears = 99

// careerLevel renders a short Korean label: 인턴 for internship roles, else
// 신입 for NEW_COMER and 경력무관 for NOT_MATTER.
func careerLevel(pos position) string {
	if pos.JobPositionEmployment != nil && pos.JobPositionEmployment.EmploymentType == empIntern {
		return "인턴"
	}
	if pos.JobPositionCareer != nil && pos.JobPositionCareer.CareerType == careerNotMatter {
		return "경력무관"
	}
	return "신입"
}

// buildTags exposes the structured occupation/job/employment fields as Tags
// for matching and display. MILITARY_SERVICE_EXCEPTION (병특) maps to a
// welfare 병역특례 가능 tag so the project's 병특 matcher reacts consistently.
func buildTags(pos position) []scraper.Tag {
	tags := []scraper.Tag{}
	if pos.WorkspaceOccupation != nil {
		if occ := strings.TrimSpace(pos.WorkspaceOccupation.Occupation); occ != "" {
			tags = append(tags, scraper.Tag{Name: occ, Category: "position"})
		}
	}
	if pos.WorkspaceJob != nil {
		if job := strings.TrimSpace(pos.WorkspaceJob.Job); job != "" {
			tags = append(tags, scraper.Tag{Name: job, Category: "position"})
		}
	}
	if pos.JobPositionEmployment != nil {
		switch pos.JobPositionEmployment.EmploymentType {
		case empIntern:
			tags = append(tags, scraper.Tag{Name: "인턴", Category: "employment_type"})
		case empFullTime:
			tags = append(tags, scraper.Tag{Name: "정규직", Category: "employment_type"})
		case empMilitary:
			tags = append(tags, scraper.Tag{Name: "병역특례 가능", Category: "welfare"})
		}
	}
	return tags
}

// composeDescription builds an FTS-indexable blob from the structured
// fields. Greeting's listing carries no JD body, so this is the matchable
// text the posting offers (occupation, job, division, employment).
func composeDescription(o opening, pos position) string {
	var parts []string
	parts = append(parts, strings.TrimSpace(o.Title))
	if pos.WorkspaceOccupation != nil && pos.WorkspaceOccupation.Occupation != "" {
		parts = append(parts, "직군: "+strings.TrimSpace(pos.WorkspaceOccupation.Occupation))
	}
	if pos.WorkspaceJob != nil && pos.WorkspaceJob.Job != "" {
		parts = append(parts, "직무: "+strings.TrimSpace(pos.WorkspaceJob.Job))
	}
	if pos.JobPositionEmployment != nil {
		if lbl := employmentLabel(pos.JobPositionEmployment.EmploymentType); lbl != "" {
			parts = append(parts, "고용형태: "+lbl)
		}
	}
	return strings.Join(parts, "\n")
}

func employmentLabel(t string) string {
	switch t {
	case empIntern:
		return "채용전환형 인턴"
	case empFullTime:
		return "정규직"
	case empMilitary:
		return "병역특례 (산업기능요원/전문연구요원)"
	default:
		return ""
	}
}

func placeOf(pos position) string {
	if pos.WorkspacePlace == nil {
		return ""
	}
	wp := pos.WorkspacePlace
	for _, v := range []*string{wp.Place, wp.Location, wp.DetailPlace} {
		if v != nil && strings.TrimSpace(*v) != "" {
			return strings.TrimSpace(*v)
		}
	}
	return ""
}

// normalizeLocation shortens a Greeting place string to the city/district
// the rest of the app expects. The strings arrive like
// "대한민국 서울특별시 강남구 역삼로1길 8" — we keep the first two tokens
// (시/도 + 구) and drop the country prefix and the street address.
func normalizeLocation(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	fields := strings.Fields(s)
	// Drop a leading 대한민국 / Korea country token.
	if len(fields) > 0 && (fields[0] == "대한민국" || strings.EqualFold(fields[0], "korea") || strings.EqualFold(fields[0], "south")) {
		fields = fields[1:]
	}
	if strings.EqualFold(strings.Join(fields, " "), "korea") {
		return "한국"
	}
	switch {
	case len(fields) >= 2:
		return fields[0] + " " + fields[1]
	case len(fields) == 1:
		return fields[0]
	default:
		return s
	}
}

// parseTimestamp reads Greeting's ISO8601 timestamps ("2026-04-28T04:53:24Z")
// and returns UTC.
func parseTimestamp(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}
