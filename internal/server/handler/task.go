package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"fmt"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/realtime"
	"github.com/solo-ai/solo/internal/i18n"
	"github.com/solo-ai/solo/internal/server/service"
	"github.com/solo-ai/solo/internal/server/ws"
)

// TaskHandler handles task-related HTTP requests.
type TaskHandler struct {
	pool       *pgxpool.Pool
	hub        realtime.Broadcaster
	agentSvc   *service.AgentService
	svc        *service.TaskService
	mentionSvc *service.MentionService
	swarm      *service.SwarmCoordinator
	watchdog   *service.WatchdogService
}

// NewTaskHandler creates a new TaskHandler.
func NewTaskHandler(pool *pgxpool.Pool, hub realtime.Broadcaster, agentSvc *service.AgentService, taskSvc *service.TaskService, mentionSvc *service.MentionService) *TaskHandler {
	return &TaskHandler{
		pool:       pool,
		hub:        hub,
		agentSvc:   agentSvc,
		svc:        taskSvc,
		mentionSvc: mentionSvc,
	}
}

// SetSwarmCoordinator injects the SwarmCoordinator into the handler.
func (h *TaskHandler) SetSwarmCoordinator(s *service.SwarmCoordinator) {
	h.swarm = s
}

// SetWatchdogService injects the WatchdogService into the handler.
func (h *TaskHandler) SetWatchdogService(s *service.WatchdogService) {
	h.watchdog = s
}

// --- Request/Response types ---

type CreateTaskRequest struct {
	Title        string     `json:"title"`
	Description  string     `json:"description,omitempty"`
	Priority     string     `json:"priority,omitempty"`
	DueDate      *time.Time `json:"due_date,omitempty"`
	ChannelID    string     `json:"channel_id,omitempty"`
	ParentTaskID string     `json:"parent_task_id,omitempty"`
	DependsOn    []string   `json:"depends_on,omitempty"`
}

type UpdateTaskRequest struct {
	Title       *string    `json:"title,omitempty"`
	Description *string    `json:"description,omitempty"`
	Status      *string    `json:"status,omitempty"`
	Priority    *string    `json:"priority,omitempty"`
	DueDate     *time.Time `json:"due_date,omitempty"`
}

type ConvertToTaskRequest struct {
	Title string `json:"title,omitempty"`
}

type TaskResponse struct {
	ID               string   `json:"id"`
	TaskNumber       int      `json:"task_number"`
	ChannelID        string   `json:"channel_id"`
	CreatorID        string   `json:"creator_id"`
	CreatorName      string   `json:"creator_name,omitempty"`
	Title            string   `json:"title"`
	Description      string   `json:"description,omitempty"`
	Status           string   `json:"status"`
	ClaimerID        string   `json:"claimer_id,omitempty"`
	ClaimerName      string   `json:"claimer_name,omitempty"`
	ClaimerDeleted   bool     `json:"claimer_deleted"`
	Priority         string   `json:"priority"`
	DueDate          *string  `json:"due_date,omitempty"`
	MessageID        string   `json:"message_id,omitempty"`
	ParentTaskID     *string  `json:"parent_task_id,omitempty"`
	SubtaskCount     int      `json:"subtask_count,omitempty"`
	DoneSubtaskCount int      `json:"done_subtask_count,omitempty"`
	BlockerIDs       []string `json:"blocker_ids,omitempty"`
	BlockedByCount   int      `json:"blocked_by_count,omitempty"`
	CreatedAt        string   `json:"created_at"`
	UpdatedAt        string   `json:"updated_at"`
}

func toTaskResponse(t *service.Task) TaskResponse {
	r := TaskResponse{
		ID:               t.ID,
		TaskNumber:       t.TaskNumber,
		ChannelID:        t.ChannelID,
		CreatorID:        t.CreatorID,
		CreatorName:      t.CreatorName,
		Title:            t.Title,
		Description:      t.Description,
		Status:           t.Status,
		ClaimerID:        t.ClaimerID,
		ClaimerName:      t.ClaimerName,
		ClaimerDeleted:   t.ClaimerDeleted,
		Priority:         t.Priority,
		MessageID:        t.MessageID,
		ParentTaskID:     t.ParentTaskID,
		SubtaskCount:     t.SubtaskCount,
		DoneSubtaskCount: t.DoneSubtaskCount,
		BlockerIDs:       t.BlockerIDs,
		BlockedByCount:   t.BlockedByCount,
		CreatedAt:        t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:        t.UpdatedAt.Format(time.RFC3339),
	}
	if t.DueDate != nil {
		s := t.DueDate.Format(time.RFC3339)
		r.DueDate = &s
	}
	return r
}

func toTaskResponseList(tasks []service.Task) []TaskResponse {
	resp := make([]TaskResponse, len(tasks))
	for i, t := range tasks {
		resp[i] = toTaskResponse(&t)
	}
	return resp
}

// --- Channel-scoped handlers ---

