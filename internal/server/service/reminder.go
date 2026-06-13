package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/realtime"
)

// Reminder represents a scheduled reminder.
type Reminder struct {
	ID            string     `json:"id"`
	AgentID       string     `json:"agent_id"`
	ChannelID     *string    `json:"channel_id,omitempty"`
	TaskID        *string    `json:"task_id,omitempty"`
	ReminderType  string     `json:"reminder_type"`
	RemindAt      time.Time  `json:"remind_at"`
	Message       string     `json:"message"`
	IsRecurring   bool       `json:"is_recurring"`
	RecurringRule string     `json:"recurring_rule,omitempty"`
	IsFired       bool       `json:"is_fired"`
	FiredAt       *time.Time `json:"fired_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// CreateReminderRequest is the payload for creating a reminder.
type CreateReminderRequest struct {
	AgentID       string `json:"agent_id"`
	ChannelID     string `json:"channel_id,omitempty"`
	TaskID        string `json:"task_id,omitempty"`
	ReminderType  string `json:"reminder_type"`
	RemindAt      string `json:"remind_at"` // RFC3339
	Message       string `json:"message"`
	IsRecurring   bool   `json:"is_recurring,omitempty"`
	RecurringRule string `json:"recurring_rule,omitempty"`
}

// ReminderService handles CRUD and delivery of reminders.
type ReminderService struct {
	pool *pgxpool.Pool
	hub  realtime.Broadcaster
}

// NewReminderService creates a new ReminderService.
func NewReminderService(pool *pgxpool.Pool, hub realtime.Broadcaster) *ReminderService {
	return &ReminderService{pool: pool, hub: hub}
}

// Create inserts a new reminder.
func (s *ReminderService) Create(ctx context.Context, req CreateReminderRequest) (*Reminder, error) {
	if req.AgentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	if req.Message == "" {
		return nil, fmt.Errorf("message is required")
	}
	if req.RemindAt == "" {
		return nil, fmt.Errorf("remind_at is required")
	}
	if req.ReminderType == "" {
		req.ReminderType = "custom"
	}

	remindAt, err := time.Parse(time.RFC3339, req.RemindAt)
	if err != nil {
		return nil, fmt.Errorf("invalid remind_at: %w", err)
	}

	id := uuid.New().String()
	now := time.Now()

	var channelID, taskID interface{}
	if req.ChannelID != "" {
		channelID = req.ChannelID
	}
	if req.TaskID != "" {
		taskID = req.TaskID
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO reminders (id, agent_id, channel_id, task_id, reminder_type, remind_at, message, is_recurring, recurring_rule, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		id, req.AgentID, channelID, taskID, req.ReminderType, remindAt, req.Message,
		req.IsRecurring, nullableStr(req.RecurringRule), now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert reminder: %w", err)
	}

	r := &Reminder{
		ID:            id,
		AgentID:       req.AgentID,
		ChannelID:     nilStr(req.ChannelID),
		TaskID:        nilStr(req.TaskID),
		ReminderType:  req.ReminderType,
		RemindAt:      remindAt,
		Message:       req.Message,
		IsRecurring:   req.IsRecurring,
		RecurringRule: req.RecurringRule,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	slog.Info("reminder created", "id", id, "agent_id", req.AgentID, "remind_at", remindAt)
	return r, nil
}

// List returns reminders, optionally filtered by agent_id and status.
func (s *ReminderService) List(ctx context.Context, agentID string, includeFired bool) ([]Reminder, error) {
	query := `SELECT id, agent_id, channel_id::text, task_id::text, reminder_type, remind_at, message,
	                  is_recurring, COALESCE(recurring_rule, ''), is_fired, fired_at, created_at, updated_at
	           FROM reminders WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if agentID != "" {
		query += fmt.Sprintf(" AND agent_id = $%d", argIdx)
		args = append(args, agentID)
		argIdx++
	}
	if !includeFired {
		query += " AND is_fired = false"
	}

	query += " ORDER BY remind_at ASC LIMIT 100"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list reminders: %w", err)
	}
	defer rows.Close()

	var reminders []Reminder
	for rows.Next() {
		var r Reminder
		var channelID, taskID *string
		if err := rows.Scan(&r.ID, &r.AgentID, &channelID, &taskID, &r.ReminderType,
			&r.RemindAt, &r.Message, &r.IsRecurring, &r.RecurringRule,
			&r.IsFired, &r.FiredAt, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan reminder: %w", err)
		}
		r.ChannelID = channelID
		r.TaskID = taskID
		reminders = append(reminders, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if reminders == nil {
		reminders = []Reminder{}
	}
	return reminders, nil
}

// Delete removes a reminder by ID.
func (s *ReminderService) Delete(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM reminders WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete reminder: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("reminder not found")
	}
	slog.Info("reminder deleted", "id", id)
	return nil
}

