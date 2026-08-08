package middleware

import (
	"net/http/httptest"
	"testing"
)

func TestWebSocketOriginAllowedUsesDevelopmentDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("CORS_ALLOWED_ORIGINS", "")
	req := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/v1/ws", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	if !WebSocketOriginAllowed(req) {
		t.Fatal("development frontend origin was rejected")
	}
}

func TestWebSocketOriginAllowedRejectsUnknownProductionOrigin(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://solo.example.com")
	req := httptest.NewRequest("GET", "https://api.solo.example.com/api/v1/ws", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	if WebSocketOriginAllowed(req) {
		t.Fatal("unknown production origin was accepted")
	}
}
