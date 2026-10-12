package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
)

func TestGeminiSponsorAndUserRuntimePacing(t *testing.T) {
	for _, tc := range []struct {
		provider string
		spacing  time.Duration
	}{{"gemini", 10 * time.Second}, {"anthropic", 1200 * time.Millisecond}, {"openai", 1200 * time.Millisecond}} {
		t.Run(tc.provider, func(t *testing.T) {
			srv, st, sponsorID, userID, sponsor, user := newStage1SponsorTestServer(t, &fakeScraper{})
			cipher := newAIRuntimeTestCipher(t, 0x57)
			srv.SetCredentialCipher(cipher)
			sponsor.NameVal = tc.provider
			user.NameVal = tc.provider
			for _, entry := range []struct {
				id  int64
				key string
			}{{sponsorID, stage1SponsorFixture}, {userID, stage1TriggerFixture}} {
				saveAIRuntimeProfile(t, st, entry.id, profile.Profile{CareerYears: 0, AIProvider: tc.provider, AIPerCallCap: 17, JobLikes: "synthetic goal"})
				saveAIRuntimeCredential(t, st, cipher, entry.id, tc.provider, entry.key)
			}
			constructions := 0
			srv.newAIProvider = func(provider, key, model string, spacing time.Duration) (ai.Provider, error) {
				constructions++
				if provider != tc.provider || spacing != tc.spacing || model != ai.DefaultModel(tc.provider) {
					t.Errorf("factory provider=%s model=%s spacing=%v", provider, model, spacing)
				}
				switch key {
				case stage1SponsorFixture:
					return sponsor, nil
				case stage1TriggerFixture:
					return user, nil
				default:
					return nil, fmt.Errorf("unexpected synthetic payer")
				}
			}
			funding, err := srv.resolveStage1Funding(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			runtime, err := srv.aiRuntimeForUser(context.Background(), userID)
			if err != nil {
				t.Fatal(err)
			}
			if constructions != 2 || funding.userID != sponsorID || funding.runtime.Provider != sponsor || runtime.UserID != userID || runtime.Provider != user {
				t.Fatal("sponsor and user runtimes crossed")
			}
			if runtime.PerCallCap != 17 || runtime.ScoreVersion != ai.ScoreVersion(tc.provider, ai.DefaultModel(tc.provider)) || runtime.DealbreakerVersion != ai.DealbreakerVersion(tc.provider, ai.DefaultModel(tc.provider)) {
				t.Fatal("pacing changed cap or contract identity")
			}
		})
	}
}

func TestGeminiQuotaFailureDoesNotBecomeFreshSuccess(t *testing.T) {
	srv, _, runtime := seedRerate(t)
	const canary = "synthetic-tenant-key-JD-goal-canary"
	runtime.Provider = &ai.StubProvider{NameVal: "gemini", ScoreDeltaFn: func(context.Context, string, string) ([]ai.RawDeltaItem, ai.Usage, error) {
		return nil, ai.Usage{}, &ai.APIError{Provider: "gemini", Status: http.StatusTooManyRequests, Body: `{"error":{"message":"` + canary + `","details":[{"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[{"quotaId":"GenerateRequestsPerDayPerProjectPerModel-FreeTier"}]}]}}`}
	}}
	events := &captureEmit{}
	summary, err := srv.runRerate(context.Background(), "today", events.emit, 1, runtime)
	if err == nil || summary.Analyzed != 0 {
		t.Fatalf("quota failure became success: analyzed=%d err=%v", summary.Analyzed, err)
	}
	cached, err := srv.store.AIScoresByPostingID(context.Background(), 1, profile.AIInputHash(profile.Profile{CareerYears: 0, JobLikes: "백엔드 서버 개발"}), runtime.ScoreVersion)
	if err != nil {
		t.Fatal(err)
	}
	if len(cached) != 0 {
		t.Fatalf("quota failure wrote %d successes", len(cached))
	}
	for _, event := range events.snapshot() {
		if strings.Contains(event[1], canary) {
			t.Fatal("SSE leaked synthetic canary")
		}
	}
	if !strings.Contains(providerFailureMessage(&ai.APIError{Provider: "gemini", Status: 429, Body: `{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[{"quotaId":"GenerateRequestsPerDayPerProjectPerModel-FreeTier"}]}]}}`}), "일일") {
		t.Fatal("daily cause lost")
	}
}
