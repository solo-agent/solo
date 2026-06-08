// ============================================================================
// TaskForm — create/edit task form with brutalist styling
// - Title (required), description, priority, due date, assignee
// - Uses input-brutal, textarea, select-brutal styling
// - Optional channel_id for channel-scoped creation
// ============================================================================

'use client';

import { useState, useCallback } from 'react';
import { Calendar, User, AlignLeft } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Label } from '@/components/ui/label';
import { Select } from '@/components/ui/select';
import type { CreateTaskInput, TaskPriority } from '@/lib/types';
import { t } from '@/lib/i18n';

// ---- Constants ----

const PRIORITY_OPTIONS: { value: TaskPriority; label: string }[] = [
  { value: 'urgent', label: t('priorityUrgent') },
  { value: 'high', label: t('priorityHigh') },
  { value: 'normal', label: t('priorityNormal') },
  { value: 'low', label: t('priorityLow') },
];

// ---- Props ----

interface TaskFormProps {
  /** Pre-selected channel ID (for channel-scoped creation) */
  channelId?: string;
  /** Available users/agents for assignee dropdown */
  assigneeOptions?: { id: string; name: string; type: 'user' | 'agent' }[];
  /** Submit handler */
  onSubmit: (input: CreateTaskInput) => Promise<void>;
  /** Loading state */
  isSubmitting: boolean;
  /** Button label */
  submitLabel?: string;
  /** Error message from parent */
  error?: string | null;
}

// ---- Component ----

export function TaskForm({
  channelId,
  assigneeOptions,
  onSubmit,
  isSubmitting,
  submitLabel = t('createTask'),
  error,
}: TaskFormProps) {
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [priority, setPriority] = useState<TaskPriority>('normal');
  const [dueDate, setDueDate] = useState('');
  const [assigneeType, setAssigneeType] = useState<'user' | 'agent'>('user');
  const [assigneeId, setAssigneeId] = useState('');
  const [validationError, setValidationError] = useState<string | null>(null);

  const filteredOptions = assigneeOptions?.filter((a) => a.type === assigneeType) ?? [];

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      setValidationError(null);

      if (!title.trim()) {
        setValidationError(t('taskTitleRequired'));
        return;
      }

      try {
        await onSubmit({
          channel_id: channelId || '',
          title: title.trim(),
          description: description.trim() || undefined,
          priority,
          assignee_id: assigneeId || undefined,
          assignee_type: assigneeId ? assigneeType : undefined,
          due_date: dueDate || undefined,
        });
      } catch {
        // Error handled by parent
      }
    },
    [title, description, priority, dueDate, assigneeId, assigneeType, channelId, onSubmit],
  );

  return (
    <form onSubmit={handleSubmit} className="space-y-5">
      {/* Title */}
      <div>
        <Label htmlFor="task-title" className="mb-1.5 block">
          {t('taskTitle')} <span className="text-brutal-danger">*</span>
        </Label>
        <Input
          id="task-title"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder={t('taskTitlePlaceholder')}
          aria-required="true"
          disabled={isSubmitting}
        />
      </div>

      {/* Description */}
      <div>
        <Label htmlFor="task-description" className="mb-1.5 block">
          <AlignLeft className="mr-1 inline h-3.5 w-3.5" />
          {t('taskDesc')}
        </Label>
        <Textarea
          id="task-description"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder={t('taskDescPlaceholder')}
          rows={4}
          disabled={isSubmitting}
        />
      </div>

      {/* Priority */}
      <div>
        <Label htmlFor="task-priority" className="mb-1.5 block">
          {t('taskPriority')}
        </Label>
        <Select
          id="task-priority"
          value={priority}
          onChange={(v) => setPriority(v as TaskPriority)}
          options={PRIORITY_OPTIONS}
          size="md"
          disabled={isSubmitting}
        />
      </div>

      {/* Assignee */}
      {assigneeOptions && assigneeOptions.length > 0 && (
        <div>
          <Label className="mb-1.5 block">
            <User className="mr-1 inline h-3.5 w-3.5" />
            {t('taskAssignee')}
          </Label>
          <div className="flex gap-2">
            <Select
              value={assigneeType}
              onChange={(v) => {
                setAssigneeType(v as 'user' | 'agent');
                setAssigneeId('');
              }}
              options={[
                { value: 'user', label: t('user') },
                { value: 'agent', label: 'Agent' },
              ]}
              size="md"
              className="w-24 flex-shrink-0"
              disabled={isSubmitting}
            />
            <Select
              value={assigneeId}
              onChange={(v) => setAssigneeId(v)}
              options={[
                { value: '', label: t('unassigned') },
                ...filteredOptions.map((opt) => ({ value: opt.id, label: opt.name })),
              ]}
              size="md"
              className="flex-1"
              disabled={isSubmitting || filteredOptions.length === 0}
            />
          </div>
        </div>
      )}

      {/* Due date */}
      <div>
        <Label htmlFor="task-due-date" className="mb-1.5 block">
          <Calendar className="mr-1 inline h-3.5 w-3.5" />
          {t('taskDueDate')}
        </Label>
        <Input
          id="task-due-date"
          type="date"
          value={dueDate}
          onChange={(e) => setDueDate(e.target.value)}
          disabled={isSubmitting}
        />
      </div>

      {/* Error display */}
      {(validationError || error) && (
        <div className="border-2 border-brutal-danger bg-brutal-danger-light p-3">
          <p className="text-sm font-bold text-brutal-danger">
            {validationError || error}
          </p>
        </div>
      )}

      {/* Submit */}
      <div className="flex justify-end pt-2">
        <button
          type="submit"
          disabled={isSubmitting}
          className="btn-brutal btn-brutal-primary px-6"
        >
          {isSubmitting ? t('submitting') : submitLabel}
        </button>
      </div>
    </form>
  );
}
