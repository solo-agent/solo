package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Scheduler runs periodic scans for reminders and watchdog timeouts.
type Scheduler struct {
	reminderSvc *ReminderService
	watchdogSvc *WatchdogService
	notifier    *AgentNotifier
}

// NewScheduler creates a new Scheduler.
func NewScheduler(reminderSvc *ReminderService, watchdogSvc *WatchdogService, notifier *AgentNotifier) *Scheduler {
	return &Scheduler{
		reminderSvc: reminderSvc,
		watchdogSvc: watchdogSvc,
		notifier:    notifier,
	}
}

// Start runs the periodic scan loop every 30 seconds.
func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	slog.Info("scheduler started (reminders, watchdog)")

	for {
		select {
		case <-ticker.C:
			s.tickReminders(ctx)
			s.tickWatchdog(ctx)
		case <-ctx.Done():
			slog.Info("scheduler stopped")
			return
		}
	}
}

func (s *Scheduler) tickReminders(ctx context.Context) {
	reminders, err := s.reminderSvc.CheckDueReminders(ctx)
	if err != nil {
		slog.Warn("scheduler: failed to check reminders", "error", err)
		return
	}
	for _, r := range reminders {
		content := fmt.Sprintf("⏰ Reminder: %s", r.Message)
		if err := s.notifier.Notify(ctx, r.AgentID, content); err != nil {
			slog.Warn("scheduler: failed to notify reminder", "reminder_id", r.ID, "agent_id", r.AgentID, "error", err)
			continue
		}
		if r.IsRecurring {
			nextTime, err := ParseCronExpression(r.RecurringRule)
			if err != nil {
				slog.Warn("scheduler: failed to parse cron, marking fired", "reminder_id", r.ID, "error", err)
				s.reminderSvc.MarkFired(ctx, r.ID)
				continue
			}
			s.reminderSvc.RescheduleRecurring(ctx, r.ID, nextTime)
		} else {
			s.reminderSvc.MarkFired(ctx, r.ID)
		}
	}
}

func (s *Scheduler) tickWatchdog(ctx context.Context) {
	if err := s.watchdogSvc.ScanTimeouts(ctx); err != nil {
		slog.Warn("scheduler: watchdog scan failed", "error", err)
	}
}
