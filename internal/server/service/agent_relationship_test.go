package service

import "testing"

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
	tests := []struct {
		name string
		req  UpdateRelationshipRequest
		ok   bool
	}{
		{"weight", UpdateRelationshipRequest{Weight: &weight}, true},
		{"instruction", UpdateRelationshipRequest{Instruction: &instruction}, true},
		{"empty", UpdateRelationshipRequest{}, false},
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
