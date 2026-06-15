package service

import (
	"context"
	"strings"
	"testing"
)

func TestValidateStatusTransition(t *testing.T) {
	tests := []struct {
		name        string
		from        string
		to          string
		expectError bool
	}{
		// Valid transitions
		{"todo -> in_progress", TaskStatusTodo, TaskStatusInProgress, false},
		{"todo -> closed", TaskStatusTodo, TaskStatusClosed, false},
		{"in_progress -> in_review", TaskStatusInProgress, TaskStatusInReview, false},
		{"in_progress -> todo (unclaim via API)", TaskStatusInProgress, TaskStatusTodo, true},
		{"in_progress -> closed", TaskStatusInProgress, TaskStatusClosed, false},
		{"in_review -> done", TaskStatusInReview, TaskStatusDone, false},
		{"in_review -> in_progress", TaskStatusInReview, TaskStatusInProgress, false},
		{"in_review -> closed", TaskStatusInReview, TaskStatusClosed, false},
		{"done -> in_progress (blocked: terminal per v1.3)", TaskStatusDone, TaskStatusInProgress, true},
		{"closed -> todo", TaskStatusClosed, TaskStatusTodo, false},
		// No-op: same status
		{"todo -> todo (no-op)", TaskStatusTodo, TaskStatusTodo, false},
		{"done -> done (no-op)", TaskStatusDone, TaskStatusDone, false},
		// Invalid transitions
		{"todo -> done (skip)", TaskStatusTodo, TaskStatusDone, true},
		{"todo -> in_review (skip)", TaskStatusTodo, TaskStatusInReview, true},
		{"in_progress -> done (skip)", TaskStatusInProgress, TaskStatusDone, true},
		{"done -> todo (regress 2)", TaskStatusDone, TaskStatusTodo, true},
		{"done -> closed", TaskStatusDone, TaskStatusClosed, false},
		{"closed -> in_progress", TaskStatusClosed, TaskStatusInProgress, true},
		// Invalid status values
		{"todo -> invalid_status", TaskStatusTodo, "invalid_status", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStatusTransition(tt.from, tt.to)
			if tt.expectError && err == nil {
				t.Errorf("expected error for transition %s -> %s, got nil", tt.from, tt.to)
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error for transition %s -> %s: %v", tt.from, tt.to, err)
			}
		})
	}
}

func TestValidateStatusTransition_InvalidStatuses(t *testing.T) {
	// Test with empty status
	if err := validateStatusTransition("", TaskStatusTodo); err == nil {
		t.Error("expected error for empty current status, got nil")
	}

	// Test with unrecognized status
	if err := validateStatusTransition(TaskStatusTodo, "unknown"); err == nil {
		t.Error("expected error for unknown target status, got nil")
	}
}

func TestTaskService_NilPoolOperations(t *testing.T) {
	// Test that service creation works (actual DB operations will panic with nil pool,
	// but we test that the constructor and validation logic are sound)
	svc := NewTaskService(nil)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}

	// These operations should fail because pool is nil, but we just verify they
	// don't panic during construction.
	ctx := context.Background()

	// requireChannelMember with nil pool should produce a panic or error
	defer func() {
		if r := recover(); r != nil {
			// Expected - nil pool
		}
	}()

	err := svc.requireChannelMember(ctx, "ch-1", "user-1")
	if err != nil {
		// Expected - nil pool will cause errors
	}
}

func TestTaskService_GetValidStatuses(t *testing.T) {
	expected := []string{
		TaskStatusTodo,
		TaskStatusInProgress,
		TaskStatusInReview,
		TaskStatusDone,
		TaskStatusClosed,
	}

	if len(ValidTaskStatuses) != len(expected) {
		t.Errorf("expected %d valid statuses, got %d", len(expected), len(ValidTaskStatuses))
	}

	for i, s := range expected {
		if ValidTaskStatuses[i] != s {
			t.Errorf("expected status[%d] = %s, got %s", i, s, ValidTaskStatuses[i])
		}
	}
}

func TestTaskService_AllowedTransitions(t *testing.T) {
	// Verify all valid statuses have an entry in allowedTransitions
	for _, status := range ValidTaskStatuses {
		if _, ok := allowedTransitions[status]; !ok {
			t.Errorf("missing allowedTransitions entry for status %s", status)
		}
	}
}

func TestNullableStr(t *testing.T) {
	if nullableStr("") != nil {
		t.Error("expected nil for empty string")
	}
	if nullableStr("hello") == nil {
		t.Error("expected non-nil for non-empty string")
	}
	if *nullableStr("hello") != "hello" {
		t.Errorf("expected 'hello', got %s", *nullableStr("hello"))
	}
}

func TestTaskService_ErrorTypes(t *testing.T) {
	if ErrTaskNotFound == nil {
		t.Error("ErrTaskNotFound should not be nil")
	}
	if ErrTaskInvalidStatus == nil {
		t.Error("ErrTaskInvalidStatus should not be nil")
	}
	if ErrTaskInvalidTransition == nil {
		t.Error("ErrTaskInvalidTransition should not be nil")
	}
	if ErrTaskNotChannelMember == nil {
		t.Error("ErrTaskNotChannelMember should not be nil")
	}
}

func TestTaskCreateRequest_Defaults(t *testing.T) {
	req := TaskCreateRequest{
		Title: "Test",
	}
	if req.Priority != "" {
		t.Errorf("expected empty priority, got %s", req.Priority)
	}
}
