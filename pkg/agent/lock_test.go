package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcquireLock(t *testing.T) {
	lockDir := t.TempDir()
	lockPath := filepath.Join(lockDir, "lock.json")

	l, err := AcquireLock(lockDir, "http://solo-server:8080")
	if err != nil {
		t.Fatalf("AcquireLock failed: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil lock")
	}

	// Verify lock fields are populated.
	if l.PID == 0 {
		t.Error("expected PID to be set")
	}
	if l.Token == "" {
		t.Error("expected Token to be non-empty")
	}
	if l.Hostname == "" {
		t.Error("expected Hostname to be non-empty")
	}
	if l.StartedAt == "" {
		t.Error("expected StartedAt to be non-empty")
	}
	if l.ServerURL != "http://solo-server:8080" {
		t.Errorf("expected ServerURL to be %q, got %q", "http://solo-server:8080", l.ServerURL)
	}

	// Verify lock file was written.
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Fatal("lock file was not created")
	}

	// Read back and verify contents.
	readBack, err := readLockFile(lockPath)
	if err != nil {
		t.Fatalf("readLockFile failed: %v", err)
	}
	if readBack.PID != l.PID {
		t.Errorf("expected PID %d, got %d", l.PID, readBack.PID)
	}
	if readBack.Token != l.Token {
		t.Errorf("expected Token %s, got %s", l.Token, readBack.Token)
	}
}

func TestAcquireLock_DefaultDir(t *testing.T) {
	// Using empty lockDir should default to $HOME/.solo/daemon.
	// Provide a non-existent path to avoid writing to real home.
	t.Setenv("HOME", t.TempDir())

	l, err := AcquireLock("", "")
	if err != nil {
		t.Fatalf("AcquireLock with empty dir failed: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil lock")
	}
}

func TestAcquireLock_Duplicate(t *testing.T) {
	lockDir := t.TempDir()

	// First acquire — should succeed.
	l1, err := AcquireLock(lockDir, "http://test")
	if err != nil {
		t.Fatalf("first AcquireLock failed: %v", err)
	}
	if l1 == nil {
		t.Fatal("expected non-nil lock")
	}

	// Second acquire — should fail because PID is still alive (it's us).
	_, err = AcquireLock(lockDir, "http://test")
	if err == nil {
		t.Fatal("expected error on duplicate AcquireLock")
	}
	if !strings.Contains(err.Error(), "daemon already running") {
		t.Errorf("expected 'daemon already running' in error, got: %v", err)
	}
}

func TestRelease(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// AcquireLock with empty dir uses defaultLockDir which reads HOME.
	l, err := AcquireLock("", "")
	if err != nil {
		t.Fatalf("AcquireLock failed: %v", err)
	}

	lockPath := filepath.Join(homeDir, ".solo", "daemon", "lock.json")

	// Verify lock file exists before release.
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Fatal("lock file should exist before Release")
	}

	// Release.
	if err := l.Release(); err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	// Verify lock file is removed.
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("lock file should be removed after Release")
	}
}

func TestRelease_Idempotent(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	l, err := AcquireLock("", "")
	if err != nil {
		t.Fatalf("AcquireLock failed: %v", err)
	}

	// First release.
	if err := l.Release(); err != nil {
		t.Fatalf("first Release failed: %v", err)
	}

	// Second release — should not error.
	if err := l.Release(); err != nil {
		t.Fatalf("second Release should not fail: %v", err)
	}
}

