package service

import (
	"context"
	"errors"
	"testing"
)

func TestValidateRelationshipCreate(t *testing.T) {
	weight := 11.0
	tests := []struct {
		name string
		req  CreateRelationshipRequest
		ok   bool
	}{
		{"valid assigns_to", CreateRelationshipRequest{FromAgentID: "a", ToAgentID: "b", RelType: RelAssignsTo}, true},
		{"valid collaborates_with", CreateRelationshipRequest{FromAgentID: "a", ToAgentID: "b", RelType: RelCollaboratesWith}, true},
		{"missing fields", CreateRelationshipRequest{}, false},
		{"self", CreateRelationshipRequest{FromAgentID: "a", ToAgentID: "a", RelType: RelAssignsTo}, false},
		{"bad type", CreateRelationshipRequest{FromAgentID: "a", ToAgentID: "b", RelType: "reports_to"}, false},
		{"bad weight", CreateRelationshipRequest{FromAgentID: "a", ToAgentID: "b", RelType: RelAssignsTo, Weight: &weight}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRelationshipCreate(tt.req)
			if tt.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.ok && err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestValidateRelationshipUpdate(t *testing.T) {
	weight := 5.0
	badWeight := -1.0
	instruction := "coordinate reviews"
	assignsTo := RelAssignsTo
	badType := "reports_to"
	tests := []struct {
		name string
		req  UpdateRelationshipRequest
		ok   bool
	}{
		{"weight", UpdateRelationshipRequest{Weight: &weight}, true},
		{"instruction", UpdateRelationshipRequest{Instruction: &instruction}, true},
		{"rel_type", UpdateRelationshipRequest{RelType: &assignsTo}, true},
		{"empty", UpdateRelationshipRequest{}, false},
		{"bad rel_type", UpdateRelationshipRequest{RelType: &badType}, false},
		{"bad weight", UpdateRelationshipRequest{Weight: &badWeight}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRelationshipUpdate(tt.req)
			if tt.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.ok && err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestAgentRelationshipRejectsAssignsToCyclesInPostgres(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	firstID := agentRunAgent(t, pool, ownerID)
	secondID := agentRunAgent(t, pool, ownerID)
	thirdID := agentRunAgent(t, pool, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE owner_id = $1`, ownerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE created_by = $1`, ownerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	svc := NewAgentRelationshipService(pool)
	if _, err := svc.Create(ctx, ownerID, CreateRelationshipRequest{
		FromAgentID: firstID, ToAgentID: secondID, RelType: RelAssignsTo,
	}); err != nil {
		t.Fatalf("create first relationship: %v", err)
	}
	if _, err := svc.Create(ctx, ownerID, CreateRelationshipRequest{
		FromAgentID: secondID, ToAgentID: thirdID, RelType: RelAssignsTo,
	}); err != nil {
		t.Fatalf("create second relationship: %v", err)
	}
	if _, err := svc.Create(ctx, ownerID, CreateRelationshipRequest{
		FromAgentID: thirdID, ToAgentID: firstID, RelType: RelAssignsTo,
	}); !errors.Is(err, ErrRelationshipCycle) {
		t.Fatalf("cycle create error = %v, want %v", err, ErrRelationshipCycle)
	}

	collaboration, err := svc.Create(ctx, ownerID, CreateRelationshipRequest{
		FromAgentID: thirdID, ToAgentID: firstID, RelType: RelCollaboratesWith,
	})
	if err != nil {
		t.Fatalf("create collaboration: %v", err)
	}
	assignsTo := RelAssignsTo
	if _, err := svc.Update(ctx, ownerID, collaboration.ID, UpdateRelationshipRequest{
		RelType: &assignsTo,
	}); !errors.Is(err, ErrRelationshipCycle) {
		t.Fatalf("cycle update error = %v, want %v", err, ErrRelationshipCycle)
	}
}
