package ai

import "context"

// StubProvider is a no-network Provider for the offline test suite (D9). Set
// ExtractFn to return canned output; a nil ExtractFn returns ErrNotImplemented.
// The whole Stage-2 offline suite leans on this seam.
type StubProvider struct {
	NameVal   string
	ExtractFn func(ctx context.Context, modelText string) (Extraction, Usage, error)

	// Calls counts how many times Extract was invoked — lets tests assert the
	// cache/budget short-circuits actually avoided a provider call.
	Calls int
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
	s.Calls++
	if s.ExtractFn == nil {
		return Extraction{}, Usage{}, ErrNotImplemented
	}
	return s.ExtractFn(ctx, modelText)
}
