package service

import (
	"encoding/json"
	"testing"
)

func TestTemplateMemberJSON(t *testing.T) {
	raw := []byte(`[{"role":"leader","name":"Lead","relationship":null},{"role":"engineer","name":"BE","relationship":"Delegate backend work"}]`)
	var members []templateMember
	if err := json.Unmarshal(raw, &members); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if members[0].Relationship != nil {
		t.Fatal("leader relationship should be nil")
	}
	if members[1].Relationship == nil || *members[1].Relationship == "" {
		t.Fatal("member relationship should be set")
	}
}
