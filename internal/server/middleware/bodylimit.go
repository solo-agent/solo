package middleware

import (
	"net/http"
)

// MaxBodySize returns a middleware that limits the size of incoming request
// bodies. If the Content-Length header exceeds the limit, the request is
// rejected immediately with 413 Request Entity Too Large. Otherwise,
// r.Body is wrapped with http.MaxBytesReader so the limit is enforced
// during reading.
//
// Use this on specific route groups that accept payloads (message creation,
// channel update, etc.) rather than globally.
func MaxBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxBytes {
				http.Error(w, "request entity too large", http.StatusRequestEntityTooLarge)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