// Create handles POST /api/v1/channels/{channelID}/tasks
func (h *TaskHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	channelID := chi.URLParam(r, "channelID")
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "channel ID is required")
		return
	}

	var req CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "task title is required")
		return
	}
	if len(req.Title) > 500 {
		writeError(w, http.StatusBadRequest, "task title exceeds maximum length of 500 characters")
		return
	}

	svcReq := service.TaskCreateRequest{
		Title:        req.Title,
		Description:  req.Description,
		Priority:     req.Priority,
		DueDate:      req.DueDate,
		ParentTaskID: req.ParentTaskID,
		DependsOn:    req.DependsOn,
	}

	task, err := h.svc.CreateTask(r.Context(), channelID, userID, svcReq)
	if err != nil {
		switch {
		case err == service.ErrTaskNotChannelMember:
			writeError(w, http.StatusForbidden, "not a channel member")
		default:
			slog.Error("failed to create task", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to create task")
		}
		return
	}

	// Create a persistent system message for the task, and a thread for that
	// message so task discussion is organized under the thread.
	var threadID string
	msgID := uuid.New().String()
	now := time.Now()
	sysContent := formatSystemMessage(task.TaskNumber, task.Title, i18n.Active.SysTaskCreated)
	_, dbErr := h.pool.Exec(r.Context(),
		`INSERT INTO messages (id, channel_id, sender_type, sender_id, content, content_type, created_at, updated_at)
		 VALUES ($1, $2, 'system', '00000000-0000-0000-0000-000000000000', $3, 'system', $4, $4)`,
		msgID, channelID, sysContent, now,
	)
	if dbErr != nil {
		slog.Error("failed to persist task system message", "task_id", task.ID, "error", dbErr)
	} else {
		threadSvc := service.NewThreadService(h.pool)
		tid, isNew, threadErr := threadSvc.GetOrCreateThread(r.Context(), channelID, msgID)
		if threadErr != nil {
			slog.Error("failed to create thread for task", "task_id", task.ID, "error", threadErr)
		} else {
			threadID = tid
			_, updErr := h.pool.Exec(r.Context(),
				`UPDATE tasks SET message_id = $1 WHERE id = $2`,
				msgID, task.ID,
			)
			if updErr != nil {
				slog.Error("failed to link task message_id", "task_id", task.ID, "error", updErr)
			}
			task.MessageID = msgID
			if isNew {
				slog.Debug("created thread for task",
					"task_id", task.ID,
					"thread_id", threadID,
				)
			}
		}
	}

	resp := toTaskResponse(task)
	writeJSON(w, http.StatusCreated, resp)

	dueDate := ""
	if task.DueDate != nil {
		dueDate = task.DueDate.Format(time.RFC3339)
	}
	ws.BroadcastTaskCreated(h.hub, ws.TaskCreatedPayload{
		ID:          task.ID,
		TaskNumber:  task.TaskNumber,
		ChannelID:   task.ChannelID,
		CreatorID:   task.CreatorID,
		CreatorName: task.CreatorName,
		Title:       task.Title,
		Description: task.Description,
		Status:      task.Status,
		ClaimerID:   task.ClaimerID,
		ClaimerName: task.ClaimerName,
		Priority:    task.Priority,
		DueDate:     dueDate,
		MessageID:   task.MessageID,
		CreatedAt:   resp.CreatedAt,
		UpdatedAt:   resp.UpdatedAt,
	})

	if task.MessageID != "" {
	}

	h.broadcastSystemMessageWithID(task.ChannelID, threadID, task.TaskNumber, task.Title, i18n.Active.SysTaskCreated, msgID, now, true)

	if h.agentSvc != nil {
		contentForMentions := task.Title
		if task.Description != "" {
			contentForMentions += " " + task.Description
		}
		var mentionedAgentIDs []string
		if h.mentionSvc != nil {
			ids, _, err := h.mentionSvc.ResolveMentions(r.Context(), contentForMentions, task.ChannelID)
			if err == nil {
				mentionedAgentIDs = ids
			}
		}
		go h.agentSvc.TriggerAllAgentsForTask(context.Background(), task.ChannelID, task.ID, task.TaskNumber, task.Title, mentionedAgentIDs, nil)
	}
}

// List handles GET /api/v1/channels/{channelID}/tasks
func (h *TaskHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	channelID := chi.URLParam(r, "channelID")
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "channel ID is required")
		return
	}

	filter := service.TaskFilter{
		Status:       r.URL.Query().Get("status"),
		ClaimerID:    r.URL.Query().Get("claimer_id"),
		CreatorID:    r.URL.Query().Get("creator_id"),
		ParentTaskID: r.URL.Query().Get("parent_id"),
	}

	tasks, err := h.svc.ListTasks(r.Context(), channelID, userID, filter)
	if err != nil {
		if err == service.ErrTaskNotChannelMember {
			writeError(w, http.StatusForbidden, "not a channel member")
			return
		}
		slog.Error("failed to list tasks", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list tasks")
		return
	}

	writeJSON(w, http.StatusOK, toTaskResponseList(tasks))
}

