package worknet

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestCallUsesCanonicalUserAgent(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("User-Agent"), "jobcron/0.1 (+github.com/ohchanwu/jobcron)"; got != want {
			t.Errorf("User-Agent = %q, want %q", got, want)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newClient(server.URL, "test-key", 0)
	if _, err := client.call(context.Background(), url.Values{"callTp": {"L"}}); err != nil {
		t.Fatalf("call: %v", err)
	}
}
