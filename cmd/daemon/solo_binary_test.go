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