// Get handles GET /api/v1/channels/{channelID}/tasks/{taskID}
func (h *TaskHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	channelID := chi.URLParam(r, "channelID")
	taskID := chi.URLParam(r, "taskID")
	if channelID == "" || taskID == "" {
		writeError(w, http.StatusBadRequest, "channel ID and task ID are required")
		return
	}

	task, err := h.svc.GetTask(r.Context(), channelID, taskID, userID)
	if err != nil {
		switch {
		case err == service.ErrTaskNotFound:
			writeError(w, http.StatusNotFound, "task not found")
		case err == service.ErrTaskNotChannelMember:
			writeError(w, http.StatusForbidden, "not a channel member")
		default:
			slog.Error("failed to get task", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to get task")
		}
		return
	}

	writeJSON(w, http.StatusOK, toTaskResponse(task))
}

// Update handles PATCH /api/v1/channels/{channelID}/tasks/{taskID}
func (h *TaskHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	channelID := chi.URLParam(r, "channelID")
	taskID := chi.URLParam(r, "taskID")
	if channelID == "" || taskID == "" {
		writeError(w, http.StatusBadRequest, "channel ID and task ID are required")
		return
	}

	var req UpdateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	svcReq := service.TaskUpdateRequest{
		Title:       req.Title,
		Description: req.Description,
		Status:      req.Status,
		Priority:    req.Priority,
		DueDate:     req.DueDate,
	}

	task, err := h.svc.UpdateTask(r.Context(), channelID, taskID, userID, svcReq)
	if err != nil {
		switch {
		case err == service.ErrTaskNotFound:
			writeError(w, http.StatusNotFound, "task not found")
		case err == service.ErrTaskNotChannelMember:
			writeError(w, http.StatusForbidden, "not a channel member")
		case err == service.ErrTaskInvalidStatus || err == service.ErrTaskInvalidTransition:
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			slog.Error("failed to update task", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to update task")
		}
		return
	}

	resp := toTaskResponse(task)
	writeJSON(w, http.StatusOK, resp)

	var dueDateStr string
	if task.DueDate != nil {
		dueDateStr = task.DueDate.Format(time.RFC3339)
	}
	ws.BroadcastTaskUpdated(h.hub, ws.TaskUpdatedPayload{
		ID:              task.ID,
		TaskNumber:      task.TaskNumber,
		ChannelID:       task.ChannelID,
		Title:           task.Title,
		Description:     task.Description,
		Status:          task.Status,
		ClaimerID:       task.ClaimerID,
		ClaimerName:     task.ClaimerName,
		Priority:        task.Priority,
		DueDate:         dueDateStr,
		MessageID:       task.MessageID,
		UpdatedAt:       task.UpdatedAt.Format(time.RFC3339),
		SubtaskCount:    task.SubtaskCount,
		DoneSubtaskCount: task.DoneSubtaskCount,
	})

	var threadID string
	if task.MessageID != "" {
		tid, err := h.resolveThreadID(r.Context(), task.MessageID)
		if err == nil {
			threadID = tid
		}
	}

	if req.Status != nil && *req.Status != "" {
		statusText := formatStatusDisplay(*req.Status)
		h.broadcastSystemMessage(task.ChannelID, threadID, task.TaskNumber, task.Title, "状态已更新为 "+statusText)
	} else {
		h.broadcastSystemMessage(task.ChannelID, threadID, task.TaskNumber, task.Title, i18n.Active.SysTaskUpdated)
	}

}

// Delete handles DELETE /api/v1/channels/{channelID}/tasks/{taskID}
func (h *TaskHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	channelID := chi.URLParam(r, "channelID")
	taskID := chi.URLParam(r, "taskID")
	if channelID == "" || taskID == "" {
		writeError(w, http.StatusBadRequest, "channel ID and task ID are required")
		return
	}

	task, getErr := h.svc.GetTask(r.Context(), channelID, taskID, userID)
	if getErr != nil {
		switch {
		case getErr == service.ErrTaskNotFound:
			writeError(w, http.StatusNotFound, "task not found")
		case getErr == service.ErrTaskNotChannelMember:
			writeError(w, http.StatusForbidden, "not a channel member")
		default:
			slog.Error("failed to get task for delete", "error", getErr)
			writeError(w, http.StatusInternalServerError, "failed to delete task")
		}
		return
	}

	if err := h.svc.DeleteTask(r.Context(), channelID, taskID, userID); err != nil {
		switch {
		case err == service.ErrTaskNotFound:
			writeError(w, http.StatusNotFound, "task not found")
		case err == service.ErrTaskNotChannelMember:
			writeError(w, http.StatusForbidden, "not a channel member")
		default:
			slog.Error("failed to delete task", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to delete task")
		}
		return
	}

	var threadID string
	if task.MessageID != "" {
		tid, err := h.resolveThreadID(r.Context(), task.MessageID)
		if err == nil {
			threadID = tid
		}
	}

	h.broadcastSystemMessage(channelID, threadID, task.TaskNumber, task.Title, i18n.Active.SysTaskDeleted)

	ws.BroadcastTaskDeleted(h.hub, ws.TaskDeletedPayload{
		ID:         taskID,
		ChannelID:  channelID,
		TaskNumber: task.TaskNumber,
	})

	w.WriteHeader(http.StatusNoContent)
}

// --- Claim / Unclaim handlers ---

// Claim handles POST /api/v1/channels/{channelID}/tasks/{taskID}/claim
func (h *TaskHandler) Claim(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	channelID := chi.URLParam(r, "channelID")
	taskID := chi.URLParam(r, "taskID")
	if channelID == "" || taskID == "" {
		writeError(w, http.StatusBadRequest, "channel ID and task ID are required")
		return
	}

	t, err := h.svc.GetTask(r.Context(), channelID, taskID, userID)
	if err != nil { writeError(w, http.StatusNotFound, "task not found"); return }

	if h.agentSvc != nil {
		allowed, reason := h.agentSvc.CheckClaimWindow(t.ID, userID)
		if !allowed {
			writeError(w, http.StatusConflict, reason)
			return
		}
	}

	task, err := h.svc.ClaimTask(r.Context(), channelID, t.ID, userID)
	if err != nil {
		switch {
		case err == service.ErrTaskNotFound:
			writeError(w, http.StatusNotFound, "task not found")
		case err == service.ErrTaskNotChannelMember:
			writeError(w, http.StatusForbidden, "not a channel member")
		case err == service.ErrTaskAlreadyClaimed:
			writeError(w, http.StatusConflict, "already assigned — do not reply, someone else is working on it")
		case err == service.ErrTaskInTerminalState:
			writeError(w, http.StatusConflict, "task is in a terminal state")
		case err == service.ErrTaskNotClaimable:
			writeError(w, http.StatusConflict, "task status does not allow claiming")
		case err == service.ErrTaskBlocked:
			writeError(w, http.StatusConflict, "task is blocked by incomplete dependencies")
		default:
			slog.Error("failed to claim task", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to claim task")
		}
		return
	}

	if h.agentSvc != nil {
		h.agentSvc.CloseClaimWindow(t.ID)
	}

	resp := toTaskResponse(task)
	writeJSON(w, http.StatusOK, resp)

	var dueDateStr string
	if task.DueDate != nil {
		dueDateStr = task.DueDate.Format(time.RFC3339)
	}
	ws.BroadcastTaskUpdated(h.hub, ws.TaskUpdatedPayload{
		ID:              task.ID,
		TaskNumber:      task.TaskNumber,
		ChannelID:       task.ChannelID,
		Title:           task.Title,
		Description:     task.Description,
		Status:          task.Status,
		ClaimerID:       task.ClaimerID,
		ClaimerName:     task.ClaimerName,
		Priority:        task.Priority,
		DueDate:         dueDateStr,
		MessageID:       task.MessageID,
		UpdatedAt:       task.UpdatedAt.Format(time.RFC3339),
		SubtaskCount:    task.SubtaskCount,
		DoneSubtaskCount: task.DoneSubtaskCount,
	})

	if task.MessageID != "" {
		threadSvc := service.NewThreadService(h.pool)
		threadID, _, tErr := threadSvc.GetOrCreateThread(r.Context(), channelID, task.MessageID)
		if tErr == nil {
			claimMsgID := uuid.New().String()
			claimNow := time.Now()
			claimContent := fmt.Sprintf("📋 @%s claimed #%d %s", task.ClaimerName, task.TaskNumber, task.Title)
			_, _ = h.pool.Exec(r.Context(),
				`INSERT INTO messages (id, channel_id, sender_type, sender_id, content, content_type, thread_id, created_at, updated_at)
				 VALUES ($1, $2, 'system', '00000000-0000-0000-0000-000000000000', $3, 'system', $4, $5, $5)`,
				claimMsgID, channelID, claimContent, threadID, claimNow,
			)
			var replyCount int
			h.pool.QueryRow(r.Context(),
				`SELECT reply_count FROM threads WHERE id = $1`, threadID,
			).Scan(&replyCount)
			threadMsgPayload := ws.ThreadMessageNewPayload{
				Message: ws.ThreadMessageItem{
					ID:          claimMsgID,
					ChannelID:   channelID,
					ThreadID:    threadID,
					SenderType:  "system",
					SenderID:    "system",
					SenderName:  "Solo",
					Content:     claimContent,
					ContentType: "system",
					CreatedAt:   claimNow.UTC().Format(time.RFC3339),
				},
				Thread: ws.ThreadMetadataItem{
					ThreadID:    threadID,
					ReplyCount:  replyCount,
					LastReplyAt: claimNow.UTC().Format(time.RFC3339),
				},
			}
			h.hub.BroadcastToThread(threadID, ws.Envelope(ws.EventThreadMessageNew, threadMsgPayload))

			slog.Debug("persisted claim system message to task thread",
				"task_id", task.ID,
				"thread_id", threadID,
				"message_id", claimMsgID,
			)
		}
	}
}

// Unclaim handles DELETE /api/v1/channels/{channelID}/tasks/{taskID}/claim
func (h *TaskHandler) Unclaim(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	channelID := chi.URLParam(r, "channelID")
	taskID := chi.URLParam(r, "taskID")
	if channelID == "" || taskID == "" {
		writeError(w, http.StatusBadRequest, "channel ID and task ID are required")
		return
	}

	t, err := h.svc.GetTask(r.Context(), channelID, taskID, userID)
	if err != nil { writeError(w, http.StatusNotFound, "task not found"); return }

	task, err := h.svc.UnclaimTask(r.Context(), channelID, t.ID, userID)
	if err != nil {
		switch {
		case err == service.ErrTaskNotFound:
			writeError(w, http.StatusNotFound, "task not found")
		case err == service.ErrTaskNotChannelMember:
			writeError(w, http.StatusForbidden, "not a channel member")
		case err == service.ErrTaskNotClaimer:
			writeError(w, http.StatusForbidden, "you are not the claimer of this task")
		case err == service.ErrTaskInTerminalState:
			writeError(w, http.StatusConflict, "task is in a terminal state")
		default:
			slog.Error("failed to unclaim task", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to unclaim task")
		}
		return
	}

	resp := toTaskResponse(task)
	writeJSON(w, http.StatusOK, resp)

	var dueDateStr string
	if task.DueDate != nil {
		dueDateStr = task.DueDate.Format(time.RFC3339)
	}
	ws.BroadcastTaskUpdated(h.hub, ws.TaskUpdatedPayload{
		ID:          task.ID,
		TaskNumber:  task.TaskNumber,
		ChannelID:   task.ChannelID,
		Title:       task.Title,
		Description: task.Description,
		Status:      task.Status,
		ClaimerID:   "",
		Priority:    task.Priority,
		DueDate:     dueDateStr,
		MessageID:   task.MessageID,
		UpdatedAt:   task.UpdatedAt.Format(time.RFC3339),
	})

	var threadID string
	if task.MessageID != "" {
		tid, err := h.resolveThreadID(r.Context(), task.MessageID)
		if err == nil {
			threadID = tid
		}
	}

	if threadID != "" {
		msgID := uuid.New().String()
		now := time.Now()
		content := fmt.Sprintf("📋 @%s released #%d %s", task.ClaimerName, task.TaskNumber, task.Title)
		_, _ = h.pool.Exec(context.Background(),
			`INSERT INTO messages (id, channel_id, thread_id, sender_type, sender_id, content, content_type, created_at, updated_at)
			 VALUES ($1, $2, $3, 'system', '00000000-0000-0000-0000-000000000000', $4, 'system', $5, $5)`,
			msgID, task.ChannelID, threadID, content, now,
		)
		var replyCount int
		_ = h.pool.QueryRow(context.Background(), `SELECT reply_count FROM threads WHERE id = $1`, threadID).Scan(&replyCount)
		threadMsgPayload := ws.ThreadMessageNewPayload{
			Message: ws.ThreadMessageItem{
				ID: msgID, ChannelID: task.ChannelID, ThreadID: threadID,
				SenderType: "system", SenderID: "system", SenderName: "Solo",
				Content: content, ContentType: "system", CreatedAt: now.UTC().Format(time.RFC3339),
			},
			Thread: ws.ThreadMetadataItem{
				ThreadID: threadID, ReplyCount: replyCount, LastReplyAt: now.UTC().Format(time.RFC3339),
			},
		}
		h.hub.BroadcastToThread(threadID, ws.Envelope(ws.EventThreadMessageNew, threadMsgPayload))
	}
}

// --- asTask — Convert message to task ---

// ConvertToTask handles POST /api/v1/channels/{channelID}/messages/{messageID}/convert-to-task
func (h *TaskHandler) ConvertToTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")
	if channelID == "" || messageID == "" {
		writeError(w, http.StatusBadRequest, "channel ID and message ID are required")
		return
	}

	task, err := h.svc.ConvertMessageToTask(r.Context(), channelID, messageID, userID)
	if err != nil {
		switch {
		case err == service.ErrTaskNotChannelMember:
			writeError(w, http.StatusForbidden, "not a channel member")
		default:
			slog.Error("failed to convert message to task", "error", err, "message_id", messageID)
			if err.Error() == "message not found" {
				writeError(w, http.StatusNotFound, "message not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to convert message to task")
		}
		return
	}

	var threadID string
	threadSvc := service.NewThreadService(h.pool)
	tid, _, tErr := threadSvc.GetOrCreateThread(r.Context(), channelID, messageID)
	if tErr != nil {
		slog.Error("failed to create thread for converted task", "task_id", task.ID, "error", tErr)
	} else {
		threadID = tid
	}

	resp := toTaskResponse(task)
	writeJSON(w, http.StatusCreated, resp)

	if h.agentSvc != nil {
		contentForMentions := task.Title
		if task.Description != "" {
			contentForMentions += " " + task.Description
		}
		var mentionedAgentIDs []string
		if h.mentionSvc != nil {
			ids, _, err := h.mentionSvc.ResolveMentions(r.Context(), contentForMentions, task.ChannelID)
			if err == nil {
				mentionedAgentIDs = ids
			}
		}
		go h.agentSvc.TriggerAllAgentsForTask(context.Background(), task.ChannelID, task.ID, task.TaskNumber, task.Title, mentionedAgentIDs, nil)
	}

	dueDate := ""
	if task.DueDate != nil {
		dueDate = task.DueDate.Format(time.RFC3339)
	}
	ws.BroadcastTaskCreated(h.hub, ws.TaskCreatedPayload{
		ID:          task.ID,
		TaskNumber:  task.TaskNumber,
		ChannelID:   task.ChannelID,
		CreatorID:   task.CreatorID,
		CreatorName: task.CreatorName,
		Title:       task.Title,
		Description: task.Description,
		Status:      task.Status,
		ClaimerID:   task.ClaimerID,
		ClaimerName: task.ClaimerName,
		Priority:    task.Priority,
		DueDate:     dueDate,
		MessageID:   task.MessageID,
		CreatedAt:   resp.CreatedAt,
		UpdatedAt:   resp.UpdatedAt,
	})

	h.broadcastSystemMessageWithID(task.ChannelID, threadID, task.TaskNumber, task.Title, i18n.Active.SysTaskCreatedFromMsg, uuid.New().String(), time.Now(), true)
}

// --- System message helpers ---

func (h *TaskHandler) resolveThreadID(ctx context.Context, messageID string) (string, error) {
	var threadID string
	err := h.pool.QueryRow(ctx,
		`SELECT id FROM threads WHERE root_message_id = $1`,
		messageID,
	).Scan(&threadID)
	return threadID, err
}

func (h *TaskHandler) broadcastSystemMessage(channelID, threadID string, taskNumber int, title, action string) {
	h.broadcastSystemMessageWithID(channelID, threadID, taskNumber, title, action, uuid.New().String(), time.Now(), false)
}

func (h *TaskHandler) broadcastSystemMessageWithID(channelID, threadID string, taskNumber int, title, action, msgID string, ts time.Time, showInChannel bool) {
	content := formatSystemMessage(taskNumber, title, action)

	if showInChannel {
		msg := ws.MessageNewPayload{
			ID:          msgID,
			ChannelID:   channelID,
			SenderType:  "system",
			SenderID:    "system",
			SenderName:  "Solo",
			Content:     content,
			ContentType: "system",
			TaskNumber:  taskNumber,
			TaskStatus:  "",
			CreatedAt:   ts.UTC().Format(time.RFC3339),
		}
		h.hub.BroadcastToChannel(channelID, ws.Envelope(ws.EventMessageNew, msg))
	}

	if threadID != "" {
		var nullableThreadID interface{}
		if !showInChannel {
			nullableThreadID = threadID
		}
		_, dbErr := h.pool.Exec(context.Background(),
			`INSERT INTO messages (id, channel_id, thread_id, sender_type, sender_id, content, content_type, created_at, updated_at)
			 VALUES ($1, $2, $3, 'system', '00000000-0000-0000-0000-000000000000', $4, 'system', $5, $5)
			 ON CONFLICT (id) DO UPDATE SET content = EXCLUDED.content`,
			msgID, channelID, nullableThreadID, content, ts,
		)
		if dbErr != nil {
			slog.Error("failed to persist task system message to thread", "msg_id", msgID, "thread_id", threadID, "error", dbErr)
			return
		}

		var replyCount int
		h.pool.QueryRow(context.Background(),
			`SELECT reply_count FROM threads WHERE id = $1`, threadID,
		).Scan(&replyCount)

		threadMsgPayload := ws.ThreadMessageNewPayload{
			Message: ws.ThreadMessageItem{
				ID:          msgID,
				ChannelID:   channelID,
				ThreadID:    threadID,
				SenderType:  "system",
				SenderID:    "system",
				SenderName:  "Solo",
				Content:     content,
				ContentType: "system",
				CreatedAt:   ts.UTC().Format(time.RFC3339),
			},
			Thread: ws.ThreadMetadataItem{
				ThreadID:    threadID,
				ReplyCount:  replyCount,
				LastReplyAt: ts.UTC().Format(time.RFC3339),
			},
		}
		h.hub.BroadcastToThread(threadID, ws.Envelope(ws.EventThreadMessageNew, threadMsgPayload))

		slog.Debug("task system message synced to thread",
			"msg_id", msgID,
			"thread_id", threadID,
			"task_number", taskNumber,
			"action", action,
		)
	}
}

func formatSystemMessage(taskNumber int, title, action string) string {
	return fmt.Sprintf("📋 Task #%d %s: %s", taskNumber, action, title)
}

func formatStatusDisplay(status string) string {
	switch status {
	case service.TaskStatusTodo:
		return "TODO"
	case service.TaskStatusInProgress:
		return "IN PROGRESS"
	case service.TaskStatusInReview:
		return "IN REVIEW"
	case service.TaskStatusDone:
		return "DONE"
	case service.TaskStatusClosed:
		return "CLOSED"
	default:
		return status
	}
}

// --- Global handlers ---

func (h *TaskHandler) ListAll(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	status := r.URL.Query().Get("status")
	claimerID := r.URL.Query().Get("claimer_id")
	channelID := r.URL.Query().Get("channel_id")
	creatorID := r.URL.Query().Get("creator_id")

	tasks, err := h.svc.ListAllUserTasks(r.Context(), userID, channelID, status, claimerID, creatorID)
	if err != nil {
		slog.Error("failed to list all tasks", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list tasks")
		return
	}

	writeJSON(w, http.StatusOK, toTaskResponseList(tasks))
}

func (h *TaskHandler) CreateGlobal(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "task title is required")
		return
	}
	if len(req.Title) > 500 {
		writeError(w, http.StatusBadRequest, "task title exceeds maximum length of 500 characters")
		return
	}

	now := time.Now()
	msgID := uuid.New().String()
	senderType := "user"
	var isAgent bool
	_ = h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1)`, userID).Scan(&isAgent)
	if isAgent {
		senderType = "agent"
	}
	_, msgErr := h.pool.Exec(r.Context(),
		`INSERT INTO messages (id, channel_id, sender_type, sender_id, content, content_type, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, 'text', $6, $6)`,
		msgID, req.ChannelID, senderType, userID, req.Title, now,
	)
	if msgErr != nil {
		slog.Error("failed to insert task user message", "error", msgErr)
		writeError(w, http.StatusInternalServerError, "failed to create task")
		return
	}

	senderName := userID
	if err := h.pool.QueryRow(r.Context(),
		`SELECT COALESCE(
			(SELECT display_name FROM users WHERE id = $1),
			(SELECT name FROM agents WHERE id = $1),
			$1
		)`, userID,
	).Scan(&senderName); err != nil {
		slog.Warn("failed to resolve sender name for task message",
			"user_id", userID,
			"error", err,
		)
	}

	if h.hub != nil {
		msgPayload := ws.Envelope(ws.EventMessageNew, ws.MessageNewPayload{
			ID: msgID, ChannelID: req.ChannelID,
			SenderType: senderType, SenderID: userID, SenderName: senderName,
			Content: req.Title, ContentType: "text", CreatedAt: now.Format(time.RFC3339),
		})
		h.hub.BroadcastToChannel(req.ChannelID, msgPayload)
	}

	task, err := h.svc.ConvertMessageToTask(r.Context(), req.ChannelID, msgID, userID)
	if err != nil {
		slog.Error("failed to convert message to task", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create task")
		return
	}

	var threadID string
	threadSvc := service.NewThreadService(h.pool)
	tid, _, threadErr := threadSvc.GetOrCreateThread(r.Context(), task.ChannelID, msgID)
	if threadErr != nil {
		slog.Error("failed to create thread for task", "task_id", task.ID, "error", threadErr)
	} else {
		threadID = tid
	}

	resp := toTaskResponse(task)
	writeJSON(w, http.StatusCreated, resp)

	if h.hub != nil {
		msgUpdated := ws.Envelope(ws.EventMessageUpdated, ws.MessageUpdatedPayload{
			ID: msgID, ChannelID: task.ChannelID,
			TaskNumber: task.TaskNumber, TaskStatus: task.Status,
			TaskClaimerName: task.ClaimerName, TaskClaimerDeleted: task.ClaimerDeleted,
		})
		h.hub.BroadcastToChannel(task.ChannelID, msgUpdated)
	}

	dueDate := ""
	if task.DueDate != nil {
		dueDate = task.DueDate.Format(time.RFC3339)
	}
	ws.BroadcastTaskCreated(h.hub, ws.TaskCreatedPayload{
		ID: task.ID, TaskNumber: task.TaskNumber, ChannelID: task.ChannelID,
		CreatorID: task.CreatorID, CreatorName: task.CreatorName,
		Title: task.Title, Description: task.Description,
		Status: task.Status, Priority: task.Priority,
		DueDate: dueDate, MessageID: task.MessageID,
		CreatedAt: resp.CreatedAt, UpdatedAt: resp.UpdatedAt,
	})

	if threadID != "" {
		h.broadcastSystemMessageWithID(task.ChannelID, threadID, task.TaskNumber, task.Title, i18n.Active.SysTaskCreated, uuid.New().String(), now, false)
	}

	if h.agentSvc != nil {
		contentForMentions := task.Title
		if task.Description != "" {
			contentForMentions += " " + task.Description
		}
		var mentionedAgentIDs []string
		if h.mentionSvc != nil {
			ids, _, err := h.mentionSvc.ResolveMentions(r.Context(), contentForMentions, task.ChannelID)
			if err == nil {
				mentionedAgentIDs = ids
			}
		}
		go h.agentSvc.TriggerAllAgentsForTask(context.Background(), task.ChannelID, task.ID, task.TaskNumber, task.Title, mentionedAgentIDs, nil)
	}
}

func (h *TaskHandler) GetGlobal(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "task ID is required")
		return
	}

	task, err := h.svc.GetTaskGlobal(r.Context(), taskID, userID)
	if err != nil {
		if err == service.ErrTaskNotFound {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		slog.Error("failed to get task", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get task")
		return
	}

	writeJSON(w, http.StatusOK, toTaskResponse(task))
}

func (h *TaskHandler) UpdateGlobal(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "task ID is required")
		return
	}

	task, err := h.svc.GetTaskGlobal(r.Context(), taskID, userID)
	if err != nil {
		if err == service.ErrTaskNotFound {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		slog.Error("failed to get task for update", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update task")
		return
	}

	var req UpdateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	svcReq := service.TaskUpdateRequest{
		Title:       req.Title,
		Description: req.Description,
		Status:      req.Status,
		Priority:    req.Priority,
		DueDate:     req.DueDate,
	}

	updated, err := h.svc.UpdateTask(r.Context(), task.ChannelID, taskID, userID, svcReq)
	if err != nil {
		switch {
		case err == service.ErrTaskNotFound:
			writeError(w, http.StatusNotFound, "task not found")
		case err == service.ErrTaskNotChannelMember:
			writeError(w, http.StatusForbidden, "not a channel member")
		case err == service.ErrTaskInvalidStatus || err == service.ErrTaskInvalidTransition:
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			slog.Error("failed to update task", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to update task")
		}
		return
	}

	resp := toTaskResponse(updated)
	writeJSON(w, http.StatusOK, resp)

	var dueDateStr string
	if updated.DueDate != nil {
		dueDateStr = updated.DueDate.Format(time.RFC3339)
	}
	ws.BroadcastTaskUpdated(h.hub, ws.TaskUpdatedPayload{
		ID:              updated.ID,
		TaskNumber:      updated.TaskNumber,
		ChannelID:       updated.ChannelID,
		Title:           updated.Title,
		Description:     updated.Description,
		Status:          updated.Status,
		ClaimerID:       updated.ClaimerID,
		ClaimerName:     updated.ClaimerName,
		Priority:        updated.Priority,
		DueDate:         dueDateStr,
		MessageID:       updated.MessageID,
		UpdatedAt:       updated.UpdatedAt.Format(time.RFC3339),
		SubtaskCount:    updated.SubtaskCount,
		DoneSubtaskCount: updated.DoneSubtaskCount,
	})

	var threadID string
	if updated.MessageID != "" {
		tid, err := h.resolveThreadID(r.Context(), updated.MessageID)
		if err == nil {
			threadID = tid
		}
	}

	if req.Status != nil && *req.Status != "" {
		statusText := formatStatusDisplay(*req.Status)
		h.broadcastSystemMessage(updated.ChannelID, threadID, updated.TaskNumber, updated.Title, "状态已更新为 "+statusText)
	} else {
		h.broadcastSystemMessage(updated.ChannelID, threadID, updated.TaskNumber, updated.Title, i18n.Active.SysTaskUpdated)
	}

}

func (h *TaskHandler) DeleteGlobal(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "task ID is required")
		return
	}

	task, err := h.svc.GetTaskGlobal(r.Context(), taskID, userID)
	if err != nil {
		if err == service.ErrTaskNotFound {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		slog.Error("failed to get task for delete", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete task")
		return
	}

	if err := h.svc.DeleteTask(r.Context(), task.ChannelID, taskID, userID); err != nil {
		switch {
		case err == service.ErrTaskNotFound:
			writeError(w, http.StatusNotFound, "task not found")
		case err == service.ErrTaskNotChannelMember:
			writeError(w, http.StatusForbidden, "not a channel member")
		default:
			slog.Error("failed to delete task", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to delete task")
		}
		return
	}

	ws.BroadcastTaskDeleted(h.hub, ws.TaskDeletedPayload{
		ID:         taskID,
		ChannelID:  task.ChannelID,
		TaskNumber: task.TaskNumber,
	})

	w.WriteHeader(http.StatusNoContent)
}

// --- Swarm handlers ---

func (h *TaskHandler) DecomposeTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "task ID is required")
		return
	}
	if h.swarm == nil {
		writeError(w, http.StatusInternalServerError, "swarm coordinator not available")
		return
	}

	var req struct {
		ChannelID string                    `json:"channel_id"`
		Subtasks  []service.SwarmSubtaskDef `json:"subtasks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ChannelID == "" {
		writeError(w, http.StatusBadRequest, "channel_id is required")
		return
	}
	if len(req.Subtasks) == 0 {
		writeError(w, http.StatusBadRequest, "subtasks array is required")
		return
	}

	created, err := h.swarm.DecomposeTask(r.Context(), taskID, req.ChannelID, userID, req.Subtasks)
	if err != nil {
		slog.Error("failed to decompose task", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"parent_task_id": taskID,
		"subtasks":       toTaskResponseList(created),
	})
}

func (h *TaskHandler) SwarmStatus(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "task ID is required")
		return
	}
	if h.swarm == nil {
		writeError(w, http.StatusInternalServerError, "swarm coordinator not available")
		return
	}

	channelID := r.URL.Query().Get("channel_id")
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "channel_id query parameter is required")
		return
	}

	status, err := h.swarm.GetSwarmStatus(r.Context(), taskID, channelID)
	if err != nil {
		slog.Error("failed to get swarm status", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, status)
}

