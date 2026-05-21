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
