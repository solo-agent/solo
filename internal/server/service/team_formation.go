package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/realtime"
)

const (
	teamFormationMinMembers   = 2
	teamFormationMaxMembers   = 5
	teamFormationMaxOverrides = 25
	teamFormationDocRetries   = 3
	teamFormationDocTimeout   = 15 * time.Second
)

var (
	ErrTeamFormationForbidden      = errors.New("only Lucy can form a team from her onboarding channel")
	ErrTeamFormationSourceNotFound = errors.New("source user message not found")
	ErrTeamFormationInProgress     = errors.New("team formation is already in progress")
	ErrInvalidTeamFormationPlan    = errors.New("invalid team formation plan")
	errTeamFormationLeaseLost      = errors.New("team formation provisioning lease was lost")

	teamRefPattern      = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	teamTemplatePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,49}$`)
	messageIDPattern    = regexp.MustCompile(`^[0-9a-fA-F-]{4,36}$`)
)

// TeamFormationService is the single trusted boundary for Lucy's automatic
// team creation. Lucy proposes a declarative plan; this service authorizes,
// validates and provisions the complete team atomically.
type TeamFormationService struct {
	pool        *pgxpool.Pool
	mdGen       relationshipDocumentGenerator
	broadcaster realtime.Broadcaster
}

type relationshipDocumentGenerator interface {
	GenerateForAgent(context.Context, string) error
}

func NewTeamFormationService(pool *pgxpool.Pool, mdGen relationshipDocumentGenerator, broadcaster realtime.Broadcaster) *TeamFormationService {
	return &TeamFormationService{pool: pool, mdGen: mdGen, broadcaster: broadcaster}
}

type TeamFormationRequest struct {
	SourceChannelID string            `json:"source_channel_id"`
	SourceMessageID string            `json:"source_message_id"`
	Plan            TeamFormationPlan `json:"plan"`
}

type TeamFormationPlan struct {
	IntentSummary         string                              `json:"intent_summary"`
	Channel               TeamFormationChannel                `json:"channel"`
	Members               []TeamFormationMember               `json:"members"`
	RelationshipTemplate  string                              `json:"relationship_template"`
	RelationshipOverrides []TeamFormationRelationshipOverride `json:"relationship_overrides,omitempty"`
	Relationships         []TeamFormationRelation             `json:"resolved_relationships,omitempty"`
	// Tasks is accepted only to return an explicit migration error to older
	// callers. Automatic team formation no longer creates initial tasks.
	Tasks []TeamFormationTask `json:"tasks,omitempty"`
}

type TeamFormationChannel struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type TeamFormationMember struct {
	Ref             string `json:"ref"`
	Role            string `json:"role"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	Instructions    string `json:"instructions"`
	ExistingAgentID string `json:"existing_agent_id,omitempty"`
}

type TeamFormationRelation struct {
	FromRef     string  `json:"from_ref"`
	ToRef       string  `json:"to_ref"`
	Type        string  `json:"type"`
	Instruction string  `json:"instruction,omitempty"`
	Weight      float64 `json:"weight,omitempty"`
}

type TeamFormationRelationshipOverride struct {
	Operation   string  `json:"operation"`
	FromRef     string  `json:"from_ref"`
	ToRef       string  `json:"to_ref"`
	Type        string  `json:"type"`
	Instruction string  `json:"instruction,omitempty"`
	Weight      float64 `json:"weight,omitempty"`
	Reason      string  `json:"reason"`
}

type TeamFormationTask struct {
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	Priority           string   `json:"priority,omitempty"`
	SuggestedOwnerRef  string   `json:"suggested_owner_ref,omitempty"`
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
}

type TeamFormationResult struct {
	FormationID           string                      `json:"formation_id"`
	SourceChannelID       string                      `json:"source_channel_id"`
	SourceMessageID       string                      `json:"source_message_id"`
	ChannelID             string                      `json:"channel_id"`
	ChannelName           string                      `json:"channel_name"`
	DashboardURL          string                      `json:"dashboard_url"`
	Members               []TeamFormationResultMember `json:"members"`
	Tasks                 []TeamFormationResultTask   `json:"tasks"`
	RelationshipTemplate  string                      `json:"relationship_template,omitempty"`
	RelationshipOverrides int                         `json:"relationship_override_count"`
	RelationshipCount     int                         `json:"relationship_count"`
	RelationshipDocsReady bool                        `json:"relationship_docs_ready"`
	Warnings              []string                    `json:"warnings,omitempty"`
	Replayed              bool                        `json:"replayed"`
	CreatedAt             time.Time                   `json:"created_at"`
}

