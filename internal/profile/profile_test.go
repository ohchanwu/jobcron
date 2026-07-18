package profile

import (
	"reflect"
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"
)

func TestProfileMarshalRoundTrip(t *testing.T) {
	p := Profile{
		Stacks:         []StackPref{{Name: "React", Weight: 20}, {Name: "Go", Weight: 30}},
		Location:       LocationPref{Cities: []string{"서울", "판교"}, Weight: 15, RemoteOK: true},
		CareerYears:    0,
		SalaryFloorKRW: 50_000_000,
		MaxEducation:   EducationBachelor,
		Dealbreakers:   []string{"병역특례"},
		JobLikes:       "백엔드 API 설계",
		LongTermGoals:  "플랫폼 엔지니어로 성장",
	}
	s, err := Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := Unmarshal(s)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(got, p) {
		t.Errorf("round-trip mismatch:\n got = %+v\nwant = %+v", got, p)
	}
}

func TestProfileMarshalIsDeterministic(t *testing.T) {
	p := Profile{
		Stacks:         []StackPref{{Name: "React", Weight: 20}},
		MaxEducation:   EducationAssociate,
		JobLikes:       "a",
		ShortTermGoals: "b",
	}
	first, err := Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for i := 0; i < 10; i++ {
		again, err := Marshal(p)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if again != first {
			t.Fatalf("Marshal is not deterministic:\n %q\n %q", again, first)
		}
	}
}

// TestProfileMarshalEmptyGoalsByteIdentical guards the omitempty discipline:
// adding the four goal fields must NOT change the canonical JSON of a profile
// that leaves them empty (so existing score hashes don't churn). MustHave was
// removed (no omitempty), so must_have is also absent — that removal does
// change the hash, but it is a one-time re-score and never alters a Total (an
// empty must-have list never excluded anything).
func TestProfileMarshalEmptyGoalsByteIdentical(t *testing.T) {
	s, err := Marshal(Profile{})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, key := range []string{"job_likes", "job_dislikes", "short_term_goals", "long_term_goals", "must_have"} {
		if strings.Contains(s, key) {
			t.Errorf("zero profile JSON should omit %q, got: %s", key, s)
		}
	}
	want := `{"stacks":null,"location":{"cities":null,"weight":0,"remote_ok":false},"career_years":0,"salary_floor_krw":0,"max_education":0,"dealbreakers":null}`
	if s != want {
		t.Errorf("canonical empty profile JSON drift (a change here invalidates every score hash):\n got = %s\nwant = %s", s, want)
	}
}

// TestProfileUnmarshalIgnoresLegacyMustHave: an old persisted profile that
// still carries "must_have" must unmarshal cleanly (the key is now unknown and
// silently ignored).
func TestProfileUnmarshalIgnoresLegacyMustHave(t *testing.T) {
	legacy := `{"stacks":[],"max_education":0,"must_have":["React","재택"],"dealbreakers":null}`
	got, err := Unmarshal(legacy)
	if err != nil {
		t.Fatalf("Unmarshal legacy profile with must_have: %v", err)
	}
	if got.JobLikes != "" {
		t.Errorf("legacy must_have must not bleed into a goal field, got JobLikes=%q", got.JobLikes)
	}
}

func TestAIProductionDefaults(t *testing.T) {
	var p Profile
	if p.ScheduledAIEnabled {
		t.Fatal("scheduled AI must default to disabled")
	}
	if got := p.EffectiveAIMonthlyUSDCapCents(); got != DefaultAIMonthlyUSDCents {
		t.Fatalf("monthly USD cap = %d, want %d", got, DefaultAIMonthlyUSDCents)
	}
	if got := p.EffectiveAIDailyUSDCapCents(); got != DefaultAIDailyUSDCents {
		t.Fatalf("daily USD cap = %d, want %d", got, DefaultAIDailyUSDCents)
	}
	if got := p.EffectiveAIRunUSDCapCents(); got != DefaultAIRunUSDCents {
		t.Fatalf("run USD cap = %d, want %d", got, DefaultAIRunUSDCents)
	}
	if DefaultAIMonthlyUSDCents != 1_000 || DefaultAIDailyUSDCents != 50 || DefaultAIRunUSDCents != 30 {
		t.Fatalf("AI USD defaults drifted: monthly=%d daily=%d run=%d",
			DefaultAIMonthlyUSDCents, DefaultAIDailyUSDCents, DefaultAIRunUSDCents)
	}
}

