package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/solo-ai/solo/internal/server/service"
)

func TestAgentRunHandlerTaskTrajectoryAuthorizationAndValidation(t *testing.T) {
	pool := trajectoryHandlerTestPool(t)
	ctx := context.Background()
	ownerID := uuid.NewString()
	otherID := uuid.NewString()
	channelID := uuid.NewString()
	taskID := uuid.NewString()
	for _, user := range []struct {
		id   string
		name string
	}{{ownerID, "Trajectory Owner"}, {otherID, "Trajectory Other"}} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO users (id, email, display_name, password_hash) VALUES ($1, $2, $3, 'test')`,
			user.id, fmt.Sprintf("%s@example.test", user.id), user.name); err != nil {
			t.Fatalf("create user: %v", err)
		}
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO channels (id, name, created_by) VALUES ($1, $2, $3)`,
		channelID, "trajectory-handler-"+channelID[:8], ownerID); err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO channel_members (channel_id, member_type, member_id, role) VALUES ($1, 'user', $2, 'owner')`,
		channelID, ownerID); err != nil {
		t.Fatalf("create channel membership: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO tasks (id, channel_id, creator_id, title, status, priority, task_number)
		 VALUES ($1, $2, $3, 'trajectory handler task', 'todo', 'normal', 1)`,
		taskID, channelID, ownerID); err != nil {
		t.Fatalf("create task: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM tasks WHERE id = $1`, taskID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channel_members WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = ANY($1::uuid[])`, []string{ownerID, otherID})
	})

	handler := NewAgentRunHandler(pool)
	router := chi.NewRouter()
	router.Get("/api/v1/tasks/{taskID}/trajectory", handler.TaskTrajectory)

	tests := []struct {
		name       string
		path       string
		userID     string
		wantStatus int
	}{
		{name: "authentication required", path: "/api/v1/tasks/" + taskID + "/trajectory", wantStatus: http.StatusUnauthorized},
		{name: "invalid task UUID", path: "/api/v1/tasks/not-a-uuid/trajectory", userID: ownerID, wantStatus: http.StatusBadRequest},
		{name: "task not found", path: "/api/v1/tasks/" + uuid.NewString() + "/trajectory", userID: ownerID, wantStatus: http.StatusNotFound},
		{name: "channel membership required", path: "/api/v1/tasks/" + taskID + "/trajectory", userID: otherID, wantStatus: http.StatusForbidden},
		{name: "authorized snapshot", path: "/api/v1/tasks/" + taskID + "/trajectory", userID: ownerID, wantStatus: http.StatusOK},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, test.path, nil)
			if test.userID != "" {
				req.Header.Set("X-User-ID", test.userID)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.wantStatus, response.Body.String())
			}
			if response.Code == http.StatusOK {
				var snapshot service.TaskTrajectorySnapshot
				if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
					t.Fatalf("decode snapshot: %v", err)
				}
				if snapshot.SchemaVersion != service.TaskTrajectorySchemaVersion || snapshot.RootTaskID != taskID {
					t.Fatalf("snapshot = %+v", snapshot)
				}
			}
		})
	}
}

func trajectoryHandlerTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://solo:solo-dev@localhost:5432/solo?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("skipping DB test: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("skipping DB test: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
