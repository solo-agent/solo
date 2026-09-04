package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestOrdinaryChannelInheritsWorkspaceUsers(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	ownerID := taskSubmitUser(t, pool)
	memberID := taskSubmitUser(t, pool)
	workspaceID := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO workspaces (id,name,created_by) VALUES ($1,$2,$3)`, workspaceID, "Channel inheritance "+workspaceID[:8], ownerID); err != nil {
		t.Fatalf("create Workspace: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO workspace_members (workspace_id,user_id,role) VALUES ($1,$2,'owner'),($1,$3,'member')`, workspaceID, ownerID, memberID); err != nil {
		t.Fatalf("create Workspace members: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM workspaces WHERE id=$1`, workspaceID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1,$2)`, ownerID, memberID)
	})

	channelID, err := NewChannelService(pool).CreateChannelInWorkspace(
		ctx, "inherited-"+workspaceID[:8], "", "channel", ownerID, workspaceID,
	)
	if err != nil {
		t.Fatalf("CreateChannelInWorkspace: %v", err)
	}

	rows, err := pool.Query(ctx, `SELECT member_id::text,role FROM channel_members WHERE channel_id=$1 AND member_type='user'`, channelID)
	if err != nil {
		t.Fatalf("list inherited members: %v", err)
	}
	defer rows.Close()
	roles := map[string]string{}
	for rows.Next() {
		var id, role string
		if err := rows.Scan(&id, &role); err != nil {
			t.Fatalf("scan inherited member: %v", err)
		}
		roles[id] = role
	}
	if roles[ownerID] != "owner" || roles[memberID] != "member" || len(roles) != 2 {
		t.Fatalf("inherited roles = %#v", roles)
	}
	if _, err := NewChannelService(pool).RemoveMember(ctx, channelID, ownerID, memberID); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("RemoveMember inherited User error = %v, want %v", err, ErrPermissionDenied)
	}
}

