package service

import (
	"context"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const PublicWorkspaceID = "00000000-0000-0000-0000-000000000001"

func PublicWorkspaceRole(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	for _, configured := range strings.Split(os.Getenv("SOLO_PUBLIC_OWNER_EMAILS"), ",") {
		if email != "" && email == strings.ToLower(strings.TrimSpace(configured)) {
			return "owner"
		}
	}
	return "member"
}

// EnsurePublicWorkspaceOwners applies the configured ownership policy to
// existing public members. It intentionally grants no role in private Workspaces.
func EnsurePublicWorkspaceOwners(ctx context.Context, pool *pgxpool.Pool) error {
	for _, configured := range strings.Split(os.Getenv("SOLO_PUBLIC_OWNER_EMAILS"), ",") {
		email := strings.ToLower(strings.TrimSpace(configured))
		if email == "" {
			continue
		}
		if _, err := pool.Exec(ctx, `
			UPDATE workspace_members wm SET role='owner'
			  FROM users u
			 WHERE wm.workspace_id=$1 AND wm.user_id=u.id AND lower(u.email)=$2
			   AND u.email_verified_at IS NOT NULL`, PublicWorkspaceID, email); err != nil {
			return err
		}
	}
	return nil
}
