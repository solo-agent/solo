package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ── Blocked args ──

// codexBlockedArgs are flags hardcoded by the backend that must not be
// overridden by user-configured CustomArgs.
var codexBlockedArgs = map[string]blockedArgMode{
	"--listen": blockedWithValue,
}

const (
	codexStderrTailBytes                  = 2048
	defaultCodexSemanticInactivityTimeout = 10 * time.Minute
)

// CodexBackend implements Backend by spawning `codex app-server --listen stdio://`
// and communicating via JSON-RPC 2.0 over stdin/stdout.
type CodexBackend struct {
	executablePath string
	logger         *slog.Logger
}

// NewCodexBackend creates a new CodexBackend.
// If executablePath is empty it defaults to "codex".
func NewCodexBackend(executablePath string, logger *slog.Logger) *CodexBackend {
	if executablePath == "" {
		executablePath = "codex"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &CodexBackend{executablePath: executablePath, logger: logger}
}

// Name returns "codex".
func (b *CodexBackend) Name() string { return "codex" }

// Execute launches the codex CLI subprocess, sends the prompt via JSON-RPC 2.0,
// streams output events through Session.Messages, and delivers the final result
// on Session.Result.
func (b *CodexBackend) Execute(ctx context.Context, req *ExecuteRequest, opts *ExecuteOptions) (*Session, error) {
	execPath := b.executablePath
	if _, err := exec.LookPath(execPath); err != nil {
		return nil, fmt.Errorf("codex executable not found at %q: %w", execPath, err)
	}

	timeout := 20 * time.Minute
	if d, ok := ctx.Deadline(); ok {
		if t := time.Until(d); t > 0 {
			timeout = t
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	semanticInactivityTimeout := defaultCodexSemanticInactivityTimeout

	args := buildCodexArgs(opts)
	b.logger.Info("codex: starting", "exec", execPath, "args", args)

	cmd := exec.CommandContext(runCtx, execPath, args...)
	cmd.WaitDelay = 10 * time.Second
	if opts.WorkspaceDir != "" {
		cmd.Dir = opts.WorkspaceDir
	}
	cmd.Env = buildEnv(opts.Env)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("codex: stdout pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("codex: stdin pipe: %w", err)
	}
	stderrLog := newLogWriter(b.logger, "[codex:stderr] ")
	stderrTail := newStderrTail(stderrLog, codexStderrTailBytes)
	cmd.Stderr = stderrTail

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		cancel()
		return nil, fmt.Errorf("codex: start: %w", err)
	}
	b.logger.Info("codex: started", "pid", cmd.Process.Pid, "cwd", cmd.Dir)

	msgCh := make(chan OutputChunk, 256)
	resCh := make(chan *Result, 1)
	prompt := buildPrompt(req, opts)

	// ── CodexClient setup ──

	var outputMu sync.Mutex
	var output strings.Builder
	turnDone := make(chan bool, 1) // true = aborted

	// Use a package-level helper constant to represent turn done sentinel.
	const turnNotAborted = false

	c := &codexClient{
		logger:               b.logger,
		stdin:                stdin,
		pending:              make(map[int]*pendingRPC),
		notificationProtocol: "unknown",
		onChunk: func(chunk OutputChunk) {
			if chunk.Type == string(MessageText) && chunk.Content != "" {
				outputMu.Lock()
				output.WriteString(chunk.Content)
				outputMu.Unlock()
			}
			trySend(msgCh, chunk)
		},
		onSemanticActivity: func(description string) {
			b.logger.Debug("codex semantic activity observed", "activity", description)
		},
		onTurnDone: func(aborted bool) {
			select {
			case turnDone <- aborted:
			default:
			}
		},
	}

	// Start reading stdout in background.
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			c.handleLine(line)
		}
		c.closeAllPending(fmt.Errorf("codex process exited"))
	}()

	var waitOnce sync.Once
	drainAndWait := func() {
		waitOnce.Do(func() {
			_ = stdin.Close()
			_ = cmd.Wait()
		})
	}

	var stopOnce sync.Once
	stop := func() error {
		stopOnce.Do(func() {
			cancel()
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		})
		return nil
	}

	// Drive the session lifecycle in a goroutine.
	go func() {
		defer cancel()
		defer close(msgCh)
		defer close(resCh)
		defer drainAndWait()

		startTime := time.Now()
		finalStatus := "completed"
		var finalError string
		finalOutput := ""

		// 1. Initialize handshake.
		_, err := c.request(runCtx, "initialize", map[string]any{
			"clientInfo": map[string]any{
				"name":    "solo-agent-sdk",
				"title":   "Solo Agent SDK",
				"version": "1.0.0",
			},
			"capabilities": map[string]any{
				"experimentalApi": true,
			},
		})
		if err != nil {
			drainAndWait()
			finalStatus = "failed"
			finalError = withAgentStderr(fmt.Sprintf("codex initialize failed: %v", err), "codex", stderrTail.Tail())
			resCh <- &Result{Status: finalStatus, Error: finalError, DurationMs: time.Since(startTime).Milliseconds()}
			return
		}
		c.notify("initialized")

		// 2. Start a new thread.
		threadID, err := c.startThread(runCtx, opts)
		if err != nil {
			drainAndWait()
			finalStatus = "failed"
			finalError = withAgentStderr(err.Error(), "codex", stderrTail.Tail())
			resCh <- &Result{Status: finalStatus, Error: finalError, DurationMs: time.Since(startTime).Milliseconds()}
			return
		}
		c.threadID = threadID
		b.logger.Info("codex thread started", "thread_id", threadID)

		// 3. Send turn and wait for completion.
		_, err = c.request(runCtx, "turn/start", map[string]any{
			"threadId": threadID,
			"input": []map[string]any{
				{"type": "text", "text": prompt},
			},
		})
		if err != nil {
			drainAndWait()
			finalStatus = "failed"
			finalError = withAgentStderr(fmt.Sprintf("codex turn/start failed: %v", err), "codex", stderrTail.Tail())
			resCh <- &Result{Status: finalStatus, Error: finalError, DurationMs: time.Since(startTime).Milliseconds()}
			return
		}

		lastSemanticActivity := time.Now()
		semanticTimer := time.NewTimer(semanticInactivityTimeout)
		defer semanticTimer.Stop()

		waitingForTurn := true
		for waitingForTurn {
			select {
			case aborted := <-turnDone:
				waitingForTurn = false
				if aborted {
					finalStatus = "aborted"
					finalError = "turn was aborted"
				} else {
					c.turnErrorMu.Lock()
					errMsg := c.turnError
					c.turnErrorMu.Unlock()
					if errMsg != "" {
						finalStatus = "failed"
						finalError = errMsg
					}
				}
			case <-semanticTimer.C:
				waitingForTurn = false
				finalStatus = "timeout"
				finalError = fmt.Sprintf("codex semantic inactivity timeout after %s without agent progress (last activity: %s)", semanticInactivityTimeout, time.Since(lastSemanticActivity).Round(time.Second))
				b.logger.Warn("codex semantic inactivity timeout",
					"pid", cmd.Process.Pid,
					"thread_id", threadID,
				)
			case <-runCtx.Done():
				waitingForTurn = false
				if runCtx.Err() == context.DeadlineExceeded {
					finalStatus = "timeout"
					finalError = fmt.Sprintf("codex timed out after %s", timeout)
				} else {
					finalStatus = "cancelled"
					finalError = "execution cancelled"
				}
			}
		}

		duration := time.Since(startTime)
		b.logger.Info("codex finished", "pid", cmd.Process.Pid, "status", finalStatus, "duration", duration.Round(time.Millisecond).String())

		_ = stdin.Close()
		cancel()
		<-readerDone

		outputMu.Lock()
		finalOutput = output.String()
		outputMu.Unlock()

		c.usageMu.Lock()
		u := c.usage
		c.usageMu.Unlock()

		if u.InputTokens == 0 && u.OutputTokens == 0 {
			if scanned := scanCodexSessionUsage(startTime); scanned != nil {
				u = scanned.usage
			}
		}

		var usageMap map[string]TokenUsage
		if u.InputTokens > 0 || u.OutputTokens > 0 || u.CacheReadTokens > 0 || u.CacheWriteTokens > 0 {
			model := opts.Model
			if model == "" {
				model = "unknown"
			}
			usageMap = map[string]TokenUsage{model: u}
		}

		if finalError != "" {
			if tail := stderrTail.Tail(); tail != "" {
				finalError = fmt.Sprintf("%s (stderr: %s)", finalError, tail)
			}
		}

		resCh <- &Result{
			Status:     finalStatus,
			Output:     finalOutput,
			Error:      finalError,
			DurationMs: duration.Milliseconds(),
			Usage:      usageMap,
		}
	}()

	return &Session{
		Messages: msgCh,
		Result:   resCh,
		Stop:     stop,
	}, nil
}

