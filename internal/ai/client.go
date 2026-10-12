package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/ohchanwu/jobcron/internal/pacing"
)

// providerSpec captures the per-provider differences (endpoint, auth header,
// request body shape, response envelope) behind one chassis. Anthropic and
// OpenAI differ only in a spec; the transport, pacing, and egress pin are
// shared.
type providerSpec struct {
	name           string
	defaultBaseURL string
	path           string // request path, e.g. "/v1/messages"
	// buildBody returns the provider-specific JSON request body for a single
	// system+user completion that must reply with JSON only.
	buildBody func(model, system, user string) any
	// setAuth sets the provider's auth + version headers on the request.
	setAuth func(h http.Header, apiKey string)
	// parseResp extracts the assistant's text output and token usage from a
	// 200 response body, normalizing both providers into the same shape.
	parseResp func(body []byte) (text string, usage Usage, err error)
}

// httpProvider shares pinned transport and provider-instance start pacing across
// extraction, contextual validation and scoring, with response latency overlap.
type httpProvider struct {
	spec    providerSpec
	apiKey  string
	model   string
	baseURL string
	http    *http.Client

	pacer *pacing.Pacer
}

// newHTTPProvider builds a provider against baseURL (defaulting to the spec's
// host) with its egress pinned to that host. rateLimit is the minimum spacing
// between requests; 0 disables pacing (tests).
func newHTTPProvider(spec providerSpec, apiKey, model, baseURL string, rateLimit time.Duration) (*httpProvider, error) {
	if baseURL == "" {
		baseURL = spec.defaultBaseURL
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("ai: parse %s base url %q: %w", spec.name, baseURL, err)
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("ai: %s base url %q has no host", spec.name, baseURL)
	}
	return &httpProvider{
		spec:    spec,
		apiKey:  apiKey,
		model:   model,
		baseURL: baseURL,
		http:    newPinnedHTTPClient(host, 60*time.Second),
		pacer:   pacing.New(rateLimit),
	}, nil
}

// Name reports the provider id.
func (p *httpProvider) Name() string { return p.spec.name }

// Extract is implemented in extract.go (the prompt + JSON/range gate on top
// of complete).

// complete performs one paced, pinned request: it sends a system+user prompt
// and returns the assistant's text output and token usage. The body is
// expected to be JSON-only; parsing/validating that JSON is the caller's job
// (T2's extraction gate). It is the load-bearing request/response helper the
// real providers share.
func (p *httpProvider) complete(ctx context.Context, system, user string) (string, Usage, error) {
	if err := ctx.Err(); err != nil {
		return "", Usage{}, err
	}
	body, err := json.Marshal(p.spec.buildBody(p.model, system, user))
	if err != nil {
		return "", Usage{}, fmt.Errorf("ai: %s marshal request: %w", p.spec.name, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+p.spec.path, bytes.NewReader(body))
	if err != nil {
		return "", Usage{}, fmt.Errorf("ai: %s build request: %w", p.spec.name, err)
	}
	req.Header.Set("Content-Type", "application/json")
	p.spec.setAuth(req.Header, p.apiKey)

	// Admission belongs at transport entry, after both our request preparation
	// and http.Client preparation. Keep the original pinned transport and client
	// policy; the shared pacer never locks response latency.
	transport := p.http.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client := &http.Client{Transport: pacedTransport{p.pacer, transport, p.http.Timeout},
		Jar: p.http.Jar, CheckRedirect: p.http.CheckRedirect}
	resp, err := client.Do(req)
	if err != nil {
		return "", Usage{}, fmt.Errorf("ai: %s request: %w", p.spec.name, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil {
		return "", Usage{}, fmt.Errorf("ai: %s read response: %w", p.spec.name, err)
	}
	if resp.StatusCode != http.StatusOK {
		apiErr := &APIError{Provider: p.spec.name, Status: resp.StatusCode}
		if len(respBody) > 1<<20 {
			// A truncated prefix cannot establish Gemini quota scope.
			if p.spec.name != "gemini" {
				apiErr.Body = string(respBody[:1<<20])
			}
		} else {
			apiErr.Body = string(respBody)
			if p.spec.name == "gemini" {
				apiErr.RetryDelay = geminiRetryDelay(apiErr.Body, resp.Header, time.Now())
			}
		}
		return "", Usage{}, apiErr
	}
	if len(respBody) > 1<<20 {
		respBody = respBody[:1<<20]
	}
	return p.spec.parseResp(respBody)
}

type pacedTransport struct {
	pacer   *pacing.Pacer
	base    http.RoundTripper
	timeout time.Duration
}

func (t pacedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.pacer.Wait(req.Context()); err != nil {
		return nil, err
	}
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	// Preserve the historical response timeout: admission waiting did not consume
	// it. The parent operation's deadline still bounds both waiting and response.
	if t.timeout <= 0 {
		return t.base.RoundTrip(req)
	}
	ctx, cancel := context.WithTimeout(req.Context(), t.timeout)
	resp, err := t.base.RoundTrip(req.Clone(ctx))
	if err != nil {
		cancel()
		return resp, err
	}
	resp.Body = cancelOnCloseBody{resp.Body, cancel}
	return resp, nil
}

type cancelOnCloseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b cancelOnCloseBody) Close() error {
	defer b.cancel()
	return b.ReadCloser.Close()
}

// APIError is a non-2xx response from a provider's API. It carries the HTTP
// status so a caller can tell a 401 (bad key) from a 404 (model not found) and
// surface a specific, calm message — the difference the BYOK user needs when a
// provider switch leaves a mismatched model or key behind. Match it with
// errors.As. Non-Gemini Error() retains the historical shape. Gemini diagnostics
// expose only HTTP status and a closed quota enum, never raw provider text.
type APIError struct {
	Provider string
	Status   int
	Body     string
	// RetryDelay carries validated Gemini metadata, never raw headers or retry authority.
	RetryDelay time.Duration
}

func (e *APIError) Error() string {
	if e.Provider == "gemini" {
		return fmt.Sprintf("ai: gemini status %d quota %s", e.Status, e.GeminiQuota())
	}
	return fmt.Sprintf("ai: %s status %d: %s", e.Provider, e.Status, truncateForError([]byte(e.Body)))
}

// truncateForError trims a response body for inclusion in an error message so
// a large error page doesn't bloat the log.
func truncateForError(b []byte) string {
	const max = 200
	s := string(b)
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

// specByName is the registry New switches on.
var specByName = map[string]providerSpec{
	anthropicSpec.name: anthropicSpec,
	openaiSpec.name:    openaiSpec,
	geminiSpec.name:    geminiSpec,
}
