package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCSRFRejectsMissingToken(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)

	req := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader("career_years=0"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw-session"})
	rec := httptest.NewRecorder()

	srv.csrfProtect(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestCSRFRejectsWrongToken(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)

	req := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader("csrf_token=wrong"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf-cookie"})
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw-session"})
	rec := httptest.NewRecorder()

	srv.csrfProtect(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestCSRFAllowsValidToken(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)

	req := httptest.NewRequest(http.MethodPost, "/profile", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf-cookie"})
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw-session"})
	req.Header.Set(csrfHeaderName, srv.csrfToken("csrf-cookie", "raw-session"))
	rec := httptest.NewRecorder()
	called := false

	srv.csrfProtect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if !called {
		t.Fatal("handler was not called")
	}
}

func TestCSRFDoesNotBlockGET(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)

	req := httptest.NewRequest(http.MethodGet, "/profile", nil)
	rec := httptest.NewRecorder()
	called := false

	srv.csrfProtect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !called {
		t.Fatal("handler was not called")
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("GET did not set a CSRF cookie")
	}
}