// startThread calls thread/start to create a fresh codex thread.
func (c *codexClient) startThread(ctx context.Context, opts *ExecuteOptions) (string, error) {
	startResult, err := c.request(ctx, "thread/start", map[string]any{
		"model":                  nilIfEmpty(opts.Model),
		"modelProvider":          nil,
		"profile":                nil,
		"cwd":                    nilIfEmpty(opts.WorkspaceDir),
		"approvalPolicy":         nil,
		"sandbox":                nil,
		"config":                 nil,
		"baseInstructions":       nil,
		"developerInstructions":  nilIfEmpty(opts.SystemPrompt),
		"compactPrompt":          nil,
		"includeApplyPatchTool":  nil,
		"experimentalRawEvents":  false,
		"persistExtendedHistory": true,
	})
	if err != nil {
		return "", fmt.Errorf("codex thread/start failed: %w", err)
	}
	threadID := extractCodexThreadID(startResult)
	if threadID == "" {
		return "", fmt.Errorf("codex thread/start returned no thread ID")
	}
	return threadID, nil
}

// ── CLI argument construction ──

func buildCodexArgs(opts *ExecuteOptions) []string {
	args := []string{"app-server", "--listen", "stdio://"}
	args = append(args, filterCustomArgs(opts.CustomArgs, codexBlockedArgs)...)
	return args
}

