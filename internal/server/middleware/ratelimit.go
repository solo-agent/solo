package middleware

import (
	"log/slog"
	"net/http"
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

// RateLimiter returns a middleware that limits requests using a token bucket
// algorithm. When the limit is exceeded, it responds with 429 Too Many Requests.
// The default rate is 100 requests per second with a burst capacity of 100.
func RateLimiter(rate, capacity float64) func(http.Handler) http.Handler {
	if rate <= 0 {
		rate = 100
	}
	if capacity <= 0 {
		capacity = 100
	}

	bucket := NewTokenBucket(rate, capacity)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !bucket.Allow() {
				slog.Warn("rate limit exceeded",
					"method", r.Method,
					"path", r.URL.Path,
					"remote_addr", r.RemoteAddr,
				)
				w.Header().Set("Retry-After", "1")
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte("Too Many Requests"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
