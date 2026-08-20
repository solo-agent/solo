'use client';

import { useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { apiClient } from '@/lib/api-client';
import { agentRunStatusText, displayAgentActivity } from '@/lib/agent-activity';
import type { AgentRunStatus } from '@/lib/agent-run-types';
import { getLocale, t } from '@/lib/i18n';
import { useWebSocket } from '@/lib/ws-context';
import { cn } from '@/lib/utils';
import { detailSectionTitleClass } from '@/components/ui/detail-section';
import { formatTokens } from '@/components/budget/budget-settings-card';

interface AgentRun {
  id: string;
  agent_id: string;
  session_id?: string;
  status: AgentRunStatus;
  activity_text: string;
  tool_name?: string;
  tool_input_summary?: string;
  source?: string;
  transcript_path?: string;
  started_at: string;
  backend_started_at?: string;
  updated_at: string;
  budget_state?: string;
  reserved_tokens?: number;
  actual_tokens?: number;
  input_tokens?: number;
  output_tokens?: number;
  cache_read_tokens?: number;
  cache_write_tokens?: number;
  token_overrun?: boolean;
}

interface AgentSession {
  id: string;
  provider: string;
  external_session_id?: string;
  transcript_path?: string;
  status: string;
  started_at: string;
  last_active_at: string;
}

interface AgentTaskSummary {
  id: string;
  task_number: number;
  title: string;
  status: string;
  last_run_id: string;
  last_run_status: string;
  last_activity: string;
  last_run_at: string;
  linked_run_count: number;
}

interface AgentTranscriptEntry {
  seq: number;
  timestamp?: string;
  role: string;
  type: string;
  text?: string;
  tool_name?: string;
  input?: unknown;
}

interface AgentRunEvent {
  id: string;
  run_id: string;
  seq: number;
  type: string;
  message: string;
  tool_name?: string;
  payload?: Record<string, unknown>;
  created_at: string;
}

type Scope = 'sessions' | 'tasks' | 'runs';

export function AgentObservabilityTab({ agentId, initialRunId }: { agentId: string; initialRunId?: string | null }) {
  const { onEvent } = useWebSocket();
  const [scope, setScope] = useState<Scope>('sessions');
  const [sessions, setSessions] = useState<AgentSession[]>([]);
  const [runs, setRuns] = useState<AgentRun[]>([]);
  const [tasks, setTasks] = useState<AgentTaskSummary[]>([]);
  const [taskRuns, setTaskRuns] = useState<AgentRun[]>([]);
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(null);
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null);
  const [selectedRunId, setSelectedRunId] = useState<string | null>(null);
  const [transcript, setTranscript] = useState<AgentTranscriptEntry[]>([]);
  const [events, setEvents] = useState<AgentRunEvent[]>([]);
  const [transcriptRefreshTick, setTranscriptRefreshTick] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setSelectedSessionId(null);
    setSelectedTaskId(null);
    setSelectedRunId(null);
    setTranscript([]);
    setEvents([]);
    Promise.all([
      apiClient.get<AgentSession[]>(`/api/v1/agents/${agentId}/sessions`).catch(() => []),
      apiClient.get<AgentRun[]>(`/api/v1/agents/${agentId}/runs`).catch(() => []),
      apiClient.get<AgentTaskSummary[]>(`/api/v1/agents/${agentId}/tasks`).catch(() => []),
    ]).then(([nextSessions, nextRuns, nextTasks]) => {
      if (cancelled) return;
      const safeSessions = Array.isArray(nextSessions) ? nextSessions : [];
      const safeRuns = Array.isArray(nextRuns) ? nextRuns : [];
      const safeTasks = Array.isArray(nextTasks) ? nextTasks : [];
      const preferredRun = initialRunId ? safeRuns.find((run) => run.id === initialRunId) : undefined;
      setSessions(safeSessions);
      setRuns(safeRuns);
      setTasks(safeTasks);
      setScope(preferredRun ? 'runs' : 'sessions');
      setSelectedSessionId(preferredRun?.session_id ?? safeSessions[0]?.id ?? null);
      setSelectedRunId(preferredRun?.id ?? safeRuns[0]?.id ?? null);
    });
    return () => {
      cancelled = true;
    };
  }, [agentId, initialRunId]);

  useEffect(() => onEvent((event) => {
    if (
      event.type === 'agent.run.started' ||
      event.type === 'agent.run.updated' ||
      event.type === 'agent.run.finished'
    ) {
      if (event.agent_id !== agentId) return;
      const nextRun: AgentRun = {
        id: event.run_id,
        agent_id: event.agent_id,
        session_id: event.session_id,
        status: event.status,
        activity_text: event.activity_text ?? '',
        tool_name: event.tool_name,
        tool_input_summary: event.tool_input_summary,
        source: event.source,
        transcript_path: event.transcript_path,
        ...(event.budget_state ? {
          budget_state: event.budget_state,
          reserved_tokens: event.reserved_tokens,
          actual_tokens: event.actual_tokens,
          input_tokens: event.input_tokens,
          output_tokens: event.output_tokens,
          cache_read_tokens: event.cache_read_tokens,
          cache_write_tokens: event.cache_write_tokens,
          token_overrun: event.token_overrun,
        } : {}),
        started_at: event.timestamp ?? new Date().toISOString(),
        updated_at: event.timestamp ?? new Date().toISOString(),
      };
      setRuns((prev) => upsertRun(prev, nextRun));
      if (!selectedRunId) setSelectedRunId(event.run_id);
      if (event.run_id === selectedRunId) setTranscriptRefreshTick((tick) => tick + 1);
      if (event.type === 'agent.run.started' || event.type === 'agent.run.finished') {
        apiClient.get<AgentSession[]>(`/api/v1/agents/${agentId}/sessions`)
          .then((items) => setSessions(Array.isArray(items) ? items : []))
          .catch(() => {});
        apiClient.get<AgentTaskSummary[]>(`/api/v1/agents/${agentId}/tasks`)
          .then((items) => setTasks(Array.isArray(items) ? items : []))
          .catch(() => {});
      }
      return;
    }
    if (event.type === 'agent.run.event' && event.run_id === selectedRunId) {
      const nextEvent: AgentRunEvent = {
        id: event.id ?? `${event.run_id}-${event.seq}`,
        run_id: event.run_id,
        seq: event.seq,
        type: event.event_type,
        message: event.message ?? '',
        tool_name: event.tool_name,
        payload: event.payload,
        created_at: event.timestamp,
      };
      setEvents((prev) => upsertEvent(prev, nextEvent));
      setTranscriptRefreshTick((tick) => tick + 1);
    }
  }), [agentId, onEvent, selectedRunId]);

  useEffect(() => {
    if (!selectedTaskId) {
      setTaskRuns([]);
      return;
    }
    let cancelled = false;
    apiClient.get<AgentRun[]>(`/api/v1/tasks/${selectedTaskId}/runs`)
      .then((items) => {
        if (cancelled) return;
        setTaskRuns(items);
        setSelectedRunId(items[0]?.id ?? null);
      })
      .catch(() => {
        if (!cancelled) setTaskRuns([]);
      });
    return () => {
      cancelled = true;
    };
  }, [selectedTaskId]);

  useEffect(() => {
    if (!selectedRunId) {
      setTranscript([]);
      setEvents([]);
      return;
    }
    let cancelled = false;
    apiClient.get<AgentTranscriptEntry[]>(`/api/v1/agent-runs/${selectedRunId}/transcript`)
      .then((items) => {
        if (!cancelled) setTranscript(Array.isArray(items) ? items : []);
      })
      .catch(() => {
        if (!cancelled) setTranscript([]);
      });
    apiClient.get<AgentRunEvent[]>(`/api/v1/agent-runs/${selectedRunId}/events`)
      .then((items) => {
        if (!cancelled) setEvents(Array.isArray(items) ? items : []);
      })
      .catch(() => {
        if (!cancelled) setEvents([]);
      });
    return () => {
      cancelled = true;
    };
  }, [selectedRunId, transcriptRefreshTick]);

  const visibleRuns = useMemo(() => {
    if (scope === 'tasks' && selectedTaskId) return taskRuns;
    if (scope === 'sessions' && selectedSessionId) {
      return runs.filter((run) => run.session_id === selectedSessionId);
    }
    return runs;
  }, [runs, scope, selectedSessionId, selectedTaskId, taskRuns]);
  const selectedRun = useMemo(
    () => [...runs, ...taskRuns].find((run) => run.id === selectedRunId),
    [runs, selectedRunId, taskRuns],
  );
  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <span className={detailSectionTitleClass()}>{t('observabilityAgentRunHistory')}</span>
        <div className="flex overflow-hidden rounded-lg border border-border font-heading text-xs font-bold">
          {(['sessions', 'tasks', 'runs'] as const).map((item) => (
            <button
              key={item}
              type="button"
              onClick={() => setScope(item)}
              className={cn('border-r border-border px-2 py-1 last:border-r-0', scope === item ? 'bg-brutal-primary' : 'bg-white')}
            >
              {scopeLabel(item)}
            </button>
          ))}
        </div>
      </div>

      <div className="grid gap-3 md:grid-cols-2">
        <ScopeList
          scope={scope}
          sessions={sessions}
          tasks={tasks}
          runs={runs}
          selectedSessionId={selectedSessionId}
          selectedTaskId={selectedTaskId}
          selectedRunId={selectedRunId}
          onSelectSession={(id) => {
            setSelectedSessionId(id);
            setSelectedTaskId(null);
            setSelectedRunId(runs.find((run) => run.session_id === id)?.id ?? null);
          }}
          onSelectTask={(id, runId) => {
            setSelectedTaskId(id);
            setSelectedSessionId(null);
            setSelectedRunId(runId);
          }}
          onSelectRun={setSelectedRunId}
        />
        <RunList runs={visibleRuns} selectedRunId={selectedRunId} onSelectRun={setSelectedRunId} />
        <div className="md:col-span-2">
          <TranscriptPanel entries={transcript} events={events} selectedRun={selectedRun} />
        </div>
      </div>
    </section>
  );
}

