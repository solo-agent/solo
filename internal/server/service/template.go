package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TemplateService struct {
	pool  *pgxpool.Pool
	mdGen *RelationshipsMDGenerator
}

var (
	ErrTemplateRuntimeUnavailable = errors.New("configure Lucy's model provider before creating a team from a template")
	ErrTemplateTargetUnavailable  = errors.New("target channel not found")
	ErrChannelTeamNotEmpty        = errors.New("a template can only be applied to an empty channel")
)

func NewTemplateService(pool *pgxpool.Pool, mdGen ...*RelationshipsMDGenerator) *TemplateService {
	s := &TemplateService{pool: pool}
	if len(mdGen) > 0 {
		s.mdGen = mdGen[0]
	}
	return s
}

type AgentTemplate struct {
	ID            string                         `json:"id"`
	Name          string                         `json:"name"`
	Description   string                         `json:"description"`
	Category      string                         `json:"category"`
	Icon          string                         `json:"icon"`
	MemberCount   int                            `json:"member_count"`
	Roles         []string                       `json:"roles"`
	AvatarURLs    []string                       `json:"avatar_urls"`
	Members       []TemplateMember               `json:"members,omitempty"`
	Relationships []TemplateRelationship         `json:"relationships,omitempty"`
	Translations  map[string]TemplateTranslation `json:"-"`
}

type TemplateTranslation struct {
	Name        string                               `json:"name"`
	Description string                               `json:"description"`
	Members     map[string]TemplateMemberTranslation `json:"members"`
}

type TemplateMemberTranslation struct {
	Role        string `json:"role"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type TemplateMember struct {
	Ref          string `json:"ref"`
	Role         string `json:"role"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
	AvatarURL    string `json:"avatar_url"`
}

type TemplateRelationship struct {
	FromRef     string  `json:"from_ref"`
	ToRef       string  `json:"to_ref"`
	Type        string  `json:"type"`
	Weight      float64 `json:"weight"`
	Instruction string  `json:"instruction,omitempty"`
}

type TemplateProvisionRequest struct {
	OwnerID         string
	TemplateID      string
	ChannelName     string
	Description     string
	ModelProvider   string
	ModelName       string
	RuntimeID       string
	AllowNameSuffix bool
	TargetChannelID string
	Locale          string
}

