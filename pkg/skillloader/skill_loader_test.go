package skillloader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFrontmatter_HiddenFromListedFalse(t *testing.T) {
	in := "---\nname: hidden\ndescription: skip me\nlisted: false\n---\n# body"
	name, desc, hidden, _, err := ParseFrontmatter(in)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if name != "hidden" || desc != "skip me" {
		t.Fatalf("got (%q,%q)", name, desc)
	}
	if !hidden {
		t.Fatal("expected hidden=true when frontmatter has `listed: false`")
	}
}

func TestParseFrontmatter_HiddenFromListedTrue(t *testing.T) {
	// `listed: true` is the default; the parser only sets hidden=true when
	// `listed: false` is explicit. For `listed: true` (or absent), hidden=false.
	in := "---\nname: visible\ndescription: show me\nlisted: true\n---\n# body"
	_, _, hidden, _, err := ParseFrontmatter(in)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if hidden {
		t.Fatal("expected hidden=false for explicit `listed: true`")
	}
}

func TestParseFrontmatter_RequiresBeta(t *testing.T) {
	in := "---\nname: experimental\ndescription: beta only\nrequiresBeta: feature-xyz\n---\n# body"
	_, _, _, beta, err := ParseFrontmatter(in)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if beta != "feature-xyz" {
		t.Fatalf("expected requiresBeta=feature-xyz, got %q", beta)
	}
}

func TestParseFrontmatter_CommentLinesIgnored(t *testing.T) {
	in := "---\n# this is a comment\nname: foo\n# another comment\ndescription: bar\n---\n# body"
	name, desc, _, _, err := ParseFrontmatter(in)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if name != "foo" || desc != "bar" {
		t.Fatalf("got (%q,%q)", name, desc)
	}
}

func TestParseFrontmatter_LinesWithoutColonSkipped(t *testing.T) {
	// Some frontmatter blocks have stray lines (e.g. indented list items) that
	// don't have a colon. Our parser should silently skip them.
	in := "---\nname: foo\n  - this is a list\ndescription: bar\n---\n# body"
	name, desc, _, _, err := ParseFrontmatter(in)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if name != "foo" || desc != "bar" {
		t.Fatalf("got (%q,%q)", name, desc)
	}
}

func TestParseFrontmatter_DuplicateKeysLastWins(t *testing.T) {
	// Last value wins (consistent with most YAML implementations).
	in := "---\nname: first\nname: second\n---\n"
	name, _, _, _, err := ParseFrontmatter(in)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if name != "second" {
		t.Fatalf("expected last-write-wins, got %q", name)
	}
}

func TestScanRoots_DeterministicTieBreak(t *testing.T) {
	// Two separate root directories each contain a skill with the same name
	// ("collide"). Same priority: the one whose root path sorts first
	// lexicographically wins.
	lowRoot := t.TempDir()
	highRoot := t.TempDir() // ensure alphabetically after lowRoot
	// (t.TempDir results are based on /tmp/TestN.../NNN suffixes; we force
	// the highRoot path to sort after lowRoot by passing the path through.)
	if highRoot < lowRoot {
		lowRoot, highRoot = highRoot, lowRoot
	}
	if err := writeSkill(t, lowRoot, "collide", "from-low-root"); err != nil {
		t.Fatal(err)
	}
	if err := writeSkill(t, highRoot, "collide", "from-high-root"); err != nil {
		t.Fatal(err)
	}
	roots := []SkillRoot{
		{Path: highRoot, Kind: "user", Priority: 50},
		{Path: lowRoot, Kind: "user", Priority: 50},
	}
	got, err := ScanRoots("/tmp", roots)
	if err != nil {
		t.Fatal(err)
	}
	ds, ok := got["collide"]
	if !ok {
		t.Fatal("expected skill 'collide' to be discovered from one of the roots")
	}
	if ds.SourcePath == "" {
		t.Fatal("discovered skill has empty SourcePath")
	}
	// The winner must come from lowRoot (its dir name sorts first when
	// priority ties).
	if !strings.HasPrefix(ds.SourcePath, lowRoot) {
		t.Fatalf("expected lowRoot (lexicographically smaller root) to win tie; got %q (lowRoot=%s highRoot=%s)",
			ds.SourcePath, lowRoot, highRoot)
	}
	if strings.Contains(ds.SourcePath, "from-high-root") {
		t.Fatalf("tied skill picked the higher-alphabetical root, want the lower one; SourcePath=%q", ds.SourcePath)
	}
}

func TestScanRoots_HigherPriorityWins(t *testing.T) {
	low := t.TempDir()
	high := t.TempDir()
	if err := writeSkill(t, low, "shared", "from-low"); err != nil {
		t.Fatal(err)
	}
	if err := writeSkill(t, high, "shared", "from-high"); err != nil {
		t.Fatal(err)
	}
	roots := []SkillRoot{
		{Path: low, Kind: "user-mavis", Priority: 25},
		{Path: high, Kind: "builtin-global", Priority: 95},
	}
	got, err := ScanRoots("/tmp", roots)
	if err != nil {
		t.Fatal(err)
	}
	if got["shared"].SourceKind != "builtin-global" {
		t.Fatalf("expected builtin-global (pri=95) to win, got kind=%s", got["shared"].SourceKind)
	}
}

func TestScanDir_SymlinkedSubdirectory(t *testing.T) {
	// Create a real directory with a SKILL.md, then symlink it under the scan root.
	realDir := t.TempDir()
	if err := writeSkill(t, realDir, "symlinked", "via symlink"); err != nil {
		t.Fatal(err)
	}
	scanRoot := t.TempDir()
	linkPath := filepath.Join(scanRoot, "linked-skill")
	if err := os.Symlink(filepath.Join(realDir, "symlinked-dir"), linkPath); err != nil {
		t.Fatal(err)
	}
	got, err := ScanDir(scanRoot, "test", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 skill via symlink, got %d", len(got))
	}
	if got[0].Name != "symlinked" {
		t.Fatalf("expected name 'symlinked', got %q", got[0].Name)
	}
}

// writeSkill is a tiny helper to make a temp dir containing a valid SKILL.md.
func writeSkill(t *testing.T, dir, name, desc string) error {
	t.Helper()
	dirName := name + "-dir"
	if err := os.MkdirAll(filepath.Join(dir, dirName), 0o755); err != nil {
		return err
	}
	body := "---\nname: " + name + "\ndescription: " + desc + "\n---\n# body\n"
	return os.WriteFile(filepath.Join(dir, dirName, "SKILL.md"), []byte(body), 0o644)
}