function ScopeList(props: {
  scope: Scope;
  sessions: AgentSession[];
  tasks: AgentTaskSummary[];
  runs: AgentRun[];
  selectedSessionId: string | null;
  selectedTaskId: string | null;
  selectedRunId: string | null;
  onSelectSession: (id: string) => void;
  onSelectTask: (id: string, runId: string) => void;
  onSelectRun: (id: string) => void;
}) {
  if (props.scope === 'sessions') {
    return (
      <Panel title={t('observabilitySessions')}>
        {props.sessions.map((session) => (
          <Row key={session.id} active={props.selectedSessionId === session.id} onClick={() => props.onSelectSession(session.id)}>
            <strong>{session.provider}</strong>
            <span>{session.external_session_id ? session.external_session_id.slice(0, 8) : session.id.slice(0, 8)}</span>
            <small>{formatTime(session.last_active_at)}</small>
          </Row>
        ))}
      </Panel>
    );
  }
  if (props.scope === 'tasks') {
    return (
      <Panel title={t('observabilityTasks')}>
        {props.tasks.map((task) => (
          <Row key={task.id} active={props.selectedTaskId === task.id} onClick={() => props.onSelectTask(task.id, task.last_run_id)}>
            <strong>#{task.task_number} {task.title}</strong>
            <span>{task.status} · {t('observabilityRuns', { n: task.linked_run_count })}</span>
            <small>{formatTime(task.last_run_at)}</small>
          </Row>
        ))}
      </Panel>
    );
  }
  return (
    <Panel title={t('observabilityRunCount')}>
      {props.runs.map((run) => (
        <Row key={run.id} active={props.selectedRunId === run.id} onClick={() => props.onSelectRun(run.id)}>
          <strong>{agentRunStatusText(run.status)}</strong>
          <span>{displayAgentActivity(run.status, run.activity_text, run.tool_input_summary, run.id.slice(0, 8))}</span>
          <RunCostLine run={run} />
          <small>{formatTime(run.updated_at)}</small>
        </Row>
      ))}
    </Panel>
  );
}

