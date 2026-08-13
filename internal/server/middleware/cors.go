package middleware

import (
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/go-chi/cors"
)

// CORS returns a CORS middleware handler configured for the Solo application.
// In development, localhost:3000 is allowed. The middleware permits the
// Authorization header and WebSocket upgrade requests.
func CORS() func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins(),
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Workspace-ID"},
		ExposedHeaders:   []string{"Link", "X-Workspace-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	})
}

func configuredAllowedOrigins() []string {
	raw := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		if origin := strings.TrimSpace(part); origin != "" {
			origins = append(origins, origin)
		}
	}
	return origins
}

func allowedOrigins() []string {
	origins := configuredAllowedOrigins()
	if len(origins) > 0 || (os.Getenv("APP_ENV") != "development" && os.Getenv("APP_ENV") != "") {
		return origins
	}
	return []string{
		"http://localhost:3000",
		"http://localhost:3001",
		"http://localhost:8080",
		"http://localhost:8081",
		"http://127.0.0.1:3000",
		"http://127.0.0.1:3001",
		"http://127.0.0.1:8080",
		"http://127.0.0.1:8081",
	}
}

// WebSocketOriginAllowed accepts non-browser clients, same-origin browsers,
// and the same explicit origins used by credentialed CORS.
func WebSocketOriginAllowed(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}
	if strings.EqualFold(parsed.Host, r.Host) {
		return true
	}
	for _, allowed := range allowedOrigins() {
		if origin == allowed {
			return true
		}
	}
	return false
}