// ── codexClient: JSON-RPC 2.0 transport ──

type turnDoneCallback func(aborted bool)

type codexClient struct {
	logger               *slog.Logger
	stdin                interface{ Write([]byte) (int, error) }
	mu                   sync.Mutex
	nextID               int
	pending              map[int]*pendingRPC
	threadID             string
	turnID               string
	onChunk              func(OutputChunk)
	onSemanticActivity   func(description string)
	onTurnDone           turnDoneCallback
	notificationProtocol string
	turnStarted          bool
	completedTurnIDs     map[string]bool

	usageMu sync.Mutex
	usage   TokenUsage

	turnErrorMu sync.Mutex
	turnError   string
}

func (c *codexClient) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	pr := &pendingRPC{ch: make(chan rpcResult, 1), method: method}
	c.pending[id] = pr
	c.mu.Unlock()

	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	data = append(data, '\n')
	if _, err := c.stdin.Write(data); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("write %s: %w", method, err)
	}

	select {
	case res := <-pr.ch:
		return res.result, res.err
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	}
}

func (c *codexClient) notify(method string) {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	data, _ := json.Marshal(msg)
	data = append(data, '\n')
	_, _ = c.stdin.Write(data)
}

func (c *codexClient) respond(id int, result any) {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	data, _ := json.Marshal(msg)
	data = append(data, '\n')
	_, _ = c.stdin.Write(data)
}

func (c *codexClient) respondError(id int, code int, message string) {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	}
	data, _ := json.Marshal(msg)
	data = append(data, '\n')
	_, _ = c.stdin.Write(data)
}

func (c *codexClient) closeAllPending(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, pr := range c.pending {
		pr.ch <- rpcResult{err: err}
		delete(c.pending, id)
	}
}

func (c *codexClient) handleLine(line string) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return
	}

	if _, hasID := raw["id"]; hasID {
		if _, hasResult := raw["result"]; hasResult {
			c.handleResponse(raw)
			return
		}
		if _, hasError := raw["error"]; hasError {
			c.handleResponse(raw)
			return
		}
		if _, hasMethod := raw["method"]; hasMethod {
			c.handleServerRequest(raw)
			return
		}
	}

	if _, hasMethod := raw["method"]; hasMethod {
		c.handleNotification(raw)
	}
}

func (c *codexClient) handleResponse(raw map[string]json.RawMessage) {
	var id int
	if err := json.Unmarshal(raw["id"], &id); err != nil {
		return
	}

	c.mu.Lock()
	pr, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	c.mu.Unlock()

	if !ok {
		return
	}

	if errData, hasErr := raw["error"]; hasErr {
		var rpcErr struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(errData, &rpcErr)
		pr.ch <- rpcResult{err: fmt.Errorf("%s: %s (code=%d)", pr.method, rpcErr.Message, rpcErr.Code)}
	} else {
		pr.ch <- rpcResult{result: raw["result"]}
	}
}

