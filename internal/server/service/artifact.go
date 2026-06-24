package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Artifact struct {
	ID             string    `json:"id"`
	TaskID         string    `json:"task_id"`
	ChannelID      string    `json:"channel_id"`
	Kind           string    `json:"kind"`
	Title          string    `json:"title"`
	HTMLPath       string    `json:"html_path"`
	URL            string    `json:"url"`
	Summary        string    `json:"summary,omitempty"`
	SourceSnapshot []byte    `json:"-"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ArtifactService struct {
	pool              *pgxpool.Pool
	rootDir           string
	artifactRequester func(context.Context, *Task, artifactRenderData, string, string) error
}

type ArtifactTask struct {
	ID, ChannelID, Title, Description, Status, Priority string
	Number                                              int
	CreatorName, ClaimerName                            string
	CreatedAt, UpdatedAt                                time.Time
}

type ArtifactMessage struct {
	ID, SenderType, SenderName, Content string
	CreatedAt                           time.Time
	Attachments                         []ArtifactAttachment
}

type ArtifactAttachment struct {
	ID, Filename, MIMEType, URL string
	Size                        int64
}

type artifactRenderData struct {
	Task        ArtifactTask
	RootMessage ArtifactMessage
	Thread      []ArtifactMessage
	GeneratedAt time.Time
	Mode        string
}

type artifactSnapshot struct {
	TaskID           string   `json:"task_id"`
	MessageID        string   `json:"message_id"`
	ThreadMessageIDs []string `json:"thread_message_ids"`
	AttachmentIDs    []string `json:"attachment_ids"`
	Mode             string   `json:"mode"`
}

func NewArtifactService(pool *pgxpool.Pool, rootDir string) *ArtifactService {
	if rootDir == "" {
		rootDir = filepath.Join(".", ".solo", "artifacts")
	}
	return &ArtifactService{pool: pool, rootDir: rootDir}
}

func (s *ArtifactService) SetAgentArtifactRequester(requester func(context.Context, *Task, artifactRenderData, string, string) error) {
	s.artifactRequester = requester
}

func (s *ArtifactService) GenerateLatest(ctx context.Context, taskID, userID string) (*Artifact, error) {
	return s.generate(ctx, taskID, userID, "latest")
}

func (s *ArtifactService) Finalize(ctx context.Context, taskID, userID string) (*Artifact, error) {
	return s.generate(ctx, taskID, userID, "final")
}

func (s *ArtifactService) Latest(ctx context.Context, taskID, userID string) (*Artifact, error) {
	return s.LatestMode(ctx, taskID, userID, "latest")
}

func (s *ArtifactService) LatestMode(ctx context.Context, taskID, userID, mode string) (*Artifact, error) {
	task, err := NewTaskService(s.pool).GetTaskGlobal(ctx, taskID, userID)
	if err != nil {
		return nil, err
	}
	return s.getByTaskPath(ctx, task.ID, userID, artifactFilename(mode))
}

func (s *ArtifactService) Get(ctx context.Context, artifactID, userID string) (*Artifact, error) {
	var a Artifact
	err := s.pool.QueryRow(ctx, `SELECT id, task_id, channel_id, kind, title, html_path, COALESCE(summary, ''), source_snapshot, created_by, created_at, updated_at FROM artifacts WHERE id = $1`, artifactID).
		Scan(&a.ID, &a.TaskID, &a.ChannelID, &a.Kind, &a.Title, &a.HTMLPath, &a.Summary, &a.SourceSnapshot, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}
	if _, err := NewTaskService(s.pool).GetTask(ctx, a.ChannelID, a.TaskID, userID); err != nil {
		return nil, err
	}
	a.URL = "/api/v1/artifacts/" + a.ID
	return &a, nil
}

func (s *ArtifactService) Publish(ctx context.Context, taskID, userID, mode, htmlContent string) (*Artifact, error) {
	if mode != "latest" && mode != "final" {
		return nil, errors.New("invalid artifact mode")
	}
	if strings.TrimSpace(htmlContent) == "" {
		return nil, errors.New("artifact html is required")
	}
	task, err := NewTaskService(s.pool).GetTaskGlobal(ctx, taskID, userID)
	if err != nil {
		return nil, err
	}
	if userID != task.CreatorID && userID != task.ClaimerID {
		return nil, ErrTaskNotClaimer
	}
	path := s.artifactPath(task.ID, mode)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(htmlContent), 0o644); err != nil {
		return nil, err
	}
	snapshot, err := s.currentArtifactSnapshot(ctx, task, mode)
	if err != nil {
		return nil, err
	}
	return s.upsertArtifact(ctx, task, userID, mode, path, snapshot, "")
}

func (s *ArtifactService) generate(ctx context.Context, taskID, userID, mode string) (*Artifact, error) {
	task, err := NewTaskService(s.pool).GetTaskGlobal(ctx, taskID, userID)
	if err != nil {
		return nil, err
	}
	data, err := s.loadRenderData(ctx, task)
	if err != nil {
		return nil, err
	}
	data.GeneratedAt = time.Now().UTC()
	data.Mode = mode
	snapshot, err := buildArtifactSnapshot(task.ID, data.RootMessage, data.Thread, mode)
	if err != nil {
		return nil, err
	}
	if existing, err := s.getByTaskPath(ctx, task.ID, userID, artifactFilename(mode)); err == nil && bytes.Equal(existing.SourceSnapshot, snapshot) {
		return existing, nil
	}
	if s.artifactRequester != nil {
		if err := s.artifactRequester(ctx, task, data, userID, mode); err != nil {
			slog.Warn("artifact: failed to request agent artifact", "task_id", taskID, "mode", mode, "error", err)
		}
	}

	path := s.artifactPath(task.ID, mode)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(renderPendingArtifactHTML(data)), 0o644); err != nil {
		return nil, err
	}

	return s.upsertArtifact(ctx, task, userID, mode, path, snapshot, "pending")
}

func (s *ArtifactService) currentArtifactSnapshot(ctx context.Context, task *Task, mode string) ([]byte, error) {
	data, err := s.loadRenderData(ctx, task)
	if err == nil {
		return buildArtifactSnapshot(task.ID, data.RootMessage, data.Thread, mode)
	}
	return json.Marshal(artifactSnapshot{TaskID: task.ID, MessageID: task.MessageID, Mode: mode})
}

func (s *ArtifactService) loadRenderData(ctx context.Context, task *Task) (artifactRenderData, error) {
	if task.MessageID == "" {
		return artifactRenderData{}, errors.New("task has no source message")
	}

	rootMessage, err := s.loadArtifactMessage(ctx, task.MessageID)
	if err != nil {
		return artifactRenderData{}, err
	}
	thread, err := s.loadArtifactThread(ctx, task.MessageID, task.ChannelID)
	if err != nil {
		return artifactRenderData{}, err
	}
	if err := s.attachArtifactMetadata(ctx, append([]*ArtifactMessage{&rootMessage}, messagePointers(thread)...)); err != nil {
		return artifactRenderData{}, err
	}

	return artifactRenderData{
		Task: ArtifactTask{
			ID:          task.ID,
			ChannelID:   task.ChannelID,
			Title:       task.Title,
			Description: task.Description,
			Status:      task.Status,
			Priority:    task.Priority,
			Number:      task.TaskNumber,
			CreatorName: task.CreatorName,
			ClaimerName: task.ClaimerName,
			CreatedAt:   task.CreatedAt,
			UpdatedAt:   task.UpdatedAt,
		},
		RootMessage: rootMessage,
		Thread:      thread,
	}, nil
}

func (s *ArtifactService) artifactPath(taskID, mode string) string {
	return filepath.Join(s.rootDir, taskID, artifactFilename(mode))
}

func (s *ArtifactService) upsertArtifact(ctx context.Context, task *Task, userID, mode, path string, snapshot []byte, summary string) (*Artifact, error) {
	var a Artifact
	err := s.pool.QueryRow(ctx, `
		INSERT INTO artifacts (task_id, channel_id, kind, title, html_path, source_snapshot, created_by, summary, updated_at)
		VALUES ($1, $2, 'task_snapshot', $3, $4, $5, $6, $7, now())
		ON CONFLICT (task_id, kind, html_path) DO UPDATE
		SET title = EXCLUDED.title,
		    source_snapshot = EXCLUDED.source_snapshot,
		    summary = EXCLUDED.summary,
		    updated_at = now()
		RETURNING id, task_id, channel_id, kind, title, html_path, COALESCE(summary, ''), source_snapshot, created_by, created_at, updated_at`,
		task.ID, task.ChannelID, task.Title, path, snapshot, userID, summary,
	).Scan(&a.ID, &a.TaskID, &a.ChannelID, &a.Kind, &a.Title, &a.HTMLPath, &a.Summary, &a.SourceSnapshot, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	a.URL = "/api/v1/artifacts/" + a.ID
	return &a, nil
}

func (s *ArtifactService) getByTaskPath(ctx context.Context, taskID, userID, filename string) (*Artifact, error) {
	var a Artifact
	err := s.pool.QueryRow(ctx, `
		SELECT id, task_id, channel_id, kind, title, html_path, COALESCE(summary, ''), source_snapshot, created_by, created_at, updated_at
		FROM artifacts
		WHERE task_id = $1 AND kind = 'task_snapshot' AND html_path LIKE '%' || $2
		ORDER BY updated_at DESC
		LIMIT 1`,
		taskID, filename,
	).Scan(&a.ID, &a.TaskID, &a.ChannelID, &a.Kind, &a.Title, &a.HTMLPath, &a.Summary, &a.SourceSnapshot, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}
	if _, err := NewTaskService(s.pool).GetTask(ctx, a.ChannelID, a.TaskID, userID); err != nil {
		return nil, err
	}
	a.URL = "/api/v1/artifacts/" + a.ID
	return &a, nil
}

func (s *ArtifactService) loadArtifactMessage(ctx context.Context, messageID string) (ArtifactMessage, error) {
	var msg ArtifactMessage
	var attachmentIDs []string
	err := s.pool.QueryRow(ctx, `
		SELECT m.id, m.sender_type, COALESCE(u.display_name, ag.name, ''), m.content, m.created_at, COALESCE(m.attachment_ids, '{}')
		FROM messages m
		LEFT JOIN users u ON m.sender_type = 'user' AND m.sender_id = u.id
		LEFT JOIN agents ag ON m.sender_type = 'agent' AND m.sender_id = ag.id
		WHERE m.id = $1`,
		messageID,
	).Scan(&msg.ID, &msg.SenderType, &msg.SenderName, &msg.Content, &msg.CreatedAt, &attachmentIDs)
	if err != nil {
		return ArtifactMessage{}, err
	}
	msg.Attachments = attachmentPlaceholders(attachmentIDs)
	return msg, nil
}

func (s *ArtifactService) loadArtifactThread(ctx context.Context, rootMessageID, channelID string) ([]ArtifactMessage, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT m.id, m.sender_type, COALESCE(u.display_name, ag.name, ''), m.content, m.created_at, COALESCE(m.attachment_ids, '{}')
		FROM threads t
		JOIN messages m ON m.thread_id = t.id
		LEFT JOIN users u ON m.sender_type = 'user' AND m.sender_id = u.id
		LEFT JOIN agents ag ON m.sender_type = 'agent' AND m.sender_id = ag.id
		WHERE t.root_message_id = $1 AND t.channel_id = $2
		ORDER BY m.created_at ASC, m.id ASC`,
		rootMessageID, channelID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []ArtifactMessage
	for rows.Next() {
		var msg ArtifactMessage
		var attachmentIDs []string
		if err := rows.Scan(&msg.ID, &msg.SenderType, &msg.SenderName, &msg.Content, &msg.CreatedAt, &attachmentIDs); err != nil {
			return nil, err
		}
		msg.Attachments = attachmentPlaceholders(attachmentIDs)
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

func (s *ArtifactService) attachArtifactMetadata(ctx context.Context, messages []*ArtifactMessage) error {
	ids := uniqueArtifactAttachmentIDs(messages)
	if len(ids) == 0 {
		return nil
	}

	rows, err := s.pool.Query(ctx, `SELECT id, filename, mime_type, size FROM attachments WHERE id = ANY($1::uuid[])`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()

	byID := make(map[string]ArtifactAttachment, len(ids))
	for rows.Next() {
		var a ArtifactAttachment
		if err := rows.Scan(&a.ID, &a.Filename, &a.MIMEType, &a.Size); err != nil {
			return err
		}
		a.URL = artifactAttachmentURL(a.ID)
		byID[a.ID] = a
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, msg := range messages {
		for i, placeholder := range msg.Attachments {
			if a, ok := byID[placeholder.ID]; ok {
				msg.Attachments[i] = a
			}
		}
	}
	return nil
}

func artifactFilename(mode string) string {
	if mode == "final" {
		return "final.html"
	}
	return "latest.html"
}

func renderArtifactAgentPrompt(data artifactRenderData, mode string) string {
	var b strings.Builder
	b.WriteString("Generate a Solo artifact for this task.\n\n")
	b.WriteString("Use the `solo-artifacts` skill from this workspace. Keep the fixed Solo-brutal template/style from that skill; fill it with this task's actual conclusions, review decision, evidence, and provenance.\n\n")
	b.WriteString("Write one self-contained HTML file. Then publish it with:\n")
	b.WriteString("solo artifact publish --task ")
	b.WriteString(data.Task.ID)
	b.WriteString(" --mode ")
	b.WriteString(mode)
	b.WriteString(" --file <path-to-your-html>\n\n")
	b.WriteString("Prefer the `review-decision` artifact type unless the thread clearly asks for progress-report or comparison.\n\n")
	b.WriteString("Task:\n")
	b.WriteString("- ID: ")
	b.WriteString(data.Task.ID)
	b.WriteString("\n- Channel ID: ")
	b.WriteString(data.Task.ChannelID)
	b.WriteString("\n- Number: #")
	b.WriteString(stringInt(data.Task.Number))
	b.WriteString("\n- Title: ")
	b.WriteString(data.Task.Title)
	b.WriteString("\n- Status: ")
	b.WriteString(data.Task.Status)
	b.WriteString("\n- Priority: ")
	b.WriteString(data.Task.Priority)
	b.WriteString("\n- Creator: ")
	b.WriteString(data.Task.CreatorName)
	b.WriteString("\n- Claimer: ")
	b.WriteString(data.Task.ClaimerName)
	b.WriteString("\n- Description:\n")
	b.WriteString(data.Task.Description)
	b.WriteString("\n\nRoot message:\n")
	writeArtifactPromptMessage(&b, data.RootMessage)
	b.WriteString("\nThread messages:\n")
	if len(data.Thread) == 0 {
		b.WriteString("(none)\n")
	}
	for _, msg := range data.Thread {
		writeArtifactPromptMessage(&b, msg)
	}
	b.WriteString("\nDo not produce a transcript clone. Make the page useful for a human reviewer inside Solo: conclusions first, decisions explicit, evidence compact, copy-ready commands where relevant.")
	return b.String()
}

func writeArtifactPromptMessage(b *strings.Builder, msg ArtifactMessage) {
	b.WriteString("- ")
	b.WriteString(msg.SenderName)
	if !msg.CreatedAt.IsZero() {
		b.WriteString(" at ")
		b.WriteString(msg.CreatedAt.Format(time.RFC3339))
	}
	b.WriteString(":\n")
	b.WriteString(msg.Content)
	b.WriteString("\n")
	if len(msg.Attachments) == 0 {
		return
	}
	b.WriteString("  Attachments:\n")
	for _, a := range msg.Attachments {
		b.WriteString("  - ")
		b.WriteString(a.Filename)
		b.WriteString(" ")
		b.WriteString(a.MIMEType)
		b.WriteString(" ")
		b.WriteString(a.URL)
		b.WriteString("\n")
	}
}

func renderPendingArtifactHTML(data artifactRenderData) string {
	var b strings.Builder
	b.WriteString("<!doctype html><html><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">")
	b.WriteString("<title>")
	b.WriteString(html.EscapeString(data.Task.Title))
	b.WriteString("</title><style>")
	b.WriteString("body{font-family:ui-sans-serif,system-ui;margin:0;background:#f8fafc;color:#0f172a}main{max-width:760px;margin:0 auto;padding:48px 24px}.panel{background:white;border:3px solid #0f172a;box-shadow:8px 8px 0 #2563eb;padding:24px}h1{font-size:28px;margin:0 0 12px;font-weight:900}.badge{display:inline-block;border:2px solid #0f172a;padding:4px 8px;font-size:12px;font-weight:900;text-transform:uppercase}.meta{display:flex;gap:8px;flex-wrap:wrap;margin:16px 0}p{line-height:1.55}footer{margin-top:20px;color:#64748b;font-size:12px}")
	b.WriteString("</style></head><body><main>")
	b.WriteString("<section class=\"panel\"><div class=\"badge\">Generating artifact</div><h1>")
	b.WriteString(html.EscapeString(data.Task.Title))
	b.WriteString("</h1><div class=\"meta\"><span class=\"badge\">Task ")
	b.WriteString(html.EscapeString(data.Task.ID))
	b.WriteString("</span><span class=\"badge\">#")
	b.WriteString(html.EscapeString(stringInt(data.Task.Number)))
	b.WriteString("</span><span class=\"badge\">")
	b.WriteString(html.EscapeString(data.Task.Status))
	b.WriteString("</span><span class=\"badge\">")
	b.WriteString(html.EscapeString(data.Mode))
	b.WriteString("</span></div>")
	b.WriteString("<p>The leader agent is building the Solo-brutal artifact and will publish it back into Solo with <code>solo artifact publish</code>.</p>")
	b.WriteString("<footer>Requested at ")
	b.WriteString(html.EscapeString(data.GeneratedAt.Format(time.RFC3339)))
	b.WriteString(" in channel ")
	b.WriteString(html.EscapeString(data.Task.ChannelID))
	b.WriteString(".</footer></section></main></body></html>")
	return b.String()
}

func stringInt(n int) string {
	return strconv.Itoa(n)
}

func attachmentPlaceholders(ids []string) []ArtifactAttachment {
	attachments := make([]ArtifactAttachment, 0, len(ids))
	for _, id := range ids {
		attachments = append(attachments, ArtifactAttachment{ID: id, URL: artifactAttachmentURL(id)})
	}
	return attachments
}

func artifactAttachmentURL(id string) string {
	return "/api/v1/attachments/" + id
}

func messagePointers(messages []ArtifactMessage) []*ArtifactMessage {
	pointers := make([]*ArtifactMessage, 0, len(messages))
	for i := range messages {
		pointers = append(pointers, &messages[i])
	}
	return pointers
}

func uniqueArtifactAttachmentIDs(messages []*ArtifactMessage) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, msg := range messages {
		for _, a := range msg.Attachments {
			if a.ID == "" || seen[a.ID] {
				continue
			}
			seen[a.ID] = true
			ids = append(ids, a.ID)
		}
	}
	return ids
}

func buildArtifactSnapshot(taskID string, root ArtifactMessage, thread []ArtifactMessage, mode string) ([]byte, error) {
	snapshot := artifactSnapshot{
		TaskID:    taskID,
		MessageID: root.ID,
		Mode:      mode,
	}
	for _, msg := range thread {
		snapshot.ThreadMessageIDs = append(snapshot.ThreadMessageIDs, msg.ID)
	}
	snapshot.AttachmentIDs = uniqueArtifactAttachmentIDs(append([]*ArtifactMessage{&root}, messagePointers(thread)...))
	return json.Marshal(snapshot)
}
