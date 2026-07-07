package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/server/ws"
)

type ThoughtHandler struct {
	pool *pgxpool.Pool
	hub  *ws.Hub
}

func NewThoughtHandler(pool *pgxpool.Pool, hub *ws.Hub) *ThoughtHandler {
	return &ThoughtHandler{pool: pool, hub: hub}
}

type CreateThoughtRequest struct {
	Title     string `json:"title,omitempty"`
	MessageID string `json:"message_id,omitempty"`
}

type CompleteThoughtRequest struct {
	Summary   string `json:"summary,omitempty"`
	MessageID string `json:"message_id,omitempty"`
}

type ThoughtNodeResponse struct {
	ID        string `json:"id"`
	ThoughtID string `json:"thought_id"`
	ParentID  string `json:"parent_id,omitempty"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	IsRoot    bool   `json:"is_root"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type ThoughtResponse struct {
	ID              string                `json:"id"`
	ChannelID       string                `json:"channel_id"`
	Title           string                `json:"title"`
	Status          string                `json:"status"`
	SelectedNodeID  string                `json:"selected_node_id,omitempty"`
	Nodes           []ThoughtNodeResponse `json:"nodes"`
	SummaryRecords  []RecordResponse      `json:"summary_records"`
	InsightRecords  []RecordResponse      `json:"insight_records"`
	ArtifactRecords []RecordResponse      `json:"artifact_records"`
	CreatedAt       string                `json:"created_at"`
	UpdatedAt       string                `json:"updated_at"`
}

type ThoughtListResponse struct {
	Thoughts []ThoughtResponse `json:"thoughts"`
}

type thoughtAgendaItem struct {
	ID       string              `json:"id"`
	Title    string              `json:"title"`
	Children []thoughtAgendaItem `json:"children,omitempty"`
}

func shortThoughtText(value string, fallback string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func thoughtTitleFromContext(inputTitle, target string) string {
	if strings.TrimSpace(inputTitle) != "" {
		return shortThoughtText(inputTitle, "产品路径探索", 100)
	}
	target = shortThoughtText(target, "", 70)
	if target == "" {
		return "产品路径探索"
	}
	return shortThoughtText(target+" 探索", "产品路径探索", 100)
}

func addThoughtNodeTitle(out *[]string, seen map[string]bool, title string) {
	title = shortThoughtText(title, "", 40)
	if title == "" || seen[title] {
		return
	}
	seen[title] = true
	*out = append(*out, title)
}

func thoughtFallbackNodeTitles(target string) []string {
	lower := strings.ToLower(target)
	if strings.Contains(lower, "chat") || strings.Contains(target, "聊天") {
		return []string{"用户场景", "对话体验", "技术方案"}
	}
	if strings.Contains(lower, "research") || strings.Contains(target, "研究") || strings.Contains(target, "调研") {
		return []string{"研究问题", "资料范围", "输出形式"}
	}
	if strings.Contains(lower, "solo") || strings.Contains(target, "复制") {
		return []string{"产品路径", "技术架构", "验证策略"}
	}
	return []string{"目标澄清", "方案选项", "风险验证"}
}

func thoughtNodeTitlesFromContext(target, agendaRaw string) []string {
	titles := []string{}
	seen := map[string]bool{}
	var agenda []thoughtAgendaItem
	if json.Unmarshal([]byte(agendaRaw), &agenda) == nil {
		for _, item := range agenda {
			if !strings.Contains(item.ID, "explore") && !strings.Contains(item.Title, "探索") {
				continue
			}
			for _, child := range item.Children {
				addThoughtNodeTitle(&titles, seen, child.Title)
			}
		}
	}
	for _, title := range thoughtFallbackNodeTitles(target) {
		addThoughtNodeTitle(&titles, seen, title)
	}
	if len(titles) > 5 {
		return titles[:5]
	}
	return titles
}

func (h *ThoughtHandler) ListByChannel(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	channelID := chi.URLParam(r, "channelID")
	if !h.requireChannelMember(r.Context(), w, channelID, userID) {
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT id::text
		   FROM thought_sessions
		  WHERE channel_id = $1
		  ORDER BY created_at DESC
		  LIMIT 10`,
		channelID,
	)
	if err != nil {
		slog.Error("failed to list thoughts", "channel_id", channelID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list thoughts")
		return
	}
	defer rows.Close()

	thoughts := []ThoughtResponse{}
	for rows.Next() {
		var thoughtID string
		if err := rows.Scan(&thoughtID); err != nil {
			continue
		}
		thought, err := h.thoughtResponse(r.Context(), thoughtID)
		if err == nil {
			thoughts = append(thoughts, thought)
		}
	}
	writeJSON(w, http.StatusOK, ThoughtListResponse{Thoughts: thoughts})
}

func (h *ThoughtHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	channelID := chi.URLParam(r, "channelID")
	if !h.requireChannelMember(r.Context(), w, channelID, userID) {
		return
	}

	var req CreateThoughtRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start thought")
		return
	}
	defer tx.Rollback(r.Context())

	target := ""
	agendaRaw := "[]"
	_ = tx.QueryRow(r.Context(),
		`SELECT target, COALESCE(agenda_json, '[]'::jsonb)::text FROM channel_contexts WHERE channel_id = $1`,
		channelID,
	).Scan(&target, &agendaRaw)

	title := thoughtTitleFromContext(req.Title, target)
	if len([]rune(title)) > 100 {
		writeError(w, http.StatusBadRequest, "title must be 100 characters or less")
		return
	}

	var thoughtID string
	if err := tx.QueryRow(r.Context(),
		`INSERT INTO thought_sessions (channel_id, title, status, created_by)
		 VALUES ($1, $2, 'in_progress', $3)
		 RETURNING id::text`,
		channelID, title, userID,
	).Scan(&thoughtID); err != nil {
		slog.Error("failed to create thought", "channel_id", channelID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to start thought")
		return
	}

	var rootID string
	if err := tx.QueryRow(r.Context(),
		`INSERT INTO thought_nodes (thought_id, title, status, is_root)
		 VALUES ($1, 'Root', 'in_progress', true)
		 RETURNING id::text`,
		thoughtID,
	).Scan(&rootID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start thought")
		return
	}

	for _, node := range thoughtNodeTitlesFromContext(target, agendaRaw) {
		if _, err := tx.Exec(r.Context(),
			`INSERT INTO thought_nodes (thought_id, parent_id, title, status)
			 VALUES ($1, $2, $3, 'todo')`,
			thoughtID, rootID, node,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to start thought")
			return
		}
	}

	if req.MessageID != "" {
		if err := markCardAccepted(r.Context(), tx, req.MessageID); err != nil {
			slog.Warn("failed to mark next step card accepted", "message_id", req.MessageID, "error", err)
		}
	}

	if err := seedThoughtRecords(r.Context(), tx, channelID, thoughtID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start thought")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start thought")
		return
	}

	thought, err := h.thoughtResponse(r.Context(), thoughtID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load thought")
		return
	}
	writeJSON(w, http.StatusCreated, thought)
}

func (h *ThoughtHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	thoughtID := chi.URLParam(r, "thoughtID")
	channelID, ok := h.channelForThought(r.Context(), w, thoughtID, userID)
	if !ok {
		return
	}
	_ = channelID

	thought, err := h.thoughtResponse(r.Context(), thoughtID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load thought")
		return
	}
	writeJSON(w, http.StatusOK, thought)
}

func (h *ThoughtHandler) RequestReview(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	thoughtID := chi.URLParam(r, "thoughtID")
	channelID, ok := h.channelForThought(r.Context(), w, thoughtID, userID)
	if !ok {
		return
	}

	var title, status string
	if err := h.pool.QueryRow(r.Context(),
		`SELECT title, status FROM thought_sessions WHERE id = $1`,
		thoughtID,
	).Scan(&title, &status); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to request thought review")
		return
	}
	if status != "done" {
		if err := h.emitThoughtReviewCard(r.Context(), channelID, thoughtID, title); err != nil {
			slog.Warn("failed to emit thought review card", "thought_id", thoughtID, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to request thought review")
			return
		}
	}

	thought, err := h.thoughtResponse(r.Context(), thoughtID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load thought")
		return
	}
	writeJSON(w, http.StatusOK, thought)
}

func (h *ThoughtHandler) Complete(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	thoughtID := chi.URLParam(r, "thoughtID")
	channelID, ok := h.channelForThought(r.Context(), w, thoughtID, userID)
	if !ok {
		return
	}

	var req CompleteThoughtRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	summary := strings.TrimSpace(req.Summary)
	if summary == "" {
		summary = "Thought 已完成，探索摘要已回传到 Channel Context。"
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to complete thought")
		return
	}
	defer tx.Rollback(r.Context())

	var title string
	if err := tx.QueryRow(r.Context(),
		`UPDATE thought_sessions
		    SET status = 'done', updated_at = now()
		  WHERE id = $1
		  RETURNING title`,
		thoughtID,
	).Scan(&title); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to complete thought")
		return
	}

	if _, err := tx.Exec(r.Context(),
		`UPDATE thought_nodes SET status = 'done', updated_at = now() WHERE thought_id = $1`,
		thoughtID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to complete thought")
		return
	}

	if _, err := tx.Exec(r.Context(),
		`INSERT INTO context_records (channel_id, scope, subject_type, subject_id, record_type, title, body, author_type)
		 VALUES ($1, 'channel', 'thought', $2, 'summary', $3, $4, 'system')`,
		channelID, thoughtID, "Thought done: "+title, summary,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to complete thought")
		return
	}

	if _, err := tx.Exec(r.Context(),
		`INSERT INTO channel_contexts (channel_id)
		 VALUES ($1)
		 ON CONFLICT (channel_id) DO UPDATE SET
			context_version = channel_contexts.context_version + 1,
			updated_at = now()`,
		channelID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to complete thought")
		return
	}

	if req.MessageID != "" {
		if err := markCardAccepted(r.Context(), tx, req.MessageID); err != nil {
			slog.Warn("failed to mark thought review card accepted", "message_id", req.MessageID, "error", err)
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to complete thought")
		return
	}

	thought, err := h.thoughtResponse(r.Context(), thoughtID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load thought")
		return
	}
	writeJSON(w, http.StatusOK, thought)
}

func (h *ThoughtHandler) requireChannelMember(ctx context.Context, w http.ResponseWriter, channelID, userID string) bool {
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "channel ID is required")
		return false
	}
	var isMember bool
	if err := h.pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM channel_members
			WHERE channel_id = $1 AND member_type = 'user' AND member_id = $2
		)`,
		channelID, userID,
	).Scan(&isMember); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return false
	}
	if !isMember {
		writeError(w, http.StatusNotFound, "channel not found")
		return false
	}
	return true
}

func (h *ThoughtHandler) channelForThought(ctx context.Context, w http.ResponseWriter, thoughtID, userID string) (string, bool) {
	if thoughtID == "" {
		writeError(w, http.StatusBadRequest, "thought ID is required")
		return "", false
	}
	var channelID string
	err := h.pool.QueryRow(ctx,
		`SELECT ts.channel_id::text
		   FROM thought_sessions ts
		   JOIN channel_members cm ON cm.channel_id = ts.channel_id
		  WHERE ts.id = $1 AND cm.member_type = 'user' AND cm.member_id = $2`,
		thoughtID, userID,
	).Scan(&channelID)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "thought not found")
			return "", false
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return "", false
	}
	return channelID, true
}

