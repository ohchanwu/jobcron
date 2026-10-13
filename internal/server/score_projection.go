package server

import (
	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
	"github.com/ohchanwu/jobcron/internal/scoring"
	"github.com/ohchanwu/jobcron/internal/scraper"
	"github.com/ohchanwu/jobcron/internal/storage"
)

// storedScoreInputs contains only committed, identity-selected cache data.
// Callers select extraction content/contract and user/goal/version before passing
// these values. Composition has no storage, runtime, credential or provider seam.
// Inputs may be shared with other projections and are never mutated.
type storedScoreInputs struct {
	Posting     scraper.Posting
	Profile     *profile.Profile
	AIEnabled   bool
	Fresh       *ai.Delta
	Latest      *ai.Delta
	Outcome     *storage.AIScoreOutcome
	AnyVersion  *ai.Delta
	Demo        bool
	Extraction  *ai.Extraction
	ContentHash string
	Validations map[string]storage.AIDealbreakerValidation
}

type storedScoreProjection struct {
	Score   scoring.ScoreResult
	Outcome string
	Fresh   bool
}

func composeStoredScore(in storedScoreInputs) storedScoreProjection {
	if in.Profile == nil {
		return storedScoreProjection{}
	}
	var delta *ai.Delta
	if in.AIEnabled && in.Fresh != nil {
		copy := *in.Fresh
		copy.Stale = false
		delta = &copy
	} else if in.AIEnabled && in.Latest != nil {
		copy := *in.Latest
		copy.Stale = true
		delta = &copy
	} else if (in.AIEnabled || in.Demo) && in.AnyVersion != nil {
		copy := *in.AnyVersion
		copy.Stale = !in.Demo
		delta = &copy
	}
	var validations map[string]ai.DealbreakerValidation
	if in.AIEnabled && len(in.Validations) > 0 {
		validations = make(map[string]ai.DealbreakerValidation)
		for _, candidate := range scoring.DealbreakerCandidates(in.Posting, *in.Profile) {
			if row, ok := in.Validations[in.ContentHash+"\x00"+candidate.ID]; ok {
				validations[candidate.ID] = row.Validation
			}
		}
	}
	out := storedScoreProjection{Score: scoring.Score(in.Posting, *in.Profile, in.Extraction, delta, validations)}
	if in.AIEnabled && in.Outcome != nil && out.Score.Total >= 0 {
		switch in.Outcome.State {
		case storage.AIScoreRated, storage.AIScoreNoSignal:
			if in.Fresh != nil {
				out.Outcome = in.Outcome.State
				out.Fresh = true
			}
		case storage.AIScoreRejected, storage.AIScoreFailed:
			out.Outcome = in.Outcome.State
		}
	}
	return out
}
