package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	ComputerProtocolVersion = 1
	computerEnrollmentTTL   = 10 * time.Minute
)

var (
	ErrComputerForbidden         = errors.New("computer access forbidden")
	ErrInvalidEnrollment         = errors.New("invalid or expired computer enrollment")
	ErrInvalidComputerCredential = errors.New("invalid computer credential")
)

type ComputerEnrollment struct {
	Computer  *Computer `json:"computer"`
	Token     string    `json:"enrollment_token"`
	ExpiresAt time.Time `json:"enrollment_expires_at"`
}

type ComputerCredential struct {
	ComputerID string `json:"computer_id"`
	Credential string `json:"credential"`
}

func (s *ComputerService) CreateComputerWithEnrollment(ctx context.Context, ownerID, name string) (*ComputerEnrollment, error) {
	token, err := randomComputerSecret()
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().UTC().Add(computerEnrollmentTTL)
	computer, err := s.createComputer(ctx, ownerID, name, hashComputerSecret(token), expiresAt)
	if err != nil {
		return nil, err
	}
	computer.PairingStatus = "pending"
	return &ComputerEnrollment{Computer: computer, Token: token, ExpiresAt: expiresAt}, nil
}

func (s *ComputerService) createComputer(ctx context.Context, ownerID, name, enrollmentHash string, enrollmentExpiresAt time.Time) (*Computer, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("create computer: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var c Computer
	err = tx.QueryRow(ctx,
		`INSERT INTO computers (name, owner_id, enrollment_token_hash, enrollment_expires_at)
		 VALUES ($1, NULLIF($2, '')::uuid, NULLIF($3, ''), $4)
		 RETURNING id, name, COALESCE(owner_id::text, ''), status, agent_ids, created_at, updated_at`,
		name, ownerID, enrollmentHash, nullableComputerTime(enrollmentExpiresAt),
	).Scan(&c.ID, &c.Name, &c.OwnerID, &c.Status, &c.AgentIDs, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create computer: %w", err)
	}
	if ownerID != "" {
		if _, err := tx.Exec(ctx,
			`INSERT INTO computer_members (computer_id, user_id, role) VALUES ($1, $2, 'owner')`,
			c.ID, ownerID,
		); err != nil {
			return nil, fmt.Errorf("create computer member: %w", err)
		}
		role := "owner"
		c.MyRole = &role
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("create computer: commit: %w", err)
	}
	if enrollmentHash == "" {
		c.PairingStatus = "unpaired"
	} else {
		c.PairingStatus = "pending"
	}
	c.RuntimeInventory = json.RawMessage("[]")
	return &c, nil
}

func (s *ComputerService) CreateEnrollment(ctx context.Context, computerID, userID string) (*ComputerEnrollment, error) {
	token, err := randomComputerSecret()
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().UTC().Add(computerEnrollmentTTL)
	result, err := s.pool.Exec(ctx,
		`UPDATE computers
		    SET enrollment_token_hash = $1,
		        enrollment_expires_at = $2,
		        enrollment_used_at = NULL,
		        updated_at = now()
		  WHERE id = $3 AND owner_id = $4`,
		hashComputerSecret(token), expiresAt, computerID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("create enrollment: %w", err)
	}
	if result.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	computer, err := s.GetComputer(ctx, computerID, userID)
	if err != nil {
		return nil, err
	}
	computer.PairingStatus = "pending"
	return &ComputerEnrollment{Computer: computer, Token: token, ExpiresAt: expiresAt}, nil
}

func (s *ComputerService) ExchangeEnrollment(ctx context.Context, computerID, token string) (*ComputerCredential, error) {
	var storedHash string
	var expiresAt time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(enrollment_token_hash, ''), enrollment_expires_at
		   FROM computers
		  WHERE id = $1 AND enrollment_used_at IS NULL`,
		computerID,
	).Scan(&storedHash, &expiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidEnrollment
		}
		return nil, fmt.Errorf("load enrollment: %w", err)
	}
	providedHash := hashComputerSecret(token)
	if time.Now().UTC().After(expiresAt) || !constantSecretEqual(storedHash, providedHash) {
		return nil, ErrInvalidEnrollment
	}

	credential, err := randomComputerSecret()
	if err != nil {
		return nil, err
	}
	result, err := s.pool.Exec(ctx,
		`UPDATE computers
		    SET credential_hash = $1,
		        credential_created_at = now(),
		        credential_revoked_at = NULL,
		        enrollment_used_at = now(),
		        enrollment_token_hash = NULL,
		        enrollment_expires_at = NULL,
		        updated_at = now()
		  WHERE id = $2
		    AND enrollment_used_at IS NULL
		    AND enrollment_token_hash = $3`,
		hashComputerSecret(credential), computerID, storedHash,
	)
	if err != nil {
		return nil, fmt.Errorf("exchange enrollment: %w", err)
	}
	if result.RowsAffected() != 1 {
		return nil, ErrInvalidEnrollment
	}
	return &ComputerCredential{ComputerID: computerID, Credential: credential}, nil
}

func (s *ComputerService) AuthenticateCredential(ctx context.Context, computerID, credential string) error {
	var storedHash string
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(credential_hash, '')
		   FROM computers
		  WHERE id = $1 AND credential_revoked_at IS NULL`,
		computerID,
	).Scan(&storedHash)
	if err != nil || storedHash == "" || !constantSecretEqual(storedHash, hashComputerSecret(credential)) {
		return ErrInvalidComputerCredential
	}
	return nil
}