type TeamFormationResultMember struct {
	Ref     string `json:"ref"`
	Role    string `json:"role"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Created bool   `json:"created"`
}

type TeamFormationResultTask struct {
	ID                 string `json:"id"`
	MessageID          string `json:"message_id"`
	Number             int    `json:"number"`
	Title              string `json:"title"`
	SuggestedOwnerName string `json:"suggested_owner_name,omitempty"`
}

type teamFormationCaller struct {
	AgentID     string
	OwnerID     string
	Provider    string
	ModelName   string
	RuntimeID   string
	ChannelID   string
	ChannelName string
	SourceID    string
	SourceText  string
}

// Form creates a team once for a source user message. Repeating the same
// request returns the stored result rather than creating duplicate resources.
func (s *TeamFormationService) Form(ctx context.Context, callerID string, req TeamFormationRequest) (*TeamFormationResult, error) {
	req.SourceChannelID = strings.TrimSpace(req.SourceChannelID)
	req.SourceMessageID = strings.TrimSpace(req.SourceMessageID)
	if req.SourceChannelID == "" || req.SourceMessageID == "" {
		return nil, fmt.Errorf("%w: source_channel_id and source_message_id are required", ErrInvalidTeamFormationPlan)
	}
	if !messageIDPattern.MatchString(req.SourceMessageID) {
		return nil, fmt.Errorf("%w: source_message_id must be a UUID or its short prefix", ErrInvalidTeamFormationPlan)
	}
	if err := normalizeAndValidateTeamPlan(&req.Plan); err != nil {
		return nil, err
	}

	caller, err := s.authorizeCaller(ctx, callerID, req.SourceChannelID, req.SourceMessageID)
	if err != nil {
		return nil, err
	}
	req.SourceMessageID = caller.SourceID
	if err := s.resolveTemplateRelationships(ctx, &req.Plan); err != nil {
		return nil, err
	}

	planJSON, err := json.Marshal(req.Plan)
	if err != nil {
		return nil, fmt.Errorf("marshal team plan: %w", err)
	}

	formationID, leaseUpdatedAt, replay, err := s.claimFormation(ctx, caller, planJSON)
	if err != nil {
		return nil, err
	}
	if replay != nil {
		s.finalizeRelationshipDocuments(replay)
		s.persistFinalizedResult(replay)
		return replay, nil
	}

	result, err := s.provision(ctx, formationID, leaseUpdatedAt, caller, req.Plan)
	if err != nil {
		if errors.Is(err, errTeamFormationLeaseLost) {
			return nil, ErrTeamFormationInProgress
		}
		failure := truncateRunes(err.Error(), 4000)
		failureCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = s.pool.Exec(failureCtx, `
			UPDATE team_formations
			   SET status = 'failed', error = $2, updated_at = now()
			 WHERE id = $1 AND status = 'provisioning'
			   AND target_channel_id IS NULL AND updated_at = $3`, formationID, failure, leaseUpdatedAt)
		return nil, err
	}

	s.finalizeRelationshipDocuments(result)
	s.persistFinalizedResult(result)
	if s.broadcaster != nil {
		s.broadcaster.SendToUser(caller.OwnerID, realtime.Envelope("team.formed", map[string]any{
			"formation_id":            result.FormationID,
			"source_channel_id":       result.SourceChannelID,
			"source_message_id":       result.SourceMessageID,
			"channel_id":              result.ChannelID,
			"channel_name":            result.ChannelName,
			"member_count":            len(result.Members),
			"task_count":              0,
			"relationship_template":   result.RelationshipTemplate,
			"relationship_overrides":  result.RelationshipOverrides,
			"relationship_docs_ready": result.RelationshipDocsReady,
			"warnings":                result.Warnings,
			"dashboard_url":           result.DashboardURL,
			"created_at":              result.CreatedAt.Format(time.RFC3339),
		}))
	}
	return result, nil
}

// finalizeRelationshipDocuments is deliberately detached from the request
// context: at this point the database transaction is committed, so a client
// disconnect must not leave the newly-created agents without their runtime
// relationship contract. Failed generation is visible in the result and the
// same idempotency key will retry it on the next request.
func (s *TeamFormationService) finalizeRelationshipDocuments(result *TeamFormationResult) {
	result.Warnings = nil
	if s.mdGen == nil {
		result.RelationshipDocsReady = true
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), teamFormationDocTimeout)
	defer cancel()
	failedNames := make([]string, 0)
	for _, member := range result.Members {
		var lastErr error
		for attempt := 1; attempt <= teamFormationDocRetries; attempt++ {
			lastErr = s.mdGen.GenerateForAgent(ctx, member.ID)
			if lastErr == nil {
				break
			}
			if attempt < teamFormationDocRetries {
				delay := time.Duration(attempt) * 100 * time.Millisecond
				select {
				case <-ctx.Done():
					lastErr = ctx.Err()
				case <-time.After(delay):
				}
			}
		}
		if lastErr != nil {
			failedNames = append(failedNames, "@"+member.Name)
			slog.Error("team formation relationship document generation failed",
				"formation_id", result.FormationID,
				"agent_id", member.ID,
				"agent_name", member.Name,
				"attempts", teamFormationDocRetries,
				"error", lastErr,
			)
		}
	}

	result.RelationshipDocsReady = len(failedNames) == 0
	if !result.RelationshipDocsReady {
		result.Warnings = []string{
			"Relationship documents are not ready for " + strings.Join(failedNames, ", ") + ". Retry the same team formation request to repair them.",
		}
	}
}

func (s *TeamFormationService) persistFinalizedResult(result *TeamFormationResult) {
	stored := *result
	stored.Replayed = false
	resultJSON, err := json.Marshal(&stored)
	if err != nil {
		slog.Error("marshal finalized team formation result", "formation_id", result.FormationID, "error", err)
		result.Warnings = append(result.Warnings, "Team readiness state could not be persisted; retry the same request to verify it.")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.pool.Exec(ctx, `
		UPDATE team_formations
		   SET result = $2, updated_at = now()
		 WHERE id = $1 AND status = 'completed'`, result.FormationID, resultJSON); err != nil {
		slog.Error("persist finalized team formation result", "formation_id", result.FormationID, "error", err)
		result.Warnings = append(result.Warnings, "Team readiness state could not be persisted; retry the same request to verify it.")
	}
}

func (s *TeamFormationService) authorizeCaller(ctx context.Context, callerID, channelID, messagePrefix string) (*teamFormationCaller, error) {
	caller := teamFormationCaller{ChannelID: channelID}
	var agentName string
	err := s.pool.QueryRow(ctx, `
		SELECT a.id, a.owner_id, a.name, a.model_provider, a.model_name,
		       COALESCE(a.runtime_id, ''), c.name
		  FROM agents a
		  JOIN channels c ON c.id = $2
		  JOIN channel_members acm
		    ON acm.channel_id = c.id
		   AND acm.member_type = 'agent'
		   AND acm.member_id = a.id
		  JOIN channel_members ucm
		    ON ucm.channel_id = c.id
		   AND ucm.member_type = 'user'
		   AND ucm.member_id = a.owner_id
		 WHERE a.id = $1
		   AND a.is_active = true
		   AND c.type = 'channel'
		   AND c.is_archived = false
		   AND c.created_by = a.owner_id
		   AND c.name LIKE 'welcome-%%'`, callerID, channelID).Scan(
		&caller.AgentID, &caller.OwnerID, &agentName, &caller.Provider,
		&caller.ModelName, &caller.RuntimeID, &caller.ChannelName,
	)
	if err != nil || !strings.EqualFold(agentName, "Lucy") {
		return nil, ErrTeamFormationForbidden
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id::text, content
		  FROM messages
		 WHERE channel_id = $1
		   AND sender_type = 'user'
		   AND sender_id = $2
		   AND thread_id IS NULL
		   AND is_deleted = false
		   AND lower(id::text) LIKE lower($3) || '%%'
		 ORDER BY created_at DESC
		 LIMIT 2`, channelID, caller.OwnerID, messagePrefix)
	if err != nil {
		return nil, fmt.Errorf("resolve source message: %w", err)
	}
	defer rows.Close()
	var count int
	for rows.Next() {
		if err := rows.Scan(&caller.SourceID, &caller.SourceText); err != nil {
			return nil, fmt.Errorf("scan source message: %w", err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("resolve source message: %w", err)
	}
	if count != 1 {
		return nil, ErrTeamFormationSourceNotFound
	}
	return &caller, nil
}

func (s *TeamFormationService) claimFormation(ctx context.Context, caller *teamFormationCaller, planJSON []byte) (string, time.Time, *TeamFormationResult, error) {
	var formationID string
	var leaseUpdatedAt time.Time
	err := s.pool.QueryRow(ctx, `
		INSERT INTO team_formations (
			owner_id, requested_by_agent_id, source_channel_id,
			source_message_id, status, plan
		) VALUES ($1, $2, $3, $4, 'provisioning', $5)
		ON CONFLICT (source_message_id) DO NOTHING
		RETURNING id, updated_at`, caller.OwnerID, caller.AgentID, caller.ChannelID, caller.SourceID, planJSON).Scan(&formationID, &leaseUpdatedAt)
	if err == nil {
		return formationID, leaseUpdatedAt, nil, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", time.Time{}, nil, fmt.Errorf("create team formation: %w", err)
	}

	var status string
	var resultJSON []byte
	var updatedAt time.Time
	err = s.pool.QueryRow(ctx, `
		SELECT id, status, result, updated_at
		  FROM team_formations
		 WHERE source_message_id = $1`, caller.SourceID).Scan(&formationID, &status, &resultJSON, &updatedAt)
	if err != nil {
		return "", time.Time{}, nil, fmt.Errorf("load team formation: %w", err)
	}
	if status == "completed" && len(resultJSON) > 0 {
		var result TeamFormationResult
		if err := json.Unmarshal(resultJSON, &result); err != nil {
			return "", time.Time{}, nil, fmt.Errorf("decode stored team formation result: %w", err)
		}
		result.Replayed = true
		return formationID, time.Time{}, &result, nil
	}

	if status == "failed" || (status == "provisioning" && time.Since(updatedAt) > 2*time.Minute) {
		updateErr := s.pool.QueryRow(ctx, `
			UPDATE team_formations
			   SET status = 'provisioning', plan = $2, result = NULL,
			       target_channel_id = NULL, error = '', updated_at = now()
			 WHERE id = $1
			   AND status = $3 AND updated_at = $4
			   AND ($3 = 'failed' OR updated_at < now() - interval '2 minutes')
			RETURNING updated_at`, formationID, planJSON, status, updatedAt).Scan(&leaseUpdatedAt)
		if updateErr != nil {
			if !errors.Is(updateErr, pgx.ErrNoRows) {
				return "", time.Time{}, nil, fmt.Errorf("retry team formation: %w", updateErr)
			}
		} else {
			return formationID, leaseUpdatedAt, nil, nil
		}
	}
	return "", time.Time{}, nil, ErrTeamFormationInProgress
}

func (s *TeamFormationService) provision(ctx context.Context, formationID string, leaseUpdatedAt time.Time, caller *teamFormationCaller, plan TeamFormationPlan) (*TeamFormationResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin team formation: %w", err)
	}
	defer tx.Rollback(ctx)

	// Channel and agent names have partial unique indexes. Serialize the small
	// provisioning critical section so suffix selection cannot race.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('solo-team-formation'))`); err != nil {
		return nil, fmt.Errorf("lock team formation: %w", err)
	}
	var currentStatus string
	var currentUpdatedAt time.Time
	if err := tx.QueryRow(ctx, `
		SELECT status, updated_at
		  FROM team_formations
		 WHERE id = $1
		 FOR UPDATE`, formationID).Scan(&currentStatus, &currentUpdatedAt); err != nil {
		return nil, fmt.Errorf("lock team formation lease: %w", err)
	}
	if currentStatus != "provisioning" || !currentUpdatedAt.Equal(leaseUpdatedAt) {
		return nil, errTeamFormationLeaseLost
	}

	channelName, err := uniqueChannelName(ctx, tx, plan.Channel.Name)
	if err != nil {
		return nil, err
	}
	channelID := uuid.New().String()
	var channelCreatedAt time.Time
	if err := tx.QueryRow(ctx, `
		INSERT INTO channels (id, name, description, type, created_by)
		VALUES ($1, $2, $3, 'channel', $4)
		RETURNING created_at`, channelID, channelName, plan.Channel.Description, caller.OwnerID).Scan(&channelCreatedAt); err != nil {
		return nil, fmt.Errorf("create team channel: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO channel_members (channel_id, member_type, member_id, role)
		VALUES ($1, 'user', $2, 'owner')`, channelID, caller.OwnerID); err != nil {
		return nil, fmt.Errorf("add owner to team channel: %w", err)
	}

	runtimeID := caller.RuntimeID
	if runtimeID == "" {
		_ = tx.QueryRow(ctx, `
			SELECT c.id
			  FROM computers c
			 WHERE c.status = 'online'
			   AND (c.owner_id = $1 OR EXISTS (
			       SELECT 1 FROM computer_members cm
			        WHERE cm.computer_id = c.id AND cm.user_id = $1
			   ))
			 ORDER BY c.created_at ASC
			 LIMIT 1`, caller.OwnerID).Scan(&runtimeID)
	}

	var allChannelID string
	_ = tx.QueryRow(ctx, `
		SELECT c.id
		  FROM channels c
		  JOIN channel_members cm ON cm.channel_id = c.id
		 WHERE cm.member_type = 'user'
		   AND cm.member_id = $1
		   AND c.name LIKE 'all-%%'
		   AND c.is_archived = false
		 ORDER BY c.created_at ASC
		 LIMIT 1`, caller.OwnerID).Scan(&allChannelID)

	membersByRef := make(map[string]TeamFormationResultMember, len(plan.Members))
	seenAgentIDs := make(map[string]bool, len(plan.Members))
	resultMembers := make([]TeamFormationResultMember, 0, len(plan.Members))
	for _, member := range plan.Members {
		resolved, resolveErr := s.resolveOrCreateMember(ctx, tx, caller, runtimeID, member)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if seenAgentIDs[resolved.ID] {
			return nil, fmt.Errorf("%w: multiple member refs resolve to agent %s", ErrInvalidTeamFormationPlan, resolved.Name)
		}
		seenAgentIDs[resolved.ID] = true
		membersByRef[member.Ref] = resolved
		resultMembers = append(resultMembers, resolved)

		if _, err := tx.Exec(ctx, `
			INSERT INTO channel_members (channel_id, member_type, member_id, role)
			VALUES ($1, 'agent', $2, 'member')
			ON CONFLICT DO NOTHING`, channelID, resolved.ID); err != nil {
			return nil, fmt.Errorf("add agent %s to team channel: %w", resolved.Name, err)
		}
		if allChannelID != "" {
			if _, err := tx.Exec(ctx, `
				INSERT INTO channel_members (channel_id, member_type, member_id, role)
				VALUES ($1, 'agent', $2, 'member')
				ON CONFLICT DO NOTHING`, allChannelID, resolved.ID); err != nil {
				return nil, fmt.Errorf("add agent %s to all channel: %w", resolved.Name, err)
			}
		}
	}

	relationshipCount := 0
	for _, relation := range plan.Relationships {
		from := membersByRef[relation.FromRef]
		to := membersByRef[relation.ToRef]
		_, relErr := upsertTeamRelationship(ctx, tx, from.ID, to.ID, relation)
		if relErr != nil {
			return nil, relErr
		}
		relationshipCount++
	}

	kickoff := buildTeamKickoffMessage(plan, caller.SourceText, channelName, resultMembers)
	if _, err := tx.Exec(ctx, `
		INSERT INTO messages (
			id, channel_id, sender_type, sender_id, content, content_type,
			created_at, updated_at
		) VALUES ($1, $2, 'system', $3, $4, 'system', now(), now())`,
		uuid.New().String(), channelID, uuid.Nil.String(), kickoff); err != nil {
		return nil, fmt.Errorf("create team kickoff message: %w", err)
	}

	result := &TeamFormationResult{
		FormationID:           formationID,
		SourceChannelID:       caller.ChannelID,
		SourceMessageID:       caller.SourceID,
		ChannelID:             channelID,
		ChannelName:           channelName,
		DashboardURL:          "/dashboard?channel=" + channelID,
		Members:               resultMembers,
		Tasks:                 []TeamFormationResultTask{},
		RelationshipTemplate:  plan.RelationshipTemplate,
		RelationshipOverrides: len(plan.RelationshipOverrides),
		RelationshipCount:     relationshipCount,
		CreatedAt:             channelCreatedAt,
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal team formation result: %w", err)
	}
	commandTag, err := tx.Exec(ctx, `
		UPDATE team_formations
		   SET target_channel_id = $2, status = 'completed', result = $3,
		       error = '', updated_at = now()
		 WHERE id = $1 AND status = 'provisioning' AND updated_at = $4`, formationID, channelID, resultJSON, leaseUpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("complete team formation: %w", err)
	}
	if commandTag.RowsAffected() != 1 {
		return nil, errTeamFormationLeaseLost
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit team formation: %w", err)
	}
	return result, nil
}

func (s *TeamFormationService) resolveOrCreateMember(ctx context.Context, tx pgx.Tx, caller *teamFormationCaller, runtimeID string, member TeamFormationMember) (TeamFormationResultMember, error) {
	if member.ExistingAgentID != "" {
		existingID := member.ExistingAgentID
		if strings.EqualFold(existingID, "self") {
			existingID = caller.AgentID
		}
		var name string
		err := tx.QueryRow(ctx, `
			SELECT name
			  FROM agents
			 WHERE id = $1 AND owner_id = $2 AND is_active = true`, existingID, caller.OwnerID).Scan(&name)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return TeamFormationResultMember{}, fmt.Errorf("%w: existing agent for member %s is unavailable", ErrInvalidTeamFormationPlan, member.Ref)
			}
			return TeamFormationResultMember{}, fmt.Errorf("resolve existing agent for %s: %w", member.Ref, err)
		}
		return TeamFormationResultMember{Ref: member.Ref, Role: member.Role, ID: existingID, Name: name}, nil
	}

	name, err := uniqueAgentName(ctx, tx, caller.OwnerID, member.Name)
	if err != nil {
		return TeamFormationResultMember{}, err
	}
	agentID := uuid.New().String()
	runtimeValue := any(nil)
	if runtimeID != "" {
		runtimeValue = runtimeID
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO agents (
			id, name, description, owner_id, model_provider, model_name,
			system_prompt, runtime_id, custom_env, custom_args
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, '{}'::jsonb, '[]'::jsonb)`,
		agentID, name, member.Description, caller.OwnerID, caller.Provider,
		caller.ModelName, member.Instructions, runtimeValue); err != nil {
		return TeamFormationResultMember{}, fmt.Errorf("create agent %s: %w", name, err)
	}
	return TeamFormationResultMember{
		Ref: member.Ref, Role: member.Role, ID: agentID, Name: name, Created: true,
	}, nil
}

