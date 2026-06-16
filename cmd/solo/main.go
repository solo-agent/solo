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
//	solo channel members -c <channel_id> [--output json]
//	solo channel join  --target <#channel-name>
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
	case "server":
		handleServer(args[1:], baseURL, token)
	case "thread":
		handleThread(args[1:], baseURL, token)
	case "knowledge":
		handleKnowledge(args[1:], baseURL, token)
	case "remind":
		handleRemind(args[1:], baseURL, token)
	default:
		fmt.Fprintf(os.Stderr, "solo: error: unknown command %q\n", args[0])
		printUsage()
		return exitUsage
	}
	// Handlers call doExit internally; this return is a safety net.
	return exitOK
}

// ---------------------------------------------------------------------------
// task
// ---------------------------------------------------------------------------


// proxyRequest calls the daemon proxy instead of the server API directly.
// This keeps local thinking separate from channel communication.
func proxyRequest(action, channelID, content, threadID, token string, taskNumber int, status string) (int, []byte, error) {
	daemonURL := os.Getenv("SOLO_DAEMON_URL")
	if daemonURL == "" { daemonURL = "http://127.0.0.1:8081" }
	agentID := os.Getenv("SOLO_AGENT_ID")
	if agentID == "" {
		return 0, nil, fmt.Errorf("SOLO_AGENT_ID not set")
	}
	body := map[string]interface{}{
		"agent_id": agentID, "action": action, "channel_id": channelID,
	}
	if content != "" { body["content"] = content }
	if threadID != "" { body["thread_id"] = threadID }
	if taskNumber > 0 { body["task_number"] = taskNumber }
	if status != "" { body["status"] = status }
	// For task_claim with -m, the message_id is passed in the content field.
	// Forward it as task_id so the proxy can construct the correct URL path.
	// Only when -n is NOT also specified (taskNumber <= 0) — -n takes priority.
	if (action == "task_claim" || action == "task_update" || action == "task_unclaim") && len(content) > 0 && taskNumber <= 0 {
		body["task_id"] = content
	}
	reqBody, _ := json.Marshal(body)
	url := daemonURL + "/internal/daemon/proxy"
	req, err := http.NewRequest("POST", url, bytes.NewReader(reqBody))
	if err != nil { return 0, nil, err }
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil { return 0, nil, err }
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody, nil
}


func handleTask(args []string, baseURL, token string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "solo: error: task subcommand required (list, claim, update, create, unclaim)")
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
	case "block":
		handleTaskBlock(args[1:], baseURL, token)
	case "unblock":
		handleTaskUnblock(args[1:], baseURL, token)
	case "blocked":
		handleTaskBlocked(args[1:], baseURL, token)
	case "split":
		handleTaskSplit(args[1:], baseURL, token)
	case "swarm-status":
		handleTaskSwarmStatus(args[1:], baseURL, token)
	case "swarm-decompose":
		handleTaskSwarmDecompose(args[1:], baseURL, token)
	case "isolate":
		handleTaskIsolate(args[1:], baseURL, token)
	case "unisolate":
		handleTaskUnisolate(args[1:], baseURL, token)
	default:
		fmt.Fprintf(os.Stderr, "solo: error: unknown task subcommand %q\n", args[0])
		printUsage()
		doExit(exitUsage)
	}
}

// --- task list ---

