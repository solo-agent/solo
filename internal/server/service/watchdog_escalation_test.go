package service

import (
	"context"
	"testing"
)

// TestWatchdogEscalation_FindsCreatorViaAssignsTo verifies that when a sub-task
// is overdue (deadline passed) and the claimer has an assigns_to edge back to
// the task's creator, the WatchdogService broadcasts a watchdog_escalation
// message to the channel.
func TestWatchdogEscalation_FindsCreatorViaAssignsTo(t *testing.T) {
	pool := setupTestPool(t)
	hub := &fakeHub{}
	aID := createTestAgent(t, pool, "Alice")
	bID := createTestAgent(t, pool, "Bob")
	channelID, _ := createTestChannel(t, pool)
	addChannelMember(t, pool, channelID, aID, "agent")
	addChannelMember(t, pool, channelID, bID, "agent")
	// Reverse edge: B → A (B claims A's task via assigns_to).
	createTestRelationship(t, pool, bID, aID, "assigns_to", nil, 1.0)
	parentID := createTestTask(t, pool, channelID, aID, "T-parent", nil)
	taskID := createTestTask(t, pool, channelID, aID, "T-sub", &parentID)

	// Insert overdue watchdog (deadline 30 min in the past, claimed 1h ago).
	_, err := pool.Exec(context.Background(), `
		INSERT INTO task_watchdog (task_id, claimer_id, claimed_at, deadline, last_activity, timeout_action, created_at)
		VALUES ($1, $2, now() - interval '1 hour', now() - interval '30 minutes', now() - interval '30 minutes', 'escalate', now() - interval '1 hour')
	`, taskID, bID)
	if err != nil {
		t.Fatalf("insert watchdog: %v", err)
	}

	svc := NewWatchdogService(pool, hub)
	svc.SetNotifier(NewSubtaskNotifier(pool, hub)) // wire escalation notifier
	if err := svc.ScanTimeouts(context.Background()); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(hub.sentToChannel) == 0 {
		t.Error("expected escalation notification, got none")
	}
}

// TestWatchdogEscalation_NoAssignsToEdge verifies that when no assigns_to
// relationship exists from the claimer back to the creator, no escalation
// notification is broadcast.
func TestWatchdogEscalation_NoAssignsToEdge(t *testing.T) {
	pool := setupTestPool(t)
	hub := &fakeHub{}
	aID := createTestAgent(t, pool, "Alice")
	bID := createTestAgent(t, pool, "Bob")
	channelID, _ := createTestChannel(t, pool)
	addChannelMember(t, pool, channelID, aID, "agent")
	addChannelMember(t, pool, channelID, bID, "agent")
	// No assigns_to edge.
	parentID := createTestTask(t, pool, channelID, aID, "T-parent", nil)
	taskID := createTestTask(t, pool, channelID, aID, "T-sub", &parentID)

	_, err := pool.Exec(context.Background(), `
		INSERT INTO task_watchdog (task_id, claimer_id, claimed_at, deadline, last_activity, timeout_action, created_at)
		VALUES ($1, $2, now() - interval '1 hour', now() - interval '30 minutes', now() - interval '30 minutes', 'escalate', now() - interval '1 hour')
	`, taskID, bID)
	if err != nil {
		t.Fatalf("insert watchdog: %v", err)
	}

	svc := NewWatchdogService(pool, hub)
	svc.SetNotifier(NewSubtaskNotifier(pool, hub))
	if err := svc.ScanTimeouts(context.Background()); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(hub.sentToChannel) != 0 {
		t.Errorf("expected no escalation without assigns_to edge, got %d notifications", len(hub.sentToChannel))
	}
}