type ProvisionedTemplateMember struct {
	Ref       string `json:"ref"`
	Role      string `json:"role"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

type TemplateProvisionResult struct {
	ChannelID         string                      `json:"channel_id"`
	ChannelName       string                      `json:"channel_name"`
	TemplateID        string                      `json:"template_id"`
	Members           []ProvisionedTemplateMember `json:"members"`
	RelationshipIDs   []string                    `json:"relationship_ids"`
	RelationshipCount int                         `json:"relationship_count"`
	CreatedAt         time.Time                   `json:"created_at"`
}

func (s *TemplateService) List(ctx context.Context, locales ...string) ([]AgentTemplate, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, description, category, icon, members, translations
		  FROM agent_templates
		 WHERE is_official = true
		 ORDER BY category ASC, name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	templates := []AgentTemplate{}
	for rows.Next() {
		var tmpl AgentTemplate
		var rawMembers, rawTranslations []byte
		if err := rows.Scan(
			&tmpl.ID, &tmpl.Name, &tmpl.Description, &tmpl.Category, &tmpl.Icon,
			&rawMembers, &rawTranslations,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(rawMembers, &tmpl.Members); err != nil {
			return nil, fmt.Errorf("parse template %s members: %w", tmpl.ID, err)
		}
		if err := json.Unmarshal(rawTranslations, &tmpl.Translations); err != nil {
			return nil, fmt.Errorf("parse template %s translations: %w", tmpl.ID, err)
		}
		setTemplateAvatarURLs(&tmpl)
		localizeTemplate(&tmpl, firstLocale(locales))
		tmpl.MemberCount = len(tmpl.Members)
		tmpl.Roles = templateRoles(tmpl.Members)
		tmpl.Members = nil
		templates = append(templates, tmpl)
	}
	return templates, rows.Err()
}

func (s *TemplateService) Get(ctx context.Context, templateID string, locales ...string) (*AgentTemplate, error) {
	return loadTemplate(ctx, s.pool, templateID, locales...)
}

func loadTemplate(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, templateID string, locales ...string) (*AgentTemplate, error) {
	var tmpl AgentTemplate
	var rawMembers, rawRelationships, rawTranslations []byte
	err := q.QueryRow(ctx, `
		SELECT id, name, description, category, icon, members, relationships, translations
		  FROM agent_templates
		 WHERE id = $1 AND is_official = true
	`, templateID).Scan(
		&tmpl.ID, &tmpl.Name, &tmpl.Description, &tmpl.Category, &tmpl.Icon,
		&rawMembers, &rawRelationships, &rawTranslations,
	)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(rawMembers, &tmpl.Members); err != nil {
		return nil, fmt.Errorf("parse template %s members: %w", templateID, err)
	}
	if err := json.Unmarshal(rawRelationships, &tmpl.Relationships); err != nil {
		return nil, fmt.Errorf("parse template %s relationships: %w", templateID, err)
	}
	if err := json.Unmarshal(rawTranslations, &tmpl.Translations); err != nil {
		return nil, fmt.Errorf("parse template %s translations: %w", templateID, err)
	}
	tmpl.MemberCount = len(tmpl.Members)
	if err := validateTemplate(&tmpl); err != nil {
		return nil, err
	}
	setTemplateAvatarURLs(&tmpl)
	localizeTemplate(&tmpl, firstLocale(locales))
	tmpl.Roles = templateRoles(tmpl.Members)
	return &tmpl, nil
}

func firstLocale(locales []string) string {
	if len(locales) == 0 {
		return ""
	}
	return locales[0]
}

func localizeTemplate(tmpl *AgentTemplate, locale string) {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "en") {
		return
	}
	translation, ok := tmpl.Translations["en"]
	if !ok {
		return
	}
	if translation.Name != "" {
		tmpl.Name = translation.Name
	}
	if translation.Description != "" {
		tmpl.Description = translation.Description
	}

	incoming := make(map[string]int, len(tmpl.Members))
	outgoing := make(map[string]int, len(tmpl.Members))
	for _, relationship := range tmpl.Relationships {
		if relationship.Type == RelAssignsTo {
			outgoing[relationship.FromRef]++
			incoming[relationship.ToRef]++
		}
	}
	for i := range tmpl.Members {
		member := &tmpl.Members[i]
		memberTranslation, exists := translation.Members[member.Ref]
		if !exists {
			continue
		}
		if memberTranslation.Role != "" {
			member.Role = memberTranslation.Role
		}
		if memberTranslation.Name != "" {
			member.Name = memberTranslation.Name
		}
		if memberTranslation.Description != "" {
			member.Description = memberTranslation.Description
		}
		responsibility := "Own the specialist contribution that fits your role and report a concrete, evidence-backed result to the delegating Agent."
		if incoming[member.Ref] == 0 && outgoing[member.Ref] > 0 {
			responsibility = "Lead the team, turn the Channel goal into clear assignments, and integrate the returned work into one decision-ready result."
		} else if outgoing[member.Ref] > 0 {
			responsibility = "Coordinate your part of the work, delegate specialist follow-ups when needed, and consolidate the returned evidence."
		}
		member.Instructions = fmt.Sprintf(
			"You are %s. %s\n\nYour responsibility in the %q team: %s\n\nTeam goal: %s\n\nUse only the current Channel's goal and context. Follow the team relationships for delegation, collaboration, and reporting.",
			member.Name, member.Description, tmpl.Name, responsibility, tmpl.Description,
		)
	}

	names := make(map[string]string, len(tmpl.Members))
	for _, member := range tmpl.Members {
		names[member.Ref] = member.Name
	}
	for i := range tmpl.Relationships {
		relationship := &tmpl.Relationships[i]
		from, to := names[relationship.FromRef], names[relationship.ToRef]
		if relationship.Type == RelAssignsTo {
			relationship.Instruction = fmt.Sprintf(
				"Delegation contract: %s delegates to %s when the team goal needs %s's specialist contribution. %s reports back with the result, evidence, open risks, and recommended next step.",
				from, to, to, to,
			)
		} else {
			relationship.Instruction = fmt.Sprintf(
				"Collaboration contract: %s and %s work together when their specialties overlap. Keep scope, evidence, trade-offs, and unresolved questions synchronized.",
				from, to,
			)
		}
	}
}

func setTemplateAvatarURLs(tmpl *AgentTemplate) {
	tmpl.AvatarURLs = make([]string, len(tmpl.Members))
	for i := range tmpl.Members {
		avatarURL := "dicebear:pixel-art:template-" + tmpl.ID + "-" + tmpl.Members[i].Ref
		tmpl.Members[i].AvatarURL = avatarURL
		tmpl.AvatarURLs[i] = avatarURL
	}
}

func templateRoles(members []TemplateMember) []string {
	seen := make(map[string]bool, len(members))
	roles := make([]string, 0, len(members))
	for _, member := range members {
		role := strings.TrimSpace(member.Role)
		if role != "" && !seen[role] {
			seen[role] = true
			roles = append(roles, role)
		}
	}
	return roles
}

func validateTemplate(tmpl *AgentTemplate) error {
	if len(tmpl.Members) == 0 {
		return errors.New("template has no members")
	}
	refs := make(map[string]bool, len(tmpl.Members))
	assignsTo := make(map[string][]string, len(tmpl.Members))
	for i := range tmpl.Members {
		member := &tmpl.Members[i]
		member.Ref = strings.ToLower(strings.TrimSpace(member.Ref))
		member.Role = strings.TrimSpace(member.Role)
		member.Name = sanitizeAgentHandle(member.Name)
		member.Description = strings.TrimSpace(member.Description)
		member.Instructions = strings.TrimSpace(member.Instructions)
		if member.Ref == "" || refs[member.Ref] || member.Role == "" || member.Name == "" || member.Instructions == "" {
			return fmt.Errorf("template %s has an invalid member", tmpl.ID)
		}
		refs[member.Ref] = true
	}
	for i := range tmpl.Relationships {
		rel := &tmpl.Relationships[i]
		rel.FromRef = strings.ToLower(strings.TrimSpace(rel.FromRef))
		rel.ToRef = strings.ToLower(strings.TrimSpace(rel.ToRef))
		rel.Type = strings.ToLower(strings.TrimSpace(rel.Type))
		rel.Instruction = strings.TrimSpace(rel.Instruction)
		if !refs[rel.FromRef] || !refs[rel.ToRef] || rel.FromRef == rel.ToRef {
			return fmt.Errorf("template %s has an invalid relationship reference", tmpl.ID)
		}
		if rel.Type != RelAssignsTo && rel.Type != RelCollaboratesWith {
			return fmt.Errorf("template %s has an invalid relationship type", tmpl.ID)
		}
		if rel.Weight == 0 {
			rel.Weight = 1
		}
		if rel.Weight < 0 || rel.Weight > 10 {
			return fmt.Errorf("template %s has an invalid relationship weight", tmpl.ID)
		}
		if rel.Type == RelAssignsTo {
			assignsTo[rel.FromRef] = append(assignsTo[rel.FromRef], rel.ToRef)
		}
	}
	visiting := make(map[string]bool, len(refs))
	visited := make(map[string]bool, len(refs))
	var hasCycle func(string) bool
	hasCycle = func(ref string) bool {
		if visiting[ref] {
			return true
		}
		if visited[ref] {
			return false
		}
		visiting[ref] = true
		for _, next := range assignsTo[ref] {
			if hasCycle(next) {
				return true
			}
		}
		visiting[ref] = false
		visited[ref] = true
		return false
	}
	for ref := range refs {
		if hasCycle(ref) {
			return fmt.Errorf("template %s has a cyclic assigns_to relationship", tmpl.ID)
		}
	}
	return nil
}

func (s *TemplateService) CreateChannel(ctx context.Context, req TemplateProvisionRequest) (*TemplateProvisionResult, error) {
	return s.provision(ctx, req)
}

func (s *TemplateService) ApplyToChannel(ctx context.Context, req TemplateProvisionRequest) (*TemplateProvisionResult, error) {
	if strings.TrimSpace(req.TargetChannelID) == "" {
		return nil, ErrTemplateTargetUnavailable
	}
	return s.provision(ctx, req)
}

func (s *TemplateService) provision(ctx context.Context, req TemplateProvisionRequest) (*TemplateProvisionResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('solo-template-channel'))`); err != nil {
		return nil, err
	}
	result, err := s.provisionChannelTx(ctx, tx, req)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	s.generateRelationshipDocs(result.Members)
	return result, nil
}

