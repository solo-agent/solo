package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ListTeamAgreements(ctx context.Context, pool *pgxpool.Pool, channelID, actorID string) (json.RawMessage, error) {
	if err := NewTaskService(pool).requireChannelMember(ctx, channelID, actorID); err != nil {
		return nil, err
	}
	var data json.RawMessage
	err := pool.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(p)||jsonb_build_object('from_name',fa.name,'to_name',ta.name,'from_owner_id',fa.owner_id,'to_owner_id',ta.owner_id,'proposed_by_agent_name',COALESCE(pa.name,'')) ORDER BY p.created_at DESC),'[]') FROM agent_relationship_proposals p JOIN agents fa ON fa.id=p.from_agent_id JOIN agents ta ON ta.id=p.to_agent_id LEFT JOIN agents pa ON pa.id=p.proposed_by_agent_id WHERE p.channel_id=$1`, channelID).Scan(&data)
	return data, err
}

func ProposeTeamAgreement(ctx context.Context, pool *pgxpool.Pool, channelID, actorID string, req CreateRelationshipRequest) (string, error) {
	if err := ValidateRelationshipCreate(req); err != nil {
		return "", invalidDelivery(err.Error())
	}
	if strings.TrimSpace(req.Instruction) == "" || len(req.Instruction) > 8000 {
		return "", invalidDelivery("a specific collaboration agreement is required")
	}
	if _, err := uuid.Parse(req.FromAgentID); err != nil {
		return "", invalidDelivery("invalid source Agent")
	}
	if _, err := uuid.Parse(req.ToAgentID); err != nil {
		return "", invalidDelivery("invalid target Agent")
	}
	if err := NewTaskService(pool).requireChannelMember(ctx, channelID, actorID); err != nil {
		return "", err
	}
	for _, id := range []string{req.FromAgentID, req.ToAgentID} {
		if err := NewTaskService(pool).requireChannelMember(ctx, channelID, id); err != nil {
			return "", err
		}
	}
	proposerOwner := actorID
	var proposerAgent *string
	if err := pool.QueryRow(ctx, `SELECT owner_id::text FROM agents WHERE id=$1 AND is_active`, actorID).Scan(&proposerOwner); err == nil {
		proposerAgent = &actorID
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	var id string
	err := pool.QueryRow(ctx, `INSERT INTO agent_relationship_proposals(channel_id,from_agent_id,to_agent_id,rel_type,instruction,proposed_by,proposed_by_agent_id) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text`, channelID, req.FromAgentID, req.ToAgentID, req.RelType, req.Instruction, proposerOwner, proposerAgent).Scan(&id)
	return id, err
}

func DecideTeamAgreement(ctx context.Context, pool *pgxpool.Pool, channelID, actorID, proposalID, decision string) error {
	if _, err := uuid.Parse(proposalID); err != nil {
		return invalidDelivery("invalid proposal")
	}
	if decision != "accept" && decision != "withdraw" {
		return invalidDelivery("decision must be accept or withdraw")
	}
	if err := NewTaskService(pool).requireChannelMember(ctx, channelID, actorID); err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = lockAgentRelationshipChannel(ctx, tx, channelID); err != nil {
		return err
	}
	var from, to, fromOwner, toOwner, kind, instruction, relationshipID, status, proposer, proposerAgent string
	err = tx.QueryRow(ctx, `SELECT p.from_agent_id::text,p.to_agent_id::text,fa.owner_id::text,ta.owner_id::text,p.rel_type,p.instruction,COALESCE(p.relationship_id::text,''),p.status,p.proposed_by::text,COALESCE(p.proposed_by_agent_id::text,'') FROM agent_relationship_proposals p JOIN agents fa ON fa.id=p.from_agent_id JOIN agents ta ON ta.id=p.to_agent_id WHERE p.id=$1 AND p.channel_id=$2 AND fa.is_active AND ta.is_active FOR UPDATE OF p`, proposalID, channelID).Scan(&from, &to, &fromOwner, &toOwner, &kind, &instruction, &relationshipID, &status, &proposer, &proposerAgent)
	if errors.Is(err, pgx.ErrNoRows) {
		return invalidDelivery("proposal not found")
	}
	if err != nil {
		return err
	}
	if actorID != fromOwner && actorID != toOwner && !(decision == "withdraw" && (actorID == proposer || (status == "pending" && actorID == proposerAgent))) {
		return ErrAgentWorkForbidden
	}
	if decision == "withdraw" {
		if relationshipID != "" {
			if _, err = tx.Exec(ctx, `DELETE FROM agent_relationships WHERE id=$1`, relationshipID); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE agent_relationship_proposals SET status='withdrawn' WHERE id=$1`, proposalID); err != nil {
			return err
		}
		// A revoked agreement must not survive inside the active team snapshot.
		if _, err = tx.Exec(ctx, `UPDATE channels SET team_version_id=NULL WHERE id=$1`, channelID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if status == "withdrawn" {
		return invalidDelivery("proposal was withdrawn")
	}
	if status == "accepted" {
		return tx.Commit(ctx)
	}
	var allAccepted bool
	err = tx.QueryRow(ctx, `UPDATE agent_relationship_proposals SET approved_owner_ids=CASE WHEN $2::uuid=ANY(approved_owner_ids) THEN approved_owner_ids ELSE array_append(approved_owner_ids,$2::uuid) END WHERE id=$1 RETURNING $3::uuid=ANY(approved_owner_ids) AND $4::uuid=ANY(approved_owner_ids)`, proposalID, actorID, fromOwner, toOwner).Scan(&allAccepted)
	if err != nil {
		return err
	}
	if !allAccepted {
		return tx.Commit(ctx)
	}
	var members int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM channel_members WHERE channel_id=$1 AND member_type='agent' AND member_id=ANY($2::uuid[])`, channelID, []string{from, to}).Scan(&members); err != nil {
		return err
	}
	if members != 2 {
		return invalidDelivery("an Agent withdrew from this channel")
	}
	if kind == RelAssignsTo {
		cycle, err := wouldCreateAssignsToCycle(ctx, tx, channelID, from, to, "")
		if err != nil {
			return err
		}
		if cycle {
			return ErrRelationshipCycle
		}
	}
	err = tx.QueryRow(ctx, `INSERT INTO agent_relationships(channel_id,from_agent_id,to_agent_id,rel_type,instruction) VALUES($1,$2,$3,$4,$5) RETURNING id::text`, channelID, from, to, kind, instruction).Scan(&relationshipID)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE agent_relationship_proposals SET status='accepted',relationship_id=$2 WHERE id=$1`, proposalID, relationshipID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
