package scraper

import "time"

// Posting is the normalized domain model for a single job posting, shared by
// the scraper, scoring, and storage layers.
type Posting struct {
	ID              int64
	Source          string // e.g. "jumpit"
	SourcePostingID string // the source's own posting ID
	URL             string
	Title           string
	Company         string
	Location        string
	Newcomer        bool   // the 신입 (new-grad) marker
	MinCareer       int    // raw required years, lower bound
	MaxCareer       int    // raw required years, upper bound
	CareerLevel     string // derived label, e.g. "신입"
	Education       *int   // source education code; nil when unknown
	EducationName   string
	StackTags       []string // normalized tech-stack tags
	Tags            []Tag    // structured salary/welfare/subway/… tags
	Description     string   // composed JD text, indexed for FTS matching
	RawJSON         string   // full upstream payload, kept for forward compat
	PublishedAt     *time.Time
	ClosedAt        *time.Time // nil when AlwaysOpen
	AlwaysOpen      bool
	FirstSeenAt     time.Time
	LastSeenAt      time.Time
	DuplicateOf     *int64 // set by the server's dedup pass; nil for canonical
}

// Tag is a structured semantic tag attached to a posting (salary / welfare /
// subway / …), sourced from the upstream detail payload.
type Tag struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
}
