package ai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func quotaFixture(metric, id string) string {
	return fmt.Sprintf(`{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[{"quotaMetric":%q,"quotaId":%q}]}]}}`, metric, id)
}

func TestGeminiQuotaStructuredBounds(t *testing.T) {
	minute := quotaFixture("generativelanguage.googleapis.com/generate_content_free_tier_requests", "GenerateRequestsPerMinutePerProjectPerModel-FreeTier")
	daily := quotaFixture("generativelanguage.googleapis.com/generate_content_free_tier_requests", "GenerateRequestsPerDayPerProjectPerModel-FreeTier")
	cases := []struct {
		name, body string
		want       QuotaScope
	}{
		{"native minute", minute, QuotaMinuteRequests},
		{"metric cannot carry id", quotaFixture("GenerateRequestsPerMinutePerProjectPerModel-FreeTier", ""), QuotaUnknown},
		{"id cannot carry metric", quotaFixture("", "generativelanguage.googleapis.com/generate_content_requests_per_minute"), QuotaUnknown},
		{"wrong compatible type", strings.Replace(minute, `"code":429`, `"type":42`, 1), QuotaUnknown},
		{"minute metric", quotaFixture("generativelanguage.googleapis.com/generate_content_requests_per_minute", ""), QuotaMinuteRequests},
		{"token metric", quotaFixture("generativelanguage.googleapis.com/generate_content_input_tokens_per_minute", ""), QuotaMinuteTokens},
		{"daily metric", quotaFixture("generativelanguage.googleapis.com/generate_content_requests_per_day", ""), QuotaDaily},
		{"daily", daily, QuotaDaily},
		{"compatible quota code with daily detail", strings.Replace(daily, `"code":429`, `"code":"insufficient_quota"`, 1), QuotaDaily},
		{"compatible rate code with minute detail", strings.Replace(minute, `"code":429`, `"code":"rate_limit_exceeded"`, 1), QuotaMinuteRequests},
		{"conflicting metric and id", quotaFixture("generativelanguage.googleapis.com/generate_content_requests_per_minute", "GenerateRequestsPerDayPerProjectPerModel-FreeTier"), QuotaMixed},
		{"unknown metric", quotaFixture("invented-requests-per-minute", "invented-per-day"), QuotaUnknown},
		{"unscoped metric", quotaFixture("generativelanguage.googleapis.com/generate_content_free_tier_requests", ""), QuotaUnknown},
		{"compatible billing code", `{"error":{"code":"billing_disabled"}}`, QuotaBilling},
		{"generic compatible quota", `{"error":{"code":"insufficient_quota"}}`, QuotaUnknown},
		{"account", `{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"ACCOUNT_PREREQUISITE_REQUIRED"}]}}`, QuotaBilling},
		{"unsupported wrapper", `{"response":` + minute + `}`, QuotaUnknown},
		{"nested details", `{"error":{"metadata":` + minute + `}}`, QuotaUnknown},
		{"message not scope", `{"error":{"message":"GenerateRequestsPerDayPerProjectPerModel-FreeTier RESOURCE_EXHAUSTED QuotaFailure"}}`, QuotaUnknown},
		{"wrong status", strings.Replace(minute, "RESOURCE_EXHAUSTED", "INTERNAL", 1), QuotaUnknown},
		{"wrong code", strings.Replace(minute, `"code":429`, `"code":500`, 1), QuotaUnknown},
		{"wrong quota type", strings.Replace(minute, `"quotaId":"GenerateRequestsPerMinutePerProjectPerModel-FreeTier"`, `"quotaId":42`, 1), QuotaUnknown},
		{"wrong details type", `{"error":{"details":{}}}`, QuotaUnknown},
		{"wrong violations type", `{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":null}]}}`, QuotaUnknown},
		{"wrong detail type", `{"error":{"details":[null]}}`, QuotaUnknown},
		{"wrong rpc type", strings.Replace(minute, "google.rpc.QuotaFailure", "other.QuotaFailure", 1), QuotaUnknown},
		{"duplicate scope", strings.Replace(minute, `"quotaId":`, `"quotaId":"GenerateRequestsPerDayPerProjectPerModel-FreeTier","quotaId":`, 1), QuotaUnknown},
		{"duplicate envelope", `{"error":{},"error":` + strings.TrimSuffix(strings.TrimPrefix(minute, `{"error":`), `}`) + `}`, QuotaUnknown},
		{"truncated", minute[:len(minute)-1], QuotaUnknown},
		{"trailing", minute + ` {}`, QuotaUnknown},
		{"oversized", strings.Repeat(" ", 1<<20) + minute, QuotaUnknown},
		{"long field", strings.Replace(minute, `"code":429`, `"message":"`+strings.Repeat("x", 2049)+`"`, 1), QuotaUnknown},
		{"depth", `{"ignored":` + strings.Repeat("[", 9) + `0` + strings.Repeat("]", 9) + `,"error":` + strings.TrimSuffix(strings.TrimPrefix(minute, `{"error":`), `}`) + `}`, QuotaUnknown},
		{"nodes", `{"ignored":[` + strings.Repeat("0,", 512) + `0],"error":{}}`, QuotaUnknown},
		{"detail count", `{"error":{"details":[` + strings.Repeat(`{"@type":"type.googleapis.com/google.rpc.RetryInfo"},`, 16) + `{"@type":"type.googleapis.com/google.rpc.RetryInfo"}]}}`, QuotaUnknown},
		{"violation count", `{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[` + strings.Repeat(`{"quotaId":"GenerateRequestsPerMinutePerProjectPerModel-FreeTier"},`, 16) + `{}]}]}}`, QuotaUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &APIError{Provider: "gemini", Status: 429, Body: tc.body}
			if got := e.GeminiQuota(); got != tc.want {
				t.Fatalf("scope=%s want %s", got, tc.want)
			}
			for _, provider := range []string{"anthropic", "openai", ""} {
				e.Provider = provider
				if e.GeminiQuota() != QuotaUnknown {
					t.Fatal("scope crossed provider boundary")
				}
			}
		})
	}
	for _, status := range []int{400, 401, 403, 404, 500, 529} {
		if (&APIError{Provider: "gemini", Status: status, Body: minute}).GeminiQuota() != QuotaUnknown {
			t.Fatal("nonquota status inferred scope")
		}
	}
}