func (h *TaskHandler) ListSwarmClaimable(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if h.swarm == nil {
		writeError(w, http.StatusInternalServerError, "swarm coordinator not available")
		return
	}

	channelID := r.URL.Query().Get("channel_id")
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "channel_id query parameter is required")
		return
	}
	tasks, err := h.swarm.ListClaimableSwarmTasks(r.Context(), channelID)
	if err != nil {
		slog.Error("failed to list claimable swarm tasks", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list claimable swarm tasks")
		return
	}
	writeJSON(w, http.StatusOK, toTaskResponseList(tasks))
}

func (h *TaskHandler) ListStale(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if h.watchdog == nil {
		writeError(w, http.StatusInternalServerError, "watchdog service not available")
		return
	}

	channelID := r.URL.Query().Get("channel_id")
	stale, err := h.watchdog.ListStaleTasks(r.Context(), channelID)
	if err != nil {
		slog.Error("failed to list stale tasks", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list stale tasks")
		return
	}
	writeJSON(w, http.StatusOK, stale)
}

// IsolateTask handles POST /api/v1/tasks/{id}/isolate (T3.2.5).
func (h *TaskHandler) IsolateTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	_ = userID

	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "task ID is required")
		return
	}

	task, err := h.svc.GetTaskGlobal(r.Context(), taskID, userID)
	if err != nil {
		if err == service.ErrTaskNotFound {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get task")
		return
	}

	var bindPath string
	dbErr := h.pool.QueryRow(r.Context(),
		`SELECT bind_path FROM channel_bindings WHERE channel_id = $1`,
		task.ChannelID,
	).Scan(&bindPath)
	if dbErr != nil {
		writeError(w, http.StatusNotFound, "channel is not bound to a project repository")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"status":         "isolated",
		"task_id":        taskID,
		"channel_id":     task.ChannelID,
		"workspace_path": bindPath,
		"task_number":    task.TaskNumber,
	})
}

// UnisolateTask handles DELETE /api/v1/tasks/{id}/isolate (T3.2.5).
func (h *TaskHandler) UnisolateTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	_ = userID

	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "task ID is required")
		return
	}

	task, err := h.svc.GetTaskGlobal(r.Context(), taskID, userID)
	if err != nil {
		if err == service.ErrTaskNotFound {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get task")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "un-isolated",
		"task_id":     taskID,
		"task_number": task.TaskNumber,
	})
}
