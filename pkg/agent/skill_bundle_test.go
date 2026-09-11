package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/solo-ai/solo/pkg/skillloader"
)

func TestPrepareRevisionSkills(t *testing.T) {
	workspace := t.TempDir()
	bundle := func(value string) []skillloader.Bundle {
		return []skillloader.Bundle{{Name: "calculation", Files: []skillloader.BundleFile{{Path: "SKILL.md", Content: []byte("---\nname: calculation\ndescription: Verify arithmetic\n---\nRun scripts/check.sh")}, {Path: "scripts/check.sh", Content: []byte("#!/bin/sh\nprintf '%s\\n' '" + value + "'\n"), Executable: true}, {Path: "resources/input.bin", Content: []byte{0, 1, 255}}}}}
	}
	first, err := PrepareRevisionSkills(workspace, "claude", bundle("42"))
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(workspace, ".claude/skills/calculation/scripts/check.sh")
	check := func(expected string) {
		t.Helper()
		output, err := exec.Command(script).Output()
		if err != nil || string(output) != expected+"\n" {
			t.Fatalf("real Skill script: %q %v", output, err)
		}
	}
	check("42")
	skills, err := skillloader.ScanDir(filepath.Join(workspace, ".claude/skills"), "claude", 1)
	if err != nil || len(skills) != 1 || skills[0].Name != "calculation" {
		t.Fatalf("discovery failed: %v %+v", err, skills)
	}
	bytes, err := os.ReadFile(filepath.Join(workspace, ".agents/skills/calculation/resources/input.bin"))
	if err != nil || len(bytes) != 3 || bytes[2] != 255 {
		t.Fatal("binary resource changed")
	}
	second, err := PrepareRevisionSkills(workspace, "claude", bundle("43"))
	if err != nil || second == first {
		t.Fatalf("new revision failed: %v", err)
	}
	check("43")
	restored, err := PrepareRevisionSkills(workspace, "claude", bundle("42"))
	if err != nil || restored != first {
		t.Fatalf("rollback failed: %v", err)
	}
	check("42")
	if _, err := PrepareRevisionSkills(workspace, "codex", bundle("42")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(workspace, ".claude/skills/calculation")); !os.IsNotExist(err) {
		t.Fatal("old provider discovery left behind")
	}
	if _, err := os.ReadFile(filepath.Join(workspace, ".codex/skills/calculation/SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareRevisionSkills(workspace, "openclaw", bundle("42")); err != nil {
		t.Fatal(err)
	}
	script = filepath.Join(workspace, "skills/calculation/scripts/check.sh")
	check("42")
	if _, err := os.Lstat(filepath.Join(workspace, ".codex/skills/calculation")); !os.IsNotExist(err) {
		t.Fatal("stale codex link after OpenClaw switch")
	}
	if _, err := PrepareRevisionSkills(workspace, "codex", bundle("42")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(workspace, "skills/calculation")); !os.IsNotExist(err) {
		t.Fatal("stale OpenClaw link after switch")
	}
	unmanaged := filepath.Join(workspace, ".agents/skills/local-only")
	if err := os.MkdirAll(unmanaged, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareRevisionSkills(workspace, "codex", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unmanaged); err != nil {
		t.Fatal("unmanaged skill removed")
	}
	if _, err := os.Lstat(filepath.Join(workspace, ".codex/skills/calculation")); !os.IsNotExist(err) {
		t.Fatal("versioned skill not removed")
	}
	// A local directory collision must fail before changing local files.
	collision := filepath.Join(workspace, ".claude/skills/calculation")
	if err := os.MkdirAll(collision, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareRevisionSkills(workspace, "claude", bundle("42")); err == nil {
		t.Fatal("unmanaged collision overwritten")
	}
	if info, err := os.Lstat(collision); err != nil || !info.IsDir() {
		t.Fatal("local directory lost")
	}
	// An attacker-controlled parent link may not escape the workspace root.
	outside := t.TempDir()
	attacked := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(attacked, ".solo")); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareRevisionSkills(attacked, "claude", bundle("42")); err == nil {
		t.Fatal("outside parent symlink accepted")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatal("wrote beyond root")
	}
}
