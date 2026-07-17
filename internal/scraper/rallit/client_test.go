package rallit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckAccessRejectsWhenDisallowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /api\n"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := newClient(srv.URL, 0)
	if err := c.checkAccess(context.Background()); err == nil {
		t.Error("checkAccess = nil, want an error when robots.txt disallows /api")
	}
}
