package service

import (
	"context"
	"log/slog"
)

const (
	wakeRecentWindowSize   = 20
	wakeSmallTeamThreshold = 4
)

type wakeRouteScope string

const (
	wakeScopeChannel wakeRouteScope = "channel"
	wakeScopeTask    wakeRouteScope = "task"
	wakeScopeThread  wakeRouteScope = "thread"
)

type wakeRouteReason string

const (
	wakeReasonExplicitMention          wakeRouteReason = "explicit_mention"
	wakeReasonUnresolvedMention        wakeRouteReason = "unresolved_mention"
	wakeReasonUniqueCoordinator        wakeRouteReason = "unique_coordinator"
	wakeReasonSmallTeamFanout          wakeRouteReason = "small_team_fanout"
	wakeReasonRecentWindow             wakeRouteReason = "recent_window"
	wakeReasonThreadCoordinator        wakeRouteReason = "thread_participant_coordinator"
	wakeReasonThreadRecentParticipant  wakeRouteReason = "thread_recent_participant"
	wakeReasonThreadRootAgent          wakeRouteReason = "thread_root_agent"
	wakeReasonThreadChannelCoordinator wakeRouteReason = "thread_channel_coordinator"
	wakeReasonThreadRecentChannelAgent wakeRouteReason = "thread_recent_channel_agent"
	wakeReasonDeterministicFirst       wakeRouteReason = "deterministic_first"
	wakeReasonNoActiveAgent            wakeRouteReason = "no_active_agent"
)

type wakeRouteRequest struct {
	Scope             wakeRouteScope
	ChannelID         string
	ThreadID          string
	TriggerMessageID  string
	MentionedAgentIDs []string
	HasMentions       bool
	ExcludeAgentID    string
}

type wakeRouteFacts struct {
	Scope                wakeRouteScope
	ActiveIDs            []string
	MentionedIDs         []string
	HasMentions          bool
	GraphNodeIDs         []string
	Edges                []relationshipEdge
	RecentActiveIDs      []string
	ThreadParticipantIDs []string
	ThreadRootAgentID    string
}

type wakeRouteDecision struct {
	AgentIDs []string
	Reason   wakeRouteReason
}

type relationshipEdge struct {
	from string
	to   string
}

func (s *AgentService) routeWakeTargets(ctx context.Context, agents []agentChannelInfo, req wakeRouteRequest) ([]agentChannelInfo, wakeRouteDecision) {
	activeIDs := agentIDs(agents)
	if req.ExcludeAgentID != "" {
		activeIDs = excludeID(activeIDs, req.ExcludeAgentID)
	}
	facts := wakeRouteFacts{
		Scope:        req.Scope,
		ActiveIDs:    activeIDs,
		MentionedIDs: req.MentionedAgentIDs,
		HasMentions:  req.HasMentions,
	}

	// Explicit and unresolved mentions do not need relationship or history reads.
	decision := selectWakeDecision(facts)
	if decision.Reason != wakeReasonExplicitMention && decision.Reason != wakeReasonUnresolvedMention && decision.Reason != wakeReasonNoActiveAgent {
		allEdges, err := s.getWakeRelationshipEdges(ctx)
		if err != nil {
			slog.Error("failed to load agent relationship graph for wake routing", "channel_id", req.ChannelID, "error", err)
		} else {
			facts.GraphNodeIDs, facts.Edges = relationshipGraphForActive(activeIDs, allEdges)
		}

		if req.Scope == wakeScopeThread {
			facts.ThreadParticipantIDs, err = s.getThreadParticipantAgents(ctx, req.ThreadID)
			if err != nil {
				slog.Error("failed to load thread agent participants for wake routing", "channel_id", req.ChannelID, "thread_id", req.ThreadID, "error", err)
			}
			facts.ThreadRootAgentID, err = s.getThreadRootAgentSender(ctx, req.ThreadID)
			if err != nil {
				slog.Error("failed to load thread root agent for wake routing", "channel_id", req.ChannelID, "thread_id", req.ThreadID, "error", err)
			}
		}

		decision = selectWakeDecision(facts)
		if decision.Reason == wakeReasonDeterministicFirst {
			facts.RecentActiveIDs, err = s.getRecentChannelAgentIDs(ctx, req.ChannelID, req.TriggerMessageID)
			if err != nil {
				slog.Error("failed to load recent channel agents for wake routing", "channel_id", req.ChannelID, "error", err)
			} else {
				decision = selectWakeDecision(facts)
			}
		}
	}

	slog.Info("agent wake route decided",
		"scope", req.Scope,
		"channel_id", req.ChannelID,
		"thread_id", req.ThreadID,
		"trigger_message_id", req.TriggerMessageID,
		"reason", decision.Reason,
		"target_ids", decision.AgentIDs,
		"target_count", len(decision.AgentIDs),
	)
	return filterAgentsByID(agents, decision.AgentIDs), decision
}

