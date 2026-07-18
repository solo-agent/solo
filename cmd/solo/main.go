// Command solo is a thin CLI wrapper around the Solo REST API, designed for
// AI Agents running inside Claude Code workspaces. It replaces raw curl
// commands with friendly subcommands.
//
// Usage:
//
//	solo message send  -c <content> --target <target>  (or stdin heredoc)
//	solo message read  --target <target> [--before <id>] [--limit <n>]
//	solo task list     -c <channel_id> [--status <s>] [--output json]
//	solo task claim    -n <number> -c <channel_id> [-m <message_id>]
//	solo task update   -n <number> -c <channel_id> -s <status>
//	solo task create   -c <channel_id> --title <title> [--description <desc>] [--priority <p0-p3>] [--parent <n>]
//	solo task unclaim  -n <number> -c <channel_id>
//	solo task submit   -n <number> -c <channel_id>
//	solo task accept   -n <number> -c <channel_id>
//	solo task reject   -n <number> -c <channel_id> --reason <reason>
//	solo task close    -n <number> -c <channel_id>
//	solo task reopen   -n <number> -c <channel_id>
//	solo artifact publish --task <task_id> --file <artifact.html> [--mode latest|final]
//	solo channel members -c <channel_id> [--output json]
//	solo channel join  --target <#channel-name>
//	solo team form     -c <channel_id> -m <message_id> [--plan <file>] [--output json]
//	solo thread unfollow --target <#channel:shortid>
//
//	--target format: '#channel' | 'dm:@peer' | '#channel:shortid' | 'dm:@peer:shortid'
//
// Authentication is via the SOLO_AUTH_TOKEN environment variable (falls back to SOLO_TOKEN).
// The API base URL is set via SOLO_API_URL (default: http://localhost:8080).
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"strings"
	"time"
)

// Exit codes matching the v1.3 CLI spec.
const (
	exitOK       = 0 // success
	exitBusiness = 1 // business error (already claimed, not found, not authorized)
	exitUsage    = 2 // usage / network / auth error
)

// doExit is a variable so tests can intercept os.Exit via panic/recover.
// In production, it points to os.Exit.
var doExit = os.Exit

func main() {
	doExit(runCLI(os.Args[1:]))
}

// runCLI contains the main CLI logic and returns an exit code. It is separated
// from main to allow testing without calling os.Exit.
func runCLI(args []string) int {
	if len(args) < 1 {
		printUsage()
		return exitUsage
	}

	// --help / -h is always allowed without auth.
	if args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printUsage()
		return exitOK
	}

	// SOLO_AUTH_TOKEN takes precedence; SOLO_TOKEN is a legacy fallback.
	token := os.Getenv("SOLO_AUTH_TOKEN")
	if token == "" {
		token = os.Getenv("SOLO_TOKEN")
	}
	if token == "" {
		fmt.Fprintln(os.Stderr, "solo: error: authentication failed -- SOLO_AUTH_TOKEN is missing or expired")
		return exitUsage
	}

	baseURL := os.Getenv("SOLO_API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	switch args[0] {
	case "task":
		handleTask(args[1:], baseURL, token)
	case "message":
		handleMessage(args[1:], baseURL, token)
	case "channel":
		handleChannel(args[1:], baseURL, token)
	case "team":
		handleTeam(args[1:], baseURL, token)
	case "artifact":
		handleArtifact(args[1:], baseURL, token)
	case "server":
		handleServer(args[1:], baseURL, token)
	case "thread":
		handleThread(args[1:], baseURL, token)
	default:
		fmt.Fprintf(os.Stderr, "solo: error: unknown command %q\n", args[0])
		printUsage()
		return exitUsage
	}
	// Handlers call doExit internally; this return is a safety net.
	return exitOK
}

// ---------------------------------------------------------------------------
// team
// ---------------------------------------------------------------------------

func handleTeam(args []string, baseURL, token string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "solo: error: team subcommand required: form")
		printUsage()
		doExit(exitUsage)
	}
	switch args[0] {
	case "form":
		handleTeamForm(args[1:], baseURL, token)
	default:
		fmt.Fprintf(os.Stderr, "solo: error: unknown team subcommand %q\n", args[0])
		printUsage()
		doExit(exitUsage)
	}
}

func handleTeamForm(args []string, baseURL, token string) {
	var channel, messageID, planFile, output string
	fs := flag.NewFlagSet("team form", flag.ExitOnError)
	fs.StringVar(&channel, "c", "", "Source onboarding channel ID or name")
	fs.StringVar(&channel, "source-channel", "", "Source onboarding channel ID or name")
	fs.StringVar(&messageID, "m", "", "Source user message UUID or short ID")
	fs.StringVar(&messageID, "source-message", "", "Source user message UUID or short ID")
	fs.StringVar(&planFile, "plan", "", "Path to a JSON team plan (defaults to stdin)")
	fs.StringVar(&output, "output", "", "Output format: json")
	fs.Parse(args)

	if strings.TrimSpace(channel) == "" || strings.TrimSpace(messageID) == "" {
		fmt.Fprintln(os.Stderr, "solo: error: --source-channel and --source-message are required")
		doExit(exitUsage)
	}

	var planBytes []byte
	var err error
	if planFile != "" {
		planBytes, err = os.ReadFile(planFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "solo: error: read team plan: %v\n", err)
			doExit(exitUsage)
		}
	} else {
		planBytes, err = io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "solo: error: read team plan from stdin: %v\n", err)
			doExit(exitUsage)
		}
	}
	planBytes = bytes.TrimSpace(planBytes)
	if len(planBytes) == 0 || !json.Valid(planBytes) {
		fmt.Fprintln(os.Stderr, "solo: error: team plan must be a valid JSON object from --plan or stdin")
		doExit(exitUsage)
	}
	var planValue any
	if err := json.Unmarshal(planBytes, &planValue); err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: parse team plan: %v\n", err)
		doExit(exitUsage)
	}
	if _, ok := planValue.(map[string]any); !ok {
		fmt.Fprintln(os.Stderr, "solo: error: team plan must be a JSON object")
		doExit(exitUsage)
	}

	channelID, resolveErr := resolveChannelParam(baseURL, token, channel)
	if resolveErr != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", resolveErr)
		doExit(exitBusiness)
	}
	reqBody, _ := json.Marshal(struct {
		SourceChannelID string          `json:"source_channel_id"`
		SourceMessageID string          `json:"source_message_id"`
		Plan            json.RawMessage `json:"plan"`
	}{
		SourceChannelID: strings.TrimSpace(strings.TrimPrefix(channelID, "#")),
		SourceMessageID: strings.TrimSpace(messageID),
		Plan:            json.RawMessage(planBytes),
	})

	statusCode, body, err := requestTeamFormation(baseURL, token, channelID, reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: request failed: %v\n", err)
		doExit(exitUsage)
	}
	if statusCode >= 400 {
		handleNonProxyHTTPError(statusCode, body)
	}
	if output == "json" {
		fmt.Println(string(body))
		doExit(exitOK)
	}

	var result struct {
		ChannelName  string `json:"channel_name"`
		ChannelID    string `json:"channel_id"`
		DashboardURL string `json:"dashboard_url"`
		Replayed     bool   `json:"replayed"`
		Members      []struct {
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"members"`
		RelationshipTemplate  string   `json:"relationship_template"`
		RelationshipOverrides int      `json:"relationship_override_count"`
		RelationshipDocsReady bool     `json:"relationship_docs_ready"`
		Warnings              []string `json:"warnings"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Println(string(body))
		doExit(exitOK)
	}
	label := "Team formed"
	if result.Replayed {
		label = "Team already formed"
	}
	fmt.Printf("%s: #%s\n", label, result.ChannelName)
	fmt.Printf("Channel ID: %s\n", result.ChannelID)
	for _, member := range result.Members {
		fmt.Printf("- @%s (%s)\n", member.Name, member.Role)
	}
	if result.RelationshipTemplate != "" {
		fmt.Printf("Relationships: %s", result.RelationshipTemplate)
		if result.RelationshipOverrides > 0 {
			fmt.Printf(" (+%d Lucy adjustment(s))", result.RelationshipOverrides)
		}
		fmt.Println()
	}
	for _, warning := range result.Warnings {
		fmt.Printf("Warning: %s\n", warning)
	}
	fmt.Printf("Open: %s\n", result.DashboardURL)
	doExit(exitOK)
}