func handleTaskList(args []string, baseURL, token string) {
	var channel, status, output string
	var stale, blocked bool
	fs := flag.NewFlagSet("task list", flag.ExitOnError)
	fs.StringVar(&channel, "c", "", "Channel ID or #name (optional, omit for all channels)")
	fs.StringVar(&channel, "channel", "", "Channel ID or #name (optional, omit for all channels)")
	fs.StringVar(&status, "status", "", "Filter by todo|in_progress|in_review|done")
	fs.StringVar(&output, "output", "", "Output format: json")
	fs.BoolVar(&stale, "stale", false, "List stale tasks (T6.2.5)")
	fs.BoolVar(&blocked, "blocked", false, "List blocked tasks (T2.2.2)")
	fs.Parse(args)

	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "solo: error: unexpected argument: %s\n", fs.Arg(0))
		doExit(exitUsage)
	}
	if output != "" && output != "json" {
		fmt.Fprintf(os.Stderr, "solo: error: invalid --output value %q (only \"json\" is supported)\n", output)
		doExit(exitUsage)
	}

	// --stale uses the dedicated stale-tasks endpoint (T6.2.5)
	if stale {
		url := fmt.Sprintf("%s/api/v1/tasks/stale", baseURL)
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
	if status != "" || blocked {
		url += "?status=" + status
		if blocked {
			url += "&blocked=true"
		}
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
	var channel, messageID, deadline, escalateTo string
	var number int
	fs := flag.NewFlagSet("task claim", flag.ExitOnError)
	fs.StringVar(&channel, "c", "", "Channel ID or #name")
	fs.StringVar(&channel, "channel", "", "Channel ID or #name")
	fs.IntVar(&number, "n", 0, "Task number")
	fs.IntVar(&number, "number", 0, "Task number")
	fs.StringVar(&messageID, "m", "", "Message ID (from msg= header)")
	fs.StringVar(&messageID, "message-id", "", "Message ID (from msg= header)")
	fs.StringVar(&deadline, "deadline", "", "Claim deadline (duration like \"24h\" or ISO timestamp) — T6.2.5")
	fs.StringVar(&escalateTo, "escalate-to", "", "Escalate unclaimed task to @agent — T6.2.5")
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

	// T6.2.5: --deadline / --escalate-to set a watchdog on the claimed task.
	if deadline != "" || escalateTo != "" {
		var claimed struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(body, &claimed) == nil && claimed.ID != "" {
			wdBody := map[string]string{}
			if deadline != "" {
				wdBody["deadline"] = deadline
			}
			if escalateTo != "" {
				resolvedEscalate, resErr := resolveAgentParam(baseURL, token, escalateTo)
				if resErr != nil {
					fmt.Fprintf(os.Stderr, "solo: error: --escalate-to: %v\n", resErr)
					doExit(exitBusiness)
				}
				wdBody["escalate_to"] = resolvedEscalate
			}
			wdReqBody, _ := json.Marshal(wdBody)
			watchdogURL := fmt.Sprintf("%s/api/v1/tasks/%s/watchdog", baseURL, claimed.ID)
			wdCode, wdResp, wdErr := doHTTP(http.MethodPatch, watchdogURL, token, wdReqBody)
			if wdErr != nil {
				fmt.Fprintf(os.Stderr, "solo: watchdog set failed: %v\n", wdErr)
			} else if wdCode >= 400 {
				fmt.Fprintf(os.Stderr, "solo: watchdog set error: %s\n", extractErrorMessage(wdResp))
			}
		}
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

	channelID, resolveErr := resolveChannelParam(baseURL, token, channel)
	if resolveErr != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", resolveErr)
		doExit(exitBusiness)
	}

	// Try daemon proxy first (uses fresh JWT)
	statusCode, body, err := proxyRequest("task_update", channelID, "", "", token, number, status)
	if err != nil {
		// Fallback to direct API
		reqBody, _ := json.Marshal(map[string]string{"status": status})
		url := fmt.Sprintf("%s/api/v1/channels/%s/tasks/%d", baseURL, channelID, number)
		statusCode, body, err = doHTTP(http.MethodPatch, url, token, reqBody)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: request failed: %v\n", err)
		doExit(exitUsage)
	}

	if statusCode >= 400 {
		handleNonProxyHTTPError(statusCode, body)
	}

	printTaskUpdateResult(body, number)
	doExit(exitOK)
}

// --- task create ---

func handleTaskCreate(args []string, baseURL, token string) {
	var channel, title, description, priority string
	var parent int
	var swarm bool
	fs := flag.NewFlagSet("task create", flag.ExitOnError)
	fs.StringVar(&channel, "c", "", "Channel ID or #name (required)")
	fs.StringVar(&channel, "channel", "", "Channel ID or #name (required)")
	fs.StringVar(&title, "title", "", "Task title (required)")
	fs.StringVar(&description, "description", "", "Task description")
	fs.StringVar(&priority, "priority", "", "Task priority: p0|p1|p2|p3")
	fs.IntVar(&parent, "parent", 0, "Parent task number")
	fs.BoolVar(&swarm, "swarm", false, "Trigger Swarm decomposition after create (T6.3.7)")
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

	// T6.3.7: --swarm triggers Swarm decomposition on the freshly created task.
	if swarm {
		var created struct{ ID string `json:"id"` }
		if json.Unmarshal(body, &created) == nil && created.ID != "" {
			decomposeURL := fmt.Sprintf("%s/api/v1/tasks/%s/decompose", baseURL, created.ID)
			swarmBody, _ := json.Marshal(map[string]string{"channel_id": channelID})
			sc, swarmResp, swarmErr := doHTTP(http.MethodPost, decomposeURL, token, swarmBody)
			if swarmErr != nil {
				fmt.Fprintf(os.Stderr, "solo: swarm decompose failed: %v\n", swarmErr)
			} else if sc >= 400 {
				fmt.Fprintf(os.Stderr, "solo: swarm decompose error: %s\n", extractErrorMessage(swarmResp))
			} else {
				fmt.Println(string(swarmResp))
			}
		} else {
			fmt.Fprintln(os.Stderr, "solo: warning: --swarm set but could not parse task ID from create response")
		}
	}

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
	case "memory":
		handleChannelMemory(args[1:], baseURL, token)
	case "bind":
		handleChannelBind(args[1:], baseURL, token)
	case "unbind":
		handleChannelUnbind(args[1:], baseURL, token)
	case "workspace":
		handleChannelWorkspace(args[1:], baseURL, token)
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

	url := fmt.Sprintf("%s/api/v1/messages/check", baseURL)
	if channelID != "" {
		url += "?channel_id=" + channelID
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
		if before != "" { url += "&before=" + before }
		if after != "" { url += "&after=" + after }
		statusCode, body, err := doHTTP(http.MethodGet, url, token, nil)
		if err != nil { fmt.Fprintf(os.Stderr, "solo: error: request failed: %v\n", err); doExit(exitUsage) }
		if statusCode >= 400 { handleNonProxyHTTPError(statusCode, body) }
		fmt.Println(string(body))
		doExit(exitOK)
	}


	channelBase := "/api/v1/channels"
	if isDM {
		channelBase = "/api/v1/dm"
	}
	url := fmt.Sprintf("%s%s/%s/messages?limit=%d", baseURL, channelBase, channelID, limit)
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

func handleTaskBlock(args []string, baseURL, token string) {
	var taskNum int
	var onTaskNum int
	flags := flag.NewFlagSet("task block", flag.ContinueOnError)
	flags.IntVar(&taskNum, "n", 0, "Task number to block")
	flags.IntVar(&onTaskNum, "on", 0, "Task number that this task depends on")
	flags.Parse(args)
	if taskNum == 0 || onTaskNum == 0 {
		fmt.Fprintln(os.Stderr, "solo: error: -n and --on are required")
		doExit(exitUsage)
	}
	channelID := os.Getenv("SOLO_CHANNEL_ID")
	taskID := os.Getenv("SOLO_TASK_ID")
	if taskNum > 0 {
		taskID = fmt.Sprintf("task:%d:%s", taskNum, channelID)
	}
	body := map[string]any{"blocked_task_id": taskID, "blocker_task_id": fmt.Sprintf("task:%d:%s", onTaskNum, channelID)}
	reqBody, _ := json.Marshal(body)
	_, respBody, err := doHTTP("POST", baseURL+"/api/v1/task-dependencies", token, reqBody)
	if err != nil {
		printJSONError(500, err.Error())
		doExit(exitUsage)
	}
	fmt.Println(string(respBody))
}

func handleTaskUnblock(args []string, baseURL, token string) {
	var taskNum int
	var onTaskNum int
	flags := flag.NewFlagSet("task unblock", flag.ContinueOnError)
	flags.IntVar(&taskNum, "n", 0, "Task number to unblock")
	flags.IntVar(&onTaskNum, "on", 0, "Dependency task number")
	flags.Parse(args)
	if taskNum == 0 || onTaskNum == 0 {
		fmt.Fprintln(os.Stderr, "solo: error: -n and --on are required")
		doExit(exitUsage)
	}
	channelID := os.Getenv("SOLO_CHANNEL_ID")
	body := map[string]any{
		"blocked_task_id": fmt.Sprintf("task:%d:%s", taskNum, channelID),
		"blocker_task_id": fmt.Sprintf("task:%d:%s", onTaskNum, channelID),
	}
	reqBody, _ := json.Marshal(body)
	url := baseURL + "/api/v1/task-dependencies"
	statusCode, respBody, err := doHTTP("DELETE", url, token, reqBody)
	if err != nil {
		printJSONError(500, err.Error())
		doExit(exitUsage)
	}
	if statusCode >= 400 {
		handleNonProxyHTTPError(statusCode, respBody)
		doExit(exitBusiness)
	}
	fmt.Println("Dependency removed")
}

func handleTaskBlocked(args []string, baseURL, token string) {
	var taskNum int
	flags := flag.NewFlagSet("task blocked", flag.ContinueOnError)
	flags.IntVar(&taskNum, "n", 0, "Task number to check")
	flags.Parse(args)
	channelID := os.Getenv("SOLO_CHANNEL_ID")
	taskID := os.Getenv("SOLO_TASK_ID")
	if taskNum > 0 {
		taskID = fmt.Sprintf("task:%d:%s", taskNum, channelID)
	}
	url := baseURL + "/api/v1/tasks/" + taskID + "/is-blocked"
	_, respBody, err := doHTTP("GET", url, token, nil)
	if err != nil {
		printJSONError(500, err.Error())
		doExit(exitUsage)
	}
	fmt.Println(string(respBody))
}

// --- task isolate (T3.2.1) ---

func handleTaskIsolate(args []string, baseURL, token string) {
	var channelID string
	var number int
	fs := flag.NewFlagSet("task isolate", flag.ExitOnError)
	fs.StringVar(&channelID, "c", "", "Channel ID or #name (required)")
	fs.StringVar(&channelID, "channel", "", "Channel ID or #name (required)")
	fs.IntVar(&number, "n", 0, "Task number (required)")
	fs.IntVar(&number, "number", 0, "Task number (required)")
	fs.Parse(args)

	if channelID == "" {
		fmt.Fprintln(os.Stderr, "solo: error: -c <channel_id> is required")
		doExit(exitUsage)
	}
	if number <= 0 {
		fmt.Fprintln(os.Stderr, "solo: error: -n <number> must be a positive integer")
		doExit(exitUsage)
	}

	resolved, resolveErr := resolveChannelParam(baseURL, token, channelID)
	if resolveErr != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", resolveErr)
		doExit(exitBusiness)
	}

	taskID, err := resolveTaskNumberToID(baseURL, token, resolved, number)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", err)
		doExit(exitBusiness)
	}

	url := fmt.Sprintf("%s/api/v1/tasks/%s/isolate", baseURL, taskID)
	statusCode, body, err := doHTTP(http.MethodPost, url, token, nil)
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

// --- task unisolate (T3.2.2) ---

func handleTaskUnisolate(args []string, baseURL, token string) {
	var channelID string
	var number int
	fs := flag.NewFlagSet("task unisolate", flag.ExitOnError)
	fs.StringVar(&channelID, "c", "", "Channel ID or #name (required)")
	fs.StringVar(&channelID, "channel", "", "Channel ID or #name (required)")
	fs.IntVar(&number, "n", 0, "Task number (required)")
	fs.IntVar(&number, "number", 0, "Task number (required)")
	fs.Parse(args)

	if channelID == "" {
		fmt.Fprintln(os.Stderr, "solo: error: -c <channel_id> is required")
		doExit(exitUsage)
	}
	if number <= 0 {
		fmt.Fprintln(os.Stderr, "solo: error: -n <number> must be a positive integer")
		doExit(exitUsage)
	}

	resolved, resolveErr := resolveChannelParam(baseURL, token, channelID)
	if resolveErr != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", resolveErr)
		doExit(exitBusiness)
	}

	taskID, err := resolveTaskNumberToID(baseURL, token, resolved, number)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", err)
		doExit(exitBusiness)
	}

	url := fmt.Sprintf("%s/api/v1/tasks/%s/isolate", baseURL, taskID)
	statusCode, body, err := doHTTP(http.MethodDelete, url, token, nil)
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

