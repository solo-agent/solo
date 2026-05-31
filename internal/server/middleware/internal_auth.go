package middleware

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
)

// InternalAuth middleware validates the internal communication token
// used between the Server and Daemon processes.
//
// The token is passed as: Authorization: Internal-Token <shared-secret>
func InternalAuth() func(http.Handler) http.Handler {
	// Get the expected token from environment
	expectedToken := os.Getenv("INTERNAL_TOKEN_SECRET")
	if expectedToken == "" {
		// Fall back to JWT secret for dev environments
		expectedToken = os.Getenv("JWT_SECRET")
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			token := strings.TrimPrefix(authHeader, "Internal-Token ")
			if token == authHeader {
				http.Error(w, `{"error":"invalid authorization scheme"}`, http.StatusUnauthorized)
				return
			}

			if expectedToken != "" && token != expectedToken {
				slog.Warn("internal auth: invalid token", "path", r.URL.Path, "remote", r.RemoteAddr)
				http.Error(w, `{"error":"invalid internal token"}`, http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