func upsertTeamRelationship(ctx context.Context, tx pgx.Tx, fromID, toID string, relation TeamFormationRelation) (bool, error) {
	lookupFrom, lookupTo := fromID, toID
	if relation.Type == "collaborates_with" && lookupFrom > lookupTo {
		lookupFrom, lookupTo = lookupTo, lookupFrom
	}
	var existingID string
	query := `SELECT id FROM agent_relationships WHERE from_agent_id = $1 AND to_agent_id = $2 AND rel_type = $3`
	if relation.Type == "collaborates_with" {
		query = `SELECT id FROM agent_relationships
		         WHERE LEAST(from_agent_id, to_agent_id) = $1
		           AND GREATEST(from_agent_id, to_agent_id) = $2
		           AND rel_type = $3`
	}
	err := tx.QueryRow(ctx, query, lookupFrom, lookupTo, relation.Type).Scan(&existingID)
	if err == nil {
		if _, updateErr := tx.Exec(ctx, `
			UPDATE agent_relationships
			   SET weight = $2, instruction = $3, updated_at = now()
			 WHERE id = $1`, existingID, relation.Weight, relation.Instruction); updateErr != nil {
			return false, fmt.Errorf("update relationship %s -> %s: %w", relation.FromRef, relation.ToRef, updateErr)
		}
		return false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("check relationship: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO agent_relationships (
			id, from_agent_id, to_agent_id, rel_type, weight, instruction
		) VALUES ($1, $2, $3, $4, $5, $6)`, uuid.New().String(), fromID, toID,
		relation.Type, relation.Weight, relation.Instruction); err != nil {
		return false, fmt.Errorf("create relationship %s -> %s: %w", relation.FromRef, relation.ToRef, err)
	}
	return true, nil
}

func uniqueChannelName(ctx context.Context, tx pgx.Tx, requested string) (string, error) {
	base := slugifyTeamName(requested)
	for suffix := 1; suffix <= 999; suffix++ {
		candidate := base
		if suffix > 1 {
			candidate = truncateRunes(base, 92) + fmt.Sprintf("-%d", suffix)
		}
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM channels
				 WHERE name = $1 AND type = 'channel' AND is_archived = false
			)`, candidate).Scan(&exists); err != nil {
			return "", fmt.Errorf("check channel name: %w", err)
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no available channel name for %q", requested)
}

func uniqueAgentName(ctx context.Context, tx pgx.Tx, ownerID, requested string) (string, error) {
	base := strings.TrimSpace(requested)
	for suffix := 1; suffix <= 999; suffix++ {
		candidate := base
		if suffix > 1 {
			candidate = truncateRunes(base, 92) + fmt.Sprintf("-%d", suffix)
		}
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM agents
				 WHERE owner_id = $1 AND lower(name) = lower($2) AND is_active = true
			)`, ownerID, candidate).Scan(&exists); err != nil {
			return "", fmt.Errorf("check agent name: %w", err)
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no available agent name for %q", requested)
}

func normalizeAndValidateTeamPlan(plan *TeamFormationPlan) error {
	plan.IntentSummary = strings.TrimSpace(plan.IntentSummary)
	plan.Channel.Name = strings.TrimSpace(plan.Channel.Name)
	plan.Channel.Description = strings.TrimSpace(plan.Channel.Description)
	plan.RelationshipTemplate = strings.ToLower(strings.TrimSpace(plan.RelationshipTemplate))
	if plan.IntentSummary == "" || len([]rune(plan.IntentSummary)) > 2000 {
		return fmt.Errorf("%w: intent_summary is required and must be at most 2000 characters", ErrInvalidTeamFormationPlan)
	}
	if plan.Channel.Name == "" || len([]rune(plan.Channel.Name)) > 100 {
		return fmt.Errorf("%w: channel.name is required and must be at most 100 characters", ErrInvalidTeamFormationPlan)
	}
	if len([]rune(plan.Channel.Description)) > 1000 {
		return fmt.Errorf("%w: channel.description must be at most 1000 characters", ErrInvalidTeamFormationPlan)
	}
	if plan.Channel.Description == "" {
		plan.Channel.Description = plan.IntentSummary
	}
	if !teamTemplatePattern.MatchString(plan.RelationshipTemplate) {
		return fmt.Errorf("%w: relationship_template is required and must be a valid template ID", ErrInvalidTeamFormationPlan)
	}
	if len(plan.Tasks) > 0 {
		return fmt.Errorf("%w: automatic team formation does not create kickoff tasks; remove tasks", ErrInvalidTeamFormationPlan)
	}
	if len(plan.Relationships) > 0 {
		return fmt.Errorf("%w: resolved_relationships is server-managed; use relationship_overrides", ErrInvalidTeamFormationPlan)
	}
	if len(plan.Members) < teamFormationMinMembers || len(plan.Members) > teamFormationMaxMembers {
		return fmt.Errorf("%w: members must contain %d-%d agents", ErrInvalidTeamFormationPlan, teamFormationMinMembers, teamFormationMaxMembers)
	}

	refs := make(map[string]bool, len(plan.Members))
	leaders := 0
	for i := range plan.Members {
		member := &plan.Members[i]
		member.Ref = strings.ToLower(strings.TrimSpace(member.Ref))
		member.Role = strings.TrimSpace(member.Role)
		member.Name = strings.TrimSpace(member.Name)
		member.Description = strings.TrimSpace(member.Description)
		member.Instructions = strings.TrimSpace(member.Instructions)
		member.ExistingAgentID = strings.TrimSpace(member.ExistingAgentID)
		if member.ExistingAgentID != "" && !strings.EqualFold(member.ExistingAgentID, "self") {
			if _, err := uuid.Parse(member.ExistingAgentID); err != nil {
				return fmt.Errorf("%w: member %s existing_agent_id must be a UUID or self", ErrInvalidTeamFormationPlan, member.Ref)
			}
		}
		if !teamRefPattern.MatchString(member.Ref) || refs[member.Ref] {
			return fmt.Errorf("%w: member refs must be unique lowercase identifiers", ErrInvalidTeamFormationPlan)
		}
		refs[member.Ref] = true
		if strings.EqualFold(member.Role, "leader") {
			leaders++
		}
		if member.Role == "" || len([]rune(member.Role)) > 50 {
			return fmt.Errorf("%w: member %s has an invalid role", ErrInvalidTeamFormationPlan, member.Ref)
		}
		if member.ExistingAgentID == "" {
			member.Name = sanitizeAgentHandle(member.Name)
			if member.Name == "" || len([]rune(member.Name)) > 100 {
				return fmt.Errorf("%w: member %s name is required and must be at most 100 characters", ErrInvalidTeamFormationPlan, member.Ref)
			}
			if member.Instructions == "" || len([]rune(member.Instructions)) > 8000 {
				return fmt.Errorf("%w: member %s instructions are required and must be at most 8000 characters", ErrInvalidTeamFormationPlan, member.Ref)
			}
		}
		if len([]rune(member.Description)) > 1000 {
			return fmt.Errorf("%w: member %s description is too long", ErrInvalidTeamFormationPlan, member.Ref)
		}
	}
	if leaders == 0 {
		plan.Members[0].Role = "leader"
	} else if leaders > 1 {
		return fmt.Errorf("%w: exactly one member may have role leader", ErrInvalidTeamFormationPlan)
	}

	if len(plan.RelationshipOverrides) > teamFormationMaxOverrides {
		return fmt.Errorf("%w: relationship_overrides may contain at most %d entries", ErrInvalidTeamFormationPlan, teamFormationMaxOverrides)
	}
	overrideKeys := make(map[string]bool, len(plan.RelationshipOverrides))
	for i := range plan.RelationshipOverrides {
		override := &plan.RelationshipOverrides[i]
		override.Operation = strings.ToLower(strings.TrimSpace(override.Operation))
		override.FromRef = strings.ToLower(strings.TrimSpace(override.FromRef))
		override.ToRef = strings.ToLower(strings.TrimSpace(override.ToRef))
		override.Type = strings.ToLower(strings.TrimSpace(override.Type))
		override.Instruction = strings.TrimSpace(override.Instruction)
		override.Reason = strings.TrimSpace(override.Reason)
		if override.Operation != "upsert" && override.Operation != "remove" {
			return fmt.Errorf("%w: relationship override %d operation must be upsert or remove", ErrInvalidTeamFormationPlan, i+1)
		}
		if !refs[override.FromRef] || !refs[override.ToRef] || override.FromRef == override.ToRef {
			return fmt.Errorf("%w: relationship override %d references are invalid", ErrInvalidTeamFormationPlan, i+1)
		}
		if override.Type != "assigns_to" && override.Type != "collaborates_with" {
			return fmt.Errorf("%w: relationship override %d type must be assigns_to or collaborates_with", ErrInvalidTeamFormationPlan, i+1)
		}
		if override.Reason == "" || len([]rune(override.Reason)) > 500 {
			return fmt.Errorf("%w: relationship override %d reason is required and must be at most 500 characters", ErrInvalidTeamFormationPlan, i+1)
		}
		if override.Operation == "upsert" && override.Weight == 0 {
			override.Weight = 1
		}
		if override.Weight < 0 || override.Weight > 10 || len([]rune(override.Instruction)) > 2000 {
			return fmt.Errorf("%w: relationship override %d weight or instruction is invalid", ErrInvalidTeamFormationPlan, i+1)
		}
		key := relationshipKey(override.FromRef, override.ToRef, override.Type)
		if overrideKeys[key] {
			return fmt.Errorf("%w: duplicate relationship override %s", ErrInvalidTeamFormationPlan, key)
		}
		overrideKeys[key] = true
	}
	return nil
}

func (s *TeamFormationService) resolveTemplateRelationships(ctx context.Context, plan *TeamFormationPlan) error {
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		SELECT members
		  FROM agent_templates
		 WHERE id = $1 AND is_official = true`, plan.RelationshipTemplate).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: relationship_template %q is not an official template", ErrInvalidTeamFormationPlan, plan.RelationshipTemplate)
		}
		return fmt.Errorf("load relationship template: %w", err)
	}
	var templateMembers []templateMember
	if err := json.Unmarshal(raw, &templateMembers); err != nil {
		return fmt.Errorf("parse relationship template %q: %w", plan.RelationshipTemplate, err)
	}
	return applyRelationshipTemplate(plan, templateMembers)
}