// --- task split ---

func handleTaskSplit(args []string, baseURL, token string) {
	var channelID string
	var number int
	var titles string
	fs := flag.NewFlagSet("task split", flag.ExitOnError)
	fs.StringVar(&channelID, "c", "", "Channel ID or #name")
	fs.StringVar(&channelID, "channel", "", "Channel ID or #name")
	fs.IntVar(&number, "n", 0, "Parent task number")
	fs.IntVar(&number, "number", 0, "Parent task number")
	fs.StringVar(&titles, "titles", "", "Comma-separated subtask titles")
	fs.Parse(args)

	if channelID == "" {
		fmt.Fprintln(os.Stderr, "solo: error: -c <channel_id> is required")
		doExit(exitUsage)
	}
	if number <= 0 {
		fmt.Fprintln(os.Stderr, "solo: error: -n <number> must be a positive integer")
		doExit(exitUsage)
	}
	if titles == "" {
		fmt.Fprintln(os.Stderr, "solo: error: --titles is required (comma-separated)")
		doExit(exitUsage)
	}

	resolved, resolveErr := resolveChannelParam(baseURL, token, channelID)
	if resolveErr != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", resolveErr)
		doExit(exitBusiness)
	}

	// Fetch parent task by number to get its UUID
	parentID, err := resolveTaskNumberToID(baseURL, token, resolved, number)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", err)
		doExit(exitBusiness)
	}

	titleList := strings.Split(titles, ",")
	success := 0
	for _, t := range titleList {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		reqBody, _ := json.Marshal(map[string]string{
			"title":          t,
			"channel_id":     resolved,
			"parent_task_id": parentID,
		})
		url := fmt.Sprintf("%s/api/v1/channels/%s/tasks", baseURL, resolved)
		statusCode, body, err := doHTTP(http.MethodPost, url, token, reqBody)
		if err != nil || statusCode >= 400 {
			errMsg := extractErrorMessage(body)
			fmt.Fprintf(os.Stderr, "solo: error creating subtask %q: %s\n", t, errMsg)
		} else {
			fmt.Printf("Created subtask: %s\n", t)
			success++
		}
	}
	if success == 0 {
		doExit(exitBusiness)
	}
	doExit(exitOK)
}

