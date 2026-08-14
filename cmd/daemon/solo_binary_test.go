package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSoloBinaryUsesConfiguredExecutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "solo")
	if err := os.WriteFile(path, []byte("test"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SOLO_CLI_BIN", path)

	if got := resolveSoloBinary(); got != path {
		t.Fatalf("resolveSoloBinary() = %q, want %q", got, path)
	}
}

func TestResolveSoloBinaryPrefersBuildCompanionOverInstalledPATH(t *testing.T) {
	root := t.TempDir()
	companion := filepath.Join(root, ".pids", "solo")
	if err := os.MkdirAll(filepath.Dir(companion), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(companion, []byte("current"), 0o755); err != nil {
		t.Fatal(err)
	}
	installedDir := filepath.Join(root, "installed")
	if err := os.MkdirAll(installedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installedDir, "solo"), []byte("stale"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	t.Setenv("PATH", installedDir)
	t.Setenv("SOLO_CLI_BIN", "")

	if got := resolveSoloBinary(); got != companion {
		t.Fatalf("resolveSoloBinary() = %q, want build companion %q", got, companion)
	}
}