func requestTeamFormation(baseURL, token, channelID string, reqBody []byte) (int, []byte, error) {
	statusCode, body, err := proxyRequest("team_form", channelID, string(reqBody), "", token, 0, "")
	if err != nil || statusCode == http.StatusBadGateway || statusCode == http.StatusGatewayTimeout {
		statusCode, body, err = doHTTPWithTimeout(http.MethodPost, baseURL+"/api/v1/team-formations", token, reqBody, teamFormationRequestTimeout)
	}
	if err != nil {
		return statusCode, body, err
	}

	// A daemon timeout cancels the original Server request. The Server then
	// records the provisioning attempt as failed with an independent context.
	// Briefly retry the same idempotency key so the CLI observes either the
	// committed result or safely reprovisions after that failure record lands.
	deadline := time.Now().Add(10 * time.Second)
	for isTeamFormationInProgress(statusCode, body) && time.Now().Before(deadline) {
		time.Sleep(250 * time.Millisecond)
		statusCode, body, err = doHTTPWithTimeout(http.MethodPost, baseURL+"/api/v1/team-formations", token, reqBody, teamFormationRequestTimeout)
		if err != nil {
			return statusCode, body, err
		}
	}
	return statusCode, body, nil
}

func isTeamFormationInProgress(statusCode int, body []byte) bool {
	return statusCode == http.StatusConflict && bytes.Contains(bytes.ToLower(body), []byte("already in progress"))
}

// ---------------------------------------------------------------------------
// artifact
// ---------------------------------------------------------------------------

func handleArtifact(args []string, baseURL, token string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "solo: error: artifact subcommand required (publish)")
		printUsage()
		doExit(exitUsage)
	}
	switch args[0] {
	case "publish":
		handleArtifactPublish(args[1:], baseURL, token)
	default:
		fmt.Fprintf(os.Stderr, "solo: error: unknown artifact subcommand %q\n", args[0])
		printUsage()
		doExit(exitUsage)
	}
}

func handleArtifactPublish(args []string, baseURL, token string) {
	var taskID, filePath, mode string
	fs := flag.NewFlagSet("artifact publish", flag.ExitOnError)
	fs.StringVar(&taskID, "task", "", "Task ID")
	fs.StringVar(&filePath, "file", "", "HTML file to publish")
	fs.StringVar(&mode, "mode", "latest", "Artifact mode: latest|final")
	fs.Parse(args)

	if taskID == "" || filePath == "" {
		fmt.Fprintln(os.Stderr, "solo: error: --task and --file are required")
		doExit(exitUsage)
	}
	if mode != "latest" && mode != "final" {
		fmt.Fprintf(os.Stderr, "solo: error: invalid --mode value %q (use latest or final)\n", mode)
		doExit(exitUsage)
	}
	htmlBytes, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: read artifact file: %v\n", err)
		doExit(exitUsage)
	}
	reqBody, _ := json.Marshal(map[string]string{
		"mode": mode,
		"html": string(htmlBytes),
	})
	url := fmt.Sprintf("%s/api/v1/tasks/%s/artifact/publish", baseURL, url.PathEscape(taskID))
	statusCode, body, err := doHTTP(http.MethodPost, url, token, reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", err)
		doExit(exitUsage)
	}
	if statusCode < 200 || statusCode >= 300 {
		fmt.Fprintf(os.Stderr, "solo: error: publish artifact failed (HTTP %d): %s\n", statusCode, string(body))
		doExit(exitBusiness)
	}
	fmt.Println(string(body))
	doExit(exitOK)
}

// ---------------------------------------------------------------------------
// task
// ---------------------------------------------------------------------------