func selectWakeDecision(facts wakeRouteFacts) wakeRouteDecision {
	if len(facts.ActiveIDs) == 0 {
		return wakeRouteDecision{Reason: wakeReasonNoActiveAgent}
	}
	if len(facts.MentionedIDs) > 0 {
		ids := intersectInOrder(facts.ActiveIDs, facts.MentionedIDs)
		if len(ids) == 0 {
			return wakeRouteDecision{Reason: wakeReasonUnresolvedMention}
		}
		return wakeRouteDecision{AgentIDs: ids, Reason: wakeReasonExplicitMention}
	}
	if facts.HasMentions {
		return wakeRouteDecision{Reason: wakeReasonUnresolvedMention}
	}

	coordinatorID := uniqueCoordinatorID(facts.GraphNodeIDs, facts.Edges, facts.ActiveIDs)
	if facts.Scope == wakeScopeThread {
		participants := intersectPreferredOrder(facts.ThreadParticipantIDs, facts.ActiveIDs)
		if len(participants) > 0 {
			if containsID(participants, coordinatorID) {
				return wakeRouteDecision{AgentIDs: []string{coordinatorID}, Reason: wakeReasonThreadCoordinator}
			}
			return wakeRouteDecision{AgentIDs: participants[:1], Reason: wakeReasonThreadRecentParticipant}
		}
		if containsID(facts.ActiveIDs, facts.ThreadRootAgentID) {
			return wakeRouteDecision{AgentIDs: []string{facts.ThreadRootAgentID}, Reason: wakeReasonThreadRootAgent}
		}
		if coordinatorID != "" {
			return wakeRouteDecision{AgentIDs: []string{coordinatorID}, Reason: wakeReasonThreadChannelCoordinator}
		}
		if recent := intersectPreferredOrder(facts.RecentActiveIDs, facts.ActiveIDs); len(recent) > 0 {
			return wakeRouteDecision{AgentIDs: recent[:1], Reason: wakeReasonThreadRecentChannelAgent}
		}
		return wakeRouteDecision{AgentIDs: facts.ActiveIDs[:1], Reason: wakeReasonDeterministicFirst}
	}

	if coordinatorID != "" {
		return wakeRouteDecision{AgentIDs: []string{coordinatorID}, Reason: wakeReasonUniqueCoordinator}
	}
	if len(facts.ActiveIDs) < wakeSmallTeamThreshold {
		return wakeRouteDecision{AgentIDs: append([]string(nil), facts.ActiveIDs...), Reason: wakeReasonSmallTeamFanout}
	}
	if recent := intersectPreferredOrder(facts.RecentActiveIDs, facts.ActiveIDs); len(recent) > 0 {
		return wakeRouteDecision{AgentIDs: recent, Reason: wakeReasonRecentWindow}
	}
	return wakeRouteDecision{AgentIDs: facts.ActiveIDs[:1], Reason: wakeReasonDeterministicFirst}
}

