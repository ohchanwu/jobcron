package ai

import (
	"context"
	"sync"
)

// StubProvider is a no-network Provider for the offline test suite (D9). Set
// ExtractFn / ScoreDeltaFn to return canned output; a nil func returns
// ErrNotImplemented. The whole AI offline suite leans on this seam.
type StubProvider struct {
	NameVal      string
	ExtractFn    func(ctx context.Context, modelText string) (Extraction, Usage, error)
	ScoreDeltaFn func(ctx context.Context, modelText, profileText string) ([]RawDeltaItem, Usage, error)

	// Calls / ScoreDeltaCalls count invocations — lets tests assert the
	// cache/budget short-circuits actually avoided a provider call. mu guards
	// the increments so the concurrent 재평가 worker pool can drive the stub
	// from several goroutines; tests read the counts after the run completes
	// (causally ordered after all writes).
	mu              sync.Mutex
	Calls           int
	ScoreDeltaCalls int
}

// StubProvider implements Provider.
var _ Provider = (*StubProvider)(nil)

// Name returns NameVal, defaulting to "stub" when unset.
func (s *StubProvider) Name() string {
	if s.NameVal == "" {
		return "stub"
	}
	return s.NameVal
}

// Extract delegates to ExtractFn, counting the call. A nil ExtractFn returns
// ErrNotImplemented.
func (s *StubProvider) Extract(ctx context.Context, modelText string) (Extraction, Usage, error) {
	s.mu.Lock()
	s.Calls++
	s.mu.Unlock()
	if s.ExtractFn == nil {
		return Extraction{}, Usage{}, ErrNotImplemented
	}
	return s.ExtractFn(ctx, modelText)
}

// ScoreDelta delegates to ScoreDeltaFn, counting the call. A nil ScoreDeltaFn
// returns ErrNotImplemented.
func (s *StubProvider) ScoreDelta(ctx context.Context, modelText, profileText string) ([]RawDeltaItem, Usage, error) {
	s.mu.Lock()
	s.ScoreDeltaCalls++
	s.mu.Unlock()
	if s.ScoreDeltaFn == nil {
		return nil, Usage{}, ErrNotImplemented
	}
	return s.ScoreDeltaFn(ctx, modelText, profileText)
}
