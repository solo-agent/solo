package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestResolveInternalToken(t *testing.T) {
	t.Run("uses dedicated internal token", func(t *testing.T) {
		if got := resolveInternalToken("internal-secret", "jwt-secret"); got != "internal-secret" {
			t.Fatalf("resolveInternalToken() = %q, want internal-secret", got)
		}
	})

	t.Run("falls back to jwt secret for backward compatibility", func(t *testing.T) {
		if got := resolveInternalToken("", "jwt-secret"); got != "jwt-secret" {
			t.Fatalf("resolveInternalToken() = %q, want jwt-secret", got)
		}
	})
}

func TestDaemonProcessRecordOnlyRemovesItsOwnPID(t *testing.T) {
	dir := t.TempDir()
	path, err := writeDaemonProcessRecord(dir, os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid()+1)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeDaemonProcessRecord(path, os.Getpid()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("another process record was removed: %v", err)
	}
	if _, err := writeDaemonProcessRecord(dir, os.Getpid()); err != nil {
		t.Fatal(err)
	}
	if err := removeDaemonProcessRecord(path, os.Getpid()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "daemon.pid")); !os.IsNotExist(err) {
		t.Fatalf("own process record still exists: %v", err)
	}
}
