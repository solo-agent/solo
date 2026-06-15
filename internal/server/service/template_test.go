package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestApplyTemplate_DevTeam_CreatesFiveAgentsAndRelationships(t *testing.T) {
	pool := setupTestPool(t)
	seedDevTeamTemplate(t, pool)
	ownerID := createTemplateTestUser(t, pool)

	svc := NewTemplateService(pool, NewRelationshipsMDGenerator(pool, t.TempDir()))
	result, err := svc.Apply(context.Background(), "dev-team", ownerID)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(result.CreatedAgentIDs) != 5 {
		t.Errorf("expected 5 agents, got %d", len(result.CreatedAgentIDs))
	}
	var relCount int
	pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM agent_relationships`).Scan(&relCount)
	if relCount < 5 {
		t.Errorf("expected >=5 relationships, got %d", relCount)
	}
}

func seedDevTeamTemplate(t *testing.T, pool *pgxpool.Pool) {
	_, err := pool.Exec(context.Background(), `
		INSERT INTO agent_templates (id, name, description, category, members, is_official)
		VALUES ('dev-team', 'Dev Team', 'PM + TPM + FE + BE + QA', 'Developer',
			'[{"role":"leader","name":"PM","instructions":"x","relationship":null},
			{"role":"engineer","name":"TPM","instructions":"y","relationship":null},
			{"role":"engineer","name":"FE","instructions":"z","relationship":null},
			{"role":"engineer","name":"BE","instructions":"w","relationship":null},
			{"role":"engineer","name":"QA","instructions":"v","relationship":null}]'::jsonb,
			true)
		ON CONFLICT (id) DO NOTHING
	`)
	if err != nil {
		t.Fatalf("seed template: %v", err)
	}
}

func createTemplateTestUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	suffix := uuid.New().String()[:8]
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (id, display_name, email, password_hash)
		 VALUES (gen_random_uuid(), $1, $2, 'test-hash')
		 RETURNING id`,
		"tmpl-owner-"+suffix, "tmpl-"+suffix+"@example.com",
	).Scan(&id)
	if err != nil {
		t.Fatalf("create template test user: %v", err)
	}
	return id
}
