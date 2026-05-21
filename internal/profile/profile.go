package profile

// Profile is the user's job-matching preferences, scored against each posting.
type Profile struct {
	Stacks         []StackPref
	Location       LocationPref
	CareerYears    int            // years of experience; 0 = 신입 (new grad)
	SalaryFloorKRW int            // minimum desired annual salary in KRW; 0 = no preference
	MaxEducation   EducationLevel // highest level the user has; EducationAny = 학력 무관
	MustHave       []string       // plain Korean phrases that must all appear
	Dealbreakers   []string       // plain Korean phrases; any match excludes the posting
}

// StackPref is a desired tech stack and the weight the user assigns it.
type StackPref struct {
	Name   string
	Weight int
}

// LocationPref is the user's location preference.
type LocationPref struct {
	Cities   []string
	Weight   int
	RemoteOK bool
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
