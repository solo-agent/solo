'use client';

import { useMemo, useState } from 'react';
import { ArrowLeft, Bot, CalendarClock, History, Loader2, Pause, Pencil, Play, Plus, Trash2 } from 'lucide-react';
import { ApiError } from '@/lib/api-client';
import { getLocale, t } from '@/lib/i18n';
import { useAutomations } from '@/lib/hooks/use-automations';
import type { Automation, AutomationInput, AutomationRun, AutomationRunStatus, AutomationScheduleType, ChannelMember } from '@/lib/types';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select } from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import { useToast } from '@/components/ui/toast';

interface AutomationWorkspaceProps {
  channelId: string;
  agents: ChannelMember[];
  onTaskCreated: () => void;
}

const WEEKDAYS = [
  'automationSunday', 'automationMonday', 'automationTuesday', 'automationWednesday',
  'automationThursday', 'automationFriday', 'automationSaturday',
] as const;

const HOUR_OPTIONS = Array.from({ length: 24 }, (_, value) => ({
  value: String(value), label: String(value).padStart(2, '0'),
}));

const MINUTE_OPTIONS = Array.from({ length: 60 }, (_, value) => ({
  value: String(value), label: String(value).padStart(2, '0'),
}));

function browserTimezone() {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Shanghai';
  } catch {
    return 'Asia/Shanghai';
  }
}

function emptyInput(): AutomationInput {
  return {
    name: '', task_title: '', task_description: '', target_agent_id: '',
    schedule_type: 'daily', schedule_hour: 9, schedule_minute: 0,
    timezone: browserTimezone(), enabled: true,
  };
}

function toInput(item: Automation): AutomationInput {
  return {
    name: item.name,
    task_title: item.task_title,
    task_description: item.task_description,
    target_agent_id: item.target_agent_id ?? '',
    schedule_type: item.schedule_type,
    schedule_hour: item.schedule_hour,
    schedule_minute: item.schedule_minute,
    schedule_weekday: item.schedule_weekday,
    timezone: item.timezone,
    enabled: item.enabled,
  };
}

function formatDate(value?: string) {
  if (!value) return t('never');
  return new Intl.DateTimeFormat(getLocale(), {
    year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
  }).format(new Date(value));
}

function runStatusLabel(status?: AutomationRunStatus) {
  if (!status) return t('never');
  return t({
    running: 'automationStatusRunning', completed: 'automationStatusCompleted',
    skipped: 'automationStatusSkipped', failed: 'automationStatusFailed',
  }[status] as 'automationStatusRunning');
}

function runReason(reason?: string) {
  if (!reason) return '';
  const key = {
    already_running: 'automationReasonAlreadyRunning',
    target_unavailable: 'automationReasonTargetUnavailable',
    dispatch_failed: 'automationReasonDispatchFailed',
    task_returned_to_human: 'automationReasonTaskReturned',
    task_missing: 'automationReasonTaskMissing',
  }[reason];
  return key ? t(key as 'automationReasonAlreadyRunning') : reason;
}

function scheduleLabel(item: Automation) {
  const frequency = t({
    daily: 'automationDaily', weekdays: 'automationWeekdays', weekly: 'automationWeekly',
  }[item.schedule_type] as 'automationDaily');
  const time = `${String(item.schedule_hour).padStart(2, '0')}:${String(item.schedule_minute).padStart(2, '0')}`;
  if (item.schedule_type === 'weekly' && item.schedule_weekday !== undefined) {
    return `${frequency} · ${t(WEEKDAYS[item.schedule_weekday])} · ${time} · ${item.timezone}`;
  }
  return `${frequency} · ${time} · ${item.timezone}`;
}

