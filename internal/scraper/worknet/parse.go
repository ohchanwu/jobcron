package worknet

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/ohchanwu/jobcron/internal/scraper"
)

// listingRoot is the wrapper for a callTp=L response. The inner <wanted>
// elements hold the per-posting fields. Field tags here track the spec in
// API_NOTES.md; the canonical names should be re-validated against the first
// real response and any drift recorded there.
type listingRoot struct {
	XMLName xml.Name        `xml:"wantedRoot"`
	Total   int             `xml:"total"`
	Items   []listingWanted `xml:"wanted"`
}

// listingWanted is one summary posting in a callTp=L response.
type listingWanted struct {
	WantedAuthNo string `xml:"wantedAuthNo"`
	Title        string `xml:"title"`
	Company      string `xml:"company"`
	Region       string `xml:"region"`
	SalTpNm      string `xml:"salTpNm"`
	Sal          string `xml:"sal"`
	EmpTpNm      string `xml:"empTpNm"`
	MinEdubg     string `xml:"minEdubg"`
	Career       string `xml:"career"`
	RegDt        string `xml:"regDt"`         // YYYYMMDD
	CloseDt      string `xml:"closeDt"`       // YYYYMMDD; 99991231 = always-open
	URL          string `xml:"wantedInfoUrl"` // public posting URL
	JobsCd       string `xml:"jobsCd"`        // KECO 직종코드
}

// detailRoot is the wrapper for a callTp=D response. The 워크넷 detail call
// returns one wantedDetail element with a superset of the listing fields
// plus the long-form description and welfare info — exact shape will be
// confirmed against the first real fixture and updated in API_NOTES.md.
type detailRoot struct {
	XMLName xml.Name     `xml:"wantedRoot"`
	Detail  wantedDetail `xml:"wanted"`
}

// wantedDetail is one full posting in a callTp=D response.
type wantedDetail struct {
	WantedAuthNo string `xml:"wantedAuthNo"`
	Title        string `xml:"title"`
	Company      string `xml:"company"`
	Region       string `xml:"region"`
	SalTpNm      string `xml:"salTpNm"`
	Sal          string `xml:"sal"`
	EmpTpNm      string `xml:"empTpNm"`
	MinEdubg     string `xml:"minEdubg"`
	Career       string `xml:"career"`
	RegDt        string `xml:"regDt"`
	CloseDt      string `xml:"closeDt"`
	URL          string `xml:"wantedInfoUrl"`
	JobsCd       string `xml:"jobsCd"`
	JobsNm       string `xml:"jobsNm"` // human-readable 직종명, included in detail
	// Description carries the long-form job description. The exact tag name
	// in the live response will be one of: jobcont, jobDesc, jobsDesc — to
	// be confirmed against a real fixture. Until then we accept any of them
	// via the chardata fallback in parseDetail.
	JobCont string `xml:"jobcont"`
	JobDesc string `xml:"jobDesc"`
}

// parseListing decodes a callTp=L response body into normalized listing-level
// Posting values. PublishedAt is set from regDt; ClosedAt/AlwaysOpen are
// derived from closeDt; FirstSeenAt/LastSeenAt are left zero for the server
// to stamp at insert time, matching the Jumpit handling.
func parseListing(body []byte) ([]scraper.Posting, error) {
	var root listingRoot
	if err := xml.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("worknet: decode listing XML: %w", err)
	}
	out := make([]scraper.Posting, 0, len(root.Items))
	for _, w := range root.Items {
		if strings.TrimSpace(w.WantedAuthNo) == "" {
			continue // malformed entry; skip rather than fail the whole batch
		}
		p := scraper.Posting{
			Source:          Source,
			SourcePostingID: strings.TrimSpace(w.WantedAuthNo),
			URL:             strings.TrimSpace(w.URL),
			Title:           strings.TrimSpace(w.Title),
			Company:         strings.TrimSpace(w.Company),
			Location:        strings.TrimSpace(w.Region),
			Newcomer:        true, // listing was filtered server-side by minCareer=0/maxCareer=0
			MinCareer:       0,
			MaxCareer:       0,
			CareerLevel:     strings.TrimSpace(w.Career),
			EducationName:   strings.TrimSpace(w.MinEdubg),
			StackTags:       []string{},
			Tags:            []scraper.Tag{},
			RawJSON:         string(body), // XML kept for forward compat
		}
		applyDates(&p, w.RegDt, w.CloseDt)
		out = append(out, p)
	}
	return out, nil
}

// parseDetail enriches a listing-level Posting with detail-level fields from
// a callTp=D response body.
func parseDetail(base scraper.Posting, body []byte) (scraper.Posting, error) {
	var root detailRoot
	if err := xml.Unmarshal(body, &root); err != nil {
		return scraper.Posting{}, fmt.Errorf("worknet: decode detail XML: %w", err)
	}
	d := root.Detail

	desc := strings.TrimSpace(d.JobCont)
	if desc == "" {
		desc = strings.TrimSpace(d.JobDesc)
	}

	enriched := base
	if t := strings.TrimSpace(d.Title); t != "" {
		enriched.Title = t
	}
	if c := strings.TrimSpace(d.Company); c != "" {
		enriched.Company = c
	}
	if loc := strings.TrimSpace(d.Region); loc != "" {
		enriched.Location = loc
	}
	if e := strings.TrimSpace(d.MinEdubg); e != "" {
		enriched.EducationName = e
	}
	if c := strings.TrimSpace(d.Career); c != "" {
		enriched.CareerLevel = c
	}
	enriched.Description = desc
	enriched.RawJSON = string(body)
	applyDates(&enriched, d.RegDt, d.CloseDt)
	return enriched, nil
}

// applyDates sets PublishedAt, ClosedAt, and AlwaysOpen from the YYYYMMDD
// strings the 워크넷 API uses. 99991231 in closeDt means always-open.
// Out-of-format values are silently dropped — the spec is loose enough that
// occasional bad rows should not poison the whole batch.
func applyDates(p *scraper.Posting, regDt, closeDt string) {
	if t, ok := parseYMD(regDt); ok {
		p.PublishedAt = &t
	}
	regCloseDt := strings.TrimSpace(closeDt)
	if regCloseDt == "99991231" {
		p.AlwaysOpen = true
		p.ClosedAt = nil
		return
	}
	if t, ok := parseYMD(closeDt); ok {
		p.ClosedAt = &t
	}
}

// parseYMD reads a YYYYMMDD string as KST midnight, returning UTC. Returns
// (zero, false) on any parse error or an empty input.
func parseYMD(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("20060102", s, kstZone)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

// kstZone is Korea Standard Time. 워크넷 dates arrive without a zone, so
// we anchor them to KST midnight before storing UTC.
var kstZone = time.FixedZone("KST", 9*60*60)
