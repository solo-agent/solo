package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/solo-ai/solo/internal/auth"
)

// Auth middleware validates JWT tokens from the Authorization header.
// On success, it sets X-User-ID, X-User-Email, and X-User-Name headers
// for downstream handlers to use.
func Auth(pools ...*pgxpool.Pool) func(http.Handler) http.Handler {
	var pool *pgxpool.Pool
	if len(pools) > 0 {
		pool = pools[0]
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// These headers are internal identity derived from the verified JWT.
			// Never trust values supplied by an external client or reverse proxy.
			r.Header.Del("X-Solo-Run-ID")
			r.Header.Del("X-Solo-Computer-ID")
			r.Header.Del("X-Solo-Actor-Type")
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
			if claims.ActorType == "agent_run" {
				if pool == nil || claims.RunID == "" || claims.ComputerID == "" {
					writeAuthError(w, http.StatusUnauthorized, "unauthorized", "invalid Agent Run credential")
					return
				}
				var active bool
				err = pool.QueryRow(r.Context(), `
					SELECT EXISTS (
					 SELECT 1 FROM agent_runs
					  WHERE id = $1 AND agent_id = $2 AND computer_id = $3 AND finished_at IS NULL
					)`, claims.RunID, claims.Subject, claims.ComputerID).Scan(&active)
				if err != nil || !active {
					writeAuthError(w, http.StatusUnauthorized, "unauthorized", "Agent Run credential is no longer active")
					return
				}
				r.Header.Set("X-Solo-Run-ID", claims.RunID)
				r.Header.Set("X-Solo-Computer-ID", claims.ComputerID)
			}
			r.Header.Set("X-Solo-Actor-Type", claims.ActorType)

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