// proxyRequest calls the daemon proxy instead of the server API directly.
// This keeps local thinking separate from channel communication.
func proxyRequest(action, channelID, content, threadID, token string, taskNumber int, status string) (int, []byte, error) {
	daemonURL := os.Getenv("SOLO_DAEMON_URL")
	if daemonURL == "" {
		daemonURL = "http://127.0.0.1:8081"
	}
	agentID := os.Getenv("SOLO_AGENT_ID")
	if agentID == "" {
		return 0, nil, fmt.Errorf("SOLO_AGENT_ID not set")
	}
	body := map[string]interface{}{
		"agent_id": agentID, "action": action, "channel_id": channelID,
	}
	if content != "" {
		body["content"] = content
	}
	if threadID != "" {
		body["thread_id"] = threadID
	}
	if nodeID := os.Getenv("SOLO_NODE_ID"); nodeID != "" {
		body["thinking_node_id"] = nodeID
	}
	if taskNumber > 0 {
		body["task_number"] = taskNumber
	}
	if status != "" {
		body["status"] = status
	}
	// For task_claim with -m, the message_id is passed in the content field.
	// Forward it as task_id so the proxy can construct the correct URL path.
	// Only when -n is NOT also specified (taskNumber <= 0) — -n takes priority.
	if (action == "task_claim" || action == "task_update" || action == "task_unclaim") && len(content) > 0 && taskNumber <= 0 {
		body["task_id"] = content
	}
	reqBody, _ := json.Marshal(body)
	url := daemonURL + "/internal/daemon/proxy"
	req, err := http.NewRequest("POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: proxyRequestTimeout(action)}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody, nil
}

func handleTask(args []string, baseURL, token string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "solo: error: task subcommand required (list, claim, update, create, unclaim, submit, accept, reject, close, reopen)")
		printUsage()
		doExit(exitUsage)
	}

	switch args[0] {
	case "list":
		handleTaskList(args[1:], baseURL, token)
	case "claim":
		handleTaskClaim(args[1:], baseURL, token)
	case "update":
		handleTaskUpdate(args[1:], baseURL, token)
	case "create":
		handleTaskCreate(args[1:], baseURL, token)
	case "unclaim":
		handleTaskUnclaim(args[1:], baseURL, token)
	case "submit", "accept", "reject", "close", "reopen":
		handleTaskLifecycle(args[1:], baseURL, token, args[0])
	default:
		fmt.Fprintf(os.Stderr, "solo: error: unknown task subcommand %q\n", args[0])
		printUsage()
		doExit(exitUsage)
	}
}

// --- task list ---

func handleTaskList(args []string, baseURL, token string) {
	var channel, status, output string
	fs := flag.NewFlagSet("task list", flag.ExitOnError)
	fs.StringVar(&channel, "c", "", "Channel ID or #name (optional, omit for all channels)")
	fs.StringVar(&channel, "channel", "", "Channel ID or #name (optional, omit for all channels)")
	fs.StringVar(&status, "status", "", "Filter by todo|in_progress|in_review|done")
	fs.StringVar(&output, "output", "", "Output format: json")
	fs.Parse(args)

	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "solo: error: unexpected argument: %s\n", fs.Arg(0))
		doExit(exitUsage)
	}
	if output != "" && output != "json" {
		fmt.Fprintf(os.Stderr, "solo: error: invalid --output value %q (only \"json\" is supported)\n", output)
		doExit(exitUsage)
	}

	if channel != "" {
		resolved, resolveErr := resolveChannelParam(baseURL, token, channel)
		if resolveErr != nil {
			fmt.Fprintf(os.Stderr, "solo: error: %v\n", resolveErr)
			doExit(exitBusiness)
		}
		channel = resolved
	}

	var url string
	if channel != "" {
		url = fmt.Sprintf("%s/api/v1/channels/%s/tasks", baseURL, channel)
	} else {
		url = fmt.Sprintf("%s/api/v1/tasks", baseURL)
	}
	if status != "" {
		url += "?status=" + status
	}

	statusCode, body, err := doHTTP(http.MethodGet, url, token, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: request failed: %v\n", err)
		doExit(exitUsage)
	}

	if statusCode >= 400 {
		if output == "json" {
			printJSONError(statusCode, extractErrorMessage(body))
		}
		handleNonProxyHTTPError(statusCode, body)
	}

	if output == "json" {
		printJSONEnvelope(body)
	} else {
		fmt.Println(string(body))
	}
	doExit(exitOK)
}

// --- task claim ---

func handleTaskClaim(args []string, baseURL, token string) {
	var channel, messageID string
	var number int
	fs := flag.NewFlagSet("task claim", flag.ExitOnError)
	fs.StringVar(&channel, "c", "", "Channel ID or #name")
	fs.StringVar(&channel, "channel", "", "Channel ID or #name")
	fs.IntVar(&number, "n", 0, "Task number")
	fs.IntVar(&number, "number", 0, "Task number")
	fs.StringVar(&messageID, "m", "", "Message ID (from msg= header)")
	fs.StringVar(&messageID, "message-id", "", "Message ID (from msg= header)")
	fs.Parse(args)

	if channel == "" {
		fmt.Fprintln(os.Stderr, "solo: error: -c <channel_id> is required")
		doExit(exitUsage)
	}
	if number <= 0 && messageID == "" {
		fmt.Fprintln(os.Stderr, "solo: error: -n <number> or -m <message_id> is required")
		doExit(exitUsage)
	}

	// Resolve channel name to UUID (strips #, URL-encodes)
	channelID, resolveErr := resolveChannelParam(baseURL, token, channel)
	if resolveErr != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", resolveErr)
		doExit(exitBusiness)
	}

	// Determine task ID: -n uses task number, -m uses message ID (alternative).
	taskID := ""
	if messageID != "" {
		taskID = messageID
	} else {
		taskID = strconv.Itoa(number)
	}

	// Try daemon proxy first (uses fresh JWT).
	// Pass messageID via taskID parameter so the proxy uses it in the URL path.
	statusCode, body, err := proxyRequest("task_claim", channelID, taskID, "", token, number, "")
	if err != nil {
		// Fallback to direct API
		apiURL := fmt.Sprintf("%s/api/v1/channels/%s/tasks/%s/claim", baseURL, channelID, taskID)
		statusCode, body, err = doHTTP(http.MethodPost, apiURL, token, nil)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: request failed: %v\n", err)
		doExit(exitUsage)
	}

	if statusCode == http.StatusConflict {
		label := fmt.Sprintf("#%d", number)
		if messageID != "" {
			label = fmt.Sprintf("msg:%s", messageID)
		}
		errMsg := extractErrorMessage(body)
		fmt.Printf("Claim results (0 claimed, 1 failed):\n")
		if strings.Contains(errMsg, "terminal") {
			fmt.Printf("%s: FAILED — task is already done/closed, cannot claim.\n", label)
		} else if strings.Contains(errMsg, "status does not allow") {
			fmt.Printf("%s: FAILED — task status does not allow claiming.\n", label)
		} else {
			fmt.Printf("%s: FAILED — already assigned. Do not reply.\n", label)
		}
		doExit(exitBusiness)
	}
	if statusCode >= 400 {
		handleNonProxyHTTPError(statusCode, body)
	}

	// Parse response to show result with thread target
	printClaimResult(body)
	doExit(exitOK)
}

