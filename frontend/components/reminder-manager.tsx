// ============================================================================
// ReminderManager — manage reminders for agents/channels
// - List reminders with status (active/fired/recurring)
// - Create/Edit/Delete reminder form
// - Support for once, recurring, and deadline types
// ============================================================================

'use client';

import { useState, useEffect, useCallback } from 'react';
import {
  Bell, Plus, Edit, Trash2, Loader2, Clock, Repeat,
  Calendar, User, Hash, Play, Pause,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select, type SelectOption } from '@/components/ui/select';
import { Dialog, DialogHeader, DialogTitle, DialogCloseButton, DialogFooter } from '@/components/ui/dialog';
import { BrutalAlert } from '@/components/ui/brutal-alert';
import { useToast } from '@/components/ui/toast';
import { apiClient } from '@/lib/api-client';
import type { Reminder, CreateReminderInput, ReminderType } from '@/lib/types';

interface ReminderManagerProps {
  /** Filter by agent */
  agentId?: string;
  /** Filter by channel */
  channelId?: string;
  /** Available agents for selector */
  agents?: SelectOption[];
  /** Available channels for selector */
  channels?: SelectOption[];
}

function getTypeOptions(): SelectOption[] {
  return [
    { value: 'custom', label: t('reminderTypeCustom') },
    { value: 'periodic_checkin', label: t('reminderTypePeriodicCheckin') },
    { value: 'task_deadline', label: t('reminderTypeTaskDeadline') },
    { value: 'stale_task', label: t('reminderTypeStaleTask') },
  ];
}

const TYPE_ICON: Record<ReminderType, React.ReactNode> = {
  custom: <Bell className="h-3 w-3" />,
  periodic_checkin: <Repeat className="h-3 w-3" />,
  task_deadline: <Calendar className="h-3 w-3" />,
  stale_task: <Clock className="h-3 w-3" />,
};