func (c *codexClient) handleServerRequest(raw map[string]json.RawMessage) {
	var id int
	_ = json.Unmarshal(raw["id"], &id)

	var method string
	_ = json.Unmarshal(raw["method"], &method)

	switch method {
	case "item/commandExecution/requestApproval", "execCommandApproval":
		c.respond(id, map[string]any{"decision": "accept"})
	case "item/fileChange/requestApproval", "applyPatchApproval":
		c.respond(id, map[string]any{"decision": "accept"})
	case "mcpServer/elicitation/request":
		c.respond(id, map[string]any{"action": "accept", "content": nil, "_meta": nil})
	default:
		c.logger.Warn("codex: unhandled server request", "method", method, "id", id)
		c.respondError(id, -32601, fmt.Sprintf("unhandled server request: %s", method))
	}
}

func (c *codexClient) handleNotification(raw map[string]json.RawMessage) {
	var method string
	_ = json.Unmarshal(raw["method"], &method)

	var params map[string]any
	if p, ok := raw["params"]; ok {
		_ = json.Unmarshal(p, &params)
	}

	if method == "codex/event" || strings.HasPrefix(method, "codex/event/") {
		c.notificationProtocol = "legacy"
		msgData, ok := params["msg"]
		if !ok {
			return
		}
		msgMap, ok := msgData.(map[string]any)
		if !ok {
			return
		}
		c.handleEvent(msgMap)
		return
	}

	if c.notificationProtocol != "legacy" {
		if c.notificationProtocol == "unknown" &&
			(method == "turn/started" || method == "turn/completed" ||
				method == "thread/started" || strings.HasPrefix(method, "item/")) {
			c.notificationProtocol = "raw"
		}
		if c.notificationProtocol == "raw" {
			c.handleRawNotification(method, params)
		}
	}
}

func (c *codexClient) handleEvent(msg map[string]any) {
	msgType, _ := msg["type"].(string)

	switch msgType {
	case "task_started":
		c.turnStarted = true
		if c.onChunk != nil {
			c.onChunk(OutputChunk{Type: string(MessageStatus), Content: "running"})
		}
	case "agent_message":
		text, _ := msg["message"].(string)
		if text != "" && c.onChunk != nil {
			c.onChunk(OutputChunk{Type: string(MessageText), Content: text})
		}
	case "exec_command_begin":
		callID, _ := msg["call_id"].(string)
		command, _ := msg["command"].(string)
		if c.onChunk != nil {
			c.onChunk(OutputChunk{
				Type: string(MessageToolUse),
				Tool: &ToolInfo{Name: "exec_command", CallID: callID, Input: map[string]any{"command": command}},
			})
		}
	case "exec_command_end":
		callID, _ := msg["call_id"].(string)
		output, _ := msg["output"].(string)
		if c.onChunk != nil {
			c.onChunk(OutputChunk{
				Type: string(MessageToolResult),
				Tool: &ToolInfo{Name: "exec_command", CallID: callID, Output: output},
			})
		}
	case "patch_apply_begin":
		callID, _ := msg["call_id"].(string)
		if c.onChunk != nil {
			c.onChunk(OutputChunk{
				Type: string(MessageToolUse),
				Tool: &ToolInfo{Name: "patch_apply", CallID: callID},
			})
		}
	case "patch_apply_end":
		callID, _ := msg["call_id"].(string)
		if c.onChunk != nil {
			c.onChunk(OutputChunk{
				Type: string(MessageToolResult),
				Tool: &ToolInfo{Name: "patch_apply", CallID: callID},
			})
		}
	case "task_complete":
		c.extractUsageFromMap(msg)
		if c.onTurnDone != nil {
			c.onTurnDone(false)
		}
	case "turn_aborted":
		if c.onTurnDone != nil {
			c.onTurnDone(true)
		}
	}
}

