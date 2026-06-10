// Package skillloader scans the local filesystem for Claude-style skill directories
// and parses their SKILL.md frontmatter. It is a leaf package (no imports of
// internal/server/handler or internal/server/service) so both the service layer
// and the handler layer can depend on it without creating an import cycle.
//
// A skill directory is any directory containing a file named SKILL.md whose
// frontmatter is a YAML block delimited by "---" lines, with required keys
// "name" and "description" and optional key "requiresBeta".
// The "listed" key, when set to "false", sets the hidden return flag.
package skillloader

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DiscoveredSkill is the result of scanning a single skill directory.
type DiscoveredSkill struct {
	Name        string // from frontmatter, falls back to dir name
	Description string // from frontmatter
	SourcePath  string // absolute path to the SKILL.md file
	SourceKind  string // e.g. claude, codex, opencode, hermes, pi, ws-claude, ws-codex
	Body        string // full SKILL.md content
	BodyHash    string // sha256 hex of Body (64 chars)
	Priority    int    // higher wins on name collision
}

// ErrInvalidFrontmatter is returned by ParseFrontmatter when the file lacks a
// usable frontmatter block. The caller should log + skip (not propagate).
var ErrInvalidFrontmatter = errors.New("invalid SKILL.md frontmatter")

// ParseFrontmatter extracts name/description/hidden/requiresBeta from a
// SKILL.md's leading "---\n...\n---\n" block. hidden is true when the
// frontmatter has `listed: false` (the skill should not appear in catalogs).
// Returns ErrInvalidFrontmatter if there is no valid frontmatter block.
//
// Only the 4 keys from the skill-system design doc are recognised; unknown
// keys are silently ignored.
func ParseFrontmatter(content string) (name, description string, hidden bool, requiresBeta string, err error) {
	const sep = "\n---\n"
	// Frontmatter must start with "---\n" and end before the second "---\n".
	if !strings.HasPrefix(content, "---\n") {
		return "", "", false, "", ErrInvalidFrontmatter
	}
	rest := content[len("---\n"):]
	end := strings.Index(rest, sep)
	if end < 0 {
		return "", "", false, "", ErrInvalidFrontmatter
	}
	block := rest[:end]
	// Tolerate Windows line endings by trimming trailing \r.
	block = strings.TrimRight(block, "\r")

	// Minimal key: value parser — handles flat scalar values only.
	// No nested structures, no lists. Sufficient for the 4 declared keys.
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		colon := strings.Index(line, ":")
		if colon < 0 {
			continue
		}
		key := strings.TrimSpace(line[:colon])
		val := strings.TrimSpace(line[colon+1:])
		// Strip surrounding quotes.
		if len(val) >= 2 && (val[0] == '"' && val[len(val)-1] == '"') {
			val = val[1 : len(val)-1]
		}
		switch key {
		case "name":
			name = val
		case "description":
			description = val
		case "listed":
			hidden = val == "false"
		case "requiresBeta":
			requiresBeta = val
		}
	}
	if name == "" {
		return "", "", false, "", ErrInvalidFrontmatter
	}
	return name, description, hidden, requiresBeta, nil
}

// ScanDir reads the immediate subdirectories of root and returns a DiscoveredSkill
// for each one that contains a valid SKILL.md. Subdirectories whose SKILL.md is
// missing or invalid are silently skipped. The returned slice is sorted by
// directory name for deterministic output.
func ScanDir(root, sourceKind string, priority int) ([]DiscoveredSkill, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", root, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	var out []DiscoveredSkill
	for _, e := range entries {
		name := e.Name()
		if isIgnoredDir(name) {
			continue
		}
		// Resolve symlinks: `e.IsDir()` is false for symlinked directories,
		// so we follow the link via os.Stat to determine the real type.
		entryPath := filepath.Join(root, name)
		if e.Type()&os.ModeSymlink != 0 {
			info, statErr := os.Stat(entryPath)
			if statErr != nil || !info.IsDir() {
				continue
			}
		} else if !e.IsDir() {
			continue
		}
		skillPath := filepath.Join(entryPath, "SKILL.md")
		f, err := os.Open(skillPath)
		if err != nil {
			continue
		}
		body, readErr := io.ReadAll(f)
		_ = f.Close()
		if readErr != nil {
			continue
		}
		name, desc, hidden, _, parseErr := ParseFrontmatter(string(body))
		if parseErr != nil {
			continue
		}
		if hidden {
			continue
		}
		sum := sha256.Sum256(body)
		out = append(out, DiscoveredSkill{
			Name:        name,
			Description: desc,
			SourcePath:  skillPath,
			SourceKind:  sourceKind,
			Body:        string(body),
			BodyHash:    hex.EncodeToString(sum[:]),
			Priority:    priority,
		})
	}
	return out, nil
}

// ScanRoots scans the given roots and returns a name->DiscoveredSkill map.
// Same-name skills are resolved by keeping the highest-priority occurrence
// (ties broken by source path lexicographic order).
//
// The dataDir parameter is currently unused but kept in the signature for
// future expansion (e.g. logging root counts per source).
//
// Returned map is keyed by skill name.
func ScanRoots(dataDir string, roots []SkillRoot) (map[string]DiscoveredSkill, error) {
	_ = dataDir
	// Sort roots by descending priority (then source path for tie-breaking),
	// then keep the FIRST occurrence per name — equivalent to "keep highest
	// priority, ties by source path".
	sort.SliceStable(roots, func(i, j int) bool {
		if roots[i].Priority != roots[j].Priority {
			return roots[i].Priority > roots[j].Priority
		}
		return roots[i].Path < roots[j].Path
	})

	out := make(map[string]DiscoveredSkill)
	for _, r := range roots {
		found, err := ScanDir(r.Path, r.Kind, r.Priority)
		if err != nil {
			return nil, err
		}
		for _, ds := range found {
			if _, exists := out[ds.Name]; exists {
				continue
			}
			out[ds.Name] = ds
		}
	}
	return out, nil
}

// SkillRoot describes one filesystem root to scan.
type SkillRoot struct {
	Path     string
	Kind     string
	Priority int
}

func isIgnoredDir(name string) bool {
	return strings.HasPrefix(name, ".")
}
