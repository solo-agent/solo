package middleware

import (
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// TokenBucket implements a simple token bucket rate limiter.
type TokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	capacity   float64
	rate       float64
	lastRefill time.Time
}

// NewTokenBucket creates a token bucket that refills at rate tokens per second
// up to the given capacity.
func NewTokenBucket(rate, capacity float64) *TokenBucket {
	return &TokenBucket{
		tokens:     capacity,
		capacity:   capacity,
		rate:       rate,
		lastRefill: time.Now(),
	}
}

// Allow reports whether one token can be consumed. It refills the bucket based
// on elapsed time before checking.
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.rate
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}
	tb.lastRefill = now

	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}
	return false
}

type clientBucket struct {
	bucket   *TokenBucket
	lastSeen time.Time
}

// RateLimiter returns a middleware that limits each client IP using a token
// bucket. Idle client buckets are discarded periodically.
// The default rate is 100 requests per second with a burst capacity of 100.
func RateLimiter(rate, capacity float64) func(http.Handler) http.Handler {
	if rate <= 0 {
		rate = 100
	}
	if capacity <= 0 {
		capacity = 100
	}

	var mu sync.Mutex
	buckets := make(map[string]*clientBucket)
	lastCleanup := time.Now()
	retryAfter := strconv.Itoa(max(1, int(math.Ceil(1/rate))))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			now := time.Now()
			clientIP := r.RemoteAddr
			if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
				clientIP = host
			}
			mu.Lock()
			if now.Sub(lastCleanup) >= 5*time.Minute {
				for key, entry := range buckets {
					if now.Sub(entry.lastSeen) >= 10*time.Minute {
						delete(buckets, key)
					}
				}
				lastCleanup = now
			}
			entry := buckets[clientIP]
			if entry == nil {
				entry = &clientBucket{bucket: NewTokenBucket(rate, capacity)}
				buckets[clientIP] = entry
			}
			entry.lastSeen = now
			mu.Unlock()

			if !entry.bucket.Allow() {
				slog.Warn("rate limit exceeded",
					"method", r.Method,
					"path", r.URL.Path,
					"client_ip", clientIP,
				)
				w.Header().Set("Retry-After", retryAfter)
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte("Too Many Requests"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