function RunList({ runs, selectedRunId, onSelectRun }: { runs: AgentRun[]; selectedRunId: string | null; onSelectRun: (id: string) => void }) {
  return (
    <Panel title={t('observabilityRelatedRuns')}>
      {runs.map((run) => (
        <Row key={run.id} active={selectedRunId === run.id} onClick={() => onSelectRun(run.id)}>
          <strong>{agentRunStatusText(run.status)}</strong>
          <span>{displayAgentActivity(run.status, run.activity_text, run.tool_input_summary, run.id.slice(0, 8))}</span>
          <RunCostLine run={run} />
          <small>{formatTime(run.updated_at)}</small>
        </Row>
      ))}
    </Panel>
  );
}

function TranscriptPanel({ entries, events, selectedRun }: { entries: AgentTranscriptEntry[]; events: AgentRunEvent[]; selectedRun?: AgentRun }) {
  const fallback = <EventsTimeline events={events} />;
  const selectedRunId = selectedRun?.id ?? null;
  const transcriptPath = selectedRun?.transcript_path;
  return (
    <Panel title={t('observabilityRunTranscript')}>
      <RecoverySummary events={events} />
      {!selectedRunId ? (
        <div className="p-3 text-sm text-muted-foreground">{t('observabilitySelectRun')}</div>
      ) : entries.length > 0 ? (
        <div className="space-y-2 p-2">
          <RunTokenSummary run={selectedRun} />
          {transcriptPath && (
            <div className="truncate rounded-md border border-border bg-white px-2 py-1 font-mono text-[11px] text-muted-foreground">
              {transcriptPath}
            </div>
          )}
          {entries.map((entry) => (
            <details key={`${entry.seq}-${entry.type}`} className="overflow-hidden rounded-lg border border-border bg-white" open={entry.type !== 'tool_use' && entry.type !== 'tool_result'}>
              <summary className="cursor-pointer px-2 py-1 font-heading text-xs font-bold">
                {entryLabel(entry)} <span className="font-mono font-normal text-muted-foreground">{formatTime(entry.timestamp)}</span>
              </summary>
              <div className="border-t border-border p-2 text-sm">
                {entry.input ? (
                  <pre className="max-h-56 overflow-auto whitespace-pre-wrap break-words bg-black p-2 font-mono text-xs text-white">
                    {JSON.stringify(entry.input, null, 2)}
                  </pre>
                ) : (
                  <div className="whitespace-pre-wrap break-words">{entry.text || entry.type}</div>
                )}
              </div>
            </details>
          ))}
        </div>
      ) : !transcriptPath ? (
        <>
          <RunTokenSummary run={selectedRun} />
          <div className="border-b border-border p-3 text-sm text-muted-foreground">{t('observabilityNoTranscriptPath')}</div>
          {fallback}
        </>
      ) : (
        <>
          <RunTokenSummary run={selectedRun} />
          <div className="border-b border-border p-3 text-sm text-muted-foreground">{t('observabilityUnreadableTranscriptPath', { path: transcriptPath })}</div>
          {fallback}
        </>
      )}
    </Panel>
  );
}

