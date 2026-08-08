package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimiterKeepsIndependentClientBuckets(t *testing.T) {
	handler := RateLimiter(1, 1)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	request := func(remoteAddr string) int {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = remoteAddr
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}

	if got := request("192.0.2.1:1000"); got != http.StatusNoContent {
		t.Fatalf("first client status = %d", got)
	}
	if got := request("192.0.2.1:2000"); got != http.StatusTooManyRequests {
		t.Fatalf("same client status = %d", got)
	}
	if got := request("192.0.2.2:1000"); got != http.StatusNoContent {
		t.Fatalf("independent client status = %d", got)
	}
}