// provisionChannelTx is the single provisioning path used by manual template
// creation and Lucy team formation. The caller owns the transaction.
func (s *TemplateService) provisionChannelTx(ctx context.Context, tx pgx.Tx, req TemplateProvisionRequest) (*TemplateProvisionResult, error) {
	tmpl, err := loadTemplate(ctx, tx, strings.TrimSpace(req.TemplateID), req.Locale)
	if err != nil {
		return nil, fmt.Errorf("load template: %w", err)
	}

	targetChannelID := strings.TrimSpace(req.TargetChannelID)
	channelID := targetChannelID
	channelName := ""
	description := strings.TrimSpace(req.Description)
	var createdAt time.Time
	if targetChannelID != "" {
		var hasTeam bool
		err = tx.QueryRow(ctx, `
			SELECT c.name,
			       c.created_at,
			       c.source_template_id IS NOT NULL
			       OR EXISTS (
			           SELECT 1
			             FROM agents a
			            WHERE a.home_channel_id = c.id
			              AND a.kind = 'agent'
			       )
			       OR EXISTS (
			           SELECT 1
			             FROM channel_members agent_member
			            WHERE agent_member.channel_id = c.id
			              AND agent_member.member_type = 'agent'
			       )
			  FROM channels c
			  JOIN channel_members owner_member
			    ON owner_member.channel_id = c.id
			   AND owner_member.member_type = 'user'
			   AND owner_member.member_id = $2
			   AND owner_member.role IN ('owner', 'admin')
			 WHERE c.id = $1
			   AND c.type = 'channel'
			   AND c.is_archived = false
			 FOR UPDATE OF c
		`, targetChannelID, req.OwnerID).Scan(&channelName, &createdAt, &hasTeam)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTemplateTargetUnavailable
		}
		if err != nil {
			return nil, err
		}
		if hasTeam {
			return nil, ErrChannelTeamNotEmpty
		}
	} else {
		channelName = slugifyChannelName(req.ChannelName)
		if channelName == "" {
			return nil, errors.New("channel name is required")
		}
		if req.AllowNameSuffix {
			channelName, err = uniqueChannelName(ctx, tx, channelName)
			if err != nil {
				return nil, err
			}
		}
		if description == "" {
			description = tmpl.Description
		}
	}

	if req.ModelProvider == "" {
		_ = tx.QueryRow(ctx, `
			SELECT model_provider, model_name, COALESCE(runtime_id, '')
			  FROM agents
			 WHERE owner_id = $1 AND kind = 'lucy' AND is_active = true
			 ORDER BY created_at ASC
			 LIMIT 1
		`, req.OwnerID).Scan(&req.ModelProvider, &req.ModelName, &req.RuntimeID)
	}
	req.ModelProvider = strings.TrimSpace(req.ModelProvider)
	if req.ModelProvider == "" {
		return nil, ErrTemplateRuntimeUnavailable
	}
	if req.RuntimeID == "" {
		_ = tx.QueryRow(ctx, `
			SELECT c.id
			  FROM computers c
			 WHERE c.status = 'online'
			   AND (c.owner_id = $1 OR EXISTS (
			       SELECT 1 FROM computer_members cm
			        WHERE cm.computer_id = c.id AND cm.user_id = $1
			   ))
			 ORDER BY c.created_at ASC
			 LIMIT 1
		`, req.OwnerID).Scan(&req.RuntimeID)
	}

	if targetChannelID == "" {
		channelID = uuid.New().String()
		err = tx.QueryRow(ctx, `
			INSERT INTO channels (
				id, name, description, type, created_by, source_template_id
			) VALUES ($1, $2, $3, 'channel', $4, $5)
			RETURNING created_at
		`, channelID, channelName, description, req.OwnerID, tmpl.ID).Scan(&createdAt)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO channel_members (channel_id, member_type, member_id, role)
			VALUES ($1, 'user', $2, 'owner')
		`, channelID, req.OwnerID); err != nil {
			return nil, err
		}
	} else if _, err := tx.Exec(ctx, `
		UPDATE channels
		   SET source_template_id = $2,
		       updated_at = now()
		 WHERE id = $1
	`, channelID, tmpl.ID); err != nil {
		return nil, err
	}

	runtimeValue := any(nil)
	if req.RuntimeID != "" {
		runtimeValue = req.RuntimeID
	}
	membersByRef := make(map[string]ProvisionedTemplateMember, len(tmpl.Members))
	resultMembers := make([]ProvisionedTemplateMember, 0, len(tmpl.Members))
	for _, member := range tmpl.Members {
		agentID := uuid.New().String()
		name, nameErr := uniqueAgentNameInChannel(ctx, tx, channelID, member.Name)
		if nameErr != nil {
			return nil, nameErr
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO agents (
				id, name, description, owner_id, model_provider, model_name,
				system_prompt, runtime_id, custom_env, custom_args,
				avatar_url, home_channel_id, kind
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8,
				'{}'::jsonb, '[]'::jsonb, $9, $10, 'agent'
			)
		`, agentID, name, member.Description, req.OwnerID, req.ModelProvider,
			req.ModelName, member.Instructions, runtimeValue, member.AvatarURL, channelID); err != nil {
			return nil, fmt.Errorf("create agent %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO channel_members (channel_id, member_type, member_id, role)
			VALUES ($1, 'agent', $2, 'member')
		`, channelID, agentID); err != nil {
			return nil, fmt.Errorf("add agent %s to channel: %w", name, err)
		}
		resultMember := ProvisionedTemplateMember{
			Ref: member.Ref, Role: member.Role, ID: agentID, Name: name, AvatarURL: member.AvatarURL,
		}
		membersByRef[member.Ref] = resultMember
		resultMembers = append(resultMembers, resultMember)
	}

	relationshipIDs := make([]string, 0, len(tmpl.Relationships))
	for _, relationship := range tmpl.Relationships {
		from := membersByRef[relationship.FromRef]
		to := membersByRef[relationship.ToRef]
		relationshipID := uuid.New().String()
		if _, err := tx.Exec(ctx, `
			INSERT INTO agent_relationships (
				id, from_agent_id, to_agent_id, rel_type, weight, instruction
			) VALUES ($1, $2, $3, $4, $5, $6)
		`, relationshipID, from.ID, to.ID, relationship.Type,
			relationship.Weight, relationship.Instruction); err != nil {
			return nil, fmt.Errorf("create relationship %s -> %s: %w", from.Name, to.Name, err)
		}
		relationshipIDs = append(relationshipIDs, relationshipID)
	}

	return &TemplateProvisionResult{
		ChannelID:         channelID,
		ChannelName:       channelName,
		TemplateID:        tmpl.ID,
		Members:           resultMembers,
		RelationshipIDs:   relationshipIDs,
		RelationshipCount: len(relationshipIDs),
		CreatedAt:         createdAt,
	}, nil
}

func (s *TemplateService) generateRelationshipDocs(members []ProvisionedTemplateMember) {
	if s.mdGen == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, member := range members {
		_ = s.mdGen.GenerateForAgent(ctx, member.ID)
	}
}

func uniqueChannelName(ctx context.Context, tx pgx.Tx, requested string) (string, error) {
	base := slugifyChannelName(requested)
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
			)
		`, candidate).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no available channel name for %q", requested)
}

func uniqueAgentNameInChannel(ctx context.Context, tx pgx.Tx, channelID, requested string) (string, error) {
	base := sanitizeAgentHandle(requested)
	for suffix := 1; suffix <= 999; suffix++ {
		candidate := base
		if suffix > 1 {
			candidate = truncateRunes(base, 92) + fmt.Sprintf("-%d", suffix)
		}
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM agents
				 WHERE home_channel_id = $1 AND lower(name) = lower($2) AND is_active = true
			)
		`, channelID, candidate).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no available agent name for %q", requested)
}

func slugifyChannelName(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	var b strings.Builder
	lastDash := false
	for _, r := range input {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return truncateRunes(strings.Trim(b.String(), "-"), 100)
}

func sanitizeAgentHandle(input string) string {
	input = strings.TrimSpace(input)
	var b strings.Builder
	for _, r := range input {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "-_")
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}
