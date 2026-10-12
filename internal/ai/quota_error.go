package ai

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// QuotaScope is explanatory only: neither scope nor retry metadata grants a retry.
type QuotaScope string

const (
	QuotaUnknown        QuotaScope = "unknown"
	QuotaMinuteRequests QuotaScope = "minute_requests"
	QuotaMinuteTokens   QuotaScope = "minute_tokens"
	QuotaDaily          QuotaScope = "daily"
	QuotaMixed          QuotaScope = "mixed"
	QuotaBilling        QuotaScope = "billing_account"
)

const (
	maxQuotaBody       = 1 << 20
	maxQuotaDepth      = 8
	maxQuotaNodes      = 512
	maxQuotaString     = 2048
	maxQuotaDetails    = 16
	maxQuotaViolations = 16
	maxQuotaRetry      = 24 * time.Hour
)

// GeminiQuota reads only the supported error object and typed Google RPC details.
// Messages, arbitrary wrappers and generic quota/retry markers cannot establish scope.
func (e *APIError) GeminiQuota() QuotaScope {
	if e.Provider != "gemini" || e.Status != http.StatusTooManyRequests {
		return QuotaUnknown
	}
	root, ok := quotaObject(e.Body)
	if !ok {
		return QuotaUnknown
	}
	envelope, ok := root["error"].(map[string]any)
	if !ok {
		return QuotaUnknown
	}
	if code, exists := envelope["code"]; exists && code != json.Number("429") && code != "429" {
		if text, ok := code.(string); !ok || (!billingReason(text) && text != "insufficient_quota" && text != "rate_limit_exceeded") {
			return QuotaUnknown
		}
	}
	if status, exists := envelope["status"]; exists && status != "RESOURCE_EXHAUSTED" {
		return QuotaUnknown
	}
	scopes := map[QuotaScope]bool{}
	billing := false
	if raw, exists := envelope["type"]; exists {
		if _, ok := raw.(string); !ok {
			return QuotaUnknown
		}
	}
	for _, key := range []string{"type", "code"} {
		if text, ok := envelope[key].(string); ok {
			billing = billing || billingReason(text)
		}
	}
	if raw, exists := envelope["details"]; exists {
		details, ok := raw.([]any)
		if !ok || len(details) > maxQuotaDetails {
			return QuotaUnknown
		}
		for _, raw := range details {
			detail, ok := raw.(map[string]any)
			if !ok {
				return QuotaUnknown
			}
			typ, ok := detail["@type"].(string)
			if !ok {
				return QuotaUnknown
			}
			switch typ {
			case "type.googleapis.com/google.rpc.QuotaFailure":
				raw, exists := detail["violations"]
				if !exists {
					continue
				}
				violations, ok := raw.([]any)
				if !ok || len(violations) > maxQuotaViolations {
					return QuotaUnknown
				}
				for _, raw := range violations {
					violation, ok := raw.(map[string]any)
					if !ok {
						return QuotaUnknown
					}
					for _, key := range []string{"quotaMetric", "quotaId"} {
						raw, exists := violation[key]
						if !exists {
							continue
						}
						text, ok := raw.(string)
						if !ok {
							return QuotaUnknown
						}
						// Keep the exact allowlist field-specific; an identifier in
						// the wrong field is not a supported metric/ID observation.
						if (key == "quotaMetric" && !strings.HasPrefix(text, "generativelanguage.googleapis.com/")) ||
							(key == "quotaId" && !strings.HasPrefix(text, "Generate")) {
							continue
						}
						if scope := geminiQuotaScopes[text]; scope != "" {
							scopes[scope] = true
						}
					}
				}
			case "type.googleapis.com/google.rpc.ErrorInfo":
				reason, ok := detail["reason"].(string)
				if !ok {
					return QuotaUnknown
				}
				billing = billing || billingReason(reason)
			}
		}
	}
	// An account prerequisite cannot be fixed by spacing requests.
	if billing {
		return QuotaBilling
	}
	if len(scopes) > 1 {
		return QuotaMixed
	}
	for scope := range scopes {
		return scope
	}
	return QuotaUnknown
}

// Exact identifiers only, not substring inference from descriptions or dimensions.
// Generic generate_content_* metrics are deliberately unscoped; their quotaId
// supplies the window. Unknown future metrics stay unknown until explicitly tested.
var geminiQuotaScopes = map[string]QuotaScope{
	"GenerateRequestsPerMinutePerProjectPerModel-FreeTier":                       QuotaMinuteRequests,
	"GenerateRequestsPerMinutePerProjectPerModel":                                QuotaMinuteRequests,
	"GenerateRequestsPerMinutePerProjectPerModel-PaidTier":                       QuotaMinuteRequests,
	"GenerateRequestsPerDayPerProjectPerModel-FreeTier":                          QuotaDaily,
	"GenerateRequestsPerDayPerProjectPerModel":                                   QuotaDaily,
	"GenerateRequestsPerDayPerProjectPerModel-PaidTier":                          QuotaDaily,
	"GenerateContentInputTokensPerModelPerMinute-FreeTier":                       QuotaMinuteTokens,
	"GenerateContentInputTokensPerModelPerMinute":                                QuotaMinuteTokens,
	"GenerateContentInputTokensPerModelPerMinute-PaidTier":                       QuotaMinuteTokens,
	"generativelanguage.googleapis.com/generate_content_requests_per_minute":     QuotaMinuteRequests,
	"generativelanguage.googleapis.com/generate_content_input_tokens_per_minute": QuotaMinuteTokens,
	"generativelanguage.googleapis.com/generate_content_requests_per_day":        QuotaDaily,
}

