package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ComputerService handles computer (daemon host) persistence and queries.
type ComputerService struct {
	pool *pgxpool.Pool
}

// Computer represents a registered daemon/computer.
type Computer struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	OwnerID          string          `json:"owner_id"`
	DaemonID         string          `json:"daemon_id,omitempty"`
	DaemonURL        string          `json:"daemon_url,omitempty"`
	Status           string          `json:"status"`
	LastHeartbeat    *time.Time      `json:"last_heartbeat,omitempty"`
	AgentIDs         []string        `json:"agent_ids,omitempty"`
	OS               string          `json:"os"`
	Hostname         string          `json:"hostname"`
	IP               string          `json:"ip"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	MyRole           *string         `json:"my_role,omitempty"`
	PairingStatus    string          `json:"pairing_status"`
	ProtocolVersion  int             `json:"protocol_version,omitempty"`
	DaemonVersion    string          `json:"daemon_version,omitempty"`
	RuntimeInventory json.RawMessage `json:"runtime_inventory,omitempty"`
	LastConnectedAt  *time.Time      `json:"last_connected_at,omitempty"`
}

// ComputerSystemInfo carries OS, hostname and IP reported by a daemon.
type ComputerSystemInfo struct {
	OS       string `json:"os"`
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
}

// NewComputerService creates a new ComputerService.
func NewComputerService(pool *pgxpool.Pool) *ComputerService {
	return &ComputerService{pool: pool}
}

// CreateComputer creates a new computer for the given owner.
func (s *ComputerService) CreateComputer(ctx context.Context, ownerID, name string) (*Computer, error) {
	return s.createComputer(ctx, ownerID, name, "", time.Time{})
}

// GetComputer retrieves a computer by ID. Only the owner can view it.
func (s *ComputerService) GetComputer(ctx context.Context, id, userID string) (*Computer, error) {
	var c Computer
	var ownerID, daemonID, daemonURL *string
	var lastHeartbeat *time.Time

	err := s.pool.QueryRow(ctx,
		`SELECT c.id, c.name, c.owner_id, c.daemon_id, c.daemon_url, c.status, c.last_heartbeat, c.agent_ids, c.os, c.hostname, c.ip, c.created_at, c.updated_at,
		        COALESCE(cm.role, CASE WHEN c.owner_id = $2 THEN 'owner' END),
		        CASE
		          WHEN c.credential_revoked_at IS NOT NULL THEN 'revoked'
		          WHEN c.credential_hash IS NOT NULL THEN 'paired'
		          WHEN c.enrollment_token_hash IS NOT NULL AND c.enrollment_expires_at > now() THEN 'pending'
		          ELSE 'unpaired'
		        END,
		        COALESCE(c.protocol_version, 0), COALESCE(c.daemon_version, ''), c.runtime_inventory, c.last_connected_at
		 FROM computers c
		 LEFT JOIN computer_members cm ON cm.computer_id = c.id AND cm.user_id = $2
		 WHERE c.id = $1
		   AND (c.owner_id = $2 OR cm.user_id IS NOT NULL)`,
		id, userID,
	).Scan(&c.ID, &c.Name, &ownerID, &daemonID, &daemonURL, &c.Status,
		&lastHeartbeat, &c.AgentIDs, &c.OS, &c.Hostname, &c.IP, &c.CreatedAt, &c.UpdatedAt,
		&c.MyRole, &c.PairingStatus, &c.ProtocolVersion, &c.DaemonVersion, &c.RuntimeInventory, &c.LastConnectedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get computer: %w", err)
	}

	if ownerID != nil {
		c.OwnerID = *ownerID
	}
	if daemonID != nil {
		c.DaemonID = *daemonID
	}
	if daemonURL != nil {
		c.DaemonURL = *daemonURL
	}
	c.LastHeartbeat = lastHeartbeat
	c.AgentIDs, err = s.activeAgentIDs(ctx, c.AgentIDs)
	if err != nil {
		return nil, fmt.Errorf("get computer active agents: %w", err)
	}

	return &c, nil
}

// ListComputers lists every computer accessible to the caller, including
// offline and pending-pairing computers.
func (s *ComputerService) ListComputers(ctx context.Context, userID string) ([]Computer, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT c.id, c.name, COALESCE(c.owner_id::text, ''), COALESCE(c.daemon_id, ''), COALESCE(c.daemon_url, ''),
		        c.status, c.last_heartbeat, c.agent_ids, COALESCE(c.os, ''), COALESCE(c.hostname, ''), COALESCE(c.ip, ''), c.created_at, c.updated_at,
		        COALESCE(cm.role, CASE WHEN c.owner_id = $1 THEN 'owner' END),
		        CASE
		          WHEN c.credential_revoked_at IS NOT NULL THEN 'revoked'
		          WHEN c.credential_hash IS NOT NULL THEN 'paired'
		          WHEN c.enrollment_token_hash IS NOT NULL AND c.enrollment_expires_at > now() THEN 'pending'
		          ELSE 'unpaired'
		        END,
		        COALESCE(c.protocol_version, 0), COALESCE(c.daemon_version, ''), c.runtime_inventory, c.last_connected_at
		 FROM computers c
		 LEFT JOIN computer_members cm ON cm.computer_id = c.id AND cm.user_id = $1
		 WHERE c.owner_id = $1
		    OR cm.user_id IS NOT NULL
		    OR (c.owner_id IS NULL AND c.credential_hash IS NULL AND c.status = 'online')
		 ORDER BY c.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list computers: %w", err)
	}
	defer rows.Close()

	var computers []Computer
	for rows.Next() {
		var c Computer
		var daemonID, daemonURL string
		var lastHeartbeat *time.Time
		var role *string
		if err := rows.Scan(&c.ID, &c.Name, &c.OwnerID, &daemonID, &daemonURL,
			&c.Status, &lastHeartbeat, &c.AgentIDs, &c.OS, &c.Hostname, &c.IP, &c.CreatedAt, &c.UpdatedAt, &role,
			&c.PairingStatus, &c.ProtocolVersion, &c.DaemonVersion, &c.RuntimeInventory, &c.LastConnectedAt); err != nil {
			return nil, fmt.Errorf("scan computer row: %w", err)
		}
		c.DaemonID = daemonID
		c.DaemonURL = daemonURL
		c.LastHeartbeat = lastHeartbeat
		c.AgentIDs, err = s.activeAgentIDs(ctx, c.AgentIDs)
		if err != nil {
			return nil, fmt.Errorf("list computers active agents: %w", err)
		}
		c.MyRole = role
		computers = append(computers, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate computers: %w", err)
	}

	if computers == nil {
		computers = []Computer{}
	}
	return computers, nil
}

// UpdateComputer updates the name of a computer. Only the owner can update it.
func (s *ComputerService) UpdateComputer(ctx context.Context, id, userID, name string) (*Computer, error) {
	var c Computer
	var ownerID, daemonID, daemonURL *string
	var lastHeartbeat *time.Time

	err := s.pool.QueryRow(ctx,
		`UPDATE computers SET name = $1, updated_at = now()
		 WHERE id = $2 AND owner_id = $3
		 RETURNING id, name, owner_id, daemon_id, daemon_url, status, last_heartbeat, agent_ids, os, hostname, ip, created_at, updated_at`,
		name, id, userID,
	).Scan(&c.ID, &c.Name, &ownerID, &daemonID, &daemonURL, &c.Status,
		&lastHeartbeat, &c.AgentIDs, &c.OS, &c.Hostname, &c.IP, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("update computer: %w", err)
	}

	if ownerID != nil {
		c.OwnerID = *ownerID
	}
	if daemonID != nil {
		c.DaemonID = *daemonID
	}
	if daemonURL != nil {
		c.DaemonURL = *daemonURL
	}
	c.LastHeartbeat = lastHeartbeat
	c.AgentIDs, err = s.activeAgentIDs(ctx, c.AgentIDs)
	if err != nil {
		return nil, fmt.Errorf("update computer active agents: %w", err)
	}

	return s.GetComputer(ctx, id, userID)
}

// DeleteComputer deletes a computer by ID. Only the owner can delete it.
func (s *ComputerService) DeleteComputer(ctx context.Context, id, userID string) error {
	result, err := s.pool.Exec(ctx,
		`DELETE FROM computers c
		 WHERE c.id = $1 AND c.owner_id = $2
		   AND NOT EXISTS (
		       SELECT 1 FROM agents a
		        WHERE a.runtime_id = c.id::text AND a.is_active = true
		   )
		   AND NOT EXISTS (
		       SELECT 1 FROM agent_runs r
		        WHERE r.computer_id = c.id AND r.finished_at IS NULL
		   )`,
		id, userID,
	)
	if err != nil {
		return fmt.Errorf("delete computer: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpsertComputerByDaemonID creates or updates a computer record based on the
// daemon_id. This is called during daemon registration.
// If ownerID is empty the computer is created unclaimed (owner_id = NULL);
// the user claims it later via ClaimComputer.
func (s *ComputerService) UpsertComputerByDaemonID(ctx context.Context, daemonID, daemonURL, ownerID string, sysinfo ComputerSystemInfo) error {
	now := time.Now()
	name := sysinfo.Hostname
	if name == "" {
		name = daemonID
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO computers (name, owner_id, daemon_id, daemon_url, status, os, hostname, ip, last_heartbeat, updated_at)
		 VALUES ($1, NULLIF($2, '')::uuid, $3, $4, 'online', $5, $6, $7, $8, $8)
		 ON CONFLICT (daemon_id) WHERE daemon_id IS NOT NULL
		 DO UPDATE SET daemon_url = $4, status = 'online', os = $5, hostname = $6, ip = $7, last_heartbeat = $8, updated_at = $8`,
		name, ownerID, daemonID, daemonURL, sysinfo.OS, sysinfo.Hostname, sysinfo.IP, now,
	)
	if err != nil {
		return fmt.Errorf("upsert computer: %w", err)
	}

	slog.Info("computer upserted via daemon registration",
		"daemon_id", daemonID,
		"daemon_url", daemonURL,
		"owner_id", ownerID,
	)
	return nil
}

