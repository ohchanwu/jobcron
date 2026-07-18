package ai

import (
	"context"
	"errors"
	"testing"
)

func TestStubProviderImplementsProvider(t *testing.T) {
	var _ Provider = (*StubProvider)(nil)

	want := Extraction{MinCareer: 0, Newcomer: true, EducationEnum: "bachelor", CareerEvidence: "신입 환영", EducationEvidence: "학사 이상"}
	s := &StubProvider{
		NameVal: "stub",
		ExtractFn: func(ctx context.Context, modelText string) (Extraction, Usage, error) {
			return want, Usage{InputTokens: 3, OutputTokens: 2}, nil
		},
	}
	got, usage, err := s.Extract(context.Background(), "any")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if got != want {
		t.Fatalf("Extract = %+v, want %+v", got, want)
	}
	if usage.InputTokens != 3 || usage.OutputTokens != 2 {
		t.Fatalf("usage = %+v", usage)
	}
	if s.Calls != 1 {
		t.Fatalf("Calls = %d, want 1", s.Calls)
	}
}

func TestStubProviderNilFnIsNotImplemented(t *testing.T) {
	s := &StubProvider{}
	if _, _, err := s.Extract(context.Background(), "x"); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("nil ExtractFn err = %v, want ErrNotImplemented", err)
	}
	if s.Name() != "stub" {
		t.Fatalf("default Name() = %q, want stub", s.Name())
	}
	if _, _, err := s.ValidateDealbreakers(context.Background(), "x", nil); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("nil ValidateDealbreakersFn err = %v, want ErrNotImplemented", err)
	}
}