func (c *codexClient) handleRawNotification(method string, params map[string]any) {
	if threadID, ok := params["threadId"].(string); ok && c.threadID != "" && threadID != c.threadID {
		return
	}

	switch method {
	case "turn/started":
		c.turnStarted = true
		if turnID := extractNestedString(params, "turn", "id"); turnID != "" {
			c.turnID = turnID
		}
		if c.onChunk != nil {
			c.onChunk(OutputChunk{Type: string(MessageStatus), Content: "running"})
		}

	case "turn/completed":
		turnID := extractNestedString(params, "turn", "id")
		status := extractNestedString(params, "turn", "status")
		c.logger.Info("codex turn/completed received", "thread_id", c.threadID, "turn_id", turnID, "status", status)

		if status == "failed" {
			errMsg := extractNestedString(params, "turn", "error", "message")
			if errMsg == "" {
				errMsg = "codex turn failed"
			}
			c.setTurnError(errMsg)
		}

		if c.completedTurnIDs == nil {
			c.completedTurnIDs = map[string]bool{}
		}
		if turnID != "" {
			if c.completedTurnIDs[turnID] {
				return
			}
			c.completedTurnIDs[turnID] = true
		}

		if turn, ok := params["turn"].(map[string]any); ok {
			c.extractUsageFromMap(turn)
		}

		aborted := status == "cancelled" || status == "canceled" ||
			status == "aborted" || status == "interrupted"
		if c.onTurnDone != nil {
			c.onTurnDone(aborted)
		}

	case "error":
		willRetry, _ := params["willRetry"].(bool)
		errMsg := extractNestedString(params, "error", "message")
		if errMsg == "" {
			errMsg = extractNestedString(params, "message")
		}
		if errMsg != "" {
			c.logger.Warn("codex error notification", "message", errMsg, "will_retry", willRetry)
			if !willRetry {
				c.setTurnError(errMsg)
			}
		}

	case "thread/status/changed":
		statusType := extractNestedString(params, "status", "type")
		if statusType == "idle" && c.turnStarted {
			if c.onTurnDone != nil {
				c.onTurnDone(false)
			}
		}

	default:
		if strings.HasPrefix(method, "item/") {
			c.handleItemNotification(method, params)
		}
	}
}

func (c *codexClient) handleItemNotification(method string, params map[string]any) {
	item, _ := params["item"].(map[string]any)
	itemType, _ := item["type"].(string)
	itemID, _ := item["id"].(string)

	if item == nil {
		return
	}

	switch {
	case method == "item/started" && itemType == "commandExecution":
		command, _ := item["command"].(string)
		if c.onChunk != nil {
			c.onChunk(OutputChunk{
				Type: string(MessageToolUse),
				Tool: &ToolInfo{Name: "exec_command", CallID: itemID, Input: map[string]any{"command": command}},
			})
		}

	case method == "item/completed" && itemType == "commandExecution":
		output, _ := item["aggregatedOutput"].(string)
		if c.onChunk != nil {
			c.onChunk(OutputChunk{
				Type: string(MessageToolResult),
				Tool: &ToolInfo{Name: "exec_command", CallID: itemID, Output: output},
			})
		}

	case method == "item/started" && itemType == "fileChange":
		if c.onChunk != nil {
			c.onChunk(OutputChunk{
				Type: string(MessageToolUse),
				Tool: &ToolInfo{Name: "patch_apply", CallID: itemID},
			})
		}

	case method == "item/completed" && itemType == "fileChange":
		if c.onChunk != nil {
			c.onChunk(OutputChunk{
				Type: string(MessageToolResult),
				Tool: &ToolInfo{Name: "patch_apply", CallID: itemID},
			})
		}

	case method == "item/completed" && itemType == "agentMessage":
		text, _ := item["text"].(string)
		if text != "" && c.onChunk != nil {
			c.onChunk(OutputChunk{Type: string(MessageText), Content: text})
		}
		phase, _ := item["phase"].(string)
		if phase == "final_answer" && c.turnStarted {
			if c.onTurnDone != nil {
				c.onTurnDone(false)
			}
		}
	}
}

func (c *codexClient) setTurnError(msg string) {
	if msg == "" {
		return
	}
	c.turnErrorMu.Lock()
	defer c.turnErrorMu.Unlock()
	if c.turnError == "" {
		c.turnError = msg
	}
}

func (c *codexClient) extractUsageFromMap(data map[string]any) {
	var usageMap map[string]any
	for _, key := range []string{"usage", "token_usage", "tokens"} {
		if v, ok := data[key].(map[string]any); ok {
			usageMap = v
			break
		}
	}
	if usageMap == nil {
		return
	}

	c.usageMu.Lock()
	defer c.usageMu.Unlock()

	c.usage.InputTokens += codexInt64(usageMap, "input_tokens", "input", "prompt_tokens")
	c.usage.OutputTokens += codexInt64(usageMap, "output_tokens", "output", "completion_tokens")
	c.usage.CacheReadTokens += codexInt64(usageMap, "cache_read_tokens", "cache_read_input_tokens")
	c.usage.CacheWriteTokens += codexInt64(usageMap, "cache_write_tokens", "cache_creation_input_tokens")
}

