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
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
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
		 VALUES ($1, $2)
		 RETURNING id, name, owner_id, status, agent_ids, created_at, updated_at`,
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
	var daemonID, daemonURL *string
	var lastHeartbeat *time.Time

	err := s.pool.QueryRow(ctx,
		`SELECT id, name, owner_id, daemon_id, daemon_url, status, last_heartbeat, agent_ids, created_at, updated_at
		 FROM computers
		 WHERE id = $1 AND owner_id = $2`,
		id, userID,
	).Scan(&c.ID, &c.Name, &c.OwnerID, &daemonID, &daemonURL, &c.Status,
		&lastHeartbeat, &c.AgentIDs, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get computer: %w", err)
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

// ListComputers lists all computers owned by the given user.
func (s *ComputerService) ListComputers(ctx context.Context, userID string) ([]Computer, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, owner_id, COALESCE(daemon_id, ''), COALESCE(daemon_url, ''),
		        status, last_heartbeat, agent_ids, created_at, updated_at
		 FROM computers
		 WHERE owner_id = $1
		 ORDER BY created_at DESC`,
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
		if err := rows.Scan(&c.ID, &c.Name, &c.OwnerID, &daemonID, &daemonURL,
			&c.Status, &lastHeartbeat, &c.AgentIDs, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan computer row: %w", err)
		}
		c.DaemonID = daemonID
		c.DaemonURL = daemonURL
		c.LastHeartbeat = lastHeartbeat
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
	var daemonID, daemonURL *string
	var lastHeartbeat *time.Time

	err := s.pool.QueryRow(ctx,
		`UPDATE computers SET name = $1, updated_at = now()
		 WHERE id = $2 AND owner_id = $3
		 RETURNING id, name, owner_id, daemon_id, daemon_url, status, last_heartbeat, agent_ids, created_at, updated_at`,
		name, id, userID,
	).Scan(&c.ID, &c.Name, &c.OwnerID, &daemonID, &daemonURL, &c.Status,
		&lastHeartbeat, &c.AgentIDs, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("update computer: %w", err)
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
func (s *ComputerService) UpsertComputerByDaemonID(ctx context.Context, daemonID, daemonURL, ownerID string) error {
	// Use 'solo-server' as default owner if not specified — the first user
	// in the system will be used for auto-registered computers.
	if ownerID == "" {
		var firstUserID string
		err := s.pool.QueryRow(ctx,
			`SELECT id FROM users ORDER BY created_at ASC LIMIT 1`,
		).Scan(&firstUserID)
		if err != nil {
			return fmt.Errorf("upsert computer: no users found for owner: %w", err)
		}
		ownerID = firstUserID
	}

	now := time.Now()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO computers (name, owner_id, daemon_id, daemon_url, status, last_heartbeat, updated_at)
		 VALUES ($1, $2, $3, $4, 'online', $5, $5)
		 ON CONFLICT (daemon_id) WHERE daemon_id IS NOT NULL
		 DO UPDATE SET daemon_url = $4, status = 'online', last_heartbeat = $5, updated_at = $5`,
		daemonID, ownerID, daemonID, daemonURL, now,
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
func (s *ComputerService) UpdateHeartbeat(ctx context.Context, daemonID, daemonURL string, agentIDs []string) error {
	now := time.Now()
	_, err := s.pool.Exec(ctx,
		`UPDATE computers SET status = 'online', last_heartbeat = $1, daemon_url = $2,
		        agent_ids = $3, updated_at = $1
		 WHERE daemon_id = $4`,
		now, daemonURL, agentIDs, daemonID,
	)
	if err != nil {
		return fmt.Errorf("update heartbeat: %w", err)
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

// ErrNotFound is returned when a requested computer does not exist.
var ErrNotFound = fmt.Errorf("computer not found")
