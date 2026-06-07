package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// ── Shared JSON-RPC types ──

type pendingRPC struct {
	ch     chan rpcResult
	method string
}

type rpcResult struct {
	result json.RawMessage
	err    error
}

// ── ACP prompt result ──

type acpPromptResult struct {
	stopReason string
	usage      TokenUsage
}

// ── ACP Client ──

// acpClient implements the ACP (Agent Communication Protocol) JSON-RPC 2.0
// transport over stdin/stdout. It is shared by Kimi, Kiro, and Hermes backends.
type acpClient struct {
	logger       *slog.Logger
	stdin        interface{ Write([]byte) (int, error) }
	writeMu      sync.Mutex
	mu           sync.Mutex
	nextID       int
	pending      map[int]*pendingRPC
	sessionID    string
	onChunk      func(OutputChunk)
	onPromptDone func(acpPromptResult)
	acceptNotification func(updateType string) bool

	toolMu       sync.Mutex
	pendingTools map[string]*pendingToolCall

	usageMu sync.Mutex
	usage   TokenUsage
}

type pendingToolCall struct {
	toolName string
	input    map[string]any
	argsText string
	emitted  bool
}

func (c *acpClient) writeLine(data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err := c.stdin.Write(data)
	return err
}

func (c *acpClient) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	id := c.nextID
	c.nextID++
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
	if err := c.writeLine(data); err != nil {
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

func (c *acpClient) closeAllPending(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, pr := range c.pending {
		pr.ch <- rpcResult{err: err}
		delete(c.pending, id)
	}
}

func (c *acpClient) handleLine(line string) {
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
			c.handleAgentRequest(raw)
			return
		}
	}

	if _, hasMethod := raw["method"]; hasMethod {
		c.handleNotification(raw)
	}
}

func (c *acpClient) handleAgentRequest(raw map[string]json.RawMessage) {
	var method string
	_ = json.Unmarshal(raw["method"], &method)

	rawID, ok := raw["id"]
	if !ok {
		return
	}

	var resp map[string]any
	switch method {
	case "session/request_permission":
		resp = map[string]any{
			"jsonrpc": "2.0",
			"id":      json.RawMessage(rawID),
			"result": map[string]any{
				"outcome": map[string]any{
					"outcome":  "selected",
					"optionId": "approve_for_session",
				},
			},
		}
		c.logger.Debug("auto-approved agent permission request", "method", method)
	default:
		resp = map[string]any{
			"jsonrpc": "2.0",
			"id":      json.RawMessage(rawID),
			"error": map[string]any{
				"code":    -32601,
				"message": "method not found: " + method,
			},
		}
		c.logger.Debug("unhandled agent client request", "method", method)
	}

	data, err := json.Marshal(resp)
	if err != nil {
		c.logger.Warn("marshal agent-request response", "method", method, "error", err)
		return
	}
	data = append(data, '\n')
	if err := c.writeLine(data); err != nil {
		c.logger.Warn("write agent-request response", "method", method, "error", err)
	}
}

func (c *acpClient) handleResponse(raw map[string]json.RawMessage) {
	var id int
	if err := json.Unmarshal(raw["id"], &id); err != nil {
		var fid float64
		if err := json.Unmarshal(raw["id"], &fid); err != nil {
			return
		}
		id = int(fid)
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
			Code    int             `json:"code"`
			Message string          `json:"message"`
			Data    json.RawMessage `json:"data"`
		}
		_ = json.Unmarshal(errData, &rpcErr)
		detail := ""
		if len(rpcErr.Data) > 0 && string(rpcErr.Data) != "null" {
			var s string
			if err := json.Unmarshal(rpcErr.Data, &s); err == nil {
				detail = s
			} else {
				detail = string(rpcErr.Data)
			}
		}
		if detail != "" {
			pr.ch <- rpcResult{err: fmt.Errorf("%s: %s (code=%d, data=%s)", pr.method, rpcErr.Message, rpcErr.Code, detail)}
		} else {
			pr.ch <- rpcResult{err: fmt.Errorf("%s: %s (code=%d)", pr.method, rpcErr.Message, rpcErr.Code)}
		}
	} else {
		if pr.method == "session/prompt" {
			c.extractPromptResult(raw["result"])
		}
		pr.ch <- rpcResult{result: raw["result"]}
	}
}

