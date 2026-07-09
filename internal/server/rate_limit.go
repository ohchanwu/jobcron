package server

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	loginRateLimitMaxFailures = 5
	loginRateLimitWindow      = 15 * time.Minute
	proxySecretHeaderName     = "X-Jobcron-Proxy"
)

type loginRateLimiter struct {
	mu       sync.Mutex
	now      func() time.Time
	attempts map[string]loginAttempts
}

type loginAttempts struct {
	count     int
	windowEnd time.Time
}

func newLoginRateLimiter() *loginRateLimiter {
	return &loginRateLimiter{
		now:      time.Now,
		attempts: map[string]loginAttempts{},
	}
}

func (l *loginRateLimiter) reserveFailure(ip, email string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := loginRateLimitKey(ip, email)
	now := l.now()
	l.pruneExpired(now)
	a, ok := l.attempts[key]
	if !ok || !now.Before(a.windowEnd) {
		l.attempts[key] = loginAttempts{count: 1, windowEnd: now.Add(loginRateLimitWindow)}
		return true
	}
	if a.count >= loginRateLimitMaxFailures {
		return false
	}
	a.count++
	l.attempts[key] = a
	return true
}

func (l *loginRateLimiter) reset(ip, email string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pruneExpired(l.now())
	delete(l.attempts, loginRateLimitKey(ip, email))
}

func (l *loginRateLimiter) pruneExpired(now time.Time) {
	for key, attempt := range l.attempts {
		if !now.Before(attempt.windowEnd) {
			delete(l.attempts, key)
		}
	}
}

func loginRateLimitKey(ip, email string) string {
	return strings.ToLower(strings.TrimSpace(ip)) + "\x00" + strings.ToLower(strings.TrimSpace(email))
}

func (s *Server) clientIP(r *http.Request) string {
	host := remoteHost(r.RemoteAddr)
	if s.trustedProxyRequest(r) {
		if forwarded := forwardedClientIP(r); forwarded != "" {
			return forwarded
		}
	}
	return host
}

func remoteHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil && host != "" {
		return host
	}
	return remoteAddr
}

func (s *Server) trustedProxyRequest(r *http.Request) bool {
	if s.proxySecret == "" {
		return false
	}
	return subtleConstantTimeEqual(r.Header.Get(proxySecretHeaderName), s.proxySecret)
}

func subtleConstantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

func forwardedClientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		for _, part := range strings.Split(forwarded, ",") {
			candidate := strings.TrimSpace(part)
			if net.ParseIP(candidate) != nil {
				return candidate
			}
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); net.ParseIP(realIP) != nil {
		return realIP
	}
	return ""
}
