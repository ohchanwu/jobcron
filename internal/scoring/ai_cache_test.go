package scoring

import (
	"encoding/json"
	"testing"

	"github.com/ohchanwu/job-scraper/internal/ai"
	"github.com/ohchanwu/job-scraper/internal/profile"
	"github.com/ohchanwu/job-scraper/internal/scraper"
)

// scoreNoAI scores the offline path (no AI extraction or delta) — what every
// pre-v2.0 test exercises.
func scoreNoAI(p scraper.Posting, prof profile.Profile) ScoreResult {
	return Score(p, prof, nil, nil)
}

func findLine(r ScoreResult, label string) (LineItem, bool) {
	for _, li := range r.Breakdown {
		if li.Label == label {
			return li, true
		}
	}
	return LineItem{}, false
}

func intPtr(n int) *int { return &n }

// TestScoreCareerCachePreference: when an AI extraction is present, scoreCareer
// uses its newcomer/min/max and SKIPS the regex override; absent, the regex
// override path is unchanged.
func TestScoreCareerCachePreference(t *testing.T) {
	p := basePosting() // source: MinCareer 10, MaxCareer 20
	p.Newcomer = false
	p.Description = "경력 5년 이상 우대" // regex parses (5, open), contradicts the source range
	prof := baseProfile()         // 신입 (CareerYears 0)

	// No extraction: regex override fires, user (0y) fits neither range -> 0.
	noAI := scoreNoAI(p, prof)
	if d, ok := lineDelta(noAI, "본문 5년 이상"); !ok || d != 0 {
		t.Fatalf("regex path: want a 0-delta '본문 5년 이상' override chip, got delta=%d ok=%v; breakdown=%+v", d, ok, noAI.Breakdown)
	}

	// AI says newcomer/0y: full newcomer award, normal "신입" label, no override.
	ext := &ai.Extraction{MinCareer: 0, MaxCareer: intPtr(0), Newcomer: true, EducationEnum: ai.EduNone}
	withAI := Score(p, prof, ext, nil)
	want := prof.EffectiveCareerWeight()
	if d, ok := lineDelta(withAI, "신입"); !ok || d != want {
		t.Fatalf("AI path: want full newcomer award %d on a '신입' chip, got delta=%d ok=%v; breakdown=%+v", want, d, ok, withAI.Breakdown)
	}
	if _, ok := lineDelta(withAI, "본문 5년 이상"); ok {
		t.Error("AI path must NOT emit a '본문 …' regex-override chip")
	}
}

// TestScoreCareerOpenUpperBound: a nil max_career reads as open-ended (99), so
// a senior user still fits.
func TestScoreCareerCacheOpenUpperBound(t *testing.T) {
	p := basePosting()
	prof := baseProfile()
	prof.CareerYears = 8
	ext := &ai.Extraction{MinCareer: 3, MaxCareer: nil, Newcomer: false, EducationEnum: ai.EduNone} // "경력 3년 이상"
	r := Score(p, prof, ext, nil)
	if d, ok := lineDelta(r, "경력 8년"); !ok || d != prof.EffectiveCareerWeight() {
		t.Fatalf("open upper bound: 8y should fit [3,∞), got delta=%d ok=%v; breakdown=%+v", d, ok, r.Breakdown)
	}
}

// TestEducationDealbreakerCachePreference: the AI education enum is mapped via
// ordinal (NOT fed to postingEducationLevel's Korean keyword matcher).
func TestEducationDealbreakerCachePreference(t *testing.T) {
	p := basePosting()
	p.EducationName = "" // no source education signal
	prof := baseProfile()
	prof.MaxEducation = profile.EducationHighSchool

	ext := &ai.Extraction{EducationEnum: ai.EduBachelor, Newcomer: true}
	r := Score(p, prof, ext, nil)
	if r.Total != -1 || r.DealbreakerHit == nil || r.DealbreakerHit.Kind != "education" {
		t.Fatalf("AI education dealbreaker: total=%d hit=%+v, want -1 / education", r.Total, r.DealbreakerHit)
	}
	if r.DealbreakerHit.Phrase != "대졸(4년)" {
		t.Errorf("phrase = %q, want 대졸(4년)", r.DealbreakerHit.Phrase)
	}

	// No extraction + empty EducationName -> postingEducationLevel("") == Any -> no hit.
	if r2 := scoreNoAI(p, prof); r2.Total == -1 {
		t.Error("no-AI path with empty EducationName must not dealbreak")
	}
}

