package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/solo-ai/solo/internal/server/service"
	serverworkspace "github.com/solo-ai/solo/internal/server/workspace"
)

func TestChannelHandler_Create_Validation(t *testing.T) {
	// Test without a real DB — we test validation logic that fails before DB calls
	h := &ChannelHandler{}

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "empty request body",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "name too long",
			body:       `{"name":"` + string(make([]byte, 101)) + `"}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/channels", bytes.NewBufferString(tt.body))
			req.Header.Set("X-User-ID", "user-1")
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			h.Create(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}
		})
	}
}

func TestChannelHandler_Get_MissingAuth(t *testing.T) {
	h := &ChannelHandler{}

	req := httptest.NewRequest("GET", "/api/v1/channels/test-id", nil)
	rr := httptest.NewRecorder()

	// Use chi URL params
	h.Get(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", rr.Code)
	}
}

func TestChannelHandler_Create_EmptyName(t *testing.T) {
	h := &ChannelHandler{}

	body := `{"name":""}`
	req := httptest.NewRequest("POST", "/api/v1/channels", bytes.NewBufferString(body))
	req.Header.Set("X-User-ID", "user-1")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty name, got %d", rr.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Message != "channel name is required" {
		t.Errorf("expected 'channel name is required', got %q", resp.Message)
	}
}

func TestChannelHandler_Delete_MissingAuth(t *testing.T) {
	h := &ChannelHandler{}

	req := httptest.NewRequest("DELETE", "/api/v1/channels/test-id", nil)
	rr := httptest.NewRecorder()

	h.Delete(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestChannelHandlerDeleteHomeChannelCleansDeactivatedAgentGlobally(t *testing.T) {
	pool := handlerTestPool(t)
	ctx := context.Background()
	ownerID, workspaceID := uuid.NewString(), uuid.NewString()
	deletedChannelID, otherChannelID := uuid.NewString(), uuid.NewString()
	homeAgentID, sharedAgentID := uuid.NewString(), uuid.NewString()

	if _, err := pool.Exec(ctx, `INSERT INTO users (id,email,display_name,password_hash) VALUES ($1,$2,'Channel Owner','test')`, ownerID, fmt.Sprintf("channel-delete-%s@example.test", ownerID)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO workspaces (id,name,created_by) VALUES ($1,$2,$3)`, workspaceID, "Channel delete "+workspaceID[:8], ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO workspace_members (workspace_id,user_id,role) VALUES ($1,$2,'owner')`, workspaceID, ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO channels (id,name,type,created_by,workspace_id)
		VALUES ($1,$3,'channel',$5,$6),($2,$4,'channel',$5,$6)`,
		deletedChannelID, otherChannelID, "deleted-"+deletedChannelID[:8], "other-"+otherChannelID[:8], ownerID, workspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO agents (id,name,owner_id,model_name,home_channel_id)
		VALUES ($1,$3,$5,'test-model',$6),($2,$4,$5,'test-model',$7)`,
		homeAgentID, sharedAgentID, "home-"+homeAgentID[:8], "shared-"+sharedAgentID[:8], ownerID, deletedChannelID, otherChannelID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO channel_members (channel_id,member_type,member_id,role)
		VALUES ($1,'user',$3,'owner'),($2,'user',$3,'owner'),
		       ($1,'agent',$4,'member'),($2,'agent',$4,'member'),
		       ($1,'agent',$5,'member'),($2,'agent',$5,'member')`,
		deletedChannelID, otherChannelID, ownerID, homeAgentID, sharedAgentID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id IN ($1,$2)`, homeAgentID, sharedAgentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM workspaces WHERE id=$1`, workspaceID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, ownerID)
	})

	homeSessionID, sharedDeletedSessionID, sharedOtherSessionID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO agent_sessions (id,agent_id,provider,external_session_id,status)
		VALUES ($1,$4,'codex','home-other','active'),
		       ($2,$5,'codex','shared-deleted','active'),
		       ($3,$5,'codex','shared-other','active')`,
		homeSessionID, sharedDeletedSessionID, sharedOtherSessionID, homeAgentID, sharedAgentID); err != nil {
		t.Fatal(err)
	}
	homeRunID, sharedDeletedRunID, sharedOtherRunID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO agent_runs (id,agent_id,session_id,trigger_type,channel_id,status)
		VALUES ($1,$4,$7,'message',$9,'running'),
		       ($2,$5,$8,'message',$10,'running'),
		       ($3,$5,$6,'message',$9,'running')`,
		homeRunID, sharedDeletedRunID, sharedOtherRunID, homeAgentID, sharedAgentID, sharedOtherSessionID,
		homeSessionID, sharedDeletedSessionID, otherChannelID, deletedChannelID); err != nil {
		t.Fatal(err)
	}
	homeTaskID, sharedTaskID := uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO tasks (id,task_number,channel_id,creator_id,title,status,claimer_id)
		VALUES ($1,101,$3,$4,'home work','in_progress',$5),
		       ($2,102,$3,$4,'shared work','in_progress',$6)`,
		homeTaskID, sharedTaskID, otherChannelID, ownerID, homeAgentID, sharedAgentID); err != nil {
		t.Fatal(err)
	}

	cleanupPaths := make(chan string, 2)
	daemonServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanupPaths <- r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer daemonServer.Close()
	parsed, _ := url.Parse(daemonServer.URL)
	host, portText, _ := net.SplitHostPort(parsed.Host)
	port, _ := strconv.Atoi(portText)
	dm := service.NewDaemonManager(pool, nil)
	dm.Register(&service.DaemonInfo{ID: "channel-delete-daemon", Host: host, Port: port, MaxConcurrent: 10})

	h := NewChannelHandler(pool, dm, nil)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/channels/"+deletedChannelID, nil)
	req.Header.Set("X-User-ID", ownerID)
	route := chi.NewRouteContext()
	route.URLParams.Add("channelID", deletedChannelID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
	req = req.WithContext(serverworkspace.WithScope(req.Context(), serverworkspace.Scope{ID: workspaceID, Role: "owner"}))
	recorder := httptest.NewRecorder()
	h.Delete(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	var active bool
	if err := pool.QueryRow(ctx, `SELECT is_active FROM agents WHERE id=$1`, homeAgentID).Scan(&active); err != nil || active {
		t.Fatalf("home Agent active = %t, err = %v", active, err)
	}
	assertStatus := func(table, id, want string) {
		t.Helper()
		var got string
		if err := pool.QueryRow(ctx, `SELECT status FROM `+table+` WHERE id=$1`, id).Scan(&got); err != nil || got != want {
			t.Fatalf("%s %s status = %q, err = %v, want %q", table, id, got, err, want)
		}
	}
	assertStatus("agent_runs", homeRunID, "cancelled")
	assertStatus("agent_runs", sharedDeletedRunID, "cancelled")
	assertStatus("agent_runs", sharedOtherRunID, "running")
	assertStatus("agent_sessions", homeSessionID, "closed")
	assertStatus("agent_sessions", sharedDeletedSessionID, "closed")
	assertStatus("agent_sessions", sharedOtherSessionID, "active")
	var homeTaskStatus, homeTaskClaimer, sharedTaskStatus, sharedTaskClaimer string
	if err := pool.QueryRow(ctx, `SELECT status,COALESCE(claimer_id::text,'') FROM tasks WHERE id=$1`, homeTaskID).Scan(&homeTaskStatus, &homeTaskClaimer); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status,COALESCE(claimer_id::text,'') FROM tasks WHERE id=$1`, sharedTaskID).Scan(&sharedTaskStatus, &sharedTaskClaimer); err != nil {
		t.Fatal(err)
	}
	if homeTaskStatus != "todo" || homeTaskClaimer != "" || sharedTaskStatus != "in_progress" || sharedTaskClaimer != sharedAgentID {
		t.Fatalf("tasks = home %s/%s shared %s/%s", homeTaskStatus, homeTaskClaimer, sharedTaskStatus, sharedTaskClaimer)
	}

	wantPaths := map[string]bool{
		"/internal/daemon/agents/" + homeAgentID + "/cleanup":                                     false,
		"/internal/daemon/agents/" + sharedAgentID + "/channels/" + deletedChannelID + "/cleanup": false,
	}
	deadline := time.After(5 * time.Second)
	for remaining := len(wantPaths); remaining > 0; {
		select {
		case path := <-cleanupPaths:
			seen, exists := wantPaths[path]
			if !exists {
				t.Fatalf("unexpected daemon cleanup path %q", path)
			}
			if !seen {
				wantPaths[path] = true
				remaining--
			}
		case <-deadline:
			t.Fatalf("daemon cleanup paths = %#v", wantPaths)
		}
	}
}

func TestChannelHandler_ResponseFormat(t *testing.T) {
	// Verify the channel response structure serializes correctly
	resp := ChannelResponse{
		ID:          "ch-1",
		Name:        "general",
		Description: "General discussion",
		Type:        "channel",
		CreatedBy:   "user-1",
		IsArchived:  false,
		CreatedAt:   "2025-01-01T00:00:00Z",
		UpdatedAt:   "2025-01-01T00:00:00Z",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded ChannelResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ID != "ch-1" {
		t.Errorf("expected id ch-1, got %s", decoded.ID)
	}
	if decoded.Name != "general" {
		t.Errorf("expected name general, got %s", decoded.Name)
	}
	if decoded.Type != "channel" {
		t.Errorf("expected type channel, got %s", decoded.Type)
	}
}