// --- channel bind ---

func handleChannelBind(args []string, baseURL, token string) {
	var channelID, repoURL, repoBranch string
	fs := flag.NewFlagSet("channel bind", flag.ExitOnError)
	fs.StringVar(&channelID, "c", "", "Channel ID or #name")
	fs.StringVar(&channelID, "channel", "", "Channel ID or #name")
	fs.StringVar(&repoURL, "repo", "", "Repository URL (required)")
	fs.StringVar(&repoBranch, "branch", "main", "Git branch (default: main)")
	fs.Parse(args)

	if channelID == "" {
		fmt.Fprintln(os.Stderr, "solo: error: -c <channel_id> is required")
		doExit(exitUsage)
	}
	if repoURL == "" {
		fmt.Fprintln(os.Stderr, "solo: error: --repo <url> is required")
		doExit(exitUsage)
	}

	resolved, resolveErr := resolveChannelParam(baseURL, token, channelID)
	if resolveErr != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", resolveErr)
		doExit(exitBusiness)
	}

	reqBody, _ := json.Marshal(map[string]string{
		"repo_url":    repoURL,
		"repo_branch": repoBranch,
	})
	url := fmt.Sprintf("%s/api/v1/channels/%s/bind-project", baseURL, resolved)
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

// --- channel unbind ---