function RecoverySummary({ events }: { events: AgentRunEvent[] }) {
  const error = [...events].reverse().find((event) => event.type === 'error');
  const started = events.find((event) => event.type === 'run_started');
  const scheduled = events.find((event) => event.type === 'task_recovery_scheduled');
  const blocked = events.find((event) => event.type === 'task_recovery_blocked');
  const exhausted = events.find((event) => event.type === 'task_retry_exhausted');
  const taskLinked = events.some((event) => event.type === 'task_linked');
  const recovery = asRecord(started?.payload?.recovery) ?? scheduled?.payload ?? blocked?.payload ?? exhausted?.payload;
  const failureCode = stringValue(recovery?.failure_code) || stringValue(error?.payload?.failure_code);
  if (!taskLinked && !recovery && !blocked && !exhausted) return null;

  const attempt = numberValue(recovery?.attempt) || numberValue(recovery?.attempts);
  const maxAttempts = numberValue(recovery?.max_attempts);
  const previousRunID = stringValue(recovery?.previous_run_id);
  const mode = stringValue(recovery?.mode);
  const workspaceReused = recovery?.workspace_reused === true;
  const action = exhausted
    ? t('observabilityRecoveryExhausted')
    : blocked
      ? t('observabilityRecoveryNeedsHuman')
      : scheduled || recovery
        ? t('observabilityRecoveryScheduled')
        : t('observabilityRecoveryPending');

  return (
    <div className="border-b border-border bg-brutal-primary-light p-3 text-foreground">
      <div className="font-heading text-xs font-black uppercase tracking-wider">{t('observabilityRecoveryTitle')}</div>
      <div className="mt-2 grid gap-2 text-xs sm:grid-cols-2">
        <RecoveryField label={t('observabilityRecoveryReason')} value={failureLabel(failureCode)} />
        {attempt > 0 && (
          <RecoveryField
            label={t('observabilityRecoveryAttempt')}
            value={maxAttempts > 0 ? `${attempt}/${maxAttempts}` : String(attempt)}
          />
        )}
        {previousRunID && previousRunID !== events[0]?.run_id && (
          <RecoveryField label={t('observabilityRecoveryPreviousRun')} value={previousRunID.slice(0, 8)} />
        )}
        {mode && (
          <RecoveryField
            label={t('observabilityRecoveryConversation')}
            value={mode === 'fresh_session' ? t('observabilityRecoveryFreshConversation') : t('observabilityRecoveryResumeConversation')}
          />
        )}
        {workspaceReused && <RecoveryField label={t('observabilityRecoveryWorkspace')} value={t('observabilityRecoverySameWorkspace')} />}
        <RecoveryField label={t('observabilityRecoveryCurrentAction')} value={action} />
        {(blocked || exhausted) && <RecoveryField label={t('observabilityRecoveryNextOwner')} value={t('observabilityRecoveryTaskCreator')} />}
      </div>
    </div>
  );
}

