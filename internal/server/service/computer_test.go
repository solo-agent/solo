package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestComputerEnrollmentIsOneTimeAndRevocable(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	ownerID := taskSubmitUser(t, pool)
	svc := NewComputerService(pool)

	enrollment, err := svc.CreateComputerWithEnrollment(ctx, ownerID, "remote-mac")
	if err != nil {
		t.Fatalf("CreateComputerWithEnrollment: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM computers WHERE id = $1`, enrollment.Computer.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	credential, err := svc.ExchangeEnrollment(ctx, enrollment.Computer.ID, enrollment.Token)
	if err != nil {
		t.Fatalf("ExchangeEnrollment: %v", err)
	}
	if credential.Credential == "" {
		t.Fatal("ExchangeEnrollment returned an empty credential")
	}
	if _, err := svc.ExchangeEnrollment(ctx, enrollment.Computer.ID, enrollment.Token); !errors.Is(err, ErrInvalidEnrollment) {
		t.Fatalf("second ExchangeEnrollment error = %v, want ErrInvalidEnrollment", err)
	}
	if err := svc.AuthenticateCredential(ctx, enrollment.Computer.ID, credential.Credential); err != nil {
		t.Fatalf("AuthenticateCredential: %v", err)
	}
	if err := svc.RevokeCredential(ctx, enrollment.Computer.ID, ownerID); err != nil {
		t.Fatalf("RevokeCredential: %v", err)
	}
	if err := svc.AuthenticateCredential(ctx, enrollment.Computer.ID, credential.Credential); !errors.Is(err, ErrInvalidComputerCredential) {
		t.Fatalf("AuthenticateCredential after revoke error = %v, want ErrInvalidComputerCredential", err)
	}
	renewed, err := svc.CreateEnrollment(ctx, enrollment.Computer.ID, ownerID)
	if err != nil {
		t.Fatalf("CreateEnrollment: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE computers SET enrollment_expires_at = now() - interval '1 second' WHERE id = $1`, enrollment.Computer.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ExchangeEnrollment(ctx, enrollment.Computer.ID, renewed.Token); !errors.Is(err, ErrInvalidEnrollment) {
		t.Fatalf("expired ExchangeEnrollment error = %v, want ErrInvalidEnrollment", err)
	}
}

func TestMarkConnectedPausesLegacyDaemonRegistration(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	ownerID := taskSubmitUser(t, pool)
	svc := NewComputerService(pool)
	target, err := svc.CreateComputerWithEnrollment(ctx, ownerID, "secure-mac")
	if err != nil {
		t.Fatalf("CreateComputerWithEnrollment: %v", err)
	}
	if _, err := svc.ExchangeEnrollment(ctx, target.Computer.ID, target.Token); err != nil {
		t.Fatalf("ExchangeEnrollment: %v", err)
	}
	legacyID := uuid.NewString()
	daemonID := "daemon-" + legacyID[:8]
	if _, err := pool.Exec(ctx,
		`INSERT INTO computers (id, name, owner_id, daemon_id, status)
		 VALUES ($1, 'legacy-mac', $2, $3, 'online')`,
		legacyID, ownerID, daemonID,
	); err != nil {
		t.Fatalf("create legacy computer: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM computers WHERE id IN ($1, $2)`, target.Computer.ID, legacyID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	if err := svc.MarkConnected(ctx, target.Computer.ID, daemonID, "test", ComputerProtocolVersion, nil, ComputerSystemInfo{}, nil); err != nil {
		t.Fatalf("MarkConnected: %v", err)
	}
	var targetDaemonID, legacyDaemonID string
	var legacyStatus string
	if err := pool.QueryRow(ctx,
		`SELECT current.daemon_id, legacy.daemon_id, legacy.status
		   FROM computers current, computers legacy
		  WHERE current.id = $1 AND legacy.id = $2`,
		target.Computer.ID, legacyID,
	).Scan(&targetDaemonID, &legacyDaemonID, &legacyStatus); err != nil {
		t.Fatal(err)
	}
	if targetDaemonID != target.Computer.ID || legacyDaemonID != daemonID || legacyStatus != "offline" {
		t.Fatalf("target daemon = %v, legacy daemon/status = %v/%s", targetDaemonID, legacyDaemonID, legacyStatus)
	}
}

func TestMarkConnectedCanonicalizesSharedReportedDaemonIDs(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	ownerID := taskSubmitUser(t, pool)
	svc := NewComputerService(pool)

	first, err := svc.CreateComputerWithEnrollment(ctx, ownerID, "first-mac")
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.CreateComputerWithEnrollment(ctx, ownerID, "second-mac")
	if err != nil {
		t.Fatal(err)
	}
	for _, enrollment := range []*ComputerEnrollment{first, second} {
		if _, err := svc.ExchangeEnrollment(ctx, enrollment.Computer.ID, enrollment.Token); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM computers WHERE id IN ($1, $2)`, first.Computer.ID, second.Computer.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	for _, enrollment := range []*ComputerEnrollment{first, second} {
		if err := svc.MarkConnected(ctx, enrollment.Computer.ID, "daemon-01", "test", ComputerProtocolVersion, nil, ComputerSystemInfo{}, nil); err != nil {
			t.Fatalf("MarkConnected(%s): %v", enrollment.Computer.Name, err)
		}
	}

	rows, err := pool.Query(ctx, `SELECT id::text, daemon_id FROM computers WHERE id IN ($1, $2)`, first.Computer.ID, second.Computer.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var computerID, daemonID string
		if err := rows.Scan(&computerID, &daemonID); err != nil {
			t.Fatal(err)
		}
		if daemonID != computerID {
			t.Fatalf("Computer %s daemon_id = %q", computerID, daemonID)
		}
		seen++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen != 2 {
		t.Fatalf("connected Computers = %d, want 2", seen)
	}
}

func TestMarkConnectedKeepsLegacyDaemonIdentityReusable(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	ownerID := taskSubmitUser(t, pool)
	svc := NewComputerService(pool)
	paired, err := svc.CreateComputerWithEnrollment(ctx, ownerID, "paired-mac")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ExchangeEnrollment(ctx, paired.Computer.ID, paired.Token); err != nil {
		t.Fatal(err)
	}
	legacyID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO computers (id, name, daemon_id, status) VALUES ($1, 'legacy-mac', 'daemon-test-reusable', 'online')`,
		legacyID,
	); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM computers WHERE id IN ($1,$2)`, paired.Computer.ID, legacyID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, ownerID)
	})

	if err := svc.MarkConnected(ctx, paired.Computer.ID, "daemon-test-reusable", "test", ComputerProtocolVersion, nil, ComputerSystemInfo{}, nil); err != nil {
		t.Fatal(err)
	}
	var daemonID, status string
	if err := pool.QueryRow(ctx, `SELECT COALESCE(daemon_id,''),status FROM computers WHERE id=$1`, legacyID).Scan(&daemonID, &status); err != nil {
		t.Fatal(err)
	}
	if daemonID != "daemon-test-reusable" || status != "offline" {
		t.Fatalf("legacy Computer daemon/status = %q/%q", daemonID, status)
	}
	if err := svc.UpsertComputerByDaemonID(ctx, "daemon-test-reusable", "http://127.0.0.1:8081", "", ComputerSystemInfo{}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM computers WHERE daemon_id='daemon-test-reusable'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("legacy Daemon rows = %d, want 1", count)
	}
}

func TestClaimComputerRejectsNewMemberOnOwnedComputer(t *testing.T) {
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

	if _, err := NewComputerService(pool).ClaimComputer(ctx, computerID, memberID); !errors.Is(err, ErrComputerForbidden) {
		t.Fatalf("ClaimComputer second user error = %v, want ErrComputerForbidden", err)
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM computer_members WHERE computer_id = $1 AND user_id = $2`,
		computerID, memberID,
	).Scan(&count); err != nil {
		t.Fatalf("count member rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("member rows = %d, want 0", count)
	}
}

func TestDeleteComputerReportsBoundAgentConflict(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	ownerID := taskSubmitUser(t, pool)
	agentID := taskSubmitAgent(t, pool, ownerID)
	computerID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO computers (id, name, owner_id, status) VALUES ($1, 'in-use-mac', $2, 'offline')`,
		computerID, ownerID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE agents SET runtime_id=$2 WHERE id=$1`, agentID, computerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE agents SET runtime_id=NULL WHERE id=$1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM computers WHERE id=$1`, computerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id=$1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, ownerID)
	})

	if err := NewComputerService(pool).DeleteComputer(ctx, computerID, ownerID); !errors.Is(err, ErrComputerInUse) {
		t.Fatalf("DeleteComputer error = %v, want ErrComputerInUse", err)
	}
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM computers WHERE id=$1)`, computerID).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("bound Computer was deleted")
	}
}

func TestClaimComputerClaimsLegacyUnownedComputer(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	ownerID := taskSubmitUser(t, pool)
	computerID := uuid.NewString()
	_, err := pool.Exec(ctx,
		`INSERT INTO computers (id, name, daemon_id, status)
		 VALUES ($1, 'legacy-mac', $2, 'online')`,
		computerID, "daemon-"+computerID[:8],
	)
	if err != nil {
		t.Fatalf("create legacy computer: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM computers WHERE id = $1`, computerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	computer, err := NewComputerService(pool).ClaimComputer(ctx, computerID, ownerID)
	if err != nil {
		t.Fatalf("ClaimComputer: %v", err)
	}
	if computer.OwnerID != ownerID || computer.MyRole == nil || *computer.MyRole != "owner" {
		t.Fatalf("claimed computer = %#v, want owner %s", computer, ownerID)
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