func (h *ThoughtHandler) thoughtResponse(ctx context.Context, thoughtID string) (ThoughtResponse, error) {
	var thought ThoughtResponse
	var createdAt, updatedAt time.Time
	if err := h.pool.QueryRow(ctx,
		`SELECT id::text, channel_id::text, title, status, created_at, updated_at
		   FROM thought_sessions
		  WHERE id = $1`,
		thoughtID,
	).Scan(&thought.ID, &thought.ChannelID, &thought.Title, &thought.Status, &createdAt, &updatedAt); err != nil {
		return thought, err
	}
	thought.CreatedAt = createdAt.Format(time.RFC3339)
	thought.UpdatedAt = updatedAt.Format(time.RFC3339)

	nodes, err := h.thoughtNodes(ctx, thoughtID)
	if err != nil {
		return thought, err
	}
	thought.Nodes = nodes
	for _, node := range nodes {
		if node.IsRoot {
			thought.SelectedNodeID = node.ID
			break
		}
	}
	thought.SummaryRecords = h.thoughtRecords(ctx, thought.ChannelID, thoughtID, "summary")
	thought.InsightRecords = h.thoughtRecords(ctx, thought.ChannelID, thoughtID, "insight")
	thought.ArtifactRecords = h.thoughtRecords(ctx, thought.ChannelID, thoughtID, "artifact")
	return thought, nil
}