func applyRelationshipTemplate(plan *TeamFormationPlan, templateMembers []templateMember) error {
	templateLeaderCount := 0
	templateNonLeaders := make([]templateMember, 0, len(templateMembers))
	for _, member := range templateMembers {
		if strings.EqualFold(member.Role, "leader") {
			templateLeaderCount++
			continue
		}
		if member.Relationship != nil {
			templateNonLeaders = append(templateNonLeaders, member)
		}
	}
	if templateLeaderCount != 1 || len(templateNonLeaders) == 0 {
		return fmt.Errorf("%w: relationship_template %q does not define a usable leader topology", ErrInvalidTeamFormationPlan, plan.RelationshipTemplate)
	}

	leaderRef := ""
	refs := make(map[string]bool, len(plan.Members))
	for _, member := range plan.Members {
		refs[member.Ref] = true
		if strings.EqualFold(member.Role, "leader") {
			leaderRef = member.Ref
		}
	}
	relations := make(map[string]TeamFormationRelation, len(plan.Members)+len(plan.RelationshipOverrides))
	roleOrdinals := make(map[string]int)
	for _, member := range plan.Members {
		if member.Ref == leaderRef {
			continue
		}
		roleKey := strings.ToLower(member.Role)
		roleOrdinal := roleOrdinals[roleKey]
		instruction := relationshipInstructionForMember(plan.RelationshipTemplate, member, roleOrdinal, templateNonLeaders)
		relation := TeamFormationRelation{
			FromRef: leaderRef, ToRef: member.Ref, Type: "assigns_to", Weight: 1,
			Instruction: instruction,
		}
		relations[relationshipKey(relation.FromRef, relation.ToRef, relation.Type)] = relation
		roleOrdinals[roleKey] = roleOrdinal + 1
	}

	for _, override := range plan.RelationshipOverrides {
		key := relationshipKey(override.FromRef, override.ToRef, override.Type)
		if override.Operation == "remove" {
			if _, exists := relations[key]; !exists {
				return fmt.Errorf("%w: relationship override cannot remove missing relation %s", ErrInvalidTeamFormationPlan, key)
			}
			delete(relations, key)
			continue
		}
		relations[key] = TeamFormationRelation{
			FromRef: override.FromRef, ToRef: override.ToRef, Type: override.Type,
			Instruction: override.Instruction, Weight: override.Weight,
		}
	}

	keys := make([]string, 0, len(relations))
	for key := range relations {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	plan.Relationships = make([]TeamFormationRelation, 0, len(keys))
	for _, key := range keys {
		plan.Relationships = append(plan.Relationships, relations[key])
	}
	if err := validateResolvedRelationships(plan.Relationships, refs); err != nil {
		return err
	}
	if err := validateRelationshipConnectivity(plan.Relationships, leaderRef, refs); err != nil {
		return err
	}
	return validateAssignmentDAG(plan.Relationships)
}

func relationshipInstructionForMember(templateID string, member TeamFormationMember, ordinal int, candidates []templateMember) string {
	matchingRole := make([]templateMember, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.EqualFold(candidate.Role, member.Role) && candidate.Relationship != nil {
			matchingRole = append(matchingRole, candidate)
		}
	}
	if len(matchingRole) == 1 {
		return strings.TrimSpace(*matchingRole[0].Relationship)
	}
	// Templates such as dev-team intentionally have several members with the
	// same broad role. Preserve their ordered, distinct delegation criteria.
	if len(matchingRole) > 1 && ordinal < len(matchingRole) && matchingRole[ordinal].Relationship != nil {
		return strings.TrimSpace(*matchingRole[ordinal].Relationship)
	}
	if ordinal < len(candidates) && candidates[ordinal].Relationship != nil {
		return strings.TrimSpace(*candidates[ordinal].Relationship)
	}
	return fmt.Sprintf("Using the %s template, coordinate scope, handoffs, and review criteria for the %s role.", templateID, member.Role)
}

func relationshipKey(fromRef, toRef, relationType string) string {
	if relationType == "collaborates_with" && fromRef > toRef {
		fromRef, toRef = toRef, fromRef
	}
	return relationType + ":" + fromRef + ":" + toRef
}

func validateResolvedRelationships(relations []TeamFormationRelation, refs map[string]bool) error {
	if len(relations) == 0 || len(relations) > teamFormationMaxMembers*teamFormationMaxMembers {
		return fmt.Errorf("%w: resolved relationships must contain a bounded team topology", ErrInvalidTeamFormationPlan)
	}
	seen := make(map[string]bool, len(relations))
	for _, relation := range relations {
		if !refs[relation.FromRef] || !refs[relation.ToRef] || relation.FromRef == relation.ToRef {
			return fmt.Errorf("%w: resolved relationship references are invalid", ErrInvalidTeamFormationPlan)
		}
		if relation.Type != "assigns_to" && relation.Type != "collaborates_with" {
			return fmt.Errorf("%w: resolved relationship type is invalid", ErrInvalidTeamFormationPlan)
		}
		if relation.Weight <= 0 || relation.Weight > 10 || len([]rune(relation.Instruction)) > 2000 {
			return fmt.Errorf("%w: resolved relationship weight or instruction is invalid", ErrInvalidTeamFormationPlan)
		}
		key := relationshipKey(relation.FromRef, relation.ToRef, relation.Type)
		if seen[key] {
			return fmt.Errorf("%w: duplicate resolved relationship %s", ErrInvalidTeamFormationPlan, key)
		}
		seen[key] = true
	}
	return nil
}

func validateRelationshipConnectivity(relations []TeamFormationRelation, leaderRef string, refs map[string]bool) error {
	adjacency := make(map[string][]string, len(refs))
	for _, relation := range relations {
		adjacency[relation.FromRef] = append(adjacency[relation.FromRef], relation.ToRef)
		adjacency[relation.ToRef] = append(adjacency[relation.ToRef], relation.FromRef)
	}
	visited := map[string]bool{leaderRef: true}
	queue := []string{leaderRef}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		for _, next := range adjacency[node] {
			if !visited[next] {
				visited[next] = true
				queue = append(queue, next)
			}
		}
	}
	if len(visited) != len(refs) {
		return fmt.Errorf("%w: relationship overrides must keep every member connected to the leader", ErrInvalidTeamFormationPlan)
	}
	return nil
}

