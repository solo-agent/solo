package middleware

import (
	"net/http"
)

// SecurityHeaders returns a middleware that sets standard security-related HTTP
// headers on every response: Content-Security-Policy, X-Frame-Options,
// X-Content-Type-Options, and Referrer-Policy.
//
// These headers help mitigate XSS, clickjacking, MIME-type sniffing, and
// referrer leakage.
func SecurityHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

			// Content-Security-Policy: allows Next.js development needs
			// (unsafe-inline for Tailwind styles, unsafe-eval for dev HMR).
			// In production, consider tightening to nonce-based CSP.
			w.Header().Set("Content-Security-Policy",
				"default-src 'self'; "+
					"script-src 'self' 'unsafe-inline' 'unsafe-eval'; "+
					"style-src 'self' 'unsafe-inline'; "+
					"img-src 'self' data: blob:; "+
					"font-src 'self' data:; "+
					"connect-src 'self' ws: wss:; "+
					"frame-ancestors 'none'; "+
					"object-src 'none'",
			)

			next.ServeHTTP(w, r)
		})
	}
}
