package service

import (
	"context"
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
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	OwnerID       string     `json:"owner_id"`
	DaemonID      string     `json:"daemon_id,omitempty"`
	DaemonURL     string     `json:"daemon_url,omitempty"`
	Status        string     `json:"status"`
	LastHeartbeat *time.Time `json:"last_heartbeat,omitempty"`
	AgentIDs      []string   `json:"agent_ids,omitempty"`
	OS            string     `json:"os"`
	Hostname      string     `json:"hostname"`
	IP            string     `json:"ip"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	MyRole        *string    `json:"my_role,omitempty"`
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
	var c Computer
	err := s.pool.QueryRow(ctx,
		`INSERT INTO computers (name, owner_id)
		 VALUES ($1, NULLIF($2, '')::uuid)
		 RETURNING id, name, COALESCE(owner_id::text, ''), status, agent_ids, created_at, updated_at`,
		name, ownerID,
	).Scan(&c.ID, &c.Name, &c.OwnerID, &c.Status, &c.AgentIDs, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create computer: %w", err)
	}
	return &c, nil
}

// GetComputer retrieves a computer by ID. Only the owner can view it.
func (s *ComputerService) GetComputer(ctx context.Context, id, userID string) (*Computer, error) {
	var c Computer
	var ownerID, daemonID, daemonURL *string
	var lastHeartbeat *time.Time

	err := s.pool.QueryRow(ctx,
		`SELECT id, name, owner_id, daemon_id, daemon_url, status, last_heartbeat, agent_ids, os, hostname, ip, created_at, updated_at
		 FROM computers
		 WHERE id = $1`,
		id,
	).Scan(&c.ID, &c.Name, &ownerID, &daemonID, &daemonURL, &c.Status,
		&lastHeartbeat, &c.AgentIDs, &c.OS, &c.Hostname, &c.IP, &c.CreatedAt, &c.UpdatedAt)
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

	return &c, nil
}

// ListComputers lists online computers and annotates the caller's membership.
func (s *ComputerService) ListComputers(ctx context.Context, userID string) ([]Computer, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT c.id, c.name, COALESCE(c.owner_id::text, ''), COALESCE(c.daemon_id, ''), COALESCE(c.daemon_url, ''),
		        c.status, c.last_heartbeat, c.agent_ids, COALESCE(c.os, ''), COALESCE(c.hostname, ''), COALESCE(c.ip, ''), c.created_at, c.updated_at,
		        cm.role
		 FROM computers c
		 LEFT JOIN computer_members cm ON cm.computer_id = c.id AND cm.user_id = $1
		 WHERE c.status = 'online'
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
			&c.Status, &lastHeartbeat, &c.AgentIDs, &c.OS, &c.Hostname, &c.IP, &c.CreatedAt, &c.UpdatedAt, &role); err != nil {
			return nil, fmt.Errorf("scan computer row: %w", err)
		}
		c.DaemonID = daemonID
		c.DaemonURL = daemonURL
		c.LastHeartbeat = lastHeartbeat
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
		 WHERE id = $2
		 RETURNING id, name, owner_id, daemon_id, daemon_url, status, last_heartbeat, agent_ids, os, hostname, ip, created_at, updated_at`,
		name, id,
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

	return &c, nil
}

// DeleteComputer deletes a computer by ID. Only the owner can delete it.
func (s *ComputerService) DeleteComputer(ctx context.Context, id, userID string) error {
	result, err := s.pool.Exec(ctx,
		`DELETE FROM computers WHERE id = $1 AND owner_id = $2`,
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
	result, err := s.pool.Exec(ctx,
		`UPDATE computers SET status = 'online', last_heartbeat = $1, daemon_url = $2,
		        agent_ids = $3, os = $4, hostname = $5, ip = $6, updated_at = $1
		 WHERE daemon_id = $7`,
		now, daemonURL, agentIDs, sysinfo.OS, sysinfo.Hostname, sysinfo.IP, daemonID,
	)
	if err != nil {
		return fmt.Errorf("update heartbeat: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("update heartbeat: no computer with daemon_id %s", daemonID)
	}
	return nil
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

// ClaimComputer joins the user to a computer. If nobody owns it yet, this user
// becomes the first owner.
func (s *ComputerService) ClaimComputer(ctx context.Context, computerID, userID string) (*Computer, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("claim computer: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var hasOwner bool
	if err := tx.QueryRow(ctx,
		`SELECT owner_id IS NOT NULL FROM computers WHERE id = $1 FOR UPDATE`,
		computerID,
	).Scan(&hasOwner); err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("claim computer: get computer: %w", err)
	}

	role := "member"
	if !hasOwner {
		role = "owner"
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO computer_members (computer_id, user_id, role)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (computer_id, user_id) DO NOTHING`,
		computerID, userID, role,
	); err != nil {
		return nil, fmt.Errorf("claim computer: insert member: %w", err)
	}

	if !hasOwner {
		if _, err := tx.Exec(ctx,
			`UPDATE computers SET owner_id = $1, updated_at = now() WHERE id = $2`,
			userID, computerID,
		); err != nil {
			return nil, fmt.Errorf("claim computer: set owner: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("claim computer: commit: %w", err)
	}
	return s.GetComputer(ctx, computerID, userID)
}

// ErrNotFound is returned when a requested computer does not exist.
var ErrNotFound = fmt.Errorf("computer not found")