func (s *ComputerService) RevokeCredential(ctx context.Context, computerID, userID string) error {
	result, err := s.pool.Exec(ctx,
		`UPDATE computers
		    SET credential_revoked_at = now(), status = 'offline', updated_at = now()
		  WHERE id = $1 AND owner_id = $2 AND credential_hash IS NOT NULL`,
		computerID, userID,
	)
	if err != nil {
		return fmt.Errorf("revoke computer credential: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *ComputerService) MarkConnected(ctx context.Context, computerID, reportedDaemonID, daemonVersion string, protocolVersion int, inventory json.RawMessage, sysinfo ComputerSystemInfo, agentIDs []string) error {
	// The authenticated Computer is the remote routing identity. Older clients
	// all report "daemon-01", so persisting that client label would make the
	// second paired Computer violate the global daemon_id uniqueness constraint.
	canonicalDaemonID := computerID
	if len(inventory) == 0 || !json.Valid(inventory) {
		inventory = json.RawMessage("[]")
	}
	activeAgentIDs, err := s.activeAgentIDs(ctx, agentIDs)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("mark computer connected: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	// Secure pairing supersedes the old unauthenticated local registration for
	// the same physical daemon while preserving that row as history.
	if _, err := tx.Exec(ctx,
		`UPDATE computers
		    SET daemon_id = NULL, daemon_url = NULL, status = 'offline', updated_at = now()
		  WHERE daemon_id = $1 AND id <> $2 AND credential_hash IS NULL`,
		reportedDaemonID, computerID,
	); err != nil {
		return fmt.Errorf("mark computer connected: release legacy registration: %w", err)
	}

	result, err := tx.Exec(ctx,
		`UPDATE computers
		    SET daemon_id = $2, daemon_url = NULL, status = 'online',
		        daemon_version = $3, protocol_version = $4,
		        runtime_inventory = $5, os = $6, hostname = $7, ip = $8,
		        agent_ids = $9, last_heartbeat = now(), last_connected_at = now(), updated_at = now()
		  WHERE id = $1 AND credential_hash IS NOT NULL AND credential_revoked_at IS NULL`,
		computerID, canonicalDaemonID, daemonVersion, protocolVersion, inventory,
		sysinfo.OS, sysinfo.Hostname, sysinfo.IP, activeAgentIDs,
	)
	if err != nil {
		return fmt.Errorf("mark computer connected: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrInvalidComputerCredential
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("mark computer connected: commit: %w", err)
	}
	return nil
}

func (s *ComputerService) UpdateRemoteHeartbeat(ctx context.Context, computerID string, agentIDs []string, inventory json.RawMessage) error {
	activeAgentIDs, err := s.activeAgentIDs(ctx, agentIDs)
	if err != nil {
		return err
	}
	if len(inventory) == 0 || !json.Valid(inventory) {
		inventory = nil
	}
	result, err := s.pool.Exec(ctx,
		`UPDATE computers
		    SET status = 'online', last_heartbeat = now(), agent_ids = $2,
		        runtime_inventory = COALESCE($3, runtime_inventory), updated_at = now()
		  WHERE id = $1 AND credential_hash IS NOT NULL AND credential_revoked_at IS NULL`,
		computerID, activeAgentIDs, nullableJSON(inventory),
	)
	if err != nil {
		return fmt.Errorf("update remote heartbeat: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrInvalidComputerCredential
	}
	return nil
}

func randomComputerSecret() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate computer secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func hashComputerSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func constantSecretEqual(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func nullableComputerTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return value
}