function RunCostLine({ run }: { run: AgentRun }) {
  if (!run.budget_state) return null;
  if (run.budget_state === 'usage_unknown') return <span className="text-brutal-danger">{t('runCostUnknown')} · {formatTokens(run.reserved_tokens ?? 0)}</span>;
  if (run.actual_tokens !== undefined) return <span>{t('runCostActual')} {formatTokens(run.actual_tokens)}</span>;
  return <span>{t('runCostReserved')} {formatTokens(run.reserved_tokens ?? 0)}</span>;
}

function RunTokenSummary({ run }: { run?: AgentRun }) {
  if (!run?.budget_state) return null;
  return (
    <div className="rounded-lg border border-border bg-brutal-primary-light px-2 py-2 text-foreground">
      <div className="font-heading text-xs font-black">{t('runCostTitle')}</div>
      <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 font-mono text-[11px]">
        <span>{t('runCostStatus')}: {budgetStateText(run.budget_state)}</span>
        <span>{t('runCostReserved')}: {formatTokens(run.reserved_tokens ?? 0)}</span>
        <span>{t('runCostActual')}: {run.actual_tokens === undefined ? '-' : formatTokens(run.actual_tokens)}</span>
        <span>{t('runTokenInput')}: {formatTokens(run.input_tokens ?? 0)}</span>
        <span>{t('runTokenOutput')}: {formatTokens(run.output_tokens ?? 0)}</span>
        <span>{t('runTokenCacheRead')}: {formatTokens(run.cache_read_tokens ?? 0)}</span>
        <span>{t('runTokenCacheWrite')}: {formatTokens(run.cache_write_tokens ?? 0)}</span>
        {run.token_overrun && <span className="bg-brutal-warning px-1 text-foreground">{t('runCostOverrun')}</span>}
      </div>
    </div>
  );
}