func billingReason(reason string) bool {
	switch reason {
	case "billing_disabled", "BILLING_DISABLED", "BILLING_NOT_ENABLED", "ACCOUNT_PREREQUISITE_REQUIRED":
		return true
	}
	return false
}

// quotaObject rejects duplicates, trailing/truncated JSON and excessive work even
// inside ignored fields. This is intentionally not a recursive envelope search.
func quotaObject(body string) (map[string]any, bool) {
	if len(body) == 0 || len(body) > maxQuotaBody {
		return nil, false
	}
	dec := json.NewDecoder(strings.NewReader(body))
	dec.UseNumber()
	nodes := 0
	value, ok := quotaValue(dec, 0, &nodes)
	if !ok {
		return nil, false
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, false
	}
	root, ok := value.(map[string]any)
	return root, ok
}

func quotaValue(dec *json.Decoder, depth int, nodes *int) (any, bool) {
	(*nodes)++
	if depth > maxQuotaDepth || *nodes > maxQuotaNodes {
		return nil, false
	}
	token, err := dec.Token()
	if err != nil {
		return nil, false
	}
	if text, ok := token.(string); ok && len(text) > maxQuotaString {
		return nil, false
	}
	switch token {
	case json.Delim('{'):
		result := map[string]any{}
		for dec.More() {
			key, err := dec.Token()
			text, ok := key.(string)
			if err != nil || !ok || len(text) > maxQuotaString {
				return nil, false
			}
			if _, exists := result[text]; exists {
				return nil, false
			}
			child, ok := quotaValue(dec, depth+1, nodes)
			if !ok {
				return nil, false
			}
			result[text] = child
		}
		end, err := dec.Token()
		return result, err == nil && end == json.Delim('}')
	case json.Delim('['):
		result := []any{}
		for dec.More() {
			child, ok := quotaValue(dec, depth+1, nodes)
			if !ok {
				return nil, false
			}
			result = append(result, child)
		}
		end, err := dec.Token()
		return result, err == nil && end == json.Delim(']')
	default:
		if _, ok := token.(json.Delim); ok {
			return nil, false
		}
		return token, true
	}
}

// Retry hints are bounded metadata only; conflicting or malformed hints vanish.
// No caller retries on them.
func geminiRetryDelay(body string, header http.Header, now time.Time) time.Duration {
	root, ok := quotaObject(body)
	if !ok {
		return 0
	}
	envelope, ok := root["error"].(map[string]any)
	if !ok {
		return 0
	}
	var delay time.Duration
	if values := header.Values("Retry-After"); len(values) != 0 {
		if len(values) != 1 || len(values[0]) > 64 {
			return 0
		}
		value := values[0]
		if seconds, err := strconv.ParseUint(value, 10, 32); err == nil {
			delay = time.Duration(seconds) * time.Second
		} else if at, err := http.ParseTime(value); err == nil {
			delay = at.Sub(now)
		} else {
			return 0
		}
		if delay <= 0 || delay > maxQuotaRetry {
			return 0
		}
	}
	if raw, exists := envelope["details"]; exists {
		details, ok := raw.([]any)
		if !ok || len(details) > maxQuotaDetails {
			return 0
		}
		seen := false
		for _, raw := range details {
			detail, ok := raw.(map[string]any)
			if !ok {
				return 0
			}
			if detail["@type"] != "type.googleapis.com/google.rpc.RetryInfo" {
				continue
			}
			if seen {
				return 0
			}
			seen = true
			value, ok := detail["retryDelay"].(string)
			if !ok || len(value) > 32 || !strings.HasSuffix(value, "s") {
				return 0
			}
			// Google's JSON Duration, not arbitrary Go duration syntax.
			parts := strings.Split(strings.TrimSuffix(value, "s"), ".")
			if len(parts) > 2 || len(parts[0]) == 0 {
				return 0
			}
			for i, part := range parts {
				if len(part) == 0 || (i == 1 && len(part) > 9) {
					return 0
				}
				for _, ch := range part {
					if ch < '0' || ch > '9' {
						return 0
					}
				}
			}
			rpc, err := time.ParseDuration(value)
			if err != nil || rpc <= 0 || rpc > maxQuotaRetry {
				return 0
			}
			if delay != 0 && delay != rpc {
				return 0
			}
			delay = rpc
		}
	}
	return delay
}