func (c *acpClient) extractPromptResult(data json.RawMessage) {
	var resp struct {
		StopReason string `json:"stopReason"`
		Usage      *struct {
			InputTokens      int64 `json:"inputTokens"`
			OutputTokens     int64 `json:"outputTokens"`
			TotalTokens      int64 `json:"totalTokens"`
			ThoughtTokens    int64 `json:"thoughtTokens"`
			CachedReadTokens int64 `json:"cachedReadTokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return
	}

	pr := acpPromptResult{
		stopReason: resp.StopReason,
	}
	if resp.Usage != nil {
		pr.usage = TokenUsage{
			InputTokens:     resp.Usage.InputTokens,
			OutputTokens:    resp.Usage.OutputTokens,
			CacheReadTokens: resp.Usage.CachedReadTokens,
		}
	}

	if c.onPromptDone != nil {
		c.onPromptDone(pr)
	}
}

func (c *acpClient) handleNotification(raw map[string]json.RawMessage) {
	var method string
	_ = json.Unmarshal(raw["method"], &method)

	if method != "session/update" && method != "session/notification" {
		return
	}

	var params struct {
		SessionID string          `json:"sessionId"`
		Update    json.RawMessage `json:"update"`
	}
	if p, ok := raw["params"]; ok {
		_ = json.Unmarshal(p, &params)
	}
	if len(params.Update) == 0 {
		return
	}

	updateType, updateData := normalizeACPUpdate(params.Update)
	if c.acceptNotification != nil && !c.acceptNotification(updateType) {
		return
	}

	switch updateType {
	case "agent_message_chunk":
		c.handleAgentMessage(updateData)
	case "agent_thought_chunk":
		c.handleAgentThought(updateData)
	case "tool_call":
		c.handleToolCallStart(updateData)
	case "tool_call_update":
		c.handleToolCallUpdate(updateData)
	case "usage_update":
		c.handleUsageUpdate(updateData)
	case "turn_end":
		c.extractPromptResult(updateData)
	}
}

func normalizeACPUpdate(data json.RawMessage) (string, json.RawMessage) {
	var updateType struct {
		SessionUpdate string `json:"sessionUpdate"`
		Type          string `json:"type"`
	}
	_ = json.Unmarshal(data, &updateType)
	if updateType.SessionUpdate != "" {
		return normalizeACPUpdateType(updateType.SessionUpdate), data
	}
	if updateType.Type != "" {
		return normalizeACPUpdateType(updateType.Type), data
	}

	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(data, &wrapper); err == nil && len(wrapper) == 1 {
		for k, v := range wrapper {
			return normalizeACPUpdateType(k), v
		}
	}

	return "", data
}

func normalizeACPUpdateType(t string) string {
	key := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(t), "_", ""), "-", ""))
	switch key {
	case "agentmessagechunk":
		return "agent_message_chunk"
	case "agentthoughtchunk":
		return "agent_thought_chunk"
	case "toolcall":
		return "tool_call"
	case "toolcallupdate":
		return "tool_call_update"
	case "usageupdate":
		return "usage_update"
	case "turnend", "endturn":
		return "turn_end"
	default:
		return ""
	}
}

func (c *acpClient) handleAgentMessage(data json.RawMessage) {
	var msg struct {
		Content struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &msg); err != nil || msg.Content.Text == "" {
		return
	}
	if c.onChunk != nil {
		c.onChunk(OutputChunk{Type: string(MessageText), Content: msg.Content.Text})
	}
}

func (c *acpClient) handleAgentThought(data json.RawMessage) {
	var msg struct {
		Content struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &msg); err != nil || msg.Content.Text == "" {
		return
	}
	if c.onChunk != nil {
		c.onChunk(OutputChunk{Type: string(MessageThinking), Content: msg.Content.Text})
	}
}

func (c *acpClient) handleToolCallStart(data json.RawMessage) {
	var msg struct {
		ToolCallID string            `json:"toolCallId"`
		Name       string            `json:"name"`
		Title      string            `json:"title"`
		Kind       string            `json:"kind"`
		RawInput   map[string]any    `json:"rawInput"`
		Input      map[string]any    `json:"input"`
		Parameters map[string]any    `json:"parameters"`
		Content    []json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	toolName := acpToolNameFromTitle(msg.Title, msg.Kind)
	if toolName == "" {
		toolName = msg.Name
	}
	rawInput := msg.RawInput
	if rawInput == nil {
		rawInput = msg.Input
	}
	if rawInput == nil {
		rawInput = msg.Parameters
	}

	if rawInput != nil {
		c.trackTool(msg.ToolCallID, &pendingToolCall{
			toolName: toolName,
			input:    rawInput,
			emitted:  true,
		})
		if c.onChunk != nil {
			c.onChunk(OutputChunk{
				Type: string(MessageToolUse),
				Tool: &ToolInfo{Name: toolName, CallID: msg.ToolCallID, Input: rawInput},
			})
		}
		return
	}

	c.trackTool(msg.ToolCallID, &pendingToolCall{
		toolName: toolName,
		argsText: extractACPToolCallText(msg.Content),
		emitted:  false,
	})
}

func (c *acpClient) handleToolCallUpdate(data json.RawMessage) {
	var msg struct {
		ToolCallID string            `json:"toolCallId"`
		Status     string            `json:"status"`
		Name       string            `json:"name"`
		Title      string            `json:"title"`
		Kind       string            `json:"kind"`
		RawInput   map[string]any    `json:"rawInput"`
		Input      map[string]any    `json:"input"`
		Parameters map[string]any    `json:"parameters"`
		RawOutput  string            `json:"rawOutput"`
		Output     string            `json:"output"`
		Content    []json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	rawInput := msg.RawInput
	if rawInput == nil {
		rawInput = msg.Input
	}
	if rawInput == nil {
		rawInput = msg.Parameters
	}
	title := msg.Title
	if title == "" {
		title = msg.Name
	}

	if msg.Status != "completed" && msg.Status != "failed" {
		if pending := c.getPendingTool(msg.ToolCallID); pending != nil && !pending.emitted {
			if text := extractACPToolCallText(msg.Content); text != "" {
				pending.argsText = text
			}
		}
		return
	}

	pending := c.takePendingTool(msg.ToolCallID)
	c.emitDeferredToolUse(pending, msg.ToolCallID, title, msg.Kind, rawInput)

	output := msg.RawOutput
	if output == "" {
		output = msg.Output
	}
	if output == "" {
		output = extractACPToolCallText(msg.Content)
	}
	if c.onChunk != nil {
		c.onChunk(OutputChunk{
			Type: string(MessageToolResult),
			Tool: &ToolInfo{CallID: msg.ToolCallID, Output: output},
		})
	}
}

func (c *acpClient) trackTool(callID string, p *pendingToolCall) {
	c.toolMu.Lock()
	defer c.toolMu.Unlock()
	if c.pendingTools == nil {
		c.pendingTools = make(map[string]*pendingToolCall)
	}
	c.pendingTools[callID] = p
}

func (c *acpClient) getPendingTool(callID string) *pendingToolCall {
	c.toolMu.Lock()
	defer c.toolMu.Unlock()
	if c.pendingTools == nil {
		return nil
	}
	return c.pendingTools[callID]
}

func (c *acpClient) takePendingTool(callID string) *pendingToolCall {
	c.toolMu.Lock()
	defer c.toolMu.Unlock()
	if c.pendingTools == nil {
		return nil
	}
	p := c.pendingTools[callID]
	delete(c.pendingTools, callID)
	return p
}

func (c *acpClient) emitDeferredToolUse(
	p *pendingToolCall,
	callID, updateTitle, updateKind string,
	updateRawInput map[string]any,
) {
	if p != nil && p.emitted {
		return
	}

	var toolName string
	var input map[string]any

	switch {
	case p != nil && p.input != nil:
		toolName = p.toolName
		input = p.input
	case p != nil:
		toolName = p.toolName
		input = parseToolArgsJSON(p.argsText)
	default:
		toolName = acpToolNameFromTitle(updateTitle, updateKind)
		input = updateRawInput
	}

	if c.onChunk == nil {
		return
	}
	c.onChunk(OutputChunk{
		Type: string(MessageToolUse),
		Tool: &ToolInfo{Name: toolName, CallID: callID, Input: input},
	})
}

func parseToolArgsJSON(argsText string) map[string]any {
	argsText = strings.TrimSpace(argsText)
	if argsText == "" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(argsText), &m); err == nil {
		return m
	}
	return map[string]any{"text": argsText}
}

func extractACPToolCallText(blocks []json.RawMessage) string {
	var b strings.Builder
	appendPiece := func(piece string) {
		if piece == "" {
			return
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(piece)
	}
	for _, raw := range blocks {
		var kind struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &kind); err != nil {
			continue
		}
		switch kind.Type {
		case "content":
			var outer struct {
				Content json.RawMessage `json:"content"`
			}
			if err := json.Unmarshal(raw, &outer); err != nil || len(outer.Content) == 0 {
				continue
			}
			var inner struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal(outer.Content, &inner); err != nil {
				continue
			}
			if inner.Type != "text" {
				continue
			}
			appendPiece(inner.Text)
		case "diff":
			var diff struct {
				Path    string `json:"path"`
				OldText string `json:"oldText"`
				NewText string `json:"newText"`
			}
			if err := json.Unmarshal(raw, &diff); err != nil || diff.Path == "" {
				continue
			}
			var piece strings.Builder
			piece.WriteString("--- ")
			piece.WriteString(diff.Path)
			piece.WriteString("\n+++ ")
			piece.WriteString(diff.Path)
			if diff.OldText == "" {
				piece.WriteString("\n(new file, ")
				piece.WriteString(strconv.Itoa(len(diff.NewText)))
				piece.WriteString(" bytes)")
			} else {
				piece.WriteString("\n(edited: ")
				piece.WriteString(strconv.Itoa(len(diff.OldText)))
				piece.WriteString("→ ")
				piece.WriteString(strconv.Itoa(len(diff.NewText)))
				piece.WriteString(" bytes)")
			}
			appendPiece(piece.String())
		}
	}
	return b.String()
}

func (c *acpClient) handleUsageUpdate(data json.RawMessage) {
	var msg struct {
		Usage struct {
			InputTokens      int64 `json:"inputTokens"`
			OutputTokens     int64 `json:"outputTokens"`
			TotalTokens      int64 `json:"totalTokens"`
			CachedReadTokens int64 `json:"cachedReadTokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	c.usageMu.Lock()
	if msg.Usage.InputTokens > c.usage.InputTokens {
		c.usage.InputTokens = msg.Usage.InputTokens
	}
	if msg.Usage.OutputTokens > c.usage.OutputTokens {
		c.usage.OutputTokens = msg.Usage.OutputTokens
	}
	if msg.Usage.CachedReadTokens > c.usage.CacheReadTokens {
		c.usage.CacheReadTokens = msg.Usage.CachedReadTokens
	}
	c.usageMu.Unlock()
}

// ── Helpers ──

func extractACPSessionID(result json.RawMessage) string {
	var r struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(result, &r); err != nil {
		return ""
	}
	return r.SessionID
}

func resolveResumedSessionID(requested string, response json.RawMessage) (string, bool) {
	got := extractACPSessionID(response)
	if got == "" {
		return requested, false
	}
	return got, got != requested
}

// buildACPUsageMap returns a usage map keyed by the given model, or nil if
// there are no tokens. It is shared by all ACP-family backends (hermes,
// kimi, kiro, openclaw, opencode) — they all wrap their per-turn usage
// the same way before emitting it to the daemon.
func buildACPUsageMap(usage TokenUsage, model string) map[string]TokenUsage {
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.CacheReadTokens == 0 {
		return nil
	}
	return map[string]TokenUsage{model: usage}
}

// acpToolNameFromTitle normalises an ACP tool title (and optional kind
// hint) into a canonical snake_case identifier used across the daemon
// and UI.
//
// Each ACP backend emits slightly different title strings — Hermes
// sends "execute code" with a structured kind, Kimi / Kiro send
// server-specific labels like "Bash" or "Read File" with no kind. The
// optional extras slice lets each backend append its own title→name
// mappings without forking this function. extras entries are matched
// case-insensitively against the trimmed title and (when present) the
// text before the first ":".
func acpToolNameFromTitle(title string, kind string, extras ...map[string]string) string {
	lookupExtras := func(s string) (string, bool) {
		lower := strings.ToLower(strings.TrimSpace(s))
		if lower == "" {
			return "", false
		}
		for _, m := range extras {
			if v, ok := m[lower]; ok {
				return v, true
			}
		}
		return "", false
	}

	switch title {
	case "execute code":
		return "execute_code"
	}

	if v, ok := lookupExtras(title); ok {
		return v
	}

	if idx := strings.Index(title, ":"); idx > 0 {
		name := strings.TrimSpace(title[:idx])
		if v, ok := lookupExtras(name); ok {
			return v
		}
		switch {
		case name == "terminal":
			return "terminal"
		case name == "read":
			return "read_file"
		case name == "write":
			return "write_file"
		case strings.HasPrefix(name, "patch"):
			return "patch"
		case name == "search":
			return "search_files"
		case name == "web search":
			return "web_search"
		case name == "extract":
			return "web_extract"
		case name == "delegate":
			return "delegate_task"
		case name == "analyze image":
			return "vision_analyze"
		}
		return name
	}

	switch kind {
	case "read":
		return "read_file"
	case "edit":
		return "write_file"
	case "execute":
		return "terminal"
	case "search":
		return "search_files"
	case "fetch":
		return "web_search"
	case "think":
		return "thinking"
	default:
		if title != "" {
			return title
		}
		return kind
	}
}

// ── Provider-error sniffing ──

type acpProviderErrorSniffer struct {
	provider string
	mu       sync.Mutex
	remains  []byte
	lines    []string
	seen     map[string]bool
	terminal bool
}

var acpErrorHeaderRe = regexp.MustCompile(`(?:⚠️|❌|\[ERROR\]).*(?:BadRequestError|AuthenticationError|RateLimitError|HTTP [0-9]{3}|Non-retryable|API call failed)`)

var acpErrorDetailRe = regexp.MustCompile(`(?:Error:|detail:|Details:)\s*(.+)`)

var acpTerminalErrorRe = regexp.MustCompile(`(?:❌|\[ERROR\]|after \d+ retr|Non-retryable|BadRequestError|AuthenticationError)`)

var acpAgentOutputTerminalRe = regexp.MustCompile(`API call failed after \d+ retr(?:y|ies)`)

const acpMaxErrorLines = 8

func newACPProviderErrorSniffer(provider string) *acpProviderErrorSniffer {
	return &acpProviderErrorSniffer{provider: provider, seen: map[string]bool{}}
}

func (s *acpProviderErrorSniffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data := append(s.remains, p...)
	nl := strings.LastIndexByte(string(data), '\n')
	var complete string
	if nl < 0 {
		s.remains = append(s.remains[:0], data...)
		return len(p), nil
	}
	complete = string(data[:nl])
	s.remains = append(s.remains[:0], data[nl+1:]...)

	for _, line := range strings.Split(complete, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !(acpErrorHeaderRe.MatchString(line) || acpErrorDetailRe.MatchString(line)) {
			continue
		}
		if acpTerminalErrorRe.MatchString(line) {
			s.terminal = true
		}
		if s.seen[line] {
			continue
		}
		s.seen[line] = true
		s.lines = append(s.lines, line)
		if len(s.lines) > acpMaxErrorLines {
			s.lines = s.lines[len(s.lines)-acpMaxErrorLines:]
		}
	}
	return len(p), nil
}

func (s *acpProviderErrorSniffer) message() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.messageLocked()
}

func (s *acpProviderErrorSniffer) terminalMessage() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.terminal {
		return ""
	}
	return s.messageLocked()
}

func (s *acpProviderErrorSniffer) messageLocked() string {
	prefix := s.provider + " provider error: "
	for _, line := range s.lines {
		if m := acpErrorDetailRe.FindStringSubmatch(line); m != nil {
			detail := strings.TrimSpace(m[1])
			if detail != "" {
				return prefix + detail
			}
		}
	}
	for _, line := range s.lines {
		if acpErrorHeaderRe.MatchString(line) {
			return prefix + line
		}
	}
	return ""
}

func promoteACPResultOnProviderError(finalStatus, finalError, finalOutput string, sniffer *acpProviderErrorSniffer) (string, string) {
	if finalStatus != "completed" {
		return finalStatus, finalError
	}
	if msg := sniffer.terminalMessage(); msg != "" {
		return "failed", msg
	}
	if acpAgentOutputTerminalRe.MatchString(finalOutput) {
		msg := sniffer.message()
		if msg == "" {
			msg = sniffer.provider + " provider error: " + acpAgentOutputTerminalRe.FindString(finalOutput)
		}
		return "failed", msg
	}
	if finalOutput == "" {
		if msg := sniffer.message(); msg != "" {
			return "failed", msg
		}
	}
	return finalStatus, finalError
}
