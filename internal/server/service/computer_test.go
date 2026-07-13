package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestClaimComputerAllowsMultipleUsers(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()

	ownerID := taskSubmitUser(t, pool)
	memberID := taskSubmitUser(t, pool)
	computerID := uuid.NewString()
	_, err := pool.Exec(ctx,
		`INSERT INTO computers (id, name, owner_id, daemon_id, status)
		 VALUES ($1, 'shared-mac', $2, $3, 'online')`,
		computerID, ownerID, "daemon-"+computerID[:8],
	)
	if err != nil {
		t.Fatalf("create computer: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM computers WHERE id = $1`, computerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1, $2)`, ownerID, memberID)
	})

	if _, err := NewComputerService(pool).ClaimComputer(ctx, computerID, memberID); err != nil {
		t.Fatalf("ClaimComputer second user error = %v, want nil", err)
	}

	var role string
	if err := pool.QueryRow(ctx,
		`SELECT role FROM computer_members WHERE computer_id = $1 AND user_id = $2`,
		computerID, memberID,
	).Scan(&role); err != nil {
		t.Fatalf("member row missing: %v", err)
	}
	if role != "member" {
		t.Fatalf("role = %q, want member", role)
	}
}

func TestListComputersFiltersInactiveAgentIDs(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	ownerID := taskSubmitUser(t, pool)
	activeAgentID := taskSubmitAgent(t, pool, ownerID)
	inactiveAgentID := taskSubmitAgent(t, pool, ownerID)
	computerID := uuid.NewString()
	_, err := pool.Exec(ctx,
		`INSERT INTO computers (id, name, owner_id, daemon_id, status, agent_ids)
		 VALUES ($1, 'filter-mac', $2, $3, 'online', ARRAY[$4::uuid, $5::uuid])`,
		computerID, ownerID, "daemon-"+computerID[:8], activeAgentID, inactiveAgentID,
	)
	if err != nil {
		t.Fatalf("create computer: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM computers WHERE id = $1`, computerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id IN ($1, $2)`, activeAgentID, inactiveAgentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	if _, err := pool.Exec(ctx, `UPDATE agents SET is_active = false WHERE id = $1`, inactiveAgentID); err != nil {
		t.Fatalf("deactivate agent: %v", err)
	}

	computers, err := NewComputerService(pool).ListComputers(ctx, ownerID)
	if err != nil {
		t.Fatalf("ListComputers: %v", err)
	}
	for _, computer := range computers {
		if computer.ID != computerID {
			continue
		}
		if len(computer.AgentIDs) != 1 || computer.AgentIDs[0] != activeAgentID {
			t.Fatalf("AgentIDs = %#v, want only %q", computer.AgentIDs, activeAgentID)
		}
		return
	}
	t.Fatalf("computer %s missing from ListComputers", computerID)
}

func TestUpdateHeartbeatFiltersInactiveAgentIDs(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	ownerID := taskSubmitUser(t, pool)
	activeAgentID := taskSubmitAgent(t, pool, ownerID)
	inactiveAgentID := taskSubmitAgent(t, pool, ownerID)
	computerID := uuid.NewString()
	daemonID := "daemon-" + computerID[:8]
	_, err := pool.Exec(ctx,
		`INSERT INTO computers (id, name, owner_id, daemon_id, status)
		 VALUES ($1, 'heartbeat-mac', $2, $3, 'online')`,
		computerID, ownerID, daemonID,
	)
	if err != nil {
		t.Fatalf("create computer: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM computers WHERE id = $1`, computerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id IN ($1, $2)`, activeAgentID, inactiveAgentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	if _, err := pool.Exec(ctx, `UPDATE agents SET is_active = false WHERE id = $1`, inactiveAgentID); err != nil {
		t.Fatalf("deactivate agent: %v", err)
	}

	err = NewComputerService(pool).UpdateHeartbeat(ctx, daemonID, "http://127.0.0.1:1", []string{activeAgentID, inactiveAgentID}, ComputerSystemInfo{})
	if err != nil {
		t.Fatalf("UpdateHeartbeat: %v", err)
	}

	var agentIDs []string
	if err := pool.QueryRow(ctx, `SELECT agent_ids FROM computers WHERE id = $1`, computerID).Scan(&agentIDs); err != nil {
		t.Fatalf("read agent_ids: %v", err)
	}
	if len(agentIDs) != 1 || agentIDs[0] != activeAgentID {
		t.Fatalf("agent_ids = %#v, want only %q", agentIDs, activeAgentID)
	}
}
