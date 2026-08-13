package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	serverworkspace "github.com/solo-ai/solo/internal/server/workspace"
)

// WorkspaceScope resolves the active logical Workspace after authentication.
// Platform-global endpoints remain usable when a remembered Workspace was
// deleted so clients can recover by fetching their accessible Workspace list.
func WorkspaceScope(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := r.Header.Get("X-User-ID")
			if userID == "" {
				writeAuthError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
				return
			}

			if workspaceScopeExempt(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			requested := strings.TrimSpace(r.Header.Get(serverworkspace.Header))
			actorType := r.Header.Get("X-Solo-Actor-Type")
			isAgent := actorType == "agent" || actorType == "agent_run" || strings.HasSuffix(r.Header.Get("X-User-Email"), "@solo.agent")
			if isAgent {
				var workspaceID string
				var err error
				if actorType == "agent_run" {
					err = pool.QueryRow(r.Context(), `
						SELECT c.workspace_id::text
						  FROM agent_runs run
						  JOIN channels c ON c.id = run.channel_id
						 WHERE run.id = $1 AND run.agent_id = $2`,
						r.Header.Get("X-Solo-Run-ID"), userID).Scan(&workspaceID)
				} else {
					err = pool.QueryRow(r.Context(), `
						SELECT c.workspace_id::text
						  FROM agents a
						  JOIN channels c ON c.id = a.home_channel_id
						 WHERE a.id = $1 AND a.is_active = true`, userID).Scan(&workspaceID)
				}
				if err != nil || (requested != "" && requested != workspaceID) {
					writeAuthError(w, http.StatusForbidden, "forbidden", "Workspace access denied")
					return
				}
				r.Header.Set(serverworkspace.Header, workspaceID)
				w.Header().Set(serverworkspace.Header, workspaceID)
				r = r.WithContext(serverworkspace.WithScope(r.Context(), serverworkspace.Scope{ID: workspaceID, Role: "agent"}))
				if !resourceBelongsToWorkspace(pool, r, workspaceID) {
					writeAuthError(w, http.StatusNotFound, "not_found", "resource not found")
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			if requested == "" {
				requested = serverworkspace.PublicID
			}
			var role string
			err := pool.QueryRow(r.Context(), `
				SELECT wm.role FROM workspace_members wm
				JOIN workspaces w ON w.id=wm.workspace_id AND w.deleted_at IS NULL
				WHERE wm.workspace_id = $1 AND wm.user_id = $2`,
				requested, userID).Scan(&role)
			if errors.Is(err, pgx.ErrNoRows) {
				writeAuthError(w, http.StatusForbidden, "forbidden", "Workspace access denied")
				return
			}
			if err != nil {
				writeAuthError(w, http.StatusInternalServerError, "internal_error", "failed to resolve Workspace")
				return
			}
			r.Header.Set(serverworkspace.Header, requested)
			w.Header().Set(serverworkspace.Header, requested)
			r = r.WithContext(serverworkspace.WithScope(r.Context(), serverworkspace.Scope{ID: requested, Role: role}))
			if !resourceBelongsToWorkspace(pool, r, requested) {
				writeAuthError(w, http.StatusNotFound, "not_found", "resource not found")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// resourceBelongsToWorkspace closes ID-based side doors for handlers whose
// authorization historically checked only ownership or channel membership.
// Collection endpoints are scoped in their queries; this guard covers concrete
// resource IDs shared by nested API paths.
func resourceBelongsToWorkspace(pool *pgxpool.Pool, r *http.Request, workspaceID string) bool {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 || parts[0] != "api" || parts[1] != "v1" {
		return true
	}
	resourceType, resourceID := parts[2], parts[3]
	if _, err := uuid.Parse(resourceID); err != nil {
		return true
	}

	var query string
	switch resourceType {
	case "channels", "dm":
		query = `SELECT EXISTS(SELECT 1 FROM channels WHERE id=$1 AND workspace_id=$2)`
	case "agents":
		query = `SELECT EXISTS(SELECT 1 FROM agents a JOIN channels c ON c.id=a.home_channel_id WHERE a.id=$1 AND c.workspace_id=$2)`
	case "tasks":
		query = `SELECT EXISTS(SELECT 1 FROM tasks t JOIN channels c ON c.id=t.channel_id WHERE t.id=$1 AND c.workspace_id=$2)`
	case "agent-runs":
		query = `SELECT EXISTS(SELECT 1 FROM agent_runs ar JOIN channels c ON c.id=ar.channel_id WHERE ar.id=$1 AND c.workspace_id=$2)`
	case "agent-sessions":
		query = `SELECT EXISTS(SELECT 1 FROM agent_sessions s JOIN agents a ON a.id=s.agent_id JOIN channels c ON c.id=a.home_channel_id WHERE s.id=$1 AND c.workspace_id=$2)`
	case "artifacts":
		query = `SELECT EXISTS(SELECT 1 FROM artifacts a JOIN tasks t ON t.id=a.task_id JOIN channels c ON c.id=t.channel_id WHERE a.id=$1 AND c.workspace_id=$2)`
	case "threads":
		query = `SELECT EXISTS(SELECT 1 FROM threads t JOIN channels c ON c.id=t.channel_id WHERE t.id=$1 AND c.workspace_id=$2)`
	case "agent-relationships":
		query = `SELECT EXISTS(SELECT 1 FROM agent_relationships ar JOIN agents a ON a.id=ar.from_agent_id JOIN channels c ON c.id=a.home_channel_id WHERE ar.id=$1 AND c.workspace_id=$2)`
	default:
		return true
	}
	var exists bool
	return pool.QueryRow(r.Context(), query, resourceID, workspaceID).Scan(&exists) == nil && exists
}

func workspaceScopeExempt(path string) bool {
	return strings.HasPrefix(path, "/api/v1/workspaces") ||
		strings.HasPrefix(path, "/api/v1/users/") ||
		strings.HasPrefix(path, "/api/v1/auth/") ||
		strings.HasPrefix(path, "/api/v1/computers")
}