export function ReminderManager({ agentId, channelId, agents = [], channels = [] }: ReminderManagerProps) {
  const [reminders, setReminders] = useState<Reminder[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [editingReminder, setEditingReminder] = useState<Reminder | null>(null);
  const [isDeleteOpen, setIsDeleteOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<Reminder | null>(null);

  // Form state
  const [formTitle, setFormTitle] = useState('');
  const [formMessage, setFormMessage] = useState('');
  const [formType, setFormType] = useState<ReminderType>('custom');
  const [formAgentId, setFormAgentId] = useState(agentId || '');
  const [formChannelId, setFormChannelId] = useState(channelId || '');
  const [formIsRecurring, setFormIsRecurring] = useState(false);
  const [formCronRule, setFormCronRule] = useState('');
  const [formTriggerAt, setFormTriggerAt] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const { showToast } = useToast();

  const fetchReminders = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const params: Record<string, string> = {};
      if (agentId) params.agent_id = agentId;
      if (channelId) params.channel_id = channelId;
      const data = await apiClient.get<Reminder[]>('/api/v1/reminders', params);
      setReminders(data || []);
    } catch {
      setError(t('reminderLoadError'));
    } finally {
      setIsLoading(false);
    }
  }, [agentId, channelId]);

  useEffect(() => {
    fetchReminders();
  }, [fetchReminders]);

  const resetForm = () => {
    setFormTitle('');
    setFormMessage('');
    setFormType('custom');
    setFormAgentId(agentId || '');
    setFormChannelId(channelId || '');
    setFormIsRecurring(false);
    setFormCronRule('');
    setFormTriggerAt('');
    setEditingReminder(null);
  };

  const openCreate = () => {
    resetForm();
    setIsCreateOpen(true);
  };

  const openEdit = (r: Reminder) => {
    setEditingReminder(r);
    setFormTitle(r.message?.split('\n')[0] || '');
    setFormMessage(r.message || '');
    setFormType(r.reminder_type);
    setFormAgentId(r.agent_id || '');
    setFormChannelId(r.channel_id || '');
    setFormIsRecurring(r.is_recurring);
    setFormCronRule(r.recurring_rule || '');
    setFormTriggerAt(r.remind_at ? r.remind_at.slice(0, 16) : '');
    setIsCreateOpen(true);
  };

  const handleSubmit = async () => {
    if (!formMessage.trim() || !formAgentId) return;
    setIsSubmitting(true);
    try {
      const body: CreateReminderInput = {
        agent_id: formAgentId,
        channel_id: formChannelId || undefined,
        reminder_type: formType,
        message: formMessage.trim(),
        is_recurring: formIsRecurring,
        recurring_rule: formIsRecurring ? formCronRule || undefined : undefined,
        remind_at: !formIsRecurring && formTriggerAt
          ? new Date(formTriggerAt).toISOString()
          : undefined,
      };

      if (editingReminder) {
        // PATCH update
        await apiClient.patch(`/api/v1/reminders/${editingReminder.id}`, body);
        showToast(t('save'), 'success');
      } else {
        await apiClient.post('/api/v1/reminders', body);
        showToast(t('reminderCreate'), 'success');
      }

      setIsCreateOpen(false);
      resetForm();
      fetchReminders();
    } catch {
      showToast(t('reminderCreateError'), 'error');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setIsSubmitting(true);
    try {
      await apiClient.delete(`/api/v1/reminders/${deleteTarget.id}`);
      showToast(t('delete'), 'success');
      setIsDeleteOpen(false);
      setDeleteTarget(null);
      fetchReminders();
    } catch {
      showToast(t('reminderDeleteError'), 'error');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleTogglePause = async (r: Reminder) => {
    try {
      await apiClient.patch(`/api/v1/reminders/${r.id}`, {
        is_fired: !r.is_fired,
      });
      fetchReminders();
      showToast(r.is_fired ? t('reminderResume') : t('reminderPause'), 'info');
    } catch {
      showToast(t('reminderCreateError'), 'error');
    }
  };

  const formatDate = (iso?: string) => {
    if (!iso) return t('never');
    try {
      return new Date(iso).toLocaleString();
    } catch {
      return iso;
    }
  };

  return (
    <div className="space-y-3">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h3 className="font-heading text-sm font-bold uppercase tracking-wider text-foreground">
          <Bell className="inline h-4 w-4 mr-1.5 -mt-0.5" />
          {t('reminderManagerTitle')}
        </h3>
        <Button variant="primary" size="sm" onClick={openCreate} className="text-xs">
          <Plus className="h-3 w-3 mr-1" />
          {t('reminderCreate')}
        </Button>
      </div>

      {/* Loading */}
      {isLoading && (
        <div className="flex items-center justify-center py-8">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      )}

      {/* Error */}
      {error && !isLoading && (
        <BrutalAlert variant="error" className="text-xs">
          {error}
          <button type="button" onClick={fetchReminders} className="ml-2 underline font-bold">
            {t('retry')}
          </button>
        </BrutalAlert>
      )}

      {/* Empty */}
      {!isLoading && !error && reminders.length === 0 && (
        <div className="border-2 border-dashed border-black bg-brutal-cream p-6 text-center">
          <Bell className="mx-auto h-6 w-6 text-muted-foreground mb-2" />
          <p className="font-heading text-sm font-bold text-foreground">
            {t('reminderNoReminders')}
          </p>
          <p className="mt-1 font-mono text-xs text-muted-foreground">
            {t('reminderNoRemindersDesc')}
          </p>
        </div>
      )}

      {/* List */}
      {!isLoading && reminders.length > 0 && (
        <div className="space-y-2">
          {reminders.map((r) => (
            <div
              key={r.id}
              className={cn(
                'border-2 border-black bg-white p-3 shadow-brutal-sm',
                r.is_fired && 'opacity-60',
              )}
            >
              <div className="flex items-start justify-between gap-2">
                <div className="flex-1 min-w-0">
                  {/* Type icon + message */}
                  <div className="flex items-center gap-1.5">
                    <span className={cn(
                      'inline-flex items-center gap-0.5 px-1 py-0.5',
                      'font-heading text-[10px] font-bold',
                      'border border-black',
                      r.is_recurring
                        ? 'bg-brutal-violet-light text-black'
                        : 'bg-brutal-primary-light text-black',
                    )}>
                      {TYPE_ICON[r.reminder_type] || TYPE_ICON.custom}
                      {r.is_recurring ? t('reminderTypeRecurring') : t('reminderTypeOnce')}
                    </span>
                    <span className={cn(
                      'font-heading text-[10px] font-bold px-1 py-0.5 border border-black',
                      r.is_fired ? 'bg-brutal-muted text-black' : 'bg-brutal-success-light text-black',
                    )}>
                      {r.is_fired ? t('reminderFired') : t('reminderActive')}
                    </span>
                  </div>

                  <p className="mt-1 font-body text-sm text-foreground line-clamp-2">
                    {r.message}
                  </p>

                  {/* Meta row */}
                  <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-[10px] text-muted-foreground">
                    {r.agent_name && (
                      <span className="flex items-center gap-0.5">
                        <User className="h-2.5 w-2.5" />
                        {r.agent_name}
                      </span>
                    )}
                    {r.channel_name && (
                      <span className="flex items-center gap-0.5 font-mono">
                        <Hash className="h-2.5 w-2.5" />
                        {r.channel_name}
                      </span>
                    )}
                    {r.is_recurring && r.recurring_rule && (
                      <span className="flex items-center gap-0.5 font-mono">
                        <Repeat className="h-2.5 w-2.5" />
                        {r.recurring_rule}
                      </span>
                    )}
                    {!r.is_recurring && (
                      <span className="flex items-center gap-0.5">
                        <Calendar className="h-2.5 w-2.5" />
                        {formatDate(r.remind_at)}
                      </span>
                    )}
                  </div>

                  {/* Last triggered */}
                  {r.is_recurring && r.fired_at && (
                    <p className="mt-0.5 font-mono text-[10px] text-muted-foreground">
                      {t('reminderLastTriggered')}: {formatDate(r.fired_at)}
                    </p>
                  )}
                </div>

                {/* Action buttons */}
                <div className="flex items-center gap-1 flex-shrink-0">
                  {r.is_recurring && (
                    <button
                      type="button"
                      onClick={() => handleTogglePause(r)}
                      className="flex h-6 w-6 items-center justify-center border border-black bg-white hover:bg-brutal-primary-light transition-colors"
                      title={r.is_fired ? t('reminderResume') : t('reminderPause')}
                    >
                      {r.is_fired ? (
                        <Play className="h-3 w-3" />
                      ) : (
                        <Pause className="h-3 w-3" />
                      )}
                    </button>
                  )}
                  <button
                    type="button"
                    onClick={() => openEdit(r)}
                    className="flex h-6 w-6 items-center justify-center border border-black bg-white hover:bg-brutal-primary-light transition-colors"
                    title={t('edit')}
                  >
                    <Edit className="h-3 w-3" />
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setDeleteTarget(r);
                      setIsDeleteOpen(true);
                    }}
                    className="flex h-6 w-6 items-center justify-center border border-black bg-white hover:bg-brutal-danger-light transition-colors"
                    title={t('delete')}
                  >
                    <Trash2 className="h-3 w-3" />
                  </button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Create / Edit dialog */}
      <Dialog open={isCreateOpen} onOpenChange={setIsCreateOpen} width="md">
        <DialogHeader>
          <DialogTitle>
            <Bell className="inline h-4 w-4 mr-1.5 -mt-0.5" />
            {editingReminder ? t('reminderEdit') : t('reminderCreate')}
          </DialogTitle>
          <DialogCloseButton onClick={() => setIsCreateOpen(false)} />
        </DialogHeader>

        <div className="space-y-4">
          {/* Type */}
          <div>
            <label className="block font-heading text-sm font-bold mb-1.5">
              {t('reminderType')}
            </label>
            <Select
              options={getTypeOptions()}
              value={formType}
              onChange={(v) => setFormType(v as ReminderType)}
              size="md"
              className="w-full"
            />
          </div>

          {/* Agent */}
          {agents.length > 0 && (
            <div>
              <label className="block font-heading text-sm font-bold mb-1.5">
                {t('reminderAgentLabel')} *
              </label>
              <Select
                options={agents}
                value={formAgentId}
                onChange={setFormAgentId}
                placeholder={t('reminderAgentLabel')}
                size="md"
                className="w-full"
              />
            </div>
          )}

          {/* Channel */}
          {channels.length > 0 && (
            <div>
              <label className="block font-heading text-sm font-bold mb-1.5">
                {t('reminderChannelLabel')}
              </label>
              <Select
                options={[{ value: '', label: t('none') }, ...channels]}
                value={formChannelId}
                onChange={setFormChannelId}
                size="md"
                className="w-full"
              />
            </div>
          )}

          {/* Message */}
          <div>
            <label className="block font-heading text-sm font-bold mb-1.5">
              {t('reminderMessage')} *
            </label>
            <Input
              value={formMessage}
              onChange={(e) => setFormMessage(e.target.value)}
              placeholder={t('reminderMessagePlaceholder')}
            />
          </div>

          {/* Recurring toggle */}
          <div className="flex items-center gap-2">
            <label className="font-heading text-sm font-bold">
              {t('reminderTypeRecurring')}
            </label>
            <button
              type="button"
              onClick={() => setFormIsRecurring(!formIsRecurring)}
              className={cn(
                'relative inline-flex h-6 w-11 items-center border-2 border-black transition-colors',
                formIsRecurring ? 'bg-brutal-success' : 'bg-brutal-muted',
              )}
            >
              <span
                className={cn(
                  'inline-block h-4 w-4 border-2 border-black bg-white transition-transform',
                  formIsRecurring ? 'translate-x-5' : 'translate-x-0.5',
                )}
              />
            </button>
          </div>

          {/* Cron or trigger time */}
          {formIsRecurring ? (
            <div>
              <label className="block font-heading text-sm font-bold mb-1.5">
                {t('reminderCronLabel')}
              </label>
              <Input
                value={formCronRule}
                onChange={(e) => setFormCronRule(e.target.value)}
                placeholder={t('reminderCronPlaceholder')}
                className="font-mono"
              />
            </div>
          ) : (
            <div>
              <label className="block font-heading text-sm font-bold mb-1.5">
                {t('reminderAt')}
              </label>
              <Input
                type="datetime-local"
                value={formTriggerAt}
                onChange={(e) => setFormTriggerAt(e.target.value)}
              />
            </div>
          )}
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setIsCreateOpen(false)}
          >
            {t('cancel')}
          </Button>
          <Button
            variant="primary"
            size="sm"
            onClick={handleSubmit}
            disabled={!formMessage.trim() || isSubmitting}
          >
            {isSubmitting ? (
              <>
                <Loader2 className="h-3 w-3 mr-1 animate-spin" />
                {t('submitting')}
              </>
            ) : editingReminder ? t('save') : t('create')}
          </Button>
        </DialogFooter>
      </Dialog>

      {/* Delete confirmation */}
      <Dialog open={isDeleteOpen} onOpenChange={setIsDeleteOpen}>
        <DialogHeader>
          <DialogTitle>{t('delete')}</DialogTitle>
          <DialogCloseButton onClick={() => setIsDeleteOpen(false)} />
        </DialogHeader>
        <p className="text-sm text-muted-foreground">
          {t('reminderDeleteConfirm')}
        </p>
        <DialogFooter>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setIsDeleteOpen(false)}
          >
            {t('cancel')}
          </Button>
          <Button
            variant="danger"
            size="sm"
            onClick={handleDelete}
            disabled={isSubmitting}
          >
            {isSubmitting ? t('deleting') : t('delete')}
          </Button>
        </DialogFooter>
      </Dialog>
    </div>
  );
}