// UpdateHeartbeat updates the last_heartbeat time and status for a computer
// identified by daemon_id. Called on daemon heartbeat.
// Returns an error if no computer row matched — this signals the daemon should
// re-register to recreate the missing row.
func (s *ComputerService) UpdateHeartbeat(ctx context.Context, daemonID, daemonURL string, agentIDs []string, sysinfo ComputerSystemInfo) error {
	now := time.Now()
	activeAgentIDs, err := s.activeAgentIDs(ctx, agentIDs)
	if err != nil {
		return fmt.Errorf("update heartbeat active agents: %w", err)
	}
	result, err := s.pool.Exec(ctx,
		`UPDATE computers SET status = 'online', last_heartbeat = $1, daemon_url = $2,
		        agent_ids = $3, os = $4, hostname = $5, ip = $6, updated_at = $1
		 WHERE daemon_id = $7`,
		now, daemonURL, activeAgentIDs, sysinfo.OS, sysinfo.Hostname, sysinfo.IP, daemonID,
	)
	if err != nil {
		return fmt.Errorf("update heartbeat: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("update heartbeat: no computer with daemon_id %s", daemonID)
	}
	return nil
}

func (s *ComputerService) activeAgentIDs(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return []string{}, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text FROM agents WHERE id = ANY($1::uuid[]) AND is_active = true`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	active := make(map[string]bool, len(ids))
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		active[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]string, 0, len(active))
	for _, id := range ids {
		if active[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

// MarkOffline marks computers as offline where last_heartbeat is older than
// 60 seconds from now.
func (s *ComputerService) MarkOffline(ctx context.Context) (int64, error) {
	result, err := s.pool.Exec(ctx,
		`UPDATE computers SET status = 'offline', updated_at = now()
		 WHERE status = 'online' AND last_heartbeat < now() - INTERVAL '60 seconds'`,
	)
	if err != nil {
		return 0, fmt.Errorf("mark offline: %w", err)
	}
	n := result.RowsAffected()
	if n > 0 {
		slog.Info("computers marked offline due to missed heartbeat", "count", n)
	}
	return n, nil
}

// ClaimComputer claims an unpaired legacy computer. Paired computers are only
// accessible through their existing owner/member records.
func (s *ComputerService) ClaimComputer(ctx context.Context, computerID, userID string) (*Computer, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("claim computer: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var ownerID *string
	var hasCredential, hasAccess bool
	var status string
	if err := tx.QueryRow(ctx,
		`SELECT c.owner_id::text, c.credential_hash IS NOT NULL, c.status,
		        COALESCE(c.owner_id = $2, false) OR EXISTS (
		            SELECT 1 FROM computer_members cm
		             WHERE cm.computer_id = c.id AND cm.user_id = $2
		        )
		   FROM computers c WHERE c.id = $1 FOR UPDATE OF c`,
		computerID, userID,
	).Scan(&ownerID, &hasCredential, &status, &hasAccess); err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("claim computer: get computer: %w", err)
	}

	if hasAccess {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("claim computer: commit: %w", err)
		}
		return s.GetComputer(ctx, computerID, userID)
	}
	if ownerID != nil || hasCredential || status != "online" {
		return nil, ErrComputerForbidden
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO computer_members (computer_id, user_id, role)
		 VALUES ($1, $2, 'owner')
		 ON CONFLICT (computer_id, user_id) DO NOTHING`,
		computerID, userID,
	); err != nil {
		return nil, fmt.Errorf("claim computer: insert member: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE computers SET owner_id = $1, updated_at = now() WHERE id = $2`,
		userID, computerID,
	); err != nil {
		return nil, fmt.Errorf("claim computer: set owner: %w", err)
	}

	// A legacy local Computer can be recreated after its row was removed while
	// active Agents still retain the old text binding. Once the user claims the
	// replacement, move only idle Agents from an offline, unpaired record for
	// the same host. Paired Computers and active Runs are never moved implicitly.
	result, err := tx.Exec(ctx, `
		UPDATE agents a
		   SET runtime_id = target.id::text, updated_at = now()
		  FROM computers target, computers previous
		 WHERE target.id = $1
		   AND previous.id::text = a.runtime_id
		   AND previous.id <> target.id
		   AND a.owner_id = $2
		   AND a.is_active = true
		   AND previous.owner_id = $2
		   AND previous.status = 'offline'
		   AND previous.credential_hash IS NULL
		   AND target.credential_hash IS NULL
		   AND previous.hostname <> ''
		   AND previous.hostname = target.hostname
		   AND previous.os = target.os
		   AND NOT EXISTS (
		       SELECT 1 FROM agent_runs r
		        WHERE r.agent_id = a.id AND r.finished_at IS NULL
		   )`, computerID, userID)
	if err != nil {
		return nil, fmt.Errorf("claim computer: recover agent bindings: %w", err)
	}
	migratedAgents := result.RowsAffected()

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("claim computer: commit: %w", err)
	}
	if migratedAgents > 0 {
		slog.Info("recovered legacy agent computer bindings",
			"computer_id", computerID,
			"user_id", userID,
			"agent_count", migratedAgents,
		)
	}
	return s.GetComputer(ctx, computerID, userID)
}

// ErrNotFound is returned when a requested computer does not exist.
var ErrNotFound = fmt.Errorf("computer not found")
