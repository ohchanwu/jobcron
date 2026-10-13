package server

import (
	"reflect"
	"testing"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
	"github.com/ohchanwu/jobcron/internal/scoring"
	"github.com/ohchanwu/jobcron/internal/scraper"
	"github.com/ohchanwu/jobcron/internal/storage"
)

func TestComposeStoredScoreSelectsWithoutMutatingCache(t *testing.T) {
	prof := profile.Profile{CareerYears: 0}
	p := scraper.Posting{ID: 7, Newcomer: true}
	old := ai.Delta{NetDelta: 7, Items: []ai.DeltaItem{{Kind: ai.KindPresence, Delta: 7, Evidence: "서버 개발 업무", MatchedGoal: "좋아하는 업무"}}}
	original := old
	got := composeStoredScore(storedScoreInputs{Posting: p, Profile: &prof, AIEnabled: true, Latest: &old})
	stale := old
	stale.Stale = true
	want := scoring.Score(p, prof, nil, &stale, nil)
	if !reflect.DeepEqual(got.Score, want) {
		t.Fatalf("composition = %+v, want %+v", got.Score, want)
	}
	if !reflect.DeepEqual(old, original) {
		t.Fatal("composition mutated shared delta")
	}
	if got.Outcome != "" || got.Fresh {
		t.Fatalf("old evidence manufactured a current outcome: %+v", got)
	}
}

func TestComposeStoredScoreFreshPrecedence(t *testing.T) {
	prof := profile.Profile{CareerYears: 0}
	p := scraper.Posting{ID: 7, Newcomer: true}
	old := ai.Delta{NetDelta: 7, Items: []ai.DeltaItem{{Kind: ai.KindPresence, Delta: 7, Evidence: "서버 개발 업무"}}}
	empty := ai.Delta{}
	got := composeStoredScore(storedScoreInputs{Posting: p, Profile: &prof, AIEnabled: true, Fresh: &empty, Latest: &old, Outcome: &storage.AIScoreOutcome{State: storage.AIScoreNoSignal}})
	if !reflect.DeepEqual(got.Score, scoring.Score(p, prof, nil, nil, nil)) || !got.Fresh || got.Outcome != storage.AIScoreNoSignal {
		t.Fatalf("genuine empty must supersede old evidence: %+v", got)
	}
}

func TestComposeStoredScoreOutcomePolicy(t *testing.T) {
	prof := profile.Profile{CareerYears: 0}
	p := scraper.Posting{Newcomer: true}
	rated := ai.Delta{Items: []ai.DeltaItem{{Kind: ai.KindPresence, Delta: 7}, {Kind: ai.KindAbsence, Delta: -7}}}
	old := ai.Delta{NetDelta: 8, Items: []ai.DeltaItem{{Kind: ai.KindPresence, Delta: 8}}}
	for _, tc := range []struct {
		name      string
		state     string
		fresh     *ai.Delta
		wantState string
		wantFresh bool
	}{
		{"net zero rated", storage.AIScoreRated, &rated, storage.AIScoreRated, true},
		{"missing provenance", "", &rated, "", false},
		{"orphan rated", storage.AIScoreRated, nil, "", false},
		{"orphan empty", storage.AIScoreNoSignal, nil, "", false},
		{"failed", storage.AIScoreFailed, nil, storage.AIScoreFailed, false},
		{"rejected", storage.AIScoreRejected, nil, storage.AIScoreRejected, false},
		{"never admitted", "", nil, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := composeStoredScore(storedScoreInputs{Posting: p, Profile: &prof, AIEnabled: true, Fresh: tc.fresh, Latest: &old, Outcome: &storage.AIScoreOutcome{State: tc.state}})
			if got.Outcome != tc.wantState || got.Fresh != tc.wantFresh {
				t.Fatalf("outcome = %+v", got)
			}
			if tc.fresh == nil && got.Score.Total != scoring.Score(p, prof, nil, &old, nil).Total {
				t.Fatal("unsuccessful/absent result lost supported evidence")
			}
		})
	}
}

func TestComposeStoredScoreMatchesDeterministicComposition(t *testing.T) {
	prof := profile.Profile{CareerYears: 0, Dealbreakers: []string{"야근"}}
	p := scraper.Posting{ID: 7, Newcomer: true, Description: "야근 없는 서버 개발 업무"}
	_, hash, _ := ai.ModelInput(p)
	candidate := scoring.DealbreakerCandidates(p, prof)[0]
	validation := ai.DealbreakerValidation{CandidateID: candidate.ID, Verdict: ai.DealbreakerNotApplicable}
	maxCareer := 10
	ext := ai.Extraction{Newcomer: false, MinCareer: 5, MaxCareer: &maxCareer}
	d := ai.Delta{NetDelta: 7, Items: []ai.DeltaItem{{Kind: ai.KindPresence, Delta: 7}}}
	for _, tc := range []struct {
		name                      string
		enabled, demo             bool
		content                   string
		wantDelta, wantValidation bool
	}{
		{"current content", true, false, hash, true, true},
		{"changed content", true, false, "old-content", true, false},
		{"AI off", false, false, hash, false, false},
		{"demo cached reading", false, true, hash, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := storedScoreInputs{Posting: p, Profile: &prof, AIEnabled: tc.enabled, Demo: tc.demo, Extraction: &ext, ContentHash: hash, AnyVersion: &d, Validations: map[string]storage.AIDealbreakerValidation{tc.content + "\x00" + candidate.ID: {Validation: validation}}}
			got := composeStoredScore(in)
			var vd map[string]ai.DealbreakerValidation
			if tc.wantValidation {
				vd = map[string]ai.DealbreakerValidation{candidate.ID: validation}
			}
			var delta *ai.Delta
			if tc.wantDelta {
				copy := d
				copy.Stale = !tc.demo
				delta = &copy
			}
			want := scoring.Score(p, prof, &ext, delta, vd)
			if !reflect.DeepEqual(got.Score, want) {
				t.Fatalf("got %+v, want %+v", got.Score, want)
			}
			if got.Score.Total < 0 && (got.Outcome != "" || got.Fresh) {
				t.Fatal("hard exclusion acquired AI outcome")
			}
		})
	}
	if got := composeStoredScore(storedScoreInputs{Posting: p}); !reflect.DeepEqual(got, storedScoreProjection{}) {
		t.Fatal("no-profile manufactured a score")
	}
}
