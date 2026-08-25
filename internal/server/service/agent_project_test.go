package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestAgentUsesOwnersProjectMappingForActualComputer(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	ownerID := taskSubmitUser(t, pool)
	agentID := taskSubmitAgent(t, pool, ownerID)
	channelID := taskSubmitChannel(t, pool, ownerID)
	computerID := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO computers (id,name,owner_id,status) VALUES ($1,'project-test',$2,'online')`, computerID, ownerID); err != nil {
		t.Fatalf("create Computer: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE agents SET runtime_id=$1 WHERE id=$2`, computerID, agentID); err != nil {
		t.Fatalf("bind Agent Computer: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE channels SET project_source='repo',project_baseline='v1' WHERE id=$1`, channelID); err != nil {
		t.Fatalf("set Channel project: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO channel_project_mappings (channel_id,user_id,computer_id,local_path,version,access_mode) VALUES ($1,$2,$3,'/tmp/project-owner','v1','read_write')`, channelID, ownerID, computerID); err != nil {
		t.Fatalf("create project mapping: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM channel_project_mappings WHERE channel_id=$1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM computers WHERE id=$1`, computerID)
	})

	svc := &AgentService{pool: pool}
	req := daemonTaskRequest{AgentID: agentID, ChannelID: channelID}
	if err := svc.applyChannelProjectBinding(ctx, &DaemonInfo{ComputerID: computerID}, &req); err != nil {
		t.Fatalf("apply project mapping: %v", err)
	}
	if req.ProjectComputerID != computerID || req.ProjectPath != "/tmp/project-owner" || req.ProjectVersion != "v1" {
		t.Fatalf("project mapping = %+v", req)
	}

	if _, err := pool.Exec(ctx, `UPDATE channel_project_mappings SET access_mode='read_only' WHERE channel_id=$1 AND user_id=$2 AND computer_id=$3`, channelID, ownerID, computerID); err != nil {
		t.Fatalf("make mapping read-only: %v", err)
	}
	if err := svc.applyChannelProjectBinding(ctx, &DaemonInfo{ComputerID: computerID}, &req); err == nil || err.Error() != agentErrorProjectReadOnly {
		t.Fatalf("read-only mapping error = %v, want %s", err, agentErrorProjectReadOnly)
	}
	if _, err := pool.Exec(ctx, `UPDATE channel_project_mappings SET access_mode='read_write',version='v2' WHERE channel_id=$1 AND user_id=$2 AND computer_id=$3`, channelID, ownerID, computerID); err != nil {
		t.Fatalf("change mapping version: %v", err)
	}
	if err := svc.applyChannelProjectBinding(ctx, &DaemonInfo{ComputerID: computerID}, &req); err == nil || err.Error() != agentErrorProjectVersion {
		t.Fatalf("version mismatch error = %v, want %s", err, agentErrorProjectVersion)
	}
	otherComputerID := uuid.NewString()
	if err := svc.applyChannelProjectBinding(ctx, &DaemonInfo{ComputerID: otherComputerID}, &req); err == nil || err.Error() != agentErrorProjectMapping {
		t.Fatalf("missing mapping error = %v, want %s", err, agentErrorProjectMapping)
	}
}