func handleChannelUnbind(args []string, baseURL, token string) {
	var channelID string
	fs := flag.NewFlagSet("channel unbind", flag.ExitOnError)
	fs.StringVar(&channelID, "c", "", "Channel ID or #name (required)")
	fs.StringVar(&channelID, "channel", "", "Channel ID or #name (required)")
	fs.Parse(args)

	if channelID == "" {
		fmt.Fprintln(os.Stderr, "solo: error: -c <channel_id> is required")
		doExit(exitUsage)
	}

	resolved, resolveErr := resolveChannelParam(baseURL, token, channelID)
	if resolveErr != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", resolveErr)
		doExit(exitBusiness)
	}

	url := fmt.Sprintf("%s/api/v1/channels/%s/bind-project", baseURL, resolved)
	statusCode, body, err := doHTTP(http.MethodDelete, url, token, nil)
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

// --- channel workspace ---

func handleChannelWorkspace(args []string, baseURL, token string) {
	var channelID string
	fs := flag.NewFlagSet("channel workspace", flag.ExitOnError)
	fs.StringVar(&channelID, "c", "", "Channel ID or #name (required)")
	fs.StringVar(&channelID, "channel", "", "Channel ID or #name (required)")
	fs.Parse(args)

	if channelID == "" {
		fmt.Fprintln(os.Stderr, "solo: error: -c <channel_id> is required")
		doExit(exitUsage)
	}

	resolved, resolveErr := resolveChannelParam(baseURL, token, channelID)
	if resolveErr != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", resolveErr)
		doExit(exitBusiness)
	}

	url := fmt.Sprintf("%s/api/v1/channels/%s/binding", baseURL, resolved)
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

// --- channel memory ---

func handleChannelMemory(args []string, baseURL, token string) {
	if len(args) < 1 {
		// Default: read CHANNEL.md
		handleChannelMemoryRead(args, baseURL, token)
		return
	}

	switch args[0] {
	case "set":
		handleChannelMemorySet(args[1:], baseURL, token)
	default:
		// Treat as subcommand to read (e.g., "solo channel memory -c #general")
		handleChannelMemoryRead(args, baseURL, token)
	}
}

func handleChannelMemoryRead(args []string, baseURL, token string) {
	var channelID string
	fs := flag.NewFlagSet("channel memory", flag.ExitOnError)
	fs.StringVar(&channelID, "c", "", "Channel ID or #name")
	fs.StringVar(&channelID, "channel", "", "Channel ID or #name")
	fs.Parse(args)

	if channelID == "" {
		fmt.Fprintln(os.Stderr, "solo: error: -c <channel_id> is required")
		doExit(exitUsage)
	}

	resolved, resolveErr := resolveChannelParam(baseURL, token, channelID)
	if resolveErr != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", resolveErr)
		doExit(exitBusiness)
	}

	url := fmt.Sprintf("%s/api/v1/channels/%s/memory/channel-md", baseURL, resolved)
	statusCode, body, err := doHTTP(http.MethodGet, url, token, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: request failed: %v\n", err)
		doExit(exitUsage)
	}
	if statusCode >= 400 {
		handleNonProxyHTTPError(statusCode, body)
		doExit(exitBusiness)
	}

	// Parse and pretty-print the content
	var resp struct {
		Content string `json:"content"`
	}
	if json.Unmarshal(body, &resp) == nil {
		fmt.Println(resp.Content)
	} else {
		fmt.Println(string(body))
	}
	doExit(exitOK)
}

