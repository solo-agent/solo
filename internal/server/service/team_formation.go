package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/realtime"
)

const (
	teamFormationDocRetries = 3
	teamFormationDocTimeout = 15 * time.Second
)

var (
	ErrTeamFormationForbidden      = errors.New("only Lucy can form a team from her Lucy channel")
	ErrTeamFormationSourceNotFound = errors.New("source user message not found")
	ErrTeamFormationInProgress     = errors.New("team formation is already in progress")
	ErrInvalidTeamFormationPlan    = errors.New("invalid team formation plan")
	errTeamFormationLeaseLost      = errors.New("team formation provisioning lease was lost")

	teamTemplatePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,49}$`)
	messageIDPattern    = regexp.MustCompile(`^[0-9a-fA-F-]{4,36}$`)
)

type relationshipDocumentGenerator interface {
	GenerateForAgent(context.Context, string) error
}

// TeamFormationService authorizes Lucy and turns one official template into a
// fresh Channel-scoped Agent team. The source message is the idempotency key.
type TeamFormationService struct {
	pool        *pgxpool.Pool
	mdGen       relationshipDocumentGenerator
	broadcaster realtime.Broadcaster
	templates   *TemplateService
}

func NewTeamFormationService(
	pool *pgxpool.Pool,
	mdGen relationshipDocumentGenerator,
	broadcaster realtime.Broadcaster,
	templates ...*TemplateService,
) *TeamFormationService {
	templateSvc := NewTemplateService(pool)
	if len(templates) > 0 && templates[0] != nil {
		templateSvc = templates[0]
	}
	return &TeamFormationService{
		pool: pool, mdGen: mdGen, broadcaster: broadcaster, templates: templateSvc,
	}
}

type TeamFormationRequest struct {
	SourceChannelID string            `json:"source_channel_id"`
	SourceMessageID string            `json:"source_message_id"`
	Plan            TeamFormationPlan `json:"plan"`
}

type TeamFormationPlan struct {
	IntentSummary string               `json:"intent_summary"`
	Channel       TeamFormationChannel `json:"channel"`
	TemplateID    string               `json:"template_id"`
}

type TeamFormationChannel struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type TeamFormationResult struct {
	FormationID           string                      `json:"formation_id"`
	SourceChannelID       string                      `json:"source_channel_id"`
	SourceMessageID       string                      `json:"source_message_id"`
	ChannelID             string                      `json:"channel_id"`
	ChannelName           string                      `json:"channel_name"`
	DashboardURL          string                      `json:"dashboard_url"`
	TemplateID            string                      `json:"template_id"`
	Members               []TeamFormationResultMember `json:"members"`
	RelationshipCount     int                         `json:"relationship_count"`
	RelationshipDocsReady bool                        `json:"relationship_docs_ready"`
	Warnings              []string                    `json:"warnings,omitempty"`
	Replayed              bool                        `json:"replayed"`
	CreatedAt             time.Time                   `json:"created_at"`
}

type TeamFormationResultMember struct {
	Ref       string `json:"ref"`
	Role      string `json:"role"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
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
		failureCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = s.pool.Exec(failureCtx, `
			UPDATE team_formations
			   SET status = 'failed', error = $2, updated_at = now()
			 WHERE id = $1 AND status = 'provisioning'
			   AND target_channel_id IS NULL AND updated_at = $3
		`, formationID, truncateRunes(err.Error(), 4000), leaseUpdatedAt)
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
			"template_id":             result.TemplateID,
			"relationship_count":      result.RelationshipCount,
			"relationship_docs_ready": result.RelationshipDocsReady,
			"warnings":                result.Warnings,
			"dashboard_url":           result.DashboardURL,
			"created_at":              result.CreatedAt.Format(time.RFC3339),
		}))
	}
	return result, nil
}