func codexInt64(m map[string]any, keys ...string) int64 {
	for _, key := range keys {
		switch v := m[key].(type) {
		case float64:
			if v != 0 {
				return int64(v)
			}
		case int64:
			if v != 0 {
				return v
			}
		}
	}
	return 0
}

// ── Codex session log scanner ──

type codexSessionUsage struct {
	usage TokenUsage
	model string
}

func scanCodexSessionUsage(startTime time.Time) *codexSessionUsage {
	root := codexSessionRoot()
	if root == "" {
		return nil
	}

	dateDir := filepath.Join(root,
		fmt.Sprintf("%04d", startTime.Year()),
		fmt.Sprintf("%02d", int(startTime.Month())),
		fmt.Sprintf("%02d", startTime.Day()),
	)

	files, err := filepath.Glob(filepath.Join(dateDir, "*.jsonl"))
	if err != nil || len(files) == 0 {
		return nil
	}

	var result codexSessionUsage
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil || info.ModTime().Before(startTime) {
			continue
		}
		if u := parseCodexSessionFile(f); u != nil {
			result = *u
		}
	}

	if result.usage.InputTokens == 0 && result.usage.OutputTokens == 0 {
		return nil
	}
	return &result
}

func codexSessionRoot() string {
	if codexHome := os.Getenv("CODEX_HOME"); codexHome != "" {
		dir := filepath.Join(codexHome, "sessions")
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	dir := filepath.Join(home, ".codex", "sessions")
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return dir
	}
	return ""
}

type codexSessionTokenCount struct {
	Type    string `json:"type"`
	Payload *struct {
		Type string `json:"type"`
		Info *struct {
			TotalTokenUsage *struct {
				InputTokens           int64 `json:"input_tokens"`
				OutputTokens          int64 `json:"output_tokens"`
				CachedInputTokens     int64 `json:"cached_input_tokens"`
				CacheReadInputTokens  int64 `json:"cache_read_input_tokens"`
				ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
			} `json:"total_token_usage"`
			LastTokenUsage *struct {
				InputTokens           int64 `json:"input_tokens"`
				OutputTokens          int64 `json:"output_tokens"`
				CachedInputTokens     int64 `json:"cached_input_tokens"`
				CacheReadInputTokens  int64 `json:"cache_read_input_tokens"`
				ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
			} `json:"last_token_usage"`
			Model string `json:"model"`
		} `json:"info"`
		Model string `json:"model"`
	} `json:"payload"`
}

func parseCodexSessionFile(path string) *codexSessionUsage {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var result codexSessionUsage
	found := false

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()

		if !bytesContainsStr(line, "token_count") && !bytesContainsStr(line, "turn_context") {
			continue
		}

		var evt codexSessionTokenCount
		if err := json.Unmarshal(line, &evt); err != nil || evt.Payload == nil {
			continue
		}

		if evt.Type == "turn_context" && evt.Payload.Model != "" {
			result.model = evt.Payload.Model
			continue
		}

		if evt.Payload.Type == "token_count" && evt.Payload.Info != nil {
			usage := evt.Payload.Info.TotalTokenUsage
			if usage == nil {
				usage = evt.Payload.Info.LastTokenUsage
			}
			if usage != nil {
				cachedTokens := usage.CachedInputTokens
				if cachedTokens == 0 {
					cachedTokens = usage.CacheReadInputTokens
				}
				result.usage = TokenUsage{
					InputTokens:     usage.InputTokens,
					OutputTokens:    usage.OutputTokens + usage.ReasoningOutputTokens,
					CacheReadTokens: cachedTokens,
				}
				if evt.Payload.Info.Model != "" {
					result.model = evt.Payload.Info.Model
				}
				found = true
			}
		}
	}

	if !found {
		return nil
	}
	return &result
}

func bytesContainsStr(b []byte, s string) bool {
	return strings.Contains(string(b), s)
}

// ── Helpers ──

func extractCodexThreadID(result json.RawMessage) string {
	var r struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(result, &r); err != nil {
		return ""
	}
	return r.Thread.ID
}

func extractNestedString(m map[string]any, keys ...string) string {
	current := any(m)
	for _, key := range keys {
		obj, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = obj[key]
	}
	s, _ := current.(string)
	return s
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func withAgentStderr(msg, label, tail string) string {
	if tail == "" {
		return msg
	}
	return msg + "; " + label + " stderr: " + tail
}