func (h *ThoughtHandler) thoughtNodes(ctx context.Context, thoughtID string) ([]ThoughtNodeResponse, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT id::text, thought_id::text, COALESCE(parent_id::text, ''), title, status, is_root, created_at, updated_at
		   FROM thought_nodes
		  WHERE thought_id = $1
		  ORDER BY is_root DESC, created_at ASC`,
		thoughtID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nodes := []ThoughtNodeResponse{}
	for rows.Next() {
		var node ThoughtNodeResponse
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&node.ID, &node.ThoughtID, &node.ParentID, &node.Title, &node.Status, &node.IsRoot, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		node.CreatedAt = createdAt.Format(time.RFC3339)
		node.UpdatedAt = updatedAt.Format(time.RFC3339)
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

func (h *ThoughtHandler) thoughtRecords(ctx context.Context, channelID, thoughtID, recordType string) []RecordResponse {
	rows, err := h.pool.Query(ctx,
		`SELECT id::text, channel_id::text, scope, COALESCE(subject_type, ''), COALESCE(subject_id::text, ''),
		        record_type, title, body, author_type, COALESCE(author_id::text, ''),
		        COALESCE(artifact_ref_json::text, ''), created_at
		   FROM context_records
		  WHERE channel_id = $1 AND scope = 'thought' AND subject_type = 'thought' AND subject_id = $2 AND record_type = $3
		  ORDER BY created_at DESC
		  LIMIT 20`,
		channelID, thoughtID, recordType,
	)
	if err != nil {
		return []RecordResponse{}
	}
	defer rows.Close()

	records := []RecordResponse{}
	for rows.Next() {
		var rec RecordResponse
		var artifactRef string
		var createdAt time.Time
		if err := rows.Scan(&rec.ID, &rec.ChannelID, &rec.Scope, &rec.SubjectType, &rec.SubjectID, &rec.RecordType, &rec.Title, &rec.Body, &rec.AuthorType, &rec.AuthorID, &artifactRef, &createdAt); err != nil {
			continue
		}
		if artifactRef != "" {
			rec.ArtifactRef = json.RawMessage(artifactRef)
		}
		rec.CreatedAt = createdAt.Format(time.RFC3339)
		records = append(records, rec)
	}
	return records
}

func seedThoughtRecords(ctx context.Context, tx pgx.Tx, channelID, thoughtID string) error {
	records := []struct {
		recordType string
		title      string
		body       string
		artifact   string
	}{
		{"summary", "探索摘要", "围绕当前 Channel Target 展开探索。", ""},
		{"insight", "关键洞察", "先收敛产品路径，再进入任务拆分。", ""},
		{"artifact", "PRD Draft", "Root 产物入口。", `{"title":"PRD Draft","kind":"md"}`},
		{"artifact", "Spec.md", "Root 产物入口。", `{"title":"Spec.md","kind":"md"}`},
		{"artifact", "Test Plan.md", "Root 产物入口。", `{"title":"Test Plan.md","kind":"md"}`},
	}
	for _, rec := range records {
		var artifact any
		if rec.artifact != "" {
			artifact = rec.artifact
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO context_records (channel_id, scope, subject_type, subject_id, record_type, title, body, author_type, artifact_ref_json)
			 VALUES ($1, 'thought', 'thought', $2, $3, $4, $5, 'system', $6::jsonb)`,
			channelID, thoughtID, rec.recordType, rec.title, rec.body, artifact,
		); err != nil {
			return err
		}
	}
	return nil
}