func TestListMembersHidesInactiveAgentMembers(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	ownerID := taskSubmitUser(t, pool)
	activeAgentID := taskSubmitAgent(t, pool, ownerID)
	inactiveAgentID := taskSubmitAgent(t, pool, ownerID)
	channelID := taskSubmitChannel(t, pool, ownerID)
	taskSubmitMember(t, pool, channelID, "user", ownerID)
	taskSubmitMember(t, pool, channelID, "agent", activeAgentID)
	taskSubmitMember(t, pool, channelID, "agent", inactiveAgentID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM channel_members WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id IN ($1, $2)`, activeAgentID, inactiveAgentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	if _, err := pool.Exec(ctx, `UPDATE agents SET is_active = false WHERE id = $1`, inactiveAgentID); err != nil {
		t.Fatalf("deactivate agent: %v", err)
	}

	members, err := NewChannelService(pool).ListMembers(ctx, channelID, ownerID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}

	foundActive := false
	for _, member := range members {
		if member.MemberID == inactiveAgentID {
			t.Fatalf("inactive agent member leaked into ListMembers")
		}
		if member.MemberID == activeAgentID {
			foundActive = true
		}
	}
	if !foundActive {
		t.Fatalf("active agent member missing from ListMembers")
	}
}

func TestChannelAgentOwnershipAndRemovalBoundaries(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	ownerID := taskSubmitUser(t, pool)
	adminID := taskSubmitUser(t, pool)
	workspaceID := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO workspaces (id,name,created_by) VALUES ($1,$2,$3)`, workspaceID, "Agent ownership "+workspaceID[:8], ownerID); err != nil {
		t.Fatalf("create Workspace: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO workspace_members (workspace_id,user_id,role) VALUES ($1,$2,'owner'),($1,$3,'member')`, workspaceID, ownerID, adminID); err != nil {
		t.Fatalf("create Workspace members: %v", err)
	}
	homeID, err := NewChannelService(pool).CreateChannelInWorkspace(ctx, "agent-home-"+workspaceID[:8], "", "channel", ownerID, workspaceID)
	if err != nil {
		t.Fatalf("create home Channel: %v", err)
	}
	sharedID, err := NewChannelService(pool).CreateChannelInWorkspace(ctx, "agent-shared-"+workspaceID[:8], "", "channel", ownerID, workspaceID)
	if err != nil {
		t.Fatalf("create shared Channel: %v", err)
	}
	foreignWorkspaceID := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO workspaces (id,name,created_by) VALUES ($1,$2,$3)`, foreignWorkspaceID, "Foreign Workspace "+foreignWorkspaceID[:8], ownerID); err != nil {
		t.Fatalf("create foreign Workspace: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO workspace_members (workspace_id,user_id,role) VALUES ($1,$2,'owner')`, foreignWorkspaceID, ownerID); err != nil {
		t.Fatalf("create foreign Workspace member: %v", err)
	}
	foreignChannelID, err := NewChannelService(pool).CreateChannelInWorkspace(ctx, "foreign-channel-"+foreignWorkspaceID[:8], "", "channel", ownerID, foreignWorkspaceID)
	if err != nil {
		t.Fatalf("create foreign Channel: %v", err)
	}
	agentID := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO agents (id,name,owner_id,model_name,home_channel_id) VALUES ($1,$2,$3,'test-model',$4)`, agentID, "owned-agent-"+agentID[:8], ownerID, homeID); err != nil {
		t.Fatalf("create Agent: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO channel_members (channel_id,member_type,member_id) VALUES ($1,'agent',$2)`, homeID, agentID); err != nil {
		t.Fatalf("add Agent to home Channel: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id=$1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM workspaces WHERE id IN ($1,$2)`, workspaceID, foreignWorkspaceID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1,$2)`, ownerID, adminID)
	})

	svc := NewChannelService(pool)
	if err := svc.AddMember(ctx, sharedID, adminID, "agent", agentID); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("non-owner AddMember error = %v, want %v", err, ErrPermissionDenied)
	}
	if err := svc.AddMember(ctx, sharedID, ownerID, "agent", agentID); err != nil {
		t.Fatalf("owner AddMember: %v", err)
	}
	if err := svc.AddMember(ctx, foreignChannelID, ownerID, "agent", agentID); err == nil {
		t.Fatalf("owner connected Agent across Workspaces")
	}
	members, err := svc.ListMembers(ctx, sharedID, ownerID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	foundOwnership := false
	for _, member := range members {
		if member.MemberID == agentID {
			foundOwnership = member.AgentOwnerID == ownerID && member.AgentHomeID == homeID
		}
	}
	if !foundOwnership {
		t.Fatalf("Agent ownership metadata missing from Channel members")
	}
	if _, err := svc.RemoveMember(ctx, sharedID, adminID, agentID); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("ordinary member RemoveMember error = %v, want %v", err, ErrPermissionDenied)
	}
	if _, err := pool.Exec(ctx, `UPDATE workspace_members SET role='admin' WHERE workspace_id=$1 AND user_id=$2`, workspaceID, adminID); err != nil {
		t.Fatalf("promote Workspace admin: %v", err)
	}
	sharedActiveID, sharedPendingID, homeActiveID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO agent_sessions (id,agent_id,provider,external_session_id,status)
		VALUES ($1,$4,'codex','shared-active','active'),
		       ($2,$4,'codex','shared-pending','rollover_pending'),
		       ($3,$4,'codex','home-active','active')
	`, sharedActiveID, sharedPendingID, homeActiveID, agentID); err != nil {
		t.Fatalf("create scoped Sessions: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO agent_runs (id,agent_id,session_id,trigger_type,channel_id,status)
		VALUES ($1,$7,$4,'message',$8,'completed'),
		       ($2,$7,$5,'message',$8,'completed'),
		       ($3,$7,$6,'message',$9,'completed')
	`, uuid.NewString(), uuid.NewString(), uuid.NewString(), sharedActiveID, sharedPendingID, homeActiveID, agentID, sharedID, homeID); err != nil {
		t.Fatalf("create scoped Runs: %v", err)
	}
	runSvc := NewAgentRunService(pool)
	sharedRun, err := runSvc.StartRun(ctx, StartRunInput{
		AgentID: agentID, TriggerType: AgentRunTriggerMessage, ChannelID: sharedID, Status: AgentRunStatusQueued,
	})
	if err != nil {
		t.Fatalf("create active shared Run: %v", err)
	}
	homeRun, err := runSvc.StartRun(ctx, StartRunInput{
		AgentID: agentID, TriggerType: AgentRunTriggerMessage, ChannelID: homeID, Status: AgentRunStatusQueued,
	})
	if err != nil {
		t.Fatalf("create active home Run: %v", err)
	}
	sharedScopedTaskID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO tasks (id,channel_id,creator_id,claimer_id,title,status,priority,task_number)
		VALUES ($1,$2,$3,$4,'scoped claimed task',$5,'normal',
		  (SELECT COALESCE(MAX(task_number),0)+1 FROM tasks WHERE channel_id=$2))`,
		sharedScopedTaskID, sharedID, ownerID, agentID, TaskStatusInReview,
	); err != nil {
		t.Fatalf("create scoped claimed Task: %v", err)
	}
	if _, err := svc.RemoveMember(ctx, sharedID, adminID, agentID); err != nil {
		t.Fatalf("Workspace admin remove connected Agent: %v", err)
	}
	statuses := map[string]string{}
	rows, err := pool.Query(ctx, `SELECT id::text,status FROM agent_sessions WHERE id::text=ANY($1)`, []string{sharedActiveID, sharedPendingID, homeActiveID})
	if err != nil {
		t.Fatalf("load scoped Session statuses: %v", err)
	}
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		statuses[id] = status
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	if statuses[sharedActiveID] != AgentSessionStatusClosed || statuses[sharedPendingID] != AgentSessionStatusClosed || statuses[homeActiveID] != AgentSessionStatusActive {
		t.Fatalf("scoped Session statuses = %#v", statuses)
	}
	var sharedRunStatus, sharedBudgetState, homeRunStatus, homeBudgetState string
	if err := pool.QueryRow(ctx, `
		SELECT r.status,u.state FROM agent_runs r JOIN agent_run_token_usage u ON u.run_id=r.id WHERE r.id=$1`, sharedRun.ID,
	).Scan(&sharedRunStatus, &sharedBudgetState); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT r.status,u.state FROM agent_runs r JOIN agent_run_token_usage u ON u.run_id=r.id WHERE r.id=$1`, homeRun.ID,
	).Scan(&homeRunStatus, &homeBudgetState); err != nil {
		t.Fatal(err)
	}
	if sharedRunStatus != string(AgentRunStatusCancelled) || sharedBudgetState != "released" ||
		homeRunStatus != string(AgentRunStatusQueued) || homeBudgetState != "pending" {
		t.Fatalf("scoped Runs = shared(%s,%s) home(%s,%s)", sharedRunStatus, sharedBudgetState, homeRunStatus, homeBudgetState)
	}
	var scopedTaskStatus, scopedTaskClaimer string
	if err := pool.QueryRow(ctx, `SELECT status,COALESCE(claimer_id::text,'') FROM tasks WHERE id=$1`, sharedScopedTaskID).Scan(&scopedTaskStatus, &scopedTaskClaimer); err != nil {
		t.Fatal(err)
	}
	if scopedTaskStatus != TaskStatusTodo || scopedTaskClaimer != "" {
		t.Fatalf("scoped Task = (%s,%s), want todo/unclaimed", scopedTaskStatus, scopedTaskClaimer)
	}
	if err := svc.AddMember(ctx, sharedID, ownerID, "agent", agentID); err != nil {
		t.Fatalf("owner reconnect Agent: %v", err)
	}
	if _, err := svc.RemoveMember(ctx, homeID, adminID, agentID); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("Workspace admin delete foreign Agent error = %v, want %v", err, ErrPermissionDenied)
	}
	sharedTaskID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO tasks (id,channel_id,creator_id,claimer_id,title,status,priority,task_number)
		VALUES ($1,$2,$3,$4,'shared claimed task',$5,'normal',
		  (SELECT COALESCE(MAX(task_number),0)+1 FROM tasks WHERE channel_id=$2))`,
		sharedTaskID, sharedID, ownerID, agentID, TaskStatusInProgress,
	); err != nil {
		t.Fatalf("create claimed shared Task: %v", err)
	}
	if _, err := svc.RemoveMember(ctx, homeID, ownerID, agentID); err != nil {
		t.Fatalf("owner remove Agent from home Channel: %v", err)
	}
	var taskStatus, taskClaimer, finalHomeRunStatus, finalHomeBudgetState string
	if err := pool.QueryRow(ctx, `SELECT status,COALESCE(claimer_id::text,'') FROM tasks WHERE id=$1`, sharedTaskID).Scan(&taskStatus, &taskClaimer); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT r.status,u.state FROM agent_runs r JOIN agent_run_token_usage u ON u.run_id=r.id WHERE r.id=$1`, homeRun.ID,
	).Scan(&finalHomeRunStatus, &finalHomeBudgetState); err != nil {
		t.Fatal(err)
	}
	if taskStatus != TaskStatusTodo || taskClaimer != "" || finalHomeRunStatus != string(AgentRunStatusCancelled) || finalHomeBudgetState != "released" {
		t.Fatalf("home removal = Task(%s,%s) Run(%s,%s)", taskStatus, taskClaimer, finalHomeRunStatus, finalHomeBudgetState)
	}
}
