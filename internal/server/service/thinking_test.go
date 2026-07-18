package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func TestThinkingEnsureConcurrentlyCreatesOneExistingAgentTeam(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	leadID := agentRunAgent(t, pool, ownerID)
	feID := agentRunAgent(t, pool, ownerID)
	beID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = ANY($1)`, []string{leadID, feID, beID})
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})
	for _, agentID := range []string{leadID, feID, beID} {
		if _, err := pool.Exec(ctx, `INSERT INTO channel_members (channel_id, member_type, member_id) VALUES ($1, 'agent', $2)`, channelID, agentID); err != nil {
			t.Fatalf("add channel Agent: %v", err)
		}
	}
	for _, childID := range []string{feID, beID} {
		if _, err := pool.Exec(ctx, `INSERT INTO agent_relationships (from_agent_id, to_agent_id, rel_type) VALUES ($1, $2, 'assigns_to')`, leadID, childID); err != nil {
			t.Fatalf("create team relationship: %v", err)
		}
	}

	const workers = 8
	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := NewThinkingService(pool).Ensure(ctx, channelID, ownerID)
			errCh <- err
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent ensure: %v", err)
		}
	}

	space, err := NewThinkingService(pool).Get(ctx, channelID)
	if err != nil {
		t.Fatalf("get Thinking space: %v", err)
	}
	var roots, teamChildren int
	for _, node := range space.Nodes {
		if node.ParentID == "" {
			roots++
			if node.AgentID != leadID {
				t.Fatalf("root agent = %q, want existing Lead %q", node.AgentID, leadID)
			}
			if _, err := NewThinkingService(pool).CreateChild(ctx, channelID, node.ID, "Not first-level manual", ownerID, "manual"); !errors.Is(err, ErrThinkingLimit) {
				t.Fatalf("manual root split with existing team error = %v, want ErrThinkingLimit", err)
			}
		}
		if node.Source == "team" {
			teamChildren++
			if node.AgentID != feID && node.AgentID != beID {
				t.Fatalf("team node uses unknown Agent %q", node.AgentID)
			}
		}
	}
	if roots != 1 || teamChildren != 2 || len(space.Nodes) != 3 {
		t.Fatalf("topology roots=%d team=%d nodes=%d, want 1/2/3", roots, teamChildren, len(space.Nodes))
	}
}

func TestThinkingChildIsolationAndAgentHandoffReturnUsePostgres(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	leadID := agentRunAgent(t, pool, ownerID)
	feID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = ANY($1)`, []string{leadID, feID})
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})
	for _, agentID := range []string{leadID, feID} {
		if _, err := pool.Exec(ctx, `INSERT INTO channel_members (channel_id, member_type, member_id) VALUES ($1, 'agent', $2)`, channelID, agentID); err != nil {
			t.Fatalf("add channel Agent: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO agent_relationships (from_agent_id, to_agent_id, rel_type) VALUES ($1, $2, 'assigns_to')`, leadID, feID); err != nil {
		t.Fatalf("create team relationship: %v", err)
	}

	svc := NewThinkingService(pool)
	space, err := svc.Ensure(ctx, channelID, ownerID)
	if err != nil {
		t.Fatalf("ensure Thinking space: %v", err)
	}
	var feNode ThinkingNode
	for _, node := range space.Nodes {
		if node.AgentID == feID {
			feNode = node
		}
	}
	if feNode.ID == "" {
		t.Fatal("FE team node not found")
	}
	parentCheckpoint := "# Handoff\n## Objective and scope\nFE parent\n## Confirmed conclusions\nUse the shared component"
	if _, err := svc.SaveCheckpointHandoff(ctx, channelID, feNode.ID, feID, parentCheckpoint); err != nil {
		t.Fatalf("save FE checkpoint Handoff: %v", err)
	}
	child, err := svc.CreateChild(ctx, channelID, feNode.ID, "FE child", ownerID, "manual")
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	sibling, err := svc.CreateChild(ctx, channelID, feNode.ID, "FE sibling", ownerID, "manual")
	if err != nil {
		t.Fatalf("create sibling: %v", err)
	}
	if child.Depth != 2 || child.AgentID != feID || child.AgentSessionID != "" || !child.ForkHandoffPending || child.InheritedHandoff != "" {
		t.Fatalf("child binding = %+v", child)
	}
	if _, err := svc.BeginReturn(ctx, channelID, child.ID); !errors.Is(err, ErrThinkingPreparing) {
		t.Fatalf("return pending child error = %v, want ErrThinkingPreparing", err)
	}
	forkHandoff := "# Handoff\n## Objective and scope\nFE child\n## Relevant parent conclusions\nUse the shared component"
	child, err = svc.CompleteForkHandoff(ctx, channelID, feNode.ID, child.ID, feID, forkHandoff)
	if err != nil {
		t.Fatalf("complete child fork Handoff: %v", err)
	}
	if child.ForkHandoffPending || child.ForkHandoffAt == nil || child.InheritedHandoff != forkHandoff {
		t.Fatalf("ready child = %+v", child)
	}
	if _, err := svc.CompleteForkHandoff(ctx, channelID, feNode.ID, sibling.ID, feID, "# Handoff\n## Objective and scope\nFE sibling"); err != nil {
		t.Fatalf("complete sibling fork Handoff: %v", err)
	}

	insertNodeMessage := func(nodeID, senderType, senderID, content string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `
			INSERT INTO messages (id, channel_id, thinking_node_id, sender_type, sender_id, content)
			VALUES ($1, $2, $3, $4, $5, $6)`, uuid.NewString(), channelID, nodeID, senderType, senderID, content); err != nil {
			t.Fatalf("insert node message: %v", err)
		}
	}
	insertNodeMessage(child.ID, "user", ownerID, "child raw fact")
	insertNodeMessage(child.ID, "agent", feID, "child result")
	insertNodeMessage(sibling.ID, "user", ownerID, "sibling private raw fact")
	childCheckpoint := "# Handoff\n## Confirmed conclusions\nchild result"
	if _, err := svc.SaveCheckpointHandoff(ctx, channelID, child.ID, feID, childCheckpoint); err != nil {
		t.Fatalf("save child checkpoint Handoff: %v", err)
	}

	agentSvc := &AgentService{pool: pool}
	childHistory, err := agentSvc.getRecentMessagesForNode(ctx, channelID, child.ID, 100)
	if err != nil {
		t.Fatalf("load child history: %v", err)
	}
	joined := ""
	for _, message := range childHistory {
		joined += message.Content
	}
	if !strings.Contains(joined, "child raw fact") || strings.Contains(joined, "sibling private raw fact") {
		t.Fatalf("child history leaked scopes: %q", joined)
	}

	returningNode, err := svc.BeginReturn(ctx, channelID, child.ID)
	if err != nil {
		t.Fatalf("begin return: %v", err)
	}
	if returningNode.ReturningAt == nil || returningNode.ReturnedAt != nil {
		t.Fatalf("returning node = %+v", returningNode)
	}
	if _, err := svc.CreateChild(ctx, channelID, child.ID, "during return", ownerID, "manual"); !errors.Is(err, ErrThinkingReturning) {
		t.Fatalf("split during return error = %v, want ErrThinkingReturning", err)
	}
	handoff := "# Handoff\n## Objective and scope\nFE child\n## Confirmed conclusions\nchild result\n## Evidence or artifacts\nnone\n## Unresolved questions\nnone\n## Risks and assumptions\nnone\n## Recommended parent action\ncontinue"
	if _, _, err := svc.CompleteReturn(ctx, channelID, child.ID, leadID, handoff); !errors.Is(err, ErrThinkingReturning) {
		t.Fatalf("handoff from wrong Agent error = %v, want ErrThinkingReturning", err)
	}
	returnedNode, returnedMessage, err := svc.CompleteReturn(ctx, channelID, child.ID, feID, handoff)
	if err != nil {
		t.Fatalf("complete return: %v", err)
	}
	if returnedMessage == nil || returnedNode.ReturningAt != nil || returnedNode.ReturnedAt == nil || returnedNode.ReturnedHandoff != handoff {
		t.Fatalf("returned node/message = %+v / %+v", returnedNode, returnedMessage)
	}
	if returnedNode.CheckpointHandoff != childCheckpoint {
		t.Fatalf("checkpoint Handoff overwritten by terminal Handoff: %q", returnedNode.CheckpointHandoff)
	}
	if _, _, err := svc.CompleteReturn(ctx, channelID, child.ID, feID, handoff); !errors.Is(err, ErrThinkingReturned) {
		t.Fatalf("duplicate completion error = %v, want ErrThinkingReturned", err)
	}
	for _, source := range []string{"manual", "auto"} {
		if _, err := svc.CreateChild(ctx, channelID, child.ID, "post-return "+source, ownerID, source); !errors.Is(err, ErrThinkingReturned) {
			t.Fatalf("%s split after return error = %v, want ErrThinkingReturned", source, err)
		}
	}
	insertNodeMessage(feNode.ID, "user", ownerID, "parent discussion")
	if _, err := svc.BeginReturn(ctx, channelID, feNode.ID); !errors.Is(err, ErrThinkingChildren) {
		t.Fatalf("parent return with active child error = %v, want ErrThinkingChildren", err)
	}
	var returnedCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM messages
		 WHERE thinking_node_id = $1 AND sender_type = 'system' AND content = $2
		   AND content_type = 'thinking_handoff'`,
		feNode.ID, returnedMessage.Content).Scan(&returnedCount); err != nil {
		t.Fatalf("count returned messages: %v", err)
	}
	if returnedCount != 1 {
		t.Fatalf("returned parent messages = %d, want 1", returnedCount)
	}
}

func TestThinkingRunBindsNodeToRealAgentSession(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	otherAgentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_sessions WHERE agent_id = ANY($1)`, []string{agentID, otherAgentID})
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = ANY($1)`, []string{agentID, otherAgentID})
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})
	if _, err := pool.Exec(ctx, `INSERT INTO channel_members (channel_id, member_type, member_id) VALUES ($1, 'agent', $2)`, channelID, agentID); err != nil {
		t.Fatalf("add channel Agent: %v", err)
	}
	space, err := NewThinkingService(pool).Ensure(ctx, channelID, ownerID)
	if err != nil {
		t.Fatalf("ensure Thinking space: %v", err)
	}
	root := space.Nodes[0]
	if root.AgentID != agentID || root.AgentSessionID != "" {
		t.Fatalf("root binding before run = agent %q session %q", root.AgentID, root.AgentSessionID)
	}

	runSvc := NewAgentRunService(pool)
	wrongOwnerSession, err := runSvc.UpsertSession(ctx, UpsertSessionInput{
		AgentID:           otherAgentID,
		Provider:          "claude",
		ExternalSessionID: "thinking-wrong-owner-session",
	})
	if err != nil {
		t.Fatalf("upsert wrong-owner Agent session: %v", err)
	}
	session, err := runSvc.UpsertSession(ctx, UpsertSessionInput{
		AgentID:           agentID,
		Provider:          "claude",
		ExternalSessionID: "thinking-provider-session",
	})
	if err != nil {
		t.Fatalf("upsert Agent session: %v", err)
	}
	run, err := runSvc.StartRun(ctx, StartRunInput{
		AgentID:        agentID,
		TriggerType:    AgentRunTriggerMessage,
		ChannelID:      channelID,
		ThinkingNodeID: root.ID,
		Status:         AgentRunStatusQueued,
		Source:         "claude",
	})
	if err != nil {
		t.Fatalf("start node run: %v", err)
	}
	if _, err := runSvc.BindRunSession(ctx, BindRunSessionInput{
		RunID:          run.ID,
		SessionID:      wrongOwnerSession.ID,
		ThinkingNodeID: root.ID,
	}); err == nil {
		t.Fatal("cross-Agent session binding succeeded")
	}
	child, err := NewThinkingService(pool).CreateChild(ctx, channelID, root.ID, "same Agent child", ownerID, "manual")
	if err != nil {
		t.Fatalf("create same-Agent child: %v", err)
	}
	if _, err := runSvc.BindRunSession(ctx, BindRunSessionInput{
		RunID:          run.ID,
		SessionID:      session.ID,
		ThinkingNodeID: child.ID,
	}); err == nil {
		t.Fatal("cross-node run binding succeeded")
	}
	run, err = runSvc.BindRunSession(ctx, BindRunSessionInput{
		RunID:          run.ID,
		SessionID:      session.ID,
		ThinkingNodeID: root.ID,
	})
	if err != nil {
		t.Fatalf("bind node run: %v", err)
	}
	if run.ThinkingNodeID != root.ID || run.SessionID != session.ID {
		t.Fatalf("run binding = node %q session %q", run.ThinkingNodeID, run.SessionID)
	}
	updated, err := NewThinkingService(pool).GetNodeForChannel(ctx, channelID, root.ID)
	if err != nil {
		t.Fatalf("reload node: %v", err)
	}
	if updated.AgentSessionID != session.ID {
		t.Fatalf("node agent_session_id = %q, want %q", updated.AgentSessionID, session.ID)
	}
}

func TestChooseThinkingTeamUsesLeadAndDirectReports(t *testing.T) {
	agents := []thinkingTeamAgent{{id: "fe", name: "FE"}, {id: "lead", name: "Lead"}, {id: "pm", name: "PM"}, {id: "rd", name: "RD"}}
	edges := []thinkingTeamEdge{{from: "lead", to: "pm"}, {from: "lead", to: "rd"}, {from: "rd", to: "fe"}}

	root, children := chooseThinkingTeam(agents, edges)
	if root.id != "lead" {
		t.Fatalf("root = %q, want lead", root.id)
	}
	if len(children) != 2 || children[0].id != "pm" || children[1].id != "rd" {
		t.Fatalf("children = %#v, want PM and RD", children)
	}
}

func TestChooseThinkingTeamFallsBackToRemainingAgents(t *testing.T) {
	agents := []thinkingTeamAgent{{id: "lead", name: "Lead"}, {id: "fe", name: "FE"}}
	root, children := chooseThinkingTeam(agents, nil)
	if root.id != "lead" || len(children) != 1 || children[0].id != "fe" {
		t.Fatalf("root = %#v, children = %#v", root, children)
	}
}

func TestExtractThinkingSplitsStripsAtMostThreeDirectives(t *testing.T) {
	content := "Main answer\n[[split: API]]\n[[split: UX]]\n[[split: QA]]\n[[split: Later]]"
	clean, titles := ExtractThinkingSplits(content)
	if strings.Join(titles, ",") != "API,UX,QA" {
		t.Fatalf("titles = %v", titles)
	}
	if clean != "Main answer\n\n\n\n[[split: Later]]" {
		t.Fatalf("clean = %q", clean)
	}
}

func TestParseThinkingHandoffProtocol(t *testing.T) {
	targetID := uuid.NewString()
	tests := []struct {
		name    string
		content string
		kind    string
		target  string
		ok      bool
	}{
		{name: "checkpoint", content: "[[handoff:checkpoint]]\n# Handoff\ncheckpoint", kind: "checkpoint", ok: true},
		{name: "return", content: "[[handoff:return]]\n# Handoff\nterminal", kind: "return", ok: true},
		{name: "fork", content: "[[handoff:fork:" + targetID + "]]\n# Handoff\nchild", kind: "fork", target: targetID, ok: true},
		{name: "invalid target", content: "[[handoff:fork:nope]]\n# Handoff", ok: false},
		{name: "missing body", content: "[[handoff:checkpoint]]", ok: false},
		{name: "ordinary message", content: "# Handoff\nvisible", ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			protocol, ok := ParseThinkingHandoffProtocol(test.content)
			if ok != test.ok || protocol.Kind != test.kind || protocol.TargetID != test.target {
				t.Fatalf("protocol = %+v, %v", protocol, ok)
			}
		})
	}
}
