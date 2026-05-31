package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/google/uuid"
)

// MachineLock prevents duplicate Daemon processes on the same machine by
// writing a lock file to a well-known path and checking the PID of any
// existing lock holder.
//
// Lock file path: ~/.solo/daemon/lock.json
//
// On startup, AcquireLock reads the lock file. If the file exists and the
// PID in it is still alive (using os.FindProcess + Signal(0)), the daemon
// refuses to start. Otherwise it writes its own lock and proceeds.
//
// On shutdown, Release removes the lock file.
type MachineLock struct {
	PID       int    `json:"pid"`
	Token     string `json:"token"`
	Hostname  string `json:"hostname"`
	StartedAt string `json:"started_at"`
	ServerURL string `json:"server_url,omitempty"`
}

// defaultLockDir returns the default lock directory (~/.solo/daemon).
func defaultLockDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".solo", "daemon")
}

// AcquireLock attempts to acquire the daemon machine lock. If an existing
// lock file is present and the PID in it is still alive, it returns an error
// indicating a duplicate daemon. If the PID is dead (stale lock), it removes
// the old lock and acquires a fresh one.
//
// lockDir is the directory for the lock file. If empty, defaults to
// $HOME/.solo/daemon.
//
// serverURL is recorded in the lock for diagnostic purposes.
func AcquireLock(lockDir string, serverURL string) (*MachineLock, error) {
	if lockDir == "" {
		lockDir = defaultLockDir()
	}

	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return nil, fmt.Errorf("lock: create lock directory %s: %w", lockDir, err)
	}

	lockPath := filepath.Join(lockDir, "lock.json")

	// Check for existing lock.
	if existing, err := readLockFile(lockPath); err == nil {
		if isProcessAlive(existing.PID) {
			return nil, fmt.Errorf("lock: daemon already running on this machine (PID %d, started at %s, hostname %q)",
				existing.PID, existing.StartedAt, existing.Hostname)
		}
		// Stale lock — remove it.
		_ = os.Remove(lockPath)
	}

	// Acquire new lock.
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = fmt.Sprintf("unknown-%s", runtime.GOOS)
	}

	l := &MachineLock{
		PID:       os.Getpid(),
		Token:     uuid.New().String(),
		Hostname:  hostname,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		ServerURL: serverURL,
	}

	if err := writeLockFile(lockPath, l); err != nil {
		return nil, fmt.Errorf("lock: write lock file: %w", err)
	}

	return l, nil
}

// Release removes the lock file. It is safe to call multiple times or
// when no lock file exists. If the lock file was written by a different
// PID (e.g. after fork recovery), Release will not remove it.
func (l *MachineLock) Release() error {
	lockDir := defaultLockDir()
	lockPath := filepath.Join(lockDir, "lock.json")

	// Only remove if the lock still belongs to us.
	existing, err := readLockFile(lockPath)
	if err != nil {
		// No lock file or unreadable — nothing to do.
		return nil
	}

	if existing.PID != l.PID || existing.Token != l.Token {
		// Lock belongs to another process — do not touch it.
		return nil
	}

	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("lock: remove lock file: %w", err)
	}

	return nil
}

// readLockFile reads and parses a lock file at the given path.
func readLockFile(path string) (*MachineLock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var l MachineLock
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("parse lock file: %w", err)
	}

	if l.PID == 0 {
		return nil, fmt.Errorf("invalid lock file: missing pid")
	}

	return &l, nil
}

// writeLockFile writes the machine lock to the given path atomically
// (write to temp file, then rename).
func writeLockFile(path string, l *MachineLock) error {
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal lock: %w", err)
	}

	dir := filepath.Dir(path)
	tmpPath := filepath.Join(dir, ".lock.json.tmp")

	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write temp lock: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename lock file: %w", err)
	}

	return nil
}

// isProcessAlive checks whether a process with the given PID is still
// running. It uses os.FindProcess + Signal(0) as a probe — signal 0 is
// defined by POSIX as a "test for existence" that does not actually send
// a signal. On Windows, FindProcess always succeeds and Signal(0) behavior
// is platform-dependent, so this is a best-effort check.
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Signal(0) — test for process existence without sending a signal.
	return proc.Signal(syscall.Signal(0)) == nil
}

// CurrentUser returns a human-readable identifier for the current OS user.
// Used for diagnostic logging in the daemon startup sequence.
func CurrentUser() string {
	u, err := user.Current()
	if err != nil {
		return "unknown"
	}
	return u.Username
}
