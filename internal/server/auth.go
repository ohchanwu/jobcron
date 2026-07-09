package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ohchanwu/job-scraper/internal/auth"
)

const (
	sessionCookieName = "job_scraper_session"
	sessionTTL        = 30 * 24 * time.Hour
	loginErrorCopy    = "이메일 또는 비밀번호를 확인해주세요."
)

type loginPage struct {
	Error     string
	Email     string
	CSRFToken string
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if publicAuthPath(r) {
			next.ServeHTTP(w, r)
			return
		}
		if _, ok := s.userFromRequest(r.Context(), r); ok {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeAuthUnauthorized(w)
			return
		}
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})
}

func publicAuthPath(r *http.Request) bool {
	if r.URL.Path == "/login" && (r.Method == http.MethodGet || r.Method == http.MethodPost) {
		return true
	}
	if r.URL.Path == "/logout" && r.Method == http.MethodPost {
		return true
	}
	if r.URL.Path == "/favicon.ico" {
		return true
	}
	return strings.HasPrefix(r.URL.Path, "/static/")
}

func (s *Server) userFromRequest(ctx context.Context, r *http.Request) (userID int64, ok bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return 0, false
	}
	user, ok, err := s.store.UserBySessionToken(ctx, cookie.Value)
	if err != nil || !ok {
		return 0, false
	}
	return user.ID, true
}

func (s *Server) stateUserID(ctx context.Context, r *http.Request) (int64, error) {
	if !s.productionMode || s.demoMode {
		return 0, nil
	}
	userID, ok := s.userFromRequest(ctx, r)
	if !ok {
		return 0, http.ErrNoCookie
	}
	return userID, nil
}

func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	s.renderWithRequest(w, r, "login.html", loginPage{})
}

func (s *Server) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	ip := s.clientIP(r)
	if !s.loginLimiter.reserveFailure(ip, email) {
		http.Error(w, "too many login attempts", http.StatusTooManyRequests)
		return
	}
	user, ok, err := s.store.UserByEmail(r.Context(), email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		s.renderLoginFailure(w, r)
		return
	}
	matches, err := auth.VerifyPassword(user.PasswordHash, password)
	if err != nil || !matches {
		s.renderLoginFailure(w, r)
		return
	}
	s.loginLimiter.reset(ip, email)
	token, err := auth.GenerateSessionToken()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.store.CreateSession(r.Context(), user.ID, auth.HashSessionToken(token), time.Now().Add(sessionTTL)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, s.sessionCookie(token, time.Now().Add(sessionTTL)))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) renderLoginFailure(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusUnauthorized)
	s.renderWithRequest(w, r, "login.html", loginPage{Error: loginErrorCopy})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		if err := s.store.RevokeSessionToken(r.Context(), cookie.Value); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	cookie := s.sessionCookie("", time.Unix(0, 0))
	cookie.MaxAge = -1
	http.SetCookie(w, cookie)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func writeAuthUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "authentication required"})
}

func (s *Server) sessionCookie(token string, expiresAt time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt.UTC(),
		HttpOnly: true,
		Secure:   s.productionMode,
		SameSite: http.SameSiteLaxMode,
	}
}