// --- task update ---

func handleTaskUpdate(args []string, baseURL, token string) {
	var channel, status string
	var number int
	fs := flag.NewFlagSet("task update", flag.ExitOnError)
	fs.StringVar(&channel, "c", "", "Channel ID")
	fs.StringVar(&channel, "channel", "", "Channel ID")
	fs.IntVar(&number, "n", 0, "Task number (required)")
	fs.IntVar(&number, "number", 0, "Task number (required)")
	fs.StringVar(&status, "s", "", "New status: todo|in_progress|in_review|done (required)")
	fs.StringVar(&status, "status", "", "New status: todo|in_progress|in_review|done (required)")
	fs.Parse(args)

	if channel == "" {
		fmt.Fprintln(os.Stderr, "solo: error: -c <channel_id> is required")
		doExit(exitUsage)
	}
	if number <= 0 {
		fmt.Fprintln(os.Stderr, "solo: error: -n <number> must be a positive integer")
		doExit(exitUsage)
	}
	if status == "" {
		fmt.Fprintln(os.Stderr, "solo: error: -s <status> is required")
		doExit(exitUsage)
	}

	fmt.Fprintln(os.Stderr, "solo: task update no longer changes lifecycle status; use task claim, unclaim, submit, accept, reject, close, or reopen")
	doExit(exitUsage)
}

// --- task create ---

func handleTaskCreate(args []string, baseURL, token string) {
	var channel, title, description, priority string
	var parent int
	fs := flag.NewFlagSet("task create", flag.ExitOnError)
	fs.StringVar(&channel, "c", "", "Channel ID or #name (required)")
	fs.StringVar(&channel, "channel", "", "Channel ID or #name (required)")
	fs.StringVar(&title, "title", "", "Task title (required)")
	fs.StringVar(&description, "description", "", "Task description")
	fs.StringVar(&priority, "priority", "", "Task priority: p0|p1|p2|p3")
	fs.IntVar(&parent, "parent", 0, "Parent task number")
	fs.Parse(args)

	if channel == "" {
		fmt.Fprintln(os.Stderr, "solo: error: -c <channel_id> is required")
		doExit(exitUsage)
	}
	if title == "" {
		fmt.Fprintln(os.Stderr, "solo: error: --title is required")
		doExit(exitUsage)
	}
	if priority != "" {
		switch priority {
		case "p0", "p1", "p2", "p3":
		default:
			fmt.Fprintf(os.Stderr, "solo: error: invalid priority %q (must be p0, p1, p2, or p3)\n", priority)
			doExit(exitUsage)
		}
	}

	channelID, resolveErr := resolveChannelParam(baseURL, token, channel)
	if resolveErr != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", resolveErr)
		doExit(exitBusiness)
	}

	bodyMap := map[string]string{
		"title":       title,
		"description": description,
		"priority":    priority,
	}

	// Resolve --parent: look up the parent task by number in the channel to get its UUID.
	if parent > 0 {
		parentID, err := resolveTaskNumberToID(baseURL, token, channelID, parent)
		if err != nil {
			fmt.Fprintf(os.Stderr, "solo: error: %v\n", err)
			doExit(exitBusiness)
		}
		bodyMap["parent_task_id"] = parentID
	}

	reqBody, _ := json.Marshal(bodyMap)
	url := fmt.Sprintf("%s/api/v1/channels/%s/tasks", baseURL, channelID)
	statusCode, body, err := doHTTP(http.MethodPost, url, token, reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: request failed: %v\n", err)
		doExit(exitUsage)
	}

	if statusCode >= 400 {
		handleNonProxyHTTPError(statusCode, body)
	}

	fmt.Println(string(body))
	doExit(exitOK)
}

// --- task unclaim ---

func handleTaskUnclaim(args []string, baseURL, token string) {
	var channel string
	var number int
	fs := flag.NewFlagSet("task unclaim", flag.ExitOnError)
	fs.StringVar(&channel, "c", "", "Channel ID (required)")
	fs.StringVar(&channel, "channel", "", "Channel ID (required)")
	fs.IntVar(&number, "n", 0, "Task number (required)")
	fs.IntVar(&number, "number", 0, "Task number (required)")
	fs.Parse(args)

	if channel == "" {
		fmt.Fprintln(os.Stderr, "solo: error: -c <channel_id> is required")
		doExit(exitUsage)
	}
	if number <= 0 {
		fmt.Fprintln(os.Stderr, "solo: error: -n <number> must be a positive integer")
		doExit(exitUsage)
	}

	channelID, resolveErr := resolveChannelParam(baseURL, token, channel)
	if resolveErr != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", resolveErr)
		doExit(exitBusiness)
	}

	// Try daemon proxy first (uses fresh JWT)
	statusCode, body, err := proxyRequest("task_unclaim", channelID, "", "", token, number, "")
	if err != nil {
		// Fallback to direct API
		url := fmt.Sprintf("%s/api/v1/channels/%s/tasks/%d/claim", baseURL, channelID, number)
		statusCode, body, err = doHTTP(http.MethodDelete, url, token, nil)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: request failed: %v\n", err)
		doExit(exitUsage)
	}

	if statusCode >= 400 {
		handleNonProxyHTTPError(statusCode, body)
	}

	fmt.Println(string(body))
	doExit(exitOK)
}