func validateAssignmentDAG(relations []TeamFormationRelation) error {
	edges := map[string][]string{}
	for _, relation := range relations {
		if relation.Type == "assigns_to" {
			edges[relation.FromRef] = append(edges[relation.FromRef], relation.ToRef)
		}
	}
	state := map[string]uint8{}
	var visit func(string) bool
	visit = func(node string) bool {
		if state[node] == 1 {
			return false
		}
		if state[node] == 2 {
			return true
		}
		state[node] = 1
		for _, next := range edges[node] {
			if !visit(next) {
				return false
			}
		}
		state[node] = 2
		return true
	}
	for node := range edges {
		if !visit(node) {
			return fmt.Errorf("%w: assigns_to relationships must be acyclic", ErrInvalidTeamFormationPlan)
		}
	}
	return nil
}

func buildTeamKickoffMessage(plan TeamFormationPlan, sourceText, channelName string, members []TeamFormationResultMember) string {
	var b strings.Builder
	fmt.Fprintf(&b, "✨ Lucy assembled this team for: %s\n\n", plan.IntentSummary)
	fmt.Fprintf(&b, "Source request: %s\n\n", truncateRunes(strings.TrimSpace(sourceText), 2000))
	b.WriteString("Team:\n")
	for _, member := range members {
		fmt.Fprintf(&b, "- @%s — %s\n", member.Name, member.Role)
	}
	fmt.Fprintf(&b, "\nRelationship template: %s", plan.RelationshipTemplate)
	if len(plan.RelationshipOverrides) > 0 {
		fmt.Fprintf(&b, " (%d Lucy adjustment(s))", len(plan.RelationshipOverrides))
	}
	fmt.Fprintf(&b, "\n\nTeam provisioned in #%s. No tasks were created automatically; create tasks after scope and ownership are agreed.", channelName)
	return b.String()
}

func slugifyTeamName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
			lastDash = false
		case !lastDash && b.Len() > 0:
			b.WriteRune('-')
			lastDash = true
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		result = "team"
	}
	return truncateRunes(result, 100)
}

func sanitizeAgentHandle(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' || r == '.':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || unicode.IsSpace(r):
			if !lastDash && b.Len() > 0 {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	return truncateRunes(strings.Trim(b.String(), "-"), 100)
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}