// TestScoreFloorClamp: a negative net AI delta floors at 0 (never -1); an
// over-100 sum caps at 100.
func TestScoreFloorClamp(t *testing.T) {
	p := basePosting()
	p.StackTags = []string{"React"}
	prof := baseProfile()
	prof.Stacks = []profile.StackPref{{Name: "React", Weight: 30}}

	if r := Score(p, prof, nil, &ai.Delta{NetDelta: -100}); r.Total != 0 {
		t.Fatalf("negative net delta: Total = %d, want 0 (floored, never -1)", r.Total)
	}
	if r := Score(p, prof, nil, &ai.Delta{NetDelta: 1000}); r.Total != maxTotal {
		t.Fatalf("huge positive delta: Total = %d, want %d (capped)", r.Total, maxTotal)
	}
}

// TestDealbreakerShortCircuitNeverReachesAI: a dealbroken posting returns -1
// with an EMPTY breakdown even when ext AND delta are present — the AI line is
// never merged (D3/S4).
func TestDealbreakerShortCircuitNeverReachesAI(t *testing.T) {
	p := basePosting()
	p.Description = "병역특례 지원자 환영"
	prof := baseProfile()
	prof.Dealbreakers = []string{"병역특례"}

	ext := &ai.Extraction{Newcomer: true, EducationEnum: ai.EduNone}
	delta := &ai.Delta{NetDelta: 50, Items: []ai.DeltaItem{{Signal: "x", Kind: "presence", Delta: 50, Evidence: "y"}}}
	r := Score(p, prof, ext, delta)
	if r.Total != -1 {
		t.Fatalf("Total = %d, want -1 (keyword dealbreaker)", r.Total)
	}
	if len(r.Breakdown) != 0 {
		t.Fatalf("dealbroken posting must carry NO breakdown (no AI line), got %+v", r.Breakdown)
	}
}

// TestAILineItemMerged: a non-nil delta becomes one "AI 분석" line with the net
// as its value, the items as Evidence, the Stale flag propagated, and the
// chips summing to Total.
func TestAILineItemMerged(t *testing.T) {
	p := basePosting()
	p.StackTags = []string{"Go"}
	prof := baseProfile()
	prof.Stacks = []profile.StackPref{{Name: "Go", Weight: 20}}

	delta := &ai.Delta{
		NetDelta: 8,
		Stale:    true,
		Items: []ai.DeltaItem{
			{Signal: "백엔드", Kind: "presence", Delta: 10, Evidence: "서버 개발", MatchedGoal: "백엔드"},
			{Signal: "야근", Kind: "absence", Delta: -2},
		},
	}
	r := Score(p, prof, nil, delta)
	li, ok := findLine(r, "AI 분석")
	if !ok {
		t.Fatalf("no 'AI 분석' line item; breakdown=%+v", r.Breakdown)
	}
	if li.Delta != 8 {
		t.Errorf("AI line delta = %d, want 8 (net)", li.Delta)
	}
	if !li.Stale {
		t.Error("Stale flag not propagated to the AI line")
	}
	if len(li.Evidence) != 2 || li.Evidence[0].Quote != "서버 개발" || li.Evidence[0].MatchedGoal != "백엔드" {
		t.Errorf("evidence = %+v", li.Evidence)
	}
	if li.Evidence[1].Kind != "absence" || li.Evidence[1].Delta != -2 {
		t.Errorf("absence evidence = %+v", li.Evidence[1])
	}
	sum := 0
	for _, x := range r.Breakdown {
		sum += x.Delta
	}
	if sum != r.Total || r.Total != 28 {
		t.Errorf("chips sum=%d Total=%d, want both 28 (20 stack + 8 AI)", sum, r.Total)
	}
}

// TestExplainSignedNegative: a negative delta renders "-7", not "+-7".
func TestExplainSignedNegative(t *testing.T) {
	r := ScoreResult{Total: 3, Breakdown: []LineItem{
		{Label: "React", Delta: 10},
		{Label: "AI 분석", Delta: -7},
	}}
	if got := Explain(r); got != "React +10 · AI 분석 -7" {
		t.Errorf("Explain = %q, want \"React +10 · AI 분석 -7\"", got)
	}
}

// TestBreakdownJSONBackCompat: breakdown_json written before v2.0 (no
// evidence/stale keys, capitalized Label/Delta/Reason keys) still unmarshals.
func TestBreakdownJSONBackCompat(t *testing.T) {
	old := `{"Total":50,"Breakdown":[{"Label":"React","Delta":50,"Reason":"기술 스택 일치"}]}`
	var res ScoreResult
	if err := json.Unmarshal([]byte(old), &res); err != nil {
		t.Fatalf("unmarshal legacy breakdown_json: %v", err)
	}
	if len(res.Breakdown) != 1 || res.Breakdown[0].Label != "React" || res.Breakdown[0].Delta != 50 {
		t.Fatalf("legacy breakdown did not round-trip: %+v", res)
	}
	if res.Breakdown[0].Evidence != nil || res.Breakdown[0].Stale {
		t.Error("missing evidence/stale keys must default to zero values")
	}
}