func normalizeAndValidateTeamPlan(plan *TeamFormationPlan) error {
	plan.IntentSummary = strings.TrimSpace(plan.IntentSummary)
	plan.Channel.Name = strings.TrimSpace(plan.Channel.Name)
	plan.Channel.Description = strings.TrimSpace(plan.Channel.Description)
	plan.TemplateID = strings.ToLower(strings.TrimSpace(plan.TemplateID))
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
	if !teamTemplatePattern.MatchString(plan.TemplateID) {
		return fmt.Errorf("%w: template_id is required and must be a valid template ID", ErrInvalidTeamFormationPlan)
	}
	return nil
}

func (s *TeamFormationService) authorizeCaller(ctx context.Context, callerID, channelID, messagePrefix string) (*teamFormationCaller, error) {
	caller := teamFormationCaller{ChannelID: channelID}
	err := s.pool.QueryRow(ctx, `
		SELECT a.id, a.owner_id, a.model_provider, a.model_name,
		       COALESCE(a.runtime_id, ''), c.name
		  FROM agents a
		  JOIN channels c
		    ON c.id = a.home_channel_id
		   AND c.id = $2
		  JOIN channel_members ucm
		    ON ucm.channel_id = c.id
		   AND ucm.member_type = 'user'
		   AND ucm.member_id = a.owner_id
		 WHERE a.id = $1
		   AND a.kind = 'lucy'
		   AND a.is_active = true
		   AND c.type = 'lucy'
		   AND c.is_archived = false
		   AND c.created_by = a.owner_id
	`, callerID, channelID).Scan(
		&caller.AgentID, &caller.OwnerID, &caller.Provider,
		&caller.ModelName, &caller.RuntimeID, &caller.ChannelName,
	)
	if err != nil {
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
		 LIMIT 2
	`, channelID, caller.OwnerID, messagePrefix)
	if err != nil {
		return nil, fmt.Errorf("resolve source message: %w", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		if err := rows.Scan(&caller.SourceID, &caller.SourceText); err != nil {
			return nil, err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return nil, err
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
		RETURNING id, updated_at
	`, caller.OwnerID, caller.AgentID, caller.ChannelID, caller.SourceID, planJSON).Scan(
		&formationID, &leaseUpdatedAt,
	)
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
		 WHERE source_message_id = $1
	`, caller.SourceID).Scan(&formationID, &status, &resultJSON, &updatedAt)
	if err != nil {
		return "", time.Time{}, nil, fmt.Errorf("load team formation: %w", err)
	}
	if status == "completed" && len(resultJSON) > 0 {
		var result TeamFormationResult
		if err := json.Unmarshal(resultJSON, &result); err != nil {
			return "", time.Time{}, nil, err
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
			RETURNING updated_at
		`, formationID, planJSON, status, updatedAt).Scan(&leaseUpdatedAt)
		if updateErr == nil {
			return formationID, leaseUpdatedAt, nil, nil
		}
		if !errors.Is(updateErr, pgx.ErrNoRows) {
			return "", time.Time{}, nil, updateErr
		}
	}
	return "", time.Time{}, nil, ErrTeamFormationInProgress
}

