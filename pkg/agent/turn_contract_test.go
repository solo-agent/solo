package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestACPTurnControllerSerializesAndTargetsActiveTurn(t *testing.T) {
	var turns acpTurnController
	first, err := turns.begin("model-1")
	if err != nil {
		t.Fatalf("begin first turn: %v", err)
	}
	if _, err := turns.begin("model-2"); err == nil || !strings.Contains(err.Error(), "already in progress") {
		t.Fatalf("overlapping begin error = %v, want already in progress", err)
	}

	turns.emit(OutputChunk{Type: string(MessageText), Content: "first output"})
	turns.recordPromptDone(acpPromptResult{usage: TokenUsage{InputTokens: 7, OutputTokens: 3}})
	turns.recordPromptDone(acpPromptResult{}) // duplicate terminal without usage
	if !turns.finish(first, "completed", "") {
		t.Fatal("finish first turn = false, want true")
	}
	if turns.finish(first, "completed", "") {
		t.Fatal("duplicate finish = true, want false")
	}

	firstResult := readTurnResult(t, first.resCh)
	if firstResult.Status != "completed" || firstResult.Output != "first output" {
		t.Fatalf("first result = %+v", firstResult)
	}
	if got := firstResult.Usage["model-1"]; got.InputTokens != 7 || got.OutputTokens != 3 {
		t.Fatalf("first usage = %+v", got)
	}

	// Late callbacks after terminal completion must be ignored rather than sent
	// to the first turn's already-closed channels.
	turns.emit(OutputChunk{Type: string(MessageText), Content: "late"})

	second, err := turns.begin("model-2")
	if err != nil {
		t.Fatalf("begin second turn: %v", err)
	}
	turns.emit(OutputChunk{Type: string(MessageText), Content: "second output"})
	if !turns.failActive("provider process exited unexpectedly") {
		t.Fatal("fail active turn = false, want true")
	}
	secondResult := readTurnResult(t, second.resCh)
	if secondResult.Status != "failed" || secondResult.Output != "second output" ||
		!strings.Contains(secondResult.Error, "process exited") {
		t.Fatalf("second result = %+v", secondResult)
	}
}

func TestClaudeTurnControllerRejectsOverlapAndFinishesOnce(t *testing.T) {
	state := &claudePersistentState{}
	first := newClaudeTurn()
	if err := state.beginTurn(first); err != nil {
		t.Fatalf("begin first turn: %v", err)
	}
	if err := state.beginTurn(newClaudeTurn()); err == nil || !strings.Contains(err.Error(), "already in progress") {
		t.Fatalf("overlapping begin error = %v, want already in progress", err)
	}

	if !state.finishTurn(first, &Result{Status: "completed", Output: "done"}) {
		t.Fatal("finish first turn = false, want true")
	}
	if state.finishTurn(first, &Result{Status: "failed"}) {
		t.Fatal("duplicate finish = true, want false")
	}
	result := readTurnResult(t, first.resCh)
	if result.Status != "completed" || result.Output != "done" {
		t.Fatalf("result = %+v", result)
	}

	if err := state.beginTurn(newClaudeTurn()); err != nil {
		t.Fatalf("begin after terminal result: %v", err)
	}
}

func TestClaudePersistentProviderTurnContract(t *testing.T) {
	tempDir := t.TempDir()
	fake := filepath.Join(tempDir, "claude")
	firstStarted := filepath.Join(tempDir, "first-started")
	releaseFirst := filepath.Join(tempDir, "release-first")
	secondStarted := filepath.Join(tempDir, "second-started")
	releaseSecond := filepath.Join(tempDir, "release-second")

	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "--version" ]; then
  printf '2.0.0\n'
  exit 0
