package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/solo-ai/solo/internal/realtime"
	"github.com/solo-ai/solo/pkg/agent"
)

// Default agent auto-response settings.
const (
	defaultContextMessageCount     = 1 // v1.3: — deliver only the triggering message
	defaultNodeContextMessageCount = 50
	defaultDebounceDuration        = 2 * time.Second
)

const (
	agentNoVisibleReplyAfter = 30 * time.Second
	agentNoProgressAfter     = 5 * time.Minute
	agentRunWatchdogInterval = 30 * time.Second

	agentRunEventNoVisibleReplyWatchdog = "watchdog_no_visible_reply"
	agentRunEventNoProgressWatchdog     = "watchdog_no_progress"

	agentActivityAccepted       = "agent.activity.accepted"
	agentActivityNoVisibleReply = "agent.activity.no_visible_reply"
	agentActivityNoProgress     = "agent.activity.no_progress"
	agentActivityCompleted      = "agent.activity.completed"
	agentActivityCancelled      = "agent.activity.cancelled"
	agentActivityTimeout        = "agent.activity.timeout"
	agentActivityFailed         = "agent.activity.failed"

	agentErrorNoAvailableDaemon = "agent.error.no_available_daemon"
)

// maxAgentChainDepth limits the depth of agent-to-agent trigger chains
// to prevent infinite loops. A chain of [A, B, C] means A triggered B,
// B triggered C; C cannot trigger another agent because depth == 3.
const maxAgentChainDepth = 3

// cascadeProtection detects runaway dispatch loops per channel.
const cascadeWindow = 10 * time.Second
const cascadeThreshold = 20
const cascadeCooldown = 60 * time.Second

type cascadeCount struct {
	count         int
	windowStart   time.Time
	cooldownUntil time.Time
}

// AgentService handles agent lifecycle, auto-response triggering, and status management.
type AgentService struct {
	pool *pgxpool.Pool
	dm   *DaemonManager
	hub  realtime.Broadcaster

	// debounceMap tracks the last trigger time per (channelID, agentID) pair
	// to prevent rapid re-triggering.
	debounceMap sync.Map
	// cascadeMap tracks dispatch velocity per channel for loop detection.
	cascadeMap sync.Map

	// httpClient for daemon communication
	httpClient *http.Client

	// claimWindow manages @mention priority claim windows for tasks.
	claimWindow *TaskClaimWindowManager

	// mentionSvc is used to resolve @mentions in agent responses
	// for agent-to-agent trigger chains.
	mentionSvc *MentionService
}

// NewAgentService creates a new AgentService.
func NewAgentService(pool *pgxpool.Pool, dm *DaemonManager, hub realtime.Broadcaster, mentionSvc *MentionService) *AgentService {
	return &AgentService{
		pool:        pool,
		dm:          dm,
		hub:         hub,
		httpClient:  &http.Client{Timeout: 120 * time.Second},
		claimWindow: NewTaskClaimWindowManager(),
		mentionSvc:  mentionSvc,
	}
}

// debounceKey generates a unique key for debounce tracking.
func debounceKey(channelID, agentID string) string {
	return channelID + ":" + agentID
}

// checkDebounce returns true if the agent was triggered within the debounce window.
func (s *AgentService) checkDebounce(channelID, agentID string) bool {
	key := debounceKey(channelID, agentID)
	lastTrigger, ok := s.debounceMap.Load(key)
	if !ok {
		return false
	}
	return time.Since(lastTrigger.(time.Time)) < defaultDebounceDuration
}

// updateDebounce records the last trigger time for debounce.
func (s *AgentService) updateDebounce(channelID, agentID string) {
	key := debounceKey(channelID, agentID)
	s.debounceMap.Store(key, time.Now())
}

// checkThreadDebounce returns true if the agent was triggered in this thread
// within the debounce window. Uses a thread-scoped key separate from channel-level
// debounce so thread follow-ups are not blocked by channel-level triggers.
func (s *AgentService) checkThreadDebounce(channelID, threadID, agentID string) bool {
	key := channelID + ":" + threadID + ":" + agentID
	lastTrigger, ok := s.debounceMap.Load(key)
	if !ok {
		return false
	}
	return time.Since(lastTrigger.(time.Time)) < defaultDebounceDuration
}

// updateThreadDebounce records the last trigger time for thread-level debounce.
func (s *AgentService) updateThreadDebounce(channelID, threadID, agentID string) {
	key := channelID + ":" + threadID + ":" + agentID
	s.debounceMap.Store(key, time.Now())
}

// TriggerAgentResponse checks whether an Agent should respond to a new message,
// gathers context, and dispatches the task to a Daemon via streaming SSE.
//
// mentionedAgentIDs: if non-empty, only the mentioned agents are triggered.
// If empty but hasMentions is true, @patterns existed but none resolved — suppress
// all agent responses. If empty and hasMentions is false, trigger the channel
// coordinator (or the first active agent as fallback).
//
// Called after a message is persisted and broadcast.
func (s *AgentService) TriggerAgentResponse(ctx context.Context, channelID, messageID, senderType, senderID string, mentionedAgentIDs []string, hasMentions bool, agentChain []string) {
	if !shouldTriggerAgentForSender(senderType, mentionedAgentIDs) {
		return
	}
	// If the message was sent by an agent, only proceed if it @mentions other agents.
	// Agents don't respond to themselves or to non-mentioned agent messages.

	// Find active agent members of this channel
	agents, err := s.getChannelActiveAgents(ctx, channelID)
	if err != nil {
		slog.Error("failed to get channel active agents", "channel_id", channelID, "error", err)
		return
	}

	// Resolve @mentioned agent names for context awareness in the system prompt.
	// This tells each agent WHO was mentioned, enabling "if it's for someone else, stay out."
	mentionedNames := s.resolveMentionedNames(ctx, mentionedAgentIDs)

	targetAgents := s.routeChannelTargets(ctx, agents, mentionedAgentIDs, hasMentions)
	if senderType == "agent" {
		targetAgents = excludeAgent(targetAgents, senderID)
	}

	if len(targetAgents) == 0 {
		return
	}

	// Get last N messages for context
	contextMessages, err := s.getRecentMessages(ctx, channelID, defaultContextMessageCount)
	if err != nil {
		slog.Error("failed to get context messages", "channel_id", channelID, "error", err)
		return
	}

	// Query for open tasks in the channel to provide task context to agents
	taskContext := s.getChannelOpenTasksSummary(ctx, channelID)

	// For each target agent, check debounce and dispatch via streaming
	for _, ag := range targetAgents {
		// Skip if agent was triggered recently
		if s.checkDebounce(channelID, ag.ID) {
			slog.Debug("agent debounced", "agent_id", ag.ID, "channel_id", channelID)
			continue
		}
		// Set debounce BEFORE spawning the goroutine to prevent
		// concurrent duplicate triggers from REST and WS handlers.
		s.updateDebounce(channelID, ag.ID)

		// SOLO-228-B: Validate agent trigger chain to prevent infinite loops.
		if agentChain != nil {
			if len(agentChain) >= maxAgentChainDepth {
				slog.Debug("agent chain depth limit reached", "agent_id", ag.ID, "channel_id", channelID, "depth", len(agentChain))
				continue
			}
			if containsStr(agentChain, ag.ID) {
				slog.Debug("agent already in trigger chain, skipping", "agent_id", ag.ID, "channel_id", channelID)
				continue
			}
		}
		newChain := append([]string(nil), agentChain...)
		newChain = append(newChain, ag.ID)

		// Select a daemon that supports this agent's model provider
		daemon := s.dm.SelectDaemon("llm")
		if daemon == nil {
			slog.Warn("no available daemon for agent task", "agent_id", ag.ID)
			s.broadcastAgentError("", channelID, ag.ID, ag.Name, agentErrorNoAvailableDaemon)
			continue
		}

		slog.Debug("triggering agent response",
			"agent_id", ag.ID,
			"agent_name", ag.Name,
			"channel_id", channelID,
			"message_id", messageID,
			"daemon_id", daemon.ID,
		)

		// Build the task request
		taskReq := daemonTaskRequest{
			TaskID:           uuid.New().String(),
			AgentID:          ag.ID,
			ChannelID:        channelID,
			TriggerMessageID: messageID,
			Messages:         contextMessages,
			SystemPrompt:     ag.SystemPrompt,
			ModelConfig: agent.ModelConfig{
				Provider: ag.ModelProvider,
				Model:    ag.ModelName,
			},
			TaskContext:    taskContext,
			AgentChain:     newChain,
			MentionedNames: mentionedNames,
		}

		// Dispatch via streaming SSE and handle events
		go s.handleStreamingAgentTask(context.Background(), daemon, taskReq, ag)
	}
}

// TriggerAgentResponseInNode dispatches an isolated Thinking node turn. Raw
// messages stay in the node; only Agent-authored Handoffs cross boundaries.
func (s *AgentService) TriggerAgentResponseInNode(ctx context.Context, channelID, nodeID, messageID, senderType, senderID string, mentionedAgentIDs []string, hasMentions bool, agentChain []string) {
	_ = s.triggerAgentResponseInNode(ctx, channelID, nodeID, messageID, senderType, senderID, mentionedAgentIDs, hasMentions, agentChain, "", false)
}

const thinkingReturnPrompt = `Complete this Thinking branch by producing its final handoff for the parent.
Use the full context already present in this exact local Agent session. Do not create branches or continue implementation work.
Send exactly one protocol message with solo message send. Its first line must be [[handoff:return]], followed by concise Markdown under 1500 words using these exact headings:
# Handoff
## Objective and scope
## Confirmed conclusions
## Evidence or artifacts
## Unresolved questions
## Risks and assumptions
## Recommended parent action`

// TriggerThinkingNodeReturn uses the node's existing Agent and provider Session
// for the final handoff. The assigned Agent's message seals the node.
func (s *AgentService) TriggerThinkingNodeReturn(ctx context.Context, channelID, nodeID string) error {
	return s.triggerAgentResponseInNode(ctx, channelID, nodeID, "", "system", "system", nil, false, nil, thinkingReturnPrompt, true)
}

func (s *AgentService) TriggerThinkingForkHandoff(ctx context.Context, channelID, parentID, childID, childTitle string) error {
	prompt := fmt.Sprintf(`Prepare the initial Handoff for the newly split child branch %q.
Use the full context already present in this exact parent Agent Session. Do not continue implementation work.
Send exactly one protocol message with solo message send. Its first line must be [[handoff:fork:%s]], followed by concise Markdown under 1000 words using these exact headings:
# Handoff
## Objective and scope
## Confirmed conclusions
## Evidence or artifacts
## Unresolved questions
## Risks and assumptions
## Recommended first action`, childTitle, childID)
	return s.triggerAgentResponseInNode(ctx, channelID, parentID, "", "system", "system", nil, false, nil, prompt, false)
}

const thinkingCheckpointRefreshPrompt = `Publish a fresh Current State for this Thinking branch.
Use the full context already present in this exact local Agent session. Do not continue implementation work and do not send a visible conversational reply.
Preserve concrete identifiers, decisions, evidence, and unresolved dependencies that another branch needs to act correctly.
Send exactly one protocol message with solo message send. Its first line must be [[handoff:checkpoint]], followed by concise Markdown under 1500 words using these exact headings:
# Handoff
## Objective and scope
## Confirmed conclusions
## Evidence or artifacts
## Unresolved questions
## Risks and assumptions
## Recommended next action`