func TestRelease_WrongToken(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// AcquireLock uses defaultLockDir which reads HOME.
	l, err := AcquireLock("", "")
	if err != nil {
		t.Fatalf("AcquireLock failed: %v", err)
	}

	lockPath := filepath.Join(homeDir, ".solo", "daemon", "lock.json")

	// Manually write a lock file with the same PID but a different token.
	wrongLock := &MachineLock{
		PID:   l.PID,
		Token: "wrong-token-for-test",
	}
	if err := writeLockFile(lockPath, wrongLock); err != nil {
		t.Fatalf("writeLockFile (wrong) failed: %v", err)
	}

	// Release — should not remove the file because token doesn't match.
	if err := l.Release(); err != nil {
		t.Fatalf("Release with wrong token should not error: %v", err)
	}

	// Lock file should still exist.
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Error("lock file should still exist after Release with mismatched token")
	}

	// Read it back to confirm our wrong lock is still there.
	existing, err := readLockFile(lockPath)
	if err != nil {
		t.Fatalf("readLockFile failed: %v", err)
	}
	if existing.Token != "wrong-token-for-test" {
		t.Errorf("expected token %q, got %q", "wrong-token-for-test", existing.Token)
	}
}

func TestRelease_WrongPID(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// AcquireLock uses defaultLockDir which reads HOME.
	l, err := AcquireLock("", "")
	if err != nil {
		t.Fatalf("AcquireLock failed: %v", err)
	}

	lockPath := filepath.Join(homeDir, ".solo", "daemon", "lock.json")

	// Manually write a lock file with a different PID.
	wrongLock := &MachineLock{
		PID:   9999999, // unlikely to be a real PID
		Token: l.Token,
	}
	if err := writeLockFile(lockPath, wrongLock); err != nil {
		t.Fatalf("writeLockFile (wrong) failed: %v", err)
	}

	// Release — should not remove because PID doesn't match.
	if err := l.Release(); err != nil {
		t.Fatalf("Release with wrong PID should not error: %v", err)
	}

	// Lock file should still exist.
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Error("lock file should still exist after Release with mismatched PID")
	}
}

func TestRelease_NoLockFile(t *testing.T) {
	l := &MachineLock{PID: 1234, Token: "test-token"}
	// Override defaultLockDir for this test... but since Release uses defaultLockDir(),
	// we can't easily redirect it. Instead, call Release when no lock file exists.
	// Since defaultLockDir is at ~/.solo/daemon, there won't be a lock file in tests.
	err := l.Release()
	if err != nil {
		t.Fatalf("Release with no lock file should not error: %v", err)
	}
}

func TestAcquireLock_StaleLock(t *testing.T) {
	lockDir := t.TempDir()
	lockPath := filepath.Join(lockDir, "lock.json")

	// Write a lock file with a PID that does not exist.
	staleLock := &MachineLock{
		PID:   9999999,
		Token: "stale-token",
	}
	if err := writeLockFile(lockPath, staleLock); err != nil {
		t.Fatalf("writeLockFile (stale) failed: %v", err)
	}

	// Acquire should succeed because stale PID is not alive.
	l, err := AcquireLock(lockDir, "")
	if err != nil {
		t.Fatalf("AcquireLock over stale lock failed: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil lock")
	}

	// Old lock file should have been replaced.
	if l.Token == "stale-token" {
		t.Error("lock token should be different from stale lock")
	}
}

func TestReadLockFile_Invalid(t *testing.T) {
	lockDir := t.TempDir()
	lockPath := filepath.Join(lockDir, "lock.json")

	// Write invalid JSON.
	_ = os.WriteFile(lockPath, []byte("not-json"), 0o644)

	_, err := readLockFile(lockPath)
	if err == nil {
		t.Fatal("expected error for invalid lock file")
	}
}

func TestReadLockFile_MissingPID(t *testing.T) {
	lockDir := t.TempDir()
	lockPath := filepath.Join(lockDir, "lock.json")

	// Write JSON with PID=0 (which readLockFile treats as missing).
	_ = os.WriteFile(lockPath, []byte(`{"pid": 0, "token": "abc"}`), 0o644)

	_, err := readLockFile(lockPath)
	if err == nil {
		t.Fatal("expected error for lock file with PID 0")
	}
	if !strings.Contains(err.Error(), "missing pid") {
		t.Errorf("expected 'missing pid' in error, got: %v", err)
	}
}