func TestBuildStage2ProfileText(t *testing.T) {
	if got := BuildStage2ProfileText(Profile{}); got != "" {
		t.Errorf("empty goals should produce empty text, got %q", got)
	}

	p := Profile{JobLikes: "백엔드 API", JobDislikes: "잦은 야근", ShortTermGoals: "신입 입사", LongTermGoals: "테크리드"}
	text := BuildStage2ProfileText(p)
	for _, want := range []string{"좋아하는 업무: 백엔드 API", "피하고 싶은 업무: 잦은 야근", "단기 목표: 신입 입사", "장기 목표: 테크리드"} {
		if !strings.Contains(text, want) {
			t.Errorf("text missing %q:\n%s", want, text)
		}
	}
	// Deterministic.
	if BuildStage2ProfileText(p) != text {
		t.Error("BuildStage2ProfileText is not deterministic")
	}
}

// TestAIInputHashInvariants is the core T1/D10 cache invariant: the hash
// changes only on a GOAL edit, never on a weight/MinScore/stack tweak.
func TestAIInputHashInvariants(t *testing.T) {
	base := Profile{JobLikes: "백엔드", ShortTermGoals: "신입"}
	h := AIInputHash(base)
	if len(h) != 12 {
		t.Fatalf("AIInputHash = %q, want 12 hex chars", h)
	}

	// Weight / MinScore / stack changes must NOT move the hash.
	min := 80
	noChurn := base
	noChurn.CareerWeight = 40
	noChurn.SalaryWeight = 25
	noChurn.MinScore = &min
	noChurn.Stacks = []StackPref{{Name: "Go", Weight: 50}}
	if AIInputHash(noChurn) != h {
		t.Error("weight/MinScore/stack tweaks must not change ai_input_hash (would re-spend AI tokens)")
	}

	// A goal edit MUST move the hash.
	goalEdit := base
	goalEdit.JobLikes = "프론트엔드"
	if AIInputHash(goalEdit) == h {
		t.Error("a goal edit must change ai_input_hash (stale AI delta)")
	}
}

func TestDealbreakerInputHashInvariants(t *testing.T) {
	base := Profile{Dealbreakers: []string{"사용자 리서치", "야근"}}
	h := DealbreakerInputHash(base)
	if len(h) != 64 {
		t.Fatalf("DealbreakerInputHash = %q, want full SHA-256 hex", h)
	}

	equivalent := Profile{Dealbreakers: []string{
		strings.ToUpper(norm.NFD.String("사용자, 리서치")),
		"  야근!!! ",
	}}
	if got := DealbreakerInputHash(equivalent); got != h {
		t.Errorf("canonical-only edits moved hash: got %q want %q", got, h)
	}

	reordered := Profile{Dealbreakers: []string{"야근", "사용자 리서치"}}
	if DealbreakerInputHash(reordered) == h {
		t.Error("phrase order must move dealbreaker input hash")
	}

	semanticEdit := Profile{Dealbreakers: []string{"사용자 인터뷰", "야근"}}
	if DealbreakerInputHash(semanticEdit) == h {
		t.Error("semantic token edit must move dealbreaker input hash")
	}

	withEmpty := Profile{Dealbreakers: []string{"사용자 리서치", "!!!", "야근"}}
	if DealbreakerInputHash(withEmpty) != h {
		t.Error("empty canonical phrases must not move dealbreaker input hash")
	}
}
