package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestValidateTemplateRequiresExplicitRelationshipReferences(t *testing.T) {
	tmpl := AgentTemplate{
		ID: "test-team",
		Members: []TemplateMember{
			{Ref: "lead", Role: "leader", Name: "Lead", Instructions: "Coordinate."},
			{Ref: "worker", Role: "worker", Name: "Worker", Instructions: "Execute."},
		},
		Relationships: []TemplateRelationship{
			{FromRef: "lead", ToRef: "worker", Type: "assigns_to"},
		},
	}
	if err := validateTemplate(&tmpl); err != nil {
		t.Fatalf("validateTemplate: %v", err)
	}
	if tmpl.Relationships[0].Weight != 1 {
		t.Fatalf("default relationship weight = %v", tmpl.Relationships[0].Weight)
	}
}

func TestValidateTemplateRejectsUnknownRelationshipReference(t *testing.T) {
	tmpl := AgentTemplate{
		ID: "test-team",
		Members: []TemplateMember{
			{Ref: "lead", Role: "leader", Name: "Lead", Instructions: "Coordinate."},
		},
		Relationships: []TemplateRelationship{
			{FromRef: "lead", ToRef: "missing", Type: "assigns_to"},
		},
	}
	if err := validateTemplate(&tmpl); err == nil {
		t.Fatal("expected invalid relationship reference")
	}
}

func TestValidateTemplateRejectsAssignsToCycle(t *testing.T) {
	tmpl := AgentTemplate{
		ID: "cyclic-team",
		Members: []TemplateMember{
			{Ref: "lead", Role: "leader", Name: "Lead", Instructions: "Coordinate."},
			{Ref: "worker", Role: "worker", Name: "Worker", Instructions: "Execute."},
		},
		Relationships: []TemplateRelationship{
			{FromRef: "lead", ToRef: "worker", Type: "assigns_to"},
			{FromRef: "worker", ToRef: "lead", Type: "assigns_to"},
		},
	}
	if err := validateTemplate(&tmpl); err == nil {
		t.Fatal("expected cyclic assigns_to relationship to be rejected")
	}
}

func TestLocalizeTemplateUsesEnglishCatalogAndRelationshipContracts(t *testing.T) {
	tmpl := AgentTemplate{
		Name:        "中文模板",
		Description: "中文描述",
		Members: []TemplateMember{
			{Ref: "lead", Role: "负责人", Name: "负责人", Description: "中文", Instructions: "中文指令"},
			{Ref: "worker", Role: "执行者", Name: "执行者", Description: "中文", Instructions: "中文指令"},
		},
		Relationships: []TemplateRelationship{
			{FromRef: "lead", ToRef: "worker", Type: RelAssignsTo},
		},
		Translations: map[string]TemplateTranslation{
			"en": {
				Name:        "English Team",
				Description: "English goal",
				Members: map[string]TemplateMemberTranslation{
					"lead":   {Role: "Lead", Name: "Lead", Description: "Coordinates the team."},
					"worker": {Role: "Worker", Name: "Worker", Description: "Delivers specialist work."},
				},
			},
		},
	}

	localizeTemplate(&tmpl, "en-US")

	if tmpl.Name != "English Team" || tmpl.Members[1].Name != "Worker" {
		t.Fatalf("template was not localized: %#v", tmpl)
	}
	if !strings.Contains(tmpl.Members[0].Instructions, "Lead the team") {
		t.Fatalf("leader instructions = %q", tmpl.Members[0].Instructions)
	}
	if !strings.Contains(tmpl.Relationships[0].Instruction, "reports back") {
		t.Fatalf("relationship instructions = %q", tmpl.Relationships[0].Instruction)
	}
}

func TestOfficialTemplateCatalogContainsDocumentedAgencyWorkflows(t *testing.T) {
	pool := agentRunTestPool(t)
	svc := NewTemplateService(pool)
	templates, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(templates) != 32 {
		t.Fatalf("official template count = %d, want 32", len(templates))
	}
	collaborationCount := 0
	for _, template := range templates {
		loaded, err := svc.Get(context.Background(), template.ID)
		if err != nil {
			t.Fatalf("Get(%q): %v", template.ID, err)
		}
		if loaded.MemberCount == 0 {
			t.Fatalf("template %q has no members", template.ID)
		}
		if len(template.AvatarURLs) != loaded.MemberCount {
			t.Fatalf("template %q avatar count = %d, want %d", template.ID, len(template.AvatarURLs), loaded.MemberCount)
		}
		for _, member := range loaded.Members {
			if !strings.HasPrefix(member.AvatarURL, "dicebear:pixel-art:template-"+template.ID+"-") {
				t.Fatalf("template %q member %q avatar = %q", template.ID, member.Ref, member.AvatarURL)
			}
		}
		for _, relationship := range loaded.Relationships {
			if relationship.Type == RelCollaboratesWith {
				collaborationCount++
			}
		}
	}
	if collaborationCount == 0 {
		t.Fatal("official templates contain no collaboration relationships")
	}
	englishTemplates, err := svc.List(context.Background(), "en")
	if err != nil {
		t.Fatalf("List(en): %v", err)
	}
	if len(englishTemplates) != 32 || englishTemplates[0].Name == templates[0].Name {
		t.Fatalf("English template catalog was not localized")
	}
}

