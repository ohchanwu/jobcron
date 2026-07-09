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
	l.pruneExpired(now)
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
	l.pruneExpired(now)
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

func clientIP(r *http.Request) string {
	host := remoteHost(r.RemoteAddr)
	if trustedProxyIP(host) {
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

func trustedProxyIP(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
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