func handleTaskLifecycle(args []string, baseURL, token, action string) {
	var channel, reason string
	var number int
	fs := flag.NewFlagSet("task "+action, flag.ExitOnError)
	fs.StringVar(&channel, "c", "", "Channel ID or #name (required)")
	fs.StringVar(&channel, "channel", "", "Channel ID or #name (required)")
	fs.IntVar(&number, "n", 0, "Task number (required)")
	fs.IntVar(&number, "number", 0, "Task number (required)")
	fs.StringVar(&reason, "r", "", "Reason for reject")
	fs.StringVar(&reason, "reason", "", "Reason for reject")
	fs.Parse(args)

	if channel == "" {
		fmt.Fprintln(os.Stderr, "solo: error: -c <channel_id> is required")
		doExit(exitUsage)
	}
	if number <= 0 {
		fmt.Fprintln(os.Stderr, "solo: error: -n <number> must be a positive integer")
		doExit(exitUsage)
	}
	if action == "reject" && strings.TrimSpace(reason) == "" {
		fmt.Fprintln(os.Stderr, "solo: error: --reason is required for task reject")
		doExit(exitUsage)
	}

	channelID, resolveErr := resolveChannelParam(baseURL, token, channel)
	if resolveErr != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", resolveErr)
		doExit(exitBusiness)
	}

	url := fmt.Sprintf("%s/api/v1/channels/%s/tasks/%d/%s", baseURL, channelID, number, action)
	var reqBody []byte
	if action == "reject" {
		reqBody, _ = json.Marshal(map[string]string{"reason": strings.TrimSpace(reason)})
	}
	statusCode, body, err := doHTTP(http.MethodPost, url, token, reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: request failed: %v\n", err)
		doExit(exitUsage)
	}
	if statusCode >= 400 {
		handleNonProxyHTTPError(statusCode, body)
	}

	fmt.Println(string(body))
	doExit(exitOK)
}

// ---------------------------------------------------------------------------
// message
// ---------------------------------------------------------------------------

func handleMessage(args []string, baseURL, token string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "solo: error: message subcommand required: send")
		printUsage()
		doExit(exitUsage)
	}

	switch args[0] {
	case "send":
		handleMessageSend(args[1:], baseURL, token)
	case "check":
		handleMessageCheck(args[1:], baseURL, token)
	case "read":
		handleMessageRead(args[1:], baseURL, token)
	default:
		fmt.Fprintf(os.Stderr, "solo: error: unknown message subcommand %q\n", args[0])
		printUsage()
		doExit(exitUsage)
	}
}

func handleMessageSend(args []string, baseURL, token string) {
	var content, target string
	fs := flag.NewFlagSet("message send", flag.ExitOnError)
	fs.StringVar(&content, "c", "", "Message content (-c or stdin)")
	fs.StringVar(&content, "content", "", "Message content (-c or stdin)")
	fs.StringVar(&target, "target", "", "Target: '#channel', 'dm:@peer', '#channel:shortid', or 'dm:@peer:shortid'")
	fs.Parse(args)

	// If no -c flag, read from stdin( heredoc support)
	if content == "" {
		stdinBytes, _ := io.ReadAll(os.Stdin)
		content = strings.TrimSpace(string(stdinBytes))
	}
	if content == "" {
		fmt.Fprintln(os.Stderr, "solo: error: -c <content> or stdin input is required")
		doExit(exitUsage)
	}
	if target == "" {
		fmt.Fprintln(os.Stderr, "solo: error: --target is required ('#channel', 'dm:@peer', '#channel:shortid')")
		doExit(exitUsage)
	}

	// Resolve target to channel ID + optional thread ID
	channelID, threadID, _, err := resolveTarget(baseURL, token, target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: failed to resolve target %q: %v\n", target, err)
		doExit(exitUsage)
	}

	reqBody := map[string]string{"content": content}
	if threadID != "" {
		reqBody["thread_id"] = threadID
	}
	if nodeID := os.Getenv("SOLO_NODE_ID"); nodeID != "" {
		reqBody["thinking_node_id"] = nodeID
	}

	body, _ := json.Marshal(reqBody)
	// Try daemon proxy first
	statusCode, respBody, err := proxyRequest("message_send", channelID, content, threadID, token, 0, "")
	if err != nil {
		// Fallback to direct API
		url := fmt.Sprintf("%s/api/v1/channels/%s/messages", baseURL, channelID)
		statusCode, respBody, err = doHTTP(http.MethodPost, url, token, body)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: request failed: %v\n", err)
		doExit(exitUsage)
	}

	if statusCode >= 400 {
		handleNonProxyHTTPError(statusCode, respBody)
	}

	printMessageSendResult(respBody, channelID, threadID)
	doExit(exitOK)
}

// ---------------------------------------------------------------------------
// channel
// ---------------------------------------------------------------------------

func handleChannel(args []string, baseURL, token string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "solo: error: channel subcommand required: members")
		printUsage()
		doExit(exitUsage)
	}

	switch args[0] {
	case "members":
		handleChannelMembers(args[1:], baseURL, token)
	case "join":
		handleChannelJoin(args[1:], baseURL, token)
	default:
		fmt.Fprintf(os.Stderr, "solo: error: unknown channel subcommand %q\n", args[0])
		printUsage()
		doExit(exitUsage)
	}
}

// proxyOrDirect tries the daemon proxy first, then falls back to direct API.
// Used for read-only commands that need fresh auth tokens in persistent sessions.
func proxyOrDirect(action, channelID, content string, token string) (int, []byte, error) {
	statusCode, body, err := proxyRequest(action, channelID, content, "", token, 0, "")
	if err != nil {
		// Fallback handled by caller
		return 0, nil, err
	}
	return statusCode, body, nil
}

func handleChannelMembers(args []string, baseURL, token string) {
	var channelID, output string
	fs := flag.NewFlagSet("channel members", flag.ExitOnError)
	fs.StringVar(&channelID, "c", "", "Channel ID")
	fs.StringVar(&channelID, "channel", "", "Channel ID")
	fs.StringVar(&output, "output", "", "Output format: json")
	fs.Parse(args)

	if channelID == "" {
		fmt.Fprintln(os.Stderr, "solo: error: -c <channel_id> is required")
		doExit(exitUsage)
	}

	// Strip # prefix from channel names and URL-encode to avoid
	// fragment parsing issues (e.g. "#test-2" → "test-2").
	channelID = strings.TrimPrefix(channelID, "#")
	url := fmt.Sprintf("%s/api/v1/channels/%s/members", baseURL, url.PathEscape(channelID))
	statusCode, body, err := doHTTP(http.MethodGet, url, token, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: request failed: %v\n", err)
		doExit(exitUsage)
	}

	if statusCode >= 400 {
		if output == "json" {
			printJSONError(statusCode, extractErrorMessage(body))
		}
		handleNonProxyHTTPError(statusCode, body)
	}

	if output == "json" {
		printJSONEnvelope(body)
	} else {
		fmt.Println(string(body))
	}
	doExit(exitOK)
}

// --- channel join ---

