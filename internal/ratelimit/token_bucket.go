package ratelimit

import (
	"net"
	"sync"
	"time"
)

type bucket struct {
	tokens    float64
	lastRefill time.Time
}

// Limiter is an IP-based token bucket limiting download requests.
// Each IP gets `max` tokens per window; they refill continuously
// at rate max/window. Failed downloads return their token.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	max     float64
	window  time.Duration
}

func NewLimiter(max int, window time.Duration) *Limiter {
	l := &Limiter{
		buckets: make(map[string]*bucket),
		max:     float64(max),
		window:  window,
	}
	go l.cleanup()
	return l
}

// clientIP normalizes to the IP itself (no port, no proxy chains).
func clientIP(s string) string {
	if ip := net.ParseIP(s); ip != nil {
		return ip.String()
	}
	// Possibly host:port form (behind proxy) — take host portion.
	if host, _, err := net.SplitHostPort(s); err == nil {
		return host
	}
	return s
}

func (l *Limiter) refillRate() float64 {
	return l.max / l.window.Seconds()
}

// Allow reports whether the given IP has a token; if so it consumes one.
func (l *Limiter) Allow(ip string) bool {
	return l.take(ip, time.Now())
}

func (l *Limiter) take(ip string, now time.Time) bool {
	ip = clientIP(ip)
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[ip]
	if !ok {
		b = &bucket{tokens: l.max - 1, lastRefill: now}
		l.buckets[ip] = b
		return true
	}

	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * l.refillRate()
	if b.tokens > l.max {
		b.tokens = l.max
	}
	b.lastRefill = now

	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// Refund returns a token to the IP's bucket (used when a download
// fails before it produced anything).
func (l *Limiter) Refund(ip string) {
	ip = clientIP(ip)
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[ip]
	if !ok {
		return
	}
	b.tokens++
	if b.tokens > l.max {
		b.tokens = l.max
	}
}

func (l *Limiter) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	for range ticker.C {
		cutoff := time.Now().Add(-24 * time.Hour)
		l.mu.Lock()
		for ip, b := range l.buckets {
			if b.lastRefill.Before(cutoff) {
				delete(l.buckets, ip)
			}
		}
		l.mu.Unlock()
	}
}