func (s *AgentService) TriggerThinkingCheckpointRefresh(ctx context.Context, channelID, nodeID string) error {
	return s.triggerAgentResponseInNode(ctx, channelID, nodeID, "", "system", "system", nil, false, nil, thinkingCheckpointRefreshPrompt, false)
}

func (s *AgentService) triggerAgentResponseInNode(ctx context.Context, channelID, nodeID, messageID, senderType, senderID string, mentionedAgentIDs []string, hasMentions bool, agentChain []string, handoffPrompt string, returnHandoff bool) error {
	handoffRun := handoffPrompt != ""
	// Agent messages are the output of the node's owning runtime. Mentions in
	// that output provide context or create splits; they must not start another
	// turn in the same node.
	if !handoffRun && senderType == "agent" {
		return nil
	}
	if !handoffRun && !shouldTriggerAgentForSender(senderType, mentionedAgentIDs) {
		return nil
	}
	agents, err := s.getChannelActiveAgents(ctx, channelID)
	if err != nil {
		slog.Error("failed to get agents for thinking node", "node_id", nodeID, "error", err)
		return err
	}
	nodeCtx, err := s.getThinkingNodeRuntimeContext(ctx, channelID, nodeID)
	if err != nil {
		slog.Error("failed to build thinking node context", "node_id", nodeID, "error", err)
		return err
	}
	// A node has one stable owning Agent. Mentions remain conversational
	// context and must not silently replace the node's runtime identity.
	targets := filterAgentsByID(agents, []string{nodeCtx.AgentID})
	if len(targets) == 0 {
		return errors.New("assigned Thinking Agent is unavailable")
	}

	coldStartMessages, err := s.getRecentMessagesForNode(ctx, channelID, nodeID, defaultNodeContextMessageCount)
	if err != nil {
		slog.Error("failed to load thinking node messages", "node_id", nodeID, "error", err)
		return err
	}
	turnMessages := coldStartMessages
	if len(turnMessages) > 1 {
		turnMessages = turnMessages[len(turnMessages)-1:]
	}
	if handoffRun {
		handoffMessage := agent.Message{Role: agent.RoleUser, Content: handoffPrompt, SenderID: "system"}
		turnMessages = []agent.Message{handoffMessage}
		coldStartMessages = append(coldStartMessages, handoffMessage)
	}
	if nodeCtx.TurnContext != "" {
		contextMessage := agent.Message{
			Role:     agent.RoleUser,
			Content:  "[Solo Thinking branch context; Agent-authored Handoffs only]\n" + nodeCtx.TurnContext,
			SenderID: "system",
		}
		turnMessages = append([]agent.Message{contextMessage}, turnMessages...)
		coldStartMessages = append([]agent.Message{contextMessage}, coldStartMessages...)
	}
	mentionedNames := s.resolveMentionedNames(ctx, mentionedAgentIDs)
	dispatched := false
	for _, ag := range targets {
		debounceScope := "node:" + nodeID
		if !handoffRun && s.checkThreadDebounce(channelID, debounceScope, ag.ID) {
			continue
		}
		s.updateThreadDebounce(channelID, debounceScope, ag.ID)
		if len(agentChain) >= maxAgentChainDepth || containsStr(agentChain, ag.ID) {
			continue
		}
		newChain := append(append([]string(nil), agentChain...), ag.ID)
		daemon := s.dm.SelectDaemon("llm")
		if daemon == nil {
			s.broadcastAgentError("", channelID, ag.ID, ag.Name, agentErrorNoAvailableDaemon)
			if handoffRun {
				return errors.New("no available local Agent daemon")
			}
			continue
		}
		taskReq := daemonTaskRequest{
			TaskID:                uuid.NewString(),
			AgentID:               ag.ID,
			ChannelID:             channelID,
			NodeID:                nodeID,
			TriggerMessageID:      messageID,
			Messages:              turnMessages,
			ColdStartMessages:     coldStartMessages,
			SystemPrompt:          ag.SystemPrompt,
			ThinkingRuntimePrompt: nodeCtx.StaticPrompt,
			ModelConfig:           agent.ModelConfig{Provider: ag.ModelProvider, Model: ag.ModelName},
			AgentChain:            newChain,
			MentionedNames:        mentionedNames,
			ResumeSessionID:       nodeCtx.ResumeSessionID,
			ReturnHandoff:         returnHandoff,
		}
		go s.handleStreamingAgentTask(context.Background(), daemon, taskReq, ag)
		dispatched = true
	}
	if handoffRun && !dispatched {
		return errors.New("failed to dispatch Thinking handoff")
	}
	return nil
}

type thinkingNodeRuntimeContext struct {
	AgentID         string
	StaticPrompt    string
	TurnContext     string
	ResumeSessionID string
}

func (s *AgentService) getThinkingNodeRuntimeContext(ctx context.Context, channelID, nodeID string) (thinkingNodeRuntimeContext, error) {
	var title, inherited, parentCheckpoint string
	var hasTeamChildren bool
	var result thinkingNodeRuntimeContext
	err := s.pool.QueryRow(ctx, `
		SELECT n.title, COALESCE(n.agent_id::text, ''), n.inherited_handoff,
		       COALESCE(parent.checkpoint_handoff, ''), COALESCE(sess.external_session_id, ''),
		       EXISTS (SELECT 1 FROM thinking_nodes team WHERE team.parent_id = n.id AND team.source = 'team')
		  FROM thinking_nodes n
		  JOIN thinking_spaces space ON space.id = n.space_id AND space.channel_id = $1
		  LEFT JOIN thinking_nodes parent ON parent.id = n.parent_id
		  LEFT JOIN agent_sessions sess ON sess.id = n.agent_session_id AND sess.agent_id = n.agent_id
		 WHERE n.id = $2`, channelID, nodeID,
	).Scan(&title, &result.AgentID, &inherited, &parentCheckpoint, &result.ResumeSessionID, &hasTeamChildren)
	if err != nil {
		return thinkingNodeRuntimeContext{}, err
	}
	siblings, err := s.thinkingHandoffs(ctx, `
		SELECT sibling.title,
		       CASE
		         WHEN sibling.returned_at IS NOT NULL THEN
		           '[final Handoff]' || CASE WHEN sibling.returned_handoff = '' THEN ' unavailable' ELSE E'\n' || sibling.returned_handoff END
		         WHEN sibling.checkpoint_handoff = '' OR sibling.checkpoint_handoff_at IS NULL THEN
		           '[active; Current State not published]'
		         WHEN EXISTS (
		           SELECT 1 FROM messages latest
		            WHERE latest.thinking_node_id = sibling.id
		              AND latest.sender_type = 'agent'
		              AND COALESCE(latest.is_deleted, false) = false
		              AND latest.created_at > sibling.checkpoint_handoff_at
		         ) THEN '[active; Current State may need refresh]' || E'\n' || sibling.checkpoint_handoff
		         ELSE '[active; Current State current]' || E'\n' || sibling.checkpoint_handoff
		       END
		  FROM thinking_nodes current
		  JOIN thinking_nodes sibling ON sibling.parent_id = current.parent_id AND sibling.id <> current.id
		 WHERE current.id = $1
		 ORDER BY sibling.sort_order`, nodeID)
	if err != nil {
		return thinkingNodeRuntimeContext{}, err
	}
	returned, err := s.thinkingHandoffs(ctx, `
		SELECT child.title, child.returned_handoff
		  FROM thinking_nodes child
		 WHERE child.parent_id = $1 AND child.returned_handoff <> ''
		 ORDER BY child.returned_at DESC NULLS LAST LIMIT 8`, nodeID)
	if err != nil {
		return thinkingNodeRuntimeContext{}, err
	}
	var staticPrompt strings.Builder
	fmt.Fprintf(&staticPrompt, "You are working inside the isolated Thinking node %q (node_id=%s).\n", title, nodeID)
	staticPrompt.WriteString("Only this node's raw conversation is available. Keep this branch focused. Reply with `solo message send` as usual; Solo routes it back to this node automatically.\n")
	if hasTeamChildren {
		staticPrompt.WriteString("The root's first-level team branches already exist. Do not emit split directives from this node; continue in the appropriate team branch instead.\n")
	} else {
		staticPrompt.WriteString("If the discussion reveals a distinct durable workstream, add at most three lines in the exact form `[[split: Branch title]]` inside the message sent with `solo message send`; Solo removes the markers and creates child nodes. Do not split for ordinary follow-up questions.\n")
	}
	staticPrompt.WriteString("Publish this branch's outward Current State after the first meaningful visible reply. Update it only when cross-node-relevant objective, conclusions, evidence, risks, dependencies, unresolved work, or next action materially change. To publish, send one additional protocol message after the visible reply. Its first line must be `[[handoff:checkpoint]]`, followed by `# Handoff` with Objective and scope, Confirmed conclusions, Evidence or artifacts, Unresolved questions, Risks and assumptions, and Recommended next action. This protocol message is hidden from conversation. Current State is a durable handoff, not a mechanical summary or copy of the last message; do not publish it for ordinary follow-up or tool noise.\n")
	result.StaticPrompt = staticPrompt.String()

	var turnContext strings.Builder
	if inherited != "" {
		fmt.Fprintf(&turnContext, "Fork Handoff prepared for this node:\n%s\n", inherited)
	} else if parentCheckpoint != "" {
		fmt.Fprintf(&turnContext, "Parent checkpoint Handoff:\n%s\n", parentCheckpoint)
	}
	if len(siblings) > 0 {
		turnContext.WriteString("\nSibling branch awareness (do not treat as raw dialogue):\n")
		turnContext.WriteString(strings.Join(siblings, "\n"))
		turnContext.WriteByte('\n')
	}
	if len(returned) > 0 {
		turnContext.WriteString("\nHandoffs returned by child nodes:\n")
		turnContext.WriteString(strings.Join(returned, "\n"))
		turnContext.WriteByte('\n')
	}
	result.TurnContext = strings.TrimSpace(turnContext.String())
	return result, nil
}

func (s *AgentService) thinkingHandoffs(ctx context.Context, query, nodeID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, query, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []string{}
	for rows.Next() {
		var title, handoff string
		if err := rows.Scan(&title, &handoff); err != nil {
			return nil, err
		}
		result = append(result, "- "+title+": "+handoff)
	}
	return result, rows.Err()
}

func (s *AgentService) routeChannelTargets(ctx context.Context, agents []agentChannelInfo, mentionedAgentIDs []string, hasMentions bool) []agentChannelInfo {
	if len(mentionedAgentIDs) > 0 || hasMentions {
		return filterAgentsByID(agents, selectWakeAgentIDs(wakeRouteInput{
			ActiveIDs:    agentIDs(agents),
			MentionedIDs: mentionedAgentIDs,
			HasMention:   hasMentions,
		}))
	}
	return s.chooseCoordinator(ctx, agents)
}

func filterAgentsByID(agents []agentChannelInfo, ids []string) []agentChannelInfo {
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	out := make([]agentChannelInfo, 0, len(ids))
	for _, ag := range agents {
		if idSet[ag.ID] {
			out = append(out, ag)
		}
	}
	return out
}

type relationshipEdge struct {
	from string
	to   string
}