func handleChannelMemorySet(args []string, baseURL, token string) {
	var channelID, filePath string
	fs := flag.NewFlagSet("channel memory set", flag.ExitOnError)
	fs.StringVar(&channelID, "c", "", "Channel ID or #name")
	fs.StringVar(&channelID, "channel", "", "Channel ID or #name")
	fs.StringVar(&filePath, "f", "", "Path to markdown file to upload")
	fs.StringVar(&filePath, "file", "", "Path to markdown file to upload")
	fs.Parse(args)

	if channelID == "" {
		fmt.Fprintln(os.Stderr, "solo: error: -c <channel_id> is required")
		doExit(exitUsage)
	}
	if filePath == "" {
		fmt.Fprintln(os.Stderr, "solo: error: -f <file_path> is required")
		doExit(exitUsage)
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: cannot read file %q: %v\n", filePath, err)
		doExit(exitUsage)
	}

	resolved, resolveErr := resolveChannelParam(baseURL, token, channelID)
	if resolveErr != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", resolveErr)
		doExit(exitBusiness)
	}

	reqBody, _ := json.Marshal(map[string]string{"content": string(content)})
	url := fmt.Sprintf("%s/api/v1/channels/%s/memory/channel-md", baseURL, resolved)
	statusCode, body, err := doHTTP(http.MethodPost, url, token, reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: request failed: %v\n", err)
		doExit(exitUsage)
	}
	if statusCode >= 400 {
		handleNonProxyHTTPError(statusCode, body)
		doExit(exitBusiness)
	}

	fmt.Println("CHANNEL.md updated successfully.")
	doExit(exitOK)
}

// HTTP helper
// ---------------------------------------------------------------------------

// doHTTP performs an HTTP request with Bearer token auth. It returns the
// response status code, body, and any network-level error. The caller is
// responsible for interpreting status codes and printing output.
func doHTTP(method, url, token string, reqBody []byte) (int, []byte, error) {
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

	client := &http.Client{Timeout: 30 * time.Second}
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

// resolveAgentParam strips the @ prefix (if present) and resolves an agent
// name/handle to its UUID via the server info API. UUIDs are returned as-is.
// Returns an error if the agent name cannot be resolved to a UUID (T2.1.3).
func resolveAgentParam(baseURL, token, agent string) (string, error) {
	agent = strings.TrimPrefix(agent, "@")
	// UUID check: 36 chars with 4 dashes
	if len(agent) == 36 && strings.Count(agent, "-") == 4 {
		return agent, nil
	}
	url := fmt.Sprintf("%s/api/v1/server/info", baseURL)
	_, body, err := doHTTP(http.MethodGet, url, token, nil)
	if err != nil {
		return "", fmt.Errorf("resolve agent: %w", err)
	}
	var resp struct {
		Agents []struct{ ID, Name, Handle string }
	}
	if json.Unmarshal(body, &resp) != nil {
		return "", fmt.Errorf("parse server info")
	}
	for _, a := range resp.Agents {
		if a.Name == agent || a.Handle == agent {
			return a.ID, nil
		}
	}
	return "", fmt.Errorf("agent %q not found — use an agent UUID or check the agent name", agent)
}

func resolveChannelName(baseURL, token, name string) (string, error) {
	url := fmt.Sprintf("%s/api/v1/server/info", baseURL)
	_, body, err := doHTTP(http.MethodGet, url, token, nil)
	if err != nil { return "", err }
	var resp struct {
		Channels []struct{ ID string `json:"id"`; Name string `json:"name"` }
		Agents   []struct{ ID, Name, Handle string }
		Humans   []struct{ ID, Name, Handle string }
	}
	if json.Unmarshal(body, &resp) != nil { return "", fmt.Errorf("parse server info") }
	for _, ch := range resp.Channels { if ch.Name == name { return ch.ID, nil } }
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

// ---------------------------------------------------------------------------
// knowledge
// ---------------------------------------------------------------------------

func handleKnowledge(args []string, baseURL, token string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "solo: error: knowledge subcommand required (add, get, search, import)")
		printUsage()
		doExit(exitUsage)
	}
	switch args[0] {
	case "add":
		handleKnowledgeAdd(args[1:], baseURL, token)
	case "get":
		handleKnowledgeGet(args[1:], baseURL, token)
	case "search":
		handleKnowledgeSearch(args[1:], baseURL, token)
	case "import":
		handleKnowledgeImport(args[1:], baseURL, token)
	default:
		fmt.Fprintf(os.Stderr, "solo: error: unknown knowledge subcommand %q\n", args[0])
		printUsage()
		doExit(exitUsage)
	}
}

func handleKnowledgeAdd(args []string, baseURL, token string) {
	var channel, title, content string
	fs := flag.NewFlagSet("knowledge add", flag.ExitOnError)
	fs.StringVar(&channel, "channel", "", "Channel ID or #name (required)")
	fs.StringVar(&channel, "c", "", "Channel ID or #name (required)")
	fs.StringVar(&title, "title", "", "Entry title (required)")
	fs.StringVar(&content, "content", "", "Entry content (required)")
	fs.Parse(args)
	if channel == "" || title == "" || content == "" {
		fmt.Fprintln(os.Stderr, "solo: error: --channel, --title, and --content are required")
		doExit(exitUsage)
	}
	resolved, err := resolveChannelParam(baseURL, token, channel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", err)
		doExit(exitBusiness)
	}
	reqBody, _ := json.Marshal(map[string]string{"channel_id": resolved, "title": title, "content": content})
	url := baseURL + "/api/v1/knowledge"
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

func handleKnowledgeGet(args []string, baseURL, token string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "solo: error: knowledge ID is required")
		doExit(exitUsage)
	}
	id := args[0]
	url := baseURL + "/api/v1/knowledge/" + id
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