func uniqueCoordinatorID(nodeIDs []string, edges []relationshipEdge, activeIDs []string) string {
	if len(edges) == 0 {
		return ""
	}
	nodes := make(map[string]bool, len(nodeIDs))
	for _, id := range nodeIDs {
		if id != "" {
			nodes[id] = true
		}
	}
	if len(nodes) == 0 {
		return ""
	}

	indegree := make(map[string]int, len(nodes))
	adjacent := make(map[string][]string, len(nodes))
	for id := range nodes {
		indegree[id] = 0
	}
	for _, edge := range edges {
		if !nodes[edge.from] || !nodes[edge.to] {
			continue
		}
		adjacent[edge.from] = append(adjacent[edge.from], edge.to)
		indegree[edge.to]++
	}

	roots := make([]string, 0, 2)
	for id, degree := range indegree {
		if degree == 0 {
			roots = append(roots, id)
		}
	}
	if len(roots) != 1 {
		return ""
	}

	queue := []string{roots[0]}
	visited := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		visited++
		for _, next := range adjacent[id] {
			indegree[next]--
			if indegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if visited != len(nodes) || !containsID(activeIDs, roots[0]) {
		return ""
	}
	return roots[0]
}

func relationshipGraphForActive(activeIDs []string, allEdges []relationshipEdge) ([]string, []relationshipEdge) {
	reachable := make(map[string]bool, len(activeIDs))
	for _, id := range activeIDs {
		reachable[id] = true
	}
	for changed := true; changed; {
		changed = false
		for _, edge := range allEdges {
			if reachable[edge.from] && !reachable[edge.to] {
				reachable[edge.to] = true
				changed = true
			}
			if reachable[edge.to] && !reachable[edge.from] {
				reachable[edge.from] = true
				changed = true
			}
		}
	}

	nodes := append([]string(nil), activeIDs...)
	seen := make(map[string]bool, len(reachable))
	for _, id := range nodes {
		seen[id] = true
	}
	edges := make([]relationshipEdge, 0, len(allEdges))
	for _, edge := range allEdges {
		if !reachable[edge.from] || !reachable[edge.to] {
			continue
		}
		edges = append(edges, edge)
		for _, id := range []string{edge.from, edge.to} {
			if !seen[id] {
				nodes = append(nodes, id)
				seen[id] = true
			}
		}
	}
	return nodes, edges
}

func (s *AgentService) getWakeRelationshipEdges(ctx context.Context) ([]relationshipEdge, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT from_agent_id::text, to_agent_id::text
		  FROM agent_relationships
		 WHERE rel_type = 'assigns_to'
		 ORDER BY created_at ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	edges := []relationshipEdge{}
	for rows.Next() {
		var edge relationshipEdge
		if err := rows.Scan(&edge.from, &edge.to); err != nil {
			return nil, err
		}
		edges = append(edges, edge)
	}
	return edges, rows.Err()
}

func (s *AgentService) getRecentChannelAgentIDs(ctx context.Context, channelID, excludeMessageID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		WITH recent AS (
			SELECT m.id, m.sender_type, m.sender_id, m.mentioned_agent_ids,
			       row_number() OVER (ORDER BY m.created_at DESC, m.id DESC) AS message_rank
			  FROM messages m
			 WHERE m.channel_id = $1
			   AND m.thread_id IS NULL
			   AND m.thinking_node_id IS NULL
			   AND ($2 = '' OR m.id::text <> $2)
			 ORDER BY m.created_at DESC, m.id DESC
			 LIMIT $3
		), involved AS (
			SELECT sender_id AS agent_id, message_rank
			  FROM recent
			 WHERE sender_type = 'agent'
			UNION ALL
			SELECT unnest(COALESCE(mentioned_agent_ids, '{}'::uuid[])) AS agent_id, message_rank
			  FROM recent
			UNION ALL
			SELECT t.claimer_id AS agent_id, recent.message_rank
			  FROM recent
			  JOIN tasks t ON t.message_id = recent.id
			 WHERE t.claimer_id IS NOT NULL
		)
		SELECT agent_id::text
		  FROM involved
		 WHERE agent_id IS NOT NULL
		 GROUP BY agent_id
		 ORDER BY MIN(message_rank) ASC, agent_id::text ASC
	`, channelID, excludeMessageID, wakeRecentWindowSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *AgentService) getThreadParticipantAgents(ctx context.Context, threadID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT participant.sender_id::text
		  FROM (
			SELECT DISTINCT ON (sender_id) sender_id, created_at, id
			  FROM messages
			 WHERE thread_id = $1 AND sender_type = 'agent'
			 ORDER BY sender_id, created_at DESC, id DESC
		  ) participant
		 ORDER BY participant.created_at DESC, participant.id DESC, participant.sender_id ASC
	`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *AgentService) getThreadRootAgentSender(ctx context.Context, threadID string) (string, error) {
	var senderID string
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE((
			SELECT m.sender_id::text
			  FROM messages m
			  JOIN threads t ON t.root_message_id = m.id
			 WHERE t.id = $1 AND m.sender_type = 'agent'
		), '')
	`, threadID).Scan(&senderID)
	return senderID, err
}

func intersectInOrder(order, allowedIDs []string) []string {
	allowed := make(map[string]bool, len(allowedIDs))
	for _, id := range allowedIDs {
		allowed[id] = true
	}
	result := make([]string, 0, len(order))
	seen := make(map[string]bool, len(order))
	for _, id := range order {
		if allowed[id] && !seen[id] {
			result = append(result, id)
			seen[id] = true
		}
	}
	return result
}

func intersectPreferredOrder(preferred, allowedIDs []string) []string {
	return intersectInOrder(preferred, allowedIDs)
}

func excludeID(ids []string, excluded string) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != excluded {
			result = append(result, id)
		}
	}
	return result
}

func containsID(ids []string, target string) bool {
	if target == "" {
		return false
	}
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}
