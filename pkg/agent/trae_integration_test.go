package agent

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestTraeRealPersistentSession exercises the installed, authenticated Trae
// CLI. It is opt-in because it uses a real account and remote model.
func TestTraeRealPersistentSession(t *testing.T) {
	if os.Getenv("SOLO_TRAE_E2E") != "1" {
		t.Skip("set SOLO_TRAE_E2E=1 to run against the real Trae CLI")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	backend := NewTraeBackend(os.Getenv("TRAEX_BIN"), nil)
	workspace := t.TempDir()
	initial, err := backend.Start(ctx, &ExecuteRequest{
		AgentID:  "trae-real-integration",
		Messages: []Message{{Role: RoleUser, Content: "In the current workspace, create TRAE_ACP_PERMISSION_E2E.txt containing exactly TRAE_FILE_OK. Then reply with exactly TRAE_OK."}},
	}, &ExecuteOptions{WorkspaceDir: workspace})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer backend.ForceClose(initial)

	initialResult := awaitTraeResult(t, initial)
	if initialResult.Status != "completed" || !strings.Contains(initialResult.Output, "TRAE_OK") {
		t.Fatalf("initial result = %+v", initialResult)
	}
	fileContent, err := os.ReadFile(workspace + "/TRAE_ACP_PERMISSION_E2E.txt")
	if err != nil {
		t.Fatalf("read file created by Trae: %v", err)
	}
	if strings.TrimSpace(string(fileContent)) != "TRAE_FILE_OK" {
		t.Fatalf("created file content = %q, want TRAE_FILE_OK", fileContent)
	}

	second, err := backend.Send(ctx, initial, []Message{{
		Role:    RoleUser,
		Content: "Read TRAE_ACP_PERMISSION_E2E.txt from the current workspace and reply with exactly TRAE_SECOND_OK: followed by its contents.",
	}})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	secondResult := awaitTraeResult(t, second)
	if secondResult.Status != "completed" ||
		!strings.Contains(secondResult.Output, "TRAE_SECOND_OK:TRAE_FILE_OK") {
		t.Fatalf("second result = %+v", secondResult)
	}
	if second.SessionID == "" || second.SessionID != initial.SessionID {
		t.Fatalf("session IDs initial=%q second=%q", initial.SessionID, second.SessionID)
	}
}

func awaitTraeResult(t *testing.T, session *PersistentSession) *Result {
	t.Helper()
	for range session.Messages {
	}
	select {
	case result := <-session.Result:
		if result == nil {
			t.Fatal("result channel returned nil")
		}
		return result
	case <-time.After(5 * time.Minute):
		t.Fatal("timed out waiting for Trae result")
		return nil
	}
}