func handleChannelJoin(args []string, baseURL, token string) {
	var target string
	fs := flag.NewFlagSet("channel join", flag.ExitOnError)
	fs.StringVar(&target, "target", "", "Channel target: '#channel-name'")
	fs.Parse(args)

	if target == "" {
		fmt.Fprintln(os.Stderr, "solo: error: --target is required (e.g. '#channel-name')")
		doExit(exitUsage)
	}
	channelName := strings.TrimPrefix(target, "#")

	joinBody, _ := json.Marshal(map[string]string{"target": channelName})
	url := fmt.Sprintf("%s/api/v1/channels/join", baseURL)
	statusCode, body, err := doHTTP(http.MethodPost, url, token, joinBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: request failed: %v\n", err)
		doExit(exitUsage)
	}
	if statusCode >= 400 {
		handleNonProxyHTTPError(statusCode, body)
	}
	fmt.Println(string(body))
	doExit(exitOK)
}

// --- message check ---

func handleMessageCheck(args []string, baseURL, token string) {
	var channelID string
	fs := flag.NewFlagSet("message check", flag.ExitOnError)
	fs.StringVar(&channelID, "c", "", "Channel ID")
	fs.StringVar(&channelID, "channel", "", "Channel ID")
	fs.Parse(args)

	requestURL := fmt.Sprintf("%s/api/v1/messages/check", baseURL)
	if channelID != "" {
		requestURL += "?channel_id=" + url.QueryEscape(channelID)
		if nodeID := os.Getenv("SOLO_NODE_ID"); nodeID != "" {
			requestURL += "&thinking_node_id=" + url.QueryEscape(nodeID)
		}
	}

	statusCode, body, err := proxyRequest("message_check", channelID, "", "", token, 0, "")
	if err != nil {
		statusCode, body, err = doHTTP(http.MethodGet, requestURL, token, nil)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: request failed: %v\n", err)
		doExit(exitUsage)
	}

	if statusCode >= 400 {
		handleNonProxyHTTPError(statusCode, body)
	}

	fmt.Println(string(body))
	doExit(exitOK)
}

// ---------------------------------------------------------------------------
// server
// ---------------------------------------------------------------------------

func handleServer(args []string, baseURL, token string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "solo: error: server subcommand required: info")
		printUsage()
		doExit(exitUsage)
	}

	switch args[0] {
	case "info":
		handleServerInfo(args[1:], baseURL, token)
	default:
		fmt.Fprintf(os.Stderr, "solo: error: unknown server subcommand %q\n", args[0])
		printUsage()
		doExit(exitUsage)
	}
}

func handleServerInfo(args []string, baseURL, token string) {
	var output string
	fs := flag.NewFlagSet("server info", flag.ExitOnError)
	fs.StringVar(&output, "output", "", "Output format: json")
	fs.Parse(args)

	statusCode, body, err := proxyOrDirect("server_info", "", "", token)
	if err != nil {
		url := fmt.Sprintf("%s/api/v1/server/info", baseURL)
		statusCode, body, err = doHTTP(http.MethodGet, url, token, nil)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: request failed: %v\n", err)
		doExit(exitUsage)
	}

	if statusCode >= 400 {
		if output == "json" {
			printJSONError(statusCode, extractErrorMessage(body))
		}
		handleNonProxyHTTPError(statusCode, body)
	}

	if output == "json" {
		printJSONEnvelope(body)
	} else {
		fmt.Println(string(body))
	}
	doExit(exitOK)
}

// --- message read ---

func handleMessageRead(args []string, baseURL, token string) {
	var target, before, after string
	var limit int
	fs := flag.NewFlagSet("message read", flag.ExitOnError)
	fs.StringVar(&target, "target", "", "Target: '#channel', 'dm:@peer', '#channel:shortid', or 'dm:@peer:shortid'")
	fs.StringVar(&before, "before", "", "Cursor for pagination (message ID)")
	fs.StringVar(&after, "after", "", "Cursor for pagination (message ID)")
	fs.IntVar(&limit, "limit", 20, "Max messages to return")
	fs.Parse(args)

	if target == "" {
		fmt.Fprintln(os.Stderr, "solo: error: --target is required ('#channel', 'dm:@peer', '#channel:shortid')")
		doExit(exitUsage)
	}

	channelID, threadMsgID, isDM, err := resolveTarget(baseURL, token, target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: failed to resolve target %q: %v\n", target, err)
		doExit(exitUsage)
	}

	// Thread target: read specific thread messages
	if threadMsgID != "" {
		channelBase := "/api/v1/channels"
		if isDM {
			channelBase = "/api/v1/dm"
		}
		url := fmt.Sprintf("%s%s/%s/messages/%s/thread?limit=%d", baseURL, channelBase, channelID, threadMsgID, limit)
		if before != "" {
			url += "&before=" + before
		}
		if after != "" {
			url += "&after=" + after
		}
		statusCode, body, err := doHTTP(http.MethodGet, url, token, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "solo: error: request failed: %v\n", err)
			doExit(exitUsage)
		}
		if statusCode >= 400 {
			handleNonProxyHTTPError(statusCode, body)
		}
		fmt.Println(string(body))
		doExit(exitOK)
	}

	channelBase := "/api/v1/channels"
	if isDM {
		channelBase = "/api/v1/dm"
	}
	requestURL := fmt.Sprintf("%s%s/%s/messages?limit=%d", baseURL, channelBase, channelID, limit)
	if before != "" {
		requestURL += "&before=" + before
	}
	if after != "" {
		requestURL += "&after=" + after
	}
	if nodeID := os.Getenv("SOLO_NODE_ID"); nodeID != "" && !isDM {
		requestURL += "&thinking_node_id=" + url.QueryEscape(nodeID)
	}

	statusCode, body, err := doHTTP(http.MethodGet, requestURL, token, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: request failed: %v\n", err)
		doExit(exitUsage)
	}

	if statusCode >= 400 {
		handleNonProxyHTTPError(statusCode, body)
	}

	fmt.Println(string(body))
	doExit(exitOK)
}

// ---------------------------------------------------------------------------
// thread
// ---------------------------------------------------------------------------

func handleThread(args []string, baseURL, token string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "solo: error: thread subcommand required: unfollow")
		printUsage()
		doExit(exitUsage)
	}

	switch args[0] {
	case "unfollow":
		handleThreadUnfollow(args[1:], baseURL, token)
	default:
		fmt.Fprintf(os.Stderr, "solo: error: unknown thread subcommand %q\n", args[0])
		printUsage()
		doExit(exitUsage)
	}
}

