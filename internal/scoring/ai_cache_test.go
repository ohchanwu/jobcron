package scoring

import (
	"encoding/json"
	"testing"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
	"github.com/ohchanwu/jobcron/internal/scraper"
)

// scoreNoAI scores the offline path (no AI extraction or delta) — what every
// pre-v2.0 test exercises.
func scoreNoAI(p scraper.Posting, prof profile.Profile) ScoreResult {
	return Score(p, prof, nil, nil, nil)
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
	withAI := Score(p, prof, ext, nil, nil)
	want := prof.EffectiveCareerWeight()
	if d, ok := lineDelta(withAI, "신입"); !ok || d != want {
		t.Fatalf("AI path: want full newcomer award %d on a '신입' chip, got delta=%d ok=%v; breakdown=%+v", want, d, ok, withAI.Breakdown)
	}
	if _, ok := lineDelta(withAI, "본문 5년 이상"); ok {
		t.Error("AI path must NOT emit a '본문 …' regex-override chip")
	}
}

// TestScoreCareerInternGuard (R3): an 인턴/internship role is new-grad-eligible
// by definition, so a bad AI extraction (newcomer=false, min_career=2) must NOT
// strip its 신입 award — the inclusive reading wins. The SAME bad extraction on a
// non-intern role still drops the award, preserving D2's source-false-positive
// correction. English is token-exact, so "internship" fires but "internal" does
// not.
func TestScoreCareerInternGuard(t *testing.T) {
	prof := baseProfile() // 신입 (CareerYears 0)
	want := prof.EffectiveCareerWeight()
	// The model misjudged the role as experienced.
	badExt := func() *ai.Extraction {
		return &ai.Extraction{MinCareer: 2, MaxCareer: intPtr(5), Newcomer: false, EducationEnum: ai.EduNone}
	}

	cases := []struct {
		name      string
		title     string
		wantAward bool
	}{
		{"korean intern bracket", "[인턴] 풀스택 개발자", true},
		{"korean internship word", "백엔드 인턴십 채용", true},
		{"english internship", "Backend Engineer Internship", true},
		{"non-intern role keeps D2", "백엔드 개발자", false},
		{"english 'internal' is not intern", "Internal Tools Engineer", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := basePosting()
			p.Title = tc.title
			p.Newcomer = true // source correctly marks it new-grad-eligible
			r := Score(p, prof, badExt(), nil, nil)
			d, ok := lineDelta(r, "신입")
			if tc.wantAward {
				if !ok || d != want {
					t.Fatalf("want full 신입 award %d, got delta=%d ok=%v; breakdown=%+v", want, d, ok, r.Breakdown)
				}
			} else if ok {
				t.Fatalf("must NOT award 신입 for %q (D2 correction must stand); breakdown=%+v", tc.title, r.Breakdown)
			}
		})
	}
}

// TestScoreCareerOpenUpperBound: a nil max_career reads as open-ended (99), so
// a senior user still fits.
func TestScoreCareerCacheOpenUpperBound(t *testing.T) {
	p := basePosting()
	prof := baseProfile()
	prof.CareerYears = 8
	ext := &ai.Extraction{MinCareer: 3, MaxCareer: nil, Newcomer: false, EducationEnum: ai.EduNone} // "경력 3년 이상"
	r := Score(p, prof, ext, nil, nil)
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
	r := Score(p, prof, ext, nil, nil)
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

	// A delta's net is the sum of its surviving items (an items-less delta emits
	// no line at all — §c), so exercise the floor/cap through real items.
	neg := &ai.Delta{NetDelta: -100, Items: []ai.DeltaItem{{Signal: "감점", Kind: "presence", Delta: -100, Evidence: "x"}}}
	if r := Score(p, prof, nil, neg, nil); r.Total != 0 {
		t.Fatalf("negative net delta: Total = %d, want 0 (floored, never -1)", r.Total)
	}
	pos := &ai.Delta{NetDelta: 1000, Items: []ai.DeltaItem{{Signal: "가점", Kind: "presence", Delta: 1000, Evidence: "y"}}}
	if r := Score(p, prof, nil, pos, nil); r.Total != maxTotal {
		t.Fatalf("huge positive delta: Total = %d, want %d (capped)", r.Total, maxTotal)
	}
}

