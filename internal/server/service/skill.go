package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/server/skillloader"
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

// RescanResult is what POST /api/v1/skills/rescan returns.
type RescanResult struct {
	OK      bool   `json:"ok"`
	Added   int    `json:"added"`
	Updated int    `json:"updated"`
	Removed int    `json:"removed"`
	Total   int    `json:"total"`
	Error   string `json:"error,omitempty"`
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

// SkillService is the entry point for skill-related operations.
type SkillService struct {
	pool *pgxpool.Pool
}

// NewSkillService creates a new SkillService.
func NewSkillService(pool *pgxpool.Pool) *SkillService {
	return &SkillService{pool: pool}
}

// StartBackgroundRescan runs a full disk rescan on startup, with a hard
// 10s deadline. Failures are logged and swallowed — the server must not
// refuse to start because rescan failed.
func (s *SkillService) StartBackgroundRescan(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res, err := s.Rescan(ctx)
	if err != nil {
		slog.Warn("background skill rescan failed", "error", err)
		return
	}
	slog.Info("background rescan complete",
		"added", res.Added, "updated", res.Updated, "removed", res.Removed, "total", res.Total)
}

// Rescan reconciles the on-disk skills with the DB. It returns counts of
// added/updated/removed/total skills, or an error if the scan itself failed.
func (s *SkillService) Rescan(ctx context.Context) (RescanResult, error) {
	discovered, err := s.scanRoots(ctx)
	if err != nil {
		return RescanResult{OK: false, Error: err.Error()}, err
	}

	// Fetch all existing skills in one query.
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, body_hash, source_path
		FROM skills
	`)
	if err != nil {
		return RescanResult{OK: false, Error: err.Error()}, fmt.Errorf("list existing: %w", err)
	}
	defer rows.Close()

	existing := make(map[string]existingSkill)
	for rows.Next() {
		var es existingSkill
		if err := rows.Scan(&es.ID, &es.Name, &es.BodyHash, &es.SourcePath); err != nil {
			return RescanResult{OK: false, Error: err.Error()}, fmt.Errorf("scan: %w", err)
		}
		existing[es.Name] = es
	}
	if err := rows.Err(); err != nil {
		return RescanResult{OK: false, Error: err.Error()}, err
	}

	var res RescanResult
	seen := make(map[string]bool, len(discovered))

	for name, ds := range discovered {
		seen[name] = true
		es, ok := existing[name]
		if !ok {
			// INSERT
			_, err := s.pool.Exec(ctx, `
				INSERT INTO skills (name, description, source_path, source_kind, body, body_hash)
				VALUES ($1, $2, $3, $4, $5, $6)
			`, ds.Name, ds.Description, ds.SourcePath, ds.SourceKind, ds.Body, ds.BodyHash)
			if err != nil {
				slog.Warn("rescan insert failed", "name", ds.Name, "error", err)
				continue
			}
			res.Added++
			continue
		}
		if es.BodyHash == ds.BodyHash {
			continue
		}
		// UPDATE
		_, err := s.pool.Exec(ctx, `
			UPDATE skills
			SET description = $1, source_path = $2, source_kind = $3,
			    body = $4, body_hash = $5, updated_at = now()
			WHERE id = $6
		`, ds.Description, ds.SourcePath, ds.SourceKind, ds.Body, ds.BodyHash, es.ID)
		if err != nil {
			slog.Warn("rescan update failed", "name", ds.Name, "error", err)
			continue
		}
		res.Updated++
	}

	// DELETE any existing skill whose source_path no longer exists on disk.
	// (We don't use the "seen" set to allow DB-only skills in Phase 2.)
	for name, es := range existing {
		if seen[name] {
			continue
		}
		if !fileExists(es.SourcePath) {
			if _, err := s.pool.Exec(ctx, `DELETE FROM skills WHERE id = $1`, es.ID); err != nil {
				slog.Warn("rescan delete failed", "name", name, "error", err)
				continue
			}
			res.Removed++
		}
	}

	// Total = current row count.
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM skills`).Scan(&res.Total); err != nil {
		return RescanResult{OK: false, Error: err.Error()}, err
	}
	res.OK = true
	return res, nil
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

// scanRoots builds the priority table from env vars and runs ScanRoots.
func (s *SkillService) scanRoots(_ context.Context) (map[string]skillloader.DiscoveredSkill, error) {
	dataDir := resolveDataDir()
	roots := buildRoots(dataDir)
	return skillloader.ScanRoots(dataDir, roots)
}

// resolveDataDir returns SOLO_DATA_DIR if set, else ~/.mavis.
func resolveDataDir() string {
	if d := os.Getenv("SOLO_DATA_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".mavis"
	}
	return filepath.Join(home, ".mavis")
}

// buildRoots returns the priority table for skill root discovery.
// Scans each agent backend's native global skill directory so solo
// sees what the user has already installed for their CLI agents.
func buildRoots(dataDir string) []skillloader.SkillRoot {
	home, err := os.UserHomeDir()
	if err != nil {
		return []skillloader.SkillRoot{
			{Path: filepath.Join(dataDir, "skills"), Kind: "mavis", Priority: 25},
		}
	}

	var roots []skillloader.SkillRoot

	// Agent builtin-skills per agent (highest priority).
	agentsDir := filepath.Join(dataDir, "agents")
	if entries, err := os.ReadDir(agentsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			roots = append(roots, skillloader.SkillRoot{
				Path:     filepath.Join(agentsDir, e.Name(), ".builtin-skills"),
				Kind:     "builtin-agent",
				Priority: 100,
			})
		}
	}

	// Agent-native global skill directories.
	roots = append(roots,
		skillloader.SkillRoot{Path: filepath.Join(home, ".claude", "skills"), Kind: "claude", Priority: 60},
		skillloader.SkillRoot{Path: filepath.Join(home, ".codex", "skills"), Kind: "codex", Priority: 35},
		skillloader.SkillRoot{Path: filepath.Join(dataDir, "skills"), Kind: "mavis", Priority: 25},
	)

	return roots
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
