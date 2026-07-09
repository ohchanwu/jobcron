package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ohchanwu/job-scraper/internal/auth"
)

func TestLoginRateLimitBlocksSixthFailedAttemptUntilWindowExpires(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	now := time.Date(2026, 7, 10, 1, 0, 0, 0, time.UTC)
	srv.loginLimiter.now = func() time.Time { return now }
	hash, err := auth.HashPassword("correct-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if _, err := st.CreateOwnerUser(context.Background(), "owner@example.com", hash); err != nil {
		t.Fatalf("CreateOwnerUser: %v", err)
	}

	for i := 0; i < 5; i++ {
		rec := postLogin(t, srv, "198.51.100.10:1234", "owner@example.com", "wrong-password")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401", i+1, rec.Code)
		}
	}

	rec := postLogin(t, srv, "198.51.100.10:1234", "owner@example.com", "wrong-password")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("sixth status = %d, want 429", rec.Code)
	}

	now = now.Add(loginRateLimitWindow + time.Second)
	rec = postLogin(t, srv, "198.51.100.10:1234", "owner@example.com", "wrong-password")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("after window status = %d, want 401", rec.Code)
	}
}

func TestLoginRateLimitDoesNotTrustForwardedFor(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	hash, err := auth.HashPassword("correct-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if _, err := st.CreateOwnerUser(context.Background(), "owner@example.com", hash); err != nil {
		t.Fatalf("CreateOwnerUser: %v", err)
	}

	for i := 0; i < 5; i++ {
		rec := postLoginWithForwardedFor(t, srv, "198.51.100.10:1234", fmt.Sprintf("203.0.113.%d", i+1), "owner@example.com", "wrong-password")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401", i+1, rec.Code)
		}
	}

	rec := postLoginWithForwardedFor(t, srv, "198.51.100.10:1234", "203.0.113.99", "owner@example.com", "wrong-password")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("sixth status = %d, want 429", rec.Code)
	}
}

func TestLoginRateLimitTrustsForwardedForFromPrivateProxy(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	hash, err := auth.HashPassword("correct-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if _, err := st.CreateOwnerUser(context.Background(), "owner@example.com", hash); err != nil {
		t.Fatalf("CreateOwnerUser: %v", err)
	}

	for i := 0; i < 5; i++ {
		rec := postLoginWithForwardedFor(t, srv, "172.18.0.2:1234", "203.0.113.10", "owner@example.com", "wrong-password")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401", i+1, rec.Code)
		}
	}

	rec := postLoginWithForwardedFor(t, srv, "172.18.0.2:1234", "203.0.113.11", "owner@example.com", "wrong-password")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("different forwarded client status = %d, want 401", rec.Code)
	}

	rec = postLoginWithForwardedFor(t, srv, "172.18.0.2:1234", "203.0.113.10", "owner@example.com", "wrong-password")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("sixth original client status = %d, want 429", rec.Code)
	}
}

func TestLoginRateLimitPrunesExpiredAttempts(t *testing.T) {
	limiter := newLoginRateLimiter()
	now := time.Date(2026, 7, 10, 1, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }

	limiter.recordFailure("198.51.100.1", "first@example.com")
	limiter.recordFailure("198.51.100.2", "second@example.com")
	if got := len(limiter.attempts); got != 2 {
		t.Fatalf("attempts before expiry = %d, want 2", got)
	}

	now = now.Add(loginRateLimitWindow + time.Second)
	if !limiter.allow("198.51.100.3", "third@example.com") {
		t.Fatal("new key should be allowed after pruning")
	}
	if got := len(limiter.attempts); got != 0 {
		t.Fatalf("attempts after pruning = %d, want 0", got)
	}
}

func postLogin(t *testing.T, srv *Server, remoteAddr, email, password string) *httptest.ResponseRecorder {
	return postLoginWithForwardedFor(t, srv, remoteAddr, "", email, password)
}

func postLoginWithForwardedFor(t *testing.T, srv *Server, remoteAddr, forwardedFor, email, password string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"email": {email}, "password": {password}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.RemoteAddr = remoteAddr
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if forwardedFor != "" {
		req.Header.Set("X-Forwarded-For", forwardedFor)
	}
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf-cookie"})
	req.Header.Set(csrfHeaderName, srv.csrfToken("csrf-cookie", ""))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}
