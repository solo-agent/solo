package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRelationshipsMD_GeneratesForAgent(t *testing.T) {
	pool := setupTestPool(t)
	aID := createTestAgent(t, pool, "Alice")
	bID := createTestAgent(t, pool, "Bob")
	createTestRelationship(t, pool, aID, bID, "assigns_to", nil, 1.0)

	tmp := t.TempDir()
	svc := NewRelationshipsMDGenerator(pool, tmp)
	err := svc.GenerateForAgent(context.Background(), aID)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	path := filepath.Join(tmp, aID, "workspace", "RELATIONSHIPS.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read md: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "# Agent @Alice") {
		t.Errorf("expected agent heading, got: %s", body)
	}
	if !strings.Contains(body, "@Bob") {
		t.Errorf("expected Bob in body, got: %s", body)
	}
	if !strings.Contains(body, "assigns_to") && !strings.Contains(body, "我委托给") {
		t.Errorf("expected assigns_to section, got: %s", body)
	}
}
