package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	serverworkspace "github.com/solo-ai/solo/internal/server/workspace"
)

type AgentRevisionApplicationRequest struct {
	ChannelID             string  `json:"channel_id"`
	SelectionID           string  `json:"selection_id,omitempty"`
	Restore               bool    `json:"restore,omitempty"`
	ExpectedTeamVersionID *string `json:"expected_team_version_id"`
	IdempotencyKey        string  `json:"idempotency_key"`
	Reason                string  `json:"reason"`
}

// ApplyAgentRevision records the owner's choice and one team's new snapshot in
// the same transaction. Global Agent configuration and existing Runs are untouched.
func ApplyAgentRevision(ctx context.Context, pool *pgxpool.Pool, agentID, actorID, revisionID string, req AgentRevisionApplicationRequest) (string, error) {
	for _, id := range []string{agentID, revisionID, req.ChannelID} {
		if _, err := uuid.Parse(id); err != nil {
			return "", invalidDelivery("invalid revision or target team")
		}
	}
	if req.ExpectedTeamVersionID == nil || !deliveryID.MatchString(req.IdempotencyKey) || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 8000 || (req.SelectionID == "") != req.Restore {
		return "", invalidDelivery("name the target team, observed version, adoption or restore, idempotency key and reason")
	}
	if *req.ExpectedTeamVersionID != "" {
		if _, err := uuid.Parse(*req.ExpectedTeamVersionID); err != nil {
			return "", invalidDelivery("invalid observed team version")
		}
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var currentTeamID string
	err = tx.QueryRow(ctx, `SELECT COALESCE(c.team_version_id::text,'') FROM channels c JOIN channel_members m ON m.channel_id=c.id AND m.member_type='user' AND m.member_id=$2 WHERE c.id=$1 AND c.type='channel' AND NOT c.is_archived AND ($3='' OR c.workspace_id::text=$3) FOR UPDATE OF c`, req.ChannelID, actorID, serverworkspace.FilterID(ctx)).Scan(&currentTeamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrAgentWorkForbidden
	}
	if err != nil {
		return "", err
	}
	var configHash string
	err = tx.QueryRow(ctx, `SELECT v.config_hash FROM agent_revisions v JOIN agents a ON a.id=v.agent_id JOIN channel_members m ON m.member_id=a.id AND m.member_type='agent' AND m.channel_id=$4 WHERE v.id=$1 AND a.id=$2 AND a.owner_id=$3 AND a.is_active`, revisionID, agentID, actorID, req.ChannelID).Scan(&configHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrAgentWorkForbidden
	}
	if err != nil {
		return "", err
	}
	var existingID, requestHash string
	err = tx.QueryRow(ctx, `SELECT id::text,lockfile->>'application_request_hash' FROM channel_team_versions WHERE channel_id=$1 AND created_by=$2 AND lockfile->>'application_key'=$3`, req.ChannelID, actorID, req.IdempotencyKey).Scan(&existingID, &requestHash)
	if err == nil {
		if requestHash != deliveryRequestHash(req) {
			return "", ErrTaskVersionConflict
		}
		return existingID, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if currentTeamID != *req.ExpectedTeamVersionID {
		return "", ErrTaskVersionConflict
	}
	var lockfile json.RawMessage
	if currentTeamID == "" {
		lockfile, err = captureTeamLockfileTx(ctx, tx, req.ChannelID, actorID)
	} else {
		err = tx.QueryRow(ctx, `SELECT lockfile FROM channel_team_versions WHERE id=$1 AND channel_id=$2`, currentTeamID, req.ChannelID).Scan(&lockfile)
	}
	if err != nil {
		return "", err
	}
	var previousRevision string
	err = tx.QueryRow(ctx, `SELECT member->>'revision_id' FROM jsonb_array_elements($1::jsonb->'members') member WHERE member->>'agent_id'=$2`, lockfile, agentID).Scan(&previousRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", invalidDelivery("Agent is not in the target team's current version")
	}
	if err != nil {
		return "", err
	}
	if previousRevision == revisionID {
		return "", invalidDelivery("this team already uses the requested revision")
	}
	if req.Restore {
		var valid bool
		if err = tx.QueryRow(ctx, `SELECT $1::jsonb->>'changed_agent_id'=$2 AND $1::jsonb->>'previous_revision_id'=$3`, lockfile, agentID, revisionID).Scan(&valid); err != nil || !valid {
			return "", invalidDelivery("restore must name this member's previous revision in the current team change")
		}
	} else {
		if _, err = uuid.Parse(req.SelectionID); err != nil {
			return "", invalidDelivery("invalid comparison")
		}
		var valid bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_selections s WHERE s.id=$1 AND s.agent_id=$2 AND s.candidate_revision_id=$3 AND s.baseline_revision_id=$4 AND s.plan->>'channel_id'=$5)`, req.SelectionID, agentID, revisionID, previousRevision, req.ChannelID).Scan(&valid); err != nil {
			return "", err
		}
		if !valid {
			return "", invalidDelivery("comparison does not match this team's current baseline")
		}
		if err = decideAgentSelectionTx(ctx, tx, agentID, actorID, req.SelectionID, "accepted", req.Reason); err != nil {
			return "", err
		}
	}
	var updated json.RawMessage
	err = tx.QueryRow(ctx, `SELECT jsonb_set($1::jsonb,'{members}',(SELECT jsonb_agg(CASE WHEN member->>'agent_id'=$2 THEN member || jsonb_build_object('revision_id',$3::text,'config_hash',$4::text) ELSE member END ORDER BY member->>'agent_id') FROM jsonb_array_elements($1::jsonb->'members') member)) || jsonb_build_object('changed_agent_id',$2::text,'previous_revision_id',$5::text,'based_on_version_id',$6::text,'selection_id',$7::text,'application_key',$8::text,'application_request_hash',$9::text)`, lockfile, agentID, revisionID, configHash, previousRevision, currentTeamID, req.SelectionID, req.IdempotencyKey, deliveryRequestHash(req)).Scan(&updated)
	if err != nil {
		return "", err
	}
	var versionID string
	err = tx.QueryRow(ctx, `INSERT INTO channel_team_versions(channel_id,lockfile,created_by,reason) VALUES($1,$2,$3,$4) RETURNING id::text`, req.ChannelID, updated, actorID, strings.TrimSpace(req.Reason)).Scan(&versionID)
	if err != nil {
		return "", err
	}
	if _, err = publishTeamVersionTx(ctx, tx, req.ChannelID, actorID, versionID, req.Reason); err != nil {
		return "", err
	}
	return versionID, tx.Commit(ctx)
}
