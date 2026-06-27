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
