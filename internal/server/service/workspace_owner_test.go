package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
)

func TestPublicWorkspaceRoleUsesConfiguredEmailsOnly(t *testing.T) {
	t.Setenv("SOLO_PUBLIC_OWNER_EMAILS", " Fredal_Zhu@outlook.com,1271123275@qq.com ")
	for _, email := range []string{"fredal_zhu@outlook.com", "FREDAL_ZHU@OUTLOOK.COM", "1271123275@qq.com"} {
		if role := PublicWorkspaceRole(email); role != "owner" {
			t.Fatalf("PublicWorkspaceRole(%q) = %q, want owner", email, role)
		}
	}
	if role := PublicWorkspaceRole("member@example.com"); role != "member" {
		t.Fatalf("ordinary email role = %q, want member", role)
	}
}

func TestEnsurePublicWorkspaceOwnersRequiresVerifiedEmail(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	suffix := uuid.NewString()
	verifiedID, unverifiedID := uuid.NewString(), uuid.NewString()
	verifiedEmail := fmt.Sprintf("public-owner-verified-%s@example.test", suffix)
	unverifiedEmail := fmt.Sprintf("public-owner-unverified-%s@example.test", suffix)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users(id,email,display_name,password_hash,email_verified_at)
		VALUES($1,$2,'Verified Owner','test',now()),($3,$4,'Unverified Owner','test',NULL)`,
		verifiedID, verifiedEmail, unverifiedID, unverifiedEmail); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workspace_members(workspace_id,user_id,role)
		VALUES($1,$2,'member'),($1,$3,'member')`, PublicWorkspaceID, verifiedID, unverifiedID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM workspace_members WHERE user_id IN ($1,$2)`, verifiedID, unverifiedID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1,$2)`, verifiedID, unverifiedID)
	})
	t.Setenv("SOLO_PUBLIC_OWNER_EMAILS", verifiedEmail+","+unverifiedEmail)

	if err := EnsurePublicWorkspaceOwners(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var verifiedRole, unverifiedRole string
	if err := pool.QueryRow(ctx, `SELECT role FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, PublicWorkspaceID, verifiedID).Scan(&verifiedRole); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT role FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, PublicWorkspaceID, unverifiedID).Scan(&unverifiedRole); err != nil {
		t.Fatal(err)
	}
	if verifiedRole != "owner" || unverifiedRole != "member" {
		t.Fatalf("roles=(%q,%q), want (owner,member)", verifiedRole, unverifiedRole)
	}
}
