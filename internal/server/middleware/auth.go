package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/solo-ai/solo/internal/auth"
)

// Auth middleware validates JWT tokens from the Authorization header.
// On success, it sets X-User-ID, X-User-Email, and X-User-Name headers
// for downstream handlers to use.
func Auth() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := extractBearerToken(r)
			if tokenString == "" {
				writeAuthError(w, http.StatusUnauthorized, "unauthorized", "missing authorization header")
				return
			}

			claims, err := auth.ValidateToken(tokenString)
			if err != nil {
				slog.Debug("auth: invalid token", "path", r.URL.Path, "error", err)
				writeAuthError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired token")
				return
			}

			r.Header.Set("X-User-ID", claims.Subject)
			r.Header.Set("X-User-Email", claims.Email)
			r.Header.Set("X-User-Name", claims.Name)

			next.ServeHTTP(w, r)
		})
	}
}

// extractBearerToken extracts a Bearer token from the Authorization header.
func extractBearerToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return ""
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == authHeader {
		return ""
	}
	return token
}

// writeAuthError writes a consistent JSON error response.
func writeAuthError(w http.ResponseWriter, status int, errorCode string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":   errorCode,
		"message": message,
	})
}