// CheckDueReminders queries reminders that are due and not yet fired.
// Called by the daemon ticker.
func (s *ReminderService) CheckDueReminders(ctx context.Context) ([]Reminder, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, agent_id, channel_id::text, task_id::text, reminder_type, remind_at, message,
		        is_recurring, COALESCE(recurring_rule, ''), is_fired, fired_at, created_at, updated_at
		 FROM reminders
		 WHERE remind_at <= now() AND is_fired = false
		 LIMIT 50`,
	)
	if err != nil {
		return nil, fmt.Errorf("check due reminders: %w", err)
	}
	defer rows.Close()

	var reminders []Reminder
	for rows.Next() {
		var r Reminder
		var channelID, taskID *string
		if err := rows.Scan(&r.ID, &r.AgentID, &channelID, &taskID, &r.ReminderType,
			&r.RemindAt, &r.Message, &r.IsRecurring, &r.RecurringRule,
			&r.IsFired, &r.FiredAt, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan due reminder: %w", err)
		}
		r.ChannelID = channelID
		r.TaskID = taskID
		reminders = append(reminders, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return reminders, nil
}

// MarkFired marks a non-recurring reminder as fired.
func (s *ReminderService) MarkFired(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE reminders SET is_fired = true, fired_at = now(), updated_at = now() WHERE id = $1`, id)
	return err
}

// RescheduleRecurring updates the remind_at for a recurring reminder.
func (s *ReminderService) RescheduleRecurring(ctx context.Context, id string, nextTime time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE reminders SET remind_at = $1, updated_at = now() WHERE id = $2`, nextTime, id)
	return err
}

// SendReminderMessage sends a reminder notification to the target agent.
// If a task is linked, the message is sent to the task's channel. Otherwise
// it uses the reminder's own channel.
func (s *ReminderService) SendReminderMessage(ctx context.Context, r Reminder) {
	if r.TaskID != nil && *r.TaskID != "" {
		// Look up the task's channel.
		var taskChannelID string
		err := s.pool.QueryRow(ctx,
			`SELECT channel_id FROM tasks WHERE id = $1`, *r.TaskID,
		).Scan(&taskChannelID)
		if err != nil {
			slog.Warn("reminder: failed to find task channel", "task_id", *r.TaskID, "error", err)
		} else if taskChannelID != "" && s.hub != nil {
			s.hub.BroadcastToChannel(taskChannelID, jsonEnvelope("reminder_fired", map[string]interface{}{
				"reminder_id": r.ID,
				"agent_id":    r.AgentID,
				"task_id":     *r.TaskID,
				"message":     r.Message,
			}))
		}
	} else if r.ChannelID != nil && *r.ChannelID != "" && s.hub != nil {
		s.hub.BroadcastToChannel(*r.ChannelID, jsonEnvelope("reminder_fired", map[string]interface{}{
			"reminder_id": r.ID,
			"agent_id":    r.AgentID,
			"message":     r.Message,
		}))
	}
}

func jsonEnvelope(event string, payload interface{}) []byte {
	b, _ := json.Marshal(map[string]interface{}{
		"type":    event,
		"payload": payload,
	})
	return b
}

func nilStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