func TestTemplateProvisionRequiresModelProviderBeforeWriting(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE created_by = $1`, ownerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	_, err := NewTemplateService(pool).CreateChannel(ctx, TemplateProvisionRequest{
		OwnerID:     ownerID,
		TemplateID:  "agency-dev-tech-design-review",
		ChannelName: "runtime-precondition-test",
	})
	if !errors.Is(err, ErrTemplateRuntimeUnavailable) {
		t.Fatalf("CreateChannel error = %v, want %v", err, ErrTemplateRuntimeUnavailable)
	}

	var count int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM channels
		 WHERE created_by = $1 AND name = 'runtime-precondition-test'
	`, ownerID).Scan(&count); err != nil {
		t.Fatalf("count channels: %v", err)
	}
	if count != 0 {
		t.Fatalf("created %d unusable channels, want 0", count)
	}
}

func TestTemplateProvisionAppliesAtomicallyToEmptyChannel(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	channelID, err := NewChannelService(pool).CreateChannel(
		ctx,
		"apply-template-test",
		"",
		"channel",
		ownerID,
	)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM agent_relationships
			 WHERE from_agent_id IN (SELECT id FROM agents WHERE home_channel_id = $1)
			    OR to_agent_id IN (SELECT id FROM agents WHERE home_channel_id = $1)
		`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channel_members WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE home_channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	svc := NewTemplateService(pool)
	result, err := svc.ApplyToChannel(ctx, TemplateProvisionRequest{
		OwnerID:         ownerID,
		TemplateID:      "agency-dev-tech-design-review",
		TargetChannelID: channelID,
		ModelProvider:   "codex",
	})
	if err != nil {
		t.Fatalf("ApplyToChannel: %v", err)
	}
	if result.ChannelID != channelID || len(result.Members) != 4 || result.RelationshipCount != 4 {
		t.Fatalf("result = %#v", result)
	}
	template, err := svc.Get(ctx, "agency-dev-tech-design-review")
	if err != nil {
		t.Fatalf("Get template: %v", err)
	}
	avatarByRef := make(map[string]string, len(template.Members))
	for _, member := range template.Members {
		avatarByRef[member.Ref] = member.AvatarURL
	}
	for _, member := range result.Members {
		var storedAvatar string
		if err := pool.QueryRow(ctx,
			`SELECT COALESCE(avatar_url, '') FROM agents WHERE id = $1`,
			member.ID,
		).Scan(&storedAvatar); err != nil {
			t.Fatalf("load agent avatar: %v", err)
		}
		if storedAvatar != avatarByRef[member.Ref] || member.AvatarURL != storedAvatar {
			t.Fatalf("member %q avatars: template=%q result=%q stored=%q",
				member.Ref, avatarByRef[member.Ref], member.AvatarURL, storedAvatar)
		}
	}

	var sourceTemplateID string
	var agents, memberships, relationships int
	err = pool.QueryRow(ctx, `
		SELECT COALESCE(c.source_template_id, ''),
		       (SELECT COUNT(*) FROM agents a WHERE a.home_channel_id = c.id),
		       (SELECT COUNT(*) FROM channel_members cm WHERE cm.channel_id = c.id AND cm.member_type = 'agent'),
		       (SELECT COUNT(*)
		          FROM agent_relationships ar
		          JOIN agents a ON a.id = ar.from_agent_id
		         WHERE a.home_channel_id = c.id)
		  FROM channels c
		 WHERE c.id = $1
	`, channelID).Scan(&sourceTemplateID, &agents, &memberships, &relationships)
	if err != nil {
		t.Fatalf("load persisted team: %v", err)
	}
	if sourceTemplateID != "agency-dev-tech-design-review" || agents != 4 || memberships != 4 || relationships != 4 {
		t.Fatalf("persisted team = template %q, agents %d, memberships %d, relationships %d",
			sourceTemplateID, agents, memberships, relationships)
	}

	_, err = svc.ApplyToChannel(ctx, TemplateProvisionRequest{
		OwnerID:         ownerID,
		TemplateID:      "agency-dev-tech-design-review",
		TargetChannelID: channelID,
		ModelProvider:   "codex",
	})
	if !errors.Is(err, ErrChannelTeamNotEmpty) {
		t.Fatalf("second ApplyToChannel error = %v, want %v", err, ErrChannelTeamNotEmpty)
	}
}
