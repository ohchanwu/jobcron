package ai

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// anthropicVersion is the Anthropic Messages API version header. Pinned; bump
// deliberately if the request/response contract changes.
const anthropicVersion = "2023-06-01"

// anthropicSpec shapes requests/responses for the Anthropic Messages API.
// Auth is x-api-key + anthropic-version; the assistant text is the first text
// block of content[]; usage is input_tokens / output_tokens.
var anthropicSpec = providerSpec{
	name:           "anthropic",
	defaultBaseURL: "https://api.anthropic.com",
	path:           "/v1/messages",
	buildBody: func(model, system, user string) any {
		return anthropicRequest{
			Model:     model,
			System:    system,
			MaxTokens: maxOutputTokens,
			Messages:  []anthropicMessage{{Role: "user", Content: user}},
		}
	},
	setAuth: func(h http.Header, apiKey string) {
		h.Set("x-api-key", apiKey)
		h.Set("anthropic-version", anthropicVersion)
	},
	parseResp: func(body []byte) (string, Usage, error) {
		var r anthropicResponse
		if err := json.Unmarshal(body, &r); err != nil {
			return "", Usage{}, fmt.Errorf("ai: anthropic decode response: %w", err)
		}
		var text string
		for _, c := range r.Content {
			if c.Type == "text" {
				text += c.Text
			}
		}
		usage := Usage{InputTokens: r.Usage.InputTokens, OutputTokens: r.Usage.OutputTokens}
		return text, usage, nil
	},
}

// maxOutputTokens bounds the model's reply. Stage-1 extraction replies are
// tiny (measured max ~120 output tokens), Stage-1B contextual validation stays
// bounded by its short evidence quotes, but Stage-2 ScoreDelta carries an
// evidence quote per signal and runs large: a live measurement over the real
// corpus (2026-06-08, Haiku) saw successful ScoreDelta replies up to ~820
// output tokens and a reply that hit the OLD 1024 ceiling and truncated
// mid-JSON — an "unexpected end of JSON input" that DROPPED the whole delta.
// Raised to 2048 (≈2.5× the largest successful reply) so a signal-rich delta
// completes. The cap only ever bites a reply that would exceed it; billing is
// per ACTUAL output token, so the headroom is free for the common case.
const maxOutputTokens = 2048

type anthropicRequest struct {
	Model     string             `json:"model"`
	System    string             `json:"system,omitempty"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}