function RecoveryField({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-border bg-white px-2 py-1.5">
      <div className="font-mono text-[10px] font-bold text-muted-foreground">{label}</div>
      <div className="mt-0.5 font-heading text-xs font-bold text-black">{value}</div>
    </div>
  );
}

function failureLabel(code: string) {
  if (code === 'daemon_lost') return t('observabilityFailureDaemonLost');
  if (code === 'timeout') return t('observabilityFailureTimeout');
  if (code === 'provider_transient') return t('observabilityFailureProviderTransient');
  if (code === 'missing_visible_result') return t('observabilityFailureMissingVisibleResult');
  if (code === 'configuration') return t('observabilityFailureConfiguration');
  return t('observabilityFailureUnknown');
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : undefined;
}

function stringValue(value: unknown) {
  return typeof value === 'string' ? value : '';
}

function numberValue(value: unknown) {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0;
}

function budgetStateText(state: string): string {
  if (state === 'pending') return t('runTokenPending');
  if (state === 'settled') return t('runTokenSettled');
  if (state === 'released') return t('runTokenReleased');
  if (state === 'usage_unknown') return t('runCostUnknown');
  return state;
}

function EventsTimeline({ events }: { events: AgentRunEvent[] }) {
  if (events.length === 0) {
    return <div className="p-3 text-sm text-muted-foreground">{t('observabilityNoEvents')}</div>;
  }
  const toolNameByCallId = new Map<string, string>();
  for (const event of events) {
    const callID = typeof event.payload?.call_id === 'string' ? event.payload.call_id : '';
    if (callID && event.tool_name) {
      toolNameByCallId.set(callID, event.tool_name);
    }
  }
  return (
    <div className="space-y-2 p-2">
      {events.map((event) => {
        const message = readableEventText(event.message || payloadText(event.payload) || t('observabilityNoSummary'));
        const meta = eventMeta(event.payload);
        const callID = typeof event.payload?.call_id === 'string' ? event.payload.call_id : '';
        const toolName = event.tool_name || toolNameByCallId.get(callID) || '';
        return (
          <details key={event.id} className="overflow-hidden rounded-lg border border-border bg-white">
            <summary className="flex cursor-pointer items-center gap-2 px-2 py-1 font-heading text-xs font-bold">
              <span>#{event.seq}</span>
              <span>{eventLabel(event.type, toolName)}</span>
              <span className="ml-auto font-mono font-normal text-muted-foreground">{formatTime(event.created_at)}</span>
            </summary>
            <div className="space-y-2 border-t border-border p-2">
              {meta.length > 0 && (
                <div className="flex flex-wrap gap-1">
                  {meta.map(([key, value]) => (
                    <span key={key} className="rounded-md border border-border bg-brutal-cream px-1.5 py-0.5 font-mono text-[11px]">
                      {key}: {value}
                    </span>
                  ))}
                </div>
              )}
              <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words bg-brutal-cream p-2 font-mono text-xs leading-5">
                {message}
              </pre>
            </div>
          </details>
        );
      })}
    </div>
  );
}