fi
turn=0
while IFS= read -r line; do
  turn=$((turn + 1))
  case "$turn" in
    1)
      : > %q
      while [ ! -f %q ]; do sleep 0.01; done
      printf '{"type":"system","subtype":"init","session_id":"claude-session-1","model":"fake"}\n'
      printf '{"type":"assistant","message":{"role":"assistant","model":"fake","content":[{"type":"text","text":"first output"}],"usage":{"input_tokens":2,"output_tokens":1}}}\n'
      printf '{"type":"result","result":"first output","is_error":false}\n'
      ;;
    2)
      : > %q
      while [ ! -f %q ]; do sleep 0.01; done
      printf '{"type":"assistant","message":{"role":"assistant","model":"fake","content":[{"type":"text","text":"second output"}]}}\n'
      printf '{"type":"result","result":"second output","is_error":false}\n'
      ;;
    3)
      exit 0
      ;;
  esac
done
`, firstStarted, releaseFirst, secondStarted, releaseSecond)
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake Claude CLI: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	backend := NewClaudeBackend(fake, slog.Default())
	initial, err := backend.Start(ctx, &ExecuteRequest{
		AgentID:  "agent-1",
		Messages: []Message{{Role: RoleUser, Content: "first"}},
	}, &ExecuteOptions{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer backend.Close(initial)

	waitForFile(t, firstStarted)
	if _, err := backend.Send(ctx, initial, []Message{{Role: RoleUser, Content: "overlap"}}); err == nil ||
		!strings.Contains(err.Error(), "already in progress") {
		t.Fatalf("overlapping Send error = %v, want already in progress", err)
	}
	if err := os.WriteFile(releaseFirst, nil, 0o600); err != nil {
		t.Fatalf("release first turn: %v", err)
	}
	firstResult := readTurnResult(t, initial.Result)
	if firstResult.Status != "completed" || firstResult.Output != "first output" {
		t.Fatalf("first result = %+v", firstResult)
	}

	second, err := backend.Send(ctx, initial, []Message{{Role: RoleUser, Content: "second"}})
	if err != nil {
		t.Fatalf("second Send: %v", err)
	}
	waitForFile(t, secondStarted)
	if err := os.WriteFile(releaseSecond, nil, 0o600); err != nil {
		t.Fatalf("release second turn: %v", err)
	}
	secondResult := readTurnResult(t, second.Result)
	if secondResult.Status != "completed" || secondResult.Output != "second output" {
		t.Fatalf("second result = %+v", secondResult)
	}

	crashed, err := backend.Send(ctx, second, []Message{{Role: RoleUser, Content: "crash"}})
	if err != nil {
		t.Fatalf("crash Send: %v", err)
	}
	crashResult := readTurnResult(t, crashed.Result)
	if crashResult.Status != "failed" || !strings.Contains(crashResult.Error, "exited unexpectedly") {
		t.Fatalf("crash result = %+v", crashResult)
	}
	state := crashed.state.(SessionStater)
	select {
	case <-state.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Claude process exit was not published")
	}
	if state.IsAlive() {
		t.Fatal("Claude remained alive after process exit")
	}
}

func TestStableACPPersistentProviderTurnContract(t *testing.T) {
	providers := []struct {
		name string
		new  func(string) PersistentBackend
	}{
		{name: "opencode", new: func(path string) PersistentBackend { return NewOpenCodeBackend(path, slog.Default()) }},
		{name: "hermes", new: func(path string) PersistentBackend { return NewHermesBackend(path, slog.Default()) }},
		{name: "openclaw", new: func(path string) PersistentBackend { return NewOpenClawBackend(path, slog.Default()) }},
	}

	for _, provider := range providers {
		provider := provider
		t.Run(provider.name, func(t *testing.T) {
			t.Parallel()
			runACPPersistentProviderTurnContract(t, provider.name, provider.new)
		})
	}
}

func TestCodexPersistentProcessExitFailsActiveTurn(t *testing.T) {
	tempDir := t.TempDir()
	fake := filepath.Join(tempDir, "codex")

	script := `#!/bin/sh
turn_count=0
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*)
      printf '{"id":1,"result":{}}\n'
      ;;
    *'"method":"thread/start"'*)
      printf '{"id":2,"result":{"thread":{"id":"codex-session-1"}}}\n'
      ;;
    *'"method":"turn/start"'*)
      turn_count=$((turn_count + 1))
      if [ "$turn_count" -eq 1 ]; then
        printf '{"id":3,"result":{}}\n'
        printf '{"method":"turn/completed","params":{"threadId":"codex-session-1","turn":{"id":"turn-1","status":"completed"}}}\n'
      else
        printf '{"id":4,"result":{}}\n'
        exit 0
      fi
      ;;
  esac