func (s *AgentService) chooseCoordinator(ctx context.Context, agents []agentChannelInfo) []agentChannelInfo {
	if len(agents) <= 1 {
		return agents
	}

	rows, err := s.pool.Query(ctx, `
		SELECT from_agent_id::text, to_agent_id::text
		  FROM agent_relationships
		 WHERE rel_type = 'assigns_to'
	`)
	if err == nil {
		defer rows.Close()
		edges := []relationshipEdge{}
		for rows.Next() {
			var fromID, toID string
			if scanErr := rows.Scan(&fromID, &toID); scanErr != nil {
				continue
			}
			edges = append(edges, relationshipEdge{from: fromID, to: toID})
		}
		if chosen := chooseCoordinatorFromEdges(agents, edges); len(chosen) > 0 {
			return chosen
		}
	}

	return agents[:1]
}

func chooseCoordinatorFromEdges(agents []agentChannelInfo, edges []relationshipEdge) []agentChannelInfo {
	return filterAgentsByID(agents, selectWakeAgentIDs(wakeRouteInput{
		ActiveIDs: agentIDs(agents),
		Edges:     edges,
	}))
}

func agentIDs(agents []agentChannelInfo) []string {
	ids := make([]string, 0, len(agents))
	for _, ag := range agents {
		ids = append(ids, ag.ID)
	}
	return ids
}

// --- Broadcast dispatch helpers ---
// These route to thread-scoped or channel-scoped broadcasts based on whether
// a ThreadID is present (P25-05-B).

func (s *AgentService) broadcastAgentThinking(threadID, channelID, agentID, agentName, thought string) {
	if threadID != "" {
		s.broadcastThreadThinking(threadID, channelID, agentID, agentName, thought)
	} else {
		s.broadcastThinking(channelID, agentID, agentName, thought)
	}
}

func (s *AgentService) broadcastAgentError(threadID, channelID, agentID, agentName, errMsg string) {
	if threadID != "" {
		s.broadcastThreadError(threadID, channelID, agentID, agentName, errMsg)
	} else {
		s.broadcastError(channelID, agentID, agentName, errMsg)
	}
}

const (
	agentTaskStreamTimeout = 20 * time.Minute
	agentRunStaleAfter     = agentTaskStreamTimeout + agentRunWatchdogInterval
)

