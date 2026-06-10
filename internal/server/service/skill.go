package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/pkg/skillloader"
)

// SkillSummary is the list-endpoint shape: everything Skill has except body.
type SkillSummary struct {
	ID          string
	Name        string
	Description string
	SourcePath  string
	SourceKind  string
	BodyHash    string
	DiscoveredAt time.Time
	UpdatedAt    time.Time
}

// Skill is the detail-endpoint shape: full content + files slice.
type Skill struct {
	SkillSummary
	Body  string
	Files []SkillFile
}

// SkillFile is a supporting file inside a skill.
type SkillFile struct {
	ID        string
	Path      string
	Content   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SkillService is the entry point for skill-related operations.
type SkillService struct {
	pool *pgxpool.Pool
}

// NewSkillService creates a new SkillService.
func NewSkillService(pool *pgxpool.Pool) *SkillService {
	return &SkillService{pool: pool}
}

// SyncFromDaemon reconciles skills reported by a daemon with the DB.
// Daemon sends {name, desc, path, kind, body, body_hash} from its local scan.
// Server inserts new, updates changed, and deletes skills whose source_path
// no longer appears in the report (but only for paths managed by this daemon's
// provider kind, to avoid deleting skills reported by other daemons).
func (s *SkillService) SyncFromDaemon(ctx context.Context, reported []skillloader.DiscoveredSkill) (added, updated, removed int, err error) {
	// Index reported skills by name.
	reportedByName := make(map[string]skillloader.DiscoveredSkill, len(reported))
	for _, ds := range reported {
		reportedByName[ds.Name] = ds
	}

	rows, err := s.pool.Query(ctx, `SELECT id, name, body_hash, source_path FROM skills`)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("list existing: %w", err)
	}
	defer rows.Close()

	type existingRow struct {
		ID, Name, BodyHash, SourcePath string
	}
	existing := make(map[string]existingRow)
	for rows.Next() {
		var r existingRow
		if err := rows.Scan(&r.ID, &r.Name, &r.BodyHash, &r.SourcePath); err != nil {
			return 0, 0, 0, fmt.Errorf("scan: %w", err)
		}
		existing[r.Name] = r
	}
	if err := rows.Err(); err != nil {
		return 0, 0, 0, err
	}

	for name, ds := range reportedByName {
		es, ok := existing[name]
		if !ok {
			_, err := s.pool.Exec(ctx, `
				INSERT INTO skills (name, description, source_path, source_kind, body, body_hash)
				VALUES ($1, $2, $3, $4, $5, $6)
			`, ds.Name, ds.Description, ds.SourcePath, ds.SourceKind, ds.Body, ds.BodyHash)
			if err != nil {
				slog.Warn("skill sync insert failed", "name", name, "error", err)
				continue
			}
			added++
			continue
		}
		if es.BodyHash == ds.BodyHash {
			continue // unchanged
		}
		_, err := s.pool.Exec(ctx, `
			UPDATE skills
			SET description = $1, source_path = $2, source_kind = $3,
			    body = $4, body_hash = $5, updated_at = now()
			WHERE id = $6
		`, ds.Description, ds.SourcePath, ds.SourceKind, ds.Body, ds.BodyHash, es.ID)
		if err != nil {
			slog.Warn("skill sync update failed", "name", name, "error", err)
			continue
		}
		updated++
	}

	// Delete skills that were not in the report.
	for name, es := range existing {
		if _, stillPresent := reportedByName[name]; stillPresent {
			continue
		}
		if _, err := s.pool.Exec(ctx, `DELETE FROM skills WHERE id = $1`, es.ID); err != nil {
			slog.Warn("skill sync delete failed", "name", name, "error", err)
			continue
		}
		removed++
	}

	return added, updated, removed, nil
}

type existingSkill struct {
	ID         string
	Name       string
	BodyHash   string
	SourcePath string
}

// ListAll returns summaries of every skill in the DB.
func (s *SkillService) ListAll(ctx context.Context) ([]SkillSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, description, source_path, source_kind, body_hash, discovered_at, updated_at
		FROM skills
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SkillSummary
	for rows.Next() {
		var ss SkillSummary
		if err := rows.Scan(&ss.ID, &ss.Name, &ss.Description, &ss.SourcePath,
			&ss.SourceKind, &ss.BodyHash, &ss.DiscoveredAt, &ss.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, ss)
	}
	return out, rows.Err()
}