func handleThreadUnfollow(args []string, baseURL, token string) {
	var target string
	fs := flag.NewFlagSet("thread unfollow", flag.ExitOnError)
	fs.StringVar(&target, "target", "", "Thread target: '#channel:shortid'")
	fs.Parse(args)

	if target == "" {
		fmt.Fprintln(os.Stderr, "solo: error: --target is required (e.g. '#general:abc123')")
		doExit(exitUsage)
	}

	url := fmt.Sprintf("%s/api/v1/threads/unfollow", baseURL)
	reqBody, _ := json.Marshal(map[string]string{"target": target})
	statusCode, body, err := doHTTP(http.MethodPost, url, token, reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: request failed: %v\n", err)
		doExit(exitUsage)
	}

	if statusCode >= 400 {
		handleNonProxyHTTPError(statusCode, body)
	}

	fmt.Println(string(body))
	doExit(exitOK)
}

// ---------------------------------------------------------------------------
// HTTP helper
// ---------------------------------------------------------------------------

// doHTTP performs an HTTP request with Bearer token auth. It returns the
// response status code, body, and any network-level error. The caller is
// responsible for interpreting status codes and printing output.
func doHTTP(method, url, token string, reqBody []byte) (int, []byte, error) {
	return doHTTPWithTimeout(method, url, token, reqBody, 30*time.Second)
}

const teamFormationRequestTimeout = 60 * time.Second

func proxyRequestTimeout(action string) time.Duration {
	if action == "team_form" {
		return teamFormationRequestTimeout
	}
	return 30 * time.Second
}

func doHTTPWithTimeout(method, url, token string, reqBody []byte, timeout time.Duration) (int, []byte, error) {
	var bodyReader io.Reader
	if reqBody != nil {
		bodyReader = bytes.NewReader(reqBody)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return 0, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("read response: %w", err)
	}

	return resp.StatusCode, respBody, nil
}

// ---------------------------------------------------------------------------
// Parent task resolution
// ---------------------------------------------------------------------------

// resolveTaskNumberToID looks up a task by its number in the given channel and
// returns its UUID. Used by task create --parent to map a human-readable task
// number to the UUID required by the API's parent_task_id field.
// resolveTarget parses a target into (channelID, threadMsgID, isDM, error).
// Supported formats:
//
//	"#channel"       → channel UUID
//	"dm:@peer"       → DM channel UUID
//	"#channel:short" → channel UUID + thread short message ID
//	"dm:@peer:short" → DM channel UUID + thread short message ID
func resolveTarget(baseURL, token, target string) (channelID, threadMsgID string, isDM bool, err error) {
	if target == "" {
		return "", "", false, fmt.Errorf("empty target")
	}
	isDM = strings.HasPrefix(target, "dm:@")
	namePart := target
	if isDM {
		namePart = strings.TrimPrefix(target, "dm:@")
	} else {
		namePart = strings.TrimPrefix(target, "#")
	}

	// Check for thread suffix: "name:shortid"
	if idx := strings.Index(namePart, ":"); idx > 0 {
		threadMsgID = namePart[idx+1:]
		namePart = namePart[:idx]
	}

	// Resolve channel name or UUID
	if len(namePart) == 36 && strings.Count(namePart, "-") == 4 {
		channelID = namePart
	} else {
		channelID, err = resolveChannelName(baseURL, token, namePart)
	}
	return
}

func resolveChannelName(baseURL, token, name string) (string, error) {
	url := fmt.Sprintf("%s/api/v1/server/info", baseURL)
	_, body, err := doHTTP(http.MethodGet, url, token, nil)
	if err != nil {
		return "", err
	}
	var resp struct {
		Channels []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		Agents []struct{ ID, Name, Handle string }
		Humans []struct{ ID, Name, Handle string }
	}
	if json.Unmarshal(body, &resp) != nil {
		return "", fmt.Errorf("parse server info")
	}
	for _, ch := range resp.Channels {
		if ch.Name == name {
			return ch.ID, nil
		}
	}
	return "", fmt.Errorf("channel %q not found", name)
}

// resolveChannelParam strips the # prefix (if present) and resolves a channel
// name to its UUID via the server info API. UUIDs are returned as-is (URL-encoded).
// Returns an error if the channel name cannot be resolved to a UUID.
func resolveChannelParam(baseURL, token, channel string) (string, error) {
	// Strip # prefix and dm:@ prefix
	channel = strings.TrimPrefix(channel, "#")
	channel = strings.TrimPrefix(channel, "dm:@")
	// UUID check: 36 chars with 4 dashes
	if len(channel) == 36 && strings.Count(channel, "-") == 4 {
		return url.PathEscape(channel), nil
	}
	// Resolve name to UUID via server info
	id, err := resolveChannelName(baseURL, token, channel)
	if err != nil {
		return "", fmt.Errorf("channel %q not found — use a channel ID (UUID) or check the channel name", channel)
	}
	return url.PathEscape(id), nil
}

func resolveTaskNumberToID(baseURL, token, channelID string, taskNumber int) (string, error) {
	url := fmt.Sprintf("%s/api/v1/channels/%s/tasks", baseURL, channelID)
	statusCode, body, err := doHTTP(http.MethodGet, url, token, nil)
	if err != nil {
		return "", fmt.Errorf("failed to list tasks for parent lookup: %w", err)
	}
	if statusCode >= 400 {
		return "", fmt.Errorf("failed to list tasks: %s", extractErrorMessage(body))
	}

	var tasks []struct {
		ID         string `json:"id"`
		TaskNumber int    `json:"task_number"`
	}
	if err := json.Unmarshal(body, &tasks); err != nil {
		return "", fmt.Errorf("failed to parse task list: %w", err)
	}

	for _, t := range tasks {
		if t.TaskNumber == taskNumber {
			return t.ID, nil
		}
	}
	return "", fmt.Errorf("parent task #%d not found in channel %s", taskNumber, channelID)
}

// ---------------------------------------------------------------------------
// Error / output helpers
// ---------------------------------------------------------------------------