// handleStreamingAgentTask dispatches a task to a daemon via SSE streaming
// and forwards events to WebSocket subscribers.
func (s *AgentService) handleStreamingAgentTask(ctx context.Context, daemon *DaemonInfo, taskReq daemonTaskRequest, ag agentChannelInfo) {
	if taskReq.ReturnHandoff {
		defer func() {
			cleared, err := NewThinkingService(s.pool).CancelReturn(context.Background(), taskReq.NodeID)
			if err != nil {
				slog.Warn("failed to release Thinking return lock", "node_id", taskReq.NodeID, "error", err)
				return
			}
			if cleared && s.hub != nil {
				payload, _ := json.Marshal(map[string]any{
					"type":       "thinking.updated",
					"channel_id": taskReq.ChannelID,
					"node_id":    taskReq.NodeID,
				})
				s.hub.BroadcastToChannel(taskReq.ChannelID, payload)
			}
		}()
	}
	// Get agent display name
	agentName := ag.Name
	if agentName == "" {
		agentName = "Agent"
	}

	// Track this task for daemon offline cleanup (cleanup on return)
	s.dm.TrackTask(taskReq.TaskID, daemon.ID, ag.ID)
	defer s.dm.RemoveTask(taskReq.TaskID)

	// Use a timeout context so tasks don't hang indefinitely (e.g., LLM API hang).
	streamCtx, streamCancel := context.WithTimeout(ctx, agentTaskStreamTimeout)
	defer streamCancel()

	// Pre-generate a message ID for the streaming message
	streamMessageID := uuid.New().String()
	runSvc := NewAgentRunService(s.pool)
	triggerType := AgentRunTriggerMessage
	if taskReq.OriginTaskID != "" {
		triggerType = AgentRunTriggerTask
	}
	run, err := runSvc.StartRun(ctx, StartRunInput{
		AgentID:          ag.ID,
		TriggerType:      triggerType,
		TriggerMessageID: taskReq.TriggerMessageID,
		ChannelID:        taskReq.ChannelID,
		ThreadID:         taskReq.ThreadID,
		ThinkingNodeID:   taskReq.NodeID,
		Status:           AgentRunStatusQueued,
		ActivityText:     "等待执行",
		Source:           taskReq.ModelConfig.Provider,
	})
	if err != nil {
		slog.Warn("failed to start agent run", "task_id", taskReq.TaskID, "agent_id", ag.ID, "error", err)
	}
	if run != nil {
		s.dm.AttachTaskRun(taskReq.TaskID, run.ID)
	}
	if run != nil && taskReq.OriginTaskID != "" {
		if err := runSvc.LinkTask(ctx, LinkRunTaskInput{RunID: run.ID, TaskID: taskReq.OriginTaskID, Role: AgentRunTaskRolePrimary, Confidence: 1}); err != nil {
			slog.Warn("failed to link agent run task", "run_id", run.ID, "task_id", taskReq.OriginTaskID, "error", err)
		} else {
			s.appendAndBroadcastRunEvent(ctx, runSvc, run, ag.ID, agentName, AgentRunEventTaskLinked, "关联 task", "", map[string]any{
				"task_id":    taskReq.OriginTaskID,
				"role":       AgentRunTaskRolePrimary,
				"confidence": 1,
			})
		}
	}
	if run != nil {
		s.broadcastAgentRun(taskReq.ChannelID, "agent.run.started", runPayload(run, ag.ID, agentName, taskReq.OriginTaskID))
		if taskReq.TriggerMessageID != "" {
			s.appendAndBroadcastRunEvent(ctx, runSvc, run, ag.ID, agentName, AgentRunEventUserMessageReceived, "用户消息触发 run", "", map[string]any{
				"message_id": taskReq.TriggerMessageID,
				"channel_id": taskReq.ChannelID,
				"thread_id":  taskReq.ThreadID,
			})
		}
		s.appendAndBroadcastRunEvent(ctx, runSvc, run, ag.ID, agentName, AgentRunEventRunStarted, "创建 run", "", map[string]any{
			"trigger_type":       triggerType,
			"trigger_message_id": taskReq.TriggerMessageID,
			"task_id":            taskReq.OriginTaskID,
		})
		if updated, err := runSvc.UpdateStatus(ctx, UpdateRunStatusInput{
			RunID:        run.ID,
			Status:       AgentRunStatusThinking,
			ActivityText: agentActivityAccepted,
			Source:       taskReq.ModelConfig.Provider,
		}); err != nil {
			slog.Warn("failed to acknowledge agent run", "run_id", run.ID, "error", err)
		} else {
			run = updated
			s.appendAndBroadcastRunEvent(ctx, runSvc, run, ag.ID, agentName, AgentRunEventActivity, agentActivityAccepted, "", map[string]any{
				"status": AgentRunStatusThinking,
			})
			s.broadcastAgentRun(taskReq.ChannelID, "agent.run.updated", runPayload(run, ag.ID, agentName, taskReq.OriginTaskID))
		}
	}

	slog.Debug("dispatching agent streaming task",
		"task_id", taskReq.TaskID,
		"agent_id", ag.ID,
		"daemon_id", daemon.ID,
		"channel_id", taskReq.ChannelID,
	)

	// Inject InitialGreeting as a system message if set (e.g. Lucy's onboarding greeting).
	// This is private context for the agent — not stored as a channel message.
	if taskReq.InitialGreeting != "" {
		taskReq.Messages = append([]agent.Message{{
			Role:     agent.RoleSystem,
			Content:  taskReq.InitialGreeting,
			SenderID: "system",
		}}, taskReq.Messages...)
	}

	// Broadcast thinking event immediately
	s.broadcastAgentThinking(taskReq.ThreadID, taskReq.ChannelID, ag.ID, agentName, "Processing request...")

	// Broadcast user trigger message as context for agent view
	if len(taskReq.Messages) > 0 {
		lastMsg := taskReq.Messages[len(taskReq.Messages)-1]
		s.broadcastAgentChunk(taskReq.ThreadID, taskReq.ChannelID, ag.ID, agentName, "context", lastMsg.Content, nil)
	}

	finishRun := func(status AgentRunStatus) {
		if run == nil {
			return
		}
		current, err := runSvc.GetRun(ctx, run.ID)
		if err == nil && !isActiveAgentRunStatus(current.Status) {
			run = current
			return
		}
		finished, err := runSvc.FinishRun(ctx, FinishRunInput{
			RunID:        run.ID,
			Status:       status,
			ActivityText: finalStateActivityText(status),
		})
		if err != nil {
			slog.Warn("failed to finish agent run", "run_id", run.ID, "status", status, "error", err)
			finished = run
			finished.Status = status
			finished.ActivityText = finalStateActivityText(status)
		}
		eventType := AgentRunEventDone
		if status != AgentRunStatusCompleted {
			eventType = AgentRunEventError
		}
		s.appendAndBroadcastRunEvent(ctx, runSvc, finished, ag.ID, agentName, eventType, finalStateActivityText(status), "", map[string]any{
			"status": status,
		})
		s.broadcastAgentRun(taskReq.ChannelID, "agent.run.finished", runPayload(finished, ag.ID, agentName, taskReq.OriginTaskID))
	}

	// Send via SSE streaming
	eventCh, err := s.dm.StreamTask(streamCtx, daemon, taskReq)
	if err != nil {
		slog.Error("failed to stream agent task",
			"task_id", taskReq.TaskID,
			"agent_id", ag.ID,
			"daemon_id", daemon.ID,
			"error", err,
		)
		s.broadcastAgentError(taskReq.ThreadID, taskReq.ChannelID, ag.ID, agentName, err.Error())
		finishRun(AgentRunStatusFailed)
		return
	}

	// Track usage for logging
	var inputTokens, outputTokens int
	taskCompleted := false
	// Track final state for the run finished broadcast. Updated by event handlers
	// and read by the deferred broadcaster. Defaults to "aborted" if the stream
	// ends without a more specific state.
	finalState := "aborted"
	bindRunSession := func(externalSessionID, transcriptPath string) {
		if run == nil || (externalSessionID == "" && transcriptPath == "") {
			return
		}
		session, err := runSvc.UpsertSession(ctx, UpsertSessionInput{
			AgentID:           ag.ID,
			Provider:          taskReq.ModelConfig.Provider,
			ExternalSessionID: externalSessionID,
			TranscriptPath:    transcriptPath,
		})
		if err != nil {
			slog.Warn("failed to upsert agent session", "run_id", run.ID, "agent_id", ag.ID, "error", err)
			return
		}
		updatedRun, err := runSvc.BindRunSession(ctx, BindRunSessionInput{
			RunID:          run.ID,
			SessionID:      session.ID,
			ThinkingNodeID: taskReq.NodeID,
		})
		if err != nil {
			slog.Warn("failed to bind agent run session", "run_id", run.ID, "session_id", session.ID, "error", err)
			return
		}
		run = updatedRun
		if transcriptPath != "" {
			updatedRun, err = runSvc.UpdateRunTranscript(ctx, UpdateRunTranscriptInput{
				RunID:          run.ID,
				TranscriptPath: transcriptPath,
			})
			if err != nil {
				slog.Warn("failed to update agent run transcript", "run_id", run.ID, "error", err)
				return
			}
			run = updatedRun
		}
		s.broadcastAgentRun(taskReq.ChannelID, "agent.run.updated", runPayload(run, ag.ID, agentName, taskReq.OriginTaskID))
	}

	defer func() {
		finishRun(finalStateToRunStatus(finalState))
	}()

	for event := range eventCh {
		switch event.Event {
		case "thinking":
			var data struct {
				AgentID string `json:"agent_id"`
				Thought string `json:"thought"`
			}
			if err := json.Unmarshal([]byte(event.Data), &data); err == nil {
				if run != nil {
					s.appendAndBroadcastRunEvent(ctx, runSvc, run, ag.ID, agentName, AgentRunEventThinking, data.Thought, "", nil)
				}
				s.broadcastAgentThinking(taskReq.ThreadID, taskReq.ChannelID, ag.ID, agentName, data.Thought)
				s.broadcastAgentChunk(taskReq.ThreadID, taskReq.ChannelID, ag.ID, agentName, "thinking", data.Thought, nil)
			}

		case "text":
			var data struct {
				AgentID   string `json:"agent_id"`
				AgentName string `json:"agent_name"`
				Content   string `json:"content"`
			}
			if err := json.Unmarshal([]byte(event.Data), &data); err == nil {
				if run != nil {
					s.appendAndBroadcastRunEvent(ctx, runSvc, run, ag.ID, agentName, AgentRunEventAssistantMessage, data.Content, "", nil)
				}
				s.broadcastAgentChunk(taskReq.ThreadID, taskReq.ChannelID, ag.ID, agentName, "text", data.Content, nil)
			}

		case "tool_use":
			var data struct {
				AgentID   string `json:"agent_id"`
				AgentName string `json:"agent_name"`
				ToolName  string `json:"tool_name"`
				ToolInput string `json:"tool_input"`
				CallID    string `json:"call_id"`
			}
			if err := json.Unmarshal([]byte(event.Data), &data); err == nil {
				if run != nil {
					s.appendAndBroadcastRunEvent(ctx, runSvc, run, ag.ID, agentName, AgentRunEventToolStarted, data.ToolName, data.ToolName, map[string]any{
						"input":   data.ToolInput,
						"call_id": data.CallID,
					})
				}
				s.broadcastAgentChunk(taskReq.ThreadID, taskReq.ChannelID, ag.ID, agentName, "tool_use", "", map[string]interface{}{
					"name":    data.ToolName,
					"input":   data.ToolInput,
					"call_id": data.CallID,
				})
			}

		case "tool_result":
			var data struct {
				AgentID   string `json:"agent_id"`
				AgentName string `json:"agent_name"`
				ToolName  string `json:"tool_name"`
				Output    string `json:"output"`
				CallID    string `json:"call_id"`
				IsError   bool   `json:"is_error"`
			}
			if err := json.Unmarshal([]byte(event.Data), &data); err == nil {
				if run != nil {
					s.appendAndBroadcastRunEvent(ctx, runSvc, run, ag.ID, agentName, AgentRunEventToolFinished, data.Output, data.ToolName, map[string]any{
						"output":   data.Output,
						"call_id":  data.CallID,
						"is_error": data.IsError,
					})
				}
				s.broadcastAgentChunk(taskReq.ThreadID, taskReq.ChannelID, ag.ID, agentName, "tool_result", data.Output, map[string]interface{}{
					"name":    data.ToolName,
					"output":  data.Output,
					"call_id": data.CallID,
				})
			}

		case "session":
			var data struct {
				ExternalSessionID string `json:"external_session_id"`
				TranscriptPath    string `json:"transcript_path"`
			}
			if err := json.Unmarshal([]byte(event.Data), &data); err == nil {
				bindRunSession(data.ExternalSessionID, data.TranscriptPath)
			}

		case "complete":
			var data struct {
				ExternalSessionID string `json:"external_session_id"`
				TranscriptPath    string `json:"transcript_path"`
				Usage             struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal([]byte(event.Data), &data); err == nil {
				bindRunSession(data.ExternalSessionID, data.TranscriptPath)
				inputTokens = data.Usage.InputTokens
				outputTokens = data.Usage.OutputTokens
				taskCompleted = true
				finalState = "completed"
			}

		case "error":
			var data struct {
				AgentID string `json:"agent_id"`
				Error   string `json:"error"`
				Status  string `json:"status"`
			}
			if err := json.Unmarshal([]byte(event.Data), &data); err == nil {
				slog.Error("agent task stream error",
					"agent_id", ag.ID,
					"error", data.Error,
					"status", data.Status,
				)
				s.broadcastAgentError(taskReq.ThreadID, taskReq.ChannelID, ag.ID, agentName, data.Error)
				finalState = finalStateFromErrorEventStatus(data.Status)
			} else {
				finalState = "failed"
			}
			// Don't return — let the loop consume the "done" sentinel. The
			// deferred run finisher will handle the exit.
			continue

		case "done":
			// Daemon's stream-end sentinel (cmd/daemon/handler.go:683). No
			// payload — we already captured the final state from "complete"
			// or "error" above. Just exit the loop; deferred run finisher
			// will broadcast agent.run.finished.

		case "agent.run.updated":
			if taskReq.ChannelID == "" {
				slog.Debug("agent.run.updated: skipping empty channel_id", "task_id", taskReq.TaskID)
				continue
			}
			if run == nil {
				continue
			}
			var activity struct {
				Status           string `json:"status"`
				ActivityText     string `json:"activity_text"`
				ToolName         string `json:"tool_name"`
				ToolInputSummary string `json:"tool_input_summary"`
				Source           string `json:"source"`
			}
			if err := json.Unmarshal([]byte(event.Data), &activity); err != nil {
				slog.Warn("agent.run.updated: failed to unmarshal payload",
					"task_id", taskReq.TaskID, "error", err)
				continue
			}
			updated, err := runSvc.UpdateStatus(ctx, UpdateRunStatusInput{
				RunID:            run.ID,
				Status:           AgentRunStatus(activity.Status),
				ActivityText:     activity.ActivityText,
				ToolName:         activity.ToolName,
				ToolInputSummary: activity.ToolInputSummary,
				Source:           activity.Source,
			})
			if err != nil {
				slog.Warn("agent.run.updated: failed to update run", "run_id", run.ID, "status", activity.Status, "error", err)
				continue
			}
			run = updated
			s.broadcastAgentRun(taskReq.ChannelID, "agent.run.updated", runPayload(updated, ag.ID, agentName, taskReq.OriginTaskID))
		}

		// Break on stream-end sentinel (empty "done" event).
		if event.Event == "done" {
			break
		}
	}
	if errors.Is(streamCtx.Err(), context.DeadlineExceeded) {
		finalState = "timeout"
	}

	// If task was not completed (no "complete" event arrived), emit a soft
	// warning. We don't return here because the deferred run finisher still
	// needs to run. finalState is already set ("completed" | "failed" |
	// "aborted" default) for the deferred broadcaster.
	if !taskCompleted && finalState == "aborted" {
		slog.Warn("agent task stream ended without complete event",
			"agent_id", ag.ID,
			"channel_id", taskReq.ChannelID,
		)
		s.broadcastAgentThinking(taskReq.ThreadID, taskReq.ChannelID, ag.ID, agentName, "Response ended unexpectedly.")
	}

	// v1.3: dual-channel architecture.
	// Agent text output is internal thinking (streamed nowhere).
	// Real messages arrive via solo message send → daemon proxy → server API → message.new.
	// The complete event here is purely for usage tracking and status notification.
	slog.Info("agent streaming task completed",
		"agent_id", ag.ID,
		"channel_id", taskReq.ChannelID,
		"message_id", streamMessageID,
		"input_tokens", inputTokens,
		"output_tokens", outputTokens,
	)
}

// --- Internal broadcast helpers (use realtime.Broadcaster directly) ---

func stringOrEmpty(session *AgentSession) string {
	if session == nil {
		return ""
	}
	return session.ID
}

func runPayload(run *AgentRun, agentID, agentName, taskID string) map[string]any {
	return map[string]any{
		"run_id":             run.ID,
		"session_id":         run.SessionID,
		"agent_id":           agentID,
		"agent_name":         agentName,
		"task_id":            taskID,
		"channel_id":         run.ChannelID,
		"thread_id":          run.ThreadID,
		"thinking_node_id":   run.ThinkingNodeID,
		"status":             run.Status,
		"activity_text":      run.ActivityText,
		"tool_name":          run.ToolName,
		"tool_input_summary": run.ToolInputSummary,
		"transcript_path":    run.TranscriptPath,
		"source":             run.Source,
		"timestamp":          time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *AgentService) broadcastAgentRun(_ string, eventType string, payload map[string]any) {
	s.hub.Broadcast(realtime.Envelope(eventType, payload))
}

func (s *AgentService) appendAndBroadcastRunEvent(ctx context.Context, runSvc *AgentRunService, run *AgentRun, agentID, agentName, eventType, message, toolName string, payload any) {
	if run == nil {
		return
	}
	event, err := runSvc.AppendEvent(ctx, AppendRunEventInput{
		RunID:    run.ID,
		Type:     eventType,
		Message:  message,
		ToolName: toolName,
		Payload:  payload,
	})
	if err != nil {
		slog.Warn("failed to append agent run event", "run_id", run.ID, "event_type", eventType, "error", err)
		return
	}
	s.hub.Broadcast(realtime.Envelope("agent.run.event", map[string]any{
		"id":               event.ID,
		"run_id":           run.ID,
		"session_id":       run.SessionID,
		"agent_id":         agentID,
		"agent_name":       agentName,
		"channel_id":       run.ChannelID,
		"thread_id":        run.ThreadID,
		"thinking_node_id": run.ThinkingNodeID,
		"seq":              event.Seq,
		"event_type":       event.Type,
		"message":          event.Message,
		"tool_name":        event.ToolName,
		"payload":          json.RawMessage(event.Payload),
		"timestamp":        event.CreatedAt.UTC().Format(time.RFC3339),
	}))
}

func (s *AgentService) StartAgentRunWatchdogLoop(ctx context.Context) {
	if err := s.CheckAgentRunWatchdogs(ctx, time.Now()); err != nil {
		slog.Warn("agent run watchdog failed", "error", err)
	}

	ticker := time.NewTicker(agentRunWatchdogInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := s.CheckAgentRunWatchdogs(ctx, time.Now()); err != nil {
				slog.Warn("agent run watchdog failed", "error", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *AgentService) CheckAgentRunWatchdogs(ctx context.Context, now time.Time) error {
	runSvc := NewAgentRunService(s.pool)
	staleRuns, err := s.listStaleActiveRuns(ctx, now.Add(-agentRunStaleAfter))
	if err != nil {
		return err
	}
	for i := range staleRuns {
		if err := s.timeoutStaleAgentRun(ctx, runSvc, &staleRuns[i]); err != nil {
			return err
		}
	}

	noVisibleRuns, err := s.listRunsWithoutVisibleReply(ctx, now.Add(-agentNoVisibleReplyAfter))
	if err != nil {
		return err
	}
	for i := range noVisibleRuns {
		if err := s.warnAgentRun(ctx, runSvc, &noVisibleRuns[i], agentRunEventNoVisibleReplyWatchdog, agentActivityNoVisibleReply); err != nil {
			return err
		}
	}

	noProgressRuns, err := s.listRunsWithoutProgress(ctx, now.Add(-agentNoProgressAfter))
	if err != nil {
		return err
	}
	for i := range noProgressRuns {
		if err := s.warnAgentRun(ctx, runSvc, &noProgressRuns[i], agentRunEventNoProgressWatchdog, agentActivityNoProgress); err != nil {
			return err
		}
	}
	return nil
}

func (s *AgentService) listStaleActiveRuns(ctx context.Context, before time.Time) ([]AgentRun, error) {
	return scanAgentRuns(s.pool.Query(ctx, baseAgentRunSelect()+`
		 WHERE r.status = ANY($1)
		   AND r.started_at <= $2
		 ORDER BY r.started_at ASC
		 LIMIT 100`,
		activeAgentRunStatuses(),
		before,
	))
}

func (s *AgentService) listRunsWithoutVisibleReply(ctx context.Context, before time.Time) ([]AgentRun, error) {
	return scanAgentRuns(s.pool.Query(ctx, baseAgentRunSelect()+`
		 WHERE r.status = ANY($1)
		   AND r.started_at <= $2
		   AND NOT EXISTS (
		         SELECT 1 FROM agent_run_events e
		          WHERE e.run_id = r.id AND e.type = $3
		       )
		   AND NOT EXISTS (
		         SELECT 1 FROM agent_run_events e
		          WHERE e.run_id = r.id AND e.type = $4
		       )
		 ORDER BY r.started_at ASC
		 LIMIT 100`,
		activeAgentRunStatuses(),
		before,
		AgentRunEventAssistantMessage,
		agentRunEventNoVisibleReplyWatchdog,
	))
}

func (s *AgentService) listRunsWithoutProgress(ctx context.Context, before time.Time) ([]AgentRun, error) {
	return scanAgentRuns(s.pool.Query(ctx, baseAgentRunSelect()+`
		 WHERE r.status = ANY($1)
		   AND r.updated_at <= $2
		   AND NOT EXISTS (
		         SELECT 1 FROM agent_run_events e
		          WHERE e.run_id = r.id AND e.type = $3
		       )
		 ORDER BY r.updated_at ASC
		 LIMIT 100`,
		activeAgentRunStatuses(),
		before,
		agentRunEventNoProgressWatchdog,
	))
}

func activeAgentRunStatuses() []string {
	return []string{
		string(AgentRunStatusQueued),
		string(AgentRunStatusThinking),
		string(AgentRunStatusRunning),
		string(AgentRunStatusStreaming),
		string(AgentRunStatusWaitingInput),
		string(AgentRunStatusWaitingApproval),
	}
}

func isActiveAgentRunStatus(status AgentRunStatus) bool {
	for _, active := range activeAgentRunStatuses() {
		if string(status) == active {
			return true
		}
	}
	return false
}

func (s *AgentService) timeoutStaleAgentRun(ctx context.Context, runSvc *AgentRunService, run *AgentRun) error {
	finished, err := runSvc.FinishRun(ctx, FinishRunInput{
		RunID:        run.ID,
		Status:       AgentRunStatusTimeout,
		ActivityText: agentActivityTimeout,
	})
	if err != nil {
		return err
	}
	s.appendAndBroadcastRunEvent(ctx, runSvc, finished, finished.AgentID, finished.AgentName, AgentRunEventError, agentActivityTimeout, "", map[string]any{
		"status": AgentRunStatusTimeout,
	})
	s.broadcastAgentRun(finished.ChannelID, "agent.run.finished", runPayload(finished, finished.AgentID, finished.AgentName, ""))
	return nil
}

func (s *AgentService) warnAgentRun(ctx context.Context, runSvc *AgentRunService, run *AgentRun, eventType, message string) error {
	updated, err := runSvc.UpdateStatus(ctx, UpdateRunStatusInput{
		RunID:        run.ID,
		Status:       run.Status,
		ActivityText: message,
		Source:       run.Source,
	})
	if err != nil {
		return err
	}
	s.appendAndBroadcastRunEvent(ctx, runSvc, updated, updated.AgentID, updated.AgentName, eventType, message, "", map[string]any{
		"status": updated.Status,
	})
	s.broadcastAgentRun(updated.ChannelID, "agent.run.updated", runPayload(updated, updated.AgentID, updated.AgentName, ""))
	return nil
}

func finalStateToRunStatus(finalState string) AgentRunStatus {
	switch finalState {
	case "completed":
		return AgentRunStatusCompleted
	case "cancelled":
		return AgentRunStatusCancelled
	case "timeout":
		return AgentRunStatusTimeout
	default:
		return AgentRunStatusFailed
	}
}

func finalStateFromErrorEventStatus(status string) string {
	switch status {
	case "timeout", "cancelled":
		return status
	case "aborted":
		return "cancelled"
	default:
		return "failed"
	}
}

func finalStateActivityText(status AgentRunStatus) string {
	switch status {
	case AgentRunStatusCompleted:
		return agentActivityCompleted
	case AgentRunStatusCancelled:
		return agentActivityCancelled
	case AgentRunStatusTimeout:
		return agentActivityTimeout
	default:
		return agentActivityFailed
	}
}

func (s *AgentService) broadcastThinking(channelID, agentID, agentName, thought string) {
	payload := map[string]interface{}{
		"channel_id": channelID,
		"agent_id":   agentID,
		"agent_name": agentName,
		"thought":    thought,
	}
	s.hub.BroadcastToChannel(channelID, realtime.Envelope("agent.thinking", payload))
}

func (s *AgentService) broadcastError(channelID, agentID, agentName, errMsg string) {
	payload := map[string]interface{}{
		"channel_id": channelID,
		"agent_id":   agentID,
		"agent_name": agentName,
		"error":      errMsg,
	}
	s.hub.BroadcastToChannel(channelID, realtime.Envelope("agent.error", payload))
}

func (s *AgentService) broadcastTaskClaimed(task *Task, channelID string) {
	if task == nil || s.hub == nil {
		return
	}

	dueDate := ""
	if task.DueDate != nil {
		dueDate = task.DueDate.Format(time.RFC3339)
	}

	// Broadcast task.updated
	taskPayload := map[string]interface{}{
		"id":          task.ID,
		"task_number": task.TaskNumber,
		"channel_id":  task.ChannelID,
		"title":       task.Title,
		"description": task.Description,
		"status":      task.Status,
		"claimer_id":  task.ClaimerID,
		"priority":    task.Priority,
		"due_date":    dueDate,
		"message_id":  task.MessageID,
		"updated_at":  task.UpdatedAt.Format(time.RFC3339),
	}
	s.hub.BroadcastToChannel(channelID, realtime.Envelope("task.updated", taskPayload))

	// Broadcast message.updated (for TaskBadge real-time update)
	if task.MessageID != "" {
		msgPayload := map[string]interface{}{
			"id":                task.MessageID,
			"channel_id":        channelID,
			"task_number":       task.TaskNumber,
			"task_status":       task.Status,
			"task_claimer_name": task.ClaimerName,
		}
		msgData, _ := json.Marshal(msgPayload)
		msgEnvelope, _ := json.Marshal(map[string]interface{}{
			"type":    "message.updated",
			"payload": json.RawMessage(msgData),
		})
		s.hub.BroadcastToChannel(channelID, msgEnvelope)
	}

	slog.Debug("broadcast task auto-claim",
		"task_id", task.ID,
		"channel_id", channelID,
		"claimer_id", task.ClaimerID,
	)
}

// parseAndExecuteTaskClaims scans the agent's accumulated output text for
// inline task claim directives and executes them. It returns the list of
// task numbers that were successfully claimed.
//
// Claim syntax: /claim #N or /认领 #N (case-insensitive)
//
// For each match:
//   - Resolves task_number → UUID via the tasks table (scoped to channel)
//   - Calls TaskService.ClaimTask
//   - On success: broadcasts task.updated + message.updated to the channel
//   - On already-claimed (409): logs and continues silently
//   - On not-found: logs and continues

func (s *AgentService) broadcastThreadThinking(threadID, channelID, agentID, agentName, thought string) {
	payload := map[string]interface{}{
		"channel_id": channelID,
		"agent_id":   agentID,
		"agent_name": agentName,
		"thought":    thought,
	}
	data, _ := json.Marshal(payload)
	envelope, _ := json.Marshal(map[string]interface{}{
		"type":    "agent.thinking",
		"payload": json.RawMessage(data),
	})
	s.hub.BroadcastToScope(realtime.ScopeThread, threadID, envelope)
}

func (s *AgentService) broadcastThreadError(threadID, channelID, agentID, agentName, errMsg string) {
	payload := map[string]interface{}{
		"channel_id": channelID,
		"thread_id":  threadID,
		"agent_id":   agentID,
		"agent_name": agentName,
		"error":      errMsg,
	}
	data, _ := json.Marshal(payload)
	envelope, _ := json.Marshal(map[string]interface{}{
		"type":    "agent.error",
		"payload": json.RawMessage(data),
	})
	s.hub.BroadcastToScope(realtime.ScopeThread, threadID, envelope)
}

// broadcastThreadMessage broadcasts final message to thread scope plus
// thread.reply notification to channel subscribers.
func (s *AgentService) broadcastThreadMessage(threadID, channelID, agentID, agentName, messageID, content string) {
	payload := map[string]interface{}{
		"id":           messageID,
		"channel_id":   channelID,
		"thread_id":    threadID,
		"sender_type":  "agent",
		"sender_id":    agentID,
		"sender_name":  agentName,
		"content":      content,
		"content_type": "text",
		"created_at":   time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(payload)
	envelope, _ := json.Marshal(map[string]interface{}{
		"type":    "thread.message.new",
		"payload": json.RawMessage(data),
	})
	s.hub.BroadcastToScope(realtime.ScopeThread, threadID, envelope)

	// Update thread reply_count and read root_message_id for thread.reply broadcast.
	now := time.Now().UTC()
	var rootMessageID string
	var replyCount int
	_, _ = s.pool.Exec(context.Background(),
		`UPDATE threads SET reply_count = reply_count + 1, last_reply_at = $1
		 WHERE id = $2`, now, threadID)
	_ = s.pool.QueryRow(context.Background(),
		`SELECT root_message_id, reply_count FROM threads WHERE id = $1`, threadID,
	).Scan(&rootMessageID, &replyCount)

	// Broadcast thread.reply to channel scope so DM message list updates reply_count.
	replyPayload := map[string]interface{}{
		"thread_id":       threadID,
		"channel_id":      channelID,
		"root_message_id": rootMessageID,
		"reply_count":     replyCount,
		"last_reply_at":   now.Format(time.RFC3339),
	}
	replyData, _ := json.Marshal(replyPayload)
	replyEnvelope, _ := json.Marshal(map[string]interface{}{
		"type":    "thread.reply",
		"payload": json.RawMessage(replyData),
	})
	s.hub.BroadcastToChannel(channelID, replyEnvelope)
}

func (s *AgentService) broadcastAgentChunk(threadID, channelID, agentID, agentName, chunkType, content string, tool map[string]interface{}) {
	if threadID != "" {
		payload := map[string]interface{}{
			"channel_id": channelID,
			"agent_id":   agentID,
			"agent_name": agentName,
			"chunk_type": chunkType,
			"content":    content,
		}
		if tool != nil {
			payload["tool"] = tool
		}
		data, _ := json.Marshal(payload)
		envelope, _ := json.Marshal(map[string]interface{}{
			"type":    "agent.chunk",
			"payload": json.RawMessage(data),
		})
		s.hub.BroadcastToScope(realtime.ScopeThread, threadID, envelope)
	} else {
		payload := map[string]interface{}{
			"channel_id": channelID,
			"agent_id":   agentID,
			"agent_name": agentName,
			"chunk_type": chunkType,
			"content":    content,
		}
		if tool != nil {
			payload["tool"] = tool
		}
		data, _ := json.Marshal(payload)
		envelope, _ := json.Marshal(map[string]interface{}{
			"type":    "agent.chunk",
			"payload": json.RawMessage(data),
		})
		s.hub.BroadcastToChannel(channelID, envelope)
	}
}

func (s *AgentService) HandleTaskComplete(ctx context.Context, req *TaskCompleteRequest) error {
	// Remove from pending task tracking
	s.dm.RemoveTask(req.TaskID)

	var senderName string
	err := s.pool.QueryRow(ctx,
		`SELECT name FROM agents WHERE id = $1`, req.AgentID,
	).Scan(&senderName)
	if err != nil {
		senderName = "Agent"
	}

	// Persist the message to DB (callback path may not go through SSE streaming)
	if req.MessageID != "" && req.Content != "" {
		_, _ = s.pool.Exec(ctx,
			`INSERT INTO messages (id, channel_id, sender_type, sender_id, content, thread_id, created_at, updated_at)
			 VALUES ($1, $2, 'agent', $3, $4, $5, $6, $6)
			 ON CONFLICT (id) DO UPDATE SET content = $4, updated_at = $6`,
			req.MessageID, req.ChannelID, req.AgentID, req.Content, nullableStr(req.ThreadID), time.Now(),
		)
	}

	// P25-05-B: Route broadcast based on thread ID.
	// If thread ID is present, broadcast to thread scope + thread.reply to channel.
	// If thread ID is empty, log error and skip (no fallback to channel).
	if req.ThreadID != "" {
		s.broadcastThreadMessage(req.ThreadID, req.ChannelID, req.AgentID, senderName, req.MessageID, req.Content)
	} else {
		slog.Error("HandleTaskComplete: no thread_id for task response, skipping broadcast",
			"task_id", req.TaskID,
			"agent_id", req.AgentID,
			"channel_id", req.ChannelID,
		)
	}

	slog.Info("agent response broadcast (callback)",
		"task_id", req.TaskID,
		"agent_id", req.AgentID,
		"channel_id", req.ChannelID,
		"message_id", req.MessageID,
	)

	return nil
}

// HandleTaskError processes a task error callback from the daemon.
func (s *AgentService) HandleTaskError(ctx context.Context, req *TaskErrorRequest) error {
	// Remove from pending task tracking
	s.dm.RemoveTask(req.TaskID)

	// Look up agent name for the error broadcast so the frontend can display
	// a meaningful sender name instead of "?".
	agentName := "Agent"
	var dbName string
	err := s.pool.QueryRow(ctx,
		`SELECT name FROM agents WHERE id = $1`, req.AgentID,
	).Scan(&dbName)
	if err == nil && dbName != "" {
		agentName = dbName
	}

	data, _ := json.Marshal(map[string]interface{}{
		"channel_id": req.ChannelID,
		"agent_id":   req.AgentID,
		"agent_name": agentName,
		"error":      req.Error,
	})
	envelope, _ := json.Marshal(map[string]interface{}{
		"type":    "agent.error",
		"payload": json.RawMessage(data),
	})
	s.hub.BroadcastToChannel(req.ChannelID, envelope)

	slog.Error("agent task error", "task_id", req.TaskID, "agent_id", req.AgentID, "error", req.Error)
	return nil
}

// --- Thread trigger ---

// TriggerAgentResponseInThread triggers agent auto-response for a thread message.
func (s *AgentService) TriggerAgentResponseInThread(ctx context.Context, channelID, threadID, senderType, senderID string, mentionedAgentIDs []string, hasMentions bool, agentChain []string) {
	if !shouldTriggerAgentForSender(senderType, mentionedAgentIDs) {
		return
	}

	agents, err := s.getChannelActiveAgents(ctx, channelID)
	if err != nil {
		slog.Error("failed to get channel active agents for thread", "channel_id", channelID, "error", err)
		return
	}

	// Determine target agents for this thread follow-up:
	// - If @mentions provided: only trigger the mentioned agents.
	// - If no @mentions: only trigger agents that have already replied in this thread.
	var targetAgents []agentChannelInfo
	if len(mentionedAgentIDs) > 0 {
		mentionedSet := make(map[string]bool, len(mentionedAgentIDs))
		for _, id := range mentionedAgentIDs {
			mentionedSet[id] = true
		}
		for _, ag := range agents {
			if mentionedSet[ag.ID] {
				targetAgents = append(targetAgents, ag)
			}
		}
	} else if hasMentions {
		// Mentions were present but none resolved to active agents — do not fall back.
		slog.Info("mentions found but none resolved to active agents in thread, skipping", "channel_id", channelID, "thread_id", threadID)
		return
	} else {
		// No @mentions — prefer agents that have already replied in this thread.
		threadAgentIDs, _ := s.getThreadParticipantAgents(ctx, threadID)
		if len(threadAgentIDs) > 0 {
			targetAgents = s.chooseCoordinator(ctx, filterAgentsByID(agents, threadAgentIDs))
		}
		// Fallback: if nobody has replied in this thread yet, trigger the
		// agent whose message was replied to (the root message sender).
		if len(targetAgents) == 0 {
			if rootSenderID := s.getThreadRootAgentSender(ctx, threadID); rootSenderID != "" {
				targetAgents = filterAgentsByID(agents, []string{rootSenderID})
			}
		}
		if len(targetAgents) == 0 {
			targetAgents = s.chooseCoordinator(ctx, agents)
		}
	}

	if len(targetAgents) == 0 {
		return
	}
	if senderType == "agent" {
		targetAgents = excludeAgent(targetAgents, senderID)
	}
	if len(targetAgents) == 0 {
		return
	}

	// Get thread context messages
	threadSvc := NewThreadService(s.pool)
	threadMsgs, err := threadSvc.GetThreadContextMessages(ctx, threadID)
	if err != nil {
		slog.Error("failed to get thread context messages", "thread_id", threadID, "error", err)
		return
	}

	// Resolve channel name/type for target headers in thread context.
	var threadChannelName, threadChannelType string
	_ = s.pool.QueryRow(ctx, `SELECT c.name, c.type FROM channels c
		JOIN threads t ON t.channel_id = c.id WHERE t.id = $1`, threadID,
	).Scan(&threadChannelName, &threadChannelType)
	if threadChannelName == "" {
		slog.Warn("thread trigger: failed to resolve channel name", "thread_id", threadID)
	}

	contextMsgs := make([]agent.Message, len(threadMsgs))
	triggerMessageID := ""
	for i, tm := range threadMsgs {
		triggerMessageID = tm.ID
		role := agent.RoleUser
		if tm.SenderType == "agent" {
			role = agent.RoleAssistant
		}
		target := "#" + threadChannelName
		if threadChannelType == "dm" {
			target = "dm:@" + threadChannelName
		}
		shortID := tm.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		// header with thread :shortid suffix.
		senderName := tm.SenderName
		if senderName == "" {
			senderName = tm.SenderID
		}
		header := fmt.Sprintf("[target=%s:%s msg=%s time=%s type=%s] @%s:",
			target, shortID, shortID,
			tm.CreatedAt.UTC().Format(time.RFC3339), tm.SenderType, senderName)
		messageContent, attachments := s.enrichMessageContentAndAttachments(ctx, tm.Content, tm.AttachmentIDs)
		contextMsgs[i] = agent.Message{
			Role:        role,
			Content:     header + " " + messageContent,
			SenderID:    tm.SenderID,
			Attachments: attachments,
		}
	}
	// Prepend a system header when the agent is @mentioned into a new thread.
	if len(mentionedAgentIDs) > 0 {
		threadTargetPrefix := "#" + threadChannelName
		if threadChannelType == "dm" {
			threadTargetPrefix = "dm:@" + threadChannelName
		}
		threadShortID := threadID
		if len(threadShortID) > 8 {
			threadShortID = threadShortID[:8]
		}
		systemHeader := fmt.Sprintf("[System: You were added to a new thread via @mention. Reply in this thread.]\n"+
			"target: %s:%s\n"+
			"read thread: solo message read --target '%s:%s'",
			threadTargetPrefix, threadShortID, threadTargetPrefix, threadShortID)
		contextMsgs = append([]agent.Message{{
			Role:     agent.RoleSystem,
			Content:  systemHeader,
			SenderID: "system",
		}}, contextMsgs...)
	}
	// Query for open tasks in the channel to provide task context to agents
	taskContext := s.getChannelOpenTasksSummary(ctx, channelID)

	// Resolve @mentioned agent names for context awareness.
	threadMentionedNames := s.resolveMentionedNames(ctx, mentionedAgentIDs)

	for _, ag := range targetAgents {
		// Use thread-scoped debounce so thread follow-ups are not blocked
		// by channel-level triggers for the same agent.
		if s.checkThreadDebounce(channelID, threadID, ag.ID) {
			slog.Debug("agent debounced for thread", "agent_id", ag.ID, "channel_id", channelID, "thread_id", threadID)
			continue
		}
		s.updateThreadDebounce(channelID, threadID, ag.ID)

		// SOLO-228-B: Validate agent trigger chain to prevent infinite loops.
		if agentChain != nil {
			if len(agentChain) >= maxAgentChainDepth {
				slog.Debug("agent chain depth limit reached (thread)", "agent_id", ag.ID, "channel_id", channelID, "thread_id", threadID, "depth", len(agentChain))
				continue
			}
			if containsStr(agentChain, ag.ID) {
				slog.Debug("agent already in trigger chain (thread), skipping", "agent_id", ag.ID, "channel_id", channelID, "thread_id", threadID)
				continue
			}
		}
		newChain := append([]string(nil), agentChain...)
		newChain = append(newChain, ag.ID)

		daemon := s.dm.SelectDaemon("llm")
		if daemon == nil {
			slog.Warn("no available daemon for thread agent task", "agent_id", ag.ID)
			s.broadcastAgentError(threadID, channelID, ag.ID, ag.Name, agentErrorNoAvailableDaemon)
			continue
		}

		taskReq := daemonTaskRequest{
			TaskID:           uuid.New().String(),
			AgentID:          ag.ID,
			ChannelID:        channelID,
			ThreadID:         threadID,
			TriggerMessageID: triggerMessageID,
			Messages:         contextMsgs,
			SystemPrompt:     ag.SystemPrompt,
			ModelConfig: agent.ModelConfig{
				Provider: ag.ModelProvider,
				Model:    ag.ModelName,
			},
			TaskContext:    taskContext,
			AgentChain:     newChain,
			MentionedNames: threadMentionedNames,
		}

		// Use streaming for thread as well
		go s.handleStreamingAgentTask(context.Background(), daemon, taskReq, ag)
	}
}

func shouldTriggerAgentForSender(senderType string, mentionedAgentIDs []string) bool {
	if senderType == "agent" {
		return len(mentionedAgentIDs) > 0
	}
	return true
}

func excludeAgent(agents []agentChannelInfo, agentID string) []agentChannelInfo {
	if agentID == "" {
		return agents
	}
	filtered := make([]agentChannelInfo, 0, len(agents))
	for _, ag := range agents {
		if ag.ID != agentID {
			filtered = append(filtered, ag)
		}
	}
	return filtered
}

// CheckClaimWindow checks whether the given claimerID is allowed to claim the
// task at this time. Returns (allowed bool, reason string).
func (s *AgentService) CheckClaimWindow(taskID, claimerID string) (bool, string) {
	return s.claimWindow.CheckClaimAllowed(taskID, claimerID)
}

// CloseClaimWindow explicitly closes the claim window for a task.
func (s *AgentService) CloseClaimWindow(taskID string) {
	s.claimWindow.CloseWindow(taskID)
}

// TriggerAgentForTask triggers an agent to respond to a task assignment.
// SOLO-123-B: Agent execution triggered by task assignment.
// The agent's response is routed to the task's thread (not the main channel),
// so replies stay organized under the task.

// TriggerAllAgentsForTask triggers active agents in a channel to respond
// to a newly created task. Agent replies are routed to the task's thread.
//
// If mentionedAgentIDs is non-empty, only the @mentioned agents are triggered
// immediately and a 30-second priority claim window is opened. After the window
// expires, remaining agents are triggered if the task is still unclaimed.
// Without @mentions, the channel coordinator is triggered.
func (s *AgentService) TriggerAllAgentsForTask(ctx context.Context, channelID, taskID string, taskNumber int, taskTitle string, mentionedAgentIDs []string, agentChain []string) {
	agents, err := s.getChannelActiveAgents(ctx, channelID)
	if err != nil || len(agents) == 0 {
		return
	}

	if len(mentionedAgentIDs) > 0 {
		s.claimWindow.OpenWindow(taskID, mentionedAgentIDs)
		for _, ag := range filterAgentsByID(agents, selectWakeAgentIDs(wakeRouteInput{
			ActiveIDs:    agentIDs(agents),
			MentionedIDs: mentionedAgentIDs,
		})) {
			go s.TriggerAgentForTask(ctx, channelID, taskID, ag.ID, taskNumber, taskTitle, "", agentChain, mentionedAgentIDs)
		}
		return
	}

	for _, ag := range s.chooseCoordinator(ctx, agents) {
		go s.TriggerAgentForTask(ctx, channelID, taskID, ag.ID, taskNumber, taskTitle, "", agentChain, mentionedAgentIDs)
	}
}

func (s *AgentService) TriggerAgentForTask(ctx context.Context, channelID, taskID, agentID string, taskNumber int, taskTitle, taskDescription string, agentChain, mentionedAgentIDs []string) {
	// Get agent info
	var ag agentChannelInfo
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, model_provider, model_name, system_prompt
		 FROM agents WHERE id = $1 AND is_active = true`,
		agentID,
	).Scan(&ag.ID, &ag.Name, &ag.ModelProvider, &ag.ModelName,
		&ag.SystemPrompt)
	if err != nil {
		slog.Error("failed to get agent info for task trigger",
			"agent_id", agentID, "task_id", taskID, "error", err,
		)
		return
	}

	// Look up the task's message_id so we can route the agent response to the
	// task's thread rather than the main channel.
	var messageID string
	var taskMsgChannelID string
	err = s.pool.QueryRow(ctx,
		`SELECT COALESCE(message_id::text, ''), channel_id FROM tasks WHERE id = $1`,
		taskID,
	).Scan(&messageID, &taskMsgChannelID)
	if err != nil {
		slog.Error("failed to look up task for thread routing",
			"task_id", taskID, "error", err,
		)
		return
	}

	// Resolve thread ID from the task's message. If the task was created from
	// a message (ConvertMessageToTask) or if CreateTask already created a thread,
	// the thread exists. Otherwise we create one so the agent response goes to
	// a dedicated thread.
	var threadID string
	if messageID != "" {
		cid := channelID
		if taskMsgChannelID != "" {
			cid = taskMsgChannelID
		}
		threadSvc := NewThreadService(s.pool)
		tid, _, err := threadSvc.GetOrCreateThread(ctx, cid, messageID)
		if err != nil {
			slog.Warn("failed to get-or-create thread for task agent response",
				"task_id", taskID, "message_id", messageID, "error", err,
			)
		} else {
			threadID = tid
		}
	}

	// P25-05-B: If no thread could be resolved for the task, create a system
	// message and thread so agent responses can be routed properly.
	if threadID == "" {
		msgID := uuid.New().String()
		now := time.Now()
		sysContent := fmt.Sprintf("Task #%d: %s", taskNumber, taskTitle)
		_, dbErr := s.pool.Exec(ctx,
			`INSERT INTO messages (id, channel_id, sender_type, sender_id, content, content_type, created_at, updated_at)
			 VALUES ($1, $2, 'system', '00000000-0000-0000-0000-000000000000', $3, 'system', $4, $4)`,
			msgID, channelID, sysContent, now,
		)
		if dbErr != nil {
			slog.Error("TriggerAgentForTask: failed to create system message for task",
				"task_id", taskID, "error", dbErr,
			)
			return
		}
		_, _ = s.pool.Exec(ctx,
			`UPDATE tasks SET message_id = $1 WHERE id = $2`,
			msgID, taskID,
		)
		threadSvc := NewThreadService(s.pool)
		tid, _, tErr := threadSvc.GetOrCreateThread(ctx, channelID, msgID)
		if tErr != nil {
			slog.Error("TriggerAgentForTask: failed to create thread for task",
				"task_id", taskID, "error", tErr,
			)
			return
		}
		threadID = tid
		messageID = msgID
		slog.Info("TriggerAgentForTask: created thread for task",
			"task_id", taskID, "thread_id", threadID,
		)
	}

	// Select daemon
	daemon := s.dm.SelectDaemon("llm")
	if daemon == nil {
		slog.Warn("no available daemon for task agent trigger", "agent_id", ag.ID, "task_id", taskID)
		s.broadcastAgentError(threadID, channelID, ag.ID, ag.Name, agentErrorNoAvailableDaemon)
		return

	}

	// SOLO-228-B: Validate agent trigger chain for task dispatching.
	if agentChain != nil {
		if len(agentChain) >= maxAgentChainDepth {
			slog.Debug("agent chain depth limit reached (task)", "agent_id", agentID, "channel_id", channelID, "depth", len(agentChain))
			return
		}
		if containsStr(agentChain, agentID) {
			slog.Debug("agent already in trigger chain (task), skipping", "agent_id", agentID, "channel_id", channelID)
			return
		}
	}
	newChain := append([]string(nil), agentChain...)
	newChain = append(newChain, agentID)

	// deliver the task as a regular channel message with
	// [task #N status=todo] suffix, preserving the original sender and content.
	// The agent sees the same format as any other message — @mention context intact.
	var channelName, channelType string
	_ = s.pool.QueryRow(ctx, `SELECT name, type FROM channels WHERE id = $1`, channelID).Scan(&channelName, &channelType)

	target := "#" + channelName
	if channelType == "dm" {
		target = "dm:@" + channelName
	}

	shortMsgID := ""
	if len(messageID) >= 8 {
		shortMsgID = messageID[:8]
		target += ":" + shortMsgID
	}

	// Fetch sender info from the task's linked message
	var senderType, senderID, senderName, msgContent, msgCreatedAt string
	if messageID != "" {
		_ = s.pool.QueryRow(ctx,
			`SELECT m.sender_type, m.sender_id,
			        COALESCE(u.display_name, a.name, m.sender_id::text),
			        COALESCE(m.content, ''),
			        COALESCE(to_char(m.created_at, 'YYYY-MM-DD HH24:MI:SS'), '')
			 FROM messages m
			 LEFT JOIN users u ON m.sender_id = u.id
			 LEFT JOIN agents a ON m.sender_id = a.id
			 WHERE m.id = $1`,
			messageID,
		).Scan(&senderType, &senderID, &senderName, &msgContent, &msgCreatedAt)
	}

	if senderName == "" || senderName == messageID {
		senderName = s.resolveSenderName(ctx, senderType, senderID)
	}

	taskContent := fmt.Sprintf("New message received:\n\n[target=%s msg=%s time=%s type=%s] @%s: %s",
		target, shortMsgID, msgCreatedAt, senderType, senderName, msgContent)
	taskContent += fmt.Sprintf(" [task #%d status=todo channel=%s]", taskNumber, channelID)
	taskContent += "\n\nRespond as appropriate. Complete all your work before stopping."
	taskContent += "\n- To reply to this message, use the `target` and `msg` fields above."
	taskContent += "\n- To claim or update this task, use the `channel` field above (e.g. `solo task claim -n N -c <channel>`)."

	contextMsgs := []agent.Message{
		{Role: agent.RoleUser, Content: taskContent, SenderID: ""},
	}

	taskReq := daemonTaskRequest{
		TaskID:           uuid.New().String(),
		AgentID:          ag.ID,
		ChannelID:        channelID,
		ThreadID:         threadID,
		TriggerMessageID: messageID,
		Messages:         contextMsgs,
		SystemPrompt:     ag.SystemPrompt,
		ModelConfig: agent.ModelConfig{
			Provider: ag.ModelProvider,
			Model:    ag.ModelName,
		},
		OriginTaskID: taskID,
	}

	slog.Info("triggering agent for task",
		"agent_id", ag.ID,
		"task_id", taskID,
		"channel_id", channelID,
		"thread_id", threadID,
	)

	// v1.3: Agent decides whether to claim autonomously after reading task context.
	// The daemon task request includes task context. The agent evaluates the task,
	// and if it decides to claim, outputs /claim #N which is parsed by
	go s.handleStreamingAgentTask(context.Background(), daemon, taskReq, ag)
}

func (s *AgentService) TriggerAgentForArtifact(ctx context.Context, task *Task, data artifactRenderData, requestedBy, mode string) error {
	if s == nil || s.dm == nil || task == nil {
		return nil
	}
	ag, ok := s.findArtifactLeader(ctx, task)
	if !ok {
		slog.Info("artifact: no active leader agent for task", "task_id", task.ID)
		return nil
	}

	threadID := ""
	if task.MessageID != "" {
		if tid, _, err := NewThreadService(s.pool).GetOrCreateThread(ctx, task.ChannelID, task.MessageID); err == nil {
			threadID = tid
		} else {
			slog.Warn("artifact: failed to resolve task thread", "task_id", task.ID, "error", err)
		}
	}

	daemon := s.dm.SelectDaemon("llm")
	if daemon == nil {
		return fmt.Errorf("no available daemon for artifact agent trigger")
	}

	taskReq := daemonTaskRequest{
		TaskID:       uuid.New().String(),
		AgentID:      ag.ID,
		ChannelID:    task.ChannelID,
		ThreadID:     threadID,
		OriginTaskID: task.ID,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: renderArtifactAgentPrompt(data, mode)},
		},
		SystemPrompt: ag.SystemPrompt,
		ModelConfig: agent.ModelConfig{
			Provider: ag.ModelProvider,
			Model:    ag.ModelName,
		},
	}

	slog.Info("triggering leader agent for artifact",
		"agent_id", ag.ID,
		"task_id", task.ID,
		"requested_by", requestedBy,
		"mode", mode,
	)
	go s.handleStreamingAgentTask(context.Background(), daemon, taskReq, ag)
	return nil
}

func (s *AgentService) findArtifactLeader(ctx context.Context, task *Task) (agentChannelInfo, bool) {
	for _, id := range []string{task.ClaimerID, task.CreatorID} {
		if id == "" {
			continue
		}
		var ag agentChannelInfo
		err := s.pool.QueryRow(ctx,
			`SELECT id, name, model_provider, model_name, system_prompt
			 FROM agents WHERE id = $1 AND is_active = true`,
			id,
		).Scan(&ag.ID, &ag.Name, &ag.ModelProvider, &ag.ModelName, &ag.SystemPrompt)
		if err == nil {
			return ag, true
		}
	}
	return agentChannelInfo{}, false
}

// agentChannelInfo holds agent data needed for triggering.
type agentChannelInfo struct {
	ID            string
	Name          string
	ModelProvider string
	ModelName     string
	SystemPrompt  string
}

// getChannelActiveAgents queries all active agent members of a channel.
func (s *AgentService) getChannelActiveAgents(ctx context.Context, channelID string) ([]agentChannelInfo, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT a.id, a.name, a.model_provider, a.model_name,
		        a.system_prompt
		 FROM channel_members cm
		 JOIN agents a ON a.id = cm.member_id
		 WHERE cm.channel_id = $1
		   AND cm.member_type = 'agent'
		   AND a.is_active = true
		 ORDER BY cm.joined_at ASC, a.created_at ASC, a.id ASC`,
		channelID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []agentChannelInfo
	for rows.Next() {
		var a agentChannelInfo
		if err := rows.Scan(&a.ID, &a.Name, &a.ModelProvider, &a.ModelName,
			&a.SystemPrompt); err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if agents == nil {
		agents = []agentChannelInfo{}
	}

	return agents, nil
}

// getThreadParticipantAgents returns the distinct agent sender IDs that have
// already replied in the given thread. Used to scope thread follow-up triggers.
func (s *AgentService) getThreadParticipantAgents(ctx context.Context, threadID string) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT sender_id FROM messages
		 WHERE thread_id = $1 AND sender_type = 'agent'`,
		threadID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agentIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		agentIDs = append(agentIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return agentIDs, nil
}

// getThreadRootAgentSender returns the agent ID of the root message sender
// in a thread, or empty string if the root message was sent by a human.
func (s *AgentService) getThreadRootAgentSender(ctx context.Context, threadID string) string {
	var senderID string
	_ = s.pool.QueryRow(ctx,
		`SELECT m.sender_id FROM messages m
		 JOIN threads t ON t.root_message_id = m.id
		 WHERE t.id = $1 AND m.sender_type = 'agent'`,
		threadID,
	).Scan(&senderID)
	return senderID
}

// getRecentMessages returns the most recent N messages in a channel as agent messages.
// v1.3: format — each message gets a structured header plus
// "Respond as appropriate" routing instruction on the last message.
func (s *AgentService) getRecentMessages(ctx context.Context, channelID string, limit int) ([]agent.Message, error) {
	return s.getRecentMessagesForNode(ctx, channelID, "", limit)
}

func (s *AgentService) getRecentMessagesForNode(ctx context.Context, channelID, nodeID string, limit int) ([]agent.Message, error) {
	// Get channel name for the target header.
	var channelName, channelType string
	_ = s.pool.QueryRow(ctx, `SELECT COALESCE(name, id::text), type FROM channels WHERE id = $1`, channelID).Scan(&channelName, &channelType)

	msgTarget := "#" + channelName
	if channelType == "dm" {
		msgTarget = "dm:@" + channelName
	}

	rows, err := s.pool.Query(ctx,
		`SELECT m.id, m.sender_type, m.sender_id, m.content, m.created_at, COALESCE(m.attachment_ids, '{}') as attachment_ids
		 FROM messages m
			 WHERE m.channel_id = $1 AND m.thread_id IS NULL
			   AND (($3 = '' AND m.thinking_node_id IS NULL) OR m.thinking_node_id = NULLIF($3, '')::uuid)
			 ORDER BY m.created_at DESC, m.id DESC
			 LIMIT $2`,
		channelID, limit, nodeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type msgRow struct {
		id            string
		senderType    string
		senderID      string
		content       string
		createdAt     string
		attachmentIDs []string
	}
	var rows_ []msgRow
	for rows.Next() {
		var r msgRow
		var t time.Time
		if err := rows.Scan(&r.id, &r.senderType, &r.senderID, &r.content, &t, &r.attachmentIDs); err != nil {
			return nil, err
		}
		r.createdAt = t.Format(time.RFC3339)
		rows_ = append(rows_, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Reverse to chronological order (oldest first)
	for i, j := 0, len(rows_)-1; i < j; i, j = i+1, j-1 {
		rows_[i], rows_[j] = rows_[j], rows_[i]
	}

	msgs := make([]agent.Message, 0, len(rows_))
	for i, row := range rows_ {
		senderName := s.resolveSenderName(ctx, row.senderType, row.senderID)
		shortID := row.id
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}

		role := agent.RoleUser
		if row.senderType == "agent" {
			role = agent.RoleAssistant
		}

		// [target=#channel msg=shortid time=isotime type=human|agent] @sender: content
		messageContent, attachments := s.enrichMessageContentAndAttachments(ctx, row.content, row.attachmentIDs)
		content := fmt.Sprintf("New message received:\n\n[target=%s msg=%s time=%s type=%s] @%s: %s",
			msgTarget, shortID, row.createdAt, row.senderType, senderName, messageContent)

		// On the LAST (most recent) message, append routing instruction.
		if i == len(rows_)-1 {
			if nodeID == "" {
				content += "\n\nRespond as appropriate. Complete all your work before stopping.\nReply in the channel or create/reply in a thread as appropriate; use each message's `target` and `msg` fields to choose the exact target."
			} else {
				content += "\n\nRespond in the current Thinking node. Complete all your work before stopping."
			}
		}

		msgs = append(msgs, agent.Message{
			Role:        role,
			Content:     content,
			SenderID:    row.senderID,
			Attachments: attachments,
		})
	}

	if msgs == nil {
		msgs = []agent.Message{}
	}

	return msgs, nil
}

// resolveSenderName resolves a sender ID to a human-readable name for
// message formatting. Returns the ID as fallback.
func (s *AgentService) resolveSenderName(ctx context.Context, senderType, senderID string) string {
	if senderType == "system" {
		return "Solo"
	}
	var name string
	var err error
	if senderType == "agent" {
		err = s.pool.QueryRow(ctx, `SELECT COALESCE(name, $1) FROM agents WHERE id = $2`, senderID, senderID).Scan(&name)
	} else {
		err = s.pool.QueryRow(ctx, `SELECT COALESCE(display_name, email, $1) FROM users WHERE id = $2`, senderID, senderID).Scan(&name)
	}
	if err != nil || name == "" {
		return senderID
	}
	return name
}

// --- Agent status management ---

// AgentStatus represents the runtime status of an agent.
type AgentStatus string

const (
	AgentStatusOnline   AgentStatus = "online"
	AgentStatusThinking AgentStatus = "thinking"
	AgentStatusTyping   AgentStatus = "typing"
	AgentStatusOffline  AgentStatus = "offline"
	AgentStatusError    AgentStatus = "error"
)

// BroadcastAgentStatus sends an agent status update to channel subscribers.
func (s *AgentService) BroadcastAgentStatus(channelID, agentID string, status AgentStatus, detail string) {
	payload := map[string]interface{}{
		"channel_id": channelID,
		"agent_id":   agentID,
		"status":     string(status),
	}
	if detail != "" {
		payload["detail"] = detail
	}

	var eventType string
	switch status {
	case AgentStatusThinking:
		eventType = "agent.thinking"
	case AgentStatusTyping:
		eventType = "agent.typing"
	case AgentStatusError:
		eventType = "agent.error"
	default:
		eventType = "agent.status"
	}

	data, _ := json.Marshal(payload)
	envelope, _ := json.Marshal(map[string]interface{}{
		"type":    eventType,
		"payload": json.RawMessage(data),
	})
	s.hub.BroadcastToChannel(channelID, envelope)
}

// nullableStr returns a *string for nullable DB columns.
func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (s *AgentService) getChannelOpenTasksSummary(ctx context.Context, channelID string) string {
	rows, err := s.pool.Query(ctx,
		`SELECT task_number, title, priority
		 FROM tasks
		 WHERE channel_id = $1 AND status = $2
		 ORDER BY created_at DESC
		 LIMIT 10`,
		channelID, TaskStatusTodo,
	)
	if err != nil {
		slog.Warn("failed to query open tasks for agent context", "channel_id", channelID, "error", err)
		return ""
	}
	defer rows.Close()

	var b strings.Builder
	hasTasks := false
	for rows.Next() {
		var number int
		var title, priority string
		if err := rows.Scan(&number, &title, &priority); err != nil {
			continue
		}
		if !hasTasks {
			b.WriteString("Available tasks in this channel:\n")
			hasTasks = true
		}
		prio := ""
		if priority != "" && priority != "none" {
			prio = fmt.Sprintf(" [%s]", priority)
		}
		fmt.Fprintf(&b, "#%d %s%s [todo]\n", number, title, prio)
	}
	if err := rows.Err(); err != nil {
		slog.Warn("error iterating open tasks for agent context", "channel_id", channelID, "error", err)
		return ""
	}
	return b.String()
}

// daemonTaskRequest is the format for tasks sent from server to daemon.
type daemonTaskRequest struct {
	TaskID                string            `json:"task_id"`
	AgentID               string            `json:"agent_id"`
	ChannelID             string            `json:"channel_id"`
	ThreadID              string            `json:"thread_id,omitempty"`
	NodeID                string            `json:"thinking_node_id,omitempty"`
	ResumeSessionID       string            `json:"resume_session_id,omitempty"`
	TriggerMessageID      string            `json:"trigger_message_id,omitempty"`
	Messages              []agent.Message   `json:"messages"`
	ColdStartMessages     []agent.Message   `json:"cold_start_messages,omitempty"`
	SystemPrompt          string            `json:"system_prompt"`
	ThinkingRuntimePrompt string            `json:"thinking_runtime_prompt,omitempty"`
	ModelConfig           agent.ModelConfig `json:"model_config"`
	OriginTaskID          string            `json:"origin_task_id,omitempty"`   // SOLO-123-B: task ID for status update
	TaskContext           string            `json:"task_context,omitempty"`     // SOLO-221-B: summary of pending tasks in channel
	AgentChain            []string          `json:"agent_chain,omitempty"`      // SOLO-228-B: agent trigger chain for loop prevention
	MentionedNames        []string          `json:"mentioned_names,omitempty"`  // v1.3: names of @mentioned agents for context awareness
	InitialGreeting       string            `json:"initial_greeting,omitempty"` // greeting message to prepend as system context
	ReturnHandoff         bool              `json:"return_handoff,omitempty"`
}

// containsStr returns true if the slice s contains value v.
func containsStr(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// isNotParticipating detects "not for me" / "not participating" messages
// that agents output despite being told to output NOTHING. These should be
// suppressed so they don't clutter the channel/thread.

// inCascadeCooldown returns true if the channel is in cascade cooldown.
func (s *AgentService) inCascadeCooldown(channelID string) bool {
	v, ok := s.cascadeMap.Load(channelID)
	if !ok {
		return false
	}
	cc := v.(*cascadeCount)
	return time.Now().Before(cc.cooldownUntil)
}

// recordCascadeDispatch records a dispatch for cascade detection.
func (s *AgentService) recordCascadeDispatch(channelID string) {
	now := time.Now()
	v, _ := s.cascadeMap.LoadOrStore(channelID, &cascadeCount{windowStart: now})
	cc := v.(*cascadeCount)
	if now.Sub(cc.windowStart) > cascadeWindow {
		cc.count = 0
		cc.windowStart = now
	}
	cc.count++
	if cc.count >= cascadeThreshold {
		cc.cooldownUntil = now.Add(cascadeCooldown)
		slog.Warn("cascade detected — pausing agent triggers",
			"channel_id", channelID, "count", cc.count, "cooldown", cascadeCooldown)
	}
}
