package ai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeminiTransportRejectsTruncatedClassificationPrefix(t *testing.T) {
	prefix := quotaFixture("", "GenerateRequestsPerDayPerProjectPerModel-FreeTier")
	cases := []struct{ name, body string }{
		{"oversized valid prefix", prefix + strings.Repeat(" ", (1<<20)-len(prefix)) + "unread-trailer"},
		{"truncated JSON", prefix[:len(prefix)-1]},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "43")
				w.WriteHeader(429)
				io.WriteString(w, tc.body)
			}))
			defer srv.Close()
			t.Logf("owned synthetic transport endpoint=%s", srv.URL)
			p, err := newHTTPProvider(geminiSpec, "synthetic", "model", srv.URL, 0)
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = p.complete(context.Background(), "synthetic goal", "synthetic JD")
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error type=%T", err)
			}
			if apiErr.GeminiQuota() != QuotaUnknown || apiErr.RetryDelay != 0 {
				t.Fatalf("untrusted prefix became scope=%s retry=%v", apiErr.GeminiQuota(), apiErr.RetryDelay)
			}
		})
	}
}

func TestGeminiDiagnosticsNeverIncludeOpaqueFields(t *testing.T) {
	const canary = "synthetic-project-key-JD-goal-URL-canary"
	body := strings.Replace(quotaFixture("", "GenerateRequestsPerMinutePerProjectPerModel-FreeTier"), `"error":{`, `"error":{"message":"`+canary+`","metadata":{"project":"`+canary+`","key":"`+canary+`","jd":"`+canary+`","goal":"`+canary+`","url":"https://`+canary+`.invalid"},`, 1)
	e := &APIError{Provider: "gemini", Status: 429, Body: body}
	if e.GeminiQuota() != QuotaMinuteRequests {
		t.Fatal("opaque fields altered explicit scope")
	}
	if strings.Contains(e.Error(), canary) {
		t.Fatal("opaque fields leaked into diagnostics")
	}
}
