package profile

import (
	"encoding/json"
	"fmt"
)

// Profile is the user's job-matching preferences, scored against each posting.
type Profile struct {
	Stacks         []StackPref    `json:"stacks"`
	Location       LocationPref   `json:"location"`
	CareerYears    int            `json:"career_years"` // years of experience; 0 = 신입
	SalaryFloorKRW int            `json:"salary_floor_krw"`
	MaxEducation   EducationLevel `json:"max_education"` // highest level the user has
	MustHave       []string       `json:"must_have"`     // plain Korean phrases that must all appear
	Dealbreakers   []string       `json:"dealbreakers"`  // plain Korean phrases; any match excludes

	// DisabledSources are source identifiers (e.g. "worknet") the user has
	// opted out of. Default empty = every registered source is active. We
	// store the opt-out list (not an allow-list) so that new sources added
	// in future releases work for existing users without a profile rewrite.
	// omitempty keeps existing canonical JSON byte-identical when unset.
	DisabledSources []string `json:"disabled_sources,omitempty"`
}

// SourceEnabled reports whether the given source identifier should be active
// for this profile. Unknown sources are enabled — the opt-out model means a
// new source ships on by default and the user mutes it if it does not help.
func (p Profile) SourceEnabled(source string) bool {
	for _, s := range p.DisabledSources {
		if s == source {
			return false
		}
	}
	return true
}

// StackPref is a desired tech stack and the weight the user assigns it.
type StackPref struct {
	Name   string `json:"name"`
	Weight int    `json:"weight"`
}

// LocationPref is the user's location preference.
type LocationPref struct {
	Cities   []string `json:"cities"`
	Weight   int      `json:"weight"`
	RemoteOK bool     `json:"remote_ok"`
}

// EducationLevel is an ordinal education level. The zero value, EducationAny,
// means 학력 무관 — the user imposes no education requirement.
type EducationLevel int

const (
	EducationAny EducationLevel = iota
	EducationHighSchool
	EducationAssociate
	EducationBachelor
	EducationGraduate
)

// Marshal encodes p as canonical JSON. The encoding is deterministic — the
// struct has no maps — so the same profile always yields the same bytes,
// which keeps storage's profile hash stable.
func Marshal(p Profile) (string, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("profile: marshal: %w", err)
	}
	return string(b), nil
}

// Unmarshal decodes a profile from its canonical JSON.
func Unmarshal(s string) (Profile, error) {
	var p Profile
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return Profile{}, fmt.Errorf("profile: unmarshal: %w", err)
	}
	return p, nil
}