// extractErrorMessage tries to pull a human-readable message from a JSON error
// response body. Falls back to the raw body string. Returns a fallback message
// with the HTTP status if the body is empty.
func extractErrorMessage(body []byte) string {
	var errResp struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &errResp) == nil {
		if errResp.Message != "" {
			return errResp.Message
		}
		if errResp.Error != "" {
			return errResp.Error
		}
	}
	if len(body) == 0 {
		return "(empty response body)"
	}
	return string(body)
}

// exitOnHTTPError maps HTTP status codes to exit codes and terminates the
// process. 400 Bad Request is treated as a usage error (exit code 2).
// All other error statuses are treated as business errors (exit code 1).
// Note: 401 Unauthorized is handled separately by handleNonProxyHTTPError
// which shows a tailored token-expiry message (SOLO-254-B).
func exitOnHTTPError(statusCode int) {
	switch statusCode {
	case http.StatusBadRequest:
		doExit(exitUsage)
	default:
		doExit(exitBusiness)
	}
}

// handleNonProxyHTTPError is the error handler for non-proxy (direct API) commands.
// For 401, it prints a clear token-expiry message and exits with code 1 per
// SOLO-254-B. For other statuses, it prints the server error and delegates to
// exitOnHTTPError.
func handleNonProxyHTTPError(statusCode int, body []byte) {
	if statusCode == http.StatusUnauthorized {
		fmt.Fprintln(os.Stderr, "solo: error: authentication failed — token expired. Re-run via daemon proxy or set SOLO_AUTH_TOKEN.")
		doExit(exitBusiness)
	}
	errMsg := extractErrorMessage(body)
	fmt.Fprintf(os.Stderr, "solo: error: %s\n", errMsg)
	exitOnHTTPError(statusCode)
}

// printTaskUpdateResult prints a task update confirmation.
func printTaskUpdateResult(body []byte, number int) {
	var resp struct {
		TaskNumber int    `json:"task_number"`
		Status     string `json:"status"`
	}
	if json.Unmarshal(body, &resp) != nil || resp.TaskNumber == 0 {
		fmt.Println(string(body))
		return
	}
	fmt.Printf("#%d moved to %s.\n", resp.TaskNumber, resp.Status)
}

// printMessageSendResult prints a message send confirmation
// including the thread target for easy follow-up.
func printMessageSendResult(body []byte, channelID, threadID string) {
	var resp struct {
		ID        string `json:"id"`
		ChannelID string `json:"channel_id"`
	}
	if json.Unmarshal(body, &resp) != nil || resp.ID == "" {
		fmt.Println(string(body))
		return
	}
	shortID := resp.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	if threadID != "" {
		fmt.Printf("Message sent to thread. Message ID: %s (to reply in this thread, use target \"%s:%s\")\n",
			resp.ID, channelID, threadID)
	} else {
		fmt.Printf("Message sent to channel. Message ID: %s (to reply in this message's thread, use --target 'channel:%s')\n",
			resp.ID, shortID)
	}
}

// printClaimResult parses the claim API response and prints a
// result showing the claimed task and its thread target.
func printClaimResult(body []byte) {
	var resp struct {
		ID          string `json:"id"`
		TaskNumber  int    `json:"task_number"`
		MessageID   string `json:"message_id,omitempty"`
		ChannelID   string `json:"channel_id"`
		ClaimerName string `json:"claimer_name,omitempty"`
	}
	if json.Unmarshal(body, &resp) == nil && resp.TaskNumber > 0 {
		shortID := ""
		if len(resp.MessageID) >= 8 {
			shortID = resp.MessageID[:8]
		}
		fmt.Printf("Claim results (1 claimed):\n")
		if shortID != "" {
			fmt.Printf("#%d (msg:%s): claimed\n\n", resp.TaskNumber, shortID)
			fmt.Printf("Follow up in the task's thread:\n")
			fmt.Printf("#%d → solo message send --target '%s:%s'\n", resp.TaskNumber, resp.ChannelID, shortID)
		} else {
			fmt.Printf("#%d: claimed\n", resp.TaskNumber)
		}
	} else {
		fmt.Println(string(body))
	}
}

// printJSONEnvelope outputs {"ok":true,"data":<body>} to stdout.
func printJSONEnvelope(body []byte) {
	var data json.RawMessage
	if err := json.Unmarshal(body, &data); err != nil {
		// Body may be plain text — wrap it as a string value.
		data, _ = json.Marshal(string(body))
	}
	out, _ := json.Marshal(map[string]interface{}{
		"ok":   true,
		"data": data,
	})
	fmt.Println(string(out))
}

// printJSONError outputs {"ok":false,"code":"...","message":"..."} to stdout.
// Used with --output json to keep error output machine-readable.
func printJSONError(statusCode int, message string) {
	code := fmt.Sprintf("%d", statusCode)
	out, _ := json.Marshal(map[string]string{
		"ok":      "false",
		"code":    code,
		"message": message,
	})
	fmt.Println(string(out))
}

// ---------------------------------------------------------------------------
// Usage
// ---------------------------------------------------------------------------

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage:
  solo message send  -c <content> --target <target>     (or stdin heredoc)
  solo message read  --target <target> [--before <id>] [--limit <n>]
  solo task list     -c <channel_id> [--status <s>] [--output json]
  solo task claim    -n <number> -c <channel_id> [-m <message_id>]
  solo task update   -n <number> -c <channel_id> -s <status>
  solo task create   -c <channel_id> --title <title> [--description <desc>] [--priority <p0-p3>] [--parent <n>]
  solo task unclaim  -n <number> -c <channel_id>
  solo task submit   -n <number> -c <channel_id>
  solo task accept   -n <number> -c <channel_id>
  solo task reject   -n <number> -c <channel_id> --reason <reason>
  solo task close    -n <number> -c <channel_id>
  solo task reopen   -n <number> -c <channel_id>
  solo artifact publish --task <task_id> --file <artifact.html> [--mode latest|final]
  solo channel members -c <channel_id> [--output json]
  solo channel join  --target <#channel-name>
  solo team form     -c <channel_id> -m <message_id> [--plan <file>] [--output json]
  solo thread unfollow --target <#channel:shortid>

  --target formats: '#channel' | 'dm:@peer' | '#channel:shortid' | 'dm:@peer:shortid'

Environment:
  SOLO_AUTH_TOKEN  JWT authentication token (required, falls back to SOLO_TOKEN)
  SOLO_API_URL     API base URL (default: http://localhost:8080)`)
}
