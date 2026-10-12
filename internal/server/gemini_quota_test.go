package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/ohchanwu/jobcron/internal/ai"
)

func TestGeminiQuotaCopyUsesExplicitScope(t *testing.T) {
	const canary = "synthetic-private-quota-canary"
	cases := []struct{ name, body, want, reject string }{
		{"minute requests", `{"error":{"status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[{"quotaMetric":"generativelanguage.googleapis.com/generate_content_free_tier_requests","quotaId":"GenerateRequestsPerMinutePerProjectPerModel-FreeTier"}]}]}}`, "분당 요청", "일일"},
		{"minute tokens", `{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[{"quotaId":"GenerateContentInputTokensPerModelPerMinute-FreeTier"}]}]}}`, "분당 토큰", "일일"},
		{"daily with retry", `{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[{"quotaId":"GenerateRequestsPerDayPerProjectPerModel-FreeTier"}]},{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"5s"}]}}`, "일일", "잠시 후 다시"},
		{"mixed", `{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[{"quotaId":"GenerateRequestsPerMinutePerProjectPerModel-FreeTier"},{"quotaId":"GenerateRequestsPerDayPerProjectPerModel-FreeTier"}]}]}}`, "여러", "잠시 후 다시"},
		{"billing", `{"error":{"type":"billing_disabled"}}`, "결제", "분당"},
		{"generic compatible quota", `{"error":{"type":"insufficient_quota"}}`, "범위", "결제"},
		{"retry alone", `{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"5s"}]}}`, "범위", "잠시 후 다시"},
		{"generic quota", `{"error":{"message":"You exceeded your current quota","details":[{"@type":"type.googleapis.com/google.rpc.QuotaFailure"}]}}`, "범위", "일일"},
		{"unknown 429", `{}`, "범위", "잠시 후 다시"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := &ai.APIError{Provider: "gemini", Status: http.StatusTooManyRequests, Body: tc.body}
			got := providerFailureMessage(err)
			if !strings.Contains(got, tc.want) || strings.Contains(got, tc.reject) {
				t.Fatalf("copy=%q, want %q, reject %q", got, tc.want, tc.reject)
			}
			err.Body = strings.Replace(tc.body, `"error":{`, `"error":{"message":"`+canary+`",`, 1)
			if strings.Contains(providerFailureMessage(err), canary) || strings.Contains(err.Error(), canary) {
				t.Fatal("Gemini diagnostics leaked synthetic canary")
			}
		})
	}
}
