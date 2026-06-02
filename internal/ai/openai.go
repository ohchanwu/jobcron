package ai

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// openaiSpec shapes requests/responses for the OpenAI Chat Completions API.
// Auth is Authorization: Bearer; the assistant text is choices[0].message.content;
// usage is prompt_tokens / completion_tokens. response_format=json_object asks
// the model for a strict JSON reply (the extraction gate validates it anyway).
var openaiSpec = providerSpec{
	name:           "openai",
	defaultBaseURL: "https://api.openai.com",
	path:           "/v1/chat/completions",
	buildBody: func(model, system, user string) any {
		return openaiRequest{
			Model:          model,
			MaxTokens:      maxOutputTokens,
			ResponseFormat: openaiResponseFormat{Type: "json_object"},
			Messages: []openaiMessage{
				{Role: "system", Content: system},
				{Role: "user", Content: user},
			},
		}
	},
	setAuth: func(h http.Header, apiKey string) {
		h.Set("Authorization", "Bearer "+apiKey)
	},
	parseResp: func(body []byte) (string, Usage, error) {
		var r openaiResponse
		if err := json.Unmarshal(body, &r); err != nil {
			return "", Usage{}, fmt.Errorf("ai: openai decode response: %w", err)
		}
		var text string
		if len(r.Choices) > 0 {
			text = r.Choices[0].Message.Content
		}
		usage := Usage{InputTokens: r.Usage.PromptTokens, OutputTokens: r.Usage.CompletionTokens}
		return text, usage, nil
	},
}

type openaiRequest struct {
	Model          string               `json:"model"`
	MaxTokens      int                  `json:"max_tokens"`
	ResponseFormat openaiResponseFormat `json:"response_format"`
	Messages       []openaiMessage      `json:"messages"`
}

type openaiResponseFormat struct {
	Type string `json:"type"`
}

type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}
