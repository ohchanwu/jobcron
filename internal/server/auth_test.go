package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ohchanwu/job-scraper/internal/auth"
)

func TestProductionAuthRedirectsAnonymousPageToLogin(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("Location = %q, want /login", loc)
	}
}

func TestProductionAuthKeepsLoginAndStaticPublic(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)

	for _, path := range []string{"/login", "/static/styles.css", "/favicon.ico"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code == http.StatusSeeOther && rec.Header().Get("Location") == "/login" {
				t.Fatalf("%s redirected to login, want public", path)
			}
		})
	}
}

func TestDemoModeDoesNotRequireLogin(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	srv.SetDemoMode(true)
	srv.SetProductionMode(true)
	ctx := context.Background()
	seedProfile(t, st, ctx)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestLoginFailureUsesGenericError(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	hash, err := auth.HashPassword("correct-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if _, err := st.CreateOwnerUser(context.Background(), "owner@example.com", hash); err != nil {
		t.Fatalf("CreateOwnerUser: %v", err)
	}

	form := url.Values{"email": {"missing@example.com"}, "password": {"wrong-password"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "이메일 또는 비밀번호를 확인해주세요") {
		t.Fatalf("body = %q, want generic login error", body)
	}
	if strings.Contains(body, "missing@example.com") || strings.Contains(body, "존재") {
		t.Fatalf("body reveals account existence: %q", body)
	}
}

func TestLoginSuccessSetsSecureSessionCookie(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	seedProfile(t, st, context.Background())
	hash, err := auth.HashPassword("correct-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if _, err := st.CreateOwnerUser(context.Background(), "owner@example.com", hash); err != nil {
		t.Fatalf("CreateOwnerUser: %v", err)
	}

	form := url.Values{"email": {"owner@example.com"}, "password": {"correct-password"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body=%q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Location") != "/" {
		t.Fatalf("Location = %q, want /", rec.Header().Get("Location"))
	}
	cookie := rec.Result().Cookies()[0]
	if cookie.Name != sessionCookieName {
		t.Fatalf("cookie name = %q, want %q", cookie.Name, sessionCookieName)
	}
	if cookie.Value == "" {
		t.Fatal("session cookie value is empty")
	}
	if !cookie.HttpOnly {
		t.Fatal("session cookie HttpOnly=false")
	}
	if !cookie.Secure {
		t.Fatal("session cookie Secure=false in production")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("SameSite = %v, want Lax", cookie.SameSite)
	}
	if cookie.Path != "/" {
		t.Fatalf("Path = %q, want /", cookie.Path)
	}

	page := httptest.NewRecorder()
	pageReq := httptest.NewRequest(http.MethodGet, "/", nil)
	pageReq.AddCookie(cookie)
	srv.Handler().ServeHTTP(page, pageReq)
	if page.Code != http.StatusOK {
		t.Fatalf("authenticated page status = %d, want 200", page.Code)
	}
}

func TestLogoutClearsSessionCookie(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw-session-token"})
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	if cookies[0].Name != sessionCookieName || cookies[0].MaxAge != -1 {
		t.Fatalf("logout cookie = %+v, want cleared session cookie", cookies[0])
	}
}

func seedProfile(t *testing.T, st interface {
	SaveProfile(context.Context, string) (string, bool, error)
}, ctx context.Context) {
	t.Helper()
	if _, _, err := st.SaveProfile(ctx, `{"career_years":0}`); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
}