// TestAIEmptyDeltaEmitsNoLine: a delta with no surviving items adds no
// "AI 분석" chip — the calm surface stays silent (§c). nil and empty both apply.
func TestAIEmptyDeltaEmitsNoLine(t *testing.T) {
	p := basePosting()
	p.StackTags = []string{"React"}
	prof := baseProfile()
	prof.Stacks = []profile.StackPref{{Name: "React", Weight: 30}}

	for name, delta := range map[string]*ai.Delta{
		"nil delta":   nil,
		"empty items": {NetDelta: 0, Items: nil},
	} {
		r := Score(p, prof, nil, delta, nil)
		if _, ok := lineDelta(r, aiLineLabel); ok {
			t.Errorf("%s produced an AI line, want none", name)
		}
		if r.Total != 30 {
			t.Errorf("%s: Total = %d, want 30 (React only)", name, r.Total)
		}
	}
}

// TestAIChipsSumToTotal: every rendered chip's Delta sums to the posting's
// Total — the user-visible invariant the briefing relies on (CLAUDE.md). The
// AI line carries the net of its surviving items.
func TestAIChipsSumToTotal(t *testing.T) {
	p := basePosting()
	p.StackTags = []string{"React"}
	prof := baseProfile()
	prof.Stacks = []profile.StackPref{{Name: "React", Weight: 30}}

	delta := &ai.Delta{NetDelta: -5, Items: []ai.DeltaItem{
		{Signal: "백엔드 중심", Kind: ai.KindPresence, Delta: 7, Evidence: "서버 개발"},
		{Signal: "야근", Kind: ai.KindAbsence, Delta: -12, Evidence: "'야근' 등 관련 언급 없음 (코드 확인)"},
	}}
	r := Score(p, prof, nil, delta, nil)

	sum := 0
	for _, li := range r.Breakdown {
		sum += li.Delta
	}
	if sum != r.Total {
		t.Fatalf("chips sum to %d but Total is %d", sum, r.Total)
	}
	if r.Total != 25 { // React 30 + AI net -5
		t.Fatalf("Total = %d, want 25", r.Total)
	}
	aiDelta, ok := lineDelta(r, aiLineLabel)
	if !ok || aiDelta != -5 {
		t.Fatalf("AI line delta = %d (present=%v), want -5", aiDelta, ok)
	}
	// The popover carries every surviving signal.
	for _, li := range r.Breakdown {
		if li.Label == aiLineLabel && len(li.Evidence) != 2 {
			t.Fatalf("AI line Evidence has %d items, want 2", len(li.Evidence))
		}
	}
}

// TestAIStaleFlowsToLineItem: a stale delta marks its LineItem stale (T6 renders
// the "(이전 프로필 기준)" chrome) and is still summed into the Total (T1).
func TestAIStaleFlowsToLineItem(t *testing.T) {
	p := basePosting()
	p.StackTags = []string{"React"}
	prof := baseProfile()
	prof.Stacks = []profile.StackPref{{Name: "React", Weight: 30}}

	delta := &ai.Delta{NetDelta: 8, Stale: true, Items: []ai.DeltaItem{
		{Signal: "성장", Kind: ai.KindPresence, Delta: 8, Evidence: "빠르게 성장하는 팀"},
	}}
	r := Score(p, prof, nil, delta, nil)

	var found bool
	for _, li := range r.Breakdown {
		if li.Label == aiLineLabel {
			found = true
			if !li.Stale {
				t.Error("stale delta must produce a Stale=true LineItem")
			}
		}
	}
	if !found {
		t.Fatal("stale delta produced no AI line")
	}
	if r.Total != 38 { // React 30 + stale AI +8 (still counted)
		t.Fatalf("Total = %d, want 38 (a stale delta is still summed)", r.Total)
	}
}

// TestScorePreservesDealbreakerBeforeStage2: a dealbroken posting returns -1
// with an EMPTY breakdown even when ext AND delta are present — the AI line is
// never merged (D3/S4).
func TestScorePreservesDealbreakerBeforeStage2(t *testing.T) {
	p := basePosting()
	p.Description = "병역특례 지원자 환영"
	prof := baseProfile()
	prof.Dealbreakers = []string{"병역특례"}

	ext := &ai.Extraction{Newcomer: true, EducationEnum: ai.EduNone}
	delta := &ai.Delta{NetDelta: 50, Items: []ai.DeltaItem{{Signal: "x", Kind: "presence", Delta: 50, Evidence: "y"}}}
	r := Score(p, prof, ext, delta, nil)
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
	r := Score(p, prof, nil, delta, nil)
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

func TestScoreResultUnmarshalsWithoutExclusionReasons(t *testing.T) {
	var got ScoreResult
	err := json.Unmarshal(
		[]byte(`{"Total":-1,"Breakdown":[],"DealbreakerHit":{"Kind":"keyword","Phrase":"리서치"}}`),
		&got,
	)
	if err != nil || got.ExclusionReasons != nil {
		t.Fatalf("legacy score JSON: result=%+v err=%v", got, err)
	}
}
