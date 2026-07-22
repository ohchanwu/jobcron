package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/auth"
)

func TestLoginRateLimitBlocksSixthFailedAttemptUntilWindowExpires(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	now := time.Date(2026, 7, 10, 1, 0, 0, 0, time.UTC)
	srv.loginLimiter.now = func() time.Time { return now }
	srv.loginIPLimiter.now = func() time.Time { return now }
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

func TestLoginRateLimitTrustsForwardedForWithProxySecret(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	srv.SetProxySecret("proxy-secret")
	hash, err := auth.HashPassword("correct-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if _, err := st.CreateOwnerUser(context.Background(), "owner@example.com", hash); err != nil {
		t.Fatalf("CreateOwnerUser: %v", err)
	}

	for i := 0; i < 5; i++ {
		rec := postLoginWithForwardedForAndProxySecret(t, srv, "172.18.0.2:1234", "203.0.113.10", "proxy-secret", "owner@example.com", "wrong-password")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401", i+1, rec.Code)
		}
	}

	rec := postLoginWithForwardedForAndProxySecret(t, srv, "172.18.0.2:1234", "203.0.113.11", "proxy-secret", "owner@example.com", "wrong-password")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("different forwarded client status = %d, want 401", rec.Code)
	}

	rec = postLoginWithForwardedForAndProxySecret(t, srv, "172.18.0.2:1234", "203.0.113.10", "proxy-secret", "owner@example.com", "wrong-password")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("sixth original client status = %d, want 429", rec.Code)
	}
}

func TestLoginRateLimitDoesNotTrustPrivatePeerForwardedForWithoutProxySecret(t *testing.T) {
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
		rec := postLoginWithForwardedFor(t, srv, "10.0.0.10:1234", fmt.Sprintf("203.0.113.%d", i+1), "owner@example.com", "wrong-password")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401", i+1, rec.Code)
		}
	}

	rec := postLoginWithForwardedFor(t, srv, "10.0.0.10:1234", "203.0.113.99", "owner@example.com", "wrong-password")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("sixth status = %d, want 429", rec.Code)
	}
}

func TestLoginRateLimitPrunesExpiredAttempts(t *testing.T) {
	limiter := newLoginRateLimiter()
	now := time.Date(2026, 7, 10, 1, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }

	limiter.reserveFailure("198.51.100.1", "first@example.com")
	limiter.reserveFailure("198.51.100.2", "second@example.com")
	if got := len(limiter.attempts); got != 2 {
		t.Fatalf("attempts before expiry = %d, want 2", got)
	}

	now = now.Add(loginRateLimitWindow + time.Second)
	if !limiter.reserveFailure("198.51.100.3", "third@example.com") {
		t.Fatal("new key should be allowed after pruning")
	}
	if got := len(limiter.attempts); got != 1 {
		t.Fatalf("attempts after pruning = %d, want 1", got)
	}
}

func TestLoginRateLimitReservationIsAtomic(t *testing.T) {
	limiter := newLoginRateLimiter()
	start := make(chan struct{})
	var ready sync.WaitGroup
	var done sync.WaitGroup
	var allowed atomic.Int64
	const attempts = loginRateLimitMaxFailures + 20
	ready.Add(attempts)
	done.Add(attempts)
	for range attempts {
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			if limiter.reserveFailure("198.51.100.10", "owner@example.com") {
				allowed.Add(1)
			}
		}()
	}
	ready.Wait()
	close(start)
	done.Wait()
	if got := allowed.Load(); got != loginRateLimitMaxFailures {
		t.Fatalf("allowed reservations = %d, want %d", got, loginRateLimitMaxFailures)
	}
}

func TestLoginRateLimiterEvictsOldestInsteadOfBlockingNewClients(t *testing.T) {
	limiter := newLoginRateLimiter()
	for i := 0; i < loginRateLimitMaxKeys; i++ {
		if !limiter.reserveFailure(fmt.Sprintf("198.51.100.%d", i), "student@example.com") {
			t.Fatalf("seed reservation %d was blocked", i)
		}
	}
	if !limiter.reserveFailure("203.0.113.250", "unrelated@example.com") {
		t.Fatal("unrelated client was blocked when limiter reached capacity")
	}
	if got := len(limiter.attempts); got != loginRateLimitMaxKeys {
		t.Fatalf("limiter entries = %d, want %d", got, loginRateLimitMaxKeys)
	}
}