done
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake Codex CLI: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	backend := NewCodexBackend(fake, slog.Default())
	initial, err := backend.Start(ctx, &ExecuteRequest{
		AgentID:  "agent-1",
		Messages: []Message{{Role: RoleUser, Content: "first"}},
	}, &ExecuteOptions{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer backend.Close(initial)
	if result := readTurnResult(t, initial.Result); result.Status != "completed" {
		t.Fatalf("initial result = %+v, want completed", result)
	}

	crashed, err := backend.Send(ctx, initial, []Message{{Role: RoleUser, Content: "crash"}})
	if err != nil {
		t.Fatalf("crash Send: %v", err)
	}
	result := readTurnResult(t, crashed.Result)
	if result.Status != "failed" || !strings.Contains(result.Error, "process exited unexpectedly") {
		t.Fatalf("crash result = %+v, want failed process exit", result)
	}

	state := crashed.state.(SessionStater)
	select {
	case <-state.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Codex process exit was not published")
	}
	if state.IsAlive() {
		t.Fatal("Codex remained alive after process exit")
	}
}

func TestCodexPersistentStopInterruptsTurnAndKeepsProcess(t *testing.T) {
	tempDir := t.TempDir()
	fake := filepath.Join(tempDir, "codex")

	script := `#!/bin/sh
turn_count=0
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*)
      printf '{"id":1,"result":{}}\n'
      ;;
    *'"method":"thread/start"'*)
      printf '{"id":2,"result":{"thread":{"id":"codex-session-1"}}}\n'
      ;;
    *'"method":"turn/start"'*)
      turn_count=$((turn_count + 1))
      if [ "$turn_count" -eq 1 ]; then
        printf '{"id":3,"result":{"turn":{"id":"turn-1"}}}\n'
      else
        printf '{"id":5,"result":{"turn":{"id":"turn-2"}}}\n'
        printf '{"method":"turn/completed","params":{"threadId":"codex-session-1","turn":{"id":"turn-2","status":"completed"}}}\n'
      fi
      ;;
    *'"method":"turn/interrupt"'*)
      case "$line" in
        *'"threadId":"codex-session-1"'*'"turnId":"turn-1"'*) ;;
        *) exit 9 ;;
      esac
      printf '{"id":4,"result":{}}\n'
      printf '{"method":"turn/completed","params":{"threadId":"codex-session-1","turn":{"id":"turn-1","status":"interrupted"}}}\n'
      ;;
  esac
done
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake Codex CLI: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	backend := NewCodexBackend(fake, slog.Default())
	initial, err := backend.Start(ctx, &ExecuteRequest{
		AgentID:  "agent-1",
		Messages: []Message{{Role: RoleUser, Content: "first"}},
	}, &ExecuteOptions{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer backend.Close(initial)

	if err := initial.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if result := readTurnResult(t, initial.Result); result.Status != "cancelled" {
		t.Fatalf("interrupted result = %+v, want cancelled", result)
	}
	if !initial.state.(SessionStater).IsAlive() {
		t.Fatal("Codex process exited after interrupting only the active turn")
	}

	second, err := backend.Send(ctx, initial, []Message{{Role: RoleUser, Content: "second"}})
	if err != nil {
		t.Fatalf("Send after interrupt: %v", err)
	}
	if result := readTurnResult(t, second.Result); result.Status != "completed" {
		t.Fatalf("second result = %+v, want completed", result)
	}
}

func runACPPersistentProviderTurnContract(t *testing.T, provider string, factory func(string) PersistentBackend) {
	t.Helper()
	tempDir := t.TempDir()
	fake := filepath.Join(tempDir, provider)
	firstStarted := filepath.Join(tempDir, "first-started")
	releaseFirst := filepath.Join(tempDir, "release-first")
	secondStarted := filepath.Join(tempDir, "second-started")
	releaseSecond := filepath.Join(tempDir, "release-second")
	thirdStarted := filepath.Join(tempDir, "third-started")

	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "--version" ]; then
  printf '2026.5.5\n'
  exit 0
fi
prompt_count=0
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*)
      printf '{"jsonrpc":"2.0","id":0,"result":{}}\n'
      ;;
    *'"method":"session/new"'*)
      printf '{"jsonrpc":"2.0","id":1,"result":{"sessionId":"%s-session-1"}}\n'
      ;;
    *'"method":"session/prompt"'*)
      prompt_count=$((prompt_count + 1))
      case "$prompt_count" in
        1)
          : > %q
          while [ ! -f %q ]; do sleep 0.01; done
          printf '{"jsonrpc":"2.0","id":2,"result":{"stopReason":"end_turn","usage":{"inputTokens":2,"outputTokens":1}}}\n'
          ;;
        2)
          : > %q
          while [ ! -f %q ]; do sleep 0.01; done
          printf '{"jsonrpc":"2.0","id":3,"result":{"stopReason":"end_turn"}}\n'
          ;;
        3)
          : > %q
          exit 0
          ;;
      esac
      ;;
  esac
done
`, provider, firstStarted, releaseFirst, secondStarted, releaseSecond, thirdStarted)
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake %s CLI: %v", provider, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	backend := factory(fake)
	initial, err := backend.Start(ctx, &ExecuteRequest{
		AgentID:  "agent-1",
		Messages: []Message{{Role: RoleUser, Content: "first"}},
	}, &ExecuteOptions{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer backend.Close(initial)

	waitForFile(t, firstStarted)
	if _, err := backend.Send(ctx, initial, []Message{{Role: RoleUser, Content: "overlap"}}); err == nil ||
		!strings.Contains(err.Error(), "already in progress") {
		t.Fatalf("overlapping Send error = %v, want already in progress", err)
	}
	if err := os.WriteFile(releaseFirst, nil, 0o600); err != nil {
		t.Fatalf("release first turn: %v", err)
	}
	if result := readTurnResult(t, initial.Result); result.Status != "completed" {
		t.Fatalf("initial result = %+v, want completed", result)
	}

	type sendOutcome struct {
		session *PersistentSession
		err     error
	}
	secondDone := make(chan sendOutcome, 1)
	go func() {
		ps, sendErr := backend.Send(ctx, initial, []Message{{Role: RoleUser, Content: "second"}})
		secondDone <- sendOutcome{session: ps, err: sendErr}
	}()
	waitForFile(t, secondStarted)
	if err := os.WriteFile(releaseSecond, nil, 0o600); err != nil {
		t.Fatalf("release second turn: %v", err)
	}

	var second *PersistentSession
	select {
	case outcome := <-secondDone:
		if outcome.err != nil {
			t.Fatalf("second Send: %v", outcome.err)
		}
		second = outcome.session
	case <-time.After(5 * time.Second):
		t.Fatal("second Send did not complete")
	}
	if result := readTurnResult(t, second.Result); result.Status != "completed" {
		t.Fatalf("second result = %+v, want completed", result)
	}

	thirdDone := make(chan error, 1)
	go func() {
		_, sendErr := backend.Send(ctx, second, []Message{{Role: RoleUser, Content: "crash"}})
		thirdDone <- sendErr
	}()
	waitForFile(t, thirdStarted)
	select {
	case err := <-thirdDone:
		if err == nil || !strings.Contains(err.Error(), "process exited") {
			t.Fatalf("crash Send error = %v, want process exited", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("crash Send did not terminate")
	}

	state, ok := second.state.(SessionStater)
	if !ok {
		t.Fatalf("%s state does not implement SessionStater", provider)
	}
	select {
	case <-state.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("provider process exit was not published")
	}
	if state.IsAlive() {
		t.Fatal("provider remained alive after process exit")
	}
}

func readTurnResult(t *testing.T, results <-chan *Result) *Result {
	t.Helper()
	select {
	case result, ok := <-results:
		if !ok || result == nil {
			t.Fatalf("result channel closed without a terminal result")
		}
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for terminal result")
		return nil
	}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", path, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", filepath.Base(path))
}
