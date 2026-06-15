package service

import (
	"context"
	"strings"
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
	foundEscalation := false
	for _, p := range hub.channelPayloads {
		if strings.Contains(string(p), "watchdog_escalation") {
			foundEscalation = true
			break
		}
	}
	if !foundEscalation {
		t.Errorf("expected watchdog_escalation notification, got payloads: %s", stringJoinPayloads(hub.channelPayloads))
	}
}

// stringJoinPayloads is a small debug helper for test failure messages.
func stringJoinPayloads(payloads [][]byte) string {
	out := ""
	for i, p := range payloads {
		if i > 0 {
			out += " | "
		}
		out += string(p)
	}
	return out
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
	// No relationship-aware escalation should fire.
	for _, p := range hub.channelPayloads {
		if strings.Contains(string(p), "watchdog_escalation") {
			t.Errorf("unexpected watchdog_escalation without assigns_to edge: %s", string(p))
		}
	}
}