function readableEventText(value: string) {
  const trimmed = value.trim();
  if (trimmed.startsWith('"') && trimmed.endsWith('"')) {
    try {
      const parsed = JSON.parse(trimmed);
      if (typeof parsed === 'string') return parsed;
    } catch {
      // fall through
    }
  }
  return value.replaceAll('\\n', '\n').replaceAll('\\"', '"');
}

function payloadText(payload?: Record<string, unknown>) {
  const value = payload?.output ?? payload?.input;
  if (typeof value === 'string') return value;
  if (value == null) return '';
  return JSON.stringify(value, null, 2);
}

function eventMeta(payload?: Record<string, unknown>) {
  if (!payload) return [];
  return Object.entries(payload)
    .filter(([key]) => key !== 'output' && key !== 'input' && key !== 'call_id')
    .map(([key, value]) => [key, typeof value === 'string' ? readableEventText(value) : String(value)] as const);
}

function eventLabel(type: string, toolName: string) {
  if (type === 'tool_started') return t('observabilityToolCall', { name: toolName || t('observabilityTool') });
  if (type === 'tool_finished') return t('observabilityToolResult', { name: toolName || t('observabilityTool') });
  if (type === 'thinking') return t('observabilityThinking');
  if (type === 'assistant_message') return t('observabilityAssistant');
  if (type === 'user_message_received') return t('observabilityUser');
  if (type === 'task_linked') return t('observabilityTaskLinked');
  if (type === 'run_started') return t('observabilityRunStarted');
  if (type === 'task_recovery_scheduled') return t('observabilityRecoveryScheduled');
  if (type === 'task_recovery_blocked') return t('observabilityRecoveryNeedsHuman');
  if (type === 'task_retry_exhausted') return t('observabilityRecoveryExhausted');
  if (type === 'done') return t('observabilityDone');
  if (type === 'error') return t('observabilityError');
  return type;
}

function upsertRun(runs: AgentRun[], nextRun: AgentRun) {
  const existing = runs.find((run) => run.id === nextRun.id);
  const merged = existing ? { ...existing, ...nextRun, started_at: existing.started_at } : nextRun;
  const rest = runs.filter((run) => run.id !== nextRun.id);
  return [merged, ...rest].sort((a, b) => Date.parse(b.updated_at) - Date.parse(a.updated_at));
}

function upsertEvent(events: AgentRunEvent[], nextEvent: AgentRunEvent) {
  const rest = events.filter((event) => event.id !== nextEvent.id);
  return [...rest, nextEvent].sort((a, b) => a.seq - b.seq);
}

function Panel({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="min-h-[260px] overflow-hidden rounded-xl border border-border bg-brutal-cream">
      <div className="border-b border-border bg-white px-2 py-1 font-heading text-xs font-bold">{title}</div>
      <div className="max-h-[520px] overflow-auto">{children}</div>
    </div>
  );
}

function Row({ active, onClick, children }: { active: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn('block w-full border-b border-border px-2 py-2 text-left last:border-b-0 hover:bg-white', active && 'bg-white')}
    >
      <div className="flex min-w-0 flex-col gap-0.5 font-mono text-xs">{children}</div>
    </button>
  );
}

function entryLabel(entry: AgentTranscriptEntry) {
  if (entry.type === 'thinking') return t('observabilityThinking');
  if (entry.type === 'tool_use') return t('observabilityToolCall', { name: entry.tool_name || t('observabilityTool') });
  if (entry.type === 'tool_result') return t('observabilityToolResult', { name: entry.tool_name || t('observabilityTool') });
  return entry.role === 'user' ? t('observabilityUser') : t('observabilityAssistant');
}

function formatTime(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '-';
  return date.toLocaleString(getLocale(), { hour12: false });
}

function scopeLabel(scope: Scope) {
  if (scope === 'sessions') return t('observabilitySessions');
  if (scope === 'tasks') return t('observabilityTasks');
  return t('observabilityRunCount');
}
