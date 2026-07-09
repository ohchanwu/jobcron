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

func (l *loginRateLimiter) allow(ip, email string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := loginRateLimitKey(ip, email)
	now := l.now()
	a, ok := l.attempts[key]
	if !ok || !now.Before(a.windowEnd) {
		return true
	}
	return a.count < loginRateLimitMaxFailures
}

func (l *loginRateLimiter) recordFailure(ip, email string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := loginRateLimitKey(ip, email)
	now := l.now()
	a, ok := l.attempts[key]
	if !ok || !now.Before(a.windowEnd) {
		l.attempts[key] = loginAttempts{count: 1, windowEnd: now.Add(loginRateLimitWindow)}
		return
	}
	a.count++
	l.attempts[key] = a
}

func (l *loginRateLimiter) reset(ip, email string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, loginRateLimitKey(ip, email))
}

func loginRateLimitKey(ip, email string) string {
	return strings.ToLower(strings.TrimSpace(ip)) + "\x00" + strings.ToLower(strings.TrimSpace(email))
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}
