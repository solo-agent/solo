package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSubmitTaskParentWithOpenSubtaskReturnsGuard(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	creatorID := taskSubmitUser(t, pool)
	agentID := taskSubmitAgent(t, pool, creatorID)
	channelID := taskSubmitChannel(t, pool, creatorID)
	taskSubmitMember(t, pool, channelID, "user", creatorID)
	taskSubmitMember(t, pool, channelID, "agent", agentID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM tasks WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channel_members WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, creatorID)
	})

	parentID := taskSubmitTask(t, pool, channelID, creatorID, agentID, TaskStatusInProgress, nil)
	_ = taskSubmitTask(t, pool, channelID, creatorID, "", TaskStatusTodo, &parentID)

	_, err := NewTaskService(pool).SubmitTask(ctx, channelID, parentID, agentID)
	if !errors.Is(err, ErrTaskHasOpenSubtasks) {
		t.Fatalf("SubmitTask error = %v, want %v", err, ErrTaskHasOpenSubtasks)
	}
}

func TestSubmitTaskTriggersArtifactForParentTaskInReview(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	creatorID := taskSubmitUser(t, pool)
	agentID := taskSubmitAgent(t, pool, creatorID)
	channelID := taskSubmitChannel(t, pool, creatorID)
	taskSubmitMember(t, pool, channelID, "user", creatorID)
	taskSubmitMember(t, pool, channelID, "agent", agentID)
	taskID := taskSubmitTask(t, pool, channelID, creatorID, agentID, TaskStatusInProgress, nil)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM tasks WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channel_members WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, creatorID)
	})

	var gotTaskID, gotUserID string
	svc := NewTaskService(pool)
	svc.SetArtifactGenerator(func(_ context.Context, taskID, userID string) (string, error) {
		gotTaskID = taskID
		gotUserID = userID
		return "pending", nil
	})

	updated, err := svc.SubmitTask(ctx, channelID, taskID, agentID)
	if err != nil {
		t.Fatalf("SubmitTask error = %v", err)
	}
	if updated.Status != TaskStatusInReview {
		t.Fatalf("SubmitTask status = %s, want %s", updated.Status, TaskStatusInReview)
	}
	if updated.ArtifactStatus != "pending" {
		t.Fatalf("SubmitTask artifact status = %q, want pending", updated.ArtifactStatus)
	}
	if gotTaskID != taskID || gotUserID != agentID {
		t.Fatalf("artifact generator called with task=%q user=%q, want task=%q user=%q", gotTaskID, gotUserID, taskID, agentID)
	}
}

func TestSubmitTaskDoesNotTriggerArtifactForChildTask(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	creatorID := taskSubmitUser(t, pool)
	agentID := taskSubmitAgent(t, pool, creatorID)
	channelID := taskSubmitChannel(t, pool, creatorID)
	taskSubmitMember(t, pool, channelID, "user", creatorID)
	taskSubmitMember(t, pool, channelID, "agent", agentID)
	parentID := taskSubmitTask(t, pool, channelID, creatorID, "", TaskStatusInProgress, nil)
	childID := taskSubmitTask(t, pool, channelID, creatorID, agentID, TaskStatusInProgress, &parentID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM tasks WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channel_members WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, creatorID)
	})

	calls := 0
	svc := NewTaskService(pool)
	svc.SetArtifactGenerator(func(context.Context, string, string) (string, error) {
		calls++
		return "pending", nil
	})

	if _, err := svc.SubmitTask(ctx, channelID, childID, agentID); err != nil {
		t.Fatalf("SubmitTask child error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("artifact generator calls = %d, want 0", calls)
	}
}

func taskSubmitTestPool(t *testing.T) *pgxpool.Pool {
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

func taskSubmitUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id := uuid.NewString()
	email := fmt.Sprintf("submit-%s@example.test", id)
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, display_name, password_hash) VALUES ($1, $2, $3, 'test')`,
		id, email, "Submit Tester",
	)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return id
}

func taskSubmitAgent(t *testing.T, pool *pgxpool.Pool, ownerID string) string {
	t.Helper()
	id := uuid.NewString()
	channelID := taskSubmitChannel(t, pool, ownerID)
	_, err := pool.Exec(context.Background(),
		`INSERT INTO agents (id, name, owner_id, model_name, home_channel_id)
		 VALUES ($1, $2, $3, 'test-model', $4)`,
		id, "submit-agent-"+id[:8], ownerID, channelID,
	)
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return id
}

func taskSubmitChannel(t *testing.T, pool *pgxpool.Pool, creatorID string) string {
	t.Helper()
	var existingID string
	if err := pool.QueryRow(context.Background(), `
		SELECT COALESCE((
			SELECT id::text
			  FROM channels
			 WHERE created_by = $1
			   AND type = 'channel'
			   AND name LIKE 'submit-test-%'
			 ORDER BY created_at ASC
			 LIMIT 1
		), '')
	`, creatorID).Scan(&existingID); err != nil {
		t.Fatalf("find channel: %v", err)
	}
	if existingID != "" {
		return existingID
	}

	id := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO channels (id, name, created_by) VALUES ($1, $2, $3)`,
		id, "submit-test-"+id[:8], creatorID,
	)
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	return id
}

func taskSubmitMember(t *testing.T, pool *pgxpool.Pool, channelID, memberType, memberID string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO channel_members (channel_id, member_type, member_id) VALUES ($1, $2, $3)`,
		channelID, memberType, memberID,
	)
	if err != nil {
		t.Fatalf("add member: %v", err)
	}
}

func taskSubmitTask(t *testing.T, pool *pgxpool.Pool, channelID, creatorID, claimerID, status string, parentID *string) string {
	t.Helper()
	id := uuid.NewString()
	var claimer any
	if claimerID != "" {
		claimer = claimerID
	}
	var parent any
	if parentID != nil {
		parent = *parentID
	}
	_, err := pool.Exec(context.Background(),
		`INSERT INTO tasks (id, channel_id, creator_id, claimer_id, title, status, priority, task_number, parent_task_id)
		 VALUES ($1, $2, $3, $4, 'submit-test', $5, 'normal',
		   (SELECT COALESCE(MAX(task_number), 0) + 1 FROM tasks WHERE channel_id = $2), $6)`,
		id, channelID, creatorID, claimer, status, parent,
	)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	return id
}