func handleKnowledgeSearch(args []string, baseURL, token string) {
	var query, channel string
	fs := flag.NewFlagSet("knowledge search", flag.ExitOnError)
	fs.StringVar(&query, "q", "", "Search query (required)")
	fs.StringVar(&channel, "channel", "", "Channel ID or #name (required)")
	fs.StringVar(&channel, "c", "", "Channel ID or #name (required)")
	fs.Parse(args)
	if query == "" || channel == "" {
		fmt.Fprintln(os.Stderr, "solo: error: --q and --channel are required")
		doExit(exitUsage)
	}
	resolved, err := resolveChannelParam(baseURL, token, channel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", err)
		doExit(exitBusiness)
	}
	url := fmt.Sprintf("%s/api/v1/knowledge/search?q=%s&channel_id=%s", baseURL, url.QueryEscape(query), url.QueryEscape(resolved))
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

func handleKnowledgeImport(args []string, baseURL, token string) {
	var channel string
	fs := flag.NewFlagSet("knowledge import", flag.ExitOnError)
	fs.StringVar(&channel, "channel", "", "Channel ID or #name (required)")
	fs.StringVar(&channel, "c", "", "Channel ID or #name (required)")
	fs.Parse(args)
	if channel == "" {
		fmt.Fprintln(os.Stderr, "solo: error: --channel is required")
		doExit(exitUsage)
	}
	resolved, err := resolveChannelParam(baseURL, token, channel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", err)
		doExit(exitBusiness)
	}
	reqBody, _ := json.Marshal(map[string]string{"channel_id": resolved})
	url := baseURL + "/api/v1/knowledge/import-decisions"
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
// remind
// ---------------------------------------------------------------------------

func handleRemind(args []string, baseURL, token string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "solo: error: remind subcommand required (create, list, delete)")
		printUsage()
		doExit(exitUsage)
	}
	switch args[0] {
	case "create":
		handleRemindCreate(args[1:], baseURL, token)
	case "list":
		handleRemindList(args[1:], baseURL, token)
	case "delete":
		handleRemindDelete(args[1:], baseURL, token)
	default:
		fmt.Fprintf(os.Stderr, "solo: error: unknown remind subcommand %q\n", args[0])
		printUsage()
		doExit(exitUsage)
	}
}

func handleRemindCreate(args []string, baseURL, token string) {
	var agentID, remindAt, message string
	fs := flag.NewFlagSet("remind create", flag.ExitOnError)
	fs.StringVar(&agentID, "agent", "", "Agent ID or @name (required)")
	fs.StringVar(&remindAt, "at", "", "Reminder time in RFC3339 format (required)")
	fs.StringVar(&message, "msg", "", "Reminder message (required)")
	fs.Parse(args)
	if agentID == "" {
		agentID = os.Getenv("SOLO_AGENT_ID")
	}
	if agentID == "" || remindAt == "" || message == "" {
		fmt.Fprintln(os.Stderr, "solo: error: --agent, --at, and --msg are required")
		doExit(exitUsage)
	}
	resolved, err := resolveAgentParam(baseURL, token, agentID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", err)
		doExit(exitBusiness)
	}
	reqBody, _ := json.Marshal(map[string]string{
		"agent_id":      resolved,
		"reminder_type": "custom",
		"remind_at":     remindAt,
		"message":       message,
	})
	url := baseURL + "/api/v1/reminders"
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

func handleRemindList(args []string, baseURL, token string) {
	var agentID string
	fs := flag.NewFlagSet("remind list", flag.ExitOnError)
	fs.StringVar(&agentID, "agent", "", "Agent ID or @name")
	fs.Parse(args)
	var resolvedID string
	if agentID != "" {
		var err error
		resolvedID, err = resolveAgentParam(baseURL, token, agentID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "solo: error: %v\n", err)
			doExit(exitBusiness)
		}
	}
	url := baseURL + "/api/v1/reminders"
	if resolvedID != "" {
		url += "?agent_id=" + resolvedID
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

func handleRemindDelete(args []string, baseURL, token string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "solo: error: reminder ID is required")
		doExit(exitUsage)
	}
	id := args[0]
	url := baseURL + "/api/v1/reminders/" + id
	statusCode, body, err := doHTTP(http.MethodDelete, url, token, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: request failed: %v\n", err)
		doExit(exitUsage)
	}
	if statusCode >= 400 {
		handleNonProxyHTTPError(statusCode, body)
	}
	fmt.Println("Reminder deleted.")
	doExit(exitOK)
}

