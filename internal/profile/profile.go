package profile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// Default scoring weights — applied when a profile field is zero. The
// values reproduce the fixed point caps the scorer used before per-
// category weights were introduced (DefaultCareerWeight = the old
// careerExact, DefaultSalaryWeight = the old salaryClear).
const (
	DefaultCareerWeight = 25
	DefaultSalaryWeight = 10

	// DefaultMinScore is the briefing's default "hide rows below this"
	// threshold for new profiles and any saved profile that predates the
	// field. The user can override to 0 ("show everything") via the
	// profile form — see EffectiveMinScore.
	DefaultMinScore = 40
)

// Profile is the user's job-matching preferences, scored against each posting.
type Profile struct {
	Stacks         []StackPref    `json:"stacks"`
	Location       LocationPref   `json:"location"`
	CareerYears    int            `json:"career_years"` // years of experience; 0 = 신입
	CareerWeight   int            `json:"career_weight,omitempty"`
	SalaryFloorKRW int            `json:"salary_floor_krw"`
	SalaryWeight   int            `json:"salary_weight,omitempty"`
	MaxEducation   EducationLevel `json:"max_education"` // highest level the user has
	Dealbreakers   []string       `json:"dealbreakers"`  // plain Korean phrases; any match excludes

	// Goal fields (v2.0) are optional free-text the Stage-2 AI prompt reads.
	// omitempty keeps an empty-goals profile's canonical JSON byte-identical to
	// a pre-v2.0 profile (so adding them does not invalidate every score hash).
	// They are the ONLY inputs to BuildStage2ProfileText / AIInputHash — weight
	// and MinScore tweaks must not churn the AI cache.
	JobLikes       string `json:"job_likes,omitempty"`
	JobDislikes    string `json:"job_dislikes,omitempty"`
	ShortTermGoals string `json:"short_term_goals,omitempty"`
	LongTermGoals  string `json:"long_term_goals,omitempty"`

	// MinScore is the briefing's "hide rows below this score" threshold.
	// Pointer so that nil (field absent in JSON) differs from explicit 0
	// (the user opted in to "show everything"). Use EffectiveMinScore to
	// get the value with the DefaultMinScore fallback applied.
	MinScore *int `json:"min_score,omitempty"`

	// DisabledSources are source identifiers (e.g. "worknet") the user has
	// opted out of. Default empty = every registered source is active. We
	// store the opt-out list (not an allow-list) so that new sources added
	// in future releases work for existing users without a profile rewrite.
	// omitempty keeps existing canonical JSON byte-identical when unset.
	DisabledSources []string `json:"disabled_sources,omitempty"`
}

// EffectiveCareerWeight returns CareerWeight when set, falling back to
// DefaultCareerWeight when the field is zero. Lets old profiles (saved
// before the field existed) score identically to defaults.
func (p Profile) EffectiveCareerWeight() int {
	if p.CareerWeight > 0 {
		return p.CareerWeight
	}
	return DefaultCareerWeight
}

// EffectiveSalaryWeight is the SalaryWeight counterpart of
// EffectiveCareerWeight.
func (p Profile) EffectiveSalaryWeight() int {
	if p.SalaryWeight > 0 {
		return p.SalaryWeight
	}
	return DefaultSalaryWeight
}

// EffectiveMinScore returns MinScore when set (including explicit 0,
// which means "show every non-dealbroken row"), falling back to
// DefaultMinScore for profiles that predate the field.
func (p Profile) EffectiveMinScore() int {
	if p.MinScore != nil {
		return *p.MinScore
	}
	return DefaultMinScore
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

// BuildStage2ProfileText assembles the four goal fields into the canonical
// text the Stage-2 AI prompt reads — and the input to AIInputHash. It reads
// ONLY the goal fields (NFC-normalized, empty fields omitted), so weight or
// MinScore changes leave it byte-stable and never invalidate the AI cache;
// only a goal edit changes it. The output is also the literal Korean text the
// model sees, so the labels read naturally.
func BuildStage2ProfileText(p Profile) string {
	var b strings.Builder
	add := func(label, val string) {
		if val = strings.TrimSpace(val); val == "" {
			return
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(label)
		b.WriteString(": ")
		b.WriteString(val)
	}
	add("좋아하는 업무", p.JobLikes)
	add("피하고 싶은 업무", p.JobDislikes)
	add("단기 목표", p.ShortTermGoals)
	add("장기 목표", p.LongTermGoals)
	return norm.NFC.String(b.String())
}

// AIInputHash is the Stage-2 cache key: the first 12 hex chars of
// sha256(BuildStage2ProfileText(p)), mirroring storage.profileHash. Because it
// hashes only the goal text, weight/MinScore tweaks keep the cached AI delta
// fresh; only a goal edit marks it stale.
func AIInputHash(p Profile) string {
	sum := sha256.Sum256([]byte(BuildStage2ProfileText(p)))
	return hex.EncodeToString(sum[:])[:12]
}