func (s *TeamFormationService) provision(
	ctx context.Context,
	formationID string,
	leaseUpdatedAt time.Time,
	caller *teamFormationCaller,
	plan TeamFormationPlan,
) (*TeamFormationResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('solo-template-channel'))`); err != nil {
		return nil, err
	}
	var currentStatus string
	var currentUpdatedAt time.Time
	if err := tx.QueryRow(ctx, `
		SELECT status, updated_at
		  FROM team_formations
		 WHERE id = $1
		 FOR UPDATE
	`, formationID).Scan(&currentStatus, &currentUpdatedAt); err != nil {
		return nil, err
	}
	if currentStatus != "provisioning" || !currentUpdatedAt.Equal(leaseUpdatedAt) {
		return nil, errTeamFormationLeaseLost
	}

	provisioned, err := s.templates.provisionChannelTx(ctx, tx, TemplateProvisionRequest{
		OwnerID:         caller.OwnerID,
		TemplateID:      plan.TemplateID,
		ChannelName:     plan.Channel.Name,
		Description:     plan.Channel.Description,
		ModelProvider:   caller.Provider,
		ModelName:       caller.ModelName,
		RuntimeID:       caller.RuntimeID,
		AllowNameSuffix: true,
	})
	if err != nil {
		return nil, err
	}

	members := make([]TeamFormationResultMember, 0, len(provisioned.Members))
	for _, member := range provisioned.Members {
		members = append(members, TeamFormationResultMember(member))
	}
	result := &TeamFormationResult{
		FormationID:       formationID,
		SourceChannelID:   caller.ChannelID,
		SourceMessageID:   caller.SourceID,
		ChannelID:         provisioned.ChannelID,
		ChannelName:       provisioned.ChannelName,
		DashboardURL:      "/dashboard?channel=" + provisioned.ChannelID,
		TemplateID:        provisioned.TemplateID,
		Members:           members,
		RelationshipCount: provisioned.RelationshipCount,
		CreatedAt:         provisioned.CreatedAt,
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	cardMetadata, err := json.Marshal(map[string]any{
		"card_type":     "channel_created",
		"formation_id":  result.FormationID,
		"template_id":   result.TemplateID,
		"channel_id":    result.ChannelID,
		"channel_name":  result.ChannelName,
		"member_count":  len(result.Members),
		"members":       result.Members,
		"status":        "completed",
		"dashboard_url": result.DashboardURL,
	})
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO messages (
			id, channel_id, sender_type, sender_id, content, content_type,
			metadata, created_at, updated_at
		) VALUES (
			$1, $2, 'system', $3, $4, 'channel_created',
			$5, now(), now()
		)
	`, uuid.New().String(), caller.ChannelID, uuid.Nil.String(),
		fmt.Sprintf("Created #%s from the %s template.", result.ChannelName, result.TemplateID),
		cardMetadata); err != nil {
		return nil, fmt.Errorf("persist Lucy result card: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO messages (
			id, channel_id, sender_type, sender_id, content, content_type,
			metadata, created_at, updated_at
		) VALUES (
			$1, $2, 'system', $3, $4, 'template_applied',
			$5, now(), now()
		)
	`, uuid.New().String(), result.ChannelID, uuid.Nil.String(),
		fmt.Sprintf("Team created from the %s template.", result.TemplateID),
		cardMetadata); err != nil {
		return nil, fmt.Errorf("persist template kickoff: %w", err)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE team_formations
		   SET target_channel_id = $2, status = 'completed', result = $3,
		       error = '', updated_at = now()
		 WHERE id = $1 AND status = 'provisioning' AND updated_at = $4
	`, formationID, result.ChannelID, resultJSON, leaseUpdatedAt)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, errTeamFormationLeaseLost
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

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
				select {
				case <-ctx.Done():
					lastErr = ctx.Err()
				case <-time.After(time.Duration(attempt) * 100 * time.Millisecond):
				}
			}
		}
		if lastErr != nil {
			failedNames = append(failedNames, "@"+member.Name)
			slog.Error("team formation relationship document generation failed",
				"formation_id", result.FormationID,
				"agent_id", member.ID,
				"error", lastErr,
			)
		}
	}
	result.RelationshipDocsReady = len(failedNames) == 0
	if len(failedNames) > 0 {
		result.Warnings = []string{
			"Relationship documents are not ready for " + strings.Join(failedNames, ", ") + ". Retry the same request to repair them.",
		}
	}
}

func (s *TeamFormationService) persistFinalizedResult(result *TeamFormationResult) {
	stored := *result
	stored.Replayed = false
	resultJSON, err := json.Marshal(&stored)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.pool.Exec(ctx, `
		UPDATE team_formations
		   SET result = $2, updated_at = now()
		 WHERE id = $1 AND status = 'completed'
	`, result.FormationID, resultJSON); err != nil {
		slog.Error("persist finalized team formation result",
			"formation_id", result.FormationID, "error", err)
	}
}