func thoughtReviewCardPayload(thoughtID, title string) map[string]any {
	return map[string]any{
		"card_type":  "thought_review",
		"thought_id": thoughtID,
		"title":      title,
		"summary":    "当前 Thought 已准备好回传到 Channel Context，并进入 Task。",
		"status":     "open",
	}
}

func (h *ThoughtHandler) emitThoughtReviewCard(ctx context.Context, channelID, thoughtID, title string) error {
	var existingID string
	err := h.pool.QueryRow(ctx,
		`SELECT id::text
		   FROM messages
		  WHERE channel_id = $1
		    AND workspace_scope = 'thought'
		    AND subject_type = 'thought'
		    AND subject_id = $2
		    AND content_type = 'card.thought_review'
		    AND COALESCE(content::jsonb->>'status', 'open') = 'open'
		  LIMIT 1`,
		channelID, thoughtID,
	).Scan(&existingID)
	if err == nil {
		return nil
	}
	if err != pgx.ErrNoRows {
		return err
	}

	content, err := json.Marshal(thoughtReviewCardPayload(thoughtID, title))
	if err != nil {
		return err
	}
	now := time.Now()
	messageID := uuid.New().String()
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO messages (id, channel_id, workspace_scope, subject_type, subject_id, sender_type, sender_id, content, content_type, created_at, updated_at)
		 VALUES ($1, $2, 'thought', 'thought', $3, 'system', '00000000-0000-0000-0000-000000000000', $4, 'card.thought_review', $5, $5)`,
		messageID, channelID, thoughtID, string(content), now,
	); err != nil {
		return err
	}
	if h.hub != nil {
		h.hub.BroadcastToChannel(channelID, ws.Envelope(ws.EventMessageNew, ws.MessageNewPayload{
			ID:             messageID,
			ChannelID:      channelID,
			WorkspaceScope: "thought",
			SubjectType:    "thought",
			SubjectID:      thoughtID,
			SenderType:     "system",
			SenderID:       "00000000-0000-0000-0000-000000000000",
			SenderName:     "Solo",
			Content:        string(content),
			ContentType:    "card.thought_review",
			CreatedAt:      now.Format(time.RFC3339),
		}))
	}
	return nil
}

func markCardAccepted(ctx context.Context, tx pgx.Tx, messageID string) error {
	var raw string
	if err := tx.QueryRow(ctx, `SELECT content FROM messages WHERE id = $1`, messageID).Scan(&raw); err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return err
	}
	payload["status"] = "accepted"
	updated, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE messages SET content = $1, updated_at = now() WHERE id = $2`, string(updated), messageID)
	return err
}