func TestGeminiRetryMetadataTransport(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	cases := []struct {
		name, body string
		headers    []string
		want       time.Duration
	}{
		{"delta", `{"error":{}}`, []string{"43"}, 43 * time.Second},
		{"date", `{"error":{}}`, []string{now.Add(time.Minute).Format(http.TimeFormat)}, time.Minute},
		{"rpc delay", `{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"5.25s"}]}}`, nil, 5250 * time.Millisecond},
		{"conflict", `{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"5s"}]}}`, []string{"43"}, 0},
		{"daily hint", quotaFixture("", "GenerateRequestsPerDayPerProjectPerModel-FreeTier"), []string{"43"}, 43 * time.Second},
	}
	for _, value := range []string{"-1", "+5", "0", "1.5", "99999999999999999999", "86401", "junk", "5, 6", strings.Repeat("x", 65), now.Add(-time.Minute).Format(http.TimeFormat)} {
		cases = append(cases, struct {
			name, body string
			headers    []string
			want       time.Duration
		}{"bad header " + value, `{"error":{}}`, []string{value}, 0})
	}
	cases = append(cases, struct {
		name, body string
		headers    []string
		want       time.Duration
	}{"duplicate headers", `{"error":{}}`, []string{"5", "6"}, 0})
	for _, value := range []string{"-5s", "0s", "25h", "1m", "86401s", "NaNs", "999999999999999999999s", "5s-private-canary"} {
		cases = append(cases, struct {
			name, body string
			headers    []string
			want       time.Duration
		}{"bad rpc " + value, `{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"` + value + `"}]}}`, nil, 0})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for _, v := range tc.headers {
					w.Header().Add("Retry-After", v)
				}
				w.Header().Set("Private-Header", "synthetic-private-retry-canary")
				w.WriteHeader(429)
				io.WriteString(w, tc.body)
			}))
			defer srv.Close()
			p, _ := newHTTPProvider(geminiSpec, "synthetic", "model", srv.URL, 0)
			_, _, err := p.complete(context.Background(), "synthetic goal", "synthetic JD")
			var e *APIError
			if !errors.As(err, &e) {
				t.Fatalf("error type=%T", err)
			}
			field := reflect.ValueOf(e).Elem().FieldByName("RetryDelay")
			if !field.IsValid() {
				t.Fatal("validated RetryDelay metadata missing")
			}
			got := time.Duration(field.Int())
			// HTTP dates are sampled at reception, not at this fixture's construction.
			if tc.name == "date" {
				if got <= 0 || got > tc.want {
					t.Fatalf("date delay=%v", got)
				}
			} else if got != tc.want {
				t.Fatalf("retry=%v want %v", got, tc.want)
			}
			if strings.Contains(e.Error(), "synthetic-private-retry-canary") {
				t.Fatal("header leaked")
			}
		})
	}
}
