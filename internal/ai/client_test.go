package ai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAnthropicClientParsesMessagesResponse drives the shared `complete`
// helper against a canned Anthropic /v1/messages body, asserting the request
// carried the right auth/version headers and path, and that the assistant
// text + token usage are parsed.
func TestAnthropicClientParsesMessagesResponse(t *testing.T) {
	var gotPath, gotKey, gotVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		io.WriteString(w, `{"content":[{"type":"text","text":"hello"}],"usage":{"input_tokens":11,"output_tokens":7}}`)
	}))
	defer srv.Close()

	p, err := newHTTPProvider(anthropicSpec, "sk-test", "claude-x", srv.URL, 0)
	if err != nil {
		t.Fatalf("newHTTPProvider: %v", err)
	}
	text, usage, err := p.complete(context.Background(), "system", "user")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if text != "hello" {
		t.Fatalf("text = %q, want %q", text, "hello")
	}
	if usage.InputTokens != 11 || usage.OutputTokens != 7 {
		t.Fatalf("usage = %+v, want {11 7}", usage)
	}
	if gotPath != "/v1/messages" {
		t.Fatalf("path = %q, want /v1/messages", gotPath)
	}
	if gotKey != "sk-test" {
		t.Fatalf("x-api-key = %q, want sk-test", gotKey)
	}
	if gotVersion != anthropicVersion {
		t.Fatalf("anthropic-version = %q, want %q", gotVersion, anthropicVersion)
	}
}

// TestOpenAIClientParsesChatCompletionResponse is symmetric for OpenAI.
func TestOpenAIClientParsesChatCompletionResponse(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		io.WriteString(w, `{"choices":[{"message":{"content":"world"}}],"usage":{"prompt_tokens":5,"completion_tokens":9}}`)
	}))
	defer srv.Close()

	p, err := newHTTPProvider(openaiSpec, "sk-oa", "gpt-x", srv.URL, 0)
	if err != nil {
		t.Fatalf("newHTTPProvider: %v", err)
	}
	text, usage, err := p.complete(context.Background(), "system", "user")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if text != "world" {
		t.Fatalf("text = %q, want %q", text, "world")
	}
	if usage.InputTokens != 5 || usage.OutputTokens != 9 {
		t.Fatalf("usage = %+v, want {5 9}", usage)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("path = %q, want /v1/chat/completions", gotPath)
	}
	if gotAuth != "Bearer sk-oa" {
		t.Fatalf("Authorization = %q, want Bearer sk-oa", gotAuth)
	}
}

func TestCompleteNon200IsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"bad key"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	p, err := newHTTPProvider(anthropicSpec, "sk-bad", "claude-x", srv.URL, 0)
	if err != nil {
		t.Fatalf("newHTTPProvider: %v", err)
	}
	if _, _, err := p.complete(context.Background(), "s", "u"); err == nil {
		t.Fatal("a 401 response must be an error")
	}
}

func TestNewFactory(t *testing.T) {
	for _, name := range []string{"anthropic", "openai"} {
		p, err := New(name, "sk", "model", 0)
		if err != nil {
			t.Fatalf("New(%q): %v", name, err)
		}
		if p.Name() != name {
			t.Fatalf("New(%q).Name() = %q", name, p.Name())
		}
	}
	if _, err := New("groq", "sk", "model", 0); !errors.Is(err, ErrUnknownProvider) {
		t.Fatalf("New(unknown) err = %v, want ErrUnknownProvider", err)
	}
}

// Both live providers satisfy Provider.
var (
	_ Provider = (*httpProvider)(nil)
)
