package agent

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// persistentRunner manages the shared subprocess lifecycle for persistent
// agent backends (hermes, openclaw, opencode, codex). Each backend wraps this
// with protocol-specific message formatting and output parsing.
//
// This is the shared infrastructure that Claude will migrate onto — it
// mirrors ClaudeBackend.Start's env and process setup exactly.
type persistentRunner struct {
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	stderrTail *stderrTail
	cancel     context.CancelFunc
	done       chan struct{}
	logger     *slog.Logger
	waitOnce   sync.Once
	doneOnce   sync.Once
	exited     atomic.Bool
}

// isAlive returns true if the subprocess is still running.
func (r *persistentRunner) isAlive() bool {
	return !r.exited.Load()
}

// finish reaps the subprocess and publishes its terminal state. Protocol
// reader goroutines must defer this instead of closing done directly so
// IsAlive cannot remain true after stdout reaches EOF.
func (r *persistentRunner) finish() {
	r.waitOnce.Do(func() {
		_ = r.cmd.Wait()
		r.exited.Store(true)
	})
	r.doneOnce.Do(func() { close(r.done) })
}

// write writes data to the subprocess stdin. Each call is serialized by the
// caller (turn acquisition in AgentSessionManager).
func (r *persistentRunner) write(data []byte) error {
	_, err := r.stdin.Write(data)
	return err
}

// close terminates the persistent subprocess. It closes stdin to signal
// graceful exit, waits up to 10 seconds, then kills if still running.
func (r *persistentRunner) close() error {
	r.logger.Info("persistent: closing session")
	_ = r.stdin.Close()

	select {
	case <-r.done:
	case <-time.After(10 * time.Second):
		r.logger.Warn("persistent: session did not exit, killing")
		r.cancel()
		_ = r.cmd.Process.Kill()
		<-r.done
	}

	return nil
}

// forceClose immediately kills the persistent subprocess without waiting
// for graceful exit. Used by hard cleanup paths (agent deletion) where
// in-flight turns can be discarded.
func (r *persistentRunner) forceClose() error {
	r.logger.Warn("persistent: force-closing session")
	r.cancel()
	if r.cmd != nil && r.cmd.Process != nil {
		_ = r.cmd.Process.Kill()
	}
	select {
	case <-r.done:
	case <-time.After(2 * time.Second):
		r.logger.Error("persistent: force-kill did not reap process within 2s")
	}
	return nil
}

// startPersistent creates a new persistent subprocess for the given CLI.
// It uses buildEnvAt to inject the workspace directory into PATH, aligning
// with ClaudeBackend.Start's behaviour. extraEnv carries backend-specific
// variables merged on top of the process environment.
func startPersistent(
	ctx context.Context,
	execPath string,
	args []string,
	dir string,
	extraEnv map[string]string,
	logger *slog.Logger,
) (*persistentRunner, error) {
	if logger == nil {
		logger = slog.Default()
	}

	// A persistent subprocess belongs to the pooled Agent session, not to the
	// task that happened to create it. Task cancellation is handled at the turn
	// protocol layer; only Close/ForceClose should terminate this process.
	runCtx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(runCtx, execPath, args...)
	cmd.WaitDelay = 10 * time.Second
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = buildEnvAt(dir, extraEnv)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("persistent: stdout pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("persistent: stdin pipe: %w", err)
	}
	stderrTail := newStderrTail(newLogWriter(logger, "[persistent] "), 64*1024)
	cmd.Stderr = stderrTail

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		cancel()
		return nil, fmt.Errorf("persistent: start: %w", err)
	}

	logger.Info("persistent: session started", "pid", cmd.Process.Pid, "exec", execPath, "cwd", cmd.Dir)

	return &persistentRunner{
		cmd:        cmd,
		stdin:      stdin,
		stdout:     stdout,
		stderrTail: stderrTail,
		cancel:     cancel,
		done:       make(chan struct{}),
		logger:     logger,
	}, nil
}