// GetByID returns a single skill with its body and supporting files. Returns
// (nil, ErrSkillNotFound) if the skill does not exist; the handler
// translates that to 404 (other errors become 500).
func (s *SkillService) GetByID(ctx context.Context, id string) (*Skill, error) {
	var sk Skill
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, description, source_path, source_kind, body, body_hash, discovered_at, updated_at
		FROM skills WHERE id = $1
	`, id).Scan(&sk.ID, &sk.Name, &sk.Description, &sk.SourcePath, &sk.SourceKind,
		&sk.Body, &sk.BodyHash, &sk.DiscoveredAt, &sk.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSkillNotFound
		}
		return nil, err
	}
	// Files: Phase 1 rescan doesn't write to skill_files, so this is typically
	// empty, but the API exposes it for Phase 2.
	frows, err := s.pool.Query(ctx, `
		SELECT id, path, content, created_at, updated_at
		FROM skill_files WHERE skill_id = $1 ORDER BY path
	`, id)
	if err != nil {
		return nil, err
	}
	defer frows.Close()
	for frows.Next() {
		var f SkillFile
		if err := frows.Scan(&f.ID, &f.Path, &f.Content, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		sk.Files = append(sk.Files, f)
	}
	return &sk, frows.Err()
}

// ErrSkillNotFound is returned by GetByID when the skill ID does not exist.
// (Other errors from GetByID are genuine DB errors and should be 500.)
var ErrSkillNotFound = errors.New("skill not found")

// ListByAgent returns the skills currently bound to the given agent.
func (s *SkillService) ListByAgent(ctx context.Context, agentID string) ([]SkillSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT s.id, s.name, s.description, s.source_path, s.source_kind, s.body_hash, s.discovered_at, s.updated_at
		FROM skills s
		JOIN agent_skills ask ON ask.skill_id = s.id
		WHERE ask.agent_id = $1
		ORDER BY s.name
	`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SkillSummary
	for rows.Next() {
		var ss SkillSummary
		if err := rows.Scan(&ss.ID, &ss.Name, &ss.Description, &ss.SourcePath,
			&ss.SourceKind, &ss.BodyHash, &ss.DiscoveredAt, &ss.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, ss)
	}
	return out, rows.Err()
}

// SetAgentSkills replaces the agent's skill bindings in a transaction.
// skillIDs is the complete desired set (full replace, not delta). All skill
// IDs are validated in a single query (not N+1) before any insert; if any
// ID is missing, the whole transaction is rejected and the existing
// bindings are left untouched.
func (s *SkillService) SetAgentSkills(ctx context.Context, agentID string, skillIDs []string) ([]SkillSummary, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM agent_skills WHERE agent_id = $1`, agentID); err != nil {
		return nil, fmt.Errorf("clear: %w", err)
	}

	// Validate every requested skill exists in a single round-trip.
	if len(skillIDs) > 0 {
		rows, err := tx.Query(ctx, `SELECT id FROM skills WHERE id = ANY($1)`, skillIDs)
		if err != nil {
			return nil, fmt.Errorf("validate skills: %w", err)
		}
		found := make(map[string]bool, len(skillIDs))
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			found[id] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		for _, sid := range skillIDs {
			if !found[sid] {
				return nil, fmt.Errorf("%w: %s", ErrSkillNotFound, sid)
			}
		}
		for _, sid := range skillIDs {
			if _, err := tx.Exec(ctx, `INSERT INTO agent_skills (agent_id, skill_id) VALUES ($1, $2)`, agentID, sid); err != nil {
				return nil, fmt.Errorf("insert skill %s: %w", sid, err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.ListByAgent(ctx, agentID)
}

// LoadAgentSkillsForTask is the task-dispatch hook: returns name + body for
// each skill bound to the agent. Phase 1 does not wire task handlers, but the
// interface is stable so Phase 2 can plug it in.
func (s *SkillService) LoadAgentSkillsForTask(ctx context.Context, agentID string) ([]AgentSkillData, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT s.name, s.body
		FROM skills s
		JOIN agent_skills ask ON ask.skill_id = s.id
		WHERE ask.agent_id = $1
		ORDER BY s.name
	`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AgentSkillData
	for rows.Next() {
		var d AgentSkillData
		if err := rows.Scan(&d.Name, &d.Content); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// AgentSkillData is the per-agent-skill payload intended for task-time use.
type AgentSkillData struct {
	Name    string
	Content string
	Files   []AgentSkillFileData
}

// AgentSkillFileData is a supporting file inside an AgentSkillData.
type AgentSkillFileData struct {
	Path    string
	Content string
}
