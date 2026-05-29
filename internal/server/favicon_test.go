package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ohchanwu/job-scraper/internal/profile"
)

func TestFaviconServedFromStatic(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	for _, path := range []string{"/static/favicon.svg", "/static/favicon.ico"} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200 (is it embedded?)", path, rec.Code)
		}
		if rec.Body.Len() == 0 {
			t.Errorf("GET %s served an empty body", path)
		}
	}
}

func TestFaviconRootRedirect(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/favicon.ico", nil))
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /favicon.ico = %d, want 301", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/static/favicon.ico" {
		t.Errorf("redirect Location = %q, want /static/favicon.ico", loc)
	}
}

func TestTemplatesReferenceFavicon(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	// A saved profile makes / render the dashboard rather than redirect.
	profJSON, _ := profile.Marshal(profile.Profile{})
	if _, _, err := st.SaveProfile(context.Background(), profJSON); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	for _, path := range []string{"/", "/archive", "/bookmarks", "/profile"} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		body := rec.Body.String()
		if !strings.Contains(body, `href="/static/favicon.svg"`) {
			t.Errorf("%s missing the favicon.svg link tag", path)
		}
		if !strings.Contains(body, `href="/static/favicon.ico"`) {
			t.Errorf("%s missing the favicon.ico link tag", path)
		}
	}
}
