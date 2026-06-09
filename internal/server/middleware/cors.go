package middleware

import (
	"net/http"
	"os"

	"github.com/go-chi/cors"
)

// CORS returns a CORS middleware handler configured for the Solo application.
// In development, localhost:3000 is allowed. The middleware permits the
// Authorization header and WebSocket upgrade requests.
func CORS() func(http.Handler) http.Handler {
	allowedOrigins := []string{"https://*", "http://*"}

	if os.Getenv("APP_ENV") == "development" || os.Getenv("APP_ENV") == "" {
		allowedOrigins = []string{
			"http://localhost:3000",
			"http://localhost:3001",
			"http://localhost:8080",
			"http://localhost:8081",
		}
	}

	return cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	})
}