func TestLoginRateLimitCannotBeBypassedByRotatingEmail(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)

	for i := 0; i < loginRateLimitMaxFailures; i++ {
		rec := postLogin(t, srv, "198.51.100.10:1234", fmt.Sprintf("missing-%d@example.com", i), "wrong-password")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401", i+1, rec.Code)
		}
	}
	rec := postLogin(t, srv, "198.51.100.10:1234", "another@example.com", "wrong-password")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("rotating-email sixth status = %d, want 429", rec.Code)
	}
	assertAccountCounts(t, st, 0, 0)
}

func TestSignupRateLimitIsIndependentAndBlocksBeforePasswordValidation(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	srv.SetSignupAccessCode(testSignupAccessCode)

	for i := 0; i < loginRateLimitMaxFailures; i++ {
		rec := postSignup(t, srv, validSignupForm("wrong-code"))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("attempt %d status = %d, want 422", i+1, rec.Code)
		}
	}
	rec := postSignup(t, srv, validSignupForm(testSignupAccessCode))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("sixth signup status = %d, want 429 before password hashing", rec.Code)
	}
	assertAccountCounts(t, st, 0, 0)

	login := postLogin(t, srv, "198.51.100.10:1234", "new@example.com", "wrong-password")
	if login.Code != http.StatusUnauthorized {
		t.Fatalf("independent first login status = %d, want 401", login.Code)
	}
}

func TestSignupRateLimitCannotBeBypassedByRotatingEmail(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	srv.SetSignupAccessCode(testSignupAccessCode)

	for i := 0; i < loginRateLimitMaxFailures; i++ {
		form := validSignupForm("wrong-code")
		form.Set("email", fmt.Sprintf("student-%d@example.com", i))
		if rec := postSignup(t, srv, form); rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("attempt %d status = %d, want 422", i+1, rec.Code)
		}
	}
	form := validSignupForm("wrong-code")
	form.Set("email", "student-6@example.com")
	if rec := postSignup(t, srv, form); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("rotating-email sixth status = %d, want 429", rec.Code)
	}
	assertAccountCounts(t, st, 0, 0)
}

func TestSignupRejectsOversizedFieldsBeforeLimiterState(t *testing.T) {
	for _, field := range []string{"email", "password", "password_confirm", "access_code"} {
		t.Run(field, func(t *testing.T) {
			srv, st := newTestServer(t, &fakeScraper{})
			srv.SetProductionMode(true)
			srv.SetSignupAccessCode(testSignupAccessCode)
			form := validSignupForm(testSignupAccessCode)
			form.Set(field, strings.Repeat("a", auth.MaxPasswordBytes+1))

			if rec := postSignup(t, srv, form); rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422", rec.Code)
			}
			if got := len(srv.signupLimiter.attempts); got != 0 {
				t.Fatalf("limiter entries = %d, want 0", got)
			}
			assertAccountCounts(t, st, 0, 0)
		})
	}
}

func TestLoginRateLimiterStateIsBounded(t *testing.T) {
	limiter := newLoginRateLimiter()
	for i := 0; i < 1100; i++ {
		limiter.reserveFailure(fmt.Sprintf("198.51.100.%d", i), "student@example.com")
	}
	if got := len(limiter.attempts); got > 1024 {
		t.Fatalf("limiter entries = %d, want at most 1024", got)
	}
}

func postLogin(t *testing.T, srv *Server, remoteAddr, email, password string) *httptest.ResponseRecorder {
	return postLoginWithForwardedFor(t, srv, remoteAddr, "", email, password)
}

func postLoginWithForwardedFor(t *testing.T, srv *Server, remoteAddr, forwardedFor, email, password string) *httptest.ResponseRecorder {
	return postLoginWithForwardedForAndProxySecret(t, srv, remoteAddr, forwardedFor, "", email, password)
}

func postLoginWithForwardedForAndProxySecret(t *testing.T, srv *Server, remoteAddr, forwardedFor, proxySecret, email, password string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"email": {email}, "password": {password}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.RemoteAddr = remoteAddr
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if forwardedFor != "" {
		req.Header.Set("X-Forwarded-For", forwardedFor)
	}
	if proxySecret != "" {
		req.Header.Set(proxySecretHeaderName, proxySecret)
	}
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf-cookie"})
	req.Header.Set(csrfHeaderName, srv.csrfToken("csrf-cookie", ""))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}