// ---------------------------------------------------------------------------
// task swarm-status
// ---------------------------------------------------------------------------

func handleTaskSwarmStatus(args []string, baseURL, token string) {
	var channelID string
	var taskNum int
	fs := flag.NewFlagSet("task swarm-status", flag.ExitOnError)
	fs.StringVar(&channelID, "c", "", "Channel ID or #name (required)")
	fs.StringVar(&channelID, "channel", "", "Channel ID or #name (required)")
	fs.IntVar(&taskNum, "n", 0, "Task number")
	fs.Parse(args)
	if channelID == "" {
		fmt.Fprintln(os.Stderr, "solo: error: -c <channel_id> is required")
		doExit(exitUsage)
	}
	resolved, err := resolveChannelParam(baseURL, token, channelID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", err)
		doExit(exitBusiness)
	}
	taskID := strconv.Itoa(taskNum)
	if taskID == "0" && len(args) > 0 {
		taskID = args[0]
	}
	url := fmt.Sprintf("%s/api/v1/tasks/%s/swarm-status?channel_id=%s", baseURL, taskID, resolved)
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

// ---------------------------------------------------------------------------
// task swarm-decompose
// ---------------------------------------------------------------------------

func handleTaskSwarmDecompose(args []string, baseURL, token string) {
	var channelID string
	var taskNum int
	var titles string
	fs := flag.NewFlagSet("task swarm-decompose", flag.ExitOnError)
	fs.StringVar(&channelID, "c", "", "Channel ID or #name (required)")
	fs.StringVar(&channelID, "channel", "", "Channel ID or #name (required)")
	fs.IntVar(&taskNum, "n", 0, "Task number (required)")
	fs.StringVar(&titles, "titles", "", "Comma-separated subtask titles (required)")
	fs.Parse(args)
	if channelID == "" || taskNum <= 0 || titles == "" {
		fmt.Fprintln(os.Stderr, "solo: error: -c, -n, and --titles are required")
		doExit(exitUsage)
	}
	resolved, err := resolveChannelParam(baseURL, token, channelID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", err)
		doExit(exitBusiness)
	}
	parentID, err := resolveTaskNumberToID(baseURL, token, resolved, taskNum)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: error: %v\n", err)
		doExit(exitBusiness)
	}
	titleList := strings.Split(titles, ",")
	subtasks := make([]map[string]interface{}, 0, len(titleList))
	for i, t := range titleList {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		subtasks = append(subtasks, map[string]interface{}{
			"title":             t,
			"description":       "",
			"depends_on_indices": []int{},
		})
		_ = i
	}
	if len(subtasks) == 0 {
		fmt.Fprintln(os.Stderr, "solo: error: no valid subtask titles provided")
		doExit(exitUsage)
	}
	reqBody, _ := json.Marshal(map[string]interface{}{
		"channel_id": resolved,
		"subtasks":   subtasks,
	})
	url := fmt.Sprintf("%s/api/v1/tasks/%s/decompose", baseURL, parentID)
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

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage:
  solo message send  -c <content> --target <target>     (or stdin heredoc)
  solo message read  --target <target> [--before <id>] [--limit <n>]
  solo task list     -c <channel_id> [--status <s>] [--stale] [--blocked] [--output json]
  solo task claim    -n <number> -c <channel_id> [-m <message_id>] [--deadline <duration>] [--escalate-to <@agent>]
  solo task update   -n <number> -c <channel_id> -s <status>
  solo task create   -c <channel_id> --title <title> [--description <desc>] [--priority <p0-p3>] [--parent <n>] [--swarm]
  solo task unclaim  -n <number> -c <channel_id>
  solo task split    -n <number> -c <channel_id> --titles <title1,title2,...>
  solo task block    -n <number> --on <number>
  solo task unblock  -n <number> --on <number>
  solo task blocked  -n <number>
  solo task isolate  -n <number> -c <channel_id>
  solo task unisolate -n <number> -c <channel_id>
  solo channel members -c <channel_id> [--output json]
  solo channel join  --target <#channel-name>
  solo channel memory [set] -c <channel_id> [-f <file>]
  solo channel bind   -c <channel_id> --repo <url> [--branch <branch>]
  solo channel unbind -c <channel_id>
  solo channel workspace -c <channel_id>
  solo thread unfollow --target <#channel:shortid>

  --target formats: '#channel' | 'dm:@peer' | '#channel:shortid' | 'dm:@peer:shortid'

Environment:
  SOLO_AUTH_TOKEN  JWT authentication token (required, falls back to SOLO_TOKEN)
  SOLO_API_URL     API base URL (default: http://localhost:8080)`)
}
