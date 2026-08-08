package middleware

import (
	"net/http"
	"strings"

	"github.com/solo-ai/solo/internal/server/service"
)

const computerIDHeader = "X-Solo-Computer-ID"

func ComputerAuth(computers *service.ComputerService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			computerID := strings.TrimSpace(r.Header.Get(computerIDHeader))
			authorization := r.Header.Get("Authorization")
			credential := strings.TrimSpace(strings.TrimPrefix(authorization, "Computer "))
			if computerID == "" || credential == "" || credential == authorization {
				writeAuthError(w, http.StatusUnauthorized, "unauthorized", "missing computer credential")
				return
			}
			if err := computers.AuthenticateCredential(r.Context(), computerID, credential); err != nil {
				writeAuthError(w, http.StatusUnauthorized, "unauthorized", "invalid computer credential")
				return
			}
			r.Header.Set(computerIDHeader, computerID)
			next.ServeHTTP(w, r)
		})
	}
}

func ComputerID(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get(computerIDHeader))
}