export function AutomationWorkspace({ channelId, agents, onTaskCreated }: AutomationWorkspaceProps) {
  const { showToast } = useToast();
  const automation = useAutomations(channelId, true);
  const [editing, setEditing] = useState<Automation | 'new' | null>(null);
  const [form, setForm] = useState<AutomationInput>(emptyInput);
  const [saving, setSaving] = useState(false);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
  const [historyFor, setHistoryFor] = useState<Automation | null>(null);
  const [history, setHistory] = useState<AutomationRun[]>([]);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  const timezoneOptions = useMemo(() => {
    const values = [browserTimezone(), 'Asia/Shanghai', 'UTC', 'Asia/Tokyo', 'America/Los_Angeles', 'America/New_York', 'Europe/London'];
    return [...new Set(values)].map((value) => ({ value, label: value }));
  }, []);

  const startCreate = () => {
    setForm(emptyInput());
    setFormError(null);
    setEditing('new');
  };

  const startEdit = (item: Automation) => {
    setForm(toInput(item));
    setFormError(null);
    setEditing(item);
  };

  const save = async () => {
    if (!form.name.trim() || !form.task_title.trim() || !form.target_agent_id || !form.timezone) {
      setFormError(t('automationRequired'));
      return;
    }
    setSaving(true);
    setFormError(null);
    try {
      const input = {
        ...form,
        name: form.name.trim(), task_title: form.task_title.trim(), task_description: form.task_description.trim(),
        schedule_weekday: form.schedule_type === 'weekly' ? (form.schedule_weekday ?? 1) : undefined,
      };
      if (editing === 'new') await automation.create(input);
      else if (editing) await automation.update(editing.id, input);
      setEditing(null);
    } catch (err) {
      setFormError(err instanceof ApiError ? err.message : t('automationSaveError'));
    } finally {
      setSaving(false);
    }
  };

  const runNow = async (item: Automation) => {
    setBusyId(item.id);
    try {
      const run = await automation.runNow(item.id);
      if (run.status === 'skipped' && run.failure_reason === 'target_unavailable') {
        showToast(t('automationTargetUnavailable'), 'error');
      } else {
        showToast(t('automationRunStarted'), 'success');
        onTaskCreated();
      }
    } catch (err) {
      if (err instanceof ApiError && err.code === 'automation_already_running') {
        showToast(t('automationAlreadyRunning'), 'info');
      } else {
        showToast(t('automationRunError'), 'error');
      }
    } finally {
      setBusyId(null);
    }
  };

  const toggle = async (item: Automation) => {
    setBusyId(item.id);
    try {
      await automation.update(item.id, { ...toInput(item), enabled: !item.enabled });
    } catch {
      showToast(t('automationSaveError'), 'error');
    } finally {
      setBusyId(null);
    }
  };

  const remove = async (item: Automation) => {
    setBusyId(item.id);
    try {
      await automation.remove(item.id);
      setConfirmDeleteId(null);
    } catch {
      showToast(t('automationDeleteError'), 'error');
    } finally {
      setBusyId(null);
    }
  };

  const showHistory = async (item: Automation) => {
    setHistoryFor(item);
    setHistoryLoading(true);
    try {
      setHistory(await automation.listRuns(item.id));
    } catch {
      setHistory([]);
      showToast(t('automationLoadError'), 'error');
    } finally {
      setHistoryLoading(false);
    }
  };

  return (
    <section data-testid="automation-workspace" className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
      <div className="mb-4 flex items-start justify-between gap-4 border-b-2 border-black pb-4">
        <div className="flex items-center gap-2">
          {(editing || historyFor) && (
            <button type="button" onClick={() => { setEditing(null); setHistoryFor(null); }} className="flex h-8 w-8 items-center justify-center border-2 border-black bg-white shadow-brutal-sm" aria-label={t('back')}>
              <ArrowLeft className="h-4 w-4" />
            </button>
          )}
          <div>
            <h2 className="font-heading text-lg font-bold text-foreground">{editing ? (editing === 'new' ? t('automationNew') : t('edit')) : historyFor ? t('automationHistory') : t('automationManage')}</h2>
            {!editing && !historyFor && <p className="mt-1 max-w-xl font-body text-sm text-muted-foreground">{t('automationDescription')}</p>}
          </div>
        </div>
        {!editing && !historyFor && <Button type="button" size="sm" onClick={startCreate}><Plus className="mr-1.5 h-4 w-4" />{t('automationNew')}</Button>}
      </div>

      {editing ? (
        <div className="space-y-4">
          <div>
            <Label htmlFor="automation-name" className="mb-1.5 block">{t('automationName')}</Label>
            <Input id="automation-name" value={form.name} onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))} placeholder={t('automationNamePlaceholder')} />
          </div>
          <div>
            <Label htmlFor="automation-title" className="mb-1.5 block">{t('automationTaskTitle')}</Label>
            <Input id="automation-title" value={form.task_title} onChange={(event) => setForm((current) => ({ ...current, task_title: event.target.value }))} placeholder={t('automationTaskTitlePlaceholder')} />
          </div>
          <div>
            <Label htmlFor="automation-description" className="mb-1.5 block">{t('automationTaskDescription')}</Label>
            <Textarea id="automation-description" rows={4} value={form.task_description} onChange={(event) => setForm((current) => ({ ...current, task_description: event.target.value }))} placeholder={t('automationTaskDescriptionPlaceholder')} />
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <div>
              <Label className="mb-1.5 block">{t('automationTargetAgent')}</Label>
              <Select aria-label={t('automationTargetAgent')} size="md" value={form.target_agent_id} onChange={(value) => setForm((current) => ({ ...current, target_agent_id: value }))} options={[{ value: '', label: t('automationChooseAgent') }, ...agents.map((agent) => ({ value: agent.member_id, label: agent.display_name }))]} />
            </div>
            <div>
              <Label className="mb-1.5 block">{t('automationFrequency')}</Label>
              <Select aria-label={t('automationFrequency')} size="md" value={form.schedule_type} onChange={(value) => setForm((current) => ({ ...current, schedule_type: value as AutomationScheduleType, schedule_weekday: value === 'weekly' ? (current.schedule_weekday ?? 1) : undefined }))} options={[
                { value: 'daily', label: t('automationDaily') },
                { value: 'weekdays', label: t('automationWeekdays') },
                { value: 'weekly', label: t('automationWeekly') },
              ]} />
            </div>
            {form.schedule_type === 'weekly' && (
              <div>
                <Label className="mb-1.5 block">{t('automationWeekday')}</Label>
                <Select aria-label={t('automationWeekday')} size="md" value={String(form.schedule_weekday ?? 1)} onChange={(value) => setForm((current) => ({ ...current, schedule_weekday: Number(value) }))} options={WEEKDAYS.map((key, index) => ({ value: String(index), label: t(key) }))} />
              </div>
            )}
            <div>
              <Label id="automation-time-label" className="mb-1.5 block">{t('automationTime')}</Label>
              <div role="group" aria-labelledby="automation-time-label" className="grid grid-cols-[1fr_auto_1fr] items-center gap-2">
                <Select aria-label={t('automationHour')} size="md" value={String(form.schedule_hour)} onChange={(value) => setForm((current) => ({ ...current, schedule_hour: Number(value) }))} options={HOUR_OPTIONS} />
                <span aria-hidden className="font-heading text-base font-bold text-foreground">:</span>
                <Select aria-label={t('automationMinute')} size="md" value={String(form.schedule_minute)} onChange={(value) => setForm((current) => ({ ...current, schedule_minute: Number(value) }))} options={MINUTE_OPTIONS} />
              </div>
            </div>
            <div>
              <Label className="mb-1.5 block">{t('automationTimezone')}</Label>
              <Select aria-label={t('automationTimezone')} size="md" value={form.timezone} onChange={(value) => setForm((current) => ({ ...current, timezone: value }))} options={timezoneOptions} />
            </div>
          </div>
          <label className="flex cursor-pointer items-start gap-3 border-2 border-black bg-brutal-cream p-3 shadow-brutal-sm">
            <input type="checkbox" checked={form.enabled} onChange={(event) => setForm((current) => ({ ...current, enabled: event.target.checked }))} className="mt-0.5 h-4 w-4 accent-black" />
            <span><span className="block font-heading text-sm font-bold">{t('automationEnable')}</span><span className="font-body text-xs text-muted-foreground">{t('automationEnabledHint')}</span></span>
          </label>
          {formError && <div role="alert" className="border-2 border-brutal-danger bg-brutal-danger-light p-3 font-body text-sm text-foreground">{formError}</div>}
          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" onClick={() => setEditing(null)}>{t('cancel')}</Button>
            <Button type="button" onClick={() => void save()} disabled={saving}>{saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}{editing === 'new' ? t('automationCreate') : t('automationUpdate')}</Button>
          </div>
        </div>
      ) : historyFor ? (
        <div className="space-y-3">
          <div className="border-2 border-black bg-brutal-cream p-3"><div className="font-heading font-bold">{historyFor.name}</div><div className="mt-1 font-mono text-xs text-muted-foreground">{scheduleLabel(historyFor)}</div></div>
          {historyLoading ? <div className="flex justify-center py-10"><Loader2 className="h-6 w-6 animate-spin" /></div> : history.length === 0 ? <div className="border-2 border-dashed border-black/40 p-8 text-center font-body text-sm text-muted-foreground">{t('automationNoHistory')}</div> : history.map((run) => (
            <div key={run.id} className="flex items-start justify-between gap-4 border-2 border-black bg-white p-3 shadow-brutal-sm">
              <div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><span className="font-heading text-sm font-bold">{runStatusLabel(run.status)}</span><span className="border border-black bg-brutal-cream px-1.5 py-0.5 font-mono text-[10px] uppercase">{run.source === 'manual' ? t('automationSourceManual') : t('automationSourceScheduled')}</span>{run.task_number && <span className="font-mono text-xs">#{run.task_number}</span>}</div>{runReason(run.failure_reason) && <div className="mt-1 font-body text-xs text-muted-foreground">{runReason(run.failure_reason)}</div>}</div>
              <time className="shrink-0 font-mono text-[10px] text-muted-foreground">{formatDate(run.created_at)}</time>
            </div>
          ))}
        </div>
      ) : (
        <div>
          {automation.isLoading ? <div className="flex justify-center py-12"><Loader2 className="h-7 w-7 animate-spin" /></div> : automation.error ? <div className="border-2 border-brutal-danger bg-brutal-danger-light p-4 font-body text-sm text-foreground">{automation.error}</div> : automation.automations.length === 0 ? <div className="border-2 border-dashed border-black/40 p-10 text-center"><CalendarClock className="mx-auto mb-3 h-8 w-8" /><div className="font-heading font-bold">{t('automationEmpty')}</div><div className="mt-1 font-body text-sm text-muted-foreground">{t('automationEmptyHint')}</div></div> : <div className="space-y-3">{automation.automations.map((item) => (
            <div key={item.id} data-automation-id={item.id} className="border-2 border-black bg-white p-4 shadow-brutal-sm">
              <div className="flex items-start justify-between gap-3"><div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><h3 className="font-heading font-bold">{item.name}</h3><span className={`border border-black px-1.5 py-0.5 font-mono text-[10px] uppercase ${item.enabled ? 'bg-brutal-success-light' : 'bg-brutal-cream'}`}>{item.enabled ? t('enabled') : t('disabled')}</span></div><div className="mt-1 truncate font-body text-sm">{item.task_title}</div><div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 font-mono text-[11px] text-muted-foreground"><span className="flex items-center gap-1"><CalendarClock className="h-3 w-3" />{scheduleLabel(item)}</span><span className="flex items-center gap-1"><Bot className="h-3 w-3" />{item.target_agent_name || t('unknown')}</span></div></div></div>
              <div className="mt-3 grid gap-2 border-t-2 border-black/20 pt-3 text-xs sm:grid-cols-2"><div><span className="font-heading font-bold">{t('automationNextRun')}:</span> <span className="font-body">{item.enabled ? formatDate(item.next_run_at) : t('disabled')}</span></div><div><span className="font-heading font-bold">{t('automationLastResult')}:</span> <span className="font-body">{runStatusLabel(item.last_run?.status)}</span></div></div>
              {confirmDeleteId === item.id ? <div className="mt-3 flex flex-wrap items-center justify-between gap-2 border-2 border-brutal-danger bg-brutal-danger-light p-2"><span className="font-body text-xs text-foreground">{t('automationDeleteConfirm')}</span><div className="flex gap-2"><Button size="sm" variant="outline" onClick={() => setConfirmDeleteId(null)}>{t('cancel')}</Button><Button size="sm" variant="danger" onClick={() => void remove(item)} disabled={busyId === item.id}>{t('delete')}</Button></div></div> : <div className="mt-3 flex flex-wrap gap-2"><Button type="button" size="sm" onClick={() => void runNow(item)} disabled={busyId === item.id}>{busyId === item.id ? <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : <Play className="mr-1.5 h-3.5 w-3.5" />}{busyId === item.id ? t('automationRunning') : t('automationRunNow')}</Button><Button type="button" size="sm" variant="outline" onClick={() => void toggle(item)} disabled={busyId === item.id}><Pause className="mr-1.5 h-3.5 w-3.5" />{item.enabled ? t('automationPause') : t('automationResume')}</Button><Button type="button" size="sm" variant="outline" onClick={() => startEdit(item)}><Pencil className="mr-1.5 h-3.5 w-3.5" />{t('edit')}</Button><Button type="button" size="sm" variant="outline" onClick={() => void showHistory(item)}><History className="mr-1.5 h-3.5 w-3.5" />{t('automationHistory')}</Button><Button type="button" size="sm" variant="outline" onClick={() => setConfirmDeleteId(item.id)} aria-label={t('delete')}><Trash2 className="h-3.5 w-3.5" /></Button></div>}
            </div>
          ))}</div>}
        </div>
      )}
    </section>
  );
}
