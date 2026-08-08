package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDaemonControlRejectsProtocolMismatchBeforeUpgrade(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/internal/v1/daemon/connect", nil)
	req.Header.Set("X-Solo-Protocol-Version", "99")
	recorder := httptest.NewRecorder()
	(&DaemonControlHandler{}).Connect(recorder, req)
	if recorder.Code != http.StatusUpgradeRequired {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUpgradeRequired)
	}
	if got := recorder.Header().Get("X-Solo-Supported-Protocol-Versions"); got != "1" {
		t.Fatalf("supported protocols = %q, want 1", got)
	}
}
